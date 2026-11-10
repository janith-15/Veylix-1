package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureLogs swaps the default slog logger for a buffer for one test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// TestPanicIsLoggedANDHidden is the defect this closes: `withRecover` used to turn
// a panic into a 500 and log NOTHING, so a production crash left no trace at all.
//
// Both halves matter and they pull in opposite directions: the log must carry
// enough to reproduce the fault (method, path, panic value), and the RESPONSE must
// carry none of it (no stack, no panic text) because the caller may be anonymous.
func TestPanicIsLoggedANDHidden(t *testing.T) {
	buf := captureLogs(t)

	h := withRecover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom-secret-detail")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/vpns/9/transfer", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	// The response must not leak the panic value.
	if strings.Contains(rec.Body.String(), "kaboom-secret-detail") {
		t.Error("response body leaked the panic value to the caller")
	}
	// The log must contain it, with the route.
	logged := buf.String()
	for _, want := range []string{"handler panic", "kaboom-secret-detail", "/api/vpns/9/transfer", "GET"} {
		if !strings.Contains(logged, want) {
			t.Errorf("log is missing %q; got:\n%s", want, logged)
		}
	}
}

// TestAbortHandlerIsNotLoggedAsAPanic: `http.ErrAbortHandler` is the documented
// way for a handler to abandon a response (client disconnect). Logging it as a
// crash would train an operator to ignore the one line that matters.
func TestAbortHandlerIsNotLoggedAsAPanic(t *testing.T) {
	buf := captureLogs(t)
	h := withRecover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/vpns", nil))
	if strings.Contains(buf.String(), "handler panic") {
		t.Errorf("ErrAbortHandler was logged as a panic: %s", buf.String())
	}
}

// TestAccessLogRecordsStatusAndHidesQuery proves the two rules of the access log:
// it records what happened (path, status), and it never records the query string —
// because reset and verification TOKENS travel there.
func TestAccessLogRecordsStatusAndHidesQuery(t *testing.T) {
	buf := captureLogs(t)

	h := LogRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/vpns":
			WriteError(w, http.StatusNotFound, "not_found", "nope")
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))

	// A 4xx carries a token in the query string that must not reach the log.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/vpns?token=SUPER-SECRET-TOKEN", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	logged := buf.String()
	if !strings.Contains(logged, "/api/vpns") {
		t.Errorf("log is missing the path: %s", logged)
	}
	if !strings.Contains(logged, "status=404") {
		t.Errorf("log is missing the status: %s", logged)
	}
	if strings.Contains(logged, "SUPER-SECRET-TOKEN") || strings.Contains(logged, "token=") {
		t.Errorf("log leaked the query string: %s", logged)
	}
}

// TestAccessLogSkipsStaticAssets: one page load is a dozen files, and logging each
// buries the request that failed.
func TestAccessLogSkipsStaticAssets(t *testing.T) {
	buf := captureLogs(t)
	h := LogRequests(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/index-a1b2.js", nil))
	if strings.Contains(buf.String(), "/assets/") {
		t.Errorf("static asset was logged: %s", buf.String())
	}
}

// TestStatusWriterStillStreams: wrapping a ResponseWriter must not break Flush,
// or every streaming response silently buffers until it ends.
func TestStatusWriterStillStreams(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}
	rc := http.NewResponseController(sw)
	if err := rc.Flush(); err != nil && err != http.ErrNotSupported {
		t.Fatalf("Flush through the wrapper failed: %v", err)
	}
}
