-- Allow a client account to be DELETED (a terminal state after the console's
-- "delete account" action).
--
-- Why a whole-table rebuild: SQLite cannot ALTER a CHECK constraint, and the
-- status CHECK lives inline in the column definition. The supported route is the
-- documented 12-step procedure: create the new table, copy every row, drop the
-- old one, rename, recreate each index.
--
-- Two things this file deliberately does NOT do:
--   * no BEGIN/COMMIT — the migration runner already applies each file inside one
--     transaction (store/migrate.go), and a nested BEGIN aborts it;
--   * no `PRAGMA foreign_keys` — it is a no-op inside a transaction, so the
--     runner's connection setting is what applies. The DROP cannot cascade because
--     SQLite's implicit DELETE only fires with foreign_keys ON, and the store
--     opens with them ON — so the rebuild is safe only while no child row points
--     at a deleted user, which is guaranteed because we copy every row first and
--     never remove one.
--
-- What this file must not change: any column, type, default or index. It
-- reproduces the schema exactly as migrations 0001, 0004, 0013 and 0016 left it,
-- with only the status list widened.

CREATE TABLE users_new (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    email          TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    password_hash  TEXT,                         -- NULL for OAuth-only accounts
    google_sub     TEXT    UNIQUE,               -- Google subject id
    display_name   TEXT    NOT NULL DEFAULT '',
    role           TEXT    NOT NULL CHECK (role IN ('super_admin', 'admin', 'client')),
    status         TEXT    NOT NULL DEFAULT 'ACTIVE'
                           CHECK (status IN ('ACTIVE', 'SUSPENDED', 'PENDING_VERIFICATION', 'DELETED')),
    email_verified INTEGER NOT NULL DEFAULT 0 CHECK (email_verified IN (0, 1)),
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    quota_limit    INTEGER NOT NULL DEFAULT 0,
    username       TEXT,
    avatar_url     TEXT    NOT NULL DEFAULT '',
    phone          TEXT    NOT NULL DEFAULT '',
    whatsapp       TEXT    NOT NULL DEFAULT '',
    telegram       TEXT    NOT NULL DEFAULT '',
    discord        TEXT    NOT NULL DEFAULT '',
    age            INTEGER NOT NULL DEFAULT 0,
    bio            TEXT    NOT NULL DEFAULT '',
    timezone       TEXT    NOT NULL DEFAULT '',
    free_vpn_limit INTEGER NOT NULL DEFAULT 0,   -- 0 = platform default
    last_login_at  INTEGER NOT NULL DEFAULT 0,
    last_login_ip  TEXT    NOT NULL DEFAULT '',
    birthday       TEXT    NOT NULL DEFAULT ''   -- ISO YYYY-MM-DD
);

INSERT INTO users_new (
    id, email, password_hash, google_sub, display_name, role, status,
    email_verified, created_at, updated_at, quota_limit, username, avatar_url,
    phone, whatsapp, telegram, discord, age, bio, timezone, free_vpn_limit,
    last_login_at, last_login_ip, birthday
)
SELECT
    id, email, password_hash, google_sub, display_name, role, status,
    email_verified, created_at, updated_at, quota_limit, username, avatar_url,
    phone, whatsapp, telegram, discord, age, bio, timezone, free_vpn_limit,
    last_login_at, last_login_ip, birthday
FROM users;

DROP TABLE users;
ALTER TABLE users_new RENAME TO users;

-- Every index from the previous migrations, recreated verbatim.
CREATE INDEX idx_users_role   ON users (role, status);
CREATE INDEX idx_users_google ON users (google_sub) WHERE google_sub IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username
    ON users (username) WHERE username IS NOT NULL AND username <> '';
CREATE INDEX IF NOT EXISTS idx_users_last_login ON users (last_login_at);
