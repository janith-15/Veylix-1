# VEYLIX — 3X-UI API CONTRACT (verified reference)

**Purpose:** the single place where the 3X-UI HTTP contract Veylix codes against is recorded (`new_update.md` §2, §5, §9).
**Spec source (machine-readable):** the panel's own `GET <base>/panel/api/openapi.json` (authoritative per deployment)
**Human docs:** https://docs.sanaei.dev/docs/reference/api/

> **Rule:** no endpoint may be coded unless it appears in this file with a verified method + path.
> **Rule:** every panel instance is re-verified at runtime — `GET <base>/panel/api/openapi.json` is the authoritative contract for that integration.

---

## 0. Verified test panel (2026-10-23) — LIVE

| Fact | Verified value |
|---|---|
| Host | `difanx4f3.difanx1.ggff.net:2053` (HTTPS) |
| **Base path** | **`/QpKFsYpDlLFdRsOoW7`** — the panel web base path; **every** API path is `<base>/panel/api/...` |
| Panel version | **3.10.2** (`obj.panelVersion`) |
| Xray | **26.10.10**, state `running` |
| Host resources | `cpuCores: 1`, `logicalPro: 2`, `mem.total: 998,576,128` (~952 MB), `disk.total: 47.3 GB` |
| Auth used | Bearer API token (stored encrypted, D-016) |
| Verified reachable | `GET /panel/api/server/status` → 200 · `GET /panel/api/inbounds/list` → 200 · `GET /panel/api/inbounds/options` → 200 · `GET /panel/api/clients/list/paged` → 200 · `GET /panel/api/openapi.json` → 200 (254 KB) |

**Note for the adapter:** without the base path every call returns **`404` with an empty body** — not `401`. A 404 must therefore never be interpreted as "token invalid". `docs/QA_PLAN.md` T-03 covers this.

### Credential handling rule (owner instruction, 2026-10-23)
The panel URL + token are kept **only** in `.veylix-secrets/dev-env.ps1`, which is git-ignored (verified via `git check-ignore`). They must never appear in source, docs, logs, API responses, or commits. The token is sealed with AES-256-GCM in `server_integrations.token_ciphertext` (D-016) and is supplied to the panel client only at call time.

---

## 1. Authentication

| Method | How | Notes |
|---|---|---|
| Bearer token | `Authorization: Bearer <token>` (`bearerAuth`) | Token created in panel **Settings → Security → API Token**. It is a **full-admin credential** → P0-class secret. |
| Session cookie | `POST /login` → cookie `3x-ui` (`cookieAuth`) | Browser-only; requires a 2FA code when enabled. |

**Decision (D-011):** Veylix stores a **Bearer token per integration**. The `/login` flow is rejected because it needs username+password+2FA state, rotates a session cookie, and is documented as browser-only.

**Rules**
- The token lives only in the backend, encrypted at rest (AES-256-GCM).
- It must never appear in an API response, a log line, an error message, a frontend bundle, or a Git object (Bible §0.6, `new_update.md` §9).
- `POST /login` body: `{"username","password","twoFactorCode?"}` — recorded for completeness only.

**Base path:** all management endpoints are under `<base>/panel/api/*`.


---

## 2. Server / health

| Method | Path | Response highlights |
|---|---|---|
| `GET` | `/panel/api/server/status` | `{cpu, mem{current,total}, swap{current,total}, disk{current,total}, netIO{up,down}, xray{state,version}, tcpCount, load{load1,load5,load15}}` — **cached, refreshed every 2 s** |
| `GET` | `/panel/api/openapi.json` | the deployed panel's own spec (runtime capability discovery) |
| `GET` | `/panel/api/server/fail2banStatus` | `{enabled, installed, usable, windows}` — gates per-client IP-limit enforcement |

**Usage:** `/server/status` is the **authenticated health check** for an integration. It proves token validity *and* returns the Xray version. Because the panel caches it for 2 s, Veylix must not poll it faster than ~30 s per panel (500 MB budget, `new_update.md` §8).

---

## 3. Inbounds

