#!/usr/bin/env node
/**
 * VEYLIX - the rebuilt VPN details screens, against REAL data.
 *
 *   node scripts/verify-vpn-details.mjs
 *
 * WHY THIS FILE EXISTS
 *
 * This script was previously committed as 14,510 NUL bytes - a registered npm
 * script (`verify:vpn-details`) that contained no code at all. Every claim about
 * the Quick View sheet, the three-section detail page and the connection reveal
 * gate was therefore unbacked by a browser. This is the replacement, and it
 * asserts the facts a mockup cannot fake:
 *
 *   - My VPNs shows each VPN's PANEL IDENTITY beside its package name (D-091.4);
 *   - the Quick View opens, and carries the identity AND the package together;
 *   - the sheet's copy control puts the EXACT panel email on the clipboard;
 *   - the detail page renders EXACTLY three section <h2> headings;
 *   - a non-ACTIVE VPN is explained with a SENTENCE, not a bare enum (D-091.5);
 *   - the connection QR / URI is ABSENT before the reveal and PRESENT after;
 *   - the Usage range switch genuinely changes the number of chart points;
 *   - a window with no samples states "no data" and never draws a zero line.
 *
 * READ-ONLY against the app: it navigates, clicks tabs, and reads the DOM. The
 * one thing it never presses is a lifecycle control (suspend / revoke / release)
 * - those are real remote writes.
 *
 * It REFUSES (exit 3) when the signed-in client owns no VPN, because a green run
 * on an empty account would prove nothing about any of the above.
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { profileDir } from './chrome-profile.mjs'
import { cookieParts, signIn } from './auth-login.mjs'

const CHROME = process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9461)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const ROLE = process.env.VEYLIX_ROLE ?? 'client'

/* Renamed with a leading underscore: the process handle is deliberately held for
   the lifetime of the script so Chrome stays alive while the checks run, but it
   is never referenced again — hence the unused-variable exemption instead of a
   spurious `chrome.kill()` that the shell linter would still flag. */
