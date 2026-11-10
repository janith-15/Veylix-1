# VEYLIX — DEVELOPMENT LEDGER

**Status:** authoritative · **Established:** 2026-11-08 · **Baseline:** `1b09553`

## DEV-041 · D12 CLOSED — real-panel provisioning proven; suspend had no confirmation (D-114)
## DEV-043 · D-116 — the order list can now be sorted, filtered and searched
## DEV-045 · v1.0.0 tagged — fresh installs carry NO dev data; installer was CRLF-broken

**The owner's question, answered with evidence (not assertion).** "Install
weddi dev data enne na needa, database eka aluthinma hedenne?"

| Method | Result |
|---|---|
| `go list -deps ./cmd/veylixd` | **no seed package** — 0 imports |
| isolated run, empty dir | `veylix.db` created (4096 B), **all tables 0 rows** |
| log scan | **0** mentions of `@veylix.test` |

Flow is `EnsureDataDir` → `store.Open` → `Migrate`. The seeders are separate
binaries (`cmd/seedacct`, `cmd/seeddemo`, `cmd/seedpremium`); `veylixd` imports
none. First account comes from `/api/setup/bootstrap`, which refuses once any
user exists. **No default password exists.**

**DEFECT — startup log advertised the dev seeder.** `main.go` said
`(seeded accounts: cmd/seeddemo)` on every production start. Now
`(first run: create the Super Admin via /api/setup/bootstrap)`.

**DEFECT — the installer would not have run on Linux.** No `.gitattributes`, and
`core.autocrlf=true` on Windows rewrote `install.sh` to CRLF → server failure
`/usr/bin/env: 'bash\r': No such file or directory`. Added
`* text=auto eol=lf` + explicit `eol=lf` for `*.sh`/`*.bash`/`*.service`/
`Caddyfile`. **Proven by bytes:** the `v1.0.0` tag's `install.sh` blob is 28015
bytes with **0 CR bytes**.

**Two measurements in this session were WRONG and are recorded as such.** Both
`git show :install.sh | Out-String` and `cat-file | Out-File` reported CRLF;
both re-add CRLF themselves. Only reading the raw blob stream is valid.

**`PLACEHOLDER_REPO` → `janith-15/veylix`**, still refused as input so a fork
must pass its own `--repo`.

**Gates:** `gofmt` clean · `go build`/`go vet` clean · `go test ./...` no FAIL ·
lint 0 errors · `docs:verify` OK · `bash -n install.sh` OK ·
`scripts/install.selftest.sh` **59 PASS / 0 FAIL** (Git Bash).
**Reported honestly, not glossed:** `go test -race` BLOCKED (no C compiler) ·
**no Linux execution happened here** — the selftest proves the guards, not the
server run. The first real install is the owner's.

**Commit `2dbbeee`, tag `v1.0.0`.** No remote is configured, so nothing is
pushed yet; the install URL does not resolve until the owner creates the
repository.


## DEV-044 · D-117 — the other three operator lists now sort, search and filter (28/28)

**Scope.** `VpnsPage`, `ClientsPage`, `PaymentsPage` gained `sortValue`,
`searchValue` and per-column `filter`; each table turned `searchable` on.

**The defect behind it.** The same one D-116 fixed, still live on three screens:
`Table` fell back to `row[column.key]`, and columns keyed `panel_email`,
`display_name`, `owner_name` had no such field, so every comparison was
`undefined` and sorting moved the arrow only. `vpn_count` / `last_login_at` /
`created_at` had no sort at all. Fixed by giving each column an explicit
`sortValue`; nothing in `Table` changed.

**Two `''` traps found while wiring, both fixed:**
- An empty `<select>` value means "no filter", so "Untagged" and "No Veylix
  record" could not be expressed. They use a `(none)` sentinel, otherwise the
  filter would silently do nothing.
- Unknown values (no expiry, never signed in, panel unreadable) sort to
  `MAX_SAFE_INTEGER`/`0` so ascending order never puts "unknown" first.

**New gate — `verify:tables.mjs` (`npm run verify:tables`), 28 checks, OK.**
Behaviour, not presence: a term narrows the rows, Clear restores the exact
count, a header click reorders the rows.

**Two harness defects caught before they were reported as product bugs:**
1. The search term was welded from a whole cell (`owner@veylix.test` +
   `for client@veylix.test` → one string). Now taken from a single **text node**.
2. The sort probe clicked one header three times; `toggleSort` cycles
   asc→desc→null, so it compared asc with asc and reported "no column
   reordered" on four correct tables. Now clicks different columns, one frame
   apart (React renders asynchronously).

**Negative control (D-115 standard).** Forcing the comparator to `return 0`
made all four sort checks FAIL; reverting made them PASS. The gate has been
observed failing.

**Also fixed in the script itself:** the first draft hung with no output — a
missing `}` kept everything inside `sleepFor`, so the file parsed as a function
body and exited silently. It now uses `profileDirUnique` + `assertPortFree` +
`waitForExit` from `chrome-profile.mjs`, which is what that module exists for.

**Gates:** `go build`/`go vet` clean · `go test ./...` 17/17 · lint 0 errors ·
`build` ok · `audit` OK · `audit:selftest` OK (8+6) · `docs:verify` OK ·
`verify:orders` OK · `verify:tables` 28/28 ×2.
**Not covered (stated, not hidden):** `AdminsPage`, `OffersPage`,
`PackagesPage`, audit and retail lists. `go test -race` remains BLOCKED.



**Requested:** *"super adminge order ha payment pages wayala sort karaganna ha
filter karaganna widihaka na. eeka hodatama hadanna hema akarayenna filter
karanna puluwan wenna one."*

Three separate defects on one screen, and the first two made the third worse.

### 1. Three columns had no sort (fixed)
`Placed by`, `Package` and `Reference` had no `sortable` flag. Now all six
columns sort.

### 2. The sorts that existed were no-ops (fixed)
`Table` sorted on `row[column.key]`. A column keyed `placedBy` has no
`placedBy` field — the payer lives in `payerEmail`. Every row compared
`undefined`, so the arrow moved and nothing changed. `Amount` had the same
shape. **`sortValue`** now declares the field, so a column may render one thing
and sort by another.

### 3. No filter, no search (added)
`Table` gained a toolbar: a global search box, a per-column filter (text or
select), a Clear button and a stated count (*"Showing 6 of 9 orders"*).

```
filter-id          Order #       filter-placedBy   Email (payer OR recipient)
filter-package     Package       filter-status     PICK ONE OF 7 STATUSES
filter-reference   Reference     search            matches every column
```

### A defect found mid-work
My first wiring made search read `sortValue`, so searching an order bought FOR
somebody returned **0 of 9** — the column sorted on the payer. Filtering and
searching are different verbs: `filter.getValue` is what one column's box
matches, `searchValue` is what the global box should find. After the split:
`client@veylix.test` → 1 of 9, `confirmed` → 6 of 9, `premium` → 6 of 9,
`17` → 1 of 9.

### 4. The 200-row cap was silent (D-116)
`LIMIT 200` with no indication. `GET /api/payments?envelope=1` now returns
`{payments, shown, total, truncated}`; the count and the list share one
**`paymentsScope`**, so a reseller is never told about rows they cannot open.
`total: -1` means "could not count" — never `0`. The bare array is untouched, so
badge counts and dashboards keep working.

### Gates
```
gofmt/build/vet clean · go test ./... 17/17 · lint 0 errors · build · audit
audit:selftest · docs:verify OK (705 tests · 143 routes) · PREFLIGHT 33/33
verify:orders  23 PASS + 1 honest SKIP (9 orders, below the cap)
```

### Files
`frontend/src/components/ui/Table.tsx` (sortValue, searchValue, filter, toolbar) ·
`ui.css` (`.ui-table__bar`) · `OrdersPage.tsx` (six sortable columns, five
filters, truncation notice) · `paymentsStore.ts` (`totalOrders`, `truncated`) ·
`lib/api.ts` (`listPaymentsPaged`) · `store/payments.go` (`paymentsScope`,
`PaymentsVisibleCount`, `PaymentListLimit`) · `payment_record_routes.go`
(`envelope=1`) · `payment_routes.go` (interface) ·
`payments_cap_test.go` + `payments_envelope_test.go` (new, 6 tests) ·
`verify-orders.mjs` (new) · `docs/API_CONTRACT.md` · `README.md`.



**This is the finding that decided "is version 1 done".** D12 had been the open
P1 since the project began: *no panel operation had ever been executed against a
real 3X-UI panel*. It is now closed with live evidence, and the run found a
genuine defect on the way.

### D12 — what was proven against the live panel (3X-UI 3.11.0, Xray 26.10.21)

| Operation | Request | Result |
|---|---|---|
| Health / auth | `POST /api/servers/11/test` | `200 ACTIVE`, panel + xray versions read |
| Client list | `GET /api/servers/11/panel-clients` | `200`, 13 real clients |
| Inbound list | `GET /api/servers/11/inbounds` | `200`, 3 inbounds (vless ×2, hysteria) |
| **Create (PREMIUM)** | order #17 → `POST /api/payments/17/confirm` | `200`, `vpn_instance_id 55` |
| **Client exists on panel** | list read | `client@veylix.test_paid_1`, `sub_id vyx-55`, `enable true`, `total_gb 536870912000`, `limit_ip 5`, `limit_hwid 5` |
| **Connection link** | `GET /api/vpns/55/links` | `200`, `source: "panel"`, real `vless://…@difanx4f3.difanx1.ggff.net:443…` |
| **Suspend** | `POST /api/vpns/55/suspend` | `204`, and the panel reported `enable: false` |
| **Revoke** | `POST /api/vpns/55/revoke` | `200 confirmed: true` — panel client really deleted |

The create went through the **legitimate path** (order → Super Admin confirm),
because the direct route is correctly locked: a FREE create answered
`409 free_concurrent_limit` and a platform-owned PREMIUM plan answered
`403 plan_not_approved`. **Both guards fired, which is itself evidence the
security model is live.**

### D-114 — the defect the run found

`SetClientEnabled` (`panel/ops.go`) issued `bulkEnable`/`bulkDisable`, checked
HTTP 200 and the envelope's `success`, and **returned without confirming the
client changed**. On the live panel a bulk call can answer
`200 {"success":true}` and leave the client alone.

Consequence: `Suspend`/`Resume` answered **204** and set the local row to
`SUSPENDED`/`ACTIVE` — a state the panel never entered. A **false success**, the
exact class of bug the project rules forbid.

**Why it survived until now:** `SetClientEnabled` had **zero tests**. No file
under `internal/panel/` referenced it.

**Fix.** The adapter now confirms every enable/disable with a fresh
single-client read (`GetClient`, `GET /clients/get`) and returns an error naming
what the panel still reports. Absent and unchanged are reported apart. The
doc comment records the live evidence and why the **paged list is not** used for
confirmation (the two panel views can disagree for a moment after a write; only
the single read is the document of record).

**Tests added** — `internal/panel/enable_confirm_test.go`:
`TestSetClientEnabledConfirmsThePanelActuallyChanged` (the exact live failure:
success envelope, client unchanged ⇒ error) and
`TestSetClientEnabledSucceedsWhenThePanelReallyDisabledIt` (the confirmation must
not break a genuine change). The first **failed before the fix and passes
after** — that is the regression proof.

### A correction I have to record

Twice during this run I read a stale panel list and nearly reported
"resume is broken" and "the panel create did nothing". Both were **my
mis-reading**, not product defects: the create had worked (12 → 13 clients once
the read settled) and resume had worked. I corrected both in the transcript
rather than leaving the wrong claim standing. The lesson is the same one D-109
already recorded: **poll for the outcome, never read once and conclude.**

### Cleanup

The probe VPN was removed through the app's own paths — revoke (panel client
deleted, `confirmed: true`) then purge (`204`). Panel back to **12 clients**, the
client's three FREE VPNs untouched. No database edits, no truncation.

**Gates:** `gofmt`/`build`/`vet` clean · `go test ./...` **17/17** · panel
package now carries 2 more tests.

### Status

`D12: CLOSED (live-verified)` · `D-114: FIXED-VERIFIED`.


## DEV-040 · An order now names who placed it and what it buys (D-113)

**Reported.** *"super admin ta oder ekak awama e oder eka wae kagenda ha oder
apu kenagena sampura wistharayak ganna widihak na"* — the Super Admin could not
see who placed an order or get its details.

**Cause — the data existed, the wire did not.** `paymentColumns` shipped
`payer_user_id` and `offer_id` (numbers). `ApiPayment` declared the same. So the
queue showed money with no owner, and the detail page showed no identity at all.

**Fixed.**

| Layer | Change |
|---|---|
| `store/payments.go` | `paymentColumns` +4 scalar subqueries: `payer_email`, `target_email`, `offer_name`, `offer_kind`; `PaymentRow` +4 fields; `scanPayment` reads them as `NullString` so a manual record cannot break the list |
| `lib/api.ts` | `ApiPayment` +4 optional fields |
| `stores/paymentsStore.ts` | `Payment` +4 non-optional fields mapped with `?? ''` |
| `OrdersPage.tsx` | **Placed by** and **Package** columns, ordered `Order → Placed by → Package` |
| `OrderDetailPage.tsx` | `Placed by`, `For` (only when it differs), `Package` facts |
| `orders.css` | `.ord-who`, `.ord-package` — in the stylesheet, no inline styles (D-059) |

**Design points worth keeping.**

- **Scalar subqueries, not joins.** `vpn_offers` joined in could multiply rows and
  silently change the page size; the file already used this pattern for
  `seller_user_id`.
- **`NullString` → `""`.** A deleted account or a manual record yields an empty
  string, which both screens **state in words** rather than rendering a blank.
- **`For` only when it differs.** Payer and target are usually the same person;
  repeating it would be noise.

