# patch-docs.ps1 - byte-safe text replacement for the docs.
#
# Why this exists: docs/IMPROVEMENT_LEDGER.md carries a UTF-8 BOM and CRLF line
# endings, and several files already contain Latin-1 mojibake from earlier passes
# ("—" for an em dash). Editing them through a normal string replace can silently
# write an empty file or re-encode the whole document. This helper does exactly
# four things and refuses everything else:
#
#   1. refuses any search or replacement text that contains U+FFFD;
#   2. requires the search text to match EXACTLY ONCE - a partial match would
#      rewrite the wrong finding;
#   3. writes UTF-8 WITHOUT a BOM, preserving the original CRLF/LF style;
#   4. re-reads the file and proves the replacement landed before reporting OK.
#
# Usage:
#   . .\docs\scripts\patch-docs.ps1 -LoadOnly
#   Invoke-DocPatch -Path docs\SOMEFILE.md -Pairs @(
#       @{ Find = 'old text'; Replace = 'new text' },
#       @{ Find = 'other';    Replace = 'changed'  }
#   )
#   Invoke-DocPatch -Path docs\SOMEFILE.md -Check -Pairs @(@{ Find='x'; Replace='y' })
#
# Exit code 1 on any refusal, so it can gate a docs change. `Invoke-DocPatch`
# throws instead of exiting, so several files can be patched in one session.

param([switch]$LoadOnly)

$ErrorActionPreference = 'Stop'

function Invoke-DocPatch {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][object[]]$Pairs,
        [switch]$Check
    )

    function Fail([string]$message) {
        throw "patch-docs: $message"
    }

    $full = if ([System.IO.Path]::IsPathRooted($Path)) { $Path } else { Join-Path (Get-Location) $Path }
    if (-not (Test-Path -LiteralPath $full)) { Fail "no such file: $full" }

    $bytes = [System.IO.File]::ReadAllBytes($full)
    $hadBom = ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF)
    $text = [System.Text.Encoding]::UTF8.GetString($bytes)
    if ($hadBom) { $text = $text.Substring(1) }
    if ($text.Contains([char]0xFFFD)) { Fail "$Path contains U+FFFD before we touch it - fix the encoding first" }

    $crlf = ([regex]::Matches($text, "`r`n")).Count
    $lf = ([regex]::Matches($text, "`n")).Count
    $newline = if ($crlf -gt ($lf - $crlf)) { "`r`n" } else { "`n" }

    # Normalise every pair to the file's line endings, so a pair written with plain
    # \n still matches a CRLF document.
    $normalised = @()
    foreach ($pair in $Pairs) {
        $find = [string]$pair.Find
        $repl = if ($null -eq $pair.Replace) { '' } else { [string]$pair.Replace }
        if ($find.Length -eq 0) { Fail 'an empty search string would match everywhere' }
        if ($find.Contains([char]0xFFFD) -or $repl.Contains([char]0xFFFD)) {
            Fail 'refusing text containing U+FFFD (a lost encoding round-trip)'
        }
        if ($newline -eq "`r`n") {
            $find = $find -replace "`r`n", "`n" -replace "`n", "`r`n"
            $repl = $repl -replace "`r`n", "`n" -replace "`n", "`r`n"
        }
        $count = ([regex]::Matches($text, [regex]::Escape($find))).Count
        if ($count -ne 1) {
            $preview = $find.Substring(0, [Math]::Min(120, $find.Length))
            Fail "search text matched $count times (expected exactly 1):`n$preview"
        }
        $normalised += @{ Find = $find; Replace = $repl }
    }

    if ($Check) {
        Write-Output "patch-docs: CHECK OK - $($normalised.Count) pair(s) match exactly once in $Path"
        return
    }

    $backup = "$full.bak"
    [System.IO.File]::WriteAllBytes($backup, $bytes)

    $updated = $text
    foreach ($pair in $normalised) { $updated = $updated.Replace($pair.Find, $pair.Replace) }

    [System.IO.File]::WriteAllText($full, $updated, (New-Object System.Text.UTF8Encoding($false)))

    # Prove it landed; roll back byte-for-byte if not.
    #
    # The check counts occurrences rather than testing presence: a replacement is
    # allowed to CONTAIN its own search text (an insertion anchored on a heading
    # must keep that heading), so "still present" alone would be a false failure.
    $verify = [System.IO.File]::ReadAllText($full, [System.Text.Encoding]::UTF8)
    $bad = $false
    foreach ($pair in $normalised) {
        if (-not $verify.Contains($pair.Replace)) {
            Write-Warning "replacement did not land: $($pair.Replace.Substring(0, [Math]::Min(80, $pair.Replace.Length)))"
            $bad = $true
        }
        $before = ([regex]::Matches($text, [regex]::Escape($pair.Find))).Count
        $after = ([regex]::Matches($verify, [regex]::Escape($pair.Find))).Count
        # One search occurrence was replaced by one replacement. The replacement may
        # legitimately embed the search text (an insertion keeps its own anchor), so
        # what remains must equal exactly what the replacement itself supplies.
        $supplied = ([regex]::Matches($pair.Replace, [regex]::Escape($pair.Find))).Count
        if ($after -ne ($before - 1) + $supplied) {
            Write-Warning "search-text accounting failed: was $before, now $after, replacement supplies $supplied"
            $bad = $true
        }
    }
    if ($verify.Contains([char]0xFFFD)) {
        Write-Warning 'the result contains U+FFFD - encoding was damaged'
        $bad = $true
    }

    if ($bad) {
        [System.IO.File]::WriteAllBytes($full, $bytes)
        Fail 'rolled back - the file is byte-identical to before this run'
    }

    Remove-Item -LiteralPath $backup -Force
    $newBytes = (Get-Item -LiteralPath $full).Length
    Write-Output "patch-docs: OK - $($normalised.Count) replacement(s), $newBytes bytes, newline=$(if ($newline -eq "`r`n") {'CRLF'} else {'LF'})"
}

if (-not $LoadOnly) {
    Write-Output 'patch-docs: loaded - call Invoke-DocPatch -Path <file> -Pairs @(@{Find=...;Replace=...})'
}

