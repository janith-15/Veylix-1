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
	"veylix/backend/internal/panel"
	"veylix/backend/internal/provisioning"
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
	// provPanel is the stub the provisioning service's factory returns, so a test
	// can prove the purge path never reaches the panel (P-3).
	provPanel *purgePanelStub
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

	/*
	 * P-3: the purge route needs a REAL provisioning service, not the no-op stub.
	 * The stub answers "not used" for everything, which the router maps to 500 —
	 * so a transport test against it would have asserted the stub, not the rule.
	 * The panel factory returns a stub whose `exists` map is empty, because the
	 * purge path must never call the panel at all; a call would be a bug.
	 */
	purgePanel := &purgePanelStub{exists: map[string]bool{}}
	provSvc := provisioning.New(st.DB(), st, auditLog,
		func(context.Context, int64) (provisioning.PanelClient, error) { return purgePanel, nil })
	env.provPanel = purgePanel

	env.srv = httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		AdminAudit:    auditLog,
		Notifications: st,
		VPNs:          provSvc,
		VPNLister:     httpapi.NewSQLVPNLister(st.DB()),
		Releases:      svc,
		Audit:         st,
		Quota:         st,
	}))
	t.Cleanup(env.srv.Close)
	return env
}

// purgePanelStub records whether the panel was touched at all (P-3 asserts it is
// not). A purge retires bookkeeping; it must never become a revocation.
type purgePanelStub struct {
	deletes int
	exists  map[string]bool
}

func (p *purgePanelStub) AddClient(_ context.Context, _ panel.AddClientRequest) error { return nil }

func (p *purgePanelStub) ClientExists(_ context.Context, email string) (bool, error) {
	return p.exists[email], nil
}

func (p *purgePanelStub) DeleteClient(_ context.Context, _ string) error {
	p.deletes++
	return nil
}

func (p *purgePanelStub) SetClientEnabled(_ context.Context, _ string, _ bool) error { return nil }

func (p *purgePanelStub) Links(_ context.Context, _ string) ([]string, error) { return nil, nil }

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

// releaseErrorCode reads the envelope's error code. Declared here because this
// file is `package httpapi_test` and cannot see the internal-package helper.
func releaseErrorCode(body map[string]any) string {
	if errObj, ok := body["error"].(map[string]any); ok {
		if code, ok := errObj["code"].(string); ok {
			return code
		}
	}
	return ""
}

// TestReleaseGraceAndLocalOnlyRemoval is the D-027 regression test.
//
// It walks the whole lifecycle with a controllable clock:
//   - release opens a 3-day window and deletes nothing;
//   - restore inside the window cancels the removal;
//   - a second release whose window expires is removed LOCALLY, its assignment
//     goes with it, the audit entry states the panel client was kept, and a
//     restored VPN is never touched by the purge;
//   - only the owner may release, EXCEPT the Super Admin (P-2), who must give a
//     reason.
func TestReleaseGraceAndLocalOnlyRemoval(t *testing.T) {
	env := newReleaseEnv(t)
	root := bootstrapSuperAdmin(t, env.srv.URL)

	client, clientID := env.signupClient(t, "rel-owner@example.com")
	env.signupClient(t, "rel-other@example.com")

	vpnA := env.seedOwnedVPN(t, clientID, clientID, 9401)
	vpnB := env.seedOwnedVPN(t, clientID, clientID, 9402)

	// --- P-2: an operator CAN release a client's VPN, but must say why -------
	//
	// The old rule was an outright 403. That left the platform owner unable to act
	// on an abused account or a support request except through the 3X-UI panel,
	// where nothing is recorded and no undo window opens. A forced release is now
	// allowed but carries a MANDATORY reason, and it is audited as
	// `vpn.force_release` rather than `vpn.release`.
	resp, body := postJSON(t, root, env.srv.URL+"/api/vpns/"+usageID(vpnB)+"/release", map[string]any{})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("super admin release with no reason = %d (%v), want 422", resp.StatusCode, body)
	}
	if code := releaseErrorCode(body); code != "reason_required" {
		t.Fatalf("error code = %q, want reason_required", code)
	}
	// The forced release itself is asserted in TestSuperAdminReleaseRequiresAReason.

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

