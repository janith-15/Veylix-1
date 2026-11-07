package httpapi_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/releases"
	"veylix/backend/internal/store"
)

// releaseEnv is a real router over real SQLite with the release service wired
// exactly as cmd/veylixd wires it, plus a controllable clock.
type releaseEnv struct {
	srv    *httptest.Server
	st     *store.Store
	svc    *releases.Service
	now    int64
	setNow func(int64)
}

func newReleaseEnv(t *testing.T) *releaseEnv {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/release.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	auditLog := audit.New(st.DB()).WithNotifier(st)
	svc := releases.New(st, auditLog)
	env := &releaseEnv{st: st, svc: svc, now: time.Now().Unix()}
	env.setNow = func(v int64) {
		env.now = v
		svc.SetClock(func() int64 { return env.now })
	}
	env.setNow(env.now)

	authSvc := auth.NewService(st.DB(), auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(st.DB(), false)

	env.srv = httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		AdminAudit:    auditLog,
		Notifications: st,
		VPNs:          &noopVPNService{},
		VPNLister:     httpapi.NewSQLVPNLister(st.DB()),
		Releases:      svc,
		Audit:         st,
		Quota:         st,
	}))
	t.Cleanup(env.srv.Close)
	return env
}

// seedOwnedVPN creates a server, a depleted VPN owned by `ownerID`, and the sale.
func (e *releaseEnv) seedOwnedVPN(t *testing.T, ownerID, sellerID, vpnID int64) int64 {
	t.Helper()
	exec := func(q string, args ...any) sql.Result {
		t.Helper()
		res, err := e.st.DB().Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		return res
	}
	exec(`INSERT OR IGNORE INTO server_integrations (id, name, panel_url, token_ciphertext, status, created_at, updated_at)
	      VALUES (1, 'rel-srv', 'https://panel.example/base', x'00', 'ACTIVE', 1, 1)`)
	exec(`INSERT INTO vpn_instances (id, server_id, owner_user_id, panel_email, panel_sub_id, status, created_at, updated_at)
	      VALUES (?, 1, ?, ?, ?, 'TRAFFIC_DEPLETED', 1, 1)`,
		vpnID, ownerID, "rel-"+usageID(vpnID)+"@panel", "sub-"+usageID(vpnID))
	exec(`INSERT INTO vpn_assignments (vpn_instance_id, seller_user_id, client_user_id, status, created_at, updated_at)
	      VALUES (?, ?, ?, 'ACTIVE', 1, 1)`, vpnID, sellerID, ownerID)
	return vpnID
}

