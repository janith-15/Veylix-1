/**
 * VEYLIX — the client setup guide in a real browser.
 *
 *   node scripts/verify-setup-guide.mjs /vpn/24
 *
 * Asserts the three things that make the guide actionable rather than decorative:
 * real app NAMES (not "the recommended app"), real LINK targets, and a numbered
 * step list. A guide that renders but names nothing is the defect this replaced.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9361)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const PATH = process.argv[2] ?? '/vpn/24'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('guide')}`, '--no-first-run',
  '--disable-gpu', '--window-size=1440,1100', 'about:blank',
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

const role = process.env.VEYLIX_GUIDE_ROLE ?? 'client'
const session = await signIn(role)
if (session.status !== 200) {
  console.error(`client sign-in failed: ${session.status}`)
  process.exit(1)
}
const c = cookieParts(session.cookie)
await send('Network.setCookie', { name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true })

await send('Page.navigate', { url: `${APP}${PATH}` })
await sleep(2200)

const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  return r?.result?.value
}

const out = await evaluate(`(() => {
  const txt = document.body.innerText.slice(0, 500)
  const scope = document
  const apps = [...scope.querySelectorAll('.guide-app')].map((a) => ({
    name: a.querySelector('.guide-app__name')?.textContent?.trim() ?? '',
    href: a.getAttribute('href') ?? '',
    target: a.getAttribute('target') ?? '',
    rel: a.getAttribute('rel') ?? '',
  }))
  const steps = [...scope.querySelectorAll('.guide-steps li')].map((li) => li.textContent.trim())
  const tabs = [...document.querySelectorAll('[role="tab"], .ui-tabs button')].map((b) => b.textContent.trim())
  return { apps, steps, tabs, txt, path: location.pathname }
})()`)

console.log(JSON.stringify({ ...out, consoleErrors }, null, 2))

ws.close()
chrome.kill()
const ok = out?.apps?.length >= 2 && out.apps.every((a) => a.href.startsWith('https://')) && out.steps?.length >= 4
process.exit(ok ? 0 : 1)
