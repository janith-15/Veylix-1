package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/settings"
)

// stubSettings records the last Save so the route contract can be proven without
// a database or a sealer.
type stubSettings struct {
	view       settings.GoogleView
	viewErr    error
	saved      settings.GoogleInput
	savedActor int64
	saveErr    error
}

func (s *stubSettings) View(context.Context) (settings.GoogleView, error) {
	return s.view, s.viewErr
}

func (s *stubSettings) Save(_ context.Context, in settings.GoogleInput, actorID int64) (settings.GoogleView, error) {
	s.saved = in
	s.savedActor = actorID
	if s.saveErr != nil {
		return settings.GoogleView{}, s.saveErr
	}
	return s.view, nil
}

// settingsMux mounts only the settings routes with a principal of `role`
// already in the request context, so the ROUTE contract (status codes, masking,
// actor plumbing) is what is under test.
func settingsMux(svc SettingsService, role auth.Role) http.Handler {
	mux := http.NewServeMux()
	inject := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithPrincipal(r.Context(), auth.Principal{UserID: 7, Email: "super@test", Role: role})
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	SettingsRoutes(mux, svc, inject)
	return mux
}

func doSettings(mux http.Handler, method, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/super/settings/google", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestSettingsGetReturnsTheMaskedView(t *testing.T) {
	stub := &stubSettings{view: settings.GoogleView{
		ClientID: "abc.apps.googleusercontent.com", HasSecret: true,
		RedirectURL: "https://vpn.example.com/api/auth/google/callback", Configured: true, Source: "database",
	}}
	rec := doSettings(settingsMux(stub, auth.RoleSuperAdmin), http.MethodGet, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d, want 200", rec.Code)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["has_secret"] != true {
		t.Fatalf("has_secret = %v, want true", out["has_secret"])
	}
	// The secret must never be a field on the wire; only its presence is reported.
	if _, ok := out["client_secret"]; ok {
		t.Fatal("the response leaked a client_secret field")
	}
	// The derived URL is present so the page can hand over the exact string to
	// register — and it must end at the one callback route.
	got, _ := out["expected_redirect_url"].(string)
	if !strings.HasSuffix(got, "/api/auth/google/callback") {
		t.Fatalf("expected_redirect_url = %q, want it to end with the callback path", got)
	}
}

// TestSettingsGetDerivesTheExpectedRedirectFromTheRequestHost proves the URL is
// built from where the operator actually is — not from the stored value, which
// can be mistyped. A proxied HTTPS request must produce the https URL.
func TestSettingsGetDerivesTheExpectedRedirectFromTheRequestHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/super/settings/google", nil)
	req.Host = "vpn.acme.com"
	req.RemoteAddr = "127.0.0.1:5555" // a trusted (loopback) peer
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	settingsMux(&stubSettings{}, auth.RoleSuperAdmin).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d, want 200", rec.Code)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := out["expected_redirect_url"]; got != "https://vpn.acme.com/api/auth/google/callback" {
		t.Fatalf("expected_redirect_url = %v, want https://vpn.acme.com/api/auth/google/callback", got)
	}
}

func TestSettingsPutPassesTheActorAndSecret(t *testing.T) {
	stub := &stubSettings{view: settings.GoogleView{HasSecret: true, Configured: true}}
	body := `{"client_id":"abc.apps.googleusercontent.com","client_secret":"GOCSPX-secret","redirect_url":"https://vpn.example.com/api/auth/google/callback"}`
	rec := doSettings(settingsMux(stub, auth.RoleSuperAdmin), http.MethodPut, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if stub.savedActor != 7 {
		t.Fatalf("actor = %d, want 7 (the principal's id)", stub.savedActor)
	}
	if stub.saved.ClientSecret != "GOCSPX-secret" {
		t.Fatalf("the route did not pass the secret through: %q", stub.saved.ClientSecret)
	}
}

func TestSettingsPutRejectsInvalidJSON(t *testing.T) {
	rec := doSettings(settingsMux(&stubSettings{}, auth.RoleSuperAdmin), http.MethodPut, "{not json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT invalid body = %d, want 400", rec.Code)
	}
}

