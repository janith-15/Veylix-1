/**
 * Chat bubble geometry probe — answers ONE question with real pixels:
 * "is my own message on the right, and is the other person's on the left?"
 *
 * It boots headless Chrome against an ALREADY RUNNING dev server (`.\dev.ps1`)
 * and reads computed styles plus bounding boxes. No writing, no state change.
 *
 *   node scripts/probe-chat-layout.mjs [threadId] [--mobile]
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9351)
const BASE = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const API = process.env.VEYLIX_API ?? 'http://127.0.0.1:8080'
const MOBILE = process.argv.includes('--mobile')
const ROLE = process.env.VEYLIX_DEV_ROLE ?? 'super-admin'
// An explicit thread id may be passed on the command line; otherwise the first
// conversation the signed-in account actually owns is used (see below).
const THREAD_ARG = process.argv.find((a) => /^\d+$/.test(a)) ?? null

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=C:\\Windows\\Temp\\veylix-probe-${process.pid}-${Date.now()}`,
    '--no-first-run',
    '--disable-gpu',
    '--hide-scrollbars',
    `--window-size=${MOBILE ? '390,844' : '1440,900'}`,
    'about:blank',
  ],
  { stdio: 'ignore' },
)

const pageTarget = async () => {
  for (let i = 0; i < 40; i++) {
    await sleep(250)
    try {
      const list = await (await fetch(`http://127.0.0.1:${PORT}/json/list`)).json()
      const page = list.find((t) => t.type === 'page')
      if (page) return page
    } catch {
      /* retry */
    }
  }
  throw new Error('no CDP page — is Chrome installed at ' + CHROME + '?')
}

/**
 * The first conversation this account can actually see, straight from the API.
 *
 * Read over HTTP rather than "whichever id is in the URL": a probe must measure a
 * transcript the viewer is entitled to, and the inbox is the server's own answer
 * to "which conversations exist for me?".
 */
const firstThreadId = async () => {
  const session = await signIn(ROLE)
  if (session.status !== 200) return null
  const res = await fetch(`${API}/api/chat/threads`, {
    headers: { Cookie: session.cookie, Accept: 'application/json' },
  })
  if (res.status !== 200) return null
  const rows = await res.json()
  return Array.isArray(rows) && rows.length > 0 ? String(rows[0].id) : null
}

