# VEYLIX — MILESTONE 7: PACKAGES, ORDERS AND THE PREMIUM VPN PAGE

**Created:** 2026-10-25
**Governing decisions:** **D-034 … D-038** (panel identity · package kinds · order/payment · assignment · Premium feature set)
**Preceded by:** `docs/MILESTONE_6_PLAN.md` (D-030 … D-033) — plan ownership, payment = release, Free/Premium split, QR.
**Rule:** every item below is `[ ]` until a command proves it. Nothing is ticked because the code exists (`new_update.md` §13).

---

## 0. The owner's requirements (verbatim intent)

> *"client ta load wenna one eyata adala vpn witharayi. anika super adminta server eka click karama e server ekata adala vpn list eka enna one — eken free ewa keeyada kata katada deela thiyenne, usage kohomda iwara wela thiyenne … primium ewa template hadanna one na. premium wala client ta adminla laga thiyena premium offersuth pennanna one … client order card ekak select karama … 200gb duration dawas keeyakatada, device keeyakatada, ip keeyada kiyana eka eka vpn ekak … price eka calculate wela pennanna one … order eka ebuwama super adminta order eka yanna one … payment methord tika pennana one … gmail eka whatsapp telegram number tika pennanna one super adminge … payment pending kiyala status wela … client ta payment methord eka maru karannath puluwan wenna one … super admin confirm kalalma vpn eka on weyi … payment enna kalin confirm button eka ebunoth one welawaka vpn eka block karanna super adminta chance eka thiyenna one … super adminge peththen 1gb ekaka price ekaka, device 1 ka price eka, ip 1 ka price eka set karanna puluwan wenna one."*

> *"free package, premium package, custom premium package … custom premium wala client ta one widihata thora gena VPN ekak order karanna puluwan … custom eka super adminta witharayi enne. adminlata custom premium kiyala ekak na … order eka super admin ta — vpn server ekak select karala e server ekatama package eka client ta release karanna puluwan … e hadana welawe order eke order eka damu client ge namin thama. 3x ui client eke email eka rename wenna one."*

---

## 1. The three packages, at a glance

```
┌───────────────┬────────────────────────┬──────────────────────────┬──────────────────┐
│ KIND          │ WHO CREATES IT         │ WHO CONFIRMS THE ORDER   │ LIMITS            │
├───────────────┼────────────────────────┼──────────────────────────┼──────────────────┤
│ FREE          │ Super Admin            │ nobody — instant claim   │ platform policy  │
│ PREMIUM       │ SA **or** a reseller   │ the package's OWNER      │ fixed (template) │
│ CUSTOM PREM.  │ Super Admin only       │ SUPER ADMIN only         │ customer spec    │
└───────────────┴────────────────────────┴──────────────────────────┴──────────────────┘
```

Panel identity for every VPN (D-034):

```
nimal@gmail.com_free_1      nimal@gmail.com_free_2
nimal@gmail.com_paid_1      nimal@gmail.com_paid_2      nimal@gmail.com_paid_3
```

---

## 2. Phase G — Server drill-down, assignment, and the client's own list

> Owner's first requirement: *"server eka click karama e server ekata adala VPN list eka … free ewa keeyada kata katada deela thiyenne, usage kohomda iwara wela thiyenne … mewa hodata filter karaganna puluwan wenna thiyenna one."*

| # | Item | Where | Test |
|---|---|---|---|
| G1 | `PanelClientView` gains the **owner** and **usage** facts: `vpn_instance_id`, `owner_user_id`, `owner_email`, `owner_name`, `seller_user_id`, `offer_id`, `offer_kind`, `used_bytes`, `limit_bytes`, `usage_pct`, `days_left`, `protocol`, `transport` | `internal/integrations/service.go` | `TestServerClientListShowsBuyer` |
| G2 | `GET /api/servers/{id}/panel-clients` accepts `?tag=&assigned=&usage=&q=` and filters server-side | `internal/httpapi/integration_routes.go` | `TestServerClientFilters` |
| G3 | Summary counts: **free / premium / assigned / unassigned / exhausted / expiring** | same | `TestServerClientSummary` |
| G4 | `GET /api/vpns` rows carry **`owner_email` + `owner_name` + `assigned`** so "whose VPN" is answerable on the wire (D-037.4) | `internal/httpapi/vpn_lister*.go` | `TestSQLVPNListerClientSeesOwnVPNs` (asserts `OwnerEmail`/`Assigned` on every owned row; the originally-named `TestVPNRowCarriesOwner` was never written) |
| G5 | **A client's own untagged VPN must not disappear** — the tag join cannot hide it | `internal/httpapi/vpn_lister.go` | `TestSQLVPNListerClientSeesOwnVPNs` (name corrected 2026-10-25; `TestClientSeesOwnVPNEvenUntagged` was never written) |
| G6 | **Assign an unassigned panel client to a client account** (`POST /api/panel-clients/{server}/{email}/assign`, SA only, audited), optional rename (D-034.5) | new route | `TestAssignUnassignedClientToUser` |
| G7 | UI **`/servers/:id`**: KPI strip · tabs *Assigned / Unassigned / All* · filters · per-row *Assign* · usage bar | `frontend/src/pages/ServerDetailPage.tsx` | `npm run build` |

