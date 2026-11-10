package httpapi

import (
	"context"
	"sync"
	"time"

	"veylix/backend/internal/integrations"
)

// CachedPanelInspector wraps a PanelInspector with a short-lived, per-server
// read-through cache, collapsing concurrent reads of the same panel into one.
//
// WHY THIS EXISTS (measured, not guessed)
//
// `GET /api/super/vpns` reads every server's whole client list on every request,
// because the panel is the truth (D-024.3) and the console cannot invent a VPN
// list. In production that produced, over 1,969 requests:
//
//	median 1,044 ms   |  579 requests over 1 s  |  103 over 3 s  |  26 over 10 s
//	worst  44,802 ms  |  total wall time 1,947 s
//
// while every other endpoint answered in 0–8 ms. `integrations.PanelClients`
// pages up to 50 × 200 clients per call and there was no cache anywhere in the
// tree, so ten dashboard refreshes cost ten complete panel reads.
//
// # WHAT THIS DOES NOT DO
//
// It does not make Veylix the authority on anything. A cached entry is a copy of
// a READ the panel already answered, kept for TTL seconds and then discarded. It
// never survives a write: a mutating route can call Invalidate to force the next
// read to go to the panel. And a failed read is never cached as a success — it is
// returned as an error, so the existing ReadError / "unread" handling keeps
// reporting the outage honestly instead of the console rendering a stale list as
// if it were current.
//
// COST ON A 1 vCPU / 500 MB BOX
//
// Two maps and a mutex. Entries are pruned on write, so the maps are bounded by
// the number of configured servers (a handful), not by traffic.
type CachedPanelInspector struct {
	next PanelInspector
	ttl  time.Duration
	// errTTL is deliberately much shorter than ttl. A working answer can be
	// reused for a few seconds; an outage is re-tested quickly so recovery is
	// visible on the next refresh rather than TTL seconds later.
	errTTL time.Duration
	// now is a seam so tests can advance time without sleeping.
	now func() time.Time

	mu      sync.Mutex
	entries map[int64]*panelCacheEntry
	// inflight collapses concurrent reads of the SAME server: the first caller
	// does the panel work, the rest wait for its result. This is the half of the
	// fix that helps under load, where TTL alone would still let a burst through.
	inflight map[int64]*panelInflight
}

type panelCacheEntry struct {
	views   []integrations.PanelClientView
	err     error
	expires time.Time
}

type panelInflight struct {
	done  chan struct{}
	views []integrations.PanelClientView
	err   error
}

// DefaultPanelCacheTTL is the reuse window for a panel client list.
//
// Five seconds is chosen against the two things that actually read it: the
// console polls the VPN list on user action (seconds apart), and the background
// sync already re-reads every panel every 5 minutes. It is short enough that a
// client created elsewhere on the panel shows up on a manual refresh without
// anyone needing to clear a cache, and long enough to turn a burst of dashboard
// requests into one panel read.
const DefaultPanelCacheTTL = 5 * time.Second

// NewCachedPanelInspector wraps next. A nil next is returned unchanged so the
// console keeps its existing "inspector not wired" behaviour.
func NewCachedPanelInspector(next PanelInspector, ttl time.Duration) PanelInspector {
	if next == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = DefaultPanelCacheTTL
	}
	return &CachedPanelInspector{
		next:     next,
		ttl:      ttl,
		errTTL:   ttl / 5,
		now:      time.Now,
		entries:  make(map[int64]*panelCacheEntry),
		inflight: make(map[int64]*panelInflight),
	}
}

// PanelClients serves a cached list when one is fresh, otherwise reads the
// panel once and shares the result with everyone waiting on the same server.
func (c *CachedPanelInspector) PanelClients(ctx context.Context, id int64) ([]integrations.PanelClientView, error) {
	if e := c.lookup(id); e != nil {
		return e.views, e.err
	}

	// Become the leader for this server, or wait on the current leader.
	c.mu.Lock()
	if e := c.freshLocked(id); e != nil {
		c.mu.Unlock()
		return e.views, e.err
	}
	if f, ok := c.inflight[id]; ok {
		c.mu.Unlock()
		select {
		case <-f.done:
			return f.views, f.err
		case <-ctx.Done():
			// A caller that gave up must not hold everyone else up.
			return nil, ctx.Err()
		}
	}
	f := &panelInflight{done: make(chan struct{})}
	c.inflight[id] = f
	c.mu.Unlock()

	views, err := c.next.PanelClients(ctx, id)
	f.views, f.err = views, err

	c.mu.Lock()
	delete(c.inflight, id)
	ttl := c.ttl
	if err != nil {
		ttl = c.errTTL
	}
	c.entries[id] = &panelCacheEntry{views: views, err: err, expires: c.now().Add(ttl)}
	c.pruneLocked()
	c.mu.Unlock()

	close(f.done)
	return views, err
}

// Invalidate drops one server's cached list (id > 0) or every server's (id == 0).
//
// Call this after any panel write so the next read cannot answer from a copy
// taken before the write. It is cheap and safe to call redundantly.
func (c *CachedPanelInspector) Invalidate(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id == 0 {
		c.entries = make(map[int64]*panelCacheEntry)
		return
	}
	delete(c.entries, id)
}

func (c *CachedPanelInspector) lookup(id int64) *panelCacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.freshLocked(id)
}

func (c *CachedPanelInspector) freshLocked(id int64) *panelCacheEntry {
	e, ok := c.entries[id]
	if !ok {
		return nil
	}
	if !c.now().Before(e.expires) {
		delete(c.entries, id)
		return nil
	}
	return e
}

// pruneLocked drops expired entries so the map cannot grow without bound.
func (c *CachedPanelInspector) pruneLocked() {
	now := c.now()
	for id, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, id)
		}
	}
}
