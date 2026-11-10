package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Payment statuses. The SQL CHECK constraint in 0002_business.sql allows
// exactly these values, so the Go constants and the schema must stay in step.
const (
	PaymentPending              = "PENDING"
	PaymentAwaitingConfirmation = "AWAITING_CONFIRMATION"
	PaymentConfirmed            = "CONFIRMED"
	PaymentRejected             = "REJECTED"
	PaymentCancelled            = "CANCELLED"
	PaymentRefundPending        = "REFUND_PENDING"
	PaymentRefunded             = "REFUNDED"
)

// Errors callers must distinguish, so the transport can answer 404/409 instead
// of a blanket 500.
var (
	ErrPaymentNotFound = errors.New("store: payment not found")
	ErrPaymentMethod   = errors.New("store: payment method not found")
	// ErrBadTransition means the requested status change is not legal from the
	// record's current status (e.g. confirming an already refunded payment).
	ErrBadTransition = errors.New("store: illegal payment transition")
	// ErrNotPayable means the caller may not pay for that assignment.
	ErrNotPayable = errors.New("store: assignment is not payable by this account")
	// ErrNoPayerAccount means the principal carries no users row. In dev-bypass
	// mode the principal is synthetic (D-022), so a write that must reference
	// users(id) cannot be attributed to anybody — answering 500 would blame the
	// server for a caller that is not really signed in.
	ErrNoPayerAccount = errors.New("store: principal has no account row")
)

// PaymentMethodRow is a bank/wallet destination the payer transfers to.
//
// `OwnerUserID` is a pointer because NULL means "global" — a platform-wide
// method — which is a different thing from "owned by user 0".
type PaymentMethodRow struct {
	ID           int64  `json:"id"`
	OwnerUserID  *int64 `json:"owner_user_id"`
	Kind         string `json:"kind"` // BANK | EZCASH | MOBILE | OTHER
	Label        string `json:"label"`
	Details      string `json:"details"`
	Instructions string `json:"instructions"`
	Active       bool   `json:"active"`
	CreatedAt    int64  `json:"created_at"`
	// Contact channels (D-036.2): where the payer sends proof of payment.
	// Structured rather than buried in `details`, so a screen can offer
	// "copy", "open WhatsApp" and "open Telegram" without parsing prose.
	ContactEmail    string `json:"contact_email,omitempty"`
	ContactWhatsApp string `json:"contact_whatsapp,omitempty"`
	ContactTelegram string `json:"contact_telegram,omitempty"`
}

