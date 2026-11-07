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

// A Super Admin must be able to give a CLIENT a new password: reset is an e-mail
// and verification is an e-mail, so an account whose mailbox is wrong or gone has
// no other way back in. Staff could already be reset; clients could not.
func TestSuperAdminCanResetAClientPassword(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir+"/clientreset.db")
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
	sessions := auth.NewSessionStore(st.DB(), false)

	owner, err := svc.Register(context.Background(), auth.RegisterInput{
		Email: "owner@veylix.test", Password: "owner-long-secret-1", DisplayName: "Owner",
	})
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	// The reset path requires the actor to BE a super admin, so promote the row.
	if _, err := st.DB().Exec(`UPDATE users SET role = 'super_admin' WHERE id = ?`, owner.ID); err != nil {
		t.Fatalf("promote: %v", err)
	}
	client, err := svc.Register(context.Background(), auth.RegisterInput{
		Email: "client@veylix.test", Password: "client-long-secret-1", DisplayName: "Client",
	})
	if err != nil {
		t.Fatalf("register client: %v", err)
	}
	// A live session that must not survive the reset.
	if _, err := sessions.Create(context.Background(), client.ID, "127.0.0.1", "test"); err != nil {
		t.Fatalf("session: %v", err)
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

	res, err := http.Post(srv.URL+"/api/super/clients/"+itoa(client.ID)+"/reset-password", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reset = %d, want 200", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	password, _ := body["password"].(string)
	if password == "" {
		t.Fatal("the response must carry the generated password exactly once")
	}

	// The new password really works...
	if _, err := svc.Authenticate(context.Background(), "client@veylix.test", password, "9.9.9.9", auth.PortalClient); err != nil {
		t.Fatalf("the generated password must authenticate: %v", err)
	}
	// ...and the old one no longer does.
	if _, err := svc.Authenticate(context.Background(), "client@veylix.test", "client-long-secret-1", "9.9.9.9", auth.PortalClient); err == nil {
		t.Fatal("the previous password must stop working")
	}
	// Every session for that account was ended.
	var live int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, client.ID).Scan(&live); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if live != 0 {
		t.Fatalf("%d session(s) survived the reset, want 0", live)
	}
}

// A Super Admin must not reset their OWN password through this route: that path
// skips the current-password check, which a borrowed session would exploit.
func TestClientResetRefusesSelf(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(context.Background(), dir+"/selfreset.db")
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
		Email: "self@veylix.test", Password: "owner-long-secret-1", DisplayName: "Owner",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := st.DB().Exec(`UPDATE users SET role = 'super_admin' WHERE id = ?`, owner.ID); err != nil {
		t.Fatalf("promote: %v", err)
	}
	principal := auth.Principal{UserID: owner.ID, Email: "self@veylix.test", Role: auth.RoleSuperAdmin}
	withActor := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
		})
	}
	mux := http.NewServeMux()
	ClientAdminRoutesWithReset(mux, st, auditLog, withActor, svc)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	res, err := http.Post(srv.URL+"/api/super/clients/"+itoa(owner.ID)+"/reset-password", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	// 422 cannot_modify_self is the package's established answer for "your own
	// account" (same as PATCH /api/admins/{id}); 403 would read as "no permission".
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("self reset = %d, want 422 cannot_modify_self", res.StatusCode)
	}
}
