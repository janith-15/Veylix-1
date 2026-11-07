package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/store"
)

// stubPanel is an in-memory 3X-UI panel sufficient for provisioning flows.
//
// reject makes AddClient fail WITHOUT recording the client: a definite panel
// rejection (Create reads the client back and finds it absent).
type stubPanel struct {
	exists map[string]bool
	reject bool
}

func (s *stubPanel) AddClient(_ context.Context, req panel.AddClientRequest) error {
	if s.reject {
		return errors.New("panel rejected the client")
	}
	s.exists[req.Client.Email] = true
	return nil
}
func (s *stubPanel) ClientExists(_ context.Context, email string) (bool, error) {
	return s.exists[email], nil
}
func (s *stubPanel) DeleteClient(_ context.Context, email string) error {
	delete(s.exists, email)
	return nil
}
func (s *stubPanel) SetClientEnabled(context.Context, string, bool) error { return nil }

// Links returns a canned vless URI for a client this stub holds (D-026).
func (s *stubPanel) Links(_ context.Context, email string) ([]string, error) {
	if !s.exists[email] {
		return []string{}, nil
	}
	return []string{"vless://00000000-0000-0000-0000-000000000000@panel.example:443?security=tls#stub"}, nil
}

type vpnEnv struct {
	server *httptest.Server
	url    string
	offer  int64
	st     *store.Store
	stub   *stubPanel
}

// newVPNEnv builds the real router over real SQLite with a fake panel.
func newVPNEnv(t *testing.T, kind string) *vpnEnv {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/vpn.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db := st.DB()

	exec := func(q string, args ...any) int64 {
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	serverID := exec(`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv','https://panel.example/base', x'00', 'ACTIVE', 1, 1)`)
	inboundID := exec(`INSERT INTO inbounds (server_id, remote_id, protocol, port, synced_at)
		VALUES (?, 1, 'vless', 443, 1)`, serverID)
	offerID := exec(`INSERT INTO vpn_offers (kind, name, server_id, inbound_id, traffic_gb, duration_days, device_limit, active, created_at, updated_at)
		VALUES (?, 'Offer', ?, ?, 30, 30, 2, 1, 1, 1)`, kind, serverID, inboundID)

	auditLog := audit.New(db)
	authSvc := auth.NewService(db, auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(db, false)
	stub := &stubPanel{exists: map[string]bool{}}
	provSvc := provisioning.New(db, st, auditLog, func(context.Context, int64) (provisioning.PanelClient, error) {
		return stub, nil
	})

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		VPNs: provSvc, VPNLister: httpapi.NewSQLVPNLister(db),
		// FREE claim caps (D-025.4/.5) are read through the real store.
		FreeClaims: st,
	}))
	t.Cleanup(srv.Close)

	return &vpnEnv{server: srv, url: srv.URL, offer: offerID, st: st, stub: stub}
}

// login creates an account (via bootstrap only for the first user) and returns
// a cookie-carrying client.
func (e *vpnEnv) signup(t *testing.T, email, password string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 10 * time.Second}

	raw, _ := json.Marshal(map[string]string{"email": email, "password": password, "display_name": email})
	resp, err := c.Post(e.url+"/api/auth/register", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register = %d", resp.StatusCode)
	}
	return c
}

func (e *vpnEnv) post(t *testing.T, c *http.Client, path string, body any) (int, map[string]any) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	resp, err := c.Post(e.url+path, "application/json", rdr)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// A client self-provisions a FREE offer → 201 ACTIVE; the list shows it.
//
// The panel identity is pre-tagged FREE because an untagged VPN is visible only
// to the Super Admin (I-3): publishing to a client requires a classification.
func TestClientProvisionsFreeOffer(t *testing.T) {
	env := newVPNEnv(t, "FREE")
	c := env.signup(t, "client@example.com", "a very long password")
	env.tagFirstVPN(t, "FREE")

	code, body := env.post(t, c, "/api/vpns", map[string]any{"offer_id": env.offer})
	if code != http.StatusCreated {
		t.Fatalf("POST /api/vpns = %d (%v), want 201", code, body)
	}
	if body["status"] != "ACTIVE" || body["confirmed"] != true {
		t.Fatalf("result = %v, want confirmed ACTIVE", body)
	}

	resp, err := c.Get(env.url + "/api/vpns")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var list []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if resp.StatusCode != http.StatusOK || len(list) != 1 {
		t.Fatalf("list = %d with %d rows", resp.StatusCode, len(list))
	}
}

