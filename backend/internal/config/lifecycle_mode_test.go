package config

import "testing"

// D-098 — the lifecycle mode must fail SAFE.
//
// Every test here is about the same fear: a mistyped environment variable must
// never be the reason a customer's VPNs start being retired. `off` is the
// answer to every input the operator did not spell correctly.
func TestResolveLifecycleModeDefaultsToOff(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"", LifecycleOff},
		{"   ", LifecycleOff},
		{"on", LifecycleOff},          // a typo for enforce — must NOT enable it
		{"true", LifecycleOff},        // plausible, still not a documented value
		{"1", LifecycleOff},           // ditto
		{"yes", LifecycleOff},         // ditto
		{"ENFORCE", LifecycleEnforce}, // case-insensitive for the real values
		{"  enforce  ", LifecycleEnforce},
		{"report", LifecycleReport},
		{"REPORT", LifecycleReport},
		{"enforce", LifecycleEnforce},
		{"off", LifecycleOff},
		{"Off", LifecycleOff},
	}
	for _, c := range cases {
		if got := resolveLifecycleMode(c.raw); got != c.want {
			t.Errorf("resolveLifecycleMode(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

// The default Config must not sweep. An upgrade that begins retiring VPNs
// because a new code path shipped is exactly what "fail safe" prevents.
func TestLoadDefaultsLifecycleToOff(t *testing.T) {
	cfg, err := Load(func(string) string { return "" })
	if err == nil && cfg != nil {
		if cfg.LifecycleMode != LifecycleOff {
			t.Errorf("default LifecycleMode = %q, want %q", cfg.LifecycleMode, LifecycleOff)
		}
	}
}
