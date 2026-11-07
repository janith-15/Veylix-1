package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// bytesReader keeps the import list honest without pulling in strings.
func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// D-077 — a Super Admin can correct a client's e-mail, and the change still goes
// through a code sent to the NEW address.
//
// The defect: the console could block, delete, reset the password and adjust the
// FREE limit, but the ADDRESS was untouchable — the page told the operator "their
// e-mail is changed by them, from Profile → Security". That is impossible for the
// customer whose mailbox is dead, which is precisely the customer an operator is
// trying to rescue and the one reset-password exists for. The account was a
// permanent dead end.
func TestAdminCanChangeAClientsEmailThroughTheCode(t *testing.T) {
	env := newEmailEnv(t)

	resp, body := env.do(t, http.MethodPost,
		env.url("/api/super/clients/"+strconv.FormatInt(env.clientID, 10)+"/email-change"),
		map[string]string{"new_email": "  New.Address@Example.COM "})
	if resp != http.StatusAccepted {
		t.Fatalf("request = %d %v, want 202", resp, body)
	}
	if got, _ := body["to"].(string); got != "new.address@example.com" {
		t.Errorf("to = %q, want the address lower-cased and trimmed", got)
	}
	// No relay in the harness, so the code must be returned — otherwise the
	// operator has no way to complete the change this route exists for.
	code, _ := body["code"].(string)
	if len(code) != 6 {
		t.Fatalf("code = %q, want a 6-digit code (delivery is outbox here)", code)
	}

	// The address must NOT have moved yet: a typo must not be able to take over
	// an account's identity without the code.
	if got := env.emailOf(t); got != "old@example.com" {
		t.Fatalf("email after request = %q, want it unchanged until confirmed", got)
	}

	confirm, cbody := env.do(t, http.MethodPost,
		env.url("/api/super/clients/"+strconv.FormatInt(env.clientID, 10)+"/email-change/confirm"),
		map[string]string{"code": code})
	if confirm != http.StatusOK {
		t.Fatalf("confirm = %d %v, want 200", confirm, cbody)
	}
	if got := env.emailOf(t); got != "new.address@example.com" {
		t.Errorf("email = %q, want new.address@example.com", got)
	}
	// Confirming verifies the address: it is the sign-in identifier now.
	var verified int
	if err := env.st.DB().QueryRow(
		`SELECT email_verified FROM users WHERE id = ?`, env.clientID).Scan(&verified); err != nil {
		t.Fatalf("read verified: %v", err)
	}
	if verified != 1 {
		t.Error("the new address was not marked verified")
	}
}

// A wrong code must not change anything, and must count against the attempt
// limit (a 6-digit code has only 10^6 values).
func TestAdminEmailChangeRefusesAWrongCode(t *testing.T) {
	env := newEmailEnv(t)
	path := "/api/super/clients/" + strconv.FormatInt(env.clientID, 10) + "/email-change"

	if resp, body := env.do(t, http.MethodPost, env.url(path),
		map[string]string{"new_email": "x@example.com"}); resp != http.StatusAccepted {
		t.Fatalf("request = %d %v", resp, body)
	}
	resp, _ := env.do(t, http.MethodPost, env.url(path+"/confirm"), map[string]string{"code": "000000"})
	if resp != http.StatusUnprocessableEntity {
		t.Fatalf("wrong code = %d, want 422", resp)
	}
	if got := env.emailOf(t); got != "old@example.com" {
		t.Errorf("email = %q, want it unchanged after a wrong code", got)
	}
}

// An address that belongs to somebody else is refused before anything is sent.
func TestAdminEmailChangeRefusesATakenAddress(t *testing.T) {
	env := newEmailEnv(t)
	mustD076User(t, env.st, "taken@example.com")

	resp, body := env.do(t, http.MethodPost,
		env.url("/api/super/clients/"+strconv.FormatInt(env.clientID, 10)+"/email-change"),
		map[string]string{"new_email": "taken@example.com"})
	// The row is unique, so the store reports the claim conflict; the handler
	// turns that into 409 email_taken.
	if resp != http.StatusConflict {
		t.Fatalf("taken address = %d %v, want 409", resp, body)
	}
	if got := env.emailOf(t); got != "old@example.com" {
		t.Errorf("email = %q, want it unchanged", got)
	}
}

