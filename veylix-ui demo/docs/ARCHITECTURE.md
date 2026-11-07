# VEYLIX — ARCHITECTURE

**Version:** 1.2 · **Created:** 2026-10-23 · **Last synced:** 2026-11-06 (§0 single-owner model, §4a isolation boundaries)
**Status:** Stages B, E2, E3, E4 implemented · E5–E7 planned
**Sources:** `new_update.md` §4 (required architecture rules), §8 (resource budget), `docs/3XUI_API_CONTRACT.md`, D-010…D-013, **D-079…D-083 (independent installation)**

---

## 0. One owner per installation — the model everything else sits inside

Veylix is a **standalone, self-hosted, single-tenant** application (Bible §1.0/§1.1, D-079). One
deployment = **one owner** = **one process group** = **one SQLite file** = **one key pair** = **one
domain** = **one version that its owner upgrades when they choose**.

```
        Server A (Owner A)                 Server B (Owner B)
  ┌──────────────────────────────┐   ┌──────────────────────────────┐
  │ Caddy ── a.example.com       │   │ Caddy ── b.example.org       │
  │ 127.0.0.1:8080 → veylixd     │   │ 127.0.0.1:8080 → veylixd     │
  │ /var/lib/veylix/veylix.db    │   │ /var/lib/veylix/veylix.db    │
  │ /etc/veylix/{enc,session}.key│   │ /etc/veylix/{enc,session}.key│
  │ 2 × 3X-UI panels (A's own)   │   │ 1 × 3X-UI panel (B's own)    │
  └──────────────────────────────┘   └──────────────────────────────┘
            ▲ no channel of any kind between the two boxes ▲
```

- **No central Veylix database, no optional central service, no server→central reporting.** The
  process dials out only to the panels its own owner configured.
- **The repository is a distribution mechanism, not a service.** The same commit and the same
  release artifact produce both columns above.
- **3X-UI is external and owner-supplied** (D-080): Veylix speaks to a remote, SSRF-guarded panel
  URL with a Bearer token. It never installs, bundles or manages a panel.
- **No `tenant_id`, and no multi-tenant refactor will be accepted.** Row-level isolation is already
  provided by the process/disk boundary, which is stronger than a column and costs nothing.

This section constrains every later section: if a design idea needs a shared component to work, it
contradicts the product definition.

---

## 1. Layers and dependency direction

```
        ┌──────────────────────────────────────────────┐
        │ cmd/veylixd            process wiring        │
        └───────────────┬──────────────────────────────┘
                        │
        ┌───────────────▼──────────────┐
        │ internal/httpapi             │  transport only: routing, decode,
        │                              │  JSON envelopes, status codes
        └───────────────┬──────────────┘
                        │ calls
        ┌───────────────▼──────────────┐        ┌────────────────────┐
        │ internal/domain              │───────▶│ internal/panel     │
        │ business rules + invariants   │        │ 3X-UI HTTP adapter │
        └───────────────┬──────────────┘        └────────────────────┘
                        │ persistence interface            ▲
        ┌───────────────▼──────────────┐                 │
        │ internal/store               │  SQLite, migrations, tx
        └──────────────────────────────┘

        internal/config        ← read by cmd only
        internal/auth          ✅ argon2id, sessions, RBAC, ownership, rate limit
        internal/audit         ✅ append-only privileged-action log
        internal/integrations  ✅ panel add/verify/inbounds/disable/remove (sealed tokens)
        internal/provisioning  ✅ quota + outbox + reconciler (Stage E4)
        internal/sync          ✅ panel sync + usage snapshots + metrics (E5, read-only)
```

**Rules enforced by this layout (`new_update.md` §4):**

| Rule | How it is honoured |
|---|---|
| Handlers hold no business rules | `httpapi` only decodes/encodes; it takes a `Health` interface, never the DB driver |
| Domain holds the rules | `domain` will own state machines, quota, provisioning; it never imports `httpapi` |
| Persistence is isolated | only `store` speaks SQL; domain receives interfaces |
| Remote integration is isolated | only `panel` performs outbound 3X-UI calls |
| No cycles | one direction: `cmd → httpapi → domain → {store, panel}` |
| No global mutable state | dependencies injected from `cmd/veylixd` |

---

## 2. Startup and shutdown (`cmd/veylixd/main.go`)

```
Load config (fail closed on missing/weak secrets)
  → ensure data dir (0700)
  → open SQLite (WAL, FK, busy_timeout=5000, maxOpenConns=4)
  → apply embedded migrations (idempotent, transactional per file)
  → log schema_version (no secrets)
  → serve HTTP with Read/Write/Idle timeouts
  → on SIGINT/SIGTERM: graceful Shutdown (10 s) → close DB
```

Failure at any step exits non-zero with a logged reason — the process never starts half-configured.

---

## 3. HTTP surface (`internal/httpapi`)

