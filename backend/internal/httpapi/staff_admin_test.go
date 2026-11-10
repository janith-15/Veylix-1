package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// stubStaffStore is the read side the console needs (list + detail).
//
// Every test in this file exercises a REFUSAL, and a refusal must happen before
// the store is consulted — so the stubs record whether they ran at all. A
// refusal that still read the store would be a gate that leaks.
type stubStaffStore struct {
	listCalled   bool
	detailCalled bool
}

func (s *stubStaffStore) ListStaff(context.Context, store.StaffFilter) ([]store.StaffRow, int, error) {
	s.listCalled = true
	return nil, 0, nil
}

func (s *stubStaffStore) StaffDetailFor(context.Context, int64) (*store.StaffDetail, error) {
	s.detailCalled = true
	return nil, auth.ErrNotFound
}

// stubStaffService records every privileged call so a test can prove it never
// ran, and lets a test force an error to check the mapping.
type stubStaffService struct {
	createCalled bool
	statusCalled bool
	quotaCalled  bool
	resetCalled  bool
	roleCalled   bool

	createErr error
	resetErr  error
	roleErr   error

	lastRole   auth.Role
	lastStatus string
	lastQuota  int
}

func (s *stubStaffService) CreateUserByRole(_ context.Context, _ auth.Principal, in auth.CreateUserInput) (*auth.UserRow, error) {
	s.createCalled = true
	s.lastRole = in.Role
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &auth.UserRow{ID: 99}, nil
}

func (s *stubStaffService) SetUserStatus(_ context.Context, _ auth.Principal, id int64, status string) (*auth.UserRow, error) {
	s.statusCalled = true
	s.lastStatus = status
	return &auth.UserRow{ID: id}, nil
}

func (s *stubStaffService) SetQuotaLimit(_ context.Context, _ auth.Principal, id int64, limit int) (*auth.UserRow, error) {
	s.quotaCalled = true
	s.lastQuota = limit
	return &auth.UserRow{ID: id}, nil
}

func (s *stubStaffService) ResetStaffPassword(_ context.Context, _ auth.Principal, _ int64, _ string) error {
	s.resetCalled = true
	return s.resetErr
}

func (s *stubStaffService) SetUserRole(_ context.Context, _ auth.Principal, id int64, role auth.Role) (*auth.UserRow, error) {
	s.roleCalled = true
	s.lastRole = role
	if s.roleErr != nil {
		return nil, s.roleErr
	}
	return &auth.UserRow{ID: id}, nil
}

// staffMux mounts only the staff console with a principal of `role` already in
// the request context — the same shape settingsMux uses, so the ROUTE contract
// (status codes, permission gate, error mapping) is what is under test.
func staffMux(st *stubStaffStore, svc *stubStaffService, role auth.Role) http.Handler {
	mux := http.NewServeMux()
	inject := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithPrincipal(r.Context(), auth.Principal{UserID: 7, Email: "super@test", Role: role})
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	StaffAdminRoutes(mux, st, svc, nil, inject)
	return mux
}

func doStaff(mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// TestStaffRoutesRefuseNonSuperAdmins is the negative-authorization gate for the
// whole console (Skill veylix-security: every changed route needs an
// unauthenticated, a wrong-role and a wrong-owner test).
//
// The reseller is the interesting role: they hold PermVPNAssignPremium and reach
// several adjacent screens, so "the sidebar hides it" is not a control. Every
// route must answer 403 and must never reach the service or the store.
func TestStaffRoutesRefuseNonSuperAdmins(t *testing.T) {
	cases := []struct {
		name   string
		role   auth.Role
		method string
		path   string
		body   string
	}{
		{"list", auth.RoleAdmin, http.MethodGet, "/api/super/staff", ""},
		{"detail", auth.RoleAdmin, http.MethodGet, "/api/super/staff/2", ""},
		{"create", auth.RoleAdmin, http.MethodPost, "/api/super/staff", `{"email":"x@y.test","role":"admin"}`},
		{"patch", auth.RoleAdmin, http.MethodPatch, "/api/super/staff/2", `{"status":"SUSPENDED"}`},
		{"reset", auth.RoleAdmin, http.MethodPost, "/api/super/staff/2/reset-password", ""},
		{"client list", auth.RoleClient, http.MethodGet, "/api/super/staff", ""},
		{"client create", auth.RoleClient, http.MethodPost, "/api/super/staff", `{"email":"x@y.test","role":"super_admin"}`},
		{"client reset", auth.RoleClient, http.MethodPost, "/api/super/staff/2/reset-password", ""},
	}

	for _, tc := range cases {
		t.Run(string(tc.role)+"/"+tc.name, func(t *testing.T) {
			st, svc := &stubStaffStore{}, &stubStaffService{}
			rec := doStaff(staffMux(st, svc, tc.role), tc.method, tc.path, tc.body)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s as %s = %d, want 403", tc.method, tc.path, tc.role, rec.Code)
			}
			if svc.createCalled || svc.statusCalled || svc.quotaCalled || svc.resetCalled || svc.roleCalled {
				t.Fatalf("%s %s as %s was REFUSED but still reached the service", tc.method, tc.path, tc.role)
			}
			if st.listCalled || st.detailCalled {
				t.Fatalf("%s %s as %s was REFUSED but still read the store", tc.method, tc.path, tc.role)
			}
		})
	}
}

