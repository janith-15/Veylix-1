package httpapi_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/store"
)

// chatEnv is a real router over real SQLite with the chat store wired exactly
// as cmd/veylixd wires it.
type chatEnv struct {
	srv *httptest.Server
	st  *store.Store
}

func newChatEnv(t *testing.T) *chatEnv {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/chat.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	auditLog := audit.New(st.DB())
	authSvc := auth.NewService(st.DB(), auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(st.DB(), false)

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		AdminAudit: auditLog,
		Directory:  st,
		Chat:       st,
		Audit:      st,
		Quota:      st,
	}))
	t.Cleanup(srv.Close)
	return &chatEnv{srv: srv, st: st}
}

func (e *chatEnv) root(t *testing.T) *http.Client {
	t.Helper()
	return bootstrapSuperAdmin(t, e.srv.URL)
}

// signup registers a client account through the real API and returns its
// signed-in client plus its numeric id (resolved via the directory).
func (e *chatEnv) signup(t *testing.T, email string) (*http.Client, int64) {
	t.Helper()
	c := &http.Client{Jar: mustJar(t), Timeout: 10 * time.Second}
	resp, body := postJSON(t, c, e.srv.URL+"/api/auth/register", map[string]string{
		"email": email, "password": "a very long password",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register %s = %d (%v)", email, resp.StatusCode, body)
	}
	return c, int64(body["id"].(float64))
}

// makeAdmin creates a reseller through the API.
func (e *chatEnv) makeAdmin(t *testing.T, root *http.Client, email string) (*http.Client, int64) {
	t.Helper()
	resp, body := postJSON(t, root, e.srv.URL+"/api/admins", map[string]any{
		"email": email, "password": "a very long password",
		"display_name": "Reseller", "role": "admin", "quota_limit": 10,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create admin %s = %d (%v)", email, resp.StatusCode, body)
	}
	return loginAs(t, e.srv.URL, email, "a very long password"), int64(body["id"].(float64))
}

// sell links a client to a seller, which is what makes them addressable.
func (e *chatEnv) sell(t *testing.T, sellerID, clientID, vpnID int64) {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := e.st.DB().Exec(q, args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	exec(`INSERT INTO server_integrations (id, name, panel_url, token_ciphertext, status, created_at, updated_at)
	      VALUES (1, 'chat-srv', 'https://panel.example/base', x'00', 'ACTIVE', 1, 1)`)
	exec(`INSERT INTO vpn_instances (id, server_id, owner_user_id, panel_email, panel_sub_id, status, created_at, updated_at)
	      VALUES (?, 1, ?, ?, ?, 'ACTIVE', 1, 1)`, vpnID, clientID, "chat-"+usageID(vpnID), "sub-"+usageID(vpnID))
	exec(`INSERT INTO vpn_assignments (vpn_instance_id, seller_user_id, client_user_id, status, created_at, updated_at)
	      VALUES (?, ?, ?, 'ACTIVE', 1, 1)`, vpnID, sellerID, clientID)
}

// deleteReq issues a DELETE, which the shared helpers do not cover.
func deleteReq(t *testing.T, c *http.Client, url string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("build DELETE: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode
}

// TestChatAccessIsParticipantOnly is the T-16 regression test.
//
// A conversation is readable by its two participants and by nobody else — not
// another client, not another reseller, and not even a super admin whose
// "everyone" scope is about the DIRECTORY, not about reading private
// transcripts. Message ids answer 404 rather than 403 so they cannot be probed.
func TestChatAccessIsParticipantOnly(t *testing.T) {
	env := newChatEnv(t)
	root := env.root(t)

	sellerA, sellerAID := env.makeAdmin(t, root, "seller-a@example.com")
	env.makeAdmin(t, root, "seller-b@example.com")
	clientA, clientAID := env.signup(t, "client-a@example.com")
	clientC, _ := env.signup(t, "client-c@example.com")
	env.sell(t, sellerAID, clientAID, 9001)

	// The client opens a conversation with their own seller.
	resp, thread := postJSON(t, clientA, env.srv.URL+"/api/chat/threads", map[string]any{
		"peer_user_id": sellerAID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("open thread = %d (%v), want 201", resp.StatusCode, thread)
	}
	threadID := int64(thread["id"].(float64))
	threadURL := env.srv.URL + "/api/chat/threads/" + usageID(threadID)

	// Idempotent: opening it again is the same conversation, not a second one.
	resp, again := postJSON(t, sellerA, env.srv.URL+"/api/chat/threads", map[string]any{
		"peer_user_id": clientAID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("reopen from the other side = %d (%v)", resp.StatusCode, again)
	}
	if int64(again["id"].(float64)) != threadID {
		t.Fatalf("reopen created a second thread: %v vs %d", again["id"], threadID)
	}

	// Both participants can write.
	resp, msg := postJSON(t, clientA, threadURL+"/messages", map[string]string{"body": "Hello, my VPN drops"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("client send = %d (%v), want 201", resp.StatusCode, msg)
	}
	msgID := int64(msg["id"].(float64))
	resp, _ = postJSON(t, sellerA, threadURL+"/messages", map[string]string{"body": "Looking into it"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("seller send = %d, want 201", resp.StatusCode)
	}
	var body map[string]any

	// An unrelated client is refused reading the thread (403, it exists) and
	// its messages.
	resp, _ = getJSON(t, clientC, threadURL)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unrelated client reads thread = %d, want 403", resp.StatusCode)
	}
	resp, _ = getJSON(t, clientC, threadURL+"/messages")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unrelated client reads messages = %d, want 403", resp.StatusCode)
	}
	// Even the super admin cannot read a private transcript they are not in.
	resp, _ = getJSON(t, root, threadURL+"/messages")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("super admin reads a private transcript = %d, want 403", resp.StatusCode)
	}

	// An unrelated reseller cannot message that client at all: the relationship
	// does not exist, so the thread is never even creatable.
	resp, body = postJSON(t, loginAs(t, env.srv.URL, "seller-b@example.com", "a very long password"),
		env.srv.URL+"/api/chat/threads", map[string]any{"peer_user_id": clientAID})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unrelated reseller opens a thread = %d (%v), want 403", resp.StatusCode, body)
	}

	// Message-level actions answer 404 for a non-participant: message ids are
	// sequential and must not become an enumeration oracle.
	resp, body = postJSON(t, clientC, env.srv.URL+"/api/chat/messages/"+usageID(msgID)+"/save", map[string]any{
		"thread_id": threadID, "on": true,
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unrelated client flags a message = %d (%v), want 404", resp.StatusCode, body)
	}

	// The threads list only ever contains the caller's own conversations.
	for _, c := range []*http.Client{clientA, sellerA} {
		rows := getJSONArray(t, c, env.srv.URL+"/api/chat/threads")
		if len(rows) != 1 {
			t.Fatalf("participant sees %d threads, want 1", len(rows))
		}
	}
	if rows := getJSONArray(t, clientC, env.srv.URL+"/api/chat/threads"); len(rows) != 0 {
		t.Fatalf("unrelated client sees %d threads, want 0", len(rows))
	}
}

// TestChatMessageLifecycle covers the behaviours that used to live only in
// localStorage: edit-with-history, hide-for-me-only, pin, lock and read marking.
//
// Each is asserted from BOTH sides, because every one of them is about "what
// does the other person see" — the exact thing a browser-local chat got wrong.
func TestChatMessageLifecycle(t *testing.T) {
	env := newChatEnv(t)
	root := env.root(t)

	seller, sellerID := env.makeAdmin(t, root, "seller@example.com")
	client, clientID := env.signup(t, "client@example.com")
	env.sell(t, sellerID, clientID, 9101)

	resp, thread := postJSON(t, client, env.srv.URL+"/api/chat/threads", map[string]any{"peer_user_id": sellerID})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("open thread = %d (%v)", resp.StatusCode, thread)
	}
	threadID := int64(thread["id"].(float64))
	base := env.srv.URL + "/api/chat/threads/" + usageID(threadID)

	// --- send + unread ------------------------------------------------------
	resp, m1 := postJSON(t, client, base+"/messages", map[string]string{"body": "first"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("send = %d (%v)", resp.StatusCode, m1)
	}
	msg1 := int64(m1["id"].(float64))

	// The seller has not opened it yet, so their badge shows 1.
	_, seen := getJSON(t, seller, base)
	if seen["unread"].(float64) != 1 {
		t.Fatalf("seller unread = %v, want 1", seen["unread"])
	}
	if resp, body := postJSON(t, seller, base+"/read", map[string]any{}); resp.StatusCode != http.StatusOK {
		t.Fatalf("mark read = %d (%v)", resp.StatusCode, body)
	}
	_, seen = getJSON(t, seller, base)
	if seen["unread"].(float64) != 0 {
		t.Fatalf("seller unread after read = %v, want 0", seen["unread"])
	}

	// --- edit keeps the original -------------------------------------------
	resp, edited := patchJSON(t, client, env.srv.URL+"/api/chat/messages/"+usageID(msg1),
		map[string]any{"body": "first (corrected)", "thread_id": threadID})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("edit own message = %d (%v)", resp.StatusCode, edited)
	}
	if edited["edited_at"] == nil {
		t.Fatalf("edit did not stamp edited_at: %v", edited)
	}
	// The seller may not edit somebody else's message.
	resp, _ = patchJSON(t, seller, env.srv.URL+"/api/chat/messages/"+usageID(msg1),
		map[string]any{"body": "hijacked", "thread_id": threadID})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("edit peer's message = %d, want 404", resp.StatusCode)
	}

	// --- pin / lock are staff-only -----------------------------------------
	resp, _ = postJSON(t, client, base+"/pin", map[string]any{"message_id": msg1})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("client pins = %d, want 403", resp.StatusCode)
	}
	if resp, pinned := postJSON(t, seller, base+"/pin", map[string]any{"message_id": msg1}); resp.StatusCode != http.StatusOK {
		t.Fatalf("staff pins = %d (%v)", resp.StatusCode, pinned)
	}
	_, seen = getJSON(t, client, base)
	if seen["pinned_message_id"] == nil || int64(seen["pinned_message_id"].(float64)) != msg1 {
		t.Fatalf("pin not visible to the client: %v", seen["pinned_message_id"])
	}

	resp, _ = postJSON(t, client, base+"/lock", map[string]any{"locked": true})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("client locks = %d, want 403", resp.StatusCode)
	}
	if resp, locked := postJSON(t, seller, base+"/lock", map[string]any{"locked": true}); resp.StatusCode != http.StatusOK {
		t.Fatalf("staff locks = %d (%v)", resp.StatusCode, locked)
	}
	// A locked thread refuses new messages from BOTH sides...
	if resp, body := postJSON(t, client, base+"/messages", map[string]string{"body": "still there?"}); resp.StatusCode != http.StatusConflict {
		t.Fatalf("send into locked thread = %d (%v), want 409", resp.StatusCode, body)
	}
	// ...but stays readable, because the transcript is evidence.
	if resp, _ := getJSON(t, client, base+"/messages"); resp.StatusCode != http.StatusOK {
		t.Fatalf("read locked thread = %d, want 200", resp.StatusCode)
	}
	// Unlocking restores writing.
	if resp, _ := postJSON(t, seller, base+"/lock", map[string]any{"locked": false}); resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock = %d", resp.StatusCode)
	}
	if resp, _ := postJSON(t, client, base+"/messages", map[string]string{"body": "back"}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("send after unlock = %d, want 201", resp.StatusCode)
	}

	// --- hide is per-viewer ------------------------------------------------
	if code := deleteReq(t, client, env.srv.URL+"/api/chat/messages/"+usageID(msg1)+
		"?thread_id="+usageID(threadID)); code != http.StatusNoContent {
		t.Fatalf("hide = %d, want 204", code)
	}
	for _, m := range getJSONArray(t, client, base+"/messages") {
		if int64(m["id"].(float64)) == msg1 {
			t.Fatalf("hidden message is still visible to the person who hid it")
		}
	}
	var sellerSees bool
	for _, m := range getJSONArray(t, seller, base+"/messages") {
		if int64(m["id"].(float64)) == msg1 {
			sellerSees = true
		}
	}
	if !sellerSees {
		t.Fatalf("hiding removed the message for the OTHER participant too")
	}
	// Hiding a pinned message also clears the pin for that viewer.
	_, seen = getJSON(t, client, base)
	if seen["pinned_message_id"] != nil {
		t.Fatalf("pin survived the hide: %v", seen["pinned_message_id"])
	}

	// --- unhide (undo) puts it back for the caller only --------------------
	//
	// A hide is a per-viewer visibility flag, not a destruction, so it must be
	// reversible: one mis-click next to Copy used to cost the reader the message
	// for good, because every read filtered the hidden row out and nothing could
	// remove that row again.
	hiddenRows := getJSONArray(t, client, base+"/hidden")
	if len(hiddenRows) != 1 || int64(hiddenRows[0]["id"].(float64)) != msg1 {
		t.Fatalf("/hidden = %v, want exactly message %d", hiddenRows, msg1)
	}
	if resp, _ := postJSON(t, client, env.srv.URL+"/api/chat/messages/"+usageID(msg1)+"/unhide",
		map[string]any{"thread_id": threadID}); resp.StatusCode != http.StatusOK {
		t.Fatalf("unhide = %d, want 200", resp.StatusCode)
	}
	var restored bool
	for _, m := range getJSONArray(t, client, base+"/messages") {
		if int64(m["id"].(float64)) == msg1 {
			restored = true
		}
	}
	if !restored {
		t.Fatalf("unhide did not bring the message back for the caller")
	}
	if n := len(getJSONArray(t, client, base+"/hidden")); n != 0 {
		t.Fatalf("/hidden still lists %d message(s) after restoring the only one", n)
	}
	// Restoring is the caller's business only: the peer never lost it, and the
	// caller's own pin is NOT silently reinstated (that would be a second,
	// hidden side effect of an undo).
	if _, seen = getJSON(t, client, base); seen["pinned_message_id"] != nil {
		t.Fatalf("unhide re-pinned the message: %v", seen["pinned_message_id"])
	}
	// Idempotent: a second undo of something already visible is a no-op, not an
	// error, because a retried request must not fail a user who is already right.
	if resp, _ := postJSON(t, client, env.srv.URL+"/api/chat/messages/"+usageID(msg1)+"/unhide",
		map[string]any{"thread_id": threadID}); resp.StatusCode != http.StatusOK {
		t.Fatalf("second unhide = %d, want 200 (idempotent)", resp.StatusCode)
	}

	// --- a non-participant can neither list nor restore ---------------------
	//
	// The hidden LIST is the restore path's index. If it answered for a
	// non-participant it would expose the existence of another conversation's
	// messages, which is exactly what the 403/404 split in writeChatError exists
	// to prevent.
	outsider, _ := env.signup(t, "chat-outsider@example.test")
	resp, err := outsider.Get(env.srv.URL + "/api/chat/threads/" + usageID(threadID) + "/hidden")
	if err != nil {
		t.Fatalf("outsider /hidden: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outsider /hidden = %d, want 403", resp.StatusCode)
	}
	if resp, _ := postJSON(t, outsider, env.srv.URL+"/api/chat/messages/"+usageID(msg1)+"/unhide",
		map[string]any{"thread_id": threadID}); resp.StatusCode == http.StatusOK {
		t.Fatalf("outsider unhide = 200, want a refusal")
	}

	// --- per-viewer bookmark ------------------------------------------------
	if resp, _ := postJSON(t, client, env.srv.URL+"/api/chat/messages/"+usageID(msg1)+"/save",
		map[string]any{"thread_id": threadID, "on": true}); resp.StatusCode != http.StatusOK {
		t.Fatalf("save = %d", resp.StatusCode)
	}
	for _, m := range getJSONArray(t, seller, base+"/messages") {
		if int64(m["id"].(float64)) == msg1 && m["saved"] == true {
			t.Fatalf("one person's bookmark leaked to the other")
		}
	}
}
