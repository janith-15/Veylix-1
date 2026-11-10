package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/domain"
	"veylix/backend/internal/provisioning"
	"veylix/backend/internal/store"
)

// VPNService is the provisioning surface the handlers need.
type VPNService interface {
	Create(ctx context.Context, in provisioning.CreateInput) (*provisioning.Result, error)
	Suspend(ctx context.Context, id int64, p auth.Principal) error
	Resume(ctx context.Context, id int64, p auth.Principal) error
	Revoke(ctx context.Context, id int64, p auth.Principal) (domain.VPNStatus, error)
	// VerifyPanel asks the panel whether the client still exists (D-024.5).
	VerifyPanel(ctx context.Context, id int64, p auth.Principal) (*provisioning.VerifyResult, error)
	// RemoveAbsent erases Veylix's record of a VPN the panel no longer holds.
	// The panel client itself is never deleted (D-027.3).
	RemoveAbsent(ctx context.Context, id int64, p auth.Principal) error
	// ForcePurgeRecord retires a local VPN record while deliberately KEEPING the
	// panel client (P-3, D-027.3). Super Admin only; the panel email must be
	// repeated back so a mis-clicked row cannot destroy the wrong record.
	ForcePurgeRecord(ctx context.Context, id int64, p auth.Principal, confirmPanelEmail string) error
	// Dormant and Wake are the customer's "not now" switch (D-094). Neither
	// touches the panel and neither deletes anything: Dormant only records when
	// the VPN was put to sleep, and Wake clears it.
	Dormant(ctx context.Context, id int64, p auth.Principal) error
	Wake(ctx context.Context, id int64, p auth.Principal) error
}

// VPNRow is the safe projection of a VPN for the API.
//
// Tag and Source exist because a VPN is no longer only a local row (I-1): it is
// a 3X-UI client. Source says where this row came from — "panel" for a client
// the panel holds (possibly created directly in 3X-UI), "local" for a Veylix
// record whose panel could not be read. A UI must never present a "local" row
// as proof the VPN exists, so the distinction is on the wire.
type VPNRow struct {
	ID         int64  `json:"id"`
	ServerID   int64  `json:"server_id"`
	PanelEmail string `json:"panel_email"`
	Status     string `json:"status"`
	OwnerID    int64  `json:"owner_user_id"`
	SellerID   int64  `json:"seller_user_id"`
	ExpiresAt  *int64 `json:"expires_at"`
	CreatedAt  int64  `json:"created_at"`
	// DormantAt is when the customer put this VPN to sleep (D-094), or nil when
	// it is awake. It is a LOCAL preference, not a status: `Status` still reports
	// whatever the panel says, so a dormant VPN cannot pretend to be healthy.
	//
	// It is on the wire so the list can sort a sleeping VPN last and leave it out
	// of "Needs attention" without the client guessing, and so the Usage screen
	// can stop polling it.
	DormantAt *int64 `json:"dormant_at"`
	// StatusChangedAt is WHEN the row entered its current status (D-098's
	// `status_changed_at`). Nil when it has never moved, or on a database that
	// predates the column's backfill.
	//
	// It is published because "SUSPENDED" alone leaves the customer asking the
	// one question the answer exists to settle — since when? A screen that can
	// only say "suspended" invites a support call to establish whether it
	// happened today or three months ago.
	//
	// It is a fact about the RECORD, not a promise: it never implies the VPN
	// will be removed at any particular time.
	StatusChangedAt *int64 `json:"status_changed_at"`
	// Tag is the Super Admin's FREE/PREMIUM classification (I-3). Nil means
	// untagged: still a VPN, but visible only to the Super Admin.
	Tag *string `json:"tag"`
	// Source is "panel" or "local" (see the struct comment).
	Source string `json:"source"`

	// ---- who holds it (D-037) -------------------------------------------
	//
	// `OwnerID` alone forced every screen to resolve a second time. These make
	// "whose VPN is this" answerable from the row, and `Assigned` lets the
	// Super Admin's list separate a sold VPN from an unowned panel client.
	OwnerEmail string `json:"owner_email,omitempty"`
	OwnerName  string `json:"owner_name,omitempty"`
	Assigned   bool   `json:"assigned"`
	// OfferKind/OfferName say which package this VPN came from, so a client can
	// separate Free from Premium without guessing (D-032).
	OfferID   int64  `json:"offer_id,omitempty"`
	OfferKind string `json:"offer_kind,omitempty"`
	OfferName string `json:"offer_name,omitempty"`
	// KindSource says WHY OfferKind holds the value it does (D-063):
	// "offer" for a real package, "tag" for the operator's locked FREE/PREMIUM
	// classification, "default" when an owned VPN had neither, and "none" for a
	// panel client nobody holds yet.
	//
	// It is on the wire so a screen can explain the classification instead of
	// implying a package that was never chosen, and so a test can prove an
	// assigned VPN was not silently degraded to "untagged".
	KindSource string `json:"kind_source,omitempty"`

	// ServerName is the operator's own label for the server this VPN lives on
	// (`server_integrations.name`, e.g. "indian-1"). It travels with the row
	// because a bare `server_id` number is not something a customer can
	// recognise on a VPN card — the label is what the operator typed when the
	// server was added, and nothing in Veylix invents it.
	ServerName string `json:"server_name,omitempty"`

	// ---- who sold it (Bible §2.3) ---------------------------------------
	//
	// A client must be able to see which seller is responsible for a Premium VPN
	// and reach them. Before these fields the only seller fact on the wire was
	// the numeric `seller_user_id`, so a screen could do no better than print
	// "Sold by user #3" — which names nobody and gives no way to make contact.
	//
	// The values are the seller's OWN configured business contacts (migration
	// 0013), the same ones the payment destination already shows a payer
	// (D-036.2). Nothing new is exposed that the platform did not already
	// publish; the change is that the VPN screen finally reads them too.
	SellerName     string `json:"seller_name,omitempty"`
	SellerEmail    string `json:"seller_email,omitempty"`
	SellerWhatsApp string `json:"seller_whatsapp,omitempty"`
	SellerTelegram string `json:"seller_telegram,omitempty"`
}

