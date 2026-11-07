package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"veylix/backend/internal/versioning"
)

// pollGap is how long waitReady sleeps between probes. A named constant so every
// command polls at the same rate.
const pollGap = time.Second

// repoSlug is the GitHub repository the release channel reads from. It is
// configurable because a fork must update from its own releases, and because the
// test suite points it at a local server.
func repoSlug() string {
	return envOr("VEYLIX_REPO", "your-org/veylix")
}

func releaseBaseURL(repo, ver string) string {
	if ver == "" || ver == "latest" {
		return fmt.Sprintf("https://github.com/%s/releases/latest/download", repo)
	}
	return fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, ver)
}

// artifactName is the release file for this architecture. It matches exactly what
// .github/workflows/release.yml produces.
func artifactName(arch string) string { return "veylixd-linux-" + arch }

// cliArtifactName is the CLI's own artifact, shipped alongside the binary so an
// update also refreshes the tool doing the updating.
func cliArtifactName(arch string) string { return "vpn-platformctl-linux-" + arch }

// cmdUpdate is the permanent upgrade path: what an existing host runs forever,
// without ever needing install.sh again.
//
// The order is deliberate and must not be reordered:
//
//	1 probe the CURRENT state     so a comparison exists afterwards
//	2 back up                     before anything is replaced
//	3 download + verify checksum  delete on mismatch, abort
//	4 keep .prev, swap binary     the rollback artefact
//	5 restart, poll readiness     the only proof it came up
//	6 compare schema_version      backwards means failure, not success
func cmdUpdate(p paths, args []string) error {
	fs := newFlagSet("update")
	o := addCommonFlags(fs)
	channel := fs.String("channel", "stable", "release channel: stable")
	ver := fs.String("version", "", "exact release tag to install (default: latest)")
	arch := fs.String("arch", runtime.GOARCH, "release architecture to fetch")
	allowDowngrade := fs.Bool("allow-downgrade", false,
		"permit installing an OLDER release (needs --restore-backup; migrations are forward-only)")
	restoreBackup := fs.String("restore-backup", "",
		"backup directory to restore when downgrading, so the older binary starts on data it can read")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	_ = rest

	if *channel != "stable" {
		return usagef("unknown channel %q (only \"stable\" exists today)", *channel)
	}
	if err := requireInstalled(p); err != nil {
		return err
	}

	r := runner{dryRun: o.dryRun}
	base := envOr("VEYLIX_RELEASE_BASE", releaseBaseURL(repoSlug(), *ver))

	step("current state")
	before, beforeErr := probeReady(o.base)
	switch {
	case beforeErr != nil:
		warnf("the running service is not readable: %v", beforeErr)
		warnf("continuing: upgrading a stopped service is a legitimate repair")
	case before.UI == "not_embedded":
		warnf("the CURRENT binary has no embedded UI; the upgrade will fix that")
	default:
		out("running: schema_version=%d ui=%s", before.SchemaVersion, before.UI)
	}
	if beforeErr == nil && before.Version != "" {
		out("version: %s", before.Version)
	}

	// A downgrade is only safe with the data that version can read. Migrations
	// are forward-only, so an older binary cannot read a database a newer one
	// migrated - the matching backup must be restored as part of the swap. Both
	// gates are required: --allow-downgrade to mean it, --restore-backup to be
	// able to do it. Without the flag a downgrade is a usage error, not a
	// silent file replacement.
	target := strings.TrimSpace(*ver)
	downgrade := target != "" && target != "latest" && beforeErr == nil &&
		before.Version != "" && versioning.CompareVersions(target, before.Version) < 0
	if downgrade {
		if !*allowDowngrade {
			return usagef("refusing to downgrade %s -> %s: migrations are forward-only, so a "+
				"downgrade needs --allow-downgrade AND --restore-backup <dir>",
				before.Version, target)
		}
		if strings.TrimSpace(*restoreBackup) == "" {
			return usagef("downgrade %s -> %s needs --restore-backup <dir>: an older binary "+
				"cannot read a database migrated by a newer one",
				before.Version, target)
		}
		if !fileExists(*restoreBackup) {
			return fmt.Errorf("the downgrade backup directory %s does not exist", *restoreBackup)
		}
		step("downgrade")
		out("target %s is older than the running %s", target, before.Version)
		out("the database will be restored from %s so the older binary starts on matching data",
			*restoreBackup)
	}

	step("backup")
	script := backupScript()
	if !fileExists(script) {
		return fmt.Errorf("refusing to upgrade without a backup: %s not found", script)
	}
	if o.dryRun {
		out("[dry-run] bash %s backup", script)
	} else {
		text, bkErr := r.run("bash", script, "backup")
		if bkErr != nil {
			return fmt.Errorf("backup failed; the upgrade was not started: %w\n%s",
				bkErr, strings.TrimSpace(redact(text)))
		}
		out("%s", strings.TrimSpace(redact(text)))
	}

	step("download")
	stage := filepath.Join(os.TempDir(), "veylix-update")
	if err := r.mkdirAll(stage); err != nil {
		return err
	}
	binName := artifactName(*arch)
	cliName := cliArtifactName(*arch)
	binPath := filepath.Join(stage, binName)
	cliPath := filepath.Join(stage, cliName)
	sumsPath := filepath.Join(stage, "SHA256SUMS")

	for _, item := range []struct{ url, dest string }{
		{base + "/" + binName, binPath},
		{base + "/SHA256SUMS", sumsPath},
	} {
		if o.dryRun {
			out("[dry-run] download %s -> %s", item.url, item.dest)
			continue
		}
		if err := download(item.url, item.dest); err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
		out("downloaded %s", filepath.Base(item.dest))
	}

	// The CLI artifact is optional: an older release may not publish it. Its
	// absence is reported, never silently ignored.
	cliAvailable := !o.dryRun
	if !o.dryRun {
		if err := download(base+"/"+cliName, cliPath); err != nil {
			cliAvailable = false
			warnf("this release publishes no %s; the CLI will not be refreshed", cliName)
		}
	}

	step("checksum")
	if o.dryRun {
		out("[dry-run] verify %s against %s", binName, sumsPath)
	} else {
		if err := verifyChecksum(binPath, sumsPath, binName); err != nil {
			return err
		}
		if cliAvailable {
			if err := verifyChecksum(cliPath, sumsPath, cliName); err != nil {
				warnf("CLI checksum mismatch; skipping the CLI refresh: %v", err)
				cliAvailable = false
			}
		}
	}

	step("swap")
	if o.dryRun {
		out("[dry-run] cp %s %s", p.binary(), p.prevBin())
		out("[dry-run] install -m 0755 %s %s", binPath, p.binary())
	} else {
		if _, err := r.run("cp", "-p", p.binary(), p.prevBin()); err != nil {
			return fmt.Errorf("cannot keep the current binary for rollback: %w", err)
		}
		if _, err := r.run("install", "-o", "root", "-g", "root", "-m", "0755",
			binPath, p.binary()); err != nil {
			return fmt.Errorf("cannot install the new binary: %w", err)
		}
		if cliAvailable {
			if _, err := r.run("install", "-o", "root", "-g", "root", "-m", "0755",
				cliPath, filepath.Join(p.BinDir, "vpn-platformctl")); err != nil {
				warnf("could not refresh the CLI: %v", err)
			}
		}
	}

	// A downgrade finishes by restoring the database the target version can read.
	// cmdRestore stops the service, moves the current data aside, restores the
	// chosen backup, restarts and waits for readiness - so it also performs the
	// "restart" step. There is no separate schema comparison to make here: the
	// restored data IS the target version's schema, and requireVersionAgreement
	// (which fails on a backwards schema) would be the wrong test for it.
	if downgrade {
		step("restore the downgrade backup")
		restoreArgs := []string{"--base", o.base}
		if o.dryRun {
			restoreArgs = append(restoreArgs, "--dry-run")
		}
		restoreArgs = append(restoreArgs, *restoreBackup)
		if err := cmdRestore(p, restoreArgs); err != nil {
			return fmt.Errorf("downgrade restore failed: %w", err)
		}
		out("")
		out("downgrade complete; the previous binary is kept at %s", p.prevBin())
		return nil
	}

	step("restart")
	if _, err := r.run("systemctl", "restart", "veylixd"); err != nil {
		return fmt.Errorf("restart failed; roll back with 'vpn-platformctl rollback': %w", err)
	}

	step("readiness")
	if o.dryRun {
		out("[dry-run] would wait for %s/api/ready", o.base)
		return nil
	}
	after, waitErr := waitReady(o.base, 30, pollGap)
	if waitErr != nil {
		return fmt.Errorf("the new binary did not become ready; roll back with "+
			"'vpn-platformctl rollback': %w", waitErr)
	}
	out("running: schema_version=%d ui=%s", after.SchemaVersion, after.UI)

	if beforeErr == nil {
		if err := requireVersionAgreement(before, after); err != nil {
			return err
		}
	}
	if after.UI == "not_embedded" {
		return errors.New("the installed artifact has no embedded UI - wrong artifact; run rollback")
	}

	out("")
	out("update complete; the previous binary is kept at %s", p.prevBin())
	return nil
}