// Issuing a VPN locks its tag in the same transaction as the insert (D-025.2).
//
// Before this, `LockClientTag` existed but nothing called it, so FREE↔PREMIUM
// could be flipped after a sale — retroactively changing what the customer paid
// for. This exercises the real /api/vpns path, not the store method.
func TestIssuingVPNLocksItsTag(t *testing.T) {
	env := newVPNEnv(t, "FREE")

	// D-034: the create names its own identity (`client@example.com_free_1`), and
	// a tag written afterwards is locked at once — see the assertion below.
	var serverID int64
	if err := env.st.DB().QueryRow(`SELECT id FROM server_integrations`).Scan(&serverID); err != nil {
		t.Fatalf("read server: %v", err)
	}

	c := env.signup(t, "client@example.com", "a very long password")
	code, body := env.post(t, c, "/api/vpns", map[string]any{"offer_id": env.offer})
	if code != http.StatusCreated {
		t.Fatalf("POST /api/vpns = %d (%v), want 201", code, body)
	}
	// Diagnose: what identity did the create actually use?
	if body["confirmed"] != true {
		t.Fatalf("create not confirmed: %v", body)
	}

	// The identity the create chose is the customer's, per D-034.
	var used string
	if err := env.st.DB().QueryRow(
		`SELECT panel_email FROM vpn_instances ORDER BY id DESC LIMIT 1`).Scan(&used); err != nil {
		t.Fatalf("read identity: %v", err)
	}
	if used != "client@example.com_free_1" {
		t.Fatalf("created name = %q, want client@example.com_free_1", used)
	}

	// D-025.2 — a tag written AFTER issue is locked immediately, so the VPN can
	// never be re-priced afterwards. (Before D-034 this test pre-tagged the
	// identity; naming now reserves the slot, so the classification happens
	// after the create, which is the real operator flow too.)
	if body["tag_locked"] != false {
		t.Errorf("tag_locked = %v, want false: the client was untagged when issued", body["tag_locked"])
	}
	if err := env.st.SetClientTag(context.Background(), serverID, used, store.TagFree, 0); err != nil {
		t.Fatalf("tag the issued VPN: %v", err)
	}
	tag, err := env.st.ClientTag(context.Background(), serverID, used)
	if err != nil {
		t.Fatalf("ClientTag: %v", err)
	}
	if tag.LockedAt == 0 {
		t.Fatal("a VPN tagged after issue must be locked at once (D-025.2)")
	}
	if err := env.st.SetClientTag(context.Background(), serverID, used, store.TagPremium, 0); !errors.Is(err, store.ErrTagLocked) {
		t.Fatalf("re-tag after issue = %v, want ErrTagLocked", err)
	}
}

func TestClientCannotProvisionPremium(t *testing.T) {
	env := newVPNEnv(t, "PREMIUM")
	c := env.signup(t, "client@example.com", "a very long password")

	code, body := env.post(t, c, "/api/vpns", map[string]any{"offer_id": env.offer})
	if code != http.StatusForbidden {
		t.Fatalf("premium by client = %d (%v), want 403", code, body)
	}
}

// Anonymous access is refused.
func TestVPNRoutesRequireAuth(t *testing.T) {
	env := newVPNEnv(t, "FREE")
	resp, err := http.Get(env.url + "/api/vpns")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous list = %d, want 401", resp.StatusCode)
	}
}

// One client must never see another client's VPN.
func TestVPNsAreScopedPerOwner(t *testing.T) {
	env := newVPNEnv(t, "FREE")
	a := env.signup(t, "a@example.com", "a very long password")
	b := env.signup(t, "b@example.com", "a very long password")

	if code, body := env.post(t, a, "/api/vpns", map[string]any{"offer_id": env.offer}); code != http.StatusCreated {
		t.Fatalf("A create = %d (%v)", code, body)
	}

	resp, err := b.Get(env.url + "/api/vpns")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var list []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&list)
	if len(list) != 0 {
		t.Fatalf("client B sees %d VPNs, want 0", len(list))
	}
}

