package main

import (
	"fmt"
	"os"
	"strings"
)

// dnsProvider describes a pluggable ACME DNS-01 challenge provider (D-125).
//
// DNS-01 is the only challenge that needs no inbound port, which is what makes
// Veylix workable on a host where 3X-UI already holds 80 and 443 (D-123). The
// hard constraint, and the reason this file is careful rather than optimistic:
// **the standard Caddy package does not contain these plugins.** Caddy's own
// install docs say the official packages "come only with the standard modules".
// A Caddyfile that names `dns cloudflare` therefore fails to load on a stock
// Caddy, and pretending otherwise would report HTTPS success that does not exist.
type dnsProvider struct {
	// name is what the Caddyfile calls it: `tls { dns cloudflare <token> }`.
	name string
	// module is the Go import path the plugin registers under.
	module string
	// caddyfileLine is the directive written inside the site's tls block.
	caddyfileLine func(token string) string
}

// dnsProviders is the table. Adding another provider means adding a row and
// nothing else - detection, rendering and the error text all key off this map.
var dnsProviders = map[string]dnsProvider{
	"cloudflare": {
		name:   "cloudflare",
		module: "dns.providers.cloudflare",
		caddyfileLine: func(token string) string {
			return "\tdns cloudflare " + token
		},
	},
}

// dnsProviderFor returns the provider, or a usage error naming the ones that
// exist. An unknown name must not silently fall back to something that would
// write a Caddyfile referencing a plugin nobody has.
func dnsProviderFor(name string) (dnsProvider, error) {
	p, ok := dnsProviders[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		known := make([]string, 0, len(dnsProviders))
		for k := range dnsProviders {
			known = append(known, k)
		}
		return dnsProvider{}, usagef("unknown DNS provider %q; supported: %s",
			name, strings.Join(known, ", "))
	}
	return p, nil
}

// validateCloudflareToken rejects an obviously wrong credential before it is
// written anywhere.
//
// It checks shape only, never the token's validity - proving a token works means
// calling the Cloudflare API, and this CLI performs no outbound request it does
// not explain. A wrong-but-well-formed token fails at certificate issuance,
// where Caddy's own error is far more specific than anything invented here.
func validateCloudflareToken(token string) error {
	// Trim first, then validate what will actually be stored. The caller writes
	// the trimmed value, so validating the untrimmed one would accept a token
	// with a trailing space that then has to be trimmed again somewhere else -
	// or, worse, is not.
	t := strings.TrimSpace(token)
	if t == "" {
		return usagef("the Cloudflare API token is empty")
	}
	// A pasted token often carries an invisible character. Refusing here is
	// better than writing it and failing at issuance with a Cloudflare auth
	// error that looks like a permissions problem.
	if t != token && strings.TrimLeft(token, " \t\r\n") == token {
		return usagef("the Cloudflare API token has trailing whitespace; paste it again without it")
	}
	// Cloudflare API tokens are 40 characters of [A-Za-z0-9_-].
	if len(t) < 30 || len(t) > 60 {
		return usagef("that does not look like a Cloudflare API token "+
			"(expected about 40 characters, got %d)", len(t))
	}
	for _, r := range t {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-'
		if !ok {
			return usagef("the Cloudflare API token contains an unexpected character %q", r)
		}
	}
	return nil
}

// caddyBuildHasDNS reports whether the Caddy binary on this host was built with
// the given DNS provider module.
//
// It asks the binary itself (`caddy list-modules`), because that is the only
// authority on what a particular build contains. The answer does not depend on
// which install method produced the binary.
func caddyBuildHasDNS(r runner, module string) (bool, error) {
	if !have("caddy") {
		return false, fmt.Errorf("caddy is not installed")
	}
	text, err := r.run("caddy", "list-modules")
	if err != nil {
		// "Cannot tell" is not "not present". Treating an error as absence would
		// send the operator to rebuild Caddy for no reason.
		return false, fmt.Errorf("cannot list the caddy modules: %w", err)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == module {
			return true, nil
		}
	}
	return false, nil
}

// dnsPluginFixHint is the instruction an operator gets when the plugin is
// missing. It names both real options and their cost, because "install the
// plugin" is not actionable on its own.
func dnsPluginFixHint(module string) string {
	return fmt.Sprintf(`this caddy was built WITHOUT the %[1]s plugin.

  The standard Caddy package ships only its built-in modules - Caddy's own
  install docs say so. DNS-01 needs the provider plugin compiled in, which
  means replacing the binary. Two ways:

  a) download a build that includes it (no Go toolchain needed):
       https://caddyserver.com/download?package=github.com/caddy-dns/cloudflare
     choose your OS/architecture, then:
       sudo systemctl stop caddy
       sudo cp caddy /usr/bin/caddy && sudo chmod +x /usr/bin/caddy
       sudo systemctl start caddy

  b) build it here with xcaddy:
       xcaddy build --with github.com/caddy-dns/cloudflare

  Until one of those is done, use a host where port 80 or 443 is free so
  HTTP-01 / TLS-ALPN-01 can issue the certificate instead.`, module)
}

