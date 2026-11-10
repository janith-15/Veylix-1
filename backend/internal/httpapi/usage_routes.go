package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/provisioning"
)

// UsageStore reads usage history and server metrics.
//
// Ownership is resolved inside the store by comparing the caller's id with the
// row owner, never by trusting a client-supplied id (REQUIREMENTS §4).
type UsageStore interface {
	VPNUsageHistory(ctx context.Context, vpnID int64, p auth.Principal, since, until, limit, bucket int64, bucketFromSpan bool) (*VPNUsage, error)
	// VPNUsageHourly is D-092: per-hour traffic DELTAS for the "usage by time"
	// chart. Same ownership rule as VPNUsageHistory — a VPN you cannot read is
	// not a VPN whose hourly buckets you can read.
	VPNUsageHourly(ctx context.Context, vpnID int64, p auth.Principal, since, until int64) ([]HourlyBucket, error)
	// VPNSpeedHistory is D-092: rates derived from consecutive stored counters.
	// It adds no table — the counters already exist in usage_snapshots.
	VPNSpeedHistory(ctx context.Context, vpnID int64, p auth.Principal, since, until int64) ([]SpeedPoint, error)
	ServerMetrics(ctx context.Context, serverID int64, p auth.Principal, since, limit int64) ([]MetricPoint, error)
	SyncStatus(ctx context.Context) ([]ServerSyncState, error)
}

// PingSampleStore persists reachability probes so the details page can plot
// latency history (D-092).
//
// It is SEPARATE from UsageStore on purpose: recording a probe is a WRITE, and
// keeping writes off the read interface means a read-only usage store can still
// be handed to every read route. `*SQLUsageStore` implements both.
type PingSampleStore interface {
	RecordPing(ctx context.Context, vpnID int64, p auth.Principal, reachable bool, latencyMS int64, scope string) error
	PingHistory(ctx context.Context, vpnID int64, p auth.Principal, since, until int64, limit int64, scope string) ([]PingPoint, error)
}

// VPNUsage is one VPN's usage history plus its totals.
type VPNUsage struct {
	VPNID        int64  `json:"vpn_id"`
	Status       string `json:"status"`
	LimitBytes   int64  `json:"limit_bytes"`
	LifetimeUp   int64  `json:"lifetime_up_bytes"`
	LifetimeDown int64  `json:"lifetime_down_bytes"`
	// LifetimeTotal is up + down, because the owner asked for "upload download
	// ha total traffic wena wenama" (D-038 P-01) — a screen must not have to add
	// them itself and risk disagreeing with the server.
	LifetimeTotal int64 `json:"lifetime_total_bytes"`
	// UsagePct is LifetimeTotal against LimitBytes (0 when the VPN is
	// unlimited), so "how far along is it" is answerable without arithmetic.
	UsagePct int `json:"usage_pct"`
	// RemainingBytes is what is left of the allowance; -1 when unlimited.
	RemainingBytes int64 `json:"remaining_bytes"`
	// CreatedAt / ExpiresAt bound the history the owner asked for: "from the day
	// the VPN was created to the day it ends".
	CreatedAt int64  `json:"created_at"`
	ExpiresAt *int64 `json:"expires_at"`
	Resets    int    `json:"remote_reset_count"`
	// Availability explains missing history honestly (REQUIREMENTS §2.7:
	// "never invent history"). Values: PARTIAL | AVAILABLE | NONE.
	Availability  string `json:"history"`
	FirstSnapshot *int64 `json:"first_snapshot_at"`
	// Window echoes the range the caller asked for, so a chart can say
	// "7 days" without guessing what the server used (D-026.2).
	Window UsageWindow  `json:"window"`
	Points []UsagePoint `json:"points"`
	// BucketSec / Bucket are what the points were ACTUALLY plotted at. For a
	// named range the route already knows them; for `all` and `custom` the
	// width is only known after the first snapshot is read, so it is reported
	// here rather than left as a misleading zero (D-096).
	BucketSec int64  `json:"bucket_sec"`
	Bucket    string `json:"bucket"`
}

// UsageWindow is the bounded range a usage read covers.
type UsageWindow struct {
	// Key is the caller's own label: 24h, 7d, 30d, 3m, 1y, all or custom.
	Key      string `json:"key"`
	SinceSec int64  `json:"since_sec"`
	UntilSec int64  `json:"until_sec"`
	// BucketSec is the width of one plotted point, chosen so the window always
	// lands in roughly the same number of points. It is reported to the client
	// because a chart that cannot name its own bucket cannot honestly label its
	// own axis (D-096).
	BucketSec int64 `json:"bucket_sec"`
	// Bucket is BucketSec in words: hour, day, week, month. The UI labels an
	// axis with this instead of inventing one.
	Bucket string `json:"bucket"`
}

