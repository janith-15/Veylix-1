/**
 * VEYLIX — audit every page, per role, in one pass.
 *
 *   node scripts/audit-ui.mjs          # table
 *   node scripts/audit-ui.mjs --json   # machine-readable
 *
 * Each verify-*.mjs proves one behaviour on one page. Nobody has looked at the whole
 * app at once, so a page that throws on load, redirects somewhere useless, or leaks a
 * console error can sit broken for weeks. This reports facts, not impressions.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { assertPortFree, profileDirUnique, waitForExit } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9431)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const AS_JSON = process.argv.includes('--json')

const PAGES = [
  { path: '/login', role: null },
  { path: '/staff/login', role: null },
  { path: '/register', role: null },
  { path: '/forgot', role: null },
  { path: '/reset', role: null },
  { path: '/dashboard', role: 'super_admin' },
  { path: '/servers', role: 'super_admin' },
  { path: '/offers', role: 'super_admin' },
  // `/admins` is a redirect to `/staff` (D-057: one admin console, not two), so
  // probing `/admins` only ever measured how fast a redirect happens.
  { path: '/staff', role: 'super_admin' },
  { path: '/clients', role: 'super_admin' },
  { path: '/clients/3', role: 'super_admin' },
  { path: '/vpns', role: 'super_admin' },
  { path: '/audit', role: 'super_admin' },
  { path: '/payments', role: 'super_admin' },
  { path: '/notifications', role: 'super_admin' },
  { path: '/profile', role: 'super_admin' },
  // `/chat` auto-opens the first conversation and mirrors it into the URL so the
  // conversation is shareable and Back works — a deliberate redirect, so the
  // expectation is a prefix, not an exact match.
  { path: '/chat', role: 'super_admin', landedPrefix: '/chat' },
  { path: '/admin', role: 'admin' },
  { path: '/admin-servers', role: 'admin' },
  { path: '/admin-clients', role: 'admin' },
  { path: '/sale', role: 'admin' },
  { path: '/retail-price', role: 'admin' },
  { path: '/payments', role: 'admin' },
  { path: '/client', role: 'client' },
  { path: '/buy', role: 'client' },
  { path: '/history', role: 'client' },
  { path: '/vpn/24', role: 'client' },
  { path: '/profile', role: 'client' },
]

// A browser this run did not start would answer with a page it never loaded.
await assertPortFree(PORT)

const chrome = spawn(CHROME, [
  '--headless=new',
  `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDirUnique('audit')}`,
  '--no-first-run',
  '--disable-gpu',
  '--window-size=1440,1100',
  'about:blank',
], { stdio: 'ignore' })

let ws
let seq = 0
const pending = new Map()

const send = (method, params = {}) =>
  new Promise((res, rej) => {
    const id = ++seq
    pending.set(id, { res, rej })
    ws.send(JSON.stringify({ id, method, params }))
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

let consoleErrors = []
await send('Runtime.enable')
await send('Page.enable')
await send('Network.enable')
// `pointer: coarse` decides whether the touch-target rules (touch.css) apply. The
// audit must be able to ask for a finger, or it would report desktop density and
// "pass" while a phone user still gets 36px buttons.
const TOUCH = process.env.VEYLIX_TOUCH === '1'
if (TOUCH) {
  await send('Emulation.setDeviceMetricsOverride', {
    width: 390, height: 844, deviceScaleFactor: 3, mobile: true,
  })
  await send('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 5 })
}

ws.addEventListener('message', (m) => {
  const msg = JSON.parse(m.data)
  if (msg.method === 'Runtime.exceptionThrown') {
    const d = msg.params?.exceptionDetails?.exception?.description ?? ''
    consoleErrors.push('EXCEPTION ' + d.slice(0, 120))
  }
  if (msg.method === 'Runtime.consoleAPICalled' && msg.params?.type === 'error') {
    const args = msg.params.args ?? []
    const text = args.map((a) => a.value ?? a.description ?? '').join(' ')
    consoleErrors.push('CONSOLE ' + text.slice(0, 120))
  }
})

const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', {
    expression,
    awaitPromise: true,
    returnByValue: true,
  })
  return r?.result?.value
}

/**
 * Wait until the page has rendered something meaningful.
 *
 * Ready = a heading, an empty/error state, or a table. Bounded, so a genuinely
 * blank screen still gets reported (after the full timeout) instead of hanging.
 */
const waitForSettled = async (timeoutMs) => {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const ready = await evaluate(
      `!!document.querySelector('h1, h2, [data-testid="empty-state"]')`,
    )
    if (ready) return true
    await sleep(150)
  }
  return false
}

const sessions = {}
for (const role of ['super_admin', 'admin', 'client']) {
  const s = await signIn(role)
  if (s.status !== 200) {
    console.error(`${role} sign-in failed: ${s.status}`)
    process.exit(1)
  }
  sessions[role] = cookieParts(s.cookie)
}

async function signInAs(role) {
  await send('Network.clearBrowserCookies')
  if (!role) return
  const c = sessions[role]
  await send('Network.setCookie', {
    name: c.name,
    value: c.value,
    domain: c.domain,
    path: '/',
    httpOnly: true,
  })
}

/*
 * Runs inside the page. Written as a plain function and serialised, so it cannot be
 * broken by template-literal escaping — which is exactly how the first version of
 * this file became unparsable.
 */
