package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"veylix/backend/internal/audit"
)

// Service implements registration and login.
type Service struct {
	db      *sql.DB
	audit   *audit.Logger
	limiter *RateLimiter

	// autoVerify marks self-registered accounts verified on creation. It is for
	// tests whose subject is NOT e-mail verification; production leaves it false so
	// a new client must confirm before signing in (D-039.6).
	autoVerify bool

	// SendVerification issues the confirmation for a newly registered account.
	// It is a function rather than a *EmailVerifier so this package keeps no
	// construction order dependency, and nil means "nowhere to send" — the
	// registration still succeeds and the caller reports `outbox`.
	//
	// Without this hook Register created an unverified account and e-mailed
	// nothing, so a new client could never confirm and was locked out at the
	// login door with a message claiming a code had been sent (D-041).
	SendVerification func(ctx context.Context, userID int64, email string) (delivery string, err error)
}

// NewService builds the auth service.
func NewService(db *sql.DB, auditLog *audit.Logger) *Service {
	return &Service{
		db:    db,
		audit: auditLog,
		// 10 attempts burst, then one every 5 minutes — enough for a human who
		// mistypes twice and retries, useless for a spray. Raised from 5 after a
		// real user locked themselves out while testing the login form
		// (new_update.md §7 rate limiting).
		limiter: NewRateLimiter(10, 1.0/300.0),
	}
}

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// SetAutoVerify makes self-registration create verified accounts. Tests only:
// production must send the confirmation code before a new client can sign in.
func (s *Service) SetAutoVerify(on bool) { s.autoVerify = on }

// SetEmailVerifiedForTest marks an account verified. Tests only: production goes
// through auth.EmailVerifier (a single-use token delivered by e-mail).
func (s *Service) SetEmailVerifiedForTest(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET email_verified = 1 WHERE id = ?`, userID)
	return err
}

// usernameTaken reports whether a login name is already in use.
func (s *Service) usernameTaken(ctx context.Context, username string) bool {
	if username == "" {
		return false
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE username = ?`, username).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// freeUsername derives a unique login name from the e-mail prefix.
//
// A user may pick a different one later (PATCH /api/me/profile); this only gives
// every account a working username at creation, so "sign in with your username"
// is true from the first second rather than after a profile visit.
func (s *Service) freeUsername(ctx context.Context, email string) (string, error) {
	base := strings.SplitN(email, "@", 2)[0]
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		}
	}
	base = b.String()
	if len(base) < 3 {
		base = "user" + base
	}
	if len(base) > 28 {
		base = base[:28]
	}
	candidate := base
	for i := 2; i < 100; i++ {
		var n int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE username = ?`, candidate).Scan(&n); err != nil {
			return "", fmt.Errorf("auth: username lookup: %w", err)
		}
		if n == 0 {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s%d", base, i)
	}
	return "", errors.New("auth: could not allocate a username")
}

// ErrEmailTaken is returned when the address already exists.
var ErrEmailTaken = errors.New("auth: email already registered")

// ErrUsernameTaken is returned when the requested login name is already in use.
var ErrUsernameTaken = errors.New("auth: username already taken")

// RegisterInput is the public registration payload.
type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
}

// User is the safe projection returned to clients (never contains a hash).
type User struct {
	ID            int64  `json:"id"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name"`
	Role          string `json:"role"`
	EmailVerified bool   `json:"email_verified"`
	// VerificationDelivery says HOW the confirmation link was handed over:
	// "smtp" when a relay delivered it, "outbox" when none is configured (so the
	// link exists only on the server), "" when no send was attempted because the
	// account was created verified. Register fills it; Authenticate leaves it empty.
	//
	// It exists because the API used to answer 201 + a session cookie and say
	// nothing about verification, so a client walked away believing the account
	// worked and was refused at the login door minutes later. A caller can now
	// tell the person what actually happened.
	VerificationDelivery string `json:"verification_delivery,omitempty"`
}

