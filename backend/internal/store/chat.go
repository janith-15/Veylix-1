package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Chat errors the transport maps to specific statuses.
var (
	ErrChatNotFound = errors.New("store: conversation not found")
	// ErrChatForbidden means the caller is not a participant. The transport
	// answers 403 — deliberately different from ErrChatNotFound, because a
	// non-participant DOES know the conversation exists (their UI would not
	// otherwise link to it), and hiding that would make the UI lie.
	ErrChatForbidden = errors.New("store: not a participant")
	ErrChatLocked    = errors.New("store: conversation is locked")
	ErrChatPeer      = errors.New("store: peer is not reachable for this account")
	ErrChatSelf      = errors.New("store: cannot start a conversation with yourself")
	ErrChatMessage   = errors.New("store: message not found")
	ErrChatEmptyBody = errors.New("store: message body is empty")
)

// ChatMessageRow is one message as returned to the API.
//
// `Edited` and `Mine` are derived for the UI rather than stored: the former
// from `EditedAt`, the latter from the caller's id. `Body` is already filtered
// for the caller — a hidden message never leaves the database.
type ChatMessageRow struct {
	ID          int64  `json:"id"`
	ThreadID    int64  `json:"thread_id"`
	SenderID    int64  `json:"sender_id"`
	SenderName  string `json:"sender_name"`
	SenderEmail string `json:"sender_email"`
	Body        string `json:"body"`
	CreatedAt   int64  `json:"created_at"`
	EditedAt    *int64 `json:"edited_at"`
	Important   bool   `json:"important"`
	Saved       bool   `json:"saved"`
}

// ChatThreadRow is one conversation for the calling participant.
//
// `Unread` is computed from the caller's `last_read_at`, so two participants
// legitimately see different numbers for the same thread.
type ChatThreadRow struct {
	ID          int64           `json:"id"`
	PeerID      int64           `json:"peer_id"`
	PeerName    string          `json:"peer_name"`
	PeerEmail   string          `json:"peer_email"`
	PeerRole    string          `json:"peer_role"`
	Locked      bool            `json:"locked"`
	LockedByID  *int64          `json:"locked_by_id"`
	PinnedID    *int64          `json:"pinned_message_id"`
	Unread      int             `json:"unread"`
	LastMessage *ChatMessageRow `json:"last_message"`
	CreatedAt   int64           `json:"created_at"`
}

// PairKey returns the canonical key for a conversation between two accounts.
//
// Ordering the ids makes the key independent of who starts the thread, which is
// what lets the UNIQUE index prevent a duplicate transcript.
func PairKey(a, b int64) string {
	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}
	return strconv.FormatInt(lo, 10) + "|" + strconv.FormatInt(hi, 10)
}

// peerKeyParticipants splits a pair key back into its two ids.
func peerKeyParticipants(key string) (int64, int64, bool) {
	parts := strings.Split(key, "|")
	if len(parts) != 2 {
		return 0, 0, false
	}
	a, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	b, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return a, b, true
}

