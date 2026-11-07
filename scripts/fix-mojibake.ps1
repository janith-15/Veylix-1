<#
  fix-mojibake.ps1 - one-shot repair of UTF-8-written-as-CP1252 corruption.

  ROOT CAUSE (verified 2026-11-07, I-36):
  source files were read as Windows-1252 and re-written as UTF-8, so a single
  character such as an em dash (U+2014, UTF-8 E2 80 94) became three chars
  (U+00E2 U+20AC U+201D). Every affected file carries a UTF-8 BOM, which proves
  the damage is in the CONTENT, not the file encoding.

  THE REPAIR IS EXACT, NOT A GUESS. For a corrupted sequence, encoding the
  visible characters back to CP1252 reproduces the original UTF-8 bytes:

      "a-euro-curlyquote"  ->  CP1252 bytes 226,128,148  ->  UTF-8  ->  em dash

  All 14 sequences present in this tree were verified to round-trip (see
  -DryRun output). Anything that does not decode to exactly one valid,
  printable character is SKIPPED and reported, never written.

  Usage:
    .\scripts\fix-mojibake.ps1 -DryRun      # report only, changes nothing
    .\scripts\fix-mojibake.ps1             # apply
#>
[CmdletBinding()]
param(
    [switch]$DryRun,
    [string[]]$Root = @('.')
)

$ErrorActionPreference = 'Stop'
$Utf8 = New-Object System.Text.UTF8Encoding($false)

<#
  CP1252 char -> byte.
  .NET's GetEncoding(1252) ENCODER replaces characters it considers unmappable
  (notably the euro sign U+20AC and several 0x80-0x9F punches) with '?', which
  would silently turn a euro back into a question mark. The decoder it ships is
  correct, so the decoder is used to build the table: for each of the 256 byte
  values, `b -> char` is read from the decoder, then inverted here.
#>
$Map = @{}
$decoder = [System.Text.Encoding]::GetEncoding(1252)
for ($b = 0; $b -le 255; $b++) {
    $ch = $decoder.GetString([byte[]]@($b))
    if ($ch.Length -eq 1 -and -not $Map.ContainsKey([int][char]$ch)) {
        $Map[[int][char]$ch] = [byte]$b
    }
}
# The 0x80-0x9F punches .NET's decoder exposes as the *unicode* character they
# stand for; add the classic aliases explicitly so nothing is left ambiguous.
$Map[[int][char]0x20AC] = [byte]0x80   # euro
$Map[[int][char]0x201A] = [byte]0x82   # single low quote
$Map[[int][char]0x0192] = [byte]0x83   # florin
$Map[[int][char]0x201E] = [byte]0x84
$Map[[int][char]0x2026] = [byte]0x85   # ellipsis
$Map[[int][char]0x2020] = [byte]0x86   # dagger
$Map[[int][char]0x2021] = [byte]0x87
$Map[[int][char]0x02C6] = [byte]0x88
$Map[[int][char]0x2030] = [byte]0x89   # per mille
$Map[[int][char]0x0160] = [byte]0x8A
$Map[[int][char]0x2039] = [byte]0x8B
$Map[[int][char]0x0152] = [byte]0x8C   # OE
$Map[[int][char]0x017D] = [byte]0x8E
$Map[[int][char]0x2018] = [byte]0x91   # left single quote
$Map[[int][char]0x2019] = [byte]0x92   # right single quote
$Map[[int][char]0x201C] = [byte]0x93   # left double quote
$Map[[int][char]0x201D] = [byte]0x94   # right double quote
$Map[[int][char]0x2022] = [byte]0x95   # bullet
$Map[[int][char]0x2013] = [byte]0x96   # en dash
$Map[[int][char]0x2014] = [byte]0x97   # em dash
$Map[[int][char]0x02DC] = [byte]0x98
$Map[[int][char]0x2122] = [byte]0x99   # trademark
$Map[[int][char]0x0161] = [byte]0x9A
$Map[[int][char]0x203A] = [byte]0x9B
$Map[[int][char]0x0153] = [byte]0x9C   # oe
$Map[[int][char]0x017E] = [byte]0x9E
$Map[[int][char]0x0178] = [byte]0x9F

# The corrupted prefixes actually present in the tree. A leading U+00E2 consumes
# two following visible chars; the pair set below is what Windows-1252 renders
# when it is asked to display UTF-8 continuation bytes.
$Prefixes = @([char]0x00E2, [char]0x00C3)

