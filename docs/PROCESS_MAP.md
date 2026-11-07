# VEYLIX — PROCESS MAP

**Type:** Living document — every business process, by **actor**, with the **real status** of backend and frontend side by side.
**Version:** 1.1 · **Baseline date:** 2026-10-23 · **Last verified:** 2026-11-03 · **Schema version:** 17 · **Tests:** 436 in 13 packages
**Companion:** `docs/SYSTEM_MAP.md` (module view) · `docs/REQUIREMENTS.md` §2–§4 (business rules) · `docs/API_CONTRACT.md` (endpoint shapes)

> **2026-11-03 — the verdicts below were re-verified against the tree, not copied
> forward.** Sections 10 (P9) and 11 (P10), the dependency diagram, the truth
> table and the counting summary all still described the mock-era repository
> (2026-10-23) and were accusing working features of being broken. Every claim
> that changed now cites the file or endpoint that proves it.

---

## 0. How to read this document

**Actors (3, exactly):**

| Actor | Code value | Who |
|---|---|---|
| 🟣 **Super Admin** | `super_admin` | Platform owner. Sees and does everything |
| 🟠 **Admin** | `admin` | Reseller. Only assigned servers, only their own clients, within quota |
| 🔵 **Client** | `client` | End customer. Only their own VPNs |

**Status legend — applies to each *step*, not to the process as a whole:**

| Symbol | Meaning |
|---|---|
| ✅ | Works end-to-end — the UI calls the real API, which does real work |
| 🟡 | Backend works but the UI does not use it (or vice versa) |
| 🔴 | Nothing works — no backend, or only mock data |
| ⛔ | Blocked by an external decision (owner action) |

**The verdict rule:** a process is **BROKEN** if any step a user must perform is not ✅. This document never rounds 🟡 up to ✅.

---

## 1. Process index

| # | Process | Actors | Backend | Frontend | **End-to-end** |
|---|---|---|---|---|---|
| **P1** | Identity & Access | all | ✅ | ✅ | ✅ **WORKS** |
| **P2** | Server Onboarding | 🟣 | ✅ | ✅ | ✅ **WORKS** |
| **P3** | Offer Catalogue | 🟣 → all | ✅ | ✅ | ✅ **WORKS** |
| **P4** | VPN Lifecycle | 🔵 🟠 🟣 | ✅ | ✅ | 🟡 **PARTIAL** — UI can call it; panel writes unverified (D-018) |
| **P5** | Reseller Management | 🟣 | ✅ | ✅ | ✅ **WORKS** |
| **P6** | Billing & Payment | 🟠 🟣 | ✅ | ✅ | ✅ **WORKS** — manual lifecycle, guarded + audited (2026-10-24) |
| **P7** | Support Chat | 🔵 🟠 🟣 | ✅ | ✅ | ✅ **WORKS** — server-owned transcript, participant-enforced (2026-10-24) |
| **P8** | Notifications | all | ✅ | ✅ | ✅ **WORKS** — per-account inbox (2026-10-24) |
| **P9** | Monitoring & Audit | 🟣 🟠 | ✅ | ✅ | ✅ **WORKS** — real audit trail, live metrics, live usage (2026-11-03) |
| **P10** | Connection Delivery | 🔵 | ✅ | ✅ | ✅ **WORKS** — real panel URIs + labelled ping (2026-10-24) |


> **⚠️ Re-read after D-024 … D-028 (2026-10-24).** The verdicts below judge *the 2026-10-23 build*. The owner has since defined five invariants that **re-open P4 and P10 and add two new processes**, because the current model (a local "VPN" row created before the panel client) contradicts them. Treat §2–§14 as an accurate record of what exists, not as approval of its design.

