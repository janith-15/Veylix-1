package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// StaffFilter is the staff-management list query (D-044): administrators and super
// admins, so the Super Admin can see who else holds power.
type StaffFilter struct {
	// Q matches name, e-mail or username (lower-cased substring).
	Q string
	// Role is admin | super_admin | "" (both).
	Role string
	// Status is ACTIVE | SUSPENDED | "".
	Status string
	// JoinedFrom / JoinedTo bound created_at (unix seconds, inclusive).
	JoinedFrom int64
	JoinedTo   int64
	// QuotaMin / QuotaMax bound the reseller quota ceiling. A quota filter only
	// ever describes resellers: a super admin has no ceiling.
	QuotaMin int
	QuotaMax int
	// ServerID restricts to staff assigned to that panel server.
	ServerID int64
	Limit    int
	Offset   int
	Sort     string // newest | oldest | name | email | quota
}

// StaffRow is one row of the staff console.
type StaffRow struct {
	ID            int64  `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Username      string `json:"username"`
	DisplayName   string `json:"display_name"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	AvatarURL     string `json:"avatar_url"`
	QuotaLimit    int    `json:"quota_limit"`
	// QuotaUsed counts the VPN assignments this reseller sold, which is the
	// number the ceiling is measured against (REQUIREMENTS §2.2).
	QuotaUsed   int     `json:"quota_used"`
	ServerIDs   []int64 `json:"server_ids"`
	CreatedAt   int64   `json:"created_at"`
	LastLoginAt int64   `json:"last_login_at"`
	LastLoginIP string  `json:"last_login_ip"`
}

// StaffDetail is the full record of one staff account.
type StaffDetail struct {
	Profile StaffRow `json:"profile"`
	// The reseller's book: what they are responsible for.
	Clients  int `json:"clients"`
	VPNsSold int `json:"vpns_sold"`
	Payments int `json:"payments"`
	// Servers is the panel scope, resolved so the page can name them.
	Servers     []ServerRow `json:"servers"`
	RecentAudit []AuditRow  `json:"recent_audit"`
}

// ErrStaffNotFound means the id is not an administrator or super admin.

// ListStaff returns a page of staff accounts plus the total that matched.
// Page-size bounds for the staff console. Exported for the same reason the client
// console's are: the transport must validate a caller's `limit` before deriving an
// offset from it, instead of guessing the store's ceiling.
const (
	StaffDefaultLimit = 50
	StaffMaxLimit     = 200
)

func (s *Store) ListStaff(ctx context.Context, f StaffFilter) ([]StaffRow, int, error) {
	where := []string{"u.role IN ('admin', 'super_admin')"}
	args := []any{}

	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		where = append(where, `(lower(u.email) LIKE ? OR lower(u.display_name) LIKE ?
			OR lower(COALESCE(u.username,'')) LIKE ?)`)
		args = append(args, like, like, like)
	}
	if f.Role != "" {
		where = append(where, "u.role = ?")
		args = append(args, f.Role)
	}
	if f.Status != "" {
		where = append(where, "u.status = ?")
		args = append(args, f.Status)
	}
	if f.JoinedFrom > 0 {
		where = append(where, "u.created_at >= ?")
		args = append(args, f.JoinedFrom)
	}
	if f.JoinedTo > 0 {
		where = append(where, "u.created_at <= ?")
		args = append(args, f.JoinedTo)
	}
	if f.QuotaMin > 0 {
		where = append(where, "u.role = 'admin' AND u.quota_limit >= ?")
		args = append(args, f.QuotaMin)
	}
	if f.QuotaMax > 0 {
		where = append(where, "u.role = 'admin' AND u.quota_limit <= ?")
		args = append(args, f.QuotaMax)
	}
	if f.ServerID > 0 {
		where = append(where, `EXISTS (SELECT 1 FROM admin_server_assignments a
		                              WHERE a.admin_id = u.id AND a.server_id = ?)`)
		args = append(args, f.ServerID)
	}

	order := "u.created_at DESC"
	switch f.Sort {
	case "oldest":
		order = "u.created_at ASC"
	case "name":
		order = "lower(u.display_name) ASC"
	case "email":
		order = "lower(u.email) ASC"
	case "quota":
		order = "u.quota_limit DESC"
	}

	// Same clamp-then-snap as the client console (B-2): the transport derives the
	// offset from a limit it already clamped, and this ensures the two can never
	// describe different windows even if a future caller gets it wrong.
	limit := f.Limit
	if limit <= 0 || limit > StaffMaxLimit {
		limit = StaffDefaultLimit
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	f.Offset = (f.Offset / limit) * limit

	const usedExpr = `(SELECT COUNT(*) FROM vpn_assignments va WHERE va.seller_user_id = u.id)`

	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.email, u.email_verified, COALESCE(u.username,''), u.display_name,
		        u.role, u.status, u.avatar_url, u.quota_limit, u.created_at,
		        u.last_login_at, u.last_login_ip, `+usedExpr+` AS quota_used
		   FROM users u
		  WHERE `+strings.Join(where, " AND ")+`
		  ORDER BY `+order+`
		  LIMIT ? OFFSET ?`, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list staff: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]StaffRow, 0, limit)
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var (
			r     StaffRow
			verif int
		)
		if err := rows.Scan(&r.ID, &r.Email, &verif, &r.Username, &r.DisplayName, &r.Role,
			&r.Status, &r.AvatarURL, &r.QuotaLimit, &r.CreatedAt, &r.LastLoginAt,
			&r.LastLoginIP, &r.QuotaUsed); err != nil {
			return nil, 0, fmt.Errorf("store: scan staff: %w", err)
		}
		r.EmailVerified = verif == 1
		r.ServerIDs = []int64{}
		out = append(out, r)
		ids = append(ids, r.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if len(ids) > 0 {
		assign, err := s.serverAssignmentsFor(ctx, ids)
		if err != nil {
			return nil, 0, err
		}
		for i := range out {
			if list, ok := assign[out[i].ID]; ok {
				out[i].ServerIDs = list
			}
		}
	}

	total := 0
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users u WHERE `+strings.Join(where, " AND "), args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count staff: %w", err)
	}
	return out, total, nil
}

