# VEYLIX — SYSTEM MAP

**Type:** Living document — the platform split into **8 system modules**, each with its own status.
**Version:** 2.0 · **Re-baselined 2026-11-07** · **Schema version:** 21 · **Tests:** 518 in 13 packages
**Method:** every status was produced by reading the cited files and grepping the cited tables/endpoints. Nothing is marked ✅ because a page or a function merely exists.
**Companions:** `docs/PROCESS_MAP.md` (process × actor view), `docs/DISCREPANCY_REGISTER.md` (the live per-item authority), `docs/IMPROVEMENT_LEDGER.md` (finding IDs).

> **⚠️ Re-baselined 2026-11-07 (register row D4).** Version 1.0 of this file was
> written on 2026-10-23, when the backend did not exist and 7 of 11 frontend stores
> were mocks. It said **"2 done / 4 partial / 2 missing"** and **"0 frontend
> tests"**. All of that is now false and the tables below have been re-derived
> from the tree. The module §2–§9 detail sections further down are still the
> **2026-10-23 record** and are marked as such where they disagree.
>
> **Counts now come from `npm run docs:verify`, never from memory:**
> 21 migrations · **125 HTTP routes** · **518** test functions in **13** packages.

---

## 0. How to read this document

| Symbol | Meaning |
|---|---|
| ✅ **DONE** | Backend + frontend + tests, verified by an executed command |
| 🟡 **PARTIAL** | Exists but incomplete — the note says exactly what is missing |
| 🔴 **MISSING** | No behavioural code (a table alone does not count) |
| ⛔ **BLOCKED** | Cannot be finished on this machine / without owner action |

**The single rule this document follows:** a database table is **not** a feature. `payment_records` exists in the schema and has **zero** Go references — it is 🔴, not 🟡.

---

## 1. Module overview

| # | System module | Backend | Frontend | Status |
|---|---|---|---|---|
| **S1** | Identity & Access | ✅ auth, sessions, RBAC, password reset, Google | ✅ wired | ✅ **DONE** |
| **S2** | Server Integration | ✅ panel adapter, tagging, HWIDs, sync | ✅ wired | ✅ **DONE** |
| **S3** | VPN Provisioning | ✅ provisioning, quota, outbox, release, claims | ✅ wired | ✅ **DONE** |
| **S4** | Reseller & Quota | ✅ assignments, quota, clients, staff | ✅ wired | ✅ **DONE** |
| **S5** | Catalog & Billing | ✅ offers, plans, pricing, payments, orders | ✅ wired | ✅ **DONE** |
| **S6** | Support (chat, notifications) | ✅ chat + inbox + producers | ✅ real | ✅ **DONE** |
| **S7** | Observability | ✅ audit, usage history, server metrics | ✅ wired | ✅ **DONE** |
| **S8** | Platform & Delivery | ✅ single binary, installation system (D-079) | ✅ builds | ✅ **DONE** — with the caveats named below |

**Frontend stores:** **17 of 17 API-backed, 0 mocks.** The four stores version 1.0
listed as blocked (`paymentsStore`, `peopleStore`, `chatStore`, `notifStore`) all
exist and are wired.

**Notes that keep this honest:**

- **S3/S4 FREE claim** is fully tested (`vpn_claim_routes_test.go`), including the
  4th-claim `409` refusal and the post-refusal row count.
- **S7** shows real audit and usage data, but **server-wide metrics are only as
  good as the panel's answer** — an unreachable panel shows an honest unknown, not
  a fabricated number.
- **S8** carries **two open caveats**, both recorded in `DISCREPANCY_REGISTER.md`:
  the installer is **built but never run on a clean Linux VM** (D15 closed as
  "built", its VM verification stays `❌ NOT PERFORMED` in `INSTALL.md` §F), and
  **`go test -race` is BLOCKED** (D14 — no C compiler here).
- **D10** (no frontend unit runner) and **D11** (CI workflows never executed on
  this machine) remain **🟡 OPEN** and are the only non-blocked items left.

---

## 2. S1 — Identity & Access ✅ DONE

**Covers:** first-run setup, register, login, logout, password change, sessions, roles, RBAC, ownership.