// A reseller must never reach the console's e-mail route.
func TestAdminEmailChangeRequiresClientManage(t *testing.T) {
	env := newEmailEnv(t)
	staff := env.loginRole(t, "reseller@example.com", "admin")

	resp, _ := env.doAs(t, staff, http.MethodPost,
		env.url("/api/super/clients/"+strconv.FormatInt(env.clientID, 10)+"/email-change"),
		map[string]string{"new_email": "hijack@example.com"})
	if resp != http.StatusForbidden {
		t.Fatalf("reseller = %d, want 403 — this route is Super-Admin only", resp)
	}
}

// emailEnv is the console's e-mail routes over a real router and store.
type emailEnv struct {
	srv      *httptest.Server
	st       *store.Store
	clientID int64
	owner    *http.Client
}

func (e *emailEnv) url(p string) string { return e.srv.URL + p }

func (e *emailEnv) do(t *testing.T, method, url string, body any) (int, map[string]any) {
	t.Helper()
	return e.doAs(t, e.owner, method, url, body)
}

func (e *emailEnv) doAs(t *testing.T, c *http.Client, method, url string, body any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// emailOf reads the stored address of the client under test.
func (e *emailEnv) emailOf(t *testing.T) string {
	t.Helper()
	var email string
	if err := e.st.DB().QueryRow(`SELECT email FROM users WHERE id = ?`, e.clientID).Scan(&email); err != nil {
		t.Fatalf("read email: %v", err)
	}
	return email
}

// newEmailEnv seeds a client, bootstraps the Super Admin and signs it in.
func newEmailEnv(t *testing.T) *emailEnv {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "email.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Bootstrap FIRST: `/api/setup/status` counts any existing account, so a
	// client seeded before this makes the install look "already set up" and the
	// Super Admin can never be created.
	auditLog := audit.New(st.DB())
	authSvc := auth.NewService(st.DB(), auditLog)
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(st.DB(), false)
	srv := httptest.NewServer(Router(Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		Clients: st, AdminAudit: auditLog,
	}))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	owner := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	const pw = "a very long password"
	if resp, body := d075Do(t, owner, http.MethodPost, srv.URL+"/api/setup/bootstrap",
		map[string]string{"email": "owner@example.com", "password": pw, "display_name": "Owner"}); resp != http.StatusOK && resp != http.StatusCreated {
		t.Fatalf("bootstrap = %d %v", resp, body)
	}
	if resp, body := d075Do(t, owner, http.MethodPost, srv.URL+"/api/auth/login",
		map[string]string{"identifier": "owner@example.com", "password": pw, "portal": "staff"}); resp != http.StatusOK {
		t.Fatalf("staff login = %d %v", resp, body)
	}

	// Now the customer under test exists.
	clientID := mustD076User(t, st, "old@example.com")

	return &emailEnv{srv: srv, st: st, clientID: clientID, owner: owner}
}

// loginRole seeds an account of the given role and returns it signed in.
//
// Seeded directly: these tests are about the permission check, not about sign-up.
func (e *emailEnv) loginRole(t *testing.T, email, role string) *http.Client {
	t.Helper()
	hash, err := auth.HashPassword("a very long password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := e.st.DB().Exec(
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES (?, ?, 'U', ?, 'ACTIVE', 1, 1, 1)`, email, hash, role); err != nil {
		t.Fatalf("seed %s: %v", role, err)
	}
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	if resp, body := d075Do(t, c, http.MethodPost, e.srv.URL+"/api/auth/login",
		map[string]string{"identifier": email, "password": "a very long password", "portal": "staff"}); resp != http.StatusOK {
		t.Fatalf("login %s = %d %v", role, resp, body)
	}
	return c
}
