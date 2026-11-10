/**
 * Render the profile page in headless Chrome and report what is actually in the
 * DOM: the three tabs, every field label the owner asked for, and console errors.
 * A screenshot-free structural check, because a page that renders but has lost a
 * field looks perfect in an image.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9341)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const ROLE = process.env.VEYLIX_DEV_ROLE ?? 'client'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  '--user-data-dir=' + profileDir('profile-check'), '--no-first-run',
  '--disable-gpu', '--window-size=1440,1000', 'about:blank',
], { stdio: 'ignore' })

let ws
const pending = new Map()
const send = (method, params = {}) =>
  new Promise((res, rej) => {
    const id = Math.floor(Math.random() * 1e6)
    pending.set(id, { res, rej })
    ws.send(JSON.stringify({ id, method, params }))
  })

for (let i = 0; i < 40; i++) {
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
if (!ws) { console.error('could not attach to chrome'); process.exit(1) }

const consoleErrors = []
await send('Runtime.enable')
await send('Page.enable')
ws.addEventListener('message', (m) => {
  const msg = JSON.parse(m.data)
  if (msg.method === 'Runtime.exceptionThrown') consoleErrors.push('EXCEPTION ' + (msg.params?.exceptionDetails?.text ?? ''))
})

const session = await signIn(ROLE)
if (session.status !== 200) { console.error(`login failed: ${session.status}`); process.exit(1) }
const host = new URL(APP).hostname
const { name, value } = cookieParts(session.cookie, host)
await send('Network.enable', {})
await send('Network.setCookie', { name, value, domain: host, path: '/', httpOnly: true })

await send('Page.navigate', { url: `${APP}/profile` })
await sleep(3500)

const probe = await send('Runtime.evaluate', {
  returnByValue: true,
  expression: `(() => {
    const labels = [...document.querySelectorAll('label, .ui-input__label')].map(e => e.textContent.trim()).filter(Boolean)
    const tabs = [...document.querySelectorAll('[role="tab"]')].map(e => e.textContent.trim())
    return JSON.stringify({ title: document.title, tabs, labels,
      hasAvatar: !!document.querySelector('.prof-avatar-img') || !!document.querySelector('.app-avatar'),
      heading: document.querySelector('.ui-page-head__title')?.textContent?.trim() ?? null })
  })()`,
})
const out = JSON.parse(probe.result.value)
console.log(JSON.stringify({ ...out, consoleErrors }, null, 2))
ws.close()
chrome.kill()
