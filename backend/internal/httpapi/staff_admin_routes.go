package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// StaffAdminStore backs the staff console (D-044).
type StaffAdminStore interface {
	ListStaff(ctx context.Context, f store.StaffFilter) ([]store.StaffRow, int, error)
	StaffDetailFor(ctx context.Context, staffID int64) (*store.StaffDetail, error)
}

// staffService is the privileged account surface G writes through. It is declared
// here so this file never imports the concrete service.
type staffService interface {
	CreateUserByRole(ctx context.Context, actor auth.Principal, in auth.CreateUserInput) (*auth.UserRow, error)
	SetUserStatus(ctx context.Context, actor auth.Principal, userID int64, status string) (*auth.UserRow, error)
	SetQuotaLimit(ctx context.Context, actor auth.Principal, userID int64, limit int) (*auth.UserRow, error)
	ResetStaffPassword(ctx context.Context, actor auth.Principal, userID int64, newPassword string) error
	SetUserRole(ctx context.Context, actor auth.Principal, userID int64, role auth.Role) (*auth.UserRow, error)
}

// StaffAdminRoutes registers the staff (admin) management console (D-044):
//
//	GET   /api/super/staff?q&role&status&from&to&quota_min&quota_max&server_id&sort&page
//	GET   /api/super/staff/{id}
//	POST  /api/super/staff          { email, display_name, role, quota_limit, username }
//	PATCH /api/super/staff/{id}     { status } | { quota_limit } | { role }
//	POST  /api/super/staff/{id}/reset-password
//
// All of it requires PermAdminManage (super admin only). Creating an account
// GENERATES its password and returns it once: an operator typing a password into a
// form would produce a weak or reused one, and it would end up in a shell history.
func StaffAdminRoutes(mux *http.ServeMux, st StaffAdminStore, svc staffService, aud *audit.Logger,
	authed func(http.Handler) http.Handler) {
	manage := func(h http.Handler) http.Handler {
		return authed(RequirePermission(auth.PermAdminManage)(h))
	}
	mux.Handle("GET /api/super/staff", manage(staffListHandler(st)))
	mux.Handle("GET /api/super/staff/{id}", manage(staffDetailHandler(st)))
	mux.Handle("POST /api/super/staff", manage(staffCreateHandler(svc, aud)))
	mux.Handle("PATCH /api/super/staff/{id}", manage(staffPatchHandler(svc, aud)))
	mux.Handle("POST /api/super/staff/{id}/reset-password", manage(staffResetHandler(svc, aud)))
}

// staffAudit records one console write.
func staffAudit(ctx context.Context, aud *audit.Logger, p auth.Principal, action string, id int64, extra map[string]any) {
	if aud == nil {
		return
	}
	_ = aud.Record(ctx, audit.Entry{
		ActorID: p.UserID, Action: action, TargetType: "user",
		TargetID: strconv.FormatInt(id, 10), Result: "OK", Context: extra,
	})
}

// staffListHandler serves the filtered, paged staff list.
func staffListHandler(st StaffAdminStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f := store.StaffFilter{
			Q:      strings.TrimSpace(q.Get("q")),
			Role:   strings.ToLower(strings.TrimSpace(q.Get("role"))),
			Status: strings.ToUpper(strings.TrimSpace(q.Get("status"))),
			Sort:   strings.TrimSpace(q.Get("sort")),
		}
		switch f.Role {
		case "", "admin", "super_admin":
		default:
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "role must be admin or super_admin")
			return
		}
		if v := q.Get("from"); v != "" {
			f.JoinedFrom = parseUnix(v)
		}
		if v := q.Get("to"); v != "" {
			f.JoinedTo = parseUnix(v)
		}
		f.QuotaMin = parseIntDefault(q.Get("quota_min"), 0)
		f.QuotaMax = parseIntDefault(q.Get("quota_max"), 0)
		f.ServerID, _ = strconv.ParseInt(q.Get("server_id"), 10, 64)

		page := parseIntDefault(q.Get("page"), 1)
		if page < 1 {
			page = 1
		}
		f.Limit = parseIntDefault(q.Get("limit"), 50)
		f.Offset = (page - 1) * f.Limit

		rows, total, err := st.ListStaff(r.Context(), f)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list staff")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"staff": rows, "total": total, "page": page, "limit": f.Limit,
		})
	})
}

// staffDetailHandler serves one staff account's full record.
func staffDetailHandler(st StaffAdminStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := clientPathID(w, r)
		if !ok {
			return
		}
		d, err := st.StaffDetailFor(r.Context(), id)
		if err != nil {
			WriteError(w, http.StatusNotFound, "not_found", "staff account not found")
			return
		}
		WriteJSON(w, http.StatusOK, d)
	})
}

