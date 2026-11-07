# VEYLIX — DEVELOPMENT LEDGER

**Status:** authoritative · **Established:** 2026-11-08 · **Baseline:** `1b09553`

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

**Priority.** `P0` security / data-loss · `P1` broken core function, unverified
remote write, incorrect money · `P2` reliability, performance, maintainability ·
`P3` polish.

---

## DEV-001 · P1 · An offer's inbound id was sent to the panel verbatim, so no FREE claim could succeed — **FIXED**

- **Date found & fixed:** 2026-11-08
- **Legacy id:** `I-36` (recorded here for the first time; the legacy entry was lost)
- **Symptom.** `POST /api/vpns` with a valid FREE offer answered **502
  `panel_rejected`**. `POST /api/offers` with `inbound_id: 1` answered **500**.
- **Cause.** Two numbering schemes in one column. `vpn_offers.inbound_id` is a
  foreign key to `inbounds(id)` — Veylix's own AUTOINCREMENT row id — while
  `inbounds.remote_id` holds the panel's number. The dev rows were `id=52,53,54`
  carrying `remote_id=1,2,3`. `provisioning.Create` copied `offer.InboundID` straight
  into the panel request, so the panel was asked for `inboundIds:[52]`, answered
  "inbound not found", and `pushCreate` — correctly — discarded the provisional row
  and reported `panel_rejected` (D-024.1a). The *rejection logic* was right; the
  *number* was wrong.
- **Fix.** `provisioning.panelInboundID(ctx, serverID, inboundID)` resolves
  `inbounds.remote_id`, and both the outbox payload and the live `AddClient` carry
  that number. The VPN row keeps the **local** id so the foreign key stays
  meaningful. A missing row, a row from a different server, or `remote_id <= 0` is
  `ErrOfferIncomplete` — refused before anything is written.
- **A second defect found while fixing it.** The resolver was first placed *inside*
  the create transaction. That held SQLite's write lock across two statements and
  made the second of two concurrent creates fail with `SQLITE_BUSY` instead of the
  quota refusal `TestQuotaIsEnforcedUnderConcurrency` expects — reproduced 4/4
  before the move, clean after. It now runs **before** the transaction opens.
- **Tests.** `TestCreateSendsThePanelsInboundNumber` (local `id=1`, `remote_id=7`,
  asserts the panel received `[7]`) and `TestCreateRefusesInboundFromAnotherServer`
  (refuses, writes nothing, never calls the panel). The fake client gained an
  `inboundIDs` field so what was *sent* is asserted, not merely that a call happened.
- **Live proof, both layers.** `POST /api/offers` → **201** (`id=17`, FREE,
  `inbound_id=52`). `POST /api/vpns {offer_id:17}` as `client@veylix.test` → **201**
  `{"vpn_id":42,"panel_email":"client@veylix.test_free_1","status":"ACTIVE","confirmed":true,"recorded":true}`.
  `cmd/panelprobe` then reported `vpn_id=42 … probe=FOUND` — the client exists **on
  the panel**, not merely in the local database.
- **Status:** ✅ **FIXED-VERIFIED**

---

## DEV-002 · P2 · 193 unreachable inbox rows left by a removed producer — **FIXED**

- **Date found & fixed:** 2026-11-08
- **Legacy id:** `I-37`
- **Symptom.** Every client inbox was dominated by identical noise: `Action not
  applied / user N · result=DENIED`. The table held 196 such rows out of 239 (82%).
- **Cause.** Already fixed **in code** — `audit.isSignInFailure` keeps a refused
  sign-in out of the inbox, and `audit.describe` now emits a specific sentence per
  action. The defect was the **history**: rows the removed producer had written had
  no code path that would clean them up, and none that would recreate them.
- **Fix.** A one-off, deliberately narrow cleanup:
  `DELETE FROM notifications WHERE title = 'Action not applied' AND body LIKE 'user %· result=DENIED'`.
  Because the current producer can no longer emit that exact shape, the predicate
  cannot delete a row the running code creates.
- **Data safety.** Backup first: `.veylix-secrets/data/veylix.pre-20261108-042952.db`
  (782,336 bytes). Dry run first (193 matched, 46 kept), then applied: **193
  deleted, 46 remained**.
