# VEYLIX — UI/UX DEFECT BOARD

**Created:** 2026-11-07 · **Owner:** UI/UX redesign programme (stages 5-8)
**Type:** Living board — every user-visible defect found by auditing `frontend/src/`,
one row each, with the file, the evidence and the fix.

**Rule.** A row is `CLOSED` only when the file is changed **and** the gates in
§5 ran with their real output recorded. A row investigated and found not to be a
defect is `WITHDRAWN` with the reason.

**This board does not duplicate other homes.** Endpoints stay in
`API_CONTRACT.md`, backend decisions in `DECISIONS.md`, backend defects in
`IMPROVEMENT_LEDGER.md`. This board is **frontend presentation only**.

---

## 0. Status legend

| Symbol | Meaning |
|---|---|
| ✅ **CLOSED** | Fixed, gates run, output recorded |
| 🟡 **OPEN** | Confirmed real, not yet fixed |
| ⚪ **WITHDRAWN** | Investigated; not a defect (reason recorded) |
| ⛔ **BLOCKED** | Needs owner decision or a capability we do not have |

---

## 1. P0 — Wrong or misleading information shown to the user

| ID | Page / File | Defect | Evidence | Fix | Status |
|---|---|---|---|---|---|
| **U1** | `frontend/src/stores/vpnStore.ts:196` `vpnDisplayName` | A VPN issued without a catalogue offer shows the **internal carrier name "Platform assignment"** as its product title to the client | `backend/internal/store/assignment.go:332` — `const platformAssignmentOffer = "Platform assignment"`; `vpnDisplayName` returned `offerName` verbatim, so the carrier string reached a customer card | **Fixed:** `INTERNAL_CARRIER_NAMES` set; that exact string (case-insensitive) is now treated as "no name" and falls back to the kind label. UI-only — the backend name stays because audit history records it | ✅ **CLOSED** |
| **U2** | `frontend/src/pages/HistoryPage.tsx:104` | Purchase history rows were titled with **`panelEmail`** (`vyx-23`) instead of the product bought | `name: v.panelEmail` in the `vpns.map(...)` row builder, under a card titled "Records" | **Fixed:** a new `title` field carries `vpnDisplayName(v)`; the panel email survives as a muted second line, so support can still read it back | ✅ **CLOSED** |
| **U3** | `frontend/src/pages/HistoryPage.tsx:117` | History rows showed **`server ${v.serverId}`** — a bare database id — instead of the operator's server label | same row builder | **Fixed:** `vpnServerLabel(v)`, the same helper every other screen uses, so an unlabelled server says so honestly | ✅ **CLOSED** |
| **U4** | `frontend/src/pages/VPNDetailPage.tsx:721` | `Panel identity: <panelEmail> on server N` read as an instruction the customer must act on | literal JSX text | **Fixed:** relabelled **"Connection ID (technical — quote it to support)"**, and the server is named with `vpnServerLabel(vpn)` instead of the raw id | ✅ **CLOSED** |
| **U5** | `frontend/src/pages/ClientDashboardPage.tsx:346` + `pages/client.css:28` | No single **verdict line**; "am I OK?" needed four metric cards to answer | the metric strip was the first quantitative content; no sentence verdict existed | **Fixed:** `data-testid="client-verdict"` sentence under the hero, built only from the server-reported `activeCount` / `attention` counts, styled with `--success` / `--warning` tokens (`.client-verdict.is-ok` / `.is-warn`) | ✅ **CLOSED** |
| **U6** | `frontend/src/components/layout/AppShell.tsx:39` | The layout debug helper was installed on every page load and could print `console.log('tokens', …)` | grep found exactly one `console.log` in `frontend/src` | **Fixed:** the effect now returns early unless `import.meta.env.DEV`, so the helper is not installed in a production bundle | ✅ **CLOSED** |


## 2. P1 — Information architecture and flow

