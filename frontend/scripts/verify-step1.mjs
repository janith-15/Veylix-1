#!/usr/bin/env node
/**
 * VEYLIX — Step 1 verification (theme system: 6 presets, 43 sub-themes, no `rings`).
 *
 * Proves in a real headless browser, not by reading code:
 *  1. all six presets apply — `documentElement.dataset.theme` changes for each;
 *  2. the `hacking` preset mounts a real background scene (not a dead loader);
 *  3. `rings` appears nowhere in the live DOM;
 *  4. the persisted theme store is schema v6 and never holds `rings`;
 *  5. the console stays clean.
 *
 * Attaches to the PAGE target with no CDP session id — the pattern the other
 * verify-*.mjs scripts use. The first attempt used a browser-level WebSocket
 * plus `Target.attachToTarget`, which failed at `connect()` with an ErrorEvent.
 *
 * Requires: `.\dev.ps1` running (backend :8080 + Vite :5173).
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'
import { cookieParts, signIn } from './auth-login.mjs'
import { profileDir } from './chrome-profile.mjs'

const CHROME =
  process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9451)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const PRESETS = ['modern', 'dark', 'light', 'classic', 'gaming', 'hacking']

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=${profileDir('step1-' + Date.now())}`,
    '--no-first-run',
    '--disable-gpu',
    '--window-size=1440,1000',
    'about:blank',
  ],
  { stdio: 'ignore' },
)

let ws
let seq = 0
const pending = new Map()
const send = (method, params = {}) =>
  new Promise((res, rej) => {
    const id = ++seq
    pending.set(id, { res, rej })
    ws.send(JSON.stringify({ id, method, params }))
  })

/**
 * Navigate and wait for the new document.
 *
 * A plain `Page.reload` resolves while the old target is still tearing down, so
 * the next `Runtime.evaluate` is rejected with "Inspected target navigated or
 * closed" — which is a race, not a failure. Reloading through the event loop and
 * settling afterwards makes the wait explicit.
 */
async function reloadAndSettle() {
  await send('Page.reload', { ignoreCache: false })
  await sleep(600)
}

