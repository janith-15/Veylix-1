package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/domain"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/integrations"
	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/store"
)

// fakeInspector reports a fixed set of panel clients per server, and can fail
// for a chosen server to exercise the outage path.
type fakeInspector struct {
	clients map[int64][]integrations.PanelClientView
	failing map[int64]bool
}

func (f *fakeInspector) PanelClients(_ context.Context, id int64) ([]integrations.PanelClientView, error) {
	if f.failing[id] {
		return nil, errors.New("panel unreachable")
	}
	return f.clients[id], nil
}

// set makes this inspector report the given panel clients for a server.
func (f *fakeInspector) set(serverID int64, clients []integrations.PanelClientView) {
	if f.clients == nil {
		f.clients = map[int64][]integrations.PanelClientView{}
	}
	f.clients[serverID] = clients
}

// fail makes PanelClients error for a server, simulating an outage.
func (f *fakeInspector) fail(serverID int64) {
	if f.failing == nil {
		f.failing = map[int64]bool{}
	}
	f.failing[serverID] = true
}

type listEnv struct {
	url      string
	st       *store.Store
	serverID int64
	insp     *fakeInspector
}

// newListEnv builds the router with BOTH listers wired: the local one for
// admin/client and a panel-backed one for the Super Admin (D-024.3).
func newListEnv(t *testing.T, insp *fakeInspector) *listEnv {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/list.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	res, err := st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('srv', 'https://panel.example/base', x'00', 'ACTIVE', 1, 1)`)
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
	serverID, _ := res.LastInsertId()

	auditLog := audit.New(st.DB())
	authSvc := auth.NewService(st.DB(), auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(st.DB(), false)

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		// Wired so the tagging route and the inspector group exist; the list
		// tests drive the panel through the injected inspector.
		Integrations: &tagStubService{}, Servers: st,
		PanelInspector: insp, PanelTags: st, PanelTagAudit: auditLog,
		VPNs:           &noopVPNService{},
		VPNLister:      httpapi.NewSQLVPNLister(st.DB()),
		PanelVPNLister: httpapi.NewPanelVPNLister(st.DB(), insp),
	}))
	t.Cleanup(srv.Close)

	return &listEnv{url: srv.URL, st: st, serverID: serverID, insp: insp}
}

// setPanelClients makes the fake panel report the given clients for this env's
// server.
func (e *listEnv) setPanelClients(t *testing.T, clients []integrations.PanelClientView) {
	t.Helper()
	e.insp.set(e.serverID, clients)
}

// signupClient registers a client account (self-registration) and returns a
// cookie-carrying client.
func (e *listEnv) signupClient(t *testing.T, email string) *http.Client {
	t.Helper()
	c := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	resp, body := postJSON(t, c, e.url+"/api/auth/register", map[string]string{
		"email": email, "password": "a very long password", "display_name": email,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register %s = %d (%v)", email, resp.StatusCode, body)
	}
	return c
}

// noopVPNService satisfies the provisioning surface; list tests never create.
type noopVPNService struct{}

func (noopVPNService) Create(context.Context, provisioning.CreateInput) (*provisioning.Result, error) {
	return nil, errors.New("not used")
}
func (noopVPNService) Suspend(context.Context, int64, auth.Principal) error { return nil }
func (noopVPNService) Resume(context.Context, int64, auth.Principal) error  { return nil }
func (noopVPNService) Revoke(context.Context, int64, auth.Principal) (domain.VPNStatus, error) {
	return "", nil
}
func (noopVPNService) VerifyPanel(context.Context, int64, auth.Principal) (*provisioning.VerifyResult, error) {
	return nil, errors.New("not used")
}
func (noopVPNService) RemoveAbsent(context.Context, int64, auth.Principal) error { return nil }

// listVPNs calls GET /api/vpns as the given client.
func (e *listEnv) listVPNs(t *testing.T, c *http.Client, query string) (int, []map[string]any) {
	t.Helper()
	resp, err := c.Get(e.url + "/api/vpns" + query)
	if err != nil {
		t.Fatalf("GET /api/vpns%s: %v", query, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// ptr returns a pointer to s, for the *string Tag field.
func ptr(s string) *string { return &s }

// putTagAs sends PUT /api/panel-clients/{server}/{email}/tag.
func (e *listEnv) putTagAs(t *testing.T, c *http.Client, email, tag string) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"tag": tag})
	path := e.url + "/api/panel-clients/" + itoa(e.serverID) + "/" + email + "/tag"
	req, err := http.NewRequest(http.MethodPut, path, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build PUT: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// seedVPN inserts a local VPN row owned by a given user, so tests can assert
// what an admin/client sees without going through provisioning.
func (e *listEnv) seedVPN(t *testing.T, panelEmail string, ownerID int64) int64 {
	t.Helper()
	res, err := e.st.DB().Exec(
		`INSERT INTO vpn_instances
		   (owner_user_id, server_id, inbound_id, offer_id, panel_email, panel_sub_id,
		    status, traffic_limit_bytes, expires_at, device_limit, created_at, updated_at)
		 VALUES (?, ?, NULL, NULL, ?, ?, 'ACTIVE', 10737418240, NULL, 2, 1, 1)`,
		ownerID, e.serverID, panelEmail, panelEmail)
	if err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// userID looks up an account id by email.
func (e *listEnv) userID(t *testing.T, email string) int64 {
	t.Helper()
	var id int64
	if err := e.st.DB().QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&id); err != nil {
		t.Fatalf("look up user %s: %v", email, err)
	}
	return id
}

// T-32 (QA_PLAN) — D-024.3 / I-3: a panel client with no tag is visible to the
// Super Admin and invisible to an admin and a client.
//
// This is the rule that makes the tag meaningful: a client created directly in
// 3X-UI is a real VPN (I-1), but it is not *published* until the Super Admin
// classifies it. Before this change neither audience could see it at all,
// because GET /api/vpns read only local rows.
func TestUntaggedPanelClientIsSuperAdminOnly(t *testing.T) {
	env := newListEnv(t, &fakeInspector{clients: map[int64][]integrations.PanelClientView{}})
	root := bootstrapSuperAdmin(t, env.url)

	// A client that exists only on the panel, untagged.
	env.setPanelClients(t, []integrations.PanelClientView{
		{PanelEmail: "raw-1", Enable: true, Tag: nil},
	})

	code, list := env.listVPNs(t, root, "")
	if code != http.StatusOK {
		t.Fatalf("super admin list = %d, want 200", code)
	}
	if len(list) != 1 || list[0]["panel_email"] != "raw-1" {
		t.Fatalf("super admin must see the untagged client, got %v", list)
	}
	if list[0]["tag"] != nil {
		t.Errorf("tag = %v, want null for an untagged client", list[0]["tag"])
	}
	if list[0]["source"] != "panel" {
		t.Errorf("source = %v, want panel", list[0]["source"])
	}

	// The Untagged queue is exactly this filter.
	_, untagged := env.listVPNs(t, root, "?tag=untagged")
	if len(untagged) != 1 {
		t.Fatalf("?tag=untagged = %d rows, want 1", len(untagged))
	}
	_, free := env.listVPNs(t, root, "?tag=free")
	if len(free) != 0 {
		t.Fatalf("?tag=free = %d rows, want 0 (nothing is tagged)", len(free))
	}

	// An admin and a client must not see it.
	_, created := postJSON(t, root, env.url+"/api/admins", map[string]any{
		"email": "reseller@example.com", "password": "a very long password",
		"display_name": "Reseller", "role": "admin",
	})
	if created["email"] == nil {
		t.Fatalf("create admin = %v", created)
	}
	admin := loginAs(t, env.url, "reseller@example.com", "a very long password")
	if code, list := env.listVPNs(t, admin, ""); code != http.StatusOK || len(list) != 0 {
		t.Fatalf("admin list = %d with %d rows, want 200 with 0", code, len(list))
	}

	client := env.signupClient(t, "client@example.com")
	if code, list := env.listVPNs(t, client, ""); code != http.StatusOK || len(list) != 0 {
		t.Fatalf("client list = %d with %d rows, want 200 with 0", code, len(list))
	}
}

// Tagging a panel client publishes it: it appears under ?tag=free and leaves
// the Untagged queue (REQUIREMENTS §2.0 — "join the catalogue the moment the
// Super Admin tags them").
//
// The tag reaches the list through the inspector, which is where the panel
// client's own row joins Veylix's classification, so the test sets the view the
// inspector reports after the store has been written through the real HTTP
// route. That verifies both halves: the write persists, and the list filters on
// the persisted value.
func TestTaggingPublishesPanelClient(t *testing.T) {
	env := newListEnv(t, &fakeInspector{})
	root := bootstrapSuperAdmin(t, env.url)
	env.setPanelClients(t, []integrations.PanelClientView{
		{PanelEmail: "raw-1", Enable: true, Tag: nil},
	})

	// The store write goes through the real route.
	code, body := env.putTagAs(t, root, "raw-1", "FREE")
	if code != http.StatusOK {
		t.Fatalf("tag = %d (%v), want 200", code, body)
	}
	stored, err := env.st.ClientTag(context.Background(), env.serverID, "raw-1")
	if err != nil || stored.Tag != store.TagFree {
		t.Fatalf("stored tag = %v (%v), want FREE", stored, err)
	}

	// Before publication the client is only in the Untagged queue.
	_, untagged := env.listVPNs(t, root, "?tag=untagged")
	if len(untagged) != 1 {
		t.Fatalf("?tag=untagged = %d rows, want 1", len(untagged))
	}

	// Publish: the inspector now reports the stored classification.
	env.setPanelClients(t, []integrations.PanelClientView{
		{PanelEmail: "raw-1", Enable: true, Tag: ptr("FREE")},
	})
	_, free := env.listVPNs(t, root, "?tag=free")
	if len(free) != 1 || free[0]["panel_email"] != "raw-1" {
		t.Fatalf("?tag=free = %v, want the published client", free)
	}
	if free[0]["tag"] != "FREE" {
		t.Errorf("tag = %v, want FREE", free[0]["tag"])
	}
	_, untagged = env.listVPNs(t, root, "?tag=untagged")
	if len(untagged) != 0 {
		t.Fatalf("?tag=untagged after publishing = %d rows, want 0", len(untagged))
	}
}

// An unknown ?tag= value is 422, never a silently unfiltered list.
func TestVPNListRejectsUnknownTagFilter(t *testing.T) {
	env := newListEnv(t, &fakeInspector{})
	root := bootstrapSuperAdmin(t, env.url)

	resp, err := root.Get(env.url + "/api/vpns?tag=nope")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("?tag=nope = %d, want 422", resp.StatusCode)
	}
}

// A panel outage must DEGRADE the Super Admin's list, not empty it: Veylix's own
// rows still come back, flagged source=local so nothing claims the panel agreed.
func TestPanelOutageDegradesToList(t *testing.T) {
	env := newListEnv(t, &fakeInspector{})
	root := bootstrapSuperAdmin(t, env.url)

	owner := env.userID(t, "owner@example.com")
	env.seedVPN(t, "vyx-1", owner)
	env.insp.fail(env.serverID)

	code, list := env.listVPNs(t, root, "")
	if code != http.StatusOK {
		t.Fatalf("list during outage = %d, want 200", code)
	}
	if len(list) != 1 || list[0]["panel_email"] != "vyx-1" {
		t.Fatalf("outage must still list the local row, got %v", list)
	}
	if list[0]["source"] != "local" {
		t.Errorf("source = %v, want local (the panel was not read)", list[0]["source"])
	}
}

// When nothing could be verified AND Veylix has no rows of its own, an empty
// list would assert "there are no VPNs" — a claim nothing supports. The list
// must fail loudly instead.
func TestUnreadablePanelWithNoLocalRowsIsAnError(t *testing.T) {
	env := newListEnv(t, &fakeInspector{})
	root := bootstrapSuperAdmin(t, env.url)
	env.insp.fail(env.serverID)

	resp, err := root.Get(env.url + "/api/vpns")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("unverified list = %d, want an error (never a confident empty list)", resp.StatusCode)
	}
}

// A VPN that exists both locally and on the panel is ONE row: the panel supplies
// its tag and existence, the local record supplies ownership and timestamps.
// Duplicating it would make a single VPN look like two.
func TestPanelAndLocalRowsMerge(t *testing.T) {
	env := newListEnv(t, &fakeInspector{})
	root := bootstrapSuperAdmin(t, env.url)

	owner := env.userID(t, "owner@example.com")
	vpnID := env.seedVPN(t, "vyx-1", owner)
	env.setPanelClients(t, []integrations.PanelClientView{
		{PanelEmail: "vyx-1", Enable: true, Tag: ptr("PREMIUM"), TotalGB: 20 << 30},
		{PanelEmail: "panel-only", Enable: false, Tag: nil},
	})

	code, list := env.listVPNs(t, root, "")
	if code != http.StatusOK {
		t.Fatalf("list = %d, want 200", code)
	}
	if len(list) != 2 {
		t.Fatalf("list = %d rows, want 2 (one merged, one panel-only): %v", len(list), list)
	}

	byEmail := map[string]map[string]any{}
	for _, r := range list {
		byEmail[r["panel_email"].(string)] = r
	}

	merged := byEmail["vyx-1"]
	if merged == nil {
		t.Fatalf("vyx-1 missing from %v", list)
	}
	if int64(merged["id"].(float64)) != vpnID {
		t.Errorf("merged id = %v, want %d (the local row's id)", merged["id"], vpnID)
	}
	if merged["tag"] != "PREMIUM" {
		t.Errorf("merged tag = %v, want PREMIUM (panel classification)", merged["tag"])
	}
	if int64(merged["owner_user_id"].(float64)) != owner {
		t.Errorf("merged owner = %v, want %d (local identity)", merged["owner_user_id"], owner)
	}
	if merged["source"] != "panel" {
		t.Errorf("merged source = %v, want panel", merged["source"])
	}

	only := byEmail["panel-only"]
	if only == nil {
		t.Fatalf("panel-only missing from %v", list)
	}
	if int64(only["id"].(float64)) != 0 {
		t.Errorf("panel-only id = %v, want 0 (no local record)", only["id"])
	}
	// A disabled panel client that Veylix never assigned reads as suspended,
	// never as active: the panel says it is off.
	if only["status"] != "SUSPENDED" {
		t.Errorf("panel-only status = %v, want SUSPENDED", only["status"])
	}
}

// TestAdminConsoleUsesThePanelBackedLister is the regression guard for a wiring
// bug the owner hit in the product: GET /api/super/vpns was registered with the
// LOCAL lister, so a panel client with no Veylix row — the whole reason the
// console exists — never appeared. A panel holding fourteen clients rendered as
// six local rows.
//
// The bug lived in Router(), which the list tests below do not exercise, so the
// suite stayed green while the screen was wrong.
func TestAdminConsoleUsesThePanelBackedLister(t *testing.T) {
	env := newListEnv(t, &fakeInspector{})
	root := bootstrapSuperAdmin(t, env.url)

	// One local row and one panel-only client: the console must show both.
	owner := env.userID(t, "owner@example.com")
	env.seedVPN(t, "vyx-1", owner)
	env.setPanelClients(t, []integrations.PanelClientView{
		{PanelEmail: "vyx-1", Enable: true, Tag: ptr("PREMIUM")},
		{PanelEmail: "panel-only", Enable: true},
	})

	resp, err := root.Get(env.url + "/api/super/vpns")
	if err != nil {
		t.Fatalf("GET /api/super/vpns: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/super/vpns = %d, want 200", resp.StatusCode)
	}
	var page struct {
		VPNs  []map[string]any `json:"vpns"`
		Total int              `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if page.Total != 2 {
		t.Fatalf("total = %d, want 2 (one local + one panel-only); a smaller number means "+
			"the console read the LOCAL lister: %v", page.Total, page.VPNs)
	}
	seen := map[string]bool{}
	for _, r := range page.VPNs {
		seen[r["panel_email"].(string)] = true
	}
	if !seen["panel-only"] {
		t.Fatalf("the panel-only client is missing from the Super Admin console: %v", page.VPNs)
	}
}
