package provisioning

import (
	"context"
	"testing"

	"veylix/backend/internal/domain"
)

// D-034.1 — the panel identity names the customer and its kind, so an operator
// looking at 3X-UI can see whose VPN it is.
func TestPanelEmailIsClientAndKind(t *testing.T) {
	h := newHarness(t, 5, "FREE")
	ctx := context.Background()

	res, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, Actor: clientPrincipal(h.client),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Email != "client@example.com_free_1" {
		t.Errorf("panel email = %q, want client@example.com_free_1", res.Email)
	}

	// A PREMIUM sale of the same customer uses the `paid` counter, not `free`.
	premium := newHarness(t, 5, "PREMIUM")
	paid, err := premium.svc.Create(ctx, CreateInput{
		OfferID: premium.offer, ClientUserID: premium.client,
		SellerID: premium.admin, Actor: adminPrincipal(premium.admin),
	})
	if err != nil {
		t.Fatalf("Create premium: %v", err)
	}
	if paid.Email != "client@example.com_paid_1" {
		t.Errorf("panel email = %q, want client@example.com_paid_1", paid.Email)
	}
}

// D-034.2 — slots only move forward: a second VPN is `_paid_2`, and a revoked
// one does not free its number.
func TestPanelSlotsOnlyMoveForward(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	first, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first.Email != "client@example.com_paid_1" {
		t.Fatalf("first = %q, want _paid_1", first.Email)
	}

	second, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Email != "client@example.com_paid_2" {
		t.Errorf("second = %q, want _paid_2", second.Email)
	}

	// Revoke the second, then create again: the number must NOT be recycled.
	if _, err := h.svc.Revoke(ctx, second.VPNID, adminPrincipal(h.admin)); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	third, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("third: %v", err)
	}
	if third.Email != "client@example.com_paid_3" {
		t.Errorf("third = %q, want _paid_3 (a revoked slot is never re-issued)", third.Email)
	}
}

// D-034.3 — the idempotency pair survives the readable name: the subId is still
// derived from the local row id.
func TestPanelSubIdStaysDeterministic(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var subID string
	if err := h.st.DB().QueryRow(
		`SELECT panel_sub_id FROM vpn_instances WHERE id = ?`, res.VPNID).Scan(&subID); err != nil {
		t.Fatalf("read subId: %v", err)
	}
	if subID != domain.PanelSubID(res.VPNID) {
		t.Errorf("panel_sub_id = %q, want %q", subID, domain.PanelSubID(res.VPNID))
	}
}
