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

// OfferStore is the persistence surface the offer handlers need.
type OfferStore interface {
	OffersVisible(ctx context.Context, role string, onlyActive bool) ([]store.OfferRow, error)
	// PremiumOffers is the resale catalogue a client may buy from (D-048). Kept
	// separate from OffersVisible so the FREE-only client view (D-024.3) is not
	// widened by accident.
	PremiumOffers(ctx context.Context) ([]store.OfferRow, error)
	OfferByID(ctx context.Context, id int64) (*store.OfferRow, error)
	CreateOffer(ctx context.Context, in store.CreateOfferInput) (*store.OfferRow, error)
	UpdateOffer(ctx context.Context, id int64, in store.UpdateOfferInput) (*store.OfferRow, error)
}

// OfferRoutes registers the offer catalogue (P3).
//
// This is the endpoint that was missing and blocked everything downstream:
// `POST /api/vpns` requires an `offer_id`, but until now the only way to create
// an offer was to write SQL by hand.
//
// Reading is role-scoped; writing is super-admin only (PermOfferManage), and
// every write is audited.
func OfferRoutes(mux *http.ServeMux, st OfferStore, aud *audit.Logger, authed func(http.Handler) http.Handler) {
	// An audit logger is best-effort for the trail but must never panic a
	// request, so writes go through this guard.
	record := func(ctx context.Context, e audit.Entry) {
		if aud == nil {
			return
		}
		_ = aud.Record(ctx, e)
	}
	// GET /api/offers[?active=1]
	//   super admin / admin -> the catalogue (optionally active only)
	//   client              -> active FREE offers only (the public catalogue)
	mux.Handle("GET /api/offers", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		onlyActive := r.URL.Query().Get("active") == "1"
		offers, err := st.OffersVisible(r.Context(), string(p.Role), onlyActive)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list offers")
			return
		}
		WriteJSON(w, http.StatusOK, offers)
	})))

	// GET /api/offers/premium — the resale catalogue a client may BUY from (D-048).
	//
	// Deliberately separate from GET /api/offers, whose client view is FREE-only
	// by D-024.3. Read-only, authenticated, and it exposes nothing a buyer does
	// not need: the plan's name, limits and price.
	mux.Handle("GET /api/offers/premium", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := PrincipalFrom(r.Context()); !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		offers, err := st.PremiumOffers(r.Context())
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list premium offers")
			return
		}
		WriteJSON(w, http.StatusOK, offers)
	})))

	// POST /api/offers — super admin only.
	mux.Handle("POST /api/offers", authed(RequirePermission(auth.PermOfferManage)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, _ := PrincipalFrom(r.Context())
			var in struct {
				Kind         string `json:"kind"`
				Name         string `json:"name"`
				ServerID     int64  `json:"server_id"`
				InboundID    int64  `json:"inbound_id"`
				TrafficGB    int64  `json:"traffic_gb"`
				DurationDays int    `json:"duration_days"`
				DeviceLimit  int    `json:"device_limit"`
				PriceCents   int64  `json:"price_cents"`
				Currency     string `json:"currency"`
				Active       *bool  `json:"active"`
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
			name := strings.TrimSpace(in.Name)
			if name == "" {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "name is required")
				return
			}
			// An offer without a server can never be provisioned: loadOffer
			// returns ErrOfferInactive for ServerID 0. Reject it here, where the
			// operator can still fix it.
			if in.ServerID <= 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "server_id is required")
				return
			}
			// An offer without an INBOUND is just as unprovisionable, and this check
			// was missing: `inbound_id` defaulted to 0, the row was saved happily
			// (HTTP 201), and the offer then appeared in neither the client catalogue
			// nor a successful claim — it simply could not build a client. The
			// failure was invisible because nothing rejected the create (D-075).
			// A FREE offer is the common case: a Super Admin publishes one, a customer
			// sees "no free offers available", and no error was ever raised.
			if in.InboundID <= 0 {
				WriteError(w, http.StatusUnprocessableEntity, "offer_incomplete",
					"inbound_id is required — an offer with no inbound cannot create a client")
				return
			}
			if in.TrafficGB < 0 || in.DurationDays < 0 || in.DeviceLimit < 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "limits cannot be negative")
				return
			}
			// A negative price would make the catalogue lie about what a buyer
			// owes, and the check must exist here as well as on PATCH — a create
			// is the first place a bad price can enter (caught by
			// TestOfferRoutesRejectNegativePrice).
			if in.PriceCents < 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "price cannot be negative")
				return
			}

			active := true
			if in.Active != nil {
				active = *in.Active
			}

			offer, err := st.CreateOffer(r.Context(), store.CreateOfferInput{
				Kind: kind, Name: name,
				ServerID: in.ServerID, InboundID: in.InboundID,
				TrafficGB: in.TrafficGB, DurationDays: in.DurationDays,
				DeviceLimit: in.DeviceLimit, PriceCents: in.PriceCents, Currency: in.Currency, Active: active,
			})
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "internal_error", "could not create the offer")
				return
			}
			record(r.Context(), audit.Entry{
				ActorID: p.UserID, Action: audit.ActionOfferCreate,
				TargetType: "offer", TargetID: strconv.FormatInt(offer.ID, 10),
				Context: map[string]any{"kind": offer.Kind, "server_id": offer.ServerID},
			})
			WriteJSON(w, http.StatusCreated, offer)
		}))))

	// PATCH /api/offers/{id} — limits and the active toggle; never a delete,
	// because assignments reference the offer and "what was sold" must survive.
	mux.Handle("PATCH /api/offers/{id}", authed(RequirePermission(auth.PermOfferManage)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, _ := PrincipalFrom(r.Context())
			id, err := pathID(r)
			if err != nil {
				WriteError(w, http.StatusBadRequest, "validation_failed", "invalid offer id")
				return
			}
			var in struct {
				Name         *string `json:"name"`
				TrafficGB    *int64  `json:"traffic_gb"`
				DurationDays *int    `json:"duration_days"`
				DeviceLimit  *int    `json:"device_limit"`
				PriceCents   *int64  `json:"price_cents"`
				Currency     *string `json:"currency"`
				Active       *bool   `json:"active"`
			}
			if err := DecodeJSON(r, &in); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
				return
			}
			if in.Name != nil {
				trimmed := strings.TrimSpace(*in.Name)
				if trimmed == "" {
					WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "name cannot be blank")
					return
				}
				in.Name = &trimmed
			}
			if (in.TrafficGB != nil && *in.TrafficGB < 0) ||
				(in.DurationDays != nil && *in.DurationDays < 0) ||
				(in.DeviceLimit != nil && *in.DeviceLimit < 0) {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "limits cannot be negative")
				return
			}
			// Its own statement, not nested inside the limits check: a patch that
			// carries ONLY a bad price must still be refused.
			if in.PriceCents != nil && *in.PriceCents < 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "price cannot be negative")
				return
			}

			offer, err := st.UpdateOffer(r.Context(), id, store.UpdateOfferInput{
				Name: in.Name, TrafficGB: in.TrafficGB, DurationDays: in.DurationDays,
				DeviceLimit: in.DeviceLimit, PriceCents: in.PriceCents, Currency: in.Currency, Active: in.Active,
			})
			if err != nil {
				if errors.Is(err, store.ErrOfferNotFound) {
					WriteError(w, http.StatusNotFound, "not_found", "offer not found")
					return
				}
				WriteError(w, http.StatusInternalServerError, "internal_error", "could not update the offer")
				return
			}
			record(r.Context(), audit.Entry{
				ActorID: p.UserID, Action: audit.ActionOfferUpdate,
				TargetType: "offer", TargetID: strconv.FormatInt(id, 10),
				Context: map[string]any{"active": offer.Active},
			})
			WriteJSON(w, http.StatusOK, offer)
		}))))
}
