package sync_test

import (
	"testing"

	veylixsync "veylix/backend/internal/sync"
)

// TestFoldAccumulatesDelta is the normal path: the panel counter grows, so the
// lifetime total grows by exactly the difference.
func TestFoldAccumulatesDelta(t *testing.T) {
	prev := veylixsync.Lifetime{UpBytes: 1000, DownBytes: 5000}
	prevUsage := veylixsync.Usage{UpBytes: 1000, DownBytes: 5000}

	got := veylixsync.Fold(prev, prevUsage, veylixsync.Usage{UpBytes: 1500, DownBytes: 8000})

	if got.ResetDetected {
		t.Fatal("a growing counter must not be reported as a reset")
	}
	if got.Lifetime.UpBytes != 1500 {
		t.Errorf("up = %d, want 1500 (1000 + 500 delta)", got.Lifetime.UpBytes)
	}
	if got.Lifetime.DownBytes != 8000 {
		t.Errorf("down = %d, want 8000 (5000 + 3000 delta)", got.Lifetime.DownBytes)
	}
}

// TestFoldSurvivesRemoteReset is the F-010 regression test.
//
// The panel resets its counters to zero. Naively storing `current` would erase
// the history; naively computing `current - previous` would go negative. The
// pre-reset bytes must be folded into the lifetime total.
func TestFoldSurvivesRemoteReset(t *testing.T) {
	const gib = int64(1 << 30)
	prev := veylixsync.Lifetime{UpBytes: 2 * gib, DownBytes: 10 * gib}
	prevUsage := veylixsync.Usage{UpBytes: 2 * gib, DownBytes: 10 * gib}

	// Remote reset: counters back to zero, resetCount bumped.
	got := veylixsync.Fold(prev, prevUsage, veylixsync.Usage{
		UpBytes: 0, DownBytes: 0, ResetCount: 1,
	})

	if !got.ResetDetected {
		t.Fatal("a counter reset was not detected")
	}
	if got.Lifetime.UpBytes < prev.UpBytes {
		t.Fatalf("lifetime up dropped from %d to %d — history was destroyed", prev.UpBytes, got.Lifetime.UpBytes)
	}
	if got.Lifetime.DownBytes < prev.DownBytes {
		t.Fatalf("lifetime down dropped from %d to %d — history was destroyed", prev.DownBytes, got.Lifetime.DownBytes)
	}
	if got.Lifetime.ResetCount != 1 {
		t.Errorf("reset count = %d, want 1", got.Lifetime.ResetCount)
	}
}

// TestFoldDetectsResetWithoutResetCount covers a panel that zeroes the counters
// but leaves resetCount alone. A decrease in either direction is enough.
func TestFoldDetectsResetWithoutResetCount(t *testing.T) {
	prev := veylixsync.Lifetime{UpBytes: 500, DownBytes: 500}
	prevUsage := veylixsync.Usage{UpBytes: 500, DownBytes: 500}

	got := veylixsync.Fold(prev, prevUsage, veylixsync.Usage{UpBytes: 0, DownBytes: 120})

	if !got.ResetDetected {
		t.Fatal("a value decrease with an unchanged resetCount must still be a reset")
	}
	if got.Lifetime.Total() < prev.Total() {
		t.Fatalf("lifetime total dropped from %d to %d", prev.Total(), got.Lifetime.Total())
	}
}

// TestFoldDetectsResetInOneDirectionOnly guards against a partial reset where
// upload keeps counting but download restarts.
func TestFoldDetectsResetInOneDirectionOnly(t *testing.T) {
	prev := veylixsync.Lifetime{UpBytes: 100, DownBytes: 900}
	prevUsage := veylixsync.Usage{UpBytes: 100, DownBytes: 900}

	got := veylixsync.Fold(prev, prevUsage, veylixsync.Usage{UpBytes: 150, DownBytes: 10})

	if !got.ResetDetected {
		t.Fatal("a decrease in only the download direction must be detected")
	}
	if got.Lifetime.Total() < prev.Total() {
		t.Fatalf("lifetime total dropped from %d to %d", prev.Total(), got.Lifetime.Total())
	}
}

