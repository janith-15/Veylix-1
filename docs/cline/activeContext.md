# VEYLIX — ACTIVE CONTEXT

**Updated:** 2026-11-10 · **Mode:** Act · **Last shipped:** D-118 (**v1.0.0 tagged**
— fresh installs proven to carry **no dev data** and an **empty** database; two
defects fixed: the startup log advertised the dev seeder, and the missing
`.gitattributes` made `install.sh` CRLF and unrunnable on Linux) ·
**Earlier:** D-117 (every operator list sorts/searches/filters), D-116 (orders +
cap stated), D-115, D-114 (D12 closed)

**Install status:** commit `2dbbeee`, tag `v1.0.0`. **No git remote exists yet**,
so nothing is pushed and the install URL does not resolve — the owner must create
the repository. `go test -race` is BLOCKED (no C compiler) and no Linux run has
ever happened; both are reported as blocked, never as pass.


sorted, filtered and searched** — `Placed by`/`Package`/`Reference` were not
sortable at all, the sorts that existed were silent no-ops, and there was no
filter or search anywhere; also the 200-row cap is now stated) ·
**Earlier:** D-115 (gates fixed), D-114 (D12 closed), D-113, D-112

## D12 is closed — this is the state that decides "is v1 done"

Live evidence against **3X-UI 3.11.0 / Xray 26.10.21** on server 11:

```
POST /api/servers/11/test          → 200 ACTIVE
GET  /api/servers/11/panel-clients → 200, 13 real clients
order #17 → POST /api/payments/17/confirm → 200, vpn_instance_id 55
panel holds  client@veylix.test_paid_1 · sub_id vyx-55 · enable true
GET  /api/vpns/55/links            → 200, source "panel", real vless:// URI
POST /api/vpns/55/suspend          → 204, panel reported enable: false
POST /api/vpns/55/revoke           → 200 confirmed:true (panel client deleted)
```

The direct create route is correctly locked: FREE → `409 free_concurrent_limit`,
platform PREMIUM → `403 plan_not_approved`. The legitimate path is **order →
confirm**. Probe data removed via revoke + purge; panel back to 12 clients.

## What D-114 changed

`SetClientEnabled` (`panel/ops.go`) trusted HTTP 200 + `success:true` and never
read the client back, so a bulk call that changed nothing produced a **204** and a
local `SUSPENDED` the panel never entered. It now confirms with a fresh
single-client read. **Confirm against the single read, never the paged list** —
the two panel views can disagree for a moment after a write.

`TestSetClientEnabledConfirmsThePanelActuallyChanged` failed before the fix and
passes after. **`SetClientEnabled` had zero tests before this.**

## Gates — last run, all green

```
backend:  gofmt clean · go build ./... · go vet ./... · go test ./... 17/17 packages
frontend: lint 0 errors (5 pre-existing warnings) · build OK · audit OK
          audit:selftest OK (8 positive + 6 negative) · docs:verify OK
preflight: PREFLIGHT OK — 33 checks passed
browser:  verify:vpn          13/13  (×3 consecutive)
          verify:super-admin 136/136 (cold start, and ×3)
          verify:vpn-replace   7/7
```

`go test -race` is **BLOCKED** (no C compiler) — report as blocked, never PASS.

## What D-115 changed about the gates

The three browser gates were not noisy — they were **wrong**, and had been red
long enough that red was expected.

- `verify:vpn` asserted testids a rebuild had deleted. Two of its checks were
  wrong in *intent*: P-08 demanded charts on a **FREE** VPN (Premium-only by
  design), P-03 demanded CPU/RAM on a page that never renders them. Both deleted,
  not softened. Now 13/13.
- `verify:super-admin` used `sleep(3500)` — the pattern D-109 removed elsewhere.
  Now polls each page's declared heading, and asserts it settled *without* a
  fixed sleep. 136/136 cold.
- `verify:vpn-replace` did not exist for D-112. Added, read-only (stops at the
  confirm dialog and cancels; refuses with exit 3 on an empty account). **It found
  a real defect:** the dialog read as "creates a new VPN *and* releases the
  current one" — simultaneous — and never stated D-112's safety property. Fixed.

