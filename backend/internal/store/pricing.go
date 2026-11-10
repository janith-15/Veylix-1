package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"veylix/backend/internal/domain"
)

// Unit pricing (D-035.4).
//
// Two scopes exist: the platform default (`server_id IS NULL`) and one override
// per server. Resolution is "the override if there is one, else the default",
// which is the owner's Q1 answer — a global rate card with per-server exceptions.

// ErrNoPricing means neither scope has a row. It is distinct from "a row with
// zero rates", because a missing row is a setup problem and a zero rate is an
// operator decision.
var ErrNoPricing = errors.New("store: no pricing rates configured")

// PricingRow is one scope's rates, as the API returns them.
type PricingRow struct {
	ID              int64  `json:"id"`
	ServerID        *int64 `json:"server_id"`
	Currency        string `json:"currency"`
	PerGBCents      int64  `json:"rate_per_gb_cents"`
	PerDayCents     int64  `json:"rate_per_day_cents"`
	PerDeviceCents  int64  `json:"rate_per_device_cents"`
	PerIPCents      int64  `json:"rate_per_ip_cents"`
	MinPriceCents   int64  `json:"min_price_cents"`
	MaxPriceCents   int64  `json:"max_price_cents"`
	MaxTrafficGB    int64  `json:"max_traffic_gb"`
	MaxDurationDays int64  `json:"max_duration_days"`
	MaxDevices      int64  `json:"max_devices"`
	MaxIPs          int64  `json:"max_ips"`
	UpdatedBy       *int64 `json:"updated_by"`
	UpdatedAt       int64  `json:"updated_at"`
}

const pricingColumns = `id, server_id, currency,
	rate_per_gb_cents, rate_per_day_cents, rate_per_device_cents, rate_per_ip_cents,
	min_price_cents, max_price_cents, max_traffic_gb, max_duration_days, max_devices, max_ips,
	updated_by, updated_at`

func scanPricing(sc interface{ Scan(...any) error }) (*PricingRow, error) {
	var (
		r        PricingRow
		server   sql.NullInt64
		updatedB sql.NullInt64
	)
	if err := sc.Scan(&r.ID, &server, &r.Currency,
		&r.PerGBCents, &r.PerDayCents, &r.PerDeviceCents, &r.PerIPCents,
		&r.MinPriceCents, &r.MaxPriceCents, &r.MaxTrafficGB, &r.MaxDurationDays,
		&r.MaxDevices, &r.MaxIPs, &updatedB, &r.UpdatedAt); err != nil {
		return nil, err
	}
	if server.Valid {
		v := server.Int64
		r.ServerID = &v
	}
	if updatedB.Valid {
		v := updatedB.Int64
		r.UpdatedBy = &v
	}
	return &r, nil
}

// ListPricing returns every configured scope, the platform default first.
func (s *Store) ListPricing(ctx context.Context) ([]PricingRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+pricingColumns+` FROM pricing_rates
		  ORDER BY (server_id IS NOT NULL), server_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list pricing: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]PricingRow, 0, 4)
	for rows.Next() {
		r, err := scanPricing(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan pricing: %w", err)
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// PricingFor resolves the rates that apply to one server (0 = the platform
// default only).
//
// A server with no override inherits the platform default — that is the whole
// point of "global default + per-server override", and it means a new panel
// needs no pricing setup before it can quote.
func (s *Store) PricingFor(ctx context.Context, serverID int64) (domain.PricingRates, error) {
	r, err := s.scopeRow(ctx, serverID)
	if errors.Is(err, sql.ErrNoRows) && serverID > 0 {
		// No override for this server: fall back to the platform default.
		r, err = s.scopeRow(ctx, 0)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PricingRates{}, ErrNoPricing
	}
	if err != nil {
		return domain.PricingRates{}, fmt.Errorf("store: pricing for server: %w", err)
	}
	return domain.PricingRates{
		Currency:        r.Currency,
		PerGBCents:      r.PerGBCents,
		PerDayCents:     r.PerDayCents,
		PerDeviceCents:  r.PerDeviceCents,
		PerIPCents:      r.PerIPCents,
		MinPriceCents:   r.MinPriceCents,
		MaxPriceCents:   r.MaxPriceCents,
		MaxTrafficGB:    r.MaxTrafficGB,
		MaxDurationDays: r.MaxDurationDays,
		MaxDevices:      r.MaxDevices,
		MaxIPs:          r.MaxIPs,
	}, nil
}

// scopeRow reads one scope verbatim: serverID 0 means the platform default.
func (s *Store) scopeRow(ctx context.Context, serverID int64) (*PricingRow, error) {
	return scanPricing(s.db.QueryRowContext(ctx,
		`SELECT `+pricingColumns+` FROM pricing_rates WHERE server_id IS ?`, nullableID(serverID)))
}

// SavePricingInput is one validated write.
type SavePricingInput struct {
	// ServerID nil writes the platform default, a value writes that override.
	ServerID        *int64
	Currency        string
	PerGBCents      int64
	PerDayCents     int64
	PerDeviceCents  int64
	PerIPCents      int64
	MinPriceCents   int64
	MaxPriceCents   int64
	MaxTrafficGB    int64
	MaxDurationDays int64
	MaxDevices      int64
	MaxIPs          int64
	ActorID         int64
}

// SavePricing creates or replaces one scope's rates.
//
// Upsert, not insert: an operator restates a rate card, they do not accumulate
// versions of it. The history of a change lives in the audit log, which is the
// only place it is trustworthy.
func (s *Store) SavePricing(ctx context.Context, in SavePricingInput) (*PricingRow, error) {
	if in.Currency == "" {
		in.Currency = "LKR"
	}
	var server any
	if in.ServerID != nil && *in.ServerID > 0 {
		server = *in.ServerID
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO pricing_rates
		   (server_id, currency, rate_per_gb_cents, rate_per_day_cents,
		    rate_per_device_cents, rate_per_ip_cents, min_price_cents, max_price_cents,
		    max_traffic_gb, max_duration_days, max_devices, max_ips, updated_by, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (COALESCE(server_id, 0)) DO UPDATE SET
		   currency = excluded.currency,
		   rate_per_gb_cents = excluded.rate_per_gb_cents,
		   rate_per_day_cents = excluded.rate_per_day_cents,
		   rate_per_device_cents = excluded.rate_per_device_cents,
		   rate_per_ip_cents = excluded.rate_per_ip_cents,
		   min_price_cents = excluded.min_price_cents,
		   max_price_cents = excluded.max_price_cents,
		   max_traffic_gb = excluded.max_traffic_gb,
		   max_duration_days = excluded.max_duration_days,
		   max_devices = excluded.max_devices,
		   max_ips = excluded.max_ips,
		   updated_by = excluded.updated_by,
		   updated_at = excluded.updated_at`,
		server, in.Currency, in.PerGBCents, in.PerDayCents,
		in.PerDeviceCents, in.PerIPCents, in.MinPriceCents, in.MaxPriceCents,
		in.MaxTrafficGB, in.MaxDurationDays, in.MaxDevices, in.MaxIPs,
		nullableID(in.ActorID), time.Now().Unix())
	if err != nil {
		return nil, fmt.Errorf("store: save pricing: %w", err)
	}

	scope := int64(0)
	if in.ServerID != nil {
		scope = *in.ServerID
	}
	r, err := s.scopeRow(ctx, scope)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoPricing
	}
	if err != nil {
		return nil, fmt.Errorf("store: read pricing: %w", err)
	}
	return r, nil
}
