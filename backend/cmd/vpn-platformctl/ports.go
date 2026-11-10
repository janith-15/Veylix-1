package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// defaultPorts are what an ordinary install uses. They match install.sh, so a
// host that never touched the flags and a host that ran `ports set` with these
// values end up identical.
const (
	defaultHTTPSPort = "443"
	defaultHTTPPort  = "80"
	defaultAppPort   = "8080"
)

// portSet is the three listeners, validated as a group.
type portSet struct {
	HTTPS string
	HTTP  string
	App   string
}

// validatePort checks a single port. It mirrors validate_port in
// scripts/lib/common.sh deliberately: an operator who types a bad value at the
// menu and a bad value at the installer must get the same answer, and "0 means
// any free port to the kernel" plus "08 is octal to the shell" are the two that
// bite.
func validatePort(p string) error {
	if p == "" {
		return usagef("a port is required")
	}
	for _, r := range p {
		if r < '0' || r > '9' {
			return usagef("%q is not a port number", p)
		}
	}
	if len(p) > 5 || (len(p) > 1 && p[0] == '0') {
		return usagef("%q is not a valid port (no leading zeros, max 65535)", p)
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 1 || n > 65535 {
		return usagef("%q is not a valid port; use 1-65535", p)
	}
	return nil
}

// validate checks all three at once and reports every problem, not just the
// first: a person who mistypes two values should not have to run the command
// three times to learn about all of them.
func (ps portSet) validate() error {
	var problems []string
	if err := validatePort(ps.HTTPS); err != nil {
		problems = append(problems, "https: "+err.Error())
	}
	if err := validatePort(ps.HTTP); err != nil {
		problems = append(problems, "http: "+err.Error())
	}
	if err := validatePort(ps.App); err != nil {
		problems = append(problems, "app: "+err.Error())
	}
	if len(problems) > 0 {
		return usagef("%s", strings.Join(problems, "; "))
	}

	// Distinctness. Two listeners on one port is a bind failure at runtime;
	// catching it here names the pair instead of surfacing "address already in
	// use" from Caddy with no clue which value caused it.
	switch {
	case ps.HTTPS == ps.HTTP:
		return usagef("https and http are both %s; they must differ", ps.HTTPS)
	case ps.App == ps.HTTPS:
		return usagef("app and https are both %s; Caddy cannot front a service on its own port", ps.App)
	case ps.App == ps.HTTP:
		return usagef("app and http are both %s; pick another app port", ps.App)
	}
	return nil
}

// readEnvFile parses KEY=value lines into a map. Comments and blanks are
// ignored, and quotes around a value are stripped.
func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.Trim(strings.TrimSpace(line[eq+1:]), `"'`)
		values[key] = val
	}
	return values, nil
}

// portSetFromEnv reads the three ports back out of veylix.env.
//
// The app port comes from VEYLIX_ADDR (that is what the daemon binds). The two
// Caddy ports come from the variables install.sh records, because the Caddyfile
// is the only other place they live and parsing it is guesswork. When those
// variables are absent - an install from before this command existed - the
// conventional defaults are assumed, which is what those installs are.
func portSetFromEnv(envFile string) portSet {
	ps := portSet{
		HTTPS: defaultHTTPSPort,
		HTTP:  defaultHTTPPort,
		App:   appPort(envFile),
	}
	values, err := readEnvFile(envFile)
	if err != nil {
		return ps
	}
	if v := values["VEYLIX_HTTPS_PORT"]; v != "" {
		ps.HTTPS = v
	}
	if v := values["VEYLIX_HTTP_PORT"]; v != "" {
		ps.HTTP = v
	}
	return ps
}

// setEnvKey rewrites one KEY=value line in place, adding it when absent.
//
// It rewrites only the target line and preserves everything else byte for byte,
// including comments and ordering. Regenerating the file would drop operator
// edits and, worse, risk losing a key this command does not know about.
func setEnvKey(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	line := key + "=" + value
	lines := strings.Split(string(data), "\n")
	found := false
	for i, existing := range lines {
		if strings.HasPrefix(strings.TrimSpace(existing), key+"=") {
			lines[i] = line
			found = true
			break
		}
	}
	if !found {
		// Replace the trailing empty element left by the final newline rather
		// than appending after it, or every run adds a blank line.
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = append(lines[:n-1], line, "")
		} else {
			lines = append(lines, line)
		}
	}
	// 0600: this file holds the encryption and session keys (Bible §0.6).
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}

