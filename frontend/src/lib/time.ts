/**
 * Formatters shared by every surface that renders a stored instant.
 *
 * Notifications, VPN expiry, "last seen", usage buckets and audit rows are all
 * stored as **UTC Unix seconds** — an absolute moment, the only form that cannot
 * drift when the server moves datacentre or a country changes its rules.
 *
 * So the question these helpers answer is never "what time is it stored as" but
 * "what time does a PERSON read". That is a display decision, and it is made
 * once, from the server (D-099):
 *
 *   - The zone is a server setting (`Asia/Colombo`), published through /api/me.
 *   - Every formatter here takes it explicitly, so no screen can quietly fall
 *     back to the device's own clock. Rendered from the device, one VPN's
 *     "last seen 14:30" reads 20:00 to a customer in Colombo and 22:30 to the
 *     same customer on a trip, and the support call starts with two people
 *     disputing a time neither of them got wrong.
 *
 * The fallback when a zone is unknown is the browser's own zone — visibly
 * labelled as such by `zoneLabel`, never silently presented as the server's.
 */

/** The server's display zone, or the browser's when the server did not say. */
export const FALLBACK_TIMEZONE: string | undefined = undefined

/**
 * The zone the current page renders instants in, set once from `GET /api/me`.
 *
 * A module-level value rather than a React context on purpose: every formatter
 * below is a plain function used from render bodies, memoised lists and CSV
 * builders alike. Threading a zone through each of those would mean either a
 * prop on every component or a hook that only works inside a component — and a
 * formatter that silently fell back to the device whenever it was called from
 * outside one is exactly the defect this change exists to remove.
 *
 * It is set before any authenticated screen renders (`authStore.hydrate`), and
 * left unset for the sign-in screens, which show no stored instants.
 */
let activeTimezone: string | undefined

/** Record the zone the server publishes. Empty leaves it unset. */
export function setActiveTimezone(zone?: string): void {
  activeTimezone = zone && zone.trim() !== '' ? zone.trim() : undefined
}

/** The zone in force for this page; `undefined` means "the device's own". */
export function activeTimeZone(): string | undefined {
  return activeTimezone
}

/**
 * A short, human label for the zone an instant was rendered in.
 *
 * "SLST" is what a Sri Lankan customer calls +05:30, and it is what a support
 * operator will say out loud. It is derived from the zone rather than hard-coded
 * so the label cannot outlive the setting that produced it.
 */
export function zoneLabel(timeZone: string | undefined = activeTimezone): string {
  if (!timeZone) return 'local time'
  if (timeZone === 'Asia/Colombo') return 'SLST'
  if (timeZone === 'UTC') return 'UTC'
  return timeZone
}

/** Build the `Intl` options once per zone — `Intl.DateTimeFormat` is expensive. */
function optionsFor(
  timeZone: string | undefined,
  options: Intl.DateTimeFormatOptions,
): Intl.DateTimeFormatOptions {
  // An invalid zone string makes every `Intl` call throw. Letting the formatter
  // use the device zone instead is the honest degradation: a wrong-but-visible
  // time beats a blank cell or a crash on a card.
  return timeZone ? { ...options, timeZone } : options
}

/** "just now" · "5m ago" · "3h ago" · "2d ago" · "12 Oct 2026" */
export function relativeTime(
  unixSeconds: number,
  now = Date.now(),
  timeZone: string | undefined = activeTimezone,
): string {
  if (!unixSeconds) return ''
  const delta = Math.floor(now / 1000) - unixSeconds
  // Sub-week deltas are relative on purpose: "3h ago" is true in every zone and
  // needs no timezone at all, so there is nothing here to get wrong.
  if (delta < 60) return 'just now'
  if (delta < 3600) return `${Math.floor(delta / 60)}m ago`
  if (delta < 86_400) return `${Math.floor(delta / 3600)}h ago`
  if (delta < 7 * 86_400) return `${Math.floor(delta / 86_400)}d ago`
  return new Date(unixSeconds * 1000).toLocaleDateString(
    'en-GB',
    optionsFor(timeZone, { day: 'numeric', month: 'short', year: 'numeric' }),
  )
}

