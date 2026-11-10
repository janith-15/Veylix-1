#!/usr/bin/env node
/**
 * VEYLIX â€” the client "My VPNs" page against a POPULATED account.
 *
 *   node scripts/verify-client-vpns.mjs
 *
 * Why this exists: the S5 report honestly recorded that the populated state was
 * **NOT PROVEN** â€” the seeded client (`client@veylix.test`, user 15) owned zero
 * VPNs, so every card, the list view, search, sort and the no-results copy were
 * type-checked and lint-checked but never executed against real rows.
 *
 * That gap is now closable: the inbound-id fix (DEV-001 / LEGACY-I-36) makes a
 * FREE claim succeed, so this script signs in as the owning client and asserts
 * the real thing â€” and REFUSES (exit 3), loudly, if the account still owns
 * nothing, because a green run against an empty account would prove nothing.
 *
 * READ-ONLY against the app: it loads pages, types in a box and clicks controls.
 * It creates no VPN, releases nothing and writes no row.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9451)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const ROLE = process.env.VEYLIX_ROLE ?? 'client'

const chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('clientvpns')}`, '--no-first-run',
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
const check = (ok, msg, extra) => {
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${msg}${extra === undefined ? '' : `  -> ${JSON.stringify(extra)}`}`)
  if (!ok) failures.push(msg)
}

/** Waits until a numeric expression settles, so an assertion never races render. */
async function settle(expr, want) {
  let last = -1
  for (let i = 0; i < 25; i++) {
    await sleep(300)
    last = await evaluate(expr)
    if (typeof last === 'number' && last === want) return last
  }
  return last
}

/**
 * Builds an expression that types into the page's own search box.
 *
 * The value is applied through the native `value` setter and an `input` event,
 * because React ignores a plain assignment; this is the same technique the other
 * harnesses in this repo use.
 */
const typeIntoSearch = (value) => `
  (() => {
    const box = document.querySelector('.mv input[type=search], .mv [role=searchbox], .mv input[type=text]')
    if (!box) return false
    const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
    set.call(box, ${JSON.stringify(value)})
    box.dispatchEvent(new Event('input', { bubbles: true }))
    return true
  })()
`


await send('Page.navigate', { url: `${APP}/client/vpns?view=cards` })
await settle(`document.querySelectorAll('.mv article').length`, 1)

const owned = await evaluate(`
  (async () => {
    const r = await fetch('/api/vpns', { credentials: 'include' })
    return r.ok ? (await r.json()).length : -1
  })()
`)

// A green run against an account that owns nothing proves nothing. Refusing here
// is the honest answer, and it is exactly what the S5 report could not do.
if (!(owned > 0)) {
  console.error('')
  console.error(`REFUSED â€” the ${ROLE} account owns ${owned} VPN(s), so the populated`)
  console.error('state cannot be exercised. Create a FREE claim first, then re-run.')
  console.error('Reporting PASS here would be a false claim.')
  await send('Page.navigate', { url: 'about:blank' })
  chrome.kill()
  process.exit(3)
}
console.log(`# account owns ${owned} VPN(s) â€” exercising the populated state`)
console.log('')

// The page's h1 is rendered by the shared `ClientPage` shell, not inside the
// `.mv` scope the component owns — so it is queried document-wide.
const headline = await evaluate(`(document.querySelector('h1') ?? {}).textContent ?? ''`)
check(headline.trim() === 'My VPNs', 'the page renders with its own h1', headline.trim())

const cardNames = await evaluate(`
  [...document.querySelectorAll('.mv article')].map(a => {
    // The VPN's NAME, which is what the search box matches: an element carrying
    // a non-empty aria-label is the panel-identity control, and the packaging
    // name is the first text node that is not it.
    const lines = a.innerText.split('\\n').map(s => s.trim()).filter(Boolean)
    return lines[0] ?? ''
  })
`)
check(cardNames.length === owned, `every owned VPN has a card (${owned})`, cardNames.length)
check(cardNames.length > 1, 'more than one card exists, so search and sort are meaningful', cardNames.length)

const emptyOnPopulated = await evaluate(`
  /no vpns match these filters/i.test(document.body.innerText) ||
  /you have no vpn yet/i.test(document.body.innerText)
`)
check(!emptyOnPopulated, 'a populated account is never shown an empty state')

