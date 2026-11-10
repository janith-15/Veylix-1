package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// OrderStore is the D-036 surface: channel changes, proof, timeline and blocks.
type OrderStore interface {
	PaymentByID(ctx context.Context, id int64) (*store.PaymentRow, error)
	CanSeePayment(ctx context.Context, p *store.PaymentRow, callerID int64, callerRole string) (bool, error)
	ChangePaymentMethod(ctx context.Context, payerUserID, paymentID, methodID int64) error
	RecordProofChannel(ctx context.Context, payerUserID, paymentID int64, channel, note string) error
	OrderEvents(ctx context.Context, paymentID int64) ([]store.OrderEvent, error)
	BlockOrderVPN(ctx context.Context, orderID, actorID int64, note string) error
	UnblockOrderVPN(ctx context.Context, orderID, actorID int64, note string) error
	VPNIDForOrder(ctx context.Context, orderID int64) (int64, error)
}

// OrderVPNBlocker stops and restarts the panel client behind an order.
//
// Kept as an interface so this file never imports provisioning: the panel is
// reached through the same service the lifecycle routes use, which already
// writes an outbox entry and confirms with a fresh read.
type OrderVPNBlocker interface {
	BlockVPN(ctx context.Context, vpnID int64) error
	UnblockVPN(ctx context.Context, vpnID int64) error
}

// writeOrderActionError maps an order-action failure to a status code.
func writeOrderActionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrPaymentNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "order not found")
	case errors.Is(err, store.ErrBadTransition):
		WriteError(w, http.StatusConflict, "illegal_transition",
			"this order can no longer be changed")
	case errors.Is(err, store.ErrPaymentMethod):
		WriteError(w, http.StatusUnprocessableEntity, "invalid_method",
			"that payment destination is not available to you")
	case errors.Is(err, store.ErrBadChannel):
		WriteError(w, http.StatusUnprocessableEntity, "invalid_channel",
			"proof must be sent over WhatsApp, Telegram or e-mail")
	case errors.Is(err, store.ErrNoVPNToBlock):
		WriteError(w, http.StatusConflict, "no_vpn_yet",
			"this order has not produced a VPN yet, so there is nothing to block")
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", "could not update the order")
	}
}

// OrderRoutes registers the payer- and owner-facing order endpoints (D-036).
//
// Split from PaymentRoutes on purpose: these are about one order's history and
// the VPN it produced, not about settlement.
func OrderRoutes(mux *http.ServeMux, st OrderStore, blocker OrderVPNBlocker,
	aud *audit.Logger, authed func(http.Handler) http.Handler) {

	record := func(ctx context.Context, e audit.Entry) {
		if aud == nil {
			return
		}
		_ = aud.Record(ctx, e)
	}

	// POST /api/payments/{id}/method — the payer moves the order to another
	// destination (D-036.4). A change is an event, never an overwrite.
	mux.Handle("POST /api/payments/{id}/method", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid order id")
			return
		}
		var in struct {
			MethodID int64 `json:"method_id"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if in.MethodID <= 0 {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "method_id is required")
			return
		}
		if err := st.ChangePaymentMethod(r.Context(), p.UserID, id, in.MethodID); err != nil {
			writeOrderActionError(w, err)
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionPaymentUpdate,
			TargetType: "payment", TargetID: strconv.FormatInt(id, 10),
			Context: map[string]any{"method_id": in.MethodID},
		})
		WriteJSON(w, http.StatusOK, map[string]any{"changed": true})
	})))

	// POST /api/payments/{id}/proof — where the payer sent the slip (D-036.3).
	mux.Handle("POST /api/payments/{id}/proof", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid order id")
			return
		}
		var in struct {
			Channel string `json:"channel"`
			Note    string `json:"note"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if err := st.RecordProofChannel(r.Context(), p.UserID, id, in.Channel, in.Note); err != nil {
			writeOrderActionError(w, err)
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionPaymentUpdate,
			TargetType: "payment", TargetID: strconv.FormatInt(id, 10),
			Context: map[string]any{"proof_channel": in.Channel},
		})
		WriteJSON(w, http.StatusOK, map[string]any{"recorded": true})
	})))

	// GET /api/payments/{id}/events — the timeline both sides read.
	mux.Handle("GET /api/payments/{id}/events", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid order id")
			return
		}
		pay, err := st.PaymentByID(r.Context(), id)
		if err != nil {
			writeOrderActionError(w, err)
			return
		}
		// A payment that exists but belongs to somebody else answers 404, so an
		// id cannot be used to probe for other people's orders.
		visible, err := st.CanSeePayment(r.Context(), pay, p.UserID, string(p.Role))
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the order")
			return
		}
		if !visible {
			WriteError(w, http.StatusNotFound, "not_found", "order not found")
			return
		}
		events, err := st.OrderEvents(r.Context(), pay.ID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the order history")
			return
		}
		WriteJSON(w, http.StatusOK, events)
	})))
}

// BlockRoutes registers the owner's block/unblock action (D-036.5).
//
// The PANEL is changed first: a local SUSPENDED row the panel never accepted
// would tell an operator a VPN is blocked while it still serves traffic.
func BlockRoutes(mux *http.ServeMux, st OrderStore, blocker OrderVPNBlocker,
	aud *audit.Logger, authed func(http.Handler) http.Handler) {

	record := func(ctx context.Context, e audit.Entry) {
		if aud == nil {
			return
		}
		_ = aud.Record(ctx, e)
	}

	for _, action := range []struct {
		path  string
		block bool
	}{
		{"/api/payments/{id}/block", true},
		{"/api/payments/{id}/unblock", false},
	} {
		path, block := action.path, action.block
		mux.Handle("POST "+path, authed(RequirePermission(auth.PermPaymentConfirm)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p, ok := PrincipalFrom(r.Context())
				if !ok {
					WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
					return
				}
				id, err := pathID(r)
				if err != nil {
					WriteError(w, http.StatusBadRequest, "validation_failed", "invalid order id")
					return
				}
				var in struct {
					Note string `json:"note"`
				}
				_ = DecodeJSON(r, &in) // the note is optional

				vpnID, err := st.VPNIDForOrder(r.Context(), id)
				if err != nil {
					writeOrderActionError(w, err)
					return
				}
				if blocker != nil {
					if block {
						err = blocker.BlockVPN(r.Context(), vpnID)
					} else {
						err = blocker.UnblockVPN(r.Context(), vpnID)
					}
					if err != nil {
						WriteError(w, http.StatusBadGateway, "panel_error",
							"the panel did not confirm the change, so the VPN state is unchanged")
						return
					}
				}

				if block {
					err = st.BlockOrderVPN(r.Context(), id, p.UserID, in.Note)
				} else {
					err = st.UnblockOrderVPN(r.Context(), id, p.UserID, in.Note)
				}
				if err != nil {
					writeOrderActionError(w, err)
					return
				}
				record(r.Context(), audit.Entry{
					ActorID: p.UserID, Action: audit.ActionPaymentUpdate,
					TargetType: "payment", TargetID: strconv.FormatInt(id, 10),
					Context: map[string]any{"vpn_id": vpnID, "blocked": block, "note": in.Note},
				})
				WriteJSON(w, http.StatusOK, map[string]any{"vpn_id": vpnID, "blocked": block})
			}))))
	}
}
