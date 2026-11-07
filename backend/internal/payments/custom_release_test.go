package payments

import (
	"context"
	"errors"
	"testing"

	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/store"
)

// D-036.6 — a CUSTOM order cannot be released until the Super Admin chose the
// panel it belongs on. Confirming without a target must refuse, and must leave
// the money untouched.
func TestCustomOrderNeedsServerBeforeRelease(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()

	// A custom order: the customer's own spec, no release target yet.
	pay, err := e.st.CreatePayment(ctx, store.CreatePaymentInput{
		PayerUserID: e.clientID, OfferID: e.planID, AmountCents: 25900,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if _, err := e.st.DB().ExecContext(ctx,
		`UPDATE payment_records SET spec_gb = 200, spec_days = 30, spec_devices = 3, spec_ips = 2
		  WHERE id = ?`, pay.ID); err != nil {
		t.Fatalf("record spec: %v", err)
	}

	_, err = e.svc.Confirm(ctx, ConfirmInput{PaymentID: pay.ID, Actor: superAdmin(1)})
	if !errors.Is(err, ErrReleaseTargetRequired) {
		t.Fatalf("error = %v, want ErrReleaseTargetRequired", err)
	}
	if len(e.fake.created) != 0 {
		t.Error("provisioning must not be reached without a server and inbound")
	}
	after, _ := e.st.PaymentByID(ctx, pay.ID)
	if after.Status == store.PaymentConfirmed {
		t.Error("the order must not be marked confirmed")
	}
}

// With a target chosen, the release carries the CUSTOMER's spec — not the
// package's defaults — onto the panel the Super Admin picked.
func TestCustomOrderReleasesWithItsOwnSpec(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()

	pay, err := e.st.CreatePayment(ctx, store.CreatePaymentInput{
		PayerUserID: e.clientID, OfferID: e.planID, AmountCents: 25900,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if _, err := e.st.DB().ExecContext(ctx,
		`UPDATE payment_records SET spec_gb = 200, spec_days = 30, spec_devices = 3, spec_ips = 2
		  WHERE id = ?`, pay.ID); err != nil {
		t.Fatalf("record spec: %v", err)
	}
	inboundID := seedInbound(t, e.st, 1)
	if err := e.st.SetReleaseTarget(ctx, pay.ID, 1, 1, inboundID); err != nil {
		t.Fatalf("set target: %v", err)
	}

	out, err := e.svc.Confirm(ctx, ConfirmInput{PaymentID: pay.ID, Actor: superAdmin(1)})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(e.fake.created) != 1 {
		t.Fatalf("provisioning calls = %d, want 1", len(e.fake.created))
	}
	spec := e.fake.created[0].Spec
	if spec == nil {
		t.Fatal("Spec is nil — a custom order must release the customer's spec, not the package's defaults")
	}
	if spec.TrafficGB != 200 || spec.DurationDays != 30 || spec.Devices != 3 || spec.IPs != 2 {
		t.Errorf("spec = %+v, want 200 GB / 30 d / 3 dev / 2 IP", *spec)
	}
	if spec.ServerID != 1 || spec.InboundID != inboundID {
		t.Errorf("target = server %d / inbound %d, want 1 / %d", spec.ServerID, spec.InboundID, inboundID)
	}
	if out.Payment.Status != store.PaymentConfirmed {
		t.Errorf("status = %s, want CONFIRMED", out.Payment.Status)
	}
}

// The chosen inbound must belong to the chosen server: attaching a client to an
// inbound on a different panel would build the VPN in the wrong place.
func TestReleaseTargetMustMatchServer(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()

	pay, err := e.st.CreatePayment(ctx, StorePayment(e))
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	// Inbound 9999 does not exist on server 1.
	if err := e.st.SetReleaseTarget(ctx, pay.ID, 1, 1, 9999); !errors.Is(err, store.ErrReleaseTargetRequired) {
		t.Fatalf("error = %v, want ErrReleaseTargetRequired", err)
	}
}

// The order states who must settle it: 0 means the platform (the Super Admin).
func TestOrderNamesTheConfirmer(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()

	pay, err := e.st.CreatePayment(ctx, StorePayment(e))
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	got, err := e.st.PaymentByID(ctx, pay.ID)
	if err != nil {
		t.Fatalf("read order: %v", err)
	}
	// An unowned package is the platform's, so the confirmer is 0 (the SA).
	if got.ConfirmerUserID != 0 {
		t.Errorf("confirmer = %d, want 0 (the platform), since the plan is unowned", got.ConfirmerUserID)
	}
}

// StorePayment builds the minimal valid order for the seeded plan.
func StorePayment(e *env) store.CreatePaymentInput {
	return store.CreatePaymentInput{PayerUserID: e.clientID, OfferID: e.planID}
}

// seedInbound returns the seeded inbound on a server, creating one if the
// environment has none (the env already seeds one, and the server/remote_id
// pair is unique).
func seedInbound(t *testing.T, st *store.Store, serverID int64) int64 {
	t.Helper()
	var id int64
	err := st.DB().QueryRow(
		`SELECT id FROM inbounds WHERE server_id = ? ORDER BY id LIMIT 1`, serverID).Scan(&id)
	if err == nil {
		return id
	}
	res, err := st.DB().Exec(
		`INSERT INTO inbounds (server_id, remote_id, remark, protocol, port, synced_at)
		 VALUES (?, 1, '(IN)-custom', 'vless', 443, 1)`, serverID)
	if err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	id, _ = res.LastInsertId()
	return id
}

// compile-time guard: the production settler must satisfy the interface.
var _ Settler = (*provisioning.Service)(nil)