// Register creates a client account.
//
// Self-registration always yields RoleClient: a caller can never choose its own
// role (Bible §8 "Never trust a client-supplied role").
func (s *Service) Register(ctx context.Context, in RegisterInput) (*User, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !emailPattern.MatchString(email) || len(email) > 190 {
		return nil, fmt.Errorf("auth: invalid email address")
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	// A verified address may sign in straight away; an unverified one must confirm
	// first (see Authenticate). Tests set AutoVerifyRegistration so a flow that is
	// not about verification does not have to click a link.
	verified := 0
	if s.autoVerify {
		verified = 1
	}

	now := time.Now().Unix()
	username, err := s.freeUsername(ctx, email)
	if err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, username, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'ACTIVE', ?, ?, ?)`,
		email, username, hash, strings.TrimSpace(in.DisplayName), string(RoleClient), verified, now, now,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("auth: create user: %w", err)
	}
	id, _ := res.LastInsertId()

	// Send the confirmation link. A failure here must NOT undo the account: the
	// person is registered and an operator can re-issue the link from their
	// profile. The outcome is recorded so the caller can report it honestly
	// instead of implying an e-mail always went out (D-041).
	delivery := ""
	if !s.autoVerify && s.SendVerification != nil {
		sent, vErr := s.SendVerification(ctx, id, email)
		delivery = sent
		entry := audit.Entry{
			ActorID: id, Action: "auth.verification_requested", TargetType: "user",
			TargetID: fmt.Sprint(id), Context: map[string]any{"delivery": sent},
		}
		if vErr != nil {
			entry.Context["error"] = vErr.Error()
		}
		_ = s.audit.Record(ctx, entry)
	}

	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: id, Action: audit.ActionRegister, TargetType: "user",
		TargetID: fmt.Sprint(id), Context: map[string]any{
			"role": string(RoleClient), "verified": verified == 1, "delivery": delivery,
		},
	})

	// Reports the row that was actually written, not an optimistic guess: before
	// this returned no `EmailVerified`, so a caller could not tell a verified
	// account from one that will be refused at the login door.
	return &User{
		ID: id, Email: email, DisplayName: in.DisplayName, Role: string(RoleClient),
		EmailVerified: verified == 1, VerificationDelivery: delivery,
	}, nil
}

// Portal is which door a sign-in attempt came through.
//
// It exists because the product has two front doors (D-041): clients sign in at
// /login, staff (admin + super admin) at /staff/login. The portal only decides
// WHICH roles may pass — it never grants access, so a client account can never
// reach an admin screen by choosing the staff door.
type Portal string

const (
	// PortalClient is the customer door: only RoleClient may pass.
	PortalClient Portal = "client"
	// PortalStaff is the operator door: admin and super admin may pass.
	PortalStaff Portal = "staff"
)

// Valid reports whether p is a known portal.
func (p Portal) Valid() bool { return p == PortalClient || p == PortalStaff }

var (
	// ErrWrongPortal means the credentials are right but the account belongs at the
	// other door. It is deliberately distinct from ErrInvalidCredentials so the UI
	// can say "use the staff login" instead of "wrong password".
	ErrWrongPortal = errors.New("auth: this account signs in through the other portal")
	// ErrEmailNotVerified means the account exists, the password is right, and the
	// address has not been confirmed yet (D-039.6). Sign-in is refused so an
	// unverified address can never be used to reach the platform.
	ErrEmailNotVerified = errors.New("auth: e-mail is not verified")
	// ErrPasswordNotSet means the account exists but was created through Google and
	// therefore has NO password to check. It is deliberately distinct from
	// ErrInvalidCredentials: telling a Google-only customer "email or password is
	// incorrect" sends them to a reset flow for a password that was never set, and
	// they have no way to discover the real answer. The account exists, the password
	// is absent — those are different facts and deserve different instructions.
	ErrPasswordNotSet = errors.New("auth: this account has no password (created with Google)")
)

// dummyHash is verified against when no account matched, so an unknown identifier
// costs the same wall-clock time as a wrong password. Without it, response timing
// is an account-enumeration oracle.
var dummyHash = "$2a$12$C6UzMDM.H6dfI/f/IKcEeO9WiGvVdKXBZ8Z8Z8Z8Z8Z8Z8Z8Z8Z8Z"

// FindUserByIdentifier resolves a login name (e-mail OR username) to its account
// without checking a password and without revealing whether the account exists.
//
// It exists for the password-reset flow, which must look the account up in order
// to mail it a code while still answering every request identically — so it
// returns (nil, nil) when nothing matches instead of an error the caller could
// turn into an existence oracle.
func (s *Service) FindUserByIdentifier(ctx context.Context, identifier string) (*User, error) {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if identifier == "" {
		return nil, nil
	}
	var (
		id    int64
		email string
		role  string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, role
		   FROM users
		  WHERE email = ? OR (username IS NOT NULL AND username <> '' AND username = ?)
		  LIMIT 1`, identifier, identifier,
	).Scan(&id, &email, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: find account: %w", err)
	}
	return &User{ID: id, Email: email, Role: role}, nil
}

