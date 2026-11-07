# VEYLIX VPN Platform — Requirements & Scope

**Version:** 1.1
**Derived from:** MASTER_PRODUCT_BIBLE.md v2.0 + owner decisions (tracked in docs/DECISIONS.md) + `docs/new_reqierment/new_update.md` (2026-10-23)
**Purpose:** This document is the detailed, implementation-oriented requirements and scope. The Bible remains the source of truth for business rules; this file clarifies scope, order, and owner-approved deviations.
**Status of this revision (2026-10-24):** added **§2.0** (the central invariant, D-024: a VPN *is* a 3X-UI client), **§2.0.1** (tag visibility, FREE quotas, VPN release — D-025/D-027/D-028) and **§2.0.2** (VPN detail page — D-026). No prior rule removed; §2.1–§2.4 are now subordinate to §2.0. (2026-10-23 revision: the verified 3X-UI contract reference (§3), requirement detail imported from `new_update.md` §5/§6 (§5), and a traceability map (§13).) Reality check per section lives in `docs/GAP_ANALYSIS.md`.

---

## 1. Overview

Build a multi-server VPN sales and management platform integrated with 3X-UI panels, with three roles:

- **Super Admin** — platform owner / ultimate controller.
- **Admin** — reseller who buys capacity from Super Admin and sells/assigns to Clients under a quota.
- **Client** — signs up, picks a Free offer, or contacts a seller to buy Premium VPN access.

Target production server: **1 vCPU / 500 MB RAM**. This repo is a **greenfield** build (it contained only the Bible; no existing app, no code, no database).

---

## 2. Confirmed business rules (from the Bible — preserved)

### 2.0 THE CENTRAL INVARIANT — a VPN *is* a 3X-UI client (D-024)

> **Every rule in §2 is subordinate to this section.** If a requirement below can only be satisfied by inventing a VPN that is not a client on its panel, the requirement is wrong, not the invariant.

The 3X-UI panel is the **only** authority on what a VPN is and what it can do. Veylix mirrors the panel; it never invents, overrides or predicts it.

| ID | Invariant |
|---|---|
| **I-1** | A VPN exists **iff** a 3X-UI client with the same `(server_id, panel_email)` exists on that panel. No panel client ⇒ no VPN. |
| **I-2** | **Capacity (`totalGB`), duration (`expiryTime`), device limit (`limitIp`), usage (`traffic`), and enable-state (`enable`) come from the panel — only.** Veylix reads and mirrors them; it never derives them from an offer, a form or a user. |
| **I-3** | **Free vs Premium is a Super-Admin-supplied tag** on a panel client. It is stored locally, never written to the panel. An **untagged client is not a VPN** in the catalogue — it is visible only to the Super Admin, in an "Untagged clients" queue, until tagged. |
| **I-4** | **The panel wins on every conflict.** A value changed on the panel (10 GB → 20 GB, expiry extended, client disabled) is reflected by the next sync. There is **no operator override** of a panel-supplied value. |
| **I-5** | **Absence is verified, never assumed.** A client missing from a page-through becomes `SERVER_UNAVAILABLE` (never `REVOKED`) until `GET /clients/get/{email}` positively reports it absent. An unreadable panel changes nothing. |

**Reading (load):** inbounds and their clients are loaded from the panel (`GET /panel/api/inbounds/options`, `GET /panel/api/clients/list/paged`); the loaded, **tagged** clients *are* the VPN list. Free/Premium is attached to them — never typed onto a separate object.

**Writing (create):** create the client on the panel **first**, confirm it with `GET /panel/api/clients/get/{email}`, and only then record it locally. A failed or unconfirmed create leaves **no VPN row at all** — only an outbox entry and an audit record.

**Clients created directly in 3X-UI are real VPNs.** They arrive *untagged* and join the catalogue the moment the Super Admin tags them.

