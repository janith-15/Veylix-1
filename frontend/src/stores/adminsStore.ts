import { create } from 'zustand'
import { api, ApiError, type ApiUser } from '../lib/api'

/**
 * Reseller (admin) accounts — backed by the real backend
 * (`GET/POST /api/admins`, `PATCH /api/admins/{id}`, Stage E2/E3).
 *
 * Phase 5: this store used to return four seeded, invented resellers. Every row
 * below now comes from the server.
 *
 * What is deliberately NOT here:
 *  - `servers`: there is no server-assignment endpoint yet (P5 step 3), so the
 *    UI must not pretend a reseller has servers. Exposing an empty list would
 *    look like "this admin has no servers"; the honest state is "unknown".
 *  - `quotaUsed`: `GET /api/admins` returns the ceiling only. Usage comes from
 *    `GET /api/quotas/me` for the caller themselves; a super admin seeing
 *    anyone's usage needs an endpoint that does not exist yet.
 *  - `removeAdmin`: removal is `SUSPENDED` by design (the audit trail and any
 *    VPNs they sold keep a valid owner), so there is no delete.
 */

export type AdminStatus = 'ACTIVE' | 'SUSPENDED' | 'PENDING_VERIFICATION'

export interface AdminAccount {
  /** Local UI identity (string); the backend's numeric id lives in `apiId`. */
  id: string
  apiId: number
  name: string
  email: string
  status: AdminStatus
  /** Ceiling only; `0` means unlimited. Usage is not exposed per-admin yet. */
  quotaMax: number
  createdAt: number
}

interface AdminsState {
  admins: AdminAccount[]
  loading: boolean
  error: string | null
  /** Admin id whose update is in flight. */
  busy: number | null

  load: () => Promise<void>
  addAdmin: (input: { email: string; password: string; name: string; quotaMax: number }) => Promise<AdminAccount>
  setStatus: (apiId: number, status: AdminStatus) => Promise<void>
  setQuota: (apiId: number, quotaMax: number) => Promise<void>
  /** Which servers a reseller may operate on (P5). */
  adminServers: (apiId: number) => Promise<number[]>
  /** Replaces the whole set — idempotent, matches the backend's PUT. */
  setAdminServers: (apiId: number, serverIds: number[]) => Promise<number[]>
}

function toRow(u: ApiUser): AdminAccount {
  return {
    id: String(u.id),
    apiId: u.id,
    name: u.display_name || u.email.split('@')[0] || u.email,
    email: u.email,
    status: (u.status as AdminStatus) ?? 'ACTIVE',
    quotaMax: u.quota_limit ?? 0,
    createdAt: u.created_at ?? 0,
  }
}

function message(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return 'Unexpected error'
}

export const useAdminsStore = create<AdminsState>()((set) => ({
  admins: [],
  loading: false,
  error: null,
  busy: null,

  load: async () => {
    set({ loading: true, error: null })
    try {
      const rows = await api.listAdmins('admin')
      set({ admins: rows.map(toRow), loading: false })
    } catch (err) {
      set({ loading: false, error: message(err) })
    }
  },

  addAdmin: async ({ email, password, name, quotaMax }) => {
    // The backend applies the shared password policy, so a weak password is
    // rejected here for the same reason it would be rejected at register.
    const created = await api.createAdmin({ email, password, displayName: name, quotaLimit: quotaMax })
    const row = toRow(created)
    set((s) => ({ admins: [...s.admins, row] }))
    return row
  },

  setStatus: async (apiId, status) => {
    set({ busy: apiId })
    try {
      const updated = await api.updateAdmin(apiId, {
        status: status === 'SUSPENDED' ? 'SUSPENDED' : 'ACTIVE',
      })
      const row = toRow(updated)
      set((s) => ({
        busy: null,
        admins: s.admins.map((a) => (a.apiId === apiId ? { ...a, ...row } : a)),
      }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  setQuota: async (apiId, quotaMax) => {
    set({ busy: apiId })
    try {
      const updated = await api.updateAdmin(apiId, { quotaLimit: quotaMax })
      const row = toRow(updated)
      set((s) => ({
        busy: null,
        admins: s.admins.map((a) => (a.apiId === apiId ? { ...a, ...row } : a)),
      }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  adminServers: async (apiId) => {
    const res = await api.adminServers(apiId)
    return res.server_ids ?? []
  },

  setAdminServers: async (apiId, serverIds) => {
    set({ busy: apiId })
    try {
      const res = await api.setAdminServers(apiId, serverIds)
      set({ busy: null })
      return res.server_ids ?? []
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },
}))

