package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Order events (D-036).
//
// The current payment row says what is true NOW; this table says how it got
// there. "Order eka demme kauda, eyata payment enne kohenda kagenda" is a
// question about history, so it is answered from history — a mutable row cannot
// answer it once the payer changes destination.

// Event kinds. Kept as constants so the CHECK constraint and the Go code cannot
// drift apart.
const (
	EventCreated       = "CREATED"
	EventMethodChanged = "METHOD_CHANGED"
	EventProofChannel  = "PROOF_CHANNEL"
	EventBlocked       = "BLOCKED"
	EventUnblocked     = "UNBLOCKED"
	EventConfirmed     = "CONFIRMED"
	EventRejected      = "REJECTED"
	// EventReleaseTarget records the panel a Super Admin chose for a CUSTOM
	// order, before it is released (D-036.6).
	EventReleaseTarget = "RELEASE_TARGET"
)

// Proof channels the payer may use (D-036.3).
const (
	ProofWhatsApp = "WHATSAPP"
	ProofTelegram = "TELEGRAM"
	ProofEmail    = "EMAIL"
)

// ErrBadChannel means the payer named a proof channel the product does not
// accept. A typo must not be stored as a channel nobody watches.
var ErrBadChannel = errors.New("store: unknown proof channel")

// OrderEvent is one line of an order's history.
type OrderEvent struct {
	ID           int64  `json:"id"`
	PaymentID    int64  `json:"payment_id"`
	Kind         string `json:"kind"`
	FromMethodID *int64 `json:"from_method_id"`
	ToMethodID   *int64 `json:"to_method_id"`
	// The destination labels as they read at the time, so the history stays
	// readable even after a method is renamed or retired.
	FromLabel string `json:"from_label,omitempty"`
	ToLabel   string `json:"to_label,omitempty"`
	Channel   string `json:"channel,omitempty"`
	ActorID   *int64 `json:"actor_id"`
	ActorMail string `json:"actor_email,omitempty"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"created_at"`
}