**Consequence for §2.1–§2.4 below:** where the Bible says "define Free offer rules (traffic limit, expiry, device limit)", those numbers now describe **what the Super Admin intends to create on the panel**, not a value Veylix imposes on a VPN afterwards. The panel's stored value is the truth for every VPN.

### 2.0.1 Tag visibility, FREE quotas, and VPN release (D-025 / D-027 / D-028)

| Rule | Requirement |
|---|---|
| **Untagged = Super Admin only** | Every panel client is a VPN (I-1), but an *untagged* VPN is visible **only** to the Super Admin. Tagging it `FREE`/`PREMIUM` publishes it to resellers/clients. |
| **FREE is public, PREMIUM is resale** | Clients self-claim FREE from the catalogue; resellers only receive/sell PREMIUM; the Super Admin may issue either. |
| **FREE caps** | At most **3 concurrent** FREE VPNs per client per server, and **5 claims per rolling 30 days** per client per server (Super Admin sets the number, default 5, adjustable per client). **Releasing does not refund a monthly claim** — the cap must be shown to the client before claiming. |
| **Tag locking** | A VPN's FREE/PREMIUM tag may be changed only while it is **unissued**. Once assigned or sold, the tag is locked. |
| **PREMIUM is unlimited** | Premium volume is bounded only by the reseller's quota. |
| **Depleted VPN release** | When a VPN's quota is exhausted the client may **release** it, starting a **3-day grace period with undo**. After 3 days Veylix **removes its record locally**; the **panel client is never deleted**. |
| **Chat storage** | A separate per-client budget (bytes and/or messages) set by the Super Admin. The client sees usage and may delete their own messages. Chat storage never blocks VPN use. |
| **Email identity** | A VPN's panel `email` is the owning client's Veylix email, suffixed `-2`, `-3`, … when taken. **Assigning a VPN renames its panel client** to the client's identity. |
| **Fixed FREE limits** | `limitIp = 2`, `limitHwid = 2`. PREMIUM limits are chosen by the Super Admin. |

### 2.0.2 VPN detail page (D-026)

- **PREMIUM:** QR + copyable link, obtained→expiry dates, a **"Test ping" button** showing the measured round-trip, **usage history** with a **7d / 30d / All-time** range toggle and a **user-chosen chart type** (line/bar/area/pie), and separate **CPU / RAM / network** server charts with their own range toggle.
- **FREE:** QR + link, the ping button, and a **7-day** usage chart only.
- **No speed test** and no background pinging (D-026.1). Every chart labels its source and window; empty charts say "no data" rather than drawing a zero line. Status colours come from **theme tokens**.



### 2.1 Super Admin
- Add/view/edit/verify/disable/remove 3X-UI server integrations (panel URL + API credentials).
- Dashboard with every server's connection/health state, inbounds, clients, traffic, metrics.
- Create/edit/suspend/remove Admin accounts; assign servers and control permissions.
- Create/manage VPN accounts on panels; choose inbound, protocol, limits supported by panel.
- Set offer type Free/Premium; define Free offer rules (server/inbound, traffic limit, expiry, device limit).
- Set wholesale price/terms for capacity sold to Admins; set direct-sale price to Clients.
- Configure global payment methods and contact defaults.
- View all assignments, users, sales, usage, audit logs, incidents, notifications.

### 2.2 Admin (reseller)
- See/manage only Clients and VPNs on **assigned servers**.
- Assign Premium VPNs **within active quota**, only from permitted servers/offers.
- Set retail price for their resale, subject to platform policy.
- Configure own contacts (phone, WhatsApp, Telegram, email, internal chat) and payment details.
- View own clients' assignments, sales, payment status, usage.
- Internal chat with assigned Clients.
- **Quota rule:** quota = max number of active Premium assignments. Expiry/revocation releases a slot.

### 2.3 Client
- Register with Google or email/password (secure storage, email verification).
- See public Free offers; selecting one provisions a **separate individual** 3X-UI client/credential.
- Buy Premium from an Admin or Super Admin after sale flow.
- Own detail page per VPN: status, usage, limits, connection details, QR/link, setup guide, seller + chat.
- View purchase/history even if the server later becomes unavailable/removed.