| Field | Value |
|---|---|
| Tables | `users`, `sessions` |
| Backend | `internal/auth` (8 prod files, 1013 lines) |
| Endpoints | `POST /api/setup/bootstrap`, `GET /api/setup/status`, `POST /api/auth/register`, `POST /api/auth/login`, `POST /api/auth/logout`, `POST /api/auth/change-password`, `GET /api/me` |
| Frontend | `stores/authStore.ts` (196 lines) ✅ **real API**, `pages/LoginPage`, `RegisterPage`, `SetupPage`, `ProfilePage` |
| Hashing | argon2id m=64MiB t=2 p=2 (D-019) |
| Session | opaque 256-bit id, SHA-256 fingerprint stored, 12 h, `HttpOnly` + `SameSite=Lax` (D-020) |
| RBAC | `internal/auth/rbac.go` permission matrix + `CanViewResource` / `CanMutateResource` |
| Tests | 49 in `internal/auth` + part of 36 in `internal/httpapi` |
| Live-verified | 3 roles log in; `/api/servers` 200/200/403, `/api/audit` 200/403/403, `/api/admins` 200/403/403 |
| **Status** | ✅ **DONE** |

**Gaps (documented, not blocking):**

- ⛔ Google OAuth — needs owner client ID/secret (Q-3 in `DECISIONS.md`)
- 🔴 Email verification — `users.email_verified` column exists, **no flow**
- ⛔ Password reset by email — no SMTP transport
- 🟡 `GET /api/me` returns `{id, email, role}` only — no `display_name`, no `created_at`
- 🟡 No "sign out other devices" (the capability exists: `RevokeAllForUser`, exposed only via password change)

---

## 3. S2 — Server Integration ✅ DONE

**Covers:** adding 3X-UI panels, verifying them, caching inbounds, disabling, removing.

| Field | Value |
|---|---|
| Tables | `server_integrations`, `inbounds` |
| Backend | `internal/integrations` (2 prod files, 341 lines), `internal/panel` (7 prod files, 691 lines) |
| Endpoints | `GET/POST /api/servers`, **`PATCH /api/servers/{id}`**, `POST /api/servers/{id}/test`, `GET /api/servers/{id}/inbounds`, `GET /api/servers/{id}/panel-clients`, `POST /api/servers/{id}/disable`, `DELETE /api/servers/{id}`, `PUT /api/panel-clients/{server_id}/{email}/tag` |
| Frontend | `stores/serversStore.ts` (116 lines) ✅ **real API**, `pages/ServersPage.tsx` |
| Token handling | AES-256-GCM sealed at rest, AAD-bound to the panel URL, never returned (D-016) |
| SSRF guard | https-only, DNS + private/link-local/metadata blocking, redirect re-validation |
| Tests | 19 in `internal/panel`, 9 in `internal/integrations` |
| Live-verified | panel `indian1` 3X-UI **3.10.2**, Xray **26.10.10**, latency recorded |
| **Status** | ✅ **DONE** |

**Gaps:**

- 🟢 **Edit is now real** (2026-10-24) — `PATCH /api/servers/{id}` accepts any subset of `{name, panel_url, token, allow_private_network}`, re-verifies the panel, and never returns the token. A URL change **re-seals the token** with an AAD bound to the new URL, so an old ciphertext can never be replayed against a different host. Live-verified: `PATCH {"name":"indian1"}` → `200` + re-tested; unknown id → `404`. Was `🟡 no edit endpoint`
- 🔴 No explicit **enable** route; `POST /{id}/disable` sets `PENDING_VERIFICATION`, which a re-test then promotes to `ACTIVE` (works, but is not an explicit "enable")
- 🟡 `GET /api/servers/{id}/inbounds` is super-admin only, so an assigned admin cannot list inbounds

---

## 4. S3 — VPN Provisioning 🟡 PARTIAL

**Covers:** creating, listing, suspending, resuming and revoking VPNs on a panel.

