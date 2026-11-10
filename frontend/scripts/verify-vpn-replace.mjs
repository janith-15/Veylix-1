#!/usr/bin/env node
/**
 * VEYLIX — "Replace VPN" (D-112), proven through the UI the customer uses.
 *
 *   node scripts/verify-vpn-replace.mjs
 *
 * Why this exists: D-112 shipped `provisioning.Replace()`, its route and a
 * Replace button, proven by 7 Go tests plus one live curl. None of that proves
 * the SCREEN works — the button could be wired to nothing, the confirm dialog
 * could never open, or the outcome dialog could claim a VPN that was never
 * built. This gate drives the real page.
 *
 * SAFETY — this gate does NOT spend a claim. It stops at the confirm dialog and
 * cancels:
 *
 *   1. the account owns a VPN at all (a green run on an empty account proves
 *      nothing, so it REFUSES with exit 3 rather than passing vacuously)
 *   2. a Replace control is offered on that VPN's page
 *   3. it opens a confirmation that explains D-112's safety order in words
 *   4. cancelling closes it and changes nothing
 *
 * A destructive run would be a separate, deliberate choice: replacing a real
 * customer VPN to satisfy a test is the write the data-safety rules forbid (see
 * VPN_FIX_LEDGER.md — a verification script once deleted live data, twice).
 *
 * READ-ONLY against the app: it loads a page, clicks controls and cancels.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9467)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const ROLE = process.env.VEYLIX_ROLE ?? 'client'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('vpnreplace')}`, '--no-first-run',
  '--disable-gpu', '--window-size=1440,1400', 'about:blank',
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

const s = await signIn(ROLE)
if (s.status !== 200) {
  console.error(`${ROLE} sign-in failed: ${s.status}`)
  process.exit(1)
}
await send('Network.clearBrowserCookies')
const c = cookieParts(s.cookie)
await send('Network.setCookie', { name: c.name, value: c.value, domain: c.domain, path: '/', httpOnly: true })

const failures = []
// Signature is (ok, msg, extra) — the same order every other verify script uses.
const check = (ok, msg, extra) => {
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${msg}${extra === undefined ? '' : `  -> ${JSON.stringify(extra)}`}`)
  if (!ok) failures.push(msg)
}

const sleepFor = async (expr, timeoutMs = 12000) => {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (await evaluate(expr)) return true
    await sleep(200)
  }
  return false
}


/* ---- 1. the account must own at least one VPN, or this proves nothing ------
 *
 * The cards are buttons, not links (`vpn-card-<id>`), and each carries its own
 * `vpn-replace-<id>` control. Reading the card ids is also how this gate finds
 * the Replace button — there is no anchor to follow.
 */
await send('Page.navigate', { url: `${APP}/client/vpns` })
await sleepFor(`!!document.querySelector('[data-testid^="vpn-card-"]')`)

const owned = await evaluate(`(() => {
  const cards = [...document.querySelectorAll('[data-testid^="vpn-card-"]')]
  const ids = cards.map((el) => el.getAttribute('data-testid').replace('vpn-card-', ''))
  return {
    count: ids.length,
    ids,
    firstId: ids[0] ?? null,
    hasReplace: ids.map((id) => !!document.querySelector('[data-testid="vpn-replace-' + id + '"]')),
    path: location.pathname,
  }
})()`)
check(
  (owned?.count ?? 0) > 0,
  'D-112 the account owns at least one VPN (a green run on an empty account proves nothing)',
  { count: owned?.count, ids: owned?.ids, path: owned?.path },
)
if ((owned?.count ?? 0) === 0) {
  console.log('\nREFUSING: nothing to replace on this account — the gate would be vacuous.\n')
  chrome.kill()
  process.exit(3)
}

check(
  (owned?.hasReplace ?? []).every(Boolean),
  'D-112 every owned VPN offers a Replace control',
  { perCard: owned?.hasReplace },
)

