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
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// transferHTTPEnv is one migrated store behind the router, with a Super Admin
// already signed in — the only role allowed to move a VPN (D-064).
type transferHTTPEnv struct {
	url      string
	st       *store.Store
	server   int64
	vpnID    int64
	owner    int64
	receiver int64
	http     *http.Client
}

func newTransferHTTPEnv(t *testing.T) *transferHTTPEnv {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "transfer-http.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	db := st.DB()
	mustID := func(q string, args ...any) int64 {
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	serverID := mustID(`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv','https://p.example/base', x'00','ACTIVE',1,1)`)
	ownerID := mustID(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('janith14@example.com','x','Janith 14','client','ACTIVE',1,1,1)`)
	receiverID := mustID(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('janith13@example.com','x','Janith 13','client','ACTIVE',1,1,1)`)
	adminID := mustID(`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		VALUES ('owner@example.com','x','Owner','super_admin','ACTIVE',1,1,1)`)
	offerID := mustID(`INSERT INTO vpn_offers (kind, name, server_id, active, created_at, updated_at)
		VALUES ('PREMIUM','Premium 100',?,1,1,1)`, serverID)
	vpnID := mustID(`INSERT INTO vpn_instances
		(owner_user_id, server_id, offer_id, panel_email, panel_sub_id, status, created_at, updated_at)
		VALUES (?, ?, ?, 'dn-test', 'sub-abc', 'ACTIVE', 1, 1)`,
		ownerID, serverID, offerID)
	mustID(`INSERT INTO vpn_assignments (vpn_instance_id, client_user_id, seller_user_id, status, started_at, created_at, updated_at)
		VALUES (?, ?, ?, 'ACTIVE', 1, 1, 1)`, vpnID, ownerID, adminID)

	hash, err := auth.HashPassword("a very long password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, adminID); err != nil {
		t.Fatalf("set password: %v", err)
	}

	auditLog := audit.New(db)
	sessions := auth.NewSessionStore(db, false)
	authSvc := auth.NewService(db, auditLog)
	srv := httptest.NewServer(Router(Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		Servers: st, PanelTags: st, Assignments: st, Transfers: st,
		PanelTagAudit: auditLog, AdminAudit: auditLog,
		VPNs: nil, VPNLister: nil,
	}))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	raw, _ := json.Marshal(map[string]string{
		"email": "owner@example.com", "password": "a very long password", "portal": "staff",
	})
	resp, err := client.Post(srv.URL+"/api/auth/login", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d, want 200", resp.StatusCode)
	}

	return &transferHTTPEnv{url: srv.URL, st: st, server: serverID, vpnID: vpnID,
		owner: ownerID, receiver: receiverID, http: client}
}

// transfer posts one move and returns the status with the decoded body.
func (e *transferHTTPEnv) transfer(t *testing.T, vpnID int64, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost,
		e.url+"/api/vpns/"+strconv.FormatInt(vpnID, 10)+"/transfer", bytes.NewReader(raw))
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

// errCodeOf reads error.code from a refusal body.
func errCodeOf(body map[string]any) string {
	if e, ok := body["error"].(map[string]any); ok {
		if c, ok := e["code"].(string); ok {
			return c
		}
	}
	return ""
}

// D-064 — the owner's own example: a VPN held by janith-14 is moved to
// janith-13 and stays a working premium VPN for its new holder.
func TestTransferRouteMovesVPN(t *testing.T) {
	env := newTransferHTTPEnv(t)

	code, body := env.transfer(t, env.vpnID, map[string]any{
		"to_client_user_id": env.receiver, "reason": "customer asked",
	})
	if code != http.StatusOK {
		t.Fatalf("transfer = %d %v, want 200", code, body)
	}
	from, _ := body["from"].(map[string]any)
	to, _ := body["to"].(map[string]any)
	if from["user_id"] != float64(env.owner) {
		t.Errorf("from = %v, want %d", from["user_id"], env.owner)
	}
	if to["user_id"] != float64(env.receiver) {
		t.Errorf("to = %v, want %d", to["user_id"], env.receiver)
	}
	if body["panel_email"] != "dn-test" {
		t.Errorf("panel_email = %v, want dn-test — the panel client is not renamed", body["panel_email"])
	}
	if body["kind"] != "PREMIUM" {
		t.Errorf("kind = %v, want PREMIUM", body["kind"])
	}
}

