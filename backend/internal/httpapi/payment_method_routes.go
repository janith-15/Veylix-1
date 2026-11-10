package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/store"
)

// paymentRoutes registers the destination and record endpoints.
func paymentRoutes(mux *http.ServeMux, st PaymentStore, record func(context.Context, audit.Entry), authed func(http.Handler) http.Handler) {
	// ---- destinations -----------------------------------------------------

	// GET /api/payment-methods — global methods plus the caller's own.
	mux.Handle("GET /api/payment-methods", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		rows, err := st.PaymentMethodsVisible(r.Context(), p.UserID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list payment methods")
			return
		}
		WriteJSON(w, http.StatusOK, rows)
	})))

	// POST /api/payment-methods — super admin publishes a global method; an
	// admin publishes their own. A client has nothing to publish.
	mux.Handle("POST /api/payment-methods", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		if p.IsClient() {
			WriteError(w, http.StatusForbidden, "forbidden", "not available for this role")
			return
		}
		var in struct {
			Kind         string `json:"kind"`
			Label        string `json:"label"`
			Details      string `json:"details"`
			Instructions string `json:"instructions"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		kind := strings.ToUpper(strings.TrimSpace(in.Kind))
		switch kind {
		case "BANK", "EZCASH", "MOBILE", "OTHER":
		default:
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
				"kind must be BANK, EZCASH, MOBILE or OTHER")
			return
		}
		label := strings.TrimSpace(in.Label)
		if label == "" {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "label is required")
			return
		}
		// Owner 0 = a platform-wide method (super admin only); an admin's
		// method belongs to that admin, so only their own clients see it.
		owner := p.UserID
		if p.IsSuperAdmin() {
			owner = 0
		}
		m, err := st.CreatePaymentMethod(r.Context(), store.CreatePaymentMethodInput{
			OwnerUserID: owner, Kind: kind, Label: label,
			Details: strings.TrimSpace(in.Details), Instructions: strings.TrimSpace(in.Instructions),
		})
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not create the payment method")
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionPaymentMethodCreate,
			TargetType: "payment_method", TargetID: strconv.FormatInt(m.ID, 10),
			Context: map[string]any{"kind": m.Kind, "global": owner == 0},
		})
		WriteJSON(w, http.StatusCreated, m)
	})))

	// PATCH /api/payment-methods/{id} — activate/retire. Never a delete: a
	// confirmed payment must keep the destination it was made to.
	mux.Handle("PATCH /api/payment-methods/{id}", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid payment method id")
			return
		}
		m, err := st.PaymentMethodByID(r.Context(), id)
		if err != nil {
			WriteError(w, http.StatusNotFound, "not_found", "payment method not found")
			return
		}
		// A global method belongs to the super admin; an admin owns only the
		// methods they published.
		if !p.IsSuperAdmin() && (m.OwnerUserID == nil || *m.OwnerUserID != p.UserID) {
			WriteError(w, http.StatusForbidden, "forbidden", "not available for this role")
			return
		}
		var in struct {
			Active *bool `json:"active"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if in.Active == nil {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "active is required")
			return
		}
		if err := st.SetPaymentMethodActive(r.Context(), id, *in.Active); err != nil {
			if errors.Is(err, store.ErrPaymentMethod) {
				WriteError(w, http.StatusNotFound, "not_found", "payment method not found")
				return
			}
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not update the payment method")
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionPaymentMethodUpdate,
			TargetType: "payment_method", TargetID: strconv.FormatInt(id, 10),
			Context: map[string]any{"active": *in.Active},
		})
		updated, err := st.PaymentMethodByID(r.Context(), id)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the payment method")
			return
		}
		WriteJSON(w, http.StatusOK, updated)
	})))
}
