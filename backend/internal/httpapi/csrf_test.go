package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// csrfProbe builds the same middleware chain the router uses.
func csrfProbe(t *testing.T, publicOrigins ...string) http.Handler {
	t.Helper()
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return SameOrigin(publicOrigins...)(ok)
}

func post(t *testing.T, h http.Handler, headers map[string]string) int {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "http://panel.test/api/auth/login", strings.NewReader(`{}`))
	r.Host = "panel.test"
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Code
}

func TestSameOriginCSRF(t *testing.T) {
	h := csrfProbe(t)

	t.Run("same-origin allowed", func(t *testing.T) {
		if got := post(t, h, map[string]string{"Origin": "http://panel.test"}); got != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", got)
		}
	})

	t.Run("cross-origin blocked", func(t *testing.T) {
		if got := post(t, h, map[string]string{"Origin": "http://evil.test"}); got != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", got)
		}
	})

	t.Run("browser fetch with no Origin blocked", func(t *testing.T) {
		// This is the case Lax cookies let through: a cross-site form/navigation
		// that carries no Origin but is unmistakably a browser request.
		if got := post(t, h, map[string]string{"Sec-Fetch-Site": "cross-site"}); got != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", got)
		}
	})

	t.Run("browser navigation with no Origin blocked", func(t *testing.T) {
		if got := post(t, h, map[string]string{"Referer": "http://evil.test/x"}); got != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", got)
		}
	})

	t.Run("scriptless API client allowed", func(t *testing.T) {
		// No Origin and no fetch metadata: curl, the test harnesses, the CLI.
		if got := post(t, h, nil); got != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", got)
		}
	})

	t.Run("GET is never blocked", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "http://panel.test/api/me", nil)
		r.Host = "panel.test"
		r.Header.Set("Origin", "http://evil.test")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rec.Code)
		}
	})

	t.Run("configured public origin allowed behind a proxy", func(t *testing.T) {
		proxied := csrfProbe(t, "https://panel.example.com/")
		got := post(t, proxied, map[string]string{"Origin": "https://panel.example.com"})
		if got != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", got)
		}
	})
}

// isForwardedHTTPS must not believe a client that is not our proxy.
func TestForwardedProtoOnlyFromLoopback(t *testing.T) {
	direct := httptest.NewRequest(http.MethodPost, "http://panel.test/x", nil)
	direct.Host = "panel.test"
	direct.RemoteAddr = "203.0.113.9:5555"
	direct.Header.Set("X-Forwarded-Proto", "https")
	if got := schemeHost(direct); got != "http://panel.test" {
		t.Fatalf("schemeHost = %q, want http://panel.test", got)
	}

	proxied := httptest.NewRequest(http.MethodPost, "http://panel.test/x", nil)
	proxied.Host = "panel.test"
	proxied.RemoteAddr = "127.0.0.1:5555"
	proxied.Header.Set("X-Forwarded-Proto", "https")
	if got := schemeHost(proxied); got != "https://panel.test" {
		t.Fatalf("schemeHost = %q, want https://panel.test", got)
	}
}