// signupClient registers a client through the real API.
func (e *releaseEnv) signupClient(t *testing.T, email string) (*http.Client, int64) {
	t.Helper()
	c := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	resp, body := postJSON(t, c, e.srv.URL+"/api/auth/register", map[string]string{
		"email": email, "password": "a very long password",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register %s = %d (%v)", email, resp.StatusCode, body)
	}
	return c, int64(body["id"].(float64))
}

func hasSub(s, sub string) bool { return strings.Contains(s, sub) }

// TestReleaseGraceAndLocalOnlyRemoval is the D-027 regression test.
//
// It walks the whole lifecycle with a controllable clock:
//   - release opens a 3-day window and deletes nothing;
//   - restore inside the window cancels the removal;
//   - a second release whose window expires is removed LOCALLY, its assignment
//     goes with it, the audit entry states the panel client was kept, and a
//     restored VPN is never touched by the purge;
//   - only the owner may release.
func TestReleaseGraceAndLocalOnlyRemoval(t *testing.T) {
	env := newReleaseEnv(t)
	root := bootstrapSuperAdmin(t, env.srv.URL)

	client, clientID := env.signupClient(t, "rel-owner@example.com")
	env.signupClient(t, "rel-other@example.com")

	vpnA := env.seedOwnedVPN(t, clientID, clientID, 9401)
	vpnB := env.seedOwnedVPN(t, clientID, clientID, 9402)

	// --- an operator may not release a client's VPN -------------------------
	resp, body := postJSON(t, root, env.srv.URL+"/api/vpns/"+usageID(vpnA)+"/release", map[string]any{})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("super admin releases a client VPN = %d (%v), want 403", resp.StatusCode, body)
	}

	// --- release A, then undo it inside the window --------------------------
	resp, rel := postJSON(t, client, env.srv.URL+"/api/vpns/"+usageID(vpnA)+"/release",
		map[string]any{"reason": "quota_exhausted"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("release = %d (%v), want 201", resp.StatusCode, rel)
	}
	relA := int64(rel["id"].(float64))
	deadlineA := int64(rel["restore_deadline"].(float64))
	if want := env.now + 3*24*60*60; deadlineA != want {
		t.Fatalf("restore deadline = %d, want %d (3 days)", deadlineA, want)
	}
	if rel["panel_client_kept"] != true {
		t.Fatalf("release does not record that the panel client is kept: %v", rel)
	}

	// Releasing again while pending is refused: it would restart the clock.
	resp, _ = postJSON(t, client, env.srv.URL+"/api/vpns/"+usageID(vpnA)+"/release", map[string]any{})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("double release = %d, want 409", resp.StatusCode)
	}

	// A stranger cannot restore it.
	stranger := loginAs(t, env.srv.URL, "rel-other@example.com", "a very long password")
	resp, _ = postJSON(t, stranger, env.srv.URL+"/api/vpn-releases/"+usageID(relA)+"/restore", map[string]any{})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("stranger restores = %d, want 404", resp.StatusCode)
	}

	// The owner restores it: the window is still open.
	resp, restored := postJSON(t, client, env.srv.URL+"/api/vpn-releases/"+usageID(relA)+"/restore", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restore = %d (%v), want 200", resp.StatusCode, restored)
	}
	if restored["restored_at"] == nil {
		t.Fatalf("restore did not stamp restored_at: %v", restored)
	}

	// --- release B and let the window expire --------------------------------
	resp, relB := postJSON(t, client, env.srv.URL+"/api/vpns/"+usageID(vpnB)+"/release", map[string]any{})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("release B = %d (%v)", resp.StatusCode, relB)
	}

	// Just before B's deadline: nothing is due yet.
	env.setNow(env.now + 3*24*60*60 - 1)
	if n, err := env.svc.PurgeDue(context.Background()); err != nil || n != 0 {
		t.Fatalf("early purge = (%d, %v), want (0, nil)", n, err)
	}

	// Past the deadline: the purge removes the LOCAL row only.
	env.setNow(env.now + 4*24*60*60)
	if n, err := env.svc.PurgeDue(context.Background()); err != nil || n != 1 {
		t.Fatalf("purge = (%d, %v), want (1, nil)", n, err)
	}

	var exists int
	if err := env.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances WHERE id = ?`, vpnB).Scan(&exists); err != nil {
		t.Fatalf("count vpn: %v", err)
	}
	if exists != 0 {
		t.Fatalf("released VPN row still present after the grace period")
	}
	var assignments int
	if err := env.st.DB().QueryRow(
		`SELECT COUNT(*) FROM vpn_assignments WHERE vpn_instance_id = ?`, vpnB).Scan(&assignments); err != nil {
		t.Fatalf("count assignment: %v", err)
	}
	if assignments != 0 {
		t.Fatalf("assignment survived its VPN: %d rows", assignments)
	}

	var ctxJSON string
	if err := env.st.DB().QueryRow(
		`SELECT context FROM audit_logs WHERE action = 'vpn.purge' ORDER BY id DESC LIMIT 1`).
		Scan(&ctxJSON); err != nil {
		t.Fatalf("audit purge entry missing: %v", err)
	}
	if !hasSub(ctxJSON, "panel_client_kept") || !hasSub(ctxJSON, "not deleted") {
		t.Fatalf("purge audit does not state the panel client was kept: %s", ctxJSON)
	}

	// The restored VPN A survives the purge.
	if err := env.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances WHERE id = ?`, vpnA).Scan(&exists); err != nil {
		t.Fatalf("count vpn A: %v", err)
	}
	if exists != 1 {
		t.Fatalf("restored VPN was removed by the purge")
	}
}
