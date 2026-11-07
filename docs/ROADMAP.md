# VEYLIX VPN — ROADMAP / PROGRESS MAP

**Status legend:** 🟢 Done · 🔄 In progress · ⬜ Planned · 🧊 Blocked/Waiting · 🔴 Failed/Needs fix

> This is the high-level progress map. Detailed checklists live in `docs/IMPLEMENTATION_PLAN.md`.
> Any flow change requires owner approval (recorded in `docs/DECISIONS.md`).

---

## Milestone 0 — Foundation & Setup 🟢
- [x] Git repository initialized
- [x] `docs/REQUIREMENTS.md` created (scope + requirements)
- [x] `docs/ROADMAP.md` created (this file)
- [x] `docs/IMPLEMENTATION_PLAN.md` created (detailed plan)
- [x] `docs/DECISIONS.md` created (decision log)

## Milestone 1 — Frontend Project Setup 🟢
- [x] Vite + React + TypeScript scaffold
- [x] Project conventions (Oxlint, TS strict)
- [x] Folder structure (app shell, components, pages, hooks, stores)
- [x] Design tokens (CSS variables), fonts incl. Sinhala fallback stack
- [x] Base layout: sidebar, topbar, responsive shell + routing
- [x] Base UI components integrated on a live page (dashboard + /sample QA page)
- [ ] Live test (owner on localhost)

## Milestone 2 — Design System & Theme Studio 🟢
- [x] Reusable components: badges, cards, tables, buttons, inputs, dialogs, toasts, skeletons, empty/error/loading states
- [x] Icon system (lucide-react)
- [x] Theme Studio: 5 presets (Dark/Light/Normal/Gaming/Modern) + palette rebuild (premium SaaS direction)
- [x] Sub-theme customization (colors, typography, effects), preview/reset — **40 variants (8 per preset, Community Legends)**
- [x] Visual effects Off/Subtle/Enhanced + reduced motion
- [x] Chart interactivity (tooltips/crosshair/legend toggles) + View Transitions navigation — UI 2.1
- [x] Persistence (localStorage active)
- [ ] Live test

## Milestone 3 — Role-aware Pages (UI with mock/API-shaped data) 🟢
- [x] Auth screens (Google + email/password UI) — mock flow per D-006
- [x] Route guards (unauthenticated → /login)
- [x] Super Admin shell + navigation
- [x] Admin shell + navigation
- [x] Client shell + navigation
- [x] Dashboards (each role)
- [x] Server integrations page (Super Admin) — add/edit/test-connection/health States (mock, D-006)
- [x] VPN offers page (Super Admin) — Free offer rules + claims (mock)
- [x] Admin management page (Super Admin) — create/suspend/quota/servers (mock)
- [x] Clients/assignments pages — admin clients + sale flow (quota-checked)
- [x] Premium sale + payment flow — 3-step sale + confirm (audited)
- [x] Client VPN detail page — status/usage/limits/QR demo/link/guide/seller
- [x] Purchase/history page — records + pending orders
- [x] Internal chat UI — ChatPanel + chatStore (role-filtered)
- [x] Notifications center page — full inbox (popover + page)
- [x] Profile/settings — role switcher + theme prefs link
- ⬜ Known gaps (non-blocking): SA *clients* screen, *incidents* screen, Admin *own contacts/payment details* — `GAP_ANALYSIS.md` §6
- [ ] Live test

## Milestone 4 — Backend Foundation (Stage B–E of `new_update.md` §11) 🔄
- [x] Stage A — baseline, inventory, 3X-UI contract verified 🟢 (2026-10-23)
- [x] Stage B — Go module + `ARCHITECTURE.md` + FE↔BE `API_CONTRACT.md` 🟢 · build/vet/gofmt clean · 13/13 tests pass
- [x] SDLC + QA framework — `SDLC_PLAN.md` (D-014), `QA_PLAN.md` (D-015), `QA_REPORT.md` 🟢
- [ ] Stage C — file-by-file audit of stores/pages + backend design review
- [ ] Stage D — prioritised plan signed off
- [x] Stage E1 (partial) — config/secrets + SQLite schema + migrations + tx helper 🟢
- [ ] Stage E1 (rest) — backup path + restore rehearsal
- [x] Stage E2 — auth (argon2id, sessions, RBAC matrix, ownership, rate limits, audit) 🟢 **live-verified** · Google OAuth pending (Q-3)
- [x] Stage E3 API wiring — `/api/servers/*` with real adapter, sealed tokens 🟢 **live-verified**
- [x] **Stage E7 — SPA embedded in the Go binary** 🟢 **DONE 2026-10-24 (D-029)** — `//go:embed all:dist` + client-route fallback; `/dashboard` and `/vpn/16` answer 200 from ONE port while `/api/*` keeps its JSON 404. Built and asserted by `.\dev.ps1 -Package` (7 tests in `spa_test.go`)
- [x] Live test (API against frontend) — the packaged binary served `/api/vpns` **and** `/dashboard` on the same port, verified 2026-10-24
- ⛔ `go test -race` — blocked on this machine (no C compiler); run in CI


