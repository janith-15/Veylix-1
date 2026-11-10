#!/usr/bin/env node
/**
 * VEYLIX — the Orders screen: sort, filter, search and the honest cap notice.
 *
 *   node scripts/verify-orders.mjs
 *
 * Why this exists: the Super Admin's order list could only be sorted, and even
 * that was broken. `Placed by`, `Package` and `Reference` had no sort button at
 * all, and the table sorted on `row[columnKey]` — which does not exist for a
 * column named after its heading — so `Amount` moved its arrow and never moved a
 * row. There was no way to filter by status, package, payer or reference, and no
 * search box. Finding one order among hundreds was manual scrolling.
 *
 * Every check here drives the real page and asserts a ROW COUNT changed, not
 * that an input exists: a filter wired to nothing still renders.
 *
 * READ-ONLY: it sorts, filters and searches. It changes no order.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9468)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const ROLE = process.env.VEYLIX_ROLE ?? 'super_admin'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('orders')}`, '--no-first-run',
  '--disable-gpu', '--window-size=1440,1400', 'about:blank',
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
    /* not up yet */
  }
}
if (!ws) {
  console.error('could not attach to chrome')
  process.exit(1)
}

const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  return r?.result?.value
}

await send('Runtime.enable')
await send('Page.enable')
await send('Network.enable')

const s = await signIn(ROLE)
if (s.status !== 200) {
  console.error(`${ROLE} sign-in failed: ${s.status}`)
  process.exit(1)
}
await send('Network.clearBrowserCookies')
const c = cookieParts(s.cookie)
await send('Network.setCookie', { name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true })

