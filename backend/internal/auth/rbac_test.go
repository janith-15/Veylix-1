package auth

import "testing"

var (
	superAdmin = Principal{UserID: 1, Role: RoleSuperAdmin}
	adminA     = Principal{UserID: 2, Role: RoleAdmin}
	adminB     = Principal{UserID: 3, Role: RoleAdmin}
	clientA    = Principal{UserID: 4, Role: RoleClient}
	clientB    = Principal{UserID: 5, Role: RoleClient}
)

// REQUIREMENTS §4, row by row.
func TestPermissionMatrix(t *testing.T) {
	cases := []struct {
		perm  Permission
		sa    bool
		adm   bool
		cli   bool
		label string
	}{
		{PermServerManage, true, false, false, "manage all panel integrations"},
		{PermServerViewAll, true, false, false, "global server list"},
		{PermServerViewAssigned, true, true, false, "assigned servers"},
		{PermOfferManage, true, false, false, "create/approve offers"},
		{PermOfferUse, true, true, false, "use allowed offers"},
		{PermAdminManage, true, false, false, "create/suspend admins"},
		{PermVPNProvisionFree, true, false, true, "provision Free account"},
		{PermVPNAssignPremium, false, true, false, "assign Premium within quota"},
		{PermVPNAssignDirect, true, false, false, "direct Super Admin sale"},
		{PermPaymentConfirm, true, true, false, "confirm payment"},
		{PermAuditView, true, false, false, "global audit"},
		{PermServerMetrics, true, true, false, "server-wide CPU/RAM"},
	}

	for _, c := range cases {
		if superAdmin.Can(c.perm) != c.sa {
			t.Errorf("%s: super_admin Can(%s) = %v, want %v", c.label, c.perm, !c.sa, c.sa)
		}
		if adminA.Can(c.perm) != c.adm {
			t.Errorf("%s: admin Can(%s) = %v, want %v", c.label, c.perm, !c.adm, c.adm)
		}
		if clientA.Can(c.perm) != c.cli {
			t.Errorf("%s: client Can(%s) = %v, want %v", c.label, c.perm, !c.cli, c.cli)
		}
	}
}

// An unknown permission must deny everyone (fail closed).
func TestUnknownPermissionDeniesEveryone(t *testing.T) {
	unknown := Permission("made:up")
	for _, p := range []Principal{superAdmin, adminA, clientA} {
		if p.Can(unknown) {
			t.Errorf("%s must not hold an unknown permission", p.Role)
		}
	}
}

func TestRequireReturnsForbiddenForMissingPerms(t *testing.T) {
	if err := clientA.Require(PermVPNProvisionFree); err != nil {
		t.Errorf("client should be allowed to provision Free: %v", err)
	}
	if err := clientA.Require(PermServerManage); err != ErrForbidden {
		t.Errorf("client must be refused server management, got %v", err)
	}
	if err := adminA.Require(PermVPNProvisionFree, PermVPNAssignPremium); err != ErrForbidden {
		t.Error("admin must be refused when one of several permissions is missing")
	}
}

// T-14 / T-15: object-level access (IDOR).
func TestCanViewResource(t *testing.T) {
	soldByA := Ownership{OwnerUserID: clientA.UserID, SellerUserID: adminA.UserID}

	cases := []struct {
		name string
		p    Principal
		want bool
	}{
		{"super admin sees everything", superAdmin, true},
		{"selling admin sees own sale", adminA, true},
		{"other admin must not see it", adminB, false},
		{"owning client sees it", clientA, true},
		{"other client must not see it", clientB, false},
	}
	for _, c := range cases {
		if got := c.p.CanViewResource(soldByA); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// A direct Super Admin sale has no reseller; a client still only sees their own.
func TestDirectSaleOwnership(t *testing.T) {
	direct := Ownership{OwnerUserID: clientA.UserID, SellerUserID: superAdmin.UserID}
	if adminA.CanViewResource(direct) {
		t.Error("a reseller must not see a direct Super Admin sale")
	}
	if !clientA.CanViewResource(direct) {
		t.Error("the owning client must see their direct sale")
	}
}

// Mutating is stricter: a viewing admin is not automatically a mutating admin.
func TestCanMutateResource(t *testing.T) {
	soldByA := Ownership{OwnerUserID: clientA.UserID, SellerUserID: adminA.UserID}

	if !adminA.CanMutateResource(soldByA) {
		t.Error("the selling admin may mutate their own sale")
	}
	if adminB.CanMutateResource(soldByA) {
		t.Error("a different admin must not mutate another seller's resource")
	}
	if !clientA.CanMutateResource(soldByA) {
		t.Error("the owning client may act on their own resource")
	}
	if clientB.CanMutateResource(soldByA) {
		t.Error("another client must never mutate it")
	}
}

func TestCanAccessServer(t *testing.T) {
	assigned := map[int64]bool{10: true}

	if !superAdmin.CanAccessServer(99, assigned) {
		t.Error("super admin may access any server")
	}
	if !adminA.CanAccessServer(10, assigned) {
		t.Error("admin may access an assigned server")
	}
	if adminA.CanAccessServer(11, assigned) {
		t.Error("admin must not access an unassigned server")
	}
	if clientA.CanAccessServer(10, assigned) {
		t.Error("a client must never access a panel directly")
	}
}

func TestRoleValid(t *testing.T) {
	for _, r := range []Role{RoleSuperAdmin, RoleAdmin, RoleClient} {
		if !r.Valid() {
			t.Errorf("%s should be valid", r)
		}
	}
	if Role("root").Valid() {
		t.Error("an unknown role must be invalid")
	}
}