## Milestone 5 — 3X-UI Integration (Stage E3–E6) 🟢
- [x] Client adapter (Bearer token, reused transport, timeouts, bounded retry on GETs only) 🟢
- [x] SSRF guard + capability detection (`/server/status`, `inbounds/options`) 🟢
- [x] Server health + status mapping (never outage → REVOKED) 🟢 **live-verified**
- [x] Client sync via `/clients/list/paged` (read path) 🟢 **live-verified**
- [x] Provisioning service: create / suspend / resume / revoke with quota + outbox 🟢 **unit-verified**
- [x] Outbox + ambiguous-write reconciliation (no duplicates) 🟢 **unit-verified**
- [x] Revocation flow with verified write / `REVOCATION_PENDING` 🟢 **unit-verified**
- [x] **Usage snapshots + lifetime history (E5)** 🟢 **live-verified 2026-10-24** — `sync` writes `usage_snapshots`, `/api/vpns/{id}/usage` reports `history: NONE` instead of inventing a series, and `GET /api/servers/{id}/metrics` returns real CPU/RAM
- [x] **QR/link from `GET /clients/links/{email}` (E6)** 🟢 **live-verified 2026-10-24** — `GET /api/vpns/{id}/links` returns the panel's own URIs untouched (`source: panel|unavailable`); a genuine `vless://…` came back for VPN #16
- [ ] Live provisioning **writes** — still blocked: single production inbound on the real panel (D-018)



### 3X-UI capability coverage (`new_update.md` §5)
| Capability | Status |
|---|---|
| Connection config + authenticated health check | 🟢 **live-verified** |
| Inbound listing / inspection | 🟢 **live-verified** |
| Inbound create / update / enable / disable / delete | ⬜ not implemented (**D-024: deliberately left to the 3X-UI panel — too destructive via a whole-config write**) |
| Client retrieve (list / paged / fresh read) | 🟢 **live-verified** |
| Client create / enable / disable / delete | 🟢 adapter + service **unit-verified** · live ❌ |
| **Client update (`clients/update/{email}`) — capacity/expiry/rename** | 🟢 adapter **done** (`UpdateClient` + `SpecFromPanelClient`); no HTTP route yet |
| Protocol-specific client configuration | 🟢 `UUID`/`Password`/`Auth` now sent when set; the panel still generates them when omitted |
| Expiry / traffic quota / IP limits | 🟢 mapped in provisioning (`expiryTime`, `totalGB`, `limitIp`) |
| **Hardware limit (`limitHwid`)** | 🟢 **FIXED 2026-10-24** — sent on create, live-verified on the panel (F-024 / D-028.5) |
| Traffic statistics + online status | 🟢 **live-verified** — persisted by the sync job (`usage_snapshots`, D-023) and served by `/api/vpns/{id}/usage` |
| Subscription + connection-link delivery | 🟢 **live-verified 2026-10-24** — `GET /api/vpns/{id}/links` + the VPN detail UI's copy/open actions (D-026) |
| Runtime / server metrics | 🟢 **live-verified** (`cpu`, `mem`, `xray`) via `GET /api/servers/{id}/metrics` |
| Multi-panel synchronisation | 🟢 **live-verified** — per-server jobs, bounded per-server timeout, `GET /api/sync/status` reports staleness |
| **Outbound create/edit** | ⚠️ **only via `POST /xray/update` (whole-config overwrite)** — out of scope by owner decision |
| **Client HWID device list / revoke** | 🟢 adapter **done** (`Hwids`, `DeleteHwid`); **no HTTP route yet** (Milestone 5-B step 15) |





## Milestone 5-B — Panel-as-truth rebuild (D-024 … D-028) 🟢 steps 1–12 shipped

