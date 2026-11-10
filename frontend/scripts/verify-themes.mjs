#!/usr/bin/env node
import { profileDir } from './chrome-profile.mjs'
/**
 * Theme matrix verification: for each preset, apply it, then measure the
 * shell contract (sidebar inset, content offset, page padding).
 * Usage: node scripts/verify-themes.mjs
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'

const CHROME = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = 9336
const BASE = 'http://localhost:5173'

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    '--user-data-dir=' + profileDir('themes'),
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

await send('Runtime.enable')
await send('Page.enable')

/* sign in */
await send('Page.navigate', { url: `${BASE}/login` })
await sleep(3000)
await send('Runtime.evaluate', {
  expression: `(() => {
    const set = (el, v) => { Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(el,v); el.dispatchEvent(new Event('input',{bubbles:true})) }
    set(document.querySelector('input[type="email"]'), 'owner@veylix.app')
    set(document.querySelector('input[type="password"]'), 'demo1234')
    document.querySelector('form').requestSubmit()
  })()`,
})

const THEMES = ['modern', 'dark', 'light', 'classic', 'gaming', 'hacking']
const results = {}

for (const theme of THEMES) {
  await send('Page.navigate', { url: `${BASE}/dashboard` })
  await sleep(2500)
  await send('Runtime.evaluate', {
    expression: `localStorage.setItem('veylix-theme', JSON.stringify({state:{settings:{...JSON.parse(localStorage.getItem('veylix-theme')||'{"state":{"settings":{}}}').state.settings}},version:5}))`,
  })
  const out = await send('Runtime.evaluate', {
    expression: `(async () => {
      const store = (await import('/src/theme/themeStore.ts')).useThemeStore
      store.getState().draftSetPreset('${theme}')
      store.getState().saveTheme()
      return 'ok'
    })()`,
    awaitPromise: true,
    returnByValue: true,
  })
  await sleep(1200)
  const probe = await send('Runtime.evaluate', {
    expression: `(() => {
      const g = (s) => document.querySelector(s)
      const box = (el) => { if (!el) return null; const r = el.getBoundingClientRect(); return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) } }
      const cs = (el, p) => el ? getComputedStyle(el)[p] : null
      const shell = g('.app-sidebar'), main = g('.app-main'), content = g('.app-content'), top = g('.app-topbar'), card = g('.ui-card')
      return {
        theme: document.documentElement.dataset.theme,
        ui: document.documentElement.dataset.ui,
        shellAttr: document.documentElement.dataset.shell,
        offset: getComputedStyle(document.documentElement).getPropertyValue('--sidebar-offset').trim(),
        sidebarPos: cs(shell,'position'),
        sidebar: box(shell), main: box(main), content: box(content), topbar: box(top),
        contentPad: cs(content,'padding'),
        cardRadius: cs(card,'borderRadius'),
        btnRadius: cs(g('.ui-btn'),'borderRadius'),
        overflowX: document.documentElement.scrollWidth > window.innerWidth,
      }
    })()`,
    returnByValue: true,
  })
  results[theme] = { store: out?.result?.value, ...probe.result.value }
}

console.log(JSON.stringify(results, null, 2))
ws.close()
chrome.kill()
