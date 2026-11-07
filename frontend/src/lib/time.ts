/**
 * Formatters shared by the inbox surfaces.
 *
 * Notifications are stored as Unix seconds, so the UI renders "how long ago"
 * from a real timestamp — never a hard-coded string like the old mock's
 * "12m ago", which stayed 12 minutes old forever.
 */

/** "just now" · "5m ago" · "3h ago" · "2d ago" · "12 Oct 2026" */
export function relativeTime(unixSeconds: number, now = Date.now()): string {
  if (!unixSeconds) return ''
  const delta = Math.floor(now / 1000) - unixSeconds
  if (delta < 60) return 'just now'
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`
  if (delta < 86_400) return `${Math.floor(delta / 3600)}h ago`
  if (delta < 7 * 86_400) return `${Math.floor(delta / 86_400)}d ago`
  return new Date(unixSeconds * 1000).toLocaleDateString('en-GB', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  })
}

/**
 * An exact timestamp, for fields where "how long ago" is not enough — a client's
 * join date, or the last sign-in an operator is checking against a report.
 * Renders "—" for 0, because "1 Jan 1970" is worse than saying "we do not know".
 */
export function formatDateTime(unixSeconds: number): string {
  if (!unixSeconds) return '—'
  return new Date(unixSeconds * 1000).toLocaleString('en-GB', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}