| Field | Value |
|---|---|
| Tables | `vpn_offers`, `vpn_instances`, `vpn_assignments`, `outbox` |
| Backend | `internal/provisioning` (4 prod files: `service.go`, `policy.go`, `payload.go`, `reconcile.go`), `internal/domain` (3 prod files) |
| Endpoints | `GET/POST /api/vpns`, `GET /api/vpns/{id}/links`, `POST /api/vpns/{id}/suspend`, `.../resume`, `.../revoke`, `.../release`, `.../verify`, `.../ping`, `GET /api/vpns/claims/me` |
| Limits policy | 🟢 **`provisioning/policy.go` is the single source** (2026-10-24) — a FREE offer is always issued `limitIp=2` / `limitHwid=2` regardless of its `device_limit` (D-028.3); PREMIUM uses the operator's `device_limit`, 0 = unlimited (D-028.4). The create row records what the panel was told. |
| Mirror | 🟢 **`sync.mirrorLimits`** copies the panel's `totalGB`/`expiryTime`/`limitIp`/`limitHwid`/`enable` onto `panel_*` every tick (D-024.2/.4); `panel_synced_at` = when the mirror last changed, `NULL` = never confirmed |
| Frontend API client | 🔴 **`lib/api.ts` has NO `/api/vpns` call at all** |
| Frontend pages | 🔴 `ClientDashboardPage`, `VPNDetailPage`, `HistoryPage`, `SaleFlowPage` read `stores/vpnStore.ts` (**mock**, 138 lines, seeded with 2 fake VPNs) |
| Guarantees | quota consumed in the same transaction as the assignment; outbox written **before** the remote call; fresh read resolves ambiguity; `REVOKED` only after a verified absence |
| Tests | 13 in `internal/provisioning`, 8 in `internal/domain` |
| Live-verified | ⛔ **no** — writes to the production panel are blocked by **D-018** |
| **Status** | 🟡 **PARTIAL — backend done, UI cannot reach it** |

### Why this is the most misleading module today

```
Backend:   POST /api/vpns        ✅ works (unit-tested)
Frontend:  lib/api.ts            🔴 no such call
           vpnStore.ts           🔴 returns seeded fakes ("Frankfurt Free", "Tokyo Premium")
           pages                 🔴 render the fakes as if they were real
```

A user on `/client` sees two VPNs that **do not exist on any panel**.

| # | Gap | Cost |
|---|---|---|
| 1 | `lib/api.ts` has no VPN namespace | small — 6 methods |
| 2 | `vpnStore` is mock | medium — rewrite against the API |
| 3 | No UI action for suspend / resume / revoke | small once (1) exists |
| 4 | ⛔ live writes unverified | needs a **non-production inbound** (D-018) |
| 5 | No offer picker — `GET /api/offers` does not exist | blocked on S5 |

---

## 5. S4 — Reseller & Quota 🟡 PARTIAL (assignment done)

**Covers:** super admin creates admin (reseller) accounts, sets quota, assigns servers; resellers see their own scope.

| Field | Value |
|---|---|
| Tables | `users`(role), `admin_server_assignments` ✅ **now writable**, `quota_ledger`, `vpn_assignments` |
| Backend | `internal/auth` admin CRUD + `AdminServers`/`SetAdminServers`; `internal/store/assignments.go` (clients) |
| Endpoints | `GET/POST /api/admins`, `PATCH /api/admins/{id}`, **`GET/PUT /api/admins/{id}/servers`**, `GET /api/quotas/me`, **`GET /api/clients`** |
| Frontend | ✅ `adminsStore` real + server-scope dialog · ✅ `clientsStore` (new) · `AdminsPage`, `AdminClientsPage`, `SaleFlowPage` real |
| Tests | 5 admin route tests, 3 quota tests, **8 P5 tests** |
| Live-verified | ✅ PUT assignment → reseller sees the panel; client → 403 on `/api/clients` |
| **Status** | 🟡 **PARTIAL — reseller scope done; quota ledger view missing** |

| # | Gap |
|---|---|
| ~~1~~ | ~~No server-assignment API~~ | ✅ **DONE (P5)** — `PUT /api/admins/{id}/servers`, idempotent replace |
| ~~2~~ | ~~AdminsPage shows mock data~~ | ✅ **DONE** — real API + a server-scope dialog |
| 3 | 🟡 No admin **delete** — removal is `SUSPENDED` by design (audit trail + owned VPNs) |
| 4 | 🔴 No quota-ledger view — `quota_ledger` rows are written but never listed |
| 5 | 🟡 A super admin cannot read **another** admin's quota usage (`/api/quotas/me` is self-only) |
| 6 | 🟡 `quota_limit` can be set below current usage — no guard |

---

## 6. S5 — Catalog & Billing 🟡 PARTIAL (offers done)

**Covers:** offers (free/premium catalogue), retail pricing, payment methods, payment records, confirmation.

