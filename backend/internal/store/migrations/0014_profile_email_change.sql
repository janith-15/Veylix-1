-- 0014 · Profile, username change and e-mail change (D-041)
--
-- Source: owner 2026-10-28 — "log unama profile eke email eka hera anek siyalla
-- change karanna puluwan wenna one, username password profile pic contact number
-- (whatsapp, telegram, discord) age … admingeth ehemayi super admingeth ehemayi"
-- and "email eka change karanna one hema welawema email hara withayi".
--
-- The profile COLUMNS already exist (migration 0013). What is missing is the one
-- thing that must not be a plain PATCH: changing the ADDRESS itself. An e-mail is
-- the login identifier and the password-recovery target, so a change is:
--
--   1. requested from the signed-in account (with its current password),
--   2. confirmed by a 6-digit code sent to the NEW address,
--   3. applied, with every other session revoked and the support inbox notified.
--
-- The code lives here, hashed, single use, short lived, with an attempt counter —
-- the same shape as a password reset, and for the same reason: a leaked database
-- row must not let anybody take an account over.

CREATE TABLE IF NOT EXISTS email_change_requests (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id        INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    new_email      TEXT    NOT NULL,
    code_hash      TEXT    NOT NULL,              -- bcrypt of the 6-digit code
    attempts       INTEGER NOT NULL DEFAULT 0,
    created_at     INTEGER NOT NULL,
    expires_at     INTEGER NOT NULL,
    confirmed_at   INTEGER
);

-- One live request per account: asking again replaces the previous code, so
-- "the newest e-mail works" is always true and an old code cannot be replayed.
CREATE UNIQUE INDEX IF NOT EXISTS idx_email_change_live
    ON email_change_requests (user_id) WHERE confirmed_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_email_change_expiry ON email_change_requests (expires_at);

-- An address may only have one pending claim at a time (two accounts must not
-- race for the same new address).
CREATE UNIQUE INDEX IF NOT EXISTS idx_email_change_pending_email
    ON email_change_requests (new_email COLLATE NOCASE) WHERE confirmed_at IS NULL;
