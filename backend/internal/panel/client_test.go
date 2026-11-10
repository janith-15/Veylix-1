package panel

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakePanel mirrors the observed 3X-UI 3.10.2 envelopes.
const statusBody = `{"success":true,"msg":"","obj":{"cpu":0.91,"cpuCores":1,` +
	`"mem":{"current":403955712,"total":998576128},"xray":{"state":"running","version":"26.10.10"},` +
	`"panelVersion":"3.10.2","panelGuid":"g","uptime":1,"tcpCount":82}}`

const optionsBody = `{"success":true,"msg":"","obj":[{"id":1,"remark":"(IN)-free_2",` +
	`"tag":"in-443-tcp","protocol":"vless","port":443,"enable":true,"network":"tcp","security":"tls"}]}`

func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	/*
	 * `AllowPrivate: true` is required, not a workaround. `httptest.NewServer`
	 * listens on 127.0.0.1, and the dial-time guard (B-4) refuses loopback —
	 * which is the whole point of it. The target carries the per-integration
	 * policy so a test that stands in for a panel on a private network declares
	 * that policy explicitly, exactly as a real self-hosted deployment does.
	 */
	c, err := New(Options{
		Target:  &Target{BaseURL: srv.URL, AllowPrivate: true},
		Token:   "secret-token-value",
		Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, srv
}

func TestHealthParsesRealPanelPayload(t *testing.T) {
	var gotAuth string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/panel/api/server/status" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(statusBody))
	})

	res := c.Health(context.Background())
	if res.Status != StatusActive {
		t.Fatalf("status = %s, want ACTIVE (err=%v)", res.Status, res.Err)
	}
	if res.PanelVersion != "3.10.2" || res.XrayVersion != "26.10.10" || res.XrayState != "running" {
		t.Errorf("parsed versions wrong: %+v", res)
	}
	if res.MemTotal != 998576128 {
		t.Errorf("memTotal = %d", res.MemTotal)
	}
	if gotAuth != "Bearer secret-token-value" {
		t.Errorf("auth header not sent as bearer: %q", gotAuth)
	}
}

// T-03: a 401/403 must map to REAUTH_REQUIRED, never to ACTIVE.
func TestHealthMapsAuthFailuresToReauthRequired(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		statusCode := code
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(statusCode)
		})
		if res := c.Health(context.Background()); res.Status != StatusReauthRequired {
			t.Errorf("HTTP %d -> %s, want REAUTH_REQUIRED", statusCode, res.Status)
		}
	}
}

// The observed panel returns 404 with an empty body for a wrong base path.
// That must NOT be reported as "invalid token".
func TestWrongBasePathIsUnavailableNotReauth(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	res := c.Health(context.Background())
	if res.Status != StatusConnectionUnavailable {
		t.Fatalf("status = %s, want CONNECTION_UNAVAILABLE", res.Status)
	}
	if res.Err == nil {
		t.Error("expected a reported error")
	}
}

// T-04: an outage must never be reported as a policy/revocation state.
func TestUnreachablePanelIsConnectionUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // now unreachable

	c, err := New(Options{Target: &Target{BaseURL: url}, Token: "t", Timeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if res := c.Health(context.Background()); res.Status != StatusConnectionUnavailable {
		t.Fatalf("status = %s, want CONNECTION_UNAVAILABLE", res.Status)
	}
}

func TestInboundsUsesOptionsEndpoint(t *testing.T) {
	var path string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(optionsBody))
	})

	inbounds, err := c.Inbounds(context.Background())
	if err != nil {
		t.Fatalf("Inbounds: %v", err)
	}
	if path != "/panel/api/inbounds/options" {
		t.Errorf("path = %q, want the lightweight options endpoint", path)
	}
	if len(inbounds) != 1 || inbounds[0].Protocol != "vless" || inbounds[0].Port != 443 {
		t.Errorf("unexpected inbounds: %+v", inbounds)
	}
}

