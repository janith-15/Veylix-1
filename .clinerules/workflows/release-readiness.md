# WORKFLOW · RELEASE READINESS

Use before building a release artifact or declaring the app shippable.

The rule that makes this workflow work: **a green build is not a release proof.**
Every stage below must produce observed output. A stage you cannot run is
`BLOCKED`, not skipped.

---

## Stage 1 — State of the tree

- `./docs/cline/activeContext.md` and `./docs/cline/progress.md` read.
- Any work-in-progress noted, or explicitly declared abandoned.
- **No Git here.** Do not run `git status`. Establish cleanliness by reading the
  files you changed and confirming nothing is half-written.

**Checkpoint:** the tree is either complete or the gap is named.

## Stage 2 — Backend gates

From `./backend/`:

```
go build ./...
go vet ./...
go test ./... -count=1
gofmt -l .
```

- `gofmt -l .` must print nothing. Formatting drift has been found here before —
  do not assume it is clean.
- `go test -race` → **BLOCKED** (no C compiler). State the reason.

**Checkpoint:** all four outputs recorded.

## Stage 3 — Frontend gates

From `./frontend/`:

```
npm run lint
npm run build
npm run audit
npm run audit:selftest
npm run docs:verify
```

**Checkpoint:** all five outputs recorded. `audit` must be genuinely clean — a
known false positive gets explained, not ignored.

## Stage 4 — Single-binary proof

```
.\dev.ps1 -Package
```

Then confirm, as the packaging step itself claims:

- `/api/ready` reports the UI as embedded,
- a real page route answers `200`,
- an unknown asset path stays `404`,
- the bundle contains no development bypass.

**Checkpoint:** the packaged binary was actually exercised, not just built.

## Stage 5 — Preflight gate

```
.\dev.ps1 -Preflight
```

This is the full end-to-end check (roles, refusals, logout, proxy). Record the
count and the result exactly as printed.

If the preflight needs a live panel or seeded data, confirm it runs against the
**development** dataset only — never live user data.

## Stage 5b — Fresh-installation and deployment QA

**This stage is the single owner of the fresh-install check list.** No other
workflow, document or script may keep a second copy of these 13 checks —
`installation-and-deployment.md` and `skills/veylix-deployment` both point here
rather than restating them. A duplicated checklist drifts, and a drifted
checklist is how a claim outlives its evidence.

Stages 2–5 prove the build works on **this** machine with **this** data. They do
not prove a new operator can install it. This stage does, and it is the one that
most often gets skipped — so it is a required stage, not an optional extra.

**Hard rule: the entire stage runs in a throwaway environment.**
`VPN_FIX_LEDGER.md` records a verification script that deleted live data, twice.
Nothing here may read or write `./var/`, `./.veylix-secrets/`, `./records/`, or
any existing database. Use a fresh temp directory as the data root and a fresh
temp path for generated secrets, and delete only that directory afterwards.

Verify, each with observed output:

| # | Check | Evidence required |
|---|---|---|
| 1 | **Fresh install** into an empty data directory | server starts, `/api/ready` reports ready |
| 2 | **Database initialization** | the DB file is created; migration count applied matches the migration directory count (count the files, do not quote a number) |
| 3 | **First Super Admin setup** | the bootstrap/first-run path creates exactly one Super Admin; a second bootstrap attempt is refused, not silently duplicated |
| 4 | **Empty initial data** | the UI and API report empty, not zero-filled fake rows and not an error |
| 5 | **Secret generation** | new secrets are generated into the throwaway secrets path only; nothing is written into the repo |
| 6 | **Configuration setup** | config is read from the documented location; a missing required value fails loudly with a clear message |
| 7 | **Panel URL configuration** | an operator-supplied panel URL is accepted only if it passes the SSRF guard; a loopback/private/link-local/metadata target is **refused** — show the refusal |
| 8 | **HTTPS setup** | the documented TLS path works, or the absence of TLS is an explicit, documented, operator-visible decision — never a silent default |
| 9 | **Database migrations from empty** | a fresh DB reaches the same schema version as the existing one; migrations are idempotent on a second start |
| 10 | **Backup and restore** | back up the throwaway DB, mutate nothing, restore into a **second** throwaway path, and confirm the restored copy opens and serves |
| 11 | **Restart and recovery** | stop and start: readiness returns, no data is lost, no migration re-runs destructively |
| 12 | **Resource usage** | idle RSS and binary size recorded per `performance-verification.md`; compared against 1 vCPU / 500 MB |
| 13 | **Deployment prerequisites** | Go/Node build-time story, the single-binary + embedded-frontend claim, and the documented prerequisites in `./docs/INSTALL.md` actually match what the binary needs |

Rules for this stage:

- A check you did not perform is `NOT RUN` with the reason. Never `PASS`.
- Never rehearse a destructive restore against live data to prove a restore
  works — use the throwaway path.
- If the install path requires a panel to be reachable and none is isolated,
  report the affected checks as `BLOCKED` and say which.
- Record the throwaway paths used, so a later reader can confirm nothing live
  was touched.

## Stage 6 — Documentation truth

- `npm run docs:verify` passed (Stage 3).
- `./README.md` status table is truthful: nothing claims to work that Stage 2–5
  did not demonstrate.
- `./docs/QA_REPORT.md` updated with the commands actually run in this pass.
- `./docs/DISCREPANCY_REGISTER.md` reflects anything still broken.
- Capability that is configured but unverified is labelled as such — never
  "working".

**Checkpoint:** no document overstates what was proven.

## Stage 7 — Rollback note

Write, for this release:

- the artifact identifier (build or version),
- the migration range it introduces,
- how to return to the previous state,
- whether a backup is required before deploying.

Do not rehearse a destructive restore against live data to prove this.

## Stage 8 — Truthful status table

Produce, in the report and in `./docs/cline/progress.md`:

| Gate | Verdict | Evidence |
|---|---|---|
| `go build` | PASS / FAIL / BLOCKED / NOT RUN | output |
| `go vet` | … | … |
| `go test` | … | … |
| `go test -race` | BLOCKED (no C compiler) | reason |
| `gofmt -l .` | … | … |
| `npm run lint` | … | … |
| `npm run build` | … | … |
| `npm run audit` | … | … |
| `docs:verify` | … | … |
| `-Package` | … | … |
| `-Preflight` | … | … |
| **5b** fresh install (isolated) | … | … |
| **5b** DB init + migration count | … | … |
| **5b** first Super Admin | … | … |
| **5b** empty initial data | … | … |
| **5b** secret generation (throwaway) | … | … |
| **5b** panel URL SSRF refusal | … | … |
| **5b** backup + restore (throwaway) | … | … |
| **5b** restart + recovery | … | … |
| **5b** resource usage vs 1 vCPU / 500 MB | … | … |

**Release is "ready" only when every row is PASS or an owner-accepted BLOCKED.**
One FAIL means not ready, regardless of everything else being green.
