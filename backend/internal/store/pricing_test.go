package store

import (
	"context"
	"errors"
	"testing"
)

// D-035.4 — a server with no override inherits the platform default, so a new
// panel can quote without any setup.
func TestPricingFallsBackToPlatformDefault(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)

	if _, err := st.SavePricing(ctx, SavePricingInput{
		PerGBCents: 50, PerDayCents: 500, PerDeviceCents: 200, PerIPCents: 150,
		MinPriceCents: 10000, MaxTrafficGB: 500,
	}); err != nil {
		t.Fatalf("save default: %v", err)
	}

	got, err := st.PricingFor(ctx, serverID)
	if err != nil {
		t.Fatalf("PricingFor: %v", err)
	}
	if got.PerGBCents != 50 || got.PerDayCents != 500 {
		t.Errorf("rates = %+v, want the platform default", got)
	}
	if got.Currency != "LKR" {
		t.Errorf("currency = %q, want LKR", got.Currency)
	}
}

// The owner's Q1 answer: a per-server override wins for that server only.
func TestServerRateOverridesGlobal(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID := seedServer(t, st)
	// A second panel with its own URL: an override is per server, so the test
	// needs two real servers.
	res, err := st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('srv2', 'https://panel2.example:2053/base', x'00', 'ACTIVE', 1, 1)`)
	if err != nil {
		t.Fatalf("seed second server: %v", err)
	}
	otherServer, _ := res.LastInsertId()

	if _, err := st.SavePricing(ctx, SavePricingInput{PerGBCents: 50, MinPriceCents: 10000}); err != nil {
		t.Fatalf("save default: %v", err)
	}
	override := serverID
	if _, err := st.SavePricing(ctx, SavePricingInput{
		ServerID: &override, Currency: "USD", PerGBCents: 900, MinPriceCents: 50000,
	}); err != nil {
		t.Fatalf("save override: %v", err)
	}

	got, err := st.PricingFor(ctx, serverID)
	if err != nil {
		t.Fatalf("PricingFor(override): %v", err)
	}
	if got.PerGBCents != 900 || got.Currency != "USD" {
		t.Errorf("override = %+v, want 900 cents in USD", got)
	}

	// A different server keeps the default: an override is scoped, not global.
	other, err := st.PricingFor(ctx, otherServer)
	if err != nil {
		t.Fatalf("PricingFor(other): %v", err)
	}
	if other.PerGBCents != 50 || other.Currency != "LKR" {
		t.Errorf("other server = %+v, want the platform default", other)
	}
}

// Saving twice replaces the rate card instead of accumulating versions.
func TestSavePricingIsUpsert(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := st.SavePricing(ctx, SavePricingInput{PerGBCents: 50}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := st.SavePricing(ctx, SavePricingInput{PerGBCents: 75}); err != nil {
		t.Fatalf("second: %v", err)
	}

	rows, err := st.ListPricing(ctx)
	if err != nil {
		t.Fatalf("ListPricing: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rate-scope rows = %d, want 1 (a restatement replaces)", len(rows))
	}
	if rows[0].PerGBCents != 75 {
		t.Errorf("rate = %d, want 75", rows[0].PerGBCents)
	}
}

// A migration seeds a zero-rate platform default, and asking for it must not
// pretend a price exists (domain.QuoteSpec refuses zero rates).
func TestPricingSeededRowIsZeroNotFake(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	got, err := st.PricingFor(ctx, 0)
	if err != nil {
		t.Fatalf("PricingFor: %v", err)
	}
	if got.PerGBCents != 0 || got.PerDayCents != 0 {
		t.Errorf("seeded rates = %+v, want zeros until an operator states prices", got)
	}
}

// Asking for a scope that does not exist and has no default is an explicit
// error, never a silent zero.
func TestPricingMissingScopeIsAnError(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := st.DB().Exec(`DELETE FROM pricing_rates`); err != nil {
		t.Fatalf("clear pricing: %v", err)
	}
	if _, err := st.PricingFor(ctx, 0); !errors.Is(err, ErrNoPricing) {
		t.Fatalf("error = %v, want ErrNoPricing", err)
	}
}