| Method | Path | Notes |
|---|---|---|
| `GET` | `/panel/api/inbounds/list` | full payload — every inbound **including `settings.clients[]` and `clientStats[]`**; expensive on large panels |
| `GET` | `/panel/api/inbounds/list/slim` | same shape, `settings.clients[]` reduced to `{email, enable, comment}`, no enriched stats |
| `GET` | `/panel/api/inbounds/options` | dropdown projection: `id, remark, tag, protocol, port, tlsFlowCapable, ssMethod` — **use for pickers** |
| `GET` | `/panel/api/inbounds/get/{id}` | full single inbound incl. complete per-client payload (uuid, password, flow) |
| `POST` | `/panel/api/inbounds/add` | create |
| `POST` | `/panel/api/inbounds/update/{id}` | update |
| `POST` | `/panel/api/inbounds/setEnable/{id}` | enable / disable |
| `POST` | `/panel/api/inbounds/del/{id}` | delete |
| `POST` | `/panel/api/inbounds/bulkDel` | bulk delete |
| `POST` | `/panel/api/inbounds/{id}/resetTraffic` | reset inbound counters |
| `POST` | `/panel/api/inbounds/resetAllTraffics` | reset all |
| `GET` | `/panel/api/inbounds/allLinks` | all connection links |
| `GET`/`POST` | `/panel/api/inbounds/{id}/fallbacks` | fallback rules (master/sub-inbound) |
| `POST` | `/panel/api/inbounds/{id}/delAllClients` | destructive — **must never be called by Veylix** |
| `POST` | `/panel/api/inbounds/pushClientTraffics` | node→master traffic push (cluster feature, unused) |

`settings`, `streamSettings`, `sniffing` are returned as **nested JSON objects**; legacy callers may still send them as JSON-encoded strings on write.

---

## 4. Clients (first-class entities, attachable to many inbounds)

| Method | Path | Notes |
|---|---|---|
| `POST` | `/panel/api/clients/add` | body **JSON** `{client: Client, inboundIds: int[]}` — *"Per-protocol secrets are generated server-side when omitted, so callers can send only the universal fields."* |
| `GET` | `/panel/api/clients/list` | all clients + attached `inboundIds` + traffic record |
| `GET` | `/panel/api/clients/list/paged` | server-side `page`, `pageSize` (max 200), `search`, `filter` (online/active/deactive/depleted/expiring), `protocol`, `inbound`, `sort`, `order`, `expiryFrom/To`, `usageFrom/To`; exact `*Count` summary + slim rows |
| `GET` | `/panel/api/clients/get/{email}` | full single client payload (used for **fresh-read verification**) |
| `GET` | `/panel/api/clients/get/tgId/{tgId}` | lookup by Telegram ID |
| `POST` | `/panel/api/clients/update/{email}` | update (expiry, limits, enable, protocol secrets…) |
| `POST` | `/panel/api/clients/del/{email}` | delete — destructive |
| `POST` | `/panel/api/clients/bulkDisable` · `bulkEnable` | suspend / resume without deletion |
| `POST` | `/panel/api/clients/bulkCreate` · `bulkDel` · `bulkAttach` · `bulkDetach` · `bulkAdjust` · `bulkResetTraffic` | bulk operations |
| `GET` | `/panel/api/clients/traffic/{email}` | per-client traffic record |
| `POST` | `/panel/api/clients/resetTraffic/{email}` | reset a client's counters |
| `POST` | `/panel/api/clients/onlines` | `["user1","user2"]` — currently connected client emails |
| `POST` | `/panel/api/clients/lastOnline` | last-seen timestamps |
| `POST` | `/panel/api/clients/ips/{email}` | source IPs (needs panel-side IP tracking) |
| `POST` | `/panel/api/clients/clearIps/{email}` | clear recorded IPs |
| `GET` | `/panel/api/clients/links/{email}` | **connection links for that client — the only legitimate QR/link source** |
| `GET` | `/panel/api/clients/subLinks/{subId}` | subscription links for a subId |
| `POST` | `/panel/api/clients/{email}/externalLinks` | external (Happ/deeplink) links |
| `POST` | `/panel/api/clients/{email}/attach` · `/detach` | attach / detach additional inbounds |
| `GET` | `/panel/api/clients/groups` (+ create / rename / delete / bulkAdd / bulkRemove / resetTraffic) | client groups |
| `GET` | `/panel/api/clients/export` · `POST` `…/import` | bulk portability |
| `POST` | `/panel/api/clients/delDepleted` · `delOrphans` | cleanup — destructive, not used by Veylix |
| `DELETE`/`POST` | `/panel/api/clients/hwids/{email}[/{id}]` | hardware-ID (device) management |
| `POST` | `/panel/api/clients/renewalPreview` | renewal quote preview |

