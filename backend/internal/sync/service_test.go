package sync_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"veylix/backend/internal/panel"
	"veylix/backend/internal/store"
	veylixsync "veylix/backend/internal/sync"
)

// newEnv opens a migrated database seeded with one server and one VPN.
func newEnv(t *testing.T) (*store.Store, int64, int64) {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, t.TempDir()+"/sync.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// The migration set must have applied with nothing pending. Assert a floor
	// rather than an exact count: an exact number breaks on every new migration
	// (this one had to be edited when 0006 landed), which trains people to bump
	// a magic constant instead of reading the failure.
	if v, err := st.SchemaVersion(ctx); err != nil || v < 6 {
		t.Fatalf("schema_version = %d (%v), want at least 6", v, err)
	}

	db := st.DB()
	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := db.Exec(q, args...)
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	ownerID := exec(`INSERT INTO users (email, password_hash, display_name, role, status, created_at, updated_at)
		VALUES ('c@example.com', 'x', 'C', 'client', 'ACTIVE', 1, 1)`)
	serverID := exec(`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv', 'https://panel.example/base', x'00', 'ACTIVE', 1, 1)`)
	vpnID := exec(`INSERT INTO vpn_instances
		(owner_user_id, server_id, panel_email, panel_sub_id, status, traffic_limit_bytes, created_at, updated_at)
		VALUES (?, ?, 'a@panel.test', 'sub1', 'ACTIVE', 0, 1, 1)`, ownerID, serverID)

	return st, serverID, vpnID
}

// snapshotCount returns how many history rows exist for a VPN.
func snapshotCount(t *testing.T, st *store.Store, vpnID int64) int {
	t.Helper()
	var n int
	if err := st.DB().QueryRow(
		`SELECT COUNT(*) FROM usage_snapshots WHERE vpn_instance_id = ?`, vpnID).Scan(&n); err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	return n
}

// lifetimeOf reads the stored monotonic totals.
func lifetimeOf(t *testing.T, st *store.Store, vpnID int64) (up, down int64, resets int) {
	t.Helper()
	if err := st.DB().QueryRow(
		`SELECT lifetime_up_bytes, lifetime_down_bytes, last_remote_reset_count
		   FROM vpn_instances WHERE id = ?`, vpnID).Scan(&up, &down, &resets); err != nil {
		t.Fatalf("read lifetime: %v", err)
	}
	return up, down, resets
}

// itoa avoids importing strconv just for a test label.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf []byte
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	return string(buf)
}

