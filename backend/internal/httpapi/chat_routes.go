package httpapi

import (
	"context"
	"errors"
	"net/http"

	"veylix/backend/internal/audit"
	"veylix/backend/internal/store"
)

// ChatStore is the persistence surface the chat handlers need.
type ChatStore interface {
	ChatThreadsFor(ctx context.Context, userID int64) ([]store.ChatThreadRow, error)
	OpenChatThread(ctx context.Context, callerID int64, callerRole string, peerID int64) (*store.ChatThreadRow, error)
	ChatThread(ctx context.Context, threadID, viewerID int64) (*store.ChatThreadRow, error)
	SetChatLocked(ctx context.Context, threadID, actorID int64, locked bool) error
	ChatMessages(ctx context.Context, threadID, viewerID, afterID int64, limit int) ([]store.ChatMessageRow, error)
	ChatMessage(ctx context.Context, threadID, viewerID, messageID int64) (*store.ChatMessageRow, error)
	SendChatMessage(ctx context.Context, threadID, senderID int64, body string) (*store.ChatMessageRow, error)
	EditChatMessage(ctx context.Context, threadID, editorID, messageID int64, body string, window int64) (*store.ChatMessageRow, error)
	HideChatMessage(ctx context.Context, threadID, messageID, viewerID int64) error
	UnhideChatMessage(ctx context.Context, threadID, messageID, viewerID int64) error
	HiddenChatMessages(ctx context.Context, threadID, viewerID int64, limit int) ([]store.ChatMessageRow, error)
	PinChatMessage(ctx context.Context, threadID, viewerID, messageID int64) error
	MarkChatRead(ctx context.Context, threadID, viewerID int64) error
	SetChatSaved(ctx context.Context, threadID, messageID, viewerID int64, saved bool) error
	SetChatImportant(ctx context.Context, threadID, messageID, viewerID int64, important bool) error
}

// writeChatError maps the store's chat errors onto the API envelope.
//
// The 403/404 split is deliberate. A non-participant who is *told the thread
// does not exist* would see their own UI link to a "missing" conversation they
// can see in a list — so 403 is the honest answer there. A message id, by
// contrast, answers 404 for both "missing" and "not yours", because message ids
// are sequential and would otherwise be an enumeration oracle.
func writeChatError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, store.ErrChatNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "conversation not found")
	case errors.Is(err, store.ErrChatMessage):
		WriteError(w, http.StatusNotFound, "not_found", "message not found")
	case errors.Is(err, store.ErrChatForbidden):
		WriteError(w, http.StatusForbidden, "forbidden", "you are not a participant of this conversation")
	case errors.Is(err, store.ErrChatLocked):
		WriteError(w, http.StatusConflict, "chat_locked", "this conversation is locked and accepts no new messages")
	case errors.Is(err, store.ErrChatPeer), errors.Is(err, store.ErrChatSelf):
		WriteError(w, http.StatusForbidden, "peer_not_reachable",
			"you can only message people connected to your account")
	case errors.Is(err, store.ErrChatEmptyBody):
		WriteError(w, http.StatusUnprocessableEntity, "validation_failed",
			"message must not be empty and must stay under 64 KiB")
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", fallback)
	}
}

// chatRouter carries the shared dependencies so each handler stays short.
type chatRouter struct {
	st      ChatStore
	record  func(context.Context, audit.Entry)
	authed  func(http.Handler) http.Handler
	editWin int64
}

// ChatRoutes registers the internal support chat (P7, REQUIREMENTS §2.6).
//
// Access is enforced by the store, not by these handlers: every read joins
// `chat_participants` on the caller's id, so "can I see this?" has exactly one
// implementation and the handlers only translate errors into status codes.
//
// Before this API the whole chat lived in the browser's localStorage: two
// devices disagreed, clearing the cache lost the transcript, and locking a
// conversation was decoration. A transcript is evidence, so it belongs on the
// server.
func ChatRoutes(mux *http.ServeMux, st ChatStore, aud *audit.Logger, authed func(http.Handler) http.Handler) {
	record := func(ctx context.Context, e audit.Entry) {
		if aud == nil {
			return
		}
		_ = aud.Record(ctx, e)
	}
	c := &chatRouter{st: st, record: record, authed: authed, editWin: 15 * 60}
	c.threadRoutes(mux)
	c.messageRoutes(mux)
}

// principal is the small accessor every handler needs.
func principal(w http.ResponseWriter, r *http.Request) (userID int64, role string, ok bool) {
	p, found := PrincipalFrom(r.Context())
	if !found {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return 0, "", false
	}
	return p.UserID, string(p.Role), true
}
