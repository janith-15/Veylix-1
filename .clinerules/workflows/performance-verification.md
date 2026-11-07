# WORKFLOW · PERFORMANCE VERIFICATION

Use when a change could move resource use, before a release, and whenever a claim
about speed or footprint is about to be made.

**The constraint is architectural, not a tuning note.** Veylix targets
**1 vCPU / 500 MB RAM**. `./docs/ARCHITECTURE.md` treats that as a design
boundary: no Redis, no PostgreSQL, no Docker, no persistent Node server. A change
that needs more headroom than this is a design change, and needs the owner.

**The rule that makes this workflow work: a number you did not measure is not a
result.** Every row is labelled `MEASURED` (command shown, output read) or
`ESTIMATED` (arithmetic from another measured number, and say from which). Never
present an estimate as a measurement. Never invent a figure — if you cannot
measure it, the verdict is `NOT RUN` with the reason.

---

## Stage 1 — Establish the baseline before optimising

You cannot report an improvement without a before. Record, in the change record:

- the commit-equivalent state (date + the files as read; **no Git here**),
- backend binary size,
- frontend chunk table,
- the notes from the previous run of this workflow in `./docs/cline/progress.md`.

**Do not attempt to "restore" a previous value by guessing.** If no baseline
exists, say so — the first run establishes it and reports no delta.

## Stage 2 — Backend memory (MEASURED)

From `./backend/`:

```
go build -o <tempdir>/veylixd-perf ./cmd/veylixd
```

Then measure the built binary's own size, and — only against an **isolated** data
directory — its resident set while idle:

- Windows: `Get-Process veylixd | Select-Object WorkingSet64,PrivateMemorySize64`
- Run with the isolated data dir and a throwaway DB; never `./var/`,
  never `./.veylix-secrets/`, never the production deployment.

Record: binary bytes, idle RSS, and peak RSS during a single authenticated
request. Threshold judgement: a **Go** server holding a SQLite handle should sit
in the low tens of MB idle; tens of MB above that is a finding worth naming.

## Stage 3 — Frontend bundle (MEASURED)

From `./frontend/`:

```
npm run build
```

Then read the sizes the build itself reports — do not recall them. Reference
point measured on 2026-11-07 from `frontend/dist/assets`:

| Artifact | Bytes | Note |
|---|---|---|
| `index-*.js` (main) | 321,995 | was 754,830 before route splitting |
| `threeScene-*.js` | 536,462 | already lazy and opt-in; the only >500 kB chunk |
| `ui-*.js` | 129,522 | shared UI chunk |
| all asset files (81) | 1,516,820 | gzip is roughly a third of this |

Findings to look for, in priority order:

1. A new **eager** import that pulls a page or a heavy library into `index-*`.
2. A chunk that grew without the change explaining why.
3. A new dependency. Justify it against the 500 MB target; prefer the existing
   25 UI primitives, 5 dependency-free charts and `AsyncState`.

An entry-chunk increase is a **P1** finding, not a preference.

## Stage 4 — Database query performance (MEASURED)

- `EXPLAIN QUERY PLAN` every statement the change adds or alters, on an
  **isolated** database, and paste the plan.
- A `SCAN` over a table that grows with users is a finding; say which table and
  which index is missing.
- Check for N+1 patterns: a query issued inside a loop over a result set.
- Confirm migration count and that a new migration does not copy a large table
  without need. (21 migration files existed as of 2026-11-07 — count them, do not
  quote this line.)

## Stage 5 — Background processes and dependencies (MEASURED)

- **One binary, one process.** Anything else running is a finding.
- Lists to check, by reading rather than assuming:
  - `./backend/go.mod` — new direct dependencies,
  - `./frontend/package.json` — new `dependencies` / `devDependencies`,
  - any new long-lived goroutine with a timer, ticker or poll loop,
  - any new cron-like sweep on a path that already runs frequently.
- Confirm no Node process is required in production (the frontend is embedded).

## Stage 6 — Startup and shutdown behaviour (MEASURED)

- Time to `/api/ready` from a cold start against the isolated data directory.
- Migrations applied on start: count and duration.
- **Clean shutdown**: the process exits on signal, closes the DB, and stops
  goroutines. A hang here is a P1 finding.
- Restart after shutdown returns the same readiness state — record it.

## Stage 7 — Report

Produce one table. **Every row carries a label.**

| Area | Metric | Value | Label | Source command | Verdict |
|---|---|---|---|---|---|
| Backend binary | size | … | MEASURED | `go build` output | PASS / FAIL / NOT RUN |
| Backend memory | idle RSS | … | MEASURED | `Get-Process` | … |
| Frontend entry | `index-*.js` | … | MEASURED | `npm run build` | … |
| Frontend heaviest | largest chunk | … | MEASURED | `ls dist/assets` | … |
| Database | query plans | … | MEASURED | `EXPLAIN QUERY PLAN` | … |
| Processes | extra processes | … | MEASURED | process list | … |
| Startup | time to ready | … | MEASURED | timing | … |
| Shutdown | clean exit | … | MEASURED | signal + exit | … |

Rules:

- A row you did not run is `NOT RUN` with the reason. It is never `PASS`.
- Do not run a load test against production, and do not load-test the
  development panel to the point of affecting anyone using it.
- If the only available evidence is an estimate, the Verdict column says
  `ESTIMATED` — not `PASS`.
- Append the table to `./docs/QA_REPORT.md` and a one-line summary to
  `./docs/cline/progress.md`.
