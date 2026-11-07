# VEYLIX — MIGRATION PLAN

**Status:** authoritative · **Established:** 2026-11-08 · **Baseline:** `1b09553`

**What this file answers.** "What had to move for the restart to be safe, what is
still open, and what would a real production migration require?"

---

## 1. Scope: what was actually migrated

| Asset | From | To | Method | Status |
|---|---|---|---|---|
| Source tree | `Veylix-2` | `_veylix-archive\veylix-2-source-2026-11-08` | copied, 636 files / 5.91 MB | ✅ **DONE** |
| Legacy docs (44 files) | `docs\`, root Bible | `_veylix-archive\docs-legacy-2026-11-08` | copied, 1.05 MB | ✅ **DONE** |
| Development database | `.veylix-secrets\data\veylix.db` | `_veylix-archive\data-2026-11-08` | copied, 798,720 bytes | ✅ **DONE** |
| Secrets (16 files) | `.veylix-secrets\` | `_veylix-archive\secrets-2026-11-08` | copied | ✅ **DONE** |
| Version control | (none) | `.git` in `Veylix-2`, commit `1b09553` | `git init` + import | ✅ **DONE** |
| Documentation | legacy set | `docs\new\` | rewritten from verified evidence | ✅ **DONE** |

**Exclusions verified absent from the archive:** `node_modules`, `dist`, `var`,
`.veylix-secrets`, `veylix-ui demo`. **Required files present: 15/15.**

---

## 2. What was deliberately NOT migrated

| Item | Reason |
|---|---|
| Legacy `docs/` originals | Kept in place — ADR-003 says copy, never move. Path changes would break code-comment references. |
| Legacy registers (`IMPROVEMENT_LEDGER`, `VPN_FIX_LEDGER`, …) | Kept as historical reference. Superseded by `DEVELOPMENT_LEDGER.md`; **never appended to again.** |
| `./veylix-ui demo/` | A separate application. Not Veylix. |
| `node_modules/`, `dist/` | Regenerated from `package.json`. |
| `var/` | Local runtime scratch. |

---

## 3. The database: no schema migration is required

**C:** all **23** migrations are applied to the development database and every one is
idempotent-by-version (tracked in `schema_migrations`). The restart changed **no**
schema. Therefore:

- **Fresh install:** `st.Migrate(ctx)` applies 0001→0023 in order. Nothing to do.
- **Existing install:** the version table already reads 23. Nothing to do.

**This is the key finding: the restart is a documentation and process change with
zero schema impact.** Any plan claiming otherwise would be inventing work.

---

## 4. The development database is test data (ADR-007)

`.veylix-secrets\data\veylix.db` — 8 users, 1 panel (`indian-1`), 5 offers, 4 VPNs,
46 notifications. Offer 17 and VPN 42 exist only because they were created on
2026-11-08 to prove the `DEV-001` / `LEGACY-I-36` fix.

**Decision:** treat it as seed data. It creates no migration obligation. It stays
usable for development, and the archive holds a byte-identical copy.

**If that judgement is ever reversed:** promote
`_veylix-archive\data-2026-11-08\veylix.db` — it is the same bytes.

---

## 5. A real production migration — what it WOULD require

No production instance exists (`UNKNOWNS.md` `Q-03`), so this section is a
**specification for a future event**, not a record of work done. It is written now so
the requirement is not rediscovered under pressure.

**Pre-flight**

1. Take a filesystem-level backup of the SQLite file **and** its `-wal` / `-shm`
   companions. A copy of the `.db` alone can be stale by a transaction.
2. Record `SELECT COUNT(*)` for every table — the before/after fingerprint.
3. Verify `./.veylix-secrets/` is intact and the AES-256-GCM key still decrypts the
   panel token. **A restored database with a lost key has no working panels**, and no
   amount of schema work fixes that.
4. Confirm the running binary version with `GET /api/system/version`.

**Execution**

5. Stop the service. A migration must never run against a live writer.
6. Apply migrations in order. Never skip, never reorder.
7. Restart and assert `GET /api/ready` returns ready with the expected
   `embedded:N_files`.

**Post-flight**

8. Re-run the fingerprint. A row count that decreased without an explained delete is
   a **stop** condition.
9. `GET /api/servers` → confirm the panel is reachable and the token still decrypts.
10. `cmd/panelprobe <vpn_id>` → confirm a known client is `FOUND` **on the panel**.
    Local state alone is not proof (INV-1).
11. `npm run preflight` against the migrated instance.

**Rollback.** Restore the pre-flight backup **and** the matching secrets directory
together. Restoring one without the other yields a database whose sealed tokens
cannot be read.

---

## 6. Open migration questions

| Id | Question | Current lean |
|---|---|---|
| `Q-02` | Migrate or discard the dev database? | **Discard** — ADR-007. It is test data. |
| `Q-03` | Does a production instance exist anywhere? | **No evidence of one.** |
| `Q-14` | Should `inbounds.id` ever be renumbered to match `remote_id`? | **No** — the translation in `panelInboundID` is the fix, and renumbering would break existing foreign keys. |

---

## 7. What must never happen during a migration here

1. Never drop or truncate a table to make a test pass.
2. Never run a verification script against live data (a recorded incident, twice).
3. Never claim a remote panel state from local database rows.
4. Never restore a database without restoring its matching secrets directory.
5. Never apply a migration with the service running.
