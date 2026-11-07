import { create } from 'zustand'
import { api, ApiError, type ApiChatMessage, type ApiChatThread } from '../lib/api'

/**
 * Internal support chat — backed by the real backend (`/api/chat/*`, P7).
 *
 * Phase 5: this was the LAST mock store. The whole transcript lived in
 * localStorage, so two devices disagreed, clearing the cache lost the
 * conversation, and "locked" / "delete for everyone" were decoration the server
 * never saw. A transcript is evidence, so it now lives on the server.
 *
 * What changed in the model, and why:
 *  - `from` is an **email the server supplied**, so "is this mine?" compares
 *    against the signed-in account rather than a guessed role prefix;
 *  - `removedBy` is gone: the backend hides a message per viewer, so a hidden
 *    message simply never arrives (the other person still has their copy);
 *  - `readBy` is gone: unread comes from the server's per-participant marker;
 *  - `pinnedMessageId`, `locked`, `saved`, `important` are server state now, so
 *    both participants finally get the same answer.
 *
 * The store keeps the shape the components already use (`text`, `createdAt`,
 * `time`), so the UI did not need rewriting — only its data source changed.
 */

export interface ChatMessage {
  id: string
  /** Author email — the identity used for "is this mine?". */
  from: string
  fromName: string
  text: string
  /** Epoch ms — used for ordering, day grouping and staleness. */
  createdAt: number
  /** Display clock time (HH:mm), derived from `createdAt`. */
  time: string
  /** Last edit (epoch ms) — renders the "edited" marker. */
  editedAt?: number
  /** Flagged important by THIS viewer (server-side, per viewer). */
  important?: boolean
  /** Bookmarked by THIS viewer (server-side, per viewer). */
  saved?: boolean
  /**
   * Only on a local echo: the send is in flight. The bubble is drawn at once so
   * the composer can be cleared, but it is replaced by the server's copy (with
   * the server's id) when the request answers.
   */
  pending?: boolean
  /** Only on a local echo: the server refused the send; the bubble offers retry. */
  failed?: boolean
}

export interface ChatThread {
  id: string
  title: string
  participants: string[]
  /** Messages fetched by THIS client (oldest → newest). */
  messages: ChatMessage[]
  /**
   * One pinned message per conversation (payment details, instructions…).
   * `pinnedPreview` carries its text even when `messages` has not been fetched
   * yet — the pin is conversation state, not a page of history.
   */
  pinnedMessageId?: string | null
  pinnedPreview?: ChatMessage | null
  /** Locked conversations accept no new messages (transcript stays readable). */
  locked?: boolean
  /** Who locked it — a staff account. */
  lockedBy?: string
  /** Server-computed unread count for the signed-in viewer. */
  unread: number
  /**
   * The newest message as the list endpoint reported it. This is what makes an
   * inbox row truthful before its transcript is opened: without it every row
   * read "No messages yet" until the user clicked it.
   */
  lastMessage?: ChatMessage | null
  /** Rows the server holds for this viewer, including the ones not fetched. */
  totalMessages: number
  /** True once `messages` starts at the newest page (the default after a load). */
  hasMessages: boolean
  /**
   * Messages THIS viewer deleted for himself, kept so Delete is undoable.
   *
   * The transcript read filters them out, so without this list the UI has no way
   * back: a mis-click next to Copy used to lose the message with no recovery
   * path at all. Loaded lazily, only when the reader asks to restore something.
   */
  hiddenMessages?: ChatMessage[]
}

/** Edit window for your own message (ms). Mirrors the server's 15-minute rule. */
export const EDIT_WINDOW_MS = 15 * 60_000

/** Page size for one transcript read. The API returns the OLDEST unread page. */
export const PAGE_SIZE = 100

interface ChatState {
  threads: ChatThread[]
  /**
   * The signed-in account's email. The store needs it for the local echo, and
   * the panel sets it once — so a send never waits on a prop to be "mine".
   */
  viewerEmail: string
  setViewerEmail: (email: string) => void
  /** The inbox itself is loading (first paint / refresh). */
  loading: boolean
  /** Thread ids whose transcript is being fetched right now. */
  loadingThreads: Record<string, boolean>
  /** Thread ids with an in-flight send — the composer disables, the bubble shows. */
  sending: Record<string, boolean>
  /**
   * Message ids with an in-flight FLAG change (save / important / pin).
   *
   * Without this, clicking Save twice in quick succession fired two POSTs whose
   * answers could land out of order: the store applied each response as it
   * arrived, so the last-arriving (not last-clicked) state won and the bubble
   * could end up showing the opposite of what the server stored.
   */
  flagging: Record<string, boolean>
  error: string | null
  /** Threads whose messages have been fetched at least once. */
  loaded: Record<string, boolean>