// Bucket widths. A 30-day request that returned per-minute rows used to plot
// whatever happened to be newest; these widths make each range a different
// shape of the SAME window, which is what the range selector promises.
const (
	bucketHour  = int64(3600)
	bucketDay   = int64(86400)
	bucketWeek  = int64(7 * 86400)
	bucketMonth = int64(30 * 86400)
)

// bucketFor returns the plotting width and its name for a window.
//
// The target is 24-40 points per chart: dense enough to show a trend, sparse
// enough that a 1 vCPU box renders it without a second thought (D-096).
//
//   - 24h  -> hourly  (24 points)
//   - 7d   -> 6-hourly (28 points)
//   - 30d  -> daily   (30 points)
//   - 3m   -> weekly  (13 points)
//   - 1y   -> monthly (12 points)
//   - all  -> sized from the real span, never an unbounded read
func bucketFor(key string, since, until int64) (int64, string) {
	switch key {
	case "24h":
		return bucketHour, "hour"
	case "7d":
		return 6 * bucketHour, "6h"
	case "30d":
		return bucketDay, "day"
	case "3m":
		return bucketWeek, "week"
	case "1y":
		return bucketMonth, "month"
	}
	// `all` and `custom` have no fixed width, so derive one: a custom window
	// must not turn into a row-per-snapshot read just because the dates are
	// wide. Any span longer than a year reads monthly.
	if since <= 0 || until <= since {
		return bucketDay, "day"
	}
	switch span := until - since; {
	case span <= 2*bucketDay:
		return bucketHour, "hour"
	case span <= 8*bucketDay:
		return 6 * bucketHour, "6h"
	case span <= 60*bucketDay:
		return bucketDay, "day"
	case span <= 400*bucketDay:
		return bucketWeek, "week"
	default:
		return bucketMonth, "month"
	}
}

// UsagePoint is one time-stamped observation.
type UsagePoint struct {
	TakenAt           int64 `json:"taken_at"`
	UpBytes           int64 `json:"up_bytes"`
	DownBytes         int64 `json:"down_bytes"`
	TotalBytes        int64 `json:"total_bytes"`
	LifetimeUpBytes   int64 `json:"lifetime_up_bytes"`
	LifetimeDownBytes int64 `json:"lifetime_down_bytes"`
	LifetimeTotal     int64 `json:"lifetime_total_bytes"`
}

// HourlyBucket is one hour of traffic movement (D-092).
//
// It is a DELTA, not a running total: `total_bytes` is what moved during that
// hour, which is the only shape a "usage by time of day" chart can honestly
// plot. The lifetime counters live on UsagePoint for the cumulative view.
//
// `hour_start` is the UTC epoch second the bucket begins at, so the reader can
// label it without knowing the server's clock. `measurements` is how many sync
// snapshots landed in the hour — 1 means the delta covers a single observation
// and is therefore a coarse estimate, which the UI is allowed to say.
type HourlyBucket struct {
	HourStart    int64 `json:"hour_start"`
	HourOfDay    int   `json:"hour_of_day"` // 0-23, the reader's own label
	UpBytes      int64 `json:"up_bytes"`
	DownBytes    int64 `json:"down_bytes"`
	TotalBytes   int64 `json:"total_bytes"`
	Measurements int   `json:"measurements"`
}

// SpeedPoint is one measured rate between two consecutive sync snapshots
// (D-092 · Connection Performance).
//
// The rate is DERIVED FROM STORED COUNTERS inside SQL — no new table, because
// the counters and their timestamps already exist in `usage_snapshots`. It is
// still the server's arithmetic, not the browser's, so a chart cannot disagree
// with the API (D-038 P-01).
//
// `source` mirrors the live-rate endpoint: an interval whose counters went
// backwards is a panel counter reset, and it is reported as `unavailable`
// rather than as a negative speed.
type SpeedPoint struct {
	TakenAt    int64  `json:"taken_at"`
	UpBps      int64  `json:"up_bps"`
	DownBps    int64  `json:"down_bps"`
	ElapsedSec int64  `json:"elapsed_sec"`
	Source     string `json:"source"` // measured | unavailable
}

// PingPoint is one stored reachability probe (D-092).
//
// `latency_ms` is -1 when the probe failed. A failed probe is a MEASUREMENT,
// not an absence of one, which is why it is stored and returned rather than
// dropped.
type PingPoint struct {
	TakenAt   int64  `json:"taken_at"`
	Reachable bool   `json:"reachable"`
	LatencyMs int64  `json:"latency_ms"`
	Scope     string `json:"scope"`
}

