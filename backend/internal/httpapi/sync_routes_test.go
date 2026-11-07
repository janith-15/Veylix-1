package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/sync"
)

// stubSync records that it was asked, so the route's wiring is provable without
// touching a panel.
type stubSync struct {
	calls   int
	lastIDs []int64
}

func (s *stubSync) RunOnce(_ context.Context, ids []int64) sync.Result {
	s.calls++
	s.lastIDs = ids
	return sync.Result{Servers: []sync.ServerResult{{ServerID: 11, Clients: 4}}}
}

// superAdminAuthed injects a Super Admin principal, because the route is behind
// RequirePermission and a bare handler would answer 401 before reaching the body.
var superAdminAuthed = func(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := auth.Principal{UserID: 1, Role: auth.RoleSuperAdmin}
		h.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
	})
}

// POST /api/sync/run exists, and asks for the pass the caller described.
//
// Without this route the console could show that sync was stale and offer no way
// to freshen it: the operator could only wait, with no evidence that waiting was
// working.
func TestSyncRunTriggersAPass(t *testing.T) {
	mux := http.NewServeMux()
	stub := &stubSync{}
	// No auth wrapper here: the permission middleware is exercised separately
	// below, and this test is about the handler body.
	SyncRoutes(mux, stub, superAdminAuthed)

	req := httptest.NewRequest(http.MethodPost, "/api/sync/run", strings.NewReader(`{"server_ids":[11]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if stub.calls != 1 {
		t.Errorf("RunOnce called %d times, want 1", stub.calls)
	}
	if len(stub.lastIDs) != 1 || stub.lastIDs[0] != 11 {
		t.Errorf("server_ids = %v, want [11] — the request's scope was ignored", stub.lastIDs)
	}
	if !strings.Contains(rec.Body.String(), `"server_id":11`) {
		t.Errorf("body does not report the pass result: %s", rec.Body.String())
	}
}

// An empty body means "every active server", not "an error": that is exactly what
// a one-click button sends.
func TestSyncRunAcceptsAnEmptyBody(t *testing.T) {
	mux := http.NewServeMux()
	stub := &stubSync{}
	SyncRoutes(mux, stub, superAdminAuthed)

	req := httptest.NewRequest(http.MethodPost, "/api/sync/run", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if stub.calls != 1 || len(stub.lastIDs) != 0 {
		t.Errorf("calls=%d ids=%v, want one unscoped pass", stub.calls, stub.lastIDs)
	}
}

// A malformed body is refused rather than silently treated as "everything".
func TestSyncRunRejectsMalformedBody(t *testing.T) {
	mux := http.NewServeMux()
	stub := &stubSync{}
	SyncRoutes(mux, stub, superAdminAuthed)

	req := httptest.NewRequest(http.MethodPost, "/api/sync/run", strings.NewReader(`{"server_ids":`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if stub.calls != 0 {
		t.Error("a malformed request still ran a sync pass")
	}
}

// A build without the scheduler must not register a route that would 500.
func TestSyncRoutesAbsentWithoutARunner(t *testing.T) {
	mux := http.NewServeMux()
	SyncRoutes(mux, nil, func(h http.Handler) http.Handler { return h })
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/sync/run", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when no runner is wired", rec.Code)
	}
}

// A client must never be able to drive a fleet-wide panel read.
func TestSyncRunRefusesAClient(t *testing.T) {
	mux := http.NewServeMux()
	stub := &stubSync{}
	authed := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := auth.Principal{UserID: 9, Role: auth.RoleClient}
			h.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
	SyncRoutes(mux, stub, authed)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/sync/run", nil))

	if rec.Code == http.StatusOK {
		t.Error("a client triggered a sync pass")
	}
	if stub.calls != 0 {
		t.Error("the pass ran despite the caller not being allowed to start one")
	}
}
