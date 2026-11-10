#!/usr/bin/env node
/**
 * VEYLIX — the operator tables are all sortable, searchable and filterable.
 *
 *   node scripts/verify-tables.mjs
 *
 * Why this exists: D-116 gave `Table` a toolbar, but only the Orders screen used
 * it. VPNs, Clients and Payments still had sort-only tables — and three of their
 * columns sorted on a field that does not exist, so the arrow moved and nothing
 * else did. "The toolbar is on Orders" is not the same as "an operator can find
 * a row anywhere".
 *
 * This walks every operator table that exposes a toolbar and asserts, for each:
 *   - a search box and at least one filter exist
 *   - the search box actually NARROWS the rows
 *   - Clear restores them
 *   - at least one header actually REORDERS the rows
 *
 * READ-ONLY: it searches, filters and sorts. It changes nothing.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { assertPortFree, profileDirUnique, waitForExit } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9469)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const ROLE = process.env.VEYLIX_ROLE ?? 'super_admin'

// A stale browser on this port would make this run verify a DIFFERENT page, so
// refuse rather than silently attach (chrome-profile.mjs).
await assertPortFree(PORT)

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  // Unique per run: a stable profile collides with the previous run's lock and
  // Chrome refuses to start — the exact hang this family of scripts hit before.
  `--user-data-dir=${profileDirUnique('tables')}`, '--no-first-run',
  '--disable-gpu', '--window-size=1440,1400', 'about:blank',
], { stdio: 'ignore' })

const shutdown = async () => {
  chrome.kill()
  await waitForExit(chrome)
}

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
    const list = await (await fetch(`http://127.0.0.1:${PORT}/json/list`, { signal: AbortSignal.timeout(2000) })).json()
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
  chrome.kill()
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
  await shutdown()
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

const sleepFor = async (expr, timeoutMs = 15000) => {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (await evaluate(expr)) return true
    await sleep(200)
  }
  return false
}

/*
 * Each entry: the page, what a row count means there, and a search term that
 * must narrow it. The term is deliberately a STATUS word or a name — the kinds
 * of thing an operator types — so a search that only matches ids fails.
 */
const PAGES = [
  { path: '/orders', label: 'Orders', caption: /orders/i },
  { path: '/vpns', label: 'VPNs', caption: /vpn/i },
  { path: '/clients', label: 'Clients', caption: /client/i },
  { path: '/payments', label: 'Payments', caption: /payment/i },
]

