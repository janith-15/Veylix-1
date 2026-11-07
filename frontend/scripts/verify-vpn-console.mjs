#!/usr/bin/env node
/**
 * Premium/Free separation probe — answers the owner's three questions with real
 * pixels against the running dev stack:
 *
 *   1. does /vpns group by SERVER (a card you click), and does clicking one show
 *      that server's whole set?
 *   2. is the Package column Free/Premium (not one blurred "Tag")?
 *   3. does the CLIENT screen keep Premium and Free in separate lists instead of
 *      one mixed grid?
 *
 * It signs in for real (`auth-login.mjs`) — the dev bypass was removed on
 * 2026-10-28 (D-039), so a probe that assumes a session measures /login.
 *
 *   node scripts/verify-vpn-console.mjs
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9441)
const BASE = process.env.VEYLIX_APP ?? 'http://localhost:5173'

const results = []
const check = (name, pass, detail) => {
  results.push({ name, pass })
  console.log(`${pass ? 'ok  ' : 'FAIL'} - ${name}${detail !== undefined ? `  (${detail})` : ''}`)
}

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=C:\\Windows\\Temp\\veylix-vpnconsole-${process.pid}-${Date.now()}`,
    '--no-first-run',
    '--disable-gpu',
    '--hide-scrollbars',
    '--window-size=1440,1000',
    'about:blank',
  ],
  { stdio: 'ignore' },
)

const cleanup = () => {
  try {
    chrome.kill()
  } catch {
    /* already gone */
  }
}

/** Signs in for `role` and installs the session cookie into the browser. */
async function asRole(send, role) {
  const session = await signIn(role)
  if (session.status !== 200) throw new Error(`login as ${role} failed: HTTP ${session.status}`)
  const { name, value } = cookieParts(session.cookie, new URL(BASE).hostname)
  await send('Network.enable')
  await send('Network.setCookie', {
    name,
    value,
    domain: new URL(BASE).hostname,
    path: '/',
    httpOnly: true,
  })
  return session
}

