package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// paymentRecordRoutes registers the payment records and their transitions.
func paymentRecordRoutes(mux *http.ServeMux, st PaymentStore, record func(context.Context, audit.Entry), authed func(http.Handler) http.Handler) {
	// GET /api/payments[?pending=1][?envelope=1] — role-scoped by the store.
	//
	// Without `envelope` the response is the bare array it has always been, so
	// every existing caller (badge counts, dashboards, stores) keeps working.
	// With `envelope=1` the caller additionally learns the UNCAPPED total, which
	// is the only way a screen can tell the difference between "there are 200
	// orders" and "there are 200 shown of 2,400" (D-116).
	mux.Handle("GET /api/payments", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		onlyPending := r.URL.Query().Get("pending") == "1"
		rows, err := st.PaymentsVisible(r.Context(), p.UserID, string(p.Role), onlyPending)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list payments")
			return
		}
		if r.URL.Query().Get("envelope") != "1" {
			WriteJSON(w, http.StatusOK, rows)
			return
		}
		total, err := st.PaymentsVisibleCount(r.Context(), p.UserID, string(p.Role), onlyPending)
		if err != nil {
			// The page itself is fine; only the count failed. Becoming 500 here
			// would hide readable orders behind a statistics problem, so the
			// count is reported as unknown rather than guessed (D-116).
			total = -1
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"payments": rows,
			"shown":    len(rows),
			"total":    total,
			// `total < 0` means "not counted", never "none".
			"truncated": total >= 0 && total > len(rows),
		})
	})))

	// GET /api/payments/{id}
	mux.Handle("GET /api/payments/{id}", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		pay, ok := loadPayment(w, r, st, p.UserID, string(p.Role))
		if !ok {
			return
		}
		WriteJSON(w, http.StatusOK, pay)
	})))

	// POST /api/payments — the payer records an intent to pay.
	mux.Handle("POST /api/payments", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		var in struct {
			AssignmentID int64  `json:"assignment_id"`
			MethodID     int64  `json:"method_id"`
			AmountCents  int64  `json:"amount_cents"`
			Reference    string `json:"reference"`
			// D-031.1: the plan being bought. When set, the server computes the
			// amount from the plan and ignores amount_cents, so a payer can never
			// price the product.
			OfferID int64 `json:"offer_id"`
			// TargetUserID is who the VPN is for (0 = the payer). A reseller or
			// the Super Admin buying for somebody else sets it; a client may only
			// ever buy for themselves.
			TargetUserID int64 `json:"target_user_id"`
			Quantity     int   `json:"quantity"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		// Money is integer cents: a fractional or absurd amount is a client
		// bug, not a rounding opportunity (new_update.md §6). An order that names
		// a plan does not need an amount at all: the plan supplies it.
		if in.OfferID <= 0 {
			if in.AmountCents <= 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
					"amount_cents must be a positive integer (or send offer_id so the plan prices it)")
				return
			}
			if in.AmountCents > 100_000_000 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "amount_cents is out of range")
				return
			}
		}
		// Only an operator may buy for another account. A client buying for
		// somebody else is an attempt to pay for a VPN they do not own.
		if in.TargetUserID > 0 && in.TargetUserID != p.UserID && p.IsClient() {
			WriteError(w, http.StatusForbidden, "forbidden", "a client may only order for their own account")
			return
		}
		if in.Quantity < 0 || in.Quantity > 1000 {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "quantity is out of range")
			return
		}
		pay, err := st.CreatePayment(r.Context(), store.CreatePaymentInput{
			PayerUserID: p.UserID, AssignmentID: in.AssignmentID, MethodID: in.MethodID,
			AmountCents: in.AmountCents, Reference: in.Reference,
			OfferID: in.OfferID, TargetUserID: in.TargetUserID, Quantity: in.Quantity,
		})
		if err != nil {
			writePaymentError(w, err, "could not create the payment")
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionPaymentCreate,
			TargetType: "payment", TargetID: strconv.FormatInt(pay.ID, 10),
			Context: map[string]any{"amount_cents": pay.AmountCents, "status": pay.Status},
		})
		WriteJSON(w, http.StatusCreated, pay)
	})))

	// POST /api/payments/{id}/reference — the payer reports the transfer ref.
	mux.Handle("POST /api/payments/{id}/reference", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid payment id")
			return
		}
		var in struct {
			Reference string `json:"reference"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		ref := strings.TrimSpace(in.Reference)
		if ref == "" {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "reference is required")
			return
		}
		pay, err := st.AttachPaymentReference(r.Context(), p.UserID, id, ref)
		if err != nil {
			writePaymentError(w, err, "could not attach the reference")
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionPaymentReference,
			TargetType: "payment", TargetID: strconv.FormatInt(id, 10),
			Context: map[string]any{"status": pay.Status},
		})
		WriteJSON(w, http.StatusOK, pay)
	})))

	// Operator transitions. All three require `payment:confirm`, which only the
	// super admin and a reseller hold — a client can never confirm their own
	// payment (REQUIREMENTS §4).
	review := func(r *http.Request, status, action string) error {
		p, _ := PrincipalFrom(r.Context())
		id, err := pathID(r)
		if err != nil {
			return errInvalidPathID
		}
		var e error
		switch status {
		case store.PaymentConfirmed:
			e = st.ConfirmPayment(r.Context(), id, p.UserID)
		case store.PaymentRejected:
			e = st.RejectPayment(r.Context(), id, p.UserID)
		default:
			e = st.RefundPayment(r.Context(), id, p.UserID)
		}
		if e != nil {
			return e
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: action,
			TargetType: "payment", TargetID: strconv.FormatInt(id, 10),
			Context: map[string]any{"status": status},
		})
		return nil
	}

	for _, t := range []struct{ path, status, action string }{
		{"/api/payments/{id}/reject", store.PaymentRejected, audit.ActionPaymentReject},
		{"/api/payments/{id}/refund", store.PaymentRefunded, audit.ActionPaymentRefund},
	} {
		path, status, action := t.path, t.status, t.action
		mux.Handle("POST "+path, authed(RequirePermission(auth.PermPaymentConfirm)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := review(r, status, action); err != nil {
					if err == errInvalidPathID {
						WriteError(w, http.StatusBadRequest, "validation_failed", "invalid payment id")
						return
					}
					writePaymentError(w, err, "could not update the payment")
					return
				}
				id, err := pathID(r)
				if err != nil {
					WriteError(w, http.StatusBadRequest, "validation_failed", "invalid payment id")
					return
				}
				pay, err := st.PaymentByID(r.Context(), id)
				if err != nil {
					WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the payment")
					return
				}
				WriteJSON(w, http.StatusOK, pay)
			}))))
	}

	// POST /api/payments/{id}/cancel — the payer withdraws, before review.
	mux.Handle("POST /api/payments/{id}/cancel", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid payment id")
			return
		}
		if err := st.CancelPayment(r.Context(), p.UserID, id); err != nil {
			writePaymentError(w, err, "could not cancel the payment")
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionPaymentCancel,
			TargetType: "payment", TargetID: strconv.FormatInt(id, 10),
		})
		pay, err := st.PaymentByID(r.Context(), id)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the payment")
			return
		}
		WriteJSON(w, http.StatusOK, pay)
	})))
}
