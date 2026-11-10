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
	//
	// Since D-096 the history is served in time buckets, and the two seeded
	// snapshots (taken_at 100 and 200) sit inside the SAME bucket. So this is one
	// point, and its `total_bytes` is what MOVED between the two readings — the
	// 200 up / 500 down delta, not the first reading's 500. The first reading has
	// no predecessor in the window, so it contributes no delta; the lifetime
	// totals above are what carry the absolute figure.
	points, _ := body["points"].([]any)
	if len(points) != 1 {
		t.Fatalf("points = %d, want 1 (both snapshots fall in one bucket)", len(points))
	}
	first, _ := points[0].(map[string]any)
	if first["total_bytes"] != float64(700) {
		t.Errorf("first point total = %v, want 700 (200 up + 500 down moved)", first["total_bytes"])
	}
	// The bucket must state the period it really covers, and must be more than a
	// single day because the seeded span is under two days (D-096).
	if win["bucket"] == nil || win["bucket"] == "" {
		t.Errorf("window.bucket = %v, want a named bucket width", win["bucket"])
	}
	if bw, _ := win["bucket_sec"].(float64); bw <= 0 {
		t.Errorf("window.bucket_sec = %v, want a positive width", win["bucket_sec"])
	}
}

// A range selector that returns the same picture for every range is a lie about
// its own window. This is the regression that made every range show "Last 30
// days" over an 18-hour slice (D-096).
func TestUsageRangesDifferFromEachOther(t *testing.T) {
	env := newUsageEnv(t)
	owner := loginAs(t, env.srv.URL, "owner@example.com", "a very long password")

	seen := map[string]float64{}
	for _, r := range []string{"24h", "7d", "30d", "3m", "1y", "all"} {
		resp, body := getJSON(t, owner,
			env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/usage?range="+r)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("usage?range=%s = %d (%v), want 200", r, resp.StatusCode, body)
		}
		win, _ := body["window"].(map[string]any)
		bw, _ := win["bucket_sec"].(float64)
		if bw <= 0 {
			t.Errorf("range %s: bucket_sec = %v, want a positive width", r, win["bucket_sec"])
		}
		seen[r] = bw
	}

	// The named ranges must not all choose the same width: 24h is hourly, 1y is
	// monthly. If they collapse to one value the selector does nothing.
	if seen["24h"] >= seen["30d"] || seen["30d"] >= seen["1y"] {
		t.Errorf("bucket widths do not grow with the window: %v", seen)
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
