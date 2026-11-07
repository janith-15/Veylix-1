# Veylix - start the backend and the frontend together (Windows / PowerShell).
#
# This is the REAL login. The development auth bypass that used to inject an
# identity was removed on 2026-10-28 (docs/DECISIONS.md D-039): every request is
# authenticated by a session cookie, and nothing in the app can skip /login.
#
#   .\dev.ps1                 # backend (:8080) + frontend (:5173), verify the login
#   .\dev.ps1 -Restart        # stop whatever holds :8080/:5173 first, then start
#   .\dev.ps1 -Wait           # keep this window attached so Ctrl+C stops everything
#   .\dev.ps1 -Preflight      # start both and run the full auth verification gate
#   .\dev.ps1 -Seed           # create the demo accounts (one per role) first
#   .\dev.ps1 -FakeAdmin      # + a SECOND admin (needed to test disabling one)
#   .\dev.ps1 -Verify <email> # mark a local account's e-mail verified
#   .\dev.ps1 -PurgeChat      # delete the test chat noise (a backup is written first)
#   .\dev.ps1 -Lan            # also expose Vite on the LAN (phone review)
#   .\dev.ps1 -Log <dir>      # where the logs go (default: .\var\logs)
#   .\dev.ps1 -NoLog          # do not capture logs at all (throwaway run)
#   .\dev.ps1 -NoBrowser      # start without opening a browser tab
#   .\dev.ps1 -Backend        # only the backend (:8080)
#   .\dev.ps1 -Frontend       # only the frontend (:5173)
#   .\dev.ps1 -Status         # what is listening, and does the login really work?
#   .\dev.ps1 -Stop           # stop whatever is listening on :8080 / :5173
#   .\dev.ps1 -Package        # build the single binary (UI embedded) and probe it
#
# Why this script exists:
#   .veylix-secrets\dev-env.ps1 must be loaded IN THE SAME SHELL that launches
#   `go run ./cmd/veylixd` (database keys + panel token). Launching the backend
#   from a fresh terminal fails to start or starts against an empty database, and
#   every screen then answers 401. This script always loads it first and verifies
#   the result instead of telling you the app is ready and letting you find out.

param(
    [switch]$Backend,
    [switch]$Frontend,
    [switch]$Stop,
    [switch]$Status,
    [switch]$Restart,
    [switch]$Wait,
    [switch]$Preflight,
    [switch]$Seed,
    [switch]$FakeAdmin,
    [string]$Verify = '',
    # Issue a password-reset code and print it (no SMTP relay needed).
    #   .\dev.ps1 -ResetLink owner@veylix.test
    [string]$ResetLink = '',
    # Send one test message to prove the SMTP relay actually works.
    #   .\dev.ps1 -TestMail difanmax@gmail.com
    [string]$TestMail = '',
    [switch]$PurgeChat,
    # Provision a staff login and print the generated password once.
    #   .\dev.ps1 -SuperAdmin                 create/reset the super admin
    #   .\dev.ps1 -SuperAdmin -Pattern passphrase
    #   .\dev.ps1 -CreateAdmin -NewEmail a@b.test -NewUsername a -Quota 25
    [switch]$SuperAdmin,
    [switch]$CreateAdmin,
    [string]$NewEmail = '',
    [string]$NewUsername = '',
    [int]$Quota = 0,
    [ValidateSet('readable', 'passphrase', 'base32')]
    [string]$Pattern = 'readable',
    [switch]$NoBrowser,
    [switch]$Lan,
    [switch]$Package,
    # Defaults to a folder rather than '' so a launched process is ALWAYS
    # diagnosable: a hidden window with no log is what turned a one-line Google
    # bug into a multi-hour hunt (IMPROVEMENT_LEDGER F-040).
    [string]$Log = 'var\logs',
    [switch]$NoLog
)

$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$backendDir = Join-Path $root 'backend'
$frontendDir = Join-Path $root 'frontend'
$envFile = Join-Path $root '.veylix-secrets\dev-env.ps1'
$seedFile = Join-Path $root '.veylix-secrets\seed.json'
# Tool paths are resolved ONCE, to absolute paths, before any command block runs:
# a Start-Process child does not always inherit a shell-profile PATH, which is how
# "go: command not found" appears only when the script (not the shell) starts the
# backend, and the provisioning helpers below need $goExe too.
$goExe = (Get-Command go -ErrorAction SilentlyContinue).Source
if (-not $goExe) { $goExe = 'C:\Program Files\Go\bin\go.exe' }
$cmdExe = (Get-Command cmd -ErrorAction SilentlyContinue).Source
if (-not $cmdExe) { $cmdExe = 'C:\Windows\System32\cmd.exe' }
$backendURL = 'http://127.0.0.1:8080'
$frontendURL = 'http://localhost:5173'