> The owner's invariant: **a VPN *is* a 3X-UI client**. Steps 1–12 are implemented and tested as of 2026-10-24; steps 10, 13, 14 and 15 remain. (This header previously read "🔴 not started" while the list below it was almost entirely `[x]` — the contradiction was corrected on 2026-10-24.)

- [x] **1. Adapter gaps** — `ClientSpec.LimitHwid`, `password`/`auth`/`uuid`, `GetClient`, `UpdateClient` (full-record replace), `ClientLinks`, hwids list/revoke ✅ **done 2026-10-24 (234 tests, live-verified)**
- [x] **2. Migration `0006`** — `panel_client_tags(+locked_at)`, `free_claims`, `vpn_releases`, `client_storage_limits`, `chat_usage`, `panel_*` mirror columns on `vpn_instances` ✅ **done 2026-10-24** — schema 5→6, live DB migrated with data intact (no `chat_usage`: usage is summed from `chat_messages` instead of a counter that could drift)
- [x] **3. Panel inspector** (read-only) — every panel client with the panel's own limits + tag, filterable ✅ **done 2026-10-24** — `GET /api/servers/{id}/panel-clients`, live-verified (3 panel clients, `tag: null`, `assigned: false`)
- [x] **4. Tagging** — tag/re-tag while unissued; refusal once issued (D-025.2) ✅ **done 2026-10-24 (backend)** — `PUT /api/panel-clients/{server_id}/{email}/tag` (Super-Admin only, `409 tag_locked` once issued, audited `panel_client.tag` / `panel_client.tag_denied`) + the create transaction now locks the tag and releases it if the panel refuses the client (268 tests, build/vet/gofmt clean). **UI button on the panel-clients screen still pending.**
- [x] **5. VPN list from the panel** — tagged clients are the catalogue; untagged are Super-Admin-only (D-024.3) ✅ **done 2026-10-24 (backend)** — `GET /api/vpns` is now panel-backed **for the Super Admin** (every panel client, including ones created directly in 3X-UI, merged with local ownership into one row, `source: panel|local`); admins/clients read the local view with a **required** tag join, so an untagged VPN is never published to them. `?tag=free|premium|untagged` filters (unknown → 422); a panel outage **degrades** the Super-Admin list to `source: local` rows instead of emptying it, and fails loudly when nothing could be verified and Veylix has no rows. **T-32 green · 275 tests**; **live smoke on the shipped binary** (`/api/vpns` → 200 `[]`, `?tag=untagged` → 200 `[]`, `?tag=nope` → 422) — panel path still unit-only, recorded in `QA_REPORT.md` §0.5. **Next: FREE claim route (step 8).**
- [x] **6. Create** — panel-first → confirm → record; `ErrOfferIncomplete` when `inbound_id = 0`; FREE fixed 2/2 ✅ **done 2026-10-24** (F-028) — **D-028.3 closed 2026-10-24 (evening)**: `provisioning/policy.go` (`limitsFor`/`specFor`) fixes every FREE offer at `limitIp=2`/`limitHwid=2` whatever its `device_limit`, PREMIUM keeps the operator's choice, and the create row records the panel-enforced limits. **Live-verified**: FREE offer with `device_limit:1` → panel returned `limit_ip:2, limit_hwid:2`.
- [x] **7. Sync** — mirror panel limits, verify absence, never revoke on outage ✅ **done 2026-10-24 (evening)** — `sync.mirrorLimits` writes `totalGB`/`expiryTime`/`limitIp`/`limitHwid`/`enable` into the `panel_*` columns every tick, only when the panel's answer changed (`panel_synced_at` = last change), rewrites pre-mirror rows once, and **clears** the mirror when a client cannot be verified (a stale figure is a claim about a panel we could not read). Create-time columns are never touched: they are history. **Live-verified**: `vpnprobe` and `GET /api/servers/1/panel-clients` agree byte-for-byte for `vyx-16`/`vyx-17`. 4 tests. (D-024.2/.4/.5)
- [x] **8. FREE claim caps** — 3 concurrent, 5/rolling-30-days, no refund on release 🟡 **store layer done 2026-10-24** → ✅ **route done 2026-10-24** — `RecordFreeClaim` now runs **inside the create transaction**, so both caps are checked and consumed atomically (a refused claim rolls the VPN back); `409 free_concurrent_limit` / `409 free_monthly_limit` name which cap was hit; `GET /api/vpns/claims/me?server_id=` shows usage + limits **before** claiming (REQUIREMENTS §2.0.1); a claim taken by a panel-**refused** create is given back. Per-client adjustable cap still pending (no column yet — `store.MonthlyLimit` is the single seam). (D-025.4/.5)
- [x] **9. Release & restore** — ✅ **done 2026-10-24** — `POST /api/vpns/{id}/release`
      (owner-only, 3-day window), `POST /api/vpn-releases/{id}/restore` (undo),
      `GET /api/vpn-releases`. An hourly job removes the **local row + assignment only**;
      the audit entry states the panel client was kept (D-027.3/.5). Releasing does not
      refund a FREE claim (D-027.4).