  load: () => Promise<void>
  /** Loads one thread's messages. Idempotent, safe on every mount. */
  loadThread: (threadId: string) => Promise<void>
  /**
   * Re-reads the inbox without blanking transcripts, and (when given) pulls any
   * message newer than the last one this client holds. Used by the poller, so a
   * reply arrives without navigating away.
   */
  refresh: (threadId?: string) => Promise<void>
  /** Prepends the page before the oldest held message. False at the start. */
  loadOlder: (threadId: string) => Promise<boolean>
  /**
   * Sends the text. Resolves `true` when the server accepted it; the caller
   * keeps the draft otherwise. Failures stay in `error` AND on the bubble.
   */
  send: (threadId: string, text: string) => Promise<boolean>
  /** Drops a failed local echo (or re-sends it) — used by the retry affordance. */
  discardFailed: (threadId: string, messageId: string) => void
  markRead: (threadId: string) => Promise<void>
  /** Opens (or reuses) the conversation with a peer and returns its id. */
  ensureThreadWith: (input: { peerUserId: number }) => Promise<string | null>
  /** Edit your own message inside the window. Returns false when rejected. */
  editMessage: (threadId: string, messageId: string, text: string) => Promise<boolean>
  /** Soft delete — hides the message for this viewer only. */
  deleteMessage: (threadId: string, messageId: string) => Promise<boolean>
  /** Undo a delete: puts the message back in this viewer's transcript. */
  restoreMessage: (threadId: string, messageId: string) => Promise<boolean>
  /** Loads this viewer's own hidden messages (the restore list). */
  loadHidden: (threadId: string) => Promise<void>
  /** Pin / unpin (pass null to unpin). Staff only, enforced by the server. */
  setPinned: (threadId: string, messageId: string | null) => Promise<boolean>
  /** Lock / unlock — staff only, enforced by the server. */
  setLocked: (threadId: string, locked: boolean) => Promise<boolean>
  toggleSaved: (threadId: string, messageId: string, saved: boolean) => Promise<boolean>
  toggleImportant: (threadId: string, messageId: string) => Promise<boolean>
}

