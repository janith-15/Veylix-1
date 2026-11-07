-- An assignment keeps the panel's own name, and is always PREMIUM (D-063).
--
-- The defect this closes: `AssignPanelClient` recorded the VPN row under an
-- INVENTED name (`<client email>_paid_<slot>`) while the panel still held the
-- original name, and then tried to rename the panel client afterwards. When the
-- rename failed the same physical VPN existed twice — under the old name on the
-- panel (so the console still offered "Assign") and under the new name locally
-- (so `panel_assignments.source_email` matched nothing the console could click).
-- A second Assign then created a SECOND row for ONE panel client, with its own
-- owner and its own locked tag. One VPN, two rows, two edits.
--
-- The rule from here on: the name the panel holds IS the name the row holds.
-- A rename is optional, and a failed rename keeps the existing name as a final,
-- correct state rather than leaving a half-applied one.
--
-- What this migration does:
--   1. rescues the rows that are already split, so an existing database stops
--      showing duplicate VPNs (a twin is tombstoned, never deleted);
--   2. makes "one panel client, one row" a database fact;
--   3. gives every assigned row a real PREMIUM package and tag, which is what
--      made an assigned VPN read as UNTAGGED on the client screen.
--
-- What this file deliberately does NOT do:

-- ---------------------------------------------------------------------------
-- 1. Rescue assigned rows that a failed rename left split.
-- ---------------------------------------------------------------------------
-- `panel_assignments` is the identity ledger: it remembers the name the panel
-- client had when it was assigned (`source_email`) and the name Veylix recorded
-- it under (`panel_email`). When the rename never landed those two disagree and
-- the local row is unreachable from the panel's name, so the console offers
-- "Assign" for a client somebody already holds.
--
-- Guarded exactly as the code guards it: only when the panel's real name is
-- free on that server. If another row already sits under it, this is not a
-- failed rename but two genuinely different VPNs, and merging them would fuse
-- two customers' records. That case is left to the explicit reconcile route,
-- which a human runs and which reports what it merged.
UPDATE vpn_instances
   SET panel_email = (
         SELECT pa.source_email
           FROM panel_assignments pa
          WHERE pa.vpn_instance_id = vpn_instances.id
            AND pa.server_id = vpn_instances.server_id
            AND pa.source_email <> vpn_instances.panel_email
       ),
       updated_at = CAST(strftime('%s', 'now') AS INTEGER)
 WHERE status = 'ACTIVE'
   AND EXISTS (
         SELECT 1
           FROM panel_assignments pa
          WHERE pa.vpn_instance_id = vpn_instances.id
            AND pa.server_id = vpn_instances.server_id
            AND pa.source_email <> vpn_instances.panel_email
       )
   AND NOT EXISTS (
         SELECT 1 FROM vpn_instances other
          WHERE other.server_id = vpn_instances.server_id
            AND other.id <> vpn_instances.id
            AND other.panel_email = (
                  SELECT pa.source_email
                    FROM panel_assignments pa
                   WHERE pa.vpn_instance_id = vpn_instances.id
                     AND pa.server_id = vpn_instances.server_id
                )
       );

-- A duplicate is a row for a panel client that assignment already gave to
-- somebody else. Keep the earliest (the one the customer was told about and the
-- one the tag was locked for) and mark the rest REVOKED so they stop counting as
-- a live VPN while remaining in the history.
UPDATE vpn_instances
   SET status = 'REVOKED',
       updated_at = CAST(strftime('%s', 'now') AS INTEGER)
 WHERE status = 'ACTIVE'
   AND EXISTS (
         SELECT 1 FROM vpn_instances keeper
          WHERE keeper.server_id = vpn_instances.server_id
            AND keeper.panel_email = vpn_instances.panel_email
            AND keeper.id < vpn_instances.id
       );

--   * no BEGIN/COMMIT — the migration runner already applies each file inside
--     one transaction (store/migrate.go), and a nested BEGIN aborts it;
--   * no ALTER of a CHECK constraint — SQLite cannot, and none needs to change;
--   * no row is ever deleted. A duplicate is marked REVOKED so the audit trail
--     survives and nothing a customer paid for disappears silently.

-- ---------------------------------------------------------------------------
-- 2. One panel client, one row — as a database fact.
-- ---------------------------------------------------------------------------
-- `panel_assignments` already carried UNIQUE (server_id, source_email), but
-- nothing stopped a second `vpn_instances` row appearing for the same panel
-- client — which is exactly how the split produced two. `vpn_instances` already
-- has UNIQUE (server_id, panel_email); this adds the missing half, on the
-- reservation itself, so the ledger is a single statement of who holds what.
CREATE UNIQUE INDEX IF NOT EXISTS ux_panel_assignments_target
    ON panel_assignments (server_id, panel_email);



-- ---------------------------------------------------------------------------
-- 3. An assigned VPN always has a package, and it is always PREMIUM.
-- ---------------------------------------------------------------------------
-- `vpn_offers.kind` was the only field the client projection read to decide
-- Free vs Premium, so an assignment created without a package had NO kind at all
-- and rendered as UNTAGGED — the state reserved for a panel client nobody holds.
-- The platform's own PREMIUM package is created on demand so the row has
-- something real to point at instead of a NULL.
--
-- `active = 0` on purpose: this is an internal carrier, not something a client
-- may buy. It never appears in the catalogue or on the pricing screen.
INSERT INTO vpn_offers (kind, name, price_cents, currency, active, created_at, updated_at)
SELECT 'PREMIUM',
       'Platform assignment',
       0,
       COALESCE((SELECT currency FROM vpn_offers WHERE currency <> '' ORDER BY id LIMIT 1), 'USD'),
       0,
       CAST(strftime('%s', 'now') AS INTEGER),
       CAST(strftime('%s', 'now') AS INTEGER)
 WHERE EXISTS (SELECT 1 FROM vpn_instances WHERE offer_id IS NULL)
   AND NOT EXISTS (
         SELECT 1 FROM vpn_offers
          WHERE kind = 'PREMIUM' AND name = 'Platform assignment'
       );

UPDATE vpn_instances
   SET offer_id = (
         SELECT id FROM vpn_offers
          WHERE kind = 'PREMIUM' AND name = 'Platform assignment'
          ORDER BY id LIMIT 1
       ),
       updated_at = CAST(strftime('%s', 'now') AS INTEGER)
 WHERE offer_id IS NULL;

-- The tag is the classification the operator sees, and the projection now
-- honours it, so an owned row that is still unlabelled gets the PREMIUM label it
-- was assigned as. An existing (possibly locked) tag is never overwritten:
-- `locked_at` is the promise that an issued VPN cannot be re-priced.
INSERT INTO panel_client_tags (server_id, panel_email, tag, tagged_at, locked_at)
SELECT i.server_id,
       i.panel_email,
       'PREMIUM',
       CAST(strftime('%s', 'now') AS INTEGER),
       CAST(strftime('%s', 'now') AS INTEGER)
  FROM vpn_instances i
 WHERE i.status <> 'REVOKED'
   AND NOT EXISTS (
         SELECT 1 FROM panel_client_tags t
          WHERE t.server_id = i.server_id AND t.panel_email = i.panel_email
       );
