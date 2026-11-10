package httpapi_test

import (
	"net/http"
	"testing"
	"time"
)

// TestUsageHourlyBucketsD000 — D-092: the "usage by time" chart is built from
// per-hour DELTAS of the stored counters, never from the lifetime totals.
//
// The env seeds two snapshots an hour apart inside the same hour bucket
// (taken_at 100 and 200 both fall in hour 0). The second one's delta is
// 200 up / 500 down, so the bucket must report exactly that.
func TestUsageHourlyBucketsD000(t *testing.T) {
	env := newUsageEnv(t)
	owner := loginAs(t, env.srv.URL, "owner@example.com", "a very long password")

	resp, body := getJSON(t, owner,
		env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/usage/hourly?range=all")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hourly = %d (%v), want 200", resp.StatusCode, body)
	}
	raw, ok := body["buckets"].([]any)
	if !ok {
		t.Fatalf("buckets missing from %v", body)
	}
	if len(raw) == 0 {
		t.Fatal("buckets empty, want at least one hour")
	}
	first, _ := raw[0].(map[string]any)
	// Snapshot 1 is the first in the window and has no predecessor, so it
	// contributes 0. Snapshot 2 moved 200 up / 500 down.
	if got := first["up_bytes"]; got != float64(200) {
		t.Errorf("hour up_bytes = %v, want 200", got)
	}
	if got := first["down_bytes"]; got != float64(500) {
		t.Errorf("hour down_bytes = %v, want 500", got)
	}
	if got := first["total_bytes"]; got != float64(700) {
		t.Errorf("hour total_bytes = %v, want 700", got)
	}
	// The lifetime total is 1200; a bucket that reported it would be the
	// cumulative-vs-window confusion this endpoint exists to avoid.
	if first["total_bytes"] == body["lifetime_total_bytes"] {
		t.Error("hour total equals the lifetime total — the bucket is cumulative, not a delta")
	}
}

// TestSpeedHistoryDerivesRateFromCounters — D-092: no new table, the rate is
// the counter delta over the elapsed time between two stored readings.
//
// 100 seconds passed between the seeds (100 → 200), moving 200 up and 500 down,
// so the measured rate is 2 B/s up and 5 B/s down.
func TestSpeedHistoryDerivesRateFromCounters(t *testing.T) {
	env := newUsageEnv(t)
	owner := loginAs(t, env.srv.URL, "owner@example.com", "a very long password")

	resp, body := getJSON(t, owner,
		env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/speed-history?range=all")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("speed-history = %d (%v), want 200", resp.StatusCode, body)
	}
	if body["scope"] != "average_between_sync_readings" {
		t.Errorf("scope = %v, want average_between_sync_readings", body["scope"])
	}
	pts, _ := body["points"].([]any)
	if len(pts) != 1 {
		t.Fatalf("points = %d, want 1 (two snapshots make one interval)", len(pts))
	}
	pt, _ := pts[0].(map[string]any)
	if pt["source"] != "measured" {
		t.Fatalf("source = %v, want measured", pt["source"])
	}
	if got := pt["up_bps"]; got != float64(2) {
		t.Errorf("up_bps = %v, want 2", got)
	}
	if got := pt["down_bps"]; got != float64(5) {
		t.Errorf("down_bps = %v, want 5", got)
	}
	if got := pt["elapsed_sec"]; got != float64(100) {
		t.Errorf("elapsed_sec = %v, want 100", got)
	}
}

// TestNewUsageRoutesRefuseStrangers is the negative test every read route owes
// (security rule 1). These endpoints expose per-VPN traffic and latency, so a
// stranger must be refused and an anonymous caller must be rejected — the same
// answers `/usage` already gives.
func TestNewUsageRoutesRefuseStrangers(t *testing.T) {
	env := newUsageEnv(t)
	stranger := env.register(t, "stranger2@example.com")
	anon := &http.Client{Timeout: 10 * time.Second}
	base := env.srv.URL + "/api/vpns/" + usageID(env.vpnID)

	paths := []string{
		"/usage/hourly",
		"/speed-history",
	}
	for _, p := range paths {
		t.Run("stranger"+p, func(t *testing.T) {
			if r, body := getJSON(t, stranger, base+p); r.StatusCode != http.StatusForbidden {
				t.Errorf("%s as a stranger = %d (%v), want 403", p, r.StatusCode, body)
			}
		})
		t.Run("anonymous"+p, func(t *testing.T) {
			if r, _ := getJSON(t, anon, base+p); r.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s unauthenticated = %d, want 401", p, r.StatusCode)
			}
		})
	}
}

// TestNewUsageRoutesRefuseMisspelledRange proves the new endpoints validate the
// window exactly as `/usage` does: a typo answers 422 instead of quietly
// showing a different period.
func TestNewUsageRoutesRefuseMisspelledRange(t *testing.T) {
	env := newUsageEnv(t)
	owner := loginAs(t, env.srv.URL, "owner@example.com", "a very long password")
	base := env.srv.URL + "/api/vpns/" + usageID(env.vpnID)

	for _, p := range []string{"/usage/hourly", "/speed-history"} {
		if r, body := getJSON(t, owner, base+p+"?range=bogus"); r.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s?range=bogus = %d (%v), want 422", p, r.StatusCode, body)
		}
	}
}

// TestPingHistoryRequiresSampleStore documents the wiring contract: without a
// PingSampleStore the route is not registered at all (404), which is honest —
// there is no latency history to serve. It must never answer 200 with an empty
// list, because that would look like "measured, and nothing happened".
func TestPingHistoryRequiresSampleStore(t *testing.T) {
	env := newUsageEnv(t) // deliberately built without PingSamples
	owner := loginAs(t, env.srv.URL, "owner@example.com", "a very long password")

	r, _ := getJSON(t, owner,
		env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/ping/history")
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("ping/history without a sample store = %d, want 404", r.StatusCode)
	}
}
