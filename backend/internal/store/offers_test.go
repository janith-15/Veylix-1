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
