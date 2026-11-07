package sync_test

import (
	"context"
	"errors"
	"testing"

	"veylix/backend/internal/panel"
	"veylix/backend/internal/store"
	veylixsync "veylix/backend/internal/sync"
)

// panelLimitsRow builds a panel row whose limits are what the panel reports —
// totalGB in BYTES (the panel's own unit), expiryTime in millis, both device
// caps and the enable flag.
func panelLimitsRow(email string, totalGB, expiryMillis int64, limitIP, limitHwid int, enable bool) panel.ClientRow {
	return panel.ClientRow{
		Email:      email,
		Enable:     enable,
		TotalGB:    totalGB,
		ExpiryTime: expiryMillis,
		LimitIP:    limitIP,
		LimitHwid:  limitHwid,
		Traffic: &panel.ClientTraffic{
			Email: email, Up: 10, Down: 20, ResetCount: 0, Enable: enable,
		},
	}
}

// mirrorOf reads the panel_* columns of a VPN row.
func mirrorOf(t *testing.T, st *store.Store, vpnID int64) (totalGB, expiry, limitIP, limitHwid, enable int64, syncedAt *int64) {
	t.Helper()
	var at *int64
	if err := st.DB().QueryRow(
		`SELECT panel_total_gb, panel_expiry_time, panel_limit_ip, panel_limit_hwid,
		        panel_enable, panel_synced_at
		   FROM vpn_instances WHERE id = ?`, vpnID,
	).Scan(&totalGB, &expiry, &limitIP, &limitHwid, &enable, &at); err != nil {
		t.Fatalf("read mirror: %v", err)
	}
	return totalGB, expiry, limitIP, limitHwid, enable, at
}

// D-024.2: a VPN's limits are whatever the PANEL says, not what the local row
// was created with. The owner changes a client directly in 3X-UI (10 GB → 50 GB,
// 2 devices → 5) and Veylix must show the panel's answer after one tick.
func TestSyncMirrorsPanelLimits(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	// The local row was created with different values on purpose.
	if _, err := st.DB().Exec(
		`UPDATE vpn_instances SET traffic_limit_bytes = ?, device_limit = ? WHERE id = ?`,
		10*1024*1024*1024, 2, vpnID); err != nil {
		t.Fatalf("seed: %v", err)
	}

	pf := &fakePanel{total: 1, rows: []panel.ClientRow{
		panelLimitsRow("a@panel.test", 50*1024*1024*1024, 1_800_000_000_000, 5, 3, true),
	}}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())
	res := svc.RunOnce(ctx, []int64{serverID})

	totalGB, expiry, limitIP, limitHwid, enable, syncedAt := mirrorOf(t, st, vpnID)
	if totalGB != 50*1024*1024*1024 {
		t.Errorf("panel_total_gb = %d, want the panel's 50 GiB", totalGB)
	}
	if expiry != 1_800_000_000_000 {
		t.Errorf("panel_expiry_time = %d, want the panel's value", expiry)
	}
	if limitIP != 5 || limitHwid != 3 {
		t.Errorf("limits = %d/%d, want the panel's 5/3", limitIP, limitHwid)
	}
	if enable != 1 {
		t.Errorf("panel_enable = %d, want 1", enable)
	}
	if syncedAt == nil || *syncedAt == 0 {
		t.Error("panel_synced_at was not stamped")
	}
	if res.Servers[0].Mirrored != 1 {
		t.Errorf("reported mirrored = %d, want 1", res.Servers[0].Mirrored)
	}

	// The create-time columns are history and must NOT be rewritten by sync:
	// they record what the VPN was issued with.
	var trafficLimit, deviceLimit int64
	if err := st.DB().QueryRow(
		`SELECT traffic_limit_bytes, device_limit FROM vpn_instances WHERE id = ?`, vpnID,
	).Scan(&trafficLimit, &deviceLimit); err != nil {
		t.Fatalf("read create columns: %v", err)
	}
	if trafficLimit != 10*1024*1024*1024 || deviceLimit != 2 {
		t.Errorf("create-time columns changed to %d/%d; the panel mirror must not overwrite history",
			trafficLimit, deviceLimit)
	}
}

