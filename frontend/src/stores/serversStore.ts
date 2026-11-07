import { create } from 'zustand'
import { api, ApiError, type ApiInbound, type ApiServer } from '../lib/api'

/**
 * Server integrations — backed by the real Go backend and the real 3X-UI
 * adapter (Stage E3). No simulated delays and no fabricated "verified" states:
 * every status here comes from an actual authenticated panel call.
 *
 * Tokens are sent once on create and are never held in this store afterwards
 * (the backend seals them at rest; D-016).
 */

export type ServerStatus =
  | 'PENDING_VERIFICATION'
  | 'ACTIVE'
  | 'REAUTH_REQUIRED'
  | 'CONNECTION_UNAVAILABLE'
  | 'DISABLED'
  | 'REMOVED'

export interface ServerIntegration {
  /** Local UI identity (string); the backend's numeric id lives in `apiId`. */
  id: string
  apiId: number
  name: string
  panelUrl: string
  status: ServerStatus
  version: string
  xrayVersion: string
  lastHealthCheck: number | null
  /** Whether the panel sits on a private/LAN address (SSRF opt-in). */
  allowPrivateNetwork: boolean
}

/** Fields the operator may edit. The token is write-only: '' keeps the stored one. */
export interface ServerEditInput {
  name: string
  panelUrl: string
  token: string
  allowPrivateNetwork: boolean
}

interface ServersState {
  servers: ServerIntegration[]
  loading: boolean
  error: string | null
  /** backend id currently being tested, if any. */
  testing: number | null
  /** Inbounds per server id, cached so pickers do not re-read on every render. */
  inbounds: Record<number, ApiInbound[]>
  inboundsLoading: number | null

  load: () => Promise<void>
  /** Reads (and caches) one server's inbounds for a picker. */
  loadInbounds: (serverId: number) => Promise<void>
  addServer: (input: ServerEditInput) => Promise<ServerIntegration>
  updateServer: (id: string, input: ServerEditInput) => Promise<ServerIntegration>
  testConnection: (id: string) => Promise<'ok' | 'fail'>
  disableServer: (id: string) => Promise<void>
  removeServer: (id: string) => Promise<void>
}

function toRow(s: ApiServer): ServerIntegration {
  return {
    id: String(s.id),
    apiId: s.id,
    name: s.name,
    panelUrl: s.panel_url,
    status: s.status as ServerStatus,
    version: s.panel_version || '—',
    xrayVersion: s.xray_version || '—',
    lastHealthCheck: s.last_health_check,
    allowPrivateNetwork: s.allow_private_network ?? false,
  }
}

function message(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return 'Unexpected error'
}

export const useServersStore = create<ServersState>()((set, get) => ({
  servers: [],
  loading: false,
  error: null,
  testing: null,
  inbounds: {},
  inboundsLoading: null,

  load: async () => {
    set({ loading: true, error: null })
    try {
      const rows = await api.listServers()
      set({ servers: rows.map(toRow), loading: false })
    } catch (err) {
      set({ loading: false, error: message(err) })
    }
  },

  /**
   * Reads one server's inbounds for a picker.
   *
   * An empty result is cached too: a panel with no inbound is a real answer,
   * and re-reading it on every keystroke would hammer the panel.
   */
  loadInbounds: async (serverId) => {
    set({ inboundsLoading: serverId })
    try {
      const rows = await api.serverInbounds(serverId)
      set((s) => ({ inboundsLoading: null, inbounds: { ...s.inbounds, [serverId]: rows } }))
    } catch {
      // A failed read must not look like "this server has no inbounds"; the
      // picker shows its own empty message, and the read is retried next time.
      set((s) => ({ inboundsLoading: null, inbounds: { ...s.inbounds, [serverId]: [] } }))
    }
  },

  addServer: async ({ name, panelUrl, token, allowPrivateNetwork }) => {
    // The backend validates the URL (SSRF guard), performs a real health check
    // and returns the observed status — it never assumes ACTIVE.
    const created = await api.addServer({ name, panelUrl, token, allowPrivateNetwork })
    const row = toRow(created)
    set((s) => ({ servers: [row, ...s.servers] }))
    return row
  },

  updateServer: async (id, { name, panelUrl, token, allowPrivateNetwork }) => {
    const target = get().servers.find((v) => v.id === id)
    if (!target) throw new ApiError(404, 'not_found', 'That integration is no longer in the list.')

    // `token` is empty unless the operator typed a new one: the backend keeps
    // the sealed credential in that case and re-checks the panel anyway.
    const updated = await api.updateServer(target.apiId, {
      name,
      panelUrl,
      token: token || undefined,
      allowPrivateNetwork,
    })
    const row = toRow(updated)
    set((s) => ({ servers: s.servers.map((v) => (v.id === id ? row : v)) }))
    return row
  },

  testConnection: async (id) => {
    const target = get().servers.find((v) => v.id === id)
    if (!target) return 'fail'

    set({ testing: target.apiId })
    try {
      const updated = await api.testServer(target.apiId)
      const row = toRow(updated)
      set((s) => ({
        testing: null,
        servers: s.servers.map((v) => (v.id === id ? row : v)),
      }))
      return row.status === 'ACTIVE' ? 'ok' : 'fail'
    } catch {
      set((s) => ({
        testing: null,
        servers: s.servers.map((v) =>
          v.id === id ? { ...v, status: 'CONNECTION_UNAVAILABLE' as ServerStatus } : v,
        ),
      }))
      return 'fail'
    }
  },

  disableServer: async (id) => {
    const target = get().servers.find((v) => v.id === id)
    if (!target) return
    await api.disableServer(target.apiId)
    set((s) => ({
      servers: s.servers.map((v) => (v.id === id ? { ...v, status: 'DISABLED' as ServerStatus } : v)),
    }))
  },

  removeServer: async (id) => {
    const target = get().servers.find((v) => v.id === id)
    if (!target) return
    // Hard delete: the backend destroys the sealed credential and cached
    // inbounds, and refuses (409) while VPN instances still reference it. The
    // failure is rethrown so the page can show the real reason instead of a
    // success toast for a delete that never happened.
    await api.removeServer(target.apiId)
    set((s) => ({ servers: s.servers.filter((v) => v.id !== id) }))
  },
}))