| ID | Page | Defect | Fix | Status |
|---|---|---|---|---|
| **U7** | `/client` | The free-offer claim sat below the VPN lists, so a brand-new client scrolled past two empty sections to reach the only action they had | Section order **Hero → Verdict → Your VPNs → Get a free VPN**, with a "Get started" card when the account owns nothing (added Pass 17, browser check pending) | 🟡 **OPEN** |
| **U8** | `/client` | Two labels for one action: cards said **"Open detail"**, the hero said **"Open details"** | One verb everywhere: **"Open"** | ✅ **CLOSED** |
| **U9** | `/vpn/:id` | The hero's body copy explained sync mechanics ("Usage below is read from the panel by the background sync…") — an internal detail in the customer's first paragraph | **Fixed:** replaced by `data-testid="vpn-verdict"` (`.vpn-verdict`) stating *data left · days left*, or the status-specific reason it is not connectable via the new `NOT_CONNECTABLE` map (one honest sentence per status rather than one generic one) | ✅ **CLOSED** |
| **U10** | `/vpn/:id` | Six cards stacked in one column with no grouping | **Fixed:** three named `<h2 class="vpn-section-title">` sections in reading order — **Usage** (PremiumInsights) → **Devices** (staff only) → **Connection** (links + setup guide). Navigable by screen reader, not just visually separated | ✅ **CLOSED** |
| **U11** | `/buy` | Step 2 stacked three warn blocks (method details, "no destinations", the how-it-works note) plus two field hints, so the form read as a wall of text and the instruction to follow was buried in the middle | **Fixed:** the how-it-works sentence moved to the **top** as plain copy; only blocks that block progress remain (`vpn-warn`); both inputs now carry a `hint` stating what is expected ("Enter what you transferred…", "A bank reference makes confirmation faster"). One stray double space in `className` also removed | ✅ **CLOSED** |
| **U12** | `/history` | The pending-order block and the records table had no visual link — two unrelated screens on one page | **Fixed:** the pending card's actions row carries a same-page anchor **"Jump to records ↓"** to `#records`; the records card has `id="records"`. No new route | ✅ **CLOSED** |
### 2b. Found by the live browser check (Pass 19) — added after the first audit

| ID | Page | Defect | Evidence | Fix | Status |
|---|---|---|---|---|---|
| **U22** | `/client` | An account with **zero VPNs sees the same message four times**: the "Get started" card says *"You don't have a VPN yet"*, then the Premium card says *"No premium VPNs yet"*, the Free card says *"You have no free VPNs"*, and the offers card says *"No free offers are available"*. Four empty states stacked read as four separate failures | Playwright accessibility snapshot of `/client` as `client@veylix.test` (0 owned VPNs): headings `You don't have a VPN yet` → `No premium VPNs yet` → `You have no free VPNs` → `No free offers are available` | When the account owns nothing, the two "your VPNs" lists collapse into **one** quiet line instead of two full empty-state blocks; the Get started card keeps the call to action | ✅ **CLOSED** |
| **U25** | `/client` | Even after U22, the "Get started" card **and** a line directly beneath it said the same thing ("You don't have a VPN yet" twice) | Playwright a11y snapshot after the U22 fix: `heading "You don't have a VPN yet"` then `paragraph "You don't own a VPN yet…"` two nodes later | The Get started card is now the **only** statement of that fact and carries both actions; the duplicate line was deleted. Final live structure: `My VPNs` → `Get started` → `Get a free VPN` → `Purchases` | ✅ **CLOSED** |
| **U26** | 8 screens | **Internal specification references leaked into user-facing copy**: `(REQUIREMENTS §2.5)`, `(REQUIREMENTS §2.2)`, `(REQUIREMENTS §2.4)`, `(REQUIREMENTS §2.6)`, `(REQUIREMENTS §8)`, `(Bible §3.3)`, `(D-082)` and `(Phase-5 UX, C1 / REQUIREMENTS §2.0.1)` — a customer reading a page subtitle was shown product-internal citations | `BuyPremiumPage:79`, `HistoryPage:161`, `AdminServersPage:61`, `ChatPage:146`, `AdminsPage:206`, `AuditPage:263`, `OffersPage:245`, `PaymentsPage:201`, `VPNDetailPage:577`, `AdminOverviewPage:116`, `UserPickerDialog:63` | Every subtitle rewritten in plain language that says the same thing; the citations moved into the code comments, where they belong. Verified live: `/buy` and `/history` now contain **zero** matches for `REQUIREMENTS §|Bible §|D-\d{3}` in rendered text | ✅ **CLOSED** |


| **U23** | `/client` | **Three different names for one page**: the nav says **"My VPNs"**, the `PageHeader` says **"My VPNs"**, the section heading says **"Your VPNs"**, and the cards are titled **"My Premium VPNs / My Free VPNs"**. "My" and "Your" for the same objects on one screen | a11y snapshot: `heading "My VPNs" [level=1]` then `heading "Your VPNs" [level=2]` then `heading "My Premium VPNs"` | One possessive everywhere on the client portal: **"My"** (the sidebar already says "My VPNs", and "My VPNs / My clients" is the established client-side convention in `navModel.ts`) | ✅ **CLOSED** |
| **U24** | `/client` | `GET /api/me` answers **401 twice** on a cold load before sign-in settles — the auth store probes before the session cookie is established | Playwright console on `/login` → `/client`: `Failed to load resource: 401 @ /api/me` ×2. **Benign** (the guard retries and the page renders correctly), but it pollutes the console and would hide a real 401 | Recorded, **not fixed**: the probe is correct behaviour and hiding it needs a deliberate auth-store change, which is not a presentation fix. See `docs/UI_UX_DEFECT_BOARD.md` §4 | ⚪ **WITHDRAWN** — not a UI defect |



