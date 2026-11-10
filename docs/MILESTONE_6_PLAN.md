# VEYLIX — MILESTONE 6: OWNER REVIEW 2026-10-25 → IMPLEMENTATION PLAN

**Created:** 2026-10-25
**Mandated by:** owner review of the shipped app
**Governing decisions:** **D-030 … D-033** (plan ownership, payment = release, Free/Premium separation, QR)
**Contract:** the single checklist for this milestone. Every item is `[ ]` until a command proves it. No item is ticked because the code exists (`new_update.md` §13).
**Companion:** `docs/IMPROVEMENT_LEDGER.md` (findings M6-xx), `docs/PROCESS_MAP.md` (process verdicts), `docs/API_CONTRACT.md` (routes).

---

## 0. What the owner asked for (source of this milestone)

> *"ne hadapu ewageth adupadu thiyena then thiynawa offer template remove karanna ba, free ewa qr eka na, ui basic wedii. overall ui basic widii thawa features add wenna one. main point eka premium vpn sell kirima super admin direct client ta ha admin haraha client ta ewagema. client lara premium free wenama clearwa bala ganna thiyenna one. clientge overview eka na adminge ekath na. clientta thawa features add karanna. vpn specially premium vpn walata adala hema deyakma function wenna one. e dewath most important"*

> *"admin buy karapu premium ekak nam eyata super admin gen awasara ganna one na. eyata kemathi deyak ekata karanna puluwan. anek premium ekiyanne admin kenek buy karala nethi premium — super admin confirm karanne nethuwa katawath use karanna beri wenna one."*

> *"usage history, server details, ram cpu, network speed upload download eka ewele yana eka 3x-ui eken ganna. chat tika, network usage, cpu ram usage chat client ta one widihata maru karanna puluwan wenna, multi chat, pie/bar/line charts, details."*

---

## 1. The target product, in one diagram

```
                        SUPER ADMIN
        plans · prices · panels · the ONE confirm button
                              │
        ┌─────────────────────┴──────────────────────┐
        │                                            │
  SA plans (unowned)                        panels (servers)
        │                                            │
  client ORDER ──► SA CONFIRM ──► VPN + QR            │
                                                assigned to
                                                     │
                                          ┌──────────┴──────────┐
                                          │   ADMIN (reseller)   │
                                          │  buys plan slots     │
                                          │  releases to client  │
                                          │  ⚡ no SA approval    │
                                          └──────────┬──────────┘
                                                     │
                                                  CLIENT
                                    ┌────────────────┴────────────┐
                                    │ FREE  — public, 3 / 5 per 30d│
                                    │ PREMIUM — order → confirmed  │
                                    │ both: QR + link + usage      │
                                    └─────────────────────────────┘
```

**Five promises this milestone must make true:**

| # | Promise | Decision |
|---|---|---|
| P-1 | A payment enters only through the Super Admin's confirm button, and that button also builds the VPN | D-031 |
| P-2 | An admin's own plan needs no approval; an unowned premium plan is unusable without the SA | D-030 |
| P-3 | A client sees Free and Premium as two separate things, always | D-032 |
| P-4 | Every VPN has a QR and a link the moment it exists | D-033 |
| P-5 | Nobody ever sees a number or a button the backend cannot back | D-015 |

---

## 2. Baseline — measured 2026-10-25 before this milestone

| Gate | Result |
|---|---|
| `go build ./...` · `go vet ./...` · `gofmt` | PASS |
| `go test ./... -count=1` | **PASS** — 9/9 packages (`audit`, `releases` have no tests) |
| `npm run lint` | PASS — 0 errors / 0 warnings, 104 files |
| `npm run build` | PASS (warning: main chunk 515 kB) |
| `npm run audit` | PASS — 0 findings |
| Backend in git | 🔴 **NOT COMMITTED** — 4 frontend-only commits; the backend lives in the working tree |
| Live panel writes beyond create | NOT VERIFIED (D-018) |


---

## 3. Phase A — Backend spine (revenue + ownership)

> Rule: **no UI is written before the endpoint behind it exists and has a test.**