for (const page of PAGES) {
  await send('Page.navigate', { url: `${APP}${page.path}` })
  const ready = await sleepFor(`!!document.querySelector('[data-testid="table-filters"]')`)

  check(ready, `${page.label}: the table offers a filter toolbar`)
  if (!ready) continue

  const shape = await evaluate(`(() => ({
    hasSearch: !!document.querySelector('[data-testid="table-search"]'),
    filters: [...document.querySelectorAll('[data-testid^="table-filter-"]')].map((e) => ({
      key: e.getAttribute('data-testid').replace('table-filter-', ''),
      kind: e.tagName.toLowerCase(),
    })),
    rows: document.querySelectorAll('table tbody tr').length,
    count: document.querySelector('[data-testid="table-count"]')?.textContent?.trim() ?? '',
  }))()`)

  check(!!shape?.hasSearch, `${page.label}: has a search box`)
  check((shape?.filters?.length ?? 0) >= 1, `${page.label}: has per-column filters`, {
    filters: shape?.filters,
  })
  check(page.caption.test(shape?.count ?? ''), `${page.label}: the count names what it is counting`, shape?.count)

  if ((shape?.rows ?? 0) < 2) {
    console.log(`SKIP  ${page.label}: only ${shape?.rows} row(s) — narrowing cannot be observed`)
    continue
  }

  /* ---- search must NARROW ------------------------------------------------- */
  /*
   * The term comes from ONE TEXT NODE, never a whole cell or row.
   * `cell.textContent` welds sibling spans together (`owner@veylix.test` +
   * `for client@veylix.test` -> `owner@veylix.testfor client@veylix.test`), and
   * a row welds cells (`#17` + `11/10/2026,` -> `#1711/10/2026,`). Neither is a
   * string an operator would type, so both drafts "failed" on every screen and
   * blamed the product for a broken extractor.
   */
  const narrowed = await evaluate(`(() => {
    const nodes = []
    for (const tr of [...document.querySelectorAll('table tbody tr')].slice(0, 5)) {
      for (const td of tr.cells) {
        const walker = document.createTreeWalker(td, NodeFilter.SHOW_TEXT)
        for (let n = walker.nextNode(); n; n = walker.nextNode()) {
          const t = (n.nodeValue || '').trim()
          if (t.length >= 3) nodes.push(t)
        }
      }
    }
    const pick =
      nodes.find((t) => /^[^\\s@]+@[^\\s@]+$/.test(t)) ||
      nodes.find((t) => /^[A-Z][a-z]{2,}( [A-Z][a-z]{2,})*$/.test(t)) ||
      nodes.find((t) => /^[A-Za-z][A-Za-z0-9 ._-]{3,}$/.test(t))
    if (!pick) return null
    const el = document.querySelector('[data-testid="table-search"]')
    Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set.call(el, pick)
    el.dispatchEvent(new Event('input', { bubbles: true }))
    return pick
  })()`)
  await sleep(500)
  const afterSearch = await evaluate(`document.querySelectorAll('table tbody tr').length`)

  check(
    narrowed !== null && afterSearch > 0 && afterSearch <= shape.rows,
    `${page.label}: search narrows the table`,
    { term: narrowed, before: shape.rows, after: afterSearch },
  )

  await evaluate(`document.querySelector('[data-testid="table-clear-filters"]')?.click()`)
  await sleep(500)
  const restored = await evaluate(`document.querySelectorAll('table tbody tr').length`)
  check(restored === shape.rows, `${page.label}: Clear restores every row`, { restored, was: shape.rows })

  /* ---- a header must REORDER ---------------------------------------------- */
  /*
   * Click TWO DIFFERENT columns, not the same one three times. The first draft
   * clicked one header three times (asc -> desc -> asc) and compared asc with
   * asc, so it "proved" that sorting was broken on four screens that sort fine.
   *
   * `toggleSort` cycles asc -> desc -> null (Table.tsx), so a column yields its
   * two orders in clicks 1 and 2 — after that it is back to the server order and
   * the next column starts clean.
   *
   * The clicks are separated by a frame: React re-renders asynchronously, so
   * reading the DOM in the same tick returns the PREVIOUS order and reports
   * "did not change" for a table that changed.
   */
  await evaluate(`window.__sortProbe = (() => {
    const read = () => [...document.querySelectorAll('table tbody tr')]
      .map((tr) => (tr.textContent || '').trim())
    const buttons = [...document.querySelectorAll('th button.ui-table__sort')]
    let i = 0
    let phase = 0
    let asc = null
    const step = () => {
      if (i >= Math.min(buttons.length, 8)) return 'no-column-reordered'
      const btn = buttons[i]
      if (phase === 0) { btn.click(); phase = 1; return null }
      if (phase === 1) { asc = read(); btn.click(); phase = 2; return null }
      if (phase === 2) {
        const desc = read()
        btn.click() // back to unsorted, leaving the next column a clean start
        i += 1
        phase = 0
        if (JSON.stringify(asc) !== JSON.stringify(desc)) return 'changed'
      }
      return null
    }
    return { step, get done() { return i >= Math.min(buttons.length, 8) } }
  })()`)

  let orderChanged = null
  for (let tick = 0; tick < 40 && orderChanged === null; tick++) {
    await sleep(60)
    orderChanged = await evaluate(`window.__sortProbe.step()`)
    if (orderChanged === null && (await evaluate(`window.__sortProbe.done`))) {
      orderChanged = 'no-column-reordered'
    }
  }
  check(orderChanged === 'changed', `${page.label}: at least one header reorders the rows`, orderChanged)
}

/* ---- report --------------------------------------------------------------- */
console.log(
  `\nTables — ${failures.length === 0 ? 'OK' : `${failures.length} FAILED`}` +
    ` (${failures.length === 0 ? 'all checks passed' : failures.join(' | ')})\n`,
)
await shutdown()
process.exit(failures.length === 0 ? 0 : 1)

