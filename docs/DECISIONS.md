# VEYLIX VPN — DECISIONS LOG

> Every owner-approved decision and Bible deviation is recorded here, with reason and date.
> Format: **ID | Date | Decision | Reason | Approval**

---

## D-001 · Greenfield build
- **Date:** 2026-10-21
- **Decision:** Build the platform from scratch in this repo. The Bible's "finish the existing app" assumption does not apply — the repository contained only `MASTER_PRODUCT_BIBLE.md` (no code, no DB, no tests). Bible §13.1 ("retain working stack") resolves to "no existing stack" â†’ recommended greenfield stack applies.
- **Approval:** Owner (explicitly confirmed — "bible eke podi mistake ekak").

## D-002 · Implementation order — Frontend first, then Backend
- **Date:** 2026-10-21
- **Decision:** Implement the frontend (UI + design system + Theme Studio) first, then the backend on top of the agreed API contract. This differs from the Bible's abstract ordering; it matches the owner's live-testing workflow.
- **Approval:** Owner ("mulin front end eka gihin e anuwa backend eka yanna one").

## D-003 · Live-testing workflow
- **Date:** 2026-10-21
- **Decision:** After every milestone the owner tests the app live on localhost (dev server). A production clean-VM install test is scheduled in the packaging phase. Implementation steps pause for user verification.
- **Approval:** Owner ("hema welema mn live test karanawa ... live testing environment ekak hadagena yanna one").

## D-004 · Scope/flow change control
- **Date:** 2026-10-21
- **Decision:** Any change to flow, scope, or plan requires explicit owner approval before implementation. Changes are recorded here and reflected in REQUIREMENTS/PLAN/ROADMAP.
- **Approval:** Owner ("flow eka wenas karanawa nm awasara ganna one ... step by step").

## D-005 · Technology stack (Bible recommended)
- **Date:** 2026-10-21
- **Decision:** React + TypeScript + Vite (frontend, build-time only) · Go single binary (backend) · SQLite on local disk · Caddy reverse proxy + auto HTTPS. No Node.js/Redis/PostgreSQL/Docker production services on the 500 MB server.
- **Approval:** Owner (adopted from Bible §13.2; no objection raised).

## D-006 · Frontend mock-data stage
- **Date:** 2026-10-21
- **Decision:** During frontend-first build, the UI consumes API-shaped mock data (clearly labeled) to enable early live testing. Mocks are replaced with real API calls in the wiring phase.
- **Approval:** Owner (implied by live-test workflow; frontend-first order makes an interim mock stage necessary).

## D-007 · Out of scope for v1
- **Date:** 2026-10-21
- **Decision:** Automated payment gateway, blockchain/NFT features, and native mobile apps are out of v1 scope. Manual payment (bank/eZ Cash) with auditable seller confirmation is the payment model.
- **Approval:** Owner (default retained from Bible §10.3).

## D-008 · Theme palette rebuild + UI 2.0
- **Date:** 2026-10-22
- **Decision:** Rebuilt all five palette identities in a premium SaaS direction, expanded sub-themes to **40** (8 per preset) including official community palettes (Nord, Dracula, Tokyo Night, Catppuccin Mocha/Latte, Solarized Light, One Light, RosÃ© Pine Dawn, Gruvbox), and shipped UI 2.0 (âŒ˜K command palette, WAI-ARIA dialog focus management, tablist arrow keys, table sorting, sidebar rail collapse, alive dashboard, motion/skeleton polish).
- **Reason:** Owner request — "colors not attractive â†’ research the most attractive; more themes; deeper sub-themes; more UI improvement". All 40 variants contrast-audited (AA).
- **Approval:** Owner (each step requested and approved interactively).

## D-009 · Command palette is in-house
- **Date:** 2026-10-22
- **Decision:** The âŒ˜K palette is implemented in-house (no `cmdk`/Radix dependency), styled fully from the theme token system.
- **Reason:** Keeps the dependency tree minimal in line with the 500 MB small-server production constraint (Bible §0.9); avoids bundling an unstyled library to restyle anyway.
- **Approval:** Owner (full-package approval; recorded for transparency).

## D-010 · Phase order supersedes D-002 — backend & 3X-UI integration first

- **Date:** 2026-10-23
- **Decision:** `docs/new_reqierment/new_update.md` §11 (Stages Aâ€“H: baseline â†’ architecture â†’ audit â†’ prioritised plan â†’ incremental fixes â†’ tests â†’ live verification â†’ final review) replaces D-002's "frontend first, then backend" ordering. The next implementation phases are the Go backend, the SQLite data layer and the 3X-UI adapter. The already-shipped frontend is kept as-is and wired to real APIs behind its existing store surface (mock stores stay until their API replacement lands — D-006 exit).
- **Reason:** The new requirement's core mission is *"Do not rebuild from scratch; improve file by file"* (§3, §4). The repository currently has **zero backend** (`GAP_ANALYSIS.md` §1), so every business rule is simulated. Continuing frontend-first would require a later wholesale rewrite of 10 mock stores — the opposite of file-by-file improvement.
- **Approval:** Owner (approved the plan and switched to act mode, 2026-10-23).

## D-011 · 3X-UI authentication uses a Bearer API token per integration

- **Date:** 2026-10-23
- **Decision:** Each `server_integrations` row stores a panel **API token** (`Authorization: Bearer <token>`, created in the panel under Settings â†’ Security â†’ API Token), encrypted at rest with AES-GCM using a key supplied by the deployment config. The `POST /login` username/password/2FA + session-cookie flow is **not** used. Runtime contracts are confirmed per panel via `GET /panel/api/openapi.json` and `GET /panel/api/server/status`.
- **Reason:** The official OpenAPI spec documents `bearerAuth` as the automation method and marks `cookieAuth` as browser-only; `POST /login` additionally requires 2FA state and session rotation. The spec also warns that API tokens are full-admin credentials — hence the encryption and the "never in responses/logs/bundles" rule (Bible §0.6, `new_update.md` §7).
- **Contract reference:** `docs/3XUI_API_CONTRACT.md` §1. Target release verified: **v3.10.2**.
- **Approval:** Owner (approved as part of the docs plan).

## D-012 · Client data is read through the paged client API; idempotency pair = `email` + deterministic `subId`

- **Date:** 2026-10-23
- **Decision:**
  1. Client lists/synchronisation use `GET /panel/api/clients/list/paged` (server-side filter/sort/pagination, max 200 rows). `GET /panel/api/inbounds/list` is used only where `clientStats` per inbound is genuinely required, and `/inbounds/options` for pickers.
  2. Every provisioned panel client gets `email = vyx-<localClientID>` and `subId = vyx-<localClientID>` so a retried write cannot silently create a duplicate (requirement §6).
  3. A timed-out write is resolved by a fresh read of `GET /panel/api/clients/get/{email}` **before** any retry; the result is recorded in a local outbox.
- **Reason:** `/inbounds/list` returns every inbound's full client array and stats — far too heavy for a 1 vCPU / 500 MB server and for panels with thousands of clients (`new_update.md` §8). The idempotency pair directly satisfies `new_update.md` §6 and §5's ambiguous-outcome rule.
- **Approval:** Owner (approved as part of the docs plan).

## D-013 · SQLite driver is CGO-free (`modernc.org/sqlite`)

- **Date:** 2026-10-23
- **Decision:** The backend uses `modernc.org/sqlite` (pure Go) with WAL mode, `foreign_keys=ON`, a bounded connection pool and embedded migrations.
- **Reason:** CGO-free builds cross-compile cleanly for linux/amd64 and arm64 from CI without a C toolchain, which matters for the one-line installer and the release checksum pipeline (Bible §13.2, REQUIREMENTS §10). The driver choice must still be benchmarked on the target VM during Stage G — this decision is revisitable with evidence.
- **Approval:** Owner (approved as part of the docs plan).

## D-014 · Explicit SDLC phases with hard gates

- **Date:** 2026-10-23
- **Decision:** Work advances through 9 phases (Documentation â†’ Baseline â†’ Architecture â†’ Audit â†’ Prioritise â†’ Build â†’ Unit test â†’ **QA** â†’ Live verify â†’ Release) each ending in a named gate with evidence-based exit criteria. Documented in `docs/SDLC_PLAN.md`; Gate 7 (**QA PASS**) is a first-class gate, not an afterthought.
- **Reason:** Owner request — the project must be run "according to the SDLC, with QA done properly". Without gates the previous phases drifted (docs claimed pages that were placeholders; README-level claims outran evidence).
- **Approval:** Owner (explicit request).

## D-015 · QA tooling and the honesty rule

- **Date:** 2026-10-23
- **Decision:** QA is executed per `docs/QA_PLAN.md` and reported in `docs/QA_REPORT.md`. Results are recorded only from commands actually executed. Blocked checks (e.g. `-race` without a C compiler, live panel checks without a panel) are recorded as BLOCKED, never as passing. Frontend unit tests will use `vitest` + `@testing-library/react` (dev-only dependencies, no production bundle impact).
- **Reason:** `new_update.md` §9 ("do not treat mocked tests as proof of live compatibility") and §13 ("never fabricate test results").
- **Approval:** Owner (part of the SDLC request).

## D-016 · API tokens encrypted with AES-256-GCM under a mandatory config key

- **Date:** 2026-10-23
- **Decision:** `server_integrations.token_ciphertext` is a BLOB; the key comes from `VEYLIX_ENCRYPTION_KEY` (base64, 32 bytes) and the process **refuses to start** if it is missing, too short or not base64.
- **Reason:** Panel API tokens are full-admin credentials (verified in the official spec). Failing open would silently store usable admin credentials in plaintext. Startup refusal is itself unit-tested (`TestLoadFailsClosedWithoutEncryptionKey`).
- **Approval:** Owner (part of Stage B approval).

## D-017 · Panel base path is part of the integration URL; 404 â‰  invalid token

- **Date:** 2026-10-23
- **Decision:** A panel integration stores the **full base URL including the web base path** (e.g. `https://host:2053/<webPath>`). Every adapter call appends `/panel/api/...` to it. HTTP `404` is classified as `CONNECTION_UNAVAILABLE`, **never** as `REAUTH_REQUIRED`.
- **Reason:** Verified on the live 3.10.2 panel — calling `https://host:2053/panel/api/server/status` (no base path) returns `404` with an empty body, while the same call under the base path returns `200`. Treating 404 as "bad credentials" would mislead the Super Admin into rotating a valid token.
- **Test:** `TestWrongBasePathIsUnavailableNotReauth`.
- **Approval:** Owner (discovered and confirmed during the requested live test).

## D-018 · Live write operations require explicit owner authorisation

- **Date:** 2026-10-23
- **Decision:** The adapter implements client create / suspend / resume / delete, but the automated live tests exercise **read-only** operations. Any live write test must be authorised explicitly and target a non-production inbound.
- **Reason:** The provided panel serves real users (`new_update.md` §11: "Do not â€¦ alter production clients", §7 data integrity).
- **Approval:** Owner (safe default while the test panel is shared with production traffic).

## D-019 · Password hashing uses argon2id from `golang.org/x/crypto`

- **Date:** 2026-10-23
- **Decision:** Passwords are hashed with **argon2id** (m=64 MiB, t=2, p=2, 16-byte salt, 32-byte key), parameters encoded in the stored string so they can be raised later, with `NeedsRehash` upgrading older hashes on the next successful login.
- **Reason:** REQUIREMENTS §8 names argon2 explicitly, and it is the OWASP-recommended password KDF. The Go standard library does not ship argon2, so one well-maintained dependency is justified. The module proxy turned out to be reachable, so the earlier standard-library-only PBKDF2 fallback was discarded.
- **Trade-off:** 64 MiB per hash is heavy for a 512 MB server; concurrency is naturally bounded because logins are rate-limited to 5 per burst per IP. Revisit with a measured benchmark if memory pressure appears.
- **Approval:** Owner (part of the Stage E2 approval).

## D-020 · Session model: server-side rows, cookie carries an opaque id only

- **Date:** 2026-10-23
- **Decision:** The session cookie holds a random 256-bit id; the database stores only its SHA-256 fingerprint, with a 12-hour expiry, `HttpOnly`, `SameSite=Lax`, and `Secure` on every network-reachable bind. Suspending a user invalidates their sessions on the next request.
- **Reason:** A database dump must not yield usable cookies, and server-side rows make immediate revocation possible (a pure JWT cannot be revoked without extra state).
- **Approval:** Owner (part of Stage E2 approval).

## D-021 · `GET /api/quotas/me` answers every role with one shape; a client gets `remaining:-1`

- **Date:** 2026-10-23
- **Decision:** The endpoint returns `{used, limit, remaining}` to every authenticated caller. A **client** has no reseller quota, so it receives the constant `{used:0, limit:0, remaining:-1}`, where `remaining:-1` means "not applicable / unlimited". It is **not** a `403`.
- **Reason:** A reseller quota is a seller-side concept (REQUIREMENTS §2.2), so a client genuinely has nothing to report. But the UI reads its quota through **one** call site shared by all three roles, and the previous `403 clients do not hold a reseller quota` made that call fail for the most common role. `docs/API_CONTRACT.md` documents exactly one response shape for this route, so a 403 also contradicted the contract. Returning a constant costs nothing and leaks nothing: the payload is the caller's own row and is identical for every client.
- **Superseded behaviour:** the earlier `403` + `{"error":{"code":"forbidden"}}` envelope documented in `API_CONTRACT.md` §1.
- **Tests:** `TestQuotaMeForClientIsZeroNotForbidden` (client â†’ 200 + the constant), `TestQuotaMeForReseller` (admin â†’ real `limit`, super admin â†’ `?admin_id=`), `TestQuotaMeRequiresAuth` (anonymous â†’ 401).
- **Also fixed in this pass:** `newHarness` in `auth_e2e_test.go` never wired `Audit`/`Quota`, so `AccountRoutes` was not mounted and the endpoint had **no** httpapi-level coverage at all — `newQuotaHarness` wires the real store.
- **Approval:** Owner (approved Option B of the QA review).

## D-023 · Sync cadence, bounded work, and the reset-proof usage fold

- **Date:** 2026-10-23
- **Decision:** Stage E5 runs a **read-only** panel sync on a fixed cadence (default **5 min**, `VEYLIX_SYNC_INTERVAL`; `0` disables it), bounded by:
  - **3 concurrent servers** per cycle, **60 s** per server (`VEYLIX_SYNC_TIMEOUT`), **200 clients per page** and at most **50 pages** (`VEYLIX_SYNC_PAGE_SIZE`, `VEYLIX_SYNC_MAX_PAGES`) — matching the panel's own page cap;
  - a **no-overlap guard**: if a cycle is still running when the next tick arrives, the tick is skipped rather than queued;
  - a **jittered first tick** (up to 10% of the interval) so a fleet restart does not hit the panels at once.

  Usage history is folded with `sync.Fold`:
  - a **decrease in either direction**, or a **`resetCount` above the stored high-water mark**, is a reset;
  - on a reset the lifetime total becomes `prev + current` — the only arithmetic that never loses bytes and never double-counts;
  - the stored `resetCount` is **monotonic** (max of stored and observed), because a stale panel read can report a lower value than one we already recorded.
- **Reason:** `REQUIREMENTS` §3 requires "per-server background jobs, timeouts, pagination, last-sync timestamps, stale/unknown states; one slow panel must not block the dashboard", and §2.7/§5 + F-010 require history that survives a remote counter reset. `server_integrations.last_health_check` guidance in `3XUI_API_CONTRACT.md` caps polling at ~30 s per panel, so a 5-minute default is safely conservative.
- **Never revokes:** sync writes only `EXPIRED`, `TRAFFIC_DEPLETED` and `SUSPENDED`, and each candidate is validated against `domain.CanTransition`. A client missing from the panel read, or an unreadable panel, leaves the VPN untouched — revocation stays with `provisioning.Revoke` + the outbox reconciler (REQUIREMENTS §7).
- **Read-only, so D-018 does not apply:** sync performs `GET /panel/api/server/status` and `GET /panel/api/clients/list/paged` only. It is live-verified against the production panel.
- **Where history can be incomplete:** snapshots only start from the first tick, so a VPN created before E5 has no earlier data. The API reports `history: "AVAILABLE" | "NONE"` and `first_snapshot_at` rather than interpolating (REQUIREMENTS §2.7 "never invent history").
- **Tests:** `internal/sync` — 23 tests (fold table incl. `TestFoldNeverLosesBytes`, stale `resetCount`, reset-only-one-direction, idempotent re-read; service: pagination 450â†’3 requests, per-server isolation, hanging-panel timeout, outage-never-revokes, expired/depleted/suspended transitions, unseen-client no-op, same-second upsert; scheduler: overlap guard, zero-interval disable, idempotent Stop). `internal/httpapi` — 6 route tests incl. the IDOR check. `internal/config` — 3 env tests.
- **Live verification (2026-10-23):** against `indian1` (3X-UI 3.10.2, Xray 26.10.10) with `VEYLIX_SYNC_INTERVAL=30s`: `schema_version:5`; 3 consecutive ticks with **zero** WARN/ERROR; `last_sync` stamped, `stale:false`; real metrics recorded (`cpu 1.22% â†’ 0.42% â†’ 0.60%`, `mem 419 MB â†’ 404 MB â†’ 407 MB` of 998 MB).
- **Approval:** Owner ("go to the next stage").




---

## D-024 · A VPN **is** a 3X-UI client; capacity and limits come from the panel; Free/Premium is an operator tag

- **Date:** 2026-10-24
- **Status:** **INVARIANT — owner-mandated. Binding on all future work.** Any code, test, migration or UI that contradicts this decision is a defect, not a design choice.
- **Decision:** The 3X-UI panel is the **only** authority on what a VPN is and what it can do. Veylix mirrors the panel; it never invents, overrides or predicts it. Five invariants follow, each independently checkable:

  | ID | Invariant | Enforced by |
  |---|---|---|
  | **D-024.1** | **VPN â‰¡ 3X-UI client.** A VPN record is legitimate **iff** a client with the same `(server_id, panel_email)` exists on that panel. A client that cannot be shown to exist is not a VPN. | create ordering (D-024.1a), verified absence (D-024.5) |
  | **D-024.1a** | **Creation order is inverted.** The panel write happens **first**; the local row is written **only after** `GET /panel/api/clients/get/{email}` confirms the client. A failed or unconfirmed create leaves **no VPN row at all** — only an outbox entry and an audit record. | `provisioning.Create` |
  | **D-024.2** | **Capacity, duration, device limit, usage and enable-state come from the panel — only.** `totalGB`, `expiryTime`, `limitIp`, `limitHwid`, `traffic.up/down`, `resetCount`, `enable` are **read** from `GET /panel/api/clients/list/paged` (`panel.ClientRow`) and mirrored. Veylix never derives a VPN's limits from an offer, a user, or a form. At **create** time the Super Admin supplies the initial values and Veylix writes them to the panel; from that moment on the panel's copy is the truth and Veylix only reads it. | sync overwrite (D-024.4) |
  | **D-024.3** | **Every panel client is a VPN. FREE/PREMIUM is a visibility tag set by the Super Admin — nothing more.** An untagged client is **still a VPN**; it is simply **unclassified**, and an unclassified VPN is visible **only to the Super Admin**. Tagging it `FREE` or `PREMIUM` is what makes it visible to resellers and clients. Veylix stores the tag locally and never writes it to the panel. A panel-side `free`-looking name means nothing — **only Veylix's own tag decides**. | `panel_client_tags` |
  | **D-024.4** | **The panel wins on every conflict.** If a value changes on the panel (capacity 10 GB â†’ 20 GB, expiry extended, client disabled), the next sync reflects it. There is **no operator override** of a panel-supplied value; the only operator-supplied value in the whole VPN model is the Free/Premium tag. | sync |
  | **D-024.5** | **Absence is verified, never assumed.** A client missing from a page-through becomes `SERVER_UNAVAILABLE` (never `REVOKED`) until a direct `GET /panel/api/clients/get/{email}` **positively** reports it absent. An unreadable panel changes nothing. | `domain.SeverOutage`, sync |

  **Clients created directly in 3X-UI are real VPNs.** Every one of them enters the system as an **unclassified** VPN (visible to the Super Admin only) and becomes visible to resellers/clients the moment it is tagged (D-024.3). Rows created by Veylix are the same kind of object — the panel is what makes them real.

  **Visibility matrix (D-024.3 + D-025):**

  | VPN state | Super Admin | Admin (reseller) | Client |
  |---|---|---|---|
  | Panel client, **untagged** (unclassified) | âœ… | âŒ | âŒ |
  | Panel client, **FREE** | âœ… | âŒ (not a resale item) | âœ… catalogue |
  | Panel client, **PREMIUM** | âœ… | âœ… (assigned servers) | âœ… (only their own) |
  | Issued to a client | âœ… | âœ… (if their sale) | âœ… (their own) |

- **Reason:** Owner, 2026-10-24 (repeated, explicit): *"ehema link hedunath 3x ui pannel eke pennana one new client kenek widihata â€¦ vpn ekak kiyala thiranaya wenna nn 3x ui client kenek wima aniwaryayi â€¦ oyata hithena hithena widihata vpn hadanna ba â€¦ 3x ui client thama vpn ekak wenne â€¦ vpn capacity 10g, 20gb device gana duration okoomama thiranaya wenne 3x ui haraha â€¦ free primium thiranaya karanne super admin eyatha tag karanne."* The failure the owner observed — VPNs listed in Veylix that do not appear as clients in 3X-UI — is exactly what an intent-row model produces. The panel is the only thing that can answer "does this VPN work".

- **Evidence that the code at the time of this decision violated D-024** (verified in this repository):

  | # | Violation | Location |
  |---|---|---|
  | 1 | The VPN row is inserted **before** any panel call, with `panel_email=''` / `panel_sub_id=''` / `status='PENDING'` — a VPN that provably does not exist yet. | `internal/provisioning/service.go:211-214` |
  | 2 | A rejected panel write stores `REVOCATION_PENDING`, which claims a client existed and was deleted. It never existed. | `internal/provisioning/service.go:298-302` |
  | 3 | Capacity/expiry/device limit are taken **from the offer**, not the panel: `TotalGB: offer.TrafficGB * bytesPerGB`, `ExpiryTime: now + offer.DurationDays`. | `payload.go:21-27`, `service.go:203-216` |
  | 4 | `vpn_instances` accepts `PENDING` / `SERVER_UNAVAILABLE` / `REVOCATION_PENDING` rows that correspond to no confirmed panel client. | `0002_business.sql:28-31` |
  | 5 | An offer with `inbound_id = 0` still provisions — `InboundIDs: []int{0}` is sent and rejected. Only `ServerID == 0` is validated. | `payload.go:29`, `service.go:117` |
  | 6 | The panel client list is **never** used to build the VPN list; `sync` reads it for usage only and silently `continue`s past absent clients. | `internal/sync/service.go:236-243` |
  | 7 | `GET /api/vpns` lists every `vpn_instances` row regardless of panel reality. | `internal/httpapi/vpn_lister.go` |
  | 8 | Offers cannot gain an inbound after creation (`UpdateOfferInput` has no `ServerID`/`InboundID`), so a broken offer can never be repaired. | `internal/store/offers.go:143-149` |

- **Data model consequences:**
  - **New:** `panel_client_tags (server_id, panel_email, tag, tagged_by, tagged_at)` — the *only* operator-supplied VPN attribute (D-024.3).
  - **New mirror columns** on `vpn_instances`: `panel_total_gb`, `panel_expiry_time`, `panel_limit_ip`, `panel_enable`, `panel_synced_at` — written by sync from the panel, never by a form (D-024.2/D-024.4).
  - **Kept but demoted:** `vpn_offers.traffic_gb` / `duration_days` / `device_limit` stay for history and to describe an *intended* offer, but they no longer decide any VPN's limits. `vpn_offers` means "what the Super Admin is offering", not "what a client receives".
  - **Kept:** `(server_id, panel_email)` is already `UNIQUE` (`0002_business.sql:40`) — identity is unchanged, which is what makes this decision implementable without data loss.

