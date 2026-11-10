# apply-ledger-updates.ps1 - records the 2026-10-25 review pass in IMPROVEMENT_LEDGER.md.
#
# Run from the repository root:
#   .\docs\scripts\apply-ledger-updates.ps1
#
# Every replacement is a literal (never a regex) and must match exactly once; the
# helper rolls the file back byte-for-byte if any of them does not land. See
# docs/scripts/patch-docs.ps1 for why this exists.

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $root

. .\docs\scripts\patch-docs.ps1 -LoadOnly

$pairs = @(
    # ---- M7-09: CUSTOM release target (I5) -------------------------------
    @{
        Find    = '- **Status:** `PLANNED` (I5 / L3)'
        Replace = '- **Status:** `FIXED-VERIFIED (code + suite)` - 2026-10-25 (backend). `SetReleaseTarget` (server must exist, inbound must belong to it, refused once released, audited as a `RELEASE_TARGET` event) and `POST /api/payments/{id}/release-target` (Super Admin only) exist. `payments.Confirm` refuses a custom order with `ErrReleaseTargetRequired` BEFORE reaching provisioning, and passes `CreateInput.Spec`. Tests: `TestCustomOrderNeedsServerBeforeRelease`, `TestCustomOrderReleasesWithItsOwnSpec`, `TestReleaseTargetMustMatchServer`, `TestSpecOverridesPackageLimits`, `TestSpecIncompleteIsRefused`. **The L3 screen is not built** - there is no `/orders/:id` route, so a Super Admin cannot pick server then inbound in the UI (M7-16).'
    },
    # ---- M7-11: payment channels (I1) ------------------------------------
    @{
        Find    = '- **Status:** `PLANNED` (I1)'
        Replace = '- **Status:** `FIXED-VERIFIED (code + suite)` - 2026-10-25 (backend). `payment_methods` carries `contact_email`, `contact_whatsapp`, `contact_telegram` (`0011_order_events.sql`) and `GET /api/payment-methods` returns them; `TestPaymentMethodExposesChannels` proves all three reach the wire. **The click-to-copy and WhatsApp deep links this finding asks for are not built** - the payer UI renders none of the three fields yet (M7-16).'
    },
    # ---- M7-10: attribution / timeline / block (Phase I) -----------------
    @{
        Find    = '- **Status:** `PLANNED` (Phase I)'
        Replace = '- **Status:** `FIXED-VERIFIED (code + suite)` - 2026-10-25 (backend). An order now carries the buyer, both ends of a method move, a proof channel and block/unblock: `0011_order_events.sql`, `payment_record_events`, `ChangePaymentMethod`, `RecordProofChannel`, `BlockOrderVPN`/`UnblockOrderVPN`, plus `POST /api/payments/{id}/{method,proof,block,unblock}` and `GET .../events`. Tests: `TestMethodChangeIsRecorded`, `TestMethodChangeToSameIsNoop`, `TestMethodChangeIsPayerOnly`, `TestProofChannelIsRecorded`, `TestBlockAndUnblockVPN`, `TestOrderTimeline`, `TestOrderProofChannelOverHTTP`, `TestBlockWithoutVPNIsRefused`. The panel is written first and confirmed, so a local SUSPENDED row cannot disagree with it. **The UI is missing**: `frontend/src/lib/api.ts` has no client for any of those five routes and there is no `/orders` screen (M7-16).'
    },
    # ---- M7-12: the Premium VPN page (Phase K) ---------------------------
    @{
        Find    = '- **Status:** `PLANNED` (Phase K)'
        Replace = '- **Status:** `PARTIAL - backend complete, UI mostly absent` - 2026-10-25. Backend: `GET /api/vpns/{id}/usage` answers `lifetime_up_bytes`, `lifetime_down_bytes`, `lifetime_total_bytes`, `usage_pct` (clamped at 100), `remaining_bytes` (-1 when unlimited) and a `?range=7d|30d|3m|1y|all` switch (422 on a typo); `GET /api/vpns/{id}/protocol` (P-09) and `GET /api/vpns/{id}/traffic/live` (P-04) exist. Tests: `TestUsageRangeWindow`, `TestUsageHistorySplitsUpDownTotal`, `TestUsageRangeTypoIsRefused`, `TestVPNUsageAgainstCapacity` (added 2026-10-25), `TestProtocolDetailIsExposed`, `TestProtocolReportsOutageHonestly`, `TestLiveTrafficRate`, `TestLiveTrafficNeedsTwoReadings`. UI: only **P-10 (QR)** is wired into `/vpn/:id`. Still absent there: the up/down/total split (P-01), the range switch (P-06), live-rate chips (P-04), the protocol block (P-09), chart tabs and the chart-type switch (P-03/P-07/P-08), and **P-05** - the page still labels its ping "server reachability, not your own latency". Tracked as M7-17.'
    }
    # ---- M7-08: no price anywhere except the client's typing (Phase H) --
    @{
        Find    = '- **Status:** `PLANNED` (Phase H)'
        Replace = '- **Status:** `PARTIAL - backend complete, the screen still types its own amount` - 2026-10-25. Backend: `0010_pricing_packages.sql` (`pricing_rates`, global + per-server override), `domain.QuoteSpec` (integer cents, reports the floor it applied, refuses rather than clamps), `GET/PUT /api/pricing` (operator only, audited) and `POST /api/orders/quote`. Tests: `TestQuoteFromRates`, `TestQuoteAppliesMinimum`, `TestQuoteRejectsOutOfRange`, `TestQuoteRefusesWhenNoRatesSet`, `TestQuoteRefusesAboveCeiling`, `TestPricingFallsBackToPlatformDefault`, `TestServerRateOverridesGlobal`, `TestSavePricingIsUpsert`, `TestQuoteFromRatesOverHTTP`, `TestPricingReadRequiresOperator`. **The finding is NOT closed**: `frontend/src/pages/BuyPremiumPage.tsx` still posts `amountCents: Math.round(units * 100)` from a free-text box, and `api.ts` has no client for `/api/orders/quote`, so in the running app the customer still states the price. Raised to **M7-15 (P0)**.'
    }
)

Invoke-DocPatch -Path 'docs\IMPROVEMENT_LEDGER.md' -Pairs $pairs