// TestSyncRecordsSnapshotAndLifetime is the happy path: one read, one snapshot,
// and the lifetime total matches the panel counters.
func TestSyncRecordsSnapshotAndLifetime(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	pf := &fakePanel{
		total:  1,
		rows:   []panel.ClientRow{clientRow("a@panel.test", 100, 400, 0, true)},
		health: panel.HealthResult{CPUPct: 12.5, MemCurrent: 100, MemTotal: 400},
	}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())

	res := svc.RunOnce(ctx, []int64{serverID})
	if len(res.Servers) != 1 || res.Servers[0].Err != nil {
		t.Fatalf("RunOnce = %+v, want one clean server", res.Servers)
	}
	if got := res.Servers[0].Snapshots; got != 1 {
		t.Errorf("snapshots = %d, want 1", got)
	}
	if n := snapshotCount(t, st, vpnID); n != 1 {
		t.Fatalf("history rows = %d, want 1", n)
	}
	up, down, _ := lifetimeOf(t, st, vpnID)
	if up != 100 || down != 400 {
		t.Errorf("lifetime = (%d,%d), want (100,400)", up, down)
	}

	// last_sync and server metrics were stamped.
	var lastSync *int64
	if err := st.DB().QueryRow(`SELECT last_sync FROM server_integrations WHERE id = ?`, serverID).
		Scan(&lastSync); err != nil || lastSync == nil {
		t.Fatalf("last_sync not stamped (%v)", err)
	}
	var metrics int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM server_metrics WHERE server_id = ?`, serverID).
		Scan(&metrics); err != nil || metrics != 1 {
		t.Errorf("server metrics = %d (%v), want 1", metrics, err)
	}
}

// TestSyncAccumulatesAcrossPasses proves the delta is relative to the previous
// snapshot, not to the process start.
func TestSyncAccumulatesAcrossPasses(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	pf := &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("a@panel.test", 100, 400, 0, true)}}

	// Advance the clock between passes so each is a distinct tick (snapshots have
	// second precision; two passes in the same second deliberately collapse).
	tick := time.Unix(1_700_000_000, 0)
	cfg := veylixsync.DefaultConfig()
	cfg.Now = func() time.Time { return tick }
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), cfg)

	svc.RunOnce(ctx, []int64{serverID})

	// The panel reports more usage next tick.
	pf.rows = []panel.ClientRow{clientRow("a@panel.test", 300, 900, 0, true)}
	tick = tick.Add(5 * time.Minute)
	svc.RunOnce(ctx, []int64{serverID})

	up, down, _ := lifetimeOf(t, st, vpnID)
	if up != 300 || down != 900 {
		t.Fatalf("lifetime = (%d,%d), want (300,900)", up, down)
	}
	if n := snapshotCount(t, st, vpnID); n != 2 {
		t.Errorf("history rows = %d, want 2 (one per tick)", n)
	}
}

// TestSyncSurvivesRemoteResetEndToEnd is the F-010 integration test: a real
// panel reset must not destroy the stored history.
func TestSyncSurvivesRemoteResetEndToEnd(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	pf := &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("a@panel.test", 5000, 50000, 0, true)}}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())
	svc.RunOnce(ctx, []int64{serverID})

	beforeUp, beforeDown, _ := lifetimeOf(t, st, vpnID)

	// Remote reset: counters zeroed, resetCount bumped.
	pf.rows = []panel.ClientRow{clientRow("a@panel.test", 0, 0, 1, true)}
	res := svc.RunOnce(ctx, []int64{serverID})

	afterUp, afterDown, resets := lifetimeOf(t, st, vpnID)
	if afterUp < beforeUp || afterDown < beforeDown {
		t.Fatalf("history destroyed by reset: (%d,%d) → (%d,%d)", beforeUp, beforeDown, afterUp, afterDown)
	}
	if resets != 1 {
		t.Errorf("stored reset count = %d, want 1", resets)
	}
	if res.Servers[0].Resets != 1 {
		t.Errorf("reported resets = %d, want 1", res.Servers[0].Resets)
	}

	// Usage after the reset is still counted on top.
	pf.rows = []panel.ClientRow{clientRow("a@panel.test", 100, 200, 1, true)}
	svc.RunOnce(ctx, []int64{serverID})
	finalUp, finalDown, _ := lifetimeOf(t, st, vpnID)
	if finalUp != beforeUp+100 || finalDown != beforeDown+200 {
		t.Errorf("post-reset lifetime = (%d,%d), want (%d,%d)",
			finalUp, finalDown, beforeUp+100, beforeDown+200)
	}
}

// TestSyncIsolatesAFailingServer is the "one slow panel must not block the
// dashboard" guarantee.
func TestSyncIsolatesAFailingServer(t *testing.T) {
	ctx := context.Background()
	st, good, _ := newEnv(t)

	pf := &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("a@panel.test", 1, 2, 0, true)}}
	svc := veylixsync.New(st.DB(), st, func(_ context.Context, serverID int64) (veylixsync.Panel, error) {
		if serverID == 999 {
			return nil, errors.New("panel offline")
		}
		return pf, nil
	}, veylixsync.DefaultConfig())

	res := svc.RunOnce(ctx, []int64{999, good})
	if len(res.Servers) != 2 {
		t.Fatalf("results = %d, want 2", len(res.Servers))
	}
	// Results keep their input positions.
	if res.Servers[0].ServerID != 999 || res.Servers[0].Err == nil {
		t.Errorf("failing server result = %+v, want an error", res.Servers[0])
	}
	if res.Servers[1].ServerID != good || res.Servers[1].Err != nil {
		t.Fatalf("healthy server was affected by the broken one: %+v", res.Servers[1])
	}
	if res.Servers[1].Snapshots != 1 {
		t.Errorf("healthy server snapshots = %d, want 1", res.Servers[1].Snapshots)
	}
}

// TestSyncFailureNeverRevokes is the REQUIREMENTS §7 regression test: an
// unreadable panel must not be interpreted as "every client is gone".
func TestSyncFailureNeverRevokes(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	svc := veylixsync.New(st.DB(), st, failingFactory(errors.New("connection refused")),
		veylixsync.DefaultConfig())
	res := svc.RunOnce(ctx, []int64{serverID})

	if res.Servers[0].Err == nil {
		t.Fatal("an offline panel should be reported as an error")
	}

	var status string
	if err := st.DB().QueryRow(`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "ACTIVE" {
		t.Fatalf("vpn status = %s, want ACTIVE (an outage must never revoke)", status)
	}

	var srvStatus string
	if err := st.DB().QueryRow(`SELECT status FROM server_integrations WHERE id = ?`, serverID).Scan(&srvStatus); err != nil {
		t.Fatalf("read server status: %v", err)
	}
	if srvStatus != "CONNECTION_UNAVAILABLE" {
		t.Errorf("server status = %s, want CONNECTION_UNAVAILABLE", srvStatus)
	}
}

