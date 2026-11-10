import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Bell, BellOff } from 'lucide-react'
import { Button, EmptyState } from '../../components/ui'
import { ClientPage, ClientSection } from '../../components/client/ClientPage'
import { useNotifStore } from '../../stores/notifStore'
import '../../components/client/client.css'
import './notifications.css'
import { activeTimeZone } from '../../lib/time'

/**
 * Notifications — client shell (S11 outer layer).
 *
 * SCOPE OF THIS FILE: the frame, plus the read/unread handling the existing
 * endpoints already support. The bell in the topbar stays the quick view; this
 * screen is the full inbox.
 *
 * WHY IT DOES NOT FILTER BY "TYPE"
 *
 *   The brief lists categories (New VPN assigned, expiry, usage, order updates,
 *   payment updates, admin messages). The backend stores a `kind` that is a
 *   SEVERITY — `info | success | warning | error` (`store/notifications.go:13-18`)
 *   — not a category. Filtering on `kind` would therefore group rows by colour,
 *   not by subject, and a category filter that silently returns the wrong rows
 *   is worse than no filter. Category support is an S11 backend decision
 *   (a second column and a producer change), so this shell does not pretend it
 *   exists.
 *
 * WHAT IT DOES DO
 *
 *   Renders the caller's own inbox with its real title, body and timestamp,
 *   supports per-row "Mark read" and "Mark all read", and states unread from a
 *   count over the rows the server returned. `GET /api/notifications` is scoped
 *   to the caller in the store, so there is no client-side ownership filter.
 */

/**
 * Relative time from a real unix timestamp.
 *
 * Deliberately coarse and honest: it never says "just now" for an old row, and
 * it falls back to a locale date once the entry is more than a month old, so a
 * stale notice cannot masquerade as fresh.
 */
function relativeTime(unixSeconds: number): string {
  const diff = Math.floor(Date.now() / 1000) - unixSeconds
  if (diff < 0) return new Date(unixSeconds * 1000).toLocaleDateString(undefined, { timeZone: activeTimeZone() })
  if (diff < 60) return 'just now'
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  if (diff < 604800) return `${Math.floor(diff / 86400)}d ago`
  return new Date(unixSeconds * 1000).toLocaleDateString(undefined, { timeZone: activeTimeZone() })
}

export function ClientNotificationsPage() {
  const navigate = useNavigate()
  const items = useNotifStore((s) => s.items)
  const loading = useNotifStore((s) => s.loading)
  const error = useNotifStore((s) => s.error)
  const load = useNotifStore((s) => s.load)
  const markAllRead = useNotifStore((s) => s.markAllRead)
  const markRead = useNotifStore((s) => s.markRead)

  useEffect(() => {
    void load()
  }, [load])

  /* Counted from the rows the server returned for this caller — not a second,
     separately-derived number that could disagree with the list below it. */
  const unread = items.filter((n) => !n.read).length

  return (
    <ClientPage
      testId="client-notifications"
      title="Notifications"
      description="VPN events, order updates and messages about your account"
      actions={
        unread > 0 ? (
          <Button size="sm" variant="outline" onClick={() => void markAllRead()}>
            Mark all read
          </Button>
        ) : undefined
      }
    >
      <ClientSection
        title="Inbox"
        actions={
          !loading && !error ? (
            <span className="ui-muted t-sm">
              {items.length} total{unread > 0 ? ` · ${unread} unread` : ''}
            </span>
          ) : undefined
        }
      >
        {error ? (
          /* A failed read is "unknown", never "empty" (D-090). */
          <EmptyState
            icon={<BellOff size={22} />}
            title="Could not load your notifications"
            description="This is a connection problem, not an empty inbox. Try again."
            action={
              <Button size="sm" variant="primary" onClick={() => void load()}>
                Try again
              </Button>
            }
          />
        ) : items.length === 0 && !loading ? (
          <EmptyState
            icon={<Bell size={22} />}
            title="Nothing to read yet"
            description="When a VPN is assigned, an order moves, or your seller replies, it arrives here."
            action={
              <Button size="sm" variant="ghost" onClick={() => navigate('/client')}>
                Back to dashboard
              </Button>
            }
          />
        ) : (
          <div className="ui-col ntf__stack">
            {items.map((n) => (
              <div key={n.id} className={`ntf__item${n.read ? '' : ' ntf__item--unread'}`}>
                <div className="ui-col ntf__body">
                  <span className="ui-strong">{n.title}</span>
                  {n.body && <span className="ui-muted t-sm2">{n.body}</span>}
                </div>
                <div className="ui-row ntf__meta">
                  <span className="ui-muted t-sm2">{relativeTime(n.at)}</span>
                  {!n.read && (
                    <Button size="sm" variant="ghost" onClick={() => void markRead(n.id)}>
                      Mark read
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </ClientSection>
    </ClientPage>
  )
}

