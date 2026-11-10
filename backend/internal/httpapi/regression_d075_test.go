package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/store"
)

// d075PanelStub is the panel the provisioning service talks to. Every method is
// a no-op: these regressions are about what Veylix does with a row, never about
// the remote panel, and a real client cannot be constructed in a unit test.
type d075PanelStub struct {
	exists map[string]bool
}

func (s *d075PanelStub) AddClient(context.Context, panel.AddClientRequest) error { return nil }
func (s *d075PanelStub) ClientExists(_ context.Context, email string) (bool, error) {
	return s.exists[email], nil
}
func (s *d075PanelStub) DeleteClient(context.Context, string) error { return nil }
func (s *d075PanelStub) SetClientEnabled(context.Context, string, bool) error {
	return nil
}
func (s *d075PanelStub) Links(context.Context, string) ([]string, error) { return nil, nil }

// D-075 · BUG 2 — a Google-only account is TOLD it has no password.
//
// The defect: `!hash.Valid || !VerifyPassword(...)` collapsed two different facts
// into ErrInvalidCredentials, so somebody who signed up with Google was told
// "email or password is incorrect" and sent to reset a password that was never
// set. That loop has no exit.
func TestLoginRefusesGoogleOnlyAccountWithItsOwnCode(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "googleonly.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := st.DB().ExecContext(ctx,
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES ('googleonly@example.com', NULL, 'Google Only', 'client', 'ACTIVE', 1, 1, 1)`); err != nil {
		t.Fatalf("seed google-only user: %v", err)
	}
	svc := auth.NewService(st.DB(), audit.New(st.DB()))

	_, err = svc.Authenticate(ctx, "googleonly@example.com", "anything", "127.0.0.1", auth.Portal("client"))
	if !errors.Is(err, auth.ErrPasswordNotSet) {
		t.Fatalf("Authenticate = %v, want ErrPasswordNotSet — a Google-only account must not be "+
			"answered with ErrInvalidCredentials, because that sends the customer to reset a "+
			"password that was never set", err)
	}
}

// And the transport must name it, so a UI can point at the Google button.
func TestPasswordNotSetMapsToItsOwnStatusCode(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAuthError(rec, auth.ErrPasswordNotSet)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "password_not_set") {
		t.Errorf("body = %s, want the code password_not_set", rec.Body.String())
	}
}

// D-075 · BUG 3 — the platform carrier is REUSED, not re-created.
//
// The defect: the carrier is created with `active = 0` (it is not a product) but
// the lookup required `active = 1`, so it could never match and every assignment
// inserted another "Platform assignment" row. Four assignments → four rows.
func TestAssignmentReusesThePlatformCarrier(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "carrier.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db := st.DB()
	must := func(q string, args ...any) int64 {
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	server := must(`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv','https://p.example/base', x'00','ACTIVE',1,1)`)
	clientA := must(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('a@example.com','x','A','client','ACTIVE',1,1,1)`)
	clientB := must(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('b@example.com','x','B','client','ACTIVE',1,1,1)`)
	admin := must(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('op@example.com','x','Op','super_admin','ACTIVE',1,1,1)`)

	for i, clientID := range []int64{clientA, clientB} {
		res, err := st.AssignPanelClient(ctx, store.AssignPanelClientInput{
			ServerID: server, PanelEmail: "panel-" + strconv.Itoa(i),
			ClientUserID: clientID, ActorID: admin, Kind: "paid",
		})
		if err != nil {
			t.Fatalf("assign %d: %v", i, err)
		}
		if res.OfferID <= 0 {
			t.Fatalf("assign %d: offer_id = %d, want a real package", i, res.OfferID)
		}
	}

	var carriers int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_offers WHERE kind = 'PREMIUM' AND name = 'Platform assignment'`).
		Scan(&carriers); err != nil {
		t.Fatalf("count carriers: %v", err)
	}
	if carriers != 1 {
		t.Errorf("carrier rows = %d, want 1 — the carrier must be reused, not re-created on "+
			"every assignment (this produced four identical rows in production)", carriers)
	}
	var distinct int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT offer_id) FROM vpn_instances`).Scan(&distinct); err != nil {
		t.Fatalf("count offers: %v", err)
	}
	if distinct != 1 {
		t.Errorf("VPNs reference %d distinct offers, want 1", distinct)
	}
}

// D-075 · BUG 4 — an offer that cannot provision is REFUSED at create time.
//
// The defect: `inbound_id` was never validated, so POST /api/offers answered 201
// for a row that could never build a client. A FREE offer published that way left
// every customer seeing "no free offers available" with no error anywhere.
func TestOfferCreateRefusesMissingInbound(t *testing.T) {
	srv, c, serverID := newD075Env(t)

	code, body := d075Do(t, c, http.MethodPost, srv.URL+"/api/offers", map[string]any{
		"kind": "FREE", "name": "Free 10 GB",
		"server_id": serverID, "inbound_id": 0,
		"traffic_gb": 10, "duration_days": 30, "active": true,
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("create = %d %v, want 422 offer_incomplete — an offer with no inbound can "+
			"never build a client, so saving it silently strands every customer", code, body)
	}
	if got := errorCode(body); got != "offer_incomplete" {
		t.Errorf("error code = %q, want offer_incomplete", got)
	}
}

// D-075 · BUG 1 (backend half) — the protocol read must survive a VPN with no
// inbound, which is what every ASSIGNED panel client looks like (D-063).
func TestProtocolSurvivesMissingInbound(t *testing.T) {
	srv, c, serverID := newD075Env(t)

	// A VPN with NO inbound_id and NO offer: exactly the shape an assigned panel
	// client has (D-063 records `inbound_id = 0`).
	if _, err := d075Store(t).DB().Exec(
		`INSERT INTO vpn_instances (owner_user_id, server_id, offer_id, panel_email, panel_sub_id, status, created_at, updated_at)
		 VALUES (1, ?, NULL, 'no-inbound', 'sub-x', 'ACTIVE', 1, 1)`, serverID); err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	var vpnID int64
	if err := d075Store(t).DB().QueryRow(
		`SELECT id FROM vpn_instances WHERE panel_email = 'no-inbound'`).Scan(&vpnID); err != nil {
		t.Fatalf("read vpn id: %v", err)
	}

	code, body := d075Do(t, c, http.MethodGet,
		srv.URL+"/api/vpns/"+strconv.FormatInt(vpnID, 10)+"/protocol", nil)
	if code == http.StatusInternalServerError {
		t.Fatalf("GET protocol = 500 %v — scanning a NULL port behind a LEFT JOIN broke the "+
			"customer's own detail page", body)
	}
	if code == http.StatusNotFound {
		// No skip: this harness mounts the VPN routes, so a 404 means the route
		// regressed, not that the test could not run. Skipping is how BUG 1
		// stayed "covered" while covering nothing.
		t.Fatalf("GET protocol = 404 — the VPN routes are no longer mounted, so this "+
			"regression is unguarded. Body: %v", body)
	}
	if code != http.StatusOK {
		t.Fatalf("GET protocol = %d %v, want 200 — a VPN with no inbound is a normal "+
			"ASSIGNED panel client (D-063), not an error", code, body)
	}
}

// d075Env is a migrated store behind the real router, with Offers and VPNs wired
// — the two routes these regressions need and the existing helpers do not both
// provide. It lives in `package httpapi` (not `httpapi_test`) because the fixes it
// guards are internal error values.
type d075Env struct {
	srv      *httptest.Server
	st       *store.Store
	serverID int64
	client   *http.Client
}

var d075Current *d075Env

func d075Store(t *testing.T) *store.Store {
	t.Helper()
	if d075Current == nil {
		t.Fatal("d075Store called before newD075Env")
	}
	return d075Current.st
}

func newD075Env(t *testing.T) (*httptest.Server, *http.Client, int64) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "d075.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	db := st.DB()
	res, err := db.ExecContext(ctx,
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('srv','https://p.example/base', x'00','ACTIVE',1,1)`)
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
	serverID, _ := res.LastInsertId()

	auditLog := audit.New(db)
	sessions := auth.NewSessionStore(db, false)
	authSvc := auth.NewService(db, auditLog)
	// The VPN routes must be mounted, not assumed away: this harness previously
	// left `VPNs` nil, so GET /api/vpns/{id}/protocol answered 404 and
	// TestProtocolSurvivesMissingInbound SKIPPED itself. The regression it guards
	// (BUG 1 — a 500 on the customer's own detail page for every ASSIGNED panel
	// client, whose inbound_id is 0) reported as covered while covering nothing.
	provSvc := provisioning.New(db, st, auditLog, func(context.Context, int64) (provisioning.PanelClient, error) {
		return &d075PanelStub{exists: map[string]bool{}}, nil
	})
	srv := httptest.NewServer(Router(Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		Servers: st, Offers: st, OfferAudit: auditLog,
		PanelTags: st, Assignments: st, Transfers: st,
		VPNs: provSvc, VPNLister: NewSQLVPNLister(db),
		// The protocol route lives in VPNConnectionRoutes, which the router only
		// mounts when this is non-nil. Without it the route is absent and the
		// regression below cannot run at all.
		VPNConnection: provSvc,
	}))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	const pw = "a very long password"
	resp, body := d075Do(t, c, http.MethodPost, srv.URL+"/api/setup/bootstrap",
		map[string]string{"email": "owner@example.com", "password": pw, "display_name": "Owner"})
	if resp != http.StatusOK && resp != http.StatusCreated {
		t.Fatalf("bootstrap = %d %v", resp, body)
	}
	resp, body = d075Do(t, c, http.MethodPost, srv.URL+"/api/auth/login",
		map[string]string{"identifier": "owner@example.com", "password": pw, "portal": "staff"})
	if resp != http.StatusOK {
		t.Fatalf("staff login = %d %v", resp, body)
	}

	env := &d075Env{srv: srv, st: st, serverID: serverID, client: c}
	d075Current = env
	t.Cleanup(func() { d075Current = nil })
	return srv, c, serverID
}

// d075Do issues one request against the router, with or without a JSON body.
func d075Do(t *testing.T, c *http.Client, method, url string, payload any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// errorCode reads `error.code` from a refusal body.
//
// Declared here rather than shared with auth_e2e_test.go: that file is
// `package httpapi_test` (an external test package), so its helpers are not
// visible from this one.
func errorCode(body map[string]any) string {
	if errObj, ok := body["error"].(map[string]any); ok {
		if code, ok := errObj["code"].(string); ok {
			return code
		}
	}
	return ""
}
