# VEYLIX — API SPEC

**Status:** authoritative · **Established:** 2026-11-08 · **Baseline:** `1b09553`

**Evidence (C).** Every route below was extracted from the compiled
`mux.Handle` / `mux.HandleFunc` registrations in `backend/internal/httpapi/*.go`
(excluding `_test.go`). **127 distinct routes; `npm run docs:verify` counts 133**
method+path pairs once path parameters are expanded — both numbers are correct and
the difference is parameterised variants, not a discrepancy.

---

## 1. Conventions

| Topic | Rule |
|---|---|
| Base | `/api/…` on the same origin as the UI |
| Auth | HttpOnly session cookie, `veylix_session` |
| Errors | `{"error":{"code":"…","message":"…"}}` — an explicit code, never a bare 500 |
| Ownership | resolved from the session, **never** from a body/query id |
| Pagination | `limit`/`offset`, bounded server-side (max 200) |
| Money | integer minor units + currency code; no floats |

### Error codes actually emitted (C)

`unauthorized` · `forbidden` · `not_found` · `invalid_body` · `validation_failed` ·
`invalid_credentials` · `password_not_set` · `email_unverified` · `wrong_portal` ·
`rate_limited` · `email_taken` · `weak_password` · `csrf_blocked` ·
`cross_origin` · `internal_error` · `not_wired` · `quota_exceeded` ·
`free_concurrent_limit` · `free_monthly_limit` · `offer_inactive` ·
`offer_incomplete` · `plan_not_approved` · `not_plan_owner` ·
`server_not_assigned` · `plan_exhausted` · `plan_archived` ·
`vpn_still_on_panel` · `panel_error` · `panel_rejected` · `bad_filter`

---

## 2. Public (no session)

| Method | Path | Behaviour |
|---|---|---|
| GET | `/api/health` | liveness |
| GET | `/api/ready` | readiness, includes `embedded:N_files` |
| GET | `/api/setup/status` | whether a super admin exists yet |
| POST | `/api/setup/bootstrap` | create the first super admin (refused once one exists) |
| POST | `/api/auth/register` | self-service sign-up; **always** `role=client` |
| POST | `/api/auth/login` | `{identifier, password, portal}` → `client` \| `staff` |
| POST | `/api/auth/forgot` | **always 202**, never an account-existence oracle |
| POST | `/api/auth/reset` | redeem an e-mailed code |
| GET | `/api/auth/google/start` | OAuth begin; 503 when unconfigured |
| GET | `/api/auth/google/callback` | OAuth return |
| GET | `/api/auth/google/status` | whether Google sign-in is available |
| GET | `/api/auth/verify/{token}` | e-mail verification link |
| POST | `/api/auth/verify/resend` | resend a verification link |

**Rate limited:** register, login, forgot, reset, verify/resend.

---

## 3. Self-service (`/api/me`, any signed-in role)

| Method | Path | Behaviour |
|---|---|---|
| GET | `/api/me` | the caller's own principal |
| GET | `/api/me/profile` | full profile incl. contact fields |
| PATCH | `/api/me/profile` | display name, username, bio, avatar, phone, whatsapp, telegram, discord, birthday, timezone |
| GET | `/api/me/username-available` | username availability |
| POST | `/api/me/email-change/request` | start an e-mail change |
| POST | `/api/me/email-change/confirm` | finish it with the code |
| POST | `/api/auth/change-password` | change the caller's password |
| POST | `/api/auth/logout` | revoke the current session |
| POST | `/api/auth/logout-others` | revoke every **other** session |

---

## 4. VPNs (`/api/vpns`)

| Method | Path | Role | Behaviour |
|---|---|---|---|
| GET | `/api/vpns` | all | role-scoped list; `?tag=free\|premium\|untagged` (422 on a bad value) |
| POST | `/api/vpns` | all | provision from `{offer_id}` or `{client_user_id}`; 201 confirmed, 202 not yet, 502 when the panel refused |
| GET | `/api/vpns/{id}/usage` | owner/admin | usage series |
| GET | `/api/vpns/{id}/links` | owner | connection links |
| GET | `/api/vpns/{id}/protocol` | owner | protocol detail |
| GET | `/api/vpns/{id}/traffic/live` | owner | live counters |
| POST | `/api/vpns/{id}/ping` | owner | reachability probe |
| POST | `/api/vpns/{id}/suspend` | owner/admin | panel disable, confirmed |
| POST | `/api/vpns/{id}/resume` | owner/admin | panel enable, confirmed |
| POST | `/api/vpns/{id}/revoke` | owner/admin | panel delete, confirmed |
| POST | `/api/vpns/{id}/release` | owner/admin | **Veylix only** — panel untouched |
| POST | `/api/vpns/{id}/transfer` | owner/admin | move ownership to another client |
| GET | `/api/vpns/{id}/transfers` | owner/admin | transfer history |
| POST | `/api/vpns/{id}/verify` | owner/admin | ask the panel whether the client exists |
| GET | `/api/vpns/claims/me` | all | FREE allowance; `server_id` **required** (422 otherwise) |
| DELETE | `/api/vpns/{id}` | super admin | hard delete; 409 `vpn_still_on_panel` while the panel holds it |
| GET | `/api/vpn-releases` | super admin | release tombstones |
| POST | `/api/vpn-releases/{id}/restore` | super admin | restore a released VPN |

