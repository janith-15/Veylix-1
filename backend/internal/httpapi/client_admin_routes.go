package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// ClientAdminStore backs the Super Admin's client console (D-042).
type ClientAdminStore interface {
	ListClientsAdmin(ctx context.Context, f store.ClientFilter) ([]store.ClientAdminRow, int, error)
	ClientDetail(ctx context.Context, clientID int64, freeDefault int) (*store.ClientDetail, error)
	ClientSetStatus(ctx context.Context, clientID int64, status string) error
	ClientSetFreeLimit(ctx context.Context, clientID int64, limit int) error
}

// ClientPasswordResetter is the Super Admin's recovery path for a customer.
//
// It exists because a client has no other way back in when the address is wrong
// or the mailbox is gone: password reset is an e-mail, and verification is an
// e-mail. Without this route the operator can only watch the account stay locked.
type ClientPasswordResetter interface {
	ResetStaffPassword(ctx context.Context, actor auth.Principal, userID int64, newPassword string) error
}

// clientPathID reads a positive id from the path.
//
// It is a separate name from the package's `pathID` (which returns a bare error)
// because these handlers answer with the standard error envelope.
func clientPathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "invalid id")
		return 0, false
	}
	return id, true
}

// parseUnix accepts unix seconds or an ISO date (YYYY-MM-DD) from a date input.
func parseUnix(v string) int64 {
	v = strings.TrimSpace(v)
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t.Unix()
	}
	return 0
}

// parseTriBool maps "1"/"true"/"yes" to true and "0"/"false"/"no" to false.
// Anything else (including empty) means "do not filter".
func parseTriBool(v string) *bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		t := true
		return &t
	case "0", "false", "no":
		f := false
		return &f
	}
	return nil
}

func parseIntDefault(v string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return n
	}
	return def
}

// ClientAdminRoutes registers the client-management console (D-042):
//
//	GET   /api/super/clients?q&status&from&to&free_min&free_max&has_premium&has_free&google&sort&page
//	GET   /api/super/clients/{id}
//	PATCH /api/super/clients/{id}   { status } and/or { free_vpn_limit }
//
// Every route requires `PermClientManage`, which only the Super Admin holds: a
// reseller keeps using the scoped `/api/clients` list, so this console can never
// widen into another seller's book of business.
//
// The handlers are named functions mounted through `manage(...)`, rather than
// inline closures, so the route table reads as a list and each handler can be
// tested on its own.
func ClientAdminRoutes(mux *http.ServeMux, st ClientAdminStore, aud *audit.Logger,
	authed func(http.Handler) http.Handler) {
	ClientAdminRoutesWithReset(mux, st, aud, authed, nil)
}

// ClientAdminRoutesWithReset mounts the same console plus the password-reset
// route. Split so the existing two-argument call sites (and tests) keep working,
// and a nil resetter simply means "that one route is not offered".
func ClientAdminRoutesWithReset(mux *http.ServeMux, st ClientAdminStore, aud *audit.Logger,
	authed func(http.Handler) http.Handler, resets ClientPasswordResetter) {
	manage := func(h http.Handler) http.Handler {
		return authed(RequirePermission(auth.PermClientManage)(h))
	}
	mux.Handle("GET /api/super/clients", manage(clientListHandler(st)))
	mux.Handle("GET /api/super/clients/{id}", manage(clientDetailHandler(st)))
	mux.Handle("PATCH /api/super/clients/{id}", manage(clientPatchHandler(st, aud)))
	mux.Handle("DELETE /api/super/clients/{id}", manage(clientDeleteHandler(st, aud)))
	if resets != nil {
		mux.Handle("POST /api/super/clients/{id}/reset-password",
			manage(clientResetPasswordHandler(resets, aud)))
	}
}

// clientResetPasswordHandler issues a fresh password for a customer and ends
// every one of that account's sessions.
//
// The password is generated server-side (never chosen by the operator), returned
// exactly once, and never stored in clear text — the same contract as the staff
// reset, because the risk is the same: whoever holds the reply holds the account.
func clientResetPasswordHandler(resets ClientPasswordResetter, aud *audit.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, ok := clientPathID(w, r)
		if !ok {
			return
		}
		password, err := auth.GeneratePassword(auth.PatternReadable)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not generate a password")
			return
		}
		if err := resets.ResetStaffPassword(r.Context(), p, id, password); err != nil {
			WriteAuthError(w, err)
			return
		}
		auditClientChange(r.Context(), aud, p, "client.password_reset", id, nil)
		WriteJSON(w, http.StatusOK, map[string]any{
			"password": password,
			"note":     "Hand this over once and ask them to change it. Every session for that account was ended.",
		})
	})
}

// clientDeleteHandler marks a client account DELETED.
//
// It is a state change, not a row delete: the database keeps every VPN, payment
// and audit row (all three reference users with ON DELETE RESTRICT), which is what
// accounting and a 3X-UI panel need. The account can no longer sign in, and the
// state is terminal — PATCH cannot bring it back.
//
// 409 while the client still owns VPNs: those live on the panel, and removing the
// account first would leave clients nobody can attribute.
func clientDeleteHandler(st ClientAdminStore, aud *audit.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, ok := clientPathID(w, r)
		if !ok {
			return
		}
		deleter, ok := st.(ClientDeleter)
		if !ok {
			WriteError(w, http.StatusNotImplemented, "not_implemented", "delete is not available")
			return
		}
		switch err := deleter.ClientDelete(r.Context(), id); {
		case err == nil:
		case errors.Is(err, store.ErrClientHasVPNs):
			WriteError(w, http.StatusConflict, "client_has_vpns",
				"this client still owns VPNs — revoke or reassign them first")
			return
		case errors.Is(err, store.ErrClientNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "client not found")
			return
		default:
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not delete the client")
			return
		}
		auditClientChange(r.Context(), aud, p, "client.deleted", id, nil)
		WriteJSON(w, http.StatusOK, map[string]any{
			"deleted": true,
			"note":    "The account can no longer sign in. Its VPNs, payments and audit rows are kept.",
		})
	})
}

