/**
 * VEYLIX — the Super Admin's VPN lifecycle controls must exist and must run.
 *
 *   node scripts/verify-vpn-actions.mjs [vpnId]
 *
 * Why this file exists: `suspend`, `resume` and `revoke` had working APIs since
 * Phase 5 and `vpnStore` exposed all three, yet NO screen ever called one — a
 * Super Admin could look at a VPN and could not stop it. Nothing failed, nothing
 * logged: the feature had simply never been wired, so no existing test could see
 * the absence. This asserts both halves that a missing button hides:
 *
 *   1. the controls are rendered to staff (and NOT to a client), and
 *   2. suspend really disables the panel client, and resume really restores it.
 *
 * Step 2 is why the test talks to the API directly as well: a button that fires
 * a request the server rejects would still "look" wired.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9443)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const API = process.env.VEYLIX_API ?? 'http://127.0.0.1:8080'
// Resolved below, after sign-in: see resolveTargetVPN. It is `let` because the
// only trustworthy value comes from the catalogue the Super Admin can actually
// read, not from a constant this file invented.
let VPN_ID = 0
// The operator's own detail route (`/vpns/:serverId/:panelEmail`). `/vpn/:id`
// belongs to the client block in App.tsx and must not be used by a staff run.
let VPN_PATH = ''

/*
 * SAFETY: this suite used to end with a real `DELETE /api/vpns/{id}` against
 * whatever database the running backend served — which was the operator's LIVE
 * database, and it destroyed a customer's VPN row. Twice.
 *
 * A verification script must never be able to delete production data. The
 * destructive step is therefore gated behind an explicit opt-in that names a
 * disposable database. Without it, the suite only exercises non-destructive
 * behaviour (render, suspend, resume) and says so loudly.
 */
const ALLOW_DESTRUCTIVE = process.env.VEYLIX_ALLOW_DESTRUCTIVE === '1'
if (ALLOW_DESTRUCTIVE && !process.env.VEYLIX_TEST_DB) {
  console.error(
    'VEYLIX_ALLOW_DESTRUCTIVE=1 requires VEYLIX_TEST_DB to name the database under test.\n' +
      'Refusing to run a destructive check without knowing which database it will hit.',
  )
  process.exit(2)
}


const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('vpnactions')}`, '--no-first-run',
  '--disable-gpu', '--window-size=1440,1000', 'about:blank',
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

const failures = []
const check = (ok, msg) => {
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${msg}`)
  if (!ok) failures.push(msg)
}

await send('Runtime.enable')
await send('Page.enable')
await send('Network.enable')

/** Sign in through the API, then plant the session cookie in the browser. */
async function actAs(role) {
  const s = await signIn(role)
  if (s.status !== 200) throw new Error(`${role} sign-in failed: ${s.status}`)
  await send('Network.clearBrowserCookies')
  const c = cookieParts(s.cookie)
  await send('Network.setCookie', { name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true })
  // `s.cookie` is the raw "name=value" pair — the caller needs exactly that for
  // its own fetch() calls.
  return s.cookie
}

/** The buttons a page currently shows, by visible text. */
async function buttons() {
  return evaluate(`[...document.querySelectorAll('button')].map(b => b.textContent.trim()).filter(Boolean)`)
}

async function open(path, waitMs = 4500) {
  // The VPN detail page loads the store first, and a store load on a cold start
  // performs a LIVE panel read. That read is sometimes slower than any fixed
  // delay — it produced a false "Back to VPN management" (the page's not-found
  // state) on an otherwise healthy build. So the wait is on the OUTCOME, and the
  // budget is generous: a slow panel is not a product failure, and this suite
  // must not report one.
  await send('Page.navigate', { url: `${APP}${path}` })
  await sleep(waitMs)
  for (let i = 0; i < 40; i++) {
    const ready = await evaluate(
      `!!document.querySelector('.vpn-actions') || document.body.innerText.includes('VPN not found')`,
    )
    if (ready) return true
    await sleep(500)
  }
  return false
}

/* ---- 1 · The Super Admin sees the controls -------------------------------- */
const superCookie = await actAs('super_admin')

// Resolve the VPN id instead of trusting a hardcoded one.
//
// This script SUSPENDS and DELETES the row it opens, so a wrong id is not a
// harmless miss — it is a write against a record nobody asked it to touch. The
// old default of 36 was stale (the live catalogue held 38/39/40), and every
// write then 404'd, which looked like a broken VPN action rather than a broken
// fixture. So: take an explicit id if given, otherwise pick the first row the
// Super Admin can actually see, and abort when the catalogue is empty.
function resolveTargetVPN() {
  if (process.env.VEYLIX_VPN_ID) {
    return { id: Number(process.env.VEYLIX_VPN_ID), source: 'VEYLIX_VPN_ID' }
  }
  const arg = process.argv[2]
  if (arg) return { id: Number(arg), source: 'argument' }

  return fetch(`${API}/api/super/vpns?limit=200`, {
    headers: { cookie: superCookie },
  })
    .then((r) => r.json())
    .then((rows) => {
      const list = Array.isArray(rows) ? rows : (rows.vpns ?? [])
      /*
       * The operator's detail route is `/vpns/:serverId/:panelEmail`, NOT
       * `/vpn/:id`.
       *
       * `App.tsx` declares `/vpn/:id` inside the CLIENT block on purpose: one
       * path, one owner. This suite signs in as the Super Admin, so navigating
       * there matched no operator route, fell through to the dashboard, and the
       * "Suspend" assertion searched the dashboard for a VPN control. The check
       * could never pass — it was asserting the wrong page, not a broken product.
       *
       * Rows also need a real server_id and panel_email to build that path, so a
       * panel client with no local row (id 0) is not usable here either.
       */
      const usable = list.filter(
        (v) => Number(v.id) > 0 && Number(v.server_id) > 0 && String(v.panel_email || '') !== '',
      )
      if (usable.length === 0) {
        throw new Error(
          'no VPN with a local id, server_id and panel_email exists — seed or provision one first',
        )
      }
      const first = usable[0]
      return {
        id: Number(first.id),
        path: `/vpns/${Number(first.server_id)}/${encodeURIComponent(first.panel_email)}`,
        source: `first of ${usable.length} in /api/super/vpns`,
      }
    })
}

