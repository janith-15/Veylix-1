package httpapi_test

import (
	"net/http"
	"testing"
)

// TestVPNLinksAndPingScope proves the two endpoints that let a client CONNECT
// respect ownership, and that an unreachable panel is never reported as "this
// VPN has no links".
//
// Step 11 of Milestone 5-B. Before it, GET /api/vpns/{id}/links answered 404 and
// the client screen showed a demo QR code: the product's core promise was
// unimplemented.
func TestVPNLinksAndPingScope(t *testing.T) {
	fake := &fakePingService{}
	env := newConnectionEnv(t, fake)
	root := env.root(t)

	// A client with a VPN, another client with their own.
	owner := env.signup(t, "link-owner@example.com")
	env.signup(t, "link-stranger@example.com")

	vpnID := env.seedVPN(t, owner.id, 9301)

	// The owner reads their own links.
	resp, info := getJSON(t, owner.client, env.srv.URL+"/api/vpns/"+usageID(vpnID)+"/links")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner links = %d (%v), want 200", resp.StatusCode, info)
	}
	links, _ := info["links"].([]any)
	if len(links) != 1 || links[0] != "vless://owner@panel" {
		t.Fatalf("owner links = %v, want the panel's own URI", info["links"])
	}
	if info["source"] != "panel" {
		t.Fatalf("source = %v, want panel", info["source"])
	}

	// A stranger gets 404, not 403: ids must not be probeable.
	stranger := loginAs(t, env.srv.URL, "link-stranger@example.com", "a very long password")
	resp, _ = getJSON(t, stranger, env.srv.URL+"/api/vpns/"+usageID(vpnID)+"/links")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("stranger links = %d, want 404", resp.StatusCode)
	}

	// An unreachable panel is NOT "no links exist" — the source says so.
	fake.linkErr = true
	resp, info = getJSON(t, owner.client, env.srv.URL+"/api/vpns/"+usageID(vpnID)+"/links")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("links during outage = %d, want 200", resp.StatusCode)
	}
	if info["source"] != "unavailable" {
		t.Fatalf("source during outage = %v, want unavailable", info["source"])
	}
	fake.linkErr = false

	// Ping reports the SERVER's reachability, labelled as such.
	fake.pingReachable = true
	fake.pingMS = 42
	resp, ping := postJSON(t, owner.client, env.srv.URL+"/api/vpns/"+usageID(vpnID)+"/ping", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ping = %d (%v), want 200", resp.StatusCode, ping)
	}
	if ping["scope"] != "server_reachability" {
		t.Fatalf("ping scope = %v, want server_reachability", ping["scope"])
	}
	if ping["latency_ms"].(float64) != 42 {
		t.Fatalf("latency = %v, want 42", ping["latency_ms"])
	}

	// An unreachable server reports -1, never 0 (0 would look like "instant").
	fake.pingReachable = false
	_, ping = postJSON(t, owner.client, env.srv.URL+"/api/vpns/"+usageID(vpnID)+"/ping", map[string]any{})
	if ping["latency_ms"].(float64) != -1 || ping["reachable"] != false {
		t.Fatalf("unreachable ping = %v, want reachable=false / latency=-1", ping)
	}

	// A stranger cannot ping either.
	resp, _ = postJSON(t, stranger, env.srv.URL+"/api/vpns/"+usageID(vpnID)+"/ping", map[string]any{})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("stranger ping = %d, want 404", resp.StatusCode)
	}
	_ = root
}
