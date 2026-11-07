package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/domain"
	"veylix/backend/internal/integrations"
)

// PanelVPNLister is the VPN catalogue for the Super Admin: the panel's clients
// ARE the VPN list (I-1, D-024.3), so a client created directly in 3X-UI shows
// up here without Veylix ever having recorded it.
//
// Why this is a separate type from SQLVPNLister: the two callers have opposite
// failure modes.
//
//   - The Super Admin must see the whole truth, including untagged clients, so
//     this reads the panel. It is bounded (the inspector pages 200×50) and only
//     touches ACTIVE servers.
//   - An admin/client must never be blocked by a panel outage, so their path
//     stays local-only (see SQLVPNLister).
//
// A panel that cannot be reached does NOT blank the answer: Veylix's own rows
// for that server are still listed, marked Source=local, so an outage degrades
// the view instead of emptying it.
type PanelVPNLister struct {
	db        *sql.DB
	inspector PanelInspector
}

// NewPanelVPNLister builds the Super-Admin lister. A nil inspector disables the
// panel read: the list then degrades to local rows only, still marked
// Source=local, so the caller can tell that the panel was not consulted.
func NewPanelVPNLister(db *sql.DB, inspector PanelInspector) *PanelVPNLister {
	return &PanelVPNLister{db: db, inspector: inspector}
}

// ListVPNs returns every VPN the Super Admin may see.
func (l *PanelVPNLister) ListVPNs(ctx context.Context, p auth.Principal, tagFilter string) ([]VPNRow, error) {
	// Local rows carry the ownership/assignment/created_at data the panel does
	// not know about, so they are the join target for panel clients.
	local, err := l.localRows(ctx)
	if err != nil {
		return nil, err
	}

	panelRows := make([]VPNRow, 0, len(local))
	seenPanel := make(map[string]bool, len(local)) // key: serverID|email
	readOK := make(map[int64]bool, 1)              // servers the panel answered for
	readFailed := make(map[int64]bool, 1)          // servers that could not be read
	if l.inspector != nil {
		servers, err := l.activeServers(ctx)
		if err != nil {
			return nil, err
		}
		for _, srv := range servers {
			views, perr := l.inspector.PanelClients(ctx, srv.ID)
			if perr != nil {
				readFailed[srv.ID] = true
				continue
			}
			readOK[srv.ID] = true
			for _, v := range views {
				key := vpnKey(srv.ID, v.PanelEmail)
				seenPanel[key] = true
				panelRows = append(panelRows, panelViewToRow(srv.ID, srv.Name, v, local[key]))
			}
		}
		// No server could be read at all. Two honest outcomes, and no third:
		//   - Veylix has rows → return them, every one marked source=local so
		//     the operator sees a verified-empty panel vs. an unreadable one;
		//   - Veylix has nothing → an empty list would claim "no VPNs exist",
		//     which nothing verified. Report the failure instead.
		if len(readOK) == 0 && len(readFailed) > 0 && len(local) == 0 {
			return nil, fmt.Errorf("httpapi: no panel could be read")
		}
	}

	// Local rows the panel did not return. A row on a server that FAILED to
	// read keeps Source=local (it is unverified); a row on a server that read
	// fine but did not list it is positively absent and keeps its own status.
	out := make([]VPNRow, 0, len(panelRows)+len(local))
	out = append(out, panelRows...)
	for key, r := range local {
		if seenPanel[key] {
			continue
		}
		if readFailed[r.ServerID] {
			r.Source = SourceLocal
		} else {
			// The panel answered and does NOT list this client, so Veylix's row
			// is positively absent from the panel.
			//
			// Source used to stay "local" here — the same value as "the panel
			// could not be read" — so a dead record was indistinguishable from an
			// unverified one, and the console then enriched it with no panel facts
			// at all: 0% usage, enabled=false, "healthy". Three separate lies on
			// one row. `local_missing` says exactly what was observed.
			r.Source = SourceLocalMissing
			// A working VPN keeps its lifecycle status. One the panel no longer
			// holds cannot be presented as working.
			if r.Status == string(domain.VPNActive) {
				r.Status = string(domain.VPNServerUnavailable)
			}
		}
		out = append(out, r)
	}

	return filterAndSort(out, tagFilter), nil
}

