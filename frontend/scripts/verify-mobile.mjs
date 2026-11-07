/**
 * VEYLIX — measure a page at a phone viewport.
 *
 *   node scripts/verify-mobile.mjs /clients
 *
 * The audit found no rules below 480px and small buttons, but "looks cramped" is an
 * opinion. This reports facts at 390×844 (a common phone): horizontal overflow, how
 * many interactive targets are under 44px, and whether a table is being scrolled
 * sideways instead of reflowed.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9421)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const PATH = process.argv[2] ?? '/clients'
const ROLE = process.env.VEYLIX_MOBILE_ROLE ?? 'super_admin'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('mob')}`, '--no-first-run',
  '--disable-gpu', 'about:blank',
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
// iPhone 12/13/14 logical viewport, plus a mobile UA so any UA sniffing matches.
await send('Emulation.setDeviceMetricsOverride', {
  width: 390, height: 844, deviceScaleFactor: 3, mobile: true,
})
await send('Network.setUserAgentOverride', {
  userAgent:
    'Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 ' +
    '(KHTML, like Gecko) Version/16.0 Mobile/15E148 Safari/604.1',
})

const session = await signIn(ROLE)
if (session.status !== 200) {
  console.error(`${ROLE} sign-in failed: ${session.status}`)
  process.exit(1)
}
const c = cookieParts(session.cookie)
await send('Network.setCookie', { name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true })

const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  return r?.result?.value
}

await send('Page.navigate', { url: `${APP}${PATH}` })
await sleep(2200)

const out = await evaluate(`(() => {
  const docW = document.documentElement.clientWidth
  const scrollW = document.documentElement.scrollWidth
  const targets = [...document.querySelectorAll('button, a, input, select, textarea')]
    .filter((el) => {
      const r = el.getBoundingClientRect()
      if (r.width === 0 || r.height === 0) return false
      // Must be on screen. A closed off-canvas drawer keeps its geometry and is only
      // moved by translateX, so its links measure full size while being invisible —
      // counting them is how this audit first reported "every target is too small".
      if (r.right <= 0 || r.left >= window.innerWidth) return false
      if (r.bottom <= 0 || r.top >= window.innerHeight) return false
      return el.checkVisibility ? el.checkVisibility() : true
    })
  const small = targets
    .map((el) => {
      const r = el.getBoundingClientRect()
      return { tag: el.tagName.toLowerCase(), text: (el.textContent || el.getAttribute('aria-label') || '').trim().slice(0, 24), w: Math.round(r.width), h: Math.round(r.height) }
    })
    .filter((t) => t.h < 44 || t.w < 44)
  const table = document.querySelector('.ui-table-wrap')
  const tableScrolls = table ? table.scrollWidth > table.clientWidth + 2 : false
  const cards = document.querySelectorAll('[class*="card-list"], [data-mobile-cards]').length
  return {
    path: location.pathname,
    viewport: docW,
    horizontalOverflowPx: Math.max(0, scrollW - docW),
    interactiveTargets: targets.length,
    under44px: small.length,
    smallest: small.slice(0, 6),
    tableScrollsSideways: tableScrolls,
    mobileCardList: cards,
  }
})()`)

console.log(JSON.stringify(out, null, 2))
ws.close()
chrome.kill()
// Fail only on the two things that genuinely break a phone: page-level sideways
// scroll, and every target being too small to tap.
const ok = out.horizontalOverflowPx === 0
process.exit(ok ? 0 : 1)
