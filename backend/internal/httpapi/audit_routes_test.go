package httpapi_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"veylix/backend/internal/audit"
)

// The audit read is the one surface where a wrong answer is worse than no
// answer: an operator who cannot find what happened must be told THAT, never
// shown an empty page that reads as "nothing happened" (D-082). These tests
// pin the server-side filter contract the audit page is built on.
//
// The bootstrap itself writes rows (auth.bootstrap, and the session it issues),
// so every total assertion COMBINES filters (kind=vpn, actor_id, q) to isolate
// exactly the rows this test seeded — the assertions stay exact without
// depending on how many rows the auth flow happens to write.

// seedAudit writes rows through the real audit logger, the same writer the
// handlers use, so the read tests exercise genuinely stored shapes.
func seedAudit(t *testing.T, env *billingEnv, entries ...audit.Entry) {
	t.Helper()
	al := audit.New(env.st.DB())
	for _, e := range entries {
		if err := al.Record(context.Background(), e); err != nil {
			t.Fatalf("seed audit entry %s: %v", e.Action, err)
		}
	}
}

// TestAuditKindFilterIsServerSide proves the filter runs in SQL: the total
// counts EVERY match, not just the rows inside the limit window — the honesty
// property the client-side filter could not give (it said "nothing recorded"
// when the match sat past its 100-row window).
func TestAuditKindFilterIsServerSide(t *testing.T) {
	env := newBillingEnv(t)
	root := env.root(t)

	seedAudit(t, env,
		audit.Entry{Action: audit.ActionProvision, TargetType: "vpn", TargetID: "11"},
		audit.Entry{Action: audit.ActionLogin, TargetType: "session", TargetID: "1"},
		audit.Entry{Action: audit.ActionPaymentConfirm, TargetType: "payment", TargetID: "3"},
		audit.Entry{Action: audit.ActionPaymentMethodCreate, TargetType: "payment_method", TargetID: "1"},
		audit.Entry{Action: audit.ActionChatLock, TargetType: "conversation", TargetID: "9"},
	)

	// The window (1) is narrower than the family's matches (2): total must
	// still report both, and payment_method.* belongs to the payment family —
	// "payments" is one answer in the operator's head.
	resp, body := getJSON(t, root, env.srv.URL+"/api/audit?kind=payment&limit=1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit kind=payment = %d (%v)", resp.StatusCode, body)
	}
	if body["total"].(float64) != 2 {
		t.Fatalf("payment total = %v, want 2", body["total"])
	}
	entries := body["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want the limit window of 1", len(entries))
	}
	if body["has_more"] != true {
		t.Fatalf("has_more = %v, want true (2 matches, window 1)", body["has_more"])
	}
	action := entries[0].(map[string]any)["action"].(string)
	if action != audit.ActionPaymentConfirm && action != audit.ActionPaymentMethodCreate {
		t.Fatalf("row outside the payment family leaked: %s", action)
	}

	// "other" is the honest remainder: chat.* is in, every nameable family out.
	_, body = getJSON(t, root, env.srv.URL+"/api/audit?kind=other")
	if body["total"].(float64) != 1 {
		t.Fatalf("other total = %v, want 1 (chat.lock)", body["total"])
	}
}

// TestAuditRefusesAnUnknownKind pins the refusal: an unrecognised kind must be
// a 400, never a silent empty page the operator reads as "nothing happened".
func TestAuditRefusesAnUnknownKind(t *testing.T) {
	env := newBillingEnv(t)
	root := env.root(t)

	resp, body := getJSON(t, root, env.srv.URL+"/api/audit?kind=bogus")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("audit kind=bogus = %d, want 400", resp.StatusCode)
	}
	errObj, _ := body["error"].(map[string]any)
	if code, _ := errObj["code"].(string); code != "bad_filter" {
		t.Fatalf("error code = %v, want bad_filter", errObj["code"])
	}
}

