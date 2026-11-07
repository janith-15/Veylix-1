# apply-ledger-findings.ps1 - appends the 2026-10-25 review findings to the ledger.
#
# Content lives in docs/scripts/ledger-findings.md and is inserted after the
# "M7-12" entry, before the "## Change log" heading. Run from the repo root:
#   .\docs\scripts\apply-ledger-findings.ps1
#
# Split into two runs on purpose: each Invoke-DocPatch call keeps the change small
# enough to review, and the helper refuses (and rolls back) on any ambiguity.

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $root
. .\docs\scripts\patch-docs.ps1 -LoadOnly

$ledger = 'docs\IMPROVEMENT_LEDGER.md'
$source = Join-Path $root 'docs\scripts\ledger-findings.md'

if (-not (Test-Path -LiteralPath $source)) {
    throw "apply-ledger-findings: missing $source"
}

$findings = [System.IO.File]::ReadAllText($source, [System.Text.Encoding]::UTF8)
if ($findings.Contains([char]0xFFFD)) {
    throw 'apply-ledger-findings: the findings source contains U+FFFD'
}

# Anchor: the blank line pair before "## Change log".
$anchor = "`n`n## Change log"

Invoke-DocPatch -Path $ledger -Pairs @(@{ Find = $anchor; Replace = "`n" + $findings + "`n## Change log" })
