package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// validatePort — the two lessons from install.sh, applied to the CLI
// ---------------------------------------------------------------------------

func TestValidatePort(t *testing.T) {
	good := []string{"1", "80", "443", "8080", "8443", "65535"}
	for _, p := range good {
		if err := validatePort(p); err != nil {
			t.Errorf("validatePort(%q) refused a valid port: %v", p, err)
		}
	}

	// 0 is "any free port" to the kernel and 08 is octal to the shell; both are
	// accepted by a naive numeric check and both are wrong here.
	bad := []string{"", "0", "65536", "99999", "abc", "08", "-1", "1.5", " 80", "80 "}
	for _, p := range bad {
		if err := validatePort(p); err == nil {
			t.Errorf("validatePort(%q) accepted an invalid port", p)
		}
	}
}

// ---------------------------------------------------------------------------
// portSet.validate — every problem reported, and duplicates named
// ---------------------------------------------------------------------------

func TestPortSetValidate(t *testing.T) {
	if err := (portSet{HTTPS: "443", HTTP: "80", App: "8080"}).validate(); err != nil {
		t.Fatalf("the defaults were rejected: %v", err)
	}
	if err := (portSet{HTTPS: "8443", HTTP: "8080", App: "9090"}).validate(); err != nil {
		t.Fatalf("the documented 3X-UI pairing was rejected: %v", err)
	}

	cases := []struct {
		name string
		set  portSet
		must string
	}{
		{"https == http", portSet{"8443", "8443", "8080"}, "must differ"},
		{"app == https", portSet{"443", "80", "443"}, "own port"},
		{"app == http", portSet{"443", "80", "80"}, "another app port"},
		{"bad https", portSet{"abc", "80", "8080"}, "https:"},
		{"bad http", portSet{"443", "0", "8080"}, "http:"},
		{"bad app", portSet{"443", "80", "08"}, "app:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.set.validate()
			if err == nil {
				t.Fatalf("%+v was accepted", c.set)
			}
			if !strings.Contains(err.Error(), c.must) {
				t.Fatalf("error %q does not mention %q", err.Error(), c.must)
			}
		})
	}

	// Two bad values at once must both be reported: a person should not have to
	// run the command twice to learn about both.
	err := (portSet{HTTPS: "abc", HTTP: "xyz", App: "8080"}).validate()
	if err == nil || !strings.Contains(err.Error(), "https:") || !strings.Contains(err.Error(), "http:") {
		t.Fatalf("two invalid ports produced %v, expected both named", err)
	}
}

// readFile is a small helper so the assertions below read as one line each.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// ---------------------------------------------------------------------------
// portSetFromEnv — reads what the installer wrote
// ---------------------------------------------------------------------------

func TestPortSetFromEnv(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "veylix.env")

	t.Run("installer-written hosts", func(t *testing.T) {
		write(t, env, "VEYLIX_ADDR=127.0.0.1:9090\nVEYLIX_HTTPS_PORT=8443\nVEYLIX_HTTP_PORT=8080\n")
		ps := portSetFromEnv(env)
		want := portSet{"8443", "8080", "9090"}
		if ps != want {
			t.Fatalf("portSetFromEnv = %+v, want %+v", ps, want)
		}
	})

	t.Run("a pre-D-123 host falls back to the defaults", func(t *testing.T) {
		write(t, env, "VEYLIX_ADDR=127.0.0.1:8080\n")
		ps := portSetFromEnv(env)
		want := portSet{"443", "80", "8080"}
		if ps != want {
			t.Fatalf("portSetFromEnv = %+v, want %+v", ps, want)
		}
	})

	t.Run("a missing env file falls back", func(t *testing.T) {
		ps := portSetFromEnv(filepath.Join(dir, "absent.env"))
		want := portSet{"443", "80", "8080"}
		if ps != want {
			t.Fatalf("portSetFromEnv = %+v, want %+v", ps, want)
		}
	})
}

// ---------------------------------------------------------------------------
// setEnvKey — rewrites the target line and leaves everything else alone
// ---------------------------------------------------------------------------

