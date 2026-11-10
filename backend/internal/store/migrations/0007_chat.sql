-- 0007_chat.sql — the chat feature set the UI already offers (REQUIREMENTS §2.6)
--
-- Source: docs/REQUIREMENTS.md §2.6 (Client ↔ seller, Admin ↔ their clients,
-- Super Admin ↔ everyone), docs/API_CONTRACT.md §5b.
--
-- 0003 created the minimum (conversations, participants, messages). The UI has
-- shipped for weeks with edit, soft delete, pin, lock, bookmark and "important"
-- — all of which lived ONLY in the browser's localStorage, so two devices
-- disagreed and a reload could lose them. This migration gives those features a
-- home on the server, which is where a transcript belongs.
--
-- Additive only: nothing is dropped, no row is rewritten.

-- ---------------------------------------------------------------------------
-- Conversations: a stable pair key + the operator controls
-- ---------------------------------------------------------------------------
-- `pair_key` is "min(userA)|max(userB)". A conversation is per PAIR, so
-- creating the same thread twice must return the existing row rather than
-- duplicating a transcript — the UNIQUE index is what makes that true even if
-- two requests race. The store computes it, never the client.
ALTER TABLE chat_conversations ADD COLUMN pair_key TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_pair ON chat_conversations (pair_key);

-- Locking stops new messages but keeps the transcript readable. Only an Admin
-- or Super Admin may set it (Bible §2.6), which the handler enforces.
ALTER TABLE chat_conversations ADD COLUMN locked_at  INTEGER;
ALTER TABLE chat_conversations ADD COLUMN locked_by  INTEGER REFERENCES users (id) ON DELETE SET NULL;

-- ---------------------------------------------------------------------------
-- Participants: per-viewer read marker
-- ---------------------------------------------------------------------------
-- Read state is per person, so it belongs on the participant row, not on the
-- conversation. NULL = never opened; it is NOT "read at epoch 0".
ALTER TABLE chat_participants ADD COLUMN last_read_at INTEGER;

-- ---------------------------------------------------------------------------
-- Messages: edit / soft delete / pin / important
-- ---------------------------------------------------------------------------
-- `edited_at` + `original_body` keep the edit auditable: the earlier text is
-- preserved rather than overwritten, so "what was actually said" survives.
ALTER TABLE chat_messages ADD COLUMN edited_at     INTEGER;
ALTER TABLE chat_messages ADD COLUMN original_body TEXT;

-- Soft delete is a visibility decision by one participant, so it is a second
-- table: removing my copy must never remove the other person's.
CREATE TABLE IF NOT EXISTS chat_message_hidden (
    message_id INTEGER NOT NULL REFERENCES chat_messages (id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    hidden_at  INTEGER NOT NULL,
    PRIMARY KEY (message_id, user_id)
);

-- A pin is a property of the conversation (one at a time), which is why it
-- lives here and not on the message.
ALTER TABLE chat_conversations ADD COLUMN pinned_message_id INTEGER
    REFERENCES chat_messages (id) ON DELETE SET NULL;

-- Bookmarks and "important" are per viewer.
CREATE TABLE IF NOT EXISTS chat_message_saved (
    message_id INTEGER NOT NULL REFERENCES chat_messages (id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    saved_at   INTEGER NOT NULL,
    PRIMARY KEY (message_id, user_id)
);

CREATE TABLE IF NOT EXISTS chat_message_important (
    message_id  INTEGER NOT NULL REFERENCES chat_messages (id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    marked_at   INTEGER NOT NULL,
    PRIMARY KEY (message_id, user_id)
);

-- Listing a user's inbox walks participants → conversations, newest first.
CREATE INDEX IF NOT EXISTS idx_chat_part_user ON chat_participants (user_id);

-- Backfill: every existing conversation gets its pair key, so the UNIQUE index
-- above can never be violated by a row created before this migration.
UPDATE chat_conversations
   SET pair_key = (
        SELECT MIN(user_id) || '|' || MAX(user_id)
          FROM chat_participants p
         WHERE p.conversation_id = chat_conversations.id
   )
 WHERE pair_key IS NULL;
