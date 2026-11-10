package store

import (
	"context"
	"path/filepath"
	"testing"
)

// TestPaymentListNamesWhoPlacedIt is the D-113 defect, stated as an assertion.
//
// The order row used to carry only `payer_user_id` and `offer_id`, so a Super
// Admin looking at their queue could not tell whose money it was or which
// package was bought without a second lookup. The API must now carry the
// payer's email and the plan's name, because that is the whole reason an
// operator opens the list.
func TestPaymentListNamesWhoPlacedIt(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "pay.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	serverID := seedServer(t, st)
	payerID := seedClient(t, st, "payer@example.com")
	targetID := seedClient(t, st, "friend@example.com")

	var offerID int64
	res, err := st.DB().ExecContext(ctx,
		`INSERT INTO vpn_offers
		   (kind, name, server_id, traffic_gb, duration_days, device_limit, active, created_at, updated_at)
		 VALUES ('PREMIUM', 'Premium 50GB', ?, 50, 30, 3, 1, 1, 1)`, serverID)
	if err != nil {
		t.Fatalf("seed offer: %v", err)
	}
	offerID, _ = res.LastInsertId()

	// A reseller bought a plan FOR somebody else: payer and target differ, which
	// is exactly the case where naming only the payer would mislead.
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO payment_records
		   (payer_user_id, offer_id, target_user_id, amount_cents, currency, status, created_at, updated_at)
		 VALUES (?, ?, ?, 150000, 'LKR', 'PENDING', 1, 1)`,
		payerID, offerID, targetID); err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	rows, err := st.PaymentsVisible(ctx, 1, "super_admin", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	got := rows[0]
	if got.PayerEmail != "payer@example.com" {
		t.Errorf("PayerEmail = %q, want payer@example.com — the operator must be able to see WHO placed the order",
			got.PayerEmail)
	}
	if got.TargetEmail != "friend@example.com" {
		t.Errorf("TargetEmail = %q, want friend@example.com — an order bought for someone else must say so",
			got.TargetEmail)
	}
	if got.OfferName != "Premium 50GB" {
		t.Errorf("OfferName = %q, want Premium 50GB — the operator must be able to see WHICH package",
			got.OfferName)
	}
	if got.OfferKind != "PREMIUM" {
		t.Errorf("OfferKind = %q, want PREMIUM", got.OfferKind)
	}
}

// TestPaymentListStatesAnAbsentPackageAsEmpty — a manual record has no plan.
//
// The absent value must come back as "" (so the UI can say "no package"),
// never as a zero id that could be mistaken for a real plan, and never as a
// scan failure that would blank the whole list.
func TestPaymentListStatesAnAbsentPackageAsEmpty(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "pay2.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	payerID := seedClient(t, st, "manual@example.com")

	// No offer_id, no target_user_id — the shape of a hand-written record.
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO payment_records (payer_user_id, amount_cents, currency, status, created_at, updated_at)
		 VALUES (?, 2500, 'USD', 'PENDING', 1, 1)`, payerID); err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	rows, err := st.PaymentsVisible(ctx, 1, "super_admin", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].PayerEmail != "manual@example.com" {
		t.Errorf("PayerEmail = %q, want manual@example.com — the payer is known even without a plan",
			rows[0].PayerEmail)
	}
	if rows[0].OfferName != "" || rows[0].OfferKind != "" {
		t.Errorf("OfferName/Kind = %q/%q, want empty for a manual record",
			rows[0].OfferName, rows[0].OfferKind)
	}
}
