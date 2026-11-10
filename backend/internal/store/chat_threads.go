package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ChatMaxBody is the maximum message length, in bytes.
//
// A 64 KiB cap keeps a single message from becoming a storage problem on the
// 500 MB target and matches the per-client chat budget (D-027.6), which is
// meaningless if one message can exceed it.
const ChatMaxBody = 64 * 1024

// ChatPeerReachable reports whether `peerID` is inside the caller's chat scope.
//
// It is the same rule `Directory` implements, re-stated as a yes/no query
// because a conversation is created by id, not by picking from a list. Living
// here means "may I write to this person?" has exactly one answer, used both
// when a thread is opened and before every message.
func (s *Store) ChatPeerReachable(ctx context.Context, callerID int64, callerRole string, peerID int64) (bool, error) {
	if callerID == peerID {
		return false, nil
	}

	var peerRole string
	err := s.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, peerID).Scan(&peerRole)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: lookup chat peer: %w", err)
	}

	switch callerRole {
	case "super_admin":
		// The platform owner may address anybody: they are the support desk.
		return true, nil
	case "admin":
		if peerRole == "super_admin" {
			return true, nil
		}
		if peerRole != "client" {
			return false, nil // another reseller is not addressable
		}
		return s.soldTo(ctx, callerID, peerID)
	default: // client
		if peerRole == "super_admin" {
			return true, nil
		}
		if peerRole != "admin" {
			return false, nil // another client is not addressable
		}
		return s.soldTo(ctx, peerID, callerID)
	}
}

// soldTo reports whether one account sold a VPN to the other.
func (s *Store) soldTo(ctx context.Context, sellerID, clientID int64) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vpn_assignments WHERE client_user_id = ? AND seller_user_id = ?`,
		clientID, sellerID).Scan(&n); err != nil {
		return false, fmt.Errorf("store: check chat relationship: %w", err)
	}
	return n > 0, nil
}

// OpenChatThread returns the conversation between the caller and `peerID`,
// creating it on first use. It is idempotent: the same pair always yields the
// same thread, so two clicks (or two devices) cannot split a transcript.
func (s *Store) OpenChatThread(ctx context.Context, callerID int64, callerRole string, peerID int64) (*ChatThreadRow, error) {
	if callerID == peerID {
		return nil, ErrChatSelf
	}
	ok, err := s.ChatPeerReachable(ctx, callerID, callerRole, peerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrChatPeer
	}

	key := PairKey(callerID, peerID)
	var threadID int64
	err = s.Tx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM chat_conversations WHERE pair_key = ?`, key).Scan(&threadID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: lookup conversation: %w", err)
		}

		res, err := tx.ExecContext(ctx,
			`INSERT INTO chat_conversations (pair_key, created_at) VALUES (?, ?)`, key, nowUnix())
		if err != nil {
			// A concurrent create won the race: the UNIQUE index is the guard,
			// and the right answer is the row that already exists.
			if again := tx.QueryRowContext(ctx,
				`SELECT id FROM chat_conversations WHERE pair_key = ?`, key).Scan(&threadID); again == nil {
				return nil
			}
			return fmt.Errorf("store: create conversation: %w", err)
		}
		threadID, _ = res.LastInsertId()
		for _, uid := range []int64{callerID, peerID} {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO chat_participants (conversation_id, user_id) VALUES (?, ?)`,
				threadID, uid); err != nil {
				return fmt.Errorf("store: add participant: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ChatThread(ctx, threadID, callerID)
}

// ChatThread reads one conversation as `viewerID` sees it.
func (s *Store) ChatThread(ctx context.Context, threadID, viewerID int64) (*ChatThreadRow, error) {
	var t ChatThreadRow
	var lockedAt, lockedBy, pinned, lastRead sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT c.id, c.created_at, c.locked_at, c.locked_by, c.pinned_message_id,
		        p.user_id, COALESCE(u.display_name, ''), u.email, u.role, me.last_read_at
		   FROM chat_conversations c
		   JOIN chat_participants me ON me.conversation_id = c.id AND me.user_id = ?
		   JOIN chat_participants p  ON p.conversation_id = c.id AND p.user_id <> ?
		   JOIN users u              ON u.id = p.user_id
		  WHERE c.id = ?`, viewerID, viewerID, threadID).
		Scan(&t.ID, &t.CreatedAt, &lockedAt, &lockedBy, &pinned,
			&t.PeerID, &t.PeerName, &t.PeerEmail, &t.PeerRole, &lastRead)
	if errors.Is(err, sql.ErrNoRows) {
		// Either the conversation does not exist, or the viewer is not in it.
		// Tell them apart so the transport can answer 404 vs 403 honestly.
		var exists int
		if e := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM chat_conversations WHERE id = ?`, threadID).Scan(&exists); e != nil {
			return nil, fmt.Errorf("store: lookup conversation: %w", e)
		}
		if exists == 0 {
			return nil, ErrChatNotFound
		}
		return nil, ErrChatForbidden
	}
	if err != nil {
		return nil, fmt.Errorf("store: read conversation: %w", err)
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

	last, err := s.lastVisibleMessage(ctx, threadID, viewerID)
	if err != nil {
		return nil, err
	}
	t.LastMessage = last

	// Unread = messages from the peer newer than this participant's marker,
	// minus anything this viewer hid. A NULL marker means "never opened", which
	// is the same arithmetic with 0.
	var unread int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chat_messages m
		  WHERE m.conversation_id = ? AND m.sender_user_id <> ? AND m.created_at > ?
		    AND NOT EXISTS (SELECT 1 FROM chat_message_hidden h
		                     WHERE h.message_id = m.id AND h.user_id = ?)`,
		threadID, viewerID, lastRead.Int64, viewerID).Scan(&unread); err != nil {
		return nil, fmt.Errorf("store: count unread: %w", err)
	}
	t.Unread = unread
	return &t, nil
}

// SetChatLocked locks or unlocks a conversation.
//
// A locked thread accepts no new messages but stays readable. Only staff may
// set it (Bible §2.6); the transport enforces the role, and this records who.
func (s *Store) SetChatLocked(ctx context.Context, threadID, actorID int64, locked bool) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		ok, err := isChatParticipantTx(ctx, tx, threadID, actorID)
		if err != nil {
			return err
		}
		if !ok {
			// Distinguish "no such thread" from "not yours" for the caller.
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
		if locked {
			_, err = tx.ExecContext(ctx,
				`UPDATE chat_conversations SET locked_at = ?, locked_by = ? WHERE id = ?`,
				nowUnix(), actorID, threadID)
		} else {
			_, err = tx.ExecContext(ctx,
				`UPDATE chat_conversations SET locked_at = NULL, locked_by = NULL WHERE id = ?`,
				threadID)
		}
		if err != nil {
			return fmt.Errorf("store: set chat lock: %w", err)
		}
		return nil
	})
}

// trimmedBody validates a message body.
func trimmedBody(body string) (string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", ErrChatEmptyBody
	}
	if len(trimmed) > ChatMaxBody {
		return "", fmt.Errorf("%w: body exceeds %d bytes", ErrChatEmptyBody, ChatMaxBody)
	}
	return trimmed, nil
}
