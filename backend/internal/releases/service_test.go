package releases

// Tests for the D-027 lifecycle: release -> 3-day undo window -> local removal.
//
// Why this suite exists: `RemoveReleasedVPN` performs the ONE hard delete of a
// `vpn_instances` row in the whole product (store/releases.go). Every other
// destructive path — quota, payment confirmation, revocation — already had
// coverage; this one had none.
//
// Two rules shape every test below:
//
//   - **Clock, never sleep.** `Service.SetClock` and the `...At`/`now` store
//     parameters exist so the 3-day boundary can be crossed instantly and
//     deterministically. No test here sleeps.
//   - **Isolated database only.** `t.TempDir()` per test, never `./var/` and
//     never `./.veylix-secrets/`. Rule 05 is explicit that a verification script
//     once deleted live data here (VPN_FIX_LEDGER).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// day is the grace period in seconds, spelled once so the boundary tests read
// as arithmetic rather than magic numbers.
const day = int64(24 * 60 * 60)

// env is one isolated database plus the service under test.
type env struct {
	t   *testing.T
	ctx context.Context

	st    *store.Store
	audit *audit.Logger
	svc   *Service

	// now is the test-controlled clock. `svc.SetClock` reads it through the
	// closure installed in newEnv, so a test advances time by assigning `now`.
	now *int64

	// seeded identities
	clientID int64
	otherID  int64
	reseller int64
	admin    int64
	serverID int64
	offerID  int64
}

// newEnv builds an isolated database, seeds the minimum rows the lifecycle
// touches, and returns a service whose clock the test owns.
func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()

	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "releases.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.MigrateFast(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	e := &env{t: t, ctx: ctx, st: st, audit: audit.New(st.DB())}
	db := st.DB()

	mustExec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := db.ExecContext(ctx, q, args...)
		if err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
		id, _ := res.LastInsertId()
		return id
	}

	e.serverID = mustExec(`INSERT INTO server_integrations
		(name, panel_url, token_ciphertext, status, created_at, updated_at)
		VALUES ('srv','https://panel.example:2053/base', x'00', 'ACTIVE', 1, 1)`)
	inboundID := mustExec(`INSERT INTO inbounds (server_id, remote_id, remark, protocol, port, synced_at)
		VALUES (?, 1, '(IN)-vip', 'vless', 443, 1)`, e.serverID)
	e.offerID = mustExec(`INSERT INTO vpn_offers
		(kind, name, server_id, inbound_id, traffic_gb, duration_days, device_limit, active, created_at, updated_at)
		VALUES ('PREMIUM','Premium 50',?,?,50,30,2,1,1,1)`, e.serverID, inboundID)

	e.admin = mustExec(`INSERT INTO users (email, role, status, created_at, updated_at)
		VALUES ('root@example.test','super_admin','ACTIVE',1,1)`)
	e.reseller = mustExec(`INSERT INTO users (email, role, status, created_at, updated_at)
		VALUES ('reseller@example.test','admin','ACTIVE',1,1)`)
	e.clientID = mustExec(`INSERT INTO users (email, role, status, created_at, updated_at)
		VALUES ('client@example.test','client','ACTIVE',1,1)`)
	e.otherID = mustExec(`INSERT INTO users (email, role, status, created_at, updated_at)
		VALUES ('other@example.test','client','ACTIVE',1,1)`)

	start := int64(1_700_000_000)
	e.now = &start
	e.svc = New(st, e.audit)
	e.svc.SetClock(func() int64 { return *e.now })
	return e
}

