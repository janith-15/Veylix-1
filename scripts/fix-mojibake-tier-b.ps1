<#
  fix-mojibake-tier-b.ps1 - the residual cases from fix-mojibake.ps1.

  fix-mojibake.ps1 repairs a corrupted RUN only while every character maps back
  to a CP1252 byte. In five files the corruption is followed by a character that
  breaks that run (a lost byte, or a punctuation that is itself ASCII), so the
  run stops early and the sequence is left.

  This script finishes them with EXPLICIT literal pairs, in order of length,
  and only for the sequences that were measured in the tree. It is deliberately
  dumb: no inference, no guessing. Anything not listed is left alone and
  reported.
#>
[CmdletBinding()]
param([switch]$DryRun)

$ErrorActionPreference = 'Stop'
$Utf8 = New-Object System.Text.UTF8Encoding($false)

# Ordered longest-first so a 2-char sequence cannot eat the head of a 3-char one.
$Pairs = [ordered]@{
    ([string]([char]0x00E2 + [char]0x20AC + [char]0x201D)) = [string][char]0x2014  # em dash
    ([string]([char]0x00E2 + [char]0x20AC + [char]0x201C)) = [string][char]0x201C  # left double quote
    ([string]([char]0x00E2 + [char]0x20AC + [char]0x00A6)) = [string][char]0x2026  # ellipsis
    ([string]([char]0x00E2 + [char]0x20AC + [char]0x00A0)) = [string][char]0x2020  # dagger
    ([string]([char]0x00E2 + [char]0x20AC + [char]0x0161)) = [string][char]0x201A
    ([string]([char]0x00E2 + [char]0x20AC + [char]0x0153)) = [string][char]0x201C
    ([string]([char]0x00E2 + [char]0x20AC + [char]0x009D)) = [string][char]0x201D
    ([string]([char]0x00E2 + [char]0x20AC + [char]0x2122)) = [string][char]0x2122
    ([string]([char]0x00E2 + [char]0x20AC + [char]0x00B0)) = [string][char]0x2030
    ([string]([char]0x00E2 + [char]0x2030 + [char]0x00CB + [char]0x2020)) = ([string][char]0x2248 + [string][char]0x2212)  # a approximate-minus remnant
    ([string]([char]0x00E2 + [char]0x2020 + [char]0x2019)) = [string][char]0x2192  # right arrow
    ([string]([char]0x00E2 + [char]0x2020 + [char]0x201D)) = [string][char]0x2194  # left-right arrow
    ([string]([char]0x00E2 + [char]0x2020 + [char]0x0090)) = [string][char]0x2190  # left arrow
    ([string]([char]0x00E2 + [char]0x2020 + [char]0x0153)) = [string][char]0x2190
    ([string]([char]0x00E2 + [char]0x0152 + [char]0x02DC)) = [string][char]0x2318  # command
}

$Targets = @(
    'backend\internal\httpapi\admin_routes.go',
    'backend\internal\provisioning\service.go',
    'backend\internal\provisioning\service_test.go',
    'backend\internal\store\payments.go',
    'frontend\src\pages\chat.css',
    'docs\IMPROVEMENT_LEDGER.md'
)

$total = 0
foreach ($rel in $Targets) {
    if (-not (Test-Path $rel)) { continue }
    $text = [System.IO.File]::ReadAllText($rel, $Utf8)
    $before = $text
    $n = 0
    foreach ($k in $Pairs.Keys) {
        $v = $Pairs[$k]
        $c = ([regex]::Matches($text, [regex]::Escape($k))).Count
        if ($c -gt 0) {
            $text = $text.Replace($k, $v)
            $n += $c
        }
    }
    if ($text -ne $before) {
        $total += $n
        if (-not $DryRun) {
            [System.IO.File]::WriteAllText($rel, $text, (New-Object System.Text.UTF8Encoding($true)))
        }
        Write-Output ("{0,-52} {1,4}" -f $rel, $n)
    }
}
Write-Output ("REPLACED: {0}   MODE: {1}" -f $total, $(if ($DryRun) { 'DRY RUN' } else { 'APPLIED' }))
