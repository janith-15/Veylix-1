package payments

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/domain"
	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/store"
)

// fakeSettler records what the confirmation asked provisioning to do and answers
// with a scripted outcome, so the money path is testable without a panel.
//
// When `result.Recorded` is true it also inserts the VPN row, exactly as
// provisioning would, so `payment_records.vpn_instance_id` can satisfy its
// foreign key — the test exercises the real constraint instead of bypassing it.
type fakeSettler struct {
	st        *store.Store
	created   []provisioning.CreateInput
	result    *provisioning.Result
	createErr error

	links    []string
	linkSrc  string
	linksErr error
}

func (f *fakeSettler) Create(ctx context.Context, in provisioning.CreateInput) (*provisioning.Result, error) {
	f.created = append(f.created, in)
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.result != nil && f.result.Recorded && f.result.VPNID > 0 && f.st != nil {
		if _, err := f.st.DB().ExecContext(ctx,
			`INSERT INTO vpn_instances
			   (id, owner_user_id, server_id, offer_id, panel_email, panel_sub_id, status, created_at, updated_at)
			 VALUES (?, ?, 1, ?, ?, ?, 'ACTIVE', 1, 1)`,
			f.result.VPNID, in.ClientUserID, in.OfferID, f.result.Email, "sub-"+f.result.Email); err != nil {
			return nil, err
		}
	}
	return f.result, nil
}

func (f *fakeSettler) Links(_ context.Context, _ int64, _ auth.Principal) (*provisioning.ConnectionInfo, error) {
	if f.linksErr != nil {
		return nil, f.linksErr
	}
	src := f.linkSrc
	if src == "" {
		src = provisioning.LinkSourcePanel
	}
	return &provisioning.ConnectionInfo{Links: f.links, Source: src}, nil
}

type env struct {
	st       *store.Store
	svc      *Service
	fake     *fakeSettler
	planID   int64
	ownerID  int64
	clientID int64
}

func newEnv(t *testing.T, planOwner *int64) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "pay.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db := st.DB()
	mustExec := func(q string, args ...any) int64 {
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	serverID := mustExec(`INSERT INTO server_integrations
		(name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv','https://panel.example:2053/base', x'00', 'ACTIVE', 1, 1)`)
	inboundID := mustExec(`INSERT INTO inbounds (server_id, remote_id, remark, protocol, port, synced_at)
		VALUES (?, 1, '(IN)-vip', 'vless', 443, 1)`, serverID)
	planID := mustExec(`INSERT INTO vpn_offers
		(kind, name, server_id, inbound_id, traffic_gb, duration_days, device_limit, active,
		 price_cents, currency, slots_total, created_at, updated_at)
		VALUES ('PREMIUM','Premium 50',?,?,50,30,2,1,150000,'LKR',5,1,1)`, serverID, inboundID)
	clientID := mustExec(`INSERT INTO users (email, role, status, created_at, updated_at)
		VALUES ('buyer@example.test','client','ACTIVE',1,1)`)
	ownerID := int64(0)
	if planOwner != nil {
		ownerID = *planOwner
	}

	fake := &fakeSettler{
		st:      st,
		result:  &provisioning.Result{VPNID: 42, Email: "vyx-42", Status: domain.VPNActive, Confirmed: true, Recorded: true},
		links:   []string{"vless://abc@panel.example:443?type=tcp#vyx-42"},
		linkSrc: provisioning.LinkSourcePanel,
	}
	return &env{st: st, svc: New(st, fake), fake: fake, planID: planID, ownerID: ownerID, clientID: clientID}
}

