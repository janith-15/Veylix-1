package provisioning

import (
	"context"
	"errors"
	"testing"

	"veylix/backend/internal/domain"
)

// D-035.2 — a CUSTOM release uses the CUSTOMER's spec, not the package's
// defaults, on the panel the Super Admin chose.
func TestSpecOverridesPackageLimits(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	// A second panel + inbound to prove the target really moves.
	res, err := h.st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('srv2', 'https://panel2.example:2053/base', x'00', 'ACTIVE', 1, 1)`)
	if err != nil {
		t.Fatalf("seed second server: %v", err)
	}
	server2, _ := res.LastInsertId()
	res, err = h.st.DB().Exec(
		`INSERT INTO inbounds (server_id, remote_id, remark, protocol, port, synced_at)
		 VALUES (?, 9, '(IN)-custom', 'vless', 8443, 1)`, server2)
	if err != nil {
		t.Fatalf("seed inbound: %v", err)
	}
	inbound2, _ := res.LastInsertId()

	got, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true,
		Spec: &Spec{TrafficGB: 200, DurationDays: 30, Devices: 3, IPs: 2, ServerID: server2, InboundID: inbound2},
	})
	if err != nil {
		t.Fatalf("Create with a spec: %v", err)
	}
	if !got.Recorded || got.Status != domain.VPNActive {
		t.Fatalf("result = %+v, want ACTIVE", got)
	}

	// The panel got the customer's numbers.
	spec := h.fake.lastSpec(t)
	if spec.TotalGB != 200*bytesPerGB {
		t.Errorf("totalGB = %d, want 200 GB", spec.TotalGB)
	}
	if spec.LimitIP != 2 || spec.LimitHwid != 3 {
		t.Errorf("limits = ip %d / hwid %d, want 2 / 3", spec.LimitIP, spec.LimitHwid)
	}

	// The VPN lives on the chosen panel, not the package's.
	var serverID int64
	if err := h.st.DB().QueryRow(
		`SELECT server_id FROM vpn_instances WHERE id = ?`, got.VPNID).Scan(&serverID); err != nil {
		t.Fatalf("read vpn: %v", err)
	}
	if serverID != server2 {
		t.Errorf("server = %d, want the chosen panel %d", serverID, server2)
	}
}

// A spec missing its traffic or target is refused before anything is written.
func TestSpecIncompleteIsRefused(t *testing.T) {
	h := newHarness(t, 5, "PREMIUM")
	ctx := context.Background()

	_, err := h.svc.Create(ctx, CreateInput{
		OfferID: h.offer, ClientUserID: h.client, Actor: superAdminPrincipal(1), Approved: true,
		Spec: &Spec{DurationDays: 30}, // no traffic, no target
	})
	if !errors.Is(err, ErrSpecIncomplete) {
		t.Fatalf("error = %v, want ErrSpecIncomplete", err)
	}
	if n := h.countVPNs(t); n != 0 {
		t.Errorf("VPNs created = %d, want 0", n)
	}
}