---

## 5. Offers, plans, pricing

| Method | Path | Role | Behaviour |
|---|---|---|---|
| GET | `/api/offers` | all | **client sees active FREE only**; staff see the catalogue |
| GET | `/api/offers/premium` | all | the resale catalogue a client may buy from |
| POST | `/api/offers` | super admin | create; **422 `offer_incomplete` when `inbound_id` is missing** |
| PATCH | `/api/offers/{id}` | super admin | limits + active toggle; never a delete |
| GET | `/api/plans` / `/api/packages` | all | plans / packages |
| GET | `/api/plans/{id}` | all | one plan |
| POST | `/api/plans` | super admin | create |
| PATCH | `/api/plans/{id}` | super admin | edit |
| POST | `/api/plans/{id}/archive` | super admin | archive (no hard delete) |
| DELETE | `/api/plans/{id}` | super admin | remove |
| GET | `/api/pricing` | staff | unit rates |
| PUT | `/api/pricing` | super admin | set rates |
| POST | `/api/orders/quote` | client | price a CUSTOM order |

---

## 6. Servers and panel

| Method | Path | Role | Behaviour |
|---|---|---|---|
| GET | `/api/servers` | staff | role-scoped server list |
| POST | `/api/servers` | super admin | register a panel; token sealed at rest |
| PATCH | `/api/servers/{id}` | super admin | edit |
| DELETE | `/api/servers/{id}` | super admin | hard delete; 409 while VPNs point at it |
| POST | `/api/servers/{id}/test` | super admin | live health check |
| POST | `/api/servers/{id}/disable` | super admin | stop using it |
| GET | `/api/servers/{id}/inbounds` | staff | **the panel's own inbound numbers** |
| GET | `/api/servers/{id}/panel-clients` | super admin | every panel client + tag + assignment |
| GET | `/api/servers/{id}/metrics` | staff | server metrics |
| POST | `/api/panel-clients/{server_id}/{email}/assign` | super admin | assign a panel client to a user |
| PUT | `/api/panel-clients/{server_id}/{email}/tag` | super admin | set FREE/PREMIUM |
| GET | `/api/panel-clients/{server_id}/{email}/hwids` | staff | device IDs |
| DELETE | `/api/panel-clients/{server_id}/{email}/hwids/{hwid_id}` | staff | remove one |
| GET | `/api/sync/status` | staff | sync worker state |
| POST | `/api/sync/run` | super admin | force a sync cycle |

> **The `inbound_id` trap.** `GET /api/servers/{id}/inbounds` returns the **panel's**
> numbers (1, 2, 3). `POST /api/offers` requires Veylix's **local** row id (52, 53,
> 54). They are not the same number. Passing the panel's number produces a foreign
> key failure; passing the local number is correct because `provisioning` translates
> it via `inbounds.remote_id` before calling the panel. See `DATABASE.md` §4.

---

## 7. Clients, quotas, staff, admins

