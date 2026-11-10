package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// stubOverviewStore records whether it was called, so every refusal test can prove
// the store was never consulted â€” a gate that leaks data while refusing is worse
// than one that admits it.
type stubOverviewStore struct {
	moneyCalled     bool
	resellersCalled bool
	monthStart      int64

	moneyErr     error
	resellersErr error

	money     store.PaymentTotals
	resellers store.ResellerHealth
}

func (s *stubOverviewStore) PaymentTotals(_ context.Context, monthStart int64) (store.PaymentTotals, error) {
	s.moneyCalled = true
	s.monthStart = monthStart
	return s.money, s.moneyErr
}

func (s *stubOverviewStore) ResellerHealth(_ context.Context, _, _ int64) (store.ResellerHealth, error) {
	s.resellersCalled = true
	return s.resellers, s.resellersErr
}

// newOverviewMux mounts the route behind a principal of `role`. A nil store proves
// the "not configured" branch.
func newOverviewMux(ov OverviewStore, role auth.Role) http.Handler {
	mux := http.NewServeMux()
	inject := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithPrincipal(r.Context(), auth.Principal{UserID: 7, Email: "super@test", Role: role})
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	OverviewRoutes(mux, ov, inject)
	return mux
}

func doOverview(mux http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/api/super/overview/summary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

/*
TestOverviewRefusesResellersAndClients is the authorization gate.

The overview shows PLATFORM revenue and every reseller's quota state. A reseller
seeing it would learn what the business earns and where every peer stands â€” so the
refusal must be server-side, and must happen before the store is read.
*/
func TestOverviewRefusesResellersAndClients(t *testing.T) {
	cases := []struct {
		name string
		role auth.Role
		want int
	}{
		{"client", auth.RoleClient, http.StatusForbidden},
		{"admin", auth.RoleAdmin, http.StatusForbidden},
		{"super admin", auth.RoleSuperAdmin, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ov := &stubOverviewStore{}
			rec := doOverview(newOverviewMux(ov, c.role))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, c.want, rec.Body.String())
			}
			// The store may only be touched when the caller is allowed in.
			refused := c.want != http.StatusOK
			if refused && (ov.moneyCalled || ov.resellersCalled) {
				t.Fatalf("store was read for a refused caller (%s): money=%v resellers=%v",
					c.name, ov.moneyCalled, ov.resellersCalled)
			}
			if !refused && !ov.moneyCalled {
				t.Fatalf("store was not read for an allowed caller")
			}
		})
	}
}

// TestOverviewAnonymousIs401 proves the route is not reachable without a session.
func TestOverviewAnonymousIs401(t *testing.T) {
	mux := http.NewServeMux()
	// No principal injected: the handler's own check must refuse.
	OverviewRoutes(mux, &stubOverviewStore{}, func(h http.Handler) http.Handler { return h })
	req := httptest.NewRequest("GET", "/api/super/overview/summary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// TestOverviewReturnsBothSections proves the payload carries the money and the
// reseller book, because a dashboard that silently drops a section on a successful
// response is the failure mode this test exists to catch.
func TestOverviewReturnsBothSections(t *testing.T) {
	ov := &stubOverviewStore{
		money: store.PaymentTotals{
			Currency: "USD", Currencies: []string{"USD"},
			MonthCents: 4750000, MonthSales: 12, PendingCount: 4,
		},
		resellers: store.ResellerHealth{Total: 11, QuotaFull: 1},
	}
	rec := doOverview(newOverviewMux(ov, auth.RoleSuperAdmin))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Money     store.PaymentTotals  `json:"money"`
		Resellers store.ResellerHealth `json:"resellers"`
		Errors    []string             `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	if body.Money.MonthCents != 4750000 || body.Money.Currency != "USD" {
		t.Fatalf("money = %+v, want the stubbed totals", body.Money)
	}
	if body.Resellers.Total != 11 {
		t.Fatalf("resellers = %+v, want Total 11", body.Resellers)
	}
	if len(body.Errors) != 0 {
		t.Fatalf("errors = %v, want none", body.Errors)
	}
	// The month boundary is a local-time fact, so it must be a real month start.
	if ov.monthStart <= 0 {
		t.Fatalf("monthStart = %d, want a real unix second", ov.monthStart)
	}
}

/*
TestOverviewNamesAFailedSectionInsteadOfBlanking.

One failing read must not take the page down: an operator staring at a 500 learns
nothing, while a page with a money row and an explicit "resellers could not be read"
is both honest and useful.
*/
func TestOverviewNamesAFailedSectionInsteadOfBlanking(t *testing.T) {
	ov := &stubOverviewStore{
		money:        store.PaymentTotals{Currency: "USD", MonthCents: 100},
		resellersErr: context.DeadlineExceeded,
	}
	rec := doOverview(newOverviewMux(ov, auth.RoleSuperAdmin))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (one section failed, not the request)", rec.Code)
	}
	var body struct {
		Money  *store.PaymentTotals `json:"money"`
		Errors []string             `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Money == nil || body.Money.MonthCents != 100 {
		t.Fatalf("the section that worked was dropped: %+v", body.Money)
	}
	if len(body.Errors) != 1 || body.Errors[0] != "resellers" {
		t.Fatalf("errors = %v, want [resellers]", body.Errors)
	}
}

// TestOverviewWithoutAStoreSays503 rather than answering zeroes, which would read
// as "nothing has ever been sold".
func TestOverviewWithoutAStoreSays503(t *testing.T) {
	mux := http.NewServeMux()
	inject := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithPrincipal(r.Context(), auth.Principal{UserID: 7, Role: auth.RoleSuperAdmin})
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	OverviewRoutes(mux, nil, inject)
	req := httptest.NewRequest("GET", "/api/super/overview/summary", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
