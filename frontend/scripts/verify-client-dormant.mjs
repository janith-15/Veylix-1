#!/usr/bin/env node
/**
 * VEYLIX — the client "Dormant VPN" switch (D-094), in a real browser.
 *
 *   node scripts/verify-client-dormant.mjs
 *
 * Why this exists: D-094 shipped with route tests, sync tests and a green
 * build. None of those prove the thing the customer actually bought — that a
 * sleeping VPN renders LAST, says why, leaves the "Needs attention" count, and
 * offers a working Wake. This script walks exactly that.
 *
 * THE ONE WRITE IT MAKES
 * ----------------------
 * Proving "asleep" requires something to fall asleep. So the script toggles ONE
 * VPN belonging to the CLIENT account it is signed in as, and **always wakes it
 * again** in a `finally` block — including on failure — so the seeded account is
 * left exactly as it was found. It never deletes, releases, provisions or edits
 * a panel client, and it never touches staff data or a production deployment.
 *
 * It REFUSES (exit 3) when there is nothing to prove: fewer than two VPNs, or no
 * row carrying `dormant_at` on the wire. A green run against an empty account
 * would be exactly the false evidence this programme exists to remove.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn, API } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9462)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const ROLE = process.env.VEYLIX_ROLE ?? 'client'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('dormant')}`, '--no-first-run',
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

/**
 * Wakes a VPN through the API.
 *
 * The `Origin` header is REQUIRED, not optional: the backend refuses any
 * state-changing request from a browser-shaped caller without one
 * (`403 csrf_blocked`) — a deliberate cross-site write guard. A raw `fetch` from
 * node sends `Sec-Fetch-*` on its own, so it is treated as a browser and refused;
 * the first run of this script failed its restore for exactly that reason.
 *
 * The origin must match the host the request ARRIVES on, which is the API
 * (`API`), not the Vite dev server — the guard compares `Origin` against the
 * request's own scheme+host. Sending the API's own origin is what a same-origin
 * page at `:8080` does, and it is what makes the restore honest.
 */
const wakeViaApi = (id) =>
  fetch(`${API}/api/vpns/${id}/wake`, {
    method: 'POST',
    headers: { cookie: s.cookie, origin: API, 'content-type': 'application/json' },
  })

/** Reads the owner's VPN rows straight from the API, bypassing the UI. */
const readRows = async () => {
  const r = await fetch(`${API}/api/vpns`, { headers: { cookie: s.cookie } })
  if (r.status !== 200) throw new Error(`list ${r.status}`)
  const j = await r.json()
  return Array.isArray(j) ? j : (j.vpns ?? [])
}

const rows = await readRows()
if (rows.length < 2) {
  console.error(`REFUSING: the ${ROLE} account owns ${rows.length} VPN(s).`)
  console.error('Normality needs at least two, so "sorts last" is observable rather than vacuous.')
  process.exit(3)
}
if (!Object.prototype.hasOwnProperty.call(rows[0], 'dormant_at')) {
  console.error('REFUSING: GET /api/vpns does not expose `dormant_at`.')
  console.error('The running backend predates D-094 (migration 0025). Restart it via .\\dev.ps1.')
  process.exit(3)
}

/** Waits for a page expression to become truthy, then returns its value. */
async function until(expr, tries = 30) {
  for (let i = 0; i < tries; i++) {
    const v = await evaluate(expr)
    if (v) return v
    await sleep(300)
  }
  return null
}

/** Card order as the DOM renders it — a higher apiId never implies "first". */
const cardOrder = () =>
  evaluate(`[...document.querySelectorAll('[data-testid^="vpn-card-"]')].map((n) =>
    ({ id: Number(n.dataset.testid.replace('vpn-card-', '')), dormant: n.dataset.dormant }))`)

/** The "Needs attention" value rendered on the overview strip. */
const attentionValue = () =>
  evaluate(`(() => {
    const boxes = [...document.querySelectorAll('.mv-ov')]
    for (const b of boxes) {
      const label = b.querySelector('.mv-ov__label')?.textContent?.trim()
      if (label === 'Needs attention') return Number(b.querySelector('.mv-ov__value')?.textContent?.trim())
    }
    return null
  })()`)

