package httpapi

import (
	"context"
	"errors"
	"net/http"

	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/store"
)

// FreeClaimLimiter reads the per-client FREE monthly claim cap (D-025.5).
//
// Declared here rather than reached through a concrete store type so the route
// depends on the capability, not the persistence layer — and so `freeClaims`
// may legitimately be nil in a harness that does not exercise the cap.
type FreeClaimLimiter interface {
	MonthlyLimit(ctx context.Context, clientUserID int64) (int, error)
}

// VPNReplacer is the D-112 composite action: build a new VPN, then release the
// old one. Kept separate from VPNService so a deployment that has provisioning
// but no release lifecycle simply does not register the route.
type VPNReplacer interface {
	Replace(ctx context.Context, in provisioning.ReplaceInput, release provisioning.ReleaseFunc) (*provisioning.ReplaceResult, error)
}

// ReplaceRoutes registers POST /api/vpns/{id}/replace (D-112).
//
// Why this is one endpoint and not "release, then claim": two calls can
// half-succeed, and the half that matters most — the customer left with no
// usable VPN and no undo — is unrecoverable. The service fixes the order (create
// first, confirmed by a panel read; release second), and this route only decides
// who may ask.
func ReplaceRoutes(mux *http.ServeMux, svc VPNReplacer, rel ReleaseService,
	freeClaims FreeClaimLimiter, authed func(http.Handler) http.Handler) {
	if svc == nil || rel == nil {
		return
	}
	mux.Handle("POST /api/vpns/{id}/replace", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
			return
		}
		// The same door as release: the VPN's owner, or the platform owner.
		// Ownership is re-checked inside `Replace`, so a reseller cannot reach
		// another account's VPN by guessing an id.
		if !p.IsClient() && !p.IsSuperAdmin() {
			WriteError(w, http.StatusForbidden, "forbidden",
				"only the VPN's owner or the platform owner can replace it")
			return
		}

		var in struct {
			OfferID int64  `json:"offer_id"`
			Reason  string `json:"reason"`
			// Confirm must be explicitly true. A replace spends a monthly claim
			// and releases a working VPN — an action with two side effects is
			// never taken on a request that could be a stray retry.
			Confirm bool `json:"confirm"`
		}
		if r.ContentLength > 0 {
			if err := DecodeJSON(r, &in); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_body", "could not read the request body")
				return
			}
		}
		if !in.Confirm {
			WriteError(w, http.StatusUnprocessableEntity, "confirmation_required",
				"replacing a VPN creates a new one and releases the current one — confirm the action to continue")
			return
		}

		// The FREE claim cap is a per-client setting; read it here so the rule has
		// one entry point, and fall back to the policy default when the lookup is
		// unavailable rather than blocking the action on a tuning value.
		freeLimit := 0
		if freeClaims != nil {
			if n, ferr := freeClaims.MonthlyLimit(r.Context(), p.UserID); ferr == nil {
				freeLimit = n
			}
		}

		res, err := svc.Replace(r.Context(), provisioning.ReplaceInput{
			VPNID:            id,
			OfferID:          in.OfferID,
			Actor:            p,
			Reason:           in.Reason,
			FreeMonthlyLimit: freeLimit,
		}, rel.Release)
		if err != nil {
			writeReplaceError(w, err)
			return
		}
		WriteJSON(w, http.StatusCreated, res)
	})))
}

// writeReplaceError maps a replace failure to a status and a sentence the
// customer can act on. An absence of claims names the count and the window; it
// is never a bare 409 (D-059).
func writeReplaceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, provisioning.ErrNotFound):
		// Same answer for "missing" and "not yours": ids stay unprobeable.
		WriteError(w, http.StatusNotFound, "not_found", "vpn not found")
	case errors.Is(err, provisioning.ErrNoClaimRemaining):
		WriteError(w, http.StatusConflict, "no_claim_remaining",
			"you have used all your FREE claims for the last 30 days on this server — "+
				"a replace needs one. Try again when the window rolls over.")
	case errors.Is(err, provisioning.ErrReplaceKindMismatch):
		WriteError(w, http.StatusUnprocessableEntity, "kind_mismatch",
			"a replacement must be the same kind as the VPN it replaces")
	case errors.Is(err, provisioning.ErrReplaceTargetMismatch):
		WriteError(w, http.StatusUnprocessableEntity, "target_mismatch",
			"a replacement must stay on the same server")
	case errors.Is(err, provisioning.ErrVPNNotReplaceable):
		WriteError(w, http.StatusConflict, "not_replaceable",
			"this VPN is already released or revoked, so there is nothing to replace")
	case errors.Is(err, provisioning.ErrOfferIncomplete), errors.Is(err, provisioning.ErrOfferInactive):
		WriteError(w, http.StatusUnprocessableEntity, "offer_incomplete",
			"the offer this VPN came from can no longer build a VPN — choose a live package")
	case errors.Is(err, provisioning.ErrPlanArchived):
		// The VPN's own package left the catalogue. A replace silently choosing a
		// different package would change what the customer holds, so it is
		// refused and the remedy is named: pick a live package.
		WriteError(w, http.StatusUnprocessableEntity, "plan_archived",
			"the package this VPN came from is no longer sold — pick a current package to replace it with")
	case errors.Is(err, provisioning.ErrPlanExhausted):
		WriteError(w, http.StatusConflict, "plan_exhausted",
			"the package this VPN came from has no slots left — pick a current package")
	case errors.Is(err, provisioning.ErrPlanNotApproved):
		WriteError(w, http.StatusForbidden, "plan_not_approved",
			"this premium package needs a Super Admin confirmation before it can be issued")
	case errors.Is(err, provisioning.ErrNotPermitted):
		WriteError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, store.ErrReleaseExists):
		WriteError(w, http.StatusConflict, "already_released",
			"this VPN is already in its undo window")
	default:
		// Includes the partial outcome (new VPN exists, old one not released).
		// The message is passed through because it names what actually happened;
		// it is built by the service and carries no secret.
		WriteError(w, http.StatusBadGateway, "replace_failed", err.Error())
	}
}

// Compile-time proof that the real service satisfies the replace surface.
//
// The router reaches VPNReplacer through a type assertion on `deps.VPNs`. If a
// future refactor changed `Replace`'s signature, that assertion would fail
// silently — the route would simply never register and every replace would 404
// — which is a defect that no unit test on the service would catch. This
// declaration turns it into a build failure instead.
var _ VPNReplacer = (*provisioning.Service)(nil)

// provisioningReplacer adapts the injected VPN service to VPNReplacer when the
// concrete service supports it.
//
// `deps.VPNs` is a `VPNService` interface, and the replace action is not part of
// it — a harness that stubs creating may legitimately not implement it. The type
// assertion keeps `ReplaceRoutes` from being registered against a service that
// would panic on call, and returns nil so the route is simply absent. A missing
// route answers 404, which is honest; a half-wired one would answer 500 on every
// request.
func provisioningReplacer(svc VPNService) VPNReplacer {
	if r, ok := svc.(VPNReplacer); ok {
		return r
	}
	return nil
}