// mustExec runs a statement and returns its last insert id.
func (e *env) mustExec(q string, args ...any) int64 {
	e.t.Helper()
	res, err := e.st.DB().ExecContext(e.ctx, q, args...)
	if err != nil {
		e.t.Fatalf("exec %q: %v", q, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedVPN inserts one ACTIVE VPN owned by ownerID and returns its id.
func (e *env) seedVPN(ownerID int64, email string) int64 {
	e.t.Helper()
	return e.mustExec(`INSERT INTO vpn_instances
		(owner_user_id, server_id, offer_id, panel_email, panel_sub_id, status, created_at, updated_at)
		VALUES (?,?,?,?,?, 'ACTIVE', ?, ?)`,
		ownerID, e.serverID, e.offerID, email, "sub-"+email, *e.now, *e.now)
}

// seedAssignment gives a VPN a reseller assignment row, so a purge test can
// prove the child row is deleted in dependency order rather than orphaning.
func (e *env) seedAssignment(vpnID, clientID, sellerID int64) int64 {
	e.t.Helper()
	return e.mustExec(`INSERT INTO vpn_assignments
		(vpn_instance_id, client_user_id, seller_user_id, status, price_cents, currency,
		 created_at, updated_at)
		VALUES (?,?,?, 'ACTIVE', 1500, 'LKR', ?, ?)`,
		vpnID, clientID, sellerID, *e.now, *e.now)
}

// seedFreeClaim links a FREE claim row to a VPN, so a purge test can prove the
// claim is detached rather than cascade-deleted (the client keeps their slot
// history; only the pointer is cleared).
func (e *env) seedFreeClaim(vpnID, clientID int64) int64 {
	e.t.Helper()
	return e.mustExec(`INSERT INTO free_claims
		(client_user_id, server_id, panel_email, vpn_instance_id, claimed_at)
		VALUES (?,?,?,?,?)`,
		clientID, e.serverID, "vyx-free-"+fmt.Sprint(vpnID), vpnID, *e.now)
}

// vpnStatus reads a VPN row's status, or "" when the row is gone.
func (e *env) vpnStatus(id int64) string {
	e.t.Helper()
	var s string
	if err := e.st.DB().QueryRowContext(e.ctx,
		`SELECT status FROM vpn_instances WHERE id = ?`, id).Scan(&s); err != nil {
		return ""
	}
	return s
}

func (e *env) vpnExists(id int64) bool { return e.vpnStatus(id) != "" }

func (e *env) countRows(table, where string, args ...any) int {
	e.t.Helper()
	var n int
	q := `SELECT COUNT(*) FROM ` + table
	if where != "" {
		q += ` WHERE ` + where
	}
	if err := e.st.DB().QueryRowContext(e.ctx, q, args...).Scan(&n); err != nil {
		e.t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func (e *env) auditCount(action string) int {
	return e.countRows("audit_logs", "action = ?", action)
}

// auditContext returns the raw JSON context of the newest entry for an action.
func (e *env) auditContext(action string) string {
	e.t.Helper()
	var c string
	if err := e.st.DB().QueryRowContext(e.ctx,
		`SELECT context FROM audit_logs WHERE action = ? ORDER BY id DESC LIMIT 1`,
		action).Scan(&c); err != nil {
		e.t.Fatalf("read audit context %q: %v", action, err)
	}
	return c
}

// Principals for the seeded users.
func (e *env) clientPrincipal() auth.Principal {
	return auth.Principal{UserID: e.clientID, Role: auth.RoleClient, Email: "client@example.test"}
}

func (e *env) otherClientPrincipal() auth.Principal {
	return auth.Principal{UserID: e.otherID, Role: auth.RoleClient, Email: "other@example.test"}
}

func (e *env) adminPrincipal() auth.Principal {
	return auth.Principal{UserID: e.admin, Role: auth.RoleSuperAdmin, Email: "root@example.test"}
}

func (e *env) resellerPrincipal() auth.Principal {
	return auth.Principal{UserID: e.reseller, Role: auth.RoleAdmin, Email: "reseller@example.test"}
}

// ---------------------------------------------------------------------------
// Release — matrix cases 1 (normal), 2 (unauthorised), 4 (wrong owner),
// 7 (duplicate)
// ---------------------------------------------------------------------------

// TestReleaseOpensThreeDayWindow pins the D-027.2 constant. The deadline is
// arithmetic the UI counts down and `Restore` compares against, so it is worth
// asserting rather than assuming.
func TestReleaseOpensThreeDayWindow(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-1")

	row, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), store.ReleaseReasonQuotaExhausted)
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if got, want := row.RestoreDeadline-row.ReleasedAt, store.ReleaseGraceDays*day; got != want {
		t.Fatalf("grace window = %d seconds, want %d", got, want)
	}
	if row.RestoredAt != nil || row.RemovedAt != nil {
		t.Fatalf("fresh release must be pending: restored=%v removed=%v", row.RestoredAt, row.RemovedAt)
	}
	if !row.PanelClientKept {
		t.Fatal("panel_client_kept must be true: the release never touches 3X-UI (D-027.3)")
	}
}

// TestReleaseMarksVPNReleased closes the precise defect migration 0018 fixed:
// a release that leaves the row ACTIVE makes the next sync pass report
// SERVER_UNAVAILABLE to a customer who simply gave the VPN up.
func TestReleaseMarksVPNReleased(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-2")

	if _, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), ""); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if got := e.vpnStatus(vpnID); got != "RELEASED" {
		t.Fatalf("vpn status = %q, want RELEASED", got)
	}
}

// TestReleaseCapturesStatusBefore makes the audit trail answerable months later:
// "released" alone loses WHY it was set aside.
func TestReleaseCapturesStatusBefore(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-3")
	e.mustExec(`UPDATE vpn_instances SET status = 'TRAFFIC_DEPLETED' WHERE id = ?`, vpnID)

	row, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), store.ReleaseReasonQuotaExhausted)
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	var before string
	if err := e.st.DB().QueryRowContext(e.ctx,
		`SELECT status_before FROM vpn_releases WHERE id = ?`, row.ID).Scan(&before); err != nil {
		t.Fatalf("read status_before: %v", err)
	}
	if before != "TRAFFIC_DEPLETED" {
		t.Fatalf("status_before = %q, want TRAFFIC_DEPLETED", before)
	}
}

