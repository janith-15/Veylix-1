#!/usr/bin/env node
/**
 * VEYLIX — Usage & History SEARCH (D-097), in a real browser.
 *
 *   node scripts/verify-client-usage-search.mjs
 *
 * Why this exists: two complaints, one cause.
 *
 *   1. "Buying a lot of VPNs makes the one you want impossible to find." The
 *      only way to reach a VPN was a select of package names, and two VPNs off
 *      the same package looked identical in it.
 *   2. "When a lot of history arrives the user gets lost." Every owned VPN was
 *      fetched whether or not it was on screen, so narrowing the view changed
 *      what you SAW but not what the page COST.
 *
 * This gate asserts both, including the one that is easy to fake: that a search
 * actually REDUCES the number of requests. A search box that filters the DOM
 * after fetching everything would pass every other check here and still leave
 * the owner's second complaint untouched.
 *
 * READ-ONLY. It navigates, types, and counts the `GET`s the page itself makes.
 * It writes nothing and changes no VPN.
 *
 * It REFUSES (exit 3) when the account owns fewer than two VPNs, because
 * "narrows the list" is not observable with a single row.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn, API } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9469)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const ROLE = process.env.VEYLIX_ROLE ?? 'client'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('usagesearch')}`, '--no-first-run',
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

/* Every usage request the page issues, so the cost of a filter can be counted
   rather than assumed. */
const usageRequests = []
ws.addEventListener('message', (m) => {
  const msg = JSON.parse(m.data)
  if (msg.method !== 'Network.requestWillBeSent') return
  const url = msg.params?.request?.url ?? ''
  if (/\/api\/vpns\/\d+\/(usage|usage\/hourly)/.test(url)) usageRequests.push(url)
})

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
/** The VPNs the screen should fetch: a dormant one is skipped by design (D-094). */
const awake = rows.filter((v) => !v.dormant_at)

async function until(expr, tries = 30) {
  for (let i = 0; i < tries; i++) {
    const v = await evaluate(expr)
    if (v) return v
    await sleep(300)
  }
  return null
}

const blockCount = () =>
  evaluate(`document.querySelectorAll('[data-testid^="usage-vpn-"]').length`)

/** Types into the search the way a person does, then waits out the debounce. */
const search = async (term) => {
  const ok = await evaluate(`(() => {
    const el = document.querySelector('[data-testid="usg-search"]')
    if (!el) return false
    const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
    set.call(el, ${JSON.stringify(term)})
    el.dispatchEvent(new Event('input', { bubbles: true }))
    return true
  })()`)
  await sleep(900)
  return ok
}

/* ---- 1. the search exists and filters ---------------------------------- */

await send('Page.navigate', { url: `${APP}/client/usage` })
await sleep(1200)
await until(`document.querySelectorAll('[data-testid^="usage-vpn-"]').length >= ${awake.length}`)

const fullBlocks = await blockCount()
check(fullBlocks === awake.length, 'every awake VPN renders a block',
  { blocks: fullBlocks, awake: awake.length })
const fullRequests = usageRequests.length
check(fullRequests >= awake.length, 'the unfiltered page reads every VPN', { requests: fullRequests })

const hasSearch = await evaluate(`!!document.querySelector('[data-testid="usg-search"]')`)
check(hasSearch, 'the Usage screen has a search box (it had none before D-097)')

/* Search by the PANEL EMAIL, not the package name: that is the identity a
   customer and an operator both use (D-091.4), and two VPNs off one package
   share a package name. */
const target = awake[0]
const email = String(target.panel_email ?? '')
check(email.length > 0, 'the target VPN has a panel email to search for', email)

await search(email)
const narrowed = await blockCount()
check(narrowed === 1, 'searching the panel email narrows to that one VPN',
  { blocks: narrowed, term: email })

/* ---- 2. the choice lives in the URL ------------------------------------ */

const url = await evaluate('location.search')
check(url.includes('q='), 'the search term is held in the URL (shareable, Back works)', url)

/* ---- 3. searching REDUCES the requests (the whole point) --------------- */

const beforeNarrow = fullRequests
const afterNarrow = usageRequests.length
check(afterNarrow > beforeNarrow, 'the narrowed view issued its own reads', {
  before: beforeNarrow, after: afterNarrow,
})

/* A search that fetches everything then hides the extras would pass every check
   above. This is the one that cannot be faked: a reload straight into the
   filtered URL must read ONE VPN, not all of them. */
usageRequests.length = 0
await send('Page.navigate', { url: `${APP}/client/usage?q=${encodeURIComponent(email)}` })
await sleep(1600)
await until(`document.querySelectorAll('[data-testid^="usage-vpn-"]').length === 1`)
const filteredOnly = usageRequests.length
check(filteredOnly <= 2,
  'a filtered view reads ONLY the filtered VPN, not the whole account (D-097)',
  { requests: filteredOnly, awakeVPNs: awake.length })

/* ---- 4. an unmatched search explains itself ---------------------------- */

await search('zzz-no-such-vpn-zzz')
const noMatch = await until(`document.body.textContent.includes('No VPN matches')`)
check(Boolean(noMatch), 'an unmatched search says no VPN matches it')
check((await blockCount()) === 0, 'an unmatched search renders no usage block')

const back = await until(
  `[...document.querySelectorAll('button')].some((b) => b.textContent.trim() === 'Show all VPNs')`)
check(Boolean(back), 'the empty state offers a way back to every VPN')

/* ---- 5. clearing restores everything ---------------------------------- */

await evaluate(`(() => {
  const b = [...document.querySelectorAll('button')].find((x) => x.textContent.trim() === 'Show all VPNs')
  if (b) { b.click(); return true }
  return false
})()`)
await sleep(1200)
const restored = await until(
  `document.querySelectorAll('[data-testid^="usage-vpn-"]').length === ${awake.length}`)
check(Boolean(restored), 'clearing the search restores every VPN block',
  { blocks: await blockCount() })

console.log(`\n${failures.length === 0 ? 'OK' : 'FAILED'}  ${failures.length} failure(s)`)
try { chrome.kill() } catch { /* already gone */ }
process.exit(failures.length === 0 ? 0 : 1)

