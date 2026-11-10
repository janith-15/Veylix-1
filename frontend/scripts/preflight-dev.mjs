#!/usr/bin/env node
/**
 * VEYLIX auth preflight — proves the REAL login works, end to end.
 *
 * This replaced the dev-login verifier when the development bypass was removed
 * (D-039). It does what a person does: sign in, call the API with the session
 * cookie, check that each role is refused what it must not see, and that
 * anonymous callers get 401.
 *
 * Run it after `.\dev.ps1 -Restart`:
 *   cd frontend ; npm run preflight          (or .\dev.ps1 -Restart -Preflight)
 *
 * Exit 0 = every check passed; 1 = at least one FAIL (printed with the reason).
 * Credentials come from .veylix-secrets/seed.json. No password is ever printed.
 */
import { signIn, workingCredentialsFor } from './auth-login.mjs'

const API = process.env.VEYLIX_API ?? 'http://127.0.0.1:8080'
const APP = process.env.VEYLIX_APP ?? 'http://localhost:5173'
const TIMEOUT_MS = 8000

const results = []
function check(group, name, ok, detail) {
  results.push({ group, name, ok, detail })
}

/**
 * One HTTP exchange. `cookie` is a raw "name=value" string or undefined.
 *
 * `origin: true` sends the request's own Origin, which every browser does on a
 * same-origin POST. The CSRF guard (D-043) refuses a browser-shaped POST that
 * omits it, and Node's fetch always sends `Sec-Fetch-*`, so a state-changing
 * call must declare where it came from to be treated like the app it stands in
 * for.
 */
async function call(path, { method = 'GET', cookie, body, base = API, accept, origin = false } = {}) {
  const headers = { Accept: accept ?? 'application/json' }
  if (cookie) headers.Cookie = cookie
  if (origin) headers.Origin = base
  let payload
  if (body !== undefined) {
    payload = JSON.stringify(body)
    headers['Content-Type'] = 'application/json'
  }
  try {
    const res = await fetch(`${base}${path}`, {
      method,
      headers,
      body: payload,
      signal: AbortSignal.timeout(TIMEOUT_MS),
      redirect: 'manual',
    })
    const text = await res.text()
    let json = null
    try {
      json = text ? JSON.parse(text) : null
    } catch {
      json = null
    }
    const setCookie = res.headers.getSetCookie?.() ?? []
    const session = setCookie.find((c) => c.startsWith('veylix_session='))
    return { status: res.status, json, raw: text, cookie: session ? session.split(';')[0] : undefined }
  } catch (err) {
    return { status: 0, json: null, raw: '', error: err.message }
  }
}

/** The app's origin, trying IPv4 and IPv6 spellings (Vite may bind [::1] only). */
async function callApp(path, opts) {
  const u = new URL(APP)
  const alt = u.hostname === 'localhost' ? '127.0.0.1' : 'localhost'
  for (const base of [APP, `${u.protocol}//${alt}:${u.port}`]) {
    const r = await call(path, { ...opts, base })
    if (r.status !== 0) return { ...r, origin: base }
  }
  return { status: 0, json: null, raw: '', origin: APP, error: 'not reachable' }
}


/* ------------------------------------------------------------------ checks */

async function checkAnonymous() {
  for (const path of ['/api/me', '/api/admins', '/api/servers', '/api/vpns', '/api/clients']) {
    const r = await call(path)
    check('anonymous', `GET ${path} → 401`, r.status === 401, `got ${r.status}`)
  }
  // A wrong password must be refused, and must not mint a session.
  const wrong = await call('/api/auth/login', {
    method: 'POST',
    origin: true,
    body: { email: 'nobody@veylix.test', password: 'wrong-password-123' },
  })
  check('anonymous', 'wrong credentials → 401 invalid_credentials',
    wrong.status === 401 && wrong.json?.error?.code === 'invalid_credentials',
    `got ${wrong.status} ${wrong.json?.error?.code ?? ''}`)
}

async function checkSignIn(role, expectRole) {
  const r = await signIn(role)
  check('sign in', `${role} signs in`, r.status === 200 && r.user?.role === expectRole,
    r.status === 200 ? `id=${r.user.id}, verified=${r.user.email_verified}` : `HTTP ${r.status}`)
  if (r.status !== 200) return null

  const me = await call('/api/me', { cookie: r.cookie })
  check('sign in', `${role} GET /api/me`, me.status === 200 && me.json?.role === expectRole,
    `${me.status} role=${me.json?.role}`)
  return r.cookie
}

async function checkPermissions(cookies) {
  const matrix = [
    ['super_admin', '/api/admins', 200],
    ['super_admin', '/api/audit', 200],
    ['super_admin', '/api/clients', 200],
    ['admin', '/api/admins', 403],
    ['admin', '/api/audit', 403],
    ['admin', '/api/clients', 200],
    ['client', '/api/admins', 403],
    ['client', '/api/clients', 403],
    ['client', '/api/audit', 403],
  ]
  for (const [role, path, want] of matrix) {
    const cookie = cookies[role]
    if (!cookie) {
      check('permissions', `${role} GET ${path} → ${want}`, false, 'no session (sign-in failed)')
      continue
    }
    const r = await call(path, { cookie })
    check('permissions', `${role} GET ${path} → ${want}`, r.status === want, `got ${r.status}`)
  }
}

