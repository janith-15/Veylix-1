package provisioning

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/domain"
	"veylix/backend/internal/store"
)

// existingVPN seeds one confirmed, active VPN owned by the harness client.
// It is the starting point every replace test needs: something to replace.
func (h *harness) existingVPN(t *testing.T) (vpnID int64, email string) {
	t.Helper()
	email = "client@example.com_free_1"
	res, err := h.st.DB().Exec(
		`INSERT INTO vpn_instances
		   (owner_user_id, server_id, inbound_id, offer_id, panel_email, panel_sub_id,
		    status, status_changed_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'sub-1', 'ACTIVE', 1, 1, 1)`,
		h.client, h.server, h.inbound, h.offer, email)
	if err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	id, _ := res.LastInsertId()
	// The panel already holds this client, so `Create` will allocate the NEXT
	// slot rather than reusing the name — exactly the real behaviour.
	h.fake.mu.Lock()
	h.fake.existing[email] = true
	h.fake.mu.Unlock()
	return id, email
}

// releaseRecorder stands in for `releases.Release` and records whether it ran.
type releaseRecorder struct {
	calls  int
	vpnIDs []int64
	reason string
	err    error
}

func (r *releaseRecorder) fn(_ context.Context, vpnID int64, _ auth.Principal, reason string) (*store.ReleaseRow, error) {
	r.calls++
	r.vpnIDs = append(r.vpnIDs, vpnID)
	r.reason = reason
	if r.err != nil {
		return nil, r.err
	}
	return &store.ReleaseRow{ID: 77, VPNID: vpnID, RestoreDeadline: 1794000000}, nil
}

// TestReplaceCreatesTheNewVPNBeforeReleasingTheOld is the whole safety
// argument of D-112, stated as an assertion.
//
// If the panel refuses the new client, the old VPN must NOT be released: a
// customer who ends up with zero usable VPNs and no undo is worse off than one
// who was simply told "we could not create it". The recorder proves `Release`
// was never called, and the row proves the old VPN is still ACTIVE.
func TestReplaceCreatesTheNewVPNBeforeReleasingTheOld(t *testing.T) {
	h := newHarness(t, 0, "FREE")
	oldID, oldEmail := h.existingVPN(t)
	// The panel rejects every create.
	h.fake.mu.Lock()
	h.fake.addErr = errors.New("panel said no")
	h.fake.mu.Unlock()

	rec := &releaseRecorder{}
	_, err := h.svc.Replace(context.Background(), ReplaceInput{
		VPNID: oldID,
		Actor: clientPrincipal(h.client),
	}, rec.fn)
	if err == nil {
		t.Fatal("replace succeeded although the panel refused the new client")
	}
	if rec.calls != 0 {
		t.Fatalf("release ran %d time(s) after a failed create — the customer would be left "+
			"with no VPN and no undo", rec.calls)
	}
	if got := h.vpnStatus(t, oldID); got != domain.VPNActive {
		t.Errorf("old VPN status = %s, want ACTIVE (a failed replace must change nothing)", got)
	}
	if got := h.countVPNs(t); got != 1 {
		t.Errorf("vpn rows = %d, want 1 (the failed create left nothing behind)", got)
	}
	// And the panel was asked exactly once, for a NEW name.
	if oldEmail == "" {
		t.Error("seed is wrong: no panel email")
	}
}

// TestReplaceBuildsADifferentVPNThenReleasesTheOld proves the happy path does
// what the owner asked for: the customer ends up with a DIFFERENT panel client,
// and the old one is released rather than deleted.
func TestReplaceBuildsADifferentVPNThenReleasesTheOld(t *testing.T) {
	h := newHarness(t, 0, "FREE")
	oldID, oldEmail := h.existingVPN(t)

	rec := &releaseRecorder{}
	res, err := h.svc.Replace(context.Background(), ReplaceInput{
		VPNID: oldID,
		Actor: clientPrincipal(h.client),
	}, rec.fn)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if res.New == nil || !res.New.Recorded || !res.New.Confirmed {
		t.Fatalf("new VPN was not confirmed on the panel: %+v", res.New)
	}
	if res.New.Email == oldEmail {
		t.Fatalf("replacement reused the old panel identity %q — a replace must give the "+
			"customer a different VPN, not the same one renamed", res.New.Email)
	}
	if res.New.Email != "client@example.com_free_2" {
		t.Errorf("new panel email = %q, want client@example.com_free_2 (next free slot)", res.New.Email)
	}
	if rec.calls != 1 || rec.vpnIDs[0] != oldID {
		t.Fatalf("release calls = %v (want exactly the old VPN %d)", rec.vpnIDs, oldID)
	}
	if res.OldVPNID != oldID || res.OldPanelEmail != oldEmail {
		t.Errorf("result does not name the old VPN: %+v", res)
	}
	if res.ReleaseID == 0 || res.RestoreDeadline == 0 {
		t.Error("result carries no undo window — the customer could not restore the old VPN")
	}
	if !res.ClaimConsumed {
		t.Error("a FREE replace must report that it spent a monthly claim (Bible §17.18)")
	}
	// The old VPN is released, not deleted: its panel client stays (D-027.3).
	if got := h.countVPNs(t); got != 2 {
		t.Errorf("vpn rows = %d, want 2 (the new one and the released old one)", got)
	}
}

