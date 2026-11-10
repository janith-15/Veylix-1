package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrResetCode is returned when a reset code is unknown, expired, already used,
// or out of attempts. One error on purpose: a caller must not be able to tell
// those apart, because that difference is a hint an attacker would use.
var ErrResetCode = errors.New("store: password reset code is not valid")

// PasswordResetCode is one live reset request.
type PasswordResetCode struct {
	ID        int64
	UserID    int64
	CodeHash  string
	Attempts  int
	ExpiresAt time.Time
}

// CreatePasswordResetCode replaces any live code and stores the new hash.
//
// Replacing is deliberate: asking again must always mean "the newest code is the
// one that works", which also makes a replayed old code impossible.
func (s *Store) CreatePasswordResetCode(ctx context.Context, userID int64, codeHash string, ttl time.Duration) error {
	now := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM password_reset_codes WHERE user_id = ? AND used_at IS NULL`, userID); err != nil {
		return fmt.Errorf("store: clear reset code: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO password_reset_codes (user_id, code_hash, created_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		userID, codeHash, now.Unix(), now.Add(ttl).Unix()); err != nil {
		return fmt.Errorf("store: create reset code: %w", err)
	}
	return tx.Commit()
}

// LivePasswordResetCode returns the account's pending code, if any.
func (s *Store) LivePasswordResetCode(ctx context.Context, userID int64) (*PasswordResetCode, error) {
	var (
		c   PasswordResetCode
		exp int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, code_hash, attempts, expires_at
		   FROM password_reset_codes WHERE user_id = ? AND used_at IS NULL`, userID).
		Scan(&c.ID, &c.UserID, &c.CodeHash, &c.Attempts, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrResetCode
	}
	if err != nil {
		return nil, fmt.Errorf("store: read reset code: %w", err)
	}
	c.ExpiresAt = time.Unix(exp, 0)
	return &c, nil
}

// BumpPasswordResetAttempts counts a wrong guess and burns the code at the limit.
//
// A 6-digit code has only 10^6 values, so unlimited guesses would find it; the
// row is deleted at the limit rather than merely locked, so a later redeploy
// cannot resurrect it.
func (s *Store) BumpPasswordResetAttempts(ctx context.Context, id int64, max int) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE password_reset_codes SET attempts = attempts + 1 WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: bump reset attempts: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM password_reset_codes WHERE id = ? AND attempts >= ?`, id, max); err != nil {
		return fmt.Errorf("store: burn reset code: %w", err)
	}
	return nil
}

// ConsumePasswordResetCode marks the code used in the same transaction that sets
// the new password, so a crash between the two cannot leave a code live after
// the password it authorised already changed.
func (s *Store) ConsumePasswordResetCode(ctx context.Context, id, userID int64, passwordHash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	res, err := tx.ExecContext(ctx,
		`UPDATE password_reset_codes SET used_at = ? WHERE id = ? AND used_at IS NULL`, now, id)
	if err != nil {
		return fmt.Errorf("store: use reset code: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Someone else consumed it first: the code is single use, full stop.
		return ErrResetCode
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, now, userID); err != nil {
		return fmt.Errorf("store: set password: %w", err)
	}
	// Every session dies with the old password; a cookie issued before the reset
	// must not outlive it.
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("store: revoke sessions after reset: %w", err)
	}
	return tx.Commit()
}