- [ ] **10. Chat storage budgets** — per-client limit + self-service reclaim (D-027.6)
- [x] **11. Links + ping** — ✅ **done 2026-10-24** — `GET /api/vpns/{id}/links`
      (panel's own URIs, `source: panel|unavailable`) and `POST /api/vpns/{id}/ping`
      (`scope: server_reachability`, `-1` when unreachable). **Live-verified against the
      real panel**: a genuine `vless://…` URI came back for VPN #16.
- [x] **12. VPN detail UI (connection part)** — ✅ **done 2026-10-24** — the page renders
      copy/open actions from the real link, distinguishes outage from "no link", and
      labels ping as server reachability. Chart-type/range toggles still pending.
- [ ] **13. Dashboards** — Super Admin totals, reseller deal summary, client summary
- [ ] **14. Panel email identity** (D-028.1/.2) — panel email = the owner's Veylix email with `-2`/`-3` suffixes instead of `vyx-<n>`; assigning a VPN renames the panel client. **Not started** — `domain.PanelEmail(vpnID)` is still the identity, and no rename path exists.
- [ ] **15. HWID + credential routes** — `Hwids`/`DeleteHwid`/`UpdateClient` exist in the adapter but have no HTTP route, so a device list cannot be viewed or freed and a client cannot change their own `uuid`/password from the UI (D-028.6).

---

## Milestone 6 — Wire Frontend ↔ Backend 🟢 (D-006 exit complete)
- [x] **P6 Billing API** — `payment_methods` + `payment_records` CRUD with a guarded
      state machine (`PENDING → AWAITING_CONFIRMATION → CONFIRMED|REJECTED`, plus
      `CANCELLED`/`REFUNDED`), every transition audited. Integer cents only.
      **9 routes, live-smoked** (`frontend/scripts/smoke-billing.ps1`).
- [x] **P8 Notifications API** — per-account inbox, unread count, mark-read,
      mark-all-read. Scoped to the caller's `user_id`; a foreign id is a 404.
- [x] **Directory API** — `GET /api/directory`, role-scoped accounts for the chat
      picker (replaces the hard-coded eight-person list).
- [x] `notifStore`, `peopleStore`, `paymentsStore` — **mocks removed**, now real API.
- [x] `HistoryPage`, `BuyPremiumPage`, `PaymentsPage`, `NotificationsPage` re-wired.
- [x] Chat, payments, notifications, audit via API (audit was already real)
- [x] **P7 Chat API** — server-owned transcript (migration `0007`), participant-enforced
      access, edit-with-history, per-viewer hide/save/important, staff pin/lock, read
      markers. **12 routes, live-smoked** (`frontend/scripts/smoke-chat.ps1`).
- [x] `chatStore` — **mock removed**; this was the last one. **No mock stores remain.**
- [x] Hardcoded sidebar nav counts (`App.tsx` — **F-019 fixed 2026-10-24**) — every badge
      is now derived from the same list its page renders (`servers`, `offers`, `admins`,
      `clients`, chat `unread`); a badge is omitted when the list is empty.
- [x] **Notification producers** (2026-10-24) — the audit logger fans every privileged
      action into the affected account's inbox (`audit.Logger.WithNotifier`), so the
      inbox is no longer structurally empty.
- [x] **Quota ledger + concurrency-safe flows** — quota is counted inside the provisioning transaction (two racers → exactly one success, `TestQuotaConcurrency`); the FREE caps (3 concurrent / 5 per 30 days) use the same pattern (D-025.4/.5)
- [ ] Full end-to-end role tests — 39 acceptance tests remain (Milestone 8)
- [x] **UI truth gate** (2026-10-24) — `npm run audit` fails on any screen claiming a capability the backend contradicts; 12 such claims were removed (F-032, D-029 era)
- [ ] Live test (owner walkthrough per role)

## Milestone 7 — Production Packaging
- [x] **Single binary (E7 + D-029)** — `go build ./cmd/veylixd` → one artifact serving UI **and** API on one port; `.\dev.ps1 -Package` builds it and asserts the result
- [ ] Caddy + systemd templates
- [ ] Installer (install/upgrade/uninstall/backup/restore/domain-set) scripts
- [ ] CI workflow (lint/test/build)
- [ ] Release workflow (linux/amd64 + arm64) + checksums
- [ ] Env/template + secret handling
- [ ] Live test on clean Linux VM

## Milestone 8 — QA & Delivery
- [ ] Acceptance tests 1–39
- [ ] 500 MB benchmark report
- [ ] docs: INSTALL, UPGRADE, DOMAIN_CHANGE, BACKUP_RESTORE, TROUBLESHOOTING, QA_REPORT
- [ ] Final delivery checklist (Bible §19)

---

## Change log (roadmap)
- 2026-10-21: Roadmap created; Milestone 0 complete. Owner decisions applied (greenfield, frontend-first, live-test workflow, approval-gated scope changes).
- 2026-10-22: M1/M2 synced with reality — Theme Studio complete (palette rebuild + **40 sub-themes**, Community Legends), UI 2.0 (⌘K palette, a11y focus management, table sorting, rail sidebar, alive dashboard). M3 auth screens started.
- 2026-10-22: UI 2.1 — chart tooltips/crosshair/legend toggles, View Transitions navigation, notifications popover. `docs/UI_INVENTORY.md` living catalog created.
- 2026-10-22: **Batch A — Super Admin core pages** shipped (Server integrations / VPN offers / Admin management, mock data per D-006, Bible §3.1–3.3 status model in UI).
- 2026-10-22: **M3 COMPLETE** — Batches B–E: Payments+audit, Client dashboard+VPN detail (QR/guide), Admin role pages (sale flow w/ quota gate, retail price), ChatPanel. Role-filtered NAV. 0 placeholders left.
- 2026-10-23: **Stage A 🟢** — audit, baseline, and 3X-UI contract verification (release **v3.10.2**, official OpenAPI, 60+ endpoints) recorded in `docs/GAP_ANALYSIS.md`, `docs/IMPROVEMENT_LEDGER.md`, `docs/3XUI_API_CONTRACT.md`. M3 marked 🟢 with 3 known page gaps; M4/M5 restructured to Stages A–H (**D-010** supersedes D-002's frontend-first order). M5 gains a 3X-UI capability coverage table.
- 2026-10-23: **Stage E2 + E3 API wiring 🟢 LIVE-VERIFIED.** Auth stack (argon2id, sessions, RBAC, audit) + integration API wired to the real adapter. Against the live 3.10.2 panel: add → `ACTIVE`, inbounds listed, re-test OK, audit trail written, client 403 / anonymous 401, token sealed in the DB. **89 tests pass.** Defects fixed: F-011, F-013, F-014. `-race` blocked (no C compiler).
- 2026-10-23: **Stage B 🟢** — backend Go module (`cmd/veylixd` + `internal/{config,httpapi,domain,store,panel}`), SQLite with 3 embedded migrations, health/ready endpoints, graceful shutdown. Docs: `ARCHITECTURE.md`, `API_CONTRACT.md`, `SDLC_PLAN.md`, `QA_PLAN.md`, `QA_REPORT.md`. Evidence: build ✓ · vet ✓ · gofmt clean · 13/13 tests pass · runtime smoke `/api/health` 200 + `/api/ready` 200. `-race` blocked (no C compiler).
- 2026-10-23: **Stage E3 (adapter) 🟢 + LIVE panel verification.** Real 3X-UI **3.10.2** panel verified over HTTPS with a Bearer token. The panel serves its API under the **web base path** and returns `404`/empty for a wrong path — documented in `docs/3XUI_API_CONTRACT.md` §0. New `internal/panel` package: `ParseTarget` SSRF guard (17-case rejection table), Bearer client with reused transport + GET-only retries + no redirect following, `Health/Inbounds/ListClients/ClientExists/AddClient/DeleteClient/SetClientEnabled`. **Defect F-011 found & fixed** (path traversal via `..` in a client email). Tests: **34 total, all pass**. **Live: 3/3 PASS** (health, inbounds, client list + fresh read). Writes remain **not live-verified**.

