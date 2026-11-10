package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrOfferNotFound is returned when an offer id does not exist.
var ErrOfferNotFound = errors.New("store: offer not found")

// OfferRow is an offer as returned to the API.
//
// `TrafficGB` / `DurationDays` / `DeviceLimit` of 0 keep their documented
// meaning from 0002_business.sql: unlimited / no expiry / unlimited devices.
type OfferRow struct {
	ID           int64  `json:"id"`
	Kind         string `json:"kind"` // FREE | PREMIUM
	Name         string `json:"name"`
	ServerID     int64  `json:"server_id"`
	InboundID    int64  `json:"inbound_id"`
	TrafficGB    int64  `json:"traffic_gb"`
	DurationDays int    `json:"duration_days"`
	DeviceLimit  int    `json:"device_limit"`
	Active       bool   `json:"active"`
	// PriceCents is what the buyer pays, in minor units (D-047).
	// The panel has no notion of price, so the catalogue is the only place one
	// can exist. Until this was exposed the buy screen asked the BUYER to type
	// the amount, which is not a price at all.
	PriceCents int64  `json:"price_cents"`
	Currency   string `json:"currency"`
	// ArchivedAt is when the offer left the catalogue (P-4); 0 while listed.
	ArchivedAt int64 `json:"archived_at"`
	CreatedAt  int64 `json:"created_at"`
	UpdatedAt  int64 `json:"updated_at"`
}