// A GET is retried on transport failure (idempotent)…
func TestGetIsRetriedOnTransportFailure(t *testing.T) {
	var calls int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Skip("hijacking unsupported")
			}
			conn, _, _ := hj.Hijack()
			_ = conn.Close() // simulate a dropped connection
			return
		}
		_, _ = w.Write([]byte(statusBody))
	})

	res := c.Health(context.Background())
	if res.Status != StatusActive {
		t.Fatalf("expected the retry to recover, got %s (%v)", res.Status, res.Err)
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Error("expected a retry attempt")
	}
}

// …but a POST (a real write) is never retried blindly.
func TestPostIsNeverRetried(t *testing.T) {
	var calls int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Skip("hijacking unsupported")
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	})

	err := c.AddClient(context.Background(), AddClientRequest{
		Client:     ClientSpec{Email: "vyx-1", SubID: "vyx-1", Enable: true},
		InboundIDs: []int{1},
	})
	if err == nil {
		t.Fatal("expected a failure")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("write was attempted %d times, want exactly 1 (no blind retry)", got)
	}
}

func TestAddClientSendsDocumentedBody(t *testing.T) {
	var got AddClientRequest
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/panel/api/clients/add" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"success":true,"msg":"","obj":null}`))
	})

	err := c.AddClient(context.Background(), AddClientRequest{
		Client:     ClientSpec{Email: "vyx-7", SubID: "vyx-7", Enable: true, TotalGB: 1024, ExpiryTime: 1893456000000, LimitIP: 2},
		InboundIDs: []int{1},
	})
	if err != nil {
		t.Fatalf("AddClient: %v", err)
	}
	if got.Client.Email != "vyx-7" || got.Client.SubID != "vyx-7" {
		t.Errorf("idempotency pair not sent: %+v", got.Client)
	}
	if len(got.InboundIDs) != 1 || got.InboundIDs[0] != 1 {
		t.Errorf("inboundIds = %v", got.InboundIDs)
	}
}

func TestAddClientSendsLimitHwid(t *testing.T) {
	// Regression for F-024 / D-028.5: the adapter used to omit limitHwid, so
	// the panel never enforced a hardware/device limit even though Veylix
	// believed it had set one. Assert on the raw body so a future struct
	// change cannot silently drop the field again.
	var body map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/panel/api/clients/add" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"success":true,"msg":"","obj":null}`))
	})

	err := c.AddClient(context.Background(), AddClientRequest{
		Client:     ClientSpec{Email: "vyx-1", SubID: "vyx-1", Enable: true, LimitIP: 2, LimitHwid: 2},
		InboundIDs: []int{1},
	})
	if err != nil {
		t.Fatalf("AddClient: %v", err)
	}

	client, ok := body["client"].(map[string]any)
	if !ok {
		t.Fatalf("no client object in body: %v", body)
	}
	if got := client["limitHwid"]; got != float64(2) {
		t.Errorf("client.limitHwid = %v, want 2 (hardware limit must be sent)", got)
	}
	if got := client["limitIp"]; got != float64(2) {
		t.Errorf("client.limitIp = %v, want 2", got)
	}
}

