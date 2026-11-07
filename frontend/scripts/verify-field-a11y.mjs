import { profileDir } from './chrome-profile.mjs'
/**
 * VEYLIX — a field's error must be announced WITH the field (D-053).
 *
 *   node scripts/verify-field-a11y.mjs /login
 *
 * `aria-invalid` says "wrong" and nothing else; the reason used to sit in an
 * unreferenced <span>. This asserts the rendered DOM links them: the input's
 * `aria-describedby` resolves to the element that actually holds the message.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9401)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const PATH = process.argv[2] ?? '/login'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('a11y')}`, '--no-first-run',
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

await send('Runtime.enable')
await send('Page.enable')
await send('Page.navigate', { url: `${APP}${PATH}` })
await sleep(1800)

const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  return r?.result?.value
}

// Trigger the form's own validation error: submit an empty sign-in form.
await evaluate(`(() => {
  const form = document.querySelector('form')
  if (form) form.requestSubmit()
  return true
})()`)
await sleep(900)

const out = await evaluate(`(() => {
  const fields = [...document.querySelectorAll('input, textarea, select')]
  const report = fields.map((el) => {
    const ids = (el.getAttribute('aria-describedby') || '').split(/\\s+/).filter(Boolean)
    const targets = ids.map((id) => {
      const t = document.getElementById(id)
      return t ? t.textContent.trim() : null
    })
    return {
      type: el.tagName.toLowerCase(),
      invalid: el.getAttribute('aria-invalid'),
      describedBy: ids,
      resolved: targets,
      broken: targets.some((t) => t === null),
    }
  })
  const described = report.filter((r) => r.describedBy.length > 0)
  return {
    fieldCount: report.length,
    describedCount: described.length,
    brokenRefs: report.filter((r) => r.broken).length,
    sample: described.slice(0, 3),
  }
})()`)

console.log(JSON.stringify(out, null, 2))
ws.close()
chrome.kill()

const ok = out.describedCount > 0 && out.brokenRefs === 0
process.exit(ok ? 0 : 1)