const main = async () => {
  const page = await pageTarget()

  const ws = new WebSocket(page.webSocketDebuggerUrl)
  await new Promise((r) => ws.addEventListener('open', r, { once: true }))
  let id = 0
  const pending = new Map()
  ws.addEventListener('message', (ev) => {
    const m = JSON.parse(ev.data || '{}')
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
  const run = async (expression) => (await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true }))?.result?.value

  await send('Runtime.enable')
  await send('Page.enable')
  if (MOBILE) {
    await send('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 2, mobile: true })
  }

  /*
   * SIGN IN FIRST — the dev-login bypass that used to make this page open
   * already authenticated was removed on 2026-10-28 (D-039). Without a real
   * session, `/chat/<id>` redirects to `/login`, `.chat-msg` matches nothing and
   * this probe reported "every bubble hugs the correct pane edge" about a page
   * that had no bubbles at all. Same mechanism as `verify-dashboard.mjs`.
   */
  const session = await signIn(ROLE)
  if (session.status !== 200) {
    console.error(`login as ${ROLE} failed: HTTP ${session.status} — seed the accounts first (.\\dev.ps1 -Seed)`)
    process.exit(1)
  }
  const { name, value } = cookieParts(session.cookie, new URL(BASE).hostname)
  await send('Network.enable')
  await send('Network.setCookie', {
    name,
    value,
    domain: new URL(BASE).hostname,
    path: '/',
    httpOnly: true,
  })

  /*
   * Resolve the thread from the account's OWN inbox instead of assuming id 1.
   * Conversation ids are global, not per account: thread 1 belongs to somebody
   * else, so hard-coding it measures a 403/404 page.
   */
  const THREAD = THREAD_ARG ?? (await firstThreadId())
  if (!THREAD) {
    console.error(`no conversation for ${session.email} — open one in the app first, or pass a thread id`)
    process.exit(2)
  }

  await send('Page.navigate', { url: `${BASE}/chat/${THREAD}` })
  await sleep(6000)

  await send('Page.navigate', { url: `${BASE}/chat/${THREAD}` })
  await sleep(6000)
  const report = await run(`(() => {
    const list = document.querySelector('.chat-panel__msg-list')
    if (!list) return { error: 'no message list on ' + location.pathname }
    const rows = [...document.querySelectorAll('.chat-msg')]
    const cs = getComputedStyle
    const box = list.getBoundingClientRect()
    // Compare against the message PANE, not the viewport: the thread list owns
    // the left column, so a viewport-centre test mislabels whole conversations.
    // Tolerance = the pane's own padding + slack; a mirrored bubble is hundreds
    // of px away, so a wrong side can never squeeze under this.
    const padLeft = parseFloat(cs(list).paddingLeft) || 0
    const padRight = parseFloat(cs(list).paddingRight) || 0
    const slack = Math.max(4, box.width * 0.02)
    const snugLeft = padLeft + slack
    const snugRight = padRight + slack
    return {
      path: location.pathname,
      viewport: innerWidth + 'x' + innerHeight,
      paneWidth: Math.round(box.width),
      // The real markup, so "0 mine" can be told apart from an empty bubble.
      // The raw markup, so 0-mine can be told apart from an empty bubble.
      // No regex or escapes below: this string is inside a template literal.
      firstBubbleHtml: rows[0] ? String(rows[0].outerHTML).slice(0, 600) : null,
      viewerEmail: document.querySelector('[data-testid="chat-page"]') ? 'page' : 'no-page',
      overflowX: document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
      rows: rows.map((r) => {
        const body = r.querySelector('.chat-msg__body')
        const b = body.getBoundingClientRect()
        const a = r.querySelector('.chat-msg__actions')
        const mine = r.classList.contains('chat-msg--mine')
        return {
          mine,
          group: (r.classList.contains('chat-msg--first') ? 'first ' : '') + (r.classList.contains('chat-msg--last') ? 'last' : 'middle'),
          text: (r.querySelector('.chat-bubble')?.textContent ?? r.querySelector('.chat-msg__text')?.textContent ?? '').slice(0, 18),
          gapLeft: Math.round(b.left - box.left),
          gapRight: Math.round(box.right - b.right),
          side: mine
            ? (box.right - b.right <= snugRight ? 'RIGHT' : 'MISALIGNED')
            : (b.left - box.left <= snugLeft ? 'LEFT' : 'MISALIGNED'),
          width: Math.round(b.width),
          radius: cs(body).borderRadius,
          toolbar: a ? { position: cs(a).position, opacity: cs(a).opacity, top: Math.round(a.getBoundingClientRect().top - b.top) } : null,
        }
      }),
    }
  })()`)

  console.log(JSON.stringify(report, null, 2))
  /*
   * A probe that measured nothing must never report success.
   *
   * This used to print "every bubble hugs the correct pane edge" whenever the
   * misaligned list was empty — which is also true when there are no bubbles at
   * all, i.e. when the page had bounced to /login. Zero rows is a FAILURE to
   * measure, distinct from a pass.
   */
  if (report.error || (report.rows ?? []).length === 0) {
    console.log(`RESULT: NOTHING MEASURED — ${report.error ?? 'no .chat-msg rendered on ' + report.path}`)
    ws.close()
    process.exitCode = 1
    return
  }
  const bad = report.rows.filter((r) => r.side === 'MISALIGNED')
  console.log(
    bad.length === 0
      ? `RESULT: every bubble hugs the correct pane edge (${report.rows.length} bubbles, ${report.rows.filter((r) => r.mine).length} mine)`
      : `RESULT: ${bad.length} bubble(s) MISALIGNED`,
  )
  if (bad.length > 0) process.exitCode = 1
  ws.close()
}

try {
  await main()
} catch (err) {
  console.error('probe failed:', err?.message ?? err)
} finally {
  try { chrome.kill() } catch { /* ignore */ }
}
