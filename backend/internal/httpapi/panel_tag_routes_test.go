package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/integrations"
	"veylix/backend/internal/panel"
	"veylix/backend/internal/store"
)

// tagStubService satisfies the integration surface the route group requires.
// The tagging route never calls it: tagging is a local write, so none of these
// methods may be reached. They panic if a future change starts calling the
// panel from the tagging path.
type tagStubService struct{}

func (tagStubService) Add(context.Context, integrations.AddInput) (*integrations.View, error) {
	panic("tagging must not reach the panel")
}
func (tagStubService) Update(context.Context, int64, integrations.UpdateInput) (*integrations.View, error) {
	panic("tagging must not reach the panel")
}
func (tagStubService) Test(context.Context, int64, int64) (*integrations.View, error) {
	panic("tagging must not reach the panel")
}
func (tagStubService) Inbounds(context.Context, int64) ([]panel.InboundOption, error) {
	panic("tagging must not reach the panel")
}
func (tagStubService) SetDisabled(context.Context, int64, bool, int64) error {
	panic("tagging must not reach the panel")
}
func (tagStubService) Remove(context.Context, int64, int64) error {
	panic("tagging must not reach the panel")
}
func (tagStubService) Purge(context.Context, int64, int64) error {
	panic("tagging must not reach the panel")
}

type tagEnv struct {
	url      string
	st       *store.Store
	serverID int64
}

