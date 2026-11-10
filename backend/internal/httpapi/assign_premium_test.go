package httpapi

import (
	"context"
	"path/filepath"
	"testing"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/integrations"
	"veylix/backend/internal/store"
)

// premiumEnv is a migrated store the test drives directly, so the listing can be
// asserted for the exact rows an assignment produced.
type premiumEnv struct {
	store    *store.Store
	serverID int64
	clientID int64
	adminID  int64
}

func newPremiumEnv(t *testing.T) *premiumEnv {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "premium.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &premiumEnv{store: st}
}

// seed inserts the server and the accounts an assignment needs.
func (e *premiumEnv) seed(t *testing.T) {
	t.Helper()
	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := e.store.DB().Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	e.serverID = exec(`INSERT INTO server_integrations
		(name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('s','https://p.example/base', x'00','ACTIVE',1,1)`)
	e.clientID = exec(`INSERT INTO users (email, password_hash, role, status, created_at, updated_at)
		VALUES ('c@example.com','x','client','ACTIVE',1,1)`)
	e.adminID = exec(`INSERT INTO users (email, password_hash, role, status, created_at, updated_at)
		VALUES ('a@example.com','x','super_admin','ACTIVE',1,1)`)
}

// D-063 — the whole point of the fix, asserted at the layer the client screen
// reads: a VPN assigned to a customer must appear in that customer's list as
// PREMIUM, even when no package was chosen at assign time.
//
// This is the regression the product actually showed: the assign dialog sent no
// `offer_id`, so the row had no package, the projection sent an EMPTY
// `offer_kind`, and the client's screen — whose fallback was
// `offer_kind ?? tag` — never reached the tag (an empty string is not nullish).
// The VPN was displayed as UNTAGGED and pushed into the "Other VPNs" card while
// the "My Premium VPNs" card said none existed.
func TestAssignedVPNIsVisibleToItsClientAsPremium(t *testing.T) {
	ctx := context.Background()
	env := newPremiumEnv(t)
	env.seed(t)

	res, err := env.store.AssignPanelClient(ctx, store.AssignPanelClientInput{
		ServerID: env.serverID, PanelEmail: "direct-in-3xui-1",
		ClientUserID: env.clientID, Kind: "paid", ActorID: env.adminID,
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}

	l := NewSQLVPNLister(env.store.DB())
	p := auth.Principal{UserID: env.clientID, Role: auth.RoleClient}
	rows, err := l.ListVPNs(ctx, p, TagFilterAny)
	if err != nil {
		t.Fatalf("ListVPNs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("client sees %d VPNs, want exactly 1: %+v", len(rows), rows)
	}
	r := rows[0]
	if r.ID != res.VPNID {
		t.Errorf("id = %d, want the assigned VPN %d", r.ID, res.VPNID)
	}
	if r.OfferKind != "PREMIUM" {
		t.Errorf("offer_kind = %q, want PREMIUM — an assigned VPN must never read as untagged", r.OfferKind)
	}
	if r.KindSource == "none" || r.KindSource == "" {
		t.Errorf("kind_source = %q, want offer/tag/default", r.KindSource)
	}
	if !r.Assigned {
		t.Error("assigned = false, want true for a VPN a customer holds")
	}
	if r.OwnerID != env.clientID {
		t.Errorf("owner = %d, want the client %d", r.OwnerID, env.clientID)
	}

	// The premium filter must find it too — that is the list the dashboard draws.
	premiumRows, err := l.ListVPNs(ctx, p, TagFilterPremium)
	if err != nil {
		t.Fatalf("ListVPNs(premium): %v", err)
	}
	if len(premiumRows) != 1 {
		t.Errorf("premium filter returned %d rows, want 1 — the VPN belongs in My Premium VPNs", len(premiumRows))
	}
}

// A panel client nobody holds is the ONLY row that may read as untagged.
func TestUnassignedPanelClientIsTheOnlyUntaggedRow(t *testing.T) {
	ctx := context.Background()
	env := newPremiumEnv(t)
	env.seed(t)

	if _, err := env.store.AssignPanelClient(ctx, store.AssignPanelClientInput{
		ServerID: env.serverID, PanelEmail: "held-by-a-client",
		ClientUserID: env.clientID, Kind: "paid", ActorID: env.adminID,
	}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	// No owned row may be missing a package: that is what made an assigned VPN
	// project an empty kind and render as UNTAGGED.
	var unclassified int
	if err := env.store.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_instances WHERE owner_user_id IS NOT NULL AND offer_id IS NULL`).
		Scan(&unclassified); err != nil {
		t.Fatalf("count: %v", err)
	}
	if unclassified != 0 {
		t.Errorf("%d owned rows have no package", unclassified)
	}
}

// D-063 - a VPN Veylix recorded as owned is ALWAYS assigned, whatever the panel
// read could resolve.
//
// The seam this covers is `panelViewToRow`, and it was untested: every other
// assignment test builds its rows by hand or reads through `SQLVPNLister`, so the
// Super Admin console path - the one that merges a panel client with the local
// row - had no test at all. The defect it let through: the panel only answers
// "assigned" when it can still resolve the client, so a client missing from the
// panel (or a panel that could not be read) reported `assigned: false` even for a
// row with an owner. The console then printed "unassigned" for a VPN a customer
// holds, and `?assigned=1` returned nothing.
func TestOwnedPanelClientIsAlwaysAssigned(t *testing.T) {
	// A local row Veylix knows to be owned.
	local := VPNRow{ID: 36, ServerID: 11, PanelEmail: "c@example.com_paid_1", OwnerID: 24,
		OwnerEmail: "c@example.com", Status: "SERVER_UNAVAILABLE", OfferKind: "PREMIUM", KindSource: "offer"}

	cases := []struct {
		name string
		view integrations.PanelClientView
		want bool
	}{
		{"panel could not resolve the owner", integrations.PanelClientView{PanelEmail: local.PanelEmail, Assigned: false}, true},
		{"panel confirms the owner", integrations.PanelClientView{PanelEmail: local.PanelEmail, Assigned: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := panelViewToRow(11, "indian-1", tc.view, local)
			if got.Assigned != tc.want {
				t.Errorf("assigned = %v, want %v - an owned row is assigned by Veylix's own record", got.Assigned, tc.want)
			}
			if got.OwnerID != 24 {
				t.Errorf("owner id = %d, want 24", got.OwnerID)
			}
		})
	}

	// A panel client nobody holds stays unassigned - that is the only case where
	// "unassigned" is the truth.
	none := panelViewToRow(11, "indian-1", integrations.PanelClientView{PanelEmail: "stranger", Assigned: false}, VPNRow{PanelEmail: "stranger"})
	if none.Assigned {
		t.Error("a panel client with no local row must stay unassigned")
	}

	// The owner id can also arrive from the panel side alone; it must still make
	// the row filter as assigned rather than leaving two fields disagreeing.
	fromPanel := panelViewToRow(11, "indian-1", integrations.PanelClientView{PanelEmail: "x", Assigned: true, OwnerUserID: 24}, VPNRow{PanelEmail: "x"})
	if !fromPanel.Assigned || fromPanel.OwnerID != 24 {
		t.Errorf("panel-resolved owner: assigned=%v owner=%d, want true/24", fromPanel.Assigned, fromPanel.OwnerID)
	}
}
