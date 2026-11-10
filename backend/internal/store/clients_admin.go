package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ClientFilter is the client-management list query (D-042).
//
// Every field is optional; zero values mean "no restriction". Filtering happens in
// SQL rather than in the browser, so the page stays correct with ten thousand
// customers and a filter cannot be bypassed by editing a request.
type ClientFilter struct {
	// Q matches name, e-mail, username, phone or WhatsApp (lower-cased substring).
	Q string
	// Status is ACTIVE / SUSPENDED / PENDING_VERIFICATION.
	Status string
	// JoinedFrom / JoinedTo bound created_at (unix seconds, inclusive).
	JoinedFrom int64
	JoinedTo   int64
	// FreeMin / FreeMax bound the FREE claims in the last 30 days.
	FreeMin int
	FreeMax int
	// HasPremium / HasFree / GoogleLinked are tri-state: nil ignores the filter.
	HasPremium   *bool
	HasFree      *bool
	GoogleLinked *bool
	// Limit / Offset page the result.
	Limit  int
	Offset int
	// Sort is newest (default) | oldest | name | email | free | premium.
	Sort string
}

// ClientAdminRow is one row of the client list.
type ClientAdminRow struct {
	ID            int64   `json:"id"`
	Email         string  `json:"email"`
	EmailVerified bool    `json:"email_verified"`
	Username      string  `json:"username"`
	DisplayName   string  `json:"display_name"`
	Status        string  `json:"status"`
	AvatarURL     string  `json:"avatar_url"`
	Phone         string  `json:"phone"`
	WhatsApp      string  `json:"whatsapp"`
	Telegram      string  `json:"telegram"`
	Discord       string  `json:"discord"`
	Age           int     `json:"age"`
	GoogleLinked  bool    `json:"google_linked"`
	FreeVPNLimit  int     `json:"free_vpn_limit"`
	FreeUsed      int     `json:"free_used"`
	PremiumCount  int     `json:"premium_count"`
	VPNCount      int     `json:"vpn_count"`
	SellerIDs     []int64 `json:"seller_ids"`
	CreatedAt     int64   `json:"created_at"`
	LastLoginAt   int64   `json:"last_login_at"`
	LastLoginIP   string  `json:"last_login_ip"`
}

// freeWindowSeconds mirrors the rolling window the FREE cap counts over.

// Page-size bounds for the client console.
//
// These are exported so the transport can validate a caller's `?limit=` BEFORE
// deriving an offset from it, instead of each layer guessing the other's ceiling
// (B-2: the store clamped 200 while the handler derived its offset from the raw
// value).
const (
	ClientAdminDefaultLimit = 50
	ClientAdminMaxLimit     = 200
)

