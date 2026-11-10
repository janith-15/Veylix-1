package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// dnsProviderFor — an unknown name must not fall back to something usable
// ---------------------------------------------------------------------------

func TestDNSProviderFor(t *testing.T) {
	p, err := dnsProviderFor("cloudflare")
	if err != nil {
		t.Fatalf("cloudflare was rejected: %v", err)
	}
	if p.module != "dns.providers.cloudflare" {
		t.Fatalf("module = %q", p.module)
	}

	// Case and whitespace must not matter: an operator types what they read.
	if _, err := dnsProviderFor("  CloudFlare  "); err != nil {
		t.Fatalf("a differently-cased name was rejected: %v", err)
	}

	for _, bad := range []string{"", "route53", "cloudfare", "none"} {
		if _, err := dnsProviderFor(bad); err == nil {
			t.Errorf("dnsProviderFor(%q) was accepted", bad)
		}
	}

	// The refusal must name what IS supported, or it is a dead end.
	_, err = dnsProviderFor("route53")
	if err == nil || !strings.Contains(err.Error(), "cloudflare") {
		t.Fatalf("the refusal does not list the supported providers: %v", err)
	}
}

// ---------------------------------------------------------------------------
// validateCloudflareToken — shape only, before the value is written anywhere
// ---------------------------------------------------------------------------