**Verified live.** `GET /api/payments` → `payer_email: janith15dawan@gmail.com`,
`offer_name: "Premium 50GB Monthly"`, `offer_kind: PREMIUM`; order `#11` →
`offer_name: ""`. UI headers read `Order · Placed by · Package · Amount ·
Status · Reference · History`; `/#16` detail showed `Placed by` +
`Package`; `/#11` showed **"no package (manual record)"** on both screens.

**Gates.** `gofmt`/`build`/`vet` clean · `go test ./...` **17/17** ·
`lint` 0 errors · `build` OK · `audit` OK · `audit:selftest` OK ·
`docs:verify` OK (**698** tests · **143** routes).

**Release deliberately untouched.** The Super Admin can already release any VPN
(`releases.Release`, `forced := p.IsSuperAdmin()`), with a required reason and
its own `vpn.force_release` audit action. The remaining limit — an order can only
be given a release target while `PENDING`/`AWAITING_CONFIRMATION` — is a
money-bearing problem, not a display bug, and is left open rather than smuggled
in.

### Status

`FIXED-VERIFIED`.
`OPEN`: acting on a `CONFIRMED` order as the Super Admin (cancel / re-target /
revoke) — needs its own decision on refunds and quota.


## DEV-039 · Replace VPN — a new VPN, not the same one released (D-112)

**What the owner asked for.** *"release eka thibbata ee release karana vpn eka
neweyi ne hedenne. ekama vpn eka. release ekak thiyenne wenas wenas vpn hadala
denna needa"* — release does not produce a new VPN, it sets the same one aside;
customers need to get *different* VPNs. Confirmed as a **replace** feature, not a
redefinition of release.

**What existed.** `Release` sets `RELEASED`, keeps the panel client and opens a
3-day undo; `Restore` undoes it; a *new* VPN already came from a new claim
(`nextPanelEmail` → `_free_1`, `_free_2`, …). The missing piece was one action
doing both, so the customer is not left to release-then-reclaim by hand.

**Built.**

| Layer | Change |
|---|---|
## DEV-042 · D-115 — three gate defects fixed; `verify:vpn` went 4/13 → 13/13

The version-1 audit said the product worked but the **gates** did not. Three were
fixed, and each fix found something real underneath.

### 1. `verify:vpn` was failing a healthy page (4/13 → 13/13)

Every failing assertion targeted testids that **no longer existed** after the
detail page was rebuilt into tabs. The gate had been red so long that "red" had
become the expected state. Rewritten against the actual page:

| Check | Was | Now |
|---|---|---|
| P-09 credential masked | `vpn-protocol` (gone) | asserts **no link is rendered before reveal**, then reveals |
| P-10 QR / link | `vpn-capacity` wait + 24-char link | drives `vcn-reveal` → `vcn-toggle-0`, reads the real 468-byte link |
| P-02 capacity | inline `style="width"` | `.ui-progress__fill` via `getComputedStyle` |
| P-01 up/down/total | `.vpn-chip` | the rendered `Download / Upload / Total` text |
| P-04 live rate | `vpn-live-traffic` (gone) | a `/s` rate, or an honest "not measured" / stopped-readings sentence |
| P-06 range switch | `role="tab"` + caption | 5 range buttons exist **and** switching re-reads the body |
| P-08 charts | demanded charts on a **FREE** VPN | conditional: FREE must *say* Premium-only; PREMIUM must render all four |
| P-09 protocol | `vpn-protocol` (gone) | protocol read from the revealed link's scheme |
| P-03 CPU/RAM | polled `.ui-metric` **on the VPN page** | **removed** — those cards live on the server/dashboard pages |

Two of those were not stale selectors but **wrong test intent**:
P-08 demanded a product change (charts on FREE), and P-03 asked a page for
content it has never rendered. Both were deleted/re-scoped rather than faked.

**Stability:** 13/13 on three consecutive runs.

### 2. `verify:super-admin` — flaky by construction (fixed)

It used `await sleep(3500)` after every navigation. A fixed sleep reports the
machine's speed, not the product: it passed warm and failed cold. Replaced with a
bounded `waitFor` poll keyed on **each page's own expected heading** (already
declared in the target table), plus an explicit wait for the dashboard's business
row. Added a `settled without a fixed sleep` assertion so a regression back to
`sleep()` fails loudly.

**Proof:** passed **136/136 on a cold start** (immediately after `-Restart`) and
3/3 afterwards — the exact case that used to fail.

### 3. `verify:vpn-replace` — did not exist (added, 7 checks)

D-112 shipped with 7 Go tests and one live curl, but **no browser proof** that the
screen works. New gate, `npm run verify:vpn-replace`, read-only (it stops at the
confirm dialog and cancels — replacing a real customer VPN to satisfy a test is
the write the data-safety rules forbid).

**It immediately found a real defect:** the confirmation said *"This creates a new
VPN with fresh traffic and expiry, **and releases the current one**"* — which
reads as simultaneous, and never stated D-112's whole safety property (new is
confirmed on the panel **before** the old is released). Fixed the copy in
`MyVpnsPage.tsx`; the gate now asserts that sentence and fails without it.

It also **refuses with exit 3** on an account that owns no VPN, because a green
run there would prove nothing.

### A mistake worth recording

My first draft of `verify-vpn-replace` called `check(msg, ok, …)` — the arguments
reversed. The output printed `PASS false`, which is how it was caught. A harness
that prints `PASS false` is worse than no harness; the signature is now documented
at the definition.

**Gates:** gofmt/build/vet clean · `go test ./...` 17/17 · lint 0 errors ·
build/audit/audit:selftest/docs:verify OK · **PREFLIGHT 33/33 PASS** ·
`verify:vpn` 13/13 · `verify:super-admin` 136/136 · `verify:vpn-replace` 7/7.


| `provisioning/replace.go` | `Replace()` — authorise, resolve target, check allowance, **create the new VPN**, then release the old one — in that order |
| `store/claims.go` | `ReplaceFreeClaim()` — swaps the replaced claim's concurrent slot inside the caller's transaction |
| `httpapi/replace_routes.go` | `POST /api/vpns/{id}/replace`, `confirm` required, full error map |
| `lib/api.ts` | `replaceVpn()` + `ApiVpnReplace` |
| `MyVpnsPage.tsx` | Replace action, confirmation naming both halves + claim cost, outcome dialog |

**Two defects found by running it, not by reading it.**

1. **`502 store: free concurrent limit reached`.** The client held 3 FREE VPNs,
   so "create first" could never succeed — the slot was held by the VPN being
   replaced. Resolved without weakening either rule: the ordering stays (safety),
   and the concurrent cap is measured correctly (it limits *live VPNs*, and a
   replace does not raise that count), so the claim's slot is **swapped in the
   same transaction** instead of taking a new one. The monthly cap still applies
   (§17.18).
2. **`ErrPlanArchived` → `502`.** A VPN whose own package left the catalogue is a
   normal situation; it now answers **422 `plan_archived`** with a remedy.

**A third defect, caught by a test.** The first `ReplaceFreeClaim` opened its own
transaction while `Create` already held one — `SQLITE_BUSY`, a deadlock. It now
takes the caller's `*sql.Tx`, which is why `RecordFreeClaim` has that parameter.

**Evidence.**

- 7 tests in `internal/provisioning/replace_test.go`, including the two that
  matter most: a panel refusal calls `release` **zero** times and leaves the old
  VPN `ACTIVE`, and a client at the concurrent cap can still replace.
- **Live:** `POST /api/vpns/42/replace` → `201`,
  `new.panel_email = client@veylix.test_free_4`, `old = free_1 → RELEASED`,
  `claim_consumed: true, claims_remaining: 1`.
- **UI:** Replace renders on exactly the 3 in-service VPNs (not the released one);
  the dialog names the old identity, the 3-day undo and the claim cost.
- Probe data removed through the app's own paths — the release was **restored**
  via `/api/vpn-releases/1/restore` (200) and the probe VPN **revoked**.
- Gates: `gofmt`/`go build`/`go vet` clean · `go test ./...` **17/17** ·
  `lint` 0 errors · `build` OK · `audit` OK · `docs:verify` OK
  (**696** tests, **143** routes).

**What I did NOT prove.** The new VPN was created against a panel that currently
reports `CONNECTION_UNAVAILABLE` on `/api/servers`, so the panel-read
confirmation came from the fake/panel path as the existing tests define it — the
**live** panel write is still the D12 gap. Replace's *logic* is proven; replace's
*remote side* inherits that same open question and neither proves nor worsens it.

### Status

`FIXED-VERIFIED` for replace (service, route, UI, 7 tests, live 201).
`OPEN` (unchanged): D12 real-panel provisioning, panel-health classification,
`verify:vpn`'s stale selectors.


## DEV-038 · Package creation worked; the package list lied about it (D-111)

**What this was.** The owner reported *"package create wenne na"* and asked what a
"package" actually is. Both were answered by driving the real UI.

**Vocabulary, settled.** **Offer** (`/api/offers`, "VPN offers") is a catalogue
entry a client claims (FREE) or buys (PREMIUM). **Package** (`/api/plans`,
"Packages") is a priced plan template — kind, traffic, duration, devices, price,
currency, slots, and the server + inbound it builds on. Same table
(`vpn_offers`), different `kind`/`price_cents`/`is_custom`.

**Creation was fine.** `POST /api/plans` → **201 Created**, row persisted with
`server_id: 11, inbound_id: 52`. The Packages screen then rendered
**"no server"** on that row — and on all four rows that already existed.

**Root cause.** `GET /api/plans` serialised `store.Plan` **directly**. That struct
has no `server_label`, `slots_remaining` or `purchasable` — they are derived —
while `ApiPlan` in `lib/api.ts` declares all three as required:

| Field | Declared | Sent | Rendered |
|---|---|---|---|
| `server_label` | `string` | absent → `undefined` | **"no server"** |
| `slots_remaining` | `number \| null` | absent | `∞` |
| `purchasable` | `boolean` | absent | `undefined` |

The **client** branch of the same handler already mapped through `toPlanFact`.
Only the operator branch forgot. TypeScript cannot catch it: a declared interface
is a promise, and nothing checks the server keeps it.

**Fixed.** New `planView` = `planFact` embedded (the derived, customer-safe fields)
+ operator-only `server_id`, `inbound_id`, `active`, `archived`, `owner_user_id`.
`GET /api/plans` answers `[]planView`. Every row now carries
`server_label: "server 11"` and renders `Premium 50GB Monthly · server 11 · …`.

**Evidence.**

- `TestPlanListCarriesTheRenderedFields` (new). Proven to bite: against the old
  `WriteJSON(plans)` it fails *"server_label = <nil>, want a non-empty string"*;
  against the fix it passes. Also asserts the four `ApiPlan` fields are present and
  that no secret leaks.
- `verify:offer-create` extended to cover **both** screens in one gate — 9 checks,
  **5/5 consecutive runs**. It creates a probe package, asserts the rendered row
  names its server and is not "no server", deletes it, and asserts both catalogues
  are back to their starting size. Verified afterwards: **zero residue**.
- Gates: `gofmt`/`go build`/`go vet` clean · `go test ./... -count=1` **17/17
  packages** · `npm run lint` 0 errors (3 pre-existing warnings in files not
  touched) · `build` OK · `audit` OK · `docs:verify` OK (**689** test functions).

**Not claimed.** Package *creation* is proven. Package *selling and provisioning*
is not — that needs a live panel write. The D-110 health finding
(`CONNECTION_UNAVAILABLE` on `/api/servers` while `/api/sync/status` reports
`ACTIVE`) is untouched. **D12 stays open.**

### Status

`FIXED-VERIFIED` for package creation and the operator package list. `OPEN` for
panel-health classification and `verify:vpn`'s stale selectors.


## DEV-037 · Super Admin could not create an offer — two id spaces, one FK (D-110)

**What this was.** The owner reported, directly, that offer/package creation still
failed for a Super Admin. Investigated live rather than argued: signed in as
`owner@veylix.test` in a real browser, opened `/offers`, clicked **New offer**,
submitted, and read the network tab.

**Both submissions refused, both correctly.**

| Attempt | Body | Answer |
|---|---|---|
| default (dropdown untouched) | `inbound_id: 0` | `422 offer_incomplete` |
| inbound chosen from the list | `inbound_id: 1` | `422 inbound_mismatch` |

**Root cause.** `vpn_offers.inbound_id` is a foreign key to `inbounds(id)` — a
local AUTOINCREMENT row id. `GET /api/servers/{id}/inbounds` returned
`panel.InboundOption` untouched, so its `id` was the **panel's** numbering. The
local mirror (`.veylix-secrets/data/veylix.db`) holds rows `52/53/54` for the same
three inbounds 3X-UI calls `1/2/3`; `inbounds.remote_id` exists precisely to carry
the panel's number, and `provisioning.panelInboundID` is the only translator. So
the form submitted `1`, the FK looked for local row `1`, found nothing, and the
guard said `inbound_mismatch` — while showing the operator an inbound listed for
the server they had named.

**Fixed.**

1. `integrations.Service.Inbounds` now returns the **local** `inbounds.id` it just
   cached (`localInbounds`: one query per server, skips rows the mirror does not
   hold — an offer cannot reference a row that is not there). `GET
   /api/servers/11/inbounds` answers `52/53/54`, was `1/2/3`.
2. The dropdown's empty option read **"Let the panel decide…"**, which names a
   capability that does not exist — the client is created inside a named inbound.
   It now reads **"Choose an inbound…"**, and `saveForm` refuses locally with
   *"Choose the inbound this offer creates its clients in."*, as `PackagesPage`
   already did.

**A claim I got wrong, and withdrew.** I first reported the 422 as invisible in
the dialog. It was not: the dialog rendered
*"inbound_id is required — an offer with no inbound cannot create a client"* as a
`role="alert"`. The finding was the misleading default and the field name, not a
swallowed error.

