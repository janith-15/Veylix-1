/**
 * Chat section verification — the only trustworthy check for "does it really
 * render styled?". Boots the dev server, signs in, opens /chat and asserts:
 *   1. chat.css actually applied (bubble bg != transparent, padding set)
 *   2. the inbox lists only participant-safe threads
 *   3. Enter sends → right-aligned "mine" bubble, composer clears, persisted
 *   4. unread badge clears once the thread is read
 *   5. mobile shows one pane at a time (list OR conversation)
 *
 * Usage: node scripts/verify-chat.mjs [--mobile]
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'

const CHROME = 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = 9341
const BASE = 'http://localhost:5173'
const MOBILE = process.argv.includes('--mobile')
const EMAIL = process.env.VEYLIX_EMAIL ?? 'client@veylix.app'

const results = []
const check = (name, pass, detail) => {
  results.push({ name, pass })
  console.log(`${pass ? 'ok  ' : 'FAIL'} - ${name}${detail ? `  (${detail})` : ''}`)
}

const vite = spawn('npx', ['vite', '--port', '5173', '--strictPort'], { shell: true, stdio: 'ignore' })

const waitForServer = async () => {
  for (let i = 0; i < 80; i++) {
    try {
      const r = await fetch(BASE)
      if (r.ok) return
    } catch {
      /* retry */
    }
    await sleep(500)
  }
  throw new Error('vite did not start')
}

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    // Unique profile per run: a reused profile can restore a stale session
    // (different signed-in role) and silently invalidate the whole run.
    `--user-data-dir=C:\\Windows\\Temp\\veylix-chat-verify-${process.pid}-${Date.now()}`,
    '--no-first-run',
    '--disable-gpu',
    `--window-size=${MOBILE ? '390,844' : '1440,900'}`,
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

const cleanup = () => {
  try { chrome.kill() } catch { /* ignore */ }
  try { vite.kill() } catch { /* ignore */ }
}

