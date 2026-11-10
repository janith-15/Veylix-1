-- 0011_order_events.sql — payment channels and the order timeline
--
-- Source: docs/DECISIONS.md D-036 (order is manual, channel-paid, blockable and
--         always attributable).
-- Owner, 2026-10-25: "eyata ethana gmail eka whatsapp telegram number tika
--         pennanna one super adminge … client ta payment methord eka maru
--         karannath puluwan wenna one … maru kaloth maru karana methord eka
--         ara super adminge hari adminge hari order eke pennanna one …
--         payment aulak thibboth VPN eka block karayine, aula wisa dunama aye
--         unblock karawe."
--
-- Nothing is dropped. Every column is additive and nullable/defaulted.

-- ---------------------------------------------------------------------------
-- Who receives the money, and how the payer reaches them (D-036.2)
-- ---------------------------------------------------------------------------
-- A method already had free-text `details`; these are the structured channels
-- the owner named. They are stored separately so a screen can offer "copy",
-- "open WhatsApp" and "open Telegram" without parsing prose.
ALTER TABLE payment_methods ADD COLUMN contact_email    TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_methods ADD COLUMN contact_whatsapp TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_methods ADD COLUMN contact_telegram TEXT NOT NULL DEFAULT '';

-- ---------------------------------------------------------------------------
-- The order's immutable timeline (D-036.1, D-036.4, D-036.5)
-- ---------------------------------------------------------------------------
-- Every decision about an order is appended, never rewritten:
--   CREATED          the payer opened the order
--   METHOD_CHANGED   the payer moved it to a different destination
--   PROOF_CHANNEL    the payer said where the proof was sent (WhatsApp/TG/e-mail)
--   BLOCKED          the owner stopped the VPN because money did not arrive
--   UNBLOCKED        the issue was resolved
--   CONFIRMED        the owner settled it (and the VPN was built)
--   REJECTED         the owner refused it
--
-- "Order eka demme kauda, eyaa payment enne kohenda kagenda" must be answerable
-- from this table alone, without reading the current row — because the current
-- row only holds the LATEST method, and the question is about history.
CREATE TABLE IF NOT EXISTS payment_record_events (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    payment_id     INTEGER NOT NULL REFERENCES payment_records (id) ON DELETE CASCADE,
    kind           TEXT    NOT NULL CHECK (kind IN (
                     'CREATED', 'METHOD_CHANGED', 'PROOF_CHANNEL',
                     'BLOCKED', 'UNBLOCKED', 'CONFIRMED', 'REJECTED',
                     'RELEASE_TARGET')),
    -- The destination involved: the new one for METHOD_CHANGED, the old one in
    -- `from_method_id` so the pair reads as a move rather than a replacement.
    from_method_id INTEGER REFERENCES payment_methods (id) ON DELETE SET NULL,
    to_method_id   INTEGER REFERENCES payment_methods (id) ON DELETE SET NULL,
    -- PROOF_CHANNEL records 'WHATSAPP' | 'TELEGRAM' | 'EMAIL'.
    channel        TEXT    NOT NULL DEFAULT '',
    -- The operator or payer who caused the event; NULL for a system event.
    actor_id       INTEGER REFERENCES users (id) ON DELETE SET NULL,
    note           TEXT    NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pay_events_order ON payment_record_events (payment_id, created_at, id);

-- ---------------------------------------------------------------------------
-- Whether proof of payment was reported (D-036.3)
-- ---------------------------------------------------------------------------
-- The owner chose channels over uploads: the payer sends the slip over
-- WhatsApp/Telegram/e-mail and records that they did, so the operator knows
-- what to look for and where.
ALTER TABLE payment_records ADD COLUMN proof_channel TEXT NOT NULL DEFAULT '';
ALTER TABLE payment_records ADD COLUMN proof_note    TEXT NOT NULL DEFAULT '';
