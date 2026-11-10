package store

import (
	"context"
	"database/sql"
	"testing"

	"veylix/backend/internal/domain"
)

// Releasing a VPN used to leave it ACTIVE, so the next sync pass saw a client
// missing from the panel and marked it SERVER_UNAVAILABLE. For the whole 3-day
// undo window the customer was told the SERVER had failed — when the truth was
// that they had released the VPN themselves and could still restore it.
//
// Two facts are asserted here, because either one alone would let the bug back
// in: the release writes RELEASED, and the state machine refuses to move a
// released VPN to SERVER_UNAVAILABLE (so a future sync path cannot reintroduce
// the mislabel through a different caller).
func TestReleaseMarksReleasedAndNeverServerUnavailable(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir()+"/released.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	serverID := seedServer(t, st)
	clientID := seedUser(t, ctx, st, "releaser@example.com", "client")

	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO vpn_instances
		   (owner_user_id, server_id, inbound_id, offer_id, panel_email, panel_sub_id, status,
		    traffic_limit_bytes, device_limit, created_at, updated_at)
		 VALUES (?, ?, NULL, NULL, 'releaser@example.com_paid_1', '', 'TRAFFIC_DEPLETED', 0, 2, 1, 1)`,
		clientID, serverID)
	if err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	vpnID, _ := res.LastInsertId()

	const now = int64(1_800_000_000)
	rel, err := st.CreateReleaseAt(ctx, vpnID, clientID, ReleaseReasonQuotaExhausted, now)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}

	statusOf := func() string {
		var s string
		if err := st.DB().QueryRowContext(ctx,
			`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&s); err != nil {
			t.Fatalf("read status: %v", err)
		}
		return s
	}

	if got := statusOf(); got != "RELEASED" {
		t.Fatalf("status after release = %q, want RELEASED", got)
	}

	// The reason survives: a depleted VPN released for quota is not the same
	// story as one the customer simply stopped wanting.
	var before string
	if err := st.DB().QueryRowContext(ctx,
		`SELECT status_before FROM vpn_releases WHERE id = ?`, rel.ID).Scan(&before); err != nil {
		t.Fatalf("read status_before: %v", err)
	}
	if before != "TRAFFIC_DEPLETED" {
		t.Errorf("status_before = %q, want TRAFFIC_DEPLETED", before)
	}

	// The invariant sync depends on.
	if domain.CanTransition(domain.VPNReleased, domain.VPNServerUnavailable) {
		t.Error("RELEASED -> SERVER_UNAVAILABLE is legal; sync would relabel a deliberate release as a server fault")
	}
	if !domain.CanTransition(domain.VPNReleased, domain.VPNActive) {
		t.Error("RELEASED -> ACTIVE is illegal; a restore could never put the VPN back")
	}

	// Restoring walks it back out of RELEASED.
	if _, err := st.RestoreRelease(ctx, rel.ID, clientID, now+60); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := statusOf(); got != "ACTIVE" {
		t.Errorf("status after restore = %q, want ACTIVE", got)
	}
}

