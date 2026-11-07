package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func key(b byte) string {
	buf := make([]byte, 32)
	for i := range buf {
		buf[i] = b
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func env(pairs map[string]string) func(string) string {
	return func(k string) string { return pairs[k] }
}

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"VEYLIX_ENCRYPTION_KEY": key(1),
		"VEYLIX_SESSION_KEY":    key(2),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Errorf("default addr = %q, want loopback 127.0.0.1:8080", cfg.HTTPAddr)
	}
	if cfg.Port() != 8080 {
		t.Errorf("Port() = %d, want 8080", cfg.Port())
	}
	if !strings.HasSuffix(cfg.DBPath, "veylix.db") {
		t.Errorf("DBPath = %q, want a veylix.db path", cfg.DBPath)
	}
}

// A missing secret must stop the server: failing open would mean panel tokens
// are stored unencrypted (D-011, new_update.md §7).
func TestLoadFailsClosedWithoutEncryptionKey(t *testing.T) {
	_, err := Load(env(map[string]string{"VEYLIX_SESSION_KEY": key(2)}))
	if err == nil {
		t.Fatal("expected error when VEYLIX_ENCRYPTION_KEY is missing")
	}
	if !strings.Contains(err.Error(), "VEYLIX_ENCRYPTION_KEY") {
		t.Errorf("error should name the missing variable, got: %v", err)
	}
}

func TestLoadRejectsShortKey(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("too-short"))
	_, err := Load(env(map[string]string{
		"VEYLIX_ENCRYPTION_KEY": short,
		"VEYLIX_SESSION_KEY":    key(2),
	}))
	if err == nil {
		t.Fatal("expected error for a key shorter than 32 bytes")
	}
}

func TestLoadRejectsNonBase64Key(t *testing.T) {
	_, err := Load(env(map[string]string{
		"VEYLIX_ENCRYPTION_KEY": "not-base64!!",
		"VEYLIX_SESSION_KEY":    key(2),
	}))
	if err == nil {
		t.Fatal("expected error for a non-base64 key")
	}
}

// The log line must never contain key material.
// The log line must never contain key material.
// Regression for a defect found during the live smoke test: the cookie was
// issued Secure over plain HTTP, so the browser never sent it back and every
// authenticated request failed with 401 while the UI looked "logged in".
func TestSecureCookiesDecision(t *testing.T) {
	cases := []struct {
		name, addr, flag string
		want             bool
	}{
		{"loopback default is secure", "127.0.0.1:8080", "", true},
		{"loopback may opt out for local http", "127.0.0.1:8080", "1", false},
		{"localhost may opt out too", "localhost:8080", "1", false},
		{"ipv6 loopback may opt out", "[::1]:8080", "1", false},
		{"network bind is always secure", "0.0.0.0:8080", "1", true},
		{"public ip is always secure", "203.0.113.5:443", "1", true},
		{"empty host is always secure", ":8080", "1", true},
	}
	for _, c := range cases {
		if got := resolveSecureCookies(c.addr, c.flag); got != c.want {
			t.Errorf("%s (%s flag=%q): got %v, want %v", c.name, c.addr, c.flag, got, c.want)
		}
	}
}

func TestStringDoesNotLeakSecrets(t *testing.T) {
	enc := key(9)
	cfg, err := Load(env(map[string]string{
		"VEYLIX_ENCRYPTION_KEY": enc,
		"VEYLIX_SESSION_KEY":    key(5),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(cfg.String(), enc) {
		t.Fatal("Config.String leaked the encryption key")
	}
	if strings.Contains(cfg.String(), base64.StdEncoding.EncodeToString(cfg.EncryptionKey)) {
		t.Fatal("Config.String leaked base64 key material")
	}
}

// TestGoogleConfiguredRejectsPlaceholders pins the rule that a template value is
// not a credential.
//
// The env template shipped placeholder strings and the old check only tested for
// an EMPTY string, so /api/auth/google/status answered configured=true while no
// usable client existed. The login page then enabled "Continue with Google" and
// sent the user to Google to come back with invalid_client. Presence is not
// configuration; shape is.
//
// SECURITY: the "valid" values below are DELIBERATE FIXTURES, not credentials.
// This test asserts the SHAPE rules (a real client id ends in
// `.apps.googleusercontent.com`; a real secret starts with `GOCSPX-`). A real
// credential would prove nothing extra here and would leak the moment this file
// is pushed. If you are tempted to paste a working client into this test: don't.
func TestGoogleConfiguredRejectsPlaceholders(t *testing.T) {
	const (
		placeholderID     = "<id>.apps.googleusercontent.com"
		placeholderSecret = "<secret>"
		// Fixture, shape-only. Not a credential; it authenticates nothing.
		fixtureID     = "000000000000-fixtureonlynotacredential.apps.googleusercontent.com"
		fixtureSecret = "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ential"
		redirect      = "http://localhost:5173/api/auth/google/callback"
	)

	tests := []struct {
		name string
		g    GoogleOAuth
		want bool
	}{
		{"template placeholders are not configured", GoogleOAuth{placeholderID, placeholderSecret, redirect}, false},
		{"a well-shaped client is configured", GoogleOAuth{fixtureID, fixtureSecret, redirect}, true},
		{"empty id", GoogleOAuth{"", fixtureSecret, redirect}, false},
		{"empty secret", GoogleOAuth{fixtureID, "", redirect}, false},
		{"missing redirect", GoogleOAuth{fixtureID, fixtureSecret, ""}, false},
		{"id without the google suffix", GoogleOAuth{"abc-123.apps.example.com", fixtureSecret, redirect}, false},
		{"secret without the GOCSPX prefix", GoogleOAuth{fixtureID, "not-a-google-secret-value", redirect}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.g.Configured(); got != tc.want {
				t.Fatalf("Configured() = %v, want %v", got, tc.want)
			}
		})
	}
}
