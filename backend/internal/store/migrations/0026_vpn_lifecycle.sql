-- 0026_vpn_lifecycle.sql — "since when?" for every VPN status (D-098)
--
-- Why this column exists
-- ----------------------
-- The owner asked for suspended and revoked VPNs to leave the system after a
-- fixed period, so the account stays readable and a reseller cannot keep minting
-- VPNs for ever. Neither was possible: Veylix recorded WHAT a status is but never
-- WHEN it became that, so "suspended 90 days ago" was a question no column could
-- answer. `updated_at` is not a substitute — it moves on every unrelated edit.
--
-- `status_changed_at` is written on every legal transition and is the only clock
-- a lifecycle rule may count from.
--
-- What it is NOT
-- --------------
--   * NOT a deletion trigger. This migration and the code behind it only DETECT
--     and RECORD. Purge is a separate, owner-approved later step.
--   * NOT a change to any existing status. No row's `status` moves here.
--
-- The backfill, and why it is the migration time
-- ---------------------------------------------
-- Existing rows have no history to derive the value from. Using `updated_at`
-- would be an invention — a VPN edited last week would look "status-changed last
-- week" and become eligible for removal immediately on a box that has run for
-- years. Instead every pre-existing row starts its clock at the moment this
-- migration runs, so no existing VPN can be swept the day the feature ships:
-- the earliest anything becomes eligible is one full lifecycle period after the
-- upgrade. New transitions overwrite the value immediately.
--
-- Safety: additive only. Column + index, one bounded UPDATE. No row is deleted,
-- no status is rewritten, and the panel is never touched.

ALTER TABLE vpn_instances ADD COLUMN status_changed_at INTEGER NOT NULL DEFAULT 0;

-- Start every existing row's clock now, not historically. See the note above:
-- this is what stops the first tick from sweeping a backlog of old rows.
UPDATE vpn_instances SET status_changed_at = CAST(strftime('%s', 'now') AS INTEGER);

-- The lifecycle sweep asks "which rows have been in a stale state longer than
-- N?" on every hourly tick, so the predicate gets an index of its own.
CREATE INDEX IF NOT EXISTS idx_vpn_status_changed
    ON vpn_instances (status, status_changed_at);
