-- 0024_ping_samples.sql — latency history for the VPN details page (D-092)
--
-- Why this table exists
-- --------------------
-- `POST /api/vpns/{id}/ping` measured the round trip and then threw the number
-- away, so the page could only ever show the LAST measurement. A customer
-- asking "is my connection getting worse?" had nothing to compare against, and
-- the Performance chart the owner asked for had no source at all.
--
-- What is stored, and what is deliberately NOT
-- -------------------------------------------
-- Only what was actually measured. `reachable = 0` with `latency_ms = -1`
-- records a FAILED probe — that is a real observation and it must survive, so
-- the reader can tell "measured and down" from "never measured" (D-026.3: an
-- unmeasured value is never 0). There is no placeholder latency and no
-- synthesised sample: a row exists only because a probe ran.
--
-- `scope` is stored because the probe in this product measures the SERVER's
-- reachability (`server_reachability`), not the customer's own device latency
-- (D-026.1/.2). Writing it down means a future client-side probe can be told
-- apart from these rows instead of being silently mixed in.
--
-- Ownership for reads is resolved from `vpn_instances` exactly as it is for
-- `/usage`; this table carries no principal of its own.
--
-- Safety: additive only. No existing table, column, index or row is touched,
-- and ON DELETE CASCADE means a released (tombstoned) VPN takes its probes with
-- it rather than leaving orphans.

CREATE TABLE ping_samples (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    vpn_instance_id INTEGER NOT NULL REFERENCES vpn_instances (id) ON DELETE CASCADE,
    taken_at        INTEGER NOT NULL,
    -- 1 = the server answered, 0 = the probe failed or timed out.
    reachable       INTEGER NOT NULL DEFAULT 0,
    -- Round-trip in milliseconds; -1 when unreachable — never 0, which would
    -- read as "instant".
    latency_ms      INTEGER NOT NULL DEFAULT -1,
    -- Which measurement point produced this row (D-026.2).
    scope           TEXT    NOT NULL DEFAULT 'server_reachability'
);

CREATE INDEX IF NOT EXISTS idx_ping_instance_taken
    ON ping_samples (vpn_instance_id, taken_at);
