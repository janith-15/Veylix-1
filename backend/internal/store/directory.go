package store

import (
	"context"
	"database/sql"
	"fmt"
)

// DirectoryRow is one entry of the account directory the UI can name.
//
// It is deliberately **not** a full user projection: no password hash, no
// session data, no per-account limits. A directory answers "who exists and what
// role do they hold", which is the minimum a chat participant picker needs
// (REQUIREMENTS §2.6).
type DirectoryRow struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
}

// Directory returns the accounts a caller may address.
//
// Scoping is a business rule, so it lives in the query rather than the UI:
//   - super admin: every active/suspended account;
//   - admin:      themselves + the clients they sold to + every super admin;
//   - client:     themselves + every super admin + the resellers who sold them
//     a VPN.
//
// A client never receives the full reseller list, and a reseller never receives
// another reseller's customers, so the picker cannot be used to enumerate the
// platform.
func (s *Store) Directory(ctx context.Context, callerID int64, callerRole string) ([]DirectoryRow, error) {
	const cols = `u.id, u.email, COALESCE(u.display_name, ''), u.role, u.status`

	var (
		rows *sql.Rows
		err  error
	)
	switch callerRole {
	case "super_admin":
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+cols+` FROM users u ORDER BY u.role, u.email`)
	case "admin":
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+cols+` FROM users u
			  WHERE u.id = ?
			     OR u.role = 'super_admin'
			     OR (u.role = 'client' AND EXISTS (
			           SELECT 1 FROM vpn_assignments a
			            WHERE a.client_user_id = u.id AND a.seller_user_id = ?))
			  ORDER BY u.role, u.email`, callerID, callerID)
	default: // client
		rows, err = s.db.QueryContext(ctx,
			`SELECT `+cols+` FROM users u
			  WHERE u.id = ?
			     OR u.role = 'super_admin'
			     OR (u.role = 'admin' AND EXISTS (
			           SELECT 1 FROM vpn_assignments a
			            WHERE a.client_user_id = ? AND a.seller_user_id = u.id))
			  ORDER BY u.role, u.email`, callerID, callerID)
	}
	if err != nil {
		return nil, fmt.Errorf("store: directory: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]DirectoryRow, 0, 16)
	for rows.Next() {
		var r DirectoryRow
		if err := rows.Scan(&r.ID, &r.Email, &r.DisplayName, &r.Role, &r.Status); err != nil {
			return nil, fmt.Errorf("store: scan directory: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