| Route | Purpose | Depends on |
|---|---|---|
| `GET /api/health` | liveness — answers even when the DB is down | nothing |
| `GET /api/ready` | readiness — DB ping + `schema_version` | `store` |
| `POST /api/setup/bootstrap` | one-time super-admin creation (409 after any user exists) | `auth`, `store` |
| `POST /api/auth/register` | self-registration (always `client`) | `auth`, `sessions` |
| `POST /api/auth/login` | credential login + session cookie (rate-limited) | `auth`, `sessions` |
| `POST /api/auth/logout` | session row deleted | `sessions` |
| `POST /api/auth/change-password` | rotate credential; revokes every other session | `auth`, `sessions` |
| `GET /api/me` | resolved principal (id, email, role) | `authn` |
| `GET/POST /api/servers`, `POST /api/servers/{id}/test` | panel integrations (role-scoped) | `integrations`, `panel` |
| `GET /api/servers/{id}/inbounds`, `POST .../disable`, `DELETE .../{id}` | inbound list / lifecycle | `integrations`, `panel` |
| `GET/POST /api/vpns`, `POST /api/vpns/{id}/{suspend,resume,revoke}` | provisioning + lifecycle | `provisioning`, `store` |
| `GET /api/audit` | privileged-action log (super admin only) | `audit` |
| `GET /api/quotas/me` | caller's own quota | `store` |
| `GET /api/vpns/{id}/usage` | one VPN's history + lifetime totals (owner/seller scope) | `store` |
| `GET /api/servers/{id}/metrics` | server-wide CPU/RAM samples (`scope:server_wide`) | `store` |
| `GET /api/sync/status` | per-server freshness (`last_sync`, `stale`) | `store` |
| `/api/*` (other) | standard `404` envelope | — |

**Error envelope** (every failure, no exceptions):

```json
{ "error": { "code": "db_unavailable", "message": "database is not reachable" } }
```

Messages are client-safe: no SQL, no panel payloads, no secrets (`new_update.md` §7). Panics are recovered into `500 internal_error` without stack details.

---

## 4. Data layer (`internal/store`)

| Concern | Decision |
|---|---|
| Driver | `modernc.org/sqlite` — CGO-free (D-013) |
| Pragmas | `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`, `synchronous=NORMAL` |
| Pool | `maxOpenConns=4` — SQLite serialises writes; 1 vCPU target |
| Migrations | embedded `migrations/*.sql`, applied in filename order, one transaction each, recorded in `schema_migrations` |
| Transactions | `Store.Tx(ctx, fn)` with panic-safe rollback — used for quota + provisioning |
| Integrity | `PRAGMA quick_check` available for readiness/ops |

**Schema groups (4 migrations):**

1. `0001_identity.sql` — `users`, `sessions`, `server_integrations`, `inbounds`, `admin_server_assignments`
2. `0002_business.sql` — `vpn_offers`, `vpn_instances`, `vpn_assignments`, `quota_ledger`, `outbox`, `payment_methods`, `payment_records`
3. `0003_observability.sql` — `usage_snapshots`, `server_metrics`, `notifications`, chat tables, `audit_logs`
4. `0004_quota_limit.sql` — per-admin quota limit column added for Stage E4

Key invariants baked into the schema:

- **A VPN *is* a 3X-UI client (D-024).** `vpn_instances` is a **mirror** of confirmed panel clients, never an intent row: the row is written only after `GET /clients/get/{email}` confirms the client. Its `panel_*` columns hold the panel's own `totalGB`/`expiryTime`/`limitIp`/`limitHwid`/`enable`, refreshed by sync — Veylix never derives a VPN's limits from an offer.
- **Free/Premium is a visibility tag** (`panel_client_tags`), the only operator-supplied VPN attribute, stored locally and never written to the panel. **Every** panel client is a VPN; an untagged one is visible only to the Super Admin (D-024.3). The tag **locks** once the VPN is issued (D-025.2).
- **FREE caps live in `free_claims`** (3 concurrent, 5 per rolling 30 days per client per server — D-025.4/.5), never inferred by scanning `vpn_instances`.
- **Release is local, never remote** (`vpn_releases`): a quota-exhausted VPN released by its client is removed from Veylix after a **3-day grace with undo**; the panel client is untouched (D-027). "Storage" in this context means **quota**, not disk.
- **Chat storage is budgeted per client** (`client_storage_limits` + `chat_usage`), separate from VPN quota, and reclaimable by the client deleting their own messages (D-027.6).
- `vpn_instances UNIQUE (server_id, panel_email)` → a panel client is one VPN, and a retried provision cannot create a second panel client (D-012)
- `outbox UNIQUE (server_id, op, idempotency_key)` → an ambiguous write is recorded once (F-009)
- `server_integrations.status` CHECK constraint = the six-state model (REQUIREMENTS §7)
- `token_ciphertext` is a BLOB — plaintext tokens have no column to live in (D-011)
- lifetime usage columns live on `vpn_instances` so a remote counter reset cannot erase history (F-010)