func TestSettingsPutMapsValidationTo400(t *testing.T) {
	stub := &stubSettings{saveErr: settings.NewValidationError("client_secret must start with GOCSPX-")}
	rec := doSettings(settingsMux(stub, auth.RoleSuperAdmin), http.MethodPut, `{"client_secret":"nope"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PUT validation error = %d, want 400", rec.Code)
	}
}

func TestSettingsAreRefusedForNonSuperAdmins(t *testing.T) {
	rec := doSettings(settingsMux(&stubSettings{}, auth.RoleAdmin), http.MethodGet, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET as admin = %d, want 403", rec.Code)
	}
}

func TestSettingsAbsentWhenServiceIsNil(t *testing.T) {
	// A deployment without the service must 404, never pretend the page works.
	rec := doSettings(settingsMux(nil, auth.RoleSuperAdmin), http.MethodGet, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET with no service = %d, want 404", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/super/settings/public-url — the address this deployment serves (D-125)
// ---------------------------------------------------------------------------

func doPublicURL(mux http.Handler, host string, secure bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/super/settings/public-url", nil)
	req.Host = host
	if secure {
		req.Header.Set("X-Forwarded-Proto", "https")
		// The forwarded-proto honouring only trusts a loopback peer.
		req.RemoteAddr = "127.0.0.1:4444"
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestPublicURLReportsTheServedOrigin(t *testing.T) {
	stub := &stubSettings{}
	mux := settingsMux(stub, auth.RoleSuperAdmin)

	rec := doPublicURL(mux, "vpn.example.com", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var out publicURLBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// The URL must be the origin this request arrived on, not a hardcoded one -
	// that is what makes it impossible for the page to show a stale domain.
	if out.URL != "https://vpn.example.com" {
		t.Fatalf("url = %q, want https://vpn.example.com", out.URL)
	}
	if out.Host != "vpn.example.com" {
		t.Fatalf("host = %q", out.Host)
	}
	if out.LoginURL != "https://vpn.example.com/login" {
		t.Fatalf("login_url = %q", out.LoginURL)
	}
}

// A non-default HTTPS port must survive into both URLs, or the operator copies a
// link that does not connect (D-123).
func TestPublicURLKeepsANonDefaultPort(t *testing.T) {
	mux := settingsMux(&stubSettings{}, auth.RoleSuperAdmin)

	rec := doPublicURL(mux, "vpn.example.com:8443", true)
	var out publicURLBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.URL != "https://vpn.example.com:8443" {
		t.Fatalf("url = %q, want the port kept", out.URL)
	}
	if out.LoginURL != "https://vpn.example.com:8443/login" {
		t.Fatalf("login_url = %q", out.LoginURL)
	}
}

// Loopback access (no forwarded proto, no TLS) must report http, not a flattering
// https that is not actually served there.
func TestPublicURLIsHonestOnPlainHTTP(t *testing.T) {
	mux := settingsMux(&stubSettings{}, auth.RoleSuperAdmin)

	rec := doPublicURL(mux, "127.0.0.1:8080", false)
	var out publicURLBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.URL != "http://127.0.0.1:8080" {
		t.Fatalf("url = %q, want http on a plain loopback request", out.URL)
	}
}

// The endpoint is Super-Admin-only: it names the operator's deployment and must
// not be readable by a reseller or a client.
func TestPublicURLRequiresSuperAdmin(t *testing.T) {
	for _, role := range []auth.Role{auth.RoleAdmin, auth.RoleClient} {
		mux := settingsMux(&stubSettings{}, role)
		rec := doPublicURL(mux, "vpn.example.com", true)
		if rec.Code == http.StatusOK {
			t.Fatalf("role %v read the public URL (got 200)", role)
		}
	}
}
