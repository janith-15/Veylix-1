package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return st
}

// T-19: migrations apply in order and are idempotent.
// T-19: the PRODUCTION migration path applies in order and is idempotent.
//
// This is the one test that deliberately uses `Migrate` rather than
// `MigrateFast`: production runs `Migrate`, so the per-migration transaction
// boundary must stay proven. The rest of the suite uses `MigrateFast` for speed.
func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	v1, err := st.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if v1 == 0 {
		t.Fatal("expected at least one applied migration")
	}

	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("second migrate must be a no-op, got: %v", err)
	}
	v2, err := st.SchemaVersion(ctx)
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if v1 != v2 {
		t.Fatalf("schema version changed on re-run: %d -> %d", v1, v2)
	}
}

// Pragmas are a correctness requirement, not a preference.
func TestPragmasAreApplied(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var journal string
	if err := st.DB().QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if journal != "wal" {
		t.Errorf("journal_mode = %q, want wal", journal)
	}

	var fk int
	if err := st.DB().QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	if err := st.IntegrityCheck(ctx); err != nil {
		t.Errorf("IntegrityCheck: %v", err)
	}
}

// rollback := a failed transaction leaves no partial rows (new_update.md Â§6).
func TestTxRollbackLeavesNoRows(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	wantErr := errRollback{}
	err := st.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO users (email, role, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			"rollback@example.com", "client", 1, 1,
		); err != nil {
			return err
		}
		return wantErr
	})
	if err == nil {
		t.Fatal("expected the transaction to fail")
	}

	var count int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("rollback left %d row(s) behind", count)
	}
}

// Uniqueness on the idempotency pair must be enforced by the database (D-012).
func TestDuplicatePanelEmailIsRejected(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	err := st.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO users (id, email, role, created_at, updated_at) VALUES (1, 'a@b.c', 'client', 1, 1)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO server_integrations (id, name, panel_url, token_ciphertext, created_at, updated_at)
			 VALUES (1, 'srv', 'https://panel.example', x'00', 1, 1)`); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	insert := func() error {
		return st.Tx(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO vpn_instances (owner_user_id, server_id, panel_email, panel_sub_id, created_at, updated_at)
				 VALUES (1, 1, 'vyx-1', 'vyx-1', 1, 1)`)
			return err
		})
	}
	if err := insert(); err != nil {
		t.Fatalf("first insert should succeed: %v", err)
	}
	if err := insert(); err == nil {
		t.Fatal("duplicate (server_id, panel_email) must be rejected")
	}
}

type errRollback struct{}

func (errRollback) Error() string { return "intentional failure" }