// ClientDeleter is the delete capability, kept separate from ClientAdminStore so
// an existing test double does not have to grow a method it never calls.
type ClientDeleter interface {
	ClientDelete(ctx context.Context, clientID int64) error
}

// clientListHandler serves the filtered, paged client list.
func clientListHandler(st ClientAdminStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f := store.ClientFilter{
			Q:      strings.TrimSpace(q.Get("q")),
			Status: strings.ToUpper(strings.TrimSpace(q.Get("status"))),
			Sort:   strings.TrimSpace(q.Get("sort")),
		}
		if v := q.Get("from"); v != "" {
			f.JoinedFrom = parseUnix(v)
		}
		if v := q.Get("to"); v != "" {
			f.JoinedTo = parseUnix(v)
		}
		f.FreeMin = parseIntDefault(q.Get("free_min"), 0)
		f.FreeMax = parseIntDefault(q.Get("free_max"), 0)
		f.HasPremium = parseTriBool(q.Get("has_premium"))
		f.HasFree = parseTriBool(q.Get("has_free"))
		f.GoogleLinked = parseTriBool(q.Get("google"))

		page := parseIntDefault(q.Get("page"), 1)
		if page < 1 {
			page = 1
		}
		// B-2: clamp the page size FIRST, then derive the offset from the clamped
		// value. The store performs the same clamp, so the two can never describe
		// different windows (`?limit=9999&page=5` used to ask for LIMIT 50 at
		// OFFSET 39996).
		f.Limit = parseIntDefault(q.Get("limit"), store.ClientAdminDefaultLimit)
		if f.Limit < 1 || f.Limit > store.ClientAdminMaxLimit {
			f.Limit = store.ClientAdminDefaultLimit
		}
		f.Offset = (page - 1) * f.Limit

		rows, total, err := st.ListClientsAdmin(r.Context(), f)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list clients")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"clients": rows, "total": total, "page": page, "limit": f.Limit,
		})
	})
}

// clientDetailHandler serves the full record for one client.
func clientDetailHandler(st ClientAdminStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := clientPathID(w, r)
		if !ok {
			return
		}
		// The platform default lives in one place (store.FreeMonthlyDefault), so the
		// page shows the EFFECTIVE limit and an operator never reconciles two numbers.
		d, err := st.ClientDetail(r.Context(), id, store.FreeMonthlyDefault)
		if err != nil {
			WriteError(w, http.StatusNotFound, "not_found", "client not found")
			return
		}
		WriteJSON(w, http.StatusOK, d)
	})
}

// clientPatchHandler blocks/unblocks a client and/or sets its FREE limit.
func clientPatchHandler(st ClientAdminStore, aud *audit.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, ok := clientPathID(w, r)
		if !ok {
			return
		}
		var in struct {
			Status       *string `json:"status"`
			FreeVPNLimit *int    `json:"free_vpn_limit"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if in.Status == nil && in.FreeVPNLimit == nil {
			WriteError(w, http.StatusUnprocessableEntity, "nothing_to_change",
				"send status and/or free_vpn_limit")
			return
		}

		if in.Status != nil {
			status := strings.ToUpper(strings.TrimSpace(*in.Status))
			// A Super Admin blocks or unblocks; PENDING_VERIFICATION belongs to the
			// registration flow, not to this screen.
			if status != "ACTIVE" && status != "SUSPENDED" {
				WriteError(w, http.StatusUnprocessableEntity, "invalid_status",
					"status must be ACTIVE or SUSPENDED")
				return
			}
			if err := st.ClientSetStatus(r.Context(), id, status); err != nil {
				WriteError(w, http.StatusNotFound, "not_found", "client not found")
				return
			}
			auditClientChange(r.Context(), aud, p, "client.status_change", id,
				map[string]any{"status": status, "sessions_revoked": status != "ACTIVE"})
		}

		if in.FreeVPNLimit != nil {
			if *in.FreeVPNLimit < 0 || *in.FreeVPNLimit > 100 {
				WriteError(w, http.StatusUnprocessableEntity, "invalid_free_limit",
					"the FREE limit must be between 0 (platform default) and 100")
				return
			}
			if err := st.ClientSetFreeLimit(r.Context(), id, *in.FreeVPNLimit); err != nil {
				WriteError(w, http.StatusNotFound, "not_found", "client not found")
				return
			}
			auditClientChange(r.Context(), aud, p, "client.free_limit", id,
				map[string]any{"limit": *in.FreeVPNLimit})
		}

		d, err := st.ClientDetail(r.Context(), id, store.FreeMonthlyDefault)
		if err != nil {
			WriteError(w, http.StatusNotFound, "not_found", "client not found")
			return
		}
		WriteJSON(w, http.StatusOK, d)
	})
}

// auditClientChange records one client-console write. A nil logger tolerates the
// write but leaves no trail, so production always wires it.
func auditClientChange(ctx context.Context, aud *audit.Logger, p auth.Principal,
	action string, clientID int64, extra map[string]any) {
	if aud == nil {
		return
	}
	_ = aud.Record(ctx, audit.Entry{
		ActorID: p.UserID, Action: action, TargetType: "user",
		TargetID: strconv.FormatInt(clientID, 10), Result: "OK", Context: extra,
	})
}
