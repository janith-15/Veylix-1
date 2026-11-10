package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// PlanStore is the persistence surface the plan handlers need (D-030).
type PlanStore interface {
	PlanByID(ctx context.Context, id int64) (*store.Plan, error)
	ListPlans(ctx context.Context, f store.PlansFilter) ([]store.Plan, error)
	CreatePlan(ctx context.Context, in store.CreatePlanInput) (*store.Plan, error)
	UpdatePlan(ctx context.Context, id int64, in store.UpdatePlanInput) (*store.Plan, error)
	ArchivePlan(ctx context.Context, id int64) (*store.Plan, error)
	DeletePlan(ctx context.Context, id int64) error
}

// planFact is the projection a **client** may read (D-032.3).
//
// It answers "what can I buy, and what does it give me" and nothing else: no
// owner, no slot inventory, no reseller economics. Keeping a separate struct —
// rather than hiding fields on the operator list — makes a leak impossible by
// construction instead of by remembering to strip fields.
type planFact struct {
	ID           int64  `json:"id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	TrafficGB    int64  `json:"traffic_gb"`
	DurationDays int    `json:"duration_days"`
	DeviceLimit  int    `json:"device_limit"`
	PriceCents   int64  `json:"price_cents"`
	Currency     string `json:"currency"`
	// ServerLabel is a coarse identifier, never a panel URL or credential.
	ServerLabel string `json:"server_label"`
	// SlotsRemaining is null when the plan is unlimited (D-030.4), so the UI can
	// distinguish "unlimited" from "sold out" without a convention.
	SlotsRemaining *int `json:"slots_remaining"`
	// Purchasable is false when the plan cannot currently be bought: archived,
	// inactive, or sold out.
	Purchasable bool `json:"purchasable"`
}

// toPlanFact strips everything a customer must not see (D-032.3).
func toPlanFact(p *store.Plan) planFact {
	f := planFact{
		ID: p.ID, Kind: p.Kind, Name: p.Name,
		TrafficGB: p.TrafficGB, DurationDays: p.DurationDays, DeviceLimit: p.DeviceLimit,
		PriceCents: p.PriceCents, Currency: p.Currency,
		ServerLabel: "server " + strconv.FormatInt(p.ServerID, 10),
	}
	if p.SlotsRemaining >= 0 {
		remaining := p.SlotsRemaining
		f.SlotsRemaining = &remaining
	}
	f.Purchasable = !p.Archived && p.Active && (p.SlotsRemaining != 0)
	return f
}

// planView is the OPERATOR shape of a plan: everything a staff screen needs to
// render and manage a package, and nothing a customer may not see.
//
// The operator list used to serialise `store.Plan` directly, which dropped the
// fields that are DERIVED rather than stored — `server_label`,
// `slots_remaining` and `purchasable` — while `ApiPlan` in `lib/api.ts`
// declares all three as required. `server_label` therefore arrived as
// `undefined` and the Packages screen rendered **"no server"** on every row,
// including rows whose `server_id` was set correctly; `slots_remaining` fell
// back to `∞` and the purchasable column looked at the wrong list. TypeScript
// cannot catch this: the declared type is a promise the server was not keeping.
//
// It embeds `planFact` for the shared derived fields and adds the operator-only
// ones (`active`, `archived`, `server_id`, `inbound_id`, owner). `owner_user_id`
// is an operator fact: a reseller may see their own packages, never another
// reseller's economics.
type planView struct {
	planFact
	ServerID    int64  `json:"server_id"`
	InboundID   int64  `json:"inbound_id"`
	Active      bool   `json:"active"`
	Archived    bool   `json:"archived"`
	OwnerUserID *int64 `json:"owner_user_id"`
}

// toPlanView is toPlanFact plus the operator-only fields.
func toPlanView(p *store.Plan) planView {
	return planView{
		planFact:    toPlanFact(p),
		ServerID:    p.ServerID,
		InboundID:   p.InboundID,
		Active:      p.Active,
		Archived:    p.Archived,
		OwnerUserID: p.OwnerUserID,
	}
}

// writePlanError maps plan errors to safe responses.
func writePlanError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrOfferNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "plan not found")
	case errors.Is(err, store.ErrPlanInUse):
		WriteError(w, http.StatusConflict, "plan_in_use",
			"this plan is referenced by a VPN, an order or a slot purchase, so it can be archived but not deleted")
	case errors.Is(err, store.ErrSlotsBelowUsed):
		WriteError(w, http.StatusConflict, "slots_below_used",
			"slots_total cannot be lower than the slots already used")
	case errors.Is(err, store.ErrPlanArchived):
		WriteError(w, http.StatusConflict, "plan_archived", "this plan is archived")
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", "could not process the plan")
	}
}

// compile-time guard: the store satisfies the interface.
var _ PlanStore = (*store.Store)(nil)

// PlanRoutes registers the plan catalogue.
//
//   - POST/PATCH/DELETE/archive — Super Admin only (D-030.5), every write audited;
//   - GET — role-scoped: the operator sees ownership and slots, a client sees
//     facts only (D-032.3).
func PlanRoutes(mux *http.ServeMux, st PlanStore, aud *audit.Logger, authed func(http.Handler) http.Handler) {
	record := func(ctx context.Context, e audit.Entry) {
		if aud == nil {
			return
		}
		_ = aud.Record(ctx, e)
	}

	manage := func(next http.HandlerFunc) http.Handler {
		return authed(RequirePermission(auth.PermOfferManage)(next))
	}

	// GET /api/plans?kind=PREMIUM&scope=unowned|mine&archived=1
	mux.Handle("GET /api/plans", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		kind := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("kind")))
		if kind != "" && kind != "FREE" && kind != "PREMIUM" {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "kind must be FREE or PREMIUM")
			return
		}

		// A client may read plan **facts** only (D-032.2/.3), and always only
		// the Premium catalogue: the free catalogue is /api/offers.
		if p.IsClient() {
			f := store.PlansFilter{Kind: kind, ActiveOnly: true}
			if kind == "" {
				f.Kind = "PREMIUM"
			}
			plans, err := st.ListPlans(r.Context(), f)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "internal_error", "could not list plans")
				return
			}
			facts := make([]planFact, 0, len(plans))
			for i := range plans {
				facts = append(facts, toPlanFact(&plans[i]))
			}
			WriteJSON(w, http.StatusOK, facts)
			return
		}

		f := store.PlansFilter{Kind: kind, IncludeArchived: r.URL.Query().Get("archived") == "1"}
		switch strings.ToLower(r.URL.Query().Get("scope")) {
		case "unowned", "inventory", "platform":
			f.UnownedOnly = true
		case "owned", "mine":
			id := p.UserID
			f.OwnerUserID = &id
		}
		plans, err := st.ListPlans(r.Context(), f)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list plans")
			return
		}
		// Answer the operator shape, not the raw store row: the derived fields
		// (`server_label`, `slots_remaining`, `purchasable`) are what the
		// Packages screen renders, and they do not exist on `store.Plan`.
		views := make([]planView, 0, len(plans))
		for i := range plans {
			views = append(views, toPlanView(&plans[i]))
		}
		WriteJSON(w, http.StatusOK, views)
	})))

	// GET /api/plans/{id}
	mux.Handle("GET /api/plans/{id}", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid plan id")
			return
		}
		plan, err := st.PlanByID(r.Context(), id)
		if err != nil {
			writePlanError(w, err)
			return
		}
		if p.IsClient() {
			WriteJSON(w, http.StatusOK, toPlanFact(plan))
			return
		}
		WriteJSON(w, http.StatusOK, plan)
	})))

	// GET /api/packages — the catalogue a CUSTOMER sees.
	//
	// Three kinds, in one answer (D-035.2):
	//   - FREE     — the public catalogue, claimable;
	//   - PREMIUM  — every active package, whoever owns it (the platform's or a
	//                reseller's), so a customer can compare and choose;
	//   - CUSTOM   — the template that says "state your own spec"; its price is
	//                computed by /api/orders/quote, never read from the row.
	//
	// Facts only: no owner, no slot inventory, no reseller economics (D-032.3).
	mux.Handle("GET /api/packages", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		out := make([]map[string]any, 0, 8)

		free, err := st.ListPlans(ctx, store.PlansFilter{Kind: "FREE", ActiveOnly: true})
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list packages")
			return
		}
		premium, err := st.ListPlans(ctx, store.PlansFilter{Kind: "PREMIUM", ActiveOnly: true})
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list packages")
			return
		}

		for _, p := range append(free, premium...) {
			f := toPlanFact(&p)
			kind := "PREMIUM"
			if p.Kind == "FREE" {
				kind = "FREE"
			}
			if p.IsCustom {
				kind = "CUSTOM"
			}
			entry := map[string]any{
				"kind":          kind,
				"id":            f.ID,
				"name":          f.Name,
				"traffic_gb":    f.TrafficGB,
				"duration_days": f.DurationDays,
				"device_limit":  f.DeviceLimit,
				"price_cents":   f.PriceCents,
				"currency":      f.Currency,
				"server_label":  f.ServerLabel,
				"purchasable":   f.Purchasable,
				// A CUSTOM package's price is computed per order, so a screen must
				// ask /api/orders/quote rather than trust the row.
				"priced_per_order": p.IsCustom,
			}
			if f.SlotsRemaining != nil {
				entry["slots_remaining"] = *f.SlotsRemaining
			}
			out = append(out, entry)
		}
		WriteJSON(w, http.StatusOK, out)
	})))

	// POST /api/plans — create (D-030.5).
	mux.Handle("POST /api/plans", manage(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		var in struct {
			Kind         string `json:"kind"`
			Name         string `json:"name"`
			ServerID     int64  `json:"server_id"`
			InboundID    int64  `json:"inbound_id"`
			TrafficGB    int64  `json:"traffic_gb"`
			DurationDays int    `json:"duration_days"`
			DeviceLimit  int    `json:"device_limit"`
			Active       *bool  `json:"active"`
			PriceCents   int64  `json:"price_cents"`
			Currency     string `json:"currency"`
			SlotsTotal   int    `json:"slots_total"`
			// IsCustom creates the CUSTOM template (D-035.2). Super Admin only:
			// a reseller sells fixed packages, never a customer-authored spec.
			IsCustom bool `json:"is_custom"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		kind := strings.ToUpper(strings.TrimSpace(in.Kind))
		if kind != "FREE" && kind != "PREMIUM" {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "kind must be FREE or PREMIUM")
			return
		}
		// D-035.2: "adminlata custom premium kiyala ekak na." A CUSTOM package
		// decides its own price from the rate card, so only the platform may
		// define one — otherwise a reseller could publish a spec-based product
		// the Super Admin never priced.
		if in.IsCustom && !p.IsSuperAdmin() {
			WriteError(w, http.StatusForbidden, "custom_is_platform_only",
				"only a Super Admin may create a custom package")
			return
		}
		if in.IsCustom && kind == "FREE" {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
				"a custom package is always PREMIUM — FREE packages have fixed limits")
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "name is required")
			return
		}
		if in.ServerID <= 0 {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "server_id is required")
			return
		}
		if in.PriceCents < 0 || in.TrafficGB < 0 || in.DurationDays < 0 || in.DeviceLimit < 0 || in.SlotsTotal < 0 {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "limits and price cannot be negative")
			return
		}
		active := true
		if in.Active != nil {
			active = *in.Active
		}
		plan, err := st.CreatePlan(r.Context(), store.CreatePlanInput{
			Kind: kind, Name: strings.TrimSpace(in.Name),
			ServerID: in.ServerID, InboundID: in.InboundID,
			TrafficGB: in.TrafficGB, DurationDays: in.DurationDays,
			DeviceLimit: in.DeviceLimit, Active: active,
			PriceCents: in.PriceCents, Currency: in.Currency, SlotsTotal: in.SlotsTotal,
			IsCustom: in.IsCustom,
		})
		if err != nil {
			writePlanError(w, err)
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionOfferCreate,
			TargetType: "plan", TargetID: strconv.FormatInt(plan.ID, 10),
			Context: map[string]any{
				"kind": plan.Kind, "price_cents": plan.PriceCents, "slots_total": plan.SlotsTotal,
			},
		})
		WriteJSON(w, http.StatusCreated, plan)
	}))

	// PATCH /api/plans/{id} — price, limits, slots, owner, active.
	mux.Handle("PATCH /api/plans/{id}", manage(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid plan id")
			return
		}
		var in struct {
			Name         *string `json:"name"`
			TrafficGB    *int64  `json:"traffic_gb"`
			DurationDays *int    `json:"duration_days"`
			DeviceLimit  *int    `json:"device_limit"`
			Active       *bool   `json:"active"`
			PriceCents   *int64  `json:"price_cents"`
			Currency     *string `json:"currency"`
			SlotsTotal   *int    `json:"slots_total"`
			OwnerUserID  *int64  `json:"owner_user_id"`
			// Unowned returns the plan to platform inventory (D-030.1).
			Unowned bool `json:"unowned"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "name cannot be blank")
			return
		}
		if (in.PriceCents != nil && *in.PriceCents < 0) ||
			(in.TrafficGB != nil && *in.TrafficGB < 0) ||
			(in.DurationDays != nil && *in.DurationDays < 0) ||
			(in.DeviceLimit != nil && *in.DeviceLimit < 0) ||
			(in.SlotsTotal != nil && *in.SlotsTotal < 0) {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "limits and price cannot be negative")
			return
		}
		plan, err := st.UpdatePlan(r.Context(), id, store.UpdatePlanInput{
			Name: in.Name, TrafficGB: in.TrafficGB, DurationDays: in.DurationDays,
			DeviceLimit: in.DeviceLimit, Active: in.Active,
			PriceCents: in.PriceCents, Currency: in.Currency, SlotsTotal: in.SlotsTotal,
			OwnerUserID: in.OwnerUserID, SetOwnerToNull: in.Unowned,
		})
		if err != nil {
			writePlanError(w, err)
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionOfferUpdate,
			TargetType: "plan", TargetID: strconv.FormatInt(id, 10),
			Context: map[string]any{"price_cents": plan.PriceCents, "active": plan.Active},
		})
		WriteJSON(w, http.StatusOK, plan)
	}))

	// POST /api/plans/{id}/archive — leave the catalogue, keep history (D-030.6).
	mux.Handle("POST /api/plans/{id}/archive", manage(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid plan id")
			return
		}
		plan, err := st.ArchivePlan(r.Context(), id)
		if err != nil {
			writePlanError(w, err)
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionOfferUpdate,
			TargetType: "plan", TargetID: strconv.FormatInt(id, 10),
			Context: map[string]any{"archived": true},
		})
		WriteJSON(w, http.StatusOK, plan)
	}))

	// DELETE /api/plans/{id} — the owner's report: a plan template must be
	// removable. Refused with 409 when anything depends on it (D-030.6).
	mux.Handle("DELETE /api/plans/{id}", manage(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid plan id")
			return
		}
		if err := st.DeletePlan(r.Context(), id); err != nil {
			writePlanError(w, err)
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionOfferUpdate,
			TargetType: "plan", TargetID: strconv.FormatInt(id, 10),
			Context: map[string]any{"deleted": true},
		})
		w.WriteHeader(http.StatusNoContent)
	}))
}
