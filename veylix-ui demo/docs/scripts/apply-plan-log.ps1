# apply-plan-log.ps1 - appends this pass's result to the progress log in
# MILESTONE_7_PLAN.md. Kept separate from the table corrections so each script can
# be re-run (or skipped) on its own without duplicating an entry.
#
# Run from the repository root:
#   .\docs\scripts\apply-plan-log.ps1

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $root
. .\docs\scripts\patch-docs.ps1 -LoadOnly

$entry = '| 2026-10-25 | **Review pass - audit only, one P1 fixed** | Ran every gate on the current tree: `go build`/`vet`/`gofmt` clean, `go test ./... -count=1` **11 packages ok** (370 test functions), `tsc -b`, `oxlint` 0/0, `npm run audit` 0 findings, `npm run qr:selftest` 9/9. **Fixed one real defect:** the QR encoder refused every `vmess://` link over 213 bytes (M7-14) - versions 11-15 added, cap now 412 bytes, the self-test grown from 7 to 9 cases. **Closed the named-test gap** for `TestOrderTimeline`, `TestVPNUsageAgainstCapacity` and `TestAssignRenamesPanelClient` (M7-20); the other four names are corrected in the tables above, not quietly dropped. **Recorded, not built:** M7-15 (P0 - `/buy` still lets the buyer type the price), M7-16 (15 backend routes with no UI caller; five Phase-L screens missing), M7-17 (P-01/P-02/P-04/P-06/P-09 not on `/vpn/:id`; P-05 not built), M7-18 (README and `API_CONTRACT.md` drift), M7-19 (the audit gate cannot see M7-15/M7-16). Details: `docs/IMPROVEMENT_LEDGER.md`. **Phase L was deliberately not started in this pass** - it is the next unit of work, in the order L4, L2, L7, L1, L3, L5. |'

$lastRow = '| 2026-10-25 | **Next** | P-05 (client-side ping), P-03/P-07/P-08 (chart switches), then Phase L (UI surfaces). |'

Invoke-DocPatch -Path 'docs\MILESTONE_7_PLAN.md' -Pairs @(
    @{ Find = $lastRow; Replace = $lastRow + "`n" + $entry }
)
