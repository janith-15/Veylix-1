package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Plan ownership, price and slot accounting (D-030, D-031).
//
// A "plan" IS a `vpn_offers` row — the same object the catalogue already had —
// extended with an owner, a price and a slot inventory. Keeping one table means
// the panel limits (`traffic_gb`, `duration_days`, `device_limit`) that a plan
// enforces are stated once, in the place provisioning already reads.

// Slot availability. A plan is one of three things:
//
//   - unlimited  (slots_total = 0): the legacy meaning, kept for FREE offers and
//     for every plan created before D-030. It is still subject to the reseller
//     quota and to the FREE claim caps.
//   - inventory  (slots_total > 0): exactly slots_total - slots_used releases.
//   - exhausted (slots_used >= slots_total > 0): refused with ErrPlanExhausted.
type PlanError string

func (e PlanError) Error() string { return string(e) }

const (
	// ErrPlanExhausted means the plan has no slot left to release (D-030.4).
	ErrPlanExhausted = PlanError("store: this plan has no slots left")
	// ErrPlanNotApproved means the caller asked to release an unowned PREMIUM
	// plan — only the Super Admin, after confirming a payment, may do that
	// (D-030.3). It is a hard refusal, never a "pending" state.
	ErrPlanNotApproved = PlanError("store: this plan belongs to the platform and needs a Super Admin confirmation")
	// ErrPlanInUse means a delete was asked for a plan that a VPN or a purchase
	// already references; it must be archived instead (D-030.6).
	ErrPlanInUse = PlanError("store: this plan is in use and cannot be deleted")
	// ErrPlanArchived means the plan has left the catalogue and can no longer be
	// bought or released.
	ErrPlanArchived = PlanError("store: this plan is archived")
)

