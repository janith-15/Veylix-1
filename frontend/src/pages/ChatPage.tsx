import { useEffect, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Button, EmptyState, PageHeader } from '../components/ui'
import { useAuthStore } from '../stores/authStore'
import { personOf, usePeopleStore, type Person } from '../stores/peopleStore'
import { threadsFor, useChatStore } from '../stores/chatStore'
import { ChatPanel } from '../components/chat/ChatPanel'
import { ChatThreadList, type ChatFilter } from '../components/chat/ChatThreadList'
import { UserPickerDialog } from '../components/chat/UserPickerDialog'
import { Plus, MessageSquare } from 'lucide-react'
import './chat.css'

export function ChatPage() {
  const user = useAuthStore((s) => s.user)
  // Only trust the role once the session is known: defaulting to 'client' while
  // hydrating made staff controls flicker on and off on every reload.
  const authStatus = useAuthStore((s) => s.status)
  const role = user?.role ?? 'client'
  const email = user?.email ?? ''
  const threads = useChatStore((s) => s.threads)
  const loadThreads = useChatStore((s) => s.load)
  const loadThread = useChatStore((s) => s.loadThread)
  const refreshChat = useChatStore((s) => s.refresh)
  const setViewerEmail = useChatStore((s) => s.setViewerEmail)
  const ensureThreadWith = useChatStore((s) => s.ensureThreadWith)
  const people = usePeopleStore((s) => s.people)
  const loadPeople = usePeopleStore((s) => s.load)
  const { threadId } = useParams()
  const [params] = useSearchParams()
  const withSeller = params.get('with')
  const navigate = useNavigate()

  const [filter, setFilter] = useState<ChatFilter>('all')
  const [pickerOpen, setPickerOpen] = useState(false)
  /* Why a `?with=` deep link could not open — shown, never swallowed. */
  const [linkError, setLinkError] = useState('')

  // The picker's candidates come from the server-scoped directory, so it can
  // only ever offer accounts this caller is allowed to address.
  useEffect(() => {
    void loadPeople()
  }, [loadPeople])

  /* Staff controls follow the signed-in role, which comes from the session —
     not from an email pattern — and not from a guess made mid-hydration. */
  const authed = authStatus === 'authed'
  const canLockThreads = authed && role !== 'client'
  const canDeleteAll = authed && role === 'super-admin'

  const myThreads = threadsFor(threads)
  const existingPartners = myThreads.flatMap((t) => t.participants.filter((p) => p !== email))

  const active = myThreads.find((t) => t.id === threadId) ?? myThreads[0] ?? null
  const activeId = active?.id ?? null

  /* The thread list is the account's own inbox. */
  useEffect(() => {
    void loadThreads()
  }, [loadThreads])

  /* Open the active conversation's messages from the server. */
  useEffect(() => {
    if (activeId) void loadThread(activeId)
  }, [activeId, loadThread])

  /* The store needs the viewer's email for the local echo ("You"), so a send
     renders correctly before the server answers. */
  useEffect(() => {
    if (email) setViewerEmail(email)
  }, [email, setViewerEmail])

  /* Freshness. The backend has no push channel, so the open conversation and the
     inbox are polled while the tab is visible. Paused when hidden (no point
     spending a request per tab in the background) and skipped while a send is in
     flight, which the store already gates. */
  useEffect(() => {
    if (!authed) return
    let cancelled = false
    const tick = () => {
      if (cancelled || document.visibilityState !== 'visible') return
      void refreshChat(activeId ?? undefined)
    }
    const timer = window.setInterval(tick, 5000)
    const onVisible = () => {
      if (document.visibilityState === 'visible') tick()
    }
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      cancelled = true
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [activeId, authed, refreshChat])

  /* "Message seller" deep link: /chat?with=<email> opens (or creates) the
     conversation with that person. The server decides whether the peer is
     reachable; if not, the store reports the refusal instead of inventing one.
     An unreachable peer used to `return` silently, which made the link look
     broken — the refusal is now shown. The "not in the directory" case is
     DERIVED during render (below), because the directory is server-scoped: a
     missing entry means "not reachable by you", never "wrong link". */
  const unreachablePeer =
    withSeller && withSeller !== email && people.length > 0 && !personOf(people, withSeller)
      ? withSeller
      : null
  useEffect(() => {
    if (!withSeller || withSeller === email) return
    const person = personOf(people, withSeller)
    if (!person) return
    let cancelled = false
    void (async () => {
      const id = await ensureThreadWith({ peerUserId: person.id })
      if (cancelled) return
      if (id) {
        setLinkError('')
        navigate(`/chat/${id}`, { replace: true })
      } else {
        setLinkError(useChatStore.getState().error ?? `Could not open a conversation with ${withSeller}.`)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [email, ensureThreadWith, navigate, people, withSeller])

  /* Keep the URL in sync so the conversation is shareable/back-navigable. */
  useEffect(() => {
    if (activeId && activeId !== threadId) navigate(`/chat/${activeId}`, { replace: true })
  }, [activeId, navigate, threadId])

  const startWith = (person: Person) => {
    void (async () => {
      const id = await ensureThreadWith({ peerUserId: person.id })
      if (!id) return
      setPickerOpen(false)
      navigate(`/chat/${id}`)
    })()
  }

  const isEmpty = myThreads.length === 0

  return (
    <div className="dash" data-testid="chat-page">
      <PageHeader
        title="Internal chat"
        subtitle="You can only see conversations you are part of"
        actions={
          <Button size="sm" leading={<Plus size={14} />} onClick={() => setPickerOpen(true)}>
            New chat
          </Button>
        }
      />
      {(linkError || unreachablePeer) && (
        <p className="chat-link-error" role="alert" data-testid="chat-link-error">
          {linkError || `You cannot start a conversation with ${unreachablePeer}.`}
        </p>
      )}
      {isEmpty ? (
        <EmptyState
          icon={<MessageSquare size={22} />}
          title="No conversations yet"
          description="Chat opens with your seller after a premium assignment. Use “New chat” to reach someone connected to your account."
          action={
            <Button size="sm" onClick={() => setPickerOpen(true)}>
              Start a conversation
            </Button>
          }
        />
      ) : (
        <div className="chat-layout" data-view={active ? 'thread' : 'list'}>
          <ChatThreadList
            threads={myThreads}
            activeId={activeId}
            filter={filter}
            onFilter={setFilter}
            onSelect={(id) => navigate(`/chat/${id}`)}
            canLockThreads={canLockThreads}
            canDeleteAll={canDeleteAll}
            viewerEmail={email}
          />
          {active && (
            <ChatPanel
              key={active.id}
              thread={active}
              viewerEmail={email}
              viewerRole={role}
              onBack={() => navigate('/chat')}
            />
          )}
        </div>
      )}

      <UserPickerDialog
        open={pickerOpen}
        onClose={() => setPickerOpen(false)}
        viewerRole={role}
        viewerEmail={email}
        people={people}
        existingPartnerEmails={Array.from(new Set(existingPartners))}
        onPick={(person) => startWith(person)}
      />
    </div>
  )
}
