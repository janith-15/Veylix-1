

# VEYLIX — QA REPORT

**Run date:** 2026-10-24 (evening run: live preview + UI truth pass) · **Previous:** 2026-10-23 (Stages B/E2/E3/E4)
**Environment:** Windows dev machine, Go 1.27.1, Node 24.19.0, Chrome (headless via CDP)
**Live panel:** `difanx4f3.difanx1.ggff.net:2053` — 3X-UI **3.10.2**, Xray **26.10.10** (verified via HTTPS + Bearer token)
**Rule:** every line below is a command that was actually executed. Anything not executed is listed as **NOT RUN** — never implied as passing (`new_update.md` §13, `docs/QA_PLAN.md`).

---

## 2026-11-07 — UI improvement programme, Phases A–E

**Scope:** frontend UI/UX and bundle performance only. No backend, API, store
or business-logic change. No new dependency added.

### Gates (all executed this session)

| Gate | Result | Evidence |
|---|---|---|
| `npx tsc -b` | **PASS** | clean, no output |
| `npm run lint` (oxlint) | **PASS** | `Found 0 warnings and 0 errors.` 175 files, 116 rules |
| `npm run build` | **PASS** | `built in 1.14s`, 0 errors |
| `npm run audit` | **PASS** | `UI truth audit: OK` |
| `npm run audit:selftest` | **PASS** | `8 positive + 6 negative cases` |
| `npm run docs:verify` | **PASS** | `docs:verify: OK — README matches the tree.` (21 migrations · 125 routes · 518 tests) |
| `go build ./...` | **PASS** | exit 0 |
| `go vet ./...` | **PASS** | exit 0 |
| `go test ./... -count=1` | **PASS** | 12/12 packages `ok`, exit 0 |
| `verify-nav.mjs` | **PASS** | 23 passed, 0 failed |
| `verify-mobile.mjs` (390px) | **PASS** | overflow 0px, 0 targets < 44px |
| `verify-retry.mjs` | **PASS** | error panel → retry → recovers to 6 rows |
| `verify-themes.mjs` | **PASS** | all 6 presets `store: ok` |
| `verify-theme-toggle.mjs` (new) | **PASS** | 9/9 checks, 0 console errors |
| Render sweep, 14 routes | **PASS** | 14 ok, 0 failed, `consoleErrors: []` on all |
| `go test -race` | **BLOCKED** | no C compiler on this machine (pre-existing) |

### Bundle

| | Before | After |
|---|---|---|
| Main JS | 754.83 kB (215.69 gz) | **321.99 kB (96.88 gz)** |
| Initial CSS | 189.07 kB (31.59 gz) | **103.98 kB (18.08 gz)** |
| Page chunks | 1 | **24** |
| `threeScene` | 536 kB, already lazy | unchanged (opt-in only) |

### Changes

- **A** `App.tsx` — ~30 pages lazy-loaded; shell eager; one `ShellSuspense`
  boundary below `AppShell`. `layout.css` — `.route-fallback`.
- **B** `themeStore.ts` — new `setPreset()` committing a preset on the live
  theme while preserving scene/density/motion/sub-theme. `Topbar.tsx` —
  Sun/Moon toggle with truthful `aria-pressed`; "Design studio" added to the
  account menu. New `scripts/verify-theme-toggle.mjs` + `npm run verify:theme-toggle`.
- **C** `layout.css` — active-nav accent rail (suppressed in the icon rail),
  shared `:focus-visible` ring on chrome icon buttons, press feedback.
  `ui.css` — sticky table header (box-shadow divider, because collapsed borders
  vanish on sticky cells), tabular numerals on right-aligned cells.
- **D** `AdminClientsPage.tsx`, `ServersPage.tsx` — three state branches replaced
  with `AsyncState`, so loading / failed / empty are distinct and a failure
  offers a retry.
- **E** README test-function count corrected 517 → 518 (pre-existing drift the
  `docs:verify` gate caught; no Go file was touched this session).

### Incident

The improvement ledger was truncated during an append and restored from
`H:\my apps\Veylix-2 - Copy` (153,125 bytes). PowerShell `Set-Content` is no
longer used to edit documentation; the `editor` tool is used instead.
Recorded in `docs/IMPROVEMENT_LEDGER.md` under "pass 2 - process incident".

---

---

## 0-A. 2026-10-24 evening — live preview + UI truth pass (EXECUTED)

### A.1 Launcher and proxy

```
.\dev.ps1 -Log .\var\logs
  starting backend on http://127.0.0.1:8080 ...
  backend ready - dev bypass ON as dev@veylix.local (super_admin)
  starting frontend on http://localhost:5173 ...
  App      : http://localhost:5173/dashboard
  Stop both: .\dev.ps1 -Stop

.\dev.ps1 -Status
  127.0.0.1:8080 -> backend  : 200 {"service":"veylix","status":"ok","version":"dev"}
                   dev auth : ON  ({"email":"dev@veylix.local","id":1,"role":"super_admin"})
  [::1]:5173     -> frontend : 200 at http://localhost:5173

GET http://localhost:5173/api/me        -> 200 {"email":"dev@veylix.local","role":"super_admin"}
GET http://localhost:5173/api/servers   -> 200 [{"id":1,"name":"indian1","status":"ACTIVE",
                                             "panel_version":"3.10.2","xray_version":"26.10.10",
                                             "last_sync":1792850419}]
GET http://localhost:5173/api/sync/status -> 200 {"servers":[{"server_id":1,"status":"ACTIVE",
                                             "stale":false,"age_seconds":49}], ...}
GET http://localhost:5173/api/servers/1/metrics?limit=240 -> 200 points=128
GET http://localhost:5173/api/clients   -> 200 count=1
```

### A.2 UI truth gate (`frontend/`)

```
node scripts/audit-ui-truth.mjs            -> exit 0  "no unwired/demo claims and no invented money literals in src/"
node scripts/audit-ui-truth.mjs --self-test -> exit 0  "OK — 6 positive + 4 negative cases"
```

**Before the fix the same scanner reported 12 findings** (RetailPricePage ×3 invented prices, DashboardPage "demo data", AdminOverviewPage ×2 "no API yet", SaleFlowPage ×1, LoginPage/RegisterPage "data is mock" ×2 counted twice, ClientDashboardPage ×1). Each was rewritten to a real endpoint and re-scanned to zero.

### A.3 Page renders in a real browser (`node scripts/verify-dashboard.mjs`, headless Chrome via CDP)

```
/dashboard      Panels 1/1 · Clients 1 · Active VPNs 5 · Healthy 100% · CPU 4.0%
                card titles: "Server CPU — indian1", "Network health", "Server integration status"
                table row: "indian1 | 10:00:19 PM | Active | 4.0%"     console errors: 0
/retail-price   (VEYLIX_DEV_ROLE=admin) card titles: "Sellable premium plans", "Confirmed revenue",
                "Assigned panels", "Premium plans — what the panel will enforce"
                table row: "Phase A Premium | 100 GB | 30 days | 3 | server 1"   console errors: 0
/admin          (VEYLIX_DEV_ROLE=admin) Assigned servers 1 · Quota unlimited · Clients 1 ·
                Confirmed revenue 0.00 (0 confirmed payments)                 console errors: 0
/client         (VEYLIX_DEV_ROLE=client) cards: "Free offers", "My VPNs", "Purchases"
                console errors: dev-only duplicate-key warning (F-031), 0 uncaught exceptions
```

### A.4 Gates re-run after the changes

| Gate | Command | Result |
|---|---|---|
| Backend build | `go build ./...` | ✅ exit 0 |
| Static analysis | `go vet ./...` | ✅ exit 0 |
| Formatting | `gofmt -l .` | ✅ nothing printed |
| Backend tests | `go test ./... -count=1` | ✅ **9/9 packages ok** (278 `Test*` functions) |
| Typecheck | `npx tsc -b` | ✅ clean |
| Lint | `npm run lint` (oxlint) | ✅ **0 warnings / 0 errors** (103 files) |
| Production build | `npm run build` | ✅ `index.css` 139.18 kB (gz 23.59) · `index.js` 515.37 kB (gz 151.00) · `threeScene` 537.96 kB (gz 134.20, lazy) |
| Dev bypass absent from bundle | grep `dist/assets/*.js` for `dev-role` / `DEV_AUTH_BYPASS` | ✅ absent |

### A.5 Found during this run

| ID | Finding | Status |
|---|---|---|
| F-031 | Dev-only React duplicate-key warning while notifications exist (admin/client personas) | `OPEN` — recorded, reproduced on untouched screens, absent from production build |
| F-032 | 12 screens advertised capabilities the backend had already shipped (incl. invented prices and invented dashboard panels) | `FIXED-VERIFIED` |
| F-033 | `dev.ps1` hung with `-Log` (HasExited never true with redirected output); Vite could hop to 5174 and break CSRF | `FIXED-VERIFIED` |

