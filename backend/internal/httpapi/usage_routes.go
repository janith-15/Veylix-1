package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"veylix/backend/internal/auth"
)

// UsageStore reads usage history and server metrics.
//
// Ownership is resolved inside the store by comparing the caller's id with the
// row owner, never by trusting a client-supplied id (REQUIREMENTS §4).
type UsageStore interface {
	VPNUsageHistory(ctx context.Context, vpnID int64, p auth.Principal, since, limit int64) (*VPNUsage, error)
	ServerMetrics(ctx context.Context, serverID int64, p auth.Principal, since, limit int64) ([]MetricPoint, error)
	SyncStatus(ctx context.Context) ([]ServerSyncState, error)
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
}

// UsageWindow is the bounded range a usage read covers.
type UsageWindow struct {
	// Key is the caller's own label: 7d, 30d, 3m, 1y or all.
	Key      string `json:"key"`
	SinceSec int64  `json:"since_sec"`
	UntilSec int64  `json:"until_sec"`
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
// The four ranges the owner named are 7 days, 30 days, 3 months and 1 year;
// `all` means "since the first snapshot". An unknown value is refused rather
// than defaulted, because a chart that silently shows 30 days when the customer
// asked for a year is a lie about its own window (D-026.2).
func UsageWindowFromQuery(raw string, now int64) (UsageWindow, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	switch key {
	case "":
		key = "30d"
	}
	const day = int64(86400)
	switch key {
	case "7d":
		return UsageWindow{Key: key, SinceSec: now - 7*day, UntilSec: now}, true
	case "30d":
		return UsageWindow{Key: key, SinceSec: now - 30*day, UntilSec: now}, true
	case "3m":
		return UsageWindow{Key: key, SinceSec: now - 90*day, UntilSec: now}, true
	case "1y":
		return UsageWindow{Key: key, SinceSec: now - 365*day, UntilSec: now}, true
	case "all":
		// 0 means "no lower bound": the store then reads from the first
		// snapshot, which is the VPN's creation day (P-01).
		return UsageWindow{Key: key, SinceSec: 0, UntilSec: now}, true
	default:
		return UsageWindow{}, false
	}
}

// queryInt reads a bounded non-negative integer query parameter.
func queryInt(r *http.Request, key string, def int64) int64 {
	v := r.URL.Query().Get(key)
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
		// `?range=7d|30d|3m|1y|all` is the switch the owner asked for (P-06); an
		// unknown value answers 422 rather than silently showing a different
		// window than the one asked for.
		window, ok := UsageWindowFromQuery(r.URL.Query().Get("range"), time.Now().Unix())
		if !ok {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
				"range must be 7d, 30d, 3m, 1y or all")
			return
		}
		since := window.SinceSec
		if v := queryInt(r, "since", 0); v > 0 {
			since = v
		}
		limit := queryInt(r, "limit", 240)

		usage, err := st.VPNUsageHistory(r.Context(), id, p, since, limit)
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
}
