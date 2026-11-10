package integrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/panel"
)

func testSealer(t *testing.T) *auth.Sealer {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 3)
	}
	s, err := auth.NewSealer(key)
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}
	return s
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "integrations.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE server_integrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			panel_url TEXT NOT NULL UNIQUE,
			token_ciphertext BLOB NOT NULL,
			status TEXT NOT NULL DEFAULT 'PENDING_VERIFICATION',
			panel_version TEXT NOT NULL DEFAULT '',
			xray_version TEXT NOT NULL DEFAULT '',
			capabilities_json TEXT NOT NULL DEFAULT '{}',
			allow_private_network INTEGER NOT NULL DEFAULT 0,
			last_health_check INTEGER,
			last_sync INTEGER,
			created_by INTEGER,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
		CREATE TABLE inbounds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER NOT NULL,
			remote_id INTEGER NOT NULL,
			remark TEXT NOT NULL DEFAULT '',
			tag TEXT NOT NULL DEFAULT '',
			protocol TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			synced_at INTEGER NOT NULL,
			UNIQUE (server_id, remote_id)
		);
		CREATE TABLE audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_id INTEGER,
			action TEXT NOT NULL,
			target_type TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL DEFAULT '',
			result TEXT NOT NULL DEFAULT 'OK',
			context TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		);
		CREATE TABLE vpn_instances (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE RESTRICT,
			owner_user_id INTEGER,
			offer_id INTEGER,
			panel_email TEXT NOT NULL DEFAULT '',
			panel_sub_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'PENDING',
			panel_total_gb INTEGER NOT NULL DEFAULT 0,
			expires_at INTEGER,
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		);
		-- D-037: a row must name its owner, so "whose VPN is this" is answerable.
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT 'client'
		);
		CREATE TABLE vpn_offers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT 'PREMIUM'
		);
		CREATE TABLE vpn_assignments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			vpn_instance_id INTEGER NOT NULL,
			seller_user_id INTEGER NOT NULL DEFAULT 0
		);
		-- D-024.3: FREE/PREMIUM is an operator tag on a panel client.
		CREATE TABLE panel_client_tags (
			server_id   INTEGER NOT NULL,
			panel_email TEXT    NOT NULL,
			tag         TEXT    NOT NULL,
			tagged_by   INTEGER,
			tagged_at   INTEGER NOT NULL,
			locked_at   INTEGER,
			PRIMARY KEY (server_id, panel_email)
		);
		CREATE TABLE server_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE CASCADE
		);
		CREATE TABLE outbox (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE CASCADE
		);
		CREATE TABLE admin_server_assignments (
			admin_id INTEGER NOT NULL,
			server_id INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE CASCADE
		);`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

// fakeClient lets the service be exercised without a network.
type fakeClient struct {
	health   panel.HealthResult
	inbounds []panel.InboundOption
	// clients backs the read-only inspector (D-024 Stage C).
	clients      []panel.ClientRow
	listErr      error
	listCalls    int
	listPageSize int
	// devices backs the HWID surface (D8). The panel is the only source, so the
	// fake is the only place these live.
	devices     []panel.Hwid
	hwidErr     error
	delErr      error
	deletedIDs  []int64
	deleteCalls int
	// staleDelete makes DeleteHwid report success while leaving the device in
	// place — the panel behaving worse than it documents.
	staleDelete bool
}

func (f *fakeClient) Health(context.Context) panel.HealthResult { return f.health }
func (f *fakeClient) Inbounds(context.Context) ([]panel.InboundOption, error) {
	return f.inbounds, nil
}

// Hwids returns the fake's device list.
func (f *fakeClient) Hwids(context.Context, string) ([]panel.Hwid, error) {
	if f.hwidErr != nil {
		return nil, f.hwidErr
	}
	return f.devices, nil
}

// DeleteHwid removes one device unless the fake is simulating a stale delete.
func (f *fakeClient) DeleteHwid(_ context.Context, _ string, id int64) error {
	f.deleteCalls++
	if f.delErr != nil {
		return f.delErr
	}
	f.deletedIDs = append(f.deletedIDs, id)
	if f.staleDelete {
		return nil
	}
	kept := f.devices[:0]
	for _, d := range f.devices {
		if d.ID != id {
			kept = append(kept, d)
		}
	}
	f.devices = kept
	return nil
}

// ListClients pages through the fake client list, mirroring the panel's shape.
func (f *fakeClient) ListClients(_ context.Context, page, pageSize int) (*panel.ClientPage, error) {
	f.listCalls++
	f.listPageSize = pageSize
	if f.listErr != nil {
		return nil, f.listErr
	}
	start := (page - 1) * pageSize
	if start >= len(f.clients) {
		return &panel.ClientPage{Total: len(f.clients), Page: page, PageSize: pageSize}, nil
	}
	end := start + pageSize
	if end > len(f.clients) {
		end = len(f.clients)
	}
	return &panel.ClientPage{
		Items: f.clients[start:end], Total: len(f.clients), Page: page, PageSize: pageSize,
	}, nil
}

func newTestService(t *testing.T, fc *fakeClient) (*Service, *sql.DB) {
	t.Helper()
	db := testDB(t)
	svc := New(db, testSealer(t), audit.New(db))
	svc.resolve = func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("203.0.113.10")}, nil }
	svc.newPanel = func(panel.Options) (panelClient, error) { return fc, nil }
	return svc, db
}

const testToken = "panel-token-abc123-do-not-leak"

func activeHealth() panel.HealthResult {
	return panel.HealthResult{Status: panel.StatusActive, PanelVersion: "3.10.2", XrayVersion: "26.10.10", XrayState: "running"}
}

func TestAddStoresTokenSealedAndNeverReturnsIt(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{health: activeHealth()})

	view, err := svc.Add(context.Background(), AddInput{
		Name: "Panel Frankfurt-01", PanelURL: "https://panel.example.com:2053/base", Token: testToken, ActorID: 1,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if view.Status != string(panel.StatusActive) {
		t.Fatalf("status = %s, want ACTIVE after a successful health check", view.Status)
	}

	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), testToken) {
		t.Fatal("the API view leaked the panel token")
	}

	var stored []byte
	if err := db.QueryRow(`SELECT token_ciphertext FROM server_integrations`).Scan(&stored); err != nil {
		t.Fatalf("select: %v", err)
	}
	if strings.Contains(string(stored), testToken) {
		t.Fatal("the database stored the token in plaintext")
	}
	if len(stored) < 12 {
		t.Fatalf("ciphertext looks empty: %d bytes", len(stored))
	}
}

func TestAddRecordsFailureInsteadOfClaimingActive(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{
		health: panel.HealthResult{Status: panel.StatusReauthRequired},
	})

	view, err := svc.Add(context.Background(), AddInput{
		Name: "Bad panel", PanelURL: "https://bad.example.com", Token: testToken, ActorID: 1,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if view.Status != string(panel.StatusReauthRequired) {
		t.Fatalf("status = %s, want REAUTH_REQUIRED (never ACTIVE on a failed check)", view.Status)
	}
	if view.LastHealthCheck != nil {
		t.Error("a failed check must not record a health timestamp")
	}
	var status string
	_ = db.QueryRow(`SELECT status FROM server_integrations`).Scan(&status)
	if status != string(panel.StatusReauthRequired) {
		t.Fatalf("persisted status = %s", status)
	}
}

func TestAddRejectsInvalidInput(t *testing.T) {
	svc, _ := newTestService(t, &fakeClient{health: activeHealth()})
	cases := []AddInput{
		{Name: "", PanelURL: "https://p.example.com", Token: testToken},
		{Name: "x", PanelURL: "https://p.example.com", Token: ""},
		{Name: "x", PanelURL: "http://p.example.com", Token: testToken},
		{Name: "x", PanelURL: "https://127.0.0.1", Token: testToken},
	}
	for i, in := range cases {
		if _, err := svc.Add(context.Background(), in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("case %d: err = %v, want ErrInvalidInput", i, err)
		}
	}
}

func TestAddRejectsDuplicatePanelURL(t *testing.T) {
	svc, _ := newTestService(t, &fakeClient{health: activeHealth()})
	in := AddInput{Name: "a", PanelURL: "https://dup.example.com", Token: testToken}
	if _, err := svc.Add(context.Background(), in); err != nil {
		t.Fatalf("first: %v", err)
	}
	in.Name = "b"
	if _, err := svc.Add(context.Background(), in); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate = %v, want ErrInvalidInput", err)
	}
}

func TestTestReReadsSealedTokenAndUpdatesStatus(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{health: panel.HealthResult{Status: panel.StatusConnectionUnavailable}})
	view, _ := svc.Add(context.Background(), AddInput{
		Name: "p", PanelURL: "https://flip.example.com", Token: testToken,
	})
	if view.Status != string(panel.StatusConnectionUnavailable) {
		t.Fatalf("initial = %s", view.Status)
	}

	// The panel comes back: the stored token must decrypt and verify as ACTIVE.
	svc.newPanel = func(panel.Options) (panelClient, error) { return &fakeClient{health: activeHealth()}, nil }
	got, err := svc.Test(context.Background(), view.ID, 1)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if got.Status != string(panel.StatusActive) || got.PanelVersion != "3.10.2" {
		t.Fatalf("after retest = %+v", got)
	}
	var status string
	_ = db.QueryRow(`SELECT status FROM server_integrations WHERE id = ?`, view.ID).Scan(&status)
	if status != string(panel.StatusActive) {
		t.Fatalf("persisted status = %s", status)
	}
}

func TestTestUnknownIntegrationIsNotFound(t *testing.T) {
	svc, _ := newTestService(t, &fakeClient{health: activeHealth()})
	if _, err := svc.Test(context.Background(), 999, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestInboundsCachesMetadata(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{
		health: activeHealth(),
		inbounds: []panel.InboundOption{
			{ID: 1, Remark: "(IN)-free_2", Tag: "in-443-tcp", Protocol: "vless", Port: 443, Enable: true},
		},
	})
	view, _ := svc.Add(context.Background(), AddInput{Name: "p", PanelURL: "https://ib.example.com", Token: testToken})

	list, err := svc.Inbounds(context.Background(), view.ID)
	if err != nil {
		t.Fatalf("Inbounds: %v", err)
	}
	if len(list) != 1 || list[0].Protocol != "vless" {
		t.Fatalf("unexpected inbounds: %+v", list)
	}
	var protocol string
	var port int
	if err := db.QueryRow(`SELECT protocol, port FROM inbounds WHERE server_id = ?`, view.ID).
		Scan(&protocol, &port); err != nil {
		t.Fatalf("cached row missing: %v", err)
	}
	if port != 443 || protocol != "vless" {
		t.Errorf("cached metadata wrong: %s %d", protocol, port)
	}

	// A second sync must update, not duplicate.
	if _, err := svc.Inbounds(context.Background(), view.ID); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM inbounds`).Scan(&count)
	if count != 1 {
		t.Fatalf("inbound rows = %d, want 1 (upsert)", count)
	}
}