func TestGetClientDecodesFullRecord(t *testing.T) {
	// Uses the REAL panel shape: {"obj":{"client":{...},"inboundIds":[...]}}.
	// The first draft of GetClient decoded obj directly and silently returned a
	// zero-valued record, which would have reported "no limits" for every VPN.
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/panel/api/clients/get/vyx-1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"success":true,"msg":"","obj":{` +
			`"client":{"id":9,"email":"vyx-1","subId":"vyx-1",` +
			`"uuid":"9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d","enable":true,` +
			`"totalGB":10737418240,"expiryTime":1893456000000,` +
			`"limitIp":2,"limitHwid":2,"comment":"veylix:vpn:1","security":"auto"},` +
			`"inboundIds":[1],"externalLinks":[],"usedTraffic":0}}`))
	})

	got, err := c.GetClient(context.Background(), "vyx-1")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if got == nil {
		t.Fatal("GetClient returned nil for a present client")
	}
	if got.LimitHwid != 2 {
		t.Errorf("LimitHwid = %d, want 2 (nested obj.client not decoded)", got.LimitHwid)
	}
	if got.LimitIP != 2 {
		t.Errorf("LimitIP = %d, want 2", got.LimitIP)
	}
	if got.UUID == "" {
		t.Error("uuid not decoded")
	}
	if got.TotalGB != 10737418240 {
		t.Errorf("TotalGB = %d", got.TotalGB)
	}
	if got.Comment != "veylix:vpn:1" {
		t.Errorf("Comment = %q", got.Comment)
	}
	if len(got.InboundIDs) != 1 || got.InboundIDs[0] != 1 {
		t.Errorf("InboundIDs = %v, want [1]", got.InboundIDs)
	}
}

func TestGetClientAcceptsUnnestedRecord(t *testing.T) {
	// A version that returns the record directly must still decode.
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"obj":{` +
			`"email":"vyx-2","enable":true,"limitIp":2,"limitHwid":2}}`))
	})
	got, err := c.GetClient(context.Background(), "vyx-2")
	if err != nil || got == nil {
		t.Fatalf("GetClient: %v / %v", got, err)
	}
	if got.LimitHwid != 2 || got.Email != "vyx-2" {
		t.Errorf("decoded = %+v", got)
	}
}

func TestGetClientReturnsNilWhenAbsent(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"msg":"not found"}`))
	})
	got, err := c.GetClient(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("absent client must not be an error, got %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

func TestUpdateClientSendsCompleteRecord(t *testing.T) {
	// clients/update REPLACES the row: anything not sent is lost. Assert that a
	// round-trip through SpecFromPanelClient carries every editable field.
	var got map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/panel/api/clients/update/vyx-1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"success":true,"msg":"Client updated","obj":null}`))
	})

	rec := &PanelClient{
		Email: "vyx-1", SubID: "vyx-1", UUID: "uuid-abc", Enable: true,
		TotalGB: 10737418240, ExpiryTime: 1893456000000, LimitIP: 2, LimitHwid: 2,
		Comment: "veylix:vpn:1", Security: "auto",
	}
	spec := SpecFromPanelClient(rec)
	spec.LimitHwid = 5 // the one intended change

	if err := c.UpdateClient(context.Background(), "vyx-1", spec); err != nil {
		t.Fatalf("UpdateClient: %v", err)
	}

	for field, want := range map[string]any{
		"email": "vyx-1", "subId": "vyx-1", "id": "uuid-abc",
		"comment": "veylix:vpn:1", "security": "auto",
	} {
		if got[field] != want {
			t.Errorf("%s = %v, want %v (replace, not patch)", field, got[field], want)
		}
	}
	if got["limitHwid"] != float64(5) {
		t.Errorf("limitHwid = %v, want the edited value 5", got["limitHwid"])
	}
	if got["totalGB"] != float64(10737418240) {
		t.Errorf("totalGB = %v, lost during the round-trip", got["totalGB"])
	}
	if got["enable"] != true {
		t.Errorf("enable = %v, lost during the round-trip", got["enable"])
	}
}

func TestUpdateClientCannotInjectPath(t *testing.T) {
	called := false
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"success":true,"obj":null}`))
	})
	if err := c.UpdateClient(context.Background(), "../admin", ClientSpec{}); err == nil {
		t.Fatal("expected an error for a traversal email")
	}
	if called {
		t.Fatal("a traversal email must not reach the panel")
	}
}

func TestLinksUsesDocumentedEndpoint(t *testing.T) {
	var gotPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"success":true,"msg":"","obj":["vless://uuid@host:443?type=tcp#user1"]}`))
	})

	links, err := c.Links(context.Background(), "vyx-1")
	if err != nil {
		t.Fatalf("Links: %v", err)
	}
	if gotPath != "/panel/api/clients/links/vyx-1" {
		t.Errorf("path = %s", gotPath)
	}
	if len(links) != 1 || links[0] != "vless://uuid@host:443?type=tcp#user1" {
		t.Errorf("links = %v", links)
	}
}

