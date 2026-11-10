-- 0028_single_super_admin.sql — enforce exactly one Super Admin
--
-- P-1: nothing prevented a second `super_admin`. `CreateUserByRole` documents
-- itself as "the only path that may mint … a second `super_admin`" — the comment
-- acknowledged the power and nothing constrained it. `SetUserRole` could also
-- promote `admin → super_admin`. Two Super Admins is not a cosmetic problem: the
-- role is the platform owner, it holds the panel credential, it can demote every
-- other account, and the "self-modification" guards are written on the assumption
-- that there is one of them.
--
-- The rule is enforced in the DATABASE, not only in the service, for the same
-- reason the audit log is (0027): a service-level rule is only as strong as the
-- next code path somebody writes.
--
-- A partial unique index is the smallest honest expression of "at most one row may
-- have this role": SQLite applies uniqueness only to rows that match the WHERE
-- clause, so any number of admins and clients remain allowed.
--
-- ---------------------------------------------------------------------------
-- PRE-MIGRATION STATE. If a deployment already holds two or more super_admin
-- rows this statement FAILS and the migration aborts — which is the behaviour we
-- want, because silently demoting someone would change who owns the platform.
-- The operator resolves it deliberately:
--
--   SELECT id, email, created_at FROM users WHERE role = 'super_admin' ORDER BY id;
--
-- The FIRST row is the one the bootstrap and every existing session belong to;
-- demote the rest through `vpn-platformctl` (which writes an audit entry) and
-- re-run the upgrade. Never delete a user to satisfy an index.
-- ---------------------------------------------------------------------------

CREATE UNIQUE INDEX idx_users_single_super_admin
    ON users (role)
    WHERE role = 'super_admin';
