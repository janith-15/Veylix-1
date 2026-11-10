# VEYLIX — FE ↔ BE API CONTRACT

**Version:** 1.1 · **Created:** 2026-10-23 · **Re-synced:** 2026-10-24 (evening) · **Status:** live contract — every ✅ row below was executed against the running binary
**Purpose:** define the HTTP surface that replaces the mock stores (D-006 exit, `docs/UI_INVENTORY.md` §10) — **that exit is complete: 0 mock stores remain.**
**Rule:** an endpoint is listed here **before** it is built; it appears in `docs/QA_PLAN.md` before it is called done. Rows marked ⬜ are **not implemented yet** — the UI must say so rather than pretend (enforced by `npm run audit`, see `docs/LIVE_PREVIEW.md` §4).


---

## 0. Conventions

| Aspect | Decision |
|---|---|
| Base path | `/api` |
| Transport | HTTPS only in production (Caddy in front); backend binds `127.0.0.1` |
| **UI serving** | The built SPA is **embedded in the same binary** (D-029). Any non-`/api` path serves the shell so client routes deep-link; a missing *file* stays 404; `/api/*` always answers JSON |
| Auth | `HttpOnly` + `Secure` + `SameSite=Strict` session cookie; server-side session row |
| CSRF | state-changing requests require `Origin` to match (own host or a configured public origin); a **browser-shaped** request with no `Origin` (`Sec-Fetch-*`/`Referer`) is refused. See D-043 |
| Content type | `application/json; charset=utf-8` |
| Errors | `{ "error": { "code": "...", "message": "..." } }` — messages are client-safe |
| Pagination | `?limit=` (max 200, default 50) + `?cursor=`; responses return `next_cursor` |
| Timestamps | unix seconds (integers) in JSON |
| IDs | local integers; remote 3X-UI ids are never exposed to non-super-admin roles |
| Secrets | never in any response body, ever (D-011) |

**Error codes:** `unauthorized` 401 · `forbidden` 403 · `not_found` 404 · `conflict` 409 · `validation_failed` 422 · `rate_limited` 429 · `panel_rejected` 502 · `panel_unavailable` 503 · `internal_error` 500

---

## 1. Auth & session *(implemented — Stage E2)*

| Method | Path | Body / query | Response | Notes |
|---|---|---|---|---|
| `POST` | `/api/auth/register` | `{email, password, display_name}` | `201 {user}` + cookie | argon2id; self-registration always `client`; email verification pending |
| `POST` | `/api/auth/login` | `{email, password}` | `200 {user}` + cookie | rate-limited (5 burst, 1/5 min per IP) |
| `POST` | `/api/auth/logout` | — | `204` | session row deleted |
| `POST` | `/api/auth/logout-others` | — | `200 {ended}` | **D3** — ends every session **except the caller's**; `ended` is the number removed. The token comes from the cookie, never the body |
| `POST` | `/api/auth/change-password` | `{current_password, new_password}` | `204` + fresh cookie | **added F-016**; revokes **every** session, re-issues the caller's; `422 password_mismatch` / `422 weak_password` |
| `POST` | `/api/setup/bootstrap` | `{email, password, display_name}` | `201 {user}` + cookie | one-time; signs you in; `409 bootstrap_complete` after any user exists |
| `GET` | `/api/setup/status` | — | `{bootstrap_required: bool}` | public; drives the `/setup` redirect |
| `GET` | `/api/admins` | `?role=admin\|client\|super_admin` | `[user]` | super-admin only |
| `POST` | `/api/admins` | `{email, password, display_name, role, quota_limit}` | `201 {user}` | super-admin only; the only way to mint an `admin` |
| `PATCH` | `/api/admins/{id}` | `{status?, quota_limit?}` | `200 {user}` | suspend revokes every session; self-suspend → 422 |
| `GET` | `/api/me` | — | `200 {id, email, role, display_name, created_at, email_verified}` | **D1/D2** — `display_name` and `created_at` are read from the account row. When no reader is wired the two keys are **absent** (never guessed) and the route still answers `200`; a failed read adds `profile_read_error: true` |
| `POST` | `/api/auth/google` | `{id_token}` | `200 {user}` + cookie | 🟡 **designed only** — token must be verified server-side (no credentials yet) |
| `PATCH` | `/api/me` | `{display_name?, contacts?, theme_prefs?}` | `200 {user}` | 🟡 designed only |

