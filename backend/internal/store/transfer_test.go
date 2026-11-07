package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// transferEnv is a migrated store with two customers, one operator and one VPN
// that somebody already holds.
type transferEnv struct {
	st       *Store
	serverID int64
	offerID  int64
	owner    int64
	receiver int64
	admin    int64
	vpnID    int64
}

func newTransferEnv(t *testing.T) *transferEnv {
	t.Helper()
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "transfer.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	db := st.DB()
	mustID := func(q string, args ...any) int64 {
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	env := &transferEnv{st: st}
	env.serverID = mustID(`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv','https://p.example/base', x'00','ACTIVE',1,1)`)
	env.owner = mustID(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('janith14@example.com','x','Janith 14','client','ACTIVE',1,1,1)`)
	env.receiver = mustID(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('janith13@example.com','x','Janith 13','client','ACTIVE',1,1,1)`)
	env.admin = mustID(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('owner@example.com','x','Owner','super_admin','ACTIVE',1,1,1)`)
	env.offerID = mustID(`INSERT INTO vpn_offers (kind, name, server_id, traffic_gb, duration_days, device_limit, active, created_at, updated_at)
		VALUES ('PREMIUM','Reseller Premium',?,100,30,3,1,1,1)`, env.serverID)
	env.vpnID = mustID(`INSERT INTO vpn_instances
		(owner_user_id, server_id, offer_id, panel_email, panel_sub_id, status, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, 'dn-test', 'sub-abc', 'ACTIVE', 1999999999, 1, 1)`,
		env.owner, env.serverID, env.offerID)
	// The lock a real assignment always writes.
	mustID(`INSERT INTO panel_client_tags (server_id, panel_email, tag, tagged_by, tagged_at, locked_at)
		VALUES (?, 'dn-test', 'PREMIUM', ?, 1, 1)`, env.serverID, env.admin)
	mustID(`INSERT INTO vpn_assignments (vpn_instance_id, client_user_id, seller_user_id, status, started_at, created_at, updated_at)
		VALUES (?, ?, ?, 'ACTIVE', 1, 1, 1)`, env.vpnID, env.owner, env.admin)
	return env
}

// TestTransferMovesOwner is the owner's own example: one VPN held by janith-14
// is handed to janith-13.
func TestTransferMovesOwner(t *testing.T) {
	env := newTransferEnv(t)
	ctx := context.Background()

	res, err := env.st.TransferVPN(ctx, TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.receiver, ActorID: env.admin,
	})
	if err != nil {
		t.Fatalf("TransferVPN: %v", err)
	}
	if res.From.UserID != env.owner || res.To.UserID != env.receiver {
		t.Errorf("from/to = %d/%d, want %d/%d", res.From.UserID, res.To.UserID, env.owner, env.receiver)
	}

	var owner int64
	if err := env.st.DB().QueryRowContext(ctx,
		`SELECT owner_user_id FROM vpn_instances WHERE id = ?`, env.vpnID).Scan(&owner); err != nil {
		t.Fatalf("read owner: %v", err)
	}
	if owner != env.receiver {
		t.Errorf("owner = %d, want %d", owner, env.receiver)
	}
}

// TestTransferKeepsPanelIdentity proves the customer's connection keeps working:
// the row keeps the panel's own identity, so every link and QR already handed
// out still resolves.
func TestTransferKeepsPanelIdentity(t *testing.T) {
	env := newTransferEnv(t)
	ctx := context.Background()
	if _, err := env.st.TransferVPN(ctx, TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.receiver, ActorID: env.admin,
	}); err != nil {
		t.Fatalf("TransferVPN: %v", err)
	}

	var email, subID, status string
	var offerID, expiresAt int64
	if err := env.st.DB().QueryRowContext(ctx,
		`SELECT panel_email, panel_sub_id, status, offer_id, expires_at
		   FROM vpn_instances WHERE id = ?`, env.vpnID).
		Scan(&email, &subID, &status, &offerID, &expiresAt); err != nil {
		t.Fatalf("read vpn: %v", err)
	}
	if email != "dn-test" || subID != "sub-abc" {
		t.Errorf("identity = %q/%q, want dn-test/sub-abc — a transfer must not rename the panel client", email, subID)
	}
	if status != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE — an ownership change is not a state change", status)
	}
	if offerID != env.offerID || expiresAt != 1999999999 {
		t.Errorf("offer/expiry = %d/%d, want %d/1999999999", offerID, expiresAt, env.offerID)
	}
}

// TestTransferKeepsTagAndLock: a transfer is not a new sale, so it must not
// re-price the VPN the first customer paid for (D-025.2).
func TestTransferKeepsTagAndLock(t *testing.T) {
	env := newTransferEnv(t)
	ctx := context.Background()
	if _, err := env.st.TransferVPN(ctx, TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.receiver, ActorID: env.admin,
	}); err != nil {
		t.Fatalf("TransferVPN: %v", err)
	}

	var tag string
	var lockedAt *int64
	if err := env.st.DB().QueryRowContext(ctx,
		`SELECT tag, locked_at FROM panel_client_tags WHERE server_id = ? AND panel_email = 'dn-test'`,
		env.serverID).Scan(&tag, &lockedAt); err != nil {
		t.Fatalf("read tag: %v", err)
	}
	if tag != "PREMIUM" {
		t.Errorf("tag = %q, want PREMIUM", tag)
	}
	if lockedAt == nil || *lockedAt != 1 {
		t.Errorf("locked_at = %v, want the original 1 — a transfer must not re-stamp the lock", lockedAt)
	}
}

// TestTransferRefusesSelf: moving a VPN to the customer who already holds it is
// a no-op disguised as an action, so it is refused rather than silently applied.
func TestTransferRefusesSelf(t *testing.T) {
	env := newTransferEnv(t)
	_, err := env.st.TransferVPN(context.Background(), TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.owner, ActorID: env.admin,
	})
	if !errors.Is(err, ErrTransferSelf) {
		t.Fatalf("err = %v, want ErrTransferSelf", err)
	}
}

// TestTransferRefusesNonClient: a VPN belongs to a customer, never to a
// reseller — assigning one to an operator would file a customer's VPN in the
// operator's own book.
func TestTransferRefusesNonClient(t *testing.T) {
	env := newTransferEnv(t)
	_, err := env.st.TransferVPN(context.Background(), TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.admin, ActorID: env.admin,
	})
	if !errors.Is(err, ErrAssignTarget) {
		t.Fatalf("err = %v, want ErrAssignTarget", err)
	}
}

// TestTransferRefusesTerminal: a revoked VPN is gone. Handing it over would give
// the new owner a credential that does not work.
func TestTransferRefusesTerminal(t *testing.T) {
	env := newTransferEnv(t)
	ctx := context.Background()
	if _, err := env.st.DB().ExecContext(ctx,
		`UPDATE vpn_instances SET status = 'REVOKED' WHERE id = ?`, env.vpnID); err != nil {
		t.Fatalf("set revoked: %v", err)
	}
	_, err := env.st.TransferVPN(ctx, TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.receiver, ActorID: env.admin,
	})
	if !errors.Is(err, ErrTransferTerminal) {
		t.Fatalf("err = %v, want ErrTransferTerminal", err)
	}
}

// TestTransferRefusesReleased: a released VPN can still be restored by its
// current owner inside the 3-day window, which would leave two owners.
func TestTransferRefusesReleased(t *testing.T) {
	env := newTransferEnv(t)
	ctx := context.Background()
	if _, err := env.st.DB().ExecContext(ctx,
		`UPDATE vpn_instances SET status = 'RELEASED' WHERE id = ?`, env.vpnID); err != nil {
		t.Fatalf("set released: %v", err)
	}
	if _, err := env.st.DB().ExecContext(ctx,
		`INSERT INTO vpn_releases (vpn_instance_id, client_user_id, reason, released_at, restore_deadline, panel_client_kept)
		 VALUES (?, ?, 'client_choice', 1, 2, 1)`, env.vpnID, env.owner); err != nil {
		t.Fatalf("seed release: %v", err)
	}
	_, err := env.st.TransferVPN(ctx, TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.receiver, ActorID: env.admin,
	})
	if !errors.Is(err, ErrTransferReleased) {
		t.Fatalf("err = %v, want ErrTransferReleased", err)
	}
}

// TestTransferRefusesUnknownVPN: a typo in the id must be a 404, never a silent
// success.
func TestTransferRefusesUnknownVPN(t *testing.T) {
	env := newTransferEnv(t)
	_, err := env.st.TransferVPN(context.Background(), TransferVPNInput{
		VPNID: 999999, ToClientID: env.receiver, ActorID: env.admin,
	})
	if !errors.Is(err, ErrTransferTarget) {
		t.Fatalf("err = %v, want ErrTransferTarget", err)
	}
}

// TestTransferRefusesBlockedClient: a blocked account keeps its rows but must
// not receive new ones — a VPN handed to a blocked customer works for nobody.
//
// The blocked state is `SUSPENDED` in this schema (users.status allows ACTIVE,
// SUSPENDED, PENDING_VERIFICATION, DELETED); only ACTIVE may receive a VPN.
func TestTransferRefusesBlockedClient(t *testing.T) {
	env := newTransferEnv(t)
	ctx := context.Background()
	if _, err := env.st.DB().ExecContext(ctx,
		`UPDATE users SET status = 'SUSPENDED' WHERE id = ?`, env.receiver); err != nil {
		t.Fatalf("suspend receiver: %v", err)
	}
	_, err := env.st.TransferVPN(ctx, TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.receiver, ActorID: env.admin,
	})
	if !errors.Is(err, ErrAssignTarget) {
		t.Fatalf("err = %v, want ErrAssignTarget for a suspended account", err)
	}
}

// TestTransferLeavesNothingBehindOnRefusal: a refused transfer must not move an
// owner, an assignment or a history row. Half-applied is worse than refused.
func TestTransferLeavesNothingBehindOnRefusal(t *testing.T) {
	env := newTransferEnv(t)
	ctx := context.Background()
	if _, err := env.st.TransferVPN(ctx, TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.owner, ActorID: env.admin,
	}); !errors.Is(err, ErrTransferSelf) {
		t.Fatalf("err = %v, want ErrTransferSelf", err)
	}

	var owner int64
	_ = env.st.DB().QueryRowContext(ctx,
		`SELECT owner_user_id FROM vpn_instances WHERE id = ?`, env.vpnID).Scan(&owner)
	if owner != env.owner {
		t.Errorf("owner = %d, want %d unchanged", owner, env.owner)
	}
	var transfers int
	_ = env.st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_transfers WHERE vpn_instance_id = ?`, env.vpnID).Scan(&transfers)
	if transfers != 0 {
		t.Errorf("history = %d rows, want 0 after a refusal", transfers)
	}
	var active int
	_ = env.st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_assignments WHERE vpn_instance_id = ? AND status = 'ACTIVE'`,
		env.vpnID).Scan(&active)
	if active != 1 {
		t.Errorf("active assignments = %d, want exactly 1", active)
	}
}

// TestTransferClosesOldAssignment: without this both customers' premium counts
// would include the VPN (they count live assignment rows), so one VPN would look
// like two.
func TestTransferClosesOldAssignment(t *testing.T) {
	env := newTransferEnv(t)
	ctx := context.Background()
	if _, err := env.st.TransferVPN(ctx, TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.receiver, ActorID: env.admin,
	}); err != nil {
		t.Fatalf("TransferVPN: %v", err)
	}

	var oldStatus string
	if err := env.st.DB().QueryRowContext(ctx,
		`SELECT status FROM vpn_assignments WHERE vpn_instance_id = ? AND client_user_id = ?`,
		env.vpnID, env.owner).Scan(&oldStatus); err != nil {
		t.Fatalf("read old assignment: %v", err)
	}
	if oldStatus != "TRANSFERRED" {
		t.Errorf("old assignment status = %q, want TRANSFERRED", oldStatus)
	}
	var newStatus string
	var newSeller int64
	if err := env.st.DB().QueryRowContext(ctx,
		`SELECT status, seller_user_id FROM vpn_assignments
		  WHERE vpn_instance_id = ? AND client_user_id = ?`, env.vpnID, env.receiver).
		Scan(&newStatus, &newSeller); err != nil {
		t.Fatalf("read new assignment: %v", err)
	}
	if newStatus != "ACTIVE" {
		t.Errorf("new assignment status = %q, want ACTIVE", newStatus)
	}
	if newSeller != env.admin {
		t.Errorf("new seller = %d, want the issuing operator %d", newSeller, env.admin)
	}
}

// TestTransferMovesFreeClaim is the cap-honesty test (D-025.4/.5).
//
// Moving a FREE VPN without moving its claim would produce two wrong answers:
// the old owner would stay charged for a VPN they no longer hold (and could not
// claim a replacement), while the new owner could exceed the cap of 3 without
// ever seeing a refusal.
func TestTransferMovesFreeClaim(t *testing.T) {
	env := newTransferEnv(t)
	ctx := context.Background()
	if _, err := env.st.DB().ExecContext(ctx,
		`INSERT INTO free_claims (client_user_id, server_id, panel_email, vpn_instance_id, claimed_at)
		 VALUES (?, ?, 'dn-test', ?, 1)`, env.owner, env.serverID, env.vpnID); err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	res, err := env.st.TransferVPN(ctx, TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.receiver, ActorID: env.admin,
	})
	if err != nil {
		t.Fatalf("TransferVPN: %v", err)
	}
	if !res.FreeClaimMoved {
		t.Error("free_claim_moved = false, want true — the slot must follow its VPN")
	}

	var claimOwner, from int64
	if err := env.st.DB().QueryRowContext(ctx,
		`SELECT client_user_id, transferred_from_user_id FROM free_claims WHERE vpn_instance_id = ?`,
		env.vpnID).Scan(&claimOwner, &from); err != nil {
		t.Fatalf("read claim: %v", err)
	}
	if claimOwner != env.receiver {
		t.Errorf("claim owner = %d, want the new owner %d", claimOwner, env.receiver)
	}
	if from != env.owner {
		t.Errorf("transferred_from = %d, want the previous owner %d", from, env.owner)
	}

	// The caps must now read the move: the old owner holds nothing, the new one
	// holds one.
	oldUse, err := env.st.FreeUsage(ctx, env.owner, env.serverID, 0)
	if err != nil {
		t.Fatalf("FreeUsage(old): %v", err)
	}
	if oldUse.Concurrent != 0 {
		t.Errorf("old owner concurrent = %d, want 0", oldUse.Concurrent)
	}
	newUse, err := env.st.FreeUsage(ctx, env.receiver, env.serverID, 0)
	if err != nil {
		t.Fatalf("FreeUsage(new): %v", err)
	}
	if newUse.Concurrent != 1 {
		t.Errorf("new owner concurrent = %d, want 1", newUse.Concurrent)
	}
}

// TestTransferWithoutFreeClaimReportsNoMove: a premium VPN has no claim, so the
// flag must stay false rather than claiming a slot moved.
func TestTransferWithoutFreeClaimReportsNoMove(t *testing.T) {
	env := newTransferEnv(t)
	res, err := env.st.TransferVPN(context.Background(), TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.receiver, ActorID: env.admin,
	})
	if err != nil {
		t.Fatalf("TransferVPN: %v", err)
	}
	if res.FreeClaimMoved {
		t.Error("free_claim_moved = true, want false — a premium VPN has no claim")
	}
}

// TestTransferWritesHistory: "who moved what, when and why" must survive.
func TestTransferWritesHistory(t *testing.T) {
	env := newTransferEnv(t)
	ctx := context.Background()
	if _, err := env.st.TransferVPN(ctx, TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.receiver, ActorID: env.admin,
		Reason: "customer moved to a new account",
	}); err != nil {
		t.Fatalf("TransferVPN: %v", err)
	}

	rows, err := env.st.TransfersForVPN(ctx, env.vpnID)
	if err != nil {
		t.Fatalf("TransfersForVPN: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("history = %d rows, want 1", len(rows))
	}
	h := rows[0]
	if h.FromUserID != env.owner || h.ToUserID != env.receiver || h.ActorID != env.admin {
		t.Errorf("history parties = %d→%d by %d, want %d→%d by %d",
			h.FromUserID, h.ToUserID, h.ActorID, env.owner, env.receiver, env.admin)
	}
	if h.Reason != "customer moved to a new account" {
		t.Errorf("reason = %q, want the operator's note", h.Reason)
	}
	if h.PanelEmail != "dn-test" || h.Kind != "PREMIUM" {
		t.Errorf("history = %q/%q, want dn-test/PREMIUM", h.PanelEmail, h.Kind)
	}
	if h.FromEmail != "janith14@example.com" || h.ToEmail != "janith13@example.com" {
		t.Errorf("history emails = %q→%q", h.FromEmail, h.ToEmail)
	}
}