// TestTransferRouteRefusesSelf: the same account twice is a 422 with a code the
// console can explain, never a silent no-op.
func TestTransferRouteRefusesSelf(t *testing.T) {
	env := newTransferHTTPEnv(t)
	code, body := env.transfer(t, env.vpnID, map[string]any{"to_client_user_id": env.owner})
	if code != http.StatusUnprocessableEntity || errCodeOf(body) != "cannot_transfer_self" {
		t.Fatalf("transfer self = %d %v, want 422 cannot_transfer_self", code, body)
	}
}

// TestTransferRouteRefusesNonClient: an operator is not a customer, so a VPN may
// not be filed in an operator's own book.
func TestTransferRouteRefusesNonClient(t *testing.T) {
	env := newTransferHTTPEnv(t)
	// A second operator, so the refusal is about the ROLE and not about the VPN
	// already belonging to that account.
	operator := int64(0)
	res, err := env.st.DB().Exec(
		`INSERT INTO users (email, password_hash, display_name, role, status, email_verified, created_at, updated_at)
		 VALUES ('reseller2@example.com','x','Reseller 2','admin','ACTIVE',1,1,1)`)
	if err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	operator, _ = res.LastInsertId()

	code, body := env.transfer(t, env.vpnID, map[string]any{"to_client_user_id": operator})
	if code != http.StatusUnprocessableEntity || errCodeOf(body) != "not_a_client" {
		t.Fatalf("transfer to operator = %d %v, want 422 not_a_client", code, body)
	}
}

// TestTransferRouteRefusesUnknownVPN: a bad id is a 404.
func TestTransferRouteRefusesUnknownVPN(t *testing.T) {
	env := newTransferHTTPEnv(t)
	code, body := env.transfer(t, 987654, map[string]any{"to_client_user_id": env.receiver})
	if code != http.StatusNotFound || errCodeOf(body) != "not_found" {
		t.Fatalf("unknown vpn = %d %v, want 404 not_found", code, body)
	}
}

// TestTransferRouteRequiresTarget: a body without an account is refused before
// anything is written.
func TestTransferRouteRequiresTarget(t *testing.T) {
	env := newTransferHTTPEnv(t)
	code, body := env.transfer(t, env.vpnID, map[string]any{})
	if code != http.StatusUnprocessableEntity || errCodeOf(body) != "validation_failed" {
		t.Fatalf("missing target = %d %v, want 422 validation_failed", code, body)
	}
}

// TestTransferRouteHistory: the move is readable afterwards, so "who moved what
// and when" survives the request.
func TestTransferRouteHistory(t *testing.T) {
	env := newTransferHTTPEnv(t)
	if code, body := env.transfer(t, env.vpnID, map[string]any{
		"to_client_user_id": env.receiver, "reason": "customer asked",
	}); code != http.StatusOK {
		t.Fatalf("transfer = %d %v", code, body)
	}

	res, err := env.http.Get(env.url + "/api/vpns/" + strconv.FormatInt(env.vpnID, 10) + "/transfers")
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("history = %d, want 200", res.StatusCode)
	}
	var rows []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("history = %d rows, want 1", len(rows))
	}
	if rows[0]["from_email"] != "janith14@example.com" || rows[0]["to_email"] != "janith13@example.com" {
		t.Errorf("history = %v → %v", rows[0]["from_email"], rows[0]["to_email"])
	}
	if rows[0]["reason"] != "customer asked" {
		t.Errorf("reason = %v, want the operator's note", rows[0]["reason"])
	}
}