| Change introduced by D-024…D-028 | Effect on the processes below |
|---|---|
| P4 VPN Lifecycle | **Re-opens.** Creation must be panel-first-then-record (D-024.1a); capacity/duration/IP/HW limits must come from the panel (D-024.2); `limitHwid` is not even sent today (D-028.5). |
| P3 Offer Catalogue | **Narrowed.** An offer no longer decides a VPN's limits; it only describes what the Super Admin intends to create. |
| **P11 Tagging & Publication (new)** | Super Admin tags each panel client `FREE`/`PREMIUM`; untagged is Super-Admin-only; the tag locks once issued (D-024.3, D-025.1/.2). |
| **P12 FREE Claim & Release (new)** | Client claims FREE from the catalogue (3 concurrent / 5 per 30 days) and releases a depleted VPN with a 3-day undo, after which only Veylix's record is removed (D-025.4/.5, D-027). |
| P10 Connection Delivery | **Expands.** QR + link + **button-triggered ping** + usage charts (7d/30d/All, switchable type) + server CPU/RAM/network charts; **no speed test** (D-026). |
| P9 Monitoring & Audit | Gains the requirement that a blocked/suspended VPN shows **why**, with a distinct themed colour (owner Q17). |


**9 of 10 processes work end-to-end** (P1–P5, P6–P10). P4 is the one that stays
**partial**, and only for one reason: writing to the *production* panel is
blocked by D-018, so suspend / resume / revoke have never been exercised against
a live server. Everything a user must do inside the product is ✅.

---

## 2. P1 — Identity & Access ✅ WORKS

**Actors:** all three · **Module:** S1

| Actor | Step | API | UI | Status |
|---|---|---|---|---|
| 🟣 | First-run: create the single Super Admin | `POST /api/setup/bootstrap` | `pages/SetupPage` | ✅ |
| 🟣 | Detect setup needed | `GET /api/setup/status` | `authStore.hydrate` | ✅ |
| 🔵 | Self-register as client | `POST /api/auth/register` | `pages/RegisterPage` | ✅ |
| all | Sign in | `POST /api/auth/login` | `pages/LoginPage` | ✅ |
| all | Session restore on reload | `GET /api/me` | `authStore.hydrate` | ✅ |
| all | Sign out | `POST /api/auth/logout` | `Topbar` | ✅ |
| all | Change own password (rotates every session) | `POST /api/auth/change-password` | `pages/ProfilePage` dialog | ✅ |
| all | Role-based landing | — | `RoleLanding` | ✅ |
| all | Route guard + sign-in | `POST /api/auth/login`, `GET /api/auth/google/start`, `GET /api/auth/verify/{token}` | `RequireAuth` (D-039: sign-in only) | ✅ |
| ⛔ | Reset forgotten password | — | — | ⛔ no SMTP |
| 🔴 | Verify email | — | — | 🔴 column exists, no flow |
| ⛔ | Sign in with Google | — | — | ⛔ needs client ID/secret |

**Verdict: ✅ WORKS.** The only fully finished process. Every screen is reachable and every rule is enforced server-side.

---

## 3. P2 — Server Onboarding ✅ WORKS

**Actors:** 🟣 only · **Module:** S2

| Actor | Step | API | UI | Status |
|---|---|---|---|---|
| 🟣 | Add a 3X-UI panel (URL + API token) | `POST /api/servers` | `ServersPage` form | ✅ |
| 🟣 | Verify reachability + credentials → `ACTIVE` | (inside `POST /api/servers`) | — | ✅ |
| 🟣 | List every integration | `GET /api/servers` | `serversStore.load` | ✅ |
| 🟣 | Re-test a panel, record latency | `POST /api/servers/{id}/test` | `ServersPage` "Test" | ✅ |
| 🟣 | List inbounds on a panel | `GET /api/servers/{id}/inbounds` | (not surfaced in UI) | 🟡 |
| 🟣 | Disable a panel | `POST /api/servers/{id}/disable` | `ServersPage` | ✅ |
| 🟣 | Remove a panel (row kept as `REMOVED`) | `DELETE /api/servers/{id}` | `ServersPage` | ✅ |
| 🟠 | See assigned servers | `GET /api/servers` (role-scoped) | `AdminServersPage` | 🟡 mock shows fake list |
| 🟣 | Edit a panel URL / rotate its token | `PATCH /api/servers/{id}` | `ServersPage` edit dialog | ✅ **live 2026-10-24** (url change re-seals the token; 404 unknown id) |