// download fetches url to dest. It is used only for release artifacts, whose
// authenticity is then proven by the checksum - never trusted on its own.
func download(url, dest string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s answered %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", dest, err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("cannot write %s: %w", dest, err)
	}
	return f.Close()
}

// verifyChecksum compares an artifact against its published digest. A mismatch
// deletes the artifact so no later step can pick up a tampered or partial file.
func verifyChecksum(artifact, sumsFile, name string) error {
	sums, err := os.ReadFile(sumsFile)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", sumsFile, err)
	}
	expected := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == name {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("SHA256SUMS has no entry for %s", name)
	}

	f, err := os.Open(artifact)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", artifact, err)
	}
	h := sha256.New()
	_, copyErr := io.Copy(h, f)
	f.Close()
	if copyErr != nil {
		return fmt.Errorf("cannot read %s: %w", artifact, copyErr)
	}
	actual := hex.EncodeToString(h.Sum(nil))

	if !strings.EqualFold(expected, actual) {
		_ = os.Remove(artifact)
		return fmt.Errorf("checksum mismatch for %s (expected %.12s..., got %.12s...) - "+
			"artifact deleted", name, expected, actual)
	}
	out("checksum ok: %s (%.12s...)", name, actual)
	return nil
}

// cmdRollback returns to the previous binary. If the schema moved forward there
// is no forward-only rollback in code, so it points at the pre-upgrade backup
// instead of pretending a binary swap is enough.
func cmdRollback(p paths, args []string) error {
	fs := newFlagSet("rollback")
	o := addCommonFlags(fs)
	restoreDir := fs.String("restore", "", "backup directory to restore as part of the rollback")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	_ = rest

	if !fileExists(p.prevBin()) {
		return fmt.Errorf("no previous binary at %s - nothing to roll back to", p.prevBin())
	}

	r := runner{dryRun: o.dryRun}

	step("current state")
	before, beforeErr := probeReady(o.base)
	if beforeErr != nil {
		warnf("the running service is not readable: %v", beforeErr)
	} else {
		out("running: schema_version=%d ui=%s", before.SchemaVersion, before.UI)
	}

	step("swap the previous binary back")
	if o.dryRun {
		out("[dry-run] install -m 0755 %s %s", p.prevBin(), p.binary())
	} else {
		if _, err := r.run("install", "-o", "root", "-g", "root", "-m", "0755",
			p.prevBin(), p.binary()); err != nil {
			return fmt.Errorf("cannot restore the previous binary: %w", err)
		}
	}

	step("restart")
	if _, err := r.run("systemctl", "restart", "veylixd"); err != nil {
		return fmt.Errorf("restart failed after the swap: %w", err)
	}

	step("readiness")
	if o.dryRun {
		out("[dry-run] would wait for %s/api/ready", o.base)
	} else {
		after, waitErr := waitReady(o.base, 30, pollGap)
		if waitErr != nil {
			return fmt.Errorf("the previous binary did not become ready: %w\n"+
				"restore a backup: vpn-platformctl restore <dir>", waitErr)
		}
		out("running: schema_version=%d ui=%s", after.SchemaVersion, after.UI)

		// A schema that moved forward cannot be rolled back by a binary swap.
		// Say so plainly rather than declaring success.
		if beforeErr == nil && after.SchemaVersion > before.SchemaVersion {
			return fmt.Errorf("the previous binary reports schema %d but the database is at %d; "+
				"the binary alone is not enough - restore the pre-upgrade backup",
				after.SchemaVersion, before.SchemaVersion)
		}
	}

	if *restoreDir != "" {
		step("restore the pre-upgrade backup")
		return cmdRestore(p, []string{"--base", o.base, *restoreDir})
	}

	out("")
	out("rollback complete. If the database was migrated, also restore the pre-upgrade backup:")
	out("  vpn-platformctl restore %s/<timestamp>", p.BackupDir)
	return nil
}

