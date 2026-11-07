package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ClientRow is one client account visible to a reseller or the operator.
type ClientRow struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"created_at"`
	// VPNCount is how many VPN instances this client owns. Scoped to the caller
	// for a reseller, so it never leaks another seller's work.
	VPNCount int `json:"vpn_count"`
}

// ClientsVisible lists the client accounts a principal may act on.
//
//   - super admin: every client;
//   - admin: only clients who own a VPN they sold (vpn_assignments.seller_user_id).
//     A reseller must not be able to enumerate the whole customer base.
//   - client: forbidden (returns ErrNotAnAdmin's sibling — an empty list would
//     hide a mistake, so callers check the role first).
//
// VPNCount is scoped exactly like the rows. Before 2026-10-25 the count was
// `COUNT(*) FROM vpn_instances WHERE owner_user_id = u.id`, so a reseller saw a
// number that included other resellers' sales to the same customer — a
// cross-tenant leak (M6-07).
func (s *Store) ClientsVisible(ctx context.Context, callerID int64, callerRole string) ([]ClientRow, error) {
	// Two statements rather than one conditional subquery: a bound parameter
	// cannot sit inside a SELECT list that only some callers use.
	baseAll := `SELECT u.id, u.email, u.display_name, u.status, u.created_at,
	                (SELECT COUNT(*) FROM vpn_instances i WHERE i.owner_user_id = u.id)`
	baseScoped := `SELECT u.id, u.email, u.display_name, u.status, u.created_at,
	                (SELECT COUNT(*) FROM vpn_instances i
	                   JOIN vpn_assignments a2 ON a2.vpn_instance_id = i.id
	                  WHERE i.owner_user_id = u.id AND a2.seller_user_id = ?)`
	var (
		rows *sql.Rows
		err  error
	)

	if strings.EqualFold(callerRole, "super_admin") {
		rows, err = s.db.QueryContext(ctx,
			baseAll+` FROM users u WHERE u.role = 'client' ORDER BY u.id`)
	} else {
		// Only clients reachable through an assignment this admin sold.
		rows, err = s.db.QueryContext(ctx,
			baseScoped+` FROM users u
			 WHERE u.role = 'client'
			   AND EXISTS (
			         SELECT 1 FROM vpn_instances i
			           JOIN vpn_assignments a ON a.vpn_instance_id = i.id
			          WHERE i.owner_user_id = u.id AND a.seller_user_id = ?
			   )
			 ORDER BY u.id`, callerID, callerID)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list clients: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ClientRow, 0, 16)
	for rows.Next() {
		var c ClientRow
		if err := rows.Scan(&c.ID, &c.Email, &c.DisplayName, &c.Status, &c.CreatedAt, &c.VPNCount); err != nil {
			return nil, fmt.Errorf("store: scan client: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
