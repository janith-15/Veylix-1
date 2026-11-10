import { Fragment, useEffect, useMemo, useState } from 'react'
import { Activity, CalendarRange, Clock, Gauge, TriangleAlert } from 'lucide-react'
import { Button, Card, Select, Tabs, formatBytesIn, BYTE_UNITS, unitLabel, type ByteUnit } from '../ui'
import { BarChart, LineChart, Progress, type Series } from '../charts'
import { activeTimeZone, formatDateTime, zoneLabel, zonedDayBounds } from '../../lib/time'
import {
  api,
  ApiError,
  type ApiHourlyBucket,
  type ApiSpeedPoint,
  type ApiUsagePoint,
  type ApiUsageRange,
  type ApiVpnUsage,
} from '../../lib/api'
import './vpn-usage-insights.css'

/**
 * The Usage section of the VPN details page (owner's brief §6 Section 02).
 *
 * WHAT WAS WRONG BEFORE, AND WHAT REPLACES IT
 *
 * The previous component drew four charts that answered one badly-asked
 * question:
 *
 *   - a cumulative line (rises forever, says nothing about *when*)
 *   - a bar of TWO values (Download/Upload), so 7d, 30d, 90d and 1y all looked
 *     identical — the range switch changed the numbers but never the shape
 *   - a donut of those same two numbers
 *   - a server CPU/RAM chart, which is an operator metric on a customer screen
 *
 * Every chart below answers a different question, and each names its window:
 *
 *   A. Usage trend      — "when did my traffic happen?"       (line, deltas)
 *   B. Package data     — "how much of my allowance is gone?" (progress bar)
 *   C. When you use it  — "which part of the day is heavy?"   (4 bars)
 *   D. Performance      — "was my connection fast, and when?" (line, bps)
 *
 * THE RANGE SWITCH ACTUALLY SWITCHES THE SHAPE
 *
 * A, C and D all read the same window, and the backend reports a bucket size
 * that fits it — hourly for 24h, daily for 30d, and so on. The chart is drawn
 * from however many points that window really produced, so 24h is dense and
 * 1y is sparse. That difference IS the feature.
 *
 * THINGS THIS COMPONENT REFUSES TO DO
 *
 *   - Draw a zero line for a window with no samples. It says "no data".
 *   - Present an `unavailable` speed interval as 0 B/s. It draws a gap.
 *   - Claim a speed test happened. The scope label travels with the number.
 *   - Show CPU/RAM. That is `/servers/:id`, for staff (brief §8).
 */

interface Props {
  vpnId: number
  /** `FREE` gets the simple view: totals, progress bar, expiry. No charts. */
  kind: 'FREE' | 'PREMIUM' | 'UNTAGGED'
  active: boolean
  /**
   * The VPN's current status and when it entered it (D-100).
   *
   * The chart below is drawn from CUMULATIVE counters, so once readings stop the
   * line goes flat — and a flat line reads as "no traffic", when the truth is
   * "nobody is measuring". These two fields let the chart say which of the two it
   * is instead of letting the reader decide, wrongly.
   *
   * Both are optional: a caller that does not know the status gets the previous
   * behaviour, never a guessed caption.
   */
  status?: string
  statusChangedAt?: number | null
}

/** The ranges offered. `custom` is driven by the two date inputs. */
const RANGES: { value: ApiUsageRange; label: string }[] = [
  { value: '24h', label: '24 hours' },
  { value: '7d', label: '7 days' },
  { value: '30d', label: '30 days' },
  { value: '3m', label: '90 days' },
  { value: '1y', label: '1 year' },
]

/** Labels for the four parts of a day, matching the brief's wording. */
const DAY_PARTS = ['Morning', 'Afternoon', 'Evening', 'Night'] as const

const DAY_PART_HINT = 'Morning 05–12 · Afternoon 12–17 · Evening 17–22 · Night 22–05'

/**
 * Which part of the day an hour belongs to.
 *
 * These boundaries are the ONLY definition in the app — a second copy elsewhere
 * would eventually disagree and move traffic from one bar to another.
 */
function dayPartOf(hour: number): number {
  if (hour >= 5 && hour < 12) return 0
  if (hour >= 12 && hour < 17) return 1
  if (hour >= 17 && hour < 22) return 2
  return 3
}

