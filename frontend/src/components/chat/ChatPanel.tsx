/**
 * Conversation panel.
 *
 * The one job of this screen is to answer "who said that?" without the reader
 * having to study a colour. So:
 *   · every bubble carries its author — `You` on yours, the name on theirs, and
 *     the name repeats when a run of messages continues (not only on the first
 *     bubble of five);
 *   · my bubbles sit right AND carry an accent edge, so the side survives
 *     greyscale, colour-blindness and a theme with a low-tint primary;
 *   · the hover toolbar spells its actions out in words instead of six grey
 *     glyphs, and the "delete for everyone" control says what the server really
 *     does (nothing here can delete the peer's copy — Bible §2.6);
 *   · a refused send keeps the words on screen, marked failed, and offers retry.
 */

import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { Button } from '../ui'
import {
  ArrowDown,
  ArchiveRestore,
  Check,
  ChevronDown,
  Copy,
  Lock,
  Pencil,
  Pin,
  PinOff,
  Save,
  Search,
  Send,
  Star,
  Trash2,
  TriangleAlert,
  Users,
  X,
} from 'lucide-react'
import {
  canEditMessage,
  dayLabel,
  messageStamp,
  useChatStore,
  visibleMessages,
  type ChatMessage,
  type ChatThread,
} from '../../stores/chatStore'
import { isMine, threadInitial } from './chat-format'

/** The server accepts up to 64 KiB; the composer matches the contract, not a guess. */
const MAX_LEN = 65_536
/** Consecutive messages from the same author within this window share a bubble group. */
const GROUP_WINDOW = 5 * 60_000

type Row =
  | { kind: 'day'; key: string; label: string }
  | {
      kind: 'msg'
      key: string
      m: ChatMessage
      mine: boolean
      /** Start / end of a same-author run: drives the grouping corners. */
      first: boolean
      last: boolean
      /**
       * The author label belongs on THIS bubble. Always true at the start of a
       * run and whenever the author changes — including the very first message
       * of the conversation, which the old rule dropped because it had no
       * predecessor to differ from.
       */
      showName: boolean
    }

/** Two messages belong to one run: same side, same day, within the window. */
const sameRun = (a: ChatMessage, b: ChatMessage, viewerEmail: string) =>
  isMine(a, viewerEmail) === isMine(b, viewerEmail) &&
  dayLabel(a.createdAt) === dayLabel(b.createdAt) &&
  b.createdAt - a.createdAt < GROUP_WINDOW &&
  b.createdAt >= a.createdAt

const buildRows = (messages: ChatMessage[], viewerEmail: string): Row[] => {
  const rows: Row[] = []
  let lastDay = ''
  messages.forEach((m, i) => {
    const day = dayLabel(m.createdAt)
    const prev = messages[i - 1]
    const next = messages[i + 1]
    if (day !== lastDay) {
      rows.push({ kind: 'day', key: `day-${day}-${i}`, label: day })
      lastDay = day
    }
    const mine = isMine(m, viewerEmail)
    const sameAsPrev = !!prev && sameRun(prev, m, viewerEmail)
    const sameAsNext = !!next && sameRun(m, next, viewerEmail)
    rows.push({
      kind: 'msg',
      key: m.id,
      m,
      mine,
      first: !sameAsPrev,
      last: !sameAsNext,
      /*
       * The author is stated on BOTH sides, at the start of every run.
       *
       * This is the fix for "sent vs received looks the same": the old rule was
       * `showName: !mine`, so a conversation that happened to be all one person's
       * messages (or any run of five from the same author) rendered bubbles with
       * no attribution at all — and two of them in a row were indistinguishable
       * from one message with two paragraphs.
       */
      showName: !sameAsPrev,
    })
  })
  return rows
}

/** Count of matches for the in-conversation search (case-insensitive). */
const countMatches = (messages: ChatMessage[], q: string) => {
  const needle = q.trim().toLowerCase()
  if (!needle) return 0
  return messages.filter((m) => m.text.toLowerCase().includes(needle)).length
}

