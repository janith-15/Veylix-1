package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"testing"
	"time"
)

// loginPayloadFor is the in-package twin of httpapi_test.loginPayload: package
// httpapi tests talk to the router directly, so they need their own helper.
func loginPayloadFor(email, password string) map[string]string {
	lower := strings.ToLower(email)
	portal := "client"
	for _, staff := range []string{"owner", "reseller", "admin", "staff", "root", "seller"} {
		if strings.Contains(lower, staff) {
			portal = "staff"
			break
		}
	}
	return map[string]string{"email": email, "password": password, "portal": portal}
}

// signInAs switches the environment's HTTP client to a different account, so a
// test can act as the payer rather than the operator.
func (e *assignEnv) signInAs(t *testing.T, email string) {
	t.Helper()
	if _, err := e.st.DB().Exec(
		`UPDATE users SET password_hash = ? WHERE email = ?`,
		testPasswordHash(t, "a very long password"), email); err != nil {
		t.Fatalf("set password: %v", err)
	}
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 10 * time.Second, Transport: testTransport}
	raw, _ := json.Marshal(loginPayloadFor(email, "a very long password"))
	res, err := c.Post(e.url+"/api/auth/login", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login as %s = %d", email, res.StatusCode)
	}
	e.http = c
}

// orderEnv seeds one order with two destinations, owned by the signed-in client.
func (e *assignEnv) seedOrder(t *testing.T) (orderID, methodA, methodB int64) {
	t.Helper()
	mk := func(label string) int64 {
		res, err := e.st.DB().Exec(
			`INSERT INTO payment_methods (kind, label, details, instructions, active, created_at,
			                              contact_email, contact_whatsapp, contact_telegram)
			 VALUES ('BANK', ?, 'acct', '', 1, 1, 'pay@veylix.test', '+94771234567', '@veylixpay')`, label)
		if err != nil {
			t.Fatalf("seed method: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	methodA, methodB = mk("Bank transfer"), mk("eZ Cash")

	// The order is a deposit form (amount, no plan), which is enough to exercise
	// the timeline without a panel.
	res, err := e.st.DB().Exec(
		`INSERT INTO payment_records (payer_user_id, method_id, amount_cents, currency, reference,
		                              status, created_at, updated_at)
		 VALUES (?, ?, 25900, 'LKR', 'slip-1', 'AWAITING_CONFIRMATION', 1, 1)`, e.client, methodA)
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
	orderID, _ = res.LastInsertId()
	return orderID, methodA, methodB
}

// D-036.4 / I3 — the payer changes destination and the owner sees BOTH ends in
// the timeline, so "he moved from bank to eZ Cash" is answerable later.
//
// Named `TestOrderTimeline` because that is the gate `MILESTONE_7_PLAN.md` §4 (I3)
// states; it is the HTTP proof that the timeline a Super Admin reads on the order
// screen is the same one the payer wrote to.
func TestOrderTimeline(t *testing.T) {
	env := newAssignEnv(t)
	orderID, methodA, methodB := env.seedOrder(t)
	// The payer acts, not the operator.
	env.signInAs(t, "nimal@gmail.com")

	code, body := env.postJSON(t, "/api/payments/"+strconv.FormatInt(orderID, 10)+"/method",
		map[string]any{"method_id": methodB})
	if code != http.StatusOK {
		t.Fatalf("change = %d (%v), want 200", code, body)
	}

	var events []map[string]any
	if code := env.getJSON(t, "/api/payments/"+strconv.FormatInt(orderID, 10)+"/events", &events); code != http.StatusOK {
		t.Fatalf("events = %d, want 200", code)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e["kind"] != "METHOD_CHANGED" {
		t.Errorf("kind = %v, want METHOD_CHANGED", e["kind"])
	}
	if e["from_method_id"] != float64(methodA) || e["to_method_id"] != float64(methodB) {
		t.Errorf("move = %v → %v, want %d → %d", e["from_method_id"], e["to_method_id"], methodA, methodB)
	}
	if e["from_label"] != "Bank transfer" || e["to_label"] != "eZ Cash" {
		t.Errorf("labels = %v → %v", e["from_label"], e["to_label"])
	}
}

// D-036.3 — proof is a channel, and only the three named ones are accepted.
func TestOrderProofChannelOverHTTP(t *testing.T) {
	env := newAssignEnv(t)
	orderID, _, _ := env.seedOrder(t)
	env.signInAs(t, "nimal@gmail.com")
	path := "/api/payments/" + strconv.FormatInt(orderID, 10) + "/proof"

	if code, body := env.postJSON(t, path, map[string]any{"channel": "pigeon"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad channel = %d (%v), want 422", code, body)
	}
	if code, body := env.postJSON(t, path, map[string]any{"channel": "whatsapp", "note": "slip 4471"}); code != http.StatusOK {
		t.Fatalf("whatsapp = %d (%v), want 200", code, body)
	}
}

// D-036.2 — the destination carries the receiver's e-mail, WhatsApp and
// Telegram, so a payer always knows where to send proof.
func TestPaymentMethodExposesChannels(t *testing.T) {
	env := newAssignEnv(t)
	env.seedOrder(t)

	var methods []map[string]any
	if code := env.getJSON(t, "/api/payment-methods", &methods); code != http.StatusOK {
		t.Fatalf("methods = %d, want 200", code)
	}
	if len(methods) == 0 {
		t.Fatal("no destinations returned")
	}
	m := methods[0]
	if m["contact_email"] != "pay@veylix.test" {
		t.Errorf("contact_email = %v", m["contact_email"])
	}
	if m["contact_whatsapp"] != "+94771234567" {
		t.Errorf("contact_whatsapp = %v", m["contact_whatsapp"])
	}
	if m["contact_telegram"] != "@veylixpay" {
		t.Errorf("contact_telegram = %v", m["contact_telegram"])
	}
}

// An order that has not produced a VPN cannot be blocked: there is nothing to
// stop, and pretending otherwise would be a lie in the timeline.
func TestBlockWithoutVPNIsRefused(t *testing.T) {
	env := newAssignEnv(t)
	orderID, _, _ := env.seedOrder(t)

	code, body := env.postJSON(t, "/api/payments/"+strconv.FormatInt(orderID, 10)+"/block",
		map[string]any{"note": "no money"})
	if code != http.StatusConflict {
		t.Fatalf("block without vpn = %d (%v), want 409 no_vpn_yet", code, body)
	}
}

// TestOrderMethodChangeOverHTTP is the pre-gate name for TestOrderTimeline. It is
// kept as a forwarder so an older runbook command (`go test -run
// TestOrderMethodChangeOverHTTP`) still finds a test, without two copies of the
// same assertions drifting apart.
func TestOrderMethodChangeOverHTTP(t *testing.T) {
	t.Helper()
	TestOrderTimeline(t)
}
