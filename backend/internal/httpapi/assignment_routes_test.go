package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/store"
)

// p5Env is a real router over real SQLite with two servers seeded.
type p5Env struct {
	srv     *httptest.Server
	st      *store.Store
	serverA int64
	serverB int64
	adminID int64
}

func newP5Env(t *testing.T) *p5Env {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/p5.db")
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

	a := exec(`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv-a', 'https://a.example/base', x'00', 'ACTIVE', 1, 1)`)
	b := exec(`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv-b', 'https://b.example/base', x'00', 'ACTIVE', 1, 1)`)

	auditLog := audit.New(db)
	authSvc := auth.NewService(db, auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(db, false)

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		AdminAudit: auditLog,
		Clients:    st,
		// Servers must be wired or GET /api/servers is not mounted at all, and a
		// role-scope assertion would silently "pass" against a 404.
		Servers: st,
	}))
	t.Cleanup(srv.Close)

	return &p5Env{srv: srv, st: st, serverA: a, serverB: b}
}

// root bootstraps the first super admin.
func (e *p5Env) root(t *testing.T) *http.Client {
	t.Helper()
	return bootstrapSuperAdmin(t, e.srv.URL)
}

// makeAdmin creates a reseller through the real API and returns its id + client.
func (e *p5Env) makeAdmin(t *testing.T, root *http.Client, email string) (int64, *http.Client) {
	t.Helper()
	resp, body := postJSON(t, root, e.srv.URL+"/api/admins", map[string]any{
		"email": email, "password": "a very long password",
		"display_name": "Reseller", "role": "admin", "quota_limit": 10,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create admin = %d (%v), want 201", resp.StatusCode, body)
	}
	id := int64(body["id"].(float64))
	return id, loginAs(t, e.srv.URL, email, "a very long password")
}

// putServers sends the replace payload.
func (e *p5Env) putServers(t *testing.T, c *http.Client, adminID int64, ids []int64) (*http.Response, map[string]any) {
	t.Helper()
	return putJSON(t, c, e.srv.URL+"/api/admins/"+usageID(adminID)+"/servers", map[string]any{
		"server_ids": ids,
	})
}

// TestAssignServersGivesResellerScope is the P5 regression test: a reseller
// created by the API must be able to see the server they were assigned.
//
// Before this endpoint existed, `admin_server_assignments` had no write path,
// so every reseller saw **zero** servers forever.
func TestAssignServersGivesResellerScope(t *testing.T) {
	env := newP5Env(t)
	root := env.root(t)
	adminID, adminClient := env.makeAdmin(t, root, "reseller@example.com")

	// Before assignment: the reseller sees nothing. `GET /api/servers` needs the
	// full integration service to be mounted, so scoping is asserted through the
	// store method that handler calls — same query, no extra wiring.
	before, err := env.st.ListServers(context.Background(), adminID)
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("reseller saw %d servers before assignment, want 0", len(before))
	}
	_ = adminClient

	resp, body := env.putServers(t, root, adminID, []int64{env.serverA})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT servers = %d (%v), want 200", resp.StatusCode, body)
	}

	// After assignment: exactly the one server.
	after, err := env.st.ListServers(context.Background(), adminID)
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("reseller saw %d servers after assignment, want 1", len(after))
	}
	if after[0].ID != env.serverA {
		t.Errorf("reseller saw server %d, want %d", after[0].ID, env.serverA)
	}
}