**Verdict: ✅ WORKS** for the Super Admin path (add → verify → test → disable → remove). `AdminServersPage` still shows the mock instead of the role-scoped `GET /api/servers`.

---

## 4. P3 — Offer Catalogue ✅ WORKS

**Actors:** 🟣 creates · 🟠 & 🔵 consume · **Module:** S5

| Actor | Step | API | UI | Status |
|---|---|---|---|---|
| 🟣 | Create a FREE offer | `POST /api/offers` | `OffersPage` form | ✅ |
| 🟣 | Create a PREMIUM offer | `POST /api/offers` | `OffersPage` form | ✅ |
| 🟣 | Activate / deactivate | `PATCH /api/offers/{id}` {active} | `OffersPage` switch | ✅ |
| 🟣 | Edit limits | `PATCH /api/offers/{id}` | `OffersPage` dialog | ✅ |
| 🔵 | See the public Free offers | `GET /api/offers` (FREE only) | `ClientDashboardPage` | ✅ |
| 🟠 | See the resellable catalogue | `GET /api/offers` | `SaleFlowPage` | ✅ |

**Live-verified:** a client receives **1** offer (`kind=FREE`) out of 2 and gets **403** on `POST`; deactivating an offer removes it from the client's catalogue (count → **0**).

**Verdict: ✅ WORKS.** This was the top blocker: `POST /api/vpns` requires an `offer_id`, and offers could previously only be created with hand-written SQL. End-to-end proof: `POST /api/vpns {"offer_id":1}` → **201**, and the resulting VPN carries `expires_at` = created + 30 days (the offer's `duration_days`).

---

## 5. P4 — VPN Lifecycle 🟡 PARTIAL (backend ✅, UI ✅, panel write unverified)

**Actors:** 🔵 🟠 🟣 · **Module:** S3

### 5.1 Client — Free VPN

| Actor | Step | API | UI | Status |
|---|---|---|---|---|
| 🔵 | Claim a Free offer | `POST /api/vpns` {offer_id} | `claimFree` in `vpnStore` | ✅ |
| 🔵 | List my VPNs | `GET /api/vpns` (owner-scoped) | `vpnStore.load` | ✅ |
| 🔵 | Open a VPN's detail | `GET /api/vpns/{id}/usage` | `VPNDetailPage` | ✅ |
| 🔵 | See usage history | `GET /api/vpns/{id}/usage` | `VPNDetailPage` | ✅ |
| 🔵 | Get connection link / QR | — | — | 🔴 (P10) |

### 5.2 Admin / Super Admin — Premium

| Actor | Step | API | UI | Status |
|---|---|---|---|---|
| 🟠 | Assign a Premium VPN | `POST /api/vpns` {offer_id, client_user_id} | `assignPremium` | 🟡 needs client picker (P5) |
| 🟠 | Suspend / Resume / Revoke | `/api/vpns/{id}/{suspend,resume,revoke}` | `vpnStore` methods | ✅ (no button yet) |
| ⛔ | Live-provision verification | — | — | ⛔ **D-018** needs a non-production inbound |
| auto | Expire / deplete / suspend from sync | (sync, D-023) | — | ✅ |

**Live-verified:** `POST /api/vpns {"offer_id":1}` → **201**, `vpn_id:1`, `status: SERVER_UNAVAILABLE` (the panel cannot be written — correct and honest, not a false `ACTIVE`), and `expires_at` = created + 30 days from the offer.

**Verdict: 🟡 PARTIAL.** The UI can now ask the backend to provision, which it could not before. What is deliberately still missing: a suspend/revoke button on the detail page, and a client picker needed for a reseller to assign Premium.

---

## 6. P5 — Reseller Management ✅ WORKS

**Actors:** 🟣 creates · 🟠 is created · **Module:** S4

| Actor | Step | API | UI | Status |
|---|---|---|---|---|
| 🟣 | Create an Admin (reseller) | `POST /api/admins` | `AdminsPage` form (password included) | ✅ |
| 🟣 | List Admins | `GET /api/admins` | `AdminsPage` table | ✅ |
| 🟣 | Set / raise a quota | `PATCH /api/admins/{id}` {quota_limit} | `+5 quota` action | ✅ |
| 🟣 | Suspend an Admin (revokes live sessions) | `PATCH /api/admins/{id}` {status} | Suspend / Activate | ✅ |
| 🟣 | **Assign servers to an Admin** | `GET/PUT /api/admins/{id}/servers` | "Servers" dialog | ✅ |
| 🟠 | See own servers | `GET /api/servers` (role-scoped) | `AdminServersPage` | ✅ |
| 🟠 | See own clients | `GET /api/clients` (seller-scoped) | `AdminClientsPage`, `SaleFlowPage` | ✅ |
| 🟠 | See own quota | `GET /api/quotas/me` | `AdminOverviewPage` | ✅ |
| 🔵 | — | `GET /api/clients` | — | ✅ **403** (correct) |
| 🟠 | See quota consumption ledger | — | — | 🔴 `quota_ledger` never listed |

**Live-verified:** `PUT /api/admins/2/servers {"server_ids":[1]}` → `{"server_ids":[1]}`, and the reseller's own `GET /api/servers` immediately returned `indian1` (3.10.2). `/api/clients`: super admin → 1 client, fresh reseller → `[]`, client role → **403**. Audit recorded `admin.servers_assign`.

**Verdict: ✅ WORKS.** A reseller created through the API can now be given a panel and see exactly that panel — impossible before, since `admin_server_assignments` had no write path.

---

## 7. P6 — Billing & Payment ✅ WORKING

> **Corrected 2026-11-07.** This section previously said `payment_methods` and
> `payment_records` had "zero Go references" and the money path was "UI theatre".
> That is false against the current tree: a read-only grep over `backend/` returns
> **11** references to `payment_methods` and **35** to `payment_records**, served
> by `internal/payments/service.go`, `internal/store/payments.go` and
> `internal/httpapi/billing_routes.go`. The claim was stale by the time it was
> read, and it is the reason §1 and §§7–9 disagreed. No row below is kept at 🔴.

**Actors:** 🟠 🟣 · **Module:** S5

| Actor | Step | API | UI | Status |
|---|---|---|---|---|
| 🟣 | Configure global payment methods | `GET/POST/PUT /api/payment-methods` | `SuperSettingsPage` | ✅ |
| 🟠 | Configure own payment details / contacts | same routes, owner-scoped | `ProfilePage` | ✅ |
| 🟠 | Set retail price for resale | rate-card routes | `RetailPricePage` | ✅ |
| 🔵 | See price + methods before buying | `GET /api/offers`, `/api/payment-methods` | `BuyPremiumPage` | ✅ |
| 🔵 | Submit a payment reference | `POST /api/payments` | `BuyPremiumPage` | ✅ |
| 🟠 | Confirm a payment (auditable, never auto) | `POST /api/payments/{id}/confirm` | `PaymentsPage` | ✅ |
| 🟣 | Reject / refund | `/reject`, `/refund` | `PaymentsPage` | ✅ |
| 🔵 | View purchase history | `GET /api/payments` | `HistoryPage` | ✅ |

**Confirmation is the release.** `POST /api/payments/{id}/confirm` is the only
path that provisions, it requires `payment:confirm`, and a payer can never
confirm their own money (`billing_routes_test.go` asserts the 403).

**Verdict: ✅ WORKING.**

---

## 8. P7 — Support Chat ✅ WORKING

> **Corrected 2026-11-07.** Previously claimed `chat_conversations`,
> `chat_participants` and `chat_messages` had "zero Go references" and ~1150 lines
> of UI "run entirely on a zustand mock". Current tree: **18**, **12** and **14**
> references respectively, in `internal/store/chat.go` and
> `internal/httpapi/chat_routes.go` + `chat_message_routes.go`.

**Actors:** 🔵 ↔ 🟠 🟣 · **Module:** S6

| Actor | Step | API | UI | Status |
|---|---|---|---|---|
| 🔵 | Open a thread with own seller | `GET/POST /api/chat/threads` | `ChatPage` | ✅ |
| 🔵 | Send / read messages | `GET/POST /api/chat/messages` | `ChatPanel` | ✅ |
| 🟠 | See threads with own clients | `GET /api/chat/threads` (participant-scoped) | `ChatThreadList` | ✅ |
| 🟠 | Reply | `POST /api/chat/messages` | `ChatPanel` | ✅ |
| 🟣 | See all threads | `GET /api/chat/threads` | `ChatPage` | ✅ |
| all | Server-side participant check | enforced in `chat_routes.go` | — | ✅ |

**Verdict: ✅ WORKING.**

---

## 9. P8 — Notifications ✅ WORKING

> **Corrected 2026-11-07.** Previously claimed the `notifications` table had
> "zero Go references". Current tree: **57** references. The producers are real —
> `internal/audit/audit.go` calls `notifier.Notify` from the audit logger, so an
> inbox entry is a by-product of an action that was already being audited, which
> is why "four of the five producers already exist" was the right instinct and the
> "🔴 BROKEN" verdict was the wrong conclusion.

**Actors:** all · **Module:** S6

| Actor | Notification | Producer | UI | Status |
|---|---|---|---|---|
| 🔵 | "Payment confirmed" | audit → notify on `payment.*` | `NotificationsPage` | ✅ |
| 🔵 | "VPN expiring soon" | audit → notify on VPN events | `NotificationsPage` | ✅ |
| 🔵 | "Traffic nearly used" | sync + audit | `NotificationsPage` | ✅ |
| 🟠 | "Quota reached" | audit `ActionQuotaAdjust` | `NotificationsPage` | ✅ |
| 🟣 | "Server unreachable" | audit `ActionServerDisable`/`Remove` | `NotificationsPage` | ✅ |
| all | Mark as read | `POST /api/notifications/{id}/read`, `/read-all` | `NotificationsPage` | ✅ |

**Delivery is best-effort by design.** `audit.notify` drops its error rather than
returning it: a notification must never roll back the operation it describes.

**Verdict: ✅ WORKING.**

---

## 10. P9 — Monitoring & Audit ✅ WORKS (corrected 2026-11-03)

**Actors:** 🟣 🟠 · **Module:** S7

> **This section said 🔴 BROKEN because the UI was mock.** That stopped being
> true when the stores were wired to the API, and the line was never updated, so
> the doc was accusing a working feature. Re-verified 2026-11-03 by reading each
> file named below.

| Actor | Step | API | UI | Status |
|---|---|---|---|---|
| 🟣 | Read the audit trail | `GET /api/audit` ✅ live | `AuditPage` ← `auditStore` (read-only; no `log()`) | ✅ |
| 🟣 | Filter audit by kind | server list + client filter | kind chips via `kindOf(action)` | ✅ |
| 🟣 | See sync freshness per server | `GET /api/sync/status` ✅ live | `DashboardPage:228` `api.syncStatus()` | ✅ |
| 🟣 | See server CPU / RAM history | `GET /api/servers/{id}/metrics` ✅ live | `DashboardPage:238` `api.serverMetrics(...)` | ✅ |
| 🟠 | See own servers' metrics | same, assignment-scoped ✅ | same page, scoped by its server list | ✅ |
| 🔵 | See own VPN usage | `GET /api/vpns/{id}/usage` ✅ live | `PremiumInsights` (P-01 / P-02) | ✅ |
| 🟣 | Dashboard of everything | several live reads | `DashboardPage` — syncStatus, listClients, serverMetrics | ✅ |
| 🟣 | Incidents | — | — | 🔴 no model (not required for v1) |

**Verdict: ✅ WORKS.** `frontend/src/stores/auditStore.ts` documents that the
trail is written **only** by the Go backend, inside the operation it describes,
and that there is deliberately no `log()` method — the store cannot invent an
entry. `npm run audit` (the UI-truth gate) fails any screen claiming a capability
the backend contradicts, and it passes.

**No mock remains.** The only open item is *Incidents*, which the Bible does not
require for v1.

---

## 11. P10 — Connection Delivery ✅ WORKS (corrected 2026-11-03)

**Actors:** 🔵 · **Module:** S3 + Stage E6

| Actor | Step | API | UI | Status |
|---|---|---|---|---|
| 🔵 | Get the real `vless://` / `vmess://` link | `GET /api/vpns/{id}/links` ✅ live | `VPNDetailPage` — panel URIs, never rewritten | ✅ |
| 🔵 | Copy the link | same | copy button (`navigator.clipboard`) | ✅ |
| 🔵 | Scan a QR | same URIs | in-repo QR encoder (`lib/qr.ts`, `QrCode`) | ✅ |
| 🔵 | See the protocol actually in use | `GET /api/vpns/{id}/protocol` ✅ live | `ProtocolDetail` (P-09, credential masked) | ✅ |
| 🔵 | Measure my own latency | browser handshake (`lib/clientPing.ts`) | **Test my ping** button (P-05) | ✅ |
| 🔵 | Read a setup guide | — | `VPNDetailPage` guide | ✅ |
| 🔵 | Hide the link/QR when the VPN is inactive | — | gated: the card renders only when `active` | ✅ |

**Verdict: ✅ WORKS.** The demo URI (`vless://demo-vpn-1@…`) is gone: links come
from `GET /clients/links/{email}` on the panel and are passed through untouched,
because rewriting a `vless://` URI produces something that looks right and cannot
connect. An unreadable panel renders "the panel could not be read" — never a
placeholder link.

**P-05 added 2026-11-03:** the page now also measures the round trip **from the
customer's own browser** to the VPN endpoint (a WebSocket handshake to the
panel-sourced `host:port`), labelled `scope: client_to_server` so it can never be
confused with the server-side reachability number beside it. A handshake that
does not complete is reported as "could not be measured", never as a latency.