| # | Item | Endpoint / file | Test that proves it |
|---|---|---|---|
| A1 | Commit the existing backend so this work has a rollback point | `git` | `git log --oneline` shows it |
| A2 | Migration `0008_plans_money.sql`: `vpn_offers.{owner_user_id, price_cents, currency, slots_total, slots_used, archived_at}`, `payment_records.{offer_id, target_user_id}`, table `vpn_plan_purchases` | `store/migrations/0008_plans_money.sql` | `TestSchema0008` |
| A3 | Plan CRUD for the SA: create / price / archive / **delete** (in-use → `409 plan_in_use`) | `PATCH`, new `DELETE /api/offers/{id}` | `TestOfferDeleteRefusesInUse`, `TestOfferDeleteUnused` |
| A4 | Slot accounting decremented **inside** the provisioning tx | `provisioning/service.go` | `TestReleaseConsumesSlotOnce` (concurrent) |
| A5 | Ownership gate: release of an unowned premium plan without SA confirm → `403 plan_not_approved` | `provisioning/service.go` | `TestUnownedPlanRequiresSuperAdmin` |
| A6 | Server-access gate for a reseller's own plan (assigned server only) | `provisioning/service.go` | `TestResellerCannotReleaseOnForeignServer` |
| A7 | `POST /api/payments` takes `offer_id`; **the server computes the amount** | `httpapi/payment_record_routes.go`, `store/payments.go` | `TestPaymentAmountComesFromPlan` |
| A8 | **`ConfirmPayment` → provisioning in one transaction**, returns `{payment, vpn, links}`; panel failure → payment stays `PENDING`, no VPN row, no slot consumed | `store/payments.go` + route | `TestConfirmPaymentBuildsVPN`, `TestConfirmPaymentPanelFailureRollsBack` |
| A9 | `GET /api/plans` (SA: all; admin: own; client: premium **facts only**) per D-032.3 | new `httpapi/plan_routes.go` | `TestClientSeesPlanFactsNotOwnership` |
| A10 | `POST /api/plans/{id}/purchase` — an admin buys slots (creates an order for the SA) | new | `TestPurchaseCreatesPendingOrder` |
| A11 | `GET /api/overview` — one call per role (SA / admin / client) | new `httpapi/overview_routes.go` | `TestOverviewPerRole` |
| A12 | VPN row carries `tag` + `offer_id` + `offer_kind` + `plan_name` (D-032.1/.4) | `httpapi/vpn_lister*.go`, `vpn_routes.go` | `TestVPNRowCarriesPlan` |
| A13 | Fix **cross-tenant `vpn_count`** (an admin counted other resellers' VPNs) | `store/assignments.go` | `TestClientVPNCountIsSellerScoped` |
| A14 | `POST /api/vpns/{id}/renew` — extend expiry, panel first, confirm, then record | `provisioning/service.go` | `TestRenewExtendsPanelExpiry` |
| A15 | Server metrics: record **real** `net_up`/`net_down` deltas (currently written as `0`) | `sync/service.go` | `TestMetricsRecordNetworkDelta` |
| A16 | `GET /api/metrics/live` — one request for every panel + per-VPN usage | new | `TestLiveMetricsBatchesPanels` |
| A17 | HWID device routes (`GET/DELETE /api/vpns/{id}/devices`) | new `httpapi/device_routes.go` | `TestDevicesListAndRemove` |
| A18 | Online / last-seen with an explicit "unsupported by this panel version" state | `panel/ops.go` + route | `TestOnlineUnsupportedIsExplicit` |

**Exit gate A:** `go build ./... && go vet ./... && go test ./... -count=1` green, with every named test present and passing.


---

## 4. Phase B — The money screens

| # | Page | Role | Must show / do |
|---|---|---|---|
| B1 | **`/orders` — Payment Confirmation Centre** | SA | Every pending order (client + reseller). One **Confirm** = VPN built, then **QR + link + plan** shown to hand over. **Reject** with a reason. Every action audited + notified. |
| B2 | **`/plans` — Plan catalogue** | SA | Create / price / slots / archive / delete. Shows remaining slots and in-use counts. |
| B3 | **`/sell` — Sell premium** | SA + admin | Client (existing **or create with invite**) → plan → payment method + reference → submit. SA path continues to B1's confirm; **admin path (own plan) produces the VPN immediately** with QR. |
| B4 | **`/buy` — Buy premium** | client | Premium plans only, server-priced. Creates an order → timeline: submitted → confirmed → VPN + QR. No amount typing. |
| B5 | **`/buy-slots` — Buy plan slots** | admin | SA plans, quantity, order → SA confirm → slots credited. |

**Exit gate B:** a client order can be created, confirmed by the SA and produce a QR in one browser session (evidence in `QA_REPORT.md`).

---

## 5. Phase C — Premium VPN on every role

| # | Item |
|---|---|
| C1 | `/vpn/:id` for **all three roles**, action bar: suspend · resume · revoke · renew · release · links · **QR** · devices |
| C2 | `/vnps` — VPN lists for the SA (all panels, tag filter, untagged queue) and for an admin (own sales) |
| C3 | `/panel-clients` — panel inspector + **tagging UI** (finishes I-3 end to end) |
| C4 | QR component (in-repo SVG encoder, 0 npm deps) + download PNG + "send to client" |
| C5 | Usage timeline chart, 7d / 30d / All + chart-type switch (line / bar / area) |
| C6 | Server CPU / RAM / network charts on the VPN page, each labelled with its own source (D-026.2) |
| C7 | Device (HWID) list with remove |
| C8 | Chat deep-link from a VPN's support card with the VPN context (removes the stale "P7" copy) |
| C9 | Client home: **Free | Premium** sections with counts, QR inline, expiry countdown, pending order |

---

## 6. Phase D — Overviews (SA / admin / client)

| # | Page | Content (all from `GET /api/overview`) |
|---|---|---|
| D1 | SA `/dashboard` | clients · admins · servers · free vs premium · revenue confirmed/pending · expiring in 7 days · sync health · activity timeline · panel grid |
| D2 | Admin `/admin` | sales today / 7d · revenue trend · top clients · quota bar · expiring · pending orders · own panel health |
| D3 | Client `/client` | active / free / premium counts · usage ring · next expiry countdown · pending order · recent activity |

---

## 7. Phase E — Chat v2

| # | Item |

---

## 8. Phase F — UI v2 (the "basic" complaint)

| # | Item |
|---|---|
| F1 | New primitives: `Breadcrumbs`, `SegmentedControl`, `DataGrid` (filter · bulk · CSV), `Drawer`, `StatStrip`, `Timeline`, `ActivityFeed`, `ConfirmDialog`, `Stepper`, `CopyField`, `Avatar`, `QrCode` |
| F2 | Page-level CSS for every route (today `offers.css` is 137 bytes, `admins.css` 672 bytes) |
| F3 | One page-header pattern: title · KPI strip · primary action · secondary actions |
| F4 | Remove stale copy (`"once the chat API exists (P7)"`), fix the `userName` fallback, fix the admin↔new-client chat dead end |

---

## 9. Definition of done

| Gate | Requirement |
|---|---|
| Backend | `go build` · `vet` · `gofmt` · `go test ./... -count=1` green; every Phase A test exists |
| Frontend | `npm run lint` 0/0 · `npm run build` · `npm run audit` 0 findings |
| Money path | One automated test proves: order → confirm → VPN row → ACTIVE → links non-empty → slot consumed |
| Ownership path | One automated test proves: unowned plan without SA → `403`; admin's own plan → no approval |
| Docs | `DECISIONS.md` D-030…D-033 · `IMPROVEMENT_LEDGER.md` M6-xx · `PROCESS_MAP.md` · `API_CONTRACT.md` · this file · `QA_REPORT.md` |
| Not claimed | anything blocked by D-018 is marked "implemented, not live-verified" |

---

## 10. Progress log (append-only)

| Date | Stage | Result |
|---|---|---|
| 2026-10-25 | Plan recorded | D-030…D-033 written; baseline measured (`go test` 9/9, lint 0/0, build OK, audit 0) |
| 2026-10-25 | **A1 done** | Backend committed — `e12a793` (248 files: backend, stores, docs). Working tree clean before any Milestone 6 code. |
| 2026-10-25 | **A2 done** | `0008_plans_money.sql`: plan owner/price/slots/archive, payment `offer_id`/`target_user_id`/`vpn_instance_id`/`quantity`, `vpn_plan_purchases`. Legacy rows keep their meaning (price 0, unowned, unlimited). Tests: `TestPlanMoneySchemaIsPresent`, `TestPlanDefaultsKeepLegacyRowsMeaningful`, `TestPaymentTargetDefaultsToNullAndIsReadable`, `TestPlanPurchaseRequiresRealPlan` — all pass. |
| 2026-10-25 | **A3/A4/A5 storage done** | `store/plans.go`: `Plan`, `CreatePlan`, `UpdatePlan` (slot guard), `ArchivePlan`, **`DeletePlan`** (`409 plan_in_use` unless unused), `ClaimPlanSlot` (atomic), `CheckPlanRelease` (D-030.2/.3 gate). Tests: `TestClaimPlanSlotIsAtomic` (**2 racers, 1 slot → exactly 1 success**), `TestClaimPlanSlotUnlimited`, `TestCheckPlanRelease`, `TestCheckPlanReleaseFreeIsUngated` — all pass. |
| 2026-10-25 | **A3/A5/A6/A13 done** | `provisioning.authorise` now enforces D-030: unowned premium → `403 plan_not_approved`; another reseller's plan → `403 not_plan_owner`; unassigned server → `403 server_not_assigned`; archived/exhausted plans refused. Create claims a plan slot inside the same transaction. Cross-tenant `vpn_count` leaked fixed (`TestClientVPNCountIsSellerScoped`). Tests: `TestUnownedPremiumPlanNeedsSuperAdminConfirm`, `TestResellerOwnPlanNeedsNoApproval`, `TestResellerCannotReleaseForeignPlan`, `TestResellerCannotReleaseOnUnassignedServer`, `TestPlanSlotsLimitReleases`. Commit `c2acf72`. |
| 2026-10-25 | **A15 done** | **M6-12 fixed**: `panel.HealthResult` now carries `netIO.up/down` (cumulative) + `Load1`; `sync.recordMetrics` stores the **per-second rate** (difference ÷ elapsed, reset → 0) instead of a constant `0`. Tests: `TestMetricsRecordNetworkRate` (clock-frozen: 60 MB / 60 s → exactly 1 MB/s), `TestMetricsTreatCounterResetAsZero`. |
| 2026-10-25 | **A7/A8/A9 done** | **Money path closed (D-031).** New `internal/payments` `OrderService.Confirm`: read order → authorise actor → provision (`Approved: true`) → `store.CompletePayment` (CONFIRMED + `vpn_instance_id`, one transaction) → links. `POST /api/payments/{id}/confirm` answers `{payment, vpn, links, link_source}`; a panel failure leaves the order pending and charges nothing. `POST /api/payments` accepts `offer_id` and **the server prices it from the plan**. New `/api/plans` catalogue with SA-only DELETE + archive, clients get facts-only projections (D-032.3). Tests: `TestConfirmPaymentBuildsVPN`, `TestPaymentAmountComesFromPlan`, `TestConfirmPaymentPanelFailureLeavesOrderPending`, `TestConfirmIsNotRepeatable`, `TestConfirmRequiresTheRightActor`, `TestResellerConfirmsOwnPlanOrder`. Commit `858b453`. **10/10 packages green.** |



|---|---|
| E1 | Three columns: threads ↔ messages ↔ **details drawer** |
| E2 | Role-aware usage widget (client → own VPN usage; admin → clients + quota; SA → platform) |
| E3 | Charts from real message data only: pie (by role), bar (weekly volume), line (response time) |
| E4 | Attachments + emoji + mentions, still participant-enforced on the server |

---

## 11. Milestone 7 progress (Phase G onward)

> Milestone 7 has its own plan file (`docs/MILESTONE_7_PLAN.md`); this table is the
> shared log so a reader of either file sees one timeline.

| Date | Stage | Result |
|---|---|---|
| 2026-10-25 | Plan recorded | D-034 … D-038 written from the owner's answers (panel identity, three package kinds, order/payment/block, assignment, the mandatory Premium feature set). Phases G … L scoped. |
| 2026-10-25 | **G1 + G2 + G3 done — GREEN** | `PanelClientView` answers *whose* VPN this is: `vpn_instance_id`, `owner_user_id`, `owner_email`, `owner_name`, `seller_user_id/email`, `offer_id/name/kind`, `created_at`, plus `limit_bytes`, `used_bytes`, `usage_pct`, `days_left`. `loadAssignments` replaced its `map[string]bool` with a `panelOwner` struct over one bounded query (joined to `users`, `vpn_assignments`, `vpn_offers`). `GET /api/servers/{id}/panel-clients` filters server-side on `?tag=&assigned=&usage=&q=` (unknown value → 422) and answers `?summary=1` with **total · free · premium · untagged · assigned · unassigned · exhausted · expiring · expired · disabled**. Tagging split into `TagRoutes` so the inspector stays read-only. Tests: `TestServerClientListShowsBuyer`, `TestServerClientSummary`, `TestServerClientFilters` (14 cases). Commit `1ef7e14`. **10/10 packages green**, `vet`/`gofmt` clean. |

