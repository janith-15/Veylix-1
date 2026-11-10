package provisioning

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/domain"
	"veylix/backend/internal/store"
)

// Replace errors, mapped to statuses by the transport.
var (
	// ErrReplaceKindMismatch refuses a replace that would change the kind of the
	// VPN (FREE -> PREMIUM or back). A replace refreshes a VPN; it is not an
	// upgrade path, and silently changing what the customer holds is not a
	// thing an automatic action may do.
	ErrReplaceKindMismatch = errors.New("provisioning: a replacement must keep the same kind as the VPN it replaces")
	// ErrReplaceTargetMismatch refuses a replace that would move the VPN to a
	// different server. The name, the panel and the inbound all belong together.
	ErrReplaceTargetMismatch = errors.New("provisioning: a replacement must stay on the same server")
	// ErrNoClaimRemaining refuses a FREE replace when the monthly claim
	// allowance is spent (D-025.5, Bible §17.18).
	ErrNoClaimRemaining = errors.New("provisioning: no FREE claims remaining this month")
	// ErrVPNNotReplaceable refuses a VPN that is not in a state a replace can
	// start from — you cannot replace something that is already released.
	ErrVPNNotReplaceable = errors.New("provisioning: this VPN cannot be replaced in its current state")
)

// ReplaceResult reports both halves of a replace honestly: the new VPN that was
// built, and the old one that was released only because the new one succeeded.
type ReplaceResult struct {
	// New is the freshly created VPN. Its Recorded/Confirmed fields are the
	// evidence that the panel holds a client — nothing is released without it.
	New *Result `json:"new"`
	// OldVPNID is the VPN that was released, and OldPanelEmail the panel
	// identity it keeps. It is NOT deleted: the 3-day undo window is open.
	OldVPNID      int64  `json:"old_vpn_id"`
	OldPanelEmail string `json:"old_panel_email"`
	// ReleaseID is the release row, so the client can undo it.
	ReleaseID int64 `json:"release_id"`
	// RestoreDeadline is when the old VPN's undo window closes (unix seconds).
	RestoreDeadline int64 `json:"restore_deadline"`
	// ClaimConsumed is true when a FREE monthly claim was spent on this replace.
	// The UI must say so BEFORE the action, and this field is the proof of what
	// happened afterwards.
	ClaimConsumed bool `json:"claim_consumed"`
	// ClaimsRemaining is what is left in the current 30-day window.
	ClaimsRemaining int `json:"claims_remaining"`
}

// ReleaseFunc is the slice of the release lifecycle a replace needs.
//
// Provisioning does not import `releases` — that would put the record lifecycle
// inside the package that talks to the panel, and `releases` deliberately never
// writes to 3X-UI. The dependency is injected as a function instead, so the
// direction stays one-way and a replace can be tested with a recording stub.
type ReleaseFunc func(ctx context.Context, vpnID int64, p auth.Principal, reason string) (*store.ReleaseRow, error)

// ReplaceInput is one replace request.
type ReplaceInput struct {
	// VPNID is the VPN being set aside.
	VPNID int64
	// OfferID overrides the replacement target. 0 means "the VPN's own offer",
	// which is the normal case: the customer wants the same product, renewed.
	OfferID int64
	// Actor is the caller. Ownership, the super-admin override and the
	// reason requirement are all enforced from it.
	Actor auth.Principal
	// Reason is required when an operator replaces somebody else's VPN.
	Reason string
	// FreeMonthlyLimit is the client's effective FREE claim cap; 0 = policy
	// default. Passed in so the cap has exactly one implementation (`store`).
	FreeMonthlyLimit int
}

var sqlErrNoRows = sql.ErrNoRows