// TestAssignServersIsReplaceNotMerge proves the idempotent replace semantics a
// checkbox UI depends on.
func TestAssignServersIsReplaceNotMerge(t *testing.T) {
	env := newP5Env(t)
	root := env.root(t)
	adminID, adminClient := env.makeAdmin(t, root, "reseller2@example.com")

	if r, _ := env.putServers(t, root, adminID, []int64{env.serverA, env.serverB}); r.StatusCode != http.StatusOK {
		t.Fatalf("initial assign = %d", r.StatusCode)
	}
	if rows, _ := env.st.ListServers(context.Background(), adminID); len(rows) != 2 {
		t.Fatalf("after assigning both, reseller sees %d, want 2", len(rows))
	}

	// Replacing with one id must *remove* the other, not add to it.
	if r, _ := env.putServers(t, root, adminID, []int64{env.serverB}); r.StatusCode != http.StatusOK {
		t.Fatalf("replace = %d", r.StatusCode)
	}
	rows, _ := env.st.ListServers(context.Background(), adminID)
	if len(rows) != 1 || rows[0].ID != env.serverB {
		t.Fatalf("after replace, reseller sees %d servers, want only %d", len(rows), env.serverB)
	}

	// Sending the same set twice is a no-op (idempotent).
	if r, _ := env.putServers(t, root, adminID, []int64{env.serverB}); r.StatusCode != http.StatusOK {
		t.Fatalf("repeat replace = %d", r.StatusCode)
	}
	if rows, _ := env.st.ListServers(context.Background(), adminID); len(rows) != 1 {
		t.Errorf("repeating the same assignment changed the result to %d servers", len(rows))
	}
	_ = adminClient
}

// TestAssignServersRejectsNonAdmin proves a client cannot hold a reseller scope.
func TestAssignServersRejectsNonAdmin(t *testing.T) {
	env := newP5Env(t)
	root := env.root(t)

	reg := &http.Client{Jar: mustJar(t)}
	postJSON(t, reg, env.srv.URL+"/api/auth/register", map[string]string{
		"email": "plain@example.com", "password": "a very long password",
	})
	clients := getJSONArray(t, root, env.srv.URL+"/api/clients")
	if len(clients) != 1 {
		t.Fatalf("clients = %d, want 1", len(clients))
	}
	clientID := int64(clients[0]["id"].(float64))

	resp, body := env.putServers(t, root, clientID, []int64{env.serverA})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("assigning servers to a client = %d (%v), want 422", resp.StatusCode, body)
	}
}

// TestAssignServersRejectsUnknownServer keeps a typo from wiping assignments.
func TestAssignServersRejectsUnknownServer(t *testing.T) {
	env := newP5Env(t)
	root := env.root(t)
	adminID, _ := env.makeAdmin(t, root, "r3@example.com")

	if r, _ := env.putServers(t, root, adminID, []int64{env.serverA}); r.StatusCode != http.StatusOK {
		t.Fatalf("initial assign = %d", r.StatusCode)
	}

	// A bad id must be rejected *and* must not clear the existing assignment.
	resp, _ := env.putServers(t, root, adminID, []int64{env.serverA, 424242})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("unknown server = %d, want 422", resp.StatusCode)
	}
	ids := getServerIDs(t, root, env.srv.URL+"/api/admins/"+usageID(adminID)+"/servers")
	if len(ids) != 1 {
		t.Errorf("existing assignment was lost after a rejected update: %v", ids)
	}
}

// TestAssignServersRequiresSuperAdmin proves an admin cannot widen their own scope.
func TestAssignServersRequiresSuperAdmin(t *testing.T) {
	env := newP5Env(t)
	root := env.root(t)
	adminID, adminClient := env.makeAdmin(t, root, "self@example.com")

	resp, _ := env.putServers(t, adminClient, adminID, []int64{env.serverB})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("admin assigning their own servers = %d, want 403", resp.StatusCode)
	}
}

// TestAssignServersAnonymousIs401 keeps the endpoint behind auth.
func TestAssignServersAnonymousIs401(t *testing.T) {
	env := newP5Env(t)
	anon := &http.Client{Jar: mustJar(t)}

	resp, _ := env.putServers(t, anon, 1, []int64{env.serverA})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous PUT = %d, want 401", resp.StatusCode)
	}
}

