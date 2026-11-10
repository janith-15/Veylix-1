# VEYLIX VPN — IMPLEMENTATION PLAN

**Version:** 1.0
**Status legend:** [x] Done · [~] In progress · [ ] Planned · [x!] Blocked/Needs decision
**Process rules (owner-mandated):**
- Update this file at every step.
- Work **step by step**; complete a piece, owner live-checks it, then proceed.
- Any change to flow/scope requires **owner approval** first (recorded in `docs/DECISIONS.md`).
- Every milestone ends with a **live test** on the owner's localhost.

---

## Phase 0 — Foundation ✅ (done)
- [x] Git init + project structure
- [x] `docs/REQUIREMENTS.md`
- [x] `docs/ROADMAP.md`
- [x] `docs/IMPLEMENTATION_PLAN.md` (this file)
- [x] `docs/DECISIONS.md`
- [x] First commit to git (`71d2605`)
- [ ] Push to remote (owner provides remote URL when ready)

## Phase 1 — Frontend scaffold
### 1.1 Vite + React + TS project
- [x] Create Vite React-TS app in `frontend/`
- [x] Linting (Oxlint) + TS strict config
- [x] `npm run build` passes (tsc + vite); dev server serves base layout
- [ ] OWNER LIVE TEST

### 1.2 Design system + Theme Studio foundation
- [x] Design tokens (CSS custom properties) for all five themes
- [x] Font stack incl. Sinhala fallback (Noto Sans Sinhala / Iskoola Pota / Nirmala UI)
- [x] Core components: Button, Input, Card, Badge, Table, Dialog, Toast, Skeleton, EmptyState, Tabs, Dropdown, Tooltip, Switch, Checkbox, Pagination, MetricCard, StatusBadge
- [x] Theme engine: 5 presets, CSS var generation (themes.ts + themeStore.ts + ThemeProvider.tsx)
- [x] **Theme Studio UI**: preset cards, colour pickers (hex-validated), typography, effects, glass toggle, reset
- [x] Persist theme (localStorage; account sync later)
- [x] **Theme-aware brand logo** (`VeylixLogo` — shield/V/lock/circuit, colours follow theme accents)
- [x] **Glassmorphism** on cards, sidebar, topbar, dialogs, toasts (per-theme intensity; Classic = solid; low-resource = off)
- [x] **3D animated background**: off / subtle (Canvas 2D particles) / enhanced (lazy Three.js scene), app-wide
- [ ] OWNER LIVE TEST

### 1.3 Theme palette rebuild + UI 2.0 ✅
- [x] Premium palette rebuild — 5 presets (Indigo Nebula / Midnight Aurora / Cloud Indigo / Slate Pro / Neon Prime)
- [x] **40 sub-themes** (8 per preset) incl. community legends: Nord, Dracula, Tokyo Night, Catppuccin Mocha, Catppuccin Latte, Solarized Light, One Light, Rosé Pine Dawn, Gruvbox (official hex specs)
- [x] Contrast audit across all 40 variants — AA-safe solid button pairs (dark-ink accents on pastel themes)
- [x] Gradient primary buttons, secondary hover, accent tint + elevation ladder + motion tokens, tabular numerals
- [x] **⌘K command palette** (in-house, zero deps — see DECISIONS D-009): navigation / theme switching / actions
- [x] WAI-ARIA: dialog focus trap + focus restore, tablist arrow-key navigation
- [x] Table sorting (aria-sort), sidebar rail collapse + footer user dropdown, topbar scroll shadow, nav count chips, avatars
- [x] Alive dashboard: time-range chips, count-up metrics, sparklines, live pulse; skeleton shimmer, toast exit animation
- [x] Glass-skin ghost-danger for pale community palettes; scene layer z-index fix (text visibility on non-shell pages)
- [ ] OWNER LIVE TEST

### 1.4 Data interaction & motion (UI 2.1) ✅
- [x] LineChart interactivity: hover **crosshair**, per-series highlight dots, HTML **tooltip** (animated series rows, edge-flip), touch support
- [x] Legend chips — click toggles series (multi-series auto)
- [x] BarChart hover: dim-others + outline + exact-value tooltip (+ `showValues`); DonutChart hover: center % swap, arc emphasis, legend %
- [x] **View Transitions API** route navigation (sidebar + ⌘K palette) — graceful fallback + reduced-motion skip
- [x] **Notifications popover** (bell): unread dots, mark read / mark all read, empty state, live badge (`stores/notifStore.ts` mock, D-006)
- [x] Full UI catalog: `docs/UI_INVENTORY.md` (living doc, §1–9)
- [ ] OWNER LIVE TEST (combined with §1.3 walkthrough)

## Phase 2 — Role-based pages (UI-first with API-shaped mock data)
- [x] Router + guards (role-based navigation) — `App.tsx:139-164` + `RequireAuth.tsx`
- [x] Auth screens (login/register UI, Google + email/password) — mock flow per D-006
- [x] App shell per role (nav filtered by `authStore` role)
- [x] Super Admin pages: **Server integrations ✅ · VPN offers ✅ · Admin management ✅ · Pricing/payments ✅ · Incidents/audit ✅** (mock stores per D-006; support chat = shared ChatPanel)
- [x] Admin pages: Overview, Assigned servers, Clients, Premium sale flow, Retail price, Support chat
- [x] Client pages: Dashboard (Free offers + own VPNs), VPN detail (status/usage/limits/QR/link/guide), Buy Premium, Purchase/history, Notifications, Profile/preferences
- [x] Notifications center (topbar popover + full page)
- [x] Internal chat UI (`ChatPanel` + `chatStore`, role-filtered)
- [ ] Known page gaps (tracked, not blocking): Super Admin **clients** screen, **incidents** screen, Admin **own contacts/payment details** screen — `GAP_ANALYSIS.md` §6
- [ ] OWNER LIVE TEST (full UI walkthrough with mocks)

## Phase 3 — Backend foundation (Go + SQLite)

> **Ordering note (D-010, 2026-10-23):** Phase 3 + Phase 4 are executed together as **Stages A–H** of `docs/new_reqierment/new_update.md` §11. The stage list below is the authoritative checklist; the Phase 3/4 headers are kept so earlier history stays readable.

### Stage A — Baseline & repository inventory ✅ (2026-10-23)
- [x] Repository, docs, git tree inspected; pre-existing changes preserved (`GAP_ANALYSIS.md` §0)
- [x] Toolchain + build/lint baseline recorded (lint 0/0 · build ✓ · **test baseline = none**)
- [x] Evidence inventory → `docs/GAP_ANALYSIS.md`
- [x] Findings ledger seeded → `docs/IMPROVEMENT_LEDGER.md` (F-001 … F-010)
- [x] Target 3X-UI contract verified + recorded → `docs/3XUI_API_CONTRACT.md` (v3.10.2, spec `3X-UI Panel API v3.x`)

### Stage B — Architecture & dependency mapping ✅ (2026-10-23)
- [x] `backend/` Go module (`go 1.27`), layout `cmd/veylixd` + `internal/{config,httpapi,domain,store,panel}` (auth/jobs/audit packages land with their stage)
- [x] `docs/ARCHITECTURE.md` — layer boundaries, dependency direction, data invariants, 500 MB budget
- [x] `docs/API_CONTRACT.md` — FE↔BE contract for all 10 mock stores (D-006 exit list)
- [x] `docs/SDLC_PLAN.md` + `docs/QA_PLAN.md` (D-014, D-015) and first real `docs/QA_REPORT.md`
- [x] Gate 2 evidence: `go build` ✓ · `go vet` ✓ · `gofmt -l` clean · `go test` 13/13 ✓ · FE lint 0/0 · FE build ✓
- [ ] OWNER REVIEW of Stage B before Stage C
- [ ] OWNER LIVE TEST: run `go run ./cmd/veylixd` locally and open `/api/health`, `/api/ready`

### Stage C — File-by-file audit
- [ ] `frontend/src/stores/*` + `pages/*` line-level audit (F-004 already characterised)
- [ ] New backend files: design review per `new_update.md` §4 before merge

### Stage D — Prioritised plan
- [ ] P0/P1/P2/P3 ordering signed off; `IMPROVEMENT_LEDGER.md` statuses updated

### Stage E — Incremental fixes
- [x] **E1 (partial)** Config + secrets + SQLite (WAL, FK, busy timeout, bounded pool, embedded migrations, tx helper) — done
- [ ] **E1 (rest)** backup path + verified restore rehearsal
- [x] **E2** Auth: email/password (argon2id), sessions (Secure/HttpOnly/SameSite), RBAC middleware, row-level ownership, rate limits, audit service — done (Google server-verify still pending: needs OAuth credentials, Q-3)
  - [x] argon2id hashing (+ rehash policy), AES-256-GCM token sealer with AAD binding
  - [x] Permission matrix per REQUIREMENTS §4 + ownership rules (view vs mutate)
  - [x] Sessions: fingerprint-only storage, expiry purge, suspend-invalidates
  - [x] Rate-limited login, constant error for unknown user vs wrong password
  - [x] CSRF origin check, `/api/setup/bootstrap` (one-time, no default password)
  - [x] Audit service (append-only, secret keys redacted)
  - [x] Live-verified: bootstrap → login → /api/me → logout → 401; client → server/audit routes = 403
  - [x] **Password policy** — 12–512 chars, no repeated rune, no straight sequence, blocklist; `HashPassword` always applies it (F-017)
  - [x] **Password change** — `POST /api/auth/change-password`, revokes every other session, re-issues the caller's cookie, audited (F-016)
  - [x] Frontend password hint mirrors the backend policy (`lib/passwordPolicy.ts`) so the UI can never green-light a rejected value (F-015)
  - [x] Profile page: change-password dialog
  - [ ] Google OAuth server-side verify (blocked: needs client ID/secret — Q-3)
  - [ ] Email verification flow
  - [ ] Email-based **password reset** (change exists; reset needs a mail transport)
  - [ ] `PATCH /api/me` (display name / contacts / theme prefs)