// TestReleaseDefaultsReason covers the empty-string branch: a caller that omits
// the reason must not write an empty column.
func TestReleaseDefaultsReason(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-4")

	row, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if row.Reason != store.ReleaseReasonQuotaExhausted {
		t.Fatalf("reason = %q, want %q", row.Reason, store.ReleaseReasonQuotaExhausted)
	}
}

// TestReleaseTwiceIsRefused — matrix case 7. A second release would restart the
// 3-day clock and let a client hold a depleted VPN forever (see the D-027 note
// on CreateRelease).
func TestReleaseTwiceIsRefused(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-5")
	p := e.clientPrincipal()

	if _, err := e.svc.Release(e.ctx, vpnID, p, ""); err != nil {
		t.Fatalf("first Release: %v", err)
	}
	before, err := e.svc.List(e.ctx, p, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Move the clock forward, then try again. If the second call were allowed it
	// would push the deadline out — that is the exact damage being prevented.
	*e.now += day
	if _, err := e.svc.Release(e.ctx, vpnID, p, ""); !errors.Is(err, ErrExists) {
		t.Fatalf("second Release err = %v, want ErrExists", err)
	}

	after, err := e.svc.List(e.ctx, p, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(before) != 1 || len(after) != 1 {
		t.Fatalf("release count = %d then %d, want 1 then 1", len(before), len(after))
	}
	if after[0].RestoreDeadline != before[0].RestoreDeadline {
		t.Fatalf("deadline moved from %d to %d: a refused release must not restart the clock",
			before[0].RestoreDeadline, after[0].RestoreDeadline)
	}
}

// TestReleaseOtherClientsVPNIsNotFound — matrix case 4. The store returns
// ErrNoRows for a foreign owner specifically so ids stay unprobeable: a client
// cannot discover that a VPN id exists by the error they get back.
func TestReleaseOtherClientsVPNIsNotFound(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.otherID, "vyx-6")

	_, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if got := e.vpnStatus(vpnID); got != "ACTIVE" {
		t.Fatalf("vpn status = %q, want ACTIVE: a refused release must not touch the row", got)
	}
	if n := e.countRows("vpn_releases", ""); n != 0 {
		t.Fatalf("release rows = %d, want 0", n)
	}
}

// TestReleaseMissingVPNIsNotFound covers the same answer for a genuinely absent
// id — the two cases must be indistinguishable.
func TestReleaseMissingVPNIsNotFound(t *testing.T) {
	e := newEnv(t)

	if _, err := e.svc.Release(e.ctx, 999_999, e.clientPrincipal(), ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestReleaseWithoutIdentityIsRefused is matrix case 2 at the service boundary:
// a zero UserID cannot own a VPN, so it is refused before the store is asked.
func TestReleaseWithoutIdentityIsRefused(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-7")

	if _, err := e.svc.Release(e.ctx, vpnID, auth.Principal{}, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if got := e.vpnStatus(vpnID); got != "ACTIVE" {
		t.Fatalf("vpn status = %q, want ACTIVE", got)
	}
}

// TestReleaseRecordsAudit proves the privileged action is audited (Rule 03 §8).
func TestReleaseRecordsAudit(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-8")

	if _, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), ""); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if n := e.auditCount(audit.ActionVPNRelease); n != 1 {
		t.Fatalf("release audit entries = %d, want 1", n)
	}
}

// TestReleaseSurvivesNilAuditLogger proves a missing audit logger cannot break
// the operation — `record` is documented as best-effort.
func TestReleaseSurvivesNilAuditLogger(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-9")

	svc := New(e.st, nil)
	svc.SetClock(func() int64 { return *e.now })
	if _, err := svc.Release(e.ctx, vpnID, e.clientPrincipal(), ""); err != nil {
		t.Fatalf("Release with nil audit logger: %v", err)
	}
	if got := e.vpnStatus(vpnID); got != "RELEASED" {
		t.Fatalf("vpn status = %q, want RELEASED", got)
	}
}

// ---------------------------------------------------------------------------
// Restore — the 3-day boundary is the whole contract (D-027.2), so both sides
// of it are asserted explicitly, one second apart.
// ---------------------------------------------------------------------------

// TestRestoreInsideWindowSucceeds sits one second BEFORE the deadline.
func TestRestoreInsideWindowSucceeds(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-10")
	p := e.clientPrincipal()

	rel, err := e.svc.Release(e.ctx, vpnID, p, "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	*e.now = rel.RestoreDeadline - 1
	got, err := e.svc.Restore(e.ctx, rel.ID, p)
	if err != nil {
		t.Fatalf("Restore one second before the deadline: %v", err)
	}
	if got.RestoredAt == nil {
		t.Fatal("restored_at must be set on a successful restore")
	}
	if s := e.vpnStatus(vpnID); s != "ACTIVE" {
		t.Fatalf("vpn status = %q, want ACTIVE: the RELEASED marker must be undone", s)
	}
	if n := e.auditCount(audit.ActionVPNRestore); n != 1 {
		t.Fatalf("restore audit entries = %d, want 1", n)
	}
}

// TestRestoreAtDeadlineSucceeds pins the inclusive edge. `DueRemovals` selects
// `restore_deadline < now` and `RestoreRelease` refuses `now > deadline`, so
// exactly at the deadline both agree the release is still restorable. If those
// two drift apart a release becomes simultaneously "not yet due" and "not
// restorable" — an orphan nobody can act on.
func TestRestoreAtDeadlineSucceeds(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-11")
	p := e.clientPrincipal()

	rel, err := e.svc.Release(e.ctx, vpnID, p, "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	*e.now = rel.RestoreDeadline
	if _, err := e.svc.Restore(e.ctx, rel.ID, p); err != nil {
		t.Fatalf("Restore exactly at the deadline: %v", err)
	}
	if s := e.vpnStatus(vpnID); s != "ACTIVE" {
		t.Fatalf("vpn status = %q, want ACTIVE", s)
	}
}

// TestRestoreAfterDeadlineIsRefused sits one second PAST the deadline and must
// leave the row untouched: a restore after removal would mean re-creating a VPN
// the product no longer tracks, and the panel client may belong to someone else
// by then.
func TestRestoreAfterDeadlineIsRefused(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-12")
	p := e.clientPrincipal()

	rel, err := e.svc.Release(e.ctx, vpnID, p, "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	*e.now = rel.RestoreDeadline + 1
	if _, err := e.svc.Restore(e.ctx, rel.ID, p); !errors.Is(err, ErrState) {
		t.Fatalf("err = %v, want ErrState", err)
	}

	after, err := e.st.ReleaseByID(e.ctx, rel.ID)
	if err != nil {
		t.Fatalf("ReleaseByID: %v", err)
	}
	if after.RestoredAt != nil {
		t.Fatal("a refused restore must not stamp restored_at")
	}
	if s := e.vpnStatus(vpnID); s != "RELEASED" {
		t.Fatalf("vpn status = %q, want RELEASED: the refused restore must not unmark it", s)
	}
}

// TestRestoreIsNotRepeatable — matrix case 7: the second call reports the state
// rather than silently succeeding twice.
func TestRestoreIsNotRepeatable(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-13")
	p := e.clientPrincipal()

	rel, err := e.svc.Release(e.ctx, vpnID, p, "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := e.svc.Restore(e.ctx, rel.ID, p); err != nil {
		t.Fatalf("first Restore: %v", err)
	}
	if _, err := e.svc.Restore(e.ctx, rel.ID, p); !errors.Is(err, ErrState) {
		t.Fatalf("second Restore err = %v, want ErrState", err)
	}
}

// TestRestoreOtherClientsReleaseIsNotFound — matrix case 4. A release id must not
// be probeable by a different client.
func TestRestoreOtherClientsReleaseIsNotFound(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-14")

	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := e.svc.Restore(e.ctx, rel.ID, e.otherClientPrincipal()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if s := e.vpnStatus(vpnID); s != "RELEASED" {
		t.Fatalf("vpn status = %q, want RELEASED", s)
	}
}

// TestRestoreMissingReleaseIsNotFound keeps the two refusals indistinguishable.
func TestRestoreMissingReleaseIsNotFound(t *testing.T) {
	e := newEnv(t)

	if _, err := e.svc.Restore(e.ctx, 999_999, e.clientPrincipal()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// PurgeDue — the only hard delete of a vpn_instances row in the product.
// ---------------------------------------------------------------------------

// TestPurgeSkipsReleaseStillInWindow is the single most important assertion in
// this file: a VPN whose undo window is open must survive the background purge.
// The clock sits one second before the deadline.
func TestPurgeSkipsReleaseStillInWindow(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-20")
	p := e.clientPrincipal()

	rel, err := e.svc.Release(e.ctx, vpnID, p, "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	*e.now = rel.RestoreDeadline - 1
	removed, err := e.svc.PurgeDue(e.ctx)
	if err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0: the undo window is still open", removed)
	}
	if !e.vpnExists(vpnID) {
		t.Fatal("vpn row was deleted while its 3-day undo window was still open")
	}
}

// TestPurgeSkipsReleaseAtDeadline holds the same line from the other side: at
// exactly the deadline the release is still restorable, so the purge must not
// have taken it. This is the mirror of TestRestoreAtDeadlineSucceeds.
func TestPurgeSkipsReleaseAtDeadline(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-21")

	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	*e.now = rel.RestoreDeadline
	removed, err := e.svc.PurgeDue(e.ctx)
	if err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0: at the deadline the release is still restorable", removed)
	}
	if !e.vpnExists(vpnID) {
		t.Fatal("vpn row was deleted at the boundary where restore still succeeds")
	}
}

// TestPurgeRemovesExpiredRelease is the happy path of the destructive branch:
// the row goes, the release is stamped, and the audit trail keeps everything the
// deleted row used to say.
func TestPurgeRemovesExpiredRelease(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-22")
	p := e.clientPrincipal()

	rel, err := e.svc.Release(e.ctx, vpnID, p, store.ReleaseReasonQuotaExhausted)
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	*e.now = rel.RestoreDeadline + 1
	removed, err := e.svc.PurgeDue(e.ctx)
	if err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if e.vpnExists(vpnID) {
		t.Fatal("vpn row survived a purge past its deadline")
	}

	// NOTE: the release tombstone is asserted by TestPurgeCascadesTheReleaseRow
	// instead of here, because today it does NOT survive — see defect I-33. The
	// purge is still correct about WHAT it removed; only the record it leaves
	// behind is wrong. Asserting removed_at here would make this test document
	// the defect as if it were the requirement.
	if n := e.auditCount(audit.ActionVPNPurge); n != 1 {
		t.Fatalf("purge audit entries = %d, want 1", n)
	}
}

// TestPurgeDeletesChildrenInDependencyOrder proves the ONE-transaction delete
// does not orphan a child row. `vpn_assignments.vpn_instance_id` is NOT NULL
// with ON DELETE RESTRICT, so a purge that skipped the child delete would fail
// the whole transaction instead of half-removing the release.
func TestPurgeDeletesChildrenInDependencyOrder(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-23")
	assignID := e.seedAssignment(vpnID, e.clientID, e.reseller)
	claimID := e.seedFreeClaim(vpnID, e.clientID)

	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	*e.now = rel.RestoreDeadline + 1
	if _, err := e.svc.PurgeDue(e.ctx); err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}

	if n := e.countRows("vpn_assignments", "id = ?", assignID); n != 0 {
		t.Fatalf("assignment row still exists after purge: %d", n)
	}

	// The FREE claim is DETACHED, not deleted: releasing does not refund the
	// monthly slot (D-027.4), so the claim history has to survive the purge.
	var detached sql.NullInt64
	if err := e.st.DB().QueryRowContext(e.ctx,
		`SELECT vpn_instance_id FROM free_claims WHERE id = ?`, claimID).Scan(&detached); err != nil {
		t.Fatalf("read free claim: %v", err)
	}
	if detached.Valid {
		t.Fatalf("free claim still points at vpn %d after purge; it must be NULL", detached.Int64)
	}
}

// TestPurgeIsIdempotent — matrix case 7. A retried background tick must be
// harmless: the second run sees removed_at stamped and reports nothing removed.
func TestPurgeIsIdempotent(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-24")

	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	*e.now = rel.RestoreDeadline + 1
	first, err := e.svc.PurgeDue(e.ctx)
	if err != nil {
		t.Fatalf("first PurgeDue: %v", err)
	}
	second, err := e.svc.PurgeDue(e.ctx)
	if err != nil {
		t.Fatalf("second PurgeDue: %v", err)
	}
	if first != 1 || second != 0 {
		t.Fatalf("removed counts = %d then %d, want 1 then 0", first, second)
	}
	if n := e.auditCount(audit.ActionVPNPurge); n != 1 {
		t.Fatalf("purge audit entries = %d, want 1: a retried tick must not duplicate the trail", n)
	}
}

// TestPurgeSkipsReleaseRestoredSinceSelection covers the `continue` branch in
// PurgeDue (store returns ErrReleaseState when the release was restored between
// the SELECT and the DELETE). A restore must never be undone by a purge that
// already had the row in hand.
func TestPurgeSkipsReleaseRestoredSinceSelection(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-25")
	p := e.clientPrincipal()

	rel, err := e.svc.Release(e.ctx, vpnID, p, "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	// RemoveReleasedVPN itself refuses a restored release; reaching that refusal
	// from PurgeDue is the race being closed. Simulate the race exactly as the
	// store sees it: select due, then restore, then purge.
	*e.now = rel.RestoreDeadline + 1
	due, err := e.st.DueRemovals(e.ctx, *e.now, 100)
	if err != nil {
		t.Fatalf("DueRemovals: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("due releases = %d, want 1", len(due))
	}

	// A restore at the deadline is still legal, so rewind, restore, then advance.
	*e.now = rel.RestoreDeadline
	if _, err := e.svc.Restore(e.ctx, rel.ID, p); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if _, err := e.st.RemoveReleasedVPN(e.ctx, rel.ID, rel.RestoreDeadline+1); !errors.Is(err, store.ErrReleaseState) {
		t.Fatalf("RemoveReleasedVPN on a restored release err = %v, want ErrReleaseState", err)
	}
	if !e.vpnExists(vpnID) {
		t.Fatal("vpn row was deleted after being restored in time")
	}
	if s := e.vpnStatus(vpnID); s != "ACTIVE" {
		t.Fatalf("vpn status = %q, want ACTIVE", s)
	}
}

// TestPurgeSkipsRestoredRelease is the same guarantee through the public
// PurgeDue entry point: a restored release is no longer due.
func TestPurgeSkipsRestoredRelease(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-26")
	p := e.clientPrincipal()

	rel, err := e.svc.Release(e.ctx, vpnID, p, "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := e.svc.Restore(e.ctx, rel.ID, p); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	*e.now = rel.RestoreDeadline + day
	removed, err := e.svc.PurgeDue(e.ctx)
	if err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0: a restored release is not due", removed)
	}
	if !e.vpnExists(vpnID) {
		t.Fatal("restored vpn was purged anyway")
	}
}

// TestPurgeAuditNamesWhatWasDeleted is why PurgedVPN exists. `vpn_instances` is
// HARD DELETED, so after the purge the audit trail is the only remaining record
// of which VPN it was, whose it was and what they had bought. Without the
// snapshot the trail names a number and nothing else.
func TestPurgeAuditNamesWhatWasDeleted(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-named")
	e.mustExec(`UPDATE vpn_instances SET status = 'TRAFFIC_DEPLETED' WHERE id = ?`, vpnID)
	e.seedAssignment(vpnID, e.clientID, e.reseller)

	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	*e.now = rel.RestoreDeadline + 1
	if _, err := e.svc.PurgeDue(e.ctx); err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}

	got := e.auditContext(audit.ActionVPNPurge)
	for _, want := range []string{
		`"panel_email":"vyx-named"`,                      // which VPN
		`"owner_email":"client@example.test"`,            // whose
		`"seller_user_id":`,                              // who sold it
		`"offer_name":"Premium 50"`,                      // what they bought
		`"vpn_status_before_release":"TRAFFIC_DEPLETED"`, // why it was set aside
		`"panel_client_kept":true`,                       // D-027.3
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("purge audit context is missing %s\nfull context: %s", want, got)
		}
	}
}

// TestPurgeAuditHasNoActor asserts the deliberate omission: a scheduled purge
// has no human behind it, so recording the client as the actor would be a lie.
func TestPurgeAuditHasNoActor(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-actorless")

	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	*e.now = rel.RestoreDeadline + 1
	if _, err := e.svc.PurgeDue(e.ctx); err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}

	var actor sql.NullInt64
	if err := e.st.DB().QueryRowContext(e.ctx,
		`SELECT actor_id FROM audit_logs WHERE action = ? ORDER BY id DESC LIMIT 1`,
		audit.ActionVPNPurge).Scan(&actor); err != nil {
		t.Fatalf("read purge audit actor: %v", err)
	}
	if actor.Valid {
		t.Fatalf("purge audit actor = %d, want NULL: a scheduled purge has no human actor", actor.Int64)
	}
}

// ---------------------------------------------------------------------------
// List — scoping rules. A wrong-owner read must return EMPTY, not an error, so
// a reseller cannot enumerate another reseller's clients by probing ids.
// ---------------------------------------------------------------------------

// TestListClientIsForcedToOwnTimeline — a client asking for another id still
// gets their own releases. The parameter is ignored, not trusted.
func TestListClientIsForcedToOwnTimeline(t *testing.T) {
	e := newEnv(t)
	mine := e.seedVPN(e.clientID, "vyx-mine")
	theirs := e.seedVPN(e.otherID, "vyx-theirs")

	if _, err := e.svc.Release(e.ctx, mine, e.clientPrincipal(), ""); err != nil {
		t.Fatalf("Release mine: %v", err)
	}
	if _, err := e.svc.Release(e.ctx, theirs, e.otherClientPrincipal(), ""); err != nil {
		t.Fatalf("Release theirs: %v", err)
	}

	got, err := e.svc.List(e.ctx, e.clientPrincipal(), e.otherID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("release rows = %d, want 1 (only the caller's own)", len(got))
	}
	if got[0].VPNID != mine {
		t.Fatalf("listed vpn = %d, want %d", got[0].VPNID, mine)
	}
}

// TestListResellerCannotReadAnotherClientsTimeline — matrix case 4, and the
// reason the refusal is an empty slice rather than an error: an error would
// confirm the id exists.
func TestListResellerCannotReadAnotherClientsTimeline(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-scoped")
	if _, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), ""); err != nil {
		t.Fatalf("Release: %v", err)
	}

	got, err := e.svc.List(e.ctx, e.resellerPrincipal(), e.clientID)
	if err != nil {
		t.Fatalf("List must not error on a wrong-owner read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("release rows = %d, want 0 for a reseller reading another reseller's client", len(got))
	}
}

// TestListSuperAdminCanReadAnyClientTimeline keeps the operator escape hatch
// honest: the same call that is empty for a reseller returns data for the owner
// of the platform.
func TestListSuperAdminCanReadAnyClientTimeline(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-admin-read")
	if _, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), ""); err != nil {
		t.Fatalf("Release: %v", err)
	}

	got, err := e.svc.List(e.ctx, e.adminPrincipal(), e.clientID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("release rows = %d, want 1 for a super admin", len(got))
	}
}

// TestListEmptyForClientWithNoReleases is matrix case 8: an empty timeline is an
// empty slice, never a nil that renders as something else downstream.
func TestListEmptyForClientWithNoReleases(t *testing.T) {
	e := newEnv(t)

	got, err := e.svc.List(e.ctx, e.clientPrincipal(), 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("release rows = %d, want 0", len(got))
	}
	if got == nil {
		t.Fatal("List returned nil, want an empty slice: a nil marshals to null, not []")
	}
}

// ---------------------------------------------------------------------------
// Defect I-33 — the cascade that erases the release trail
// ---------------------------------------------------------------------------

// TestPurgeRetainsTheReleaseTrail is the acceptance test for I-33 (D-087).
//
// The defect it fixed: `vpn_releases.vpn_instance_id` was declared `NOT NULL
// REFERENCES vpn_instances (id) ON DELETE CASCADE` (migration 0006), and
// `store.Open` applies `_pragma=foreign_keys(ON)` to every connection.
// `RemoveReleasedVPN` therefore did two contradictory things inside one
// transaction:
//
//	UPDATE vpn_releases SET removed_at = ? WHERE id = ?   -- intend to keep a tombstone
//	DELETE FROM vpn_instances WHERE id = ?                -- cascade erased that row
//
// The second statement destroyed the very row the first one had just stamped, so
// `vpn_releases` emptied and the client's release history vanished at the moment
// it became most relevant. Migration 0023 changes the FK to `ON DELETE SET NULL`,
// so the tombstone now survives with a NULL pointer.
//
// This test was written RED and skipped while the defect existed; it must stay
// green from now on.
func TestPurgeRetainsTheReleaseTrail(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-trail")

	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	*e.now = rel.RestoreDeadline + 1
	if _, err := e.svc.PurgeDue(e.ctx); err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}

	if n := e.countRows("vpn_releases", "id = ?", rel.ID); n != 1 {
		t.Fatalf("release row survived = %d, want 1: the removed tombstone must outlive the vpn row", n)
	}
	got, err := e.st.ReleaseByID(e.ctx, rel.ID)
	if err != nil {
		t.Fatalf("ReleaseByID after purge: %v", err)
	}
	if got.RemovedAt == nil {
		t.Fatal("removed_at must be readable after the purge")
	}
	// The pointer is cleared by the FK, not by hand: that is what "SET NULL"
	// buys us, and asserting it here proves migration 0023 actually applied
	// rather than the row merely surviving by accident.
	if got.VPNID != 0 {
		t.Fatalf("vpn_id = %d after purge, want 0 (the FK must have SET NULL)", got.VPNID)
	}
}

// TestPurgeNoLongerCascadesTheReleaseRow pins the mechanism of the fix.
//
// While I-33 was open this test asserted the OPPOSITE (0 rows, "if this now
// fails, I-33 is fixed") so the regression was understood rather than merely
// observed. It is rewritten here to assert the fixed behaviour, which is the
// signal the fix landed.
func TestPurgeNoLongerCascadesTheReleaseRow(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-cascade")

	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	*e.now = rel.RestoreDeadline + 1
	if _, err := e.svc.PurgeDue(e.ctx); err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}

	if e.vpnExists(vpnID) {
		t.Fatal("precondition failed: the vpn row should be gone")
	}
	if n := e.countRows("vpn_releases", ""); n != 1 {
		t.Fatalf("release rows = %d, want 1: a purge must leave a tombstone, not erase the trail", n)
	}
	// The audit trail is the other survivor. Both must hold: the audit row is an
	// operator record and the release row is the client's timeline.
	if n := e.auditCount(audit.ActionVPNPurge); n != 1 {
		t.Fatalf("purge audit entries = %d, want 1", n)
	}
}