// Plan is the read projection of a `vpn_offers` row for the API.
//
// `OwnerUserID` nil means **unowned inventory**: the platform's own plan, which
// only the Super Admin may release (D-030.3). `SlotsTotal == 0` keeps its legacy
// meaning — unlimited (D-030.4).
type Plan struct {
	ID           int64  `json:"id"`
	Kind         string `json:"kind"` // FREE | PREMIUM
	Name         string `json:"name"`
	ServerID     int64  `json:"server_id"`
	InboundID    int64  `json:"inbound_id"`
	TrafficGB    int64  `json:"traffic_gb"`
	DurationDays int    `json:"duration_days"`
	DeviceLimit  int    `json:"device_limit"`
	Active       bool   `json:"active"`
	OwnerUserID  *int64 `json:"owner_user_id"`
	PriceCents   int64  `json:"price_cents"`
	Currency     string `json:"currency"`
	SlotsTotal   int    `json:"slots_total"`
	SlotsUsed    int    `json:"slots_used"`
	// SlotsRemaining is -1 when the plan is unlimited, so a client can tell
	// "no limit" from "no slots left" without re-deriving it.
	SlotsRemaining int  `json:"slots_remaining"`
	Archived       bool `json:"archived"`
	// IsCustom marks the CUSTOM template (D-035): the customer states the spec
	// and the price is computed from the rate card, so this row's traffic and
	// duration are only defaults, never the thing that is sold.
	IsCustom  bool  `json:"is_custom"`
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// SpokenFor reports how many plan slots are spoken for.
func (p Plan) SpokenFor() int { return p.SlotsUsed }

const planColumns = `id, kind, name, COALESCE(server_id,0), COALESCE(inbound_id,0),
       traffic_gb, duration_days, device_limit, active,
       owner_user_id, price_cents, currency, slots_total, slots_used, archived_at,
       is_custom, created_at, updated_at`

func scanPlan(sc interface{ Scan(...any) error }) (*Plan, error) {
	var (
		p        Plan
		active   int
		owner    sql.NullInt64
		archived sql.NullInt64
		custom   int
	)
	if err := sc.Scan(&p.ID, &p.Kind, &p.Name, &p.ServerID, &p.InboundID,
		&p.TrafficGB, &p.DurationDays, &p.DeviceLimit, &active,
		&owner, &p.PriceCents, &p.Currency, &p.SlotsTotal, &p.SlotsUsed, &archived,
		&custom, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.Active = active == 1
	p.Archived = archived.Valid
	p.IsCustom = custom == 1
	if owner.Valid {
		v := owner.Int64
		p.OwnerUserID = &v
	}
	switch {
	case p.SlotsTotal <= 0:
		p.SlotsRemaining = -1
	case p.SlotsUsed >= p.SlotsTotal:
		p.SlotsRemaining = 0
	default:
		p.SlotsRemaining = p.SlotsTotal - p.SlotsUsed
	}
	return &p, nil
}

// PlanByID reads one plan, archived or not, for a caller that has already
// decided it may see it.
func (s *Store) PlanByID(ctx context.Context, id int64) (*Plan, error) {
	p, err := scanPlan(s.db.QueryRowContext(ctx,
		`SELECT `+planColumns+` FROM vpn_offers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOfferNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get plan: %w", err)
	}
	return p, nil
}

// PlanByIDTx reads a plan inside a caller's transaction, so a decision made on
// it (a slot check, an ownership check) is made against the same snapshot the
// write will use.
func (s *Store) PlanByIDTx(ctx context.Context, tx *sql.Tx, id int64) (*Plan, error) {
	p, err := scanPlan(tx.QueryRowContext(ctx,
		`SELECT `+planColumns+` FROM vpn_offers WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOfferNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get plan: %w", err)
	}
	return p, nil
}

// PlansFilter narrows a plan listing.
type PlansFilter struct {
	// Kind restricts to FREE or PREMIUM ("" = both).
	Kind string
	// OwnerUserID restricts to one owner (nil = no owner constraint).
	OwnerUserID *int64
	// UnownedOnly restricts to platform inventory, which is what the Super
	// Admin's sell screen needs (D-030.1).
	UnownedOnly bool
	// IncludeArchived keeps archived plans visible (operator views only).
	IncludeArchived bool
	// ActiveOnly hides plans switched off in the catalogue.
	ActiveOnly bool
}

// ListPlans returns plans matching f, cheapest first within a kind.
func (s *Store) ListPlans(ctx context.Context, f PlansFilter) ([]Plan, error) {
	where := make([]string, 0, 5)
	args := make([]any, 0, 3)
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, f.Kind)
	}
	if f.UnownedOnly {
		where = append(where, "owner_user_id IS NULL")
	} else if f.OwnerUserID != nil {
		where = append(where, "owner_user_id = ?")
		args = append(args, *f.OwnerUserID)
	}
	if !f.IncludeArchived {
		where = append(where, "archived_at IS NULL")
	}
	if f.ActiveOnly {
		where = append(where, "active = 1")
	}

	query := `SELECT ` + planColumns + ` FROM vpn_offers`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY kind, price_cents, name"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list plans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]Plan, 0, 8)
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan plan: %w", err)
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// CreatePlanInput is the validated create payload for a plan (D-030).
//
// `OwnerUserID` nil = platform inventory. `SlotsTotal` 0 = unlimited.
type CreatePlanInput struct {
	Kind         string
	Name         string
	ServerID     int64
	InboundID    int64
	TrafficGB    int64
	DurationDays int
	DeviceLimit  int
	Active       bool
	OwnerUserID  *int64
	PriceCents   int64
	Currency     string
	SlotsTotal   int
	// IsCustom marks the CUSTOM template (D-035.2, Super Admin only).
	IsCustom bool
}

// CreatePlan inserts a plan and returns it.
func (s *Store) CreatePlan(ctx context.Context, in CreatePlanInput) (*Plan, error) {
	if in.Currency == "" {
		in.Currency = "LKR"
	}
	now := time.Now().Unix()
	active := 0
	if in.Active {
		active = 1
	}
	custom := 0
	if in.IsCustom {
		custom = 1
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO vpn_offers
		   (kind, name, server_id, inbound_id, traffic_gb, duration_days, device_limit, active,
		    owner_user_id, price_cents, currency, slots_total, slots_used, is_custom, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
		in.Kind, in.Name, nullableID(in.ServerID), nullableID(in.InboundID),
		in.TrafficGB, in.DurationDays, in.DeviceLimit, active,
		nullablePtr(in.OwnerUserID), in.PriceCents, in.Currency, in.SlotsTotal, custom, now, now)
	if err != nil {
		return nil, fmt.Errorf("store: create plan: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.PlanByID(ctx, id)
}

// nullablePtr maps a nil or non-positive *int64 to SQL NULL.
func nullablePtr(v *int64) any {
	if v == nil || *v <= 0 {
		return nil
	}
	return *v
}

// ErrSlotsBelowUsed refuses a slot reduction that would invalidate issued VPNs.
var ErrSlotsBelowUsed = PlanError("store: slots_total cannot be lower than the slots already used")

// UpdatePlanInput carries optional patch fields (nil = leave unchanged).
type UpdatePlanInput struct {
	Name         *string
	TrafficGB    *int64
	DurationDays *int
	DeviceLimit  *int
	Active       *bool
	PriceCents   *int64
	Currency     *string
	SlotsTotal   *int
	OwnerUserID  *int64
	// SetOwnerToNull clears the owner, returning the plan to platform
	// inventory. It exists because nil in a JSON patch means "unchanged", so
	// "make it unowned" needs its own signal.
	SetOwnerToNull bool
}

// UpdatePlan applies a partial update.
//
// `slots_total` may only be raised to, or stay at, a value that still covers
// the slots already handed out: lowering it below `slots_used` would silently
// invalidate VPNs that customers already hold.
func (s *Store) UpdatePlan(ctx context.Context, id int64, in UpdatePlanInput) (*Plan, error) {
	sets := make([]string, 0, 10)
	args := make([]any, 0, 11)

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
	if in.Active != nil {
		sets = append(sets, "active = ?")
		if *in.Active {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	if in.PriceCents != nil {
		sets = append(sets, "price_cents = ?")
		args = append(args, *in.PriceCents)
	}
	if in.Currency != nil {
		sets = append(sets, "currency = ?")
		args = append(args, *in.Currency)
	}
	if in.SetOwnerToNull {
		sets = append(sets, "owner_user_id = NULL")
	} else if in.OwnerUserID != nil {
		sets = append(sets, "owner_user_id = ?")
		args = append(args, nullablePtr(in.OwnerUserID))
	}
	if in.SlotsTotal != nil {
		sets = append(sets, "slots_total = ?")
		args = append(args, *in.SlotsTotal)
	}

	if len(sets) == 0 {
		return s.PlanByID(ctx, id)
	}

	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().Unix(), id)

	// The slot guard lives in the WHERE, not in a check-then-write: SQLite has
	// no multi-column CHECK that can compare against another column of the same
	// row inside an UPDATE, and a separate SELECT would be a race.
	query := `UPDATE vpn_offers SET ` + strings.Join(sets, ", ") + ` WHERE id = ?`
	if in.SlotsTotal != nil {
		query += " AND slots_used <= ?"
		args = append(args, *in.SlotsTotal)
	}

	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: update plan: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Either the row is missing or the slot guard refused it; say which.
		if _, lookupErr := s.PlanByID(ctx, id); lookupErr != nil {
			return nil, ErrOfferNotFound
		}
		return nil, ErrSlotsBelowUsed
	}
	return s.PlanByID(ctx, id)
}

// ArchivePlan removes a plan from every catalogue without destroying the
// history that references it (D-030.6).
func (s *Store) ArchivePlan(ctx context.Context, id int64) (*Plan, error) {
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx,
		`UPDATE vpn_offers SET archived_at = ?, active = 0, updated_at = ?
		  WHERE id = ? AND archived_at IS NULL`, now, now, id); err != nil {
		return nil, fmt.Errorf("store: archive plan: %w", err)
	}
	return s.PlanByID(ctx, id)
}

// DeletePlan hard-deletes a plan that nothing depends on (D-030.6).
//
// The owner's report — "offer template remove karanna ba" — is exactly this
// case: a mis-created template must be removable. A plan that has produced a
// VPN, or that has been sold as slots, is refused with ErrPlanInUse so the
// record of what a customer bought survives.
//
// The guard is `NOT EXISTS` inside the DELETE rather than a preceding COUNT, so
// a VPN created between a check and the delete cannot slip through.
func (s *Store) DeletePlan(ctx context.Context, id int64) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM vpn_offers WHERE id = ?`, id).Scan(&exists); err != nil {
			return fmt.Errorf("store: lookup plan: %w", err)
		}
		if exists == 0 {
			return ErrOfferNotFound
		}

		res, err := tx.ExecContext(ctx,
			`DELETE FROM vpn_offers
			  WHERE id = ?
			    AND NOT EXISTS (SELECT 1 FROM vpn_instances       WHERE offer_id = ?)
			    AND NOT EXISTS (SELECT 1 FROM payment_records     WHERE offer_id = ?)
			    AND NOT EXISTS (SELECT 1 FROM vpn_plan_purchases  WHERE offer_id = ?)`,
			id, id, id, id)
		if err != nil {
			return fmt.Errorf("store: delete plan: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrPlanInUse
		}
		return nil
	})
}

// ClaimPlanSlot consumes one slot of a plan inside the caller's transaction and
// returns the row id that proves it (D-030.4).
//
// This is the whole reason the plan has a slot inventory: two concurrent
// releases against the last slot must produce exactly one VPN. The UPDATE is the
// guard — `slots_used < slots_total` is evaluated by SQLite while it holds the
// write lock, so no separate SELECT can be raced.
//
// A plan with `slots_total = 0` is unlimited (the legacy meaning) and claims
// nothing: it returns 0 and writes no purchase row.
//
// `buyerID` is the reseller whose inventory this slot comes from (0 for the
// platform's own plan). `vpnID` is written once the VPN row exists, so an audit
// can answer "which VPN used this slot"; pass 0 when the caller fills it later
// via LinkSlotToVPN.
func (s *Store) ClaimPlanSlot(ctx context.Context, tx *sql.Tx, planID, buyerID, vpnID int64) (int64, error) {
	var total, used int
	err := tx.QueryRowContext(ctx,
		`SELECT slots_total, slots_used FROM vpn_offers WHERE id = ?`, planID).Scan(&total, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrOfferNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("store: read plan slots: %w", err)
	}
	if total <= 0 {
		// Unlimited: nothing to reserve, nothing to record.
		return 0, nil
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE vpn_offers
		    SET slots_used = slots_used + 1, updated_at = ?
		  WHERE id = ? AND slots_used < slots_total`,
		time.Now().Unix(), planID)
	if err != nil {
		return 0, fmt.Errorf("store: claim plan slot: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrPlanExhausted
	}

	purchaseRes, err := tx.ExecContext(ctx,
		`INSERT INTO vpn_plan_purchases (offer_id, buyer_user_id, quantity, consumed_vpn_id, created_at)
		 VALUES (?, ?, 1, ?, ?)`,
		planID, buyerID, nullableID(vpnID), time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("store: record plan slot: %w", err)
	}
	id, _ := purchaseRes.LastInsertId()
	return id, nil
}

// LinkSlotToVPN attaches a claimed slot to the VPN it produced.
//
// The VPN row is created in the same transaction as the claim in the normal
// path, so this is only needed when the VPN id is not yet known at claim time.
func (s *Store) LinkSlotToVPN(ctx context.Context, tx *sql.Tx, purchaseID, vpnID int64) error {
	if purchaseID <= 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE vpn_plan_purchases SET consumed_vpn_id = ? WHERE id = ?`, vpnID, purchaseID); err != nil {
		return fmt.Errorf("store: link plan slot: %w", err)
	}
	return nil
}

// ReleasePlanSlot gives a claimed slot back, used when a release is rolled back
// (the panel refused the write, so no VPN exists).
func (s *Store) ReleasePlanSlot(ctx context.Context, tx *sql.Tx, planID, purchaseID int64) error {
	if purchaseID > 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM vpn_plan_purchases WHERE id = ? AND consumed_vpn_id IS NULL`, purchaseID); err != nil {
			return fmt.Errorf("store: delete plan slot: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE vpn_offers SET slots_used = MAX(slots_used - 1, 0), updated_at = ?
		  WHERE id = ? AND slots_total > 0`, time.Now().Unix(), planID); err != nil {
		return fmt.Errorf("store: return plan slot: %w", err)
	}
	return nil
}

// PlanReleaseDecision is what the ownership gate answers before any write.
type PlanReleaseDecision struct {
	// NeedsSuperAdminConfirm is true for an unowned PREMIUM plan (D-030.3): the
	// caller must refuse the release unless a Super Admin has confirmed a
	// payment for it.
	NeedsSuperAdminConfirm bool
	// OwnerID is the reseller that owns the plan (0 = platform inventory).
	OwnerID int64
}

// CheckPlanRelease encodes D-030.2 and D-030.3 in one place, so provisioning and
// the API cannot disagree about who may release a plan.
//
// Rules:
//   - FREE plans never need a confirmation (they are capped by D-025 instead);
//   - a PREMIUM plan with no owner needs a Super-Admin confirmation;
//   - a PREMIUM plan owned by the caller may be released immediately;
//   - a PREMIUM plan owned by someone else is not the caller's to release
//     (the caller gets ErrNotPlanOwner via the returned error).
func (s *Store) CheckPlanRelease(p *Plan, callerID int64, callerRole string) (PlanReleaseDecision, error) {
	if p == nil {
		return PlanReleaseDecision{}, ErrOfferNotFound
	}
	if p.Kind != "PREMIUM" {
		// FREE: public and claimable, governed by the FREE caps.
		return PlanReleaseDecision{}, nil
	}
	if p.OwnerUserID == nil {
		return PlanReleaseDecision{NeedsSuperAdminConfirm: true}, nil
	}
	owner := *p.OwnerUserID
	if owner == callerID || callerRole == "super_admin" {
		return PlanReleaseDecision{OwnerID: owner}, nil
	}
	return PlanReleaseDecision{OwnerID: owner}, ErrNotPlanOwner
}

// ErrNotPlanOwner means the caller tried to release a plan that belongs to
// another reseller.
var ErrNotPlanOwner = PlanError("store: this plan belongs to another reseller")