**Password policy (shared by register, bootstrap and change):**
12–512 characters (runes) · no single repeated character · no straight ascending/descending sequence · not in the built-in common-password blocklist. Rejections return `422 weak_password` with a message naming the rule — never the password. Client mirror: `frontend/src/lib/passwordPolicy.ts`; server source of truth: `backend/internal/auth/password.go`.

---

## 2. Server integrations *(implemented — Stage E3, live-verified)*

| Method | Path | Body / query | Response | Panel call |
|---|---|---|---|---|
| `GET` | `/api/servers` | — | `[{id, name, panel_url, status, panel_version, xray_version, last_health_check, last_sync}]` | — |
| `POST` | `/api/servers` | `{name, panel_url, token, allow_private_network?}` | `201 {id, status:"PENDING_VERIFICATION"}` | `GET /panel/api/server/status` |
| `PATCH` | `/api/servers/:id` | `{name?, panel_url?, token?, allow_private_network?}` | `200 {server}` | re-verify on credential change |
| `POST` | `/api/servers/:id/test` | — | `200 {status, panel_version, xray_version, latency_ms}` | `GET /panel/api/server/status` |
| `POST` | `/api/servers/:id/disable` | — | `204` | — |
| `DELETE` | `/api/servers/:id` | — | `204` (status → `REMOVED`, history kept) | — |
| `GET` | `/api/servers/:id/inbounds` | — | `[{id, remark, tag, protocol, port, enabled}]` | `GET /panel/api/inbounds/list/slim` |

**Status mapping** (`docs/3XUI_API_CONTRACT.md` §7): `200` → `ACTIVE` · `401/403` → `REAUTH_REQUIRED` · timeout/DNS/TLS/5xx → `CONNECTION_UNAVAILABLE`. An outage **never** yields `REVOKED`.

---

## 3. Offers, admins, quota *(quota + account management implemented · offers CRUD planned — E4)*

| Method | Path | Body | Response | Rules |
|---|---|---|---|---|
| `GET` | `/api/offers` | — | `[{id, kind, name, limits, active}]` | Free offers visible to clients |
| `POST` | `/api/offers` | `{kind, name, server_id, inbound_id, traffic_gb, duration_days, device_limit}` | `201 {offer}` | super-admin only |
| `PATCH` | `/api/offers/:id` | partial | `200 {offer}` | deactivating removes it from the client catalog (REQUIREMENTS §2.4) |
| `GET` | `/api/admins` | — | `[{id, email, status, quota_used, quota_limit}]` | super-admin only |
| `POST` | `/api/admins` | `{email, quota_limit, server_ids[]}` | `201 {admin}` | super-admin only |
| `PATCH` | `/api/admins/:id` | `{status?, quota_limit?}` | `200 {admin}` | audited |
| `GET` | `/api/quotas/me` | — | `{used, limit, remaining}` — a client (no reseller quota) gets the constant `{0, 0, -1}` (D-021) | any authenticated role |
| `GET` | `/api/vpns/{id}/usage` | `?since&limit` | `{vpn_id, status, limit_bytes, lifetime_up_bytes, lifetime_down_bytes, remote_reset_count, history, first_snapshot_at, points[]}` | owner / seller / super admin (D-023) |
| `GET` | `/api/servers/{id}/metrics` | `?since&limit` | `{scope:"server_wide", points[]}` | super admin, or an admin **assigned** to it |
| `GET` | `/api/sync/status` | — | `{stale_after_seconds, servers[{server_id,name,status,last_sync,stale,age_seconds}]}` | super admin / admin (clients → 403) |
| `GET` | `/api/offers` | `?active=1` | `[{id,kind,name,server_id,inbound_id,traffic_gb,duration_days,device_limit,active,…}]` | client → **active FREE only**; admin/super → all (P3) |
| `POST` | `/api/offers` | `{kind,name,server_id,inbound_id?,traffic_gb?,duration_days?,device_limit?,active?}` | `201 {offer}` | super admin only; audited |
| `PATCH` | `/api/offers/{id}` | `{name?,traffic_gb?,duration_days?,device_limit?,active?}` | `200 {offer}` | super admin only; audited; never a delete |
| `GET` | `/api/admins/{id}/servers` | — | `{admin_id, server_ids[]}` | super admin only (P5) |
| `PUT` | `/api/admins/{id}/servers` | `{server_ids[]}` | `{admin_id, server_ids[]}` | super admin only; **replace**, idempotent; audited `admin.servers_assign` |
| `GET` | `/api/clients` | — | `[{id,email,display_name,status,created_at,vpn_count}]` | super admin → all; admin → only clients they sold to; client → **403** (P5) |

