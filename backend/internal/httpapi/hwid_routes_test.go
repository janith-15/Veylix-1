package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/panel"
)

// fakeHwids is a stand-in for the integrations service. The routes under test
// only need to know that a call happened and whether it succeeded, so the fake
// records exactly that.
type fakeHwids struct {
	devices  []panel.Hwid
	err      error
	listed   int
	revoked  []int64
	revokeAt int64
}

func (f *fakeHwids) Devices(_ context.Context, _ int64, _ string) ([]panel.Hwid, error) {
	f.listed++
	if f.err != nil {
		return nil, f.err
	}
	return f.devices, nil
}

func (f *fakeHwids) RevokeDevice(_ context.Context, _ int64, _ string, id int64) error {
	f.revokeAt = id
	if f.err != nil {
		return f.err
	}
	f.revoked = append(f.revoked, id)
	return nil
}

// hwidHarness mounts only HwidRoutes on a fresh mux, signing the caller in as a
// super admin.
//
// The wrapper injects a principal rather than faking the middleware: the routes
// call RequirePermission(PermServerManage), which reads the principal from the
// context, so a pass-through alone correctly answers 401. That 401 is itself
// proof the routes are protected — TestHwidRoutesRequireAPrincipal asserts it.
func hwidHarness(t *testing.T, hw httpapi.HwidService) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	superAdmin := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := auth.Principal{UserID: 1, Email: "root@veylix.test", Role: auth.RoleSuperAdmin}
			h.ServeHTTP(w, r.WithContext(httpapi.WithPrincipal(r.Context(), p)))
		})
	}
	httpapi.HwidRoutes(mux, hw, nil, superAdmin)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestHwidRoutesAreRegistered proves D8's actual complaint: the routes exist.
//
// The defect was not a wrong answer — it was that the adapter could list and
// revoke devices while no HTTP path reached either, so the capability was
// unreachable from the product.
func TestHwidRoutesAreRegistered(t *testing.T) {
	hw := &fakeHwids{devices: []panel.Hwid{{ID: 7, Fingerprint: "ab12cd34"}}}
	srv := hwidHarness(t, hw)
	c := &http.Client{Timeout: 5 * time.Second}

	resp, body := getJSON(t, c, srv.URL+"/api/panel-clients/1/nimal@example.com_free_1/hwids")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET hwids = %d %v, want 200", resp.StatusCode, body)
	}
	if hw.listed != 1 {
		t.Errorf("the service was called %d time(s), want 1", hw.listed)
	}
	if got, _ := body["panel_email"].(string); got != "nimal@example.com_free_1" {
		t.Errorf("panel_email = %q, want the client the route addressed", got)
	}

	req, _ := http.NewRequest(http.MethodDelete,
		srv.URL+"/api/panel-clients/1/nimal@example.com_free_1/hwids/7", nil)
	dresp, err := c.Do(req)
	if err != nil {
		t.Fatalf("DELETE hwids: %v", err)
	}
	defer func() { _ = dresp.Body.Close() }()
	if dresp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE hwids = %d, want 204", dresp.StatusCode)
	}
	if hw.revokeAt != 7 {
		t.Errorf("revoked id = %d, want 7", hw.revokeAt)
	}
}

// TestHwidRoutesRejectBadIDs proves the guards run before the service is called,
// so a malformed request cannot reach the panel.
func TestHwidRoutesRejectBadIDs(t *testing.T) {
	hw := &fakeHwids{}
	srv := hwidHarness(t, hw)
	c := &http.Client{Timeout: 5 * time.Second}

	req, _ := http.NewRequest(http.MethodDelete,
		srv.URL+"/api/panel-clients/1/nimal@example.com_free_1/hwids/not-a-number", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad device id = %d, want 400", resp.StatusCode)
	}
	if hw.revokeAt != 0 {
		t.Error("a malformed request reached the service")
	}
}

// TestHwidRoutesNilServiceIs503 proves "not wired" cannot look like "no devices":
// an unwired deployment must say so, never answer an empty list.
func TestHwidRoutesNilServiceIs503(t *testing.T) {
	srv := hwidHarness(t, nil)
	c := &http.Client{Timeout: 5 * time.Second}

	resp, body := getJSON(t, c, srv.URL+"/api/panel-clients/1/x@y.test/hwids")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unwired GET = %d %v, want 503", resp.StatusCode, body)
	}
}

// TestHwidRoutesRequireAPrincipal proves an anonymous caller cannot list or
// free a device. The device list names the machines that hold a paying
// customer's cap, so it must not be readable without a session.
func TestHwidRoutesRequireAPrincipal(t *testing.T) {
	hw := &fakeHwids{devices: []panel.Hwid{{ID: 7}}}
	mux := http.NewServeMux()
	passThrough := func(h http.Handler) http.Handler { return h }
	httpapi.HwidRoutes(mux, hw, nil, passThrough)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &http.Client{Timeout: 5 * time.Second}
	resp, _ := getJSON(t, c, srv.URL+"/api/panel-clients/1/x@y.test/hwids")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous GET = %d, want 401", resp.StatusCode)
	}
	if hw.listed != 0 {
		t.Error("an unauthenticated request reached the service")
	}
}
