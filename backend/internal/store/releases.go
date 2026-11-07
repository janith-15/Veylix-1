package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ReleaseGraceDays is the undo window for a released VPN (D-027.2).
//
// Three days is an owner decision, not a knob: it is long enough for a client
// to notice they released the wrong VPN and short enough that a depleted record
// does not sit in the catalogue forever.
const ReleaseGraceDays = 3

// Release reasons.
const (
	ReleaseReasonQuotaExhausted = "quota_exhausted"
	ReleaseReasonClientChoice   = "client_choice"
)

// Errors the transport maps to specific statuses.
var (
	ErrReleaseNotFound = errors.New("store: release not found")
	// ErrReleaseState means the release is not in a state that allows the action
	// (already restored, already removed, or past its undo window).
	ErrReleaseState  = errors.New("store: release is not in a restorable state")
	ErrReleaseExists = errors.New("store: vpn is already released")
)

// ReleaseRow is one release record as returned to the API.
//
// `RestoreDeadline` is what the UI counts down; a NULL `RestoredAt` with a NULL
// `RemovedAt` means "pending removal" — the only state where undo is possible.
type ReleaseRow struct {
	ID              int64  `json:"id"`
	VPNID           int64  `json:"vpn_id"`
	ClientUserID    int64  `json:"client_user_id"`
	Reason          string `json:"reason"`
	ReleasedAt      int64  `json:"released_at"`
	RestoreDeadline int64  `json:"restore_deadline"`
	RestoredAt      *int64 `json:"restored_at"`
	RemovedAt       *int64 `json:"removed_at"`
	// PanelClientKept is always 1 and is on the wire so a caller can prove the
	// panel client was never deleted (D-027.3).
	PanelClientKept bool `json:"panel_client_kept"`
	// PanelEmail and ServerID say WHICH panel client this release was. They are
	// carried on the row rather than joined from `vpn_instances` because a
	// purged VPN has no row left to join (I-33, D-087): without them a removed
	// release would survive but identify nothing.
	PanelEmail string `json:"panel_email"`
	ServerID   int64  `json:"server_id"`
}

const releaseColumns = `r.id, r.vpn_instance_id, r.client_user_id, r.reason,
	r.released_at, r.restore_deadline, r.restored_at, r.removed_at, r.panel_client_kept,
	r.panel_email, r.server_id`

func scanRelease(sc interface{ Scan(...any) error }) (*ReleaseRow, error) {
	var r ReleaseRow
	var vpnID sql.NullInt64
	var restored, removed sql.NullInt64
	var kept int
	if err := sc.Scan(&r.ID, &vpnID, &r.ClientUserID, &r.Reason, &r.ReleasedAt,
		&r.RestoreDeadline, &restored, &removed, &kept,
		&r.PanelEmail, &r.ServerID); err != nil {
		return nil, err
	}
	// A purged release has no VPN to point at (migration 0023, ON DELETE SET
	// NULL). It surfaces as 0 rather than failing the scan: the tombstone must
	// stay readable precisely when the row it described is gone.
	if vpnID.Valid {
		r.VPNID = vpnID.Int64
	}
	if restored.Valid {
		v := restored.Int64
		r.RestoredAt = &v
	}
	if removed.Valid {
		v := removed.Int64
		r.RemovedAt = &v
	}
	r.PanelClientKept = kept == 1
	return &r, nil
}

// CreateRelease marks a VPN as released and starts its grace period.
//
// Rules encoded here (D-027):
//   - a VPN can only be released once while a release is pending — releasing an
//     already-released VPN would restart the 3-day clock and let a client keep a
//     depleted VPN indefinitely;
//   - the FREE claim is **not** released (D-027.4): releasing does not refund a
//     monthly slot, otherwise release/re-claim becomes a free unlimited loop;
//   - the VPN row stays intact. Removal happens later, and locally only.
func (s *Store) CreateRelease(ctx context.Context, vpnID, clientUserID int64, reason string) (*ReleaseRow, error) {
	return s.CreateReleaseAt(ctx, vpnID, clientUserID, reason, nowUnix())
}

