package sync

// This file exercises the metric recorder directly: recordMetrics is
// unexported, and the network rate it computes is the only honest source for a
// server traffic chart (M6-12). It lives in package sync rather than
// package sync_test for that reason.
//
// Note (apple/CGO): the build uses modernc.org/sqlite, which is pure Go, so
// nothing here drags in CGO — keep new imports off anything that would.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"veylix/backend/internal/panel"
	"veylix/backend/internal/store"
)

func openMetricsStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return st
}

func seedMetricsServer(t *testing.T, st *store.Store) int64 {
	t.Helper()
	res, err := st.DB().Exec(
		`INSERT INTO server_integrations (name, panel_url, token_ciphertext, status, created_at, updated_at)
		 VALUES ('srv', 'https://panel.example:2053/base', x'00', 'ACTIVE', 1, 1)`)
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// M6-12: a network chart needs a rate, and the panel only gives cumulative
// counters. The recorder differences two readings over the elapsed time.
func TestMetricsRecordNetworkRate(t *testing.T) {
	ctx := context.Background()
	st := openMetricsStore(t)

	// A frozen clock the test advances by hand, so the rate is exact.
	clock := int64(1_000_000)
	svc := New(st.DB(), st, nil, Config{Now: func() time.Time { return time.Unix(clock, 0) }})
	serverID := seedMetricsServer(t, st)

	// First reading: no previous sample, so the rate is 0 rather than a spike
	// computed against "zero bytes a long time ago".
	svc.recordMetrics(ctx, serverID, panel.HealthResult{CPUPct: 10, NetUpBytes: 0, NetDownBytes: 0})

	// 60 seconds later, 60 MB up and 600 MB down: 1 MB/s up, 10 MB/s down.
	clock += 60
	svc.recordMetrics(ctx, serverID, panel.HealthResult{
		CPUPct: 20, NetUpBytes: 60 * 1024 * 1024, NetDownBytes: 600 * 1024 * 1024,
	})

	var firstUp, firstDown, secondUp, secondDown int64
	if err := st.DB().QueryRowContext(ctx,
		`SELECT net_up, net_down FROM server_metrics WHERE server_id = ? ORDER BY taken_at ASC LIMIT 1`,
		serverID).Scan(&firstUp, &firstDown); err != nil {
		t.Fatalf("read first sample: %v", err)
	}
	if firstUp != 0 || firstDown != 0 {
		t.Errorf("first sample = %d/%d, want 0/0 (nothing to difference against)", firstUp, firstDown)
	}
	if err := st.DB().QueryRowContext(ctx,
		`SELECT net_up, net_down FROM server_metrics WHERE server_id = ? ORDER BY taken_at DESC LIMIT 1`,
		serverID).Scan(&secondUp, &secondDown); err != nil {
		t.Fatalf("read second sample: %v", err)
	}
	if want := int64(1024 * 1024); secondUp != want {
		t.Errorf("net_up = %d B/s, want %d (1 MB/s)", secondUp, want)
	}
	if want := int64(10 * 1024 * 1024); secondDown != want {
		t.Errorf("net_down = %d B/s, want %d (10 MB/s)", secondDown, want)
	}
}

// A counter reset (panel restart) must read as 0, never a negative rate.
func TestMetricsTreatCounterResetAsZero(t *testing.T) {
	ctx := context.Background()
	st := openMetricsStore(t)
	clock := int64(2_000_000)
	svc := New(st.DB(), st, nil, Config{Now: func() time.Time { return time.Unix(clock, 0) }})
	serverID := seedMetricsServer(t, st)

	svc.recordMetrics(ctx, serverID, panel.HealthResult{NetUpBytes: 9_000_000_000, NetDownBytes: 9_000_000_000})
	clock += 60
	svc.recordMetrics(ctx, serverID, panel.HealthResult{NetUpBytes: 10, NetDownBytes: 10})

	var up, down int64
	if err := st.DB().QueryRowContext(ctx,
		`SELECT net_up, net_down FROM server_metrics WHERE server_id = ? ORDER BY taken_at DESC LIMIT 1`,
		serverID).Scan(&up, &down); err != nil {
		t.Fatalf("read sample: %v", err)
	}
	if up != 0 || down != 0 {
		t.Errorf("after a reset = %d/%d, want 0/0", up, down)
	}
}
