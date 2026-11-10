// Package store owns the SQLite database: opening, pragmas, migrations and
// transaction helpers.
//
// Design rules (new_update.md §4, §6, §8):
//   - persistence is isolated from business rules (domain layer never speaks SQL);
//   - WAL + foreign keys + busy timeout always on;
//   - connection count is bounded for the 1 vCPU / 500 MB target;
//   - schema changes go through embedded, ordered migrations.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // CGO-free driver (D-013)
)

// Store wraps the database handle.
type Store struct {
	db   *sql.DB
	path string
}

const (
	// maxOpenConns is deliberately small: SQLite writes are serialised and the
	// production server has 1 vCPU (new_update.md §8).
	maxOpenConns = 4
	// busyTimeoutMs lets concurrent writers wait instead of failing immediately.
	busyTimeoutMs = 5000
)

// Open opens (creating if needed) the SQLite database at path and applies pragmas.
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("store: empty database path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("store: create data dir: %w", err)
	}

	dsn := "file:" + url.PathEscape(filepath.ToSlash(path)) + "?" + pragmaQuery()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}

	return &Store{db: db, path: path}, nil
}

// pragmaQuery returns the DSN parameters applied to every connection.
func pragmaQuery() string {
	return "_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=busy_timeout(" + fmt.Sprint(busyTimeoutMs) + ")" +
		"&_pragma=synchronous(NORMAL)"
}

// DB exposes the handle for repositories inside this package.
func (s *Store) DB() *sql.DB { return s.db }

// Path returns the database file path.
func (s *Store) Path() string { return s.path }

// Close releases the handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Ping verifies the database is reachable (readiness probe).
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Tx runs fn inside a transaction, rolling back on error or panic.
//
// Business rules that must be atomic locally (quota consumption, assignment
// creation) go through this helper (new_update.md §6). It never implies
// atomicity with a remote 3X-UI panel.
func (s *Store) Tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit tx: %w", err)
	}
	return nil
}

// IntegrityCheck runs SQLite's quick_check; used by the readiness probe.
func (s *Store) IntegrityCheck(ctx context.Context) error {
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&result); err != nil {
		return fmt.Errorf("store: quick_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("store: integrity check failed: %s", result)
	}
	return nil
}
