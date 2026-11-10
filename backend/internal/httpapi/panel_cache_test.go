package httpapi

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"veylix/backend/internal/integrations"
)

// countingInspector records how many times the real panel was asked.
type countingInspector struct {
	calls atomic.Int32
	// block, when non-nil, makes every call wait so concurrency is observable.
	block chan struct{}
	views []integrations.PanelClientView
	err   error
}

func (c *countingInspector) PanelClients(_ context.Context, _ int64) ([]integrations.PanelClientView, error) {
	c.calls.Add(1)
	if c.block != nil {
		<-c.block
	}
	return c.views, c.err
}

func newTestCache(next PanelInspector, ttl time.Duration) *CachedPanelInspector {
	c := NewCachedPanelInspector(next, ttl)
	ci, ok := c.(*CachedPanelInspector)
	if !ok {
		panic("NewCachedPanelInspector did not return *CachedPanelInspector")
	}
	return ci
}

// A repeated read inside the TTL must reach the panel exactly once. This is the
// fix for the measured 1,044 ms median / 44.8 s worst case on GET /api/super/vpns:
// before the cache, every request paged the whole panel.
func TestPanelCacheServesRepeatedReadsFromOnePanelCall(t *testing.T) {
	inner := &countingInspector{views: []integrations.PanelClientView{{PanelEmail: "a@b.c"}}}
	c := newTestCache(inner, 5*time.Second)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		views, err := c.PanelClients(ctx, 7)
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if len(views) != 1 || views[0].PanelEmail != "a@b.c" {
			t.Fatalf("read %d returned %+v, want the cached client", i, views)
		}
	}
	if got := inner.calls.Load(); got != 1 {
		t.Fatalf("panel calls = %d, want 1 (10 reads must collapse to one panel read)", got)
	}
}

// Once the entry expires the panel must be read again: a cache that never
// refreshes would be Veylix inventing a VPN list, which D-024.3 forbids.
func TestPanelCacheRefetchesAfterTTL(t *testing.T) {
	inner := &countingInspector{}
	c := newTestCache(inner, 5*time.Second)
	ctx := context.Background()

	now := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return now }

	if _, err := c.PanelClients(ctx, 3); err != nil {
		t.Fatalf("first read: %v", err)
	}
	now = now.Add(4 * time.Second) // still inside the TTL
	if _, err := c.PanelClients(ctx, 3); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if got := inner.calls.Load(); got != 1 {
		t.Fatalf("panel calls = %d, want 1 while inside the TTL", got)
	}

	now = now.Add(2 * time.Second) // TTL has elapsed
	if _, err := c.PanelClients(ctx, 3); err != nil {
		t.Fatalf("third read: %v", err)
	}
	if got := inner.calls.Load(); got != 2 {
		t.Fatalf("panel calls = %d, want 2 after the TTL expired", got)
	}
}

// An unreachable panel must be reported as an error, never cached as an empty
// list. Caching the failure as "no clients" would make the console show every
// VPN as missing from the panel — a fabricated fact.
func TestPanelCacheNeverCachesAFailureAsSuccess(t *testing.T) {
	wantErr := errors.New("dial tcp: connection refused")
	inner := &countingInspector{err: wantErr}
	c := newTestCache(inner, 5*time.Second)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		views, err := c.PanelClients(ctx, 9)
		if !errors.Is(err, wantErr) {
			t.Fatalf("read %d: err = %v, want the panel error", i, err)
		}
		if views != nil {
			t.Fatalf("read %d returned %+v alongside an error, want nil", i, views)
		}
	}
	// Errors ARE cached, but only for errTTL, to avoid hammering a dead panel.
	if got := inner.calls.Load(); got != 1 {
		t.Fatalf("panel calls = %d, want 1 (a failure is briefly memoised)", got)
	}
}

// A recovered panel must be noticed quickly, not after the full TTL.
func TestPanelCacheRetestsOutageQuickly(t *testing.T) {
	inner := &countingInspector{err: errors.New("down")}
	c := newTestCache(inner, 5*time.Second) // errTTL = 1s
	ctx := context.Background()

	now := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return now }

	if _, err := c.PanelClients(ctx, 4); err == nil {
		t.Fatal("first read: expected the outage to be reported")
	}
	inner.err = nil // the panel came back
	inner.views = []integrations.PanelClientView{{PanelEmail: "x@y.z"}}

	now = now.Add(1500 * time.Millisecond) // past errTTL, inside ttl
	views, err := c.PanelClients(ctx, 4)
	if err != nil {
		t.Fatalf("after recovery: %v (recovery must be visible sooner than the full TTL)", err)
	}
	if len(views) != 1 {
		t.Fatalf("views = %+v, want the recovered client", views)
	}
}

// Concurrent reads of the same server must produce ONE panel call, not one each.
// This is what protects the box during a dashboard refresh burst.
func TestPanelCacheCollapsesConcurrentReads(t *testing.T) {
	inner := &countingInspector{block: make(chan struct{})}
	c := newTestCache(inner, 5*time.Second)
	ctx := context.Background()

	const readers = 12
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			<-start
			if _, err := c.PanelClients(ctx, 5); err != nil {
				t.Errorf("concurrent read: %v", err)
			}
		}()
	}
	close(start)

	// Give the leader time to enter the panel call, then release it.
	time.Sleep(80 * time.Millisecond)
	close(inner.block)
	wg.Wait()

	if got := inner.calls.Load(); got != 1 {
		t.Fatalf("panel calls = %d, want 1 (%d concurrent readers must share one read)", got, readers)
	}
}

// A write must be able to force the next read to hit the panel.
func TestPanelCacheInvalidateForcesReread(t *testing.T) {
	inner := &countingInspector{}
	c := newTestCache(inner, 5*time.Second)
	ctx := context.Background()

	if _, err := c.PanelClients(ctx, 2); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if _, err := c.PanelClients(ctx, 2); err != nil {
		t.Fatalf("cached read: %v", err)
	}
	if got := inner.calls.Load(); got != 1 {
		t.Fatalf("panel calls = %d, want 1", got)
	}

	c.Invalidate(2)
	if _, err := c.PanelClients(ctx, 2); err != nil {
		t.Fatalf("read after invalidate: %v", err)
	}
	if got := inner.calls.Load(); got != 2 {
		t.Fatalf("panel calls = %d, want 2 (invalidate must force a fresh panel read)", got)
	}

	// Invalidate(0) clears everything.
	c.Invalidate(0)
	if _, err := c.PanelClients(ctx, 2); err != nil {
		t.Fatalf("read after global invalidate: %v", err)
	}
	if got := inner.calls.Load(); got != 3 {
		t.Fatalf("panel calls = %d, want 3", got)
	}
}

// A nil inspector stays nil, so the console keeps its "not wired" behaviour
// instead of panicking on a nil dereference.
func TestNewCachedPanelInspectorKeepsNil(t *testing.T) {
	if got := NewCachedPanelInspector(nil, time.Second); got != nil {
		t.Fatalf("got %T, want nil", got)
	}
}
