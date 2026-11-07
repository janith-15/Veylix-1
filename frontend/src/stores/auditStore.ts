import { create } from 'zustand'
import { api, ApiError, type ApiAuditEntry, type ApiAuditQuery } from '../lib/api'

/**
 * Audit trail — backed by the real backend (`GET /api/audit`, Stage E2; D-082
 * made its filters server-side).
 *
 * The trail is written **only** by the Go backend, inside the same operation it
 * describes, so this store is read-only by design. There is no `log()` method
 * and there must never be one (REQUIREMENTS §8: append-only, server-side).
 *
 * The PAGE owns the filters (URL-driven, D-051); this store is the fetch cache
 * for the window the page last asked for, and nothing more.
 */

export type AuditKind = 'auth' | 'server' | 'admin' | 'vpn' | 'payment' | 'client' | 'other'

export interface AuditEntry {
  id: string
  /** Unix seconds, as stored by the backend. */
  at: number
  actorId: number | null
  /** The actor's display name or e-mail; '' for system entries. */
  actorName: string
  action: string
  target: string
  result: string
  kind: AuditKind
  /** Non-secret context the backend chose to store, as pretty JSON. */
  context: string
}

/** The filters the page keeps in the URL and hands to `load`. */
export interface AuditFilters {
  /** '' = every kind; otherwise one of the backend's accepted tokens. */
  kind: string
  /** Substring across action, actor, target and context — server-side. */
  q: string
  /** Age window: '' = all time; otherwise the backend `from` is derived. */
  range: '' | '24h' | '7d' | '30d'
}

/** The window each range option covers, in seconds. */
export const AUDIT_RANGES: Record<Exclude<AuditFilters['range'], ''>, number> = {
  '24h': 24 * 60 * 60,
  '7d': 7 * 24 * 60 * 60,
  '30d': 30 * 24 * 60 * 60,
}

interface AuditState {
  items: AuditEntry[]
  /** Matches AFTER the filters but BEFORE the window — the honest count. */
  total: number
  offset: number
  hasMore: boolean
  loading: boolean
  error: string | null
  load: (filters: AuditFilters, offset?: number) => Promise<void>
}

/** Maps the backend's dotted action (`vpn.provision`) to a display category. */
function kindOf(action: string): AuditKind {
  if (action.startsWith('auth.')) return 'auth'
  if (action.startsWith('server.')) return 'server'
  if (action.startsWith('admin.')) return 'admin'
  if (action.startsWith('vpn.')) return 'vpn'
  // payment_method.* is a payments answer too — the dotted prefix is an
  // implementation detail the operator should not have to know.
  if (action.startsWith('payment.') || action.startsWith('payment_method.')) return 'payment'
  if (action.startsWith('user.')) return 'client'
  return 'other'
}

function toRow(e: ApiAuditEntry): AuditEntry {
  return {
    id: String(e.id),
    at: e.created_at,
    actorId: e.actor_id,
    actorName: e.actor_name ?? '',
    action: e.action,
    target: e.target_type ? `${e.target_type}:${e.target_id}` : e.target_id,
    result: e.result,
    kind: kindOf(e.action),
    context: e.context,
  }
}

export const useAuditStore = create<AuditState>()((set) => ({
  items: [],
  total: 0,
  offset: 0,
  hasMore: false,
  loading: false,
  error: null,

  load: async (filters, offset = 0) => {
    set({ loading: true, error: null })
    try {
      const query: ApiAuditQuery = {
        kind: filters.kind || undefined,
        q: filters.q.trim() || undefined,
        limit: 50,
        offset,
      }
      if (filters.range) {
        query.from = Math.floor(Date.now() / 1000) - AUDIT_RANGES[filters.range]
      }
      const page = await api.audit(query)
      set({
        items: page.entries.map(toRow),
        total: page.total,
        offset: page.offset,
        hasMore: page.has_more,
        loading: false,
      })
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Could not read the audit log'
      set({ loading: false, error: msg })
    }
  },
}))
