-- VPN transfer: an assigned VPN moves from one client account to another
-- (D-064).
--
-- The defect this closes: there was no way to move a VPN between customers.
-- `AssignPanelClient` refused a panel client Veylix already held
-- (`ErrPanelClientTaken` -> 409 `panel_client_taken`), and that refusal is
-- correct for an *assignment* — it is what stops one physical VPN becoming two
-- rows. But it also made the documented recovery path in `ErrClientHasVPNs`
-- ("the operator revokes **or reassigns** them first") impossible: the only way
-- to release a client's account for deletion was to REVOKE their VPN, which
-- destroys something they may have paid for.
--
-- A transfer is therefore a fifth kind of write, distinct from all four that
-- already exist:
--
--   * create      — a NEW panel client (outbox, panel write, rollback);
--   * assign      — a panel client nobody holds gains an owner;
--   * transfer    — a panel client that ALREADY has an owner gains a different
--                   one. Ownership moves; the panel client does not;
--   * tag         — classification only, no owner;
--   * revoke      — the VPN stops being usable.
--
-- What a transfer deliberately does NOT change: `vpn_instances.panel_email`,
-- `panel_sub_id`, `offer_id`, `expires_at`, `traffic_limit_bytes` and every
-- `panel_*` mirror column. Those describe the VPN, and the VPN does not change
-- hands physically — only the Veylix account allowed to manage it does. That is
-- also why a transfer performs NO panel call at all: 3X-UI has no concept of
-- "this client belongs to Veylix account 23", so there is nothing to write, and
-- nothing to fail halfway.
--
-- The tag is likewise untouched. `panel_client_tags.locked_at` is the promise
-- that an issued VPN cannot be re-priced (D-025.2); a change of owner is not a
-- new sale, so flipping or re-stamping the tag would retroactively rewrite what
-- the first customer paid for.


-- This file adds:
--   1. `vpn_transfers` — the durable history. Nothing is ever mutated or
--      deleted, so "who held this VPN and when" is answerable forever;
--   2. two `free_claims` columns, so a FREE claim slot can follow its VPN to
--      the new owner instead of being double-counted or silently bypassed.
--
-- It deliberately does NOT do:
--   * no BEGIN/COMMIT — the migration runner already applies each file inside
--     one transaction (store/migrate.go), and a nested BEGIN aborts it;
--   * no new `vpn_instances.status` value — an ownership change is not a state
--     change, and SQLite cannot ALTER a CHECK constraint without a full table
--     rebuild, which this feature does not need;
--   * no change to `panel_assignments` — its `(server_id, panel_email)` pair is
--     what identifies the physical panel client, and that pair is identical
--     before and after a transfer, so the existing unique indexes stay valid.

-- ---------------------------------------------------------------------------
-- 1. The transfer ledger.
-- ---------------------------------------------------------------------------
-- `from_*` and `to_*` are both NOT NULL: a transfer always has a definite
-- before and after, and a record that could not name either would be useless
-- in an audit. `kind` and `offer_id` are copied by value rather than joined at
-- read time, because an offer can later be archived or re-priced and the
-- transfer fact must still say what was moved.
CREATE TABLE IF NOT EXISTS vpn_transfers (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    vpn_instance_id        INTEGER NOT NULL REFERENCES vpn_instances (id) ON DELETE CASCADE,
    server_id              INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE CASCADE,
    panel_email            TEXT    NOT NULL,
    from_user_id           INTEGER NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    to_user_id             INTEGER NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    -- The seller who vouches for each holder. A platform issue records the
    -- issuing operator, mirroring how `vpn_assignments.seller_user_id` is
    -- filled for a direct Super-Admin assignment (D-063).
    from_seller_id         INTEGER NOT NULL DEFAULT 0,
    to_seller_id           INTEGER NOT NULL DEFAULT 0,
    offer_id               INTEGER NOT NULL DEFAULT 0,
    kind                   TEXT    NOT NULL DEFAULT 'PREMIUM',
    actor_id               INTEGER NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    reason                 TEXT    NOT NULL DEFAULT '',
    -- Set when this transfer also moved a FREE claim slot, so the ledger can
    -- answer "was a quota slot involved" without inspecting `free_claims`.
    free_claim_id          INTEGER REFERENCES free_claims (id) ON DELETE SET NULL,
    created_at             INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_vpn_transfers_vpn
    ON vpn_transfers (vpn_instance_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_vpn_transfers_from
    ON vpn_transfers (from_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_vpn_transfers_to
    ON vpn_transfers (to_user_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- 2. A FREE claim slot can move with the VPN it belongs to.
-- ---------------------------------------------------------------------------
-- `free_claims.client_user_id` is what both caps count (D-025.4/.5). Moving a
-- FREE VPN to another customer without moving its claim would produce two
-- wrong answers at once:
--   * the OLD owner's concurrent count would stay at 1 for a VPN they no longer
--     hold, so they could not claim the replacement they are entitled to;
--   * the NEW owner's count would not include the VPN they now hold, so they
--     could exceed the cap of 3 without ever seeing a refusal.
-- `transferred_at` / `transferred_from_user_id` keep that move auditable, and
-- NULL on both means "never transferred" — which is true for every existing
-- row, so no backfill is required.
ALTER TABLE free_claims ADD COLUMN transferred_at INTEGER;
ALTER TABLE free_claims ADD COLUMN transferred_from_user_id INTEGER;
