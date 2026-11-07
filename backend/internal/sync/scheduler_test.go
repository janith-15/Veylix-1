package sync_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"veylix/backend/internal/panel"
	veylixsync "veylix/backend/internal/sync"
)

// countingPanel records how many concurrent reads happen, to prove cycles do
// not overlap.
type countingPanel struct {
	fakePanel
	inFlight atomic.Int32
	maxSeen  atomic.Int32
	delay    time.Duration
}

func (c *countingPanel) ListClients(ctx context.Context, page, pageSize int) (*panel.ClientPage, error) {
	n := c.inFlight.Add(1)
	defer c.inFlight.Add(-1)
	for {
		old := c.maxSeen.Load()
		if n <= old || c.maxSeen.CompareAndSwap(old, n) {
			break
		}
	}
	time.Sleep(c.delay)
	return c.fakePanel.ListClients(ctx, page, pageSize)
}

// TestSchedulerSkipsOverlappingTicks proves a slow cycle makes the next tick
// skip rather than pile up work (REQUIREMENTS §3).
func TestSchedulerSkipsOverlappingTicks(t *testing.T) {
	ctx := context.Background()
	st, serverID, _ := newEnv(t)

	slow := &countingPanel{
		fakePanel: fakePanel{total: 1, rows: []panel.ClientRow{clientRow("a@panel.test", 1, 1, 0, true)}},
		delay:     120 * time.Millisecond,
	}

	var cycles atomic.Int32
	svc := veylixsync.New(st.DB(), st, func(context.Context, int64) (veylixsync.Panel, error) {
		cycles.Add(1)
		return slow, nil
	}, veylixsync.DefaultConfig())

	sched := veylixsync.NewScheduler(svc, func(context.Context) ([]int64, error) {
		return []int64{serverID}, nil
	}, 20*time.Millisecond) // shorter than one cycle on purpose

	sched.Start(ctx)
	time.Sleep(400 * time.Millisecond)
	sched.Stop()

	// The panel must never have been read by two overlapping passes.
	if got := slow.maxSeen.Load(); got > 1 {
		t.Fatalf("max concurrent panel reads = %d, want 1 (ticks must not overlap)", got)
	}
	// And it must actually have run more than once.
	if got := cycles.Load(); got < 2 {
		t.Fatalf("cycles = %d, want repeated ticks", got)
	}
}

// TestSchedulerDisabledWhenIntervalIsZero proves a zero interval really does
// nothing, so a read-only deployment can switch sync off.
func TestSchedulerDisabledWhenIntervalIsZero(t *testing.T) {
	ctx := context.Background()
	st, serverID, _ := newEnv(t)

	var calls atomic.Int32
	svc := veylixsync.New(st.DB(), st, func(context.Context, int64) (veylixsync.Panel, error) {
		calls.Add(1)
		return &fakePanel{total: 1, rows: []panel.ClientRow{clientRow("a@panel.test", 1, 1, 0, true)}}, nil
	}, veylixsync.DefaultConfig())

	sched := veylixsync.NewScheduler(svc, func(context.Context) ([]int64, error) {
		return []int64{serverID}, nil
	}, 0)

	sched.Start(ctx)
	time.Sleep(120 * time.Millisecond)
	sched.Stop()

	if got := calls.Load(); got != 0 {
		t.Fatalf("factory called %d times with a zero interval, want 0", got)
	}
}

// TestSchedulerStopIsIdempotent keeps a double Stop (shutdown + defer) safe.
func TestSchedulerStopIsIdempotent(t *testing.T) {
	svc := veylixsync.New(nil, nil, nil, veylixsync.DefaultConfig())
	sched := veylixsync.NewScheduler(svc, nil, time.Hour)
	sched.Stop()
	sched.Stop()
}