// TestPurgedReleaseRemembersItsPanelIdentity proves the tombstone still says
// WHICH panel client it was after `vpn_instances` has no row left to ask.
//
// Without this the row would survive but identify nothing: a client looking at
// their history would see a release with no email, and an operator could not
// match it to a 3X-UI client. The identity is stored by value (migration 0023)
// because a pointer to a deleted row cannot resolve.
func TestPurgedReleaseRemembersItsPanelIdentity(t *testing.T) {
	e := newEnv(t)
	const email = "vyx-identity@test.local"
	vpnID := e.seedVPN(e.clientID, email)

	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if rel.PanelEmail != email {
		t.Fatalf("release panel_email = %q, want %q (before any purge)", rel.PanelEmail, email)
	}
	if rel.ServerID != e.serverID {
		t.Fatalf("release server_id = %d, want %d (before any purge)", rel.ServerID, e.serverID)
	}

	*e.now = rel.RestoreDeadline + 1
	if _, err := e.svc.PurgeDue(e.ctx); err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}

	got, err := e.st.ReleaseByID(e.ctx, rel.ID)
	if err != nil {
		t.Fatalf("ReleaseByID after purge: %v", err)
	}
	if got.PanelEmail != email {
		t.Fatalf("panel_email after purge = %q, want %q: the tombstone must carry the identity by value",
			got.PanelEmail, email)
	}
	if got.ServerID != e.serverID {
		t.Fatalf("server_id after purge = %d, want %d", got.ServerID, e.serverID)
	}
}

