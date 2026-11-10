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

// newQuotaHarness builds the real router over a real SQLite file with the quota
// and audit stores actually wired (newHarness leaves them nil, which disables
// AccountRoutes entirely — a nil handler there would make every assertion below
// vacuous).
func newQuotaHarness(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/quota.db")
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
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		Audit: st, Quota: st,
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestQuotaMeForReseller proves an admin reads its own reseller quota and that
// a super admin may target another account.
func TestQuotaMeForReseller(t *testing.T) {
	srv := newQuotaHarness(t)
	root := bootstrapSuperAdmin(t, srv.URL)

	postJSON(t, root, srv.URL+"/api/admins", map[string]any{
		"email": "reseller@example.com", "password": "a very long password",
		"role": "admin", "quota_limit": 5,
	})
	admin := loginAs(t, srv.URL, "reseller@example.com", "a very long password")

	resp, body := getJSON(t, admin, srv.URL+"/api/quotas/me")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin GET /api/quotas/me = %d (%v), want 200", resp.StatusCode, body)
	}
	if body["limit"] != float64(5) || body["used"] != float64(0) || body["remaining"] != float64(5) {
		t.Errorf("fresh admin quota = %v, want used=0 limit=5 remaining=5", body)
	}

	// The super admin sees any account via ?admin_id=. The admin id is 2 (the
	// owner bootstrapped first).
	resp, body = getJSON(t, root, srv.URL+"/api/quotas/me?admin_id=2")
	if resp.StatusCode != http.StatusOK || body["limit"] != float64(5) {
		t.Errorf("super admin ?admin_id=2 = %d %v, want 200 limit=5", resp.StatusCode, body)
	}
}

// TestQuotaMeForClientIsZeroNotForbidden is the D-021 regression test.
//
// Before the fix a client got 403 on GET /api/quotas/me. A 403 makes the single
// "read my quota" call site in the UI unusable for the most common role, and it
// contradicts docs/API_CONTRACT.md, which documents one response shape. A client
// has no reseller quota, so it now gets that shape with limit=0 and
// remaining=-1 ("unlimited / not applicable").
func TestQuotaMeForClientIsZeroNotForbidden(t *testing.T) {
	srv := newQuotaHarness(t)
	bootstrapSuperAdmin(t, srv.URL)

	client := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	postJSON(t, client, srv.URL+"/api/auth/register", map[string]string{
		"email": "client@example.com", "password": "a very long password",
	})

	resp, body := getJSON(t, client, srv.URL+"/api/quotas/me")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("client GET /api/quotas/me = %d (%v), want 200", resp.StatusCode, body)
	}
	if body["used"] != float64(0) || body["limit"] != float64(0) || body["remaining"] != float64(-1) {
		t.Errorf("client quota = %v, want used=0 limit=0 remaining=-1", body)
	}
}

// TestQuotaMeRequiresAuth keeps the endpoint closed to anonymous callers.
func TestQuotaMeRequiresAuth(t *testing.T) {
	srv := newQuotaHarness(t)
	anon := &http.Client{Jar: mustJar(t)}

	if resp, _ := getJSON(t, anon, srv.URL+"/api/quotas/me"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous GET /api/quotas/me = %d, want 401", resp.StatusCode)
	}
}