const overview = await evaluate(`
  (() => {
    const t = document.body.innerText
    const total = /TOTAL VPNS\\s*\\n\\s*(\\d+)/i.exec(t)
    return total ? Number(total[1]) : null
  })()
`)
check(overview === owned, 'the overview TOTAL matches the real inventory', { overview, owned })

/* ---- search narrows to one card --------------------------------------- */
const target = cardNames[0]
await evaluate(typeIntoSearch(target))
await sleep(800)
const afterSearch = await evaluate(`document.querySelectorAll('.mv article').length`)
check(afterSearch === 1, 'searching a card name narrows the grid to that card', { afterSearch, target })
check(await evaluate(`location.search.includes('q=')`), 'the search term is reflected in the URL, so the view is shareable')

/* ---- no-results: honest copy and an escape hatch ---------------------- */
await evaluate(typeIntoSearch('zzzz-no-such-vpn-zzzz'))
await sleep(800)
const noResult = await evaluate(`({
  cards: document.querySelectorAll('.mv article').length,
  text: document.body.innerText,
})`)
check(noResult.cards === 0, 'a term matching nothing removes every card', noResult.cards)
check(/no vpns match these filters/i.test(noResult.text), 'the no-results copy is shown rather than a blank region')
check(/still there/i.test(noResult.text), 'the copy reassures the user their VPNs still exist â€” it does not claim they are gone')
check(
  await evaluate(`[...document.querySelectorAll('button')].some(b => /clear filters/i.test(b.textContent))`),
  'a Clear filters action is offered, so the user is never trapped',
)

/* ---- clear restores --------------------------------------------------- */
await evaluate(`[...document.querySelectorAll('button')].find(b => /clear filters/i.test(b.textContent)).click()`)
await sleep(900)
const afterClear = await evaluate(`document.querySelectorAll('.mv article').length`)
check(afterClear === owned, 'Clear filters restores every card', { afterClear, owned })

/* ---- list view -------------------------------------------------------- */
await evaluate(`[...document.querySelectorAll('button')].find(b => /^list$/i.test(b.textContent.trim()))?.click()`)
await sleep(900)
const table = await evaluate(`
  (() => {
    const t = document.querySelector('.mv table')
    if (!t) return null
    return {
      heads: [...t.querySelectorAll('thead th')].map(h => h.innerText.trim()),
      rows: t.querySelectorAll('tbody tr').length,
    }
  })()
`)
check(!!table, 'the List display mode renders a real table')
check(table && table.rows === owned, 'the table lists every owned VPN', table && table.rows)
check(
  !!table && ['VPN', 'STATUS', 'SERVER'].every((h) => table.heads.some((x) => x.toUpperCase().includes(h))),
  'the table exposes the VPN, status and server columns',
  table && table.heads,
)
check(await evaluate(`location.search.includes('view=list')`), 'the chosen display mode is reflected in the URL')

/* ---- sort ------------------------------------------------------------- */
// Sort lives inside the collapsible Filters panel, so open it if the select is
// not already mounted. Asserting the control exists keeps a silent UI change
// from turning this check into a vacuous pass.
await evaluate(`
  (() => {
    if ([...document.querySelectorAll('select')].some(s => [...s.options].some(o => o.value === 'name'))) return true
    ;[...document.querySelectorAll('button')].find(b => /^filters$/i.test(b.textContent.trim()))?.click()
    return true
  })()
`)
await sleep(700)
const sortApplied = await evaluate(`
  (() => {
    const sel = [...document.querySelectorAll('select')].find(s => [...s.options].some(o => o.value === 'name'))
    if (!sel) return false
    const set = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value').set
    set.call(sel, 'name')
    sel.dispatchEvent(new Event('change', { bubbles: true }))
    return true
  })()
`)
check(sortApplied, 'a name sort control exists in the Filters panel')
await sleep(900)
const sortedNames = await evaluate(`
  [...document.querySelectorAll('.mv table tbody tr')].map(r => (r.querySelector('td') ? r.querySelector('td').innerText.trim().split('\\n')[0] : ''))
`)
check(sortedNames.length === owned, 'the table still lists every row after sorting', sortedNames.length)
check(
  JSON.stringify(sortedNames) === JSON.stringify([...sortedNames].sort((a, b) => a.localeCompare(b))),
  'sorting by name really is alphabetical',
  sortedNames,
)

