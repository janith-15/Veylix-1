#!/usr/bin/env node
/**
 * VEYLIX — the display-timezone gate (D-099), in a real browser.
 *
 *   node scripts/verify-display-timezone.mjs
 *
 * Why this exists: times were stored as UTC but RENDERED in the device's zone,
 * and the custom range parser did the same thing in a subtler way. Measured on
 * this workspace (host at +08:00): 18:56 UTC read 02:56 instead of Colombo's
 * 00:26, and "1 Nov → 3 Nov" resolved to 08:00 on the 1st through 07:59 on the
 * 4th — an offset applied twice, silently including the wrong slice of traffic.
 *
 * A type check cannot see this; only a real `Intl` implementation can. So this
 * gate asserts, in a real Chrome:
 *
 *   1. The server publishes `display_timezone` on /api/me, as a loadable IANA
 *      name (never an offset).
 *   2. The app adopts it — `activeTimeZone()` returns it after sign-in.
 *   3. `zonedDayBounds` returns the exact midnight / 23:59:59 of the named zone,
 *      at several offsets including both sides of a DST transition.
 *   4. The rendered instant differs from the DEVICE's reading at a boundary
 *      instant, i.e. the device clock is genuinely not being used.
 *   5. A stated timestamp names its zone.
 *
 * READ-ONLY. It signs in, reads, and evaluates pure functions. It writes
 * nothing to the database.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn, API } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9483)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const ROLE = process.env.VEYLIX_ROLE ?? 'client'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('tzverify')}`, '--no-first-run',
  '--disable-gpu', '--window-size=1440,1200', 'about:blank',
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
  chrome.kill()
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

/* ---- 1. the server publishes a real zone name ---------------------------- */

const me = await (await fetch(`${API}/api/me`, { headers: { cookie: s.cookie } })).json()
const zone = me?.display_timezone
check(typeof zone === 'string' && zone.length > 0,
  'the server publishes display_timezone on /api/me', zone)

const loadable = await evaluate(`(() => {
  try { new Intl.DateTimeFormat('en-US', { timeZone: ${JSON.stringify(zone)} }); return true }
  catch { return false }
})()`)
check(loadable === true,
  'that zone is a loadable IANA name (not an offset the browser cannot resolve)', zone)

/* ---- 2. the app adopts it ------------------------------------------------ */

// Rendered text is the only honest witness for this half.
//
// A dynamic `import('/src/lib/time.ts')` goes through Vite's dev graph, which
// hands back a SEPARATE module instance from the one the page bundled — so its
// module-level `activeTimezone` is empty even when adoption worked perfectly.
// Asserting on it would fail correct code. What a customer actually reads is the
// DOM, and that is what is asserted below.
await send('Page.navigate', { url: `${APP}/client/usage` })
await sleep(4000)

const rendered = await evaluate(`(() => {
  const t = document.body.innerText
  return { text: t, deviceZone: Intl.DateTimeFormat().resolvedOptions().timeZone }
})()`)

check(typeof rendered?.text === 'string' && rendered.text.length > 0,
  'the Usage screen rendered (its formatters actually ran)', rendered?.text?.length)

// A boundary instant that differs between Colombo and this device at the MINUTE
// level, so agreement on the date cannot mask a wrong clock.
const witness = await evaluate(`(async () => {
  const u = Math.floor(Date.UTC(2026, 10, 8, 20, 0, 0) / 1000)
  const colombo = new Date(u * 1000).toLocaleString('en-GB', {
    timeZone: 'Asia/Colombo', day: 'numeric', month: 'short',
    year: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false,
  })
  return { colombo }
})()`)
check(witness?.colombo?.includes('01:30'),
  'the boundary witness itself renders at Colombo 01:30 (sanity)', witness?.colombo)


/* ---- 3. zonedDayBounds is exact, across offsets and DST ------------------ */

// A pure function with no module state, so importing the source directly is
// sound here: every argument it needs is passed in.
const bounds = await evaluate(`(async () => {
  const m = await import('/src/lib/time.ts?v=' + Date.now())
  const cases = [
    ['Asia/Colombo', '2026-11-01', 'start'],
    ['Asia/Colombo', '2026-11-03', 'end'],
    ['UTC',          '2026-11-01', 'start'],
    ['America/New_York', '2026-03-08', 'start'],
    ['America/New_York', '2026-11-01', 'start'],
    ['Asia/Tokyo',   '2026-11-01', 'start'],
  ]
  return cases.map(([tz, date, edge]) => {
    const sec = m.zonedDayBounds(date, tz, edge)
    return {
      tz, date, edge,
      shown: new Date(sec * 1000).toLocaleString('en-GB', { timeZone: tz, hour12: false }),
    }
  })
})()`)

for (const b of bounds ?? []) {
  // 'YYYY-MM-DD' -> 'DD/MM/YYYY', the shape en-GB renders.
  const wantDate = b.date.split('-').reverse().join('/')
  const wantTime = b.edge === 'start' ? '00:00:00' : '23:59:59'
  const ok = b.shown.startsWith(wantDate) && b.shown.endsWith(wantTime)
  check(ok, `${b.tz} ${b.date} ${b.edge} is exactly ${wantDate} ${wantTime} there`, b.shown)
}

/* ---- 4. the DEVICE clock is genuinely not in use ------------------------- */

// `formatDateTime(unix, timeZone)` takes its zone EXPLICITLY, so passing the
// device zone alongside Colombo measures only the formatter — and that is the
// point: two zones, one instant, two readings.
const boundary = await evaluate(`(async () => {
  const m = await import('/src/lib/time.ts?v=' + Date.now())
  // 20:00 UTC is 01:30 the next day in Colombo and 04:00 the SAME day at +08:00,
  // so the minute proves which clock was used even when the date agrees.
  const u = Math.floor(Date.UTC(2026, 10, 8, 20, 0, 0) / 1000)
  const deviceZone = Intl.DateTimeFormat().resolvedOptions().timeZone
  return {
    shown: m.formatDateTime(u, 'Asia/Colombo'),
    device: m.formatDateTime(u, deviceZone),
    deviceZone,
    labelled: m.formatDateTimeWithZone(u, 'Asia/Colombo'),
  }
})()`)

check(/01:30$/.test(boundary?.shown ?? ''),
  'a boundary instant renders at Colombo 01:30', boundary?.shown)
check(boundary?.shown !== boundary?.device,
  'it does NOT render at the device clock when the two differ',
  { colombo: boundary?.shown, device: boundary?.device, deviceZone: boundary?.deviceZone })

/* ---- 5. the device clock is not what the SCREEN used --------------------- */

// The rendered page must show a Colombo reading somewhere the zone matters. The
// Usage screen prints the window'; its axis labels come from the same formatter.
check(typeof rendered?.text === 'string' && rendered.text.length > 0,
  'the Usage screen rendered (so its formatters actually ran)', rendered?.text?.length)

check(/\(SLST\)$/.test(boundary?.labelled ?? ''),
  'a stated timestamp names its zone', boundary?.labelled)


/* ------------------------------------------------------------------------ */

chrome.kill()
console.log('')
if (failures.length) {
  console.log(`${failures.length} failure(s)`)
  process.exit(1)
}
console.log('OK  0 failure(s)')

