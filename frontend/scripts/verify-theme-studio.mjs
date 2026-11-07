/**
 * VEYLIX - prove the Design Studio header controls work (regression test).
 *
 * The Studio header carries an expand/compact switch and a Close button. The
 * defect: the expand button was rendered INSIDE the Close button, so React event
 * bubbling sent a click on "Full view" straight into onClose and the whole studio
 * dismissed. A type-check cannot see this; only the rendered DOM and a real click can.
 *
 * Requires: dev.ps1 running (backend :8080 + Vite :5173).
 */
import { spawn } from 'node:child_process'
import { join } from 'node:path'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? join('C:', 'Program Files', 'Google', 'Chrome', 'Application', 'chrome.exe')
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9347)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    '--user-data-dir=' + profileDir('studio-check'),
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
// Vite dev on a cold profile compiles modules on first load, so poll for what we
// need instead of trusting a fixed delay.
async function waitFor(expression, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (await evaluate(expression)) return true
    await sleep(300)
  }
  return false
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
await send('Page.navigate', { url: APP + '/dashboard' })
await waitFor(`!!document.querySelector('.app-topbar')`)

const opened = await evaluate(`(() => {
  const btn = [...document.querySelectorAll('.app-topbar__icon-btn')]
    .find((b) => (b.getAttribute('aria-label') || '') === 'Open design studio');
  if (!btn) return false;
  btn.click();
  return true;
})()`)
await waitFor(`!!document.querySelector('.studio')`)

const afterOpen = await evaluate(`(() => {
  const studio = document.querySelector('.studio');
  const close = document.querySelector('.studio .ui-dialog__close');
  const expand = document.querySelector('.studio .studio__expand');
  return {
    studioPresent: !!studio,
    dataView: studio ? studio.getAttribute('data-view') : null,
    closePresent: !!close,
    expandPresent: !!expand,
    expandInsideClose: !!(close && expand && close.contains(expand)),
  };
})()`)

await evaluate(`(() => {
  const expand = document.querySelector('.studio .studio__expand');
  if (expand) expand.click();
  return true;
})()`)
await sleep(600)
const afterExpand = await evaluate(`(() => {
  const studio = document.querySelector('.studio');
  return { studioPresent: !!studio, dataView: studio ? studio.getAttribute('data-view') : null };
})()`)

await evaluate(`(() => {
  const close = document.querySelector('.studio .ui-dialog__close');
  if (close) close.click();
  return true;
})()`)
await sleep(600)
const afterClose = await evaluate(`!!document.querySelector('.studio')`)

const checks = {
  studioOpens: opened === true && afterOpen.studioPresent === true,
  closeButtonPresent: afterOpen.closePresent === true,
  expandButtonPresent: afterOpen.expandPresent === true,
  expandIsNotInsideClose: afterOpen.expandInsideClose === false,
  expandKeepsStudioOpen: afterExpand.studioPresent === true,
  expandTogglesToFull: afterExpand.dataView === 'full',
  closeDismisses: afterClose === false,
}

const report = { opened, afterOpen, afterExpand, checks, consoleErrors }
const failed = Object.entries(checks).filter(([, v]) => !v)
console.log(JSON.stringify(report, null, 2))
console.log(
  failed.length === 0 && consoleErrors.length === 0
    ? '\nTHEME-STUDIO OK - all checks passed, 0 console errors.'
    : '\nTHEME-STUDIO FAILED - ' + failed.map(([k]) => k).join(', '),
)

ws.close()
chrome.kill()
process.exit(failed.length === 0 && consoleErrors.length === 0 ? 0 : 1)
