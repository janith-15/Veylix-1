/**
 * VEYLIX - prove the package-create payload the UI sends is the one the backend
 * accepts (regression for the CUSTOM package creation defect).
 *
 * The bug: creating a CUSTOM PREMIUM package failed. The UI posted
 * kind: CUSTOM_PREMIUM, but POST /api/plans accepts only FREE/PREMIUM plus the
 * is_custom flag, so the request was refused 422.
 *
 * This runs the REAL src/lib/api.ts (loaded from the Vite dev server, exactly as
 * the browser does) with fetch stubbed, and reads the body it serialises - the
 * wire contract the backend must accept.
 *
 * Requires: dev.ps1 running (backend :8080 + Vite :5173).
 */
import { spawn } from 'node:child_process'
import { join } from 'node:path'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? join('C:', 'Program Files', 'Google', 'Chrome', 'Application', 'chrome.exe')
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9348)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const API = process.env.VEYLIX_API ?? 'http://127.0.0.1:8080'

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    '--user-data-dir=' + profileDir('pkg-check'),
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
  throw new Error('no CDP page - is Chrome installed?')
}

const t = await pageTarget()
const ws = new WebSocket(t.webSocketDebuggerUrl)
await new Promise((r) => ws.addEventListener('open', r, { once: true }))

let id = 0
const pending = new Map()
const consoleErrors = []
ws.addEventListener('message', (ev) => {
  const msg = JSON.parse(ev.data)
  if (msg.id && pending.has(msg.id)) {
    pending.get(msg.id)(msg)
    pending.delete(msg.id)
  }
  if (msg.method === 'Runtime.consoleAPICalled' && msg.params.type === 'error') {
    consoleErrors.push(msg.params.args.map((a) => a.value ?? a.description ?? '').join(' '))
  }
  if (msg.method === 'Runtime.exceptionThrown') {
    consoleErrors.push(msg.params.exceptionDetails.text ?? 'exception')
  }
})
const send = (method, params = {}) =>
  new Promise((res) => {
    const mid = ++id
    pending.set(mid, res)
    ws.send(JSON.stringify({ id: mid, method, params }))
  })
async function evaluate(expression) {
  const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  if (r.result?.exceptionDetails) throw new Error(JSON.stringify(r.result.exceptionDetails))
  return r.result?.result?.value
}

await send('Runtime.enable')
await send('Page.enable')

const auth = await signIn('super_admin')
if (!auth.cookie) {
  console.error('sign-in failed: status=' + auth.status)
  process.exit(1)
}
await send('Network.enable')
await send('Network.setCookie', { ...cookieParts(auth.cookie, 'localhost'), url: APP })
await send('Page.navigate', { url: APP + '/packages' })
await sleep(2500)

const captured = await evaluate(`(async () => {
  const mod = await import('/src/lib/api.ts');
  const api = mod.api;
  const realFetch = window.fetch;
  let seen = null;
  window.fetch = (url, init) => {
    seen = { url: String(url), method: (init && init.method) || 'GET', body: init && init.body };
    return Promise.resolve(new Response(JSON.stringify({ id: 999 }), { status: 201, headers: { 'Content-Type': 'application/json' } }));
  };
  try {
    await api.createPlan({
      kind: 'CUSTOM_PREMIUM', name: 'Studio probe', serverId: 11, inboundId: 1,
      trafficGb: 0, durationDays: 0, deviceLimit: 0, priceCents: 0,
      currency: 'LKR', slotsTotal: 0, isCustom: true,
    });
  } catch (err) {
    return { error: String((err && err.message) || err) };
  } finally {
    window.fetch = realFetch;
  }
  return { url: seen && seen.url, method: seen && seen.method, body: seen && seen.body };
})()`)

const payload = captured && captured.body ? JSON.parse(captured.body) : null

let rawStatus = 0
try {
  const raw = await fetch(API + '/api/plans', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Cookie: auth.cookie, Origin: API },
    body: JSON.stringify({
      kind: 'CUSTOM_PREMIUM', name: 'studio-probe-raw', server_id: 11, inbound_id: 1,
      traffic_gb: 0, duration_days: 0, device_limit: 0, price_cents: 0,
      currency: 'LKR', slots_total: 0, is_custom: true,
    }),
  })
  rawStatus = raw.status
} catch {
  /* backend not running - the frontend check still stands */
}

const checks = {
  createPlanPostsToPlans: !!captured && captured.method === 'POST' && String(captured.url).includes('/api/plans'),
  kindSentIsPremium: !!payload && payload.kind === 'PREMIUM',
  kindNotRawUiValue: !!payload && payload.kind !== 'CUSTOM_PREMIUM',
  isCustomFlagSet: !!payload && payload.is_custom === true,
  rawUiKindIsRefused: rawStatus === 422,
}

const report = { captured, payload, rawStatus, checks, consoleErrors }
const failed = Object.entries(checks).filter(([, v]) => !v)
console.log(JSON.stringify(report, null, 2))
console.log(
  failed.length === 0 && consoleErrors.length === 0
    ? '\nPACKAGE-CREATE OK - the UI now sends kind=PREMIUM + is_custom, the shape the backend accepts.'
    : '\nPACKAGE-CREATE FAILED - ' + failed.map(([k]) => k).join(', '),
)

ws.close()
chrome.kill()
process.exit(failed.length === 0 && consoleErrors.length === 0 ? 0 : 1)