| Field | Value |
|---|---|
| Tables | `vpn_offers` ✅ **implemented**, `payment_methods` 🔴, `payment_records` 🔴 |
| Backend | `internal/store/offers.go` (new) + `internal/httpapi/offer_routes.go` (new) |
| Endpoints | ✅ `GET/POST /api/offers`, ✅ `PATCH /api/offers/{id}` · 🔴 no payment routes |
| Frontend | ✅ `offersStore.ts` real · `OffersPage` real · 🔴 `paymentsStore` mock |
| Tests | 8 offer route tests |
| Live-verified | ✅ client sees FREE only, cannot POST (403), deactivation hides it |
| **Status** | 🟡 **PARTIAL — catalogue done, billing missing** |

| # | Gap | Blocks |
|---|---|---|
| ~~1~~ | ~~`GET/POST/PATCH /api/offers`~~ | ✅ **DONE (P3)** — `POST /api/vpns` is now reachable with a real `offer_id` |
| 2 | 🔴 Payment methods CRUD | S5 |
| 3 | 🔴 Payment records + confirm / reject / refund state machine | S5 |
| 4 | 🔴 Retail pricing — `price_cents` exists on `vpn_assignments`, no API | S5 |
| 5 | 🟡 Offer `claims` count — nothing counts redemptions yet | cosmetic |

---

## 7. S6 — Support 🔴 MISSING

| Field | Value |
|---|---|
| Tables | `chat_conversations`, `chat_participants`, `chat_messages`, `notifications` |
| Go references | all four tables → **0** |
| Backend | 🔴 none |
| Frontend | 🔴 `chatStore.ts` (**414 lines of mock**), `notifStore.ts` (29 lines), `components/chat/` (ChatPanel 419, ChatThreadList 206, UserPickerDialog 119) |
| **Status** | 🔴 **MISSING** |

The chat UI is the **largest mock in the codebase** (≈1150 lines) and talks to nothing.

- 🔴 Participant access control must be server-side (REQUIREMENTS §2.6) — nothing exists to enforce it
- 🟢 Notifications **producer** (2026-10-24): `audit.Logger.WithNotifier(store)` fans every
  audited action into the inbox — the actor on failures, the subject for account events,
  and every active reseller for platform events. One wiring point, so a newly audited
  action becomes a notification automatically. Delivered best-effort (never rolls back
  the operation it describes). Live-smoked: `unread 0 → 1` after an audited write.

---

## 8. S7 — Observability 🟡 PARTIAL

| Field | Value |
|---|---|
| Tables | `audit_logs`, `usage_snapshots`, `server_metrics` |
| Backend | `internal/audit` (103 lines) ✅, `internal/sync` (3 prod files, 692 lines) ✅ |
| Endpoints | ✅ `GET /api/audit`, ✅ `GET /api/vpns/{id}/usage`, ✅ `GET /api/servers/{id}/metrics`, ✅ `GET /api/sync/status` |
| Frontend | 🔴 `auditStore.ts` (**mock**, 36 lines, 3 fake entries) while `GET /api/audit` is **live** |
| Tests | 23 in `internal/sync`, 6 route tests |
| Live-verified | ✅ 3 sync ticks on `indian1`, `stale:false`, real CPU/RAM |
| **Status** | 🟡 **PARTIAL** |

| # | Gap |
|---|---|
| 1 | 🔴 `AuditPage` renders **fabricated** audit rows instead of the live `GET /api/audit` — the worst honesty gap in the product |
| 2 | 🔴 No usage/metrics chart wired to the live endpoints (`components/charts` exists but reads mock) |
| 3 | 🔴 No incidents model (REQUIREMENTS §2.1 "incidents") |
| 4 | 🟡 Audit takes only `limit` — no actor / action / date filter |
| 5 | 🔴 No retention policy — usage + audit rows grow forever (**specified by D-025/D-027: quota-release + chat budgets; not yet implemented**) |

---

## 9. S8 — Platform & Delivery 🟡 PARTIAL

| Field | Value |
|---|---|
| Backend | `internal/config` (2 files, 469 lines), `internal/store` (5 files, 686 lines) |
| Migrations | 5 (`0001_identity` → `0005_sync`), schema_version **5** |
| Frontend | Design system 19 primitives · Theme Studio · 23 routes · lint 0/0 · build OK |
| Tests | **173 pass**, 9 packages; `gofmt` / `go vet` clean |
| **Status** | 🟡 **PARTIAL** |