func TestLinksEmptyWhenAbsent(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	links, err := c.Links(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("absent client must not be an error: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("links = %v, want none", links)
	}
}

func TestLinksCannotInjectPath(t *testing.T) {
	called := false
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"success":true,"obj":[]}`))
	})
	if _, err := c.Links(context.Background(), "../admin"); err == nil {
		t.Fatal("expected an error for a traversal email")
	}
	if called {
		t.Fatal("a traversal email must not reach the panel")
	}
}

func TestHwidsDecodesDeviceList(t *testing.T) {
	var gotMethod, gotPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_, _ = w.Write([]byte(`{"success":true,"msg":"","obj":[` +
			`{"id":1,"fingerprint":"6ad17c93e821","userAgent":"Happ/1.0",` +
			`"deviceOs":"android","osVersion":"15","deviceModel":"Pixel 9"}]}`))
	})

	devices, err := c.Hwids(context.Background(), "vyx-1")
	if err != nil {
		t.Fatalf("Hwids: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/panel/api/clients/hwids/vyx-1" {
		t.Errorf("unexpected %s %s", gotMethod, gotPath)
	}
	if len(devices) != 1 {
		t.Fatalf("devices = %v", devices)
	}
	if devices[0].Fingerprint != "6ad17c93e821" || devices[0].DeviceOS != "android" {
		t.Errorf("decoded device = %+v", devices[0])
	}
}

func TestDeleteHwidUsesDeleteVerb(t *testing.T) {
	var gotMethod, gotPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_, _ = w.Write([]byte(`{"success":true,"msg":""}`))
	})

	if err := c.DeleteHwid(context.Background(), "vyx-1", 7); err != nil {
		t.Fatalf("DeleteHwid: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/panel/api/clients/hwids/vyx-1/7" {
		t.Errorf("unexpected %s %s", gotMethod, gotPath)
	}
}

func TestDeleteHwidTreats404AsSuccess(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if err := c.DeleteHwid(context.Background(), "vyx-1", 7); err != nil {
		t.Fatalf("an already-absent device is the desired state, got %v", err)
	}
}

// The real 3X-UI panel answers an unknown client with HTTP 200 and
// {"success":false,"msg":"Obtain (record not found)"} — NOT a 404. Before this
// was handled, ClientExists returned an error for a deleted client, so verified
// absence (D-024.5) could never succeed and stale "ACTIVE" VPNs could never be
// reconciled.
func TestClientExistsHandlesPanelRecordNotFoundEnvelope(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/panel/api/clients/get/gone":
			_, _ = w.Write([]byte(`{"success":false,"msg":"Obtain (record not found)","obj":null}`))
		case "/panel/api/clients/get/broken":
			_, _ = w.Write([]byte(`{"success":false,"msg":"database is locked","obj":null}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"obj":{"client":{"email":"x"}}}`))
		}
	})

	ok, err := c.ClientExists(context.Background(), "gone")
	if err != nil {
		t.Fatalf("a 'record not found' envelope must not be an error, got %v", err)
	}
	if ok {
		t.Error("ClientExists = true for a client the panel says is gone")
	}

	if _, err := c.ClientExists(context.Background(), "broken"); err == nil {
		t.Error("a genuine failure must still be an error, never 'absent'")
	}
}

func TestClientExistsDistinguishesStates(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/panel/api/clients/get/present":
			_, _ = w.Write([]byte(`{"success":true,"obj":{"email":"present"}}`))
		case "/panel/api/clients/get/absent":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	if ok, err := c.ClientExists(context.Background(), "present"); err != nil || !ok {
		t.Errorf("present: ok=%v err=%v", ok, err)
	}
	if ok, err := c.ClientExists(context.Background(), "absent"); err != nil || ok {
		t.Errorf("absent: ok=%v err=%v", ok, err)
	}
	if _, err := c.ClientExists(context.Background(), "broken"); err == nil {
		t.Error("a 500 must surface as an error, never as 'absent'")
	}
}

// Path traversal in a client email must never escape the endpoint.
func TestClientEmailCannotInjectPath(t *testing.T) {
	var path string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"success":true,"obj":null}`))
	})
	_, _ = c.ClientExists(context.Background(), "../../panel/api/settings")
	if strings.Contains(path, "settings") {
		t.Fatalf("path traversal escaped: %q", path)
	}
}

