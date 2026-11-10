package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// D-098 — the lifecycle migration must be idempotent and must never sweep a
// pre-existing row.
//
// The backfill is the dangerous part: it writes a timestamp onto every existing
// VPN. If it wrote a HISTORICAL value (created_at, updated_at) then the first
// sweep after an upgrade would find years of "stale" VPNs and set them all aside
// at once. These tests pin the safe behaviour.
func TestLifecycleMigrationBackfillsToMigrationTime(t *testing.T) {
	ctx := context.Background()

	st, err := Open(ctx, filepath.Join(t.TempDir(), "mig026.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = st.Close() }()

	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// The column exists after the migration, with a zero default meaning
	// "unknown". A row inserted with no explicit clock therefore gets 0, which
	// the sweep skips — the safe answer, because 0 can never be "ancient".
	var def sql.NullString
	if err := st.DB().QueryRow(
		`SELECT dflt_value FROM pragma_table_info('vpn_instances') WHERE name = 'status_changed_at'`,
	).Scan(&def); err != nil {
		t.Fatalf("read column default: %v", err)
	}
	if !def.Valid || def.String != "0" {
		t.Errorf("status_changed_at default = %v, want 0 (unknown, never assumed ancient)", def)
	}

	// The index the sweep relies on exists, so the hourly query is not a scan.
	var idx int
	if err := st.DB().QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_vpn_status_changed'`,
	).Scan(&idx); err != nil {
		t.Fatalf("read index: %v", err)
	}
	if idx != 1 {
		t.Errorf("idx_vpn_status_changed missing — the sweep would scan the table every hour")
	}
}

// Running the whole migration set twice must apply nothing the second time.
func TestLifecycleMigrationIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "idem.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = st.Close() }()

	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var n int
	if err := st.DB().QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE name LIKE '0026%'`).Scan(&n); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if n != 1 {
		t.Errorf("0026 recorded %d times, want 1", n)
	}
}
