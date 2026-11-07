/**
 * VEYLIX — a failed list must offer a working "Try again" (D-052).
 *
 *   node scripts/verify-retry.mjs
 *
 * The app had 211 places that render an error and only three retries: a failed
 * request left reloading the whole page as the only way forward. This test forces a
 * real failure by intercepting the API call, asserts the retry affordance appears,
 * then lets the request through and asserts the list actually recovers.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { assertPortFree, profileDirUnique, waitForExit } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9391)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const PATH = process.argv[2] ?? '/clients'
/** Which API call to fail. Defaults to the clients console; override for other lists. */
const API_PATTERN = process.env.VEYLIX_FAIL_PATTERN ?? '*/api/super/clients*'

// A stale browser on this port answers with a page this run never loaded, which
// reads as "no retry button" for a reason that has nothing to do with the app.
await assertPortFree(PORT)

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDirUnique('retry')}`, '--no-first-run',
  '--disable-gpu', '--window-size=1440,1000', 'about:blank',
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
await send('Fetch.enable', { patterns: [{ urlPattern: API_PATTERN }] })

let failNext = true
let intercepted = 0

ws.addEventListener('message', async (m) => {
  const msg = JSON.parse(m.data)
  if (msg.method === 'Fetch.requestPaused') {
    if (process.env.VEYLIX_DEBUG_FETCH) console.error('PAUSED', msg.params.request.url)
    const { requestId } = msg.params
    intercepted++
    if (failNext) {
      await send('Fetch.fulfillRequest', {
        requestId,
        responseCode: 500,
        responseHeaders: [{ name: 'Content-Type', value: 'application/json' }],
        body: Buffer.from(
          JSON.stringify({ error: { code: 'internal_error', message: 'simulated failure' } }),
        ).toString('base64'),
      }).catch(() => {})
    } else {
      await send('Fetch.continueRequest', { requestId }).catch(() => {})
    }
  }
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

await send('Page.navigate', { url: `${APP}${PATH}` })
// Wait for the failed state itself; a cold profile makes a fixed sleep a coin toss.
const failedReady = await waitFor(`!!document.querySelector('.ui-async--error')`)

// 1. The failure must be shown WITH a retry.
const failed = await evaluate(`(() => ({
  hasErrorPanel: !!document.querySelector('.ui-async--error'),
  text: document.querySelector('.ui-async--error')?.innerText ?? '',
  retry: [...document.querySelectorAll('button')].some((b) => b.textContent.includes('Try again')),
}))()`)

// 2. Let the next request succeed, then press Try again.
failNext = false
const clicked = await evaluate(`(() => {
  const b = [...document.querySelectorAll('button')].find((x) => x.textContent.includes('Try again'))
  if (!b) return 'no-button'
  b.click()
  return 'ok'
})()`)
// Wait for the list to come back instead of assuming two seconds is enough.
await waitFor(`document.querySelectorAll('tbody tr').length > 0`, 10000)

const recovered = await evaluate(`(() => ({
  stillError: !!document.querySelector('.ui-async--error'),
  rows: document.querySelectorAll('tbody tr').length,
}))()`)

// A failure must explain itself: which page, how many interceptions, what it said.
const diagnostics = failed.hasErrorPanel
  ? null
  : await evaluate(`(() => ({
      path: location.pathname,
      text: document.body.innerText.slice(0, 240).replace(/\\s+/g, ' '),
    }))()`)

const out = { path: PATH, intercepted, failedReady, failed, clicked, recovered, diagnostics }
console.log(JSON.stringify(out, null, 2))

ws.close()
await waitForExit(chrome)

const ok =
  failed.hasErrorPanel === true &&
  failed.retry === true &&
  clicked === 'ok' &&
  recovered.stillError === false &&
  recovered.rows > 0
process.exit(ok ? 0 : 1)
