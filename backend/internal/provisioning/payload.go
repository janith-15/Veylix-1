package provisioning

import (
	"encoding/json"
	"fmt"
	"time"

	"veylix/backend/internal/domain"
	"veylix/backend/internal/panel"
)

// buildClientRequest derives the panel client body from an offer.
//
// The field mapping itself lives in specFor (policy.go) so the device limits,
// capacity and expiry have exactly one implementation. This wrapper only adds
// the inbound the client is attached to.
func buildClientRequest(vpnID int64, panelEmail string, offer *Offer, inboundID int64) panel.AddClientRequest {
	return panel.AddClientRequest{Client: specFor(vpnID, panelEmail, offer), InboundIDs: []int{int(inboundID)}}
}

// The panel identity of a VPN is derived from its local id (D-012), so these
// two helpers keep the identity in one place for both the create body and the
// local row.

func subIDFor(vpnID int64) string { return domain.PanelSubID(vpnID) }

func commentFor(vpnID int64) string { return fmt.Sprintf("veylix:vpn:%d", vpnID) }

// expiryFor is the create-time expiry derived from the offer's duration.
func expiryFor(offer *Offer) int64 {
	if offer == nil || offer.DurationDays <= 0 {
		return 0
	}
	return time.Now().AddDate(0, 0, offer.DurationDays).UnixMilli()
}

// clientPayload is what the outbox stores so a reconciled create is identical
// to the original attempt (new_update.md §5: no silent drift).
type clientPayload struct {
	Client     panel.ClientSpec `json:"client"`
	InboundIDs []int            `json:"inbound_ids"`
}

func encodePayload(req panel.AddClientRequest) string {
	raw, err := json.Marshal(clientPayload{Client: req.Client, InboundIDs: req.InboundIDs})
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func decodePayload(s string) (clientPayload, bool) {
	var p clientPayload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return clientPayload{}, false
	}
	if p.Client.Email == "" {
		return clientPayload{}, false
	}
	return p, true
}
