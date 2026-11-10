import { create } from 'zustand'
import { api, ApiError, type ApiPortal, type ApiRole } from '../lib/api'
import { setActiveTimezone } from '../lib/time'

/** The three UI roles (the API spells the first one `super_admin`). */
export type UserRole = 'super-admin' | 'admin' | 'client'

/**
 * Session state backed by the real Go backend (Stage E2).
 *
 * The backend owns the session in an HttpOnly cookie, so this store never
 * holds a credential — it only mirrors the resolved user for the UI. Roles are
 * decided server-side and merely translated here.
 *
 * There is no shortcut identity: every request is authenticated by that cookie
 * (the development bypass was removed on 2026-10-28, D-039).
 *
 * `status` drives the app shell:
 *   unknown  — booting (checking the cookie)
 *   anon     — no session
 *   authed   — user present
 */

export interface AuthUser {
  name: string
  email: string
  role: UserRole
  id: number
  /** Unix seconds; present once the account row has been read (D2). */
  createdAt?: number
  emailVerified?: boolean
}

type AuthStatus = 'unknown' | 'anon' | 'authed'

interface AuthState {
  user: AuthUser | null
  status: AuthStatus
  error: string | null
  pending: boolean
  bootstrapRequired: boolean
  /**
   * True when the last sign-in was refused with `email_unverified` (D-078).
   *
   * It is a flag rather than a string so the form can offer the ONE action that
   * helps — sending a new link — instead of leaving the person reading a message
   * about a link they may never have received.
   */
  needsVerification: boolean

  /** Restore the session from the cookie on app start. */
  hydrate: () => Promise<void>
  signIn: (identifier: string, password: string, portal?: ApiPortal) => Promise<boolean>
  /**
   * Sign up (D-076).
   *
   * Resolves to `'authed'` when the server actually created a session, and to
   * `'pending'` when the account exists but must be confirmed first. The caller
   * MUST branch on it: pretending a pending account is signed in is exactly the
   * bug this replaces — the session cookie outlived one page load, and the same
   * credentials were then refused at the login door forever.
   */
  signUp: (name: string, email: string, password: string) => Promise<'authed' | 'pending' | null>
  bootstrap: (name: string, email: string, password: string) => Promise<boolean>
  /** Rotate the password. Resolves to null on success, else a user-safe reason. */
  changePassword: (
    currentPassword: string,
    newPassword: string,
  ) => Promise<{ error: string | null; delivery?: 'smtp' | 'outbox' }>
  signOut: () => Promise<void>
  /**
   * Ends every OTHER session for this account (D3). The current browser stays
   * signed in. Returns how many sessions the server ended, or an error string —
   * the screen must be able to say which, never "done" on a failed call.
   */
  signOutOthers: () => Promise<{ ended?: number; error?: string }>
  /** Sends a NEW confirmation link (D-078). Safe to call without a session. */
  resendVerification: (identifier: string) => Promise<{ error: string | null; note?: string }>
  clearError: () => void
}

/** Backend role (snake_case) — UI role (hyphenated). */
function toUserRole(role: ApiRole): UserRole {
  if (role === 'super_admin') return 'super-admin'
  return role
}

function toUser(u: {
  id: number
  email: string
  display_name?: string
  role: ApiRole
  created_at?: number
  email_verified?: boolean
}): AuthUser {
  return {
    id: u.id,
    email: u.email,
    name: u.display_name || u.email.split('@')[0] || u.email,
    role: toUserRole(u.role),
    createdAt: u.created_at,
    emailVerified: u.email_verified,
  }
}

/** Asks the backend whether the installation still needs its first super admin. */
async function detectBootstrap(): Promise<boolean> {
  try {
    const { bootstrap_required } = await api.setupStatus()
    return bootstrap_required
  } catch {
    // If the status call fails we must NOT offer a setup wizard: that would let
    // a transient network error look like "no accounts exist".
    return false
  }
}

function message(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case 'invalid_credentials':
        return 'Email or password is incorrect.'
      case 'weak_password':
        return err.message || 'Password does not meet the policy.'
      case 'password_too_long':
        return 'Password is too long.'
      case 'password_mismatch':
        return 'Your current password is incorrect.'
      case 'email_taken':
        return 'That email is already registered.'
      case 'rate_limited':
        return 'Too many attempts. Please wait a few minutes.'
      case 'network_error':
        return 'Cannot reach the server. Is the backend running?'
      case 'bootstrap_complete':
        return 'Setup has already been completed. Please sign in.'
      // D-041: the two doors. These are precise on purpose — "wrong password"
      // would send the person to a password reset they do not need.
      case 'wrong_portal':
        return 'This account signs in through the other login page (clients use /login, staff use /staff/login).'
      case 'password_not_set':
        // A Google-only account has no password to be wrong. "Wrong password" would
        // send the customer to a reset flow that cannot help them.
        return 'This account was created with Google — use “Continue with Google” to sign in, or ask the seller to issue you a password from the console.'
      case 'email_unverified':
        return 'Confirm your e-mail address first — we sent you a code.'
      default:
        return err.message
    }
  }
  return 'Something went wrong. Please try again.'
}