// Replace builds a NEW VPN and only then releases the old one (D-112).
//
// The ordering is the entire safety argument. Releasing first and then failing
// to create would leave the customer with no usable VPN and no undo — strictly
// worse than doing nothing. Creating first means the worst case is "the new one
// was refused and nothing else changed", because `Create` already guarantees
// that a panel rejection records no VPN row at all (D-024.1a).
//
// Two distinct panel clients exist when this returns, which is what the
// customer asked for: a replace gives them a *different* VPN, not the same one
// renamed.
func (s *Service) Replace(ctx context.Context, in ReplaceInput, release ReleaseFunc) (*ReplaceResult, error) {
	if in.Actor.UserID <= 0 {
		return nil, ErrNotPermitted
	}
	if release == nil {
		return nil, fmt.Errorf("provisioning: replace needs a release function")
	}

	// --- 1. The old VPN, its owner, and its current state. ------------------
	var (
		ownerID    int64
		serverID   int64
		offerID    int64
		panelEmail string
		status     string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(owner_user_id,0), COALESCE(server_id,0), COALESCE(offer_id,0),
		        COALESCE(panel_email,''), status
		   FROM vpn_instances WHERE id = ?`, in.VPNID).
		Scan(&ownerID, &serverID, &offerID, &panelEmail, &status)
	if errors.Is(err, sqlErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("provisioning: replace: read vpn: %w", err)
	}

	// Ownership. A Super Admin may act on any VPN (P-2) but must say why — the
	// reason is the only record of intent the customer will ever see.
	forced := in.Actor.IsSuperAdmin()
	if ownerID != in.Actor.UserID && !forced {
		// Same answer as "missing", so ids stay unprobeable.
		return nil, ErrNotFound
	}
	if forced && ownerID != in.Actor.UserID && strings.TrimSpace(in.Reason) == "" {
		return nil, fmt.Errorf("%w: a reason is required to replace another account's VPN", ErrNotPermitted)
	}

	// A released (or already-removed) VPN cannot be replaced: it is not in use,
	// so there is nothing to refresh. A customer who released by mistake wants
	// `restore`, not a second VPN.
	switch domain.VPNStatus(status) {
	case domain.VPNReleased, domain.VPNRevoked, domain.VPNRemovedFromCatalog:
		return nil, ErrVPNNotReplaceable
	}

	// --- 2. The replacement target. ----------------------------------------
	target := in.OfferID
	if target <= 0 {
		target = offerID
	}
	if target <= 0 {
		return nil, ErrOfferIncomplete
	}
	newOffer, err := s.loadOffer(ctx, target)
	if err != nil {
		return nil, err
	}
	if newOffer.ServerID != 0 && serverID != 0 && newOffer.ServerID != serverID {
		return nil, ErrReplaceTargetMismatch
	}
	// The kind check needs the OLD offer, which may itself be archived. A
	// refresh onto a live target must not be blocked by the old row leaving the
	// catalogue, so the check is skipped when it cannot be read.
	if oldOffer, oerr := s.loadOffer(ctx, offerID); oerr == nil && oldOffer != nil {
		if newOffer.Kind != "" && oldOffer.Kind != "" && newOffer.Kind != oldOffer.Kind {
			return nil, ErrReplaceKindMismatch
		}
	}

	// --- 3. The claim allowance, checked BEFORE it is spent (D-025.5). ------
	// Bible §17.18: releasing a VPN does not refund a monthly claim, so a
	// replace consumes one. Checked here so the refusal can name the remaining
	// count instead of failing deep inside the create with a bare cap error.
	claimsConsumed := 0
	claimsRemaining := 0
	if newOffer.Kind == "FREE" {
		if usage, uerr := s.st.FreeUsage(ctx, ownerID, newOffer.ServerID, in.FreeMonthlyLimit); uerr == nil && usage != nil {
			claimsRemaining = usage.MonthlyLimit - usage.UsedThisWindow
			if claimsRemaining <= 0 {
				return nil, fmt.Errorf("%w — %d of %d used in the last 30 days",
					ErrNoClaimRemaining, usage.UsedThisWindow, usage.MonthlyLimit)
			}
		}
	}

	// --- 4. Build the NEW VPN first. ---------------------------------------
	// A panel rejection rolls the provisional rows back inside `Create`, so a
	// failure here leaves the old VPN untouched. Nothing below runs unless the
	// panel positively confirmed a client.
	created, err := s.Create(ctx, CreateInput{
		OfferID:          target,
		ClientUserID:     ownerID,
		Actor:            in.Actor,
		FreeMonthlyLimit: in.FreeMonthlyLimit,
		Approved:         forced,
		// The FREE claim is swapped, not added (D-112): the old VPN is released
		// only after this create succeeds, so at the concurrent cap an ordinary
		// claim would always be refused.
		ReplacesPanelEmail: panelEmail,
	})
	if err != nil {
		return nil, err
	}
	if created == nil || !created.Recorded || !created.Confirmed {
		// A create that produced no confirmed client is not a foundation for
		// releasing anything. Say so instead of half-doing the replace.
		return nil, ErrPanelFailed
	}
	if newOffer.Kind == "FREE" {
		claimsConsumed = 1
	}

	// --- 5. Only now release the old VPN. ----------------------------------
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = fmt.Sprintf("replaced by VPN %d (%s)", created.VPNID, created.Email)
	}
	row, err := release(ctx, in.VPNID, in.Actor, reason)
	if err != nil {
		// The new VPN exists and the old one was not released. That is a partial
		// outcome and is reported as one: deleting the confirmed panel client to
		// "tidy up" would be worse than the mismatch.
		return nil, fmt.Errorf("%w: the new VPN %d (%s) was created, but releasing VPN %d failed: %v",
			ErrPanelFailed, created.VPNID, created.Email, in.VPNID, err)
	}

	out := &ReplaceResult{
		New:           created,
		OldVPNID:      in.VPNID,
		OldPanelEmail: panelEmail,
		ClaimConsumed: claimsConsumed == 1,
	}
	if row != nil {
		out.ReleaseID = row.ID
		out.RestoreDeadline = row.RestoreDeadline
	}
	if claimsRemaining > 0 {
		out.ClaimsRemaining = claimsRemaining - claimsConsumed
	}
	return out, nil
}
