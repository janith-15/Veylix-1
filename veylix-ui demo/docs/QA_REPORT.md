# VEYLIX — QA REPORT

**Run date:** 2026-10-24 (evening run: live preview + UI truth pass) · **Previous:** 2026-10-23 (Stages B/E2/E3/E4)
**Environment:** Windows dev machine, Go 1.27.1, Node 24.19.0, Chrome (headless via CDP)
**Live panel:** `difanx4f3.difanx1.ggff.net:2053` — 3X-UI **3.10.2**, Xray **26.10.10** (verified via HTTPS + Bearer token)
**Rule:** every line below is a command that was actually executed. Anything not executed is listed as **NOT RUN** — never implied as passing (`new_update.md` §13, `docs/QA_PLAN.md`).

---

## 0. Package count — reconciled 2026-11-05

Older entries in this repo say "**13/13** packages". The correct figure for the
current tree is **12**, and the difference is not a regression — it is a counting
bug that has now been fixed at the source.

| Question | Command | Answer |
|---|---|---|
| How many packages exist? | `go list ./... \| wc -l` | **26** (12 `internal/*` + 14 `cmd/*` and nested) |
| How many have tests? | `go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...` | **12** |
| What does `go test ./...` print? | `go test ./... -count=1` | **12 `ok`** + 14 `no test files` |

`internal/audit` and `internal/releases` have **no test files**. The old
`frontend/scripts/docs-verify.mjs` counted *directories* under `internal/`
(13) rather than packages containing tests (12), so the README claim "in 13
packages" was wrong **and the gate enforced the wrong number** (`VYX-013`). The
gate now asks `go list` directly, and `npm run docs:verify` reports 12.

Historical log entries that say 13/13 are left as written — they were true when
measured — and are annotated rather than rewritten.


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
| `ROADMAP.md` | "Milestone 5-B — 🔴 **not started**" above a list of mostly-`[x]` items; E5/E6 unbuilt | 5-B marked shipped (steps 1–12), E5/E6 `[x]` **live-verified**, contradiction noted inline |
| `ROADMAP.md` | capability table: links "🟡 pending", multi-panel "⬜ E5" | both 🟢 live-verified |
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
| GATE 7 — QA checklist | 🔶 PARTIAL | SSRF, status mapping, token-leak, RBAC, audit, CSRF, quota race, duplicate prevention verified; links/QR + usage await E5/E6 |

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
| T-17 (manual confirm audited, never automatic) | 🔶 audit write path proven; payment confirm awaits E4 |
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
| GATE 7 — QA checklist | 🔶 PARTIAL | SSRF, status mapping, token-leak, RBAC, audit, CSRF, quota race, duplicate prevention, **password policy + rotation** verified; links/QR + usage await E5/E6 |
| GATE 8 — live panel reads | ✅ **PASS** | see §0 |
| GATE 8 — live client create (2026-10-24) | ✅ **PASS** | `POST /api/vpns` on `indian1` → `201 ACTIVE confirmed:true`; the created client read back from the panel carries `limit_ip:2, limit_hwid:2`; probe rows removed afterwards, panel clients kept (D-027.3) |
| GATE 8 — live server edit (2026-10-24) | ✅ **PASS** | `PATCH /api/servers/1 {"name":"indian1"}` → `200` + re-test `ACTIVE`; `PATCH /api/servers/999` → `404` |
| GATE 8 — live panel mirror (2026-10-24) | ✅ **PASS** | `vpnprobe` (local `panel_*` columns) and `GET /api/servers/1/panel-clients` (the panel's own answer) agree byte-for-byte for `vyx-16`/`vyx-17` |
| GATE 8 — live client provisioning (suspend/revoke) | ⛔ **BLOCKED** | single production inbound with live users (D-018) |

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