**Quota invariant:** consumption happens inside `store.Tx` together with the assignment row; two concurrent requests cannot both take the last slot (`docs/QA_PLAN.md` T-08).

---

## 4. VPN provision & lifecycle *(provisioning + lifecycle implemented — E4 · links/usage planned — E5/E6)*

> **D-024 governs this whole section.** A VPN *is* a 3X-UI client. `GET /api/vpns` returns **tagged panel clients**, and every limit field it exposes is the **panel's own value** (`totalGB`, `expiryTime`, `limitIp`, `traffic`, `enable`), never a value derived from an offer. Free/Premium is the operator's tag, not a property of the panel.

| Method | Path | Body | Response | Rules |
|---|---|---|---|---|
| `POST` | `/api/vpns` | `{offer_id, client_user_id?}` | `201 {vpn, provisioning_state}` | ✅ Free: self-service; Premium: seller with quota. **Panel write first, confirm, then record (D-024.1a)** — a failed create returns an error and leaves **no** VPN row. A FREE claim is booked in the **same transaction**; hitting a cap returns **409 `free_concurrent_limit`** (3 concurrent) or **409 `free_monthly_limit`** (5 per rolling 30 days, D-025.4/.5) and creates nothing |
| `GET` | `/api/vpns/claims/me` | `?server_id=N` (required), `?client_user_id=` (admin only) | `{concurrent, used_this_window, monthly_limit, concurrent_limit}` | ✅ **implemented 2026-10-24** — show the caps **before** claiming, because releasing does not refund a slot (REQUIREMENTS §2.0.1); missing `server_id` → 422; a client asking for another client → 403 |
| `GET` | `/api/vpns` | `?tag=free\|premium\|untagged` | `[{id, server_id, panel_email, status, owner_user_id, seller_user_id, expires_at, created_at, tag\|null, source}]` | ✅ **implemented 2026-10-24 (D-024.3)** — **Super Admin**: panel-backed and includes **untagged** clients (`source:"panel"`, merged with local ownership) or `source:"local"` when the panel could not be read. **Admin/Client**: local rows with a **required** tag join, so only published (tagged) VPNs appear. Unknown `?tag=` → **422** |
| `GET` | `/api/vpns/:id` | — | `{vpn, usage, limits, seller}` | 🟡 404 for non-owners (T-14/T-15); `limits` are panel values |
| `POST` | `/api/vpns/:id/suspend` | — | `200 {vpn}` | ✅ `POST /clients/bulkDisable` |
| `POST` | `/api/vpns/:id/resume` | — | `200 {vpn}` | ✅ `POST /clients/bulkEnable` |
| `POST` | `/api/vpns/:id/revoke` | — | `202 {state:"REVOCATION_PENDING"}` | ✅ verified delete → `REVOKED`; unreachable → stays pending (T-09/T-10) |
| `POST` | `/api/vpns/:id/verify` | — | `200 {exists, status}` | ✅ **implemented** — asks the panel whether the client still exists; positively gone ⇒ `SERVER_UNAVAILABLE`, never `REVOKED` (D-024.5) |
| `DELETE` | `/api/vpns/:id` | — | `204` | ✅ **implemented** — erases **only Veylix's record**, never the panel client (D-027.3). `409 vpn_still_on_panel` while the panel still holds it; admin-only |
| `GET` | `/api/vpns/:id/links` | — | `{links:[...], qr_svg}` | ⬜ **E6** — `GET /clients/links/{email}`; `404` when inactive (T-12) |
| `GET` | `/api/vpns/:id/usage` | `?range=24h\|7d\|30d` | `{series:[{t,up,down}], lifetime:{up,down}}` | ⬜ **E5** — real samples only; gaps labelled |

**Provisioning response states (D-024.1a):** the operation is `CREATE` on the panel → confirmed by `GET /clients/get/{email}` → then `ACTIVE` locally. If the panel rejects or the outcome is ambiguous there is **no VPN row**; the outbox entry records the attempt and the reconciler resolves it. `PENDING` and `REVOCATION_PENDING` therefore never describe a VPN that was never created.

### 4.1 Panel tag management (D-024.3 / D-025) *(planned — Stage D)*

