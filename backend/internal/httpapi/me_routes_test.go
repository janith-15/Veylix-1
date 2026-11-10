package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/config"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/store"
)

// meHarness builds the real router over a real SQLite file WITH the Accounts
// reader wired, which is what main.go does. newHarness (auth_e2e_test.go)
// deliberately leaves it nil, so the two helpers together prove both branches of
// the D1 fallback.
func meHarness(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/me.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	auditLog := audit.New(st.DB())
	authSvc := auth.NewService(st.DB(), auditLog)
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(st.DB(), false)

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test", Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		Accounts: st,
		// D-099: the zone a real deployment always publishes. Without it here,
		// /api/me would answer without `display_timezone` and every screen would
		// quietly render in the device's zone — so the harness carries it, not
		// just the assertion.
		DisplayTimezone: config.DefaultDisplayTimezone,
	}))
	t.Cleanup(srv.Close)
	return srv, st
}

// TestMeReturnsDisplayNameAndCreatedAt is the D1/D2 gate.
//
// It exists because GET /api/me used to answer {id, email, role} only, while the
// UI's toUser() reads `display_name` and falls back to the e-mail prefix. A user
// called "Nimal Perera" therefore became "nimal" after every reload — silently,
// because a wrong name still renders. Revert the handler and this fails on the
// display name; drop created_at from the query and it fails on that.
func TestMeReturnsDisplayNameAndCreatedAt(t *testing.T) {
	srv, _ := meHarness(t)
	client := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}

	const pw = "a very long password"
	before := time.Now().Add(-time.Minute).Unix()
	resp, body := postJSON(t, client, srv.URL+"/api/setup/bootstrap", map[string]string{
		"email": "nimal@example.com", "password": pw, "display_name": "Nimal Perera",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("bootstrap = %d %v", resp.StatusCode, body)
	}

	resp, me := getJSON(t, client, srv.URL+"/api/me")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/me = %d, want 200", resp.StatusCode)
	}

	// The display name the account was created with — not the e-mail prefix.
	if got := me["display_name"]; got != "Nimal Perera" {
		t.Errorf("display_name = %v, want \"Nimal Perera\" (the UI falls back to %q when this is absent)",
			got, "nimal")
	}
	// The session's own facts must still be present and correct.
	if got := me["email"]; got != "nimal@example.com" {
		t.Errorf("email = %v, want nimal@example.com", got)
	}
	if got := me["role"]; got != "super_admin" {
		t.Errorf("role = %v, want super_admin", got)
	}

	// created_at must be a real timestamp, not zero and not in the future.
	raw, ok := me["created_at"].(float64)
	if !ok {
		t.Fatalf("created_at missing or not a number: %v (%T)", me["created_at"], me["created_at"])
	}
	created := int64(raw)
	if created < before {
		t.Errorf("created_at = %d, want >= %d (a real creation time)", created, before)
	}
	if created > time.Now().Add(time.Minute).Unix() {
		t.Errorf("created_at = %d is in the future", created)
	}

	// A successful read must NOT be flagged as a failed one.
	if flagged, _ := me["profile_read_error"].(bool); flagged {
		t.Error("profile_read_error is set on a read that succeeded")
	}
}

// TestMePublishesTheDisplayTimezone is D-099's contract with the UI.
//
// Every screen renders stored instants in this zone. If /api/me stopped sending
// it, each formatter would silently fall back to the DEVICE's zone — the exact
// defect this decision removed — and nothing else in the product would fail.
func TestMePublishesTheDisplayTimezone(t *testing.T) {
	srv, _ := meHarness(t)
	client := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}

	// Bootstrap signs the caller in, so the very next call can read /api/me.
	// Creating a real account rather than faking a principal keeps this on the
	// same path a customer takes.
	resp, body := postJSON(t, client, srv.URL+"/api/setup/bootstrap", map[string]string{
		"email": "tz@example.com", "password": "a very long password",
		"display_name": "Timezone Check",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("bootstrap = %d %v", resp.StatusCode, body)
	}

	resp, me := getJSON(t, client, srv.URL+"/api/me")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/me = %d, want 200", resp.StatusCode)
	}

	zone, ok := me["display_timezone"].(string)
	if !ok || zone == "" {
		t.Fatalf("display_timezone = %v, want a zone name (the UI has no other source)",
			me["display_timezone"])
	}
	// A zone NAME, not an offset: "+05:30" would go stale the moment the zone's
	// rules changed, and a browser could not resolve it to a location.
	if _, err := time.LoadLocation(zone); err != nil {
		t.Errorf("display_timezone = %q is not a loadable IANA zone: %v", zone, err)
	}
}

