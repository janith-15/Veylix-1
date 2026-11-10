package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// backupScript is the single backup implementation. This CLI delegates to it
// rather than reimplementing it: two backup implementations is two chances to be
// wrong about WAL files, and the shipped script already stops the service,
// copies all three database files plus both keys, and verifies the result.
func backupScript() string {
	return filepath.Join(envOr("VEYLIX_DEPLOY_DIR", "/etc/veylix/deploy"), "veylix-backup.sh")
}

// cmdBackup takes a verified backup. It is not destructive, so --dry-run only
// suppresses the invocation.
func cmdBackup(p paths, args []string) error {
	fs := newFlagSet("backup")
	o := addCommonFlags(fs)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("backup takes no positional arguments")
	}

	step("backup")
	script := backupScript()
	if !fileExists(script) {
		return fmt.Errorf("backup script not found at %s - it ships in deploy/; "+
			"set VEYLIX_DEPLOY_DIR to the directory that holds it", script)
	}
	r := runner{dryRun: o.dryRun}
	text, runErr := r.run("bash", script, "backup")
	if text != "" && !o.dryRun {
		out("%s", strings.TrimSpace(redact(text)))
	}
	if runErr != nil {
		return runErr
	}
	if !o.dryRun {
		out("backup complete under %s", p.BackupDir)
	}
	return nil
}

// cmdRestore restores a backup directory over the live data. Destructive: the
// backup must verify first, and the shipped script moves the current state aside
// rather than deleting it.
func cmdRestore(p paths, args []string) error {
	fs := newFlagSet("restore")
	o := addCommonFlags(fs)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usagef("restore needs exactly one backup directory")
	}
	dir := rest[0]
	if !fileExists(dir) {
		return fmt.Errorf("backup directory %s does not exist", dir)
	}

	script := backupScript()
	if !fileExists(script) {
		return fmt.Errorf("backup script not found at %s", script)
	}
	r := runner{dryRun: o.dryRun}

	step("backup verification")
	// Prove the backup is readable BEFORE touching live data. This is the check
	// that stops a bad backup from destroying a good database.
	text, verErr := r.run("bash", script, "verify", dir)
	if text != "" && !o.dryRun {
		out("%s", strings.TrimSpace(redact(text)))
	}
	if verErr != nil {
		return fmt.Errorf("refusing to restore: %w", verErr)
	}

	step("restore")
	text, runErr := r.run("bash", script, "restore", dir)
	if text != "" && !o.dryRun {
		out("%s", strings.TrimSpace(redact(text)))
	}
	if runErr != nil {
		return runErr
	}

	step("readiness")
	if o.dryRun {
		out("[dry-run] would wait for %s/api/ready", o.base)
		return nil
	}
	rep, waitErr := waitReady(o.base, 30, pollGap)
	if waitErr != nil {
		return fmt.Errorf("restore completed but the service is not ready: %w", waitErr)
	}
	out("ready: schema_version=%d ui=%s", rep.SchemaVersion, rep.UI)
	out("restore complete. The pre-restore copy is under %s/pre-restore-*", p.DataDir)
	return nil
}