## What D-116 changed on the Orders screen

`Table` gained `sortValue` (sort by a declared field, not the display key),
`searchValue` (what the global box matches), `filter` (a text or select control
per column), a toolbar and a stated count. `OrdersPage` uses all four.

The root defect was that `Table` sorted `row[column.key]`; a column keyed
`placedBy` has no such field, so **every sort on such a column compared
`undefined`** — the arrow moved, the rows did not. That is why the screen
"could not be sorted" even where a sort button existed.

```
search  "client@veylix.test" → 1 of 9      filter  status=CONFIRMED → 6 of 9
search  "confirmed"          → 6 of 9      filter  Placed by "janith14" → 3 of 9
sort    six columns, asc ≠ desc proven by the gate
```

The 200-row cap is no longer silent: `GET /api/payments?envelope=1` carries
`{shown, total, truncated}`, and the page says *"Showing the newest N of M
orders"*. `total: -1` means the count failed — never rendered as zero.

## Open — what is left before v1 can be called done

1. **Panel list vs get can disagree transiently.** Writes are confirmed via the
   single read (D-114), but the operator-facing `enable` column comes from the
   paged list and can lag.
2. **Acting on a `CONFIRMED` order as Super Admin** (cancel / re-target / revoke
   after the money) — needs an owner decision on refunds and quota.
3. **The same filter/search treatment is not yet on other tables.** Clients,
   VPNs and payments-methods still have sort-only tables. D-116 made it possible;
   applying it is a separate pass.
4. **`verify:orders` SKIPs the truncation check** while the account holds fewer
   than 200 orders — it is honest, not vacuous, but the capped state is proven in
   Go (`TestPaymentsEnvelopeStatesTruncation`), not in the browser.
5. **D10** (no frontend unit tests), **D11** (CI never executed here).
6. `go test -race` is **BLOCKED** (no C compiler).

## Environment notes

- Server 11 is **live**: `https://difanx4f3.difanx1.ggff.net:2083/4pNsFUtRtpTIsgdvUz`
  — 3X-UI 3.11.0. Real writes are now safe to test.
- Client VPN list is **`/client/vpns`** (`/my-vpns` redirects to the dashboard).
- `/vpn/:id` is **client-only**; staff use `/vpns/:serverId/:panelEmail`.
- `GET /api/servers/{id}/panel-clients` is **Super-Admin only** and reads live.
- **Reading a panel list twice in a row can differ** — always settle before
  concluding. This cost me two false findings today.

## Previous: D-113 (orders name the payer + package)


The Super Admin could not see **who placed an order** or **which package** it
bought, because `paymentColumns` shipped `payer_user_id` and `offer_id` —
numbers, not names. The row now carries `payer_email`, `target_email`,
`offer_name`, `offer_kind`.

| Surface | Where |
|---|---|
| Store | `backend/internal/store/payments.go` — `paymentColumns`, `PaymentRow`, `scanPayment` |
| Wire | `frontend/src/lib/api.ts` — `ApiPayment` |
| Client model | `frontend/src/stores/paymentsStore.ts` — `Payment`, `toRow` |
| List | `frontend/src/pages/OrdersPage.tsx` — **Order → Placed by → Package** |
| Detail | `frontend/src/pages/OrderDetailPage.tsx` — `Placed by`, `For`, `Package` |
| Styles | `frontend/src/pages/orders.css` — `.ord-who`, `.ord-package` |
| Tests | `backend/internal/store/payments_identity_test.go` — 2 |
| Decisions | `docs/DECISIONS.md` — **D-113** |

Two rules from it that are easy to break:

1. **Scalar subqueries, not joins.** `vpn_offers` joined in could multiply rows
   and silently change the page size. The file already did it this way.
2. **Absence is stated in words.** A manual record reads *"no package (manual
   record)"*, a vanished account *"account no longer exists"*. Never a blank cell.

## The ordering, because it is the whole safety argument (D-112)