// The token must never appear in an error returned to a caller.
func TestErrorsNeverContainToken(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if _, err := c.ListClients(context.Background(), 1, 10); err == nil {
		t.Fatal("expected an error")
	} else if strings.Contains(err.Error(), "secret-token-value") {
		t.Fatalf("token leaked into error: %v", err)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	var redirected int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&redirected, 1)
		_, _ = w.Write([]byte(statusBody))
	}))
	defer target.Close()

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/panel/api/server/status", http.StatusFound)
	})

	if res := c.Health(context.Background()); res.Status == StatusActive {
		t.Fatal("a redirect must not be followed to another host")
	}
	if atomic.LoadInt32(&redirected) != 0 {
		t.Fatal("redirect target was contacted")
	}
}

// Regression for the path-traversal defect found by TestClientEmailCannotInjectPath:
// url.PathEscape leaves ".." intact because dots are legal in a path segment.
func TestValidateEmailRejectsTraversalAndUnsafeChars(t *testing.T) {
	bad := []string{
		"",
		"../../panel/api/settings",
		"a..b",
		"a/b",
		"a\\b",
		"a b",
		"a%2e%2e",
		"a;b",
		"a?x=1",
		"a#frag",
		strings.Repeat("a", 191),
	}
	for _, email := range bad {
		if err := ValidateEmail(email); err == nil {
			t.Errorf("email %q must be rejected", email)
		}
	}

	good := []string{"vyx-1", "vyx-42", "client@example.com", "a_b.c+d", "ABC123"}
	for _, email := range good {
		if err := ValidateEmail(email); err != nil {
			t.Errorf("email %q should be accepted: %v", email, err)
		}
	}
}

// A rejected email must never reach the network.
func TestInvalidEmailPerformsNoRequest(t *testing.T) {
	var calls int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	})

	if _, err := c.ClientExists(context.Background(), "../../etc/passwd"); err == nil {
		t.Fatal("expected a validation error")
	}
	if err := c.DeleteClient(context.Background(), "../x"); err == nil {
		t.Fatal("expected a validation error")
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("validation failure still issued %d request(s)", n)
	}
}

func TestNewRejectsMissingInputs(t *testing.T) {
	if _, err := New(Options{Target: &Target{BaseURL: "https://x"}}); err == nil {
		t.Fatal("empty token must be rejected")
	}
	if _, err := New(Options{Token: "t"}); err == nil {
		t.Fatal("missing target must be rejected")
	}
}

