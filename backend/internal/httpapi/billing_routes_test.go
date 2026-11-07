package httpapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/store"
)

// billingEnv is a real router over real SQLite with the inbox, directory and
// billing stores wired exactly as cmd/veylixd wires them.
type billingEnv struct {
	srv *httptest.Server
	st  *store.Store
}

func newBillingEnv(t *testing.T) *billingEnv {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/billing.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	auditLog := audit.New(st.DB())
	authSvc := auth.NewService(st.DB(), auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(st.DB(), false)

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		AdminAudit:    auditLog,
		Notifications: st,
		Directory:     st,
		Payments:      st,
		// Wired so the billing test can prove every transition left a trail.
		Audit: st,
		Quota: st,
	}))
	t.Cleanup(srv.Close)
	return &billingEnv{srv: srv, st: st}
}

func (e *billingEnv) root(t *testing.T) *http.Client {
	t.Helper()
	return bootstrapSuperAdmin(t, e.srv.URL)
}

// signup registers a client account through the real API.
func (e *billingEnv) signup(t *testing.T, email string) *http.Client {
	t.Helper()
	c := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	resp, body := postJSON(t, c, e.srv.URL+"/api/auth/register", map[string]string{
		"email": email, "password": "a very long password",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register %s = %d (%v)", email, resp.StatusCode, body)
	}
	return c
}

// ---- P8: notifications ----------------------------------------------------

// TestNotificationsArePrivateToTheirOwner is the privacy regression test for
// the inbox: a notification belongs to exactly one account, and no role — not
// even the super admin — may read or mark somebody else's.
//
// The store is seeded directly because no producer writes notifications yet;
// the point under test is the read/mark scoping, not the producer.
func TestNotificationsArePrivateToTheirOwner(t *testing.T) {
	env := newBillingEnv(t)
	root := env.root(t)
	owner := env.signup(t, "payer@example.com")
	other := env.signup(t, "stranger@example.com")

	// Resolve the two client ids through the API so the test does not guess.
	rows := getJSONArray(t, root, env.srv.URL+"/api/directory")
	var ownerID int64
	for _, r := range rows {
		if r["email"] == "payer@example.com" {
			ownerID = int64(r["id"].(float64))
		}
	}
	if ownerID == 0 {
		t.Fatalf("client not found in the directory: %v", rows)
	}

	n, err := env.st.CreateNotification(context.Background(), store.CreateNotificationInput{
		UserID: ownerID, Kind: store.NotifSuccess, Title: "VPN active", Body: "Your free VPN is ready",
	})
	if err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}

	// The owner sees it and the unread badge counts it.
	list := getJSONArray(t, owner, env.srv.URL+"/api/notifications")
	if len(list) != 1 {
		t.Fatalf("owner inbox = %d rows, want 1", len(list))
	}
	_, unread := getJSON(t, owner, env.srv.URL+"/api/notifications/unread")
	if unread["unread"].(float64) != 1 {
		t.Fatalf("owner unread = %v, want 1", unread["unread"])
	}

	// Everybody else's inbox is empty — including the super admin's.
	if again := getJSONArray(t, other, env.srv.URL+"/api/notifications"); len(again) != 0 {
		t.Fatalf("other client inbox = %d rows, want 0", len(again))
	}
	if rootBox := getJSONArray(t, root, env.srv.URL+"/api/notifications"); len(rootBox) != 0 {
		t.Fatalf("super admin inbox = %d rows, want 0", len(rootBox))
	}

	// Another account marking it read is a 404 — not a silent success.
	resp, body := postJSON(t, other, env.srv.URL+"/api/notifications/"+usageID(n.ID)+"/read", map[string]any{})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("other marks read = %d (%v), want 404", resp.StatusCode, body)
	}
	// ... and the owner's badge is untouched by that attempt.
	_, unread = getJSON(t, owner, env.srv.URL+"/api/notifications/unread")
	if unread["unread"].(float64) != 1 {
		t.Fatalf("owner unread after foreign mark = %v, want 1", unread["unread"])
	}

	// The owner marks it read: idempotent, and the badge clears.
	for i := 0; i < 2; i++ {
		resp, body = postJSON(t, owner, env.srv.URL+"/api/notifications/"+usageID(n.ID)+"/read", map[string]any{})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("owner marks read (try %d) = %d (%v), want 200", i+1, resp.StatusCode, body)
		}
	}
	_, unread = getJSON(t, owner, env.srv.URL+"/api/notifications/unread")
	if unread["unread"].(float64) != 0 {
		t.Fatalf("owner unread after read = %v, want 0", unread["unread"])
	}
}