// getServerIDs reads the server_ids array from a response object.
func getServerIDs(t *testing.T, c *http.Client, url string) []int64 {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		ServerIDs []int64 `json:"server_ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return out.ServerIDs
}

// TestClientsVisibilityIsRoleScoped is the P5 clients regression test.
//
// A reseller must see **only** clients who own a VPN they sold — never the whole
// customer base, and never a client at all.
func TestClientsVisibilityIsRoleScoped(t *testing.T) {
	env := newP5Env(t)
	root := env.root(t)

	for _, email := range []string{"alice@example.com", "bob@example.com"} {
		c := &http.Client{Jar: mustJar(t)}
		postJSON(t, c, env.srv.URL+"/api/auth/register", map[string]string{
			"email": email, "password": "a very long password",
		})
	}

	// The super admin sees both.
	if all := getJSONArray(t, root, env.srv.URL+"/api/clients"); len(all) != 2 {
		t.Fatalf("super admin sees %d clients, want 2", len(all))
	}

	// A fresh reseller who has sold nothing sees none of them.
	_, reseller := env.makeAdmin(t, root, "newreseller@example.com")
	if mine := getJSONArray(t, reseller, env.srv.URL+"/api/clients"); len(mine) != 0 {
		t.Fatalf("a reseller with no sales sees %d clients, want 0", len(mine))
	}

	// A client is refused outright rather than shown an empty list: an empty
	// list would read as "there are no customers" instead of "not yours".
	client := &http.Client{Jar: mustJar(t)}
	postJSON(t, client, env.srv.URL+"/api/auth/register", map[string]string{
		"email": "nosy@example.com", "password": "a very long password",
	})
	if resp, _ := getJSON(t, client, env.srv.URL+"/api/clients"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("client listing clients = %d, want 403", resp.StatusCode)
	}
}

// TestClientsShowsSellerScope proves a sale brings that client into the
// reseller's list — the link the sale flow depends on.
func TestClientsShowsSellerScope(t *testing.T) {
	env := newP5Env(t)
	root := env.root(t)

	c := &http.Client{Jar: mustJar(t)}
	if resp, body := postJSON(t, c, env.srv.URL+"/api/auth/register", map[string]string{
		"email": "buyer@example.com", "password": "a very long password",
	}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("register buyer = %d (%v)", resp.StatusCode, body)
	}

	adminID, reseller := env.makeAdmin(t, root, "seller@example.com")

	if before := getJSONArray(t, reseller, env.srv.URL+"/api/clients"); len(before) != 0 {
		t.Fatalf("seller sees %d clients with no sales, want 0", len(before))
	}

	clientID := int64(getJSONArray(t, root, env.srv.URL+"/api/clients")[0]["id"].(float64))
	insertVPN(t, env, clientID, adminID)

	after := getJSONArray(t, reseller, env.srv.URL+"/api/clients")
	if len(after) != 1 {
		t.Fatalf("after selling a VPN the reseller sees %d clients, want 1", len(after))
	}
	if after[0]["vpn_count"] != float64(1) {
		t.Errorf("vpn_count = %v, want 1", after[0]["vpn_count"])
	}
}

// insertVPN creates the instance + assignment pair the provisioning service
// writes, so the seller-scope join has real rows to find.
func insertVPN(t *testing.T, env *p5Env, clientID, sellerID int64) int64 {
	t.Helper()
	res, err := env.st.DB().Exec(
		`INSERT INTO vpn_instances (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		 VALUES (?, ?, 'buyer-panel', 'sub-x', 'ACTIVE', 1, 1)`, clientID, env.serverA)
	if err != nil {
		t.Fatalf("insert vpn: %v", err)
	}
	vpnID, _ := res.LastInsertId()
	if _, err := env.st.DB().Exec(
		`INSERT INTO vpn_assignments (vpn_instance_id, client_user_id, seller_user_id, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'ACTIVE', 1, 1)`, vpnID, clientID, sellerID); err != nil {
		t.Fatalf("insert assignment: %v", err)
	}
	return vpnID
}
