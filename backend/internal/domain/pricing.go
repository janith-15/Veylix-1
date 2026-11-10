package domain

import (
	"errors"
	"fmt"
)

// Unit pricing (D-035.4).
//
// A CUSTOM order is priced from a spec the CUSTOMER chooses, so the platform
// needs a unit price per dimension. Everything here is integer arithmetic on
// integer cents: money is never a float (`new_update.md` §6).

// PricingRates is the resolved set of rates that applies to one server: the
// per-server override when one exists, otherwise the platform default.
type PricingRates struct {
	Currency        string
	PerGBCents      int64
	PerDayCents     int64
	PerDeviceCents  int64
	PerIPCents      int64
	MinPriceCents   int64
	MaxPriceCents   int64 // 0 = no ceiling
	MaxTrafficGB    int64 // 0 = no bound
	MaxDurationDays int64
	MaxDevices      int64
	MaxIPs          int64
}

// Spec is what a customer asks for in a CUSTOM PREMIUM order.
type Spec struct {
	TrafficGB int64 `json:"traffic_gb"`
	Days      int64 `json:"days"`
	Devices   int64 `json:"devices"`
	IPs       int64 `json:"ips"`
}

// Quote is the priced answer: the line items, so a screen can show the customer
// why the total is what it is, plus the total itself.
type Quote struct {
	Spec          Spec   `json:"spec"`
	Currency      string `json:"currency"`
	TrafficCents  int64  `json:"traffic_cents"`
	DurationCents int64  `json:"duration_cents"`
	DevicesCents  int64  `json:"devices_cents"`
	IPsCents      int64  `json:"ips_cents"`
	SubtotalCents int64  `json:"subtotal_cents"`
	TotalCents    int64  `json:"total_cents"`
	// MinimumApplied is true when the floor raised the total, so the customer is
	// never surprised: the screen can say "minimum order".
	MinimumApplied bool `json:"minimum_applied"`
}

// Quote errors the transport maps to 422 — a spec is a request, and a refusal
// has to name what is wrong with it.
var (
	ErrSpecEmpty     = errors.New("domain: a spec needs at least one non-zero dimension")
	ErrSpecTraffic   = errors.New("domain: traffic is out of range")
	ErrSpecDays      = errors.New("domain: duration is out of range")
	ErrSpecDevices   = errors.New("domain: device count is out of range")
	ErrSpecIPs       = errors.New("domain: IP count is out of range")
	ErrRatesNotSet   = errors.New("domain: this plan has no unit rates yet")
	ErrQuoteTooLarge = errors.New("domain: the price exceeds the configured ceiling")
)

// QuoteSpec prices one spec against one set of rates.
//
// Rules:
//   - every dimension must be non-negative, and at least one must be positive;
//   - a configured bound refuses the spec instead of quietly clamping it — a
//     customer must never be sold something other than what they typed;
//   - the floor is applied last and reported, so the total is explainable.
func QuoteSpec(r PricingRates, s Spec) (Quote, error) {
	if s.TrafficGB < 0 || s.Days < 0 || s.Devices < 0 || s.IPs < 0 {
		return Quote{}, ErrSpecEmpty
	}
	if s.TrafficGB == 0 && s.Days == 0 && s.Devices == 0 && s.IPs == 0 {
		return Quote{}, ErrSpecEmpty
	}
	if s.TrafficGB == 0 {
		// A VPN with no traffic allowance cannot be used at all; the panel
		// would create a client capped at zero bytes.
		return Quote{}, ErrSpecTraffic
	}
	if r.MaxTrafficGB > 0 && s.TrafficGB > r.MaxTrafficGB {
		return Quote{}, fmt.Errorf("%w: at most %d GB", ErrSpecTraffic, r.MaxTrafficGB)
	}
	if r.MaxDurationDays > 0 && s.Days > r.MaxDurationDays {
		return Quote{}, fmt.Errorf("%w: at most %d days", ErrSpecDays, r.MaxDurationDays)
	}
	if r.MaxDevices > 0 && s.Devices > r.MaxDevices {
		return Quote{}, fmt.Errorf("%w: at most %d", ErrSpecDevices, r.MaxDevices)
	}
	if r.MaxIPs > 0 && s.IPs > r.MaxIPs {
		return Quote{}, fmt.Errorf("%w: at most %d", ErrSpecIPs, r.MaxIPs)
	}
	if r.PerGBCents <= 0 && r.PerDayCents <= 0 && r.PerDeviceCents <= 0 && r.PerIPCents <= 0 {
		// No price stated anywhere: refuse rather than quote zero, which would
		// hand out a paid VPN for free.
		return Quote{}, ErrRatesNotSet
	}

	q := Quote{
		Spec:          s,
		Currency:      r.Currency,
		TrafficCents:  s.TrafficGB * r.PerGBCents,
		DurationCents: s.Days * r.PerDayCents,
		DevicesCents:  s.Devices * r.PerDeviceCents,
		IPsCents:      s.IPs * r.PerIPCents,
	}
	q.SubtotalCents = q.TrafficCents + q.DurationCents + q.DevicesCents + q.IPsCents
	q.TotalCents = q.SubtotalCents
	if r.MinPriceCents > 0 && q.TotalCents < r.MinPriceCents {
		q.TotalCents = r.MinPriceCents
		q.MinimumApplied = true
	}
	if r.MaxPriceCents > 0 && q.TotalCents > r.MaxPriceCents {
		return Quote{}, fmt.Errorf("%w: at most %d %s", ErrQuoteTooLarge, r.MaxPriceCents, r.Currency)
	}
	return q, nil
}