| Method | Path | Body | Response | Rules |
|---|---|---|---|---|
| `GET` | `/api/panel-clients` | `?server_id=&tag=free\|premium\|untagged` | `[{server_id, panel_email, sub_id, inbound_ids, total_gb, expiry_time, limit_ip, limit_hwid, enable, tag\|null, assigned}]` | super admin only; **every** panel client, tagged or not, with the panel's own limits. ⚠️ **the endpoint is implemented as `/api/servers/{id}/panel-clients` (below); this shape is what it returns** |
| `GET` | `/api/servers/:id/panel-clients` | `?tag=free\|premium\|untagged` | `[{panel_email, sub_id, inbound_ids, total_gb, expiry_time, limit_ip, limit_hwid, enable, up_bytes, down_bytes, tag\|null, assigned, veylix_status}]` | ✅ **implemented** — **read-only**, every panel client with the panel's own limits (D-024 Stage C); super admin only; bad `tag` → 422 |
| `PUT` | `/api/panel-clients/{server_id}/{email}/tag` | `{tag:"FREE"\|"PREMIUM"}` | `200 {tagged:true, tag:"FREE"}` | ✅ **implemented** — super admin only; audited `panel_client.tag` (context: `previous`, `tag`); **refused (409 `tag_locked`) once the VPN is issued** (D-025.2) and the refusal is audited as `panel_client.tag_denied` / `DENIED`; unknown tag → 422 |
| `GET` | `/api/panel-clients/{server_id}/{email}/hwids` | — | `200 {server_id, panel_email, devices: [{id, fingerprint, userAgent, deviceOs, osVersion, deviceModel, firstSeen, lastSeen}]}` | **D8** — the panel is the only source; `PermServerManage`; an empty list means "nothing registered", a panel that cannot be read answers **503**, never an empty list. `fingerprint` is 3X-UI's short label, never a full hardware hash |
| `DELETE` | `/api/panel-clients/{server_id}/{email}/hwids/{hwid_id}` | — | `204` | **D8** — frees one device slot; **confirmed by a fresh panel read** before 204 (an unconfirmed removal is an error, not a success); an already-absent device is a success and does not call the panel; audited `panel_client.hwid_revoked` with the fingerprint read *before* deletion; panel refusal → `502 panel_rejected`, unreachable → `503 panel_unavailable` |
### 4.2 My VPNs - client-facing (D-025 / D-027 / D-028) *(status as of 2026-10-24)*

| Method | Path | Body | Response | Rules |
|---|---|---|---|---|
| `GET` | `/api/vpns` | `?tag=free\|premium\|untagged` | `[{id, ..., tag, source}]` | ✅ shipped — see §4 |
| `GET` | `/api/vpns/catalogue` | - | `[free_vpn]` | ⬜ **not built** - a client claims by POSTing an **offer** id today; a panel-first catalogue of FREE-tagged unassigned clients is the remaining piece |
| `POST` | `/api/vpns` | `{offer_id, client_user_id?}` | `201 {vpn_id, panel_email, status, confirmed}` | ✅ shipped - the claim/create path; enforces **3 concurrent** and **5/30 days** (D-025.4/.5), caps shown first via `GET /api/vpns/claims/me` |
| `POST` | `/api/vpns/{id}/release` | `{reason}` | `201 {release}` | ✅ shipped - **3-day grace with undo** (D-027.2) |
| `POST` | `/api/vpn-releases/{id}/restore` | - | `200 {release}` | ✅ shipped - undo before the deadline |
| `POST` | `/api/vpns/{id}/ping` | - | `{latency_ms, at, scope}` | ✅ shipped - **button-triggered only**; no speed test, no background loop (D-026.1) |
| `GET` | `/api/vpns/{id}/links` | - | `{links[], source}` | ✅ shipped - the panel own URIs, passed through untouched |
| `GET` | `/api/vpns/{id}/credential` | - | `{uuid|null, password|null, protocol}` | ⬜ **not built** - owner only; uuid for VLESS, password otherwise |
| `PUT` | `/api/vpns/{id}/credential` | `{uuid? | password?}` | `200 {updated}` | ⬜ **not built** - owner only; must be audited and confirmed by a fresh read (D-028.6) |
| `GET` | `/api/vpns/{id}/usage` | `?range=24h|7d|30d` | `{series[], lifetime, history}` | ✅ shipped - real samples only; `history: NONE` when nothing was sampled |
| `GET` | `/api/servers/{id}/metrics` | `?limit=` | `{scope:"server_wide", points[]}` | ✅ shipped - labelled server-wide, never per-VPN |
| `GET` | `/api/vpns/{id}/devices` | - | `[hwid]` | ⬜ **not built** - HWID list (adapter support exists) |
| `DELETE` | `/api/vpns/{id}/devices/{hwid}` | - | `204` | ⬜ **not built** - HWID revoke (adapter support exists) |
| `GET` | `/api/servers/{id}/health-series` | `?metric=cpu|mem&range=` | `{series[], source:"whole server"}` | ⬜ **not built** - raw points exist; bucketing lives in the UI until it lands |