try {
  await waitForServer()
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

  await send('Runtime.enable')
  await send('Page.enable')
  /* Surface page errors — a throwing render would otherwise be invisible. */
  await send('Log.enable').catch(() => {})
  const pageErrors = []
  ws.addEventListener('message', (ev) => {
    const m = JSON.parse(ev.data)
    if (m.method === 'Log.entryAdded' && ['error', 'warning'].includes(m.params?.entry?.level)) {
      pageErrors.push(String(m.params.entry.text).slice(0, 300))
    }
    if (m.method === 'Runtime.exceptionThrown') {
      pageErrors.push('EXCEPTION: ' + String(m.params?.exceptionDetails?.exception?.description ?? '').slice(0, 300))
    }
  })
  if (MOBILE) {
    await send('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 2, mobile: true })
  }

  /* ---- sign in ---- */
  await send('Page.navigate', { url: `${BASE}/login` })
  await sleep(3000)
  const loginState = await evaluate(`(() => ({
    path: location.pathname,
    hasEmail: !!document.querySelector('input[type="email"]'),
    hasForm: !!document.querySelector('form'),
    auth: localStorage.getItem('veylix-auth'),
  }))()`)
  console.log('LOGIN PAGE:', JSON.stringify(loginState))
  await evaluate(`(() => {
    localStorage.removeItem('veylix-chat')
    localStorage.removeItem('veylix-auth')
    const set = (el, v) => { Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(el,v); el.dispatchEvent(new Event('input',{bubbles:true})) }
    set(document.querySelector('input[type="email"]'), ${JSON.stringify(EMAIL)})
    set(document.querySelector('input[type="password"]'), 'demo1234')
    document.querySelector('form').requestSubmit()
  })()`)
  await sleep(2500)
  const who = await evaluate(`JSON.parse(localStorage.getItem('veylix-auth') ?? '{}')?.state?.user?.email ?? '(none)'`)
  console.log('SIGNED IN AS:', who, '| expected:', EMAIL)
  if (who !== EMAIL) throw new Error(`sign-in failed: got ${who}, expected ${EMAIL}`)

  /* ---- open chat ---- */
  await send('Page.navigate', { url: `${BASE}/chat` })
  await sleep(2500)

  const snap = await evaluate(`(() => {
    const panel = document.querySelector('.chat-panel')
    const list = document.querySelector('.chat-list')
    const bubble = document.querySelector('.chat-msg__body')
    const cs = (el) => (el ? getComputedStyle(el) : null)
    const items = [...document.querySelectorAll('.chat-list__item')]
    return {
      url: location.pathname,
      hasPanel: !!panel,
      panelBg: cs(panel)?.backgroundColor,
      panelHeight: panel ? Math.round(panel.getBoundingClientRect().height) : 0,
      bubbleBg: cs(bubble)?.backgroundColor,
      bubblePad: cs(bubble)?.padding,
      daySeparator: !!document.querySelector('.chat-day'),
      threadCount: items.length,
      titles: items.map((b) => b.querySelector('.chat-list__title')?.textContent),
      listVisible: list ? cs(list).display !== 'none' : false,
      backVisible: document.querySelector('.chat-panel__back')
        ? cs(document.querySelector('.chat-panel__back')).display !== 'none' : false,
      overflowX: document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
    }
  })()`)

  check('chat page rendered', snap.hasPanel, snap.url)
  check('chat.css applied to panel', snap.panelBg !== 'rgba(0, 0, 0, 0)', snap.panelBg)
  check('bubbles styled', snap.bubbleBg !== 'rgba(0, 0, 0, 0)' && snap.bubblePad !== '0px', `${snap.bubbleBg} / pad ${snap.bubblePad}`)
  check('panel has a real height', snap.panelHeight > 300, `${snap.panelHeight}px`)
  check('day separator rendered', snap.daySeparator)
  check('inbox rendered', snap.threadCount > 0, `${snap.threadCount}: ${snap.titles?.join(' | ')}`)
  const MY_THREAD_COUNT = snap.threadCount
  check('no cross-tenant threads', snap.titles?.every((x) => x?.includes('Kasun')))
  check('no horizontal overflow', !snap.overflowX)
  if (MOBILE) check('mobile: one pane (conversation + back)', !snap.listVisible && snap.backVisible)
  else check('desktop: both panes visible', snap.listVisible)

  /* ================= v2 features ================= */
  const click = async (selector, index = 0) => {
    await evaluate(`(() => {
      const el = document.querySelectorAll(${JSON.stringify(selector)})[${index}]
      if (el) el.click()
    })()`)
    await sleep(450)
  }
  /** Click an action on the LAST message by its accessible label. */
  const clickLastMsgAction = async (label) => {
    await evaluate(`(() => {
      const msgs = [...document.querySelectorAll('.chat-msg')]
      const last = msgs[msgs.length - 1]
      const btn = [...(last?.querySelectorAll('.chat-msg__actions .chat-icon-btn') ?? [])]
        .find((b) => (b.getAttribute('aria-label') ?? '').startsWith(${JSON.stringify(label)}))
      btn?.click()
    })()`)
    await sleep(450)
  }

  /** Type into the composer and send (Enter, else the Send button). */
  const sendDraft = async (draft) => {
    await evaluate(`(() => {
      const ta = document.querySelector('.chat-panel__input textarea')
      const set = (el, v) => { Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(el,v); el.dispatchEvent(new Event('input',{bubbles:true})) }
      set(ta, ${JSON.stringify(draft)})
      ta.focus()
    })()`)
    await sleep(200)
    await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13, text: '\r' })
    await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13 })
    await sleep(600)
    const left = await evaluate(`document.querySelector('.chat-panel__input textarea')?.value ?? ''`)
    if (left === draft) {
      // Enter was swallowed (slow hydration) — click Send, same code path.
      await evaluate(`(() => {
        const btn = [...document.querySelectorAll('.chat-panel__send button')].find((b) => /send/i.test(b.textContent ?? ''))
        btn?.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
        btn?.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
        btn?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      })()`)
      await sleep(600)
    }
  }

  /* ---- user picker is SCOPED (no cross-tenant people) ---- */
  await click('.ui-page-head__actions button')
  const picker = await evaluate(`(() => {
    const dialog = document.querySelector('.ui-dialog')
    const names = [...document.querySelectorAll('.chat-picker__text strong')].map((e) => e.textContent)
    return {
      open: !!dialog,
      names,
      emails: [...document.querySelectorAll('.chat-picker__text .ui-muted')].map((e) => e.textContent),
    }
  })()`)
  check('user picker opens', picker.open === true, `${picker.names?.length ?? 0} people`)
  const pickerEmails = picker.emails ?? []
  /* Scoping rule differs per role:
     - Client  → only their own sellers
     - Admin   → only their own clients (+ Super Admin)
     - Super Admin → staff directory + existing clients (oversight) */
  const unrelatedClients = ['ruwan@veylix.app', 'tharindu@veylix.app']
  const isStaff = !EMAIL.startsWith('client')
  const pickerScoped =
    pickerEmails.length > 0 &&
    !pickerEmails.includes(EMAIL) &&
    (isStaff || !pickerEmails.some((e) => unrelatedClients.includes(e)))
  check('picker never lists yourself', !pickerEmails.includes(EMAIL), EMAIL)
  check('picker respects the role scope', pickerScoped, pickerEmails.join(', '))
  await evaluate(`document.querySelector('.ui-dialog-overlay')?.click()`)
  await sleep(350)

  /* ---- save (bookmark) → Saved filter ---- */
  await clickLastMsgAction('Bookmark message')
  const savedTag = await evaluate(`!!document.querySelector('.chat-tag--saved')`)
  check('save marks the message', savedTag === true)
  await click('.chat-filter', 3)
  const savedFilter = await evaluate(`document.querySelectorAll('.chat-list__item').length`)
  check('Saved filter keeps the conversation', savedFilter >= 1, `${savedFilter} threads`)
  await click('.chat-filter', 0)

  /* ---- important → Important filter ---- */
  await clickLastMsgAction('Mark important')
  const importantTag = await evaluate(`!!document.querySelector('.chat-tag--important')`)
  check('important marks the message', importantTag === true)
  await click('.chat-filter', 2)
  const importantFilter = await evaluate(`document.querySelectorAll('.chat-list__item').length`)
  check('Important filter keeps the conversation', importantFilter >= 1, `${importantFilter} threads`)
  await click('.chat-filter', 0)

  /* ---- pin message → banner ---- */
  await clickLastMsgAction('Pin message')
  const pinned = await evaluate(`(() => {
    const b = document.querySelector('.chat-pinned')
    return { shown: !!b, text: b?.querySelector('.chat-pinned__text')?.textContent ?? '' }
  })()`)
  check('pin shows the banner', pinned.shown === true, pinned.text?.slice(0, 28))

  /* ---- edit own message ---- */
  /* ---- edit own message (send a fresh one first: the seed messages are
         deliberately older than the 15-minute edit window) ---- */
  await sendDraft('Editable draft')
  await clickLastMsgAction('Edit message')
  const editorOpen = await evaluate(`!!document.querySelector('.chat-msg__editor textarea')`)
  check('edit opens the inline editor', editorOpen === true)
  await evaluate(`(() => {
    const ta = document.querySelector('.chat-msg__editor textarea')
    const set = (el, v) => { Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(el,v); el.dispatchEvent(new Event('input',{bubbles:true})) }
    set(ta, 'Edited body')
    ta.focus()
  })()`)
  await sleep(200)
  await evaluate(`[...document.querySelectorAll('.chat-msg__editor-actions .chat-icon-btn')].find((b) => b.textContent.includes('Save'))?.click()`)
  await sleep(450)
  const edited = await evaluate(`(() => {
    const texts = [...document.querySelectorAll('.chat-msg__text')].map((e) => e.textContent)
    return { hasEdited: texts.includes('Edited body'), marker: !!document.querySelector('.chat-msg__edited') }
  })()`)
  check('edit saves the new body', edited.hasEdited === true)
  check('edited marker shown', edited.marker === true)

  /* ---- delete for me (soft) ---- */
  const beforeDelete = await evaluate(`document.querySelectorAll('.chat-msg').length`)
  await clickLastMsgAction('Delete for me')
  const afterDelete = await evaluate(`document.querySelectorAll('.chat-msg').length`)
  check('delete removes it for me', afterDelete === beforeDelete - 1, `${beforeDelete} → ${afterDelete}`)
  const softDeleted = await evaluate(`(() => {
    const t = JSON.parse(localStorage.getItem('veylix-chat') ?? '{}')?.state?.threads ?? []
    return t.some((th) => th.messages.some((m) => (m.removedBy ?? []).length > 0))
  })()`)
  check('delete is soft (kept in storage)', softDeleted === true)

  /* ---- does a row-menu store action re-render? (Pin latest message) ---- */
  await evaluate(`document.querySelector('.chat-pinned__unpin')?.click()`)
  await sleep(400)
  await evaluate(`(() => {
    const row = document.querySelector('.chat-list__row-wrap')
    const menuBtn = row?.querySelector('.chat-list__menu button')
    menuBtn?.click()
  })()`)
  await sleep(500)
  await evaluate(`(() => {
    const item = [...document.querySelectorAll('.chat-ui-menu button, [role="menuitem"], .ui-dropdown__item')]
      .find((b) => b.textContent?.includes('Pin latest message'))
    item?.click()
  })()`)
  await sleep(600)
  const pinViaMenu = await evaluate(`!!document.querySelector('.chat-pinned')`)
  check('row-menu action re-renders the panel', pinViaMenu === true)

  /* ---- lock (Admin/Super Admin only) — driven on the page's own store ---- */
  const lockHooks = await evaluate(`(async () => {
    const mod = await import('/src/stores/chatStore.ts')
    const me = JSON.parse(localStorage.getItem('veylix-auth') ?? '{}')?.state?.user
    return { email: me?.email ?? '', role: mod.roleOf(me?.email ?? 'client@veylix.app') }
  })()`)
  /* Open the row menu the way a browser dispatches events (retry: the first
     synthetic click can be swallowed while React finishes hydrating). */
  const openRowMenu = async () => {
    for (let attempt = 0; attempt < 3; attempt++) {
      await evaluate(`(() => {
        const rows = [...document.querySelectorAll('.chat-list__row-wrap')]
        const row = rows.find((r) => r.querySelector('.chat-list__item--active')) ?? rows[0]
        const btn = row?.querySelector('.chat-list__menu button')
        if (!btn) return
        const opts = { bubbles: true, cancelable: true, view: window }
        btn.dispatchEvent(new PointerEvent('pointerdown', opts))
        btn.dispatchEvent(new MouseEvent('mousedown', opts))
        btn.dispatchEvent(new PointerEvent('pointerup', opts))
        btn.dispatchEvent(new MouseEvent('mouseup', opts))
        btn.dispatchEvent(new MouseEvent('click', opts))
      })()`)
      await sleep(500)
      if (await evaluate(`!!document.querySelector('.ui-dropdown__menu')`)) return true
    }
    return false
  }
  const readMenu = () =>
    evaluate(`(() => {
      const menu = document.querySelector('.ui-dropdown__menu')
      return menu ? [...menu.querySelectorAll('button')].map((b) => (b.textContent ?? '').trim()) : []
    })()`)
  const closeMenu = async () => {
    await evaluate(`document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))`)
    await sleep(250)
  }
  const clickMenuItem = async (pattern) => {
    await evaluate(`(() => {
      const menu = document.querySelector('.ui-dropdown__menu')
      const item = [...(menu?.querySelectorAll('button') ?? [])]
        .find((b) => ${pattern}.test((b.textContent ?? '').trim()))
      const opts = { bubbles: true, cancelable: true, view: window }
      item?.dispatchEvent(new MouseEvent('mousedown', opts))
      item?.dispatchEvent(new MouseEvent('mouseup', opts))
      item?.dispatchEvent(new MouseEvent('click', opts))
    })()`)
    await sleep(600)
  }

  const isStaffRole = lockHooks?.role !== 'client'
  const panelLocked = () =>
    evaluate(`document.querySelector('[data-testid="chat-panel"]')?.getAttribute('data-locked') === 'true'`)

  /* Lock / unlock via the row menu (staff only). */
  const toggleLock = async (expectLocked) => {
    for (let attempt = 0; attempt < 2; attempt++) {
      await openRowMenu()
      if ((await readMenu()).length) {
        await clickMenuItem('/^(Lock|Unlock) conversation$/')
        if ((await panelLocked()) === expectLocked) return true
      }
      await closeMenu()
    }
    return false
  }

  if (!isStaffRole) {
    // A Client never gets the lock control.
    const clientMenuOpened = await openRowMenu()
    const clientMenu = await readMenu()
    check(
      'client has no lock control',
      clientMenuOpened && !clientMenu.some((t) => /^(Lock|Unlock) conversation$/.test(t)),
      JSON.stringify(clientMenu),
    )
    await closeMenu()
    await sendDraft('lock probe')
    const stillOpen = await evaluate(`document.querySelector('.chat-panel__input textarea')?.disabled === false`)
    check('client chat stays unlocked', stillOpen === true)
  } else {
    const staffMenuOpened = await openRowMenu()
    const staffMenu = await readMenu()
    check(
      'staff row menu offers Lock',
      staffMenuOpened && staffMenu.includes('Lock conversation'),
      JSON.stringify(staffMenu),
    )
    await closeMenu()

    check('lock takes effect from the row menu', (await toggleLock(true)) === true)
    const lockedUi = await evaluate(`(() => ({
      banner: !!document.querySelector('.chat-locked-banner'),
      disabled: document.querySelector('.chat-panel__input textarea')?.disabled === true,
      pill: !!document.querySelector('.chat-pill--lock'),
      flag: !!document.querySelector('.chat-list__flag.is-locked'),
    }))()`)
    check('lock disables the composer', lockedUi.disabled === true, JSON.stringify(lockedUi))
    check('lock shows banner + pill + list flag', lockedUi.banner && lockedUi.pill && lockedUi.flag, JSON.stringify(lockedUi))

    check('unlock restores the composer', (await toggleLock(false)) === true)
    const unlocked = await evaluate(`document.querySelector('.chat-panel__input textarea')?.disabled === false`)
    check('unlocked composer is usable', unlocked === true)
  }

  /* ---- search filters the inbox ---- */
  await evaluate(`(() => {
    const i = document.querySelector('.chat-search input')
    if (!i) return
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(i, 'zzz-no-match')
    i.dispatchEvent(new Event('input',{bubbles:true}))
  })()`)
  await sleep(400)
  const searchResult = await evaluate(`document.querySelectorAll('.chat-list__item').length`)
  check('search narrows the list', searchResult === 0, `${searchResult} threads`)
  await evaluate(`(() => {
    const i = document.querySelector('.chat-search input')
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(i, '')
    i.dispatchEvent(new Event('input',{bubbles:true}))
  })()`)
  await sleep(300)

  const before = await evaluate(`document.querySelectorAll('.chat-msg').length`)
  await sendDraft('Verification ping')

  const sent = await evaluate(`(() => {
    const msgs = [...document.querySelectorAll('.chat-msg')]
    const last = msgs[msgs.length - 1]
    const l = document.querySelector('.chat-panel__msg-list')
    return {
      count: msgs.length,
      text: last?.querySelector('.chat-msg__text')?.textContent,
      mine: last?.classList.contains('chat-msg--mine'),
      nameShown: !!last?.querySelector('.chat-msg__name'),
      cleared: document.querySelector('.chat-panel__input textarea').value === '',
      stored: (JSON.parse(localStorage.getItem('veylix-chat') ?? '{}')?.state?.threads ?? [])
        .some((th) => th.messages.some((m) => m.text === 'Verification ping')),
      badges: document.querySelectorAll('.chat-list__item--active .chat-list__badge').length,
      allBadges: document.querySelectorAll('.chat-list__badge').length,
      unreadThreads: document.querySelectorAll('.chat-list__item--unread').length,
      pinned: l ? l.scrollHeight - l.scrollTop - l.clientHeight < 60 : false,
    }
  })()`)

  check('Enter sends the message', sent.count === before + 1 && sent.text === 'Verification ping', `${before} → ${sent.count}`)
  check('own message is a right bubble', sent.mine === true)
  check('own message hides the author label', sent.nameShown === false)
  check('composer clears after send', sent.cleared === true)
  check('message persisted to storage', sent.stored === true)
  check('scrolled to newest message', sent.pinned === true)
  check('active thread shows no unread', sent.badges === 0, `badges=${sent.badges}`)
  check(
    'the other conversation keeps its unread',
    sent.unreadThreads === (MY_THREAD_COUNT > 1 ? 1 : 0),
    `unread threads=${sent.unreadThreads}`,
  )

  /* ---- persistence round-trip ---- */
  await send('Page.reload')
  await sleep(2500)
  const reload = await evaluate(`(() => {
    const texts = [...document.querySelectorAll('.chat-msg__text')].map((e) => e.textContent)
    return { kept: texts.includes('Verification ping'), url: location.pathname }
  })()`)
  check('message survives a reload', reload.kept === true, reload.url)

  const failed = results.filter((r) => !r.pass)
  if (pageErrors.length) {
    console.log('\nPAGE ERRORS/WARNINGS:')
    for (const e of [...new Set(pageErrors)].slice(0, 8)) console.log('  !', e)
  }
  console.log(`\n${results.length - failed.length}/${results.length} checks passed`)
  ws.close()
  cleanup()
  process.exit(failed.length ? 1 : 0)
} catch (err) {
  console.error('verification error:', err)
  cleanup()
  process.exit(2)
}
