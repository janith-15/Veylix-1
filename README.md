# VEYLIX

A self-hosted VPN reseller platform: Go backend, React UI, and **3X-UI as the source of truth** for every VPN.
One binary serves both the API and the interface.

```
Go 1.27  ·  React 19 + TypeScript + Vite 8  ·  SQLite  ·  3X-UI 3.10.2
```

---

## Run it (2 commands)

```powershell
cd "h:\my apps\Veylix-2"
.\dev.ps1 -Restart -Preflight
```

Then open the door for the account you want (**staff and clients use different
portals**; the wrong one answers `403 wrong_portal`):

| Account | Open |
|---|---|
| `super_admin` / `admin` | **http://localhost:5173/staff/login** |
| `client` | **http://localhost:5173/login** |

Addresses are in `.veylix-secrets/seed.json` (printed by the script). Google
sign-in works once `VEYLIX_GOOGLE_CLIENT_ID/_SECRET` are set — see
[`docs/AUTH.md`](docs/AUTH.md) §2b. The script prints the URL only after it has
proved that anonymous calls are refused (**401**) and that a seeded account really
signs in; `-Preflight` runs the full 27-check gate (roles, refusals, logout, proxy).

Stop everything with `.\dev.ps1 -Stop`; restart with `.\dev.ps1 -Restart`; check
state with `.\dev.ps1 -Status`. Full guide (sign-in methods, Google setup, gate,
troubleshooting): [`docs/AUTH.md`](docs/AUTH.md) and
[`docs/LIVE_PREVIEW.md`](docs/LIVE_PREVIEW.md).

**Authentication is real, and only real.** The development bypass was removed on
2026-10-28 (`docs/DECISIONS.md` D-039): there is no auto-login, no persona switch
and no code path that grants a session without a password or a verified Google
sign-in.

## Build one binary

```powershell
.\dev.ps1 -Package
# -> backend\bin\veylixd.exe  (19 MB, UI embedded)
```

The packaged binary serves the app **and** the API on one port:

```
$env:VEYLIX_ADDR='127.0.0.1:8080'; .\backend\bin\veylixd.exe
# http://localhost:8080/dashboard
```

`-Package` builds the SPA, embeds it, builds the binary, then proves it: `/api/ready`
must report `ui: embedded`, `/dashboard` and `/vpn/16` must answer 200, `/assets/nope.js`
must stay 404, and the bundle must contain no dev bypass. See
[`docs/DECISIONS.md`](docs/DECISIONS.md) D-029.

## What is in the box

| Layer | Contents |
|---|---|
| **Backend** (`backend/`) | Go + SQLite (29 migrations · schema v29), argon2id auth with server-side sessions, RBAC + row-level ownership, audit log, **144 HTTP routes** |
| **3X-UI integration** | Bearer token sealed with AES-256-GCM, SSRF guard, per-request timeouts, client CRUD + HWIDs + connection links, provisioning **write → fresh read → record**, outbox with reconciliation, read-only sync + usage history |
| **Frontend** (`frontend/`) | React 19 + TS, 5 theme presets + 40 sub-themes + Theme Studio, 23 pages, **11 stores — all API-backed, 0 mocks** |
| **Product rules** | 3 concurrent FREE VPNs / 5 per 30 days, reseller quota counted in-transaction, 3-day release grace with local-only removal, panel wins every conflict |

## Verify it

```powershell
# backend
cd backend ; go build ./... ; go vet ./... ; go test ./... -count=1     # 11/11 packages

# frontend
cd frontend ; npm run lint ; npm run build                              # 0/0 lint
npm run audit                                                           # UI truth gate
npm run audit:selftest                                                  # 8 + 6 cases
npm run verify:ui -- /dashboard                                         # real browser render
npm run verify:tables                                                   # every operator list sorts/searches/filters

# everything, one source of truth
.\dev.ps1 -Status
```

## Documentation

Start at **[`docs/DOC_INDEX.md`](docs/DOC_INDEX.md)** (22 documents, grouped by question).