### `Client` schema — required fields

`comment`, `email`, `enable`, `expiryTime`, `limitIp`, `reset`, `resetDay`, `resetMax`, `resetWeekday`, `security`, `subId`, `tgId`, `totalGB`

Optional: `id` (uuid), `flow`, `auth`, `password`, `secret`, `privateKey`, `publicKey`, `preSharedKey`, `allowedIPs`, `allowedIPsByInbound`, `keepAlive`, `forwardedPorts`, `group`, `reverse`, `trafficReset`, `trafficResetDay`, `adTag`, `created_at`, `updated_at`.

`ClientTraffic` / `clientStats` fields: `id`, `inboundId`, `email`, `up`, `down`, `total`, `expiryTime`, `enable`, `uuid`, `subId`, `lastOnline`, `lastSubFetch`, `reset`, `resetCount`, `resetDay`, `resetMax`, `resetWeekday`.

### 4.1 Verified against the published OpenAPI spec (2026-10-24)

Source: `https://docs.sanaei.dev/openapi.json` (502 KB, **214 paths**, `3X-UI Panel API v3.x`). The deployed panel serves the same document at `GET /panel/api/openapi.json` and **that** copy is authoritative per integration (§6).

**Client write shape — `POST /panel/api/clients/add`**, per the spec's own description:

> *"Client fields: `email`, `subId`, `id` (uuid), `password`, `auth`, `flow`, `totalGB`, `expiryTime`, `limitIp`, **`limitHwid`**, `tgId`, `comment`, `enable`."*

**`POST /panel/api/clients/update/{email}` is a REPLACE, not a patch** — the spec:

> *"…supply the **full set of fields you want to keep** (the server **replaces the row, it does not patch**)."* It also applies to every attached inbound **concurrently and independently**, so a `success:false` response can still have applied the edit to some inbounds. Example body: `{email,totalGB,expiryTime,limitHwid,tgId,enable}`.

**Read shape differs from the write shape.** `ClientRecord` (returned by `clients/get` / `clients/list`) exposes `limitHwid`, `uuid`, `id` as an **integer** row id, and `createdAt`/`updatedAt` — so a read → modify → write cycle **must convert field names and types** or it silently corrupts the record.

**Hardware (device) limits are a first-class API concept:**

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/panel/api/clients/hwids/{email}` | list registered HWID devices (short fingerprints only) |
| `DELETE` | `/panel/api/clients/hwids/{email}/{id}` | remove one device, freeing a slot under the HWID limit |
| `DELETE` | `/panel/api/clients/hwids/{email}` | clear all devices for a client |

**Outbounds: there is no per-outbound CRUD.** The only write path is a whole-config overwrite:

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/panel/api/xray/` | read the Xray config template (`xraySetting` JSON **string**) + `inboundTags` + `clientReverseTags` + `outboundTestUrl` |
| `POST` | `/panel/api/xray/update` | **save the entire Xray config** (form field `xraySetting`) |
| `POST` | `/xray/testOutbound` · `/xray/testOutbounds` · `/xray/routeTest` | reachability / routing tests |
| `GET` | `/xray/getOutboundsTraffic` · `POST /xray/resetOutboundsTraffic` | outbound counters |
| `GET,POST` | `/xray/outbound-subs` (+ `/{id}`, `/{id}/del`, `/{id}/move`, `{id}/refresh`, `/parse`) | outbound subscriptions (WARP/NordVPN…) |