### 2.4 Free vs Premium
- Free = public offer, per-Client distinct credential (never a single shared credential).
- Free ends when configured limit exhausts; removed offers leave Clients' active catalog.
- Premium = sold/assigned VPN with stored seller, price, expiry, limits, history.

### 2.5 Payment process
- Two paths: **Reseller sale** (Admin sets retail on wholesale allocation) and **Direct sale** (Super Admin to Client).
- Manual payment default: bank/eZ Cash/mobile-money instructions; authorized seller confirms payment (auditable — never auto-confirm).
- Client sees price, methods, instructions, seller contact before purchase.

### 2.6 Seller ownership & chat
- Seller stored **per Premium assignment** (a Client can hold VPNs from different Admins).
- Support phone, WhatsApp, Telegram, email + internal chat; hide unsupplied methods.
- Chat participant access enforced server-side.

### 2.7 VPN detail & statistics
- Separate per-VPN detail page. Sections: status, usage, limits, connection details, server metrics, history.
- Never invent history; store time-stamped snapshots going forward; label old periods unavailable.
- Distinguish **Client VPN traffic** from **whole-server network traffic**.

### 2.8 QR / links / setup guides
- Correct connection data per 3X-UI client/protocol (never derive fake links).
- Copyable QR/link; guides for Android/Windows/macOS stored as editable content.
- Never show a working-looking QR/link for inactive/revoked VPN.

---

## 3. Server integration & lifecycle

- **Add/update server:** save panel URL + encrypted credentials; verify connectivity/auth + API capabilities before ACTIVE; capture version.
- **Credential change detection:** mark REAUTH_REQUIRED / CONNECTION_UNAVAILABLE; block new assignments; notify Super Admin + responsible Admins; bounded retry; never claim remote revocation without verified API response.
- **Remove/offline:** Free → remove from active list (keep audit). Premium → **never hard-delete**; keep history; state → SERVER_UNAVAILABLE / REAUTH_REQUIRED / REVOCATION_PENDING; no active QR/link.
- **Sync:** per-server background jobs, timeouts, pagination, last-sync timestamps, stale/unknown states; one slow panel must not block the dashboard.

### 3.0 3X-UI is an external, owner-supplied dependency (D-080)

- **External:** the owner installs and runs their own 3X-UI panel(s) and creates an API token. Veylix connects through the panel's HTTP API only; no panel source is vendored or forked.
- **Not bundled:** Veylix does not install, bundle, upgrade or auto-provision 3X-UI, and does not manage a panel process.
- **Many panels, one owner:** one Veylix installation may connect to multiple panels — all belonging to that installation's owner.
- **Owned records:** panel records and credentials are rows in that installation's own database; the token is sealed with that installation's key (`docs/DECISIONS.md` D-011/D-016).
- **Owners requiring complete VPN-resource isolation must run their own panels.** Veylix's installation boundary isolates Veylix data; it cannot isolate the panel itself.
- **Shared-target hazard:** manually pointing two Veylix installations at the **same** external panel lets both manage the same underlying VPN resources; local database isolation cannot prevent that. Nothing in Veylix discovers, imports or synchronises another installation's panels, so it can only happen deliberately.
- **No cross-instance panel discovery or synchronisation** will be implemented.

### 3.5 Verified 3X-UI contract (2026-10-23)

All panel behaviour is coded against the **official OpenAPI spec** recorded in `docs/3XUI_API_CONTRACT.md`:

- **Auth:** `Authorization: Bearer <token>` per integration (D-011). The `POST /login` session-cookie flow is documented as browser-only and is not used.
- **Health check / capability detect:** `GET /panel/api/server/status` (returns CPU/RAM/swap/disk/netIO, `xray.state`, `xray.version`; panel-caches for 2 s → never poll faster than ~30 s per panel).
- **Runtime version check:** `GET /panel/api/openapi.json` on the deployed panel; capabilities absent from that spec are shown as "unsupported by this panel version", never silently faked.
- **Client reads:** `GET /panel/api/clients/list/paged` for lists (D-012); `/inbounds/list` only when per-inbound `clientStats` is required.
- **Status mapping:** 401/403 → `REAUTH_REQUIRED`; timeout/DNS/TLS/5xx → `CONNECTION_UNAVAILABLE`; unverified new integration → `PENDING_VERIFICATION`; operator → `DISABLED` / `REMOVED`. **An outage must never produce `REVOKED`.**
- **Fresh-state verification:** every state-changing panel call is confirmed with a follow-up read; an ambiguous timeout is resolved by reading `GET /clients/get/{email}` before any retry (outbox, D-012).
- **Links/QR:** `GET /clients/links/{email}` is the only permitted source.
- **Panel authority over existence and limits (D-024):** whether a VPN exists, and what its capacity / expiry / device limit / usage / enable-state are, is decided **only** by the panel (`GET /clients/get/{email}`, `GET /clients/list/paged`, `GET /inbounds/options`). A local row never overrides a panel value. The single operator-supplied VPN attribute is the Free/Premium tag, stored locally and never written to the panel.
- **Forbidden endpoints:** bulk/destructive ones (`delAllClients`, `delDepleted`, `delOrphans`, `resetAllTraffics`, `pushClientTraffics`) must never be called by Veylix.


---

## 4. Roles & authorization matrix

| Action | Super Admin | Admin | Client |
|---|---|---|---|
| Manage all panel integrations | Yes | No | No |
| Global server/inbound list | Yes | Assigned only | Safe details for own VPN / public offers |
| Create/approve offers + base limits | Yes | No (use allowed offers) | No |
| Provision Free account | Governs rules | No (unless delegated) | Selects offer; no self-set limits |
| Assign Premium VPN | Yes (incl. direct) | Within assigned server + quota | No |
| Direct sale price | Yes | No | No |
| Retail price for resale | Policy/constraints | Yes | No |
| Create/edit/suspend Admins | Yes | No | No |
| Edit own profile/contacts | Yes | Yes | Permitted fields only |
| See own VPN usage/history | Yes | Relevant assignments | Yes |
| Other Clients' credentials | Audited only | No | No |
| Server-wide CPU/RAM/network | Yes | Assigned servers | Labeled server-wide only if enabled |
| Configure own payment details | Global | Own account only | No |
| Global audit/system settings | Yes | No | No |

> All rules **enforced in the backend**, not just hidden in the UI.

---

## 5. Data model (concepts to implement)

users, server_integrations, inbounds, admins (reseller), admin_server_assignments, vpn_offers, vpn_instances/remote_clients, vpn_assignments, quota_ledger, prices/sales, payment_methods, payment_records, usage_snapshots, notifications, chat_conversations, chat_messages, audit_logs.

Rules: stable local IDs for remote objects; idempotency/unique keys to prevent duplicates on retry.

**Data-integrity rules imported from `new_update.md` §6 (2026-10-23):**

- **Veylix is authoritative** for customers, account ownership, packages/pricing, orders/payments/invoices, subscription lifecycle and renewal, roles/authorization, and the application audit history. 3X-UI remains the external VPN-management integration only.
- Maintain **explicit mappings** between Veylix customers ↔ subscriptions ↔ panel (server integration) ↔ inbound ↔ panel client identifier.
- Prevent duplicate client creation (idempotency pair `email` + deterministic `subId`, D-012), cross-customer data leakage, concurrent-update corruption, and inconsistent billing records.
- Use **transactions** for local consistency and an **outbox + reconciliation** workflow for remote operations.
- **Never claim** that a transaction atomically covers both the local database and a remote 3X-UI panel.
- **Preserve historical usage** when remote traffic counters are reset (monotonic lifetime totals, `docs/IMPROVEMENT_LEDGER.md` F-010).

