#!/usr/bin/env node
import { profileDir } from './chrome-profile.mjs'
/**
 * Popup stacking verification — the only trustworthy test.
 * For each theme, open every popup and assert that the element at the
 * popup's centre point IS the popup (i.e. it really is on top).
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'

const CHROME = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = 9338
const BASE = 'http://localhost:5173'

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    '--user-data-dir=' + profileDir('popups'),
    '--no-first-run',
    '--disable-gpu',
    '--window-size=1440,900',
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
ws.addEventListener('message', (ev) => {
  const m = JSON.parse(ev.data)
  if (m.id && pending.has(m.id)) {
    pending.get(m.id)(m.result ?? m.error)
    pending.delete(m.id)
  }
})
const send = (method, params = {}) =>
  new Promise((res) => {
    const i = ++id
    pending.set(i, res)
    ws.send(JSON.stringify({ id: i, method, params }))
  })
const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true })
  return r?.result?.value
}
const esc = async () => {
  await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 })
  await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27 })
  await sleep(400)
}

await send('Runtime.enable')
await send('Page.enable')

await send('Page.navigate', { url: `${BASE}/login` })
await sleep(3000)
await evaluate(`(() => {
  const set = (el, v) => { Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(el,v); el.dispatchEvent(new Event('input',{bubbles:true})) }
  set(document.querySelector('input[type="email"]'), 'owner@veylix.app')
  set(document.querySelector('input[type="password"]'), 'demo1234')
  document.querySelector('form').requestSubmit()
})()`)
await sleep(2500)

const PROBE = (selector) => `(() => {
  const el = document.querySelector(${JSON.stringify(selector)})
  if (!el) return { found: false }
  const r = el.getBoundingClientRect()
  const cx = Math.round(r.x + r.width / 2)
  const cy = Math.round(r.y + r.height / 2)
  const hit = document.elementFromPoint(cx, cy)
  return {
    found: true,
    onTop: !!hit && (el.contains(hit) || hit === el),
    hitClass: hit ? String(hit.className).slice(0, 40) : null,
    zIndex: getComputedStyle(el).zIndex,
    rect: { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) },
    fits: r.x >= -1 && r.y >= -1 && r.right <= window.innerWidth + 1 && r.bottom <= window.innerHeight + 1,
  }
})()`

const THEMES = ['modern', 'dark', 'light', 'classic', 'gaming', 'hacking']
const report = {}

for (const theme of THEMES) {
  await send('Page.navigate', { url: `${BASE}/dashboard` })
  await sleep(2200)
  await evaluate(`(async () => {
    const store = (await import('/src/theme/themeStore.ts')).useThemeStore
    store.getState().draftSetPreset('${theme}')
    store.getState().saveTheme()
  })()`)
  await sleep(900)

  const row = {}

  /* 1. Command palette (Ctrl+K) */
  await send('Input.dispatchKeyEvent', { type: 'keyDown', modifiers: 2, key: 'k', code: 'KeyK', windowsVirtualKeyCode: 75 })
  await send('Input.dispatchKeyEvent', { type: 'keyUp', modifiers: 2, key: 'k', code: 'KeyK', windowsVirtualKeyCode: 75 })
  await sleep(800)
  row.palette = await evaluate(PROBE('.palette'))
  await esc()

  /* 2. Notification modal (bell) */
  await evaluate(`document.querySelector('.app-topbar__notif button')?.click()`)
  await sleep(800)
  row.notifications = await evaluate(PROBE('.notif-modal'))
  /* Close by clicking the overlay itself (the modal has no Esc handler yet). */
  await evaluate(`(() => {
    const ov = document.querySelector('.notif-overlay')
    if (ov) ov.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
  })()`)
  await sleep(600)
  row.notifClosed = await evaluate(`!document.querySelector('.notif-overlay')`)

  /* 3. Account dropdown (portal) */
  await evaluate(`document.querySelector('.app-topbar .ui-dropdown > button')?.click()`)
  await sleep(700)
  row.dropdown = await evaluate(PROBE('.ui-dropdown__menu'))
  await evaluate(`document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))`)
  await sleep(400)

  /* 4. Dialog — "Add server" on the Servers page */
  await send('Page.navigate', { url: `${BASE}/servers` })
  await sleep(2200)
  await evaluate(`[...document.querySelectorAll('button')].find((b) => /add server/i.test(b.textContent || ''))?.click()`)
  await sleep(700)
  row.dialog = await evaluate(PROBE('.ui-dialog'))
  await esc()

  /* 5. Theme Studio */
  await send('Page.navigate', { url: `${BASE}/dashboard` })
  await sleep(2200)
  await evaluate(`[...document.querySelectorAll('.app-topbar__icon-btn')].find((b) => /theme studio/i.test(b.getAttribute('aria-label') || ''))?.click()`)
  await sleep(800)
  row.studio = await evaluate(PROBE('.studio'))
  await evaluate(`document.querySelector('.studio-scrim')?.click()`)
  await sleep(400)

  report[theme] = row
}

console.log(JSON.stringify(report, null, 2))
ws.close()
chrome.kill()

