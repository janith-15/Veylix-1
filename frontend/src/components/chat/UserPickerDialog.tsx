import { useMemo, useState } from 'react'
import { Dialog } from '../ui'
import { MessageSquarePlus, Search, ShieldCheck, Users } from 'lucide-react'
import { allowedChatPartners } from './chat-permissions'
import { threadInitial } from './chat-format'
import type { Person, PersonRole } from '../../stores/peopleStore'

const ROLE_LABEL: Record<Person['role'], string> = {
  'super-admin': 'Super Admin',
  admin: 'Admin',
  client: 'Client',
}

/**
 * Scoped user picker — only lists people this account is allowed to chat with
 * (see chat-permissions.ts). People you already talk to show a shortcut to the
 * existing thread instead of starting a duplicate.
 */
export function UserPickerDialog({
  open,
  onClose,
  viewerRole,
  viewerEmail,
  people,
  existingPartnerEmails,
  onPick,
}: {
  open: boolean
  onClose: () => void
  viewerRole: PersonRole | null
  viewerEmail: string
  people: Person[]
  /** Emails the viewer already has a thread with → "Open" instead of "Start". */
  existingPartnerEmails: string[]
  onPick: (person: Person, hasThread: boolean) => void
}) {
  const [query, setQuery] = useState('')

  const allowed = useMemo(
    () => allowedChatPartners({ viewerRole, viewerEmail, people }).sort((a, b) => a.name.localeCompare(b.name)),
    [viewerRole, viewerEmail, people],
  )

  const filtered = allowed.filter((p) => {
    const q = query.trim().toLowerCase()
    return !q || p.name.toLowerCase().includes(q) || p.email.toLowerCase().includes(q)
  })

  const groups: { key: string; label: string; icon: typeof Users; items: Person[] }[] = [
    {
      key: 'super-admin',
      label: 'Super Admin',
      icon: ShieldCheck,
      items: filtered.filter((p) => p.role === 'super-admin'),
    },
    { key: 'admin', label: 'Admins', icon: ShieldCheck, items: filtered.filter((p) => p.role === 'admin') },
    { key: 'client', label: 'Clients', icon: Users, items: filtered.filter((p) => p.role === 'client') },
  ].filter((g) => g.items.length > 0)

  return (
    <Dialog open={open} onClose={onClose} title="New conversation">
      <p className="ui-muted chat-picker__hint">
        You can only start a chat with people connected to your account.
      </p>
      <label className="chat-picker__search">
        <Search size={14} aria-hidden="true" />
        <input
          data-autofocus
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search by name or email…"
          aria-label="Search people"
        />
      </label>

      {groups.length === 0 ? (
        <p className="ui-muted chat-picker__empty">
          No one available to chat with. Sellers appear here after a Premium assignment.
        </p>
      ) : (
        groups.map((g) => (
          <section key={g.key} className="chat-picker__group">
            <h3 className="chat-picker__group-title">
              <g.icon size={13} aria-hidden="true" /> {g.label}
            </h3>
            <ul className="chat-picker__items">
              {g.items.map((p) => {
                const hasThread = existingPartnerEmails.includes(p.email)
                return (
                  <li key={p.email}>
                    <button type="button" className="chat-picker__item" onClick={() => onPick(p, hasThread)}>
                      <span className="chat-avatar" aria-hidden="true">
                        {threadInitial(p.name)}
                      </span>
                      <span className="chat-picker__text">
                        <strong>{p.name}</strong>
                        <span className="ui-muted">{p.email}</span>
                      </span>
                      <span className={`chat-picker__action${hasThread ? ' is-open' : ''}`}>
                        {hasThread ? (
                          <>
                            <MessageSquarePlus size={13} aria-hidden="true" /> Open
                          </>
                        ) : (
                          'Start'
                        )}
                      </span>
                      <span className="sr-only">{ROLE_LABEL[p.role]}</span>
                    </button>
                  </li>
                )
              })}
            </ul>
          </section>
        ))
      )}
    </Dialog>
  )
}