function Get-PortOwner([int[]]$ports) {
    Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue |
        Where-Object { $_.LocalPort -in $ports } |
        Select-Object -ExpandProperty OwningProcess -Unique
}

function Get-Listening([int[]]$ports) {
    Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue |
        Where-Object { $_.LocalPort -in $ports } |
        Select-Object LocalAddress, LocalPort, OwningProcess
}

# Stop-Ports also waits for the port to go quiet: `go run` starts the real binary
# as a CHILD, so killing the wrapper can leave the listener behind.
function Stop-Ports([int[]]$ports) {
    $owners = Get-PortOwner $ports
    if (-not $owners) { return $false }
    foreach ($procId in $owners) {
        try { Stop-Process -Id $procId -Force -ErrorAction Stop } catch { }
    }
    foreach ($i in 1..20) {
        Start-Sleep -Milliseconds 250
        if (-not (Get-PortOwner $ports)) { return $true }
    }
    return $true
}

# Test-AnonymousMe returns the status of /api/me with NO cookie. 401 is the only
# healthy answer now: 200 would mean something still grants an identity.
function Test-AnonymousMe {
    try {
        Invoke-WebRequest -Uri "$backendURL/api/me" -UseBasicParsing -TimeoutSec 3 | Out-Null
        return 200
    } catch {
        try { return [int]$_.Exception.Response.StatusCode } catch { return 0 }
    }
}

# Test-RealLogin proves the login works by doing what a person does: POST
# /api/auth/login with the seed credentials, then read /api/me with that cookie.
# Get-SuperAdminCredential returns the credentials that actually work:
# `.veylix-secrets/superadmin.txt` (written by -SuperAdmin) first, then seed.json.
# Without this, a probe would test a password the account no longer has.
function Get-SuperAdminCredential {
    $credFile = Join-Path (Split-Path -Parent $envFile) 'superadmin.txt'
    if (Test-Path $credFile) {
        $text = Get-Content $credFile -Raw
        $u = [regex]::Match($text, 'username\s*:\s*(\S+)')
        $e = [regex]::Match($text, 'email\s*:\s*(\S+)')
        $p = [regex]::Match($text, 'password\s*:\s*(\S+)')
        if ($p.Success) {
            return [pscustomobject]@{
                username = $(if ($u.Success) { $u.Groups[1].Value } else { '' })
                email    = $(if ($e.Success) { $e.Groups[1].Value } else { '' })
                password = $p.Groups[1].Value
            }
        }
    }
    if (Test-Path $seedFile) {
        $data = Get-Content $seedFile -Raw | ConvertFrom-Json
        $owner = $data.accounts | Where-Object { $_.role -eq 'super_admin' } | Select-Object -First 1
        if ($owner) {
            return [pscustomobject]@{ username = ''; email = $owner.email; password = $owner.password }
        }
    }
    return $null
}