// MetricPoint is one server-wide sample.
type MetricPoint struct {
	TakenAt  int64   `json:"taken_at"`
	CPUPct   float64 `json:"cpu_pct"`
	MemUsed  int64   `json:"mem_used"`
	MemTotal int64   `json:"mem_total"`
}

// ServerSyncState reports freshness per server.
type ServerSyncState struct {
	ServerID int64  `json:"server_id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	LastSync *int64 `json:"last_sync"`
	Stale    bool   `json:"stale"`
	AgeSecs  int64  `json:"age_seconds"`
}

// ErrUsageForbidden is returned when a caller may not see the resource.
var ErrUsageForbidden = errors.New("httpapi: usage not visible to this caller")

// UsageWindowFromQuery resolves `?range=` into a bounded window (D-038 P-06).
//
// The ranges the owner named are 24 hours, 7 days, 30 days, 3 months and
// 1 year; `all` means "since the first snapshot". An unknown value is refused
// rather than defaulted, because a chart that silently shows 30 days when the
// customer asked for a year is a lie about its own window (D-026.2).
//
// `24h` was added by D-092 for the same reason `7d` exists: a customer with
// ~15-minute sync snapshots can have several MB of movement today and 2 GB over
// the month, and a 24-hour view is the only one that answers "is something
// happening right now" without a second endpoint.
func UsageWindowFromQuery(raw string, now int64) (UsageWindow, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	switch key {
	case "":
		key = "30d"
	}
	const (
		day  = int64(86400)
		hour = int64(3600)
	)
	w := UsageWindow{Key: key}
	switch key {
	case "24h":
		w.SinceSec, w.UntilSec = now-24*hour, now
	case "7d":
		w.SinceSec, w.UntilSec = now-7*day, now
	case "30d":
		w.SinceSec, w.UntilSec = now-30*day, now
	case "3m":
		w.SinceSec, w.UntilSec = now-90*day, now
	case "1y":
		w.SinceSec, w.UntilSec = now-365*day, now
	case "all":
		// 0 means "no lower bound": the store then reads from the first
		// snapshot, which is the VPN's creation day (P-01).
		w.SinceSec, w.UntilSec = 0, now
	case "custom":
		// The bounds arrive as `?since=` / `?until=` and are validated by
		// ResolveWindow; the range key alone carries no dates.
		w.SinceSec, w.UntilSec = 0, now
	default:
		return UsageWindow{}, false
	}
	// `all` has no lower bound yet, so its width is resolved later from the
	// real first snapshot rather than guessed here (D-096).
	if w.SinceSec > 0 {
		w.BucketSec, w.Bucket = bucketFor(key, w.SinceSec, w.UntilSec)
	} else {
		w.Bucket = "day"
	}
	return w, true
}

// ResolveWindow turns the query string into the concrete window every usage
// read uses, applying the custom bounds on top of the named range (D-092).
//
// The rules, stated once so `/usage`, `/usage/hourly` and `/speed-history`
// cannot drift apart:
//
//   - `range` must be one of the six the backend knows (422 otherwise).
//   - `since` overrides the range's lower bound when it is positive.
//   - `until` overrides the range's upper bound when it is positive; a `until`
//     in the future is clamped to now, because a window cannot contain traffic
//     that has not been recorded yet.
//   - `since >= until` is refused. An inverted window would otherwise return a
//     silently empty chart, which reads as "no traffic" instead of "your dates
//     are backwards".
//
// It returns a `reason` for the refusal so the handler can answer 422 with the
// actual defect rather than a generic message.
func ResolveWindow(q url.Values, now int64) (UsageWindow, string, bool) {
	rawRange := q.Get("range")
	w, ok := UsageWindowFromQuery(rawRange, now)
	if !ok {
		return UsageWindow{}, "range must be 24h, 7d, 30d, 3m, 1y, all or custom", false
	}

	since := queryIntFrom(q, "since", 0)
	until := queryIntFrom(q, "until", 0)
	if until > now {
		until = now
	}

	switch {
	case since > 0 && until > 0:
		// A genuine custom window: both edges are the caller's.
		w.Key = "custom"
		w.SinceSec = since
		w.UntilSec = until
	case since > 0:
		// One edge moved. This stays a NAMED range with its lower bound
		// replaced, which is what "the last N days from this moment" means. It
		// is not a custom window: demanding an `until` the caller never asked
		// for would refuse a request that is completely well defined.
		w.SinceSec = since
	case until > 0:
		w.UntilSec = until
	case strings.EqualFold(strings.TrimSpace(rawRange), "custom"):
		// The caller named a custom window but gave no edges to bound it.
		return UsageWindow{}, "a custom window needs since and/or until", false
	}

	if w.SinceSec >= w.UntilSec {
		return UsageWindow{}, "since must be earlier than until", false
	}
	// Re-derive the bucket width once the edges are final. A `since` override
	// can turn a named range into a different span, and `all` starts with no
	// lower bound at all, so the width has to be recomputed here rather than
	// trusted from the range key alone (D-096). When the lower edge is unknown
	// the store resolves it from the first snapshot and reports the width back.
	if w.SinceSec > 0 {
		w.BucketSec, w.Bucket = bucketFor(w.Key, w.SinceSec, w.UntilSec)
	}
	return w, "", true
}

// PingScopeFromQuery resolves the `source` filter for ping history (D-095).
//
// The product today records exactly ONE measurement source: the SERVER's
// reachability (`ping_samples.scope = 'server_reachability'`, D-026.1/.2). The
// filter exists so a future client-side probe can be shown separately instead of
// being silently merged into the same line — mixing two different measurements
// would make the chart compare unlike things.
//
// An unknown value is REFUSED (`ok=false`), never defaulted: a typo that quietly
// returned everything would look like "this source has data" when it does not.
// The empty value means "all sources", which is the pre-filter behaviour.
func PingScopeFromQuery(raw string) (scope string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return "", true
	case provisioning.ScopeServerReachability:
		return provisioning.ScopeServerReachability, true
	default:
		return "", false
	}
}

// queryInt reads a bounded non-negative integer query parameter.
func queryInt(r *http.Request, key string, def int64) int64 {
	return queryIntFrom(r.URL.Query(), key, def)
}

// queryIntFrom is queryInt against a plain url.Values, so ResolveWindow can be
// unit-tested without a live request.
func queryIntFrom(q url.Values, key string, def int64) int64 {
	v := q.Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return def
	}
	return n
}

// UsageRoutes registers the read-only usage/metrics endpoints.
//
// Every route here is a READ: the sync layer is the only writer, and it only
// writes locally. Nothing in this file can change a panel.
func UsageRoutes(mux *http.ServeMux, st UsageStore, staleAfter time.Duration,
	authed func(http.Handler) http.Handler) {

	// GET /api/vpns/{id}/usage — own VPN for a client, seller scope for an admin.
	mux.Handle("GET /api/vpns/{id}/usage", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			WriteError(w, http.StatusBadRequest, "invalid_id", "vpn id must be a positive integer")
			return
		}
		// The owner is NOT taken from the query string — the store resolves it
		// from the authenticated principal.
		//
		// `?range=24h|7d|30d|3m|1y|all` is the switch the owner asked for
		// (P-06, extended by D-092), and `?since=&until=` is the custom window.
		// An unknown value or an inverted window answers 422 rather than
		// silently showing a different window than the one asked for.
		window, reason, ok := ResolveWindow(r.URL.Query(), time.Now().Unix())
		if !ok {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", reason)
			return
		}
		since := window.SinceSec
		limit := queryInt(r, "limit", 240)

		usage, err := st.VPNUsageHistory(r.Context(), id, p, since, window.UntilSec,
			limit, window.BucketSec, window.BucketSec == 0)
		switch {
		case errors.Is(err, ErrUsageForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", "this vpn is not visible to you")
			return
		case errors.Is(err, sql.ErrNoRows):
			WriteError(w, http.StatusNotFound, "not_found", "vpn not found")
			return
		case err != nil:
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read usage history")
			return
		}
		// The bucket the points were really plotted at wins over the route's
		// pre-read guess: for `all`/`custom` the width is only knowable after
		// the first snapshot is read, and a chart that labels itself from the
		// guess would be lying about its own shape (D-096).
		if usage.BucketSec > 0 {
			window.BucketSec = usage.BucketSec
			if usage.Bucket != "" {
				window.Bucket = usage.Bucket
			}
		}
		usage.Window = window
		// `?summary=1` answers the CARD's question only: how much has been used
		// and how much is left. The history points are what makes this response
		// several kilobytes, and a dashboard drawing six cards would pay that six
		// times over on a 1 vCPU host. The full history stays the default so no
		// existing caller changes behaviour (D-068).
		if r.URL.Query().Get("summary") == "1" {
			WriteJSON(w, http.StatusOK, map[string]any{
				"vpn_id":               usage.VPNID,
				"status":               usage.Status,
				"limit_bytes":          usage.LimitBytes,
				"lifetime_total_bytes": usage.LifetimeTotal,
				"usage_pct":            usage.UsagePct,
				"remaining_bytes":      usage.RemainingBytes,
				"expires_at":           usage.ExpiresAt,
				"point_count":          len(usage.Points),
			})
			return
		}
		WriteJSON(w, http.StatusOK, usage)
	})))

	// GET /api/servers/{id}/metrics — super admin, or an admin assigned to it.
	mux.Handle("GET /api/servers/{id}/metrics", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			WriteError(w, http.StatusBadRequest, "invalid_id", "server id must be a positive integer")
			return
		}
		since := queryInt(r, "since", 0)
		limit := queryInt(r, "limit", 240)

		points, err := st.ServerMetrics(r.Context(), id, p, since, limit)
		switch {
		case errors.Is(err, ErrUsageForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", "this server is not visible to you")
			return
		case errors.Is(err, sql.ErrNoRows):
			WriteError(w, http.StatusNotFound, "not_found", "server not found")
			return
		case err != nil:
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read server metrics")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			// Labelled so the UI can never present these as VPN traffic
			// (REQUIREMENTS §2.7: distinguish client traffic from server traffic).
			"scope":  "server_wide",
			"points": points,
		})
	})))

	// GET /api/sync/status — freshness per server, so the UI can show "stale".
	mux.Handle("GET /api/sync/status", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		// A client has no business knowing the server fleet.
		if p.IsClient() {
			WriteError(w, http.StatusForbidden, "forbidden", "not available for this role")
			return
		}
		states, err := st.SyncStatus(r.Context())
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read sync status")
			return
		}
		staleSecs := int64(staleAfter / time.Second)
		now := time.Now().Unix()
		for i := range states {
			if states[i].LastSync == nil {
				states[i].Stale = true
				continue
			}
			states[i].AgeSecs = now - *states[i].LastSync
			states[i].Stale = states[i].AgeSecs > staleSecs
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"stale_after_seconds": staleSecs,
			"servers":             states,
		})
	})))

	// GET /api/vpns/{id}/usage/hourly — D-092: traffic per hour, for the
	// "usage by time" chart (morning / afternoon / evening / night).
	//
	// The buckets are DELTAS between consecutive stored counters, grouped by
	// hour. The window is the one `/usage` resolves, so changing the range on the
	// page moves both charts together instead of leaving one showing a different
	// period.
	mux.Handle("GET /api/vpns/{id}/usage/hourly", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			WriteError(w, http.StatusBadRequest, "invalid_id", "vpn id must be a positive integer")
			return
		}
		window, reason, ok := ResolveWindow(r.URL.Query(), time.Now().Unix())
		if !ok {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", reason)
			return
		}
		buckets, err := st.VPNUsageHourly(r.Context(), id, p, window.SinceSec, window.UntilSec)
		switch {
		case errors.Is(err, ErrUsageForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", "this vpn is not visible to you")
			return
		case errors.Is(err, sql.ErrNoRows):
			WriteError(w, http.StatusNotFound, "not_found", "vpn not found")
			return
		case err != nil:
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read hourly usage")
			return
		}
		// An empty array means "no measurement in this window", which is not the
		// same as a zero bar, so the source travels with the data.
		WriteJSON(w, http.StatusOK, map[string]any{
			"vpn_id":  id,
			"window":  window,
			"source":  "panel sync snapshots",
			"buckets": buckets,
		})
	})))

	// GET /api/vpns/{id}/speed-history — D-092: measured rates over time.
	//
	// Derived from the same counters `/traffic/live` uses, but as a series
	// instead of the last interval. An interval the counters cannot describe (a
	// reset, or two readings in the same second) is returned with
	// `source: "unavailable"` rather than a 0 that would read as idle.
	mux.Handle("GET /api/vpns/{id}/speed-history", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			WriteError(w, http.StatusBadRequest, "invalid_id", "vpn id must be a positive integer")
			return
		}
		window, reason, ok := ResolveWindow(r.URL.Query(), time.Now().Unix())
		if !ok {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", reason)
			return
		}
		points, err := st.VPNSpeedHistory(r.Context(), id, p, window.SinceSec, window.UntilSec)
		switch {
		case errors.Is(err, ErrUsageForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", "this vpn is not visible to you")
			return
		case errors.Is(err, sql.ErrNoRows):
			WriteError(w, http.StatusNotFound, "not_found", "vpn not found")
			return
		case err != nil:
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read speed history")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"vpn_id": id,
			"window": window,
			// Said explicitly: a rate here is an AVERAGE between two sync
			// readings, not a live speed test (D-026.2).
			"scope":  "average_between_sync_readings",
			"points": points,
		})
	})))
}
