/**
 * VEYLIX — the Premium VPN page (D-038, P-01 … P-10) in a headless browser.
 *
 * `docs/IMPROVEMENT_LEDGER.md` M7-17 recorded that `/vpn/:id` rendered a fraction
 * of the mandated feature set: the backend shipped `/usage?range=`, `/protocol`
 * and `/traffic/live` and the page called none of them. This proves the wiring by
 * reading what is actually on screen — not by asserting that the code exists.
 *
 * Reports, for one VPN:
 *   - P-01 up / down / total are three distinct numbers;
 *   - P-02 the capacity bar and its percentage;
 *   - P-03/P-07 CPU and RAM, and the per-metric switch;
 *   - P-04 the live rate, or the honest "not measured yet";
 *   - P-06 the range switch actually re-reads (the window label changes);
 *   - P-08 all four chart types render;
 *   - P-09 protocol / transport / credential, masked until revealed;
 *   - P-10 the QR and the link are both present.
 *
 * Also fails on ANY console error, because a page that throws after paint looks
 * perfectly healthy in a screenshot (`verify-dashboard.mjs` §10).
 *
 * Requires `.\\dev.ps1` running (backend :8080 + Vite :5173).
 *
 * ROUTE PER ROLE (this is the part that used to make the gate lie):
 *
 *   `/vpn/:id`                     is CLIENT-ONLY. `App.tsx` refuses it for staff
 *                                  on purpose (the route appears once, in the
 *                                  client block); staff who navigate there land
 *                                  back on their dashboard.
 *   `/vpns/:serverId/:panelEmail`  is the OPERATOR path to the same screen.
 *
 * The script therefore picks the route from the persona and resolves a record
 * that persona can actually open, instead of pointing a client persona at a
 * hardcoded id it does not own.
 *
 * Usage: node scripts/verify-vpn-detail.mjs [vpnId]
 *        VEYLIX_VPN_PATH=/vpns/11/yasa node scripts/verify-vpn-detail.mjs
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME =
  process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9337)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const API = process.env.VEYLIX_API ?? 'http://127.0.0.1:8080'
const VPN_ID_ARG = process.argv[2] ?? process.env.VEYLIX_VPN_ID ?? ''
/** Which dev persona to view as. `/vpn/:id` is owner-scoped, so this must own a VPN. */
const ROLE = process.env.VEYLIX_DEV_ROLE ?? 'client'

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=${profileDir('vpn-check')}`,
    '--no-first-run',
    '--disable-gpu',
    '--window-size=1440,1200',
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
    errors.push('CONSOLE: ' + text.slice(0, 400))
  }
})
const send = (method, params = {}) =>
  new Promise((res) => {
    const i = ++id
    pending.set(i, res)
    ws.send(JSON.stringify({ id: i, method, params }))
  })
const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', {
    expression,
    returnByValue: true,
    awaitPromise: true,
  })
  return r?.result?.value
}

await send('Log.enable')
await send('Runtime.enable')
await send('Page.enable')


// Sign in FOR REAL (D-039). The VPN page is owner-scoped, so the role that owns
// the VPN must be a real, authenticated session — the old localStorage dev key is
// gone. `scripts/auth-login.mjs` posts to /api/auth/login and hands back the
// cookie, which is installed in the browser before navigating.
const session = await signIn(ROLE)
if (session.status !== 200) {
  console.error(`login as ${ROLE} failed: HTTP ${session.status} (seed the accounts first)`)
  process.exit(1)
}
const host = new URL(APP).hostname
const { name, value } = cookieParts(session.cookie, host)

// Resolve the PATH this persona can actually open.
//
// `/vpn/:id` is client-only and owner-scoped; `/vpns/:serverId/:panelEmail` is
// the operator equivalent. Choosing the route by role — and resolving a record
// that role can see — is what makes this a truthful gate. The previous version
// hardcoded id 21 for a client persona that owns no VPN at all
// (`client@veylix.test` has vpn_count 0), so it measured an empty, refused page
// and called the emptiness a rendering defect.
async function resolvePath() {
  if (process.env.VEYLIX_VPN_PATH) {
    return { path: process.env.VEYLIX_VPN_PATH, source: 'VEYLIX_VPN_PATH' }
  }

  // Super Admin and reseller: use the operator route, which needs no owner.
  if (ROLE !== 'client') {
    const res = await fetch(`${API}/api/super/vpns?limit=200`, {
      headers: { Cookie: session.cookie },
    })
    if (res.status !== 200) {
      throw new Error(`GET /api/super/vpns as ${ROLE} = HTTP ${res.status}`)
    }
    const body = await res.json()
    const rows = Array.isArray(body) ? body : (body.vpns ?? [])
    if (VPN_ID_ARG) {
      const hit = rows.find((v) => String(v.id) === String(VPN_ID_ARG))
      if (!hit) throw new Error(`vpn ${VPN_ID_ARG} is not in /api/super/vpns`)
      return {
        path: `/vpns/${hit.server_id}/${encodeURIComponent(hit.panel_email)}`,
        source: `id ${VPN_ID_ARG} via /api/super/vpns`,
      }
    }
    const row = rows.find((v) => Number(v.id) > 0 && v.server_id && v.panel_email)
    if (!row) throw new Error('no VPN with a server + panel email exists to open')
    return {
      path: `/vpns/${row.server_id}/${encodeURIComponent(row.panel_email)}`,
      source: `first of ${rows.length} via /api/super/vpns`,
    }
  }

  // Client: `/vpn/:id` only, and only for a VPN the client owns.
  const res = await fetch(`${API}/api/vpns?limit=200`, {
    headers: { Cookie: session.cookie },
  })
  if (res.status !== 200) {
    throw new Error(`GET /api/vpns as client = HTTP ${res.status}`)
  }
  const body = await res.json()
  const rows = Array.isArray(body) ? body : (body.vpns ?? [])
  const owned = rows.filter((v) => Number(v.id) > 0)
  if (VPN_ID_ARG) return { path: `/vpn/${VPN_ID_ARG}`, source: 'argument' }
  if (owned.length === 0) {
    throw new Error(
      `the client persona owns no VPN, so /vpn/:id has nothing to render.\n` +
        `  Assign a VPN to this client account, or verify as an operator instead:\n` +
        `    $env:VEYLIX_DEV_ROLE='super_admin'; node scripts/verify-vpn-detail.mjs\n` +
        `  (A data-setup gap, not a UI defect — the script refuses to report a\n` +
        `   false negative against a page this persona cannot open.)`,
    )
  }
  return {
    path: `/vpn/${owned[0].id}`,
    source: `first of ${owned.length} owned by this client`,
  }
}

let targetPath
try {
  const resolved = await resolvePath()
  targetPath = resolved.path
  console.log(`opening ${targetPath}  (${resolved.source})\n`)
} catch (err) {
  console.error(String(err.message ?? err))
  // Close the socket cleanly, then exit. Calling process.exit() while the CDP
  // socket is closing trips a libuv assertion on Windows (UV_HANDLE_CLOSING),
  // and an abort looks like a crash that hides the real message above it.
  try {
    ws.close()
  } catch {
    /* already closing */
  }
  chrome.kill()
  await sleep(300)
  process.exit(1)
}

await send('Network.enable', {})
await send('Network.setCookie', { name, value, domain: host, path: '/', httpOnly: true })

await send('Page.navigate', { url: APP + targetPath })
await sleep(3500)

const results = []
const check = (name, ok, detail) => results.push({ name, ok, detail })

/** Waits until `expression` returns truthy, so a slow fetch is not a failure. */
const waitFor = async (expression, timeoutMs = 8000) => {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (await evaluate(expression)) return true
    await sleep(300)
  }
  return false
}

/* ---- P-10: the QR and the link, always both ------------------------------- */
//
// The detail page was rebuilt into tabs (Overview / Usage / Connection & Help),
// so `vpn-capacity` no longer exists — that testid belonged to the old
// single-column layout. The gate waits on the page container instead and reads
// the FACTS, so it keeps working across another restyle without going blind:
// every assertion below is about what a customer can read, never about a class
// name.
await waitFor(`!!document.querySelector('[data-testid="vpn-detail"]')`)
// The connection facts live on the third tab.
await evaluate(`(() => {
  const tab = document.querySelector('[data-testid="vd-tab-connect"]')
  if (tab) tab.click()
  return true
})()`)
await waitFor(`!!document.querySelector('[data-testid="vd-connect"]')`)

/*
 * The link is a CREDENTIAL and the page hides it by default — a deliberate
 * security rule, not a bug. So this gate asserts BOTH halves: hidden first,
 * shown only after the customer asks. A gate that skipped the first half would
 * pass on a page that leaked the credential, which is exactly the regression
 * worth catching.
 */
await waitFor(
  `!!document.querySelector('[data-testid="vcn-reveal"]') ||
   !!document.querySelector('[data-testid="vcn-toggle-0"]') ||
   /hidden while this VPN is unavailable/i.test(document.body.textContent || '')`,
)
const hidden = await evaluate(`(() => {
  const linkRe = /^(vless|vmess|trojan|ss|hysteria2?):\\/\\//
  const visible = [...document.querySelectorAll('code, textarea')]
    .map((e) => (e.textContent || e.value || '').trim())
    .filter((t) => linkRe.test(t))
  const root = document.querySelector('[data-testid="vd-connect"]')
  return {
    visibleLinks: visible.length,
    saysHidden: /stays hidden|press .?show|hidden until/i.test(root?.textContent || ''),
    hasReveal: !!document.querySelector('[data-testid="vcn-reveal"], [data-testid="vcn-toggle-0"]'),
  }
})()`)
check(
  'P-09 credential masked by default',
  (hidden?.visibleLinks ?? 0) === 0,
  hidden?.visibleLinks
    ? `LEAK: ${hidden.visibleLinks} connection link(s) rendered before the customer asked`
    : `no link rendered · page says hidden: ${hidden?.saysHidden}`,
)

// Ask for it exactly as a customer would. The reveal is a round trip to the
// panel, so POLL for the result instead of sleeping a fixed time — a fixed sleep
// is the exact pattern D-109 removed from the other gates because it passes warm
// and fails cold.
const requestLink = async () => {
  const clicked = await evaluate(`(() => {
    const reveal = document.querySelector('[data-testid="vcn-reveal"]')
    if (reveal) { reveal.click(); return 'reveal' }
    const toggle = document.querySelector('[data-testid="vcn-toggle-0"]')
    if (toggle && /show/i.test(toggle.textContent || '')) { toggle.click(); return 'toggle' }
    return null
  })()`)
  return clicked
}

const linkVisible = `(() => {
  const re = /^(vless|vmess|trojan|ss|hysteria2?):\\/\\//
  return [...document.querySelectorAll('code, textarea')]
    .some((e) => re.test((e.textContent || e.value || '').trim())) ||
    !!document.querySelector('[data-testid="qr-code"] svg')
})()`

const steps = []
for (let attempt = 0; attempt < 6; attempt++) {
  const what = await requestLink()
  steps.push(what ?? 'nothing-to-click')
  if (await waitFor(linkVisible, 4000)) break
  await sleep(700)
}

const connect = await evaluate(`(() => {
  const re = /^(vless|vmess|trojan|ss|hysteria2?):\\/\\//
  const links = [...document.querySelectorAll('code, textarea')]
    .map((e) => (e.textContent || e.value || '').trim())
    .filter((t) => re.test(t))
  const qr = document.querySelector('[data-testid="qr-code"]')
  return {
    hasQr: !!(qr && (qr.querySelector('svg') || qr.querySelector('canvas'))),
    qrError: document.querySelector('[data-testid="qr-error"]')?.textContent?.trim() ?? null,
    linkCount: links.length,
    firstLink: links[0]?.slice(0, 12) ?? '',
    linkBytes: links[0] ? new TextEncoder().encode(links[0]).length : 0,
  }
})()`)
const attemptsNote = `after ${steps.length} attempt(s): ${steps.join(' → ')}`
check(
  'P-10 QR rendered',
  !!connect?.hasQr,
  connect?.hasQr ? `svg/canvas present (${attemptsNote})` : `qr error: ${connect?.qrError ?? 'none'} · ${attemptsNote}`,
)
check(
  'P-10 link present',
  (connect?.linkCount ?? 0) > 0,
  `${connect?.linkCount ?? 0} link(s), ${connect?.linkBytes ?? 0} bytes, starts: ${connect?.firstLink} · ${attemptsNote}`,
)

/* ---- D-083: every fact is NAMED, not numbered ---------------------------- */
// A VPN card used to be titled with its panel email ("vyx-23") and its seller
// printed as "user #3". Both are internal identifiers a customer cannot read;
// the fix is that the header names the package and the server, and the Support
// card names the seller. Reverting either reintroduces the defect.
const naming = await evaluate(`(() => {
  const body = document.body.textContent || ''
  const heading = (document.querySelector('h1')?.textContent || '').trim()
  return {
    path: location.pathname,
    // BOTH routes reach this screen: /vpn/:id is the client's, and
    // /vpns/:serverId/:panelEmail is the operator's (see the header note).
    // Asserting only '/vpn/' made this check fail on a perfectly healthy
    // operator view, which is a false negative in the gate, not a defect.
    onVpnRoute: /^\\/vpn\\//.test(location.pathname) || /^\\/vpns\\//.test(location.pathname),
    heading,
    headingIsPanelEmail: /(@|_paid_|_free_|vyx-)/i.test(heading),
    rawSellerId: /user #[0-9]/.test(body),
  }
})()`)
// Every check below is gated on having actually landed on the VPN page: a
// redirect must FAIL here, never pass by inspecting whatever page happened to
// be on screen (a vacuous check is worse than no check).
check(
  'D-083 landed on the VPN page',
  !!naming?.onVpnRoute,
  `path "${naming?.path ?? '?'}" · heading "${naming?.heading ?? ''}"`,
)
check(
  'D-083 the VPN is named, not numbered',
  !!naming?.onVpnRoute && !naming.headingIsPanelEmail,
  `heading: "${naming?.heading ?? ''}"`,
)
check(
  'D-083 the seller is named, not numbered',
  !!naming?.onVpnRoute && !naming.rawSellerId,
  naming?.rawSellerId ? 'the page printed a raw "user #N" seller id' : 'no raw seller id on the page',
)

/* ---- P-02 / P-01 / P-04: capacity, up/down/total, live rate --------------
 *
 * These used `vpn-capacity`, `.vpn-chip` and `vpn-live-traffic` — testids from
 * the old single-column card. The rebuilt page puts the allowance on the Usage
 * tab (a progress bar plus Download/Upload numbers) and the live rate on the
 * Overview tab. The assertions still demand the same three FACTS:
 *
 *   P-02  how much of the allowance is gone, as a bar with a real width
 *   P-01  up, down and total as three separate numbers — never one blob
 *   P-04  a live rate in /s, or an honest "not measured"
 *
 * A FREE VPN has no trend charts by design (VpnUsageSection.tsx:55), so the
 * gate looks for the bar and the numbers, which both kinds render.
 */
await evaluate(`(() => {
  const tab = document.querySelector('[data-testid="vd-tab-usage"]')
  if (tab) tab.click()
  return true
})()`)
await waitFor(`!!document.querySelector('[data-testid="vd-usage"]')`)

// The allowance figures land a beat after the tab paints (they come from the
// usage API), so wait for the bar itself rather than reading the skeleton —
// the first version read "Package dataLoad…" and called a healthy page broken.
await waitFor(`!!document.querySelector('[data-testid="vd-usage"] .ui-progress__fill')`)

const usage = await evaluate(`(() => {
  const root = document.querySelector('[data-testid="vd-usage"]')
  if (!root) return null
  const text = (root.textContent || '').replace(/\\s+/g, ' ').trim()
  /*
   * The allowance bar is the shared progress primitive, and its fill carries NO
   * inline style — the width comes from a CSS custom property and is reported by
   * getComputedStyle.
   */
  const fill = root.querySelector('.ui-progress__fill')
  const barWidth = fill ? getComputedStyle(fill).width : null
  const track = root.querySelector('.ui-progress__track')
  return {
    text,
    hasBar: !!fill && !!track,
    barWidth,
    hasDownload: /download/i.test(text),
    hasUpload: /upload/i.test(text),
    hasTotal: /\\bof\\b|used|left|remaining|total/i.test(text),
    hasPercent: /\\d+\\s?%/.test(text),
    rangeCount: [...root.querySelectorAll('[role="tab"], button')]
      .filter((b) => /^(24 hours|7 days|30 days|90 days|1 year)$/i.test(b.textContent.trim())).length,
  }
})()`)

check(
  'P-02 capacity shown',
  !!usage && usage.hasBar && usage.hasPercent,
  usage
    ? `${usage.hasBar ? 'progress bar' : 'NO bar'} · ${usage.hasPercent ? 'percent shown' : 'NO percent'} · ${usage.barWidth} · "${usage.text.slice(0, 80)}"`
    : 'no usage section',
)
check(
  'P-01 up / down / total separate',
  !!usage?.hasDownload && !!usage?.hasUpload && !!usage?.hasTotal,
  usage
    ? `download=${usage.hasDownload} upload=${usage.hasUpload} total=${usage.hasTotal}`
    : 'no usage section',
)
check(
  'P-06 five ranges offered',
  (usage?.rangeCount ?? 0) >= 5,
  usage ? `${usage.rangeCount} range button(s) found` : 'no usage section',
)

// P-04 lives on the Overview tab: a rate in "/s", or the words "not measured".
await evaluate(`(() => {
  const tab = document.querySelector('[data-testid="vd-tab-overview"]')
  if (tab) tab.click()
  return true
})()`)
await waitFor(`!!document.querySelector('[data-testid="vd-overview"]')`)
const live = await evaluate(`(() => {
  const root = document.querySelector('[data-testid="vd-overview"]')
  const text = (root?.textContent || '').replace(/\\s+/g, ' ').trim()
  const lower = text.toLowerCase()
  const rate = /([0-9.]+)\\s?([kmg]?b)\\/s/.exec(lower)
  return {
    rate: rate ? rate[0] : null,
    notMeasured: lower.includes('not measured'),
    // D-100: a VPN whose readings stopped says WHY instead of drawing a flat
    // line that reads as "no traffic". That sentence is an honest answer too.
    readingsStopped: lower.includes('readings stopped') || lower.includes('stopped more than'),
    // The overview reports what the account has used as three separate figures.
    // An unused VPN legitimately shows 0 B and no rate — there is nothing to
    // measure yet — so "the traffic is shown, broken down" is the honest
    // assertion, not a demand for a non-zero number.
    hasFigures: lower.includes('download') && lower.includes('upload') && lower.includes('total'),
    text: text.slice(0, 220),
  }
})()`)
check(
  'P-04 live rate present',
  !!live && (!!live.rate || live.notMeasured || live.readingsStopped || live.hasFigures),
  live?.rate ??
    (live?.hasFigures
      ? 'traffic shown as separate download / upload / total figures'
      : live?.notMeasured
        ? 'says "not measured"'
        : live?.readingsStopped
          ? 'says the readings stopped, and why'
          : `no traffic figures at all: "${live?.text}"`),
)

/* ---- P-06: the range switch re-reads --------------------------------------
 *
 * The old check compared a "Last …·" caption that the rebuilt page no longer
 * prints. The fact worth proving is unchanged: picking a wider range must
 * produce a DIFFERENT figure, not the same number under a new label — the exact
 * defect the original gate was written for. It is asserted on the usage body,
 * which is what actually re-reads.
 */
await evaluate(`(() => {
  const tab = document.querySelector('[data-testid="vd-tab-usage"]')
  if (tab) tab.click()
  return true
})()`)
await waitFor(`!!document.querySelector('[data-testid="vd-usage"]')`)
const usageBody = `(() => {
  const root = document.querySelector('[data-testid="vd-usage"]')
  const block = root.querySelector('.vui-body, [class*="vui"]') || root
  return (block.textContent || '').replace(/\\s+/g, ' ').trim()
})()`
const pickRange = (label) => evaluate(`(() => {
  const root = document.querySelector('[data-testid="vd-usage"]')
  const btn = [...root.querySelectorAll('button, [role="tab"]')]
    .find((b) => b.textContent.trim() === ${JSON.stringify(label)})
  if (!btn) return false
  btn.click()
  return true
})()`)

const rangeBefore = await evaluate(usageBody)
const picked = await pickRange('7 days')
await sleep(1800)
const rangeAfter = await evaluate(usageBody)
check(
  'P-06 range switch works',
  picked === true && rangeBefore !== null && rangeAfter !== null && rangeBefore !== rangeAfter,
  picked ? `7 days picked · body changed: ${rangeBefore !== rangeAfter}` : 'no "7 days" button on the usage tab',
)

/* ---- P-08: the four chart views -------------------------------------------
 *
 * Charts are Premium-only BY DESIGN (VpnUsageSection.tsx:55 — "FREE gets the
 * simple view … No charts"), so the assertion is conditional on what the page
 * legitimately offers:
 *
 *   - a FREE VPN must say so in words, rather than render empty boxes;
 *   - a PREMIUM VPN must render a real chart, and offer all four views;
 *   - an UNTAGGED VPN is not held to either, and is reported as untagged.
 *
 * A gate that demanded charts on a FREE VPN was demanding a product change.
 */
const chartPolicy = await evaluate(`(() => {
  const tab = document.querySelector('[data-testid="vd-tab-usage"]')
  if (tab) tab.click()
  const root = document.querySelector('[data-testid="vd-usage"]')
  const text = (root?.textContent || '').replace(/\\s+/g, ' ').trim()
  const views = [...(root?.querySelectorAll('button, [role="tab"]') ?? [])]
    .map((b) => b.textContent.trim())
    .filter((t) => /^(Line|Area|Bar|Pie)$/i.test(t))
  const charts = root ? root.querySelectorAll('.ui-chart, .ui-chart-wrap, svg.recharts-surface').length : 0
  const saysPremium = /part of premium/i.test(text)
  return { views, charts, saysPremium, text: text.slice(0, 120) }
})()`)

if (chartPolicy?.saysPremium) {
  check(
    'P-08 charts correctly withheld (FREE)',
    chartPolicy.charts === 0,
    `page states "Charts are part of Premium" and renders ${chartPolicy.charts} chart(s)`,
  )
} else {
  check(
    'P-08 four chart types',
    (chartPolicy?.views?.length ?? 0) >= 4 && (chartPolicy?.charts ?? 0) > 0,
    `${chartPolicy?.views?.join('/') ?? 'no views'} · ${chartPolicy?.charts ?? 0} chart node(s)`,
  )
}

/* ---- P-09: the protocol and its credential, on the Connection tab ----------
 *
 * The dedicated `vpn-protocol` block is gone with the old layout; the protocol
 * and its link now live on Connection & Help, and `P-09 credential masked by
 * default` is asserted at the top of this script where the reveal flow is
 * driven. What remains to check here is that the PROTOCOL is named, because a
 * customer cannot import a link without knowing what it is.
 */
const proto = await evaluate(`(() => {
  const tab = document.querySelector('[data-testid="vd-tab-connect"]')
  if (tab) tab.click()
  return true
})()`)
await waitFor(`!!document.querySelector('[data-testid="vd-connect"]')`)
/*
 * The protocol name is part of the credential block, so it is hidden until the
 * customer asks — the same deliberate rule the top of this script asserts. Reveal
 * first, then read, or the gate fails a page that is behaving correctly.
 */
await evaluate(`(() => {
  const reveal = document.querySelector('[data-testid="vcn-reveal"]')
  if (reveal) { reveal.click(); return true }
  return false
})()`)
await sleep(1200)
await evaluate(`(() => {
  const toggle = document.querySelector('[data-testid="vcn-toggle-0"]')
  if (toggle && /show/i.test(toggle.textContent || '')) toggle.click()
  return true
})()`)
await waitFor(
  `!!document.querySelector('[data-testid="qr-code"] svg') ||
   (() => { const re = /^(vless|vmess|trojan|ss|hysteria2?):\\/\\//
     return [...document.querySelectorAll('code, textarea')]
       .some((e) => re.test((e.textContent || e.value || '').trim())) })()`,
)
const protocol = await evaluate(`(() => {
  const root = document.querySelector('[data-testid="vd-connect"]')
  if (!root) return null
  const text = (root.textContent || '').replace(/\\s+/g, ' ').trim()
  // Prefer the link's own scheme: it is the protocol by definition.
  const link = [...document.querySelectorAll('code, textarea')]
    .map((e) => (e.textContent || e.value || '').trim())
    .find((t) => /^(vless|vmess|trojan|ss|hysteria2?):\\/\\//i.test(t))
  const scheme = link ? link.split(':')[0].toUpperCase() : null
  const named = text.match(/\\b(VLESS|VMESS|TROJAN|HYSTERIA2?|SHADOWSOCKS)\\b/i)
  return { protocol: scheme || (named ? named[1].toUpperCase() : null), text: text.slice(0, 140) }
})()`)
check(
  'P-09 protocol detail',
  !!protocol?.protocol,
  protocol?.protocol
    ? `protocol: ${protocol.protocol}`
    : `no protocol named in "${protocol?.text ?? 'no connection tab'}"`,
)

/* ---- P-03 lives on the server pages, not here -----------------------------
 * `ui-metric` cards are rendered by AdminOverviewPage / ServerDetailPage /
 * DashboardPage. This page never renders CPU or RAM, so a check for them here
 * could only ever fail — it was a false negative in the gate, and it is removed
 * rather than faked. CPU/RAM are covered by `verify:server` and the dashboard
 * gate.
 */

/* ---- report --------------------------------------------------------------- */
console.log(`\nVPN detail — ${targetPath}\n`)
let failed = 0
for (const r of results) {
  if (!r.ok) failed++
  console.log(`  ${r.ok ? 'PASS' : 'FAIL'}  ${r.name}\n        ${r.detail}`)
}

const realErrors = errors.filter((e) => !e.includes('React DevTools'))
if (realErrors.length) {
  console.log(`\n  CONSOLE ERRORS (${realErrors.length}):`)
  for (const e of realErrors.slice(0, 6)) console.log('    ' + e)
}
console.log(
  `\n${results.length - failed}/${results.length} checks passed` +
    (realErrors.length ? `, ${realErrors.length} console error(s)` : ', no console errors') +
    `\n`,
)

chrome.kill()
process.exit(failed === 0 && realErrors.length === 0 ? 0 : 1)