// TestTransferRouteAuditsTheMove: an ownership change with no audit entry is
// indistinguishable from an accident, so the trail is part of the contract.
func TestTransferRouteAuditsTheMove(t *testing.T) {
	env := newTransferHTTPEnv(t)
	if code, body := env.transfer(t, env.vpnID, map[string]any{
		"to_client_user_id": env.receiver,
	}); code != http.StatusOK {
		t.Fatalf("transfer = %d %v", code, body)
	}

	var n int
	if err := env.st.DB().QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE action = ?`, audit.ActionVPNTransfer).Scan(&n); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
}

// AdminVPNSummary tells the console WHAT IS THERE before it filters, so the
// operator stops counting rows by hand (Phase-1 UX, P2).
//
// The two properties that matter: the buckets describe the UNFILTERED catalogue
// (a tile must not change its number because a different tile was clicked), and
// a row whose panel could not be read is counted as `unread` only — never also
// as `expired` or `disabled`, which would turn "we could not ask" into a fact.
func TestAdminVPNSummaryBuckets(t *testing.T) {
	p := func(s string) *string { return &s }
	rows := []AdminVPNRow{
		// premium, assigned, healthy, 30 days left
		{VPNRow: VPNRow{Tag: p("PREMIUM"), Assigned: true}, Enabled: true, DaysLeft: 30,
			UsedBytes: 10, LimitBytes: 100},
		// premium, assigned, expiring in 3 days
		{VPNRow: VPNRow{Tag: p("PREMIUM"), Assigned: true}, Enabled: true, DaysLeft: 3},
		// free, unassigned, expired
		{VPNRow: VPNRow{Tag: p("FREE")}, Enabled: true, DaysLeft: -2},
		// untagged, unassigned, traffic exhausted
		{VPNRow: VPNRow{Tag: nil}, Enabled: true, UsedBytes: 100, LimitBytes: 100},
		// premium, assigned, switched off on the panel
		{VPNRow: VPNRow{Tag: p("PREMIUM"), Assigned: true}, Enabled: false, DaysLeft: 20},
		// panel could not be read: only `unread` may count it
		{VPNRow: VPNRow{Tag: p("PREMIUM"), Assigned: true}, ReadError: true},
	}

	s := summariseVPNs(rows)

	if s.Total != 6 {
		t.Errorf("total = %d, want 6", s.Total)
	}
	if s.Premium != 4 {
		t.Errorf("premium = %d, want 4", s.Premium)
	}
	if s.Free != 1 {
		t.Errorf("free = %d, want 1", s.Free)
	}
	if s.Untagged != 1 {
		t.Errorf("untagged = %d, want 1", s.Untagged)
	}
	if s.Assigned != 4 || s.Unassigned != 2 {
		t.Errorf("assigned/unassigned = %d/%d, want 4/2", s.Assigned, s.Unassigned)
	}
	if s.ExpiringSoon != 1 {
		t.Errorf("expiring_soon = %d, want 1", s.ExpiringSoon)
	}
	if s.Expired != 1 {
		t.Errorf("expired = %d, want 1", s.Expired)
	}
	if s.Exhausted != 1 {
		t.Errorf("exhausted = %d, want 1", s.Exhausted)
	}
	if s.Disabled != 1 {
		t.Errorf("disabled = %d, want 1", s.Disabled)
	}
	if s.Unread != 1 {
		t.Errorf("unread = %d, want 1", s.Unread)
	}
}

// TestTransferDoesNotDuplicateTheVPN (regression): the list joins assignments, so
// a transfer that closes one row and opens another made the same VPN appear
// twice. That is exactly what a customer saw immediately after receiving a moved
// VPN.
//
// The join is now restricted to the LIVE assignment, and a seller who once sold
// the VPN still sees it through the history subquery, so neither the owner's
// list nor the reseller's book loses a row.
func TestTransferDoesNotDuplicateTheVPN(t *testing.T) {
	env := newTransferHTTPEnv(t)
	ctx := context.Background()
	if _, err := env.st.TransferVPN(ctx, store.TransferVPNInput{
		VPNID: env.vpnID, ToClientID: env.receiver, ActorID: 1,
	}); err != nil {
		t.Fatalf("TransferVPN: %v", err)
	}

	lister := NewSQLVPNLister(env.st.DB())
	rows, err := lister.ListVPNs(ctx,
		auth.Principal{UserID: env.receiver, Role: auth.RoleClient}, TagFilterAny)
	if err != nil {
		t.Fatalf("ListVPNs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("new owner sees %d rows for one VPN, want 1", len(rows))
	}
	if rows[0].ID != env.vpnID {
		t.Errorf("row id = %d, want %d", rows[0].ID, env.vpnID)
	}
}