async function checkPortals() {
  // D-041: two doors. Each account must be REFUSED at the other one, with the
  // precise `wrong_portal` code, and accepted at its own.
  //
  // NOTE: this runs AFTER the sign-in checks, and the login rate limiter allows a
  // small burst per account. The staff-door success case is therefore the only
  // one that spends its allowance here; the refusals are checked for the CODE, so
  // a 429 (already-throttled account) is reported as such rather than as a
  // mysterious failure.
  const cases = [
    ['super_admin', 'staff', 200, null],
    ['super_admin', 'client', 403, 'wrong_portal'],
    ['admin', 'staff', 200, null],
    ['admin', 'client', 403, 'wrong_portal'],
    ['client', 'client', 200, null],
    ['client', 'staff', 403, 'wrong_portal'],
  ]
  for (const [role, portal, want, code] of cases) {
    let creds
    try {
      creds = await workingCredentialsFor(role)
    } catch (err) {
      check('portals', `${role} @ ${portal} door`, false, err.message)
      continue
    }
    const r = await call('/api/auth/login', {
      method: 'POST',
      origin: true,
      body: { identifier: creds.email, password: creds.password, portal },
    })
    const gotCode = r.json?.error?.code ?? null
    const ok = r.status === want && (code === null || gotCode === code)
    check('portals', `${role} @ ${portal} door → ${want}${code ? ' ' + code : ''}`, ok,
      `${r.status}${gotCode ? ' ' + gotCode : ''}${r.status === 429 ? ' (rate limited — see note)' : ''}`)
  }
}

async function checkSessionLifecycle() {
  // Logout must end the session: the same cookie stops working.
  const r = await signIn('client')
  if (r.status !== 200) {
    check('session', 'logout ends the session', false, 'sign-in failed')
    return
  }
  const before = await call('/api/me', { cookie: r.cookie })
  const out = await call('/api/auth/logout', { method: 'POST', origin: true, cookie: r.cookie })
  const after = await call('/api/me', { cookie: r.cookie })
  check('session', 'logout ends the session',
    before.status === 200 && out.status === 204 && after.status === 401,
    `before=${before.status} logout=${out.status} after=${after.status}`)
}

async function checkFrontend() {
  const page = await callApp('/', { accept: 'text/html' })
  check('frontend', `${APP}/ serves the app`, page.status === 200 && /<html/i.test(page.raw ?? ''),
    `HTTP ${page.status} on ${page.origin}`)
  const login = await callApp('/login', { accept: 'text/html' })
  check('frontend', `${APP}/login loads`, login.status === 200 && /<html/i.test(login.raw ?? ''),
    `HTTP ${login.status} on ${login.origin}`)
  // The browser talks to the API through the dev proxy; a stranger must be refused.
  const proxied = await callApp('/api/me', { accept: 'application/json' })
  check('frontend', 'proxy answers /api/me → 401 for a stranger', proxied.status === 401, `got ${proxied.status}`)
}

/* -------------------------------------------------------------------- main */

function print() {
  const groups = new Map()
  for (const r of results) {
    if (!groups.has(r.group)) groups.set(r.group, [])
    groups.get(r.group).push(r)
  }
  for (const [group, rows] of groups) {
    console.log(`\n== ${group} ==`)
    for (const r of rows) console.log(`  ${r.ok ? 'PASS' : 'FAIL'}  ${r.name}${r.detail ? `  (${r.detail})` : ''}`)
  }
}

async function main() {
  const health = await call('/api/health')
  if (health.status !== 200) {
    check('backend', `is up on ${API}`, false,
      `HTTP ${health.status || health.error} — start it with .\\dev.ps1 -Restart`)
    print()
    console.log('\nPREFLIGHT FAILED — the backend is not answering.')
    return 1
  }
  check('backend', `is up on ${API}`, true, 'ok')

  await checkAnonymous()

  const cookies = {
    super_admin: await checkSignIn('super_admin', 'super_admin'),
    admin: await checkSignIn('admin', 'admin'),
    client: await checkSignIn('client', 'client'),
  }

  if (!cookies.super_admin) {
    try {
      const { email } = await credentialsFor('super_admin')
      check('accounts', 'super_admin account exists', false,
        `no account ${email} — run .\\dev.ps1 -Seed (or go run ./cmd/seeddemo)`)
    } catch (err) {
      check('accounts', 'seed.json readable', false, err.message)
    }
  } else {
    const admins = await call('/api/admins?role=admin', { cookie: cookies.super_admin })
    check('accounts', 'super_admin can list admins',
      admins.status === 200 && Array.isArray(admins.json),
      `HTTP ${admins.status}, ${Array.isArray(admins.json) ? admins.json.length : 0} admin(s)`)
  }

  await checkPermissions(cookies)
  await checkPortals()
  await checkSessionLifecycle()
  await checkFrontend()

  print()
  const failed = results.filter((r) => !r.ok)
  if (failed.length === 0) {
    console.log(`\nPREFLIGHT OK — ${results.length} checks passed.`)
    return 0
  }
  console.log(`\nPREFLIGHT FAILED — ${failed.length} of ${results.length} checks failed:`)
  for (const f of failed) console.log(`  - [${f.group}] ${f.name} (${f.detail})`)
  return 1
}

main().then((code) => process.exit(code))