// cmdRepair re-renders configuration and reloads services. It may NOT touch the
// database, rewrite keys or modify accounts; that restriction is structural -
// this function has no path that writes to the data directory or the key files.
func cmdRepair(p paths, args []string) error {
	fs := newFlagSet("repair")
	o := addCommonFlags(fs)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("repair takes no positional arguments")
	}
	if err := requireInstalled(p); err != nil {
		return err
	}

	r := runner{dryRun: o.dryRun}

	step("configuration")
	if !fileExists(p.envFile()) {
		warnf("%s is missing; repair does not create secrets - run install.sh", p.envFile())
	} else {
		out("environment file present: %s", p.envFile())
	}
	if !fileExists(p.UnitPath) {
		warnf("%s is missing; re-install the unit with install.sh", p.UnitPath)
	} else {
		out("systemd unit present: %s", p.UnitPath)
	}
	// Stated explicitly so a reader can confirm the constraint holds.
	out("repair does not write to %s or %s", p.DataDir, p.ConfigDir)

	step("systemd reload")
	if _, err := r.run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if _, err := r.run("systemctl", "restart", "veylixd"); err != nil {
		return fmt.Errorf("restart failed: %w", err)
	}

	step("caddy validation")
	// A syntax error takes the WHOLE site down, including a domain that was
	// working. Validate first; reload only if the config parses.
	if have("caddy") && fileExists(p.Caddyfile) {
		if _, err := r.run("caddy", "validate", "--config", p.Caddyfile); err != nil {
			return fmt.Errorf("caddy config is invalid; nothing was reloaded: %w", err)
		}
		if _, err := r.run("systemctl", "reload", "caddy"); err != nil {
			warnf("caddy reload failed: %v", err)
		}
	} else {
		skipf("caddy validation", "caddy is not installed or the Caddyfile is absent")
	}

	step("readiness")
	if o.dryRun {
		out("[dry-run] would wait for %s/api/ready", o.base)
		return nil
	}
	rep, waitErr := waitReady(o.base, 30, pollGap)
	if waitErr != nil {
		return fmt.Errorf("repair finished but the service is not ready: %w", waitErr)
	}
	out("ready: schema_version=%d ui=%s", rep.SchemaVersion, rep.UI)
	return nil
}

// cmdUninstall removes the service. Data is preserved by default; a purge needs
// the flag AND the operator typing the configured domain, because two
// independent gates are the only way a destructive default stays deliberate.
func cmdUninstall(p paths, args []string) error {
	fs := newFlagSet("uninstall")
	o := addCommonFlags(fs)
	purge := fs.Bool("purge-data", false,
		"ALSO delete the data, config, backups and the Caddy block (irreversible)")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	_ = rest

	r := runner{dryRun: o.dryRun}
	domain := configuredDomain(p.envFile())

	if *purge {
		if domain == "" {
			return errors.New("cannot purge: no VEYLIX_PUBLIC_BASE_URL in " + p.envFile() +
				"; set it, or uninstall without --purge-data")
		}
		out("")
		out("This will PERMANENTLY DELETE:")
		out("  %s  (the database)", p.DataDir)
		out("  %s  (both keys)", p.ConfigDir)
		out("  %s  (every backup)", p.BackupDir)
		out("  the %q block in %s", domain, p.Caddyfile)
		out("  the veylix service account")
		out("It will NOT delete any client from 3X-UI.")
		out("")

		// --dry-run must never block on stdin: it is what CI runs and what an
		// operator runs first. The confirmation is shown, not solicited.
		if o.dryRun {
			out("[dry-run] would ask you to type %q to confirm the purge", domain)
		} else {
			out("To confirm, type the configured domain (%s) and press enter:", domain)
			if !confirmTyped(domain) {
				return errors.New("confirmation did not match; nothing was deleted")
			}
		}
	}

	step("stop and disable")
	if have("systemctl") {
		if _, err := r.run("systemctl", "disable", "--now", "veylixd"); err != nil {
			warnf("could not stop veylixd: %v", err)
		}
	} else {
		skipf("stop and disable", "systemctl is not available on this host")
	}

	step("remove unit and binaries")
	for _, f := range []string{p.UnitPath, p.binary(), p.prevBin(), p.BinDir + "/vpn-platformctl"} {
		if !fileExists(f) {
			continue
		}
		if err := r.remove(f); err != nil {
			return err
		}
		if !o.dryRun {
			out("removed %s", f)
		}
	}
	if have("systemctl") {
		if _, err := r.run("systemctl", "daemon-reload"); err != nil {
			warnf("daemon-reload failed: %v", err)
		}
	}

	if !*purge {
		step("done")
		out("Service removed. Preserved:")
		out("  %s  (database + keys)", p.DataDir)
		out("  %s", p.ConfigDir)
		out("  %s  (backups)", p.BackupDir)
		out("")
		out("Re-install with install.sh; it refuses while the database exists.")
		out("To delete everything, including the data: vpn-platformctl uninstall --purge-data")
		out("Removing Veylix never deletes a 3X-UI client (D-027.3).")
		return nil
	}

	// ---- purge: everything the install created -------------------------------

	step("purge data")
	for _, d := range []string{p.DataDir, p.ConfigDir} {
		if !fileExists(d) {
			continue
		}
		if err := r.removeAll(d); err != nil {
			return err
		}
		if !o.dryRun {
			out("deleted %s", d)
		}
	}

	// Backups go too. A purge that leaves a copy of the database behind is
	// not a purge, and an operator who asks for one is usually about to hand
	// the host back or rebuild it.
	step("purge backups")
	if fileExists(p.BackupDir) {
		if err := r.removeAll(p.BackupDir); err != nil {
			return err
		}
		if !o.dryRun {
			out("deleted %s", p.BackupDir)
		}
	} else {
		skipf("purge backups", "no backup directory at "+p.BackupDir)
	}

	step("remove the caddy block")
	if err := removeCaddyBlock(r, p.Caddyfile, domain, o.dryRun); err != nil {
		warnf("%v", err)
	}

	step("remove the service account")
	removeServiceAccount(r, o.dryRun)

	step("remove the install lock")
	if fileExists(p.LockPath) {
		if err := r.remove(p.LockPath); err != nil {
			warnf("%v", err)
		}
	}

	reinstallBanner(domain, o.dryRun)
	return nil
}