Additional tables implied by the rules above: `outbox` (remote operation queue with idempotency key, attempts, last error) and `usage_snapshots.lifetime_*` columns.


---

## 6. UX / pages required

- **Super Admin:** dashboard, server integrations, VPN offers, admin management, clients, pricing/payments, incidents/audit/chat.
- **Admin:** overview (quota/clients/payments/sales), assigned servers, client mgmt + premium flow, retail price + own payment/contacts, support chats.
- **Client:** dashboard (free offers + own VPN cards), VPN detail (status/limits/usage/QR/link/guide), premium purchase/contact, purchase/history, notifications/chat, profile/preferences.

---

## 7. Status models (explicit state machines)

- **Server:** PENDING_VERIFICATION, ACTIVE, REAUTH_REQUIRED, CONNECTION_UNAVAILABLE, DISABLED, REMOVED.
- **VPN account/assignment:** PENDING, ACTIVE, TRAFFIC_DEPLETED, EXPIRED, SUSPENDED, SERVER_UNAVAILABLE, REVOCATION_PENDING, REVOKED, REMOVED_FROM_CATALOG.
- **Payment:** PENDING, AWAITING_CONFIRMATION, CONFIRMED, REJECTED, CANCELLED, REFUND_PENDING, REFUNDED.

Never map an API outage directly to REVOKED. Keep purchase history independent of active access status.


---

## 8. Security & reliability

- Backend authorization for every resource; server-side row-level ownership checks.
- Encrypt integration credentials at rest; never commit prod secrets.
- HTTPS/TLS validation; no global certificate-disable shortcuts.
- SSRF protection for panel URLs (scheme + destination validation; private-network allowance is an explicit Super Admin policy).
- Rate-limit sign-in, Free provisioning, chat, payment, server ops.
- Server-side Google token verification; strong password hashing; secure sessions.
- Validate all URL/protocol/limit/currency/price/remote-ID inputs.
- CSRF/origin protections; transactions/locks for quota + provisioning; audit logs; mask secrets in logs; backup/restore + migrations documented.

---

## 9. Theme Studio (mandatory feature)

- **5 main presets:** Dark, Light, Normal/Classic, Gaming, Modern (recommended default).
- Editable sub-theme: primary/secondary accent, backgrounds (page/sidebar/card), border, focus-ring, gradient presets, radius, border weight, shadow, density; status colors tunable but never color-only.
- Typography: curated font stacks, size scale, line-height, density (comfortable/compact), weight preset; **Sinhala + English** glyph support (Noto Sans Sinhala / Iskoola Pota / Nirmala UI fallbacks); no mandatory external font.
- Visual effects: **Off / Subtle / Enhanced** (3D/tilt cards, gradients, glows, motion), Off = true low-resource mode (no WebGL/Three.js, no animated bg), lazy-load 3D only when needed, `prefers-reduced-motion` respected.
- Persistence across sign-in/reload (and cross-device where preference model supports); Super Admin default applies to new/unconfigured users without overwriting saved personal prefs.
- Invalid values must never allow CSS/script injection; color values validated.

---

## 10. Deployment & operations

> **Framing (Bible §1.0/§1.1, D-079…D-083):** one installation serves **one owner** on **one
> server**. The repository is a distribution mechanism, not a service. Not every line below is
> implemented — each is marked.

**Implemented / verified in the tree today:**

- **Stack (owner-approved, see DECISIONS):** React + TypeScript + Vite (build-time only), Go backend (single binary), SQLite (local disk, WAL, FK, embedded migrations), Caddy (auto HTTPS).
- **No production Node.js / Redis / PostgreSQL / Docker** on the 500 MB server.
- **Per-install secrets, fail closed:** the binary refuses to start without its two keys (`internal/config/decodeKey`); tokens are AES-256-GCM sealed (`internal/auth/seal.go`).
- **Secure first-time Super Admin bootstrap, no default password:** one-shot, `409` after any account exists.
- Caddy + systemd templates (`deploy/`), CI + release workflows with checksums (never executed here — `DISCREPANCY_REGISTER.md` D11).
- Domain change runbook with Caddy validation + rollback (`docs/DOMAIN_CHANGE.md`); panel URL/credential change in the Super Admin UI without a redeploy.

