-- 0004_quota_limit.sql — reseller quota ceiling on the user row
--
-- REQUIREMENTS §2.2: quota = max number of ACTIVE Premium assignments.
-- The live count is derived from vpn_assignments (status = 'ACTIVE'), so expiry
-- or revocation releases a slot automatically; this column is only the ceiling.
--
-- 0 means "no quota configured" and is treated as unlimited by the service layer.

ALTER TABLE users ADD COLUMN quota_limit INTEGER NOT NULL DEFAULT 0;
