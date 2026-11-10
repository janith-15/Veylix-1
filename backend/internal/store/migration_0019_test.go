package store

import (
	"context"
	"testing"

	_ "modernc.org/sqlite"
)

// Migration 0019 repairs the rows a failed rename left split (D-063).
//
// The defect it closes produced TWO `vpn_instances` rows for ONE panel client:
// the assignment recorded the row under an invented name while the panel kept
// the original, so the console still offered "Assign" for a client somebody
// already held. A migration that silently merged two customers' VPNs would be
// worse than the bug, so this test pins exactly what it may and may not touch.
func TestMigration0019RepairsSplitAssignments(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db := st.DB()

	serverID := seedServer(t, st)
	owner := seedUser(t, ctx, st, "nimal@gmail.com", "client")
	other := seedUser(t, ctx, st, "kamal@gmail.com", "client")

	// A split assignment: the row says the NEW name, the ledger remembers that
	// the panel client was called "orphan-1".
	splitID := mustInsert(t, st,
		`INSERT INTO vpn_instances (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		 VALUES (?, ?, 'nimal@gmail.com_paid_1', 'vyx-1', 'ACTIVE', 1, 1)`, owner, serverID)
	mustInsert(t, st,
		`INSERT INTO panel_assignments (server_id, source_email, panel_email, vpn_instance_id, created_at)
		 VALUES (?, 'orphan-1', 'nimal@gmail.com_paid_1', ?, 1)`, serverID, splitID)

	// A DIFFERENT customer's VPN that is NOT split: its row and its reservation
	// agree. The repair must leave it completely alone, so the guard that only
	// rescues rows whose ledger disagrees is exercised.
	keepName := "kamal-original"
	takenID := mustInsert(t, st,
		`INSERT INTO vpn_instances (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'vyx-2', 'ACTIVE', 1, 1)`, other, serverID, keepName)
	mustInsert(t, st,
		`INSERT INTO panel_assignments (server_id, source_email, panel_email, vpn_instance_id, created_at)
		 VALUES (?, ?, ?, ?, 1)`, serverID, keepName, keepName, takenID)

	if _, err := db.ExecContext(ctx, readMigration(t, "0019_assign_keeps_panel_name.sql")); err != nil {
		t.Fatalf("apply 0019: %v", err)
	}

	// The split row is back under the name the panel actually holds.
	var name string
	if err := db.QueryRowContext(ctx,
		`SELECT panel_email FROM vpn_instances WHERE id = ?`, splitID).Scan(&name); err != nil {
		t.Fatalf("read repaired row: %v", err)
	}
	if name != "orphan-1" {
		t.Errorf("repaired name = %q, want the panel's name orphan-1", name)
	}

	// The other customer's row is untouched.
	if err := db.QueryRowContext(ctx,
		`SELECT panel_email FROM vpn_instances WHERE id = ?`, takenID).Scan(&name); err != nil {
		t.Fatalf("read untouched row: %v", err)
	}
	if name != keepName {
		t.Errorf("another customer's row moved to %q — two customers were merged", name)
	}

	// No assigned row is left without a package: that is what made an assigned
	// VPN project an empty kind and render as UNTAGGED.
	var unclassified int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_instances WHERE owner_user_id IS NOT NULL AND offer_id IS NULL`).
		Scan(&unclassified); err != nil {
		t.Fatalf("count: %v", err)
	}
	if unclassified != 0 {
		t.Errorf("%d owned rows still have no package after the backfill", unclassified)
	}

	// An owned row that had no label now carries a locked PREMIUM tag.
	var tagged, locked int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(locked_at IS NOT NULL), 0)
		   FROM panel_client_tags WHERE server_id = ? AND tag = 'PREMIUM'`, serverID).
		Scan(&tagged, &locked); err != nil {
		t.Fatalf("read tags: %v", err)
	}
	if tagged == 0 || locked != tagged {
		t.Errorf("premium tags = %d, locked = %d — every backfilled tag must be locked", tagged, locked)
	}

	// The ledger refuses a second reservation that claims the same target name,
	// so the database itself stops a duplicate even if the Go check is bypassed.
	var heldBy int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM panel_assignments WHERE server_id = ? AND panel_email = 'kamal-original'`,
		serverID).Scan(&heldBy); err != nil {
		t.Fatalf("count target: %v", err)
	}
	if heldBy != 1 {
		t.Fatalf("kamal-original reservations = %d, want 1 (the seed)", heldBy)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO panel_assignments (server_id, source_email, panel_email, vpn_instance_id, created_at)
		 VALUES (?, 'another-name', 'kamal-original', NULL, 1)`, serverID); err == nil {
		t.Error("a second reservation for the same target was accepted")
	}
	// ...and the same source name cannot be reserved twice either (0009's rule,
	// which is what makes "already assigned" answerable by the name a screen has).
	if _, err := db.ExecContext(ctx,
		`INSERT INTO panel_assignments (server_id, source_email, panel_email, vpn_instance_id, created_at)
		 VALUES (?, 'orphan-1', 'someone-else', NULL, 1)`, serverID); err == nil {
		t.Error("a second reservation for the same panel client was accepted")
	}
}

// mustInsert runs a seed statement and returns its row id.
func mustInsert(t *testing.T, db *Store, query string, args ...any) int64 {
	t.Helper()
	res, err := db.DB().Exec(query, args...)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}