---

## 3. Phase H — Rates, packages and the price quote

| # | Item | Where | Test |
|---|---|---|---|
| H1 | Migration `0009_packages_orders.sql`: `pricing_rates` (global + per-server), `vpn_packages`, `payment_records` spec/server/inbound columns | `store/migrations/` | `TestSchema0009` |
| H2 | `GET/PUT /api/pricing` — SA sets **per-GB, per-device, per-IP, per-day**, global + per-server (D-035.4) | new `pricing_routes.go` | `TestQuoteFromRates`, `TestServerRateOverridesGlobal` |
| H3 | `POST /api/orders/quote` — spec in → **server-computed price** (the client never states an amount) | new | `TestQuoteRejectsOutOfRange` |
| H4 | `GET /api/packages` — role-scoped: a client sees FREE + **every** PREMIUM (SA's and resellers'), plus the CUSTOM template | `plan_routes.go` | `TestClientSeesAllPremiumPackages` |
| H5 | A reseller **cannot** own or create a CUSTOM package (D-035.2) | `plan_routes.go` | `TestResellerCannotOwnCustom` |


---

## 4. Phase I — Orders, payment channels, block/unblock

| # | Item | Where | Test |
|---|---|---|---|
| I1 | Payment method carries **e-mail / WhatsApp / Telegram** (D-036.2) | `store/payments.go` + `0009` | `TestPaymentMethodExposesChannels` |
| I2 | `POST /api/payments/{id}/method` — payer changes destination; immutable event (D-036.4) | new | `TestMethodChangeIsRecorded` |
| I3 | `GET /api/payments/{id}/events` — the timeline both sides see | new | `TestOrderTimeline` (renamed 2026-10-25 from `TestOrderMethodChangeOverHTTP`, which is kept as a forwarder) |
| I4 | `POST /api/payments/{id}/block` · `/unblock` — owner blocks the VPN when money does not arrive (D-036.5) | new | `TestBlockAndUnblockVPN` |
| I5 | A `CUSTOM PREMIUM` release needs **server + inbound** picked by the SA (D-036.6) | `payments/service.go` | `TestCustomOrderNeedsServerBeforeRelease` |
| I6 | The order names **who must confirm** (`confirmer_user_id`, `confirmer_email`) | `store/payments.go` | `TestOrderNamesTheConfirmer` |

---

## 5. Phase J — Panel identity (D-034)

| # | Item | Test |
|---|---|---|
| J1 | Panel email → `<client_email>_<kind>_<slot>`, slot reserved **inside** the provisioning transaction | `TestPanelEmailIsClientAndKind` |
| J2 | Slots only move forward after a revoke (D-034.2) | `TestPanelSlotsOnlyMoveForward` |
| J3 | `panel_sub_id` stays `vyx-<id>` so D-012 idempotency survives | `TestPanelSubIdStaysDeterministic` |
| J4 | Rename once on assignment (D-034.5), audited | `TestAssignRenamesPanelClient` (written 2026-10-25) |

---

## 6. Phase K — The Premium VPN page (D-038, P-01 … P-10)

