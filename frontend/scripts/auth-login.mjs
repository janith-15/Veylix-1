#!/usr/bin/env node
/**
 * Sign in for real, and hand the session cookie to a browser/CDP script.
 *
 * Why this exists: the browser verification scripts (`verify-dashboard.mjs`,
 * `verify-vpn-detail.mjs`, …) used to fake an identity by writing a dev-login key
 * into localStorage. That shortcut is gone (D-039), so a script that wants an
 * authenticated page must do what a person does: POST /api/auth/login and keep
 * the HttpOnly session cookie.
 *
 * Credentials come from `.veylix-secrets/seed.json` (git-ignored) by role, or
 * from VEYLIX_EMAIL / VEYLIX_PASSWORD. Never hard-coded.
 *
 * Usage (from frontend/):
 *   node scripts/auth-login.mjs super_admin            → prints the cookie JSON
 *   import { signIn, sessionCookie } from './auth-login.mjs'
 */
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const HERE = path.dirname(fileURLToPath(import.meta.url))
const SEED = process.env.VEYLIX_SEED_FILE ?? path.join(HERE, '..', '..', '.veylix-secrets', 'seed.json')
export const API = process.env.VEYLIX_API ?? 'http://127.0.0.1:8080'

/** Reads the demo credentials for a role from the seed file. */
export async function credentialsFor(role) {
  if (process.env.VEYLIX_EMAIL && process.env.VEYLIX_PASSWORD) {
    return { email: process.env.VEYLIX_EMAIL, password: process.env.VEYLIX_PASSWORD }
  }
  // The UI calls the role `super-admin`; seed.json (and the API) call it
  // `super_admin`. Accept both so a test never fails on the spelling.
  const wanted = String(role).replace(/-/g, '_')
  const raw = await readFile(SEED, 'utf8')
  const { accounts } = JSON.parse(raw)
  const match = accounts.find((a) => a.role === wanted || a.role === role)
  if (!match) throw new Error(`no "${role}" account in ${SEED}`)
  return { email: match.email, password: match.password }
}

/**
 * The credentials that actually work for a role: the generated
 * `.veylix-secrets/superadmin.txt` first (a reset account no longer has the seed
 * password), then seed.json.
 */
export async function workingCredentialsFor(role) {
  const wanted = String(role).replace(/-/g, '_')
  if (wanted === 'super_admin') {
    try {
      const text = await readFile(path.join(HERE, '..', '..', '.veylix-secrets', 'superadmin.txt'), 'utf8')
      const email = /email\s*:\s*(\S+)/.exec(text)?.[1]
      const password = /password\s*:\s*(\S+)/.exec(text)?.[1]
      if (email && password) return { email, password }
    } catch {
      // No generated credential: fall through to the seed file.
    }
  }
  return credentialsFor(role)
}

/**
 * POSTs /api/auth/login and returns `{ status, user, cookie }`, where `cookie` is
 * the raw `Set-Cookie` value (name=value) ready for CDP's Network.setCookie.
 */
export async function signIn(role = 'super_admin') {
  const { email, password } = await workingCredentialsFor(role)
  // D-041: staff sign in through the staff door; clients through the client door.
  const isStaff = ['super_admin', 'admin'].includes(String(role).replace(/-/g, '_'))
  // The CSRF guard (D-043) rejects a browser-shaped POST that omits `Origin`, and
  // Node's fetch sends `Sec-Fetch-*` by itself. Declaring the API origin is what a
  // real browser does, so the harness must do the same rather than look like a
  // cross-site request with no provenance.
  const res = await fetch(`${API}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Origin: API },
    body: JSON.stringify({ identifier: email, password, portal: isStaff ? 'staff' : 'client' }),
    redirect: 'manual',
  })
  const text = await res.text()
  const user = text ? JSON.parse(text) : null
  const setCookie = res.headers.getSetCookie?.() ?? []
  const raw = setCookie.find((c) => c.startsWith('veylix_session=')) ?? ''
  const cookie = raw.split(';')[0] // "veylix_session=<token>"
  return { status: res.status, user, cookie, email }
}

/** Splits a "name=value" cookie for CDP's Network.setCookie parameters. */
export function cookieParts(cookie, domain = 'localhost') {
  const [name, ...rest] = cookie.split('=')
  return { name, value: rest.join('='), domain, path: '/' }
}

// `node scripts/auth-login.mjs <role>` prints the result, which is handy in a shell.
if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  const role = process.argv[2] ?? 'super_admin'
  signIn(role)
    .then((r) => {
      console.log(JSON.stringify({ status: r.status, user: r.user, cookie: r.cookie ? '(issued)' : '(none)' }, null, 2))
      if (r.status !== 200) process.exitCode = 1
    })
    .catch((err) => {
      console.error(err.message)
      process.exitCode = 1
    })
}