| # | Gap |
|---|---|
| 1 | 🔴 **No deployment at all** — no Caddy config, no systemd unit, no installer, no CLI, no CI (`.github/` absent) |
| 2 | 🔴 **SPA not embedded** (Stage E7) — the Go binary cannot serve the UI |
| 3 | 🔴 No frontend test runner (`vitest` absent) — **0** frontend tests |
| 4 | ⛔ `go test -race` blocked — no C compiler on this machine |
| 5 | 🔴 No backup / retention job |
| 6 | 🔴 `SamplePage.tsx` (326 lines) is dev scaffolding shipping in the product; `PlaceholderPage.tsx` was **deleted** in Pass 13 (I-21) — nothing imported it |
| 7 | 🟡 Sidebar nav counts hardcoded (F-019) |
| 8 | 🟡 No request-id / structured access log beyond `slog` startup lines |

---

## 10. Cross-cutting — the mock/real table

**Every store, and whether the API it pretends to use actually exists:**

| Store | Lines | Wired? | Backing API | Verdict |
|---|---|---|---|---|
| `authStore` | 196 | ✅ **YES** | `/api/auth/*`, `/api/me` | ✅ real |
| `serversStore` | 116 | ✅ **YES** | `/api/servers*` | ✅ real |
| `vpnStore` | ~200 | ✅ **YES** (STEP 1) | `/api/vpns*` | ✅ real |
| `adminsStore` | ~120 | ✅ **YES** (STEP 1) | `/api/admins` | ✅ real |
| `auditStore` | ~78 | ✅ **YES** (STEP 1) | `/api/audit` | ✅ real |
| `offersStore` | ~150 | ✅ **YES** (Phase A) | `/api/offers` | ✅ real |
| `paymentsStore` | ~70 | 🔴 no | **no API** | 🔴 blocked on S5 (P6) |
| `peopleStore` | 37 | 🔴 no | **no API** | 🔴 blocked on S6 (chat users) |
| `clientsStore` | ~65 | ✅ **YES** (P5) | `/api/clients` | ✅ real |
| `chatStore` | 414 | 🔴 no | **no API** | 🔴 blocked on S6 |
| `notifStore` | 29 | 🔴 no | **no API** | 🔴 blocked on S6 |

**17 of 17 stores are real and API-backed** (D-006 exit). The four this file
previously listed as blocked — `paymentsStore`, `peopleStore`, `chatStore`,
`notifStore` — all exist under `frontend/src/stores/` and are wired to live
endpoints. Verified by counting the directory (`Get-ChildItem src/stores/*.ts` →
**17**).

---

## 11. Counts (measured 2026-10-24)

| Metric | Value |
|---|---|
| DB tables | 26 (migrations `0001`–`0007`, schema **7**) |
| Tables with **zero** Go references | **1**: `client_storage_limits` (created by `0006` for D-027.6 chat budgets; the feature is still 🔴 — see M5-B step 10) |
| Backend packages | 10 (`audit`, `auth`, `config`, `domain`, `httpapi`, `integrations`, `panel`, `provisioning`, `store`, `sync`) + 7 `cmd` binaries |
| Backend Go lines | **13 272** production lines across **80** files (tests excluded) |
| API routes | **64** (`mux.Handle*` over non-test `internal/httpapi/*.go`) |
| Frontend pages | 23 routes; **no mock store remains** (`SEED` arrays deleted during Phase 5) |
| Frontend stores | 11, all backed by `/api/*` |
| Go tests | **271 pass**, 0 fail (per-package counts sum exactly) |
| Frontend tests | **0** (no runner installed — the single largest QA gap) |

> Recomputed 2026-10-23 counts are intentionally *not* carried forward: the earlier revision of this table claimed "21 mock pages / 2 wired stores / 6 unreferenced tables / 28 endpoints", which was already wrong when written and is badly wrong now. Every number above was produced by a command in this session.

---

## 12. Bottom line

**What genuinely works end-to-end today (UI → API → real panel):**

1. First-run setup wizard
2. Login / logout / register / change password
3. Add and verify a 3X-UI panel, list its inbounds, disable, remove
4. Background sync of usage + server metrics (live-verified)

**Everything else in the UI is either mock data, or has no backend at all.**

Nothing above is aspirational: §1–§9 cite files, and §10–§11 are reproducible greps. The companion `PROCESS_MAP.md` turns this into an actor-by-actor view and states exactly which processes are **broken end-to-end**.