// dnsConfig is the non-secret part of a DNS-01 setup, recorded in veylix.env so
// a later repair or domain-set keeps using the same challenge type.
type dnsConfig struct {
	Provider string
}

// readDNSConfig reads the provider recorded by a previous domain-set.
func readDNSConfig(envFile string) dnsConfig {
	values, err := readEnvFile(envFile)
	if err != nil {
		return dnsConfig{}
	}
	return dnsConfig{Provider: values["VEYLIX_DNS_PROVIDER"]}
}

// tokenFile is where the provider credential is stored at rest.
//
// It is a separate 0600 root-only file rather than a line in veylix.env, because
// veylix.env is loaded into the daemon's environment; a DNS-provider token there
// would be visible to every `systemctl show`.
func tokenFile(p paths, provider string) string {
	return p.ConfigDir + "/caddy-dns-" + provider + ".token"
}

// writeTokenFile stores the credential with owner-only permissions and returns
// the previous contents so a failed validation can put them back.
func writeTokenFile(path, token string) (previous []byte, existed bool, err error) {
	if data, readErr := os.ReadFile(path); readErr == nil {
		previous, existed = data, true
	}
	// 0600: a DNS provider token can rewrite the zone's records, which is enough
	// to take the domain over.
	if err := os.WriteFile(path, []byte(strings.TrimSpace(token)+"\n"), 0o600); err != nil {
		return nil, existed, fmt.Errorf("cannot write %s: %w", path, err)
	}
	return previous, existed, nil
}

// restoreTokenFile undoes writeTokenFile.
func restoreTokenFile(path string, previous []byte, existed bool) {
	if existed {
		_ = os.WriteFile(path, previous, 0o600)
		return
	}
	_ = os.Remove(path)
}

// renderDNSChallenge adds a `tls { dns <provider> <token> }` block to every site
// block in the Caddyfile, and removes any block it previously added.
//
// Why replace rather than append: domain-set can run more than once, and two
// `tls` blocks in one site is a parse error, while an old token left behind after
// a rotation is a credential that no longer works but looks configured. The
// markers make the block this tool's own, so it can be rewritten safely and so a
// human reading the file can see where it came from.
//
// The token is written into the file because a Caddyfile is Caddy's only
// declarative input here. That is acceptable only because the file is 0644 and
// the token is a DNS-scoped credential, not a full account token - which is why
// docs/INSTALL.md tells the operator to scope it to Zone:DNS:Edit on one zone.
func renderDNSChallenge(caddyfile string, dns dnsProvider, token string) string {
	const begin = "# BEGIN veylix-dns"
	const end = "# END veylix-dns"

	clean := removeMarkedBlock(caddyfile, begin, end)

	block := begin + "\n" +
		"\ttls {\n" +
		dns.caddyfileLine(token) + "\n" +
		"\t}\n" +
		"\t" + end + "\n"

	lines := strings.Split(clean, "\n")
	outLines := make([]string, 0, len(lines)+6)
	inserted := 0
	for _, line := range lines {
		outLines = append(outLines, line)
		// A site block opening line looks like `https://host {` or `host {` at
		// column zero. The global options block is a bare `{`, which is not a
		// site and must be skipped or the tls directive lands somewhere invalid.
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(line, "{") && !strings.HasPrefix(trimmed, "{") &&
			strings.HasPrefix(line, "#") == false && len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			outLines = append(outLines, strings.TrimRight(block, "\n"))
			inserted++
		}
	}
	if inserted == 0 {
		// No site block found: return the input untouched rather than a file
		// with a stray tls block that cannot belong to anything.
		return caddyfile
	}
	return strings.Join(outLines, "\n")
}

// removeMarkedBlock deletes everything between two marker lines, inclusive.
func removeMarkedBlock(text, begin, end string) string {
	lines := strings.Split(text, "\n")
	outLines := make([]string, 0, len(lines))
	inside := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == begin:
			inside = true
		case trimmed == end:
			inside = false
		case !inside:
			outLines = append(outLines, line)
		}
	}
	return strings.Join(outLines, "\n")
}

// caddyfileHasDNSChallenge reports whether a Caddyfile already configures DNS-01.
// Used by `cert` and by repair to explain why issuance may have failed.
func caddyfileHasDNSChallenge(caddyfile string) bool {
	return strings.Contains(caddyfile, "# BEGIN veylix-dns")
}
