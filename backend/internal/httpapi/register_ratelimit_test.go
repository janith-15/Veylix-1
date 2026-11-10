package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// Registration sends a verification e-mail, so an unthrottled endpoint lets one
// host create accounts in a loop and burn the relay's reputation. The refusal
// must also carry Retry-After: "too many" with no duration is not actionable.
func TestRegisterIsRateLimited(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir+"/reglimit.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	auditLog := audit.New(st.DB())
	svc := auth.NewService(st.DB(), auditLog)
	svc.SetAutoVerify(true) // keep this test about throttling, not verification
	sessions := auth.NewSessionStore(st.DB(), false)

	mux := http.NewServeMux()
	AuthRoutes(mux, svc, sessions, func(h http.Handler) http.Handler { return h },
		auth.NewRateLimiter(2, 0.001), // burst of 2, effectively no refill
		nil,                           // no mail relay
		nil,                           // no account reader: /api/me falls back to the session
		"Asia/Colombo")                // D-099 display zone; this test never reads /api/me
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	post := func(email string) *http.Response {
		body, _ := json.Marshal(map[string]string{
			"email": email, "password": "a-proper-long-secret-7", "display_name": "X",
		})
		res, err := http.Post(srv.URL+"/api/auth/register", "application/json", strings.NewReader(string(body)))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })
		return res
	}

	// The burst is allowed.
	for i := 0; i < 2; i++ {
		if got := post("a" + string(rune('0'+i)) + "@veylix.test").StatusCode; got == http.StatusTooManyRequests {
			t.Fatalf("attempt %d was throttled inside the burst (status %d)", i+1, got)
		}
	}
	// The next one must be refused, with a Retry-After.
	res := post("third@veylix.test")
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 after the burst", res.StatusCode)
	}
	if res.Header.Get("Retry-After") == "" {
		t.Fatal("a 429 must carry Retry-After, or the user cannot know how long to wait")
	}
}
