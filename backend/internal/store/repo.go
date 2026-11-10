package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Repositories live in this package so only one layer speaks SQL
// (docs/ARCHITECTURE.md §1).

// ErrNoRows is returned when a lookup finds nothing; transport maps it to 404.
var ErrNoRows = errors.New("store: no rows")

// AuditRow is one audit entry returned to the API.
type AuditRow struct {
	ID int64 `json:"id"`
	// ActorName is the actor's display name, or their e-mail when the display
	// name is empty, or "" for system entries. "user #12" made the operator
	// open the directory to answer "who did this" — the trail can say so.
	ActorID    *int64 `json:"actor_id"`
	ActorName  string `json:"actor_name,omitempty"`
	Action     string `json:"action"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Result     string `json:"result"`
	Context    string `json:"context"`
	CreatedAt  int64  `json:"created_at"`
}

// AuditQuery narrows an audit-trail read (docs/API_CONTRACT.md §5).
//
// Zero values mean "no restriction": the trail is append-only and the operator
// usually wants the newest page, so every filter is opt-in.
type AuditQuery struct {
	Limit   int    // window size; <=0 or >200 clamps to 100
	Offset  int    // rows to skip; negative clamps to 0
	Kind    string // "" = all; one of AuditKinds
	ActorID int64  // 0 = any actor
	From    int64  // created_at >= From (unix seconds); 0 = unbounded
	To      int64  // created_at <= To (unix seconds); 0 = unbounded
	Q       string // substring across action, actor name, target and context
}

// AuditPage is one window over the trail plus the count the window was cut from.
// `total` is the count AFTER the filters but BEFORE the window — the reason a
// filtered page can honestly say "3 of 3 matches" where the old `limit`-only
// read said "nothing recorded" because the match sat past the window.
type AuditPage struct {
	Entries []AuditRow `json:"entries"`
	Total   int64      `json:"total"`
	Limit   int        `json:"limit"`
	Offset  int        `json:"offset"`
	HasMore bool       `json:"has_more"`
}

// auditKindFamilies maps a public kind token onto the action prefixes it covers.
// `payment` deliberately covers payment_method.* too: "payments" is one answer
// in the operator's head, and a dotted prefix is an implementation detail.
// `client` covers user.* — the audit table names the SUBJECT's table, the UI
// names the person.
var auditKindFamilies = map[string][]string{
	"auth":    {"auth."},
	"server":  {"server."},
	"admin":   {"admin."},
	"vpn":     {"vpn."},
	"payment": {"payment.", "payment_method."},
	"client":  {"user."},
}

// AuditKinds is the accepted `kind` set — "" plus the families above plus
// "other" (anything matching none of them: chat.*, panel_client.*, …).
var AuditKinds = []string{"", "auth", "server", "admin", "vpn", "payment", "client", "other"}

// ValidAuditKind reports whether the kind token is one the store understands.
func ValidAuditKind(kind string) bool {
	for _, k := range AuditKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// escapeLike neutralises LIKE wildcards in user input, so a search for "50%"
// finds the literal "50%" instead of "50" followed by anything.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// actorNameExpr is the actor's best human label straight from the join.
const actorNameExpr = `COALESCE(NULLIF(u.display_name, ''), u.email)`

// ListAudit returns one window over the trail, newest first, with the total
// count the window was cut from.
//
// The filters run in SQL — the trail grows without bound, so filtering a
// fixed 100-row window in the UI could silently hide the very entry the
// operator is looking for (the "nothing recorded" lie).
func (s *Store) ListAudit(ctx context.Context, q AuditQuery) (AuditPage, error) {
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	where := make([]string, 0, 6)
	args := make([]any, 0, 8)
	if q.Kind != "" {
		if families, ok := auditKindFamilies[q.Kind]; ok {
			ors := make([]string, 0, len(families))
			for _, fam := range families {
				ors = append(ors, `a.action LIKE ?`)
				args = append(args, fam+"%")
			}
			where = append(where, "("+strings.Join(ors, " OR ")+")")
		} else if q.Kind == "other" {
			// "Other" is honest exclusion: every family an operator can name
			// directly, so chat/panel_client and future prefixes land here.
			for _, fam := range []string{"auth.", "server.", "admin.", "vpn.", "payment.", "payment_method.", "user."} {
				where = append(where, `a.action NOT LIKE ?`)
				args = append(args, fam+"%")
			}
		}
	}
	if q.ActorID > 0 {
		where = append(where, `a.actor_id = ?`)
		args = append(args, q.ActorID)
	}
	if q.From > 0 {
		where = append(where, `a.created_at >= ?`)
		args = append(args, q.From)
	}
	if q.To > 0 {
		where = append(where, `a.created_at <= ?`)
		args = append(args, q.To)
	}
	if needle := strings.TrimSpace(q.Q); needle != "" {
		like := "%" + escapeLike(needle) + "%"
		where = append(where,
			`(a.action LIKE ? ESCAPE '\'
			   OR a.target_type LIKE ? ESCAPE '\'
			   OR a.target_id LIKE ? ESCAPE '\'
			   OR a.context LIKE ? ESCAPE '\'
			   OR `+actorNameExpr+` LIKE ? ESCAPE '\')`)
		for i := 0; i < 5; i++ {
			args = append(args, like)
		}
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	base := ` FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id` + clause

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*)`+base, args...).Scan(&total); err != nil {
		return AuditPage{}, fmt.Errorf("store: count audit: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.actor_id, a.action, a.target_type, a.target_id, a.result, a.context, a.created_at, `+
			actorNameExpr+base+` ORDER BY a.id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return AuditPage{}, fmt.Errorf("store: list audit: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]AuditRow, 0, limit)
	for rows.Next() {
		var r AuditRow
		var actor sql.NullInt64
		var name sql.NullString
		if err := rows.Scan(&r.ID, &actor, &r.Action, &r.TargetType, &r.TargetID, &r.Result, &r.Context, &r.CreatedAt, &name); err != nil {
			return AuditPage{}, fmt.Errorf("store: scan audit: %w", err)
		}
		if actor.Valid {
			id := actor.Int64
			r.ActorID = &id
		}
		r.ActorName = name.String
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return AuditPage{}, fmt.Errorf("store: list audit: %w", err)
	}

	return AuditPage{
		Entries: out,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: offset+limit < int(total),
	}, nil
}

// QuotaUsage is an admin's current assignment usage.
type QuotaUsage struct {
	Used      int `json:"used"`
	Limit     int `json:"limit"`
	Remaining int `json:"remaining"`
}

// QuotaForAdmin computes used/limit from the ledger and the user row.
//
// "used" counts ACTIVE assignments only, so expiry/revocation releases a slot
// (REQUIREMENTS §2.2 quota rule).
func (s *Store) QuotaForAdmin(ctx context.Context, adminID int64) (QuotaUsage, error) {
	var limit int
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(quota_limit, 0) FROM users WHERE id = ?`, adminID).Scan(&limit)
	if errors.Is(err, sql.ErrNoRows) {
		return QuotaUsage{}, ErrNoRows
	}
	if err != nil {
		return QuotaUsage{}, fmt.Errorf("store: quota limit: %w", err)
	}

	var used int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_assignments WHERE seller_user_id = ? AND status = 'ACTIVE'`,
		adminID).Scan(&used); err != nil {
		return QuotaUsage{}, fmt.Errorf("store: quota usage: %w", err)
	}
	remaining := -1
	if limit > 0 {
		remaining = limit - used
		if remaining < 0 {
			remaining = 0
		}
	}
	return QuotaUsage{Used: used, Limit: limit, Remaining: remaining}, nil
}

// ServerRow is a server integration as returned to the API (never the token).
type ServerRow struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	PanelURL        string `json:"panel_url"`
	Status          string `json:"status"`
	PanelVersion    string `json:"panel_version"`
	XrayVersion     string `json:"xray_version"`
	LastHealthCheck *int64 `json:"last_health_check"`
	LastSync        *int64 `json:"last_sync"`
}

// ListServers returns integrations, optionally scoped to one admin's assignments.
func (s *Store) ListServers(ctx context.Context, adminID int64) ([]ServerRow, error) {
	var (
		rows *sql.Rows
		err  error
	)
	base := `SELECT s.id, s.name, s.panel_url, s.status, s.panel_version, s.xray_version,
	                s.last_health_check, s.last_sync
	           FROM server_integrations s`
	if adminID > 0 {
		rows, err = s.db.QueryContext(ctx, base+`
		  JOIN admin_server_assignments a ON a.server_id = s.id AND a.admin_id = ?
		 WHERE s.status <> 'REMOVED' ORDER BY s.name`, adminID)
	} else {
		rows, err = s.db.QueryContext(ctx, base+` WHERE s.status <> 'REMOVED' ORDER BY s.name`)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list servers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ServerRow, 0, 8)
	for rows.Next() {
		var r ServerRow
		var hc, ls sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Name, &r.PanelURL, &r.Status, &r.PanelVersion,
			&r.XrayVersion, &hc, &ls); err != nil {
			return nil, fmt.Errorf("store: scan server: %w", err)
		}
		if hc.Valid {
			v := hc.Int64
			r.LastHealthCheck = &v
		}
		if ls.Valid {
			v := ls.Int64
			r.LastSync = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ServerPanelURL returns a server's panel URL.
//
// Used by the reachability ping, which needs the host (never the token) to time
// a TCP connect. A REMOVED integration is treated as absent so a deleted server
// cannot be "pinged back to life".
func (s *Store) ServerPanelURL(ctx context.Context, id int64) (string, error) {
	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT panel_url FROM server_integrations WHERE id = ? AND status <> 'REMOVED'`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoRows
	}
	if err != nil {
		return "", fmt.Errorf("store: server panel url: %w", err)
	}
	return raw, nil
}

// HTTPStatusFor maps repository errors to HTTP codes (shared by handlers).
func HTTPStatusFor(err error) int {
	switch {
	case errors.Is(err, ErrNoRows):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// nowUnix is a test seam for time-dependent repositories.
func nowUnix() int64 { return time.Now().Unix() }
