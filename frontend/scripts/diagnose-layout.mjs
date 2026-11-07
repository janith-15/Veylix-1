import { profileDir } from './chrome-profile.mjs'
/**
 * Layout diagnostics via Chrome DevTools Protocol (no npm dependencies).
 * Usage:  node scripts/diagnose-layout.mjs [url]
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import fs from 'node:fs'

const CHROME = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const URL = process.argv[2] ?? 'http://localhost:5173/dashboard'
const PORT = 9333
const PROFILE = profileDir('diagnose-profile')

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=${PROFILE}`,
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-gpu',
    '--window-size=1400,900',
    'about:blank',
  ],
  { stdio: 'ignore', detached: false },
)

async function getTargets() {
  for (let i = 0; i < 40; i++) {
    try {
      const res = await fetch(`http://127.0.0.1:${PORT}/json/list`)
      const list = await res.json()
      const page = list.find((t) => t.type === 'page')
      if (page) return page
    } catch {
      /* not up yet */
    }
    await sleep(250)
  }
  throw new Error('Chrome CDP endpoint never became available')
}

function cdp(ws) {
  let id = 0
  const pending = new Map()
  ws.addEventListener('message', (ev) => {
    const msg = JSON.parse(ev.data)
    if (msg.id && pending.has(msg.id)) {
      const { resolve, reject } = pending.get(msg.id)
      pending.delete(msg.id)
      if (msg.error) reject(new Error(msg.error.message))
      else resolve(msg.result)
    }
  })
  return (method, params = {}) =>
    new Promise((resolve, reject) => {
      const mid = ++id
      pending.set(mid, { resolve, reject })
      ws.send(JSON.stringify({ id: mid, method, params }))
    })
}

const PROBE = `(() => {
  const pick = (sel) => {
    const el = document.querySelector(sel)
    if (!el) return { sel, missing: true }
    const r = el.getBoundingClientRect()
    const s = getComputedStyle(el)
    return {
      sel,
      x: Math.round(r.x), y: Math.round(r.y),
      w: Math.round(r.width), h: Math.round(r.height),
      cls: el.className,
      position: s.position, left: s.left, marginLeft: s.marginLeft,
      paddingLeft: s.paddingLeft,
      transform: s.transform,
      filter: s.filter,
      drawerClosed: el.getAttribute('data-drawer-closed'),
      isDesktop: el.getAttribute('data-is-desktop'),
    }
  }
  const root = getComputedStyle(document.documentElement)
  return {
    url: location.pathname,
    viewport: { w: window.innerWidth, dpr: window.devicePixelRatio },
    mqDesktop: window.matchMedia('(min-width: 1024px)').matches,
    dataShell: document.documentElement.dataset.shell,
    dataTheme: document.documentElement.dataset.theme,
    tokens: {
      sidebarWidth: root.getPropertyValue('--sidebar-width').trim(),
      sidebarOffset: root.getPropertyValue('--sidebar-offset').trim(),
      mainOffset: root.getPropertyValue('--main-offset').trim(),
    },
    boxes: ['.app-shell', '.app-sidebar', '.app-main', '.app-content', '.app-topbar'].map(pick),
  }
})()`

try {
  const target = await getTargets()
  const ws = new WebSocket(target.webSocketDebuggerUrl)
  await new Promise((res, rej) => {
    ws.addEventListener('open', res, { once: true })
    ws.addEventListener('error', rej, { once: true })
  })
  const send = cdp(ws)

  const errors = []
  await send('Runtime.enable')
  await send('Page.enable')
  await send('Log.enable')
  ws.addEventListener('message', (ev) => {
    const m = JSON.parse(ev.data)
    if (m.method === 'Runtime.exceptionThrown') {
      errors.push(m.params.exceptionDetails?.exception?.description ?? 'exception')
    }
    if (m.method === 'Runtime.consoleAPICalled' && m.params.type === 'error') {
      errors.push(m.params.args.map((a) => a.value ?? a.description ?? '').join(' '))
    }
  })

  /* Sign in through the real UI so every store hydrates naturally. */
  await send('Page.navigate', { url: 'http://localhost:5173/login' })
  await sleep(3000)
  await send('Runtime.evaluate', {
    expression: `(() => {
      const email = document.querySelector('input[type="email"]')
      const pass = document.querySelector('input[type="password"]')
      const setVal = (el, v) => {
        const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
        setter.call(el, v)
        el.dispatchEvent(new Event('input', { bubbles: true }))
      }
      setVal(email, 'owner@veylix.app')
      setVal(pass, 'demo1234')
      const form = document.querySelector('form')
      form.requestSubmit()
      return 'submitted'
    })()`,
    returnByValue: true,
  })
  await sleep(3000)
  const where = await send('Runtime.evaluate', { expression: 'location.pathname', returnByValue: true })
  console.log('after login →', where.result?.value)

  await send('Page.navigate', { url: URL })
  await sleep(3500)

  const results = {}
  for (const width of [1400, 1100, 900, 700, 420]) {
    await send('Emulation.setDeviceMetricsOverride', {
      width,
      height: 900,
      deviceScaleFactor: 1,
      mobile: false,
    })
    await sleep(600)
    const { result } = await send('Runtime.evaluate', { expression: PROBE, returnByValue: true })
    results[width] = result.value
  }

  const shot = await send('Page.captureScreenshot', { format: 'png' })
  fs.writeFileSync('diagnose-layout.png', Buffer.from(shot.data, 'base64'))

  console.log(JSON.stringify({ results, errors }, null, 2))
  ws.close()
} finally {
  chrome.kill()
}
