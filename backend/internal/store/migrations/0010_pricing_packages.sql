-- 0010_pricing_packages.sql — unit rates and the CUSTOM package kind
--
-- Source: docs/DECISIONS.md D-035 (three package kinds, D-035.4 unit pricing),
--         owner 2026-10-25: "super adminge peththen 1gb ekaka price ekaka,
--         device 1 ka price eka, ip 1 ka price eka set karanna puluwan wenna one".
--
-- Why rates rather than prices on every plan: a CUSTOM order is priced from a
-- spec the CUSTOMER chooses (traffic, days, devices, IPs), so the platform needs
-- a unit price per dimension. A PREMIUM package keeps its own fixed price.
--
-- Nothing is dropped. A row with `server_id IS NULL` is the platform default; a
-- row with a server id overrides it for that one panel (owner's Q1 answer:
-- "global default + per-server override").

-- ---------------------------------------------------------------------------
-- Unit rates
-- ---------------------------------------------------------------------------
-- Money is integer cents and every rate is per ONE unit, so the quote is a sum
-- of exact integers and never a float (`new_update.md` §6).
CREATE TABLE IF NOT EXISTS pricing_rates (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    -- NULL = the platform default; a value = that server's override.
    server_id             INTEGER REFERENCES server_integrations (id) ON DELETE CASCADE,
    currency              TEXT    NOT NULL DEFAULT 'LKR',
    rate_per_gb_cents     INTEGER NOT NULL DEFAULT 0,
    rate_per_day_cents    INTEGER NOT NULL DEFAULT 0,
    rate_per_device_cents INTEGER NOT NULL DEFAULT 0,
    rate_per_ip_cents     INTEGER NOT NULL DEFAULT 0,
    -- A floor and a ceiling, so a spec can never be priced at zero by accident
    -- and a typo cannot produce a five-figure order.
    min_price_cents       INTEGER NOT NULL DEFAULT 0,
    max_price_cents       INTEGER NOT NULL DEFAULT 0,   -- 0 = no ceiling
    -- Allowed spec ranges. 0 = no bound on that dimension.
    max_traffic_gb        INTEGER NOT NULL DEFAULT 0,
    max_duration_days     INTEGER NOT NULL DEFAULT 0,
    max_devices           INTEGER NOT NULL DEFAULT 0,
    max_ips               INTEGER NOT NULL DEFAULT 0,
    updated_by            INTEGER REFERENCES users (id) ON DELETE SET NULL,
    updated_at            INTEGER NOT NULL
);
-- One row per scope: the platform default, or one override per server.
CREATE UNIQUE INDEX IF NOT EXISTS idx_pricing_server ON pricing_rates (COALESCE(server_id, 0));

-- ---------------------------------------------------------------------------
-- CUSTOM as a package kind (D-035)
-- ---------------------------------------------------------------------------
-- The offer table's CHECK constraint allows FREE and PREMIUM only. A CUSTOM
-- package is not a catalogue row with fixed limits — it is a *template* saying
-- "the customer states the spec". It is represented as a PREMIUM row with
-- `is_custom = 1`, so every existing query keeps working and the row still
-- carries the server/inbound a release needs.
ALTER TABLE vpn_offers ADD COLUMN is_custom INTEGER NOT NULL DEFAULT 0 CHECK (is_custom IN (0, 1));

CREATE INDEX IF NOT EXISTS idx_offer_custom ON vpn_offers (is_custom, active);

-- ---------------------------------------------------------------------------
-- The spec an order carries (D-035.2, D-036.1)
-- ---------------------------------------------------------------------------
-- A CUSTOM order's limits are the CUSTOMER's, so they are recorded on the order
-- rather than on a package. They are copied onto the VPN row at release, so the
-- panel and the local record agree from the first second.
ALTER TABLE payment_records ADD COLUMN spec_gb      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payment_records ADD COLUMN spec_days    INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payment_records ADD COLUMN spec_devices INTEGER NOT NULL DEFAULT 0;
ALTER TABLE payment_records ADD COLUMN spec_ips     INTEGER NOT NULL DEFAULT 0;
-- The server and inbound the Super Admin picked when releasing a custom order
-- (D-036.6). 0 / NULL means "not chosen yet", which is a legal, visible state.
ALTER TABLE payment_records ADD COLUMN release_server_id  INTEGER REFERENCES server_integrations (id) ON DELETE SET NULL;
ALTER TABLE payment_records ADD COLUMN release_inbound_id INTEGER REFERENCES inbounds (id) ON DELETE SET NULL;

-- ---------------------------------------------------------------------------
-- Backfill — never guesses
-- ---------------------------------------------------------------------------
-- A platform default rate row so a fresh install can quote without a setup step.
-- Zero rates are deliberate: an operator must state their prices, and a zero
-- quote is refused by the validation below rather than sold for free.
INSERT INTO pricing_rates (server_id, currency, updated_at)
SELECT NULL, 'LKR', strftime('%s','now')
WHERE NOT EXISTS (SELECT 1 FROM pricing_rates WHERE server_id IS NULL);