- **Non-goals (explicitly rejected):**
  - Writing the tag to the panel's `comment` field (a panel-side edit could silently reclassify a VPN).
  - Deriving Free/Premium from the inbound, the protocol, or the client's limits (two VPNs on one inbound must be able to differ).
  - Any "optimistic" VPN row that exists before the panel confirms it.

- **Implementation stages** (each independently verifiable; a stage may not be merged while an earlier stage is red):

  | Stage | Change | Proof |
  |---|---|---|
  | **A** | This document + `REQUIREMENTS.md` §2.0 + `MASTER_PRODUCT_BIBLE.md` §0.18 + `API_CONTRACT.md` | Docs agree with each other |
  | **B** | Migration `0006`: `panel_client_tags` + `panel_*` mirror columns | âœ… **done 2026-10-24** — also `free_claims`, `vpn_releases`, `client_storage_limits`; live DB at `schema_version: 6`, data intact |
  | **C** | Read-only panel inspector (`GET /api/servers/{id}/panel-clients`) | âœ… **done 2026-10-24** — every panel client with the panel's own limits, joined with tag/assignment; `?tag=free\|premium\|untagged`; live-verified against the real panel |
  | **D** | Tag API: list untagged, set tag (Super Admin only, audited) | Test: an untagged client is invisible to client/reseller |
  | **E** | VPN list built from tagged panel clients, showing the **panel's** limits | Test: a client created in 3X-UI appears after tagging |
  | **F** | `Create`: panel write â†’ confirm â†’ row; `ErrOfferIncomplete` on `inbound_id = 0` | Test: panel rejects â†’ zero `vpn_instances` rows |
  | **G** | Sync mirrors panel values (D-024.4) + verified absence (D-024.5) | Test: panel 10 GB â†’ 20 GB reflected; deleted client â†’ `SERVER_UNAVAILABLE` |
  | **H** | Links/QR from `GET /clients/links/{email}` (Stage E6) | A link exists only for a confirmed client |

- **Migration policy for existing rows:** rows that no panel client can be shown to hold are **not deleted** (REQUIREMENTS §3 "never hard-delete Premium"). They are reconciled on the first sync after Stage G: unconfirmable rows become `SERVER_UNAVAILABLE` and are excluded from every client- and reseller-facing list, so the invariant holds from that moment without destroying history.
- **Approval:** Owner (explicit and repeated; Act mode granted 2026-10-24: *"doc update karala patan ganna mn kalin kiwwe widihata"*).
- **Reversal:** Only by a new owner decision superseding D-024. Mirror data and the original columns are retained, so reverting the write ordering loses nothing.

---

## D-025 · Tag lifecycle, assignment locking, and FREE claim quotas

- **Date:** 2026-10-24
- **Status:** **INVARIANT — owner-mandated.** Subordinate to D-024.
- **Decision:** Three rules govern what the Super Admin's tag means over a VPN's life, and how many FREE VPNs a client may hold.

  | ID | Invariant |
  |---|---|---|
  | **D-025.1** | **Untagged = Super Admin only.** An untagged panel client is a VPN, but it is visible **only** to the Super Admin and cannot be issued, claimed, or sold. Tagging is the act that publishes it. |
  | **D-025.2** | **The tag is editable only while the VPN is unissued.** Once a VPN is assigned to a client or sold to a reseller, its FREE/PREMIUM tag is **locked** — flipping it afterwards would retroactively change what a customer paid for. Unassigned VPNs may be re-tagged freely by the Super Admin. |
  | **D-025.3** | **Only FREE is publicly claimable. Only PREMIUM is resellable.** A client may self-claim a FREE VPN from the public catalogue. A reseller may only receive/sell PREMIUM VPNs. The Super Admin may issue either directly. |
  | **D-025.4** | **Free concurrent cap = 3 per client** per server. To take a fourth they must release one first. Fixed, not configurable. |
  | **D-025.5** | **Free monthly cap = 5 claims per client per server, rolling 30 days.** The Super Admin sets the number (default **5**) globally and may lower or raise it per client. **Releasing a FREE VPN still consumes a monthly claim** — release does not refund the slot. Clients must be told this before claiming. |
  | **D-025.6** | **Premium sales are unlimited** and governed by the reseller's quota, not by a client cap. |

- **Reason:** Owner, 2026-10-24: *"client kenekta free ewa thiyaganna puluwan uparima 3k dakwa pamanayi â€¦ masekata free ewa 5kata wada eka user kenekta use karanna beri widihata hadanna â€¦ clientlata free vpn keeyak use karanna puluwanda kiyala masekata dana limit eka super adminta thiranaya karanna denna â€¦ admita sell karanne premium ewa witharayi â€¦ assing karala neththan eka super adminta thiranaya karanna puluwan â€¦ client kenekta ho adminta sel karapu assing karapu ekak aye wenas karanna ba."* Without these caps one account can drain the FREE catalogue; without locking, a tag change would silently rewrite a paid deal.

- **Data model:** `panel_client_tags` gains `locked_at INTEGER` (set on first assignment; non-NULL blocks re-tagging). New `free_claims (id, client_user_id, server_id, panel_email, claimed_at, released_at, claim_month)` supplies both the rolling-30-day count and the concurrent count without scanning `vpn_instances`.

- **Tests:** cap 3 rejects a 4th; monthly cap 5 rejects a 6th within 30 days; a release does **not** free a monthly slot; re-tagging an assigned VPN is refused; an untagged VPN is invisible to reseller and client.

---

## D-026 · Client-facing VPN detail: panel truth, live ping, usage charts, server health

- **Date:** 2026-10-24
- **Status:** **INVARIANT — owner-mandated.** The owner calls this *"the main point"*.
- **Decision:** Opening a VPN must show everything the owner listed, sourced from the panel or from measured data — **nothing invented**.

  **PREMIUM detail page** (rich):

  | Section | Content | Source |
  |---|---|---|
  | Identity | VPN name, `panel_email`, status with **themed** colour | local + panel |
  | Credential | `uuid` (vless) / password (other protocols), changeable by the owner of the VPN | panel |
  | QR + link | QR image and copyable connection link | `GET /clients/links/{email}` |
  | Validity | **Obtained date â†’ expiry date**, days remaining | panel `expiryTime` |
  | **Live ping** | A **"Test ping" button**; the measured round-trip appears immediately on click | Veylix backend â†’ panel host |
  | Usage | used / remaining, **usage-history charts** with a **7d / 30d / All time** range toggle and a user-chosen **chart type** (line / bar / area / pie) | `usage_snapshots` |
  | Server health | **CPU, RAM, network traffic** charts — each switchable, with a range toggle | `GET /server/status` + `server_metrics` |

  **FREE detail page** (deliberately simpler, per the owner): QR + link, the live-ping button, and a **7-day usage chart** only.

  **Rules:**
  - **D-026.1** — Live ping is **on demand only** (a button). There is **no speed test** (owner decision, 2026-10-24) and no background ping loop: a 1 vCPU / 500 MB host must not carry continuous synthetic traffic.
  - **D-026.2** — Every chart states its **source and window** ("this VPN", "whole server", "7 days"). Server-wide numbers are never presented as the VPN's own.
  - **D-026.3** — A chart with no data says so; it never draws a zero line implying "no traffic" (`REQUIREMENTS §2.7`).
  - **D-026.4** — Status colours come from **theme tokens**, never hard-coded, so the Theme Studio recolours them (owner: *"theme anuwa wenas karanna weyi"*).

- **Reason:** Owner, 2026-10-24: *"vpn eka clik karma detals page eka hodata wistharathmakawa thiyenna one â€¦ usage histy 7 day 30 all time widihata maru weddi chat wenas wenna one â€¦ line chat details chart pie chat nar chat e kemathi chart ekakata toggle karakara â€¦ live ping eka test karanna button ekak thiyenna one sema vpn ekakama â€¦ server ram cpu char maru karanna puluwan wenna one network traffic ekath."*

- **Non-goals:** speed test (explicitly dropped); timer-driven ping; attributing server-wide network totals to a single VPN.

---

## D-027 · Releasing a depleted VPN: 3-day grace, then local removal only

- **Date:** 2026-10-24
- **Status:** **INVARIANT — owner-mandated.** Corrects the earlier reading that "storage" meant disk space.
- **Decision:** *"Storage full"* means a **VPN's traffic quota is exhausted**, not the host's disk. The lifecycle for a depleted VPN is:

  | ID | Rule |
  |---|---|
  | **D-027.1** | When a VPN's quota is exhausted the client may **release** it. Until released it stays visible, marked depleted. |
  | **D-027.2** | A release starts a **3-day grace period** with **undo**: restoring within those 3 days cancels the auto-removal. |
  | **D-027.3** | After 3 days the system **removes the record from Veylix**. It **does not** delete the client on the panel (D-024.1; owner: *"3x ui panel eken ewa ain wenne nane"*). The panel keeps the client; the Super Admin may reuse or clear it there. |
  | **D-027.4** | A released claim **still counts** against the monthly FREE cap (D-025.5). |
  | **D-027.5** | Removal is **local and auditable**: the audit entry records the VPN, the reason (`quota_exhausted`), and that the panel client was left intact. |
  | **D-027.6** | **Chat storage is a separate, per-client limit** set by the Super Admin (both a byte budget and a message count — the Super Admin chooses). The client sees usage and may delete their own messages to free space. Chat storage never blocks VPN use. |

- **Reason:** Owner, 2026-10-24: *"storage full kiyala mn adahas kale vpn usage eka â€¦ remove karama dawas 3kin athulatha system eken auto e vpn eka ain karala danna â€¦ undo kaanna den eka â€¦ eka hida super adminta clientlata dena storage manage karanna puluwan â€¦ clintlata thamange anasaya chat delete karala storage hdaganna chance eka denna."*
- **Data model:** `vpn_releases (id, vpn_id, client_user_id, released_at, restore_deadline, restored_at, removed_at, reason)`; `chat_usage (user_id, bytes_used, messages)`; `client_storage_limits (user_id, chat_bytes_limit, chat_message_limit, set_by, set_at)`.
- **Tests:** release â†’ pending-removal; restore within 3 days cancels it; after 3 days the local row is gone, the panel client is untouched, an audit entry exists; the monthly claim slot stays consumed; chat limit blocks sending but not VPN use.

---

## D-028 · Panel email identity and FREE/PREMIUM limits

