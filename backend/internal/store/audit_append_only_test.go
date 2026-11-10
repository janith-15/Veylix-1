package store

import (
	"context"
	"strings"
	"testing"
)

// TestAuditLogIsAppendOnly is the B-5 regression test.
//
// The append-only rule was documented in the Bible and in DECISIONS but enforced
// nowhere. The proof was inverted: a test ran `UPDATE audit_logs` and the
// statement SUCCEEDED. A rule that only lives in prose survives exactly as long as
// nobody writes the wrong query — and the wrong query can come from a future
// migration, from an operator with `sqlite3` open, or from a bug in this codebase.
//
// `0027_audit_append_only.sql` installs BEFORE UPDATE and BEFORE DELETE triggers.
// Triggers, not privileges: SQLite has no table-level privilege model, and a
// trigger fires for EVERY writer, including ones this application does not own.
//
// What is asserted:
//   - an UPDATE is refused, and the row is unchanged afterwards
//   - a DELETE is refused, and the row still exists
//   - an INSERT still works, because append-only means append is allowed
func TestAuditLogIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir()+"/audit.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := st.DB().Exec(
		`INSERT INTO audit_logs (actor_id, action, target_type, target_id, result, context, created_at)
		 VALUES (NULL, 'vpn.provision', 'vpn', '1', 'OK', '{}', 1000)`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 1. UPDATE must be refused at the database, not merely avoided in code.
	_, err = st.DB().Exec(`UPDATE audit_logs SET action = 'tampered' WHERE target_id = '1'`)
	if err == nil {
		t.Fatal("UPDATE audit_logs SUCCEEDED — the append-only trigger is missing")
	}
	if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}

	// 2. DELETE must be refused too, so history cannot be quietly removed.
	_, err = st.DB().Exec(`DELETE FROM audit_logs WHERE target_id = '1'`)
	if err == nil {
		t.Fatal("DELETE FROM audit_logs SUCCEEDED — the append-only trigger is missing")
	}
	if !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}

	// 3. The row survives both attempts, unchanged.
	var action string
	if err := st.DB().QueryRow(`SELECT action FROM audit_logs WHERE target_id = '1'`).Scan(&action); err != nil {
		t.Fatalf("the refused DELETE removed the row: %v", err)
	}
	if action != "vpn.provision" {
		t.Fatalf("the refused UPDATE changed the row: action = %q", action)
	}

	// 4. Appending is still allowed — append-only must not mean read-only.
	if _, err := st.DB().Exec(
		`INSERT INTO audit_logs (actor_id, action, target_type, target_id, result, context, created_at)
		 VALUES (NULL, 'vpn.revoke', 'vpn', '1', 'OK', '{}', 2000)`,
	); err != nil {
		t.Fatalf("an append was refused: %v", err)
	}
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE target_id = '1'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("rows = %d, want 2 (the original plus the append)", n)
	}
}
