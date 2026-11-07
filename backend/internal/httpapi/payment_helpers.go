package httpapi

import (
	"errors"
	"net/http"

	"veylix/backend/internal/payments"
	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/store"
)

// errInvalidPathID marks a malformed `{id}` segment so the caller can answer
// 400 instead of the generic 500 used for unexpected failures.
var errInvalidPathID = errors.New("invalid path id")

// loadPayment reads `{id}` and enforces the caller's read scope.
//
// A payment that exists but belongs to somebody else answers **404**, not 403:
// a 403 would confirm that the id exists, which is an enumeration oracle.
func loadPayment(w http.ResponseWriter, r *http.Request, st PaymentStore, callerID int64, callerRole string) (*store.PaymentRow, bool) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_failed", "invalid payment id")
		return nil, false
	}
	pay, err := st.PaymentByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrPaymentNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "payment not found")
			return nil, false
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the payment")
		return nil, false
	}
	ok, err := st.CanSeePayment(r.Context(), pay, callerID, callerRole)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the payment")
		return nil, false
	}
	if !ok {
		WriteError(w, http.StatusNotFound, "not_found", "payment not found")
		return nil, false
	}
	return pay, true
}

// writeOrderError maps a D-031 settlement failure onto the API envelope, so an
// operator is told why the money was not taken instead of receiving a 500.
func writeOrderError(w http.ResponseWriter, err error) {
	writePaymentError(w, err, "could not complete the order")
}

// writePaymentError maps the store's typed errors onto the API envelope.
//
// The codes are stable on purpose: the UI shows a specific message for
// `illegal_transition` ("this payment was already reviewed") instead of the
// generic "something went wrong".
func writePaymentError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, payments.ErrNotConfirmable):
		WriteError(w, http.StatusConflict, "not_confirmable",
			"this order is not awaiting confirmation — it may already be settled")
	case errors.Is(err, payments.ErrNoPlan):
		WriteError(w, http.StatusUnprocessableEntity, "no_plan",
			"this order has no plan attached, so no VPN can be built from it")
	case errors.Is(err, store.ErrPaymentNotFound), errors.Is(err, store.ErrPaymentMethod):
		WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, store.ErrBadTransition):
		WriteError(w, http.StatusConflict, "illegal_transition",
			"this payment cannot move to that state from its current state")
	case errors.Is(err, store.ErrNotPayable):
		WriteError(w, http.StatusForbidden, "not_payable", "that sale does not belong to this account")
	case errors.Is(err, store.ErrPlanArchived):
		WriteError(w, http.StatusConflict, "plan_archived", "this plan has been archived")
	case errors.Is(err, store.ErrPlanExhausted), errors.Is(err, provisioning.ErrPlanExhausted):
		WriteError(w, http.StatusConflict, "plan_exhausted", "this plan has no slots left")
	case errors.Is(err, provisioning.ErrPanelFailed):
		WriteError(w, http.StatusBadGateway, "panel_error",
			"the panel did not accept the client, so the payment stays pending and nothing was charged")
	case errors.Is(err, store.ErrNoPayerAccount):
		// The dev bypass (D-022) runs as a synthetic principal that has no
		// users row, so a payment cannot be attributed. Say that plainly
		// instead of reporting an internal error.
		WriteError(w, http.StatusForbidden, "no_payer_account",
			"this session has no account to attribute a payment to; sign in with a real account")
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", fallback)
	}
}
