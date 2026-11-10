package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/httpapi"
	"veylix/backend/internal/store"
)

// TestAuditFansOutIntoTheInbox proves the notification PRODUCER exists.
//
// A real inbox with no producer is a screen that is always empty — the last
// untruth left in the UI. The audit logger now fans privileged actions into the
// affected account's inbox, so this asserts delivery, not an endpoint shape.
func TestAuditFansOutIntoTheInbox(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir()+"/inbox.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Exactly the production wiring: the logger notifies through the store.
	auditLog := audit.New(st.DB()).WithNotifier(st)
	authSvc := auth.NewService(st.DB(), auditLog)
	// Self-registered accounts are verified in tests (see newHarness).
	authSvc.SetAutoVerify(true)
	sessions := auth.NewSessionStore(st.DB(), false)

	srv := httptest.NewServer(httpapi.Router(httpapi.Deps{
		Health: st, Version: "test",
		Auth: authSvc, Sessions: sessions, Authn: sessions,
		UserAdmin: authSvc, SetupState: authSvc,
		AdminAudit:    auditLog,
		Notifications: st,
		Audit:         st,
		Quota:         st,
		Payments:      st,
		Offers:        st,
		OfferAudit:    auditLog,
	}))
	t.Cleanup(srv.Close)

	root := bootstrapSuperAdmin(t, srv.URL)

	// Something successful that the actor should have a receipt for.
	resp, body := postJSON(t, root, srv.URL+"/api/payment-methods", map[string]string{
		"kind": "BANK", "label": "Inbox Bank",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create payment method = %d (%v)", resp.StatusCode, body)
	}

	// A second producer path: updating the destination is audited separately, so
	// a second, different action must also produce its own entry — that is what
	// proves the fan-out is generic rather than special-cased.
	methodID := int64(body["id"].(float64))
	resp, _ = patchJSON(t, root, srv.URL+"/api/payment-methods/"+usageID(methodID), map[string]any{
		"active": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("deactivate payment method = %d", resp.StatusCode)
	}

	inbox := getJSONArray(t, root, srv.URL+"/api/notifications")
	if len(inbox) == 0 {
		t.Fatalf("the super admin's inbox is empty: the producer never fired")
	}
	titles := make(map[string]bool, len(inbox))
	for _, n := range inbox {
		titles[n["title"].(string)] = true
	}
	if !titles["Payment method added"] {
		t.Fatalf("no receipt for the successful action; got %v", titles)
	}
	if !titles["Payment method updated"] {
		t.Fatalf("the fan-out is not generic: a second audited action produced no entry; got %v", titles)
	}
}
