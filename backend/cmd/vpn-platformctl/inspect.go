package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// cmdLogs prints the end of the service log.
//
// The unit logs to journald (deploy/veylixd.service: StandardOutput=journal), so
// there is no file to tail. Saying so is better than inventing a path that does
// not exist - a `logs` command that silently prints nothing is worse than none.
func cmdLogs(p paths, args []string) error {
	fs := newFlagSet("logs")
	o := addCommonFlags(fs)
	lines := fs.Int("n", 50, "how many lines to show")
	follow := fs.Bool("f", false, "follow the log (Ctrl-C to stop)")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	_ = rest

	if *lines <= 0 || *lines > 10000 {
		return usagef("--n must be between 1 and 10000")
	}

	if !have("journalctl") {
		return fmt.Errorf("journalctl is not available on this host - " +
			"veylixd logs to the systemd journal; read it with your system's log tool")
	}

	unit := unitName(p.UnitPath)
	argv := []string{"-u", unit, "-n", strconv.Itoa(*lines), "--no-pager"}
	if *follow {
		argv = append(argv, "-f")
	}

	if o.dryRun {
		out("[dry-run] journalctl %s", strings.Join(argv, " "))
		return nil
	}

	// Logs are shown live, not captured: `logs -f` has to stream, and buffering a
	// follow would never terminate. This is the one place exec is used directly
	// instead of through runner, and it is safe because it changes no state.
	cmd := exec.Command("journalctl", argv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("journalctl failed: %w", err)
	}
	return nil
}

// cmdCert reports what certificate the public domain is actually serving.
//
// It answers the operator's real question - "is HTTPS working, and when does it
// expire?" - by connecting to the domain over TLS, rather than by reading Caddy's
// storage or trusting the Caddyfile. A Caddyfile that configures HTTPS is not
// proof that a certificate was issued.
func cmdCert(p paths, args []string) error {
	fs := newFlagSet("cert")
	o := addCommonFlags(fs)
	domain := fs.String("domain", "", "hostname to check (default: VEYLIX_PUBLIC_BASE_URL)")
	port := fs.String("port", "443", "port to connect to")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	_ = rest

	host := *domain
	if host == "" {
		// configuredDomain already returns a bare hostname (scheme and port
		// stripped), so no further parsing is needed - and re-parsing would risk
		// silently accepting something it had deliberately rejected.
		host = configuredDomain(p.envFile())
	}
	if host == "" {
		return usagef("no domain: set VEYLIX_PUBLIC_BASE_URL in %s or pass --domain", p.envFile())
	}

	addr := net.JoinHostPort(host, *port)
	if o.dryRun {
		out("[dry-run] TLS dial %s and print the served certificate", addr)
		return nil
	}

	step("check %s", addr)
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp", addr,
		&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12},
	)
	if err != nil {
		// Name the likely cause. A TLS dial failure on a fresh install is almost
		// always ACME not having completed, and the fix is `repair`, not a
		// certificate edit.
		return fmt.Errorf("cannot establish TLS with %s: %w\n"+
			"  if the certificate was never issued, run: vpn-platformctl repair", addr, err)
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("%s completed a TLS handshake but presented no certificate", addr)
	}
	cert := state.PeerCertificates[0]

	out("subject:   %s", cert.Subject.CommonName)
	out("issuer:    %s", cert.Issuer.CommonName)
	out("expires:   %s", cert.NotAfter.UTC().Format(time.RFC3339))
	out("protocol:  %s", tlsName(state.Version))
	out("names:     %s", strings.Join(cert.DNSNames, ", "))

	days := int(time.Until(cert.NotAfter).Hours() / 24)
	switch {
	case days < 0:
		return fmt.Errorf("the certificate EXPIRED %d days ago - run: vpn-platformctl repair", -days)
	case days < 14:
		warnf("the certificate expires in %d days; Caddy renews automatically, but watch it", days)
	default:
		out("valid for %d more days", days)
	}
	return nil
}

// tlsName renders a TLS version constant for humans.
func tlsName(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	}
	return fmt.Sprintf("0x%04x", v)
}

// domainFromBase strips the scheme and any path from a public base URL.
//
// Kept for callers that hold a full URL (the installer's report does); the CLI
// itself gets a bare host from configuredDomain.
func domainFromBase(base string) string {
	s := strings.TrimSpace(base)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/#?"); i >= 0 {
		s = s[:i]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		return h
	}
	return s
}
