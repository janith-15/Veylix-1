/**
 * VEYLIX - prove a Super Admin can actually create an offer.
 *
 * Regression for D-110. Two defects, one symptom: every offer create failed.
 *
 *  1. `GET /api/servers/{id}/inbounds` answered with the PANEL's inbound number,
 *     while `vpn_offers.inbound_id` is a foreign key to the LOCAL `inbounds.id`.
 *     The form submitted the panel's number, the FK could not resolve it, and
 *     `POST /api/offers` refused with 422 `inbound_mismatch` — while the inbound
 *     was visibly listed for the server the operator had named.
 *  2. The dropdown's empty option read "Let the panel decide…", which names a
 *     capability Veylix does not have. Picking the default was therefore the one
 *     guaranteed-refused choice.
 *
 * This drives the real UI and the real backend — no stubs. It creates a probe
 * offer, asserts it exists, then DELETES it and asserts the catalogue is back to
 * the size it started at, so a green run leaves no residue.
 *
 * Requires: dev.ps1 running (backend :8080 + Vite :5173).
 */
import { spawn } from 'node:child_process'
import { join } from 'node:path'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? join('C:', 'Program Files', 'Google', 'Chrome', 'Application', 'chrome.exe')
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9351)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    '--user-data-dir=' + profileDir('offer-check'),
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
  chrome.kill()
  process.exit(1)
}
await send('Network.enable')
await send('Network.setCookie', { ...cookieParts(auth.cookie, 'localhost'), url: APP })

const open = async (path) => {
  await send('Page.navigate', { url: APP + path })
  await sleep(2200)
}

// --- 1. The inbounds endpoint must speak the id space the offer FK accepts ---
await open('/offers')
const inbounds = await evaluate(`(async () => {
  const r = await fetch('/api/servers/11/inbounds', { credentials: 'include' });
  if (!r.ok) return { status: r.status, rows: [] };
  const rows = await r.json();
  return { status: r.status, rows: rows.map((i) => ({ id: i.id, remark: i.remark })) };
})()`)

const panelIds = Array.isArray(inbounds?.rows) ? inbounds.rows : []

const countOffers = () =>
  evaluate(`(async () => {
    const r = await fetch('/api/offers', { credentials: 'include' });
    const d = await r.json();
    const rows = Array.isArray(d) ? d : (d.offers || []);
    return rows.length;
  })()`)

// --- 2. Every id the list offers must be accepted by POST /api/offers ---
// This is the assertion that would have caught D-110: the mismatch is only
// visible when the list's id is handed back to the write path.
const probeName = 'QA offer-create gate probe'
let createdId = 0
let createStatus = 0
let createCode = ''
let createBody = ''

const before = await countOffers()

if (panelIds.length > 0) {
  const first = panelIds[0].id
  const res = await evaluate(`(async () => {
    const r = await fetch('/api/offers', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        kind: 'FREE', name: ${JSON.stringify(probeName)}, server_id: 11,
        inbound_id: ${JSON.stringify(first)}, traffic_gb: 1, duration_days: 7,
        device_limit: 1, active: false,
      }),
    });
    const text = await r.text();
    return { status: r.status, body: text };
  })()`)
  createStatus = res?.status ?? 0
  createBody = res?.body ?? ''
  try {
    const j = JSON.parse(createBody)
    createdId = j?.offer?.id ?? j?.id ?? 0
    createCode = j?.error?.code ?? ''
  } catch {
    /* non-JSON answer */
  }
}

// --- 3. Clean up: the probe offer must not survive the gate ---
let deleted = false
if (createdId > 0) {
  const del = await evaluate(`(async () => {
    const r = await fetch('/api/offers/${createdId}', { method: 'DELETE', credentials: 'include' });
    return r.status;
  })()`)
  deleted = del === 204 || del === 200
}
const after = await countOffers()

// --- 4. The form no longer promises a choice the system cannot honour ---
await open('/offers')
const dropdown = await evaluate(`(async () => {
  const btn = [...document.querySelectorAll('button')].find((b) => /new offer/i.test(b.textContent || ''));
  if (!btn) return { error: 'New offer button not found' };
  btn.click();
  // Poll for the OUTCOME, never a fixed sleep. The inbound list is read from the
  // panel when the dialog opens, so the first option is briefly
  // "Reading the panel…". A fixed wait made this check pass two runs in three
  // (D-109: a gate that fails at random is worse than no gate).
  const deadline = Date.now() + 15000;
  let opts = [];
  let dialog = null;
  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, 200));
    dialog = document.querySelector('[role="dialog"]');
    if (!dialog) continue;
    const selects = [...dialog.querySelectorAll('select')];
    const inbound = selects[selects.length - 1];
    if (!inbound) continue;
    opts = [...inbound.options].map((o) => ({ value: o.value, label: o.textContent.trim() }));
    // Settled when the prompt is no longer the loading sentence.
    if (opts.length > 0 && !/reading the panel/i.test(opts[0].label || '')) break;
  }
  if (!dialog) return { error: 'offer dialog did not open' };
  return { opts };
})()`)

