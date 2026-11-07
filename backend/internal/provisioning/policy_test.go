package provisioning

import (
	"encoding/json"
	"testing"
)

// D-028.3: a FREE VPN is issued with a FIXED device cap of 2 IPs and 2
// hardware devices. The offer's device_limit is a PREMIUM setting, so a FREE
// offer that carries a different value (0, 1, 5, 100) must still produce 2/2 —
// otherwise a mis-typed offer silently hands out a free VPN with unlimited
// devices, which is both a product and a revenue defect.
func TestFreeOfferIsFixedAtTwoDevices(t *testing.T) {
	for _, offerSetting := range []int{0, 1, 5, 100, -1} {
		offer := &Offer{Kind: KindFree, TrafficGB: 10, DurationDays: 30, DeviceLimit: offerSetting}
		req := buildClientRequest(11, "vyx-11", offer, 3)

		if req.Client.LimitIP != FreeLimitIP || req.Client.LimitHwid != FreeLimitHwid {
			t.Errorf("FREE offer with device_limit=%d produced limitIp=%d limitHwid=%d, want 2/2",
				offerSetting, req.Client.LimitIP, req.Client.LimitHwid)
		}
	}
}

// A PREMIUM offer keeps the operator's choice: the Super Admin sets the caps
// per offer (D-028.4), including 0 = unlimited. Inventing a cap here would
// change what a customer bought.
func TestPremiumOfferKeepsItsDeviceLimit(t *testing.T) {
	for _, want := range []int{0, 1, 3, 10} {
		offer := &Offer{Kind: "PREMIUM", TrafficGB: 100, DurationDays: 90, DeviceLimit: want}
		req := buildClientRequest(12, "vyx-12", offer, 3)

		if req.Client.LimitIP != want || req.Client.LimitHwid != want {
			t.Errorf("PREMIUM device_limit=%d produced limitIp=%d limitHwid=%d, want %d/%d",
				want, req.Client.LimitIP, req.Client.LimitHwid, want, want)
		}
	}
}

// The FREE cap must reach the panel as an int on the wire, not merely as a
// struct field the encoder drops (the F-024 failure mode: a field that exists
// locally but is never serialised).
func TestFreeCapIsSerialised(t *testing.T) {
	req := buildClientRequest(13, "vyx-13", &Offer{Kind: KindFree, TrafficGB: 5, DeviceLimit: 99}, 1)
	raw, err := json.Marshal(req.Client)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for field, want := range map[string]float64{"limitIp": 2, "limitHwid": 2} {
		if got := body[field]; got != want {
			t.Errorf("%s on the wire = %v, want %v", field, got, want)
		}
	}
}

// The remaining create fields must not have been disturbed by routing the
// mapping through specFor: identity, capacity, expiry and the inbound.
func TestSpecStillCarriesCapacityAndIdentity(t *testing.T) {
	offer := &Offer{Kind: "PREMIUM", TrafficGB: 20, DurationDays: 0, DeviceLimit: 4}
	req := buildClientRequest(21, "vyx-21", offer, 7)

	if req.Client.Email != "vyx-21" {
		t.Errorf("email = %q, want vyx-21", req.Client.Email)
	}
	if req.Client.Comment != "veylix:vpn:21" {
		t.Errorf("comment = %q, want veylix:vpn:21", req.Client.Comment)
	}
	if req.Client.TotalGB != 20*bytesPerGB {
		t.Errorf("totalGB = %d, want %d", req.Client.TotalGB, 20*bytesPerGB)
	}
	// A zero/absent duration means "never expires" — the panel reads 0 that way.
	if req.Client.ExpiryTime != 0 {
		t.Errorf("expiryTime = %d, want 0 for an offer without a duration", req.Client.ExpiryTime)
	}
	if !req.Client.Enable {
		t.Error("a freshly created client must be enabled")
	}
	if len(req.InboundIDs) != 1 || req.InboundIDs[0] != 7 {
		t.Errorf("inboundIds = %v, want [7]", req.InboundIDs)
	}
}

// An offer with a duration must carry a future expiry, otherwise a paid VPN
// would be issued as permanent.
func TestSpecSetsExpiryFromDuration(t *testing.T) {
	req := buildClientRequest(22, "vyx-22", &Offer{Kind: "PREMIUM", TrafficGB: 1, DurationDays: 30}, 1)
	if req.Client.ExpiryTime <= 0 {
		t.Fatalf("expiryTime = %d, want a future timestamp in millis", req.Client.ExpiryTime)
	}
}
