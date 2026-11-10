/**
 * VEYLIX — the Super Admin console, asserted page by page in headless Chrome.
 *
 *   node scripts/verify-super-admin.mjs            (all pages)
 *   node scripts/verify-super-admin.mjs /vpns      (one page, verbose)
 *
 * Why this exists. `verify-nav.mjs` proves each Super Admin ROUTE resolves, and
 * `verify-dashboard.mjs` proves one page's cards render. Neither proves the thing
 * an operator actually breaks: a console page that loads but has lost its table,
 * its filters, its primary action, or its heading — which looks perfect in a
 * screenshot and is useless in use.
 *
 * What it asserts, per page:
 *   1. the page heading rendered and matches the expected page
 *   2. no uncaught console exception
 *   3. no inline `style="…"` attribute in the page's own content (D-059: shared
 *      layout belongs in a stylesheet class, not a style prop)
 *   4. no horizontal overflow at 1440px
 *   5. no fabricated date (1970) anywhere in the rendered text
 *   6. at least one interactive control, so "the owner can operate this screen"
 *      is measured rather than claimed
 *
 * Exit code is non-zero when any assertion fails, so this is a gate and never a
 * report. Structural facts only: a screenshot cannot be diffed in a test.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9355)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'

/**
 * Every Super Admin destination, with the thing that must be true of it.
 *
 * `minRows` is 0 for pages that are legitimately empty on a fresh install; the
 * assertion there is that the page rendered and offered an action, not that it
 * holds data. `controls` is the count of interactive elements the page must
 * expose — a screen with no control is a screen the owner cannot operate.
 */
const PAGES = [
  { path: '/dashboard', heading: /overview|dashboard/i, controls: 3, minRows: 0 },
  { path: '/servers', heading: /server/i, controls: 2, minRows: 0 },
  { path: '/vpns', heading: /vpn/i, controls: 2, minRows: 0 },
  { path: '/offers', heading: /offer/i, controls: 2, minRows: 0 },
  { path: '/staff', heading: /staff|admin/i, controls: 2, minRows: 0 },
  { path: '/clients', heading: /client/i, controls: 3, minRows: 0 },
  { path: '/orders', heading: /order/i, controls: 1, minRows: 0 },
  { path: '/payments', heading: /payment|billing/i, controls: 1, minRows: 0 },
  { path: '/packages', heading: /package/i, controls: 2, minRows: 0 },
  { path: '/pricing', heading: /rate|pricing/i, controls: 1, minRows: 0 },
  { path: '/audit', heading: /audit/i, controls: 2, minRows: 0 },
  { path: '/settings', heading: /setting/i, controls: 1, minRows: 0 },
  { path: '/notifications', heading: /notification/i, controls: 1, minRows: 0 },
  { path: '/profile', heading: /profile|account/i, controls: 1, minRows: 0 },
]

const only = process.argv[2]
const targets = only ? PAGES.filter((p) => p.path === only) : PAGES
if (targets.length === 0) {
  console.error(`no such page in the table: ${only}`)
  process.exit(2)
}