// confirmTyped reads one line from stdin and reports whether it equals want.
//
// Separated from cmdUninstall so the purge gate can be read on its own: the
// domain has to be TYPED, which is what makes the purge deliberate rather than
// a flag someone copies from a runbook.
func confirmTyped(want string) bool {
	typed, err := bufio.NewReader(os.Stdin).ReadString(newlineByte)
	if err != nil {
		return false
	}
	return strings.TrimSpace(typed) == want
}

const newlineByte = 10

// nlStr is the newline as a string, used where a literal would have to be
// escaped inside another literal.
const nlStr = "\n"

// removeCaddyBlock takes Veylix out of a Caddyfile without harming another site.
//
// install.sh backs the operator file up as <path>.pre-veylix before replacing it,
// so the honest restore is that backup: put it back, validate, reload. When there
// is no backup the block is deleted by domain and the result is validated before
// reload - a Caddyfile that fails validation takes down EVERY site on the host,
// so the original is restored and the error is returned.
func removeCaddyBlock(r runner, caddyfile, domain string, dryRun bool) error {
	if !fileExists(caddyfile) {
		skipf("remove the caddy block", "no Caddyfile at "+caddyfile)
		return nil
	}
	backup := caddyfile + ".pre-veylix"
	if dryRun {
		if fileExists(backup) {
			out("[dry-run] would restore %s from %s", caddyfile, backup)
		} else {
			out("[dry-run] would delete the %q block from %s", domain, caddyfile)
		}
		out("[dry-run] caddy validate + systemctl reload caddy")
		return nil
	}

	original, err := os.ReadFile(caddyfile)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", caddyfile, err)
	}

	if fileExists(backup) {
		prev, err := os.ReadFile(backup)
		if err != nil {
			return fmt.Errorf("cannot read %s: %w", backup, err)
		}
		if err := os.WriteFile(caddyfile, prev, 0o644); err != nil {
			return fmt.Errorf("cannot restore %s: %w", caddyfile, err)
		}
		out("restored %s from %s", caddyfile, backup)
	} else if trimmed := dropSiteBlock(string(original), domain); trimmed != string(original) {
		if err := os.WriteFile(caddyfile, []byte(trimmed), 0o644); err != nil {
			return fmt.Errorf("cannot rewrite %s: %w", caddyfile, err)
		}
		out("removed the %q block from %s", domain, caddyfile)
	} else {
		skipf("remove the caddy block", "no site block for "+domain)
		return nil
	}

	// Validate before reloading. A broken Caddyfile is an outage for every site
	// on the host, not just this one - the same rule install.sh follows.
	if have("caddy") {
		if _, err := r.run("caddy", "validate", "--config", caddyfile); err != nil {
			if werr := os.WriteFile(caddyfile, original, 0o644); werr != nil {
				return fmt.Errorf("caddy rejected the result AND the original could not be restored: %v (restore: %w)", err, werr)
			}
			return fmt.Errorf("caddy rejected the result; the original Caddyfile was restored: %w", err)
		}
		if have("systemctl") {
			if _, err := r.run("systemctl", "reload", "caddy"); err != nil {
				warnf("caddy reload failed: %v", err)
			}
		}
	} else {
		skipf("caddy reload", "caddy is not installed on this host")
	}
	return nil
}

