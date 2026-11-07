-- ---------------------------------------------------------------------------
-- Panel assignment ledger (D-037, D-034.5)
-- ---------------------------------------------------------------------------
-- A panel client created directly in 3X-UI can be given to a client account.
-- The VPN row is then recorded under the customer's slot name
-- (`nimal@gmail.com_paid_1`), which is NOT the name the panel currently uses,
-- so the original name has to be remembered for two reasons:
--   1. a second assignment of the same panel row must be refused, not duplicated
--      (the "already assigned" check cannot look it up by the new name);
--   2. the operator must be able to see that `direct-3xui-1` became
--      `nimal@gmail.com_paid_1`, and why.
CREATE TABLE IF NOT EXISTS panel_assignments (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id       INTEGER NOT NULL REFERENCES server_integrations (id) ON DELETE CASCADE,
    source_email    TEXT    NOT NULL,
    panel_email     TEXT    NOT NULL,
    vpn_instance_id INTEGER REFERENCES vpn_instances (id) ON DELETE SET NULL,
    actor_id        INTEGER REFERENCES users (id) ON DELETE SET NULL,
    created_at      INTEGER NOT NULL,
    UNIQUE (server_id, source_email)
);
CREATE INDEX IF NOT EXISTS idx_panel_assignments_vpn ON panel_assignments (vpn_instance_id);