// Authenticate verifies credentials and returns the user.
//
// `identifier` is an e-mail address OR a username (D-041). The same error is
// returned for an unknown identifier and a wrong password, and the rate limiter is
// consulted first, so neither the response nor its timing distinguishes the two.
func (s *Service) Authenticate(ctx context.Context, identifier, password, ip string, portal Portal) (*User, error) {
	if !portal.Valid() {
		return nil, fmt.Errorf("auth: unknown portal %q", portal)
	}
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if identifier == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	// Two limits: per client address (a password spray from one host) and per
	// account (a targeted attack from many hosts).
	if !s.limiter.Allow("login:"+ip) || !s.limiter.Allow("login-id:"+identifier) {
		// A Retry-After the caller can act on. Shared state on the Service would
		// race between concurrent requests, so the wait travels in the error.
		wait := s.limiter.RetryAfter("login:" + ip)
		if w := s.limiter.RetryAfter("login-id:" + identifier); w > wait {
			wait = w
		}
		_ = s.audit.Record(ctx, audit.Entry{
			Action: audit.ActionLoginFailed, TargetType: "user", TargetID: identifier,
			Result:  "DENIED",
			Context: map[string]any{"reason": "rate_limited", "retry_after_s": int(wait.Seconds())},
		})
		return nil, &RateLimitedError{RetryAfter: wait}
	}

	var (
		id     int64
		email  string
		hash   sql.NullString
		role   string
		status string
		disp   string
		verif  int
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, role, status, display_name, email_verified
		   FROM users
		  WHERE email = ? OR (username IS NOT NULL AND username <> '' AND username = ?)
		  LIMIT 1`, identifier, identifier,
	).Scan(&id, &email, &hash, &role, &status, &disp, &verif)

	if errors.Is(err, sql.ErrNoRows) {
		// Burn the same CPU a real verification would, then answer identically.
		_ = VerifyPassword(dummyHash, password)
		_ = s.audit.Record(ctx, audit.Entry{
			Action: audit.ActionLoginFailed, TargetType: "user", TargetID: identifier,
			Result: "DENIED", Context: map[string]any{"reason": "no_such_account"},
		})
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: lookup user: %w", err)
	}
	// A Google-only account has NO password stored. That is a different fact from
	// a wrong password, and it must be said separately or the customer is sent to a
	// reset flow for a credential that never existed.
	//
	// The distinction is only revealed to somebody who named a real address, which
	// is not a leak worth trading for: "this e-mail exists" is already answerable
	// from the registration and reset forms (D-041 already accepts that), and the
	// alternative is a permanent dead end for every Google customer.
	if !hash.Valid {
		_ = s.audit.Record(ctx, audit.Entry{
			ActorID: id, Action: audit.ActionLoginFailed, TargetType: "user",
			TargetID: fmt.Sprint(id), Result: "DENIED",
			Context: map[string]any{"reason": "password_not_set"},
		})
		return nil, ErrPasswordNotSet
	}
	if !VerifyPassword(hash.String, password) {
		_ = s.audit.Record(ctx, audit.Entry{
			ActorID: id, Action: audit.ActionLoginFailed, TargetType: "user",
			TargetID: fmt.Sprint(id), Result: "DENIED",
		})
		return nil, ErrInvalidCredentials
	}
	if status != "ACTIVE" {
		return nil, ErrForbidden
	}

	// Portal check happens AFTER the password, so it can never be used to learn
	// whether an account exists without knowing its password.
	if !roleAllowedInPortal(role, portal) {
		_ = s.audit.Record(ctx, audit.Entry{
			ActorID: id, Action: audit.ActionLoginFailed, TargetType: "user",
			TargetID: fmt.Sprint(id), Result: "DENIED",
			Context: map[string]any{"reason": "wrong_portal", "portal": string(portal)},
		})
		return nil, ErrWrongPortal
	}

	// Unverified addresses are refused at the door. Accounts created by an operator
	// (super admin, admins) and Google accounts are verified on creation, so this
	// only affects self-registered clients who have not confirmed yet.
	//
	// The password was CORRECT, so this attempt must not spend the rate-limit
	// budget: a person waiting for a confirmation link would otherwise hit 429
	// after a handful of honest tries and be told to "wait a few minutes" for a
	// problem that waiting cannot fix. Only a real failure counts against them.
	if verif != 1 {
		s.limiter.Reset("login:" + ip)
		s.limiter.Reset("login-id:" + identifier)
		return nil, ErrEmailNotVerified
	}

	// Opportunistically upgrade an older hash after a successful login.
	if NeedsRehash(hash.String) {
		if newHash, hErr := HashPassword(password); hErr == nil {
			_, _ = s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
				newHash, time.Now().Unix(), id)
		}
	}

	now := time.Now().Unix()
	_, _ = s.db.ExecContext(ctx,
		`UPDATE users SET last_login_at = ?, last_login_ip = ? WHERE id = ?`, now, truncate(ip, 64), id)

	// Clear both counters: a person who signs in successfully is not an attacker.
	s.limiter.Reset("login:" + ip)
	s.limiter.Reset("login-id:" + identifier)
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: id, Action: audit.ActionLogin, TargetType: "user", TargetID: fmt.Sprint(id),
	})

	return &User{ID: id, Email: email, DisplayName: disp, Role: role, EmailVerified: true}, nil
}

// roleAllowedInPortal encodes which role may use which front door.
func roleAllowedInPortal(role string, portal Portal) bool {
	switch portal {
	case PortalClient:
		return role == string(RoleClient)
	case PortalStaff:
		return role == string(RoleAdmin) || role == string(RoleSuperAdmin)
	}
	return false
}

// BootstrapSuperAdmin creates the first super admin when the users table is
// empty. It refuses to run when any user exists, so it can never be used to
// escalate an existing installation (Bible §14.2: no default password).
func (s *Service) BootstrapSuperAdmin(ctx context.Context, email, password, displayName string) (*User, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return nil, fmt.Errorf("auth: count users: %w", err)
	}
	if count > 0 {
		return nil, ErrBootstrapNotAllowed
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if !emailPattern.MatchString(email) {
		return nil, fmt.Errorf("auth: invalid email address")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES (?, ?, ?, 'super_admin', 'ACTIVE', 1, ?, ?)`,
		email, hash, displayName, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("auth: bootstrap: %w", err)
	}
	id, _ := res.LastInsertId()

	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: id, Action: "auth.bootstrap", TargetType: "user", TargetID: fmt.Sprint(id),
	})

	return &User{ID: id, Email: email, DisplayName: displayName, Role: string(RoleSuperAdmin), EmailVerified: true}, nil
}

