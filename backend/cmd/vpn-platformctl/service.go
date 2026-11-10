package main

import (
	"errors"
	"fmt"
	"strings"
)

// cmdService runs one systemd verb and then VERIFIES the result.
//
// The verification is the point. `systemctl start` returns 0 as soon as the unit
// is queued, and this codebase has already been bitten once by trusting a remote
// 200 instead of reading the state back (D-114). So each verb is followed by a
// different question:
//
//	start   - is it now active?
//	stop    - is it now inactive?
//	restart - is it active, and is the service actually answering /api/ready?
//	enable  - is it now enabled?
//	disable - is it now disabled?
//
// The service is named by the unit file the installer writes, not by a constant,
// so a host that renamed it keeps working.
func cmdService(p paths, args []string, verb string) error {
	fs := newFlagSet(verb)
	o := addCommonFlags(fs)
	rest, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	_ = rest

	if !have("systemctl") {
		return fmt.Errorf("systemctl is not available on this host - "+
			"%s manages the service through systemd; start veylixd directly instead", verb)
	}

	r := runner{dryRun: o.dryRun}
	unit := unitName(p.UnitPath)

	step("%s %s", verb, unit)
	// `disable --now` is the documented way to stop AND stop-on-boot in one
	// transaction; plain `stop` leaves the unit enabled, which silently restarts
	// a service the operator believed they had turned off.
	if _, err := r.run("systemctl", verb, unit); err != nil {
		return err
	}
	if o.dryRun {
		out("[dry-run] would then verify the new state")
		return nil
	}

	step("verify")
	active, enabled := serviceState(r, unit)

	switch verb {
	case "start", "restart":
		if !active {
			return fmt.Errorf("%s %s returned success but the unit is NOT active", verb, unit)
		}
		out("%s is active", unit)
		if verb == "restart" {
			// A unit can be "active" while the process is still booting, so ask
			// the process itself. This is the only honest proof it serves.
			if err := probeReadyStrict(o.base); err != nil {
				return fmt.Errorf("%s is active but the service is not ready: %w", unit, err)
			}
			out("the service answers /api/ready")
		}
	case "stop":
		if active {
			return fmt.Errorf("stop %s returned success but the unit is STILL active", unit)
		}
		out("%s is inactive", unit)
	case "enable":
		if !enabled {
			return fmt.Errorf("enable %s returned success but the unit is NOT enabled", unit)
		}
		out("%s will start on boot", unit)
	case "disable":
		if enabled {
			return fmt.Errorf("disable %s returned success but the unit is STILL enabled", unit)
		}
		out("%s will not start on boot", unit)
	}
	return nil
}

// unitName turns a path into the name systemctl expects. systemctl accepts a
// full path, but `--now`/verification messages read better as a unit name, and a
// path with a stray directory has bitten operators before.
func unitName(unitPath string) string {
	base := unitPath
	for i := len(base) - 1; i >= 0; i-- {
		if base[i] == '/' {
			base = base[i+1:]
			break
		}
	}
	if base == "" {
		return "veylixd.service"
	}
	return base
}

// serviceState asks systemd two independent questions.
//
// `is-enabled` exits non-zero for the disabled/masked cases and prints the state
// on stdout, so the output is read rather than the exit code: refusing to treat
// "disabled" as an error is what makes the check usable.
func serviceState(r runner, unit string) (active, enabled bool) {
	if text, err := r.run("systemctl", "is-active", unit); err == nil {
		active = strings.TrimSpace(text) == "active"
	}
	if text, err := r.run("systemctl", "is-enabled", unit); err == nil {
		enabled = strings.TrimSpace(text) == "enabled"
	}
	return active, enabled
}

// probeReadyStrict is probeReady with the "is it actually serving?" question
// answered as an error. probeReady already distinguishes transport/status
// failures, so this only needs to pass them through.
func probeReadyStrict(base string) error {
	rep, err := probeReady(base)
	if err != nil {
		return err
	}
	if rep.SchemaVersion == 0 && rep.Version == "" {
		return errors.New("the service answered /api/ready but reported nothing identifiable")
	}
	return nil
}
