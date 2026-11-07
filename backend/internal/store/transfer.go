// transfer.go owns moving an already-assigned VPN from one client account to
// another (D-064).
//
// Why this is a separate file from assignment.go: the two writes answer
// different questions and must not share an entry point. An assignment asks
// "does anybody hold this panel client yet?" — and when somebody does, the
// correct answer is a refusal, because recording a second owner is how one
// physical VPN becomes two rows (the failure `panelClientHeld` exists to
// prevent). A transfer asks a different question: "who holds it now, and who
// should hold it instead?". It is only legal precisely because somebody already
// holds it.
//
// Everything a transfer touches lives in one transaction, and it touches NO
// panel. 3X-UI does not model "which Veylix account manages this client", so
// the panel has nothing to learn; the client's UUID, sub-id, limits and
// connection links stay valid for its whole life. That also means a transfer
// works during a panel outage and can never half-apply on a remote system.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"veylix/backend/internal/domain"
)

// Errors a transfer returns, each mapped to its own status code so an operator
// is told what is actually wrong instead of a generic refusal.
var (
	// ErrTransferTarget means the row is not a VPN Veylix knows about.
	ErrTransferTarget = errors.New("store: vpn not found")
	// ErrTransferSelf means the VPN already belongs to that customer.
	ErrTransferSelf = errors.New("store: this VPN already belongs to that customer")
	// ErrTransferTerminal means the VPN is REVOKED or REMOVED_FROM_CATALOG — it
	// is gone for good, so handing it to somebody would give them nothing.
	ErrTransferTerminal = errors.New("store: this VPN can no longer be transferred")
	// ErrTransferNotReady means the panel never confirmed the create, so there
	// is no working credential to hand over.
	ErrTransferNotReady = errors.New("store: this VPN is not confirmed by the panel yet")
	// ErrTransferNotOnPanel means the panel positively does not hold the client.
	// The VPN is dead; transferring a dead VPN only moves the disappointment.
	ErrTransferNotOnPanel = errors.New("store: the panel no longer holds this VPN")
	// ErrTransferReleased means the VPN sits inside its 3-day undo window, where
	// the old owner could still restore it — which would leave two owners.
	ErrTransferReleased = errors.New("store: this VPN is released and can still be restored")
	// ErrTransferChanged means the owner changed under us between the read and
	// the write (two consoles, one click each).
	ErrTransferChanged = errors.New("store: the owner of this VPN changed during the transfer")
)

// TransferVPNInput is one transfer request.
type TransferVPNInput struct {
	// VPNID is the VPN being moved.
	VPNID int64
	// ToClientID is the account that receives it; must be an ACTIVE client.
	ToClientID int64
	// ActorID is the operator performing the move, recorded in the ledger and
	// used as the seller for a platform issue.
	ActorID int64
	// Reason is free text kept in the history. Optional by decision (Q5).
	Reason string
	// SellerID names a reseller to credit for the new holder. Zero means a
	// platform issue, credited to ActorID — the same rule AssignPanelClient
	// already applies (D-063).
	SellerID int64
}

