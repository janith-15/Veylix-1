package store

import (
	"context"
	"fmt"
	"testing"
)

// The 0006 schema is the foundation of D-024 … D-028: tags, FREE claim caps,
// release with a grace period, chat budgets and the panel-mirror columns. If
// any of these are missing, the service layers above them cannot work.
func TestPanelTruthSchemaIsPresent(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for _, table := range []string{
		"panel_client_tags",
		"free_claims",
		"vpn_releases",
		"client_storage_limits",
	} {
		var n int
		if err := st.DB().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil {
			t.Fatalf("look up %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %s is missing", table)
		}
	}

	// The mirror columns must exist so sync can record the panel's own limits.
	cols, err := tableColumns(ctx, t, st, "vpn_instances")
	if err != nil {
		t.Fatalf("table_info(vpn_instances): %v", err)
	}
	for _, col := range []string{
		"panel_total_gb", "panel_expiry_time", "panel_limit_ip",
		"panel_limit_hwid", "panel_enable", "panel_synced_at",
	} {
		if !cols[col] {
			t.Errorf("vpn_instances.%s is missing", col)
		}
	}

	// A nil map/slice answers "missing" for everything, so if the PRAGMA ever
	// returned no rows at all the loop above would pass vacuously and report one
	// misleading error per column. Say what actually happened instead.
	if len(cols) == 0 {
		t.Fatal("table_info(vpn_instances) returned no columns; the table is missing")
	}
}

// A tag must be FREE or PREMIUM — nothing else may be stored (D-024.3).
func TestPanelClientTagValuesAreConstrained(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)

	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO panel_client_tags (server_id, panel_email, tag, tagged_at)
		 VALUES (?, 'a@panel.test', 'GOLD', 1)`, serverID); err == nil {
		t.Error("an unknown tag must be rejected by the CHECK constraint")
	}
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO panel_client_tags (server_id, panel_email, tag, tagged_at)
		 VALUES (?, 'a@panel.test', 'FREE', 1)`, serverID); err != nil {
		t.Errorf("a valid tag must be accepted: %v", err)
	}
}

// A panel identity can only carry one tag (PRIMARY KEY server_id + panel_email):
// two rows for the same client would make its Free/Premium status ambiguous.
func TestPanelClientTagIsUniquePerClient(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)

	ins := `INSERT INTO panel_client_tags (server_id, panel_email, tag, tagged_at) VALUES (?, ?, 'FREE', 1)`
	if _, err := st.DB().ExecContext(ctx, ins, serverID, "dup@panel.test"); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx, ins, serverID, "dup@panel.test"); err == nil {
		t.Error("a duplicate tag row must be rejected")
	}
}

// tableColumns returns the set of column names declared on table, read through
// PRAGMA table_info.
//
// The rows loop lives here so the *sql.Rows lifecycle has exactly one owner:
// rows is always closed (defer) — a leaked *sql.Rows pins a connection out of
// the pool — and rows.Err() is checked after the loop. Next() returning false
// means "no more rows" OR "the iteration stopped on an error", so without the
// check a mid-stream failure (busy timeout, context cancel, I/O error) would
// return a shorter column set that looks complete and make a schema assertion
// fail for the wrong reason. Callers only ever see the error, so the check can
// never be skipped.
func tableColumns(ctx context.Context, t *testing.T, st *Store, table string) (map[string]bool, error) {
	t.Helper()

	// table is a PRAGMA argument, not a bound parameter, so it cannot be
	// passed as ?. The name is interpolated, never user input.
	rows, err := st.DB().QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return nil, fmt.Errorf("query table_info(%s): %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	cols := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("scan column of %s: %w", table, err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate columns of %s: %w", table, err)
	}
	return cols, nil
}

// seedServer inserts the minimum integration row the FK-bearing tables need.
func seedServer(t *testing.T, st *Store) int64 {
	t.Helper()
	res, err := st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('srv', 'https://panel.example:2053/base', x'00', 'ACTIVE', 1, 1)`)
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}
