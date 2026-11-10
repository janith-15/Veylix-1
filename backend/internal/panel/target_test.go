package panel

import (
	"errors"
	"net"
	"strings"
	"testing"
)

func resolver(m map[string][]net.IP) func(string) ([]net.IP, error) {
	return func(host string) ([]net.IP, error) {
		if ips, ok := m[host]; ok {
			return ips, nil
		}
		return nil, errors.New("no such host")
	}
}

// The SSRF fuzz table from docs/QA_PLAN.md §3 — every entry must be refused.
func TestParseTargetRejectsDangerousURLs(t *testing.T) {
	pub := resolver(map[string][]net.IP{
		"panel.example.com": {net.ParseIP("203.0.113.10")},
		"rebind.example":    {net.ParseIP("203.0.113.10"), net.ParseIP("127.0.0.1")},
		"private.example":   {net.ParseIP("10.0.0.5")},
	})

	cases := []struct{ name, url string }{
		{"http scheme", "http://panel.example.com"},
		{"file scheme", "file:///etc/passwd"},
		{"gopher scheme", "gopher://panel.example.com"},
		{"loopback literal", "https://127.0.0.1"},
		{"loopback v6", "https://[::1]"},
		{"localhost name", "https://localhost"},
		{"private 10/8", "https://10.0.0.1"},
		{"private 172.16", "https://172.16.0.1"},
		{"private 192.168", "https://192.168.0.1"},
		{"metadata", "https://169.254.169.254"},
		{"alibaba metadata", "https://100.100.100.200"},
		{"embedded credentials", "https://user:pass@panel.example.com"},
		{"query string", "https://panel.example.com/?token=abc"},
		{"fragment", "https://panel.example.com/#x"},
		{"unresolvable", "https://nope.invalid"},
		{"dns rebinding", "https://rebind.example"},
		{"resolves to private", "https://private.example"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseTarget(tc.url, false, pub); err == nil {
				t.Fatalf("%s must be rejected, got nil error", tc.url)
			}
		})
	}
}

func TestParseTargetAcceptsPublicHTTPS(t *testing.T) {
	pub := resolver(map[string][]net.IP{
		"panel.example.com": {net.ParseIP("203.0.113.10")},
	})
	target, err := ParseTarget("https://panel.example.com:2053/QpKFsYpDlLFdRsOoW7", false, pub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Host != "panel.example.com" {
		t.Errorf("host = %q", target.Host)
	}
	if !strings.Contains(target.BaseURL, "/QpKFsYpDlLFdRsOoW7") {
		t.Errorf("base path lost: %q", target.BaseURL)
	}
	if strings.HasSuffix(target.BaseURL, "/") {
		t.Error("base url must not have a trailing slash")
	}
}

// A private address is allowed only under the explicit per-integration policy.
func TestParseTargetPrivateAllowedOnlyWhenEnabled(t *testing.T) {
	priv := resolver(map[string][]net.IP{"internal.example": {net.ParseIP("192.168.1.10")}})

	if _, err := ParseTarget("https://internal.example", false, priv); err == nil {
		t.Fatal("private address must be refused by default")
	}
	if _, err := ParseTarget("https://internal.example", true, priv); err != nil {
		t.Fatalf("private address must be allowed when policy is enabled: %v", err)
	}
}

// Even with the private-network policy on, metadata endpoints stay blocked.
func TestMetadataBlockedEvenWithPrivatePolicy(t *testing.T) {
	if _, err := ParseTarget("https://100.100.100.200", true, resolver(nil)); err == nil {
		t.Fatal("cloud metadata endpoint must never be allowed")
	}
}