// TestPurgeRouteRefusesResellersAndAnonymous is the transport half of P-3.
// The service refuses a non-Super-Admin, but a route that let the request through
// would still be a hole the service has to catch on every path. The route refuses
// FIRST, and it refuses anonymous callers as anonymous — not as "forbidden", which
// would tell them the route exists.
func TestPurgeRouteRefusesResellersAndAnonymous(t *testing.T) {
	env := newReleaseEnv(t)
	root := bootstrapSuperAdmin(t, env.srv.URL)
	_, clientID := env.signupClient(t, "purge-owner@example.com")
	vpn := env.seedOwnedVPN(t, clientID, clientID, 9601)
	path := env.srv.URL + "/api/vpns/" + usageID(vpn) + "/purge"

	// Anonymous: no session cookie at all. A raw request, because the shared
	// `postJSON` helper dereferences its client — passing nil would test the helper,
	// not the route.
	anonReq, _ := http.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
	anonReq.Header.Set("Content-Type", "application/json")
	anonRec := httptest.NewRecorder()
	env.srv.Config.Handler.ServeHTTP(anonRec, anonReq)
	if anonRec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous purge = %d (%s), want 401", anonRec.Code, anonRec.Body.String())
	}

	// The VPN's own owner is a client, not an operator.
	owner, _ := env.signupClient(t, "purge-owner2@example.com")
	resp, body := postJSON(t, owner, path, map[string]any{})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a client purging a VPN = %d (%v), want 403", resp.StatusCode, body)
	}

	// A reseller is the interesting case: they hold PermVPNAssignPremium and reach
	// the VPN console, so "the sidebar hides it" is not a control. Created directly
	// in the database and signed in through the normal door, because this env mounts
	// only the release/vpn routes — the staff console's own creation path is covered
	// by staff_admin_test.go.
	if _, err := env.st.DB().Exec(
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, quota_limit, created_at, updated_at)
		 VALUES ('purge-admin@example.com', ?, 'Reseller', 'admin', 'ACTIVE', 1, 5, 1, 1)`,
		testPasswordHash(t, "a very long password"),
	); err != nil {
		t.Fatalf("seed reseller: %v", err)
	}
	admin := loginAnyDoor(t, env.srv.URL, "purge-admin@example.com", "a very long password")
	resp, body = postJSON(t, admin, path, map[string]any{})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a reseller purging a VPN = %d (%v), want 403", resp.StatusCode, body)
	}

	// The Super Admin without the confirmation gets 422 and is told what to type.
	resp, body = postJSON(t, root, path, map[string]any{})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("super admin purge without confirmation = %d (%v), want 422", resp.StatusCode, body)
	}
	if code := releaseErrorCode(body); code != "confirm_panel_email_required" {
		t.Fatalf("error code = %q, want confirm_panel_email_required", code)
	}
}

// Before this change the Super Admin was refused with 403 — the platform owner
// could not release a customer's VPN at all. Routing that action through the 3X-UI
// panel instead left NO audit entry and opened NO undo window, so the "safe"
// refusal produced the less safe outcome.
//
// What is asserted here:
//   - with a reason, the operator's release SUCCEEDS (201)
//   - without one, it is refused (422 reason_required) — an operator acting on
//     someone else's property must say why
//   - the audit trail records `vpn.force_release`, NOT `vpn.release`, so the two
//     events are never confused months later
//   - it is still a RELEASE, not a deletion: the VPN row survives and an undo
//     window opened
func TestSuperAdminReleaseRequiresAReason(t *testing.T) {
	env := newReleaseEnv(t)
	root := bootstrapSuperAdmin(t, env.srv.URL)

	_, clientID := env.signupClient(t, "p2-owner@example.com")
	vpn := env.seedOwnedVPN(t, clientID, clientID, 9501)

	resp, body := postJSON(t, root, env.srv.URL+"/api/vpns/"+usageID(vpn)+"/release",
		map[string]any{"reason": "abuse report received from the upstream provider"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("super admin forced release = %d (%v), want 201", resp.StatusCode, body)
	}
	if dl, ok := body["restore_deadline"].(float64); !ok || dl <= 0 {
		t.Fatalf("a forced release opened no undo window: %v", body["restore_deadline"])
	}

	// The trail must name the FORCED action, with the reason attached.
	var action, ctxJSON string
	if err := env.st.DB().QueryRow(
		`SELECT action, context FROM audit_logs WHERE action IN (?, ?) ORDER BY id DESC LIMIT 1`,
		audit.ActionVPNForceRelease, audit.ActionVPNRelease,
	).Scan(&action, &ctxJSON); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if action != audit.ActionVPNForceRelease {
		t.Fatalf("audited as %q, want %q — a forced release must be distinguishable from the owner's own", action, audit.ActionVPNForceRelease)
	}
	if !strings.Contains(ctxJSON, "abuse report") {
		t.Fatalf("the mandatory reason never reached the trail: %s", ctxJSON)
	}

	// A release is never a deletion (D-027.3).
	var still int
	if err := env.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances WHERE id = ?`, vpn).Scan(&still); err != nil {
		t.Fatalf("count vpn: %v", err)
	}
	if still != 1 {
		t.Fatalf("the forced release deleted the VPN row (rows = %d); release must never delete", still)
	}
}