**Create the new VPN → confirm it on the panel → only then release the old one.**
`TestReplaceCreatesTheNewVPNBeforeReleasingTheOld` holds this: on a panel refusal
the release function is called zero times. `ReplaceFreeClaim` takes the caller's
`*sql.Tx` (a second transaction deadlocks SQLite) and **swaps** the concurrent
slot (at the cap an ordinary claim would always be refused).

## Gates — last run, all green

```
backend:  gofmt clean · go build ./... · go vet ./... · go test ./... 17/17 packages
frontend: lint 0 errors (4 pre-existing warnings, files untouched) · build OK
          audit OK · audit:selftest OK · docs:verify OK (698 tests · 143 routes)
live:     GET /api/payments -> payer_email + offer_name + offer_kind populated,
          #11 (manual record) -> offer_name "" and rendered "no package (manual record)"
          POST /api/vpns/42/replace -> 201, free_1 -> free_4, old RELEASED
```

`go test -race` is **BLOCKED** (no C compiler) — report it as blocked, never PASS.

## Open, and what to do next

1. **Acting on a `CONFIRMED` order as the Super Admin.** A release target can
   only be set while `PENDING`/`AWAITING_CONFIRMATION`. Cancel / re-target /
   revoke after confirmation involves refunds and quota — needs its own decision
   (D-113 deliberately left it out).
2. **D12 real-panel provisioning — the biggest gap.** Replace's logic is proven;
   the actual panel write is not.
3. **Panel health classification.** `/api/servers` reports
   `CONNECTION_UNAVAILABLE` while `/api/sync/status` reports `ACTIVE`.
4. **`/api/payments` is capped at `LIMIT 200`** with no indication that older
   orders exist. A Super Admin with a large history silently stops seeing them.
5. **`verify:vpn`** is red on stale selectors (`vpn-capacity`,
   `vpn-live-traffic`, `vpn-metrics` gone after the tab rebuild).
6. **No `verify-*.mjs` gate** for Replace or for the orders list — the browser
   proof for both was Playwright (exploratory, not a gate).

## Environment notes

- Client VPN list is **`/client/vpns`**, not `/my-vpns` (redirects to dashboard).
- Orders live at **`/orders`**; detail at **`/orders/{id}`**.
- `/api/vpn-releases` is scoped to the caller — sign in as the owner.
- In the Vite dev proxy an unauthenticated accessor gets **404**, not 401, on
  several routes (`/api/me` too). Pre-existing.

## Previous: D-112 (Replace VPN)


A **Replace** action: one request builds a **new** VPN and releases the old one.
The customer gets a genuinely different panel client (`_free_1` → `_free_4`), the
old one goes to its 3-day undo window, and a FREE replace costs one monthly claim
(§17.18).

| Surface | Where |
|---|---|
| Service | `backend/internal/provisioning/replace.go` — `Replace()` |
| Claim swap | `backend/internal/store/claims.go` — `ReplaceFreeClaim()` |
| Route | `backend/internal/httpapi/replace_routes.go` — `POST /api/vpns/{id}/replace` |
| Client API | `frontend/src/lib/api.ts` — `replaceVpn()`, `ApiVpnReplace` |
| UI | `frontend/src/pages/client/MyVpnsPage.tsx` — action + confirm + outcome |
| Tests | `backend/internal/provisioning/replace_test.go` — 7 |
| Decisions | `docs/DECISIONS.md` — **D-112** (read it before touching this) |
| Ledger | `docs/new/DEVELOPMENT_LEDGER.md` — DEV-039 |

## The ordering, because it is the whole safety argument

**Create the new VPN → confirm it on the panel → only then release the old one.**
Releasing first and failing to create leaves a customer with zero usable VPNs and
no undo — worse than doing nothing.
`TestReplaceCreatesTheNewVPNBeforeReleasingTheOld` holds this: on a panel refusal
the release function is called zero times.

Two consequences that are easy to break:

