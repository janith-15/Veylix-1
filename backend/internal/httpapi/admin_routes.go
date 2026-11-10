package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
)

// UserAdminService is the account-management surface the handlers need.
//
// Every method is super-admin-only in the service layer as well as behind the
// permission middleware — defence in depth, because a routing mistake must not
// become a privilege escalation.
type UserAdminService interface {
	ListUsers(ctx context.Context, role auth.Role) ([]auth.UserRow, error)
	CreateUserByRole(ctx context.Context, actor auth.Principal, in auth.CreateUserInput) (*auth.UserRow, error)
	SetUserStatus(ctx context.Context, actor auth.Principal, userID int64, status string) (*auth.UserRow, error)
	SetQuotaLimit(ctx context.Context, actor auth.Principal, userID int64, limit int) (*auth.UserRow, error)
	// AdminServers / SetAdminServers give a reseller a server scope. Without a
	// write path a created admin could see zero servers forever (P5).
	AdminServers(ctx context.Context, adminID int64) ([]int64, error)
	SetAdminServers(ctx context.Context, adminID int64, serverIDs []int64) error
}

// AdminRoutes registers /api/admins — the only way an `admin` account is born.
//
// Self-registration is intentionally limited to `client` (auth.Service.Register);
// creating a reseller or another super admin requires an authenticated super
// admin, and every creation is audited.
func AdminRoutes(mux *http.ServeMux, svc UserAdminService, aud *audit.Logger, authed func(http.Handler) http.Handler) {
	manage := func(h http.HandlerFunc) http.Handler {
		return authed(RequirePermission(auth.PermAdminManage)(h))
	}

	// Assignment writes are audited; a nil logger must not panic a request.
	record := func(ctx context.Context, e audit.Entry) {
		if aud == nil {
			return
		}
		_ = aud.Record(ctx, e)
	}

	// List reseller accounts. ?role=admin is the default; pass role=client or
	// role=super_admin to see the others (super admin only).
	mux.Handle("GET /api/admins", manage(func(w http.ResponseWriter, r *http.Request) {
		role := auth.RoleAdmin
		if v := r.URL.Query().Get("role"); v != "" {
			role = auth.Role(v)
			if !role.Valid() {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "unknown role")
				return
			}
		}
		users, err := svc.ListUsers(r.Context(), role)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list accounts")
			return
		}
		WriteJSON(w, http.StatusOK, users)
	}))

	// Create an account with an explicit role.
	mux.Handle("POST /api/admins", manage(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		var in struct {
			Email       string `json:"email"`
			Password    string `json:"password"`
			DisplayName string `json:"display_name"`
			Role        string `json:"role"`
			QuotaLimit  int    `json:"quota_limit"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		role := auth.RoleAdmin
		if in.Role != "" {
			role = auth.Role(in.Role)
		}
		if role == auth.RoleClient {
			// Clients self-register; minting one here would skip the public
			// consent flow for no reason.
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "clients must self-register")
			return
		}
		user, err := svc.CreateUserByRole(r.Context(), p, auth.CreateUserInput{
			Role: role, Email: in.Email, Password: in.Password,
			DisplayName: in.DisplayName, QuotaLimit: in.QuotaLimit,
		})
		if err != nil {
			WriteAuthError(w, err)
			return
		}
		WriteJSON(w, http.StatusCreated, user)
	}))

	// Change status (suspend / reactivate) and/or the quota ceiling.
	mux.Handle("PATCH /api/admins/{id}", manage(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid account id")
			return
		}
		var in struct {
			Status     *string `json:"status"`
			QuotaLimit *int    `json:"quota_limit"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if in.Status == nil && in.QuotaLimit == nil {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "nothing to update")
			return
		}

		var updated *auth.UserRow
		if in.Status != nil {
			updated, err = svc.SetUserStatus(r.Context(), p, id, *in.Status)
			if err != nil {
				WriteAuthError(w, err)
				return
			}
		}
		if in.QuotaLimit != nil {
			updated, err = svc.SetQuotaLimit(r.Context(), p, id, *in.QuotaLimit)
			if err != nil {
				WriteAuthError(w, err)
				return
			}
		}
		WriteJSON(w, http.StatusOK, updated)
	}))

	getServers := func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid account id")
			return
		}
		ids, err := svc.AdminServers(r.Context(), id)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the assignments")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"admin_id": id, "server_ids": ids})
	}

	putServers := func(w http.ResponseWriter, r *http.Request) {
		p, _ := PrincipalFrom(r.Context())
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid account id")
			return
		}
		var in struct {
			ServerIDs []int64 `json:"server_ids"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}

		if err := svc.SetAdminServers(r.Context(), id, in.ServerIDs); err != nil {
			switch {
			case errors.Is(err, auth.ErrUserNotFound):
				WriteError(w, http.StatusNotFound, "not_found", "account not found")
			case errors.Is(err, auth.ErrNotAnAdmin):
				// Servers on a client would be meaningless state; on a super admin
				// it would be misleading, since they already see every server.
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
					"only reseller accounts hold server assignments")
			default:
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "one of the servers does not exist")
			}
			return
		}

		ids, err := svc.AdminServers(r.Context(), id)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "assignments saved but could not be read back")
			return
		}
		record(r.Context(), audit.Entry{
			ActorID: p.UserID, Action: audit.ActionAdminServerAssign,
			TargetType: "admin", TargetID: strconv.FormatInt(id, 10),
			Context: map[string]any{"server_ids": ids, "count": len(ids)},
		})
		WriteJSON(w, http.StatusOK, map[string]any{"admin_id": id, "server_ids": ids})
	}

	// GET /api/admins/{id}/servers - which panels this reseller may operate on.
	mux.Handle("GET /api/admins/{id}/servers", manage(getServers))

	// PUT /api/admins/{id}/servers - replace the whole assignment set.
	// Replace rather than merge keeps the call idempotent: sending the same list
	// twice is a no-op, and an omitted server really is unassigned. That is the
	// only shape a checkbox UI can use safely.
	mux.Handle("PUT /api/admins/{id}/servers", manage(putServers))
}
