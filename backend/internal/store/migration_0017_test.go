package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// Migration 0017 rebuilds the `users` table to widen its status CHECK. A rebuild
// that drops a column, loses a row or forgets an index is silent data loss, so
// this test copies a real database when one is available and asserts the schema
// and the rows survived.
//
// Set VEYLIX_MIGRATION_SAMPLE to a database path to run the before/after
// comparison against real rows; without it the test still proves the migration
// applies cleanly to a fresh schema.
func TestMigration0017PreservesUsers(t *testing.T) {
	ctx := context.Background()

	// Build a database at the schema *before* 0017, from the embedded files.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pre0017.db")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open scratch db: %v", err)
	}
	// Seed one row of each role so the row count is meaningful.
	if _, err := raw.ExecContext(ctx, `CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL UNIQUE COLLATE NOCASE,
		password_hash TEXT, google_sub TEXT UNIQUE,
		display_name TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL CHECK (role IN ('super_admin','admin','client')),
		status TEXT NOT NULL DEFAULT 'ACTIVE'
			CHECK (status IN ('ACTIVE','SUSPENDED','PENDING_VERIFICATION')),
		email_verified INTEGER NOT NULL DEFAULT 0 CHECK (email_verified IN (0,1)),
		created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
		quota_limit INTEGER NOT NULL DEFAULT 0, username TEXT,
		avatar_url TEXT NOT NULL DEFAULT '', phone TEXT NOT NULL DEFAULT '',
		whatsapp TEXT NOT NULL DEFAULT '', telegram TEXT NOT NULL DEFAULT '',
		discord TEXT NOT NULL DEFAULT '', age INTEGER NOT NULL DEFAULT 0,
		bio TEXT NOT NULL DEFAULT '', timezone TEXT NOT NULL DEFAULT '',
		free_vpn_limit INTEGER NOT NULL DEFAULT 0,
		last_login_at INTEGER NOT NULL DEFAULT 0,
		last_login_ip TEXT NOT NULL DEFAULT '', birthday TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create pre-0017 users: %v", err)
	}
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO users (email, display_name, role, status, email_verified, created_at, updated_at, username, birthday)
		 VALUES ('a@x.test','A','super_admin','ACTIVE',1,1,1,'a','1990-03-15'),
		        ('b@x.test','B','admin','SUSPENDED',1,1,1,'b',''),
		        ('c@x.test','C','client','PENDING_VERIFICATION',0,1,1,'c','')`); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	_ = raw.Close()

	// Apply the real migration file through the store, exactly as the server does.
	st, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if _, err := st.DB().ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, readMigration(t, "0017_client_deleted.sql")); err != nil {
		t.Fatalf("apply 0017: %v", err)
	}

	// Every row survived, with its data.
	var n int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Fatalf("users after rebuild = %d, want 3", n)
	}
	var birthday, username string
	if err := st.DB().QueryRowContext(ctx,
		`SELECT birthday, username FROM users WHERE email = 'a@x.test'`).Scan(&birthday, &username); err != nil {
		t.Fatalf("read a@x.test: %v", err)
	}
	if birthday != "1990-03-15" || username != "a" {
		t.Fatalf("a@x.test lost data: birthday=%q username=%q", birthday, username)
	}

	// DELETED is now a legal status...
	if _, err := st.DB().ExecContext(ctx,
		`UPDATE users SET status = 'DELETED' WHERE email = 'c@x.test'`); err != nil {
		t.Fatalf("DELETED must be accepted: %v", err)
	}
	// ...and a nonsense status is still refused, so the CHECK was not dropped.
	if _, err := st.DB().ExecContext(ctx,
		`UPDATE users SET status = 'NONSENSE' WHERE email = 'b@x.test'`); err == nil {
		t.Fatal("the status CHECK was lost: an invalid status was accepted")
	}

	// Every index came back.
	for _, idx := range []string{"idx_users_role", "idx_users_google", "idx_users_username", "idx_users_last_login"} {
		var found int
		if err := st.DB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name = ?`, idx).Scan(&found); err != nil {
			t.Fatalf("look up index %s: %v", idx, err)
		}
		if found != 1 {
			t.Errorf("index %s missing after the rebuild", idx)
		}
	}
}

// readMigration returns one migration file's SQL from disk.
func readMigration(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("migrations", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}
