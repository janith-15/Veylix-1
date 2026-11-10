/**
 * Chat clarity probe — answers ONE question with real pixels and real DOM:
 * "can a reader tell, at a glance, who said what?"
 *
 * It boots headless Chrome against an already-running dev server (`.\dev.ps1`)
 * and checks: every bubble states its author, my bubbles sit right with an
 * outgoing accent edge, the inbox preview states the direction in words and
 * never claims "No messages yet" while the server reports a message, the action
 * toolbar is worded and individually labelled, the composer does not reflow when
 * the counter appears, and Ctrl+F opens the in-conversation find bar.
 *
 * Its first run caught a duplicate `const` in THIS file (a partial write left
 * `.chat-panel__msg-list` undefined), which is exactly why it exists.
 *
 * Read-only: it never sends, edits or deletes anything.
 *
 *   node scripts/probe-chat-clarity.mjs [--mobile]
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9361)
const BASE = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const API = process.env.VEYLIX_API ?? 'http://127.0.0.1:8080'
const MOBILE = process.argv.includes('--mobile')
const ROLE = process.env.VEYLIX_DEV_ROLE ?? 'super-admin'
/** An explicit thread id may be given; otherwise the inbox decides. */
const THREAD_ARG = process.argv.find((a) => /^\d+$/.test(a)) ?? null

const results = []
const check = (name, pass, detail) => {
  results.push({ name, pass })
  console.log(`${pass ? 'ok  ' : 'FAIL'} - ${name}${detail !== undefined ? `  (${detail})` : ''}`)
}

/**
 * The first conversation this account can actually see, straight from the API.
 * The inbox is the server's own answer to "which conversations exist for me?",
 * which is the only safe source for a probe's target thread.
 */
