-- 0001_identity.sql — identity, sessions, panel integrations
--
-- Conventions:
--   * local integer primary keys; remote identities live in explicit columns
--   * timestamps are unix seconds (INTEGER)
--   * secrets are stored as ciphertext only — never plaintext
--
-- Source: docs/REQUIREMENTS.md §5, docs/3XUI_API_CONTRACT.md, D-011

CREATE TABLE users (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    email          TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    password_hash  TEXT,                         -- NULL for OAuth-only accounts
    google_sub     TEXT    UNIQUE,               -- Google subject id
    display_name   TEXT    NOT NULL DEFAULT '',
    role           TEXT    NOT NULL CHECK (role IN ('super_admin', 'admin', 'client')),
    status         TEXT    NOT NULL DEFAULT 'ACTIVE'
                           CHECK (status IN ('ACTIVE', 'SUSPENDED', 'PENDING_VERIFICATION')),
    email_verified INTEGER NOT NULL DEFAULT 0 CHECK (email_verified IN (0, 1)),
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
);
CREATE INDEX idx_users_role   ON users (role, status);
CREATE INDEX idx_users_google ON users (google_sub) WHERE google_sub IS NOT NULL;

CREATE TABLE sessions (
    id         TEXT    PRIMARY KEY,              -- opaque id; the cookie holds it, DB stores a hash
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    ip         TEXT    NOT NULL DEFAULT '',
    user_agent TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_sessions_user ON sessions (user_id, expires_at);

CREATE TABLE server_integrations (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    name                  TEXT    NOT NULL,
    panel_url             TEXT    NOT NULL UNIQUE,
    token_ciphertext      BLOB    NOT NULL,      -- AES-256-GCM (D-011); never returned
    status                TEXT    NOT NULL DEFAULT 'PENDING_VERIFICATION'
                          CHECK (status IN ('PENDING_VERIFICATION', 'ACTIVE', 'REAUTH_REQUIRED',
                                            'CONNECTION_UNAVAILABLE', 'DISABLED', 'REMOVED')),
    panel_version         TEXT    NOT NULL DEFAULT '',
    xray_version          TEXT    NOT NULL DEFAULT '',
    capabilities_json     TEXT    NOT NULL DEFAULT '{}',
    allow_private_network INTEGER NOT NULL DEFAULT 0 CHECK (allow_private_network IN (0, 1)),
    last_health_check     INTEGER,
    last_sync             INTEGER,
    created_by            INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at            INTEGER NOT NULL,
    updated_at            INTEGER NOT NULL
);
CREATE INDEX idx_servers_status ON server_integrations (status, name);

CREATE TABLE inbounds (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id  INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE CASCADE,
    remote_id  INTEGER NOT NULL,                 -- 3X-UI inbound id
    remark     TEXT    NOT NULL DEFAULT '',
    tag        TEXT    NOT NULL DEFAULT '',
    protocol   TEXT    NOT NULL DEFAULT '',
    port       INTEGER NOT NULL DEFAULT 0,
    enabled    INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    synced_at  INTEGER NOT NULL,
    UNIQUE (server_id, remote_id)
);

CREATE TABLE admin_server_assignments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    admin_id   INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    server_id  INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    UNIQUE (admin_id, server_id)
);