**Evidence.**

- `TestInboundsReturnsLocalIDNotPanelID` — new; fails against the old
  `return inbounds, nil` (*"returned id 1 is not a local inbounds.id"*), passes
  against the fix, and asserts the id the list hands out satisfies the exact
  predicate `POST /api/offers` runs.
- Live: `POST /api/offers {inbound_id: 52}` → **201 Created**; row rendered
  `QA Fix Proof 10GB · FREE · indian-1 · 10 GB · active`; probe offer deleted
  afterwards and the catalogue verified back at its original 5 rows.
- Gates: `go build ./...`, `go vet ./...` clean; `go test ./... -count=1` **17/17
  packages pass**; `npm run lint` 0 errors (3 pre-existing warnings in
  `Sidebar.tsx`, `SuperSettingsPage.tsx`, `VpnQuickView.tsx` — files not touched);
  `npm run build` OK; `npm run audit` OK; `npm run docs:verify` OK (README corrected
  from a stale 687 to the tree's 688, which already counted the new test).

**Not claimed.** `/api/servers` still reports `CONNECTION_UNAVAILABLE` while
`/api/sync/status` reports `ACTIVE` for the same panel, so every VPN detail page
still reads `SERVER_UNAVAILABLE`; the panel serves its login page (HTTP 200) but
its `/panel/api/...` paths answer 404 from here. Offer creation needs no panel
write, which is why it works; provisioning against the live panel is **not**
proven and D12 stays open. A temporary read-only `cmd/inboundprobe` was used to
inspect the mirror and then deleted.

### Status

`FIXED-VERIFIED` for offer creation (both defects). `OPEN` for the panel-health
classification (`CONNECTION_UNAVAILABLE` vs `ACTIVE`) and for `verify:vpn`, whose
script still asserts `vpn-capacity` / `vpn-live-traffic` / `vpn-metrics`, none of
which exist after the detail page was rebuilt into tabs.


## DEV-036 · Full end-to-end verification of both roles (D-109)

**What this was.** The owner asked for the whole app to be tested — Super Admin
side and client side — with every gate run one by one, and anything broken fixed.

**Executed.** All 18 committed browser gates, plus `go build`/`go vet`/`go test`
across 17 backend packages, plus `lint`, `build`, `audit`, `audit:css`,
`docs:verify`. Then the real UI was driven in a browser as both a Super Admin and
a client on the seeded panel.

**Result: 18/18 gates pass, 17/17 backend packages pass, 0 FAIL.**
Both roles verified end to end. Two defects and two gate defects were found and
fixed; the client side needed no product change.

**The Super Admin, verified live:** staff login (correct portal split),
`/dashboard` (LKR 2,500 · 1 sale · 1/1 panels · 6 clients · 8/10 VPNs · 100%
health · "8 items in 2 groups"), `/vpns` (10 rows, tags, Assign, Actions),
VPN detail `ravi` (Suspend/Revoke/Release, Copy identity, real date),
`/payments` (3 records, 2 awaiting, Confirm/Reject/Cancel).

**The client, verified live:** login, `/client/vpns` (3 VPNs, panel identity +
Copy), `/client/offers` (Claim correctly disabled with "Claim unavailable here" +
"Concurrent limit: 3 — you have reached it"), `/buy` (price fixed by the plan),
`/client/billing` (honest empty state), `/client/usage`, `/vpn/42` (detail with
Overview/Usage/Connection and **no** lifecycle controls). Zero console errors.

**Defect 1 — a date was invented.** `VPNDetailPage` rendered
`new Date(vpn.createdAt * 1000)` unconditionally. A panel client Veylix did not
create has `created_at = 0`, so the page said **"added 1/1/1970"** — a date
nobody chose, presented as fact. It now says *"created outside Veylix, so its
start date is unknown"* when `createdAt` is 0, and keeps the real date otherwise
(`ravi` still shows 11/3/2026).

**Defect 2 — a gate asserted the wrong page.** `verify:vpn-actions` signs in as
the Super Admin and then opened `/vpn/${id}`. `App.tsx` declares `/vpn/:id`
inside the **client** block on purpose ("one path, one owner", documented at
`App.tsx:357-367`), so the staff run matched no operator route, fell through to
the dashboard, and the "staff sees Suspend" assertion searched a dashboard for a
VPN control. The check could not pass. It now resolves the row's own
`server_id` + `panel_email` and opens the operator route
`/vpns/:serverId/:panelEmail`, which is what a Super Admin actually uses.

**Defect 3 — a permanently red gate.** `verify:vpn-actions` exited 1 on every run
because of defect 2. It now passes all 8 of its checks (Suspend/Revoke visibility,
client denial, suspend 204, no-404 regression, status change, resume 204).

**Defect 4 — a gate that failed one run in three.** `verify:studio` was flaky:
the Design Studio **persists its view** (`studioView` in `localStorage`), so it
reopens in whatever state the previous run left it. The gate assumed `compact`
and clicked "expand"; when the studio opened `full`, that click *collapsed* it and
`expandTogglesToFull` failed. It also clicked blind (`if (expand) expand.click()`)
and slept a fixed 600 ms. Fixed three ways: assert the **toggle** behaviour
relative to the observed start state, fail loudly when the control is absent, and
poll for the outcome instead of sleeping. **8/8 consecutive runs now pass.**

**Not changed.** `/vpn/:id` remains client-only and `/vpns/:serverId/:panelEmail`
remains the operator route. Neither the client block nor the super-admin block was
altered — the route design was already correct, and the earlier suspicion that
`/vpn/:id` was a dead route was wrong: a client opening `/vpn/42` gets the full
detail screen. `id: 0` on a panel client with no local Veylix row is also correct
for the UI, which navigates by server + panel email.

**Still not proven.** `go test -race` stays **BLOCKED** (no C compiler). The
band is green, but no provisioning write has been exercised against a real 3X-UI
panel — D12 remains ⛔ BLOCKED and is still the single biggest gap (D-108 §"still
not proven" applies unchanged).

## DEV-035 · The fulfilment chain delivered money without a product (D-108)

**Symptom reported by the owner.** A reseller could not meaningfully create an
offer, a client could order, and after confirmation no VPN appeared for the
client. Three complaints, which turned out to be one broken chain.

**Root cause, traced to a line.** `BuyPremiumPage.tsx` held the chosen plan in
component state (`offerId`, set when the customer picked a plan) and then called
`createPayment` with `{amountCents, methodId, reference}` only. `POST /api/payments`
therefore recorded `offer_id = NULL`. `payment_routes.go` then evaluated
`planAttached := o.OfferID != nil && *o.OfferID > 0` → `false`, skipped the
settlement service, and took the settlement-only branch:
`st.ConfirmPayment(...)`. The payment became `CONFIRMED` and no VPN was built.

`payments.Service.Confirm` already refused this case with `ErrNoPlan`, mapped to
`422 no_plan` — but the route never called it, so the guard was unreachable. The
backend schema and service were correct; the order simply never carried its plan.

**Proof this was the cause, not a guess.** `TestOrderWithPlanBuildsTheVpnOnConfirm`
passes `offer_id` and confirms the buyer owns one VPN built by the panel stub.
`TestConfirmingAPlanlessOrderSettlesCashOnly` omits it and confirms
`vpn_instance_id` stays null. Same route, same store, one field different.

**Fixed.**

1. `frontend/src/lib/api.ts` + `stores/paymentsStore.ts` — `createPayment` accepts
   `offerId` / `quantity` and sends `offer_id` / `quantity`.
2. `frontend/src/pages/BuyPremiumPage.tsx` — sends `offerId`, and **refuses to
   submit without a plan** instead of recording a payment that cannot be fulfilled.
3. `backend/internal/httpapi/payment_routes.go` — a plan order always goes through
   the settlement service; with the service absent it answers
   `503 orders_unavailable`. The settlement-only branch is now reachable only by a
   payment that genuinely carries no plan.
4. `backend/internal/httpapi/offer_routes.go` + `store/offers.go` — creation
   refuses an inbound belonging to another panel (`422 inbound_mismatch`).
5. `frontend/src/pages/OffersPage.tsx` — the inbound dropdown distinguishes
   *panel unreadable* from *panel has no inbound* and states the 3X-UI step;
   the previous `.catch(() => setInbounds([]))` swallowed the reason, which is why
   offer creation looked broken with no explanation.
6. `frontend/src/pages/client/ClientOffersPage.tsx` — `Claim unavailable here`
   renders whenever the claim is disabled, including at the concurrent limit;
   disabled offer cards state why they are disabled.

**Pre-existing failures found and fixed while verifying (not caused by this work).**

- `scripts/verify-client-offers.mjs` counted `.of-grid .of-card`, but the page
  renders its featured card outside `.of-grid`, so the gate under-counted by one
  whenever a package was featured. It now counts `.of-card`.
- `ClientOffersPage` disabled the claim at the concurrent limit without the
  sentence the same gate asserts. Fixed in the component, not by weakening the gate.

**Verification actually run.** `go build ./...`, `go vet ./...`,
`go test ./... -count=1` → 17/17 packages ok. `npm run lint` → 3 warnings, 0 errors
(unchanged baseline). `npm run build` → built. `npm run audit`,
`audit:selftest`, `audit:css`, `audit:css-selftest`, `docs:verify` → OK.
`verify:super-admin` 122/0, `verify:nav` 74/0, `verify:client-offers`, `verify:ui`,
`verify:clients`, `verify:client-vpns` → pass. New gate `verify:order-chain` reads
the real request body and asserts `offer_id` names a catalogue plan.

**Proven to bite.** Reverting the `offerId` line makes `verify:order-chain` report
`FAIL the order carries offer_id -> {"offer_id":0}` and exit non-zero. Removing the
`503` guard makes `TestPlanOrderRefusedWhenSettlementServiceIsMissing` fail.

**Not fixed here — reported.** `npm run verify:vpn` exits 1 (3/13). It asserts
blocks (`PremiumInsights`, `vpn-insights.css`) that are **deleted in the working
tree** by work that predates this session; `VPNDetailPage.tsx`,
`DeviceList.tsx` and `VpnHeroCard.tsx` are modified by the same unfinished
refactor. This change touches none of those files.


﻿# VEYLIX â€” DEVELOPMENT LEDGER

**Status:** authoritative Â· **Established:** 2026-11-08 Â· **Baseline:** `1b09553`

**What this file is.** The one register for implementation work: every defect, every
fix, every feature, from 2026-11-08 onward. It replaces the five overlapping
registers that preceded it (`IMPROVEMENT_LEDGER`, `VPN_FIX_LEDGER`,
`DISCREPANCY_REGISTER`, `UI_UX_DEFECT_BOARD`, `GAP_ANALYSIS`), which remain as
**historical reference** and are never appended to again.

**Rules.**
1. **Append-only.** A wrong entry gets a superseding entry; its text stays.
2. **One id, one entry.** `DEV-nnn`, strictly increasing.
3. **A fix is not "done" without a test and a gate run.** The entry names both.
4. **Never claim remote success from local state.** A panel effect is proven by
   reading the panel.
5. **Cite old ids as `LEGACY-<id>`** so their origin is unambiguous.

**Priority.** `P0` security / data-loss Â· `P1` broken core function, unverified
remote write, incorrect money Â· `P2` reliability, performance, maintainability Â·
`P3` polish.

---

## DEV-001 Â· P1 Â· An offer's inbound id was sent to the panel verbatim, so no FREE claim could succeed â€” **FIXED**

- **Date found & fixed:** 2026-11-08
- **Legacy id:** `I-36` (recorded here for the first time; the legacy entry was lost)
- **Symptom.** `POST /api/vpns` with a valid FREE offer answered **502
  `panel_rejected`**. `POST /api/offers` with `inbound_id: 1` answered **500**.
- **Cause.** Two numbering schemes in one column. `vpn_offers.inbound_id` is a
  foreign key to `inbounds(id)` â€” Veylix's own AUTOINCREMENT row id â€” while
  `inbounds.remote_id` holds the panel's number. The dev rows were `id=52,53,54`
  carrying `remote_id=1,2,3`. `provisioning.Create` copied `offer.InboundID` straight
  into the panel request, so the panel was asked for `inboundIds:[52]`, answered
  "inbound not found", and `pushCreate` â€” correctly â€” discarded the provisional row
  and reported `panel_rejected` (D-024.1a). The *rejection logic* was right; the
  *number* was wrong.
- **Fix.** `provisioning.panelInboundID(ctx, serverID, inboundID)` resolves
  `inbounds.remote_id`, and both the outbox payload and the live `AddClient` carry
  that number. The VPN row keeps the **local** id so the foreign key stays
  meaningful. A missing row, a row from a different server, or `remote_id <= 0` is
  `ErrOfferIncomplete` â€” refused before anything is written.
- **A second defect found while fixing it.** The resolver was first placed *inside*
  the create transaction. That held SQLite's write lock across two statements and
  made the second of two concurrent creates fail with `SQLITE_BUSY` instead of the
  quota refusal `TestQuotaIsEnforcedUnderConcurrency` expects â€” reproduced 4/4
  before the move, clean after. It now runs **before** the transaction opens.
- **Tests.** `TestCreateSendsThePanelsInboundNumber` (local `id=1`, `remote_id=7`,
  asserts the panel received `[7]`) and `TestCreateRefusesInboundFromAnotherServer`
  (refuses, writes nothing, never calls the panel). The fake client gained an
  `inboundIDs` field so what was *sent* is asserted, not merely that a call happened.
- **Live proof, both layers.** `POST /api/offers` â†’ **201** (`id=17`, FREE,
  `inbound_id=52`). `POST /api/vpns {offer_id:17}` as `client@veylix.test` â†’ **201**
  `{"vpn_id":42,"panel_email":"client@veylix.test_free_1","status":"ACTIVE","confirmed":true,"recorded":true}`.
  `cmd/panelprobe` then reported `vpn_id=42 â€¦ probe=FOUND` â€” the client exists **on
  the panel**, not merely in the local database.
- **Status:** âœ… **FIXED-VERIFIED**

---

## DEV-002 Â· P2 Â· 193 unreachable inbox rows left by a removed producer â€” **FIXED**

- **Date found & fixed:** 2026-11-08
- **Legacy id:** `I-37`
- **Symptom.** Every client inbox was dominated by identical noise: `Action not
  applied / user N Â· result=DENIED`. The table held 196 such rows out of 239 (82%).
- **Cause.** Already fixed **in code** â€” `audit.isSignInFailure` keeps a refused
  sign-in out of the inbox, and `audit.describe` now emits a specific sentence per
  action. The defect was the **history**: rows the removed producer had written had
  no code path that would clean them up, and none that would recreate them.
- **Fix.** A one-off, deliberately narrow cleanup:
  `DELETE FROM notifications WHERE title = 'Action not applied' AND body LIKE 'user %Â· result=DENIED'`.
  Because the current producer can no longer emit that exact shape, the predicate
  cannot delete a row the running code creates.
- **Data safety.** Backup first: `.veylix-secrets/data/veylix.pre-20261108-042952.db`
  (782,336 bytes). Dry run first (193 matched, 46 kept), then applied: **193
  deleted, 46 remained**.
- **Verification.** `GET /api/notifications` as `client@veylix.test` afterwards
  returns two rows, both meaningful and both from real actions: `VPN created / vpn
  42 Â· status=ACTIVE` (success) and `VPN could not be created / vpn 41` (error â€” the
  502 attempt above). No `Action not applied` row remains.
- **Status:** âœ… **FIXED** (data). The code was already correct.

---

## DEV-003 Â· P2 Â· `httpapi` runs SQL, contradicting the "only `store` writes SQL" rule â€” **OPEN**

- **Date recorded:** 2026-11-08
- **Evidence.** 12 SQL statements in `internal/httpapi/`, measured:
  `usage_store.go` 7 Â· `vpn_lister.go` 2 Â· `vpn_lister_panel.go` 2 Â· `panel_cache.go`
  1. Across the whole tree, 112 statements live outside `store/`.
- **Why recorded rather than fixed now.** It is not a correctness bug â€” the code
  works and 617 tests pass. It is a maintainability finding, and fixing it touches
  the VPN list and usage read paths the client UI depends on.
- **Decision needed.** `UNKNOWNS.md` `Q-13`: Option A (move the 12 queries into
  `store`) or Option B (rewrite the rule to describe reality).
- **Status:** ðŸ”¶ **OPEN** â€” recorded, not fixed.

---

## DEV-004 Â· P3 Â· `vpn_transfers` rows die with their VPN â€” **OPEN**

- **Date recorded:** 2026-11-08 Â· **Carried from** `LEGACY-I-34`
- **Evidence.** `0020_vpn_transfer.sql:63` declares `ON DELETE CASCADE`.
- **Why it matters.** The record "this VPN moved from client A to client B" vanishes
  when the VPN row is deleted, so transfer history cannot outlive its subject.
- **Same class already fixed:** `LEGACY-I-33` on `vpn_releases`, fixed by migration
  `0023` (nullable + `SET NULL` + `panel_email`/`server_id` retained). That migration
  is the template for this fix.
- **Status:** ðŸ”¶ **OPEN** (P3)

---

## DEV-005 Â· P3 Â· `usage_snapshots` rows die with their VPN â€” **OPEN**

- **Date recorded:** 2026-11-08 Â· **Carried from** `LEGACY-I-35`
- **Evidence.** `0003_observability.sql:7` declares `ON DELETE CASCADE`.
- **Why it matters.** Per-VPN usage history disappears when the VPN is purged, so a
  client's own traffic trend cannot survive a revoke-and-recreate.
- **Caution carried forward.** Snapshots are the largest table by row count, so a
  `SET NULL` fix would retain dead series indefinitely on a 1 vCPU / 500 MB host. The
  earlier pass recommended **not** applying it without a consumer. That
  recommendation stands and is not overturned here.
- **Status:** ðŸ”¶ **OPEN** (P3, deliberately deferred)

---

## DEV-006 Â· P2 Â· Client-side style debt: 224 inline `style={{â€¦}}` â€” **OPEN**

- **Date recorded:** 2026-11-08 Â· **Carried from** `LEGACY-U13`
- **Evidence.** 224 inline style objects across `frontend/src/**/*.tsx`, repeating
  font sizes, gaps and colours that the design tokens already define.
- **Why it matters.** It defeats the theme system: a token change cannot reach an
  inline literal, so a sub-theme preset drifts from the rest of the UI on exactly
  the screens that use inline styles.
- **Status:** ðŸ”¶ **OPEN** (P2)

---

## DEV-007 Â· P2 Â· `EmptyState` missing on three Admin pages â€” **OPEN**

- **Date recorded:** 2026-11-08 Â· **Carried from** `LEGACY-U16`
- **Evidence.** `EmptyState` is used on `/client`, `/history` and `/buy`; three Admin
  pages render a bare empty table instead.
- **Why it matters.** An empty table is ambiguous â€” it reads the same whether there
  are no rows or the read failed. An explicit empty state is the honest answer, and
  the client surface already does this correctly.
- **Status:** ðŸ”¶ **OPEN** (P2)

---

## DEV-008 Â· P2 Â· Mixed loading indicators (`Skeleton` vs inline "Loadingâ€¦") â€” **OPEN**

- **Date recorded:** 2026-11-08 Â· **Carried from** `LEGACY-U17`
- **Evidence.** Lists use `Skeleton`; several detail panes render the text
  "Loadingâ€¦".
- **Status:** ðŸ”¶ **OPEN** (P2)

---

## DEV-009 Â· P2 Â· Focus ring unverified on three components â€” **OPEN**

- **Date recorded:** 2026-11-08 Â· **Carried from** `LEGACY-U18`
- **Evidence.** No automated check asserts a visible focus ring on `SavedViewsBar`,
  `Pagination`, or table sort headers.
- **Why it matters.** Keyboard-only navigation is a real path, and an invisible focus
  ring makes it unusable. "Not verified" is not the same as "wrong" â€” but it is not
  `PASS` either.
- **Status:** ðŸ”¶ **OPEN** (P2, accessibility)

---

## DEV-010 Â· P2 Â· `verify-mobile.mjs` measures only 390 px and never enables touch â€” **OPEN**

- **Date recorded:** 2026-11-08 Â· **Carried from** `LEGACY-S3-1` + `LEGACY-S3-4`
- **Evidence.** The script asserts layout at 390 px only, while the design brief
  requires several widths. It also never turns on CDP touch emulation, so a
  tap-target regression would pass.
- **Why it matters.** A responsive gate that checks one width reports `PASS` for
  viewports it never rendered. That is the failure mode the audit rules exist to
  prevent.
- **Status:** ðŸ”¶ **OPEN** (P2)

---

## DEV-011 Â· P1 Â· The client "My VPNs" page was never executed against a populated account â€” **FIXED**

- **Date found:** 2026-11-08 (recorded in the S5 report as "populated state is NOT
  PROVEN") Â· **Fixed:** 2026-11-08 (same day, once `DEV-001` unblocked it)
- **Symptom â€” a verification gap, not a user-visible bug.** S5 shipped
  `MyVpnsPage.tsx` with cards, a compact list, search, category chips, a filter
  panel, sort and paging. Every one of those was type-checked and lint-checked, and
  the CSS was probed by mounting a detached replica â€” but **none of it had ever run

## DEV-012 - `/api/packages` was typed as `ApiPlan[]`, which is not what it sends

- **Severity:** P2 (a false type, not a runtime crash - but it made every caller
  read `undefined` for three fields while TypeScript reported success).
- **Where:** `frontend/src/lib/api.ts` - `listPackages()`.
- **What was wrong.** `listPackages()` declared `request<ApiPlan[]>('/api/packages')`.
  `ApiPlan` requires `slots_remaining`, `active` and `archived`. The running server
  was queried on 2026-11-08 and `/api/packages` sends **eleven** keys:
  `currency Â· device_limit Â· duration_days Â· id Â· kind Â· name Â· price_cents Â·
  priced_per_order Â· purchasable Â· server_label Â· traffic_gb`.
  It sends **none** of the three `ApiPlan` requires. Any code reading
  `pkg.active` or `pkg.slots_remaining` from this endpoint got `undefined` and the
  compiler stayed silent.
- **Why it mattered for Section 03.** The catalogue screen must decide whether a Buy
  button may be enabled. The fields that answer that question - `purchasable` and
  `priced_per_order` - were not in the declared type at all, so the honest data was
  on the wire and invisible to the type system.
- **The fix, and the boundary it respected.** A new `ApiPackage` interface describes
  the real payload, and `listPackagesTyped()` exposes it. `listPackages()` was **left
  unchanged**, because `packagesStore.ts` already consumes it and repointing one
  endpoint at a second shape is how a shared module breaks a screen it was not meant
  to touch. Editing `packagesStore.ts` was outside the Section 03 allowlist, so the
  additive route was taken instead.
  - Caught by `tsc -b --noEmit`: the first attempt *did* repoint `listPackages()`,
    which immediately failed `packagesStore.ts:93` with `Type 'ApiPackage[]' is not
    assignable to type 'ApiPlan[]'`. That is recorded here because it is evidence the
    older signature is genuinely load-bearing for another screen.
- **`kind` is wider than the union.** The handler rewrites a priced-per-order package
  to `"CUSTOM"` before serialising, so a client can receive a third value that
  `ApiPlanKind` does not admit. `ApiPackage.kind` is therefore `string`, and the
  screen folds CUSTOM into Premium rather than trusting the discriminant.
- **Status:** âœ… **FIXED-VERIFIED** - `tsc` clean, and
  `npm run verify:client-offers` asserts that CUSTOM counts as Premium.

## DEV-013 - `server_label` is an internal placeholder and reached the customer surface

- **Severity:** P2 (a customer-facing screen would have printed `server 11`).
- **Where:** the value comes from `backend/internal/httpapi/plan_routes.go:56`, which
  formats the label as `server %d`. No backend file was changed.
- **What was wrong, and what was NOT.** `GET /api/packages` sends `server_label` but
  **not** `server_id`. The obvious implementation - render the label - would have
  shown a customer the literal string `server 11`, which is an internal identifier
  wearing a display name. The obvious alternative - call `GET /api/servers` - is
  **403 for a client** (verified), so it is not available to this screen either.
- **The resolution.** The real name is read from the client's **own** VPN rows
  (`GET /api/vpns`, which already returns `server_id` and `server_name` such as
  `indian-1`). The same rows supply the `offer_id -> server_id` map a Free claim
  needs, so one request answers both questions and no request is made per card.
  - When the name cannot be resolved, it is **omitted** and the cell reads `n/a`.
    Aliases such as "Server A" are not invented.
  - No inbound id, panel URL, token or credential is ever rendered.
- **A second, related limitation, accepted and stated on screen.** A **Free** offer
  the client has never held has no `offer_id` row, so its `server_id` cannot be
  established. The claim is then **disabled with a stated reason** and a link to the
  Dashboard, where the supported flow exists. Fixing this properly needs the
  catalogue to expose `server_id`, which is a backend change outside Section 03.
  This is recorded as an open limitation, not silently worked around.
- **Status:** âœ… **FIXED-VERIFIED** for the placeholder leak.
  âš ï¸ **PARTIAL, with a stated reason,** for the never-held Free offer: the claim is
  honestly disabled rather than guessed. `npm run verify:client-offers` asserts that
  no `server N` placeholder and no panel URL reaches the DOM.

  against real rows**, because the seeded client owned zero VPNs.
- **Root cause of the gap.** It was a chain, not a single fault: the seeded client
  held no VPN â†’ assigning one was refused (`409 panel_client_taken`) â†’ creating a
  FREE offer to claim was impossible because `POST /api/offers` returned **500**.
  That last link is `DEV-001` / `LEGACY-I-36`. Once the inbound-id translation was
  fixed, every link above it broke loose and the populated state became reachable.
- **What was done.**
  1. Three FREE offers created via `POST /api/offers` (**201** each): `Free 2GB
     Standard` (inbound 53), `Free 5GB Hysteria` (inbound 54), and the existing
     `Free 1GB Trial` (inbound 52).
  2. Two claims via `POST /api/vpns` as the client (**201**, `ACTIVE`,
     `confirmed:true`) â€” VPN **43** and **44**, alongside the existing VPN **42**.
  3. **Both layers verified.** `cmd/panelprobe -server 11` reported `probe=FOUND`
     for **all six** panel clients, including 42/43/44 â€” so these are real 3X-UI
     clients, not local-only rows (INV-1).
  4. `frontend/scripts/verify-client-vpns.mjs` written and registered as
     `npm run verify:client-vpns`.
- **The committed gate â€” 19 checks, `19/19 PASS`.**
  Page h1 Â· every owned VPN rendered as a card Â· more than one card (so search and
  sort are meaningful) Â· no empty state on a populated account Â· overview TOTAL
  matches the real inventory Â· search narrows to one card Â· the search term reaches
  the URL Â· a non-matching term empties the grid Â· the no-results copy appears Â· the
  copy reassures the user their VPNs still exist Â· a Clear filters action exists Â·
  Clear filters restores every card Â· the List mode renders a real table Â· the table
  lists every row Â· the table exposes VPN/STATUS/SERVER Â· the display mode reaches
  the URL Â· a name-sort control exists Â· sorting keeps every row Â· the sort is
  genuinely alphabetical.
- **The gate refuses to lie.** Against an account that owns nothing it prints
  `REFUSED â€¦ Reporting PASS here would be a false claim.` and exits **3**. Proven:
  run as `reseller@veylix.test` (owns 0) â†’ `REFUSED`, exit **3**. A regression that
  empties the account therefore fails the gate instead of silently passing it.
- **A correction made during this work, recorded rather than hidden.** My first
  probe reported `sortable: []` and I nearly recorded "sort is missing". It is not:
  sort lives in the **Filters** panel as a `<select>` (`SortKey = newest | name |
  expiry`), not as sortable table headers. The first probe was looking in the wrong
  place. Reading `MyVpnsPage.tsx` L563â€“570 settled it.
- **Status:** âœ… **FIXED-VERIFIED**



## DEV-014 - The client VPN list never shows the panel identity

- **Severity:** P1 (the customer cannot identify the VPN that D-034 says is its name).
- **Raised by:** the owner, reviewing `MyVpnsPage` against `D-034`:
  *"3x ui eke email eka vpn name eka una ewa mn doc eke pehediliwa kiwwa"*.
- **Where:** `frontend/src/pages/client/MyVpnsPage.tsx`.
- **What is wrong.** `D-034.1` defines a VPN's identity as
  `<client email>_<kind>_<n>` â€” `nimal@gmail.com_paid_2`. The client's own VPN list
  renders `vpnDisplayName(v)`, which is the **package** name (`Free 5GB Hysteria`)
  or the generic kind label, and it never renders `vpn.panelEmail` at all. Grepping
  the file for `panelEmail` returns **0 hits**.
- **Why it matters.** Every other VPN surface already shows it
  (`VPNDetailPage.tsx:787`, `HistoryPage.tsx:106`, `ClientDetailPage.tsx:38`,
  `AdminVpnsPage`, `VpnsPage`). So the one screen a customer opens most was the only
  one that hid the identity â€” and a customer who cannot read their panel email cannot
  tell support which client they hold, so the operator has to guess on every ticket.
- **The fix owed.** Render the panel identity beside the package name, readable and
  copyable, with the copy target being the **exact** panel email. Both facts, never
  one instead of the other â€” replacing the friendly package name with a raw
  `_paid_1` blob would be a regression, not a fix.
- **Status:** âœ… **FIXED-VERIFIED** (2026-11-08, D-092) â€” `MyVpnsPage` now renders
  `<PanelIdentityRow panelEmail={v.panelEmail}>` in **both** views (card and list row),
  giving `vpn-identity-<id>` / `vpn-row-identity-<id>` test ids. The component shows the
  account part readably plus a `Free/Premium Â· #n` chip, and its copy button copies the
  **exact** panel email (`panel-identity.css`, `PanelIdentityRow.tsx`). The package name
  is still the headline, so nothing was traded away. `grep -c panelEmail MyVpnsPage.tsx`
  went from **0** to a rendered row in both layouts. Confirmed by `npm run build` and
  `npm run audit` (PASS).

## DEV-015 - A vanished panel client is not explained on the client list

- **Severity:** P1 (the state exists to be shown, and the main screen does not show it).
- **Raised by:** the owner: *"yamkisi hethuwa kin 3x ui client eke vpn eka nethi unoth
  ee eroor eka vpn eke pennana one"*.
- **Where:** `frontend/src/pages/client/MyVpnsPage.tsx` (list) vs
  `frontend/src/pages/VPNDetailPage.tsx` (detail).
- **What is right already.** The detail page carries a `NOT_CONNECTABLE` map that
  explains **all nine** emitted statuses, including
  `SERVER_UNAVAILABLE: 'The server this VPN lives on is not reachable right now.'`,
  and it deliberately gives an **unknown** status a generic sentence rather than an
  invented one. That is the reference implementation and it should not be rewritten.
- **What is missing.** The list renders only a `StatusBadge`. A customer scanning
  their VPNs sees a grey chip and no sentence at all, so "Veylix cannot see your VPN
  right now" is indistinguishable from "your VPN is fine" unless they open it. The
  whole point of `SERVER_UNAVAILABLE` (D-024.5, INV-1) is that it is an honest
  *pending* state rather than a fake success â€” a state nobody is told about is not
  honest, it is merely not lying.
- **The fix owed.** A row-level explanation for every non-`ACTIVE` status, reusing the
  same wording the detail page already uses so the two screens cannot drift apart.
  `SERVER_UNAVAILABLE` must remain listed, never hidden and never `REVOKED`.
- **Status:** âœ… **FIXED-VERIFIED** (2026-11-08, D-092) â€” the nine sentences now live in
  `vpnStore.ts` as `NOT_CONNECTABLE` + `GENERIC_STATUS_NOTE`, exposed through
  `statusNote(status)`. That is the **single** source: `MyVpnsPage` (card and row),
  `VpnQuickView` and `VPNDetailPage` all call it, so the three screens cannot drift.
  The card renders `vpn-note-<id>` for every non-`ACTIVE` status; the list row renders

---

## DEV-017 - `verify-vpn-details.mjs` was a registered npm script containing zero code

| | |
|---|---|
| **Status** | `FIXED-VERIFIED` (2026-11-08) |
| **Affected** | `frontend/scripts/verify-vpn-details.mjs`, `frontend/package.json` |
| **Evidence** | The file was **14,510 bytes of NUL** â€” `nulCount=14510, nonzeroCount=0`. `npm run verify:vpn-details` was registered and pointed at it. All 26 other `verify-*.mjs` files were intact (nonzero count == filesize). |
| **Why it matters** | Every browser claim about the Quick View, the three-section detail page and the connection reveal gate was unbacked. QA area 7 was **NOT RUN** while the repo looked as though it had a gate. |
| **Fix** | Rewritten from `verify-client-offers.mjs`'s pattern: headless Chrome over raw CDP, `check()`/`exit(1)`, read-only against the app (navigates, clicks tabs, reads the DOM â€” never presses a lifecycle control). |
| **Result** | **18 PASS Â· 0 FAIL Â· 5 SKIP**, each skip stating its reason. |

**Three false failures were produced by the first draft of the replacement, all
recorded because they were my errors:**

1. It read `VpnKindBadge` (`FREE`) where it meant `StatusBadge` â€” the wrong fact.
2. It expected three `<h2>` simultaneously on a screen that renders one section at
   a time. The real rule is "each section has exactly one `<h2>`", so the check now
   visits all three tabs.
3. It treated a FREE VPN's deliberately absent charts as failures. Those are now
   **SKIP with a reason**, never PASS.

A fourth: the URI regex omitted `hysteria2`, so a working reveal
(`hysteria2://8844ee5aâ€¦@difanx4f3.difan`) was reported as a failure. Root cause was
found by dumping the live DOM, not by guessing.

---

## DEV-018 - Charts plotted raw byte counts and stated no value at all

| | |
|---|---|
| **Status** | `FIXED-VERIFIED` (2026-11-08) |
| **Affected** | `components/charts/LineChart.tsx`, `BarChart.tsx`, `charts.css`, `lib/format.ts` |
| **Evidence** | `BarChart` labelled bars with `d.value.toLocaleString()` â†’ `33,327,637,690`; the tooltip did the same; `LineChart`'s tooltip likewise. Neither chart drew **any** value axis (`padX = 8`, no tick text), so a reader could see a shape but never a magnitude. |
| **Fix** | `valueFormat?: (n) => string`, `yAxis?: boolean` and `zeroBased?: boolean` on both charts. `formatBytesIn()` + `ByteUnit` in `lib/format.ts`. `.ui-chart-axis` is a class, not an inline style (D-059). Defaults preserve every existing caller's behaviour. |
| **Verified in a browser** | axis ticks `18` at `10px/600`; tooltip `0 B`; unit picker `auto/GB/MB/TB/KB/B`; `0 B of 5.0 GB` â†’ `0.0 MB of 5120 MB` on switch, with **no request issued**. |

---

## DEV-019 - The exhaustion projection did not exist, and a projection is not a measurement

| | |
|---|---|
| **Status** | `FIXED-VERIFIED` (2026-11-08), estimate form only |
| **Affected** | `components/vpn/VpnUsageSection.tsx` |
| **Decision** | D-088.1 forbids the UI from stating a number the server never produced. The server has **no** projected-exhaustion endpoint, so the screen states an **estimate**: `~N days`, computed only from the server's own `remaining_bytes` and the window's timestamps, capped at 730 days, and returning nothing when there is no measurable traffic. The card carries *"an estimate, not a guarantee"* in the body text, not in a tooltip. |
| **Also built in the same card set** | 7Ã—24 heatmap (no backend change: `/usage/hourly` carries both `hour_start` and `hour_of_day`), daily bars, and a busiest-hour sentence. |

  the same sentence under the badge; the detail page renders `vd-status-note`.
  `SERVER_UNAVAILABLE` keeps its "nothing has been deleted, it will show again" wording
  and is never reported as `REVOKED`. An unrecognised status gets the generic sentence.

## DEV-016 - The VPN detail page has no single layout hierarchy

- **Severity:** P2 (it renders, but the outline is illegible and the page is fragile).
- **Raised by:** the owner: *"vpn detals page eke desing eka front end desaing eka
  kethayi upset"*.
- **Where:** `frontend/src/pages/VPNDetailPage.tsx` (898 lines).
- **Three concrete faults, each checkable.**
  1. **A heading is duplicated at two levels.** `<h2 className="vpn-section-title">
     Connection</h2>` sits directly above `<Card title="Connection">` â€” the same word
     in the same column, twice, at two different levels.
  2. **Two section patterns are mixed in one column.** `Usage` and `Devices` are bare
     `<h2>` headings over a component, `Connection` is an `<h2>` **plus** a `Card`, and
     `Setup guide` / `Keep this VPN going` / `Support` are `<Card title>` **with no
     `<h2>` at all**. The page therefore has no consistent outline for a screen
     reader or for the eye.
  3. **23 inline `style={{ â€¦ }}` remain.** `D-059` converted inline styles to token
     classes; this page regressed, with the same block
     (`{ display: 'flex', gap: 10, alignItems: 'flex-start' }`) repeated on four
     `vpn-warn` divs instead of one class in `vpns.css`.
- **What is already right, and must be preserved.** Zero hardcoded colours â€” every
  value comes from a token. The status explanations are good. The verdict band, the
  usage figures and the "server's numbers, not ours" discipline (D-038 P-01/P-02) are
  all correct.
- **The fix owed.** One hierarchy (all `<h2>` sections, each containing its cards),
  no repeated heading word, and the four inline layout blocks promoted to a class in
  `vpns.css`.
- **Status:** âœ… **FIXED-VERIFIED** (2026-11-08, D-092) â€” `VPNDetailPage.tsx` rewritten
  from 943 lines to **~340**, now three sections (`Overview` / `Usage` /
  `Connection & Help`) selected by a `vd-tabs` row. The three faults:
  1. The duplicate heading is gone â€” the section is `<h2>Connection &amp; Help</h2>` and
     the card inside it never repeats that string; `Overview` and `Usage` follow the
     same shape.
  2. One pattern only: every section is an `<h2>` `.vd-h2` followed by `Card`s whose
     titles describe their content. No section is a bare heading and none is a
     card-without-a-heading.
  3. **0** inline `style={{ â€¦ }}` in the JSX â€” the only remaining occurrence of that
     string in the file is inside the comment explaining the old defect
     (`Select-String -Pattern 'style=\{\{'` â†’ 1 hit, a comment line). The repeated
     4-line flex block became `.vd-actions`, and section layout lives in
     `vpn-detail.css`. The four new components beside it are also 0
     (`VpnUsageSection`, `VpnConnectSection`, `VpnQuickView`, `VpnUsageCards`,
     `PanelIdentityRow`).
  Preserved as required: zero hardcoded colours, the status sentences, and the
  server's-numbers discipline (all figures still come from `usage_*` fields).
  Side effect recorded: the route bundle fell **41.86 kB â†’ 33.15 kB** (gzip
  **12.59 â†’ 9.94 kB**) because the four-chart `PremiumInsights` was replaced.

---

## DEV-020 - A dead VPN was permanently in the customer's face with no "not now"

- **Severity:** P1 (the only escape was a deletion-shaped action)
- **Found:** 2026-11-08, from the owner's request for a way to stop a dead VPN
  demanding attention.
- **Symptom:** a VPN whose panel client is gone becomes `SERVER_UNAVAILABLE`.
  That is correct (D-091.5, INV-1) â€” but it then sorted **first** under
  `sort=newest`, counted toward "Needs attention" on every visit, and the Usage
  screen fired two requests per visit to measure a VPN that cannot change. The
  only tool available was **Release** (D-027): a 3-day window with an undo the
  customer must perform, for what is often just "not now".
- **Root cause:** the product modelled *delete* but not *ignore*. Every
  mechanism that could lower a VPN's prominence was destructive by design, so a
  customer who wanted the noise to stop had to destroy the record.
- **Fix:** D-094. `dormant_at` (migration `0025`), `POST /api/vpns/{id}/dormant`
  and `/wake`, `VPNRow.dormant_at`, the client list sorts dormant last under
  **every** sort key and drops them from `attentionCount`, the card explains the
  choice in a muted sentence (not a warning colour) with a Wake button, and the
  Usage block **skips the fetch entirely** rather than making and discarding it.
- **What it deliberately does not do:** no panel call, no status change, no
  deletion, no hiding. A dormant `SERVER_UNAVAILABLE` VPN still reads
  `SERVER_UNAVAILABLE`, and `TestDormantRoutePutsVPNToSleep` asserts the status
  did **not** move.
- **Tests:** `TestDormantRoutePutsVPNToSleep`, `TestWakeRouteClearsDormancy`,
  `TestDormantIsIdempotent`, `TestDormantRouteRefusesOtherOwnersVPN`,
  `TestDormantRouteRefusesAnonymous`, `TestSyncLeavesDormantVPNStatusAlone`.
- **Status:** `FIXED-VERIFIED (backend code + suite; frontend typecheck + lint +
  build + audit)`. Browser-level proof of the sort order and the stopped polling
  is **not yet committed** â€” see DEV-023.

## DEV-021 - A dormant VPN was re-flagged `SERVER_UNAVAILABLE` on every sync tick

- **Severity:** P1 (would have made DEV-020's switch silently do nothing)
- **Found:** 2026-11-08, while writing the first dormancy test â€” the test failed
  with `reported absent = 1, want 0 while dormant`.
- **Symptom:** `markAbsent` promoted a dormant VPN to `SERVER_UNAVAILABLE` on
  every pass. The customer put it to sleep, and the sync loop woke it back into
  "Needs attention" within a minute.
- **Root cause:** the presence reconciler knew about `RELEASED` (deliberately
  absent) and `REVOKED`, but not about dormancy. Its `res.Absent` counter also
  incremented unconditionally, so the pass claimed work it had not done.
- **Fix:** `sync.Service.markAbsent` now returns `(changed bool, err error)` and
  short-circuits on `v.DormantAt.Valid`; the caller only increments
  `res.Absent` when the row actually changed. `localVPN` gained `DormantAt`, read
  in `localVPNs`.
- **Honesty preserved:** dormancy defers the alarm, it does not erase it. The
  same test then wakes the VPN and asserts the **next** pass records
  `SERVER_UNAVAILABLE` â€” so the absence is never lost, only postponed.
- **Status:** `FIXED-VERIFIED` â€” `TestSyncLeavesDormantVPNStatusAlone`.

## DEV-022 - A NUL-corrupted comment in `vpn_lister_panel.go`, and a duplicated doc comment

- **Severity:** P3 (cosmetic, but it corrupted the explanation a future reader
  depends on)
- **Found:** 2026-11-08, while adding `dormant_at` to the same scan block.
- **Symptom:** the D-063 comment read `// \x07ssigned: false whenever â€¦` with the
  next line duplicated (`// confirm it, so the console printed unassigned â€¦`), so
  the sentence explaining why `Assigned` is derived from `OwnerID > 0` was
  partly unreadable. Separately, `MyVpnsPage.tsx` had a stray `/**` opening a
  second comment inside `needsAttention`'s doc block.
- **Root cause:** earlier multi-part edits truncated mid-character. The same
  editor-`insert_line` hazard recorded in DEV-017/018/019 (CSS brace loss, split
  JS blocks) â€” this time in a Go comment, which no gate catches because it still
  compiles.
- **Fix:** both comments restored to their intended text. No behaviour change.
- **Status:** `FIXED-VERIFIED (read back; build + vet clean)`.

## DEV-025 - Usage & History had no way to narrow which VPN it was showing

- **Severity:** P2 (usability; the owner asked for a "source" filter)
- **Found:** 2026-11-08, from the owner's request.
- **Symptom:** `/client/usage` rendered every owned VPN as one flat list with no
  way to isolate one VPN or one server. A customer with several VPNs had to scan
  all of them.
- **Root cause:** the page never had filter state; the only control was the byte
  unit. The list state pattern (`useListQuery`) existed and was used by My VPNs,
  but was never applied here.
- **Fix (D-095):** a **source picker** â€” *All VPNs Â· Server: X Â· a named VPN* â€”
  held in the URL through `useListQuery`, so a filtered view is shareable and Back
  undoes it. The count reads "N of M VPNs". A filter matching nothing shows an
  explicit empty state with a **Show all VPNs** button instead of an empty list,
  which would read as "no traffic".
- **What it deliberately is NOT:** a filter by traffic origin. Audited and refused
  â€” the panel exposes one aggregate `up`/`down`/`total` per client with no
  per-origin split, so "device" or "upstream" options would have to invent one.
  Recorded in D-095 so it is not re-explored.
- **Browser proof:** `frontend/scripts/verify-client-usage-source.mjs`
  (`npm run verify:client-usage-source`) â€” **9 PASS Â· 0 FAIL**, read-only: it
  proves the list narrows to one block, the count follows, the URL carries the
  choice (`?source=vpn%3A44`), an unmatched filter explains itself and renders no
  block, and clearing it restores all three.
- **Status:** `FIXED-VERIFIED`.

## DEV-026 - `vpnPingHistory` was a callable endpoint with zero UI callers (B-1)

- **Severity:** P2 (dead surface)
- **Found:** recorded earlier as B-1; confirmed again this session
  (`SELECT`-level: `api.ts` defines it, nothing calls it).
- **Fix:** the endpoint gained the D-095 `source` filter and now **echoes the
  source actually applied**, so any future consumer cannot mislabel a filtered
  read. The filter is validated (`422` on an unknown value, never a silent
  match-all) and passed as a **bound parameter**, never concatenated.
- **Live proof:** `?source=all` â†’ `200 source=all`; `?source=server_reachability`
  â†’ `200 source=server_reachability`; `?source=device` and `?source=typo` â†’ **`422
  validation_failed`**.
- **Still outstanding, stated honestly:** no screen renders a latency chart yet,
  so the endpoint remains **unconsumed** by the UI. Wiring it needs a History
  surface that does not exist; the filter work is complete and tested.
- **Status:** `PARTIAL` â€” server side `FIXED-VERIFIED`, UI consumption still open.



- **Severity:** P2 (evidence gap â€” one browser assertion stays SKIP)
- **Found:** 2026-11-08, attempting to unblock the range-switch assertion in
  `verify-vpn-details.mjs`.
- **Attempted:** `cmd/seedpremium` (new, `--dry-run`-first, idempotent) inserted
  `client@veylix.test_paid_1` on offer 20 `"Premium 50GB Monthly"`. It worked, and
  the row listed as `PREMIUM` â€” but `GET /api/vpns/48/usage` answered
  `history: NONE, points: 0`.
- **Why the fixture could not work:** `sync` only writes a `usage_snapshots` row
  when the **panel** returns the client (`applyReadings`: `row, seen :=
  byPanelEmail[...]`; `!seen` â†’ verified-absent path â†’ `markAbsent`, no snapshot).
  A local-only row has no panel client, so it can never accumulate history. The
  fixture was therefore **deleted again** (`DELETE ... WHERE id = 48`, 1 row, no
  snapshots existed, no panel client existed).
- **Then attempted, through the product's own routes only:**
  1. `POST /api/vpns {offer_id:20}` â†’ **`403 plan_not_approved`**. D-030.3: a
     platform-owned PREMIUM plan may only be issued after a Super Admin confirms an
     order. **Correct behaviour, not a bug.**
  2. `POST /api/panel-clients/11/athoof/assign` â†’ **`409 panel_client_taken`**.
     `panel_assignments` row 5 shows `source_email=athoof` already reserved to
     `janith15dawan@gmail.com_paid_1` (whose VPN row no longer exists). Re-assigning
     would move another customer's record. **Correct behaviour.**
- **Data safety:** the tag I set on `athoof` during the attempt was reverted to its
  original `FREE`, the fixture row was deleted, and the temporary inspection tool
  (`cmd/inspecttmp`) was removed. Verified afterwards: `athoof` = `FREE`,
  `assigned=false`; the client owns exactly 42/43/44, all FREE.
- **What actually blocks it:** the charts are gated on `kind !== 'FREE'`
  (`VpnUsageSection.tsx:526`) **by product decision** ("Charts are part of
  Premium"). Unblocking needs a **real** PREMIUM VPN on the test client â€” which
  requires either a confirmed payment order (Super Admin) or a genuinely free panel
  client on the dev panel. Both are owner/ops actions, not code changes.
- **Not done, deliberately:** I did not weaken the `kind !== 'FREE'` gate, invent
  snapshot rows, or steal a reserved panel client. Any of those would have produced a
  green tick that proves nothing.
- **Status:** `BLOCKED` â€” stated, with the two exact remedies above.



- **Severity:** P2 (evidence gap, not a code defect)
- **Found:** 2026-11-08, at the end of the D-094 implementation.
- **Detail:** the sort order ("dormant last under every key"), the excluded
  `attentionCount`, the muted sentence, and the **stopped usage polling** are all
  proved by unit/route tests and by reading the code, but no `verify-*.mjs` walks
  the flow in a browser. Per `.clinerules/05-quality-testing.md`, a Playwright
  session by hand is **not** a gate.
- **Fix:** `frontend/scripts/verify-client-dormant.mjs`, registered as
  `npm run verify:client-dormant`. It signs in as the client, sleeps VPN 44,
  asserts the sentence ("â€¦nothing was deletedâ€¦"), that the card sorts last under
  **both** `newest` and `name`, that it leaves "Needs attention", that the control
  flips to "Wake", and that the Usage screen shows the asleep notice and **no
  allowance meter**. It always wakes the VPN again in a `finally` block. It
  **refuses (exit 3)** when the running backend predates `dormant_at`.
- **Result:** `15 PASS Â· 0 FAIL`, `RESTORED VPN 44 is awake again`, account state
  re-read afterwards as `44,43,42` all `ACTIVE` with `dormant_at: null`.
- **A real defect the script caught on its first run:** the restore answered
  `403 csrf_blocked` and left the VPN asleep. The `SameOrigin` guard refuses a
  state-changing request from a browser-shaped caller whose `Origin` does not
  match the host the request arrived on â€” node's `fetch` sends `Sec-Fetch-*`, so
  it is treated as a browser. **The server was right.** The script now sends
  `origin: API` (the `:8080` host, not the Vite `:5173` one), which is what a
  same-origin page does.
- **Status:** `FIXED-VERIFIED` â€” gate committed and green.




---

## DEV-027 Â· Usage & Dashboard: the range selector returned ONE series for every range, and the chart labelled it with the range that was asked for

- **Reported by:** owner â€” *"usage and data ekata records godak apu gaman user
  pissu wetenawa eka hadanna kiwwa hadala nane"*, and *"vpn details page eke
  Usage eke niweradi nowana details pennanawada â€¦ fake ewa pennana epa"*.
- **Symptom:** a customer picks 24h, 7d, 30d, 3m, 1y or all and gets the same
  chart every time, captioned **"Last 30 days"** with an axis reading
  **"30d ago â€¦ now"**. On the VPN details page the same, via
  `Last {windowLabel}`.
- **Measured on the dev data before the fix** (`GET /api/vpns/42/usage`):

  | range | points | real span |
  |---|---|---|
  | 24h | 116 | 0.80 days |
  | 7d | 116 | 0.80 days |
  | 30d | 116 | 0.80 days |
  | 3m | 116 | 0.80 days |
  | 1y | 116 | 0.80 days |
  | all | 116 | 0.80 days |

  Six ranges, one identical series, six different names. That is fake data.
- **Root cause:** `usage_store.go` read
  `ORDER BY taken_at DESC LIMIT 240` â€” the **newest 240 raw rows regardless of
  the window**, so every range collapsed onto the same recent slice. The UI then
  labelled the slice with the requested range instead of the real span. An
  unreadable `limit_bytes` additionally rendered as `0 B` through a `?? 0`.
- **Fix (D-096):**
  - SQL now aggregates into fixed-width buckets chosen by the window
    (`24hâ†’hour`, `7dâ†’6h`, `30dâ†’day`, `3mâ†’week`, `1yâ†’month`; `all`/`custom`
    derive from the real span). ~24â€“40 points per chart â€” fewer rows than before.
  - The server reports `window.bucket_sec` / `window.bucket`, so an axis cannot
    be labelled from a guess.
  - The UI states the requested window **and** the real span separately; the
    hard-coded `labels={['30d ago','now']}` is removed.
  - One reading is described in a sentence, never drawn as a dot or zero line.
  - Usage blocks are fetched and committed in one pass, so the list no longer
    pops in row by row (the "records arriving one at a time" stutter).
- **Verified:** `go build`/`vet`/`test ./...` PASS (634 test functions);
  `TestUsageRangesDifferFromEachOther` is the regression test that fails if the
  ranges collapse again; `npm run verify:client-usage-range` **8 PASS Â· 0 FAIL**
  in headless Chrome. After the fix the same account reports
  `24h=hour`, `7d=6h`, `30d=day`, `3m=week`, `1y=month` and the caption reads
  *"2 readings Â· 24 hours of history Â· day per point"*.
- **Not weakened:** the `kind !== 'FREE'` chart gate was left alone. The two
  `verify:vpn-details` SKIPs remain SKIP (DEV-024).

---

## DEV-028 Â· Usage & History: no way to find one VPN among many, and every VPN was fetched whether or not it was on screen

- **Reported by:** owner â€” *"vpn godak buy kaloth adala vpn hoya ganna userta
  aberi wenawa filter fetuer eka denna"*, together with *"usage & history page eke
  ekawara godak vpn awoth user amaruwe wetnawa"*.
- **Symptom 1 â€” unfindable:** the only control was a `<select>` of every owned
  VPN keyed by **package name**. Three VPNs off one package rendered as three
  identical options, so a customer could not reach `â€¦_free_2` specifically.
- **Symptom 2 â€” expensive:** `awakeKey` was built from **every** owned VPN, so the
  page issued **two requests per VPN** no matter what was on screen. Narrowing to
  one VPN still fetched all of them: the filter changed the DOM and nothing about
  the cost. Measured on the dev account: 3 VPNs = 6 requests, and still 6 after
  filtering to one.
- **Fix (D-097):**
  - `?q=` search, URL-backed and debounced (300 ms), reusing `useListQuery`.
  - The haystack includes the **panel email** (D-091.4): `â€¦_free_2` and `â€¦_free_3`
    hold the same package and are only separable by their real name.
  - `shown` is computed first and `awakeKey` from `shown` â€” so the fetch follows
    the filter. Filtering to one VPN now issues **2** requests, not 6.
  - The source select lists only what the search left, and picking a single VPN
    clears `q` (no accidental `vpn:X AND q=nama` conjunction).
  - The empty state names the cause: "No VPN matches Â«termÂ»" vs "No usage to show
    for <source>", with the same "Show all VPNs" way back.
  - The source options now read `<package> Â· <panel email>`, so two VPNs off one
    package are distinguishable in the menu as well as on the card.
- **Verified:** `npx tsc -b` PASS Â· `npm run build` PASS Â· `lint` 3 warnings / 0
  errors (baseline held) Â· `audit` PASS Â· `audit:css` PASS (55 stylesheets) Â·
  `docs:verify` PASS. New gate `npm run verify:client-usage-search`
  **12 PASS Â· 0 FAIL**, including the assertion that cannot be faked:
  *"a filtered view reads ONLY the filtered VPN, not the whole account"*
  â†’ **2 requests for 3 owned VPNs**. `verify:client-usage-source`,
  `verify:client-usage-range`, `verify:client-dormant` and `verify:vpn-details`
  all still pass.
- **Not weakened:** no server-side search endpoint was added (the rows are already
  in the store, and the saving comes from fetching `shown`); no second filter bar
  on this screen (My VPNs owns "what do I own"); the server still owns visibility
  via `requireVPNVisible`, so a client-side filter cannot reach a refused VPN.
- **Status:** `FIXED-VERIFIED` â€” gate committed and green.


---

## DEV-029 Â· `vpn_assignments` is DELETED on purge, so a reseller's quota reopens while the panel client still exists

- **Found while auditing D-098** (the stale-VPN lifecycle). Not introduced by that
  work â€” it is in the D-027 purge path that already shipped.
- **Symptom:** `RemoveReleasedVPN` runs
  `DELETE FROM vpn_assignments WHERE vpn_instance_id = ?`. The reseller quota is
  `SELECT COUNT(*) FROM vpn_assignments WHERE seller_user_id = ? AND status =
  'ACTIVE'` (`repo.go:233`), so purging one VPN **frees a slot**. The 3X-UI client
  is deliberately **kept** (D-027.3), so the seller can now sell a slot whose
  panel object still exists â€” a **panel asset leak**. Repeat it and the reseller
  has sold more clients than the server can account for.
- **Why it is not fixed here:** changing what counts against a reseller's quota is
  a **commercial** decision (does a purged VPN stay on the seller's ledger?), and
  D-098 explicitly refuses to retro-fit it as a side effect of a tidy-up.
- **Safe alternatives, for the owner to choose:**
  - **A.** Keep the assignment row: `UPDATE vpn_assignments SET status =

---

## DEV-030 Â· VPNs had no "since when?" clock, so no lifecycle rule could exist

- **Reported by:** owner â€” *"me suspend ha revoke wachchi vpn nn mara wadayakâ€¦
  kohe ara thenaka thiyala nischitha kalayakara passe system eken sampurnayenma
  ayi karanna patta wathayakne"*.
- **Symptom:** `SUSPENDED`, `EXPIRED` and `TRAFFIC_DEPLETED` had **no timestamp
  column at all**. A VPN could have been suspended for three years and the
  database had no way to say so, so "remove after N days" was not implementable.
- **Root cause:** status was stored as a value only. `updated_at` moves on every
  unrelated edit, so counting from it would reset a VPN's age on a rename.
- **Fix (D-098, Pass 1 â€” detect and record only):**
  - `migration 0026` adds `status_changed_at` (default `0` = unknown) plus
    `idx_vpn_status_changed`.
  - Every legal transition writes it: sync status changes, block/unblock, release,
    restore.
  - `store.StaleVPNs` + `releases.ExpireStale(ctx, dryRun)` find rows past their
    window and move them to `RELEASED` â€” **never** delete.
  - `VEYLIX_LIFECYCLE_MODE` = `off` (default) Â· `report` (audit what would move,
    change nothing) Â· `enforce`. **Anything unrecognised resolves to `off`.**
- **Verified:** `go build`/`vet`/`test ./...` **PASS** (649 test functions, 17/17
  packages); `gofmt` clean; `docs:verify` **PASS** (26 migrations). New tests cover
  the dangerous cases specifically: `SERVER_UNAVAILABLE` is never swept,
  report mode changes nothing, the row is never deleted, the window boundaries
  hold, `status_changed_at = 0` is skipped, and an unknown mode fails safe.
  The migration was applied to the dev database with the sweep **off** and the
  three seeded VPNs re-read afterwards as `ACTIVE` â€” untouched.
- **Found while building it:** DEV-029 (reseller quota reopens on purge).
- **Status:** `PARTIAL by design` â€” detection and preview are complete and proven;
  acting on it is an explicit operator switch, and the quota question is DEV-029.

    'RELEASED', released_at = ?` instead of `DELETE`. The slot stays spent.
  - **B.** Keep the count but detach the pointer: leave the row and add
    `vpn_panel_email`, so quota survives without a live VPN reference.
- **Status:** `OPEN â€” decision required`. Not a regression; a pre-existing
  accounting gap that D-098 would have amplified if the sweep had been allowed to
  purge directly (it is not â€” it only moves rows to `RELEASED`).


---

## DEV-031 Â· Every screen rendered stored timestamps in the DEVICE's zone, and a date range shifted by the offset

- **Reported by:** owner â€” *"me data thiyenne utc time eken needa api srilankawe SLST
  Time eken needa enna oneâ€¦ kohoma hari oyÄ hariyata welÄwa hadala clientá¹­a niwaradi
  output eka dennako"*.
- **Storage was correct.** Every timestamp is a UTC epoch second. **No migration, no
  schema change, no data rewritten.**
- **Symptom (display):** `toLocaleString()` / `toLocaleDateString()` were called with
  no `timeZone`, so the browser's zone decided. Measured on this workspace:
  `2026-11-08T18:56Z` rendered as **00:26** in Colombo and **02:56** on the host
  (+08:00). Four consequences:
  1. The same VPN read differently on different devices.
  2. Chart bucket labels sat up to **5h30** from the bars they labelled â€” boundaries
     are built server-side, labels were not.
  3. `new Date('2026-11-01T00:00:00')` read a date input in the device zone, so
     "1st â†’ 3rd" sent a window 5h30 early and cut the end of the 3rd.
  4. Server logs said UTC while the UI said something else.
- **Fix (D-099):** `VEYLIX_DISPLAY_TIMEZONE` (default **`Asia/Colombo`**) resolved
  once at startup into `cfg.DisplayZone`; published as a zone **name** on
  `GET /api/me`; `frontend/src/lib/time.ts` owns `setActiveTimezone`,
  `activeTimeZone`, `zoneLabel` ("SLST"), `formatDateTimeWithZone` and
  **`zonedDayBounds`**; `authStore.hydrate` adopts the zone; 20 screens switched
  off the device clock.
- **A second bug, found while verifying:** the first `zonedDayBounds` measured the
  zone offset by parsing `toLocaleString('en-US')` with `new Date(...)`. That string
  is locale-shaped, so `new Date` re-read it in the DEVICE's locale and the offset
  was applied **twice** â€” "1 Nov" resolved to **08:00 on the 1st** and "3 Nov" to
  **07:59 on the 4th**. Caught by a browser probe, not by the type checker. Fixed by
  reading the zone's fields with `Intl.DateTimeFormat.formatToParts`, which is
  locale-independent.
- **Also fixed, unrelated to D-099:** `verify-client-vpns` searched the COPY
  control's `aria-label` ("Copy the panel identity â€¦") instead of the card's name,
  so it reported 0 results on correct code. It now reads the visible name.
- **Verified:** `gofmt` clean Â· `go build`/`vet`/`test ./...` **PASS** (654 tests,
  17/17) Â· `lint` 3-warning baseline Â· `build` Â· `audit` Â· `audit:css` Â·
  `docs:verify` Â· `verify:display-timezone` **13 PASS / 0 failures** Â·
  `verify:client-usage-range`, `-search`, `-source`, `-dormant`, `client-vpns`,
  `vpn-details` all **PASS**.
- **Live:** `/api/me` answers `display_timezone: "Asia/Colombo"`; measured offset
  **330 min = +05:30**; at 18:56 UTC the UI reads **09/11 00:26** where the device
  read **02:56**.
- **Residual:** `Asia/Colombo` is a **default, not a detection**. A deployment in
  another region must set `VEYLIX_DISPLAY_TIMEZONE`.
- **Status:** `FIXED-VERIFIED`.

- **Status:** `FIXED-VERIFIED` â€” gates committed and green.

---

## DEV-032 â€” A suspended or revoked VPN floated to the top of My VPNs, with no date and a chart that looked like zero usage

- **Found:** 2026-11-09, reported by the owner â€” "when a VPN is suspended or
  revoked it appears at the top of My VPNs, and I cannot use it anyway".
- **Root cause:** `MyVpnsPage`'s sort had exactly one outer comparison â€”
  `Number(a.dormantAt !== null) - Number(b.dormantAt !== null)`. Every status that
  was not dormant kept whatever position the chosen key gave it, so under the
  default **Newest first** a recently-created SUSPENDED/REVOKED row sorted to the
  very top. The customer's most prominent slot was occupied by the one VPN they
  could do nothing with.
- **Why it could not simply be hidden:** a suspended VPN *is* useless to the
  client, so deletion looks attractive â€” but D-091.5 / INV-1 require the record to
  stay visible. "Where did my VPN go?" must be answered by seeing it, never by a
  list that quietly stopped listing it.
- **Fix (D-100):**
  1. `STATUS_RANK` + `rankOf` make **status outrank the chosen sort key** â€”
     ACTIVE â†’ needs-attention â†’ customer-recoverable â†’ not-in-service â†’ dormant.
     Within each rank the customer's chosen sort still applies in full.
  2. A second `ClientSection` â€” **"Not in service"** â€” holds SUSPENDED, REVOKED,
     RELEASED and REMOVED_FROM_CATALOG with a sentence explaining that the rows are
     still theirs and still on file.
  3. `status_changed_at` is now projected onto `GET /api/vpns` (D-098's column,
     previously internal only) so a row can say **since when**.
  4. `VpnUsageSection` states, when the newest reading predates the status change:
     *"Counters stopped on âŸ¨dateâŸ© (âŸ¨zoneâŸ©) â€” â€¦ the flat line after that point is an
     absence of readings, not zero usage."* Snapshots are cumulative counters, so a
     suspended VPN's chart goes flat; a flat line reads as "no traffic" unless the
     screen says which absence it is. **No backfill, no notification.**
- **Deliberately not changed:** the Overview counts still count **every** owned VPN
  (D-088) â€” "Total VPNs" describes the account, not a section. `domain.VPNRevoked`
  stays **terminal**: revoked is not resumable, so no transition was invented.
  Nothing was deleted or purged.
- **Also in this pass (D-101):** the sidebar footer now carries the build stamp â€”
  `v<version>` read from `GET /api/system/version` (the RUNNING binary, not the
  bundle) plus a source link. The repo URL is a **build-time constant** injected by
  `vite.config.ts` (`__VEYLIX_REPO_URL__`, default
  `https://github.com/janith-15/Veylix-1`), **not** a `.env` value, because
  `frontend/.env` is git-ignored here and a value living only there would vanish in
  every deployment built from the repo. Unreadable version â†’ no line; empty URL â†’
  no link; never a placeholder. Rail mode falls back to tooltips. The link is
  `target="_blank" rel="noopener noreferrer"`.
- **Evidence â€” every command run, with its real result:**
  - `gofmt -l .` â†’ no output. `go build ./...` â†’ clean. `go vet ./...` â†’ clean.
  - `go test ./... -count=1` â†’ **17/17 packages `ok`**, including
    `internal/httpapi` 118.7 s and `internal/store` 34.7 s. **655** test functions
    (was 654; +1 = `TestVpnListPublishesStatusChangedAt`).
  - New regression test `TestVpnListPublishesStatusChangedAt`
    (`backend/internal/httpapi/vpn_status_changed_routes_test.go`) asserts both
    halves: a suspended row publishes its stored clock unchanged, and a row that
    never moved publishes **null** â€” not `0`, which a screen would render as
    "1 January 1970". **PASS**.
  - `npx tsc -b` â†’ clean. `npm run build` â†’ `built in 1.60s`.
  - `npm run lint` â†’ **3 warnings, 0 errors** (unchanged baseline).
  - `npm run audit` â†’ *"UI truth audit: OK"*. `audit:selftest` â†’ 8 positive + 6
    negative. `audit:css` â†’ *"55 stylesheets parsed, no truncation found"*.
  - `npm run docs:verify` â†’ first run **FAILED** (`README says 654, tree has 655`),
    then **PASS** after the README was corrected. Recorded rather than smoothed
    over: the gate caught real drift and it was fixed, not bypassed.
  - `.\dev.ps1 -Preflight` â†’ **PREFLIGHT OK â€” 33 checks passed** (roles, doors,
    refusals, logout, proxy).
  - `npm run verify:client-vpns` â†’ **25 PASS / 0 failures**, now including the new
    D-100 and D-101 assertions: an account with nothing suspended renders no empty
    section; no fabricated 1970 date; the footer carries `vdev`, links to
    `https://github.com/janith-15/Veylix-1`, and uses
    `target="_blank" rel="noopener noreferrer"`.
  - `verify:vpn-details`, `verify:client-dormant`, `verify:display-timezone`,
    `verify:client-usage-range`, `-search`, `-source` â†’ all **PASS / 0 failures**.
  - `go test -race` â†’ **BLOCKED** (no C compiler on this machine), reported as
    blocked, not as pass.
- **Residual, stated rather than hidden:**
  - The seeded client account owns three VPNs that have **never changed status**,
    so the "Not in service" section and the `since` label have **no live row to
    render**. Their code paths are proven by the backend test (the API field) and
    by the gate's negative assertions (no section on a clean account, no 1970
    date), **not** by a live screenshot. Producing a live suspended row would mean
    mutating the seeded panel client, which the data-safety rule forbids.
  - The usage caption fires only when a VPN's status change is **newer** than its
    newest reading. On the seeded data that ordering does not occur, so
    `verify:vpn-details` reports its chart checks as **SKIP** for a FREE VPN â€” an
    honest skip, not a pass.
  - D-100 does **not** add a notification when a VPN is suspended. The screen
    explains the state when the customer looks; it does not go and tell them.
- **Status:** `FIXED-VERIFIED` for the API field, the ordering rule, the section,
  the footer and the gates. `PARTIAL` for a live rendered suspended row â€” see
  residual above.


---

## DEV-033 â€” The Super Admin could not do the Super Admin's job, and the console rendered 373 inline styles

**Date:** 2026-11-09
**Scope:** `internal/auth`, `internal/store`, `internal/provisioning`,
`internal/releases`, `internal/panel`, `internal/httpapi`, `internal/audit`,
`internal/store/migrations`, `frontend/src/{lib,pages,stores,components}`, `README.md`
**Decisions:** D-102, D-103, D-104, D-105, D-106
**Plan file:** `docs/new/SUPER_ADMIN_PERFECTION_PLAN.md` â€” a temporary working file,
**deleted after every phase was proven** (Phase 10). This ledger entry is the
permanent record; do not look for the plan file.

### The owner's instruction

*"super admin side ekath full audit karanna â€¦ hema page ekakma wage redesing wenna
one â€¦ super adminta hema deyakma controle karanna puluwan wenna one â€¦ kisima
adupaduwak thiyanna epa â€¦ 100% perfect"*

### What was actually wrong

Ten defects, each confirmed by reading the source **and** by a command that failed:

| ID | Defect | How it was proven |
|---|---|---|
| P-1 | A second `super_admin` could be minted | `CreateUserByRole`'s own comment admitted it; no constraint anywhere |
| P-2 | The Super Admin could not release a VPN | `release_routes.go:42` â†’ 403 for every non-client |
| P-3 | A record could not be retired while its panel client lived | `provisioning/service.go:1073` â†’ `ErrVPNStillOnPanel`, with no alternative path |
| P-4 | Offers had no archive and no delete | `store/offers.go` had only Create/Update |
| B-1 | Clients pager divided by the page **number** | `verify-clients.mjs` saw 6 pages for 6 clients |
| B-2 | Offset derived from the **unclamped** limit | `?limit=9999&page=5` â†’ `LIMIT 50 OFFSET 39996` |
| B-3 | CSV export allowed formula injection | `=`, `+`, `-`, `@` passed through |
| B-4 | DNS-rebinding window between check and dial | no `Dialer.Control` |
| B-5 | The audit log was **not** append-only | a test ran `UPDATE audit_logs` and it succeeded |
| B-6 | A nav gate failed on correct code | `73 passed, 1 failed` |

### Three defects found only by building the gate

None of these were in the original audit. Each was found because a measurement was
added **before** a fix:

1. **`Table` emitted an inline style per CELL.** A source grep counted 30 inline
   styles across the console; the browser measured **373**, because thirteen shared
   components were doing it internally. `Table` alone put `textAlign` on every cell
   of every table in the product.
2. **`StaffPage` imported the wrong stylesheet.** It used `cl-quotabar` classes but
   imported `clients.css`; those rules live in `admins.css`. The quota meter rendered
   **completely unstyled** â€” 19/20 slots looked identical to 1/20.
3. **`errors.As(err, new(*T))` in a `switch` case silently discards the match.** The
   purge route answered **500** for a refusal that should have been a **422** naming
   the string to type. A named helper fixed it.

### Two latent security findings, surfaced by the B-4 fix

- **`allowPrivate` did not lift loopback**, so every panel behind an SSH tunnel â€” a
  normal self-hosted deployment â€” would have been refused on every call.
- **`169.254.169.254` was reachable under `allowPrivate`.** The comment claimed it
  was "covered by link-local, kept explicit", but it was NOT in the explicit list,
  and the private policy lifted the link-local rule. Relaxing the policy for a
  private panel silently opened cloud IMDS. The metadata list is now explicit (IMDS,
  ECS, Alibaba, IPv6 IMDS) and is checked FIRST.


### What was built

- `0027_audit_append_only.sql` â€” `BEFORE UPDATE`/`BEFORE DELETE` triggers.
- `0028_single_super_admin.sql` â€” partial unique index on `role = 'super_admin'`.
- `POST /api/vpns/{id}/purge` â€” retire a record, keep the panel client.
- `POST /api/offers/{id}/archive` â€” retire an offer, keep the row.
- `DELETE /api/offers/{id}` â€” remove an offer nothing depends on.
- `vpn.force_release` Â· `vpn.force_purge` Â· `offer.archive` Â· `offer.delete` audit
  actions, each carrying identity by value.
- `DeleteOffer`'s explicit usage guard over BOTH `ON DELETE SET NULL` tables;
  `OfferRow.ArchivedAt` + `archived_at` in `offerColumns`.
- Exported page-size bounds (`ClientAdminDefaultLimit/Max`, `StaffDefaultLimit/Max`)
  so transport and store cannot derive different windows.
- `lib/csv.ts` â€” the CSV escaper, extracted so it can be tested.
- `verify-super-admin.mjs` â€” a real gate over all 14 console pages.
- `csv-selftest.mjs` â€” 14 assertions on the escaper.
- `staff_admin_test.go` â€” the staff console's first tests (5 tests, 17 sub-cases).
- Console controls: archive/delete on offers; release-with-reason and
  type-to-confirm purge on VPNs; a staff pager.
- `Table` alignment moved from a style prop on every cell to `data-align` + CSS.

### Evidence

| Gate | Result |
|---|---|
| `gofmt -l .` Â· `go build ./...` Â· `go vet ./...` | clean Â· exit 0 Â· clean |
| `go test ./... -count=1` | **17/17 packages ok, zero FAIL** (671 test functions, +16) |
| `npm run lint` | 3 warnings, 0 errors (**unchanged**) |
| `npm run build` | built |
| `npm run audit` Â· `audit:selftest` Â· `audit:css` | OK Â· OK Â· OK |
| `npm run audit:csv-selftest` | **14 passed, 0 failed** |
| `npm run verify:super-admin` | **112 passed, 0 failed** (373 inline styles â†’ **0**) |
| `npm run verify:nav` | **74 passed, 0 failed** (was 73/1) |
| `npm run verify:clients` | **PASS** â€” pager bounded |
| `npm run docs:verify` | **OK** â€” 28 migrations Â· 141 routes Â· 671 test functions |
| `.\dev.ps1 -Check` | **PREFLIGHT OK â€” 33 checks passed** |

### Blocked

- **`go test -race`** â€” `-race requires cgo`; there is no C compiler on this machine.
  Reported BLOCKED, never PASS.
- **`.\dev.ps1 -Preflight`** â€” port 8080 was already serving. `-Check` ran the same
  gate against the running server rather than stopping a live process.

### Residual

- **Performance (QA area 9) is NOT RUN.** No benchmark moved, so no claim is made.
- **`verify:ui` is report-only.** It prints a DOM dump and exits without a pass/fail
  line, so it is not counted as a browser gate.
- **A purge has no undo.** A purged record is gone from Veylix permanently; the panel
  client survives, which is what the customer needs, but the *record* cannot be
  recovered. Deliberate: the type-to-confirm is the guard, not an undo.
- **`Dropdown`'s portal is not covered by the inline-style gate.** Its menu is
  portalled to `<body>` and only exists while open, so `verify:super-admin` cannot see
  it. It was converted to custom properties by hand (D-106), and it will not be caught
  if it regresses. A gate for it needs an assertion that opens a menu.
- **`TestDemotingTheOwnerIsAllowedOnceOnly` is weaker than its name.** It proves a
  non-owner cannot demote the owner; it cannot prove a *handover*, because a Super
  Admin cannot demote themselves (`ErrCannotModifySelf`) and there is deliberately no
  second Super Admin to do it. The handover path is therefore **untested end to end**.

### Status

`FIXED-VERIFIED` for all ten defects, the two latent security findings and the three
gate-discovered defects. `PARTIAL` for performance (not measured), for the two
blocked gates, and for the Super Admin handover path (residual above).

---

## DEV-034 Â· Super Admin overview rebuilt around the business question (D-107)

**Date:** 2026-11-10 Â· **Plan file:** `docs/new/SUPER_ADMIN_OVERVIEW_PLAN.md` â€” a
temporary working file, **deleted in Phase 10**. This entry is the permanent record.

### What was wrong

The dashboard was technically clean and business-useless. It passed `verify:super-admin`
at 112/0, had zero inline styles and zero fabricated data â€” and it **could not tell the
owner whether the business was alive**. No revenue appeared anywhere in the product;
`grep 'SUM(amount_cents)' internal/store` returned **0 matches**. Two of the four KPI
cards showed the same ratio (`activeServers/totalServers`) under different names, and
two `delta` strings were developer notes (`'from /api/clients'`, `'N total records'`)
rendered as if they were insights.

### What was built

| Layer | Change |
|---|---|
| `store/overview_summary.go` | `PaymentTotals` + `ResellerHealth` + `paymentMonths`, `MonthStartUnix`, `MonthEndUnix`. Three queries, each aimed at an existing index |
| `store/migrations/0029_overview_indexes.sql` | `idx_pay_confirmed` â€” a **read-ahead**, documented as such |
| `httpapi/overview_routes.go` | `GET /api/super/overview/summary`, Super Admin only, section-level failure reporting |
| `httpapi/router.go` | wired on `deps.Clients.(OverviewStore)` |
| `lib/api.ts` | `ApiSuperOverview`, `ApiPaymentTotals`, `ApiResellerHealth`, `superOverview()` |
| `pages/DashboardPage.tsx` | `MoneyHero`, `MoneyMini`, `ResellerSection`; eight P1 fixes; two new queue rows |
| `pages/dashboard.css` | `.dash-business` 3fr/2fr, `.dash-money*`, `.dash-resellers*`, stale freshness state |
| `scripts/verify-super-admin.mjs` | 10 rendered-text assertions (D-107 block) |

### Decisions taken inside this entry (recorded in D-107)

Revenue = **NET**. Month = **calendar month**. Multi-currency = **never summed**
(`mixed_currency` + zeroed amounts + counts still exact). A failed section is `null`
and named in `errors`, never `0`.

### Evidence

| Gate | Result |
|---|---|
| `gofmt -l .` | clean |
| `go build ./...` Â· `go vet ./...` | **0** / **0** |
| `go test ./... -count=1` | **17/17 packages ok, 0 FAIL** â€” 682 test functions (+11) |
| New store tests | `TestPaymentTotalsNetRevenue`, `â€¦IgnoresUndatedConfirmations`, `â€¦RefusesToSumCurrencies`, `â€¦OnAnEmptyDatabase`, `TestOverviewQueriesUseIndexes`, `TestResellerHealthCountsTheThingsOnlyAnOwnerCanFix` |
| New route tests | `TestOverviewRefusesResellersAndClients` (client/admin 403 **and store never read**, super admin 200), `â€¦AnonymousIs401`, `â€¦ReturnsBothSections`, `â€¦NamesAFailedSectionInsteadOfBlanking`, `â€¦WithoutAStoreSays503` |
| `npm run lint` | 3 warnings / 0 errors â€” **unchanged from baseline** |
| `npm run build` | âœ“ built |
| `audit` Â· `audit:selftest` Â· `audit:css` Â· `audit:csv-selftest` | OK Â· 8+6 Â· 55 sheets Â· 14/14 |
| `npm run verify:super-admin` | **122 passed, 0 failed** (was 112 â€” +10) |
| `verify:nav` Â· `verify:clients` | 74/0 Â· PASS |
| `npm run docs:verify` | OK â€” 29 migrations Â· 142 routes Â· 682 tests |
| `.\dev.ps1 -Check` | **PREFLIGHT OK â€” 33 checks passed** |

### The gate was proven to bite

Reverting the `Panel health` label to the old `Healthy` produced
`FAIL  D-107 fourth tile is named for what it measures  -> Panels, Clients, Active VPNs, Healthy`
and `SUPER ADMIN FAILED â€” 117 passed, 3 failed`, exit code 1. Restored, and 122/0
returned. A gate that cannot fail is not a gate.

### Two things found by building it, not by reading it

1. **`payment totals by status` performs a full SCAN.** Grouping by `currency` with no
   `WHERE` clause reads the whole table. That is unavoidable for an all-time aggregate
   (every row must be read once), so the honest response is to document it and index
   the *windowed* queries â€” which is exactly what `0029` does. Recorded rather than
   hidden: the alternative was an index that would never be used.
2. **The month queries already used `idx_pay_status`.** `EXPLAIN QUERY PLAN` proved it,
   so `0029` was added as a read-ahead and its header says so. A migration that claims
   to fix something it does not fix is the same lie as a UI that claims a number it
   cannot support.

### Residual, stated plainly

- **Performance (QA area 9) is `PARTIAL`.** The query plans were verified; **no
  wall-clock benchmark was run**, so no timing claim is made.
- **`go test -race` remains `BLOCKED`** â€” no C compiler on this machine.
- **Only the store and route layers are unit-tested.** No component-level test exists
  for `MoneyHero` / `ResellerSection` (D10 â€” no frontend test runner). The proof is the
  page-level browser gate, which asserts rendered text.
- **`resellers.active` means "signed in within 30 days"**, not "has an active
  subscription". The label in the UI says *signed in recently* rather than "active" so
  the two cannot be confused.

### Status

`FIXED-VERIFIED` for the overview route, the eight P1 defects and the gate. `PARTIAL`
for performance (plans verified, timings not measured) and for component unit tests
(no runner). `BLOCKED` for `-race`.

