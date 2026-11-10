package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/store"
)

// offerEnv is a real router over real SQLite with one server + offers seeded.
type offerEnv struct {
	srv       *httptest.Server
	serverID  int64
	inboundID int64
	freeID    int64
	premID    int64
}

// newOfferEnv seeds a server, one active FREE offer and one PREMIUM offer
// (which a client must never see).
func newOfferEnv(t *testing.T) *offerEnv {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/offers.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db := st.DB()

	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	serverID := exec(`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv', 'https://panel.example/base', x'00', 'ACTIVE', 1, 1)`)
	// D-075 requires an offer to name the inbound it provisions on, so an offer
	// that cannot build a client is refused instead of saved. The inbound is a
	// foreign key, so the fixture seeds a real row rather than a bare number.
	inboundID := exec(`INSERT INTO inbounds (server_id, remote_id, remark, tag, protocol, port, enabled, synced_at)
		VALUES (?, 1, 'vless-443', 'vless', 'vless', 443, 1, 1)`, serverID)
	freeID := exec(`INSERT INTO vpn_offers (kind, name, server_id, inbound_id, traffic_gb, duration_days, device_limit, active, created_at, updated_at)
		VALUES ('FREE', 'Public Free', ?, ?, 10, 30, 1, 1, 1, 1)`, serverID, inboundID)
	premID := exec(`INSERT INTO vpn_offers (kind, name, server_id, inbound_id, traffic_gb, duration_days, device_limit, active, created_at, updated_at)
		VALUES ('PREMIUM', 'Reseller Premium', ?, ?, 100, 30, 3, 1, 1, 1)`, serverID, inboundID)

	auditLog := audit.New(db)
	authSvc := auth.NewService(db, auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(db, false)

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		Offers:     st,
		OfferAudit: auditLog,
	}))
	t.Cleanup(srv.Close)

	return &offerEnv{srv: srv, serverID: serverID, inboundID: inboundID, freeID: freeID, premID: premID}
}

// superAdmin bootstraps the first account and returns a signed-in client.
func (e *offerEnv) superAdmin(t *testing.T) *http.Client {
	t.Helper()
	return bootstrapSuperAdmin(t, e.srv.URL)
}

// client registers a plain client account and returns it signed in.
func (e *offerEnv) client(t *testing.T, email string) *http.Client {
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

// TestOffersClientSeesOnlyFree proves the public catalogue rule: a client may
// never see a PREMIUM offer, even though the endpoint is authenticated.
func TestOffersClientSeesOnlyFree(t *testing.T) {
	env := newOfferEnv(t)
	c := env.client(t, "buyer@example.com")

	resp, _ := getJSON(t, c, env.srv.URL+"/api/offers")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("client GET /api/offers = %d, want 200", resp.StatusCode)
	}

	rows := getJSONArray(t, c, env.srv.URL+"/api/offers")
	if len(rows) != 1 {
		t.Fatalf("client saw %d offers, want exactly the 1 FREE one", len(rows))
	}
	if rows[0]["kind"] != "FREE" {
		t.Errorf("kind = %v, want FREE (a client must never see a premium offer)", rows[0]["kind"])
	}
}

// TestOffersSuperAdminSeesBoth proves the owner sees the whole catalogue.
func TestOffersSuperAdminSeesBoth(t *testing.T) {
	env := newOfferEnv(t)
	root := env.superAdmin(t)

	rows := getJSONArray(t, root, env.srv.URL+"/api/offers")
	if len(rows) != 2 {
		t.Fatalf("super admin saw %d offers, want 2", len(rows))
	}
}

// TestOffersAnonymousIs401 keeps the catalogue behind authentication.
func TestOffersAnonymousIs401(t *testing.T) {
	env := newOfferEnv(t)
	anon := &http.Client{Jar: mustJar(t)}

	if resp, _ := getJSON(t, anon, env.srv.URL+"/api/offers"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous GET /api/offers = %d, want 401", resp.StatusCode)
	}
}