// TestFoldNeverLosesBytes asserts the real invariant: for any sequence of
// readings, the lifetime total never decreases and never under-counts the sum
// of the deltas that were actually observed.
func TestFoldNeverLosesBytes(t *testing.T) {
	readings := []veylixsync.Usage{
		{UpBytes: 0, DownBytes: 0},
		{UpBytes: 100, DownBytes: 400},
		{UpBytes: 300, DownBytes: 900},
		{UpBytes: 0, DownBytes: 0, ResetCount: 1}, // reset
		{UpBytes: 50, DownBytes: 250},
		{UpBytes: 0, DownBytes: 0, ResetCount: 2}, // reset again
		{UpBytes: 10, DownBytes: 10},
	}

	var life veylixsync.Lifetime
	var prev veylixsync.Usage

	for i, cur := range readings {
		res := veylixsync.Fold(life, prev, cur)
		if res.Lifetime.Total() < life.Total() {
			t.Fatalf("step %d: lifetime went backwards (%d → %d)", i, life.Total(), res.Lifetime.Total())
		}
		life = res.Lifetime
		prev = cur
	}

	// Observed bytes, computed independently of Fold. The first reading (0,0) is
	// the baseline, so it contributes 0.
	//   step 1: (0,0)       → (100,400)  = +100/+400   running (100,400)
	//   step 2: (100,400)   → (300,900)  = +200/+500   running (300,900)   = 1200
	//   step 3: (300,900)   → reset(0,0) = +0/+0       running (300,900)   = 1200
	//   step 4: (0,0)       → (50,250)   = +50/+250    running (350,1150)  = 1500
	//   step 5: (50,250)    → reset(0,0) = +0/+0       running (350,1150)  = 1500
	//   step 6: (0,0)       → (10,10)    = +10/+10     running (360,1160)  = 1520
	const want = 1520
	if life.Total() != want {
		t.Fatalf("lifetime total = %d, want %d", life.Total(), want)
	}
}

// TestFoldTreatsStaleResetCountAsMonotonic covers a panel whose resetCount read
// lags or wraps (returns a value BELOW the high-water mark we already stored).
// The stored count must never go backwards, and a value decrease must still be
// caught as a reset.
func TestFoldTreatsStaleResetCountAsMonotonic(t *testing.T) {
	prev := veylixsync.Lifetime{UpBytes: 1000, DownBytes: 1000, ResetCount: 3}

	// The panel now reports resetCount=0 (stale) with counters that did not
	// decrease: growth must still be counted, but the stored count stays at 3.
	grown := veylixsync.Fold(prev, veylixsync.Usage{UpBytes: 900, DownBytes: 900},
		veylixsync.Usage{UpBytes: 950, DownBytes: 950})
	if grown.Lifetime.ResetCount != 3 {
		t.Errorf("stored resetCount = %d, want 3 (must be monotonic)", grown.Lifetime.ResetCount)
	}
	if grown.Lifetime.UpBytes != 1050 || grown.Lifetime.DownBytes != 1050 {
		t.Errorf("lifetime = (%d,%d), want (1050,1050)",
			grown.Lifetime.UpBytes, grown.Lifetime.DownBytes)
	}

	// Now the counters drop: that is a reset even though resetCount went down.
	shrunk := veylixsync.Fold(prev, veylixsync.Usage{UpBytes: 900, DownBytes: 900},
		veylixsync.Usage{UpBytes: 100, DownBytes: 100})
	if !shrunk.ResetDetected {
		t.Fatal("a value decrease with a stale resetCount must still be a reset")
	}
	if shrunk.Lifetime.UpBytes != 1100 || shrunk.Lifetime.DownBytes != 1100 {
		t.Errorf("lifetime = (%d,%d), want (1100,1100) = prev + current",
			shrunk.Lifetime.UpBytes, shrunk.Lifetime.DownBytes)
	}
}

// TestFoldIsIdempotentForRepeatedReadings: re-reading the same counters (a
// double sync pass) must not inflate the total.
func TestFoldIsIdempotentForRepeatedReadings(t *testing.T) {
	life := veylixsync.Lifetime{UpBytes: 1000, DownBytes: 2000}
	usage := veylixsync.Usage{UpBytes: 1000, DownBytes: 2000}

	got := veylixsync.Fold(life, usage, usage)

	if got.Lifetime.Total() != life.Total() {
		t.Fatalf("repeating the same reading changed the total: %d → %d", life.Total(), got.Lifetime.Total())
	}
}

// TestLifetimeExceeded covers the traffic-limit comparison (0 = unlimited).
func TestLifetimeExceeded(t *testing.T) {
	cases := []struct {
		name  string
		life  veylixsync.Lifetime
		limit int64
		want  bool
	}{
		{"zero limit is unlimited", veylixsync.Lifetime{UpBytes: 1 << 40}, 0, false},
		{"under limit", veylixsync.Lifetime{UpBytes: 10, DownBytes: 10}, 100, false},
		{"exactly at limit", veylixsync.Lifetime{UpBytes: 50, DownBytes: 50}, 100, true},
		{"over limit", veylixsync.Lifetime{UpBytes: 80, DownBytes: 80}, 100, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.life.Exceeded(c.limit); got != c.want {
				t.Errorf("Exceeded(%d) = %v, want %v", c.limit, got, c.want)
			}
		})
	}
}
