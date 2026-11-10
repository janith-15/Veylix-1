# Live smoke test for the P8 notification PRODUCER (Sprint 2).
# Runs against the dev stack (:5173 proxy -> :8080) with the dev-auth bypass ON.
#
# Proves the piece that was missing before: an audited action now WRITES an inbox
# entry, so the bell and the inbox page are not permanently empty.
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

Write-Host "`n== before ==" -ForegroundColor Cyan
$before = Call GET '/api/notifications/unread'

Write-Host "`n== trigger an audited action ==" -ForegroundColor Cyan
$m = Call POST '/api/payment-methods' '{"kind":"EZCASH","label":"Inbox smoke","details":"077-0000"}'

Write-Host "`n== after ==" -ForegroundColor Cyan
Call GET '/api/notifications/unread'
$inbox = Call GET '/api/notifications?limit=5'
$titles = @($inbox | ForEach-Object { $_.title })
Write-Host ("titles: " + ($titles -join ' | '))

if ($titles -contains 'Payment method added') {
  Write-Host '`nPASS: the audited action produced an inbox entry.' -ForegroundColor Green
} else {
  Write-Host '`nFAIL: no inbox entry for an audited action.' -ForegroundColor Red
}

Write-Host "`n== mark one read ==" -ForegroundColor Cyan
$first = @($inbox)[0]
if ($first) {
  Call POST "/api/notifications/$($first.id)/read" '{}'
  Call GET  '/api/notifications/unread'
}
