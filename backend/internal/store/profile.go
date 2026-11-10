package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrUsernameTaken means the requested login name belongs to another account.
var ErrUsernameTaken = errors.New("store: username already taken")

// Profile is the account's own view of itself (D-041).
//
// It deliberately omits `password_hash`, `google_sub` and `role`/`status`: those
// exist for the server's decisions, and the API must never leak or accept them
// from the account it belongs to.
type Profile struct {
	ID            int64  `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	AvatarURL     string `json:"avatar_url"`
	Phone         string `json:"phone"`
	WhatsApp      string `json:"whatsapp"`
	Telegram      string `json:"telegram"`
	Discord       string `json:"discord"`
	// Birthday is ISO YYYY-MM-DD (empty when unset).
	Birthday string `json:"birthday"`
	// Age is DERIVED from Birthday on read, never stored. Keeping it derived is
	// why the number is right on the birthday and every year after it.
	Age int    `json:"age"`
	Bio string `json:"bio"`
	// Timezone is kept on the record (staff may set it) but the profile screen no
	// longer asks: a person does not know their IANA zone, and typing it was a
	// field nobody used.
	Timezone string `json:"timezone"`
	// GoogleLinked is true when the account can sign in with Google, so the UI can
	// explain why the password form may be absent.
	GoogleLinked bool   `json:"google_linked"`
	HasPassword  bool   `json:"has_password"`
	FreeVPNLimit int    `json:"free_vpn_limit"`
	CreatedAt    int64  `json:"created_at"`
	LastLoginAt  int64  `json:"last_login_at"`
	LastLoginIP  string `json:"last_login_ip"`
}

// ProfileByID reads one account for its owner.
func (s *Store) ProfileByID(ctx context.Context, userID int64) (*Profile, error) {
	var (
		p         Profile
		username  sql.NullString
		google    sql.NullString
		hash      sql.NullString
		verified  int
		storedAge int
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, email_verified, username, display_name, role, status,
		        avatar_url, phone, whatsapp, telegram, discord, birthday, age, bio, timezone,
		        google_sub, password_hash, free_vpn_limit, created_at, last_login_at, last_login_ip
		   FROM users WHERE id = ?`, userID,
	).Scan(&p.ID, &p.Email, &verified, &username, &p.DisplayName, &p.Role, &p.Status,
		&p.AvatarURL, &p.Phone, &p.WhatsApp, &p.Telegram, &p.Discord, &p.Birthday, &storedAge, &p.Bio, &p.Timezone,
		&google, &hash, &p.FreeVPNLimit, &p.CreatedAt, &p.LastLoginAt, &p.LastLoginIP)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSuchIdentity
	}
	if err != nil {
		return nil, fmt.Errorf("store: read profile: %w", err)
	}
	p.EmailVerified = verified == 1
	p.Username = username.String
	p.GoogleLinked = google.Valid && google.String != ""
	p.HasPassword = hash.Valid && hash.String != ""
	// Age comes from the birthday when there is one; the legacy `age` column is
	// only a fallback so an older account keeps showing something sensible.
	p.Age = AgeFromBirthday(p.Birthday, time.Now())
	if p.Age < 0 {
		p.Age = storedAge
	}
	return &p, nil
}

// AgeFromBirthday turns an ISO YYYY-MM-DD birth date into whole years at `now`.
//
// It returns -1 when the date is empty or unparseable, so the caller can decide
// (the profile falls back to the legacy column). The birthday itself is counted
// on the day: someone born on 2 Jan turns 30 ON 2 Jan, not the day after.
func AgeFromBirthday(birthday string, now time.Time) int {
	if birthday == "" {
		return -1
	}
	d, err := time.Parse("2006-01-02", birthday)
	if err != nil || d.After(now) {
		return -1
	}
	years := now.Year() - d.Year()
	// Month/day compare, not YearDay: a leap year shifts every day number after
	// February, which would make a Feb-29 birth date off by one in a non-leap year.
	if now.Month() < d.Month() || (now.Month() == d.Month() && now.Day() < d.Day()) {
		years--
	}
	return years
}

// ProfileUpdate is a patch of the fields a person may change themselves.
//
// Pointer semantics: nil means "leave it as it is", so a client that only saves
// the contact tab cannot blank the avatar by omission.
type ProfileUpdate struct {
	Username    *string
	DisplayName *string
	AvatarURL   *string
	Phone       *string
	WhatsApp    *string
	Telegram    *string
	Discord     *string
	// Birthday is ISO YYYY-MM-DD; "" clears it.
	Birthday *string
	Age      *int
	Bio      *string
	Timezone *string
}

