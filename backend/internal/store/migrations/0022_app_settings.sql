-- Operator-editable settings that used to be environment-only (D-080).
--
-- The Google OAuth client is the first row: an operator could previously change
-- it only by editing /etc/veylix/veylix.env and restarting the process. Storing
-- it here lets a Super Admin edit it in the UI and have it applied live, with the
-- secret sealed exactly as a panel token already is (AES-256-GCM, D-016).
--
-- Invariants this table keeps:
--   * a secret is NEVER stored in plaintext: `is_secret = 1` means
--     `value_ciphertext` is a sealed blob; `is_secret = 0` means it is the raw
--     UTF-8 value (a client id and a redirect url are not secrets);
--   * the ciphertext is bound to its key (AAD), so a row copied onto another key
--     fails to open rather than decrypting into the wrong place;
--   * `updated_by` records which account last changed it, for the audit trail.

CREATE TABLE app_settings (
    key              TEXT PRIMARY KEY,
    value_ciphertext BLOB    NOT NULL,
    is_secret        INTEGER NOT NULL DEFAULT 0,
    updated_at       INTEGER NOT NULL,
    updated_by       INTEGER
);