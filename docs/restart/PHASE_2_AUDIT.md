# VEYLIX — PHASE 2 AUDIT REPORT

**Created:** 2026-11-08 · **Method:** read-only inspection of the repository and the
live development database. No file was modified and no data was written while
producing this report.

**Status of every claim below:**

- **CONFIRMED** — verified by reading the repository or running a read-only query.
- **ASSUMED** — stated by a document but not re-verified in code for this report.
- **UNKNOWN** — not established; listed in `UNKNOWNS.md`.

---

## 1. What the product is (CONFIRMED)

A **self-hosted VPN reseller platform**. One Go binary serves both the HTTP API
and a React interface that is compiled and embedded into it. **3X-UI is the source
of truth for every VPN**; Veylix mirrors the panel and never invents a VPN.

Three roles (CONFIRMED, `MASTER_PRODUCT_BIBLE.md` §4):

| Role | Who | What they do |
|---|---|---|
| **Super Admin** | platform owner | Owns panels, plans, pricing, payments, staff accounts |
| **Admin** | reseller | Holds a quota; assigns and sells VPNs to their clients |
| **Client** | end customer | Claims a public FREE VPN, or buys PREMIUM from a seller |

---

## 2. Stack (CONFIRMED — read from the manifests)

| Layer | Technology | Evidence |
|---|---|---|
| Backend | Go **1.27** | `backend/go.mod` |
| Database | SQLite via `modernc.org/sqlite` **v1.60.1** (pure Go, no CGO) | `backend/go.mod` |
| Crypto | `golang.org/x/crypto` **v0.58.0** (argon2id) | `backend/go.mod` |
| Frontend | React **19.3**, TypeScript, Vite, Zustand 5, React Router 7, Tailwind 4 | `frontend/package.json` |
| Lint | **oxlint** (not ESLint) | `frontend/package.json` |
| Charts / 3D | `three` 0.186, `lucide-react` | `frontend/package.json` |

**Target host: 1 vCPU / 500 MB.** No Redis, no PostgreSQL, no Docker, no
persistent Node process.

---

## 3. Code size (CONFIRMED — measured)

| Metric | Count |
|---|---|
| Go source files | **136** · 29,716 lines |
| Go test files | **116** · 21,629 lines · **617 test functions in 15 packages** |
| TS/TSX files | **148** · 31,527 lines |
| Backend packages | **15** |
| CLI tools (`backend/cmd`) | **13** |
| Migrations | **23** |
| API routes | **133** (`npm run docs:verify`) |
| Frontend pages | **37** |
| Docs files | **31** under `docs/` |

---

## 4. Backend packages (CONFIRMED)

Dependency direction: `cmd/veylixd` → `httpapi` → `domain` → `{store, panel}`.

| Package | Responsibility |
|---|---|
| `httpapi` | routing, decoding, JSON envelopes, status codes |
| `domain` | state machines, quota, provisioning rules |
| `store` | **all** SQL, migrations, transactions |
| `panel` | **all** outbound 3X-UI HTTP calls, SSRF guard, sealing |
| `auth` | argon2id, sessions, RBAC, portal split |
| `audit` | append-only trail and the inbox notifier |
| `integrations` | server registration, health, inbounds, panel clients |
| `provisioning` | VPN create/lifecycle across DB and panel |
| `sync` | read-only panel polling, usage snapshots |
| `releases` | D-027 release lifecycle |
| `payments` | order settlement |
| `settings` | operator-editable settings, sealed secrets |
| `versioning` | update feed |
| `mail` | SMTP + outbox |
| `config` | environment (read by `cmd` only) |

---

## 5. Database (CONFIRMED — read from the 23 migration files)

23 migrations, `0001` → `0023`, applied in order. Core tables:

| Table | Purpose |
|---|---|
| `users` | accounts: `role`, `status`, `quota_limit`, profile columns |
| `auth_identities` | password vs Google provider mapping (D-039) |
| `sessions` | server-side sessions |
| `server_integrations` | 3X-UI panels; **sealed** token in `token_ciphertext` |
| `inbounds` | cached panel inbounds; **`id` = local row, `remote_id` = panel's number** |
| `vpn_instances` | mirror of a confirmed panel client |
| `vpn_assignments` | VPN → client, with seller and expiry |
| `vpn_offers` | the catalogue: FREE and PREMIUM offers |
| `vpn_releases` | release tombstones (D-027, rebuilt by `0023`) |
| `vpn_transfers` | VPN moved between clients |
| `panel_client_tags` | FREE/PREMIUM visibility tag, lockable |
| `free_claims` | per-client FREE claim ledger (concurrent + monthly caps) |
| `quota_ledger` | reseller quota movements |
| `payment_records` / `payment_methods` | orders and channels |
| `pricing_rates` | unit rates, CUSTOM packages |
| `order_events` | order timeline |
| `notifications` | per-user inbox |
| `usage_snapshots` / `server_metrics` | history |
| `chat_conversations` / `chat_participants` / `chat_messages` | internal chat |
| `audit_logs` | append-only audit trail |
| `outbox` | remote-write reconciliation queue |
| `app_settings` | operator-editable settings (sealed where secret) |

