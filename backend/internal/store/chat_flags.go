package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// requireMessageParticipant checks the viewer belongs to the message's thread
// and returns the message's sender.
//
// The PARTICIPANT check runs FIRST on purpose. Reading the message first would
// turn this into an enumeration oracle: a non-participant could tell a real
// message id (→ 403 "not a participant") from a fake one (→ 404), which leaks
// the existence of other people's conversations. With the order below, both
// answers are identical.
func requireMessageParticipant(ctx context.Context, tx *sql.Tx, threadID, messageID, viewerID int64) (int64, error) {
	ok, err := isChatParticipantTx(ctx, tx, threadID, viewerID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrChatMessage
	}
	var senderID int64
	err = tx.QueryRowContext(ctx,
		`SELECT sender_user_id FROM chat_messages WHERE id = ? AND conversation_id = ?`,
		messageID, threadID).Scan(&senderID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrChatMessage
	}
	if err != nil {
		return 0, fmt.Errorf("store: lookup chat message: %w", err)
	}
	return senderID, nil
}

// HideChatMessage removes a message from ONE participant's view.
//
// This is what the UI calls "delete": it must never remove the other person's
// copy — a chat transcript is evidence, and one side silently erasing it would
// be a lie. Hiding is idempotent.
func (s *Store) HideChatMessage(ctx context.Context, threadID, messageID, viewerID int64) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := requireMessageParticipant(ctx, tx, threadID, messageID, viewerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO chat_message_hidden (message_id, user_id, hidden_at)
			 VALUES (?, ?, ?)`, messageID, viewerID, nowUnix()); err != nil {
			return fmt.Errorf("store: hide chat message: %w", err)
		}
		// A hidden message cannot stay pinned: the pin would leak the text
		// back into the viewer's header after they removed it.
		if _, err := tx.ExecContext(ctx,
			`UPDATE chat_conversations SET pinned_message_id = NULL
			  WHERE id = ? AND pinned_message_id = ?`, threadID, messageID); err != nil {
			return fmt.Errorf("store: clear pin: %w", err)
		}
		return nil
	})
}

// UnhideChatMessage puts a hidden message back into the caller's view.
//
// Without this, "Delete" was a one-way door: the row went into
// `chat_message_hidden` and every read filtered it out, so a mis-click next to
// Copy cost the user a message with no way back. The product already ships a
// 3-day undo window for releasing a VPN (D-027.2); permanent loss on a one-click
// control was not consistent with that, and nothing in REQUIREMENTS ever asked
// for irreversibility.
//
// It only ever touches the CALLER's own hidden row, so it cannot resurrect a
// message on somebody else's screen. Idempotent: unhiding something that was
// never hidden is a no-op that still answers with the message.
func (s *Store) UnhideChatMessage(ctx context.Context, threadID, messageID, viewerID int64) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		// Same order as Hide: the participant check runs before any lookup so a
		// non-participant learns nothing about the message's existence.
		if _, err := requireMessageParticipant(ctx, tx, threadID, messageID, viewerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM chat_message_hidden WHERE message_id = ? AND user_id = ?`,
			messageID, viewerID); err != nil {
			return fmt.Errorf("store: unhide chat message: %w", err)
		}
		return nil
	})
}

// HiddenChatMessages lists the messages THIS viewer has hidden, newest first.
//
// A reversible delete needs somewhere to restore from: the transcript read
// filters hidden rows out, so without this list a restored message would be
// invisible to the UI after a reload (the reader has no id to ask for).
func (s *Store) HiddenChatMessages(ctx context.Context, threadID, viewerID int64, limit int) ([]ChatMessageRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
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
		   JOIN chat_message_hidden h ON h.message_id = m.id AND h.user_id = ?
		  WHERE m.conversation_id = ?
		  ORDER BY m.id DESC LIMIT ?`,
		viewerID, viewerID, viewerID, threadID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list hidden chat messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ChatMessageRow, 0, limit)
	for rows.Next() {
		m, err := scanChatMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan hidden chat message: %w", err)
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// PinChatMessage pins (or, with messageID 0, unpins) one message in a thread.
// A pin is conversation-wide, so it is a staff-level control and audited by the
// transport.
func (s *Store) PinChatMessage(ctx context.Context, threadID, viewerID, messageID int64) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		ok, err := isChatParticipantTx(ctx, tx, threadID, viewerID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrChatForbidden
		}
		if messageID == 0 {
			if _, err := tx.ExecContext(ctx,
				`UPDATE chat_conversations SET pinned_message_id = NULL WHERE id = ?`, threadID); err != nil {
				return fmt.Errorf("store: unpin chat message: %w", err)
			}
			return nil
		}
		if _, err := requireMessageParticipant(ctx, tx, threadID, messageID, viewerID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE chat_conversations SET pinned_message_id = ? WHERE id = ?`,
			messageID, threadID); err != nil {
			return fmt.Errorf("store: pin chat message: %w", err)
		}
		return nil
	})
}

// MarkChatRead advances the caller's read marker to now.
func (s *Store) MarkChatRead(ctx context.Context, threadID, viewerID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE chat_participants SET last_read_at = ?
		  WHERE conversation_id = ? AND user_id = ?`, nowUnix(), threadID, viewerID)
	if err != nil {
		return fmt.Errorf("store: mark chat read: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrChatForbidden
	}
	return nil
}

// toggleChatFlag inserts or deletes a per-viewer (message_id, user_id) row.
//
// Both bookmarks and "important" are per person, so they share this helper: the
// only difference is which table they live in. The viewer must be a participant
// of the message's thread, otherwise one account could flag another's messages.
func (s *Store) toggleChatFlag(ctx context.Context, table string, threadID, messageID, viewerID int64, on bool) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := requireMessageParticipant(ctx, tx, threadID, messageID, viewerID); err != nil {
			return err
		}
		var err error
		if on {
			_, err = tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO `+table+` (message_id, user_id, marked_at) VALUES (?, ?, ?)`,
				messageID, viewerID, nowUnix())
		} else {
			_, err = tx.ExecContext(ctx,
				`DELETE FROM `+table+` WHERE message_id = ? AND user_id = ?`, messageID, viewerID)
		}
		if err != nil {
			return fmt.Errorf("store: set chat flag: %w", err)
		}
		return nil
	})
}

// SetChatSaved bookmarks (or un-bookmarks) a message for one viewer.
func (s *Store) SetChatSaved(ctx context.Context, threadID, messageID, viewerID int64, saved bool) error {
	// chat_message_saved uses `saved_at` rather than `marked_at`, so it cannot
	// share the helper's column name; keep the two statements explicit.
	return s.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := requireMessageParticipant(ctx, tx, threadID, messageID, viewerID); err != nil {
			return err
		}
		var err error
		if saved {
			_, err = tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO chat_message_saved (message_id, user_id, saved_at)
				 VALUES (?, ?, ?)`, messageID, viewerID, nowUnix())
		} else {
			_, err = tx.ExecContext(ctx,
				`DELETE FROM chat_message_saved WHERE message_id = ? AND user_id = ?`,
				messageID, viewerID)
		}
		if err != nil {
			return fmt.Errorf("store: set chat saved: %w", err)
		}
		return nil
	})
}

// SetChatImportant flags (or un-flags) a message as important for one viewer.
func (s *Store) SetChatImportant(ctx context.Context, threadID, messageID, viewerID int64, important bool) error {
	return s.toggleChatFlag(ctx, "chat_message_important", threadID, messageID, viewerID, important)
}