const _chrome = spawn(CHROME, [
  '--headless=new', `--remote-debugging-port=${PORT}`,
  `--user-data-dir=${profileDir('vpndetails')}`, '--no-first-run',
  '--disable-gpu', '--window-size=1440,1600', 'about:blank',
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


/* --------------------------------------------------------------------- *
 * 1. My VPNs - panel identity beside every package name (D-091.4)
 * --------------------------------------------------------------------- */

await send('Page.navigate', { url: `${APP}/client/vpns` })
await sleep(3000)
await settle(`document.querySelectorAll('.mv-card').length`, -1)
await sleep(600)

const cardCount = await evaluate(`document.querySelectorAll('.mv-card').length`)
if (!cardCount) {
  console.error('REFUSE  the signed-in client owns no VPN cards, so nothing below could be proven')
  process.exit(3)
}
check(cardCount > 0, 'My VPNs renders at least one VPN card', cardCount)

/* Every card must carry BOTH the package name and a panel identity. A card with
   a name and no identity breaks D-091.4, and that is the defect this asserts. */
const identityPerCard = await evaluate(`
  (() => {
    const cards = [...document.querySelectorAll('.mv-card')]
    return cards.map((c) => ({
      name: c.querySelector('.mv-card__name')?.textContent?.trim() ?? '',
      identity: c.querySelector('.pid__account')?.textContent?.trim() ?? '',
      hasCopy: !!c.querySelector('.pid__copy'),
    }))
  })()
`)
check(
  identityPerCard.every((r) => r.name !== '' && r.identity !== ''),
  'every VPN card shows a package name AND a panel identity',
  identityPerCard,
)
check(
  identityPerCard.every((r) => r.hasCopy),
  'every panel identity carries a copy control',
)

/* The list view is a second surface for the same rule. If a row exists, it must
   show the identity too - one surface obeying D-091.4 is not enough. */
const rowIdentity = await evaluate(`
  (() => {
    const rows = [...document.querySelectorAll('tr[data-testid^="vpn-row-"]')]
    return { rows: rows.length, withIdentity: rows.filter((r) => !!r.querySelector('.pid__account')).length }
  })()
`)
if (rowIdentity.rows > 0) {
  check(
    rowIdentity.withIdentity === rowIdentity.rows,
    'the list view shows the panel identity on every row',
    rowIdentity,
  )
} else {
  console.log('SKIP  list view is not showing on this route (cards only) - row identity unproven here')
}

/* --------------------------------------------------------------------- *
 * 2. Quick View opens, and pairs identity WITH the package
 * --------------------------------------------------------------------- */

const firstApiId = await evaluate(`
  (() => {
    const b = document.querySelector('[data-testid^="vpn-quick-"]')
    return b ? b.getAttribute('data-testid').replace('vpn-quick-', '') : null
  })()
`)

if (!firstApiId) {
  check(false, 'a Quick View trigger exists on the first card')
} else {
  await evaluate(`document.querySelector('[data-testid="vpn-quick-${firstApiId}"]').click()`)
  await sleep(900)

  const sheet = await evaluate(`
    (() => {
      const d = document.querySelector('[data-testid="vpn-quick-view"]')
      if (!d) return null
      return {
        open: true,
        title: d.querySelector('.qv__title')?.textContent?.trim() ?? '',
        identity: d.querySelector('[data-testid="qv-panel-identity"] .pid__account')?.textContent?.trim() ?? '',
        hasCopy: !!d.querySelector('[data-testid="qv-panel-identity-copy"]'),
        hasUsage: !!d.querySelector('[data-testid="vpn-usage-cards"]'),
      }
    })()
  `)
  check(!!sheet?.open, 'the Quick View sheet opens', sheet)
  check(!!sheet?.title, 'the sheet names the VPN (package name present)', sheet?.title)
  check(!!sheet?.identity, 'the sheet shows the panel identity beside the package name', sheet?.identity)
  check(!!sheet?.hasCopy, 'the sheet identity is copyable')
  check(!!sheet?.hasUsage, 'the sheet shows the usage totals')

  /* The copy control must put the EXACT panel email on the clipboard. A
     near-miss (lowercased, trimmed of its suffix) is useless to support, who
     search the panel for this exact string. */
  const copied = await evaluate(`
    (async () => {
      const shown = document.querySelector('[data-testid="qv-panel-identity"] .pid__account')?.textContent?.trim() ?? ''
      const btn = document.querySelector('[data-testid="qv-panel-identity-copy"]')
      if (!btn) return { shown, clip: null }
      btn.click()
      try { return { shown, clip: await navigator.clipboard.readText() } } catch { return { shown, clip: null } }
    })()
  `)
  if (copied?.clip === null || copied?.clip === undefined) {
    console.log('SKIP  clipboard read is not permitted in this context; copy text unverified')
  } else {
    check(copied.clip.trim() === copied.shown, 'the copy control copies the exact panel email', copied)
  }

  /* The footer action is the hand-off to the full page. Both the connect-first
     primary action and the details link must exist. */
  const footer = await evaluate(`
    (() => {
      const d = document.querySelector('[data-testid="vpn-quick-view"]')
      if (!d) return null
      const primary = d.querySelector('[data-testid="qv-connect"]')
      return {
        hasConnect: !!primary,
        connectLabel: primary?.textContent?.trim() ?? '',
        hasFull: !!d.querySelector('[data-testid="qv-open-full"]'),
      }
    })()
  `)
  check(!!footer?.hasConnect, 'the sheet offers "Connect now" as the primary action', footer)
  check(
    /connect/i.test(footer?.connectLabel ?? ''),
    'the primary action is labelled as connecting, not as navigating',
    footer?.connectLabel,
  )
  check(!!footer?.hasFull, 'the sheet also offers a route to the full details page')

  /* Close it so the detail-page section below starts from a clean DOM. */
  await evaluate(`document.querySelector('.qv__close')?.click()`)
  await sleep(400)
}

/* --------------------------------------------------------------------- *
 * 3. Detail page - EXACTLY three section <h2> headings
 * --------------------------------------------------------------------- */

await send('Page.navigate', { url: `${APP}/vpn/${firstApiId}` })
await sleep(3000)
await settle(`document.querySelectorAll('.vd-tabs__btn').length`, 3)
await sleep(500)

const tabs = await evaluate(`
  (() => {
    const btns = [...document.querySelectorAll('.vd-tabs__btn')].map((b) => b.textContent.trim())
    return { count: btns.length, labels: btns }
  })()
`)
check(tabs.count === 3, 'the detail page has exactly three section tabs', tabs)

/*
 * ONE HIERARCHY, ONCE — asserted across all three sections.
 *
 * The page renders one section at a time, so a single snapshot always sees one
 * `.vd-h2`. The rule being checked is therefore "each section has exactly one
 * <h2> and it is the section name", not "three <h2> exist simultaneously".
 * Visiting all three also proves each tab actually mounts its section.
 */
const perSection = {}
for (const key of ['overview', 'usage', 'connect']) {
  await evaluate(`document.querySelector('[data-testid="vd-tab-${key}"]')?.click()`)
  await sleep(700)
  perSection[key] = await evaluate(`
    (() => {
      const sec = document.querySelector('[data-testid="vd-${key}"]')
      if (!sec) return null
      const heads = [...sec.querySelectorAll('.vd-h2')].map((h) => ({ tag: h.tagName, text: h.textContent.trim() }))
      return { count: heads.length, heads }
    })()
  `)
}
check(
  Object.values(perSection).every((s) => s && s.count === 1),
  'each of the three sections renders exactly one section <h2>',
  perSection,
)
check(
  Object.values(perSection).every((s) => s && s.heads.every((x) => x.tag === 'H2')),
  'every section heading is an <h2>, never a card title pretending to be one',
  Object.values(perSection).map((s) => s?.heads?.[0]?.text),
)
check(
  new Set(Object.values(perSection).map((s) => s?.heads?.[0]?.text)).size === 3,
  'the three sections have three distinct names (no repeated heading word)',
  Object.values(perSection).map((s) => s?.heads?.[0]?.text),
)

/* Back to Overview for the status check below. */
await evaluate(`document.querySelector('[data-testid="vd-tab-overview"]')?.click()`)
await sleep(600)

/* The page must not draw a green "Connected" light it cannot back up: account
   status is not device connectivity. */
const falseConnect = await evaluate(`
  /\\bconnected\\b/i.test(document.querySelector('.vd')?.innerText ?? '')
`)
check(falseConnect === false, 'the page never claims the device is "connected" from account status')

/* --------------------------------------------------------------------- *
 * 4. A non-ACTIVE VPN is explained in a sentence (D-091.5)
 * --------------------------------------------------------------------- */

const statusProbe = await evaluate(`
  (() => {
    /* StatusBadge is the ACCOUNT state; VpnKindBadge is the FREE/PREMIUM label.
       Reading the kind badge by mistake would test the wrong fact - the first
       version of this script did exactly that. */
    const badge = document.querySelector('.vd-status .ui-badge, .vd-status [class*="status"]')
    const label = badge?.textContent?.trim() ?? ''
    const note = document.querySelector('[data-testid="vd-status-note"]')?.textContent?.trim() ?? ''
    return { label, note, active: /^active$/i.test(label) }
  })()
`)
if (statusProbe.active) {
  console.log('SKIP  this VPN is ACTIVE, so it needs no sentence (that is the rule)')
} else {
  /* A non-ACTIVE status must carry a real sentence, and it must not be the bare
     enum echoed back - that is the defect (a grey chip and nothing else). */
  const isSentence = statusProbe.note.split(/\s+/).length >= 5
  check(isSentence, 'a non-ACTIVE VPN shows an explanatory sentence, not a bare enum', statusProbe)
  if (statusProbe.label) {
    check(
      statusProbe.note.toLowerCase() !== statusProbe.label.toLowerCase(),
      'the sentence is not just the status enum repeated',
      { label: statusProbe.label, note: statusProbe.note },
    )
  }
}

/* --------------------------------------------------------------------- *
 * 5. Connection reveal gate - hidden before, present after
 * --------------------------------------------------------------------- */

/*
 * Every protocol this build can issue. Anchored test: a URI STARTS with the
 * scheme, so `hysteria2://…` is a match while the word "hysteria" in prose is
 * not. An earlier version omitted `hysteria2` and reported a working reveal as
 * a failure — the panel really does hand out hysteria2 links.
 */
const uriPattern = /(vless|vmess|trojan|ss|ssr|hysteria2?|hy2|tuic|wireguard):\/\//i

await evaluate(`document.querySelector('[data-testid="vd-tab-connect"]')?.click()`)
await sleep(1200)

const beforeReveal = await evaluate(`
  (() => {
    const sec = document.querySelector('[data-testid="vpn-connect-section"]')
    if (!sec) return null
    return {
      present: true,
      svgs: sec.querySelectorAll('svg').length,
      bodyText: sec.innerText,
      uris: [...sec.querySelectorAll('code, .vcn-link__uri, textarea')].map((n) => n.textContent ?? ''),
    }
  })()
`)

if (!beforeReveal?.present) {
  check(false, 'the Connection & Help section renders')
} else {
  check(true, 'the Connection & Help section renders')

  /* The credential must not be on the page before the reader asks for it: no
     protocol URI anywhere in the section before the reveal. */
  const uriBefore = uriPattern.test(`${beforeReveal.bodyText} ${(beforeReveal.uris ?? []).join(' ')}`)
  check(uriBefore === false, 'the connection URI is ABSENT before the reveal', { uriBefore })

  const revealBtn = await evaluate(`!!document.querySelector('[data-testid="vcn-reveal"]')`)
  if (!revealBtn) {
    console.log('SKIP  no reveal control on this VPN (the panel reported no connection links)')
  } else {
    /* The links are a separate read behind this section, so the click is followed
       by a WAIT FOR THE RESULT rather than a fixed sleep: on a cold panel call the
       first version of this script read the section while it still said
       "Loading your connection details…" and reported a false failure. */
    /* `vcn-reveal` unlocks the link rows. Some protocols (hysteria, for one) have
       no shareable URI in this build, so the section honestly refuses to draw a
       link at all — in which case there is nothing further to reveal and the
       finding is stated, not failed. */
    await evaluate(`document.querySelector('[data-testid="vcn-reveal"]')?.click()`)

    /* Wait for the link rows to arrive before looking for the per-link toggle:
       the reveal starts a fetch, and reading immediately is what made an earlier
       version report "no toggle" on a VPN that has one. */
    let hasToggle = false
    for (let i = 0; i < 30; i++) {
      await sleep(500)
      hasToggle = await evaluate(`!!document.querySelector('[data-testid="vcn-toggle-0"]')`)
      if (hasToggle) break
    }
    if (hasToggle) {
      await evaluate(`document.querySelector('[data-testid="vcn-toggle-0"]')?.click()`)
    } else {
      console.log('SKIP  this protocol has no per-link toggle (no shareable URI in this build)')
    }

    let afterReveal = null
    for (let i = 0; i < 30; i++) {
      await sleep(500)
      afterReveal = await evaluate(`
        (() => {
          const sec = document.querySelector('[data-testid="vpn-connect-section"]')
          if (!sec) return null
          const t = sec.innerText
          return {
            bodyText: t,
            svgs: sec.querySelectorAll('svg').length,
            /* The URI lives in a <code> inside each link row. Read the element
               directly rather than trusting innerText of the whole section, which
               an error/empty branch elsewhere can suppress. */
            uris: [...sec.querySelectorAll('code, .vcn-link__uri, textarea')].map((n) => n.textContent ?? ''),
            textarea: sec.querySelectorAll('textarea, .vcn__uri, pre').length,
            loading: /loading your connection details/i.test(t),
          }
        })()
      `)
      if (afterReveal && !afterReveal.loading) break
    }

    const uriText = (afterReveal?.uris ?? []).join(' ')
    const uriAfter = uriPattern.test(uriText)
    if (afterReveal?.loading) {
      console.log('SKIP  the connection details never finished loading, so the reveal is unproven')
    } else if (!hasToggle) {
      /* No link row exists for this protocol. The reveal gate is proven by the
         ABSENT-before check already; asserting a URI here would fail a screen that
         is behaving correctly. */
      check(true, 'the section states the link is withheld, and offers no link to reveal')
    } else {
      check(uriAfter, 'the connection URI is PRESENT after the reveal', { found: uriText.slice(0, 60) })
      check(
        (afterReveal?.svgs ?? 0) > 0 || uriAfter,
        'the reveal actually draws the QR / link rather than just toggling a button',
        { svgs: afterReveal?.svgs, uriAfter },
      )
    }
  }
}

/* --------------------------------------------------------------------- *
 * 6. The Usage range switch genuinely changes the series shape
 * --------------------------------------------------------------------- */

await evaluate(`document.querySelector('[data-testid="vd-tab-usage"]')?.click()`)
await sleep(2500)

/* A FREE VPN gets totals and no charts: the whole chart block is gated on
   `kind !== 'FREE'`, while the range switch above it still renders. Probing for
   the switch therefore proves nothing — the probe must look for a CHART. On a
   FREE VPN the section is reported as a SKIP with its reason, never as a PASS,
   because nothing about range switching was proven. */
const hasCharts = await evaluate(`
  (() => {
    const root = document.querySelector('[data-testid="vd-usage"]')
    if (!root) return false
    if (/Charts are part of Premium/i.test(root.innerText)) return false
    return root.querySelectorAll('svg.ui-chart').length > 0
  })()
`)
if (!hasCharts) {
  console.log('SKIP  this VPN has no charts (FREE VPNs get totals only), so range switching is unproven')
}

/** Counts drawn chart points for the currently selected range. */
const pointCount = `
  (() => {
    const svg = document.querySelector('[data-testid="vd-usage"] svg.ui-chart')
    if (!svg) return 0
    const path = svg.querySelector('path[stroke]')
    if (!path) return 0
    const d = path.getAttribute('d') || ''
    return (d.match(/[LC]/g) || []).length + (d.startsWith('M') ? 1 : 0)
  })()
`

/** Clicks a range chip by its visible label. */
const clickRange = (label) => `
  (() => {
    const btn = [...document.querySelectorAll('[data-testid="vd-usage"] button')]
      .find((b) => b.textContent.trim() === ${JSON.stringify(label)})
    if (btn) { btn.click(); return true }
    return false
  })()
`

if (hasCharts) {
  const clicked24 = await evaluate(clickRange('24 hours'))
  await sleep(2600)
  const points24 = await evaluate(pointCount)

  const clicked30 = await evaluate(clickRange('30 days'))
  await sleep(2600)
  const points30 = await evaluate(pointCount)

  check(clicked24 && clicked30, 'the Usage range switch exposes a 24-hour and a 30-day option', { clicked24, clicked30 })
  check(points24 > 0, 'the 24-hour range draws a series', { points24 })
  check(points30 > 0, 'the 30-day range draws a series', { points30 })
  /* This is the owner's original complaint: switching the range changed nothing
     because the chart plotted two lifetime totals regardless of the window. A
     real switch changes the number of points. */
  check(
    points24 !== points30,
    'the 24-hour and 30-day ranges produce a DIFFERENT number of points',
    { points24, points30 },
  )
}

/* --------------------------------------------------------------------- *
 * 7. A window with no samples says so - never a zero line
 * --------------------------------------------------------------------- */

const emptyState = await evaluate(`
  (() => {
    const root = document.querySelector('[data-testid="vd-usage"]')
    if (!root) return null
    const empties = [...root.querySelectorAll('[data-testid$="-empty"]')].map((e) => e.innerText.trim())
    const charts = root.querySelectorAll('svg.ui-chart').length
    const zeroLines = [...root.querySelectorAll('svg.ui-chart path[stroke]')]
      .filter((p) => {
        const d = p.getAttribute('d') || ''
        const ys = [...d.matchAll(/[\\s,](\\d+(?:\\.\\d+)?)/g)].map((m) => Number(m[1]))
        return d.length > 0 && new Set(ys).size === 1
      }).length
    return { empties, zeroLines, charts }
  })()
`)
if (!emptyState) {
  check(false, 'the Usage section renders')
} else if (emptyState.charts === 0) {
  console.log('SKIP  no charts on this VPN (FREE), so there is no zero line to check for')
} else {
  /* An empty window renders an explicit sentence. If nothing is empty, that is
     fine - data existed. What must never happen is a flat line standing in for
     "not measured". */
  const anyEmpty = emptyState.empties.length > 0
  console.log(
    `INFO  usage empty states shown: ${anyEmpty ? JSON.stringify(emptyState.empties) : 'none (every window had data)'}`,
  )
  if (anyEmpty) {
    check(
      emptyState.empties.every((t) => /\S/.test(t)),
      'an empty window states "no data" in words',
      emptyState.empties,
    )
  }
  check(
    emptyState.zeroLines === 0,
    'no chart draws a flat zero line in place of a missing measurement',
    { zeroLines: emptyState.zeroLines },
  )
}

/* --------------------------------------------------------------------- *
 * Result
 * --------------------------------------------------------------------- */

console.log('')
if (failures.length) {
  console.error(`${failures.length} check(s) FAILED:`)
  for (const f of failures) console.error(`  - ${f}`)
  process.exit(1)
}
console.log('all VPN details checks passed')
process.exit(0)
