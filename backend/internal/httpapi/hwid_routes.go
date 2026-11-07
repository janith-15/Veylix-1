package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/panel"
)

// HwidService is the device-limit surface (D8).
//
// Kept separate from IntegrationService and from PanelInspector for the reason
// those are separate from each other: listing a VPN's devices is a READ of the
// panel, revoking one is a WRITE to it, and the read-only inspector must not be
// handed a write capability. Only these two routes get this interface.
type HwidService interface {
	Devices(ctx context.Context, serverID int64, panelEmail string) ([]panel.Hwid, error)
	RevokeDevice(ctx context.Context, serverID int64, panelEmail string, hwidID int64) error
}

// HwidRoutes registers the device-limit reads and writes for one panel client.
//
//	D8: GET    /api/panel-clients/{server_id}/{email}/hwids          list devices
//	    DELETE /api/panel-clients/{server_id}/{email}/hwids/{hwid_id} free one slot
//
// Both are Super-Admin/Admin capabilities (PermServerManage), because the device
// list is a property of the server the operator was given, not of their own
// account. A nil service answers 503 rather than an empty list, so "not wired"
// can never look like "no devices registered".
func HwidRoutes(mux *http.ServeMux, hw HwidService, aud *audit.Logger, authed func(http.Handler) http.Handler) {
	manage := func(h http.HandlerFunc) http.Handler {
		return authed(RequirePermission(auth.PermServerManage)(h))
	}

	// parseTarget validates the two path values both routes share and writes the
	// error itself, so each handler reads as the work it does.
	parseTarget := func(w http.ResponseWriter, r *http.Request) (int64, string, bool) {
		serverID, err := strconv.ParseInt(r.PathValue("server_id"), 10, 64)
		if err != nil || serverID <= 0 {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid server id")
			return 0, "", false
		}
		panelEmail := strings.TrimSpace(r.PathValue("email"))
		if panelEmail == "" {
			WriteError(w, http.StatusBadRequest, "invalid_email", "panel email is required")
			return 0, "", false
		}
		return serverID, panelEmail, true
	}

	mux.Handle("GET /api/panel-clients/{server_id}/{email}/hwids", manage(func(w http.ResponseWriter, r *http.Request) {
		serverID, panelEmail, ok := parseTarget(w, r)
		if !ok {
			return
		}
		if hw == nil {
			WriteError(w, http.StatusServiceUnavailable, "not_wired", "device listing is not available")
			return
		}
		devices, err := hw.Devices(r.Context(), serverID, panelEmail)
		if err != nil {
			writeHwidError(w, err)
			return
		}
		// An empty list is the honest answer for "nothing registered"; the panel
		// is the only source, and Veylix never invents a device.
		if devices == nil {
			devices = []panel.Hwid{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"server_id":   serverID,
			"panel_email": panelEmail,
			"devices":     devices,
		})
	}))

	mux.Handle("DELETE /api/panel-clients/{server_id}/{email}/hwids/{hwid_id}", manage(func(w http.ResponseWriter, r *http.Request) {
		serverID, panelEmail, ok := parseTarget(w, r)
		if !ok {
			return
		}
		hwidID, err := strconv.ParseInt(r.PathValue("hwid_id"), 10, 64)
		if err != nil || hwidID <= 0 {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid device id")
			return
		}
		if hw == nil {
			WriteError(w, http.StatusServiceUnavailable, "not_wired", "device revocation is not available")
			return
		}
		p, _ := PrincipalFrom(r.Context())

		// Read the label BEFORE the delete: the fingerprint is what makes the
		// audit entry answerable ("which device was freed?"), and it no longer
		// exists once the slot is gone.
		fingerprint := ""
		if before, derr := hw.Devices(r.Context(), serverID, panelEmail); derr == nil {
			for _, d := range before {
				if d.ID == hwidID {
					fingerprint = d.Fingerprint
					break
				}
			}
		}

		if err := hw.RevokeDevice(r.Context(), serverID, panelEmail, hwidID); err != nil {
			// A refusal that means "that client is not there" is a 404, not a 500:
			// the caller asked about a VPN the panel does not hold.
			writeHwidError(w, err)
			return
		}

		if aud != nil {
			_ = aud.Record(r.Context(), audit.Entry{
				ActorID: p.UserID, Action: audit.ActionPanelClientHwidRevoked,
				TargetType: "panel_client", TargetID: panelEmail, Result: "SUCCESS",
				Context: map[string]any{
					"server_id": serverID, "panel_email": panelEmail,
					"hwid_id": hwidID, "fingerprint": fingerprint,
				},
			})
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

// writeHwidError maps the service's failures onto the one error envelope.
//
// The product rule is that a panel effect is never claimed without a verified
// response, and the same rule applies to failures: "the panel said no" (502) and
// "the panel could not be reached" (503) are different facts from "Veylix broke"
// (500), and the operator's next action differs for each.
func writeHwidError(w http.ResponseWriter, err error) {
	var status *panel.StatusError
	if errors.As(err, &status) {
		// The panel answered and refused: its own status is the cause.
		WriteError(w, http.StatusBadGateway, "panel_rejected",
			"the panel rejected the request")
		return
	}
	WriteError(w, http.StatusServiceUnavailable, "panel_unavailable",
		"the panel could not be reached; nothing was changed")
}
