import { profileDir } from './chrome-profile.mjs'
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'

const CHROME = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = 9334
const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    '--user-data-dir=' + profileDir('root-check'),
    '--no-first-run',
    '--disable-gpu',
    '--window-size=1400,900',
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
  throw new Error('no CDP page')
}

const t = await pageTarget()
const ws = new WebSocket(t.webSocketDebuggerUrl)
await new Promise((r) => ws.addEventListener('open', r, { once: true }))

let id = 0
const pending = new Map()
const consoleErrors = []
ws.addEventListener('message', (ev) => {
  const m = JSON.parse(ev.data)
  if (m.id && pending.has(m.id)) {
    pending.get(m.id)(m.result ?? m.error)
    pending.delete(m.id)
  }
  if (m.method === 'Runtime.exceptionThrown') {
    consoleErrors.push('EXC: ' + (m.params.exceptionDetails?.exception?.description ?? '?'))
  }
  if (m.method === 'Runtime.consoleAPICalled') {
    const txt = (m.params.args ?? []).map((a) => a.value ?? a.description ?? '').join(' ')
    if (m.params.type === 'error') consoleErrors.push('CONSOLE ERR: ' + txt)
    else consoleErrors.push(`[${m.params.type}] ` + txt)
  }
})
const send = (method, params = {}) =>
  new Promise((res) => {
    const i = ++id
    pending.set(i, res)
    ws.send(JSON.stringify({ id: i, method, params }))
  })

await send('Runtime.enable')
await send('Page.enable')
await send('Log.enable')

const results = {}
for (const path of ['/', '/dashboard', '/login']) {
  const nav = await send('Page.navigate', { url: 'http://localhost:5173' + path })
  await sleep(3500)
  const ev = await send('Runtime.evaluate', {
    expression: `JSON.stringify({
      href: location.href,
      rootChildren: document.getElementById('root')?.children.length ?? -1,
      shellExists: !!document.querySelector('.app-shell'),
      sidebarExists: !!document.querySelector('.app-sidebar'),
      bodyStart: document.body.innerHTML.slice(0, 300),
      authLS: localStorage.getItem('veylix-auth'),
    })`,
    returnByValue: true,
    awaitPromise: false,
  })
  results[path] = { nav, ev }
}

console.log(JSON.stringify({ results, consoleEvents: consoleErrors.slice(0, 40) }, null, 2))
ws.close()
chrome.kill()