// AppendOrderEvent records one event. It runs inside the caller's transaction
// whenever there is one, so an event can never describe a change that rolled
// back.
func (s *Store) AppendOrderEvent(ctx context.Context, tx *sql.Tx, e OrderEvent) error {
	exec := s.execer(tx)
	res, err := exec.ExecContext(ctx,
		`INSERT INTO payment_record_events
		   (payment_id, kind, from_method_id, to_method_id, channel, actor_id, note, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.PaymentID, e.Kind, nullableIDPtr(e.FromMethodID), nullableIDPtr(e.ToMethodID),
		strings.ToUpper(strings.TrimSpace(e.Channel)), nullableIDPtr(e.ActorID),
		strings.TrimSpace(e.Note), time.Now().Unix())
	if err != nil {
		return fmt.Errorf("store: append order event: %w", err)
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

// nullableIDPtr maps nil to SQL NULL and a value to that value.
func nullableIDPtr(v *int64) any {
	if v == nil || *v <= 0 {
		return nil
	}
	return *v
}

// OrderEvents returns one order's history, oldest first.
func (s *Store) OrderEvents(ctx context.Context, paymentID int64) ([]OrderEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT e.id, e.payment_id, e.kind, e.from_method_id, e.to_method_id,
		        COALESCE(mf.label, ''), COALESCE(mt.label, ''),
		        e.channel, e.actor_id, COALESCE(u.email, ''), e.note, e.created_at
		   FROM payment_record_events e
		   LEFT JOIN payment_methods mf ON mf.id = e.from_method_id
		   LEFT JOIN payment_methods mt ON mt.id = e.to_method_id
		   LEFT JOIN users u            ON u.id  = e.actor_id
		  WHERE e.payment_id = ?
		  ORDER BY e.created_at ASC, e.id ASC`, paymentID)
	if err != nil {
		return nil, fmt.Errorf("store: list order events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]OrderEvent, 0, 8)
	for rows.Next() {
		var (
			e        OrderEvent
			from, to sql.NullInt64
			actor    sql.NullInt64
		)
		if err := rows.Scan(&e.ID, &e.PaymentID, &e.Kind, &from, &to,
			&e.FromLabel, &e.ToLabel, &e.Channel, &actor, &e.ActorMail, &e.Note, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan order event: %w", err)
		}
		if from.Valid {
			v := from.Int64
			e.FromMethodID = &v
		}
		if to.Valid {
			v := to.Int64
			e.ToMethodID = &v
		}
		if actor.Valid {
			v := actor.Int64
			e.ActorID = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ChangePaymentMethod moves an order to a different destination (D-036.4).
//
// Only the payer may do it, and only while the order is still awaiting a
// decision: moving a settled order would rewrite what the money was sent to.
// The old destination is kept in the event, so "he first chose eZ Cash and then
// moved to a bank transfer" survives.
func (s *Store) ChangePaymentMethod(ctx context.Context, payerUserID, paymentID, methodID int64) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		var (
			status string
			payer  int64
			from   sql.NullInt64
		)
		err := tx.QueryRowContext(ctx,
			`SELECT status, payer_user_id, method_id FROM payment_records WHERE id = ?`,
			paymentID).Scan(&status, &payer, &from)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lookup payment: %w", err)
		}
		if payer != payerUserID {
			// Not the payer: answer as if it does not exist, so ids cannot be
			// probed (the transport maps this to 404).
			return ErrPaymentNotFound
		}
		if status != PaymentPending && status != PaymentAwaitingConfirmation {
			return ErrBadTransition
		}

		// The destination must be a live method the payer may use.
		var active int
		err = tx.QueryRowContext(ctx,
			`SELECT active FROM payment_methods
			  WHERE id = ? AND (owner_user_id IS NULL OR owner_user_id = ?)`,
			methodID, payerUserID).Scan(&active)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentMethod
		}
		if err != nil {
			return fmt.Errorf("store: lookup method: %w", err)
		}
		if active != 1 {
			return ErrPaymentMethod
		}
		if from.Valid && from.Int64 == methodID {
			// Re-choosing the same destination is a no-op, not an event: a
			// timeline full of "changed to the same thing" is noise.
			return nil
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_records SET method_id = ?, updated_at = ? WHERE id = ?`,
			methodID, nowUnix(), paymentID); err != nil {
			return fmt.Errorf("store: change method: %w", err)
		}

		var fromID *int64
		if from.Valid {
			v := from.Int64
			fromID = &v
		}
		toID := methodID
		return s.AppendOrderEvent(ctx, tx, OrderEvent{
			PaymentID: paymentID, Kind: EventMethodChanged,
			FromMethodID: fromID, ToMethodID: &toID, ActorID: &payerUserID,
		})
	})
}

// RecordProofChannel says where the payer sent proof of payment (D-036.3).
//
// A channel is not a file: Veylix stores "WhatsApp, reference 4471" so the
// operator knows where to look, never a copy of the customer's slip. Recording
// it moves the order to AWAITING_CONFIRMATION, because proof means the money
// has left the payer.
func (s *Store) RecordProofChannel(ctx context.Context, payerUserID, paymentID int64, channel, note string) error {
	ch := strings.ToUpper(strings.TrimSpace(channel))
	switch ch {
	case ProofWhatsApp, ProofTelegram, ProofEmail:
	default:
		return ErrBadChannel
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		var (
			status string
			payer  int64
		)
		err := tx.QueryRowContext(ctx,
			`SELECT status, payer_user_id FROM payment_records WHERE id = ?`, paymentID).Scan(&status, &payer)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lookup payment: %w", err)
		}
		if payer != payerUserID {
			return ErrPaymentNotFound
		}
		if status != PaymentPending && status != PaymentAwaitingConfirmation {
			return ErrBadTransition
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_records
			    SET proof_channel = ?, proof_note = ?, status = ?, updated_at = ?
			  WHERE id = ?`,
			ch, strings.TrimSpace(note), PaymentAwaitingConfirmation, nowUnix(), paymentID); err != nil {
			return fmt.Errorf("store: record proof: %w", err)
		}
		return s.AppendOrderEvent(ctx, tx, OrderEvent{
			PaymentID: paymentID, Kind: EventProofChannel, Channel: ch,
			ActorID: &payerUserID, Note: strings.TrimSpace(note),
		})
	})
}

// BlockOrderVPN stops the VPN an order produced, because the money did not
// arrive or does not match (D-036.5).
//
// This is a LOCAL state change plus one event; the panel is stopped by the
// caller through the provisioning service, which is the only component allowed
// to talk to a panel. Recording it first means an operator never sees "blocked"
// with no reason attached.
func (s *Store) BlockOrderVPN(ctx context.Context, orderID, actorID int64, note string) error {
	return s.setBlocked(ctx, orderID, actorID, note, true)
}

// UnblockOrderVPN lifts a block once the issue is resolved.
func (s *Store) UnblockOrderVPN(ctx context.Context, orderID, actorID int64, note string) error {
	return s.setBlocked(ctx, orderID, actorID, note, false)
}

