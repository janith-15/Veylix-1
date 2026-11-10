-- 0025_vpn_dormant.sql — a customer's "put this VPN to sleep" flag (D-094)
--
-- Why this column exists
-- ----------------------
-- A VPN whose panel client has gone away becomes SERVER_UNAVAILABLE and, by
-- design, is never hidden and never deleted (D-091.5, INV-1). That rule is
-- right, but on its own it left a dead VPN sitting at the top of the list for
-- ever: `sort=newest` puts it first, it counts toward "Needs attention" every
-- time the page opens, and the Usage screen spends two requests polling a VPN
-- that cannot change. The customer's only way to make the noise stop was to
-- release the VPN — which is a deletion-shaped action for what is really
-- "not now".
--
-- `dormant_at` lets the customer say "not now" without deleting anything.
--
-- What it is NOT
-- --------------
--   * NOT a status. `status` keeps reporting the truth from the panel
--     (ACTIVE, SERVER_UNAVAILABLE, EXPIRED …). Dormant is a separate, LOCAL
--     preference, so a VPN cannot stop being EXHAUSTED because it was put to
--     sleep, and the panel is never lied to.
--   * NOT a panel change. No 3X-UI client is touched, disabled or removed.
--     Nothing leaves this database.
--   * NOT a release. Release (D-027) opens a 3-day deletion window; dormant
--     never deletes and has no deadline.
--
-- Why a timestamp instead of a boolean
-- ------------------------------------
-- "Asleep since 3 Nov" is what the card shows, and a future auto-archive rule
-- needs a date to count from. A boolean would have to grow a second column to
-- answer either question.
--
-- Safety: additive only. A NULL value is every existing row, which means
-- "awake" — the exact behaviour before this migration, so no data changes and
-- no backfill is needed.

ALTER TABLE vpn_instances ADD COLUMN dormant_at INTEGER;

-- The list asks "which of this owner's VPNs are awake?" on every load, so the
-- predicate gets an index alongside the owner/status one it already filters by.
CREATE INDEX IF NOT EXISTS idx_vpn_owner_dormant
    ON vpn_instances (owner_user_id, dormant_at);