## 3. P2 — Consistency and polish

| ID | Area | Defect | Fix | Status |
|---|---|---|---|---|
| **U13** | `.tsx` files | **224** inline `style={{…}}` occurrences; repeated font sizes / gaps / colours written per site | Move repeated patterns into page CSS classes using tokens | 🟡 **OPEN** |
| **U14** | `components/ui/QrCode.tsx:68` | Hard-coded `#ffffff` / `#000000` QR modules — deliberate for scannability but undocumented | Document; keep the values | 🟡 **OPEN** |
| **U15** | `components/charts/LineChart.tsx:10` | `var(--chart-N, #hex)` fallbacks must agree with `theme/themes.ts` | Verify and note | 🟡 **OPEN** |
| **U16** | Admin pages | `EmptyState` is used on `/client`, `/history`, `/buy`; the three Admin pages use a bare `<p>` | Standardise on `EmptyState` | 🟡 **OPEN** |
| **U17** | Loading states | Mixed `Skeleton` and inline "Loading…" text | `Skeleton` for lists, inline text only for one-line reads | 🟡 **OPEN** |
| **U18** | A11y | Focus ring not verified on `SavedViewsBar`, `Pagination`, table sort headers | Audit against `--ring-focus` | 🟡 **OPEN** |

## 4. P3 — Missing capability (needs an owner decision)

| ID | Item | Note | Status |
|---|---|---|---|
| **U19** | Per-client FREE monthly cap screen | Backend shipped (`ClientSetFreeLimit` + `regression_d076_test.go`); no UI | ⛔ **BLOCKED** — owner decides whether it is a UI task |
| **U20** | `/vpn/:id/usage` route | Analytics live inside the detail page; the nav has no entry | ⛔ **BLOCKED** — needs a route decision |
| **U21** | No frontend unit runner (register **D10**) | Adding `vitest` to a 1 vCPU / 500 MB product is an owner trade-off | ⛔ **BLOCKED** |

## 5. Gates used to close a row

From `./frontend/`:

```
npm run lint
npm run build
npm run audit
npm run audit:selftest
npm run docs:verify
```

A row that changes a route's rendering also needs `npm run verify:ui -- <route>`.
A console-clean claim needs the browser console, not the build.

---

## 6. S3 — Client UI foundation (2026-11-08)

**Nothing here is `CLOSED`.** S3 is the foundation stage: it split a screen and
added a navigation model. Where a row below moved, it is marked **ADDRESSED —
awaiting its own stage's gate**, because the defect is only closed when that
section's screen and browser gate are green.

### 6.1 Rows moved by S3

| ID | What changed | Status after S3 |
|---|---|---|
| **F-1** (audit) — `/client` was two products in one page (dashboard + inventory + free-claim shop, ~610 lines) | Split: `/client` is now the **Dashboard** (verdict, counters, primary connection, free-claim card, purchases link); the owned inventory moved to **`/client/vpns`**. Both render with `consoleErrors: []`, verified. | **ADDRESSED** — 3 cards on `/client`, 1 empty state on `/client/vpns`, both console-clean. Closed only after S4/S5 finish their own gates. |
| **F-2** (audit) — no client filtering, search, sort or pagination anywhere | `/client/vpns` now has search, category / status / server / expiry filters, three sorts and paging, built on the **existing** `useListQuery` (URL-as-truth) and the previously-unused `Pagination` primitive. | **ADDRESSED** — the infrastructure exists and renders. Its behaviour could not be exercised end-to-end because the seeded client owns no VPN (pre-existing data gap, `QA_REPORT.md` S3). Closed when a client-owned VPN exists to filter. |
| **F-9** (audit) — responsive coverage thin on client pages (`client.css` had one breakpoint) | New `components/client/client.css` states 360 / 390 / 768 / 1024 / 1440 / 1920 explicitly. Verified: 0 px horizontal overflow at all six widths on both client pages; grid resolves 1→2→3→4→5 columns. | **ADDRESSED**, verified at all six widths. Not `CLOSED` because S4–S11 will add surfaces that need the same sweep. |
| **U23** — "My"/"Your" mixed on one page | `/client` header is now **Dashboard** and `/client/vpns` is **My VPNs**; the section heading inside is "My VPNs" / "My Free VPNs" / "My Premium VPNs". One possessive. | **ADDRESSED** — remains `CLOSED` from Pass 20; S3 did not regress it. |

### 6.2 New rows found during S3

