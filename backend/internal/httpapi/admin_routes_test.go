package httpapi_test

import (
	"net/http"
	"testing"
)

// TestAdminRoutesRequireSuperAdmin proves the account API cannot be used to
// escalate privileges from an admin or client session.
func TestAdminRoutesRequireSuperAdmin(t *testing.T) {
	srv, _ := newHarness(t)
	root := bootstrapSuperAdmin(t, srv.URL)

	postJSON(t, root, srv.URL+"/api/admins", map[string]any{
		"email": "reseller@example.com", "password": "a very long password", "role": "admin",
	})
	admin := loginAs(t, srv.URL, "reseller@example.com", "a very long password")

	// An admin may not create accounts — and certainly not a super admin.
	if resp, _ := postJSON(t, admin, srv.URL+"/api/admins", map[string]any{
		"email": "sneaky@example.com", "password": "a very long password", "role": "super_admin",
	}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("admin creating a super admin = %d, want 403", resp.StatusCode)
	}
	// An admin may not list accounts.
	if resp, _ := getJSON(t, admin, srv.URL+"/api/admins"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("admin listing accounts = %d, want 403", resp.StatusCode)
	}

	// A client may not either.
	clientC := &http.Client{Jar: mustJar(t), Timeout: 0}
	postJSON(t, clientC, srv.URL+"/api/auth/register", map[string]string{
		"email": "client@example.com", "password": "a very long password",
	})
	if resp, _ := getJSON(t, clientC, srv.URL+"/api/admins"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("client listing accounts = %d, want 403", resp.StatusCode)
	}

	// Anonymous callers are refused outright.
	anon := &http.Client{Jar: mustJar(t)}
	if resp, _ := getJSON(t, anon, srv.URL+"/api/admins"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous listing = %d, want 401", resp.StatusCode)
	}
}

// TestCreateAdminRejectsWeakPasswordAndClients checks the shared policy and the
// "clients self-register" rule at the API edge.
func TestCreateAdminRejectsWeakPasswordAndClients(t *testing.T) {
	srv, _ := newHarness(t)
	root := bootstrapSuperAdmin(t, srv.URL)

	if resp, _ := postJSON(t, root, srv.URL+"/api/admins", map[string]any{
		"email": "weak@example.com", "password": "aaaaaaaaaaaa", "role": "admin",
	}); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("weak password = %d, want 422", resp.StatusCode)
	}

	if resp, _ := postJSON(t, root, srv.URL+"/api/admins", map[string]any{
		"email": "c2@example.com", "password": "a very long password", "role": "client",
	}); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("client role = %d, want 422", resp.StatusCode)
	}

	if resp, _ := postJSON(t, root, srv.URL+"/api/admins", map[string]any{
		"email": "dup@example.com", "password": "a very long password", "role": "admin",
	}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("first create = %d", resp.StatusCode)
	}
	if resp, _ := postJSON(t, root, srv.URL+"/api/admins", map[string]any{
		"email": "dup@example.com", "password": "a very long password", "role": "admin",
	}); resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate email = %d, want 409", resp.StatusCode)
	}
}

// TestSuspendAdminRevokesLiveSession proves a suspension takes effect on
// sessions that already exist, not just on the next login.
func TestSuspendAdminRevokesLiveSession(t *testing.T) {
	srv, _ := newHarness(t)
	root := bootstrapSuperAdmin(t, srv.URL)

	postJSON(t, root, srv.URL+"/api/admins", map[string]any{
		"email": "reseller@example.com", "password": "a very long password", "role": "admin",
	})
	admin := loginAs(t, srv.URL, "reseller@example.com", "a very long password")
	if resp, _ := getJSON(t, admin, srv.URL+"/api/me"); resp.StatusCode != http.StatusOK {
		t.Fatal("admin session should work before suspension")
	}

	resp, body := send(t, root, http.MethodPatch, srv.URL+"/api/admins/2", map[string]any{"status": "SUSPENDED"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("suspend = %d (%v)", resp.StatusCode, body)
	}

	// The live session is dead immediately.
	if resp, _ := getJSON(t, admin, srv.URL+"/api/me"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("suspended admin session = %d, want 401", resp.StatusCode)
	}
	// And the credential no longer signs in.
	if resp, _ := postJSON(t, admin, srv.URL+"/api/auth/login", loginPayload("reseller@example.com", "a very long password")); resp.StatusCode != http.StatusForbidden {
		t.Errorf("suspended login = %d, want 403", resp.StatusCode)
	}
}

// TestSuperAdminCannotSuspendSelf is the lockout-prevention regression test.
func TestSuperAdminCannotSuspendSelf(t *testing.T) {
	srv, _ := newHarness(t)
	root := bootstrapSuperAdmin(t, srv.URL)

	if resp, _ := send(t, root, http.MethodPatch, srv.URL+"/api/admins/1",
		map[string]any{"status": "SUSPENDED"}); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("self-suspend = %d, want 422 (lockout prevention)", resp.StatusCode)
	}
	if resp, _ := getJSON(t, root, srv.URL+"/api/me"); resp.StatusCode != http.StatusOK {
		t.Error("a refused self-suspend must not break the caller's own session")
	}
}

// TestSetQuotaLimitOnNonAdminIsNotFound avoids leaking which accounts are
// admins: a quota change against a client reads as "not found".
func TestSetQuotaLimitOnNonAdminIsNotFound(t *testing.T) {
	srv, _ := newHarness(t)
	root := bootstrapSuperAdmin(t, srv.URL)

	c := &http.Client{Jar: mustJar(t)}
	postJSON(t, c, srv.URL+"/api/auth/register", map[string]string{
		"email": "client@example.com", "password": "a very long password",
	})
	// The client is user #2.
	if resp, _ := send(t, root, http.MethodPatch, srv.URL+"/api/admins/2",
		map[string]any{"quota_limit": 10}); resp.StatusCode != http.StatusNotFound {
		t.Errorf("quota on a client = %d, want 404", resp.StatusCode)
	}
}