// setBlocked records the block/unblock decision and its event.
func (s *Store) setBlocked(ctx context.Context, orderID, actorID int64, note string, block bool) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		var vpnID sql.NullInt64
		err := tx.QueryRowContext(ctx,
			`SELECT vpn_instance_id FROM payment_records WHERE id = ?`, orderID).Scan(&vpnID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lookup order: %w", err)
		}
		if !vpnID.Valid {
			// There is no VPN to block yet: the order has not been settled.
			// Refusing is honest — a block on nothing is not a block.
			return ErrNoVPNToBlock
		}

		target := "ACTIVE"
		kind := EventUnblocked
		if block {
			target = "SUSPENDED"
			kind = EventBlocked
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE vpn_instances SET status = ?, updated_at = ? WHERE id = ?`,
			target, nowUnix(), vpnID.Int64); err != nil {
			return fmt.Errorf("store: %s vpn: %w", strings.ToLower(kind), err)
		}
		return s.AppendOrderEvent(ctx, tx, OrderEvent{
			PaymentID: orderID, Kind: kind, ActorID: &actorID, Note: strings.TrimSpace(note),
		})
	})
}

// ErrNoVPNToBlock means the order has not produced a VPN yet, so there is
// nothing to stop.
var ErrNoVPNToBlock = errors.New("store: this order has no VPN to block yet")

// SetReleaseTarget records the panel a Super Admin chose for a CUSTOM order
// (D-036.6).
//
// Kept separate from the confirmation on purpose: choosing where a VPN will live
// is a decision an operator can make before the money arrives, and the release
// then reads it. A custom order with no target is a legal state — it simply
// cannot be released yet.
func (s *Store) SetReleaseTarget(ctx context.Context, orderID, actorID, serverID, inboundID int64) error {
	if serverID <= 0 || inboundID <= 0 {
		return ErrReleaseTargetRequired
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		var (
			status string
			plane  sql.NullInt64
		)
		err := tx.QueryRowContext(ctx,
			`SELECT status, vpn_instance_id FROM payment_records WHERE id = ?`, orderID).Scan(&status, &plane)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPaymentNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lookup order: %w", err)
		}
		if plane.Valid {
			// Already released: moving the target afterwards would describe a
			// VPN that lives somewhere else.
			return ErrBadTransition
		}
		if status != PaymentPending && status != PaymentAwaitingConfirmation {
			return ErrBadTransition
		}
		// Both must exist, and the inbound must belong to that server: a
		// mismatch would attach a client to an inbound on a different panel.
		var serverExists int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM server_integrations WHERE id = ?`, serverID).Scan(&serverExists); err != nil {
			return fmt.Errorf("store: lookup server: %w", err)
		}
		if serverExists == 0 {
			return ErrReleaseTargetRequired
		}
		var inboundBelongs int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM inbounds WHERE id = ? AND server_id = ?`,
			inboundID, serverID).Scan(&inboundBelongs); err != nil {
			return fmt.Errorf("store: lookup inbound: %w", err)
		}
		if inboundBelongs == 0 {
			return ErrReleaseTargetRequired
		}

		if _, err := tx.ExecContext(ctx,
			`UPDATE payment_records
			    SET release_server_id = ?, release_inbound_id = ?, updated_at = ?
			  WHERE id = ?`,
			serverID, inboundID, nowUnix(), orderID); err != nil {
			return fmt.Errorf("store: set release target: %w", err)
		}
		return s.AppendOrderEvent(ctx, tx, OrderEvent{
			PaymentID: orderID, Kind: EventReleaseTarget, ActorID: &actorID,
			Note: fmt.Sprintf("server %d, inbound %d", serverID, inboundID),
		})
	})
}

// ErrReleaseTargetRequired means a custom order cannot be released until the
// operator says which panel it belongs on.
var ErrReleaseTargetRequired = errors.New("store: this order needs a server and an inbound before it can be released")

// VPNIDForOrder returns the VPN an order produced, if it produced one.
func (s *Store) VPNIDForOrder(ctx context.Context, orderID int64) (int64, error) {
	var vpn sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT vpn_instance_id FROM payment_records WHERE id = ?`, orderID).Scan(&vpn)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrPaymentNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("store: lookup order: %w", err)
	}
	if !vpn.Valid {
		return 0, ErrNoVPNToBlock
	}
	return vpn.Int64, nil
}
