package httpapi

import (
	"net/http"
	"strconv"
)

// threadRoutes registers /api/chat/threads*.
func (c *chatRouter) threadRoutes(mux *http.ServeMux) {
	// GET /api/chat/threads — the caller's own inbox.
	mux.Handle("GET /api/chat/threads", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _, ok := principal(w, r)
		if !ok {
			return
		}
		rows, err := c.st.ChatThreadsFor(r.Context(), uid)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "could not read conversations")
			return
		}
		WriteJSON(w, http.StatusOK, rows)
	})))

	// POST /api/chat/threads — open (or reuse) the thread with a peer.
	mux.Handle("POST /api/chat/threads", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, role, ok := principal(w, r)
		if !ok {
			return
		}
		var in struct {
			PeerID int64 `json:"peer_user_id"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if in.PeerID <= 0 {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "peer_user_id is required")
			return
		}
		thread, err := c.st.OpenChatThread(r.Context(), uid, role, in.PeerID)
		if err != nil {
			writeChatError(w, err, "could not open the conversation")
			return
		}
		// 201 when created, 200 when it already existed — the store returns the
		// same row either way, so the caller can simply navigate to it.
		WriteJSON(w, http.StatusCreated, thread)
	})))

	// GET /api/chat/threads/{id}
	mux.Handle("GET /api/chat/threads/{id}", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _, ok := principal(w, r)
		if !ok {
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid conversation id")
			return
		}
		thread, err := c.st.ChatThread(r.Context(), id, uid)
		if err != nil {
			writeChatError(w, err, "could not read the conversation")
			return
		}
		WriteJSON(w, http.StatusOK, thread)
	})))

	// GET /api/chat/threads/{id}/messages?after=&limit=
	mux.Handle("GET /api/chat/threads/{id}/messages", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _, ok := principal(w, r)
		if !ok {
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid conversation id")
			return
		}
		after := int64(0)
		if v := r.URL.Query().Get("after"); v != "" {
			n, convErr := strconv.ParseInt(v, 10, 64)
			if convErr != nil || n < 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "after must be a non-negative integer")
				return
			}
			after = n
		}
		limit := 100
		if v := r.URL.Query().Get("limit"); v != "" {
			n, convErr := strconv.Atoi(v)
			if convErr != nil || n <= 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "limit must be a positive integer")
				return
			}
			limit = n
		}
		rows, err := c.st.ChatMessages(r.Context(), id, uid, after, limit)
		if err != nil {
			writeChatError(w, err, "could not read messages")
			return
		}
		WriteJSON(w, http.StatusOK, rows)
	})))

	// GET /api/chat/threads/{id}/hidden — what THIS viewer deleted for himself.
	//
	// The restore path needs it: the transcript read filters hidden rows out, so
	// once the screen is reloaded the reader has no id to ask for and the message
	// would look lost for good. Scoped to the caller, never the peer.
	mux.Handle("GET /api/chat/threads/{id}/hidden", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _, ok := principal(w, r)
		if !ok {
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid conversation id")
			return
		}
		limit := 100
		if v := r.URL.Query().Get("limit"); v != "" {
			n, convErr := strconv.Atoi(v)
			if convErr != nil || n <= 0 {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "limit must be a positive integer")
				return
			}
			limit = n
		}
		rows, err := c.st.HiddenChatMessages(r.Context(), id, uid, limit)
		if err != nil {
			writeChatError(w, err, "could not read hidden messages")
			return
		}
		WriteJSON(w, http.StatusOK, rows)
	})))

	// POST /api/chat/threads/{id}/read
	mux.Handle("POST /api/chat/threads/{id}/read", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _, ok := principal(w, r)
		if !ok {
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid conversation id")
			return
		}
		if err := c.st.MarkChatRead(r.Context(), id, uid); err != nil {
			writeChatError(w, err, "could not mark the conversation read")
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"thread_id": id, "read": true})
	})))
}
