# VEYLIX — MASTER SPEC

**Status:** authoritative. This file replaces `MASTER_PRODUCT_BIBLE.md` as the
product specification. The Bible is preserved under
`H:\my apps\_veylix-archive\docs-legacy-2026-11-08\` and is **historical
reference only**; where the two disagree, this file governs.

**Established:** 2026-11-08 · **Baseline commit:** `1b09553`

**Evidence rule.** Every statement carries a marker:

| Marker | Meaning |
|---|---|
| **C** | **Confirmed** — verified in code, schema, or a read-only query during the Phase 2 audit |
| **A** | **Assumed** — documented somewhere but not re-verified |
| **U** | **Unknown** — see `docs/restart/UNKNOWNS.md` |

A statement with no marker is a **requirement** (a rule the system must obey),
not a claim about the implementation.

---

## 1. What Veylix is

A **self-hosted VPN reseller platform**. One Go binary serves the HTTP API and the
React interface, which is compiled and embedded into that binary. **3X-UI is the
source of truth for every VPN.** Veylix mirrors the panel and never invents a VPN.

**Deployment reality (C):** a single process on 1 vCPU / 500 MB. No Redis, no
PostgreSQL, no Docker, no persistent Node process. `backend/go.mod` shows exactly
two direct dependencies: `golang.org/x/crypto` (argon2id) and
`modernc.org/sqlite` (pure Go, no CGO).

---

## 2. Roles

**C** — `auth.Role` defines exactly three, and the database constrains them.

| Role | Identity | Capability |
|---|---|---|
| **Super Admin** | platform owner | panels, plans, pricing, payments, staff accounts, every server, every VPN |
| **Admin** | reseller | a quota of sellable slots; clients; the servers assigned to them |
| **Client** | end customer | claims a public FREE VPN; buys PREMIUM from a seller; sees only their own VPNs |

**Rule.** Authorization is enforced **server-side on every request**, checking
**role and row-level ownership**. A UI that hides a control is not authorization.

---

## 3. The rules that define the product

These are the non-negotiables. Each is implemented today (**C**) and must survive
any change.

1. **A VPN *is* a 3X-UI client.** The panel is the only authority on what a VPN is.
2. **The panel wins.** On any disagreement between Veylix and 3X-UI, the panel is
   right. Veylix mirrors; it never invents.
3. **FREE / PREMIUM is a visibility tag**, stored locally in `panel_client_tags`,
   **never written to the panel**. It is not an account tier. **It is a business
   distinction, not a technical one:** a FREE VPN and a PREMIUM VPN are the *same
   kind of object* on the panel — an identical 3X-UI client. The tag exists so the
   platform can do two different jobs with the same thing: sell PREMIUM for
   revenue, and give FREE away to acquire customers. **The Super Admin is the only
   role that may set or change the tag** (`PermOfferManage`, `PUT
   /api/panel-clients/{server_id}/{email}/tag`), and the tag **locks** once the VPN
   is issued, so nobody can re-price what a customer already has.
4. **`vpn_instances` is a mirror of a confirmed panel client**, never an intent row.
5. **Release removes the VPN from Veylix only.** It never deletes a panel client.
6. **Quota is counted inside the transaction.** Two concurrent requests can never
   exceed a reseller's quota, nor create a duplicate panel client.
7. **A refused panel write leaves no VPN row behind** (D-024.1a). There is no
   phantom VPN.
8. **A local transaction never spans a remote call.** Remote effects are
   reconciled through the `outbox` plus a fresh read.
9. **The panel email *is* the VPN's name, and the customer must be able to read
   it.** A 3X-UI client's `email` is derived from the human who owns it:
   `<client account email>_<kind>_<n>` — `nimal@gmail.com_free_1`,
   `nimal@gmail.com_paid_2` (D-034.1). `<n>` is a per-(client, server, kind) counter
   that **only moves forward**: a revoked `_paid_2` is never re-issued, so the next
   paid VPN is `_paid_3` (D-034.2). `panel_sub_id` stays `vyx-<vpn_instance_id>` so
   D-012's idempotency survives the human-readable name (D-034.3). An opaque
   `vyx-21` on the panel is a defect, not a name.
   **The client surface must show it.** Any screen that lists a VPN the customer
   owns renders the panel email in a form a customer can read *and* copy — a raw
   `_paid_1` suffix alone is not readable, and a package name alone leaves the
   customer unable to identify their own client on the panel when they contact
   support. Both facts are shown: the package it came from **and** the panel
   identity. (**C** backend: `domain.PanelBase`, `PanelFindNextFreeSlot`,
   `PanelSlotNext`. **Bug** frontend: `MyVpnsPage.tsx` renders `vpnDisplayName`
   (the package name) and **never** `panelEmail`, so the customer cannot see the
   identity that D-034 defines — see `DEV-014`.)
10. **A panel client that has gone missing is shown on the VPN itself, prominently,
    and is never hidden.** If a client Veylix mirrors is no longer on its panel the
    VPN becomes `SERVER_UNAVAILABLE` — never `REVOKED` and never deleted (D-024.5,
    INV-1). That state must be **visible on the VPN's own row and on its detail
    page**, with a sentence a customer can act on, because it is the difference
    between "your VPN is gone" and "Veylix cannot see it right now". Hiding it, or
    rendering it as an ordinary grey status chip, tells the customer nothing.
    **C:** `VPNDetailPage.tsx` explains all nine statuses in `NOT_CONNECTABLE` and
    deliberately gives an **unknown** status a generic sentence rather than an
    invented one. **Bug** `MyVpnsPage.tsx`: the list carries only a `StatusBadge`,
    so a customer scanning their VPNs sees no explanation of
    `SERVER_UNAVAILABLE` at all — see `DEV-015`.
11. **A screen's layout follows one hierarchy, and a heading appears once.** A page
    groups its content into named sections using a single pattern — either every
    group is an `<h2>` section, or every group is a titled `Card`; not both mixed
    in one column. The same word is never used for two different levels at once
    (`<h2>Connection</h2>` immediately above `<Card title="Connection">` reads as a
    rendering fault). Shared layout is expressed as a class in the section's own
    stylesheet, not as an inline `style={{ … }}`, because inline styles are what
    make two screens that should match drift apart (D-059).
    **Bug** `VPNDetailPage.tsx`: sections are three `<h2>`s plus three `<Card>`
    titles, "Connection" is duplicated at two levels, and 23 inline styles remain
    after D-059 — see `DEV-016`.

---


## 4. The three operational invariants

**INV-1 · Honest failure.** An unreachable panel produces an explicit pending
state (`REVOCATION_PENDING`, `SERVER_UNAVAILABLE`) — never a fabricated `REVOKED`,
never a fabricated success. **C:** `provisioning.pushCreate` sets
`Recorded=false` and the route answers 502 `panel_rejected` when the panel
positively reports the client absent.

**INV-2 · Delivery is never faked.** With no SMTP relay the response and the audit
row say `outbox`; nothing claims an e-mail went out. **C:** `auth_routes.go`
returns the literal string `outbox`.

**INV-3 · The audit trail is append-only.** Every privileged action — credential
changes, provisioning and revocation, quota and price edits, payment confirmation,
role changes — is recorded. **C:** `internal/audit`, table `audit_logs`.

---

## 5. The lifecycle of a VPN

```
offer (Free|Premium)  ──claim/buy──▶  provisioning.Create
                                          │
                        ┌─────────────────┴──────────────────┐
                        │ 1 local tx: vpn row + assignment    │
                        │    + quota slot + FREE claim        │
                        │    + outbox entry                   │
                        └─────────────────┬──────────────────┘
                                          │
                                   panel AddClient
                                          │
                            ┌─────────────┴─────────────┐
                     fresh read: exists          fresh read: absent
                            │                           │
                       ACTIVE (confirmed)          discard rows, 502

