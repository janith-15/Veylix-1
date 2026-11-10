package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ProviderGoogle is the only identity provider wired today (D-039). The table is
// keyed by provider so a second one does not need another column on `users`.
const ProviderGoogle = "google"

var (
	// ErrIdentityTaken means this provider account is already linked to someone else.
	ErrIdentityTaken = errors.New("store: identity already linked")
	// ErrNoSuchIdentity means nothing links that provider account (or token) to a user.
	ErrNoSuchIdentity = errors.New("store: identity not linked")
)

// OAuthState is one in-flight authorization request.
type OAuthState struct {
	State    string
	Verifier string
	Nonce    string
	Expires  time.Time
}

// CreateOAuthState stores the handshake. Expired rows are pruned on the way so
// abandoned logins cannot grow the table without bound.
func (s *Store) CreateOAuthState(ctx context.Context, st OAuthState) error {
	now := time.Now()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM oauth_states WHERE expires_at <= ?`, now.Unix()); err != nil {
		return fmt.Errorf("store: prune oauth states: %w", err)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO oauth_states (state, verifier, nonce, purpose, created_at, expires_at)
		 VALUES (?, ?, ?, 'google_login', ?, ?)`,
		st.State, st.Verifier, st.Nonce, now.Unix(), st.Expires.Unix())
	if err != nil {
		return fmt.Errorf("store: create oauth state: %w", err)
	}
	return nil
}

// ConsumeOAuthState returns AND deletes the state row in one statement.
//
// Single use is the point: a replayed callback (the redirect URL opened twice, or
// a captured one reused) finds nothing and is refused. DELETE ... RETURNING makes
// that atomic under SQLite's write lock.
func (s *Store) ConsumeOAuthState(ctx context.Context, state string) (OAuthState, error) {
	var (
		out OAuthState
		exp int64
	)
	err := s.db.QueryRowContext(ctx,
		`DELETE FROM oauth_states WHERE state = ? RETURNING verifier, nonce, expires_at`,
		state).Scan(&out.Verifier, &out.Nonce, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthState{}, ErrNoSuchIdentity
	}
	if err != nil {
		return OAuthState{}, fmt.Errorf("store: consume oauth state: %w", err)
	}
	out.State = state
	out.Expires = time.Unix(exp, 0)
	if time.Now().After(out.Expires) {
		return OAuthState{}, ErrNoSuchIdentity
	}
	return out, nil
}

// UserIDForIdentity resolves a provider account to a Veylix user.
func (s *Store) UserIDForIdentity(ctx context.Context, provider, subject string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM auth_identities WHERE provider = ? AND subject = ?`,
		provider, subject).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNoSuchIdentity
	}
	if err != nil {
		return 0, fmt.Errorf("store: lookup identity: %w", err)
	}
	return id, nil
}

// LinkIdentity records that a provider account belongs to userID, and keeps
// users.google_sub (from migration 0001) meaningful for Google.
func (s *Store) LinkIdentity(ctx context.Context, userID int64, provider, subject, email string) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO auth_identities (user_id, provider, subject, email, linked_at)
		 VALUES (?, ?, ?, ?, ?)`, userID, provider, subject, strings.ToLower(email), now)
	if err != nil {
		if isUniqueViolation(err) {
			// Already linked to this user is fine (idempotent); to anyone else is
			// a conflict the caller must report, never silently reassign.
			if existing, lookErr := s.UserIDForIdentity(ctx, provider, subject); lookErr == nil && existing == userID {
				return nil
			}
			return ErrIdentityTaken
		}
		return fmt.Errorf("store: link identity: %w", err)
	}
	if provider == ProviderGoogle {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE users SET google_sub = ?, email_verified = 1, updated_at = ? WHERE id = ?`,
			subject, now, userID); err != nil {
			return fmt.Errorf("store: set google_sub: %w", err)
		}
	}
	return nil
}

// UserByEmail returns the id and role for an address.
func (s *Store) UserByEmail(ctx context.Context, email string) (int64, string, error) {
	var (
		id   int64
		role string
	)
	err := s.db.QueryRowContext(ctx, `SELECT id, role FROM users WHERE email = ?`,
		strings.ToLower(strings.TrimSpace(email))).Scan(&id, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNoSuchIdentity
	}
	if err != nil {
		return 0, "", fmt.Errorf("store: lookup user: %w", err)
	}
	return id, role, nil
}

// EmailVerification is one pending verification token.
type EmailVerification struct {
	Token     string
	UserID    int64
	Email     string
	ExpiresAt time.Time
}

// CreateEmailVerification stores a token, invalidating any earlier pending one for
// the same user, so "the newest link works" is always true.
func (s *Store) CreateEmailVerification(ctx context.Context, v EmailVerification) error {
	now := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM email_verifications WHERE user_id = ? AND used_at IS NULL`, v.UserID); err != nil {
		return fmt.Errorf("store: clear verifications: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO email_verifications (token, user_id, email, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		v.Token, v.UserID, strings.ToLower(v.Email), now.Unix(), v.ExpiresAt.Unix()); err != nil {
		return fmt.Errorf("store: create verification: %w", err)
	}
	return tx.Commit()
}