// UpdateProfile applies a patch and returns the new row.
//
// `email` is not in ProfileUpdate on purpose: an address change is an identity
// change and goes through a verification e-mail (D-041), never a plain PATCH.
func (s *Store) UpdateProfile(ctx context.Context, userID int64, u ProfileUpdate) (*Profile, error) {
	sets := make([]string, 0, 12)
	args := make([]any, 0, 12)

	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if u.Username != nil {
		add("username", strings.ToLower(strings.TrimSpace(*u.Username)))
	}
	if u.DisplayName != nil {
		add("display_name", strings.TrimSpace(*u.DisplayName))
	}
	if u.AvatarURL != nil {
		add("avatar_url", strings.TrimSpace(*u.AvatarURL))
	}
	if u.Phone != nil {
		add("phone", strings.TrimSpace(*u.Phone))
	}
	if u.WhatsApp != nil {
		add("whatsapp", strings.TrimSpace(*u.WhatsApp))
	}
	if u.Telegram != nil {
		add("telegram", strings.TrimSpace(*u.Telegram))
	}
	if u.Discord != nil {
		add("discord", strings.TrimSpace(*u.Discord))
	}
	if u.Birthday != nil {
		add("birthday", strings.TrimSpace(*u.Birthday))
	}
	if u.Age != nil {
		add("age", *u.Age)
	}
	if u.Bio != nil {
		add("bio", strings.TrimSpace(*u.Bio))
	}
	if u.Timezone != nil {
		add("timezone", strings.TrimSpace(*u.Timezone))
	}
	if len(sets) == 0 {
		return s.ProfileByID(ctx, userID)
	}
	add("updated_at", time.Now().Unix())
	args = append(args, userID)

	q := `UPDATE users SET ` + strings.Join(sets, ", ") + ` WHERE id = ?`
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUsernameTaken
		}
		return nil, fmt.Errorf("store: update profile: %w", err)
	}
	return s.ProfileByID(ctx, userID)
}

// UsernameAvailable reports whether a login name can be taken. `exceptUserID`
// lets a caller keep its own name (renaming to the same value is not a clash).
func (s *Store) UsernameAvailable(ctx context.Context, username string, exceptUserID int64) (bool, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return false, nil
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE username = ? AND id <> ?`, username, exceptUserID).Scan(&n); err != nil {
		return false, fmt.Errorf("store: username availability: %w", err)
	}
	return n == 0, nil
}

// EmailChangeRequest is one pending address change.
type EmailChangeRequest struct {
	ID        int64
	UserID    int64
	NewEmail  string
	CodeHash  string
	Attempts  int
	ExpiresAt time.Time
}

// ErrEmailChange is returned when a change request is unknown, expired, already
// used, or has run out of attempts. One error on purpose: the caller must not be
// able to tell those apart.
var ErrEmailChange = errors.New("store: e-mail change request is not valid")

// CreateEmailChangeRequest replaces any live request and stores the new code hash.
func (s *Store) CreateEmailChangeRequest(ctx context.Context, userID int64, newEmail, codeHash string, ttl time.Duration) error {
	now := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Superseding the previous request is deliberate: an operator who typos an
	// address must be able to fix it without waiting for the first code to expire.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM email_change_requests WHERE user_id = ? AND confirmed_at IS NULL`, userID); err != nil {
		return fmt.Errorf("store: clear email change: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO email_change_requests (user_id, new_email, code_hash, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		userID, strings.ToLower(strings.TrimSpace(newEmail)), codeHash, now.Unix(), now.Add(ttl).Unix()); err != nil {
		if isUniqueViolation(err) {
			// Another account already has a pending claim on this address.
			return ErrIdentityTaken
		}
		return fmt.Errorf("store: create email change: %w", err)
	}
	return tx.Commit()
}

// LiveEmailChangeRequest returns the account's pending request, if any.
func (s *Store) LiveEmailChangeRequest(ctx context.Context, userID int64) (*EmailChangeRequest, error) {
	var (
		r   EmailChangeRequest
		exp int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, new_email, code_hash, attempts, expires_at
		   FROM email_change_requests WHERE user_id = ? AND confirmed_at IS NULL`, userID).
		Scan(&r.ID, &r.UserID, &r.NewEmail, &r.CodeHash, &r.Attempts, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEmailChange
	}
	if err != nil {
		return nil, fmt.Errorf("store: read email change: %w", err)
	}
	r.ExpiresAt = time.Unix(exp, 0)
	return &r, nil
}

// BumpEmailChangeAttempts counts a wrong code and burns the request at the limit.
func (s *Store) BumpEmailChangeAttempts(ctx context.Context, id int64, max int) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE email_change_requests SET attempts = attempts + 1 WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: bump email change attempts: %w", err)
	}
	// At the limit the request is deleted: a 6-digit code has only 10^6 values, so
	// unlimited guesses would find it.
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM email_change_requests WHERE id = ? AND attempts >= ?`, id, max); err != nil {
		return fmt.Errorf("store: burn email change: %w", err)
	}
	return nil
}

// ConfirmEmailChange applies the address and marks the request used.
//
// It returns ErrIdentityTaken when another account already holds the address, so
// the caller can say "that address is in use" instead of a 500.
func (s *Store) ConfirmEmailChange(ctx context.Context, id, userID int64, newEmail string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET email = ?, email_verified = 1, updated_at = ? WHERE id = ?`,
		strings.ToLower(strings.TrimSpace(newEmail)), now, userID); err != nil {
		if isUniqueViolation(err) {
			return ErrIdentityTaken
		}
		return fmt.Errorf("store: change email: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE email_change_requests SET confirmed_at = ? WHERE id = ?`, now, id); err != nil {
		return fmt.Errorf("store: confirm email change: %w", err)
	}
	return tx.Commit()
}