ACTIVE ──suspend──▶ SUSPENDED ──resume──▶ ACTIVE
ACTIVE ──revoke──▶ REVOKED          (panel delete, confirmed)
ACTIVE ──release─▶ released         (Veylix only; panel untouched)
ACTIVE ──transfer▶ other client     (ownership moves)
```

**C:** this flow is `provisioning/service.go`, covered by
`TestCreateSendsThePanelsInboundNumber`, `TestPanelRejectionLeavesNoVPNRow`,
`TestUnreachablePanelIsUnavailableNotRevoked`, and 30+ peers.

---

## 6. The FREE claim caps

Two independent caps, both checked and consumed **in the same transaction** as the
VPN row (**C:** `store.RecordFreeClaim`):

| Cap | Meaning |
|---|---|
| **Concurrent** | how many FREE VPNs one client may hold at once |
| **Monthly** | how many claims in a rolling 30-day window |

Releasing a FREE VPN does **not** refund the monthly slot. Both caps are per
server. The UI must show the remaining allowance **before** a claim.

---

## 7. Pricing and payment

- A price lives **only** in the catalogue (`vpn_offers.price_cents`). The panel has
  no notion of price. **The buyer never types an amount.**
- Money is integer **minor units** plus a currency code. No floating point.
- A PREMIUM plan the platform owns can only be released by a **Super Admin
  confirmation of an order** (payment path). A reseller may release **their own**
  plan on **their own assigned server** with no approval step.
- `Approved` is set only by the payment path. The HTTP provisioning route never
  sets it, so an unowned premium plan cannot be reached from outside.

---

## 8. Authentication

Two doors, **decided by the server** (**C:** `auth.Portal`, 403 `wrong_portal`):

| Door | Path | Who may pass |
|---|---|---|
| Client | `/login` | `client` only |
| Staff | `/staff/login` | `admin`, `super_admin` |

Passwords are **argon2id**. Sessions are **server-side rows**, never a client-
supplied identity. A missing `portal` field defaults to `client`; it can never
unlock staff.

---

## 9. Out of scope

`./veylix-ui demo/` is a **separate application** with its own `package.json`,
`src/`, and documentation tree. It is not Veylix and is not covered by this
specification.

---

## 10. Where the detail lives

One home per fact. This file states the rules; these files carry the depth:

| Question | File |
|---|---|
| How are the layers split? | `docs/new/ARCHITECTURE.md` |
| What is the schema? | `docs/new/DATABASE.md` |
| Which endpoint exists, for which role? | `docs/new/API_SPEC.md` |
| What was decided, and why? | `docs/new/DECISION_LOG.md` |
| Which defect was fixed, and how? | `docs/new/DEVELOPMENT_LEDGER.md` |
| What was actually executed? | `docs/new/QA_TEST_REPORT.md` |
| How does data move to the new system? | `docs/new/MIGRATION_PLAN.md` |
| What is confirmed / assumed / unknown? | `docs/new/EVIDENCE_INDEX.md` |
| What was lost? | `docs/restart/KNOWN_LOSSES.md` |
| What is unresolved? | `docs/restart/UNKNOWNS.md` |
| Phase 2 audit | `docs/restart/PHASE_2_AUDIT.md` |
| How do I work on this repo? | `docs/new/DEVELOPMENT_BASELINE.md` |