export const useAuthStore = create<AuthState>()((set) => ({
  user: null,
  status: 'unknown',
  error: null,
  pending: false,
  bootstrapRequired: false,
  needsVerification: false,

  hydrate: async () => {
    try {
      const me = await api.me()
      // D-099: adopt the server's display zone before any screen renders a
      // stored instant. Set here rather than in a component because a card can
      // render its loading skeleton first — and a timestamp drawn in the
      // device's zone, then silently redrawn in the server's, is a visible jump
      // the customer has no way to explain.
      setActiveTimezone(me.display_timezone)
      set({ user: toUser(me), status: 'authed', error: null, bootstrapRequired: false })
    } catch (err) {
      if (err instanceof ApiError && err.isAuthError) {
        // No session. Ask the backend whether this install still needs its
        // first super admin, so the UI can show the setup wizard instead of a
        // login form nobody can use yet.
        set({ user: null, status: 'anon', bootstrapRequired: await detectBootstrap() })
        return
      }
      // Backend unreachable: stay anon but tell the user why.
      set({ user: null, status: 'anon', error: message(err) })
    }
  },

  signIn: async (identifier, password, portal = 'client') => {
    set({ pending: true, error: null, needsVerification: false })
    try {
      const u = await api.login(identifier, password, portal)
      set({ user: toUser(u), status: 'authed', pending: false })
      return true
    } catch (err) {
      // `email_unverified` is not a failed password — it is a door that opens
      // once the address is confirmed. Remembering that lets the form offer the
      // resend instead of a dead end.
      const unverified = err instanceof ApiError && err.code === 'email_unverified'
      set({ pending: false, error: message(err), needsVerification: unverified })
      return false
    }
  },

  signUp: async (name, email, password) => {
    set({ pending: true, error: null })
    try {
      const u = await api.register(email, password, name)
      // 202: the account exists, but no session was issued — it cannot sign in
      // until the address is confirmed. Do NOT set `authed`.
      if ('pending_verification' in u && u.pending_verification) {
        set({ pending: false, status: 'anon', user: null })
        return 'pending'
      }
      set({ user: toUser(u), status: 'authed', pending: false })
      return 'authed'
    } catch (err) {
      set({ pending: false, error: message(err) })
      return null
    }
  },

  bootstrap: async (name, email, password) => {
    set({ pending: true, error: null })
    try {
      const u = await api.bootstrap(email, password, name)
      set({ user: toUser(u), status: 'authed', pending: false, bootstrapRequired: false })
      return true
    } catch (err) {
      const required = err instanceof ApiError && err.code === 'bootstrap_complete'
      set({ pending: false, error: message(err), bootstrapRequired: required })
      return false
    }
  },

  changePassword: async (currentPassword, newPassword) => {
    set({ pending: true, error: null })
    try {
      const res = await api.changePassword(currentPassword, newPassword)
      set({ pending: false })
      // The backend e-mailed a warning to the account holder. Hand the delivery
      // back so the toast can say what actually happened instead of assuming.
      return { error: null, delivery: res?.delivery }
    } catch (err) {
      const reason = message(err)
      set({ pending: false, error: reason })
      return { error: reason, delivery: undefined }
    }
  },

  signOut: async () => {
    try {
      await api.logout()
    } catch {
      // Signing out locally must succeed even if the call fails.
    }
    set({ user: null, status: 'anon', error: null })
  },

  signOutOthers: async () => {
    try {
      const { ended } = await api.logoutOthers()
      return { ended }
    } catch (err) {
      // The server said the sessions were NOT ended; reporting success here
      // would tell someone a lost device had been evicted when it had not.
      return { error: message(err) }
    }
  },

  clearError: () => set({ error: null, needsVerification: false }),

  /**
   * Sends a NEW confirmation link (D-078).
   *
   * The server answers identically whether or not the address is registered, so
   * the caller is given its wording rather than a claim of success — telling
   * somebody "we sent it" when nothing left the building is the exact lie that
   * made this dead end hard to notice in the first place.
   */
  resendVerification: async (identifier) => {
    set({ pending: true, error: null })
    try {
      const res = await api.resendVerification(identifier.trim())
      set({ pending: false })
      return { error: null, note: res.note }
    } catch (err) {
      const reason = message(err)
      set({ pending: false, error: reason })
      return { error: reason }
    }
  },
}))