/** Splits text around the query so the match can be marked, without innerHTML. */
function Marked({ text, query }: { text: string; query: string }) {
  const q = query.trim()
  if (!q) return <>{text}</>
  const lower = text.toLowerCase()
  const needle = q.toLowerCase()
  const parts: { text: string; hit: boolean }[] = []
  let from = 0
  for (let at = lower.indexOf(needle); at !== -1; at = lower.indexOf(needle, from)) {
    if (at > from) parts.push({ text: text.slice(from, at), hit: false })
    parts.push({ text: text.slice(at, at + needle.length), hit: true })
    from = at + needle.length
  }
  if (from < text.length) parts.push({ text: text.slice(from), hit: false })
  return (
    <>
      {parts.map((p, i) => (p.hit ? <mark key={i} className="chat-msg__hit">{p.text}</mark> : <span key={i}>{p.text}</span>))}
    </>
  )
}

export function ChatPanel({
  thread,
  viewerEmail,
  viewerRole,
  onBack,
}: {
  thread: ChatThread
  viewerEmail: string
  /** The signed-in role, from the session (never guessed from the email). */
  viewerRole: 'super-admin' | 'admin' | 'client'
  /** Mobile-only: return to the thread list. */
  onBack?: () => void
}) {
  const send = useChatStore((s) => s.send)
  const markRead = useChatStore((s) => s.markRead)
  const editMessage = useChatStore((s) => s.editMessage)
  const deleteMessage = useChatStore((s) => s.deleteMessage)
  const setPinned = useChatStore((s) => s.setPinned)
  const toggleSaved = useChatStore((s) => s.toggleSaved)
  const toggleImportant = useChatStore((s) => s.toggleImportant)
  const loadOlder = useChatStore((s) => s.loadOlder)
  const discardFailed = useChatStore((s) => s.discardFailed)
  const restoreMessage = useChatStore((s) => s.restoreMessage)
  const loadHidden = useChatStore((s) => s.loadHidden)
  const loading = useChatStore((s) => !!s.loadingThreads[thread.id])
  const flagging = useChatStore((s) => s.flagging)
  const storeError = useChatStore((s) => s.error)

  const [text, setText] = useState('')
  const [editing, setEditing] = useState<{ id: string; draft: string } | null>(null)
  const [confirmAll, setConfirmAll] = useState<string | null>(null)
  const [actionError, setActionError] = useState('')
  const [copied, setCopied] = useState('')
  /** Why a copy failed, shown in place of "Copied" (http://, denied permission). */
  const [copyError, setCopyError] = useState('')
  /** Echo ids currently being re-sent after a failure. */
  const [retrying, setRetrying] = useState<Record<string, boolean>>({})
  /**
   * Messages this viewer just deleted, newest first, offered back as Undo.
   *
   * Delete is a one-click control sitting next to Copy, and a hide is only a
   * visibility flag — so a mistake must cost nothing. The strip stays until the
   * reader leaves the conversation (the older ones remain in "Hidden messages").
   */
  const [undoStack, setUndoStack] = useState<{ id: string; text: string }[]>([])
  /** Older deletions, loaded on demand from the server's hidden list. */
  const [showHidden, setShowHidden] = useState(false)
  const [query, setQuery] = useState('')
  const [searchOpen, setSearchOpen] = useState(false)
  const [pinned, setPinnedView] = useState(true)
  /* The thread the reader is currently pinned to. Because the panel is keyed
     by thread id it remounts on switch, so deriving from state (instead of
     resetting inside an effect) keeps the "pinned" intent predictable. */
  const [pinnedThread, setPinnedThread] = useState(thread.id)
  if (pinnedThread !== thread.id) {
    setPinnedThread(thread.id)
    if (!pinned) setPinnedView(true)
    // A search belongs to the conversation it was typed in.
    if (query) setQuery('')
  }
  const listRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)

  /* Soft-deleted messages never arrive: the server hides them per viewer. */
  const messages = useMemo(() => visibleMessages(thread), [thread])
  const rows = useMemo(() => buildRows(messages, viewerEmail), [messages, viewerEmail])
  /* The pin is conversation state: fall back to the preview carried by the list
     when the message itself is older than the fetched page. */
  const pinnedMsg =
    messages.find((m) => m.id === thread.pinnedMessageId) ??
    (thread.pinnedPreview?.id === thread.pinnedMessageId ? thread.pinnedPreview : null)
  const locked = !!thread.locked
  const matches = countMatches(messages, query)
  /* Pin is a staff control and the server refuses it for a client (403), so a
     client is never shown a button that can only fail. */
  const canPin = viewerRole !== 'client'
  /* What the hide control really does is per-viewer; only a Super Admin gets the
     bulk variant, and it is labelled with its real meaning, never "delete for
     everyone" (Bible §2.6 — the peer's copy is evidence). */
  const mayHideForEveryone = viewerRole === 'super-admin'
  const canLoadOlder = thread.hasMessages && messages.length > 0 && messages.length < thread.totalMessages

  /* Auto-grow the composer up to ~5 lines. */
  useLayoutEffect(() => {
    const el = inputRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 120)}px`
  }, [text])

  /* Only follow new messages while the reader is pinned to the bottom. */
  useEffect(() => {
    if (!pinned) return
    const el = listRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [rows.length, pinned, thread.id])

  /* Opening / switching a conversation starts at the newest message. */
  useEffect(() => {
    requestAnimationFrame(() => {
      const el = listRef.current
      if (el) el.scrollTop = el.scrollHeight
    })
  }, [thread.id])

  /* Reading a thread clears its unread marker (server-side). */
  useEffect(() => {
    void markRead(thread.id)
  }, [markRead, thread.id, thread.messages.length])

  /* Ctrl/⌘+F opens the in-conversation search instead of the browser's. */
  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'f') {
        e.preventDefault()
        setSearchOpen(true)
        requestAnimationFrame(() => searchRef.current?.focus())
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const onScroll = () => {
    const el = listRef.current
    if (!el) return
    setPinnedView(el.scrollHeight - el.scrollTop - el.clientHeight < 48)
  }

  /** Brings one message into view and flashes it, so the eye finds it. */
  const jumpTo = (id: string) => {
    const el = listRef.current?.querySelector<HTMLElement>(`[data-msg-id="${CSS.escape(id)}"]`)
    if (!el) return
    el.scrollIntoView({ block: 'center', behavior: 'smooth' })
    el.classList.add('chat-msg--flash')
    window.setTimeout(() => el.classList.remove('chat-msg--flash'), 1200)
  }

  const submit = () => {
    const trimmed = text.trim()
    if (!trimmed || locked) return
    setPinnedView(true)
    setActionError('')
    // Clear at once: the store draws a local echo, so the message is visibly out
    // while the POST is in flight. If the server refuses, the words come back —
    // a failed send must never eat what the user typed.
    setText('')
    void send(thread.id, trimmed).then((ok) => {
      if (ok) return
      setText((current) => (current ? current : trimmed))
      setActionError(useChatStore.getState().error ?? 'The message could not be sent.')
      inputRef.current?.focus()
    })
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    // Enter sends, Shift+Enter adds a line (never send mid-IME composition).
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      submit()
    }
    if (e.key === 'Escape' && query) {
      setQuery('')
      setSearchOpen(false)
    }
  }

  const startEdit = (m: ChatMessage) => {
    setActionError('')
    setEditing({ id: m.id, draft: m.text })
  }

  const commitEdit = () => {
    if (!editing) return
    void (async () => {
      const ok = await editMessage(thread.id, editing.id, editing.draft)
      if (ok) {
        setActionError('')
        setEditing(null)
      } else {
        setActionError('That message can no longer be edited (only your own, within 15 minutes).')
      }
    })()
  }

  const onEditKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Escape') setEditing(null)
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      commitEdit()
    }
  }

  const doDelete = (m: ChatMessage) => {
    // A pending/failed echo is local only — dropping it removes nothing else.
    if (m.pending || m.failed) {
      discardFailed(thread.id, m.id)
      return
    }
    // Keep the words so the reader can put them back: a hide is only a
    // per-viewer visibility flag, so undoing costs nothing and removes the
    // "I deleted it and now it is gone forever" trap entirely.
    setUndoStack((s) => [{ id: m.id, text: m.text }, ...s.filter((u) => u.id !== m.id)].slice(0, 5))
    void deleteMessage(thread.id, m.id).then((ok) => {
      // If the server refused, the message is still in the transcript — drop the
      // undo entry so the strip never offers to restore something still there.
      if (!ok) setUndoStack((s) => s.filter((u) => u.id !== m.id))
    })
    if (editing?.id === m.id) setEditing(null)
  }

  /** Puts a deleted message back (undo strip and the hidden-messages list). */
  const undoDelete = (messageId: string) => {
    void restoreMessage(thread.id, messageId).then((ok) => {
      if (ok) setUndoStack((s) => s.filter((u) => u.id !== messageId))
    })
  }

  const doCopy = (m: ChatMessage) => {
    const done = () => {
      setCopyError('')
      setCopied(m.id)
      window.setTimeout(() => setCopied((c) => (c === m.id ? '' : c)), 1500)
    }
    const failed = () => {
      // `navigator.clipboard` is undefined on a non-secure origin (plain http://
      // to a LAN address), and the permission can be denied. The old code called
      // `navigator.clipboard?.writeText()` and said nothing either way, so the
      // button appeared dead.
      setCopied('')
      setCopyError(m.id)
      window.setTimeout(() => setCopyError((c) => (c === m.id ? '' : c)), 2500)
    }
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(m.text).then(done, failed)
      return
    }
    // Legacy path: a hidden textarea + execCommand still works over plain http.
    try {
      const ta = document.createElement('textarea')
      ta.value = m.text
      ta.setAttribute('readonly', '')
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      const ok = document.execCommand('copy')
      document.body.removeChild(ta)
      if (ok) done()
      else failed()
    } catch {
      failed()
    }
  }

  const remaining = MAX_LEN - text.length
  /* Most urgent message wins: a locked conversation explains why the input is
     disabled, so it must never sit BELOW a pin notice or a stale error. */
  const banner = locked
    ? `This conversation is locked — new messages are disabled.${thread.lockedBy ? ` (locked by ${thread.lockedBy})` : ''}`
    : actionError || storeError || ''

  return (
    <div className="chat-panel glass" data-testid="chat-panel" data-locked={locked ? 'true' : 'false'}>
      <div className="chat-panel__head">
        <div className="chat-panel__head-main">
          {onBack && (
            <button type="button" className="chat-panel__back" onClick={onBack} aria-label="Back to conversations">
              <ArrowDown size={16} style={{ transform: 'rotate(90deg)' }} />
            </button>
          )}
          <span className="chat-avatar" aria-hidden="true">
            {threadInitial(thread.title)}
          </span>
          <div className="chat-panel__title">
            <strong>{thread.title}</strong>
            <span className="ui-muted chat-panel__meta">
              {locked ? `locked by ${thread.lockedBy ?? 'staff'}` : 'internal · server-enforced'}
            </span>
          </div>
        </div>
        <span className="chat-panel__head-meta">
          {locked && (
            <span className="chat-pill chat-pill--lock" title={`Locked by ${thread.lockedBy ?? 'staff'}`}>
              <Lock size={11} aria-hidden="true" /> Locked
            </span>
          )}
          <button
            type="button"
            className={`chat-icon-btn has-label${showHidden ? ' is-accent' : ''}${
              (thread.hiddenMessages?.length ?? 0) > 0 ? ' has-count' : ''
            }`}
            onClick={() => {
              const next = !showHidden
              setShowHidden(next)
              if (next) void loadHidden(thread.id)
            }}
            aria-expanded={showHidden}
            aria-label="Deleted messages you can restore"
            title="Messages you deleted — you can restore them"
          >
            <ArchiveRestore size={12} aria-hidden="true" />
            <span>Deleted</span>
            {(thread.hiddenMessages?.length ?? 0) > 0 && (
              <span className="chat-icon-btn__count">{thread.hiddenMessages?.length}</span>
            )}
          </button>
          <button
            type="button"
            className={`chat-icon-btn has-label${searchOpen ? ' is-accent' : ''}`}
            onClick={() => {
              setSearchOpen((open) => !open)
              if (!searchOpen) requestAnimationFrame(() => searchRef.current?.focus())
              else setQuery('')
            }}
            aria-pressed={searchOpen}
            title="Search in this conversation (Ctrl+F)"
          >
            <Search size={12} aria-hidden="true" />
            <span>Find</span>
          </button>
          {/* "Of what the server holds", not "of what I happened to fetch". */}
          <span
            className="chat-panel__count"
            title={`${messages.length} loaded of ${thread.totalMessages} in this conversation`}
          >
            {thread.totalMessages > messages.length
              ? `${messages.length}/${thread.totalMessages}`
              : messages.length}{' '}
            msg
          </span>
        </span>
      </div>

      {searchOpen && (
        <div className="chat-find" role="search">
          <Search size={13} aria-hidden="true" />
          <input
            ref={searchRef}
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                setQuery('')
                setSearchOpen(false)
              }
            }}
            placeholder="Find in conversation…"
            aria-label="Find in conversation"
          />
          <span className="chat-find__count" aria-live="polite">
            {query.trim() ? `${matches} match${matches === 1 ? '' : 'es'}` : ''}
          </span>
          <button
            type="button"
            className="chat-icon-btn"
            onClick={() => {
              setQuery('')
              setSearchOpen(false)
            }}
            aria-label="Close search"
            title="Close search"
          >
            <X size={12} />
          </button>
        </div>
      )}

      {banner && (
        <div
          className={`chat-banner${locked ? ' chat-banner--locked' : ' chat-banner--error'}`}
          role={locked ? 'status' : 'alert'}
        >
          {locked ? <Lock size={13} aria-hidden="true" /> : <TriangleAlert size={13} aria-hidden="true" />}
          <span>{banner}</span>
          {!locked && (
            <button
              type="button"
              className="chat-icon-btn"
              onClick={() => {
                setActionError('')
                useChatStore.setState({ error: null })
              }}
              aria-label="Dismiss"
              title="Dismiss"
            >
              <X size={12} />
            </button>
          )}
        </div>
      )}

      {pinnedMsg && (
        <button
          type="button"
          className="chat-pinned"
          onClick={() => jumpTo(pinnedMsg.id)}
          title="Jump to the pinned message"
        >
          <Pin size={13} aria-hidden="true" />
          <span className="chat-pinned__label">Pinned</span>
          <span className="chat-pinned__text">{pinnedMsg.text}</span>
          {canPin && (
            <span
              className="chat-pinned__unpin"
              role="button"
              tabIndex={0}
              onClick={(e) => {
                e.stopPropagation()
                void setPinned(thread.id, null)
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.stopPropagation()
                  e.preventDefault()
                  void setPinned(thread.id, null)
                }
              }}
              aria-label="Unpin message"
              title="Unpin"
            >
              <PinOff size={13} />
            </span>
          )}
        </button>
      )}
      {/* Undo for a delete. Delete is a one-click control beside Copy, so the
          wrong-click case has to be free: the strip sits above the transcript
          with the actual words, and a restore puts the message straight back. */}
      {undoStack.length > 0 && (
        <div className="chat-undo" role="status" aria-label="Recently deleted">
          <Trash2 size={13} aria-hidden="true" />
          <span className="chat-undo__label">
            Deleted {undoStack.length === 1 ? '1 message' : `${undoStack.length} messages`} from your view
          </span>
          <span className="chat-undo__text">
            {undoStack[0].text.slice(0, 40)}
            {undoStack[0].text.length > 40 ? '…' : ''}
          </span>
          <button type="button" className="chat-link-btn chat-undo__btn" onClick={() => undoDelete(undoStack[0].id)}>
            Undo
          </button>
          {undoStack.length > 1 && <span className="chat-undo__more">+{undoStack.length - 1} more</span>}
          <button
            type="button"
            className="chat-icon-btn"
            onClick={() => setUndoStack([])}
            aria-label="Dismiss undo"
            title="Dismiss"
          >
            <X size={12} />
          </button>
        </div>
      )}

      {/* Older deletions live behind the header's menu (below) instead of a
          permanent bar: a full-width row reserved for a rare action was costing
          every conversation a slice of its transcript. */}
      {showHidden && (
        <div className="chat-hidden-list" aria-label="Messages you deleted">
          {(thread.hiddenMessages ?? []).length === 0 ? (
            <p className="ui-muted chat-hidden-list__empty">Nothing deleted in this conversation.</p>
          ) : (
            (thread.hiddenMessages ?? []).map((h) => (
              <div key={h.id} className="chat-hidden-list__row">
                <span className="chat-hidden-list__meta">
                  {isMine(h, viewerEmail) ? 'You' : h.fromName} · {messageStamp(h.createdAt)}
                </span>
                <span className="chat-hidden-list__text">{h.text}</span>
                <button type="button" className="chat-link-btn" onClick={() => undoDelete(h.id)}>
                  Restore
                </button>
              </div>
            ))
          )}
        </div>
      )}



      <div
        className="chat-panel__msg-list"
        ref={listRef}
        onScroll={onScroll}
        role="log"
        aria-live="polite"
        aria-relevant="additions"
        aria-label={`Conversation: ${thread.title}`}
        tabIndex={0}
      >
        {canLoadOlder && (
          <div className="chat-load-older">
            <Button size="sm" variant="ghost" onClick={() => void loadOlder(thread.id)} disabled={loading}>
              <ChevronDown size={13} style={{ transform: 'rotate(180deg)' }} />
              {loading ? 'Loading…' : `Load earlier messages (${thread.totalMessages - messages.length} older)`}
            </Button>
          </div>
        )}

        {rows.length === 0 && <p className="chat-panel__empty">No messages yet — say hi 👋</p>}
        {rows.map((row) =>
          row.kind === 'day' ? (
            <div key={row.key} className="chat-day" role="separator">
              <span>{row.label}</span>
            </div>
          ) : (
            <div
              key={row.key}
              id={`msg-${row.m.id}`}
              data-msg-id={row.m.id}
              className={`chat-msg${row.mine ? ' chat-msg--mine' : ' chat-msg--theirs'}${
                row.first ? ' chat-msg--first' : ''
              }${row.last ? ' chat-msg--last' : ''}${row.m.important ? ' chat-msg--important' : ''}${
                thread.pinnedMessageId === row.m.id ? ' chat-msg--pinned' : ''
              }${row.m.pending ? ' chat-msg--pending' : ''}${row.m.failed ? ' chat-msg--failed' : ''}`}
            >
              <div className="chat-msg__body">
                {/* The author is named on BOTH sides — `You` for mine, the
                    person's name for theirs — at the start of every run. Without
                    this, an outgoing message is only distinguishable from an
                    incoming one by a colour tint. */}
                {row.showName && (
                  <span className={`chat-msg__name${row.mine ? ' chat-msg__name--mine' : ''}`}>
                    {row.mine ? 'You' : row.m.fromName}
                  </span>
                )}
                {editing?.id === row.m.id ? (
                  <div className="chat-msg__editor">
                    <textarea
                      data-autofocus
                      value={editing.draft}
                      maxLength={MAX_LEN}
                      onChange={(e) => setEditing({ ...editing, draft: e.target.value })}
                      onKeyDown={onEditKeyDown}
                      aria-label="Edit message"
                      rows={2}
                    />
                    <div className="chat-msg__editor-actions">
                      <button type="button" className="chat-icon-btn has-label" onClick={() => setEditing(null)}>
                        Cancel
                      </button>
                      <button
                        type="button"
                        className="chat-icon-btn has-label is-accent"
                        onClick={commitEdit}
                        disabled={!editing.draft.trim()}
                      >
                        <Check size={12} aria-hidden="true" /> Save
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="chat-bubble">
                    <Marked text={row.m.text} query={query} />
                    <span className="chat-bubble__meta">
                      {/* Direction is carried by the author label at the start of
                          each run, the bubble's side and its accent edge. A glyph
                          on all 24 bubbles was noise, so the meaning is kept for
                          assistive tech only. */}
                      <span className="sr-only">{row.mine ? 'Sent by you' : 'Received'}</span>
                      {row.m.editedAt && <em className="chat-msg__edited">edited</em>}
                      {row.m.pending ? (
                        <em className="chat-msg__state">sending…</em>
                      ) : row.m.failed ? (
                        <em className="chat-msg__state is-failed">not sent</em>
                      ) : (
                        <time dateTime={new Date(row.m.createdAt).toISOString()}>
                          {messageStamp(row.m.createdAt)}
                        </time>
                      )}
                    </span>
                  </div>
                )}

                {row.m.failed && (
                  <div className="chat-msg__failed" role="alert">
                    <TriangleAlert size={12} aria-hidden="true" />
                    <span>Not delivered.</span>
                    {/* Retry re-sends THIS message in place. The old control
                        moved the text back to the composer, which allowed the
                        same words to be sent twice if the server had in fact
                        stored the first attempt (a timeout looks like a failure).
                        The echo is dropped only once the send is confirmed. */}
                    <button
                      type="button"
                      className="chat-link-btn"
                      disabled={!!retrying[row.m.id]}
                      onClick={() => {
                        const text = row.m.text
                        setRetrying((r) => ({ ...r, [row.m.id]: true }))
                        setActionError('')
                        discardFailed(thread.id, row.m.id)
                        void send(thread.id, text).then((ok) => {
                          setRetrying((r) => ({ ...r, [row.m.id]: false }))
                          if (!ok) setActionError(useChatStore.getState().error ?? 'Still not delivered.')
                        })
                      }}
                    >
                      {retrying[row.m.id] ? 'Sending…' : 'Retry'}
                    </button>
                    <button
                      type="button"
                      className="chat-link-btn"
                      onClick={() => {
                        // Copy-and-discard, for when the user would rather reword.
                        setText(row.m.text)
                        discardFailed(thread.id, row.m.id)
                        inputRef.current?.focus()
                      }}
                    >
                      Edit instead
                    </button>
                  </div>
                )}

                {/* Worded controls, not six identical glyphs: the pointer is not
                    the only way to read them, and two trash cans no longer look
                    the same. */}
                {!row.m.pending && !row.m.failed && editing?.id !== row.m.id && (
                  <div className="chat-msg__actions has-labels" role="toolbar" aria-label="Message actions">
                    <button
                      type="button"
                      className={`chat-icon-btn has-label${copyError === row.m.id ? ' is-danger' : ''}`}
                      onClick={() => doCopy(row.m)}
                      title={copyError === row.m.id ? 'The browser blocked the clipboard' : 'Copy text'}
                      aria-label="Copy message"
                    >
                      {copyError === row.m.id ? (
                        <TriangleAlert size={12} />
                      ) : copied === row.m.id ? (
                        <Check size={12} />
                      ) : (
                        <Copy size={12} />
                      )}
                      <span>
                        {copyError === row.m.id ? 'Blocked' : copied === row.m.id ? 'Copied' : 'Copy'}
                      </span>
                    </button>
                    {row.mine && canEditMessage(row.m, viewerEmail) && (
                      <button
                        type="button"
                        className="chat-icon-btn has-label"
                        onClick={() => startEdit(row.m)}
                        title="Edit (your own message, within 15 minutes)"
                        aria-label="Edit message"
                      >
                        <Pencil size={12} />
                        <span>Edit</span>
                      </button>
                    )}
                    <button
                      type="button"
                      className={`chat-icon-btn has-label${row.m.saved ? ' is-accent' : ''}`}
                      onClick={() => void toggleSaved(thread.id, row.m.id, !row.m.saved)}
                      aria-pressed={!!row.m.saved}
                      disabled={!!flagging[row.m.id]}
                      title={row.m.saved ? 'Remove your bookmark' : 'Bookmark for yourself'}
                      aria-label="Bookmark message"
                    >
                      <Save size={12} />
                      <span>{row.m.saved ? 'Saved' : 'Save'}</span>
                    </button>
                    <button
                      type="button"
                      className={`chat-icon-btn has-label${row.m.important ? ' is-accent' : ''}`}
                      onClick={() => void toggleImportant(thread.id, row.m.id)}
                      aria-pressed={!!row.m.important}
                      disabled={!!flagging[row.m.id]}
                      title={row.m.important ? 'Unmark important' : 'Mark important for yourself'}
                      aria-label="Mark important"
                    >
                      <Star size={12} />
                      <span>Important</span>
                    </button>
                    {canPin && (
                      <button
                        type="button"
                        className={`chat-icon-btn has-label${thread.pinnedMessageId === row.m.id ? ' is-accent' : ''}`}
                        onClick={() =>
                          void setPinned(thread.id, thread.pinnedMessageId === row.m.id ? null : row.m.id)
                        }
                        title={
                          thread.pinnedMessageId === row.m.id
                            ? 'Unpin'
                            : 'Pin to the top of the conversation (staff only)'
                        }
                        aria-label={thread.pinnedMessageId === row.m.id ? 'Unpin message' : 'Pin message'}
                        aria-pressed={thread.pinnedMessageId === row.m.id}
                      >
                        <Pin size={12} />
                        <span>{thread.pinnedMessageId === row.m.id ? 'Pinned' : 'Pin'}</span>
                      </button>
                    )}
                    <button
                      type="button"
                      className="chat-icon-btn has-label is-danger"
                      onClick={() => doDelete(row.m)}
                      title="Delete for me — the other person keeps their copy"
                      aria-label="Delete for me"
                    >
                      <Trash2 size={12} />
                      <span>Delete</span>
                    </button>
                    {mayHideForEveryone && (
                      <button
                        type="button"
                        className="chat-icon-btn has-label is-danger"
                        onClick={() => setConfirmAll(row.m.id)}
                        title="Staff-only: hide from every participant (audited)"
                        aria-label="Delete for everyone"
                      >
                        <Users size={12} />
                        <span>everyone</span>
                      </button>
                    )}
                  </div>
                )}
              </div>
            </div>
          ),
        )}
      </div>


      {confirmAll && (
        <div className="chat-confirm" role="alertdialog" aria-label="Confirm staff hide">
          <span>
            Hide this message from <strong>both</strong> participants? The original stays in the audit trail and cannot
            be restored from here.
          </span>
          <div className="chat-confirm__actions">
            <button type="button" className="chat-icon-btn" onClick={() => setConfirmAll(null)}>
              Cancel
            </button>
            <Button
              size="sm"
              variant="danger"
              onClick={() => {
                void deleteMessage(thread.id, confirmAll)
                setConfirmAll(null)
              }}
            >
              Hide for everyone
            </Button>
          </div>
        </div>
      )}

      {!pinned && (
        <button type="button" className="chat-jump" onClick={() => setPinnedView(true)}>
          <ArrowDown size={14} /> Jump to latest
        </button>
      )}

      <form
        className="chat-panel__input"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <textarea
          ref={inputRef}
          value={text}
          maxLength={MAX_LEN}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
          disabled={locked}
          placeholder={
            locked
              ? 'Conversation locked — messaging is disabled'
              : 'Write a message…  (Enter to send, Shift+Enter for a new line)'
          }
          aria-label="Message"
          rows={1}
        />
        <div className="chat-panel__send">
          {/* Reserved width: a counter appearing must not nudge the Send button. */}
          <span className={`chat-panel__counter${remaining < 200 ? ' is-low' : ''}`}>
            {remaining < 200 ? remaining : ''}
          </span>
          <Button type="submit" size="sm" leading={<Send size={14} />} disabled={locked || !text.trim()}>
            Send
          </Button>
        </div>
      </form>
    </div>
  )
}
