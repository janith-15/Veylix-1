package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/store"
)

// usageEnv is a real router over real SQLite with usage history seeded.
type usageEnv struct {
	srv      *httptest.Server
	db       *sql.DB
	vpnID    int64
	serverID int64
}

func usageID(v int64) string { return strconv.FormatInt(v, 10) }

// getJSONArray reads a JSON array response (the shared helpers in
// auth_e2e_test.go decode objects only).
func getJSONArray(t *testing.T, c *http.Client, url string) []map[string]any {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d (the route may not be mounted)", url, resp.StatusCode)
	}
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode array from %s: %v", url, err)
	}
	return out
}

// patchJSON sends a PATCH with a JSON body (postJSON only does POST).
func patchJSON(t *testing.T, c *http.Client, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build PATCH %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// putJSON sends a PUT with a JSON body.
func putJSON(t *testing.T, c *http.Client, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build PUT %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// newUsageEnv seeds one client ("owner") with a real argon2id password, one
// VPN they own with two snapshots, and one server metric sample.
func newUsageEnv(t *testing.T) *usageEnv {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/usage.db")
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

	// A real hash so the owner can actually sign in. Cached per process: argon2id
	// is deliberately slow and this test must not pay for it every run.
	hash := testPasswordHash(t, "a very long password")
	ownerID := exec(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('owner@example.com', ?, 'Owner', 'client', 'ACTIVE', 1, 1, 1)`, hash)
	serverID := exec(`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv', 'https://panel.example/base', x'00', 'ACTIVE', 1, 1)`)
	vpnID := exec(`INSERT INTO vpn_instances
		(owner_user_id, server_id, panel_email, panel_sub_id, status, traffic_limit_bytes,
		 lifetime_up_bytes, lifetime_down_bytes, created_at, updated_at)
		VALUES (?, ?, 'own@panel.test', 'sub1', 'ACTIVE', 1000, 300, 900, 1, 1)`, ownerID, serverID)
	exec(`INSERT INTO usage_snapshots
		(vpn_instance_id, taken_at, up_bytes, down_bytes, lifetime_up_bytes, lifetime_down_bytes, remote_reset_count)
		VALUES (?, 100, 100, 400, 100, 400, 0)`, vpnID)
	exec(`INSERT INTO usage_snapshots
		(vpn_instance_id, taken_at, up_bytes, down_bytes, lifetime_up_bytes, lifetime_down_bytes, remote_reset_count)
		VALUES (?, 200, 300, 900, 300, 900, 0)`, vpnID)
	exec(`INSERT INTO server_metrics (server_id, taken_at, cpu_pct, mem_used, mem_total, net_up, net_down)
		VALUES (?, 200, 12.5, 100, 400, 0, 0)`, serverID)

	auditLog := audit.New(db)
	authSvc := auth.NewService(db, auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(db, false)

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		Usage:          httpapi.NewSQLUsageStore(db),
		SyncStaleAfter: 15 * time.Minute,
	}))
	t.Cleanup(srv.Close)

	return &usageEnv{srv: srv, db: db, vpnID: vpnID, serverID: serverID}
}

// superAdmin returns a signed-in client whose account has role super_admin.
//
// The env seeds an owner account, so bootstrap is already complete — the role is
// promoted directly through the database, which is exactly what the bootstrap
// route would have written.
func (e *usageEnv) superAdmin(t *testing.T) *http.Client {
	t.Helper()
	if _, err := e.db.Exec(`UPDATE users SET role = 'super_admin' WHERE email = 'owner@example.com'`); err != nil {
		t.Fatalf("promote owner: %v", err)
	}
	return loginAs(t, e.srv.URL, "owner@example.com", "a very long password")
}

// register creates a client account and returns a signed-in client.
func (e *usageEnv) register(t *testing.T, email string) *http.Client {
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

// P-02 / `MILESTONE_7_PLAN.md` §6 — "how far along is it" must be stated, not
// left to the screen's arithmetic. The env seeds 300 up + 900 down against a
// 1000-byte limit, so the answer is 100% used and 0 bytes left — and the bar must
// never read more than 100 even when the panel reports an overrun.
func TestVPNUsageAgainstCapacity(t *testing.T) {
	env := newUsageEnv(t)
	owner := loginAs(t, env.srv.URL, "owner@example.com", "a very long password")

	resp, body := getJSON(t, owner, env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/usage?range=all")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET usage = %d (%v), want 200", resp.StatusCode, body)
	}

	if body["limit_bytes"] != float64(1000) {
		t.Errorf("limit_bytes = %v, want 1000", body["limit_bytes"])
	}
	if body["usage_pct"] != float64(100) {
		t.Errorf("usage_pct = %v, want 100 (1200 used of 1000)", body["usage_pct"])
	}
	if body["remaining_bytes"] != float64(0) {
		t.Errorf("remaining_bytes = %v, want 0 — never a negative \"left\"", body["remaining_bytes"])
	}

	// A panel can report past the nominal limit; the percentage is clamped so a
	// progress ring cannot draw 104%.
	if _, err := env.db.Exec(
		`UPDATE vpn_instances SET lifetime_up_bytes = 5000, lifetime_down_bytes = 5000 WHERE id = ?`,
		env.vpnID); err != nil {
		t.Fatalf("overrun: %v", err)
	}
	_, body = getJSON(t, owner, env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/usage?range=all")
	if body["usage_pct"] != float64(100) {
		t.Errorf("usage_pct after an overrun = %v, want 100 (clamped)", body["usage_pct"])
	}

	// Unlimited (limit 0) states 0% and -1 remaining, so "unlimited" is never
	// rendered as "0 left" — D-038 P-02's stated rule.
	if _, err := env.db.Exec(
		`UPDATE vpn_instances SET traffic_limit_bytes = 0 WHERE id = ?`, env.vpnID); err != nil {
		t.Fatalf("unlimited: %v", err)
	}
	_, body = getJSON(t, owner, env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/usage?range=all")
	if body["usage_pct"] != float64(0) {
		t.Errorf("usage_pct for an unlimited VPN = %v, want 0", body["usage_pct"])
	}
	if body["remaining_bytes"] != float64(-1) {
		t.Errorf("remaining_bytes for an unlimited VPN = %v, want -1", body["remaining_bytes"])
	}
}

// A VPN whose limit lives on the PANEL must report that limit here.
//
// `traffic_limit_bytes` is create-time history — migration 0006 says the
// `panel_*` columns "are no longer what the UI trusts" for exactly this reason.
// Reading the history column made `/api/vpns/{id}/usage` answer `limit_bytes: 0`
// and `remaining_bytes: -1` ("unlimited") for a VPN with a real 500 GiB cap,
// while `/servers/:id` and `/api/super/vpns` reported 500 GiB for the same VPN —
// two endpoints disagreeing about one VPN (VPN_FIX_LEDGER #22).
func TestUsageLimitComesFromPanelNotCreateTime(t *testing.T) {
	env := newUsageEnv(t)
	owner := loginAs(t, env.srv.URL, "owner@example.com", "a very long password")

	const limitBytes = int64(536870912000) // 500 GiB, as the panel reports it

	// The panel mirror holds the truth; the create-time column is stale at 1000.
	if _, err := env.db.Exec(
		`UPDATE vpn_instances
		    SET panel_total_gb = ?, traffic_limit_bytes = 1000
		  WHERE id = ?`, limitBytes, env.vpnID); err != nil {
		t.Fatalf("seed panel limit: %v", err)
	}

	_, body := getJSON(t, owner, env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/usage?range=all")

	if body["limit_bytes"] != float64(limitBytes) {
		t.Errorf("limit_bytes = %v, want %d (the panel's limit, not traffic_limit_bytes = 1000)",
			body["limit_bytes"], limitBytes)
	}
	// 1000 - (300+900) used bytes is 0 remaining; a 500 GiB cap is nowhere near
	// exhausted, so "unlimited" (-1) is the wrong answer either way.
	if body["remaining_bytes"] == float64(-1) {
		t.Errorf("remaining_bytes = -1 (unlimited) for a VPN with a %d byte cap", limitBytes)
	}
	wantRemaining := float64(limitBytes - 1200)
	if body["remaining_bytes"] != wantRemaining {
		t.Errorf("remaining_bytes = %v, want %v", body["remaining_bytes"], wantRemaining)
	}
}

// TestUsageHistoryOwnerReadsOwnVPN proves the owner sees their history
// oldest-first with totals attached.
func TestUsageHistoryOwnerReadsOwnVPN(t *testing.T) {
	env := newUsageEnv(t)
	owner := loginAs(t, env.srv.URL, "owner@example.com", "a very long password")

	resp, body := getJSON(t, owner, env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/usage?range=all")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner GET usage = %d (%v), want 200", resp.StatusCode, body)
	}
	if body["history"] != "AVAILABLE" {
		t.Errorf("history = %v, want AVAILABLE", body["history"])
	}
	if body["lifetime_up_bytes"] != float64(300) || body["lifetime_down_bytes"] != float64(900) {
		t.Errorf("totals = %v/%v, want 300/900",
			body["lifetime_up_bytes"], body["lifetime_down_bytes"])
	}

	// Since D-096 the points are time buckets, not raw snapshots. The two seeded
	// readings (taken_at 100 and 200) sit inside one bucket, so the read returns
	// a single point — and the assertion that matters for this test is ownership
	// plus "the history is readable and says AVAILABLE", not the row count.
	pts, ok := body["points"].([]any)
	if !ok || len(pts) != 1 {
		t.Fatalf("points = %v, want 1 bucket", body["points"])
	}
	if _, ok := pts[0].(map[string]any); !ok {
		t.Fatalf("point is not an object: %v", pts[0])
	}
}

// TestUsageHistoryHidesAnotherUsersVPN is the IDOR regression test: a client
// must not read a VPN they do not own, even by guessing the id.
func TestUsageHistoryHidesAnotherUsersVPN(t *testing.T) {
	env := newUsageEnv(t)
	stranger := env.register(t, "stranger@example.com")

	resp, body := getJSON(t, stranger, env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/usage")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("stranger reading another VPN = %d (%v), want 403", resp.StatusCode, body)
	}
}

// TestUsageHistoryRequiresAuth keeps the endpoint closed to anonymous callers.
func TestUsageHistoryRequiresAuth(t *testing.T) {
	env := newUsageEnv(t)
	anon := &http.Client{Jar: mustJar(t)}

	if resp, _ := getJSON(t, anon, env.srv.URL+"/api/vpns/"+usageID(env.vpnID)+"/usage"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous usage = %d, want 401", resp.StatusCode)
	}
}

// TestUsageUnknownVPNIs404 avoids leaking existence via a different code.
func TestUsageUnknownVPNIs404(t *testing.T) {
	env := newUsageEnv(t)
	root := env.superAdmin(t)

	if resp, _ := getJSON(t, root, env.srv.URL+"/api/vpns/9999/usage"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown vpn = %d, want 404", resp.StatusCode)
	}
}

// TestServerMetricsRoleScoping covers the server_wide label and the client ban.
func TestServerMetricsRoleScoping(t *testing.T) {
	env := newUsageEnv(t)
	root := env.superAdmin(t)

	resp, body := getJSON(t, root, env.srv.URL+"/api/servers/"+usageID(env.serverID)+"/metrics")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("super admin metrics = %d (%v), want 200", resp.StatusCode, body)
	}
	// The label stops server-wide data being shown as VPN traffic.
	if body["scope"] != "server_wide" {
		t.Errorf("scope = %v, want server_wide", body["scope"])
	}

	client := env.register(t, "m@example.com")
	if r, _ := getJSON(t, client, env.srv.URL+"/api/servers/"+usageID(env.serverID)+"/metrics"); r.StatusCode != http.StatusForbidden {
		t.Errorf("client reading server metrics = %d, want 403", r.StatusCode)
	}
}

// TestSyncStatusIsNotForClients proves a client cannot enumerate the fleet.
func TestSyncStatusIsNotForClients(t *testing.T) {
	env := newUsageEnv(t)
	root := env.superAdmin(t)

	if resp, body := getJSON(t, root, env.srv.URL+"/api/sync/status"); resp.StatusCode != http.StatusOK {
		t.Fatalf("super admin sync status = %d (%v), want 200", resp.StatusCode, body)
	}

	client := env.register(t, "sc@example.com")
	if r, _ := getJSON(t, client, env.srv.URL+"/api/sync/status"); r.StatusCode != http.StatusForbidden {
		t.Errorf("client sync status = %d, want 403", r.StatusCode)
	}
}
