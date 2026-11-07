-- A released VPN gets its own state (D-027).
--
-- The defect this closes: releasing a VPN wrote a row to `vpn_releases` and left
-- `vpn_instances.status` untouched. The panel client is disabled at release, so
-- the next sync pass saw a VPN missing from the panel, could not find it, and
-- marked it SERVER_UNAVAILABLE. For the whole 3-day undo window the customer was
-- told the SERVER had failed, when in fact they had released the VPN themselves
-- and could still restore it. "The server broke" and "you gave this up" are not
-- the same message, and the operator reading the console needs to tell them apart.
--
-- Why a whole-table rebuild: SQLite cannot ALTER a CHECK constraint, and the
-- status CHECK lives inline in the column definition. The supported route is the
-- documented 12-step procedure: create the new table, copy every row, drop the
-- old one, rename, recreate each index.
--
-- What this file deliberately does NOT do:
--   * no BEGIN/COMMIT — the migration runner already applies each file inside one
--     transaction (store/migrate.go), and a nested BEGIN aborts it;
--   * no `PRAGMA foreign_keys` — it is a no-op inside a transaction, so the
--     runner's connection setting is what applies. The DROP cannot cascade because
--     we copy every row first and never remove one.
--
-- What this file must not change: any column, type, default, constraint or index.
-- It reproduces the schema exactly as migrations 0002, 0005 and 0006 left it,
-- with only the status list widened by one value.

CREATE TABLE vpn_instances_new (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_user_id           INTEGER NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    server_id               INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE RESTRICT,
    inbound_id              INTEGER REFERENCES inbounds (id) ON DELETE SET NULL,
    offer_id                INTEGER REFERENCES vpn_offers (id) ON DELETE SET NULL,
    panel_email             TEXT    NOT NULL,
    panel_sub_id            TEXT    NOT NULL,
    status                  TEXT    NOT NULL DEFAULT 'PENDING'
                            CHECK (status IN ('PENDING', 'ACTIVE', 'TRAFFIC_DEPLETED', 'EXPIRED',
                                              'SUSPENDED', 'SERVER_UNAVAILABLE', 'REVOCATION_PENDING',
                                              'REVOKED', 'REMOVED_FROM_CATALOG', 'RELEASED')),
    traffic_limit_bytes     INTEGER NOT NULL DEFAULT 0,
    expires_at              INTEGER,
    device_limit            INTEGER NOT NULL DEFAULT 0,
    lifetime_up_bytes       INTEGER NOT NULL DEFAULT 0,
    lifetime_down_bytes     INTEGER NOT NULL DEFAULT 0,
    last_remote_reset_count INTEGER NOT NULL DEFAULT 0,
    created_at              INTEGER NOT NULL,
    updated_at              INTEGER NOT NULL,
    panel_total_gb          INTEGER NOT NULL DEFAULT 0,
    panel_expiry_time       INTEGER NOT NULL DEFAULT 0,
    panel_limit_ip          INTEGER NOT NULL DEFAULT 0,
    panel_limit_hwid        INTEGER NOT NULL DEFAULT 0,
    panel_enable            INTEGER NOT NULL DEFAULT 1,
    panel_synced_at         INTEGER,
    UNIQUE (server_id, panel_email)
);

INSERT INTO vpn_instances_new (
    id, owner_user_id, server_id, inbound_id, offer_id, panel_email, panel_sub_id,
    status, traffic_limit_bytes, expires_at, device_limit,
    lifetime_up_bytes, lifetime_down_bytes, last_remote_reset_count,
    created_at, updated_at, panel_total_gb, panel_expiry_time, panel_limit_ip,
    panel_limit_hwid, panel_enable, panel_synced_at
)
SELECT
    id, owner_user_id, server_id, inbound_id, offer_id, panel_email, panel_sub_id,
    status, traffic_limit_bytes, expires_at, device_limit,
    lifetime_up_bytes, lifetime_down_bytes, last_remote_reset_count,
    created_at, updated_at, panel_total_gb, panel_expiry_time, panel_limit_ip,
    panel_limit_hwid, panel_enable, panel_synced_at
FROM vpn_instances;

DROP TABLE vpn_instances;
ALTER TABLE vpn_instances_new RENAME TO vpn_instances;

-- Every index from the previous migrations, recreated verbatim.
CREATE INDEX idx_vpn_owner  ON vpn_instances (owner_user_id, status);
CREATE INDEX idx_vpn_server ON vpn_instances (server_id, status);

-- A VPN already in its undo window is RELEASED from now on. A release that was
-- restored keeps whatever state it had; one already removed has no row left.
-- This cannot use `vpn_releases.vpn_instance_id` for rows that were already
-- purged, and it must not invent a status for a VPN nobody released: the
-- subquery is the whole guard.
--
-- Order matters: `status_before` is populated from the CURRENT status first,
-- while it still holds the real state, and only then is that status replaced by
-- RELEASED. Doing it the other way round would record "RELEASED" as the reason.
ALTER TABLE vpn_releases ADD COLUMN status_before TEXT NOT NULL DEFAULT '';

UPDATE vpn_releases
   SET status_before = (
         SELECT v.status FROM vpn_instances v WHERE v.id = vpn_releases.vpn_instance_id
       )
 WHERE restored_at IS NULL AND removed_at IS NULL;

UPDATE vpn_instances
   SET status = 'RELEASED'
 WHERE id IN (
        SELECT vpn_instance_id FROM vpn_releases
         WHERE restored_at IS NULL AND removed_at IS NULL
       );

