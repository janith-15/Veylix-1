# VEYLIX — DECISION LOG

**Status:** authoritative · **Established:** 2026-11-08 · **Baseline:** `1b09553`

**Relationship to `docs/DECISIONS.md`.** The legacy log holds **86 decisions,
`D-001`–`D-088`**, in 205 KB. It is preserved (archive + Git) and remains the
**historical record of why the current code looks the way it does**. This file is
the **forward** log: the `ADR-` series starts at `ADR-001` and records decisions
made from 2026-11-08 onward, in a form that states the trade-off rather than only
the outcome.

**Rule.** Append-only. A superseded decision keeps its text and gains a
`Superseded by` line. Nothing is rewritten.

---

## Why the legacy log is not reproduced here

Re-typing 86 decisions would create two sources of truth, and the Phase 2 audit
found that overlap is exactly what produced the drift this restart exists to fix.
The legacy `D-nnn` ids therefore remain valid references for the code that exists;
new work cites `ADR-nnn`.

`D-062`, `D-077`, `D-078`, `D-089` have **no recorded text** — see
`docs/restart/KNOWN_LOSSES.md` `L-02`.

---

## ADR-001 · Continue on the existing codebase instead of rebuilding

- **Date:** 2026-11-08 · **Status:** ACCEPTED · **Owner:** project owner
- **Context.** A "clean restart" was requested for documentation and process. Two
  readings were possible: rebuild the system, or keep the code and rebuild the
  documentation and process around it.
- **Evidence considered.**
  - 617 test functions across 15 packages, all passing.
  - The panel integration is proven live: `cmd/panelprobe` reported
    `vpn_id=42 … probe=FOUND`, meaning 3X-UI itself confirmed the client.
  - The security model (argon2id, server-side sessions, sealed tokens, SSRF guard,
    rate limits, two login doors, append-only audit) is implemented, not planned.
  - A rewrite would discard all of the above in exchange for **no new capability**.
- **Decision.** **Continue** on the existing backend and schema. Rebuild the
  documentation, the defect tracking and the process controls.
- **Consequences.** The legacy `D-nnn` ids stay valid. The `I-36`/`I-37` work stays
  in the tree. Phase 3 documents what exists rather than designing what should.
- **Alternative rejected.** Full rebuild — weeks-to-months of work to reach the
  current state, with a high chance of reintroducing defects the 617 tests catch.

---

## ADR-002 · Adopt Git as the change record

- **Date:** 2026-11-08 · **Status:** ACCEPTED · **Owner:** project owner
- **Context.** There was **no version control**. That absence is the direct cause of
  `KNOWN_LOSSES` `L-01`: a documentation file was damaged and no earlier version
  existed anywhere on disk.
- **Decision.** `git init` in `H:\my apps\Veylix-2`, branch `main`, with the existing
  `.gitignore` (which already excluded `node_modules/`, `dist/`, `var/`,
  `.veylix-secrets/`, `*.db`). The existing tree was committed as
  `1b09553 baseline: import existing Veylix tree before the documented restart`
  (**717 files, 165,391 insertions**), with secret/database exclusions verified
  before the commit.
- **Consequences.** Every future change is diffable and revertible. `L-01` cannot
  recur. A commit is now part of "done".
- **Note.** The commit is an **import**, not a history. Nothing before 2026-11-08
  is individually recoverable; that loss is permanent and recorded.

---

## ADR-003 · Archive rather than delete the legacy documentation

- **Date:** 2026-11-08 · **Status:** ACCEPTED · **Owner:** project owner
- **Context.** `MASTER_PRODUCT_BIBLE.md` and 31 files under `docs/` are superseded
  by this new set. Deleting them would destroy the only explanation of why the
  current schema and code look as they do.