**Live development database** (`.veylix-secrets/data/veylix.db`, 798,720 bytes):

| Table | Rows |
|---|---|
| `server_integrations` | 1 |
| `vpn_offers` | 5 |
| `vpn_instances` | 4 |
| `vpn_assignments` | 8 |
| `payment_records` | 1 |
| `payment_methods` | 1 |
| `pricing_rates` | 1 |
| `users` | 8 |

> **This is development seed data plus test rows. It is NOT production data.**
> Offer 17 (`Free 1GB Trial`) and VPN 42 were created during the 2026-11-08
> session to prove the provisioning fix; they are test data, not requirements.

---

## 6. API surface (CONFIRMED)

**133 routes**, all served by one mux. Grouped by audience:

| Group | Examples |
|---|---|
| Public / auth | `/api/health`, `/api/ready`, `POST /api/auth/{register,login,logout}`, `/api/auth/forgot`, `/api/auth/reset`, `/api/auth/google/*` |
| Self-service (`/api/me`) | profile, profile patch, e-mail change, sessions |
| VPNs (`/api/vpns`) | list, create, detail, usage, connection, suspend, resume, revoke, release, transfer, verify, ping, claims |
| Servers (`/api/servers`) | CRUD, test, inbounds, panel-clients, disable, purge, assign |
| Offers / plans | `/api/offers`, `/api/offers/premium`, `/api/plans`, archive |
| Payments / orders | `/api/payments`, `/api/orders`, confirm, reject |
| Admin | `/api/clients`, `/api/quotas`, `/api/admins`, `/api/audit` |
| Chat | conversations, messages |
| Notifications | list, unread, mark read, mark all read |
| Super | `/api/super/*` — settings, vpns, stats |

Every route re-checks **role + row-level ownership** server-side. A refused
action returns an explicit code (`forbidden`, `not_found`, `quota_exceeded`,
`free_concurrent_limit`, `panel_error`, `wrong_portal`, …), never a silent 200.

---

## 7. Security requirements already implemented (CONFIRMED by reading code)

| Requirement | Where |
|---|---|
| argon2id password hashing | `internal/auth` |
| Server-side sessions; no client-supplied identity | `sessions` table + middleware |
| Sealed panel tokens, AES-256-GCM | `server_integrations.token_ciphertext` |
| SSRF guard on operator-supplied panel URLs | `internal/panel` |
| Per-integration TLS exception only, never global | `integrations` allow-private flag |
| Rate limits: login, register, reset, claim, chat | `auth.RateLimiter` call sites |
| Two login doors (`portal: client \| staff`) | `auth.Portal`, 403 `wrong_portal` |
| Append-only audit on privileged actions | `internal/audit` |
| Quota counted **in transaction** | `provisioning.Create` + `quota_ledger` |
| Honest failure: never fabricate remote success | `pushCreate` → `REVOCATION_PENDING` / `panel_rejected` |

---

## 8. Business rules that must survive (CONFIRMED — from the Bible)

These are **requirements**, not implementation detail. The new system must honour
them, or an explicit decision must replace them.

1. **A VPN *is* a 3X-UI client.** The panel is the only authority on what a VPN is.
2. **The panel wins** on any disagreement; Veylix mirrors, never invents.
3. **FREE / PREMIUM is a visibility tag**, stored locally, never written to the
   panel. It is not an account tier.
4. **`vpn_instances` is a mirror of a confirmed panel client**, never an intent row.
5. **Release removes the VPN from Veylix only** — it never deletes a panel client.
6. **Quota** is a reseller's sellable slots, counted inside the transaction.
7. **A refused panel write never leaves a VPN row behind** (D-024.1a).
8. **A local transaction never spans a remote call** — outbox + fresh read.
9. **Two FREE caps**: concurrent, and per rolling 30 days.
10. **Two login doors**, decided by the server, not the UI.
11. **Delivery is never faked** — no relay means `outbox`, never "e-mailed you".
12. **A client may only provision for themselves.**

The Bible's own section list (§0–§20) is the fuller source: product summary,
business rules, integration lifecycle, roles matrix, data model concepts,
required UX/pages, status model, security, acceptance tests, design system, the
1 CPU / 500 MB architecture, release + install, operations, change-control,
theme acceptance tests, delivery checklist.

---

## 9. Unresolved defects to carry forward (CONFIRMED by reading the registers)

