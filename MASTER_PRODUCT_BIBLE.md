# MASTER PRODUCT BIBLE — 3X-UI VPN Business Platform

**Version:** 2.0 — Product, UX, Architecture, Deployment and QA Source of Truth  
**Status:** Implementation specification for Cline  
**Primary goal:** Finish the existing app into a secure, modern, production-ready 3X-UI VPN sales/management platform that can run efficiently on a very small server (target: **1 vCPU / 500 MB RAM**).

> **Priority rule:** Existing confirmed business rules in this document must be preserved. If a requirement conflicts with existing project code, audit the code and database first, document the conflict, and implement a safe backward-compatible solution. Do not silently remove features or data.

## 0. Mandatory instructions for Cline — read before touching code

1. **Audit first.** Inspect the complete repository: framework, source tree, routes, database/schema, migrations, authentication, role enforcement, tests, environment files, deployment scripts and unfinished TODOs. Do not assume the existing stack and do not rewrite from scratch without a documented reason.
2. Create `docs/GAP_ANALYSIS.md` and `docs/IMPLEMENTATION_PLAN.md`. Map every requirement to `Already works`, `Partial`, `Missing`, `Bug`, or `Blocked`; cite real files/tests as evidence. Then implement in small, reviewable stages.
3. Keep this file at the repository root as `MASTER_PRODUCT_BIBLE.md`. It is the implementation source of truth. When a real business decision changes, update this file and record the reason in `docs/DECISIONS.md` rather than silently drifting.
4. Preserve working behavior, user data and current design conventions where useful. Use backward-compatible database migrations. Never reset production data or drop tables merely to make a test pass.
5. Do not report a feature as complete because a page/button exists. Verify UI, API authorization, database persistence, remote 3X-UI behavior, failure cases and tests.
6. Never put panel URLs with embedded credentials, API tokens, VPN UUIDs/passwords, signing secrets or session secrets into frontend code, browser storage, public API responses, Git, test snapshots or logs. Redact secrets in every error path.
7. Use the official 3X-UI API specification for each connected panel/version. Make integration behavior capability/version aware. Never fabricate endpoints, traffic fields, QR/config links, historical statistics or successful remote revocation.
8. Keep the app useful during partial outages. One slow/offline panel must not block the whole dashboard. Use bounded concurrency, request timeouts, pagination, background sync and accurate stale/unknown states.
9. **Small-server constraint is a design requirement, not a guess.** Target 1 vCPU and 500 MB total RAM. Measure memory/CPU on a clean low-resource test VM. Do not install a persistent Node.js production server, Redis, PostgreSQL, Docker or extra agents unless audit/evidence proves they are necessary and the owner approves the trade-off.
10. **Recommended greenfield/default architecture only if a migration is justified:** React + TypeScript + Vite for the frontend; Go for a compact backend binary; SQLite on the same local disk; Caddy for reverse proxy and automatically managed HTTPS. If the existing application is sound, keep it and apply equivalent resource and security principles rather than doing a needless rewrite.
11. Frontend assets should be built in CI/GitHub Actions and packaged with the backend release. Production server should download a verified prebuilt release. Node.js is a build-time tool only; it should not need to run on the 500 MB server.
12. The UI must include **Theme Studio** with Dark, Light, Normal/Classic, Gaming and Modern main themes, editable sub-theme colors, typography/text settings and visual/3D-effect controls. These settings must actually affect the whole app, persist after sign-in/reload, and include a Low-Resource/Reduced-Motion mode.
13. Installer/deployment scripts must be idempotent, validate inputs, back up before upgrading, detect port conflicts, avoid overwriting existing services/configuration without explicit confirmation, and fail with clear recovery instructions. Never blindly change firewall rules or lock the operator out of SSH.
14. Never put secrets in Git. Provide `.env.example` with placeholders only, a documented secret-generation step and restrictive ownership/permissions for production config/data directories.
15. Before finishing, run all feasible formatter, type-check, linter, unit, integration, migration, build and security checks. Add `docs/QA_REPORT.md` with exact commands, actual pass/fail results and any genuine blockers. Never fabricate test output.
16. If a requirement is unclear but a safe default is documented here, apply the default and record it. Do not stop implementation to ask about minor details. If genuinely blocked by external access/credentials, implement the complete safe interface and explain exactly what remains unverified.
17. **A VPN *is* a 3X-UI client (D-024 — non-negotiable).** The panel is the only authority on what a VPN is and what it can do. Never create, list, or claim a VPN that does not exist as a client on its panel. Load inbounds and clients **from** the panel; **every** panel client is a VPN. Capacity (`totalGB`), duration (`expiryTime`), IP limit (`limitIp`), hardware limit (`limitHwid`), usage and enable-state come from the panel — Veylix writes them once at create time, then mirrors them and never overrides them. Free vs Premium is a **Super-Admin visibility tag** stored locally, never written to the panel: an untagged VPN exists but is visible only to the Super Admin. Create on the panel **first**, confirm with `GET /clients/get/{email}`, then record locally — a failed create leaves no VPN row at all. A client missing from a panel read is `SERVER_UNAVAILABLE`, never `REVOKED`. If any other requirement conflicts with this, this rule wins.
18. **Tag lifecycle, FREE quotas and release (D-025 / D-027 / D-028).** A VPN's FREE/PREMIUM tag is editable only while the VPN is **unissued**; assigning or selling it locks the tag. Only FREE is publicly claimable and only PREMIUM is resellable. A client may hold at most **3** FREE VPNs per server at once and **5 claims per rolling 30 days** per server (Super Admin sets the number; releasing a VPN does **not** refund a monthly claim — tell the client before they claim). PREMIUM volume is bounded only by the reseller's quota. A quota-exhausted VPN may be **released**, which starts a **3-day grace period with undo**; after that Veylix deletes its own record and **never deletes the panel client**. Chat storage is a separate per-client budget the Super Admin sets, which the client can reclaim by deleting their own messages. A VPN's panel email is its owner's Veylix email (`-2`, `-3`, … when taken) and **assigning renames the panel client**. FREE uses `limitIp=2` and `limitHwid=2`; PREMIUM limits are chosen by the Super Admin.
19. **The VPN detail page is the product (D-026).** A Premium VPN shows QR + link, obtained→expiry dates, a **button-triggered live ping** (no speed test, no background pinging), usage history with 7d/30d/All-time ranges and user-selectable chart types, and separate CPU/RAM/network server charts. A Free VPN shows QR + link, ping, and a 7-day usage chart. Every chart names its source and window; an empty chart says "no data" instead of drawing a zero line; status colours come from theme tokens, never hard-coded.

### Definition of “done”

The product is not done until the confirmed business workflows work end-to-end, authorization is enforced server-side, existing data survives migrations, the five-theme Theme Studio works, the clean-server installer and domain-change operation are tested, HTTPS behavior is verified, backups can be restored, and the acceptance tests near the end of this document are reported truthfully.

## 1. Product summary

Build and improve a multi-server VPN sales and management platform integrated with multiple 3X-UI panels. The application has three roles:

- **Super Admin:** platform owner and ultimate controller.
- **Admin:** reseller who purchases/receives VPN capacity from the Super Admin, then sells/assigns VPN access to clients under a quota.
- **Client:** signs up, selects public Free VPN offers, or contacts the relevant seller to buy Premium VPN access.

