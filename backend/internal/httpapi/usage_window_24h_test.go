package httpapi_test

import (
	"net/url"
	"testing"
	"time"

	"veylix/backend/internal/httpapi"
)

// TestUsageWindowNamedRanges pins every named range including the new `24h`
// (D-092). A silent fallback to 30 days is the exact failure this guards: a
// chart would then label itself "24 hours" while showing a month.
func TestUsageWindowNamedRanges(t *testing.T) {
	const now = int64(1_700_000_000)
	const day = int64(86400)
	const hour = int64(3600)

	cases := []struct {
		rangeKey  string
		wantSince int64
	}{
		{"24h", now - 24*hour},
		{"7d", now - 7*day},
		{"30d", now - 30*day},
		{"3m", now - 90*day},
		{"1y", now - 365*day},
		{"all", 0},
		{"", now - 30*day}, // the documented default
	}
	for _, c := range cases {
		got, ok := httpapi.UsageWindowFromQuery(c.rangeKey, now)
		if !ok {
			t.Fatalf("range %q refused, want accepted", c.rangeKey)
		}
		if got.SinceSec != c.wantSince {
			t.Errorf("range %q since = %d, want %d", c.rangeKey, got.SinceSec, c.wantSince)
		}
		if got.UntilSec != now {
			t.Errorf("range %q until = %d, want %d", c.rangeKey, got.UntilSec, now)
		}
	}
}

// TestUsageWindowUnknownRangeIsRefused proves an unknown range answers "no"
// rather than defaulting, so the handler can return 422 (D-026.2).
func TestUsageWindowUnknownRangeIsRefused(t *testing.T) {
	for _, bad := range []string{"1d", "90d", "2y", "yesterday", "24"} {
		if _, ok := httpapi.UsageWindowFromQuery(bad, 1000); ok {
			t.Errorf("range %q accepted, want refused", bad)
		}
	}
}

// TestResolveWindowCustomBounds covers the `?since=&until=` window (D-092):
// both bounds become a custom window, a future `until` is clamped to now, and
// an inverted or half-specified window is refused rather than silently empty.
func TestResolveWindowCustomBounds(t *testing.T) {
	const now = int64(1_700_000_000)

	t.Run("both bounds", func(t *testing.T) {
		q := url.Values{"since": {"1000"}, "until": {"2000"}}
		w, reason, ok := httpapi.ResolveWindow(q, now)
		if !ok {
			t.Fatalf("refused: %s", reason)
		}
		if w.Key != "custom" || w.SinceSec != 1000 || w.UntilSec != 2000 {
			t.Fatalf("window = %+v, want custom 1000..2000", w)
		}
	})

	t.Run("future until is clamped", func(t *testing.T) {
		q := url.Values{"since": {"1000"}, "until": {"9999999999"}}
		w, reason, ok := httpapi.ResolveWindow(q, now)
		if !ok {
			t.Fatalf("refused: %s", reason)
		}
		if w.UntilSec != now {
			t.Errorf("until = %d, want clamped to %d", w.UntilSec, now)
		}
	})

	t.Run("inverted window refused", func(t *testing.T) {
		q := url.Values{"since": {"2000"}, "until": {"1000"}}
		if _, _, ok := httpapi.ResolveWindow(q, now); ok {
			t.Error("inverted window accepted, want refused")
		}
	})

	t.Run("equal bounds refused", func(t *testing.T) {
		q := url.Values{"since": {"1000"}, "until": {"1000"}}
		if _, _, ok := httpapi.ResolveWindow(q, now); ok {
			t.Error("empty window accepted, want refused")
		}
	})

	t.Run("half specified keeps the named range", func(t *testing.T) {
		// `since` alone is still a well-defined request: a named range whose
		// lower edge was moved. Refusing it would reject a valid query.
		q := url.Values{"range": {"30d"}, "since": {"1600000000"}}
		w, reason, ok := httpapi.ResolveWindow(q, now)
		if !ok {
			t.Fatalf("refused: %s", reason)
		}
		if w.SinceSec != 1600000000 {
			t.Errorf("since = %d, want the caller's bound", w.SinceSec)
		}
		if w.UntilSec != now {
			t.Errorf("until = %d, want the range default %d", w.UntilSec, now)
		}
		if w.Key == "custom" {
			t.Error("key = custom, want the named range kept for a one-edge move")
		}
	})

	t.Run("custom range without bounds refused", func(t *testing.T) {
		// Naming `custom` and supplying no edges is genuinely under-specified.
		q := url.Values{"range": {"custom"}}
		if _, _, ok := httpapi.ResolveWindow(q, now); ok {
			t.Error("custom without bounds accepted, want refused")
		}
	})

	t.Run("one bound moves a named range", func(t *testing.T) {
		// `since` alone is still a valid request: it is a named range whose
		// lower edge was moved, which is what "last 7 days up to now" means.
		q := url.Values{"range": {"7d"}, "until": {""}}
		if _, _, ok := httpapi.ResolveWindow(q, now); !ok {
			t.Error("named range with an empty until refused, want accepted")
		}
	})

	t.Run("unknown range still refused", func(t *testing.T) {
		q := url.Values{"range": {"bogus"}}
		if _, _, ok := httpapi.ResolveWindow(q, now); ok {
			t.Error("unknown range accepted through ResolveWindow, want refused")
		}
	})
}

// TestResolveWindowEchoesOneClock proves the window is computed against ONE
// reading of the clock. Two now() calls a millisecond apart would otherwise make
// `/usage` and `/usage/hourly` resolve windows that differ, and their totals
// would stop agreeing.
func TestResolveWindowEchoesOneClock(t *testing.T) {
	now := time.Now().Unix()
	q := url.Values{"range": {"24h"}}
	w, _, ok := httpapi.ResolveWindow(q, now)
	if !ok {
		t.Fatal("24h refused")
	}
	if w.UntilSec != now {
		t.Fatalf("until = %d, want the single clock read %d", w.UntilSec, now)
	}
	if w.UntilSec-w.SinceSec != 24*3600 {
		t.Fatalf("span = %d, want exactly 24h", w.UntilSec-w.SinceSec)
	}
}