**Ping note:** measured by the Veylix backend to the panel host (TCP dial latency). It is **not** the user's own latency, and the UI must label it as the server's reachability, not the client's connection quality.

---

## 5. Payments, notifications, directory — **shipped 2026-10-24** (P6/P8)

| Method | Path | Body / query | Response | Rules |
|---|---|---|---|---|
| `GET` | `/api/payment-methods` | — | `[payment_method]` | active global + own |
| `POST` | `/api/payment-methods` | `{kind, label, details, instructions}` | `201 {payment_method}` | super admin → **global** (`owner_user_id: null`); admin → own; client → **403** |
| `PATCH` | `/api/payment-methods/:id` | `{active}` | `200 {payment_method}` | owner or super admin; **never a delete** |
| `GET` | `/api/payments` | `?pending=1` | `[payment]` | super admin all / admin own + sold / client own |
| `POST` | `/api/payments` | `{assignment_id, method_id, amount_cents, reference}` | `201 {payment}` | `amount_cents` is an **integer**; no reference → `PENDING`, with one → `AWAITING_CONFIRMATION` |
| `GET` | `/api/payments/:id` | — | `{payment}` | not the caller's → **404** (never 403: no id enumeration) |
| `POST` | `/api/payments/:id/reference` | `{reference}` | `200 {payment}` | payer only, from `PENDING` only |
| `POST` | `/api/payments/:id/confirm` | — | `200 {payment}` | `payment:confirm`; from `PENDING`/`AWAITING_CONFIRMATION`; **audited**, never automatic (T-17) |
| `POST` | `/api/payments/:id/reject` | — | `200 {payment}` | reviewed once, never re-opened |
| `POST` | `/api/payments/:id/refund` | — | `200 {payment}` | from `CONFIRMED` only |
| `POST` | `/api/payments/:id/cancel` | — | `200 {payment}` | payer, before review (`PENDING` only) |
| `GET` | `/api/notifications` | `?limit=N` | `[notification]` | **own only** (session `user_id`, never a parameter) |
| `GET` | `/api/notifications/unread` | — | `{unread:N}` | own only |
| `POST` | `/api/notifications/:id/read` | — | `200` | own only; a foreign id → **404** |
| `POST` | `/api/notifications/read-all` | — | `{marked:N}` | own only |
| `GET` | `/api/directory` | — | `[{id, email, display_name, role, status, phone, whatsapp, telegram, discord}]` | super admin all / admin own clients + super admins / client super admins + own sellers |

An illegal payment transition answers **`409 illegal_transition`**. A principal with no
`users` row (the dev bypass, D-022) answers **`403 no_payer_account`** — never a 500,
because "who paid" is never invented.

## 5b. Chat — **shipped 2026-10-24** (P7)

Access is enforced by the store: every read joins `chat_participants` on the session's
`user_id`, so "can I see this?" has exactly one implementation.