// dropSiteBlock removes the top-level site block whose address list contains
// domain, and NOTHING else: the rest of the Caddyfile is returned untouched, so
// a second domain on the same host keeps working. It counts braces rather than
// matching one line, because Caddy site blocks nest.
func dropSiteBlock(src, domain string) string {
	lines := strings.Split(src, nlStr)
	kept := make([]string, 0, len(lines))
	depth := 0
	skip := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if !skip && depth == 0 && isSiteLineFor(t, domain) {
			skip = true
		}
		if skip {
			depth += strings.Count(line, "{") - strings.Count(line, "}")
			if depth <= 0 {
				skip = false
				depth = 0
			}
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, nlStr)
}

// isSiteLineFor reports whether a top-level line opens the site block for
// domain. Caddy accepts several addresses per block (example.com, www.example.com);
// a comment or a directive line is not a site address.
func isSiteLineFor(line, domain string) bool {
	if line == "" || strings.HasPrefix(line, "#") || !strings.HasSuffix(line, "{") {
		return false
	}
	addrs := strings.TrimSpace(strings.TrimSuffix(line, "{"))
	for _, a := range strings.Split(addrs, ",") {
		if strings.TrimSpace(a) == domain {
			return true
		}
	}
	return false
}

// removeServiceAccount deletes the veylix system user and group.
//
// Only for a purge: a default uninstall leaves them, because a re-install reuses
// them and a lingering account owns nothing once the data directory is gone.
// userdel is best-effort - an operator may have run Veylix as their own user, and
// a missing account is not a failure.
func removeServiceAccount(r runner, dryRun bool) {
	if !have("userdel") {
		skipf("remove the service account", "userdel is not installed")
		return
	}
	if dryRun {
		out("[dry-run] userdel veylix ; groupdel veylix")
		return
	}
	if _, err := r.run("userdel", "veylix"); err != nil {
		skipf("remove the service account", "no veylix user (or it still owns files)")
	} else {
		out("deleted the veylix user")
	}
	if have("groupdel") {
		if _, err := r.run("groupdel", "veylix"); err == nil {
			out("deleted the veylix group")
		}
	}
}

// reinstallBanner is the last thing an operator reads after a purge, and the
// only thing they need if they change their mind: the exact command, with the
// domain already in it.
func reinstallBanner(domain string, dryRun bool) {
	const script = "https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh"
	// A dry run must not claim anything was done. It says what WOULD have
	// happened, and the operator is told to drop --dry-run.
	done, verb := "has been completely removed.", "   "
	if dryRun {
		done, verb = "WOULD have been completely removed (--dry-run).", "[dry-run] "
	}
	out("")
	out("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
	out(" VEYLIX %s", done)
	out("")
	out("%sservice    stopped, disabled, unit and binaries deleted", verb)
	out("%sdatabase   deleted", verb)
	out("%skeys       deleted", verb)
	out("%sbackups    deleted", verb)
	out("%scaddy      block removed (another site on this host is untouched)", verb)
	out("")
	out(" Your 3X-UI clients were NOT touched (D-027.3).")
	out("")
	out(" To install again:")
	out("")
	out("   bash <(curl -Ls %s) --domain %s", script, domain)
	out("")
	if dryRun {
		out(" Nothing above has happened yet: drop --dry-run to execute it.")
	} else {
		out(" Re-run this command on the same host: the database is gone, so the")
		out(" installer takes the fresh-install branch.")
	}
	out("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
	out("")
}

// configuredDomain returns the public hostname recorded in veylix.env, or "".
func configuredDomain(envFile string) string {
	data, err := os.ReadFile(envFile)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "VEYLIX_PUBLIC_BASE_URL=") {
			continue
		}
		v := strings.TrimPrefix(line, "VEYLIX_PUBLIC_BASE_URL=")
		v = strings.Trim(v, `"'`)
		v = strings.TrimPrefix(v, "https://")
		v = strings.TrimPrefix(v, "http://")
		if i := strings.IndexAny(v, "/:"); i >= 0 {
			v = v[:i]
		}
		return v
	}
	return ""
}

