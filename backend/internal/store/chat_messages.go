package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ChatMessages returns a conversation's messages, oldest first, as the viewer
// sees them: their own hidden messages are excluded.
//
// `afterID` is a cursor rather than an offset. An offset would skip or repeat a
// message whenever somebody else sends one between two pages, which is exactly
// the situation a chat screen is in all the time.
func (s *Store) ChatMessages(ctx context.Context, threadID, viewerID, afterID int64, limit int) ([]ChatMessageRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	// Participant check first: a non-participant must not learn whether the
	// thread even has messages.
	if _, err := s.ChatThread(ctx, threadID, viewerID); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.conversation_id, m.sender_user_id,
		        COALESCE(u.display_name, ''), u.email, m.body, m.created_at, m.edited_at,
		        EXISTS (SELECT 1 FROM chat_message_important i
		                 WHERE i.message_id = m.id AND i.user_id = ?),
		        EXISTS (SELECT 1 FROM chat_message_saved sv
		                 WHERE sv.message_id = m.id AND sv.user_id = ?)
		   FROM chat_messages m
		   JOIN users u ON u.id = m.sender_user_id
		  WHERE m.conversation_id = ? AND m.id > ?
		    AND NOT EXISTS (SELECT 1 FROM chat_message_hidden h
		                     WHERE h.message_id = m.id AND h.user_id = ?)
		  ORDER BY m.id ASC LIMIT ?`,
		viewerID, viewerID, threadID, afterID, viewerID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list chat messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ChatMessageRow, 0, limit)
	for rows.Next() {
		m, err := scanChatMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan chat message: %w", err)
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// ChatMessage reads one message as the viewer sees it.
func (s *Store) ChatMessage(ctx context.Context, threadID, viewerID, messageID int64) (*ChatMessageRow, error) {
	m, err := scanChatMessage(s.db.QueryRowContext(ctx,
		`SELECT m.id, m.conversation_id, m.sender_user_id,
		        COALESCE(u.display_name, ''), u.email, m.body, m.created_at, m.edited_at,
		        EXISTS (SELECT 1 FROM chat_message_important i
		                 WHERE i.message_id = m.id AND i.user_id = ?),
		        EXISTS (SELECT 1 FROM chat_message_saved sv
		                 WHERE sv.message_id = m.id AND sv.user_id = ?)
		   FROM chat_messages m
		   JOIN users u ON u.id = m.sender_user_id
		  WHERE m.id = ? AND m.conversation_id = ?
		    AND NOT EXISTS (SELECT 1 FROM chat_message_hidden h
		                     WHERE h.message_id = m.id AND h.user_id = ?)`,
		viewerID, viewerID, messageID, threadID, viewerID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrChatMessage
	}
	if err != nil {
		return nil, fmt.Errorf("store: read chat message: %w", err)
	}
	return m, nil
}

// SendChatMessage appends a message and stamps the sender as having read the
// thread up to now.
//
// A locked conversation refuses new messages (ErrChatLocked) but stays
// readable — the transcript is evidence, so locking must never hide it.
func (s *Store) SendChatMessage(ctx context.Context, threadID, senderID int64, body string) (*ChatMessageRow, error) {
	trimmed, err := trimmedBody(body)
	if err != nil {
		return nil, err
	}

	var messageID int64
	err = s.Tx(ctx, func(tx *sql.Tx) error {
		ok, err := isChatParticipantTx(ctx, tx, threadID, senderID)
		if err != nil {
			return err
		}
		if !ok {
			var exists int
			if e := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM chat_conversations WHERE id = ?`, threadID).Scan(&exists); e != nil {
				return fmt.Errorf("store: lookup conversation: %w", e)
			}
			if exists == 0 {
				return ErrChatNotFound
			}
			return ErrChatForbidden
		}

		var locked sql.NullInt64
		if err := tx.QueryRowContext(ctx,
			`SELECT locked_at FROM chat_conversations WHERE id = ?`, threadID).Scan(&locked); err != nil {
			return fmt.Errorf("store: read lock state: %w", err)
		}
		if locked.Valid {
			return ErrChatLocked
		}

		now := nowUnix()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO chat_messages (conversation_id, sender_user_id, body, created_at)
			 VALUES (?, ?, ?, ?)`, threadID, senderID, trimmed, now)
		if err != nil {
			return fmt.Errorf("store: insert chat message: %w", err)
		}
		messageID, _ = res.LastInsertId()

		// Sending is an acknowledgement: the sender has obviously read
		// everything before it, so their badge for this thread clears.
		if _, err := tx.ExecContext(ctx,
			`UPDATE chat_participants SET last_read_at = ? WHERE conversation_id = ? AND user_id = ?`,
			now, threadID, senderID); err != nil {
			return fmt.Errorf("store: update read marker: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ChatMessage(ctx, threadID, senderID, messageID)
}

// EditChatMessage rewrites the author's own message inside the edit window.
//
// The previous text is preserved in `original_body` (first edit only), so an
// edit never destroys what was said — the transcript stays evidence. Only the
// author may edit, and only within `window` seconds of sending.
func (s *Store) EditChatMessage(ctx context.Context, threadID, editorID, messageID int64, body string, window int64) (*ChatMessageRow, error) {
	trimmed, err := trimmedBody(body)
	if err != nil {
		return nil, err
	}
	if window <= 0 {
		window = 15 * 60
	}

	err = s.Tx(ctx, func(tx *sql.Tx) error {
		var senderID, createdAt int64
		var current string
		err := tx.QueryRowContext(ctx,
			`SELECT sender_user_id, created_at, body FROM chat_messages
			  WHERE id = ? AND conversation_id = ?`, messageID, threadID).
			Scan(&senderID, &createdAt, &current)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChatMessage
		}
		if err != nil {
			return fmt.Errorf("store: lookup chat message: %w", err)
		}
		if senderID != editorID {
			// Not the author: refuse exactly like "missing", so a caller cannot
			// probe which ids exist in somebody else's conversation.
			return ErrChatMessage
		}
		if nowUnix()-createdAt > window {
			return fmt.Errorf("%w: edit window has passed", ErrChatMessage)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE chat_messages
			    SET body = ?,
			        original_body = COALESCE(original_body, ?),
			        edited_at = ?
			  WHERE id = ?`, trimmed, current, nowUnix(), messageID); err != nil {
			return fmt.Errorf("store: edit chat message: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ChatMessage(ctx, threadID, editorID, messageID)
}