// ErrPasswordMismatch is returned when the supplied current password is wrong.
// It is distinct from ErrInvalidCredentials so the caller can show a precise
// message without making login errors more informative.
var ErrPasswordMismatch = errors.New("auth: current password is incorrect")

// VerifyPassword reports whether `password` is the account's current password.
//
// It is the re-authentication check for a sensitive self-service change (an
// e-mail move): a stolen session must not be enough. An unknown account, or one
// with no password (Google-only), answers false — never an error page.
func (s *Service) VerifyPassword(ctx context.Context, userID int64, password string) bool {
	if password == "" {
		return false
	}
	var stored sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&stored); err != nil {
		return false
	}
	return stored.Valid && stored.String != "" && VerifyPassword(stored.String, password)
}

// ResetStaffPassword sets a new password for another account and ends every one of
// its sessions. It is the Super Admin's recovery path (D-044), and it exists
// because a reseller whose password is lost has no e-mail relay to recover with.
//
// The password arrives already-generated (`auth.GeneratePassword`), and the full
// policy is applied here, so a weak value can never be set through this path.
func (s *Service) ResetStaffPassword(ctx context.Context, actor Principal, userID int64, newPassword string) error {
	if !actor.IsSuperAdmin() {
		return ErrForbidden
	}
	if actor.UserID == userID {
		// Changing your OWN password must go through ChangePassword, which asks
		// for the current one. A reset that skips that would let a borrowed
		// super-admin session take over the account permanently.
		return ErrCannotModifySelf
	}
	if _, err := s.loadUser(ctx, userID); err != nil {
		return err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		hash, time.Now().Unix(), userID); err != nil {
		return fmt.Errorf("auth: reset password: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("auth: revoke sessions after reset: %w", err)
	}
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: actor.UserID, Action: audit.ActionPasswordChange, TargetType: "user",
		TargetID: fmt.Sprint(userID), Context: map[string]any{"source": "super_admin_reset"},
	})
	return nil
}