func TestValidateCloudflareToken(t *testing.T) {
	// A realistic 40-character token.
	good := strings.Repeat("a", 37) + "b_C-D9"
	if err := validateCloudflareToken(good); err != nil {
		t.Fatalf("a well-formed token was rejected: %v", err)
	}
	if err := validateCloudflareToken("  " + good + "\n"); err != nil {
		t.Fatalf("surrounding whitespace was not tolerated: %v", err)
	}

	bad := map[string]string{
		"empty":        "",
		"too short":    "abc",
		"too long":     strings.Repeat("a", 100),
		"a space":      strings.Repeat("a", 39) + " ",
		"a slash":      strings.Repeat("a", 39) + "/",
		"a colon":      strings.Repeat("a", 39) + ":",
		"non-ascii":    strings.Repeat("a", 39) + "é",
		"bearer token": "Bearer " + strings.Repeat("a", 40),
	}
	for name, v := range bad {
		t.Run(name, func(t *testing.T) {
			if err := validateCloudflareToken(v); err == nil {
				t.Fatalf("%q was accepted", v)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// renderDNSChallenge — the Caddyfile block, written once and replaced safely
// ---------------------------------------------------------------------------

const sampleCaddyfile = `{
	http_port 80
	https_port 443
}

https://vpn.test {
	encode zstd gzip
	reverse_proxy 127.0.0.1:8080 {
		header_up X-Real-IP {remote_host}
	}
}
`

func TestRenderDNSChallenge(t *testing.T) {
	p, _ := dnsProviderFor("cloudflare")
	const token = "abcdefghijklmnopqrstuvwxyz0123456789ABCD"

	got := renderDNSChallenge(sampleCaddyfile, p, token)

	for _, want := range []string{
		"# BEGIN veylix-dns",
		"# END veylix-dns",
		"dns cloudflare " + token,
		"tls {",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}

	// The tls block must be INSIDE the site block, not in the global options
	// block - a tls directive there is a parse error.
	globalEnd := strings.Index(got, "}\n")
	tlsAt := strings.Index(got, "# BEGIN veylix-dns")
	if tlsAt < globalEnd {
		t.Fatalf("the dns block landed inside the global options block:\n%s", got)
	}
	if !caddyfileHasDNSChallenge(got) {
		t.Fatal("caddyfileHasDNSChallenge did not see the block it just wrote")
	}

	// Exactly one block: two `tls` blocks in one site is a parse error.
	if n := strings.Count(got, "# BEGIN veylix-dns"); n != 1 {
		t.Fatalf("wrote %d dns blocks, want 1:\n%s", n, got)
	}

	// Running it again must not duplicate, and must replace the token - a stale
	// token after rotation looks configured but does not work.
	second := renderDNSChallenge(got, p, "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ")
	if n := strings.Count(second, "# BEGIN veylix-dns"); n != 1 {
		t.Fatalf("a second run produced %d blocks, want 1:\n%s", n, second)
	}
	if strings.Contains(second, token) {
		t.Fatalf("the old token survived a rotation:\n%s", second)
	}
	if !strings.Contains(second, "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ") {
		t.Fatalf("the new token is missing:\n%s", second)
	}
}

// A Caddyfile with no site block must come back untouched, not with a stray
// tls block that belongs to nothing.
func TestRenderDNSChallengeWithoutASiteBlock(t *testing.T) {
	p, _ := dnsProviderFor("cloudflare")
	noSite := "{\n\thttp_port 80\n}\n"
	if got := renderDNSChallenge(noSite, p, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); got != noSite {
		t.Fatalf("a Caddyfile with no site block was modified:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// The token must never reach the console or veylix.env
//
// This is the P0 rule from .clinerules/03-security: a DNS provider token can
// rewrite the zone, so it is treated as a production credential. It lives in its
// own 0600 file and nowhere else.
// ---------------------------------------------------------------------------

func TestTokenFileIsOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	p := paths{ConfigDir: dir}
	const token = "abcdefghijklmnopqrstuvwxyz0123456789ABCD"

	tf := tokenFile(p, "cloudflare")
	if !strings.Contains(tf, dir) {
		t.Fatalf("tokenFile = %q, not under the config dir", tf)
	}
	if strings.Contains(tf, token) {
		t.Fatal("the path itself must not contain the token")
	}

	previous, existed, err := writeTokenFile(tf, "  "+token+"\n")
	if err != nil {
		t.Fatalf("writeTokenFile: %v", err)
	}
	if existed {
		t.Fatal("the file reported as pre-existing on a fresh directory")
	}
	if previous != nil {
		t.Fatal("a fresh file reported previous contents")
	}

	data, err := os.ReadFile(tf)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	// Stored trimmed, with exactly one trailing newline.
	if got, want := string(data), token+"\n"; got != want {
		t.Fatalf("stored %q, want %q", got, want)
	}

	info, err := os.Stat(tf)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Windows does not enforce POSIX bits, so this assertion is meaningful on
	// the Linux target and vacuous here - stated rather than silently skipped.
	if info.Mode().Perm() != 0o600 && os.PathSeparator == '/' {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}

	// A rotation must be able to restore the old value on failure.
	prev2, existed2, err := writeTokenFile(tf, "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ")
	if err != nil {
		t.Fatalf("writeTokenFile (rotate): %v", err)
	}
	if !existed2 {
		t.Fatal("a rotation reported no previous file")
	}
	if strings.TrimSpace(string(prev2)) != token {
		t.Fatalf("previous = %q, want the old token", string(prev2))
	}
	restoreTokenFile(tf, prev2, existed2)
	back, _ := os.ReadFile(tf)
	if strings.TrimSpace(string(back)) != token {
		t.Fatalf("after restore the file holds %q, want the old token", string(back))
	}
}

// A token that was never stored must not leave a file behind when the run fails:
// the absence of the file is what makes the next run ask for it again.
func TestRestoreTokenFileRemovesANewFile(t *testing.T) {
	dir := t.TempDir()
	tf := filepath.Join(dir, "caddy-dns-cloudflare.token")

	if _, _, err := writeTokenFile(tf, "abcdefghijklmnopqrstuvwxyz0123456789ABCD"); err != nil {
		t.Fatalf("writeTokenFile: %v", err)
	}
	restoreTokenFile(tf, nil, false)
	if _, err := os.Stat(tf); err == nil {
		t.Fatal("restoreTokenFile left a file that did not exist before")
	}
}

// readDNSConfig and setEnvKey together must round-trip the provider, and must
// report nothing when no DNS-01 setup has happened.
func TestDNSConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "veylix.env")
	write(t, env, "VEYLIX_ADDR=127.0.0.1:8080\n")

	if got := readDNSConfig(env); got.Provider != "" {
		t.Fatalf("a fresh env file reported provider %q", got.Provider)
	}

	if err := setEnvKey(env, "VEYLIX_DNS_PROVIDER", "cloudflare"); err != nil {
		t.Fatalf("setEnvKey: %v", err)
	}
	if got := readDNSConfig(env); got.Provider != "cloudflare" {
		t.Fatalf("readDNSConfig = %q, want cloudflare", got.Provider)
	}

	// The provider is not a secret; the token is, and it must not be in this file.
	if strings.Contains(readFile(t, env), "dns cloudflare") {
		t.Fatal("the env file gained a Caddyfile directive")
	}
}