// openOrder creates a priced order for the seeded plan.
func (e *env) openOrder(t *testing.T, withReference bool) *store.PaymentRow {
	t.Helper()
	ref := ""
	if withReference {
		ref = "slip-9911"
	}
	pay, err := e.st.CreatePayment(context.Background(), store.CreatePaymentInput{
		PayerUserID: e.clientID,
		OfferID:     e.planID,
		Reference:   ref,
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	return pay
}

func superAdmin(id int64) Principal { return Principal{UserID: id, Role: string(auth.RoleSuperAdmin)} }
func reseller(id int64) Principal   { return Principal{UserID: id, Role: string(auth.RoleAdmin)} }

// D-031 — the owner's sentence, as a test: confirming an order builds the VPN,
// marks the money received, records what it produced and hands back the panel's
// links for the QR.
func TestConfirmPaymentBuildsVPN(t *testing.T) {
	e := newEnv(t, nil)
	pay := e.openOrder(t, true)

	out, err := e.svc.Confirm(context.Background(), ConfirmInput{PaymentID: pay.ID, Actor: superAdmin(1)})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	// One provisioning call, approved on behalf of the confirmation.
	if len(e.fake.created) != 1 {
		t.Fatalf("provisioning calls = %d, want 1", len(e.fake.created))
	}
	call := e.fake.created[0]
	if !call.Approved {
		t.Error("Approved = false, want true — the confirmation is what authorises the release")
	}
	if call.OfferID != e.planID {
		t.Errorf("OfferID = %d, want the order's plan %d", call.OfferID, e.planID)
	}
	if call.ClientUserID != e.clientID {
		t.Errorf("ClientUserID = %d, want the payer %d", call.ClientUserID, e.clientID)
	}

	// The money and the VPN are recorded together.
	if out.Payment.Status != store.PaymentConfirmed {
		t.Errorf("payment status = %s, want CONFIRMED", out.Payment.Status)
	}
	if out.Payment.VPNInstanceID == nil || *out.Payment.VPNInstanceID != 42 {
		t.Errorf("vpn_instance_id = %v, want 42", out.Payment.VPNInstanceID)
	}
	if out.Payment.ConfirmedBy == nil || *out.Payment.ConfirmedBy != 1 {
		t.Errorf("confirmed_by = %v, want 1", out.Payment.ConfirmedBy)
	}
	if out.VPN == nil || out.VPN.Status != domain.VPNActive {
		t.Errorf("vpn = %+v, want ACTIVE", out.VPN)
	}

	// The links a screen needs for the QR come back with the answer.
	if out.LinkSource != provisioning.LinkSourcePanel || len(out.Links) != 1 {
		t.Errorf("links = %v (source %s), want one panel link", out.Links, out.LinkSource)
	}
}

// A payment amount must come from the plan, never from the caller (M6-02).
func TestPaymentAmountComesFromPlan(t *testing.T) {
	e := newEnv(t, nil)
	ctx := context.Background()

	pay, err := e.st.CreatePayment(ctx, store.CreatePaymentInput{
		PayerUserID: e.clientID,
		OfferID:     e.planID,
		AmountCents: 1, // an attempt to buy a 1500.00 plan for one cent
		Quantity:    2,
	})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}
	if pay.AmountCents != 300000 {
		t.Errorf("amount = %d, want 300000 (2 x 1500.00 from the plan)", pay.AmountCents)
	}
	if pay.Currency != "LKR" {
		t.Errorf("currency = %q, want LKR (from the plan)", pay.Currency)
	}
}

// A panel failure must leave the operator's money untouched: the order stays
// pending and no VPN is claimed (D-031.3).
func TestConfirmPaymentPanelFailureLeavesOrderPending(t *testing.T) {
	e := newEnv(t, nil)
	pay := e.openOrder(t, true)
	e.fake.createErr = provisioning.ErrPanelFailed

	if _, err := e.svc.Confirm(context.Background(), ConfirmInput{PaymentID: pay.ID, Actor: superAdmin(1)}); err == nil {
		t.Fatal("Confirm must fail when the panel refuses")
	}

	after, err := e.st.PaymentByID(context.Background(), pay.ID)
	if err != nil {
		t.Fatalf("read payment: %v", err)
	}
	if after.Status != store.PaymentAwaitingConfirmation {
		t.Errorf("status = %s, want it unchanged (AWAITING_CONFIRMATION)", after.Status)
	}
	if after.VPNInstanceID != nil {
		t.Errorf("vpn_instance_id = %v, want nil", after.VPNInstanceID)
	}
}

// Double-clicking confirm must not build a second VPN.
func TestConfirmIsNotRepeatable(t *testing.T) {
	e := newEnv(t, nil)
	pay := e.openOrder(t, true)
	ctx := context.Background()

	if _, err := e.svc.Confirm(ctx, ConfirmInput{PaymentID: pay.ID, Actor: superAdmin(1)}); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	if _, err := e.svc.Confirm(ctx, ConfirmInput{PaymentID: pay.ID, Actor: superAdmin(1)}); !errors.Is(err, ErrNotConfirmable) {
		t.Fatalf("second confirm error = %v, want ErrNotConfirmable", err)
	}
	if len(e.fake.created) != 1 {
		t.Errorf("provisioning calls = %d, want 1 — a confirmed order cannot be confirmed again", len(e.fake.created))
	}
}

// Only the Super Admin (or the plan's own reseller) may confirm an order.
func TestConfirmRequiresTheRightActor(t *testing.T) {
	e := newEnv(t, nil)
	pay := e.openOrder(t, true)
	ctx := context.Background()

	// A reseller confirming another payer's platform order.
	if _, err := e.svc.Confirm(ctx, ConfirmInput{PaymentID: pay.ID, Actor: reseller(e.clientID)}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
	if len(e.fake.created) != 0 {
		t.Error("a forbidden confirmation must not reach provisioning")
	}
}

// A reseller confirms an order on his OWN plan without the Super Admin
// (D-030.2) — the same money path, a different actor.
func TestResellerConfirmsOwnPlanOrder(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, nil)
	ownerID := newUser(t, e.st, "reseller@example.test", "admin")

	plan, err := e.st.CreatePlan(ctx, store.CreatePlanInput{
		Kind: "PREMIUM", Name: "Reseller plan", ServerID: 1, PriceCents: 50000,
		Active: true, OwnerUserID: &ownerID, SlotsTotal: 3,
	})
	if err != nil {
		t.Fatalf("create owned plan: %v", err)
	}
	if _, err := e.st.DB().Exec(
		`UPDATE vpn_offers SET inbound_id = (SELECT id FROM inbounds LIMIT 1) WHERE id = ?`, plan.ID); err != nil {
		t.Fatalf("attach inbound: %v", err)
	}

	pay, err := e.st.CreatePayment(ctx, store.CreatePaymentInput{PayerUserID: ownerID, OfferID: plan.ID})
	if err != nil {
		t.Fatalf("create payment: %v", err)
	}

	out, err := e.svc.Confirm(ctx, ConfirmInput{PaymentID: pay.ID, Actor: reseller(ownerID)})
	if err != nil {
		t.Fatalf("reseller confirm: %v", err)
	}
	if out.Payment.Status != store.PaymentConfirmed {
		t.Errorf("status = %s, want CONFIRMED", out.Payment.Status)
	}
	// A reseller's own sale is credited to him, so his quota is charged.
	if got := e.fake.created[0].SellerID; got != ownerID {
		t.Errorf("SellerID = %d, want the owning reseller %d", got, ownerID)
	}
}

// newUser inserts an account and returns its id.
func newUser(t *testing.T, st *store.Store, email, role string) int64 {
	t.Helper()
	res, err := st.DB().Exec(
		`INSERT INTO users (email, role, status, created_at, updated_at) VALUES (?, ?, 'ACTIVE', 1, 1)`,
		email, role)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}
