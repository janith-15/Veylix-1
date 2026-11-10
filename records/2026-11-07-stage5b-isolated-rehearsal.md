# RECORD · 2026-11-07 · release-readiness Stage 5b, isolated rehearsal

**What was run:** `scripts/stage5b-rehearsal.ps1` — the 13 checks of
`release-readiness.md` Stage 5b, executed against the **real packaged binary**.

**Where it ran:** a throwaway directory under `$env:TEMP`, on port **18080**, with
keys **generated in memory**. `./var/`, `./.veylix-secrets/` and `./records/` were
never read from or written to.

---

## 1. Result

```
PASS=12   FAIL=0   BLOCKED=1   ESTIMATED=1   (of 14 rows)
exit 0
```

| # | Check | Verdict |
|---|---|---|
| 1 | Fresh install into an empty data directory | PASS |
| 2 | Database initialisation + migration count (21/21) | PASS |
| 3 | First Super Admin, no hard-coded password | PASS |
| 4 | Empty initial data | PASS |
| 5 | Secret generation (throwaway) | PASS |
| 5b | Secrets never logged | PASS |
| 6 | Configuration fail-closed | PASS |
| 7 | Panel URL SSRF refusal (7 targets) | PASS |
| 8 | HTTPS setup | **BLOCKED** |
| 9 | Migrations idempotent on a second start | PASS |
| 10 | Backup and restore (throwaway ×2) | PASS |
| 11 | Restart and recovery | PASS |
| 12 | Resource usage vs 1 vCPU / 500 MB | **ESTIMATED** |
| 13 | Deployment prerequisites (single binary + embedded UI) | PASS |

## 2. Data-safety proof (after the run)

| Assertion | Observed |
|---|---|
| Throwaway directories remaining | **0** |
| Files under `./var/` modified in the last hour | **none** |
| Files under `./.veylix-secrets/` modified in the last hour | **none** |
| Only path deleted | `$env:TEMP\veylix-5b-<stamp>` — the one the script created |

The delete is guarded in code: `Remove-Item -LiteralPath $Root -Recurse` runs only
when `$Root -like "$env:TEMP*"`.

## 3. Two findings the rehearsal produced

Both were **real**, both were **fixed**, and neither was visible before the run.

### 3a. The packaged binary was stale — 20 of 21 migrations applied

| Fact | Value |
|---|---|
| `backend/bin/veylixd.exe` built | `2026-11-04 02:04` |
| `0021_dedupe_carrier_offer.sql` dated | `2026-11-04 03:38` |
| `schema_version` the old binary reported | **20** |
| Migration files on disk | **21** |

This is a **stale build artifact**, not a product defect: migrations are embedded
at compile time (`//go:embed migrations/*.sql`), so a binary older than a
migration cannot contain it. Fixed by rebuilding. The script now compares the
binary's mtime against the newest migration and, when the binary is older, reports
`STALE BINARY: built <t>, newest migration <t>` — so this can never again be
mistaken for a schema bug.

**After rebuild:** `schema_version=21` matches 21 files.

### 3b. The embedded UI was stale — 7 files instead of 85

`/api/ready` reported `ui: embedded:7_files`. `backend/internal/httpapi/dist/`
held **7** files, which is the placeholder set plus a stale fragment — not a built
app. The check `ui != not_embedded` alone would have **passed** and hidden it.

Fixed by running the project's own packaging command:

```
.\dev.ps1 -Package
  embedded 84 file(s)
  embedded UI: no development-login markers (ok)
  /api/ready  -> ready  (ui: embedded:85_files)
```

The rehearsal now reports the **file count** in its evidence line, so a shrunken
bundle is visible rather than merely "embedded".

## 4. What this run does NOT claim

| Item | Status | Reason |
|---|---|---|
| HTTPS / ACME (Bible 33, 34) | **BLOCKED** | no public domain; cannot bind 80/443 |
| Clean **Linux** VM install (Bible 30) | **BLOCKED** | this is Windows; no Linux host available |
| 1 vCPU / 500 MB benchmark (Bible 28) | **ESTIMATED** | Windows working set is not a Linux VM measurement |
| `go test -race` | **BLOCKED** | no C compiler |
| Live panel writes (D12) | **BLOCKED** | needs a non-production inbound (D-018) |
| Live Google tenant (D13) | **BLOCKED** | needs real client credentials |

A `BLOCKED` row is never a `PASS`, and this record does not round it up.

## 5. Gates re-run after the doc changes

| Gate | Result |
|---|---|
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `gofmt -l .` | clean |
| `npm run docs:verify` | OK — 21 migrations · 125 routes · 518 test functions |
| `npm run audit` | OK |
| `npm run lint` | 0 warnings / 0 errors |

## 6. Files added or changed by this pass

| File | Kind |
|---|---|
| `scripts/stage5b-rehearsal.ps1` | **new** — the isolated rehearsal |
| `backend/bin/veylixd.exe` | rebuilt (stale → current, 20.4 MB, `embedded:85_files`) |
| `backend/internal/httpapi/dist/` | rebuilt (7 → 85 files) |
| `docs/QA_REPORT.md` | §3.2 added |
| `docs/IMPROVEMENT_LEDGER.md` | pass 9 (I-08, I-09) |
| `docs/INSTALL.md` §F | row 12/13 evidence updated |
| `docs/cline/progress.md`, `activeContext.md` | session record |

**No application source changed.** No route, no migration, no business rule, no
frontend component. The only edits under `backend/` are the rebuilt `bin/` and
`dist/` artifacts.
