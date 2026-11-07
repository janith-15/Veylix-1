package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ClientIP decides the key a rate limit is counted against, so a caller must
// never be able to choose it freely.
func TestClientIP(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		header string
		want   string
	}{
		{"direct peer, no header", "203.0.113.9:5555", "", "203.0.113.9"},
		{"non-loopback peer ignores spoofed header", "203.0.113.9:5555", "10.9.9.9", "203.0.113.9"},
		{"loopback peer trusts first entry", "127.0.0.1:5555", "198.51.100.4, 10.0.0.1", "198.51.100.4"},
		{"loopback peer, single entry", "127.0.0.1:5555", "198.51.100.4", "198.51.100.4"},
		{"loopback peer, no header", "127.0.0.1:5555", "", "127.0.0.1"},
		{"loopback peer, garbage header falls back", "127.0.0.1:5555", "not-an-ip", "127.0.0.1"},
		{"IPv6 loopback peer", "[::1]:5555", "198.51.100.4", "198.51.100.4"},
		{"IPv6 loopback peer, no header", "[::1]:5555", "", "::1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
			r.RemoteAddr = tc.remote
			if tc.header != "" {
				r.Header.Set("X-Forwarded-For", tc.header)
			}
			if got := ClientIP(r); got != tc.want {
				t.Fatalf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// A non-loopback caller must not be able to claim https and steer the CSRF
// origin check.
func TestTrustForwardedProto(t *testing.T) {
	direct := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	direct.RemoteAddr = "203.0.113.9:5555"
	direct.Header.Set("X-Forwarded-Proto", "https")
	if TrustForwardedProto(direct) {
		t.Fatal("non-loopback peer must not be trusted for X-Forwarded-Proto")
	}

	proxied := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	proxied.RemoteAddr = "127.0.0.1:5555"
	proxied.Header.Set("X-Forwarded-Proto", "https")
	if !TrustForwardedProto(proxied) {
		t.Fatal("loopback proxy must be trusted for X-Forwarded-Proto")
	}
}

// The cookie must withhold itself from cross-site requests.
func TestSessionCookieIsStrict(t *testing.T) {
	store := &SessionStore{secure: true}
	rec := httptest.NewRecorder()
	store.SetCookie(rec, "token-value")

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("SameSite = %v, want Strict", c.SameSite)
	}
	if !c.HttpOnly {
		t.Fatal("cookie must be HttpOnly")
	}
	if !c.Secure {
		t.Fatal("cookie must be Secure when the store is secure")
	}
}