**Planned / NOT implemented (do not describe these as existing):**

- **One-line installer:** `scripts/install.sh --repo OWNER/REPO --domain --email --channel`; must be **installation-aware** — detect an existing installation and offer upgrade/reconfigure rather than reinstall; generate secrets **only when absent**; validate OS/arch/DNS/ports; least-privilege dirs; use the repository's established paths (`/opt/veylix`, `/etc/veylix`, `/var/lib/veylix`, `/var/backups/veylix`) unless renamed consistently. (`DISCREPANCY_REGISTER.md` D15)
- **Installation identity:** an id recorded at first startup, preserved across upgrades and carried in every backup (`DISCREPANCY_REGISTER.md` D17).
- **Restore provenance:** validate backup identity + integrity before restoring; refuse a foreign backup; distinguish "move the same installation" from "create a separate installation from another installation's data"; **no generic destructive `--force`** (`DISCREPANCY_REGISTER.md` D17, D-082).
- **Per-install CLI / upgrade tooling:** `status`, `health`, `logs`, `backup`, `restore`, `upgrade`, `rollback`, `domain set`, `uninstall --preserve-data` (`DISCREPANCY_REGISTER.md` D18).
- **Independent versions:** each installation upgrades only when **its owner** runs the upgrade; publishing or downloading a release must never deploy it (D-081).
- **Rehearsals:** clean-VM install, restore, and the 1 vCPU / 500 MB benchmark are **not performed** — see `docs/INSTALL.md` §F. The resource target is a requirement, **not a measured result**.


---

## 11. Acceptance tests (39, from Bible §9/§18)

All 39 tests in Bible sections 9 and 18 must pass before "done" — covering admin panel operations, multi-server resilience, role boundaries, quota concurrency, Free-offer isolation, seller ownership per assignment, QR correctness, migration safety, Theme Studio behavior, low-resource architecture, and install/HTTPS/domain-change operations.

---

## 12. Owner scope decisions (this version)

1. **Greenfield build** — no existing app to preserve.
2. **Implementation order:** Frontend first, then Backend (owner preference). **Superseded 2026-10-23 by D-010** — see item 7.
3. **Live testing:** after each milestone, owner tests on localhost; production install on a clean Linux VM will be tested in a later milestone.
4. **Scope control:** any flow/scope change requires owner approval, recorded in DECISIONS.md and reflected in this doc + plan.
5. **Out of scope for v1:** automated payment gateway, NFT/blockchain features, mobile apps (web only).
6. **Theme/UI (D-008, D-009):** 5 preset palettes + 40 sub-themes (8 per preset, AA-audited); ⌘K command palette implemented in-house (no third-party dependency).
7. **Order superseded (D-010):** backend + 3X-UI integration now precede further frontend breadth, following `new_update.md` §11 Stages A–H.
8. **Panel auth (D-011):** Bearer API token per integration; `/login` cookie flow not used.
9. **Panel reads/idempotency (D-012):** paged client API; `email` + deterministic `subId`; outbox reconciliation on ambiguous writes.
10. **Data layer (D-013):** CGO-free SQLite driver (`modernc.org/sqlite`), WAL + FK + bounded pool + embedded migrations.

---

## 13. Traceability map (2026-10-23)

Every section above is tracked in three places: reality (`GAP_ANALYSIS.md`), findings (`IMPROVEMENT_LEDGER.md`), and work (`IMPLEMENTATION_PLAN.md` stages).

