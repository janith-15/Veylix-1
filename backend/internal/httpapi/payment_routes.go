package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/payments"
	"veylix/backend/internal/store"
)

// PaymentStore is the persistence surface the payment handlers need.
type PaymentStore interface {
	PaymentMethodsVisible(ctx context.Context, ownerUserID int64) ([]store.PaymentMethodRow, error)
	PaymentMethodByID(ctx context.Context, id int64) (*store.PaymentMethodRow, error)
	CreatePaymentMethod(ctx context.Context, in store.CreatePaymentMethodInput) (*store.PaymentMethodRow, error)
	SetPaymentMethodActive(ctx context.Context, id int64, active bool) error

	PaymentsVisible(ctx context.Context, callerID int64, callerRole string, onlyPending bool) ([]store.PaymentRow, error)
	// PaymentsVisibleCount is the uncapped total the list is a window into, so a
	// screen can state "showing the newest 200 of 2,400" (D-116).
	PaymentsVisibleCount(ctx context.Context, callerID int64, callerRole string, onlyPending bool) (int, error)
	PaymentByID(ctx context.Context, id int64) (*store.PaymentRow, error)
	CanSeePayment(ctx context.Context, p *store.PaymentRow, callerID int64, callerRole string) (bool, error)
	CreatePayment(ctx context.Context, in store.CreatePaymentInput) (*store.PaymentRow, error)
	AttachPaymentReference(ctx context.Context, payerUserID, id int64, reference string) (*store.PaymentRow, error)
	ConfirmPayment(ctx context.Context, id, actorID int64) error
	RejectPayment(ctx context.Context, id, actorID int64) error
	CancelPayment(ctx context.Context, payerUserID, id int64) error
	RefundPayment(ctx context.Context, id, actorID int64) error
}

// OrderService is the D-031 settlement surface: one confirmation that also
// builds the VPN. Declared here so the transport depends on a behaviour, not on
// the concrete payments package.
type OrderService interface {
	Confirm(ctx context.Context, in payments.ConfirmInput) (*payments.OrderResult, error)
}

// PaymentRoutes registers billing (P6) and the D-031 settlement path.
//
// Design (D-007, REQUIREMENTS §2.5): payments are **manual**. The platform
// never contacts a gateway and never marks money as received on its own. The
// state machine is PENDING → AWAITING_CONFIRMATION → CONFIRMED | REJECTED, with
// CANCELLED (payer, before review) and REFUNDED (operator, after confirmation)
// as the two exits. Every transition is guarded in the store and audited here,
// so the trail can answer "who confirmed this, and when".
//
// `orders` is D-031: confirming an order that names a plan builds the VPN in the
// same click and answers with the links a QR needs. Nil keeps the route alive in
// settlement-only mode, used by tests that do not wire provisioning.
func PaymentRoutes(mux *http.ServeMux, st PaymentStore, orders OrderService, aud *audit.Logger, authed func(http.Handler) http.Handler) {
	record := func(ctx context.Context, e audit.Entry) {
		if aud == nil {
			return
		}
		_ = aud.Record(ctx, e)
	}
	paymentRoutes(mux, st, record, authed)
	paymentRecordRoutes(mux, st, record, authed)
	orderConfirmRoute(mux, st, orders, record, authed)
}

// orderConfirmRoute registers D-031: one click settles the money **and** builds
// the VPN, answering with the links a QR needs.
//
// It has exactly one owner so the confirm behaviour cannot drift between two
// registrations. A record with no plan keeps the settlement-only answer
// (a deposit), which is why this is not simply another entry in the loop.
func orderConfirmRoute(mux *http.ServeMux, st PaymentStore, orders OrderService,
	record func(context.Context, audit.Entry), authed func(http.Handler) http.Handler) {

	mux.Handle("POST /api/payments/{id}/confirm", authed(RequirePermission(auth.PermPaymentConfirm)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			o, readErr := st.PaymentByID(r.Context(), id)
			if readErr != nil {
				writePaymentError(w, readErr, "could not read the payment")
				return
			}
			planAttached := o.OfferID != nil && *o.OfferID > 0

			/*
			 * A payment that names a plan MUST go through the settlement service,
			 * because that is the only path that builds the VPN (D-031.1). When the
			 * service is not wired we refuse rather than fall through to the
			 * cash-only path below, which would mark a paid plan confirmed and
			 * deliver nothing.
			 */
			if planAttached {
				if orders == nil {
					WriteError(w, http.StatusServiceUnavailable, "orders_unavailable",
						"orders that name a plan can only be confirmed through the "+
							"provisioning service, which is not available")
					return
				}
				out, cerr := orders.Confirm(r.Context(), payments.ConfirmInput{
					PaymentID: id,
					Actor:     payments.Principal{UserID: p.UserID, Role: string(p.Role)},
				})
				if cerr != nil {
					writeOrderError(w, cerr)
					return
				}
				record(r.Context(), audit.Entry{
					ActorID: p.UserID, Action: audit.ActionPaymentConfirm,
					TargetType: "payment", TargetID: strconv.FormatInt(id, 10),
					Context: map[string]any{
						"status": store.PaymentConfirmed,
						"vpn_id": out.VPN.VPNID,
					},
				})
				WriteJSON(w, http.StatusOK, out)
				return
			}

			/*
			 * Settlement only: the record carries no plan, so there is nothing to
			 * provision. This is a deposit or a manual top-up, and it is reported as
			 * such — `vpn_id: 0` in the audit entry says no product was released, so
			 * a later reader cannot mistake it for a sale.
			 */
			if err := st.ConfirmPayment(r.Context(), id, p.UserID); err != nil {
				writePaymentError(w, err, "could not confirm the payment")
				return
			}
			record(r.Context(), audit.Entry{
				ActorID: p.UserID, Action: audit.ActionPaymentConfirm,
				TargetType: "payment", TargetID: strconv.FormatInt(id, 10),
				Context: map[string]any{"status": store.PaymentConfirmed, "vpn_id": 0},
			})
			pay, err := st.PaymentByID(r.Context(), id)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the payment")
				return
			}
			WriteJSON(w, http.StatusOK, pay)
		}))))
}
