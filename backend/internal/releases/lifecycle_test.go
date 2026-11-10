package releases

// D-098 — the stale-VPN lifecycle sweep.
//
// These tests exist because every failure mode here is "a customer loses a VPN
// they did not expect to lose". Each case therefore asserts BOTH halves: what the
// sweep moved, and — just as important — what it left alone.

import (
	"strings"
	"testing"
	"time"

	"veylix/backend/internal/audit"
)

// `day` and `env` already exist in service_test.go; this file only adds the
// lifecycle helpers so the two do not disagree about what an hour is.

// seedStaleVPN creates a VPN already holding `status` for `ageDays`.
func (e *env) seedStaleVPN(email, status string, ageDays int64) int64 {
	e.t.Helper()
	since := *e.now - ageDays*day
	return e.mustExec(`INSERT INTO vpn_instances
		(owner_user_id, server_id, offer_id, panel_email, panel_sub_id,
		 status, status_changed_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		e.clientID, e.serverID, e.offerID, email, "sub-"+email,
		status, since, since, since)
}

// statusOf reads one VPN's status, so an assertion states the fact rather than
// passing because the row vanished.
func (e *env) statusOf(vpnID int64) string {
	e.t.Helper()
	var st string
	if err := e.st.DB().QueryRow(
		`SELECT status FROM vpn_instances WHERE id = ?`, vpnID).Scan(&st); err != nil {
		return ""
	}
	return st
}

func TestStaleSuspendedVPNIsSetAsideAfterItsWindow(t *testing.T) {
	e := newEnv(t)

	old := e.seedStaleVPN("old@example.com_free_1", "SUSPENDED", StaleSuspendedDays+1)
	fresh := e.seedStaleVPN("fresh@example.com_free_1", "SUSPENDED", StaleSuspendedDays-1)

	moved, err := e.svc.ExpireStale(e.ctx, false)
	if err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}
	if len(moved) != 1 || moved[0].VPNID != old {
		t.Fatalf("moved = %+v, want only VPN %d", moved, old)
	}
	if got := e.statusOf(old); got != "RELEASED" {
		t.Errorf("old VPN status = %q, want RELEASED", got)
	}
	// The near-miss must be untouched: a window is not a suggestion.
	if got := e.statusOf(fresh); got != "SUSPENDED" {
		t.Errorf("fresh VPN status = %q, want SUSPENDED (inside its window)", got)
	}
}

func TestExpiredVPNUsesTheShorterWindow(t *testing.T) {
	e := newEnv(t)

	// Expired long enough for its own 30-day window but not the 90-day one.
	due := e.seedStaleVPN("due@example.com_free_1", "EXPIRED", StaleExpiredDays+1)
	waiting := e.seedStaleVPN("waiting@example.com_free_1", "EXPIRED", StaleExpiredDays-1)

	moved, err := e.svc.ExpireStale(e.ctx, false)
	if err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}
	if len(moved) != 1 || moved[0].VPNID != due {
		t.Fatalf("moved = %+v, want only VPN %d", moved, due)
	}
	if got := e.statusOf(waiting); got != "EXPIRED" {
		t.Errorf("waiting VPN status = %q, want EXPIRED", got)
	}
}

// The single most dangerous case: a panel outage is NOT an ending. A VPN the
// panel cannot be reached for is exactly what D-091.5 exists to protect, and a
// lifecycle rule that swept it would delete customers' VPNs during an outage.
func TestServerUnavailableIsNeverSwept(t *testing.T) {
	e := newEnv(t)

	gone := e.seedStaleVPN("gone@example.com_free_1", "SERVER_UNAVAILABLE", 365)

	moved, err := e.svc.ExpireStale(e.ctx, false)
	if err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}
	if len(moved) != 0 {
		t.Fatalf("moved = %+v, want nothing (SERVER_UNAVAILABLE is an outage)", moved)
	}
	if got := e.statusOf(gone); got != "SERVER_UNAVAILABLE" {
		t.Errorf("status = %q, want SERVER_UNAVAILABLE — an outage must never be swept", got)
	}
}

// Report mode must change NOTHING. It exists so an operator can preview the rule
// on a live box, so "found it" and "took it" have to be different outcomes.
func TestReportModeChangesNothing(t *testing.T) {
	e := newEnv(t)

	vpn := e.seedStaleVPN("old@example.com_free_1", "SUSPENDED", StaleSuspendedDays+5)

	moved, err := e.svc.ExpireStale(e.ctx, true)
	if err != nil {
		t.Fatalf("ExpireStale(dryRun): %v", err)
	}
	if len(moved) != 1 || moved[0].VPNID != vpn {
		t.Fatalf("moved = %+v, want the VPN to be REPORTED", moved)
	}
	if got := e.statusOf(vpn); got != "SUSPENDED" {
		t.Errorf("status = %q, want SUSPENDED — report mode must not act", got)
	}
	// And no release may exist: an undo window that opened is an act.
	if n := e.countRows("vpn_releases", "vpn_instance_id = ?", vpn); n != 0 {
		t.Errorf("vpn_releases rows = %d, want 0 in report mode", n)
	}
}

// The audit trail has to distinguish a preview from an act, or the preview
// poisons the history it was meant to protect.
func TestReportModeAuditSaysNothingChanged(t *testing.T) {
	e := newEnv(t)

	e.seedStaleVPN("old@example.com_free_1", "SUSPENDED", StaleSuspendedDays+5)
	if _, err := e.svc.ExpireStale(e.ctx, true); err != nil {
		t.Fatalf("ExpireStale(dryRun): %v", err)
	}
	if n := e.auditCount(audit.ActionVPNStaleReleased); n != 1 {
		t.Fatalf("audit entries = %d, want 1 report entry", n)
	}
	ctx := e.auditContext(audit.ActionVPNStaleReleased)
	if !strings.Contains(ctx, "REPORT ONLY") {
		t.Errorf("audit context = %q, want it to say REPORT ONLY", ctx)
	}
	if !strings.Contains(ctx, `"dry_run":true`) {
		t.Errorf("audit context = %q, want dry_run true", ctx)
	}
}

// Enforce mode must record that the panel client was KEPT (D-027.3) and that the
// undo window opened, because the whole safety argument rests on those two facts.
func TestEnforceModeRecordsUndoWindowAndKeptClient(t *testing.T) {
	e := newEnv(t)

	vpn := e.seedStaleVPN("old@example.com_free_1", "SUSPENDED", StaleSuspendedDays+5)
	if _, err := e.svc.ExpireStale(e.ctx, false); err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}

	ctx := e.auditContext(audit.ActionVPNStaleReleased)
	if !strings.Contains(ctx, `"panel_client_kept":true`) {
		t.Errorf("audit context = %q, want panel_client_kept true", ctx)
	}
	if !strings.Contains(ctx, `"dry_run":false`) {
		t.Errorf("audit context = %q, want dry_run false", ctx)
	}
	if !strings.Contains(ctx, `"undo_window_days":3`) {
		t.Errorf("audit context = %q, want the 3-day undo window", ctx)
	}

	// The undo window is real: the release row exists with a deadline 3 days out.
	var deadline, released int64
	if err := e.st.DB().QueryRow(
		`SELECT released_at, restore_deadline FROM vpn_releases WHERE vpn_instance_id = ?`,
		vpn).Scan(&released, &deadline); err != nil {
		t.Fatalf("release row: %v", err)
	}
	if got, want := deadline-released, int64(3)*day; got != want {
		t.Errorf("restore window = %d, want %d", got, want)
	}
}

// The row must still exist. A lifecycle sweep that deleted it would be a purge
// wearing a different name.
func TestSweepDoesNotDeleteTheRow(t *testing.T) {
	e := newEnv(t)

	vpn := e.seedStaleVPN("old@example.com_free_1", "SUSPENDED", StaleSuspendedDays+5)
	if _, err := e.svc.ExpireStale(e.ctx, false); err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}
	if !e.vpnExists(vpn) {
		t.Fatal("the VPN row was deleted — the sweep must only move it to RELEASED")
	}
	if got := e.statusOf(vpn); got != "RELEASED" {
		t.Errorf("status = %q, want RELEASED", got)
	}
}

// A healthy VPN is invisible to the sweep, whatever its age.
func TestActiveVPNIsNeverSwept(t *testing.T) {
	e := newEnv(t)

	ancient := e.seedStaleVPN("ancient@example.com_free_1", "ACTIVE", 3650)

	moved, err := e.svc.ExpireStale(e.ctx, false)
	if err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}
	if len(moved) != 0 {
		t.Fatalf("moved = %+v, want nothing (ACTIVE is not a lifecycle status)", moved)
	}
	if got := e.statusOf(ancient); got != "ACTIVE" {
		t.Errorf("status = %q, want ACTIVE", got)
	}
}

// A row whose status clock was never set is skipped, so a half-migrated database
// reports nothing rather than treating every row as infinitely old.
func TestUnsetClockIsNotTreatedAsAncient(t *testing.T) {
	e := newEnv(t)

	vpn := e.mustExec(`INSERT INTO vpn_instances
		(owner_user_id, server_id, offer_id, panel_email, panel_sub_id,
		 status, status_changed_at, created_at, updated_at)
		VALUES (?,?,?,?,?, 'SUSPENDED', 0, ?, ?)`,
		e.clientID, e.serverID, e.offerID, "noclock@example.com_free_1",
		"sub-no", *e.now, *e.now)

	moved, err := e.svc.ExpireStale(e.ctx, false)
	if err != nil {
		t.Fatalf("ExpireStale: %v", err)
	}
	if len(moved) != 0 {
		t.Fatalf("moved = %+v, want nothing (status_changed_at = 0)", moved)
	}
	if got := e.statusOf(vpn); got != "SUSPENDED" {
		t.Errorf("status = %q, want SUSPENDED", got)
	}
}

// The store refuses the statuses that have no business in a sweep, so a future
// caller cannot add SERVER_UNAVAILABLE to the list and start deleting VPNs
// during an outage.
func TestStoreRefusesServerUnavailableAsLifecycleStatus(t *testing.T) {
	e := newEnv(t)

	for _, bad := range []string{"SERVER_UNAVAILABLE", "RELEASED"} {
		if _, err := e.st.StaleVPNs(e.ctx, []string{bad}, *e.now-day, *e.now, 10); err == nil {
			t.Errorf("StaleVPNs(%q) = nil error, want a refusal", bad)
		}
	}
}

// Moving a VPN must happen once. A second tick sees RELEASED and does nothing,
// so a slow purge cannot open two undo windows on one VPN.
func TestSweepIsIdempotent(t *testing.T) {
	e := newEnv(t)

	e.seedStaleVPN("old@example.com_free_1", "SUSPENDED", StaleSuspendedDays+5)

	first, err := e.svc.ExpireStale(e.ctx, false)
	if err != nil {
		t.Fatalf("first ExpireStale: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first sweep moved %d, want 1", len(first))
	}
	// Advance a tick and sweep again.
	*e.now += int64(time.Hour / time.Second)
	second, err := e.svc.ExpireStale(e.ctx, false)
	if err != nil {
		t.Fatalf("second ExpireStale: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("second sweep moved %d, want 0 (already RELEASED)", len(second))
	}
}