const clock = (ms: number) => {
  const d = new Date(ms)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getHours())}:${p(d.getMinutes())}`
}

const DAY = 86_400_000

function toMessage(m: ApiChatMessage): ChatMessage {
  const createdAt = m.created_at * 1000
  return {
    id: String(m.id),
    from: m.sender_email,
    fromName: m.sender_name || m.sender_email.split('@')[0] || m.sender_email,
    text: m.body,
    createdAt,
    time: clock(createdAt),
    editedAt: m.edited_at ? m.edited_at * 1000 : undefined,
    important: m.important || undefined,
    saved: m.saved || undefined,
  }
}

/**
 * A conversation row from `GET /api/chat/threads`.
 *
 * `messages` starts empty on purpose: the list is cheap and the transcript is a
 * separate page. What the row CAN answer immediately — the newest message, the
 * pin, the count — is carried over, because an inbox row that says "No messages
 * yet" about a conversation with forty messages is not a placeholder, it is a
 * lie the user reads.
 */
function toThread(t: ApiChatThread): ChatThread {
  const last = t.last_message ? toMessage(t.last_message) : null
  return {
    id: String(t.id),
    title: t.peer_name || t.peer_email,
    participants: [t.peer_email],
    messages: [],
    pinnedMessageId: t.pinned_message_id ? String(t.pinned_message_id) : null,
    pinnedPreview: null,
    locked: t.locked || undefined,
    lockedBy: t.locked ? 'staff' : undefined,
    unread: t.unread,
    lastMessage: last,
    totalMessages: last ? 1 : 0,
    hasMessages: false,
  }
}

function describe(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return 'Unexpected error'
}

/** Newest message this client HOLDS — the cursor a poll continues from. */
const newestHeld = (t: ChatThread) => {
  const held = t.messages[t.messages.length - 1]
  if (held) return Number(held.id)
  return t.lastMessage ? Number(t.lastMessage.id) : 0
}

/** Merges by id, keeping the server's copy of anything it sent twice. */
function mergeById(oldest: ChatMessage[], newer: ChatMessage[]): ChatMessage[] {
  const out = new Map<string, ChatMessage>()
  for (const m of oldest) out.set(m.id, m)
  for (const m of newer) out.set(m.id, m)
  return [...out.values()].sort((a, b) => a.createdAt - b.createdAt || Number(a.id) - Number(b.id))
}

/**
 * A list refresh must not blank what the user is reading. The transcript, the
 * pin preview and the "has a page" flag come from the client; the row itself
 * (title, unread, lock, newest message) comes from the server.
 */
function keepFetched(next: ChatThread, prev: ChatThread | undefined): ChatThread {
  if (!prev) return next
  return {
    ...next,
    messages: prev.messages,
    pinnedPreview: prev.pinnedPreview,
    hasMessages: prev.hasMessages,
    totalMessages: Math.max(prev.totalMessages, next.totalMessages),
  }
}

export const useChatStore = create<ChatState>()((set, get) => ({
  threads: [],
  viewerEmail: '',
  setViewerEmail: (email) => {
    if (get().viewerEmail !== email) set({ viewerEmail: email })
  },
  loading: false,
  loadingThreads: {},
  sending: {},
  flagging: {},
  error: null,
  loaded: {},

  load: async () => {
    set({ loading: true, error: null })
    try {
      const rows = await api.listChatThreads()
      // Keep messages already fetched for a thread: refreshing the list must
      // not blank the conversation the user is reading (or lose its anchor).
      const existing = new Map(get().threads.map((t) => [t.id, t]))
      set({ loading: false, threads: rows.map((r) => keepFetched(toThread(r), existing.get(String(r.id)))) })
    } catch (err) {
      set({ loading: false, error: describe(err) })
    }
  },

  loadThread: async (threadId) => {
    if (get().loadingThreads[threadId]) return
    set((s) => ({ loadingThreads: { ...s.loadingThreads, [threadId]: true } }))
    try {
      const rows = await api.listChatMessages(Number(threadId), 0, PAGE_SIZE)
      set((s) => ({
        error: null,
        loaded: { ...s.loaded, [threadId]: true },
        loadingThreads: { ...s.loadingThreads, [threadId]: false },
        threads: s.threads.map((t) =>
          t.id === threadId
            ? {
                ...t,
                messages: rows.map(toMessage),
                totalMessages: Math.max(t.totalMessages, rows.length),
                hasMessages: true,
              }
            : t,
        ),
      }))
    } catch (err) {
      set((s) => ({
        error: describe(err),
        loadingThreads: { ...s.loadingThreads, [threadId]: false },
      }))
    }
  },

  refresh: async (threadId) => {
    // The inbox is re-read first: unread counts, the pin, the lock and the
    // preview all move without a new message of ours.
    try {
      const rows = await api.listChatThreads()
      set((s) => {
        const existing = new Map(s.threads.map((t) => [t.id, t]))
        return { threads: rows.map((r) => keepFetched(toThread(r), existing.get(String(r.id)))) }
      })
    } catch {
      // A failed poll is silent: the next tick retries, and a banner blinking
      // every few seconds would be worse than a stale row.
    }

    if (!threadId || !get().loaded[threadId]) return
    const thread = get().threads.find((t) => t.id === threadId)
    // Nothing new is possible while our own send is in flight.
    if (!thread || get().sending[threadId]) return
    try {
      const rows = await api.listChatMessages(Number(threadId), newestHeld(thread), PAGE_SIZE)
      if (rows.length === 0) return
      const incoming = rows.map(toMessage)
      set((s) => ({
        threads: s.threads.map((t) => {
          if (t.id !== threadId) return t
          // A local echo the server already confirmed would duplicate its row.
          const kept = t.messages.filter(
            (m) => !(m.pending && incoming.some((i) => i.text === m.text && i.from === m.from)),
          )
          return {
            ...t,
            messages: mergeById(kept, incoming),
            totalMessages: Math.max(t.totalMessages, t.messages.length + incoming.length),
            lastMessage: incoming[incoming.length - 1],
          }
        }),
      }))
    } catch {
      // Same reasoning as the inbox read above.
    }
  },

  loadOlder: async (threadId) => {
    const thread = get().threads.find((t) => t.id === threadId)
    if (!thread || thread.messages.length === 0) return false
    if (thread.messages.length >= thread.totalMessages) return false
    if (get().loadingThreads[threadId]) return false
    set((s) => ({ loadingThreads: { ...s.loadingThreads, [threadId]: true } }))
    try {
      // The API cursor is forward-only, so "older" is the oldest page it hands
      // out; anything before the message we already hold is new to us.
      const page = (await api.listChatMessages(Number(threadId), 0, PAGE_SIZE)).map(toMessage)
      const firstHeld = Number(thread.messages[0].id)
      const older = page.filter((m) => Number(m.id) < firstHeld)
      set((s) => ({
        loadingThreads: { ...s.loadingThreads, [threadId]: false },
        threads: s.threads.map((t) =>
          t.id === threadId ? { ...t, messages: mergeById(older, t.messages) } : t,
        ),
      }))
      return older.length > 0
    } catch (err) {
      set((s) => ({
        error: describe(err),
        loadingThreads: { ...s.loadingThreads, [threadId]: false },
      }))
      return false
    }
  },

  send: async (threadId, text) => {
    const body = text.trim()
    if (!body) return false
    if (get().sending[threadId]) return false
    const viewer = get().viewerEmail
    // A local echo is drawn at once, so the composer can be cleared and the
    // message is visibly OUT there. It is provisional (`pending`), and swapped
    // for the server's row — the real id — when the POST answers.
    const now = Date.now()
    const echoId = `local-${now}`
    const echo: ChatMessage = {
      id: echoId,
      from: viewer,
      fromName: 'You',
      text: body,
      createdAt: now,
      time: clock(now),
      pending: true,
    }
    set((s) => ({
      sending: { ...s.sending, [threadId]: true },
      error: null,
      threads: s.threads.map((t) => (t.id === threadId ? { ...t, messages: [...t.messages, echo] } : t)),
    }))
    try {
      const saved = toMessage(await api.sendChatMessage(Number(threadId), body))
      set((s) => ({
        sending: { ...s.sending, [threadId]: false },
        threads: s.threads.map((t) =>
          t.id === threadId
            ? {
                ...t,
                messages: t.messages.map((m) => (m.id === echoId ? saved : m)),
                lastMessage: saved,
                totalMessages: Math.max(t.totalMessages + 1, t.messages.length),
              }
            : t,
        ),
      }))
      return true
    } catch (err) {
      // A locked thread answers 409 with a clear message. The echo stays — marked
      // failed — so the words are never lost; the caller restores the draft too.
      set((s) => ({
        sending: { ...s.sending, [threadId]: false },
        error: describe(err),
        threads: s.threads.map((t) =>
          t.id === threadId
            ? {
                ...t,
                messages: t.messages.map((m) => (m.id === echoId ? { ...m, pending: false, failed: true } : m)),
              }
            : t,
        ),
      }))
      return false
    }
  },

  discardFailed: (threadId, messageId) => {
    set((s) => ({
      threads: s.threads.map((t) =>
        t.id === threadId ? { ...t, messages: t.messages.filter((m) => m.id !== messageId) } : t,
      ),
    }))
  },

  markRead: async (threadId) => {
    try {
      await api.markChatRead(Number(threadId))
      set((s) => ({ threads: s.threads.map((t) => (t.id === threadId ? { ...t, unread: 0 } : t)) }))
    } catch {
      // A failed read-mark is not worth an error banner: the badge just stays.
    }
  },

  ensureThreadWith: async ({ peerUserId }) => {
    try {
      const thread = await api.openChatThread(peerUserId)
      const id = String(thread.id)
      if (!get().threads.some((t) => t.id === id)) {
        await get().load()
      }
      await get().loadThread(id)
      return id
    } catch (err) {
      set({ error: describe(err) })
      return null
    }
  },

  editMessage: async (threadId, messageId, text) => {
    const body = text.trim()
    if (!body) return false
    try {
      const updated = toMessage(await api.editChatMessage(Number(threadId), Number(messageId), body))
      set((s) => ({
        error: null,
        threads: s.threads.map((t) =>
          t.id === threadId
            ? {
                ...t,
                messages: t.messages.map((m) => (m.id === messageId ? updated : m)),
                // Editing the newest message changes the inbox preview too.
                lastMessage: t.lastMessage?.id === messageId ? updated : t.lastMessage,
              }
            : t,
        ),
      }))
      return true
    } catch (err) {
      set({ error: describe(err) })
      return false
    }
  },

  deleteMessage: async (threadId, messageId) => {
    try {
      await api.hideChatMessage(Number(threadId), Number(messageId))
      // The server removed it for this viewer only, so drop it from our copy and
      // clear the pin if it was the pinned one (the server does the same).
      set((s) => ({
        error: null,
        threads: s.threads.map((t) => {
          if (t.id !== threadId) return t
          const kept = t.messages.filter((m) => m.id !== messageId)
          return {
            ...t,
            pinnedMessageId: t.pinnedMessageId === messageId ? null : t.pinnedMessageId,
            pinnedPreview: t.pinnedMessageId === messageId ? null : t.pinnedPreview,
            messages: kept,
            // If the preview pointed at the hidden message, fall back to what is
            // left — "No messages yet" would be the old lie in a new place.
            lastMessage:
              t.lastMessage?.id === messageId ? (kept[kept.length - 1] ?? null) : t.lastMessage,
          }
        }),
      }))
      return true
    } catch (err) {
      set({ error: describe(err) })
      return false
    }
  },

  restoreMessage: async (threadId, messageId) => {
    try {
      const restored = toMessage(await api.unhideChatMessage(Number(threadId), Number(messageId)))
      set((s) => ({
        error: null,
        threads: s.threads.map((t) => {
          if (t.id !== threadId) return t
          // Splice it back where it belongs, not at the end: the server returned
          // the original id and timestamp, so ordering is the same as a re-read.
          const without = t.messages.filter((m) => m.id !== restored.id)
          return {
            ...t,
            messages: mergeById(without, [restored]),
            hiddenMessages: (t.hiddenMessages ?? []).filter((m) => m.id !== restored.id),
            lastMessage:
              !t.lastMessage || restored.createdAt >= t.lastMessage.createdAt ? restored : t.lastMessage,
          }
        }),
      }))
      return true
    } catch (err) {
      set({ error: describe(err) })
      return false
    }
  },

  loadHidden: async (threadId) => {
    try {
      const rows = await api.listHiddenChatMessages(Number(threadId))
      set((s) => ({
        threads: s.threads.map((t) =>
          t.id === threadId ? { ...t, hiddenMessages: rows.map(toMessage) } : t,
        ),
      }))
    } catch (err) {
      // Not fatal: the transcript is still readable, the restore list just stays
      // closed. Said out loud rather than swallowed, because it is the undo path.
      set({ error: describe(err) })
    }
  },

  setPinned: async (threadId, messageId) => {
    try {
      await api.pinChatMessage(Number(threadId), messageId ? Number(messageId) : 0)
      set((s) => ({
        error: null,
        threads: s.threads.map((t) =>
          t.id === threadId
            ? {
                ...t,
                pinnedMessageId: messageId,
                // Keep the banner readable even if the message itself is not
                // in the fetched page yet.
                pinnedPreview: messageId ? (t.messages.find((m) => m.id === messageId) ?? t.pinnedPreview) : null,
              }
            : t,
        ),
      }))
      return true
    } catch (err) {
      set({ error: describe(err) })
      return false
    }
  },

  setLocked: async (threadId, locked) => {
    try {
      await api.lockChatThread(Number(threadId), locked)
      set((s) => ({
        error: null,
        threads: s.threads.map((t) =>
          t.id === threadId
            ? { ...t, locked: locked || undefined, lockedBy: locked ? 'staff' : undefined }
            : t,
        ),
      }))
      return true
    } catch (err) {
      set({ error: describe(err) })
      return false
    }
  },

  toggleSaved: async (threadId, messageId, saved) => {
    if (get().flagging[messageId]) return false
    set((s) => ({ flagging: { ...s.flagging, [messageId]: true } }))
    try {
      await api.saveChatMessage(Number(threadId), Number(messageId), saved)
      set((s) => ({
        error: null,
        flagging: { ...s.flagging, [messageId]: false },
        threads: s.threads.map((t) =>
          t.id === threadId
            ? {
                ...t,
                messages: t.messages.map((m) => (m.id === messageId ? { ...m, saved: saved || undefined } : m)),
                // The inbox preview keeps its own copy of the newest message, so
                // a flag on it must move with the transcript or the row's marker
                // strip contradicts the bubble.
                lastMessage:
                  t.lastMessage?.id === messageId ? { ...t.lastMessage, saved: saved || undefined } : t.lastMessage,
              }
            : t,
        ),
      }))
      return true
    } catch (err) {
      set((s) => ({ error: describe(err), flagging: { ...s.flagging, [messageId]: false } }))
      return false
    }
  },

  toggleImportant: async (threadId, messageId) => {
    const thread = get().threads.find((t) => t.id === threadId)
    const msg = thread?.messages.find((m) => m.id === messageId)
    if (!msg) return false
    // One flag request at a time per message: a double click must not race.
    if (get().flagging[messageId]) return false
    const next = !msg.important
    set((s) => ({ flagging: { ...s.flagging, [messageId]: true } }))
    try {
      await api.markChatMessageImportant(Number(threadId), Number(messageId), next)
      set((s) => ({
        error: null,
        flagging: { ...s.flagging, [messageId]: false },
        threads: s.threads.map((t) =>
          t.id === threadId
            ? {
                ...t,
                messages: t.messages.map((m) => (m.id === messageId ? { ...m, important: next } : m)),
                lastMessage:
                  t.lastMessage?.id === messageId ? { ...t.lastMessage, important: next } : t.lastMessage,
              }
            : t,
        ),
      }))
      return true
    } catch (err) {
      set((s) => ({ error: describe(err), flagging: { ...s.flagging, [messageId]: false } }))
      return false
    }
  },
}))

/* ---------------- Derived selectors (pure, no hooks) ---------------- */

/**
 * The newest message this row knows about: the fetched transcript first, then
 * the server's `last_message`. The inbox reads this, so a row is truthful the
 * moment the list loads — not only after the user has opened it.
 */
export const newestMessage = (t: ChatThread): ChatMessage | null =>
  t.messages[t.messages.length - 1] ?? t.lastMessage ?? null

/** When the thread last changed — used to sort the list by activity. */
export const lastMessageAt = (t: ChatThread) => newestMessage(t)?.createdAt ?? 0

/** Unread as the server computed it for this viewer. */
export const unreadCount = (t: ChatThread) => t.unread

/**
 * Threads the viewer may see. The backend already scoped the list, so this is
 * only a sort — adding another filter here would be a role rule the server does
 * not enforce, which is exactly the kind of drift the mock store had.
 */
export const threadsFor = (threads: ChatThread[]) =>
  [...threads].sort((a, b) => lastMessageAt(b) - lastMessageAt(a))

export const totalUnread = (threads: ChatThread[]) =>
  threads.reduce((sum, t) => sum + t.unread, 0)

/** Messages the viewer may see — hidden ones never arrive from the server. */
export const visibleMessages = (t: ChatThread) => t.messages

export const unreadVisible = (t: ChatThread) => t.unread

export const pinnedMessage = (t: ChatThread) =>
  t.messages.find((m) => m.id === t.pinnedMessageId) ?? null

/** Can this viewer still edit that message (their own, inside the window)? */
export const canEditMessage = (m: ChatMessage, viewerEmail: string, now = Date.now()) =>
  m.from.toLowerCase() === viewerEmail.toLowerCase() && now - m.createdAt <= EDIT_WINDOW_MS

/** "Today" / "Yesterday" / "12 Oct" separator label. */
export const dayLabel = (ms: number) => {
  const d = new Date(ms)
  const midnight = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime()
  const days = Math.round((midnight(new Date()) - midnight(d)) / DAY)
  if (days <= 0) return 'Today'
  if (days === 1) return 'Yesterday'
  return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })
}

/** Clock for today, short date for older messages. */
export const messageStamp = (ms: number) =>
  Date.now() - ms < DAY ? clock(ms) : new Date(ms).toLocaleDateString(undefined, { day: 'numeric', month: 'short' })

/** Relative label for thread-list previews. */
export const relativeTime = (ms: number) => {
  if (!ms) return ''
  const diff = Date.now() - ms
  const minute = 60_000
  const hour = 60 * minute
  if (diff < minute) return 'now'
  if (diff < hour) return `${Math.floor(diff / minute)}m`
  if (diff < DAY) return `${Math.floor(diff / hour)}h`
  if (diff < 7 * DAY) return `${Math.floor(diff / DAY)}d`
  return new Date(ms).toLocaleDateString(undefined, { day: 'numeric', month: 'short' })
}
