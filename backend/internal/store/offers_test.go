package store

import (
	"context"
	"testing"
	"time"
)

// newOfferStore opens a migrated database and one server to point offers at.
func newOfferStore(t *testing.T) (*Store, int64) {
	t.Helper()
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir()+"/offers.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	res, err := st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, created_at, updated_at)
		 VALUES ('s1', 'https://panel.test', x'00', ?, ?)`,
		time.Now().Unix(), time.Now().Unix())
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
	id, _ := res.LastInsertId()
	return st, id
}

// A price must survive the round-trip. Before D-047 the catalogue carried no
// price at all, so the buy screen had to ask the BUYER what they were paying.
func TestOfferPriceRoundTrips(t *testing.T) {
	st, serverID := newOfferStore(t)
	ctx := context.Background()

	created, err := st.CreateOffer(ctx, CreateOfferInput{
		Kind: "PREMIUM", Name: "Premium 50GB", ServerID: serverID,
		TrafficGB: 50, DurationDays: 30, DeviceLimit: 2,
		PriceCents: 150000, Currency: "LKR",
		Active: true,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}
	if created.PriceCents != 150000 {
		t.Fatalf("PriceCents = %d, want 150000", created.PriceCents)
	}
	if created.Currency != "LKR" {
		t.Fatalf("Currency = %q, want LKR", created.Currency)
	}

	// The read path used by the catalogue must agree with the write.
	rows, err := st.OffersVisible(ctx, "super_admin", false)
	if err != nil {
		t.Fatalf("OffersVisible: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].PriceCents != 150000 || rows[0].Currency != "LKR" {
		t.Fatalf("listed price = %d %s, want 150000 LKR", rows[0].PriceCents, rows[0].Currency)
	}
}

// A PATCH that only re-sends an unchanged price must be idempotent, and a real
// change must stick. Offers are priced per catalogue row, so an edit here is the
// only way a seller can correct a mistake.
func TestUpdateOfferChangesPrice(t *testing.T) {
	st, serverID := newOfferStore(t)
	ctx := context.Background()

	created, err := st.CreateOffer(ctx, CreateOfferInput{
		Kind: "PREMIUM", Name: "P", ServerID: serverID, PriceCents: 100000, Currency: "LKR", Active: true,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	newPrice := int64(125000)
	updated, err := st.UpdateOffer(ctx, created.ID, UpdateOfferInput{PriceCents: &newPrice})
	if err != nil {
		t.Fatalf("UpdateOffer: %v", err)
	}
	if updated.PriceCents != 125000 {
		t.Fatalf("PriceCents = %d, want 125000", updated.PriceCents)
	}

	// Re-sending the same value is a no-op, not an error.
	again, err := st.UpdateOffer(ctx, created.ID, UpdateOfferInput{PriceCents: &newPrice})
	if err != nil {
		t.Fatalf("idempotent UpdateOffer: %v", err)
	}
	if again.PriceCents != 125000 {
		t.Fatalf("PriceCents = %d after re-send, want 125000", again.PriceCents)
	}

	// Currency is independently patchable.
	cur := "USD"
	got, err := st.UpdateOffer(ctx, created.ID, UpdateOfferInput{Currency: &cur})
	if err != nil {
		t.Fatalf("UpdateOffer currency: %v", err)
	}
	if got.Currency != "USD" || got.PriceCents != 125000 {
		t.Fatalf("currency update changed more than it should: %+v", got)
	}
}

// TestArchiveOfferRemovesItWithoutDestroyingIt is the P-4 regression test, part one.
//
// Before this, the ONLY way to take an offer out of the shop was `PATCH
// {active:false}`. That hid it but recorded nothing about WHY, and the row stayed
// in every listing labelled "inactive" forever — indistinguishable from an offer
// somebody deliberately paused.
//
// Archiving records the moment, removes it from the catalogue, and keeps the row
// for anything that references it.
func TestArchiveOfferRemovesItWithoutDestroyingIt(t *testing.T) {
	st, serverID := newOfferStore(t)
	ctx := context.Background()

	created, err := st.CreateOffer(ctx, CreateOfferInput{
		Kind: "PREMIUM", Name: "Retire me", ServerID: serverID,
		TrafficGB: 10, DurationDays: 30, DeviceLimit: 1,
		PriceCents: 50000, Currency: "LKR", Active: true,
	})
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	archived, err := st.ArchiveOffer(ctx, created.ID)
	if err != nil {
		t.Fatalf("ArchiveOffer: %v", err)
	}
	if archived.Active {
		t.Fatal("an archived offer must not stay active — it would still be sellable")
	}

	// Still present: the record survives.
	again, err := st.OfferByID(ctx, created.ID)
	if err != nil || again == nil {
		t.Fatalf("the offer row disappeared: %v", err)
	}

	// Gone from the catalogue a client buys from.
	visible, err := st.PremiumOffers(ctx)
	if err != nil {
		t.Fatalf("PremiumOffers: %v", err)
	}
	for _, o := range visible {
		if o.ID == created.ID {
			t.Fatal("an archived offer is still offered to clients")
		}
	}

	// Idempotent: the FIRST archive timestamp is the interesting one.
	var first int64
	if err := st.DB().QueryRow(`SELECT archived_at FROM vpn_offers WHERE id = ?`, created.ID).Scan(&first); err != nil {
		t.Fatalf("read archived_at: %v", err)
	}
	if first <= 0 {
		t.Fatal("archived_at was not recorded")
	}
}

// TestDeleteOfferRefusesOneThatHasBeenSold is part two of P-4.
//
// An offer nothing depends on must be removable (a mis-created row), but the record
// of what a customer BOUGHT must survive. Both `vpn_instances.offer_id` and
// `payment_records.offer_id` are `ON DELETE SET NULL`, so a bare DELETE would NOT
// fail — it would quietly erase which product a customer paid for. The guard has to
// be explicit, and both tables have to be checked.
func TestDeleteOfferRefusesOneThatHasBeenSold(t *testing.T) {
	st, serverID := newOfferStore(t)
	ctx := context.Background()

	mk := func(name string) *OfferRow {
		o, err := st.CreateOffer(ctx, CreateOfferInput{
			Kind: "PREMIUM", Name: name, ServerID: serverID,
			TrafficGB: 10, DurationDays: 30, DeviceLimit: 1,
			PriceCents: 50000, Currency: "LKR", Active: true,
		})
		if err != nil {
			t.Fatalf("CreateOffer(%s): %v", name, err)
		}
		return o
	}

	// --- a never-used offer CAN be deleted ---------------------------------
	unused := mk("Mis-created template")
	if err := st.DeleteOffer(ctx, unused.ID); err != nil {
		t.Fatalf("deleting an unused offer = %v, want nil", err)
	}
	if _, err := st.OfferByID(ctx, unused.ID); err == nil {
		t.Fatal("the row survived a successful delete")
	}

	// --- one a VPN came from CANNOT ---------------------------------------
	usedByVPN := mk("Sold once")
	// A real owner: vpn_instances.owner_user_id is a foreign key.
	res, err := st.DB().Exec(
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES ('buyer@example.com', 'x', 'Buyer', 'client', 'ACTIVE', 1, 1, 1)`)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	buyerID, _ := res.LastInsertId()
	if _, err := st.DB().Exec(
		`INSERT INTO vpn_instances (owner_user_id, server_id, panel_email, panel_sub_id, status, offer_id, created_at, updated_at)
		 VALUES (?, ?, 'buyer@example.com_paid_1', 'sub-1', 'ACTIVE', ?, 1, 1)`,
		buyerID, serverID, usedByVPN.ID,
	); err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	if err := st.DeleteOffer(ctx, usedByVPN.ID); err != ErrOfferInUse {
		t.Fatalf("deleting an offer a VPN used = %v, want ErrOfferInUse", err)
	}
	if _, err := st.OfferByID(ctx, usedByVPN.ID); err != nil {
		t.Fatal("the offer was deleted despite being used — the customer's record of what they bought is gone")
	}

	// --- one a PAYMENT names CANNOT ---------------------------------------
	paid := mk("Paid for")
	if _, err := st.DB().Exec(
		`INSERT INTO payment_records (payer_user_id, offer_id, amount_cents, currency, status, created_at, updated_at)
		 VALUES (?, ?, 50000, 'LKR', 'PENDING', 1, 1)`,
		buyerID, paid.ID,
	); err != nil {
		t.Fatalf("seed payment: %v", err)
	}
	if err := st.DeleteOffer(ctx, paid.ID); err != ErrOfferInUse {
		t.Fatalf("deleting an offer a payment names = %v, want ErrOfferInUse", err)
	}

	// --- a missing id is a clean not-found --------------------------------
	if err := st.DeleteOffer(ctx, 999999); err != ErrOfferNotFound {
		t.Fatalf("deleting a missing offer = %v, want ErrOfferNotFound", err)
	}
}