// TestSyncPaginatesThroughEveryClient proves pagination reads past page 1.
func TestSyncPaginatesThroughEveryClient(t *testing.T) {
	ctx := context.Background()
	st, serverID, _ := newEnv(t)

	const total = 450
	rows := make([]panel.ClientRow, 0, total)
	for i := 0; i < total; i++ {
		rows = append(rows, clientRow("c"+itoa(i)+"@panel.test", 1, 1, 0, true))
	}
	pf := &fakePanel{rows: rows, total: total}

	cfg := veylixsync.DefaultConfig()
	cfg.PageSize = 200
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), cfg)

	res := svc.RunOnce(ctx, []int64{serverID})
	if res.Servers[0].Err != nil {
		t.Fatalf("sync error: %v", res.Servers[0].Err)
	}
	if res.Servers[0].Clients != total {
		t.Errorf("clients read = %d, want %d", res.Servers[0].Clients, total)
	}
	// 450 @ 200/page = 3 requests (200 + 200 + 50).
	if pf.listCalls != 3 {
		t.Errorf("page requests = %d, want 3", pf.listCalls)
	}
}

// TestSyncTimesOutOneServerOnly proves a hanging panel is cut off by its own
// deadline without stopping the others.
func TestSyncTimesOutOneServerOnly(t *testing.T) {
	ctx := context.Background()
	st, good, _ := newEnv(t)

	hung := &fakePanel{hang: true}
	ok := &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("a@panel.test", 5, 5, 0, true)}}

	cfg := veylixsync.DefaultConfig()
	cfg.Timeout = 150 * time.Millisecond

	svc := veylixsync.New(st.DB(), st, func(_ context.Context, id int64) (veylixsync.Panel, error) {
		if id == 7 {
			return hung, nil
		}
		return ok, nil
	}, cfg)

	started := time.Now()
	res := svc.RunOnce(ctx, []int64{7, good})
	elapsed := time.Since(started)

	if res.Servers[0].Err == nil {
		t.Error("the hanging panel should time out")
	}
	if res.Servers[1].Err != nil {
		t.Fatalf("the healthy panel was dragged down: %v", res.Servers[1].Err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("cycle took %v — the timeout did not bound the hanging panel", elapsed)
	}
}

