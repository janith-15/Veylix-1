# VEYLIX — ACTIVE CONTEXT

**Updated:** 2026-11-08 · **Baseline commit:** `1b09553` (main)

---

## Where the project stands

**Phases 1–3 of the 5-phase restart are complete.** Phase 4 (baseline) and Phase 5
(development) have not started and are waiting on the owner reading Phase 3.

| Phase | State |
|---|---|
| 1 · PROTECT | ✅ **DONE** — archive at `H:\my apps\_veylix-archive\` |
| 2 · AUDIT | ✅ **DONE** — `docs/restart/` (3 files) |
| 3 · NEW DOCS | ✅ **DONE** — `docs/new/` (10 files, ~78 KB) |
| 4 · BASELINE | ⏸ **NOT STARTED** — awaits owner review |
| 5 · DEVELOPMENT | ⏸ **NOT STARTED** |

---

## What changed in this session

1. **Git adopted** (ADR-002). No version control existed before; that absence caused
   the ledger loss `L-01`. `git init` → branch `main` → commit `1b09553`:
   **717 files, 165,391 insertions**, with `node_modules/`, `dist/`, `var/`,
   `.veylix-secrets/` and `*.db` verified excluded before committing.
2. **`docs/new/` written** — 10 authoritative documents, all clean UTF-8, no BOM,
   zero replacement characters.
3. **A real architecture violation found and recorded, not hidden.** 112 SQL
   statements live outside `store/`; **12 of them inside `httpapi`**, which the rule
   forbids outright. Recorded as `DEV-003` / `Q-13` with two remediation options.
4. **A fabricated finding caught and corrected.** A first draft of
   `ARCHITECTURE.md` §6.2 claimed `versioning/service.go` called the 3X-UI panel. It
   does not — the string appears only in a comment. Corrected, and the correction is
   kept in the file per ADR-005.
5. **`Q-12` resolved:** `docs/new/MASTER_SPEC.md` is authoritative; the Bible is
   historical reference.

---

## Gates — actual state on 2026-11-08

| Gate | Result |
|---|---|
| `go build ./...` | **PASS** (exit 0) |
| `go vet ./...` | **PASS** (exit 0) |
| `gofmt -l internal\ cmd\` | **PASS** (no files listed) |
| `go test ./... -count=1` | **PASS** — 17/17 packages ok, **617 tests** |
| `go test -race` | **BLOCKED** — no C compiler. Never PASS, never FAIL. |
| `npm run lint` | **PASS** — 2 pre-existing warnings, 0 errors |
| `npm run build` | **PASS** — built in 1.04 s |
| `npm run audit` | **PASS** |
| `npm run audit:selftest` | **PASS** — 8 positive + 6 negative |
| `npm run docs:verify` | **PASS** — 23 migrations · 133 routes · 617 tests / 15 packages |
| `npm run preflight` | **NOT RUN** — needs a live server |
| `verify:nav` / `verify:ui` | **PASS** earlier this session (74/74, 0 console errors); not re-run in the docs pass |

---

## The five things that matter most right now

1. **`DEV-003` / `Q-13` is the one open architectural decision.** `httpapi` runs 12
   SQL statements. Either move them into `store` (Option A) or rewrite the rule to
   describe reality (Option B). Recommendation: **A for `httpapi`** — the layer with
   the clearest prohibition — and **B for the service packages**, where extracting
   in-transaction queries would be a large risk for no functional gain.
2. **`DEV-001` / `LEGACY-I-36` is fixed and proven at both layers.** The panel was
   asked with the right inbound number, and `cmd/panelprobe` confirmed `vpn_id=42 →
   FOUND` **on the panel itself**, not merely in local rows.
3. **The dev database is seed data, not production data** (ADR-007).
   `.veylix-secrets\data\veylix.db`, 798,720 bytes, 23/23 migrations. No schema change
   happened in this restart, so no database migration is required.
4. **`go test -race` cannot run here.** No C compiler. Report `BLOCKED`.
5. **Five P2 UI defects and two P3 cascade defects remain open** — `DEV-004`–`DEV-010`.

---

## Open questions — none blocking, all needing an answer

| Id | Question | Lean |
|---|---|---|
| `Q-02` | Migrate or discard the dev database? | Discard — ADR-007 |
| `Q-03` | Does a production instance exist? | No evidence of one |
| `Q-05` | Is chat in scope? Already built (7 routes, 3 tables) | Keep |
| `Q-11` | Re-create or retire `D-062`, `D-077`, `D-078`, `D-089`? | Retire formally |
| `Q-13` | `httpapi` SQL: move the code, or change the rule? | A for `httpapi`, B for services |
| `Q-14` | Renumber `inbounds.id` to equal `remote_id`? | No — the translation already fixes it |

---

## Next action

Read `docs/new/MASTER_SPEC.md` and `docs/new/ARCHITECTURE.md` §6, then decide
**`Q-13`**. That decision gates Phase 4.

**Before any release claim:** run `npm run preflight` against a live server. It is
`NOT RUN`, not `PASS`.
