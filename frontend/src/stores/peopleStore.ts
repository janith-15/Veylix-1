import { create } from 'zustand'
import { api, ApiError, type ApiDirectoryEntry } from '../lib/api'

/**
 * The account directory — backed by the real backend (`GET /api/directory`).
 *
 * Phase 5: this store used to hard-code eight invented people
 * (`kumara@resell.veylix`, …) as "the ONE list of accounts the UI can chat
 * with". A picker built on that list could never start a conversation with a
 * real customer, and could offer conversations with people who do not exist.
 *
 * The backend decides the scope (super admin: everyone; admin: own clients +
 * super admins; client: super admins + own sellers), so the UI must not filter
 * further — anything it adds would be a role rule the server does not enforce.
 */

export type PersonRole = 'super-admin' | 'admin' | 'client'

export interface Person {
  /** The backend's `users.id` — the only value `POST /api/chat/threads` accepts. */
  id: number
  email: string
  name: string
  role: PersonRole
}

interface PeopleState {
  people: Person[]
  loading: boolean
  error: string | null
  load: () => Promise<void>
}

/** The backend's snake_case role → the UI's hyphenated role. */
function toRole(role: string): PersonRole {
  if (role === 'super_admin') return 'super-admin'
  if (role === 'admin') return 'admin'
  return 'client'
}

function toRow(d: ApiDirectoryEntry): Person {
  return {
    id: d.id,
    email: d.email,
    name: d.display_name || d.email.split('@')[0] || d.email,
    role: toRole(d.role),
  }
}

export const usePeopleStore = create<PeopleState>()((set) => ({
  people: [],
  loading: false,
  error: null,

  load: async () => {
    set({ loading: true, error: null })
    try {
      const rows = await api.directory()
      set({ people: rows.map(toRow), loading: false })
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Could not load the directory'
      set({ loading: false, error: msg })
    }
  },
}))

export const personOf = (people: Person[], email: string) =>
  people.find((p) => p.email.toLowerCase() === email.toLowerCase()) ?? null

/** Display name for any email, falling back to a prettified local part. */
export const nameOf = (people: Person[], email: string) => {
  const found = personOf(people, email)
  if (found) return found.name
  const local = email.split('@')[0] ?? ''
  const cleaned = local.replace(/[-_.]+/g, ' ').trim()
  return cleaned.charAt(0).toUpperCase() + cleaned.slice(1) || email
}