// renderPorts substitutes the three port placeholders and rewrites literal ports
// already present in the file.
//
// The Caddyfile may be in either state: freshly templated (placeholders) or
// already rendered by install.sh (literal numbers). Both are handled, because
// `ports set` runs on installed hosts, where it holds literal values.
func renderPorts(template string, ps portSet) string {
	outText := template
	outText = strings.ReplaceAll(outText, "__HTTPS_PORT__", ps.HTTPS)
	outText = strings.ReplaceAll(outText, "__HTTP_PORT__", ps.HTTP)
	outText = strings.ReplaceAll(outText, "__APP_PORT__", ps.App)

	lines := strings.Split(outText, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "http_port "):
			lines[i] = "\thttp_port " + ps.HTTP
		case strings.HasPrefix(trimmed, "https_port "):
			lines[i] = "\thttps_port " + ps.HTTPS
		case strings.HasPrefix(trimmed, "reverse_proxy 127.0.0.1:"):
			rest := ""
			if idx := strings.Index(trimmed, "{"); idx >= 0 {
				rest = " " + trimmed[idx:]
			}
			lines[i] = "\treverse_proxy 127.0.0.1:" + ps.App + rest
		case strings.HasPrefix(trimmed, "https://"):
			// Keep the host, replace only the port. A site address with no port
			// is the 443 default and needs one only when the port is not 443.
			host := strings.TrimPrefix(trimmed, "https://")
			if idx := strings.IndexAny(host, ": {"); idx >= 0 {
				host = host[:idx]
			}
			if ps.HTTPS == defaultHTTPSPort {
				lines[i] = "https://" + host + " {"
			} else {
				lines[i] = "https://" + host + ":" + ps.HTTPS + " {"
			}
		}
	}
	return strings.Join(lines, "\n")
}

// publicURL is the origin a customer types: no ":443", because it is noise and
// looks broken, and an explicit port otherwise, because without it the URL does
// not connect.
func publicURL(domain, httpsPort string) string {
	if httpsPort == defaultHTTPSPort || httpsPort == "" {
		return "https://" + domain
	}
	return "https://" + domain + ":" + httpsPort
}