| ID | Feature | Backend | UI | Test |
|---|---|---|---|---|
| P-01 | Usage create→end, **up / down / total separate** | `total_bytes` on usage | timeline chart | `TestUsageHistorySplitsUpDownTotal` |
| P-02 | Usage vs capacity | `limit_bytes`/`usage_pct` | progress ring | `TestVPNUsageAgainstCapacity` (written 2026-10-25) |
| P-03 | Server live CPU / RAM | `GET /api/servers/{id}/metrics` (exists) | gauge cards | existing |
| P-04 | Live traffic rate "↑ 2 kbps / ↓ 40 kB/s" | `GET /api/vpns/{id}/traffic/live` | live chips | `TestLiveTrafficRate` |
| P-05 | **Client-side ping** (browser → VPN server) | browser handshake, scope labelled | *Test ping* button | **NOT BUILT** (M7-17): the page labels its ping "server reachability, not your own latency", so the gate test is absent - stated, not skipped. |
| P-06 | 7d / 30d / 3m / 1y | range parameter | range switch | `TestUsageRangeWindow` |
| P-07 | CPU / RAM / network charts, each switchable | metrics + net rate | chart tabs | — |
| P-08 | pie / line / bar / area switch | — | chart-type switch | `npm run build` |
| P-09 | **vless/vmess · TCP/WS · uuid/username/password** | `GET /api/vpns/{id}/protocol` (panel truth only) | detail block — **built** (`ProtocolDetail.tsx`, credential masked until revealed) | `TestProtocolDetailIsExposed` + `verify:vpn` |
| P-10 | **QR + link, always** | links (exists) | in-repo QR encoder — **built**, and its cap raised to 666 bytes after a real 442-byte link was refused (M7-21) | `npm run qr:selftest` (10 cases) + `verify:vpn` |

---

## 7. Phase L — Surfaces

| # | Screen | Role | Contents |
|---|---|---|---|
| L1 | `/buy` | client | three package kinds: FREE claim · PREMIUM package · CUSTOM spec → live price → method (channels) → confirm |
| L2 | `/orders` | SA (+ admin for own packages) | pending queue: buyer, package, spec, amount, method, channels, **timeline**; **Confirm** (VPN + QR) · **Reject** · **Block / Unblock** |
| L3 | `/orders/:id` (custom) | SA only | **server picker → inbound picker → Release to client** |
| L4 | `/pricing` | SA | per-GB / per-device / per-IP / per-day, global + per-server override |
| L5 | `/packages` | SA / admin | package CRUD (the old `/offers` screen, renamed) |
| L6 | `/vpn/:id` | all | the P-01 … P-10 page |
| L7 | `/servers/:id` | SA | Phase G drill-down + assignment |

---

## 8. Definition of done

| Gate | Requirement |
|---|---|
| Backend | `go build` · `vet` · `gofmt` · `go test ./... -count=1` green, every named test present |
| Frontend | `npm run lint` 0/0 · `npm run build` · `npm run audit` 0 findings |
| Money | order → owner confirm → VPN + QR in one flow, proven by test and a browser walkthrough |
| Privacy | no client can see another client's VPN — proven in SQL and by test |
| Docs | `DECISIONS.md` D-034…D-038 · this file · `IMPROVEMENT_LEDGER.md` M7-xx · `API_CONTRACT.md` |

---

## 9. Progress log (append-only)

