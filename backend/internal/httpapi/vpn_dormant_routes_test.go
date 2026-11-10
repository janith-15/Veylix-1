package httpapi_test

import (
	"net/http"
	"testing"
	"time"
)

// sleepState reads the two facts that must move together: the dormant stamp and
// the panel-derived status. They are read from the database rather than from an
// API response so the test cannot pass on a projection that disagrees with the
// stored truth.
func (e *connectionEnv) sleepState(t *testing.T, vpnID int64) (dormant bool, status string) {
	t.Helper()
	var stamp *int64
	if err := e.st.DB().QueryRow(
		`SELECT dormant_at, status FROM vpn_instances WHERE id = ?`, vpnID).
		Scan(&stamp, &status); err != nil {
		t.Fatalf("read dormancy: %v", err)
	}
	return stamp != nil, status
}

// TestDormantRoutePutsVPNToSleep is the happy path of D-094.
//
// The claim being proved is deliberately narrow, because a wider one would be a
// lie: dormancy changes what the CUSTOMER is shown, and nothing else. So the
// test asserts that the stamp was written AND that the status did not move —
// a dormant VPN must not be able to pretend its panel state changed.
func TestDormantRoutePutsVPNToSleep(t *testing.T) {
	env := newConnectionEnv(t, &fakePingService{})
	owner := env.signup(t, "sleeper@example.com")
	vpnID := env.seedVPN(t, owner.id, 601)

	resp, body := postJSON(t, owner.client, env.srv.URL+"/api/vpns/601/dormant", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("dormant = %d (%v), want 204 (vpnAction answers a state change with no body)", resp.StatusCode, body)
	}

	dormant, status := env.sleepState(t, vpnID)
	if !dormant {
		t.Error("dormant_at was not written, so the VPN would still sort first and keep polling")
	}
	if status != "ACTIVE" {
		t.Errorf("status = %s, want ACTIVE untouched — dormancy is not a status", status)
	}

	// The row must still be LISTED. Dormancy hides a VPN from attention, never
	// from its owner's list (INV-1 read backwards: nothing a customer owns may
	// silently disappear), and the stamp must reach the wire so the client can
	// sort it last and stop polling it.
	rows := getJSONArray(t, owner.client, env.srv.URL+"/api/vpns")
	if len(rows) == 0 {
		t.Fatal("list returned no rows at all; the owner cannot see their own VPN")
	}
	for _, row := range rows {
		if id, _ := row["id"].(float64); int64(id) != vpnID {
			continue
		}
		if row["dormant_at"] == nil {
			t.Error("listed row has no dormant_at, so the client cannot sort or stop polling it")
		}
		return
	}
	t.Error("a dormant VPN vanished from the owner's list; dormancy must not hide it from its owner")
}

// TestWakeRouteClearsDormancy proves the inverse, and that waking does NOT
// invent a healthy status.
func TestWakeRouteClearsDormancy(t *testing.T) {
	env := newConnectionEnv(t, &fakePingService{})
	owner := env.signup(t, "waker@example.com")
	vpnID := env.seedVPN(t, owner.id, 602)

	if resp, _ := postJSON(t, owner.client, env.srv.URL+"/api/vpns/602/dormant", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("dormant = %d", resp.StatusCode)
	}
	resp, body := postJSON(t, owner.client, env.srv.URL+"/api/vpns/602/wake", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("wake = %d (%v), want 204 (vpnAction answers a state change with no body)", resp.StatusCode, body)
	}
	if dormant, _ := env.sleepState(t, vpnID); dormant {
		t.Error("dormant_at survived wake, so the VPN would stay asleep for ever")
	}
}

// mustStamp reads the stored dormant stamp, failing if it is absent.
func (e *connectionEnv) mustStamp(t *testing.T, vpnID int64) int64 {
	t.Helper()
	var stamp *int64
	if err := e.st.DB().QueryRow(
		`SELECT dormant_at FROM vpn_instances WHERE id = ?`, vpnID).Scan(&stamp); err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	if stamp == nil {
		t.Fatalf("vpn %d has no dormant_at", vpnID)
	}
	return *stamp
}

// TestDormantIsIdempotent proves a double-click does not reset "asleep since".
// The stamp is shown to the customer, so silently rewriting it would be a small
// lie about when they acted.
func TestDormantIsIdempotent(t *testing.T) {
	env := newConnectionEnv(t, &fakePingService{})
	owner := env.signup(t, "double@example.com")
	vpnID := env.seedVPN(t, owner.id, 603)

	if resp, _ := postJSON(t, owner.client, env.srv.URL+"/api/vpns/603/dormant", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first dormant failed")
	}
	// Pin a known stamp: if a second call rewrote the column, this value is
	// replaced and the assertion below catches it.
	if _, err := env.st.DB().Exec(
		`UPDATE vpn_instances SET dormant_at = 1600000000 WHERE id = ?`, vpnID); err != nil {
		t.Fatalf("restamp: %v", err)
	}
	if resp, _ := postJSON(t, owner.client, env.srv.URL+"/api/vpns/603/dormant", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("second dormant failed")
	}
	if got := env.mustStamp(t, vpnID); got != 1600000000 {
		t.Errorf("dormant_at = %d, want the original 1600000000 kept (idempotent)", got)
	}
}

// TestDormantRouteRefusesOtherOwnersVPN is the ownership negative case. A VPN
// belonging to somebody else must be indistinguishable from one that does not
// exist, or the route becomes an id probe.
func TestDormantRouteRefusesOtherOwnersVPN(t *testing.T) {
	env := newConnectionEnv(t, &fakePingService{})
	victim := env.signup(t, "victim@example.com")
	attacker := env.signup(t, "attacker@example.com")
	vpnID := env.seedVPN(t, victim.id, 604)

	resp, body := postJSON(t, attacker.client, env.srv.URL+"/api/vpns/604/dormant", nil)
	if resp.StatusCode == http.StatusNoContent {
		t.Fatalf("a stranger put someone else's VPN to sleep (200): %v", body)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("dormant on a foreign VPN = %d, want 404 so ids cannot be probed", resp.StatusCode)
	}
	if dormant, _ := env.sleepState(t, vpnID); dormant {
		t.Error("dormant_at was written by a caller who does not own the VPN")
	}
}

// TestDormantRouteRefusesAnonymous is the unauthenticated negative case.
func TestDormantRouteRefusesAnonymous(t *testing.T) {
	env := newConnectionEnv(t, &fakePingService{})
	owner := env.signup(t, "anon-target@example.com")
	vpnID := env.seedVPN(t, owner.id, 605)

	anon := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	resp, _ := postJSON(t, anon, env.srv.URL+"/api/vpns/605/dormant", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous dormant = %d, want 401", resp.StatusCode)
	}
	if dormant, _ := env.sleepState(t, vpnID); dormant {
		t.Error("dormant_at was written without a session")
	}
}
