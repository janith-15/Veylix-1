// Package releases owns the "storage full → release → 3-day undo → local
// removal" lifecycle (D-027).
//
// It is its own package rather than more methods on provisioning because the
// rule it enforces is about the RECORD, not the panel: nothing here ever writes
// to 3X-UI. Keeping that boundary explicit is what stops a future change from
// "helpfully" deleting the panel client when the row goes away (D-027.3).
package releases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// Errors the transport maps to statuses.
var (
	ErrNotFound = errors.New("releases: release not found")
	ErrState    = errors.New("releases: release is not in a restorable state")
	ErrExists   = errors.New("releases: vpn is already released")
	// ErrReasonRequired — an operator acting on someone else's VPN must say why.
	// The owner's own release defaults to "quota exhausted", but a forced release
	// has no honest default: the reason is the only record of intent that the
	// customer and the audit trail will ever see (P-2).
	ErrReasonRequired = errors.New("releases: a reason is required to release another account's VPN")
)

// Service applies the D-027 lifecycle.
type Service struct {
	st    *store.Store
	audit *audit.Logger
	// now is a seam so tests can cross the 3-day boundary instantly.
	now func() int64
}

// New builds the service.
func New(st *store.Store, auditLog *audit.Logger) *Service {
	return &Service{st: st, audit: auditLog, now: func() int64 { return time.Now().Unix() }}
}

// Lifecycle windows (D-098). Stated once, so the sweep, the log line and any
// future screen all count from the same numbers.
const (
	// StaleSuspendedDays — how long a SUSPENDED VPN may sit before it is set
	// aside. A suspension is reversible by design, so this is generous: the
	// point is to stop the account filling with VPNs nobody will resume, not to
	// punish a customer who paused for a while.
	StaleSuspendedDays = 90
	// StaleExpiredDays — how long an EXPIRED VPN may sit. Shorter than suspended
	// because the customer can already re-buy, and an expired row that lingers
	// reads as "still yours" while it serves nobody.
	StaleExpiredDays = 30
	// StaleTrafficDepletedDays — a depleted VPN stays usable-looking until the
	// panel is reset, so it gets the suspended window rather than the shorter one.
	StaleTrafficDepletedDays = 90
)

// lifecycleStatuses are the statuses the sweep may move. SERVER_UNAVAILABLE is
// absent on purpose and refused by the store even if added here later: "Veylix
// cannot see this VPN" is an outage, not an ending (D-091.5, INV-1).
var lifecycleStatuses = []string{
	"SUSPENDED",
	"EXPIRED",
	"TRAFFIC_DEPLETED",
}

// StaleWindow returns how long each status may sit, in seconds.
func StaleWindow(status string) (int64, bool) {
	switch status {
	case "SUSPENDED":
		return StaleSuspendedDays * 86400, true
	case "EXPIRED":
		return StaleExpiredDays * 86400, true
	case "TRAFFIC_DEPLETED":
		return StaleTrafficDepletedDays * 86400, true
	}
	return 0, false
}

// ExpireStale finds VPNs that have been in a stale status longer than its window
// and moves them into the RELEASE path (D-098).
//
// `dryRun` finds exactly the same set and writes an audit entry for each, but
// changes NO VPN. Nothing is set aside, no undo window opens, and the returned
// slice is what WOULD have moved. This is the mode to run for a few days on a
// live box: an operator can read the audit trail and see precisely which VPNs
// the rule is about to take before it takes them.
//
// Why the release path and not a second purge: release already owns the only
// hard delete in the product (D-027), together with its 3-day undo window, its
// audit snapshot and its "the panel client is never deleted" rule. A second
// removal mechanism would duplicate all four and the two would eventually
// disagree. Moving a stale VPN to RELEASED means the customer gets a warning
// and an undo before anything disappears — which is the difference between a
// tidy-up and data loss.
//
// It never deletes, never touches the panel, and never touches a status it does
// not own.
func (s *Service) ExpireStale(ctx context.Context, dryRun bool) ([]store.StaleVPN, error) {
	now := s.now()

	// Query ONCE PER STATUS, each with its OWN cutoff.
	//
	// A single query built from the longest window would silently drop every
	// status whose window is shorter: an EXPIRED VPN is due after 30 days, so a
	// 90-day global cutoff never selects it, and the shorter rule would never
	// fire. That bug is why this loop exists rather than one shared query.
	var candidates []store.StaleVPN
	for _, st := range lifecycleStatuses {
		window, ok := StaleWindow(st)
		if !ok {
			continue
		}
		batch, err := s.st.StaleVPNs(ctx, []string{st}, now-window, now, 100)
		if err != nil {
			return nil, fmt.Errorf("releases: list stale %s: %w", st, err)
		}
		candidates = append(candidates, batch...)
	}

	moved := make([]store.StaleVPN, 0, len(candidates))
	for _, v := range candidates {
		window, ok := StaleWindow(v.Status)
		if !ok {
			// Not a lifecycle status: the store would have refused it, but a
			// future addition must not become an accidental purge rule.
			continue
		}
		if v.AgeSecs < window {
			continue
		}
		if dryRun {
			// Find and report only. The audit entry says so in words, because an
			// operator reading the trail later must not mistake a preview for a
			// removal that happened.
			moved = append(moved, v)
			s.recordStale(ctx, v, window/86400, true)
			continue
		}
		// Release on the customer's behalf: the actor is the SYSTEM, because no
		// human clicked anything. Recording the owner as the actor would be a
		// lie in exactly the way the purge path already avoids.
		if _, err := s.st.CreateReleaseAt(ctx, v.VPNID, v.OwnerID,
			fmt.Sprintf("automatically set aside: %s for %d days", v.Status, window/86400),
			now); err != nil {
			s.recordStaleFailure(ctx, v, err)
			continue
		}
		moved = append(moved, v)
		s.recordStale(ctx, v, window/86400, false)
	}
	return moved, nil
}

