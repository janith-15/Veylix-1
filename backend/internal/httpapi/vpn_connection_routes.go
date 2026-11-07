package httpapi

import (
	"context"
	"net/http"

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
func VPNConnectionRoutes(mux *http.ServeMux, svc VPNConnectionService,
	ping provisioning.PanelPinger, authed func(http.Handler) http.Handler) {
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
		WriteJSON(w, http.StatusOK, res)
	})))

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