// newTagEnv builds the real router over real SQLite, with the real store as the
// tag backend and a real audit logger — so a test asserts what is actually
// persisted, not what a double remembers.
func newTagEnv(t *testing.T) *tagEnv {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/tags.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	res, err := st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('srv', 'https://panel.example/base', x'00', 'ACTIVE', 1, 1)`)
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
	serverID, _ := res.LastInsertId()

	auditLog := audit.New(st.DB())
	authSvc := auth.NewService(st.DB(), auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(st.DB(), false)

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		Integrations: tagStubService{}, Servers: st,
		PanelTags: st, PanelTagAudit: auditLog,
		UserAdmin: authSvc, SetupState: authSvc,
	}))
	t.Cleanup(srv.Close)

	return &tagEnv{url: srv.URL, st: st, serverID: serverID}
}

// putTag sends PUT /api/panel-clients/{server}/{email}/tag.
func (e *tagEnv) putTag(t *testing.T, c *http.Client, serverID int64, email string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	path := e.url + "/api/panel-clients/" + strconv.FormatInt(serverID, 10) + "/" + email + "/tag"
	req, err := http.NewRequest(http.MethodPut, path, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build PUT: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// A Super Admin tags an untagged panel client FREE; the tag is persisted and
// the write is audited with the transition.
func TestSuperAdminTagsPanelClient(t *testing.T) {
	env := newTagEnv(t)
	root := bootstrapSuperAdmin(t, env.url)

	code, body := env.putTag(t, root, env.serverID, "DNuser", map[string]any{"tag": "FREE"})
	if code != http.StatusOK {
		t.Fatalf("PUT tag = %d (%v), want 200", code, body)
	}
	if body["tagged"] != true || body["tag"] != "FREE" {
		t.Fatalf("body = %v, want {tagged:true, tag:FREE}", body)
	}

	// Persisted, read back through the store — not through the response.
	got, err := env.st.ClientTag(context.Background(), env.serverID, "DNuser")
	if err != nil {
		t.Fatalf("ClientTag: %v", err)
	}
	if got.Tag != store.TagFree {
		t.Fatalf("stored tag = %q, want FREE", got.Tag)
	}
	if got.LockedAt != 0 {
		t.Errorf("an unissued client must not be locked (locked_at=%d)", got.LockedAt)
	}

	// Audited, with the previous value so the transition is reconstructable.
	var n int
	var ctxJSON string
	if err := env.st.DB().QueryRow(
		`SELECT COUNT(*), COALESCE(MAX(context),'') FROM audit_logs
		  WHERE action = ? AND target_id = ? AND result = 'OK'`,
		audit.ActionPanelClientTag, "DNuser").Scan(&n, &ctxJSON); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if n != 1 {
		t.Fatalf("audit rows for panel_client.tag = %d, want 1", n)
	}
	if !bytes.Contains([]byte(ctxJSON), []byte("untagged")) {
		t.Errorf("audit context = %s, want the previous value recorded", ctxJSON)
	}
}

// Re-tagging is allowed while the client is unissued (D-025.2).
func TestRetaggingIsAllowedWhileUnissued(t *testing.T) {
	env := newTagEnv(t)
	root := bootstrapSuperAdmin(t, env.url)
	ctx := context.Background()

	if code, b := env.putTag(t, root, env.serverID, "yasa", map[string]any{"tag": "FREE"}); code != http.StatusOK {
		t.Fatalf("first tag = %d (%v)", code, b)
	}
	// Lowercase is accepted from a UI and stored canonically.
	if code, b := env.putTag(t, root, env.serverID, "yasa", map[string]any{"tag": "premium"}); code != http.StatusOK {
		t.Fatalf("re-tag = %d (%v), want 200", code, b)
	}
	got, err := env.st.ClientTag(ctx, env.serverID, "yasa")
	if err != nil {
		t.Fatalf("ClientTag: %v", err)
	}
	if got.Tag != store.TagPremium {
		t.Fatalf("stored tag = %q, want PREMIUM", got.Tag)
	}
}

// Once the VPN is issued the tag is locked: 409, and the refusal is audited as
// DENIED rather than silently ignored.
func TestTagIsRefusedOnceIssued(t *testing.T) {
	env := newTagEnv(t)
	root := bootstrapSuperAdmin(t, env.url)
	ctx := context.Background()

	if code, b := env.putTag(t, root, env.serverID, "athoof", map[string]any{"tag": "FREE"}); code != http.StatusOK {
		t.Fatalf("initial tag = %d (%v)", code, b)
	}
	if err := env.st.LockClientTag(ctx, env.serverID, "athoof"); err != nil {
		t.Fatalf("lock: %v", err)
	}

	code, body := env.putTag(t, root, env.serverID, "athoof", map[string]any{"tag": "PREMIUM"})
	if code != http.StatusConflict {
		t.Fatalf("re-tag after issue = %d (%v), want 409", code, body)
	}
	errBody, _ := body["error"].(map[string]any)
	if errBody == nil || errBody["code"] != "tag_locked" {
		t.Fatalf("error = %v, want code tag_locked", body)
	}

	// The stored tag is untouched — a refused write must change nothing.
	got, err := env.st.ClientTag(ctx, env.serverID, "athoof")
	if err != nil {
		t.Fatalf("ClientTag: %v", err)
	}
	if got.Tag != store.TagFree {
		t.Fatalf("stored tag = %q after refusal, want FREE (unchanged)", got.Tag)
	}

	var denied int
	if err := env.st.DB().QueryRow(
		`SELECT COUNT(*) FROM audit_logs WHERE action = ? AND result = 'DENIED'`,
		audit.ActionPanelClientTagDenied).Scan(&denied); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if denied != 1 {
		t.Fatalf("DENIED audit rows = %d, want 1", denied)
	}
}

// An unknown tag is a validation error, not a silent success.
func TestTagRejectsUnknownValue(t *testing.T) {
	env := newTagEnv(t)
	root := bootstrapSuperAdmin(t, env.url)

	code, body := env.putTag(t, root, env.serverID, "DNuser", map[string]any{"tag": "GOLD"})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("tag GOLD = %d (%v), want 422", code, body)
	}
	if _, err := env.st.ClientTag(context.Background(), env.serverID, "DNuser"); !errors.Is(err, store.ErrNoRows) {
		t.Fatalf("a rejected tag must not be stored, got %v", err)
	}
}

// Only a Super Admin may classify: a reseller manages servers but must not
// re-price what the platform sells.
func TestTaggingRequiresSuperAdmin(t *testing.T) {
	env := newTagEnv(t)
	root := bootstrapSuperAdmin(t, env.url)

	resp, created := postJSON(t, root, env.url+"/api/admins", map[string]any{
		"email": "reseller@example.com", "password": "a very long password",
		"display_name": "Reseller", "role": "admin",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create admin = %d (%v), want 201", resp.StatusCode, created)
	}
	admin := loginAs(t, env.url, "reseller@example.com", "a very long password")

	code, body := env.putTag(t, admin, env.serverID, "DNuser", map[string]any{"tag": "FREE"})
	if code != http.StatusForbidden {
		t.Fatalf("admin tagging = %d (%v), want 403", code, body)
	}
	if _, err := env.st.ClientTag(context.Background(), env.serverID, "DNuser"); !errors.Is(err, store.ErrNoRows) {
		t.Fatalf("a forbidden write must store nothing, got %v", err)
	}

	// Anonymous access is refused too.
	anon := &http.Client{Jar: mustJar(t), Timeout: 5 * time.Second}
	if code, _ := env.putTag(t, anon, env.serverID, "DNuser", map[string]any{"tag": "FREE"}); code != http.StatusUnauthorized {
		t.Fatalf("anonymous tagging = %d, want 401", code)
	}
}