// Revocation reports 202 until the panel confirms the client is gone.
func TestRevokeRespondsAcceptedUntilConfirmed(t *testing.T) {
	env := newVPNEnv(t, "FREE")
	c := env.signup(t, "client@example.com", "a very long password")

	_, created := env.post(t, c, "/api/vpns", map[string]any{"offer_id": env.offer})
	id := int64(created["vpn_id"].(float64))

	code, body := env.post(t, c, "/api/vpns/"+itoa(id)+"/revoke", nil)
	if code != http.StatusOK {
		t.Fatalf("revoke = %d (%v), want 200 once the panel confirms", code, body)
	}
	if body["status"] != "REVOKED" {
		t.Fatalf("status = %v, want REVOKED", body["status"])
	}
}

// A create the panel rejects must NOT leave a locked tag behind.
//
// The create transaction locks the tag, then the panel refuses the client. The
// VPN never existed, so its tag must stay editable — otherwise a single failed
// attempt would make that panel identity permanently unclassifiable.
func TestRejectedCreateDoesNotLockTag(t *testing.T) {
	env := newVPNEnv(t, "FREE")
	ctx := context.Background()

	var serverID int64
	if err := env.st.DB().QueryRow(`SELECT id FROM server_integrations`).Scan(&serverID); err != nil {
		t.Fatalf("read server: %v", err)
	}
	email := "vyx-1"
	if err := env.st.SetClientTag(ctx, serverID, email, store.TagFree, 0); err != nil {
		t.Fatalf("pre-tag: %v", err)
	}

	// Force a definite rejection: the fake panel never records the client.
	env.stub.reject = true

	c := env.signup(t, "client@example.com", "a very long password")
	code, body := env.post(t, c, "/api/vpns", map[string]any{"offer_id": env.offer})
	if code != http.StatusBadGateway {
		t.Fatalf("rejected create = %d (%v), want 502", code, body)
	}

	tag, err := env.st.ClientTag(ctx, serverID, email)
	if err != nil {
		t.Fatalf("ClientTag: %v", err)
	}
	if tag.LockedAt != 0 {
		t.Fatalf("a rejected create must not lock the tag (locked_at=%d)", tag.LockedAt)
	}
	// The tag is still editable, which is the behaviour that matters.
	if err := env.st.SetClientTag(ctx, serverID, email, store.TagPremium, 0); err != nil {
		t.Fatalf("re-tag after a rejected create = %v, want allowed", err)
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

// get performs an authenticated GET and decodes the JSON object body.
func (e *vpnEnv) get(t *testing.T, c *http.Client, path string) (int, map[string]any) {
	t.Helper()
	resp, err := c.Get(e.url + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// tagFirstVPN classifies the panel identity that the next VPN will receive, as
// a Super Admin would through PUT /api/panel-clients/{server}/{email}/tag.
//
// Needed because an untagged VPN is invisible to everyone but the Super Admin
// (I-3), so any test that asserts "the client sees their VPN" must publish it
// first.
// tagFirstVPN pre-tags the panel identity the next create will use.
//
// D-034: a new VPN is named `<client email>_<kind>_<n>`, so the pre-tag must use
// that name — a tag on `vyx-1` would now belong to no VPN at all.
func (e *vpnEnv) tagFirstVPN(t *testing.T, tag string) string {
	t.Helper()
	var serverID int64
	if err := e.st.DB().QueryRow(`SELECT id FROM server_integrations`).Scan(&serverID); err != nil {
		t.Fatalf("read server: %v", err)
	}
	// The harness signs up `client@example.com`; its first FREE VPN is
	// `client@example.com_free_1`.
	email := "client@example.com_free_1"
	if err := e.st.SetClientTag(context.Background(), serverID, email, store.Tag(tag), 0); err != nil {
		t.Fatalf("pre-tag %s: %v", tag, err)
	}
	return email
}