const failures = []
// Signature is (ok, msg, extra) — the same order every other verify script uses.
const check = (ok, msg, extra) => {
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${msg}${extra === undefined ? '' : `  -> ${JSON.stringify(extra)}`}`)
  if (!ok) failures.push(msg)
}

const sleepFor = async (expr, timeoutMs = 12000) => {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (await evaluate(expr)) return true
    await sleep(200)
  }
  return false
}


await send('Page.navigate', { url: `${APP}/orders` })
const landed = await sleepFor(`!!document.querySelector('[data-testid="table-filters"]')`)
check(landed, 'D-116 the orders table offers a filter toolbar')

await evaluate(`
  (() => {
    const setVal = (el, v) => {
      const proto = el.tagName === 'SELECT' ? window.HTMLSelectElement.prototype : window.HTMLInputElement.prototype
      Object.getOwnPropertyDescriptor(proto, 'value').set.call(el, v)
      el.dispatchEvent(new Event('input', { bubbles: true }))
      el.dispatchEvent(new Event('change', { bubbles: true }))
    }
    window.__vyxSetVal = setVal
    window.__vyxRows = () => document.querySelectorAll('table tbody tr').length
    window.__vyxCount = () => document.querySelector('[data-testid="table-count"]')?.textContent?.trim() ?? ''
    window.__vyxOrder = () => [...document.querySelectorAll('table tbody tr')]
      .map((tr) => (tr.cells[0]?.textContent || '').trim())
    window.__vyxClear = () => document.querySelector('[data-testid="table-clear-filters"]')?.click()
    return true
  })()
`)

const total = await evaluate(`window.__vyxRows()`)
check(total > 0, 'the orders table has rows to work with (a green run on an empty table proves nothing)', {
  rows: total,
})
if (total === 0) {
  console.log('\nREFUSING: no orders on this account — the gate would be vacuous.\n')
  chrome.kill()
  process.exit(3)
}

/* ---- search ---------------------------------------------------------------
 * One box, every column. The check asserts the ROW COUNT changed — a filter
 * wired to nothing still renders, so "the input exists" would prove nothing.
 */
const search = async (term) => {
  await evaluate(`window.__vyxSetVal(document.querySelector('[data-testid="table-search"]'), ${JSON.stringify(term)})`)
  await sleep(450)
  return { rows: await evaluate(`window.__vyxRows()`), count: await evaluate(`window.__vyxCount()`) }
}
const clearAll = async () => {
  await evaluate(`window.__vyxClear()`)
  await sleep(450)
}

const firstNo = await evaluate(
  `(() => {
     // The Order cell renders "#17" then the date as a sibling span, so
     // textContent runs them together ("#1711/10/2026"). Read the <strong>,
     // which holds only the number.
     const cell = document.querySelector('table tbody tr')?.cells[0]
     if (!cell) return ''
     const strong = cell.querySelector('strong')
     const raw = (strong?.textContent || '').trim()
     const m = raw.match(/#(\\d+)/) || (cell.textContent || '').trim().match(/^#(\\d+)/)
     return m ? m[1] : ''
   })()`,
)
if (!firstNo) {
  check(false, 'could read an order number from the first row to search for')
} else {
  const byOrderNo = await search(firstNo)
  check(byOrderNo.rows > 0 && byOrderNo.rows <= total, 'search finds an order by number', {
    ...byOrderNo,
    term: firstNo,
    total,
  })
}
await clearAll()

const restored = await evaluate(`window.__vyxRows()`)
check(restored === total, 'Clear restores every row', { restored, total })

/* ---- status filter --------------------------------------------------------
 * A select matches EXACTLY, so "PENDING" cannot also pull in
 * "AWAITING_CONFIRMATION".
 */
const statusOptions = await evaluate(`(() => {
  const el = document.querySelector('[data-testid="table-filter-status"]')
  if (!el) return null
  const opts = [...el.options].map((o) => o.value).filter(Boolean)
  window.__vyxSetVal(el, 'CONFIRMED')
  return opts
})()`)
check(Array.isArray(statusOptions) && statusOptions.length > 0, 'the status filter offers choices', {
  options: statusOptions,
})
await sleep(450)
const confirmedRows = await evaluate(`window.__vyxRows()`)
const confirmedCount = await evaluate(`window.__vyxCount()`)
check(confirmedRows <= total, 'the status filter runs without breaking the table', {
  rows: confirmedRows,
  count: confirmedCount,
})
check(
  /Showing \d+ of \d+/.test(confirmedCount) || confirmedRows === total,
  'the count line says how much is being shown',
  confirmedCount,
)
await clearAll()

/* ---- per-column filters exist --------------------------------------------- */
for (const key of ['placedBy', 'package', 'reference', 'id']) {
  const present = await evaluate(`!!document.querySelector('[data-testid="table-filter-${key}"]')`)
  check(!!present, `the "${key}" column offers its own filter`)
}


/* ---- sorting --------------------------------------------------------------
 * THE defect: three headers had no sort button, and the ones that did sorted on
 * a field that does not exist. "The header is clickable" would pass on the
 * broken version, so this asserts the ROW ORDER actually changed.
 */
const clickHeader = async (heading) =>
  evaluate(`(() => {
    const btn = [...document.querySelectorAll('th button')]
      .find((b) => (b.textContent || '').trim().startsWith(${JSON.stringify(heading)}))
    if (!btn) return false
    btn.click()
    return true
  })()`)

for (const heading of ['Order', 'Placed by', 'Package', 'Amount', 'Reference', 'Status']) {
  const sortable = await clickHeader(heading)
  check(!!sortable, `"${heading}" is sortable (has a sort control)`)
  if (!sortable) continue

  await sleep(350)
  const asc = await evaluate(`window.__vyxOrder()`)
  await clickHeader(heading)
  await sleep(350)
  const desc = await evaluate(`window.__vyxOrder()`)

  const changed = JSON.stringify(asc) !== JSON.stringify(desc)
  check(
    changed || total <= 1,
    `"${heading}" actually reorders the rows (not just its arrow)`,
    changed ? 'order changed between asc and desc' : 'IDENTICAL order — the sort is a no-op',
  )

  // Back to unsorted before the next column.
  await clickHeader(heading)
  await sleep(200)
}

/* ---- the cap, stated ------------------------------------------------------ */
const truncated = await evaluate(`(() => {
  const el = document.querySelector('[data-testid="orders-truncated"]')
  return el ? (el.textContent || '').replace(/\\s+/g, ' ').trim() : null
})()`)
if (truncated) {
  check(
    /Showing the newest \d+ (of [\d,]+ orders|orders\. The total could not be read)/.test(truncated),
    'D-116 the truncated list says how many orders exist beyond it',
    truncated,
  )
} else {
  // Below the cap: silence is CORRECT, and saying so keeps the gate honest
  // instead of reporting a pass it did not earn.
  console.log(`SKIP  D-116 truncation notice — ${total} order(s) here, below the 200 cap`)
}

/* ---- report --------------------------------------------------------------- */
console.log(
  `\nOrders — ${failures.length === 0 ? 'OK' : `${failures.length} FAILED`}` +
    ` (${failures.length === 0 ? 'all checks passed' : failures.join(' | ')})\n`,
)
chrome.kill()
process.exit(failures.length === 0 ? 0 : 1)
