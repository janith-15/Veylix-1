package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/integrations"
)

// AdminVPNRow is one row of the VPN management console (D-043).
//
// It is VPNRow plus the facts an operator filters on and the panel actually owns:
// usage, expiry and the panel's own enable flag. They are copied from the panel
// view at read time and never cached, so the console cannot show a number the
// panel has since changed (D-024: the panel is the authority).
type AdminVPNRow struct {
	VPNRow
	UsagePct   int    `json:"usage_pct"`
	UsedBytes  int64  `json:"used_bytes"`
	LimitBytes int64  `json:"limit_bytes"`
	Protocol   string `json:"protocol,omitempty"`
	Transport  string `json:"transport,omitempty"`
	Enabled    bool   `json:"enabled"`
	LimitIP    int    `json:"limit_ip"`
	LimitHwid  int    `json:"limit_hwid"`
	// DaysLeft is negative once expired: a caller must be able to tell "expires in
	// 3 days" from "expired 3 days ago" without doing date arithmetic.
	DaysLeft int `json:"days_left"`
	// ReadError means this VPN's panel could not be read on THIS request, so the
	// usage/expiry numbers are absent rather than zero. A zero would claim "no
	// traffic used", which is a different statement from "we could not ask".
	ReadError bool `json:"read_error"`
	// PanelMissing means the panel answered and positively does NOT hold this
	// client, while Veylix still has a row for it. Distinct from ReadError: the
	// read succeeded and the answer was "gone".
	//
	// Both exist because the console used to present a missing client with
	// ReadError=false and no panel facts, which rendered as 0% usage, disabled
	// and — by the usage filter's definition — "healthy".
	PanelMissing bool `json:"panel_missing"`
}

// AdminVPNFilter is the console's query.
type AdminVPNFilter struct {
	Tag      string // free | premium | untagged | ""
	Assigned string // "1" | "0" | ""
	ServerID int64
	OwnerID  int64
	SellerID int64
	Q        string
	Usage    string // exhausted | expiring | expired | unused | healthy | ""
	Expiry   string // "7d" | "30d" | ""
	// Window. Limit is always concrete after parsing (never 0), and Offset is
	// never negative, so the handler cannot be asked for an unbounded slice.
	Limit  int
	Offset int
}

// Pagination bounds for the console.
//
// The default page is deliberately large: an operator scanning a few dozen VPNs
// should not have to page through them. The ceiling exists because the endpoint
// reads the panel per server, and a caller asking for 100 000 rows would turn a
// dashboard refresh into a load test.
const (
	AdminVPNDefaultLimit = 100
	AdminVPNMaxLimit     = 500
)

// PageCount is the number of pages this total would fill, never less than 1.
func PageCount(total, limit int) int {
	if limit <= 0 || total <= 0 {
		return 1
	}
	return (total + limit - 1) / limit
}

// matches applies the console's filters to one row.
func (f AdminVPNFilter) matches(r AdminVPNRow) bool {
	if f.ServerID > 0 && r.ServerID != f.ServerID {
		return false
	}
	if f.OwnerID > 0 && r.OwnerID != f.OwnerID {
		return false
	}
	if f.SellerID > 0 && r.SellerID != f.SellerID {
		return false
	}
	switch f.Tag {
	case "free":
		if r.Tag == nil || *r.Tag != "FREE" {
			return false
		}
	case "premium":
		if r.Tag == nil || *r.Tag != "PREMIUM" {
			return false
		}
	case "untagged":
		if r.Tag != nil {
			return false
		}
	}
	switch f.Assigned {
	case "1", "true":
		if !r.Assigned {
			return false
		}
	case "0", "false":
		if r.Assigned {
			return false
		}
	}
	if q := strings.ToLower(strings.TrimSpace(f.Q)); q != "" {
		hay := strings.ToLower(strings.Join([]string{
			r.PanelEmail, r.OwnerEmail, r.OwnerName, r.OfferName, r.Protocol,
		}, " "))
		if !strings.Contains(hay, q) {
			return false
		}
	}
	// Usage windows come from the panel's numbers, never from a cached status
	// column, so they agree with what the panel reports right now. "Exhausted" is
	// read from the percentage the panel computed: `UsagePct >= 100` is the same
	// claim the progress bar makes, and it stays true whether or not the byte
	// fields are present on this row.
	if f.Usage != "" {
		expired := r.ExpiresAt != nil && *r.ExpiresAt > 0 && *r.ExpiresAt < time.Now().Unix()
		exhausted := r.UsagePct >= 100 || (r.LimitBytes > 0 && r.UsedBytes >= r.LimitBytes)
		switch f.Usage {
		case "exhausted":
			if !exhausted {
				return false
			}
		case "unused":
			if r.UsedBytes != 0 || r.UsagePct != 0 {
				return false
			}
			// 0% usage only means "nothing used yet" when the panel answered.
			// An unread row and a vanished client both show 0 for a different
			// reason: nobody asked successfully.
			if r.ReadError || r.PanelMissing {
				return false
			}
		case "expired":
			if !expired {
				return false
			}
		case "expiring":
			if expired || r.DaysLeft < 0 || r.DaysLeft > 7 {
				return false
			}
		case "healthy":
			// "Unknown" is not "fine": a row whose panel could not be read, or that
			// the panel no longer holds, must never appear under Healthy.
			if expired || exhausted || r.ReadError || r.PanelMissing {
				return false
			}
		}
	}
	if f.Expiry != "" {
		days := 7
		if f.Expiry == "30d" {
			days = 30
		}
		if r.ExpiresAt == nil || *r.ExpiresAt == 0 || r.DaysLeft > days {
			return false
		}
	}
	return true
}

