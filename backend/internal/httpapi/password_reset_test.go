package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// resetFixture boots a router with the password-reset routes mounted, plus one
// real account, and offers a helper that performs JSON calls.
type resetFixture struct {
	t     *testing.T
	srv   *httptest.Server
	store *store.Store
	user  *auth.User
}

func newResetFixture(t *testing.T) *resetFixture {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir+"/reset.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// A real audit logger: Register writes an entry, and a nil one would panic.
	auditLog := audit.New(st.DB())
	svc := auth.NewService(st.DB(), auditLog)
	// The reset flow does not depend on verification, but Register refuses an
	// unverified sign-in; tests create accounts directly, so keep them usable.
	svc.SetAutoVerify(true)
	// Register through the service so the row matches what production creates.
	user, err := svc.Register(context.Background(), auth.RegisterInput{
		Email: "reset@veylix.test", Password: "correct-horse-battery-9", DisplayName: "Reset Tester",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	mux := http.NewServeMux()
	PasswordResetRoutes(mux, svc, st, nil, nil, "support@veylix.test",
		auth.NewRateLimiter(1000, 1000))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &resetFixture{t: t, srv: srv, store: st, user: user}
}

func (f *resetFixture) post(path string, body any) (int, map[string]any) {
	f.t.Helper()
	raw, _ := json.Marshal(body)
	res, err := http.Post(f.srv.URL+path, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		f.t.Fatalf("post %s: %v", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// issueCode writes a known code through the store, standing in for the mail the
// server would send: with no relay configured the code is stored hashed, so a test
// cannot read it back and must supply its own.
func (f *resetFixture) issueCode() string {
	f.t.Helper()
	code, err := newNumericCode(6)
	if err != nil {
		f.t.Fatalf("code: %v", err)
	}
	hash, err := hashCode(code)
	if err != nil {
		f.t.Fatalf("hash: %v", err)
	}
	if err := f.store.CreatePasswordResetCode(context.Background(), f.user.ID, hash, resetCodeTTL); err != nil {
		f.t.Fatalf("store code: %v", err)
	}
	return code
}

// forgot must never reveal whether an account exists: same status, same body, and
// never the code itself.
func TestForgotDoesNotRevealAccounts(t *testing.T) {
	f := newResetFixture(t)
	knownStatus, knownBody := f.post("/api/auth/forgot", map[string]any{"identifier": "reset@veylix.test"})
	unknownStatus, unknownBody := f.post("/api/auth/forgot", map[string]any{"identifier": "nobody@veylix.test"})

	if knownStatus != http.StatusAccepted || unknownStatus != http.StatusAccepted {
		t.Fatalf("status: known=%d unknown=%d, want 202 for both", knownStatus, unknownStatus)
	}
	if knownBody["sent"] != unknownBody["sent"] || knownBody["message"] != unknownBody["message"] {
		t.Fatalf("bodies differ: %v vs %v", knownBody, unknownBody)
	}
	// A caller who can read the code from the response defeats the mailbox.
	for _, body := range []map[string]any{knownBody, unknownBody} {
		if _, leaked := body["code"]; leaked {
			t.Fatal("forgot response must never contain the code")
		}
	}
}

// A code is single use: the second redemption of the same value must fail.
func TestResetCodeIsSingleUse(t *testing.T) {
	f := newResetFixture(t)
	code := f.issueCode()

	status, body := f.post("/api/auth/reset", map[string]any{
		"identifier": "reset@veylix.test", "code": code, "new_password": "a-brand-new-secret-42",
	})
	if status != http.StatusOK || body["reset"] != true {
		t.Fatalf("first reset: %d %v", status, body)
	}

	status, body = f.post("/api/auth/reset", map[string]any{
		"identifier": "reset@veylix.test", "code": code, "new_password": "another-secret-777",
	})
	if status != http.StatusUnprocessableEntity ||
		body["error"].(map[string]any)["code"] != "invalid_code" {
		t.Fatalf("replay: %d %v, want 422 invalid_code", status, body)
	}
}

// Five wrong guesses burn the code, so a 6-digit value cannot be brute forced.
func TestResetCodeBurnsAfterMaxAttempts(t *testing.T) {
	f := newResetFixture(t)
	code := f.issueCode()

	for i := 0; i < resetCodeMaxAttempts; i++ {
		status, _ := f.post("/api/auth/reset", map[string]any{
			"identifier": "reset@veylix.test", "code": "000000", "new_password": "whatever-secret-11",
		})
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("guess %d: status %d, want 422", i+1, status)
		}
	}
	// The real code is dead now, even though it was never guessed.
	status, _ := f.post("/api/auth/reset", map[string]any{
		"identifier": "reset@veylix.test", "code": code, "new_password": "too-late-secret-99",
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("code must be burned after %d attempts, got %d", resetCodeMaxAttempts, status)
	}
}

// An expired code is refused even when the digits are right.
func TestResetCodeExpires(t *testing.T) {
	f := newResetFixture(t)
	code, err := newNumericCode(6)
	if err != nil {
		t.Fatalf("code: %v", err)
	}
	hash, _ := hashCode(code)
	// Negative TTL: already expired on arrival.
	if err := f.store.CreatePasswordResetCode(context.Background(), f.user.ID, hash, -time.Minute); err != nil {
		t.Fatalf("store: %v", err)
	}
	status, body := f.post("/api/auth/reset", map[string]any{
		"identifier": "reset@veylix.test", "code": code, "new_password": "expired-secret-123",
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status %d %v, want 422", status, body)
	}
}

// A successful reset must end every existing session: the old cookie cannot
// outlive the password it was issued under.
func TestResetRevokesEverySession(t *testing.T) {
	f := newResetFixture(t)
	ctx := context.Background()
	sessions := auth.NewSessionStore(f.store.DB(), false)
	token, err := sessions.Create(ctx, f.user.ID, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("session: %v", err)
	}

	code := f.issueCode()
	status, body := f.post("/api/auth/reset", map[string]any{
		"identifier": "reset@veylix.test", "code": code, "new_password": "after-reset-secret-8",
	})
	if status != http.StatusOK {
		t.Fatalf("reset: %d %v", status, body)
	}
	if _, err := sessions.Lookup(ctx, token); err == nil {
		t.Fatal("the pre-reset session must no longer authenticate")
	}
}

// A weak password is refused by the same policy the sign-up form uses, and the
// refusal must not consume the code — the owner can retry with a good one.
func TestResetEnforcesPasswordPolicy(t *testing.T) {
	f := newResetFixture(t)
	code := f.issueCode()
	status, body := f.post("/api/auth/reset", map[string]any{
		"identifier": "reset@veylix.test", "code": code, "new_password": "short",
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status %d %v, want 422 for a weak password", status, body)
	}
	status, body = f.post("/api/auth/reset", map[string]any{
		"identifier": "reset@veylix.test", "code": code, "new_password": "a-proper-long-secret-7",
	})
	if status != http.StatusOK {
		t.Fatalf("retry after a weak password: %d %v", status, body)
	}
}