// staffCreateHandler creates an admin (or another super admin) and returns the
// GENERATED password exactly once.
//
// Returning it in the response (and nowhere else) is deliberate: the operator
// hands it to the person, who changes it at first sign-in. Storing or logging it
// would turn a one-time handover into a permanent credential.
func staffCreateHandler(svc staffService, aud *audit.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		var in struct {
			Email       string `json:"email"`
			DisplayName string `json:"display_name"`
			Role        string `json:"role"`
			QuotaLimit  int    `json:"quota_limit"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		role := strings.ToLower(strings.TrimSpace(in.Role))
		if role == "" {
			role = string(auth.RoleAdmin)
		}
		if role != string(auth.RoleAdmin) && role != string(auth.RoleSuperAdmin) {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
				"role must be admin or super_admin (clients self-register)")
			return
		}
		// One generated password, already policy-valid by construction.
		password, err := auth.GeneratePassword(auth.PatternReadable)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not generate a password")
			return
		}
		user, err := svc.CreateUserByRole(r.Context(), p, auth.CreateUserInput{
			Role:        auth.Role(role),
			Email:       strings.TrimSpace(in.Email),
			Password:    password,
			DisplayName: strings.TrimSpace(in.DisplayName),
			QuotaLimit:  in.QuotaLimit,
		})
		if err != nil {
			WriteAuthError(w, err)
			return
		}
		staffAudit(r.Context(), aud, p, "staff.create", user.ID,
			map[string]any{"role": role, "generated_password": true})
		WriteJSON(w, http.StatusCreated, map[string]any{
			"account": user,
			// Shown once; the operator must pass it on and the person must change it.
			"password": password,
			"note":     "Share this once. It cannot be read again; use reset-password if it is lost.",
		})
	})
}

// staffPatchHandler changes status, quota or role.
func staffPatchHandler(svc staffService, aud *audit.Logger) http.Handler {
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
			Status     *string `json:"status"`
			QuotaLimit *int    `json:"quota_limit"`
			Role       *string `json:"role"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if in.Status == nil && in.QuotaLimit == nil && in.Role == nil {
			WriteError(w, http.StatusUnprocessableEntity, "nothing_to_change",
				"send status, quota_limit and/or role")
			return
		}
		// Every branch records what it changed, so the trail says WHICH decision
		// was taken, not merely that the account was touched.
		if in.Status != nil {
			status := strings.ToUpper(strings.TrimSpace(*in.Status))
			if status != "ACTIVE" && status != "SUSPENDED" {
				WriteError(w, http.StatusUnprocessableEntity, "invalid_status", "status must be ACTIVE or SUSPENDED")
				return
			}
			if _, err := svc.SetUserStatus(r.Context(), p, id, status); err != nil {
				WriteAuthError(w, err)
				return
			}
			staffAudit(r.Context(), aud, p, "staff.status_change", id,
				map[string]any{"status": status, "sessions_revoked": status != "ACTIVE"})
		}
		if in.QuotaLimit != nil {
			if *in.QuotaLimit < 0 || *in.QuotaLimit > 10000 {
				WriteError(w, http.StatusUnprocessableEntity, "invalid_quota", "quota must be between 0 and 10000")
				return
			}
			if _, err := svc.SetQuotaLimit(r.Context(), p, id, *in.QuotaLimit); err != nil {
				WriteAuthError(w, err)
				return
			}
			staffAudit(r.Context(), aud, p, "staff.quota_change", id, map[string]any{"limit": *in.QuotaLimit})
		}
		if in.Role != nil {
			role := auth.Role(strings.ToLower(strings.TrimSpace(*in.Role)))
			if _, err := svc.SetUserRole(r.Context(), p, id, role); err != nil {
				WriteAuthError(w, err)
				return
			}
			staffAudit(r.Context(), aud, p, "staff.role_change", id, map[string]any{"role": string(role)})
		}
		WriteJSON(w, http.StatusOK, map[string]any{"id": id, "ok": true})
	})
}

// staffResetHandler sets a NEW generated password for another staff account.
func staffResetHandler(svc staffService, aud *audit.Logger) http.Handler {
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
		if err := svc.ResetStaffPassword(r.Context(), p, id, password); err != nil {
			WriteAuthError(w, err)
			return
		}
		staffAudit(r.Context(), aud, p, "staff.password_reset", id, nil)
		WriteJSON(w, http.StatusOK, map[string]any{
			"password": password,
			"note":     "Share this once. Every session for that account was ended.",
		})
	})
}