function inspectPage() {
  const h = document.querySelector('h1, h2')
  const bodyText = document.body.innerText || ''
  const all = [...document.querySelectorAll('button, a, input, select, textarea')]
  const visible = all.filter((el) => {
    const r = el.getBoundingClientRect()
    if (r.width === 0 || r.height === 0) return false
    // A closed off-canvas drawer keeps its geometry (only translateX moves it), so
    // an on-screen test is the only honest filter for "a control the user can reach".
    if (r.right <= 0 || r.left >= window.innerWidth) return false
    if (r.bottom <= 0 || r.top >= window.innerHeight) return false
    return el.checkVisibility ? el.checkVisibility() : true
  })
  // WCAG 2.5.8 (AA) is the minimum this product must meet: 24x24 CSS px.
  let small = 0
  // WCAG 2.5.5 (AAA) is 44x44. Reported for information, never as a defect:
  // calling every dense console control "broken" is how a real finding gets lost.
  let belowAaa = 0
  const smallSample = []
  for (const el of visible) {
    const r = el.getBoundingClientRect()
    if (r.height < 44 || r.width < 44) belowAaa++
    if (r.height < 24 || r.width < 24) {
      small++
      if (smallSample.length < 5) {
        smallSample.push(
          el.tagName.toLowerCase() +
            '["' +
            (el.textContent || el.getAttribute('aria-label') || '').trim().slice(0, 18) +
            '"] ' +
            Math.round(r.width) +
            'x' +
            Math.round(r.height),
        )
      }
    }
  }
  const deadEnd = /something went wrong|unexpected error|cannot read|is not a function/i.test(
    bodyText,
  )
  return {
    landed: location.pathname,
    heading: h ? h.textContent.trim().slice(0, 46) : '',
    textLen: bodyText.length,
    targets: visible.length,
    small,
    belowAaa,
    smallSample,
    deadEnd,
  }
}

const rows = []
let currentRole = 'unset'

for (const page of PAGES) {
  if (page.role !== currentRole) {
    await signInAs(page.role)
    currentRole = page.role
  }

  consoleErrors = []
  await send('Page.navigate', { url: `${APP}${page.path}` })
  // Wait for the page to actually render rather than trusting a fixed sleep. A
  // cold profile has no HTTP cache, so a lazy route chunk can take several
  // seconds; a short sleep then reports "no heading" and a tiny text length for
  // a screen that was merely still loading (a probe must not invent a defect).
  await waitForSettled(12000)

  const dom = (await evaluate(`(${inspectPage.toString()})()`)) ?? {}

  const errs = consoleErrors.filter((e) => !/favicon/i.test(e))
  const landed = dom.landed ?? '?'
  // A page may declare that it is expected to redirect (e.g. /chat → /chat/3).
  const ok = page.landedPrefix ? landed.startsWith(page.landedPrefix) : landed === page.path
  rows.push({
    role: page.role ?? 'public',
    path: page.path,
    landed,
    ok,
    heading: dom.heading ?? '',
    text: dom.textLen ?? 0,
    small: dom.small ?? 0,
    belowAaa: dom.belowAaa ?? 0,
    smallSample: dom.smallSample ?? [],
    targets: dom.targets ?? 0,
    deadEnd: dom.deadEnd ?? false,
    errors: errs.length,
    firstError: errs[0] ?? '',
  })
}

if (AS_JSON) {
  console.log(JSON.stringify({ pages: rows }, null, 2))
} else {
  const pad = (s, n) => String(s).padEnd(n)
  const head =
    pad('role', 12) +
    pad('path', 17) +
    pad('landed', 17) +
    pad('heading', 30) +
    pad('txt', 6) +
    pad('tap', 8) +
    pad('aaa', 8) +
    pad('err', 5) +
    'status'
  console.log(head)
  console.log('-'.repeat(126))

  for (const r of rows) {
    let status = 'ok'
    if (!r.ok) status = 'REDIRECT'
    else if (r.errors > 0) status = 'CONSOLE'
    else if (r.deadEnd) status = 'BROKEN'
    // tap = controls meeting WCAG 2.5.8 AA (24px); aaa = meeting 2.5.5 AAA (44px).
    const tap = String(r.targets - r.small) + '/' + String(r.targets)
    const aaa = String(r.targets - r.belowAaa) + '/' + String(r.targets)
    console.log(
      pad(r.role, 12) +
        pad(r.path, 17) +
        pad(r.landed, 17) +
        pad(r.heading || '(none)', 30) +
        pad(r.text, 6) +
        pad(tap, 8) +
        pad(aaa, 8) +
        pad(r.errors, 5) +
        status,
    )
  }

  const bad = rows.filter((r) => !r.ok || r.errors > 0 || r.deadEnd)
  const good = rows.filter((r) => r.ok).length
  console.log('')
  console.log(`${rows.length} pages · ${good} landed as expected · ${bad.length} problem(s)`)
  for (const r of bad) {
    let why = 'dead-end text on screen'
    if (!r.ok) why = 'redirected to ' + r.landed
    else if (r.errors > 0) why = String(r.errors) + ' console error(s): ' + r.firstError
    console.log('  ' + r.path + ' (' + r.role + '): ' + why)
  }

  // Target size is reported separately, because it is an accessibility finding
  // and not a broken page. AA (24px) is the bar; AAA (44px) is information.
  const aaBad = rows.filter((r) => r.small > 0)
  const aaaBad = rows.filter((r) => r.belowAaa > 0)
  console.log('')
  console.log(`WCAG 2.5.8 AA (24px) shortfall: ${aaBad.length} page(s)`)
  for (const r of aaBad) {
    console.log(
      '  ' + r.path + ': ' + r.small + '/' + r.targets + ' under 24px — ' + r.smallSample.join(', '),
    )
  }
  console.log(`WCAG 2.5.5 AAA (44px) shortfall: ${aaaBad.length} page(s) — not a defect, just not AAA`)
}

ws.close()
await waitForExit(chrome)
process.exit(0)