// TestMeStaysUsableWhenTheAccountCannotBeRead is the other half of D1.
//
// The route must degrade to the session's own facts rather than failing the
// caller out: the user is demonstrably signed in, and a 500 here would log them
// out of a working session because a name could not be read.
func TestMeStaysUsableWhenTheAccountCannotBeRead(t *testing.T) {
	// newHarness leaves Deps.Accounts nil, which is exactly a deployment that
	// has not wired the reader.
	srv, client := newHarness(t)

	const pw = "a very long password"
	resp, body := postJSON(t, client, srv.URL+"/api/setup/bootstrap", map[string]string{
		"email": "fallback@example.com", "password": pw, "display_name": "Fallback Name",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("bootstrap = %d %v", resp.StatusCode, body)
	}

	resp, me := getJSON(t, client, srv.URL+"/api/me")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/me without an Accounts reader = %d, want 200 (the session is valid)",
			resp.StatusCode)
	}
	if got := me["email"]; got != "fallback@example.com" {
		t.Errorf("email = %v, want fallback@example.com", got)
	}
	// Absent, never wrong: the UI's fallback then applies.
	if _, present := me["display_name"]; present {
		t.Error("display_name is present although no reader is wired; it must be absent, not guessed")
	}
}

// TestLogoutOthersKeepsThisSession is the D3 gate.
//
// Before this route existed, RevokeAllForUser was reachable only through a
// password change, so a user who had left a session open on a lost laptop had no
// way to end it. The route must therefore end the OTHER sessions and leave the
// caller signed in — signing yourself out is not the feature.
func TestLogoutOthersKeepsThisSession(t *testing.T) {
	srv, _ := meHarness(t)
	const pw = "a very long password"
	const email = "multi@example.com"

	// Three browsers, three sessions, one account.
	here := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	if resp, body := postJSON(t, here, srv.URL+"/api/setup/bootstrap", map[string]string{
		"email": email, "password": pw, "display_name": "Multi Device",
	}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("bootstrap = %d %v", resp.StatusCode, body)
	}

	other := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	if resp, body := postJSON(t, other, srv.URL+"/api/auth/login", map[string]string{
		"email": email, "password": pw, "portal": "staff",
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("second login = %d %v", resp.StatusCode, body)
	}

	third := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	if resp, body := postJSON(t, third, srv.URL+"/api/auth/login", map[string]string{
		"email": email, "password": pw, "portal": "staff",
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("third login = %d %v", resp.StatusCode, body)
	}

	// All three really are signed in before the action.
	for name, c := range map[string]*http.Client{"here": here, "other": other, "third": third} {
		if resp, _ := getJSON(t, c, srv.URL+"/api/me"); resp.StatusCode != http.StatusOK {
			t.Fatalf("%s is not signed in before the call: %d", name, resp.StatusCode)
		}
	}

	resp, body := postJSON(t, here, srv.URL+"/api/auth/logout-others", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout-others = %d %v, want 200", resp.StatusCode, body)
	}
	// Exactly the two other sessions, not one and not three.
	if got, _ := body["ended"].(float64); int(got) != 2 {
		t.Errorf("ended = %v, want 2 (the other two sessions)", body["ended"])
	}

	// The caller is STILL signed in — that is the whole point of "others".
	if resp, _ := getJSON(t, here, srv.URL+"/api/me"); resp.StatusCode != http.StatusOK {
		t.Errorf("the caller was signed out: /api/me = %d, want 200", resp.StatusCode)
	}
	// The other two are gone.
	for name, c := range map[string]*http.Client{"other": other, "third": third} {
		if resp, _ := getJSON(t, c, srv.URL+"/api/me"); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s is still signed in: /api/me = %d, want 401", name, resp.StatusCode)
		}
	}
}

// TestLogoutOthersRequiresASession proves the route is not a way for an
// unauthenticated caller to end somebody else's sessions.
func TestLogoutOthersRequiresASession(t *testing.T) {
	srv, _ := meHarness(t)
	anon := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}

	resp, _ := postJSON(t, anon, srv.URL+"/api/auth/logout-others", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous logout-others = %d, want 401", resp.StatusCode)
	}
}