- **Date:** 2026-10-24
- **Status:** **INVARIANT — owner-mandated.**
- **Decision:** The panel client's `email` is derived from the **Veylix user's email**, and the limits are fixed per tag.

  | ID | Rule |
  |---|---|---|
  | **D-028.1** | A VPN's panel `email` is the **owning client's Veylix email**. If already taken on that panel, a numeric suffix is appended: `kasun@gmail.com`, `kasun@gmail.com-2`, `kasun@gmail.com-3`, â€¦ Monotonic per (server, email); a number is never reused. |
  | **D-028.2** | **On assignment the panel client is renamed.** Whatever name the Super Admin used at creation, assigning the VPN to a client **updates the panel client's `email`** to that client's identity via `POST /clients/update/{email}` — which **replaces** the row, so the full record is sent back rather than a patch. |
  | **D-028.3** | **FREE limits are fixed: `limitIp = 2` and `limitHwid = 2`.** The Super Admin does not choose these. |
  | **D-028.4** | **PREMIUM limits are chosen by the Super Admin** at create/assign time (`totalGB`, `expiryTime`, `limitIp`, `limitHwid`). |
  | **D-028.5** | **`limitHwid` must be sent in `clients/add`.** It was absent from `ClientSpec`, which is why hardware limits never applied on the panel (owner's observation, 2026-10-24). The panel supports the field. |
  | **D-028.6** | A client may **change their own VPN password** from the VPN detail page. For VLESS the meaningful secret is `uuid`; for password protocols it is `password`. Both are shown to the VPN's owner and editable there. |
  | **D-028.7** | **One email identifies one user**: a panel email belongs to exactly one Veylix client; a client's VPNs differ only by the numeric suffix. |

- **Reason:** Owner, 2026-10-24: *"clame karana welawe clame karana clientge emain eke namin hedenna one â€¦ 1, 2 ,3 ,4 widihata â€¦ free ewa clame karaddi device 2 ip 2 aniwaryayi â€¦ premium ewa client kenekta assing karaddi eke emai eka 3x ui clientge emal eka oya widhata rename wenna one â€¦ hardware limit eka withari set wela thibbe neththe. eka set karanna option ekak dennako."*
- **Tests:** a second claim produces `â€¦-2`; assignment renames the panel client and the rename is confirmed by a fresh read; `clients/add` carries `limitHwid`; FREE is refused unless `limitIp=2` and `limitHwid=2`.

---

## Bible deviation register
| # | Bible statement | Reality / Deviation | Reason | Ref |

|---|---|---|---|---|
| 1 | "Finish the existing app" (§0, §13.1) | No existing app; greenfield | Repo contained only the Bible | D-001, REQUIREMENTS §12 |
| 2 | Implementation sequence (§17) | Frontend-first per owner (superseded 2026-10-23) | Live-test workflow & owner preference | D-002, **D-010** |
| 3 | §17 order: inventory â†’ gap analysis â†’ architecture â†’ data/auth â†’ server integration â†’ business â†’ frontend â†’ theme â†’ release â†’ QA | Stages Aâ€“H from the new requirement (`new_update.md` §11) are used, with §17 steps folded into Stage B/C/E | The new requirement is the owner's current instruction set and defines the same progression at a finer granularity | D-010, `docs/IMPLEMENTATION_PLAN.md` |
| 4 | §4 "Every authorization rule must be enforced by the backend" | Currently **not** enforced anywhere (no backend) — recorded as F-007, P1 | Backend not built yet | F-007, GAP_ANALYSIS §4 |
| 5 | §0.11 "Frontend assets built in CI, not on the server" | Honoured — no Node.js production dependency is introduced; SPA is embedded in the Go binary | 500 MB constraint | D-013, plan Stage E7 |

---

## Open questions requiring owner input
| # | Question | Blocks | Default if unanswered |
|---|---|---|---|
| Q-1 | Which 3X-UI release runs on the first real panel? | exact capability gating (Stage E3) | assume v3.10.2 and detect at runtime |
| Q-2 | Is a test panel + API token available for Stage G? | all live verification | mark everything "implemented but not live-verified" |
| Q-3 | Google OAuth client ID/secret for server-side token verification | Stage E2 Google login | ship email/password first, Google second |


---

## D-030 Ã‚· Plan ownership: an admin's own plan needs no approval; an unowned premium plan is dead until the Super Admin releases it

- **Date:** 2026-10-25
- **Status:** **INVARIANT — owner-mandated.** Subordinate to D-024 (a VPN is a 3X-UI client).
- **Decision:** A PREMIUM **plan** (a `vpn_offers` row) has an **owner** — the Super Admin or a reseller. Ownership, not the tag, decides who may turn a plan into a VPN.

  | ID | Invariant |
  |---|---|---|
  | **D-030.1** | **A plan has exactly one owner** (`vpn_offers.owner_user_id`). A Super-Admin plan with no reseller owner is **unowned inventory**. |
  | **D-030.2** | **A reseller releases his own plan with no approval step.** A plan he bought/owns on a server assigned to him may be turned into a VPN for any of his clients, immediately. No Super-Admin confirmation exists on that path. |
  | **D-030.3** | **An unowned PREMIUM plan can never be used without the Super Admin.** Any release against it answers `403 plan_not_approved` until the Super Admin confirms the payment. It must never be possible to obtain such a VPN another way. |
  | **D-030.4** | **A plan is a finite resource.** Releasing a plan consumes one slot (`vpn_offers.slots_total - slots_used`), decremented **inside the provisioning transaction** so two concurrent releases cannot take the last slot. A plan with 0 remaining slots is refused (`409 plan_exhausted`). |
  | **D-030.5** | **Only the Super Admin creates, prices, archives or deletes plans.** An admin acquires slots from an SA plan (`vpn_plan_purchases`); that purchase is a Super-Admin-confirmed order like any other payment. |
  | **D-030.6** | **A plan that has ever produced a VPN, or that has ever been sold as slots, is archived, never deleted** (`vpn_offers.archived_at`). Only a plan with no dependent row is hard-deleted. |

- **Reason:** Owner, 2026-10-25: *"admin buy karapu premium ekak nam eyata super admin gen awasara ganna one na, eyata kemathi deyak ekata karanna puluwan. anek premium ekiyanne admin kenek buy karala nethi premium — super admin confirm karanne nethuwa katawath use karanna beri wenna one."* Two rules fall out: ownership grants autonomy, and an unowned plan must be a hard gate — not a soft "pending" state that something else could bypass.
- **Why a plan row and not just a payment:** a payment says money arrived; a plan row says what may be built. Separating them is what lets one confirmed payment buy *N* slots, and what lets the panel limits be fixed before money is involved.
- **Data model:** `vpn_offers` gains `owner_user_id`, `price_cents`, `currency`, `slots_total`, `slots_used`, `archived_at`. New table `vpn_plan_purchases`.
- **Tests:** an admin with 0 slots is refused; an admin with 1 slot releases one and is then refused a second; an unowned plan answers `403 plan_not_approved`; deleting an in-use plan answers `409 plan_in_use`; two concurrent releases of the last slot produce exactly one VPN.
- **Approval:** Owner (explicit, 2026-10-25).
- **Reversal:** stop reading `owner_user_id` and treat every plan as unowned (SA-gated). No data is destroyed by the reversal.

## D-031 · One payment confirmation, and it *is* the release: the Super Admin confirms money and the VPN is built in the same step

- **Date:** 2026-10-25
- **Status:** **INVARIANT — owner-mandated.**
- **Decision:** Money and provisioning are one action, not two screens.

  | ID | Invariant |
  |---|---|---|
  | **D-031.1** | A client (or a reseller buying slots) **records an order**: plan + payment method + reference + the amount **the server computed from the plan**. The payer never prices the product. |
  | **D-031.2** | The Super Admin (or the plan's owner, per D-030) sees the order in **one confirmation centre** and either **confirms** or **rejects** it. |
  | **D-031.3** | **Confirming a payment builds the VPN inside the same transaction** — payment `CONFIRMED`, plan slot consumed, panel client written, confirmed by a fresh read, links fetched. If any step fails nothing is final: the payment returns to `PENDING` and the answer names the reason. |
  | **D-031.4** | A confirmed order answers with the **VPN id, the panel links and the QR payload**, so one screen can hand the customer a working configuration. |
  | **D-031.5** | **Rejects and refusals are audited and notified** to the payer, with the reason. A silently dropped order is a defect. |
  | **D-031.6** | The **existing manual settlement states stay legal** (`PENDING â†’ AWAITING_CONFIRMATION â†’ CONFIRMED/REJECTED/REFUNDED`), because cash sales happen before any reference exists. |

- **Reason:** Owner, 2026-10-25: *"cilent or admin order eka danwa, super adminta thama confirm karanna thiyenne. payment eka wetilada kiyala balala super admin premium eka release karayi. super admin confirm karama premium vpn eka active wenawa, vpn link eka, qr eka."* A confirm button that only flips a status is a lie: the customer has paid and has nothing.
- **Non-goals:** automated gateway callbacks (D-007 keeps gateways out of v1); partial refunds of plan slots.
- **Related:** `store.ConfirmPayment` today only rewrites `payment_records.status` — that is the defect D-031 closes.
- **Tests:** confirm â†’ one `vpn_instances` row + `ACTIVE` + non-empty links; panel failure â†’ payment back to `PENDING`, no VPN row, no slot consumed; reject â†’ audited + notified, no VPN.
- **Approval:** Owner (explicit, 2026-10-25).

---

## D-032 · Free and Premium are separated on the surface, and the plan is on the wire

- **Date:** 2026-10-25
- **Status:** INVARIANT — owner-mandated.
- **Decision:**
  - **D-032.1** — `GET /api/vpns` returns each VPN's **classification** (`tag` = FREE/PREMIUM, `offer_id`, `offer_kind`, `plan_name`), so a screen can separate Free from Premium without inferring it from a name.
  - **D-032.2** — The client surface has **two distinct sections**: a public **Free** catalogue (claimable, capped by D-025.4/.5) and **Premium plans** (priced, ordered, Super-Admin-confirmed per D-031). Neither is shown inside the other.
  - **D-032.3** — A client may read **Premium plan facts** (name, price, traffic, duration, devices, server label) but never premium ownership data — who owns which plan, which reseller sells it, or another customer's purchase.
  - **D-032.4** — Every VPN row a client sees shows its **plan name and classification**, so "what did I pay for" is answerable from the row itself.
- **Reason:** Owner, 2026-10-25: *"client lara premium free wenama clearwa bala ganna thiyenna one."*
- **Implementation note:** `store.OffersVisible` currently returns **active FREE only** to a client (`kind = 'FREE'`), and `httpapi.VPNRow` carries no offer field, so today the UI *cannot* separate them — this decision is what changes that.
- **Tests:** `/api/plans/premium` returns plan facts to a client and no ownership fields; a VPN row carries its `tag`/`offer_kind`; an admin's `vpn_count` counts only his own sales.

---

## D-033 · Every VPN ships with a QR code; the QR is generated locally, never outsourced

- **Date:** 2026-10-25
- **Status:** INVARIANT — owner-mandated. Extends D-026 (connection material comes from the panel).
- **Decision:**
  - **D-033.1** — The QR encodes **exactly** the URI returned by `GET /panel/api/clients/links/{email}`. It is never synthesised from a template.
  - **D-033.2** — The QR is rendered **client-side from an in-repo SVG encoder** — no npm dependency, no external QR service. A credential must never be sent to a third party to be drawn (the same rule that keeps the panel token server-side).
  - **D-033.3** — Every VPN gets a QR: FREE and PREMIUM, and on every role's detail screen.
  - **D-033.4** — A QR is shown only when `source = panel` and at least one link exists. An outage shows "could not be read", never an empty or placeholder QR.
- **Reason:** Owner, 2026-10-25: *"free ewa qr eka na"* — a QR is the only practical way a phone user connects, and it was missing for every VPN.
- **Tests:** the encoder round-trips a vless URI against a decoder; a `source: 'unavailable'` payload renders no QR.

## D-022 · Development auth bypass (UI review mode), fail-closed by construction

> **SUPERSEDED 2026-10-28 by D-039.** The bypass was deleted from the source, not
> disabled; this record is kept for history. Sign-in is password + Google now —
> see `docs/AUTH.md`.

- **Date:** 2026-10-23
- **Decision:** Local development may run with authentication bypassed so every screen (including role-guarded ones) can be reviewed before the business rules behind them are finished. Two independent, explicitly-flagged switches:
  - **Frontend:** `VITE_DEV_AUTH=1` supplied by `frontend/.env.development`. It only takes effect when `import.meta.env.DEV` is also true, so `vite build` removes the entire branch.
  - **Backend:** `VEYLIX_DEV_AUTH=1`, honoured only when the bind address is **loopback** *and* `VEYLIX_INSECURE_COOKIES=1` is set (`config.resolveDevAuth`). All three conditions are required and unit-tested; a network-reachable bind can never enable it.
- **What the bypass does:** the frontend boots straight into one of three personas (super-admin / admin / client) and shows a permanent **`DEV · auto-login`** chip in the topbar; the backend injects `httpapi.DevPrincipal()` when a request has no valid session.
- **Amendment 2026-10-28 — the backend follows the persona shown.** The bypass used to inject a super admin whatever the UI displayed, so the "admin" chip passed every super-admin check: a reseller's 403 could not be observed from the dev build, and "I tested it as admin" was false. Now:
  - **D-022.1** — the frontend sends `X-Veylix-Dev-Role: super_admin|admin|client` (`auth.DevRoleHeader`) with every API call, only in a dev build (`DEV_AUTH_BYPASS`).
  - **D-022.2** — `httpapi.DevPrincipalFor(role)` builds the dev principal for that role; an unknown/absent value falls back to `super_admin` (the old behaviour, never an error).
  - **D-022.3** — honouring the header needs a **second, stricter switch**: `VEYLIX_DEV_ROLE_SWITCH=1`, ANDed with `config.DevAuth` (`config.Load`), so a network bind or a missing bypass flag can never enable it. Off by default for a bare `go run`; `dev.ps1` turns it on (documented in `dev-env.ps1`).
  - **D-022.4** — a request that carries **any** session cookie ignores the header; an invalid cookie also gets the default dev principal, never the requested role. A real `/login` session therefore always wins.
  - **D-022.5** — `/api/me` reports `dev_impersonating: true` when the identity is the dev principal, so the UI (dev chip) and `npm run preflight` can prove which identity the API used.
  - **D-022.6** — the dev login is **scaffolding, removed at release**: `.\dev.ps1 -DisableDevAuth` flips the flags, rebuilds, and fails unless the built bundle contains **zero** dev markers and an anonymous `/api/me` answers **401**. For that grep to be meaningful, no dev-identifiable string may survive minification — the dev store method is therefore named `previewAs`, not `devSwitchRole` (the old name was found in the built bundle).
- **What the bypass does NOT do:** it does not remove authorization. `RequirePermission`, ownership checks and every service-layer rule still run, and every write is still audited under the dev principal. A **real `/login` session always takes precedence**, so the login flow stays fully testable.
- **Reason:** Owner request — screens must be reviewable without a login step while features are still being built, but the login system itself is finished and live-verified, so it must not be deleted or disabled.
- **Verification:** production bundle contains **zero** bypass logic (personas, localStorage key, flag read, header name and branch all tree-shaken). D-039 later **deleted** the bypass entirely — see `docs/AUTH.md` §7. With `VEYLIX_DEV_AUTH=0` the same routes return **401** again. Evidence recorded in `docs/AUTH.md`.
- **Approval:** Owner (explicit request, option B).
- **Reversal:** set `VITE_DEV_AUTH=0` and `VEYLIX_DEV_AUTH=0` — no code change, no rebuild of logic, full login restored. `.\dev.ps1 -DisableDevAuth` does exactly that, rebuilds, and proves both facts.
## D-029 · Single binary: the built SPA is embedded in the Go server

- **Date:** 2026-10-24
- **Status:** **INVARIANT — owner-mandated** ("software eka hariya build wenna one").
- **Decision:** `go build ./cmd/veylixd` produces **one artifact** that serves both the API and the user interface on one port. The Vite bundle is copied into `backend/internal/httpapi/dist/` and compiled in with `//go:embed all:dist`.
- **Why this and not "two services":** before D-029 the backend answered **404 for `/dashboard`**, so installing Veylix meant running Go, Node/Vite and a reverse proxy, with no documented path. A single binary is what makes `docs/INSTALL.md` a true statement rather than an aspiration, and it removes CORS/origin mismatches because the UI and API share an origin.
- **Routing rules (each has a test):**
  - `/api/*` is matched first and keeps the **JSON** 404 envelope — the SPA fallback can never swallow an API path (`TestRouterKeepsApi404AsJSON`).
  - A real file is served as itself; anything under `assets/` is cached `immutable` for a year because Vite hashes the filename (`TestSPAServesAssetsImmutable`).
  - A **missing file** stays **404** (`/assets/nope.js` â†’ 404) — returning `index.html` for a missing script produces "Unexpected token '<'" instead of an honest error (`TestSPAMissingFileIs404`).
  - Any other path serves `index.html`, so deep links like `/vpn/16` survive a hard refresh (`TestSPADeepLinkServesIndex`).
  - `HEAD` answers with no body; non-GET to a UI path is **405**.
- **Absence is stated, never faked:** the directory is committed with only `.gitkeep`, so `go build ./...` works on a fresh clone. With no build present, `GET /api/ready` reports `"ui": "not_embedded"` and UI paths answer a JSON 404 naming the missing step. Once a build is embedded, it reports `"ui": "embedded:N_files"`. `index.html` decides this, not a file count (`TestSPAStatusRequiresIndex`).
- **Build path:** `.\dev.ps1 -Package` — builds the SPA, **wipes** the embed target first (so no stale hashed asset can linger), copies the build, builds `backend/bin/veylixd.exe`, then runs that binary on a throwaway port and asserts `/api/ready`, `/dashboard`, `/vpn/16` and the 404 rule, and greps the bundle for the dev bypass (D-019).
- **Generated files are not committed:** `.gitignore` excludes `backend/internal/httpapi/dist/*` except `.gitkeep`.
- **Approval:** Owner (explicit request).
- **Reversal:** delete the copy step and mount the SPA from a CDN/proxy instead; the handler in `internal/httpapi/spa.go` is the only place that changes.

---

## D-034 · Panel identity is the client's e-mail plus a per-kind slot

- **Date:** 2026-10-25
- **Status:** **INVARIANT — owner-mandated.** Refines D-012 (idempotency pair) and D-028 (panel email identity).
- **Decision:** A 3X-UI client's `email` is no longer an opaque Veylix id. It is derived from the human who owns it, so an operator looking at the panel can see whose VPN it is.

  | ID | Invariant |
  |---|---|---|
  | **D-034.1** | `panel_email = <client account email>_<kind>_<n>`, where `kind` is `free` or `paid`. Examples: `nimal@gmail.com_free_1`, `nimal@gmail.com_paid_1`. |
  | **D-034.2** | `<n>` is a **per-(client, server, kind) counter** that only moves forward. A revoked slot is never re-issued: if `_paid_1` and `_paid_2` existed and `_paid_2` is revoked, the next paid VPN is `_paid_3`. |
  | **D-034.3** | `panel_sub_id` stays `vyx-<vpn_instance_id>` — deterministic, so D-012's idempotency survives a retry even though the e-mail is now human-readable. |
  | **D-034.4** | The e-mail is assigned **at creation**, never guessed afterwards: a FREE claim and a PREMIUM order both reserve their slot inside the provisioning transaction. |
  | **D-034.5** | A panel client created directly in 3X-UI may be **renamed once** when the Super Admin assigns it to a client (D-037), so it joins the same naming scheme. The previous name is audited. |

- **Reason:** Owner, 2026-10-25: *"free ewa hadaddi vyx-21 kiyala 3x ui eke email name eka wetla thiyenne ehema ba, free ofer eka clame karana kenage email eka enna one, premium eketh ehemayi â€¦ premium ewa email_paid_1, email_paid_2 â€¦ custom ekath ehemayi"*.
- **Consequence (accepted):** one panel client per (client, server, kind, slot). The panel's e-mail key is unique, which is exactly what makes the slot counter necessary.
- **Tests:** `TestPanelEmailIsClientAndKind`, `TestPanelSlotsOnlyMoveForward`, `TestPanelSubIdStaysDeterministic`.

---

## D-035 · Three package kinds: FREE, PREMIUM (owned), CUSTOM PREMIUM (Super Admin only)

- **Date:** 2026-10-25
- **Status:** **INVARIANT — owner-mandated.** Extends D-030 (plan ownership).
- **Decision:** The catalogue has exactly three kinds, and who may own and confirm an order follows from the kind.

  | Kind | Created by | Limits | Who confirms the order |
  |---|---|---|---|
  | **FREE** | Super Admin | fixed by platform policy (2 IP / 2 HWID, D-028.3) | nobody — the claim is instant and capped by D-025.4/.5 |
  | **PREMIUM** | Super Admin **or** a reseller | the package's own fields — exactly like a FREE template, the customer cannot edit them | **the package's owner**: the SA for a platform package, that reseller for his own |
  | **CUSTOM PREMIUM** | Super Admin **only** | the customer states traffic (GB), duration (days), devices and IPs; the price is computed from the SA's rates (D-035.4) | **the Super Admin only** |

  | ID | Invariant |
  |---|---|---|
  | **D-035.1** | `PREMIUM` packages work like FREE templates: fixed limits, fixed price, no per-order editing. |
  | **D-035.2** | A `CUSTOM PREMIUM` order carries a **customer-authored spec**; no reseller can own a custom package (owner: *"adminlata custom premium kiyala ekak na"*). |
  | **D-035.3** | A `CUSTOM PREMIUM` order is **not** releasable by anyone but the Super Admin, who chooses the **server** and the **inbound** at release time (D-036.6). |
  | **D-035.4** | Price = `traffic_gb Ã— rate_per_gb + devices Ã— rate_per_device + ips Ã— rate_per_ip + days Ã— rate_per_day`, using the SA's rates — a **global default with a per-server override** (owner's Q1 answer). The server computes it; the client never states an amount. |

- **Reason:** Owner, 2026-10-25: *"free package, premium package, custom premium package â€¦ custom premium wala client ta one widihata thora gena VPN ekak order karanna puluwan â€¦ custom eka super adminta witharayi enne â€¦ SA server ekak select karala inbound ekak select karala e order ekata package eka client ta release karanna puluwan"*.
- **Tests:** `TestQuoteFromRates`, `TestServerRateOverridesGlobal`, `TestResellerCannotOwnCustom`, `TestPremiumPackageLimitsAreFixed`.



---

## D-036 · An order is manual, channel-paid, blockable, and always attributable

- **Date:** 2026-10-25
- **Status:** **INVARIANT — owner-mandated.** Extends D-031.
- **Decision:** The confirmation screen must answer, without ambiguity, **who ordered, how they paid, where the money came from, and who receives it.**

  | ID | Invariant |
  |---|---|---|
  | **D-036.1** | Every order records: buyer (account + display name), package, spec, amount, currency, the chosen payment method, and the **owner who must confirm it**. |
  | **D-036.2** | Payment methods carry the receiver's contact channels — **e-mail, WhatsApp, Telegram** — taken from the SA's or the reseller's own method row, so the payer always knows where to send proof. |
  | **D-036.3** | Proof of payment travels over **WhatsApp / Telegram / e-mail** by the payer. Veylix records the chosen channel, not a file upload. |
  | **D-036.4** | The payer may **change the payment method** before confirmation; the change is recorded as an immutable event and surfaced on the owner's order screen, never overwriting the previous choice. |
  | **D-036.5** | The owner (SA, or the package's reseller) may **block** a VPN when payment does not arrive or matches nothing, and **unblock** it once the issue is resolved. Both actions are audited and notified. |
  | **D-036.6** | A `CUSTOM PREMIUM` order is released only after the owner picks a **server** and an **inbound**; the release then creates the panel client under the customer's e-mail (D-034). |

- **Reason:** Owner, 2026-10-25: *"order eka demmana super adminta order eka yanna one, customer premium eke package wala nam eka dapu adminta ho super adminta â€¦ order eka demme kauda kiyana eka pehediliwa note wenna one, eyage payment method eka wetennath one, eyaa payment enne kohenda kagenda kiyana eka clear wima thama main aramuna â€¦ payment aulak thibboth eyala VPN eka block karayine, aula wisa dunama aye unblock karawe"*.
- **Tests:** `TestMethodChangeIsRecorded`, `TestBlockAndUnblockVPN`, `TestCustomOrderNeedsServerBeforeRelease`, `TestOrderNamesTheConfirmer`.

---

## D-037 · A panel client Veylix has never seen can be assigned to a client

- **Date:** 2026-10-25
- **Status:** **INVARIANT — owner-mandated.**
- **Decision:**
  - **D-037.1** — Opening a server shows **every** panel client, split into *Assigned* and **Unassigned** (created directly in 3X-UI, or one Veylix has no owner for).
  - **D-037.2** — The Super Admin may **assign an unassigned panel client to any client account**, choosing the kind (FREE/PREMIUM). The assignment renames the panel client per D-034.5 and locks its tag.
  - **D-037.3** — An assigned client behaves as a normal PREMIUM VPN: every feature of D-038 applies to it.
  - **D-037.4** — **A client's list never contains another client's VPN.** Every row carries its owner; the client endpoint filters by owner in SQL, never in the browser.
- **Reason:** Owner, 2026-10-25: *"inbound eke thiyena hema vpn ekakma client lage load karanna epa, e clientge vpn witharai â€¦ 3x ui eken direct hadapu vpn clientla â€¦ kemathi kenekta assign karanna puluwan wenna one â€¦ assign karata passe eka e adala clientge premium VPN ekak hetiyata pennanna one"*.
- **Note:** this decision records a **defect** as well as a rule — `PanelVPNLister` returned every panel client to the Super Admin's `/api/vpns` with no owner field, so "whose VPN is this" was unanswerable on the wire.
- **Tests:** `TestVPNServerListSplitsAssignedAndUnassigned`, `TestAssignUnassignedClientToUser`, `TestClientListNeverLeaksAnotherClient`.

---

## D-038 · The Premium VPN page is a fixed feature set — none of it optional

- **Date:** 2026-10-25
- **Status:** **INVARIANT — owner-mandated.** The owner calls these *"à¶…à¶±à·’à·€à·à¶»à·Šà¶º requirements"*.
- **Decision:** Opening a PREMIUM VPN must show all of the following. A screen that omits one is incomplete, not "simplified".

  | ID | Feature |
  |---|---|---|
  | **P-01** | Usage history from the VPN's creation day to its end date, with **upload, download and total shown separately**. |
  | **P-02** | The VPN's own usage against its capacity (used / limit, percentage). |
  | **P-03** | Its server's live **CPU and RAM**. |
  | **P-04** | **Live traffic** for this VPN — e.g. "â†‘ 2 kbps / â†“ 40 kB/s" — labelled as a rate, never as a total. |
  | **P-05** | **Ping measured from the customer's own browser to the VPN server** (owner: Option 1). A server-side ping is never presented as the customer's latency. |
  | **P-06** | Range switch: **7 days / 30 days / 3 months / 1 year**. |
  | **P-07** | CPU, RAM and network charts, **each independently switchable**. |
  | **P-08** | Chart type switchable by the customer: **pie / line / bar / area**. |
  | **P-09** | Protocol detail: **vless / vmess**, transport **TCP / WS**, and the credential material (uuid / username / password) for the protocol in use. |
  | **P-10** | **QR code and the connection link — both, always.** |

- **Reason:** Owner, 2026-10-25: *"usage history â€¦ hadapu dawase idan end wena dawasa dakwa traffic usage wagua, upload download ha total traffic wena wenama â€¦ server details ram cpu, ewele yana current traffic up 2kbps down 40kbps wage â€¦ ping eka test karaddi e app eke ewele ping eka, methanadi server eke ping eka denna epa â€¦ 7 day 30 day 3 month 1 year maru kara â€¦ pie chart / bar chart â€¦ vless vmess, protocol eka mokadda tcp ws, username password â€¦ QR eka wagema VPN link ekath thiyenna one, aniwaaryen premium vpn page eke thiyenna one fetuers"*.
- **Boundary:** P-05 cannot be a raw TCP ping from a browser. It is a handshake round-trip to the VPN server's endpoint, and the UI states that scope (D-026.2 — a number always names its source).
- **Tests:** `TestUsageHistorySplitsUpDownTotal`, `TestUsageRangeWindow`, `TestProtocolDetailIsExposed`, `TestPingScopeIsClientNotPanel`.


## D-039 · The dev login is removed; authentication is Google + password, server-side

- **Date:** 2026-10-28 (owner: *"real log in eka enable karanna dev log in eka completly ain karanna â€¦ google sing up ekath hariyata thiyenna one"*).
- **Decision:** D-022 (the development auth bypass) is **deleted**, not disabled. No code path, flag, header or persona grants an identity without a session. Authentication is now:
  - **email + password** (`POST /api/auth/login`, HttpOnly session cookie, 12h);
  - **Google** (authorization-code flow with PKCE, exchanged **server-side**), for both sign-in and sign-up.
- **D-039.1 — The bypass is gone from the source.** `DevPrincipal`, `DevPrincipalFor`, `RequireAuthOrDev`, `auth.DevRoleHeader`, `config.DevAuth`/`DevRoleSwitch`/`resolveDevAuth`, `frontend/lib/devMode.ts`, `components/dev/*`, `dev_impersonating` and every `VITE_DEV_AUTH`/`VEYLIX_DEV_AUTH` reference were removed. A grep for those names in `backend/` and `frontend/src/` returns **0 hits**.
- **D-039.2 — Google is verified, never trusted.** The browser is redirected to Google and back with a `code`; the server exchanges it (client secret + PKCE verifier) and verifies the **ID token**: RS256 signature against Google's JWKS, `iss âˆˆ {accounts.google.com, https://accounts.google.com}`, `aud == client id`, `exp`/`iat`, and the `nonce` this server issued. `alg: none` and HS* are refused. No access token ever reaches JavaScript.
- **D-039.3 — The linking rules are the security model.** After verification:
  1. Google subject already linked â†’ sign in as that user;
  2. e-mail exists and Google reports it **verified** â†’ link it to that account (`users.google_sub` + `auth_identities`);
  3. e-mail exists and Google reports it **unverified** â†’ **refuse** (the account-takeover path);
  4. unknown e-mail â†’ create a **client** account (`password_hash` NULL, `email_verified` 1). A Google sign-up can never choose a role.
- **D-039.4 — CSRF and replay are closed by construction.** Each login stores a single-use `state` (with PKCE verifier and nonce) in `oauth_states`; the callback `DELETE â€¦ RETURNING`s it, so a replayed or never-issued callback is refused before any token work happens.
- **D-039.5 — No credentials, no button.** `GET /api/auth/google/status` reports `{configured}`; the UI disables the button with a reason; `/api/auth/google/start` answers **503 `google_not_configured`** (never a 404, never a fake success).
- **D-039.6 — E-mail verification exists at last.** `Register` wrote `email_verified = 0` and nothing could ever set it. Now `POST /api/auth/verify/request` issues a single-use 24h token and `GET /api/auth/verify/{token}` redeems it (replay â†’ `verified=invalid`). SMTP is not wired yet, so the token is returned **only to a loopback caller**, and `cmd/accounttool -verify <email>` covers a manual check.
- **Migration:** `0012_google_identity.sql` (purely additive): `auth_identities`, `oauth_states`, `email_verifications`.
- **Tests:** `internal/auth/google` — a valid token is accepted; wrong audience, wrong issuer, expired, future `iat`, nonce mismatch, no subject, no e-mail, unknown signing key, `alg:none`/HS256 and non-JWT are all refused; an unconfigured client fails closed; the PKCE challenge is S256(verifier). `internal/auth` — the three linking rules including the **unverified-address refusal**, a second login does not create an account, the state is single-use, and it fails closed without credentials.
- **Live verification:** `/api/auth/google/status` â†’ `{configured:false}`; `/api/auth/google/start` â†’ **503**; register â†’ `email_verified=false`; redeem â†’ DB row `1|ACTIVE`; replay â†’ `303 â€¦?verified=invalid`; `npm run preflight` â†’ **27/27**; `go test ./...` â†’ 11/11 packages.
- **A real defect found by the new tests:** the verifier passed `0` instead of `crypto.SHA256` to `rsa.VerifyPKCS1v15`, so **every genuine Google token would have been rejected**. Fixed before release; `TestVerifyIDTokenAcceptsAValidToken` is the test that caught it.
- **Reversal:** none needed — there is no bypass left to re-enable. Without Google credentials, use the seeded password accounts.

## D-040 · Chat data is purgeable, with a backup and an audit row

- **Date:** 2026-10-28 (owner: *"denata chat eke data clean karanna"*).
- **Decision:** chat content is test/support data and may be discarded deliberately, but never silently:
  - `cmd/accounttool -purge-chat` (or `.\dev.ps1 -PurgeChat`) copies the database file first, prints the row counts per table, deletes the six chat tables childâ†’parent inside one transaction, and writes an audit row (`chat.purge`) carrying the counts;
  - `-dry-run` reports exactly what would be deleted;
  - users, VPNs, payments, offers and the audit log are never touched.
- **Why a backup here:** the same command sits next to production data, so the wrong click must be reversible. A file copy costs nothing.
- **Result on this database:** 44 messages, 2 conversations, 4 participants and 24 flag rows removed; `users` 5, `vpn_instances` 6, `payment_records` 3 unchanged; backup `veylix.pre-purge-20261028-204638.db`.
- **Verification:** dry run and real run both executed; counts cross-checked with `sqlite3`; the deletion runs in a transaction with its audit row.

## D-041 · One sign-in page per audience, and a code by e-mail to reset

- **Date:** 2026-10-28 (owner: *"admin portal ekatada client portal ekatada kiyala adahha"*, *"forget password eka difanmax@gmail.com eka ta danna"*).
- **Decision — two doors, decided by the server.** `POST /api/auth/login` takes `portal: "client" | "staff"`. A client hitting the staff door gets **403 `wrong_portal`**, and an admin or super admin hitting the client door gets the same code in the other direction. The UI picks which door to *show*; the server decides who is *allowed*. `/login` is the client door, `/staff/login` is the staff door, and `/admin/login` + `/super/login` are shortcuts to it.
- **Why not one door:** a single form that "figures out" the role leaks the role of any address to whoever types it, and it makes a typo look like a wrong password. Two doors make the audience explicit and keep the refusal unambiguous.
- **Only a super admin can create staff.** `Register` and the Google flow force `role = client`; the staff routes are `super_admin` only. There is no path from self-service sign-up to admin access.
- **`identifier`, not `email`:** login accepts the **e-mail address or the username** (`users.username`, unique, case-insensitive, 3â€“32 chars of `[a-z0-9._-]`). `username` is backfilled from the address local-part on migration, and `Register` allocates a free one.
- **Reset is a 6-digit code, never a link.** `POST /api/auth/forgot` always answers **202** (an unknown address is indistinguishable from a known one) and mails a code stored **hashed** with a 10-minute TTL, 5 attempts, and single use. `POST /api/auth/reset` redeems it and revokes every session for that account.
- **Who receives what:** a **super admin** reset or password change is copied to `VEYLIX_SUPPORT_EMAIL` (`difanmax@gmail.com`); a client or admin reset goes to **their own** verified address. The sender is always `VEYLIX_MAIL_FROM`.
- **Delivery is never faked.** With no SMTP settings the sender is nil and the API reports `delivery: "outbox"` rather than claiming an e-mail went out — the one rule that keeps "we mailed you" honest.
- **Migration:** `0013_login_profile.sql` (username + profile columns + `free_vpn_limit` + `last_login_*`), `0014_profile_email_change.sql` (e-mail change codes).
- **Verification:** `go test ./...` 12/12 packages; live matrix — owner/staff **200**, owner/client **403 `wrong_portal`**, admin/staff **200**, admin/client **403**, client/client **200**, client/staff **403**; wrong password **401**; sixth attempt **429**; unverified account refused with `403 email_unverified`.

## D-042 · A profile edits everything except the two things that are identities

- **Date:** 2026-10-28 (owner: *"email eka eka unic hinda eka gannaâ€¦ anek siyalla change karanna puluwan wenna one"*).
- **Decision:** every account — client, admin, super admin — may change its own **display name, username, age, bio, timezone, avatar URL, phone, WhatsApp, Telegram and Discord**. Two things stay outside self-service:
  - **`email`** is the login identity and must stay unique. A change is requested with the **current password**, confirmed by a code sent to the **new address only**, and on success updates `email`, sets `email_verified = 1`, and **revokes every session** (including the one that asked) so a stale cookie cannot outlive the identity change.
  - **`role`, `status`, `quota_limit`, `free_vpn_limit`** are never writable through `/api/me/*`. They belong to the super admin console; letting a row's owner set them would be privilege escalation.
- **Avatar is a URL, whitelisted to `http`/`https`.** `javascript:` and `data:` are refused, because an avatar is rendered in `<img src>` and in the console.
- **Verification:** live — 9 profile fields saved and read back; `username: owner` â†’ **409 `username_taken`**; `username: ab` â†’ **422**; `avatar: javascript:â€¦` â†’ **422**; `email` in the profile PATCH â†’ **400** and unchanged; e-mail change with a wrong password â†’ **422 `password_mismatch`**; wrong code â†’ **422**; correct code â†’ **200**, new address signs in **200**, old address **401**, code replay **401**.

## D-043 · CSRF refuses a browser that cannot name its origin, and the cookie is Strict

- **Date:** 2026-10-29 (owner: *"security issues bugs error case kisiwama nethi bawath hariyatama thahauru karanna"*).
- **The gap that was closed:** `SameOrigin` let **any** request through when `Origin` was absent. Combined with `SameSite=Lax` — which still sends the cookie on a cross-site *top-level navigation* — a cross-site form POST could carry the session into a state-changing route.
- **Decision, two parts:**
  1. The session cookie is **`SameSite=Strict`**. Nothing needs a cross-site cookie: the SPA is same-origin and the Google callback returns to our own origin, which is same-site.
  2. A state-changing request with **no `Origin`** is refused **if it looks like a browser** — it carries `Sec-Fetch-Site`, `Sec-Fetch-Mode` or `Referer`, none of which page script can suppress. A request with none of those is a script/CLI client and is allowed, because it carries no ambient cookie by default.
  - Allowed origins are the request's own `scheme://Host` plus any configured `Deps.PublicOrigins`, so a proxy need not rewrite headers.
- **`X-Forwarded-Proto` is believed only from a loopback peer.** Otherwise a direct caller could claim `https` and steer the origin comparison. Same rule as `ClientIP`, which already ignored `X-Forwarded-For` from a non-loopback peer — that rule is now stated and tested rather than implied.
- **Expired sessions are purged hourly** on the existing maintenance ticker (`PurgeExpired`), beside the release purge.
- **A real consequence the tests found:** the harnesses sign in with Node `fetch`, which sends `Sec-Fetch-*` on its own, so they were correctly treated as browser POSTs with no `Origin` and answered **403 `csrf_blocked`** — 21 of 30 preflight checks failed until the harness declared `Origin` as a same-origin page does. **The server was right and the tests were wrong**; `scripts/auth-login.mjs` and `scripts/preflight-dev.mjs` were fixed, not the guard.
- **Verification:** `internal/auth` — 8 `ClientIP` cases (direct peer, spoofed header from a non-loopback peer, first-entry parsing, IPv6 loopback, garbage fallback), `TrustForwardedProto`, and the cookie's `Strict`/`HttpOnly`/`Secure` flags. `internal/httpapi` — same-origin **204**, cross-origin **403**, browser POST with no `Origin` **403**, `Referer`-only **403**, scriptless client **204**, `GET` never blocked, configured public origin **204**; `schemeHost` ignores `X-Forwarded-Proto` from a non-loopback peer. Live: **15/15** security checks, preflight **33/33**, and a real Chrome sign-in renders `/dashboard` with **0 console errors** under `Strict`.

## D-044 · A reset code is issued by the server, and printed by the operator when there is no mail relay

- **Date:** 2026-10-29 (owner: *"forget password eka difanmax@gmail.com eka ta danna"*, *"client lata adminlata adala email gmail dila ewata code yawala reset karaganna puluwan widihata hadanna"*).
- **Decision:** `POST /api/auth/forgot` issues a **6-digit code** (migration `0015_password_reset_codes`) and `POST /api/auth/reset` redeems it. The recipient is the account's own address, and a **super admin's** code is additionally copied to `VEYLIX_SUPPORT_EMAIL` so the highest-privilege account stays recoverable. `forgot` always answers **202** with an identical body, and never puts the code in the response.
- **The code is stored the same way an e-mail-change code is:** `argon2id` hash over `code + "|veylix-verification-code|"`, 10-minute TTL, **5 attempts**, single use. Consumption and the password write happen in **one transaction** together with a session purge, so a crash cannot leave a live code next to a changed password.
- **No relay, no lie.** With `VEYLIX_SMTP_*` unset the sender is nil, the audit row says `delivery: "outbox"`, and nothing claims an e-mail was sent.
- **`.\dev.ps1 -ResetLink <email|username>`** (`accounttool -reset-link`) issues a code and prints it for a loopback operator. It is the same store call the API makes and the same salt, so a code printed here redeems through the API unchanged.
- **Rate limiting is sized from the threat, not from feel:** per **code** 5 attempts (the real brute-force defence), per **caller** a 60 burst refilling one token per 30 s (~120/hour). An earlier 15-burst figure was too tight and produced `429` for an operator testing by hand; the limiter must bound abuse, not trap the user.
- **A real defect the live probe found:** `.\dev.ps1 -ResetLink` existed as a helper function but was **never wired to a parameter**, and it sat after the port guard — so it could not run while the server was up, and `accounttool -reset-link` did not exist at all. Both fixed; the maintenance commands now run beside `-Verify`/`-PurgeChat` against the live database.
- **Two probe mistakes worth recording**, because both are the system behaving correctly and the test behaving wrongly:
  1. the probe called `/api/auth/forgot` *after* the tool issued a code — issuing **supersedes** any live code, so it silently replaced the handover code;
  2. each wrong guess **spends one of the 5 attempts**, so a probe that "tests a wrong code first" burns the real code's budget. The live probe now issues via the tool, redeems directly, and never guess-tests in the same run.
- **Verification:** `go test` — `TestForgotDoesNotRevealAccounts` (identical status/body, no code in response), `TestResetCodeIsSingleUse`, `TestResetCodeBurnsAfterMaxAttempts`, `TestResetCodeExpires`, `TestResetRevokesEverySession`, `TestResetEnforcesPasswordPolicy` (a weak password is refused **and does not consume the code**), `TestOperatorIssuedCodeRedeems` (the tool's salt and the API's verifier agree, 20 samples). Live: **9/9** flow checks, then full loop **7/7** — real code â†’ **200**, old password **401**, new password **200**, fresh session issued, replay **422**.
- **UI:** `/forgot` and `/reset` exist and both sign-in pages link to them (the links were present but **dead** before — no route matched). The forgot screen states delivery honestly: with no relay it says *"no e-mail was sent — ask an operator for the code"* rather than "check your inbox". `scripts/verify-reset-ui.mjs` drives the real form in headless Chrome (types, submits, follows the step, redeems) and reported `success state: True`, console errors **0**.
- **A trap worth knowing when testing this by hand:** submitting `/forgot` **supersedes** any live code, so a run that asks for a new code and then types an operator-issued code will always fail with `invalid_code`. Issue the code *last*.

## D-045 · A failed panel check must say why, and a traffic limit is stored in bytes

- **Date:** 2026-10-29. Found while chasing "the VPN list shows `—` for usage and expiry".
- **Symptom:** the panel answered, `/api/servers/{id}/test` returned **ACTIVE** (`panel 3.10.2`, `xray 26.10.10`), yet the VPN console reported every row as unreadable and `/api/servers/{id}/panel-clients` answered **502 `panel_error`** with one generic sentence.
- **Two defects, both about *diagnosis*:**
  1. **`writeIntegrationError` replaced every panel failure with the same sentence**, and `integrations.Test` discarded `health.Err` entirely. An operator could not tell a wrong URL from a rejected token from a 404 endpoint. Now the route returns the wrapped reason (single line, â‰¤220 chars, no secret ever in it) and `Test` returns it as `View.Detail`.
  2. That change immediately named the real bug: **`sql: Scan error â€¦ COALESCE(i.panel_total_gb,0) * 1073741824: converting float64 (1.152921504606847e+19) to int64`.**
- **Root cause:** `panel_total_gb` stores the panel's `totalGB` field **verbatim**, and 3X-UI reports that field **in bytes** (10 GiB arrives as `10737418240`). The join multiplied it by `1073741824` a second time, which overflows `int64` at exactly **2â¶Â³**. Every VPN with a real limit was therefore unreadable, and the failure looked like a panel outage.
- **Fix:** read the column as it is stored. No unit conversion belongs in that join; the column is bytes despite its name.
- **Verification:** `TestPanelClientsKeepsLargeTrafficLimits` seeds a 10 GiB limit and asserts the listing succeeds. Reverting the one-line fix reproduces the **exact** production error, so the test is a genuine guard rather than a restatement. Live after the fix: `/api/servers/1/panel-clients?summary=1` â†’ `{"total":7,"untagged":7,"assigned":3,"unassigned":4,"expired":1,"disabled":2}`, and `/api/super/vpns` â†’ `unread=0`. The `/vpns` page renders "VPN management" with **0 console errors**.
- **Lesson recorded:** a status without a reason is not a status. Both switches that hid the cause are now removed, and the fix took minutes once the message was visible.

## D-046 · "Configured" and "working" are different claims about the mail relay

- **Date:** 2026-10-29.
- **What was missing:** the mailer had four tests, and **none of them opened a socket**. They proved the config rules and the MIME bytes, so a relay that rejected our handshake would still have looked healthy.
- **Added:** `internal/mail/smtp_e2e_test.go` — a minimal fake SMTP server plus three tests that speak the real protocol: a complete `EHLO â†’ AUTH â†’ MAIL â†’ RCPT â†’ DATA â†’ QUIT` conversation whose delivered multipart message is inspected; a `550` on `RCPT` becoming an error that names the step; and a closed port failing fast instead of hanging.
- **Added:** `.\dev.ps1 -TestMail <address>` (`accounttool -test-mail`), which sends one real message and reports **which step** failed — dial, handshake, auth, MAIL, RCPT or DATA — without ever printing the password. With no relay set it prints exactly which four variables to set.
- **Verified against a live relay:** a throwaway local SMTP server received the message, and the delivered bytes contained `To: difanmax@gmail.com`, `Subject: â€¦ SMTP test`, the body text and `multipart/alternative`. What remains unverified is only *which server answers* — Gmail instead of the local fake — and that is a credential, not code.
- **Why this matters here:** a password-reset code that silently fails to arrive is a person locked out of their account. The relay is the one component where "looks fine" is worth nothing.

## D-047 · A plan has a price, and the catalogue carries it

- **Date:** 2026-10-29 (owner: *"mata hodama ui ekak one"* — found while auditing the Buy Premium screen).
- **The defect:** `vpn_offers` has carried `price_cents` and `currency` since migration 0002, but the API **never returned them** and `OfferRow` had no field for them. The Buy Premium screen therefore asked the **buyer** to type the amount they were paying. That is not a price — it lets a customer decide what a product costs, and it means the catalogue cannot show a price at all.
- **Decision:** price is part of the catalogue, in **minor units** (`price_cents`, integer) plus an ISO `currency`. It flows through create, update and both read paths, and is validated on create **and** patch (a negative price is refused with `422`).
- **The buy screen now:** shows `formatMoney(priceCents, currency)` per plan, pre-fills the payment amount from it, and makes that field **read-only** when the plan is priced. An unpriced plan says *"Price on request"* and leaves the amount open, instead of pretending.
- **Money formatting:** a new `formatMoney` in `lib/format.ts`, converting minor units at the edge only. Money never travels as a float, and an unknown ISO code falls back to `"123.45 XYZ"` rather than throwing away the render.
- **A bug the tests caught:** the first implementation validated the price on PATCH **only**. `TestOfferRoutesRejectNegativePrice` created an offer with `price_cents: -1` and got **201 Created**. Both paths are now checked, with the test naming the reason in a comment.
- **Verification:** `internal/store` — `TestOfferPriceRoundTrips`, `TestUpdateOfferChangesPrice` (including re-sending the same value and patching currency alone). `internal/httpapi` — `TestOfferRoutesExposePrice` (create **and** list carry the price), `TestOfferRoutesRejectNegativePrice`. Live: `PATCH /api/offers/7 {"price_cents":150000,"currency":"LKR"}` â†’ **200** with the value echoed, a `-1` price â†’ **422**.

## D-048 · A client may buy premium, so it may read the premium catalogue

- **Date:** 2026-10-29 (owner chose this explicitly when asked, rather than having it inferred).
- **The conflict:** D-024.3 makes `GET /api/offers` return **FREE only** to a client. That rule is about which **VPNs** a client may *see*. But the Buy Premium screen needs the **plans** it may *buy*, and a client saw **zero** premium offers — measured live — so the page could never show a plan. Premium could then only ever be sold by a reseller, which contradicts having a Buy page.
- **Decision:** a **separate**, read-only `GET /api/offers/premium` returns active PREMIUM offers to any authenticated principal. `GET /api/offers` is **untouched**, so D-024.3 still governs the free catalogue and no existing rule is widened by accident.
- **Why separate and not a flag:** a flag on the same endpoint invites a future change to leak premium rows into the free list. A second route with its own store method (`PremiumOffers`) makes the distinction structural, and it is visible in the route table.
- **What this does NOT change:** visibility of a premium **VPN** is still assignment-only (D-024.3). A client can now see the *plan* it may buy; it still sees only the *instances* it owns.
- **Verification:** `TestClientSeesPremiumCatalogueButNotInFreeList` asserts **both** halves in one test — the FREE list must not leak a premium row, and the premium route must carry the priced plan. `TestPremiumCatalogueIsAuthenticated` keeps the new route behind a session. Live, as a client: `GET /api/offers/premium` â†’ **200**, three plans including `pr-1` at `150000 LKR`; `GET /api/offers?active=1` â†’ still FREE only.

## D-049 · The setup guide must name the app, not describe it

- **Date:** 2026-10-29 (found while auditing the client screens).
- **The defect:** the guide read *"Install the recommended client app from the setup guide sent with your VPN."* — it named **no app**. A customer holding a working VPN could not install anything, so a working product produced a support ticket instead of a connection.
- **Decision:** each platform lists **two real clients** with a link to their own official source, a short honest note (open source / paid / needs .NET), and steps written for that app's actual import flow:
  - Android â†’ **v2rayNG**, **NekoBox**
  - iOS â†’ **Streisand** (free), **Shadowrocket** (paid)
  - Windows â†’ **v2rayN**, **NekoRay**
- **Why two and not one:** one recommendation is a single point of failure — if an app is pulled or breaks on a new OS version, the customer is stranded. Two gives a fallback, and the notes let the customer choose on price and licence.
- **Links open in a new tab** with `rel="noopener noreferrer"`, and carry a visually-hidden "(opens in a new tab)" for screen readers — the visual signal is only an icon.
- **New shared utility:** `.ui-sr-only` in `styles/base.css`. There was **no** visually-hidden helper in the project, which is why that cue had nowhere to live.
- **Verification:** `scripts/verify-setup-guide.mjs` drives a real client session through `/vpn/24` and asserts the apps have names, the hrefs are `https:`, and there are at least four steps. Result: **v2rayNG + NekoBox** with their GitHub URLs, **5 steps**, tabs `Android / iOS / Windows`, console errors **0**.
- **A finding from building the check:** the seeded `client@veylix.test` owned **no VPN at all**, so no client screen that needs a VPN could be exercised. VPN 24 was assigned to it for the test; a client with zero VPNs is a legitimate state, but it means a "first-run" path deserves its own test rather than being assumed.

## D-050 · A multi-step flow must say which step it is on — to everyone

- **Date:** 2026-10-29 (Step 3 of the UI improvement pass).
- **The defect:** both checkout flows rendered their progress as
  `<div className="sell-step-dots" aria-hidden="true">`. The dots were the **only**
  indication of position, and they were deliberately hidden from assistive
  technology — so a screen-reader user experienced a 3-step flow as one long form
  with no beginning, middle or end.
- **Decision:** one shared `StepProgress` component, used by **both** flows:
  - the dot strip stays `aria-hidden` (it is decoration), **and**
  - a visible `Step 2 of 3 — Payment` line carries the meaning for everyone, **and**
  - a `role="status" aria-live="polite"` region announces the same words when the
    step changes. Polite, not assertive: progress is not an interruption.
- **One component, not two:** the two flows had hand-rolled identical markup, which
  is how the a11y gap existed twice.
- **Back behaviour made honest:** step 2's Back now **clears the chosen plan**, so
  step 1 is a neutral choice again rather than a preselected card the buyer has to
  notice and undo. Step 3 offers *"Buy another plan"* rather than a Back arrow,
  because a submitted order cannot be un-sent and a Back arrow would imply it can.
  The price is also shown as a chip on step 2, so the buyer confirms what they are
  paying without scrolling back.
- **Verification:** `scripts/verify-step-progress.mjs` drives `/buy` as a client and
  asserts a visible `Step N of M` line **and** a live region carrying it:
  `live: ["Step 1 of 3 — Choose a plan"]`, `visibleStep: true`,
  `dotsHidden: true`, console errors **0**. `tsc` 0, `oxlint` 0/0, `npm run build`
  0, preflight **33/33**, backend `go test` 12/12.

## D-051 · A filtered list is a link, and typing is not a query

- **Date:** 2026-10-29 (Step 4).
- **The defect:** **all eight** list screens (clients, VPNs, staff, admins,
  payments, audit, servers, offers) kept their filters in `useState` — 55 `useState`
  calls between them. Consequences, all visible to an operator: refresh dropped
  every filter, a filtered view could not be sent to a colleague, the browser Back
  button left the page instead of undoing the last filter, and typing in a search
  box either fired a request per keystroke or needed a submit button.
- **Decision: one `useListQuery` hook**, with the URL as the source of truth.
  - Only **non-default** values are written, so `/clients` stays clean instead of
    `?status=&q=&page=1&sort=`.
  - **Text is debounced** (300 ms, configurable); selects, dates and paging apply at
    once. `setNow` exists for Enter/blur so a deliberate action is never delayed.
  - Changing a filter **resets to page 1** — staying on page 7 of the previous
    result set is how a freshly filtered list looks empty for no reason.
  - Types are derived from the declared defaults, because `"false"` must not become
    truthy and `"12"` must not stay a string.
  - `isFiltered` drives a **Clear filters** affordance that only appears when there
    is something to clear.
- **A React detail worth recording:** the first implementation synced the draft
  text in a `useEffect`, which `oxlint`'s `react(set-state-in-effect)` flagged as a
  cascading render. The draft is now rebuilt **during render** by comparing the
  params string to the one it was built from — the same intent, one render instead
  of two.
- **Verification:** `scripts/verify-url-state.mjs` — clean visit â†’ `""`; choosing
  *Blocked* â†’ `?status=SUSPENDED` **immediately**; typing `kasun` â†’ URL **unchanged**
  at first (debounce proven) then `?status=SUSPENDED&q=kasun`; **reload reproduces
  the filtered view**, and *Clear filters* is offered. Console errors **0**.
- **Scope note:** `ClientsPage` is converted and proven. The remaining seven screens
  share the identical local-state shape and will move to the same hook one at a
  time, each with the same check run against it — the hook is the contract, and
  converting them in one commit would make a regression impossible to attribute.

## D-052 · A failed screen must offer a way forward, and a hook must be stable

- **Date:** 2026-10-29 (Step 5).
- **The defect:** 18 screens hand-rolled loading/error/empty. **211** error surfaces
  existed and only **three** retries: a failed request left reloading the page as the
  only way out. Loading often rendered as a sentence that looked identical to "no
  data", and empty states were written 44 different ways.
- **Decision: one `AsyncState`** with an explicit `status` (`loading | error | empty |
  ready`), never inferred from `data.length` — an empty array means both "still
  fetching" and "genuinely empty", and those must never look the same.
  - `error` renders a real panel with **Try again** when `onRetry` is given, and
    still shows the message when it is not (never swallowed).
  - `loading` renders a skeleton shaped like the content.
  - `empty` carries the next step ("No clients match these filters â†’ Clear filters").
  - **An error wins over the row count**: showing "no clients match" for a server
    fault blames the operator's filters for something they did not do.
- **A serious bug this step found — a request loop.** Wiring `ClientsPage` to
  `useListQuery` produced **653 identical `/api/super/clients` requests in a single
  paint**. Cause: the hook memoised `values` on the `params` object and the
  `defaults` literal, both of which are new on every render, so the caller's
  `useEffect([values])` fired forever (effect â†’ request â†’ store update â†’ render â†’ new
  `values` â†’ effect). Fixed by keying the memo on **strings**
  (`params.toString()` and a serialised `defaults`). After the fix: **3**
  intercepted requests.
- **How it was found, and why that matters:** the retry test intercepts the API to
  force a failure. It reported `intercepted: 653` — a number that is impossible for
  one page load. A test written only to assert the happy path would never have shown
  it. The loop is now regression-covered by that same count.
- **Verification:** `scripts/verify-retry.mjs` — force `500` on `/api/super/clients`,
  assert `.ui-async--error` + a **Try again** button, click it, assert the panel is
  gone and 3 rows render. Result: `intercepted: 3`,
  `text: "That didnâ€™t load / simulated failure / Try again"`, `recovered: { stillError:
  false, rows: 3 }`. `verify-url-state.mjs` still passes; `tsc` 0, `oxlint` 0/0,
  `npm run build` 0, preflight **33/33**, backend 12/12.

## D-053 · A field's error must be announced with the field

- **Date:** 2026-10-29 (Step 6).
- **What the plan expected vs what was true.** Step 6 was scheduled as a
  `useMutation` refactor on the premise that writes "hand-roll pending/error/rollback
  and some leave an optimistic update in place on failure". **That premise was
  wrong, and checking it before building was the point of the step:**
  - `clientAdminStore.setStatus` / `setFreeLimit` mutate **only after** the server
    confirms, so there is nothing to roll back;
  - `chatStore` keeps the optimistic echo but marks it `failed: true` and preserves
    the text — words are never lost and nothing is shown as sent when it was not;
  - `notifStore` marks read optimistically and **reloads server truth** on failure;
  - every destructive action is already behind a dialog (`blockOpen` in
    `ClientDetailPage`, dialogs in servers, admins, staff, payments, offers, chat).
  **No `useMutation` was written.** Adding one would have been churn dressed as
  improvement, and the honest result of the audit is "already correct".
- **The real gap the audit found instead:** **`aria-describedby` appeared zero times
  in the entire application.** `Input`/`Textarea`/`Select` set `aria-invalid` and
  rendered the message in a sibling `<span>` with no `id`, so a screen reader
  announced *"invalid"* and never the reason. A blind operator could not tell
  "too short" from "already taken".
- **Decision:** each hint/error gets a derived id (`${fieldId}-error`,
  `${fieldId}-hint`) and the control points at it. An explicit `aria-describedby`
  passed by a caller is preserved and merged ahead of the derived ids.
- **Verification:** `scripts/verify-field-a11y.mjs` submits the empty `/login` form
  and reads the rendered DOM: the input's `aria-describedby` is `["_r_1_-error"]`,
  it resolves to `"Email and password are required."`, and **`brokenRefs: 0`** — no
  reference points at a missing element. `tsc` 0, `oxlint` 0/0, build 0, preflight
  **33/33**, `verify-retry` and `verify-url-state` still green, backend 12/12.

## D-054 · Bulk actions exist, and select only what you can see

- **Date:** 2026-10-29 (Step 7).
- **The defect:** the `Table` had **no selection API at all**. Blocking or tagging
  fifty clients meant fifty dialogs, which is not something an operator will ever do
  — so the "client management" console could only manage one client at a time.
- **Decision:** selection is **opt-in** on `Table` (`selection` prop) so no existing
  table changes behaviour, and it is **controlled** (`selected` + `onSelectedChange`)
  because the bulk bar lives outside the table. A `SelectionBar` shows the count in
  words and can be dismissed.
- **Select-all means "this page, under this filter"** — never "the whole database".
  A checkbox that silently includes rows you cannot see is how a bulk action destroys
  data that was never on screen.
- **The confirm is mandatory and names the consequence**, not just the verb:
  *"2 clients will be blocked. Every one of their sessions ends immediately and they
  cannot sign in until unblocked."*
- **Sequential, and honest about partial failure.** The API takes one client per
  call, so the loop runs **one at a time** — firing fifty parallel writes at a 1 vCPU
  host turns a convenience feature into an outage — and reports *"Blocked 12 of 15;
  3 failed"* rather than claiming success.
- **Verification:** `scripts/verify-bulk.mjs` drives the real page: ticks two rows,
  asserts the bar reads `2 selected | Unblock | Block | Clear`, asserts the dialog
  names the count, confirms, then **reads the truth back from the API** rather than
  the DOM: `suspendedOnServer: 2`, page text `Blocked 2 clients.` The seeded clients
  were restored to `ACTIVE` afterwards. `tsc` 0, `oxlint` 0/0, build 0, preflight
  **33/33**, and `verify-retry` / `verify-url-state` / `verify-field-a11y` /
  `verify-step-progress` all still return 0.

## D-055 · Convert only what is genuinely a filter

- **Date:** 2026-10-29 (Step 8).
- **What the plan assumed vs what the code showed.** Steps 8â€“9 were scoped as
  "seven more list pages to `useListQuery`, seventeen to `AsyncState`". Reading each
  page first shrank that honestly:
  - **Only `VpnsPage` and `StaffPage` had real server-side filters.** `AdminsPage`,
    `PaymentsPage`, `ServersPage` and `OffersPage` use `useState` for **dialogs and
    forms**, not filters — converting them would have invented URL parameters nobody
    changes.
  - `AuditPage` and `PaymentsPage` filter a **already-loaded array** in memory, so
    there is nothing to put in the URL that a reload could not reproduce for free.
- **Converted:** `VpnsPage` and `StaffPage` now read filters from the URL
  (debounced text, immediate selects) and render through `AsyncState` (error + **Try
  again**, empty + **Clear filters**). Two pages, both proven.
- **A gap the verification found, not the code:** `verify-url-state.mjs` was
  hard-coded to the clients page — it looked for a `SUSPENDED` option and one
  placeholder string, so on `/vpns` it reported `no-status-select` and still exited
  **0**, because the old assertion only checked `afterFilter` contained a specific
  string it never matched. The script now drives the first available select and the
  first text input, and its assertions include `applied !== 'no-select'` and
  `immediateSearch === afterFilter` — so a page it cannot drive **fails** instead of
  quietly passing.
- **A real omit the run exposed:** `/vpns` had no *Clear filters* control, so its
  `clearOffered` was `false` while `/clients` and `/staff` were `true`. Added.
- **Verification:** all three list pages, driven for real —
  `/clients` â†’ `?status=ACTIVE&q=kasun`, `/vpns` â†’ `?tag=free&q=kasun`,
  `/staff` â†’ `?role=admin&q=kasun`, each **identical after reload**, each

## D-063 · An assignment keeps the panel's name, is always premium, and can never be untagged

- **Date:** 2026-11-03. **Owner's rule, verbatim:** *"assign karapu vpn untag node
  thiynnath ba"* and *"rename une neththan e thyna namama thiyanna"* — an assigned
  VPN must never land in the untagged group, and if the rename did not happen the
  VPN keeps the name it already has.
- **The defect, in three parts.** One physical panel client could be shown three
  different ways:
  1. **It displayed as UNTAGGED.** `AssignPanelClient` stored `offer_id = NULL` when
     the dialog sent no package, the projection sent `offer_kind: ""`, and the client
     screen resolved it with `offer_kind ?? tag`. An **empty string is not nullish**,
     so `??` never fell through to the tag: a VPN with a locked `PREMIUM` tag rendered
     as `UNTAGGED` and was filed under "Other VPNs" while "My Premium VPNs" told the
     customer they had none.
  2. **It counted as zero premium VPNs.** `premium_count` came from
     `vpn_assignments`, but that row was only written when a reseller was named — so
     a direct Super-Admin issue reported `0`.
  3. **It could be edited twice.** The row was recorded under an invented
     `<client email>_paid_<slot>` name and the panel renamed afterwards. When the
     rename failed the panel kept the old name while the row kept the new one, so the
     console still offered "Assign" for a client somebody already held — and a second
     click created a **second row for one VPN**, with its own owner and its own tag.
- **Decision — three rules, enforced at four layers.**
  - **One name.** The row, the tag and the ledger all carry the name the panel
    actually holds. A rename into the D-034 scheme is **opt-in**, runs only after the
    assignment is committed, and a failure changes nothing: the VPN keeps its existing
    name, which is a finished state, not a half-applied one.
  - **Always premium.** There is no FREE assignment in the product (a FREE VPN is
    *claimed* from an offer). `kind: "free"` is refused `422`; the row always carries
    a package — the operator's choice, else an active PREMIUM catalogue row, else the
    internal `Platform assignment` carrier — so it always has a kind.
  - **Never untagged.** `UNTAGGED` now means exactly one thing: a panel client **no
    Veylix account holds**, which only a Super Admin sees.
- **The four layers.** `migration 0019` (repairs already-split rows, tombstones
  duplicates without deleting them, adds `ux_panel_assignments_target`, backfills a
  package and a locked PREMIUM tag) · the store (name preserved, offer resolved,
  `panelClientHeld` checks row **and** both ledger columns, `RenameAssignmentName`
  renames the panel before writing anything locally) · the projection
  (`COALESCE(o.kind, t.tag, 'PREMIUM')` for owned rows, plus a new `kind_source` of
  `offer|tag|default|none`) · the UI (`toKind(v.offer_kind || v.tag, assigned)`, and
  `toKind` cannot return `UNTAGGED` when `assigned`).
- **A new gate, because a comment cannot fail a build.** `audit-ui-truth.mjs` now
  rejects a **nullish coalesce on a classification field** — `offer_kind ?? tag` and
  its aliases — with two positive and two negative self-test cases, so the exact
  one-character mistake cannot return.
- **Verification, run 2026-11-03.** Backend `go build`, `go vet` and
  **`go test ./... -count=1` â†’ 12/12 packages pass (452 test functions)**;
  `tsc -b` 0, `oxlint` **0/0**, `npm run build` 0, `audit` OK,
  `audit:selftest` **8 positive + 6 negative**, `docs:verify` OK.
  **Live, against the real dev database** (migration applied: `schema_version: 19`):
  `GET /api/vpns` as the Super Admin showed the owned row `janith14dawan@â€¦` as
  `offer_kind: PREMIUM, kind_source: offer` while the three genuinely unassigned panel
  clients (`id: 0`, no owner) stayed untagged; `POST â€¦/panel-clients/11/yasa/assign`
  answered `201 {assigned: true, panel_email: "yasa", offer_id: 12, tag: "PREMIUM",
  renamed: false}`; the **client's own** `GET /api/vpns` then returned exactly one row,
  `yasa`, `offer_kind: PREMIUM`, `assigned: true`, and `?tag=premium` found it;
  `/api/super/clients/15` reported `premium_count: 1`; and a **second** assign answered
  **409**. The test assignment was removed afterwards.
- **What this supersedes.** D-034's rename-on-assign becomes opt-in for assignments;
  D-037.2's "the panel is renamed â€¦ a failure is reported as 502" is replaced — a
  failed rename is now a successful assignment that kept its name. `D-025.2` (an
  issued tag locks) is unchanged and still enforced.

  `clearOffered: true`, console errors **0**. `verify-retry` now takes
  `VEYLIX_FAIL_PATTERN`, so `/vpns` (`intercepted: 3`, recovered **6 rows**) and
  `/staff` (`intercepted: 3`, recovered **3 rows**) are covered too. `tsc` 0,
  `oxlint` 0/0, build 0, preflight **33/33**, backend 12/12.

## D-056 · A phone is not a small desktop

- **Date:** 2026-10-29 (Step 9).
- **The measurement, before any change.** At 390Ã—844 (iPhone-class), on `/clients`,
  `/vpns` and `/staff`: **every visible interactive control was under 44Ã—44** — menu
  and topbar buttons 36Ã—36, buttons 34px tall, inputs 41px — and each data table
  **scrolled sideways** instead of reflowing. A thumb target needs to be bigger than
  a cursor target, and a table that scrolls horizontally hides the column carrying
  the action.
- **What was added** (`styles/mobile.css`, `@media (max-width: 480px)`):
  - 44px minimum on buttons, inputs and topbar controls; form inputs go to **16px**
    text, because iOS zooms the whole page on focus below that.
  - **Tables reflow into stacked cards**, one row per card, each cell labelled from
    its column header via `data-label`. The header strip is hidden rather than
    dropped — its text becomes the per-cell labels, so no value appears without the
    column that explains it.
  - Filters stack to one column; the page header stacks so its action stays reachable.
  - `hide-sm` columns become visible again: once a row is a card there is nothing to
    save by hiding a field, and hiding one would drop information the operator came for.
- **The bug that made the first attempt do nothing.** The card reflow was written and
  still measured `tableScrollsSideways: true`. Cause: the desktop rule is
  `.ui-table { min-width: 560px }` so columns cannot crush — and a `min-width` survives
  a `display: block` change, so the table kept its 560px floor and kept scrolling. It
  must be explicitly released inside the media query.
- **Two audit-tool defects found on the way, both of which had produced false numbers:**
  1. The off-canvas drawer is hidden with `translateX`, which does **not** zero its
     `getBoundingClientRect()` — so the closed menu's 224px links were counted as
     visible targets, and the audit reported "every target is too small" instead of
     the real dozen.
  2. A grep for `max-width: 480px` in the built CSS returned 0 because the minifier
     rewrites it to `width<=480px`. The rules were present all along; the check was
     wrong, not the build.
- **After:** `/clients`, `/vpns`, `/staff` at 390px — **`horizontalOverflowPx: 0`,
  `under44px: 0`, `tableScrollsSideways: false`**. Desktop unchanged: `verify-bulk`,
  `verify-url-state`, `verify-retry` and preflight **33/33** all still pass.

## D-057 · Navigation is not optional, and a demo page is not a feature

- **Date:** 2026-10-29 (Step 12).
- **Two gaps, both found by comparing routes against the sidebar:**
  1. **`/staff` (Admin management) had no nav entry.** The page, its route and its API
     all worked — an operator simply had to know the URL. This is one of the three
     consoles the owner asked for, so "reachable by typing it" is not reachable.
  2. **`Buy premium` and `Purchases` were missing from the client sidebar.** They were
     linked from the dashboard, so a client who navigated anywhere else could not get
     back without the browser's Back button.
- **`/sample` was a public production route.** It is a component/theme showcase that
  renders demo data and its own 197-line stylesheet — a development tool, not a
  feature. It is now gated on `import.meta.env.DEV` and **compiled out of a production
  build** (verified: 0 occurrences of the route in `dist/assets/*.js`).
- **`/setup` needed no change.** Reading it first showed it already redirects: an
  authenticated visitor goes to `/dashboard`, and an anonymous one goes to `/login`
  when no bootstrap is required. The plan's note that it "had no guard" was wrong.
- **Verification:** `scripts/verify-nav.mjs` signs in as each role, reads every
  `a[href]` in the shell, and asserts the sidebar contains the routes that role may
  use — **17/17 pass** (8 super-admin, 6 admin, 3 client). `tsc` 0, `oxlint` **0/0**,
  build 0, preflight **33/33**, and `verify-bulk` / `verify-url-state` /
  `verify-retry` / `verify-field-a11y` all still return 0.

## D-058 · Touch targets scale for a finger, not for a mouse

- **Date:** 2026-10-29 (Step 11).
- **The finding the audit produced:** on **every** page, most controls were under the
  44Ã—44 minimum — sidebar links 224Ã—37, auth buttons 396Ã—37, inputs 396Ã—41, topbar
  icons 36Ã—36, column sort headers 18px tall, filter chips 28px, the staff role
  toggle 35px. Step 9 had fixed this only below 480px.
- **The decision (B, not A):** the enlargement is scoped to
  **`@media (pointer: coarse)`**. A mouse is precise and 36â€“40px rows are a deliberate
  desktop density choice; a thumb is not. Enlarging everywhere would have cost ~20%
  of visible rows to fix a problem desktop users do not have. A laptop with both a
  touchscreen and a trackpad reports `fine` — correct, because that user is on the
  trackpad.
- **Fixed, by class, each for a reason:** `.ui-btn--sm/-md` (44px tall),
  `.ui-input` (44px), `.app-nav__item`, `.app-topbar__icon-btn` (36Ã—36 â†’ 44Ã—44, since
  an icon has no text to grow with), `.ui-table__sort` (**width too** — "Offer â†•" was
  37px wide, so height alone was not enough), `.dash-range` / `.audit-filters button`,
  `.auth__role`, `.ui-tabs` / `.prof-tabs`, `button[aria-label]`, `.ui-switch`,
  `.chat-find button`.
- **Deliberately NOT fixed:** inline text links inside a sentence (`Create one`,
  `Forgot your password?`, `Staff sign-in`). Growing them to 44px would break the line
  they sit in, and they are not how anyone navigates on a phone.
- **Result:** at a coarse pointer, **23 of 29 pages have zero undersized controls**
  (from **0 of 29**), and the **9** that remain are those inline text links. All 29
  pages still land as expected with **0 console errors**.
- **The audit learned to emulate a finger.** It now takes `VEYLIX_TOUCH=1` and sets
  `Emulation.setTouchEmulationEnabled`, because without it the run measures desktop
  density and reports `fine` while a phone user still gets 36px buttons.
- **Verification:** `tsc` 0, `oxlint` **0/0**, build 0, preflight **33/33**, and
  `verify-nav` / `verify-bulk` / `verify-url-state` / `verify-retry` /
  `verify-mobile` all return 0.

## D-059 · Inline styles become token classes, and the visual result must not move

- **Date:** 2026-10-29 (Step 13).
- **The measurement, after correcting my own earlier count.** I had reported "155
  inline styles"; measuring every `.tsx` (components included, not only pages) gave
  **244 sites**. Their shape was extremely repetitive: `fontSize: 12` Ã—63,
  `margin: '10px 0'` Ã—22, `fontSize: 13` Ã—10, `marginTop: 12` Ã—8, and so on.
- **Why it mattered:** a `style={{ fontSize: 12.5 }}` is a hard-coded pixel that never
  reads `--font-size-scale`, so **Theme Studio cannot reach it**. Changing the type
  scale restyled the token-driven text and left 63 labels behind.
- **Decision:** add token-backed utilities to `tailwind.css` (`.t-xs`, `.t-sm`,
  `.t-sm2`, `.t-base` — each `calc(Npx * var(--font-size-scale, 1))`; `.rhythm`,
  `.rhythm-loose`, `.flow-top`, `.flow-top-3`, `.no-underline`), then convert **only**
  sites whose inline style is EXACTLY one of those values. **107 converted, 137 left**:
  anything carrying a second property (`gap`, `maxWidth`, `display`) stays inline,
  because a half-converted style is how a layout silently breaks.
- **A defect the converter had, caught by `tsc` before it shipped:** for a style-only
  element it emitted a second `className` next to an existing one —
  `<div className="ui-chart-legend" â€¦ className="flow-top">`. `tsc` refused to compile
  it (`JSX elements cannot have multiple attributes with the same name`). The rewrite
  is now attribute-aware and refuses that case outright.
- **Behaviourally verified, not assumed:** the audit was captured **before** and
  **after** — `29/29` pages land in both, **0 console errors** in both, undersized
  touch targets `10 â†’ 9`. Text-length drift appeared on four pages and each was
  traced: `/audit` and `/notifications` count live rows, `/clients/3` renders an
  audit trail, and `LoginPage` shows a welcome banner depending on auth state —
  `git diff --stat` on `ClientDetailPage.tsx` is **empty**, so no label was lost.
- **Tooling added:** `scripts/check-inline-styles.mjs` (counts sites, grouped by
  whether a token class covers them — the number that must not grow) and
  `scripts/convert-inline-styles.mjs` (`--dry` by default). `tsc` 0, `oxlint` 0/0,
  build 0, preflight 33/33, all five browser suites 0.

## D-060 · Every data view offers a way back from failure

- **Date:** 2026-10-29 (Step 14).
- **The plan said "17 pages". The code said eight.** A raw count of `error` hits
  flagged 24 files, but most were auth forms with **inline field validation**
  (`"Email and password are required."`), which is not a failed request and must stay
  inline — a `AsyncState` panel there would be wrong. After filtering to pages that
  actually fetch and render a list/detail, **8** remained:
  `AuditPage`, `NotificationsPage`, `PaymentsPage`, `AdminsPage`,
  `AdminServersPage`, `ClientDashboardPage`, `OffersPage`, and `DashboardPage`.
- **Each got the same three states**, with an empty state that says what to do next
  rather than only that something is missing: *"No resellers yet â†’ New admin"*,
  *"No plans in the catalogue yet â†’ New offer"*, *"You have no VPNs yet â†’ Buy
  premium"*, *"Nothing recorded for 'auth' â†’ Show all events"*.
- **A false claim removed while converting `AdminServersPage`:** its empty state said
  *"A Super Admin has to assign one — that endpoint does not exist yet (P5)"*. That
  endpoint **does** exist (`PATCH /api/admins/{id}` with server ids, wired in Step 7's
  console), so the page was telling a reseller that the product could not do something
  it can. The text now states the real situation.
- **Deliberately left alone:** `AdminClientsPage`, `ProfilePage`, `ChatPage`,
  `BuyPremiumPage`, `SaleFlowPage`, `LoginPage` and the other auth screens — their
  errors are **form submission failures** shown next to the field or button that
  caused them, which is more specific than a page-level panel.
- **Verification:** **zero** pages still use the old `{loading ? â€¦ : error ? â€¦}`
  pattern. `scripts/audit-ui.mjs` — **29/29 pages land as expected, 0 console
  errors, 0 problems**. `tsc` 0, `oxlint` 0/0, build 0, preflight **33/33**, and
  `verify-nav` / `verify-bulk` / `verify-url-state` / `verify-retry` /
  `verify-mobile` / `verify-field-a11y` all return 0.

## D-061 · A dashboard that cannot reach the backend must not look empty

- **Date:** 2026-10-29 (Step 15 — the last page).
- **The defect, and why it was the worst instance.** `/dashboard` reported failures
  **only** in its subtitle (`Some panels could not be read: â€¦`): no `role="alert"`, no
  retry, and the metric cards kept rendering their numbers. A dashboard whose reads
  all failed looked exactly like a dashboard of a quiet system — the one screen where
  silence is most misleading.
- **Decision:** an explicit `role="alert"` banner directly under the page header —
  *"Some figures could not be loaded"* with the reason and a **Try again** button —
  while the partial data still renders. Partial failure is expressed rather than
  hidden, because this page deliberately keeps going when one source answers 403
  (see `guard` inside `runRefresh`) and blanking it would be a worse lie.
- **Not converted to `AsyncState`, on purpose.** `AsyncState` models "one source,
  four states". The dashboard aggregates **five independent** reads; wrapping it
  would have replaced its tailored skeleton (metric cards, chart panels, status rows)
  with one generic block and hidden which source failed. The pattern is shared where
  it fits, not applied everywhere for consistency's sake.
- **Verification:** `verify-retry.mjs` with `VEYLIX_FAIL_PATTERN='*/api/sync/status*'`
  against `/dashboard` — `intercepted: 3`, `hasErrorPanel: true`,
  `text: "Some figures could not be loaded / simulated failure / Try again"`,
  `recovered: { stillError: false }`. Full audit **29/29 pages, 0 problems, 0 console
  errors**; preflight **33/33**; all **8** browser suites return 0.- **Addendum, 2026-11-03 - the same rule had a second, untested seam.** Live
  verification of the console found it: `?assigned=1` returned **0 rows** and
  `?assigned=0` returned all **5**, including a VPN Veylix records as owned by
  `janith14dawan@gmail.com` (`owner_user_id: 24`). Two causes, both fixed:
  - `PanelVPNLister.localRows` never set `Assigned`, so the `local_missing` branch -
    which appends the local row **without** passing through `panelViewToRow` - sent
    `assigned: false` for every row whose panel client had vanished. It now sets
    `r.Assigned = r.OwnerID > 0`, because a local row exists only when an account
    holds it.
  - `panelViewToRow` copied `v.Assigned` (the panel read's answer) over the local
    truth, so a panel that could not resolve a client erased the ownership Veylix
    already knew. It is now `local.OwnerID > 0 || v.Assigned` - the panel may only
    **add** that fact, never remove it - plus the `OwnerUserID` recovery when only
    the panel side of the join resolved an owner.
  - **Why no test caught it:** every assignment test exercised `SQLVPNLister` (the
    client path) or built rows by hand. `panelViewToRow` and `localRows` - the Super
    Admin console path - had **no test at all**. `TestOwnedPanelClientIsAlwaysAssigned`
    now covers the seam directly, including the "panel could not resolve the owner"
    case that produced the bug.
- **Live result after the fix:** `assigned=1` -> **1** row
  (`janith14dawan@gmail.com_paid_1`, `assigned: true`), `assigned=0` -> **4** rows
  (only the genuinely unowned panel clients `athoof`, `dn-test`, `ravi`, `yasa`).
  `go test ./...` 12/12 packages **453 test functions**, `tsc` 0, `oxlint` 0/0,

## D-064 · A VPN can be moved between customers, and the panel is never touched

- **Date:** 2026-11-03. **Owner's rule, verbatim:** *"vpn assign karala thiyena ewa
  clientla athare maru karanna puluwan wenna one. hariyata janith-14 gen ekak
  janith-13 kiyana client ta maru karu karanna puluwan wage."*
- **Decision — `POST /api/vpns/{id}/transfer` moves OWNERSHIP, not the VPN.**
  `vpn_instances.owner_user_id` changes; `panel_email`, `panel_sub_id`,
  `offer_id`, `expires_at`, `traffic_limit_bytes`, every `panel_*` mirror column
  and the whole `panel_client_tags` row stay exactly as they were.
- **Why no panel write.** 3X-UI has no concept of "this client belongs to Veylix
  account 23", so there is nothing to write. The consequence is the feature's
  whole value: the customer's UUID, sub-id, limits, links and QR keep working
  because nothing about the client changed. A transfer also works during a panel
  outage, and cannot half-apply on a remote system.
- **Why the tag is not re-stamped.** `locked_at` is the promise that an issued
  VPN cannot be re-priced (D-025.2). A change of owner is not a new sale, so
  touching the tag would retroactively rewrite what the first customer paid for.
  `tagged_by` keeps naming whoever classified it — that is history, not ownership.
- **Why a transfer is not an assignment.** `AssignPanelClient` refuses a panel
  client Veylix already holds (`ErrPanelClientTaken` â†’ `409 panel_client_taken`),
  and that refusal is correct: it is what stops one physical VPN becoming two
  rows (D-063). A transfer is the *only* write allowed to touch a held VPN, and it
  lives in its own store file (`store/transfer.go`) so the two rules cannot leak
  into each other. The header comment on `assignment.go` has named this state for
  months: *"if a row exists, this is a move, not an assignment"*.
- **Why `ErrClientHasVPNs` finally has a recovery path.** Deleting a client with
  VPNs was refused with *"revoke **or reassign** them first"*, but reassigning was
  impossible, so the only way to free an account was to **revoke** a VPN somebody
  paid for. The documented option now exists.
- **What a transfer refuses, each with its own code** — an unexplained refusal is
  what makes an operator revoke a working VPN to get past it:
  - `vpn_terminal` — `REVOKED` / `REMOVED_FROM_CATALOG`;
  - `vpn_not_ready` — `PENDING` / `REVOCATION_PENDING` (the panel never confirmed it);
  - `vpn_released` — inside the 3-day undo window, where the current owner could
    still restore it and produce **two owners**; a pending `vpn_releases` row is
    also checked, because that is the shape the D-027 defect took;
  - `vpn_not_on_panel` — `SERVER_UNAVAILABLE`, i.e. the panel positively does not
    hold the client; transferring it would only move the disappointment;
  - `cannot_transfer_self`, `not_a_client` (only an `ACTIVE` client may receive),
    `concurrent_change` (the `owner_user_id` guards the UPDATE, so two consoles
    cannot both move the same VPN).
- **FREE quota honesty.** `free_claims.client_user_id` is what both caps count
  (D-025.4/.5). The claim row now follows its VPN, stamped with
  `transferred_at` / `transferred_from_user_id`. Without that, the old owner would
  stay charged for a VPN they no longer hold **and** the new owner could exceed
  the cap of 3 without ever seeing a refusal.
- **History that cannot be rewritten.** `vpn_transfers` (migration `0020`) records
  from/to user and seller, `offer_id` and `kind` **by value**, the actor, the
  reason and whether a claim moved. `vpn_assignments` closes the old row as
  `TRANSFERRED` and opens a new `ACTIVE` one, so every "premium VPNs held" count
  stays correct instead of counting one VPN twice.
- **A defect this exposed and closed.** `SQLVPNLister` joined `vpn_assignments`
  unconditionally, so the two assignment rows a transfer creates listed the VPN
  **twice**; the customer who had just received it saw a duplicate immediately.
  The join is now restricted to the live row (`status IN ('ACTIVE','PENDING')`),
  and a reseller who once sold the VPN still sees it through an `EXISTS` subquery
  on the history, so narrowing the join loses no row. Regression test:
  `TestTransferDoesNotDuplicateTheVPN`.
- **Audit + trail.** `vpn.transfer` is its own action (never folded into
  `panel_client.tag`), recorded for both outcomes — `DENIED` carries
  `refused_because`, so "somebody tried to move this and the server said no" is
  answerable after an ownership dispute.
- **Verification.** `store/transfer_test.go` (14 cases) + `transfer_routes_test.go`
  (8 cases) green; `go test ./...` 12/12 packages; `go vet` clean; `gofmt` clean
  on every new file; `tsc` 0, `oxlint` 0/0, `npm run audit` OK, `/vpns` renders
  with `consoleErrors: []`. **Live, on the real panel:** `dn-test` held by
  `janith14dawan@gmail.com` moved to `janith13dawan@gmail.com` (`200`,
  `panel_email` unchanged) — the new owner's `/api/vpns` listed it as PREMIUM, the
  old owner's list lost it, the history showed
  `janith14 â†’ janith13 by owner@veylix.test`, and moving it back produced a second
  history row with both entries intact.


## D-065 · The Super Admin console answers "what needs me today?" without being asked

- **Date:** 2026-11-03. Owner's instruction: *"mulinma super admindingen patan ganna,
  thwath geburata gihin balanna eyata denata karanna puluwan dewal ha eyage UI
  thiyena widiha user friendly widihata hadanna."*
- **The finding.** A deep pass over all 17 Super-Admin screens found the backend
  knew far more than the screens showed, and that two screens claimed the same
  name. The fixes below are ordered by how much friction each removed.
- **P2 — `/vpns` Command Bar.** `GET /api/super/vpns` now returns a `summary`
  counted over the **unfiltered** catalogue (total · free · premium · untagged ·
  assigned · unassigned · exhausted · expiring_soon · expired · disabled ·
  unread). Every tile is a one-click filter and the strip is rendered above the
  server picker, because "what needs me" outranks "which server". Two rules:
  - the counts are computed **before** filtering, so opening one tile does not
    zero the others — a strip that changed shape on click reads as "the
    catalogue shrank";
  - a zero-count tile is **disabled**, because a control that provably returns an
    empty list is a dead end dressed as an action.
  - a row whose panel could not be read counts as `unread` **only** — never also
    `expired` or `disabled`, which would turn "we could not ask" into a fact.
- **P3 — the FREE/PREMIUM tag finally has a control.** `PUT
  /api/panel-clients/{server_id}/{email}/tag` shipped with five tests and **no
  caller**, so a panel client created directly in 3X-UI could never enter the FREE
  catalogue at all. `/vpns` now renders a two-state picker per row. It is
  **disabled on issued rows**, mirroring the SQL lock: the server answers `409
  tag_locked` and the UI does not offer a control that cannot work. Tagging
  reloads the list, so the last untagged VPN visibly empties the Untagged tile.
- **P1 — one "Admin management", not two.** `/admins` and `/staff` carried the
  **same page title** while one owned quota + panel scope and the other owned
  profile + password, and the command palette only knew about one of them. They
  are now one screen (`/staff`, "Staff & Admins") with a Panel Scope editor
  absorbed from `/admins`; `/admins` redirects there so old bookmarks survive.
  Quota says *how many*, scope says *where* — they belong side by side.
- **P4 — `/clients/:id` lists the VPNs it counts.** The page reported `VPNs
  owned: 3` and showed none of them, so "is this customer's VPN working?" meant
  leaving for `/vpns` and searching by e-mail. "Their VPNs" reads the same
  `?q=<email>` search the console uses, so the count and the list cannot disagree,
  and each row opens the VPN.
- **P5 — quota is a meter, not a pair of numbers.** `28 / 30` and `2 / 30` occupy
  the same width; `/staff` now draws a bar (warning at 80%, danger at full).
- **P8 — Dashboard "Needs attention".** The four metrics describe the platform;
  none answered "what do I have to do today?". A card now lists only the
  non-zero problems (out of data â†’ expired â†’ expiring â†’ untagged â†’ unreadable),
  each a link to the filtered list that fixes it. Nothing wrong means no card,
  not five zero rows.
- **Verification.** `go test ./...` 12/12 packages (476 test functions, incl. the
  new `TestAdminVPNSummaryBuckets`); `go vet` clean; `tsc` 0; `oxlint` 0/0;
  `npm run build` clean; `audit` OK; `docs:verify` OK. **Live:** summary returned
  `total 4 · premium 3 · untagged 1 · unassigned 1`; tagging `athoof` â†’ FREE
  moved it to `free 1 · untagged 0`; every tile's count matched its own filter
  exactly (`?tag=free` â†’ 1, `?tag=premium` â†’ 3, `?assigned=0` â†’ 1); an issued VPN
  (`dn-test`) refused retagging with `409 tag_locked`; `/clients/24` rendered
  "Their VPNs" whose single row matched its own `vpn_count: 1`; `/dashboard`,
  `/vpns`, `/staff` and `/clients/24` all reported `consoleErrors: []`.


## D-066 · Bulk, search and a row menu: the console stops making the operator repeat themselves

- **Date:** 2026-11-04. Continues D-065 ("what needs me today?"). This one answers
  the follow-up: *"and now do it to twenty of them."*
- **3.1 — bulk classification on `/vpns`.** The untagged queue arrives in batches
  and the decision is usually the same for all of them, but classifying was one
  dialog per row. `/vpns` now uses the same controlled selection `ClientsPage`
  already had (`SelectionBar` + `Table.selection`), with three rules:
  - **issued rows are not selectable.** An issued tag is locked (D-025.2), so
    offering the checkbox and then refusing every row would teach the operator
    that the bulk action is *unreliable* rather than that the tag is *locked*;
  - the writes run **sequentially**, because fifty parallel writes against SQLite
    on a 1 vCPU host is how a bulk action becomes an outage;
  - the result reports **real outcomes** (`7 applied, 1 refused`), never a
    rounded "done" — a partial success is the most likely outcome here and
    hiding it would leave tagged rows the operator believes are untagged.
- **3.2 — the command palette resolves entities, not just pages.** `âŒ˜K` could only
  jump to a screen, so "find janith's VPN" meant: close the palette, open `/vpns`,
  type the e-mail, wait. It now searches VPNs and clients through the console's own
  endpoints and offers the rows themselves, with entities **leading** the list —
  when somebody types a name they mean a person, not a page whose description
  happens to contain that name. Gated on 2 characters and debounced 200 ms, because
  every keystroke would otherwise be two panel-backed queries. `Promise.allSettled`
  keeps a role-refused source (an admin may not read the client console) from
  deleting the other source's results.
- **3.3 — one row menu instead of four targets.** The action column carried an
  "Owner â†’" link, a "Move" button and a "Detail â†’" button inside a row that is
  itself clickable — four targets in one cell, on a phone. The primary intent stays
  a button (Assign / Move); everything else moved behind `â‹¯`. A mis-tap can no
  longer mean the wrong write.
- **A lint rule that was right.** The palette's first cut cleared its results with
  a synchronous `setState` inside the effect, which `react(set-state-in-effect)`
  flagged as a cascading render. The fix was not a suppression: the "is this a
  searchable term" flag is now **derived during render**, and stale results are
  filtered as a value rather than blanked by a write. `useMemo`, not state, for
  anything that is a function of other state.
- **Verification.** `go test ./...` 12/12 packages (476 test functions); `go vet`
  clean; `tsc` 0; `oxlint` **0 warnings, 0 errors**; `npm run build` clean;
  `audit` OK; `docs:verify` OK. **Live:** the palette's two sources answered
  `q=janith` with **3 VPNs and 4 clients**; `/vpns`, `/dashboard`, `/staff` and
  `/clients` all rendered with `consoleErrors: []`.


## D-067 · A reseller can finally see the VPNs it sold — and knows when it is out of quota

- **Date:** 2026-11-04. The Admin audit that followed the Super-Admin passes
  (D-065, D-066).
- **What the audit found first: the reseller console was empty.** Live,
  `reseller@veylix.test` answered `GET /api/servers` â†’ `[]`, `GET /api/vpns` â†’ `[]`,
  `GET /api/clients` â†’ `[]`. Not a bug in those routes: `admin_server_assignments`
  held `server_ids: []`, so the server-side scope refused everything. The Panel
  Scope editor added in D-065 fixes it — assigning `indian-1` immediately produced
  `1 server · quota 0/30 · 1 VPN · 1 client`. This is recorded because the symptom
  ("the reseller sees nothing") looks exactly like a broken scoping query, and the
  next person to see it should check the assignment table before the code.
- **R2 — `/admin-vpns`, "My VPNs".** A reseller could see the *clients* it sold to
  but never the *VPNs* it sold them, so "is Nimal's VPN out of data?" meant opening
  every client in turn or asking the Super Admin — and that is the support question
  that arrives most often. The page lists expiry, kind and status per VPN, with
  tiles for premium sold, quota, expiring â‰¤7d and out of data.
  - Scope comes from `/api/vpns`, not from the screen. The server already returns
    only rows this reseller sold and only rows the Super Admin published (D-025.1);
    a filter applied in the browser would look like a permission and behave like a
    decoration.
  - `days_left` is computed against **one clock read per load**, not `Date.now()` in
    the render: `react(purity)` is right that a re-render must not produce a
    different number for the same row, and a stale-by-a-minute countdown is still
    the truthful one.
  - `null` expiry ("never") is rendered as its own statement, never as `0 days
    left` — the same distinction `days_left` makes on the Super Admin console.
- **R1 — the quota is a meter, not a comment.** `AdminClientsPage` carried the note
  *"per-admin usage is not exposed yet, so 'left' is unknown"* while
  `GET /api/quotas/me` had been returning `{used, limit, remaining}` all along. The
  page now shows `quota 0 / 30 used, 30 slot(s) left` plus the same bar the staff
  console uses (warning at 80%, danger at full). A reseller selling the last slot
  could not previously tell it was the last slot.
- **Verification.** `go test ./...` 12/12 packages (476 test functions); `go vet`
  clean; `tsc` 0; `oxlint` **0 warnings, 0 errors**; `npm run build` clean; `audit`
  OK; `docs:verify` OK. **Live as `reseller@veylix.test`:** `/api/servers` â†’ 1,
  `/api/quotas/me` â†’ `{"used":0,"limit":30,"remaining":30}`, `/api/vpns` â†’ 1,
  `/api/clients` â†’ 1; `/admin-vpns` rendered with the `Premium sold` and `Quota`
  tiles, `/admin-clients` rendered `quota 0 / 30 used, 30 slot(s) left`, and
  `/admin`, `/admin-vpns`, `/admin-clients` all reported `consoleErrors: []`.


## D-068 · The client learns the limit before hitting it

- **Date:** 2026-11-04. The Client audit, after the Super Admin (D-065/066) and
  Admin (D-067) passes.
- **C1 — the FREE allowance is shown BEFORE the claim.** `GET
  /api/vpns/claims/me?server_id=` has returned `{concurrent, used_this_window,
  monthly_limit, concurrent_limit}` since the caps shipped (D-025.4/.5) and the
  client screen **never asked** — a search of `src/` for `claims/me` found zero
  hits. A customer therefore learned the limit by hitting it: claim â†’ 409
  `free_concurrent_limit` â†’ try again later. REQUIREMENTS §2.0.1 requires the cap
  to be visible first, and that requirement has teeth here because **releasing a
  VPN does not refund the monthly slot** — that is a decision somebody must be
  able to make informed, not discover afterwards.
  - Both ceilings are drawn, because they answer different questions: *holding*
    (3 at once) and *this month* (5 per rolling 30 days). Showing one would make
    the other look like a bug.
  - The rule that surprises people is stated in place, not in documentation:
    *"Releasing a free VPN frees a holding slot but does not give the monthly one
    back."*
  - The server is asked, never inferred: `concurrent` counts claim **rows**, and a
    released VPN keeps its row while no longer counting, so summing the client's
    own VPN list would be a different and wrong answer.
  - The meter is **guidance, not a gate**: a refused read hides it and the claim
    button still works.
- **C2 — usage on the card, without the history.** The usage endpoint returns every
  snapshot — **8,145 bytes** for one VPN — which is right for the detail chart and
  wrong for a card that needs two numbers; six cards would pay it six times on a
  1 vCPU host. `GET /api/vpns/{id}/usage?summary=1` now answers only the card's
  question and measured **151 bytes — 54Ã— smaller**. The full history stays the
  default, so no existing caller changes behaviour.
  - `limit_bytes: 0` means unlimited and is rendered as *"used · unlimited"*, never
    as "0 of 0 GB" or a 0 % bar — the same distinction the Super Admin console
    makes.
  - A failed per-VPN read leaves that card without a usage line rather than
    claiming 0 B used.
- **C3 — "12 days left", not a date.** `2026-12-01` requires the reader to know
  today and do arithmetic; `12d left` is the number a customer is actually asking
  for. Under a week it turns amber; past expiry it reads `expired 3d ago`. The
  count is computed against **one clock read per load**, because `Date.now()` in a
  render makes the component impure (`react(purity)`) and could give the same card
  a different answer on every re-render.
- **Verification.** `go test ./...` 12/12 packages (476 test functions); `go vet`
  clean; `gofmt` clean; `tsc` 0; `oxlint` **0 warnings, 0 errors**; `npm run
  build` clean; `audit` OK. **Live as a real client** (`janith14dawan@gmail.com`):
  `claims/me` â†’ `{"concurrent":0,"used_this_window":0,"monthly_limit":5,
  "concurrent_limit":3}`; `/api/vpns/39/usage` â†’ 8,145 B vs `?summary=1` â†’ 151 B
  (`452,458,585` bytes used, `limit_bytes: 0` = unlimited); `/client` rendered the
  allowance rule and reported `consoleErrors: []`.


## D-069 · The whole session, proved against the real gates — not just the unit tests

- **Date:** 2026-11-04. Every change from D-064 onward had been justified by
  `go test ./...`, `tsc`, `oxlint` and a browser render. None of those exercise the
  **auth gate** or the **packaged binary**, so this entry is the proof that the work
  actually ships.
- **`.\dev.ps1 -Preflight` — PASSED, 33 checks.** Run directly, the gate is
  green: backend up; `/api/me`, `/api/admins`, `/api/servers`, `/api/vpns`,
  `/api/clients` all `401` anonymously; wrong credentials `401
  invalid_credentials`; all three roles sign in and read `/api/me`; the RBAC
  matrix holds (`admin`/`client` get `403` on `/api/admins` and `/api/audit`);
  both portals refuse the wrong door with `403 wrong_portal` in every direction;
  logout ends the session (`200 â†’ 204 â†’ 401`); the Vite proxy answers `/api/me`
  correctly.
- **A flake worth recording, because it looks like a defect.** The first attempt
  through `.\dev.ps1 -Restart -Preflight` reported **PREFLIGHT FAILED** — the
  second attempt passed. Cause: `-Restart` performs its own login probe and the
  gate then performs six more, and the login **rate limiter** (D-041) throttles the
  surplus to `429`; the script already labels that case *"(rate limited — see
  note)"* rather than pretending it is an auth failure. Re-running `-Preflight`
  against an already-running backend is the reliable way to read the gate. This is
  recorded so the next reader does not go looking for a broken login.
- **`.\dev.ps1 -Package` — PACKAGED OK.** One binary, one port:
  - `/api/ready` â†’ `ready` **`ui: embedded:7_files`**, `schema_version: 20` (the
    transfer migration is in the released build);
  - `/dashboard` â†’ `200` (SPA), `/vpn/16` â†’ `200` (deep link survives a reload);
  - `/api/me` anonymous â†’ `401` (login enforced in the packaged build too);
  - `/assets/nope.js` â†’ `404` (a missing asset is not swallowed by the SPA);
  - the embedded bundle contains **no development-login markers**;
  - `backend/bin/veylixd.exe` is 20.2 MB.
- **This session's features verified INSIDE that binary**, not just in source:
  `GET /api/super/vpns` returned the full `summary`
  (`total 4 · premium 3 · free 1 · untagged 0 · unassigned 1`), `GET
  /api/vpns/39/transfers` returned its history row, `GET
  /api/vpns/claims/me?server_id=11` returned the caps
  (`concurrent 0 / 3`, `used_this_window 0 / 5`), `GET
  /api/vpns/39/usage?summary=1` returned **151 bytes** (vs 8,145 full), and
  `/client` answered `200`.
- **Nothing was left running.** The throwaway instance on `:18080` was stopped and
  `.\dev.ps1 -Status` confirms the dev stack is healthy on `:8080` (backend `200`,
  auth enforced, real login works) and `:5173` (frontend `200`).


## D-070 · A paying customer can see where their money is

- **Date:** 2026-11-04. Follows the client pass (D-068).
- **A claim I had to retract first.** I listed "`/orders` list page is missing"
  as the next task. **It is not missing.** `App.tsx` has the nav entry, both
  routes (`/orders`, `/orders/:id`) and a working `OrdersPage` — rendered live as
  *"Every order is a payment. Confirming one builds the customer's VPNâ€¦"* with
  `consoleErrors: []`. The page only looked empty because `/api/payments` returned
  `[]` (no orders existed yet). Recorded so nobody rebuilds a screen that works:
  an empty list is not a missing page.
- **C5 — the order timeline.** `/history` printed `· PENDING` and stopped. The one
  question somebody who has just sent money actually asks — *"did it arrive, and
  what happens next?"* — had no answer on the page, even though
  `GET /api/payments/{id}/events` records every step.
  - The three steps are **fixed by the product**, not inferred from the log: order
    placed â†’ transfer reported â†’ seller confirms (and confirming is what builds the
    VPN, D-031). Deriving the step from the order's STATUS means the timeline stays
    honest when the event log is empty or unreadable; the recorded events are shown
    underneath as evidence, not as the only source of truth.
  - Only **open** orders are fetched. A confirmed order's story is finished and its
    result is already on the row; reading a timeline for every historical order
    would be one request each for information nobody is asking for.
  - A timeline that cannot be read **must not hide the order itself** — the failure
    is swallowed per order and the row still renders.
  - The rule that is easy to get wrong is stated in place: *"Confirming is what
    builds your VPN — nothing is created before the payment is confirmed."*
- **Verification.** `go test ./...` 12/12 packages (476 test functions); `go vet`
  clean; `tsc` 0; `oxlint` **0 warnings, 0 errors**; `npm run build` clean; `audit`
  OK; `docs:verify` OK. **Live:** created a real pending order as a client
  (`POST /api/payments` â†’ `201`, id 9, `AWAITING_CONFIRMATION`, `LKR 259.00`,
  reference `TXN-DEMO-001`); `/history` then rendered the timeline's own step text
  (*"Confirming is what builds your VPN — nothing is created before it"*), the
  Records row `payment 9 · AWAITING_CONFIRMATION`, and `consoleErrors: []`.


## D-071 · Two of my own claims were wrong, and the render harness was lying

- **Date:** 2026-11-04. The polish pass, which turned into an audit of my own
  previous reports.
- **Four "missing" features were already built.** I had listed six polish items as
  outstanding. Checked against the tree, four already existed:
  notification badge with unread count (`Topbar.tsx`, fed by `useNotifStore`),
  `Ctrl/âŒ˜+K` (`App.tsx`), `EmptyState`'s `action` prop, and **the iOS setup guide**
  (`VPNDetailPage.tsx` has Android **and** iOS **and** Windows tabs with real App
  Store links). I also claimed `/orders` had no list page — it has a nav entry, both
  routes and a working page. The lesson is the same one this project keeps
  teaching: **an empty screen is not a missing screen**, and a feature is not
  absent because I did not remember writing it. Every claim below was re-derived
  from the tree before acting.
- **The render harness was reporting empty pages that were not empty.** Every task
  in this session asserted `consoleErrors: []`, and more than once a page was
  reported with `tableRows: []`. The cause was the probe, not the app: it settled
  on `/Loading|Refreshing/` **text**, but `AsyncState` renders a **skeleton**,
  which carries no text. `/vpns` takes **1.7 s** (one panel round-trip), so the
  probe declared the page settled while placeholders were still on screen and
  measured an empty table. Fixed by treating a visible skeleton (or spinner) as
  "still loading" — the same fix the probe already applied to text.
  - **Proof of the fix:** `/vpns` went from `commandBar: 0 · tagPicker: 0 ·
    tableRows: 0` to `commandBar: 1 · tagPicker: 4 · tableRows: 4`, and a new
    `skeletonVisible` field reads `0`. The app never changed between those runs.
  - Every Super-Admin page then re-measured with real rows: `/vpns` 4,
    `/dashboard` 1, `/clients` 6, `/staff` 2, `/orders` 1, `/payments` 1 — all
    `consoleErrors: []`.
- **The probe gained named controls**, because two-word buttons are invisible to a
  text extractor that only keeps strings over 40 characters: `savedViews`,
  `commandBar`, `selectionBar`, `tagPicker`, `orderTimeline`, plus
  `emptyStates`/`emptyStatesWithAction` and a `probes` block
  (`skeletonVisible`, `tableRows`, `cardCount`). A future session can now assert
  "this control exists" rather than "some text appeared".
- **4.4 — saved views (the one item that really was missing).** Filters already
  lived in the URL, so a link could be shared, but there was nowhere to KEEP one:
  an operator working the same queue every morning re-built the filter from four
  dropdowns each day. `useSavedViews` + `SavedViewsBar` now store up to eight named
  filter sets per screen in `localStorage`, namespaced `veylix.views.<screen>`.
  - **Local, not a server table.** A saved view is one person's shortcut, contains
    no shared truth, and must survive a reload — a server table would need a
    migration, an owner, a scope rule and a sync story to do the same thing.
  - The hook never interprets a filter, so a screen cannot save a view its own URL
    logic would reject; storage is validated on read and a corrupt entry is dropped
    rather than crashing the page it belongs to.
  - `localStorage` is read **during render** (`useState` initialiser), not in an
    effect: it is synchronously available, and `react(set-state-in-effect)` is right
    that an effect would only add a render.
  - Saving an unfiltered list is refused — that is the page the operator is
    already on, and a shortcut to it would be a control that does nothing.
- **4.6 — empty states that are not dead ends.** 18 `EmptyState` usages had no
  action. Three were fixed where the reader is genuinely stuck: `/admin-servers`
  ("Ask the Super Admin" — the one person who can assign a panel), `/orders`
  ("Check the package catalogue" — an empty queue is usually an empty catalogue),
  and `/vpns` ("Add a 3X-UI server" — the actual first step on a fresh install).
  The rest are lists that fill themselves; adding a button would be decoration.
- **Verification.** `go test ./...` 12/12 packages (476 test functions); `go vet`
  clean; `tsc` 0; `oxlint` **0 warnings, 0 errors**; `npm run build` clean; `audit`
  OK; `docs:verify` OK. Live: Super Admin (6 screens) and Admin (6 screens) all
  `consoleErrors: []`; `/history` shows the order timeline from D-070.


## D-072 · The packaged binary, measured with the probe that was just fixed

- **Date:** 2026-11-04. Closes the loop on D-069/D-071: the earlier "packaged
  binary verified" run used the **broken** probe, so its UI evidence was weak. This
  run repeats it against the fixed one.
- **`.\dev.ps1 -Package` — PACKAGED OK.** One binary, one port: `/api/ready` â†’
  `ready` with `ui: embedded:7_files`; `/dashboard` and `/vpn/16` â†’ `200`;
  anonymous `/api/me` â†’ `401`; a missing asset stays `404`; the bundle carries no
  development-login markers. `backend/bin/veylixd.exe` is 20.2 MB and reports
  **`schema_version: 20`** — the transfer migration is in the released build.
- **Why this run matters more than the last one.** D-071 established that the
  render probe settled on `Loading` **text** while `AsyncState` draws a **skeleton**,
  so a slow panel round-trip (1.7 s on `/vpns`) was measured mid-flight and
  reported as an empty page. The packaged build was previously "verified" with that
  probe. Re-running it with the skeleton-aware settle is the difference between
  "the binary serves HTML" and "the binary renders its data".
- **Packaged binary, all three roles, `consoleErrors: []` everywhere:**
  - **Super Admin** — `/vpns` `commandBar 1 · tagPicker 4 · tableRows 4 ·
    skeletonVisible 0`; `/dashboard` 1 row; `/clients` 6; `/staff` 2; `/orders` 1;
    `/payments` 1.
  - **Admin** — `/admin`, `/admin-vpns`, `/admin-clients`, `/admin-servers` all clean.
  - **Client** — `/client`, `/history`, `/buy` clean; and for the customer who
    actually holds an order (`janith14dawan@gmail.com`), `/history` reports
    **`orderTimeline: 1`** — the D-070 timeline renders from the embedded bundle.
- **A zero that is not a failure.** `/history` reported `consoleErrors: []` and
  `orderTimeline: 0` under the SEED client, who owns no orders. Recording it
  because a future reader will see the zero in a log and wonder: the probe signs in
  by role from `seed.json` unless `VEYLIX_EMAIL`/`VEYLIX_PASSWORD` override it, and
  the seed client legitimately has nothing to show.
- **Nothing left running.** The throwaway `:18080` instance was stopped; the probe
  environment variables were cleared; `.\dev.ps1 -Status` confirms the dev stack is
  healthy (`:8080` backend `200`, auth enforced, real login works; `:5173`
  frontend `200`).


## D-073 · A panic and a 500 used to leave no trace at all

- **Date:** 2026-11-04. The code/security audit, and the only **production-affecting
  defect** found in it.
- **Defect 1 — `withRecover` logged nothing.** `router.go` converted a panicking
  handler into `500 internal_error` and **discarded the panic**. Nothing else in
  the transport layer logged it either. In production that means: a handler
  crashes, the caller sees one failed request, and the operator has **no stack, no
  route, no panic value, no timestamp** — the next occurrence is diagnosed by
  guessing. Fixed: the panic is logged at ERROR with `method`, `path`, `panic` and
  `debug.Stack()` **before** the 500 is written. The **response** still carries none
  of it, because a stack trace is an internal detail an anonymous caller must never
  receive — the two halves pull in opposite directions and both matter.
  - `http.ErrAbortHandler` is excluded. It is the documented way for a handler to
    abandon a response (client disconnect mid-stream); logging it as a crash would
    train an operator to ignore the one line that matters.
- **Defect 2 — there was no access log.** Apart from `"http server listening"`, the
  backend logged nothing per request. The audit trail records **business writes**;
  it does not record reads, refusals or 500s, so "who called this route at 14:02 and
  what did it answer?" had no answer anywhere. Added `LogRequests`, outermost in the
  chain so it sees `SameOrigin`'s 403s and `withRecover`'s 500s with their real
  status.
  - **It deliberately does not log the query string.** Reset links and e-mail
    verifications carry their TOKENS there
    (`/api/auth/verify/<token>`); a log is a file that gets copied, tailed and
    shipped. It logs the path only.
  - It logs no body and no identity: bodies may carry credentials, and the identity
    of a writer is already the audit trail's job — duplicating it into a second
    place is how personal data spreads.
  - Level follows the status (5xx â†’ ERROR, 4xx â†’ WARN, else INFO), so a healthy log
    stays quiet and a failure is not buried. `/assets/` is skipped: one page load is
    a dozen files.
  - `statusWriter` implements `Unwrap()`, so `http.ResponseController` can still
    reach the real writer — otherwise wrapping silently breaks `Flush` on every
    streaming response.
- **Tests (5 new, `internal/httpapi/observability_test.go`):** the panic is both
  logged **and** hidden from the response; `ErrAbortHandler` is not logged as a
  crash; the access log records status and path but **not** the query string; static
  assets are skipped; `Flush` still reaches the underlying writer through the
  wrapper.
- **Verification.** `go test ./...` 12/12 packages (**481** test functions, was 476);
  `go vet` clean; `gofmt` clean; live access log confirmed —
  `{"level":"INFO","msg":"http","method":"GET","path":"/api/vpns/39/traffic/live",
  "status":200,"bytes":127,"ms":15}` and a 401 logged at **WARN**, while a request
  made with `?token=SHOULD-NOT-APPEAR` appeared as `/api/vpns` with the token
  absent.
- **Audited and found sound, recorded so it is not re-litigated:** graceful shutdown
  (`signal.NotifyContext` + `srv.Shutdown` with a 10 s deadline + `st.Close()`),
  SSRF guard on panel URLs, AES-256-GCM token sealing, per-route
  `RequirePermission`, and row-level ownership checks. The 211 `_ =` error
  suppressions were sampled; the ones on `rows.Close()` and audit writes are
  deliberate and documented at their call sites.


## D-074 · Seven files were unformatted while four documents said they were not

- **Date:** 2026-11-04. Found by checking the gates one last time instead of
  trusting the previous run.
- **The finding.** `gofmt -l .` in `backend/` listed **7 files**:
  `internal/audit/audit.go`, `internal/domain/vpn.go`,
  `internal/httpapi/sync_routes.go`, `internal/httpapi/sync_routes_test.go`,
  `internal/store/releases_purge_test.go`, `cmd/dbcheck/main.go`,
  `cmd/veylixd/main.go`. All seven predate this session.
- **Why it matters more than the formatting itself.** Four places asserted the
  opposite: `docs/FINAL_REPORT.md` §"gofmt -l . â†’ nothing printed", and
  `docs/QA_REPORT.md` three times (line 67, 138, and GATE 5 twice). A gate whose
  recorded result is false is worse than a gate that was never run, because it
  stops the next person from looking.
- **The fix** was mechanical (`gofmt -w` on the seven), and the point is what it
  proves: these files drifted after the QA report was written and nothing noticed,
  because nothing re-runs the check. `go build`, `go vet` and all 12 test packages
  passed **before** and **after** the reformat — formatting is not caught by any of
  them, which is exactly why the claim in the docs needed verifying by hand.
- **Verified now:** `gofmt -l .` â†’ **0 files**; `go build ./...` clean;
  `go vet ./...` clean; `go test ./...` 12/12 packages (**481** test functions).
  The four documents' claim is true again as of this entry.
- **Note for the next reader:** this repo has no CI running the formatter (D-029
  ships the build, not the gates). Until one exists, `gofmt -l .` has to be part of
  the manual checklist, or the same drift returns silently.

  `audit` OK, `docs:verify` OK.

## D-075 · GitHub MCP: OAuth client approach abandoned; PAT-with-placeholder, server disabled

- **Date:** 2026-11-07. Owner-approved. Supersedes the `oauthClient` approach
  recorded in `./records/2026-11-07-github-mcp-oauth-config.md`.
- **The finding.** The OAuth App pair the owner supplied no longer works. Live
  evidence: `POST https://github.com/login/oauth/access_token` with
  `client_id=Ov23liWpWahs7rga9AlE` plus the supplied secret returns
  `incorrect_client_credentials` — **identical** to a control call made with a
  deliberately fake secret. The earlier record observed `bad_verification_code`
  for the same pair, which proves the pair has **changed**, i.e. the secret was
  rotated out or never belonged to this App. It is unusable.
- **The second, structural blocker.** A GitHub OAuth App is a *confidential*
  client: `/mcp/readonly` requires a `client_secret` at token exchange. Cline
  offers no supported field for that, and `oauthClient` appears nowhere in Cline's
  published MCP documentation. So `oauthClient.clientId` was never a supported
  configuration — it was recorded `UNVERIFIED` at the time, and is now removed
  rather than left in place pretending to work.
- **The decision.** The `github` entry now uses the standard
  `Authorization: Bearer <token>` header with the literal placeholder
  `PASTE_YOUR_FINE_GRAINED_PAT_HERE`, and is set `"disabled": true`. The owner
  inserts a **fine-grained, read-only** PAT locally and flips `disabled` — the
  token never enters chat, a repo file, `./docs/`, or a record.
- **Why this does not breach Rule 03.** No credential exists on disk: verified by
  substring scan (`5569e85f` absent; no `ghp_`/`github_pat_`/`gho_`/`ghu_`/
  `ghs_`/`ghr_` shape present), and no token was requested or received in chat.
  The placeholder is a marker, not a secret.
- **Why `disabled: true` matters.** With the placeholder in place, enabling the
  server yields `401 invalid_token` on every call. Disabled is the honest state
  until a real token exists; a server that silently 401s is worse than one that
  is explicitly off.
- **Preserved byte-exactly:** `context7` and `playwright` (JSON-normalised
  before/after compare, exact match). Backup
  `cline_mcp_settings.json.bak-patoption-20261107-014711`, pre-edit SHA-256
  `76D01AC00C21C1232082CC4AB1611C4FD5E0DF774B4DCD81689FEC169A62099E`.
- **Not verified:** no `initialize`, no `tools/list`, no tool call — deliberately,
  per the owner's instruction. Status stays **DISABLED**, never `ACTIVE`.
  Evidence: `./records/2026-11-07-github-mcp-pat-placeholder.md`.

## D-076 - The workflow system gains a design-audit workflow and an installer workflow

- **Date:** 2026-11-07
- **Decision:** Two workflows were added and are now the routing targets for their
  domains:
  - `./.clinerules/workflows/system-design-audit.md` - read-only; produces the 13
    audit deliverables using only the five statuses (`IMPLEMENTED AND VERIFIED` /
    `IMPLEMENTED BUT NOT VERIFIED` / `PARTIALLY IMPLEMENTED` / `NOT IMPLEMENTED` /
    `BLOCKED`), separates confirmed requirements from proposed choices, and
    **ends at an owner approval gate**.
  - `./.clinerules/workflows/installation-and-deployment.md` - governs any change
    to the installer, upgrade path, `vpn-platformctl`, Caddy/systemd templates,
    backup/restore or uninstall. Carries the data-loss checks and the
    `--dry-run`-first rule.
  - `./.clinerules/skills/veylix-deployment/SKILL.md` - the mechanism: the
    three-way fresh/upgrade branch, `flock` guard, secret generation, checksum
    verification, preflight ordering, log redaction.
- **Why:** the owner requested an audit of a proposed installation system, and it
  became clear that **no workflow governed that class of work** - the six
  existing workflows are built for changing the product, not for auditing and
  designing a subsystem. The 13-deliverable audit had nowhere to run. The
  installer is a permanent product surface required by Bible section 14 and
  acceptance tests 30-39, and it was reachable only as a verification stage
  inside `release-readiness.md`.
- **Data-loss is now its own axis.** `system-design-audit.md` Stage 5 and
  `installation-and-deployment.md` Stage 1 own data-loss risk as a separate axis,
  not as a security sub-category. `./VPN_FIX_LEDGER.md` records a verification
  script deleting live data **twice**; that lesson was previously prose in one
  skill and appeared in no checklist.
- **Single owner for the fresh-install check list:** `release-readiness.md`
  Stage 5b. Both new files point at it instead of restating its 13 checks.
- **Also changed:** `feature-development.md` gained a conditional **Stage 0**
  that delegates to `system-design-audit.md`; `./.clinerules/01-project-core.md`
  and `06-tooling-and-routing.md` index both new files.
- **Preserved:** no existing workflow rule was weakened. The PASS / FAIL /
  BLOCKED / NOT RUN vocabulary, the read-only rule for `security-review.md` and
  `code-review.md`, and the Playwright-vs-`verify-*.mjs` boundary are unchanged.
- **Not verified:** Cline UI activation of the new workflow and skill requires a
  window reload. The files are valid Markdown with the required skill
  frontmatter, but their listing in the Cline UI was not confirmed in this pass.
- **Gates (actually run, this pass):** `go build ./...` exit 0 - `go vet ./...`
  exit 0, no output - `go test ./... -count=1` **12/12 packages ok** -
  `npm run lint` 0 warnings / 0 errors, 175 files - `npm run audit` OK -
  `npm run docs:verify` OK (21 migrations, **125 routes**, **518** test functions
  in 13 packages) - `check-workflows.mjs` OK, 2 files.
- **Correction to an earlier claim in this session:** the route count is **125**
  (`docs:verify`, derived from the tree), not the 128 reported from a raw
  `mux.Handle` grep - that grep counted multi-line registrations. The tool is the
  authority.

---

## D-079 - Installation system: bootstrap installer plus a permanent Go CLI

**Date:** 2026-11-07. **Trigger:** Bible §14 and §18 tests 30-39 were **Missing**
or runbook-only; `docs/INSTALL.md` §F recorded the one-line installer and
`vpn-platformctl` as "not built". The owner asked whether an installer built
today would be invalidated by later project updates, and then approved building
the design in Act mode.

- **The split that answers the owner's question.** The whole application - Go
  code, every SQL migration and the entire React UI - is compiled into **one
  `veylixd` file**. Therefore a change to business logic, a new migration or a new
  page reaches an installed host as a **new binary**, and the installer does not
  need to change at all. `install.sh` is a **one-time bootstrap**; `vpn-platformctl`
  is the **permanent** update path. `install.sh` consequently contains **no
  version number and no schema knowledge** - it resolves the release channel at
  run time (`releases/latest/download`).
- **Added, in this order:**
  1. `scripts/lib/common.sh` - `die` / `say` / `redact` / `acquire_lock` /
     `gen_key` / `fingerprint` / `verify_checksum` / `wait_for_ready` /
     `validate_domain` and the preflight primitives. Every deploy script sources
     it instead of reimplementing those.
  2. `backend/cmd/vpn-platformctl` - `status` `doctor` `update` `rollback`
     `domain-set` `backup` `restore` `repair` `uninstall`. **Go, not shell**:
     versioned with the binary it manages, covered by `go test`, and unable to
     drift from a second implementation of the same checks. Imports nothing from
     `internal/`; touches no business rule.
  3. `install.sh` - preflight (root, lock, OS, arch, disk, RAM, ports, DNS,
     artifact) then phases: download+checksum, **branch**, user+dirs, secrets,
     config, binaries, systemd, caddy, report.
  4. `.github/workflows/release.yml` - also builds and checksums
     `vpn-platformctl-linux-<arch>`, so a release that omits the CLI is not a
     release.
  5. `docs/INSTALLATION_DESIGN.md` - the design of record (§1-§12).
- **Data-loss rules, all structural rather than advisory:** the branch decides
  from **the database**, not the binary; keys are generated **only when absent**
  and an existing key plus a fresh-install intent is a `die`; an artifact that
  fails its checksum is **deleted**; a backup is taken before any destructive
  path; `uninstall --purge-data` needs the flag **and** the operator typing the
  domain; `repair` has no code path that writes to the data or key directories;
  `removeAll` refuses root and top-level system directories.
- **Truthful-status rules:** HTTPS is claimed **only** after
  `https://<domain>/api/ready` returns 200; a DNS/ACME failure exits **2**
  ("installed, not publicly served") and is never reported as success. Every
  credential printed is a **fingerprint**, never the value.
- **Exit codes are part of the contract:** `0` success, `1` failed, `2` bad
  arguments, `3` installed-but-not-publicly-served (CLI) / installer uses `2`
  for incomplete.
- **Gates actually run this pass:** `go build ./...` exit 0 - `go vet ./...`
  exit 0 - `gofmt -l .` **clean** - `go test ./... -count=1` **13/13 packages
  ok** (the new package is the 13th) - `npm run lint` 0/0 - `npm run audit` OK -
  `npm run audit:selftest` OK (8 positive + 6 negative) - `npm run docs:verify`
  OK (21 migrations, 125 routes, 518 test functions in 13 packages) -
  `npm run workflows:check` OK, 2 files - `scripts/lib/common.selftest.sh`
  **31 PASS**, 1 SKIPPED - `scripts/install.selftest.sh` **31 PASS**, 1 SKIPPED -
  `bash -n` on `install.sh`, `common.sh` and both self-tests, exit 0 - linux
  cross-build of `vpn-platformctl` exit 0.
- **BLOCKED, stated plainly:** no clean-VM install has been run, no certificate
  has been issued, no backup/restore has been rehearsed on a clean VM, and no
  1 vCPU / 500 MB benchmark exists. Bible tests 30-34, 37, 38 and 39 are
  **built, not verified**. `go test -race` remains BLOCKED (no C compiler).
- **Two real defects found and fixed while building this** (both recorded in
  `IMPROVEMENT_LEDGER.md`): `write_secret` reported success while `install -m`
  had failed on a non-root host; and parsing flags twice in `commonFlags` made
  `update --channel` / `uninstall --purge-data` unreachable.
- **Supersedes nothing.** This appends to D-076 (which created the workflows this
  work executes) and to the `INSTALL.md` §F rows that previously read "not built".
---

## D-080 · The Google client is edited in the app, and the secret is sealed like a panel token

**Date:** 2026-11-07 · **Status:** Accepted

**Context.** The Google OAuth client (D-039) could only be changed by editing
`/etc/veylix/veylix.env` over SSH and restarting the process. The owner asked for
it to be editable after a Super Admin signs in, with the change applied live —
the same "settings" idea 3X-UI exposes in its panel menu. `.veylix-secrets/` and
`server_integrations.token_ciphertext` were the only two places a P0 secret was
allowed to live, so a new storage location is a deliberate decision, not an
incident waiting to happen.

**Decision.**

- **New table `app_settings(key, value_ciphertext, is_secret, updated_at,
  updated_by)`** (migration `0022_app_settings.sql`). `value_ciphertext` is a
  sealed blob when `is_secret = 1`, raw UTF-8 otherwise. This is the **third**
  approved home for a sealed secret, and it follows the same rule as the other
  two: AES-256-GCM via `auth.Sealer`, **AAD-bound to the key**
  (`app_settings:<key>`), so a ciphertext copied onto another row fails to open.
- **Only `store` writes SQL.** `internal/store/app_settings.go` owns the two
  queries; `internal/settings` orchestrates sealing and audit and imports no SQL
  driver. `httpapi` imports `settings` for its view/input types — one direction,
  no cycle.
- **The secret is write-only.** `GET /api/super/settings/google` returns
  `has_secret: true`, never the secret. `PUT` accepts an empty `client_secret`
  meaning "keep the stored one", so an operator can change the client id without
  re-typing a secret they can no longer read.
- **DB-first, env fallback.** `settings.Resolve` reads the database over the
  environment, so an installation that only ever set `VEYLIX_GOOGLE_*` keeps
  working unchanged. `config.Load` still parses the env values; they are the
  fallback, not the source of truth once an operator saves.
- **Applies live.** `settings.Service.OnGoogleChange` rebuilds the Google client
  in the running process after a save. The response reports what the process now
  holds. No `systemctl restart` is invoked from the web app: a web process with
  root is a far bigger risk than the restart it would save (the `A5c` option was
  rejected for exactly that reason).
- **Access: Super Admin only** (`PermServerViewAll`), audited as
  `settings.update` (the secret is never in the audit — only `secret_set`).

**Validation.** Client id must end in `.apps.googleusercontent.com`; the secret
must start with `GOCSPX-`; the redirect must be an absolute `http(s)` URL. A
first-time save without a secret is refused, because a half-set client that
reports `configured` would be a lie.

**Tests.** `internal/settings` (7): the secret is sealed and not stored in
plaintext, database beats environment, a blank secret keeps the stored one,
shape validation, first-time-needs-a-secret, the live-change callback fires.
`internal/httpapi` (6): masked GET, PUT plumbs the principal id + secret, invalid
JSON â†’ 400, validation error â†’ 400, a non-super-admin â†’ 403, a nil service â†’ 404.

**Gates run this pass.** `go build ./...` exit 0 · `go vet ./...` exit 0 ·
`gofmt -l .` clean · `go test ./... -count=1` **15/15 packages ok** · `npm run
lint` 0 errors · `npm run build` exit 0 · `npm run audit` OK · `npm run
docs:verify` OK (22 migrations, 133 routes, 560 tests in 15 packages).

**BLOCKED, stated plainly.** The live OAuth round-trip against a Google tenant
was **not** re-run this pass (no credentials in this environment); the settings
storage, masking, validation and access control are unit- and route-tested, but
"a real sign-in with a saved client" remains NOT PERFORMED here.

**Supersedes nothing.** Extends D-039 (the Google flow) and D-016 (AES-GCM at
rest) to a new, approved storage home.

---

## D-081 · Version is visible in the product, and a downgrade restores matching data

**Date:** 2026-11-07 · **Status:** Accepted

**Context.** An operator could not tell, from inside the product, which release
was running — only `vpn-platformctl version` on the box answered it, and a
`--version <tag>` on the updater was silently blocked by the schema guard, so a
"downgrade" wrote the old binary over a database migrated by the new one and then
failed. The owner asked for three things, modelled on 3X-UI's `x-ui -v` plus its
update menu: show the version, say whether an update exists, and let any published
version be installed — including going backwards.

**Decision.**

- **The RUNNING PROCESS is the authority on the current version.** `/api/ready`
  now returns `version`; `GET /api/system/version` returns version, applied
  schema, whether the UI is embedded, and the Go/platform. A file on disk can be
  replaced without the process restarting, so it is never the evidence.
- **`internal/versioning`** answers "what else exists and is one newer?" over the
  release feed, with a 1-hour TTL cache (GitHub rate-limits unauthenticated
  callers). A failed read returns an **explicit error**, never "up to date".
  Version comparison is numeric (`v1.10.0 > v1.9.0`), and a `dev` build sorts
  below every numbered release so it reads as out of date, not current.
- **`GET /api/system/releases`** (Super Admin) lists every release with
  `is_current` / `is_latest`. **`GET /api/system/update-check`** compares the
  running version against the newest stable release and returns `error` instead
  of a fabricated answer when the feed cannot be read.
- **`vpn-platformctl versions`** prints the same list on the box and marks the
  one running. **`install.sh --list-versions`** lists published tags before a
  fresh install, so `--version <tag>` has a discoverable set to choose from.
- **Downgrade is allowed, but only as a data move.** Migrations are forward-only,
  so an older binary cannot read a database a newer one migrated. Therefore
  `update --version <older>` requires **both** `--allow-downgrade` (to mean it)
  and `--restore-backup <dir>` (to be able to do it). The flow swaps the binary,
  then restores the backup **before** the older binary starts — and never runs
  the "schema went backwards" check that is correct for an upgrade and wrong for
  a downgrade. Without the flag a downgrade is a **usage error**, not a silent
  file replacement. `--dry-run` is propagated into the restore it triggers.

**Honest limits.** The UI shows the update and copies the exact
`sudo vpn-platformctl update` command; it does **not** install an update itself —
that needs root, and a web process with root is a rejected trade-off (D-080).
The release feed is read from GitHub (`VEYLIX_REPO`, default `your-org/veylix`);
a fork points `VEYLIX_RELEASE_API`/`VEYLIX_REPO` at its own.

**Tests.** `internal/versioning` (service: caching, comparison, marked
releases, error-not-fresh). `internal/httpapi` system routes (version, releases,
update-check, 503-when-unwired, auth). `cmd/vpn-platformctl` (versions reads a
local feed; downgrade refused without the flag; refused without a restore dir;
refused when the dir is missing; an upgrade is not misread as a downgrade).

**Gates run this pass.** Same table as D-080: build/vet/gofmt clean, `go test`
15/15, lint/build/audit/docs:verify OK.

**BLOCKED, stated plainly.** No real downgrade was executed against a live
installation in this environment; the guard logic and the ordering are unit-
tested, but "install an older release over real data" remains NOT PERFORMED here.

**Supersedes nothing.** Extends D-079 (the installation system and its CLI),
which this work turns from "install and upgrade" into "see, choose, and roll
back".

## D-082 · The Google redirect URL is derived from the request, and must end at the one callback path

**Context.** A Google sign-in fails with `redirect_uri_mismatch` whenever the URL
registered in Google Cloud Console differs from the one the server sends — and
that error reads as "Google is broken", not "the path is wrong". The Settings page
(D-080) let an operator type a redirect URL by hand, so one typo (a wrong path, a
`www.`, an `http`, a trailing slash) silently disabled the whole feature with an
error that blames the wrong party. The server already knows the truth: the
callback is served at exactly one path, `GoogleCallbackPath =
/api/auth/google/callback`, and the origin is the request's own host.

**Decision.**
- **Derive, do not trust the typed value.** `GET`/`PUT /api/super/settings/google`
  return `expected_redirect_url` = `schemeHost(r) + settings.GoogleCallbackPath`.
  `schemeHost` is the mechanism the CSRF guard already trusts (the request's own
  Host, honouring `X-Forwarded-Proto` only from a loopback peer), so the value is
  the URL the running server actually answers at — verified in `DOMAIN_CHANGE.md`,
  not invented here. The stored value is left untouched (a read still reports it);
  the derived value is what the page hands over.
- **Show it, and let it fill the field.** `SuperSettingsPage.tsx` renders the
  derived URL with a **Copy** button and a **Use this URL** button that writes it
  into the Redirect URL field — so the operator registers the exact string and a
  typo is not possible.
- **Reject a wrong path at save time.** `settings.validateRedirect` now requires
  the parsed path to **end with** `GoogleCallbackPath`. A leading sub-path is
  allowed (a reverse proxy that rewrites `/veylix/...` still ends at the
  callback), but a trailing slash, a missing tail, or another provider's shape
  (`/oauth2/callback`) is a `ValidationError` â†’ `400 invalid_setting`.
- **One home for the string.** `settings.GoogleCallbackPath` is the single
  constant; the HTTP layer imports it rather than keeping a second copy.

**Why strict rather than a warning.** The consequence of a wrong path is the
entire feature failing behind a misleading error. A save that cannot work should
not be accepted; the operator-approved default was strict. The cost — a
reverse-proxy deployment whose URL does not end at the callback must fix its
rewrite — is the correct cost, because such a URL would not work anyway.

**Tests.** `internal/settings`: `TestSaveRequiresTheCallbackPath` (five wrong
shapes refused, the canonical path and a proxied sub-path accepted);
`TestSaveValidatesShapes` gained a wrong-suffix case; every existing save fixture
was moved onto the real callback path. `internal/httpapi`:
`TestSettingsGetDerivesTheExpectedRedirectFromTheRequestHost` (a proxied HTTPS
request yields `https://vpn.acme.com/api/auth/google/callback`) and the GET test
now asserts the field is present and ends at the callback.

**Honest limits.** No live Google round-trip was performed here (no credentials,
no tenant); the derivation, the refusal and the fill are unit/route-tested, and
the real tenant path stays NOT PERFORMED. `docs:verify` moved 562 â†’ 564 test
functions, which README was corrected to.

**Supersedes nothing.** Extends D-080 (the editable Google client) by making the
one value most likely to be mistyped impossible to mistype.


## D-083 · A VPN row carries the server it is on and the seller who sold it

**Status:** implemented. **Scope:** backend lister + frontend display.

**Decision.** `VPNRow` gains `server_name` and the seller identity
(`seller_name`, `seller_email`, `seller_whatsapp`, `seller_telegram`). Both the
SQL lister and the live panel lister populate them; `panelViewToRow` gained a
`serverName` parameter.

**Why.** A VPN list that shows only an e-mail address does not answer the two
questions an operator and a client both ask first: *which server is this on* and
*who do I talk to about it*. The data was already in the database
(`server_integrations.name` and the seller's `users` row); it simply was not
selected. This is a read-side change — no new table, no new endpoint, no write.

**Consequence.** The client portal can render "sold by <name>" with real
`wa.me` / `t.me` links, and the operator list can show the server per row,
without inventing any contact detail.

**Tests.** `internal/httpapi/vpn_lister_identity_test.go` (2 tests).

---

## D-084 · The navigation is three role-scoped trees, not one filtered array

**Status:** implemented for stages 2â€“3 (design tokens + shell/navigation).

**Decision.** `frontend/src/components/layout/navModel.ts` replaces the old flat
`NAV` array in `App.tsx`. Each role — Super Admin, Admin, Client — gets its own
tree of labelled groups, returned by `navigationFor(role)`. The Topbar and
Breadcrumbs read the same module, so there is exactly one definition of "where
can this role go".

**Why.** The audit found the previous model was one array of 22 entries in two
generic groups ("Main", "Operations"), each item carrying a `roles` filter. The
observable results were:

- a Super Admin sidebar of 12 items in which *Settings* and *Rate card* sat
  beside daily *Orders* and *Payments*, with no hierarchy;
- two different nav items both labelled **Clients** (`/clients` for the owner,
  `/admin-clients` for the reseller) and two both labelled around "My VPNs";
- a **client** sidebar of 6 entries of which none named a server, a connection
  or usage — and `/vpn/:id`, the only VPN-centric screen in the product, had no
  navigation entry at all.

Three roles are three different products. One filtered array cannot express
that; three trees can.

**Route paths are unchanged.** 30+ committed `verify-*.mjs` scripts and the
server-side role guards depend on those strings, so only labels, grouping and
ordering were redesigned. Every destination in `navModel.ts` is a route actually
declared in `App.tsx` — inventing one would create a dead link that a role guard
turns into a silent redirect, which is the exact defect being removed.

**New shell capability.** Breadcrumbs (`Breadcrumbs.tsx`) derived from the route
via a `TRAILS` table, rendering nothing at depth 1; a role badge in the Topbar;
collapsible nav groups (`> 4` items, persisted, and force-opened when they
contain the active route so the sidebar can never show no active item); and the
mobile drawer now closes on navigation.

**Design tokens.** `tokens.css` gained an appended, additive block: a three-tier
surface ladder (`--panel-rail/header/body`), a semantic state ramp
(`--state-online/degraded/offline/pending` + washes), one connection accent
(`--vpn-accent`) and `--ring-focus`. Every value is a `color-mix()` of an
existing token, so all six presets inherit them and no page needs a colour
literal. The audit also established that `--elev-1..3`, `--space-*`, `--z-*` and
the entrance-motion tokens **already existed per-theme**, so no duplicate ladder
was introduced.

**Tests.** `frontend/scripts/verify-nav.mjs` extended from reachability only to
reachability **plus role isolation** — each role must not be offered another
role's destinations. Result: **58 passed, 0 failed** (23 positive, 35 negative).

**Honest limits.** The client portal's VPN *hero card* and the three role
overviews are later stages and are not implemented by this record. `/vpn/:id`
remains client-only by design; `verify-vpn-actions.mjs` hardcodes `VPN_ID = 36`
and the live database holds `[38, 39, 40]`, so that script fails on stale data
independently of this change.

**Supersedes nothing.** Replaces the `NAV` constant inside `App.tsx`; extends
D-004 (single shell + role-filtered navigation) by giving each role its own tree.


---

## D-085 · The outbox is retried by a background worker with a hard attempt ceiling

**Status:** implemented.

**Decision.** Two things, because either alone is incomplete.

1. `main.go` runs a goroutine that calls `provisioning.Reconcile(ctx, 25)` every
   two minutes, once immediately at startup, recovering from a panic per tick.
2. `provisioning.MaxOutboxAttempts = 8` caps retries. On the 8th failure the row
   becomes `FAILED` with `ErrOutboxExhausted` and no `next_attempt_at`, and the
   pass reports the new `ReconcileExhausted` outcome.

**Why.** `Reconcile` was fully implemented and covered by three tests but had
**zero production callers** — `main.go` built `provSvc` and never scheduled it.
`pushCreate` deliberately enqueues an outbox row when a panel write's outcome is
unknown (a timeout mid-write), and that row lands in `RETRY`. Nothing retried it.
The recovery path the architecture promised did not exist at runtime, so a VPN
whose create timed out stayed `SERVER_UNAVAILABLE` until a human fixed it by
hand.

The ceiling is the other half. `backoff()` saturates at 6 â†’ 640 s, so without a
cap a permanently broken operation — a deleted inbound, a panel the token no
longer opens, a client the panel rejects by validation — is retried roughly 135
times a day forever. On a 1 vCPU box that is real waste, and a `RETRY` that never
resolves never tells anyone there is a problem.

**What it deliberately does not do.** Exhaustion does **not** write `REVOKED`.
D-024 says the panel wins and Veylix never invents a remote fact; after 8 failed
attempts we still cannot prove the panel lacks that client, so the VPN keeps
whatever honest state the outage gave it (`SERVER_UNAVAILABLE`). `EXHAUSTED` is
also kept distinct from `RESOLVED`: `RESOLVED` means "proven impossible" and
nothing can fix it, while `EXHAUSTED` means "we gave up" — the panel might come
back, so that one is worth re-queueing by hand.

**Tests.** `TestReconcileStopsAfterMaxAttempts` (row reaches `FAILED`, no next
attempt, a later pass ignores it) and `TestReconcileRetriesBelowMaxAttempts` (the
cap does not fire one attempt early). Disabling the cap makes the first test fail
with `outcomes = map[RETRY:1], want 1 EXHAUSTED`.

## D-086 · Panel reads are cached for five seconds, per server, sharing concurrent callers

**Status:** implemented.

**Decision.** `httpapi.CachedPanelInspector` wraps the panel inspector and caches
each server's client list for `DefaultPanelCacheTTL` (5 s). Concurrent reads of
the same server collapse into one panel call. A failed read is returned as an
error and memoised only for `ttl/5`, so recovery is visible on the next refresh.
`Invalidate(id)` / `Invalidate(0)` force a fresh read. **One** instance is built
in `main.go` and handed to every consumer.

**Why.** Measured, not assumed. Over 1,969 requests in the production log,
`GET /api/{super/,}vpns` had a **1,044 ms median**, **579** requests over a
second, **26** over ten, and a **44,802 ms worst case**, with **1,947 s** of total
wall time — while every other endpoint answered in 0â€“8 ms. The cause is that the
panel is the truth (D-024.3) so the console re-reads the whole panel per request
(`integrations.PanelClients` pages up to 50 Ã— 200 clients), and the tree had no
cache at all.

After the change, measured live as super admin over 12 sequential requests:
**cold 480 ms, then 3.2â€“6.3 ms, median 3.9 ms**, identical payload every time.

**Why this does not weaken D-024.** A cached entry is a copy of a read the panel
already answered, discarded after 5 s. It is short enough that a client created
directly in 3X-UI appears on a manual refresh, and the background sync already
re-reads every panel every 5 minutes. Nothing is cached as a *success* that was
not one: an unreachable panel still returns an error, so the existing
`ReadError`/"unread" handling keeps reporting the outage honestly instead of
rendering a stale list as current.

**Wiring trap, recorded on purpose.** The first attempt passed the raw
`integrationsSvc` to `NewPanelVPNLister` while only `PanelInspector` got the
wrapper. The lister reads the panel directly, so it bypassed the cache entirely
and the endpoint stayed at 545 ms — the fix looked like it had failed. That is
why the instance is now constructed once into `panelReads` and given to both
call sites, with a comment saying why it must stay that way.

**Tests.** `panel_cache_test.go` (7 tests): repeated reads collapse to one panel
call; the entry refetches after the TTL; a failure is never cached as an empty
list; an outage is re-tested sooner than the full TTL; 12 concurrent readers
produce exactly 1 panel call; `Invalidate` forces a reread; a nil inspector stays
nil.

**Trade-off accepted by the owner.** Up to 5 s of staleness on the Super Admin
VPN list, in exchange for removing a 1-second median from every console
interaction.

---

## D-087 · A release record must outlive the VPN it describes (I-33)

**Decision.** `vpn_releases` is a **tombstone table**. Its `vpn_instance_id` is
nullable and `ON DELETE SET NULL`, and the panel identity (`panel_email`,
`server_id`) is stored on the release row **by value** rather than joined at read
time. Migration `0023_release_tombstone.sql` rebuilds the table to say so.

**Why.** `vpn_releases` was created (migration 0006) with
`vpn_instance_id INTEGER NOT NULL REFERENCES vpn_instances (id) ON DELETE
CASCADE`. With `foreign_keys(ON)` on every connection, `RemoveReleasedVPN` did two
contradictory things inside one transaction: it stamped `removed_at` to record
that the VPN had been purged, then deleted the `vpn_instances` row — and the
cascade deleted the record that had just been stamped. The table emptied on every
purge. A tombstone destroyed by the event it exists to record is not a tombstone:
there was no reachable "removed" state, so `GET /api/vpn-releases` could never
show a client that their VPN had been removed, only that it had been released.

**Alternatives rejected.**

1. *`ON DELETE RESTRICT`.* Would make the purge fail outright until the release
   row is deleted first — turning a working cleanup path into an error path, and
   still leaving no record behind.
2. *Remove the FK entirely.* The FK is doing real work before a purge: it
   guarantees a release always names an existing VPN. `SET NULL` keeps that
   guarantee for live rows and gives the honest answer (`NULL`) afterwards.
3. *Stamp `panel_email` only at purge time.* Tried first and **caught by the red
   test run**: a release created today had an empty `panel_email` until the purge,
   so every live release in the client's history was unidentifiable. Identity is
   therefore written at **creation**, and re-stamped at purge from the same
   snapshot the audit entry already used.
4. *Leave it and rely on the audit log.* Rejected: `audit_logs` is an operator
   record. The client reading their own timeline is the affected party, and they
   cannot read the audit log.

**Consequences.**

- `ReleaseRow` gains `panel_email` and `server_id` on the wire; a purged release
  reports `vpn_id: 0`, which is the documented meaning of "no VPN to point at".
  Recorded in `./docs/API_CONTRACT.md`.
- `scanRelease` reads `vpn_instance_id` through `sql.NullInt64`. Every other
  reader of the column was audited: the only remaining NULL-sensitive site is the
  purge's own lookup, which now refuses an unmoved release that has no pointer
  rather than guessing.
- The migration **deletes no row**. Pending, restored and already-removed
  releases are all copied, so an installation in the middle of a 3-day window is
  unaffected.
- Same-class cascades on `vpn_transfers` (I-34) and `usage_snapshots` (I-35) were
  found during this audit and deliberately **not** changed here. One migration,
  one decision, one review.

**Supersedes** nothing. **Relates to** D-027 (release lifecycle), D-027.3 (a
purge never touches the panel).



---

## D-090 · Client Dashboard — VPN Summary and In-Progress Orders

**Decision.** The Client Dashboard at `/client` is a **summary-only surface**. It
answers exactly three questions:

1. What does the client own?
2. What needs the client's attention?
3. What orders are still in progress?

It reads **only** existing, role-scoped API endpoints. It never provisions a VPN,
never confirms a payment, never claims a Free VPN and never writes to a panel.

A number is displayed **only** when its source provides a reliable value. An API
failure or unavailable data is **never** converted into zero: a failed read
renders an explicit unavailable state rather than pretending no records exist,
because "none" and "unknown" are different facts. Where
`./MASTER_PRODUCT_BIBLE.md` requires a count that existing APIs do not reliably
expose, the count is **omitted** and the limitation documented here rather than
approximated.

**Why.** S3 split `/client` into a Dashboard and `/client/vpns`. The Dashboard
kept the verdict line, four counters and the primary connection card, and every
one of those four counters is derived from the VPN list alone
(`ClientDashboardPage.tsx:172`, `:175-177`, `:183-187`, `:280-282`). Nothing on the
surface told a client that an order of theirs was sitting with an operator —
`ClientDashboardPage.tsx` contained **zero** references to payments. Bible §267
requires the purchase surface to retain Premium records and §308 requires the
client to keep seeing them, so a client with a pending order had no dashboard
signal at all. Answering "is something of mine waiting on someone else?" requires
a decision about which payment facts are safe and honest to aggregate — that is
what this record settles.

**Approved behaviour.**

- **Preserved unchanged:** the primary VPN hero card, the verdict line, the four
  existing counters, the Free VPN claim card, the claim-allowance information and
  its explanatory copy, and the purchases/history navigation. Nothing existing is
  removed or weakened.
- **Added — one compact section, "Orders in progress",** fed by the existing
  `GET /api/payments?pending=1`, whose store-side filter is
  `status IN ('PENDING','AWAITING_CONFIRMATION')`.
- The section is **not rendered at all** when no orders match — no empty card and
  no zero, consistent with the existing rule that the counter strip is omitted
  rather than drawn as four zeroes.
- When orders match it shows a suitable **non-numeric indicator** plus a link to
  `/history`. The combined result must **not** be labelled "awaiting operator
  confirmation", because the filter spans two states with different meanings.
- On failure it shows "Could not load order status" with a `/history` link, and
  never a zero.
- Exactly **one** payment-list request on Dashboard mount; it does not re-fire on
  unrelated VPN usage updates.

**Consequences.**

- No new endpoint, field, table or migration. `GET /api/payments` and its
  `?pending=1` filter already exist (`payment_record_routes.go:17-29`) and the
  client binding is already written (`api.ts:1811-1812`).
- Client scope stays enforced **in the store** (`PaymentsVisible` takes `payer_user_id = ?`), never by the query string, so a client cannot read another payer's rows. No privileged request, permission or endpoint is added.
- The extra read is one indexed local query (`idx_pay_status`), so the 1 vCPU / 500 MB ceiling is unaffected and no panel is contacted.
- Because the store caps the list at `LIMIT 200`, the Dashboard shows an indicator rather than a count. That is the documented limitation required above: an exact total is unavailable until an API can state one authoritatively for the complete filtered result.

**Out of scope for this decision.** Deposit classification (deferred to the S7 Payments & Billing stage); total money spent, revenue, outstanding balance, deposit totals, per-order amounts and any financial summary; changes to payment state transitions or payment business logic; confirming, rejecting, cancelling or refunding a payment; any Super Admin or Admin functionality; new API endpoints, database fields, tables or migrations; changes to provisioning, integrations or panel code; Free VPN claims; VPN provisioning; external 3X-UI writes; S5, S6 or S7 implementation; navigation destination additions; Advanced QA infrastructure.

**Supersedes** nothing. **Relates to** D-025.4/.5 (Free claim caps, and the rule that releasing does not refund a slot), D-031 (one payment confirmation is the release), D-032 / D-037.4 (role-scoped listing), D-056 / D-058 (touch targets), D-084 (role-scoped navigation).


---

## D-088 · My VPNs — three categories, one list, two views

**Recorded** 2026-11-08 on the owner''s explicit authorisation, before Section 02
(My VPNs) was implemented. This record documents an owner-approved product
decision; it introduces no new requirement the owner did not state.

**Decision.** The client screen at `/client/vpns` presents ONE inventory of the
VPNs the client owns, organised by exactly **three** categories and available in
two display modes.

1. **Categories are `All`, `Free`, `Premium` — three, not four.** There is no
   `Custom Premium` filter. A Custom Premium VPN is **a Premium VPN**: the
   customer bought a premium package whose specification they chose, and that is
   a pricing detail, not a different kind of VPN. Presenting it as a fourth
   category would tell a customer their Premium VPN is not Premium.
2. **The underlying `is_custom` information is preserved, never discarded.** A
   custom package''s specification (traffic, duration, device limit) may be shown
   on the Premium card or its detail screen **where the API actually sends it**.
   Where it is not sent, it is omitted — not guessed, and not replaced by a
   category that implies it.
3. **The database enum is not changed and no new category semantics are
   invented.** The wire values stay `FREE` / `PREMIUM` / empty, exactly as
   `store/offers.go` writes them.
4. **Page size stays nine.** This preserves the S3 behaviour and the per-page
   bound that keeps a 1 vCPU host from drawing an unbounded list.
5. **Two display modes: Cards and Compact List.** They render the **same**
   already-fetched rows. Switching views issues **no** request and duplicates no
   dataset.

**Why.** Section 01 established that a client screen states facts the server
sent. A fourth category would be a fact the server does not have: `offer_kind`
carries `FREE` or `PREMIUM` only, and `assigned = true` normalises an
unclassified owned row to `PREMIUM` (`vpnStore.ts:173-180`). Meanwhile a client
managing 50-100 VPNs needs a dense view the S3 card grid cannot provide, and a
client managing three needs cards, not a table.

**Approved behaviour.**

- **Overview counts cover the WHOLE owned inventory**, never the visible page and
  never the filtered subset. `GET /api/vpns` already returns every owned row for
a client (the server scopes it to the caller), so the count is a count of that
  list and requires no new endpoint.
- **Search** matches VPN name, server name and offer/package name — the three
  labels a card actually displays, so a person never has to guess which field
matched.
- **Filters** are Status, Server and Expiry, combinable with the category and
  with each other, each showing how many rows it would return, with a Clear
  action that appears only when something is filtered.
- **Sorting** is Newest First, Name A-Z and Expiry Date. A VPN with no expiry
  sorts LAST under "Expiry Date", because a reader asking "what runs out next"
does not want a never-expiring row at the top.
- **Changing a filter or the search resets to page 1**; the page number lives in
  the URL, so Back undoes a filter rather than leaving the screen.
- **Primary action is View Details**, using the existing `/vpn/:id` route. No new
  action is added and no connection is described as working unless its status
  says so.
- **Unclaimed Free offers never appear.** `GET /api/vpns` for a client returns
  owned rows only, and this screen applies **no** second, weaker ownership filter
  in the UI.

**One implementation consequence, recorded because it changes a file outside the
page.**

The usage figures a card shows (`used_bytes`, `limit_bytes`, `usage_pct`) are
**already present on every `GET /api/vpns` row** (`api.ts:213-215`). The S3 page
ignored them and issued one `GET /api/vpns/{id}/usage` call **per card**, which
is an N+1 pattern. Section 02 therefore maps those three fields onto `ClientVpn`
and renders usage from the list response. **No endpoint, query or backend file
changes; the requests are removed, not added.** Where a row reports no limit,
the card says "unlimited" rather than `0`; where usage is absent it says nothing
rather than `0 B`.

**Limitations recorded, not worked around.**

- `GET /api/vpns` does **not** send `is_custom`, `traffic_gb`, `duration_days` or
  `device_limit`. Custom specifications therefore cannot be rendered on this
  screen from the list endpoint. They remain visible where they already are (the
  offer catalogue and the VPN detail screen). Adding them to the list row would
  be a backend change and is **out of scope** for this decision.
- The list is not paginated by the server, so the whole owned inventory is
  fetched once and paged in the browser. That is unchanged from S3 and is
  correct for the stated target of up to ~100 VPNs per client.

**Out of scope.** Any backend, migration, schema, provisioning, payment or panel
change; any new endpoint; changing the nine-record page size; Section 03-09
implementation; Super Admin or Admin surfaces; Advanced QA infrastructure.

**Supersedes** nothing. **Relates to** D-032 / D-037.4 (role-scoped listing),
D-035 / D-035.2 (CUSTOM templates are a pricing concept, not a VPN category),
D-063 (an owned, unclassified row reads as PREMIUM), D-084 (role-scoped
navigation), D-090 (a client screen states only what the server sent).

---

### D-088.1 · Correction - the list row does NOT carry usage

**Recorded** 2026-11-08 during Section 02 implementation, superseding the
"One implementation consequence" paragraph of D-088. The earlier text stays in
place (this file is append-only); this entry is what governs.

**What D-088 asserted, and why it was wrong.** D-088 stated that
`used_bytes` / `limit_bytes` / `usage_pct` are "already present on every
`GET /api/vpns` row (`api.ts:213-215`)". They are **not**. Lines 213-215 belong
to **`ApiAdminVPNRow`**, the Super Admin console row returned by
`GET /api/super/vpns`. The client-facing `ApiVpn` interface (`api.ts:491-522`)
and its Go counterpart in `httpapi/vpn_routes.go` carry **no** usage fields at
all - verified by reading both.

**Corrected position.**

1. **A client's VPN list row carries no usage figure.** Therefore no card or
   list cell on `/client/vpns` may show data usage, and none does. Rendering a
   bar from a value the endpoint does not send is exactly the fabrication the
   product rules forbid.
2. **The N+1 pattern is removed, not replaced.** S3 issued one
   `GET /api/vpns/{id}/usage` per visible card. Section 02 issues **zero**
   per-card usage requests: the list reads the inventory once and shows what
   the row actually contains. Usage remains available where it already was -
   the VPN's own detail screen, which asks the server for that one VPN.
3. **`ClientVpn` gains no usage fields.** The three optional fields proposed in
   D-088 are **not** added, because there is nothing on the wire to populate
   them and an optional field that is always absent invites a future card to
   render `0`.
4. **Adding usage to the client list row is a backend change and is out of
   scope** for Section 02. It would need an owner decision of its own, with the
   cost of a per-row usage read on a 1 vCPU host measured first.

**What is unchanged from D-088.** Three categories (`All` / `Free` / `Premium`,
never four), `is_custom` never reclassified, page size nine, two views over the
same fetched rows, overview counts over the whole inventory, search over name /
server / package, combinable Status / Server / Expiry filters, three sorts,
`/vpn/:id` as the only action.

**Supersedes** the "One implementation consequence" paragraph of D-088.
**Relates to** D-088, D-038 P-06 (usage lives on the VPN detail screen).