// serverAssignmentsFor maps each admin to its assigned server ids in one query.
func (s *Store) serverAssignmentsFor(ctx context.Context, ids []int64) (map[int64][]int64, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT admin_id, server_id FROM admin_server_assignments
		  WHERE admin_id IN (`+placeholders+`) ORDER BY server_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: server assignments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[int64][]int64{}
	for rows.Next() {
		var admin, server int64
		if err := rows.Scan(&admin, &server); err != nil {
			return nil, err
		}
		out[admin] = append(out[admin], server)
	}
	return out, rows.Err()
}

var ErrStaffNotFound = errors.New("store: staff account not found")

// StaffDetailFor reads everything the staff page needs for one account.
func (s *Store) StaffDetailFor(ctx context.Context, staffID int64) (*StaffDetail, error) {
	var (
		r     StaffRow
		verif int
	)
	const usedExpr = `(SELECT COUNT(*) FROM vpn_assignments va WHERE va.seller_user_id = u.id)`
	err := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.email, u.email_verified, COALESCE(u.username,''), u.display_name,
		        u.role, u.status, u.avatar_url, u.quota_limit, u.created_at,
		        u.last_login_at, u.last_login_ip, `+usedExpr+`
		   FROM users u WHERE u.id = ? AND u.role IN ('admin', 'super_admin')`, staffID).
		Scan(&r.ID, &r.Email, &verif, &r.Username, &r.DisplayName, &r.Role, &r.Status,
			&r.AvatarURL, &r.QuotaLimit, &r.CreatedAt, &r.LastLoginAt, &r.LastLoginIP, &r.QuotaUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrStaffNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: read staff: %w", err)
	}
	r.EmailVerified = verif == 1
	r.ServerIDs = []int64{}

	if r.ServerIDs, err = s.adminServerIDs(ctx, staffID); err != nil {
		return nil, err
	}

	d := &StaffDetail{Profile: r}
	// The book: distinct customers they sold to, VPNs sold, and payments taken.
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT client_user_id) FROM vpn_assignments WHERE seller_user_id = ?`,
		staffID).Scan(&d.Clients); err != nil {
		return nil, fmt.Errorf("store: staff clients: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_assignments WHERE seller_user_id = ?`, staffID).Scan(&d.VPNsSold); err != nil {
		return nil, fmt.Errorf("store: staff vpn count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM payment_records WHERE confirmed_by = ?`, staffID).Scan(&d.Payments); err != nil {
		// A confirmation column that does not exist yet is not an error here: the
		// page simply reports 0 rather than failing to open.
		d.Payments = 0
	}

	if len(r.ServerIDs) > 0 {
		if d.Servers, err = s.serversByID(ctx, r.ServerIDs); err != nil {
			return nil, err
		}
	} else {
		d.Servers = []ServerRow{}
	}
	if d.RecentAudit, err = s.auditForUser(ctx, staffID, 20); err != nil {
		return nil, err
	}
	return d, nil
}

// adminServerIDs returns the panel servers an admin is scoped to.
func (s *Store) adminServerIDs(ctx context.Context, adminID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT server_id FROM admin_server_assignments WHERE admin_id = ? ORDER BY server_id`, adminID)
	if err != nil {
		return nil, fmt.Errorf("store: admin servers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]int64, 0, 4)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// serversByID resolves ids to rows, preserving the requested order.
func (s *Store) serversByID(ctx context.Context, ids []int64) ([]ServerRow, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, panel_url, status, COALESCE(panel_version,''), COALESCE(xray_version,''),
		        last_health_check, last_sync
		   FROM server_integrations WHERE id IN (`+placeholders+`) ORDER BY id`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: servers by id: %w", err)
	}
	defer func() { _ = rows.Close() }()

	found := map[int64]ServerRow{}
	for rows.Next() {
		var (
			r    ServerRow
			hc   sql.NullInt64
			sync sql.NullInt64
		)
		if err := rows.Scan(&r.ID, &r.Name, &r.PanelURL, &r.Status, &r.PanelVersion,
			&r.XrayVersion, &hc, &sync); err != nil {
			return nil, err
		}
		if hc.Valid {
			v := hc.Int64
			r.LastHealthCheck = &v
		}
		if sync.Valid {
			v := sync.Int64
			r.LastSync = &v
		}
		found[r.ID] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]ServerRow, 0, len(ids))
	for _, id := range ids {
		if r, ok := found[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}
