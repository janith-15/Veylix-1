# VEYLIX — DATABASE

**Status:** authoritative · **Established:** 2026-11-08 · **Baseline:** `1b09553`

**Evidence.** Everything here was read from the 23 migration files in
`backend/internal/store/migrations/` and from a **read-only** query of the
development database. No statement is inferred from prose.

---

## 1. Engine facts (C)

| Property | Value | Where |
|---|---|---|
| Engine | SQLite via `modernc.org/sqlite` v1.60.1 — **pure Go, no CGO** | `backend/go.mod` |
| Journal | `WAL` | `store.pragmaQuery()` |
| Foreign keys | `ON` — per connection, applied via the DSN | `store.pragmaQuery()` |
| Busy timeout | **5000 ms** | `store.pragmaQuery()` |
| Synchronous | `NORMAL` | `store.pragmaQuery()` |
| Max open conns | **4** (writes are serialised by SQLite) | `store.maxOpenConns` |
| Migrations | **23**, embedded at compile time, applied in filename order | `internal/store/migrations/` |
| Migration setter | `st.MigrateFast(ctx)` in tests, `st.Migrate(ctx)` in production | `store` |

**Consequence.** Because the driver is pure Go, `go test -race` cannot run on this
machine (no C compiler). That gate is **BLOCKED**, never PASS or FAIL.

---

## 2. Migration history (C)

| # | File | What it added |
|---|---|---|
| 0001 | `identity.sql` | `users`, `sessions`, `server_integrations`, `inbounds`, `admin_server_assignments` |
| 0002 | `business.sql` | `vpn_offers`, `vpn_instances`, `vpn_assignments`, `quota_ledger` |
| 0003 | `observability.sql` | `usage_snapshots`, `server_metrics`, `notifications`, `audit_logs` |
| 0004 | `quota_limit.sql` | reseller quota ceiling on the user |
| 0005 | `sync.sql` | idempotent write keys |
| 0006 | `panel_truth.sql` | the panel becomes the source of truth |
| 0007 | `chat.sql` | `chat_conversations`, `chat_participants`, `chat_messages` |
| 0008 | `plans_money.sql` | plan ownership, price, slots |
| 0009 | `panel_assignments.sql` | panel-side assignment records |
| 0010 | `pricing_packages.sql` | unit rates, CUSTOM packages |
| 0011 | `order_events.sql` | payment channels + order timeline |
| 0012 | `google_identity.sql` | `auth_identities` (D-039) |
| 0013 | `login_profile.sql` | username + profile columns + `free_vpn_limit` + `last_login_*` (D-041) |
| 0014 | `profile_email_change.sql` | e-mail change codes |
| 0015 | `password_reset_codes.sql` | reset by e-mailed code |
| 0016 | `birthday.sql` | birthday replaces free-text `age` |
| 0017 | `client_deleted.sql` | allow a client account to be DELETED (terminal state) |
| 0018 | `vpn_released.sql` | a released VPN gets its own state (D-027) |
| 0019 | `assign_keeps_panel_name.sql` | an assignment keeps the panel's own name |
| 0020 | `vpn_transfer.sql` | VPN moves between client accounts |
| 0021 | `dedupe_carrier_offer.sql` | collapse duplicate "Platform assignment" carriers |
| 0022 | `app_settings.sql` | operator-editable settings, sealed where secret |
| 0023 | `release_tombstone.sql` | a release record outlives the VPN it describes (D-087 / I-33) |

---

## 3. Tables (C)

Grouped by purpose. `→` reads "references".

### Identity
| Table | Holds |
|---|---|
| `users` | `role` (client/admin/super_admin), `status`, `quota_limit`, profile columns, `free_vpn_limit`, `last_login_at`, `last_login_ip` |
| `auth_identities` | provider (`password` / `google`) → subject |
| `sessions` | server-side sessions: token hash, ip, user agent, expiry |

### Panel
| Table | Holds |
|---|---|
| `server_integrations` | panel URL, **sealed** `token_ciphertext`, status, versions, last health check |
| `inbounds` | cached panel inbounds. **`id` = local AUTOINCREMENT row; `remote_id` = the panel's own inbound number.** `UNIQUE(server_id, remote_id)` |
| `server_metrics` | per-server CPU/RAM/network samples |
| `panel_client_tags` | FREE/PREMIUM visibility tag, `locked_at` |
| `panel_assignments` | panel-side assignment records |

### VPN
| Table | Holds |
|---|---|
| `vpn_instances` | mirror of a **confirmed** panel client |
| `vpn_assignments` | VPN → client, seller, status, expiry |
| `vpn_releases` | release tombstones; rebuilt by 0023 so the row survives a purge |
| `vpn_transfers` | VPN moved between clients |
| `free_claims` | per-client FREE claim ledger, `UNIQUE` per rolling window |
| `usage_snapshots` | per-VPN usage history |

### Commerce
| Table | Holds |
|---|---|
| `vpn_offers` | the catalogue — FREE and PREMIUM, `price_cents`, `currency`, slots, `owner_user_id` |
| `payment_records` / `payment_methods` | orders and channels |
| `pricing_rates` | unit rates, CUSTOM packages |
| `order_events` | order timeline |
| `quota_ledger` | reseller quota movements |

### Platform
| Table | Holds |
|---|---|
| `notifications` | per-user inbox: `user_id`, `kind` (**severity**, not category), title, body, `read_at` |
| `audit_logs` | append-only trail: actor, action, target, result, context JSON |
| `outbox` | remote-write queue: op, key, payload, state, attempts |
| `app_settings` | operator-editable settings, sealed where secret |
| `chat_conversations` / `chat_participants` / `chat_messages` | internal chat |

