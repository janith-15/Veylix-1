package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
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
	"veylix/backend/internal/store"
)

// assignEnv is one migrated store behind the router, with the three accounts.
type assignEnv struct {
	url     string
	st      *store.Store
	renames *fakeRenamer
	client  int64
	admin   int64
	server  int64
	http    *http.Client
}

type fakeRenamer struct {
	calls []string
	err   error
}

func (f *fakeRenamer) RenameClient(_ context.Context, serverID int64, from, to string) error {
	f.calls = append(f.calls, from+"→"+to)
	return f.err
}

func newAssignEnv(t *testing.T) *assignEnv {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "assign.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db := st.DB()
	mustExec := func(q string, args ...any) int64 {
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	server := mustExec(`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv','https://p.example/base', x'00','ACTIVE',1,1)`)
	client := mustExec(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('nimal@gmail.com','x','Nimal','client','ACTIVE',1,1,1)`)
	admin := mustExec(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('reseller@example.com','x','Reseller','admin','ACTIVE',1,1,1)`)

	renamer := &fakeRenamer{}
	auditLog := audit.New(db)
	authSvc := auth.NewService(db, auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(db, false)
	srv := httptest.NewServer(Router(Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		Servers:      st,
		PanelTags:    st,
		Assignments:  st,
		PanelRenamer: renamer,
		AdminAudit:   auditLog,
		Pricing:      st,
		Plans:        st,
		Payments:     st,
		OrderActions: st,
	}))
	_ = auditLog
	t.Cleanup(srv.Close)

	// A real signed-in Super Admin. Bootstrap is refused once any account
	// exists, and this environment seeds accounts, so a hashed password row is
	// written directly and the client signs in — the same path an operator takes.
	hash, err := auth.HashPassword("a very long password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET role = 'super_admin', password_hash = ? WHERE id = ?`,
		hash, admin); err != nil {
		t.Fatalf("promote admin: %v", err)
	}

	jar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	raw, _ := json.Marshal(map[string]string{"email": "reseller@example.com", "password": "a very long password", "portal": "staff"})
	resp, err := adminClient.Post(srv.URL+"/api/auth/login", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d", resp.StatusCode)
	}

	return &assignEnv{url: srv.URL, st: st, renames: renamer, client: client, admin: admin,
		server: server, http: adminClient}
}

func (e *assignEnv) assign(t *testing.T, server int64, email string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost,
		e.url+"/api/panel-clients/"+strconv.FormatInt(server, 10)+"/"+email+"/assign", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := e.http.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// D-063 — assigning an unassigned panel client creates the VPN row under the
// name the PANEL holds. A rename is optional and off unless asked for, because a
// forced rename is what produced one VPN under two names when it failed.
func TestAssignPanelClientRoute(t *testing.T) {
	env := newAssignEnv(t)

	code, body := env.assign(t, env.server, "direct-3xui-1", map[string]any{
		"client_user_id": env.client, "kind": "paid",
	})
	if code != http.StatusCreated {
		t.Fatalf("status = %d (%v), want 201", code, body)
	}
	if body["panel_email"] != "direct-3xui-1" {
		t.Errorf("panel_email = %v, want the panel's own name direct-3xui-1", body["panel_email"])
	}
	if body["tag"] != "PREMIUM" {
		t.Errorf("tag = %v, want PREMIUM", body["tag"])
	}
	if body["assigned"] != true {
		t.Errorf("assigned = %v, want true", body["assigned"])
	}
	if body["renamed"] != false {
		t.Errorf("renamed = %v, want false — a rename must be opt-in", body["renamed"])
	}
	if len(env.renames.calls) != 0 {
		t.Errorf("rename calls = %v, want none when the operator did not ask", env.renames.calls)
	}
	// An assigned VPN always carries a package: without one the client screen has
	// no kind to render and shows the VPN as UNTAGGED.
	if offerID, _ := body["offer_id"].(float64); offerID <= 0 {
		t.Errorf("offer_id = %v, want a real package", body["offer_id"])
	}

	var owner int64
	if err := env.st.DB().QueryRow(`SELECT owner_user_id FROM vpn_instances`).Scan(&owner); err != nil {
		t.Fatalf("read vpn: %v", err)
	}
	if owner != env.client {
		t.Errorf("owner = %d, want %d", owner, env.client)
	}
}

// An assignment is a premium issue: asking for a FREE one is refused rather than
// silently recorded with a contradictory tag and package.
func TestAssignRouteRefusesFreeKind(t *testing.T) {
	env := newAssignEnv(t)

	code, body := env.assign(t, env.server, "direct-3xui-9", map[string]any{
		"client_user_id": env.client, "kind": "free",
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d (%v), want 422", code, body)
	}
	var rows int
	if err := env.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 0 {
		t.Errorf("vpn rows = %d, want 0 — a refused assignment must write nothing", rows)
	}
}

// D-063 / D-034.5 — the rename is OPT-IN, and it is the customer-visible half of
// the naming scheme: after it the panel shows `nimal@gmail.com_paid_1` rather
// than the name the client was created with in 3X-UI. This asserts exactly one
// rename over the real HTTP route, and that the VPN row stores the new identity —
// a rename that is announced but not persisted would drift back on the next sync.
func TestAssignRenameWhenAsked(t *testing.T) {
	env := newAssignEnv(t)

	code, body := env.assign(t, env.server, "direct-3xui-2", map[string]any{
		"client_user_id": env.client, "kind": "paid", "rename": true,
	})
	if code != http.StatusCreated {
		t.Fatalf("status = %d (%v), want 201", code, body)
	}
	if body["renamed"] != true {
		t.Errorf("renamed = %v, want true", body["renamed"])
	}
	if body["assigned"] != true {
		t.Errorf("assigned = %v, want true", body["assigned"])
	}

	if len(env.renames.calls) != 1 {
		t.Fatalf("rename calls = %v, want exactly one", env.renames.calls)
	}
	// The fake records "from→to", so split on the recorded separator rather than
	// joining them here, which keeps this assertion independent of the fixture.
	call := env.renames.calls[0]
	if !strings.Contains(call, "direct-3xui-2") {
		t.Errorf("rename record %q does not start from the panel's original name", call)
	}
	if !strings.Contains(call, "nimal@gmail.com_paid_1") {
		t.Errorf("rename record %q does not end at the D-034 identity", call)
	}

	var panelEmail string
	if err := env.st.DB().QueryRow(
		`SELECT panel_email FROM vpn_instances WHERE owner_user_id = ?`, env.client,
	).Scan(&panelEmail); err != nil {
		t.Fatalf("read vpn: %v", err)
	}
	if panelEmail != "nimal@gmail.com_paid_1" {
		t.Errorf("stored panel_email = %q, want the renamed identity", panelEmail)
	}
}

// A reseller is not a client: a VPN can only be given to a customer.
func TestAssignRouteRefusesNonClient(t *testing.T) {
	env := newAssignEnv(t)

	code, body := env.assign(t, env.server, "direct-3xui-2", map[string]any{
		"client_user_id": env.admin, "kind": "paid",
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d (%v), want 422", code, body)
	}
	if code2, _ := env.assign(t, env.server, "direct-3xui-2", map[string]any{}); code2 != http.StatusUnprocessableEntity {
		t.Errorf("missing client_user_id = %d, want 422", code2)
	}
}

// A rename the panel does not confirm must NOT fail the assignment (D-063).
//
// The old contract answered 502 rename_failed even though the VPN had been
// assigned, so the operator saw a failure for work that succeeded and re-clicked
// into a 409. The assignment stands, the VPN keeps the name the panel holds, and
// the answer says exactly that.
func TestAssignRouteKeepsNameWhenRenameFails(t *testing.T) {
	env := newAssignEnv(t)
	env.renames.err = errFakeRename

	code, body := env.assign(t, env.server, "direct-3xui-3", map[string]any{
		"client_user_id": env.client, "kind": "paid", "rename": true,
	})
	if code != http.StatusCreated {
		t.Fatalf("status = %d (%v), want 201 — the assignment succeeded", code, body)
	}
	if body["assigned"] != true {
		t.Errorf("assigned = %v, want true", body["assigned"])
	}
	if body["renamed"] != false {
		t.Errorf("renamed = %v, want false", body["renamed"])
	}
	if body["panel_email"] != "direct-3xui-3" {
		t.Errorf("panel_email = %v, want the name the panel still holds", body["panel_email"])
	}
	if warn, _ := body["warning"].(string); !strings.Contains(warn, "keeps its existing name") {
		t.Errorf("warning = %q, want a note that the existing name was kept", warn)
	}

	// The row is exactly where the panel is: one name, one row, still premium.
	var (
		panelEmail string
		rows       int
	)
	if err := env.st.DB().QueryRow(
		`SELECT panel_email FROM vpn_instances WHERE owner_user_id = ?`, env.client,
	).Scan(&panelEmail); err != nil {
		t.Fatalf("read vpn: %v", err)
	}
	if panelEmail != "direct-3xui-3" {
		t.Errorf("stored panel_email = %q, want the panel's own name", panelEmail)
	}
	if err := env.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("vpn rows = %d, want 1 — a failed rename must never duplicate the VPN", rows)
	}
}

// Assigning the same panel client twice is refused, not duplicated — and that is
// the check that used to miss, because the row was stored under the NEW name
// while the panel still offered the old one.
func TestAssignRouteRefusesDoubleAssign(t *testing.T) {
	env := newAssignEnv(t)
	body := map[string]any{"client_user_id": env.client, "kind": "paid"}

	if code, out := env.assign(t, env.server, "direct-3xui-4", body); code != http.StatusCreated {
		t.Fatalf("first assign = %d (%v), want 201", code, out)
	}
	if code, out := env.assign(t, env.server, "direct-3xui-4", body); code != http.StatusConflict {
		t.Fatalf("second assign = %d (%v), want 409 panel_client_taken", code, out)
	}
	var rows int
	if err := env.st.DB().QueryRow(`SELECT COUNT(*) FROM vpn_instances`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("vpn rows = %d, want 1 — one panel client is one VPN", rows)
	}
}

var errFakeRename = &fakeRenameErr{}

type fakeRenameErr struct{}

func (*fakeRenameErr) Error() string { return "panel rename failed" }