function dayLabel(sec: number): string {
  // D-099: rendered in the SERVER's zone, because the bucket boundary was built
  // there. Labelling from the device would print a time up to 5h30 away from the
  // data the bar actually covers.
  return new Date(sec * 1000).toLocaleDateString(undefined, {
    day: '2-digit',
    month: 'short',
    timeZone: activeTimeZone(),
  })
}

function hourLabel(sec: number): string {
  return new Date(sec * 1000).toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: activeTimeZone(),
  })
}

function formatRate(bps: number): string {
  if (!Number.isFinite(bps) || bps < 0) return '—'
  if (bps < 1024) return `${bps.toFixed(0)} B/s`
  const units = ['kB/s', 'MB/s', 'GB/s']
  let v = bps / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v >= 10 ? 0 : 1)} ${units[i]}`
}

/**
 * The label for one bucket of the current window.
 *
 * Driven by the REAL spacing between the points the backend returned, not by
 * the range name: hourly points get a clock time, daily points get a date.
 * Labelling by range name would print "00:00" over a whole day of traffic.
 */
function bucketLabel(sec: number, spanSec: number): string {
  return spanSec >= 86400 ? dayLabel(sec) : hourLabel(sec)
}

/** The delta series a window-trend chart needs, computed from the totals. */
function trendSeries(points: ApiUsagePoint[]): Series[] {
  const down: number[] = []
  const up: number[] = []
  for (let i = 0; i < points.length; i++) {
    const cur = points[i]
    const prev = i === 0 ? null : points[i - 1]
    // The first point has no predecessor inside the window, so it contributes
    // 0 rather than its whole lifetime total.
    down.push(prev ? Math.max(0, cur.lifetime_down_bytes - prev.lifetime_down_bytes) : 0)
    up.push(prev ? Math.max(0, cur.lifetime_up_bytes - prev.lifetime_up_bytes) : 0)
  }
  return [
    { label: 'Download', values: down },
    { label: 'Upload', values: up },
  ]
}

export function VpnUsageSection({ vpnId, kind, active, status, statusChangedAt }: Props) {
  const [range, setRange] = useState<ApiUsageRange>('30d')
  const [customOpen, setCustomOpen] = useState(false)
  const [fromDate, setFromDate] = useState('')
  const [toDate, setToDate] = useState('')

  /*
   * The display unit for every figure and every axis below.
   *
   * `auto` is the default because it is the honest reading of one number. A
   * pinned unit exists for the AXIS: ticks that jump from `900 MB` to `1.1 GB`
   * across one window are hard to compare, and a reader who wants to reason in
   * GB should be able to say so. It is display only — it never triggers a fetch,
   * because it changes no data the server holds.
   */
  const [unit, setUnit] = useState<ByteUnit>('auto')
  const fmt = useMemo(() => (n: number) => formatBytesIn(n, unit), [unit])

  const [usage, setUsage] = useState<ApiVpnUsage | null>(null)
  const [usageError, setUsageError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const [hourly, setHourly] = useState<ApiHourlyBucket[] | null>(null)
  const [hourlyError, setHourlyError] = useState<string | null>(null)

  const [speed, setSpeed] = useState<ApiSpeedPoint[] | null>(null)
  const [speedError, setSpeedError] = useState<string | null>(null)

  /* The custom window as epoch seconds. `null` means the inputs are not both
     filled, in which case the named range is used unchanged.
     
     D-099: "1 Nov → 3 Nov" means midnight to midnight IN THE SERVER'S ZONE, not
     in the device's. Reading the dates with `new Date('…T00:00:00')` used the
     device clock, so a customer in Colombo asking for 1st–3rd sent a window that
     started and ended 5h30 early — silently including the wrong slice of traffic
     and excluding the end of the 3rd. `zonedDayBounds` resolves each edge in the
     configured zone, and `to` is pushed to the END of its day so the range
     includes the day the customer named. */
  const bounds = useMemo(() => {
    if (!customOpen || fromDate === '' || toDate === '') return undefined
    const since = zonedDayBounds(fromDate, activeTimeZone(), 'start')
    const until = zonedDayBounds(toDate, activeTimeZone(), 'end')
    if (since === null || until === null) return undefined
    if (since >= until) return undefined
    return { since, until }
  }, [customOpen, fromDate, toDate])

  const effectiveRange: ApiUsageRange = bounds ? 'custom' : range

  /* -------- the totals read (A and B) ---------------------------------- */
  useEffect(() => {
    const ctrl = new AbortController()
    const timer = window.setTimeout(() => {
      setBusy(true)
      setUsageError(null)
      void api
        .vpnUsage(vpnId, effectiveRange, 240, bounds)
        .then((u) => {
          if (!ctrl.signal.aborted) setUsage(u)
        })
        .catch((err: unknown) => {
          if (ctrl.signal.aborted) return
          setUsage(null)
          setUsageError(err instanceof ApiError ? err.message : 'Usage history could not be read')
        })
        .finally(() => {
          if (!ctrl.signal.aborted) setBusy(false)
        })
    }, 0)
    return () => {
      ctrl.abort()
      window.clearTimeout(timer)
    }
  }, [vpnId, effectiveRange, bounds])

  /* -------- C: hour-of-day distribution -------------------------------- */
  useEffect(() => {
    if (kind === 'FREE') return
    const ctrl = new AbortController()
    const timer = window.setTimeout(() => {
      setHourlyError(null)
      void api
        .vpnUsageHourly(vpnId, effectiveRange, bounds)
        .then((res) => {
          if (!ctrl.signal.aborted) setHourly(res.buckets)
        })
        .catch((err: unknown) => {
          if (ctrl.signal.aborted) return
          setHourly(null)
          setHourlyError(err instanceof ApiError ? err.message : 'Hourly usage could not be read')
        })
    }, 0)
    return () => {
      ctrl.abort()
      window.clearTimeout(timer)
    }
  }, [vpnId, effectiveRange, bounds, kind])

  /* -------- D: performance --------------------------------------------- */
  useEffect(() => {
    if (kind === 'FREE') return
    const ctrl = new AbortController()
    const timer = window.setTimeout(() => {
      setSpeedError(null)
      void api
        .vpnSpeedHistory(vpnId, effectiveRange, bounds)
        .then((res) => {
          if (!ctrl.signal.aborted) setSpeed(res.points)
        })
        .catch((err: unknown) => {
          if (ctrl.signal.aborted) return
          setSpeed(null)
          setSpeedError(err instanceof ApiError ? err.message : 'Speed history could not be read')
        })
    }, 0)
    return () => {
      ctrl.abort()
      window.clearTimeout(timer)
    }
  }, [vpnId, effectiveRange, bounds, kind])

  const limit = usage?.limit_bytes ?? 0
  const used = usage?.lifetime_total_bytes ?? 0
  const usagePct = usage?.usage_pct ?? 0
  const unlimited = limit <= 0
  const points = useMemo(() => usage?.points ?? [], [usage])
  const hasPoints = points.length > 1

  /* The spacing between the first two points decides whether a bucket label is
     a clock time or a date. */
  const bucketSpan = points.length > 1 ? points[1].taken_at - points[0].taken_at : 86400
  const trendLabels = useMemo(
    () => points.map((p) => bucketLabel(p.taken_at, bucketSpan)),
    [points, bucketSpan],
  )
  const series = useMemo(() => trendSeries(points), [points])

  /* Hour-of-day aggregation. The backend returns hourly buckets; the brief asks
     for four parts of the day, so they are summed here — over buckets that
     actually carry measurements. */
  const dayPartData = useMemo(() => {
    const byPart = [0, 0, 0, 0]
    for (const b of hourly ?? []) byPart[dayPartOf(b.hour_of_day)] += b.total_bytes
    return DAY_PARTS.map((label, i) => ({ label, value: byPart[i] }))
  }, [hourly])

  const hourlyTotal = useMemo(() => dayPartData.reduce((a, b) => a + b.value, 0), [dayPartData])

  /*
   * The busiest hour of the day, as a SENTENCE.
   *
   * This is the one-line answer to "when should I avoid using this?" — a chart
   * of four bars makes the reader do the comparison themselves. `peak` is null
   * when nothing was recorded, and the card then says so rather than picking
   * hour 0 as a winner among zeros.
   */
  const peak = useMemo(() => {
    const byHour = new Array<number>(24).fill(0)
    for (const b of hourly ?? []) {
      if (b.hour_of_day >= 0 && b.hour_of_day < 24) byHour[b.hour_of_day] += b.total_bytes
    }
    let bestHour = -1
    let best = 0
    for (let h = 0; h < 24; h++) {
      if (byHour[h] > best) {
        best = byHour[h]
        bestHour = h
      }
    }
    if (bestHour < 0 || best <= 0) return null
    return { hour: bestHour, bytes: best, byHour }
  }, [hourly])

  /* Daily totals for the bar chart (6.6), from the same points the trend uses.
     Bucketed by calendar day in the reader's own zone, so a "day" is a day they
     recognise rather than a UTC boundary crossing their evening. */
  const dailyBars = useMemo(() => {
    const byDay = new Map<string, number>()
    for (const p of points) {
      const key = new Date(p.taken_at * 1000).toLocaleDateString(undefined, { timeZone: activeTimeZone() })
      byDay.set(key, (byDay.get(key) ?? 0) + p.total_bytes)
    }
    return [...byDay.entries()].slice(-14).map(([label, value]) => ({ label, value }))
  }, [points])

  /*
   * The 7-day x 24-hour heatmap (6.5).
   *
   * The backend returns `hour_start` (a full timestamp) AND `hour_of_day`, so the
   * weekday and the hour are both available without a new endpoint. `hour_of_day`
   * is UTC by design, so the weekday is derived from the same UTC instant to keep
   * a bucket in exactly one cell.
   */
  const heatmap = useMemo(() => {
    const grid: number[][] = Array.from({ length: 7 }, () => new Array<number>(24).fill(0))
    let max = 0
    let total = 0
    for (const b of hourly ?? []) {
      const d = new Date(b.hour_start * 1000)
      const dow = d.getUTCDay()
      if (dow < 0 || dow > 6 || b.hour_of_day < 0 || b.hour_of_day > 23) continue
      grid[dow][b.hour_of_day] += b.total_bytes
      if (grid[dow][b.hour_of_day] > max) max = grid[dow][b.hour_of_day]
      total += b.total_bytes
    }
    return { grid, max, total }
  }, [hourly])

  const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

  /*
   * The exhaustion estimate (6.8).
   *
   * DELIBERATELY AN ESTIMATE, AND LABELLED AS ONE. The server never states a
   * projected date, so this extrapolates the observed rate over the current
   * window. It is computed ONLY from figures the server did state
   * (`remaining_bytes`, `usage_pct`) plus the window's own timestamps, and it is
   * never shown as a fact — the card carries "an estimate, not a guarantee".
   *
   * It returns null when the answer would be meaningless: no limit, nothing
   * left, or no measurable traffic (a zero rate projects to "never", which is
   * not something to tell a customer).
   */
  const projection = useMemo(() => {
    if (limit <= 0 || !usage) return null
    const remaining = Math.max(0, usage.remaining_bytes ?? limit - used)
    if (remaining <= 0) return { exhausted: true as const }
    if (points.length < 2) return null
    const first = points[0]
    const last = points[points.length - 1]
    const spanSec = last.taken_at - first.taken_at
    if (spanSec <= 0) return null
    const moved = Math.max(0, last.lifetime_total_bytes - first.lifetime_total_bytes)
    if (moved <= 0) return null
    const perDay = moved / (spanSec / 86400)
    if (perDay <= 0) return null
    const days = remaining / perDay
    /* Beyond two years the number stops being useful and starts being noise. */
    if (!Number.isFinite(days) || days > 730) return null
    return { exhausted: false as const, days: Math.ceil(days), perDay }
  }, [limit, usage, used, points])


  /* Measured intervals only. An `unavailable` interval is a gap, so it gets no
     point at all rather than a zero. */
  const measured = useMemo(() => (speed ?? []).filter((p) => p.source === 'measured'), [speed])
  const speedLabels = useMemo(
    () => measured.map((p) => bucketLabel(p.taken_at, bucketSpan)),
    [measured, bucketSpan],
  )
  const speedSeries = useMemo<Series[]>(
    () => [
      { label: 'Download', values: measured.map((p) => p.down_bps) },
      { label: 'Upload', values: measured.map((p) => p.up_bps) },
    ],
    [measured],
  )
  const lastSpeed = measured.length > 0 ? measured[measured.length - 1] : null

  const windowLabel =
    effectiveRange === 'custom'
      ? 'the dates you chose'
      : (RANGES.find((r) => r.value === effectiveRange)?.label ?? effectiveRange)

  /*
   * What the selected window ACTUALLY covers, in words (D-096).
   *
   * `windowLabel` is what the customer asked for. Before this, that label was
   * all the chart said, so a 30-day selection rendered "Last 30 days" over an
   * 18-hour slice of data and the axis read "30d ago .. now" — a claim the
   * points never supported. This derives the honest span from the points
   * themselves, and is empty when there is nothing to describe.
   */
  const realSpanSec = useMemo(() => {
    if (points.length < 2) return 0
    const first = points[0].taken_at
    const last = points[points.length - 1].taken_at
    return last > first ? last - first : 0
  }, [points])

  const realSpanLabel = useMemo(() => {
    if (realSpanSec <= 0) return ''
    if (realSpanSec < 7200) return `${Math.round(realSpanSec / 60)} min`
    if (realSpanSec < 172800) return `${Math.round(realSpanSec / 3600)} h`
    const days = Math.round(realSpanSec / 86400)
    return `${days} day${days === 1 ? '' : 's'}`
  }, [realSpanSec])

  /* The width one plotted point covers, as the server reported it (D-096). */
  const bucketWidthLabel = useMemo(() => {
    const name = usage?.window?.bucket ?? usage?.bucket ?? ''
    if (name) return name
    const sec = usage?.window?.bucket_sec ?? usage?.bucket_sec ?? 0
    if (sec >= 2592000) return 'month'
    if (sec >= 604800) return 'week'
    if (sec >= 86400) return 'day'
    if (sec >= 3600) return `${Math.round(sec / 3600)}h`
    return ''
  }, [usage])

  /**
   * Did readings STOP part-way through this window? (D-100)
   *
   * The chart is drawn from cumulative counters, so when the server stops
   * recording, the deltas become zero and the line goes FLAT. A flat line is
   * indistinguishable from "connected but idle" — two very different facts that
   * a customer will read as the second one. This detects the first.
   *
   * It fires only on two pieces of evidence the server actually sent:
   *   - the VPN is not ACTIVE, and
   *   - its status changed AFTER the newest point in the window.
   *
   * Anything less than that is silence, not a claim: no status, no
   * `status_changed_at`, or a status change preceded by a fresh reading all
   * leave the chart exactly as it was.
   *
   * The last point is shown as the date the counters stopped. This is NOT a
   * backfill and NOT a notification — it is a label on a line that already
   * exists.
   */
  const readingsStopped = useMemo<{ at: number; label: string } | null>(() => {
    if (!points.length) return null
    if (!status || status === 'ACTIVE') return null
    if (statusChangedAt === null || statusChangedAt === undefined || statusChangedAt <= 0) return null
    const lastReading = points[points.length - 1].taken_at
    // A reading newer than the status change means the VPN is being measured
    // again (a restored or re-synced row): no flat line to explain.
    if (statusChangedAt <= lastReading) return null
    return { at: lastReading, label: formatDateTime(lastReading) }
  }, [points, status, statusChangedAt])

  /* The status in lowercase for the middle of a sentence, so the caption reads
     "suspended" and not "SUSPENDED" — the badge above already shouts it. */
  const stoppedBecause = (status ?? '').toLowerCase().replace(/_/g, ' ')

  /* -------- the switch, shared by every chart -------------------------- */
  const rangeSwitch = (
    <div className="vui-switch">
      <Tabs
        items={RANGES.map((r) => ({ value: r.value, label: r.label }))}
        value={range}
        onChange={(v) => {
          setRange(v as ApiUsageRange)
          setCustomOpen(false)
        }}
      />
      <Button
        size="sm"
        variant={customOpen ? 'primary' : 'outline'}
        leading={<CalendarRange size={13} />}
        onClick={() => setCustomOpen((o) => !o)}
      >
        Custom
      </Button>
      {/* The unit is display-only, so it sits with the range rather than in a
          settings panel: both answer "how am I reading this chart?". */}
      <Select
        id="vui-unit"
        value={unit}
        aria-label="Display unit for usage figures"
        data-testid="vui-unit"
        onChange={(e) => setUnit(e.target.value as ByteUnit)}
      >
        {BYTE_UNITS.map((u) => (
          <option key={u} value={u}>
            {u === 'auto' ? 'Auto units' : unitLabel(u)}
          </option>
        ))}
      </Select>
    </div>
  )

  return (
    <div className="vui" data-testid="vpn-usage-section">
      {rangeSwitch}

      {customOpen && (
        <div className="vui-custom">
          <label className="vui-custom__field">
            <span>From</span>
            <input
              type="date"
              value={fromDate}
              onChange={(e) => setFromDate(e.target.value)}
              aria-label="Start date"
            />
          </label>
          <label className="vui-custom__field">
            <span>To</span>
            <input
              type="date"
              value={toDate}
              onChange={(e) => setToDate(e.target.value)}
              aria-label="End date"
            />
          </label>
          {!bounds && (
            <span className="vui-custom__hint">
              {fromDate === '' || toDate === ''
                ? 'Pick both dates to use a custom range.'
                : 'The start must be before the end.'}
            </span>
          )}
        </div>
      )}

      {usageError && (
        <div className="vui-warn" role="alert">
          <TriangleAlert size={15} aria-hidden="true" />
          <span>{usageError}. This is a connection problem — not an empty account.</span>
        </div>
      )}

      {/* ---------- B: package data — the plainest question ---------- */}
      <Card title="Package data">
        {busy && !usage ? (
          <p className="vui-muted">Loading usage…</p>
        ) : !usage ? (
          <p className="vui-muted">Usage is unavailable right now.</p>
        ) : (
          <div className="vui-pkg">
            <Progress
              value={unlimited ? 0 : usagePct}
              label={
                unlimited
                  ? `${fmt(used)} used — no data limit on this VPN`
                  : `${fmt(used)} of ${fmt(limit)} used`
              }
            />
            <div className="vui-pkg__figures">
              <span>
                <strong>{unlimited ? 'Unlimited' : fmt(Math.max(0, limit - used))}</strong>
                <span className="vui-muted">{unlimited ? 'data' : 'left'}</span>
              </span>
              <span>
                <strong>{fmt(usage.lifetime_down_bytes)}</strong>
                <span className="vui-muted">download</span>
              </span>
              <span>
                <strong>{fmt(usage.lifetime_up_bytes)}</strong>
                <span className="vui-muted">upload</span>
              </span>
              <span>
                <strong>
                  {usage.expires_at
                    ? new Date(usage.expires_at * 1000).toLocaleDateString(undefined, { timeZone: activeTimeZone() })
                    : 'No expiry'}
                </strong>
                <span className="vui-muted">expires</span>
              </span>
            </div>
          </div>
        )}
      </Card>

      {/* A FREE VPN stops here: the brief asks for a simple usage summary and
          no advanced charting. */}
      {kind === 'FREE' && (
        <p className="vui-muted vui-note">
          Charts are part of Premium. Your data totals above are read from the server the same
          way a Premium VPN&apos;s are.
        </p>
      )}


      {kind !== 'FREE' && (
        <>
          {/* ---------- A: usage over time ---------- */}
          <Card
            title="Usage over time"
            actions={
          /*
           * "Last 30 days" alone was the lie (D-096): it named the range that
           * was REQUESTED, while the chart showed whatever span the data had.
           * The requested window stays (it is what the switch says), but the
           * real coverage is stated beside it whenever the two disagree.
           */
          <span className="vui-muted t-xs" data-testid="usage-window-truth">
            Last {windowLabel}
            {realSpanLabel ? ` · ${realSpanLabel} of data` : ''}
            {bucketWidthLabel ? ` · ${bucketWidthLabel}/point` : ''}
          </span>
        }
          >
            {!active && readingsStopped === null ? (
              <p className="vui-muted">
                This VPN is not active, so its traffic is not changing. The history it recorded
                is still shown here.
              </p>
            ) : null}
            {/* D-100: the line is flat, and WHY matters.
                Without this, a suspended VPN's chart is a horizontal line that
                reads as "you used nothing" — when the honest meaning is "nobody
                measured anything after this point". Both sentences below are
                derived from a real status and a real timestamp; neither invents
                data, and there is no backfill behind them. */}
            {readingsStopped !== null && (
              <p className="vui-muted vui-note" data-testid="vui-readings-stopped">
                Counters stopped on {readingsStopped.label} ({zoneLabel()}) — this VPN became{' '}
                {stoppedBecause}. The flat line after that point is an absence of readings, not
                zero usage: traffic that happened after it was never recorded here.
              </p>
            )}
            {!hasPoints ? (
              <div className="vui-empty" data-testid="vui-trend-empty">
                <Activity size={16} aria-hidden="true" />
                <span>
                  {usage?.history === 'NONE'
                    ? 'No history yet — the first server sync starts it.'
                    : 'Nothing was recorded in this window.'}
                </span>
              </div>
            ) : (
              <>
                <LineChart
                  series={series}
                  labels={trendLabels}
                  style="area"
                  yAxis
                  zeroBased
                  valueFormat={fmt}
                  ariaLabel={`Download and upload traffic over the last ${windowLabel}`}
                />
                <p className="vui-muted vui-note">
                  Each point is what moved between two server readings, so this shows when
                  traffic happened rather than the running total.
                </p>
              </>
            )}
          </Card>

          {/* ---------- C: when you use it ---------- */}
          <Card
            title="When you use it"
            actions={<span className="vui-muted t-xs">{DAY_PART_HINT}</span>}
          >
            {hourlyError ? (
              <p className="vui-muted">{hourlyError}</p>
            ) : hourly === null ? (
              <p className="vui-muted">Loading…</p>
            ) : hourlyTotal <= 0 ? (
              <div className="vui-empty" data-testid="vui-daypart-empty">
                <Activity size={16} aria-hidden="true" />
                <span>No traffic was recorded in this window.</span>
              </div>
            ) : (
              <>
                <BarChart
                  data={dayPartData}
                  showValues
                  yAxis
                  valueFormat={fmt}
                  ariaLabel={`Traffic by part of day over the last ${windowLabel}`}
                />
                <p className="vui-muted vui-note">
                  Built from {hourly.length} hourly reading{hourly.length === 1 ? '' : 's'} the
                  server recorded in this window.
                </p>
              </>
            )}
          </Card>
          {/* ---------- C2: the busiest hour, as a sentence ---------- */}
          <Card
            title="Your busiest hour"
            actions={<span className="vui-muted t-xs">From hourly readings in this window</span>}
          >
            {hourlyError ? (
              <p className="vui-muted">{hourlyError}</p>
            ) : hourly === null ? (
              <p className="vui-muted">Loading…</p>
            ) : peak === null ? (
              <div className="vui-empty" data-testid="vui-peak-empty">
                <Clock size={16} aria-hidden="true" />
                <span>No traffic was recorded in this window, so there is no busiest hour.</span>
              </div>
            ) : (
              <div className="vui-figure" data-testid="vui-peak">
                <strong>
                  {String(peak.hour).padStart(2, '0')}:00–{String((peak.hour + 1) % 24).padStart(2, '0')}:00
                </strong>
                <span className="vui-muted">
                  is your heaviest hour, carrying {fmt(peak.bytes)}. Times are UTC, matching the
                  server&apos;s own bucketing.
                </span>
              </div>
            )}
          </Card>

          {/* ---------- C3: the 7 x 24 heatmap ---------- */}
          <Card
            title="When in the week you use it"
            actions={<span className="vui-muted t-xs">7 days × 24 hours · UTC</span>}
          >
            {hourlyError ? (
              <p className="vui-muted">{hourlyError}</p>
            ) : hourly === null ? (
              <p className="vui-muted">Loading…</p>
            ) : heatmap.total <= 0 ? (
              <div className="vui-empty" data-testid="vui-heat-empty">
                <Activity size={16} aria-hidden="true" />
                <span>Nothing was recorded in this window, so there is no pattern to show.</span>
              </div>
            ) : (
              <>
                <div className="vui-heat" data-testid="vui-heat">
                  <span />
                  {Array.from({ length: 24 }, (_, h) => (
                    <span className="vui-heat__hour" key={`hh-${h}`}>
                      {h % 3 === 0 ? String(h).padStart(2, '0') : ''}
                    </span>
                  ))}
                  {heatmap.grid.map((row, dow) => (
                    <Fragment key={`dow-${dow}`}>
                      <span className="vui-heat__day">{WEEKDAYS[dow]}</span>
                      {row.map((v, h) => {
                        /* Level 0 is "nothing measured here", drawn as an empty cell
                           rather than the lightest traffic shade. */
                        const level = v <= 0 ? 0 : Math.min(4, Math.max(1, Math.ceil((v / heatmap.max) * 4)))
                        return (
                          <span
                            className="vui-heat__cell"
                            key={`c-${dow}-${h}`}
                            data-level={level}
                            title={`${WEEKDAYS[dow]} ${String(h).padStart(2, '0')}:00 — ${v > 0 ? fmt(v) : 'no measurement'}`}
                          />
                        )
                      })}
                    </Fragment>
                  ))}
                </div>
                <div className="vui-heat__legend">
                  <span>Less</span>
                  {[1, 2, 3, 4].map((l) => (
                    <span className="vui-heat__swatch vui-heat__cell" data-level={l} key={`lg-${l}`} />
                  ))}
                  <span>More</span>
                  <span className="vui-muted">· empty cells had no reading</span>
                </div>
              </>
            )}
          </Card>

          {/* ---------- C4: daily bars ---------- */}
          <Card
            title="Day by day"
            actions={<span className="vui-muted t-xs">Last {dailyBars.length} day(s) with readings</span>}
          >
            {dailyBars.length === 0 ? (
              <div className="vui-empty" data-testid="vui-daily-empty">
                <CalendarRange size={16} aria-hidden="true" />
                <span>Not enough snapshots yet to draw days.</span>
              </div>
            ) : (
              <BarChart
                data={dailyBars}
                yAxis
                valueFormat={fmt}
                ariaLabel={`Traffic per day over the last ${windowLabel}`}
              />
            )}
          </Card>

          {/* ---------- C5: the exhaustion estimate ---------- */}
          <Card
            title="When your data runs out"
            actions={<span className="vui-muted t-xs">An estimate, not a guarantee</span>}
          >
            {projection === null ? (
              <div className="vui-empty" data-testid="vui-projection-empty">
                <Gauge size={16} aria-hidden="true" />
                <span>
                  Not enough movement in this window to estimate a date. Use more data and this
                  fills in.
                </span>
              </div>
            ) : projection.exhausted ? (
              <div className="vui-figure" data-testid="vui-projection">
                <strong>Already used up</strong>
                <span className="vui-muted">
                  Your allowance on this VPN is spent. Renew the package to keep using it.
                </span>
              </div>
            ) : (
              <>
                <div className="vui-figure" data-testid="vui-projection">
                  <strong>
                    ~{projection.days} day{projection.days === 1 ? '' : 's'}
                  </strong>
                  <span className="vui-muted">
                    of data left at your current rate of {fmt(projection.perDay)} a day.
                  </span>
                </div>
                <p className="vui-muted vui-note vui-estimate">
                  This is an <strong>estimate, not a guarantee</strong>: it repeats what you used
                  over the last {windowLabel} and divides the remainder by it. One heavy evening
                  changes the answer.
                </p>
              </>
            )}
          </Card>



          {/* ---------- D: connection performance ---------- */}
          <Card
            title="Connection performance"
            actions={
              <span className="vui-muted t-xs">
                Averaged between server readings — not a speed test
              </span>
            }
          >
            {speedError ? (
              <p className="vui-muted">{speedError}</p>
            ) : speed === null ? (
              <p className="vui-muted">Loading…</p>
            ) : measured.length === 0 ? (
              <div className="vui-empty" data-testid="vui-speed-empty">
                <Gauge size={16} aria-hidden="true" />
                <span>
                  No measured speed yet — this needs two server readings with traffic between
                  them.
                </span>
              </div>
            ) : (
              <>
                <div className="vui-speedfigs">
                  <span>
                    <strong>{formatRate(lastSpeed?.down_bps ?? 0)}</strong>
                    <span className="vui-muted">download, latest</span>
                  </span>
                  <span>
                    <strong>{formatRate(lastSpeed?.up_bps ?? 0)}</strong>
                    <span className="vui-muted">upload, latest</span>
                  </span>
                  <span>
                    <strong>{measured.length}</strong>
                    <span className="vui-muted">measured interval(s)</span>
                  </span>
                </div>
                {/* bps is a RATE, so it is formatted as one — a bytes formatter
                    here would label 8 Mbps as "1 GB", which is a different fact. */}
                <LineChart
                  series={speedSeries}
                  labels={speedLabels}
                  style="smooth"
                  yAxis
                  zeroBased
                  valueFormat={formatRate}
                  ariaLabel={`Download and upload speed over the last ${windowLabel}`}
                />
                <p className="vui-muted vui-note">
                  A rate is the traffic counter&apos;s change divided by the time between two
                  readings. An interval the server could not measure is left out rather than
                  drawn as zero.
                </p>
              </>
            )}
          </Card>
        </>
      )}
    </div>
  )
}
