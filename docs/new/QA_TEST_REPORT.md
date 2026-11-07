# VEYLIX — QA TEST REPORT

**Run:** 2026-11-08 · **Baseline commit:** `1b09553` · **Machine:** Windows 11,
Go 1.27, Node 24.19.0

**Rule.** Every row is exactly one of **PASS** (command shown, output seen) ·
**FAIL** (output shown) · **BLOCKED** (reason shown) · **NOT RUN** (reason).
Nothing is rounded up. A gate that was not executed is never `PASS`.

---

## 1. Backend gates

| # | Command | Result | Evidence |
|---|---|---|---|
| 1 | `go build ./...` | **PASS** | exit `0`, no output |
| 2 | `go vet ./...` | **PASS** | exit `0`, no output |
| 3 | `gofmt -l internal/ cmd/` | **PASS** | no files listed |
| 4 | `go test ./... -count=1` | **PASS** | **17/17 packages ok**, 0 failures |
| 5 | `go test -race ./...` | **BLOCKED** | no C compiler on this machine; the pure-Go SQLite driver needs no CGO, but `-race` does. Never reported as PASS or FAIL. |

### Package-by-package (`go test ./... -count=1`)

```
ok  veylix/backend/cmd/vpn-platformctl   2.921s
ok  veylix/backend/internal/audit        1.698s
ok  veylix/backend/internal/auth        14.726s
ok  veylix/backend/internal/auth/google  3.975s
ok  veylix/backend/internal/config       1.340s
ok  veylix/backend/internal/domain       1.231s
ok  veylix/backend/internal/httpapi     69.090s
ok  veylix/backend/internal/integrations 3.768s
ok  veylix/backend/internal/mail         3.304s
ok  veylix/backend/internal/panel        6.505s
ok  veylix/backend/internal/payments     5.676s
ok  veylix/backend/internal/provisioning 13.284s
ok  veylix/backend/internal/releases     9.398s
ok  veylix/backend/internal/settings     3.707s
ok  veylix/backend/internal/store       17.335s
ok  veylix/backend/internal/sync         8.491s
ok  veylix/backend/internal/versioning   4.012s
```

**617 test functions across 15 packages** (`npm run docs:verify` counts the tree
independently and agrees).

---

## 2. Frontend gates

| # | Command | Result | Evidence |
|---|---|---|---|
| 6 | `npm run lint` (oxlint) | **PASS** | `Found 2 warnings and 0 errors.` — both warnings pre-existing, in a sidebar effect, not introduced by this pass |
| 7 | `npm run build` (`tsc -b && vite build`) | **PASS** | `built in 1.04s` |
| 8 | `npm run audit` | **PASS** | `UI truth audit: OK — no unwired/demo claims and no invented money literals in src/.` |
| 9 | `npm run audit:selftest` | **PASS** | `OK — 8 positive + 6 negative cases.` |
| 10 | `npm run docs:verify` | **PASS** | `tree: 23 migrations · 133 routes · 617 test functions in 15 packages` → `README matches the tree` |

**Why `audit` matters more than `build`.** A passing build proves the code
compiles, not that a screen is honest. `audit-ui-truth.mjs` is the gate that
rejects a page claiming a capability with no backing endpoint, and rejects invented
money literals. Its self-test proves the gate itself still catches 6 known-bad
patterns.

---

## 3. Browser behaviour gates

| # | Command | Result | Evidence |
|---|---|---|---|
| 11 | `npm run verify:nav` | **PASS** | `NAV OK — 74 passed, 0 failed` (re-run 2026-11-08 after the S5 proof) |
| 12 | `npm run verify:ui` | **PASS** | `consoleErrors: []`, `tableRows: 1`, `cardCount: 4`, `emptyStates: 0` |
| 13 | `npm run verify:client-vpns` | **PASS** | **19/19** against a populated account (client owns 3 VPNs) — see `DEV-011` |
| 14 | `npm run verify:client-vpns` (refusal path) | **PASS** | run as an account owning 0 → `REFUSED … Reporting PASS here would be a false claim.`, exit **3** |
| 15 | `npm run preflight` | **NOT RUN** | needs a running backend + seeded panel; must run before any release claim |

`verify:nav`, `verify:ui` and `verify:client-vpns` were all re-run on **2026-11-08**
against this exact tree. `verify:client-vpns` is new in this pass: it is the
committed proof that the client "My VPNs" page works against **real rows**, which
the S5 report had honestly recorded as NOT PROVEN.

**Row 14 matters as much as row 13.** A gate that passes on an empty account proves
nothing, so the script refuses that case with exit **3** instead of reporting a green
run. The refusal was executed and observed, not assumed.


---

## 4. Documentation gates

| # | Check | Result |
|---|---|---|
| 14 | `docs:verify` (README ↔ tree) | **PASS** — 23 / 133 / 617 / 15 |
| 15 | New docs valid UTF-8, no U+FFFD | **PASS** — verified byte-level for all files under `docs/new/` and `docs/restart/` |
| 16 | New docs have no BOM | **PASS** — checked `EF BB BF` prefix |

---

## 5. Data-safety verification

| Check | Result |
|---|---|
| No `var/` file deleted or modified | **PASS** — no write issued to `var/` |
| No row deleted except the 193 recorded I-37 cleanup | **PASS** — that deletion ran against a taken backup (`.veylix-secrets/data/veylix.pre-20261108-042952.db`, 782,336 bytes) and used a predicate the current producer cannot emit |
| Live database still readable after all work | **PASS** — 798,720 bytes, 23/23 migrations applied |
| Source archive verified complete | **PASS** — 636 files, 5.91 MB, 15/15 required files present |

---

## 6. What this report does NOT claim

- **No production verification.** There is no production deployment; every
  measurement above is against the local dev tree and the seeded dev database.
- **No HTTPS / TLS verification.** No domain, no certificate, nothing on 80/443.
- **No Linux verification.** This machine is Windows.
- **No 1 vCPU / 500 MB benchmark.** Windows working set is not a Linux VM, so
  any figure would be `ESTIMATED`, not measured.
- **No accessibility audit.** Focus rings on three components remain unverified
  (`U18`).
- **`go test -race` did not run.** See row 5.

Each of these is a genuine gap, not a formality. Any of them being reported as
`PASS` elsewhere should be treated as a defect in that report.
