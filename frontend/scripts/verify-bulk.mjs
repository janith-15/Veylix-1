/**
 * VEYLIX — bulk selection must actually work, and must confirm (D-054).
 *
 *   node scripts/verify-bulk.mjs
 *
 * Before this there was no selection API at all: blocking fifty clients meant fifty
 * dialogs. This drives the real page: select two rows, open the bulk action, assert a
 * confirmation names the count, confirm, and assert the UI issued exactly one PATCH
 * per selected client, carrying the requested status.
 *
 * It does NOT let those PATCHes reach the API. The earlier version did, and because
 * it ticks the first two REAL rows it blocked two real customers and ended every one
 * of their sessions (VPN_FIX_LEDGER, 2026-11-07). A verification script must never
 * write to live user data, so the bulk writes are intercepted with CDP Fetch and
 * answered locally. The server-side change itself is covered by the Go tests around
 * `clientPatchHandler` in backend/internal/httpapi and internal/store.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { assertPortFree, profileDirUnique, waitForExit } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9411)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
// Never attach to a browser this run did not start: a foreign page verifies
// nothing and reports a false failure (assertPortFree exits 2, not 1).
await assertPortFree(PORT)

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDirUnique('bulk')}`, '--no-first-run',
  '--disable-gpu', '--window-size=1440,1100', 'about:blank',
], { stdio: 'ignore' })

let ws
let seq = 0
const pending = new Map()
const send = (m, p = {}) =>
  new Promise((res, rej) => {
    const id = ++seq
    pending.set(id, { res, rej })
    ws.send(JSON.stringify({ id, method: m, params: p }))
  })

for (let i = 0; i < 60; i++) {
  await sleep(250)
  try {
    const list = await (await fetch(`http://127.0.0.1:${PORT}/json/list`)).json()
    const page = list.find((t) => t.type === 'page')
    if (!page) continue
    ws = new WebSocket(page.webSocketDebuggerUrl)
    await new Promise((r) => (ws.onopen = r))
    ws.onmessage = (m) => {
      const msg = JSON.parse(m.data)
      const p = pending.get(msg.id)
      if (!p) return
      pending.delete(msg.id)
      if (msg.error) p.rej(new Error(msg.error.message))
      else p.res(msg.result)
    }
    break
  } catch { /* not up yet */ }
}
if (!ws) {
  console.error('could not attach to chrome')
  process.exit(1)
}

await send('Runtime.enable')
await send('Page.enable')
await send('Network.enable')

// The bulk action writes one client at a time. Hold every PATCH here and answer
// it locally, so the assertion still proves the page issued the right requests
// while no real account is touched.
await send('Fetch.enable', {
  patterns: [{ urlPattern: '*/api/super/clients/*', requestStage: 'Request' }],
})
const patched = []
let fetchHandlerError = null
ws.addEventListener('message', (m) => {
  const msg = JSON.parse(m.data)
  if (msg.method !== 'Fetch.requestPaused') return
  const { requestId, request } = msg.params
  if (request.method !== 'PATCH') {
    send('Fetch.continueRequest', { requestId }).catch(() => {})
    return
  }
  const id = Number((request.url.match(/\/clients\/(\d+)/) ?? [])[1] ?? 0)
  let body = {}
  try {
    body = JSON.parse(request.postData ?? '{}')
  } catch (err) {
    fetchHandlerError = String(err)
  }
  patched.push({ id, body })
  send('Fetch.fulfillRequest', {
    requestId,
    responseCode: 200,
    responseHeaders: [{ name: 'Content-Type', value: 'application/json' }],
    // clientAdminStore.setStatus reads detail.profile.status back.
    body: Buffer.from(
      JSON.stringify({ profile: { id, status: body.status ?? 'SUSPENDED' } }),
    ).toString('base64'),
  }).catch(() => {})
})

const session = await signIn('super_admin')
if (session.status !== 200) {
  console.error(`super admin sign-in failed: ${session.status}`)
  process.exit(1)
}
const c = cookieParts(session.cookie)
await send('Network.setCookie', { name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true })