// SetUserRole promotes or demotes an account between admin and super admin.
//
// Deliberately narrow: only those two roles, only by a super admin, never for
// yourself (an operator must not be able to demote the only way back in), and
// every session is revoked because the account's power just changed.
func (s *Service) SetUserRole(ctx context.Context, actor Principal, userID int64, role Role) (*UserRow, error) {
	if !actor.IsSuperAdmin() {
		return nil, ErrForbidden
	}
	if role != RoleAdmin && role != RoleSuperAdmin {
		return nil, fmt.Errorf("auth: role must be admin or super_admin")
	}
	if actor.UserID == userID {
		return nil, ErrCannotModifySelf
	}
	target, err := s.loadUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if target.Role == string(RoleClient) {
		// A client is not staff: promoting one here would skip the whole account
		// model. Staff accounts are created by CreateUserByRole.
		return nil, ErrUserNotFound
	}
	if target.Role == string(role) {
		return target, nil // idempotent: re-stating the same role changes nothing
	}
	/*
	 * P-1: promoting to `super_admin` is only legal when the seat is empty.
	 *
	 * Without this, `SetUserRole` was a second door into the same power that
	 * `CreateUserByRole` guards — the unique index (0028) would refuse the UPDATE,
	 * but with a constraint error instead of an explanation. Demoting the existing
	 * owner is a deliberate, audited act; it must happen first.
	 */
	if role == RoleSuperAdmin {
		var existing int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE role = 'super_admin' AND id <> ?`, userID).Scan(&existing); err != nil {
			return nil, fmt.Errorf("auth: count super admins: %w", err)
		}
		if existing > 0 {
			return nil, ErrSuperAdminExists
		}
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET role = ?, updated_at = ? WHERE id = ?`,
		string(role), time.Now().Unix(), userID); err != nil {
		return nil, fmt.Errorf("auth: set role: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return nil, fmt.Errorf("auth: revoke sessions after role change: %w", err)
	}
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: actor.UserID, Action: audit.ActionRoleChange, TargetType: "user",
		TargetID: fmt.Sprint(userID), Context: map[string]any{"from": target.Role, "to": string(role)},
	})
	return s.loadUser(ctx, userID)
}

