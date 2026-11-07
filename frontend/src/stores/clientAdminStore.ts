import { create } from 'zustand'
import {
  api,
  ApiError,
  type ApiAdminClientDetail,
  type ApiAdminClientRow,
  type ApiClientQuery,
} from '../lib/api'

/**
 * The Super Admin's client console (D-042).
 *
 * Two separate concerns live here because they share one cache:
 *   - the LIST (filters + paging), and
 *   - the DETAIL of one client, including the block/free-limit writes.
 *
 * The list is refreshed after a write only when the write can change what the list
 * shows (status), so blocking a client updates the row you are looking at without
 * a full reload.
 */
interface ClientAdminState {
  rows: ApiAdminClientRow[]
  total: number
  page: number
  limit: number
  loading: boolean
  error: string | null
  query: ApiClientQuery

  detail: ApiAdminClientDetail | null
  detailLoading: boolean
  saving: boolean
  detailError: string | null

  load: (query?: ApiClientQuery) => Promise<void>
  setQuery: (patch: Partial<ApiClientQuery>) => Promise<void>
  loadDetail: (id: number) => Promise<void>
  setStatus: (id: number, status: 'ACTIVE' | 'SUSPENDED') => Promise<string | null>
  setFreeLimit: (id: number, limit: number) => Promise<string | null>
  /** Super Admin only: issues a customer a new password (shown once). */
  resetPassword: (id: number) => Promise<{ error: string | null; password?: string }>
  /**
   * Super Admin only (D-077): starts an address change. Resolves to the delivery
   * channel and, when no relay is configured, the code to read out.
   */
  requestEmailChange: (
    id: number,
    newEmail: string,
  ) => Promise<
    | { error: string }
    | { error?: undefined; delivery: 'smtp' | 'outbox'; to: string; code?: string }
  >
  /** Super Admin only: finishes it with the code from the new mailbox. */
  confirmEmailChange: (id: number, code: string) => Promise<string | null>
  /** Super Admin only: marks the account DELETED (irreversible, history kept). */
  deleteClient: (id: number) => Promise<string | null>
  clearError: () => void
}

/** A user-safe message for an API failure. */
function reason(err: unknown): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case 'forbidden':
        return 'Only a Super Admin can manage clients.'
      case 'not_found':
        return 'That client no longer exists.'
      case 'invalid_status':
        return 'Status must be ACTIVE or SUSPENDED.'
      case 'invalid_free_limit':
        return 'The FREE limit must be between 0 (platform default) and 100.'
      default:
        return err.message
    }
  }
  return 'Something went wrong. Please try again.'
}

export const useClientAdminStore = create<ClientAdminState>()((set, get) => ({
  rows: [],
  total: 0,
  page: 1,
  limit: 25,
  loading: false,
  error: null,
  query: { page: 1, limit: 25 },

  detail: null,
  detailLoading: false,
  saving: false,
  detailError: null,

  load: async (query) => {
    const effective = { ...get().query, ...(query ?? {}) }
    set({ loading: true, error: null, query: effective })
    try {
      const res = await api.adminClients(effective)
      set({ rows: res.clients, total: res.total, page: res.page, limit: res.limit, loading: false })
    } catch (err) {
      set({ loading: false, error: reason(err) })
    }
  },

  // Changing a filter always returns to page 1: staying on page 4 of a
  // newly-narrowed result shows an empty page that looks like "no results".
  setQuery: async (patch) => {
    const next: ApiClientQuery = { ...get().query, ...patch }
    if (!('page' in patch)) next.page = 1
    await get().load(next)
  },

  loadDetail: async (id) => {
    set({ detailLoading: true, detailError: null })
    try {
      const detail = await api.adminClient(id)
      set({ detail, detailLoading: false })
    } catch (err) {
      set({ detailLoading: false, detailError: reason(err) })
    }
  },

  setStatus: async (id, status) => {
    set({ saving: true, detailError: null })
    try {
      const detail = await api.updateAdminClient(id, { status })
      set((s) => ({
        detail,
        saving: false,
        // Keep the list row in step, so going back does not show a stale badge.
        rows: s.rows.map((r) => (r.id === id ? { ...r, status: detail.profile.status } : r)),
      }))
      return null
    } catch (err) {
      const msg = reason(err)
      set({ saving: false, detailError: msg })
      return msg
    }
  },

  setFreeLimit: async (id, limit) => {
    set({ saving: true, detailError: null })
    try {
      const detail = await api.updateAdminClient(id, { free_vpn_limit: limit })
      set((s) => ({
        detail,
        saving: false,
        rows: s.rows.map((r) =>
          r.id === id
            ? { ...r, free_vpn_limit: detail.free_limit_override }
            : r,
        ),
      }))
      return null
    } catch (err) {
      const msg = reason(err)
      set({ saving: false, detailError: msg })
      return msg
    }
  },

  /**
   * Starts an e-mail change for a customer (D-077) and returns what the operator
   * needs to finish it: the delivery channel, and the code when no relay exists.
   *
   * The profile is NOT touched here — the address only moves once the code from
   * the new mailbox is confirmed, so this must not update `detail`.
   */
  requestEmailChange: async (id, newEmail) => {
    set({ saving: true, detailError: null })
    try {
      const res = await api.requestClientEmailChange(id, newEmail)
      set({ saving: false })
      return res
    } catch (err) {
      const msg = reason(err)
      set({ saving: false, detailError: msg })
      return { error: msg }
    }
  },

  /** Finishes the change; on success the detail is replaced with the new address. */
  confirmEmailChange: async (id, code) => {
    set({ saving: true, detailError: null })
    try {
      await api.confirmClientEmailChange(id, code)
      const detail = await api.adminClient(id)
      set((s) => ({
        detail,
        saving: false,
        // The list row shows the address, so it must not keep the old one.
        rows: s.rows.map((r) => (r.id === id ? { ...r, email: detail.profile.email } : r)),
      }))
      return null
    } catch (err) {
      const msg = reason(err)
      set({ saving: false, detailError: msg })
      return msg
    }
  },

  /**
   * Marks a customer's account deleted. Returns the error message instead of
   * throwing, like every other action here; a 409 means they still own VPNs.
   */
  deleteClient: async (id) => {
    set({ saving: true, detailError: null })
    try {
      await api.deleteClient(id)
      set({ saving: false })
      return null
    } catch (err) {
      const msg = reason(err)
      set({ saving: false, detailError: msg })
      return msg
    }
  },

  /**
   * Issues a new password for a customer and returns it once.
   *
   * Returns the error message instead of throwing, matching every other action in
   * this store; the caller decides how to show the generated password.
   */
  resetPassword: async (id) => {
    set({ saving: true, detailError: null })
    try {
      const res = await api.resetClientPassword(id)
      set({ saving: false })
      return { error: null, password: res.password }
    } catch (err) {
      const msg = reason(err)
      set({ saving: false, detailError: msg })
      return { error: msg, password: undefined }
    }
  },



  clearError: () => set({ error: null, detailError: null }),
}))
