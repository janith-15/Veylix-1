---
name: veylix-testing-review
description: Use when writing or changing a Go test or frontend check, when choosing which gates to run, when about to claim a task is complete, or when reviewing a change before reporting it. Provides the test placement convention, the required case matrix covering normal unauthorized invalid-input stale-offline and duplicate-retry, the exact gate commands and their order, the data-safety rules that keep verification away from live data, and the truthful reporting format with PASS FAIL BLOCKED and NOT RUN. Load before asserting that anything works.
---

# VEYLIX — TESTING & REVIEW

Applied form of Rule 05. Read `./VPN_FIX_LEDGER.md` — it contains a recorded
incident where a verification script deleted live data, twice. That is why the
data-safety section below is not advisory.

## Test placement

- Go: `<file>_test.go` beside the source it tests, same package. Follow the
  existing pairing in `./backend/internal/store/`, `./internal/httpapi/`,
  `./internal/panel/`.
- Frontend: `./frontend/scripts/*.mjs`, exposed as an `npm run` script. Do not
  add an unregistered script.

## The required case matrix

For any changed behaviour, cover the cases that apply:

1. **Normal** — the happy path.
2. **Unauthorized** — unauthenticated request is refused.
3. **Wrong role** — an authenticated caller of the wrong role is refused.
4. **Wrong owner** — the right role but the wrong row is refused.
5. **Invalid input** — bad URL, protocol, limit, currency, price, remote ID.
6. **Stale / offline** — the panel is unreachable; the state is explicit and
   pending, never a fake success.
7. **Duplicate / retry** — an idempotent retry creates no second record or
   second panel client.
8. **Empty** — no data renders "no data", not a zero.

A change to an existing route without a negative test is incomplete.

## Gate order (fastest feedback first)

From `./backend/`:

```
go build ./...
go vet ./...                 # the only authority on Go diagnostics
go test ./... -count=1
```

From `./frontend/`:

```
npm run lint                 # oxlint
npm run build                # tsc -b && vite build
npm run audit                # UI truth gate
npm run docs:verify          # documentation drift
```

Whole app: `.\dev.ps1 -Status`, `.\dev.ps1 -Preflight`, `.\dev.ps1 -Package`.

**`go test -race` is BLOCKED here** — no C compiler on this machine. Report it
as blocked with that reason. Never silently omit it and never mark it passing.

## Data safety — mandatory, no exceptions

1. **Never execute a verification script against live user data.**
2. **Never delete, reset, truncate or overwrite existing runtime data.**
3. A test that touches persistent data uses an **isolated test database and an
   isolated data directory** — never `./var/`, never `./.veylix-secrets/`.
4. Treat `./backend/cmd/dbexec`, `seedacct`, `seeddemo` and any `-Clean` /
   `-Reset` / `-Fresh` flag as **destructive**. Confirm the target path before
   running, and prefer not to run it at all.
5. Never point a check at the seeded development panel or a production
   deployment unless the owner asked for exactly that.

## Review before reporting

Read the change back. Ask:

- Did the rule land in **all four** surfaces — API, UI, database, tests?
- Is any claim in my summary something I actually observed?
- Did I leave a working build?
- Did I update `./docs/cline/progress.md`?

## Truthful reporting format

For every gate, exactly one of:

| Verdict | Requires |
|---|---|
| **PASS** | the command shown **and** its output seen |
| **FAIL** | the command and the failing output |
| **BLOCKED** | the reason it cannot run |
| **NOT RUN** | why it was skipped |

Never convert a partial result into success. Never say "should work". Never
report a number you did not read from a real command — if you need the
migration count, count the files; do not quote `./README.md`.

## Two browser mechanisms — do not conflate them

- **Committed `frontend/scripts/verify-*.mjs`** are the only browser **gate**.
  They drive headless Chrome over raw CDP, assert specific facts and exit
  non-zero on failure. Exposed as `npm run verify:ui`, `verify:nav`,
  `verify:themes`, `verify:theme-toggle`, `verify:vpn`, `verify:vpn-actions`,
  `verify:subthemes`. Register any new script in `package.json` — an
  unregistered script is not a gate.
- **Playwright MCP** (`browser_*` tools) is **exploratory only**: reproduce,
  inspect a snapshot, confirm what renders. It produces no reproducible
  assertion, so it can never be reported as `PASS` for browser coverage. Use it
  to find the defect, then commit a `verify-*.mjs` to prove the fix.

Playwright MCP runs `--headless --isolated --browser chrome`: no browser
download, no persisted profile, and it must never be pointed at a production
Veylix service or the seeded development panel.

## Isolated data is mandatory, per layer

Restated for each layer because this is where it goes wrong:

- **Go tests** — set an isolated DB path / temp data dir in the test itself.
- **Migrations** — run against the isolated DB, never `./var/`.
- **Panel-facing tests** — a fake/local panel only, never the seeded dev panel.
- **Frontend verifiers** — a locally served build; their Chrome profile already
  resolves to `os.tmpdir()` via `chrome-profile.mjs`.
- **Fresh-install checks** — a throwaway data directory, and the backup is
  restored only into that directory.

Never delete, reset, truncate or overwrite runtime data to make a check pass.
`VPN_FIX_LEDGER.md` records a verification script that deleted live data twice.

## Findings (R3, R7) — historical only

Prior backend investigation (R3) and documentation/roadmap reconciliation (R7)
are **historical context**. Re-check the current source and the current docs
before acting on either. Do not treat a previously recorded gap or roadmap
position as still accurate — verify it.

## Checklist

- [ ] Applicable cases from the matrix are covered.
- [ ] Gate commands actually run, in order, output recorded.
- [ ] Blocked gates reported as blocked, with the reason.
- [ ] No test touched live data; any persistent test used an isolated directory.
- [ ] Ledgers and `./docs/cline/progress.md` updated.
