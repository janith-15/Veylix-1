package httpapi_test

import (
	"net/http"
	"testing"
)

// A client must be able to SEE the premium plans it may buy (D-048), while the
// FREE catalogue stays FREE-only (D-024.3). Both halves are asserted in one test,
// because widening one by accident is exactly what the split exists to prevent:
// TestOffersClientSeesOnlyFree already pins the FREE side, and this pins the pair.
func TestClientSeesPremiumCatalogueButNotInFreeList(t *testing.T) {
	env := newOfferEnv(t)
	c := env.client(t, "buyer@example.com")

	// The FREE list must not leak the premium plan.
	freeRows := getJSONArray(t, c, env.srv.URL+"/api/offers")
	if len(freeRows) != 1 {
		t.Fatalf("FREE list = %d rows, want 1", len(freeRows))
	}
	if freeRows[0]["kind"] != "FREE" {
		t.Fatalf("the FREE list leaked a %v offer", freeRows[0]["kind"])
	}

	// The separate premium catalogue is what makes /buy possible at all.
	status, body := getJSON(t, c, env.srv.URL+"/api/offers/premium")
	if status.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/offers/premium = %d (%v)", status.StatusCode, body)
	}
	prem := getJSONArray(t, c, env.srv.URL+"/api/offers/premium")
	if len(prem) != 1 {
		t.Fatalf("premium catalogue = %d rows, want 1", len(prem))
	}
	if prem[0]["kind"] != "PREMIUM" {
		t.Fatalf("premium catalogue returned %v", prem[0]["kind"])
	}
	if prem[0]["name"] != "Reseller Premium" {
		t.Fatalf("premium catalogue name = %v", prem[0]["name"])
	}
}

// The premium catalogue is still behind authentication.
func TestPremiumCatalogueIsAuthenticated(t *testing.T) {
	env := newOfferEnv(t)
	anon := &http.Client{Jar: mustJar(t)}

	resp, _ := getJSON(t, anon, env.srv.URL+"/api/offers/premium")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous GET /api/offers/premium = %d, want 401", resp.StatusCode)
	}
}
