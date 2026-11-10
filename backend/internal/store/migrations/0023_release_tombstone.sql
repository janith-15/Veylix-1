-- A release record must outlive the VPN it describes (I-33, D-087).
--
-- The defect this repairs: `vpn_releases.vpn_instance_id` was declared
-- `NOT NULL REFERENCES vpn_instances (id) ON DELETE CASCADE` (0006). Purging an
-- expired release therefore did two contradictory things in one transaction
-- (store/releases.go, RemoveReleasedVPN): it stamped `removed_at` on the release
-- to record that the VPN had been purged, and then deleted the `vpn_instances`
-- row — which the cascade used to delete the very record that had just been
-- stamped. After any purge, `vpn_releases` held ZERO rows for that VPN, so:
--   * GET /api/vpn-releases could never show a removed release to the client;
--   * `DueRemovals` and the pending index silently lost the trail;
--   * the only surviving trace was the audit_logs PurgedVPN snapshot, which is
--     an operator record, not the client's timeline.
-- A tombstone that is deleted by the event it is supposed to record is not a
-- tombstone.
--
-- The fix, mirroring `free_claims` (0006), which had this right from the start:
--   1. `vpn_instance_id` loses NOT NULL and becomes ON DELETE SET NULL. After a
--      purge there is no VPN to point at, and that is the honest value; before a
--      purge the FK still enforces that a release names a real VPN.
--   2. `panel_email` and `server_id` are denormalised onto the release row. A
--      pointer to a deleted row identifies nothing, so the record carries the
--      panel identity BY VALUE. This is the same reasoning `vpn_transfers` uses
--      for `kind` and `offer_id` (0020): the fact must still say what it means
--      after the thing it refers to is gone.
--
-- Why a whole-table rebuild: SQLite cannot ALTER a column's NOT NULL or its FK
-- action. The supported route is the documented 12-step procedure already used
-- by 0018: create the new table, copy every row, drop the old one, rename,
-- recreate each index.
--
-- What this file deliberately does NOT do:
--   * no BEGIN/COMMIT — the runner applies each file inside one transaction
--     (store/migrate.go), and a nested BEGIN aborts it (same note as 0018);
--   * no `PRAGMA foreign_keys` — it is a no-op inside a transaction. The DROP
--     cannot cascade outward: `vpn_releases` is a CHILD table and nothing
--     references it, so removing it removes nothing else;
--   * it DELETES NO ROW. Every existing release — pending, restored or already
--     removed — is copied across verbatim. That is the one non-negotiable from
--     VPN_FIX_LEDGER (a verification script destroyed live data twice);
--   * it does not touch `vpn_assignments` (ON DELETE RESTRICT, so nothing to
--     repair) nor `vpn_transfers` / `usage_snapshots` (both ON DELETE CASCADE,
--     same class of defect, deliberately left as a separate decision — recorded
--     as I-34/I-35 in IMPROVEMENT_LEDGER, not fixed here).

CREATE TABLE vpn_releases_new (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    -- NULL means "this VPN has been purged". Before the purge the FK is real,
    -- so a release can never be created against an id that does not exist.
    vpn_instance_id   INTEGER REFERENCES vpn_instances (id) ON DELETE SET NULL,
    client_user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    reason            TEXT    NOT NULL DEFAULT 'quota_exhausted',
    released_at       INTEGER NOT NULL,
    restore_deadline  INTEGER NOT NULL,
    restored_at       INTEGER,
    removed_at        INTEGER,
    panel_client_kept INTEGER NOT NULL DEFAULT 1,
    status_before     TEXT    NOT NULL DEFAULT '',
    -- Identity held by value so the tombstone still says WHICH panel client it
    -- was after vpn_instances no longer has an answer.
    panel_email       TEXT    NOT NULL DEFAULT '',
    server_id         INTEGER NOT NULL DEFAULT 0
);

-- Column order matches `releaseColumns` in store/releases.go (positional scan),
-- with the two new columns last.
INSERT INTO vpn_releases_new (
    id, vpn_instance_id, client_user_id, reason, released_at, restore_deadline,
    restored_at, removed_at, panel_client_kept, status_before,
    panel_email, server_id
)
SELECT
    r.id, r.vpn_instance_id, r.client_user_id, r.reason, r.released_at, r.restore_deadline,
    r.restored_at, r.removed_at, r.panel_client_kept, r.status_before,
    COALESCE(i.panel_email, ''),
    COALESCE(i.server_id, 0)
FROM vpn_releases r
LEFT JOIN vpn_instances i ON i.id = r.vpn_instance_id;

DROP TABLE vpn_releases;
ALTER TABLE vpn_releases_new RENAME TO vpn_releases;

-- Both indexes from 0006, recreated verbatim. The partial index is what keeps
-- `DueRemovals` cheap on a 1 vCPU / 500 MB host: a removed release must fall out
-- of it, or a purge would keep selecting the same rows it already finished.
CREATE INDEX idx_vpn_releases_pending
    ON vpn_releases (restore_deadline) WHERE removed_at IS NULL AND restored_at IS NULL;
CREATE INDEX idx_vpn_releases_vpn ON vpn_releases (vpn_instance_id);
