// Order settlement — D-031: confirming a payment IS the release.
//
// The whole point of this file is one sentence from the owner (2026-10-25):
//
//	"payment eka wetilada kiyala balala super admin premium eka release karayi.
//	 super admin confirm karama premium vpn eka active wenawa, vpn link eka,
//	 qr eka."
//
// Before it, `ConfirmPayment` flipped a status and the customer, having paid,
// still had no VPN (M6-01). OrderService makes the two one action.
package payments

import (
	"context"
	"errors"
	"fmt"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/store"
)

// Settler is the provisioning surface an order settlement needs. It is an
// interface so the confirmation path can be tested without a panel.
type Settler interface {
	// Create provisions a VPN. `Approved` must be set by the caller when this
	// release is authorised by a confirmation (D-030.3).
	Create(ctx context.Context, in provisioning.CreateInput) (*provisioning.Result, error)
	// Links returns the panel's own connection URIs for a VPN. The principal is
	// the one that will show the credential, because reading a link goes through
	// the same ownership check as the detail page.
	Links(ctx context.Context, id int64, p auth.Principal) (*provisioning.ConnectionInfo, error)
}

// ConfirmInput is one operator decision.
type ConfirmInput struct {
	PaymentID int64
	// Actor is the operator confirming: a Super Admin, or the owner of the plan
	// being released (D-030.2).
	Actor Principal
}

// Principal is the minimal shape of the confirming user.
type Principal struct {
	UserID int64
	Role   string
}

// OrderResult is what the confirmation centre renders after one click: the
// payment, the VPN it produced, and everything needed to hand the customer a
// working configuration (D-031.4).
type OrderResult struct {
	Payment *store.PaymentRow    `json:"payment"`
	VPN     *provisioning.Result `json:"vpn"`
	Links   []string             `json:"links"`
	// LinkSource is "panel" when the panel answered, "unavailable" when it could
	// not be read. A screen must never present an outage as "no links exist".
	LinkSource string `json:"link_source"`
}

// Errors the transport maps to status codes.
var (
	// ErrNotConfirmable means the record is not awaiting a decision.
	ErrNotConfirmable = errors.New("payments: this order is not awaiting confirmation")
	// ErrForbidden means the caller may not confirm this particular order.
	ErrForbidden = errors.New("payments: you may not confirm this order")
	// ErrNoPlan means the order names no plan, so no VPN can be built from it.
	ErrNoPlan = errors.New("payments: this order has no plan attached")
	// ErrReleaseTargetRequired means a CUSTOM order cannot be released until the
	// Super Admin has chosen the panel it belongs on (D-036.6).
	ErrReleaseTargetRequired = errors.New("payments: this order needs a server and an inbound before it can be released")
)

// Service settles orders.
type Service struct {
	st      *store.Store
	settler Settler
}

// New builds the service.
func New(st *store.Store, settler Settler) *Service {
	return &Service{st: st, settler: settler}
}

