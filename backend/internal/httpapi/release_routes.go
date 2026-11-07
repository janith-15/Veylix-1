package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/releases"
	"veylix/backend/internal/store"
)

// ReleaseService is the D-027 lifecycle surface.
type ReleaseService interface {
	Release(ctx context.Context, vpnID int64, p auth.Principal, reason string) (*store.ReleaseRow, error)
	Restore(ctx context.Context, releaseID int64, p auth.Principal) (*store.ReleaseRow, error)
	List(ctx context.Context, p auth.Principal, clientUserID int64) ([]store.ReleaseRow, error)
}

// ReleaseRoutes registers the release/restore endpoints (Milestone 5-B step 9).
//
// "Storage full" means the VPN's TRAFFIC QUOTA is exhausted, not the host's disk
// (D-027). Releasing never deletes anything: it opens a 3-day undo window, after
// which a background job removes the LOCAL row only — the 3X-UI client stays
// (D-027.3).
func ReleaseRoutes(mux *http.ServeMux, svc ReleaseService, authed func(http.Handler) http.Handler) {
	// POST /api/vpns/{id}/release {reason}
	mux.Handle("POST /api/vpns/{id}/release", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid vpn id")
			return
		}
		// Only the owner releases. An operator acting on a client's behalf would
		// destroy property they do not own, so it is refused rather than
		// "helpfully" allowed.
		if !p.IsClient() {
			WriteError(w, http.StatusForbidden, "forbidden",
				"only the VPN's owner can release it")
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		// A body is optional: releasing with no reason uses the default.
		if r.ContentLength > 0 {
			if err := DecodeJSON(r, &in); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
				return
			}
		}
		row, err := svc.Release(r.Context(), id, p, in.Reason)
		if err != nil {
			writeReleaseError(w, err, "could not release the VPN")
			return
		}
		WriteJSON(w, http.StatusCreated, row)
	})))

	// POST /api/vpn-releases/{id}/restore — the undo button
	mux.Handle("POST /api/vpn-releases/{id}/restore", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_id", "invalid release id")
			return
		}
		row, err := svc.Restore(r.Context(), id, p)
		if err != nil {
			writeReleaseError(w, err, "could not restore the VPN")
			return
		}
		WriteJSON(w, http.StatusOK, row)
	})))

	// GET /api/vpn-releases[?client_user_id=]
	mux.Handle("GET /api/vpn-releases", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		target := int64(0)
		if v := r.URL.Query().Get("client_user_id"); v != "" {
			n, convErr := strconv.ParseInt(v, 10, 64)
			if convErr != nil || n <= 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
					"client_user_id must be a positive integer")
				return
			}
			target = n
		}
		rows, err := svc.List(r.Context(), p, target)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read releases")
			return
		}
		WriteJSON(w, http.StatusOK, rows)
	})))
}

// writeReleaseError maps the service's typed errors onto the API envelope.
func writeReleaseError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, releases.ErrNotFound):
		// Also the answer for somebody else's VPN: ids stay unprobeable.
		WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, releases.ErrExists):
		WriteError(w, http.StatusConflict, "already_released", "this VPN is already released")
	case errors.Is(err, releases.ErrState):
		WriteError(w, http.StatusConflict, "not_restorable",
			"the 3-day undo window has closed, or the release was already restored")
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", fallback)
	}
}
