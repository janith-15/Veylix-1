package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/integrations"
)

// row builds an AdminVPNRow with the facts a filter cares about.
func row(tag string, assigned bool, usagePct int, daysLeft int) AdminVPNRow {
	r := AdminVPNRow{Assigned: assigned, UsagePct: usagePct, DaysLeft: daysLeft}
	if tag != "" {
		t := tag
		r.Tag = &t
	}
	exp := time.Now().Add(time.Duration(daysLeft) * 24 * time.Hour).Unix()
	r.ExpiresAt = &exp
	if daysLeft < 0 {
		exp = time.Now().Add(time.Duration(daysLeft) * 24 * time.Hour).Unix()
		r.ExpiresAt = &exp
	}
	return r
}

// The console's filters must each narrow correctly, and compose.
//
// This is the test that would catch the two mistakes that matter here: a filter
// that silently matches everything (a typo looks like "no filter"), and one that
// reads a cached column instead of the panel's numbers.
func TestAdminVPNFilterMatches(t *testing.T) {
	free := row("FREE", true, 10, 20)
	premium := row("PREMIUM", true, 100, -3) // exhausted AND expired
	untagged := row("", false, 0, 5)         // unassigned, unused, expiring

	cases := []struct {
		name string
		f    AdminVPNFilter
		r    AdminVPNRow
		want bool
	}{
		{"no filter keeps everything", AdminVPNFilter{}, free, true},
		{"tag free", AdminVPNFilter{Tag: "free"}, free, true},
		{"tag free rejects premium", AdminVPNFilter{Tag: "free"}, premium, false},
		{"tag untagged", AdminVPNFilter{Tag: "untagged"}, untagged, true},
		{"tag untagged rejects free", AdminVPNFilter{Tag: "untagged"}, free, false},
		{"assigned=1", AdminVPNFilter{Assigned: "1"}, free, true},
		{"assigned=1 rejects unassigned", AdminVPNFilter{Assigned: "1"}, untagged, false},
		{"assigned=0 keeps the unassigned", AdminVPNFilter{Assigned: "0"}, untagged, true},
		{"assigned=0 rejects assigned", AdminVPNFilter{Assigned: "0"}, free, false},
		{"usage exhausted", AdminVPNFilter{Usage: "exhausted"}, premium, true},
		{"usage exhausted rejects part-used", AdminVPNFilter{Usage: "exhausted"}, free, false},
		{"usage expired", AdminVPNFilter{Usage: "expired"}, premium, true},
		{"usage expired rejects live", AdminVPNFilter{Usage: "expired"}, free, false},
		{"usage expiring (<=7d)", AdminVPNFilter{Usage: "expiring"}, untagged, true},
		{"usage expiring rejects 20d", AdminVPNFilter{Usage: "expiring"}, free, false},
		{"usage unused", AdminVPNFilter{Usage: "unused"}, untagged, true},
		{"usage unused rejects used", AdminVPNFilter{Usage: "unused"}, free, false},
		{"usage healthy rejects exhausted", AdminVPNFilter{Usage: "healthy"}, premium, false},
		{"expiry 7d", AdminVPNFilter{Expiry: "7d"}, untagged, true},
		{"expiry 7d rejects 20d", AdminVPNFilter{Expiry: "7d"}, free, false},
		{"expiry 30d includes 20d", AdminVPNFilter{Expiry: "30d"}, free, true},
		{"composed: unassigned + expiring", AdminVPNFilter{Assigned: "0", Usage: "expiring"}, untagged, true},
		{"composed rejects when one part fails", AdminVPNFilter{Assigned: "0", Tag: "premium"}, untagged, false},
	}
	for _, c := range cases {
		if got := c.f.matches(c.r); got != c.want {
			t.Errorf("%s: matches = %v, want %v", c.name, got, c.want)
		}
	}
}

