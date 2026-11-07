-- 0013 · Login identity and self-service profile (D-041)
--
-- Source: owner 2026-10-28 — "login wena thenadi admin portal ekatada client portal
-- ekatada kiyala adahha … log unama profile eke email eka hera anek siyalla change
-- karanna puluwan wenna one".
--
-- What this adds, and why each part exists:
--   * `username` — a login identifier that is not an e-mail address, so a client can
--     have a short name and an admin can be reached without publishing an address.
--     Unique, case-insensitive, and optional (older rows keep logging in by e-mail).
--   * Profile fields — avatar, contacts (phone/WhatsApp/Telegram/Discord), age, bio,
--     timezone. The API lets a user edit these; `email` is deliberately NOT one of
--     them (an address change goes through a verified e-mail flow).
--   * `free_vpn_limit` — a per-client override of the platform FREE cap, so a Super
--     Admin can raise or lower one customer without changing the policy for everyone.
--   * `last_login_at` — "when did this account last sign in" on the management pages.
--
-- Everything is additive: no existing column changes type, and no row is rewritten.

ALTER TABLE users ADD COLUMN username        TEXT;
ALTER TABLE users ADD COLUMN avatar_url      TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN phone           TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN whatsapp        TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN telegram        TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN discord         TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN age             INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN bio             TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN timezone        TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN free_vpn_limit  INTEGER NOT NULL DEFAULT 0;  -- 0 = platform default
ALTER TABLE users ADD COLUMN last_login_at   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN last_login_ip   TEXT    NOT NULL DEFAULT '';

-- A username is a login identifier: unique and case-insensitive. NULL is allowed
-- (an account that has not chosen one yet), and SQLite treats NULLs as distinct.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username
    ON users (username COLLATE NOCASE) WHERE username IS NOT NULL AND username <> '';

-- Backfill from the e-mail prefix so every existing account can log in by name
-- immediately. A collision gets a numeric suffix (the index above would otherwise
-- abort the migration), and nothing is overwritten.
UPDATE users
   SET username = (
         SELECT CASE
                  WHEN (SELECT COUNT(*) FROM users u2
                         WHERE u2.id <> users.id
                           AND substr(lower(u2.email), 1, instr(lower(u2.email), '@') - 1)
                             = substr(lower(users.email), 1, instr(lower(users.email), '@') - 1)) > 0
                  THEN substr(lower(users.email), 1, instr(lower(users.email), '@') - 1) || users.id
                  ELSE substr(lower(users.email), 1, instr(lower(users.email), '@') - 1)
                END
       )
 WHERE (username IS NULL OR username = '')
   AND instr(email, '@') > 1;

-- The login lookup hits this column on every sign-in.
CREATE INDEX IF NOT EXISTS idx_users_last_login ON users (last_login_at);
