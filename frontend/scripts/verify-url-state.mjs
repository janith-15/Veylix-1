/**
 * VEYLIX — filters must live in the URL (D-051).
 *
 *   node scripts/verify-url-state.mjs
 *
 * Proves the thing that was broken on all eight list screens: set a filter, then
 * read `location.search`. A refresh and a pasted link then give the same view,
 * which is what local useState could never do.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9381)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const PATH = process.argv[2] ?? '/clients'
const ROLE = process.env.VEYLIX_URL_ROLE ?? 'super_admin'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('url')}`, '--no-first-run',
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
  } catch { /* not up yet */ }
}
if (!ws) {
  console.error('could not attach to chrome')
  process.exit(1)
}

const consoleErrors = []
await send('Runtime.enable')
await send('Page.enable')
await send('Network.enable')
ws.addEventListener('message', (m) => {
  const msg = JSON.parse(m.data)
  if (msg.method === 'Runtime.exceptionThrown') {
    consoleErrors.push('EXCEPTION ' + (msg.params?.exceptionDetails?.exception?.description ?? ''))
  }
})

const session = await signIn(ROLE)
if (session.status !== 200) {
  console.error(`${ROLE} sign-in failed: ${session.status}`)
  process.exit(1)
}
const c = cookieParts(session.cookie)
await send('Network.setCookie', { name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true })

const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  return r?.result?.value
}

// 1. A clean visit must have a clean URL.
await send('Page.navigate', { url: `${APP}${PATH}` })
await sleep(1800)
const cleanSearch = await evaluate('location.search')

// 2. Choose a status-like filter — a <select> applies immediately (not debounced).
//    Different pages have different first selects, so this picks the first one with
//    more than one option rather than assuming a "SUSPENDED" status exists.
const applied = await evaluate(`(() => {
  const sels = [...document.querySelectorAll('select')].filter((s) => s.options.length > 1)
  const sel = sels[0]
  if (!sel) return 'no-select'
  const target = [...sel.options].find((o) => o.value !== '')
  if (!target) return 'no-option'
  window.__filterLabel = target.value
  sel.value = target.value
  sel.dispatchEvent(new Event('change', { bubbles: true }))
  return target.value
})()`)
await sleep(900)
const afterFilter = await evaluate('location.search')

// 3. Type into the search box — it is debounced, so the URL follows after a pause.
const typed = await evaluate(`(() => {
  const input = [...document.querySelectorAll('input[type="text"], input:not([type])')]
    .find((i) => (i.closest('.cl-filters, form') && !i.type.match(/date|number|checkbox/)))
  if (!input) return 'no-search-input'
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
  setter.call(input, 'kasun')
  input.dispatchEvent(new Event('input', { bubbles: true }))
  return 'ok'
})()`)
const immediateSearch = await evaluate('location.search')
await sleep(900) // past the 300 ms debounce
const afterTyping = await evaluate('location.search')

// 4. A reload must reproduce the filtered view, not the unfiltered one.
await send('Page.reload')
await sleep(1800)
const afterReload = await evaluate('location.search')
const clearOffered = await evaluate(
  `!![...document.querySelectorAll('button')].find((b) => b.textContent.includes('Clear filters'))`,
)

const out = {
  path: PATH,
  role: ROLE,
  cleanSearch,
  applied,
  afterFilter,
  typed,
  immediateSearch,
  afterTyping,
  afterReload,
  clearOffered,
  consoleErrors,
}
console.log(JSON.stringify(out, null, 2))

ws.close()
chrome.kill()

const ok =
  cleanSearch === '' &&
  applied !== 'no-select' &&
  applied !== 'no-option' &&
  afterFilter.includes('=') &&
  typed === 'ok' &&
  immediateSearch === afterFilter && // debounced: not yet written
  afterTyping.includes('q=kasun') &&
  afterReload === afterTyping &&
  clearOffered === true
process.exit(ok ? 0 : 1)