// A client the panel no longer holds must never be enriched into a "healthy"
// zero-usage row.
//
// The defect: enrichVPNRow returned early when the panel's answer did not list
// the client, leaving UsagePct 0, Enabled false and ReadError false. The console
// then rendered that as 0% usage / disabled — and selected it under the "Healthy"
// filter, because healthy was defined as "not expired and not exhausted". A VPN
// that had vanished from the panel was indistinguishable from a brand-new,
// untouched one, which is the most dangerous way to be wrong on that screen: it
// is exactly the row an operator needs to act on.
func TestPanelMissingIsNotHealthy(t *testing.T) {
	// The panel answered for server 4 and listed a DIFFERENT client.
	facts := map[int64]map[string]integrations.PanelClientView{
		4: {"someone@else": {PanelEmail: "someone@else", Enable: true}},
	}
	vanish := VPNRow{ID: 7, ServerID: 4, PanelEmail: "gone@example", Status: "ACTIVE", Assigned: true}
	got := enrichVPNRow(vanish, facts, time.Now().Unix())

	if !got.PanelMissing {
		t.Error("PanelMissing = false for a client the panel positively does not hold")
	}
	if got.ReadError {
		t.Error("ReadError = true, but the panel DID answer — a failed read and an absence are different facts")
	}
	if (AdminVPNFilter{Usage: "healthy"}).matches(got) {
		t.Error("a vanished VPN matched the Healthy filter")
	}
	if (AdminVPNFilter{Usage: "unused"}).matches(got) {
		t.Error("a vanished VPN matched the Unused filter — its 0 bytes were never measured")
	}
	if n := countUnread([]AdminVPNRow{got}); n != 1 {
		t.Errorf("countUnread = %d, want 1: the UI must be able to say this row is unverified", n)
	}

	// The contrast that proves the distinction is real: an UNREAD panel keeps
	// ReadError and is never called missing.
	unreadable := map[int64]map[string]integrations.PanelClientView{4: nil}
	unread := enrichVPNRow(vanish, unreadable, time.Now().Unix())
	if !unread.ReadError {
		t.Error("ReadError = false for a panel that could not be read")
	}
	if unread.PanelMissing {
		t.Error("PanelMissing = true for an unread panel — nothing was observed about this client")
	}
}

// The console returned EVERY VPN in one response. With a few hundred panel
// clients that is a multi-megabyte payload rebuilt on every filter change, and
// the page had no pager at all.
//
// The two things that must hold: a page is a window on the SORTED, FILTERED
// list (not on the raw fetch), and the counts describe the whole set so the
// pager can be trusted.
func TestAdminVPNPaginationWindowsTheFilteredSet(t *testing.T) {
	// Usage differs on purpose so the primary sort key is exercised: the order is
	// unassigned-first, THEN the fullest, THEN e-mail.
	rows := []AdminVPNRow{
		{VPNRow: VPNRow{ID: 1, PanelEmail: "c@x", Assigned: true}, UsagePct: 90},
		{VPNRow: VPNRow{ID: 2, PanelEmail: "a@x", Assigned: false}, UsagePct: 10},
		{VPNRow: VPNRow{ID: 3, PanelEmail: "b@x", Assigned: true}, UsagePct: 50},
		{VPNRow: VPNRow{ID: 4, PanelEmail: "d@x", Assigned: false}, UsagePct: 80},
		{VPNRow: VPNRow{ID: 5, PanelEmail: "e@x", Assigned: true}, UsagePct: 20},
	}
	all := append([]AdminVPNRow{}, rows...)
	adminSort(all)

	// The documented order: every unassigned row first (they are the ones needing
	// a decision), fullest first inside each group.
	wantOrder := []string{"d@x", "a@x", "c@x", "b@x", "e@x"}
	for i, want := range wantOrder {
		if all[i].PanelEmail != want {
			t.Fatalf("sorted[%d] = %q, want %q (order: %v)", i, all[i].PanelEmail, want,
				emailsOf(all))
		}
	}

	// Page 1 is the first three of that order — not "the three assigned rows".
	first := paginate(all, 3, 0)
	if len(first) != 3 {
		t.Fatalf("page 1 has %d rows, want 3", len(first))
	}
	for i, want := range wantOrder[:3] {
		if first[i].PanelEmail != want {
			t.Errorf("page 1[%d] = %q, want %q — pagination must window the SORTED list", i, first[i].PanelEmail, want)
		}
	}

	second := paginate(all, 3, 3)
	if len(second) != 2 {
		t.Fatalf("page 2 has %d rows, want the remaining 2", len(second))
	}
	for i, want := range wantOrder[3:] {
		if second[i].PanelEmail != want {
			t.Errorf("page 2[%d] = %q, want %q", i, second[i].PanelEmail, want)
		}
	}

	// The two pages together are the whole set, with no row lost or repeated.
	seen := map[string]int{}
	for _, r := range append(append([]AdminVPNRow{}, first...), second...) {
		seen[r.PanelEmail]++
	}
	for _, r := range rows {
		if seen[r.PanelEmail] != 1 {
			t.Errorf("%q appeared %d times across the two pages, want exactly 1", r.PanelEmail, seen[r.PanelEmail])
		}
	}

	// An offset past the end is an empty page, never a panic or a full slice.
	if got := paginate(all, 3, 99); len(got) != 0 {
		t.Errorf("offset past the end returned %d rows, want 0", len(got))
	}
	// A limit larger than the set returns the set, not an error.
	if got := paginate(all, 500, 0); len(got) != len(all) {
		t.Errorf("oversized limit returned %d rows, want %d", len(got), len(all))
	}

	// PageCount drives the pager, and must never report 0 pages.
	if n := PageCount(5, 3); n != 2 {
		t.Errorf("PageCount(5,3) = %d, want 2", n)
	}
	if n := PageCount(0, 3); n != 1 {
		t.Errorf("PageCount(0,3) = %d, want 1 (a pager with zero pages has nothing to draw)", n)
	}
}

