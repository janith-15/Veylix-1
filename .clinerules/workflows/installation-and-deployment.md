# WORKFLOW · INSTALLATION AND DEPLOYMENT

Use for any change to **how Veylix gets onto a server and stays there**: the
installer, the upgrade path, the CLI, the Caddy/systemd templates, the backup and
restore path, uninstall, or the release artifacts those depend on.

This is a **permanent product surface**, not a one-off build step. The Bible
requires it (§14, and acceptance tests 30–39), so it is governed like any other
feature — with one addition no other workflow has: **every stage here can
destroy a customer's data**, and this project has a recorded incident where it
did exactly that (`./VPN_FIX_LEDGER.md`: a verification script deleted live data,
**twice**).

Load Skill `veylix-deployment` for the mechanism of each check, and
`veylix-security` for the secret-handling rules.

**Relationship to the other workflows:** `system-design-audit.md` decides *what*
this surface should be and stops at an approval gate. This workflow executes an
**approved** design. `release-readiness.md` Stage 5b verifies the result.

---

## The three rules that make this workflow work

**1. Fresh install and upgrade are different workflows, and the code must branch
on it.** An installer that cannot tell "empty server" from "live production" is
a data-loss bug waiting to be run twice.

**2. Nothing executes outside an isolated environment until the owner names a
real target.** `./var/`, `./.veylix-secrets/`, `./records/` and any existing
database are off limits for every test in this workflow.

**3. `--dry-run` first, always.** A destructive script's first execution in a
session is its dry run. This is not a preference; it is how phase ordering and
preflight logic get proven without touching anything.

---

## Stage 1 — Classify the change

Answer in writing before touching a file:

| Question | Why it matters |
|---|---|
| Is this fresh-install, upgrade, repair, uninstall, or shared machinery? | each has a different blast radius |
| Can this change overwrite or delete existing data on any path? | if yes it is `P0`-class work and needs a backup step before it |
| Does it touch the two key files (`enc.key`, `session.key`)? | losing `enc.key` makes every sealed panel token unrecoverable |
| Does it change the schema, or only the artifact? | a schema change needs `veylix-database-engineering` |
| Does it change any documented command in `./docs/INSTALL.md` §F? | that status table is a claim and must stay true |

**Checkpoint:** the change is classified and its data-destruction surface named.

## Stage 2 — Fresh-install path safeguards

For the fresh-install path, confirm **in the code**, not in a comment:

- [ ] It **refuses** to run when a database already exists at the configured
      data directory. Show the branch.
- [ ] It has a **lock** so two concurrent runs cannot interleave. Show the lock.
- [ ] It generates **new** secrets only when the key files are absent, and
      refuses rather than silently reusing another install's keys. Show the
      guard.
- [ ] The artifact is **checksum-verified** before install, and a mismatch
      aborts. Show the comparison.
- [ ] Preflight validates OS, architecture, disk, RAM, DNS and ports **before**
      installing anything, and exits with a correction rather than a stack trace.
- [ ] Failure at any phase leaves a described end state — never a
      half-installed service that looks started.
- [ ] Re-running on an installed host is **idempotent**: it reports "already
      installed" and changes nothing.

Every box needs a `file:line`, or it stays unticked and the stage is not done.

**Checkpoint:** seven boxes ticked with evidence, or the gaps are named.

## Stage 3 — Upgrade path safeguards

- [ ] A **pre-upgrade backup** is taken automatically, not merely recommended.
- [ ] The schema version **before** and **after** is recorded and compared.
- [ ] Migrations are additive; nothing drops or rewrites a populated table.
- [ ] The previous binary is kept, so a binary rollback exists.
- [ ] A failed migration leaves the service **down**, not half-migrated — and the
      documented recovery restores the backup.
- [ ] Rollback across a migration is honest: it is a **restore**, not a schema
      downgrade, and the doc says so.

**Checkpoint:** every box has evidence, and the rollback path is described for
both the migrated and non-migrated case.

## Stage 4 — Repair and uninstall

- **Repair** may re-render configuration, reload services and re-check readiness.
  It may **not** touch the database, rewrite keys, or modify accounts. Show that
  it cannot.
- **Uninstall** preserves data by default. Removing the service must not remove
  `/var/lib/veylix` or `/etc/veylix`. A destructive purge exists only behind an
  explicit flag **and** a typed confirmation.
- Removing Veylix **never** deletes a 3X-UI client. That is a product invariant
  (D-027.3) and it applies to the uninstaller too.

**Checkpoint:** the data-preserving default is visible in the code path.

## Stage 5 — Execute, isolated only

Order of execution. Do not skip a step:

1. **`bash -n`** every new or changed script.
2. **`shellcheck`** if it is available; if not, say so and never claim it ran.
3. **`--dry-run`** and read the phase list it prints.
4. **Isolated run**: a throwaway data directory and a throwaway secrets path
   under `os.tmpdir()`. Record both absolute paths in the record.
5. **Second run** on the same isolated host, to prove idempotency.
6. **Delete only the throwaway directory you created.** Never a repo path.

If the change needs a real Linux VM, a domain, or TLS issuance, it is
`BLOCKED` — say so, name what is missing, and stop. Do not simulate a pass.

**Checkpoint:** dry-run output and isolated-run output both recorded, with the
throwaway paths named.

## Stage 6 — Verify through the one canonical check list

Fresh-install and deployment verification lives in
**`release-readiness.md` Stage 5b**, and only there. Do not restate its 13 checks
in this file, and do not keep a second copy anywhere — a duplicated checklist
drifts, and a drifted checklist is how a claim outlives its evidence.

Run that stage and record its table.

**Checkpoint:** the Stage 5b table is filled with this pass's real output.

## Stage 7 — Record

- New or changed commands → the status table in `./docs/INSTALL.md` §F.
  A row flips from not-built to built **only** when a stage above observed it.
- New operator-facing behaviour → `./docs/DECISIONS.md` as the next `D-nnn`.
- Defects found → `./docs/IMPROVEMENT_LEDGER.md` (or `./VPN_FIX_LEDGER.md` for
  data incidents) in the existing format.
- Commands run → `./docs/QA_REPORT.md` and a dated `./records/` file.
- Summary → `./docs/cline/progress.md`.

**Checkpoint:** no document claims a capability this pass did not observe.

---

## Checklist

- [ ] Change classified; data-destruction surface named before editing.
- [ ] Fresh path refuses over an existing database; lock present.
- [ ] Keys generated only when absent; reuse refused without an explicit flag.
- [ ] Artifact checksum verified; mismatch aborts.
- [ ] Preflight exits with a correction, not a crash.
- [ ] Upgrade takes a backup automatically and records schema before/after.
- [ ] Failed migration leaves the service down, and recovery is documented.
- [ ] Repair cannot touch the database, keys or accounts.
- [ ] Uninstall preserves data; purge needs an explicit flag and confirmation.
- [ ] `bash -n` and `--dry-run` ran; isolated-run paths recorded.
- [ ] Second run proven idempotent.
- [ ] Only the throwaway directory was deleted.
- [ ] Verification run through `release-readiness.md` Stage 5b, not a copy of it.
- [ ] `INSTALL.md` §F, ledger, decisions and progress updated truthfully.