1. **`ReplaceFreeClaim` takes the caller's `*sql.Tx`.** `Create` already holds a
   transaction; opening a second deadlocks SQLite (`SQLITE_BUSY`). That is why
   `RecordFreeClaim` has the same parameter.
2. **The concurrent cap is swapped, not added.** A replace does not increase the
   number of live VPNs, so it takes over the replaced claim's slot. Without this a
   client holding 3 FREE VPNs — the cap — could never replace anything. The
   monthly cap still applies.

## Gates — last run, all green

```
backend:  gofmt clean · go build ./... · go vet ./... · go test ./... 17/17 packages
frontend: lint 0 errors (4 pre-existing warnings, files untouched) · build OK
          audit OK · docs:verify OK (696 tests · 143 routes)
live:     POST /api/vpns/42/replace -> 201, free_1 -> free_4, old RELEASED,
          claim_consumed true, claims_remaining 1
```

`go test -race` is **BLOCKED** (no C compiler) — report it as blocked, never PASS.

## Open, and what to do next

1. **D12 real-panel provisioning — the biggest gap.** Replace's logic is proven;
   the actual panel write is not, and neither fixes nor worsens D12. Start there.
2. **Panel health classification.** `/api/servers` reports
   `CONNECTION_UNAVAILABLE` while `/api/sync/status` reports `ACTIVE` for the same
   panel (login page 200, `/panel/api/...` 404). Suspected classification bug.
3. **`verify:vpn`** is red on stale selectors (`vpn-capacity`, `vpn-live-traffic`,
   `vpn-metrics` disappeared in the tab rebuild).
4. **No `verify:vpn-replace.mjs` gate yet.** The browser proof this session was
   Playwright (exploratory, not a gate). Commit a deterministic `verify-*.mjs` if
   this surface is to be regression-covered — per the browser test boundary in
   `06-tooling-and-routing.md`.
5. **Orphan panel clients.** A released-then-purged VPN leaves its panel client
   (D-027.3). Pre-existing, tracked separately.

## Environment notes learned this session

- The client VPN list is at **`/client/vpns`**, not `/my-vpns` (that redirects to
  the dashboard).
- `/api/vpn-releases` is **scoped to the caller** — sign in as the owner to see
  their releases.
- In the Vite dev proxy an unauthenticated accessor gets **404**, not 401, for
  several routes (`/api/me` does too). Pre-existing, not a defect in the new route.

## Previous: D-111 (package creation worked but the list said "no server")


---

## Where the project is

**Green.** 19/19 browser gates pass (18 previous + `verify:offer-create`, which
covers **offers and packages** in one run), 17/17 backend packages pass, 0 FAIL.

`npm run verify:vpn` **still exits 1**: it asserts `vpn-capacity`,
`vpn-live-traffic` and `vpn-metrics`, which **no longer exist** — the detail page
was rebuilt into tabs (`vd-tab-overview|usage|connect`, `vuc-*`, `vui-*`). The
data is present; the selectors are stale. It is not in the gate set for that
reason. Re-check before any release claim.

## What changed in session 32 — the short version

| Before | After |
|---|---|
| **Offer** create failed on every attempt: `422 inbound_mismatch` with the inbound visibly listed for that server | `201 Created`; proven live and by `verify:offer-create` 5/5 (D-110) |
| `GET /api/servers/{id}/inbounds` answered with the **panel's** number (`1/2/3`) | Answers the **local `inbounds.id`** (`52/53/54`) — the id `vpn_offers.inbound_id` accepts |
| Offer dropdown read **"Let the panel decide…"** — a capability Veylix does not have | Reads **"Choose an inbound…"**; the form refuses locally in product words |
| **Package** create worked (`201`) but **every** row rendered **"no server"** | `GET /api/plans` answers the declared `ApiPlan` shape; rows read **`server 11`** (D-111) |

## Offer vs Package — the two words, settled

- **Offer** (`/api/offers`, "VPN offers") — a catalogue entry a client **claims**
  (FREE) or **buys** (PREMIUM). Each claim builds its own panel client.
- **Package** (`/api/plans`, "Packages") — a **priced plan template**: kind,
  traffic, duration, devices, price, currency, slots, and the server + inbound it
  builds on.