// TestAuditActorAndSearchFilters covers the actor filter and the substring
// query, including the two reads the old page could not answer at all:
// "what did THIS person do" and "find me the entry about X".
func TestAuditActorAndSearchFilters(t *testing.T) {
	env := newBillingEnv(t)
	root := env.root(t) // user id 1 — display_name "Owner"

	seedAudit(t, env,
		audit.Entry{ActorID: 1, Action: audit.ActionProvision, TargetType: "vpn", TargetID: "11", Context: map[string]any{"offer": "free"}},
		audit.Entry{Action: audit.ActionChatPin, TargetType: "conversation", TargetID: "4"}, // system row, no actor
	)

	// The actor filter returns only that actor's rows — and a NAME the page can
	// render instead of "user #1".
	resp, body := getJSON(t, root, env.srv.URL+"/api/audit?actor_id=1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit actor_id=1 = %d", resp.StatusCode)
	}
	entries := body["entries"].([]any)
	if len(entries) == 0 {
		t.Fatalf("actor 1 returned no rows — the seeded provision entry is missing")
	}
	for _, e := range entries {
		if e.(map[string]any)["actor_id"].(float64) != 1 {
			t.Fatalf("another actor's row leaked into actor_id=1: %v", e)
		}
	}
	if entries[0].(map[string]any)["actor_name"] != "Owner" {
		t.Fatalf("actor_name = %v, want Owner", entries[0].(map[string]any)["actor_name"])
	}

	// A system row (no actor) still reads — its actor_name is simply absent.
	_, body = getJSON(t, root, env.srv.URL+"/api/audit?q=conversation")
	if body["total"].(float64) != 1 {
		t.Fatalf("q=conversation total = %v, want 1 (the system row)", body["total"])
	}

	// The search also reaches the ACTOR's name, so "what did Owner do about
	// VPNs" is one box, not a cross-reference chore.
	_, body = getJSON(t, root, env.srv.URL+"/api/audit?q=Owner&kind=vpn")
	if body["total"].(float64) != 1 {
		t.Fatalf("q=Owner&kind=vpn total = %v, want 1", body["total"])
	}
}

// TestAuditTimeWindowBounds covers from/to in unix seconds against a row that
// was explicitly backdated, so the bounds are proven rather than assumed.
func TestAuditTimeWindowBounds(t *testing.T) {
	env := newBillingEnv(t)
	root := env.root(t)

	seedAudit(t, env,
		audit.Entry{Action: audit.ActionProvision, TargetType: "vpn", TargetID: "1"},
		audit.Entry{Action: audit.ActionRevoke, TargetType: "vpn", TargetID: "1"},
	)

	yesterday := time.Now().Add(-24 * time.Hour).Unix()
	if _, err := env.st.DB().Exec(
		`UPDATE audit_logs SET created_at = ? WHERE action = ?`, yesterday, audit.ActionProvision,
	); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// from = 12h ago: only the fresh revoke survives the window.
	from := time.Now().Add(-12 * time.Hour).Unix()
	_, body := getJSON(t, root, env.srv.URL+"/api/audit?kind=vpn&from="+strconv.FormatInt(from, 10))
	if body["total"].(float64) != 1 {
		t.Fatalf("kind=vpn&from=12h total = %v, want 1", body["total"])
	}

	// to = 12h ago: only the backdated provision.
	_, body = getJSON(t, root, env.srv.URL+"/api/audit?kind=vpn&to="+strconv.FormatInt(from, 10))
	if body["total"].(float64) != 1 {
		t.Fatalf("kind=vpn&to=12h total = %v, want 1", body["total"])
	}
}

// TestAuditPaginationEchoesItsWindow pins the envelope: limit/offset echoed,
// total counted before the window, has_more true exactly while rows remain.
func TestAuditPaginationEchoesItsWindow(t *testing.T) {
	env := newBillingEnv(t)
	root := env.root(t)

	seedAudit(t, env,
		audit.Entry{Action: audit.ActionProvision, TargetType: "vpn", TargetID: "1"},
		audit.Entry{Action: audit.ActionRevoke, TargetType: "vpn", TargetID: "1"},
		audit.Entry{Action: audit.ActionSuspend, TargetType: "vpn", TargetID: "1"},
	)

	resp, body := getJSON(t, root, env.srv.URL+"/api/audit?kind=vpn&limit=2&offset=2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit page = %d", resp.StatusCode)
	}
	if body["limit"].(float64) != 2 || body["offset"].(float64) != 2 {
		t.Fatalf("echo limit/offset = %v/%v, want 2/2", body["limit"], body["offset"])
	}
	if body["total"].(float64) != 3 {
		t.Fatalf("total = %v, want 3", body["total"])
	}
	if body["has_more"] != false {
		t.Fatalf("has_more = %v, want false at the final page", body["has_more"])
	}
	if entries := body["entries"].([]any); len(entries) != 1 {
		t.Fatalf("entries = %d, want the single last row", len(entries))
	}
}
