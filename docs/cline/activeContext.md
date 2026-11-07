# VEYLIX — ACTIVE CONTEXT

**Updated:** 2026-11-08 · **Baseline commit:** `1b09553` (main) + Phase-3 commit `3a57724`

---

## Where the project stands

Phases 1–3 of the restart are complete. Separately, the **client portal** has been
resumed at the point where it stopped (**S5, My VPNs**), and that section is now
finally *proven* rather than merely written.

| Phase | State |
|---|---|
| 1 · PROTECT | ✅ **DONE** — archive at `H:\my apps\_veylix-archive\` |
| 2 · AUDIT | ✅ **DONE** — `docs/restart/` |
| 3 · NEW DOCS | ✅ **DONE** — `docs/new/` (10 files) |
| 4 · BASELINE | ⏸ **NOT STARTED** — `npm run preflight` still NOT RUN |
| 5 · DEVELOPMENT | 🟢 **IN PROGRESS** — client portal resumed |

### Client portal sections

| # | Section | Route | State |
|---|---|---|---|
| S4 | Dashboard (01) | `/client` | ✅ done |
| S5 | My VPNs (02) | `/client/vpns` | ✅ **PROVEN 19/19 against real rows** |
| **S6** | **Offers & Orders (03)** | `/client/offers` | 🔴 **next** — needs `ADR-008` |
| S7–S11 | Billing · Usage · Guides · Support · Notifications | `/client/*` | 🔴 empty shells |

---

## What changed in this pass

1. **S5's `NOT PROVEN` caveat is closed.** The blocking chain dissolved once
   `DEV-001` landed: `POST /api/offers` 500 → 201, so the client could be given
   VPNs. Client 15 now owns **3** VPNs (42, 43, 44).
2. **Both layers proven.** `cmd/panelprobe -server 11` → `probe=FOUND` for **6/6**
   panel clients (panel 3.11.0, xray 26.10.21 running). These are real 3X-UI
   clients, not local rows.
3. **A new committed gate.** `npm run verify:client-vpns` — **19 checks, 19/19
   PASS**. It is the proof the S5 report could not produce.
4. **The gate refuses to lie.** On an account owning 0 VPNs it prints `REFUSED …
   Reporting PASS here would be a false claim.` and exits **3**. Executed and
   observed.
5. **A wrong finding corrected.** My first probe said sort was missing
   (`sortable: []`). It is not — sort is a `<select>` inside the collapsible Filters
   panel (`SortKey = newest | name | expiry`). The probe looked in the wrong place.
   Recorded in `DEV-011` and in `progress.md` rather than quietly dropped.

**No production source changed in this pass.** Only a new test script, its
`package.json` entry, and documentation.

---

## Gates — actual state, 2026-11-08

| Gate | Result |
|---|---|
| `go build ./...` | **PASS** (exit 0) |
| `go vet ./...` | **PASS** (exit 0) |
| `gofmt -l internal\ cmd\` | **PASS** (no files listed) |
| `go test ./... -count=1` | **PASS** — 17/17 packages, **617 tests** |
| `go test -race` | **BLOCKED** — no C compiler. Never PASS, never FAIL. |
| `npm run lint` | **PASS** — 2 pre-existing warnings, 0 errors |
| `npm run build` | **PASS** — 1.32 s |
| `npm run audit` | **PASS** |
| `npm run audit:selftest` | **PASS** — 8 positive + 6 negative |
| `npm run docs:verify` | **PASS** — 23 migrations · 133 routes · 617 / 15 packages |
| `npm run verify:nav` | **PASS** — 74/74 |
| `npm run verify:ui` | **PASS** — `consoleErrors: []` |
| `npm run verify:client-vpns` | **PASS** — **19/19** |
| `verify:client-vpns` refusal path | **PASS** — `REFUSED`, exit **3** |
| `npm run preflight` | **NOT RUN** — needs a live server. Must run before any release claim. |

---

## The five things that matter most right now

1. **S6 is the next section, and it needs one decision first (`ADR-008`).** The
   `/offers` "collision" is **not** a backend defect — `GET /api/offers` is already
   role-scoped (staff → full catalogue, client → active FREE only). It is a
   **navigation** question: `/offers` renders the staff create/edit page and should
   move under a staff-scoped path. `D-089` was meant to record this and its text is
   lost (`KNOWN_LOSSES` `L-02`).
2. **`DEV-003` / `Q-13` is still the one open architectural decision.** 112 SQL
   statements live outside `store/`, 12 inside `httpapi`. Recommendation stands:
   Option A for `httpapi`, Option B (rewrite the rule) for the service packages.
3. **`DEV-001` is fixed and proven at both layers** — the panel received the correct
   inbound number and 3X-UI itself confirmed the clients `FOUND`.
4. **The dev database is seed data, not production data** (ADR-007).
   `.veylix-secrets\data\veylix.db`, 23/23 migrations. No schema change in this
   restart, so no migration is required.
5. **Open P2/P3 debt:** `DEV-004`–`DEV-010` (two cascade defects, 224 inline styles,
   three missing empty states, mixed loaders, unverified focus rings, a one-width
   mobile gate).

---

## Open questions

| Id | Question | Lean |
|---|---|---|
| `Q-02` | Migrate or discard the dev database? | Discard — ADR-007 |
| `Q-03` | Does a production instance exist? | No evidence of one — **owner must answer** |
| `Q-05` | Is chat in scope? Already built (7 routes, 3 tables) | Keep |
| `Q-11` | Re-create or retire `D-062`, `D-077`, `D-078`, `D-089`? | Retire formally; re-record `D-089`'s intent as `ADR-008` |
| `Q-13` | `httpapi` SQL: move the code, or change the rule? | A for `httpapi`, B for services |
| `Q-14` | Renumber `inbounds.id` to equal `remote_id`? | No — the translation already fixes it |

---

## Next action

Write **`ADR-008`** for the `/offers` navigation decision, then build **S6 Client
Offers** (`/client/offers`): FREE claim + PREMIUM browse, with a committed
`verify-client-offers.mjs` written the same way as `verify-client-vpns.mjs` — one
that **refuses** rather than passing on an empty catalogue.

**Before any release claim:** run `npm run preflight`. It is `NOT RUN`, not `PASS`.