// localRows loads Veylix's own VPN records with their tag, keyed by
// server+panel_email so a panel client can be enriched with its local identity.
//
// The kind expression is the same one `SQLVPNLister` uses, and for the same
// reason (D-063): an owned row must never project an empty kind, or the client
// screen shows a VPN somebody holds as UNTAGGED.
func (l *PanelVPNLister) localRows(ctx context.Context) (map[string]VPNRow, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT i.id, i.server_id, i.panel_email, i.status, i.owner_user_id,
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
		   LEFT JOIN vpn_assignments a ON a.vpn_instance_id = i.id
		   LEFT JOIN panel_client_tags t
		     ON t.server_id = i.server_id AND t.panel_email = i.panel_email
		   LEFT JOIN users u ON u.id = i.owner_user_id
		   LEFT JOIN users su ON su.id = a.seller_user_id
		   LEFT JOIN vpn_offers o ON o.id = i.offer_id
		   LEFT JOIN server_integrations si ON si.id = i.server_id`)
	if err != nil {
		return nil, fmt.Errorf("httpapi: list local vpns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]VPNRow, 32)
	for rows.Next() {
		var r VPNRow
		var exp sql.NullInt64
		if err := rows.Scan(&r.ID, &r.ServerID, &r.PanelEmail, &r.Status, &r.OwnerID,
			&r.SellerID, &exp, &r.CreatedAt, &r.Tag,
			&r.OwnerEmail, &r.OwnerName,
			&r.OfferID, &r.OfferKind, &r.KindSource, &r.OfferName,
			&r.ServerName, &r.SellerName, &r.SellerEmail, &r.SellerWhatsApp, &r.SellerTelegram); err != nil {
			return nil, fmt.Errorf("httpapi: scan local vpn: %w", err)
		}
		if exp.Valid {
			v := exp.Int64
			r.ExpiresAt = &v
		}
		r.Source = SourceLocal
		// A local row exists because a Veylix account holds it, so it is assigned by
		// definition (D-063). Leaving this false made a VPN a customer owns report
		// ssigned: false whenever the panel could not confirm it, so the console
		// confirm it, so the console printed unassigned and ?assigned=1 returned nothing.
		r.Assigned = r.OwnerID > 0
		out[vpnKey(r.ServerID, r.PanelEmail)] = r
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("httpapi: iterate local vpns: %w", err)
	}
	return out, nil
}

// serverRef is one readable integration: its id plus the operator's label.
//
// The label travels with the id because a panel-only client — one Veylix has no
// local row for — has no other way to learn its server's name, and a bare
// `server_id` is not something a customer can recognise on a card.
type serverRef struct {
	ID   int64
	Name string
}

// activeServers lists integrations worth reading from the panel, carrying each
// one's operator label so a row can be named without a second query.
func (l *PanelVPNLister) activeServers(ctx context.Context) ([]serverRef, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT id, COALESCE(name, '') FROM server_integrations WHERE status = 'ACTIVE' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("httpapi: list active servers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]serverRef, 0, 8)
	for rows.Next() {
		var s serverRef
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, fmt.Errorf("httpapi: scan server: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("httpapi: iterate server ids: %w", err)
	}
	return out, nil
}

func vpnKey(serverID int64, email string) string {
	return fmt.Sprintf("%d|%s", serverID, email)
}

// panelViewToRow turns one panel client into an API row, borrowing the local
// identity fields when Veylix has a record for it. A client that exists only on
// the panel gets ID=0 and no owner — it is a VPN nobody has been given yet.
//
// `serverName` is the integration's own label and is set even for a panel-only
// row, because that row has no local record to borrow a name from and a card
// must not fall back to a bare `server_id` number.
func panelViewToRow(serverID int64, serverName string, v integrations.PanelClientView, local VPNRow) VPNRow {
	r := VPNRow{
		ID:         local.ID,
		ServerID:   serverID,
		PanelEmail: v.PanelEmail,
		Status:     local.Status,
		OwnerID:    local.OwnerID,
		SellerID:   local.SellerID,
		ExpiresAt:  local.ExpiresAt,
		CreatedAt:  local.CreatedAt,
		Tag:        v.Tag,
		Source:     SourcePanel,
		ServerName: serverName,
		// Identity and package facts come from the same join the server
		// drill-down uses (D-037), so the two screens can never disagree about
		// who holds a VPN.
		OwnerEmail: local.OwnerEmail,
		OwnerName:  local.OwnerName,
		// Ownership is Veylix's own record, never the panel's guess (D-063).
		//
		// The panel only knows a client is held when it can still resolve that
		// client; when the client is missing, or the panel is unreachable, it
		// answers false. Copying that over a row Veylix recorded as owned made the
		// console print "unassigned" for a VPN a customer holds, and made the
		// ?assigned=1 filter return nothing at all. A local row with an owner IS
		// assigned; the panel may only ever ADD that fact, never remove it.
		Assigned:   local.OwnerID > 0 || v.Assigned,
		OfferID:    local.OfferID,
		OfferKind:  local.OfferKind,
		OfferName:  local.OfferName,
		KindSource: local.KindSource,
		// Seller identity is local-only: the panel knows nothing about who sold a
		// client, so a panel-only row correctly carries none. Copying it here is
		// what lets a merged row keep its seller (Bible §2.3) — without it, the
		// Super Admin path would name the server but lose the seller.
		SellerName:     local.SellerName,
		SellerEmail:    local.SellerEmail,
		SellerWhatsApp: local.SellerWhatsApp,
		SellerTelegram: local.SellerTelegram,
	}
	if r.OwnerEmail == "" {
		// A panel client with no local row still names its owner when the panel
		// read resolved one — a client created directly in 3X-UI has none, so
		// this only fires for a row Veylix knows.
		r.OwnerEmail = v.OwnerEmail
		r.OwnerName = v.OwnerName
		r.Assigned = r.Assigned || v.Assigned
	}
	// The owner id can also arrive from the panel side of the join, so a row
	// enriched only by the panel still filters like one Veylix knows.
	// The > 0 guard matters: 0 means unknown, not a real user id.
	if r.OwnerID == 0 && v.OwnerUserID > 0 {
		r.OwnerID = v.OwnerUserID
	}
	if r.OwnerID > 0 {
		r.Assigned = true
	}
	// A panel-only client has no package; say so by leaving the fields empty
	// rather than inventing one.
	if v.OfferKind != "" && r.OfferKind == "" {
		r.OfferKind = v.OfferKind
		r.OfferName = v.OfferName
		r.OfferID = v.OfferID
		r.KindSource = "offer"
	}
	// A panel client nobody holds is the only row that may read as UNTAGGED.
	// Once a local row exists it has an owner, and the local projection has
	// already resolved a kind for it (D-063).
	if r.OwnerID > 0 && r.OfferKind == "" {
		r.OfferKind = "PREMIUM"
		r.KindSource = "default"
	}
	if r.Status == "" {
		// No local record: the VPN is healthy only if the panel says enabled.
		// Naming a lifecycle state Veylix never assigned would be a guess, so
		// the honest default follows the panel's own enable flag.
		if v.Enable {
			r.Status = string(domain.VPNActive)
		} else {
			r.Status = string(domain.VPNSuspended)
		}
	}
	return r
}

// filterAndSort applies the tag filter and gives a stable order.
//
// Ordering falls back to panel email when IDs tie at 0 (panel-only clients),
// so two identical requests return the same list — a UI that reorders on every
// poll looks broken.
func filterAndSort(rows []VPNRow, tagFilter string) []VPNRow {
	out := make([]VPNRow, 0, len(rows))
	for _, r := range rows {
		switch tagFilter {
		case TagFilterUntagged:
			if r.Tag != nil {
				continue
			}
		case TagFilterFree:
			if r.Tag == nil || *r.Tag != "FREE" {
				continue
			}
		case TagFilterPremium:
			if r.Tag == nil || *r.Tag != "PREMIUM" {
				continue
			}
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ServerID != out[j].ServerID {
			return out[i].ServerID < out[j].ServerID
		}
		if out[i].ID != out[j].ID && out[i].ID != 0 && out[j].ID != 0 {
			return out[i].ID < out[j].ID
		}
		return out[i].PanelEmail < out[j].PanelEmail
	})
	return out
}