The platform must support multiple 3X-UI servers (initially at least 20–30, with room to grow), role-based server access, client assignments, per-client limits, payment instructions, internal support chat, external contact methods, QR/link import, and reliable usage/history pages.

## 2. Confirmed business rules

### 2.1 Super Admin

Super Admin can:

- Add, view, edit, verify, temporarily disable, and remove 3X-UI server integrations using panel URL and API credentials.
- See all registered servers and their connection/health states in one dashboard, including inbounds, clients, traffic information, server details, and supported resource metrics.
- Create, edit, suspend, and remove Admin accounts.
- Assign one or more servers to each Admin and control the Admin's permissions.
- Create/manage VPN accounts on the connected 3X-UI panels; configure the inbound, protocol and limits supported by that panel.
- Decide whether a VPN offer is **Free** or **Premium**.
- Define the Free VPN offer/pool, allowed server/inbound, account-creation rules, traffic limit, expiry/duration, device limit (if supported), and other permitted limits.
- Set the wholesale price/terms for VPN access or quota sold to Admins.
- Directly assign/sell a Premium VPN to a Client and set the price for that direct sale.
- Configure global payment methods and contact defaults.
- View all assignments, users, sales/payment records, usage/history, audit logs, server incidents, and notifications.
- Receive notifications for server connection failures, credential changes/reverification failures, and affected Premium VPN assignments.

Super Admin remains responsible for authorizing VPN offers, protocols, inbound/server allocation and the limits. Clients must not be able to create arbitrary server configurations or bypass those limits.

### 2.2 Admin

Admin can:

- See the Clients relevant to the platform's Admin-facing customer-management view, but only see/manage VPNs that belong to servers assigned to that Admin. Do not expose another Admin's restricted VPN credentials or server data.
- See only the servers/inbounds/available VPN offers permitted by the Super Admin's server assignment and permissions.
- Assign available VPN access to Clients only within the Admin's active quota and only from permitted servers/offers.
- Set and update a retail selling price for VPN access purchased/allocated from the Super Admin, subject to any platform policy the Super Admin configures.
- Configure their own support/contact details: phone number, WhatsApp, Telegram, email, and optional internal-chat availability.
- Configure permitted payment instructions/details for their sales. They may only change their own payment details, not the Super Admin's global payment details.
- View their clients' VPN assignments, sales, payment status, and relevant usage history for support purposes.
- Communicate with their assigned Clients through the app's internal chat.

Quota rule: Admin quota is the maximum number of **active Premium VPN assignments** the Admin can maintain for Clients at one time. When an assignment expires or is revoked and is no longer active, its slot may be released. A Client receiving multiple Premium VPNs consumes one slot per active assignment unless the existing business model explicitly defines a different unit. Free public offers do not consume an Admin's Premium quota unless the Super Admin explicitly configures otherwise.

### 2.3 Client

A Client can:

- Register/sign in with Google, or with email and password. Email/password registration must include appropriate validation, secure password storage, and email verification where supported by the existing auth system.
- See public Free VPN offers approved by the Super Admin.
- Select a Free VPN offer and receive an individual 3X-UI client/account created under that pre-authorized offer's rules. Users may not change server settings or increase their own limits.
- Receive a Premium VPN assignment from an Admin or the Super Admin after the sale/assignment flow is completed.
- Buy more than one Premium VPN assignment if the seller has capacity and quota.
- Open each VPN's own detail page to view the information relevant to that VPN: connection state, assigned server/inbound details suitable for display, the Client's own traffic usage, upload/download totals, used/remaining quota, expiry, applicable limits, supported daily usage/history, QR code, import/subscription link, and setup guidance.
- Copy the link, show the QR code, and follow platform-specific setup steps.
- See the responsible seller for each Premium VPN and contact them through internal chat or the seller's configured phone/WhatsApp/Telegram/email. If the VPN was sold/assigned directly by the Super Admin, show the Super Admin's configured contacts and chat.
- View purchase/assignment records and historical usage even if the remote server later becomes unavailable or is removed from the active server list.
- Update their own permitted profile fields and UI preferences without changing their role or permissions.

### 2.4 Public Free VPN offer versus individual Free account

A Free VPN is a public offer, not a single shared credential used by every Client. The Super Admin creates/approves the offer and configures its server/inbound and limits. When a Client selects it, the backend provisions a separate individual 3X-UI client credential for that Client if the selected panel/version supports it.

- Every Client sees the same eligible public Free offer.
- Each Client gets their own QR code/link and own usage/account limits.
- A Client cannot see another Client's credentials, identifying details, or personal usage history.
- It is acceptable to display shared offer details and public server-wide metrics to all Free users only when the Super Admin has enabled that disclosure.
- Free usage ends when the configured traffic/time/other limit is exhausted. Explain the reason/status clearly in the UI.
- When a Free offer/server is removed, remove it from clients' active Free catalog/active access view. Preserve internal audit records needed to investigate incidents; do not leave it looking active to Clients.

### 2.5 Premium VPN pricing and payment process

There are two sales paths:

1. **Reseller sale:** Super Admin defines a wholesale price/allocation for Admins. An Admin obtains VPN capacity/access from the Super Admin, then chooses a retail selling price for Clients.
2. **Direct sale:** Super Admin assigns/sells directly to a Client and sets the Client-facing price.

The Client should see price, available payment methods, payment details/instructions and the correct seller contact while attempting to buy a Premium VPN. The Client can contact that seller before/during the purchase.

Payment support should allow configurable manual-payment options, including bank transfer and eZ Cash/mobile-money details where applicable. Store provider/account holder, bank or payment method name, account/phone details, instructions, and an optional QR/image only when supported by the existing stack. Mask sensitive payment details when appropriate. Do not hard-code one payment provider into the UI.

Default flow until an automated gateway is explicitly requested:

1. Client chooses a Premium offer or requests a Premium VPN.
2. App shows the seller, price, payment methods/details, and instructions.
3. Client contacts the seller through internal chat and/or configured external contact details; seller confirms payment according to the business process.
4. Authorized seller/Admin/Super Admin records payment status and completes the assignment, or the app uses an existing verified payment integration if already implemented.
5. App provisions/assigns the VPN, applies limits, records the price/seller/payment relationship, and exposes QR/link only when the VPN is confirmed active.

Do not mark a manual payment as verified just because the Client opens the payment page or uploads an unverified message. Payment confirmation must be an auditable action.

### 2.6 Seller/contact ownership and internal chat

- Each Premium VPN assignment stores its seller/owner reference.
- An Admin-issued VPN displays that Admin's details and offers Client↔that-Admin chat.
- A Super Admin-issued VPN displays Super Admin contact details and offers Client↔Super-Admin chat.
- For a Client who receives VPNs from different Admins, keep the seller relationship **per VPN assignment**. Do not force one single primary Admin to own all the Client's VPNs.
- Support phone, WhatsApp, Telegram, email and internal chat as profile-configured options. Hide contact methods not supplied by that user.
- Super Admin/Admin profiles may update their own contacts. Client sees only the relevant seller's current public contact details.
- Chat must enforce participant access server-side; a Client must not read a conversation with another Client or an unrelated Admin.

### 2.7 Client VPN details and statistics

Each VPN must have a separate details view so a Client with 3–4 VPNs can select one VPN and see only the selected assignment's relevant details.

Show separate sections for:

- **VPN/account status:** active, expired, traffic-depleted, suspended, server unavailable, pending verification, or revoked.
- **Usage:** upload, download, total traffic, remaining quota, applicable reset period, and a timeline/daily history where underlying data is available.
- **Limits:** traffic cap, expiry, device/connection limit if the protocol/panel supports it, and remaining time/data.
- **Connection details:** protocol, server address/port and other safe fields needed by the client; QR code, copyable connection link and/or subscription link where supported.
- **Server metrics:** server CPU, RAM, network status/throughput and resource history when the connected 3X-UI server/API or an approved monitoring source actually supplies the data.
- **History:** assignment, seller, price/purchase date where relevant, usage periods, resets/changes, incidents and notifications.

Never invent unavailable historical data. If the source exposes only current counters rather than historical daily samples, store time-stamped snapshots in the platform to build history going forward and label older periods as unavailable. Clearly distinguish **Client VPN traffic** from **whole-server network traffic**; they are not interchangeable.

### 2.8 QR codes, links and setup guide

- Use the connection data returned/generated for the correct 3X-UI client and protocol. Do not derive a fake link from a panel URL.
- Each individual Free account and each Premium assignment must show its own correct QR/link when active.
- QR/link should be copyable and easy to access on desktop and mobile.
- Offer concise platform-specific guides for Android, Windows and macOS, including recommended client apps and official download links. Store these recommendations/configuration guides in editable application content so they can be updated later.
- Each guide explains: install the client app, open/import from QR or link, grant permissions if needed, connect, and how to troubleshoot an expired/invalid link.
- Recommendations must be protocol-compatible. Do not show an Android-only or protocol-incompatible client for a different platform/protocol.
- Do not expose a working-looking QR/link for an inactive/revoked VPN.

## 3. Server integration and lifecycle

### 3.1 Add or update a server

Save the panel URL and required credentials securely on the backend. Verify connectivity/authentication and required API capabilities before marking the integration `ACTIVE`. Capture panel/software version where available. Validate URL format, TLS/HTTPS, timeouts and response structure.

### 3.2 Monitor and detect changed credentials

A scheduled health check and API error handling should detect connection/authentication failures. If a previously working integration cannot authenticate or its URL/credentials are changed:

- Mark the server `REAUTH_REQUIRED` or `CONNECTION_UNAVAILABLE`; do not silently treat it as healthy.
- Immediately prevent new assignments/provisioning against it until verification succeeds.
- Hide/deactivate affected active connection material in the App UI as appropriate; show a clear status rather than silently deleting Premium purchase history.
- Trigger notifications to the Super Admin and each responsible Admin whose assignments are affected.
- Record the detection time, last successful sync, safe error category and affected assignment counts. Never log secrets.
- Retry with bounded backoff. Do not run tight, unlimited retry loops.
- When credentials are replaced, verify the panel before restoring availability and resync data cautiously.

An integration credential change does not itself prove that the remote Xray client has been disabled. Never claim remote revocation unless the corresponding API action was accepted/verified.

### 3.3 Server remove / offline behavior

**Free:** Remove the offer/access from the Client's active Free list when the offer/server is removed or no longer usable. Do not show it as active. Internal audit data may remain.

**Premium:** Never hard-delete the Client's purchase/assignment/history record just because a server has gone away. Keep the VPN name/plan, seller, purchase date/price, original server/inbound metadata that can safely be retained, saved usage snapshots and incident history. Change its state to `SERVER_UNAVAILABLE`, `REAUTH_REQUIRED`, `REVOCATION_PENDING`, or another accurate state.

- Notify the Super Admin and the seller Admin.
- Explain the issue to the Client on the VPN details page and show the support contact/chat.
- Do not display a QR/link as an active connection option after the app knows the VPN is unavailable or revoked. Preserve a non-active historical record. If product owner later wants the original configuration downloadable as a historical artifact, this must be an explicit separately approved policy because it may contain working credentials.
- Do not automatically delete records from the remote server unless that action is explicitly requested and properly confirmed. Removing an app integration and revoking a remote VPN account are different operations.

### 3.4 Sync strategy

Support many servers without making every page wait for all servers. Use per-server sync jobs, timeout limits, pagination where available, last-sync timestamps, retries, partial-failure handling, and background refresh. Show stale/unknown values as stale rather than presenting old values as live.

## 4. Roles and authorization matrix

| Action | Super Admin | Admin | Client |
|---|---|---|---|
| Manage all panel integrations | Yes | No | No |
| See global server/inbound list | Yes | Assigned servers only | Only relevant safe details for own VPN/approved public Free offers |
| Create/approve VPN offers and base limits | Yes | No; use allowed offers | No |
| Provision a Free account | Governs offer/rules; backend may provision per Client | No unless explicitly delegated | Requests/selects offer; cannot set own limits |
| Assign Premium VPN | Yes, including direct-to-Client | Only within assigned server and quota | No |
| Set direct-to-Client Super Admin price | Yes | No | No |
| Set retail price for Admin-owned sales | May set policy/constraints | Yes, for their resale offer | No |
| Create/edit/suspend Admins | Yes | No | No |
| Edit own profile/contact preferences | Yes | Yes | Yes, permitted fields only |
| See own VPN usage/history | Yes | Relevant clients/assignments within permission | Yes |
| See other Clients' credentials/history | Only if needed for platform operation and audited | No, unless explicitly necessary and authorized | No |
| See server-wide CPU/RAM/network | Yes | Assigned servers | Per confirmed product choice, show on selected VPN page but label as server-wide data |
| Configure own payment details | Yes/global | Own account only, within policy | No |
| Global audit/system settings | Yes | No | No |

Every authorization rule must be enforced by the backend API/service layer, not just by hiding frontend controls.

## 5. Data model concepts (adapt to the existing schema)

Do not blindly create duplicate tables if equivalent structures already exist. Ensure the existing schema can represent these concepts and relationships:

- `users`: identity, role, auth provider, status, profile fields.
- `server_integrations`: panel URL, encrypted credentials/token, panel version, integration status, last sync, last health check, created/updated by.
- `inbounds`: remote inbound identifiers and safe metadata, linked to `server_integrations`.
- `admins`: reseller settings/relationship if roles are not already represented in `users`.
- `admin_server_assignments`: Admin ↔ allowed server mappings.
- `vpn_offers` or equivalent: offer type (Free/Premium), server/inbound template, allowed limits, visibility, availability and price policy.
- `vpn_instances` / `remote_clients`: local mapping to remote 3X-UI client identity, protocol, server/inbound, actual remote state, encrypted secrets only when required, traffic counters and remote identifiers.
- `vpn_assignments`: client, VPN instance/offer, seller/admin, status, assigned/activated/expiry times and limit snapshot.
- `quota_ledger`: auditable quota changes and active-count calculation, avoiding race conditions/double consumption.
- `prices` / `sales`: wholesale price, retail price, currency, seller and price snapshot at sale time.
- `payment_methods`: owner (Super Admin or Admin), type, public instructions/details, active state.
- `payment_records`: amount/currency, method, reference, status, timestamps, who confirmed it and audit history.
- `usage_snapshots`: time-stamped per-client usage and separate server metrics for historical charts.
- `notifications`: recipient, event, status, read time and related server/assignment.
- `chat_conversations` and `chat_messages`: participants, assignment/context and access checks.
- `audit_logs`: actor, action, target, timestamp, result and non-secret context.

Keep a stable local ID for each remote object. Do not use a mutable remote name/email as the only relational key. Prevent duplicate provisioning on retries with idempotency/unique keys.

