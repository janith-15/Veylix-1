/**
 * VEYLIX — dashboard smoke in a headless browser (`node scripts/verify-dashboard.mjs`).
 *
 * The unit-less frontend has no other way to prove a page actually renders, and
 * the 2026-10-24 rewrite of DashboardPage changed where every number comes from.
 * This navigates to the real dev server, waits for the data to settle and then
 * reports: what the subtitle says, every metric card's label/value/delta, whether
 * a chart or an empty-state message rendered, and the server table's first row.
 *
 * It also fails on any console error / uncaught exception, because a page that
 * throws after paint looks perfectly healthy in a screenshot.
 *
 * Requires: `.\dev.ps1` running (backend :8080 + Vite :5173).
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME =
  process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9335)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const PATH = process.argv[2] ?? '/dashboard'

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    '--user-data-dir=' + profileDir('dashboard-check'),
    '--no-first-run',
    '--disable-gpu',
    '--window-size=1440,1000',
    'about:blank',
  ],
  { stdio: 'ignore' },
)

async function pageTarget() {
  for (let i = 0; i < 40; i++) {
    try {
      const res = await fetch(`http://127.0.0.1:${PORT}/json/list`)
      const list = await res.json()
      const p = list.find((t) => t.type === 'page')
      if (p) return p
    } catch {
      /* retry */
    }
    await sleep(250)
  }
  throw new Error('no CDP page — is Chrome installed at ' + CHROME + '?')
}

const t = await pageTarget()
const ws = new WebSocket(t.webSocketDebuggerUrl)
await new Promise((r) => ws.addEventListener('open', r, { once: true }))

let id = 0
const pending = new Map()
const errors = []
ws.addEventListener('message', (ev) => {
  const m = JSON.parse(ev.data)
  if (m.id && pending.has(m.id)) {
    pending.get(m.id)(m.result ?? m.error)
    pending.delete(m.id)
  }
  if (m.method === 'Runtime.exceptionThrown') {
    errors.push('EXC: ' + (m.params.exceptionDetails?.exception?.description ?? '?'))
  }
  if (m.method === 'Runtime.consoleAPICalled' && m.params.type === 'error') {
    const text = (m.params.args ?? []).map((a) => a.value ?? a.description ?? '').join(' ')
    // React logs the component stack as an extra console ARGUMENT, not as a JS
    // call stack. Keep a compact slice of it — that is what names the offender.
    const stackArg = (m.params.args ?? [])
      .map((a) => a.value ?? a.description ?? '')
      .filter((s) => typeof s === 'string' && s.includes('at ') && s.includes('<'))
      .join(' ')
    const compact = stackArg
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l.startsWith('at ') || l.startsWith('<'))
      .slice(0, 8)
      .join(' | ')
    errors.push('CONSOLE: ' + text + (compact ? `\n      stack: ${compact}` : ''))
  }
})
const send = (method, params = {}) =>
  new Promise((res) => {
    const i = ++id
    pending.set(i, res)
    ws.send(JSON.stringify({ id: i, method, params }))
  })

// `Runtime.consoleAPICalled` is captured above; `Log.enable` additionally gives
// the browser's own log entries, and a duplicate-key warning whose React object
// arg is not a string still arrives there with a readable description.
await send('Log.enable')
ws.addEventListener('message', (ev) => {
  const m = JSON.parse(ev.data)
  if (m.method === 'Log.entryAdded' && m.params.entry.level === 'error') {
    errors.push('LOG: ' + m.params.entry.text.slice(0, 400))
  }
})

await send('Runtime.enable')
await send('Page.enable')

// React's duplicate-key warning is only actionable with its component stack, and
// that arrives as a second console argument that CDP reports as an object. Wrap
// console.error before the app boots so every warning keeps its stack on
// `window.__veylixConsole`.
await send('Runtime.evaluate', {
  expression: `(() => {
    window.__veylixConsole = [];
    const original = console.error;
    console.error = function (...args) {
      try {
        window.__veylixConsole.push(
          args.map((a) => (typeof a === 'string' ? a : String(a && a.message ? a.message : a))).join(' '),
        );
      } catch { /* ignore */ }
      return original.apply(this, args);
    };
  })()`,
})

// Sign in FOR REAL before navigating (D-039).
//
// This used to seed a dev-login key in localStorage, which no longer exists. The
// script therefore logs in through `scripts/auth-login.mjs` (POST /api/auth/login
// with the seeded account for VEYLIX_DEV_ROLE) and installs the resulting session
// cookie in the browser, so what is measured is the same thing a person sees.
const role = process.env.VEYLIX_DEV_ROLE ?? 'super-admin'
{
  const session = await signIn(role)
  if (session.status !== 200) {
    console.error(`login as ${role} failed: HTTP ${session.status} (seed the accounts first)`)
    process.exit(1)
  }
  const { name, value } = cookieParts(session.cookie, new URL(APP).hostname)
  await send('Network.enable', {})
  await send('Network.setCookie', { name, value, domain: new URL(APP).hostname, path: '/', httpOnly: true })
}

