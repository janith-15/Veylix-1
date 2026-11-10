# 05 · QUALITY AND TESTING

Always loaded. Full matrix and evidence format: Skill `veylix-testing-review`.

Source of the commands below: `./README.md` §Verify, `./frontend/package.json`,
`./dev.ps1`.

## The gates (run these — do not invent replacements)

**Backend** — from `./backend/`:

```
go build ./...
go vet ./...
go test ./... -count=1
```

`go build` and `go vet` are the **only** authority on Go diagnostics. The
workspace editor config deliberately disables gopls staticcheck, so editor
squiggles are not evidence.

**Frontend** — from `./frontend/`:

```
npm run lint          # oxlint — this repo uses oxlint, not ESLint
npm run build         # tsc -b && vite build
npm run audit         # UI truth gate: no mocks, no unbacked claims
npm run audit:selftest
npm run docs:verify   # documentation drift gate
```

**Whole app**: `.\dev.ps1 -Status`, `.\dev.ps1 -Preflight`,
`.\dev.ps1 -Package`. `dev.ps1` is the single source of truth for run and
verify — call it, never reimplement it.

## Known blocked gate

`go test -race` is **blocked** on this machine (no C compiler). Report it as
`BLOCKED`, never as `PASS` and never as `FAIL`.

## Data safety — mandatory

`./VPN_FIX_LEDGER.md` records an incident: **a verification script deleted live
data, twice.** Therefore:

1. **Never execute a verification script against live user data.**
2. **Never delete, reset, truncate or overwrite existing runtime data.**
3. Any test touching persistent data uses an **isolated test database and an
   isolated data directory** — never `./var/`, never `./.veylix-secrets/`.
4. Prefer read-only verification. If a check must write, it writes to a
   temporary location and says so.
5. Never point a test at the production deployment or the seeded development
   panel.

## Truthful reporting

For each gate report exactly one of: **PASS** (command shown, output seen),
**FAIL** (output shown), **BLOCKED** (reason shown), **NOT RUN** (reason).

Never round a partial result into success. Never claim the UI works because the
build passed.

## Required QA coverage map — 11 areas

Every one of these is in scope for a change that touches it. Each area names the
**existing** command that satisfies it; do not invent a replacement, and do not
report an area as covered when its command did not run.

| # | Area | Satisfied by | Notes |
|---|---|---|---|
| 1 | Original requirement verification | cite the `./MASTER_PRODUCT_BIBLE.md` section in the change record | a requirement that is not in the Bible is not a requirement — ask the owner |
| 2 | Architecture boundary verification | `go build ./...` + reading the imports of every changed file | `httpapi` must not import the SQL driver; only `store` writes SQL; only `panel` calls outbound |
| 3 | Backend unit tests | `go test ./... -count=1` | `<file>_test.go` beside the source, same package |
| 4 | API integration tests | `go test ./... -count=1` + `.\dev.ps1 -Preflight` | preflight is the end-to-end route/role/refusal proof |
| 5 | Database tests | `go test ./... -count=1` with an **isolated** DB path | never `./var/`; never `./.veylix-secrets/` |
| 6 | Frontend tests | `npm run lint` + `npm run build` + `npm run audit` | `audit` is the UI truth gate; a passing build proves nothing about behaviour |
| 7 | Browser tests | `npm run verify:ui` (and the specific `verify:*` script for the surface) | deterministic, committed, headless-Chrome CDP; see the boundary rule below |
| 8 | Security regression checks | workflow `security-review.md` | read-only; emits findings, does not fix |
| 9 | Performance checks | workflow `performance-verification.md` | measured vs estimated must be labelled per row |
| 10 | Regression testing | the applicable rows of the 8-case matrix in Skill `veylix-testing-review` | a changed route with no negative test is incomplete |
| 11 | Final verification report | the truthful status table in workflow `release-readiness.md` | one FAIL means not ready, regardless of everything else |

**A skipped area is `NOT RUN` with a reason. It is never `PASS`.** "Not
applicable" is only valid when the change genuinely cannot reach that area, and
the reason must say why.

### Browser test boundary — Playwright MCP vs the verify scripts

Two browser mechanisms exist. They do not overlap and must not be confused:

- **`frontend/scripts/verify-*.mjs` (committed, deterministic).** These are the
  regression proof. They launch headless Chrome over raw CDP, assert specific
  facts, and exit non-zero on failure. A gate row in area 7 means **one of
  these ran**, not that someone looked at a page.
- **Playwright MCP (interactive, exploratory).** Use it to *investigate* — walk a
  flow, read a snapshot, reproduce a bug, check what a page actually renders.
  It is **not** a gate: an agent driving a browser by hand produces no
  reproducible assertion, so its output can never be reported as `PASS` for
  area 7.

Use Playwright MCP to find the problem; commit a `verify-*.mjs` to prove it is
fixed.

## Definition of done

A change is done only when all of these are true:

1. The behaviour works end to end, not just at the endpoint.
2. Authorization is enforced **server-side** and the negative case is tested.
3. Data survives migration.
4. The Failure, stale and duplicate/retry cases behave honestly.
5. The gates above were run and their real output recorded.
6. The ledgers and `./docs/cline/progress.md` are updated.