| Method | Path | Role | Behaviour |
|---|---|---|---|
| GET | `/api/clients` | admin, super admin | the reseller's clients (row-scoped) |
| GET | `/api/super/clients` | super admin | all clients |
| GET | `/api/super/clients/{id}` | super admin | one client |
| PATCH | `/api/super/clients/{id}` | super admin | edit |
| DELETE | `/api/super/clients/{id}` | super admin | delete (terminal) |
| POST | `/api/super/clients/{id}/reset-password` | super admin | issue a password |
| POST | `/api/super/clients/{id}/email-change` | super admin | start an e-mail change |
| POST | `/api/super/clients/{id}/email-change/confirm` | super admin | finish it |
| GET | `/api/quotas/me` | admin | the caller's own quota |
| GET | `/api/admins` | super admin | staff resellers |
| POST | `/api/admins` | super admin | create a reseller |
| PATCH | `/api/admins/{id}` | super admin | edit |
| GET | `/api/admins/{id}/servers` | super admin | which servers they may use |
| PUT | `/api/admins/{id}/servers` | super admin | set that assignment |
| GET | `/api/super/staff` | super admin | staff list |
| POST | `/api/super/staff` | super admin | create |
| GET | `/api/super/staff/{id}` | super admin | one |
| PATCH | `/api/super/staff/{id}` | super admin | edit |
| POST | `/api/super/staff/{id}/reset-password` | super admin | issue a password |
| GET | `/api/directory` | staff | user directory for pickers |
| GET | `/api/audit` | super admin | filtered envelope; unknown `kind` is **400**, never a silent empty page |
| GET | `/api/super/vpns` | super admin | panel-backed catalogue with owner + usage |
| GET | `/api/super/settings/google` | super admin | Google OAuth config |
| PUT | `/api/super/settings/google` | super admin | set it (secret sealed) |

---

## 8. Payments and orders

| Method | Path | Role | Behaviour |
|---|---|---|---|
| GET | `/api/payments` | staff | role-scoped queue |
| POST | `/api/payments` | client/admin | create an order |
| GET | `/api/payments/{id}` | owner/staff | one order |
| GET | `/api/payments/{id}/events` | owner/staff | timeline |
| POST | `/api/payments/{id}/proof` | client | upload proof |
| POST | `/api/payments/{id}/method` | client | choose a channel |
| POST | `/api/payments/{id}/reference` | client | attach a reference |
| POST | `/api/payments/{id}/cancel` | client | cancel |
| POST | `/api/payments/{id}/confirm` | **super admin only** | the only path that releases an unowned premium plan |
| POST | `/api/payments/{id}/release-target` | super admin | choose which panel/inbound the VPN is created on |
| GET | `/api/payment-methods` | all | channels |
| POST | `/api/payment-methods` | super admin | create |
| PATCH | `/api/payment-methods/{id}` | super admin | edit |

---

## 9. Chat and notifications

| Method | Path | Role | Behaviour |
|---|---|---|---|
| GET | `/api/chat/threads` | staff, client | the caller's conversations |
| POST | `/api/chat/threads` | staff, client | start one |
| GET | `/api/chat/threads/{id}` | participant | one thread |
| GET | `/api/chat/threads/{id}/messages` | participant | messages |
| POST | `/api/chat/threads/{id}/messages` | participant | post |
| POST | `/api/chat/threads/{id}/read` | participant | mark read |
| POST | `/api/chat/threads/{id}/lock` | staff | lock |
| POST | `/api/chat/threads/{id}/pin` | staff | pin |
| GET | `/api/chat/threads/{id}/hidden` | participant | hidden messages |
| POST | `/api/chat/messages/{id}` | author | edit |
| PATCH | `/api/chat/messages/{id}` | author | patch |
| DELETE | `/api/chat/messages/{id}` | author/staff | delete |
| POST | `/api/chat/messages/{id}/unhide` | staff | restore |
| GET | `/api/notifications` | caller | **own inbox only** — scoped by `user_id` in the query |
| GET | `/api/notifications/unread` | caller | badge count |
| POST | `/api/notifications/{id}/read` | owner | idempotent; a foreign id is `not_found` |
| POST | `/api/notifications/read-all` | caller | clear the badge |

**`notifications.kind` is a SEVERITY**, not a category: `info | success | warning |
error`. There is no category column, so a "category" filter over this data would
group rows by colour and silently show the wrong thing.

---

## 10. System

| Method | Path | Role | Behaviour |
|---|---|---|---|
| GET | `/api/system/version` | all | running version |
| GET | `/api/system/update-check` | staff | is a newer release available |
| GET | `/api/system/releases` | staff | release feed |

---

## 11. Authorization pattern (C)

Every protected route is wrapped in `authed(...)`; privileged routes add
`RequirePermission(auth.PermX)` or an explicit role check. Three verifications
happen on **every** request:

1. **Session** — a valid server-side session, else 401.
2. **Role** — the route's permission, else 403.
3. **Ownership** — the row belongs to the caller (or the caller is entitled to it),
   else **404**, deliberately indistinguishable from "does not exist" so ids cannot
   be probed.

**A UI that hides a button is not authorization.** The unauthenticated, wrong-role
and wrong-owner cases are covered in `internal/httpapi/*_test.go` and in
`npm run preflight`.


