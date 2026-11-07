package httpapi

import (
	"context"
	"net/http"

	"veylix/backend/internal/store"
)

// DirectoryStore lists the accounts a caller may address.
type DirectoryStore interface {
	Directory(ctx context.Context, callerID int64, callerRole string) ([]store.DirectoryRow, error)
}

// DirectoryRoutes registers GET /api/directory.
//
// The chat participant picker used to read a hard-coded list of eight invented
// people, so a real client could not be messaged and a fake one could. The
// backend decides the scope (super admin: everyone; admin: own clients +
// super admins; client: super admins + own sellers), never the UI.
func DirectoryRoutes(mux *http.ServeMux, st DirectoryStore, authed func(http.Handler) http.Handler) {
	mux.Handle("GET /api/directory", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		rows, err := st.Directory(r.Context(), p.UserID, string(p.Role))
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read the directory")
			return
		}
		WriteJSON(w, http.StatusOK, rows)
	})))
}