- Both live in `vpn_offers`, told apart by `kind` / `price_cents` / `is_custom`.
  That shared storage is why the two screens can disagree about one row.

## ⚠️ The open defect that will bite next

`/api/servers` reports **`CONNECTION_UNAVAILABLE`** for server 11 while
`/api/sync/status` reports **`ACTIVE`** and a sync 59 seconds old — for the *same
panel*. Every VPN detail page therefore reads `SERVER_UNAVAILABLE` and hides its
connection links. The panel itself serves its login page (HTTP 200) but every
`/panel/api/...` path answers **404** from here. Treat the health classification
as **suspect**, not as proven fact — but do **not** "fix" it by making a failed
read look healthy. D-110 and D-111 do not touch this; it is a separate finding.

## The facts most likely to be forgotten

1. **There are two inbound id spaces, and they are not interchangeable.**
   `vpn_offers.inbound_id` → `inbounds(id)` is a **local AUTOINCREMENT row id**
   (`52/53/54`). The panel numbers its own inbounds independently (`1/2/3`) and
   that number lives in `inbounds.remote_id`. `provisioning.panelInboundID` is the
   **only** translator. Any new endpoint that hands an inbound id to a caller must
   hand out the **local** one (D-110).
2. **A declared response shape is a promise the server must keep.** `ApiPlan` in
   `lib/api.ts` declares `server_label`, `slots_remaining` and `purchasable` as
   required, but they are **derived** — not columns on `store.Plan`. A handler that
   serialises the store row blanks them, and the screen invents a fallback like
   **"no server"** on correct data. TypeScript cannot catch this: it checks the
   frontend against itself, never against the wire. Any handler returning a storage
   row to a screen is suspect (D-111).
3. **`offer_id` is what makes a payment an order.** Without it the row is a deposit:
   `payments.Service.Confirm` refuses it with `ErrNoPlan`, so no VPN is ever built.
   `BuyPremiumPage` must always send it, and `verify:order-chain` fails if it does not.
4. **`payment_routes.go` must never let a plan order reach the settlement-only branch.**
   That branch is for deposits. A plan that settles there is money without a product —
   the exact defect D-108 records.
5. **Currencies are never summed.** When `mixed_currency` is true the server answers `0`
   for every amount **on purpose** and the UI must branch on the flag. Rendering that `0`
   is a lie; summing USD and LKR is a worse one.
6. **A missing section is not a zero.** `errors[]` names the sections that failed; a
   failed read must never render as "you have sold nothing".
7. **Revenue is NET**, and the month is a CALENDAR month (D-107): a refunded payment is
   money that went back out, and a rolling 30-day window silently changes meaning daily.
8. **A gate polls for the outcome.** `verify:offer-create` first passed 2 runs in 3
   because it slept 1200 ms and read the inbound dropdown while it still said
   "Reading the panel…". It now polls until the option settles. Never replace a
   poll with a sleep (D-109, re-learned in D-110).

## Gates

```
backend/   gofmt -l .   go build ./...   go vet ./...   go test ./... -count=1
frontend/  npm run lint  build  audit  audit:selftest  audit:css
           audit:csv-selftest  docs:verify
           verify:super-admin  verify:nav  verify:clients  verify:client-vpns
           verify:offer-create   ← D-110 + D-111, covers offers and packages
whole app  .\dev.ps1 -Status    .\dev.ps1 -Check
```

`verify:offer-create` creates a probe **offer** and a probe **package**, asserts
both return `201`, asserts the package row names its server and is not
"no server", asserts the dropdown prompt, then **deletes both** and asserts both
catalogues are back to their starting size. **9 checks, 5/5 consecutive passes,
zero residue.**

`verify:super-admin` is **122/0** and now has 10 **rendered-text** assertions for the
business row, so it fails if a section disappears or developer vocabulary returns. It was
proven to bite (reverting `Panel health` → `Healthy` produced exit 1).