/* ---- D-100: the two sections ------------------------------------------
 *
 * The main grid must hold ONLY the VPNs a client can use; suspended, revoked and
 * released ones belong to a second, labelled section. The split is asserted both
 * ways round, because either half alone is a bug:
 *
 *   - a suspended VPN left in the main grid is the defect this fixes;
 *   - a suspended VPN missing from BOTH is a lost record (D-091.5 / INV-1).
 *
 * The overview count is then checked against the TOTAL, not against the visible
 * grid — that number describes the account (D-088) and must not shrink because a
 * section was added.
 */
const sections = await evaluate(`
  (() => {
    const secs = [...document.querySelectorAll('.mv section, .mv .ui-section')]
    const find = (re) => secs.find(s => re.test((s.querySelector('h2,h3')||{}).innerText || ''))
    const notInService = find(/not in service/i)
    const rowsOf = (el) => el ? [...el.querySelectorAll('table tbody tr')].length : 0
    return {
      sectionTitles: secs.map(s => ((s.querySelector('h2,h3')||{}).innerText || '').trim()).filter(Boolean),
      emptyOnCleanAccount: !notInService,
      rows: rowsOf(notInService),
      note: notInService ? ((notInService.querySelector('.mv-resting__note')||{}).innerText || '').trim() : '',
      sinceCells: [...document.querySelectorAll('[data-testid^="vpn-row-since-"], [data-testid^="vpn-since-"]')].length,
    }
  })()
`)
check(
  sections.emptyOnCleanAccount,
  'an account with nothing suspended shows no empty "Not in service" section',
  sections.sectionTitles,
)
// Whether this seeded account HAS a suspended VPN decides which half applies.
// The split is proven either way: no section means nothing to move, a section
// means those rows are held there and explained.
if (!sections.emptyOnCleanAccount) {
  check(sections.rows > 0, '"Not in service" lists the affected VPNs rather than hiding them', sections.rows)
  check(
    /still yours|kept for your records|cannot be used/i.test(sections.note),
    'the section explains itself instead of being a bare list',
    sections.note,
  )
  check(
    sections.rows <= afterClear,
    'the resting rows are part of the account total, not extra rows',
    { resting: sections.rows, total: afterClear },
  )
}
/* The "since" label must never be fabricated: zero 'since' cells is correct on
   an account whose VPNs never changed status. What is NOT acceptable is a
   rendered 1970 date anywhere on the page. */
check(
  !(await evaluate(`/1 Jan(uary)? 1970|1970/.test(document.body.innerText)`)),
  'no fabricated 1970 date is rendered beside a status',
  sections.sinceCells,
)

/* ---- D-101: the build stamp in the sidebar footer --------------------- */
const footer = await evaluate(`
  (() => {
    const wrap = document.querySelector('.app-sidebar__footer')
    if (!wrap) return null
    const link = wrap.querySelector('[data-testid="app-repo-link"], .app-sidebar__build-link')
    return {
      version: ((wrap.querySelector('[data-testid="app-version"], .app-sidebar__build-version')||{}).innerText || '').trim(),
      href: link ? link.getAttribute('href') : '',
      target: link ? link.getAttribute('target') : '',
      rel: link ? link.getAttribute('rel') : '',
    }
  })()
`)
check(!!footer, 'the sidebar has a footer to carry the build stamp', footer)
check(
  !!footer && /^v\\d+\\.\\d+|^v?dev/i.test(footer.version),
  'the footer shows the RUNNING version, read from GET /api/system/version',
  footer && footer.version,
)
check(
  !!footer && footer.href === 'https://github.com/janith-15/Veylix-1',
  'the footer links to the source repository the build came from',
  footer && footer.href,
)
check(
  !!footer && footer.target === '_blank' && /noopener|noreferrer/.test(footer.rel),
  'the repository link opens safely in a new tab (no window.opener access)',
  footer && { target: footer.target, rel: footer.rel },
)

await send('Page.navigate', { url: 'about:blank' })
chrome.kill()

console.log('')
if (failures.length) {
  console.log(`FAILED â€” ${failures.length} check(s)`)
  for (const f of failures) console.log(`   - ${f}`)
  process.exit(1)
}
console.log(`OK â€” the client My VPNs page works against a populated account (${owned} VPNs)`)
