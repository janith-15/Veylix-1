-- Collapse the duplicate "Platform assignment" carriers into one (D-075).
--
-- The defect this repairs: `resolveAssignmentOffer` looked the carrier up with
-- `active = 1`, but the carrier is deliberately created with `active = 0` (it is
-- not a product — it is the row that says "this issue is premium", D-063). The
-- lookup therefore never matched, and every assignment INSERTed a fresh carrier.
-- A live installation ended up with four identical rows (two of them differing
-- only in currency, because the currency was also hard-coded to 'LKR').
--
-- What this file does NOT do:
--   * it deletes nothing. `vpn_instances.offer_id` and `payment_records.offer_id`
--     point at these rows, so the KEEPER is the oldest one (the row existing VPNs
--     already reference) and the rest are archived, which removes them from every
--     catalogue read (`archived_at IS NULL`) while leaving the history intact;
--   * it does not touch a REAL package. Only rows named exactly
--     'Platform assignment' are collapsed, so an operator's own "Premium 100"
--     catalogue row is never a candidate;
--   * it does not renumber anything.
--
-- The code fix (store/assignment.go: look the carrier up BY NAME) is what stops
-- new duplicates; this migration only cleans up what the defect already produced.

-- 1. Re-point any orphaned VPN at the keeper.
--    A row whose offer was one of the duplicates moves to the oldest carrier, so
--    the "an owned VPN always has a package" invariant (D-063) keeps holding for
--    every row rather than only for the ones that happened to use the keeper.
UPDATE vpn_instances
   SET offer_id = (
         SELECT MIN(id) FROM vpn_offers
          WHERE kind = 'PREMIUM' AND name = 'Platform assignment'
       ),
       updated_at = CAST(strftime('%s', 'now') AS INTEGER)
 WHERE offer_id IN (
         SELECT id FROM vpn_offers
          WHERE kind = 'PREMIUM' AND name = 'Platform assignment'
            AND id <> (SELECT MIN(id) FROM vpn_offers
                        WHERE kind = 'PREMIUM' AND name = 'Platform assignment')
       );

-- 2. Archive every duplicate except the oldest.
UPDATE vpn_offers
   SET archived_at = CAST(strftime('%s', 'now') AS INTEGER)
 WHERE kind = 'PREMIUM'
   AND name = 'Platform assignment'
   AND archived_at IS NULL
   AND id <> (SELECT MIN(id) FROM vpn_offers
               WHERE kind = 'PREMIUM' AND name = 'Platform assignment');

-- 3. Repair the hard-coded currency on the surviving carrier.
--    'LKR' was written regardless of what the platform actually bills in, so a USD
--    installation carried an LKR label on every assignment it ever made. It is
--    corrected from the catalogue the operator really uses, and left alone when
--    there is no other row to learn from.
UPDATE vpn_offers
   SET currency = (SELECT currency FROM vpn_offers
                    WHERE currency <> '' AND name <> 'Platform assignment'
                    ORDER BY id LIMIT 1)
 WHERE name = 'Platform assignment'
   AND archived_at IS NULL
   AND currency = 'LKR'
   AND EXISTS (SELECT 1 FROM vpn_offers
                WHERE currency <> '' AND currency <> 'LKR'
                  AND name <> 'Platform assignment');
