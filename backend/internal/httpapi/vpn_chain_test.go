package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/payments"
	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/store"
)

/*
 * The VPN fulfilment chain: offer -> order -> confirmation -> VPN.
 *
 * These tests exist because the chain could break silently in the middle. A
 * payment created WITHOUT offer_id settles as cash and builds nothing, so the
 * customer is charged and receives no product, and the confirmation screen
 * called it a success.
 *
 * The rule under test is D-031.1: an order that names a plan must produce a VPN,
 * and an order that names no plan must never be presented as one that did.
 */

// chainPanel is a panel stub that records what provisioning asked it to do.
type chainPanel struct {
	added []panel.AddClientRequest
}

func (p *chainPanel) AddClient(_ context.Context, req panel.AddClientRequest) error {
	p.added = append(p.added, req)
	return nil
}

func (p *chainPanel) ClientExists(_ context.Context, email string) (bool, error) {
	for _, a := range p.added {
		if a.Client.Email == email {
			return true, nil
		}
	}
	return false, nil
}

func (p *chainPanel) DeleteClient(_ context.Context, _ string) error { return nil }

func (p *chainPanel) SetClientEnabled(_ context.Context, _ string, _ bool) error { return nil }

func (p *chainPanel) Links(_ context.Context, _ string) ([]string, error) {
	return []string{"vless://chain@panel.example:443#chain"}, nil
}

// chainEnv is one router with a real provisioning and payments service, so the
// test exercises the wiring the binary ships rather than a stub that cannot fail.
type chainEnv struct {
	srv   *httptest.Server
	st    *store.Store
	panel *chainPanel
}

func newChainEnv(t *testing.T) *chainEnv {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/chain.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	env := &chainEnv{st: st, panel: &chainPanel{}}
	srv := mountChain(t, st, env.panel, true)
	env.srv = srv
	return env
}

// newChainEnvWith mounts a full router over an EXISTING store with the settlement
// service omitted. It exists so a test can prove what happens when a deployment
// forgets to wire Orders — the case that used to settle money silently.
func newChainEnvWith(t *testing.T, st *store.Store, pn *chainPanel) *httptest.Server {
	t.Helper()
	return mountChain(t, st, pn, false)
}

// mountChain builds the router. `withOrders` is the only difference between the
// shipping wiring and the deliberately broken one.
func mountChain(t *testing.T, st *store.Store, pn *chainPanel, withOrders bool) *httptest.Server {
	t.Helper()
	if pn == nil {
		pn = &chainPanel{}
	}

	auditLog := audit.New(st.DB()).WithNotifier(st)
	provSvc := provisioning.New(st.DB(), st, auditLog,
		func(context.Context, int64) (provisioning.PanelClient, error) { return pn, nil })

	authSvc := auth.NewService(st.DB(), auditLog)
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(st.DB(), false)

	deps := httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		AdminAudit:    auditLog,
		Notifications: st,
		VPNs:          provSvc,
		VPNLister:     httpapi.NewSQLVPNLister(st.DB()),
		Offers:        st,
		Payments:      st,
		Audit:         st,
		Quota:         st,
	}
	if withOrders {
		deps.Orders = payments.New(st, provSvc)
	}

	srv := httptest.NewServer(httpapi.Router(deps))
	t.Cleanup(srv.Close)
	return srv
}

