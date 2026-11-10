package httpapi

import (
	"context"
	"net/http"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/store"
)

// OverviewStore is the read-only aggregation surface behind the Super Admin
// overview (D-107). Declared here so this file never imports the concrete store.
//
// Both methods are pure reads: the overview cannot change platform state, which is
// why it needs no audit entry and no transaction.
type OverviewStore interface {
	PaymentTotals(ctx context.Context, monthStart int64) (store.PaymentTotals, error)
	ResellerHealth(ctx context.Context, monthStart, nowUnix int64) (store.ResellerHealth, error)
}

// OverviewPlatform describes the platform counts the dashboard assembles from the
// reads it already makes. It is a TYPE for the client's shape, not a server
// payload — see the note in OverviewRoutes about why this route does not compute it.
type OverviewPlatform struct {
	ServersTotal  int `json:"servers_total"`
	ServersActive int `json:"servers_active"`
	ServersStale  int `json:"servers_stale"`
	Clients       int `json:"clients"`
	VPNsActive    int `json:"vpns_active"`
	VPNsTotal     int `json:"vpns_total"`
}

// OverviewRoutes registers the Super Admin overview summary.
//
//	GET /api/super/overview/summary
//
// Three sections in ONE response, because a dashboard that resolves in three
// staggered flashes reads as slower than the same data arriving once (§3 of the
// plan). Every section is optional in the JSON: when a read fails the section is
// `null` and `errors` names it, so the page can render the parts that worked
// instead of blanking.
func OverviewRoutes(mux *http.ServeMux, ov OverviewStore, authed func(http.Handler) http.Handler) {
	mux.Handle("GET /api/super/overview/summary", authed(
		RequirePermission(auth.PermAdminManage)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := PrincipalFrom(r.Context()); !ok {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
				return
			}
			if ov == nil {
				WriteError(w, http.StatusServiceUnavailable, "not_configured",
					"the overview store is not configured on this server")
				return
			}

			now := time.Now()
			monthStart := store.MonthStartUnix(now)

			/*
			 * A failed section is reported, not swallowed. The alternative — one
			 * 500 for the whole page — throws away two sections that did resolve,
			 * and an owner staring at an error learns nothing. The page shows the
			 * sections that worked and says which ones did not.
			 */
			failures := []string{}

			money, err := ov.PaymentTotals(r.Context(), monthStart)
			if err != nil {
				failures = append(failures, "money")
			}

			resellers, err := ov.ResellerHealth(r.Context(), monthStart, now.Unix())
			if err != nil {
				failures = append(failures, "resellers")
			}

			/*
			 * Platform counts are NOT collected here. They are assembled on the
			 * client from the reads it already makes (`/api/servers`, `/api/clients`,
			 * `/api/super/vpns?limit=1`), because duplicating three interfaces in
			 * this file to re-answer questions three other routes already answer
			 * would add a second source of truth for the same number — and the two
			 * would drift.
			 *
			 * `platform` is therefore absent from the response rather than zeroed:
			 * a section that says 0/0/0 when it simply did not look would be a claim
			 * the data does not support.
			 */
			WriteJSON(w, http.StatusOK, map[string]any{
				"money":        money,
				"resellers":    resellers,
				"errors":       failures,
				"generated_at": now.Unix(),
			})
		}))))
}
