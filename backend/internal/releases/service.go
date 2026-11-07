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

// SetClock replaces the clock (tests only).
func (s *Service) SetClock(now func() int64) { s.now = now }

// Release marks a VPN as released and opens the 3-day undo window.
//
// Only the OWNER may release (a reseller releasing a client's VPN would destroy
// the client's property without their consent). The VPN row and the panel client
// both stay: the grace period exists precisely so nothing is destroyed yet.
func (s *Service) Release(ctx context.Context, vpnID int64, p auth.Principal, reason string) (*store.ReleaseRow, error) {
	if p.UserID <= 0 {
		return nil, ErrNotFound
	}
	// The deadline is computed from the SERVICE's clock so the whole lifecycle
	// (release, restore, purge) agrees on one notion of "now" — and so a test can
	// cross three days without sleeping.
	row, err := s.st.CreateReleaseAt(ctx, vpnID, p.UserID, reason, s.now())
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
	s.record(ctx, audit.Entry{
		ActorID: p.UserID, Action: audit.ActionVPNRelease,
		TargetType: "vpn", TargetID: fmt.Sprint(vpnID),
		Context: map[string]any{
			"reason":           row.Reason,
			"restore_deadline": row.RestoreDeadline,
			"grace":            "3 days",
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
