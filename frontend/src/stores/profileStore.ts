import { create } from 'zustand'
import {
  api,
  ApiError,
  type ApiEmailChangeRequest,
  type ApiProfile,
  type ApiProfilePatch,
} from '../lib/api'

/**
 * The signed-in account's own profile (D-041).
 *
 * Why a store and not local state in the page: the sidebar/topbar show the name
 * and avatar, and a change on the profile page must be visible there without a
 * reload. It mirrors the server — every field here came from an API response.
 *
 * `email` is read-only in this store. Changing an address is an identity change:
 * it goes through requestEmailChange → confirmEmailChange, and the server is the
 * only thing that can apply it.
 */
interface ProfileState {
  profile: ApiProfile | null
  loading: boolean
  saving: boolean
  error: string | null
  /** The pending e-mail change, while its code is being confirmed. */
  emailChange: ApiEmailChangeRequest | null

  load: () => Promise<void>
  /** Saves a patch. Resolves to null on success, or a user-safe message. */
  save: (patch: ApiProfilePatch) => Promise<string | null>
  /** Live "is this username free" check for the form. */
  usernameFree: (username: string) => Promise<boolean>
  requestEmailChange: (newEmail: string, currentPassword: string) => Promise<string | null>
  confirmEmailChange: (code: string) => Promise<string | null>
  cancelEmailChange: () => void
  clearError: () => void
}

/** A user-safe message for an API failure. */
function reason(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case 'username_taken':
        return 'That username is already in use.'
      case 'invalid_username':
        return 'A username is 3–32 characters: letters, digits, dot, hyphen or underscore.'
      case 'invalid_display_name':
        return 'Please enter a name (1–80 characters).'
      case 'invalid_age':
        return 'Age must be between 1 and 120.'
      case 'invalid_avatar':
        return 'The avatar must be an http(s) URL or an uploaded image.'
      case 'invalid_email':
        return 'That is not a valid e-mail address.'
      case 'same_email':
        return 'That is already your e-mail address.'
      case 'email_taken':
        return 'That address is already registered.'
      case 'password_mismatch':
        return 'Your current password is incorrect.'
      case 'invalid_code':
        return 'That code is not correct.'
      case 'code_expired':
        return 'That code has expired — request a new one.'
      case 'no_pending_change':
        return 'There is no pending e-mail change. Request a new code.'
      case 'invalid_bio':
        return 'The bio is limited to 500 characters.'
      default:
        return err.message
    }
  }
  return 'Something went wrong. Please try again.'
}

export const useProfileStore = create<ProfileState>()((set) => ({
  profile: null,
  loading: false,
  saving: false,
  error: null,
  emailChange: null,

  load: async () => {
    set({ loading: true, error: null })
    try {
      const profile = await api.profile()
      set({ profile, loading: false })
    } catch (err) {
      set({ loading: false, error: reason(err) })
    }
  },

  save: async (patch) => {
    set({ saving: true, error: null })
    try {
      const profile = await api.updateProfile(patch)
      set({ profile, saving: false })
      return null
    } catch (err) {
      const msg = reason(err)
      set({ saving: false, error: msg })
      return msg
    }
  },

  usernameFree: async (username) => {
    try {
      const res = await api.usernameAvailable(username)
      return res.available
    } catch {
      return false
    }
  },

  requestEmailChange: async (newEmail, currentPassword) => {
    set({ saving: true, error: null })
    try {
      const res = await api.requestEmailChange(newEmail, currentPassword)
      set({ emailChange: res, saving: false })
      return null
    } catch (err) {
      const msg = reason(err)
      set({ saving: false, error: msg })
      return msg
    }
  },

  confirmEmailChange: async (code) => {
    set({ saving: true, error: null })
    try {
      const res = await api.confirmEmailChange(code)
      set({ profile: res.profile, emailChange: null, saving: false })
      return null
    } catch (err) {
      const msg = reason(err)
      set({ saving: false, error: msg })
      return msg
    }
  },

  cancelEmailChange: () => set({ emailChange: null, error: null }),
  clearError: () => set({ error: null }),
}))

// Re-export so a page can read the state type without another import.
export type { ApiProfile }
