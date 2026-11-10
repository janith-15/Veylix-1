package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// AuditStore reads the append-only audit trail.
type AuditStore interface {
	ListAudit(ctx context.Context, q store.AuditQuery) (store.AuditPage, error)
}

// QuotaStore computes an admin's quota usage.
type QuotaStore interface {
	QuotaForAdmin(ctx context.Context, adminID int64) (store.QuotaUsage, error)
}

// AccountRoutes registers audit and quota endpoints.
func AccountRoutes(mux *http.ServeMux, aud AuditStore, quota QuotaStore, authed func(http.Handler) http.Handler) {
	// Global audit is Super-Admin only (REQUIREMENTS §4).
	//
	// The read is FILTERED SERVER-SIDE (D-082): kind, actor, a time window and
	// a substring query, plus a limit/offset window over the filtered count.
	// The old `?limit=`-only read let the UI show "nothing recorded" while the
	// matching entry sat one page deeper — a filter that can hide its own
	// answer is not a filter.
	mux.Handle("GET /api/audit", authed(RequirePermission(auth.PermAuditView)(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			query := store.AuditQuery{
				Kind: r.URL.Query().Get("kind"),
				Q:    r.URL.Query().Get("q"),
			}
			if v := r.URL.Query().Get("limit"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					query.Limit = n
				}
			}
			if v := r.URL.Query().Get("offset"); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					query.Offset = n
				}
			}
			if v := r.URL.Query().Get("actor_id"); v != "" {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					query.ActorID = n
				}
			}
			if v := r.URL.Query().Get("from"); v != "" {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					query.From = n
				}
			}
			if v := r.URL.Query().Get("to"); v != "" {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					query.To = n
				}
			}
			// An unrecognised kind is REFUSED, not silently emptied: the caller
			// must learn the token was wrong rather than read an empty page as
			// "nothing happened".
			if !store.ValidAuditKind(query.Kind) {
				WriteError(w, http.StatusBadRequest, "bad_filter", "unknown kind - use one of: auth, server, admin, vpn, payment, client, other")
				return
			}
			page, err := aud.ListAudit(r.Context(), query)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the audit log")
				return
			}
			WriteJSON(w, http.StatusOK, page)
		}))))

	// An admin reads only their own quota; a super admin may pass ?admin_id=.
	//
	// A client holds no reseller quota — consumption is a seller-side concept
	// (REQUIREMENTS §2.2, D-021). Rather than a 403 that would break a shared
	// "read my quota" call site in the UI, the client gets the same documented
	// shape (docs/API_CONTRACT.md) with a zero usage: `{used:0, limit:0,
	// remaining:-1}` where -1 means "no reseller quota". That is not a data
	// leak — it is the caller's own row and it is always the same constant.
	mux.Handle("GET /api/quotas/me", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		if p.IsClient() {
			WriteJSON(w, http.StatusOK, store.QuotaUsage{Used: 0, Limit: 0, Remaining: -1})
			return
		}
		target := p.UserID
		if p.IsSuperAdmin() {
			if v := r.URL.Query().Get("admin_id"); v != "" {
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					target = n
				}
			}
		}
		usage, err := quota.QuotaForAdmin(r.Context(), target)
		if err != nil {
			WriteError(w, http.StatusNotFound, "not_found", "no quota for that account")
			return
		}
		WriteJSON(w, http.StatusOK, usage)
	})))
}