// cmdDomainSet changes the public domain. It validates the Caddy config BEFORE
// reloading, and reverts to the previous Caddyfile if the reload or the HTTPS
// probe fails - a typo must not take a working site down.
func cmdDomainSet(p paths, args []string) error {
	fs := newFlagSet("domain-set")
	o := addCommonFlags(fs)
	skipDNS := fs.Bool("skip-dns", false, "skip the DNS check (recorded in the output)")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usagef("domain-set needs exactly one domain, e.g. vpn.example.com")
	}
	domain := strings.ToLower(strings.TrimSpace(rest[0]))
	if err := validateDomain(domain); err != nil {
		return err
	}
	if !fileExists(p.Caddyfile) {
		return fmt.Errorf("Caddyfile not found at %s", p.Caddyfile)
	}

	r := runner{dryRun: o.dryRun}

	step("validate %s", domain)
	if *skipDNS {
		skipf("dns", "--skip-dns was passed; a wrong DNS record will fail at certificate issuance")
	} else {
		ip := resolveHost(domain)
		if ip == "" {
			return fmt.Errorf("%s does not resolve to any IPv4 address; "+
				"create an A record first, or pass --skip-dns", domain)
		}
		out("dns: %s -> %s", domain, ip)
	}

	step("render and validate the Caddyfile")
	original, err := os.ReadFile(p.Caddyfile)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", p.Caddyfile, err)
	}
	rendered := renderDomain(string(original), domain)
	if rendered == string(original) {
		warnf("the Caddyfile does not contain a placeholder domain; it may already be %s", domain)
	}

	backup := p.Caddyfile + ".pre-domain-set"
	if !o.dryRun {
		if err := os.WriteFile(backup, original, 0o644); err != nil {
			return fmt.Errorf("cannot keep the previous Caddyfile: %w", err)
		}
		if err := os.WriteFile(p.Caddyfile, []byte(rendered), 0o644); err != nil {
			return fmt.Errorf("cannot write %s: %w", p.Caddyfile, err)
		}
		out("wrote %s (previous kept at %s)", p.Caddyfile, backup)
	}

	if !have("caddy") {
		return restoreCaddyfile(p, backup, original,
			errors.New("caddy is not installed; reverted the Caddyfile"))
	}

	step("caddy validate")
	if _, err := r.run("caddy", "validate", "--config", p.Caddyfile); err != nil {
		return restoreCaddyfile(p, backup, original,
			fmt.Errorf("caddy rejected the new config; reverted: %w", err))
	}

	step("reload")
	if _, err := r.run("systemctl", "reload", "caddy"); err != nil {
		return restoreCaddyfile(p, backup, original,
			fmt.Errorf("caddy reload failed; reverted: %w", err))
	}

	step("verify over HTTPS")
	if o.dryRun {
		out("[dry-run] would GET https://%s/api/ready", domain)
		return nil
	}
	rep, probeErr := waitReady("https://"+domain, 30, 2*pollGap)
	if probeErr != nil {
		// The service may be fine on loopback; what failed is public delivery.
		// Returning the incomplete exit code keeps that distinction visible.
		return incompleteError{err: fmt.Errorf(
			"domain changed but HTTPS is NOT active for %s: %w\n"+
				"the previous Caddyfile is at %s; the service keeps running on %s",
			domain, probeErr, backup, o.base)}
	}
	out("https://%s/api/ready -> schema_version=%d ui=%s", domain, rep.SchemaVersion, rep.UI)

	step("persist")
	envText := "VEYLIX_PUBLIC_BASE_URL=https://" + domain + "\n"
	if err := appendEnvIfMissing(p.envFile(), "VEYLIX_PUBLIC_BASE_URL", envText); err != nil {
		warnf("could not persist VEYLIX_PUBLIC_BASE_URL: %v", err)
	} else {
		out("recorded VEYLIX_PUBLIC_BASE_URL=https://%s in %s", domain, p.envFile())
	}
	out("")
	out("Domain changed to %s. Restart the service so e-mail links use the new origin:")
	out("  systemctl restart veylixd")
	return nil
}