// parseAdminVPNFilter reads and validates the query. A bad value is refused with a
// precise code instead of being ignored, so a typo never looks like "no results".
func parseAdminVPNFilter(w http.ResponseWriter, r *http.Request) (AdminVPNFilter, bool) {
	q := r.URL.Query()
	f := AdminVPNFilter{
		Tag:      strings.ToLower(strings.TrimSpace(q.Get("tag"))),
		Assigned: strings.ToLower(strings.TrimSpace(q.Get("assigned"))),
		Q:        strings.TrimSpace(q.Get("q")),
		Usage:    strings.ToLower(strings.TrimSpace(q.Get("usage"))),
		Expiry:   strings.ToLower(strings.TrimSpace(q.Get("expiry"))),
	}
	bad := func(msg string) (AdminVPNFilter, bool) {
		WriteError(w, http.StatusUnprocessableEntity, "validation_failed", msg)
		return f, false
	}
	switch f.Tag {
	case "", "free", "premium", "untagged":
	default:
		return bad("tag must be free, premium or untagged")
	}
	switch f.Assigned {
	case "", "1", "0", "true", "false":
	default:
		return bad("assigned must be 1 or 0")
	}
	switch f.Usage {
	case "", "exhausted", "expiring", "expired", "unused", "healthy":
	default:
		return bad("usage must be exhausted, expiring, expired, unused or healthy")
	}
	switch f.Expiry {
	case "", "7d", "30d":
	default:
		return bad("expiry must be 7d or 30d")
	}
	f.ServerID, _ = strconv.ParseInt(q.Get("server_id"), 10, 64)
	f.OwnerID, _ = strconv.ParseInt(q.Get("owner_id"), 10, 64)
	f.SellerID, _ = strconv.ParseInt(q.Get("seller_id"), 10, 64)

	// limit / offset. A malformed or out-of-range value is refused rather than
	// clamped silently: a pager that asked for 10 000 and received 500 would show
	// the wrong page count with no hint that anything was adjusted.
	f.Limit = AdminVPNDefaultLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > AdminVPNMaxLimit {
			return bad("limit must be a whole number between 1 and " + strconv.Itoa(AdminVPNMaxLimit))
		}
		f.Limit = n
	}
	f.Offset = 0
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return bad("offset must be zero or a positive whole number")
		}
		f.Offset = n
	}
	return f, true
}

// adminSort orders the console: UNASSIGNED first (they are the ones needing a
// decision), then the fullest, then by name — a stable, explainable order.
func adminSort(rows []AdminVPNRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Assigned != rows[j].Assigned {
			return !rows[i].Assigned
		}
		if rows[i].UsagePct != rows[j].UsagePct {
			return rows[i].UsagePct > rows[j].UsagePct
		}
		return rows[i].PanelEmail < rows[j].PanelEmail
	})
}

// AdminVPNRoutes registers the VPN management console (D-043):
//
//	GET /api/super/vpns?tag&assigned&server_id&owner_id&seller_id&q&usage&expiry
//
// It answers the owner's requirement in one place — "the system's every VPN,
// including the ones nobody has been assigned" — because the list is the panel's
// clients PLUS Veylix's local rows, merged by PanelVPNLister. An untagged or
// unassigned panel client therefore appears here even though no Veylix record
// exists for it (D-024.3, D-037).
//
// Super Admin only (PermServerViewAll): a reseller's own book is served by the
// scoped /api/vpns, and mixing the two would let a reseller see another seller's
// panel.
func AdminVPNRoutes(mux *http.ServeMux, lister adminVPNLister, reader adminPanelReader,
	authed func(http.Handler) http.Handler) {
	handler := authed(RequirePermission(auth.PermServerViewAll)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			adminVPNHandler(w, r, lister, reader)
		})))
	mux.Handle("GET /api/super/vpns", handler)
}

