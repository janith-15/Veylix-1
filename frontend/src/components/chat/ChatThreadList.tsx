import { useMemo, useState } from 'react'
import {
  newestMessage,
  relativeTime,
  unreadVisible,
  useChatStore,
  visibleMessages,
  type ChatThread,
} from '../../stores/chatStore'
import { previewText, rowSignals, threadInitial } from './chat-format'
import { Search, Lock, Pin, Save, Star, MoreVertical, Trash2 } from 'lucide-react'
import { Dropdown, DropdownItem, DropdownSeparator } from '../ui'

export type ChatFilter = 'all' | 'unread' | 'important' | 'saved' | 'locked'

const FILTERS: { key: ChatFilter; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'unread', label: 'Unread' },
  { key: 'important', label: 'Important' },
  { key: 'saved', label: 'Saved' },
  { key: 'locked', label: 'Locked' },
]

/** A thread matches a quick filter given what THIS viewer can see. */
const matchesFilter = (t: ChatThread, filter: ChatFilter) => {
  switch (filter) {
    case 'all':
      return true
    case 'unread':
      return unreadVisible(t) > 0
    case 'important':
      // An unopened thread has no transcript in memory, so claiming "nothing
      // important here" would be a guess. Matches are kept to what we hold, and
      // the empty state says so.
      return t.messages.some((m) => m.important)
    case 'saved':
      // Bookmarks are per-viewer on the server, so a saved flag always refers
      // to THIS viewer — there is no map to look up any more.
      return t.messages.some((m) => m.saved)
    case 'locked':
      return !!t.locked
  }
}


export function ChatThreadList({
  threads,
  activeId,
  filter,
  onFilter,
  onSelect,
  canLockThreads,
  canDeleteAll,
  viewerEmail,
}: {
  threads: ChatThread[]
  activeId: string | null
  filter: ChatFilter
  onFilter: (f: ChatFilter) => void
  onSelect: (id: string) => void
  /** Admin / Super Admin — gets the lock control. */
  canLockThreads: boolean
  /** Super Admin — gets the delete-for-everyone shortcut. */
  canDeleteAll: boolean
  /** The signed-in account, so a preview can say "You:" instead of guessing. */
  viewerEmail: string
}) {
  const [query, setQuery] = useState('')
  const setLocked = useChatStore((s) => s.setLocked)
  const setPinned = useChatStore((s) => s.setPinned)

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return threads.filter((t) => {
      if (!matchesFilter(t, filter)) return false
      if (!q) return true
      const inMessages = visibleMessages(t).some((m) => m.text.toLowerCase().includes(q))
      // The server's newest message counts too: searching before opening a
      // thread must still find what it last said.
      const inPreview = !!t.lastMessage && newestMessage(t)!.text.toLowerCase().includes(q)
      return t.title.toLowerCase().includes(q) || inMessages || inPreview
    })
  }, [threads, filter, query])

  return (
    <nav className="chat-list glass" aria-label="Conversations">
      <div className="chat-list__head">
        <strong>Conversations</strong>
        <span className="ui-muted">{filtered.length}</span>
      </div>

      <div className="chat-list__tools">
        <label className="chat-search chat-list__search">
          <Search size={13} aria-hidden="true" />
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search conversations…"
            aria-label="Search conversations"
          />
        </label>
        <div className="chat-filters" role="tablist" aria-label="Filter conversations">
          {FILTERS.map((f) => (
            <button
              key={f.key}
              type="button"
              role="tab"
              aria-selected={filter === f.key}
              className={`chat-filter${filter === f.key ? ' is-active' : ''}`}
              onClick={() => onFilter(f.key)}
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>

      <ul className="chat-list__items">
        {filtered.length === 0 && (
          <li>
            <p className="ui-muted chat-list__empty">
              {query.trim()
                ? 'No conversations match that search.'
                : filter === 'important' || filter === 'saved'
                  ? `Nothing ${filter === 'important' ? 'marked important' : 'bookmarked'} in the conversations opened on this device. Open one to check it here.`
                  : 'Nothing in this filter.'}
            </p>
          </li>
        )}
        {filtered.map((t) => {
          const signals = rowSignals(t, viewerEmail)
          const last = signals.last
          const unread = unreadVisible(t)
          const active = t.id === activeId
          return (
            <li key={t.id} className="chat-list__row-wrap">
              <button
                type="button"
                className={`chat-list__item${active ? ' chat-list__item--active' : ''}${unread ? ' chat-list__item--unread' : ''}${
                  signals.mine ? ' chat-list__item--mine' : ''
                }`}
                onClick={() => onSelect(t.id)}
                aria-current={active ? 'true' : undefined}
              >
                <span className="chat-avatar" aria-hidden="true">
                  {threadInitial(t.title)}
                </span>
                <span className="chat-list__text">
                  <span className="chat-list__row">
                    <strong className="chat-list__title">{t.title}</strong>
                    <time className="chat-list__time">{relativeTime(last?.createdAt ?? 0)}</time>
                  </span>
                  <span className="chat-list__row">
                    <span className="chat-list__preview">{previewText(t, viewerEmail)}</span>
                    {unread > 0 && (
                      <span className="chat-list__badge" aria-label={`${unread} unread`}>
                        {unread > 9 ? '9+' : unread}
                      </span>
                    )}
                  </span>
                </span>
              </button>

              {/* Sibling (not nested) so it stays valid HTML + independently focusable */}
              <span className="chat-list__flags" aria-hidden="true">
                {signals.pinned && (
                  <span className="chat-list__flag" title="Has a pinned message">
                    <Pin size={10} />
                  </span>
                )}
                {signals.important && (
                  <span className="chat-list__flag is-important" title="Has important messages">
                    <Star size={10} />
                  </span>
                )}
                {signals.saved > 0 && (
                  <span className="chat-list__flag is-saved" title={`${signals.saved} saved`}>
                    <Save size={10} />
                  </span>
                )}
                {t.locked && (
                  <span className="chat-list__flag is-locked" title="Locked">
                    <Lock size={10} />
                  </span>
                )}
              </span>

              <span className="chat-list__menu">
                <Dropdown
                  ariaLabel={`Actions for ${t.title}`}
                  trigger={<MoreVertical size={14} aria-hidden="true" />}
                >
                  {canLockThreads && (
                    <DropdownItem onClick={() => void setLocked(t.id, !t.locked)}>
                      {t.locked ? 'Unlock conversation' : 'Lock conversation'}
                    </DropdownItem>
                  )}
                  <DropdownItem
                    onClick={() => {
                      if (last && !last.pending) setPinned(t.id, last.id)
                      onSelect(t.id)
                    }}
                  >
                    <Pin size={12} aria-hidden="true" /> Pin latest message
                  </DropdownItem>
                  {canDeleteAll && (
                    <>
                      <DropdownSeparator />
                      <DropdownItem onClick={() => onSelect(t.id)}>
                        <Trash2 size={12} aria-hidden="true" /> Open to delete messages
                      </DropdownItem>
                    </>
                  )}
                </Dropdown>
              </span>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