// appPort returns the loopback port veylixd is configured to listen on, read
// from VEYLIX_ADDR in veylix.env, and falls back to 8080 (D-123).
//
// It exists because `install.sh --app-port 9090` writes VEYLIX_ADDR=127.0.0.1:9090,
// so a hardcoded probe to :8080 would report a running, healthy service as DOWN.
// Reading the same file the daemon reads is the only way the two cannot drift.
//
// Only the port is returned: every probe in this tool is loopback by design, so
// the host part of VEYLIX_ADDR is deliberately ignored rather than trusted.
func appPort(envFile string) string {
	const fallback = "8080"
	data, err := os.ReadFile(envFile)
	if err != nil {
		return fallback
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "VEYLIX_ADDR=") {
			continue
		}
		addr := strings.Trim(strings.TrimPrefix(line, "VEYLIX_ADDR="), `"' `)
		// SplitHostPort handles "127.0.0.1:9090", "[::1]:9090" and ":9090"
		// alike. A bare "127.0.0.1" has no port, so it is not a match.
		_, port, splitErr := net.SplitHostPort(addr)
		if splitErr != nil {
			continue
		}
		// A non-numeric or out-of-range port is a broken env file, not a port
		// to probe: fall back rather than build "http://127.0.0.1:abc".
		if n, convErr := strconv.Atoi(port); convErr != nil || n < 1 || n > 65535 {
			continue
		}
		return port
	}
	return fallback
}

// defaultBaseURL is the loopback origin every probe uses when the operator did
// not pass --base explicitly.
func defaultBaseURL(envFile string) string {
	return "http://127.0.0.1:" + appPort(envFile)
}

// remove deletes one file through the runner so --dry-run covers it.
func (r runner) remove(path string) error {
	if r.dryRun {
		out("[dry-run] rm -f %s", path)
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove %s: %w", path, err)
	}
	return nil
}

// removeAll deletes a directory tree. It is a method so a reviewer can find every
// call site of a recursive delete with one grep, and so --dry-run is guaranteed
// to be honoured.
//
// The guard refuses any path that is not a leaf: an empty path, a filesystem
// root, or a top-level system directory. It counts BOTH separators, because on
// Windows a path like C:\Users\... contains no "/" at all and a slash-only guard
// would refuse every legitimate directory there.
func (r runner) removeAll(dir string) error {
	if r.dryRun {
		out("[dry-run] rm -rf %s", dir)
		return nil
	}
	if err := guardDeletable(dir); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("cannot delete %s: %w", dir, err)
	}
	return nil
}

// guardDeletable is separated from removeAll so it can be tested directly
// without creating and destroying directories.
//
// Note on Windows: filepath.Clean turns "/var" into "\var", so the system-dir
// comparison uses the ORIGINAL string with forward slashes normalised rather
// than the cleaned one. Getting this wrong silently disables the guard.
func guardDeletable(dir string) error {
	clean := filepath.Clean(dir)
	if clean == "" || clean == "." {
		return fmt.Errorf("refusing to delete suspicious path %q", dir)
	}

	// Depth counts separators of either flavour. A bare volume ("C:\\", "/") has
	// depth 0 and must always be refused.
	depth := strings.Count(clean, "/") + strings.Count(clean, `\`)
	if depth < 1 {
		return fmt.Errorf("refusing to delete suspicious path %q", dir)
	}

	// Normalise to a forward-slash, lower-cased form for the allow-list checks,
	// so "/var", "\var" and "/VAR/" all compare the same way.
	//
	// The product's OWN directories (/var/lib/veylix, /etc/veylix) are
	// deliberately NOT in this list: --purge-data must be able to delete them,
	// and they are only reachable through an explicit, confirmed flag.
	norm := strings.ToLower(strings.ReplaceAll(clean, `\`, "/"))
	switch norm {
	case "/", "/var", "/etc", "/usr", "/home", "/root", "/tmp", "/opt", "/srv",
		"/bin", "/boot", "/dev", "/lib", "/proc", "/sys", "/run",
		"/var/lib", "/var/log", "/var/backups", "/etc/caddy":
		return fmt.Errorf("refusing to delete suspicious path %q", dir)
	}
	return nil
}