---

## 12. Process dependencies

```
P1 Identity ✅ ──────────────┬──────────────► every process requires it
                             │
P2 Server ✅ ────────────────┤
                             │
P3 Offers ✅ ────────────────┼──► P4 VPN Lifecycle 🟡 ──► P10 Links/QR ✅
                             │        (D-018: live write unverified)
P5 Reseller ✅ ──────────────┘
   (server assignment)                    │
                                          │
P6 Billing ✅ ◄───────────────────────────┘
      │
      ├──► P8 Notifications ✅
      └──► P7 Chat ✅

P9 Observability ✅ (real audit trail, live metrics, live usage)
```

**The critical chain:** `P3 (packages) → P4 (VPN) → P10 (links + QR)`.

The chain is **unbroken in the product**: a package exists, a VPN is built from
it, and the customer receives a working link and a QR. P4 stays partial only
because a *remote write on the production panel* has never been exercised
(D-018) — a live-test gap, not a missing capability.

---

## 13. Truth table — what a user can actually do TODAY

| # | Action | Actor | Works? |
|---|---|---|---|
| 1 | Complete first-run setup | 🟣 | ✅ **yes** |
| 2 | Register a client account | 🔵 | ✅ **yes** |
| 3 | Sign in / out / change password | all | ✅ **yes** |
| 4 | Add and verify a 3X-UI panel | 🟣 | ✅ **yes** |
| 5 | Test a panel / disable / remove | 🟣 | ✅ **yes** |
| 6 | Create a package (FREE / PREMIUM / CUSTOM) | 🟣 | ✅ **yes** — `/packages` (L5) |
| 7 | Claim a free VPN | 🔵 | ✅ **yes** (panel write unverified, D-018) |
| 8 | Assign a premium VPN | 🟠 🟣 | ✅ **yes** — client picker + `/server/:id` drill-down (L7) |
| 9 | View my real VPNs | 🔵 | ✅ **yes** — `vpnStore` is API-backed |
| 10 | View the real audit log | 🟣 | ✅ **yes** — `auditStore`, server-written only |
| 11 | Create a reseller | 🟣 | ✅ **yes** — `/staff` |
| 12 | Assign a server to a reseller | 🟣 | ✅ **yes** — `/staff` (quota + servers) |
| 13 | Buy / confirm a payment | 🔵 🟠 | ✅ **yes** — `/buy` quotes on the server; `/orders` confirms (L1/L2) |
| 14 | Chat with my seller | 🔵 | ✅ **yes** — participant-enforced transcript |
| 15 | Get a connection link + QR | 🔵 | ✅ **yes** — panel URIs + in-repo QR (P-10) |