// signup registers a client through the real route and returns its id.
func (e *chainEnv) signupClient(t *testing.T, email string) (*http.Client, int64) {
	t.Helper()
	c := &http.Client{Jar: mustJar(t)}
	resp, body := postJSON(t, c, e.srv.URL+"/api/auth/register", map[string]string{
		"email": email, "password": "a very long password",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register %s = %d (%v)", email, resp.StatusCode, body)
	}
	return c, int64(body["id"].(float64))
}

// seedChainCatalog creates one ACTIVE server and one PREMIUM offer, which is the
// minimum a sellable plan needs (D-075: an offer must name a real inbound).
func (e *chainEnv) seedChainCatalog(t *testing.T) (serverID, offerID int64) {
	t.Helper()
	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := e.st.DB().Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	serverID = exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('chain-srv', 'https://panel.example/base', x'00', 'ACTIVE', 1, 1)`)

	// An offer must name a real inbound (D-075), and the inbound must belong to
	// the same panel, or the plan describes a product that cannot be built.
	exec(
		`INSERT INTO inbounds (server_id, remote_id, remark, tag, protocol, port, enabled, synced_at)
		 VALUES (?, 7, 'chain-in', 'chain-in', 'vless', 443, 1, 1)`, serverID)

	offerID = exec(
		`INSERT INTO vpn_offers (kind, name, server_id, inbound_id, traffic_gb, duration_days,
		                         device_limit, price_cents, currency, active, created_at, updated_at)
		 VALUES ('PREMIUM', 'Chain 50GB', ?, 1, 50, 30, 2, 150000, 'LKR', 1, 1, 1)`,
		serverID)
	return serverID, offerID
}

// TestOrderWithPlanBuildsTheVpnOnConfirm is the P0 guarantee.
//
// A client buys a plan: the payment must record WHICH plan, and confirming it
// must build the VPN on that plan's panel. If this fails, the platform takes
// money and delivers nothing.
func TestOrderWithPlanBuildsTheVpnOnConfirm(t *testing.T) {
	env := newChainEnv(t)
	root := bootstrapSuperAdmin(t, env.srv.URL)
	client, clientID := env.signupClient(t, "chain-buyer@example.com")

	_, offerID := env.seedChainCatalog(t)

	// --- the order names the plan -------------------------------------------
	resp, order := postJSON(t, client, env.srv.URL+"/api/payments", map[string]any{
		"offer_id":  offerID,
		"quantity":  1,
		"reference": "SLIP-9001",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create payment with offer_id = %d (%v), want 201", resp.StatusCode, order)
	}

	// The plan must be recorded on the order. A NULL here is the defect this
	// test exists to catch: settlement would then have nothing to build from.
	var recordedOffer *int64
	var payID int64
	if err := env.st.DB().QueryRow(
		`SELECT id, offer_id FROM payment_records ORDER BY id DESC LIMIT 1`).
		Scan(&payID, &recordedOffer); err != nil {
		t.Fatalf("read payment: %v", err)
	}
	if recordedOffer == nil || *recordedOffer != offerID {
		t.Fatalf("payment %d recorded offer_id = %v, want %d — the order lost its "+
			"plan, so no VPN can ever be built from it", payID, recordedOffer, offerID)
	}

	// --- the operator confirms ----------------------------------------------
	resp, confirmed := postJSON(t, root,
		env.srv.URL+"/api/payments/"+itoa(payID)+"/confirm", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm = %d (%v), want 200", resp.StatusCode, confirmed)
	}

	// --- a VPN exists, owned by the buyer -----------------------------------
	var vpnCount int
	if err := env.st.DB().QueryRow(
		`SELECT COUNT(*) FROM vpn_instances WHERE owner_user_id = ?`, clientID).
		Scan(&vpnCount); err != nil {
		t.Fatalf("count vpn: %v", err)
	}
	if vpnCount != 1 {
		t.Fatalf("after confirming a plan order the buyer owns %d VPNs, want 1 — "+
			"the money was taken and no product was built", vpnCount)
	}

	// The panel was actually asked to create it: local state alone is not proof.
	if len(env.panel.added) != 1 {
		t.Fatalf("panel AddClient calls = %d, want 1 — the VPN exists only in the "+
			"local mirror", len(env.panel.added))
	}

	// The payment points at the VPN it produced, so the two can never drift.
	var linked *int64
	if err := env.st.DB().QueryRow(
		`SELECT vpn_instance_id FROM payment_records WHERE id = ?`, payID).
		Scan(&linked); err != nil {
		t.Fatalf("read payment link: %v", err)
	}
	if linked == nil {
		t.Fatalf("payment %d confirmed without recording the VPN it produced", payID)
	}

	// The customer is handed a working configuration.
	if links, ok := confirmed["links"].([]any); !ok || len(links) == 0 {
		t.Fatalf("confirmation returned no links (%v), so the buyer cannot connect",
			confirmed["links"])
	}
}

// TestConfirmingAPlanlessOrderSettlesCashOnly is the other half of the rule.
//
// A payment with no plan is a deposit: it settles as money and releases no
// product. That is legal, but it must be UNMISTAKABLE in the result — the
// response carries no VPN and the audit entry says vpn_id 0 — so no screen and no
// later reader can present a deposit as a delivered VPN (D-031.1).
func TestConfirmingAPlanlessOrderSettlesCashOnly(t *testing.T) {
	env := newChainEnv(t)
	root := bootstrapSuperAdmin(t, env.srv.URL)
	client, _ := env.signupClient(t, "chain-cash@example.com")

	// A deposit: money, no product.
	resp, order := postJSON(t, client, env.srv.URL+"/api/payments", map[string]any{
		"amount_cents": 500000,
		"reference":    "DEPOSIT-1",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create deposit = %d (%v), want 201", resp.StatusCode, order)
	}

	var payID int64
	if err := env.st.DB().QueryRow(
		`SELECT id FROM payment_records ORDER BY id DESC LIMIT 1`).Scan(&payID); err != nil {
		t.Fatalf("read payment: %v", err)
	}

	resp, body := postJSON(t, root,
		env.srv.URL+"/api/payments/"+itoa(payID)+"/confirm", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm a deposit = %d (%v), want 200 (a top-up is legal)",
			resp.StatusCode, body)
	}
	if body["offer_id"] != nil {
		t.Fatalf("a deposit gained an offer_id (%v) — it was never an order for a plan",
			body["offer_id"])
	}
	if body["vpn_instance_id"] != nil {
		t.Fatalf("a deposit points at a VPN (%v) — settlement must never invent a product",
			body["vpn_instance_id"])
	}

	// Nothing was provisioned, and the panel was never asked to create anything.
	var vpns int
	if err := env.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances`).Scan(&vpns); err != nil {
		t.Fatalf("count vpn: %v", err)
	}
	if vpns != 0 {
		t.Fatalf("a deposit confirmation created %d VPNs", vpns)
	}
	if len(env.panel.added) != 0 {
		t.Fatalf("a deposit reached the panel %d times", len(env.panel.added))
	}

	// The audit entry states plainly that no product was released.
	var ctxJSON string
	if err := env.st.DB().QueryRow(
		`SELECT context FROM audit_logs WHERE action = 'payment.confirm' ORDER BY id DESC LIMIT 1`).
		Scan(&ctxJSON); err != nil {
		t.Fatalf("audit entry missing: %v", err)
	}
	if !strings.Contains(ctxJSON, `"vpn_id":0`) {
		t.Fatalf("deposit audit entry does not record that no VPN was released: %s", ctxJSON)
	}
}

