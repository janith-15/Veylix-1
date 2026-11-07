package httpapi

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// captureMail records what would have been sent.
type captureMail struct {
	configured bool
	to         []string
}

func (m *captureMail) Configured() bool { return m.configured }
func (m *captureMail) Send(_ context.Context, to, _, _, _ string) error {
	m.to = append(m.to, to)
	return nil
}

// D-078 — an unverified account can ask for a NEW confirmation link.
//
// The defect: the only resend route required a session, and an unverified account
// can never obtain one (`Authenticate` refuses it with `email_unverified` before
// any cookie is issued). So the route to a fresh link required the one thing the
// person could not have: sign up, get locked out, and no way back in.
func TestUnverifiedAccountCanRequestANewLink(t *testing.T) {
	env := newResendEnv(t, true)
	env.registerUnverified(t, "stuck@example.com")

	before := len(env.mail.to)
	resp, body := env.post(t, "/api/auth/verify/resend",
		map[string]string{"identifier": "stuck@example.com"})
	if resp != http.StatusAccepted {
		t.Fatalf("resend = %d %v, want 202", resp, body)
	}
	if len(env.mail.to) != before+1 {
		t.Fatalf("mails sent = %d, want %d — the link was never re-issued",
			len(env.mail.to), before+1)
	}
	if got := env.mail.to[len(env.mail.to)-1]; got != "stuck@example.com" {
		t.Errorf("mailed %q, want stuck@example.com", got)
	}
}

// The answer must not reveal whether an address is registered: a public endpoint
// that said "no such account" would be an existence oracle.
func TestResendAnswersTheSameForUnknownAndVerified(t *testing.T) {
	env := newResendEnv(t, true)

	unknown, ubody := env.post(t, "/api/auth/verify/resend",
		map[string]string{"identifier": "nobody@example.com"})
	verified, vbody := env.post(t, "/api/auth/verify/resend",
		map[string]string{"identifier": "owner@veylix.test"})

	if unknown != verified {
		t.Fatalf("unknown = %d, verified = %d — the two must be indistinguishable",
			unknown, verified)
	}
	if unknown != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", unknown)
	}
	// Same wording too: a different note would leak just as much as a status.
	if note(ubody) != note(vbody) {
		t.Errorf("notes differ:\n unknown: %q\n verified: %q", note(ubody), note(vbody))
	}
	// And nothing was sent for either: `env.mail.to` only holds the sign-up mail.
	if len(env.mail.to) != 1 {
		t.Errorf("mails sent = %d, want only the original sign-up", len(env.mail.to))
	}
}

// No relay configured must still be answered generically, and must not send.
func TestResendIsGenericWithNoRelay(t *testing.T) {
	env := newResendEnv(t, false)
	env.registerUnverified(t, "stuck@example.com")

	resp, body := env.post(t, "/api/auth/verify/resend",
		map[string]string{"identifier": "stuck@example.com"})
	if resp != http.StatusAccepted {
		t.Fatalf("resend = %d %v, want 202", resp, body)
	}
	if len(env.mail.to) != 0 {
		t.Errorf("a relay-less server reported a send: %v", env.mail.to)
	}
}

// note reads the fixed message, which must never vary by outcome.
func note(body map[string]any) string {
	s, _ := body["note"].(string)
	return s
}

// resendEnv is the real router with the public resend route mounted.
type resendEnv struct {
	srv  *httptest.Server
	st   *store.Store
	mail *captureMail
	c    *http.Client
}

func (e *resendEnv) post(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	return d075Do(t, e.c, http.MethodPost, e.srv.URL+path, body)
}

// registerUnverified creates an account exactly as a customer would.
func (e *resendEnv) registerUnverified(t *testing.T, email string) {
	t.Helper()
	if resp, body := e.post(t, "/api/auth/register", map[string]string{
		"email": email, "password": "a very long password",
	}); resp != http.StatusAccepted {
		t.Fatalf("register = %d %v, want 202", resp, body)
	}
}

// newResendEnv builds a real router whose e-mail verification is wired.
func newResendEnv(t *testing.T, relayConfigured bool) *resendEnv {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "resend.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	auditLog := audit.New(st.DB())
	mail := &captureMail{configured: relayConfigured}
	verifier := &auth.EmailVerifier{Store: st, Mail: mail, BaseURL: "https://vpn.example"}
	authSvc := auth.NewService(st.DB(), auditLog) // autoVerify false, as in production
	authSvc.SendVerification = func(ctx context.Context, userID int64, email string) (string, error) {
		_, delivery, err := verifier.IssueAndSend(ctx, userID, email)
		return delivery, err
	}
	// A verified account, so the "already confirmed" branch is reachable.
	if _, err := st.DB().Exec(
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES ('owner@veylix.test', 'x', 'Owner', 'super_admin', 'ACTIVE', 1, 1, 1)`); err != nil {
		t.Fatalf("seed owner: %v", err)
	}

	sessions := auth.NewSessionStore(st.DB(), false)
	srv := httptest.NewServer(Router(Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		EmailVerifier: verifier,
	}))
	t.Cleanup(srv.Close)

	return &resendEnv{srv: srv, st: st, mail: mail, c: newJarClient(t)}
}

// newJarClient is a plain client with a cookie jar.
func newJarClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}
}