# It signs out afterwards so the probe never leaves a session behind.
function Test-RealLogin([string]$email, [string]$password) {
    $session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    try {
        # D-041: staff sign in through the staff door, clients through the client
        # door. The API decides from the account's role; ask both.
        foreach ($portal in @('client', 'staff')) {
            $body = @{ email = $email; password = $password; portal = $portal } | ConvertTo-Json -Compress
            try {
                $login = Invoke-WebRequest -Uri "$backendURL/api/auth/login" -Method POST -UseBasicParsing `
                    -ContentType 'application/json' -Body $body -WebSession $session -TimeoutSec 15
            } catch {
                continue   # wrong door (403) — try the other
            }
            if ($login.StatusCode -ne 200) { continue }
            $me = Invoke-WebRequest -Uri "$backendURL/api/me" -UseBasicParsing -WebSession $session -TimeoutSec 10
            $user = $me.Content | ConvertFrom-Json
            # Leave no session behind: this is a probe, not a person.
            Invoke-WebRequest -Uri "$backendURL/api/auth/logout" -Method POST -UseBasicParsing `
                -WebSession $session -TimeoutSec 10 | Out-Null
            return $user
        }
        return $null
    } catch {
        return $null
    }
}

# -SuperAdmin / -CreateAdmin provision a staff login and hand the password over ONCE.
#
# Why this shape: the password must never appear in source, docs, logs or shell
# history. accounttool prints it as a `CREDENTIAL {...}` line; this function writes
# that line to a 0600 file inside the git-ignored secrets folder and prints it to
# the console one time. The audit log records the change, never the password.
function New-StaffCredential {
    param(
        [Parameter(Mandatory = $true)][string]$Action,   # superadmin | admin
        [string]$AccountEmail = '',
        [string]$AccountUsername = '',
        [int]$AccountQuota = 0,
        [string]$AccountPattern = 'readable',
        [Parameter(Mandatory = $true)][string]$FileSuffix
    )
    if (-not (Test-Path $envFile)) { throw "Missing $envFile" }
    . $envFile
    $go = (Get-Command $goExe -ErrorAction SilentlyContinue).Source
    if (-not $go) { $go = 'C:\Program Files\Go\bin\go.exe' }

    # Build the argument string once, explicitly (see the note where it is used).
    $ActionArgs = "-$Action -pattern $AccountPattern"
    if ($AccountEmail) { $ActionArgs += " -email $AccountEmail" }
    if ($AccountUsername) { $ActionArgs += " -username $AccountUsername" }
    if ($AccountQuota -gt 0) { $ActionArgs += " -quota $AccountQuota" }

    # Run the tool through cmd with an explicit argument string. PowerShell 5.1
    # mangles arrays passed to native commands in more than one way (it can join
    # them into a single argument), and this path must be reliable: it is the only
    # way an operator gets a working password.
    $argLine = "/c `"`"$go`" run ./cmd/accounttool $ActionArgs`""
    Push-Location $backendDir
    try {
        $outFile = [System.IO.Path]::GetTempFileName()
        $proc = Start-Process -FilePath $cmdExe -ArgumentList $argLine -NoNewWindow -Wait -PassThru `
            -RedirectStandardOutput $outFile -RedirectStandardError "$outFile.err"
        $lines = @(Get-Content $outFile -ErrorAction SilentlyContinue)
        $errLines = @(Get-Content "$outFile.err" -ErrorAction SilentlyContinue)
        Remove-Item $outFile, "$outFile.err" -Force -ErrorAction SilentlyContinue
        $exit = $proc.ExitCode
        $lines = @($lines) + @($errLines)
    } finally { Pop-Location }

    $credLine = $lines | Where-Object { $_ -match '^CREDENTIAL ' } | Select-Object -Last 1
    $logLines = $lines | Where-Object { $_ -notmatch '^CREDENTIAL ' }
    foreach ($l in $logLines) { Write-Host $l }
    if ($exit -ne 0) { throw "accounttool failed (exit $exit)" }
    if (-not $credLine) { return }

    $cred = ($credLine -replace '^CREDENTIAL ', '') | ConvertFrom-Json
    $secretsDir = Split-Path -Parent $envFile
    $outFile = Join-Path $secretsDir ("$FileSuffix.txt")
    $stamp = Get-Date -Format 'yyyy-MM-dd HH:mm:ss'
    @(
        "Veylix - $($cred.role) login (generated $stamp)",
        "  URL      : http://localhost:5173/staff/login",
        "  username : $($cred.username)",
        "  email    : $($cred.email)",
        "  password : $($cred.password)",
        "",
        "This file is git-ignored and should be deleted once the password is changed",
        "(Profile -> Security). Never paste this into chat, tickets or source."
    ) | Set-Content -Path $outFile -Encoding UTF8
    # Owner-only where the platform supports it.
    try { $acl = Get-Acl $outFile; $acl.SetAccessRuleProtection($true, $false) } catch { }

    Write-Host ''
    Write-Host '============================================================' -ForegroundColor Cyan
    Write-Host "  ONE-TIME CREDENTIAL (printed once, also saved to a file)" -ForegroundColor Cyan
    Write-Host '============================================================' -ForegroundColor Cyan
    Write-Host ("  role     : {0}" -f $cred.role)
    Write-Host ("  URL      : {0}/staff/login" -f $frontendURL)
    Write-Host ("  username : {0}" -f $cred.username)
    Write-Host ("  email    : {0}" -f $cred.email)
    Write-Host ("  password : {0}" -f $cred.password) -ForegroundColor Yellow
    Write-Host ''
    Write-Host ("  saved to : {0}" -f $outFile) -ForegroundColor DarkGray
    Write-Host '  CHANGE IT at Profile -> Security before doing anything else.' -ForegroundColor Yellow
    Write-Host '============================================================' -ForegroundColor Cyan
}

# -ResetLink prints the 6-digit code / reset link for an account. Loopback only:
# it exists so an operator can hand a code over when no SMTP relay is configured.
function Get-ResetLink {
    param([string]$Identifier)
    Invoke-BackendTool @('-reset-link', $Identifier)
}

# -SuperAdmin / -CreateAdmin: generate a staff login, save it to
# .veylix-secrets\<role>.txt and print it ONCE. No password exists in source.
if ($SuperAdmin) {
    New-StaffCredential -Action 'superadmin' -AccountPattern $Pattern -FileSuffix 'superadmin'
    return
}

if ($CreateAdmin) {
    if (-not $NewEmail) { throw '-CreateAdmin needs -NewEmail' }
    New-StaffCredential -Action 'admin' -AccountEmail $NewEmail -AccountUsername $NewUsername `
        -AccountQuota $Quota -AccountPattern $Pattern -FileSuffix 'admins'
    return
}

function Invoke-BackendTool([string[]]$arguments) {
    if (-not (Test-Path $envFile)) { throw "Missing $envFile" }
    . $envFile
    Push-Location $backendDir
    try {
        & $goExe run ./cmd/accounttool @arguments
        if ($LASTEXITCODE -ne 0) { throw "accounttool failed (exit $LASTEXITCODE)" }
    } finally { Pop-Location }
}


# -Verify / -PurgeChat are one-shot maintenance commands against the same DB the
# server uses (the server keeps working; SQLite is WAL with a 5s busy timeout).
# -Verify / -ResetLink / -TestMail / -PurgeChat are one-shot maintenance commands
# against the same DB the server uses (the server keeps working; SQLite is WAL with
# a 5s busy timeout). They run through accounttool, which refuses a non-loopback bind.
if ($Verify -or $PurgeChat -or $ResetLink -or $TestMail) {
    if ($Verify) { Invoke-BackendTool @('-verify', $Verify) }
    if ($ResetLink) { Invoke-BackendTool @('-reset-link', $ResetLink) }
    if ($TestMail) { Invoke-BackendTool @('-test-mail', $TestMail) }
    if ($PurgeChat) { Invoke-BackendTool @('-purge-chat') }
    return
}

if ($Stop) {
    if (Stop-Ports 8080, 5173) { Write-Host 'stopped :8080 / :5173' }
    else { Write-Host 'Nothing is listening on :8080 or :5173.' }
    return
}

# -Status answers "is it up, and is the login really enforced?" without starting
# or stopping anything. The anonymous /api/me probe is the honest signal (401 is
# healthy), and a real sign-in with the seeded super admin proves the whole path.
if ($Status) {
    $listening = Get-Listening 8080, 5173
    if (-not $listening) {
        Write-Host 'Nothing is listening on :8080 or :5173. Start it with .\dev.ps1'
        return
    }
    $listening | Format-Table -AutoSize | Out-String | Write-Host
    try {
        $r = Invoke-WebRequest -Uri "$backendURL/api/health" -UseBasicParsing -TimeoutSec 3
        Write-Host ("backend  : {0} {1}" -f $r.StatusCode, $r.Content)
        $anon = Test-AnonymousMe
        if ($anon -eq 401) { Write-Host 'auth     : enforced - anonymous requests are refused (401)' -ForegroundColor Green }
        elseif ($anon -eq 200) { Write-Host 'auth     : NOT ENFORCED - anonymous /api/me answered 200 (FAIL)' -ForegroundColor Red }
        else { Write-Host ("auth     : unknown - /api/me answered {0}" -f $anon) -ForegroundColor Yellow }
        if (Test-Path $seedFile) {
            $seedData = Get-Content $seedFile -Raw | ConvertFrom-Json
            $owner = $seedData.accounts | Where-Object { $_.role -eq 'super_admin' } | Select-Object -First 1
            if ($owner) {
                $cred = Get-SuperAdminCredential
            $probe = if ($cred) { Test-RealLogin $cred.email $cred.password } else { $null }
                if ($probe) { Write-Host ("login    : works - {0} ({1})" -f $probe.email, $probe.role) -ForegroundColor Green }
                else { Write-Host 'login    : FAILED for the seeded super admin' -ForegroundColor Red }
            }
        }
    } catch {
        Write-Host ("backend  : not healthy on {0} ({1})" -f $backendURL, $_.Exception.Message)
    }
    try {
        $fe = Invoke-WebRequest -Uri $frontendURL -UseBasicParsing -TimeoutSec 3
        Write-Host ("frontend : {0} at {1}" -f $fe.StatusCode, $frontendURL)
    } catch {
        Write-Host ("frontend : not answering on {0}" -f $frontendURL)
    }
    Write-Host ''
    Write-Host "App      : $frontendURL/login   (sign in with an account from seed.json)"
    return
}

# -Restart stops whatever holds :8080/:5173 first, so the usual "port already in
# use" dead end does not need a second command.
if ($Restart) { Stop-Ports 8080, 5173 | Out-Null }


# -Package builds the UI, embeds it in the Go binary and proves the result serves
# both. The embed directory is wiped first so no stale asset can survive.
if ($Package) {
    $embedDir = Join-Path $backendDir 'internal\httpapi\dist'
    $binDir = Join-Path $backendDir 'bin'
    New-Item -ItemType Directory -Force -Path $embedDir, $binDir | Out-Null

    Write-Host 'building the frontend ...'
    Push-Location $frontendDir
    try {
        # Vite writes its chunk-size hint to stderr, and PowerShell turns any
        # stderr line into a NativeCommandError under $ErrorActionPreference='Stop'.
        # Redirect to a file and judge by the exit code plus a real artifact.
        & $cmdExe /c 'npm run build > build.log 2>&1'
        $buildExit = $LASTEXITCODE
        $buildLog = Join-Path $frontendDir 'build.log'
        if ($buildExit -ne 0) {
            if (Test-Path $buildLog) { Get-Content $buildLog | Select-Object -Last 30 | Write-Host }
            throw "npm run build failed (exit $buildExit)"
        }
        if (-not (Test-Path (Join-Path $frontendDir 'dist\index.html'))) {
            throw "npm run build produced no dist/index.html"
        }
        if (Test-Path $buildLog) { Remove-Item $buildLog -Force }
    } finally { Pop-Location }

    Write-Host 'embedding the build into backend/internal/httpapi/dist ...'
    Get-ChildItem -Path $embedDir -Force |
        Where-Object { $_.Name -ne '.gitkeep' } |
        Remove-Item -Recurse -Force
    Copy-Item -Path (Join-Path $frontendDir 'dist\*') -Destination $embedDir -Recurse -Force
    if (-not (Test-Path (Join-Path $embedDir 'index.html'))) {
        throw "embed copy produced no index.html in $embedDir"
    }
    $embedded = (Get-ChildItem -Path $embedDir -Recurse -File | Where-Object { $_.Name -ne '.gitkeep' } | Measure-Object).Count
    Write-Host ("embedded {0} file(s)" -f $embedded)

    # PROVE the embedded UI carries no development login (D-039/D-041). The embed
    # directory is committed-empty and only written by this block, so a stale copy
    # from an older build is the one way dev code could ship inside the binary.
    # A hit here FAILS the package instead of printing a warning nobody reads.
    $devMarkers = @('veylix:dev-role', 'dev@veylix.local', 'auto-login', 'devSwitchRole',
                    'previewAs', 'dev_impersonating', 'DEV_AUTH_BYPASS', 'VEYLIX_DEV_AUTH')
    $leaks = @()
    foreach ($js in (Get-ChildItem -Path (Join-Path $embedDir 'assets') -Filter '*.js' -File -ErrorAction SilentlyContinue)) {
        foreach ($m in $devMarkers) {
            if (Select-String -Path $js.FullName -Pattern $m -SimpleMatch -Quiet) { $leaks += ("{0}: {1}" -f $js.Name, $m) }
        }
    }
    if ($leaks.Count -gt 0) {
        $leaks | ForEach-Object { Write-Host ("  LEAK  {0}" -f $_) -ForegroundColor Red }
        throw ("the embedded UI still contains development-login code (see LEAK lines): " + $embedDir)
    }
    Write-Host 'embedded UI: no development-login markers (ok)' -ForegroundColor Green

    $exe = Join-Path $binDir 'veylixd.exe'
    Write-Host 'building the single binary ...'
    Push-Location $backendDir
    try {
        & $goExe build -o $exe ./cmd/veylixd
        if ($LASTEXITCODE -ne 0) { throw "go build failed (exit $LASTEXITCODE)" }
    } finally { Pop-Location }
    Write-Host ("binary: {0} ({1:N1} MB)" -f $exe, ((Get-Item $exe).Length / 1MB))

    if (-not (Test-Path $envFile)) { throw "Missing $envFile - the packaged server needs its secrets." }
    . $envFile
    $probePort = 18080
    $env:VEYLIX_ADDR = "127.0.0.1:$probePort"
    $env:VEYLIX_SYNC_INTERVAL = '0'   # a packaging probe must not poll a real panel
    $proc = Start-Process -FilePath $exe -WorkingDirectory $backendDir -PassThru -WindowStyle Hidden
    try {
        $ok = $false
        foreach ($i in 1..40) {
            Start-Sleep -Milliseconds 250
            try {
                if ((Invoke-WebRequest -Uri "http://127.0.0.1:$probePort/api/health" -UseBasicParsing -TimeoutSec 2).StatusCode -eq 200) { $ok = $true; break }
            } catch { }
        }
        if (-not $ok) { throw "the packaged binary did not answer /api/health on :$probePort" }

        $ready = (Invoke-WebRequest -Uri "http://127.0.0.1:$probePort/api/ready" -UseBasicParsing -TimeoutSec 5).Content | ConvertFrom-Json
        $ui = Invoke-WebRequest -Uri "http://127.0.0.1:$probePort/dashboard" -UseBasicParsing -TimeoutSec 5
        $deep = Invoke-WebRequest -Uri "http://127.0.0.1:$probePort/vpn/16" -UseBasicParsing -TimeoutSec 5
        $anon = 0
        try { Invoke-WebRequest -Uri "http://127.0.0.1:$probePort/api/me" -UseBasicParsing -TimeoutSec 5 | Out-Null; $anon = 200 }
        catch { $anon = [int]$_.Exception.Response.StatusCode }
        $missing = $null
        try { Invoke-WebRequest -Uri "http://127.0.0.1:$probePort/assets/nope.js" -UseBasicParsing -TimeoutSec 5 | Out-Null }
        catch { $missing = [int]$_.Exception.Response.StatusCode }
        Write-Host ''
        Write-Host "PACKAGED OK - one binary, one port ($probePort)"
        Write-Host ("  /api/ready  -> {0}  (ui: {1})" -f $ready.status, $ready.ui)
        Write-Host ("  /dashboard  -> {0} (SPA)" -f $ui.StatusCode)
        Write-Host ("  /vpn/16     -> {0} (SPA deep link)" -f $deep.StatusCode)
        Write-Host ("  /api/me anonymous -> {0} (401 = login enforced)" -f $anon)
        Write-Host ("  /assets/nope.js   -> {0} (missing file stays 404)" -f $missing)
    } finally {
        if ($proc -and -not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
    }
    Write-Host ''
    Write-Host "Run the packaged app: `$env:VEYLIX_ADDR='127.0.0.1:8080'; .\backend\bin\veylixd.exe  (then open http://localhost:8080/login)"
    return
}

$startBackend = -not $Frontend -or $Backend
$startFrontend = -not $Backend -or $Frontend

# Capture both processes into a folder. This is the DEFAULT now: a hidden window
# with no log is undiagnosable, and its absence once hid a one-line bug for hours
# (IMPROVEMENT_LEDGER F-040). Pass -NoLog for a throwaway run.
$logDir = ''
if (-not $NoLog -and $Log) {
    $logDir = if ([System.IO.Path]::IsPathRooted($Log)) { $Log } else { Join-Path $root $Log }
    New-Item -ItemType Directory -Force -Path $logDir | Out-Null
}

if ($startBackend) {
    if (-not (Test-Path $envFile)) { throw "Missing $envFile - the database keys and panel token live there." }

    $busy = Get-PortOwner 8080
    if ($busy) { throw "Port 8080 is already in use (pid $busy). Run .\dev.ps1 -Restart (or -Stop) first." }

    # Load VEYLIX_* into THIS process, then launch the backend from it so the
    # child inherits them. `go run` is used so the current source is what runs.
    . $envFile

    if ($env:VEYLIX_INSECURE_COOKIES -ne '1') {
        # Only matters for plain http:// on loopback: a Secure cookie is never
        # sent back over cleartext, so the login would appear to succeed and then
        # every call would answer 401 (IMPROVEMENT_LEDGER F-013).
        Write-Host 'warning: VEYLIX_INSECURE_COOKIES is not 1 - a plain-http local login will not keep its session.' -ForegroundColor Yellow
    }

    # Demo accounts: seeddemo is idempotent (existing e-mails are skipped), so it
    # is safe on every start. Without accounts there is nothing to sign in as.
    if ((Test-Path $seedFile) -and -not $PSBoundParameters.ContainsKey('Frontend')) {
        Push-Location $backendDir
        try {
            $seedOut = & $goExe run ./cmd/seeddemo 2>&1
            $seedOut | Where-Object { $_ -match 'created|skip|done|error' } | ForEach-Object { Write-Host ("  seed: {0}" -f $_) }
        } catch {
            Write-Host ("  seed: skipped ({0})" -f $_.Exception.Message) -ForegroundColor DarkYellow
        } finally { Pop-Location }
    }

    # A second admin exists so "super admin disables an admin" can be tested: the
    # action refuses to target yourself, so one admin is never enough.
    if ($FakeAdmin) {
        Push-Location $backendDir
        try {
            & $goExe run ./cmd/seedacct --role admin --email admin2@veylix.test --name Second `
                --password 'Veylix-Admin2-2026' --quota 25 2>&1 |
                Where-Object { $_ -match 'created|skip|reset|error|refus' } |
                ForEach-Object { Write-Host ("  seedacct: {0}" -f $_) }
        } finally { Pop-Location }
    }

    Write-Host ('starting backend on {0} ...' -f $backendURL)
    if ($logDir) {
        $backendProc = Start-Process -FilePath $goExe -ArgumentList 'run', './cmd/veylixd' `
            -WorkingDirectory $backendDir -PassThru -WindowStyle Hidden `
            -RedirectStandardOutput (Join-Path $logDir 'backend.log') `
            -RedirectStandardError (Join-Path $logDir 'backend.err.log')
    } else {
        $backendProc = Start-Process -FilePath $goExe -ArgumentList 'run', './cmd/veylixd' `
            -WorkingDirectory $backendDir -PassThru -WindowStyle Hidden
    }

    $ready = $false
    foreach ($i in 1..120) {
        Start-Sleep -Milliseconds 500
        # Do NOT poll $backendProc.HasExited when output is redirected: PowerShell
        # keeps the reader handles open, so the flag can stay $false forever.
        try {
            $r = Invoke-WebRequest -Uri "$backendURL/api/health" -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200) { $ready = $true; break }
        } catch { }
    }
    if (-not $ready) {
        if ($logDir) { Write-Host ("  log: {0}" -f (Join-Path $logDir 'backend.err.log')) }
        throw 'The backend did not become healthy on :8080. Run it in a terminal to see the log: cd backend; . ..\.veylix-secrets\dev-env.ps1; go run ./cmd/veylixd'
    }

    # Verify the two facts that actually matter: strangers are refused, and a
    # real account can sign in. "Healthy" alone says nothing about the login.
    $anon = Test-AnonymousMe
    if ($anon -eq 401) {
        Write-Host 'backend ready - login ENFORCED (anonymous /api/me -> 401)' -ForegroundColor Green
    } else {
        Write-Host ("backend ready, but anonymous /api/me answered {0} (expected 401) - STOP and inspect." -f $anon) -ForegroundColor Red
    }
    if (Test-Path $seedFile) {
        $seedData = Get-Content $seedFile -Raw | ConvertFrom-Json
        $owner = $seedData.accounts | Where-Object { $_.role -eq 'super_admin' } | Select-Object -First 1
        if ($owner) {
            $cred = Get-SuperAdminCredential
            $probe = if ($cred) { Test-RealLogin $cred.email $cred.password } else { $null }
            if ($probe) {
                Write-Host ("login works - signed in as {0} ({1})" -f $probe.email, $probe.role) -ForegroundColor Green
            } else {
                Write-Host ("login FAILED for {0} - check the password in .veylix-secrets/seed.json" -f $owner.email) -ForegroundColor Red
            }
        }
    }
}



if ($startFrontend) {
    if (-not (Test-Path (Join-Path $frontendDir 'node_modules'))) {
        Write-Host 'installing frontend dependencies (first run only) ...'
        Push-Location $frontendDir
        try { npm install } finally { Pop-Location }
    }
    $busy = Get-PortOwner 5173
    if ($busy) { throw "Port 5173 is already in use (pid $busy). Run .\dev.ps1 -Restart (or -Stop) first." }

    # Vite reads these two switches (see frontend/vite.config.ts):
    #   VEYLIX_DEV_HOST=1  -> bind 0.0.0.0 so a phone on the same Wi-Fi can open it
    #   VEYLIX_DEV_QUIET=1 -> `dev:quiet` (no browser sheet, log-friendly)
    $env:VEYLIX_DEV_HOST = if ($Lan) { '1' } else { '0' }
    $env:VEYLIX_DEV_QUIET = if ($logDir -or $NoBrowser) { '1' } else { '0' }

    Write-Host ('starting frontend on {0} ...' -f $frontendURL)
    # npm is a .cmd shim on Windows, so it must go through cmd.exe.
    $script = if ($env:VEYLIX_DEV_QUIET -eq '1') { 'npm run dev:quiet' } else { 'npm run dev' }
    if ($logDir) {
        Start-Process -FilePath $cmdExe -ArgumentList '/c', "$script > `"$(Join-Path $logDir 'frontend.log')`" 2>&1" `
            -WorkingDirectory $frontendDir -WindowStyle Hidden | Out-Null
    } else {
        Start-Process -FilePath $cmdExe -ArgumentList '/c', $script `
            -WorkingDirectory $frontendDir -WindowStyle Normal | Out-Null
    }

    # Wait for the first 200 so the printed URL is one that actually answers.
    $feReady = $false
    foreach ($i in 1..60) {
        Start-Sleep -Milliseconds 500
        try {
            $r = Invoke-WebRequest -Uri $frontendURL -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200) { $feReady = $true; break }
        } catch { }
    }
    if (-not $feReady) {
        Write-Host 'frontend did not answer on :5173 - check the Vite output'
        if ($logDir) { Write-Host ("  log: {0}" -f (Join-Path $logDir 'frontend.log')) }
    }
}

Write-Host ''
Write-Host "App      : $frontendURL/login"
Write-Host "Dashboard: $frontendURL/dashboard   (after signing in)"
Write-Host "Backend  : $backendURL/api/health"
if ($Lan) {
    $ips = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -notlike '127.*' -and $_.IPAddress -notlike '169.254.*' } |
        Select-Object -ExpandProperty IPAddress
    foreach ($ip in $ips) { Write-Host "LAN      : http://${ip}:5173/login" }
}
if ($logDir) { Write-Host ("Logs     : {0}" -f $logDir) }

# What to do with those URLs. Being explicit here is the difference between "the
# backend is running" and "I know how to sign in right now".
Write-Host ''
Write-Host 'SIGN IN at /login with an account from .veylix-secrets/seed.json:' -ForegroundColor Cyan
if (Test-Path $seedFile) {
    $seedData = Get-Content $seedFile -Raw | ConvertFrom-Json
    foreach ($a in $seedData.accounts) {
        Write-Host ("    {0,-12} {1}" -f $a.role, $a.email)
    }
}
if ($FakeAdmin) { Write-Host '    admin        admin2@veylix.test' }
Write-Host '  "Continue with Google" needs VEYLIX_GOOGLE_CLIENT_ID/SECRET in dev-env.ps1'
Write-Host '  Full verification : .\dev.ps1 -Preflight'

if ($Preflight) {
    Write-Host ''
    Write-Host 'running the auth preflight gate ...' -ForegroundColor Cyan
    Push-Location $frontendDir
    try {
        & node (Join-Path $frontendDir 'scripts\preflight-dev.mjs')
        $pfExit = $LASTEXITCODE
    } finally { Pop-Location }
    if ($pfExit -ne 0) {
        Write-Host ''
        Write-Host 'PREFLIGHT FAILED - see the FAIL lines above.' -ForegroundColor Red
        throw "preflight failed (exit $pfExit)"
    }
    Write-Host ''
    Write-Host 'PREFLIGHT PASSED - the real login works for all three roles.' -ForegroundColor Green
}

Write-Host ''
Write-Host 'Status   : .\dev.ps1 -Status      Restart: .\dev.ps1 -Restart      Stop both: .\dev.ps1 -Stop'

# -Wait keeps this window attached to both processes so a single Ctrl+C (or
# closing the window) stops the backend and the frontend together.
if ($Wait) {
    Write-Host ''
    Write-Host 'Ctrl+C stops everything.' -ForegroundColor DarkGray
    try {
        while ($true) { Start-Sleep -Seconds 2 }
    } finally {
        Stop-Ports 8080, 5173 | Out-Null
        Write-Host 'stopped.'
    }
}

if (-not $cmdExe) { $cmdExe = 'C:\Windows\System32\cmd.exe' }