const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  return r?.result?.value
}

/** Poll a predicate instead of guessing how long a page needs to boot. */
const waitFor = async (expression, timeoutMs = 25000) => {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (await evaluate(expression)) return true
    await sleep(200)
  }
  return false
}

await send('Page.navigate', { url: `${APP}/clients` })
// A fresh profile has no HTTP cache, so the dev server's module graph loads cold
// and a fixed sleep is a coin toss. Wait for the table itself.
const rowsReady = await waitFor(
  `document.querySelectorAll('tbody input[type="checkbox"]').length >= 2`,
)

// 1. Tick the first two row checkboxes.
const ticked = await evaluate(`(() => {
  const boxes = [...document.querySelectorAll('tbody input[type="checkbox"]')]
    .filter((b) => !b.disabled)
  if (boxes.length < 2) return 'not-enough-rows'
  boxes[0].click()
  boxes[1].click()
  return boxes.length
})()`)
await sleep(400)

// 2. The bar must appear and state the count.
const bar = await evaluate(`(() => {
  const el = document.querySelector('.ui-selection')
  return {
    present: !!el,
    text: el ? el.innerText.replace(/\\n+/g, ' | ') : '',
  }
})()`)

// 3. Open the bulk action; the dialog must name the count.
const opened = await evaluate(`(() => {
  const b = [...document.querySelectorAll('.ui-selection button')]
    .find((x) => x.textContent.trim() === 'Block')
  if (!b) return 'no-block-button'
  b.click()
  return 'ok'
})()`)
await sleep(500)
const dialog = await evaluate(`(() => {
  const d = document.querySelector('[role="dialog"]')
  return { present: !!d, text: d ? d.innerText.slice(0, 160).replace(/\\n+/g, ' ') : '' }
})()`)

// 4. Confirm, then verify against the API that they really are suspended.
const confirmed = await evaluate(`(() => {
  const b = [...document.querySelectorAll('[role="dialog"] button')]
    .find((x) => x.textContent.includes('Block them'))
  if (!b) return 'no-confirm'
  b.click()
  return 'ok'
})()`)
await sleep(3000)

const afterText = await evaluate(
  `document.querySelector('.ui-async [role="status"], .ui-async + p')?.textContent ?? document.querySelector('[role="status"]')?.textContent ?? ''`,
)

// The truth is the request the page actually issued: one PATCH per selected
// client, each carrying the requested status. Nothing reached the database.
const distinctClients = new Set(patched.map((p) => p.id)).size
const allSuspended = patched.length > 0 && patched.every((p) => p.body?.status === 'SUSPENDED')

// A failure must explain itself: which page, how many rows, what it said.
const diagnostics =
  typeof ticked === 'number'
    ? null
    : await evaluate(`(() => ({
        path: location.pathname,
        rows: document.querySelectorAll('tbody tr').length,
        checkboxes: document.querySelectorAll('tbody input[type="checkbox"]').length,
        text: document.body.innerText.slice(0, 240).replace(/\\s+/g, ' '),
      }))()`)

const out = {
  ticked, bar, opened, dialog, confirmed, afterText,
  rowsReady,
  patchedCount: patched.length,
  distinctClients,
  patched,
  liveWrite: 'intercepted — no customer was modified',
  fetchHandlerError,
  diagnostics,
}
console.log(JSON.stringify(out, null, 2))

ws.close()
await waitForExit(chrome)

const ok =
  typeof ticked === 'number' &&
  bar.present === true &&
  /2\s*selected/.test(bar.text) &&
  opened === 'ok' &&
  dialog.present === true &&
  /2/.test(dialog.text) &&
  confirmed === 'ok' &&
  /Blocked 2 clients/.test(afterText ?? '') &&
  patched.length === 2 &&
  distinctClients === 2 &&
  allSuspended
process.exit(ok ? 0 : 1)
