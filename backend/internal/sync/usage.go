// Package sync keeps the local database in step with the 3X-UI panels and
// records time-stamped usage history.
//
// Guarantees (REQUIREMENTS §2.7, §3; new_update.md §6):
//   - one slow or broken panel can never block another, or the dashboard
//     (per-server isolation, bounded workers);
//   - every remote read is paginated and bounded, so a 10k-client panel does not
//     produce a 10k-row response;
//   - usage history is NEVER destroyed by a remote counter reset (F-010): a reset
//     is detected and the pre-reset bytes are folded into a monotonic lifetime
//     total instead of being lost;
//   - history is never invented (REQUIREMENTS §2.7): a period with no snapshot is
//     reported as a gap, not interpolated;
//   - sync is READ-ONLY. It never writes to the panel, so it needs no D-018
//     authorisation.
package sync

// Usage is one observed counter reading for a panel client.
type Usage struct {
	UpBytes   int64
	DownBytes int64
	// ResetCount is the panel's own reset counter (clientStats.resetCount).
	ResetCount int
}

// Lifetime is the monotonic, reset-proof total a VPN has ever used.
type Lifetime struct {
	UpBytes    int64
	DownBytes  int64
	ResetCount int
}

// DeltaResult is the outcome of folding one reading into a lifetime total.
type DeltaResult struct {
	Lifetime Lifetime
	// ResetDetected reports that the panel counter went backwards, so the
	// pre-reset remainder was folded in rather than subtracted.
	ResetDetected bool
}

// Fold applies one reading to a running lifetime total.
//
// The rule (F-010):
//
//	delta = current - previous, computed PER DIRECTION;
//	a negative delta in EITHER direction (or a resetCount increase) means the
//	remote counter was reset, and the pre-reset total plus the fresh reading is
//	the new cumulative value.
//
// Folding `previous + current` on a reset is the only arithmetic that never
// loses bytes: whatever the client used before the reset is already inside
// `prev` (it was folded in on an earlier pass), and whatever it has used since
// is `current`. Adding `prevUsage` as well would count the last reading twice —
// `prev` already contains it (regression test: TestFoldNeverLosesBytes).
//
// A reset is detected by EITHER of two independent signals:
//
//   - a decrease in either direction (unambiguous: a counter cannot go down);
//   - a `resetCount` that is higher than the highest one ever stored for this
//     VPN, not merely higher than the previous reading. The panel's own counter
//     can lag (a stale read returns 0 after a reset), so comparing only against
//     `prevUsage` would miss it; the stored `prev.ResetCount` is the monotonic
//     high-water mark.
func Fold(prev Lifetime, prevUsage Usage, cur Usage) DeltaResult {
	resetCount := prev.ResetCount
	if cur.ResetCount > resetCount {
		resetCount = cur.ResetCount
	}

	resetByCount := cur.ResetCount > prev.ResetCount
	resetByValue := cur.UpBytes < prevUsage.UpBytes || cur.DownBytes < prevUsage.DownBytes

	if resetByCount || resetByValue {
		// `prev` already accounts for everything observed so far; the fresh
		// reading is the only new information.
		return DeltaResult{
			Lifetime: Lifetime{
				UpBytes:    prev.UpBytes + cur.UpBytes,
				DownBytes:  prev.DownBytes + cur.DownBytes,
				ResetCount: resetCount,
			},
			ResetDetected: true,
		}
	}

	return DeltaResult{
		Lifetime: Lifetime{
			UpBytes:    prev.UpBytes + (cur.UpBytes - prevUsage.UpBytes),
			DownBytes:  prev.DownBytes + (cur.DownBytes - prevUsage.DownBytes),
			ResetCount: resetCount,
		},
	}
}

// Total returns the combined up+down lifetime bytes.
func (l Lifetime) Total() int64 { return l.UpBytes + l.DownBytes }

// Exceeded reports whether the lifetime total has reached a byte budget.
// A limit of 0 means unlimited.
func (l Lifetime) Exceeded(limitBytes int64) bool {
	return limitBytes > 0 && l.Total() >= limitBytes
}

// boolToInt64 stores the panel's `enable` flag in an INTEGER column, matching
// the convention the schema already uses for every other boolean.
func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