// CreateReleaseAt is CreateRelease with an explicit clock, so the whole D-027
// lifecycle (release, restore, purge) can be driven by one injected time in
// tests instead of three independent reads of the wall clock.
func (s *Store) CreateReleaseAt(ctx context.Context, vpnID, clientUserID int64, reason string, now int64) (*ReleaseRow, error) {
	if reason == "" {
		reason = ReleaseReasonQuotaExhausted
	}
	var id int64
	deadline := now + int64(ReleaseGraceDays)*24*60*60

	err := s.Tx(ctx, func(tx *sql.Tx) error {
		// Ownership: a client may only release their own VPN.
		var owner int64
		err := tx.QueryRowContext(ctx,
			`SELECT owner_user_id FROM vpn_instances WHERE id = ?`, vpnID).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoRows
		}
		if err != nil {
			return fmt.Errorf("store: lookup vpn owner: %w", err)
		}
		if owner != clientUserID {
			return ErrNoRows // same answer as "missing": ids stay unprobeable
		}

		var pending int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM vpn_releases
			  WHERE vpn_instance_id = ? AND restored_at IS NULL AND removed_at IS NULL`,
			vpnID).Scan(&pending); err != nil {
			return fmt.Errorf("store: check pending release: %w", err)
		}
		if pending > 0 {
			return ErrReleaseExists
		}

		// One read gives the previous status AND the panel identity. The status
		// is the only record of why the VPN was set aside (a depleted VPN and an
		// expired one are released the same way but mean different things to
		// whoever reads the trail in three months). The identity is stored on the
		// release row by value (I-33, D-087): once the VPN is purged there is no
		// row left to join, so a release that did not capture it at creation
		// would survive as an unidentifiable timestamp.
		var previousStatus string
		var panelEmail string
		var serverID int64
		if err := tx.QueryRowContext(ctx,
			`SELECT status, panel_email, server_id FROM vpn_instances WHERE id = ?`,
			vpnID).Scan(&previousStatus, &panelEmail, &serverID); err != nil {
			return fmt.Errorf("store: read vpn status: %w", err)
		}

		res, err := tx.ExecContext(ctx,
			`INSERT INTO vpn_releases
			   (vpn_instance_id, client_user_id, reason, released_at, restore_deadline,
			    panel_client_kept, status_before, panel_email, server_id)
			 VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?)`,
			vpnID, clientUserID, reason, now, deadline, previousStatus, panelEmail, serverID)
		if err != nil {
			return fmt.Errorf("store: create release: %w", err)
		}
		id, _ = res.LastInsertId()

		// The release is only half-done without this. Leaving the row ACTIVE is
		// what let the next sync pass see a VPN missing from the panel and mark it
		// SERVER_UNAVAILABLE — telling the customer the server had failed during
		// an undo window they themselves opened.
		//
		// RELEASED is written here rather than by the caller so the row and its
		// release record can never disagree about whether the VPN was set aside.
		if _, err := tx.ExecContext(ctx,
			`UPDATE vpn_instances SET status = 'RELEASED', updated_at = ?
			  WHERE id = ? AND status NOT IN ('REVOKED', 'REMOVED_FROM_CATALOG')`,
			now, vpnID); err != nil {
			return fmt.Errorf("store: mark vpn released: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ReleaseByID(ctx, id)
}

// ReleaseByID reads one release.
func (s *Store) ReleaseByID(ctx context.Context, id int64) (*ReleaseRow, error) {
	r, err := scanRelease(s.db.QueryRowContext(ctx,
		`SELECT `+releaseColumns+` FROM vpn_releases r WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReleaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: read release: %w", err)
	}
	return r, nil
}