// Tag filters for ListVPNs.
const (
	TagFilterAny      = ""
	TagFilterFree     = "free"
	TagFilterPremium  = "premium"
	TagFilterUntagged = "untagged"
	SourcePanel       = "panel"
	SourceLocal       = "local"
	// SourceLocalMissing means the panel answered and positively does NOT hold
	// this client, while Veylix still has a row for it. It is a different fact
	// from SourceLocal ("the panel could not be read"), and a UI must never
	// show it as a working VPN.
	SourceLocalMissing = "local_missing"
)

// VPNLister reads VPNs scoped to the caller's permissions.
//
// tagFilter is one of the TagFilter* values (TagFilterAny lists everything the
// caller may see). The caller validates it before passing it in.
type VPNLister interface {
	ListVPNs(ctx context.Context, p auth.Principal, tagFilter string) ([]VPNRow, error)
}

// VPNTagFilterFromQuery validates the ?tag= value of GET /api/vpns.
//
// Returns ok=false for an unknown value so the handler can answer 422: silently
// ignoring a typo would show every VPN and look like a filter that "found all".
func VPNTagFilterFromQuery(raw string) (filter string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return TagFilterAny, true
	case TagFilterFree:
		return TagFilterFree, true
	case TagFilterPremium:
		return TagFilterPremium, true
	case TagFilterUntagged:
		return TagFilterUntagged, true
	default:
		return "", false
	}
}

// FreeClaimStore exposes the FREE claim ledger (D-025.4/.5).
//
// Cap enforcement itself lives in the create transaction, not here: these two
// calls only read what the client has used and what ceiling applies to them, so
// the UI can show the cap before claiming (REQUIREMENTS §2.0.1).
type FreeClaimStore interface {
	FreeUsage(ctx context.Context, clientUserID, serverID int64, monthlyLimit int) (*store.FreeClaimUsage, error)
	MonthlyLimit(ctx context.Context, clientUserID int64) (int, error)
}

