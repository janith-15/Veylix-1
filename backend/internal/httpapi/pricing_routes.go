package httpapi

import (
	"context"
	"errors"
	"net/http"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/domain"
	"veylix/backend/internal/store"
)

// PricingStore is the rate-card surface (D-035.4).
type PricingStore interface {
	ListPricing(ctx context.Context) ([]store.PricingRow, error)
	PricingFor(ctx context.Context, serverID int64) (domain.PricingRates, error)
	SavePricing(ctx context.Context, in store.SavePricingInput) (*store.PricingRow, error)
}

// PricingRoutes registers the rate card and the quote.
//
//   - GET /api/pricing            — every scope (operator only: a customer has no
//     business seeing the platform's wholesale rates);
//   - PUT /api/pricing            — Super Admin states the rates (D-030.5);
//   - POST /api/orders/quote      — a CUSTOM spec in, the server-computed price
//     out. This is what makes "price eka calculate wela pennanna one" true, and
//     it is the only place an order's amount may come from for a custom spec.
func PricingRoutes(mux *http.ServeMux, st PricingStore, aud *audit.Logger, authed func(http.Handler) http.Handler) {
	record := func(ctx context.Context, e audit.Entry) {
		if aud == nil {
			return
		}
		_ = aud.Record(ctx, e)
	}

	// GET /api/pricing — operators only.
	mux.Handle("GET /api/pricing", authed(RequirePermission(auth.PermOfferManage)(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			rows, err := st.ListPricing(r.Context())
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the rates")
				return
			}
			WriteJSON(w, http.StatusOK, rows)
		}))))

	// PUT /api/pricing — Super Admin states the rate card.
	mux.Handle("PUT /api/pricing", authed(RequirePermission(auth.PermOfferManage)(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			p, _ := PrincipalFrom(r.Context())
			var in struct {
				// ServerID nil writes the platform default.
				ServerID        *int64  `json:"server_id"`
				Currency        *string `json:"currency"`
				PerGBCents      *int64  `json:"rate_per_gb_cents"`
				PerDayCents     *int64  `json:"rate_per_day_cents"`
				PerDeviceCents  *int64  `json:"rate_per_device_cents"`
				PerIPCents      *int64  `json:"rate_per_ip_cents"`
				MinPriceCents   *int64  `json:"min_price_cents"`
				MaxPriceCents   *int64  `json:"max_price_cents"`
				MaxTrafficGB    *int64  `json:"max_traffic_gb"`
				MaxDurationDays *int64  `json:"max_duration_days"`
				MaxDevices      *int64  `json:"max_devices"`
				MaxIPs          *int64  `json:"max_ips"`
			}
			if err := DecodeJSON(r, &in); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
				return
			}
			// A negative rate or bound is a client bug: refuse it rather than
			// store a value that would make every future quote wrong.
			for _, v := range []*int64{
				in.PerGBCents, in.PerDayCents, in.PerDeviceCents, in.PerIPCents,
				in.MinPriceCents, in.MaxPriceCents, in.MaxTrafficGB,
				in.MaxDurationDays, in.MaxDevices, in.MaxIPs,
			} {
				if v != nil && *v < 0 {
					WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
						"rates and bounds cannot be negative")
					return
				}
			}
			// Start from the scope as it stands, so a partial write does not
			// silently zero the fields it did not mention.
			scope := int64(0)
			if in.ServerID != nil {
				scope = *in.ServerID
			}
			save := store.SavePricingInput{ServerID: in.ServerID, ActorID: p.UserID}
			cur, err := st.PricingFor(r.Context(), scope)
			if err == nil {
				save.Currency = cur.Currency
				save.PerGBCents = cur.PerGBCents
				save.PerDayCents = cur.PerDayCents
				save.PerDeviceCents = cur.PerDeviceCents
				save.PerIPCents = cur.PerIPCents
				save.MinPriceCents = cur.MinPriceCents
				save.MaxPriceCents = cur.MaxPriceCents
				save.MaxTrafficGB = cur.MaxTrafficGB
				save.MaxDurationDays = cur.MaxDurationDays
				save.MaxDevices = cur.MaxDevices
				save.MaxIPs = cur.MaxIPs
			} else if !errors.Is(err, store.ErrNoPricing) {
				WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the current rates")
				return
			}
			if in.Currency != nil {
				save.Currency = *in.Currency
			}
			if in.PerGBCents != nil {
				save.PerGBCents = *in.PerGBCents
			}
			if in.PerDayCents != nil {
				save.PerDayCents = *in.PerDayCents
			}
			if in.PerDeviceCents != nil {
				save.PerDeviceCents = *in.PerDeviceCents
			}
			if in.PerIPCents != nil {
				save.PerIPCents = *in.PerIPCents
			}
			if in.MinPriceCents != nil {
				save.MinPriceCents = *in.MinPriceCents
			}
			if in.MaxPriceCents != nil {
				save.MaxPriceCents = *in.MaxPriceCents
			}
			if in.MaxTrafficGB != nil {
				save.MaxTrafficGB = *in.MaxTrafficGB
			}
			if in.MaxDurationDays != nil {
				save.MaxDurationDays = *in.MaxDurationDays
			}
			if in.MaxDevices != nil {
				save.MaxDevices = *in.MaxDevices
			}
			if in.MaxIPs != nil {
				save.MaxIPs = *in.MaxIPs
			}

			row, err := st.SavePricing(r.Context(), save)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "internal_error", "could not save the rates")
				return
			}
			record(r.Context(), audit.Entry{
				ActorID: p.UserID, Action: audit.ActionOfferUpdate,
				TargetType: "pricing", TargetID: "pricing",
				Context: map[string]any{
					"server_id": row.ServerID, "currency": row.Currency,
					"per_gb": row.PerGBCents, "per_day": row.PerDayCents,
					"per_device": row.PerDeviceCents, "per_ip": row.PerIPCents,
					"min_price": row.MinPriceCents, "max_price": row.MaxPriceCents,
				},
			})
			WriteJSON(w, http.StatusOK, row)
		}))))

	// POST /api/orders/quote — a spec in, the server-computed price out.
	//
	// Any signed-in account may quote: the customer is deciding what to buy, and
	// the quote states facts about the platform's own rate card — never another
	// customer's data. The client never sends an amount; this is where an amount
	// comes from (D-035.4, D-036.1).
	mux.Handle("POST /api/orders/quote", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ServerID  int64 `json:"server_id"`
			TrafficGB int64 `json:"traffic_gb"`
			Days      int64 `json:"days"`
			Devices   int64 `json:"devices"`
			IPs       int64 `json:"ips"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		rates, err := st.PricingFor(r.Context(), in.ServerID)
		if errors.Is(err, store.ErrNoPricing) {
			WriteError(w, http.StatusConflict, "pricing_not_set",
				"the platform has not published its rates yet, so nothing can be priced")
			return
		}
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the rates")
			return
		}

		quote, err := domain.QuoteSpec(rates, domain.Spec{
			TrafficGB: in.TrafficGB, Days: in.Days, Devices: in.Devices, IPs: in.IPs,
		})
		if err != nil {
			// A refused spec is a bad request, and the message names the field —
			// a customer must be able to fix it without guessing.
			WriteError(w, http.StatusUnprocessableEntity, "invalid_spec", err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, quote)
	})))
}
