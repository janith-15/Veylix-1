package httpapi

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// fakeSPA is a minimal Vite-shaped build: a shell, one hashed asset, one icon.
func fakeSPA() fstest.MapFS {
	return fstest.MapFS{
		"index.html":          {Data: []byte("<!doctype html><div id=root></div>")},
		"assets/index-abc.js": {Data: []byte("console.log('app')")},
		"favicon.svg":         {Data: []byte("<svg/>")},
	}
}

func serve(t *testing.T, assets fs.FS, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	spaHandler(assets).ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

// A deep link must return the shell so the client router can take over. This is
// the whole reason the fallback exists: /vpn/16 is a client route with no file.
func TestSPADeepLinkServesIndex(t *testing.T) {
	assets := fakeSPA()
	for _, route := range []string{"/", "/dashboard", "/vpn/16", "/chat/42", "/profile"} {
		rec := serve(t, assets, http.MethodGet, route)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got %d, want 200", route, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Fatalf("%s: content-type %q, want html", route, ct)
		}
		if !strings.Contains(rec.Body.String(), `id=root`) {
			t.Fatalf("%s: body is not the shell: %q", route, rec.Body.String())
		}
	}
}

// A real asset is served as itself and cached immutably, because Vite puts a
// content hash in the filename.
func TestSPAServesAssetsImmutable(t *testing.T) {
	rec := serve(t, fakeSPA(), http.MethodGet, "/assets/index-abc.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("asset: got %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("asset: cache-control %q, want immutable", got)
	}
	if !strings.Contains(rec.Body.String(), "console.log") {
		t.Fatalf("asset: body %q", rec.Body.String())
	}
}

// A MISSING file must be a 404, never the shell: answering HTML for a missing
// script is what produces "Unexpected token '<'" in the console instead of an
// honest 404 that names the real problem.
func TestSPAMissingFileIs404(t *testing.T) {
	for _, target := range []string{"/assets/index-gone.js", "/favicon.png", "/robots.txt"} {
		rec := serve(t, fakeSPA(), http.MethodGet, target)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: got %d, want 404", target, rec.Code)
		}
		if strings.Contains(rec.Body.String(), `id=root`) {
			t.Fatalf("%s: served the shell for a missing file", target)
		}
	}
}

// HEAD must not carry a body but must answer like GET (uptime checks, caches).
func TestSPAHeadHasNoBody(t *testing.T) {
	rec := serve(t, fakeSPA(), http.MethodHead, "/dashboard")
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD: got %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD: body length %d, want 0", rec.Body.Len())
	}
}

// A non-GET reaching the SPA handler is a routing mistake and must not be
// answered with HTML.
func TestSPARejectsNonGet(t *testing.T) {
	rec := serve(t, fakeSPA(), http.MethodPost, "/dashboard")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: got %d, want 405", rec.Code)
	}
}

// statusOf is the presence rule: index.html decides, not the file count.
func TestSPAStatusRequiresIndex(t *testing.T) {
	if present, files := spaStatus(fakeSPA()); !present || files != 3 {
		t.Fatalf("full build: present=%v files=%d, want true/3", present, files)
	}
	noIndex := fstest.MapFS{"assets/index-abc.js": {Data: []byte("x")}}
	if present, _ := spaStatus(noIndex); present {
		t.Fatal("a build without index.html reported present")
	}
	if present, files := spaStatus(fstest.MapFS{".gitkeep": {Data: []byte("")}}); present || files != 1 {
		t.Fatalf("placeholder: present=%v files=%d, want false/1", present, files)
	}
}

// The router must never let the SPA fallback swallow an API path: a client
// asking for an unknown endpoint needs the JSON envelope, not an HTML page it
// cannot parse.
func TestRouterKeepsApi404AsJSON(t *testing.T) {
	handler := Router(Deps{Health: fakeHealth{}, Version: "test"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Fatalf("content-type %q, want json (the SPA must not answer API routes)", ct)
	}
}

// Ready must say whether this binary carries a UI, so "the app is blank" can be
// diagnosed from one request.
func TestReadyReportsUIStatus(t *testing.T) {
	handler := Router(Deps{Health: fakeHealth{schema: 7}, Version: "test"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"ui"`) {
		t.Fatalf("ready body lacks a ui field: %s", body)
	}
	// The value depends on whether a build is embedded, which varies per
	// checkout; the contract is only that it is one of the two honest states.
	if !strings.Contains(body, "not_embedded") && !strings.Contains(body, "embedded:") {
		t.Fatalf("ui field is neither state: %s", body)
	}
}