// restoreCaddyfile puts the previous configuration back. It is the revert path
// for every failure after the file was written.
func restoreCaddyfile(p paths, backup string, original []byte, cause error) error {
	if err := os.WriteFile(p.Caddyfile, original, 0o644); err != nil {
		return fmt.Errorf("%v (AND the revert failed: %w)", cause, err)
	}
	return fmt.Errorf("%v (the previous Caddyfile was restored from %s)", cause, backup)
}

// renderDomain replaces the placeholder host in the template. The template ships
// with vpn.example.com, so that is what gets substituted.
func renderDomain(template, domain string) string {
	outText := strings.ReplaceAll(template, "vpn.example.com", domain)
	outText = strings.ReplaceAll(outText, "example.com", strings.TrimPrefix(domain, "vpn."))
	return outText
}

// validateDomain rejects anything that cannot receive a publicly trusted
// certificate: an IP literal, localhost, or a name with no dot.
func validateDomain(d string) error {
	switch d {
	case "", "localhost":
		return usagef("invalid domain %q", d)
	}
	if strings.HasPrefix(d, ".") || strings.HasSuffix(d, ".") || strings.Contains(d, "..") {
		return usagef("invalid domain %q", d)
	}
	if !strings.Contains(d, ".") {
		return usagef("%q is not a fully qualified domain name", d)
	}
	if strings.Trim(d, "0123456789.") == "" {
		return usagef("%q is an IP address; certificates need a hostname", d)
	}
	return nil
}

// appendEnvIfMissing adds KEY=value to the env file when KEY is absent. It never
// rewrites an existing line: a key that is already set is the operator's choice.
func appendEnvIfMissing(envFile, key, line string) error {
	data, err := os.ReadFile(envFile)
	if err == nil {
		for _, existing := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(existing), key+"=") {
				return nil
			}
		}
	}
	f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("\n# Set by vpn-platformctl domain-set\n" + line)
	return err
}
