import { create } from 'zustand'
import { api, ApiError, type ApiClient } from '../lib/api'

/**
 * Client accounts — backed by the real backend (`GET /api/clients`, P5).
 *
 * Before this existed the reseller screens had no way to name a real customer,
 * so `SaleFlowPage` and `AdminClientsPage` offered a hard-coded demo list and a
 * Premium assignment could not be aimed at anybody.
 *
 * The backend decides the scope, never the UI: a super admin receives every
 * client, a reseller only the clients who own a VPN they sold. A client role
 * gets 403, so this store is only mounted under admin routes.
 */

export interface ClientAccount {
  /** Local UI identity (string); the backend's numeric id lives in `apiId`. */
  id: string
  apiId: number
  email: string
  name: string
  status: string
  createdAt: number
  vpnCount: number
}

interface ClientsState {
  clients: ClientAccount[]
  loading: boolean
  error: string | null
  load: () => Promise<void>
}

function toRow(c: ApiClient): ClientAccount {
  return {
    id: String(c.id),
    apiId: c.id,
    email: c.email,
    name: c.display_name || c.email.split('@')[0] || c.email,
    status: c.status,
    createdAt: c.created_at,
    vpnCount: c.vpn_count,
  }
}

export const useClientsStore = create<ClientsState>()((set) => ({
  clients: [],
  loading: false,
  error: null,

  load: async () => {
    set({ loading: true, error: null })
    try {
      const rows = await api.listClients()
      set({ clients: rows.map(toRow), loading: false })
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Could not load clients'
      set({ loading: false, error: msg })
    }
  },
}))
