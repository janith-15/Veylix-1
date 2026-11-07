# apply-plan-updates.ps1 - records the 2026-10-25 review result in MILESTONE_7_PLAN.md.
#
# Run ONCE per repository state, from the root:
#   .\docs\scripts\apply-plan-updates.ps1     # the table/test-name corrections
#   .\docs\scripts\apply-plan-log.ps1         # the progress-log entry (separate, so
#                                             # each step is idempotent on its own)
#
# A second run refuses rather than duplicating: every search text must match exactly
# once, and the corrected tables no longer contain the old text.
#
# Nothing here changes a scope item: it corrects the TEST NAMES the §8 gate looks
# for and records the pass. Anything not proved is stated as open (`new_update.md` §13).

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $root
. .\docs\scripts\patch-docs.ps1 -LoadOnly

$pairs = @(
    # ---- §2 G4/G5: the named tests do not exist under these names ----------
    @{
        Find    = '| G4 | `GET /api/vpns` rows carry **`owner_email` + `owner_name` + `assigned`** so "whose VPN" is answerable on the wire (D-037.4) | `internal/httpapi/vpn_lister*.go` | `TestVPNRowCarriesOwner` |'
        Replace = '| G4 | `GET /api/vpns` rows carry **`owner_email` + `owner_name` + `assigned`** so "whose VPN" is answerable on the wire (D-037.4) | `internal/httpapi/vpn_lister*.go` | `TestSQLVPNListerClientSeesOwnVPNs` (asserts `OwnerEmail`/`Assigned` on every owned row; the originally-named `TestVPNRowCarriesOwner` was never written) |'
    },
    @{
        Find    = '| `internal/httpapi/vpn_lister.go` | `TestClientSeesOwnVPNEvenUntagged` |'
        Replace = '| `internal/httpapi/vpn_lister.go` | `TestSQLVPNListerClientSeesOwnVPNs` (name corrected 2026-10-25; `TestClientSeesOwnVPNEvenUntagged` was never written) |'
    },
    # ---- §4 I1: typo in the test name --------------------------------------
    @{
        Find    = '| `TestMethodExposesChannels` |'
        Replace = '| `TestPaymentMethodExposesChannels` |'
    },
    # ---- §4 I3: renamed to the gate name -----------------------------------
    @{
        Find    = '| new | `TestOrderTimeline` |'
        Replace = '| new | `TestOrderTimeline` (renamed 2026-10-25 from `TestOrderMethodChangeOverHTTP`, which is kept as a forwarder) |'
    },
    # ---- §5 J4: written this pass ------------------------------------------
    @{
        Find    = '| J4 | Rename once on assignment (D-034.5), audited | `TestAssignRenamesPanelClient` |'
        Replace = '| J4 | Rename once on assignment (D-034.5), audited | `TestAssignRenamesPanelClient` (written 2026-10-25) |'
    },
    # ---- §6 P-02 and P-10: written this pass / covered elsewhere -----------
    @{
        Find    = '| P-02 | Usage vs capacity | `limit_bytes`/`usage_pct` | progress ring | `TestVPNUsageAgainstCapacity` |'
        Replace = '| P-02 | Usage vs capacity | `limit_bytes`/`usage_pct` | progress ring | `TestVPNUsageAgainstCapacity` (written 2026-10-25) |'
    },
    @{
        Find    = '| P-10 | **QR + link, always** | links (exists) | in-repo QR encoder | `TestQRPayloadIsPanelLink` |'
        Replace = '| P-10 | **QR + link, always** | links (exists) | in-repo QR encoder | `npm run qr:selftest` (9 cases, incl. a 264-byte `vmess://` link; `TestQRPayloadIsPanelLink` is not a Go test) |'
    },
    @{
        Find    = '| *Test ping* button | `TestPingScopeIsClientNotPanel` |'
        Replace = '| *Test ping* button | **NOT BUILT** (M7-17): the page labels its ping "server reachability, not your own latency", so the gate test is absent - stated, not skipped. |'
    }
)

Invoke-DocPatch -Path 'docs\MILESTONE_7_PLAN.md' -Pairs $pairs
