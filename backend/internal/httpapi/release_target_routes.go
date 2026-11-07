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

// ReleaseTargetStore records which panel a CUSTOM order will be built on.
type ReleaseTargetStore interface {
	SetReleaseTarget(ctx context.Context, orderID, actorID, serverID, inboundID int64) error
}

// ReleaseTargetRoutes registers the Super Admin's choice of panel for a CUSTOM
// order (D-036.6).
//
// This is deliberately separate from the confirmation: an operator can decide
// where a VPN will live before the money arrives, and the order then simply
// waits. The route is Super-Admin only — a reseller has no custom packages, so
// there is nothing for him to place.
func ReleaseTargetRoutes(mux *http.ServeMux, st ReleaseTargetStore,
	aud *audit.Logger, authed func(http.Handler) http.Handler) {

	mux.Handle("POST /api/payments/{id}/release-target",
		authed(RequirePermission(auth.PermOfferManage)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				ServerID  int64 `json:"server_id"`
				InboundID int64 `json:"inbound_id"`
			}
			if err := DecodeJSON(r, &in); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
				return
			}
			if in.ServerID <= 0 || in.InboundID <= 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
					"server_id and inbound_id are both required")
				return
			}
			if err := st.SetReleaseTarget(r.Context(), id, p.UserID, in.ServerID, in.InboundID); err != nil {
				writeReleaseTargetError(w, err)
				return
			}
			if aud != nil {
				_ = aud.Record(r.Context(), audit.Entry{
					ActorID: p.UserID, Action: audit.ActionPaymentUpdate,
					TargetType: "payment", TargetID: strconv.FormatInt(id, 10),
					Context: map[string]any{"server_id": in.ServerID, "inbound_id": in.InboundID},
				})
			}
			WriteJSON(w, http.StatusOK, map[string]any{
				"server_id": in.ServerID, "inbound_id": in.InboundID,
			})
		}))))
}

// writeReleaseTargetError maps a refused target to a status code.
func writeReleaseTargetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrPaymentNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "order not found")
	case errors.Is(err, store.ErrBadTransition):
		WriteError(w, http.StatusConflict, "already_released",
			"this order has already been settled, so its panel cannot change")
	case errors.Is(err, store.ErrReleaseTargetRequired):
		WriteError(w, http.StatusUnprocessableEntity, "invalid_target",
			"that inbound does not belong to that server")
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", "could not set the release target")
	}
}