| Date | Stage | Result |
|---|---|---|
| 2026-10-25 | Plan recorded | D-034 … D-038 written from the owner's answers (identity, packages, orders, assignment, Premium page). Phases G … L scoped. |
| 2026-10-25 | **G1/G2/G3 done** | The server drill-down answers **whose VPN it is** (owner id/e-mail/name, seller, offer, `vpn_instance_id`) and **how far it has come** (`used_bytes`, `limit_bytes`, `usage_pct`, `days_left`) beside the panel's own limits. Filters: `tag`, `assigned`, `usage`, `q` — server-side, 422 on a typo. Summary: total · free · premium · untagged · assigned · unassigned · exhausted · expiring · expired · disabled. Tests `TestServerClientListShowsBuyer`, `TestServerClientSummary`, `TestServerClientFilters`. |
| 2026-10-25 | **G4 + G5 done** | **M7-04 fixed:** `SQLVPNLister` no longer inner-joins `panel_client_tags`, so a client sees every VPN it owns (tagged or not) and never another account's — privacy is an ownership rule. `VPNRow` carries `owner_email`, `owner_name`, `assigned`, `offer_id/kind/name` in both listers. Tests `TestSQLVPNListerClientSeesOwnVPNs`, `TestSQLVPNListerAdminNeverSeesUntagged`. |
| 2026-10-25 | **G6 done (backend)** | `POST /api/panel-clients/{server_id}/{email}/assign` (Super Admin): creates the VPN row, locks the tag and names it per D-034, then renames the panel client and confirms with a fresh read. A rename failure answers `502 rename_failed` — never a silent success. New `0009_panel_assignments.sql` remembers the source name, so a second assignment of the same panel row is refused (`409 panel_client_taken`) instead of duplicated. Naming helpers (`PanelBase`, `PanelSlotNext`, `PanelFindNextFreeSlot`) live in `internal/domain`. Tests: `TestAssignPanelClientRoute`, `TestAssignRouteRefusesNonClient`, `TestAssignRouteReportsRenameFailure`, `TestAssignRouteRefusesDoubleAssign`, plus store tests `TestAssignUnassignedClientToUser`, `TestAssignRefusesAnAlreadyHeldPanelClient`, `TestAssignRefusesNonClientTarget`, `TestAssignSlotsMoveForward`, `TestAssignIsPerCustomer`. |
| 2026-10-25 | **G7 next** | UI `/servers/:id` — KPI strip, Assigned/Unassigned/All tabs, filters, per-row *Assign*. |
| 2026-10-25 | **H1 + H2 + H3 done** | `0010_pricing_packages.sql`: `pricing_rates` (platform default + per-server override), `vpn_offers.is_custom`, `payment_records.spec_gb/days/devices/ips`, `release_server_id`, `release_inbound_id`. `domain.QuoteSpec` prices a spec with integer cents (owner's example: 200 GB/30 d/3 dev/2 IP = **LKR 259.00**), applies a floor and reports it, and **refuses** rather than clamps on an out-of-range spec, a missing rate card, or a ceiling breach. `GET/PUT /api/pricing` (operator only, audited), `POST /api/orders/quote` (server-computed amount — the client never sends one). Tests: `TestQuoteFromRates`, `TestQuoteAppliesMinimum`, `TestQuoteRejectsOutOfRange`, `TestQuoteRefusesWhenNoRatesSet`, `TestQuoteRefusesAboveCeiling`, `TestPricingFallsBackToPlatformDefault`, `TestServerRateOverridesGlobal`, `TestSavePricingIsUpsert`, `TestPricingSeededRowIsZeroNotFake`, `TestPricingMissingScopeIsAnError`, `TestQuoteFromRatesOverHTTP`, `TestQuoteRefusesOutOfRangeOverHTTP`, `TestQuoteRefusesBeforeRatesExist`, `TestPricingReadRequiresOperator`. |
| 2026-10-25 | **H4 + H5 done** | `GET /api/packages` returns the three kinds in one role-scoped answer: **FREE**, **PREMIUM** (every active package, whoever owns it) and **CUSTOM** (marked `priced_per_order: true`, because its amount comes from `/api/orders/quote`, never from the row). Facts only — `owner_user_id`, `slots_used`, `slots_total` are never on the wire for a customer. `POST /api/plans` gains `is_custom`, refused for a reseller (`403`) and refused as FREE (`422`): a custom package is always PREMIUM and always the platform's. `Plan` carries `is_custom`. Tests: `TestClientSeesAllPremiumPackages`, `TestResellerCannotOwnCustom`. |
| 2026-10-25 | **Phase H complete** | Rate card (global + per-server), server-computed quote, three-kind catalogue, CUSTOM guard. |
| 2026-10-25 | **I1–I4 done** | `0011_order_events.sql`: `payment_methods.contact_{email,whatsapp,telegram}`, `payment_record_events` (an append-only timeline), `payment_records.proof_channel/proof_note`. Store: `ChangePaymentMethod` (payer-only, open orders only, keeps BOTH ends of the move with their labels, no-op when unchanged), `RecordProofChannel` (WhatsApp/Telegram/e-mail only, moves the order to AWAITING_CONFIRMATION), `BlockOrderVPN`/`UnblockOrderVPN` (refused with `409 no_vpn_yet` when there is nothing to stop). Routes: `POST /api/payments/{id}/method`, `/proof`, `GET …/events`, `POST …/block`, `/unblock` — the **panel is written first and confirmed** (`provisioning.BlockVPN`), so a local SUSPENDED row can never disagree with the panel. Tests: `TestMethodChangeIsRecorded`, `TestMethodChangeToSameIsNoop`, `TestMethodChangeIsPayerOnly`, `TestProofChannelIsRecorded`, `TestBlockAndUnblockVPN`, `TestOrderMethodChangeOverHTTP`, `TestOrderProofChannelOverHTTP`, `TestPaymentMethodExposesChannels`, `TestBlockWithoutVPNIsRefused`. |
| 2026-10-25 | **I5 + I6 done** | A CUSTOM order now releases **its own spec** onto the **chosen** panel. `payment_records` carries `spec_gb/days/devices/ips`, `release_server_id`, `release_inbound_id` and `confirmer_user_id`; `PaymentRow.IsCustomSpec()` makes "is this a custom order" answerable. `store.SetReleaseTarget` (server must exist, inbound must belong to it, refused once released, audited as a `RELEASE_TARGET` event) + `POST /api/payments/{id}/release-target` (Super Admin only). `payments.Confirm` refuses a custom order with `ErrReleaseTargetRequired` **before reaching provisioning**, and otherwise passes `CreateInput.Spec`. `provisioning` applies the spec over the package's limits and target (traffic/days/devices/IPs/server/inbound) and refuses a half-stated one (`ErrSpecIncomplete`). Tests: `TestCustomOrderNeedsServerBeforeRelease`, `TestCustomOrderReleasesWithItsOwnSpec`, `TestReleaseTargetMustMatchServer`, `TestOrderNamesTheConfirmer`, `TestSpecOverridesPackageLimits`, `TestSpecIncompleteIsRefused`. |
| 2026-10-25 | **Phase I complete** | Next: **Phase J** — panel identity (`<client email>_<kind>_<n>`) wired into provisioning. |
| 2026-10-25 | **J1–J3 done + M7-13 fixed** | A new VPN is named `<client email>_<kind>_<n>` (`nimal@gmail.com_free_1`, `…_paid_2`) instead of `vyx-<id>`: `nextPanelEmail` reads every name the server already uses (rows + tags) and reserves the first free slot **inside the create transaction**, so two concurrent creates cannot collide. `panelKind` puts FREE plans on the `free` counter and everything else (PREMIUM package, CUSTOM order) on `paid`. `panel_sub_id` stays `vyx-<row id>`, so D-012's idempotency pair survives. **Real defect found by the tests:** `store.SetClientTag` only *refused* re-tagging a locked row — it never locked a VPN tagged **after** issue, so an issued VPN could still be re-priced. It now locks on write when a live VPN row exists (`ErrTagLocked` on any later change). Tests: `TestPanelEmailIsClientAndKind`, `TestPanelSlotsOnlyMoveForward`, `TestPanelSubIdStaysDeterministic`, `TestIssuingVPNLocksItsTag`. |
| 2026-10-25 | **Phase J complete** | Next: **Phase K** — the Premium VPN page (P-01 … P-10). |
| 2026-10-25 | **K: P-01, P-02, P-06 done (backend)** | `GET /api/vpns/{id}/usage` now answers the three numbers the owner asked for — `lifetime_up_bytes`, `lifetime_down_bytes` and **`lifetime_total_bytes`** — plus `usage_pct` (clamped at 100, so a bar never reads 104%), `remaining_bytes` (`-1` when unlimited, never "0 left"), `created_at`/`expires_at` (the window the owner described as "from the creation day to the end day"), and an echoed **`window`** object. `?range=7d\|30d\|3m\|1y\|all` implements the switch; an unknown value answers **422** instead of silently showing a different window. Every point also carries `total_bytes` and `lifetime_total_bytes`. Tests: `TestUsageRangeWindow` (7 cases), `TestUsageHistorySplitsUpDownTotal`, `TestUsageRangeTypoIsRefused`. |
| 2026-10-25 | **K next** | P-03/P-07 (server CPU/RAM/network charts), P-04 (live rate), P-05 (client-side ping), P-09 (protocol detail), P-10 (QR). |
| 2026-10-25 | **K: P-04 + P-09 done** | `GET /api/vpns/{id}/protocol` (P-09) answers the credential shape the owner asked for: `protocol` (from Veylix's inbound record), `server` (host:inbound-port), `uuid`/`password`/`flow`/`security` (from the **panel's** client record), `panel_email`, `sub_id` and an explicit `source`. An unreadable panel answers `source: "unavailable"` while still reporting the locally-known protocol — an outage must never read as "this VPN has no credential". `GET /api/vpns/{id}/traffic/live` (P-04) returns **bytes per second** computed from the last two sync snapshots, with `measured_from`/`measured_to`; one reading, an elapsed time of zero, or a counter reset answers `source: "unavailable"` instead of a misleading `0`. Tests: `TestProtocolDetailIsExposed`, `TestProtocolReportsOutageHonestly`, `TestLiveTrafficRate` (1 MB/s up, 10 MB/s down), `TestLiveTrafficNeedsTwoReadings`. |
| 2026-10-25 | **K remaining** | P-03/P-07 (chart switches), P-05 (client-side ping), P-08 (chart type), P-10 (QR), then Phase L (UI). |
| 2026-10-25 | **K: P-10 done (QR)** | An **in-repo, dependency-free QR encoder** (`frontend/src/lib/qr.ts`): byte mode, error-correction level M, versions 1–10, spec-exact RS block table, mask chosen by the four penalty rules. `QrCode` renders it as inline SVG with a 4-module quiet zone and a hard white background (a scanner needs contrast), and shows a readable message instead of a blank square when a link cannot be encoded (D-033.4). Wired into `/vpn/:id` beside the link, in a new `.vpn-connect-grid`. **Why no library:** the URI carries the customer's credential, so it is never sent to a QR service (D-033.2). Self-test `npm run qr:selftest` — **7/7**: real `vless://` URI, three finder patterns, timing patterns, version grows with the payload, a realistic ~200-byte URI fits, an oversized payload is **refused not truncated**, and the encoder is deterministic. `npx tsc -b`, `npm run lint` (0/0), `npm run build` and `npm run audit` all green. |
| 2026-10-25 | **Next** | P-05 (client-side ping), P-03/P-07/P-08 (chart switches), then Phase L (UI surfaces). |
| 2026-10-28 | **K: P-01…P-04, P-06…P-09 wired into the page** | `/vpn/:id` now renders the mandated set instead of a fraction of it. New `components/vpn/PremiumInsights.tsx` (P-01 up/down/total from the server, P-02 capacity, P-03 CPU/RAM, P-04 live rate as a rate, P-06 the 7d/30d/3m/1y switch, P-07 per-metric charts, P-08 line/area/bar/pie) and `components/vpn/ProtocolDetail.tsx` (P-09, credential masked until revealed). The page **stopped** computing `up + down` itself — it repeats `lifetime_total_bytes` / `usage_pct`, so two places can no longer disagree. New `scripts/verify-vpn-detail.mjs` (`npm run verify:vpn`) proves it in a real browser: **11/11 checks, no console errors**, against VPN 17 and 21 on the live panel. **P-05 remains open** — the ping is still server reachability, correctly labelled, and the client-side handshake is not built. |
| 2026-10-28 | **M7-21 fixed (found by the browser check)** | The QR refused VPN 17's **442-byte** real panel link (cap was 412 at v15), so the customer saw "too long to encode" where their QR belonged. Versions **16–20** added from the spec's level-M table; cap now **666 bytes**; `npm run qr:selftest` grown to **10 cases** including a fixture fixed at exactly 442 bytes and a 1…666 walk proving every version 1–20 is reachable and 667 is refused, not truncated. Not camera-verified. |
| 2026-10-25 | **Review pass - audit only, one P1 fixed** | Ran every gate on the current tree: `go build`/`vet`/`gofmt` clean, `go test ./... -count=1` **11 packages ok** (370 test functions), `tsc -b`, `oxlint` 0/0, `npm run audit` 0 findings, `npm run qr:selftest` 9/9. **Fixed one real defect:** the QR encoder refused every `vmess://` link over 213 bytes (M7-14) - versions 11-15 added, cap now 412 bytes, the self-test grown from 7 to 9 cases. **Closed the named-test gap** for `TestOrderTimeline`, `TestVPNUsageAgainstCapacity` and `TestAssignRenamesPanelClient` (M7-20); the other four names are corrected in the tables above, not quietly dropped. **Recorded, not built:** M7-15 (P0 - `/buy` still lets the buyer type the price), M7-16 (15 backend routes with no UI caller; five Phase-L screens missing), M7-17 (P-01/P-02/P-04/P-06/P-09 not on `/vpn/:id`; P-05 not built), M7-18 (README and `API_CONTRACT.md` drift), M7-19 (the audit gate cannot see M7-15/M7-16). Details: `docs/IMPROVEMENT_LEDGER.md`. **Phase L was deliberately not started in this pass** - it is the next unit of work, in the order L4, L2, L7, L1, L3, L5. |