// emailsOf is a readable failure message for an ordering assertion.
func emailsOf(rows []AdminVPNRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.PanelEmail)
	}
	return out
}

// A bad window is refused, never silently clamped: a pager told "500" when it
// asked for 10 000 would show a page count that does not match reality.
func TestAdminVPNPaginationValuesAreValidated(t *testing.T) {
	cases := []struct{ query, want string }{
		{"?limit=0", "limit must be a whole number between 1 and 500"},
		{"?limit=501", "limit must be a whole number between 1 and 500"},
		{"?limit=abc", "limit must be a whole number between 1 and 500"},
		{"?offset=-1", "offset must be zero or a positive whole number"},
		{"?offset=xyz", "offset must be zero or a positive whole number"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/super/vpns"+c.query, nil)
		if _, ok := parseAdminVPNFilter(rec, req); ok {
			t.Errorf("%s was accepted, want a 422", c.query)
			continue
		}
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d, want 422", c.query, rec.Code)
		}
	}

	// The defaults are concrete, so the handler never has to guess.
	rec := httptest.NewRecorder()
	f, ok := parseAdminVPNFilter(rec, httptest.NewRequest(http.MethodGet, "/api/super/vpns", nil))
	if !ok {
		t.Fatal("a bare request was refused")
	}
	if f.Limit != AdminVPNDefaultLimit || f.Offset != 0 {
		t.Errorf("defaults = (limit %d, offset %d), want (%d, 0)", f.Limit, f.Offset, AdminVPNDefaultLimit)
	}
}

// stubLister serves a fixed VPN list to the console handler.
type stubLister struct{ rows []VPNRow }

func (s stubLister) ListVPNs(context.Context, auth.Principal, string) ([]VPNRow, error) {
	return s.rows, nil
}

// stubReader answers panel facts per server; an error marks that server unread.
type stubReader struct {
	views map[int64][]integrations.PanelClientView
	errs  map[int64]error
}

func (s stubReader) PanelClients(_ context.Context, id int64) ([]integrations.PanelClientView, error) {
	if err, ok := s.errs[id]; ok {
		return nil, err
	}
	return s.views[id], nil
}