const target = rows[0].id
let slept = false

try {
  await send('Page.navigate', { url: `${APP}/client/vpns` })
  await sleep(1500)
  await until(`document.querySelectorAll('[data-testid^="vpn-card-"]').length >= ${rows.length}`)

  const before = await cardOrder()
  check(Array.isArray(before) && before.length === rows.length,
    'every owned VPN renders a card', { cards: before?.length, rows: rows.length })
  check(before?.every((c) => c.dormant === 'false'),
    'nothing is dormant before the switch is used')
  const beforeAttention = await attentionValue()
  check(typeof beforeAttention === 'number',
    'the overview renders a numeric "Needs attention" value', beforeAttention)

  /* ---- sleep it ------------------------------------------------------- */
  const btn = `[data-testid="vpn-sleep-${target}"]`
  const label = await evaluate(`document.querySelector('${btn}')?.textContent?.trim()`)
  check(label === 'Sleep', 'the control offers "Sleep" for an awake VPN', label)

  await evaluate(`document.querySelector('${btn}').click(); true`)
  await sleep(800)
  slept = true

  const noted = await until(
    `document.querySelector('[data-testid="vpn-dormant-note-${target}"]') !== null`)
  check(Boolean(noted), 'the card explains the sleep state in a sentence')

  const sentence = await evaluate(
    `document.querySelector('[data-testid="vpn-dormant-note-${target}"]')?.textContent ?? ''`)
  check(/put this VPN to sleep/i.test(sentence),
    'the sentence names what happened', sentence.slice(0, 80))
  check(/nothing was deleted/i.test(sentence),
    'the sentence states that nothing was deleted — the exact fear this must not create')

  const after = await cardOrder()
  check(after?.[after.length - 1]?.id === target,
    'the dormant VPN sorts LAST', { order: after?.map((v) => v.id), target })
  check(after?.find((v) => v.id === target)?.dormant === 'true',
    'the card advertises its dormant state in the DOM')

  const afterAttention = await attentionValue()
  check(afterAttention === Math.max(0, beforeAttention - 1),
    'the dormant VPN leaves the "Needs attention" count',
    { before: beforeAttention, after: afterAttention })

  const wakeLabel = await evaluate(`document.querySelector('${btn}')?.textContent?.trim()`)
  check(wakeLabel === 'Wake', 'the control flips to "Wake"', wakeLabel)

  /* ---- the sort survives a different sort key ------------------------- */
  await send('Page.navigate', { url: `${APP}/client/vpns?sort=name` })
  await sleep(1500)
  await until(`document.querySelectorAll('[data-testid^="vpn-card-"]').length === ${rows.length}`)
  const byName = await cardOrder()
  check(byName?.[byName.length - 1]?.id === target,
    'sorting by NAME does not float it back to the top', { order: byName?.map((v) => v.id) })

  /* ---- sleeping really stops the polling ------------------------------ */
  await send('Page.navigate', { url: `${APP}/client/usage` })
  await sleep(600)
  const dormantNote = await until(
    `document.querySelector('[data-testid="usage-dormant-${target}"]') !== null`)
  check(Boolean(dormantNote), 'the Usage screen explains that a dormant VPN is not measured')
  check(!(await evaluate(
    `document.querySelector('[data-testid="usage-vpn-${target}"] .usg__meter') !== null`)),
    'the dormant VPN renders NO allowance meter (nothing was requested, so nothing is loading)')
} finally {
  /* ---- ALWAYS wake it again, even if an assertion above threw --------- */
  if (slept) {
    await wakeViaApi(target)
    const restored = await readRows()
    const row = restored.find((v) => v.id === target)
    if (row?.dormant_at !== null && row?.dormant_at !== undefined) {
      console.error(`WARNING: could not restore VPN ${target} to awake (dormant_at=${row?.dormant_at})`)
      failures.push('the seeded VPN was left dormant')
    } else {
      console.log(`RESTORED  VPN ${target} is awake again — the account is as it was found`)
    }
  }
}

console.log(`\n${failures.length === 0 ? 'OK' : 'FAILED'}  ${failures.length} failure(s)`)
try { chrome.kill() } catch { /* already gone */ }
process.exit(failures.length === 0 ? 0 : 1)
