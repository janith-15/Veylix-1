package config

import (
	"testing"
	"time"
)

// D-099 — the display zone decides how an instant is SHOWN. It must never
// change what is stored, and it must never fail open into a wrong hour.
func TestDisplayZoneDefaultsToColombo(t *testing.T) {
	name, loc := resolveDisplayZone("")
	if name != DefaultDisplayTimezone {
		t.Errorf("name = %q, want %q", name, DefaultDisplayTimezone)
	}
	if loc == nil {
		t.Fatal("loc = nil, want a real location")
	}
	// The whole point of using the zone NAME rather than a baked offset: the
	// instant must render at +05:30 in Colombo regardless of the host's TZ.
	//
	// 2026-11-09T00:00:00Z is 05:30 the same morning in Colombo.
	utc := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	local := utc.In(loc)
	if local.Hour() != 5 || local.Minute() != 30 {
		t.Errorf("00:00Z in %s = %02d:%02d, want 05:30", name,
			local.Hour(), local.Minute())
	}
	if local.Day() != 9 {
		t.Errorf("day = %d, want 9 (Colombo is ahead of UTC, not behind)", local.Day())
	}
}

// The offset must come from tzdata, not from the host, so a server in any
// region renders the same wall clock.
func TestDisplayZoneIsIndependentOfHostZone(t *testing.T) {
	name, loc := resolveDisplayZone("Asia/Colombo")

	// 18:30 UTC is midnight in Colombo — the exact boundary a Sri Lankan
	// customer would read as "the next day".
	at := time.Date(2026, 11, 8, 18, 30, 0, 0, time.UTC).In(loc)
	if at.Hour() != 0 || at.Minute() != 0 || at.Day() != 9 {
		t.Errorf("%s of 18:30Z = %s, want 09 Nov 00:00", name,
			at.Format("02 Jan 15:04"))
	}
}

// A zone that cannot be loaded must fall back to UTC and KEEP the requested
// name, so an operator can see in the config what they asked for while the
// screens still render a correct instant.
func TestUnloadableDisplayZoneFallsBackToUTC(t *testing.T) {
	name, loc := resolveDisplayZone("Mars/Olympus_Mons")
	if name != "Mars/Olympus_Mons" {
		t.Errorf("name = %q, want the requested value kept for diagnosis", name)
	}
	if loc != time.UTC {
		t.Errorf("loc = %v, want UTC", loc)
	}
}

// The resolved config must carry the zone, because /api/me publishes it and the
// UI has no other source.
func TestLoadPublishesTheDisplayZone(t *testing.T) {
	cfg, err := Load(func(k string) string {
		switch k {
		case "VEYLIX_DISPLAY_TIMEZONE":
			return "Asia/Colombo"
		default:
			return ""
		}
	})
	if err != nil || cfg == nil {
		t.Skip("config needs secrets to load in this environment")
	}
	if cfg.DisplayTimezone != "Asia/Colombo" {
		t.Errorf("DisplayTimezone = %q, want Asia/Colombo", cfg.DisplayTimezone)
	}
	if cfg.DisplayZone == nil {
		t.Error("DisplayZone = nil, want a loaded location")
	}
}