| Method | Path | Body / query | Response | Rules |
|---|---|---|---|---|
| `GET` | `/api/chat/threads` | — | `[chat_thread]` | participant only; `unread` + `last_message` are per viewer |
| `POST` | `/api/chat/threads` | `{peer_user_id}` | `201 {chat_thread}` | **idempotent per pair** (`pair_key` is `min|max`); peer must be reachable for the caller's role, else **403 `peer_not_reachable`**; self → 403 |
| `GET` | `/api/chat/threads/:id` | — | `{chat_thread}` | not a participant → **403** (the thread exists; hiding that would make the UI lie) |
| `GET` | `/api/chat/threads/:id/messages` | `?after=&limit=` | `[chat_message]` | cursor (`after`), never an offset — an offset skips messages under concurrency. Own hidden messages excluded |
| `POST` | `/api/chat/threads/:id/messages` | `{body}` | `201 {chat_message}` | non-empty, ≤ 64 KiB, thread unlocked (else **409 `chat_locked`**); stamps the sender as read |
| `PATCH` | `/api/chat/messages/:id` | `{body, thread_id}` | `200 {chat_message}` | author only, within the **15-minute** window; the old text is kept in `original_body` |
| `DELETE` | `/api/chat/messages/:id` | `?thread_id=` | `204` | **hides for the caller only** — the peer's copy is evidence; also clears a pin on it |
| `POST` | `/api/chat/threads/:id/pin` | `{message_id}` (`0` = unpin) | `200` | **staff only** (client → 403); **audited** (`chat.pin`) |
| `POST` | `/api/chat/threads/:id/lock` | `{locked}` | `200` | **staff only**; locked = no new messages but the transcript stays readable; **audited** (`chat.lock`) |
| `POST` | `/api/chat/threads/:id/read` | — | `200` | advances the caller's own read marker |
| `POST` | `/api/chat/messages/:id/save` | `{thread_id, on}` | `200` | per viewer |
| `POST` | `/api/chat/messages/:id/important` | `{thread_id, on}` | `200` | per viewer |

**Peer reachability** (mirrors `GET /api/directory`): super admin → anyone; admin → their
own clients + super admins; client → super admins + the resellers who sold to them.

**Message ids answer 404, never 403**, for a non-participant: they are sequential, so a
403 would tell a stranger that a given id exists. The participant check therefore runs
**before** the message lookup.

## 5c. Audit — shipped (Stage E2; filters server-side since D-082, 2026-11-04)

| Method | Path | Body / query | Response | Rules |
|---|---|---|---|---|
| `GET` | `/api/audit` | `?kind=&actor_id=&from=&to=&q=&limit=&offset=` | `{entries:[entry], total, limit, offset, has_more}` | super-admin only (`audit:view`); unknown `kind` → **400 `bad_filter`** |

* Filters (all optional, combined with AND): `kind` ∈ `auth · server · admin · vpn · payment · client · other` — `payment` covers `payment_method.*`, `client` covers `user.*`, `other` is every action matching none of the nameable families; `actor_id` — one actor's rows; `from`/`to` — unix seconds on `created_at`; `q` — substring across `action`, actor name/e-mail, `target_type`, `target_id` and `context` (LIKE-wildcards in the input are escaped, so `"50%"` finds `"50%"`).
* `total` counts matches **after** the filters but **before** the window (`limit` default 100, capped 200; `offset` ≥ 0; `has_more` = `offset+limit < total`) — a filter can never hide its own answer behind a fixed window, which is why the read is an envelope and not a bare array any more.
* Each `entry` = `{id, actor_id, actor_name?, action, target_type, target_id, result, context, created_at}` — `actor_name` is the actor's display name or e-mail (LEFT JOIN; absent for system rows), so the UI need not render `user #12`.
* The trail stays **append-only**: this change widens the READ only — no update or delete path was added (REQUIREMENTS §8).

## 5d. VPN connection — **shipped 2026-10-24** (P10 / Milestone 5-B step 11, D-026)

The endpoints that let a client actually connect. Before these, `GET /api/vpns/{id}/links`
answered 404 and the client screen showed a demo QR — the product's core promise was
unimplemented.

| Method | Path | Response | Rules |
|---|---|---|---|
| `GET` | `/api/vpns/{id}/links` | `{vpn_id, panel_email, status, links[], source}` | ownership enforced (a foreign VPN is **404**, so ids cannot be probed); `links` are the **panel's own URIs, passed through untouched** |
| `POST` | `/api/vpns/{id}/ping` | `{vpn_id, server_id, reachable, latency_ms, scope}` | read-only TCP dial to the panel host; `scope` is always **`server_reachability`** |
| `POST` | `/api/vpns/{id}/credential` | `{value?, random?, current_value?}` → the fresh `ProtocolDetail` | **D-028.6** — rotates the secret the protocol actually uses (`uuid` for vless/vmess, `password` otherwise). Empty `value`/`random:true` generates a crypto/rand v4 UUID or a 20+ char password. The panel is **re-read to confirm** before answering. `422 invalid_credential` (malformed UUID, <8-char or unchanged password), `409 no_credential` (the panel exposes no secret), `502 panel_error`, foreign VPN → **404**. Audits the field, never the value |

**Three states, three meanings — the UI must not collapse them:**
`source:"unavailable"` = the panel could not be read (an outage is *not* "no link");
`links: []` = the panel answered and had no URI; `links: [...]` = a connectable config.