// VPNRoutes registers /api/vpns/*.
//
// Two listers because the two audiences are different: superLister is the
// panel-backed catalogue for the Super Admin (D-024.3, where an untagged panel
// client is still a VPN), while lister is the local, tag-filtered view for
// admins and clients. nil superLister falls back to lister.
//
// freeClaims is optional: nil leaves the monthly cap at the policy default and
// disables GET /api/vpns/claims/me.
//
// Every route re-checks role + ownership server-side; the client's UI state is
// never trusted (REQUIREMENTS §4).
func VPNRoutes(mux *http.ServeMux, svc VPNService, lister, superLister VPNLister,
	freeClaims FreeClaimStore, authed func(http.Handler) http.Handler) {
	if superLister == nil {
		superLister = lister
	}
	mux.Handle("GET /api/vpns", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		// ?tag=free|premium|untagged filters the list (D-024.3). An unknown
		// value is 422, never a silent "no filter": a typo must not look like
		// a filter that matched everything.
		filter, valid := VPNTagFilterFromQuery(r.URL.Query().Get("tag"))
		if !valid {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
				"tag must be free, premium or untagged")
			return
		}
		// The Super Admin's catalogue comes from the panel (untagged clients
		// included); everyone else reads the local, tag-filtered view.
		active := lister
		if p.Role == auth.RoleSuperAdmin {
			active = superLister
		}
		rows, err := active.ListVPNs(r.Context(), p, filter)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list VPNs")
			return
		}
		WriteJSON(w, http.StatusOK, rows)
	})))

	mux.Handle("POST /api/vpns", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		var in struct {
			OfferID      int64 `json:"offer_id"`
			ClientUserID int64 `json:"client_user_id"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}

		// A client may only provision for themselves.
		clientID := in.ClientUserID
		if p.IsClient() || clientID == 0 {
			clientID = p.UserID
		}
		seller := int64(0)
		if p.IsAdmin() {
			seller = p.UserID
		}
		// The per-client FREE claim cap (D-025.5). Read here so the policy has
		// one entry point; a read failure falls back to the default rather than
		// blocking a purchase on a lookup that only tunes a limit.
		freeLimit := 0
		if freeClaims != nil {
			if n, ferr := freeClaims.MonthlyLimit(r.Context(), clientID); ferr == nil {
				freeLimit = n
			}
		}

		res, err := svc.Create(r.Context(), provisioning.CreateInput{
			OfferID: in.OfferID, ClientUserID: clientID, SellerID: seller, Actor: p,
			FreeMonthlyLimit: freeLimit,
		})
		if err != nil {
			writeVPNError(w, err)
			return
		}
		// 202 when the outcome is not yet confirmed by the panel.
		status := http.StatusCreated
		if !res.Confirmed {
			status = http.StatusAccepted
		}
		// D-024.1a: the panel refused the write and the local rows were rolled
		// back. There is no VPN, so answering 2xx would claim one exists.
		if !res.Recorded {
			WriteError(w, http.StatusBadGateway, "panel_rejected",
				"the panel rejected this VPN, so nothing was created")
			return
		}
		WriteJSON(w, status, res)
	})))

	// GET /api/vpns/claims/me?server_id=N — what the caller has used of their
	// FREE allowance (D-025.4/.5).
	//
	// This exists so the caps are SHOWN before a claim: REQUIREMENTS §2.0.1
	// requires the monthly cap to be visible first, because releasing does not
	// refund a slot. Clients and admins read their own usage; a Super Admin may
	// ask for any client via ?client_user_id=.
	mux.Handle("GET /api/vpns/claims/me", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		if freeClaims == nil {
			WriteError(w, http.StatusServiceUnavailable, "not_wired", "FREE claim tracking is not available")
			return
		}
		clientID := p.UserID
		if v := r.URL.Query().Get("client_user_id"); v != "" {
			if !p.IsSuperAdmin() && !p.IsAdmin() {
				WriteError(w, http.StatusForbidden, "forbidden", "only an admin may read another client's claims")
				return
			}
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil || id <= 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid client_user_id")
				return
			}
			clientID = id
		}
		serverID, err := strconv.ParseInt(r.URL.Query().Get("server_id"), 10, 64)
		if err != nil || serverID <= 0 {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
				"server_id is required (the FREE caps are per server)")
			return
		}
		limit, err := freeClaims.MonthlyLimit(r.Context(), clientID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the claim limit")
			return
		}
		usage, err := freeClaims.FreeUsage(r.Context(), clientID, serverID, limit)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read claim usage")
			return
		}
		WriteJSON(w, http.StatusOK, usage)
	})))

	mux.Handle("POST /api/vpns/{id}/suspend", authed(vpnAction(func(w http.ResponseWriter, r *http.Request, p auth.Principal, id int64) error {
		return svc.Suspend(r.Context(), id, p)
	})))

	mux.Handle("POST /api/vpns/{id}/resume", authed(vpnAction(func(w http.ResponseWriter, r *http.Request, p auth.Principal, id int64) error {
		return svc.Resume(r.Context(), id, p)
	})))

	// POST /api/vpns/{id}/dormant — put a VPN to sleep for THIS customer (D-094).
	//
	// It is not a lifecycle change: no panel client is disabled or removed and the
	// status keeps reporting the panel. What changes is the customer's own view —
	// the VPN sorts last, stops counting toward "Needs attention", and its Usage
	// block stops polling — so a dead VPN stops being a standing chore without
	// anyone deleting anything.
	mux.Handle("POST /api/vpns/{id}/dormant", authed(vpnAction(func(w http.ResponseWriter, r *http.Request, p auth.Principal, id int64) error {
		return svc.Dormant(r.Context(), id, p)
	})))

	// POST /api/vpns/{id}/wake — the inverse. It does NOT make the VPN work again;
	// if the panel still cannot see the client, the status stays SERVER_UNAVAILABLE
	// and the customer is told so.
	mux.Handle("POST /api/vpns/{id}/wake", authed(vpnAction(func(w http.ResponseWriter, r *http.Request, p auth.Principal, id int64) error {
		return svc.Wake(r.Context(), id, p)
	})))

	mux.Handle("POST /api/vpns/{id}/revoke", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
			return
		}
		status, err := svc.Revoke(r.Context(), id, p)
		if err != nil {
			writeVPNError(w, err)
			return
		}
		code := http.StatusOK
		if status != domain.VPNRevoked {
			// Not confirmed yet — the caller must not see "done".
			code = http.StatusAccepted
		}
		WriteJSON(w, code, map[string]any{"status": string(status), "confirmed": status == domain.VPNRevoked})
	})))

	// POST /api/vpns/{id}/verify — ask the panel whether this VPN is real.
	//
	// Read-only against the panel. It reconciles Veylix's status with the
	// answer, so an operator can fix a stale "ACTIVE" without waiting for the
	// next sync tick (D-024.5).
	mux.Handle("POST /api/vpns/{id}/verify", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
			return
		}
		res, err := svc.VerifyPanel(r.Context(), id, p)
		if err != nil {
			writeVPNError(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"exists": res.Exists,
			"status": string(res.Status),
		})
	})))

	// DELETE /api/vpns/{id} — erase Veylix's record of a VPN the panel no
	// longer holds. The panel client is NEVER deleted (D-027.3); revoke is the
	// operation that touches the panel.
	//
	// Refused (409) while the panel still holds the client, so a working VPN
	// cannot be dropped by accident.
	mux.Handle("DELETE /api/vpns/{id}", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
			return
		}
		if err := svc.RemoveAbsent(r.Context(), id, p); err != nil {
			writeVPNError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	/*
	 * POST /api/vpns/{id}/purge — SUPER ADMIN ONLY (P-3).
	 *
	 * The problem this solves: a VPN record whose panel client still exists could
	 * not be removed at all. `DELETE` correctly refuses that case, but the refusal
	 * was also the ONLY path, so an operator had no way to retire a record while
	 * keeping its panel client (D-027.3 forbids Veylix from deleting a panel
	 * client, ever).
	 *
	 * Three guards, because this deletes a row a customer can see:
	 *   1. Super Admin only — a reseller cannot purge somebody else's record;
	 *   2. `confirm_panel_email` must repeat the VPN's identity back, so a
	 *      mis-clicked row cannot destroy the wrong record;
	 *   3. the panel client is left alone. A purge retires the BOOKKEEPING; it is
	 *      not a revocation and it never reaches the panel.
	 */
	mux.Handle("POST /api/vpns/{id}/purge", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		if !p.IsSuperAdmin() {
			WriteError(w, http.StatusForbidden, "forbidden", "only the platform owner can purge a VPN record")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
			return
		}
		var in struct {
			ConfirmPanelEmail string `json:"confirm_panel_email"`
		}
		if r.ContentLength > 0 {
			if err := DecodeJSON(r, &in); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
				return
			}
		}
		if err := svc.ForcePurgeRecord(r.Context(), id, p, in.ConfirmPanelEmail); err != nil {
			writeVPNError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))
}

// vpnAction adapts a simple lifecycle mutation to a handler.
func vpnAction(fn func(w http.ResponseWriter, r *http.Request, p auth.Principal, id int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
			return
		}
		if err := fn(w, r, p, id); err != nil {
			writeVPNError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// writeVPNError maps provisioning errors to safe responses.
// isPurgeConfirmError reports whether err wraps a *provisioning.PurgeConfirmError.
//
// A named helper rather than an inline `errors.As(err, new(*T))` in the switch: that
// form looks like it works but the matched value is discarded, so the message could
// never name the string the operator has to type. Found by
// `TestPurgeRouteRefusesResellersAndAnonymous`, which got a 500 instead of a 422.
func isPurgeConfirmError(err error) bool {
	var pc *provisioning.PurgeConfirmError
	return errors.As(err, &pc)
}

func writeVPNError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, provisioning.ErrNotFound):
		// 404 even for a foreign object so ids cannot be probed.
		WriteError(w, http.StatusNotFound, "not_found", "vpn not found")
	case errors.Is(err, provisioning.ErrQuotaExceeded):
		WriteError(w, http.StatusConflict, "quota_exceeded", "reseller quota is exhausted")
	// D-025.4/.5: the two FREE caps. 409 like the reseller quota, because both
	// mean "a limit, not a malformed request", and the two are told apart so
	// the UI can name which cap was hit.
	case errors.Is(err, store.ErrFreeConcurrentLimit):
		WriteError(w, http.StatusConflict, "free_concurrent_limit",
			"you already hold the maximum number of FREE VPNs on this server")
	case errors.Is(err, store.ErrFreeMonthlyLimit):
		WriteError(w, http.StatusConflict, "free_monthly_limit",
			"you have used all your FREE claims for the last 30 days on this server")
	case errors.Is(err, provisioning.ErrOfferNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "offer not found")
	case errors.Is(err, provisioning.ErrOfferInactive):
		WriteError(w, http.StatusConflict, "offer_inactive", "this offer is not available")
	// D-030: ownership decides who may release a plan. Each refusal has its own
	// code so a screen can say exactly why, and none of them is a 500.
	case errors.Is(err, provisioning.ErrPlanNotApproved):
		// The plan is the platform's own inventory; only a Super-Admin payment
		// confirmation may release it (D-030.3).
		WriteError(w, http.StatusForbidden, "plan_not_approved",
			"this premium plan belongs to the platform and can only be issued after a Super Admin confirms an order")
	case errors.Is(err, provisioning.ErrNotPlanOwner):
		WriteError(w, http.StatusForbidden, "not_plan_owner",
			"this premium plan belongs to another reseller")
	case errors.Is(err, provisioning.ErrServerNotAssigned):
		WriteError(w, http.StatusForbidden, "server_not_assigned",
			"that server is not assigned to your account")
	case errors.Is(err, provisioning.ErrPlanExhausted):
		WriteError(w, http.StatusConflict, "plan_exhausted",
			"this plan has no slots left — buy more slots or use another plan")
	case errors.Is(err, provisioning.ErrPlanArchived):
		WriteError(w, http.StatusConflict, "plan_archived",
			"this plan has been archived and can no longer be issued")
	case errors.Is(err, provisioning.ErrOfferIncomplete):
		// The offer has no inbound, so no client could be created on the panel.
		WriteError(w, http.StatusUnprocessableEntity, "offer_incomplete",
			"this offer has no inbound selected, so no VPN can be created from it")
	case errors.Is(err, provisioning.ErrVPNStillOnPanel):
		// The VPN works: erasing the record would hide a live client. A Super Admin
		// can still retire it with POST /vpns/{id}/purge, which keeps the client.
		WriteError(w, http.StatusConflict, "vpn_still_on_panel",
			"the panel still holds this client, so the record cannot be removed — revoke it, or purge the record and keep the client")
	case isPurgeConfirmError(err):
		// P-3: the operator did not repeat the VPN's identity back. 422 because the
		// request is well-formed and the missing piece is a field the UI must ask
		// for, not a permission the caller lacks.
		//
		// NOTE: `errors.As(err, new(*T))` in a `switch` CASE does not work the way it
		// looks — it allocates a fresh pointer, matches, and then the value is
		// discarded, so the branch below could not read `Want`. Hence the helper.
		var pc *provisioning.PurgeConfirmError
		_ = errors.As(err, &pc)
		want := ""
		if pc != nil {
			want = pc.Want
		}
		WriteError(w, http.StatusUnprocessableEntity, "confirm_panel_email_required",
			"type the VPN's panel email ("+want+") to confirm the purge")
	case errors.Is(err, provisioning.ErrPanelFailed):
		WriteError(w, http.StatusBadGateway, "panel_error",
			"the panel could not be reached, so nothing was changed")
	case errors.Is(err, provisioning.ErrNotPermitted):
		WriteError(w, http.StatusForbidden, "forbidden", "you may not perform this action")
	case errors.Is(err, provisioning.ErrPanelFailed):
		WriteError(w, http.StatusBadGateway, "panel_error", "the panel did not confirm the operation")
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", "unexpected server error")
	}
}

// unused guard for strconv used by pathID elsewhere.
var _ = strconv.Atoi