// isChatParticipant reports whether `userID` belongs to the conversation.
func isChatParticipantTx(ctx context.Context, tx *sql.Tx, threadID, userID int64) (bool, error) {
	var n int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chat_participants WHERE conversation_id = ? AND user_id = ?`,
		threadID, userID).Scan(&n); err != nil {
		return false, fmt.Errorf("store: check participant: %w", err)
	}
	return n > 0, nil
}

// ChatThreadsFor lists the caller's conversations, newest activity first.
//
// Only conversations the caller participates in are returned — the join on
// `chat_participants` IS the access rule, so there is no separate check to
// forget. `last_message` honours the caller's own hidden messages: a message
// they deleted for themselves must not reappear in a preview.
func (s *Store) ChatThreadsFor(ctx context.Context, userID int64) ([]ChatThreadRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.created_at, c.locked_at, c.locked_by, c.pinned_message_id,
		        p.user_id, COALESCE(u.display_name, ''), u.email, u.role, p.last_read_at
		   FROM chat_conversations c
		   JOIN chat_participants me  ON me.conversation_id = c.id AND me.user_id = ?
		   JOIN chat_participants p   ON p.conversation_id = c.id AND p.user_id <> ?
		   JOIN users u               ON u.id = p.user_id
		  ORDER BY c.id DESC`, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list chat threads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ChatThreadRow, 0, 8)
	lastReads := make(map[int64]*int64, 8)
	for rows.Next() {
		var t ChatThreadRow
		var lockedAt, lockedBy, pinned sql.NullInt64
		var lastRead sql.NullInt64
		if err := rows.Scan(&t.ID, &t.CreatedAt, &lockedAt, &lockedBy, &pinned,
			&t.PeerID, &t.PeerName, &t.PeerEmail, &t.PeerRole, &lastRead); err != nil {
			return nil, fmt.Errorf("store: scan chat thread: %w", err)
		}
		t.Locked = lockedAt.Valid
		if lockedBy.Valid {
			v := lockedBy.Int64
			t.LockedByID = &v
		}
		if pinned.Valid {
			v := pinned.Int64
			t.PinnedID = &v
		}
		if lastRead.Valid {
			v := lastRead.Int64
			lastReads[t.ID] = &v
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Second pass: the preview and the unread count, per thread. Two short
	// queries per conversation beats one unreadable mega-join, and the number of
	// conversations a single account has is inherently small.
	for i := range out {
		last, err := s.lastVisibleMessage(ctx, out[i].ID, userID)
		if err != nil {
			return nil, err
		}
		out[i].LastMessage = last
		var unread int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*)
			   FROM chat_messages m
			  WHERE m.conversation_id = ?
			    AND m.sender_user_id <> ?
			    AND m.created_at > COALESCE(?, 0)
			    AND NOT EXISTS (SELECT 1 FROM chat_message_hidden h
			                     WHERE h.message_id = m.id AND h.user_id = ?)`,
			out[i].ID, userID, nullInt64(lastReads[out[i].ID]), userID).Scan(&unread); err != nil {
			return nil, fmt.Errorf("store: count unread: %w", err)
		}
		out[i].Unread = unread
	}
	return out, nil
}

// lastVisibleMessage returns the newest message the viewer may see, with the
// caller's own `important`/`saved` flags applied.
func (s *Store) lastVisibleMessage(ctx context.Context, threadID, viewerID int64) (*ChatMessageRow, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT m.id, m.conversation_id, m.sender_user_id,
		        COALESCE(u.display_name, ''), u.email, m.body, m.created_at,
		        m.edited_at,
		        EXISTS (SELECT 1 FROM chat_message_important i
		                 WHERE i.message_id = m.id AND i.user_id = ?),
		        EXISTS (SELECT 1 FROM chat_message_saved sv
		                 WHERE sv.message_id = m.id AND sv.user_id = ?)
		   FROM chat_messages m
		   JOIN users u ON u.id = m.sender_user_id
		  WHERE m.conversation_id = ?
		    AND NOT EXISTS (SELECT 1 FROM chat_message_hidden h
		                     WHERE h.message_id = m.id AND h.user_id = ?)
		  ORDER BY m.id DESC LIMIT 1`, viewerID, viewerID, threadID, viewerID)
	msg, err := scanChatMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read last message: %w", err)
	}
	return msg, nil
}

func scanChatMessage(sc interface{ Scan(...any) error }) (*ChatMessageRow, error) {
	var m ChatMessageRow
	var edited sql.NullInt64
	var important, saved int
	if err := sc.Scan(&m.ID, &m.ThreadID, &m.SenderID, &m.SenderName, &m.SenderEmail,
		&m.Body, &m.CreatedAt, &edited, &important, &saved); err != nil {
		return nil, err
	}
	if edited.Valid {
		v := edited.Int64
		m.EditedAt = &v
	}
	m.Important = important == 1
	m.Saved = saved == 1
	return &m, nil
}

// nullInt64 maps a nil pointer to SQL NULL.
func nullInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