func TestSetEnvKey(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "veylix.env")

	const original = "# managed by install.sh\nVEYLIX_ADDR=127.0.0.1:8080\nVEYLIX_LOG_LEVEL=info\nVEYLIX_PUBLIC_BASE_URL=https://vpn.test\n"
	write(t, env, original)

	if err := setEnvKey(env, "VEYLIX_ADDR", "127.0.0.1:9090"); err != nil {
		t.Fatalf("setEnvKey: %v", err)
	}
	got := readFile(t, env)

	if !strings.Contains(got, "VEYLIX_ADDR=127.0.0.1:9090") {
		t.Fatalf("the new value is missing:\n%s", got)
	}
	if strings.Contains(got, "8080") {
		t.Fatalf("the old value survived:\n%s", got)
	}
	// The keys this command does not manage must be byte-for-byte intact.
	for _, keep := range []string{"# managed by install.sh", "VEYLIX_LOG_LEVEL=info", "VEYLIX_PUBLIC_BASE_URL=https://vpn.test"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("setEnvKey dropped %q:\n%s", keep, got)
		}
	}

	t.Run("an absent key is added once, with no blank-line growth", func(t *testing.T) {
		if err := setEnvKey(env, "VEYLIX_HTTPS_PORT", "8443"); err != nil {
			t.Fatalf("setEnvKey: %v", err)
		}
		first := readFile(t, env)
		if !strings.Contains(first, "VEYLIX_HTTPS_PORT=8443") {
			t.Fatalf("the key was not added:\n%s", first)
		}
		// Running it again must not append a second copy or a blank line.
		if err := setEnvKey(env, "VEYLIX_HTTPS_PORT", "8443"); err != nil {
			t.Fatalf("setEnvKey (second): %v", err)
		}
		second := readFile(t, env)
		if strings.Count(second, "VEYLIX_HTTPS_PORT=") != 1 {
			t.Fatalf("the key was added more than once:\n%s", second)
		}
		if strings.Count(second, "\n\n") > strings.Count(first, "\n\n") {
			t.Fatalf("a blank line was added on the second run:\n%q", second)
		}
	})
}

// ---------------------------------------------------------------------------
// renderPorts — the Caddyfile is rewritten in both of its possible states
// ---------------------------------------------------------------------------

func TestRenderPortsFromPlaceholders(t *testing.T) {
	template := "{\n\thttp_port __HTTP_PORT__\n\thttps_port __HTTPS_PORT__\n}\n\nhttps://vpn.test:__HTTPS_PORT__ {\n\treverse_proxy 127.0.0.1:__APP_PORT__ {\n\t\theader_up X-Real-IP {remote_host}\n\t}\n}\n"

	got := renderPorts(template, portSet{HTTPS: "8443", HTTP: "8080", App: "9090"})

	if strings.Contains(got, "PORT__") {
		t.Fatalf("a placeholder survived:\n%s", got)
	}
	for _, want := range []string{
		"http_port 8080",
		"https_port 8443",
		"https://vpn.test:8443 {",
		"reverse_proxy 127.0.0.1:9090 {",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

// An installed host has literal numbers, not placeholders. This is the case
// that actually runs, so it is the one that matters most.
func TestRenderPortsFromRenderedFile(t *testing.T) {
	rendered := "{\n\thttp_port 80\n\thttps_port 443\n}\n\nhttps://vpn.test {\n\treverse_proxy 127.0.0.1:8080 {\n\t\theader_up X-Real-IP {remote_host}\n\t}\n}\n"

	got := renderPorts(rendered, portSet{HTTPS: "8443", HTTP: "8080", App: "9090"})

	for _, want := range []string{
		"http_port 8080",
		"https_port 8443",
		"https://vpn.test:8443 {",
		"reverse_proxy 127.0.0.1:9090 {",
		"header_up X-Real-IP {remote_host}",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}

	t.Run("going back to 443 drops the port from the site address", func(t *testing.T) {
		back := renderPorts(got, portSet{HTTPS: "443", HTTP: "80", App: "8080"})
		if !strings.Contains(back, "https://vpn.test {") {
			t.Fatalf("the site address still carries a port:\n%s", back)
		}
		if strings.Contains(back, "https://vpn.test:443") {
			t.Fatalf(":443 must not appear in the site address:\n%s", back)
		}
	})
}

// ---------------------------------------------------------------------------
// publicURL — the printed link must be one a customer can actually open
// ---------------------------------------------------------------------------

func TestPublicURL(t *testing.T) {
	cases := []struct{ domain, port, want string }{
		{"vpn.test", "443", "https://vpn.test"},
		{"vpn.test", "8443", "https://vpn.test:8443"},
		{"vpn.test", "", "https://vpn.test"},
	}
	for _, c := range cases {
		if got := publicURL(c.domain, c.port); got != c.want {
			t.Errorf("publicURL(%q,%q) = %q, want %q", c.domain, c.port, got, c.want)
		}
	}
}