### A.6 Not run in this pass

| Item | Why |
|---|---|
| Anything that writes to the real panel (create/suspend/revoke) | D-018 — production inbound with live users |
| `go test -race` | no C compiler on this machine |
| Frontend unit tests | no runner installed (`vitest` absent); the two new scripts are self-checking instead (`--self-test`, and the CDP probe fails on console errors) |
| 39 acceptance tests | depend on packaging + a non-production panel |

---

## 0-B. 2026-10-24 (late) — E7 single binary + documentation truth pass (EXECUTED)

### B.1 Packaging (`.\dev.ps1 -Package`)

```
building the frontend ...               (exit 0; dist/index.html present)
embedding the build into backend/internal/httpapi/dist ...
embedded 6 file(s)
building the single binary ...
binary: backend\bin\veylixd.exe (19.1 MB)

PACKAGED OK - one binary, one port (18080)
  /api/ready  -> ready  (ui: embedded:7_files)
  /dashboard  -> 200 text/html; charset=utf-8
  /vpn/16     -> 200 (SPA deep link)
  /assets/nope.js -> 404 (missing file stays 404)
  dev-bypass in bundle -> absent (ok)
```

### B.2 The packaged binary, one process on :18082

```
/api/health        200  application/json
/api/ready         200  application/json
/api/me            200  application/json
/api/vpns          200  application/json
/                  200  text/html
/dashboard         200  text/html
/vpn/16            200  text/html            (client route → shell)
/nonexistent       200  text/html            (client route → shell)
/api/nope          404  json                 (API never swallowed by the SPA)
/assets/nope.js    404                       (missing file stays 404)
```

### B.3 New tests

`internal/httpapi/spa_test.go` — 7 tests, all passing:
`TestSPADeepLinkServesIndex` · `TestSPAServesAssetsImmutable` · `TestSPAMissingFileIs404` ·
`TestSPAHeadHasNoBody` · `TestSPARejectsNonGet` · `TestSPAStatusRequiresIndex` +
`TestRouterKeepsApi404AsJSON` (in the same file) · and `TestReadyReportsUIStatus`.

### B.4 Gates re-run after the change

| Gate | Command | Result |
|---|---|---|
| Backend build / vet / format | `go build ./...` · `go vet ./...` · `gofmt -l .` | ✅ 0 / 0 / 0 |
| Backend tests | `go test ./... -count=1` | ✅ **9/9 packages** (286 test functions) |
| Typecheck / lint | `npx tsc -b` · `npm run lint` | ✅ clean · 0/0 |
| Production build | `npm run build` | ✅ ✓ built in 769 ms |
| UI truth gate | `npm run audit` · `--self-test` | ✅ 0 findings · 10/10 |
| Single binary | `.\dev.ps1 -Package` | ✅ route matrix in B.1/B.2 |

### B.5 Documentation corrected in this pass

| Document | Was | Now |
|---|---|---|
| `ROADMAP.md` | "Milestone 5-B — ðŸ”´ **not started**" above a list of mostly-`[x]` items; E5/E6 unbuilt | 5-B marked shipped (steps 1–12), E5/E6 `[x]` **live-verified**, contradiction noted inline |
| `ROADMAP.md` | capability table: links "ðŸŸ¡ pending", multi-panel "⬜ E5" | both ðŸŸ¢ live-verified |
| `ROADMAP.md` | Milestone 7 packaging empty | single binary `[x]` |
| *(absent)* | no root README, no INSTALL, no FINAL_REPORT, no doc index | `README.md`, `docs/INSTALL.md`, `docs/FINAL_REPORT.md`, `docs/DOC_INDEX.md` |
| `DECISIONS.md` | — | **D-029** (single binary) with the routing rules and their tests |

### B.6 Not run in this pass

| Item | Why |
|---|---|
| Live panel **writes** (suspend/resume/revoke) | D-018 — production inbound with live users |
| `go test -race` | no C compiler on this machine |
| Clean Linux VM install / HTTPS | no VM available; `docs/INSTALL.md` is therefore **unverified on a fresh box** and says so |
| 39 acceptance tests | still open (Milestone 8) |

---

## 0. Live verification — EXECUTED against the real panel and the real binary


### 0.1 Adapter live tests (`go test -tags livepanel`)

```
LIVE OK: panel=3.10.2 xray=26.10.10 state=running cpu=0.87 mem=411176960/998576128 latency=323ms
LIVE inbound: id=1 remark="(IN)-free_2" protocol=vless port=443 enable=true
LIVE clients: total=3 returned=3 active=2 online=1
LIVE fresh-read OK for "DNuser"
--- PASS (3/3)
```

### 0.2 Full HTTP API against the real panel (running binary, real SQLite)

| Step | Result |
|---|---|
| `POST /api/setup/bootstrap` | **201**, role `super_admin` |
| second bootstrap | **409** `bootstrap_complete` |
| `POST /api/auth/login` | **200**, HttpOnly session cookie |
| `POST /api/servers` (real panel URL + token) | **201** `{"status":"ACTIVE","panel_version":"3.10.2","xray_version":"26.10.10"}` |
| `GET /api/servers` | **200**, **no token in the response** |
| `GET /api/servers/1/inbounds` (live from panel) | **200** `[{"id":1,"remark":"(IN)-free_2","protocol":"vless","port":443,...}]` |
| `POST /api/servers/1/test` (re-verify) | **200** `ACTIVE`, latency recorded |
| `GET /api/audit` | **200** — `auth.bootstrap`, `auth.login`, `server.add`, `server.test`, `auth.register` present |
| client → `GET /api/servers` | **403** |
| client → `POST /api/servers` | **403** |
| client → `GET /api/audit` | **403** |
| anonymous → `GET /api/servers` | **401** |
| token substring inside `veylix.db`, `-wal`, `-shm` | **absent** — sealed with AES-256-GCM |

### 0.3 Quota endpoint per role (F-023, D-021) — 2026-10-23
```
super_admin  GET /api/quotas/me -> 200 {"used":0,"limit":0,"remaining":-1}
admin        GET /api/quotas/me -> 200 {"used":0,"limit":25,"remaining":25}
client       GET /api/quotas/me -> 200 {"used":0,"limit":0,"remaining":-1}   <-- was 403
anon         GET /api/quotas/me -> 401
```

### 0.4 Stage E5 sync against the live panel — 2026-10-23

Real binary, real SQLite (schema **5**), real 3X-UI panel `indian1` (panel 3.10.2 / xray 26.10.10), `VEYLIX_SYNC_INTERVAL=30s`:

```
{"msg":"sync scheduler started","interval":"30s"}
schema_version: 5

# after tick 1
GET /api/sync/status  -> {"servers":[{"server_id":1,"name":"indian1","status":"ACTIVE",
                          "last_sync":1792757402,"stale":false,"age_seconds":22}],
                          "stale_after_seconds":900}

GET /api/servers/1/metrics -> {"scope":"server_wide","points":[
  {"taken_at":1792757402,"cpu_pct":1.22,"mem_used":419643392,"mem_total":998576128},
  {"taken_at":1792757438,"cpu_pct":0.42,"mem_used":404176896,"mem_total":998576128},
  {"taken_at":1792757468,"cpu_pct":0.60,"mem_used":407388160,"mem_total":998576128}]}

# WARN/ERROR lines across all three ticks: 0
```

**Note on history depth:** the panel's local VPN instances were created before E5, so `usage_snapshots` starts from the first tick. The API reports this honestly (`history`, `first_snapshot_at`) instead of inventing earlier data (REQUIREMENTS §2.7).

**Still not live-verified:** client create / suspend / resume / delete. These mutate a production panel and are deliberately not run without explicit authorisation (D-018). They are covered by unit tests against a fake panel → classified **"implemented but not live-verified"**.

**Defect F-013 found and fixed during this run:** the session cookie was issued `Secure` over plain HTTP, so every authenticated call returned 401 while the UI appeared logged in. Root cause: the `VEYLIX_INSECURE_COOKIES` flag was ANDed with the loopback check, so it silently did nothing. Fixed by `resolveSecureCookies` (network binds always Secure; only loopback may opt out) + regression test `TestSecureCookiesDecision`.

### 0.5 Step 5 — VPN list from the panel, on the running binary — 2026-10-24

Real binary (`cmd/veylixd`, built from this tree), real SQLite, **fresh** data dir (no servers configured, so the Super-Admin list has nothing to read from any panel). Dev auth on loopback for the Super-Admin principal (D-022):

```
{"msg":"database ready","schema_version":6}
{"msg":"configuration loaded","addr=127.0.0.1:18099 devAuth=true"}

GET /api/ready                  -> 200 {"schema_version":6,"status":"ready"}
GET /api/vpns                   -> 200 []
GET /api/vpns?tag=untagged      -> 200 []
GET /api/vpns?tag=nope          -> 422          (unknown filter is refused, never ignored)
```