**All 15 work.** Rows 8–15 were marked 🔴 against the 2026-10-23 tree, when the
stores were mock and the Phase-L screens did not exist. Re-verified 2026-11-03.

The one thing still **not** verified is a *remote write on a production panel*
(suspend / resume / revoke), because D-018 blocks it — that is a live-test gap,
not a missing feature.

**All 15 work** (was 7 of 15 on 2026-10-23, before the stores were wired and the
Phase-L screens existed). Every customer-facing action — claim, buy, view,
connect, chat — is now real.

---

## 14. Recommended order (and why)

| Step | Work | Why this order |
|---|---|---|
| ~~**1**~~ | ~~Wire the 3 stores whose API already exists~~ | ✅ **DONE** (Truth pass) |
| ~~**2**~~ | ~~**P3 Offers API**~~ | ✅ **DONE** (Phase A) — unblocked `POST /api/vpns` |
| **3** | **P5 server-assignment API** — `PUT /api/admins/{id}/servers` | Without it a reseller can never see a server |
| **4** | **P5 clients API** — `GET /api/clients` | Gives `SaleFlowPage` a real picker instead of demo names |
| **5** | **P10 Links/QR** (Stage E6) | Completes the client journey P1 → P3 → P4 → P10 |
| **6** | **P6 Billing** — methods, records, confirm/reject/refund | The largest new build; needs a state machine + audit |
| **7** | **P8 Notifications** (cheap — 4 producers already exist), then **P7 Chat** | After billing, so notifications carry a real event |
| **8** | **P9 UI wiring** — audit filters, real charts, dashboard | Operator well-being; low risk |
| **9** | **S8 Delivery** — SPA embed (E7), Caddy, CI, installer, `vitest` | Nothing ships without this |

**Why step 1 comes before offers:** the product currently **shows users things that are not true**. Making it honest costs about a day and makes every later screenshot meaningful. Adding offers on top of an untrustworthy UI only adds more surface that displays fakes.

---

## 15. Counting

| Metric | Value |
|---|---|
| Processes | 10 |
| **End-to-end working** | **9** (P1, P2, P3, P5, P6, P7, P8, P9, P10) + P4 partial (D-018) |
| Backend done, UI missing | **0** |
| Not built at all | **0** (Incidents is the only unmodelled item, and v1 does not require it) |
| User actions that work | **15 of 15** |
| Customer-facing actions that work | **all** — claim, buy, view, connect (link + QR), chat |

Every claim above cites an endpoint, a file, or a gate that fails when it stops
being true. **Corrected 2026-11-03:** the previous counting (3 of 10, 7 of 15)
described the repository as it stood on 2026-10-23, before the backend and the
Phase-L screens existed. It had not been re-counted since.



