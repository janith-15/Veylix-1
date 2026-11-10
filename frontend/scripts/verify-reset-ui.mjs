import { profileDir } from './chrome-profile.mjs'
/**
 * VEYLIX — password-reset UI flow in a headless browser.
 *
 *   node scripts/verify-reset-ui.mjs <identifier> <code> <new-password>
 *
 * Types a real identifier into /forgot, submits it, follows the "I have the code"
 * step to /reset, types the code and a new password, and reports the outcome.
 *
 * It exists because rendering a page and USIING it are different claims: the
 * earlier checks only proved /forgot and /reset paint. This one proves the form
 * posts to the API and the success state appears.
 *
 * Requires: .\dev.ps1 running, and a code from ".\dev.ps1 -ResetLink <identifier>".
 */
import { spawn } from 'node:child_process'
import { setTimeout as sleep } from 'node:timers/promises'

const CHROME =
  process.env.VEYLIX_CHROME ?? 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const PORT = Number(process.env.VEYLIX_CDP_PORT ?? 9341)
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'

const [identifier, code, newPassword] = process.argv.slice(2)
if (!identifier || !code || !newPassword) {
  console.log('usage: node scripts/verify-reset-ui.mjs <identifier> <code> <new-password>')
  process.exit(2)
}

const chrome = spawn(
  CHROME,
  [
    '--headless=new',
    `--remote-debugging-port=${PORT}`,
    '--user-data-dir=' + profileDir('reset-check'),
    '--no-first-run',
    '--disable-gpu',
    '--window-size=1440,1000',
    'about:blank',
  ],
  { stdio: 'ignore' },
)

async function pageTarget() {
  for (let i = 0; i < 40; i++) {
    try {
      const list = await (await fetch(`http://127.0.0.1:${PORT}/json/list`)).json()
      const page = list.find((t) => t.type === 'page')
      if (page) return page
    } catch {
      /* not up yet */
    }
    await sleep(250)
  }
  throw new Error('Chrome DevTools endpoint never appeared')
}

const target = await pageTarget()
const ws = new WebSocket(target.webSocketDebuggerUrl)
await new Promise((res, rej) => {
  ws.onopen = res
  ws.onerror = rej
})

let seq = 0
const pending = new Map()
const consoleErrors = []
ws.onmessage = (m) => {
  const msg = JSON.parse(m.data)
  if (msg.id && pending.has(msg.id)) {
    pending.get(msg.id)(msg)
    pending.delete(msg.id)
  }
  if (msg.method === 'Runtime.exceptionThrown') {
    consoleErrors.push('EXCEPTION ' + (msg.params?.exceptionDetails?.text ?? ''))
  }
  if (msg.method === 'Runtime.consoleAPICalled' && msg.params?.type === 'error') {
    consoleErrors.push('CONSOLE ' + (msg.params.args ?? []).map((a) => a.value ?? a.description).join(' '))
  }
}
const send = (method, params = {}) =>
  new Promise((res) => {
    const id = ++seq
    pending.set(id, res)
    ws.send(JSON.stringify({ id, method, params }))
  })

await send('Runtime.enable')
await send('Page.enable')

async function goto(path) {
  await send('Page.navigate', { url: `${APP}${path}` })
  await sleep(1200)
}

async function evaluate(expression) {
  const r = await send('Runtime.evaluate', {
    expression,
    awaitPromise: true,
    returnByValue: true,
  })
  return r.result?.result?.value
}

/** Types into a React-controlled input the way a user does. */
async function typeInto(selector, value) {
  return evaluate(`(() => {
    const el = document.querySelector(${JSON.stringify(selector)});
    if (!el) return 'missing';
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
    setter.call(el, ${JSON.stringify(value)});
    el.dispatchEvent(new Event('input', { bubbles: true }));
    return 'ok';
  })()`)
}

async function clickText(text) {
  return evaluate(`(() => {
    const btn = [...document.querySelectorAll('button')].find((b) => b.textContent.includes(${JSON.stringify(text)}));
    if (!btn) return 'missing';
    btn.click();
    return 'ok';
  })()`)
}

const out = { path: '/forgot', steps: [], consoleErrors }

// --- step 1: /forgot -------------------------------------------------------
// NOTE: submitting /forgot ISSUES A NEW CODE and supersedes the one the operator
// obtained from ".\dev.ps1 -ResetLink". So the forgot step is exercised only when
// SKIP_FORGOT is not set; the default run proves the /reset page against the code
// that was actually handed over.
const SKIP_FORGOT = process.env.VEYLIX_SKIP_FORGOT === '1'

await goto('/forgot')
out.forgotHeading = await evaluate(`document.querySelector('h1.auth__title')?.textContent ?? ''`)
if (SKIP_FORGOT) {
  out.steps.push({ step: 'forgot page rendered', heading: out.forgotHeading })
} else {
  out.steps.push({
    step: 'forgot page',
    heading: out.forgotHeading,
    typed: await typeInto('input[autocomplete="username"]', identifier),
  })
  out.steps.push({ step: 'submit forgot', clicked: await clickText('Send me a code') })
  await sleep(1500)
  out.steps.push({
    step: 'outcome shown',
    notice: await evaluate(`document.querySelector('.auth__notice')?.textContent ?? ''`),
    hasContinue: await evaluate(
      `!![...document.querySelectorAll('button')].find((b) => b.textContent.includes('I have the code'))`,
    ),
  })
}

// --- step 2: /reset --------------------------------------------------------
await goto(`/reset?identifier=${encodeURIComponent(identifier)}`)
out.resetHeading = await evaluate(`document.querySelector('h1.auth__title')?.textContent ?? ''`)
out.steps.push({
  step: 'typed code',
  typed: await typeInto('input[inputmode="numeric"]', code),
})
out.steps.push({
  step: 'typed password',
  typed: await typeInto('input[autocomplete="new-password"]', newPassword),
})
out.steps.push({ step: 'submit reset', clicked: await clickText('Set new password') })

// The success state replaces the form; poll for it rather than guessing a delay.
let success = ''
let hasGoToSignIn = false
for (let i = 0; i < 20; i++) {
  await sleep(250)
  success = await evaluate(`document.querySelector('.auth__notice')?.textContent ?? ''`)
  hasGoToSignIn = await evaluate(
    `!![...document.querySelectorAll('button')].find((b) => b.textContent.includes('Go to sign in'))`,
  )
  if (hasGoToSignIn) break
}
// A refusal shows the reason on the password field; capture it for the report.
out.resetError = await evaluate(
  `document.querySelector('.ui-field__error, [role="alert"]')?.textContent ?? ''`,
)
out.steps.push({ step: 'success state', notice: success, hasGoToSignIn })
out.url = await evaluate('location.pathname')

console.log(JSON.stringify(out, null, 2))

ws.close()
chrome.kill()
process.exit(out.steps.find((s) => s.hasGoToSignIn) ? 0 : 1)