`go test -race` is **BLOCKED** (no C compiler). Never report it PASS.
`verify:ui` is **report-only** — a DOM dump with no pass/fail line, so not a gate.

## Open, honestly

- **Performance (QA area 9) is PARTIAL.** `EXPLAIN QUERY PLAN` verified; **no wall-clock
  benchmark was run**, so no timing claim is made.
- **No component unit tests** for `MoneyHero` / `ResellerSection` (D10 — no runner). The
  page-level browser gate is the proof.
- **`resellers.active` means "signed in within 30 days"**, not "has a subscription" — the
  UI says *signed in recently* so the two cannot be confused.
- **A purge has no undo; `Dropdown`'s portal is invisible to the inline-style gate**
  (carried over from session 28).

## If you are picking this up

1. `docs/DECISIONS.md` **D-107** before touching money, quota or the overview route.
2. `docs/new/DEVELOPMENT_LEDGER.md` **DEV-034** for the full evidence table.
3. Run `.\dev.ps1 -Check` and `npm run verify:super-admin` before believing this file.

---

## Older context (session 28, retained)

D-102 … D-106 (Super Admin perfection): one owner enforced by a DB index, forced release
with a reason, purge that keeps the panel client, offer archive/delete, 373 → 0 inline
styles. See **DEV-033**.


---

## Where the project is

**Green.** Every gate passes. The Super Admin console was audited end to end, ten
defects were fixed behind failing tests, six decisions were recorded, and the
temporary plan file was deleted after every phase was proven.

## What changed in session 28 — the short version

| Area | Before | After |
|---|---|---|
| Super Admin accounts | any number could exist | **exactly one**, enforced by a DB index |
| Releasing a VPN | Super Admin → 403 | **allowed with a mandatory audited reason** |
| Retiring a VPN record | impossible while the panel held the client | **purge keeps the panel client, needs type-to-confirm** |
| Offers | no archive, no delete | **archive + guarded delete** |
| Clients pager | divided by the page **number** | divided by the page **size** |
| CSV export | formula injection possible | `=` `+` `-` `@` neutralised, 14 tests |
| Panel dialling | DNS-rebinding window | `Dialer.Control` + explicit IMDS list |
| Audit log | not append-only in the DB | `BEFORE UPDATE`/`BEFORE DELETE` triggers |
| Console inline styles | **373** | **0**, gated by a browser probe |

## The four facts most likely to be forgotten

1. **`Table` is where the inline styles came from.** It wrote a `textAlign` style prop
   on every cell of every table in the application. It now writes `data-align` /
   `data-width` attributes and `ui.css` owns the rule. **Do not put a style prop back
   into a shared component** — `verify:super-admin` will fail, which is the point.
2. **A forced release and a forced purge are different events** with their own audit
   actions (`vpn.force_release`, `vpn.force_purge`). Do not collapse them into a flag
   — the distinction is what someone asks about months later.
3. **`errors.As(err, new(*T))` inside a `switch` case does not give you the value.**
   It matches, then discards. Use a named helper (`isPurgeConfirmError`). This cost a
   500-instead-of-422 defect.
4. **`vpn_instances.offer_id` and `payment_records.offer_id` are both
   `ON DELETE SET NULL`.** A bare `DELETE` from `vpn_offers` will NOT fail — it will
   silently erase which product a customer paid for. `DeleteOffer` checks them
   explicitly; keep that check if the schema changes.

## Gates

```
backend/   gofmt -l .    go build ./...    go vet ./...    go test ./... -count=1
frontend/  npm run lint  build  audit  audit:selftest  audit:css
           audit:csv-selftest  docs:verify
           verify:super-admin  verify:nav  verify:clients  verify:client-vpns
whole app  .\dev.ps1 -Status    .\dev.ps1 -Check
```

`verify:super-admin` is the console's regression proof: all 14 pages, heading ·
no exception · controls present · **zero inline styles** · no overflow · no 1970 date.

`go test -race` is **BLOCKED** on this machine (no C compiler). Never report it PASS.
`verify:ui` is **report-only** — a DOM dump with no pass/fail line, so not a gate.