// Purge is the ONE place a VPN row is destroyed (D-027.5). After it runs, the row
// — and with it the panel identity, the owner and the package — no longer exists,
// so anything still describing the VPN has to be captured DURING the delete.
//
// Before this test, RemoveReleasedVPN returned nothing: the audit trail recorded
// a bare numeric id, and "which VPN was removed, whose was it, what had they
// bought" became unanswerable the moment the grace period ended.
func TestPurgeReturnsSnapshotBeforeDeleting(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir()+"/purge.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	serverID := seedServer(t, st)
	clientID := seedUser(t, ctx, st, "buyer@example.com", "client")
	sellerID := seedUser(t, ctx, st, "seller@example.com", "admin")

	// A plan, so the snapshot can prove it names the package rather than an id.
	ores, err := st.DB().ExecContext(ctx,
		`INSERT INTO vpn_offers (kind, name, server_id, traffic_gb, duration_days, device_limit, active, created_at, updated_at)
		 VALUES ('PREMIUM', 'Starter 50GB', ?, 50, 30, 2, 1, 1, 1)`, serverID)
	if err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	offerID, _ := ores.LastInsertId()

	var vpnID int64
	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO vpn_instances
		   (owner_user_id, server_id, inbound_id, offer_id, panel_email, panel_sub_id, status,
		    traffic_limit_bytes, device_limit, created_at, updated_at)
		 VALUES (?, ?, NULL, ?, ?, '', 'TRAFFIC_DEPLETED', 0, 2, 1, 1)`,
		clientID, serverID, offerID, "buyer@example.com_paid_1")
	if err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	vpnID, _ = res.LastInsertId()

	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO vpn_assignments (vpn_instance_id, client_user_id, seller_user_id, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'ACTIVE', 1, 1)`, vpnID, clientID, sellerID); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	const now = int64(1_800_000_000)
	rel, err := st.CreateReleaseAt(ctx, vpnID, clientID, ReleaseReasonQuotaExhausted, now)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}

	snap, err := st.RemoveReleasedVPN(ctx, rel.ID, now+int64(ReleaseGraceDays)*86400)
	if err != nil {
		t.Fatalf("RemoveReleasedVPN: %v", err)
	}
	if snap == nil {
		t.Fatal("snapshot is nil — the audit trail would name only a number")
	}
	if snap.VPNID != vpnID {
		t.Errorf("snapshot VPNID = %d, want %d", snap.VPNID, vpnID)
	}
	if snap.PanelEmail != "buyer@example.com_paid_1" {
		t.Errorf("panel_email = %q, want the panel identity", snap.PanelEmail)
	}
	if snap.OwnerEmail != "buyer@example.com" || snap.OwnerID != clientID {
		t.Errorf("owner = (%q, %d), want (buyer@example.com, %d)", snap.OwnerEmail, snap.OwnerID, clientID)
	}
	if snap.SellerID != sellerID {
		t.Errorf("seller_id = %d, want %d", snap.SellerID, sellerID)
	}
	if snap.OfferName != "Starter 50GB" || snap.OfferID != offerID {
		t.Errorf("offer = (%q, %d), want (Starter 50GB, %d)", snap.OfferName, snap.OfferID, offerID)
	}
	// Status at purge is RELEASED (the undo window's own state), and the state it
	// had before the release is preserved alongside it — "released" alone would
	// not say whether the VPN was depleted, expired or simply unwanted.
	if snap.Status != "RELEASED" {
		t.Errorf("status at purge = %q, want RELEASED", snap.Status)
	}
	if snap.StatusBeforeRelease != "TRAFFIC_DEPLETED" {
		t.Errorf("status before release = %q, want TRAFFIC_DEPLETED", snap.StatusBeforeRelease)
	}

	// The row really is gone — the snapshot is the only description left.
	var n int
	if err := st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_instances WHERE id = ?`, vpnID).Scan(&n); err != nil {
		t.Fatalf("count vpn: %v", err)
	}
	if n != 0 {
		t.Fatalf("vpn row still present (%d) after purge", n)
	}

	// Idempotent: a retried tick must not remove twice. Since migration 0023
	// (I-33, D-087) the release row SURVIVES the purge as a tombstone, so the
	// second call finds it, sees `removed_at` already set, and returns the
	// documented no-op (nil snapshot, nil error) rather than ErrReleaseNotFound.
	// The old expectation pinned the cascade: the row only looked "not found"
	// because the purge had destroyed it.
	again, err := st.RemoveReleasedVPN(ctx, rel.ID, now+int64(ReleaseGraceDays)*86400)
	if err != nil {
		t.Fatalf("second purge err = %v, want nil (already removed is a no-op)", err)
	}
	if again != nil {
		t.Errorf("second purge returned a snapshot (%+v); an already-removed release must be a no-op", again)
	}
	// The tombstone is still there and still stamped: a no-op must not restamp
	// removed_at with the second call's clock.
	var removedAt sql.NullInt64
	if err := st.DB().QueryRowContext(ctx,
		`SELECT removed_at FROM vpn_releases WHERE id = ?`, rel.ID).Scan(&removedAt); err != nil {
		t.Fatalf("read tombstone: %v", err)
	}
	if !removedAt.Valid || removedAt.Int64 != now+int64(ReleaseGraceDays)*86400 {
		t.Fatalf("removed_at = %v, want the FIRST purge's stamp", removedAt)
	}

	// A row with no plan and no assignment must still snapshot (COALESCE), so a
	// legacy VPN cannot stall the purge loop.
	var legacyID int64
	lres, err := st.DB().ExecContext(ctx,
		`INSERT INTO vpn_instances
		   (owner_user_id, server_id, inbound_id, offer_id, panel_email, panel_sub_id, status,
		    traffic_limit_bytes, device_limit, created_at, updated_at)
		 VALUES (?, ?, NULL, NULL, 'legacy@example.com_free_1', '', 'EXPIRED', 0, 0, 1, 1)`,
		clientID, serverID)
	if err != nil {
		t.Fatalf("seed legacy vpn: %v", err)
	}
	legacyID, _ = lres.LastInsertId()
	lrel, err := st.CreateReleaseAt(ctx, legacyID, clientID, ReleaseReasonClientChoice, now)
	if err != nil {
		t.Fatalf("create legacy release: %v", err)
	}
	lsnap, err := st.RemoveReleasedVPN(ctx, lrel.ID, now+int64(ReleaseGraceDays)*86400)
	if err != nil {
		t.Fatalf("purge legacy: %v", err)
	}
	if lsnap == nil || lsnap.PanelEmail != "legacy@example.com_free_1" {
		t.Fatalf("legacy snapshot = %+v, want the panel email captured", lsnap)
	}
	if lsnap.OfferID != 0 || lsnap.SellerID != 0 {
		t.Errorf("legacy snapshot inventing links: offer=%d seller=%d", lsnap.OfferID, lsnap.SellerID)
	}
}
