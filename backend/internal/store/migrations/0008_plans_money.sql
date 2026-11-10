-- 0008_plans_money.sql — plan ownership, price, slots, and order→VPN linkage
--
-- Source: docs/DECISIONS.md D-030 (plan ownership), D-031 (payment = release),
--         D-032 (Free/Premium separation), D-033 (QR).
--
-- Why a plan row and not just a payment: a payment says money arrived; a plan
-- row says what may be built. Separating them is what lets one confirmed
-- payment buy N slots, and what lets the panel limits be fixed before money is
-- involved.
--
-- Nothing is dropped here. Every new column is nullable or defaulted, and the
-- backfill at the bottom keeps existing rows coherent instead of guessing.

-- ---------------------------------------------------------------------------
-- vpn_offers: ownership, price, slot inventory, archive (D-030)
-- ---------------------------------------------------------------------------
-- `owner_user_id` answers "whose plan is this?":
--   NULL       -> unowned inventory: only the Super Admin may release it
--                 (D-030.3 `403 plan_not_approved` without a confirmation).
--   a user id  -> that reseller's plan: he releases it with no approval step
--                 (D-030.2), on a server assigned to him.
ALTER TABLE vpn_offers ADD COLUMN owner_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL;
-- Money is integer cents; 0 keeps its old meaning (no price recorded), so every
-- pre-existing row stays valid and readable.
ALTER TABLE vpn_offers ADD COLUMN price_cents   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE vpn_offers ADD COLUMN currency      TEXT    NOT NULL DEFAULT 'LKR';
-- Slot inventory (D-030.4). 0 total = unlimited, which is what every existing
-- FREE offer and every pre-migration PREMIUM row must keep meaning.
ALTER TABLE vpn_offers ADD COLUMN slots_total   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE vpn_offers ADD COLUMN slots_used    INTEGER NOT NULL DEFAULT 0;
-- Archive instead of delete (D-030.6). Non-NULL hides the plan from every
-- catalogue while keeping the history that references it.
ALTER TABLE vpn_offers ADD COLUMN archived_at   INTEGER;

CREATE INDEX idx_offer_owner  ON vpn_offers (owner_user_id, kind, active);
CREATE INDEX idx_offer_public ON vpn_offers (kind, active, archived_at);

-- ---------------------------------------------------------------------------
-- payment_records: what an order is for, and what it produced (D-031)
-- ---------------------------------------------------------------------------
-- `offer_id` is the plan being bought. NULL keeps every pre-existing manual
-- record valid (a deposit, or a sale recorded before plans had prices).
ALTER TABLE payment_records ADD COLUMN offer_id       INTEGER REFERENCES vpn_offers (id) ON DELETE SET NULL;
-- The account the order is FOR. For a self-service order this equals
-- payer_user_id; for a reseller buying for a customer, or the Super Admin
-- buying for someone, it differs.
ALTER TABLE payment_records ADD COLUMN target_user_id INTEGER REFERENCES users (id) ON DELETE SET NULL;
-- What the confirmation produced. Both are NULL until the payment is
-- confirmed, and both stay NULL if provisioning failed (D-031.3).
ALTER TABLE payment_records ADD COLUMN vpn_instance_id INTEGER REFERENCES vpn_instances (id) ON DELETE SET NULL;
-- Quantity, so one order can buy N slots of a plan (D-030.5).
ALTER TABLE payment_records ADD COLUMN quantity        INTEGER NOT NULL DEFAULT 1;

CREATE INDEX idx_pay_offer ON payment_records (offer_id, status);

-- ---------------------------------------------------------------------------
-- vpn_plan_purchases: which slots belong to which reseller (D-030.5)
-- ---------------------------------------------------------------------------
-- A slot row is created the moment the Super Admin confirms a purchase, so
-- "how many slots do I have left" is a COUNT over real rows rather than a
-- number kept in two places. `consumed_vpn_id` records which VPN used the
-- slot, which makes an audit answerable and a reversal possible.
CREATE TABLE IF NOT EXISTS vpn_plan_purchases (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    offer_id        INTEGER NOT NULL REFERENCES vpn_offers (id) ON DELETE RESTRICT,
    buyer_user_id   INTEGER NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    payment_id      INTEGER REFERENCES payment_records (id) ON DELETE SET NULL,
    quantity        INTEGER NOT NULL DEFAULT 1,
    consumed_vpn_id INTEGER REFERENCES vpn_instances (id) ON DELETE SET NULL,
    created_at      INTEGER NOT NULL
);
CREATE INDEX idx_plan_purchase_buyer ON vpn_plan_purchases (buyer_user_id, offer_id, consumed_vpn_id);

-- ---------------------------------------------------------------------------
-- Backfill (never guesses: it only states what the old rows already implied)
-- ---------------------------------------------------------------------------
-- An existing PREMIUM offer was created by the Super Admin and has no reseller
-- owner, so it is unowned inventory — exactly D-030.3's default.
UPDATE vpn_offers SET owner_user_id = NULL WHERE owner_user_id IS NULL;

-- Revenue already recorded stays attached to the payer as its own target, so
-- the confirmation centre can list old orders beside new ones without a NULL.
UPDATE payment_records
   SET target_user_id = payer_user_id
 WHERE target_user_id IS NULL;