---

## 4. The single most dangerous schema fact (C)

**`inbounds` carries two different inbound numbers.**

```sql
CREATE TABLE inbounds (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,   -- Veylix's own row id
    server_id  INTEGER NOT NULL REFERENCES server_integrations (id),
    remote_id  INTEGER NOT NULL,                    -- the 3X-UI panel's number
    ...
    UNIQUE (server_id, remote_id)
);
```

On the development database these diverged: rows `52, 53, 54` carried
`remote_id` `1, 2, 3`.

- `vpn_offers.inbound_id` is a **foreign key to `inbounds(id)`** — so it stores
  the **local** number (52).
- The panel only understands its **own** number (1).

**This produced defect `I-36`**: a FREE claim sent `inboundIds: [52]`, the panel
answered "inbound not found", and every claim failed with 502. The fix is
`provisioning.panelInboundID`, which resolves `inbounds.remote_id` before anything
reaches the panel.

**Rule.** Any code that hands an inbound number to the panel **must** translate it
through `remote_id`. This is covered by `TestCreateSendsThePanelsInboundNumber`,
which deliberately sets local `id=1` / `remote_id=7` so forwarding the wrong one
is visible rather than accidentally correct.

---

## 5. Foreign-key actions (C, counted across all 23 migrations)

| Action | Count | Meaning |
|---|---|---|
| `ON DELETE CASCADE` | **38** | the child dies with the parent |
| `ON DELETE SET NULL` | **34** | the child survives, orphaned by design |
| `ON DELETE RESTRICT` | **14** | the parent cannot be deleted while children exist |

**Two cascade sites are recorded, known defects** — a history row dies with the
VPN it describes:

| Defect | Table | Why it matters |
|---|---|---|
| **I-34** | `vpn_transfers` | the record of "this VPN moved to that client" disappears |
| **I-35** | `usage_snapshots` | the usage history disappears |

`I-33` was the same class on `vpn_releases` and was **fixed** by migration `0023`,
which made `vpn_instance_id` nullable + `SET NULL` and added `panel_email` /
`server_id` so the tombstone still identifies the client. `I-34` and `I-35` are
still **OPEN**.

---

## 6. Indexes (C — 23 named indexes)

`idx_assign_client`, `idx_assign_seller`, `idx_audit_action`, `idx_audit_actor`,
`idx_chat_msg`, `idx_metrics_server`, `idx_notif_user`, `idx_offer_owner`,
`idx_offer_public`, `idx_outbox_state`, `idx_pay_offer`, `idx_pay_status`,
`idx_plan_purchase_buyer`, `idx_quota_admin`, `idx_servers_status`,
`idx_sessions_user`, `idx_usage_instance`, `idx_users_google`, `idx_users_role`,
`idx_vpn_owner`, `idx_vpn_releases_pending`, `idx_vpn_releases_vpn`,
`idx_vpn_server`.

Every role-scoped list query has a supporting index on the column its `WHERE`
filters by, which is why a 4-row development database and a real one take the same
plan shape.

---

## 7. Invariants the database itself enforces (C)

| Invariant | Mechanism |
|---|---|
| A tag is one of FREE / PREMIUM | `CHECK` on the tag column |
| An offer is one of FREE / PREMIUM | `CHECK (kind IN ('FREE','PREMIUM'))` |
| `active` is 0 or 1 | `CHECK (active IN (0,1))` |
| One inbound per `(server, remote_id)` | `UNIQUE (server_id, remote_id)` |
| No plaintext panel token has a column | only `token_ciphertext` exists |
| A FREE claim cannot duplicate | `UNIQUE` on the claim ledger |
| Quota cannot go negative | the ledger is append-only rows, summed |

---

## 8. What the database does NOT enforce (and code must)

| Not enforced in schema | Enforced by |
|---|---|
| "A VPN row only exists if the panel confirmed it" | `provisioning.pushCreate` + discard on rejection |
| "Release never deletes a panel client" | `releases` service never calls panel delete |
| "Quota counted in-transaction" | `provisioning.Create` wraps count + insert in one tx |
| "A refused write leaves no row" | rollback in the same tx |
| "`remote_id` must be > 0" | `provisioning.panelInboundID` refuses 0 |
| "An audit row is never updated or deleted" | no code path does it; the trail is append-only by convention |

The last row is **convention, not constraint** — worth noting honestly. A
`BEFORE UPDATE`/`BEFORE DELETE` trigger would make it structural, and is not
present today.

---

## 9. Development database snapshot (C, read-only, 2026-11-08)

`.veylix-secrets/data/veylix.db` — **798,720 bytes**, all 23 migrations applied.

| Table | Rows |
|---|---|
| `users` | 8 (1 super_admin, 1 admin, 5 client, 1 DELETED) |
| `server_integrations` | 1 (`indian-1`) |
| `vpn_offers` | 5 |
| `vpn_instances` | 4 |
| `vpn_assignments` | 8 |
| `payment_records` / `payment_methods` / `pricing_rates` | 1 / 1 / 1 |
| `notifications` | 46 (after the 193-row cleanup on 2026-11-08) |

**This is development seed data plus test rows — not production data.** Offer 17
and VPN 42 exist because they were created to prove the `I-36` fix. See
`MIGRATION_PLAN.md` for what happens to this data.

