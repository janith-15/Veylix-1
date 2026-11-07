package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// D-076 · BUG 1 — signing up must NOT hand out a session the login door will
// refuse.
//
// The defect: POST /api/auth/register created a session, set the cookie and
// answered 201 for an account whose `email_verified` was 0, while `Authenticate`
// refuses an unverified address with 403 email_unverified. A customer therefore
// ended up "signed in" to an account that could never sign in again — which is
// the "email and password do not work" report. The API now answers honestly: no
// session, and a body that says a confirmation is outstanding.
func TestRegisterDoesNotIssueASessionForAnUnverifiedAccount(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "register.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	auditLog := audit.New(st.DB())
	authSvc := auth.NewService(st.DB(), auditLog) // autoVerify stays false, as in prod
	sessions := auth.NewSessionStore(st.DB(), false)
	srv := httptest.NewServer(Router(Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
	}))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	resp, body := d075Do(t, c, http.MethodPost, srv.URL+"/api/auth/register", map[string]string{
		"email": "newcomer@example.com", "password": "a very long password",
	})

	// NOT 201: that status means "created and signed in", and it was a lie.
	if resp != http.StatusAccepted {
		t.Fatalf("register = %d %v, want 202 — an unverified account must not be "+
			"handed a session it can never reuse", resp, body)
	}
	if pending, _ := body["pending_verification"].(bool); !pending {
		t.Errorf("body = %v, want pending_verification:true so the UI can say "+
			"\"check your e-mail\" instead of silently succeeding", body)
	}
	if verified, ok := body["email_verified"].(bool); !ok || verified {
		t.Errorf("email_verified = %v, want an explicit false", body["email_verified"])
	}

	// And the account cannot sign in yet — the two answers must agree.
	login, _ := d075Do(t, c, http.MethodPost, srv.URL+"/api/auth/login", map[string]string{
		"identifier": "newcomer@example.com", "password": "a very long password",
		"portal": "client",
	})
	if login != http.StatusForbidden {
		t.Errorf("login after register = %d, want 403 email_unverified — register "+
			"and login must tell the same story", login)
	}

	// A cookie handed out here would be the original bug in a quieter form.
	if u, perr := url.Parse(srv.URL); perr == nil {
		if got := jar.Cookies(u); len(got) != 0 {
			t.Errorf("register set %d session cookie(s) for an unverified account", len(got))
		}
	}
}

// D-076 · BUG 2 — the per-client FREE limit the console saves is the one that is
// enforced.
//
// The defect: `MonthlyLimit` ignored its own clientID and returned the platform
// default, so /clients/{id} showed "5 / 10 (own limit)" while the sixth claim was
// refused. `ClientFreeLimitOverride` existed and was tested, but nothing in the
// claim path ever called it.
func TestMonthlyLimitHonoursThePerClientOverride(t *testing.T) {
	ctx, st := newD076Store(t, "limit.db")
	id := mustD076User(t, st, "limit@example.com")

	if got, err := st.MonthlyLimit(ctx, id); err != nil || got != store.FreeMonthlyDefault {
		t.Fatalf("default MonthlyLimit = %d (%v), want %d", got, err, store.FreeMonthlyDefault)
	}

	const raised = 10
	if err := st.ClientSetFreeLimit(ctx, id, raised); err != nil {
		t.Fatalf("ClientSetFreeLimit: %v", err)
	}
	if got, err := st.MonthlyLimit(ctx, id); err != nil || got != raised {
		t.Fatalf("MonthlyLimit = %d (%v), want %d — the console's own limit must be "+
			"the number the claim path enforces", got, err, raised)
	}

	// 0 means "platform default", which is what the console documents it as.
	if err := st.ClientSetFreeLimit(ctx, id, 0); err != nil {
		t.Fatalf("reset limit: %v", err)
	}
	if got, _ := st.MonthlyLimit(ctx, id); got != store.FreeMonthlyDefault {
		t.Errorf("MonthlyLimit after reset = %d, want the default %d", got, store.FreeMonthlyDefault)
	}
}

// And the raised limit actually lets one MORE claim through: the cap is enforced,
// not merely reported.
func TestRaisedLimitAllowsTheNextClaim(t *testing.T) {
	ctx, st := newD076Store(t, "raise.db")
	id := mustD076User(t, st, "raise@example.com")
	server := mustD076Server(t, st)

	// Burn the default allowance. The concurrent cap is 3, so release as we go.
	for i := 0; i < store.FreeMonthlyDefault; i++ {
		email := "raise-" + strconv.Itoa(i)
		if _, err := st.RecordFreeClaim(ctx, nil, id, server, email, store.FreeMonthlyDefault); err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		if err := st.ReleaseFreeClaim(ctx, id, server, email); err != nil {
			t.Fatalf("release %d: %v", i+1, err)
		}
	}

	// The default is now exhausted.
	if _, err := st.RecordFreeClaim(ctx, nil, id, server, "over", store.FreeMonthlyDefault); !errors.Is(err, store.ErrFreeMonthlyLimit) {
		t.Fatalf("claim past the default = %v, want ErrFreeMonthlyLimit", err)
	}

	// Raising the limit must open the door — that is the whole point of the fix.
	if err := st.ClientSetFreeLimit(ctx, id, store.FreeMonthlyDefault+1); err != nil {
		t.Fatalf("ClientSetFreeLimit: %v", err)
	}
	limit, err := st.MonthlyLimit(ctx, id)
	if err != nil {
		t.Fatalf("MonthlyLimit: %v", err)
	}
	if _, err := st.RecordFreeClaim(ctx, nil, id, server, "allowed", limit); err != nil {
		t.Fatalf("claim inside the RAISED limit = %v — the console's override is "+
			"still being ignored by the claim path", err)
	}
}

// newD076Store opens a migrated store in a temp dir.
func newD076Store(t *testing.T, name string) (context.Context, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return ctx, st
}

// mustD076User seeds one active client and returns its id.
func mustD076User(t *testing.T, st *store.Store, email string) int64 {
	t.Helper()
	res, err := st.DB().Exec(
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES (?, 'x', 'U', 'client', 'ACTIVE', 1, 1, 1)`, email)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// mustD076Server seeds one active panel integration and returns its id.
func mustD076Server(t *testing.T, st *store.Store) int64 {
	t.Helper()
	res, err := st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('srv', 'https://p.example/base', x'00', 'ACTIVE', 1, 1)`)
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}