await send('Page.navigate', { url: APP + PATH })
// The pages fire several API calls; the slowest is one panel round-trip. Wait
// for the app shell AND for the "Loading…" state to clear instead of guessing a
// fixed delay — probing too early is how a passing page looks broken.
const deadline = Date.now() + 20000
let settled = false
while (Date.now() < deadline && !settled) {
  await sleep(500)
  const check = await send('Runtime.evaluate', {
    expression: `(() => {
      const body = document.body.innerText || ''
      return JSON.stringify({
        hasShell: !!document.querySelector('.app-shell') || !!document.querySelector('.ui-page-head'),
        // Two loading signals, because the app uses both: literal "Loading…" text
        // and SKELETON placeholders, which carry no text at all. Waiting only for
        // the text let this probe settle while a panel round-trip was still in
        // flight, so a page that was about to render four rows was measured while
        // it still showed placeholders (and was reported as empty).
        loading: /Loading|Refreshing/.test(body) ||
          document.querySelectorAll('.ui-skeleton, [class*="skeleton"], .ui-spinner').length > 0,
      })
    })()`,
    returnByValue: true,
  })
  try {
    const s = JSON.parse(check.result?.value ?? '{}')
    settled = s.hasShell && !s.loading
  } catch {
    /* keep waiting */
  }
}
await sleep(800)

const probe = await send('Runtime.evaluate', {
  expression: `JSON.stringify({
    href: location.href,
    title: (document.querySelector('.ui-page-head__title') || document.querySelector('h1'))?.textContent?.trim() ?? null,
    subtitle: document.querySelector('.ui-page-head__subtitle')?.textContent?.trim() ?? null,
    metrics: [...document.querySelectorAll('.ui-metric')].map((el) => ({
      label: el.querySelector('.ui-metric__label')?.textContent?.trim() ?? '',
      value: el.querySelector('.ui-metric__value')?.textContent?.trim() ?? '',
      delta: el.querySelector('.ui-metric__delta')?.textContent?.trim() ?? '',
    })),
    chartMarks: document.querySelectorAll('svg.ui-chart polyline, svg.ui-chart path, svg.ui-chart line').length,
    notes: [...document.querySelectorAll('.ui-muted')]
      .map((e) => e.textContent.replace(/\\s+/g, ' ').trim())
      .filter((x) => x.length > 40)
      .slice(0, 4),
    tableRows: [...document.querySelectorAll('table tbody tr')]
      .map((tr) => tr.textContent.replace(/\\s+/g, ' ').trim())
      .slice(0, 3),
    cardTitles: [...document.querySelectorAll('.ui-card__title')].map((e) => e.textContent.trim()),
    // Named controls worth asserting exist: the harness only extracts text over
    // 40 characters, so a two-word button is invisible to the notes list. Each
    // key is a selector that is absent on screens without the control.
    controls: {
      savedViews: document.querySelectorAll('[aria-label="Saved views"]').length,
      commandBar: document.querySelectorAll('[aria-label="VPN catalogue at a glance"]').length,
      selectionBar: document.querySelectorAll('[aria-label="Bulk actions"]').length,
      tagPicker: document.querySelectorAll('.cl-tagpick').length,
      orderTimeline: document.querySelectorAll('[data-testid^="order-timeline-"]').length,
    },
    emptyStatesWithAction: [...document.querySelectorAll('[data-testid="empty-state"]')]
      .filter((el) => el.querySelector('button, a')).length,
    emptyStates: document.querySelectorAll('[data-testid="empty-state"]').length,
    // What the page actually asked for, and what it rendered from the answer.
    // A table that is empty while the API returns rows is the difference between
    // a broken query and a broken projection, and this is the field that says which.
    probes: {
      skeletonVisible: document.querySelectorAll('.ui-skeleton, [class*="skeleton"]').length,
      tableRows: document.querySelectorAll('table tbody tr').length,
      cardCount: document.querySelectorAll('.ui-card').length,
    },
  })`,
  returnByValue: true,
})

// A failed evaluate must still produce a report — silence is how a broken probe
// gets mistaken for a passing page.
if (!probe.result?.value) {
  console.error('PROBE FAILED:', JSON.stringify(probe).slice(0, 400))
}
const result = JSON.parse(probe.result?.value ?? '{"error":"probe returned no value"}')
console.log(JSON.stringify({ path: PATH, ...result, consoleErrors: errors }, null, 2))

ws.close()
chrome.kill()
// Chrome can take a moment to release the profile dir; wait for the process to
// go away so a following run does not attach to a half-dead instance.
await new Promise((r) => {
  const timer = setTimeout(r, 3000)
  chrome.once('exit', () => {
    clearTimeout(timer)
    r()
  })
})

// A redirect can mean a role guard fired (correct) or a broken page. Printing
// the final URL makes the difference visible instead of silently passing.
const landedElsewhere = !result.href?.endsWith(PATH)
if (landedElsewhere) {
  console.error(`\nNOTE: ${PATH} redirected to ${result.href} — role guard or role landing.`)
}
process.exit(errors.length === 0 ? 0 : 1)