**Atomicity statement (`new_update.md` §6):** a local transaction **never** covers a remote panel. Remote effects are reconciled through `outbox` + fresh reads.

---

## 4a. Installation isolation boundaries (what is local, and what is not yet provable)

"Two installations share nothing" is a property of *where the state lives*, so it is worth naming
each boundary and the file that holds it.

| Boundary | Where it lives | Why an installation cannot leak across it |
|---|---|---|
| Database | one file from `VEYLIX_DATA_DIR` — `backend/internal/config/config.go:27-31`, opened by `backend/internal/store/store.go:39-64` | there is no remote database, no replica and no shared mount; SQLite is a local file |
| Migrations | embedded in the binary, applied per process at startup (`store.Migrate`) | each install migrates its own file; schema moves forward only (`docs/UPGRADE.md` §4) |
| Accounts / sessions | `users`, `sessions` (`backend/internal/store/migrations/0001_identity.sql`) | created by *this* install; first Super Admin is one-shot (`backend/internal/auth/service.go:438-445`) |
| Panel connections | `server_integrations` (`0001_identity.sql:36-54`), used only via `integrationsSvc.PanelClient(ctx, serverID)` (`backend/cmd/veylixd/main.go`) | per-request resolution from the local row; only `ACTIVE` rows are polled (`main.go` `activeServerIDs`) |
| Panel secrets | `token_ciphertext BLOB` sealed by `backend/internal/auth/seal.go` | AES-256-GCM with per-seal nonce and AAD bound to the integration; another install's key cannot open it |
| Config / domain | `/etc/veylix/veylix.env` + `deploy/Caddyfile` per host | one env file and one Caddy site per installation |
| Version | `/usr/local/bin/veylixd` per host | a binary belongs to one installation; nothing pushes upgrades (D-081) |

**What this architecture does *not* isolate:** the **3X-UI panel itself**. A panel is a remote
resource owned by an operator; if two installations are pointed at the same panel, both hold a
full-admin token for it (D-080). Veylix never discovers another installation's panel, so that can
only happen deliberately.

**Installation identity — future requirement, not implemented.** There is **no** `installation_id`
in any of the 22 migrations, so today an installation cannot name itself and a backup cannot prove
its provenance. The intended design (Bible §14.7/§14.8, `DISCREPANCY_REGISTER.md` D17) is a single
row created on first startup, preserved across upgrades, carried in the backup manifest and checked
before a restore replaces anything. It is **not** a tenant key: it partitions nothing and is never
sent anywhere.

**No shared state to reconsider.** There is no cache server, queue daemon, session store or
scheduler shared between installations — the periodic work (sync, release purge, session purge) runs
in-process per installation (`backend/cmd/veylixd/main.go`), which is also what keeps the 1 vCPU /
500 MB target reachable.

---

## 5. Resource budget (1 vCPU / 500 MB)

| Budget | Value | Rationale |
|---|---|---|
| DB connections | 4 | SQLite write serialisation |
| HTTP server timeouts | read 15 s · write 30 s · idle 60 s | no hung sockets |
| Panel per-request timeout | (Stage E3) 10 s, bounded retries on GET only | one slow panel must not stall others |
| Panel health polling | ≥ 30 s per panel | the panel caches `/server/status` for 2 s |
| Client list fetch | `/clients/list/paged`, ≤ 200 rows | avoids the `/inbounds/list` payload explosion |
| Frontend | built in CI, embedded in the binary; **Node.js never runs on the server** | Bible §0.11 |

---

## 6. What exists today vs what is planned

| Component | State (2026-10-23, post E4) |
|---|---|
| config, SQLite open + pragmas + migrations, HTTP health/ready, graceful shutdown | **implemented + unit tested** |
| schema (all tables) | **implemented + tested** (idempotent migrate, rollback, uniqueness) |
| auth / sessions / RBAC / ownership | **implemented + tested** (argon2id, rate limit, CSRF, AES-GCM sealing) |
| password policy + change | **implemented + tested** (length/entropy/blocklist; `POST /api/auth/change-password`) |
| panel adapter / SSRF guard / capability detect | **implemented + live-verified** against 3X-UI 3.10.2 |
| provisioning / quota / outbox execution | **implemented + unit tested** (live writes blocked, D-018) |
| sync jobs / usage accounting | **not implemented** — Stage E5 |
| links/QR endpoint, SPA embedding | **not implemented** — Stage E6/E7 |
| `go test -race` | **blocked** — no C compiler on the dev machine |
| frontend: auth + servers pages wired to the API | **implemented** (`lib/api.ts`); other stores still mock (D-006) |

No component is claimed to work beyond the evidence recorded in `docs/QA_REPORT.md`.