// ConsumeEmailVerification marks a token used and returns it. A used or expired
// token is refused, so a verification link cannot be replayed.
func (s *Store) ConsumeEmailVerification(ctx context.Context, token string) (EmailVerification, error) {
	var (
		out EmailVerification
		exp int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, email, expires_at FROM email_verifications
		  WHERE token = ? AND used_at IS NULL`, token).Scan(&out.UserID, &out.Email, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return EmailVerification{}, ErrNoSuchIdentity
	}
	if err != nil {
		return EmailVerification{}, fmt.Errorf("store: lookup verification: %w", err)
	}
	out.Token = token
	out.ExpiresAt = time.Unix(exp, 0)
	if time.Now().After(out.ExpiresAt) {
		return EmailVerification{}, ErrNoSuchIdentity
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE email_verifications SET used_at = ? WHERE token = ?`, time.Now().Unix(), token); err != nil {
		return EmailVerification{}, fmt.Errorf("store: use verification: %w", err)
	}
	return out, nil
}

// CreateOAuthUser inserts a client account for a provider login: no password (the
// account can only be used through the provider), the provider already verified
// the address, and the status is ACTIVE.
func (s *Store) CreateOAuthUser(ctx context.Context, email, displayName, subject string) (int64, error) {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, google_sub, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES (?, NULL, ?, ?, 'client', 'ACTIVE', 1, ?, ?)`,
		strings.ToLower(strings.TrimSpace(email)), subject, displayName, now, now)
	if err != nil {
		if isUniqueViolation(err) {
			// Two callbacks for the same new address raced; whichever lost should
			// sign in as the row the winner created rather than fail the user.
			if id, _, lookErr := s.UserByEmail(ctx, email); lookErr == nil {
				return id, nil
			}
			return 0, ErrIdentityTaken
		}
		return 0, fmt.Errorf("store: create oauth user: %w", err)
	}
	return res.LastInsertId()
}

// UserRow is the safe projection of a user (never a password hash).
type UserRow struct {
	ID            int64  `json:"id"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name"`
	Role          string `json:"role"`
	EmailVerified bool   `json:"email_verified"`
	// CreatedAt is Unix seconds, the same unit every other timestamp on this
	// wire uses. It is here so GET /api/me can answer "when was this account
	// made" without a second query (D2).
	CreatedAt int64 `json:"created_at"`
}

// UserByID reads the safe projection of one account.
func (s *Store) UserByID(ctx context.Context, id int64) (*UserRow, error) {
	var (
		row UserRow
		ver int
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, display_name, role, email_verified, created_at
		   FROM users WHERE id = ?`, id).
		Scan(&row.ID, &row.Email, &row.DisplayName, &row.Role, &ver, &row.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSuchIdentity
	}
	if err != nil {
		return nil, fmt.Errorf("store: read user: %w", err)
	}
	row.EmailVerified = ver == 1
	return &row, nil
}

// isUniqueViolation reports whether err is a UNIQUE-constraint conflict, which is
// how "this row already exists" surfaces from modernc.org/sqlite (it has no typed
// error code we can rely on).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}

// MarkEmailVerified flips users.email_verified and unlocks a pending account.
func (s *Store) MarkEmailVerified(ctx context.Context, userID int64) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET email_verified = 1, updated_at = ?,
		        status = CASE WHEN status = 'PENDING_VERIFICATION' THEN 'ACTIVE' ELSE status END
		  WHERE id = ?`, time.Now().Unix(), userID); err != nil {
		return fmt.Errorf("store: mark verified: %w", err)
	}
	return nil
}