// TransferParty names one side of a transfer with the values a screen needs, so
// the console never has to resolve two ids before it can say what happened.
type TransferParty struct {
	UserID   int64  `json:"user_id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	SellerID int64  `json:"seller_id"`
}

// TransferResult is what a completed transfer produced.
type TransferResult struct {
	VPNID      int64         `json:"vpn_id"`
	ServerID   int64         `json:"server_id"`
	PanelEmail string        `json:"panel_email"`
	OfferID    int64         `json:"offer_id"`
	Kind       string        `json:"kind"`
	From       TransferParty `json:"from"`
	To         TransferParty `json:"to"`
	// FreeClaimMoved reports whether a FREE quota slot followed the VPN. It is
	// on the wire because the two caps only stay honest if the slot moves, and
	// an operator deserves to see that it did.
	FreeClaimMoved bool `json:"free_claim_moved"`
}

// TransferRow is one entry of a VPN's transfer history.
type TransferRow struct {
	ID         int64  `json:"id"`
	VPNID      int64  `json:"vpn_id"`
	ServerID   int64  `json:"server_id"`
	PanelEmail string `json:"panel_email"`
	FromUserID int64  `json:"from_user_id"`
	FromEmail  string `json:"from_email"`
	ToUserID   int64  `json:"to_user_id"`
	ToEmail    string `json:"to_email"`
	ActorID    int64  `json:"actor_id"`
	ActorEmail string `json:"actor_email"`
	OfferID    int64  `json:"offer_id"`
	Kind       string `json:"kind"`
	Reason     string `json:"reason"`
	FreeClaim  bool   `json:"free_claim_moved"`
	CreatedAt  int64  `json:"created_at"`
}

// transferStatuses lists the lifecycle states a VPN may be in and still be
// handed to somebody else.
//
// The list is an allow-list, not a deny-list, on purpose: a status added to
// `domain` in future is refused until somebody decides what a transfer of it
// means. Every exclusion in checkTransferable names the reason, because
// "cannot transfer" with no explanation is the note that makes an operator
// revoke a working VPN to get past it.
var transferStatuses = map[domain.VPNStatus]bool{
	domain.VPNActive:            true,
	domain.VPNTrafficDepleted:   true,
	domain.VPNExpired:           true,
	domain.VPNSuspended:         true,
	domain.VPNServerUnavailable: true,
}

// checkTransferable refuses a VPN whose transfer would hand over something
// broken, and says which.
func checkTransferable(status domain.VPNStatus) error {
	switch status {
	case domain.VPNRevoked, domain.VPNRemovedFromCatalog:
		return ErrTransferTerminal
	case domain.VPNPending, domain.VPNRevocationPending:
		return ErrTransferNotReady
	case domain.VPNReleased:
		return ErrTransferReleased
	}
	if !transferStatuses[status] {
		return ErrTransferTerminal
	}
	return nil
}

// TransferVPN moves an already-assigned VPN to another client account.
//
// Rules (D-064), all enforced inside one transaction so the panel client, the
// owner, the assignment history and the FREE claim ledger cannot disagree
// afterwards:
//
//   - the target must be an ACTIVE `client` account (ErrAssignTarget);
//   - the VPN must not already belong to that account (ErrTransferSelf);
//   - the VPN must be in a state that still works (see transferStatuses);
//   - the VPN must not be sitting in its 3-day undo window, where the previous
//     owner could restore it and produce two owners (ErrTransferReleased);
//   - the row's `owner_user_id` guards the UPDATE, so a concurrent transfer
//     cannot silently move a VPN somebody else already moved
//     (ErrTransferChanged);
//   - the panel client's name, sub-id, package, expiry and limits are NOT
//     touched — the VPN does not change hands physically;
//   - the tag is NOT touched — a change of owner is not a new sale, so
//     re-pricing it would rewrite what the first customer paid for (D-025.2);
//   - a FREE claim slot follows the VPN, so both customers' caps stay correct
//     (D-025.4/.5).
//
// No panel call is made. See the file comment in transfer.go.
func (s *Store) TransferVPN(ctx context.Context, in TransferVPNInput) (*TransferResult, error) {
	if in.VPNID <= 0 {
		return nil, ErrTransferTarget
	}
	if in.ToClientID <= 0 {
		return nil, ErrAssignTarget
	}
	if in.ActorID <= 0 {
		return nil, fmt.Errorf("store: transfer: an acting operator is required")
	}

	reason := strings.TrimSpace(in.Reason)
	if len(reason) > 500 {
		// The reason is a note, not a document: truncating here keeps a pasted
		// paragraph from bloating a ledger row a UI has to render.
		reason = reason[:500]
	}

	res := &TransferResult{VPNID: in.VPNID}
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		return s.transferInTx(ctx, tx, in, reason, res)
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// transferInTx is the body of TransferVPN, kept separate so the function above
// stays a readable statement of the contract rather than 120 lines of SQL.
func (s *Store) transferInTx(ctx context.Context, tx *sql.Tx,
	in TransferVPNInput, reason string, res *TransferResult) error {
	// ---- 1. The VPN, and who holds it right now. ------------------------
	var (
		ownerID    int64
		serverID   int64
		panelEmail string
		status     string
		offerID    sql.NullInt64
		kind       string
		fromEmail  string
		fromName   string
		fromSeller int64
	)
	err := tx.QueryRowContext(ctx,
		`SELECT i.owner_user_id, i.server_id, i.panel_email, i.status, i.offer_id,
		        COALESCE(o.kind, ''), COALESCE(u.email, ''), COALESCE(u.display_name, ''),
		        COALESCE(a.seller_user_id, 0)
		   FROM vpn_instances i
		   LEFT JOIN vpn_offers o ON o.id = i.offer_id
		   LEFT JOIN users u ON u.id = i.owner_user_id
		   LEFT JOIN vpn_assignments a
		          ON a.vpn_instance_id = i.id AND a.status IN ('ACTIVE', 'PENDING')
		  WHERE i.id = ?`, in.VPNID).
		Scan(&ownerID, &serverID, &panelEmail, &status, &offerID, &kind,
			&fromEmail, &fromName, &fromSeller)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTransferTarget
	}
	if err != nil {
		return fmt.Errorf("store: transfer: read vpn: %w", err)
	}
	if ownerID == in.ToClientID {
		return ErrTransferSelf
	}
	if err := checkTransferable(domain.VPNStatus(status)); err != nil {
		return err
	}

	// ---- 2. Not inside a restorable release window. ---------------------
	// Released VPNs are already refused by status above; this also catches a
	// release row that outlived a status change, which is the shape the D-027
	// defect took.
	var pendingReleases int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_releases
		  WHERE vpn_instance_id = ? AND restored_at IS NULL AND removed_at IS NULL`,
		in.VPNID).Scan(&pendingReleases); err != nil {
		return fmt.Errorf("store: transfer: read releases: %w", err)
	}
	if pendingReleases > 0 {
		return ErrTransferReleased
	}

	// ---- 3. The recipient must be an ACTIVE client. ---------------------
	var targetRole, targetStatus, targetEmail, targetName string
	err = tx.QueryRowContext(ctx,
		`SELECT role, status, COALESCE(email, ''), COALESCE(display_name, '')
		   FROM users WHERE id = ?`, in.ToClientID).
		Scan(&targetRole, &targetStatus, &targetEmail, &targetName)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAssignTarget
	}
	if err != nil {
		return fmt.Errorf("store: transfer: lookup target: %w", err)
	}
	// A DELETED or blocked account keeps its rows but must not receive new
	// ones: handing a VPN to a blocked customer leaves it working for nobody.
	if targetRole != "client" || targetStatus != "ACTIVE" {
		return ErrAssignTarget
	}
	return s.applyTransfer(ctx, tx, in, reason, res,
		ownerID, serverID, panelEmail, offerID.Int64, kind, fromSeller,
		fromEmail, fromName, targetEmail, targetName)
}

