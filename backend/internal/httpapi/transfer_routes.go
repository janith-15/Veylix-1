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

// TransferStore is the persistence a transfer needs (D-064).
//
// Only two operations exist because a transfer is only two things: a write that
// moves ownership atomically, and a read of the history that write produced.
// There is deliberately no "preview" and no "confirm" step — the write itself
// re-reads the owner inside its own transaction, so a confirmation step would
// only add a window in which the console's view can go stale.
type TransferStore interface {
	// TransferVPN moves an assigned VPN to another client account.
	TransferVPN(ctx context.Context, in store.TransferVPNInput) (*store.TransferResult, error)
	// TransfersForVPN returns one VPN's transfer history, newest first.
	TransfersForVPN(ctx context.Context, vpnID int64) ([]store.TransferRow, error)
}

// TransferRoutes registers the ownership-move endpoints (D-064):
//
//	POST /api/vpns/{id}/transfer    — move a VPN to another customer
//	GET  /api/vpns/{id}/transfers   — its transfer history
//
// Both are Super Admin only. Moving an assigned VPN rewrites who may see, use
// and pay for a working credential, which is the most consequential local write
// in the system; a reseller holding `vpn:assign_premium` sells NEW VPNs and has
// no business moving one that already belongs to somebody.
//
// No panel call is made by either route: 3X-UI does not model "which Veylix
// account manages this client", so the client's UUID, sub-id, limits and
// connection links stay valid across the whole life of the VPN. The consequence
// an operator must be told about is therefore the opposite of a rename — the
// customer's existing connection keeps working, and only the account that may
// manage it changes.
func TransferRoutes(mux *http.ServeMux, st TransferStore, aud *audit.Logger,
	authed func(http.Handler) http.Handler) {

	mux.Handle("POST /api/vpns/{id}/transfer",
		authed(RequirePermission(auth.PermAdminManage)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if st == nil {
				WriteError(w, http.StatusServiceUnavailable, "not_wired", "transfers are not available")
				return
			}
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
				return
			}
			vpnID, err := pathID(r)
			if err != nil || vpnID <= 0 {
				WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
				return
			}

			var in struct {
				ToClientUserID int64  `json:"to_client_user_id"`
				SellerUserID   int64  `json:"seller_user_id"`
				Reason         string `json:"reason"`
			}
			if err := DecodeJSON(r, &in); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_body",
					`body must be {"to_client_user_id":<id>}`)
				return
			}
			if in.ToClientUserID <= 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
					"to_client_user_id is required")
				return
			}

			res, err := st.TransferVPN(r.Context(), store.TransferVPNInput{
				VPNID: vpnID, ToClientID: in.ToClientUserID, ActorID: p.UserID,
				SellerID: in.SellerUserID, Reason: in.Reason,
			})
			if err != nil {
				// A refused attempt is audited too: "somebody tried to move
				// this VPN to that account and the server said no" is exactly
				// the question an audit trail is asked after an ownership
				// dispute.
				auditTransfer(aud, r, p.UserID, vpnID, in.ToClientUserID, "DENIED",
					err.Error(), nil)
				writeTransferError(w, err)
				return
			}

			auditTransfer(aud, r, p.UserID, vpnID, in.ToClientUserID, "OK", "",
				map[string]any{
					"server_id":        res.ServerID,
					"panel_email":      res.PanelEmail,
					"from":             res.From.UserID,
					"to":               res.To.UserID,
					"kind":             res.Kind,
					"offer_id":         res.OfferID,
					"free_claim_moved": res.FreeClaimMoved,
					"reason":           strings.TrimSpace(in.Reason),
					"panel_touched":    false,
				})
			WriteJSON(w, http.StatusOK, res)
		}))))

	// GET /api/vpns/{id}/transfers — the history, Super Admin only for the same
	// reason the list above is: it names other customers.
	mux.Handle("GET /api/vpns/{id}/transfers",
		authed(RequirePermission(auth.PermServerViewAll)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if st == nil {
				WriteError(w, http.StatusServiceUnavailable, "not_wired", "transfers are not available")
				return
			}
			vpnID, err := pathID(r)
			if err != nil || vpnID <= 0 {
				WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
				return
			}
			rows, err := st.TransfersForVPN(r.Context(), vpnID)
			if err != nil {
				writeTransferError(w, err)
				return
			}
			WriteJSON(w, http.StatusOK, rows)
		}))))
}

// auditTransfer records one attempt, successful or refused.
func auditTransfer(aud *audit.Logger, r *http.Request, actorID, vpnID, toUserID int64,
	result, refusedBecause string, extra map[string]any) {
	if aud == nil {
		return
	}
	ctx := map[string]any{"vpn_id": vpnID, "to": toUserID}
	if refusedBecause != "" {
		ctx["refused_because"] = refusedBecause
	}
	for k, v := range extra {
		ctx[k] = v
	}
	_ = aud.Record(r.Context(), audit.Entry{
		ActorID: actorID, Action: audit.ActionVPNTransfer,
		TargetType: "vpn", TargetID: strconv.FormatInt(vpnID, 10),
		Result: result, Context: ctx,
	})
}

// writeTransferError maps a store refusal to the one status code that says what
// actually happened, so the console can explain it instead of showing a generic
// failure with no next step.
func writeTransferError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrTransferTarget):
		WriteError(w, http.StatusNotFound, "not_found", "this VPN does not exist")
	case errors.Is(err, store.ErrTransferSelf):
		WriteError(w, http.StatusUnprocessableEntity, "cannot_transfer_self",
			"this VPN already belongs to that customer")
	case errors.Is(err, store.ErrTransferTerminal):
		WriteError(w, http.StatusUnprocessableEntity, "vpn_terminal",
			"this VPN was revoked or removed, so it can no longer be transferred")
	case errors.Is(err, store.ErrTransferNotReady):
		WriteError(w, http.StatusUnprocessableEntity, "vpn_not_ready",
			"the panel has not confirmed this VPN yet — it cannot be handed over")
	case errors.Is(err, store.ErrTransferNotOnPanel):
		WriteError(w, http.StatusUnprocessableEntity, "vpn_not_on_panel",
			"the panel no longer holds this VPN, so there is nothing to hand over")
	case errors.Is(err, store.ErrTransferReleased):
		WriteError(w, http.StatusUnprocessableEntity, "vpn_released",
			"this VPN is released and can still be restored by its owner, so it cannot be transferred")
	case errors.Is(err, store.ErrTransferChanged):
		WriteError(w, http.StatusConflict, "concurrent_change",
			"the owner of this VPN changed while you were moving it — reload and try again")
	case errors.Is(err, store.ErrAssignTarget):
		WriteError(w, http.StatusUnprocessableEntity, "not_a_client",
			"a VPN can only be transferred to an active client account")
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error",
			"could not transfer this VPN")
	}
}
