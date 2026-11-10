/**
 * VEYLIX — every page a role can reach must be reachable by CLICKING (D-057),
 * and no role may be offered another role's destinations (D-084).
 *
 *   node scripts/verify-nav.mjs
 *
 * A page that exists at a URL but has no sidebar entry is a page the operator will
 * never find: `Admin management` was reachable only by typing `/staff`, and a client
 * could not reach `Buy premium` or `Purchases` from anywhere in the nav. This asserts
 * each role's sidebar contains the routes that role is allowed to use.
 *
 * D-084 note: the expectation tables below are kept in step with
 * `src/components/layout/navModel.ts`, which is now the single definition of the
 * role-scoped tree. When a destination moves, it moves there and here together —
 * a route missing HERE is exactly how a whole screen used to go unnoticed.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9441)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'

/**
 * What each role's sidebar must contain, read from `App.tsx`'s NAV (D-057).
 *
 * Kept in step with the real nav: `/staff` replaced the old `/admins` link when
 * the Staff console landed, and the Operations block (packages, orders, pricing)
 * rides the sidebar since Phase L — a route missing HERE is exactly how a whole
 * screen used to go unnoticed (the admin could never reach /staff at all).
 */
const EXPECTED = {
  super_admin: [
    '/dashboard', '/servers', '/offers', '/staff', '/clients', '/vpns', '/audit',
    '/packages', '/orders', '/payments', '/pricing', '/settings',
    '/chat', '/notifications', '/profile',
  ],
  admin: [
    '/admin', '/admin-servers', '/admin-clients', '/admin-vpns', '/sale', '/retail-price',
    '/packages', '/orders', '/payments',
    '/chat', '/notifications', '/profile',
  ],
  client: [
    '/client', '/client/vpns', '/client/usage',
    '/client/offers', '/buy', '/client/billing',
    '/client/guides', '/client/support',
    '/client/notifications', '/profile',
  ],
}

/**
 * Destinations that must be ABSENT from a role's sidebar even though the route
 * still resolves.
 *
 * `/history` is the case this exists for (D-091 billing merge). `navModel.ts`
 * folded "Payments & Billing" and "Purchases" into ONE destination and kept the
 * `/history` ROUTE alive, because bookmarks, e-mail links and the Dashboard's
 * "Open Purchases" button all resolve to it. Being reachable is not the same as
 * being a second navigation item — and asserting the opposite made this gate fail
 * on correct code, which is worse than no gate at all.
 */
const NOT_A_NAV_ITEM = {
  client: ['/history'],
}

/**
 * D-084 — a role must NOT see another role's destinations.
 *
 * Reachability alone cannot catch the defect this redesign fixes. The old model
 * was ONE nav array filtered per item, so the failure mode was never a missing
 * link — it was three products sharing one overloaded sidebar. Proving the
 * redesign therefore needs the negative case: the reseller must not be offered
 * the owner's `/clients`, `/staff`, `/audit` or `/settings`, and a client must
 * not be offered any operator screen. Both statements are checked below against
 * what the browser actually renders, not against the source array.
 *
 * `/vpns` is asserted forbidden for a client and stays that way: it is the Super
 * Admin VPN console (D-043). The client's own inventory lives at `/client/vpns`
 * precisely so the two paths can never collide — one path, one owner.
 */
const FORBIDDEN = {
  super_admin: ['/admin', '/admin-clients', '/admin-vpns', '/sale', '/retail-price'],
  admin: ['/clients', '/staff', '/audit', '/settings', '/vpns', '/offers', '/pricing', '/dashboard'],
  client: [
    '/dashboard', '/admin', '/servers', '/clients', '/staff', '/vpns', '/audit',
    '/settings', '/orders', '/payments', '/packages', '/sale',
  ],
}

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('nav')}`, '--no-first-run',
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

await send('Runtime.enable')
await send('Page.enable')
await send('Network.enable')

const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  return r?.result?.value
}

let pass = 0
let fail = 0
function check(label, ok, detail = '') {
  if (ok) pass++
  else fail++
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${label.padEnd(52)} ${detail}`)
}

for (const [role, paths] of Object.entries(EXPECTED)) {
  const s = await signIn(role)
  if (s.status !== 200) {
    check(`${role} sign-in`, false, `status ${s.status}`)
    continue
  }
  await send('Network.clearBrowserCookies')
  const c = cookieParts(s.cookie)
  await send('Network.setCookie', { name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true })

  await send('Page.navigate', { url: `${APP}/` })
  await sleep(1600)

  const links = await evaluate(
    `[...document.querySelectorAll('a[href]')].map((a) => new URL(a.href).pathname)`,
  )
  const hrefs = Array.isArray(links) ? links : []
  for (const p of paths) {
    check(`${role} nav has ${p}`, hrefs.includes(p), hrefs.includes(p) ? '' : 'missing from sidebar')
  }
  for (const p of FORBIDDEN[role] ?? []) {
    check(`${role} nav omits ${p}`, !hrefs.includes(p), hrefs.includes(p) ? 'LEAKED from another role' : '')
  }
  for (const p of NOT_A_NAV_ITEM[role] ?? []) {
    // The route still resolves — it is only not a second navigation item. The
    // pair of assertions (absent here, and "resolves" further down) is what
    // proves the merge instead of merely tolerating it.
    check(
      `${role} nav omits ${p} (route kept, not a nav item)`,
      !hrefs.includes(p),
      hrefs.includes(p) ? 're-added as a nav item after the billing merge' : '',
    )
  }
}

/*
 * S3 — every client nav destination must RESOLVE to a client screen.
 *
 * The reachability check above proves a link is present; it cannot prove the
 * link goes anywhere. A destination whose route is missing, or whose route is
 * declared inside another role's guard, renders a silent redirect back to
 * `/client` — the nav looks correct and the screen is unreachable. This walks
 * each client destination and asserts the URL did not bounce.
 */
{
  const s = await signIn('client')
  if (s.status === 200) {
    await send('Network.clearBrowserCookies')
    const c = cookieParts(s.cookie)
    await send('Network.setCookie', { name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true })
    for (const dest of EXPECTED.client) {
      await send('Page.navigate', { url: `${APP}${dest}` })
      await sleep(1200)
      const landed = await evaluate('location.pathname')
      check(`client ${dest} resolves`, landed === dest, `landed: ${landed}`)
    }
    /*
     * A route that is no longer a nav item must still RESOLVE — bookmarks,
     * e-mail links and the Dashboard's "Open Purchases" button all point at
     * `/history`. Removing it from the sidebar was the fix; removing the route
     * would have been a regression, so both halves are asserted.
     */
    for (const dest of NOT_A_NAV_ITEM.client ?? []) {
      await send('Page.navigate', { url: `${APP}${dest}` })
      await sleep(1200)
      const landed = await evaluate('location.pathname')
      check(`client ${dest} still resolves (bookmark-safe)`, landed === dest, `landed: ${landed}`)
    }
  }
}

console.log(`\nNAV ${fail === 0 ? 'OK' : 'FAILED'} — ${pass} passed, ${fail} failed`)
ws.close()
chrome.kill()
process.exit(fail === 0 ? 0 : 1)