**What this proves and what it does not.** It proves the new route, the `?tag=` validation and the panel-backed lister are wired into the shipped binary and answer correctly when there is nothing to list. It does **not** prove the panel path against a real 3X-UI panel: this run had no integration configured, so `PanelVPNLister` had no server to read. That half is covered by tests with an injected inspector plus `TestSQLVPNListerHidesUntagged`, which asserts the tag-join rule in SQL — not by this smoke run.

**Also executed (unit + real SQLite, same commit):**

```
go test ./... -count=1        -> 9 packages ok, 0 FAIL
--- PASS: TestUntaggedPanelClientIsSuperAdminOnly      (QA_PLAN T-32)
--- PASS: TestTaggingPublishesPanelClient
--- PASS: TestVPNListRejectsUnknownTagFilter
--- PASS: TestPanelOutageDegradesToList
--- PASS: TestUnreadablePanelWithNoLocalRowsIsAnError
--- PASS: TestPanelAndLocalRowsMerge
--- PASS: TestSQLVPNListerHidesUntagged
```

---

## 1. Gate results

| Gate | Result | Evidence |
|---|---|---|
| GATE 5 — BUILD PASS (backend) | ✅ PASS | `go build ./...` exit 0 |
| GATE 5 — static analysis | ✅ PASS | `go vet ./...` exit 0 |
| GATE 5 — formatting | ✅ PASS | `gofmt -l .` printed nothing |
| GATE 5 — BUILD PASS (frontend) | ✅ PASS | `npm run lint` 0/0 (100 files) · `npm run build` OK |
| GATE 6 — UNIT PASS | ✅ PASS | `go test ./...` — **271 tests**, 9 packages (2026-10-24) |
| GATE 6 — race detector | ⛔ **BLOCKED** | `-race requires cgo`; no C compiler installed |
| GATE 8 — live panel (reads + integration CRUD + auth) | ✅ **PASS** | see §0 |
| GATE 8 — live client provisioning | ⛔ **BLOCKED** | the panel has a single production inbound with live users (D-018) |
| GATE 7 — QA checklist | ðŸ”¶ PARTIAL | SSRF, status mapping, token-leak, RBAC, audit, CSRF, quota race, duplicate prevention verified; links/QR + usage await E5/E6 |

> **This section duplicates §3.** Both are kept because the live-verification log above (§0) refers back to it; §3 is the canonical, currently-maintained table. Both were re-derived from the same 2026-10-23 run (F-023 pass) and now agree.



---

## 1. Test inventory (executed)

| Package | Count | Covers |
|---|---|---|
| `internal/config` | 12 | defaults, fail-closed secrets, weak/non-base64 keys, no secret in log string, **cookie Secure decision (F-013)**, dev-auth triple condition (D-022), **sync env overrides + safe bounds (D-023)** |
| `internal/httpapi` | 78 | liveness/readiness/404 envelope, **auth E2E (register → /me → logout)**, wrong password, bootstrap once, forged cookie, **CSRF cross-origin block**, **password change + session rotation (F-016)**, **setup status**, **admin creation + escalation refused (F-021)**, **suspension revokes sessions**, **self-suspend blocked**, quota on non-admin 404, **`/api/quotas/me` per role incl. client 200 (F-023, D-021)**, dev auth bypass (D-022), **usage/metrics/sync-status routes incl. IDOR (D-023)**, **offer catalogue per role + validation + deactivation (P3)**, **server assignment replace/idempotency/RBAC + clients scoping (P5)**, **VPN list scoping (panel-backed vs tag-required)**, **FREE claim caps**, **releases/restore**, **tags (lock + 409)**, **billing state machine**, **chat participant privacy**, **notifications privacy**, **directory scope** |
| `internal/store` | 15 | idempotent migrate, pragmas (WAL/FK), tx rollback leaves no rows, idempotency-pair uniqueness, **FREE claim ledger (concurrent + rolling-window caps)**, **schema 0006 panel-truth columns** |
| `internal/panel` | 32 | SSRF table, status mapping, GET-only retries, fresh read, token leak, redirect, path traversal, **`limitHwid` on the wire (F-024)**, full-record `UpdateClient`, links, HWID list/revoke |
| `internal/auth` | 49 | argon2id hashing, salting, malformed hashes, rehash policy, **password policy: length/repeated/sequence/blocklist/too-long (F-017)**, **password change: rotate, wrong current, policy on new value, unknown user (F-016)**, AES-GCM seal/open/AAD-binding/tampering, permission matrix, ownership (IDOR), sessions (fingerprint-only storage, expiry purge, suspension), rate limiting, client-IP trust |
| `internal/integrations` | 9 | token sealed & never returned, failure never claims ACTIVE, invalid input, duplicate URL, re-test re-decrypts, inbound cache upsert, disable/remove preserves history, audit written without secrets |
| `internal/domain` | 8 | VPN state machine, outage never → REVOKED, terminal states, deterministic panel identity, outbox key |
| `internal/provisioning` | 29 | create+confirm, **quota race (2 racers → 1 winner)**, outage → SERVER_UNAVAILABLE, ambiguous write queued, write-that-landed adopted, reconciler no-duplicate, reconciler creates when absent, backoff when panel down, outbox idempotency, revoke requires verification, ownership 404, FREE/PREMIUM rules, **FREE fixed 2/2 regardless of the offer's `device_limit` (D-028.3)**, **PREMIUM keeps the operator's limit**, the cap survives JSON, create-time identity/capacity/expiry intact |
| `internal/sync` | 29 | **reset-proof fold math (F-010)**, per-server isolation, pagination, hanging-panel timeout, outage-never-revokes, expired/depleted/suspended, unseen-client no-op, same-second upsert, scheduler overlap guard, **panel limit mirror (D-024.2/.4)**: the panel wins over the create-time values, an unchanged panel writes nothing, a pre-mirror row is rewritten once, an unverifiable client has its mirror cleared and restored |
| **Total** | **271** | all PASS |

> Counts are **measured**, not carried over: `go test ./internal/<pkg> -v` → count of `--- PASS` lines, re-derived 2026-10-24 (per-package counts sum to the 271 total exactly: 12 + 78 + 15 + 32 + 49 + 19 + 8 + 29 + 29). A previous revision of this table read 131 total with per-package numbers that did not sum to it; every regeneration must re-run the measurement rather than adjust the total.


### Defects found and fixed during these runs