- **Decision.** **Copy** (never move, never delete) the entire legacy set to
  `H:\my apps\_veylix-archive\docs-legacy-2026-11-08\` (44 files, 1.05 MB), and keep
  the originals in place. Archive the source tree, the database and the secrets
  separately.
- **Consequences.** Two documentation trees exist. The rule that resolves any
  conflict is stated in `MASTER_SPEC.md`: **this set governs; the Bible is
  historical reference.**
- **Rejected.** Moving `docs/` to `docs/_archive/` — a path change would break every
  existing reference in code comments and the ledgers.

---

## ADR-004 · New ids start at ADR-001 and DEV-001

- **Date:** 2026-11-08 · **Status:** ACCEPTED · **Owner:** project owner
- **Context.** Five registers used overlapping id schemes: `F-nnn`, `M6-nn`,
  `M7-nn`, `I-nn`, `D-nnn`, plus `U-nn` and `S3-n`. A reader could not tell which
  register owned a given number, and `I-33` was described both as a ledger entry
  and as a defect id.
- **Decision.**
  - **`ADR-nnn`** — architectural / product decisions from 2026-11-08 onward.
  - **`DEV-nnn`** — implementation work, defects and fixes, in
    `DEVELOPMENT_LEDGER.md`.
  - **`LEGACY-<id>`** — when citing an old id (`I-34`, `U18`, `S3-1`), prefix it so
    its origin is unambiguous.
- **Consequences.** A new defect gets one id in one register. Old ids stay citable
  but are never re-minted.

---

## ADR-005 · The evidence markers C / A / U are mandatory

- **Date:** 2026-11-08 · **Status:** ACCEPTED · **Owner:** project owner
- **Context.** The restart directive requires documentation to distinguish
  confirmed facts, assumptions and unknowns. The Phase 2 audit produced a concrete
  demonstration of why: a first draft of `ARCHITECTURE.md` §6.2 asserted that
  `versioning/service.go` called the 3X-UI panel. **It did not** — the string
  appeared only inside a comment. The claim was corrected, and the correction was
  kept in the file rather than quietly deleted.
- **Decision.** Every factual statement in `docs/new/` carries **C** (confirmed in
  code / schema / a read-only query), **A** (assumed), or **U** (unknown, linked to
  `UNKNOWNS.md`). An unmarked statement is a rule the system must obey, not a claim
  about the implementation.
- **Consequences.** Slower to write, faster to read, and a fabricated finding
  cannot hide inside confident prose.

---

## ADR-006 · Record what the code actually does, including where it breaks its own rules

- **Date:** 2026-11-08 · **Status:** ACCEPTED · **Owner:** project owner
- **Context.** The architecture rule "only `store` writes SQL" is stated in
  `.clinerules` and in the legacy `ARCHITECTURE.md`. It is **not true**: 112 SQL
  statements live outside `store/`, twelve of them inside `httpapi`, which the rule
  forbids outright.
- **Decision.** Document the real state and record the divergence, rather than
  repeating the rule as though it held. `ARCHITECTURE.md` §6 carries the measured
  counts, the assessment, two remediation options and a recommendation.
- **Consequences.** The rule becomes an honest debt register instead of an
  aspiration a future contributor will discover to be false. Fixing it becomes
  scheduled work rather than silent drift.
- **Follow-up.** `UNKNOWNS.md` `Q-13` asks the owner to choose Option A (move the
  12 `httpapi` queries into `store`) or Option B (rewrite the rule to describe
  reality).

---

## ADR-007 · The development database is test data and is not migrated

- **Date:** 2026-11-08 · **Status:** ACCEPTED · **Owner:** project owner
- **Context.** `.veylix-secrets/data/veylix.db` holds 8 users, 1 panel, 5 offers, 4
  VPNs. Some rows (`Free 1GB Trial` offer 17, VPN 42) were created during the
  2026-11-08 session purely to prove the `I-36` provisioning fix.
- **Decision.** Treat it as **development seed data**, not as data to preserve
  through a migration. It is archived (`_veylix-archive\data-2026-11-08\`) and
  remains usable for development, but it creates no migration obligation.
- **Consequences.** `MIGRATION_PLAN.md` has no data-migration step for this file.
  A real production instance would need its own plan — and no production instance
  exists (`UNKNOWNS.md` `Q-03`).
- **Reversible.** If the owner later says this data matters, the archive copy is
  byte-identical to the live file and can be promoted.

