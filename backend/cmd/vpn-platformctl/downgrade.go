package main

import "fmt"

// cmdDowngrade installs a SPECIFIC OLDER release.
//
// It is not a second implementation: it builds the exact argument list that
// `update --allow-downgrade` needs and hands it to cmdUpdate. That way the
// forward-only-migration guard, the mandatory backup and the --restore-backup
// requirement all stay in the one place that already tests them.
//
// The version is REQUIRED. `downgrade` with no target is a question, not an
// instruction - and guessing "the previous one" from veylixd.prev would silently
// pick whatever happened to be replaced last, which is not the same thing as
// "the release I want".
func cmdDowngrade(p paths, args []string) error {
	fs := newFlagSet("downgrade")
	o := addCommonFlags(fs)
	restoreBackup := fs.String("restore-backup", "",
		"backup directory to restore, so the older binary starts on data it can read")
	// Registered so `downgrade --help` documents it, but always false: a
	// downgrade without a matching database is not an operation this command
	// offers, because the older binary cannot read the newer schema.
	_ = fs.Bool("allow-downgrade", true, "always on for this command")
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}

	if len(rest) == 0 {
		return usagef("downgrade needs a version, for example: vpn-platformctl downgrade v1.0.0 " +
			"--restore-backup /var/backups/veylix/2026-11-01T10-00-00\n" +
			"  see `vpn-platformctl versions` for the tags that exist")
	}
	if len(rest) > 1 {
		return usagef("downgrade takes exactly one version, got %d", len(rest))
	}
	target := rest[0]

	if *restoreBackup == "" {
		return usagef("downgrade %s also needs --restore-backup <dir>: an older binary cannot read a "+
			"database a newer one has migrated, and the restore is what makes the downgrade safe\n"+
			"  the most recent backup is listed by: vpn-platformctl backup --list", target)
	}
	if !o.dryRun && !fileExists(*restoreBackup) {
		return fmt.Errorf("no backup directory at %s", *restoreBackup)
	}

	updateArgs := []string{"--version", target, "--allow-downgrade", "--restore-backup", *restoreBackup}
	if o.dryRun {
		updateArgs = append(updateArgs, "--dry-run")
	}
	if o.base != "" {
		updateArgs = append(updateArgs, "--base", o.base)
	}
	return cmdUpdate(p, updateArgs)
}