// TestPlanOrderRefusedWhenSettlementServiceIsMissing proves the guard, not the
// happy path. If the settlement service is not wired, a plan order must be
// REFUSED (503) rather than falling through to the cash-only path, which would
// mark a paid plan confirmed and deliver nothing (D-031.1).
func TestPlanOrderRefusedWhenSettlementServiceIsMissing(t *testing.T) {
	env := newChainEnv(t)
	root := bootstrapSuperAdmin(t, env.srv.URL)
	client, _ := env.signupClient(t, "chain-nosvc@example.com")
	_, offerID := env.seedChainCatalog(t)

	resp, order := postJSON(t, client, env.srv.URL+"/api/payments", map[string]any{
		"offer_id": offerID, "quantity": 1,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create payment = %d (%v)", resp.StatusCode, order)
	}
	var payID int64
	if err := env.st.DB().QueryRow(
		`SELECT id FROM payment_records ORDER BY id DESC LIMIT 1`).Scan(&payID); err != nil {
		t.Fatalf("read payment: %v", err)
	}

	// A second router over the SAME store, with Orders deliberately nil: this is
	// the deployment mistake the guard exists for.
	bare := newChainEnvWith(t, env.st, nil)
	defer bare.Close()

	resp, body := postJSON(t, root,
		bare.URL+"/api/payments/"+itoa(payID)+"/confirm", map[string]any{})
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("confirming a plan order with no settlement service = %d (%v), want 503 — "+
			"falling back to settlement would take the money and build nothing",
			resp.StatusCode, body)
	}

	// The payment is untouched: still awaiting a decision, not confirmed.
	var status string
	if err := env.st.DB().QueryRow(
		`SELECT status FROM payment_records WHERE id = ?`, payID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status == string(store.PaymentConfirmed) {
		t.Fatalf("a refused plan confirmation marked payment %d CONFIRMED", payID)
	}
}

// TestOfferCreateRefusesAnInboundFromAnotherPanel closes the other door into an
// unprovisionable offer. The form shows one server's inbounds, but a direct POST
// can pair any inbound with any server. Such a row would save, appear in the
// catalogue and then fail at release time (or worse, attach a client to an inbound
// on a different panel) — so creation refuses it.
func TestOfferCreateRefusesAnInboundFromAnotherPanel(t *testing.T) {
	env := newChainEnv(t)
	root := bootstrapSuperAdmin(t, env.srv.URL)

	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := env.st.DB().Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	serverA := exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('panel-a', 'https://a.example/base', x'00', 'ACTIVE', 1, 1)`)
	serverB := exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('panel-b', 'https://b.example/base', x'00', 'ACTIVE', 1, 1)`)

	// The inbound lives on panel B.
	inboundOnB := exec(
		`INSERT INTO inbounds (server_id, remote_id, remark, tag, protocol, port, enabled, synced_at)
		 VALUES (?, 11, 'b-in', 'b-in', 'vless', 443, 1, 1)`, serverB)

	// The offer names panel A with panel B's inbound.
	resp, body := postJSON(t, root, env.srv.URL+"/api/offers", map[string]any{
		"kind": "PREMIUM", "name": "Cross-panel", "server_id": serverA,
		"inbound_id": inboundOnB, "traffic_gb": 10, "duration_days": 30,
		"device_limit": 1, "price_cents": 100000, "currency": "LKR",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("offer pairing server A with an inbound on server B = %d (%v), "+
			"want 422 — such an offer can never build a client", resp.StatusCode, body)
	}
	if code, _ := body["error"].(map[string]any)["code"].(string); code != "inbound_mismatch" {
		t.Fatalf("refusal code = %q, want inbound_mismatch", code)
	}

	var created int
	if err := env.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_offers`).Scan(&created); err != nil {
		t.Fatalf("count offers: %v", err)
	}
	if created != 0 {
		t.Fatalf("the mismatched offer was saved anyway (%d rows)", created)
	}

	// The matching pairing is still accepted, so the guard is not just refusing
	// everything.
	inboundOnA := exec(
		`INSERT INTO inbounds (server_id, remote_id, remark, tag, protocol, port, enabled, synced_at)
		 VALUES (?, 12, 'a-in', 'a-in', 'vless', 443, 1, 1)`, serverA)

	resp, body = postJSON(t, root, env.srv.URL+"/api/offers", map[string]any{
		"kind": "PREMIUM", "name": "Matched", "server_id": serverA,
		"inbound_id": inboundOnA, "traffic_gb": 10, "duration_days": 30,
		"device_limit": 1, "price_cents": 100000, "currency": "LKR",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("offer with a matching inbound = %d (%v), want 201", resp.StatusCode, body)
	}
}

// TestClientCannotConfirmPayments pins the role rule: confirmation is the act

// that releases product, so it belongs to the seller, not the buyer (D-030.2).
func TestClientCannotConfirmPayments(t *testing.T) {
	env := newChainEnv(t)
	_ = bootstrapSuperAdmin(t, env.srv.URL)
	client, _ := env.signupClient(t, "chain-selfconf@example.com")

	_, offerID := env.seedChainCatalog(t)
	resp, order := postJSON(t, client, env.srv.URL+"/api/payments", map[string]any{
		"offer_id": offerID, "quantity": 1,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create payment = %d (%v)", resp.StatusCode, order)
	}

	var payID int64
	if err := env.st.DB().QueryRow(
		`SELECT id FROM payment_records ORDER BY id DESC LIMIT 1`).Scan(&payID); err != nil {
		t.Fatalf("read payment: %v", err)
	}

	resp, body := postJSON(t, client,
		env.srv.URL+"/api/payments/"+itoa(payID)+"/confirm", map[string]any{})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a client confirming their own order = %d (%v), want 403",
			resp.StatusCode, body)
	}
}
