package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

// migrationsFS embeds the ordered SQL files shipped inside the binary so a
// release never needs a separate migration directory (`new_update.md` §4).
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration is one versioned schema step.
type migration struct {
	Name string
	SQL  string
}

// loadMigrations returns every embedded migration ordered by filename.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("store: read migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	// Filenames are zero-padded (0001_, 0002_, …) so lexical order == apply order.
	sort.Strings(names)

	out := make([]migration, 0, len(names))
	for _, name := range names {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, fmt.Errorf("store: read migration %s: %w", name, err)
		}
		out = append(out, migration{Name: name, SQL: string(body)})
	}
	return out, nil
}

// Migrate applies every pending migration inside its own transaction and
// records it in schema_migrations.
//
// It is idempotent: running it twice applies nothing the second time
// (tests: GATE 6 / T-19).
//
// Each migration gets its own transaction on purpose: a partially applied
// schema would be worse than a stopped one. The cost is one commit per
// migration, which is invisible in production (11 commits, once per install)
// but was 79 × 11 commits across the test suite — see MigrateFast.
func (s *Store) Migrate(ctx context.Context) error {
	if err := s.ensureMigrationsTable(ctx); err != nil {
		return err
	}
	applied, err := s.appliedMigrations(ctx)
	if err != nil {
		return err
	}
	pending, err := loadMigrations()
	if err != nil {
		return err
	}
	for _, m := range pending {
		if applied[m.Name] {
			continue
		}
		if err := s.applyMigration(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// MigrateFast applies every pending migration inside ONE transaction.
//
// It exists for tests, and only for tests — every store test starts from an
// empty database, and paying 11 commits per test made the suite noticeably
// slower as migrations were added.
//
// It is still a real migration: the SAME embedded SQL, the SAME schema_migrations
// rows, the same idempotence. What changes is the transaction boundary. A caller
// must not use it on a database it cares about, because one failing statement
// would roll back every migration in the batch instead of stopping at the
// failure — production keeps `Migrate` for exactly that reason.
func (s *Store) MigrateFast(ctx context.Context) error {
	if err := s.ensureMigrationsTable(ctx); err != nil {
		return err
	}
	applied, err := s.appliedMigrations(ctx)
	if err != nil {
		return err
	}
	pending, err := loadMigrations()
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		for _, m := range pending {
			if applied[m.Name] {
				continue
			}
			if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
				return fmt.Errorf("store: apply %s: %w", m.Name, err)
			}
			if err := recordMigration(ctx, tx, m.Name); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ensureMigrationsTable(ctx context.Context) error {
	const ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
		name       TEXT    PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`
	if _, err := s.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}
	return nil
}

func (s *Store) appliedMigrations(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("store: list migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("store: scan migration: %w", err)
		}
		applied[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate migrations: %w", err)
	}
	return applied, nil
}

func (s *Store) applyMigration(ctx context.Context, m migration) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
			return fmt.Errorf("store: apply %s: %w", m.Name, err)
		}
		return recordMigration(ctx, tx, m.Name)
	})
}

// recordMigration writes one schema_migrations row inside the caller's
// transaction, so a migration and its record land together or not at all.
func recordMigration(ctx context.Context, tx *sql.Tx, name string) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)`,
		name, time.Now().Unix(),
	); err != nil {
		return fmt.Errorf("store: record %s: %w", name, err)
	}
	return nil
}

// SchemaVersion returns the number of applied migrations.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: schema version: %w", err)
	}
	return n, nil
}