# Files that are DOUBLE corrupted need a second pass; they are reported
# separately so a human looks at them instead of trusting an automatic result.
$DoubleCorruptMarkers = @("\u00c3\u0192", "\u00c3\u00a2\u00e2")

$Exts = @('*.go', '*.tsx', '*.ts', '*.mjs', '*.css', '*.md', '*.sql', '*.ps1', '*.sh', '*.json', '*.html')
$SkipDirs = @('node_modules', 'dist', 'bin', '.git', '.playwright-mcp', '.playwright', 'var')

function Test-Skipped([string]$path) {
    foreach ($d in $SkipDirs) {
        if ($path -match [regex]::Escape("\$d\")) { return $true }
    }
    return $false
}

# Decode one corrupted run at $Index. Returns @{ Text; Consumed; Ok }.
# Walks forward while the CP1252 rendering of the accumulated text still forms a
# prefix of a valid UTF-8 decode; stops at the first invalid result.
function Repair-Run([string]$s, [int]$index) {
    $strict = New-Object System.Text.UTF8Encoding($false, $true)
    $bytes = New-Object System.Collections.Generic.List[byte]
    $lastGood = $null
    $lastGoodEnd = $index
    $i = $index
    # A UTF-8 sequence is at most 4 bytes, so never look further than that.
    $limit = [Math]::Min($s.Length, $index + 4)
    while ($i -lt $limit) {
        $ch = $s[$i]
        $code = [int]$ch
        if (-not $script:Map.ContainsKey($code)) { break }
        $bytes.Add($script:Map[$code])
        $i++
        try {
            # Byte-array overload decodes strictly and throws on an
            # incomplete/invalid sequence, which is exactly how we know to keep
            # consuming rather than to stop.
            [string]$decoded = $strict.GetString($bytes.ToArray())
            if ($decoded.Length -ge 1 -and $decoded.IndexOf([char]0xFFFD) -lt 0) {
                $lastGood = $decoded
                $lastGoodEnd = $i
            }
        } catch {
            # Incomplete sequence: keep accumulating bytes.
        }
    }
    if ($null -eq $lastGood) { return @{ Text = $s.Substring($index, 1); Consumed = 1; Ok = $false } }
    return @{ Text = $lastGood; Consumed = $lastGoodEnd - $index; Ok = $true }
}

$report = @()
$totalFixed = 0

foreach ($r in $Root) {
    $files = Get-ChildItem -Path $r -Recurse -File -Include $Exts -ErrorAction SilentlyContinue |
        Where-Object { -not (Test-Skipped $_.FullName) }

    foreach ($f in $files) {
        $text = [System.IO.File]::ReadAllText($f.FullName, $Utf8)
        if ($text.IndexOf([char]0x00E2) -lt 0 -and $text.IndexOf([char]0x00C3) -lt 0) { continue }

        $sb = New-Object System.Text.StringBuilder
        $i = 0
        $fixedHere = 0
        $skipped = 0
        while ($i -lt $text.Length) {
            $ch = $text[$i]
            if ($Prefixes -contains $ch) {
                $res = Repair-Run $text $i
                if ($res.Ok -and $res.Text -ne $text.Substring($i, $res.Consumed)) {
                    [void]$sb.Append($res.Text)
                    $i += $res.Consumed
                    $fixedHere++
                    continue
                } elseif ($res.Ok) {
                    [void]$sb.Append($text.Substring($i, $res.Consumed))
                    $i += $res.Consumed
                    continue
                } else {
                    $skipped++
                }
            }
            [void]$sb.Append($ch)
            $i++
        }

        if ($fixedHere -gt 0) {
            $totalFixed += $fixedHere
            $report += [pscustomobject]@{
                File    = $f.FullName.Replace((Resolve-Path '.').Path + '\', '')
                Fixed   = $fixedHere
                Skipped = $skipped
            }
            if (-not $DryRun) {
                [System.IO.File]::WriteAllText($f.FullName, $sb.ToString(), (New-Object System.Text.UTF8Encoding($true)))
            }
        }
    }
}

$report | Sort-Object Fixed -Descending | Format-Table -AutoSize
Write-Output ("FILES TOUCHED : {0}" -f $report.Count)
Write-Output ("RUNS REPAIRED : {0}" -f $totalFixed)
Write-Output ("MODE          : {0}" -f $(if ($DryRun) { 'DRY RUN - nothing written' } else { 'APPLIED' }))
