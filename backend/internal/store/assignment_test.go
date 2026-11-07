package store

import (
	"context"
	"errors"
	"testing"
)

// seedAssignEnv inserts a server plus one client and one reseller account.
func seedAssignEnv(t *testing.T, ctx context.Context, st *Store) (serverID, clientID, adminID int64) {
	t.Helper()
	serverID = seedServer(t, st)
	clientID = seedUser(t, ctx, st, "nimal@gmail.com", "client")
	adminID = seedUser(t, ctx, st, "reseller@example.com", "admin")
	return serverID, clientID, adminID
}

// D-037 — an unassigned panel client can be given to a client account, and it
// becomes that client's Premium VPN with every feature a normal one has.
//
// The panel name follows D-034: `<client email>_<kind>_<slot>`.
// D-063 — an unassigned panel client can be given to a client account, and it
// becomes that client's PREMIUM VPN with every feature a normal one has.
//
// The VPN KEEPS the name the panel holds. It used to be recorded under an
// invented `<client email>_paid_<slot>` name, and when the follow-up panel
// rename failed the same physical VPN ended up under two names: the panel still
// listed the original, so the console offered "Assign" again, and a second click
// created a duplicate row for one VPN.
func TestAssignKeepsThePanelName(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID, clientID, _ := seedAssignEnv(t, ctx, st)

	res, err := st.AssignPanelClient(ctx, AssignPanelClientInput{
		ServerID: serverID, PanelEmail: "direct-in-3xui-1", ClientUserID: clientID,
		Kind: "paid", ActorID: 1,
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if res.PanelEmail != "direct-in-3xui-1" {
		t.Errorf("panel_email = %q, want the panel's own name direct-in-3xui-1", res.PanelEmail)
	}
	if res.Tag != "PREMIUM" {
		t.Errorf("tag = %q, want PREMIUM", res.Tag)
	}
	// An assigned VPN must carry a package: with none, the client projection had
	// no kind to send and the VPN rendered as UNTAGGED.
	if res.OfferID <= 0 {
		t.Errorf("offer_id = %d, want a real package (never 0)", res.OfferID)
	}

	var (
		owner  int64
		status string
	)
	if err := st.DB().QueryRowContext(ctx,
		`SELECT owner_user_id, status FROM vpn_instances WHERE id = ?`, res.VPNID).
		Scan(&owner, &status); err != nil {
		t.Fatalf("read vpn: %v", err)
	}
	if owner != clientID {
		t.Errorf("owner = %d, want the target client %d", owner, clientID)
	}
	if status != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE", status)
	}

	// The tag is written AND locked: an issued VPN can never be re-priced.
	var locked bool
	if err := st.DB().QueryRowContext(ctx,
		`SELECT locked_at IS NOT NULL FROM panel_client_tags WHERE server_id = ? AND panel_email = ?`,
		serverID, res.PanelEmail).Scan(&locked); err != nil {
		t.Fatalf("read tag: %v", err)
	}
	if !locked {
		t.Error("the tag was not locked — an assigned VPN must be re-priceable never")
	}

	// The identity ledger records the SAME name on both sides: one panel client,
	// one name, one row.
	var source, target string
	if err := st.DB().QueryRowContext(ctx,
		`SELECT source_email, panel_email FROM panel_assignments WHERE vpn_instance_id = ?`,
		res.VPNID).Scan(&source, &target); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if source != "direct-in-3xui-1" || target != "direct-in-3xui-1" {
		t.Errorf("ledger = %q -> %q, want both sides on the panel's name", source, target)
	}

	// The client holds exactly one premium VPN even though no reseller was
	// named: the console used to report 0 for a direct Super-Admin issue.
	var premium int
	if err := st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_instances i
		   JOIN vpn_offers o ON o.id = i.offer_id
		  WHERE i.owner_user_id = ? AND o.kind = 'PREMIUM'`, clientID).Scan(&premium); err != nil {
		t.Fatalf("count premium: %v", err)
	}
	if premium != 1 {
		t.Errorf("premium count = %d, want 1", premium)
	}
}

// A panel client Veylix already holds is not "unassigned": re-assigning it would
// silently move somebody's VPN.
func TestAssignRefusesAnAlreadyHeldPanelClient(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID, clientID, _ := seedAssignEnv(t, ctx, st)

	in := AssignPanelClientInput{ServerID: serverID, PanelEmail: "orphan-1", ClientUserID: clientID, Kind: "paid", ActorID: 1}
	if _, err := st.AssignPanelClient(ctx, in); err != nil {
		t.Fatalf("first assign: %v", err)
	}
	// The same panel client is now held; a second assign must be refused rather
	// than create a duplicate row.
	if _, err := st.AssignPanelClient(ctx, in); !errors.Is(err, ErrPanelClientTaken) {
		t.Fatalf("second assign error = %v, want ErrPanelClientTaken", err)
	}
}

// A VPN belongs to a customer, so the target must be a customer.
func TestAssignRefusesNonClientTarget(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID, _, adminID := seedAssignEnv(t, ctx, st)

	_, err := st.AssignPanelClient(ctx, AssignPanelClientInput{
		ServerID: serverID, PanelEmail: "orphan-2", ClientUserID: adminID, Kind: "paid", ActorID: 1,
	})
	if !errors.Is(err, ErrAssignTarget) {
		t.Fatalf("error = %v, want ErrAssignTarget", err)
	}
}

// D-034.2 — slots only move forward: the second paid VPN of one customer is
// `_paid_2`, even though the first one is a panel client Veylix never created.
// D-063 — the D-034 slot naming still exists, but it belongs to the optional
// rename step, not to the assignment. Assigning keeps the panel's name; asking
// for a rename afterwards is what produces `<client email>_paid_<slot>`, and
// slots only move forward.
func TestAssignThenRenameUsesSlots(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID, clientID, _ := seedAssignEnv(t, ctx, st)

	first, err := st.AssignPanelClient(ctx, AssignPanelClientInput{
		ServerID: serverID, PanelEmail: "a", ClientUserID: clientID, Kind: "paid", ActorID: 1,
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := st.AssignPanelClient(ctx, AssignPanelClientInput{
		ServerID: serverID, PanelEmail: "b", ClientUserID: clientID, Kind: "paid", ActorID: 1,
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	// Assignment itself keeps the panel's name.
	if first.PanelEmail != "a" || second.PanelEmail != "b" {
		t.Fatalf("assign must keep the panel names, got %q and %q", first.PanelEmail, second.PanelEmail)
	}

	// A failing panel rename changes nothing: the VPN keeps the name it has.
	failing := func(context.Context, string, string) error { return errors.New("panel offline") }
	if _, err := st.RenameAssignment(ctx, serverID, first.VPNID, "paid", failing); err == nil {
		t.Fatal("a failing panel rename must be reported, not swallowed")
	}
	var still string
	if err := st.DB().QueryRowContext(ctx,
		`SELECT panel_email FROM vpn_instances WHERE id = ?`, first.VPNID).Scan(&still); err != nil {
		t.Fatalf("read name after failed rename: %v", err)
	}
	if still != "a" {
		t.Errorf("after a failed rename the name = %q, want the existing name a", still)
	}

	// A confirmed rename moves the row, the tag and the ledger together.
	ok := func(context.Context, string, string) error { return nil }
	got, err := st.RenameAssignment(ctx, serverID, first.VPNID, "paid", ok)
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got != "nimal@gmail.com_paid_1" {
		t.Errorf("renamed to %q, want nimal@gmail.com_paid_1", got)
	}
	var vpnName, tagName, ledgerSource, ledgerTarget string
	if err := st.DB().QueryRowContext(ctx,
		`SELECT v.panel_email,
		        COALESCE(t.panel_email, ''),
		        COALESCE(p.source_email, ''),
		        COALESCE(p.panel_email, '')
		   FROM vpn_instances v
		   LEFT JOIN panel_client_tags t
		          ON t.server_id = v.server_id AND t.panel_email = v.panel_email
		   LEFT JOIN panel_assignments p ON p.vpn_instance_id = v.id
		  WHERE v.id = ?`, first.VPNID).Scan(&vpnName, &tagName, &ledgerSource, &ledgerTarget); err != nil {
		t.Fatalf("read after rename: %v", err)
	}
	if vpnName != "nimal@gmail.com_paid_1" {
		t.Errorf("row name = %q, want the new name", vpnName)
	}
	if tagName != "nimal@gmail.com_paid_1" {
		t.Errorf("tag name = %q — the locked tag must follow the name it labels", tagName)
	}
	if ledgerSource != "a" {
		t.Errorf("ledger source = %q, want the original panel name a", ledgerSource)
	}
	if ledgerTarget != "nimal@gmail.com_paid_1" {
		t.Errorf("ledger target = %q, want the name now in force", ledgerTarget)
	}

	// The second VPN renames to slot 2, not slot 1.
	got2, err := st.RenameAssignment(ctx, serverID, second.VPNID, "paid", ok)
	if err != nil {
		t.Fatalf("rename second: %v", err)
	}
	if got2 != "nimal@gmail.com_paid_2" {
		t.Errorf("second renamed to %q, want _paid_2", got2)
	}
}

// Two customers never collide on one panel name.
func TestAssignIsPerCustomer(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	serverID, clientID, _ := seedAssignEnv(t, ctx, st)
	other := seedUser(t, ctx, st, "kamal@gmail.com", "client")

	a, err := st.AssignPanelClient(ctx, AssignPanelClientInput{
		ServerID: serverID, PanelEmail: "x", ClientUserID: clientID, Kind: "paid", ActorID: 1,
	})
	if err != nil {
		t.Fatalf("assign to nimal: %v", err)
	}
	b, err := st.AssignPanelClient(ctx, AssignPanelClientInput{
		ServerID: serverID, PanelEmail: "y", ClientUserID: other, Kind: "paid", ActorID: 1,
	})
	if err != nil {
		t.Fatalf("assign to kamal: %v", err)
	}
	if a.PanelEmail == b.PanelEmail {
		t.Fatalf("both customers got %q — one panel name cannot serve two VPNs", a.PanelEmail)
	}
	if a.PanelEmail != "x" || b.PanelEmail != "y" {
		t.Errorf("assign must keep each panel's own name, got %q and %q", a.PanelEmail, b.PanelEmail)
	}
	// Two customers, two rows, two reservations.
	var rows int
	if err := st.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_instances WHERE server_id = ?`, serverID).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 2 {
		t.Errorf("vpn rows = %d, want 2", rows)
	}
}