// TestOffersCreateRequiresSuperAdmin proves a client cannot mint an offer.
func TestOffersCreateRequiresSuperAdmin(t *testing.T) {
	env := newOfferEnv(t)
	c := env.client(t, "sneaky@example.com")

	resp, _ := postJSON(t, c, env.srv.URL+"/api/offers", map[string]any{
		"kind": "FREE", "name": "Free stuff", "server_id": env.serverID,
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("client creating an offer = %d, want 403", resp.StatusCode)
	}
}

// TestOffersCreateValidates covers the fields that would otherwise produce an
// offer nobody can provision from.
func TestOffersCreateValidates(t *testing.T) {
	env := newOfferEnv(t)
	root := env.superAdmin(t)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"bad kind", map[string]any{"kind": "GOLD", "name": "x", "server_id": env.serverID}},
		{"blank name", map[string]any{"kind": "FREE", "name": "   ", "server_id": env.serverID}},
		{"no server", map[string]any{"kind": "FREE", "name": "No server"}},
		{"negative limit", map[string]any{"kind": "FREE", "name": "Neg", "server_id": env.serverID, "traffic_gb": -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := postJSON(t, root, env.srv.URL+"/api/offers", tc.body)
			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Errorf("%s = %d (%v), want 422", tc.name, resp.StatusCode, body)
			}
		})
	}
}

// TestOffersCreateAndPatch is the happy path: create, then deactivate.
//
// The body carries an `inbound_id` because D-075 made it REQUIRED: an offer with
// no inbound can never build a client, so it is refused with 422 offer_incomplete
// rather than being saved and silently stranding every customer who claims it.
// This fixture predated the rule, which is why the suite was red.
func TestOffersCreateAndPatch(t *testing.T) {
	env := newOfferEnv(t)
	root := env.superAdmin(t)

	resp, body := postJSON(t, root, env.srv.URL+"/api/offers", map[string]any{
		"kind": "free", "name": "  Weekend Free  ", "server_id": env.serverID,
		"inbound_id":    env.inboundID,
		"traffic_gb":    5,
		"duration_days": 7,
		"device_limit":  2,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d (%v), want 201", resp.StatusCode, body)
	}
	if body["kind"] != "FREE" {
		t.Errorf("kind = %v, want FREE (normalised)", body["kind"])
	}
	if body["name"] != "Weekend Free" {
		t.Errorf("name = %q, want the trimmed value", body["name"])
	}
	if body["active"] != true {
		t.Errorf("active = %v, want true by default", body["active"])
	}

	id := int64(body["id"].(float64))
	presp, pbody := patchJSON(t, root, env.srv.URL+"/api/offers/"+usageID(id), map[string]any{"active": false})
	if presp.StatusCode != http.StatusOK {
		t.Fatalf("patch = %d (%v), want 200", presp.StatusCode, pbody)
	}
	if pbody["active"] != false {
		t.Errorf("active = %v, want false", pbody["active"])
	}
}

// TestOffersPatchUnknownIs404 avoids leaking existence via another code.
func TestOffersPatchUnknownIs404(t *testing.T) {
	env := newOfferEnv(t)
	root := env.superAdmin(t)

	resp, _ := patchJSON(t, root, env.srv.URL+"/api/offers/99999", map[string]any{"active": false})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("patch unknown offer = %d, want 404", resp.StatusCode)
	}
}

// TestOffersDeactivateHidesFromClient proves deactivating really removes it
// from the public catalogue — the reason `active` exists.
func TestOffersDeactivateHidesFromClient(t *testing.T) {
	env := newOfferEnv(t)
	root := env.superAdmin(t)
	client := env.client(t, "late@example.com")

	presp, _ := patchJSON(t, root, env.srv.URL+"/api/offers/"+usageID(env.freeID), map[string]any{"active": false})
	if presp.StatusCode != http.StatusOK {
		t.Fatalf("deactivate = %d", presp.StatusCode)
	}

	if rows := getJSONArray(t, client, env.srv.URL+"/api/offers"); len(rows) != 0 {
		t.Errorf("client still sees %d offers after deactivation, want 0", len(rows))
	}
}