// ListClientsAdmin returns a page of client accounts plus the total that matched.
func (s *Store) ListClientsAdmin(ctx context.Context, f ClientFilter) ([]ClientAdminRow, int, error) {
	where := []string{"u.role = 'client'"}
	args := []any{}
	freeSince := time.Now().Unix() - freeWindowSeconds

	// Two reusable sub-selects: the FREE claims inside the window, and the
	// premium VPNs this client holds. Keeping them in one place means the filter
	// and the column can never disagree.
	const freeExpr = `(SELECT COUNT(*) FROM free_claims fc WHERE fc.client_user_id = u.id AND fc.claimed_at >= ?)`
	// Premium is counted from the VPN, not from the reseller link (D-063).
	//
	// It used to be `COUNT(*) FROM vpn_assignments`, which only existed when a
	// reseller was named on the assign. A Super Admin issuing a VPN directly —
	// the ordinary path — created no `vpn_assignments` row, so the client
	// console reported 0 premium VPNs for a customer who visibly held one, and
	// the `has_premium` filter skipped them. Counting the owned PREMIUM VPN
	// answers the question the screen actually asks: "how many premium VPNs does
	// this customer hold?", whoever issued them.
	//
	// The kind expression matches `httpapi`'s projection exactly, so this number
	// and the client's own VPN list can never disagree.
	const premiumExpr = `(SELECT COUNT(*) FROM vpn_instances i2
	                       LEFT JOIN vpn_offers o2 ON o2.id = i2.offer_id
	                       LEFT JOIN panel_client_tags t2
	                              ON t2.server_id = i2.server_id AND t2.panel_email = i2.panel_email
	                      WHERE i2.owner_user_id = u.id
	                        AND i2.status NOT IN ('REVOKED', 'REMOVED_FROM_CATALOG')
	                        AND (COALESCE(o2.kind, '') = 'PREMIUM'
	                             OR (COALESCE(o2.kind, '') = '' AND COALESCE(t2.tag, '') = 'PREMIUM')
	                             OR (COALESCE(o2.kind, '') = '' AND COALESCE(t2.tag, '') = '')))`

	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		where = append(where, `(lower(u.email) LIKE ? OR lower(u.display_name) LIKE ?
			OR lower(COALESCE(u.username,'')) LIKE ? OR u.phone LIKE ? OR u.whatsapp LIKE ?)`)
		args = append(args, like, like, like, like, like)
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
	if f.GoogleLinked != nil {
		if *f.GoogleLinked {
			where = append(where, "u.google_sub IS NOT NULL AND u.google_sub <> ''")
		} else {
			where = append(where, "(u.google_sub IS NULL OR u.google_sub = '')")
		}
	}
	// Each expression binds its own parameter, in the order it appears.
	if f.FreeMin > 0 {
		where = append(where, freeExpr+" >= ?")
		args = append(args, freeSince, f.FreeMin)
	}
	if f.FreeMax > 0 {
		where = append(where, freeExpr+" <= ?")
		args = append(args, freeSince, f.FreeMax)
	}
	if f.HasFree != nil {
		if *f.HasFree {
			where = append(where, freeExpr+" > 0")
		} else {
			where = append(where, freeExpr+" = 0")
		}
		args = append(args, freeSince)
	}
	if f.HasPremium != nil {
		if *f.HasPremium {
			where = append(where, premiumExpr+" > 0")
		} else {
			where = append(where, premiumExpr+" = 0")
		}
	}

	order := "u.created_at DESC"
	switch f.Sort {
	case "oldest":
		order = "u.created_at ASC"
	case "name":
		order = "lower(u.display_name) ASC"
	case "email":
		order = "lower(u.email) ASC"
	case "free":
		order = "free_used DESC"
	case "premium":
		order = "premium_count DESC"
	}

	// B-2: the handler computes Offset as (page-1)*Limit using the RAW limit the
	// client sent. Clamping `limit` here without recomputing the offset left the
	// two derived from DIFFERENT numbers: `?limit=9999&page=5` produced
	// `LIMIT 50 OFFSET 39996`, so an operator asking for "page 5" was shown rows
	// 40 000 deep and told the account had no clients.
	//
	// The honest fix is to clamp the page size and then derive the offset from the
	// clamped value, so offset and limit always describe the same window. A
	// client-supplied limit is a request, not a reservation.
	limit := f.Limit
	if limit <= 0 || limit > ClientAdminMaxLimit {
		limit = ClientAdminDefaultLimit
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	// Snap the offset onto the clamped page grid: whatever page number the caller
	// computed it from, the resulting window must start at a multiple of `limit`.
	f.Offset = (f.Offset / limit) * limit

	listSQL := `SELECT u.id, u.email, u.email_verified, COALESCE(u.username,''), u.display_name,
	                   u.status, u.avatar_url, u.phone, u.whatsapp, u.telegram, u.discord, u.age,
	                   COALESCE(u.google_sub,''), u.free_vpn_limit, u.created_at,
	                   u.last_login_at, u.last_login_ip,
	                   ` + freeExpr + ` AS free_used,
	                   ` + premiumExpr + ` AS premium_count,
	                   (SELECT COUNT(*) FROM vpn_instances i WHERE i.owner_user_id = u.id) AS vpn_count
	              FROM users u
	             WHERE ` + strings.Join(where, " AND ") + `
	             ORDER BY ` + order + `
	             LIMIT ? OFFSET ?`

	// The SELECT list's free_used expression comes BEFORE the WHERE parameters.
	listArgs := append([]any{freeSince}, args...)
	listArgs = append(listArgs, limit, f.Offset)

	rows, err := s.db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list clients: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ClientAdminRow, 0, limit)
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var (
			r      ClientAdminRow
			verif  int
			google string
		)
		if err := rows.Scan(&r.ID, &r.Email, &verif, &r.Username, &r.DisplayName, &r.Status,
			&r.AvatarURL, &r.Phone, &r.WhatsApp, &r.Telegram, &r.Discord, &r.Age, &google,
			&r.FreeVPNLimit, &r.CreatedAt, &r.LastLoginAt, &r.LastLoginIP,
			&r.FreeUsed, &r.PremiumCount, &r.VPNCount); err != nil {
			return nil, 0, fmt.Errorf("store: scan client: %w", err)
		}
		r.EmailVerified = verif == 1
		r.GoogleLinked = google != ""
		r.SellerIDs = []int64{}
		out = append(out, r)
		ids = append(ids, r.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// One extra query for the sellers of the whole page (never one per row).
	if len(ids) > 0 {
		sellers, err := s.sellersFor(ctx, ids)
		if err != nil {
			return nil, 0, err
		}
		for i := range out {
			if list, ok := sellers[out[i].ID]; ok {
				out[i].SellerIDs = list
			}
		}
	}

	total := 0
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users u WHERE `+strings.Join(where, " AND "), args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count clients: %w", err)
	}
	return out, total, nil
}

// sellersFor maps each client to the resellers who sold them something.
func (s *Store) sellersFor(ctx context.Context, ids []int64) (map[int64][]int64, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT i.owner_user_id, a.seller_user_id
		   FROM vpn_instances i
		   JOIN vpn_assignments a ON a.vpn_instance_id = i.id
		  WHERE i.owner_user_id IN (`+placeholders+`)
		  GROUP BY i.owner_user_id, a.seller_user_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: client sellers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[int64][]int64{}
	for rows.Next() {
		var owner, seller int64
		if err := rows.Scan(&owner, &seller); err != nil {
			return nil, err
		}
		out[owner] = append(out[owner], seller)
	}
	return out, rows.Err()
}

// ErrClientNotFound means the id is not a client account.
var ErrClientNotFound = errors.New("store: client not found")

// ErrClientHasVPNs refuses a delete while the client still owns VPNs on a panel.
// The operator revokes (or reassigns) them first, so no panel client is orphaned.
var ErrClientHasVPNs = errors.New("store: client still owns VPNs")

// ErrClientDeleted refuses to change the state of an account that was deleted.
// DELETED is terminal on purpose: "deleted" must not be a phrase that can quietly
// become ACTIVE again a week later.
var ErrClientDeleted = errors.New("store: client is deleted")

// ClientSetStatus blocks or unblocks a client and, when blocking, ends every live
// session in the SAME transaction — a blocked account must not keep working until
// its cookie happens to expire.
func (s *Store) ClientSetStatus(ctx context.Context, clientID int64, status string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// DELETED is terminal. Reading the current status first is what makes that
	// true: without it, DELETE then PATCH {status:"ACTIVE"} would resurrect an
	// account an operator had removed on purpose.
	var current string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM users WHERE id = ? AND role = 'client'`, clientID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrClientNotFound
	}
	if err != nil {
		return fmt.Errorf("store: read client status: %w", err)
	}
	if current == ClientStatusDeleted {
		return ErrClientDeleted
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE users SET status = ?, updated_at = ? WHERE id = ? AND role = 'client'`,
		status, time.Now().Unix(), clientID)
	if err != nil {
		return fmt.Errorf("store: set client status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrClientNotFound
	}
	if status != "ACTIVE" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, clientID); err != nil {
			return fmt.Errorf("store: revoke blocked sessions: %w", err)
		}
	}
	return tx.Commit()
}

// ClientStatusDeleted is the terminal state written by ClientDelete.
//
// Soft, not hard: `vpn_instances`, `payment_records` and `vpn_assignments` all
// reference users with ON DELETE RESTRICT, so the database refuses a real row
// delete anyway — and it should. Billing history and the panel's VPNs have to
// stay for accounting, and a 3X-UI client is not ours to erase.
const ClientStatusDeleted = "DELETED"

// ClientDelete marks a client account DELETED and ends every session.
//
// DELETED is terminal: ClientSetStatus refuses to move an account out of it, so a
// deleted client can never quietly become ACTIVE again.
//
// Refuses while the client still owns VPNs — those live on the panel, so removing
// the account first would leave orphaned clients nobody can attribute.
func (s *Store) ClientDelete(ctx context.Context, clientID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Only a client may be deleted from this console; a staff account is a
	// different decision with a different blast radius.
	var status string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM users WHERE id = ? AND role = 'client'`, clientID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrClientNotFound
	}
	if err != nil {
		return fmt.Errorf("store: read client: %w", err)
	}
	if status == ClientStatusDeleted {
		return nil // idempotent: deleting twice is a no-op, not an error
	}

	var vpns int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_instances WHERE owner_user_id = ?`, clientID).Scan(&vpns); err != nil {
		return fmt.Errorf("store: count client vpns: %w", err)
	}
	if vpns > 0 {
		return ErrClientHasVPNs
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET status = ?, updated_at = ? WHERE id = ? AND role = 'client'`,
		ClientStatusDeleted, time.Now().Unix(), clientID); err != nil {
		return fmt.Errorf("store: delete client: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, clientID); err != nil {
		return fmt.Errorf("store: revoke deleted client sessions: %w", err)
	}
	return tx.Commit()
}

