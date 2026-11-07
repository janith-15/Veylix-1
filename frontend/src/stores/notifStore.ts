import { create } from 'zustand'
import { api, ApiError, type ApiNotification } from '../lib/api'

/**
 * Notifications — backed by the real backend (`/api/notifications`, P8).
 *
 * Phase 5: this store used to hold three invented entries ("Panel Tokyo-03
 * rejected credentials", "311 clients reconciled") so the bell always looked
 * busy. That is the worst kind of placeholder on an operations screen: it reads
 * like a record of something that happened.
 *
 * Every row now comes from the caller's own inbox, which the backend scopes by
 * `user_id` — there is no "load somebody else's notifications" call, on either
 * side of the wire. An empty inbox renders as empty.
 */
export interface Notification {
  /** Local UI identity (string); the backend's numeric id lives in `apiId`. */
  id: string
  apiId: number
  kind: 'info' | 'success' | 'warning' | 'error'
  title: string
  body?: string
  /** Unix seconds — formatted by the caller, never a fake "12m ago". */
  at: number
  read: boolean
}

interface NotifState {
  items: Notification[]
  loading: boolean
  error: string | null
  load: () => Promise<void>
  markRead: (id: string) => Promise<void>
  markAllRead: () => Promise<void>
}

function toRow(n: ApiNotification): Notification {
  return {
    id: String(n.id),
    apiId: n.id,
    kind: n.kind,
    title: n.title,
    body: n.body || undefined,
    at: n.created_at,
    read: n.read_at !== null,
  }
}

function message(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return 'Could not load notifications'
}

export const useNotifStore = create<NotifState>()((set, get) => ({
  items: [],
  loading: false,
  error: null,

  load: async () => {
    set({ loading: true, error: null })
    try {
      const rows = await api.listNotifications()
      set({ items: rows.map(toRow), loading: false })
    } catch (err) {
      set({ loading: false, error: message(err) })
    }
  },

  markRead: async (id) => {
    const row = get().items.find((n) => n.id === id)
    if (!row || row.read) return
    // Optimistic: the badge should clear on click. A failure reloads the
    // server's truth rather than leaving the UI displaying a lie.
    set((s) => ({ items: s.items.map((n) => (n.id === id ? { ...n, read: true } : n)) }))
    try {
      await api.markNotificationRead(row.apiId)
    } catch {
      await get().load()
    }
  },

  markAllRead: async () => {
    set((s) => ({ items: s.items.map((n) => ({ ...n, read: true })) }))
    try {
      await api.markAllNotificationsRead()
    } catch {
      await get().load()
    }
  },
}))