// The HANDLER must window the response, not just the paginate() helper.
//
// The first version of this test called paginate() directly and therefore passed
// even when the handler was changed to return every row — it verified the helper
// and said nothing about the endpoint. This one drives the real handler and reads
// the JSON body, which is the only thing a pager can actually see.
func TestAdminVPNHandlerPaginatesResponseBody(t *testing.T) {
	rows := make([]VPNRow, 0, 7)
	for i := 1; i <= 7; i++ {
		rows = append(rows, VPNRow{ID: int64(i), ServerID: 1, PanelEmail: fmt.Sprintf("v%d@x", i), Assigned: true})
	}
	lister := stubLister{rows: rows}
	reader := stubReader{views: map[int64][]integrations.PanelClientView{}}
	principal := auth.Principal{UserID: 1, Role: auth.RoleSuperAdmin}

	serve := func(query string) (code int, body map[string]any) {
		req := httptest.NewRequest(http.MethodGet, "/api/super/vpns"+query, nil)
		req = req.WithContext(WithPrincipal(req.Context(), principal))
		rec := httptest.NewRecorder()
		adminVPNHandler(rec, req, lister, reader)
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s: %v (%s)", query, err, rec.Body.String())
		}
		return rec.Code, body
	}

	code, page1 := serve("?limit=3&offset=0")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	vpns, _ := page1["vpns"].([]any)
	if len(vpns) != 3 {
		t.Fatalf("page 1 returned %d rows, want 3 — the handler is not paginating", len(vpns))
	}
	if total, _ := page1["total"].(float64); total != 7 {
		t.Errorf("total = %v, want 7 (the WHOLE set, not the page)", page1["total"])
	}
	if more, _ := page1["has_more"].(bool); !more {
		t.Error("has_more = false on page 1 of 3")
	}

	_, page3 := serve("?limit=3&offset=6")
	vpns3, _ := page3["vpns"].([]any)
	if len(vpns3) != 1 {
		t.Fatalf("page 3 returned %d rows, want the last 1", len(vpns3))
	}
	if more, _ := page3["has_more"].(bool); more {
		t.Error("has_more = true on the final page")
	}

	// An offset beyond the end is an empty page with the real total, not an error.
	code, past := serve("?limit=3&offset=99")
	if code != http.StatusOK {
		t.Errorf("offset past the end answered %d, want 200", code)
	}
	if got, _ := past["vpns"].([]any); len(got) != 0 {
		t.Errorf("offset past the end returned %d rows, want 0", len(got))
	}

	// A bad window is a 422 with a code, never a silently clamped page.
	code, _ = serve("?limit=9999")
	if code != http.StatusUnprocessableEntity {
		t.Errorf("limit=9999 answered %d, want 422", code)
	}
}

// A bad filter value is refused with a precise code, never silently ignored: a
// typo must not look like "no results".
func TestAdminVPNFilterRefusesUnknownValues(t *testing.T) {
	cases := []struct{ query, code string }{
		{"?tag=gold", "validation_failed"},
		{"?assigned=yes", "validation_failed"},
		{"?usage=probably", "validation_failed"},
		{"?expiry=1y", "validation_failed"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/super/vpns"+c.query, nil)
		if _, ok := parseAdminVPNFilter(rec, req); ok {
			t.Errorf("%s was accepted, want a refusal", c.query)
			continue
		}
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422", c.query, rec.Code)
		}
	}

	// Valid values pass, and the ids are parsed.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/super/vpns?tag=premium&assigned=0&server_id=7&owner_id=9&seller_id=2&usage=expiring&expiry=30d", nil)
	f, ok := parseAdminVPNFilter(rec, req)
	if !ok {
		t.Fatalf("a valid filter was refused: %d", rec.Code)
	}
	if f.Tag != "premium" || f.Assigned != "0" || f.ServerID != 7 || f.OwnerID != 9 || f.SellerID != 2 {
		t.Fatalf("parsed filter = %+v", f)
	}
}

// Unassigned VPNs sort first: they are the ones needing a decision.
func TestAdminSortPutsUnassignedFirst(t *testing.T) {
	rows := []AdminVPNRow{
		row("FREE", true, 90, 5),
		row("", false, 0, 5),
		row("PREMIUM", true, 10, 5),
		row("", false, 50, 5),
	}
	adminSort(rows)
	if rows[0].Assigned || rows[1].Assigned {
		t.Fatalf("the two unassigned rows must come first, got assigned=%v,%v", rows[0].Assigned, rows[1].Assigned)
	}
	// Within the assigned ones, the fullest comes first.
	if !rows[2].Assigned || rows[2].UsagePct != 90 {
		t.Fatalf("assigned rows must be ordered by usage, got %+v", rows[2])
	}
}