Most used:

| Question | Document |
|---|---|
| How do I run / preview it? | [`docs/LIVE_PREVIEW.md`](docs/LIVE_PREVIEW.md) |
| Which endpoint exists? | [`docs/API_CONTRACT.md`](docs/API_CONTRACT.md) |
| What works, what doesn't? | [`docs/FINAL_REPORT.md`](docs/FINAL_REPORT.md) §7 |
| Why was it built this way? | [`docs/DECISIONS.md`](docs/DECISIONS.md) (D-001 … D-029) |
| What was tested? | [`docs/QA_REPORT.md`](docs/QA_REPORT.md) |
| What defects existed? | [`docs/IMPROVEMENT_LEDGER.md`](docs/IMPROVEMENT_LEDGER.md) (F-001 … F-033) |

## Secrets

`.veylix-secrets/` is **git-ignored** and holds the local panel credentials and the
32-byte keys. Nothing in source, docs, logs or API responses carries a token
(verified by grep over the DB files and the production bundle). See
[`.veylix-secrets/README.md`](.veylix-secrets/README.md).

## Status

| | |
|---|---|
| Backend tests | **709** in 15 packages — all passing (counted 2026-11-10 by `npm run docs:verify`, `internal/` only; that gate keeps this honest; +5 are the D-116 order-list tests `TestPaymentListIsCappedButTheCountIsNot`, `TestPaymentCountMatchesTheListScope`, `TestPaymentCountRespectsThePendingFilter`, `TestPaymentCountOnAnEmptyDatabase`, `TestPaymentsEnvelopeStatesTruncation`; +2 are the D-114 panel-write confirmation tests `TestSetClientEnabledConfirmsThePanelActuallyChanged`, `TestSetClientEnabledSucceedsWhenThePanelReallyDisabledIt`; +2 are the D-113 order-identity tests `TestPaymentListNamesWhoPlacedIt`, `TestPaymentListStatesAnAbsentPackageAsEmpty`; +7 are the D-112 replace tests `TestReplaceCreatesTheNewVPNBeforeReleasingTheOld`, `TestReplaceBuildsADifferentVPNThenReleasesTheOld`, `TestReplaceRefusesAnotherOwnersVPN`, `TestReplaceRefusesWhenNoClaimRemains`, `TestReplaceRefusesAReleasedVPN`, `TestReplaceRefusesAKindChange`, `TestReplaceWorksAtTheConcurrentCap`; +1 is the D-111 operator-package-view test `TestPlanListCarriesTheRenderedFields`; +1 is the D-110 inbound-id regression test `TestInboundsReturnsLocalIDNotPanelID`; +11 are the D-107 overview tests `TestPaymentTotalsNetRevenue`, `TestPaymentTotalsIgnoresUndatedConfirmations`, `TestPaymentTotalsRefusesToSumCurrencies`, `TestPaymentTotalsOnAnEmptyDatabase`, `TestOverviewQueriesUseIndexes`, `TestResellerHealthCountsTheThingsOnlyAnOwnerCanFix`, `TestOverviewRefusesResellersAndClients`, `TestOverviewAnonymousIs401`, `TestOverviewReturnsBothSections`, `TestOverviewNamesAFailedSectionInsteadOfBlanking`, `TestOverviewWithoutAStoreSays503`; +16 are the Super Admin perfection tests `TestStaffRoutesRefuseNonSuperAdmins`, `TestStaffCreateRejectsUnknownRole`, `TestStaffCreateDefaultsEmptyRoleToAdmin`, `TestStaffResetPasswordRefusesSelf`, `TestStaffPatchValidationOrder`, `TestListClientsAdminOffsetFollowsClampedLimit`, `TestDialGuardRefusesLoopbackWithoutThePrivatePolicy`, `TestDialGuardRefusesCloudMetadataEvenWithThePrivatePolicy`, `TestAuditLogIsAppendOnly`, `TestOnlyOneSuperAdminCanExist`, `TestDemotingTheOwnerIsAllowedOnceOnly`, `TestSuperAdminReleaseRequiresAReason`, `TestForcePurgeKeepsThePanelClient`, `TestArchiveOfferRemovesItWithoutDestroyingIt`, `TestDeleteOfferRefusesOneThatHasBeenSold`, `TestPurgeRouteRefusesResellersAndAnonymous` +2 are the inbound-numbering regression tests `TestCreateSendsThePanelsInboundNumber` / `TestCreateRefusesInboundFromAnotherServer`; +9 are the D-092 usage/latency tests `TestUsageWindowNamedRanges`, `TestUsageWindowUnknownRangeIsRefused`, `TestResolveWindowCustomBounds`, `TestResolveWindowEchoesOneClock`, `TestUsageHourlyBucketsD000`, `TestSpeedHistoryDerivesRateFromCounters`, `TestNewUsageRoutesRefuseStrangers`, `TestNewUsageRoutesRefuseMisspelledRange`, `TestPingHistoryRequiresSampleStore`; +6 are the D-094 dormancy tests `TestDormantRoutePutsVPNToSleep`, `TestWakeRouteClearsDormancy`, `TestDormantIsIdempotent`, `TestDormantRouteRefusesOtherOwnersVPN`, `TestDormantRouteRefusesAnonymous`, `TestSyncLeavesDormantVPNStatusAlone`; +1 is the D-095 source-filter test `TestPingScopeFromQuery`; +1 is the D-096 bucket-width regression test `TestUsageRangesDifferFromEachOther`; +10 are the D-098 stale-lifecycle tests `TestStaleSuspendedVPNIsSetAsideAfterItsWindow`, `TestExpiredVPNUsesTheShorterWindow`, `TestServerUnavailableIsNeverSwept`, `TestReportModeChangesNothing`, `TestReportModeAuditSaysNothingChanged`, `TestEnforceModeRecordsUndoWindowAndKeptClient`, `TestSweepDoesNotDeleteTheRow`, `TestActiveVPNIsNeverSwept`, `TestUnsetClockIsNotTreatedAsAncient`, `TestStoreRefusesServerUnavailableAsLifecycleStatus`, `TestSweepIsIdempotent`; +2 are the D-098 mode/migration tests `TestResolveLifecycleModeDefaultsToOff`, `TestLoadDefaultsLifecycleToOff`, `TestLifecycleMigrationBackfillsToMigrationTime`, `TestLifecycleMigrationIsIdempotent`; +5 are the D-108 fulfilment-chain tests `TestOrderWithPlanBuildsTheVpnOnConfirm`, `TestConfirmingAPlanlessOrderSettlesCashOnly`, `TestPlanOrderRefusedWhenSettlementServiceIsMissing`, `TestOfferCreateRefusesAnInboundFromAnotherPanel`, `TestClientCannotConfirmPayments`) |
| Frontend | typecheck · lint · build · UI-truth gate — all green |
| Live-verified against 3X-UI 3.10.2 | health, inbounds, client read, **client create**, links, ping, sync, metrics |
| Browser-verified (2026-11-04) | `/orders`, `/orders/9`, `/packages`, `/pricing`, `/servers/11` render with **0 console errors** (`npm run verify:ui`) · nav gate **23/23** (`npm run verify:nav`) |
| Not live-verified | suspend / resume / revoke (blocked: production inbound, D-018) · Google sign-in (configured and exercised end-to-end, not against a live Google tenant) |
| Not built / status | **panel-tagging UI** (the `PUT /api/panel-clients/…/tag` route and its tests exist; no screen mounts it) · **HWID UI** (the device list and revocation routes shipped 2026-11-04 with 9 tests; the DeviceList component mounts it on the VPN detail page (staff only) — see `docs/DISCREPANCY_REGISTER.md` D8). The **one-line installer** now ships (`install.sh` + `vpn-platformctl`, D-079). |