`latency_ms` is **-1** when unreachable — never 0, which would read as "instant".
Live-verified against the real panel: a genuine
`vless://a033313e-…@difanx4f3.difanx1.ggff.net:443?…#(IN)-free_2-vyx-16` was returned
with `source: panel`, and `ping` answered `{reachable:true, latency_ms:1}`.

## 5e. Release / restore — **shipped 2026-10-24** (Milestone 5-B step 9, D-027)

"Storage full" means the VPN's **traffic quota** is exhausted, not the host's disk.
Releasing is undoable for **3 days**; after that only Veylix's record is removed —
**the 3X-UI client is never deleted** (D-027.3).

| Method | Path | Body / query | Response | Rules |
|---|---|---|---|---|
| `POST` | `/api/vpns/{id}/release` | `{reason}` (optional) | `201 {release}` | **owner only** (an operator → **403**); open release already pending → **409 `already_released`** |
| `POST` | `/api/vpn-releases/{id}/restore` | — | `200 {release}` | owner only; past the deadline or already restored → **409 `not_restorable`** |
| `GET` | `/api/vpn-releases` | `?client_user_id=` | `[release]` | a client sees only their own; a reseller cannot read another's client timeline |

`release.restore_deadline` is `released_at + 3 days` and is what the UI counts down.
`panel_client_kept` is always `true` — it is on the wire so a caller can prove the
panel was not touched.

**After the window**, an hourly job removes the local row **and its assignment** in one
transaction and writes a `vpn.purge` audit entry whose context states
`panel_client_kept: true` / *"the 3X-UI client was not deleted"* (D-027.5) — that
sentence is how a later reader tells "we deliberately kept it" apart from "we forgot".
**A release does not refund a FREE monthly claim** (D-027.4).

---

## 6. Mock store → endpoint mapping (D-006 exit)

| Store (`frontend/src/stores/`) | Endpoint(s) | Status |
|---|---|---|
| `authStore` | §1 | ✅ real |
| `serversStore` | §2 | ✅ real |
| `offersStore`, `adminsStore` | §3 | ✅ real |
| `vpnStore`, `clientsStore` | §4 | ✅ real |
| `paymentsStore` | §5 | ✅ **real (2026-10-24)** |
| `notifStore` | §5 | ✅ **real (2026-10-24)** |
| `auditStore` | §5c | ✅ real |
| `peopleStore` | `GET /api/directory` (role-scoped) | ✅ **real (2026-10-24)** |
| `chatStore` | §5b | ✅ **real (2026-10-24) — no mock stores remain** |

---

## 7. Implemented today (2026-10-23) — live-verified against 3X-UI 3.10.2

| Method | Path | State |
|---|---|---|
| `GET` | `/api/health` | **implemented + tested** |
| `GET` | `/api/ready` | **implemented + tested** |
| `POST` | `/api/setup/bootstrap` | **implemented + live** (one-time only) |
| `POST` | `/api/auth/register` | **implemented + live** |
| `POST` | `/api/auth/login` | **implemented + live** |
| `POST` | `/api/auth/logout` | **implemented + live** |
| `POST` | `/api/auth/change-password` | **implemented + tested** (F-016; rotates all sessions) |
| `GET` | `/api/me` | **implemented + live** |
| `GET` | `/api/servers` | **implemented + live** (role-scoped) |
| `POST` | `/api/servers` | **implemented + live** (real health check, sealed token) |
| `POST` | `/api/servers/{id}/test` | **implemented + live** |
| `GET` | `/api/servers/{id}/inbounds` | **implemented + live** |
| `POST` | `/api/servers/{id}/disable` | **implemented** (unit-tested) |
| `PATCH` | `/api/servers/{id}` | **implemented + live** (2026-10-24) — any subset of `{name, panel_url, token, allow_private_network}`; re-verifies the panel; a URL change re-seals the token (AAD bound to the new URL); token never returned; unknown id → 404 |
| `DELETE` | `/api/servers/{id}` | **implemented** (unit-tested; row preserved as `REMOVED`) |
| `GET` | `/api/audit` | **implemented + live** (super admin only) |
| `GET` | `/api/quotas/me` | **implemented + live** (D-021: client → `200 {0,0,-1}`, admin → real limit, super admin `?admin_id=`, anonymous → 401) |
| `GET` | `/api/vpns/{id}/usage` | **implemented + live** (D-023; IDOR-tested: another client → 403) |
| `GET` | `/api/servers/{id}/metrics` | **implemented + live** (real CPU/RAM from `indian1`) |
| `GET` | `/api/sync/status` | **implemented + live** (`stale:false` after a tick) |
| `GET /api/offers` | **implemented + live** (client → FREE only, super admin → all) |
| `POST /api/offers` | **implemented + live** (201; client → 403; 422 on bad input) |
| `PATCH /api/offers/{id}` | **implemented + live** (deactivation removes it from the client catalogue) |
| `GET/PUT /api/admins/{id}/servers` | **implemented + live** (reseller immediately sees the assigned panel; replace semantics tested) |
| `GET /api/clients` | **implemented + live** (super admin 1, fresh reseller `[]`, client 403) |
| `GET` | `/api/setup/status` | **implemented + live** (drives `/setup`) |
| `POST /api/admins`, `GET /api/admins`, `PATCH /api/admins/{id}` | **implemented + live** (role-scoped, 403 for admin/client, 401 anonymous) |
| `GET` | `/api/vpns` | **implemented + live** (role-scoped list) |
| `POST` | `/api/vpns` | **implemented + tested** (201 ACTIVE confirmed, 202 when unconfirmed) |
| `POST` | `/api/vpns/{id}/suspend` | **implemented + tested** |
| `POST` | `/api/vpns/{id}/resume` | **implemented + tested** |
| `POST` | `/api/vpns/{id}/revoke` | **implemented + tested** (200 only when verified; else 202 `REVOCATION_PENDING`) |