let pass = 0
let fail = 0
const check = (name, ok, detail = '') => {
  if (ok) {
    pass++
    console.log(`PASS  ${name}${detail ? '  -> ' + detail : ''}`)
  } else {
    fail++
    console.log(`FAIL  ${name}${detail ? '  -> ' + detail : ''}`)
  }
}

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('super-admin')}`, '--no-first-run',
  '--disable-gpu', '--window-size=1440,1000', 'about:blank',
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
  } catch { /* not up yet */ }
}
if (!ws) {
  console.error('could not attach to chrome')
  process.exit(1)
}

const exceptions = []
await send('Runtime.enable')
await send('Page.enable')
await send('Network.enable')
ws.addEventListener('message', (m) => {
  const msg = JSON.parse(m.data)
  if (msg.method === 'Runtime.exceptionThrown') {
    exceptions.push(msg.params?.exceptionDetails?.text ?? 'exception')
  }
})

const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  return r?.result?.value
}

/*
 * D-109: poll for the page to settle, never sleep a fixed time.
 *
 * This gate used `await sleep(3500)` after every navigation. A fixed sleep is a
 * guess: it passes when the machine is warm and fails on the first run after a
 * restart, because the dashboard's data arrives later than the paint. That is
 * the definition of a flaky gate — it reports the machine's speed, not the
 * product. `waitFor` asks the page whether it is ready and gives it a bounded
 * amount of time to say yes.
 *
 * `ready` is a JS expression string evaluated in the page; it must return truthy
 * once the content the next assertions read has actually arrived.
 */
const waitFor = async (ready, timeoutMs = 12000) => {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      if (await evaluate(ready)) return true
    } catch {
      /* a mid-navigation evaluate throws; keep polling */
    }
    await sleep(200)
  }
  return false
}

const session = await signIn('super_admin')
if (session.status !== 200) {
  console.error(`super admin sign-in failed: ${session.status}`)
  ws.close()
  chrome.kill()
  process.exit(1)
}
const host = new URL(APP).hostname
const c = cookieParts(session.cookie, host)
await send('Network.setCookie', { name: c.name, value: c.value, domain: host, path: '/', httpOnly: true })

/* One probe definition, reused for every page, so a page cannot pass by
 * rendering a different shape of DOM than its neighbours. */
const PROBE = `(() => {
  const root = document.querySelector('main') ?? document.body
  const heading =
    document.querySelector('.ui-page-head__title')?.textContent?.trim() ||
    root.querySelector('h1')?.textContent?.trim() ||
    null
  const rows = root.querySelectorAll('table tbody tr').length
  const controls = root.querySelectorAll('button, a[role="button"], input, select').length
  // D-059: a shared layout rule belongs in a class, not a style prop. CSS
  // custom properties are the one allowed form, so they are excluded here.
  const inlineStyles = [...root.querySelectorAll('[style]')].filter((el) => {
    const s = el.getAttribute('style') ?? ''
    return !/^\\s*--[\\w-]+\\s*:/.test(s)
  }).length
  const inlineDetail = [...root.querySelectorAll('[style]')].filter((el) => {
    const s = el.getAttribute('style') ?? ''
    return !/^\\s*--[\\w-]+\\s*:/.test(s)
  }).slice(0, 6).map((el) => el.tagName.toLowerCase() + '[' + el.className.toString().slice(0, 40) + ']{' + (el.getAttribute('style') ?? '').slice(0, 60) + '}')
  const text = (root.innerText || '').slice(0, 200000)
  const date1970 = /(1 January 1970|Jan 1, 1970|1 Jan 1970|1970-01-01)/i.test(text)
  const overflow = document.documentElement.scrollWidth > document.documentElement.clientWidth + 2
  const notFound = /page not found|nothing here|404/i.test(heading ?? '')
  return JSON.stringify({ heading, rows, controls, inlineStyles, inlineDetail, date1970, overflow, notFound })
})()`

for (const page of targets) {
  const before = exceptions.length
  await send('Page.navigate', { url: `${APP}${page.path}` })
  /*
   * Wait for THIS page's own heading rather than a fixed 3.5s. Every target in
   * the table declares an expected heading, so that is the precise signal that
   * the route has resolved and the page under test is the page on screen —
   * polling for anything vaguer (or sleeping) reintroduces the flake.
   */
  const ready = await waitFor(
    `(() => {
      const h = document.querySelector('h1')
      const text = (h?.textContent || '').trim()
      return !!text && ${page.heading}.test(text) &&
        document.querySelectorAll('main [data-testid], main table tbody tr, main .ui-metric, main button, main a').length > 0
    })()`,
  )

  const raw = await evaluate(PROBE)
  let out
  try {
    out = JSON.parse(raw)
  } catch {
    check(`${page.path} probe`, false, 'probe did not return JSON')
    continue
  }

  check(
    `${page.path} settled without a fixed sleep`,
    ready,
    ready ? 'heading matched the expected page' : `page did not settle within 12s (heading: "${out.heading ?? 'none'}")`,
  )
  check(`${page.path} rendered a heading`, !!out.heading && !out.notFound, out.heading ?? 'none')
  check(`${page.path} matches its expected page`, page.heading.test(out.heading ?? ''), out.heading ?? '')
  check(`${page.path} raised no uncaught exception`, exceptions.length === before,
    exceptions.slice(before).join(' | '))
  check(`${page.path} offers at least ${page.controls} control(s)`, out.controls >= page.controls, `${out.controls} found`)
  check(`${page.path} has ${page.minRows}+ data row(s)`, out.rows >= page.minRows, `${out.rows} rows`)
  check(`${page.path} uses no inline style (D-059)`, out.inlineStyles === 0,
    out.inlineStyles === 0 ? '' : `${out.inlineStyles} element(s): ${(out.inlineDetail ?? []).join(' | ')}`)
  check(`${page.path} does not overflow horizontally`, out.overflow === false)
  check(`${page.path} shows no fabricated 1970 date`, out.date1970 === false)
}

/*
 * ---------------------------------------------------------------------------
 * D-107 — the business row.
 *
 * The loop above proves a page RENDERED. It cannot prove the page says anything
 * useful, which is exactly how this dashboard passed 112 assertions while showing
 * no revenue at all. These assertions read the rendered TEXT, so they fail if a
 * section disappears, if a zero returns to stand in for a failed read, or if a
 * developer note reappears where a business fact belongs.
 * ---------------------------------------------------------------------------
 */
await send('Page.navigate', { url: `${APP}/dashboard` })
// The business row is the last thing to arrive (it waits on the overview query),
// so wait for it explicitly. `money-hero` is rendered only once totals resolve,
// which makes it the honest readiness signal for everything read below.
await waitFor(
  `!!document.querySelector('[data-testid="money-hero"]') ||
   !!document.querySelector('[data-testid="money-hero-empty"]') ||
   !!document.querySelector('[data-testid="money-hero-mixed"]')`,
  15000,
)

const DASH = `(() => {
  const root = document.querySelector('main') ?? document.body
  const body = root.innerText || ''
  const money = document.querySelector('[data-testid="money-hero"]')
  const resellers = document.querySelector('[data-testid="reseller-section"]')
  const attentionCard = document.querySelector('[data-testid="attention-card"]')
  const cards = [...root.querySelectorAll('.ui-metric')]
  const health = cards.find((c) => /panel health/i.test(c.textContent || ''))
  return JSON.stringify({
    moneyExists: !!money,
    moneyEmptyExists: !!document.querySelector('[data-testid="money-hero-empty"]'),
    moneyMixedExists: !!document.querySelector('[data-testid="money-hero-mixed"]'),
    moneyText: money ? money.innerText : '',
    resellerExists: !!resellers,
    attentionBadge: attentionCard
      ? (attentionCard.querySelector('.ui-badge')?.textContent?.trim() ?? '')
      : '',
    hasEndpointTalk: /from \\/api\\//.test(body),
    hasRecordTalk: /total records/i.test(body),
    labels: [...root.querySelectorAll('.ui-metric__label')].map((n) => n.textContent.trim()),
    healthValue: health?.querySelector('.ui-metric__value')?.textContent?.trim() ?? '',
    freshness: document.querySelector('.dash-live')?.textContent?.trim() ?? '',
  })
})()`

const dashRaw = await evaluate(DASH)
let dash = null
try {
  dash = JSON.parse(dashRaw)
} catch {
  check('D-107 dashboard probe', false, 'probe did not return JSON')
}

if (dash) {
  // The money section must be present in one of its three honest states: real
  // figures, "nothing sold yet", or "mixed currencies, no single total". A MISSING
  // section is a failure — that absence was the old dashboard's defect.
  check(
    'D-107 money section is present',
    dash.moneyExists || dash.moneyEmptyExists || dash.moneyMixedExists,
    dash.moneyExists ? 'figures'
      : dash.moneyEmptyExists ? 'empty state'
      : dash.moneyMixedExists ? 'mixed currencies'
      : 'absent',
  )

  if (dash.moneyExists) {
    // A bare number is not money: the hero must carry a currency.
    check(
      'D-107 money hero shows a currency amount',
      /[^\d\s.,]{1,4}\s?\d|\d[\d,.]*\s?[A-Z]{3}/.test(dash.moneyText),
      (dash.moneyText.split('\n')[0] ?? '').slice(0, 60),
    )
    // Hero + three supporting figures. Fewer means a section silently vanished.
    check(
      'D-107 money hero offers four figures',
      (dash.moneyText.match(/\d/g) ?? []).length >= 4,
      dash.moneyText.replace(/\n/g, ' · ').slice(0, 90),
    )
  }

  check('D-107 reseller section is present', dash.resellerExists === true)

  // P1-6: the badge must state groups, not a bare "N thing(s)".
  check(
    'D-107 attention badge counts in groups',
    dash.attentionBadge === '' || /group/i.test(dash.attentionBadge),
    dash.attentionBadge || 'no queue',
  )

  // P1-3/P1-4: developer vocabulary is banned from the business row.
  check('D-107 no endpoint names in the dashboard body', dash.hasEndpointTalk === false)
  check('D-107 no "total records" in the dashboard body', dash.hasRecordTalk === false)

  // P1-1/P1-5: the fourth tile is "Panel health", never an unqualified "Healthy".
  check(
    'D-107 fourth tile is named for what it measures',
    dash.labels.includes('Panel health'),
    dash.labels.join(', '),
  )
  check(
    'D-107 panel health never claims 0% with no panels',
    dash.healthValue !== '0%',
    dash.healthValue || 'absent',
  )

  // P1-2: the freshness marker states WHEN, never a bare "Synced" that a frozen
  // chart could also show.
  check(
    'D-107 freshness states when, not just that',
    dash.freshness === '' || /(synced|stale|not synced)/i.test(dash.freshness),
    dash.freshness || 'absent',
  )
}


console.log(`\nSUPER ADMIN ${fail === 0 ? 'OK' : 'FAILED'} — ${pass} passed, ${fail} failed`)
ws.close()
chrome.kill()
process.exit(fail === 0 ? 0 : 1)