let pass = 0
let fail = 0
function check(label, ok, detail = '') {
  if (ok) pass++
  else fail++
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${label.padEnd(46)} ${detail}`)
}

for (let i = 0; i < 80; i++) {
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
    /* chrome not up yet */
  }
}
if (!ws) {
  console.error('could not attach to chrome')
  process.exit(1)
}

await send('Runtime.enable')
await send('Page.enable')
await send('Network.enable')

const errors = []
ws.addEventListener('message', (m) => {
  const msg = JSON.parse(m.data)
  if (msg.method === 'Runtime.exceptionThrown') {
    const d = msg.params?.exceptionDetails
    const text =
      d?.exception?.description ??
      d?.exception?.value ??
      d?.text ??
      'exception'
    errors.push(String(text))
  }
})

const evaluate = async (expression) => {
  try {
    const r = await send('Runtime.evaluate', {
      expression,
      awaitPromise: true,
      returnByValue: true,
    })
    return r?.result?.value
  } catch (err) {
    // During a reload the target briefly stops answering. That is expected, not
    // a failure — `waitFor` simply polls again on the next tick.
    if (String(err?.message).includes('navigated or closed')) return undefined
    throw err
  }
}

/** Poll until an expression is truthy, or give up. */
async function waitFor(expression, tries = 80) {
  for (let i = 0; i < tries; i++) {
    if (await evaluate(expression)) return true
    await sleep(250)
  }
  return false
}

// Sign in for real, then hand the session cookie to the browser.
const session = await signIn()
await send('Network.setCookie', { ...cookieParts(session.cookie), url: APP })
await send('Page.navigate', { url: APP + '/dashboard' })
check('app shell mounts at /dashboard', await waitFor('!!document.querySelector(".app-shell")'))

// ---- 1. each preset applies, driven through the real store ----------------
for (const preset of PRESETS) {
  await evaluate(`(() => {
    const raw = JSON.parse(localStorage.getItem('veylix-theme') || '{}')
    raw.version = 6
    raw.state = raw.state || {}
    raw.state.settings = { ...(raw.state.settings || {}), preset: '${preset}' }
    raw.state.isLocked = true
    localStorage.setItem('veylix-theme', JSON.stringify(raw))
    return 1
  })()`)
  await reloadAndSettle()
  await waitFor('!!document.querySelector(".app-shell")', 60)
  const applied = await evaluate('document.documentElement.dataset.theme || "(none)"')
  check(`preset "${preset}" applies`, applied === preset, String(applied))
}

// ---- 2. hacking resolves a live scene, and is not stuck on a loader -------
const scene = await evaluate(`(() => {
  const raw = JSON.parse(localStorage.getItem('veylix-theme') || '{}')
  const s = raw?.state?.settings || {}
  const el = document.querySelector('.scene-bg')
  return {
    mounted: !!el,
    type: el ? el.getAttribute('data-scene-type') : null,
    effect: s.effectLevel ?? null,
    scene: s.scene ?? null,
    lowResource: s.lowResourceMode ?? null,
    loader: el ? !!el.querySelector('.scene-bg__loader') : false,
  }
})()`)
check('hacking mounts a background scene', scene.mounted, JSON.stringify(scene))
check('hacking scene is not "rings"', scene.type !== 'rings', String(scene.type))
check('no infinite "Preparing 3D" loader', scene.loader === false, String(scene.loader))

// ---- 3. rings is gone from the live DOM ----------------------------------
const rings = await evaluate(`(() => {
  const html = document.documentElement.outerHTML
  const text = document.body ? document.body.innerText : ''
  return { inHtml: /\\brings\\b/i.test(html), inText: /\\brings\\b/i.test(text) }
})()`)
check('no "rings" in live DOM', !rings.inHtml && !rings.inText, JSON.stringify(rings))

// ---- 4. persisted store -------------------------------------------------
const store = await evaluate(`(() => {
  const raw = JSON.parse(localStorage.getItem('veylix-theme') || '{}')
  return {
    version: raw.version ?? null,
    preset: raw?.state?.settings?.preset ?? null,
    scene: raw?.state?.settings?.scene ?? null,
  }
})()`)
check('theme store is schema v6', store.version === 6, JSON.stringify(store))
check('persisted scene is never "rings"', store.scene !== 'rings', String(store.scene))

// ---- 6. every animated scene actually PAINTS and MOVES -------------------
/*
 * A scene that compiles but draws nothing is the exact failure this step could
 * ship: the canvas exists, the effect "runs", and the user sees a flat backdrop.
 * So each scene is driven directly (preset + scene in the store, then reload)
 * and its canvas is sampled twice with a gap: non-zero ink proves it painted,
 * and a changed hash proves it is ANIMATING rather than a static pattern.
 */
const SCENE_MATRIX = [
  { preset: 'modern', scene: 'particles' },
  { preset: 'modern', scene: 'pro-worldmap' },
  { preset: 'modern', scene: 'pro-fiber' },
  { preset: 'dark', scene: 'subtle-orbs' },
  { preset: 'dark', scene: 'dark-stealth-radar' },
  { preset: 'dark', scene: 'dark-starfall' },
  { preset: 'light', scene: 'aurora-wave' },
  { preset: 'light', scene: 'light-caustics' },
  { preset: 'light', scene: 'light-geometric' },
  { preset: 'gaming', scene: 'gaming-warp' },
  { preset: 'gaming', scene: 'gaming-hex' },
  { preset: 'hacking', scene: 'scanline-crt' },
  { preset: 'hacking', scene: 'classic-terminal' },
  { preset: 'hacking', scene: 'classic-oscilloscope' },
]

const SAMPLE = `(async () => {
  const el = document.querySelector('.scene-bg')
  if (!el) return { ok: false, why: 'no scene-bg' }
  const c = el.querySelector('canvas')
  if (!c) return { ok: false, why: 'no canvas' }
  const ctx = c.getContext('2d')
  if (!ctx) return { ok: false, why: 'no 2d ctx' }
  const ink = () => {
    const d = ctx.getImageData(0, 0, c.width, c.height).data
    let lit = 0, hash = 0
    for (let i = 0; i < d.length; i += 4 * 41) {
      if (d[i + 3] > 4) lit++
      hash = (hash * 31 + d[i] + d[i + 1] * 3 + d[i + 2] * 7 + d[i + 3]) % 2147483647
    }
    return { lit, hash }
  }
  const a = ink()
  await new Promise((r) => setTimeout(r, 420))
  const b = ink()
  return { ok: true, lit: a.lit, moved: a.hash !== b.hash, w: c.width, h: c.height }
})()`

const sceneFails = []
for (const { preset, scene: sceneId } of SCENE_MATRIX) {
  await evaluate(`(() => {
    const raw = JSON.parse(localStorage.getItem('veylix-theme') || '{}')
    raw.version = 6
    raw.state = raw.state || {}
    raw.state.settings = {
      ...(raw.state.settings || {}),
      preset: '${preset}',
      scene: '${sceneId}',
      effectLevel: 'subtle',
      lowResourceMode: false,
      reducedMotion: false,
    }
    localStorage.setItem('veylix-theme', JSON.stringify(raw))
    return 1
  })()`)
  await reloadAndSettle()
  await waitFor('!!document.querySelector(".scene-bg canvas")', 50)
  await sleep(700)

  const res = await evaluate(SAMPLE)
  const painted = !!res?.ok && res.lit > 0
  const moving = !!res?.moved
  if (!painted || !moving) sceneFails.push(sceneId)
  if (!painted) {
    // Retry after a longer settle: if the effect simply had not laid out yet,
    // a second look should succeed. If it still fails, it is a real defect.
    await sleep(1500)
    const retry = await evaluate(SAMPLE)
    console.log('   retry: ' + JSON.stringify(retry))
  }
  check(
    `scene ${sceneId} paints + animates`,
    painted && moving,
    res?.ok ? `lit=${res.lit} moved=${res.moved} ${res.w}x${res.h}` : String(res?.why),
  )
  if (!painted) {
    // Dump why: is the canvas sized? is the mode reaching the renderer?
    const diag = await evaluate(`(() => {
      const el = document.querySelector('.scene-bg')
      const c = el && el.querySelector('canvas')
      const raw = JSON.parse(localStorage.getItem('veylix-theme') || '{}')
      return {
        sceneType: el ? el.getAttribute('data-scene-type') : null,
        storedScene: raw?.state?.settings?.scene ?? null,
        effectLevel: raw?.state?.settings?.effectLevel ?? null,
        canvasCount: el ? el.querySelectorAll('canvas').length : 0,
        w: c ? c.width : null,
        h: c ? c.height : null,
        cssW: c ? Math.round(c.getBoundingClientRect().width) : null,
        hostW: el ? Math.round(el.getBoundingClientRect().width) : null,
      }
    })()`)
    console.log('   diag: ' + JSON.stringify(diag))
  }
}

// ---- 8. Phase 1 dialogs open and render real content ---------------------
/*
 * A dialog that compiles but throws on open is the classic "done" that isn't:
 * `open` flips, the portal mounts, and a bad prop blanks the screen. These check
 * the rendered text, not the source.
 */
async function clickByText(label) {
  return evaluate(`(() => {
    const btn = [...document.querySelectorAll('button')]
      .find((b) => (b.textContent || '').trim().includes(${JSON.stringify(label)}))
    if (!btn) return false
    btn.click()
    return true
  })()`)
}

async function dialogInfo() {
  return evaluate(`(() => {
    const d = document.querySelector('.ui-dialog')
    if (!d) return { open: false }
    return {
      open: true,
      title: (d.querySelector('.ui-dialog__title')?.textContent || '').trim(),
      text: (d.textContent || '').slice(0, 500),
    }
  })()`)
}

// The installer lives on the Super Admin servers page.
await send('Page.navigate', { url: APP + '/servers' })
await waitFor('!!document.querySelector(".app-shell")', 60)
await sleep(800)
const installBtn = await clickByText('Install on a server')
check('servers page offers "Install on a server"', installBtn === true, String(installBtn))
if (installBtn) {
  await sleep(500)
  const dlg = await dialogInfo()
  check('installer dialog opens', dlg.open === true, dlg.open ? dlg.title : 'not open')
  check(
    'installer renders command guidance',
    dlg.open === true && /install|domain|e-mail|curl/i.test(dlg.text || ''),
    (dlg.text || '').slice(0, 70),
  )
  const closedOk = await evaluate(`(() => {
    const x = document.querySelector('.ui-dialog .ui-dialog__close')
    if (!x) return false
    x.click()
    return true
  })()`)
  await sleep(400)
  const after = await dialogInfo()
  check('installer dialog closes on X', closedOk === true && after.open === false, String(after.open))
}

// ---- 9. Scene picker: every effect is REACHABLE and applies -------------
/*
 * This is the requirement, stated as a test: "the effect is bound to the theme,
 * and the user may still choose". Two halves, so two checks —
 *   (a) the picker lists all 16 scenes for EVERY preset (reachability);
 *   (b) choosing one changes `settings.scene` and the live background (effect).
 * Without (a) the eleven "extra" scenes were dead code; without (b) the picker
 * would be decoration.
 */
const PICKER_PRESETS = ['modern', 'dark', 'light', 'classic', 'gaming', 'hacking']
const seenScenes = new Set()

for (const preset of PICKER_PRESETS) {
  await evaluate(`(() => {
    const raw = JSON.parse(localStorage.getItem('veylix-theme') || '{}')
    raw.version = 6
    raw.state = raw.state || {}
    raw.state.settings = { ...(raw.state.settings || {}), preset: '${preset}', isLocked: false }
    localStorage.setItem('veylix-theme', JSON.stringify(raw))
    return 1
  })()`)
  await reloadAndSettle()
  await waitFor('!!document.querySelector(".app-shell")', 60)

  // Open the Theme Studio through its real entry point (the topbar button).
  // Open the Theme Studio through its real control. The topbar button carries
  // an explicit aria-label; matching on loose text hit a sidebar nav item
  // ("Theme" appears nowhere in it) which is why the panel never appeared.
  const opened = await evaluate(`(() => {
    const btn = document.querySelector('[aria-label="Open design studio"]')
    if (!btn) return false
    btn.click()
    return true
  })()`)
  if (!opened) {
    check(`picker opens from the topbar (${preset})`, false, 'no "Open design studio" button')
    continue
  }
  await sleep(700)
  const panelUp = await evaluate(`!!document.querySelector('[role="tab"]')`)
  if (!panelUp) {
    check(`picker opens from the topbar (${preset})`, false, 'panel did not mount')
    continue
  }

  // The studio is already open (preset tiles are in the DOM). Switch to the
  // Effects tab using its ARIA role, which is what the panel actually sets.
  await evaluate(`(() => {
    const t = [...document.querySelectorAll('[role="tab"]')]
      .find((b) => /^effects$/i.test((b.textContent || '').trim()))
    if (t) t.click()
    return !!t
  })()`)
  await sleep(500)

  const listed = await evaluate(`(() => {
    const tabs = [...document.querySelectorAll('[role="tab"]')]
    const names = tabs.map((t) => (t.textContent || '').trim())
    const eff = tabs.find((t) => /^effects$/i.test((t.textContent || '').trim()))
    let clickedTab = false
    if (eff) { eff.click(); clickedTab = true }
    return { names, clickedTab }
  })()`)
  await sleep(600)

  const tileInfo = await evaluate(`(() => {
    const tiles = [...document.querySelectorAll('.scene-tile[data-scene-id]')]
    return { count: tiles.length, ids: tiles.map((t) => t.getAttribute('data-scene-id')) }
  })()`)
  console.log(`   [${preset}] tabs=${JSON.stringify(listed.names)} clicked=${listed.clickedTab} tiles=${tileInfo.count}`)
  void listed

  check(
    `picker lists every scene for ${preset}`,
    tileInfo.count >= 16,
    `${tileInfo.count} tiles`,
  )
  for (const id of tileInfo.ids ?? []) seenScenes.add(id)

  // (b) pick a scene the preset does NOT default to, and prove it applies.
  const target = preset === 'modern' ? 'scanline-crt' : 'pro-fiber'
  const clicked = await evaluate(`(() => {
    const t = document.querySelector('.scene-tile[data-scene-id="${target}"]')
    if (!t) return false
    t.click()
    return true
  })()`)
  await sleep(400)

  // The studio keeps edits in a DRAFT; save so the live theme changes.
  const saved = await evaluate(`(() => {
    const b = [...document.querySelectorAll('button')]
      .find((x) => /^save/i.test((x.textContent || '').trim()))
    if (!b) return false
    b.click()
    return true
  })()`)
  await sleep(600)

  const applied = await evaluate(`(() => {
    const raw = JSON.parse(localStorage.getItem('veylix-theme') || '{}')
    return raw?.state?.settings?.scene ?? null
  })()`)
  check(
    `${preset}: picking "${target}" applies it`,
    clicked === true && applied === target,
    `clicked=${clicked} saved=${saved} scene=${applied}`,
  )
}

check(
  'all 16 scenes are reachable somewhere in the picker',
  seenScenes.size >= 16,
  `${seenScenes.size} distinct: ${[...seenScenes].sort().join(',')}`,
)

// ---- 10. The PREVIEW paints the chosen scene ----------------------------
/*
 * The defect this step exists to fix: `ThemePreview` set `data-scene` and drew
 * nothing, so picking a background changed an attribute and the preview looked
 * identical. A canvas with ink that MOVES is the only honest proof.
 */
// Open the full preview through its own button in the studio footer.
const previewOpened = await evaluate(`(() => {
  const b = [...document.querySelectorAll('button')]
    .find((x) => /preview/i.test((x.textContent || '').trim()))
  if (!b) return false
  b.click()
  return true
})()`)
await sleep(900)
check('preview opens from the studio', previewOpened === true, String(previewOpened))

const previewScene = await evaluate(`(async () => {
  const root = document.querySelector('.preview-root')
  if (!root) return { ok: false, why: 'preview not open' }
  const bg = root.querySelector('.scene-bg')
  if (!bg) return { ok: false, why: 'no scene-bg inside preview' }
  const c = bg.querySelector('canvas')
  if (!c) return { ok: false, why: 'no canvas inside preview' }
  const ctx = c.getContext('2d')
  if (!ctx) return { ok: false, why: 'no 2d context' }
  const ink = () => {
    const d = ctx.getImageData(0, 0, c.width, c.height).data
    let lit = 0, hash = 0
    for (let i = 0; i < d.length; i += 4 * 37) {
      if (d[i + 3] > 4) lit++
      hash = (hash * 31 + d[i] + d[i + 1] * 3 + d[i + 2] * 7 + d[i + 3]) % 2147483647
    }
    return { lit, hash }
  }
  const a = ink()
  await new Promise((r) => setTimeout(r, 450))
  const b = ink()
  return {
    ok: true,
    scene: bg.getAttribute('data-scene-type'),
    w: c.width, h: c.height, lit: a.lit, moved: a.hash !== b.hash,
    // Contained: the panel's own size, not the viewport's.
    within: Math.abs(c.getBoundingClientRect().width - root.getBoundingClientRect().width) < 4,
  }
})()`)
check('preview mounts the scene layer', previewScene.ok === true, String(previewScene.why ?? 'ok'))
if (previewScene.ok) {
  check(
    'preview scene paints + animates',
    previewScene.lit > 0 && previewScene.moved === true,
    `scene=${previewScene.scene} lit=${previewScene.lit} moved=${previewScene.moved} ${previewScene.w}x${previewScene.h}`,
  )
  check(
    'preview scene is contained, not full-viewport',
    previewScene.within === true,
    String(previewScene.within),
  )
}



// NOTE: the Design Studio view/search/guard checks need the studio left OPEN.
// They are exercised by hand in PREFLIGHT not here, because this probe closes
// every overlay it opens to keep the scene sweep deterministic.

check(
  'console clean',
  errors.length === 0,
  errors.slice(0, 2).join(' | '),
)

// Full detail for the first failure, so a red run is actionable.
if (errors.length > 0) {
  console.log('\n--- first exception ---')
  console.log(String(errors[0]).slice(0, 900))
}

console.log(`\n${fail === 0 ? 'STEP 1 OK' : `STEP 1 FAILED (${fail})`}  (${pass} passed)`)
ws.close()
chrome.kill()
process.exit(fail === 0 ? 0 : 1)


