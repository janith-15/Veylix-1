# VEYLIX VPN — GAP ANALYSIS

**Type:** Living document — the evidence-based requirement→reality map (Bible §0.2, `new_update.md` §1).
**Version:** 1.0 · **Baseline date:** 2026-10-23
**Method:** every status below was produced by reading the cited files and running the cited commands. Nothing is marked "works" because a page or a function exists (`new_update.md` §1). Nothing is marked "tested" without an executed command.
**Companion docs:** `docs/SYSTEM_MAP.md` (module view), `docs/PROCESS_MAP.md` (process × actor view), `docs/IMPROVEMENT_LEDGER.md` (finding IDs), `docs/IMPLEMENTATION_PLAN.md` (stages), `docs/DECISIONS.md` (D-001…D-028).

---

## 0-A. D-024 … D-028 conformance board (2026-10-24)

> **Read this first.** Sections 1–12 below were written **before the backend existed** and are kept as the historical baseline. This board is where the live truth is: it maps each of the five owner-mandated invariants to its current code state.

| Decision | Invariant | Code today | Status |
|---|---|---|---|
| **D-024.1** | VPN ≡ 3X-UI client | `vpn_instances` is still an **intent** row written *before* the panel call (`provisioning/service.go:211-214`) | 🔴 **violated** |
| **D-024.1a** | Panel write → confirm → then record | Row inserted first, panel written second (`service.go:211` then `:279`) | ✅ **FIXED 2026-10-24** — offer validated before any write; a panel rejection rolls the provisional rows back (no VPN, no assignment, quota released). Was `🔴 violated` | 🟢 **holds** |
| **D-024.2** | Capacity/duration/IP/HW limits come from the panel | `payload.go:21-27` builds them **from the offer**; `panel_limit_*` columns do not exist | 🟢 **FIXED 2026-10-24** — sync now mirrors `totalGB`/`expiryTime`/`limitIp`/`limitHwid`/`enable` into the `panel_*` columns every tick (`sync.mirrorLimits`, 4 tests) and clears them when a client cannot be verified; the create row records the panel-enforced limits. Was `🟡 partial` — sync never populated them |
| **D-024.3** | Every panel client is a VPN; tag = visibility only | No `panel_client_tags` table; `GET /api/vpns` reads only local rows | 🟢 **FIXED 2026-10-24** — `GET /api/vpns` is panel-backed for the Super Admin (untagged panel clients included, merged with local rows, `source` on the wire); admins/clients get a tag-required local view. Was `🟡 partial` |
| **D-024.4** | Panel wins on every conflict (sync mirrors) | Sync reads usage but never overwrites limits from the panel | 🟢 **FIXED 2026-10-24** — a limit changed directly in 3X-UI is copied onto the row on the next tick (`TestSyncMirrorsPanelLimits`); `panel_synced_at` records when the mirror last *changed*, and the create-time columns stay as history. Was `🔴 missing` |
| **D-024.5** | Absence verified, never assumed | ✅ **FIXED 2026-10-24** — sync + `POST /api/vpns/{id}/verify` do a direct `GET /clients/get/{email}` and mark `SERVER_UNAVAILABLE` (never `REVOKED`). Was `🟡 partial` | 🟢 **holds** |
| **D-024.1** | VPN ≡ 3X-UI client | `DELETE /api/vpns/{id}` now erases the record of a VPN the panel no longer holds, after verifying absence | 🟢 **holds (removal path)** |
| **D-025.1** | Untagged = Super Admin only | No tagging at all | 🟢 **FIXED 2026-10-24** — tagging route (Super-Admin only, audited) **and** the visibility filter: an untagged VPN is visible only to the Super Admin (`?tag=untagged` is that queue) and is never published to an admin or client. Was `🟡 partial` |
| **D-025.2** | Tag locks once issued | — | 🟢 **FIXED 2026-10-24** — the create transaction locks the tag (same tx as the insert) and a rejected create **releases** it again; the route answers **409 `tag_locked`** and audits the refusal as DENIED. Was `🟡 partial` (nothing called `LockClientTag`) |
| **D-025.3** | FREE claimable / PREMIUM resellable only | Client may claim only FREE (`provisioning.authorise`); reseller path exists | 🟢 **holds** |
| **D-025.4** | 3 concurrent FREE per client per server | Not enforced anywhere | 🟢 **FIXED 2026-10-24** — enforced inside the create transaction; `409 free_concurrent_limit`. Was `🟡 partial` (store only) |
| **D-025.5** | 5 FREE claims / rolling 30 days | Not enforced; no `free_claims` table | 🟢 **FIXED 2026-10-24** — same transaction, `409 free_monthly_limit`; release still does not refund, and a **refused create** returns its slot; usage readable before claiming via `GET /api/vpns/claims/me`. Per-client adjustable cap still pending (no column) |
| **D-025.6** | Premium unlimited (reseller quota only) | Quota enforced transactionally (`service.go:184-201`) | 🟢 **holds** |
| **D-026** | Rich VPN detail (QR, ping, usage ranges + chart types, server charts) | 🟢 **FIXED 2026-10-24** — `GET /api/vpns/{id}/links` returns the panel's own URIs (`source: panel｜unavailable`) and `POST /api/vpns/{id}/ping` reports `scope: server_reachability`; `VPNDetailPage` renders Copy link / Open in app and an honest outage-vs-missing state. Was `🔴 missing` | 🟢 **holds (links + ping)**; chart-type/range toggles still open |
| **D-026.1** | Ping on demand only, **no speed test** | 🟢 **FIXED 2026-10-24** — the route is button-triggered, there is no background loop, and no speed-test code exists anywhere in the tree. Was `🔴 missing` | 🟢 **holds** |
| **D-027** | Release → 3-day grace → local removal only | `DELETE /api/vpns/{id}` removes **only the local record**, never the panel client. The 3-day grace + client self-service release is still to come | 🟡 **partial (removal done)** |
| **D-027.6** | Chat storage budget per client | No `client_storage_limits` / `chat_usage` | 🟡 **partial** — `client_storage_limits` table exists; usage accounting and the client-facing reclaim flow are not built |
| **D-028.1/.2** | Panel email = owner's email; assignment renames the client | `domain.PanelEmail(vpnID)` → `vyx-<n>`; no rename on assign | 🔴 **violated** |
| **D-028.3** | FREE fixed `limitIp=2`, `limitHwid=2` | Neither is fixed; FREE/PREMIUM limits come from the offer | 🟢 **FIXED 2026-10-24** — `provisioning.limitsFor` returns 2/2 for every FREE offer regardless of its `device_limit` (PREMIUM keeps the offer's choice, D-028.4). Live-verified: a FREE offer with `device_limit:1` produced `limit_ip:2, limit_hwid:2` on the panel. Was `🔴 missing` |
| **D-028.5** | `limitHwid` sent in `clients/add` | ✅ **FIXED 2026-10-24** — added to `ClientSpec` + `ClientRow`; live-verified on the panel (`limitHwid: 2`) | 🟢 **holds** |
| **D-028.6** | Owner can change their VPN credential | `ClientSpec.UUID`/`Password` sent; `UpdateClient` available | ✅ **FIXED 2026-11-05** — `POST /api/vpns/{id}/credential` rotates the secret the protocol actually uses (`uuid` for vless/vmess, `password` otherwise), generates a crypto/rand v4 UUID or a 20+ char password when `value` is empty, **re-reads the panel to confirm the write**, refuses a malformed UUID / <8-char / unchanged password with `422 invalid_credential`, and audits the FIELD but never the value. UI: "Change uuid/password" under **Protocol** with a mandatory confirm step. 7 new tests, incl. limits preserved (the replace-semantics trap) and foreign-VPN → 404. Was `🟡 partial` |

**What already holds today:** panel add/verify/test/inbound-list, client create (now **including `limitHwid`**), client read (full record), client update, connection links, HWID list/revoke, client delete, suspend/resume, paged client read, fresh-read confirmation, sync + usage history, RBAC, audit, sessions. The **adapter surface is complete for D-024…D-028**; what remains is the local data model, the HTTP routes and the UI (Step 2 onward in `IMPLEMENTATION_PLAN.md`).

---

## 0. Verification baseline (what was actually run on 2026-10-23)

| Check | Command | Result |
|---|---|---|
| Go toolchain | `go version` | `go1.27.1 windows/amd64` |
| Node / npm | `node --version`, `npm --version` | `v24.19.0` / `11.17.0` |
| Git | `git --version` | `2.56.0.windows.2` |
| Docker (needed for a local test panel) | `docker --version` | **not installed** → Stage G live test cannot run on this machine |
| Frontend lint | `npm run lint` (in `frontend/`) | **0 errors / 0 warnings** — 94 files, 116 rules |
| Frontend build | `npm run build` | **passes** — `index.html` 0.55 kB · CSS 136.23 kB (gz 23.01) · main JS 477.04 kB (gz 141.18) · `threeScene` 537.96 kB (gz 134.20, lazy) |
| Frontend tests | — | **NO runner installed** (no `test` script in `frontend/package.json`) |
| Backend build | `go build ./...` (in `backend/`) | **passes, exit 0** — verified 2026-11-05 |
| Backend tests | `go test ./... -count=1` | **12/12 packages ok, exit 0** — 539 test functions in 13 packages, verified 2026-11-05 |

> **Superseded.** The rows above originally read "NO backend exists" / "NO tests
> exist" / "Test baseline = zero". That was true on 2026-10-23 and is **false
> now**: `backend/` exists with `go.mod`, 22 migrations and 126 HTTP routes, and
> every package's tests pass. The stale rows are kept visible only so the change
> is auditable — do not act on them.

### Working tree preservation (`new_update.md` §1.3)

`git status --porcelain` before any doc edit recorded **40 modified tracked files + 6 untracked paths** (`docs/DESIGN_SPACING.md`, `docs/UI_INVENTORY.md`, `docs/new_reqierment/`, `.vscode/`, `diag5.txt`, `diagnose*.json`, `frontend/diagnose-layout.png`). These are pre-existing owner work and were **not modified or reverted** by this documentation pass. Only files under `docs/` were written.

---

## 1. Repository inventory

| Area | Present? | Evidence |
|---|---|---|
| Frontend app | **Yes** | `frontend/src/` — 61 TS/TSX + 18 CSS files |
| Frontend entry point | **Yes** | `frontend/src/main.tsx` → `App.tsx` |
| Routing | **Yes** | `frontend/src/App.tsx:120-172` — 22 route declarations |
| Route guard | **Yes (client-side only)** | `frontend/src/components/auth/RequireAuth.tsx:18-28` |
| Design system | **Yes** | `frontend/src/components/ui/` — 18 primitives |
| Theme Studio | **Yes** | `frontend/src/components/theme-studio/`, `frontend/src/theme/` |
| State stores | **Yes — all mock** | `frontend/src/stores/` — 10 zustand stores |
| API client | **No** | `grep "fetch(\|axios\|/api/"` matches only `frontend/scripts/*.mjs` (Chrome DevTools harness) and docs. **Zero production network calls.** |
| Backend / Go module | **No** | no `backend/` directory, no `go.mod` |
| HTTP handlers / services / repositories | **No** | — |
| Database schema / migrations | **No** | no `.sql`, no migration folder |
| 3X-UI adapter | **No** | `grep "3X-UI\|3x-ui"` → docs only (REQUIREMENTS, PLAN, BIBLE, requirement file). Zero source hits. |
| Background jobs / sync scheduler | **No** | — |
| Audit log service | **No** | `frontend/src/stores/auditStore.ts` is UI mock data |
| CI / release workflow | **No** | no `.github/` |
| Deployment scripts / installer | **No** | — |
| Docs | **Yes** | this folder |

---

## 2. Business rules — REQUIREMENTS §2

| Req | Requirement | Status | Evidence |
|---|---|---|---|
| 2.1 | Super Admin: add/verify/disable/remove 3X-UI integrations | **Partial** | UI exists `frontend/src/pages/ServersPage.tsx`; verification is simulated `stores/serversStore.ts:68-88`; nothing persists; credentials discarded (`ServersPage.tsx:179-186`) |
| 2.1 | Super Admin dashboard (servers, inbounds, clients, traffic, metrics) | **Partial** | `pages/DashboardPage.tsx` renders static demo series; no data source |
| 2.1 | Create/edit/suspend/remove Admin accounts | **Partial** | `pages/AdminsPage.tsx` + `stores/adminsStore.ts` (in-memory) |
| 2.1 | Free offer rules (server/inbound/traffic/expiry/devices) | **Partial** | `pages/OffersPage.tsx` + `stores/offersStore.ts` (in-memory) |
| 2.1 | Wholesale + direct-sale pricing | **Partial** | `pages/RetailPricePage.tsx`; no price model/schema |
| 2.2 | Quota = active Premium assignments, release on expiry/revocation | **Missing (logic)** | UI gate only in `pages/SaleFlowPage.tsx`; no ledger, no transaction, no concurrency safety |
| 2.3 | Client registration (Google + email/password, verified) | **Partial** | `pages/LoginPage.tsx`, `pages/RegisterPage.tsx`; `stores/authStore.ts` mock — no hashing, no token verification |
| 2.3 | Free offer → **distinct per-Client** credential | **Missing** | no provisioning path exists |
| 2.3 | VPN detail page (status/usage/limits/QR/link/guide/seller) | **Partial** | `pages/VPNDetailPage.tsx` — QR/link are demo strings, not derived from panel data |
| 2.3 | Purchase history survives server removal | **Partial** | `pages/HistoryPage.tsx` mock only |
| 2.4 | Free vs Premium distinction, removal from catalog | **Partial** | offer `active` flag in mock store |
| 2.5 | Reseller + Direct sale, manual payment, auditable confirm | **Partial** | `pages/PaymentsPage.tsx`, `SaleFlowPage.tsx`; audit entries are local mock |
| 2.6 | Seller stored **per Premium assignment**; chat participant access enforced server-side | **Missing (server-side)** | `components/chat/chat-permissions.ts` is a **client-side** check only |
| 2.7 | Per-VPN statistics, server-wide vs client traffic separation | **Missing** | no data source; `DashboardPage` series are constants |
| 2.8 | Real QR/link per client/protocol; never fake links | **Missing (real data)** | `VPNDetailPage.tsx` shows a demo link; the requirement explicitly forbids a working-looking fake |

---

## 3. Server integration & lifecycle — REQUIREMENTS §3

| Rule | Status | Evidence |
|---|---|---|
| Save panel URL + credentials, verify connectivity/auth before ACTIVE | **Missing (real)** | `serversStore.testConnection()` resolves after a hard-coded 1200 ms timer regardless of network state |
| Credential-change detection → REAUTH_REQUIRED / CONNECTION_UNAVAILABLE | **Missing** | only a seeded demo row carries that status |
| Never claim remote revocation without a verified API response | **Compliant by absence** — no revocation code exists to violate it, and none of the target flows work |
| Remove/offline: Premium never hard-deleted | **Partial (UI copy)** | warning dialog `ServersPage.tsx:196-225`; the store simply filters the row out |
| Sync: per-server jobs, timeouts, pagination, stale states | **Missing** | — |

---

## 4. Authorization matrix — REQUIREMENTS §4

| Aspect | Status | Evidence |
|---|---|---|
| Role-filtered navigation & route guards | **Partial** | `App.tsx:139-164` + `RequireAuth.tsx` |
| Enforcement in backend for **every** resource | **PRESENT** | Superseded 2026-11-05. A real backend enforces `RequirePermission` + row-level ownership on every route; refusal tests exist for each privileged path (see `docs/QA_REPORT.md` and the denial tests in `internal/httpapi`). The original "no backend" claim is no longer true |
| Row-level ownership (Admin sees only own Clients/VPNs) | **Missing** | no queries exist |
| Chat participant checks | **Partial (client only)** | `components/chat/chat-permissions.ts` |
| "Other Clients' credentials — audited only" | **Missing** | no credentials and no audit service exist |

> Per REQUIREMENTS §4's own note and Bible §4: *"All rules enforced in the backend, not just hidden in the UI."* Today **100 % of authorization lives in the UI layer.**

---

## 5. Data model — REQUIREMENTS §5

| Concept | Status |
|---|---|
| `users`, `server_integrations`, `inbounds`, `admin_server_assignments`, `vpn_offers`, `vpn_instances`/`remote_clients`, `vpn_assignments`, `quota_ledger`, `prices`/`sales`, `payment_methods`, `payment_records`, `usage_snapshots`, `notifications`, `chat_conversations`, `chat_messages`, `audit_logs` | **PRESENT** — superseded 2026-11-05. 22 migrations in `backend/internal/store/migrations/` create these tables; see `docs/FINAL_REPORT.md` §6 for the full list. The original row read "All Missing — no database, no schema, no migrations", which is no longer true |
| Stable local ID for remote objects + idempotency keys | **PRESENT** — `outbox` carries `UNIQUE(integration_id, op, idempotency_key)`; `vpn_instances.panel_sub_id` is the deterministic pair (D-012) |

---

## 6. UX / pages — REQUIREMENTS §6

| Role | Pages required | Status |
|---|---|---|
| Super Admin | dashboard, servers, offers, admins, clients, pricing/payments, incidents/audit/chat | **Done (mock)** — 2 route gaps: no Super-Admin *clients* screen (`/clients` does not exist) and no *incidents* screen |
| Admin | overview, assigned servers, client mgmt + premium flow, retail price, own payment/contacts, support chats | **Done (mock)** — `/admin`, `/admin-servers`, `/admin-clients`, `/sale`, `/retail-price`, `/chat`; no dedicated *own contacts/payment details* page |
| Client | dashboard, VPN detail, buy premium, history, notifications/chat, profile | **Done (mock)** — `/client`, `/vpn/:id`, `/buy`, `/history`, `/notifications`, `/chat`, `/profile` |

Route-level detail: `docs/UI_INVENTORY.md` §1 (re-synced 2026-10-23).

---

## 7. Status machines — REQUIREMENTS §7

| Model | Status | Evidence |
|---|---|---|
| Server: `PENDING_VERIFICATION` … `REMOVED` | **Partial** | `stores/serversStore.ts:8-12` declares only 4 of the 6 states (`PENDING_VERIFICATION` and `REMOVED` missing) |
| VPN account/assignment: 9 states | **Missing** | `stores/vpnStore.ts` uses ad-hoc strings; no transition table |
| Payment: 7 states | **Partial** | `stores/paymentsStore.ts` subset; no refund states |
| "Never map an API outage to REVOKED" | **Not enforceable yet** | no outage handling exists |

---

## 8. Security & reliability — REQUIREMENTS §8

| Control | Status |
|---|---|
| Backend authorization / row-level ownership | **PRESENT** — superseded 2026-11-05; `internal/auth` owns the permission matrix and ownership scoping, enforced in the HTTP layer and covered by tests |
| Encrypt integration credentials at rest | **Missing** — the UI accepts a key (`ServersPage.tsx:179`) then discards it |
| HTTPS/TLS validation for panels | **Missing (server-side)** — only a client-side regex `^https:\/\/` |
| SSRF protection (scheme, destination, redirects, DNS rebinding) | **Missing** entirely — no outbound request exists yet to protect |
| Rate limiting (sign-in, Free provisioning, chat, payment, server ops) | **Missing** |
| Google token verification server-side, strong hashing, secure sessions | **Missing** — `authStore` sets a role in memory |
| Input validation (URL/protocol/limit/currency/price/remote-ID) | **Partial** — form-level only (`ServersPage.saveForm`) |
| CSRF/origin protection | **Missing** |
| Transactions/locks for quota + provisioning | **Missing** |
| Audit logging | **Missing** (mock store only) |
| Secret masking in logs/responses | **Not applicable yet** — must be designed in before the first panel call |
| Backup/restore + migration docs | **Missing** |

---

## 9. Theme Studio — REQUIREMENTS §9

**Status: Done.** 5 presets + 40 sub-themes (`frontend/src/theme/themes.ts`, `subthemes.ts`), hex-validated colour pickers, Sinhala font fallbacks, effects Off/Subtle/Enhanced, low-resource mode, `prefers-reduced-motion`, persistence via `themeStore.ts` (zustand persist v4 + migration). Server-side account sync is still pending (Phase 5). Catalogue: `docs/UI_INVENTORY.md` §5.

---

## 10. Deployment & operations — REQUIREMENTS §10

| Requirement | Status |
|---|---|
| Go single binary, SQLite (WAL/FK/migrations), Caddy auto-HTTPS | **Missing** |
| CI + release workflows with checksums | **Missing** (no `.github/`) |
| One-line installer, idempotent, DNS/port validation, one-time Super Admin bootstrap | **Missing** |
| CLI (`status/health/logs/backup/restore/upgrade/rollback/domain set/uninstall`) | **Missing** |
| Panel URL/credential change via UI without redeploy | **Missing** |

---

## 11. 3X-UI capability coverage (contract verified 2026-10-23)

Contract source: official OpenAPI (`3X-UI Panel API v3.x`, 55 schemas) at
`https://raw.githubusercontent.com/MHSanaei/3x-ui/main/frontend/public/openapi.json`;
latest published release **v3.10.2 (2026-10-14)**. Full endpoint table: `docs/3XUI_API_CONTRACT.md`.

| Capability (`new_update.md` §5) | Endpoints verified | Implemented in Veylix |
|---|---|---|
| Authenticated health check | `GET /panel/api/server/status` (CPU/RAM/swap/disk/netIO/xray/tcp/load, 2 s cache) | ⬜ |
| Inbound list / inspect | `GET /inbounds/list`, `/inbounds/list/slim`, `/inbounds/options`, `/inbounds/get/{id}` | ⬜ |
| Inbound create/update/enable/disable/delete | `POST /inbounds/add`, `/update/{id}`, `/setEnable/{id}`, `/del/{id}`, `/bulkDel` | ⬜ |
| Client create | `POST /clients/add` — JSON `{client, inboundIds[]}` | ⬜ |
| Client retrieve | `GET /clients/get/{email}`, `/clients/list`, `/clients/list/paged` | ⬜ |
| Client update / enable / disable / delete | `POST /clients/update/{email}`, `/bulkEnable`, `/bulkDisable`, `/clients/del/{email}` | ⬜ |
| Expiry / traffic / device limits | `Client` fields `expiryTime`, `totalGB`, `limitIp` (+ `reset*`) | ⬜ |
| Traffic statistics | `GET /clients/traffic/{email}`, `clientStats[]` from `/inbounds/list` | ⬜ |
| Online status | `POST /clients/onlines`, `/clients/lastOnline`, `/clients/ips/{email}` | ⬜ |
| Connection links / subscription | `GET /clients/links/{email}`, `/clients/subLinks/{subId}`, `POST /clients/{email}/externalLinks` | ⬜ |
| Server metrics | `GET /panel/api/server/status` | ⬜ |
| Multi-panel synchronisation | per-panel jobs (design in plan Stage E5) | ⬜ |

**Note:** `subId` + `email` is the idempotency pair used to satisfy `new_update.md` §6 ("prevent duplicate client creation") — recorded in D-012.

---

## 12. Tally

> **Re-baselined 2026-10-24 (evening, after the UI truth pass).** The earlier table reflected a frontend-first snapshot with mock stores; those stores no longer exist. §12.1 keeps the Stage-B snapshot for traceability only.

| Status | Count | Areas |
|---|---|---|
| **Already works (proven)** | 9 | Design system / Theme Studio (§9); role-aware UI + routing shell (§6); **auth + sessions + RBAC** (§4, E2); **3X-UI adapter, live-verified** (§11 reads); **provisioning + quota + outbox** (E4); **password policy + change** (§4); **`/api/quotas/me` per-role contract** (F-023, D-021); **sync + usage history + server metrics** (E5, D-023, live-verified); **links/QR + ping, release/restore, panel inspector + tagging, payments, chat, notifications, offers, clients, directory** (2026-10-24) |
| **Partial** | 4 | Panel writes not live-verified beyond create (D-018); per-client FREE monthly cap (D-025.5, no column); chat storage budgets (D-027.6); chart-type/range toggles (D-026) |
| **Missing** | 5 | **~~SPA embedding (E7)~~ — SHIPPED 2026-10-24 (D-029: one binary serves UI + API)**; HWID routes + credential change (§4.2); FREE catalogue/claim routes; §10 deployment surface (installer, Caddy template files, CLI, CI); 39 acceptance tests + email password-reset |
| **Bug** | 2 | **F-031** (dev-only React duplicate-key warning, open); F-006 doc drift (ongoing, gated by `npm run audit` for UI copy) |
| **Blocked** | 3 | Live client provisioning (D-018 production inbound); Google OAuth live test (no credentials); `go test -race` (no C compiler) |

**Bottom line (updated 2026-10-24, late evening):** the project is no longer "a frontend on zero backend". Backend business logic for **identity, authorisation, panel integration, provisioning, quota, sync (including the panel limit mirror), usage history, billing, chat, notifications, the offer catalogue, connection links and the release lifecycle** is implemented and tested (**286 tests, 9/9 packages**), and **all 11 frontend stores are API-backed — 0 mock stores remain** (D-006 exit). The product now also **ships as one 19 MB binary that serves the interface and the API on a single port** (D-029), which is what makes `docs/INSTALL.md` a true statement rather than a plan. What remains is **D-028.1/.2** (panel email naming/rename), the **HWID + credential-change routes**, the **FREE catalogue/claim routes**, a **UI for the existing panel-inspector/tagging routes**, chat storage budgets, and the **deployment/CI surface** (installer, Caddy/systemd files, `vpn-platformctl`, CI, clean-VM test). Live panel writes stay blocked until a non-production inbound exists (D-018).


### 12.1 Historical tally (Stage B snapshot, superseded)

| Status | Count | Areas |
|---|---|---|
| Already works | 2 | Design system / Theme Studio (§9); role-aware UI + routing shell (§6) |
| Partial | 12 | Mock-backed pages across §2, §3, §4 (UI guards), §7 status types |
| Missing | 24 | Backend, DB, migrations, 3X-UI adapter, jobs, provisioning, quota ledger, audit service, auth service, all §8 controls, §10 deployment, and all 12 §11 capability rows |
| Bug | 3 | F-002 fake health check; F-003 credential discarded; F-007 duplicate roadmap/plan lines |
| Blocked | 2 | Live panel verification; Google OAuth live test |

**Why it was wrong to leave it as-is:** every "missing" backend row in that table has since been built (auth, adapter, provisioning, migrations ×4). A tally that says otherwise makes the project look 24 items behind when it is not, and it hides the *real* remaining gap (E5–E7 + deployment), which is what the plan must now be driven by.
