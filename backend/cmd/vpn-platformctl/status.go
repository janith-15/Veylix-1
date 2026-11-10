package main

import (
	"fmt"
	"runtime"
)

// cmdStatus reports what the host actually looks like right now. It is
// read-only by construction: it starts no service, writes no file and takes no
// lock, so it is always safe to run, including during an incident.
func cmdStatus(p paths, args []string) error {
	fs := newFlagSet("status")
	o := addCommonFlags(fs)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("status takes no positional arguments")
	}

	r := runner{dryRun: o.dryRun}

	doc := statusDoc{
		Installed:     fileExists(p.binary()),
		Binary:        p.binary(),
		ServiceActive: serviceActive(r, "veylixd"),
		DataDirSizeMB: dirSizeMB(p.DataDir),
	}

	if !doc.Installed {
		doc.Error = "veylixd is not installed at " + p.binary()
		if o.json {
			return printJSONErr(doc, doc.Error)
		}
		out("status:      NOT INSTALLED (%s)", p.binary())
		out("             run install.sh to install, or check VEYLIX_BIN_DIR")
		return fmt.Errorf("%s", doc.Error)
	}

	rep, probeErr := probeReady(o.base)
	if probeErr != nil {
		doc.Error = probeErr.Error()
	} else {
		doc.Reachable = true
		doc.Ready = rep.Status == "ready"
		doc.SchemaVersion = rep.SchemaVersion
		doc.UI = rep.UI
	}

	if o.json {
		if doc.Ready {
			return printJSON(doc)
		}
		return printJSONErr(doc, doc.Error)
	}

	out("binary:      %s", doc.Binary)
	out("service:     %s", activeWord(doc.ServiceActive))
	out("readiness:   %s", doc.readinessWord())
	if doc.Reachable {
		out("schema:      %d", doc.SchemaVersion)
		out("ui:          %s", doc.UI)
		if doc.UI == "not_embedded" {
			warnf("this binary does not carry the interface; every page will answer a JSON 404")
		}
	}
	out("data dir:    %s (%d MB)", p.DataDir, doc.DataDirSizeMB)
	out("enc key:     fingerprint %s", fingerprint(p.encKey()))
	out("session key: fingerprint %s", fingerprint(p.sessKey()))

	if !doc.Ready {
		return fmt.Errorf("%s", doc.Error)
	}
	return nil
}

// cmdDoctor re-runs the preflight checks against an installed host. It is the
// "why is this host not healthy" command: every failure prints a correction.
func cmdDoctor(p paths, args []string) error {
	fs := newFlagSet("doctor")
	o := addCommonFlags(fs)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("doctor takes no positional arguments")
	}

	r := runner{dryRun: o.dryRun}
	var failures int
	check := func(name string, ok bool, correction string) {
		if ok {
			out("ok    %s", name)
			return
		}
		failures++
		out("FAIL  %s", name)
		if correction != "" {
			out("      -> %s", correction)
		}
	}

	out("vpn-platformctl %s doctor (GOOS=%s GOARCH=%s)", version, runtime.GOOS, runtime.GOARCH)

	check("veylixd installed", fileExists(p.binary()),
		fmt.Sprintf("install it at %s, or set VEYLIX_BIN_DIR", p.binary()))
	check("veylixd.prev kept for rollback", fileExists(p.prevBin()),
		"no previous binary yet: the first update will create one")
	check("backup script present", fileExists(backupScript()),
		fmt.Sprintf("expected %s; it ships in the release bundle as "+
			"deploy/veylix-backup.sh - set VEYLIX_DEPLOY_DIR (or re-install). "+
			"Without it BOTH update and rollback refuse to run", backupScript()))
	check("database present", fileExists(p.dbFile()),
		fmt.Sprintf("no database at %s: run install.sh, or check VEYLIX_DATA_DIR", p.dbFile()))
	check("enc.key present", fileExists(p.encKey()),
		"missing enc.key makes every sealed panel token unrecoverable; restore it from backup")
	check("session.key present", fileExists(p.sessKey()),
		"missing session.key only signs everyone out")
	check("environment file present", fileExists(p.envFile()),
		fmt.Sprintf("expected %s (mode 0600); re-render it with 'vpn-platformctl repair'", p.envFile()))
	check("systemd unit present", fileExists(p.UnitPath),
		fmt.Sprintf("expected %s; re-render it with 'vpn-platformctl repair'", p.UnitPath))

	if have("systemctl") {
		check("service active", serviceActive(r, "veylixd"),
			"start it with: systemctl start veylixd")
	} else {
		skipf("service active", "systemctl is not available on this host")
	}

	rep, probeErr := probeReady(o.base)
	if probeErr != nil {
		failures++
		out("FAIL  readiness")
		out("      -> %v", probeErr)
	} else {
		check("readiness reports ready", rep.Status == "ready",
			"/api/ready did not report status=ready")
		check("interface is embedded", rep.UI != "not_embedded",
			"this binary was built without the frontend; install a release artifact")
		out("      schema_version=%d ui=%s", rep.SchemaVersion, rep.UI)
	}

	out("")
	if failures > 0 {
		return fmt.Errorf("doctor found %d problem(s)", failures)
	}
	out("doctor: all checks passed")
	return nil
}
