package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// postQuote sends POST /api/orders/quote with a cookie-carrying client.
func (e *assignEnv) postJSON(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, e.url+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := e.http.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// putJSON sends PUT with a cookie-carrying client.
func (e *assignEnv) putJSON(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPut, e.url+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := e.http.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// D-035.4 — the Super Admin states unit rates, and a CUSTOM spec is priced by
// the SERVER. The owner's own example: 200 GB / 30 days / 3 devices / 2 IPs.
func TestQuoteFromRatesOverHTTP(t *testing.T) {
	env := newAssignEnv(t)

	code, body := env.putJSON(t, "/api/pricing", map[string]any{
		"rate_per_gb_cents":     50,
		"rate_per_day_cents":    500,
		"rate_per_device_cents": 200,
		"rate_per_ip_cents":     150,
		"min_price_cents":       10000,
		"max_traffic_gb":        500,
		"max_duration_days":     365,
		"max_devices":           10,
		"max_ips":               5,
	})
	if code != http.StatusOK {
		t.Fatalf("PUT /api/pricing = %d (%v), want 200", code, body)
	}

	code, quote := env.postJSON(t, "/api/orders/quote", map[string]any{
		"traffic_gb": 200, "days": 30, "devices": 3, "ips": 2,
	})
	if code != http.StatusOK {
		t.Fatalf("quote = %d (%v), want 200", code, quote)
	}
	if quote["total_cents"] != float64(25900) {
		t.Errorf("total_cents = %v, want 25900 (LKR 259.00)", quote["total_cents"])
	}
	if quote["currency"] != "LKR" {
		t.Errorf("currency = %v, want LKR", quote["currency"])
	}
	if quote["minimum_applied"] != false {
		t.Errorf("minimum_applied = %v, want false", quote["minimum_applied"])
	}
}

// A spec outside the stated bounds is refused with a message that names the
// field — never clamped into a sale the customer did not ask for.
func TestQuoteRefusesOutOfRangeOverHTTP(t *testing.T) {
	env := newAssignEnv(t)
	if code, _ := env.putJSON(t, "/api/pricing", map[string]any{
		"rate_per_gb_cents": 50, "max_traffic_gb": 100,
	}); code != http.StatusOK {
		t.Fatalf("seed rates = %d", code)
	}

	code, body := env.postJSON(t, "/api/orders/quote", map[string]any{"traffic_gb": 101, "days": 30})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d (%v), want 422", code, body)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "invalid_spec" {
		t.Errorf("code = %v, want invalid_spec", errObj["code"])
	}
}

// A platform with no rates yet says so, instead of quoting zero — a paid VPN
// must never become free by omission.
func TestQuoteRefusesBeforeRatesExist(t *testing.T) {
	env := newAssignEnv(t)
	if _, err := env.st.DB().Exec(`DELETE FROM pricing_rates`); err != nil {
		t.Fatalf("clear pricing: %v", err)
	}
	code, body := env.postJSON(t, "/api/orders/quote", map[string]any{"traffic_gb": 10, "days": 30})
	if code != http.StatusConflict {
		t.Fatalf("status = %d (%v), want 409 pricing_not_set", code, body)
	}
}

// GET /api/pricing is operator-only: a customer has no business reading the
// platform's wholesale rate card.
//
// The client account in this environment has no password seeded, so the check is
// made at the permission layer (a real signed-in client is covered by the RBAC
// matrix test); this asserts the route is wired to that permission rather than
// left open.
func TestPricingReadRequiresOperator(t *testing.T) {
	env := newAssignEnv(t)
	if _, err := env.st.DB().Exec(
		`UPDATE users SET role = 'client' WHERE role = 'super_admin'`); err != nil {
		t.Fatalf("demote operator: %v", err)
	}
	res, err := env.http.Get(env.url + "/api/pricing")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("non-operator GET /api/pricing = %d, want 403", res.StatusCode)
	}
}
