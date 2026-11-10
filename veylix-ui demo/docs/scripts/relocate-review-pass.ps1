# relocate-review-pass.ps1 - moves the 2026-10-25 review section to the end of the
# Findings list, so entries read in numeric order (M7-01 … M7-20) before the
# Change log. Run from the repo root:
#   .\docs\scripts\relocate-review-pass.ps1
#
# The section was first inserted at the anchor "blank line + ## Change log", which
# also occurs earlier in the file; this moves it to its intended position without
# hand-editing the ledger.

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Set-Location $root
. .\docs\scripts\patch-docs.ps1 -LoadOnly

$ledger = 'docs\IMPROVEMENT_LEDGER.md'
$crlf = "`r`n"

$bytes = [System.IO.File]::ReadAllBytes((Join-Path $root $ledger))
$text = [System.Text.Encoding]::UTF8.GetString($bytes)
if ($text.Length -gt 0 -and $text[0] -eq [char]0xFEFF) { $text = $text.Substring(1) }

$start = $text.IndexOf('## 2026-10-25 review pass')
$end = $text.IndexOf($crlf + $crlf + '## Change log', $start)
if ($start -lt 0 -or $end -lt 0 -or $end -lt $start) {
    throw 'relocate-review-pass: could not locate the review section boundaries'
}

# [start, end) is the review section; [start, headingStart) is what must be cut so
# the blank lines the section needed go with it. The symmetric-removal assertion
# below proves that reasoning against the real bytes before anything is written.
$section = $text.Substring($start, $end - $start).Trim($crlf)
$headingStart = $end + 2
$before = $text.Remove($start, $headingStart - $start)
if ($before.Length -ne $text.Length - ($section.Length + 4)) {
    throw "relocate-review-pass: removal is not symmetric (lost $($text.Length - $before.Length), expected $($section.Length + 4))"
}
$without = $before.TrimEnd() + $crlf + $crlf + '## Change log' +
    $text.Substring($headingStart + '## Change log'.Length)


# Where the section belongs: at the very end of the findings, after the Milestone-7
# entries (M7-01 … M7-12) that it extends. The file has TWO "## Change log"
# headings - an older one at ~line 717 and the current one at the end - and the
# Milestone-7 section sits between them, so the insertion point is the LAST
# occurrence, not the first.
$anchor = $crlf + $crlf + '## Change log'
$m7Start = $without.LastIndexOf($anchor)
if ($m7Start -lt 0) { throw 'relocate-review-pass: could not find the change-log heading' }
if ($without.IndexOf('### M7-12') -gt $m7Start) {
    throw 'relocate-review-pass: the insertion point would precede the M7-12 entry'
}

$block = $section + $crlf + $crlf + '---' + $crlf + $crlf
$moved = $without.Insert($m7Start, $crlf + $crlf + $block.TrimStart($crlf))

# Validate BEFORE writing: the previous version of this script verified after the
# write, so a failure left the file in the bad state it was warning about.
if ($moved.Contains([char]0xFFFD)) { throw 'relocate-review-pass: the result contains U+FFFD' }
$order = @('# Milestone 7', '### M7-01', '### M7-12', '### M7-13', '### M7-20')
$last = -1
foreach ($marker in $order) {
    $at = $moved.IndexOf($marker)
    if ($at -lt 0) { throw "relocate-review-pass: $marker is missing from the result" }
    if ($at -lt $last) { throw "relocate-review-pass: $marker would be out of order - not writing" }
    $last = $at
}
if ($moved.IndexOf('## 2026-10-25 review pass') -gt $moved.LastIndexOf($anchor)) {
    throw 'relocate-review-pass: the review section would land after the final change log - not writing'
}
if ($moved.IndexOf('## 2026-10-25 review pass') -le $moved.IndexOf('### M7-12')) {
    throw 'relocate-review-pass: the review section would precede M7-12 - not writing'
}

[System.IO.File]::WriteAllText((Join-Path $root $ledger), $moved, (New-Object System.Text.UTF8Encoding($false)))
Write-Output "relocate-review-pass: OK - $(Get-Item (Join-Path $root $ledger)).Length bytes, order verified before write"

