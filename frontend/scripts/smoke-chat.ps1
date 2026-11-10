# Live smoke test for the P7 chat endpoints.
# Runs against the dev stack (:5173 proxy -> :8080) with the dev-auth bypass ON,
# which is a super admin: enough to prove routing, the pair-key idempotency and
# the staff-only guards. Participant scoping is covered by the Go tests, which
# can create several real accounts.
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

Write-Host "`n== reads ==" -ForegroundColor Cyan
Call GET '/api/chat/threads'
$dir = Invoke-WebRequest "$b/api/directory" -UseBasicParsing | Select-Object -ExpandProperty Content | ConvertFrom-Json
$peer = $dir | Where-Object { $_.role -ne 'super_admin' } | Select-Object -First 1
if (-not $peer) { Write-Host 'no peer account to chat with'; return }
Write-Host "peer: $($peer.email) (#$($peer.id))"

Write-Host "`n== open (twice: must be the same thread) ==" -ForegroundColor Cyan
$t1 = Call POST '/api/chat/threads' ("{`"peer_user_id`":$($peer.id)}")
$t2 = Call POST '/api/chat/threads' ("{`"peer_user_id`":$($peer.id)}")
$tid = if ($t1) { $t1.id } else { 0 }
if ($t1 -and $t2 -and $t1.id -ne $t2.id) { Write-Host 'FAIL: duplicate thread created' -ForegroundColor Red }

Write-Host "`n== send / read / edit ==" -ForegroundColor Cyan
$m1 = Call POST "/api/chat/threads/$tid/messages" '{"body":"Hello from the smoke test"}'
$mid = if ($m1) { $m1.id } else { 0 }
Call GET  "/api/chat/threads/$tid/messages"
Call PATCH "/api/chat/messages/$mid" ("{`"body`":`"edited`",`"thread_id`":$tid}")
Call POST "/api/chat/messages/$mid/save" ("{`"thread_id`":$tid,`"on`":true}")
Call POST "/api/chat/messages/$mid/important" ("{`"thread_id`":$tid,`"on`":true}")

Write-Host "`n== pin / lock (staff) ==" -ForegroundColor Cyan
Call POST "/api/chat/threads/$tid/pin" ("{`"message_id`":$mid}")
Call POST "/api/chat/threads/$tid/lock" '{"locked":true}'
Write-Host '  (send into a locked thread must be refused with 409)' -ForegroundColor DarkGray
Call POST "/api/chat/threads/$tid/messages" '{"body":"should fail"}'
Call POST "/api/chat/threads/$tid/lock" '{"locked":false}'
Call POST "/api/chat/threads/$tid/read" '{}'

Write-Host "`n== hide (per viewer) ==" -ForegroundColor Cyan
# NOTE: a body-less DELETE must NOT send a JSON content type, or the backend
# (correctly) answers 400 "body is not valid JSON". This is what the browser's
# fetch does, so the UI path is unaffected.
$del = Invoke-WebRequest "$b/api/chat/messages/$mid`?thread_id=$tid" -Method Delete -UseBasicParsing
Write-Host ("{0,-6} {1,-40} -> {2}" -f 'DELETE', "/api/chat/messages/$mid", $del.StatusCode)
Call GET "/api/chat/threads/$tid/messages"
Write-Host '  (the hidden message must be gone from this list)' -ForegroundColor DarkGray

Write-Host "`n== validation ==" -ForegroundColor Cyan
Call POST '/api/chat/threads/999/messages' '{"body":"ghost thread"}'
Call POST '/api/chat/threads' ("{`"peer_user_id`":1}")
Call POST "/api/chat/threads/$tid/messages" '{"body":"   "}'