// ChangePassword verifies the current password, applies the full policy to the
// new one, and rotates the stored hash.
//
// It deliberately does NOT revoke sessions: the caller decides (the HTTP layer
// revokes every session, so a stolen cookie cannot outlive the password change
// — see RevokeAllForUser). Accounts that signed in through OAuth only have no
// password row; they may set one without a current password.
func (s *Service) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	var stored sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnauthenticated
	}
	if err != nil {
		return fmt.Errorf("auth: lookup password: %w", err)
	}

	// Refuse before doing any expensive work if the new password is unusable.
	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return err
	}

	if stored.Valid && stored.String != "" {
		if !VerifyPassword(stored.String, currentPassword) {
			_ = s.audit.Record(ctx, audit.Entry{
				ActorID: userID, Action: audit.ActionPasswordChangeFailed, TargetType: "user",
				TargetID: fmt.Sprint(userID), Result: "DENIED",
			})
			return ErrPasswordMismatch
		}
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		hash, time.Now().Unix(), userID); err != nil {
		return fmt.Errorf("auth: update password: %w", err)
	}

	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: userID, Action: audit.ActionPasswordChange, TargetType: "user",
		TargetID: fmt.Sprint(userID),
	})
	return nil
}

// ErrRateLimited is returned when too many failed attempts occur.
var ErrRateLimited = errors.New("auth: too many attempts, try again later")

// RateLimitedError carries how long the caller must wait before trying again.
//
// It wraps ErrRateLimited, so every existing `errors.Is(err, ErrRateLimited)`
// keeps working, while the HTTP layer can add a Retry-After header. Five typos
// used to lock a person out for five minutes with no explanation; the lockout
// is deliberate, but the silence was not.
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string { return ErrRateLimited.Error() }

func (e *RateLimitedError) Unwrap() error { return ErrRateLimited }

// ErrBootstrapNotAllowed prevents a second bootstrap.
var ErrBootstrapNotAllowed = errors.New("auth: bootstrap already completed")

// ErrUserNotFound is returned when a management lookup misses.
var ErrUserNotFound = errors.New("auth: user not found")

// ErrCannotModifySelf prevents a super admin from locking themselves out.
var ErrCannotModifySelf = errors.New("auth: you cannot change your own status or role")

// ErrSuperAdminExists is returned when a second Super Admin is requested.
//
// P-1: the platform has exactly ONE owner. Enforced in the database by
// `idx_users_single_super_admin` (0028); this sentinel exists so the operator gets
// a sentence they can act on ("there is already an owner — demote them first")
// instead of a raw UNIQUE-constraint error.
var ErrSuperAdminExists = errors.New("auth: the platform already has a Super Admin; demote them first")

// ErrNotAnAdmin keeps server assignment off accounts that have no reseller scope.
var ErrNotAnAdmin = errors.New("auth: target account is not an admin")

// ErrServerNotFound is returned when an assignment names a server that is gone.
var ErrServerNotFound = errors.New("auth: server not found")

// AdminServers returns the server ids assigned to one reseller.
func (s *Service) AdminServers(ctx context.Context, adminID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT server_id FROM admin_server_assignments WHERE admin_id = ? ORDER BY server_id`, adminID)
	if err != nil {
		return nil, fmt.Errorf("auth: list admin servers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]int64, 0, 4)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("auth: scan admin server: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetAdminServers replaces one reseller's server assignments.
//
// Replace rather than merge keeps the call idempotent — the shape a checkbox UI
// needs. Every id is validated before anything is deleted, so a typo cannot
// leave a reseller with no access at all. Only `role='admin'` accounts may hold
// assignments: a client has no reseller scope, and a super admin already sees
// every server, so a row for either would be meaningless state.
func (s *Service) SetAdminServers(ctx context.Context, adminID int64, serverIDs []int64) error {
	ids := dedupeInt64(serverIDs)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("auth: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var role string
	err = tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, adminID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("auth: load admin: %w", err)
	}
	if role != string(RoleAdmin) {
		return ErrNotAnAdmin
	}

	for _, sid := range ids {
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM server_integrations WHERE id = ? AND status <> 'REMOVED'`, sid).Scan(&n); err != nil {
			return fmt.Errorf("auth: check server: %w", err)
		}
		if n == 0 {
			return ErrServerNotFound
		}
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM admin_server_assignments WHERE admin_id = ?`, adminID); err != nil {
		return fmt.Errorf("auth: clear assignments: %w", err)
	}
	now := time.Now().Unix()
	for _, sid := range ids {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO admin_server_assignments (admin_id, server_id, created_at) VALUES (?, ?, ?)`,
			adminID, sid, now); err != nil {
			return fmt.Errorf("auth: assign server %d: %w", sid, err)
		}
	}
	return tx.Commit()
}