// A steady panel costs no writes: the second tick sees identical limits and
// reports nothing mirrored, leaving `panel_synced_at` on the first change.
func TestSyncMirrorIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	pf := &fakePanel{total: 1, rows: []panel.ClientRow{
		panelLimitsRow("a@panel.test", 1<<30, 1_800_000_000_000, 2, 2, true),
	}}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())

	first := svc.RunOnce(ctx, []int64{serverID})
	if first.Servers[0].Mirrored != 1 {
		t.Fatalf("first pass mirrored = %d, want 1", first.Servers[0].Mirrored)
	}
	_, _, _, _, _, syncedAtAfterFirst := mirrorOf(t, st, vpnID)

	second := svc.RunOnce(ctx, []int64{serverID})
	if second.Servers[0].Mirrored != 0 {
		t.Errorf("second pass mirrored = %d, want 0 (nothing changed)", second.Servers[0].Mirrored)
	}
	_, _, _, _, _, syncedAtAfterSecond := mirrorOf(t, st, vpnID)
	if syncedAtAfterFirst == nil || syncedAtAfterSecond == nil ||
		*syncedAtAfterFirst != *syncedAtAfterSecond {
		t.Errorf("panel_synced_at moved on an unchanged pass: %v -> %v",
			syncedAtAfterFirst, syncedAtAfterSecond)
	}
}

// A row written before the mirror existed (panel_synced_at IS NULL) is rewritten
// once even when the numbers happen to match, because until then the values came
// from the create-time offer and are not evidence about the panel.
func TestSyncWritesMirrorOnceForLegacyRows(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	if _, _, _, _, _, at := mirrorOf(t, st, vpnID); at != nil {
		t.Fatalf("fixture should start with a NULL panel_synced_at, got %v", at)
	}

	pf := &fakePanel{total: 1, rows: []panel.ClientRow{
		panelLimitsRow("a@panel.test", 0, 0, 0, 0, true),
	}}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())
	res := svc.RunOnce(ctx, []int64{serverID})

	if res.Servers[0].Mirrored != 1 {
		t.Errorf("legacy row mirrored = %d, want 1", res.Servers[0].Mirrored)
	}
	if _, _, _, _, _, at := mirrorOf(t, st, vpnID); at == nil {
		t.Error("panel_synced_at is still NULL after a successful pass")
	}
}

// An unreadable panel must not leave a stale mirror behind: the limits are
// cleared so the UI can say "unknown" instead of showing a figure it could not
// confirm, while the VPN's own status stays exactly as it was.
func TestSyncClearsMirrorWhenVerificationFails(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	pf := &fakePanel{total: 1, rows: []panel.ClientRow{
		panelLimitsRow("a@panel.test", 1<<30, 1_700_000_000_000, 2, 2, true),
	}}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())

	if res := svc.RunOnce(ctx, []int64{serverID}); res.Servers[0].Mirrored != 1 {
		t.Fatalf("setup pass mirrored = %d, want 1", res.Servers[0].Mirrored)
	}
	if _, _, _, _, _, at := mirrorOf(t, st, vpnID); at == nil {
		t.Fatal("setup pass did not stamp the mirror")
	}

	// Now the panel lists someone else and the verifying read fails.
	pf.rows = []panel.ClientRow{panelLimitsRow("other@panel.test", 0, 0, 0, 0, true)}
	pf.existsErr = errors.New("panel: request failed: timeout")

	res := svc.RunOnce(ctx, []int64{serverID})
	if res.Servers[0].Unverified != 1 {
		t.Errorf("reported unverified = %d, want 1", res.Servers[0].Unverified)
	}

	totalGB, expiry, limitIP, limitHwid, enable, at := mirrorOf(t, st, vpnID)
	if totalGB != 0 || expiry != 0 || limitIP != 0 || limitHwid != 0 || enable != 0 || at != nil {
		t.Errorf("stale mirror survived an unreadable panel: %d/%d/%d/%d/%d synced=%v",
			totalGB, expiry, limitIP, limitHwid, enable, at)
	}

	// The VPN itself must be untouched — an outage is not evidence of anything.
	var status string
	if err := st.DB().QueryRow(`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "ACTIVE" {
		t.Errorf("status = %s, want ACTIVE — clearing the mirror must not change state", status)
	}

	// The next readable tick restores the mirror.
	pf.rows = []panel.ClientRow{panelLimitsRow("a@panel.test", 1<<30, 1_700_000_000_000, 2, 2, true)}
	pf.existsErr = nil
	if res := svc.RunOnce(ctx, []int64{serverID}); res.Servers[0].Mirrored != 1 {
		t.Errorf("recovery pass mirrored = %d, want 1", res.Servers[0].Mirrored)
	}
	if _, _, _, _, _, at := mirrorOf(t, st, vpnID); at == nil {
		t.Error("mirror was not restored by a readable pass")
	}
}
