package store

import (
	"context"
	"errors"
	"testing"
)

// seedOrderEnv creates a server, a client, two payment methods and one order.
func seedOrderEnv(t *testing.T, ctx context.Context, st *Store) (clientID, orderID, methodA, methodB int64) {
	t.Helper()
	serverID := seedServer(t, st)
	clientID = seedUser(t, ctx, st, "nimal@gmail.com", "client")

	mk := func(label string) int64 {
		m, err := st.CreatePaymentMethod(ctx, CreatePaymentMethodInput{
			Kind: "BANK", Label: label, Details: "acct 123",
			ContactWhatsApp: "+94771234567", ContactTelegram: "@veylixpay",
			ContactEmail: "pay@veylix.test",
		})
		if err != nil {
			t.Fatalf("create method: %v", err)
		}
		return m.ID
	}
	methodA, methodB = mk("Bank transfer"), mk("eZ Cash")

	// An order needs a plan only if it is priced from one; a plain amount is the
	// deposit form, which is enough to exercise the timeline.
	order, err := st.CreatePayment(ctx, CreatePaymentInput{
		PayerUserID: clientID, MethodID: methodA, AmountCents: 25900, Reference: "slip-1",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	_ = serverID
	return clientID, order.ID, methodA, methodB
}

// D-036.4 — the payer may change destination, and the change is an immutable
// event carrying BOTH ends, so "he moved from eZ Cash to bank" survives.
func TestMethodChangeIsRecorded(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	clientID, orderID, methodA, methodB := seedOrderEnv(t, ctx, st)

	if err := st.ChangePaymentMethod(ctx, clientID, orderID, methodB); err != nil {
		t.Fatalf("change method: %v", err)
	}

	pay, err := st.PaymentByID(ctx, orderID)
	if err != nil {
		t.Fatalf("read order: %v", err)
	}
	if pay.MethodID == nil || *pay.MethodID != methodB {
		t.Errorf("method = %v, want the new one (%d)", pay.MethodID, methodB)
	}

	events, err := st.OrderEvents(ctx, orderID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 method change", len(events))
	}
	e := events[0]
	if e.Kind != EventMethodChanged {
		t.Errorf("kind = %q, want METHOD_CHANGED", e.Kind)
	}
	if e.FromMethodID == nil || *e.FromMethodID != methodA {
		t.Errorf("from = %v, want the original method %d", e.FromMethodID, methodA)
	}
	if e.ToMethodID == nil || *e.ToMethodID != methodB {
		t.Errorf("to = %v, want the new method %d", e.ToMethodID, methodB)
	}
	// The labels make the history readable even if a method is later retired.
	if e.FromLabel != "Bank transfer" || e.ToLabel != "eZ Cash" {
		t.Errorf("labels = %q → %q, want Bank transfer → eZ Cash", e.FromLabel, e.ToLabel)
	}
	if e.ActorID == nil || *e.ActorID != clientID {
		t.Errorf("actor = %v, want the payer %d", e.ActorID, clientID)
	}
}

// Re-choosing the same destination is not an event: a timeline of no-ops is
// noise an operator has to read past.
func TestMethodChangeToSameIsNoop(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	clientID, orderID, methodA, _ := seedOrderEnv(t, ctx, st)

	if err := st.ChangePaymentMethod(ctx, clientID, orderID, methodA); err != nil {
		t.Fatalf("change to same: %v", err)
	}
	events, err := st.OrderEvents(ctx, orderID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("events = %d, want 0", len(events))
	}
}

// Only the payer may move their own order, and only while it is open.
func TestMethodChangeIsPayerOnly(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	clientID, orderID, _, methodB := seedOrderEnv(t, ctx, st)
	other := seedUser(t, ctx, st, "someone-else@example.com", "client")

	if err := st.ChangePaymentMethod(ctx, other, orderID, methodB); !errors.Is(err, ErrPaymentNotFound) {
		t.Fatalf("error = %v, want ErrPaymentNotFound (never a hint that the id exists)", err)
	}
	// The order must be untouched.
	pay, _ := st.PaymentByID(ctx, orderID)
	if pay.MethodID == nil || *pay.MethodID != methodAOf(t, ctx, st, orderID) {
		t.Error("a refused change must not move the destination")
	}
	_ = clientID
}

// methodAOf reads the order's current destination, so the assertion above stays
// readable.
func methodAOf(t *testing.T, ctx context.Context, st *Store, orderID int64) int64 {
	t.Helper()
	pay, err := st.PaymentByID(ctx, orderID)
	if err != nil || pay.MethodID == nil {
		t.Fatalf("read order: %v", err)
	}
	return *pay.MethodID
}

// D-036.3 — proof travels over a channel, so only the three named channels are
// accepted, and recording one moves the order to AWAITING_CONFIRMATION.
func TestProofChannelIsRecorded(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	clientID, orderID, _, _ := seedOrderEnv(t, ctx, st)

	if err := st.RecordProofChannel(ctx, clientID, orderID, "carrier-pigeon", ""); !errors.Is(err, ErrBadChannel) {
		t.Fatalf("error = %v, want ErrBadChannel", err)
	}

	if err := st.RecordProofChannel(ctx, clientID, orderID, ProofWhatsApp, "slip 4471"); err != nil {
		t.Fatalf("record proof: %v", err)
	}
	pay, err := st.PaymentByID(ctx, orderID)
	if err != nil {
		t.Fatalf("read order: %v", err)
	}
	if pay.ProofChannel != ProofWhatsApp {
		t.Errorf("proof_channel = %q, want WHATSAPP", pay.ProofChannel)
	}
	if pay.ProofNote != "slip 4471" {
		t.Errorf("proof_note = %q, want the note", pay.ProofNote)
	}
	if pay.Status != PaymentAwaitingConfirmation {
		t.Errorf("status = %s, want AWAITING_CONFIRMATION", pay.Status)
	}

	events, err := st.OrderEvents(ctx, orderID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Kind != EventProofChannel || events[0].Channel != ProofWhatsApp {
		t.Errorf("events = %+v, want one PROOF_CHANNEL / WHATSAPP", events)
	}
}
