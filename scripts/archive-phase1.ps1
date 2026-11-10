# VEYLIX — Phase 1 backup script
#
# Copies (never moves, never deletes) the source tree to the archive folder.
# Excludes are chosen to keep the archive SMALL and FREE OF SECRETS:
#   node_modules/ dist/  -> reproducible from package.json
#   var/                 -> runtime logs + a stale dev database
#   .veylix-secrets/     -> live credentials; archived separately, never here
#   veylix-ui demo/      -> a separate application, not part of Veylix
$ErrorActionPreference = 'Stop'

$src   = 'H:\my apps\Veylix-2'
$stamp = '2026-11-08'
$dest  = "H:\my apps\_veylix-archive\veylix-2-source-$stamp"

if (Test-Path $dest) { throw "refusing to overwrite an existing archive: $dest" }

# Pass the exclusions as BARE NAMES, not absolute paths: robocopy matches /XD and
# /XF against the names it walks, and an absolute path silently matched nothing
# on the first attempt (node_modules was copied in full, 174 MB).
$excludeDirs = @('node_modules', 'dist', '.git', 'var', '.veylix-secrets',
                 '.vscode', 'veylix-ui demo', '.playwright-mcp', 'bin', 'obj')
$excludeFiles = @('*.exe', '*.db', '*.db-wal', '*.db-shm', '*.log')

New-Item -ItemType Directory -Path $dest -Force | Out-Null

$rcArgs = @($src, $dest, '/E', '/NFL', '/NDL', '/NJH', '/NJS', '/NP', '/R:1', '/W:1')
$rcArgs += @('/XD') + $excludeDirs
$rcArgs += @('/XF') + $excludeFiles

& robocopy @rcArgs | Out-Null
$code = $LASTEXITCODE
if ($code -ge 8) { throw "robocopy failed with exit code $code" }

$files = Get-ChildItem -Path $dest -Recurse -File -Force
$bytes = ($files | Measure-Object -Property Length -Sum).Sum
Write-Host "backup dir : $dest"
Write-Host "files      : $($files.Count)"
Write-Host "size       : $([math]::Round($bytes / 1MB, 2)) MB"
Write-Host "robocopy   : exit $code (0-7 = OK)"

# Prove the exclusions actually held — the whole point of the re-run.
foreach ($bad in @('node_modules', 'dist', '.veylix-secrets', 'var', 'veylix-ui demo')) {
    $hit = Get-ChildItem -Path $dest -Recurse -Directory -Force -ErrorAction SilentlyContinue |
           Where-Object { $_.Name -eq $bad } | Select-Object -First 1
    Write-Host ("  exclude {0,-20} : {1}" -f $bad, $(if ($hit) { "PRESENT  <-- BAD" } else { "clean" }))
}
