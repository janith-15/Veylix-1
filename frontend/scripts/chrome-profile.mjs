#!/usr/bin/env node
/**
 * One place that decides WHERE a headless-Chrome verification run keeps its
 * profile directory.
 *
 * Why this exists: every verifier used to pass its own literal
 * `--user-data-dir=C:\Windows\Temp\veylix-…`. On a normal Windows install that
 * folder is writable only by an elevated process, so Chrome silently failed to
 * create its cache and the page under test rendered EMPTY. The script then
 * reported a zero-metric page — which reads exactly like a broken screen:
 *
 *   "metrics": [], "tableRows": [], "cardTitles": [],
 *   "consoleErrors": ["Failed to load resource: net::ERR_CACHE_READ_FAILURE"]
 *
 * The screen was fine. The profile was not. A verification tool that fails for
 * an environment reason must fail *because of that reason*, never by blaming
 * the product (IMPROVEMENT_LEDGER — a probe must not invent a defect).
 *
 * `os.tmpdir()` resolves to the per-user temp (`%LOCALAPPDATA%\Temp` on Windows,
 * `/tmp` elsewhere), which the process is always allowed to write.
 *
 * Usage:
 *   import { profileDir } from './chrome-profile.mjs'
 *   `--user-data-dir=${profileDir('dashboard')}`
 *
 * A caller that needs a *persistent* profile (theme checks that rely on
 * localStorage) keeps a stable name; a caller that only needs a throwaway one
 * passes a unique suffix (pid + timestamp) as before.
 */
import os from 'node:os'
import path from 'node:path'

/** Absolute, user-writable Chrome profile path for `name`. */
export function profileDir(name) {
  return path.join(os.tmpdir(), `veylix-${name}`)
}

/**
 * A profile directory unique to THIS process and THIS moment.
 *
 * Use it in any verifier that does not need to persist browser storage.
 * `profileDir(name)` is stable by design, so two runs of the same verifier
 * back-to-back collide: the second Chrome finds the profile still locked by the
 * first, refuses to start, and the port poll then attaches to the FOREIGN
 * browser that is still shutting down. The script reports `not-enough-rows` or
 * `intercepted: 0` and blames the product for a page it never loaded
 * (IMPROVEMENT_LEDGER — a probe must not invent a defect).
 */
export function profileDirUnique(name) {
  return path.join(os.tmpdir(), `veylix-${name}-${process.pid}-${Date.now().toString(36)}`)
}

/** True when something already answers on the Chrome DevTools endpoint. */
export async function portInUse(port) {
  try {
    await fetch(`http://127.0.0.1:${port}/json/version`)
    return true
  } catch {
    return false
  }
}

/**
 * Refuse to verify on a port another browser already owns.
 *
 * Exiting with code 2 (environment problem, not a product failure) is the whole
 * point: attaching to a foreign Chrome silently verifies a different page.
 */
export async function assertPortFree(port) {
  if (await portInUse(port)) {
    console.error(
      `port ${port} already serves a Chrome DevTools endpoint.\n` +
        'Another verification run (or a stale browser) still owns it.\n' +
        'Set VEYLIX_CDP_PORT to a free port, or close the other run first.',
    )
    process.exit(2)
  }
}

/**
 * Wait for a spawned Chrome to really disappear before the next run starts.
 *
 * `chrome.kill()` only sends the signal. A verifier that exits immediately hands
 * the next run both the locked profile AND the live debug port.
 */
export async function waitForExit(child, timeoutMs = 10000) {
  if (!child || child.exitCode !== null) return true
  const exited = new Promise((resolve) => child.once('exit', () => resolve(true)))
  const timedOut = new Promise((resolve) => setTimeout(() => resolve(false), timeoutMs))
  const ok = await Promise.race([exited, timedOut])
  if (!ok) {
    try {
      child.kill('SIGKILL')
    } catch {
      /* already gone */
    }
  }
  return ok
}
export default profileDir