// ClientSetFreeLimit sets a per-client FREE ceiling. 0 means "use the platform
// default", which is why 0 is a valid value rather than "no change".
func (s *Store) ClientSetFreeLimit(ctx context.Context, clientID int64, limit int) error {
	if limit < 0 {
		limit = 0
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET free_vpn_limit = ?, updated_at = ? WHERE id = ? AND role = 'client'`,
		limit, time.Now().Unix(), clientID)
	if err != nil {
		return fmt.Errorf("store: set free limit: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrClientNotFound
	}
	return nil
}

// ClientFreeLimitOverride reads the per-client FREE ceiling (0 = default).
func (s *Store) ClientFreeLimitOverride(ctx context.Context, clientID int64) (int, error) {
	var limit int
	err := s.db.QueryRowContext(ctx,
		`SELECT free_vpn_limit FROM users WHERE id = ? AND role = 'client'`, clientID).Scan(&limit)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrClientNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("store: read free limit: %w", err)
	}
	return limit, nil
}

const freeWindowSeconds = 30 * 24 * 3600

// ClientDetail is the full record the client-management page shows (D-042).
//
// It is ONE response rather than six calls: a page that renders "who is this
// customer, what do they own, what did they pay, what did we do to them" must not
// show half of it while the other half is still loading.
type ClientDetail struct {
	Profile *Profile `json:"profile"`
	// FreeUsed / FreeLimit describe the FREE claims inside the rolling cap window.
	// FreeLimit is the EFFECTIVE ceiling (per-client override, else the platform
	// default), and FreeOverride is the raw override (0 = none).
	FreeUsed       int          `json:"free_used"`
	FreeLimit      int          `json:"free_limit"`
	FreeOverride   int          `json:"free_limit_override"`
	FreeWindowDays int          `json:"free_window_days"`
	MonthlyClaims  []CountPoint `json:"monthly_claims"`
	PremiumCount   int          `json:"premium_count"`
	VPNCount       int          `json:"vpn_count"`
	Sellers        []int64      `json:"seller_ids"`
	RecentAudit    []AuditRow   `json:"recent_audit"`
}

// CountPoint is one bucket of a small series (a month) for the graph.
type CountPoint struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// ClientDetail reads everything the management page needs for one client.
func (s *Store) ClientDetail(ctx context.Context, clientID int64, freeDefault int) (*ClientDetail, error) {
	prof, err := s.ProfileByID(ctx, clientID)
	if err != nil || prof.Role != "client" {
		return nil, ErrClientNotFound
	}

	d := &ClientDetail{Profile: prof, FreeOverride: prof.FreeVPNLimit, FreeWindowDays: 30}
	d.FreeLimit = freeDefault
	if prof.FreeVPNLimit > 0 {
		d.FreeLimit = prof.FreeVPNLimit
	}

	since := time.Now().Unix() - freeWindowSeconds
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM free_claims WHERE client_user_id = ? AND claimed_at >= ?`,
		clientID, since).Scan(&d.FreeUsed); err != nil {
		return nil, fmt.Errorf("store: client free usage: %w", err)
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_instances WHERE owner_user_id = ?`, clientID).Scan(&d.VPNCount); err != nil {
		return nil, fmt.Errorf("store: client vpn count: %w", err)
	}
	// Premium is counted from the VPN, not from the reseller link — the same
	// question, and the same answer, as the console list (D-063).
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_instances i
		   LEFT JOIN vpn_offers o ON o.id = i.offer_id
		   LEFT JOIN panel_client_tags t
		          ON t.server_id = i.server_id AND t.panel_email = i.panel_email
		  WHERE i.owner_user_id = ?
		    AND i.status NOT IN ('REVOKED', 'REMOVED_FROM_CATALOG')
		    AND (COALESCE(o.kind, '') = 'PREMIUM'
		         OR (COALESCE(o.kind, '') = '' AND COALESCE(t.tag, '') = 'PREMIUM')
		         OR (COALESCE(o.kind, '') = '' AND COALESCE(t.tag, '') = ''))`,
		clientID).Scan(&d.PremiumCount); err != nil {
		return nil, fmt.Errorf("store: client premium count: %w", err)
	}

	d.Sellers = []int64{}
	if sellers, err := s.sellersFor(ctx, []int64{clientID}); err == nil {
		if list, ok := sellers[clientID]; ok {
			d.Sellers = list
		}
	}
	if d.MonthlyClaims, err = s.monthlyClaims(ctx, clientID); err != nil {
		return nil, err
	}
	if d.RecentAudit, err = s.auditForUser(ctx, clientID, 20); err != nil {
		return nil, err
	}
	return d, nil
}

