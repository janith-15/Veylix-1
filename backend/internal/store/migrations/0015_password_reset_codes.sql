-- 0015 · Password reset by e-mail CODE (D-041)
--
-- Source: owner 2026-10-29 — "super adminge password eka change karaddi thama me
-- mail ekata enna one athi app wala password ee ee client lata adminlata adala
-- email gmail dila ewata code yawala rest karaganna puluwan widihata hadanna".
--
-- A reset is a 6-DIGIT CODE, not a link:
--   * a code cannot be guessed by a mail scanner that "clicks" every link,
--   * it works when the owner is on a different device than the browser,
--   * and a link would have to encode a bearer token in a URL that lands in logs.
--
-- The same three rules as an e-mail change (migration 0014), for the same reason:
-- the row is hashed, single use, short lived, and guess-limited, so a database
-- dump does not hand out account takeovers.
--
-- One live row per account: requesting a new code replaces the previous one, so
-- "the newest code works" is always true and an old code cannot be replayed.

CREATE TABLE IF NOT EXISTS password_reset_codes (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash    TEXT    NOT NULL,              -- bcrypt of the 6-digit code
    attempts     INTEGER NOT NULL DEFAULT 0,
    created_at   INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL,
    used_at      INTEGER
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_password_reset_live
    ON password_reset_codes (user_id) WHERE used_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_password_reset_expiry ON password_reset_codes (expires_at);
