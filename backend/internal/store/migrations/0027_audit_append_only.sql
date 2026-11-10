-- 0027_audit_append_only.sql — make the audit log immutable in the DATABASE
--
-- B-5: `docs/DECISIONS.md` and the Bible both state that the audit log is
-- append-only, but nothing enforced it. `audit_routes_test.go` proved the point
-- the wrong way round: it ran `UPDATE audit_logs` and the statement SUCCEEDED.
-- A rule that lives only in a comment is a rule that survives exactly as long as
-- nobody writes the wrong query — and the wrong query can come from a future
-- migration, an operator with sqlite3 open, or a bug in this codebase.
--
-- Triggers, not a CHECK or a privilege: SQLite has no table-level privilege model,
-- and `BEFORE UPDATE`/`BEFORE DELETE` are the only hooks that fire for every
-- writer, including ones this application does not own.
--
-- Deliberate consequence: a migration can no longer "fix" history. That is the
-- point. A correction is a NEW appended entry, which is what an audit trail means
-- (see DECISIONS: append, never rewrite).

CREATE TRIGGER audit_logs_no_update
BEFORE UPDATE ON audit_logs
BEGIN
    SELECT RAISE(ABORT, 'audit_logs is append-only: rows cannot be updated');
END;

CREATE TRIGGER audit_logs_no_delete
BEFORE DELETE ON audit_logs
BEGIN
    SELECT RAISE(ABORT, 'audit_logs is append-only: rows cannot be deleted');
END;
