package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"veylix/backend/internal/store"
)

// NotificationStore is the persistence surface the inbox handlers need.
type NotificationStore interface {
	NotificationsFor(ctx context.Context, userID int64, limit int) ([]store.NotificationRow, error)
	UnreadNotificationCount(ctx context.Context, userID int64) (int, error)
	MarkNotificationRead(ctx context.Context, userID, id int64) error
	MarkAllNotificationsRead(ctx context.Context, userID int64) (int64, error)
}

// NotificationRoutes registers the per-account inbox (P8).
//
// Every route is scoped to the caller's own `user_id` taken from the session,
// never from a query parameter — an inbox is private, so there is no role that
// may read another account's notifications (REQUIREMENTS §2.8).
//
// Before this endpoint the bell and the inbox page rendered three invented
// entries; they now show what the backend actually recorded, and an empty inbox
// says so instead of inventing a "Panel rejected credentials" story.
func NotificationRoutes(mux *http.ServeMux, st NotificationStore, authed func(http.Handler) http.Handler) {
	// GET /api/notifications[?limit=N]
	mux.Handle("GET /api/notifications", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "limit must be a positive integer")
				return
			}
			limit = n
		}
		rows, err := st.NotificationsFor(r.Context(), p.UserID, limit)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read notifications")
			return
		}
		WriteJSON(w, http.StatusOK, rows)
	})))

	// GET /api/notifications/unread
	// The badge number, so the bell does not have to download the inbox.
	mux.Handle("GET /api/notifications/unread", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		n, err := st.UnreadNotificationCount(r.Context(), p.UserID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not count notifications")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]int{"unread": n})
	})))

	// POST /api/notifications/{id}/read
	mux.Handle("POST /api/notifications/{id}/read", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid notification id")
			return
		}
		if err := st.MarkNotificationRead(r.Context(), p.UserID, id); err != nil {
			if errors.Is(err, store.ErrNotificationNotFound) {
				// Also the answer for somebody else's id: the caller must not
				// be able to probe which notifications exist.
				WriteError(w, http.StatusNotFound, "not_found", "notification not found")
				return
			}
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not mark the notification read")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"id": id, "read": true})
	})))

	// POST /api/notifications/read-all
	mux.Handle("POST /api/notifications/read-all", authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		n, err := st.MarkAllNotificationsRead(r.Context(), p.UserID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not mark notifications read")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]int64{"marked": n})
	})))
}