## 6. UX/pages required

### Super Admin UI

- Overview/dashboard: healthy, disconnected, reauth-required servers; active clients; Free/Premium assignments; usage and incidents.
- Server integrations: add/edit/test/disable/remove, per-server status, last successful sync, inbounds and metrics.
- VPN offers/instances: create and configure on allowed 3X-UI server/inbound, limits, Free/Premium status, pricing, active state and assignments.
- Admin management: create/edit/suspend, assigned servers, quota, permissions, wholesale allocation/pricing and reseller status.
- Clients: search/view profile, assignments, sales/payment status and supported history.
- Pricing and payment settings: global methods/details, direct-sale prices and contact details.
- Server incident centre/notifications, audit logs and support chat.

### Admin UI

- Overview: quota capacity/used slots, clients, assignments, pending payments, sales and server health for assigned servers.
- Assigned servers/inbounds and permitted VPN offers.
- Client management and Premium assignment flow.
- Retail price and own payment/contact settings.
- Client support chats, incident notifications and relevant usage details.

### Client UI

- Dashboard with Free offers and the Client's separate VPN cards.
- VPN detail page containing status, limits, usage/history, safe server details, QR, link and platform guide.
- Premium purchase/contact flow with seller information and payment instructions.
- Purchase/history page that retains Premium records on server failure/removal.
- Notifications and internal support chat.
- Profile/security settings and theme/preferences.

## 7. Status model

Use explicit state transitions rather than relying on one boolean `active` field. Adapt names to the existing code if needed:

- Server integration: `PENDING_VERIFICATION`, `ACTIVE`, `REAUTH_REQUIRED`, `CONNECTION_UNAVAILABLE`, `DISABLED`, `REMOVED`.
- VPN account/assignment: `PENDING`, `ACTIVE`, `TRAFFIC_DEPLETED`, `EXPIRED`, `SUSPENDED`, `SERVER_UNAVAILABLE`, `REVOCATION_PENDING`, `REVOKED`, `REMOVED_FROM_CATALOG`.
- Payment: `PENDING`, `AWAITING_CONFIRMATION`, `CONFIRMED`, `REJECTED`, `CANCELLED`, `REFUND_PENDING`, `REFUNDED` if refund support is implemented.

Do not map a temporary API outage directly to `REVOKED`; these mean different things. Keep purchase history independently of active access status.

## 8. Security and reliability requirements

- Backend authorization for every server/admin/client resource.
- Encrypt integration credentials at rest using a managed environment secret/key; never commit production secrets.
- Use HTTPS for panel/API access where possible; validate TLS and do not disable certificate checks globally as a shortcut.
- Prevent server-side request forgery (SSRF) when a Super Admin enters a panel URL: validate schemes and destinations, block loopback/private/link-local/metadata ranges unless explicitly allowed by a secure deployment policy, and prevent redirects to prohibited destinations.
- Rate-limit sign-in, Free provisioning, chat, payment actions and server API operations.
- Verify Google login tokens server-side; protect email/password accounts with modern password hashing and secure sessions.
- Validate all URL, protocol, limit, currency, price and remote-ID fields.
- Add CSRF/origin protections where appropriate for the chosen framework.
- Use transactions/locks for quota consumption and provisioning state. Avoid two simultaneous requests exceeding the Admin quota or creating duplicate remote clients.
- Use audit logs for server credential changes, account provisioning/revocation, quota changes, prices, payment verification, and role/permission edits.
- Mask secrets and personal payment details in logs and error responses.
- Add backup/restore documentation and database migrations.

## 9. Acceptance tests — must pass before marking done

1. Super Admin can add a panel, pass a connectivity/API check, and see its inbounds/clients; bad credentials produce a clear failed state.
2. With 20+ integrations, the dashboard remains usable when some servers are slow or offline; one server failure does not break the rest.
3. An Admin sees and operates only servers/inbounds assigned by Super Admin. Direct API calls cannot bypass this boundary.
4. An Admin cannot provision Premium VPN assignments beyond their active quota, including concurrent requests. Expiry/revocation releases a slot according to the quota rule.
5. Super Admin can define a public Free offer and its limits. Two Clients selecting it receive two separate remote client credentials and independent limits/usage, not the same credential.
6. A Free VPN offer removed by Super Admin no longer appears as active/available to Clients, though internal audit data can be retained.
7. A Premium VPN assigned by an Admin shows that Admin as seller/support contact; a direct Super Admin sale shows Super Admin contacts.
8. One Client can hold Premium VPN assignments from more than one Admin. Each VPN shows its own seller and chat route.
9. The Client sees the correct QR/link for the selected active VPN only. Links/QR are not mixed between VPNs, clients, servers or inbounds.
10. Client cannot see another Client's QR, UUID/password, traffic history, payment record or chat.
11. A Premium purchase record, price/seller snapshot and saved usage history remain visible when the remote server is removed or becomes unavailable; it is clearly marked unavailable and notifications go to the Client, the seller Admin (if any) and Super Admin.
12. When credentials change and the health check fails, new provisioning stops; affected services display an accurate state; the system does not falsely claim remote revocation.
13. When possible, a remote revocation is confirmed by 3X-UI before the App displays `REVOKED`. When the server is unreachable, use `REVOCATION_PENDING` and retry safely.
14. Daily usage charts only display actual sampled history. If old historical records are unavailable, the UI says so instead of inventing history.
15. Client setup guides show protocol-compatible steps for Android, Windows and macOS and are editable/maintainable.
16. Payment methods/details can be configured by the permitted owner. Payment confirmation is an authenticated, audited action; the UI does not auto-confirm manual payments.
17. Internal chat endpoints reject messages/read requests from non-participants.
18. Existing data and features continue to work after migrations; tests cover upgrade from the current database state.

## 10. Decisions/assumptions to confirm or retain as defaults

These are policy refinements, not blockers for starting the audit and implementation:

1. **Premium access when a server disappears:** keep the Premium historical purchase/assignment record, seller/price, metadata and usage history visible; stop showing QR/link as an active connection method when unavailable/revoked. Recommended default. If a link/config is stored for a historical purpose, do not present it as working; treat it as sensitive.
2. **Replacement/refund when Premium becomes unavailable:** no automatic refund or replacement is assumed yet. Keep a clear support flow and allow Admin/Super Admin to resolve the case. If refunds, compensation, or automatic replacement are wanted, add an explicit policy and audit flow.
3. **Manual payment default:** bank/eZ Cash or other enabled payment instructions are shown, and an authorized seller marks payment confirmed. No automated gateway is required unless one already exists or is later requested.
4. **Resource statistics:** show per-VPN/client usage separately from whole-server CPU/RAM/network metrics. Because server-wide metrics may reveal information about other users, the Super Admin should be able to enable/disable server-metric visibility to Clients; when enabled, label the values as server-wide.
5. **Free account provisioning:** the Super Admin owns the offer and limit rules. Client selection can trigger backend provisioning of a distinct account under those rules; this does not grant the Client permission to create arbitrary VPN accounts.

## 11. Official technical references

Use the exact docs for the installed 3X-UI version during implementation:

- API overview and authentication (including Bearer API-token handling): https://github.com/MHSanaei/3x-ui/blob/main/docs/content/docs/en/reference/api/index.mdx
- Inbounds/client traffic and related operations: https://github.com/MHSanaei/3x-ui/blob/main/docs/content/docs/en/reference/api/inbounds.mdx
- Client API reference: https://github.com/MHSanaei/3x-ui/blob/main/docs/content/docs/en/reference/api/clients.mdx
- Release versions: https://github.com/MHSanaei/3x-ui/releases

The 3X-UI API documentation notes that API tokens act as full-admin credentials and must be stored securely. Available traffic/status fields depend on the installed version and underlying panel data; the platform must verify capabilities and preserve accurate status instead of inventing missing metrics.

## 12. Modern frontend experience and design system

### 12.1 Design objective

Build a polished, responsive administration product with the clarity and operational density expected from a modern 3X-UI management panel, while using this platform's own coherent visual identity. The interface should feel premium and intentional—not like a raw table-based CRUD template. Visual polish must never obscure server state, limits, payment status, warnings or security-relevant actions.

### 12.2 Shared layout and components

- Use a consistent app shell: collapsible sidebar, compact top bar, breadcrumbs where helpful, account/profile menu, notifications and a role-aware navigation tree.
- Keep Super Admin, Admin and Client navigation focused on what that role can actually do. Hide inappropriate controls in the UI, but enforce every permission again in the backend.
- Build reusable components for status badges, metric cards, charts, searchable/filterable tables, pagination, tabs, forms, input validation, dialogs, destructive-action confirmations, empty states, loading skeletons, retry states, toast feedback and help/tooltips.
- Every important async page must distinguish loading, success, empty, stale, partially failed and failed states. Show last-sync timestamps when displaying remote panel data.
- Tables should support search, useful filters, sorting where safe, pagination and small-screen handling. Avoid loading every client/record in one request.
- Use charts only when the underlying data is real. Label client VPN traffic separately from whole-server traffic. Show “history not available yet” instead of inventing values.
- Add clear confirmation for remote-impact actions such as revoke/delete, server removal, payment confirmation, role changes and quota changes. Show the consequences before commit.
- Support desktop, tablet and mobile widths. On mobile, cards/forms must remain usable and wide tables should use a deliberate responsive presentation rather than causing page-wide horizontal overflow.
- Use meaningful icons consistently; do not use emoji as a substitute for the core icon system. Keep contrast, spacing, focus rings, disabled states and error messages consistent.
- Keep the UI fast: code-split large pages, lazy-load chart/3D modules, debounce search, cancel obsolete requests and cache only safe data. Avoid excessive animation, oversized background assets, unbounded lists and unnecessary dependencies.

### 12.3 Theme Studio — mandatory user-facing feature

Add a first-class **Settings → Appearance / Theme Studio** screen. It must work as a real design configuration tool, not a mockup. Users should see a live preview before saving and be able to reset to a preset.

**Main theme presets (all five required):**

1. **Dark** — dark surfaces, comfortable contrast and a restrained accent color.
2. **Light** — light surfaces, readable text and clear borders.
3. **Normal / Classic** — neutral, familiar admin-panel appearance without strong visual effects.
4. **Gaming** — configurable neon/accent highlights, glowing edges and optional animated visual details; effects remain tasteful and must not make tables/forms hard to read.
5. **Modern** — recommended default: clean spacing, refined cards, subtle gradients, crisp typography and minimal visual noise.

**Editable sub-theme settings:**

- Primary/accent color and secondary accent color.
- Page background, sidebar/background surface, card/surface color, border color and focus-ring color.
- Optional gradient presets, surface transparency within readable limits, card radius, border weight, shadow intensity and visual density.
- Status colors (success/warning/error/info) may be visually tuned within accessible limits, but statuses must never be communicated by color alone; keep text/icon labels.
- Preset palettes plus a custom color picker and reset action. Validate color values; never accept arbitrary CSS/HTML from a theme setting.

**Typography and text appearance:**

- Let users choose a small curated font stack/preset, font size scale, line-height and interface density (comfortable/compact), plus an optional stronger text-weight preset.
- Support Sinhala and English text without broken glyphs or clipped labels. Use sensible local/system fallbacks, for example a stack that can fall back to Noto Sans Sinhala, Iskoola Pota, Nirmala UI and sans-serif. Avoid a mandatory external font request for normal operation; if a font is bundled, verify its licence and keep the asset size reasonable.
- Text size changes must preserve layout, form controls and mobile usability. Do not allow settings that reduce essential text below an accessible readable size.
- Theme color/font preferences must not change business data, role permissions, prices or security states.

**3D graphics and visual-effect controls:**

- Provide clear user controls for `Off`, `Subtle` and `Enhanced` visual effects, including optional 3D/tilt cards, depth/shadow, background shapes, gradients, glows and motion where appropriate to the design.
- `Off` must be a real low-resource mode with no unnecessary WebGL/Three.js canvas, no continuously animated background and no expensive parallax loop.
- `Subtle` is the safe default. `Enhanced` may use a lightweight CSS/canvas effect and, only when genuinely valuable, a lazy-loaded 3D library. Do not load a large 3D bundle on every route.
- Provide `Reduce motion` and `Low-resource mode` toggles; respect `prefers-reduced-motion`. Disable tilt/parallax for keyboard/touch cases where it harms usability.
- All effects are decorative. They must never cover controls, reduce contrast, conceal warnings or delay primary actions. Animations should be short, interruptible and absent from critical data refresh loops.

**Preference ownership and persistence:**

- Each signed-in user can save personal appearance preferences. Persist them securely on the backend or in the existing supported preference model so they survive another browser/session/device; use local preference state only as a fast preview/cache, not as the sole source if cross-device persistence is promised.
- Super Admin can set the platform default theme and, if needed, a brand-wide preset. The safe default is that users can still choose their own theme; if a future policy allows theme locking, make it explicit and visible.
- Anonymous/login screens use the configured platform default and must stay readable.
- The same theme tokens must apply consistently to login, dashboards, forms, tables, charts, dialog windows, chat, notifications, setup guides and error pages.
- Theme changes apply immediately without a full page reload. Save, cancel, preview and reset must each have working behavior and visible feedback.
- Store a versioned, validated theme-preference structure. If a new app release changes tokens, gracefully migrate old preferences and fall back to the closest built-in preset rather than breaking the UI.

### 12.4 Technical implementation of themes

- Create a single design-token layer using CSS custom properties (for example `--color-bg`, `--color-surface`, `--color-text`, `--color-primary`, `--radius-card`, `--shadow-card`, `--font-family-ui` and `--density-scale`). Components must consume tokens instead of hard-coded colors scattered through source files.
- Represent a theme as validated data/preset configuration; map it to known CSS variables. Do not inject saved arbitrary styles or script fragments.
- Theme mode, palette and typography settings should be independent dimensions so switching Dark → Gaming or Light → Modern does not destroy the user's chosen accent/font settings unless the user explicitly selects “reset to preset”.
- Charts must update to the current theme and retain clear series contrast. Do not use red/green alone to express status.
- Test every core page in all five main themes, especially Theme Studio, data tables, modal dialogs, charts, payment instructions and mobile layouts.
- Target WCAG 2.2 AA contrast and keyboard accessibility for essential workflows. Add visible focus indicators, semantic labels and accessible dialog behavior.

## 13. Recommended technical architecture for the 1 CPU / 500 MB server

### 13.1 Existing-codebase rule

The stack below is the recommended target **when starting fresh or when an evidence-based refactor is needed**. Cline must first audit the actual repository. Retaining a working stack is preferred over a risky, unnecessary rewrite. If a different current stack is retained, document its measured resource footprint and equivalent deployment approach.