// ---- P6: billing ----------------------------------------------------------

// TestPaymentLifecycleIsGuarded walks the whole manual-payment flow and proves
// each illegal jump is refused rather than silently applied.
//
// The rule being protected (REQUIREMENTS §2.5): money is only ever marked
// received by a human operator, and a reviewed payment can never be re-opened.
func TestPaymentLifecycleIsGuarded(t *testing.T) {
	env := newBillingEnv(t)
	root := env.root(t)
	payer := env.signup(t, "buyer@example.com")

	// The super admin publishes a global destination.
	resp, method := postJSON(t, root, env.srv.URL+"/api/payment-methods", map[string]string{
		"kind": "BANK", "label": "Veylix Bank", "details": "AC 0012", "instructions": "Use your email as the reference",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create method = %d (%v), want 201", resp.StatusCode, method)
	}
	methodID := int64(method["id"].(float64))

	// The payer sees the global method (their own list includes it).
	methods := getJSONArray(t, payer, env.srv.URL+"/api/payment-methods")
	if len(methods) != 1 || methods[0]["label"] != "Veylix Bank" {
		t.Fatalf("payer methods = %v, want the global method", methods)
	}

	// A client may not publish a destination.
	resp, body := postJSON(t, payer, env.srv.URL+"/api/payment-methods",
		map[string]string{"kind": "BANK", "label": "Mine"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("client creates a method = %d (%v), want 403", resp.StatusCode, body)
	}

	// Create the payment: no reference yet → PENDING.
	resp, pay := postJSON(t, payer, env.srv.URL+"/api/payments", map[string]any{
		"amount_cents": 1500, "method_id": methodID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create payment = %d (%v), want 201", resp.StatusCode, pay)
	}
	if pay["status"] != store.PaymentPending {
		t.Fatalf("new payment status = %v, want %s", pay["status"], store.PaymentPending)
	}
	payID := usageID(int64(pay["id"].(float64)))
	payURL := env.srv.URL + "/api/payments/" + payID

	// A payer can never confirm their own money.
	resp, body = postJSON(t, payer, payURL+"/confirm", map[string]any{})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("client confirms own payment = %d (%v), want 403", resp.StatusCode, body)
	}

	// Report the transfer: PENDING → AWAITING_CONFIRMATION.
	resp, pay = postJSON(t, payer, payURL+"/reference", map[string]string{"reference": "TXN-991"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("attach reference = %d (%v), want 200", resp.StatusCode, pay)
	}
	if pay["status"] != store.PaymentAwaitingConfirmation {
		t.Fatalf("status after reference = %v, want %s", pay["status"], store.PaymentAwaitingConfirmation)
	}

	// Re-sending the reference is refused: the record has moved on.
	resp, body = postJSON(t, payer, payURL+"/reference", map[string]string{"reference": "TXN-992"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("re-attach reference = %d (%v), want 409", resp.StatusCode, body)
	}

	// The operator confirms.
	resp, pay = postJSON(t, root, payURL+"/confirm", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm = %d (%v), want 200", resp.StatusCode, pay)
	}
	if pay["status"] != store.PaymentConfirmed {
		t.Fatalf("status after confirm = %v, want %s", pay["status"], store.PaymentConfirmed)
	}
	if pay["confirmed_at"] == nil {
		t.Fatalf("confirm did not stamp confirmed_at: %v", pay)
	}

	// Confirming twice is a 409, not a second confirmation.
	resp, body = postJSON(t, root, payURL+"/confirm", map[string]any{})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("double confirm = %d (%v), want 409", resp.StatusCode, body)
	}
	// A confirmed payment can no longer be cancelled by the payer.
	resp, body = postJSON(t, payer, payURL+"/cancel", map[string]any{})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("cancel after confirm = %d (%v), want 409", resp.StatusCode, body)
	}

	// Refund is the only exit from CONFIRMED, and only once.
	resp, pay = postJSON(t, root, payURL+"/refund", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refund = %d (%v), want 200", resp.StatusCode, pay)
	}
	if pay["status"] != store.PaymentRefunded {
		t.Fatalf("status after refund = %v, want %s", pay["status"], store.PaymentRefunded)
	}
	resp, body = postJSON(t, root, payURL+"/refund", map[string]any{})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("double refund = %d (%v), want 409", resp.StatusCode, body)
	}

	// Every transition left a trail. The read returns the D-082 envelope — the
	// entries live under "entries", so the assertion reads that window.
	_, auditPage := getJSON(t, root, env.srv.URL+"/api/audit?kind=payment&limit=50")
	auditRows, _ := auditPage["entries"].([]any)
	want := map[string]bool{
		audit.ActionPaymentMethodCreate: false,
		audit.ActionPaymentCreate:       false,
		audit.ActionPaymentReference:    false,
		audit.ActionPaymentConfirm:      false,
		audit.ActionPaymentRefund:       false,
	}
	for _, r := range auditRows {
		action, _ := r.(map[string]any)["action"].(string)
		if _, tracked := want[action]; tracked {
			want[action] = true
		}
	}
	for action, seen := range want {
		if !seen {
			t.Fatalf("audit trail is missing %q", action)
		}
	}
}

// TestPaymentWithoutAnAccountIsRefused is the dev-bypass regression test.
//
// The D-022 bypass injects a synthetic principal whose id has no `users` row.
// Writing a payment for it must fail with a *named* refusal (403
// `no_payer_account`), never a 500: the server is fine, the caller simply is
// not a real account, and "who paid" must never be invented.
func TestPaymentWithoutAnAccountIsRefused(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir()+"/nopayer.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := st.CreatePayment(ctx, store.CreatePaymentInput{
		PayerUserID: 99, AmountCents: 1500,
	}); !errors.Is(err, store.ErrNoPayerAccount) {
		t.Fatalf("CreatePayment with no account = %v, want ErrNoPayerAccount", err)
	}
}

