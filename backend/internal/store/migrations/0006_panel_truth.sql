-- 0006_panel_truth.sql — the panel becomes the source of truth (D-024 … D-028)
--
-- Source: docs/DECISIONS.md D-024 (VPN ≡ 3X-UI client), D-025 (tag lifecycle +
-- FREE quotas), D-027 (release + chat storage), D-028 (panel email identity).
--
-- Three things change together:
--   1. A VPN's limits are MIRRORED from the panel (panel_* columns), not derived
--      from an offer. The offer describes an intent; the panel holds the truth.
--   2. FREE/PREMIUM is an operator TAG on a panel client — the only attribute
--      Veylix supplies itself, and it locks once the VPN is issued.
--   3. A released VPN is removed locally after a grace period; the panel client
--      is never deleted by Veylix.
--
-- Nothing is dropped and no row is deleted here: this migration only adds. The
-- backfill at the bottom keeps existing rows coherent instead of guessing.

-- ---------------------------------------------------------------------------
-- FREE / PREMIUM tag on a panel client (D-024.3, D-025.1, D-025.2)
-- ---------------------------------------------------------------------------
-- Keyed by the panel identity, not by a local VPN id: the tag must survive even
-- when no VPN row exists yet (a panel client nobody has claimed).
-- `locked_at` is set the first time the client is issued; a non-NULL value
-- blocks re-tagging, because flipping FREE↔PREMIUM after a sale would rewrite
-- what the customer paid for.
CREATE TABLE IF NOT EXISTS panel_client_tags (
    server_id    INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE CASCADE,
    panel_email  TEXT    NOT NULL,
    tag          TEXT    NOT NULL CHECK (tag IN ('FREE', 'PREMIUM')),
    tagged_by    INTEGER REFERENCES users (id) ON DELETE SET NULL,
    tagged_at    INTEGER NOT NULL,
    locked_at    INTEGER,
    PRIMARY KEY (server_id, panel_email)
);
CREATE INDEX IF NOT EXISTS idx_panel_tags_tag ON panel_client_tags (tag);
CREATE INDEX IF NOT EXISTS idx_panel_tags_unlocked ON panel_client_tags (server_id, locked_at);

-- ---------------------------------------------------------------------------
-- FREE claim ledger (D-025.4, D-025.5, D-027.4)
-- ---------------------------------------------------------------------------
-- One row per FREE claim, kept even after the VPN is released, because the
-- monthly cap counts CLAIMS not live VPNs: releasing does not refund a slot.
-- Both counters derive from this table:
--   concurrent = COUNT(*) WHERE released_at IS NULL
--   monthly    = COUNT(*) WHERE claimed_at > now - 30d
CREATE TABLE IF NOT EXISTS free_claims (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    client_user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    server_id      INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE CASCADE,
    panel_email    TEXT    NOT NULL,
    vpn_instance_id INTEGER REFERENCES vpn_instances (id) ON DELETE SET NULL,
    claimed_at     INTEGER NOT NULL,
    released_at    INTEGER
);
CREATE INDEX IF NOT EXISTS idx_free_claims_client_server
    ON free_claims (client_user_id, server_id, claimed_at);
CREATE INDEX IF NOT EXISTS idx_free_claims_live
    ON free_claims (client_user_id, server_id, released_at);

-- ---------------------------------------------------------------------------
-- Release with a grace period (D-027.2, D-027.3)
-- ---------------------------------------------------------------------------
-- A client may release a quota-exhausted VPN. It stays restorable for 3 days;
-- after that Veylix deletes its own record. The panel client is NOT deleted —
-- `panel_client_kept` records that the removal was deliberately local.
CREATE TABLE IF NOT EXISTS vpn_releases (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    vpn_instance_id  INTEGER NOT NULL REFERENCES vpn_instances (id) ON DELETE CASCADE,
    client_user_id   INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    reason           TEXT    NOT NULL DEFAULT 'quota_exhausted',
    released_at      INTEGER NOT NULL,
    restore_deadline INTEGER NOT NULL,
    restored_at      INTEGER,
    removed_at       INTEGER,
    panel_client_kept INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_vpn_releases_pending
    ON vpn_releases (restore_deadline) WHERE removed_at IS NULL AND restored_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_vpn_releases_vpn ON vpn_releases (vpn_instance_id);

-- ---------------------------------------------------------------------------
-- Chat storage budget per client (D-027.6)
-- ---------------------------------------------------------------------------
-- A separate budget from the VPN quota: running out of chat space must never
-- block VPN use. The Super Admin sets the ceiling; the client reclaims space by
-- deleting their own messages, so usage is summed from chat_messages rather
-- than stored as a mutable counter (which could drift).
CREATE TABLE IF NOT EXISTS client_storage_limits (
    user_id            INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    chat_bytes_limit   INTEGER NOT NULL DEFAULT 0,   -- 0 = unlimited
    chat_message_limit INTEGER NOT NULL DEFAULT 0,   -- 0 = unlimited
    set_by             INTEGER REFERENCES users (id) ON DELETE SET NULL,
    set_at             INTEGER NOT NULL
);

-- ---------------------------------------------------------------------------
-- Panel-mirrored limits on vpn_instances (D-024.2, D-024.4)
-- ---------------------------------------------------------------------------
-- These columns hold what the PANEL reports (totalGB, expiryTime, limitIp,
-- limitHwid, enable). They are written by sync from the panel, never by a form,
-- so the panel wins every conflict. The original columns
-- (traffic_limit_bytes, expires_at, device_limit) are kept: they record what the
-- VPN was created with, which is still useful history, but they are no longer
-- what the UI trusts.
ALTER TABLE vpn_instances ADD COLUMN panel_total_gb    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE vpn_instances ADD COLUMN panel_expiry_time INTEGER NOT NULL DEFAULT 0;
ALTER TABLE vpn_instances ADD COLUMN panel_limit_ip    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE vpn_instances ADD COLUMN panel_limit_hwid  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE vpn_instances ADD COLUMN panel_enable      INTEGER NOT NULL DEFAULT 1;
ALTER TABLE vpn_instances ADD COLUMN panel_synced_at   INTEGER;

-- Backfill from the create-time values so existing rows are not suddenly shown
-- as "0 GB / no expiry". This is a deliberate one-time copy, not a new source of
-- truth: the next sync overwrites it with the panel's answer.
UPDATE vpn_instances SET
    panel_total_gb    = traffic_limit_bytes,
    panel_expiry_time = COALESCE(expires_at, 0) * 1000,   -- seconds → millis
    panel_limit_ip    = device_limit,
    panel_limit_hwid  = device_limit;