// TestPurgedReleaseSurvivesTheClientTimeline is why the fix is worth doing:
// `GET /api/vpn-releases` must be able to show a client that their VPN was
// removed, not just that it was released.
//
// Before I-33 was fixed this endpoint could never return a removed release — the
// row was already gone — so the wire contract's "removed" state was unreachable.
func TestPurgedReleaseSurvivesTheClientTimeline(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-timeline")

	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	*e.now = rel.RestoreDeadline + 1
	if _, err := e.svc.PurgeDue(e.ctx); err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}

	rows, err := e.svc.List(e.ctx, e.clientPrincipal(), 0)
	if err != nil {
		t.Fatalf("List after purge: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("client timeline rows = %d, want 1: the purge erased the client's history", len(rows))
	}
	if rows[0].ID != rel.ID {
		t.Fatalf("timeline row id = %d, want %d", rows[0].ID, rel.ID)
	}
	if rows[0].RemovedAt == nil {
		t.Fatal("the timeline row must be readable as removed")
	}
	if rows[0].RestoredAt != nil {
		t.Fatal("a removed release must not also look restored")
	}
}

// TestReleaseTombstoneIsNotPending guards the partial index from the other side.
//
// `idx_vpn_releases_pending` is `WHERE removed_at IS NULL AND restored_at IS NULL`
// and `DueRemovals` selects on the same predicate. A purged release that leaked
// back into that set would be selected, found already removed, and retried on
// every tick — an unbounded loop after the very first purge.
func TestReleaseTombstoneIsNotPending(t *testing.T) {
	e := newEnv(t)
	vpnID := e.seedVPN(e.clientID, "vyx-notpending")

	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	*e.now = rel.RestoreDeadline + 1
	if _, err := e.svc.PurgeDue(e.ctx); err != nil {
		t.Fatalf("PurgeDue: %v", err)
	}

	due, err := e.st.DueRemovals(e.ctx, *e.now+1, 100)
	if err != nil {
		t.Fatalf("DueRemovals: %v", err)
	}
	for _, r := range due {
		if r.ID == rel.ID {
			t.Fatal("a purged release must not reappear in DueRemovals: the purge would loop forever")
		}
	}
	// And the purge stays idempotent: a second tick removes nothing.
	removed, err := e.svc.PurgeDue(e.ctx)
	if err != nil {
		t.Fatalf("second PurgeDue: %v", err)
	}
	if removed != 0 {
		t.Fatalf("second purge removed %d, want 0", removed)
	}
	if n := e.countRows("vpn_releases", "id = ?", rel.ID); n != 1 {
		t.Fatalf("release row = %d after a second purge, want 1", n)
	}
}

