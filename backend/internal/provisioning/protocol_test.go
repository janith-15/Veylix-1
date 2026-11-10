package provisioning

import (
	"context"
	"testing"

	"veylix/backend/internal/domain"
)

// P-09 — the protocol detail names what the customer connects with, and the
// credential comes from the PANEL, never from a local guess.
func TestProtocolDetailIsExposed(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The panel's own record for this client, as GetClient would answer.
	h.fake.mu.Lock()
	h.fake.records[res.Email] = &panelRecord{
		uuid: "5f6a1c4e-1111-2222-3333-444455556666", password: "", flow: "xtls-rprx-vision",
	}
	h.fake.mu.Unlock()

	detail, err := h.svc.Protocol(ctx, res.VPNID, adminPrincipal(h.admin))
	if err != nil {
		t.Fatalf("Protocol: %v", err)
	}
	if detail.Protocol != "vless" {
		t.Errorf("protocol = %q, want vless (from the inbound Veylix recorded)", detail.Protocol)
	}
	if detail.UUID != "5f6a1c4e-1111-2222-3333-444455556666" {
		t.Errorf("uuid = %q, want the panel's value", detail.UUID)
	}
	if detail.Flow != "xtls-rprx-vision" {
		t.Errorf("flow = %q, want the panel's value", detail.Flow)
	}
	if detail.PanelEmail != res.Email {
		t.Errorf("panel_email = %q, want %q", detail.PanelEmail, res.Email)
	}
	if detail.SubID != domain.PanelSubID(res.VPNID) {
		t.Errorf("sub_id = %q, want %q", detail.SubID, domain.PanelSubID(res.VPNID))
	}
	if detail.Source != LinkSourcePanel {
		t.Errorf("source = %q, want panel", detail.Source)
	}
	if detail.Server == "" {
		t.Error("server is empty; want host:port from the panel URL and the inbound port")
	}
}

// An unreadable panel is reported as such: the customer's VPN is fine, our read
// failed. It must never look like "this VPN has no credential".
func TestProtocolReportsOutageHonestly(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	h.fake.getErr = errPanicRead

	detail, err := h.svc.Protocol(ctx, res.VPNID, adminPrincipal(h.admin))
	if err != nil {
		t.Fatalf("Protocol must not fail on an outage: %v", err)
	}
	if detail.Source != LinkSourceUnavailable {
		t.Errorf("source = %q, want unavailable", detail.Source)
	}
	if detail.Protocol != "vless" {
		t.Errorf("protocol = %q — the inbound fact is known locally and must still be reported", detail.Protocol)
	}
}

// P-04 — the live rate is a RATE, computed from the last two snapshots.
func TestLiveTrafficRate(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Two readings 60 s apart: 60 MB up and 600 MB down is 1 MB/s and 10 MB/s.
	for _, s := range []struct {
		at       int64
		up, down int64
	}{
		{1_000, 0, 0},
		{1_060, 60 * 1024 * 1024, 600 * 1024 * 1024},
	} {
		if _, err := h.st.DB().Exec(
			`INSERT INTO usage_snapshots
			   (vpn_instance_id, taken_at, up_bytes, down_bytes, lifetime_up_bytes, lifetime_down_bytes, remote_reset_count)
			 VALUES (?, ?, ?, ?, ?, ?, 0)`, res.VPNID, s.at, s.up, s.down, s.up, s.down); err != nil {
			t.Fatalf("seed snapshot: %v", err)
		}
	}

	live, err := h.svc.LiveTraffic(ctx, res.VPNID, adminPrincipal(h.admin))
	if err != nil {
		t.Fatalf("LiveTraffic: %v", err)
	}
	if live.Source != TrafficSourceSync {
		t.Fatalf("source = %q, want sync", live.Source)
	}
	if want := int64(1024 * 1024); live.UpBps != want {
		t.Errorf("up = %d B/s, want %d (1 MB/s)", live.UpBps, want)
	}
	if want := int64(10 * 1024 * 1024); live.DownBps != want {
		t.Errorf("down = %d B/s, want %d (10 MB/s)", live.DownBps, want)
	}
	if live.MeasuredFrom != 1_000 || live.MeasuredTo != 1_060 {
		t.Errorf("measured %d→%d, want 1000→1060", live.MeasuredFrom, live.MeasuredTo)
	}
}

// One reading is a baseline, not a rate: the answer is "unavailable", never a
// zero that would claim the VPN is idle (D-026.3).
func TestLiveTrafficNeedsTwoReadings(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	res, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, SellerID: h.admin, Actor: adminPrincipal(h.admin),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := h.st.DB().Exec(
		`INSERT INTO usage_snapshots
		   (vpn_instance_id, taken_at, up_bytes, down_bytes, lifetime_up_bytes, lifetime_down_bytes, remote_reset_count)
		 VALUES (?, 1000, 500, 500, 500, 500, 0)`, res.VPNID); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	live, err := h.svc.LiveTraffic(ctx, res.VPNID, adminPrincipal(h.admin))
	if err != nil {
		t.Fatalf("LiveTraffic: %v", err)
	}
	if live.Source != TrafficSourceUnavailable {
		t.Errorf("source = %q, want unavailable with a single reading", live.Source)
	}
}
