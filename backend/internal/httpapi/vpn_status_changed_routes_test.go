package httpapi_test

import (
	"net/http"
	"testing"

	"veylix/backend/internal/integrations"
)

// D-100 — GET /api/vpns must publish WHEN a row entered its current status.
//
// The My VPNs screen puts suspended and revoked VPNs in a separate section and
// labels each one "since …". That label has one source: this field. Without it
// the screen can only say "SUSPENDED", which leaves the customer unable to tell
// whether it happened this morning or last quarter — the exact gap a support
// call fills.
//
// Two halves are asserted, because either alone would let a wrong answer ship:
//
//  1. a row that HAS a status-change clock publishes it, unchanged, as seconds;
//  2. a row that has NEVER moved publishes null — NOT 0 and NOT a zero date.
//     `0` is a real number, and a screen that rendered it would print
//     "1 January 1970", which is a fabricated fact about the customer's VPN.
func TestVpnListPublishesStatusChangedAt(t *testing.T) {
	env := newListEnv(t, &fakeInspector{clients: map[int64][]integrations.PanelClientView{}})
	root := bootstrapSuperAdmin(t, env.url)

	client := env.signupClient(t, "since@example.com")
	ownerID := env.userID(t, "since@example.com")

	// A suspended VPN whose status moved at a known instant.
	suspended := env.seedVPN(t, "nimal@gmail.com_paid_1", ownerID)
	const changedAt int64 = 1_760_000_000
	if _, err := env.st.DB().Exec(
		`UPDATE vpn_instances SET status = 'SUSPENDED', status_changed_at = ? WHERE id = ?`,
		changedAt, suspended); err != nil {
		t.Fatalf("mark suspended: %v", err)
	}

	// A second VPN that has never changed status, so the column keeps its
	// default. This is the null case, and it must stay null on the wire.
	never := env.seedVPN(t, "nimal@gmail.com_paid_2", ownerID)

	// The panel must report both clients, or the panel view would answer for
	// the list and the local mirror would never be read.
	env.setPanelClients(t, []integrations.PanelClientView{
		{PanelEmail: "nimal@gmail.com_paid_1", Enable: false, Tag: ptr("PREMIUM")},
		{PanelEmail: "nimal@gmail.com_paid_2", Enable: true, Tag: ptr("PREMIUM")},
	})
	_ = root

	code, list := env.listVPNs(t, client, "")
	if code != http.StatusOK {
		t.Fatalf("client list = %d, want 200", code)
	}
	if len(list) != 2 {
		t.Fatalf("client list = %d rows, want 2 (both VPNs are owned)", len(list))
	}

	byEmail := map[string]map[string]any{}
	for _, row := range list {
		byEmail[row["panel_email"].(string)] = row
	}

	row := byEmail["nimal@gmail.com_paid_1"]
	if row == nil {
		t.Fatalf("suspended VPN missing from the list: %v", list)
	}
	got, ok := row["status_changed_at"].(float64)
	if !ok {
		t.Fatalf("status_changed_at = %v (%T), want the status clock as a number",
			row["status_changed_at"], row["status_changed_at"])
	}
	if int64(got) != changedAt {
		t.Errorf("status_changed_at = %d, want %d (the stored clock, unchanged)", int64(got), changedAt)
	}
	if row["status"] != "SUSPENDED" {
		t.Errorf("status = %v, want SUSPENDED — the clock must describe the status beside it", row["status"])
	}

	row = byEmail["nimal@gmail.com_paid_2"]
	if row == nil {
		t.Fatalf("never-changed VPN missing from the list: %v", list)
	}
	if v, present := row["status_changed_at"]; present && v != nil {
		t.Errorf("status_changed_at = %v, want null — a VPN that never moved has no date, "+
			"and 0 would render as 1 January 1970", v)
	}
	_ = never
}
