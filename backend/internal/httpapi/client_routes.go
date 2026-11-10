package httpapi

import (
	"context"
	"net/http"

	"veylix/backend/internal/store"
)

// ClientStore lists the client accounts a caller may act on.
type ClientStore interface {
	ClientsVisible(ctx context.Context, callerID int64, callerRole string) ([]store.ClientRow, error)
}

// ClientRoutes registers GET /api/clients.
//
// Before this endpoint the reseller screens had no way to name a real customer,
// so `SaleFlowPage` offered a hard-coded demo list and a premium assignment
// could not be aimed at anybody. The scoping lives in the query, not in the UI
// (REQUIREMENTS §2.2, §2.6 — a reseller sees only their own clients).
func ClientRoutes(mux *http.ServeMux, st ClientStore, authed func(http.Handler) http.Handler) {
	mux.Handle("GET /api/clients", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		// A client has no business enumerating other customers.
		if p.IsClient() {
			WriteError(w, http.StatusForbidden, "forbidden", "not available for this role")
			return
		}
		rows, err := st.ClientsVisible(r.Context(), p.UserID, string(p.Role))
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not list clients")
			return
		}
		WriteJSON(w, http.StatusOK, rows)
	})))
}

// compile-time guard: the store satisfies the interface.
var _ ClientStore = (*store.Store)(nil)
