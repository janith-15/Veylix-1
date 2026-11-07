---

## 2026-10-25 review pass — findings recorded, nothing rebuilt

Scope: read the tree, ran every gate, compared `docs/new_reqierment/new_update.md`
and `docs/MILESTONE_7_PLAN.md` against the code, and fixed only what could be
proved. Phase L (the screens) was deliberately **not** started — it is recorded
here as remaining work, not claimed.

**Gates actually run (2026-10-25):** `go build ./...` · `go vet ./...` ·
`gofmt -l .` (empty) · `go test ./... -count=1` → **11 packages ok** · `npx tsc -b` ·
`npm run lint` → 0 warnings / 0 errors · `npm run audit` → 0 findings ·
`npm run qr:selftest` → **9/9**.

### M7-13 · P1 · `SetClientTag` never locked a VPN tagged after issue

- **Affected:** `backend/internal/store/tags.go` (`SetClientTag`)
- **Evidence:** a tag could be rewritten on a VPN that already had a live
  `vpn_instances` row, so an issued, paid-for VPN could still be re-priced.
- **Found by:** the Phase J tests (`TestIssuingVPNLocksItsTag`), not by reading.
- **Fix:** the write locks when a live VPN row exists; any later change answers
  `ErrTagLocked`.
- **Tests:** `TestIssuingVPNLocksItsTag` (green).
- **Status:** `FIXED-VERIFIED (code + suite)`.

### M7-14 · P1 · The QR refused every `vmess://` link (found this pass)

- **Affected:** `frontend/src/lib/qr.ts`, `frontend/scripts/qr-selftest.mjs`
- **Evidence:** the encoder was capped at version 10 (level M = **213 bytes**). A
  `vmess://` link is base64 JSON and routinely 250–360 bytes, so the customer saw
  "too long to encode" instead of a QR — while `vless://` worked, which is why the
  original self-test passed.
- **Root cause:** the scope note ("versions 1–10 … fits well inside that") was an
  assumption about link length that was never measured.
- **Fix:** versions **11–15** added (`TOTAL_CODEWORDS`, `RS_BLOCKS_M` rows derived
  from the level-M spec tables, `ALIGNMENT` centres), cap now **412 bytes**, and
  `MAX_VERSION` replaces the literal `10`. The error now states the byte length and
  the real limit.
- **Evidence:** each row's `data + ecc` equals the spec's codeword total (the
  encoder asserts this at run time). `npm run qr:selftest` **9/9**, including a
  264-byte `vmess://` link that must use a version above 10.
- **Regression risk:** larger modules for long links (v15 = 77×77). A phone camera
  reads that at `QrCode`'s render size, but this was **not** scan-verified with a
  physical camera.
- **Status:** `FIXED-VERIFIED (code + self-test)`; not camera-verified.

### M7-15 · P0 · The buyer still types their own price

- **Affected:** `frontend/src/pages/BuyPremiumPage.tsx:49-72`
- **Evidence:** `amountCents: Math.round(units * 100)` from a free-text input,
  posted straight to `POST /api/payments`. `api.ts` has no `/api/orders/quote`
  client, so the backend's server-computed price is unreachable from the app.
- **Impact:** revenue integrity — `new_update.md` §6 makes this the boundary the
  product must not cross, and it does.
  Owner's rule: *"price eka calculate wela pennanna one"*.
- **Why `npm run audit` misses it:** the gate looks for *invented literal* prices,
  not a user-supplied amount (M7-19).
- **Planned change:** build L4 `/pricing` (rate card), then rewrite `/buy` (L1) to
  quote via `/api/orders/quote` and drop the amount input.
- **Status:** `OPEN` — the highest-priority defect in the tree.

### M7-16 · P1 · Phase L is not built: the backend has no screen

- **Affected:** `frontend/src/App.tsx` (routes), `frontend/src/lib/api.ts`
- **Evidence:** routes are `/dashboard /servers /offers /admins /audit /admin
  /admin-servers /admin-clients /sale /retail-price /client /vpn/:id /buy /history
  /payments /notifications /profile /chat`. **Missing:** `/servers/:id` (G7/L7),
  `/orders` (L2), `/orders/:id` (L3), `/pricing` (L4), `/packages` (L5). `api.ts`
  has no client for `/api/pricing`, `/api/packages`, `/api/orders/quote`, the five
  `/api/payments/{id}/…` order routes, `/api/payments/{id}/release-target`, the
  three `/api/vpns/{id}/…` routes or `/api/servers/{id}/panel-clients` — **15 routes**
  are implemented and tested on the server with no caller.