- **Verification.** `GET /api/notifications` as `client@veylix.test` afterwards
  returns two rows, both meaningful and both from real actions: `VPN created / vpn
  42 · status=ACTIVE` (success) and `VPN could not be created / vpn 41` (error — the
  502 attempt above). No `Action not applied` row remains.
- **Status:** ✅ **FIXED** (data). The code was already correct.

---

## DEV-003 · P2 · `httpapi` runs SQL, contradicting the "only `store` writes SQL" rule — **OPEN**

- **Date recorded:** 2026-11-08
- **Evidence.** 12 SQL statements in `internal/httpapi/`, measured:
  `usage_store.go` 7 · `vpn_lister.go` 2 · `vpn_lister_panel.go` 2 · `panel_cache.go`
  1. Across the whole tree, 112 statements live outside `store/`.
- **Why recorded rather than fixed now.** It is not a correctness bug — the code
  works and 617 tests pass. It is a maintainability finding, and fixing it touches
  the VPN list and usage read paths the client UI depends on.
- **Decision needed.** `UNKNOWNS.md` `Q-13`: Option A (move the 12 queries into
  `store`) or Option B (rewrite the rule to describe reality).
- **Status:** 🔶 **OPEN** — recorded, not fixed.

---

## DEV-004 · P3 · `vpn_transfers` rows die with their VPN — **OPEN**

- **Date recorded:** 2026-11-08 · **Carried from** `LEGACY-I-34`
- **Evidence.** `0020_vpn_transfer.sql:63` declares `ON DELETE CASCADE`.
- **Why it matters.** The record "this VPN moved from client A to client B" vanishes
  when the VPN row is deleted, so transfer history cannot outlive its subject.
- **Same class already fixed:** `LEGACY-I-33` on `vpn_releases`, fixed by migration
  `0023` (nullable + `SET NULL` + `panel_email`/`server_id` retained). That migration
  is the template for this fix.
- **Status:** 🔶 **OPEN** (P3)

---

## DEV-005 · P3 · `usage_snapshots` rows die with their VPN — **OPEN**

- **Date recorded:** 2026-11-08 · **Carried from** `LEGACY-I-35`
- **Evidence.** `0003_observability.sql:7` declares `ON DELETE CASCADE`.
- **Why it matters.** Per-VPN usage history disappears when the VPN is purged, so a
  client's own traffic trend cannot survive a revoke-and-recreate.
- **Caution carried forward.** Snapshots are the largest table by row count, so a
  `SET NULL` fix would retain dead series indefinitely on a 1 vCPU / 500 MB host. The
  earlier pass recommended **not** applying it without a consumer. That
  recommendation stands and is not overturned here.
- **Status:** 🔶 **OPEN** (P3, deliberately deferred)

---

## DEV-006 · P2 · Client-side style debt: 224 inline `style={{…}}` — **OPEN**

- **Date recorded:** 2026-11-08 · **Carried from** `LEGACY-U13`
- **Evidence.** 224 inline style objects across `frontend/src/**/*.tsx`, repeating
  font sizes, gaps and colours that the design tokens already define.
- **Why it matters.** It defeats the theme system: a token change cannot reach an
  inline literal, so a sub-theme preset drifts from the rest of the UI on exactly
  the screens that use inline styles.
- **Status:** 🔶 **OPEN** (P2)

---

## DEV-007 · P2 · `EmptyState` missing on three Admin pages — **OPEN**

- **Date recorded:** 2026-11-08 · **Carried from** `LEGACY-U16`
- **Evidence.** `EmptyState` is used on `/client`, `/history` and `/buy`; three Admin
  pages render a bare empty table instead.
- **Why it matters.** An empty table is ambiguous — it reads the same whether there
  are no rows or the read failed. An explicit empty state is the honest answer, and
  the client surface already does this correctly.
- **Status:** 🔶 **OPEN** (P2)

---

## DEV-008 · P2 · Mixed loading indicators (`Skeleton` vs inline "Loading…") — **OPEN**

- **Date recorded:** 2026-11-08 · **Carried from** `LEGACY-U17`
- **Evidence.** Lists use `Skeleton`; several detail panes render the text
  "Loading…".
- **Status:** 🔶 **OPEN** (P2)

---

## DEV-009 · P2 · Focus ring unverified on three components — **OPEN**