// cmdPorts shows or changes the listeners.
//
// `ports` with no arguments prints the current configuration. `ports set` is the
// menu item; each flag defaults to the value already in use, so
// `ports set --https-port 8443` changes one port and leaves the other two alone.
func cmdPorts(p paths, args []string) error {
	sub := "show"
	rest := args
	if len(args) > 0 && args[0] == "set" {
		sub = "set"
		rest = args[1:]
	}

	fs := newFlagSet("ports")
	o := addCommonFlags(fs)
	httpsPort := fs.String("https-port", "", "the port customers type (default: current)")
	httpPort := fs.String("http-port", "", "the ACME challenge / redirect port (default: current)")
	appPortFlag := fs.String("app-port", "", "the loopback port veylixd binds (default: current)")
	rest, err := parseArgs(fs, rest)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("ports takes no positional arguments; use --https-port, --http-port, --app-port")
	}

	current := portSetFromEnv(p.envFile())

	if sub == "show" {
		step("current ports")
		out("https      %s   <- the port customers type", current.HTTPS)
		out("http       %s   <- ACME challenge and redirect", current.HTTP)
		out("app        %s   <- loopback only (127.0.0.1)", current.App)
		out("")
		if domain := configuredDomain(p.envFile()); domain != "" {
			out("customers reach: %s", publicURL(domain, current.HTTPS))
		}
		out("change them with: vpn-platformctl ports set --https-port 8443")
		return nil
	}

	// Every unset flag keeps the current value, so one port can change alone.
	next := current
	if *httpsPort != "" {
		next.HTTPS = *httpsPort
	}
	if *httpPort != "" {
		next.HTTP = *httpPort
	}
	if *appPortFlag != "" {
		next.App = *appPortFlag
	}

	// Validate before reading or writing anything.
	if err := next.validate(); err != nil {
		return err
	}

	if !fileExists(p.Caddyfile) {
		return fmt.Errorf("Caddyfile not found at %s", p.Caddyfile)
	}

	original, err := os.ReadFile(p.Caddyfile)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", p.Caddyfile, err)
	}
	rendered := renderPorts(string(original), next)
	if rendered == string(original) && next == current {
		out("the ports are already %s/%s (app %s); nothing to do", next.HTTPS, next.HTTP, next.App)
		return nil
	}

	r := runner{dryRun: o.dryRun}

	step("ports %s/%s (app %s) -> %s/%s (app %s)",
		current.HTTPS, current.HTTP, current.App, next.HTTPS, next.HTTP, next.App)

	if o.dryRun {
		out("[dry-run] would write %s", p.Caddyfile)
		out("[dry-run] would set VEYLIX_ADDR=127.0.0.1:%s in %s", next.App, p.envFile())
		out("[dry-run] would restart veylixd and reload caddy")
		out("[dry-run] nothing above has happened yet")
		return nil
	}

	backup := p.Caddyfile + ".pre-ports"
	if err := os.WriteFile(backup, original, 0o644); err != nil {
		return fmt.Errorf("cannot keep the previous Caddyfile: %w", err)
	}
	if err := os.WriteFile(p.Caddyfile, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("cannot write %s: %w", p.Caddyfile, err)
	}
	out("wrote %s (previous kept at %s)", p.Caddyfile, backup)

	// Validate BEFORE reloading; a bad config takes the whole site down, and
	// that includes a domain that was working a second ago.
	if !have("caddy") {
		warnf("caddy is not installed; the Caddyfile was written but not validated or reloaded")
	} else if _, err := r.run("caddy", "validate", "--config", p.Caddyfile); err != nil {
		return restoreCaddyfile(p, backup, original, err)
	} else {
		out("caddy validate: ok")
	}

	// The daemon must bind the new port before Caddy proxies to it, or there is
	// a window where the site answers 502.
	if err := setEnvKey(p.envFile(), "VEYLIX_ADDR", "127.0.0.1:"+next.App); err != nil {
		return restoreCaddyfile(p, backup, original,
			fmt.Errorf("cannot update VEYLIX_ADDR: %w", err))
	}
	_ = setEnvKey(p.envFile(), "VEYLIX_HTTPS_PORT", next.HTTPS)
	_ = setEnvKey(p.envFile(), "VEYLIX_HTTP_PORT", next.HTTP)
	out("updated %s (VEYLIX_ADDR=127.0.0.1:%s)", p.envFile(), next.App)

	if _, err := r.run("systemctl", "restart", "veylixd"); err != nil {
		return restoreCaddyfile(p, backup, original,
			fmt.Errorf("veylixd did not restart on the new app port: %w", err))
	}
	if have("caddy") {
		if _, err := r.run("systemctl", "reload", "caddy"); err != nil {
			return fmt.Errorf("the Caddyfile is valid but caddy did not reload: %w", err)
		}
	}

	// Verify rather than assume: systemctl returns 0 on queueing, and the whole
	// point of this command is that the site answers on the port it claims.
	step("verify")
	base := "http://127.0.0.1:" + next.App
	if _, err := probeReady(base); err != nil {
		return fmt.Errorf("veylixd is not answering on %s after the change: %w", base, err)
	}
	out("veylixd answers on %s", base)

	out("")
	if domain := configuredDomain(p.envFile()); domain != "" {
		out("customers now reach: %s", publicURL(domain, next.HTTPS))
	}
	if next.HTTPS != defaultHTTPSPort {
		warnf("HTTPS is now on %s. Check the certificate with: vpn-platformctl cert --port %s",
			next.HTTPS, next.HTTPS)
	}
	return nil
}
