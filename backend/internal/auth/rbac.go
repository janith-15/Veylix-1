package auth

// Role is the platform role (REQUIREMENTS §4).
type Role string

const (
	RoleSuperAdmin Role = "super_admin"
	RoleAdmin      Role = "admin"
	RoleClient     Role = "client"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	switch r {
	case RoleSuperAdmin, RoleAdmin, RoleClient:
		return true
	}
	return false
}

// Principal is the authenticated actor for one request.
type Principal struct {
	UserID int64
	Email  string
	Role   Role
}

// IsSuperAdmin / IsAdmin / IsClient keep call sites readable.
func (p Principal) IsSuperAdmin() bool { return p.Role == RoleSuperAdmin }
func (p Principal) IsAdmin() bool      { return p.Role == RoleAdmin }
func (p Principal) IsClient() bool     { return p.Role == RoleClient }

// Permission is a named capability. Every route maps to one, and the matrix
// below is the single place authorization is decided (REQUIREMENTS §4 —
// "enforced in the backend, not just hidden in the UI").
type Permission string

const (
	PermServerManage       Permission = "server:manage"
	PermServerViewAll      Permission = "server:view_all"
	PermServerViewAssigned Permission = "server:view_assigned"
	PermOfferManage        Permission = "offer:manage"
	PermOfferUse           Permission = "offer:use"
	PermAdminManage        Permission = "admin:manage"
	PermVPNProvisionFree   Permission = "vpn:provision_free"
	PermVPNAssignPremium   Permission = "vpn:assign_premium"
	PermVPNAssignDirect    Permission = "vpn:assign_direct"
	PermVPNViewOwn         Permission = "vpn:view_own"
	PermVPNViewSellerScope Permission = "vpn:view_seller_scope"
	PermPaymentConfigure   Permission = "payment:configure"
	PermPaymentConfirm     Permission = "payment:confirm"
	PermAuditView          Permission = "audit:view"
	PermServerMetrics      Permission = "server:metrics"
	// PermClientManage is the Super Admin's client-management console (D-042):
	// seeing every customer's full record, blocking an account, and setting a
	// per-client FREE limit. It is deliberately Super-Admin-only — a reseller sees
	// the clients it sold through the scoped /api/clients list, never this one.
	PermClientManage Permission = "client:manage"
)

// permissionMatrix encodes REQUIREMENTS §4. Order: super_admin, admin, client.
var permissionMatrix = map[Permission]map[Role]bool{
	PermServerManage:       {RoleSuperAdmin: true},
	PermServerViewAll:      {RoleSuperAdmin: true},
	PermServerViewAssigned: {RoleSuperAdmin: true, RoleAdmin: true},
	PermOfferManage:        {RoleSuperAdmin: true},
	PermOfferUse:           {RoleSuperAdmin: true, RoleAdmin: true},
	PermAdminManage:        {RoleSuperAdmin: true},
	PermVPNProvisionFree:   {RoleSuperAdmin: true, RoleClient: true},
	PermVPNAssignPremium:   {RoleAdmin: true},
	PermVPNAssignDirect:    {RoleSuperAdmin: true},
	PermVPNViewOwn:         {RoleSuperAdmin: true, RoleAdmin: true, RoleClient: true},
	PermVPNViewSellerScope: {RoleSuperAdmin: true, RoleAdmin: true},
	PermPaymentConfigure:   {RoleSuperAdmin: true, RoleAdmin: true},
	PermPaymentConfirm:     {RoleSuperAdmin: true, RoleAdmin: true},
	PermAuditView:          {RoleSuperAdmin: true},
	PermServerMetrics:      {RoleSuperAdmin: true, RoleAdmin: true},
	PermClientManage:       {RoleSuperAdmin: true},
}

// Can reports whether p holds perm.
func (p Principal) Can(perm Permission) bool {
	roles, ok := permissionMatrix[perm]
	if !ok {
		return false // unknown permission → deny (fail closed)
	}
	return roles[p.Role]
}

// Require returns nil when the principal holds every listed permission.
func (p Principal) Require(perms ...Permission) error {
	for _, perm := range perms {
		if !p.Can(perm) {
			return ErrForbidden
		}
	}
	return nil
}

// Ownership describes who may see one resource.
type Ownership struct {
	OwnerUserID  int64 // the client that holds the VPN / client account
	SellerUserID int64 // the admin (or super admin) who sold it; 0 = direct
	ServerID     int64
}

// CanViewResource applies row-level ownership (new_update.md §7 "insecure
// object references").
//
//   - super admin: everything (audited elsewhere);
//   - admin: only resources they sold or own;
//   - client: only their own resources.
//
// A caller must never be able to widen this by sending another id
// (Bible §8: "Never trust a client-supplied user ID").
func (p Principal) CanViewResource(o Ownership) bool {
	switch p.Role {
	case RoleSuperAdmin:
		return true
	case RoleAdmin:
		return o.SellerUserID == p.UserID || o.OwnerUserID == p.UserID
	case RoleClient:
		return o.OwnerUserID == p.UserID
	}
	return false
}

// CanMutateResource is stricter than viewing: a seller may manage what they
// sold, a client may manage only their own (e.g. request revocation).
func (p Principal) CanMutateResource(o Ownership) bool {
	switch p.Role {
	case RoleSuperAdmin:
		return true
	case RoleAdmin:
		return o.SellerUserID == p.UserID
	case RoleClient:
		return o.OwnerUserID == p.UserID
	}
	return false
}

// CanAccessServer reports whether the principal may operate on a panel server
// (assignments come from admin_server_assignments for admins).
func (p Principal) CanAccessServer(serverID int64, assigned map[int64]bool) bool {
	switch p.Role {
	case RoleSuperAdmin:
		return true
	case RoleAdmin:
		return assigned[serverID]
	}
	return false
}