/* ---- 2. the Replace button on the first card ------------------------------ */
const firstReplace = `[data-testid="vpn-replace-${owned.firstId}"]`
const control = await evaluate(`(() => {
  const btn = document.querySelector('${firstReplace}')
  if (!btn) return { found: false }
  return { found: true, text: (btn.textContent || '').trim(), disabled: !!btn.disabled }
})()`)
check(!!control?.found, 'D-112 a Replace control is offered on the VPN card', control)

/* ---- 3. it opens a confirmation that explains the safety order ------------ */
if (control?.found) {
  await evaluate(`(() => {
    const btn = document.querySelector('${firstReplace}')
    if (btn) btn.click()
    return true
  })()`)
  const dialogOpened = await sleepFor(`!!document.querySelector('[data-testid="vpn-replace-dialog"]')`)
  check(dialogOpened, 'D-112 Replace opens a confirmation before acting')

  const dialog = await evaluate(`(() => {
    const el = document.querySelector('[data-testid="vpn-replace-dialog"]')
    if (!el) return null
    const text = (el.textContent || '').replace(/\\s+/g, ' ').trim()
    return {
      text: text.slice(0, 400),
      // The safety order D-112 is built on: the OLD one is released only once
      // the NEW one exists. The dialog must say so, in words — never just imply
      // it with a disabled button.
      explainsOrder: /new[^.]*\\bbefore\\b[^.]*(release|released)/i.test(text),
      hasCancel: !!el.closest('[role="dialog"], dialog')?.querySelector('button'),
      oldIdentity: el.querySelector('[data-testid="vpn-replace-old"]')?.textContent?.trim() ?? null,
      claimNote: !!el.querySelector('[data-testid="vpn-replace-claim-note"]'),
    }
  })()`)
  check(
    !!dialog?.explainsOrder,
    'D-112 the confirmation explains the new-before-old order',
    dialog ? dialog.text.slice(0, 240) : 'no dialog',
  )
  check(
    !!dialog?.oldIdentity,
    'D-112 the confirmation names the VPN being replaced (D-091.4)',
    { oldIdentity: dialog?.oldIdentity },
  )

  /* ---- 4. cancelling changes nothing --------------------------------------
   * "Changes nothing" means the SAME card ids are still there afterwards. A
   * replace that went through would have released one and created another, so
   * the id set would differ — that is the check, not just "a dialog closed".
   */
  const before = await evaluate(`[...document.querySelectorAll('[data-testid^="vpn-card-"]')]
    .map((el) => el.getAttribute('data-testid')).sort().join(',')`)
  const dismissed = await evaluate(`(() => {
    const el = document.querySelector('[data-testid="vpn-replace-dialog"]')?.closest('[role="dialog"], dialog') ||
               document.querySelector('[role="dialog"], dialog')
    if (!el) return 'no-dialog'
    const btn = [...el.querySelectorAll('button')].find((b) => /^cancel$/i.test((b.textContent || '').trim()))
    if (!btn) return 'no-cancel'
    btn.click()
    return 'clicked'
  })()`)
  await sleep(900)
  const after = await evaluate(`(() => ({
    dialogOpen: !!document.querySelector('[data-testid="vpn-replace-dialog"]'),
    path: location.pathname,
    cards: [...document.querySelectorAll('[data-testid^="vpn-card-"]')]
      .map((el) => el.getAttribute('data-testid')).sort().join(','),
  }))()`)
  check(
    dismissed === 'clicked' && after?.dialogOpen === false && after?.cards === before,
    'D-112 cancelling closes the dialog and changes nothing',
    { dismissed, path: after?.path, cardsUnchanged: after?.cards === before, before, after: after?.cards },
  )
}

/* ---- report --------------------------------------------------------------- */
console.log(
  `\nVPN replace — ${failures.length === 0 ? 'OK' : `${failures.length} FAILED`}` +
    ` (${failures.length === 0 ? 'all checks passed' : failures.join(' | ')})\n`,
)
chrome.kill()
process.exit(failures.length === 0 ? 0 : 1)
