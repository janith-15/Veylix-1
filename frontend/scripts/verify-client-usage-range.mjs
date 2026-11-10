#!/usr/bin/env node
/**
 * VEYLIX — the Usage range/window honesty gate (D-096), in a real browser.
 *
 *   node scripts/verify-client-usage-range.mjs
 *
 * Why this exists: the Usage screen used to label its chart "Last 30 days" while
 * the server returned the newest 240 raw rows for EVERY range. Measured on the
 * dev data, all six ranges came back as the same 116 points spanning 0.80 days —
 * so the label named a period the data never covered. That is fake data on a
 * customer screen, and no build or type check can see it.
 *
 * This gate asserts the two things that make the label true:
 *
 *   1. The server gives each range a DIFFERENT bucket width (hourly 24h, daily
 *      30d, monthly 1y). Identical widths mean the selector does nothing.
 *   2. The caption states the REAL span of the points, and never claims a period
 *      longer than the data actually covers.
 *
 * READ-ONLY. It navigates, reads the DOM and issues the same `GET`s the page
 * makes. It writes nothing.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn, API } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9467)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const ROLE = process.env.VEYLIX_ROLE ?? 'client'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('usagerange')}`, '--no-first-run',
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
if (rows.length === 0) {
  console.error(`REFUSING: the ${ROLE} account owns no VPN.`)
  process.exit(3)
}
const vpnId = rows[0].id

async function until(expr, tries = 30) {
  for (let i = 0; i < tries; i++) {
    const v = await evaluate(expr)
    if (v) return v
    await sleep(300)
  }
  return null
}


/* ---- 1. the SERVER must give every range a different bucket width -------- */

const perRange = []
for (const range of ['24h', '7d', '30d', '3m', '1y']) {
  const r = await fetch(`${API}/api/vpns/${vpnId}/usage?range=${range}`, { headers: { cookie: s.cookie } })
  const j = await r.json()
  perRange.push({
    range,
    bucketSec: j.window?.bucket_sec ?? 0,
    bucket: j.window?.bucket ?? '',
    points: (j.points ?? []).length,
  })
}

const widths = perRange.map((p) => p.bucketSec)
check(widths.every((w) => w > 0), 'every range reports a bucket width', perRange)
check(new Set(widths).size >= 4,
  'the ranges choose DIFFERENT bucket widths, so the selector changes the picture',
  perRange.map((p) => `${p.range}=${p.bucket}`))
check(perRange[0].bucketSec < perRange[2].bucketSec && perRange[2].bucketSec < perRange[4].bucketSec,
  'the bucket width grows with the window (24h < 30d < 1y)',
  perRange.map((p) => `${p.range}=${p.bucketSec}`))

/* The bug this gate exists for: six ranges, one identical series. */
const signatures = []
for (const range of ['24h', '7d', '30d']) {
  const r = await fetch(`${API}/api/vpns/${vpnId}/usage?range=${range}`, { headers: { cookie: s.cookie } })
  const j = await r.json()
  const pts = j.points ?? []
  const first = pts.length > 0 ? pts[0].taken_at : 0
  const last = pts.length > 0 ? pts[pts.length - 1].taken_at : 0
  signatures.push(`${pts.length}:${last - first}`)
}
check(new Set(signatures).size > 1,
  'different ranges no longer return an identical series (the D-096 regression)',
  signatures)

/* ---- 2. the CAPTION must state the real span ---------------------------- */

await send('Page.navigate', { url: `${APP}/client/usage` })
await sleep(1200)
await until(`document.querySelectorAll('[data-testid^="usage-vpn-"]').length > 0`)

const caption = await until(
  `document.querySelector('[data-testid="usage-daily-span"]')?.textContent ?? ''`)
check(Boolean(caption), 'the daily chart caption renders', caption)

/* The caption must name the bucket the server reported — that is how a reader
   can tell an hourly chart from a daily one. */
const bucketForBlock = await (async () => {
  const r = await fetch(`${API}/api/vpns/${vpnId}/usage?range=30d`, { headers: { cookie: s.cookie } })
  const j = await r.json()
  return j.window?.bucket ?? ''
})()
check(typeof caption === 'string' && caption.includes(`${bucketForBlock} per point`),
  'the caption names the bucket width the server actually used', { caption, bucketForBlock })

/* The caption must never claim more history than the points cover. If the data
   is only hours long, the caption must not read "30 days of history". */
const realDays = await (async () => {
  const r = await fetch(`${API}/api/vpns/${vpnId}/usage?range=30d`, { headers: { cookie: s.cookie } })
  const j = await r.json()
  const pts = j.points ?? []
  if (pts.length < 2) return 0
  return (pts[pts.length - 1].taken_at - pts[0].taken_at) / 86400
})()
const claimsDays = /(\d+)\s*days? of history/.exec(String(caption ?? ''))
if (claimsDays) {
  const claimed = Number(claimsDays[1])
  check(claimed <= Math.max(1, Math.ceil(realDays)),
    'the caption does not claim more history than the data covers',
    { claimed, realDays: Math.round(realDays * 100) / 100 })
} else {
  check(true, 'the caption states no day count, so it cannot overstate days', caption)
}

/* The axis must not hard-code "30d ago" any more — that label was the visible
   symptom of the bug. The chart is an SVG, so this reads its text nodes. */
const axisText = await evaluate(
  `[...document.querySelectorAll('figure svg text')].map((t) => t.textContent.trim()).join('|')`)
check(!String(axisText).includes('30d ago'),
  'the axis no longer labels itself "30d ago" regardless of the data', axisText)

console.log(`\n${failures.length === 0 ? 'OK' : 'FAILED'}  ${failures.length} failure(s)`)
try { chrome.kill() } catch { /* already gone */ }
process.exit(failures.length === 0 ? 0 : 1)

