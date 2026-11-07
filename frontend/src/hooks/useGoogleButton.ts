import { useEffect, useState } from 'react'
import { api, GOOGLE_START_URL } from '../lib/api'

/**
 * Whether Google sign-in is available, and what to do about it.
 *
 * The hook asks the server (`GET /api/auth/google/status`) instead of assuming:
 * a deployment without an OAuth client must show a disabled button with a reason,
 * not one that redirects into a 503. `start()` is a full navigation, so the code
 * exchange and the tokens stay entirely server-side.
 *
 * Google is a CLIENT sign-in only (D-041): staff accounts are created by a Super
 * Admin and sign in with a password, so the staff page never offers it.
 */
export function useGoogleButton(enabled = true) {
  // Derived during render, not in an effect: `enabled` is a prop, so the value is
  // known immediately and setting it from an effect would cause a second render.
  const [remote, setRemote] = useState<boolean | null>(null)
  const configured = enabled ? remote : false

  useEffect(() => {
    if (!enabled) return
    let alive = true
    api
      .googleStatus()
      .then((res) => {
        if (alive) setRemote(Boolean(res.configured))
      })
      .catch(() => {
        // Unknown is treated as unavailable: never offer a button that cannot work.
        if (alive) setRemote(false)
      })
    return () => {
      alive = false
    }
  }, [enabled])

  return {
    /** null while the status is still unknown. */
    configured,
    available: enabled && configured === true,
    start: () => {
      window.location.assign(GOOGLE_START_URL)
    },
  }
}

/** Human-readable explanation for the `?google=` code the callback redirects with. */
export function googleErrorMessage(code: string | null): string | null {
  switch (code) {
    case null:
    case '':
      return null
    case 'denied':
      return 'Google sign-in was cancelled.'
    case 'state_invalid':
      return 'That sign-in attempt expired. Please try again.'
    case 'email_unverified':
      return 'Your Google account e-mail is not verified by Google, so it cannot be linked to an existing account.'
    case 'identity_taken':
      return 'That Google account is already linked to a different Veylix account.'
    case 'session_failed':
      return 'Signed in with Google, but the session could not be created. Please try again.'
    case 'not_configured':
      return 'Google sign-in is not configured on this server.'
    case 'failed':
      // The server could not complete the exchange with Google (bad client
      // secret, a rejected redirect URI, an expired code). The reason is in the
      // backend log; the user only needs to know it is not their password.
      return 'Google could not confirm that sign-in. Please try again, or sign in with your password.'
    default:
      return 'Signing in with Google failed. Please try again or use your password.'
  }
}