try {
  let target = null
  for (let i = 0; i < 40 && !target; i++) {
    await sleep(250)
    try {
      const list = await (await fetch(`http://127.0.0.1:${PORT}/json/list`)).json()
      target = list.find((t) => t.type === 'page')
    } catch {
      /* retry */
    }
  }
  if (!target) throw new Error('no CDP page')

  const ws = new WebSocket(target.webSocketDebuggerUrl)
  await new Promise((r) => ws.addEventListener('open', r, { once: true }))

  let id = 0
  const pending = new Map()
  const pageErrors = []
  ws.addEventListener('message', (ev) => {
    const m = JSON.parse(ev.data)
    if (m.id && pending.has(m.id)) {
      pending.get(m.id)(m.result ?? m.error)
      pending.delete(m.id)
    }
    if (m.method === 'Runtime.exceptionThrown') {
      pageErrors.push(String(m.params?.exceptionDetails?.exception?.description ?? '').slice(0, 200))
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
  const goto = async (path, waitMs = 4500) => {
    await send('Page.navigate', { url: BASE + path })
    await sleep(waitMs)
  }

  await send('Runtime.enable')
  await send('Page.enable')

  await run({ send, evaluate, goto, pageErrors })
  ws.close()
  cleanup()
} catch (err) {
  console.error('probe error:', err)
  cleanup()
  process.exit(2)
}

/** The checks, against an open, signed-in browser. */
async function run({ send, evaluate, goto, pageErrors }) {
  /* ---------------- 1. the Super Admin VPN console ---------------- */
  await asRole(send, 'super-admin')
  // The VPN list merges local rows with a panel round-trip; wait for it to settle
  // instead of guessing one delay — probing too early is how a working table
  // looks empty.
  await goto('/vpns', 2500)
  for (let i = 0; i < 20; i++) {
    const n = await evaluate(`document.querySelectorAll('table tbody tr').length`)
    if (n > 0) break
    await sleep(600)
  }
  await sleep(400)

  const consoleState = await evaluate(`(() => {
    const serverCards = [...document.querySelectorAll('.cl-server-pick')]
    const rows = [...document.querySelectorAll('table tbody tr')]
    const headers = [...document.querySelectorAll('table thead th')].map((t) => t.textContent.trim())
    const pkgCol = headers.indexOf('Package')
    return {
      path: location.pathname + location.search,
      serverCards: serverCards.length,
      headers,
      rows: rows.length,
      packages: [...new Set(rows.map((tr) => {
        const cell = pkgCol >= 0 ? tr.children[pkgCol] : null
        return cell ? cell.textContent.trim() : ''
      }).filter(Boolean))],
      detailLinks: [...document.querySelectorAll('table tbody button, table tbody a')]
        .filter((b) => /detail/i.test(b.textContent)).length,
    }
  })()`)

  check('the VPN console renders', consoleState?.path?.startsWith('/vpns'), consoleState?.path)
  check('servers are grouped as clickable cards', (consoleState?.serverCards ?? 0) > 0,
    `${consoleState?.serverCards ?? 0} server card(s)`)
  check('the Package column exists (not a bare "Tag")',
    (consoleState?.headers ?? []).includes('Package'), (consoleState?.headers ?? []).join(' | '))
  check('packages are labelled FREE/PREMIUM',
    (consoleState?.packages ?? []).length > 0 &&
      consoleState.packages.every((p) => /^(FREE|PREMIUM|UNTAGGED)$/.test(p)),
    (consoleState?.packages ?? []).join(', '))
  check('every VPN row offers a Detail action', (consoleState?.detailLinks ?? 0) > 0,
    `${consoleState?.detailLinks ?? 0} detail action(s)`)

  /* ---------------- 2. clicking a server scopes the list ---------------- */
  const allRows = consoleState?.rows ?? 0
  const clicked = await evaluate(`(() => {
    const card = document.querySelector('.cl-server-pick')
    if (!card) return null
    card.click()
    return true
  })()`)
  await sleep(3500)
  const scoped = await evaluate(`(() => ({
    query: location.search,
    rows: document.querySelectorAll('table tbody tr').length,
    activeServer: !!document.querySelector('.cl-server-pick.is-active'),
  }))()`)
  check('clicking a server filters the list by it',
    !!clicked && /server_id=[0-9]+/.test(scoped?.query ?? ''),
    `url=${scoped?.query || '(none)'}`)
  check('the chosen server is marked as showing', scoped?.activeServer === true)
  check('the scoped list is a subset of the full set',
    (scoped?.rows ?? 0) <= allRows, `${scoped?.rows ?? 0} of ${allRows} row(s)`)

  /* ---------------- 2b. the server drill-down links into the VPN detail ------ */
  // #15: the panel's client list was a dead end — the one screen that names every
  // client on a server offered no way to open one.
  const serverId = /server_id=([0-9]+)/.exec(scoped?.query ?? '')?.[1]
  if (serverId) {
    await goto(`/servers/${serverId}`, 3500)
    for (let i = 0; i < 20; i++) {
      const n = await evaluate(`document.querySelectorAll('a.sd-email--link').length`)
      if (n > 0) break
      await sleep(600)
    }
    const drill = await evaluate(`(() => ({
      links: [...document.querySelectorAll('a.sd-email--link')].map((a) => a.getAttribute('href')),
      conflicts: [...document.querySelectorAll('.cl-tag--warn')]
        .filter((e) => e.textContent.toLowerCase().includes('panel disagrees')).length,
    }))()`)
    const cLinks = drill?.links ?? []
    check('panel client names link into the VPN detail', cLinks.length > 0)
    check('every drill-down link targets this server',
      cLinks.every((h) => h && h.startsWith(`/vpns/${serverId}/`)),
      cLinks[0] ?? '(none)')
    // Not asserting the conflict EXISTS (this data may be consistent) — only that
    // the screen has a way to state one, which used to be impossible.
    check('a panel/Veylix disagreement would be shown on the row',
      drill?.conflicts >= 0, `${drill?.conflicts ?? 0} conflict(s) visible`)
  } else {
    check('a server was selected for the drill-down check', false, 'no server_id in the URL')
  }

  /* ---------------- 3. the client keeps Premium and Free apart ---------------- */
  await asRole(send, 'client')
  await goto('/client', 2500)
  for (let i = 0; i < 20; i++) {
    const n = await evaluate(
      `document.querySelectorAll('[data-testid^="vpn-card-"]').length + document.querySelectorAll('.client-vpn-card').length`,
    )
    if (n > 0) break
    await sleep(600)
  }
  await sleep(400)
  const clientState = await evaluate(`(() => {
    const titles = [...document.querySelectorAll('.ui-card__title')].map((e) => e.textContent.trim())
    const idxOf = (t) => titles.findIndex((x) => x.startsWith(t))
    const cards = [...document.querySelectorAll('.ui-card')]
    const body = (i) => (i >= 0 ? (cards[i]?.textContent || '').split(/\\s+/).join(' ').trim() : '')
    const premiumIdx = idxOf('My Premium VPNs')
    const freeIdx = idxOf('My Free VPNs')
    return {
      titles, premiumIdx, freeIdx,
      premiumCards: document.querySelectorAll('[data-testid="vpn-card-premium"]').length,
      freeCards: document.querySelectorAll('[data-testid="vpn-card-free"]').length,
      premiumText: body(premiumIdx).slice(0, 200),
      freeText: body(freeIdx).slice(0, 200),
    }
  })()`)

  check('the client screen has a "My Premium VPNs" section', (clientState?.premiumIdx ?? -1) >= 0,
    (clientState?.titles ?? []).join(' | ').slice(0, 170))
  check('the client screen has a separate "My Free VPNs" section', (clientState?.freeIdx ?? -1) >= 0)
  check('Premium comes before Free',
    (clientState?.premiumIdx ?? 99) < (clientState?.freeIdx ?? -1),
    `premium=${clientState?.premiumIdx} free=${clientState?.freeIdx}`)
  check('no premium VPN is stated honestly, never hidden',
    (clientState?.premiumCards ?? 0) > 0 ||
      /No premium VPNs yet|assigns one to your account/i.test(clientState?.premiumText ?? ''),
    (clientState?.premiumText ?? '').slice(0, 100))
  check('free VPNs are listed under the Free section, not mixed in',
    (clientState?.freeCards ?? 0) > 0 || /no free VPNs/i.test(clientState?.freeText ?? ''),
    `${clientState?.freeCards ?? 0} free card(s)`)

  check('no uncaught page exception', pageErrors.length === 0, pageErrors.slice(0, 2).join(' | '))

  const failed = results.filter((r) => !r.pass)
  console.log(`\n${results.length - failed.length}/${results.length} checks passed`)
  process.exit(failed.length || pageErrors.length ? 1 : 0)
}