- **Impact:** the owner's order → confirm → VPN → block flow cannot be performed in
  the product, only by hand against the API.
- **Planned change:** L4 → L2 → L7 → L1 → L3 → L5 in that order (L1 closes M7-15).
- **Status:** `OPEN`.

### M7-17 · P1 · The Premium VPN page shows a fraction of P-01 … P-10

- **Affected:** `frontend/src/pages/VPNDetailPage.tsx`
- **Evidence:** the page renders status, limits, one usage bar, links, the QR and a
  setup guide. It never calls `/usage?range=`, `/protocol` or `/traffic/live`, all
  of which exist and are tested. **P-05** is still server-side reachability,
  honestly labelled — but the owner asked for a ping *from the client's own
  device*, so `TestPingScopeIsClientNotPanel` is absent by construction.
- **Planned change:** wire P-01/P-02/P-04/P-06/P-09 into the page, then implement
  P-05 in the browser and label its scope.
- **Status:** `OPEN` (the backend portions are `FIXED-VERIFIED`).

### M7-18 · P2 · Docs drifted from the code (README, API contract)

- **Affected:** `README.md:48,57,95,99`, `docs/API_CONTRACT.md`
- **Evidence:** README claims "7 migrations · schema v7" (there are **11**),
  "72 HTTP routes" (**84** `mux.Handle` calls), "9/9 packages" (**11**) and
  "**286** tests" (the tree has **367** `func Test…`), and lists "panel-tagging UI"
  as not built although `PUT /api/panel-clients/…/tag` shipped with 5 tests.
  `API_CONTRACT.md` (2026-10-24) predates the 15 routes in M7-16.
- **Why it matters:** `new_update.md` §13 forbids reporting more than was proved; a
  stale README is the same defect in a quieter form. `npm run audit` only inspects
  `src/`, never the markdown.
- **Planned change:** a `docs:verify` check that recomputes these counts from the
  tree and fails on drift; correct the text in the same change.
- **Status:** `OPEN`.

### M7-19 · P3 · The gates cannot see the two defects that matter most

- **Affected:** `frontend/scripts/audit-ui-truth.mjs`
- **Evidence:** the UI-truth audit passes while `BuyPremiumPage` still accepts a
  typed amount (M7-15) and while five plan-mandated screens do not exist (M7-16).
  It catches invented literals and "demo data" copy — not an unwired form or a
  missing route.
- **Planned change:** add a rule that a screen may not post a money amount
  collected from a text input, and a route-inventory check that names any
  plan-mandated path absent from `App.tsx`.
- **Status:** `PLANNED`.

### M7-20 · P1 · Seven tests named by the plan did not exist

- **Affected:** the plan ↔ code naming contract (`MILESTONE_7_PLAN.md` §8 gate)
- **Evidence (before this pass):** `TestVPNRowCarriesOwner`,
  `TestClientSeesOwnVPNEvenUntagged`, `TestOrderTimeline`,
  `TestMethodExposesChannels`, `TestVPNUsageAgainstCapacity`,
  `TestAssignRenamesPanelClient` and `TestQRPayloadIsPanelLink` did not exist, so
  the plan's "every named test present" gate could not pass even where the
  behaviour was covered under another name.
- **Closed this pass:** `TestOrderTimeline` (renamed; the old name is kept as a
  forwarder), `TestVPNUsageAgainstCapacity` (written),
  `TestAssignRenamesPanelClient` (written). `TestMethodExposesChannels` was a typo
  for `TestPaymentMethodExposesChannels`, now stated in the plan.
- **Recorded, not written:** `TestVPNRowCarriesOwner` and
  `TestClientSeesOwnVPNEvenUntagged` — the behaviour is proved by
  `TestSQLVPNListerClientSeesOwnVPNs`, and the plan's names are corrected here.
  `TestQRPayloadIsPanelLink` — proved by `npm run qr:selftest` (a node script, not
  `go test`). `TestPingScopeIsClientNotPanel` — P-05 is not built (M7-17).
- **Status:** `PARTIAL` — 4 of 7 closed, 3 recorded as corrections.