| ID | Area | Defect | Evidence | Status |
|---|---|---|---|---|
| **S3-1** | `frontend/scripts/verify-mobile.mjs` | Measures **only 390 px**. The design brief requires 360 / 390 / 768 / 1024 / 1440 / 1920, so four of six widths have no committed gate. S3 measured them with a throwaway probe; that evidence is in `QA_REPORT.md` but is **not reproducible by a committed script**. | `scripts/verify-mobile.mjs` fixes the viewport at 390×844 | 🟡 **OPEN** — needs the width list parameterised, then a committed multi-width gate. Not fixed in S3 (script changes outside the approved allowlist). |
| **S3-2** | `frontend/scripts/verify-chat.mjs` | Spawns its **own** `npx vite --port 5173 --strictPort`, so it cannot run while the dev server holds 5173 — it fails at sign-in with `(none)`. | `scripts/verify-chat.mjs:27` | 🟡 **OPEN — pre-existing**, not caused by S3. Same family as the missing `assertPortFree` / `profileDirUnique` noted in `docs/cline/activeContext.md`. |
| **S3-3** | Seed data | The seeded **client owns no VPN**, so every client-VPN gate refuses: `verify:vpn` (client), `verify-client-vpn.mjs`, and any real exercise of the new list/filters. Already recorded in `activeContext.md`, now confirmed to block S5 too. | `verify-vpn-detail.mjs` → *"the client persona owns no VPN, so `/vpn/:id` has nothing to render"* | 🟡 **OPEN — pre-existing data gap**, not a UI defect. Needs a seeded client-owned VPN before S5 or its gate is `NOT PERFORMED` again. |
| **S3-4** | `frontend/scripts/verify-mobile.mjs` | **Never enables touch emulation.** It imports only `profileDir` and never calls `Emulation.setTouchEmulationEnabled`, so `pointer` stays `fine`. Because D-058 scoped every general 44×44 rule to `@media (pointer: coarse)` (`styles/touch.css:13`), those rules are **inert** during the run — yet the script still reports an `under44px` count. That figure describes desktop density and cannot support any touch-target claim. | `verify-mobile.mjs:13` (imports), `:66-73` (no touch emulation), `:110` (`h < 44 \|\| w < 44`), `:131` (exits 0 on overflow alone) vs `styles/touch.css:13` and `audit-ui.mjs:114-119` (which *does* gate on `VEYLIX_TOUCH=1`) | 🟡 **OPEN — not fixed in this pass.** Closure requires: coarse emulation enabled, `matchMedia('(pointer: coarse)')` asserted, a hard failure when emulation is not confirmed, and a negative self-check proving the gate fails when emulation is disabled. Until then, **no `under44px: 0` from this script may be cited as evidence**. |
| **S3-5** | `frontend/src/components/client/client.css` | Four control families on the S3 Client surfaces were under the 44×44 thumb minimum (D-056 / D-058): `.client-filters__clear` **22×22**, `.client-filters__select` ≈**33 px** tall, `.client-filters__input` ≈**32 px** tall, and `.client-page .ui-pagination__btn` **32×32** (from the shared `ui.css:591` rule). None of them was covered by `touch.css` or `mobile.css`. | `client.css:102-106, 131-137, 160-166`; `components/ui/ui.css:591-594`; `styles/touch.css:13-71` (no client selectors) | 🟡 **OPEN — fix applied 2026-11-08, closure PENDING.** A `@media (pointer: coarse)` block at the end of `client.css` now sets `min-height:44px` on the input/select, `44×44` on the clear button, and `44×44` on the client-scoped pagination button. Desktop fine-pointer density is untouched, and `touch.css` / `mobile.css` / `ui.css` / shell components were **not** modified. **Verified shipped** in `dist` (`pointer:coarse){.client-filters__input,.client-filters__select{min-height:44px}.client-filters__clear{width:44px;height:44px}.client-page .ui-pagination__btn{min-width:44px;height:44px}}`). **Not yet CLOSED** because the committed coarse-pointer gate does not exist yet — closure requires `verify-mobile.mjs` to enable touch emulation, assert `matchMedia('(pointer: coarse)')`, and report `clientSurface.under44px === 0` at 360/390/768/1024/1440/1920. U18 remains OPEN and untouched. |

### 6.3 Not touched, and therefore not claimed

- **F-12** (internal spec citations in user-facing copy) — S3 added **no** copy
  containing a `§` citation, a `D-0xx` reference or a `REQUIREMENTS`/`Bible`
  mention; every new string is in plain language. But S3 did not *audit* the
  existing surfaces for it, so **F-12 is neither fixed nor addressed here**. The
  guard that enforces it (`npm run audit`) passes.
- **U13** (224 inline styles), **U16** (bare `<p>` empty states), **U17** (mixed
  loading states), **U18** (focus ring unverified) — all still **OPEN**. S3's new
  components use `EmptyState`, `Skeleton` and `AsyncState` correctly, which is the
  pattern those rows want propagated, but the rows cover the whole app and are
  untouched by a client-only foundation.
- **U19 / U20 / U21** — still **BLOCKED** on an owner decision. S3 deliberately
  did **not** add `/usage`, so U20 remains blocked exactly as recorded.