// nullableID maps 0 to SQL NULL so the FK columns stay clean.
func nullableID(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

const offerColumns = `id, kind, name, COALESCE(server_id,0), COALESCE(inbound_id,0),
	       traffic_gb, duration_days, device_limit, price_cents, currency, active,
	       COALESCE(archived_at,0), created_at, updated_at`

// scanOffer reads one row in the offerColumns order.
func scanOffer(sc interface{ Scan(...any) error }) (*OfferRow, error) {
	var o OfferRow
	var active int
	if err := sc.Scan(&o.ID, &o.Kind, &o.Name, &o.ServerID, &o.InboundID,
		&o.TrafficGB, &o.DurationDays, &o.DeviceLimit, &o.PriceCents, &o.Currency, &active,
		&o.ArchivedAt, &o.CreatedAt, &o.UpdatedAt); err != nil {
		return nil, err
	}
	o.Active = active == 1
	return &o, nil
}

// OffersVisible returns the offers a principal may read.
//
// Rules (REQUIREMENTS §2.4, §4):
//   - super admin / admin: every offer, or only the active ones with onlyActive;
//   - client: **active FREE offers only** — that is the public catalogue.
//
// A reseller's server scope is enforced where it matters: provisioning
// re-checks ownership of the target server, so listing a catalogue here cannot
// grant the ability to use it.
// PremiumOffers is the resale catalogue a CLIENT may read (D-048).
//
// D-024.3 governs VISIBILITY OF VPNs: a client sees a PREMIUM VPN only once it is
// assigned to them. It says nothing about the PLANS a client may buy, and the
// Buy Premium screen needs those — otherwise the page can never show a plan and
// premium could only ever be sold by a reseller.
//
// So this is a deliberate, separate read path: active PREMIUM offers only. The
// free catalogue (`OffersVisible` for a client) is untouched.
func (s *Store) PremiumOffers(ctx context.Context) ([]OfferRow, error) {
	return s.offersWhere(ctx, "active = 1", "kind = 'PREMIUM'")
}

// offersWhere runs the catalogue query with the given extra conditions.
func (s *Store) offersWhere(ctx context.Context, conditions ...string) ([]OfferRow, error) {
	query := `SELECT ` + offerColumns + ` FROM vpn_offers`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY name`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("store: list offers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]OfferRow, 0, 8)
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan offer: %w", err)
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

func (s *Store) OffersVisible(ctx context.Context, role string, onlyActive bool) ([]OfferRow, error) {
	query := `SELECT ` + offerColumns + ` FROM vpn_offers`
	conditions := make([]string, 0, 2)

	switch {
	case role == "client":
		conditions = append(conditions, "active = 1", "kind = 'FREE'")
	case onlyActive:
		conditions = append(conditions, "active = 1")
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY kind, name`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("store: list offers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]OfferRow, 0, 8)
	for rows.Next() {
		o, err := scanOffer(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan offer: %w", err)
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

// OfferByID reads one offer.
func (s *Store) OfferByID(ctx context.Context, id int64) (*OfferRow, error) {
	o, err := scanOffer(s.db.QueryRowContext(ctx,
		`SELECT `+offerColumns+` FROM vpn_offers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOfferNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get offer: %w", err)
	}
	return o, nil
}

// InboundBelongsToServer reports whether `inboundID` is cached against
// `serverID`.
//
// An offer pairs a server with an inbound, and only that pairing can build a
// client: the panel call sends the inbound to the server that owns it. A row that
// names server A and an inbound from server B is unprovisionable, so creation
// refuses it rather than saving a plan nobody can buy. A read error is treated as
// "not confirmed", because guessing here produces exactly the row we must avoid.
func (s *Store) InboundBelongsToServer(ctx context.Context, serverID, inboundID int64) bool {
	if serverID <= 0 || inboundID <= 0 {
		return false
	}
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM inbounds WHERE id = ? AND server_id = ?`,
		inboundID, serverID).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// CreateOfferInput is the validated create payload.
type CreateOfferInput struct {
	Kind         string
	Name         string
	ServerID     int64
	InboundID    int64
	TrafficGB    int64
	DurationDays int
	DeviceLimit  int
	PriceCents   int64
	Currency     string
	Active       bool
}

// CreateOffer inserts an offer and returns it.
func (s *Store) CreateOffer(ctx context.Context, in CreateOfferInput) (*OfferRow, error) {
	now := time.Now().Unix()
	active := 0
	if in.Active {
		active = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO vpn_offers
	   (kind, name, server_id, inbound_id, traffic_gb, duration_days, device_limit, price_cents, currency, active, created_at, updated_at)
	 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.Kind, in.Name, nullableID(in.ServerID), nullableID(in.InboundID),
		in.TrafficGB, in.DurationDays, in.DeviceLimit, in.PriceCents, in.Currency, active, now, now)
	if err != nil {
		return nil, fmt.Errorf("store: create offer: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.OfferByID(ctx, id)
}

// UpdateOfferInput carries optional patch fields (nil = leave unchanged).
type UpdateOfferInput struct {
	Name         *string
	TrafficGB    *int64
	DurationDays *int
	DeviceLimit  *int
	PriceCents   *int64
	Currency     *string
	Active       *bool
}

// UpdateOffer applies a partial update.
//
// An offer is never deleted: existing VPN assignments reference it, and the
// record of "what was sold" must survive (REQUIREMENTS §3.3). Deactivating
// (`active=false`) is how an offer leaves the catalogue.
func (s *Store) UpdateOffer(ctx context.Context, id int64, in UpdateOfferInput) (*OfferRow, error) {
	sets := make([]string, 0, 6)
	args := make([]any, 0, 7)

	if in.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *in.Name)
	}
	if in.TrafficGB != nil {
		sets = append(sets, "traffic_gb = ?")
		args = append(args, *in.TrafficGB)
	}
	if in.DurationDays != nil {
		sets = append(sets, "duration_days = ?")
		args = append(args, *in.DurationDays)
	}
	if in.DeviceLimit != nil {
		sets = append(sets, "device_limit = ?")
		args = append(args, *in.DeviceLimit)
	}
	if in.PriceCents != nil {
		sets = append(sets, "price_cents = ?")
		args = append(args, *in.PriceCents)
	}
	if in.Currency != nil {
		sets = append(sets, "currency = ?")
		args = append(args, *in.Currency)
	}
	if in.Active != nil {
		sets = append(sets, "active = ?")
		if *in.Active {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if len(sets) == 0 {
		// Nothing to change: return the current row rather than an error, so a
		// PATCH that only re-sends the same value is idempotent.
		return s.OfferByID(ctx, id)
	}

	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().Unix(), id)

	res, err := s.db.ExecContext(ctx,
		`UPDATE vpn_offers SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: update offer: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrOfferNotFound
	}
	return s.OfferByID(ctx, id)
}

// ArchiveOffer retires an offer from the catalogue without destroying it (P-4).
//
// The comment on `UpdateOffer` says an offer is "never deleted: existing VPN
// assignments reference it". That reasoning is correct for an offer that HAS been
// sold, and it was silently applied to offers that never were: the only way to get
// a mis-created offer out of the shop was `active = false`, which hides it but
// leaves no record of WHY, and every listing still shows it as "inactive" forever.
//
// Archiving is the honest middle step and it mirrors `ArchivePlan` (plans.go):
// `archived_at` records when and `active = 0` removes it from the catalogue, so the
// row still exists for anything that references it. It is IDEMPOTENT — archiving an
// already-archived offer returns the row rather than moving the timestamp, because
// the interesting fact is when it was retired, not when someone last pressed the
// button.
func (s *Store) ArchiveOffer(ctx context.Context, id int64) (*OfferRow, error) {
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE vpn_offers SET archived_at = ?, active = 0, updated_at = ?
		  WHERE id = ? AND archived_at IS NULL`, now, now, id); err != nil {
		return nil, fmt.Errorf("store: archive offer: %w", err)
	}
	return s.OfferByID(ctx, id)
}

// ErrOfferInUse refuses to delete an offer that a VPN already came from.
var ErrOfferInUse = errors.New("store: offer has been used and cannot be deleted")

// DeleteOffer hard-deletes an offer that nothing depends on (P-4).
//
// The same rule as `DeletePlan` (D-030.6): a mis-created offer must be removable,
// but the record of what a customer bought must survive. An offer that produced a
// VPN, or that was sold as slots, is refused with ErrOfferInUse and the operator
// archives it instead.
//
// The guard is `NOT EXISTS` INSIDE the DELETE rather than a preceding COUNT, so a
// VPN created between a check and the delete cannot slip through.
func (s *Store) DeleteOffer(ctx context.Context, id int64) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM vpn_offers WHERE id = ?`, id).Scan(&exists); err != nil {
			return fmt.Errorf("store: lookup offer: %w", err)
		}
		if exists == 0 {
			return ErrOfferNotFound
		}

		/*
		 * Anything that represents money or a customer's property keeps the offer
		 * alive. `vpn_instances.offer_id` and `payment_records.offer_id` are both
		 * `ON DELETE SET NULL`, so a bare DELETE would not fail — it would quietly
		 * erase which product a customer paid for. That is exactly the record the
		 * comment on UpdateOffer says must survive, so it is checked explicitly.
		 */
		var used int
		if err := tx.QueryRowContext(ctx,
			`SELECT (SELECT COUNT(*) FROM vpn_instances   WHERE offer_id = ?)
			      + (SELECT COUNT(*) FROM payment_records WHERE offer_id = ?)`,
			id, id).Scan(&used); err != nil {
			return fmt.Errorf("store: check offer usage: %w", err)
		}
		if used > 0 {
			return ErrOfferInUse
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM vpn_offers WHERE id = ?`, id); err != nil {
			return fmt.Errorf("store: delete offer: %w", err)
		}
		return nil
	})
}