const firstOption = Array.isArray(dropdown?.opts) ? dropdown.opts[0] : null

// --- 4. A package create must persist AND render its server ---
// D-111: `GET /api/plans` served `store.Plan` raw, so `server_label` arrived as
// `undefined` and every row rendered "no server" — including rows whose
// `server_id` was correct. TypeScript could not catch it, because `ApiPlan`
// declares the field as required. This asserts the wire shape and the rendered
// text, then removes the probe so the catalogue is unchanged.
const pkgName = 'QA package-create gate probe'
let pkgId = 0
let pkgStatus = 0
let pkgBody = ''

const plansBefore = await evaluate(`(async () => {
  const r = await fetch('/api/plans', { credentials: 'include' });
  const d = await r.json();
  return Array.isArray(d) ? d.length : 0;
})()`)

const pkgRes = await evaluate(`(async () => {
  const inb = await fetch('/api/servers/11/inbounds', { credentials: 'include' }).then((x) => x.json());
  if (!inb || !inb.length) return { status: 0, body: 'no inbound to target' };
  const r = await fetch('/api/plans', {
    method: 'POST', credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      kind: 'PREMIUM', name: ${JSON.stringify(pkgName)}, server_id: 11,
      inbound_id: inb[0].id, traffic_gb: 5, duration_days: 30, device_limit: 1,
      price_cents: 100000, currency: 'LKR', slots_total: 0, is_custom: false,
    }),
  });
  return { status: r.status, body: await r.text() };
})()`)
pkgStatus = pkgRes?.status ?? 0
pkgBody = pkgRes?.body ?? ''
try {
  pkgId = JSON.parse(pkgBody)?.id ?? 0
} catch {
  /* non-JSON */
}

// The rendered row must name its server, not "no server".
await open('/packages')
const pkgRow = await evaluate(`(async () => {
  const deadline = Date.now() + 15000;
  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, 200));
    const row = [...document.querySelectorAll('tr')].find((tr) => (tr.textContent || '').includes(${JSON.stringify(pkgName)}));
    if (row) return { text: row.textContent.replace(/\\s+/g, ' ').trim() };
  }
  return { text: '' };
})()`)

const plansAfter = await evaluate(`(async () => {
  if (${pkgId} > 0) {
    await fetch('/api/plans/' + ${pkgId}, { method: 'DELETE', credentials: 'include' });
  }
  const r = await fetch('/api/plans', { credentials: 'include' });
  const d = await r.json();
  return Array.isArray(d) ? d.length : 0;
})()`)

const checks = {
  // The endpoint answered and listed at least one inbound to choose from.
  inboundsListed: panelIds.length > 0,
  // D-110 core: the id the list hands out is accepted by the write path.
  listedIdIsAccepted: createStatus === 201,
  // A wrong id space surfaces as exactly this refusal; name it so a regression
  // reads as the known defect rather than a mystery failure.
  noInboundMismatch: createCode !== 'inbound_mismatch',
  // The empty option is a prompt, not a phantom capability.
  emptyOptionIsPrompt: !!firstOption && /choose an inbound/i.test(firstOption.label || ''),
  noPanelDecideOption: !(dropdown?.opts || []).some((o) => /panel decide/i.test(o.label || '')),
  // D-111: a package is created and its row names the server it sells from.
  packageCreated: pkgStatus === 201 && pkgId > 0,
  packageRowNamesServer: /server 11/i.test(pkgRow?.text || ''),
  packageRowNotNoServer: !/no server/i.test(pkgRow?.text || ''),
  // The gate leaves the catalogue exactly as it found it.
  probeCleanedUp: after === before && plansAfter === plansBefore,
}

const report = {
  inbounds: panelIds,
  create: { status: createStatus, code: createCode, id: createdId, body: createBody.slice(0, 300) },
  dropdown: { firstOption, count: (dropdown?.opts || []).length, error: dropdown?.error },
  package: { status: pkgStatus, id: pkgId, row: pkgRow?.text, body: pkgBody.slice(0, 300) },
  catalogue: { offersBefore: before, offersAfter: after, plansBefore, plansAfter },
  checks,
  consoleErrors,
}
const failed = Object.entries(checks).filter(([, v]) => !v)
console.log(JSON.stringify(report, null, 2))
console.log(
  failed.length === 0 && consoleErrors.length === 0
    ? '\nOFFER-CREATE OK - offers and packages both create, both name their server, and the forms offer no impossible choice.'
    : '\nOFFER-CREATE FAILED - ' + failed.map(([k]) => k).join(', '),
)

ws.close()
chrome.kill()
process.exit(failed.length === 0 && consoleErrors.length === 0 ? 0 : 1)

