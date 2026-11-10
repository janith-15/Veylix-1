---
name: veylix-3xui-integration
description: Use when a change touches the 3X-UI panel adapter, integrations, provisioning, the outbox, sync or usage snapshots in Veylix. Encodes the verified 3X-UI contract rules - the web base path means every call is under base/panel/api, a missing base path returns 404 rather than 401, Bearer token auth only, runtime capability discovery via openapi.json, the exact status mapping, the 10 second per-request timeout, health polling no faster than 30 seconds, and provisioning as write then fresh read then record. Load before editing backend/internal/panel, integrations, provisioning or sync.
---

# VEYLIX — 3X-UI INTEGRATION

Authoritative contract: `./docs/3XUI_API_CONTRACT.md`. Read the section for the
endpoint you are touching; do not code an endpoint that is not in that file with
a verified method and path.

## Rule zero — the base path

Every management call is `<base>/panel/api/...`, where `<base>` is the panel's
**web base path**, not the host root.

> A wrong or missing base path returns **`404` with an empty body** — *not* `401`.

Therefore: **`404` must never be interpreted as "token invalid".** An invalid
token is `401`. A missing base path is `404`. Getting this backwards produces a
"credentials rejected" bug that is really a URL bug.

## Authentication (D-011)

- **Bearer token only.** The `/login` flow is rejected: it needs username +
  password + optional 2FA, rotates a `3x-ui` session cookie, and is documented
  as browser-only.
- The token is a **full-admin credential** → P0 secret (see `veylix-security`).
- Stored sealed in `server_integrations.token_ciphertext`; supplied to the panel
  client only at call time; never returned, logged or embedded.

## Capability discovery, not assumption

`GET <base>/panel/api/openapi.json` is the **authoritative** contract for that
deployed instance. Discover at runtime and adapt. Never assume a field exists
because a newer panel has it. Never fabricate traffic fields, QR/config links,
historical statistics or a successful remote revocation.

## Verified endpoints (shape, for orientation only)

| Method | Path | Note |
|---|---|---|
| `GET` | `/panel/api/server/status` | authenticated health check; **panel caches 2 s** |
| `GET` | `/panel/api/openapi.json` | per-deployment capability source |
| `GET` | `/panel/api/inbounds/list` | inbounds |
| `GET` | `/panel/api/inbounds/options` | inbound options |
| `GET` | `/panel/api/clients/list/paged` | paged client list |
| `GET` | `/panel/api/clients/get/{email}` | confirm a single client |

A row here is orientation, not permission. Confirm against
`./docs/3XUI_API_CONTRACT.md` and the live `openapi.json` before coding.

## Status mapping (HTTP outcome → Veylix state)

`2xx` → success. `401` → credential invalid. **`404` → base path / route wrong.**
`5xx` or timeout → unreachable, which is a **pending/unknown** state, never a
success and never a fabricated `REVOKED`.

## Rate and budget discipline (1 vCPU / 500 MB)

- Per-request timeout **10 s**; bounded retries on **`GET` only**.
- Health polling **no faster than ~30 s per panel** (the panel caches status 2 s).
- Client lists via `/clients/list/paged`, **≤ 200 rows** per fetch — never pull
  `/inbounds/list` to enumerate clients.
- One slow panel must never stall another. Bounded concurrency, always.

## Provisioning is write → fresh read → record

1. **Write** to the panel.
2. **Fresh read** back (`GET /clients/get/{email}`) to confirm it exists.
3. **Only then record** the local mirror row in `vpn_instances`.

The row is a **mirror of a confirmed client**, never an intent row.
A local transaction **never** spans the remote call — the ambiguous case is
recorded once in `outbox` (`UNIQUE(server_id, op, idempotency_key)`) and
reconciled. A retried provision must never create a second panel client
(`vpn_instances UNIQUE(server_id, panel_email)`).

A panel rejection is a **failure**: roll back the local row, its assignment and
the quota slot in one transaction (`discardProvisional`), answer `502
panel_rejected`. There is no phantom VPN (D-024.1a).

## The panel email IS the VPN's name (D-034, D-091.4)

