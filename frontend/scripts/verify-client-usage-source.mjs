#!/usr/bin/env node
/**
 * VEYLIX — the Usage & History SOURCE filter (D-095), in a real browser.
 *
 *   node scripts/verify-client-usage-source.mjs
 *
 * Why this exists: D-095 added a source picker to `/client/usage`. A select that
 * renders but filters nothing is indistinguishable from a working one until
 * somebody looks, so this walks it: it proves the list actually narrows, that the
 * choice lives in the URL (shareable, and Back works), and that a filter matching
 * nothing says so instead of rendering an empty list that reads as "no traffic".
 *
 * READ-ONLY. It loads a page, picks options and reads the DOM. It issues no
 * write, creates nothing and changes no VPN. The only server calls are the `GET`s
 * the page itself makes.
 *
 * It REFUSES (exit 3) when the account owns fewer than two VPNs, because
 * "narrows the list" is not observable with a single row.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn, API } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9464)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const ROLE = process.env.VEYLIX_ROLE ?? 'client'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('usagesource')}`, '--no-first-run',
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
const check = (ok, msg, extra) => {
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${msg}${extra === undefined ? '' : `  -> ${JSON.stringify(extra)}`}`)
  if (!ok) failures.push(msg)
}

const rows = await (async () => {
  const r = await fetch(`${API}/api/vpns`, { headers: { cookie: s.cookie } })
  const j = await r.json()
  return Array.isArray(j) ? j : (j.vpns ?? [])
})()
if (rows.length < 2) {
  console.error(`REFUSING: the ${ROLE} account owns ${rows.length} VPN(s).`)
  console.error('Narrowing a list needs at least two rows to be observable.')
  process.exit(3)
}

/** Waits for an expression to become truthy. */
async function until(expr, tries = 30) {
  for (let i = 0; i < tries; i++) {
    const v = await evaluate(expr)
    if (v) return v
    await sleep(300)
  }
  return null
}

/** How many VPN blocks the page is rendering. */
const blockCount = () =>
  evaluate(`document.querySelectorAll('[data-testid^="usage-vpn-"]').length`)

const pickerValue = () =>
  evaluate(`document.querySelector('[data-testid="usg-source"]')?.value ?? null`)

/** Picks a value in the source select the way a user would. */
const pick = (value) => evaluate(`(() => {
  const el = document.querySelector('[data-testid="usg-source"]')
  if (!el) return false
  const set = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value').set
  set.call(el, ${JSON.stringify(value)})
  el.dispatchEvent(new Event('change', { bubbles: true }))
  return true
})()`)

await send('Page.navigate', { url: `${APP}/client/usage` })
await sleep(1200)
await until(`document.querySelectorAll('[data-testid^="usage-vpn-"]').length >= ${rows.length}`)

const all = await blockCount()
check(all === rows.length, 'every owned VPN renders a usage block', { blocks: all, rows: rows.length })
check((await pickerValue()) === 'all', 'the picker starts on "All VPNs"', await pickerValue())

/* ---- narrow to ONE VPN ------------------------------------------------- */
const targetId = rows[0].id
check(await pick(`vpn:${targetId}`), 'the source picker accepts a VPN selection')
await sleep(700)

const narrowed = await blockCount()
check(narrowed === 1, 'selecting one VPN leaves exactly one block', { blocks: narrowed })

/* The URL must carry the choice: that is what makes it shareable and lets Back
   undo it. Component-only state would pass the check above and fail this one. */
const url = await evaluate('location.search')
check(url.includes('source=vpn%3A' + targetId) || url.includes('source=vpn:' + targetId),
  'the choice is held in the URL', url)

/* ---- a filter that matches nothing must SAY so ------------------------- */
await send('Page.navigate', { url: `${APP}/client/usage?source=vpn:999999` })
await sleep(1200)
const stale = await until(`document.body.textContent.includes('No usage to show')`)
check(Boolean(stale), 'a filter matching no VPN explains itself instead of rendering an empty list')
check((await blockCount()) === 0, 'the empty state renders no usage block at all')
const back = await until(
  `[...document.querySelectorAll('button')].some((b) => b.textContent.trim() === 'Show all VPNs')`)
check(Boolean(back), 'the empty state offers a way back to every VPN')

/* ---- and clearing it really shows everything again --------------------- */
await evaluate(`(() => {
  const b = [...document.querySelectorAll('button')].find((x) => x.textContent.trim() === 'Show all VPNs')
  if (b) { b.click(); return true }
  return false
})()`)
await sleep(900)
const restored = await until(
  `document.querySelectorAll('[data-testid^="usage-vpn-"]').length === ${rows.length}`)
check(Boolean(restored), 'clearing the filter restores every VPN block', { blocks: await blockCount() })

console.log(`\n${failures.length === 0 ? 'OK' : 'FAILED'}  ${failures.length} failure(s)`)
try { chrome.kill() } catch { /* already gone */ }
process.exit(failures.length === 0 ? 0 : 1)