// Confirm is the one action of D-031: money in, VPN out.
//
// Order of operations, and why:
//  1. **read the order** — refuse anything not awaiting a decision, so a double
//     click cannot build two VPNs;
//  2. **decide who may confirm it** — a Super Admin, or the plan's owner for a
//     plan that reseller owns (D-030.2). Checked before any write;
//  3. **provision** through the existing panel-first path, which writes the VPN
//     row, claims the plan slot and confirms with a fresh read;
//  4. **only then** mark the payment CONFIRMED and record what it produced, in
//     one transaction. If the panel failed, the payment stays exactly as it was
//     and the error reaches the operator — nothing is half-done (D-031.3).
//
// The links are read after the fact and their absence never fails the
// confirmation: a VPN whose links cannot be read right now still exists, and the
// UI shows "could not be read" rather than implying no links (D-026.3).
func (s *Service) Confirm(ctx context.Context, in ConfirmInput) (*OrderResult, error) {
	pay, err := s.st.PaymentByID(ctx, in.PaymentID)
	if err != nil {
		return nil, err
	}
	if pay.Status != store.PaymentPending && pay.Status != store.PaymentAwaitingConfirmation {
		return nil, ErrNotConfirmable
	}
	if pay.OfferID == nil || *pay.OfferID <= 0 {
		return nil, ErrNoPlan
	}

	plan, err := s.st.PlanByID(ctx, *pay.OfferID)
	if err != nil {
		return nil, err
	}
	if err := s.authoriseConfirm(pay, plan, in.Actor); err != nil {
		return nil, err
	}

	target := pay.PayerUserID
	if pay.TargetUserID != nil && *pay.TargetUserID > 0 {
		target = *pay.TargetUserID
	}

	// D-030.2 vs D-030.3: a plan owned by a reseller is released by that owner
	// as their own sale; an unowned plan is released on behalf of the
	// confirmation, which is what makes it reachable at all.
	seller := pay.PayerUserID
	if plan.OwnerUserID != nil {
		seller = *plan.OwnerUserID
	} else {
		// Platform inventory: the seller is the platform, recorded as 0 so no
		// reseller's quota is charged for the Super Admin's own sale.
		seller = 0
	}

	create := provisioning.CreateInput{
		OfferID:      plan.ID,
		ClientUserID: target,
		SellerID:     seller,
		Actor:        principalFor(in.Actor),
		Approved:     true,
	}

	// D-035.2 / D-036.6: a CUSTOM order is not the package's limits — it is the
	// customer's spec, on the panel the Super Admin chose. Releasing it with the
	// template's defaults would hand over a VPN nobody ordered.
	if pay.IsCustomSpec() {
		if pay.ReleaseServerID == nil || pay.ReleaseInboundID == nil ||
			*pay.ReleaseServerID <= 0 || *pay.ReleaseInboundID <= 0 {
			return nil, ErrReleaseTargetRequired
		}
		create.Spec = &provisioning.Spec{
			TrafficGB:    pay.SpecGB,
			DurationDays: int(pay.SpecDays),
			Devices:      int(pay.SpecDevices),
			IPs:          int(pay.SpecIPs),
			ServerID:     *pay.ReleaseServerID,
			InboundID:    *pay.ReleaseInboundID,
		}
	}

	result, err := s.settler.Create(ctx, create)
	if err != nil {
		return nil, err
	}

	// A recorded VPN is a confirmed order. `Recorded == false` means the panel
	// rejected the write and the provisional rows were rolled back: the money
	// must NOT be marked received, so the operator can retry or refund.
	if result == nil || !result.Recorded {
		return nil, fmt.Errorf("%w: the panel did not accept the client, so the payment stays pending",
			provisioning.ErrPanelFailed)
	}

	if err := s.st.CompletePayment(ctx, pay.ID, in.Actor.UserID, result.VPNID); err != nil {
		return nil, err
	}

	out := &OrderResult{Payment: pay, VPN: result, Links: []string{}, LinkSource: provisioning.LinkSourceUnavailable}
	if info, linkErr := s.settler.Links(ctx, result.VPNID, principalFor(in.Actor)); linkErr == nil && info != nil {
		out.Links = info.Links
		out.LinkSource = info.Source
	}
	if refreshed, err := s.st.PaymentByID(ctx, pay.ID); err == nil {
		out.Payment = refreshed
	}
	return out, nil
}

// authoriseConfirm encodes who may confirm an order.
//
//   - a Super Admin may confirm anything (they are the platform's bank);
//   - a reseller may confirm only an order for a plan he owns, paid by him
//     (D-030.2) — anything else is somebody else's money.
func (s *Service) authoriseConfirm(pay *store.PaymentRow, plan *store.Plan, actor Principal) error {
	if actor.Role == "super_admin" {
		return nil
	}
	if plan.OwnerUserID != nil && *plan.OwnerUserID == actor.UserID && pay.PayerUserID == actor.UserID {
		return nil
	}
	return ErrForbidden
}

// principalFor adapts the confirming user to the provisioning service's
// principal. The role string is validated before this point, so an unknown role
// falls through to the least privileged value and provisioning refuses it.
func principalFor(p Principal) auth.Principal {
	role := auth.RoleClient
	switch p.Role {
	case string(auth.RoleSuperAdmin):
		role = auth.RoleSuperAdmin
	case string(auth.RoleAdmin):
		role = auth.RoleAdmin
	}
	return auth.Principal{UserID: p.UserID, Role: role}
}
