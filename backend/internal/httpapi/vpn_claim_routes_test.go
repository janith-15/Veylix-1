package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"veylix/backend/internal/store"
)

// The 3-concurrent cap (D-025.4) must be enforced on the REAL create path, not
// only in the store: a client claiming a fourth FREE VPN on one server is
// refused with 409 free_concurrent_limit and no VPN is created.
func TestFreeConcurrentCapIsEnforced(t *testing.T) {
	env := newVPNEnv(t, "FREE")
	c := env.signup(t, "client@example.com", "a very long password")

	// Pre-tag every panel identity the claims will receive, so the list/cap
	// path is the only thing under test.
	var serverID int64
	if err := env.st.DB().QueryRow(`SELECT id FROM server_integrations`).Scan(&serverID); err != nil {
		t.Fatalf("read server: %v", err)
	}

	for i := 0; i < store.FreeConcurrentCap; i++ {
		if err := env.st.SetClientTag(context.Background(), serverID,
			"vyx-"+itoa(int64(i+1)), store.TagFree, 0); err != nil {
			t.Fatalf("pre-tag %d: %v", i+1, err)
		}
		code, body := env.post(t, c, "/api/vpns", map[string]any{"offer_id": env.offer})
		if code != http.StatusCreated {
			t.Fatalf("claim %d = %d (%v), want 201", i+1, code, body)
		}
	}

	// The fourth claim exceeds the concurrent cap.
	code, body := env.post(t, c, "/api/vpns", map[string]any{"offer_id": env.offer})
	if code != http.StatusConflict {
		t.Fatalf("4th claim = %d (%v), want 409", code, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if errBody == nil || errBody["code"] != "free_concurrent_limit" {
		t.Fatalf("error = %v, want code free_concurrent_limit", body)
	}

	// Nothing was left behind by the refused claim: 3 VPNs, 3 claims.
	var vpns, claims int
	if err := env.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances`).Scan(&vpns); err != nil {
		t.Fatalf("count vpns: %v", err)
	}
	if err := env.st.DB().QueryRow(`SELECT COUNT(*) FROM free_claims`).Scan(&claims); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if vpns != store.FreeConcurrentCap || claims != store.FreeConcurrentCap {
		t.Fatalf("after refusal: %d vpns / %d claims, want %d/%d",
			vpns, claims, store.FreeConcurrentCap, store.FreeConcurrentCap)
	}
}

// The caps must be readable BEFORE claiming (REQUIREMENTS §2.0.1), because
// releasing does not refund a slot — a client has to see what they have left.
func TestFreeClaimUsageIsReadable(t *testing.T) {
	env := newVPNEnv(t, "FREE")
	c := env.signup(t, "client@example.com", "a very long password")

	var serverID int64
	if err := env.st.DB().QueryRow(`SELECT id FROM server_integrations`).Scan(&serverID); err != nil {
		t.Fatalf("read server: %v", err)
	}

	// Before any claim.
	code, body := env.get(t, c, "/api/vpns/claims/me?server_id="+itoa(serverID))
	if code != http.StatusOK {
		t.Fatalf("claims/me = %d (%v), want 200", code, body)
	}
	if body["concurrent"] != float64(0) || body["used_this_window"] != float64(0) {
		t.Fatalf("fresh usage = %v, want 0/0", body)
	}
	if body["concurrent_limit"] != float64(store.FreeConcurrentCap) ||
		body["monthly_limit"] != float64(store.FreeMonthlyDefault) {
		t.Fatalf("limits = %v, want %d concurrent / %d monthly", body,
			store.FreeConcurrentCap, store.FreeMonthlyDefault)
	}

	// After one claim.
	if err := env.st.SetClientTag(context.Background(), serverID, "vyx-1", store.TagFree, 0); err != nil {
		t.Fatalf("pre-tag: %v", err)
	}
	if code, b := env.post(t, c, "/api/vpns", map[string]any{"offer_id": env.offer}); code != http.StatusCreated {
		t.Fatalf("claim = %d (%v)", code, b)
	}
	_, body = env.get(t, c, "/api/vpns/claims/me?server_id="+itoa(serverID))
	if body["concurrent"] != float64(1) || body["used_this_window"] != float64(1) {
		t.Fatalf("after one claim = %v, want 1/1", body)
	}

	// server_id is required: the caps are per server, so no server means no answer.
	if code, _ := env.get(t, c, "/api/vpns/claims/me"); code != http.StatusUnprocessableEntity {
		t.Fatalf("claims/me without server_id = %d, want 422", code)
	}
}

// A FREE claim taken by a create the panel REFUSED must be given back: the
// client never got a VPN, so charging them a monthly slot would be wrong.
func TestRefusedCreateRefundsTheFreeClaim(t *testing.T) {
	env := newVPNEnv(t, "FREE")
	c := env.signup(t, "client@example.com", "a very long password")

	var serverID int64
	if err := env.st.DB().QueryRow(`SELECT id FROM server_integrations`).Scan(&serverID); err != nil {
		t.Fatalf("read server: %v", err)
	}
	if err := env.st.SetClientTag(context.Background(), serverID, "vyx-1", store.TagFree, 0); err != nil {
		t.Fatalf("pre-tag: %v", err)
	}

	env.stub.reject = true
	if code, b := env.post(t, c, "/api/vpns", map[string]any{"offer_id": env.offer}); code != http.StatusBadGateway {
		t.Fatalf("refused create = %d (%v), want 502", code, b)
	}

	var claims int
	if err := env.st.DB().QueryRow(`SELECT COUNT(*) FROM free_claims`).Scan(&claims); err != nil {
		t.Fatalf("count claims: %v", err)
	}
	if claims != 0 {
		t.Fatalf("claims after a refused create = %d, want 0 (the slot must come back)", claims)
	}
}