// TestSyncMarksExpiredAndDepleted covers the states sync may set on its own.
// Expiry takes precedence: an expired VPN must not read as traffic-depleted.
func TestSyncMarksExpiredAndDepleted(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	now := time.Now().Add(-time.Hour).Unix()
	if _, err := st.DB().Exec(
		`UPDATE vpn_instances SET traffic_limit_bytes = ?, expires_at = ? WHERE id = ?`,
		1024, now, vpnID); err != nil {
		t.Fatalf("seed limits: %v", err)
	}

	pf := &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("a@panel.test", 10, 10, 0, true)}}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())
	svc.RunOnce(ctx, []int64{serverID})

	var status string
	if err := st.DB().QueryRow(`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "EXPIRED" {
		t.Errorf("status = %s, want EXPIRED (expiry wins over depletion)", status)
	}
}

// TestSyncMarksTrafficDepleted checks the depletion transition on its own.
func TestSyncMarksTrafficDepleted(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	if _, err := st.DB().Exec(
		`UPDATE vpn_instances SET traffic_limit_bytes = ? WHERE id = ?`, 100, vpnID); err != nil {
		t.Fatalf("seed limit: %v", err)
	}
	pf := &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("a@panel.test", 60, 60, 0, true)}}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())

	res := svc.RunOnce(ctx, []int64{serverID})

	var status string
	if err := st.DB().QueryRow(`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "TRAFFIC_DEPLETED" {
		t.Errorf("status = %s, want TRAFFIC_DEPLETED", status)
	}
	if res.Servers[0].Depleted != 1 {
		t.Errorf("reported depleted = %d, want 1", res.Servers[0].Depleted)
	}
}

// TestSyncLeavesDormantVPNStatusAlone is the dormancy counterpart to the test
// above (D-094). A dormant VPN's client may really be gone, but the customer
// explicitly asked to stop being told about it — so the pass must NOT promote it
// to SERVER_UNAVAILABLE and put it back on the card every tick.
//
// What it must still do is be HONEST: the status is left as it was, never
// upgraded to something cheerful. Waking the VPN lets the next pass record the
// absence for real, which the second half of this test proves.
func TestSyncLeavesDormantVPNStatusAlone(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	if _, err := st.DB().Exec(
		`UPDATE vpn_instances SET dormant_at = 1700000000 WHERE id = ?`, vpnID); err != nil {
		t.Fatalf("mark dormant: %v", err)
	}

	pf := &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("other@panel.test", 9, 9, 0, true)}}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())
	res := svc.RunOnce(ctx, []int64{serverID})

	var status string
	if err := st.DB().QueryRow(`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "ACTIVE" {
		t.Errorf("status = %s, want ACTIVE left untouched while dormant", status)
	}
	if res.Servers[0].Absent != 0 {
		t.Errorf("reported absent = %d, want 0 while dormant", res.Servers[0].Absent)
	}

	// Waking it must let the truth through: the very next pass records the
	// absence normally. Dormancy defers the alarm, it does not erase it.
	if _, err := st.DB().Exec(
		`UPDATE vpn_instances SET dormant_at = NULL WHERE id = ?`, vpnID); err != nil {
		t.Fatalf("wake: %v", err)
	}
	pf2 := &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("other@panel.test", 9, 9, 0, true)}}
	svc2 := veylixsync.New(st.DB(), st, factoryFor(pf2), veylixsync.DefaultConfig())
	svc2.RunOnce(ctx, []int64{serverID})

	if err := st.DB().QueryRow(`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&status); err != nil {
		t.Fatalf("read status after wake: %v", err)
	}
	if status != "SERVER_UNAVAILABLE" {
		t.Errorf("status after wake = %s, want SERVER_UNAVAILABLE — dormancy must not hide the absence for ever", status)
	}
}