## Open, honestly

- **Performance (QA area 9) is NOT RUN.** No benchmark moved this session.
- **The Super Admin handover is untested end to end.** A Super Admin cannot demote
  themselves and there is deliberately no second one, so the demote-then-promote
  sequence has no test that performs a full handover. Recorded as residual in
  `DEVELOPMENT_LEDGER.md` DEV-033.
- **A purge has no undo.** The panel client survives; the record does not. Deliberate
  — the type-to-confirm is the guard, not an undo.
- **`Dropdown`'s portal is invisible to the inline-style gate.** Its menu is portalled
  to `<body>` and only exists while open, so `verify:super-admin` never measures it. It
  was converted to custom properties by hand (D-106) and will not be caught if it
  regresses; a gate for it needs an assertion that opens a menu.

## If you are picking this up

1. Read `docs/new/DEVELOPMENT_LEDGER.md` **DEV-033** — the whole session with evidence,
   including the three defects found only by measuring.
2. Read `docs/DECISIONS.md` **D-102 … D-106** before touching auth, releases,
   provisioning or offers.
3. Run `.\dev.ps1 -Check` and `npm run verify:super-admin` before believing anything
   in this file.

---

## Older context (session 27, retained)

**Last shipped before this session:** D-101 (sidebar build stamp) on top of D-100
(not-in-service section) over D-098/D-099.

## What is true right now

**D-100 — a VPN that is not in service is shown in its own section, and the
ordering bug behind the owner's complaint is fixed.**

The owner reported that a suspended VPN "appears at the top of My VPNs" and "I
cannot use it anyway". That was a real defect, not a preference: the sort had
exactly one outer comparison (dormancy), so under the default **Newest first** a
recently-created SUSPENDED/REVOKED row floated to the very top.

- **`STATUS_RANK` + `rankOf`** now make status outrank the chosen key: ACTIVE →
  needs-attention → customer-recoverable → not-in-service → dormant. The chosen
  sort still applies **in full inside** each rank, so it is grouped, never
  replaced.
- **A "Not in service" section** holds SUSPENDED / REVOKED / RELEASED /
  REMOVED_FROM_CATALOG with a sentence stating the rows are still theirs and still
  on file. Each still carries its status sentence and its panel identity
  (D-091.4/.5). **Nothing is hidden and nothing is deleted.**
- **`status_changed_at` is now published on `GET /api/vpns`** (D-098's column,
  previously internal), so a row can say *since when*. `null`/`0` renders
  **nothing** — printing `0` would show "1 January 1970".
- **The usage chart is labelled honestly.** Snapshots are cumulative counters, so
  readings stopping makes the line flat, and a flat line reads as "zero usage".
  `VpnUsageSection` now says *"Counters stopped on ⟨date⟩ — the flat line after
  that point is an absence of readings, not zero usage"*, and only when the newest
  reading genuinely predates the status change. **No backfill, no notification.**

**D-101 — the sidebar footer carries the build stamp.** `v<version>` read from
`GET /api/system/version` (the RUNNING binary, not the bundle) plus a source link.
The repo URL is a **build-time constant** in `vite.config.ts`
(`__VEYLIX_REPO_URL__`, default `https://github.com/janith-15/Veylix-1`), **not** a
`.env` value — `frontend/.env` is git-ignored, so a value living only there would
vanish in every deployment built from the repo. Unreadable version → no line.
Empty URL → no link. Never a placeholder. `target="_blank" rel="noopener
noreferrer"`.

### Two answers that settled this pass

1. **`REVOKED` stays terminal.** No `REVOKED → ACTIVE` transition was added.
   Resuming a revoked VPN is a new VPN, not a state change.
2. **The Overview counts still count every VPN** (D-088). "Total VPNs" describes
   the account, not a section.

---

## What is NOT proven — read this before claiming D-100 is finished

The seeded client owns three VPNs that **never changed status**. Therefore:

- the **"Not in service" section has no live row to render** on this machine, and
- the **`since` label and the usage caption have no live row either**.

