package httpapi_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/store"
)

// linkPanel is a stub 3X-UI that hands out one canonical URI per known client.
type linkPanel struct {
	mu      sync.Mutex
	exists  map[string]bool
	linkErr bool
	panels  []string
}

func (p *linkPanel) AddClient(_ context.Context, req panel.AddClientRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.exists[req.Client.Email] = true
	return nil
}

func (p *linkPanel) ClientExists(_ context.Context, email string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exists[email], nil
}

func (p *linkPanel) DeleteClient(_ context.Context, email string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.exists, email)
	return nil
}

func (p *linkPanel) SetClientEnabled(_ context.Context, _ string, _ bool) error { return nil }

func (p *linkPanel) Links(_ context.Context, email string) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.linkErr {
		return nil, context.DeadlineExceeded
	}
	if !p.exists[email] {
		return []string{}, nil
	}
	return []string{"vless://owner@panel"}, nil
}

// fakePingService measures nothing: it reports what the test asked for.
type fakePingService struct {
	pingReachable bool
	pingMS        int64
	linkErr       bool
}

type connectionEnv struct {
	srv   *httptest.Server
	st    *store.Store
	panel *linkPanel
}

type signedIn struct {
	client *http.Client
	id     int64
}

func newConnectionEnv(t *testing.T, fake *fakePingService) *connectionEnv {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/conn.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	panelStub := &linkPanel{exists: map[string]bool{}}
	auditLog := audit.New(st.DB())
	factory := func(context.Context, int64) (provisioning.PanelClient, error) {
		if fake.linkErr {
			panelStub.linkErr = true
		} else {
			panelStub.linkErr = false
		}
		return panelStub, nil
	}
	provSvc := provisioning.New(st.DB(), st, auditLog, factory)

	authSvc := auth.NewService(st.DB(), auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(st.DB(), false)

	ping := func(context.Context, int64) (bool, int64) {
		if fake.pingReachable {
			return true, fake.pingMS
		}
		return false, -1
	}

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		AdminAudit:    auditLog,
		VPNs:          provSvc,
		VPNLister:     httpapi.NewSQLVPNLister(st.DB()),
		VPNConnection: provSvc,
		PanelPing:     provisioning.PanelPinger(ping),
	}))
	t.Cleanup(srv.Close)
	return &connectionEnv{srv: srv, st: st, panel: panelStub}
}

func (e *connectionEnv) root(t *testing.T) *http.Client {
	t.Helper()
	return bootstrapSuperAdmin(t, e.srv.URL)
}

func (e *connectionEnv) signup(t *testing.T, email string) *signedIn {
	t.Helper()
	c := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	resp, body := postJSON(t, c, e.srv.URL+"/api/auth/register", map[string]string{
		"email": email, "password": "a very long password",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register %s = %d (%v)", email, resp.StatusCode, body)
	}
	return &signedIn{client: c, id: int64(body["id"].(float64))}
}

// seedVPN inserts a VPN owned by a client and registers its panel client with
// the stub, so Links has something to return.
func (e *connectionEnv) seedVPN(t *testing.T, ownerID, vpnID int64) int64 {
	t.Helper()
	exec := func(q string, args ...any) sql.Result {
		t.Helper()
		res, err := e.st.DB().Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		return res
	}
	exec(`INSERT INTO server_integrations (id, name, panel_url, token_ciphertext, status, created_at, updated_at)
	      VALUES (1, 'conn-srv', 'https://panel.example/base', x'00', 'ACTIVE', 1, 1)`)
	email := "owner-" + usageID(vpnID) + "@panel"
	exec(`INSERT INTO vpn_instances (id, server_id, owner_user_id, panel_email, panel_sub_id, status, created_at, updated_at)
	      VALUES (?, 1, ?, ?, ?, 'ACTIVE', 1, 1)`, vpnID, ownerID, email, "sub-"+usageID(vpnID))
	// Register the panel client so the stub hands out a link for it.
	e.panel.mu.Lock()
	e.panel.exists[email] = true
	e.panel.mu.Unlock()
	return vpnID
}
