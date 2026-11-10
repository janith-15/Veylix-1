# Live smoke test for the P6/P8 endpoints (Sprint 1).
# Run against a throwaway backend started with VEYLIX_DEV_AUTH=0 and a temp
# VEYLIX_DATA_DIR. Proves the whole manual-payment lifecycle plus the refusals.
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$b = 'http://127.0.0.1:8099'
$session = $null

function Call($m, $p, $body, $auth = $false) {
  $params = @{
    Uri = "$b$p"; Method = $m; UseBasicParsing = $true; TimeoutSec = 15
    WebSession = $script:session
  }
  if ($body) { $params.Body = $body; $params.ContentType = 'application/json' }
  try {
    $r = Invoke-WebRequest @params
    Write-Host ("{0,-6} {1,-42} -> {2} {3}" -f $m, $p, $r.StatusCode, $r.Content)
    return ($r.Content | ConvertFrom-Json)
  } catch {
    $s = $_.Exception.Response.StatusCode.value__
    $t = ''
    try { $t = (New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())).ReadToEnd() } catch {}
    Write-Host ("{0,-6} {1,-42} -> {2} {3}" -f $m, $p, $s, $t)
    return $null
  }
}

Write-Host "`n== bootstrap + sign in ==" -ForegroundColor Cyan
$session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
Call POST '/api/setup/bootstrap' '{"email":"owner@example.com","password":"a very long password","display_name":"Owner"}'
Call GET  '/api/me'
Call GET  '/api/setup/status'

Write-Host "`n== reads ==" -ForegroundColor Cyan
Call GET  '/api/notifications'
Call GET  '/api/notifications/unread'
Call GET  '/api/directory'
Call GET  '/api/payment-methods'
Call GET  '/api/payments'

Write-Host "`n== writes ==" -ForegroundColor Cyan
$method = Call POST '/api/payment-methods' '{"kind":"BANK","label":"Smoke Bank","details":"AC 0012","instructions":"Use your email as reference"}'
$mid = if ($method) { $method.id } else { 0 }
$pay = Call POST '/api/payments' ("{`"amount_cents`":1500,`"method_id`":$mid}")
$payId = if ($pay) { $pay.id } else { 0 }
Call POST "/api/payments/$payId/reference" '{"reference":"TXN-SMOKE-1"}'
Call POST "/api/payments/$payId/confirm"
Write-Host '  (the second confirm must be refused with 409)' -ForegroundColor DarkGray
Call POST "/api/payments/$payId/confirm"
Call POST "/api/payments/$payId/refund"
Call GET  '/api/payments'
Call GET  '/api/audit?limit=10'

Write-Host "`n== validation ==" -ForegroundColor Cyan
Call POST '/api/payment-methods' '{"kind":"CRYPTO","label":"Bad kind"}'
Call POST '/api/payments' '{"amount_cents":0}'
Call POST '/api/notifications/999/read'