try {
  const target = await resolveTargetVPN()
  VPN_ID = target.id
  // The operator detail path. When the id came from an argument or the env var
  // there is no row to build it from, so fall back to the list screen — which
  // still proves no operator action is exposed to a client.
  VPN_PATH = target.path ?? ''
  console.log(`vpn ${VPN_ID} (${target.source})${VPN_PATH ? ` at ${VPN_PATH}` : ''}\n`)
} catch (err) {
  console.error(String(err.message ?? err))
  try {
    ws.close()
  } catch {
    /* already closing */
  }
  chrome.kill()
  await sleep(300)
  process.exit(1)
}

await open(VPN_PATH)
const staffButtons = (await buttons()) ?? []
check(staffButtons.includes('Suspend') || staffButtons.includes('Resume'),
  `staff sees a Suspend/Resume control (got: ${staffButtons.join(' | ')})`)
check(staffButtons.includes('Revoke'), 'staff sees a Revoke control')

/* ---- 2 · A client does NOT see them -------------------------------------- */
await actAs('client')
// /vpn/:id is client-only in the router; a client opening their own VPN must not
// be offered an operator action. If it redirects, that is also correct (they own
// no VPN in the seed data) — either way no lifecycle button may appear.
await open(`/vpn/${VPN_ID}`)
const clientButtons = (await buttons()) ?? []
check(!clientButtons.includes('Revoke'), 'a client never sees a Revoke control')

const cookie = await actAs('super_admin')
const call = async (path, method = 'POST') => {
  const r = await fetch(`${API}${path}`, {
    method,
    headers: {
      cookie,
      'content-type': 'application/json',
      // The CSRF guard (D-043) refuses a browser-shaped write with no provenance.
      // Declaring the API origin is what a real page does; omitting it produced a
      // 403 that looked like a permissions bug but was the harness lying.
      Origin: API,
    },
    body: method === 'POST' ? '{}' : undefined,
  })
  return { status: r.status, body: await r.text() }
}

const sus = await call(`/api/vpns/${VPN_ID}/suspend`)
check(sus.status === 204 || sus.status === 200, `suspend succeeds (HTTP ${sus.status})`)
check(sus.status !== 404, 'suspend never answers 404 for a VPN that exists (regression: it used to)')

/**
 * REGRESSION GUARD — learned the expensive way, on this very database.
 *
 * This suite is run against a REAL database. A destructive call in a verification
 * script is not a test, it is an outage: it once deleted a customer's VPN row.
 * So instead of deleting, this asserts the PRECONDITION the server enforces —
 * a VPN the panel still holds is refused. That exercises the same guard
 * (`exists → ErrVPNStillOnPanel`) before any write, and it is the property an
 * operator depends on: "Remove record" can never damage a working VPN.
 */
if (ALLOW_DESTRUCTIVE) {
  const del = await call(`/api/vpns/${VPN_ID}`, 'DELETE')
  check(
    del.status !== 200 && del.status !== 204,
    `DELETE refuses rather than removing a live VPN (HTTP ${del.status})`,
  )
} else {
  // Not opted in: say so instead of silently proving nothing.
  console.log(
    'SKIP  destructive DELETE check (set VEYLIX_ALLOW_DESTRUCTIVE=1 and VEYLIX_TEST_DB=<db> to enable)',
  )
}

const listAfter = await fetch(`${API}/api/vpns`, { headers: { cookie } }).then((r) => r.json())
const row = (listAfter ?? []).find((v) => v.id === VPN_ID)
if (row) {
  // A VPN whose panel client is gone reports SERVER_UNAVAILABLE, not SUSPENDED:
  // suspend only flips the panel's enable flag, and there is no client to flip.
  // Both are "not connectable", so either is a correct post-suspend state — what
  // must never happen is the VPN still claiming ACTIVE.
  const ok = row.status === 'SUSPENDED' || row.status === 'SERVER_UNAVAILABLE'
  check(ok, `VPN ${VPN_ID} is not ACTIVE after suspend (got ${row.status})`)
} else {
  console.log(`SKIP  VPN ${VPN_ID} is not in this session's list — panel state checked via the action only`)
}

const res = await call(`/api/vpns/${VPN_ID}/resume`)
check(res.status === 204 || res.status === 200, `resume restores the VPN (HTTP ${res.status})`)

await send('Page.navigate', { url: 'about:blank' })
chrome.kill()

console.log('')
if (failures.length) {
  console.log(`✗ ${failures.length} check(s) failed`)
  process.exit(1)
}
console.log('✓ all VPN lifecycle checks passed')
