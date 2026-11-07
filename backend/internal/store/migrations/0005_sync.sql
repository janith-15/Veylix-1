-- 0005_sync.sql — idempotent write keys for the sync layer (Stage E5)
--
-- Source: docs/REQUIREMENTS.md §2.7, §3; F-010 (lifetime totals); D-023.
--
-- The sync pass writes one snapshot per VPN per tick and one metrics sample per
-- server per tick. A unique key makes a re-run of the same tick idempotent
-- (ON CONFLICT DO UPDATE) instead of inserting duplicates, and gives the history
-- queries a covering index.
--
-- No column is added to vpn_instances: lifetime_up_bytes, lifetime_down_bytes and
-- last_remote_reset_count already exist from 0002_business.sql.

-- One snapshot per (vpn, second). taken_at has second precision, so a retried
-- tick inside the same second overwrites rather than duplicating.
CREATE UNIQUE INDEX IF NOT EXISTS ux_usage_instance_taken
    ON usage_snapshots (vpn_instance_id, taken_at);

-- One metrics sample per (server, second).
CREATE UNIQUE INDEX IF NOT EXISTS ux_metrics_server_taken
    ON server_metrics (server_id, taken_at);

-- Fast "latest snapshot for this VPN" lookup used by Fold to compute a delta.
CREATE INDEX IF NOT EXISTS idx_usage_instance_latest
    ON usage_snapshots (vpn_instance_id, taken_at DESC);

-- Fast "history for this VPN in a time window" and "stale servers" queries.
CREATE INDEX IF NOT EXISTS idx_metrics_server_latest
    ON server_metrics (server_id, taken_at DESC);
CREATE INDEX IF NOT EXISTS idx_servers_last_sync
    ON server_integrations (status, last_sync);
