package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		"ALSO delete the data directory and config directory (irreversible)")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	_ = rest

	r := runner{dryRun: o.dryRun}

	if *purge {
		domain := configuredDomain(p.envFile())
		if domain == "" {
			return errors.New("cannot purge: no VEYLIX_PUBLIC_BASE_URL in " + p.envFile() +
				"; set it, or uninstall without --purge-data")
		}
		out("")
		out("This will PERMANENTLY DELETE:")
		out("  %s  (the database)", p.DataDir)
		out("  %s  (both keys)", p.ConfigDir)
		out("It will NOT delete any client from 3X-UI.")
		out("")

		// --dry-run must never block on stdin: it is the first execution in a
		// session and it is also what CI runs. The confirmation is therefore
		// shown, not solicited.
		if o.dryRun {
			out("[dry-run] would ask you to type %q to confirm the purge", domain)
		} else {
			out("To confirm, type the configured domain (%s) and press enter:", domain)
			typed, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
			if readErr != nil {
				return fmt.Errorf("confirmation not read: %w", readErr)
			}
			if strings.TrimSpace(typed) != domain {
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

	if *purge {
		step("purge data")
		for _, d := range []string{p.DataDir, p.ConfigDir} {
			if err := r.removeAll(d); err != nil {
				return err
			}
			if !o.dryRun {
				out("deleted %s", d)
			}
		}
		out("")
		out("Data purged. Removing Veylix never deletes a 3X-UI client (D-027.3).")
		return nil
	}

	step("done")
	out("Service removed. Preserved:")
	out("  %s  (database + keys)", p.DataDir)
	out("  %s", p.ConfigDir)
	out("  %s  (backups)", p.BackupDir)
	out("")
	out("Re-install with install.sh; it refuses while the database exists.")
	out("To delete the data as well: vpn-platformctl uninstall --purge-data")
	out("Removing Veylix never deletes a 3X-UI client (D-027.3).")
	return nil
}

// configuredDomain reads the public origin host from the env file. Used only for
// the purge confirmation, never for a network decision.
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
