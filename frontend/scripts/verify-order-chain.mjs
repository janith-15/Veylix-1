#!/usr/bin/env node
/**
 * VEYLIX - the buy page must send the PLAN with the order.
 *
 *   node scripts/verify-order-chain.mjs
 *
 * Why this exists: the fulfilment chain broke silently in the middle. The buy
 * page let a client choose a plan, then submitted a payment carrying only an
 * amount. The server recorded a deposit, confirmation settled it as money, and
 * the VPN was never built - the customer paid and received nothing, while every
 * screen called it a success (D-031.1, D-108).
 *
 * This script watches the ACTUAL network request the page makes. It does not
 * trust the store, the component state or a mock: it asserts that the body of
 * POST /api/payments carries offer_id when a plan was chosen.
 *
 * It is deliberately READ-ONLY. It never submits an order, so no payment row and
 * no VPN are created. To see a request without creating one, it arms a request
 * observer, drives the page to the confirm step, and reads the body the page
 * WOULD send by intercepting the fetch call and aborting it.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9461)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('orderchain')}`, '--no-first-run',
  '--disable-gpu', '--window-size=1440,1600', 'about:blank',
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
  } catch {
    /* Chrome is still starting. */
  }
}
if (!ws) {
  console.error('FAIL  could not attach to Chrome on port ' + PORT)
  chrome.kill()
  process.exit(1)
}

const evaluate = async (functionBody) => {
  const r = await send('Runtime.evaluate', {
    expression: functionBody, awaitPromise: true, returnByValue: true,
  })
  if (r.exceptionDetails) throw new Error(r.exceptionDetails.text)
  return r.result.value
}

await send('Network.enable')

const s = await signIn(process.env.VEYLIX_ROLE ?? 'client')
if (s.status !== 200) {
  console.error(`sign-in failed: ${s.status}`)
  chrome.kill()
  process.exit(1)
}
await send('Network.clearBrowserCookies')
const c = cookieParts(s.cookie)
await send('Network.setCookie', {
  name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true,
})

const failures = []
const check = (ok, msg, extra) => {
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${msg}${extra === undefined ? '' : `  -> ${JSON.stringify(extra)}`}`)
  if (!ok) failures.push(msg)
}

// --- refusal gate: an empty catalogue proves nothing ----------------------
// The page must be on the app origin before a relative fetch is meaningful.
await send('Page.navigate', { url: `${APP}/buy` })
await sleep(2500)

const cat = await evaluate(`
  (async () => {
    const r = await fetch('/api/packages', { credentials: 'include' })
    if (!r.ok) return { ok: false, status: r.status, rows: [] }
    return { ok: true, status: r.status, rows: await r.json() }
  })()
`)
if (!cat || !cat.ok) {
  console.error('')
  console.error(`REFUSED - /api/packages answered ${cat ? cat.status : 'nothing'}, so there`)
  console.error('is no plan to order. Seed a PREMIUM offer and sign in as a client.')
  await send('Page.navigate', { url: 'about:blank' })
  chrome.kill()
  process.exit(3)
}
const buyable = cat.rows.filter((p) => p.purchasable === true)
if (buyable.length === 0) {
  console.error('')
  console.error('REFUSED - the catalogue holds no purchasable plan, so the order path')
  console.error('cannot be exercised. A green run here would prove nothing.')
  await send('Page.navigate', { url: 'about:blank' })
  chrome.kill()
  process.exit(3)
}
console.log(`# catalogue: ${cat.rows.length} package(s), ${buyable.length} purchasable`)

// --- drive the real page -------------------------------------------------
await send('Page.navigate', { url: `${APP}/buy` })
await sleep(3000)

const plans = await evaluate(`document.querySelectorAll('[data-testid="buy-premium"] .client-vpn-card').length`)
check(plans > 0, 'the buy page renders at least one plan card', plans)

/*
 * Capture the body the page WOULD send, without sending it. We patch
 * window.fetch so the request is recorded and then rejected locally, which means
 * no payment row, no audit entry and no VPN can be created by this check.
 */
await evaluate(`
  (() => {
    window.__veyBody = null
    const real = window.fetch
    window.fetch = (url, opts) => {
      if (typeof url === 'string' && url.endsWith('/api/payments') && opts && opts.method === 'POST') {
        try { window.__veyBody = JSON.parse(opts.body) } catch { window.__veyBody = { parse: 'failed' } }
        return Promise.reject(new Error('aborted by verify-order-chain'))
      }
      return real(url, opts)
    }
    return true
  })()
`)

// Choose the first plan, then advance to the payment step.
await evaluate(`document.querySelectorAll('[data-testid="buy-premium"] .client-vpn-card button')[0].click(); true`)
await sleep(800)

// Fill the amount so the submit is not blocked by the emptiness guard.
await evaluate(`
  (() => {
    const input = document.querySelector('[data-testid="buy-premium"] input[type="number"], [data-testid="buy-premium"] input')
    if (!input) return false
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set
    setter.call(input, '1500')
    input.dispatchEvent(new Event('input', { bubbles: true }))
    return true
  })()
`)
await sleep(400)

// Submit: the patched fetch records the body and rejects.
await evaluate(`
  (() => {
    const buttons = [...document.querySelectorAll('[data-testid="buy-premium"] button')]
    const submit = buttons.find((b) => /submit|record|place|pay|confirm order/i.test(b.textContent || ''))
    if (!submit) return false
    submit.click()
    return true
  })()
`)
await sleep(1200)

const body = await evaluate('window.__veyBody')
check(body !== null, 'submitting the buy form built a POST /api/payments request', body)

if (body) {
  check(
    typeof body.offer_id === 'number' && body.offer_id > 0,
    'the order carries offer_id, so confirmation can build the VPN (D-031.1)',
    { offer_id: body.offer_id },
  )
  check(
    typeof body.amount_cents === 'number' && Number.isInteger(body.amount_cents),
    'the amount crosses the wire as integer cents, never a float',
    { amount_cents: body.amount_cents },
  )
  // The plan must be one the catalogue actually offered, not an invented id.
  const ids = buyable.map((p) => p.id)
  check(ids.includes(body.offer_id), 'offer_id names a plan from the real catalogue', {
    offer_id: body.offer_id, catalogue: ids,
  })
}

await send('Page.navigate', { url: 'about:blank' })
chrome.kill()

if (failures.length > 0) {
  console.log('')
  console.log(`FAILED - ${failures.length} check(s)`)
  for (const f of failures) console.log(`   - ${f}`)
  process.exit(1)
}
console.log('')
console.log('OK - the buy page sends the chosen plan with the order, so confirmation builds a VPN')