/**
 * An exact timestamp, for fields where "how long ago" is not enough — a client's
 * join date, an expiry, or the last sign-in an operator is checking against a
 * report.
 *
 * Renders "—" for 0, because "1 Jan 1970" is worse than saying "we do not know".
 */
export function formatDateTime(
  unixSeconds: number,
  timeZone: string | undefined = activeTimezone,
): string {
  if (!unixSeconds) return '—'
  return new Date(unixSeconds * 1000).toLocaleString(
    'en-GB',
    optionsFor(timeZone, {
      day: 'numeric',
      month: 'short',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    }),
  )
}

/**
 * The same instant, with the zone named — for the few places where a reader must
 * be able to tell WHICH clock they are reading (an expiry, a "last seen", an
 * audit row). "9 Nov 2026, 23:59 (SLST)" cannot be misread the way a bare
 * "23:59" can.
 */
export function formatDateTimeWithZone(
  unixSeconds: number,
  timeZone: string | undefined = activeTimezone,
): string {
  if (!unixSeconds) return '—'
  return `${formatDateTime(unixSeconds, timeZone)} (${zoneLabel(timeZone)})`
}

/**
 * Midnight (or the last second of the day) in a NAMED zone, as Unix seconds.
 *
 * This is the function that makes a date range mean what the customer said. A
 * `<input type="date">` yields "2026-11-01", and `new Date('2026-11-01T00:00:00')`
 * reads it in the DEVICE's zone — so a customer in Colombo asking for "1st to
 * 3rd" would send a window that starts 5h30 early and ends 5h30 early, silently
 * including the wrong slice of traffic.
 *
 * The offset is measured, not assumed:
 *
 *  1. Ask `Intl` what wall-clock time the zone shows for the target instant.
 *  2. Read the zone's numeric parts with `formatToParts`, which is unambiguous.
 *  3. Rebuild that wall clock as if it were UTC, and subtract.
 *
 * Step 2 is why this does NOT parse `toLocaleString()`: that returns a
 * locale-shaped string ("11/1/2026, 12:00:00 AM"), and `new Date(...)` on it
 * re-interprets it in the DEVICE's locale. On a host at +08:00 the round trip
 * silently applied the wrong offset twice and pushed midnight to 08:00.
 *
 * The offset is taken for THAT date, so a zone with DST resolves correctly on
 * either side of the change.
 */
export function zonedDayBounds(
  dateStr: string,
  timeZone: string | undefined = activeTimezone,
  edge: 'start' | 'end' = 'start',
): number | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(dateStr)
  if (!m) return null
  const year = Number(m[1])
  const month = Number(m[2]) - 1
  const day = Number(m[3])

  if (!timeZone) {
    // No zone configured: fall back to the device, which is what the screen did
    // before D-099. Still returns a real instant rather than nothing.
    const local = new Date(year, month, day, edge === 'start' ? 0 : 23,
      edge === 'start' ? 0 : 59, edge === 'start' ? 0 : 59)
    return Number.isNaN(local.getTime()) ? null : Math.floor(local.getTime() / 1000)
  }

  const wall = edge === 'start'
    ? { hour: 0, minute: 0, second: 0 }
    : { hour: 23, minute: 59, second: 59 }

  // The wall clock we want, expressed as a UTC epoch — a placeholder to be
  // corrected by the zone's offset.
  const wanted = Date.UTC(year, month, day, wall.hour, wall.minute, wall.second)

  // The zone's offset (in minutes) at that moment. `formatToParts` gives named
  // fields, so no locale or day/month ordering can corrupt it.
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).formatToParts(new Date(wanted))

  const pick = (type: string) => {
    const p = parts.find((x) => x.type === type)
    return p ? Number(p.value) : 0
  }
  // `hour` can come back as 24 for midnight in some engines; normalise it.
  const shownAsUTC = Date.UTC(
    pick('year'),
    pick('month') - 1,
    pick('day'),
    pick('hour') % 24,
    pick('minute'),
    pick('second'),
  )
  const offsetMs = shownAsUTC - wanted
  return Math.floor((wanted - offsetMs) / 1000)
}