// TestStaffCreateRejectsUnknownRole proves the role enum is enforced at the
// transport layer, so the service never receives a role it would have to guess
// about. `client` is the important case: promoting one here would skip the whole
// account model, because a client self-registers into `client`.
//
// An EMPTY role is deliberately not in this list: `staffCreateHandler` defaults
// it to `admin`, which is the documented behaviour of the console (a new reseller
// is the common case) and is asserted separately below.
func TestStaffCreateRejectsUnknownRole(t *testing.T) {
	for _, role := range []string{"client", "root", "SUPERADMIN", "super-admin"} {
		t.Run("role="+role, func(t *testing.T) {
			svc := &stubStaffService{}
			rec := doStaff(staffMux(&stubStaffStore{}, svc, auth.RoleSuperAdmin), http.MethodPost,
				"/api/super/staff", `{"email":"new@staff.test","role":"`+role+`"}`)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("role %q = %d, want 422", role, rec.Code)
			}
			if svc.createCalled {
				t.Fatalf("role %q was rejected at the door but still reached the service", role)
			}
		})
	}
}

// TestStaffCreateDefaultsEmptyRoleToAdmin pins the documented default: a body
// with no role creates a reseller, not a Super Admin. Guessing the LOUDER role
// would be the dangerous direction, so this is worth a test of its own.
func TestStaffCreateDefaultsEmptyRoleToAdmin(t *testing.T) {
	svc := &stubStaffService{}
	rec := doStaff(staffMux(&stubStaffStore{}, svc, auth.RoleSuperAdmin), http.MethodPost,
		"/api/super/staff", `{"email":"new@staff.test"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("empty role = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if svc.lastRole != auth.RoleAdmin {
		t.Fatalf("empty role defaulted to %q, want admin", svc.lastRole)
	}
}

// TestStaffResetPasswordRefusesSelf proves the console cannot reset the caller's
// own password. That path must go through ChangePassword, which asks for the
// current one; otherwise a borrowed session becomes a permanent takeover.
//
// The status is 422, not 403: `WriteAuthError` maps `ErrCannotModifySelf` to
// `cannot_modify_self` (a request the caller may re-issue differently), while 403
// is reserved for a role that may never reach the route at all. Both are refusals;
// the distinction tells the UI whether to say "not allowed" or "do it this way".
//
// The password assertion is the important half: a refused reset must not leak the
// generated text, because whoever holds the reply holds the account.
func TestStaffResetPasswordRefusesSelf(t *testing.T) {
	svc := &stubStaffService{resetErr: auth.ErrCannotModifySelf}
	// The principal id is 7 (see staffMux), so this targets the caller.
	rec := doStaff(staffMux(&stubStaffStore{}, svc, auth.RoleSuperAdmin), http.MethodPost,
		"/api/super/staff/7/reset-password", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("self reset = %d, want 422 (%s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if pw, ok := body["password"]; ok && pw != nil && pw != "" {
		t.Fatalf("a refused reset leaked a password: %v", pw)
	}
}

// TestStaffPatchValidationOrder proves a malformed field is refused WITHOUT
// applying the sibling fields. The console can send status and quota together;
// a partially applied patch would leave the operator believing nothing changed
// while half of it did.
func TestStaffPatchValidationOrder(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"bad status", `{"status":"DELETED"}`, http.StatusUnprocessableEntity},
		{"quota negative", `{"quota_limit":-1}`, http.StatusUnprocessableEntity},
		{"quota too large", `{"quota_limit":10001}`, http.StatusUnprocessableEntity},
		{"nothing to change", `{}`, http.StatusUnprocessableEntity},
		{"bad json", `{not json`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubStaffService{}
			rec := doStaff(staffMux(&stubStaffStore{}, svc, auth.RoleSuperAdmin), http.MethodPatch,
				"/api/super/staff/2", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("%s = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
			}
			if svc.statusCalled || svc.quotaCalled || svc.roleCalled {
				t.Fatalf("%s was refused but still reached the service", tc.name)
			}
		})
	}
}