| ID | Sev | Defect | Status |
|---|---|---|---|
| **I-36** | P1 | An offer's `inbound_id` (a **local** row id) was sent to the panel verbatim, so every FREE claim answered 502. Fixed 2026-11-08 via `provisioning.panelInboundID` + 2 regression tests. | **FIXED** |
| **I-37** | P2 | 193 unreachable `Action not applied / result=DENIED` inbox rows left by the removed producer. Cleaned 2026-11-08; backup taken first. | **FIXED** |
| **I-34** | P3 | `vpn_transfers` FK is `ON DELETE CASCADE` — a transfer record dies with its VPN. | **OPEN** |
| **I-35** | P3 | `usage_snapshots` FK is `ON DELETE CASCADE` — history dies with the VPN. | **OPEN** |
| **U13** | P2 | 224 inline `style={{…}}` occurrences; repeated spacing/font values. | OPEN |
| **U16** | P2 | `EmptyState` missing on three Admin pages. | OPEN |
| **U17** | P2 | Mixed `Skeleton` and inline "Loading…". | OPEN |
| **U18** | P2 | Focus ring unverified on `SavedViewsBar`, `Pagination`, table sort headers. | OPEN |
| **S3-1** | P2 | `verify-mobile.mjs` measures only 390 px; the brief requires more widths. | OPEN |
| **S3-3** | P2 | The seeded client owned no VPN, so client-VPN gates refused to run. **Now unblocked** — VPN 42 exists. | OPEN |
| **S3-4** | P2 | `verify-mobile.mjs` never enables touch emulation. | OPEN |
| **S3-5** | P2 | Four control families on the S3 Client surface. | OPEN |

---

## 10. Documentation integrity findings

| Finding | Detail |
|---|---|
| **I-series loss** | `docs/IMPROVEMENT_LEDGER.md` lost entries `I-01`–`I-35` on 2026-11-08 to an agent editing error. `I-08`, `I-09`, `I-27`–`I-35` are reconstructable from `docs/cline/progress.md`; `I-10`–`I-26` are **unrecoverable**. See `KNOWN_LOSSES.md`. |
| **D-number gaps** | `DECISIONS.md` holds **86** decisions, `D-001`–`D-088`. Missing: **D-062, D-077, D-078, D-089**. `D-077` and `D-078` *are* referenced by ledger entries, so those numbers were used but their records are absent. |
| **No version control** | There is **no Git repository**. Nothing here is diff- or revert-able. **The single largest process risk**; fix before Phase 5. |
| **Doc volume** | `DECISIONS.md` 205 KB · `IMPROVEMENT_LEDGER.md` 148 KB · `IMPLEMENTATION_PLAN.md` 75 KB · `QA_REPORT.md` 66 KB. |

---

## 11. What is genuinely reusable

Marked so Phase 3 decides deliberately rather than by accident.

| Asset | Reuse? | Why |
|---|---|---|
| The **business rules** (§8) | **Yes — mandatory** | they are requirements |
| The **database schema** (23 migrations) | **Yes, as the starting point** | it encodes accumulated invariants; changing it needs its own decisions |
| The **API shape** (133 routes) | **Yes, as reference** | the UI and the 617 tests are written against it |
| The **security model** (§7) | **Yes — mandatory** | every row is a hard requirement |
| Backend **package boundaries** | **Yes** | the dependency rule already holds |
| The **frontend pages** (37) | **Review each** | several are staff-only and were not audited in this pass |
| The **CLI tools** (13) | **Review** | `dbexec`, `panelprobe`, `outboxprobe`, `pwreset` carry real operational value; others are one-off debugging aids |
| `IMPLEMENTATION_PLAN.md` | **Reference only** | it describes work already done, not a forward plan |
| Historical ledgers | **Reference only** | history, not requirements |

---

## 12. Constraints the new system inherits

1. **1 vCPU / 500 MB.** No new daemon, no extra datastore.
2. **One binary.** The frontend is built and embedded; Node never runs in production.
3. **SQLite only**, pure-Go driver (no CGO).
4. **The panel contract** in `docs/3XUI_API_CONTRACT.md` is what Veylix codes to.
5. **`go test -race` is BLOCKED** on this machine (no C compiler). Report `BLOCKED`,
   never PASS or FAIL.

---

## 13. Phase 2 conclusion

The codebase is **substantial and working**: 617 tests pass, the security model is
implemented rather than aspirational, and the panel integration is proven live
(`probe=FOUND` for a client this session created).

What it **lacks** is process durability: no version control, a documentation set
whose history was damaged, and defect tracking spread across five registers with
overlapping ID schemes.

**Recommendation.** Phase 3 should *document what exists accurately*. Phase 4
should establish the process controls that were missing (Git, one defect register,
one ID scheme). Phase 5 should continue feature work on top of the existing
backend rather than starting from zero — a rewrite would discard 617 passing tests
and a proven panel integration in exchange for no new capability.