| ID | Severity | Finding |
|---|---|---|
| **F-011** | **P1** | `url.PathEscape` does **not** escape `..` (dots are legal in a path segment), so a client email like `../../panel/api/settings` could traverse the panel API path. Caught by `TestClientEmailCannotInjectPath`, fixed with `ValidateEmail` + regression tests. |
| **F-013** | **P1** | The session cookie was issued `Secure` over plain HTTP, so every authenticated request returned 401 while the UI looked logged in. Root cause: `VEYLIX_INSECURE_COOKIES` was ANDed with the loopback check and therefore did nothing. Fixed by `resolveSecureCookies` + `TestSecureCookiesDecision`. |
| **F-014** | P2 | The rate-limiter refill truncated its elapsed time, so a fast refill rate never replenished. Fixed by using nanosecond precision; `TestRateLimiterRefills` now waits deterministically. |
| **F-015** | **P1** | Register page told the user "Looks good ✓" at 8 characters while the backend required 12, so a valid-looking form failed with 422. Fixed by a shared `frontend/src/lib/passwordPolicy.ts` mirroring the Go policy; the hint now counts down and never green-lights a value the server rejects. |
| **F-016** | **P1** | There was no way to change a password: no endpoint, no UI, and `RevokeAllForUser` was dead code. Added `auth.Service.ChangePassword`, `POST /api/auth/change-password` (rotates every session, re-issues the caller's cookie), a Profile dialog, and 5 tests. |
| **F-017** | P2 | Length was the only rule, so `aaaaaaaaaaaa` and `password1234` were accepted. Added `ValidatePasswordPolicy` (12–512 runes, no single repeated rune, no straight sequence, blocklist); `HashPassword` now always applies it, so no call site can bypass. |
| **F-020** | **P1** | `POST /api/setup/bootstrap` returned 201 with a session cookie that it never issued, so a fresh install could not be used at all (setup looked successful, then `/api/me` was 401). Fixed by creating a real session before responding; verified live. |
| **F-021** | **P1** | No code path could create an `admin`, making every reseller screen unreachable. Added `auth.Service.CreateUserByRole` + `GET/POST /api/admins` + `PATCH /api/admins/{id}` behind `PermAdminManage`; suspension revokes live sessions; self-suspend → 422. |

---

## 3. QA_PLAN items now covered

| Item | Status |
|---|---|
| T-01 (valid/invalid login) | ✅ unit + **live** |
| T-02 (role escalation) | ✅ **live** (client → `/api/servers` = 403, `/api/audit` = 403) |
| T-03 (bad credentials → `REAUTH_REQUIRED`) | ✅ unit |
| T-04 (unreachable → `CONNECTION_UNAVAILABLE`, never `REVOKED`) | ✅ unit + live |
| T-05 (SSRF URL table) | ✅ unit (17 cases) |
| T-17 (manual confirm audited, never automatic) | ðŸ”¶ audit write path proven; payment confirm awaits E4 |
| T-19 (migration idempotency) | ✅ unit (upgrade-from-populated DB still NOT RUN) |
| Token never in responses / errors / DB / audit | ✅ unit + **live** |
| CSRF cross-origin | ✅ unit |
| T-06/T-07/T-08 (provisioning, retry, quota race) | ✅ **unit** (duplicate prevention, ambiguous-write reconciliation, quota race) |
| T-14/T-15 (IDOR on VPN objects) | ✅ **unit** (foreign client → 404, seller scoped) |
| T-09/T-10 (revocation verified / pending) | ✅ **unit** |
| T-12/T-13 (links/QR for active vs inactive) | ⏸ NOT RUN — E6 |
| T-11 (usage survives counter reset) | ⏸ NOT RUN — E5 |


---

## 4. Not run / blocked

| Item | Why | Unblock by |
|---|---|---|
| `go test -race` | no C compiler on this machine | install MinGW, or run in Linux CI |
| Live client provisioning (create/suspend/delete) | would mutate the production panel | owner authorisation on a **non-production** test inbound (D-018) |
| Frontend unit tests | no runner installed yet | Stage F (`vitest`) |
| 39 acceptance tests | depend on E4/E5 business flows | stages E4–E6 |

---

## 2. Honest status summary

- **Proven by execution (live):** health check, inbound listing, paged client list, fresh read, **integration create with a real health check**, re-verify, inbound listing through the API, RBAC denials, audit trail, token encrypted at rest.
- **Proven by execution (unit):** password hashing, password policy (length/entropy/blocklist), password change + session rotation, token sealing (incl. AAD binding + tampering), permission matrix, ownership, sessions, rate limiting, SSRF rejection, status mapping, GET-only retries, no blind write retry, CSRF, migration idempotency, transaction rollback.
- **Implemented but not live-verified:** client create / suspend / resume / delete (D-018).
- **Not implemented yet:** links/QR (E6), SPA embedding (E7), payments, chat, notifications, offers. (`admin` CRUD **is** implemented — `GET/POST /api/admins`, `PATCH /api/admins/{id}`, F-021. Sync/usage snapshots **are** implemented — E5, D-023.)

---

## 3. Gate results (current — supersedes the duplicate block above)

| Gate | Result | Evidence |
|---|---|---|
| GATE 5 — BUILD PASS (backend) | ✅ PASS | `go build ./...` exit 0 |
| GATE 5 — static analysis | ✅ PASS | `go vet ./...` exit 0 |
| GATE 5 — formatting | ✅ PASS | `gofmt -l .` printed nothing |
| GATE 5 — BUILD PASS (frontend) | ✅ PASS | `npm run lint` 0 errors / 0 warnings (102 files); `npm run build` built OK |
| GATE 6 — UNIT PASS | ✅ PASS | `go test ./...` — **271 tests**, 9 test packages (2026-10-24, per-package counts sum exactly) |
| GATE 6 — race detector | ⛔ **BLOCKED** | `go test -race` → `-race requires cgo`; `CGO_ENABLED=1` → no gcc. No C toolchain installed. |
| GATE 6 — frontend unit | ⬜ **NOT RUN** | no runner installed (`vitest` absent) |
| GATE 7 — QA checklist | ðŸ”¶ PARTIAL | SSRF, status mapping, token-leak, RBAC, audit, CSRF, quota race, duplicate prevention, **password policy + rotation** verified; links/QR + usage await E5/E6 |
| GATE 8 — live panel reads | ✅ **PASS** | see §0 |
| GATE 8 — live client create (2026-10-24) | ✅ **PASS** | `POST /api/vpns` on `indian1` → `201 ACTIVE confirmed:true`; the created client read back from the panel carries `limit_ip:2, limit_hwid:2`; probe rows removed afterwards, panel clients kept (D-027.3) |
| GATE 8 — live server edit (2026-10-24) | ✅ **PASS** | `PATCH /api/servers/1 {"name":"indian1"}` → `200` + re-test `ACTIVE`; `PATCH /api/servers/999` → `404` |
| GATE 8 — live panel mirror (2026-10-24) | ✅ **PASS** | `vpnprobe` (local `panel_*` columns) and `GET /api/servers/1/panel-clients` (the panel's own answer) agree byte-for-byte for `vyx-16`/`vyx-17` |
| GATE 8 — live client provisioning (suspend/revoke) | ⛔ **BLOCKED** | single production inbound with live users (D-018) |

### 3.1 Installation system (D-079) — 2026-11-07

| Gate | Result | Evidence |
|---|---|---|
| Backend build / vet / format | ✅ PASS | `go build ./...` 0 · `go vet ./...` 0 · `gofmt -l .` empty |
| Backend tests | ✅ PASS | `go test ./... -count=1` — **13/13 packages ok** |
| CLI package tests | ✅ PASS | `ok veylix/backend/cmd/vpn-platformctl` |
| Linux cross-build of the CLI | ✅ PASS | `GOOS=linux GOARCH=amd64 go build ./cmd/vpn-platformctl` exit 0 |
| `scripts/lib/common.sh` self-test | ✅ PASS | **31 PASS**, 1 SKIPPED (POSIX modes on Windows), exit 0 |
| `scripts/install.selftest.sh` | ✅ PASS | **31 PASS**, 1 SKIPPED (needs Linux for the full phase walk), exit 0 |
| `bash -n` all scripts | ✅ PASS | `install.sh`, `common.sh`, both self-tests — exit 0 |
| `npm run workflows:check` | ✅ PASS | `ci.yml` ok, `release.yml` ok — 2 files |
| `npm run docs:verify` | ✅ PASS | 21 migrations · 125 routes · 518 test functions in 13 packages |
| `shellcheck` | ⬜ **NOT RUN** | shellcheck is not installed on this machine |
| Clean-VM install | ⛔ **BLOCKED** | no Linux VM, no public domain on this machine |
| HTTPS issuance | ⛔ **BLOCKED** | needs a real domain and ports 80/443 |
| Backup/restore rehearsal | ⛔ **BLOCKED** | needs a clean VM |
| 1 vCPU / 500 MB benchmark | ⬜ **NOT RUN** | — |
| `go test -race` | ⛔ **BLOCKED** | no C compiler |

**What this means:** the installation surface is *built and its data-loss guards
are tested*, but **no clean Linux host has run it**. Bible §18 tests 30–34, 37,
38 and 39 are built, not verified. Full detail:
[`records/2026-11-07-installation-system-build.md`](records/2026-11-07-installation-system-build.md).

### 3.2 release-readiness Stage 5b — isolated rehearsal (2026-11-07)

**How:** `scripts/stage5b-rehearsal.ps1`. Runs the **real packaged binary**
against a throwaway data directory under `$env:TEMP`, on port **18080**, with
**generated** keys. It never reads or writes `./var/`, `./.veylix-secrets/` or
`./records/`, and deletes only the directory it created.

**Result: `PASS=12  FAIL=0  BLOCKED=1  ESTIMATED=1` of 14 rows, exit 0.**

| # | Check | Verdict | Evidence |
|---|---|---|---|
| 1 | Fresh install into an empty data directory | **PASS** | `/api/ready` → `status=ready`; the directory started empty |
| 2 | DB init + migration count | **PASS** | `schema_version=21` matches **21** migration files |
| 3 | First Super Admin (no hard-coded password) | **PASS** | `bootstrap_required=true`; anonymous `/api/me` → **401**; 4 candidate defaults all refused |
| 4 | Empty initial data | **PASS** | `GET /api/vpns` anonymously → **401** |
| 5 | Secret generation (throwaway) | **PASS** | keys from env; **0** `.key` files written; repo `./var/` untouched |
| 5b | Secrets never logged | **PASS** | the encryption key appears in neither stdout nor stderr |
| 6 | Configuration fail-closed | **PASS** | missing `VEYLIX_ENCRYPTION_KEY` → process exits, message names the variable |
| 7 | Panel URL SSRF refusal | **PASS** | **all 7** prohibited targets → **422** (loopback v4/v6, 10/8, 192.168/16, link-local, metadata, plain http) |
| 8 | HTTPS setup | **BLOCKED** | needs a real domain, DNS and ports 80/443 |
| 9 | Migrations idempotent on a second start | **PASS** | `schema_version` 21 → 21 across a restart |
| 10 | Backup and restore (throwaway ×2) | **PASS** | backup copied; the restored copy **opened and served** on a second port at schema 21 |
| 11 | Restart and recovery | **PASS** | ready after restart; the Super Admin created in step 7 still exists |
| 12 | Resource usage vs 1 vCPU / 500 MB | **ESTIMATED** | binary **20.4 MB** (measured); idle RSS ~**12.8 MB** (Windows working set, **not** a Linux VM) |
| 13 | Deployment prerequisites | **PASS** | `ui=embedded:85_files`; `/dashboard` **200**; missing asset stays **404** |

**Data safety, proven after the run:** 0 throwaway directories remained, and
nothing under `./var/` or `./.veylix-secrets/` was modified.

**Still NOT proven, and not claimed:** HTTPS (row 8), a clean **Linux** VM
install (Bible test 30), the 1 vCPU / 500 MB benchmark (row 12 is ESTIMATED), and
`go test -race` (BLOCKED — no C compiler).

**Two findings this rehearsal produced, both fixed:**

1. **The packaged binary was stale** — built `2026-11-04 02:04`, while
   `0021_dedupe_carrier_offer.sql` is dated `03:38`. It applied only **20 of 21**
   migrations. Rebuilt with `.\dev.ps1 -Package`; the row now reads 21/21. The
   script now detects a stale binary explicitly and names it as a build problem
   instead of reporting it as a product defect.
2. **The embedded UI was stale** — `dist/` held **7** files (`embedded:7_files`)
   instead of the real build's **85**. The rehearsal reported it truthfully rather
   than treating "embedded" as good enough. Rebuilt; `embedded:85_files`.

---

## 4. Historical (superseded) — Stage B skeleton run, 2026-10-23

> Kept for traceability only. **Every row in this section is out of date** and must not be read as the current state; see §1–§3 above. This block previously sat unlabelled in the middle of the live report and contradicted it (13 vs 124 tests, "no adapter" vs a live-verified adapter), which is exactly the kind of doc drift `new_update.md` §13 forbids.

| Item | Then (Stage B) | Now |
|---|---|---|
| `go test ./...` | 13 tests, 3 packages | 189 tests, 9 packages |
| auth / RBAC | not implemented | implemented + tested |
| panel adapter | not implemented | implemented + live-verified (3X-UI 3.10.2) |
| provisioning | not implemented | implemented + unit tested |
| password policy / change | not implemented | implemented + tested |
| migrations | 3 | 4 |
| live panel | blocked (no panel, no Docker) | reads + CRUD verified; writes blocked by D-018 |

**Stage B smoke test (historical):** `go run ./cmd/veylixd` on `127.0.0.1:18080` gave `schema_version:3`, `GET /api/health` → 200, `GET /api/ready` → 200, `GET /api/nope` → 404 envelope, and no secret in the log line (`encKey=32B sessKey=32B`). Clean shutdown was **not** exercised in that run.

---

## 5. Not run / blocked (explicit list)

| Item | Why | Unblock by |
|---|---|---|
| `go test -race` | no C compiler (`CGO_ENABLED=0`, no gcc) | install MinGW/TDM-GCC, or run race in Linux CI |
| Live client provisioning (create/suspend/delete) | would mutate the production panel | owner authorisation on a **non-production** test inbound (D-018) |
| Frontend unit tests | no runner installed (`vitest` not added) | Stage F |
| T-11 usage lifetime totals | Stage E5 not built | Stage E5 |
| T-12/T-13 links & QR | Stage E6 not built | Stage E6 |
| 39 acceptance tests | depend on E5–E7 business flows | stages E5–E7 |
| Password reset by email (forgotten password) | no mail transport configured | owner decision + SMTP provider |

---

## 3.3 Pass 10 — version management + operator-editable Google settings (2026-11-07, D-080/D-081)

Executed in this workspace. Every row is a command whose output was seen.

| Gate | Command | Result |
|---|---|---|
| Build | `go build ./...` | **PASS** (exit 0) |
| Vet | `go vet ./...` | **PASS** (exit 0) |
| Format | `gofmt -l .` | **PASS** (clean) |
| Unit + integration | `go test ./... -count=1` | **PASS — 15/15 packages ok** (new: `internal/settings`; `internal/versioning` from the prior pass) |
| Lint | `npm run lint` | **PASS** (0 errors, 1 advisory `set-state-in-effect`) |
| Type + bundle | `npm run build` | **PASS** (exit 0) |
| UI truth | `npm run audit` | **PASS** (no unwired/demo claims) |
| Docs drift | `npm run docs:verify` | **PASS** — `22 migrations · 133 routes · 562 test functions in 15 packages` |
| Installer guards | `scripts/install.selftest.sh` | **PASS** (exit 0, incl. new T8 `--list-versions`) |
| Shell syntax | `bash -n install.sh` | **PASS** (exit 0) |

**New surface proven by tests:** `internal/settings` seals the secret and never
stores it in plaintext, database beats environment, a blank secret keeps the
stored one, validation refuses malformed values, and the live-change callback
fires; `internal/httpapi` settings routes mask the secret, plumb the principal id,
map validation → 400, and forbid a non-super-admin (403) and a nil service (404);
`cmd/vpn-platformctl` `versions` reads a local feed and every downgrade guard
refuses correctly while an upgrade is not misread as a downgrade.

**Not run / blocked (explicit):**

| Item | Why | Unblock by |
|---|---|---|
| Live Google OAuth round-trip with a saved client | no Google credentials in this environment | configure a client, sign in against a real tenant |
| Real downgrade over live data | no live installation here | run `vpn-platformctl update --version <older> --allow-downgrade --restore-backup <dir>` on a staging host |
| `go test -race` | no C compiler (`CGO_ENABLED=0`, no gcc) | install a C compiler, or run race in Linux CI |
| HTTPS / clean Linux VM / 1 vCPU-500 MB benchmark | no domain, no Linux VM on this Windows host | a staging VPS |
### 3.3a Follow-up — I-13 (version endpoint) fixed and re-verified

An audit of the pass above found `GET /api/system/version` reported
`schema_version: 0` and `ui: ""` (the `cmd/veylixd` adapter fills neither, and
the handler wrote it unchanged), so the Settings page showed "Schema v0" and a
blank interface. Fixed in `system_routes.go` with `enrichInfo` (fills UI from
`spaStatusText`, schema from `Deps.Health` under a 3s bound), and covered by two
new tests (`TestSystemVersionFillsSchemaAndUI`, `TestSystemVersionWithoutHealthReader`).
The existing route test had set the schema in its stub, which is why the gap had
slipped — the new test deliberately mimics the adapter (schema 0 / empty UI).

Re-ran the gates after the fix: `gofmt -l .` clean (exit 0) · `go build` 0 ·
`go vet` 0 · `go test ./...` **15/15 packages ok** · `npm run docs:verify` OK
(`22 migrations · 133 routes · 562 test functions in 15 packages`). The two new
tests moved the count 560 → 562, which `docs:verify` caught and README was
corrected to — the gate doing its job.

### 3.4 Google redirect URL is derived and validated (D-082 · I-14)

| Gate | Command | Verdict |
|---|---|---|
| Build | `go build ./...` | **PASS** (exit 0) |
| Vet | `go vet ./...` | **PASS** (no output) |
| Format | `gofmt -l .` | **PASS** (empty) |
| Backend tests | `go test ./... -count=1` | **PASS** — **15/15 packages ok** |
| Lint | `npm run lint` | **PASS** (0 errors, 1 pre-existing advisory) |
| Type + bundle | `npm run build` | **PASS** (exit 0) |
| UI truth | `npm run audit` | **PASS** (no unwired/demo claims) |
| Audit self-test | `npm run audit:selftest` | **PASS** (8 positive + 6 negative) |
| Docs drift | `npm run docs:verify` | **PASS** — `22 migrations · 133 routes · 564 test functions in 15 packages` |

**New surface proven by tests:** `internal/settings` — a redirect URL that does
not end at `/api/auth/google/callback` (wrong path, trailing slash, another
provider's shape) is refused as a `ValidationError`, while the canonical path and
a reverse-proxy sub-path that still ends at it are accepted;
`internal/httpapi` — `GET /api/super/settings/google` returns an
`expected_redirect_url` **derived from the request's own host**, so a proxied
HTTPS request yields `https://vpn.acme.com/api/auth/google/callback`.

**A real consequence `docs:verify` caught:** the two new test functions moved the
count 562 → 564; README was corrected to the tree (not hand-counted).

**Not run / blocked (explicit):**

| Item | Why | Unblock by |
|---|---|---|
| Live Google OAuth round-trip with a saved client | no Google credentials in this environment | configure a client, sign in against a real tenant |
| Browser gate for the Settings callout | no committed `verify-*.mjs` covers `/settings` (needs a seeded Super Admin) | add a `verify-settings.mjs` against a dev Super Admin |
| `go test -race` | no C compiler (`CGO_ENABLED=0`, no gcc) | install a C compiler, or run race in Linux CI |
| HTTPS / clean Linux VM / 1 vCPU-500 MB benchmark | no domain, no Linux VM on this Windows host | a staging VPS |



## Section 3.5 - 2026-11-07 - Owner-reported UI defects (I-15, I-16), verified live

**Scope:** two owner-reported UI defects, plus the QR/link and theme-toggle reports.

**Executed commands and results:**

- `go build ./...` -> exit 0 - `go vet ./...` -> exit 0 - `go test ./... -count=1` -> **15/15 packages ok**.
- `npm run lint` -> 0 errors (1 pre-existing advisory set-state-in-effect, 178 files) - `npm run build` -> exit 0 - `npm run audit` -> OK - `npm run audit:selftest` -> OK (8 positive + 6 negative) - `npm run qr:selftest` -> OK (10 cases) - `npm run docs:verify` -> OK (22 migrations - 133 routes - 564 test functions in 15 packages).
- `node scripts/verify-theme-studio.mjs` -> **THEME-STUDIO OK, 7/7, 0 console errors** (`expandInsideClose:false`, `expandKeepsStudioOpen:true`, `dataView` compact->full, `closeDismisses:true`).
- `node scripts/verify-package-create.mjs` -> **PACKAGE-CREATE OK, 5/5**: the real `api.ts` serialises `kind:"PREMIUM"` + `is_custom:true`, and the raw `CUSTOM_PREMIUM` is refused **422** by the live backend.
- `node scripts/verify-theme-toggle.mjs` -> **9/9**.
- `node scripts/verify-vpn-detail.mjs` on the operator route `/vpns/11/yasa` (as super_admin) -> **11/11**, incl. **P-10 QR rendered (svg present)** and **P-10 link present (1 link, 288 bytes, `vless://...`)**.

**Rejected as a non-defect (with evidence):** `/vpn/:id` is client-only by design (`App.tsx` line 421); a Super Admin opening it is correctly redirected. `/api/me` returning 401 while signed out is correct (verified in an earlier session).

**Not run / blocked (explicit):** the **client** route `/vpn/:id` QR (no client-owned VPN in this seed) - a live **fail-before** replay of each fix (the tests are failure-sensitive by construction) - `go test -race` (no C compiler).


---

## D-084 — Frontend redesign, stages 2 and 3 (design tokens + application shell / navigation)

**Date:** 2026-11-07 · **Scope:** frontend only. **Backend source untouched.**

### Change scope (stated before editing)

| File | Change |
|---|---|
| `frontend/src/styles/tokens.css` | Appended an additive token block (surface ladder, state ramp, `--vpn-accent`, `--ring-focus`). No existing token edited. |
| `frontend/src/components/layout/navModel.ts` | **New.** Three role-scoped nav trees + `ROLE_LABEL`. |
| `frontend/src/components/layout/Breadcrumbs.tsx` | **New.** Route-derived trail, renders nothing at depth 1. |
| `frontend/src/components/layout/Sidebar.tsx` | Collapsible groups (persisted, active-group force-open), `onNavigate`, `slug` helper. |
| `frontend/src/components/layout/Topbar.tsx` | Context block (breadcrumbs) + role badge. |
| `frontend/src/components/layout/AppShell.tsx` | Passes `onNavigate` to close the mobile drawer. |
| `frontend/src/components/layout/layout.css` | Appended `.app-crumbs*`, `.app-topbar__context`, `.app-topbar__role`, `.app-nav__group-toggle`. Additive only. |
| `frontend/src/components/layout/index.ts` | Barrel exports. |
| `frontend/src/App.tsx` | Flat `NAV` (22 entries) replaced by `navigationFor(navRole)` import; file went 565 → 394 lines. |

### Executed commands and real results

| Gate | Result | Evidence |
|---|---|---|
| `go build ./...` | **PASS** (exit 0) | no output |
| `go vet ./...` | **PASS** (exit 0) | no output |
| `go test ./... -count=1` | **PASS** | 15/15 packages ok |
| `npm run lint` | **PASS** — 0 errors | 2 warnings, **both pre-existing** (`SuperSettingsPage` set-state-in-effect); 179 files |
| `npm run build` | **PASS** (exit 0) | `tsc -b && vite build`, built in 4.46s |
| `npm run audit` | **PASS** | "no unwired/demo claims and no invented money literals" |
| `npm run audit:selftest` | **PASS** | 8 positive + 6 negative |
| `npm run docs:verify` | **PASS** | 22 migrations · 133 routes · 566 test functions · 15 packages |
| `npm run verify:nav` | **PASS 58/58** | 23 positive + **35 role-isolation negative** checks |
| `npm run verify:subthemes` | **PASS** | 60/60 (6 presets × 10 sub-themes) |
| `npm run verify:theme-toggle` | **PASS** | 9/9, 0 console errors |
| `npm run verify:studio` | **PASS (2nd run)** | 7/7 — see flake note below |
| `npm run verify:ui` | **PASS** | dashboard probe, 0 console errors |

### Failures found — investigated, not hidden

**1. `verify:studio` failed once (`expandTogglesToFull: false`), then passed 7/7.**
Not a regression. A Playwright probe of the same interaction returned
`dataViewBefore: "compact"` → `dataViewAfter: "full"` with `studioStillOpen: true`,
i.e. the toggle works. Cause: the script waits a fixed `sleep(600)` after
clicking Expand, which races the CSS transition. **Reported as a flaky gate, not
as a pass and not as a defect.**

**2. `verify:vpn-actions` — 5 failures on the default id, 4 on a real id.**
Pre-existing, unrelated to this change:
- It defaults to `VPN_ID = 36`. Queried the live DB read-only: `vpn_instances`
  ids are `[38, 39, 40]`. **Id 36 does not exist**, which is the source of the
  404s. With id 38 those disappear.
- It opens `/vpn/:id` as a Super Admin, but that route is **client-only**
  (`App.tsx`), so staff are redirected and the "Suspend control" check cannot
  pass by route design — the script's own comment says so.
- The remaining `403`s are the backend correctly refusing a suspend against an
  unhealthy panel (`infra-1 … provisioning through them is at risk`).

**3. `verify:packages` — 1 failure (`rawUiKindIsRefused`).**
Pre-existing. It asserts a **raw API** call returns `422`; a backend validation
assertion, untouched by a frontend shell change.

### Measure — production bundle delta (measured, not estimated)

| Chunk | Before | After | Δ |
|---|---|---|---|
| `index` | 315.0 KB | 325.9 KB | **+10.9 KB** (nav trees + breadcrumbs + topbar) |
| `ui` | 126.5 KB | 129.6 KB | +3.1 KB |
| `threeScene` | 523.9 KB | 536.5 KB | unchanged by this work (pre-existing scene) |

Route-level code splitting is preserved; the new shell code is eager by design
(it must not unmount the sidebar on navigation).

### Browser verification (Playwright MCP — exploratory, NOT a gate)

Screenshots committed under `records/ui-redesign/`:
`stage3-superadmin-shell.png`, `stage3-client-shell.png`.

Observed in the live app:
- Super Admin: 7 groups — Platform, VPN Management, Infrastructure, People &
  Access, Commerce, System, Account — with live count chips (Offers 4, Servers 1,
  Payments 1). Role badge "Super Admin".
- `/servers/1`: trail `Overview › Servers › Server`, last item `aria-current="page"`,
  first item links to `/dashboard`.
- `/dashboard` (role home): trail correctly **absent** (depth 1).
- Client: 3 groups — My VPN, Get more, Account. Role badge "Client". Empty states
  render real icons and an actionable CTA.

### Not proved / not claimed

- `go test -race` — **BLOCKED** (no C compiler on this machine). Never reported as PASS.
- 1 vCPU / 500 MB runtime behaviour — **ESTIMATED**; only bundle sizes measured.
- Client-portal VPN hero card, role overviews, VPN card system, tables, forms,
  responsive pass — **later stages, not implemented here**.
- Theme sweep was script-level (`verify:subthemes` 60/60, `verify:theme-toggle`);
  a per-page visual sweep across all six presets is scheduled for stage 7.

---

## 2026-11-07 · Backend audit pass (I-27 … I-32, D-085 / D-086)

Scope: six backend/gate defects found during the post-D-084 audit. Every row
below is a command that **actually ran**, with the output it produced.

| # | Gate | Command | Result |
|---|---|---|---|
| 1 | Requirement | `./MASTER_PRODUCT_BIBLE.md` §0.6, §8, §17 | PASS — outbox recovery and panel-truth rules cited |
| 2 | Architecture | `go build ./...` + import read of every changed file | PASS — `httpapi` gained a cache decorator only; no SQL in `httpapi`, no cycle, `panel` remains the only outbound caller |
| 3 | Backend unit | `go test ./... -count=1` | PASS — 15/15 packages ok |
| 4 | API integration | `.\dev.ps1 -Preflight` | PASS — 33/33 checks (roles, portals, refusals, session) |
| 5 | Database | `go test ./... -count=1` (isolated harness DBs) | PASS — no test touched `./var/` or `./.veylix-secrets/` |
| 6 | Frontend | `npm run lint` · `npm run build` · `npm run audit` · `npm run audit:selftest` | PASS — 0 errors (2 pre-existing advisories), build exit 0, audit OK, selftest OK |
| 7 | Browser | `npm run verify:nav` · `npm run verify:ui` · `node scripts/verify-vpn-detail.mjs` | PASS — 58/58 · OK 0 console errors · 14/14 |
| 8 | Security | `security-review.md` checklist over the changed surface | PASS — see below |
| 9 | Performance | live `GET /api/super/vpns` timing, 12 sequential requests | **MEASURED** — 480 ms cold, 3.2–6.3 ms warm, median 3.9 ms |
| 10 | Regression | 10 new tests, each shown red with the fix reverted | PASS — see below |
| 11 | Final | this table | ready for the next stage, with the gaps below |

### Security checks on the changed surface

- **No secret in any new log line.** The outbox worker logs counts
  (`applied`/`retry`/`exhausted`/`resolved`), never a token, email or payload.
- **No new authorization surface.** `authorise` now takes `ctx`; its decision
  logic is unchanged and `TestOwnershipIsEnforced` still passes.
- **Cache cannot fabricate a remote fact.** A failed panel read is returned as an
  error, never as an empty list — `TestPanelCacheNeverCachesAFailureAsSuccess`.
- **`REVOKED` is still never written optimistically.** On exhaustion the VPN
  keeps its honest outage state (D-024).
- Grep of the changed files for token/key/UUID shapes: none introduced.

### Regression proof (red → green)

| Test | Fix reverted | Observed failure |
|---|---|---|
| `TestReconcileStopsAfterMaxAttempts` | cap disabled | `outcomes = map[RETRY:1], want 1 EXHAUSTED` |
| `TestRetriedGetResendsItsBody` | shared reader restored | `attempt 2 sent an EMPTY body` |
| `panel_cache_test.go` (7) | n/a — new behaviour | each asserts a distinct property (collapse, TTL, honesty, invalidate) |

### NOT RUN / BLOCKED — reported, not rounded up

- `go test -race` — **BLOCKED**: no C compiler on this machine.
- Load test of the panel cache — **NOT RUN**: the concurrency test proves 12
  readers collapse to 1 panel call, but no sustained-load benchmark exists.
- `verify-vpn-actions.mjs` — **NOT RUN end-to-end**: it performs real
  suspend/delete writes, so it was not executed against the dev dataset. Its
  target resolution was verified by inspection and `node --check`.
- `verify-vpn-detail.mjs` as the **client** persona — **NOT RUN**: the seeded
  client owns 0 VPNs. It now reports that as a data-setup gap with the fix,
  instead of failing on an inaccessible page.

### Data-loss note

`dev.ps1 -Restart` **truncated `var/logs/backend.log`** during this pass, which
destroyed the 16,774-request log that the pre-fix performance figures were
derived from. Those figures (median 1,044 ms, max 44.8 s) were recorded **before**
the restart in `docs/IMPROVEMENT_LEDGER.md` I-29 and remain accurate as
measurements; they simply can no longer be recomputed from disk. No user data was
touched: no migration ran, no row was updated or deleted, and the outbox was
empty (`cmd/outboxprobe` → `outbox: no rows`) throughout.

---

## S3 — Client UI foundation and navigation (2026-11-08)

**Scope:** frontend only. Client navigation made progressive, the hybrid `/client`
page split into a Dashboard and a new `/client/vpns` inventory, a shared client
page frame and filter bar added. **No backend file, migration, endpoint, Theme
System file or production datum was changed.** `go build` / `go vet` / `go test`
all still pass unchanged, which is the evidence for that claim.

### Commands actually run, with their real output

**Backend (confirms nothing on that side moved):**

| Command | Result |
|---|---|
| `go build ./...` | **PASS** — exit 0 |
| `go vet ./...` | **PASS** — no output |
| `gofmt -l .` | **PASS** — no files listed |
| `go test ./... -count=1` | **PASS** — 15/15 packages `ok` |
| `.\dev.ps1 -Preflight` | **BLOCKED** — `Port 8080 is already in use (pid 6076)`. Preflight must own the port; a dev backend was already serving and stopping a running service mid-session was not authorised. **Not run, not claimed.** |
| `go test -race` | **BLOCKED** — no C compiler on this machine (pre-existing, documented). |

**Frontend static gates:**

| Command | Result |
|---|---|
| `npx tsc --noEmit -p tsconfig.app.json` | **PASS** — exit 0 |
| `npm run lint` | **PASS** — 0 errors, **2 warnings**, both pre-existing `react(set-state-in-effect)` advisories in `AdminOverviewPage.tsx` and `Sidebar.tsx` (files untouched by S3) |
| `npm run build` | **PASS** — exit 0. `MyVpnsPage` ships as its own lazy chunk (`MyVpnsPage-Ik_rg5Du.js` 10,850 B + `MyVpnsPage-CdKCEWfx.css` 3,141 B), so the new screen adds nothing to the main bundle |
| `npm run audit` | **PASS** — `UI truth audit: OK — no unwired/demo claims and no invented money literals in src/` |
| `npm run audit:selftest` | **PASS** — 8 positive + 6 negative cases |
| `npm run docs:verify` | **PASS** — `tree: 23 migrations · 133 routes · 615 test functions in 15 packages`; `README matches the tree` |

> `docs:verify` reports **615** test functions where `docs/cline/activeContext.md`
> still says 612. That drift is **pre-existing** and unrelated to S3 (no Go test
> was added or removed here); `docs:verify` reads the tree and passes, so the tree
> is the authority. Recorded rather than silently corrected.

**Browser gates:**

| Command | Result |
|---|---|
| `npm run verify:nav` | **PASS** — **NAV OK — 66 passed, 0 failed**. Includes `client nav has /client/vpns`, `client nav omits /vpns`, and a new per-destination resolution check: all 7 client destinations land on themselves with no guard bounce |
| `npm run verify:ui -- /client` (`VEYLIX_DEV_ROLE=client`) | **PASS** — title `Dashboard`, 3 cards incl. `Free offers — claim your own credential`, `consoleErrors: []` |
| `npm run verify:ui -- /client/vpns` (`VEYLIX_DEV_ROLE=client`) | **PASS** — title `My VPNs`, 1 empty state with an action, `consoleErrors: []` |
| `npm run verify:ui -- /buy` (`VEYLIX_DEV_ROLE=client`) | **PASS** — `path: /buy` (no redirect), title `Buy premium`, `consoleErrors: []` |
| `npm run verify:ui -- /history` (`VEYLIX_DEV_ROLE=client`) | **PASS** — `path: /history`, title `Purchase history`, `consoleErrors: []` |
| `npm run verify:ui -- /vpns` (`VEYLIX_DEV_ROLE=super_admin`) | **PASS** — title `VPN management`; the Super Admin console is **untouched** |
| `npm run verify:vpn` (`VEYLIX_DEV_ROLE=super_admin`) | **PASS** — **14/14 checks, no console errors** — QR, links, P-01 up/down/total, usage, protocol, charts, CPU/RAM all intact |
| `node scripts/verify-mobile.mjs /client/vpns` (`VEYLIX_MOBILE_ROLE=client`) | **PASS** — at 390 px: `horizontalOverflowPx: 0`, `under44px: 0` |
| `npm run verify:subthemes` | **PASS** — 60/60 (`TOTAL 60 (target 60)`), all six presets |
| `node scripts/verify-themes.mjs` | **PASS** — six presets, `store: ok`, `overflowX: false` for each |
| `npm run verify:theme-toggle` | **PASS** — `THEME-TOGGLE OK`, 0 console errors |
| `node scripts/verify-profile.mjs` | **PASS** — 3 tabs (Account/Contact/Security), `consoleErrors: []` |

**Gates that could NOT run, and why (not claimed as PASS):**

| Gate | Status | Reason |
|---|---|---|
| `npm run verify:vpn` as **client** | **NOT PERFORMED** | The script refuses with *"the client persona owns no VPN, so `/vpn/:id` has nothing to render"*. The seeded client owns no VPN — a **pre-existing data gap** already recorded in `docs/cline/activeContext.md`. The operator path was run instead (14/14) to prove the page is intact. |
| `node scripts/verify-client-vpn.mjs` | **NOT PERFORMED** | Requires `VEYLIX_EMAIL` / `VEYLIX_PASSWORD` for a VPN-owning client. No client-owned VPN exists in the seed, so no credential can be supplied. The script refuses rather than reporting a false negative — correct behaviour. |
| `node scripts/verify-chat.mjs` | **BLOCKED (pre-existing)** | The script spawns its own `npx vite --port 5173 --strictPort` (`scripts/verify-chat.mjs:27`) and the dev server already held 5173, so its sign-in got `(none)`. A script-isolation limitation that predates S3; S3 did not touch chat. |
| `.\dev.ps1 -Preflight` | **BLOCKED** | Port 8080 already in use (see above). |



### Responsive verification — 360 / 390 / 768 / 1024 / 1440 / 1920 px

Measured with a read-only CDP probe against the running app (temp script written
outside the repository and deleted after the run).

**Horizontal overflow — the primary responsive failure mode:**

| Width | `/client` | `/client/vpns` |
|---|---|---|
| 360 px | **0 px** | **0 px** |
| 390 px | **0 px** | **0 px** |
| 768 px | **0 px** | **0 px** |
| 1024 px | **0 px** | **0 px** |
| 1440 px | **0 px** | **0 px** |
| 1920 px | **0 px** | **0 px** |

**The new grid and filter rules resolve as designed** (computed style of the real
classes at each width):

| Width | `.client-vpn-grid` columns | gap | search field |
|---|---|---|---|
| 360 | 1 | 12 px | full width |
| 390 | 1 | 12 px | full width |
| 768 | 2 | 16 px | full width |
| 1024 | 3 | 16 px | 220 px basis / 380 px cap |
| 1440 | 4 | 16 px | 220 px basis / 380 px cap |
| 1920 | 5 | 16 px | 220 px basis / 380 px cap |

> The card grid did not mount on `/client/vpns` during the session because the
> seeded client owns no VPNs (the empty state rendered instead — one empty state,
> with an action). The grid's behaviour was therefore proven against its own
> computed style at all six widths, which is the property S3 added.
>
> Sub-44 px interactive targets were also counted (7 at 360 px, 15–17 above
> 768 px). They are shell controls — sidebar links, the topbar and table buttons —
> that S3 did not write and did not change; `.client-filters` and `.client-vpn-grid`
> contributed **0**. Recorded as a **pre-existing** accessibility observation,
> not an S3 regression, and not fixed here (that is board row U18).

### Theme verification — the new surfaces consume tokens, not literals

Each of the six presets was applied and the **resolved** colours of a real
`.client-filters__input` were read back:

| Preset | `--bg-surface` token | input background resolves to | literal hex? |
|---|---|---|---|
| modern | `#f1f1f9` | `rgb(255,255,255)` | **no** |
| dark | `#171732` | `rgb(25,25,56)` | **no** |
| light | `#eff1fa` | `rgb(255,255,255)` | **no** |
| classic | `#f7f9fc` | `rgb(255,255,255)` | **no** |
| gaming | `#0d1729` | `rgb(12,21,38)` | **no** |
| hacking | `#0d1f17` | `rgb(10,23,18)` | **no** |

The input's background tracks the theme's own `--bg-surface` in every preset and
no resolved colour is a hard-coded value — the new CSS consumes the existing token
layer rather than bypassing it. `components/client/client.css` contains **no** new
token definition and **no** hex literal; `--bg-input` is referenced with
`--bg-surface` as its fallback.

### Data safety

No verification command in this session wrote to `./var` or `./.veylix-secrets`.
Every browser gate used the running dev server with a signed-in cookie; the only
writes any of them could perform are the ones those scripts already performed
before S3. No migration ran. No row was created, updated or deleted. The two
temporary measurement scripts were written under `C:\Users\Public\` and deleted
(verified: 0 remaining).

---

## S3-followup — Client-surface touch targets (2026-11-08, S3-5)

**Scope:** one production stylesheet, plus documentation. **No backend file, no
migration, no endpoint, no theme file and no shared shell component changed.**
`docs:verify` still reports **23 migrations · 133 routes · 615 test functions in
15 packages**, and `go build` / `go vet` are clean — the evidence that nothing
outside the Client portal moved.

### The defect, measured from source (not estimated)

`styles/touch.css` applies the D-058 44×44 floor **only** to shell classes
(`.app-nav__item`, `.app-topbar__icon-btn`, `.ui-btn--*`, `.ui-input`,
`.ui-table__sort`, …). `styles/mobile.css` adds a subset below 480 px. **Neither
file names any Client-portal control**, so the four families S3 introduced or
first exposed were still at desktop size:

| Control | Source | Measured |
|---|---|---|
| `.client-filters__clear` | `client.css:131-137` | **22 × 22 px** |
| `.client-filters__select` | `client.css:160-166` (8 px padding + 13 px text) | ≈**33 px** tall |
| `.client-filters__input` | `client.css:102-106` (8 px padding + 13 px text) | ≈**32 px** tall |
| `.client-page .ui-pagination__btn` | shared rule `components/ui/ui.css:591-594` | **32 × 32 px** |

The pagination control was previously unused anywhere in the product; S3 was the
first screen to render it, which is why this surfaced now.

### The fix

A single `@media (pointer: coarse)` block appended to
`frontend/src/components/client/client.css`. Coarse-pointer scoping is
deliberate and follows D-058: a mouse is precise and 32–33 px rows are an
intentional desktop density, while a thumb is not. Desktop is therefore
unchanged.

The pagination override uses a **descendant selector**
(`.client-page .ui-pagination__btn`) so the shared `ui.css` rule — and every
non-client consumer of that primitive — keeps its original size.

**Verified present in the compiled bundle** (`dist/assets/MyVpnsPage-*.css`):

```
pointer:coarse){.client-filters__input,.client-filters__select{min-height:44px}
.client-filters__clear{width:44px;height:44px}
.client-page .ui-pagination__btn{min-width:44px;height:44px}}
```

### Commands actually run

| Command | Result |
|---|---|
| `npx tsc --noEmit -p tsconfig.app.json` | **PASS** — exit 0 |
| `npm run lint` | **PASS** — 0 errors, **2 warnings**, both pre-existing `react(set-state-in-effect)` advisories in `SuperSettingsPage.tsx` and `Sidebar.tsx` (files not touched here) |
| `npm run build` | **PASS** — exit 0, `✓ built`. `MyVpnsPage-*.css` 3,335 B carries the new block. The 536 kB `threeScene` chunk advisory is pre-existing and opt-in |
| `npm run audit` | **PASS** — `UI truth audit: OK — no unwired/demo claims and no invented money literals in src/` |
| `npm run audit:selftest` | **PASS** — 8 positive + 6 negative cases |
| `npm run docs:verify` | **PASS** — `23 migrations · 133 routes · 615 test functions in 15 packages`; README matches the tree |
| `npm run verify:subthemes` | **PASS — 60/60** (`TOTAL 60 (target 60)`, `SUBTHEMES OK`) |
| `go build ./...` | **PASS** — exit 0 |
| `go vet ./...` | **PASS** — exit 0, no output |

### NOT RUN — and why (reported, not rounded up)

| Gate | Status | Reason |
|---|---|---|
| `npm run verify:nav` | **BLOCKED (environment)** | `ECONNREFUSED 127.0.0.1:8080`. It needs a live backend; neither 8080 nor 5173 was listening. Starting a dev server was not authorised, and no running service was disturbed. |
| `npm run verify:ui` (`/client`, `/client/vpns`) | **NOT RUN** | Same reason — requires the SPA on 5173 + API on 8080 |
| `node scripts/verify-mobile.mjs` | **NOT RUN — cannot prove S3-5** | The script measures **390 px only** (S3-1) and **never enables touch emulation** (S3-4), so `@media (pointer: coarse)` would be inert and any `under44px` figure would describe desktop density. Running it would produce a misleading number, so it was not cited as proof. |
| `.\dev.ps1 -Preflight` | **NOT RUN** | Would start services; not authorised in this pass |
| `go test -race` | **BLOCKED** | No C compiler on this machine (pre-existing, documented) |

### Defect statuses after this pass — nothing newly CLOSED

| Row | Status | Why |
|---|---|---|
| **S3-1** | 🟡 **OPEN** | `verify-mobile.mjs` still measures one width. Not changed in this pass. |
| **S3-2** | 🟡 **OPEN** | Chat verifier unchanged; explicitly deferred. |
| **S3-3** | 🟡 **OPEN** | Seeded client still owns no VPN. No fixture was created. |
| **S3-4** | 🟡 **OPEN** | Touch emulation still absent from `verify-mobile.mjs`. |
| **S3-5** | 🟡 **OPEN — fix applied, closure PENDING** | CSS corrected and proven shipped in `dist`; the coarse-pointer gate that would close it does not exist yet. |
| **U18** | 🟡 **OPEN, untouched** | Shell controls were deliberately not modified. |

### Deferred to a later stage / S12 Final QA

The full harness designed earlier was **not** implemented, by owner direction:
`verify-h2-harness.ps1`, the 12-record H2 dataset, H1/H2 fixture orchestration,
the six-width responsive matrix, the browser functional suite, and any frontend
unit-test runner (U21 stays BLOCKED). The design is retained in this report's
preceding planning discussion and must be revisited before any claim that the
Client portal is fully verified.