// ActiveReleaseForVPN returns the pending (restorable) release of a VPN, if any.
func (s *Store) ActiveReleaseForVPN(ctx context.Context, vpnID int64) (*ReleaseRow, error) {
	r, err := scanRelease(s.db.QueryRowContext(ctx,
		`SELECT `+releaseColumns+` FROM vpn_releases r
		  WHERE r.vpn_instance_id = ? AND r.restored_at IS NULL AND r.removed_at IS NULL
		  ORDER BY r.id DESC LIMIT 1`, vpnID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReleaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: read active release: %w", err)
	}
	return r, nil
}

// ReleasesForClient lists a client's releases, newest first.
func (s *Store) ReleasesForClient(ctx context.Context, clientUserID int64) ([]ReleaseRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+releaseColumns+` FROM vpn_releases r
		  WHERE r.client_user_id = ? ORDER BY r.id DESC LIMIT 100`, clientUserID)
	if err != nil {
		return nil, fmt.Errorf("store: list releases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ReleaseRow, 0, 8)
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan release: %w", err)
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// RestoreRelease undoes a release inside the grace window (D-027.2).
//
// It refuses once the deadline has passed: allowing a restore after removal
// would mean re-creating a VPN Veylix no longer tracks, and the panel client may
// by then belong to somebody else. `now` is a parameter so a test can sit on
// either side of the boundary without sleeping for three days.
func (s *Store) RestoreRelease(ctx context.Context, releaseID, clientUserID int64, now int64) (*ReleaseRow, error) {
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		var owner, deadline int64
		var restored, removed sql.NullInt64
		err := tx.QueryRowContext(ctx,
			`SELECT client_user_id, restore_deadline, restored_at, removed_at
			   FROM vpn_releases WHERE id = ?`, releaseID).
			Scan(&owner, &deadline, &restored, &removed)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrReleaseNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lookup release: %w", err)
		}
		if owner != clientUserID {
			return ErrReleaseNotFound // not ours: indistinguishable from missing
		}
		if restored.Valid || removed.Valid {
			return ErrReleaseState
		}
		if now > deadline {
			return fmt.Errorf("%w: the 3-day undo window has closed", ErrReleaseState)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE vpn_releases SET restored_at = ? WHERE id = ?`, now, releaseID); err != nil {
			return fmt.Errorf("store: restore release: %w", err)
		}
		// Undo the RELEASED marker the release wrote. The VPN goes back to ACTIVE
		// because that is the only state a restored client can legitimately
		// resume in; if it was suspended or depleted, the next sync pass observes
		// that again from the panel rather than guessing it here.
		if _, err := tx.ExecContext(ctx,
			`UPDATE vpn_instances SET status = 'ACTIVE', updated_at = ?
			  WHERE id = (SELECT vpn_instance_id FROM vpn_releases WHERE id = ?)
			    AND status = 'RELEASED'`, now, releaseID); err != nil {
			return fmt.Errorf("store: unmark released vpn: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ReleaseByID(ctx, releaseID)
}

// DueRemovals lists releases whose grace period has expired and that have not
// been restored or removed yet.
func (s *Store) DueRemovals(ctx context.Context, now int64, limit int) ([]ReleaseRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+releaseColumns+` FROM vpn_releases r
		  WHERE r.restored_at IS NULL AND r.removed_at IS NULL AND r.restore_deadline < ?
		  ORDER BY r.restore_deadline LIMIT ?`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list due removals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ReleaseRow, 0, limit)
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan due release: %w", err)
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// PurgedVPN is the record of a VPN that purge removed (D-027).
//
// It exists because `vpn_instances` is HARD DELETED on purge: after that moment
// the audit trail could name only a numeric id, so "which VPN was this, whose was
// it, and what had they bought" became unanswerable — exactly the question an
// operator asks months later. The snapshot is read inside the same transaction
// that deletes the row, so it can never describe a row other than the one removed.
type PurgedVPN struct {
	VPNID      int64
	PanelEmail string
	OwnerID    int64
	OwnerEmail string
	SellerID   int64
	ServerID   int64
	OfferID    int64
	OfferName  string
	// Status is the row's state at the moment of purge (normally RELEASED).
	Status string
	// StatusBeforeRelease is what it was before the release marker replaced it
	// (TRAFFIC_DEPLETED, EXPIRED, …). Without this the audit trail would say a
	// depleted VPN was "released" and lose why it was set aside in the first
	// place — which is the question an operator actually asks later.
	StatusBeforeRelease string
}

// RemoveReleasedVPN deletes Veylix's record of a VPN whose grace period expired.
//
// The panel is NOT touched (D-027.3): the 3X-UI client keeps working, and the
// owner asked for exactly that — the panel is where they reuse or clear it. This
// is the one place in the product where a VPN row disappears, so everything it
// touches is deleted explicitly and in dependency order inside ONE transaction;
// a half-removed release would leave an assignment pointing at nothing.
func (s *Store) RemoveReleasedVPN(ctx context.Context, releaseID int64, now int64) (*PurgedVPN, error) {
	var snap *PurgedVPN
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		var vpnIDNull sql.NullInt64
		var restored, removed sql.NullInt64
		err := tx.QueryRowContext(ctx,
			`SELECT vpn_instance_id, restored_at, removed_at FROM vpn_releases WHERE id = ?`,
			releaseID).Scan(&vpnIDNull, &restored, &removed)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrReleaseNotFound
		}
		if err != nil {
			return fmt.Errorf("store: lookup release: %w", err)
		}
		if restored.Valid {
			// Restored in time: nothing to remove.
			return ErrReleaseState
		}
		if removed.Valid {
			return nil // already done — idempotent so a retried tick is harmless
		}
		// An unmoved release always names a live VPN: ON DELETE SET NULL only
		// blanks the pointer when the vpn_instances row is deleted, and that can
		// only happen through the purge below. Reaching here with a NULL means
		// the row was removed without being stamped, which would silently leave
		// a dangling "pending" release — refuse rather than guess.
		if !vpnIDNull.Valid {
			return fmt.Errorf("store: release %d is not pending but has no vpn (%w)",
				releaseID, ErrReleaseState)
		}
		vpnID := vpnIDNull.Int64

		// Snapshot BEFORE the delete, inside this transaction: afterwards the row
		// (and its panel identity) is gone, and an audit entry can no longer say
		// whose VPN was removed or what they had bought. Every column is
		// COALESCEd so a legacy row with a NULL join still yields a usable record
		// instead of failing the purge.
		//
		// StatusBeforeRelease is read from the release row's own `status_before`
		// column, because the VPN's live status is now RELEASED and no longer says
		// WHY it was set aside. A legacy row released before that column existed
		// yields '' and the audit entry simply omits the field.
		var p PurgedVPN
		if err := tx.QueryRowContext(ctx,
			`SELECT i.id, i.panel_email, i.owner_user_id,
			        COALESCE(u.email, ''), COALESCE(a.seller_user_id, 0), i.server_id,
			        COALESCE(i.offer_id, 0), COALESCE(o.name, ''), i.status,
			        COALESCE(r.status_before, '')
			   FROM vpn_instances i
			   LEFT JOIN vpn_assignments a ON a.vpn_instance_id = i.id
			   LEFT JOIN users u           ON u.id = i.owner_user_id
			   LEFT JOIN vpn_offers o      ON o.id = i.offer_id
			   LEFT JOIN vpn_releases r    ON r.id = ?
			  WHERE i.id = ?`, releaseID, vpnID).Scan(
			&p.VPNID, &p.PanelEmail, &p.OwnerID,
			&p.OwnerEmail, &p.SellerID, &p.ServerID,
			&p.OfferID, &p.OfferName, &p.Status,
			&p.StatusBeforeRelease); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: snapshot vpn for purge: %w", err)
		}
		if p.VPNID != 0 {
			snap = &p
		}

		// Children first: assignments and timeline rows reference the VPN.
		if _, err := tx.ExecContext(ctx,
			`UPDATE free_claims SET vpn_instance_id = NULL WHERE vpn_instance_id = ?`, vpnID); err != nil {
			return fmt.Errorf("store: detach free claim: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM vpn_assignments WHERE vpn_instance_id = ?`, vpnID); err != nil {
			return fmt.Errorf("store: delete assignment: %w", err)
		}
		// The tombstone is stamped AND given the panel identity in the same
		// statement (I-33, D-087). From the next statement onward there is no
		// `vpn_instances` row left to ask, so the record has to carry by value
		// what it used to point at: the release row is the client's timeline
		// entry for a VPN that no longer exists.
		//
		// `vpn_instance_id` is deliberately NOT nulled here by hand: the FK is
		// ON DELETE SET NULL since migration 0023, so the DELETE below clears it.
		// Writing it twice would be one more thing that can disagree.
		if _, err := tx.ExecContext(ctx,
			`UPDATE vpn_releases
			    SET removed_at = ?, panel_email = ?, server_id = ?
			  WHERE id = ?`, now, p.PanelEmail, p.ServerID, releaseID); err != nil {
			return fmt.Errorf("store: stamp release removed: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM vpn_instances WHERE id = ?`, vpnID); err != nil {
			return fmt.Errorf("store: delete vpn: %w", err)
		}
		return nil
	})
	return snap, err
}