// dedupeInt64 drops invalid and repeated ids while preserving order.
func dedupeInt64(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// UserRow is the management projection of an account (never a hash).
type UserRow struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	QuotaLimit  int    `json:"quota_limit"`
	CreatedAt   int64  `json:"created_at"`
}

// CreateUserInput is the privileged account-creation payload.
type CreateUserInput struct {
	Role        Role
	Email       string
	Password    string
	DisplayName string
	QuotaLimit  int
}

// CreateUserByRole creates an account with an explicitly chosen role.
//
// This is the only path that may mint an `admin` or a second `super_admin`;
// self-registration (`Register`) is hard-wired to `client`. The caller MUST
// already have been authorised by the transport layer, and the actor is
// recorded in the audit trail (Bible §8: never trust a client-supplied role).
func (s *Service) CreateUserByRole(ctx context.Context, actor Principal, in CreateUserInput) (*UserRow, error) {
	if !actor.IsSuperAdmin() {
		return nil, ErrForbidden
	}
	if !in.Role.Valid() {
		return nil, fmt.Errorf("auth: unknown role %q", in.Role)
	}
	/*
	 * P-1: the platform has exactly ONE Super Admin.
	 *
	 * The unique index (0028) is the real guard — it cannot be bypassed by a code
	 * path that forgets this check. Checking here as well turns a raw
	 * UNIQUE-constraint error into a sentence the operator can act on, and it
	 * avoids hashing a password (argon2id, 64 MB) for a request that cannot
	 * succeed.
	 */
	if in.Role == RoleSuperAdmin {
		var existing int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE role = 'super_admin'`).Scan(&existing); err != nil {
			return nil, fmt.Errorf("auth: count super admins: %w", err)
		}
		if existing > 0 {
			return nil, ErrSuperAdminExists
		}
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !emailPattern.MatchString(email) || len(email) > 190 {
		return nil, fmt.Errorf("auth: invalid email address")
	}

	// HashPassword applies the full policy (length/entropy/blocklist).
	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	// Only admins carry a reseller quota; everyone else gets 0.
	limit := 0
	if in.Role == RoleAdmin {
		limit = in.QuotaLimit
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, quota_limit, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'ACTIVE', 1, ?, ?, ?)`,
		email, hash, strings.TrimSpace(in.DisplayName), string(in.Role), limit, now, now,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("auth: create user: %w", err)
	}
	id, _ := res.LastInsertId()

	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: actor.UserID, Action: "user.create", TargetType: "user",
		TargetID: fmt.Sprint(id), Context: map[string]any{"role": string(in.Role)},
	})

	return &UserRow{
		ID: id, Email: email, DisplayName: strings.TrimSpace(in.DisplayName),
		Role: string(in.Role), Status: "ACTIVE", QuotaLimit: limit, CreatedAt: now,
	}, nil
}