### 13.2 Preferred low-footprint deployment

- **Frontend:** React + TypeScript + Vite; production build output is static files. Use the latest mutually compatible stable versions at implementation time and commit the package lockfile. Use a lightweight component system and CSS design tokens; avoid an oversized UI framework bundle.
- **Frontend build:** run in GitHub Actions/CI, not on the small production server. Build, type-check, test and bundle the UI, then embed/copy the static assets into the backend release.
- **Backend:** Go HTTP API compiled to a single Linux binary. It serves `/api/*` and the built SPA, including client-side route fallback. Prefer a maintained SQLite driver with reproducible builds and documented CGO requirements; do not choose a driver without testing its target architecture and binary.
- **Database:** SQLite database on a local persistent disk, with migrations, foreign keys enabled, WAL mode where appropriate, bounded connection usage and tested backup/restore. Do not store the production database on a network filesystem. SQLite is the default only while this remains a single-instance, modest-write workload; document when a client/server database becomes necessary.
- **HTTPS/reverse proxy:** Caddy manages public TLS and proxies to the backend on `127.0.0.1` only. The Go backend must not be directly exposed to the public Internet.
- **Process manager:** systemd service running as a dedicated unprivileged account. Add restart policy, startup ordering, controlled shutdown, log rotation/journald use and conservative resource settings where supported.
- **No unnecessary production services:** no persistent Node.js server, Redis, separate queue daemon, PostgreSQL, MySQL, container engine or second web server by default. Use a bounded in-process scheduler and a database-backed job/lease model if background tasks are needed. Do not add a dependency without documenting its resource and operational cost.

### 13.3 Request and job behavior

- Serve fast dashboard queries from the local database/cache, not by waiting synchronously for every remote 3X-UI panel.
- Use bounded per-panel timeouts, small global concurrency (initially around 3 concurrent external-panel calls; tune using benchmarks), bounded retry/backoff and random jitter to avoid synchronized polling.
- Keep provisioning, health checks, usage snapshots and notification work separate from the page request where possible. Persist job state so a process restart does not silently lose a pending operation.
- Use idempotency keys and stable local IDs to prevent duplicate client creation after timeout/retry.
- Limit page size and payload size; use pagination. Avoid scanning all rows or calling all 30 panels for every rendered page.
- Use the correct remote API response schema and a version-aware adapter. Save safe diagnostic metadata and timestamps, not secrets or unnecessary raw API bodies.
- If SQLite is retained, keep write transactions short, enable foreign keys, use safe busy timeout/WAL configuration, migrate schema transactionally, and test multiple concurrent operations. Never assume WAL permits multiple simultaneous writers.

### 13.4 Resource budgets and performance guardrails

The 500 MB requirement applies to the **whole server**, not just the application process. Real usage depends on the OS image, providers, traffic and connected-panel response sizes, so Cline must measure instead of promising an unverified number.

- Target Ubuntu Server LTS or supported Debian stable releases after validating package availability. Document exact versions actually tested.
- Keep the backend at one CPU by default (`GOMAXPROCS=1` where Go is used); set a measured Go memory limit and connection/request bounds rather than guessing. Do not use a CPU-intensive dashboard rendering loop.
- Avoid starting many background goroutines/workers proportional to the number of servers. Keep 20–30 integrations manageable with bounded sync batches.
- Avoid retaining large raw server responses or image payloads in memory. Do not poll remote APIs from every user browser.
- Optimize production bundle size; lazy-load less-used charts, QR tools and optional 3D effects. Avoid loading the 3D module if visual effects are `Off`.
- Provide a `LOW_RESOURCE_MODE` setting and ensure all 3D/background effects can be disabled.
- Add a documented benchmark for idle, normal use and a controlled sync run against up to 30 configured integrations. Capture peak RSS/CPU and identify exactly what was measured.
- If the app cannot safely fit 500 MB for the expected workload, report measured evidence and the smallest safe resource adjustment; never hide OOMs or disable security controls to meet a number.

## 14. Git-based release and one-command server installation

### 14.1 Deployment experience required

The repository must provide a tested Linux installer so an operator can install or upgrade the application from a GitHub repository using one terminal line after the repository's CI release has been built. The installer should create the required app directories, database, system user/service, web app and Caddy HTTPS configuration. Because the selected production design uses a Go binary plus static frontend assets, **Node.js is not required on the production server** and should not be installed there by default.

The one-line installation depends on DNS and network prerequisites. It must not claim HTTPS succeeded until certificate issuance and an external/local HTTPS smoke test succeed.

### 14.2 Required repository assets

Cline must create or adapt these files to match the repository structure:

- `scripts/install.sh` — first-time installer with interactive mode and flags.
- `scripts/upgrade.sh` or `vpn-platformctl upgrade` — safe upgrade path with automatic pre-upgrade database backup and rollback to the previous binary if safe.
- `scripts/uninstall.sh` — optional clean removal that explicitly preserves or exports data by default; no silent database deletion.
- `scripts/backup.sh` and `scripts/restore.sh` or equivalent CLI commands.
- `deploy/Caddyfile.template` — domain/reverse-proxy template with safe headers and validated config reload.
- `deploy/vpn-platform.service` — systemd service template with least privilege and controlled restart.
- `.github/workflows/ci.yml` — format/lint/type-check/test/build workflow.
- `.github/workflows/release.yml` — reproducible release artifact workflow for supported Linux architectures (at minimum `linux/amd64`, plus `linux/arm64` if verified).
- `checksums.txt` and release metadata for generated binaries/assets; installer verifies the downloaded checksum before installing. Add artifact signing/SBOM where practical and document verification limitations honestly.
- `.env.example` — placeholders only; no production values.
- `docs/INSTALL.md`, `docs/UPGRADE.md`, `docs/BACKUP_RESTORE.md`, `docs/DOMAIN_CHANGE.md`, `docs/TROUBLESHOOTING.md` and `docs/QA_REPORT.md`.

The release workflow may publish a rolling `edge` build after successful pushes to the default branch for test installs, plus stable versioned releases for production. The installer must fail clearly if no compatible successful release exists. Never download an arbitrary binary from an untrusted URL or silently skip checksum verification.

### 14.3 One-line install command pattern

Document and test this exact command shape, replacing the placeholders with the actual public repository owner/name and the user's domain/email:

```bash
curl -fsSL "https://raw.githubusercontent.com/OWNER/REPO/main/scripts/install.sh" -o /tmp/3x-vpn-install.sh && sudo bash /tmp/3x-vpn-install.sh --repo "OWNER/REPO" --domain "vpn.example.com" --email "admin@example.com" --channel "edge"
```

