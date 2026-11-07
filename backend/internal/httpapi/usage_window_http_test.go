package httpapi_test

import (
	"net/http"
	"testing"
)

// P-01 / P-02 / P-06 — the usage read must carry up/down/TOTAL separately, the
// percentage used, and the range it actually covers.
func TestUsageHistorySplitsUpDownTotal(t *testing.T) {
	env := newUsageEnv(t)
	owner := loginAs(t, env.srv.URL, "owner@example.com", "a very long password")

	resp, body := getJSON(t, owner,
		env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/usage?range=all")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("usage = %d (%v), want 200", resp.StatusCode, body)
	}

	// Totals: 300 up + 900 down = 1200 against a 1000-byte allowance (clamped).
	if body["lifetime_up_bytes"] != float64(300) {
		t.Errorf("up = %v, want 300", body["lifetime_up_bytes"])
	}
	if body["lifetime_down_bytes"] != float64(900) {
		t.Errorf("down = %v, want 900", body["lifetime_down_bytes"])
	}
	if body["lifetime_total_bytes"] != float64(1200) {
		t.Errorf("total = %v, want 1200 (up + down)", body["lifetime_total_bytes"])
	}
	if body["usage_pct"] != float64(100) {
		t.Errorf("usage_pct = %v, want 100 (clamped, never 120)", body["usage_pct"])
	}
	if body["remaining_bytes"] != float64(0) {
		t.Errorf("remaining = %v, want 0", body["remaining_bytes"])
	}
	// The window is echoed, so a chart can label itself (D-026.2).
	win, _ := body["window"].(map[string]any)
	if win["key"] != "all" {
		t.Errorf("window = %v, want key all", body["window"])
	}

	// Every point carries its own total too.
	points, _ := body["points"].([]any)
	if len(points) != 2 {
		t.Fatalf("points = %d, want 2", len(points))
	}
	first, _ := points[0].(map[string]any)
	if first["total_bytes"] != float64(500) {
		t.Errorf("first point total = %v, want 500 (100 up + 400 down)", first["total_bytes"])
	}
}

// An unknown range is refused instead of quietly showing a different window.
func TestUsageRangeTypoIsRefused(t *testing.T) {
	env := newUsageEnv(t)
	owner := loginAs(t, env.srv.URL, "owner@example.com", "a very long password")

	resp, body := getJSON(t, owner,
		env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/usage?range=2w")
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("usage?range=2w = %d (%v), want 422", resp.StatusCode, body)
	}
}
