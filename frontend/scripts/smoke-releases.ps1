# Live smoke test for Milestone 5-B step 9 (release + 3-day undo, D-027).
# Runs against the dev stack (:5173 proxy -> :8080).
#
# Proves the routes exist and their refusals are the documented ones. The full
# lifecycle (release -> restore, and release -> expire -> LOCAL-only removal) is
# covered deterministically by TestReleaseGraceAndLocalOnlyRemoval, which can
# move the clock three days.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$b = 'http://localhost:5173'

function Call($m, $p, $body) {
  $params = @{ Uri = "$b$p"; Method = $m; UseBasicParsing = $true; TimeoutSec = 15 }
  if ($body) { $params.Body = $body; $params.ContentType = 'application/json' }
  try {
    $r = Invoke-WebRequest @params
    Write-Host ("{0,-6} {1,-38} -> {2} {3}" -f $m, $p, $r.StatusCode, $r.Content)
    if ($r.Content) { return ($r.Content | ConvertFrom-Json) }
    return $null
  } catch {
    $s = $_.Exception.Response.StatusCode.value__
    $t = ''
    try { $t = (New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())).ReadToEnd() } catch {}
    Write-Host ("{0,-6} {1,-38} -> {2} {3}" -f $m, $p, $s, $t)
    return $null
  }
}

Write-Host "`n== the timeline ==" -ForegroundColor Cyan
Call GET '/api/vpn-releases'

Write-Host "`n== release a VPN that does not exist ==" -ForegroundColor Cyan
Call POST '/api/vpns/99999/release' '{"reason":"quota_exhausted"}'

Write-Host "`n== restore a release that does not exist ==" -ForegroundColor Cyan
Call POST '/api/vpn-releases/99999/restore'

Write-Host "`n== operator cannot release somebody else's VPN ==" -ForegroundColor Cyan
Write-Host '  (this dev session IS the super admin, so 403 is the expected answer)' -ForegroundColor DarkGray
$vpns = @(Call GET '/api/vpns')
$local = $vpns | Where-Object { $_.id -gt 0 } | Select-Object -First 1
if ($local) {
  Call POST "/api/vpns/$($local.id)/release" '{"reason":"quota_exhausted"}'
} else {
  Write-Host '  no local VPN on this dev database to attempt.'
}