- **Date recorded:** 2026-11-08 · **Carried from** `LEGACY-U18`
- **Evidence.** No automated check asserts a visible focus ring on `SavedViewsBar`,
  `Pagination`, or table sort headers.
- **Why it matters.** Keyboard-only navigation is a real path, and an invisible focus
  ring makes it unusable. "Not verified" is not the same as "wrong" — but it is not
  `PASS` either.
- **Status:** 🔶 **OPEN** (P2, accessibility)

---

## DEV-010 · P2 · `verify-mobile.mjs` measures only 390 px and never enables touch — **OPEN**

- **Date recorded:** 2026-11-08 · **Carried from** `LEGACY-S3-1` + `LEGACY-S3-4`
- **Evidence.** The script asserts layout at 390 px only, while the design brief
  requires several widths. It also never turns on CDP touch emulation, so a
  tap-target regression would pass.
- **Why it matters.** A responsive gate that checks one width reports `PASS` for
  viewports it never rendered. That is the failure mode the audit rules exist to
  prevent.
- **Status:** 🔶 **OPEN** (P2)

---

## DEV-011 · P1 · The client "My VPNs" page was never executed against a populated account — **FIXED**

- **Date found:** 2026-11-08 (recorded in the S5 report as "populated state is NOT
  PROVEN") · **Fixed:** 2026-11-08 (same day, once `DEV-001` unblocked it)
- **Symptom — a verification gap, not a user-visible bug.** S5 shipped
  `MyVpnsPage.tsx` with cards, a compact list, search, category chips, a filter
  panel, sort and paging. Every one of those was type-checked and lint-checked, and
  the CSS was probed by mounting a detached replica — but **none of it had ever run
  against real rows**, because the seeded client owned zero VPNs.
- **Root cause of the gap.** It was a chain, not a single fault: the seeded client
  held no VPN → assigning one was refused (`409 panel_client_taken`) → creating a
  FREE offer to claim was impossible because `POST /api/offers` returned **500**.
  That last link is `DEV-001` / `LEGACY-I-36`. Once the inbound-id translation was
  fixed, every link above it broke loose and the populated state became reachable.
- **What was done.**
  1. Three FREE offers created via `POST /api/offers` (**201** each): `Free 2GB
     Standard` (inbound 53), `Free 5GB Hysteria` (inbound 54), and the existing
     `Free 1GB Trial` (inbound 52).
  2. Two claims via `POST /api/vpns` as the client (**201**, `ACTIVE`,
     `confirmed:true`) — VPN **43** and **44**, alongside the existing VPN **42**.
  3. **Both layers verified.** `cmd/panelprobe -server 11` reported `probe=FOUND`
     for **all six** panel clients, including 42/43/44 — so these are real 3X-UI
     clients, not local-only rows (INV-1).
  4. `frontend/scripts/verify-client-vpns.mjs` written and registered as
     `npm run verify:client-vpns`.
- **The committed gate — 19 checks, `19/19 PASS`.**
  Page h1 · every owned VPN rendered as a card · more than one card (so search and
  sort are meaningful) · no empty state on a populated account · overview TOTAL
  matches the real inventory · search narrows to one card · the search term reaches
  the URL · a non-matching term empties the grid · the no-results copy appears · the
  copy reassures the user their VPNs still exist · a Clear filters action exists ·
  Clear filters restores every card · the List mode renders a real table · the table
  lists every row · the table exposes VPN/STATUS/SERVER · the display mode reaches
  the URL · a name-sort control exists · sorting keeps every row · the sort is
  genuinely alphabetical.
- **The gate refuses to lie.** Against an account that owns nothing it prints
  `REFUSED … Reporting PASS here would be a false claim.` and exits **3**. Proven:
  run as `reseller@veylix.test` (owns 0) → `REFUSED`, exit **3**. A regression that
  empties the account therefore fails the gate instead of silently passing it.
- **A correction made during this work, recorded rather than hidden.** My first
  probe reported `sortable: []` and I nearly recorded "sort is missing". It is not:
  sort lives in the **Filters** panel as a `<select>` (`SortKey = newest | name |
  expiry`), not as sortable table headers. The first probe was looking in the wrong
  place. Reading `MyVpnsPage.tsx` L563–570 settled it.
- **Status:** ✅ **FIXED-VERIFIED**

