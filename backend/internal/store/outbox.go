package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Outbox ops (the only remote writes Veylix performs).
const (
	OpCreateClient  = "CREATE_CLIENT"
	OpDeleteClient  = "DELETE_CLIENT"
	OpDisableClient = "DISABLE_CLIENT"
	OpEnableClient  = "ENABLE_CLIENT"
)

// OutboxState is the lifecycle of a queued remote operation.
type OutboxState string

const (
	OutboxPending  OutboxState = "PENDING"
	OutboxInFlight OutboxState = "IN_FLIGHT"
	OutboxApplied  OutboxState = "APPLIED"
	OutboxRetry    OutboxState = "RETRY"
	OutboxFailed   OutboxState = "FAILED"
)

// OutboxRow is one queued remote operation.
type OutboxRow struct {
	ID             int64
	Op             string
	ServerID       int64
	IdempotencyKey string
	PayloadJSON    string
	State          OutboxState
	Attempts       int
	LastError      string
	NextAttemptAt  *int64
	CreatedAt      int64
	UpdatedAt      int64
}

// EnqueueOutbox records a remote operation idempotently.
//
// UNIQUE(server_id, op, idempotency_key) means a retry of the same logical
// operation never creates a second row (F-009 / D-012).
func (s *Store) EnqueueOutbox(ctx context.Context, tx *sql.Tx, op string, serverID int64, key, payload string) (int64, error) {
	now := time.Now().Unix()
	exec := s.execer(tx)
	res, err := exec.ExecContext(ctx,
		`INSERT INTO outbox (op, server_id, idempotency_key, payload_json, state, attempts, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'PENDING', 0, ?, ?)
		 ON CONFLICT (server_id, op, idempotency_key) DO UPDATE SET updated_at = excluded.updated_at`,
		op, serverID, key, payload, now, now)
	if err != nil {
		return 0, fmt.Errorf("store: enqueue outbox: %w", err)
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		// ON CONFLICT took the update path; fetch the existing row id.
		if err := s.execer(tx).QueryRowContext(ctx,
			`SELECT id FROM outbox WHERE server_id = ? AND op = ? AND idempotency_key = ?`,
			serverID, op, key).Scan(&id); err != nil {
			return 0, fmt.Errorf("store: outbox id: %w", err)
		}
	}
	return id, nil
}

// MarkOutbox updates the outcome of a remote operation.
func (s *Store) MarkOutbox(ctx context.Context, tx *sql.Tx, id int64, state OutboxState, lastErr string, nextAt *int64) error {
	_, err := s.execer(tx).ExecContext(ctx,
		`UPDATE outbox SET state = ?, last_error = ?, attempts = attempts + 1,
		        next_attempt_at = ?, updated_at = ?
		  WHERE id = ?`,
		string(state), truncateErr(lastErr), nextAt, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("store: mark outbox: %w", err)
	}
	return nil
}

// PendingOutbox returns operations waiting to be retried (oldest first).
func (s *Store) PendingOutbox(ctx context.Context, limit int) ([]OutboxRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, op, server_id, idempotency_key, payload_json, state, attempts,
		        last_error, next_attempt_at, created_at, updated_at
		   FROM outbox
		  WHERE state IN ('PENDING','RETRY')
		    AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
		  ORDER BY id LIMIT ?`, time.Now().Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("store: pending outbox: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]OutboxRow, 0, limit)
	for rows.Next() {
		var r OutboxRow
		var next sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Op, &r.ServerID, &r.IdempotencyKey, &r.PayloadJSON,
			&r.State, &r.Attempts, &r.LastError, &next, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan outbox: %w", err)
		}
		if next.Valid {
			v := next.Int64
			r.NextAttemptAt = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetOutbox returns one row (used by the reconciler and tests).
func (s *Store) GetOutbox(ctx context.Context, id int64) (OutboxRow, error) {
	var r OutboxRow
	var next sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, op, server_id, idempotency_key, payload_json, state, attempts,
		        last_error, next_attempt_at, created_at, updated_at
		   FROM outbox WHERE id = ?`, id,
	).Scan(&r.ID, &r.Op, &r.ServerID, &r.IdempotencyKey, &r.PayloadJSON, &r.State,
		&r.Attempts, &r.LastError, &next, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return OutboxRow{}, ErrNoRows
	}
	if err != nil {
		return OutboxRow{}, fmt.Errorf("store: get outbox: %w", err)
	}
	if next.Valid {
		v := next.Int64
		r.NextAttemptAt = &v
	}
	return r, nil
}

// execer lets a helper run inside or outside a transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type dbExecer struct{ db *sql.DB }

func (d dbExecer) ExecContext(ctx context.Context, q string, a ...any) (sql.Result, error) {
	return d.db.ExecContext(ctx, q, a...)
}
func (d dbExecer) QueryRowContext(ctx context.Context, q string, a ...any) *sql.Row {
	return d.db.QueryRowContext(ctx, q, a...)
}

type txExecer struct{ tx *sql.Tx }

func (t txExecer) ExecContext(ctx context.Context, q string, a ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, q, a...)
}
func (t txExecer) QueryRowContext(ctx context.Context, q string, a ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, q, a...)
}

func (s *Store) execer(tx *sql.Tx) execer {
	if tx != nil {
		return txExecer{tx: tx}
	}
	return dbExecer{db: s.db}
}

// truncateErr bounds stored error text so a verbose panel response cannot
// bloat the outbox row (and secrets are never stored — callers pass safe text).
func truncateErr(s string) string {
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