// TestSyncMarksVerifiedAbsentClient proves D-024.5: a VPN whose panel client is
// *positively* gone becomes SERVER_UNAVAILABLE, and it is never REVOKED (we did
// not delete it, so we cannot claim a revocation). This replaces the older
// "unseen client keeps ACTIVE" expectation, which was exactly the defect the
// owner reported: VPNs listed as working while 3X-UI had no such client.
func TestSyncMarksVerifiedAbsentClient(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	// The panel returns a different client entirely.
	pf := &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("other@panel.test", 9, 9, 0, true)}}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())
	res := svc.RunOnce(ctx, []int64{serverID})

	var status string
	if err := st.DB().QueryRow(`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "SERVER_UNAVAILABLE" {
		t.Errorf("status = %s, want SERVER_UNAVAILABLE for a verified-absent client", status)
	}
	if status == "REVOKED" {
		t.Error("sync must never produce REVOKED")
	}
	if pf.existsCalls == 0 {
		t.Error("absence was assumed without a verifying read")
	}
	if res.Servers[0].Absent != 1 {
		t.Errorf("reported absent = %d, want 1", res.Servers[0].Absent)
	}
	if n := snapshotCount(t, st, vpnID); n != 0 {
		t.Errorf("history rows = %d, want 0 for an absent client", n)
	}
}

// TestSyncDoesNotMarkWhenVerificationFails proves an unreadable panel changes
// nothing: a failed verification is not evidence of absence (REQUIREMENTS §7).
func TestSyncDoesNotMarkWhenVerificationFails(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	pf := &fakePanel{
		total:     1,
		rows:      []panel.ClientRow{clientRow("other@panel.test", 9, 9, 0, true)},
		existsErr: errors.New("panel: request failed: timeout"),
	}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())
	res := svc.RunOnce(ctx, []int64{serverID})

	var status string
	if err := st.DB().QueryRow(`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "ACTIVE" {
		t.Errorf("status = %s, want ACTIVE — a failed read must change nothing", status)
	}
	if res.Servers[0].Unverified != 1 {
		t.Errorf("reported unverified = %d, want 1", res.Servers[0].Unverified)
	}
}

// TestSyncKeepsStatusWhenClientExistsButPageWasShort proves the page-through is
// not trusted over a direct read: the client exists, so it must not be marked
// absent just because a page did not include it.
func TestSyncKeepsStatusWhenClientExistsButPageWasShort(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	// The list omits our client, but the verifying read says it is there.
	pf := &fakePanel{
		total:         1,
		rows:          []panel.ClientRow{clientRow("other@panel.test", 9, 9, 0, true)},
		existsByEmail: map[string]bool{"a@panel.test": true},
	}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())
	res := svc.RunOnce(ctx, []int64{serverID})

	var status string
	if err := st.DB().QueryRow(`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "ACTIVE" {
		t.Errorf("status = %s, want ACTIVE — the client does exist on the panel", status)
	}
	if res.Servers[0].Unverified != 1 {
		t.Errorf("reported unverified = %d, want 1", res.Servers[0].Unverified)
	}
	if res.Servers[0].Absent != 0 {
		t.Errorf("reported absent = %d, want 0", res.Servers[0].Absent)
	}
}

// TestSyncReflectsPanelDisable maps a panel-disabled client to SUSPENDED so the
// UI never shows a VPN that cannot connect.
func TestSyncReflectsPanelDisable(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	pf := &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("a@panel.test", 1, 1, 0, false)}}
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), veylixsync.DefaultConfig())
	svc.RunOnce(ctx, []int64{serverID})

	var status string
	if err := st.DB().QueryRow(`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "SUSPENDED" {
		t.Errorf("status = %s, want SUSPENDED", status)
	}
}

// TestSyncIsIdempotentWithinOneSecond proves a re-run of the same tick does not
// duplicate history or double-count the lifetime total.
func TestSyncIsIdempotentWithinOneSecond(t *testing.T) {
	ctx := context.Background()
	st, serverID, vpnID := newEnv(t)

	pf := &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("a@panel.test", 700, 700, 0, true)}}

	// A frozen clock makes both passes land on the same `taken_at`.
	frozen := time.Unix(1_700_000_000, 0)
	cfg := veylixsync.DefaultConfig()
	cfg.Now = func() time.Time { return frozen }
	svc := veylixsync.New(st.DB(), st, factoryFor(pf), cfg)

	svc.RunOnce(ctx, []int64{serverID})
	svc.RunOnce(ctx, []int64{serverID})

	if n := snapshotCount(t, st, vpnID); n != 1 {
		t.Fatalf("history rows = %d, want 1 (same tick must upsert)", n)
	}
	up, down, _ := lifetimeOf(t, st, vpnID)
	if up != 700 || down != 700 {
		t.Fatalf("lifetime = (%d,%d), want (700,700) — a repeated tick must not double-count", up, down)
	}
}