// adminVPNHandler is the body of the route, kept flat so each step is readable.
func adminVPNHandler(w http.ResponseWriter, r *http.Request, lister adminVPNLister, reader adminPanelReader) {
	p, ok := PrincipalFrom(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	filter, ok := parseAdminVPNFilter(w, r)
	if !ok {
		return
	}

	base, err := lister.ListVPNs(r.Context(), p, TagFilterAny)
	if err != nil {
		WriteError(w, http.StatusServiceUnavailable, "panel_unavailable",
			"no VPN list could be read: the panel did not answer")
		return
	}

	// Panel facts are read ONCE per server, not once per VPN: a server with fifty
	// clients must cost one read, not fifty.
	facts := readPanelFacts(r.Context(), base, reader)

	out := make([]AdminVPNRow, 0, len(base))
	now := time.Now().Unix()
	// The summary is computed over EVERY row, before the filter is applied.
	//
	// That is the whole point of it: the operator asks "how many untagged VPNs do
	// I have?" and then clicks that tile to see them. A summary computed from the
	// filtered slice would show `untagged: 4` and then, one click later,
	// `untagged: 4 of 4` on a page where every other count is zero — which reads
	// as "the catalogue shrank" rather than "you are looking through a window".
	all := make([]AdminVPNRow, 0, len(base))
	for _, row := range base {
		ar := enrichVPNRow(row, facts, now)
		all = append(all, ar)
		if !filter.matches(ar) {
			continue
		}
		out = append(out, ar)
	}
	adminSort(out)

	// Pagination is applied AFTER filtering and sorting, so a page is a window on
	// the real result set rather than a slice of an arbitrary order. The counts
	// describe the WHOLE filtered set: a "total" that changed with the page number
	// would be useless for the pager that consumes it.
	total := len(out)
	unread := countUnread(out)
	page := paginate(out, filter.Limit, filter.Offset)

	WriteJSON(w, http.StatusOK, map[string]any{
		"vpns":    page,
		"total":   total,
		"unread":  unread,
		"summary": summariseVPNs(all),
		// Echo the window back so a client can prove what it received instead of
		// assuming the server honoured its request.
		"limit":    filter.Limit,
		"offset":   filter.Offset,
		"has_more": filter.Offset+len(page) < total,
	})
}

// AdminVPNSummary is the At-a-glance state of the whole VPN catalogue.
//
// Every field is a number the console can turn into a one-click filter, so the
// operator stops counting rows by hand. The buckets are deliberately
// non-exclusive where the underlying facts are: a VPN can be `premium` AND
// `expiring_soon`, because those answer different questions ("what does it cost
// the customer" vs "what needs attention this week").
type AdminVPNSummary struct {
	Total   int `json:"total"`
	Free    int `json:"free"`
	Premium int `json:"premium"`
	// Untagged is the queue waiting for the Super Admin's FREE/PREMIUM decision.
	Untagged int `json:"untagged"`
	// Assigned / Unassigned answer "does a customer hold it".
	Assigned   int `json:"assigned"`
	Unassigned int `json:"unassigned"`
	// Exhausted / ExpiringSoon / Expired / Disabled are the four things that
	// generate a support ticket.
	Exhausted    int `json:"exhausted"`
	ExpiringSoon int `json:"expiring_soon"`
	Expired      int `json:"expired"`
	Disabled     int `json:"disabled"`
	// Unread means the panel could not be read for that row, so its usage and
	// expiry are unknown rather than zero.
	Unread int `json:"unread"`
}

// expiringWindowSeconds is how far ahead "expiring soon" looks.
//
// Seven days matches the `/vpns` expiry filter and the owner's own renewal
// habit, so the tile and the filter it opens always agree.
const expiringWindowSeconds = 7 * 24 * 60 * 60

// summariseVPNs counts one enriched row into every bucket it belongs to.
func summariseVPNs(rows []AdminVPNRow) AdminVPNSummary {
	s := AdminVPNSummary{Total: len(rows)}
	for _, r := range rows {
		if r.Tag == nil {
			s.Untagged++
		} else if *r.Tag == "FREE" {
			s.Free++
		} else if *r.Tag == "PREMIUM" {
			s.Premium++
		}
		if r.Assigned {
			s.Assigned++
		} else {
			s.Unassigned++
		}
		if r.ReadError {
			s.Unread++
			// A row whose panel could not be read has no trustworthy usage or
			// expiry, so counting it in any of the four buckets below would be a
			// guess dressed as a fact.
			continue
		}
		if r.DaysLeft < 0 {
			s.Expired++
		} else if r.DaysLeft > 0 && r.DaysLeft <= expiringWindowSeconds/86400 {
			s.ExpiringSoon++
		}
		if r.LimitBytes > 0 && r.UsedBytes >= r.LimitBytes {
			s.Exhausted++
		}
		// `enabled` is the panel's own flag, which is the only authority on
		// whether the client can connect (D-024). A row the panel no longer holds
		// has no enable flag to report, so it must not be counted as "disabled" —
		// that is a different fact, already covered by `unread`.
		if !r.Enabled && !r.ReadError && !r.PanelMissing {
			s.Disabled++
		}
	}
	return s
}

// paginate returns the requested window of rows.
//
// Both bounds are already normalised by parseAdminVPNFilter (limit 1..500,
// offset >= 0), so this cannot be asked for an unbounded slice.
func paginate(rows []AdminVPNRow, limit, offset int) []AdminVPNRow {
	if offset >= len(rows) {
		return []AdminVPNRow{}
	}
	end := offset + limit
	if end > len(rows) || end <= 0 {
		end = len(rows)
	}
	return rows[offset:end]
}

// readPanelFacts maps each server to its panel clients keyed by lower-cased
// e-mail. A nil map for a server means "the read failed", which is different from
// "the panel returned nothing" — see AdminVPNRow.ReadError.
func readPanelFacts(ctx context.Context, rows []VPNRow, reader adminPanelReader) map[int64]map[string]integrations.PanelClientView {
	facts := make(map[int64]map[string]integrations.PanelClientView)
	if reader == nil {
		return facts
	}
	for _, row := range rows {
		if _, done := facts[row.ServerID]; done {
			continue
		}
		views, err := reader.PanelClients(ctx, row.ServerID)
		if err != nil {
			facts[row.ServerID] = nil
			continue
		}
		idx := make(map[string]integrations.PanelClientView, len(views))
		for _, v := range views {
			idx[strings.ToLower(v.PanelEmail)] = v
		}
		facts[row.ServerID] = idx
	}
	return facts
}

// enrichVPNRow attaches the panel facts to one local/merged row.
func enrichVPNRow(row VPNRow, facts map[int64]map[string]integrations.PanelClientView, now int64) AdminVPNRow {
	ar := AdminVPNRow{VPNRow: row}
	idx, tried := facts[row.ServerID]
	if !tried {
		return ar // the inspector is not wired: no panel numbers, no error
	}
	if idx == nil {
		ar.ReadError = true
		return ar
	}
	v, found := idx[strings.ToLower(row.PanelEmail)]
	if !found {
		// The panel answered and does NOT list this client: Veylix's row is stale.
		// Flag it, so no screen can present a vanished VPN as healthy (0% usage,
		// enabled) — "the panel no longer holds it" is the fact, and it is the
		// one an operator needs to decide between verifying and removing.
		ar.PanelMissing = true
		return ar
	}
	ar.UsagePct = v.UsagePct
	ar.UsedBytes = v.UsedBytes
	ar.LimitBytes = v.LimitBytes
	ar.Protocol = v.Protocol
	ar.Transport = v.Transport
	ar.Enabled = v.Enable
	ar.LimitIP = v.LimitIP
	ar.LimitHwid = v.LimitHwid
	// DaysLeft is computed once here, so the list and the filter can never
	// disagree about what "expiring" means.
	if v.ExpiryTime > 0 {
		ar.DaysLeft = int((v.ExpiryTime - now) / 86400)
	}
	return ar
}

// countUnread reports how many rows could not be verified against the panel, so
// the UI can say "3 VPNs could not be read" instead of showing them as empty.
// A row the panel positively does not hold counts too: it is equally unverified.
func countUnread(rows []AdminVPNRow) int {
	n := 0
	for _, r := range rows {
		if r.ReadError || r.PanelMissing {
			n++
		}
	}
	return n
}

// adminVPNLister is the read surface the console needs: the Super Admin's merged
// VPN list (panel clients + local rows).
type adminVPNLister interface {
	ListVPNs(ctx context.Context, p auth.Principal, tagFilter string) ([]VPNRow, error)
}

// adminPanelReader reads one server's panel clients. It is a narrow interface so
// the console keeps working (with ReadError set) when the inspector is not wired,
// instead of refusing to render at all.
type adminPanelReader interface {
	PanelClients(ctx context.Context, id int64) ([]integrations.PanelClientView, error)
}