// the platform: a reseller never sees another reseller's customers, and a
// client never sees the reseller list.
//
// Before this endpoint the picker read a hard-coded list of eight invented
// people, so the "who may I chat with" rule was decoration, not enforcement.
func TestDirectoryScopesWhoCanBeAddressed(t *testing.T) {
	env := newBillingEnv(t)
	root := env.root(t)

	// Two resellers, each with one client, created through the real API.
	mkAdmin := func(email string) *http.Client {
		t.Helper()
		resp, body := postJSON(t, root, env.srv.URL+"/api/admins", map[string]any{
			"email": email, "password": "a very long password",
			"display_name": "Reseller", "role": "admin", "quota_limit": 5,
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create admin %s = %d (%v)", email, resp.StatusCode, body)
		}
		return loginAs(t, env.srv.URL, email, "a very long password")
	}
	adminA := mkAdmin("seller-a@example.com")
	mkAdmin("seller-b@example.com")

	clientA := env.signup(t, "client-a@example.com")
	env.signup(t, "client-b@example.com")

	// Give each client a sale from their own reseller, so `vpn_assignments`
	// links them. The VPN itself is irrelevant to the directory.
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := env.st.DB().Exec(q, args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	var idA, idB, idSellerA, idSellerB int64
	row := env.st.DB().QueryRow(`SELECT id FROM users WHERE email = 'client-a@example.com'`)
	if err := row.Scan(&idA); err != nil {
		t.Fatalf("lookup client-a: %v", err)
	}
	row = env.st.DB().QueryRow(`SELECT id FROM users WHERE email = 'client-b@example.com'`)
	if err := row.Scan(&idB); err != nil {
		t.Fatalf("lookup client-b: %v", err)
	}
	row = env.st.DB().QueryRow(`SELECT id FROM users WHERE email = 'seller-a@example.com'`)
	if err := row.Scan(&idSellerA); err != nil {
		t.Fatalf("lookup seller-a: %v", err)
	}
	row = env.st.DB().QueryRow(`SELECT id FROM users WHERE email = 'seller-b@example.com'`)
	if err := row.Scan(&idSellerB); err != nil {
		t.Fatalf("lookup seller-b: %v", err)
	}
	exec(`INSERT INTO server_integrations (id, name, panel_url, token_ciphertext, status, created_at, updated_at)
	      VALUES (1, 'dir-srv', 'https://panel.example/base', x'00', 'ACTIVE', 1, 1)`)
	exec(`INSERT INTO vpn_instances (id, server_id, owner_user_id, panel_email, panel_sub_id, status, created_at, updated_at)
	      VALUES (9001, 1, ?, 'dir-a@panel', 'sub-a', 'ACTIVE', 1, 1)`, idA)
	exec(`INSERT INTO vpn_instances (id, server_id, owner_user_id, panel_email, panel_sub_id, status, created_at, updated_at)
	      VALUES (9002, 1, ?, 'dir-b@panel', 'sub-b', 'ACTIVE', 1, 1)`, idB)
	exec(`INSERT INTO vpn_assignments (vpn_instance_id, seller_user_id, client_user_id, status, created_at, updated_at)
	      VALUES (9001, ?, ?, 'ACTIVE', 1, 1)`, idSellerA, idA)
	exec(`INSERT INTO vpn_assignments (vpn_instance_id, seller_user_id, client_user_id, status, created_at, updated_at)
	      VALUES (9002, ?, ?, 'ACTIVE', 1, 1)`, idSellerB, idB)
	exec(`INSERT INTO vpn_assignments (vpn_instance_id, seller_user_id, client_user_id, status, created_at, updated_at)
	      VALUES (9001, ?, ?, 'ACTIVE', 1, 1)`, idSellerA, idA)
	exec(`INSERT INTO vpn_assignments (vpn_instance_id, seller_user_id, client_user_id, status, created_at, updated_at)
	      VALUES (9002, ?, ?, 'ACTIVE', 1, 1)`, idSellerB, idB)

	emails := func(c *http.Client) map[string]bool {
		t.Helper()
		out := map[string]bool{}
		for _, r := range getJSONArray(t, c, env.srv.URL+"/api/directory") {
			out[r["email"].(string)] = true
		}
		return out
	}

	a := emails(adminA)
	if !a["client-a@example.com"] {
		t.Fatalf("reseller A cannot see their own client: %v", a)
	}
	if a["client-b@example.com"] {
		t.Fatalf("reseller A can see another reseller's client: %v", a)
	}
	if a["seller-b@example.com"] {
		t.Fatalf("reseller A can see another reseller: %v", a)
	}
	if !a["owner@veylix.local"] && !a["admin@veylix.local"] {
		// The bootstrap super admin must be addressable by everyone; its exact
		// address comes from the bootstrap helper, so assert "some super admin".
		hasSuper := false
		for _, r := range getJSONArray(t, adminA, env.srv.URL+"/api/directory") {
			if r["role"] == "super_admin" {
				hasSuper = true
			}
		}
		if !hasSuper {
			t.Fatalf("reseller A cannot see any super admin: %v", a)
		}
	}

	c := emails(clientA)
	if c["client-b@example.com"] || c["seller-b@example.com"] {
		t.Fatalf("client A can see unrelated accounts: %v", c)
	}
	if !c["seller-a@example.com"] {
		t.Fatalf("client A cannot see their own seller: %v", c)
	}

	// The super admin sees everybody.
	all := emails(root)
	for _, want := range []string{"client-a@example.com", "client-b@example.com", "seller-a@example.com"} {
		if !all[want] {
			t.Fatalf("super admin directory is missing %s: %v", want, all)
		}
	}
}