// TestPackageNeverImportsThePanel is the executable form of the decision that
// makes the whole lifecycle safe: nothing in `releases` may call out to 3X-UI,
// so a purge can never delete a panel client however the code is later changed.
//
// It is asserted at rest by scanning this package's own imports rather than by
// mocking a panel: a mock would only prove that today's code does not call it,
// while the import rule proves the code CANNOT without failing to compile here.
// The alternative — a future "helpful" cleanup that deletes the 3X-UI client —
// is exactly what D-027.3 forbids, and it would sail past every behavioural test
// above because the local behaviour would look identical.
func TestPackageNeverImportsThePanel(t *testing.T) {
	const forbidden = "veylix/backend/internal/panel"

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	scanned := 0
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		scanned++
		if strings.Contains(string(src), forbidden) {
			t.Fatalf("%s references %s: releases must never touch the panel (D-027.3)", name, forbidden)
		}
	}
	if scanned == 0 {
		t.Fatal("scanned 0 files: the guard proved nothing")
	}
}

// TestPurgeWorksWithNoPanelConfigured is the behavioural half of the same
// guarantee. There is no panel in this environment at all — no integration
// token, no reachable host — and the full release→purge cycle still completes.
// If a future change made the purge depend on the panel, this test would hang or
// fail, which is the loudest possible warning.
func TestPurgeWorksWithNoPanelConfigured(t *testing.T) {
	e := newEnv(t)
	// Make the only integration unusable. A purge that consulted the panel would
	// now have to fail; a purge that is purely local cannot notice.
	e.mustExec(`UPDATE server_integrations SET status = 'CONNECTION_UNAVAILABLE' WHERE id = ?`, e.serverID)

	vpnID := e.seedVPN(e.clientID, "vyx-nopanel")
	rel, err := e.svc.Release(e.ctx, vpnID, e.clientPrincipal(), "")
	if err != nil {
		t.Fatalf("Release: %v", err)
	}

	*e.now = rel.RestoreDeadline + 1
	removed, err := e.svc.PurgeDue(e.ctx)
	if err != nil {
		t.Fatalf("PurgeDue with no reachable panel: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1: the purge is local only and must not need the panel", removed)
	}
}
