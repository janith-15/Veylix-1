package domain

import (
	"errors"
	"testing"
)

// rates is a realistic rate card: LKR 0.50/GB, LKR 5/day, LKR 2/device, LKR 1.50/IP.
func rates() PricingRates {
	return PricingRates{
		Currency: "LKR", PerGBCents: 50, PerDayCents: 500,
		PerDeviceCents: 200, PerIPCents: 150,
		MinPriceCents: 10000, MaxPriceCents: 0,
		MaxTrafficGB: 500, MaxDurationDays: 365, MaxDevices: 10, MaxIPs: 5,
	}
}

// The owner's own example: 200 GB, 30 days, 3 devices, 2 IPs.
//
//	200×0.50 + 3×2.00 + 2×1.50 + 30×5.00 = 256.00
func TestQuoteFromRates(t *testing.T) {
	q, err := QuoteSpec(rates(), Spec{TrafficGB: 200, Days: 30, Devices: 3, IPs: 2})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if q.TrafficCents != 10000 {
		t.Errorf("traffic = %d, want 10000 (200 × 0.50)", q.TrafficCents)
	}
	if q.DurationCents != 15000 {
		t.Errorf("duration = %d, want 15000 (30 × 5.00)", q.DurationCents)
	}
	if q.DevicesCents != 600 {
		t.Errorf("devices = %d, want 600 (3 × 2.00)", q.DevicesCents)
	}
	if q.IPsCents != 300 {
		t.Errorf("ips = %d, want 300 (2 × 1.50)", q.IPsCents)
	}
	if q.SubtotalCents != 25900 {
		t.Errorf("subtotal = %d, want 25900", q.SubtotalCents)
	}
	if q.TotalCents != 25900 {
		t.Errorf("total = %d, want 25900 (the floor did not apply)", q.TotalCents)
	}
	if q.MinimumApplied {
		t.Error("MinimumApplied = true, but the subtotal is above the floor")
	}
}

// A spec that prices below the floor is raised to it, and says so.
func TestQuoteAppliesMinimum(t *testing.T) {
	q, err := QuoteSpec(rates(), Spec{TrafficGB: 1, Days: 1})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if q.SubtotalCents != 550 {
		t.Errorf("subtotal = %d, want 550 (1×0.50 + 1×5.00)", q.SubtotalCents)
	}
	if q.TotalCents != 10000 {
		t.Errorf("total = %d, want the 10000 floor", q.TotalCents)
	}
	if !q.MinimumApplied {
		t.Error("MinimumApplied = false, but the floor raised the total")
	}
}

// A bound refuses the spec; it never clamps it. A customer must not be sold
// something other than what they typed.
func TestQuoteRejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		want error
	}{
		{"traffic too high", Spec{TrafficGB: 501, Days: 30}, ErrSpecTraffic},
		{"days too high", Spec{TrafficGB: 10, Days: 366}, ErrSpecDays},
		{"devices too high", Spec{TrafficGB: 10, Days: 30, Devices: 11}, ErrSpecDevices},
		{"ips too high", Spec{TrafficGB: 10, Days: 30, IPs: 6}, ErrSpecIPs},
		{"no traffic", Spec{Days: 30}, ErrSpecTraffic},
		{"everything zero", Spec{}, ErrSpecEmpty},
		{"negative", Spec{TrafficGB: -5, Days: 30}, ErrSpecEmpty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := QuoteSpec(rates(), tc.spec); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

// No rates at all must refuse, never quote zero — a paid VPN must not be free.
func TestQuoteRefusesWhenNoRatesSet(t *testing.T) {
	r := PricingRates{Currency: "LKR"}
	if _, err := QuoteSpec(r, Spec{TrafficGB: 10, Days: 30}); !errors.Is(err, ErrRatesNotSet) {
		t.Fatalf("error = %v, want ErrRatesNotSet", err)
	}
}

// A ceiling refuses rather than silently capping, so a typo cannot become a
// five-figure order.
func TestQuoteRefusesAboveCeiling(t *testing.T) {
	r := rates()
	r.MaxPriceCents = 20000 // LKR 200
	if _, err := QuoteSpec(r, Spec{TrafficGB: 200, Days: 30, Devices: 3, IPs: 2}); !errors.Is(err, ErrQuoteTooLarge) {
		t.Fatalf("error = %v, want ErrQuoteTooLarge", err)
	}
}
