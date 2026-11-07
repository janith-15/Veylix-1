package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"veylix/backend/internal/audit"
)

// messageRoutes registers /api/chat/threads/{id}/messages* and the per-message
// actions. Locking and pinning are staff-only (Bible §2.6), gated here.
func (c *chatRouter) messageRoutes(mux *http.ServeMux) {
	// POST /api/chat/threads/{id}/messages — send.
	mux.Handle("POST /api/chat/threads/{id}/messages", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _, ok := principal(w, r)
		if !ok {
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid conversation id")
			return
		}
		var in struct {
			Body string `json:"body"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		msg, err := c.st.SendChatMessage(r.Context(), id, uid, in.Body)
		if err != nil {
			writeChatError(w, err, "could not send the message")
			return
		}
		WriteJSON(w, http.StatusCreated, msg)
	})))

	// PATCH /api/chat/messages/{id} — the author edits within the window.
	mux.Handle("PATCH /api/chat/messages/{id}", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _, ok := principal(w, r)
		if !ok {
			return
		}
		msgID, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid message id")
			return
		}
		var in struct {
			Body     string `json:"body"`
			ThreadID int64  `json:"thread_id"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if in.ThreadID <= 0 {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "thread_id is required")
			return
		}
		msg, err := c.st.EditChatMessage(r.Context(), in.ThreadID, uid, msgID, in.Body, c.editWin)
		if err != nil {
			writeChatError(w, err, "could not edit the message")
			return
		}
		WriteJSON(w, http.StatusOK, msg)
	})))

	// DELETE /api/chat/messages/{id}?thread_id= — hide from the caller's view.
	//
	// This is the UI's "delete". It removes the message for the caller ONLY: the
	// other participant's copy is evidence and must survive (REQUIREMENTS §2.6).
	mux.Handle("DELETE /api/chat/messages/{id}", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _, ok := principal(w, r)
		if !ok {
			return
		}
		msgID, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid message id")
			return
		}
		threadID, convErr := strconv.ParseInt(r.URL.Query().Get("thread_id"), 10, 64)
		if convErr != nil || threadID <= 0 {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "thread_id is required")
			return
		}
		if err := c.st.HideChatMessage(r.Context(), threadID, msgID, uid); err != nil {
			writeChatError(w, err, "could not delete the message")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})))

	// POST /api/chat/messages/{id}/unhide {thread_id} — put it back.
	//
	// The counterpart of the DELETE above. A hide is a per-viewer visibility
	// flag, not a destruction, so it must be reversible: a mis-click next to
	// Copy used to cost the reader the message forever. Only the caller's own
	// hidden row is removed, so this can never reveal a message on the peer's
	// screen, and it is idempotent.
	mux.Handle("POST /api/chat/messages/{id}/unhide", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _, ok := principal(w, r)
		if !ok {
			return
		}
		msgID, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid message id")
			return
		}
		var in struct {
			ThreadID int64 `json:"thread_id"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if in.ThreadID <= 0 {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "thread_id is required")
			return
		}
		if err := c.st.UnhideChatMessage(r.Context(), in.ThreadID, msgID, uid); err != nil {
			writeChatError(w, err, "could not restore the message")
			return
		}
		// Answer with the message itself so the client can splice it back into
		// the transcript at the right position without re-reading the page.
		msg, err := c.st.ChatMessage(r.Context(), in.ThreadID, uid, msgID)
		if err != nil {
			writeChatError(w, err, "could not read the restored message")
			return
		}
		WriteJSON(w, http.StatusOK, msg)
	})))

	// POST /api/chat/messages/{id}/save       — bookmark for the caller
	// POST /api/chat/messages/{id}/important  — flag for the caller
	//
	// Both are per-viewer, so one person flagging a message never changes what
	// the other sees.
	for _, f := range []struct {
		path string
		key  string
		set  func(ctx context.Context, threadID, messageID, viewerID int64, on bool) error
	}{
		{"/save", "saved", c.st.SetChatSaved},
		{"/important", "important", c.st.SetChatImportant},
	} {
		path, key, set := f.path, f.key, f.set
		mux.Handle("POST /api/chat/messages/{id}"+path, c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, _, ok := principal(w, r)
			if !ok {
				return
			}
			msgID, err := pathID(r)
			if err != nil {
				WriteError(w, http.StatusBadRequest, "validation_failed", "invalid message id")
				return
			}
			var in struct {
				ThreadID int64 `json:"thread_id"`
				On       *bool `json:"on"`
			}
			if err := DecodeJSON(r, &in); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
				return
			}
			if in.ThreadID <= 0 || in.On == nil {
				WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "thread_id and on are required")
				return
			}
			if err := set(r.Context(), in.ThreadID, msgID, uid, *in.On); err != nil {
				writeChatError(w, err, "could not update the message")
				return
			}
			WriteJSON(w, http.StatusOK, map[string]any{"message_id": msgID, key: *in.On})
		})))
	}

	// POST /api/chat/threads/{id}/pin {message_id} — staff only. message_id 0
	// unpins, because "no pin" is a state the conversation can be in.
	mux.Handle("POST /api/chat/threads/{id}/pin", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, role, ok := principal(w, r)
		if !ok {
			return
		}
		if role == "client" {
			WriteError(w, http.StatusForbidden, "forbidden", "only staff can pin a message")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid conversation id")
			return
		}
		var in struct {
			MessageID int64 `json:"message_id"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if in.MessageID < 0 {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "message_id cannot be negative")
			return
		}
		if err := c.st.PinChatMessage(r.Context(), id, uid, in.MessageID); err != nil {
			writeChatError(w, err, "could not pin the message")
			return
		}
		c.record(r.Context(), audit.Entry{
			ActorID: uid, Action: audit.ActionChatPin,
			TargetType: "chat_thread", TargetID: strconv.FormatInt(id, 10),
			Context: map[string]any{"message_id": in.MessageID},
		})
		WriteJSON(w, http.StatusOK, map[string]any{"thread_id": id, "pinned_message_id": in.MessageID})
	})))

	// POST /api/chat/threads/{id}/lock {locked} — staff only.
	//
	// A lock changes what the OTHER participant may do, so it is always audited:
	// "who stopped this conversation, and when" must be answerable.
	mux.Handle("POST /api/chat/threads/{id}/lock", c.authed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, role, ok := principal(w, r)
		if !ok {
			return
		}
		if role == "client" {
			WriteError(w, http.StatusForbidden, "forbidden", "only staff can lock a conversation")
			return
		}
		id, err := pathID(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "validation_failed", "invalid conversation id")
			return
		}
		var in struct {
			Locked *bool `json:"locked"`
		}
		if err := DecodeJSON(r, &in); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}
		if in.Locked == nil {
			WriteError(w, http.StatusUnprocessableEntity, "validation_failed", "locked is required")
			return
		}
		if err := c.st.SetChatLocked(r.Context(), id, uid, *in.Locked); err != nil {
			writeChatError(w, err, "could not update the conversation")
			return
		}
		c.record(r.Context(), audit.Entry{
			ActorID: uid, Action: audit.ActionChatLock,
			TargetType: "chat_thread", TargetID: strconv.FormatInt(id, 10),
			Context: map[string]any{"locked": *in.Locked},
		})
		WriteJSON(w, http.StatusOK, map[string]any{"thread_id": id, "locked": *in.Locked})
	})))
}
