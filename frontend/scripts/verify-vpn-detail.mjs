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
await waitFor(`!!document.querySelector('[data-testid="vpn-capacity"]')`)
// The links come from the panel, so they land a beat after the page paints.
await waitFor(
  `!!document.querySelector('[data-testid="qr-code"]') ||
   !!document.querySelector('[data-testid="qr-error"]') ||
   /hidden while this VPN is unavailable/i.test(document.body.textContent || '')`,
)
const connect = await evaluate(`(() => {
  const qrOk = document.querySelector('[data-testid="qr-code"] svg')
  const qrErr = document.querySelector('[data-testid="qr-error"]')
  const links = [...document.querySelectorAll('code')]
    .map((c) => c.textContent ?? '')
    .filter((t) => /^(vless|vmess|trojan|ss):\\/\\//.test(t))
  return {
    hasQr: !!qrOk,
    qrError: qrErr ? qrErr.textContent.replace(/\\s+/g, ' ').trim() : null,
    linkCount: links.length,
    firstLink: links[0]?.slice(0, 24) ?? '',
    linkBytes: links[0] ? new TextEncoder().encode(links[0]).length : 0,
  }
})()`)
check('P-10 QR rendered', !!connect?.hasQr, connect?.hasQr ? 'svg present' : `qr error: ${connect?.qrError ?? 'none'}`)
check(
  'P-10 link present',
  (connect?.linkCount ?? 0) > 0,
  `${connect?.linkCount ?? 0} link(s), ${connect?.linkBytes ?? 0} bytes, first: ${connect?.firstLink}`,
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

/* ---- P-02: capacity bar --------------------------------------------------- */
const cap = await evaluate(`(() => {
  const el = document.querySelector('[data-testid="vpn-capacity"]')
  if (!el) return null
  const fill = el.querySelector('.vpn-cap__fill')
  return {
    text: el.textContent.replace(/\\s+/g, ' ').trim(),
    width: fill ? fill.style.width : '(none)',
  }
})()`)
check('P-02 capacity shown', !!cap, cap ? `${cap.text} [bar ${cap.width}]` : 'no capacity block')

/* ---- P-01: up, down and total are three separate numbers ------------------ */
const chips = await evaluate(`(() => {
  const el = document.querySelector('[data-testid="vpn-capacity"]')?.parentElement
  return [...(el ? el.querySelectorAll('.vpn-chip') : [])].map((c) => c.textContent.trim())
})()`)
const hasDown = chips.some((c) => c.includes('down'))
const hasUp = chips.some((c) => c.includes('up'))
const hasTotal = chips.some((c) => c.includes('total'))
check('P-01 up / down / total separate', hasUp && hasDown && hasTotal, chips.join(' · '))

/* ---- P-04: live rate, or an honest "not measured" ------------------------- */
const live = await evaluate(`(() => {
  const el = document.querySelector('[data-testid="vpn-live-traffic"]')
  return el ? el.textContent.replace(/\\s+/g, ' ').trim() : null
})()`)
check(
  'P-04 live rate present',
  !!live && (live.includes('/s') || live.toLowerCase().includes('not measured')),
  live ?? 'no live block',
)

/* ---- P-06: the range switch re-reads -------------------------------------- */
const rangeBefore = await evaluate(`(() => {
  const m = document.body.textContent.match(/Last ([^·]+)·/); return m ? m[1].trim() : null
})()`)
await evaluate(`(() => {
  const btn = [...document.querySelectorAll('[role="tab"]')].find((b) => b.textContent.trim() === '7 days')
  if (btn) btn.click()
  return true
})()`)
await sleep(1600)
const rangeAfter = await evaluate(`(() => {
  const m = document.body.textContent.match(/Last ([^·]+)·/); return m ? m[1].trim() : null
})()`)
check(
  'P-06 range switch works',
  rangeBefore !== null && rangeAfter !== null && rangeBefore !== rangeAfter,
  `${rangeBefore} → ${rangeAfter}`,
)

/* ---- P-08: all four chart types render ------------------------------------ */
const chartKinds = []
for (const kind of ['Line', 'Area', 'Bar', 'Pie']) {
  await evaluate(`(() => {
    const btn = [...document.querySelectorAll('[role="tab"]')].find((b) => b.textContent.trim() === '${kind}')
    if (btn) btn.click()
    return true
  })()`)
  await sleep(400)
  const rendered = await evaluate(`!!document.querySelector('.ui-chart, .ui-chart-wrap')`)
  chartKinds.push(`${kind}:${rendered ? 'ok' : 'MISSING'}`)
}
check('P-08 four chart types', chartKinds.every((c) => c.endsWith('ok')), chartKinds.join(' '))

/* ---- P-09: protocol detail, masked until revealed ------------------------- */
const proto = await evaluate(`(() => {
  const el = document.querySelector('[data-testid="vpn-protocol"]')
  if (!el) return null
  const secret = el.querySelector('.vpn-proto__secret')
  return {
    text: el.textContent.replace(/\\s+/g, ' ').trim(),
    masked: secret ? secret.textContent.includes('•') : null,
    canReveal: !!el.querySelector('.vpn-proto__reveal'),
  }
})()`)
check('P-09 protocol detail', !!proto, proto ? proto.text.slice(0, 130) : 'no protocol block')
check('P-09 credential masked by default', proto?.masked === true, `masked=${proto?.masked}`)

if (proto?.canReveal) {
  await evaluate(`document.querySelector('.vpn-proto__reveal').click()`)
  await sleep(300)
  const after = await evaluate(`document.querySelector('.vpn-proto__secret')?.textContent ?? ''`)
  check('P-09 credential revealable', !!after && !after.includes('•'), `revealed ${after.length} chars`)
}

/* ---- P-03 / P-07: CPU and RAM --------------------------------------------- */
await waitFor(
  `[...document.querySelectorAll('.ui-metric')].some((c) => c.textContent.includes('CPU'))`,
)
const health = await evaluate(
  `[...document.querySelectorAll('.ui-metric')].map((c) => c.textContent.replace(/\\s+/g, ' ').trim())`,
)
check(
  'P-03 CPU / RAM present',
  (health ?? []).some((c) => c.includes('CPU')) && (health ?? []).some((c) => c.includes('RAM')),
  (health ?? []).join(' | ') || 'no metric cards',
)

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

