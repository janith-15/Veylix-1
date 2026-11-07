package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// AppSetting is one operator-editable setting row. ValueCiphertext is either a
// sealed blob (IsSecret) or the raw UTF-8 bytes; the store never interprets it —
// sealing and decoding are the settings service's job, so this layer stays a
// plain persistence table.
type AppSetting struct {
	Key             string
	ValueCiphertext []byte
	IsSecret        bool
	UpdatedBy       int64
}

// GetAppSetting reads one key. It reports (zero, false, nil) when the key is
// absent, so a caller can fall back to an environment value without treating a
// missing row as an error.
func (s *Store) GetAppSetting(ctx context.Context, key string) (AppSetting, bool, error) {
	const q = `SELECT key, value_ciphertext, is_secret, COALESCE(updated_by, 0)
	             FROM app_settings WHERE key = ?`
	var row AppSetting
	var secret int
	err := s.db.QueryRowContext(ctx, q, key).
		Scan(&row.Key, &row.ValueCiphertext, &secret, &row.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return AppSetting{}, false, nil
	}
	if err != nil {
		return AppSetting{}, false, err
	}
	row.IsSecret = secret != 0
	return row, true, nil
}

// SetAppSetting upserts one key, stamping updated_at and updated_by. updated_by
// is stored NULL when zero, because a bootstrap write has no acting account.
func (s *Store) SetAppSetting(ctx context.Context, key string, value []byte, isSecret bool, updatedBy int64) error {
	const q = `INSERT INTO app_settings (key, value_ciphertext, is_secret, updated_at, updated_by)
	           VALUES (?, ?, ?, ?, ?)
	           ON CONFLICT(key) DO UPDATE SET
	             value_ciphertext = excluded.value_ciphertext,
	             is_secret        = excluded.is_secret,
	             updated_at       = excluded.updated_at,
	             updated_by       = excluded.updated_by`
	flag := 0
	if isSecret {
		flag = 1
	}
	var uid any
	if updatedBy > 0 {
		uid = updatedBy
	}
	_, err := s.db.ExecContext(ctx, q, key, value, flag, time.Now().Unix(), uid)
	return err
}