// recordStale writes the audit entry for a VPN the sweep set aside.
//
// `dryRun` is part of the RECORD, not just the log: an entry that says "would
// have been set aside" and one that says "was set aside" must be distinguishable
// afterwards, or the preview poisons the history it was meant to protect.
func (s *Service) recordStale(ctx context.Context, v store.StaleVPN, days int64, dryRun bool) {
	scope := "moved to RELEASED — the 3-day undo window is open and the 3X-UI client was not touched (D-027.3, D-098)"
	if dryRun {
		scope = "REPORT ONLY — nothing was changed; this VPN is still " + v.Status + " (D-098)"
	}
	s.record(ctx, audit.Entry{
		Action: audit.ActionVPNStaleReleased, TargetType: "vpn", TargetID: fmt.Sprint(v.VPNID),
		Context: map[string]any{
			"panel_email":       v.PanelEmail,
			"owner_user_id":     v.OwnerID,
			"owner_email":       v.OwnerEmail,
			"status_at_sweep":   v.Status,
			"status_since":      v.StatusSince,
			"held_for_days":     v.AgeSecs / 86400,
			"window_days":       days,
			"panel_client_kept": true,
			"undo_window_days":  store.ReleaseGraceDays,
			"dry_run":           dryRun,
			"scope":             scope,
		},
	})
}

// recordStaleFailure logs a row the sweep could not move, without inventing a
// result. The next tick retries it.
func (s *Service) recordStaleFailure(ctx context.Context, v store.StaleVPN, cause error) {
	s.record(ctx, audit.Entry{
		Action: audit.ActionVPNStaleFailed, TargetType: "vpn", TargetID: fmt.Sprint(v.VPNID),
		Context: map[string]any{
			"panel_email": v.PanelEmail,
			"status":      v.Status,
			"error":       cause.Error(),
			"note":        "the VPN was NOT set aside; the next sweep retries it",
		},
	})
}

// SetClock replaces the clock (tests only).
func (s *Service) SetClock(now func() int64) { s.now = now }

// Release marks a VPN as released and opens the 3-day undo window.
//
// Only the OWNER may release (a reseller releasing a client's VPN would destroy
// the client's property without their consent). The VPN row and the panel client
// both stay: the grace period exists precisely so nothing is destroyed yet.
// Release sets a VPN aside for its owner.
//
// P-2: a Super Admin may release ANY VPN. That is not a privilege escalation —
// the platform owner already holds the panel credential and can act on the client
// directly through 3X-UI — but routing it through Veylix means the act is
// recorded, the 3-day undo window opens, and the customer sees an explained state
// instead of an unexplained disappearance.
//
// A forced release REQUIRES a reason. An operator acting on someone else's
// property must say why, in words that reach the audit trail; the owner's own
// release defaults to "quota exhausted" because that is what it almost always is.
func (s *Service) Release(ctx context.Context, vpnID int64, p auth.Principal, reason string) (*store.ReleaseRow, error) {
	if p.UserID <= 0 {
		return nil, ErrNotFound
	}
	forced := p.IsSuperAdmin()
	if forced && strings.TrimSpace(reason) == "" {
		return nil, ErrReasonRequired
	}
	// The deadline is computed from the SERVICE's clock so the whole lifecycle
	// (release, restore, purge) agrees on one notion of "now" — and so a test can
	// cross three days without sleeping.
	row, err := s.st.CreateReleaseAtAs(ctx, vpnID, p.UserID, reason, s.now(), forced)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNoRows):
			return nil, ErrNotFound
		case errors.Is(err, store.ErrReleaseExists):
			return nil, ErrExists
		default:
			return nil, fmt.Errorf("releases: release vpn %d: %w", vpnID, err)
		}
	}
	// A forced release gets its own action, so "the owner set this aside" and "an
	// operator set this aside on the owner's behalf" are never confused in the
	// trail — the distinction is exactly what someone asks about months later.
	action := audit.ActionVPNRelease
	if forced {
		action = audit.ActionVPNForceRelease
	}
	s.record(ctx, audit.Entry{
		ActorID: p.UserID, Action: action,
		TargetType: "vpn", TargetID: fmt.Sprint(vpnID),
		Context: map[string]any{
			"reason":           row.Reason,
			"restore_deadline": row.RestoreDeadline,
			"grace":            "3 days",
			"force":            forced,
		},
	})
	return row, nil
}

