<#
.SYNOPSIS
    Starts / stops / inspects the Veylix UI dev server (Vite) on http://localhost:3000.

.DESCRIPTION
    This repository is the frontend-only Veylix app (React 19 + Vite 8 + Tailwind 4).
    There is no backend process and no database: the store persists to localStorage,
    so the only thing that has to be running is Vite.

    The script exists because "just run npm run dev" gets three things wrong:
      1. dependencies may not be installed yet (node_modules missing),
      2. the port may already be held by a previous run,
      3. you cannot tell from the outside whether the page really answers.

.PARAMETER Stop
    Kill whatever is listening on the dev port.

.PARAMETER Status
    Show what is running and whether http://localhost:3000 answers.

.PARAMETER Restart
    Stop, then start again.

.PARAMETER Port
    Port to serve on. Default 3000 (matches package.json "dev").

.PARAMETER NoBrowser
    Do not open a browser tab after the server answers.

.PARAMETER Log
    Keep the Vite output in .\logs\dev.log instead of a visible window.

.EXAMPLE
    .\dev.ps1
    Starts the dev server and opens http://localhost:3000.

.EXAMPLE
    .\dev.ps1 -Restart -Log
    Restarts it with output redirected to .\logs\dev.log.
#>
[CmdletBinding()]
param(
    [switch]$Stop,
    [switch]$Status,
    [switch]$Restart,
    [int]$Port = 3000,
    [switch]$NoBrowser,
    [switch]$Log
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$root = $PSScriptRoot
$logDir = Join-Path $root '.logs'
$logFile = Join-Path $logDir 'dev.log'
$url = "http://localhost:$Port/"

function Test-HttpOk {
    param([string]$Url)
    # curl.exe ships with Windows 10 1803+ and avoids Invoke-WebRequest's quirks.
    $code = & curl.exe -s -o NUL -w '%{http_code}' --max-time 3 $Url 2>$null
    if ($LASTEXITCODE -eq 0 -and "$code" -match '^2\d\d$') {
        return [pscustomobject]@{ Ok = $true; Status = [int]$code }
    }
    return [pscustomobject]@{ Ok = $false; Status = 0 }
}

function Get-DevPortProcess {
    param([int]$Port)
    Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue |
        Select-Object -ExpandProperty OwningProcess -Unique |
        Where-Object { $_ -gt 0 }
}

function Stop-DevServer {
    $procs = @(Get-DevPortProcess -Port $Port)
    if ($procs.Count -eq 0) {
        Write-Host "Nothing is listening on port $Port." -ForegroundColor DarkGray
        return
    }
    foreach ($processId in $procs) {
        $name = (Get-Process -Id $processId -ErrorAction SilentlyContinue).ProcessName
        Write-Host "Stopping $name (pid $processId) on port $Port..." -ForegroundColor Yellow
        # /T kills the whole tree: cmd -> npm -> node -> esbuild
        & taskkill.exe /PID $processId /T /F 2>&1 | Out-Null
    }
    Start-Sleep -Milliseconds 800
}

function Show-Status {
    $procs = @(Get-DevPortProcess -Port $Port)
    if ($procs.Count -eq 0) {
        Write-Host "Port ${Port}: free (server not running)" -ForegroundColor DarkGray
    } else {
        $names = ($procs | ForEach-Object {
            "$((Get-Process -Id $_ -ErrorAction SilentlyContinue).ProcessName)($_)"
        }) -join ', '
        Write-Host "Port ${Port}: listening - $names" -ForegroundColor Green
    }
    $probe = Test-HttpOk -Url $url
    if ($probe.Ok) {
        Write-Host "GET $url -> $($probe.Status) OK" -ForegroundColor Green
    } else {
        Write-Host "GET $url -> no answer" -ForegroundColor Red
    }
}

if ($Stop) {
    Stop-DevServer
    Show-Status
    return
}

if ($Status) {
    Show-Status
    return
}

if ($Restart) {
    Stop-DevServer
}

# --- Preflight ---------------------------------------------------------------
if (-not (Get-Command node.exe -ErrorAction SilentlyContinue)) {
    throw 'node.exe not found on PATH. Install Node.js 20.19+ (or 22.12+) first.'
}
if (-not (Get-Command npm.cmd -ErrorAction SilentlyContinue)) {
    throw 'npm not found on PATH. Install Node.js 20.19+ (or 22.12+) first.'
}

Push-Location $root
try {
    if (-not (Test-Path (Join-Path $root 'node_modules\vite\package.json'))) {
        Write-Host 'node_modules missing - running npm install...' -ForegroundColor Yellow
        & npm.cmd install --no-audit --no-fund
        if ($LASTEXITCODE -ne 0) { throw "npm install failed (exit $LASTEXITCODE)" }
    }

    if (@(Get-DevPortProcess -Port $Port).Count -gt 0) {
        Show-Status
        throw "Port $Port is already in use. Re-run with .\dev.ps1 -Restart to replace it."
    }

    # --- Start ---------------------------------------------------------------
    $viteArgs = "--port $Port --strictPort --host=0.0.0.0"
    # package.json "dev" already pins --port=3000 --host=0.0.0.0; passing the flags
    # again is harmless and keeps -Port working for any other port.
    if ($Log) {
        if (-not (Test-Path $logDir)) { New-Item -ItemType Directory -Path $logDir | Out-Null }
        Write-Host "Starting Vite on port $Port (log: .\logs\dev.log)..." -ForegroundColor Cyan
        $command = "npm run dev -- $viteArgs > `"$logFile`" 2>&1"
        Start-Process -FilePath 'cmd.exe' -ArgumentList '/c', $command -WorkingDirectory $root -WindowStyle Hidden | Out-Null
    } else {
        Write-Host "Starting Vite in a new window on port $Port..." -ForegroundColor Cyan
        Start-Process -FilePath 'cmd.exe' -ArgumentList '/c', "npm run dev -- $viteArgs" -WorkingDirectory $root | Out-Null
    }

    # --- Wait for a real answer, not for a process flag ----------------------
    $deadline = (Get-Date).AddSeconds(45)
    $probe = $null
    while ((Get-Date) -lt $deadline) {
        Start-Sleep -Milliseconds 500
        $probe = Test-HttpOk -Url $url
        if ($probe.Ok) { break }
        if ($Log -and (Test-Path $logFile)) {
            $tail = Get-Content $logFile -Raw -ErrorAction SilentlyContinue
            if ($tail -match 'EADDRINUSE|failed to load config') { break }
        }
    }

    if (-not $probe -or -not $probe.Ok) {
        Write-Host "The dev server did not answer on $url" -ForegroundColor Red
        if ($Log -and (Test-Path $logFile)) {
            Write-Host '--- last log lines ---' -ForegroundColor DarkGray
            Get-Content $logFile -Tail 25
        }
        throw 'Startup failed. Check the output above.'
    }

    Write-Host ''
    Write-Host 'DEV SERVER UP' -ForegroundColor Green
    Write-Host "  App      -> $url" -ForegroundColor Green
    Write-Host "  Health   -> GET $url -> $($probe.Status)" -ForegroundColor Green
    Write-Host ''
    Write-Host '  Frontend-only app: state lives in localStorage (key veylix_platform_state_v2).' -ForegroundColor DarkGray
    Write-Host '  Blank screen or odd data? Run in the browser console, then reload:' -ForegroundColor DarkGray
    Write-Host "    localStorage.removeItem('veylix_platform_state_v2')" -ForegroundColor DarkGray
    Write-Host ''
    Write-Host '  Stop     -> .\dev.ps1 -Stop' -ForegroundColor DarkGray
    Write-Host '  Restart  -> .\dev.ps1 -Restart' -ForegroundColor DarkGray
    Write-Host '  Status   -> .\dev.ps1 -Status' -ForegroundColor DarkGray

    if (-not $NoBrowser) {
        Start-Process $url
    }
} finally {
    Pop-Location
}