Their code paths are proven by:
- `TestVpnListPublishesStatusChangedAt` (the API field, both the set and null
  cases), and
- `verify:client-vpns`'s **negative** assertions — a clean account renders **no**
  empty section, and **no** 1970 date appears anywhere.

They are **not** proven by a screenshot. Producing a live suspended row would mean
mutating the seeded panel client, which the data-safety rule forbids. This is
recorded as `PARTIAL` in **DEV-032**, deliberately, rather than rounded up.

---

## Gates as last run (2026-11-09)

| Gate | Result |
|---|---|
| `gofmt -l .` · `go build ./...` · `go vet ./...` | clean, no output |
| `go test ./... -count=1` | **17/17 packages ok**, **655** test functions |
| `TestVpnListPublishesStatusChangedAt` | **PASS** (new, D-100) |
| `npx tsc -b` · `npm run build` | clean · built |
| `npm run lint` | **3 warnings, 0 errors** — the known baseline |
| `npm run audit` · `audit:selftest` · `audit:css` | OK · OK (8+6 cases) · 55 sheets |
| `npm run docs:verify` | **FAILED first** (README 654 vs tree 655) → fixed → **PASS** |
| `.\dev.ps1 -Preflight` | **33 checks passed** |
| `verify:client-vpns` | **25 PASS / 0 failures** (8 new) |
| `verify:vpn-details` · `-dormant` · `display-timezone` · `usage-range` · `-search` · `-source` | all **PASS** |
| `go test -race` | **BLOCKED** — no C compiler on this machine |

---

## Earlier, still true

**D-099 — stored UTC, displayed in one server-chosen zone.**
`VEYLIX_DISPLAY_TIMEZONE`, default **`Asia/Colombo`**, published on `GET /api/me`
and adopted by `authStore.hydrate`. Storage unchanged; no migration. It is a
**default, not a detection** — another region must set the variable.

**D-098 — the stale-VPN lifecycle.** `status_changed_at` (migration 0026),
`StaleVPNs`, `ExpireStale(ctx, dryRun)`, `VEYLIX_LIFECYCLE_MODE` off/report/enforce.
D-100 is the first consumer of that column; **`off` remains the default mode**.

**D-096 / D-094 / D-091** — honest chart windows, dormant sorts last, panel
identity beside every VPN, every non-active state explained in a sentence.

---

## Open, needing an owner decision

- **DEV-029** — purge deletes `vpn_assignments`, so a reseller's quota reopens
  while the panel client is kept. Still awaiting a decision.
- **DEV-024** — blocked: no PREMIUM VPN can be seeded on this machine.
- **D-100 notification** — a suspended VPN is explained when the customer *looks*;
  nothing goes and tells them. If that is wanted, it is a separate decision.

---

## Hazards this repo has actually bitten us with

1. **`editor` insert/replace can orphan a brace or duplicate a line.** It happened
   twice this session (`bucketWidthLabel`, and a duplicated `<th>Usage</th>`).
   Always read the region back after editing JSX or Go blocks.
2. **Dynamic `import('/src/lib/time.ts')` in a Vite dev page returns a SEPARATE
   module instance** — it cannot be used to assert module state.
3. **`frontend/.env` is git-ignored.** Anything that must survive into a deployment
   belongs in `vite.config.ts`, not there. (See D-101.)
4. **`.\dev.ps1 -Preflight` refuses to run while :8080 is occupied** — `-Stop`
   first. There is no `-BackendOnly`; the switch is **`-Backend`**.
5. **The frontend and backend each need a restart** after a `vite.config.ts`
   change or a migration, or the new field is simply absent from the wire.
6. **A verification script once deleted live data, twice.** Read-only checks only;
   never point a test at `./var/` or the seeded panel.
7. **The seeded client VPNs are ids 42, 43, 44.** `/vpn/1` is not this client's VPN
   and renders "VPN not found" — never hardcode a low id in a test.
8. **`./veylix-ui demo/` is a different application.** Never cite it for Veylix.
9. **There is no Git repository** — review by reading the files back.


