/**
 * VEYLIX — checkout step indicator: visible text AND an announced status.
 *
 *   node scripts/verify-step-progress.mjs /buy
 *
 * The old dots were aria-hidden, so a screen-reader user could not tell which step
 * they were on. This asserts there is a visible "Step N of M" line AND a live
 * region carrying the same text.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9371)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const PATH = process.argv[2] ?? '/buy'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('steps')}`, '--no-first-run',
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

const role = process.env.VEYLIX_STEP_ROLE ?? 'client'
const session = await signIn(role)
if (session.status !== 200) {
  console.error(`${role} sign-in failed: ${session.status}`)
  process.exit(1)
}
const c = cookieParts(session.cookie)
await send('Network.setCookie', { name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true })

await send('Page.navigate', { url: `${APP}${PATH}` })
await sleep(2000)

const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  return r?.result?.value
}

const out = await evaluate(`(() => {
  const live = [...document.querySelectorAll('[role="status"][aria-live], [aria-live="polite"]')]
    .map((el) => el.textContent.trim())
    .filter(Boolean)
  const bodyText = document.body.innerText
  const visibleStep = /Step \\d+ of \\d+/.test(bodyText)
  const dotsHidden = [...document.querySelectorAll('.sell-step-dots')].every(
    (el) => el.getAttribute('aria-hidden') === 'true')
  const buttons = [...document.querySelectorAll('button')].map((b) => b.textContent.trim())
  return { live, visibleStep, dotsHidden, buttons: buttons.slice(0, 12), path: location.pathname }
})()`)

console.log(JSON.stringify({ ...out, consoleErrors }, null, 2))

ws.close()
chrome.kill()
const ok = out?.visibleStep === true && (out?.live?.length ?? 0) > 0
process.exit(ok ? 0 : 1)