- [ ] **E3** `internal/panel` 3X-UI adapter — Bearer auth (D-011), SSRF guard (scheme/DNS/private-block/redirect re-validation/rebinding), reused `http.Transport`, per-request timeout, bounded retry on idempotent GETs only, capability detection via `/server/status` + `openapi.json`, status mapping (§3.5)
  - [x] Base path handling (`<base>/panel/api/...`), Bearer client, reused transport, per-request timeout
  - [x] SSRF guard `ParseTarget` + `checkIP` (17-case rejection table) + no redirect following
  - [x] `Health()` / `Inbounds()` / `ListClients()` / `ClientExists()` / `AddClient()` / `DeleteClient()` / `SetClientEnabled()`
  - [x] Email validation (F-011 path-traversal fix)
  - [x] Live verification (read-only): health, inbounds, client list, fresh read → 3/3 PASS on 3.10.2
  - [ ] Live verification of writes (create/suspend/delete) — needs owner go-ahead on a non-production inbound
  - [x] Wired into the HTTP API: `POST /api/servers`, `GET /api/servers`, `POST /api/servers/:id/test`, `GET /api/servers/:id/inbounds`, `POST /api/servers/:id/disable`, `DELETE /api/servers/:id` — **live-verified against 3.10.2**
  - [x] Token never returned in API responses; verified absent from DB files (sealed)
  - [x] Server edit (PATCH) endpoint — **2026-10-24**: `PATCH /api/servers/{id}` accepts any subset of `{name, panel_url, token, allow_private_network}`, re-verifies the panel, and re-seals the token when the URL changes (AAD is bound to the URL). Live-verified (`200` + re-test; `404` unknown id).
- [x] **E4** Provisioning + quota — Free (distinct `email`+`subId` per client), Premium (tx + unique constraints), outbox + reconciliation (D-012), revocation with verified write or `REVOCATION_PENDING` — **done (unit-verified)**
  - [x] `domain` state machine: 9 VPN states + transition table; outage can only yield `SERVER_UNAVAILABLE`
  - [x] Deterministic identity `vyx-<localID>` → duplicates impossible by construction
  - [x] Quota counted inside the provisioning transaction (concurrency test: 2 racers → exactly 1 success)
  - [x] Outbox: enqueue-before-write, idempotent (`UNIQUE`), exponential backoff, `PENDING/RETRY/APPLIED/FAILED`
  - [x] Ambiguous write resolved by fresh read; "write landed but reported an error" is adopted, never duplicated
  - [x] Reconciler replays the exact original payload (limits preserved)
  - [x] Revoke → `REVOKED` only after a confirmed absence; otherwise `REVOCATION_PENDING`
  - [x] Endpoints: `GET/POST /api/vpns`, `/suspend`, `/resume`, `/revoke` — role + ownership enforced (404 for foreign ids)
  - [ ] Live provisioning against the real panel — **blocked**: the panel has a single production inbound (D-018)

- [x] **E5** Sync jobs — bounded worker pool, `/clients/list/paged`, per-server isolation, backoff + jitter, `usage_snapshots` lifetime delta math, server metrics — **DONE (live, D-023)**
- [x] **E6** Links/QR from `GET /clients/links/{email}`; inactive → hidden — **done 2026-10-24** (`GET /api/vpns/{id}/links`, `POST /api/vpns/{id}/ping`, live-verified); QR *image* rendering on the client page is the remaining UI piece
- [x] **E7** Serve built SPA via `embed.FS` + client-route fallback; FE stores wired to the API contract — **DONE 2026-10-24 (D-029)**: `internal/httpapi/spa.go` (`//go:embed all:dist`), 7 tests, `.\dev.ps1 -Package` builds and asserts a single binary (`/dashboard` 200, `/vpn/16` 200, `/api/nope` JSON 404, `/assets/nope.js` 404, no dev bypass)

### Stage F — Tests
- [x] Go unit (adapter vs `httptest` panel, status mapping matrix, SSRF table, quota concurrency, delta math, **password policy + change**) — **189 tests green**
- [x] Go integration (migrations, reopen, transaction rollback)
- [x] `go vet` / `gofmt` clean
- [ ] `-race` — blocked (no C compiler on this machine; run in Linux CI)
- [ ] FE: add `vitest`; form/error/empty/authz tests; typecheck + lint + build gates — lint/typecheck/build already green
- [x] Security: token never in responses/logs/bundle; IDOR matrix per role
- [x] Contract: replay the verified OpenAPI examples (`internal/panel`)

### Stage G — Live verification (partially executed)
- [x] Real 3X-UI 3.10.2 panel reached; health, inbounds, client list, fresh read verified
- [x] Integration CRUD through the HTTP API verified (add → ACTIVE, re-test, inbounds, audit)
- [ ] Verify: client add/get/update/disable/delete **live** — blocked by D-018 (production inbound with live users)
- [ ] Verify ambiguous-write reconciliation and revocation semantics live
- [ ] Provision a throwaway 3X-UI panel; create API token (Docker not installed on this machine)
- [x] Record actual results; anything not executed stays "not live-verified"

### Stage H — Final report
- [x] **`docs/FINAL_REPORT.md`** per `new_update.md` §13 (14 items, per-feature classification, rollback steps) — **written 2026-10-24**
- [x] `README.md` (root) — entry point with run/build/verify commands
- [x] `docs/INSTALL.md` — honest manual install (single binary + Caddy + systemd), gaps named
- [x] `docs/DOC_INDEX.md` — which document answers which question

## Phase 5 — Wire frontend ↔ backend

- [x] **Auth + Server integrations wired** to the real API (`frontend/src/lib/api.ts`, `authStore`, `serversStore`, `RequireAuth`, Vite `/api` proxy) — verified through the dev proxy
- [x] **Password change** wired end-to-end (API → store → Profile dialog)
- [x] **Login system complete** — `/setup` first-run wizard, `GET /api/setup/status` bootstrap detection, `AdminsPage` account API (`GET/POST /api/admins`, `PATCH /api/admins/{id}`), role switcher removed (roles are server-owned)
- [x] **Dev seeding** — `go run ./cmd/seeddemo` creates one account per role (guard: `VEYLIX_SEED=1`)
- [x] Replace the remaining mock stores with real API calls (`offersStore`, `adminsStore`, `paymentsStore`, `chatStore`, `notifStore`, `auditStore`, `peopleStore`, `vpnStore`) — **D-006 exit complete 2026-10-24: 11/11 stores API-backed, 0 mock**
- [x] Remove hardcoded sidebar nav counts (`App.tsx` — F-019) once the counts come from the API — done (badges derive from the same store list the page renders)
- [ ] Payments (manual confirmation, auditable), sales, quota screens — **note:** the API side shipped 2026-10-24 (`/api/payments` state machine, `/api/quotas/me`, per-role scoping) and the screens are wired; what remains is a per-offer retail price column, which no endpoint stores yet
- [ ] Chat (server-enforced participant access) — **shipped server-side 2026-10-24** (`/api/chat/*` participant checks run before any message lookup); open item is the client-facing storage budget (D-027.6)
- [ ] Notifications — **shipped 2026-10-24** (`/api/notifications`, audit-driven fan-out); open item is the dev-mode duplicate-key console warning (F-031)
- [ ] Theme persistence via account
- [ ] Error/stale/partial-failure states bound to real backend
- [ ] OWNER LIVE TEST (end-to-end per role)


## Phase 6 — Production packaging
- [ ] Caddy template + systemd unit (least privilege)
- [ ] CLI: vpn-platformctl (status/health/logs/backup/restore/upgrade/rollback/domain set/uninstall)
- [ ] Installer: one-line, idempotent, DNS/ports validation, first Super Admin bootstrap (no default password)
- [ ] CI workflow (lint/test/build produces bundled assets)
- [ ] Release workflow (linux/amd64 + arm64) + checksums
- [ ] docs: INSTALL, UPGRADE, DOMAIN_CHANGE, BACKUP_RESTORE, TROUBLESHOOTING, QA_REPORT
- [ ] Clean Linux VM install + HTTPS test
- [ ] OWNER LIVE TEST (production flow)

## Phase 7 — QA & hardening
- [ ] Acceptance tests 1–39 (Bible §9 + §18)
- [ ] Permission-negative tests; quota concurrency tests; outage simulations
- [ ] Cross-theme UI checks; Sinhala/English rendering
- [ ] 500 MB / 1 vCPU benchmark report
- [ ] Backup → restore rehearsal
- [ ] Final delivery checklist (Bible §19)

---

## Test strategy (`new_update.md` §9)

| Layer | Tooling | Gate |
|---|---|---|
| Go unit | `go test ./...` + `net/http/httptest` fake panel | every PR |
| Go race | `go test -race ./...` | Stage F + CI |
| Go static | `go vet ./...`, `gofmt -l .` | every PR |
| Go integration | temp-dir SQLite: migrate → write → reopen → rollback | every PR |
| FE static | `tsc -b`, `oxlint` | every PR (already green: 0/0) |
| FE build | `vite build` | every PR |
| FE unit | `vitest` + `@testing-library/react` (to be added) | Stage F |
| FE manual | preview on localhost: forms, loading, empty, error, authz, mobile 360/768/1280 | D-003 owner live test |
| Contract | replay verified OpenAPI request/response examples | Stage F |
| Security | token-leak grep gate (bundle + logs), IDOR matrix per role, SSRF table | Stage F |
| Live | throwaway 3X-UI panel + real token | Stage G (blocked) |