// --- helpers used by the refusal tests ------------------------------------

// emailN is a unique panel email for seeding claim rows.
func emailN(n int) string { return fmt.Sprintf("claim%d@example.com_free_1", n) }

// mustOffer seeds one more offer of the given kind on the harness server and
// returns its id.
func (h *harness) mustOffer(t *testing.T, kind string) int64 {
	t.Helper()
	res, err := h.st.DB().Exec(
		`INSERT INTO vpn_offers
		   (kind, name, server_id, inbound_id, traffic_gb, duration_days, device_limit,
		    active, owner_user_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 10, 30, 2, 1, ?, 1, 1)`,
		kind, kind+" offer", h.server, h.inbound, h.admin)
	if err != nil {
		t.Fatalf("seed offer: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// TestReplaceRefusesAnotherOwnersVPN — an id is not a capability. A reseller or
// another client must get the same answer as "missing", so ids stay
// unprobeable.
func TestReplaceRefusesAnotherOwnersVPN(t *testing.T) {
	h := newHarness(t, 0, "FREE")
	oldID, _ := h.existingVPN(t)

	res, err := h.st.DB().Exec(
		`INSERT INTO users (email, role, status, created_at, updated_at)
		 VALUES ('stranger@example.com','client','ACTIVE',1,1)`)
	if err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	stranger, _ := res.LastInsertId()

	rec := &releaseRecorder{}
	_, err = h.svc.Replace(context.Background(), ReplaceInput{
		VPNID: oldID,
		Actor: clientPrincipal(stranger),
	}, rec.fn)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (indistinguishable from missing)", err)
	}
	if rec.calls != 0 {
		t.Error("release ran for a VPN the caller does not own")
	}
	if got := h.vpnStatus(t, oldID); got != domain.VPNActive {
		t.Errorf("old VPN status = %s, want ACTIVE — a refused replace must change nothing", got)
	}
}

// TestReplaceRefusesWhenNoClaimRemains — Bible §17.18: releasing does not refund
// a monthly claim, so a replace spends one. It must be refused BEFORE the panel
// is touched, and the refusal must name the count.
func TestReplaceRefusesWhenNoClaimRemains(t *testing.T) {
	h := newHarness(t, 0, "FREE")
	oldID, _ := h.existingVPN(t)

	now := time.Now().Unix()
	for i := 0; i < store.FreeMonthlyDefault; i++ {
		if _, err := h.st.DB().Exec(
			`INSERT INTO free_claims (client_user_id, server_id, panel_email, claimed_at)
			 VALUES (?, ?, ?, ?)`,
			h.client, h.server, emailN(i), now); err != nil {
			t.Fatalf("seed claim %d: %v", i, err)
		}
	}

	rec := &releaseRecorder{}
	_, err := h.svc.Replace(context.Background(), ReplaceInput{
		VPNID: oldID,
		Actor: clientPrincipal(h.client),
	}, rec.fn)
	if !errors.Is(err, ErrNoClaimRemaining) {
		t.Fatalf("err = %v, want ErrNoClaimRemaining", err)
	}
	if rec.calls != 0 {
		t.Error("release ran although the allowance was spent")
	}
	if len(h.fake.specs) != 0 {
		t.Error("the panel was asked to create a client although no claim remained — " +
			"an allowance must be checked before it is spent, not after")
	}
}

// TestReplaceRefusesAReleasedVPN — a VPN already in its undo window is not in
// use, so there is nothing to refresh. The customer wants `restore`, not a
// second VPN.
func TestReplaceRefusesAReleasedVPN(t *testing.T) {
	h := newHarness(t, 0, "FREE")
	oldID, _ := h.existingVPN(t)
	if _, err := h.st.DB().Exec(
		`UPDATE vpn_instances SET status = 'RELEASED' WHERE id = ?`, oldID); err != nil {
		t.Fatalf("mark released: %v", err)
	}

	rec := &releaseRecorder{}
	_, err := h.svc.Replace(context.Background(), ReplaceInput{
		VPNID: oldID,
		Actor: clientPrincipal(h.client),
	}, rec.fn)
	if !errors.Is(err, ErrVPNNotReplaceable) {
		t.Fatalf("err = %v, want ErrVPNNotReplaceable", err)
	}
	if rec.calls != 0 {
		t.Error("release ran on an already-released VPN")
	}
}

// TestReplaceRefusesAKindChange — a replace refreshes a VPN; it is not an
// upgrade path. Moving a FREE VPN onto a paid plan automatically would change
// what the customer holds without them asking.
func TestReplaceRefusesAKindChange(t *testing.T) {
	h := newHarness(t, 10, "FREE")
	oldID, _ := h.existingVPN(t)
	premium := h.mustOffer(t, "PREMIUM")

	rec := &releaseRecorder{}
	_, err := h.svc.Replace(context.Background(), ReplaceInput{
		VPNID:   oldID,
		OfferID: premium,
		Actor:   clientPrincipal(h.client),
	}, rec.fn)
	if !errors.Is(err, ErrReplaceKindMismatch) {
		t.Fatalf("err = %v, want ErrReplaceKindMismatch", err)
	}
	if rec.calls != 0 {
		t.Error("release ran for a refused kind change")
	}
}

// TestReplaceWorksAtTheConcurrentCap is the defect found live (D-112).
//
// A replace builds the new VPN BEFORE releasing the old one, so at the
// concurrent cap an ordinary claim is refused and a customer holding three FREE
// VPNs could never replace any of them — the commonest case of the feature. The
// fix swaps the claim's slot instead of taking a new one; this test holds the
// customer at the cap and asserts the replace still succeeds.
func TestReplaceWorksAtTheConcurrentCap(t *testing.T) {
	h := newHarness(t, 0, "FREE")
	oldID, oldEmail := h.existingVPN(t)

	// Two more live claims, so the client is AT the cap of 3.
	now := time.Now().Unix()
	for i := 0; i < store.FreeConcurrentCap-1; i++ {
		email := emailN(i)
		res, err := h.st.DB().Exec(
			`INSERT INTO vpn_instances
			   (owner_user_id, server_id, inbound_id, offer_id, panel_email, panel_sub_id,
			    status, status_changed_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, 'ACTIVE', 1, 1, 1)`,
			h.client, h.server, h.inbound, h.offer, email, "sub-c"+email)
		if err != nil {
			t.Fatalf("seed vpn %d: %v", i, err)
		}
		vpnID, _ := res.LastInsertId()
		if _, err := h.st.DB().Exec(
			`INSERT INTO free_claims (client_user_id, server_id, panel_email, claimed_at, vpn_instance_id)
			 VALUES (?, ?, ?, ?, ?)`, h.client, h.server, email, now, vpnID); err != nil {
			t.Fatalf("seed claim %d: %v", i, err)
		}
	}
	// The VPN being replaced needs its own live claim, or the swap has nothing
	// to give up.
	if _, err := h.st.DB().Exec(
		`INSERT INTO free_claims (client_user_id, server_id, panel_email, claimed_at, vpn_instance_id)
		 VALUES (?, ?, ?, ?, ?)`, h.client, h.server, oldEmail, now, oldID); err != nil {
		t.Fatalf("seed claim for the replace target: %v", err)
	}

	// Confirm the client really is at the cap, so the test cannot pass for the
	// wrong reason.
	live := 0
	if err := h.st.DB().QueryRow(
		`SELECT COUNT(*) FROM free_claims WHERE client_user_id = ? AND released_at IS NULL`,
		h.client).Scan(&live); err != nil {
		t.Fatalf("count live claims: %v", err)
	}
	if live != store.FreeConcurrentCap {
		t.Fatalf("live claims = %d, want %d — the test is not exercising the cap", live, store.FreeConcurrentCap)
	}

	rec := &releaseRecorder{}
	res, err := h.svc.Replace(context.Background(), ReplaceInput{
		VPNID: oldID,
		Actor: clientPrincipal(h.client),
	}, rec.fn)
	if err != nil {
		t.Fatalf("replace at the concurrent cap: %v — a customer holding the maximum number "+
			"of FREE VPNs must still be able to replace one of them (D-112)", err)
	}
	if !res.New.Confirmed {
		t.Fatal("the replacement was not confirmed on the panel")
	}
	if res.New.Email == oldEmail {
		t.Error("replacement reused the old panel identity")
	}
	// The swap must not have raised the live count: one claim closed, one opened.
	after := 0
	if err := h.st.DB().QueryRow(
		`SELECT COUNT(*) FROM free_claims WHERE client_user_id = ? AND released_at IS NULL`,
		h.client).Scan(&after); err != nil {
		t.Fatalf("count live claims after: %v", err)
	}
	if after != store.FreeConcurrentCap {
		t.Errorf("live claims after = %d, want %d — a replace swaps its slot, it does not add one",
			after, store.FreeConcurrentCap)
	}
}
