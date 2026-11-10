import { create } from 'zustand'
import { api, ApiError, type ApiPanelClient } from '../lib/api'

/**
 * One server's panel clients (L7, D-037).
 *
 * The list lives in a store rather than in page state because the codebase's
 * convention is that every screen reads from an API-backed store, and because
 * a fetch that calls `setState` from inside a component effect is a
 * render-time side effect the React Compiler flags.
 *
 * A failed read is surfaced as an outage, never as an empty list: "the panel
 * could not be read" and "this panel holds no clients" are different facts and
 * a VPN platform must not confuse them (D-045).
 */

export interface PanelClientQuery {
  tag?: string
  assigned?: 'yes' | 'no'
  usage?: 'empty' | 'low' | 'high'
  q?: string
}

function message(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 503) {
      return 'The panel could not be read. This is an outage, not an empty server — the list below is not proof that nothing exists.'
    }
    return err.message
  }
  return 'Could not read the panel.'
}

interface ServerDetailState {
  rows: ApiPanelClient[]
  loading: boolean
  error: string | null
  /** The server currently loaded, so a stale switch cannot mix two panels. */
  serverId: number | null

  load: (serverId: number, query?: PanelClientQuery) => Promise<void>
  reset: () => void

  /** Counts for the summary cards, derived from the real rows. */
  summary: () => { total: number; free: number; premium: number; unowned: number }
}

export const useServerDetailStore = create<ServerDetailState>()((set, get) => ({
  rows: [],
  loading: false,
  error: null,
  serverId: null,

  load: async (serverId, query = {}) => {
    set({ loading: true, error: null, serverId })
    try {
      const rows = await api.panelClients(serverId, query)
      // Drop the answer if the operator moved to another server meanwhile: a
      // late response must never paint one panel's clients onto another's page.
      if (get().serverId !== serverId) return
      set({ rows, loading: false })
    } catch (err) {
      if (get().serverId !== serverId) return
      set({ rows: [], loading: false, error: message(err) })
    }
  },

  reset: () => set({ rows: [], loading: false, error: null, serverId: null }),

  summary: () => {
    const { rows } = get()
    return {
      total: rows.length,
      free: rows.filter((r) => r.tag === 'FREE').length,
      premium: rows.filter((r) => r.tag === 'PREMIUM').length,
      unowned: rows.filter((r) => !r.assigned).length,
    }
  },
}))