A client's `email` is derived from the human who owns it — never an opaque id:

```
<client account email>_<kind>_<n>        nimal@gmail.com_free_1
                                          nimal@gmail.com_paid_2
```

- `kind` is `free` or `paid`. `<n>` is a per-**(client, server, kind)** counter that
  **only moves forward**: a revoked `_paid_2` is never re-issued, so the next paid
  VPN is `_paid_3` (D-034.2). That monotonicity is what makes a panel email a stable
  audit trail rather than a recycled label.
- `panel_sub_id` stays `vyx-<vpn_instance_id>` so D-012's idempotency survives the
  human-readable name (D-034.3).
- The name is assigned **at creation, inside the provisioning transaction** — never
  guessed afterwards (D-034.4). `domain.PanelBase` / `PanelFindNextFreeSlot` /
  `PanelSlotNext` are the only producers; never hand-build one.
- **Assigning a client created directly in 3X-UI renames it once** into the same
  scheme (D-034.5, D-037.2). The rename runs **after** the assignment is committed,
  and a failed rename changes nothing — the VPN keeps the name it already has, which
  is a finished state, not a half-applied one (D-063). See `RenameAssignmentName`.
- An opaque `vyx-21` on the panel for a **new** VPN is a defect, not a name.
- **`email` is unique per panel** — that is exactly why the slot counter exists, and
  why a rename must check the target before writing.

## FREE and PREMIUM are the same panel object (D-091.2)

Both are identical 3X-UI clients. The FREE/PREMIUM distinction is a **business**
label Veylix stores locally and **never writes to the panel** (D-024.3): PREMIUM is
sold for revenue, FREE is given away to acquire customers. Concretely:

- Do **not** infer a kind from the panel email, the inbound, the protocol or a
  panel-side `comment`. Only Veylix's own tag decides.
- Do **not** write the tag to the panel's `comment` — a panel-side edit could
  silently reclassify a VPN.
- **Only the Super Admin sets or changes the tag** (`PermOfferManage`, `PUT
  /api/panel-clients/{server_id}/{email}/tag`), and it **locks** once the VPN is
  issued, so nobody can re-price what a customer already holds.
- FREE device limits are a platform rule (`limitIp=2`, `limitHwid=2`) and are not
  taken from the offer; PREMIUM keeps the operator's value (D-028.3/.4).

## A missing client is surfaced, not hidden (D-091.5, INV-1)

A client missing from a page-through becomes `SERVER_UNAVAILABLE` — **never**
`REVOKED`, and its row is **never** deleted — until a direct
`GET /clients/get/{email}` positively reports it absent. An unreadable panel changes
nothing. Whatever the backend concludes, the customer-facing screen must **say it**:
a status chip alone is not an explanation.

## Release and revocation


**Release is local, never remote.** A released VPN is removed from Veylix after a
3-day grace with undo. **The panel client is never deleted.** Do not "fix" a
release by adding a remote delete.

## Integration findings (R5) — historical only

Prior investigation covered the API and 3X-UI integration surface. It is
**historical context**: re-read `./backend/internal/panel` and
`./docs/3XUI_API_CONTRACT.md` as they are now. Never assume a previously
verified endpoint or status still behaves the same — the deployed panel is the
runtime authority.

## Checklist

- [ ] Endpoint exists in `./docs/3XUI_API_CONTRACT.md` with method + path.
- [ ] Base path applied; `404` is not treated as auth failure.
- [ ] Token sealed, never logged, never in a response.
- [ ] Timeout 10 s; retries on `GET` only.
- [ ] Health poll ≥ 30 s per panel.
- [ ] Provisioning did write → fresh read → record.
- [ ] Failure state is explicit and pending, not a pretend success.
- [ ] Panel email built by `domain.PanelBase` / `PanelFindNextFreeSlot`, never by hand.
- [ ] The slot was reserved **inside** the provisioning transaction.
- [ ] The tag was not written to the panel, and not inferred from a panel-side field.
- [ ] Any screen listing the VPN shows the panel identity, readable and copyable.
- [ ] A `SERVER_UNAVAILABLE` client is explained to the customer, not just coloured.
