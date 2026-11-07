package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// SessionCookieName is the cookie that carries the opaque session id.
const SessionCookieName = "veylix_session"

// SessionLifetime bounds how long a session stays valid.
const SessionLifetime = 12 * time.Hour

// SessionStore persists sessions in SQLite.
//
// Only a SHA-256 fingerprint of the session id is stored, so a database read
// does not hand an attacker usable cookies (defence in depth).
type SessionStore struct {
	db     *sql.DB
	secure bool
}

// NewSessionStore binds a store to the database. secure marks cookies as
// Secure; it must be false only for plain-HTTP local development.
func NewSessionStore(db *sql.DB, secure bool) *SessionStore {
	return &SessionStore{db: db, secure: secure}
}

// Create issues a new session for userID and returns the raw cookie value.
// The raw value is returned once and never persisted.
func (s *SessionStore) Create(ctx context.Context, userID int64, ip, userAgent string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("auth: session id: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, created_at, expires_at, ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		string(HashToken(token)), userID, now.Unix(), now.Add(SessionLifetime).Unix(),
		truncate(ip, 64), truncate(userAgent, 255),
	)
	if err != nil {
		return "", fmt.Errorf("auth: create session: %w", err)
	}
	return token, nil
}

// Lookup resolves a raw cookie value to a principal, rejecting expired rows.
func (s *SessionStore) Lookup(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}
	var (
		p         Principal
		role      string
		expiresAt int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.email, u.role, s.expires_at
		   FROM sessions s
		   JOIN users u ON u.id = s.user_id
		  WHERE s.id = ? AND u.status = 'ACTIVE'`,
		string(HashToken(token)),
	).Scan(&p.UserID, &p.Email, &role, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("auth: lookup session: %w", err)
	}
	if time.Now().Unix() >= expiresAt {
		_ = s.Revoke(ctx, token)
		return Principal{}, ErrUnauthenticated
	}
	p.Role = Role(role)
	if !p.Role.Valid() {
		return Principal{}, ErrUnauthenticated
	}
	return p, nil
}

// Revoke deletes one session (logout).
func (s *SessionStore) Revoke(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, string(HashToken(token)))
	return err
}

// RevokeAllForUser is used after a password change or an admin suspension.
func (s *SessionStore) RevokeAllForUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// RevokeOthersForUser ends every session for a user EXCEPT the one identified by
// keepToken, and reports how many were ended (D3: "sign out other devices").
//
// The caller's own session is kept on purpose: the goal is to evict a device you
// no longer hold, not to sign yourself out. An empty keepToken therefore revokes
// nothing rather than everything — a request that arrived without a session has
// no business ending someone else's.
//
// The comparison is on the stored hash, never on the raw token, so this works
// with the same value `Revoke` deletes by.
func (s *SessionStore) RevokeOthersForUser(ctx context.Context, userID int64, keepToken string) (int64, error) {
	if keepToken == "" {
		return 0, nil
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = ? AND id <> ?`,
		userID, string(HashToken(keepToken)))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// PurgeExpired removes stale rows; safe to call from a periodic job.
func (s *SessionStore) PurgeExpired(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// sessionSameSite is Strict, not Lax.
//
// Lax still sends the cookie on a cross-site *top-level navigation*, which is
// exactly how a "form CSRF" or a session-fixation login lands: the attacker's
// page POSTs to /api/auth/login and the browser attaches the existing session.
// Strict withholds the cookie from every cross-site request, so that class of
// attack has no cookie to ride on.
//
// Nothing in the app needs a cross-site cookie: the SPA is same-origin, and the
// Google callback is a top-level navigation back to our own origin, which is
// same-site and therefore still carries the cookie.
const sessionSameSite = http.SameSiteStrictMode

// SetCookie writes the session cookie with hardened attributes.
func (s *SessionStore) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: sessionSameSite,
		MaxAge:   int(SessionLifetime.Seconds()),
	})
}

// ClearCookie expires the cookie client-side.
func (s *SessionStore) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: sessionSameSite,
		MaxAge:   -1,
	})
}

// TokenFromRequest extracts the raw session id from the cookie.
func TokenFromRequest(r *http.Request) string {
	c, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
