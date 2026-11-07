package provisioning

import "veylix/backend/internal/panel"

// KindFree is the offer kind that identifies a FREE VPN. It is compared against
// the offer row's kind, which is the only classification the create path trusts
// (an operator's tag is a visibility label, D-024.3).
const KindFree = "FREE"

// FREE device policy (D-028.3).
//
// A FREE VPN is fixed at 2 simultaneous IPs and 2 simultaneous hardware
// devices. This is NOT taken from the offer: an offer describes the capacity,
// duration and price the Super Admin intends, but the device cap of a free
// account is a platform rule, so a mis-set offer field cannot hand out a FREE
// VPN with an unlimited or an arbitrary device count.
//
// PREMIUM keeps using the offer's DeviceLimit, which the Super Admin chooses
// per offer (D-028.4).
const (
	FreeLimitIP   = 2
	FreeLimitHwid = 2
)

// limitsFor resolves the two device limits the panel will enforce for an offer.
//
// Both limits are returned together on purpose: the panel enforces `limitIp`
// and `limitHwid` independently, and keeping them derived from one function
// means they can never drift apart (F-024 was exactly that kind of drift — the
// hardware limit was never sent at all).
//
// A PREMIUM offer with no device limit configured keeps 0, which the panel
// reads as "unlimited". That is the existing PREMIUM behaviour and is left as
// the operator's choice rather than silently invented here.
func limitsFor(offer *Offer) (limitIP, limitHwid int) {
	if offer == nil {
		return 0, 0
	}
	if offer.Kind == KindFree {
		return FreeLimitIP, FreeLimitHwid
	}
	// A custom spec may state a different IP cap from its device cap (D-035.2),
	// so it wins when present. A package leaves IPLimit at 0 and keeps the
	// documented behaviour of one number for both.
	if offer.IPLimit > 0 {
		return offer.IPLimit, offer.DeviceLimit
	}
	return offer.DeviceLimit, offer.DeviceLimit
}

// specFor builds the writable panel client body.
//
// This is the ONLY place the mapping limit → panel field is decided, so a create
// and a reconciled create can never diverge (e.g. silently losing the quota).
// The inbound list is applied separately by buildClientRequest, so the spec can
// also be used where no inbound applies (an update replaces the whole record).
func specFor(vpnID int64, panelEmail string, offer *Offer) panel.ClientSpec {
	limitIP, limitHwid := limitsFor(offer)
	// A zero-valued offer must not panic: a caller that has no offer (or a
	// malformed row) still gets a body the panel will reject loudly rather than
	// a crash inside a background worker.
	spec := panel.ClientSpec{
		Email:     panelEmail,
		Enable:    true,
		LimitIP:   limitIP,
		LimitHwid: limitHwid,
		Security:  "auto",
		Comment:   commentFor(vpnID),
	}
	if offer == nil {
		return spec
	}
	spec.SubID = subIDFor(vpnID)
	spec.TotalGB = offer.TrafficGB * bytesPerGB
	if offer.DurationDays > 0 {
		spec.ExpiryTime = expiryFor(offer)
	}
	return spec
}