// Restore undoes a release inside the window.
func (s *Service) Restore(ctx context.Context, releaseID int64, p auth.Principal) (*store.ReleaseRow, error) {
	row, err := s.st.RestoreRelease(ctx, releaseID, p.UserID, s.now())
	if err != nil {
		switch {
		case errors.Is(err, store.ErrReleaseNotFound):
			return nil, ErrNotFound
		case errors.Is(err, store.ErrReleaseState):
			return nil, ErrState
		default:
			return nil, fmt.Errorf("releases: restore release %d: %w", releaseID, err)
		}
	}
	s.record(ctx, audit.Entry{
		ActorID: p.UserID, Action: audit.ActionVPNRestore,
		TargetType: "vpn_release", TargetID: fmt.Sprint(releaseID),
		Context: map[string]any{"vpn_id": row.VPNID, "verdict": "restored"},
	})
	return row, nil
}

// List returns a client's releases (or, for an operator, one client's via
// clientUserID). A client may only ask for their own.
func (s *Service) List(ctx context.Context, p auth.Principal, clientUserID int64) ([]store.ReleaseRow, error) {
	if p.IsClient() || clientUserID == 0 {
		clientUserID = p.UserID
	} else if !p.IsSuperAdmin() && p.UserID != clientUserID {
		// A reseller may not read another reseller's client timeline.
		return []store.ReleaseRow{}, nil
	}
	return s.st.ReleasesForClient(ctx, clientUserID)
}

// PurgeDue removes every release whose grace period has expired.
//
// Called from the background loop. For each one it records an audit entry that
// states the panel client was LEFT INTACT (D-027.5) — that sentence is the whole
// point of the decision, and an operator reading the trail later must be able to
// tell "we forgot to clean up the panel" apart from "we deliberately kept it".
func (s *Service) PurgeDue(ctx context.Context) (int, error) {
	now := s.now()
	due, err := s.st.DueRemovals(ctx, now, 100)
	if err != nil {
		return 0, fmt.Errorf("releases: list due removals: %w", err)
	}
	removed := 0
	for _, r := range due {
		snap, err := s.st.RemoveReleasedVPN(ctx, r.ID, now)
		if err != nil {
			if errors.Is(err, store.ErrReleaseState) {
				// Restored between the SELECT and the DELETE: nothing to do.
				continue
			}
			return removed, fmt.Errorf("releases: remove vpn %d: %w", r.VPNID, err)
		}
		removed++
		// No actor: a scheduled purge has no human behind it, and recording the
		// client as the actor would be a lie.
		//
		// The VPN row is gone after this call, so the snapshot taken inside the
		// delete transaction is the ONLY remaining description of what was
		// removed. Without it the trail names a number and nothing else.
		ctxFields := map[string]any{
			"reason":            r.Reason,
			"released_at":       r.ReleasedAt,
			"restore_deadline":  r.RestoreDeadline,
			"panel_client_kept": true,
			"scope":             "local record only — the 3X-UI client was not deleted (D-027.3)",
		}
		if snap != nil {
			ctxFields["panel_email"] = snap.PanelEmail
			ctxFields["owner_user_id"] = snap.OwnerID
			ctxFields["owner_email"] = snap.OwnerEmail
			ctxFields["seller_user_id"] = snap.SellerID
			ctxFields["server_id"] = snap.ServerID
			ctxFields["offer_id"] = snap.OfferID
			ctxFields["offer_name"] = snap.OfferName
			ctxFields["vpn_status_at_purge"] = snap.Status
			if snap.StatusBeforeRelease != "" {
				ctxFields["vpn_status_before_release"] = snap.StatusBeforeRelease
			}
		}
		s.record(ctx, audit.Entry{
			Action: audit.ActionVPNPurge, TargetType: "vpn", TargetID: fmt.Sprint(r.VPNID),
			Context: ctxFields,
		})
	}
	return removed, nil
}

// record writes an audit entry without letting a failure break the operation.
func (s *Service) record(ctx context.Context, e audit.Entry) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, e)
}
