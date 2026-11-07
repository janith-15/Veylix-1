import { useEffect } from 'react'
import { Bell, CheckCheck } from 'lucide-react'
import { AsyncState, Button, Card, EmptyState, PageHeader, Skeleton, useToast } from '../components/ui'
import { useNotifStore } from '../stores/notifStore'
import { relativeTime } from '../lib/time'

export function NotificationsPage() {
  const { items, load, loading, error, markAllRead, markRead } = useNotifStore()
  const toast = useToast()
  const unread = items.filter((n) => !n.read).length

  // Always reload on mount: the inbox is the account's own record, so another
  // device's read state must win over whatever this tab last saw.
  useEffect(() => {
    void load()
  }, [load])

  return (
    <div className="dash" data-testid="notifications-page">
      <PageHeader
        title="Notifications"
        subtitle="Server health, VPN events and account alerts from your own inbox"
        actions={
          <Button
            size="sm"
            variant="outline"
            leading={<CheckCheck size={14} />}
            disabled={unread === 0}
            onClick={() => {
              void markAllRead()
              toast('success', 'All caught up', `${unread} notifications marked as read`)
            }}
          >
            Mark all read {unread > 0 ? `(${unread})` : ''}
          </Button>
        }
      />

      <Card
        title="Inbox"
        actions={
          <span className="ui-muted t-sm">
            {items.length} total
          </span>
        }
      >
        <AsyncState
          status={
            error ? 'error' : loading && items.length === 0 ? 'loading' : items.length === 0 ? 'empty' : 'ready'
          }
          error={error}
          onRetry={() => void load()}
          skeleton={<Skeleton height={160} />}
          empty={
            <EmptyState
              icon={<Bell size={22} />}
              title="No notifications yet"
              description="Panel health, VPN events and payment decisions appear here as the backend records them."
            />
          }
        >
          <div className="notif-page-list">
            {items.map((n) => (
              <button
                key={n.id}
                type="button"
                className={`notif-modal__item ${n.read ? '' : 'notif-modal__item--unread'}`}
                onClick={() => void markRead(n.id)}
                style={{ borderRadius: 'var(--radius-sm)' }}
              >
                <span className={`notif-modal__dot notif-modal__dot--${n.kind}`} aria-hidden="true" />
                <span className="notif-modal__txt">
                  <strong>{n.title}</strong>
                  {n.body && <span>{n.body}</span>}
                  <time dateTime={new Date(n.at * 1000).toISOString()}>{relativeTime(n.at)}</time>
                </span>
                {!n.read && <Bell size={14} aria-hidden="true" style={{ color: 'var(--c-primary)' }} />}
              </button>
            ))}
          </div>
        </AsyncState>
      </Card>
    </div>
  )
}


