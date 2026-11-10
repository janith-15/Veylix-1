package store

import (
	"context"
	"errors"
	"testing"
)

// D-036.5 — the owner stops a VPN when the money does not arrive, and lifts the
// block once the issue is resolved. Both are events, so the reason survives.
func TestBlockAndUnblockVPN(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	clientID, orderID, _, _ := seedOrderEnv(t, ctx, st)

	// An order with no VPN yet has nothing to block: refusing is honest.
	if err := st.BlockOrderVPN(ctx, orderID, 1, "no money"); !errors.Is(err, ErrNoVPNToBlock) {
		t.Fatalf("error = %v, want ErrNoVPNToBlock", err)
	}

	// Give the order a VPN, as a confirmation would.
	serverID := seedServerRow(t, ctx, st)
	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO vpn_instances
		   (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		 VALUES (?, ?, 'nimal@gmail.com_paid_1', 'vyx-1', 'ACTIVE', 1, 1)`,
		clientID, serverID)
	if err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	vpnID, _ := res.LastInsertId()
	if _, err := st.DB().ExecContext(ctx,
		`UPDATE payment_records SET vpn_instance_id = ? WHERE id = ?`, vpnID, orderID); err != nil {
		t.Fatalf("link vpn: %v", err)
	}

	if err := st.BlockOrderVPN(ctx, orderID, 1, "slip does not match"); err != nil {
		t.Fatalf("block: %v", err)
	}
	if got := vpnStatus(t, ctx, st, vpnID); got != "SUSPENDED" {
		t.Errorf("status = %q, want SUSPENDED after a block", got)
	}

	if err := st.UnblockOrderVPN(ctx, orderID, 1, "money received"); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	if got := vpnStatus(t, ctx, st, vpnID); got != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE after an unblock", got)
	}

	events, err := st.OrderEvents(ctx, orderID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want BLOCKED then UNBLOCKED", len(events))
	}
	if events[0].Kind != EventBlocked || events[0].Note != "slip does not match" {
		t.Errorf("first event = %+v, want BLOCKED with the reason", events[0])
	}
	if events[1].Kind != EventUnblocked || events[1].Note != "money received" {
		t.Errorf("second event = %+v, want UNBLOCKED with the reason", events[1])
	}
}

// seedServerRow inserts a second panel, for a test that already created one.
func seedServerRow(t *testing.T, ctx context.Context, st *Store) int64 {
	t.Helper()
	var id int64
	if err := st.DB().QueryRowContext(ctx,
		`SELECT id FROM server_integrations ORDER BY id LIMIT 1`).Scan(&id); err == nil {
		return id
	}
	return seedServer(t, st)
}

// vpnStatus reads one VPN's lifecycle state.
func vpnStatus(t *testing.T, ctx context.Context, st *Store, vpnID int64) string {
	t.Helper()
	var s string
	if err := st.DB().QueryRowContext(ctx,
		`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&s); err != nil {
		t.Fatalf("read vpn status: %v", err)
	}
	return s
}
