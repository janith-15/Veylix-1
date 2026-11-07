import { useNavigate } from 'react-router-dom'
import { MessageSquare, ShieldQuestion } from 'lucide-react'
import { Button, Card, EmptyState } from '../../components/ui'
import { ClientPage, ClientSection } from '../../components/client/ClientPage'
import '../../components/client/client.css'

/**
 * Support & Chat — client shell (S10 outer layer).
 *
 * SCOPE OF THIS FILE: the frame. The working chat itself is `/chat`, which has
 * a complete inbox/list/thread implementation and ten backend routes behind it
 * (`chat_thread_routes.go`, `chat_message_routes.go`). This screen does not
 * re-implement it — it explains what support is and hands over.
 *
 * WHAT IS DELIBERATELY ABSENT
 *
 *   The brief asks for WhatsApp, Telegram and Gmail contact links. There is NO
 *   backend endpoint, setting or column that stores such a contact for a
 *   seller — verified by searching the whole backend for `whatsapp`, `telegram`
 *   and `contact_`, which returns nothing. Rendering those buttons would mean
 *   either hardcoding a number that belongs to nobody, or shipping a link that
 *   goes nowhere. Both are false affordances, so neither exists here.
 *
 *   The in-product chat IS real and IS wired, so that is what this screen
 *   offers. External contact channels are an S10 decision (they need a stored,
 *   operator-editable contact) and are not invented on the client.
 *
 * SCOPE NOTE: who a client may converse with is decided by the SERVER
 * (`GET /api/directory` returns, for a client, super admins plus their own
 * sellers). This UI never builds or filters that list.
 */
export function ClientSupportPage() {
  const navigate = useNavigate()

  return (
    <ClientPage
      testId="client-support"
      title="Support & Chat"
      description="Talk to the seller who owns your account"
      actions={
        <Button size="sm" variant="primary" leading={<MessageSquare size={14} />} onClick={() => navigate('/chat')}>
          Open chat
        </Button>
      }
    >
      <ClientSection title="How to reach us">
        <Card>
          <div className="ui-col" style={{ gap: 10 }}>
            <span className="ui-row" style={{ gap: 8, alignItems: 'center' }}>
              <MessageSquare size={16} aria-hidden="true" style={{ color: 'var(--text-muted)' }} />
              <strong>In-product chat is the quickest route</strong>
            </span>
            <p className="ui-muted t-sm2" style={{ margin: 0 }}>
              Messages are tied to your account, so the person answering can already see which VPNs
              you own without you having to describe them. Their reply arrives in your notifications
              as well as in the conversation.
            </p>
            <p className="ui-muted t-sm2" style={{ margin: 0 }}>
              You can only start a conversation with someone connected to your account — your seller,
              or a platform administrator. A message history is never deleted.
            </p>
            <div className="ui-row" style={{ gap: 8 }}>
              <Button size="sm" variant="primary" onClick={() => navigate('/chat')}>
                Open chat
              </Button>
              <Button size="sm" variant="ghost" onClick={() => navigate('/notifications')}>
                Check notifications
              </Button>
            </div>
          </div>
        </Card>
      </ClientSection>

      <ClientSection title="Outside the app">
        <EmptyState
          icon={<ShieldQuestion size={22} />}
          title="No external contact channels are published"
          description="This build does not advertise WhatsApp, Telegram or e-mail support links, because no such contact is stored for your seller — showing one would point you at a number nobody monitors. Use the in-product chat: it reaches the same person and keeps the history attached to your account."
          action={
            <Button size="sm" variant="outline" onClick={() => navigate('/chat')}>
              Use in-product chat
            </Button>
          }
        />
      </ClientSection>
    </ClientPage>
  )
}