**Auth model as built:** HttpOnly session cookie (`Secure` unless a loopback dev bind explicitly opts out), server-side session rows storing only a SHA-256 fingerprint, 12-hour lifetime, CSRF origin check on state-changing requests, rate-limited login, argon2id password hashes.

**Still designed-only:** a panel-first FREE catalogue (`GET /api/vpns/catalogue` / `POST /api/vpns/{id}/claim`), HWID routes (`GET/DELETE /api/vpns/{id}/devices`), a per-client FREE monthly-cap column (D-025.5), `/api/servers/{id}/health-series` and `PATCH /api/me`. Everything else listed above is implemented — `/api/offers`, `/api/admins` (CRUD), `/api/payments`, `/api/chat/*`, `/api/notifications`, `/api/vpns/{id}/links`, `/api/vpns/{id}/usage`, `/api/vpns/{id}/ping`, the release/restore pair, `/api/servers/{id}/panel-clients` + tagging and `PATCH /api/servers/{id}` all shipped. **`POST /api/vpns/{id}/credential` also shipped 2026-11-05 (D-028.6).**

## 7.1 Developer verification surface (not part of the API)

| Tool | What it proves | Where |
|---|---|---|
| `.\dev.ps1 -Status` | both ports listen, `/api/health` is 200, and `/api/me` returns the dev principal (a 401 here means the backend was started without the dev env) | repo root |
| `.\dev.ps1 -Package` | builds the SPA + embeds it + builds `backend/bin/veylixd.exe`, then **runs** it and asserts `/api/ready` says `ui: embedded`, `/dashboard` and `/vpn/16` are 200, `/assets/nope.js` is 404 and the bundle has no dev bypass (D-029) | repo root |
| `npm run audit` | no screen claims a capability the backend contradicts ("demo data", "no API yet", "data is mock", internal stage ids) and no invented business numbers | `frontend/scripts/audit-ui-truth.mjs` |
| `npm run audit:selftest` | the audit rules themselves still fire (6 positive + 4 negative cases) | same |
| `npm run verify:ui -- /path` | a real page renders: title, metric cards, chart marks, empty-state copy, table rows — and **fails on any console error**. `VEYLIX_DEV_ROLE=admin\|client` reviews role-only screens | `frontend/scripts/verify-dashboard.mjs` |
| `frontend/scripts/smoke-*.ps1` | live endpoint smokes through the Vite proxy (`billing`, `chat`, `links`, `notifications`, `releases`) | `frontend/scripts/` |


**Provisioning guarantees as built (Stage E4):**
- local row + quota ledger + outbox entry are written in **one transaction**;
- quota is counted **inside** that transaction (concurrency test proves exactly one winner);
- the panel write happens after the outbox record exists, so an ambiguous outcome is always recoverable;
- every write is confirmed by a **fresh read**; a write that landed despite an error is adopted, never duplicated;
- `REVOKED` is only written after the client is verified absent.