| Requirements section | Reality (GAP_ANALYSIS) | Findings | Plan stage |
|---|---|---|---|
| §2.1–2.8 business rules | §2 | F-001, F-002, F-004 | B → E2–E6 |
| §3 server lifecycle + §3.5 contract | §3 | F-002, F-003, F-008 | E3 |
| §4 authorization matrix | §4 | F-007 | E2 |
| §5 data model + integrity | §5 | F-001, F-009, F-010 | E1, E4, E5 |
| §6 UX/pages | §6 | F-004, F-006 | E7 |
| §7 status machines | §7 | F-008 | E3 |
| §8 security & reliability | §8 | F-003, F-007 | E2, E3 |
| §9 Theme Studio | §9 | — | done |
| §10 deployment & operations | §10 | F-001 | E1, E7, Phase 6 |
| §11 acceptance tests (39) | — | F-005 | F |
| `new_update.md` §12 ledger | — | all | maintained continuously |

**Source files:** `docs/GAP_ANALYSIS.md` · `docs/IMPROVEMENT_LEDGER.md` · `docs/3XUI_API_CONTRACT.md` · `docs/IMPLEMENTATION_PLAN.md` · `docs/DECISIONS.md`.

---

## 14. Independent installation model (R-INSTALL-01 … 13)

**Authority:** Bible §1.0/§1.1 and `docs/DECISIONS.md` **D-079 … D-083** (2026-11-06). This section
is normative for deployment; it does not restate the business rules, it constrains how they are
delivered.

### 14.1 Product definition (authoritative)

Veylix is a **standalone, self-hosted, single-tenant VPN Business Management Platform** that
manages one owner's VPN business through a web application connected to one or more **externally
hosted** 3X-UI panels. It is **not** a 3X-UI replacement or fork, **not** a central multi-owner SaaS
control plane, and **not** a shared database for multiple owners.

### 14.2 Requirements

| ID | Requirement | Status |
|---|---|---|
| **R-INSTALL-01** | The same repository and release deploy on any number of servers, producing independent instances. | ✅ holds by construction |
| **R-INSTALL-02** | Each instance has its own database and data; no central database, no automatic cross-installation sync. | ✅ holds (local SQLite, no phone-home) |
| **R-INSTALL-03** | Each instance generates and stores its own encryption/session secrets; the binary fails closed without them. | ✅ implemented (fail-closed); generation is procedural |
| **R-INSTALL-04** | Fresh install initializes an **empty** database and never imports another installation's users, panels, VPNs, settings or history. | ✅ implemented (no seed path in `veylixd`) |
| **R-INSTALL-05** | The new owner performs secure first-time Super Admin setup; no default password; a second bootstrap is refused. | ✅ implemented + tested |
| **R-INSTALL-06** | Each instance has its own **installation identity**, created at first startup and preserved across upgrades. | 🔴 **not implemented** (D17) |
| **R-INSTALL-07** | Each instance has its own domain and HTTPS, configurable by its owner. | ✅ Caddy template + runbook |
| **R-INSTALL-08** | Each instance has its **own version** and upgrade lifecycle; nothing deploys into an installation except its owner's own action. | 🟡 possible today; tooling missing (D18) |
| **R-INSTALL-09** | The installer detects an existing installation and **refuses to overwrite** it; secrets are generated only when absent. | 🔴 **not implemented** — no installer (D15) |
| **R-INSTALL-10** | Backup carries installation provenance; restore validates identity + integrity and refuses a foreign backup; "move the same installation" and "create a separate installation from another's data" are distinct operations. | 🔴 **not implemented** (D17) |
| **R-INSTALL-11** | 3X-UI is external and owner-supplied; Veylix does not install or bundle it; panel records belong to their installation; owners needing full VPN isolation run their own panels. | ✅ holds (remote, SSRF-guarded API) |
| **R-INSTALL-12** | No multi-tenant database architecture and no `tenant_id` refactor. Isolation is by process, storage, database, keys and configuration. | ✅ architectural constraint |
| **R-INSTALL-13** | Deployment stays lightweight: no Docker, Redis, PostgreSQL or extra runtime services; target ≈ 1 vCPU / 500 MB, and that target is **not** claimed as benchmarked. | ✅ constraint held; benchmark not run |