**Consequences (owner decision):** Veylix does **not** create or edit outbounds. A whole-config write has no per-object id, races any concurrent panel edit, can restart Xray (dropping every connected user), and cannot be validated by Veylix — a malformed outbound takes the entire Xray service down. Inbound create/update/delete is left to the panel for the same class of reasons.

**Additional endpoints available but unused:** `clients/onlines`, `clients/lastOnline`, `clients/ips/{email}`, `clients/groups*`, `clients/export` / `import`, `clients/renewalPreview`, `inbounds/allLinks`, `server/history/{metric}/{bucket}`, `server/xrayMetricsState`, `server/logs/{count}`.

---

## 5. Veylix mapping rules (derived from this contract)
| Veylix need | Contract decision |
|---|---|
| Health check / capability detect | `GET /server/status` → token valid + `xray.version` + `xray.state` |
| Inbound picker | `GET /inbounds/options` (small payload) |
| Client sync | `GET /clients/list/paged` — **never** full `/inbounds/list` for list pages |
| Provision Free/Premium | `POST /clients/add` with `client.email` = Veylix-generated stable key and `client.subId` = deterministic value |
| Duplicate prevention (idempotency) | `email` + deterministic `subId` (`vyx-<localClientID>`) — a retry re-uses the same pair, so a duplicate cannot be created silently |
| Ambiguous write (timeout) | fresh read `GET /clients/get/{email}` before any retry (outbox, F-009) |
| Suspend | `POST /clients/bulkDisable` (reversible) — **not** `del` |
| Revoke | `POST /clients/del/{email}` — only after a verified response may the state become `REVOKED`; otherwise `REVOCATION_PENDING` |
| QR / connection link | `GET /clients/links/{email}` — never synthesized |
| Traffic / usage | `clientStats` from `/inbounds/list` or `GET /clients/traffic/{email}` |
| Online status | `POST /clients/onlines` |
| Server metrics | `GET /server/status` (whole-server — must be labelled as such) |
| Device limit | `limitIp` on create/update |

**Endpoints Veylix must not call:** `inbounds/{id}/delAllClients`, `clients/delDepleted`, `clients/delOrphans`, `clients/resetAllTraffics`, `inbounds/resetAllTraffics`, `inbounds/pushClientTraffics` — bulk/destructive or cluster-internal.

---

## 6. Version handling

| Concern | Rule |
|---|---|
| Target version | latest verified **v3.10.2**; the deployed version may differ |
| Runtime detection | `GET /panel/api/openapi.json` per integration; cache the spec hash + `xray.version` from `/server/status` |
| Capability gating | if the deployed spec lacks an endpoint, the capability is disabled in the UI with an explicit "unsupported by this panel version" state — **never** a silent fallback |
| Legacy JSON-string fields | accept both nested objects and JSON-encoded strings for `settings`/`streamSettings`/`sniffing` on write |

---

## 7. Status mapping (HTTP outcome → Veylix state)

| Outcome | Integration state |
|---|---|
| 200 + `success:true` | `ACTIVE` |
| 401 / 403 | `REAUTH_REQUIRED` |
| timeout / DNS / TLS failure / 5xx / malformed body | `CONNECTION_UNAVAILABLE` |
| never successfully checked yet | `PENDING_VERIFICATION` |
| operator disabled | `DISABLED` |
| operator removed | `REMOVED` (history preserved) |

**Never:** outage → `REVOKED` (REQUIREMENTS §7, Bible §7).

---

## 8. Unverified items (open)

| Item | Reason |
|---|---|
| Exact per-protocol `Client` requirements | spec says secrets are generated server-side when omitted; per-protocol behaviour needs a live panel to confirm |
| `hwids` device-limit semantics | documented endpoint, unclear interaction with `limitIp` on older versions |
| Failed-write error shape | `{success, msg}` documented, but per-endpoint failure codes need a live panel |
| Behaviour under concurrent writes | untested |

These are carried as **"blocked — needs live panel"** in `docs/IMPROVEMENT_LEDGER.md`.