- Installer supports interactive prompts if flags are omitted and prints each stage/result.
- Prefer the tagged stable channel for production once a versioned release exists; `edge` is for the latest successful branch build/testing.
- Provide documentation that explains how to inspect the installer before running it with elevated privileges. The downloaded release must be checksum-verified, and the installation script itself must be fetched from the user's trusted repository.
- The script must check that the OS and architecture are supported, enough disk space exists, DNS points to the intended server, required ports are available, and an existing service/configuration will not be overwritten. When DNS/ports are not ready, explain the exact correction and exit safely.
- Install only missing required dependencies (Caddy, systemd integration and basic OS utilities). The database file is initialized/migrated by the app; do not install a separate database server for SQLite.
- Create a dedicated app user/group and directories with least-privilege permissions, e.g. `/opt/vpn-platform` for release files, `/etc/vpn-platform` for secrets/config, `/var/lib/vpn-platform` for the database, and `/var/backups/vpn-platform` for backups. Adapt paths consistently if the existing project has established conventions.
- Generate session/encryption secrets securely using a cryptographic random source; never use hard-coded defaults. Restrict config files to root/app-group access as required.
- Run migrations with a pre-migration backup. Create or securely initialize the first Super Admin without shipping a known default password; show a one-time bootstrap secret only once or use a secure interactive setup flow.
- Configure the backend to listen on loopback only. Configure and validate Caddy, start/reload services, wait with a bounded timeout for readiness, verify HTTPS and API health, then display the URL and next setup steps.
- Do not turn on a restrictive firewall blindly. If a firewall is present, show proposed rules and apply them only through a safe, explicit option that preserves the current SSH connection. Public entry ports normally include 80/443; the SSH port must remain reachable.
- Use a lock to prevent two installers/upgrades from running concurrently. Make repeat runs detect an existing install and choose safe upgrade/reconfigure behavior, not reinstall from zero.

### 14.4 DNS, TLS and public access prerequisites

- Before installation, the domain's `A` record must point to the server's public IPv4 address. If IPv6 is in use, its `AAAA` record must point to a working public IPv6 endpoint; remove stale `AAAA` records.
- TCP ports `80` and `443` must be reachable from the Internet and not held by another web server. Allow UDP `443` only if the deployment enables/needs HTTP/3.
- Use Caddy's automated ACME certificate management and renewal; redirect HTTP to HTTPS. Do not disable certificate validation or use self-signed TLS for the public production hostname as a shortcut.
- If the domain is behind a DNS/CDN proxy, document the supported proxy mode and required settings instead of assuming direct DNS validation will always work.
- Detect and explain certificate failures (DNS, firewall, port conflict, rate limit, CAA or incorrect IPv6) without printing secrets. A failed certificate issuance must leave a clear non-production/incomplete status rather than reporting the website secure.

### 14.5 Change the application domain after installation

A working production domain must be changeable without reinstalling the app or touching business data. Provide a command such as:

```bash
sudo vpn-platformctl domain set vpn-new.example.com --email admin@example.com
```

The final CLI name can follow the existing repo, but the behavior is mandatory:

1. Validate the hostname and confirm the operator understands the DNS/ports prerequisites.
2. Back up current reverse-proxy/app configuration.
3. Confirm the new domain's DNS and ports before applying the change.
4. Update the app's canonical/public URL, validated Caddy configuration and any required trusted-origin/cookie settings; do not hard-code a domain in the frontend bundle.
5. Validate Caddy configuration before reload; preserve the old working configuration for rollback.
6. Wait for certificate provisioning and verify HTTPS, login/session, API calls and SPA deep links on the new domain.
7. Keep the old domain working during a configurable transition if requested and technically safe. Do not silently delete its certificate/config. Report DNS/ACME errors clearly.
8. Keep existing database records, passwords, server integrations and user settings untouched.

### 14.6 Change a 3X-UI panel URL/credentials

This is a separate operation from changing the application's domain. Super Admin must be able to edit a panel URL/API credential through the secure Server Integrations UI, click **Test connection**, inspect detected version/capabilities and explicitly save verified changes. Encrypt secrets at rest, redact them in the UI after save, stop new provisioning while re-verification is required, and retain accurate history/incident status. Do not require a redeploy to change a connected 3X-UI panel URL.

## 15. Production operations, upgrades and recovery

- Store production config outside the Git checkout; production secrets must not be overwritten by releases.
- Use versioned, repeatable migrations. Before any schema upgrade, create and verify a database backup. If a migration fails, stop safely and document the restore/forward-repair path; do not delete the database to “fix” startup.
- Maintain at least one previous binary/release to support rollback. Roll back application code only when compatible with the current schema; restore the pre-upgrade DB snapshot when a migration rollback cannot be safely automated.
- The CLI must provide clear equivalents of `status`, `health`, `logs`, `backup`, `restore`, `upgrade`, `rollback` (if supported), `domain set`, and `uninstall --preserve-data`.
- Health output must not contain credentials, UUIDs, tokens or connection strings. Logs use structured levels, request IDs and safe error categories.
- Provide an operator-visible server-integration incident list, last successful sync time and bounded retry behavior. Avoid repeated notification spam by de-duplicating incidents and marking recovery.
- Backups should include the SQLite DB using a consistency-safe method, app configuration required for recovery, and a manifest of app/schema versions. Encrypt/export sensitive backups securely and define retention. Never put backup files in the public web root.
- Document restore validation, including restoring to a clean test directory/VM and checking users, assignments, payment history, themes and audit logs.
- Provide a scheduled backup option without requiring another always-on daemon. Do not claim a backup is usable until a restore test has passed.
- Admin setup/change screens must warn when 3X-UI API tokens have full-admin permissions and explain why restricted storage/transport is essential.

## 16. Security additions and operational boundaries

- Use HTTPS externally and secure cookies (`Secure`, `HttpOnly`, appropriate `SameSite`), session rotation, session expiry, CSRF/origin safeguards and rate limiting appropriate to the authentication model.
- Enforce role and row-level ownership checks in backend services for all list/detail/mutation/chat/payment/QR/usage routes. Never trust a client-supplied user ID, admin ID, seller ID, inbound ID or role.
- Protect panel-URL validation against SSRF: allow approved HTTP(S) schemes only, validate resolved IP addresses, block loopback/private/link-local/cloud-metadata destinations unless a deliberate documented private-network policy allows them, validate every redirect target and prevent DNS-rebinding bypasses. Since panel endpoints can legitimately be private IPs, make private-address allowance an explicit Super Admin deployment policy—not an unrestricted default.
- Never disable TLS certificate checking globally. If a panel uses private CA/self-signed TLS, require a deliberate per-integration trust configuration with clear risk messaging, not an insecure process-wide switch.
- Limit QR/link access to the assignment owner or permitted operational roles. Do not log generated connection URIs. Return connection material only on authorized endpoints when the assignment is confirmed active and the integration state permits it.
- Use an explicit state machine for payment, assignment and remote revocation. All manual payment confirmations, quota changes, credential updates and destructive actions must be attributable in audit logs.
- Restrict uploaded payment/QR images by file type/size, randomize stored file names, store outside public execution paths, and authorize every download.
- Do not use client-only restrictions for quotas. Enforce quota allocation and provisioning in one transactional/idempotent flow, including concurrency tests.
- Add dependency scanning and secret scanning to CI where practical. Pin/lock dependency versions, use least-privilege CI tokens, and never expose build/release signing secrets to untrusted pull-request workflows.

## 17. Cline implementation sequence and change-control rules

Cline must work in this order and record progress in `docs/IMPLEMENTATION_PLAN.md`:

