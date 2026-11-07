/** Presentation helpers for chat — kept out of the component files so
 *  React Fast Refresh stays happy (only components are exported there).
 *  Person names come from `stores/peopleStore` (`nameOf`). */

import type { ChatMessage, ChatThread } from '../../stores/chatStore'

/** Initials avatar label shared by the thread list, panel header and picker. */
export const threadInitial = (title: string) => title.replace(/.*↔\s*/, '').charAt(0).toUpperCase() || '#'

/** Is this message the signed-in viewer's own? Case-insensitive: emails are. */
export const isMine = (m: ChatMessage, viewerEmail: string) =>
  !!viewerEmail && m.from.toLowerCase() === viewerEmail.toLowerCase()

/**
 * The inbox preview for one message, in the fewest words that still answer
 * "who spoke last, and what about?".
 *
 * The tint of a bubble cannot do this job in a 300px row: a reader skimming the
 * inbox needs the direction IN TEXT. So a message of yours is prefixed `You:`,
 * and a message with a marker carries the marker first — the same icons the
 * row's flag strip uses, so the symbols mean one thing everywhere.
 */
export function previewText(t: ChatThread, viewerEmail: string): string {
  const last = t.messages[t.messages.length - 1] ?? t.lastMessage ?? null
  if (!last) return 'No messages yet'

  const single = last.text.replace(/\s+/g, ' ').trim()
  const clipped = single.length > 90 ? `${single.slice(0, 90).trimEnd()}…` : single

  const marks: string[] = []
  if (t.pinnedMessageId === last.id) marks.push('📌')
  if (last.important) marks.push('⭐')
  if (last.saved) marks.push('🔖')
  marks.push(isMine(last, viewerEmail) ? 'You:' : `${last.fromName.split(/\s+/)[0]}:`)

  return `${marks.join(' ')} ${clipped}`
}

/**
 * What the last message proves about this row, without the transcript:
 * `loaded` is false while only the server's preview is known, which lets the
 * list show "open to check" instead of asserting there is nothing flagged.
 */
export function rowSignals(t: ChatThread, viewerEmail: string) {
  const last = t.messages[t.messages.length - 1] ?? t.lastMessage ?? null
  const loaded = t.messages.length > 0
  const mine = !!last && isMine(last, viewerEmail)
  return {
    last,
    loaded,
    mine,
    /** Pinned in this thread — conversation state, so known even when unloaded. */
    pinned: !!t.pinnedMessageId,
    /** A flag is only claimed from a message we actually hold. */
    important: loaded && t.messages.some((m) => m.important),
    saved: loaded ? t.messages.filter((m) => m.saved).length : 0,
  }
}
