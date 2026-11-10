-- 0003_observability.sql — usage history, server metrics, notifications, chat, audit
--
-- Source: docs/REQUIREMENTS.md §2.6, §2.7, §5, §8; F-010 (lifetime totals)

CREATE TABLE usage_snapshots (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    vpn_instance_id       INTEGER NOT NULL REFERENCES vpn_instances (id) ON DELETE CASCADE,
    taken_at              INTEGER NOT NULL,
    up_bytes              INTEGER NOT NULL DEFAULT 0,
    down_bytes            INTEGER NOT NULL DEFAULT 0,
    lifetime_up_bytes     INTEGER NOT NULL DEFAULT 0,
    lifetime_down_bytes   INTEGER NOT NULL DEFAULT 0,
    remote_reset_count    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_usage_instance ON usage_snapshots (vpn_instance_id, taken_at);

CREATE TABLE server_metrics (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE CASCADE,
    taken_at  INTEGER NOT NULL,
    cpu_pct   REAL    NOT NULL DEFAULT 0,
    mem_used  INTEGER NOT NULL DEFAULT 0,
    mem_total INTEGER NOT NULL DEFAULT 0,
    net_up    INTEGER NOT NULL DEFAULT 0,
    net_down  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_metrics_server ON server_metrics (server_id, taken_at);

CREATE TABLE notifications (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       TEXT    NOT NULL,
    title      TEXT    NOT NULL,
    body       TEXT    NOT NULL DEFAULT '',
    read_at    INTEGER,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_notif_user ON notifications (user_id, read_at, created_at);

CREATE TABLE chat_conversations (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at INTEGER NOT NULL
);

CREATE TABLE chat_participants (
    conversation_id INTEGER NOT NULL REFERENCES chat_conversations (id) ON DELETE CASCADE,
    user_id         INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (conversation_id, user_id)
);

CREATE TABLE chat_messages (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    conversation_id INTEGER NOT NULL REFERENCES chat_conversations (id) ON DELETE CASCADE,
    sender_user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body            TEXT    NOT NULL,
    created_at      INTEGER NOT NULL
);
CREATE INDEX idx_chat_msg ON chat_messages (conversation_id, created_at);

CREATE TABLE audit_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id    INTEGER REFERENCES users (id) ON DELETE SET NULL,
    action      TEXT    NOT NULL,
    target_type TEXT    NOT NULL DEFAULT '',
    target_id   TEXT    NOT NULL DEFAULT '',
    result      TEXT    NOT NULL DEFAULT 'OK',
    context     TEXT    NOT NULL DEFAULT '',     -- non-secret context only
    created_at  INTEGER NOT NULL
);
CREATE INDEX idx_audit_actor  ON audit_logs (actor_id, created_at);
CREATE INDEX idx_audit_action ON audit_logs (action, created_at);
