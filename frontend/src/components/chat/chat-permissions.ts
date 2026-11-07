import type { Person, PersonRole } from '../../stores/peopleStore'

/**
 * Who may an account START a new conversation with?
 *
 * Bible §2.6 / REQUIREMENTS §17: a Client must not read a conversation with
 * another Client or an unrelated Admin, and chat access is per participant.
 * So the picker is SCOPED — never a free-text "chat with anyone" box.
 *
 * Phase 5: this used to *derive* the role from the email string
 * (`e.startsWith('client')`), which was a guess even in the mock. The role now
 * comes from the signed-in account, and the backend re-checks every rule when
 * the thread is created — so this function only narrows a list the server
 * already scoped (`GET /api/directory`).
 */
export function allowedChatPartners(input: {
  viewerRole: PersonRole | null
  viewerEmail: string
  people: Person[]
}): Person[] {
  const { viewerRole, viewerEmail, people } = input
  const norm = (e: string) => e.toLowerCase()
  const withoutSelf = people.filter((p) => norm(p.email) !== norm(viewerEmail))

  // No role (not signed in, or the session is still loading): offer nothing
  // rather than guess and let the user pick somebody the server will refuse.
  if (!viewerRole) return []

  if (viewerRole === 'client') {
    // The directory already contains only this client's sellers + super admins.
    return withoutSelf.filter((p) => p.role === 'admin' || p.role === 'super-admin')
  }

  if (viewerRole === 'admin') {
    // The directory already contains only this reseller's clients + super admins.
    return withoutSelf.filter((p) => p.role === 'client' || p.role === 'super-admin')
  }

  // Super Admin: the whole (server-supplied) directory is addressable.
  return withoutSelf
}