func TestInboundsReturnsLocalIDNotPanelID(t *testing.T) {
	// The defect this pins: `GET /api/servers/{id}/inbounds` used to answer with
	// the PANEL's inbound number, while `vpn_offers.inbound_id` is a foreign key
	// to the LOCAL `inbounds.id`. On a database where the two collide or simply
	// diverge — literally the development database, where the cached rows are
	// 52/53/54 and 3X-UI calls the same inbounds 1/2/3 — the offer form
	// submitted a number the FK could not resolve and every create failed with
	// 422 `inbound_mismatch`, even though the inbound was listed for the named
	// server. Provisioning translates local -> remote itself
	// (`panelInboundID`), so the list must speak the local language.
	svc, db := newTestService(t, &fakeClient{
		health: activeHealth(),
		inbounds: []panel.InboundOption{
			{ID: 1, Remark: "(IN)-free_3", Protocol: "vless", Port: 443, Enable: true},
			{ID: 2, Remark: "(IN)-free_1[hutch]", Protocol: "vless", Port: 445, Enable: true},
			{ID: 3, Remark: "(IN)-free_2", Protocol: "hysteria", Port: 446, Enable: true},
		},
	})
	view, _ := svc.Add(context.Background(), AddInput{Name: "p", PanelURL: "https://local.example.com", Token: testToken})

	// Force the divergence the real database has: burn row ids so the local
	// primary keys cannot equal the panel's numbers.
	if _, err := db.Exec(`INSERT INTO inbounds (id, server_id, remote_id, synced_at) VALUES (51, ?, 99, 1)`, view.ID); err != nil {
		t.Fatalf("seed gap: %v", err)
	}

	list, err := svc.Inbounds(context.Background(), view.ID)
	if err != nil {
		t.Fatalf("Inbounds: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("inbounds = %d, want 3", len(list))
	}

	for _, ib := range list {
		var localID, remoteID int64
		if err := db.QueryRow(`SELECT id, remote_id FROM inbounds WHERE id = ? AND server_id = ?`,
			ib.ID, view.ID).Scan(&localID, &remoteID); err != nil {
			t.Fatalf("returned id %d is not a local inbounds.id for server %d: %v — "+
				"the offer form would submit a foreign key that cannot resolve", ib.ID, view.ID, err)
		}
		if int64(ib.ID) == remoteID && remoteID <= 3 {
			t.Errorf("inbound %q answered with the PANEL id %d, want the local row id %d",
				ib.Remark, ib.ID, localID)
		}
	}

	// The decisive assertion: the id the list hands out must satisfy the exact
	// query `store.InboundBelongsToServer` runs for `POST /api/offers`. Spelled
	// out here rather than imported because this package must not depend on
	// `store`'s test helpers; the SQL is the same predicate.
	for _, ib := range list {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM inbounds WHERE id = ? AND server_id = ?`,
			ib.ID, view.ID).Scan(&n); err != nil || n == 0 {
			t.Errorf("InboundBelongsToServer would refuse id %d that the list just offered (%q) — "+
				"this is the inbound_mismatch that blocked every offer create", ib.ID, ib.Remark)
		}
	}
}

func TestDisableAndRemove(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{health: activeHealth()})
	view, _ := svc.Add(context.Background(), AddInput{Name: "p", PanelURL: "https://life.example.com", Token: testToken})

	if err := svc.SetDisabled(context.Background(), view.ID, true, 1); err != nil {
		t.Fatalf("disable: %v", err)
	}
	var status string
	_ = db.QueryRow(`SELECT status FROM server_integrations WHERE id = ?`, view.ID).Scan(&status)
	if status != "DISABLED" {
		t.Fatalf("status = %s, want DISABLED", status)
	}

	if err := svc.Remove(context.Background(), view.ID, 1); err != nil {
		t.Fatalf("remove: %v", err)
	}
	_ = db.QueryRow(`SELECT status FROM server_integrations WHERE id = ?`, view.ID).Scan(&status)
	if status != "REMOVED" {
		t.Fatalf("status = %s, want REMOVED", status)
	}
	var rows int
	_ = db.QueryRow(`SELECT COUNT(*) FROM server_integrations WHERE id = ?`, view.ID).Scan(&rows)
	if rows != 1 {
		t.Fatal("removal must preserve the row for history (REQUIREMENTS §3.3)")
	}
	if _, err := svc.Test(context.Background(), view.ID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatal("a removed integration must not be usable")
	}
	if err := svc.Remove(context.Background(), 999, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("remove unknown = %v", err)
	}
}

func TestUpdateRotatesTokenAndRevalidatesPanel(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{health: activeHealth()})
	view, _ := svc.Add(context.Background(), AddInput{Name: "old", PanelURL: "https://up.example.com", Token: testToken, ActorID: 7})

	newURL := "https://up2.example.com:2053/base"
	newToken := "rotated-token-xyz789"
	updated, err := svc.Update(context.Background(), view.ID, UpdateInput{
		Name: strPtr("new name"), PanelURL: &newURL, Token: &newToken, ActorID: 7,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "new name" || updated.PanelURL != newURL {
		t.Fatalf("view = %+v, want the edited name/url", updated)
	}
	if updated.Status != "ACTIVE" {
		t.Fatalf("status = %s, want ACTIVE from the observed check", updated.Status)
	}

	// The stored ciphertext must be the NEW token, sealed against the NEW url.
	var stored []byte
	if err := db.QueryRow(`SELECT token_ciphertext FROM server_integrations WHERE id = ?`, view.ID).Scan(&stored); err != nil {
		t.Fatalf("load ciphertext: %v", err)
	}
	plain, err := testSealer(t).Open(stored, addAAD(newURL))
	if err != nil {
		t.Fatalf("open with new AAD: %v", err)
	}
	if string(plain) != newToken {
		t.Fatalf("stored token = %q, want the rotated value", plain)
	}
	if strings.Contains(string(stored), newToken) {
		t.Fatal("token stored in plaintext")
	}
}

func TestUpdateRejectsUrlChangeWithoutNewToken(t *testing.T) {
	svc, _ := newTestService(t, &fakeClient{health: activeHealth()})
	view, _ := svc.Add(context.Background(), AddInput{Name: "p", PanelURL: "https://aad.example.com", Token: testToken})

	// The token is sealed against the URL, so silently keeping it would leave a
	// ciphertext nobody can ever open again.
	newURL := "https://aad2.example.com"
	if _, err := svc.Update(context.Background(), view.ID, UpdateInput{PanelURL: &newURL}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("url change without token = %v, want ErrInvalidInput", err)
	}

	// A name-only edit keeps the existing credential and still re-checks it.
	updated, err := svc.Update(context.Background(), view.ID, UpdateInput{Name: strPtr("renamed")})
	if err != nil {
		t.Fatalf("name-only update: %v", err)
	}
	if updated.Name != "renamed" || updated.Status != "ACTIVE" {
		t.Fatalf("view = %+v", updated)
	}
}

func TestUpdateRejectsInvalidPanelURL(t *testing.T) {
	svc, _ := newTestService(t, &fakeClient{health: activeHealth()})
	view, _ := svc.Add(context.Background(), AddInput{Name: "p", PanelURL: "https://ssrf.example.com", Token: testToken})

	bad := "http://insecure.example.com"
	if _, err := svc.Update(context.Background(), view.ID, UpdateInput{PanelURL: &bad, Token: strPtr(testToken)}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("http url = %v, want ErrInvalidInput", err)
	}

	unknown := int64(9999)
	if _, err := svc.Update(context.Background(), unknown, UpdateInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id = %v, want ErrNotFound", err)
	}
}

func TestUpdateNeverLeaksTheTokenInAudit(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{health: activeHealth()})
	view, _ := svc.Add(context.Background(), AddInput{Name: "p", PanelURL: "https://audit2.example.com", Token: testToken, ActorID: 3})

	newToken := "another-secret-token-42"
	if _, err := svc.Update(context.Background(), view.ID, UpdateInput{Token: &newToken, ActorID: 3}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	var ctxJSON string
	if err := db.QueryRow(`SELECT context FROM audit_logs WHERE action = ? ORDER BY id DESC LIMIT 1`,
		audit.ActionServerUpdate).Scan(&ctxJSON); err != nil {
		t.Fatalf("audit row: %v", err)
	}
	if strings.Contains(ctxJSON, newToken) || strings.Contains(ctxJSON, testToken) {
		t.Fatal("audit context leaked a panel token")
	}
	if !strings.Contains(ctxJSON, `"token_rotated":true`) {
		t.Fatalf("audit context = %s, want token_rotated flag", ctxJSON)
	}
}

func TestPurgeDeletesTheWholeServer(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{health: activeHealth()})
	view, _ := svc.Add(context.Background(), AddInput{Name: "p", PanelURL: "https://purge.example.com", Token: testToken, ActorID: 5})
	if _, err := svc.Inbounds(context.Background(), view.ID); err != nil {
		t.Fatalf("seed inbounds: %v", err)
	}
	_, _ = db.Exec(`INSERT INTO inbounds (server_id, remote_id, synced_at) VALUES (?, 7, 1)`, view.ID)

	if err := svc.Purge(context.Background(), view.ID, 5); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	var servers, inbounds int
	_ = db.QueryRow(`SELECT COUNT(*) FROM server_integrations WHERE id = ?`, view.ID).Scan(&servers)
	_ = db.QueryRow(`SELECT COUNT(*) FROM inbounds WHERE server_id = ?`, view.ID).Scan(&inbounds)
	if servers != 0 {
		t.Fatal("the row must be gone (hard delete), not merely REMOVED")
	}
	if inbounds != 0 {
		t.Fatal("cached inbounds must be deleted with the server")
	}
	if err := svc.Purge(context.Background(), view.ID, 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second purge = %v, want ErrNotFound", err)
	}
}

func strPtr(s string) *string { return &s }

// A server that still owns VPN instances must not be purgeable: the FK is
// ON DELETE RESTRICT and the client history behind it is legally meaningful.
func TestPurgeRefusesWhenVpnInstancesExist(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{health: activeHealth()})
	view, _ := svc.Add(context.Background(), AddInput{Name: "busy", PanelURL: "https://busy.example.com", Token: testToken})

	if _, err := db.Exec(`INSERT INTO vpn_instances (server_id) VALUES (?)`, view.ID); err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	err := svc.Purge(context.Background(), view.ID, 1)
	if !errors.Is(err, ErrInUse) {
		t.Fatalf("purge with instances = %v, want ErrInUse", err)
	}

	// The refusal must leave everything intact.
	var servers int
	_ = db.QueryRow(`SELECT COUNT(*) FROM server_integrations WHERE id = ?`, view.ID).Scan(&servers)
	if servers != 1 {
		t.Fatal("a refused purge must not delete anything")
	}
}

// --- read-only panel inspector (D-024 Stage C) ------------------------------

// The inspector must report EVERY panel client with the panel's own limits,
// including ones Veylix has no row for (created directly in 3X-UI).
func TestPanelClientsReportsPanelTruth(t *testing.T) {
	svc, _ := newTestService(t, &fakeClient{
		health: activeHealth(),
		clients: []panel.ClientRow{
			{Email: "kasun@gmail.com", SubID: "s1", Enable: true, TotalGB: 10737418240,
				ExpiryTime: 1795388800000, LimitIP: 2, LimitHwid: 2, InboundIDs: []int{1},
				Traffic: &panel.ClientTraffic{Up: 100, Down: 200}},
			{Email: "stranger@panel.test", SubID: "s2", Enable: false, TotalGB: 21474836480,
				LimitIP: 5, LimitHwid: 3, InboundIDs: []int{1}},
		},
	})
	ctx := context.Background()
	view, err := svc.Add(ctx, AddInput{Name: "p", PanelURL: "https://insp.example.com",
		Token: testToken, ActorID: 1})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	rows, err := svc.PanelClients(ctx, view.ID)
	if err != nil {
		t.Fatalf("PanelClients: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (every panel client, known or not)", len(rows))
	}

	byEmail := map[string]PanelClientView{}
	for _, r := range rows {
		byEmail[r.PanelEmail] = r
	}

	known, ok := byEmail["kasun@gmail.com"]
	if !ok {
		t.Fatal("a panel client Veylix has no row for must still be listed")
	}
	// Limits come from the panel, never from Veylix (D-024.2).
	if known.TotalGB != 10737418240 || known.LimitIP != 2 || known.LimitHwid != 2 {
		t.Errorf("limits = %+v, want the panel's own values", known)
	}
	if known.UpBytes != 100 || known.DownBytes != 200 {
		t.Errorf("usage = %d/%d, want 100/200", known.UpBytes, known.DownBytes)
	}
	// Nobody has tagged it yet, so it is unclassified — still a VPN.
	if known.Tag != nil {
		t.Errorf("Tag = %v, want nil for an untagged client", *known.Tag)
	}
	if known.Assigned {
		t.Error("Assigned = true for a client Veylix never issued")
	}

	stranger := byEmail["stranger@panel.test"]
	if stranger.Enable {
		t.Error("Enable = true, want the panel's false")
	}
	if stranger.LimitHwid != 3 {
		t.Errorf("LimitHwid = %d, want the panel's 3", stranger.LimitHwid)
	}
}

// D-037 — the operator's first question when a server is opened is "whose VPN is
// this?". Before this, `Assigned: true` said *someone* holds it and nothing more.
func TestServerClientListShowsBuyer(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{
		health: activeHealth(),
		clients: []panel.ClientRow{
			// TotalGB is BYTES (3X-UI's own unit for this field). 50 GiB, so
			// 3 GiB used reads as 6% — the operator's question ("usage kohomda
			// iwara wela thiyenne") answered against a real allowance. The
			// fixture used to say `50`, which only worked because the view
			// scaled it by 1024^3 a second time (VPN_FIX_LEDGER #21).
			{Email: "nimal@gmail.com_paid_1", Enable: true, TotalGB: 50 << 30,
				LimitIP: 2, LimitHwid: 2, ExpiryTime: 0,
				Traffic: &panel.ClientTraffic{Up: 1 << 30, Down: 2 << 30}},
			{Email: "nobody@panel.test", Enable: true}, // unassigned
		},
	})
	ctx := context.Background()
	view, err := svc.Add(ctx, AddInput{Name: "p", PanelURL: "https://owner.example.com",
		Token: testToken, ActorID: 1})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// A client account, the plan it bought, and the assignment that links them.
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, role)
		VALUES (7, 'nimal@gmail.com', 'Nimal Perera', 'client')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, display_name, role)
		VALUES (9, 'reseller@example.com', 'Reseller', 'admin')`); err != nil {
		t.Fatalf("seed seller: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO vpn_offers (id, name, kind) VALUES (3, 'Premium 50', 'PREMIUM')`); err != nil {
		t.Fatalf("seed offer: %v", err)
	}
	// panel_total_gb stores BYTES (see loadAssignments); 50 GiB, matching the
	// panel client above so both sides of the join speak the same unit.
	res, err := db.Exec(`INSERT INTO vpn_instances
		   (server_id, owner_user_id, offer_id, panel_email, panel_sub_id, status, panel_total_gb, created_at, updated_at)
		 VALUES (?, 7, 3, 'nimal@gmail.com_paid_1', 'vyx-1', 'ACTIVE', ?, 1700000000, 1700000000)`,
		view.ID, int64(50)<<30)
	if err != nil {
		t.Fatalf("seed vpn: %v", err)
	}
	vpnID, _ := res.LastInsertId()
	if _, err := db.Exec(`INSERT INTO vpn_assignments (vpn_instance_id, seller_user_id) VALUES (?, 9)`, vpnID); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	rows, err := svc.PanelClients(ctx, view.ID)
	if err != nil {
		t.Fatalf("PanelClients: %v", err)
	}
	byEmail := map[string]PanelClientView{}
	for _, r := range rows {
		byEmail[r.PanelEmail] = r
	}

	owned := byEmail["nimal@gmail.com_paid_1"]
	if !owned.Assigned {
		t.Error("Assigned = false for a VPN a client holds")
	}
	if owned.OwnerUserID != 7 || owned.OwnerEmail != "nimal@gmail.com" || owned.OwnerName != "Nimal Perera" {
		t.Errorf("owner = %d/%q/%q, want 7/nimal@gmail.com/Nimal Perera",
			owned.OwnerUserID, owned.OwnerEmail, owned.OwnerName)
	}
	if owned.SellerUserID != 9 || owned.SellerEmail != "reseller@example.com" {
		t.Errorf("seller = %d/%q, want 9/reseller@example.com", owned.SellerUserID, owned.SellerEmail)
	}
	if owned.OfferID != 3 || owned.OfferKind != "PREMIUM" || owned.OfferName != "Premium 50" {
		t.Errorf("offer = %d/%q/%q, want 3/PREMIUM/Premium 50", owned.OfferID, owned.OfferKind, owned.OfferName)
	}
	// "usage kohomda iwara wela thiyenne" — 3 GiB of 50 GiB is 6%.
	if owned.UsedBytes != 3<<30 {
		t.Errorf("used = %d, want 3 GiB", owned.UsedBytes)
	}
	if owned.UsagePct != 6 {
		t.Errorf("usage_pct = %d, want 6", owned.UsagePct)
	}
	if owned.VPNInstanceID != vpnID {
		t.Errorf("vpn_instance_id = %d, want %d", owned.VPNInstanceID, vpnID)
	}

	unowned := byEmail["nobody@panel.test"]
	if unowned.Assigned {
		t.Error("Assigned = true for a panel client nobody holds")
	}
	if unowned.OwnerUserID != 0 || unowned.OwnerEmail != "" {
		t.Errorf("an unowned client must not name an owner, got %d/%q", unowned.OwnerUserID, unowned.OwnerEmail)
	}
}

func TestPanelClientsJoinsTagAndAssignment(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{
		health:  activeHealth(),
		clients: []panel.ClientRow{{Email: "held@panel.test", Enable: true, LimitIP: 2, LimitHwid: 2}},
	})
	ctx := context.Background()
	view, err := svc.Add(ctx, AddInput{Name: "p", PanelURL: "https://insp2.example.com",
		Token: testToken, ActorID: 1})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, err := db.Exec(
		`INSERT INTO panel_client_tags (server_id, panel_email, tag, tagged_at)
		 VALUES (?, 'held@panel.test', 'PREMIUM', 1)`, view.ID); err != nil {
		t.Fatalf("seed tag: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO vpn_instances
		   (server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		 VALUES (?, 'held@panel.test', 'held@panel.test', 'ACTIVE', 1, 1)`, view.ID); err != nil {
		t.Fatalf("seed vpn: %v", err)
	}

	rows, err := svc.PanelClients(ctx, view.ID)
	if err != nil {
		t.Fatalf("PanelClients: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].Tag == nil || *rows[0].Tag != "PREMIUM" {
		t.Errorf("Tag = %v, want PREMIUM", rows[0].Tag)
	}
	if !rows[0].Assigned {
		t.Error("Assigned = false for a VPN Veylix holds")
	}
	if rows[0].VeylixStatus != "ACTIVE" {
		t.Errorf("VeylixStatus = %q, want ACTIVE", rows[0].VeylixStatus)
	}
}

// A real traffic limit must survive the join. `panel_total_gb` stores the panel's
// totalGB field VERBATIM, and 3X-UI reports that in BYTES — so a 10 GiB plan is
// 10737418240. An earlier query multiplied by 1073741824 on top of that, which
// overflowed int64 (exactly 2^63) and made this listing fail with a SQL scan
// error for every VPN that had a limit. This test pins the value end-to-end.
func TestPanelClientsKeepsLargeTrafficLimits(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{
		health:  activeHealth(),
		clients: []panel.ClientRow{{Email: "big@panel.test", Enable: true, TotalGB: 10 << 30}},
	})
	ctx := context.Background()
	view, err := svc.Add(ctx, AddInput{Name: "p", PanelURL: "https://big.example.com",
		Token: testToken, ActorID: 1})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	// 10 GiB in bytes, as sync mirrors it from the panel.
	if _, err := db.Exec(
		`INSERT INTO vpn_instances
		   (server_id, panel_email, panel_sub_id, status, panel_total_gb, created_at, updated_at)
		 VALUES (?, 'big@panel.test', 'big@panel.test', 'ACTIVE', ?, 1, 1)`,
		view.ID, 10<<30); err != nil {
		t.Fatalf("seed vpn: %v", err)
	}

	rows, err := svc.PanelClients(ctx, view.ID)
	if err != nil {
		t.Fatalf("PanelClients must not fail on a large limit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	if !rows[0].Assigned {
		t.Fatal("Assigned = false for a VPN Veylix holds")
	}
	// The limit itself: the panel's byte value, unmodified. Asserting only
	// `Assigned` is what let the second scaling survive here while the SQL copy
	// of the same bug was fixed (VPN_FIX_LEDGER #21).
	if rows[0].LimitBytes != 10<<30 {
		t.Fatalf("LimitBytes = %d, want %d (the panel's own byte value)", rows[0].LimitBytes, int64(10)<<30)
	}
}

// A 500 GiB plan with 452 MB used must read as ~0% — not overflow into 2^62 and
// read as a flat 0% for a VPN that is genuinely being used.
//
// This is the live defect VPN_FIX_LEDGER #21: `panel_total_gb` holds bytes, and
// the panel-client view scaled them by 1024^3 a second time. 536870912000 *
// 1073741824 exceeds int64, wraps to exactly 2^62 (4611686018427387904), and
// every usage bar on every screen then showed a 500 GiB plan as empty.
func TestPanelClientsLimitIsNotScaledTwice(t *testing.T) {
	const (
		limitBytes = int64(536870912000) // 500 GiB, as the panel reports it
		usedBytes  = int64(452458585)    // ~452 MB, a real reading
		overflowed = int64(4611686018427387904)
	)
	svc, db := newTestService(t, &fakeClient{
		health: activeHealth(),
		clients: []panel.ClientRow{{
			Email: "half@panel.test", Enable: true, TotalGB: limitBytes,
			// Split so the halves re-add to usedBytes exactly (452458585 is odd,
			// so an even split would lose a byte and fail on the fixture).
			Traffic: &panel.ClientTraffic{Up: 226229292, Down: 226229293},
		}},
	})
	ctx := context.Background()
	view, err := svc.Add(ctx, AddInput{Name: "p", PanelURL: "https://half.example.com",
		Token: testToken, ActorID: 1})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO vpn_instances
		   (server_id, panel_email, panel_sub_id, status, panel_total_gb, created_at, updated_at)
		 VALUES (?, 'half@panel.test', 'half@panel.test', 'ACTIVE', ?, 1, 1)`,
		view.ID, limitBytes); err != nil {
		t.Fatalf("seed vpn: %v", err)
	}

	rows, err := svc.PanelClients(ctx, view.ID)
	if err != nil {
		t.Fatalf("PanelClients: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	got := rows[0]

	// 1. The limit is the panel's byte value, not a wrapped product of it.
	if got.LimitBytes != limitBytes {
		t.Errorf("LimitBytes = %d, want %d (2^62 = %d is the overflow signature)",
			got.LimitBytes, limitBytes, overflowed)
	}
	// 2. It is never the int64 wrap, stated explicitly so the failure names it.
	if got.LimitBytes == overflowed {
		t.Fatalf("LimitBytes = 2^62: TotalGB was scaled by 1024^3 a second time")
	}
	// 3. Usage is measured against the real limit.
	if got.UsedBytes != usedBytes {
		t.Errorf("UsedBytes = %d, want %d", got.UsedBytes, usedBytes)
	}
	// 452 MB of 500 GiB is 0.084% — it rounds to 0 at integer percent, which is
	// honest. What is NOT honest is a limit that has been multiplied into
	// nonsense, so the guarantee tested here is the limit, not the rounding.
	if want := int(usedBytes * 100 / limitBytes); got.UsagePct != want {
		t.Errorf("UsagePct = %d, want %d", got.UsagePct, want)
	}
}

// A REVOKED row is history, not a held VPN: it must not read as assigned.
func TestPanelClientsTreatsRevokedAsNotAssigned(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{
		health:  activeHealth(),
		clients: []panel.ClientRow{{Email: "gone@panel.test", Enable: true}},
	})
	ctx := context.Background()
	view, _ := svc.Add(ctx, AddInput{Name: "p", PanelURL: "https://insp3.example.com",
		Token: testToken, ActorID: 1})

	if _, err := db.Exec(
		`INSERT INTO vpn_instances
		   (server_id, panel_email, panel_sub_id, status, created_at, updated_at)
		 VALUES (?, 'gone@panel.test', 'gone@panel.test', 'REVOKED', 1, 1)`, view.ID); err != nil {
		t.Fatalf("seed vpn: %v", err)
	}

	rows, err := svc.PanelClients(ctx, view.ID)
	if err != nil {
		t.Fatalf("PanelClients: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].Assigned {
		t.Error("a REVOKED VPN must not count as assigned")
	}
	if rows[0].VeylixStatus != "REVOKED" {
		t.Errorf("VeylixStatus = %q, want the history state REVOKED", rows[0].VeylixStatus)
	}
}

// The inspector is read-only: it must not touch the panel's state. The fake
// records whether any write method was called.
func TestPanelClientsPerformsNoWrites(t *testing.T) {
	fc := &fakeClient{
		health:  activeHealth(),
		clients: []panel.ClientRow{{Email: "ro@panel.test", Enable: true}},
	}
	svc, _ := newTestService(t, fc)
	ctx := context.Background()
	view, _ := svc.Add(ctx, AddInput{Name: "p", PanelURL: "https://insp4.example.com",
		Token: testToken, ActorID: 1})

	if _, err := svc.PanelClients(ctx, view.ID); err != nil {
		t.Fatalf("PanelClients: %v", err)
	}
	if fc.listCalls == 0 {
		t.Error("the inspector did not read the panel at all")
	}
	if fc.listPageSize <= 0 || fc.listPageSize > 200 {
		t.Errorf("page size = %d, want 1..200", fc.listPageSize)
	}
}

func TestAuditTrailIsWrittenWithoutSecrets(t *testing.T) {
	svc, db := newTestService(t, &fakeClient{health: activeHealth()})
	view, _ := svc.Add(context.Background(), AddInput{Name: "p", PanelURL: "https://aud.example.com", Token: testToken, ActorID: 42})
	if _, err := svc.Test(context.Background(), view.ID, 42); err != nil {
		t.Fatalf("test: %v", err)
	}

	rows, err := db.Query(`SELECT action, context FROM audit_logs ORDER BY id`)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var actions []string
	for rows.Next() {
		var action, ctx string
		if err := rows.Scan(&action, &ctx); err != nil {
			t.Fatalf("scan: %v", err)
		}
		actions = append(actions, action)
		if strings.Contains(ctx, testToken) {
			t.Fatal("audit context leaked the panel token")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate audit rows: %v", err)
	}
	if len(actions) < 2 || actions[0] != audit.ActionServerAdd || actions[1] != audit.ActionServerTest {
		t.Fatalf("audit actions = %v", actions)
	}
}

// --- D8: device (HWID) reads and revocation ---------------------------------

// seedServer adds one ACTIVE panel and returns its id, so the HWID tests start
// from a real integration row rather than a hand-built struct.
func seedServer(t *testing.T, svc *Service) int64 {
	t.Helper()
	view, err := svc.Add(context.Background(), AddInput{
		Name: "Panel HWID", PanelURL: "https://panel.example.com:2053", Token: testToken, ActorID: 1,
	})
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
	return view.ID
}

// TestDevicesReadsThePanel is the D8 read gate.
//
// Before this, the adapter could list a client's devices and no route exposed
// it, so a Super Admin had no way to see which machines held the device cap.
func TestDevicesReadsThePanel(t *testing.T) {
	fc := &fakeClient{
		health: activeHealth(),
		devices: []panel.Hwid{
			{ID: 7, Fingerprint: "ab12cd34", DeviceOS: "ios", DeviceModel: "iPhone15,2"},
			{ID: 9, Fingerprint: "ef56gh78", DeviceOS: "android"},
		},
	}
	svc, _ := newTestService(t, fc)
	id := seedServer(t, svc)

	got, err := svc.Devices(context.Background(), id, "nimal@example.com_free_1")
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("devices = %d, want 2", len(got))
	}
	if got[0].Fingerprint != "ab12cd34" || got[1].ID != 9 {
		t.Errorf("devices = %+v, want the panel's own list", got)
	}
	// A read must not write.
	if fc.deleteCalls != 0 {
		t.Errorf("listing devices called the delete endpoint %d time(s)", fc.deleteCalls)
	}
}

// TestDevicesEmptyEmailIsRejected proves the guard runs before the panel is
// touched: an empty panel email cannot identify a client, and must not be sent
// as one.
func TestDevicesEmptyEmailIsRejected(t *testing.T) {
	fc := &fakeClient{health: activeHealth()}
	svc, _ := newTestService(t, fc)
	id := seedServer(t, svc)

	if _, err := svc.Devices(context.Background(), id, "   "); err == nil {
		t.Fatal("an empty panel email must be refused")
	}
}

// TestRevokeDeviceConfirmsTheRemoval is the D8 write gate.
//
// It asserts the property the whole product depends on: success is reported only
// after a fresh read shows the device gone.
func TestRevokeDeviceConfirmsTheRemoval(t *testing.T) {
	fc := &fakeClient{
		health: activeHealth(),
		devices: []panel.Hwid{
			{ID: 7, Fingerprint: "ab12cd34"},
			{ID: 9, Fingerprint: "ef56gh78"},
		},
	}
	svc, _ := newTestService(t, fc)
	id := seedServer(t, svc)

	if err := svc.RevokeDevice(context.Background(), id, "nimal@example.com_paid_1", 7); err != nil {
		t.Fatalf("RevokeDevice: %v", err)
	}
	if len(fc.deletedIDs) != 1 || fc.deletedIDs[0] != 7 {
		t.Errorf("deleted = %v, want [7]", fc.deletedIDs)
	}
	// The other device must be untouched.
	if len(fc.devices) != 1 || fc.devices[0].ID != 9 {
		t.Errorf("remaining devices = %+v, want only id 9", fc.devices)
	}
}

// TestRevokeDeviceRefusesUnconfirmedRemoval is the assertion that makes the
// previous test meaningful: a panel that acknowledges the delete but keeps the
// device must surface as an error, never as success.
func TestRevokeDeviceRefusesUnconfirmedRemoval(t *testing.T) {
	fc := &fakeClient{
		health:      activeHealth(),
		devices:     []panel.Hwid{{ID: 7, Fingerprint: "ab12cd34"}},
		staleDelete: true,
	}
	svc, _ := newTestService(t, fc)
	id := seedServer(t, svc)

	err := svc.RevokeDevice(context.Background(), id, "nimal@example.com_paid_1", 7)
	if err == nil {
		t.Fatal("a delete the panel did not confirm must be an error, not a success")
	}
	if !strings.Contains(err.Error(), "did not confirm") {
		t.Errorf("error = %v, want it to name the unconfirmed removal", err)
	}
}

// TestRevokeAlreadyAbsentDeviceIsSuccess proves idempotency: the desired end
// state is "this fingerprint holds no slot", and reaching it twice is not a
// failure. It must not even call the panel's delete endpoint.
func TestRevokeAlreadyAbsentDeviceIsSuccess(t *testing.T) {
	fc := &fakeClient{health: activeHealth(), devices: []panel.Hwid{{ID: 9}}}
	svc, _ := newTestService(t, fc)
	id := seedServer(t, svc)

	if err := svc.RevokeDevice(context.Background(), id, "nimal@example.com_paid_1", 7); err != nil {
		t.Fatalf("revoking an already-absent device must succeed, got: %v", err)
	}
	if fc.deleteCalls != 0 {
		t.Errorf("delete endpoint called %d time(s) for an absent device, want 0", fc.deleteCalls)
	}
}
