-- 0012 · Google identity (D-039)
--
-- Source: docs/DECISIONS.md D-039 ("Continue with Google" is a real OAuth flow,
-- server-side, and no credential ever reaches the browser).
--
-- `users.google_sub` already exists from 0001_identity.sql, and was intended for
-- exactly this. What is missing is everything that makes it operable:
--   * an audit-friendly link record, so "which provider account is this user"
--     is answerable without reading a nullable column (and future providers do
--     not need another column on `users`);
--   * the OAuth state/PKCE handshake, stored server-side so a callback that was
--     not started by this server (CSRF / code injection) is refused;
--   * one-shot e-mail verification tokens (SMTP is not wired yet, so the token is
--     what an operator hands over after a manual check).
--
-- Nothing here is destructive: every statement is additive.

CREATE TABLE IF NOT EXISTS auth_identities (
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider   TEXT    NOT NULL CHECK (provider IN ('google')),
    subject    TEXT    NOT NULL,               -- the provider's stable id (Google: `sub`)
    email      TEXT    NOT NULL DEFAULT '',    -- the address the provider asserted
    linked_at  INTEGER NOT NULL,
    PRIMARY KEY (user_id, provider)
);

-- One provider account may be linked to at most one Veylix user.
CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_identities_subject
    ON auth_identities (provider, subject);

-- A callback must present the state this server issued, and it may be used once.
CREATE TABLE IF NOT EXISTS oauth_states (
    state      TEXT    PRIMARY KEY,            -- random 32 bytes, base64url
    verifier   TEXT    NOT NULL,               -- PKCE code_verifier (S256)
    nonce      TEXT    NOT NULL,               -- echoed in the ID token
    purpose    TEXT    NOT NULL DEFAULT 'google_login',
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_oauth_states_expiry ON oauth_states (expires_at);

-- Email verification: single use, short lived, opaque.
CREATE TABLE IF NOT EXISTS email_verifications (
    token      TEXT    PRIMARY KEY,            -- random 32 bytes, base64url
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    email      TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER
);

CREATE INDEX IF NOT EXISTS idx_email_verifications_user ON email_verifications (user_id);
