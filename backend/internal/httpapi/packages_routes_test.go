package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// getJSON performs an authenticated GET and decodes the body into out.
func (e *assignEnv) getJSON(t *testing.T, path string, out any) int {
	t.Helper()
	res, err := e.http.Get(e.url + path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	_ = json.NewDecoder(res.Body).Decode(out)
	return res.StatusCode
}

// seedPackage inserts one package row.
func (e *assignEnv) seedPackage(t *testing.T, name, kind string, custom bool, price int64) {
	t.Helper()
	c := 0
	if custom {
		c = 1
	}
	if _, err := e.st.DB().Exec(
		`INSERT INTO vpn_offers (kind, name, server_id, inbound_id, traffic_gb, duration_days,
		                         device_limit, active, price_cents, currency, is_custom, created_at, updated_at)
		 VALUES (?, ?, ?, NULL, 50, 30, 2, 1, ?, 'LKR', ?, 1, 1)`,
		kind, name, e.server, price, c); err != nil {
		t.Fatalf("seed package: %v", err)
	}
}

// D-035.2 — a customer's catalogue shows all three kinds, each labelled, and a
// CUSTOM package is marked as priced per order rather than at a fixed amount.
func TestClientSeesAllPremiumPackages(t *testing.T) {
	env := newAssignEnv(t)
	env.seedPackage(t, "Free 10", "FREE", false, 0)
	env.seedPackage(t, "Premium 50", "PREMIUM", false, 150000)
	env.seedPackage(t, "Build your own", "PREMIUM", true, 0)

	// Make the signed-in account a customer, so RBAC treats it as one.
	if _, err := env.st.DB().Exec(
		`UPDATE users SET role = 'client' WHERE role = 'super_admin'`); err != nil {
		t.Fatalf("demote: %v", err)
	}

	var out []map[string]any
	code := env.getJSON(t, "/api/packages", &out)
	if code != http.StatusOK {
		t.Fatalf("GET /api/packages = %d, want 200", code)
	}
	if len(out) != 3 {
		t.Fatalf("packages = %d, want 3 (free, premium, custom)", len(out))
	}

	kinds := map[string]map[string]any{}
	for _, p := range out {
		k, _ := p["kind"].(string)
		kinds[k] = p
	}
	for _, k := range []string{"FREE", "PREMIUM", "CUSTOM"} {
		if _, ok := kinds[k]; !ok {
			t.Errorf("kind %s is missing from the catalogue", k)
		}
	}
	if kinds["PREMIUM"]["price_cents"] != float64(150000) {
		t.Errorf("premium price = %v, want 150000", kinds["PREMIUM"]["price_cents"])
	}
	if kinds["CUSTOM"]["priced_per_order"] != true {
		t.Error("a custom package must be marked priced_per_order — its amount comes from /api/orders/quote")
	}
	// A customer must never see reseller economics.
	for _, p := range out {
		for _, forbidden := range []string{"owner_user_id", "slots_used", "slots_total"} {
			if _, ok := p[forbidden]; ok {
				t.Errorf("package leaked %q to a customer", forbidden)
			}
		}
	}
}

// D-111 — the operator package list must carry the fields the screen renders.
//
// `GET /api/plans` used to serialise `store.Plan` straight out. That struct has
// no `server_label`, `slots_remaining` or `purchasable` — they are DERIVED — so
// all three arrived as `undefined` while `ApiPlan` in lib/api.ts declares them
// required. The Packages screen therefore rendered **"no server"** on every row,
// including rows whose `server_id` was set correctly, and the slots column fell
// back to `∞`. TypeScript cannot catch a promise the server does not keep, which
// is exactly why this is pinned here.
func TestPlanListCarriesTheRenderedFields(t *testing.T) {
	env := newAssignEnv(t)
	env.seedPackage(t, "Premium 50", "PREMIUM", false, 150000)

	var out []map[string]any
	code := env.getJSON(t, "/api/plans", &out)
	if code != http.StatusOK {
		t.Fatalf("GET /api/plans = %d, want 200", code)
	}
	if len(out) != 1 {
		t.Fatalf("plans = %d, want 1", len(out))
	}
	p := out[0]

	// The row the screen prints under the package name.
	label, ok := p["server_label"].(string)
	if !ok || label == "" {
		t.Fatalf("server_label = %#v, want a non-empty string — the Packages screen "+
			"falls back to \"no server\" and the operator cannot tell where it sells from", p["server_label"])
	}
	want := fmt.Sprintf("server %d", env.server)
	if label != want {
		t.Errorf("server_label = %q, want %q", label, want)
	}

	// The fields `ApiPlan` declares as required must all be present, so the
	// declared contract and the wire contract cannot drift apart again.
	for _, key := range []string{"slots_remaining", "purchasable", "active", "archived"} {
		if _, present := p[key]; !present {
			t.Errorf("%q is missing from GET /api/plans — ApiPlan declares it required", key)
		}
	}

	// The operator view may carry operator facts; it must never carry a secret.
	for _, forbidden := range []string{"token_ciphertext", "panel_url", "password_hash"} {
		if _, leaked := p[forbidden]; leaked {
			t.Errorf("GET /api/plans leaked %q", forbidden)
		}
	}
}

// D-035.2 — "adminlata custom premium kiyala ekak na." A reseller cannot create
// a custom package, and a custom package can never be FREE.
func TestResellerCannotOwnCustom(t *testing.T) {
	env := newAssignEnv(t)

	// As the Super Admin: a FREE custom package is refused.
	code, body := env.postJSON(t, "/api/plans", map[string]any{
		"kind": "FREE", "name": "Custom free", "server_id": env.server, "is_custom": true,
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("FREE custom = %d (%v), want 422", code, body)
	}

	// As a reseller: any custom package is refused. The refusal arrives at the
	// permission layer (`offer:manage` is Super-Admin-only), which is stronger
	// than my field check — a reseller cannot create ANY package, custom or not.
	if _, err := env.st.DB().Exec(
		`UPDATE users SET role = 'admin' WHERE role = 'super_admin'`); err != nil {
		t.Fatalf("demote: %v", err)
	}
	code, body = env.postJSON(t, "/api/plans", map[string]any{
		"kind": "PREMIUM", "name": "Custom premium", "server_id": env.server, "is_custom": true,
	})
	if code != http.StatusForbidden {
		t.Fatalf("reseller custom = %d (%v), want 403", code, body)
	}
	// And the ordinary package case is refused for the same reason, so the
	// custom rule is not the only thing standing in the way.
	code, _ = env.postJSON(t, "/api/plans", map[string]any{
		"kind": "PREMIUM", "name": "Normal premium", "server_id": env.server,
	})
	if code != http.StatusForbidden {
		t.Fatalf("reseller plain package = %d, want 403 (offer:manage is SA-only)", code)
	}
}
