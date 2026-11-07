/**
 * VEYLIX — a client can open their OWN VPN.
 *
 *   node scripts/verify-client-vpn.mjs <vpnId>
 *
 * `audit-ui.mjs` reports `/vpn/24` as REDIRECT for the seeded client. That is
 * correct — user 15 owns no VPN — but it left the real question unanswered: can
 * a client who DOES own a VPN open it, or does the route bounce them?
 *
 * This signs in as the VPN's actual owner (VEYLIX_EMAIL/VEYLIX_PASSWORD) and
 * asserts the detail page renders instead of redirecting to /client.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9447)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const VPN_ID = Number(process.argv[2] ?? process.env.VEYLIX_VPN_ID ?? 36)

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('clientvpn')}`, '--no-first-run',
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
  } catch {
    /* not up yet */
  }
}
if (!ws) {
  console.error('could not attach to chrome')
  process.exit(1)
}

const evaluate = async (expression) => {
  const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
  return r?.result?.value
}

await send('Runtime.enable')
await send('Page.enable')
await send('Network.enable')

// VEYLIX_EMAIL/PASSWORD identify the VPN's owner; without them this suite has
// nothing meaningful to assert and must say so rather than "pass".
if (!process.env.VEYLIX_EMAIL || !process.env.VEYLIX_PASSWORD) {
  console.error('set VEYLIX_EMAIL and VEYLIX_PASSWORD to the VPN owner\'s credentials')
  process.exit(2)
}

const s = await signIn('client')
if (s.status !== 200) {
  console.error(`client sign-in failed: ${s.status}`)
  process.exit(1)
}
await send('Network.clearBrowserCookies')
const c = cookieParts(s.cookie)
await send('Network.setCookie', { name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true })

await send('Page.navigate', { url: `${APP}/vpn/${VPN_ID}` })
// The detail page reads the store, then the panel. Wait for either outcome.
let landed = ''
let hasDetail = false
for (let i = 0; i < 30; i++) {
  await sleep(400)
  landed = (await evaluate('location.pathname')) ?? ''
  hasDetail = await evaluate(
    `!!document.querySelector('[data-testid="vpn-detail"]') || document.body.innerText.includes('VPN not found')`,
  )
  if (hasDetail || landed !== `/vpn/${VPN_ID}`) break
}

const failures = []
const check = (ok, msg) => {
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${msg}`)
  if (!ok) failures.push(msg)
}

check(landed === `/vpn/${VPN_ID}`, `stays on /vpn/${VPN_ID} (landed: ${landed})`)
check(hasDetail, 'the VPN detail body rendered (not a blank page)')
const body = (await evaluate('document.body.innerText')) ?? ''
check(!body.includes('VPN not found'), 'the owner is not told "VPN not found" about their own VPN')
check(!body.includes('Revoke'), 'a client is never shown operator controls')

await send('Page.navigate', { url: 'about:blank' })
chrome.kill()

console.log('')
if (failures.length) {
  console.log(`✗ ${failures.length} check(s) failed`)
  process.exit(1)
}
console.log('✓ a client can open their own VPN')
