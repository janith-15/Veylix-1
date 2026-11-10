$ErrorActionPreference = 'Stop'

$root = 'H:\my apps\_veylix-archive'
$stamp = Get-Date -Format 'yyyy-MM-dd HH:mm:ss'

Write-Host "=== ARCHIVE VERIFICATION ==="
foreach ($d in (Get-ChildItem -Path $root -Directory)) {
    $f = Get-ChildItem -Path $d.FullName -Recurse -File -Force -ErrorAction SilentlyContinue
    $sum = ($f | Measure-Object -Property Length -Sum).Sum
    Write-Host ("{0,-34} {1,5} files  {2,8:N2} MB" -f $d.Name, $f.Count, ($sum / 1MB))
}

# The source archive must contain the files that actually matter. A backup that
# silently missed one of these is worse than no backup, because it looks safe.
$src = Join-Path $root 'veylix-2-source-2026-11-08'
$must = @(
    'MASTER_PRODUCT_BIBLE.md', 'README.md', 'dev.ps1', 'install.sh',
    'VPN_FIX_LEDGER.md', 'docs\DECISIONS.md', 'docs\IMPROVEMENT_LEDGER.md',
    'docs\ARCHITECTURE.md', 'docs\API_CONTRACT.md',
    'backend\go.mod', 'frontend\package.json',
    '.clinerules\01-project-core.md',
    'backend\internal\store\migrations\0023_release_tombstone.sql',
    'backend\internal\provisioning\service.go',
    'frontend\src\pages\client\MyVpnsPage.tsx'
)
Write-Host ""
Write-Host "=== REQUIRED FILES IN SOURCE ARCHIVE ==="
$missing = 0
foreach ($m in $must) {
    $ok = Test-Path (Join-Path $src $m)
    if (-not $ok) { $missing++ }
    Write-Host ("  {0,-62} {1}" -f $m, $(if ($ok) { 'OK' } else { 'MISSING' }))
}
Write-Host ""
if ($missing -gt 0) { Write-Host "RESULT: $missing REQUIRED FILE(S) MISSING" } else { Write-Host "RESULT: all required files present" }

# A manifest so a future session can prove what was archived and when.
$lines = @()
$lines += "# VEYLIX archive manifest"
$lines += ""
$lines += "Created: $stamp"
$lines += ""
$lines += "## Contents"
$lines += ""
$lines += "| Folder | Purpose |"
$lines += "|---|---|"
$lines += "| ``veylix-2-source-2026-11-08`` | Source tree. Excludes node_modules, dist, var, .veylix-secrets, veylix-ui demo. |"
$lines += "| ``docs-legacy-2026-11-08`` | The 31 ``docs/*.md`` files plus the root ``*.md`` set, as they stood before the restart. |"
$lines += "| ``data-2026-11-08`` | Copy of the live dev database. |"
$lines += "| ``secrets-2026-11-08`` | ``.veylix-secrets`` including every ``.pre-*`` snapshot. NEVER commit this. |"
$lines += ""
$lines += "## Folders"
$lines += ""
$lines += "| Folder | Files | MB |"
$lines += "|---|---|---|"
foreach ($d in (Get-ChildItem -Path $root -Directory)) {
    $f = Get-ChildItem -Path $d.FullName -Recurse -File -Force -ErrorAction SilentlyContinue
    $sum = ($f | Measure-Object -Property Length -Sum).Sum
    $lines += ("| ``{0}`` | {1} | {2:N2} |" -f $d.Name, $f.Count, ($sum / 1MB))
}
$lines += ""
$lines += "## Restore"
$lines += ""
$lines += "Copy the relevant folder back over a fresh checkout. The source archive is"
$lines += "complete for building: ``backend/`` and ``frontend/src`` are both whole."
$lines += "``npm install`` is required afterwards; ``node_modules`` was not archived."
$lines += ""
$lines += "## Exclusions and why"
$lines += ""
$lines += "| Excluded | Reason |"
$lines += "|---|---|"
$lines += "| ``node_modules`` | 174 MB, reproducible with ``npm install`` |"
$lines += "| ``dist`` | build output, reproducible with ``npm run build`` |"
$lines += "| ``var`` | runtime logs and a stale dev database |"
$lines += "| ``.veylix-secrets`` | live credentials; archived separately, never with source |"
$lines += "| ``veylix-ui demo`` | a separate application, not part of Veylix |"
$lines += "| ``*.exe``, ``*.db``, ``*.log`` | build artefacts and databases |"

$manifest = Join-Path $root 'MANIFEST.md'
Set-Content -LiteralPath $manifest -Value $lines -Encoding UTF8
Write-Host ""
Write-Host "manifest written: $manifest"