**Rule:** mocks prove code shape only — never live compatibility (`new_update.md` §9). Severity of any finding is recorded in `IMPROVEMENT_LEDGER.md` and must not be downgraded to obtain a green run.


---

## Progress log
- 2026-10-24: **Single binary + documentation truth pass (owner: "software eka hariya build wenna one, doc tika balala adu dewa thiranaya karanna").**
  - **E7 shipped — D-029.** `go build ./cmd/veylixd` now produces **one** 19 MB artifact
    that serves the UI *and* the API. Before this, `GET /dashboard` on the backend was a
    **404**, so "install Veylix" meant Go + Node + a proxy with no documented path.
  - **Four routing rules, each with a test** (`internal/httpapi/spa_test.go`, 7 tests):
    `/api/*` keeps the JSON 404 (the SPA can never swallow an API path) · real files are
    served as themselves with `immutable` caching under `assets/` · a **missing file stays
    404** (returning `index.html` for a missing script is what produces "Unexpected
    token '<'") · every other path serves the shell so `/vpn/16` survives a hard refresh.
    `HEAD` has no body; non-GET to a UI path is 405.
  - **Absence is stated, not faked.** The embed target is committed as `.gitkeep` only, so
    `go build ./...` works on a fresh clone; with no build, `/api/ready` reports
    `"ui": "not_embedded"` and UI paths answer a JSON 404 naming the missing step. With a
    build it reports `"ui": "embedded:N_files"`. `index.html` decides, not a file count.
  - **`.\dev.ps1 -Package`** builds the SPA, **wipes** the embed target (no stale hashed
    asset can linger), copies, builds, then proves the artifact on a throwaway port:
    ```
    PACKAGED OK - one binary, one port (18080)
      /api/ready  -> ready  (ui: embedded:7_files)
      /dashboard  -> 200 text/html
      /vpn/16     -> 200 (SPA deep link)
      /assets/nope.js -> 404 (missing file stays 404)
      dev-bypass in bundle -> absent (ok)
    ```
    A packaging step that reports success without *running* the binary is how a broken
    release ships, so the probe is part of the script.
  - **PowerShell gotcha fixed:** Vite writes its chunk-size hint to stderr and
    `$ErrorActionPreference='Stop'` turns that into a failed command — the first `-Package`
    run "failed" on a successful build. The step now redirects to a file and judges by exit
    code plus a real `dist/index.html`.
  - **Docs brought back to truth.** ROADMAP said *"Milestone 5-B — 🔴 not started"* while the
    list beneath it was almost entirely `[x]`; Milestone 5 still showed usage/link work as
    unbuilt; Milestone 6 was unheaded after all 11 stores shipped. All corrected, and the
    self-contradiction is now noted inline so a reader can see why it changed.
  - **New documents:** `README.md` (root — was absent, so a newcomer had no entry point),
    `docs/INSTALL.md` (manual single-binary + Caddy + systemd, with an explicit "not automated
    yet" table), `docs/FINAL_REPORT.md` (`new_update.md` §13's 14 items **and** the mandated
    per-feature classification), `docs/DOC_INDEX.md` (22 docs, grouped by question).
  - **Evidence:** `go build`/`vet`/`gofmt` clean · **`go test ./... -count=1` → 9/9 packages**
    (286 test functions) · `tsc -b` clean · `oxlint` 0/0 · `vite build` ✓ · `npm run audit` 0 ·
    audit self-test 10/10 · packaged-binary route matrix above.

- 2026-10-24: **Live preview + UI truth pass (owner: "live preview eka start karanna, eelagata develop wenna one deewal thiranaya karanna, iwara unama doc update karanna").**
  - **Live preview, one command.** `.\dev.ps1` is now a real operator tool: it
    loads `.veylix-secrets\dev-env.ps1`, starts the backend, proves the D-022 bypass
    by calling `/api/me`, starts Vite, and only then prints the URLs.
    New switches: `-Log` (both processes to files), `-Lan` (phone/other device),
    `-NoBrowser`, `-Status` (ports + health + dev-auth state). Guide:
    `docs/LIVE_PREVIEW.md`.
  - **A hang found and fixed while using it.** The wait loop polled
    `$backendProc.HasExited`, which stays `false` forever when the child's output
    is redirected (PowerShell keeps the reader handles open) — the first `-Log`
    run never returned. It now waits on `/api/health`, which is the real signal.
  - **Port 5173 is pinned** (`strictPort`). Vite's hop to 5174 produced a page
    that loads while every POST answers `403 cross_origin`; a hard error is the
    honest outcome.
  - **UI truth pass — 12 lies removed, each now gated.** `npm run audit` scans
    `src/` for claims the backend contradicts ("demo data", "no API yet",
    "data is mock", "issued in Stage E6", hard-coded LKR wholesale/retail prices)
    and fails on any hit; `npm run audit:selftest` proves the rules still fire
    (6 positive + 4 negative cases — it caught a gap in the Stage-E rule during
    this very pass).
    Fixed: **RetailPricePage** (three invented plans + a Save button that only
    toasted, replaced by the real PREMIUM catalogue and confirmed revenue),
    **DashboardPage** (four invented panels, a count-up to 1,284 clients and a
    random "client connected" ticker, replaced by `/api/servers`,
    `/api/sync/status`, `/api/servers/{id}/metrics`, `/api/clients`, `/api/vpns`),
    **AdminOverviewPage** ("no API yet (P5)/(P6)" long after both shipped),
    **SaleFlowPage** (two stale stage references), **LoginPage/RegisterPage**
    ("demo build, data is mock (D-006)", false since the mock stores were deleted),
    **ClientDashboardPage** (an empty-state that blamed a missing API).
  - **A page-level smoke that needs no test runner.** `npm run verify:ui -- /path`
    drives headless Chrome through CDP, waits for the shell to settle (not a fixed
    sleep), and reports the title, every metric card's label/value/delta, chart
    marks, empty-state copy and the first table rows — failing on any console
    error. `VEYLIX_DEV_ROLE=admin|client` seeds the dev chip so role-only screens
    can be reviewed.
  - **Live evidence (real panel `indian1`, 3X-UI 3.10.2):** `/dashboard` →
    `Panels 1/1 · Clients 1 · Active VPNs 5 · Healthy 100%`, CPU 4.0%, chart
    rendered, **0 console errors**; `/retail-price` → the real offer
    (`Phase A Premium · 100 GB · 30 days · server 1`), no invented prices;
    `/admin` → `Assigned servers 1 · Clients 1 · Confirmed revenue 0.00`.
  - **Gates:** `go build ./...` · `go vet ./...` · `gofmt -l` clean ·
    `go test ./... -count=1` **9/9 packages pass** · `tsc -b` clean ·
    `oxlint` **0 warnings / 0 errors** · `vite build` OK · `npm run audit` OK ·
    audit self-test 10/10.
  - **Found, not fixed (recorded as F-031):** React's dev-only
    `two children with the same key, 0` warning appears for the reseller/client
    personas while several notifications exist. It reproduces on untouched
    screens, does not occur in a production build, and hiding items makes it
    disappear — so the next session starts from the notification list. It is
    documented rather than suppressed.
  - **Next:** the docs truth pass (GAP_ANALYSIS board, API_CONTRACT planned→shipped,
    QA_REPORT, FINAL_REPORT), then the remaining product gaps in
    `docs/API_CONTRACT.md` (HWID routes, credential change, FREE catalogue,
    chat storage budget, D-028 panel email), then packaging.

- 2026-10-24: **Milestone 5-B step 9 SHIPPED — release with a 3-day undo (D-027).**
  - **What the rule actually is.** "Storage full" means the VPN's *traffic quota* is
    exhausted — not the host's disk. A depleted VPN can be released, which opens a
    **3-day window with undo**; after that Veylix removes **its own record** and
    nothing else. The owner's words were explicit: *"3x ui panel eken ewa ain wenne
    nane"* — the panel client is never deleted by this path.
  - **A new `internal/releases` package, deliberately not more `provisioning`.** The
    rule is about the RECORD, not the panel: nothing in this package writes to 3X-UI.
    Keeping that boundary visible is what stops a future change from "helpfully"
    deleting the panel client when the row goes away.
  - **Owner-only release.** An operator releasing a client's VPN would destroy
    property they do not own, so it is **403**, not a convenience. Restore is scoped the
    same way and answers **404** for anybody else's release id.
  - **Two guards that matter:** a second release while one is pending is **409**
    (otherwise a client restarts the clock forever and never loses a depleted VPN), and
    a release **does not refund** the FREE monthly claim (D-027.4) — otherwise
    release→re-claim becomes an unlimited-free-VPN loop.
  - **Removal is one transaction.** The local VPN row, its assignment and the free-claim
    link go together, and the release row is stamped `removed_at`. A half-removed
    release would leave an assignment pointing at a VPN that no longer exists.
  - **The audit sentence is the feature.** `vpn.purge` records
    `panel_client_kept: true` and *"the 3X-UI client was not deleted"*, so a reader a
    month later can tell "we deliberately kept it" apart from "we forgot to clean up".
    `release.panel_client_kept` is on the wire for the same reason.
  - **Flaky test caught and fixed.** The first version compared the service's real clock
    against the test's captured clock and failed intermittently (`want …025, got …026`).
    The release path now takes the clock from the service, so `Now()` is injected once
    for release, restore and purge alike. Verified with `-count=3`.
  - **UI.** A depleted VPN shows "Out of traffic? … 3 days to undo" with a Release
    button that becomes "Undo release (N days left)". The countdown reads the clock
    once per render (`useState(() => Date.now())`) rather than inside the JSX, which
    also removes an `oxlint` purity warning.
  - **Evidence:** `go test ./... -count=1` → **all 9 packages pass** (1 new:
    `TestReleaseGraceAndLocalOnlyRemoval`, which walks release → restore → expire →
    purge with a moved clock and asserts the VPN row, its assignment and the audit
    context) · `go vet`/`gofmt` clean · `tsc -b` clean · `oxlint` **0 warnings / 0
    errors** · `vite build` OK · **live smoke** (`smoke-releases.ps1`): empty timeline
    200, unknown VPN release **403** (operator), unknown release restore **404**.
  - **Remaining in 5-B:** step 10 (chat storage budgets), step 12's chart toggles,
    step 13 (dashboards).

- 2026-10-24: **Milestone 5-B step 11 SHIPPED — a client can now actually connect.**
  The gap this closed was the biggest one left: `GET /api/vpns/{id}/links` answered
  404 and the client screen showed a demo QR, so every VPN in the product was
  unusable by design. Step 11 makes "here is your VPN" true.
  - **`GET /api/vpns/{id}/links`** returns the panel's own URIs, **passed through
    untouched** — rewriting a `vless://` string produces something that looks
    correct and cannot connect. Ownership is checked by `loadVPN`, so a VPN that
    is not the caller's is **404** (ids are not probeable). Links are fetched
    fresh on every request; caching a credential would leave a link on screen
    that silently stopped working after the panel rotated it.
  - **Three states, three meanings.** `source: "unavailable"` = the panel could
    not be read (an outage is **not** "no link"); `links: []` = the panel answered
    and had no URI for that client; `links: [...]` = a connectable configuration.
    Collapsing these is how a UI ends up telling a paying customer their VPN
    works when it does not — so the distinction is on the wire, not in the UI.
  - **`POST /api/vpns/{id}/ping`** times a TCP connect to the panel host (not
    ICMP: that needs root, and a TCP handshake is what actually decides whether
    the panel answers). The response carries `scope: "server_reachability"` **in
    the payload**, so a screen physically cannot present it as the user's own
    latency. Unreachable reports **-1**, never 0, which would read as "instant".
  - **Detail page** now renders Copy link / Open in app from the real URI, shows a
    distinct warning for an outage vs. a missing link, and renders the ping with
    its honest label.
  - **Evidence:** `go test ./... -count=1` → **all 9 packages pass** (1 new:
    `TestVPNLinksAndPingScope`, covering owner-200 / stranger-404 for BOTH routes,
    the outage `source`, and `latency_ms=-1`) · `go vet`/`gofmt` clean · `tsc -b`
    clean · `oxlint` **0 warnings / 0 errors** · `vite build` OK · **live smoke on
    the real panel**: `GET /api/vpns/16/links` → **200** with a genuine
    `vless://a033313e-…@difanx4f3.difanx1.ggff.net:443?…#(IN)-free_2-vyx-16`,
    `source: panel`; `POST /api/vpns/16/ping` → `{reachable:true, latency_ms:1,
    scope:"server_reachability"}`; a REVOKED VPN correctly answered
    `links: [], source: "unavailable"` instead of pretending it had none.
  - **Remaining in 5-B:** step 9 (release + 3-day restore, D-027), step 10 (chat
    storage budgets), step 12's chart-type/range toggles, step 13 (dashboards).

- 2026-10-24: **F-019 fixed + notification producers wired (the two loose ends Sprint 1 left).**
  - **F-019 — nav counts.** `App.tsx` hardcoded `count: 30 / 12 / 8 / 2 / 24` on the Servers,
    Offers, Admins, Assigned-servers and Clients items. Those were the last invented
    numbers in the chrome, and they contradicted the very pages they pointed at. Each
    badge now derives from the SAME store list its destination renders (`servers`,
    `offers`, `admins`, `clients`, plus the chat unread total), is fetched only for the
    role that can see it (a client never asks for the reseller list, which would just
    earn a 403), and is **omitted** rather than shown as `0` when the list is empty.
  - **Notification producers.** A real inbox with no producer is a screen that is
    always empty — the last structural untruth left in the UI. Rather than adding a
    `CreateNotification` call to a dozen handlers (which silently drift), the fan-out
    hangs off the **audit logger**, which already sees every privileged action with its
    actor and target: `audit.Logger.WithNotifier(store)` → `notify()` maps an action to
    a kind/title and delivers to (a) the actor on any non-OK result — "your action did
    not happen" is the message users otherwise never receive — (b) the SUBJECT for
    account-level events (`user.*`, quota), (c) every active reseller for
    platform-level events (credentials rotated, server disabled/removed, VPN absent),
    and (d) the actor for a set of actions worth a receipt (provisioning, payments,
    offers, servers). Delivery is **best-effort and never returns an error**: a
    notification must not roll back the operation it describes.
  - **New audit titles** so an entry is readable rather than a dotted code:
    `Payment method added/updated`, `Conversation locked state changed`,
    `Pinned message changed`, plus the existing payment/server/VPN ones.
  - **Evidence:** `go test ./... -count=1` → **all 9 packages pass, 260 tests**
    (1 new: `TestAuditFansOutIntoTheInbox`, which asserts delivery through the real
    router and proves the fan-out is generic by checking TWO different actions) ·
    `go vet`/`gofmt` clean · `tsc -b` clean · `oxlint` **0 warnings / 0 errors** ·
    `vite build` OK · **live smoke** (`smoke-notifications.ps1`, through the running
    preview): `unread 0` → create a payment method (201) → `unread 1` with
    `{"title":"Payment method added","body":"payment_method 1 · kind=EZCASH"}` →
    mark read → `unread 0`.
  - **Next: Milestone 5-B steps 9–13** — release/restore (D-027), links/QR + ping (D-026),
    VPN detail UI, dashboards. Those are the remaining product gaps; the wiring phase
    is finished.

- 2026-10-24: **Milestone 6 COMPLETE — P7 Chat shipped, and there are now ZERO mock stores.**
  - **Why it mattered.** The chat transcript lived in `localStorage`: two devices
    disagreed, clearing the cache destroyed the conversation, and "locked" /
    "delete for everyone" were decoration no server ever saw. A transcript is
    evidence, so it now lives in SQLite.
  - **Schema (migration `0007`, additive only).** `chat_conversations.pair_key`
    with a **UNIQUE** index (`min|max` of the two ids) so opening the same thread
    twice returns the same row even under a race; `locked_at`/`locked_by`;
    `chat_participants.last_read_at` (NULL = never opened, which is *not* "read at
    epoch 0"); `chat_messages.edited_at` + `original_body` (an edit preserves what
    was said); `chat_message_hidden`, `chat_message_saved`, `chat_message_important`
    for the per-viewer flags; `pinned_message_id` on the conversation. A backfill
    fills `pair_key` for pre-existing rows before the index can be violated.
  - **Access is one SQL join.** Every read joins `chat_participants` on the session
    id, so a non-participant simply gets no rows. The Go tests prove the cases that
    matter: an unrelated client, an unrelated reseller **and the super admin**
    cannot read a private transcript (T-16).
  - **Enumeration oracle found and closed.** `requireMessageParticipant` first read
    the message, then checked participation — so a stranger could tell a real
    message id (403) from a fake one (404) and map other people's conversations.
    The participant check now runs FIRST and both answers are 404.
  - **Honest semantics.** "Delete" hides the message for the caller only — the
    peer's copy survives, because quietly erasing the other side's record is a
    lie. Locking blocks new messages (409 `chat_locked`) but never hides the
    transcript. Pinning/locking is staff-only and **audited** (`chat.pin`/`chat.lock`).
  - **Frontend.** `chatStore` rewritten (no `SEED`, no `persist`): the UI's shape
    (`text`, `createdAt`, `time`) was kept so the components did not have to be
    rewritten, but `readBy`/`removedBy`/`savedBy` maps are gone — those answers are
    the server's now. `roleOf(email)` (a guess from the email string) is deleted:
    staff controls follow the **session** role. `peopleStore` gained the numeric
    `id` because `POST /api/chat/threads` takes a peer id, not an email.
  - **Evidence:** `go test ./... -count=1` → **all 9 packages pass, 259 tests**
    (2 new: participant-only access, full message lifecycle) · `go vet`/`gofmt`
    clean · `tsc -b` clean · `oxlint` **0 warnings / 0 errors** · `vite build` OK ·
    **live smoke through the running preview** (`:5173` proxy → `:8080`, schema
    **7**): thread opened twice → same id · send 201 · edit 200 (stamps
    `edited_at`) · save/important 200 · pin/lock 200 · send into a locked thread
    **409** · unlock 200 · DELETE **204** and the message gone from the list ·
    ghost thread 404 · self-peer 403 · blank body 422.
  - **Still open, and stated as such:** sidebar nav counts are hardcoded (F-019);
    notification *producers* are not wired yet, so an inbox is real but stays empty
    until panel/lockout events write to it; chat has no live transport (the UI
    polls by re-loading a thread on mount — an SSE channel is the next step if
    real-time is wanted).
  - **Next: F-019 nav counts + notification producers**, then Milestone 5-B steps
    9–13 (release/restore, links/QR, VPN detail UI, dashboards).

- 2026-10-24: **Milestone 6 (part 1) COMPLETE — P6 Billing, P8 Notifications and the
  account directory are now real, and three mock stores are gone.**
  - **Backend.** New stores `store/notifications.go`, `store/directory.go`,
    `store/payments.go` and routes `notification_routes.go`, `directory_routes.go`,
    `payment_routes.go` + `payment_method_routes.go` + `payment_record_routes.go`,
    all wired in `cmd/veylixd/main.go`. **17 new endpoints**
    (`/api/notifications*` ×4, `/api/directory`, `/api/payment-methods*` ×3,
    `/api/payments*` ×9).
  - **Why a state machine, not a boolean.** The old store let any screen set a
    payment to "confirmed" locally, so the UI could claim money was received when
    no record existed. Now one guarded transition (`transitionPayment`) is the
    only writer: an illegal jump is `409 illegal_transition`, not a silent rewrite,
    and `confirmed_by`/`confirmed_at` are stamped by the operator who did it. Every
    transition writes an audit entry (`payment.create|reference|confirm|reject|refund|cancel`,
    `payment_method.create|update`).
  - **Money is integer cents** end to end (`amount_cents`); the UI converts at the
    boundary (`Math.round(units * 100)`) and formats only at render time.
  - **Privacy.** An inbox is scoped to the session's own `user_id`; a foreign id
    answers **404**, never 403, so notification ids cannot be enumerated. A payment
    that exists but is not the caller's is also 404 for the same reason.
  - **Scope rule lives in SQL,** not the UI: `Directory()` returns everyone to a
    super admin, own clients + super admins to a reseller, super admins + own
    sellers to a client — a reseller can never enumerate another reseller's
    customers.
  - **Bug found and fixed during the live smoke:** `POST /api/payments` returned
    **500** under the dev-auth bypass (D-022). The bypass principal has no `users`
    row, so the FK failed and the server blamed itself. Now a typed
    `ErrNoPayerAccount` → **403 `no_payer_account`** with an actionable message,
    because "who paid" must never be invented. Regression test added.
  - **Frontend.** `notifStore`, `peopleStore`, `paymentsStore` rewritten against
    the API (no `SEED` arrays left in them); `PaymentsPage` rebuilt as the operator
    queue (offers only the transitions the server allows), `BuyPremiumPage` now
    offers **real offers and real destinations** instead of two invented plans,
    `HistoryPage` reads the account's own payments, `NotificationsPage`/`Topbar`
    render real timestamps via `lib/time.ts` (`relativeTime`) instead of the fake
    "12m ago" strings.
  - **Evidence:** `go test ./... -count=1` → **all 9 packages pass (257 tests, was
    256; 4 new: payment lifecycle + guarded refusals, inbox privacy, directory
    scope, no-payer refusal)** · `go vet` clean · `gofmt` clean · `tsc -b` clean ·
    `oxlint` **0 warnings / 0 errors** · `vite build` OK · **live smoke on the
    shipped binary**: bootstrap → create method (201) → create payment (201
    PENDING) → reference (200 AWAITING_CONFIRMATION) → confirm (200 CONFIRMED) →
    duplicate confirm (**409**) → refund (200 REFUNDED) → audit shows all six
    entries; validation 422/404 as specified.
  - **Not done, and stated as such:** chat is still local-only (P7 — tables exist,
    no API); sidebar nav counts are still hardcoded (F-019); notification
    *producers* (panel/lockout events) are not wired to `CreateNotification` yet,
    so an inbox is real but empty until something writes to it.
  - **Next: P7 Chat API** (the last mock store), then F-019 nav counts, then
    Milestone 5-B steps 9–13.

- 2026-10-23: **P5 COMPLETE (live + unit) — reseller scope + client list.** Two endpoints that had no write/read path at all now exist: **`GET/PUT /api/admins/{id}/servers`** (idempotent *replace*, super-admin only, audited as `admin.servers_assign`) and **`GET /api/clients`** (super admin → all; reseller → only clients who own a VPN they sold; client → **403**). New `internal/store/assignments.go`, `internal/httpapi/client_routes.go`, and `auth.Service.AdminServers/SetAdminServers` with `ErrNotAnAdmin`/`ErrServerNotFound`. Frontend: new `clientsStore`, `adminsStore` gained `adminServers`/`setAdminServers`, and **`AdminsPage` regained the "Assigned servers" column** — this time backed by real data via a scope dialog. `AdminClientsPage` and `SaleFlowPage` lost their hard-coded demo client lists and invented price tables; both now pick from `GET /api/clients` and the active PREMIUM catalogue. **Live proof:** `PUT {"server_ids":[1]}` → the reseller's own `GET /api/servers` immediately returned `indian1` (3.10.2) — previously every reseller saw **zero** servers forever. `/api/clients`: super admin 1, fresh reseller `[]`, client 403. **Evidence: `go build`/`vet`/`gofmt` clean · 189 tests pass (was 181) · `tsc` 0 · `oxlint` 0/0 · `vite build` OK.** **Next: P10 (links/QR, Stage E6).**
- 2026-10-23: **PHASE A / P3 COMPLETE (live + unit) — offer catalogue.** The endpoint that blocked everything downstream now exists: `GET/POST /api/offers` + `PATCH /api/offers/{id}`. New `internal/store/offers.go` (role-scoped list, create, partial update — never a delete, because assignments reference the offer) and `internal/httpapi/offer_routes.go` (validation, `PermOfferManage`, audited writes via new `offer.create`/`offer.update` actions). Frontend: `lib/api.ts` gained `listOffers`/`createOffer`/`updateOffer`; `offersStore` (3 seeded fakes → real); `OffersPage` gained the `kind` selector and a **real inbound picker** from the panel, and lost the invented "Claims" column. **End-to-end proof:** `POST /api/vpns {"offer_id":1}` → **201** with a real instance whose `expires_at` is exactly the offer's `duration_days` later — impossible before, since `offer_id` could only come from hand-written SQL. **Role rules live-verified:** client sees 1 of 2 offers (`FREE` only), `POST` → 403, deactivating → client count 0. **Evidence: `go build`/`vet`/`gofmt` clean · 189 tests pass (was 173) · `tsc` 0 · `oxlint` 0/0 · `vite build` OK.** **Next: P5 (server assignment + clients API).**
- 2026-10-23: **STEP 1 COMPLETE — truth pass (Phase 5, first slice).** Wired the three stores whose backend already existed and deleted the fabricated data behind them. `lib/api.ts` gained a **VPN namespace** (`listVpns`, `createVpn`, `suspend`, `resume`, `revoke`, `vpnUsage`) plus `serverMetrics`/`syncStatus` and the matching `ApiVpn` / `ApiVpnUsage` / `ApiSyncServer` / `ApiMetricPoint` types — before this, `lib/api.ts` had **zero** `/api/vpns` references while the backend served five. `vpnStore` (138 → ~200 lines) lost its two seeded VPNs and the `vless://demo-…` link generator; it now loads from the API and exposes `suspend`/`resume`/`revoke`/`loadUsage`. `auditStore` lost its three invented entries and its `log()` method — the trail is server-owned and append-only, so a client-side writer was a fiction. `adminsStore` lost four seeded resellers and gained real create/suspend/quota. **Removed from the UI rather than faked:** `plan` (no such column), `link`/QR (Stage E6), `servers` per admin (no endpoint, P5), `quotaUsed` per admin (not returned by the API), `purchases` (P6). Pages updated: ClientDashboard, VPNDetail (demo QR + fake link deleted), History, Audit, Admins, AdminOverview, AdminServers, AdminClients, SaleFlow, Chat. **Evidence: `tsc -b` 0 errors · `oxlint` 0/0 · `vite build` OK (487.99 kB) · bundle grep: `Tokyo Premium` 0, `demo.veylix` 0, `18,400 LKR` 0 · backend untouched, 189 tests still pass · live shapes verified against the running binary (`/api/vpns` `[]`, `/api/admins` real row, `/api/audit` real logins).** **Next: STEP 2 — offers API.**
- 2026-10-23: **System & process maps written (`SYSTEM_MAP.md`, `PROCESS_MAP.md`).** The platform is now split two ways: **8 system modules** (S1–S8) and **10 business processes** (P1–P10) × **3 actors** (super_admin / admin / client). Both documents are evidence-based — every status cites an endpoint, a file, or a grep with a count of zero. **Headline finding: only 2 of 10 processes work end-to-end (P1 Identity, P2 Server Onboarding), and both belong to the operator — zero customer-facing flows work.** The blocker chain is `P3 offers (no API) → P4 VPN (backend done, `lib/api.ts` has no `/api/vpns` at all) → P10 links`. Also recorded: 6 tables with zero Go references, 21 of 23 pages on mock data, 28 API endpoints, 5 828 production Go lines. Counts were re-measured after two of my own first-pass figures proved wrong (endpoints 24→28, prod LOC ≈10 100→5 828). **Next: wire the 3 stores whose API already exists** (audit, admins, vpn).
- 2026-10-23: **Stage E5 COMPLETE (live + unit) — sync jobs, usage snapshots, server metrics (D-023, F-010 closed).** New `internal/sync`: `Fold` (reset-proof lifetime math), per-server service (bounded 3-way concurrency, 60 s per-server deadline, 200/page × 50 pages), and a scheduler (5 min default, jittered first tick, **no-overlap guard**, graceful `Stop`). New migration `0005_sync.sql` (schema 4→5) adds `ux_usage_instance_taken` + `ux_metrics_server_taken` so a repeated tick upserts instead of duplicating. New read-only routes: `GET /api/vpns/{id}/usage` (owner/seller scope, IDOR-tested), `GET /api/servers/{id}/metrics` (labelled `server_wide`, assigned-admin scoped), `GET /api/sync/status`. New env: `VEYLIX_SYNC_INTERVAL` / `_TIMEOUT` / `_PAGE_SIZE` / `_MAX_PAGES`. **Two real bugs were caught by my own tests in the first draft of `Fold`** — a double-count on reset (`prevUsage` added to `prev`) and a stale `resetCount` masking a reset — both fixed and regression-tested. Sync is READ-ONLY, so it does not need D-018 and was verified against production. **Evidence: `go build`/`vet`/`gofmt` clean · 189 tests pass (was 141) · live: 3 clean ticks on `indian1`, `stale:false`, real CPU/RAM recorded.** **Next: E6** (links/QR).
- 2026-10-23: **QA review + Option B fixes (F-023, D-021/D-022 renumbering).** Independent re-run of every gate on this machine: `go build`/`go vet`/`gofmt` clean · **141 tests pass, 0 fail** (was documented as 131) · `npm run lint` 0/0 (100 files) · `npm run build` OK. Live smoke against the real binary + real SQLite: health/ready/404 envelope, and a full three-role login + RBAC matrix. **Found F-023:** `GET /api/quotas/me` returned **403** to a client, contradicting `API_CONTRACT.md` and breaking the single `myQuota()` call site. Fixed to return `{0,0,-1}` (D-021) with three new handler-level tests — which also exposed that `newHarness` never mounted `AccountRoutes` at all, so the endpoint had **no** coverage. Doc drift corrected: `QA_REPORT.md` §1 per-package counts re-derived and now sum to the total (they previously did not), `QA_PLAN.md`, `GAP_ANALYSIS.md`, `API_CONTRACT.md`. **Duplicate decision id repaired:** two different entries were both `D-019`; the dev-auth bypass is now `D-022` (20 references across docs + source updated). **Next: E5.**
- 2026-10-21: Phase 0 created. Owner approved frontend-first order + live-test workflow. Greenfield confirmed (repo had no app code).
- 2026-10-21: Phase 0 + Frontend scaffold (1.1) + design system basics (1.2 partial) built. Build passes; lint clean (1 low-priority warning). **Awaiting owner live test** before next step.
- 2026-10-22: Theme palette rebuild + **40 sub-themes** (Community Legends) shipped. UI 2.0 shipped: ⌘K command palette (in-house), WAI-ARIA dialog focus management, tablist arrow keys, table sorting, sidebar rail, alive dashboard, skeleton/toast polish. Contrast audit across all 40 variants (AA).
- 2026-10-22: Docs catch-up (duplicate Phase 2 block removed; roadmap checkboxes synced with reality). **Auth screens (mock, D-006) started — Phase 2 underway.**
- 2026-10-22: **UI 2.1** shipped — chart interactivity (tooltips/crosshair/legend toggles on Line/Bar/Donut), View Transitions navigation, notifications popover (`notifStore` mock), `docs/UI_INVENTORY.md` catalog created (living doc). Lint 0/0, build ✓.
- 2026-10-22: **Batch A (Super Admin core)** shipped — Server integrations page (add/edit/test-connection/disable/remove States per Bible §3.1–3.3), VPN offers page (Free offer rules form + claims), Admin management page (create/suspend/quota). Mock stores: `serversStore`/`offersStore`/`adminsStore` (D-006). Lint 0/0, build ✓.
- 2026-10-22: **M3 COMPLETE** — Batch B (Payments+confirm/audit trail), C (Client dashboard/VPN detail/QR/useless-Buy-premium/history), D (Admin role NAV/assigned-servers/clients/sale-flow-quota-checked/retail-price), E (ChatPanel+chatStore). Role-filtered NAV via authStore role. `RequireAuth` guards routes. All placeholders replaced. Routes: 20+ real screens.
- 2026-10-23: **Stage A COMPLETE** — requirement intake (`docs/new_reqierment/new_update.md`). Full repo/docs/git audit (pre-existing owner work preserved), toolchain + build/lint baseline recorded (**test baseline = none**), 3X-UI contract verified against the official OpenAPI spec (release **v3.10.2**, 55 schemas, 60+ endpoints) → `docs/3XUI_API_CONTRACT.md`. New docs: `GAP_ANALYSIS.md`, `IMPROVEMENT_LEDGER.md` (F-001…F-010). Decisions **D-010** (order), **D-011** (Bearer auth), **D-012** (paged API + idempotency), **D-013** (CGO-free SQLite).
- 2026-10-23: **Stage B COMPLETE** — `backend/` Go module (`cmd/veylixd`, `internal/{config,httpapi,domain,store,panel}`), SQLite with 3 embedded migrations, health/ready + envelope layer, graceful shutdown. Docs: `ARCHITECTURE.md`, `API_CONTRACT.md`, `SDLC_PLAN.md` (D-014), `QA_PLAN.md` (D-015), `QA_REPORT.md`. Decisions **D-014** (SDLC gates), **D-015** (QA honesty), **D-016** (AES-GCM token at rest, fail-closed key). Evidence: build/vet/gofmt clean · 13/13 tests · runtime smoke 200/200/404.
- 2026-10-23: **Stage E2 (auth/RBAC) + E3 API wiring COMPLETE & LIVE-VERIFIED.** Packages: `internal/auth` (argon2id, AES-GCM sealer w/ AAD binding, permission matrix, ownership, sessions, rate limiter), `internal/audit`, `internal/integrations` (real panel add/verify/inbounds/disable/remove, sealed tokens). Endpoints: `/api/auth/*`, `/api/me`, `/api/setup/bootstrap`, `/api/servers/*`, `/api/audit`, `/api/quotas/me`. Migration `0004_quota_limit.sql`. Live: panel → `ACTIVE` (3.10.2), inbounds listed, re-test OK, audit written, client 403, anon 401, token sealed. Defects fixed: F-011, F-013, F-014.
- 2026-10-23: **Frontend wired to the real backend (D-006 exit started).** New `frontend/src/lib/api.ts`, Vite `/api` dev proxy, `authStore` rewritten against `/api/auth/*` + `/api/me`, `RequireAuth` waits for session restore, `serversStore` + `ServersPage` use the real adapter. Demo role-login buttons removed. Verified through the dev proxy (SPA, health, bootstrap, login, `/api/me`, real panel add → `ACTIVE`, inbounds).
- 2026-10-23: **Stage E4 COMPLETE (unit-verified) — provisioning, quota, outbox, reconciliation.** New `internal/domain` (VPN state machine + deterministic panel identity) and `internal/provisioning` (create with transactional quota, outbox-before-write, fresh-read confirmation, reconciler, suspend/resume/revoke). New endpoints `GET/POST /api/vpns` and `POST /api/vpns/{id}/{suspend,resume,revoke}` with role + ownership enforcement. **Tests: 115 pass** (13 provisioning incl. quota race, duplicate prevention, "never REVOKED on outage"; 7 domain). Live: server `ACTIVE`, inbound cached, `/api/vpns` scoped, client 403/404, anon 401. **Live provisioning writes blocked** — the panel has one production inbound with live users (D-018). **Next: E5** (sync jobs + usage snapshots) → E6 (links/QR) → E7 (SPA embed).
- 2026-10-23: **Password hardening + doc re-sync.** New `ValidatePasswordPolicy` (12–512 runes, no repeated rune, no straight sequence, blocklist) applied by `HashPassword` itself so no call site can bypass it (**F-017**). New `auth.Service.ChangePassword` + `POST /api/auth/change-password` that revokes every session and re-issues the caller's cookie (**F-016** — `RevokeAllForUser` was dead code until now). Frontend: `lib/passwordPolicy.ts` mirrors the Go policy, RegisterPage hint no longer says "Looks good ✓" at 8 chars (**F-015**), Profile page gains a change-password dialog. Docs re-synced where they contradicted the code: `ARCHITECTURE.md` (routes, 4 migrations, component states), `QA_PLAN.md` (§1/§7, T-21…T-23), `QA_REPORT.md` (stale Stage-B block moved to a clearly-labelled historical section, 124 tests), `GAP_ANALYSIS.md` §12 tally, `API_CONTRACT.md` §1–§4 statuses, `UI_INVENTORY.md`, this plan. **Evidence: `go build`/`vet`/`gofmt` clean · 124 tests pass (was 115) · lint 0/0 (96 files) · build OK.**
- 2026-10-23: **LOGIN SYSTEM COMPLETE (live-verified).** Found and fixed two blocking defects while wiring the first-run path: (1) `POST /api/setup/bootstrap` created the Super Admin but **never issued a session** — setup "succeeded" then every call was 401 (**F-020**); (2) no code path could create an **`admin`** account at all, so the whole reseller half of the app was unreachable (**F-021**). Shipped: `auth.Service.CreateUserByRole/ListUsers/SetUserStatus/SetQuotaLimit/BootstrapRequired`, `GET /api/setup/status`, `GET/POST /api/admins` + `PATCH /api/admins/{id}` (super-admin only, suspend revokes live sessions, self-suspend → 422), `pages/SetupPage.tsx` (`/setup` wizard), bootstrap detection in `authStore.hydrate`, fake role switcher removed from Profile, `cmd/seeddemo` (guard `VEYLIX_SEED=1`, idempotent, passwords only in git-ignored `.veylix-secrets/seed.json`), `docs/DEV_LOGIN.md`. **Live evidence: three roles log in through the Vite proxy (HttpOnly cookie, `/api/me` 200 each); RBAC matrix `/api/admins` 200/403/403 · `/api/audit` 200/403/403 · `/api/servers` 200/200/403 · `/api/quotas/me` 200/200/403 · admin creating an account 403 · self-suspend 422 · anonymous 401.** Build/vet/gofmt clean · **131 tests pass** · lint 0/0 (97 files) · build OK.
- 2026-10-24: **D-024 WRITTEN — "a VPN *is* a 3X-UI client" is now an invariant.** Owner-mandated: the panel is the only authority; capacity/duration/device-limit/usage/enable are **read from the panel** (`totalGB`, `expiryTime`, `limitIp`, `traffic`, `enable`) and never derived from an offer; Free vs Premium is a **Super-Admin tag** stored locally and never written to the panel; an untagged panel client is not in the catalogue; creation is **panel write → confirm → record**, so a failed create leaves **no VPN row**. Written into `DECISIONS.md` (D-024 with 5 sub-invariants, 8 documented code violations with `file:line`, data-model consequences, non-goals, 8 implementation stages A–H), `REQUIREMENTS.md` (**new §2.0**, to which §2.1–§2.4 are now subordinate, plus §3.5 panel-authority clause), `MASTER_PRODUCT_BIBLE.md` (§0 instruction 17), `API_CONTRACT.md` (§4 rewritten + new §4.1 tag endpoints), `ARCHITECTURE.md` (schema invariants). **No code changed in this step** — it is the specification the next stages implement. **Next: Stage B** (migration `0006`: `panel_client_tags` + `panel_*` mirror columns) → **Stage C** (read-only panel inspector) → D/E/F/G/H.
- 2026-10-24: **Milestone 5-B step 1 COMPLETE — panel adapter gaps closed (F-024, F-026).** `ClientSpec` gained **`limitHwid`** (the owner's live-verified defect: hardware limits never reached the panel), plus `UUID`/`Password`/`Auth` for D-028.6; `ClientRow` gained `limitHwid`; new `PanelClient` read shape with `SpecFromPanelClient` so a read-modify-write cannot drop fields (clients/update **replaces**, not patches). New adapter methods: `GetClient`, `UpdateClient`, `Links`, `Hwids`, `DeleteHwid`. `buildClientRequest` now sends both `limitIp` and `limitHwid`. **Live proof:** `POST /api/vpns` → `201 ACTIVE confirmed:true`, and reading `vyx-11` back from the panel returned `limitIp:2, limitHwid:2, totalGB:10737418240, enable:true`. The live probe also caught **F-026** — `clients/get` nests the record under `obj.client`, and the first draft silently decoded a zero-valued client; fixed with a nested-shape decode plus a version-tolerant fallback. **Evidence: 234 tests pass (was 220) · build/vet/gofmt clean.** **Next: step 2 — migration `0006`.**
- 2026-10-24: **Milestone 5-B step 7 (first half) — verified absence + local removal (F-027).** The owner cleared the 3X-UI panel and reported that Veylix still showed the VPNs. Two real defects: `sync` silently `continue`d past a client missing from a page-through (and a test *asserted* that), and — the blocker — **`panel.ClientExists` could not detect absence at all**, because 3X-UI answers an unknown client with `HTTP 200 {"success":false,"msg":"Obtain (record not found)"}` rather than a 404, which the envelope decoder turned into an error. Shipped: `ClientExists` recognises the record-not-found envelope as positive absence (any other failure stays an error); `sync.applyReadings` does the verified-absence read (gone → `SERVER_UNAVAILABLE`, never `REVOKED`; page short but client exists → untouched; read failed → nothing changes) with new `Absent`/`Unverified` counters; `POST /api/vpns/{id}/verify` for an on-demand check; `DELETE /api/vpns/{id}` that erases **only the local record** (409 while the panel still holds the client, admin-only), audited as `vpn.absent` / `vpn.remove_absent`. **Live:** `verify 11` → `{"exists":false,"status":"SERVER_UNAVAILABLE"}`, `DELETE 11` → 204, list → `[]`, quota → 0, and the panel's own clients were never touched (integration stayed `ACTIVE`). **Evidence: 243 tests pass (was 234) · build/vet/gofmt clean.** **Next: step 2 — migration `0006`.**
- 2026-10-24: **D-024.1a fixed (F-028) — a refused create no longer leaves a VPN row.** Two defects found while verifying F-027 live: `inbound_id = 0` was never validated (offers 1/2 have no inbound, so `inboundIds:[0]` was sent and rejected *after* the row was written), and a definite rejection stored `REVOCATION_PENDING` — a state claiming a client existed and was deleted. Shipped: `loadOffer` rejects an inbound-less offer with `ErrOfferIncomplete` (**422 `offer_incomplete`**) before any write; a panel rejection now calls `discardProvisional`, which deletes the VPN row, its assignment and releases the quota slot in one transaction, returns **502 `panel_rejected`** with `recorded:false`, and keeps the outbox row for diagnosis. **Live:** `{offer_id:1}` → 422 with the VPN count unchanged; `{offer_id:3}` → `201 confirmed:true recorded:true`, `verify` → `{"exists":true,"status":"ACTIVE"}`; a live VPN refused `DELETE` with **409 `vpn_still_on_panel`**; `revoke` left `REVOCATION_PENDING` on a `TLS handshake timeout` and reached **REVOKED** on retry. **Evidence: 246 tests pass (was 243) · build/vet/gofmt clean.** **Next: step 2 — migration `0006`.**
- 2026-10-24: **Milestone 5-B step 2 COMPLETE — migration `0006_panel_truth.sql` (schema 5→6).** Four new tables — `panel_client_tags` (with `locked_at` so a tag cannot flip after a sale, D-025.2), `free_claims` (the ledger both FREE caps derive from; a released claim keeps counting, D-025.5), `vpn_releases` (3-day grace + `panel_client_kept`, D-027), `client_storage_limits` (a chat budget separate from the VPN quota, D-027.6) — plus six `panel_*` mirror columns on `vpn_instances` with a one-time backfill from the create-time values so existing rows are not shown as "0 GB / no expiry". Deliberately **no `chat_usage` table**: usage is summed from `chat_messages`, because a mutable counter can drift from the rows it describes. Store layer: `tags.go` (`SetClientTag`, `ClientTag`, `LockClientTag`, `UntaggedClients` — with `ErrTagLocked` enforced by a conditional upsert, so a stale write cannot overwrite an issued tag) and `claims.go` (`FreeUsage`, `RecordFreeClaim`, `ReleaseFreeClaim` — both caps checked inside the transaction, mirroring the resale-quota pattern). **Live:** `/api/ready` → `schema_version: 6`; all four tables and all six columns present; data intact (vpns 1, users 3, audit 156); backfill sample `vyx-15` → `total=10GB, expiry=…000ms, ip=2, hwid=2`. **Evidence: 257 tests pass (was 246; 11 new store tests) · build/vet/gofmt clean.** Also removed a brittle `schema_version != 5` assertion in the sync harness (now `>= 6`). **Next: step 3 — read-only panel inspector.**
- 2026-10-24: **Milestone 5-B step 3 COMPLETE — read-only panel inspector (D-024 Stage C).** `GET /api/servers/{id}/panel-clients` returns **every** client the panel holds — including ones created directly in 3X-UI that Veylix has no row for — with the **panel's own** `total_gb`/`expiry_time`/`limit_ip`/`limit_hwid`/`enable`/traffic, joined with the operator's `tag` (`null` when unclassified) and `assigned`/`veylix_status`. Read-only **by construction**: the `PanelInspector` interface exposes no write method, so the route cannot change the panel. Pagination is bounded (200 × 50, the same caps as sync) and local data is read in two queries, not one per client. `?tag=free|premium|untagged` filters; an unknown value is **422** rather than a silent empty list. Super-Admin only, and a nil inspector answers **503** instead of pretending the panel is empty. A `REVOKED` row does not read as `assigned` — history is not a held VPN. **Live:** the real panel returned its 3 clients (`DNuser`, `yasa`, `athoof`) with `tag: null, assigned: false`; `?tag=free` → `[]`; `?tag=nope` → 422; `/api/servers/999/...` → 404; the panel stayed `ACTIVE` (nothing was written). **Evidence: 261 tests pass (was 257) · build/vet/gofmt clean.** **Next: step 4 — tagging API/UI.**
- 2026-10-24: **Milestone 5-B step 4 COMPLETE (backend) — tagging API + the D-025.2 lock that was never wired.** `PUT /api/panel-clients/{server_id}/{email}/tag` classifies a panel client FREE or PREMIUM (D-024.3). It is a **local** write — the `PanelInspector` interface stays read-only by construction, and the route never reaches the panel, so a panel outage cannot block classification. Super-Admin only (`PermServerManage`), lowercase accepted from a UI and stored canonically, an unknown value is **422** rather than a silent success, and a missing `PanelTags` dep answers **503** instead of pretending. **The real defect fixed here:** `LockClientTag` and `ErrTagLocked` existed but *nothing called them*, so "tag locks once issued" was a rule with no enforcement — FREE↔PREMIUM could be flipped after a sale. The create transaction now locks the tag **inside the same transaction as the insert**, so there is no window where an issued VPN has an unlocked tag; the route maps the store's refusal to **409 `tag_locked`** and audits it as `panel_client.tag_denied` / `DENIED` (an attempt to re-price an issued VPN is exactly what a trail must show). A **rejected** create releases the lock again, otherwise one failed attempt would make that panel identity permanently unclassifiable. Audit: `panel_client.tag` records `previous` → `tag` so the transition is reconstructable. **Evidence: 268 tests pass (was 261; 7 new: 5 route + lock-on-issue + rollback-releases-lock) · build/vet/gofmt clean.** **Next: the tag button on the panel-clients screen (UI), then step 5 — VPN list from the panel.**
- 2026-10-24: **Milestone 5-B step 5 COMPLETE — the VPN list now comes from the panel, and the tag finally controls visibility (D-024.3 / D-025.1 / I-3).** `GET /api/vpns` no longer reads local rows for everyone: the **Super Admin** gets a panel-backed catalogue (`PanelVPNLister`) where a client created **directly in 3X-UI** appears even though Veylix has no row for it, while an **admin/client** gets a local view whose join on `panel_client_tags` is **required** — so an untagged VPN is never published to them. Each row now carries `tag` (null = untagged) and `source` (`panel` = the panel confirmed it, `local` = Veylix's record, unverified), because a list that cannot say which of the two it is invites a wrong conclusion. `?tag=free|premium|untagged` filters (unknown → **422**, never a silently unfiltered list); `?tag=untagged` **is** the Untagged queue. A VPN that exists both locally and on the panel is **one** row (panel tag + existence, local ownership/timestamps) — duplicating it would make one VPN look like two. **Outage policy:** a panel that cannot be read degrades the Super-Admin list to the local rows marked `source: local` instead of emptying it; but when **nothing** could be verified *and* Veylix has no rows, the list fails rather than confidently answering "no VPNs exist". **Tests: T-32 from `QA_PLAN.md` is green** (untagged panel client → visible to Super Admin, invisible to admin and client), plus publish-on-tag, 422 filter, outage degradation, unverifiable-empty, and the local+panel merge (id kept, tag from panel, disabled panel client reads `SUSPENDED`). One existing test changed meaning and was corrected rather than the code: `TestClientProvisionsFreeOffer` now pre-tags the panel identity, because a client may no longer see an untagged VPN. **Evidence: 274 tests pass (was 268; 6 new) · build/vet/gofmt clean.** **Next: step 12 (VPN detail UI) or step 8 (FREE claim route).**
- 2026-10-24: **Milestone 5-B step 8 COMPLETE — FREE claim caps enforced on the real create path (D-025.4/.5).** The store primitives existed but **nothing called `RecordFreeClaim`** from a create, so "3 concurrent / 5 per rolling 30 days" was a rule with no enforcement — the same class of defect as F-029. `provisioning.Create` now books the claim **inside the same transaction** as the VPN row, assignment and outbox entry, so both caps are checked and consumed atomically: two concurrent claims cannot both observe the last slot. A refusal returns a **distinct** 409 so the UI can name the cap — `free_concurrent_limit` vs `free_monthly_limit` — rather than a generic quota error. `GET /api/vpns/claims/me?server_id=` reports `concurrent / used_this_window / monthly_limit / concurrent_limit`, because REQUIREMENTS §2.0.1 requires the cap to be **shown before claiming** (releasing does not refund a slot); `server_id` is required (the caps are per server) and a client may not ask for another client's usage. **Subtlety handled:** a create the panel REFUSES must give the claim back — the client never got a VPN, so `discardProvisional` deletes the claim row it wrote, matched by `vpn_instance_id`, which does not weaken D-025.5 (that rule protects against farming a VPN the client actually held). The claim row is also linked to its VPN (`free_claims.vpn_instance_id`), which the column existed for but nothing set. **Not done, and stated as such:** the per-client adjustable cap (D-025.5) has no column yet — `store.MonthlyLimit` is the single seam and returns the policy default for every account. **Evidence: 278 tests pass (was 275; 3 new: cap blocks the 4th claim with nothing left behind, usage readable before and after a claim, refused create refunds the slot) · build/vet/gofmt clean.** **Next: step 11 (links/QR + ping) or step 12 (VPN detail UI).**











- 2026-10-24: **Panel-as-truth, evening slice COMPLETE (live + unit) — server edit, FREE 2/2, and the panel limit mirror.** Three gaps that the docs had been carrying since D-024/D-028 are now closed in code, and each one was proven against the running binary and the real 3.10.2 panel, not just in a unit test.
  - **`PATCH /api/servers/{id}` (A1).** The frontend's `serversStore.updateServer` had been calling a route that did not exist, so server editing failed silently. New handler + `integrations.Service.Update` accept any subset of `{name, panel_url, token, allow_private_network}`, keep every field the caller omitted, **re-verify the panel** and only then persist, and **re-seal the token** when the URL changes — the AES-GCM AAD is bound to the URL, so a ciphertext from the old host can never be replayed against a new one. The token is still never returned. **Live:** `PATCH {"name":"indian1"}` → `200` + `ACTIVE` re-tested against `indian1`; `PATCH /api/servers/999` → `404`.
  - **FREE is fixed 2/2 (A2, D-028.3).** `provisioning/policy.go` is now the single place the limit → panel-field mapping lives (`limitsFor` / `specFor`), and `payload.go` only adds the inbound. A FREE offer is issued `limitIp=2`/`limitHwid=2` **whatever its `device_limit` says**; PREMIUM keeps the operator's value (0 = unlimited, D-028.4), so a customer's paid cap is never invented. The create row now records the panel-enforced limits (`device_limit`, `panel_limit_ip`, `panel_limit_hwid`) so it cannot disagree with what the panel was told. **Live:** a FREE offer created with `device_limit:1` produced `limit_ip:2, limit_hwid:2` on the panel — the exact defect this rule exists to prevent (_a mis-typed offer silently handing out an unlimited free VPN_).
  - **Sync mirrors the panel (A3, D-024.2/.4).** `sync.mirrorLimits` writes the panel's `totalGB`/`expiryTime`/`limitIp`/`limitHwid`/`enable` into the `panel_*` columns **only when the panel's answer changed**, so `panel_synced_at` means "when the mirror last changed" rather than "when sync last ran" (that is `server_integrations.last_sync`). Rows written before the mirror existed are rewritten once (their values came from an offer, which is not evidence about the panel). When a client cannot be verified — the one case where sync knows nothing — the mirror is **cleared** instead of left stale, because a figure about a panel we could not read is a claim we cannot support. Create-time columns are never touched: they are history, and overwriting them would destroy the record of what the VPN was issued with. **Live:** `vpnprobe` output for `vyx-16`/`vyx-17` (10 GiB, expiry, 2/2, enable) matches `GET /api/servers/1/panel-clients` byte-for-byte.
  - **Two new operator binaries.** `cmd/vpnprobe` prints a VPN's issued limits beside its panel mirror (read-only, never migrates, never prints a secret) so the mirror can be checked by hand; `cmd/dbexec` runs one SQL statement, refuses to run without `VEYLIX_DBEXEC=1`, and prints only a row count — it exists because a smoke test must be able to clean up its own rows on a machine with no `sqlite3` client.
  - **Evidence:** `go build`/`go vet`/`gofmt` clean · **271 tests pass, 0 fail** (was **262** — measured by re-running with the nine new tests skipped — so **+9**: 5 in `provisioning/policy_test.go` — FREE 2/2 across `device_limit` 0/1/5/100/−1, PREMIUM keeps its choice, the cap survives JSON, identity/capacity untouched, expiry from duration — and 4 in `sync/mirror_test.go` — panel beats local, idempotent second tick, legacy row rewritten once, outage clears then restores the mirror) · `oxlint` 0 warnings / 0 errors (102 files) · live smoke on the shipped binary against `indian1`.
  - **Cleanup honesty:** the live FREE claims created during the smoke (`vyx-18`, `vyx-19`) plus the probe offer were removed from the local database and the panel clients were kept, exactly as D-027.3 specifies; the pre-existing rows (`vyx-15/16/17`) and the three original offers are unchanged.
  - **Still open, and stated as such:** the panel-clients **tagging button** and the panel-inspector / `/servers/:id` screens are still UI-less (the APIs are live); **D-028.1/.2** (panel email = owner's email with `-2`/`-3`, rename on assign) is not started; the HWID and credential-change routes have no HTTP surface; the per-client adjustable FREE cap still has no column. **Next: D-028.1/.2 + the HWID/credential routes, then the three missing screens.**


- 2026-10-24: **D-024 corrected + D-025…D-028 written after owner clarification.** Corrections: **every** panel client is a VPN (not "only tagged ones"); the tag is a **visibility** control, so an untagged VPN exists but is Super-Admin-only; create-time limits are supplied by the Super Admin and then the panel's copy is truth. New decisions: **D-025** (tag locks once issued; only FREE claimable / only PREMIUM resellable; **3 concurrent** FREE per client per server; **5 claims per rolling 30 days**, Super-Admin-settable, and a release does **not** refund a claim), **D-026** (Premium detail page: QR+link, obtained→expiry, **button-triggered live ping**, usage charts with 7d/30d/All range **and** user-selectable chart type, separate CPU/RAM/network charts; **no speed test**; Free gets QR+link+ping+7-day chart), **D-027** ("storage" means **quota**, not disk: release → **3-day grace with undo** → local removal only, panel client untouched; chat storage is a separate Super-Admin-set per-client budget), **D-028** (panel email = owner's Veylix email with `-2`/`-3` suffixes; **assigning renames the panel client**; FREE is fixed `limitIp=2`+`limitHwid=2`; PREMIUM limits chosen by Super Admin; **`limitHwid` was missing from `ClientSpec`** — the owner's live observation that hardware limits never applied). Synced into `REQUIREMENTS.md` (§2.0/.1/.2), `MASTER_PRODUCT_BIBLE.md` (§0.17–.19), `API_CONTRACT.md` (§4.1/§4.2), `ARCHITECTURE.md`, `SYSTEM_MAP.md` (retention gap now specified). **No code changed** — docs are the specification for Stage B onward: migration `0006` (`panel_client_tags`+`locked_at`, `free_claims`, `vpn_releases`, `client_storage_limits`, `chat_usage`, `panel_*` mirror columns) → adapter (`limitHwid`, `UpdateClient`, `ClientLinks`, hwids) → tag/claim/release APIs → detail-page UI.