// monthLayout is the bucket format for the claims graph.
const monthLayout = "2006-01"

// monthlyClaims returns the last 6 months of FREE claims, oldest first. Months
// with zero are included: a graph that omits empty months hides a gap.
func (s *Store) monthlyClaims(ctx context.Context, clientID int64) ([]CountPoint, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT strftime('%Y-%m', claimed_at, 'unixepoch') AS m, COUNT(*)
		   FROM free_claims
		  WHERE client_user_id = ? AND claimed_at >= ?
		  GROUP BY m`, clientID, time.Now().AddDate(0, -6, 0).Unix())
	if err != nil {
		return nil, fmt.Errorf("store: monthly claims: %w", err)
	}
	defer func() { _ = rows.Close() }()

	seen := map[string]int{}
	for rows.Next() {
		var (
			month string
			n     int
		)
		if err := rows.Scan(&month, &n); err != nil {
			return nil, err
		}
		seen[month] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]CountPoint, 0, 6)
	now := time.Now()
	for i := 5; i >= 0; i-- {
		key := now.AddDate(0, -i, 0).Format(monthLayout)
		out = append(out, CountPoint{Label: key, Count: seen[key]})
	}
	return out, nil
}

// auditForUser returns the most recent audit entries that targeted this account.
func (s *Store) auditForUser(ctx context.Context, userID int64, limit int) ([]AuditRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, actor_id, action, target_type, target_id, result, context, created_at
		   FROM audit_logs WHERE target_type = 'user' AND target_id = ?
		  ORDER BY id DESC LIMIT ?`, fmt.Sprint(userID), limit)
	if err != nil {
		return nil, fmt.Errorf("store: audit for user: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]AuditRow, 0, limit)
	for rows.Next() {
		var (
			r       AuditRow
			actor   sql.NullInt64
			ctxJSON string
		)
		if err := rows.Scan(&r.ID, &actor, &r.Action, &r.TargetType, &r.TargetID, &r.Result, &ctxJSON, &r.CreatedAt); err != nil {
			return nil, err
		}
		if actor.Valid {
			id := actor.Int64
			r.ActorID = &id
		}
		r.Context = ctxJSON
		out = append(out, r)
	}
	return out, rows.Err()
}
