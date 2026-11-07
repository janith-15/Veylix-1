package httpapi

import "testing"

// P-06 — the four ranges the owner named, plus "all", and a refusal for a typo.
func TestUsageRangeWindow(t *testing.T) {
	const now = int64(1_700_000_000)
	const day = int64(86400)

	cases := []struct {
		in     string
		want   string
		since  int64
		reject bool
	}{
		{"", "30d", now - 30*day, false},
		{"7d", "7d", now - 7*day, false},
		{"30d", "30d", now - 30*day, false},
		{"3m", "3m", now - 90*day, false},
		{"1y", "1y", now - 365*day, false},
		{"ALL", "all", 0, false},
		{"2w", "", 0, true},
	}
	for _, tc := range cases {
		got, ok := UsageWindowFromQuery(tc.in, now)
		if tc.reject {
			if ok {
				t.Errorf("%q must be refused, not defaulted", tc.in)
			}
			continue
		}
		if !ok {
			t.Fatalf("%q was refused", tc.in)
		}
		if got.Key != tc.want || got.SinceSec != tc.since {
			t.Errorf("%q → %+v, want key %q since %d", tc.in, got, tc.want, tc.since)
		}
		if got.UntilSec != now {
			t.Errorf("%q until = %d, want now (%d)", tc.in, got.UntilSec, now)
		}
	}
}

// P-01 / P-02 — the read carries up, down and TOTAL separately, plus the
// percentage used, so a screen never has to derive them.