1. **Inventory & baseline:** inspect repo, run existing tests/build, check current memory/deployment setup, identify live schemas and known incomplete features.
2. **Gap analysis:** map sections 1–11 business rules, Theme Studio requirements, deployment requirements, and tests against actual code. Mark external/API limitations explicitly.
3. **Architecture decision:** retain or adjust stack based on repository evidence and 1 CPU / 500 MB constraints. Record the decision and avoid rewrite-for-preference.
4. **Data/auth foundation:** migration plan, role/ownership checks, secrets protection, status state machines, idempotency and audit events.
5. **Server integration:** version/capability adapter, panel validation, health jobs, partial failures and verified provisioning/revocation behavior.
6. **Business workflows:** Free offers/accounts, Premium reseller/direct sale paths, quota ledger, manual payments, seller ownership, contacts/chat, QR/links and history.
7. **Frontend modernization:** shared design system and role-based layouts, responsive pages, accessible form/table states, true data loading/error states.
8. **Theme Studio:** five main presets, custom sub-theme colors, typography, 3D/effect controls, reduced motion, persistence, preview/reset and cross-page verification.
9. **Release/installer:** CI outputs verified Linux artifacts; one-command install, HTTPS, domain change, upgrade, backup and restore paths.
10. **QA & hardening:** acceptance tests, permission-negative tests, failure simulations, cross-theme UI checks, low-resource benchmarks and clean-install rehearsal.
11. **Delivery:** update README/docs, `docs/QA_REPORT.md`, release/install instructions, residual limitations and exact commands run.

### Mandatory behavior for every implementation task

- Before editing: identify the relevant files and state the change scope in the implementation notes.
- During work: make small coherent changes, do not overwrite unrelated work, inspect `git diff`, and keep business rules consistent across API/UI/database/tests.
- After editing: run focused tests first, then relevant full tests/build; inspect failures and fix root causes.
- For each feature, include normal, unauthorized, invalid-input, stale/offline and duplicate/retry cases where relevant.
- Never claim a command was executed when it was not executed. Never claim remote server success based solely on local database state.
- If the full application cannot be completed in a single implementation pass, leave a working build, list exactly what is done/untested/blocked, and do not conceal unfinished critical flows behind fake buttons or mocked production data.

## 18. Additional acceptance tests — themes, modern UX and deployment

The original business/security acceptance tests in section 9 remain mandatory. Add all tests below:

### Theme Studio / visual UX

19. A signed-in user can choose Dark, Light, Normal/Classic, Gaming and Modern; each preset visibly changes the full app, not just the settings page.
20. User can adjust accent/sub-theme colors, typography/text size and visual-effect level; preview/cancel/save/reset all work and invalid values cannot inject CSS/script.
21. A saved theme persists after logout/login, refresh and (where supported by the preference model) on another device. Super Admin's default applies to new/unconfigured users without unexpectedly overwriting saved personal preferences.
22. `Off` / Low-Resource mode disables heavy 3D, continuously animated backgrounds and unnecessary graphical imports. `prefers-reduced-motion` is respected.
23. Essential screens pass a keyboard-navigation and contrast review in all five themes; dialogs, form errors, tables, charts and mobile layouts remain readable.
24. Sinhala and English UI text display with working glyphs and no unacceptable clipping or overflow at the supported font sizes.

### Architecture / low-resource behavior

25. Production process listing confirms Node.js is not running as a persistent production application server in the recommended Go-binary deployment.
26. Static frontend assets, backend API and SPA deep links are served correctly through the same public hostname and reverse proxy.
27. With 20–30 configured panel integrations and slow/offline panels, the dashboard still loads from cached/local data, shows stale/unknown status clearly, and does not create an unbounded number of workers/connections.
28. A documented benchmark measures idle and typical load on a 1-vCPU/500-MB VM; no OOM/restart loop occurs during the defined test. Record actual peak RAM and CPU, not an estimate.
29. No page displays invented traffic history, metrics, link details or false success status; theme and chart components do not create unbounded polling.

### Installation / HTTPS / domain change

30. A clean supported Linux VM can install from the documented single-line command after the appropriate release artifact is available; rerunning the installer does not wipe data.
31. The installer validates architecture, DNS, ports and existing services; detects conflicts and exits safely with useful instructions.
32. The first install creates the required database/schema, systemd service, app config, Caddy configuration and initial Super Admin setup without a hard-coded password.
33. With correct DNS and reachable ports 80/443, a public trusted certificate is issued and HTTPS succeeds; HTTP redirects to HTTPS and certificate renewal remains configured.
34. Bad DNS, closed ports, stale IPv6 DNS, a port conflict or failed ACME issuance results in a truthful incomplete/error state, not a false “HTTPS active” message.
35. The application domain can be changed using the documented CLI without losing users, themes, assignments, payment records, integrations or audit history. Rollback works if Caddy validation/certificate setup fails.
36. A 3X-UI panel URL/credential can be changed in the Super Admin UI, reverified and used without rebuilding or redeploying the app.
37. Upgrade runs a pre-upgrade backup, applies migrations safely, confirms health and provides a documented recovery route if it fails.
38. A backup restores to a clean test environment and preserves users, VPN assignments, payment/audit history and appearance preferences.
39. Installer/release download verifies the artifact checksum; logs, frontend bundles, errors and browser network responses contain no panel tokens or server-side secrets.

## 19. Final delivery checklist

Before Cline marks the project complete, the repository should contain or document the following (adapt exact names only where existing conventions justify it):

- `MASTER_PRODUCT_BIBLE.md` — this source of truth.
- `docs/GAP_ANALYSIS.md` — requirement-by-requirement initial audit and final status.
- `docs/IMPLEMENTATION_PLAN.md` — ordered phases, progress and remaining work.
- `docs/DECISIONS.md` — documented assumptions and architecture trade-offs.
- `docs/QA_REPORT.md` — exact commands/tests and truthful results.
- `docs/INSTALL.md`, `docs/UPGRADE.md`, `docs/DOMAIN_CHANGE.md`, `docs/BACKUP_RESTORE.md`, `docs/TROUBLESHOOTING.md`.
- Repeatable database migrations and a tested backup/restore path.
- CI workflow for tests/build and a release workflow for verified Linux artifacts.
- One-line first install and a tested domain-change CLI command.
- A secure initial Super Admin bootstrap flow and no default production credentials.
- All three role experiences, all original business flows, Theme Studio and required theme presets.
- Automated tests for authorization, quota concurrency, idempotent provisioning, panel outage states, payments/chat access and theme persistence.
- A documented 1-vCPU/500-MB benchmark and a clear list of any residual limits.

Do not mark complete just because the files exist. Confirm that the implementation and tests actually exercise them.

## 20. Official references to re-check during implementation

Use current official documentation when implementing; confirm it matches the actual dependency/panel version rather than copying stale examples blindly.

- 3X-UI API overview/authentication: https://docs.sanaei.dev/docs/reference/api/ and https://github.com/MHSanaei/3x-ui/blob/main/docs/content/docs/en/reference/api/index.mdx
- 3X-UI inbound operations: https://github.com/MHSanaei/3x-ui/blob/main/docs/content/docs/en/reference/api/inbounds.mdx
- 3X-UI client operations: https://github.com/MHSanaei/3x-ui/blob/main/docs/content/docs/en/reference/api/clients.mdx
- 3X-UI releases: https://github.com/MHSanaei/3x-ui/releases
- React + TypeScript: https://react.dev/learn/typescript
- Vite production build/deployment: https://vite.dev/guide/build and https://vite.dev/guide/static-deploy.html
- SQLite appropriate uses and concurrency trade-offs: https://www.sqlite.org/whentouse.html
- Caddy automatic HTTPS prerequisites and certificate automation: https://caddyserver.com/docs/automatic-https and https://caddyserver.com/docs/quick-starts/https