// A retried GET must re-send its request body.
//
// Regression test: the body was encoded once into a single *bytes.Reader held
// outside the retry loop. A reader is consumed by the first attempt, so every
// retry sent an empty body while still setting Content-Type: application/json —
// the panel would answer 200 and do nothing, which reads as success. The bug
// was latent only because every currently-retried call is a GET with no body;
// it would have surfaced the first time a body-bearing GET was retried.
func TestRetriedGetResendsItsBody(t *testing.T) {
	var calls int32
	var bodies []string

	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		buf := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			if _, err := r.Body.Read(buf); err != nil && err.Error() != "EOF" {
				t.Logf("read body: %v", err)
			}
		}
		bodies = append(bodies, string(buf))
		if n == 1 {
			// Transport-level failure: the client retries a GET.
			// Hijack and close so no response is written.
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					_ = conn.Close()
					return
				}
			}
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"msg":"","obj":{}}`))
	})

	type payload struct {
		Emails []string `json:"emails"`
	}
	body := payload{Emails: []string{"vyx-1@example.invalid"}}

	raw, status, err := c.do(context.Background(), http.MethodGet, "/panel/api/test", body)
	if err != nil {
		t.Fatalf("do: %v (status=%d raw=%s)", err, status, raw)
	}
	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("panel received %d request(s), want at least 2 — the retry did not happen", got)
	}
	// Every attempt must carry the full body. The second attempt is the one
	// that used to arrive empty.
	for i, b := range bodies {
		if b == "" {
			t.Fatalf("attempt %d sent an EMPTY body; want %s", i+1, string(mustJSON(t, body)))
		}
		if !strings.Contains(b, "vyx-1@example.invalid") {
			t.Fatalf("attempt %d body = %q, want it to contain the email", i+1, b)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// TestDialGuardRefusesLoopbackWithoutThePrivatePolicy is the B-4 regression test.
//
// `ParseTarget` checks the addresses a host resolves to, but it does that ONCE,
// at registration. A hostile or compromised DNS server can answer that query with
// a public address and answer the DIAL query with `127.0.0.1` — classic
// DNS rebinding. The dial-time guard inspects the address that is actually
// connected to, so it cannot be raced.
//
// This test uses a real loopback listener and asserts that a client WITHOUT the
// private-network policy cannot reach it, while the SAME target WITH the policy
// can. The second half matters as much as the first: a guard that refuses
// everything would also pass a "refuses loopback" test, and would break every
// self-hosted panel behind an SSH tunnel.
func TestDialGuardRefusesLoopbackWithoutThePrivatePolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(statusBody))
	}))
	defer srv.Close()

	// Same URL, two policies. Only the policy differs.
	strict, err := New(Options{
		Target:  &Target{BaseURL: srv.URL}, // AllowPrivate false
		Token:   "secret-token-value",
		Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatalf("New(strict): %v", err)
	}
	if h := strict.Health(context.Background()); h.Status != StatusConnectionUnavailable {
		t.Fatalf("a client without the private policy reached a loopback panel — status %q, the dial guard is not enforcing", h.Status)
	} else if h.Err == nil || !strings.Contains(h.Err.Error(), "loopback") {
		t.Fatalf("refused for the wrong reason: %v", h.Err)
	}

	relaxed, err := New(Options{
		Target:  &Target{BaseURL: srv.URL, AllowPrivate: true},
		Token:   "secret-token-value",
		Timeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatalf("New(relaxed): %v", err)
	}
	if h := relaxed.Health(context.Background()); h.Status != StatusActive {
		t.Fatalf("a client WITH the private policy could not reach its panel: %q (%v)", h.Status, h.Err)
	}
}

// TestDialGuardRefusesCloudMetadataEvenWithThePrivatePolicy pins the one refusal
// the private-network policy must never lift. IMDS credentials are not panel
// configuration; a rebinding attack that lands on 169.254.169.254 would otherwise
// turn an allowed private-network integration into a credential-theft primitive.
func TestDialGuardRefusesCloudMetadataEvenWithThePrivatePolicy(t *testing.T) {
	for _, ip := range []net.IP{
		net.ParseIP("169.254.169.254"), // AWS / GCP / Azure IMDS
		net.ParseIP("100.100.100.200"), // Alibaba
	} {
		if err := checkIP(ip, true); err == nil {
			t.Fatalf("%s was accepted under the private-network policy — IMDS must never be reachable", ip)
		}
	}
}