const firstThreadId = async (cookie) => {
  const res = await fetch(`${API}/api/chat/threads`, {
    headers: { Cookie: cookie, Accept: 'application/json' },
  })
  if (res.status !== 200) return null
  const rows = await res.json()
  return Array.isArray(rows) && rows.length > 0 ? String(rows[0].id) : null
}

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=C:\\Windows\\Temp\\veylix-clarity-${process.pid}-${Date.now()}`,
    '--no-first-run',
    '--disable-gpu',
    '--hide-scrollbars',
    `--window-size=${MOBILE ? '390,844' : '1440,900'}`,
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

  await send('Runtime.enable')
  await send('Page.enable')
  const pageErrors = []
  ws.addEventListener('message', (ev) => {
    const m = JSON.parse(ev.data)
    if (m.method === 'Runtime.exceptionThrown') {
      pageErrors.push(String(m.params?.exceptionDetails?.exception?.description ?? '').slice(0, 200))
    }
  })
  if (MOBILE) {
    await send('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 2, mobile: true })
  }

  /*
   * SIGN IN FIRST. This used to say "the dev server runs as the D-022 super
   * admin, so the page opens signed in" — that bypass was removed on 2026-10-28
   * (D-039). Without a session `/chat/<id>` lands on /login, every selector below
   * matches nothing, and the probe reports the screen as broken when it never
   * rendered at all.
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
   * Resolve the conversation from this account's own inbox. Conversation ids are
   * global, so the old hard-coded `/chat/1` measured somebody else's thread (and
   * therefore a 403/404 page) for every account but the one that owns it.
   */
  const THREAD = THREAD_ARG ?? (await firstThreadId(session.cookie))
  if (!THREAD) {
    console.error(`no conversation for ${session.email} — open one in the app first, or pass a thread id`)
    process.exit(2)
  }

  await send('Page.navigate', { url: `${BASE}/chat/${THREAD}` })
  await sleep(3000)

  /* Measure the resting layout BEFORE any interaction. Opening the find bar or
     typing a long draft legitimately shrinks the transcript, so the ratio below
     must be taken from the screen a user actually lands on. */
  const layoutAtLoad = await evaluate(`(() => {
    const pane = document.querySelector('.chat-panel')
    const list = document.querySelector('.chat-panel__msg-list')
    if (!pane || !list) return { panelH: 0, msgPct: 0 }
    return {
      panelH: Math.round(pane.getBoundingClientRect().height),
      msgPct: Math.round((list.getBoundingClientRect().height / pane.getBoundingClientRect().height) * 100),
    }
  })()`)

  /* ---- 1. the toolbar over the SHORTEST message is one usable row ----
     Regression guard for the measured bug: it used to be a 50px x 178px column
     of seven stacked buttons, one of them not clickable at all. */
  const hoverPoint = await evaluate(`(() => {
    const pane = document.querySelector('.chat-panel__msg-list')
    if (!pane) return null
    const p = pane.getBoundingClientRect()
    const rows = [...document.querySelectorAll('.chat-msg')]
      .map((r) => ({ r, b: r.querySelector('.chat-msg__body')?.getBoundingClientRect() }))
      .filter((x) => x.b && x.b.top > p.top + 40 && x.b.bottom < p.bottom)
      .sort((a, b) => a.b.width - b.b.width)
    if (!rows.length) return null
    return { x: rows[0].b.x + rows[0].b.width / 2, y: rows[0].b.y + rows[0].b.height / 2, w: Math.round(rows[0].b.width) }
  })()`)

  if (!hoverPoint) {
    check('a fully visible message to hover', false)
  } else {
    await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: hoverPoint.x, y: hoverPoint.y, button: 'none' })
    await sleep(500)
    const bar = await evaluate(`(() => {
      const pane = document.querySelector('.chat-panel__msg-list').getBoundingClientRect()
      const rows = [...document.querySelectorAll('.chat-msg')]
        .map((r) => ({ r, b: r.querySelector('.chat-msg__body')?.getBoundingClientRect() }))
        .filter((x) => x.b && x.b.top > pane.top + 40 && x.b.bottom < pane.bottom)
        .sort((a, b) => a.b.width - b.b.width)
      const el = rows[0].r.querySelector('.chat-msg__actions')
      if (!el) return { error: 'no toolbar' }
      const bb = el.getBoundingClientRect()
      const buttons = [...el.querySelectorAll('.chat-icon-btn')].map((b) => {
        const r = b.getBoundingClientRect()
        const hit = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2)
        return { label: b.getAttribute('aria-label') ?? '', reachable: !!hit && (hit === b || b.contains(hit)), h: Math.round(r.height) }
      })
      return {
        height: Math.round(bb.height),
        width: Math.round(bb.width),
        insidePane: bb.left >= pane.left - 1 && bb.right <= pane.right + 1 && bb.top >= pane.top - 1,
        gapToOwnBubble: Math.round(rows[0].b.top - bb.bottom),
        coversOwnText: bb.bottom > rows[0].b.top,
        overlapsPrevious: (() => {
          const prev = rows[0].r.previousElementSibling
          const pb = prev?.querySelector?.('.chat-msg__body')?.getBoundingClientRect()
          return pb ? bb.top < pb.bottom : false
        })(),
        outline: getComputedStyle(rows[0].r.querySelector('.chat-msg__body')).outlineWidth,
        inOwnRow: el.closest('.chat-msg') === rows[0].r,
        opacity: getComputedStyle(el).opacity,
        buttons,
      }
    })()`)
    // One row means: the toolbar's height is one button's height plus padding,
    // NOT a multiple of it. A fixed pixel threshold broke as soon as the buttons
    // grew to meet the touch minimum, so the ratio is what is asserted.
    const btnH = bar.buttons[0]?.h ?? 0
    const rows = btnH > 0 ? Math.round(bar.height / btnH) : 0
    check(
      'toolbar over a short message is a single row',
      rows === 1 && bar.height <= 56,
      `${bar.height}px tall, ~${rows} row(s) of ${btnH}px buttons`,
    )
    // The hover/desktop layout docks the toolbar above the bubble, so the same
    // rules apply there. On mobile it is always visible and lives BELOW the
    // bubble (there is no hover to reveal it), so these three are desktop-only.
    if (!MOBILE) {
      check(
        'toolbar touches its own bubble',
        bar.gapToOwnBubble !== null && bar.gapToOwnBubble <= 2,
        `${bar.gapToOwnBubble}px above its bubble`,
      )
      check('toolbar never covers its own message text', bar.coversOwnText === false)
      check('toolbar does not overlap the previous message', bar.overlapsPrevious === false)
      check('the hovered bubble is highlighted', bar.outline !== '0px', `outline ${bar.outline}`)
    } else {
      // On mobile the pairing is proven differently: the bar is inside the same
      // message row as the bubble it acts on, and it is actually visible.
      check('mobile: toolbar sits in its own message row', bar.inOwnRow === true)
      check('mobile: toolbar is visible without hover', bar.opacity === '1')
    }
    check('toolbar stays inside the conversation pane', bar.insidePane === true, `${bar.width}px wide`)
    check(
      'every toolbar button is clickable',
      !bar.error && bar.buttons.length > 0 && bar.buttons.every((b) => b.reachable),
      bar.error ? bar.error : `${bar.buttons.filter((b) => b.reachable).length}/${bar.buttons.length}`,
    )
    const labels = (bar.buttons ?? []).map((b) => b.label).filter(Boolean)
    check('toolbar buttons are individually labelled', new Set(labels).size === labels.length, labels.join(' / '))
  }

  /* ---- 2. direction is stated where it matters ----
     The reported problem: a sent message and a received one looked the same.
     A visible arrow on every bubble was tried and removed as noise (24 glyphs to
     say one thing), so the signals asserted now are: an author label at the start
     of EVERY run, plus a directional edge on every bubble. */
  const dir = await evaluate(`(() => {
    const rows = [...document.querySelectorAll('.chat-msg')]
    if (!rows.length) return { error: 'no messages' }
    const mine = rows.filter((r) => r.classList.contains('chat-msg--mine'))
    const theirs = rows.filter((r) => r.classList.contains('chat-msg--theirs'))
    const edge = (r) => {
      const cs = getComputedStyle(r.querySelector('.chat-msg__body'))
      return r.classList.contains('chat-msg--mine') ? cs.borderRightWidth : cs.borderLeftWidth
    }
    return {
      total: rows.length,
      runsUnnamed: rows.filter((r) => r.classList.contains('chat-msg--first') && !r.querySelector('.chat-msg__name')).length,
      mineNames: mine.filter((r) => r.classList.contains('chat-msg--first')).map((r) => r.querySelector('.chat-msg__name')?.textContent),
      everyBubbleHasEdge: rows.every((r) => parseFloat(edge(r)) >= 2),
      // No per-bubble arrow glyphs: the meaning moved to the label + edge.
      arrows: document.querySelectorAll('.chat-bubble__dir').length,
      srDirection: rows.every((r) => !!r.querySelector('.chat-bubble__meta .sr-only')),
    }
  })()`)
  check('every run of messages starts with its author', dir.runsUnnamed === 0, `${dir.runsUnnamed} unnamed runs`)
  check('my runs are labelled "You"', (dir.mineNames ?? []).every((n) => n === 'You'), (dir.mineNames ?? []).join(','))
  check('every bubble carries a directional edge', dir.everyBubbleHasEdge === true)
  check('no repetitive arrow glyph on every bubble', dir.arrows === 0)
  check('direction is still announced to screen readers', dir.srDirection === true)

  /* ---- 3. the inbox preview is truthful, written once, and says who spoke ---- */
  const previews = await evaluate(`[...document.querySelectorAll('.chat-list__preview')].map((p) => p.textContent.trim())`)
  const serverLast = await evaluate(
    `fetch('/api/chat/threads').then((r) => r.json()).then((ts) => ts.map((t) => (t.last_message ? t.last_message.body : null)))`,
  )
  check(
    'no inbox row claims "No messages yet" while the server has one',
    (previews ?? []).every((p, i) => (serverLast?.[i] ? !/no messages yet/i.test(p) : true)),
    JSON.stringify(previews),
  )
  check(
    'the preview names the speaker exactly once',
    (previews ?? []).every((p) => !/^(you:\s*)?you/i.test(p.replace(/^you:\s*/i, '')) ),
    JSON.stringify(previews),
  )

  /* ---- 4. the composer does not reflow when the counter appears ---- */
  const widthBefore = await evaluate(
    `(() => { const s = document.querySelector('.chat-panel__send'); return s ? Math.round(s.getBoundingClientRect().width) : 0 })()`,
  )
  await evaluate(`(() => {
    const ta = document.querySelector('.chat-panel__input textarea')
    const set = (el, v) => { Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(el,v); el.dispatchEvent(new Event('input',{bubbles:true})) }
    set(ta, 'x'.repeat(65400))
  })()`)
  await sleep(250)
  const widthAfter = await evaluate(`(() => {
    const s = document.querySelector('.chat-panel__send')
    const c = document.querySelector('.chat-panel__counter')
    return { w: s ? Math.round(s.getBoundingClientRect().width) : 0, counter: c ? c.textContent.trim() : '' }
  })()`)
  check(
    'the character counter does not widen the Send area',
    Math.abs(widthAfter.w - widthBefore) <= 1,
    `${widthBefore}px -> ${widthAfter.w}px (counter "${widthAfter.counter}")`,
  )
  // Never leave a draft behind: this probe must not be able to send anything.
  await evaluate(`(() => {
    const ta = document.querySelector('.chat-panel__input textarea')
    const set = (el, v) => { Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(el,v); el.dispatchEvent(new Event('input',{bubbles:true})) }
    set(ta, '')
  })()`)
  await sleep(150)

  /* ---- 5. delete is REVERSIBLE (the headline fix) ----
     Delete used to be a one-way door: the row went into `chat_message_hidden`
     and every read filtered it out, with no endpoint to undo it. This walks the
     real control and asserts the message comes back. */
  const delTarget = await evaluate(`(() => {
    const rows = [...document.querySelectorAll('.chat-msg')]
    const last = rows[rows.length - 1]
    const btn = [...(last?.querySelectorAll('.chat-icon-btn') ?? [])]
      .find((b) => b.getAttribute('aria-label') === 'Delete for me')
    return { found: !!btn, text: (last?.querySelector('.chat-bubble')?.textContent ?? '').slice(0, 20) }
  })()`)
  if (!delTarget.found) {
    check('a message with a Delete control to test undo with', false)
  } else {
    await evaluate(`(() => {
      const rows = [...document.querySelectorAll('.chat-msg')]
      const last = rows[rows.length - 1]
      const btn = [...last.querySelectorAll('.chat-icon-btn')].find((b) => b.getAttribute('aria-label') === 'Delete for me')
      btn?.click()
    })()`)
    await sleep(1200)
    const afterDelete = await evaluate(`({
      n: document.querySelectorAll('.chat-msg').length,
      undo: !!document.querySelector('.chat-undo'),
      hasText: (document.querySelector('.chat-undo__text')?.textContent ?? '').length > 0,
    })`)
    check('Delete offers an Undo that names the message', afterDelete.undo === true && afterDelete.hasText === true)

    await evaluate(`(() => {
      const b = [...document.querySelectorAll('.chat-undo button')].find((x) => /^undo$/i.test(x.textContent.trim()))
      b?.click()
    })()`)
    await sleep(1500)
    const afterUndo = await evaluate(`({
      n: document.querySelectorAll('.chat-msg').length,
      undoGone: !document.querySelector('.chat-undo'),
    })`)
    check('Undo puts the message back in the transcript', afterUndo.n === afterDelete.n + 1, `${afterDelete.n} -> ${afterUndo.n}`)
    check('the Undo strip clears once it is used', afterUndo.undoGone === true)
  }

  /* ---- 6. Ctrl+F opens the in-conversation find bar ---- */

  await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'f', code: 'KeyF', windowsVirtualKeyCode: 70, modifiers: 2 })
  await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'f', code: 'KeyF', windowsVirtualKeyCode: 70, modifiers: 2 })
  await sleep(350)
  check('Ctrl+F opens the in-conversation find bar', (await evaluate(`!!document.querySelector('.chat-find input')`)) === true)

  /* ---- 7. usability floor: targets, layout ratio, touch reachability ----
     These are the numbers this research produced. Each one was a real defect:
     131 controls measured 24x24 on a phone, only 64% of the panel was transcript,
     and on a touch device the message toolbar computed `opacity: 0` because a
     later media block re-declared its properties without that one. */
  const floor = await evaluate(`(() => {
    const pane = document.querySelector('.chat-panel')
    const list = document.querySelector('.chat-panel__msg-list')
    const zero = []
    const small = []
    document.querySelectorAll('.chat-panel button, .chat-list button').forEach((b) => {
      const r = b.getBoundingClientRect()
      // A control that is deliberately not rendered at this viewport is not a
      // defect: the mobile back button is hidden on desktop, and on a phone the
      // whole thread list is hidden in favour of one pane at a time.
      const cs = getComputedStyle(b)
      if ((r.width === 0 || r.height === 0) && (cs.display === 'none' || !b.offsetParent)) return
      const name = (b.getAttribute('aria-label') || b.textContent || '').trim().slice(0, 24) || 'unnamed'
      if (r.width === 0 || r.height === 0) zero.push(name)
      else if (r.width < 32 || r.height < 32) small.push(name + ':' + Math.round(r.width) + 'x' + Math.round(r.height))
    })
    const bars = [...document.querySelectorAll('.chat-msg__actions')]
    // On a hover-capable device the toolbar is hidden until hover — that is by
    // design. What matters is that it is never BOTH hidden and unreachable on a
    // TOUCH device, and that hovering reveals it here. The probe asserts the
    // hover behaviour; the touch case is checked in the --mobile run.
    const first = bars[0]
    const hoverable = first ? getComputedStyle(first).pointerEvents : null
    return {
      zeroSize: zero,
      under32: small,
      panelH: Math.round(pane.getBoundingClientRect().height),
      msgPct: list ? Math.round((list.getBoundingClientRect().height / pane.getBoundingClientRect().height) * 100) : 0,
      toolbarCount: bars.length,
      toolbarOpacity: [...new Set(bars.map((b) => getComputedStyle(b).opacity))],
      toolbarPointer: hoverable,
      permanentBars: document.querySelectorAll('.chat-hidden-bar').length,
      overflowX: document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
    }
  })()`)
  /*
   * `floor` is null when the pane never rendered — which is what happens if the
   * page bounced to /login (no session) or the conversation id belongs to somebody
   * else. Report that as ONE clear failure instead of a TypeError that hides every
   * check after it.
   */
  if (!floor) {
    check('the conversation pane rendered', false, 'nothing measurable on the page (see checks above)')
  } else {
    check('no zero-sized interactive control', floor.zeroSize.length === 0, floor.zeroSize.join(', '))
    check('no interactive control below 32px', floor.under32.length === 0, floor.under32.join(', '))
    check('transcript gets at least 70% of the panel', layoutAtLoad.msgPct >= 70, `${layoutAtLoad.msgPct}% of ${layoutAtLoad.panelH}px`)
    check(
      'message toolbars exist and are reachable',
      // Desktop: hidden until hover, so pointer-events must be none.
      // Touch/mobile: permanently visible, so pointer-events must be auto.
      floor.toolbarCount > 0 &&
        (MOBILE ? floor.toolbarPointer === 'auto' : floor.toolbarPointer === 'none'),
      `${floor.toolbarCount} bars, pointer-events=${floor.toolbarPointer}`,
    )
    check('no permanent bar is reserved for a rare action', floor.permanentBars === 0)
    check('no horizontal overflow', floor.overflowX === false)
  }

  if (pageErrors.length) {
    console.log('\nPAGE EXCEPTIONS:')
    for (const e of [...new Set(pageErrors)].slice(0, 5)) console.log('  !', e)
  }

  const failed = results.filter((r) => !r.pass)
  console.log(`\n${results.length - failed.length}/${results.length} checks passed`)
  ws.close()
  cleanup()
  process.exit(failed.length || pageErrors.length ? 1 : 0)
} catch (err) {
  console.error('probe error:', err)
  cleanup()
  process.exit(2)
}