// PaymentRow is one payment record.
//
// Currency is a fixed 3-letter code and amounts are integer cents, so money is
// never a float (`new_update.md` Â§6 — no silent rounding of money).
type PaymentRow struct {
	ID           int64  `json:"id"`
	AssignmentID *int64 `json:"assignment_id"`
	PayerUserID  int64  `json:"payer_user_id"`
	MethodID     *int64 `json:"method_id"`
	AmountCents  int64  `json:"amount_cents"`
	Currency     string `json:"currency"`
	Reference    string `json:"reference"`
	Status       string `json:"status"`
	ConfirmedBy  *int64 `json:"confirmed_by"`
	ConfirmedAt  *int64 `json:"confirmed_at"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	// OfferID is the plan this order buys (D-031.1). Nil keeps every manual
	// record written before plans had prices valid.
	OfferID *int64 `json:"offer_id"`
	// TargetUserID is the account the order is FOR. It equals PayerUserID for a
	// self-service order, and differs when a reseller or the Super Admin buys
	// for someone else.
	TargetUserID *int64 `json:"target_user_id"`
	// VPNInstanceID is what the confirmation produced, so the screen that
	// confirmed it can hand over a link and a QR (D-031.4).
	VPNInstanceID *int64 `json:"vpn_instance_id"`
	// Quantity is how many slots of the plan one order buys (D-030.5).
	Quantity int `json:"quantity"`
	// ProofChannel / ProofNote say where the payer sent proof of payment
	// (D-036.3) — a channel, never a copy of the slip.
	ProofChannel string `json:"proof_channel,omitempty"`
	ProofNote    string `json:"proof_note,omitempty"`

	// ---- what a CUSTOM order asks for (D-035.2, D-036.6) ------------------
	//
	// A custom order's limits are the customer's, so they live on the order and
	// are copied onto the VPN at release. All zero means "not a custom order".
	SpecGB      int64 `json:"spec_gb"`
	SpecDays    int64 `json:"spec_days"`
	SpecDevices int64 `json:"spec_devices"`
	SpecIPs     int64 `json:"spec_ips"`
	// ReleaseServerID / ReleaseInboundID are the panel the Super Admin picked
	// when releasing a custom order. Nil means "not chosen yet" — a legal,
	// visible state, and the release refuses to proceed without them.
	ReleaseServerID  *int64 `json:"release_server_id"`
	ReleaseInboundID *int64 `json:"release_inbound_id"`
	// ConfirmerUserID is who settles this order (D-036.1): 0 means the platform
	// (the Super Admin); a reseller id means that reseller's own package.
	ConfirmerUserID int64 `json:"confirmer_user_id"`

	// PayerEmail is WHO PLACED THE ORDER (D-113). The row used to carry only
	// `payer_user_id`, so an operator holding a queue of orders could not tell
	// whose money it was without a second lookup — the one thing they need.
	// Empty means the account no longer exists; the UI says so rather than
	// showing a blank.
	PayerEmail string `json:"payer_email"`
	// TargetEmail is the account the order is FOR. It differs from PayerEmail
	// when a reseller or the Super Admin bought on someone's behalf, which is
	// exactly the case where naming only the payer would mislead.
	TargetEmail string `json:"target_email"`
	// OfferName / OfferKind name the package being bought. Empty for a manual
	// record or when the plan was deleted — an absence the UI states plainly.
	OfferName string `json:"offer_name"`
	OfferKind string `json:"offer_kind"`
}

// IsCustomSpec reports whether the order carries a customer-authored spec.
func (p PaymentRow) IsCustomSpec() bool {
	return p.SpecGB > 0 || p.SpecDays > 0 || p.SpecDevices > 0 || p.SpecIPs > 0
}

const paymentColumns = `p.id, p.assignment_id, p.payer_user_id, p.method_id, p.amount_cents,
	p.currency, p.reference, p.status, p.confirmed_by, p.confirmed_at, p.created_at, p.updated_at,
	p.offer_id, p.target_user_id, p.vpn_instance_id, p.quantity,
	p.proof_channel, p.proof_note,
	p.spec_gb, p.spec_days, p.spec_devices, p.spec_ips,
	p.release_server_id, p.release_inbound_id,
	(SELECT a.seller_user_id FROM vpn_assignments a WHERE a.id = p.assignment_id),
	(SELECT u.email FROM users u WHERE u.id = p.payer_user_id),
	(SELECT u.email FROM users u WHERE u.id = p.target_user_id),
	(SELECT o.name FROM vpn_offers o WHERE o.id = p.offer_id),
	(SELECT o.kind FROM vpn_offers o WHERE o.id = p.offer_id)`

func scanPayment(sc interface{ Scan(...any) error }) (*PaymentRow, error) {
	var r PaymentRow
	var assignment, method, confirmedBy, confirmedAt sql.NullInt64
	var offer, target, vpn sql.NullInt64
	var relServer, relInbound, confirmer sql.NullInt64
	// The four identifying columns (D-113) are LEFT-join shaped: a manual record
	// has no offer, and an order whose account was deleted has no email. They
	// scan as NullString so a missing value becomes "" instead of failing the
	// whole list.
	var payerEmail, targetEmail, offerName, offerKind sql.NullString
	if err := sc.Scan(&r.ID, &assignment, &r.PayerUserID, &method, &r.AmountCents,
		&r.Currency, &r.Reference, &r.Status, &confirmedBy, &confirmedAt,
		&r.CreatedAt, &r.UpdatedAt, &offer, &target, &vpn, &r.Quantity,
		&r.ProofChannel, &r.ProofNote,
		&r.SpecGB, &r.SpecDays, &r.SpecDevices, &r.SpecIPs,
		&relServer, &relInbound, &confirmer,
		&payerEmail, &targetEmail, &offerName, &offerKind); err != nil {
		return nil, err
	}
	if assignment.Valid {
		v := assignment.Int64
		r.AssignmentID = &v
	}
	// D-113: NullString.String is "" when NULL, which is exactly the "absent"
	// value the UI is told to state plainly rather than render as a blank.
	r.PayerEmail = payerEmail.String
	r.TargetEmail = targetEmail.String
	r.OfferName = offerName.String
	r.OfferKind = offerKind.String
	if method.Valid {
		v := method.Int64
		r.MethodID = &v
	}
	if confirmedBy.Valid {
		v := confirmedBy.Int64
		r.ConfirmedBy = &v
	}
	if confirmedAt.Valid {
		v := confirmedAt.Int64
		r.ConfirmedAt = &v
	}
	if offer.Valid {
		v := offer.Int64
		r.OfferID = &v
	}
	if target.Valid {
		v := target.Int64
		r.TargetUserID = &v
	}
	if vpn.Valid {
		v := vpn.Int64
		r.VPNInstanceID = &v
	}
	if relServer.Valid {
		v := relServer.Int64
		r.ReleaseServerID = &v
	}
	if relInbound.Valid {
		v := relInbound.Int64
		r.ReleaseInboundID = &v
	}
	if confirmer.Valid {
		r.ConfirmerUserID = confirmer.Int64
	}
	return &r, nil
}

// PaymentMethodsVisible returns the methods a payer may transfer to: active
// global methods plus the active methods owned by `ownerUserID`.
//
// A method owned by somebody else is never listed, because the payer would be
// sending money to an account that cannot confirm the transfer.
func (s *Store) PaymentMethodsVisible(ctx context.Context, ownerUserID int64) ([]PaymentMethodRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, owner_user_id, kind, label, details, instructions, active, created_at,
		        contact_email, contact_whatsapp, contact_telegram
		   FROM payment_methods
		  WHERE active = 1 AND (owner_user_id IS NULL OR owner_user_id = ?)
		  ORDER BY (owner_user_id IS NULL), kind, label`, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("store: list payment methods: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]PaymentMethodRow, 0, 8)
	for rows.Next() {
		var m PaymentMethodRow
		var owner sql.NullInt64
		var active int
		if err := rows.Scan(&m.ID, &owner, &m.Kind, &m.Label, &m.Details,
			&m.Instructions, &active, &m.CreatedAt,
			&m.ContactEmail, &m.ContactWhatsApp, &m.ContactTelegram); err != nil {
			return nil, fmt.Errorf("store: scan payment method: %w", err)
		}
		if owner.Valid {
			v := owner.Int64
			m.OwnerUserID = &v
		}
		m.Active = active == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

// PaymentMethodByID reads one method.
func (s *Store) PaymentMethodByID(ctx context.Context, id int64) (*PaymentMethodRow, error) {
	var m PaymentMethodRow
	var owner sql.NullInt64
	var active int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, owner_user_id, kind, label, details, instructions, active, created_at,
		        contact_email, contact_whatsapp, contact_telegram
		   FROM payment_methods WHERE id = ?`, id).
		Scan(&m.ID, &owner, &m.Kind, &m.Label, &m.Details, &m.Instructions, &active, &m.CreatedAt,
			&m.ContactEmail, &m.ContactWhatsApp, &m.ContactTelegram)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPaymentMethod
	}
	if err != nil {
		return nil, fmt.Errorf("store: get payment method: %w", err)
	}
	if owner.Valid {
		v := owner.Int64
		m.OwnerUserID = &v
	}
	m.Active = active == 1
	return &m, nil
}

// CreatePaymentMethodInput is the validated create payload. `OwnerUserID` 0
// means a global method (super admin only — enforced by the transport).
type CreatePaymentMethodInput struct {
	OwnerUserID  int64
	Kind         string
	Label        string
	Details      string
	Instructions string
	// Contact channels (D-036.2): where the payer sends proof of payment.
	ContactEmail    string
	ContactWhatsApp string
	ContactTelegram string
}

// CreatePaymentMethod inserts a destination and returns it.
func (s *Store) CreatePaymentMethod(ctx context.Context, in CreatePaymentMethodInput) (*PaymentMethodRow, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO payment_methods
		   (owner_user_id, kind, label, details, instructions, active, created_at,
		    contact_email, contact_whatsapp, contact_telegram)
		 VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, ?)`,
		nullableID(in.OwnerUserID), in.Kind, in.Label, in.Details, in.Instructions, nowUnix(),
		strings.TrimSpace(in.ContactEmail), strings.TrimSpace(in.ContactWhatsApp),
		strings.TrimSpace(in.ContactTelegram))
	if err != nil {
		return nil, fmt.Errorf("store: create payment method: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.PaymentMethodByID(ctx, id)
}

// SetPaymentMethodActive activates or retires a destination.
//
// Methods are never deleted: a confirmed payment references the destination it
// was made to, and that record must survive (REQUIREMENTS Â§3.3).
func (s *Store) SetPaymentMethodActive(ctx context.Context, id int64, active bool) error {
	v := 0
	if active {
		v = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE payment_methods SET active = ? WHERE id = ?`, v, id)
	if err != nil {
		return fmt.Errorf("store: set payment method active: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPaymentMethod
	}
	return nil
}

// PaymentListLimit caps how many order records one list call returns.
//
// It is a ceiling on work, not on truth: the caller is told the real total
// alongside the capped page, so a screen can say "showing the newest 200 of
// 2,400" instead of silently implying there are 200 orders (D-116).
const PaymentListLimit = 200

// paymentsScope builds the WHERE clause and args shared by the list and its
// count, so the two can never disagree about WHICH rows a caller may see.
//
// Kept as one function on purpose: a count that filtered differently from the
// list would produce the worst possible answer — "there are 3 more" pointing at
// rows the caller cannot read, or a hidden set they never knew about.
func paymentsScope(callerID int64, callerRole string, onlyPending bool) (string, []any) {
	args := make([]any, 0, 2)
	where := make([]string, 0, 3)

	switch callerRole {
	case "super_admin":
		// no ownership filter
	case "admin":
		where = append(where, `(p.payer_user_id = ? OR EXISTS (
			SELECT 1 FROM vpn_assignments a
			 WHERE a.id = p.assignment_id AND a.seller_user_id = ?))`)
		args = append(args, callerID, callerID)
	default: // client
		where = append(where, `p.payer_user_id = ?`)
		args = append(args, callerID)
	}
	if onlyPending {
		where = append(where, `p.status IN ('PENDING', 'AWAITING_CONFIRMATION')`)
	}
	if len(where) == 0 {
		return "", args
	}
	return ` WHERE ` + strings.Join(where, " AND "), args
}

// PaymentsVisibleCount counts every row the caller may see, ignoring the list
// cap. Used together with PaymentsVisible so a screen can state how many orders
// exist beyond the newest page (D-116).
func (s *Store) PaymentsVisibleCount(ctx context.Context, callerID int64, callerRole string, onlyPending bool) (int, error) {
	scope, args := paymentsScope(callerID, callerRole, onlyPending)
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM payment_records p`+scope, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count payments: %w", err)
	}
	return n, nil
}

// PaymentsVisible returns the payment records a caller may read.
//
//   - super admin: every record;
//   - admin:       records they paid plus records for assignments they sold;
//   - client:      only their own payments.
//
// `onlyPending` narrows the operator views to the confirmation queue.
//
// The result is CAPPED at PaymentListLimit rows, newest first. A caller that
// needs to know whether older orders exist asks PaymentsVisibleCount — the cap
// used to be silent, so an order list could look complete while showing the
// newest 200 of thousands (D-116).
func (s *Store) PaymentsVisible(ctx context.Context, callerID int64, callerRole string, onlyPending bool) ([]PaymentRow, error) {
	scope, args := paymentsScope(callerID, callerRole, onlyPending)
	query := `SELECT ` + paymentColumns + ` FROM payment_records p` + scope +
		` ORDER BY p.id DESC LIMIT ` + strconv.Itoa(PaymentListLimit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list payments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]PaymentRow, 0, 16)
	for rows.Next() {
		r, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan payment: %w", err)
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// PaymentByID reads one record without any scope check. Handlers that serve a
// payer must call CanSeePayment first (the rule lives next to the query that
// implements it, not in the UI).
func (s *Store) PaymentByID(ctx context.Context, id int64) (*PaymentRow, error) {
	r, err := scanPayment(s.db.QueryRowContext(ctx,
		`SELECT `+paymentColumns+` FROM payment_records p WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPaymentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get payment: %w", err)
	}
	return r, nil
}

// CanSeePayment reports whether `callerID` may read the record `p`.
//
// It mirrors PaymentsVisible exactly, so a list and a detail view can never
// disagree about who owns what.
func (s *Store) CanSeePayment(ctx context.Context, p *PaymentRow, callerID int64, callerRole string) (bool, error) {
	switch callerRole {
	case "super_admin":
		return true, nil
	case "admin":
		if p.PayerUserID == callerID {
			return true, nil
		}
		if p.AssignmentID == nil {
			return false, nil
		}
		var n int
		err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM vpn_assignments WHERE id = ? AND seller_user_id = ?`,
			*p.AssignmentID, callerID).Scan(&n)
		if err != nil {
			return false, fmt.Errorf("store: check payment scope: %w", err)
		}
		return n > 0, nil
	default:
		return p.PayerUserID == callerID, nil
	}
}

// CreatePaymentInput is the payer's payload.
type CreatePaymentInput struct {
	PayerUserID  int64
	AssignmentID int64 // 0 = no assignment (a deposit / manual purchase)
	MethodID     int64 // 0 = no method selected yet
	AmountCents  int64
	Reference    string
	// OfferID is the plan being bought (D-031.1). When set, the amount is
	// resolved from the plan and AmountCents is ignored: the payer never prices
	// the product.
	OfferID int64
	// TargetUserID is the account the VPN is for. 0 means "the payer".
	TargetUserID int64
	// Quantity buys N slots of a plan. 0/1 buys one.
	Quantity int
	// Currency overrides the plan's currency, for a plan created before prices
	// existed. Ignored when the plan states one.
	Currency string
}

// PaymentAmountFromPlan resolves what an order costs, server-side (D-031.1).
//
// A plan with no price recorded (price_cents = 0, every pre-0008 row) cannot be
// priced here, so the caller must supply the amount — that path keeps the old
// manual behaviour rather than silently selling a paid plan for zero.
func PaymentAmountFromPlan(p *Plan, quantity int) (cents int64, currency string, priced bool) {
	if quantity <= 0 {
		quantity = 1
	}
	if p == nil || p.PriceCents <= 0 {
		return 0, "", false
	}
	return p.PriceCents * int64(quantity), p.Currency, true
}

// CreatePayment records an intent to pay.
//
// Payments are **manual** (REQUIREMENTS Â§2.5, D-007): the platform never talks
// to a gateway. A new record starts at PENDING (money not yet sent) and moves
// to AWAITING_CONFIRMATION when the payer supplies a transfer reference. Only an
// operator may then confirm or reject it — and, since D-031, confirming it is
// what builds the VPN.
//
// When an assignment is given it must belong to the payer, so a client cannot
// attach a payment to somebody else's sale.
func (s *Store) CreatePayment(ctx context.Context, in CreatePaymentInput) (*PaymentRow, error) {
	if in.Quantity <= 0 {
		in.Quantity = 1
	}
	if in.AmountCents <= 0 && in.OfferID <= 0 {
		return nil, fmt.Errorf("store: create payment: amount must be positive")
	}
	var id int64
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		// The payer must be a real account: payment_records.payer_user_id is a
		// foreign key, and "who paid" is not a field that may be invented.
		var exists int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE id = ?`, in.PayerUserID).Scan(&exists); err != nil {
			return fmt.Errorf("store: lookup payer: %w", err)
		}
		if exists == 0 {
			return ErrNoPayerAccount
		}
		if in.AssignmentID > 0 {
			var owner int64
			err := tx.QueryRowContext(ctx,
				`SELECT client_user_id FROM vpn_assignments WHERE id = ?`, in.AssignmentID).Scan(&owner)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotPayable
			}
			if err != nil {
				return fmt.Errorf("store: lookup assignment: %w", err)
			}
			if owner != in.PayerUserID {
				return ErrNotPayable
			}
		}
		if in.MethodID > 0 {
			var active int
			err := tx.QueryRowContext(ctx,
				`SELECT active FROM payment_methods WHERE id = ?`, in.MethodID).Scan(&active)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPaymentMethod
			}
			if err != nil {
				return fmt.Errorf("store: lookup method: %w", err)
			}
			if active != 1 {
				return ErrPaymentMethod
			}
		}

		// The plan decides the money (D-031.1): a caller-supplied amount is only
		// honoured for a plan that carries no price at all.
		amount := in.AmountCents
		currency := in.Currency
		if currency == "" {
			currency = "LKR"
		}
		if in.OfferID > 0 {
			plan, err := s.PlanByIDTx(ctx, tx, in.OfferID)
			if err != nil {
				return err
			}
			if plan.Archived {
				return ErrPlanArchived
			}
			if cents, planCurrency, priced := PaymentAmountFromPlan(plan, in.Quantity); priced {
				amount = cents
				currency = planCurrency
			} else if amount <= 0 {
				// No price on the plan and no amount given: refuse rather than
				// create a zero-value order for a product.
				return fmt.Errorf("store: create payment: this plan has no price; an amount is required")
			}
		}
		if amount <= 0 {
			return fmt.Errorf("store: create payment: amount must be positive")
		}

		target := in.TargetUserID
		if target <= 0 {
			target = in.PayerUserID
		}

		now := nowUnix()
		// A reference means the payer has actually made the transfer.
		status := PaymentPending
		if strings.TrimSpace(in.Reference) != "" {
			status = PaymentAwaitingConfirmation
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO payment_records
			   (assignment_id, payer_user_id, method_id, amount_cents, currency, reference,
			    status, confirmed_by, confirmed_at, offer_id, target_user_id, quantity,
			    created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?, ?, ?, ?)`,
			nullableID(in.AssignmentID), in.PayerUserID, nullableID(in.MethodID),
			amount, currency, strings.TrimSpace(in.Reference), status,
			nullableID(in.OfferID), target, in.Quantity, now, now)
		if err != nil {
			return fmt.Errorf("store: insert payment: %w", err)
		}
		id, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.PaymentByID(ctx, id)
}

// CreatePayment records an intent to pay.

// AttachPaymentReference moves a PENDING record to AWAITING_CONFIRMATION.
//
// Only the payer may do it, and only from PENDING: re-submitting a reference on
// an already reviewed payment would let a rejected payment jump the queue.
func (s *Store) AttachPaymentReference(ctx context.Context, payerUserID, id int64, reference string) (*PaymentRow, error) {
	ref := strings.TrimSpace(reference)
	if ref == "" {
		return nil, fmt.Errorf("store: attach reference: reference is required")
	}
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		var payer int64
		var status string
		err := tx.QueryRowContext(ctx,
			`SELECT payer_user_id, status FROM payment_records WHERE id = ?`, id).Scan(&payer, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lookup payment: %w", err)
		}
		if payer != payerUserID {
			// The same error as "missing": a payer must not be able to probe
			// for other people's payment ids.
			return ErrPaymentNotFound
		}
		if status != PaymentPending {
			return ErrBadTransition
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_records SET reference = ?, status = ?, updated_at = ? WHERE id = ?`,
			ref, PaymentAwaitingConfirmation, nowUnix(), id); err != nil {
			return fmt.Errorf("store: attach reference: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.PaymentByID(ctx, id)
}

// transitionPayment is the single guarded state change. Every action goes
// through it, so an illegal jump (e.g. CONFIRMED → CONFIRMED) is refused with
// ErrBadTransition instead of silently rewriting a reviewed record.
func (s *Store) transitionPayment(ctx context.Context, id int64, from []string, to string, actorID int64, stampReviewed bool) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		var status string
		err := tx.QueryRowContext(ctx, `SELECT status FROM payment_records WHERE id = ?`, id).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lookup payment: %w", err)
		}
		allowed := false
		for _, f := range from {
			if status == f {
				allowed = true
				break
			}
		}
		if !allowed {
			return ErrBadTransition
		}
		now := nowUnix()
		if stampReviewed {
			_, err = tx.ExecContext(ctx,
				`UPDATE payment_records SET status = ?, confirmed_by = ?, confirmed_at = ?, updated_at = ?
				  WHERE id = ?`, to, actorID, now, now, id)
		} else {
			_, err = tx.ExecContext(ctx,
				`UPDATE payment_records SET status = ?, updated_at = ? WHERE id = ?`, to, now, id)
		}
		if err != nil {
			return fmt.Errorf("store: update payment status: %w", err)
		}
		return nil
	})
}

// ConfirmPayment marks a payment as received.
//
// Legal from PENDING too: an operator may confirm a cash payment whose
// reference the payer never typed. The reviewer is always recorded.
//
// This is the settlement-only form, kept for a manual record that carries no
// plan (a deposit). An order that names a plan must go through CompletePayment,
// which also records the VPN it produced (D-031.3).
func (s *Store) ConfirmPayment(ctx context.Context, id, actorID int64) error {
	return s.transitionPayment(ctx, id,
		[]string{PaymentPending, PaymentAwaitingConfirmation}, PaymentConfirmed, actorID, true)
}

// CompletePayment marks an order confirmed **and** records the VPN it produced,
// in one transaction (D-031.3).
//
// Both facts move together on purpose: a payment that says CONFIRMED while
// naming no VPN is exactly the defect this closes (M6-01), and a VPN whose
// payment is not confirmed is money nobody accounted for. If the row is no
// longer in a confirmable state the whole thing is refused, so a double click
// cannot confirm twice.
func (s *Store) CompletePayment(ctx context.Context, id, actorID, vpnID int64) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		var status string
		err := tx.QueryRowContext(ctx, `SELECT status FROM payment_records WHERE id = ?`, id).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lookup payment: %w", err)
		}
		if status != PaymentPending && status != PaymentAwaitingConfirmation {
			return ErrBadTransition
		}
		now := nowUnix()
		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_records
			    SET status = ?, confirmed_by = ?, confirmed_at = ?, vpn_instance_id = ?, updated_at = ?
			  WHERE id = ? AND status IN (?, ?)`,
			PaymentConfirmed, actorID, now, nullableID(vpnID), now, id,
			PaymentPending, PaymentAwaitingConfirmation); err != nil {
			return fmt.Errorf("store: complete payment: %w", err)
		}
		return nil
	})
}

// RejectPayment refuses a submitted payment.
func (s *Store) RejectPayment(ctx context.Context, id, actorID int64) error {
	return s.transitionPayment(ctx, id,
		[]string{PaymentPending, PaymentAwaitingConfirmation}, PaymentRejected, actorID, true)
}

// CancelPayment is the payer withdrawing their own payment before any review.
func (s *Store) CancelPayment(ctx context.Context, payerUserID, id int64) error {
	var owner int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT payer_user_id FROM payment_records WHERE id = ?`, id).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentNotFound
		}
		return fmt.Errorf("store: lookup payment: %w", err)
	}
	if owner != payerUserID {
		return ErrPaymentNotFound
	}
	return s.transitionPayment(ctx, id, []string{PaymentPending}, PaymentCancelled, payerUserID, false)
}

// RefundPayment refunds a confirmed payment.
func (s *Store) RefundPayment(ctx context.Context, id, actorID int64) error {
	return s.transitionPayment(ctx, id, []string{PaymentConfirmed}, PaymentRefunded, actorID, true)
}