// applyTransfer performs the writes, once every rule has been checked.
//
// It is split out purely to keep each step small enough to read: one owner
// change, one assignment swap, one claim move, one history row — in that order,
// because each later step depends on the one before it.
func (s *Store) applyTransfer(ctx context.Context, tx *sql.Tx,
	in TransferVPNInput, reason string, res *TransferResult,
	ownerID, serverID int64, panelEmail string, offerID int64, kind string,
	fromSeller int64, fromEmail, fromName, targetEmail, targetName string) error {
	now := time.Now().Unix()

	// ---- 4. Move the owner, guarded by the owner we just read. ----------
	moved, err := tx.ExecContext(ctx,
		`UPDATE vpn_instances
		    SET owner_user_id = ?, updated_at = ?
		  WHERE id = ? AND owner_user_id = ?`,
		in.ToClientID, now, in.VPNID, ownerID)
	if err != nil {
		return fmt.Errorf("store: transfer: update owner: %w", err)
	}
	if n, _ := moved.RowsAffected(); n != 1 {
		return ErrTransferChanged
	}

	// ---- 5. Close the old assignment, open the new one. -----------------
	// Without this both customers' premium counts would include the VPN (the
	// count reads live assignment rows), so one VPN would look like two.
	if _, err := tx.ExecContext(ctx,
		`UPDATE vpn_assignments SET status = 'TRANSFERRED', updated_at = ?
		  WHERE vpn_instance_id = ? AND status IN ('ACTIVE', 'PENDING')`,
		now, in.VPNID); err != nil {
		return fmt.Errorf("store: transfer: close assignment: %w", err)
	}
	toSeller := in.SellerID
	if toSeller <= 0 {
		toSeller = in.ActorID
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO vpn_assignments
		   (vpn_instance_id, client_user_id, seller_user_id, status, started_at, created_at, updated_at)
		 VALUES (?, ?, ?, 'ACTIVE', ?, ?, ?)`,
		in.VPNID, in.ToClientID, toSeller, now, now, now); err != nil {
		return fmt.Errorf("store: transfer: open assignment: %w", err)
	}

	// ---- 6. A FREE claim slot follows the VPN. --------------------------
	// Matched on `vpn_instance_id` and a live row, so a claim that was already
	// released — or that belongs to a different VPN — is never moved.
	var claimID int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM free_claims
		  WHERE vpn_instance_id = ? AND released_at IS NULL
		  ORDER BY id DESC LIMIT 1`, in.VPNID).Scan(&claimID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: transfer: read free claim: %w", err)
	}
	if claimID > 0 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE free_claims
			    SET client_user_id = ?, transferred_at = ?, transferred_from_user_id = ?
			  WHERE id = ? AND released_at IS NULL`,
			in.ToClientID, now, ownerID, claimID); err != nil {
			return fmt.Errorf("store: transfer: move free claim: %w", err)
		}
		res.FreeClaimMoved = true
	}

	// ---- 7. The durable history. ----------------------------------------
	var claimForRow any
	if claimID > 0 {
		claimForRow = claimID
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO vpn_transfers
		   (vpn_instance_id, server_id, panel_email, from_user_id, to_user_id,
		    from_seller_id, to_seller_id, offer_id, kind, actor_id, reason,
		    free_claim_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.VPNID, serverID, panelEmail, ownerID, in.ToClientID,
		fromSeller, toSeller, offerID, kind, in.ActorID, reason,
		claimForRow, now); err != nil {
		return fmt.Errorf("store: transfer: record history: %w", err)
	}

	res.ServerID = serverID
	res.PanelEmail = panelEmail
	res.OfferID = offerID
	res.Kind = kind
	if res.Kind == "" {
		// The projection treats an owned row with no package as PREMIUM
		// (D-063); the ledger must not disagree with the screen.
		res.Kind = "PREMIUM"
	}
	res.From = TransferParty{UserID: ownerID, Email: fromEmail, Name: fromName, SellerID: fromSeller}
	res.To = TransferParty{UserID: in.ToClientID, Email: targetEmail, Name: targetName, SellerID: toSeller}
	return nil
}

// TransfersForVPN returns one VPN's transfer history, newest first.
//
// The e-mail addresses are joined rather than stored twice, because a customer
// who later changes their address should be shown under the address they use
// now while the transfer date still tells the story. The ids never change, so
// nothing is lost either way.
func (s *Store) TransfersForVPN(ctx context.Context, vpnID int64) ([]TransferRow, error) {
	if vpnID <= 0 {
		return nil, ErrTransferTarget
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT t.id, t.vpn_instance_id, t.server_id, t.panel_email,
		        t.from_user_id, COALESCE(fu.email, ''), COALESCE(fu.display_name, ''),
		        t.to_user_id,   COALESCE(tu.email, ''), COALESCE(tu.display_name, ''),
		        t.actor_id,     COALESCE(au.email, ''),
		        t.offer_id, t.kind, t.reason, t.free_claim_id, t.created_at
		   FROM vpn_transfers t
		   LEFT JOIN users fu ON fu.id = t.from_user_id
		   LEFT JOIN users tu ON tu.id = t.to_user_id
		   LEFT JOIN users au ON au.id = t.actor_id
		  WHERE t.vpn_instance_id = ?
		  ORDER BY t.created_at DESC, t.id DESC`, vpnID)
	if err != nil {
		return nil, fmt.Errorf("store: list transfers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]TransferRow, 0, 4)
	for rows.Next() {
		var r TransferRow
		var claim sql.NullInt64
		if err := rows.Scan(&r.ID, &r.VPNID, &r.ServerID, &r.PanelEmail,
			&r.FromUserID, &r.FromEmail, new(string),
			&r.ToUserID, &r.ToEmail, new(string),
			&r.ActorID, &r.ActorEmail,
			&r.OfferID, &r.Kind, &r.Reason, &claim, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan transfer: %w", err)
		}
		r.FreeClaim = claim.Valid
		out = append(out, r)
	}
	return out, rows.Err()
}
