package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/store"
)

// newHarness builds the real router over a real SQLite file. No mocks.
func newHarness(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/e2e.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	auditLog := audit.New(st.DB())
	authSvc := auth.NewService(st.DB(), auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(st.DB(), false)

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test", Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
	}))
	t.Cleanup(srv.Close)

	return srv, &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
}

// mustJar returns a cookie jar that also stores cookies for plain-HTTP test
// servers. A real browser treats http://localhost as a trustworthy origin and
// stores Secure cookies there; net/http/cookiejar does not, so without this
// every httptest-based authenticated flow would look like a 401.
func mustJar(t *testing.T) http.CookieJar {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &loopbackJar{CookieJar: jar}
}

type loopbackJar struct{ http.CookieJar }

func (j *loopbackJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		mapped := make([]*http.Cookie, len(cookies))
		for i, c := range cookies {
			clone := *c
			clone.Secure = false
			mapped[i] = &clone
		}
		j.CookieJar.SetCookies(u, mapped)
		return
	}
	j.CookieJar.SetCookies(u, cookies)
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func postJSON(t *testing.T, c *http.Client, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := c.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func getJSON(t *testing.T, c *http.Client, url string) (*http.Response, map[string]any) {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestAuthFlowEndToEnd(t *testing.T) {
	srv, client := newHarness(t)
	const pw = "a very long password"

	// 1. Anonymous access to a protected route is refused.
	if resp, _ := getJSON(t, client, srv.URL+"/api/me"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/api/me anonymous = %d, want 401", resp.StatusCode)
	}

	// 2. Register → session cookie issued, account is a client.
	resp, body := postJSON(t, client, srv.URL+"/api/auth/register", map[string]string{
		"email": "e2e@example.com", "password": pw, "display_name": "E2E",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register = %d (%v)", resp.StatusCode, body)
	}
	if body["role"] != "client" {
		t.Fatalf("role = %v, want client", body["role"])
	}
	if len(resp.Cookies()) == 0 {
		t.Fatal("no session cookie issued")
	}
	if !resp.Cookies()[0].HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}

	// 3. The session resolves.
	resp, body = getJSON(t, client, srv.URL+"/api/me")
	if resp.StatusCode != http.StatusOK || body["role"] != "client" {
		t.Fatalf("/api/me = %d %v", resp.StatusCode, body)
	}

	// 4. Logout invalidates it.
	if resp, _ := postJSON(t, client, srv.URL+"/api/auth/logout", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d", resp.StatusCode)
	}
	if resp, _ := getJSON(t, client, srv.URL+"/api/me"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/api/me after logout = %d, want 401", resp.StatusCode)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	srv, client := newHarness(t)
	const pw = "a very long password"
	postJSON(t, client, srv.URL+"/api/auth/register", map[string]string{"email": "w@example.com", "password": pw})

	resp, body := postJSON(t, client, srv.URL+"/api/auth/login", loginPayload("w@example.com", "definitely wrong"))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login = %d, want 401", resp.StatusCode)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "invalid_credentials" {
		t.Fatalf("code = %v, want invalid_credentials", errObj["code"])
	}
}

func TestBootstrapThenLoginAsSuperAdmin(t *testing.T) {
	srv, client := newHarness(t)
	const pw = "a very long password"

	resp, body := postJSON(t, client, srv.URL+"/api/setup/bootstrap", map[string]string{
		"email": "owner@example.com", "password": pw, "display_name": "Owner",
	})
	if resp.StatusCode != http.StatusCreated || body["role"] != "super_admin" {
		t.Fatalf("bootstrap = %d %v", resp.StatusCode, body)
	}

	// A second bootstrap must be refused (no privilege escalation).
	if resp, _ := postJSON(t, client, srv.URL+"/api/setup/bootstrap", map[string]string{
		"email": "other@example.com", "password": pw,
	}); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second bootstrap = %d, want 409", resp.StatusCode)
	}

	// D-041: a super admin belongs at the STAFF door. The client door must refuse
	// it (with a precise code, so the UI can point at /staff/login), and the staff
	// door must accept it.
	if resp, body := postJSON(t, client, srv.URL+"/api/auth/login", map[string]string{
		"email": "owner@example.com", "password": pw, "portal": "client",
	}); resp.StatusCode != http.StatusForbidden || errCode(body) != "wrong_portal" {
		t.Fatalf("super admin at the client door = %d %v, want 403 wrong_portal", resp.StatusCode, body)
	}
	if resp, body := postJSON(t, client, srv.URL+"/api/auth/login", map[string]string{
		"email": "owner@example.com", "password": pw, "portal": "staff",
	}); resp.StatusCode != http.StatusOK || body["role"] != "super_admin" {
		t.Fatalf("super admin at the staff door = %d %v, want 200 super_admin", resp.StatusCode, body)
	}
}

// errCode pulls `error.code` out of a standard error envelope.
func errCode(body map[string]any) string {
	errObj, _ := body["error"].(map[string]any)
	code, _ := errObj["code"].(string)
	return code
}

// F-016: the password can be changed through the API, the old credential stops
// working immediately, and every other session is revoked (the caller's own
// browser is re-issued a cookie so it stays signed in).
func TestChangePasswordRotatesSessions(t *testing.T) {
	srv, client := newHarness(t)
	const oldPW = "a very long password"
	const newPW = "an entirely different passphrase"

	postJSON(t, client, srv.URL+"/api/auth/register", map[string]string{
		"email": "rot@example.com", "password": oldPW, "display_name": "Rot",
	})

	// A second login = a second device holding a second session for the account.
	browser2 := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	if resp, body := postJSON(t, browser2, srv.URL+"/api/auth/login", loginPayload("rot@example.com", oldPW)); resp.StatusCode != http.StatusOK {
		t.Fatalf("second login = %d (%v)", resp.StatusCode, body)
	}
	if resp, _ := getJSON(t, browser2, srv.URL+"/api/me"); resp.StatusCode != http.StatusOK {
		t.Fatalf("second browser /api/me = %d", resp.StatusCode)
	}

	// Wrong current password is rejected and changes nothing.
	resp, body := postJSON(t, client, srv.URL+"/api/auth/change-password", map[string]string{
		"current_password": "wrong password here", "new_password": newPW,
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("wrong current = %d (%v), want 422", resp.StatusCode, body)
	}
	if errObj, _ := body["error"].(map[string]any); errObj["code"] != "password_mismatch" {
		t.Fatalf("code = %v, want password_mismatch", errObj["code"])
	}

	// A guessable new password is rejected by the policy (F-017).
	if resp, body := postJSON(t, client, srv.URL+"/api/auth/change-password", map[string]string{
		"current_password": oldPW, "new_password": "aaaaaaaaaaaa",
	}); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("weak new = %d (%v), want 422", resp.StatusCode, body)
	}

	// The real change succeeds. 200 (not 204) because the body now reports how the
	// notification e-mail was delivered, so the UI can say what really happened.
	resp, body = postJSON(t, client, srv.URL+"/api/auth/change-password", map[string]string{
		"current_password": oldPW, "new_password": newPW,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("change = %d (%v), want 200", resp.StatusCode, body)
	}
	if body["changed"] != true {
		t.Fatalf("change body = %v, want changed:true", body)
	}
	if d, _ := body["delivery"].(string); d != "smtp" && d != "outbox" {
		t.Fatalf("delivery = %v, want smtp or outbox (never an assumed send)", body["delivery"])
	}

	// The caller's browser is still signed in (fresh cookie issued)...
	if resp, _ := getJSON(t, client, srv.URL+"/api/me"); resp.StatusCode != http.StatusOK {
		t.Errorf("/api/me after change = %d, want 200 (cookie must be re-issued)", resp.StatusCode)
	}
	// ...but every other session was revoked.
	if resp, _ := getJSON(t, browser2, srv.URL+"/api/me"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("other device after change = %d, want 401 (sessions must rotate)", resp.StatusCode)
	}

	// The new credential works and the old one is dead.
	anon := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	if resp, _ := postJSON(t, anon, srv.URL+"/api/auth/login", loginPayload("rot@example.com", newPW)); resp.StatusCode != http.StatusOK {
		t.Errorf("login with the new password = %d, want 200", resp.StatusCode)
	}
	if resp, _ := postJSON(t, anon, srv.URL+"/api/auth/login", loginPayload("rot@example.com", oldPW)); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("login with the old password = %d, want 401", resp.StatusCode)
	}
}

func TestChangePasswordRequiresAuthentication(t *testing.T) {
	srv, client := newHarness(t)
	resp, _ := postJSON(t, client, srv.URL+"/api/auth/change-password", map[string]string{
		"current_password": "x", "new_password": "a very long password",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous change = %d, want 401", resp.StatusCode)
	}
}

// send issues an arbitrary-method request with an optional JSON body.
func send(t *testing.T, c *http.Client, method, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	reader := bytes.NewReader(nil)
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// loginPayload builds a /api/auth/login body that declares the right portal.
//
// Since D-041 the server refuses a client at the staff door and vice versa, so a
// test must say which door it means. Staff test accounts are spelled with
// "reseller"/"owner"/"admin" (and are created by a super admin); everything else is
// a client. Explicit is better here: a wrong guess fails loudly, not silently.
// signInVia opens a login attempt through the given door.
func signInVia(t *testing.T, srvURL, email, password, portal string) (*http.Response, map[string]any) {
	t.Helper()
	c := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	return postJSON(t, c, srvURL+"/api/auth/login",
		map[string]string{"email": email, "password": password, "portal": portal})
}

// loginAnyDoor signs in through whichever portal the account belongs to.
//
// A test knows an account's credentials, not which door it uses: since D-041 the
// server refuses a client at /staff/login and staff at /login. Trying the client
// door first and the staff door second keeps every flow test honest about WHAT it
// is testing (authorization on the route) without encoding the door in its name.
func loginAnyDoor(t *testing.T, srvURL, email, password string) *http.Client {
	t.Helper()
	for _, portal := range []string{"client", "staff"} {
		c := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
		resp, body := postJSON(t, c, srvURL+"/api/auth/login",
			map[string]string{"email": email, "password": password, "portal": portal})
		if resp.StatusCode == http.StatusOK {
			return c
		}
		if code, _ := body["error"].(map[string]any); code != nil {
			if c2, ok := code["code"].(string); ok && c2 == "wrong_portal" {
				continue // right credentials, other door
			}
		}
		t.Fatalf("login %s via %s = %d (%v)", email, portal, resp.StatusCode, body)
	}
	t.Fatalf("login %s: refused at both portals", email)
	return nil
}

// loginPayload keeps older call sites working: it asks for the client door and the
// caller is expected to use loginAnyDoor for anything that needs the right one.
func loginPayload(email, password string) map[string]string {
	return map[string]string{"email": email, "password": password, "portal": "client"}
}

// loginAs returns an authenticated client for an existing account.
func loginAs(t *testing.T, srvURL, email, password string) *http.Client {
	t.Helper()
	return loginAnyDoor(t, srvURL, email, password)
}

// bootstrapSuperAdmin creates the first super admin and returns a client for it.
func bootstrapSuperAdmin(t *testing.T, srvURL string) *http.Client {
	t.Helper()
	c := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	resp, body := postJSON(t, c, srvURL+"/api/setup/bootstrap", map[string]string{
		"email": "owner@example.com", "password": "a very long password", "display_name": "Owner",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("bootstrap = %d (%v)", resp.StatusCode, body)
	}
	return c
}

func TestSetupStatusReflectsBootstrap(t *testing.T) {
	srv, client := newHarness(t)

	_, body := getJSON(t, client, srv.URL+"/api/setup/status")
	if body["bootstrap_required"] != true {
		t.Fatalf("fresh install bootstrap_required = %v, want true", body["bootstrap_required"])
	}

	bootstrapSuperAdmin(t, srv.URL)

	_, body = getJSON(t, client, srv.URL+"/api/setup/status")
	if body["bootstrap_required"] != false {
		t.Fatalf("after bootstrap bootstrap_required = %v, want false", body["bootstrap_required"])
	}
}

func TestSuperAdminCreatesAdminWhoCanLogin(t *testing.T) {
	srv, _ := newHarness(t)
	root := bootstrapSuperAdmin(t, srv.URL)

	resp, body := postJSON(t, root, srv.URL+"/api/admins", map[string]any{
		"email": "reseller@example.com", "password": "a very long password",
		"display_name": "Reseller", "role": "admin", "quota_limit": 40,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create admin = %d (%v)", resp.StatusCode, body)
	}
	if body["role"] != "admin" {
		t.Fatalf("role = %v, want admin", body["role"])
	}
	if body["quota_limit"] != float64(40) {
		t.Errorf("quota_limit = %v, want 40", body["quota_limit"])
	}

	// The new admin can actually sign in and lands in the admin role.
	admin := loginAs(t, srv.URL, "reseller@example.com", "a very long password")
	_, me := getJSON(t, admin, srv.URL+"/api/me")
	if me["role"] != "admin" {
		t.Fatalf("/api/me role = %v, want admin", me["role"])
	}

	resp, _ = getJSON(t, root, srv.URL+"/api/admins")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list admins = %d", resp.StatusCode)
	}
}

// A forged/garbage cookie must never authenticate.
func TestForgedSessionCookieIsRejected(t *testing.T) {
	srv, client := newHarness(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "forged-token-value"})

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("forged cookie = %d, want 401", resp.StatusCode)
	}
}

// Cross-origin state-changing requests are blocked (CSRF).
func TestCrossOriginPostIsBlocked(t *testing.T) {
	srv, client := newHarness(t)
	raw, _ := json.Marshal(map[string]string{"email": "x@example.com", "password": "a very long password", "portal": "staff"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/login", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin POST = %d, want 403", resp.StatusCode)
	}
}
