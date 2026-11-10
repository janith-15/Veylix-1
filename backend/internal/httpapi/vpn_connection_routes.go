package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/provisioning"
)

// VPNConnectionService is the links/ping surface (D-026) plus the two Premium
// page reads the owner listed: the protocol detail (P-09) and the live rate
// (P-04).
type VPNConnectionService interface {
	Links(ctx context.Context, id int64, p auth.Principal) (*provisioning.ConnectionInfo, error)
	Ping(ctx context.Context, id int64, p auth.Principal, ping provisioning.PanelPinger) (*provisioning.PingResult, error)
	// Protocol is P-09: the credential shape (vless/vmess, tcp/ws, uuid or
	// password). Ownership is checked exactly as for the links.
	Protocol(ctx context.Context, id int64, p auth.Principal) (*provisioning.ProtocolDetail, error)
	// LiveTraffic is P-04: bytes per second from the last two snapshots.
	LiveTraffic(ctx context.Context, id int64, p auth.Principal) (*provisioning.LiveTraffic, error)
}

// VPNConnectionRoutes registers the two endpoints a client needs to actually
// USE a VPN: the connection links and the reachability ping.
//
// Before this, `GET /api/vpns/{id}/links` answered 404 and the client screen
// showed a demo QR code — the product's core promise ("here is your VPN") was
// unimplemented (GAP_ANALYSIS §12). Both routes are read-only on the panel.
//
// `ping` is optional: nil leaves latency unmeasured (Reachable=false,
// LatencyMS=-1) rather than inventing a number.
//
// `samples` is optional too (D-092). When it is supplied, every probe is STORED
// so the details page can plot latency over time, and `GET
// /api/vpns/{id}/ping/history` is registered. A nil store simply leaves the
// history empty — the probe itself still answers, because failing the response
// over a history write would hide the number the customer just asked for.
func VPNConnectionRoutes(mux *http.ServeMux, svc VPNConnectionService,
	ping provisioning.PanelPinger, samples PingSampleStore,
	authed func(http.Handler) http.Handler) {
	mux.Handle("GET /api/vpns/{id}/links", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
			return
		}
		info, err := svc.Links(r.Context(), id, p)
		if err != nil {
			writeVPNError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, info)
	})))

	mux.Handle("POST /api/vpns/{id}/ping", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
			return
		}
		res, err := svc.Ping(r.Context(), id, p, ping)
		if err != nil {
			writeVPNError(w, err)
			return
		}
		// D-092: remember the probe so the details page can plot a trend. The
		// write is BEST-EFFORT by design — a storage failure must not turn a
		// successful measurement into an error the customer sees, and it must
		// not be silently reported as success either: the response below is the
		// measurement, and a missing history row only means the chart stays
		// short.
		if samples != nil && ping != nil {
			_ = samples.RecordPing(r.Context(), id, p, res.Reachable, res.LatencyMS, res.Scope)
		}
		WriteJSON(w, http.StatusOK, res)
	})))

	// GET /api/vpns/{id}/ping/history — D-092: stored reachability probes.
	//
	// Only registered when a store can answer it, so a deployment without the
	// migration keeps working and simply has no history. The window is resolved
	// by the same helper the usage routes use, and the default is 24 hours
	// because a latency chart is about "recently", not "since creation".
	if samples != nil {
		mux.Handle("GET /api/vpns/{id}/ping/history", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
				return
			}
			id, err := pathID(r)
			if err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
				return
			}
			q := r.URL.Query()
			if q.Get("range") == "" && q.Get("since") == "" {
				q.Set("range", "24h")
			}
			window, reason, ok := ResolveWindow(q, time.Now().Unix())
			if !ok {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", reason)
				return
			}
			limit := queryIntFrom(q, "limit", 120)

			// `source` (D-095) narrows to ONE measurement source. An unknown
			// value is REFUSED rather than ignored: a filter that silently
			// matched everything after a typo would tell the reader their
			// source has data when it does not.
			scope, ok := PingScopeFromQuery(q.Get("source"))
			if !ok {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
					"source must be server_reachability or all")
				return
			}

			points, err := samples.PingHistory(r.Context(), id, p, window.SinceSec, window.UntilSec, limit, scope)
			switch {
			case errors.Is(err, ErrUsageForbidden):
				WriteError(w, http.StatusForbidden, "forbidden", "this vpn is not visible to you")
				return
			case errors.Is(err, sql.ErrNoRows):
				WriteError(w, http.StatusNotFound, "not_found", "vpn not found")
				return
			case err != nil:
				WriteError(w, http.StatusInternalServerError, "internal_error", "could not read ping history")
				return
			}
			// The scope that was ACTUALLY applied travels with the data, so no
			// screen can claim a filtered chart covers every source.
			applied := scope
			if applied == "" {
				applied = "all"
			}
			WriteJSON(w, http.StatusOK, map[string]any{
				"vpn_id": id,
				"window": window,
				// The label travels with the data so no screen can present this
				// as the customer's own device latency (D-026.2).
				"scope":  provisioning.ScopeServerReachability,
				"source": applied,
				"points": points,
			})
		})))
	}

	// GET /api/vpns/{id}/protocol — P-09: what this customer connects with.
	//
	// A read: it asks the panel for the client's credential and joins Veylix's
	// record of the inbound. Ownership is the same check as the links route, so a
	// VPN you cannot see is a VPN whose credential you cannot read.
	mux.Handle("GET /api/vpns/{id}/protocol", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
			return
		}
		detail, err := svc.Protocol(r.Context(), id, p)
		if err != nil {
			writeVPNError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, detail)
	})))

	// GET /api/vpns/{id}/traffic/live — P-04: "↑ 2 kB/s / ↓ 40 kB/s".
	//
	// Computed from the last two sync snapshots, so the response says when it was
	// measured and reports "unavailable" rather than 0 when it cannot know.
	mux.Handle("GET /api/vpns/{id}/traffic/live", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
			return
		}
		live, err := svc.LiveTraffic(r.Context(), id, p)
		if err != nil {
			writeVPNError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, live)
	})))
}
