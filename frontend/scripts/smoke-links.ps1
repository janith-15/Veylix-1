# Live smoke test for Milestone 5-B step 11 (links + ping, D-026).
# Runs against the dev stack (:5173 proxy -> :8080) with the dev-auth bypass ON.
#
# Proves the two endpoints that let a client actually CONNECT now exist and that
# their refusals are the documented ones.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$b = 'http://localhost:5173'

function Call($m, $p, $body) {
  $params = @{ Uri = "$b$p"; Method = $m; UseBasicParsing = $true; TimeoutSec = 15 }
  if ($body) { $params.Body = $body; $params.ContentType = 'application/json' }
  try {
    $r = Invoke-WebRequest @params
    Write-Host ("{0,-6} {1,-40} -> {2} {3}" -f $m, $p, $r.StatusCode, $r.Content)
    if ($r.Content) { return ($r.Content | ConvertFrom-Json) }
    return $null
  } catch {
    $s = $_.Exception.Response.StatusCode.value__
    $t = ''
    try { $t = (New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())).ReadToEnd() } catch {}
    Write-Host ("{0,-6} {1,-40} -> {2} {3}" -f $m, $p, $s, $t)
    return $null
  }
}

Write-Host "`n== refusals on a VPN that does not exist ==" -ForegroundColor Cyan
Call GET  '/api/vpns/99999/links'
Call POST '/api/vpns/99999/ping'

Write-Host "`n== the real routes on this installation's VPNs ==" -ForegroundColor Cyan
$vpns = Call GET '/api/vpns'
$list = @($vpns)
Write-Host ("VPNs visible to this account: " + $list.Count)
if ($list.Count -gt 0) {
  $id = $list[0].id
  Write-Host "`n-- vpn #$id --" -ForegroundColor DarkGray
  Call GET  "/api/vpns/$id/links"
  Call POST "/api/vpns/$id/ping"
  Write-Host '  (source=panel|unavailable and scope=server_reachability are the contract)' -ForegroundColor DarkGray
} else {
  Write-Host '  no VPNs on this dev database, so only the refusal paths are exercised here.'
  Write-Host '  The ownership + outage behaviour is covered by TestVPNLinksAndPingScope.'
}
