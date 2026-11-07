package sync

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"
)

// ServerLister returns the ids of integrations that should be synced.
type ServerLister func(ctx context.Context) ([]int64, error)

// Scheduler runs a sync cycle on a fixed cadence.
//
// Design rules (REQUIREMENTS §3, new_update.md §8):
//   - a cycle never overlaps itself: if the previous one is still running, the
//     tick is skipped rather than queued, so a slow panel cannot pile up work;
//   - the first tick is jittered so a fleet of instances restarting together
//     does not hit the panels simultaneously;
//   - Stop waits for an in-flight cycle, so shutdown never leaves a write
//     half-applied.
type Scheduler struct {
	svc      *Service
	list     ServerLister
	interval time.Duration
	log      *slog.Logger
	now      func() time.Time
	// jitter returns a fraction of the interval to offset the first tick.
	jitter func(time.Duration) time.Duration

	mu      sync.Mutex
	running bool
	last    time.Time
	lastRes Result
	lastErr error
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewScheduler builds a scheduler. A non-positive interval disables it.
func NewScheduler(svc *Service, list ServerLister, interval time.Duration) *Scheduler {
	return &Scheduler{
		svc:      svc,
		list:     list,
		interval: interval,
		log:      slog.Default(),
		now:      time.Now,
		// Up to 10% of the interval, so restarts spread out.
		jitter: func(d time.Duration) time.Duration {
			if d <= 0 {
				return 0
			}
			max := int64(d / 10)
			if max <= 0 {
				return 0
			}
			return time.Duration(rand.Int63n(max))
		},
	}
}

// Status reports the last completed cycle for the health/status endpoint.
type Status struct {
	Running           bool
	IntervalSeconds   int64
	LastRunUnix       int64
	LastDurationUnix  int64
	ServersOK         int
	ServersFailed     int
	Snapshots         int
	Errors            []string
	AgeSeconds        int64
	StaleAfterSeconds int64
}

// Status snapshots the scheduler state.
func (s *Scheduler) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := Status{
		Running:           s.running,
		IntervalSeconds:   int64(s.interval / time.Second),
		LastDurationUnix:  0,
		StaleAfterSeconds: staleAfter(s.interval),
	}
	if !s.last.IsZero() {
		st.LastRunUnix = s.last.Unix()
		st.AgeSeconds = int64(s.now().Sub(s.last) / time.Second)
		st.LastDurationUnix = int64(s.lastRes.Duration / time.Second)
		for _, r := range s.lastRes.Servers {
			if r.Err != nil {
				st.ServersFailed++
				st.Errors = append(st.Errors, "server "+itoa64(r.ServerID)+": "+r.Err.Error())
			} else {
				st.ServersOK++
			}
			st.Snapshots += r.Snapshots
		}
	}
	return st
}

// staleAfter is three intervals, floored so a very short interval still has a
// meaningful stale window.
func staleAfter(interval time.Duration) int64 {
	d := 3 * interval
	if d < 15*time.Minute {
		d = 15 * time.Minute
	}
	return int64(d / time.Second)
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// Start begins the loop. It returns immediately; cancel ctx or call Stop to end.
func (s *Scheduler) Start(ctx context.Context) {
	if s == nil || s.interval <= 0 {
		return
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return // already started
	}
	loopCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	done := s.done
	s.mu.Unlock()

	go s.loop(loopCtx, done)
}

// Stop ends the loop and waits for an in-flight cycle to finish, so shutdown
// never leaves a half-applied write.
func (s *Scheduler) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.mu.Unlock()

	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			s.log.Warn("sync: stop timed out waiting for the in-flight cycle")
		}
	}
}

// loop is the ticker, with a jittered first run.
func (s *Scheduler) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	s.log.Info("sync scheduler started", "interval", s.interval.String())

	first := s.jitter(s.interval)
	timer := time.NewTimer(first)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("sync scheduler stopped")
			return
		case <-timer.C:
		}

		s.runCycle(ctx)
		timer.Reset(s.interval)
	}
}

// runCycle executes one cycle unless one is already running.
func (s *Scheduler) runCycle(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		s.log.Warn("sync: previous cycle still running, skipping this tick")
		return
	}
	s.running = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	ids, err := s.list(ctx)
	if err != nil {
		s.mu.Lock()
		s.lastErr = err
		s.mu.Unlock()
		s.log.Warn("sync: cannot list servers", "err", err)
		return
	}
	if len(ids) == 0 {
		return
	}

	res := s.svc.RunOnce(ctx, ids)

	s.mu.Lock()
	s.last = s.now()
	s.lastRes = res
	s.lastErr = nil
	s.mu.Unlock()
}

// RunNow forces one cycle (used by tests and the manual sync endpoint).
func (s *Scheduler) RunNow(ctx context.Context, ids []int64) Result {
	res := s.svc.RunOnce(ctx, ids)
	s.mu.Lock()
	s.last = s.now()
	s.lastRes = res
	s.mu.Unlock()
	return res
}