// ListUsers returns accounts, optionally filtered to one role ("" = all).
func (s *Service) ListUsers(ctx context.Context, role Role) ([]UserRow, error) {
	query := `SELECT id, email, display_name, role, status, COALESCE(quota_limit, 0), created_at
	            FROM users`
	args := []any{}
	if role != "" {
		query += ` WHERE role = ?`
		args = append(args, string(role))
	}
	query += ` ORDER BY id`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("auth: list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]UserRow, 0, 16)
	for rows.Next() {
		var u UserRow
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.QuotaLimit, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("auth: scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// loadUser re-reads one account for the management projections.
func (s *Service) loadUser(ctx context.Context, userID int64) (*UserRow, error) {
	var u UserRow
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, display_name, role, status, COALESCE(quota_limit, 0), created_at
		   FROM users WHERE id = ?`, userID).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.Role, &u.Status, &u.QuotaLimit, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth: reload user: %w", err)
	}
	return &u, nil
}

// SetUserStatus suspends or reactivates an account.
//
// Suspension revokes every session immediately, so a suspended admin cannot
// keep using a cookie issued before the decision. A super admin may never
// suspend themselves (lockout prevention).
func (s *Service) SetUserStatus(ctx context.Context, actor Principal, userID int64, status string) (*UserRow, error) {
	if !actor.IsSuperAdmin() {
		return nil, ErrForbidden
	}
	if status != "ACTIVE" && status != "SUSPENDED" {
		return nil, fmt.Errorf("auth: invalid status %q", status)
	}
	if actor.UserID == userID {
		return nil, ErrCannotModifySelf
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().Unix(), userID)
	if err != nil {
		return nil, fmt.Errorf("auth: set status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrUserNotFound
	}

	if status == "SUSPENDED" {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
			return nil, fmt.Errorf("auth: revoke sessions: %w", err)
		}
	}

	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: actor.UserID, Action: "user.status_change", TargetType: "user",
		TargetID: fmt.Sprint(userID), Context: map[string]any{"status": status},
	})

	return s.loadUser(ctx, userID)
}

// SetQuotaLimit updates an admin's active-assignment quota ceiling.
// Only the `admin` role carries a quota, so other roles return ErrUserNotFound
// (which the transport renders as 404 — never "this account exists but is not
// an admin", which would leak role information).
func (s *Service) SetQuotaLimit(ctx context.Context, actor Principal, userID int64, limit int) (*UserRow, error) {
	if !actor.IsSuperAdmin() {
		return nil, ErrForbidden
	}
	if limit < 0 {
		return nil, fmt.Errorf("auth: quota limit cannot be negative")
	}
	// Any STAFF account may be given a ceiling, not only an admin: promoting an
	// admin to super admin used to make this statement match zero rows, so a later
	// quota change answered 404 for an account the operator was looking at.
	// A client is still refused — self-registration owns that account model.
	var role string
	if err := s.db.QueryRowContext(ctx,
		`SELECT role FROM users WHERE id = ?`, userID).Scan(&role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("auth: lookup role for quota: %w", err)
	}
	if Role(role) != RoleAdmin && Role(role) != RoleSuperAdmin {
		return nil, ErrUserNotFound
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET quota_limit = ?, updated_at = ? WHERE id = ?`,
		limit, time.Now().Unix(), userID); err != nil {
		return nil, fmt.Errorf("auth: set quota: %w", err)
	}
	_ = s.audit.Record(ctx, audit.Entry{
		ActorID: actor.UserID, Action: audit.ActionQuotaAdjust, TargetType: "user",
		TargetID: fmt.Sprint(userID), Context: map[string]any{"quota_limit": limit},
	})
	return s.loadUser(ctx, userID)
}

// BootstrapRequired reports whether the installation has no accounts yet.
// The setup wizard uses it to choose between /setup and /login.
func (s *Service) BootstrapRequired(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return false, fmt.Errorf("auth: count users: %w", err)
	}
	return count == 0, nil
}

// isUniqueViolation detects a SQLite UNIQUE constraint failure without
// importing driver-specific error types.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// Only a UNIQUE conflict. The earlier version also matched "constraint failed",
	// which modernc.org/sqlite emits for ANY constraint (CHECK, NOT NULL, FK) — so
	// a malformed INSERT was reported to the user as "email already registered",
	// and a first-ever registration could never succeed.
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
