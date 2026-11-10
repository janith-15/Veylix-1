package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// deleteFixture boots the console with a super admin, one client and (optionally)
// a VPN row owned by that client.
func deleteFixture(t *testing.T, withVPN bool) (*httptest.Server, *store.Store, *auth.Service, int64, int64) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir+"/delete.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	auditLog := audit.New(st.DB())
	svc := auth.NewService(st.DB(), auditLog)
	svc.SetAutoVerify(true)

	owner, err := svc.Register(context.Background(), auth.RegisterInput{
		Email: "owner@veylix.test", Password: "owner-long-secret-1", DisplayName: "Owner",
	})
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	if _, err := st.DB().Exec(`UPDATE users SET role = 'super_admin' WHERE id = ?`, owner.ID); err != nil {
		t.Fatalf("promote: %v", err)
	}
	client, err := svc.Register(context.Background(), auth.RegisterInput{
		Email: "client@veylix.test", Password: "client-long-secret-1", DisplayName: "Client",
	})
	if err != nil {
		t.Fatalf("register client: %v", err)
	}
	if withVPN {
		// The real vpn_instances schema: owner_user_id (not client_user_id) and no
		// protocol/total_bytes columns — those live on the panel, not here. A VPN row
		// also needs a server row, because server_id is a NOT NULL foreign key.
		if _, err := st.DB().Exec(`
			INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
			VALUES ('test-panel', 'http://127.0.0.1:1', 'x', 'ACTIVE', 1, 1)`); err != nil {
			t.Fatalf("insert server: %v", err)
		}
		if _, err := st.DB().Exec(`
			INSERT INTO vpn_instances
			  (owner_user_id, server_id, panel_email, panel_sub_id, status, created_at, updated_at)
			VALUES (?, (SELECT id FROM server_integrations LIMIT 1), 'c_free_1', 'vyx-1', 'ACTIVE', 1, 1)`,
			client.ID); err != nil {
			t.Fatalf("insert vpn: %v", err)
		}
	}

	principal := auth.Principal{UserID: owner.ID, Email: "owner@veylix.test", Role: auth.RoleSuperAdmin}
	withActor := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
		})
	}
	mux := http.NewServeMux()
	ClientAdminRoutesWithReset(mux, st, auditLog, withActor, svc)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st, svc, owner.ID, client.ID
}

func doDelete(t *testing.T, srv *httptest.Server, id int64) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/super/clients/"+itoa(id), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	var body map[string]any
	_ = json.NewDecoder(res.Body).Decode(&body)
	return res.StatusCode, body
}

// Deleting a client is a state change that locks them out and keeps the history.
func TestClientDeleteLocksTheAccountOut(t *testing.T) {
	srv, st, svc, _, clientID := deleteFixture(t, false)

	if code, body := doDelete(t, srv, clientID); code != http.StatusOK || body["deleted"] != true {
		t.Fatalf("delete = %d %v, want 200 deleted:true", code, body)
	}

	var status string
	if err := st.DB().QueryRow(`SELECT status FROM users WHERE id = ?`, clientID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != store.ClientStatusDeleted {
		t.Fatalf("status = %q, want %q", status, store.ClientStatusDeleted)
	}

	// The account cannot sign in any more.
	if _, err := svc.Authenticate(context.Background(), "client@veylix.test",
		"client-long-secret-1", "9.9.9.9", auth.PortalClient); err == nil {
		t.Fatal("a deleted account must not be able to sign in")
	}

	// Its sessions are gone.
	var sessions int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, clientID).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("%d session(s) survived the delete", sessions)
	}

	// The row itself is kept: billing and audit history reference it.
	var rows int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, clientID).Scan(&rows); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if rows != 1 {
		t.Fatal("the user row must be kept (soft delete), not removed")
	}
}

// PATCH must not be able to bring a deleted account back.
func TestClientDeleteIsTerminal(t *testing.T) {
	srv, _, _, _, clientID := deleteFixture(t, false)
	if code, _ := doDelete(t, srv, clientID); code != http.StatusOK {
		t.Fatalf("delete = %d, want 200", code)
	}

	req, _ := http.NewRequest(http.MethodPatch,
		srv.URL+"/api/super/clients/"+itoa(clientID), strings.NewReader(`{"status":"ACTIVE"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusOK {
		t.Fatal("PATCH resurrected a deleted account; DELETED must be terminal")
	}
}

// A client that still owns VPNs cannot be deleted: those live on the panel, and
// the rows that name their owner would be left pointing at a ghost.
func TestClientDeleteRefusesWhileVPNsExist(t *testing.T) {
	srv, _, _, _, clientID := deleteFixture(t, true)
	code, body := doDelete(t, srv, clientID)
	if code != http.StatusConflict {
		t.Fatalf("delete with VPNs = %d %v, want 409", code, body)
	}
	if errObj, ok := body["error"].(map[string]any); ok {
		if c, _ := errObj["code"].(string); c != "client_has_vpns" {
			t.Fatalf("error code = %q, want client_has_vpns", c)
		}
	}
}

// Deleting twice is a no-op, not an error: an operator retrying after a timeout
// must not see a failure.
func TestClientDeleteIsIdempotent(t *testing.T) {
	srv, _, _, _, clientID := deleteFixture(t, false)
	if code, _ := doDelete(t, srv, clientID); code != http.StatusOK {
		t.Fatalf("first delete = %d, want 200", code)
	}
	if code, _ := doDelete(t, srv, clientID); code != http.StatusOK {
		t.Fatalf("second delete = %d, want 200 (idempotent)", code)
	}
}
