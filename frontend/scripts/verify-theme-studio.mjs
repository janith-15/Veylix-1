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

/*
 * Wait for the BUTTON, not just for the dialog.
 *
 * `.studio` can mount one frame before its header controls do. Clicking blind with
 * `if (expand) expand.click()` then silently did nothing, the check read the
 * unchanged data-view, and the gate reported "expand does not work" — about one
 * run in three. A silent no-op inside a gate is a false failure.
 */
const clickedExpand = await evaluate(`(() => {
  const expand = document.querySelector('.studio .studio__expand');
  if (!expand) return false;
  expand.click();
  return true;
})()`)
if (!clickedExpand) {
  console.error('FAIL  the design studio has no expand control to click')
  ws.close()
  chrome.kill()
  process.exit(1)
}
/*
 * Poll for the OUTCOME instead of sleeping a fixed 600 ms.
 *
 * A fixed delay is a race: on a loaded machine the React re-render plus the CSS
 * transition can exceed it, and the check then reports "expand did not work" when
 * it merely had not finished. That made this gate fail about one run in three,
 * and a flaky gate gets ignored — which is worse than no gate.
 */
async function waitForDataView(want, budgetMs = 6000) {
  const deadline = Date.now() + budgetMs
  let last = null
  while (Date.now() < deadline) {
    last = await evaluate(`(() => {
      const studio = document.querySelector('.studio');
      return studio ? studio.getAttribute('data-view') : null;
    })()`)
    if (last === want) return last
    await sleep(150)
  }
  return last
}
/*
 * The studio PERSISTS its view (`studioView` in localStorage), so it reopens in
 * whatever state the previous run left it. Asserting "clicking expand yields full"
 * therefore passed or failed depending on the run before it — the gate was
 * order-dependent, which is a false failure about half the time.
 *
 * What must be true is that the control is a real TOGGLE: clicking it flips
 * data-view to the other value, and the button's own label describes the action.
 */
const expandBefore = afterOpen.dataView
const expandWanted = expandBefore === 'full' ? 'compact' : 'full'
const expandedView = await waitForDataView(expandWanted)
const afterExpand = await evaluate(`(() => {
  const studio = document.querySelector('.studio');
  return { studioPresent: !!studio, dataView: ${JSON.stringify(expandedView)} };
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
  expandToggles: afterExpand.dataView === expandWanted,
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
