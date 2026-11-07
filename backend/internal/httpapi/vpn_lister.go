package httpapi

import (
	"context"
	"database/sql"
	"fmt"

	"veylix/backend/internal/auth"
)

// SQLVPNLister implements VPNLister with role-scoped queries.
//
// The WHERE clause is chosen by the caller's role, never by a client-supplied
// id, so an admin cannot list another reseller's clients and a client sees only
// their own VPNs (REQUIREMENTS §4).
type SQLVPNLister struct {
	db *sql.DB
}

// NewSQLVPNLister builds the lister.
func NewSQLVPNLister(db *sql.DB) *SQLVPNLister { return &SQLVPNLister{db: db} }

// ListVPNs returns VPNs visible to p.
//
// Rules (D-037.4, D-032.4):
//   - a **client** sees every VPN it owns, tagged or not. The ownership filter
//     is what protects privacy, not the tag: a tag is a catalogue label, and
//     hiding a paid-for VPN because the operator never labelled it would tell
//     the customer their VPN had disappeared.
//   - a **reseller** sees the VPNs they sold, plus (per D-025.1) nothing that is
//     still unclassified — an untagged client is the platform's to publish.
//
// Before this the query inner-joined `panel_client_tags`, so ANY VPN without a
// tag row disappeared from its owner's list (M7-04).
func (l *SQLVPNLister) ListVPNs(ctx context.Context, p auth.Principal, tagFilter string) ([]VPNRow, error) {
	// tagClause narrows to one classification; empty means "any tag".
	tagClause := ""
	switch tagFilter {
	case TagFilterFree:
		tagClause = ` AND t.tag = 'FREE'`
	case TagFilterPremium:
		tagClause = ` AND t.tag = 'PREMIUM'`
	case TagFilterUntagged:
		tagClause = ` AND t.tag IS NULL`
	}

	// LEFT JOIN on purpose: a tag row is optional for the owner's own VPN.
	//
	// The assignment join is narrowed to the LIVE row (`ACTIVE`/`PENDING`), not
	// every row this VPN ever had. A transfer closes the old assignment as
	// TRANSFERRED and opens a new ACTIVE one, so joining the whole table listed
	// the same VPN twice — once per assignment — and a customer who had just
	// received a moved VPN saw it duplicated (F-<transfer>). A closed row
	// describes history, and history belongs in the transfer ledger, not in a
	// live list.
	//
	// `offer_kind` is the field every screen reads to decide Free vs Premium, so
	// it must never be empty for a VPN somebody holds (D-063). An assignment
	// created without a package has NO `vpn_offers` row, and the projection used
	// to send `''` — which is not nullish, so the UI's `offer_kind ?? tag`
	// fallback never fired and the VPN was shown as UNTAGGED even though its tag
	// said PREMIUM. The COALESCE therefore falls back through the tag and, for an
	// owned row, lands on PREMIUM rather than on "no kind at all":
	// `UNTAGGED` now means exactly what it should — a panel client nobody holds.
	base := `SELECT i.id, i.server_id, i.panel_email, i.status, i.owner_user_id,
	                COALESCE(a.seller_user_id, 0), i.expires_at, i.created_at, t.tag,
	                COALESCE(u.email, ''), COALESCE(u.display_name, ''),
	                COALESCE(i.offer_id, 0),
	                CASE
	                  WHEN COALESCE(o.kind, '') <> '' THEN o.kind
	                  WHEN COALESCE(t.tag, '') <> ''  THEN t.tag
	                  WHEN i.owner_user_id > 0        THEN 'PREMIUM'
	                  ELSE ''
	                END,
	                CASE
	                  WHEN COALESCE(o.kind, '') <> '' THEN 'offer'
	                  WHEN COALESCE(t.tag, '') <> ''  THEN 'tag'
	                  WHEN i.owner_user_id > 0        THEN 'default'
	                  ELSE 'none'
	                END,
	                COALESCE(o.name, ''),
	                COALESCE(si.name, ''),
	                COALESCE(su.display_name, ''), COALESCE(su.email, ''),
	                COALESCE(su.whatsapp, ''), COALESCE(su.telegram, '')
	           FROM vpn_instances i
	           LEFT JOIN vpn_assignments a
	                  ON a.vpn_instance_id = i.id AND a.status IN ('ACTIVE', 'PENDING')
	           LEFT JOIN panel_client_tags t
	                  ON t.server_id = i.server_id AND t.panel_email = i.panel_email
	           LEFT JOIN users u ON u.id = i.owner_user_id
	           LEFT JOIN users su ON su.id = a.seller_user_id
	           LEFT JOIN vpn_offers o ON o.id = i.offer_id
	           LEFT JOIN server_integrations si ON si.id = i.server_id`

	var (
		rows *sql.Rows
		err  error
	)
	switch p.Role {
	case auth.RoleAdmin:
		// A reseller sees what he sold, but not a VPN the Super Admin has not
		// classified yet: publishing an untagged client would leak the
		// platform's catalogue (D-025.1).
		if tagFilter == TagFilterUntagged {
			// An admin can never see an untagged VPN, so the honest answer is an
			// empty list — not an error and not the full catalogue.
			return []VPNRow{}, nil
		}
		rows, err = l.db.QueryContext(ctx, base+
			` WHERE (a.seller_user_id = ?
			         OR i.owner_user_id = ?
			         OR EXISTS (SELECT 1 FROM vpn_assignments h
			                     WHERE h.vpn_instance_id = i.id AND h.seller_user_id = ?))
			   AND t.tag IS NOT NULL`+tagClause+` ORDER BY i.id DESC`,
			p.UserID, p.UserID, p.UserID)
	default:
		// The client's own VPNs, whether or not anybody tagged them.
		rows, err = l.db.QueryContext(ctx, base+
			` WHERE i.owner_user_id = ?`+tagClause+` ORDER BY i.id DESC`, p.UserID)
	}
	if err != nil {
		return nil, fmt.Errorf("httpapi: list vpns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]VPNRow, 0, 16)
	for rows.Next() {
		var r VPNRow
		var exp sql.NullInt64
		if err := rows.Scan(&r.ID, &r.ServerID, &r.PanelEmail, &r.Status, &r.OwnerID,
			&r.SellerID, &exp, &r.CreatedAt, &r.Tag,
			&r.OwnerEmail, &r.OwnerName,
			&r.OfferID, &r.OfferKind, &r.KindSource, &r.OfferName,
			&r.ServerName, &r.SellerName, &r.SellerEmail, &r.SellerWhatsApp, &r.SellerTelegram); err != nil {
			return nil, fmt.Errorf("httpapi: scan vpn: %w", err)
		}
		if exp.Valid {
			v := exp.Int64
			r.ExpiresAt = &v
		}
		r.Source = SourceLocal
		// A local row is a VPN a Veylix account holds by definition; the panel
		// read is what can disagree, and the panel lister overwrites this.
		r.Assigned = r.OwnerID > 0
		out = append(out, r)
	}
	return out, rows.Err()
}
