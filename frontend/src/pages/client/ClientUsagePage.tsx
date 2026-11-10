import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Activity, ExternalLink, Moon, Search, TrendingUp, X } from 'lucide-react'
import {
  Button,
  EmptyState,
  Select,
  Skeleton,
  formatBytesIn,
  BYTE_UNITS,
  unitLabel,
  type ByteUnit,
} from '../../components/ui'
import { ClientPage, ClientSection } from '../../components/client/ClientPage'
import { useListQuery } from '../../hooks/useListQuery'
import { LineChart, seriesColor } from '../../components/charts/LineChart'
import { VpnKindBadge } from '../../components/vpn/VpnKindBadge'
import { PanelIdentityRow } from '../../components/vpn/PanelIdentityRow'
import {
  useVpnStore,
  vpnDisplayName,
  vpnServerLabel,
  statusNote,
  type ClientVpn,
} from '../../stores/vpnStore'
import { api, type ApiHourlyBucket, type ApiUsagePoint } from '../../lib/api'
import './usage.css'
import '../../components/client/client.css'

/**
 * Usage & History — client (S8).
 *
 * WHAT THIS SCREEN MAY AND MAY NOT SAY
 *
 *   There is still **no account-wide usage endpoint**. `GET /usage`,
 *   `GET /usage/hourly`, `GET /speed-history` and `GET /ping/history` are all
 *   `{vpn_id}`-scoped, so this screen draws one block per owned VPN and never
 *   sums their series into an account total. A total the backend never stated is
 *   exactly the invented number the UI must not produce (D-088.1).
 *
 *   The consequence a customer notices is that there is no single "you used
 *   12 GB this month" figure here. That is stated in the page description rather
 *   than papered over with a sum.
 *
 * WHAT EACH BLOCK SHOWS
 *
 *   - the lifetime total and the package limit, as the server reports them;
 *   - a 24-hour sparkline from the hourly buckets (deltas of stored counters);
 *   - a 30-day sparkline from the usage points.
 *
 *   Every read has its own failure path. A series the server could not measure
 *   renders as an explicit sentence, never as a flat zero line: `0 B` and "not
 *   measured" are different facts.
 */

/** One VPN's read state. `'error'` is kept distinct from "no data". */
type Series =
  | { state: 'loading' }
  | { state: 'error' }
  | {
      state: 'ready'
      total: number
      limit: number
      pct: number
      hourly: number[]
      daily: number[]
      /*
       * The real period the 30-day points cover, in seconds.
       *
       * The chart must NOT claim "Last 30 days" on its own authority: before
       * D-096 the server returned the newest 240 raw rows for every range, so
       * "30 days" was routinely an 18-hour slice and the label was simply
       * false. These values come from the data, and the caption states them.
       */
      dailySpanSec: number
      /** Bucket width the server actually plotted at, in seconds (0 = unknown). */
      bucketSec: number
      /** `hour` | `6h` | `day` | `week` | `month`, or '' when unreported. */
      bucketName: string
    }

function bytesOrDash(n: number | null, unit: ByteUnit): string {
  return n === null ? '\u2014' : formatBytesIn(n, unit)
}

/**
 * A span in words, for a caption that must not overstate its window (D-096).
 *
 * Deliberately coarse: "0.8 days of history" reads as precision the data does
 * not have, so anything under two days is stated in hours.
 */
function spanWords(sec: number): string {
  if (sec <= 0) return 'no span'
  if (sec < 7200) return `${Math.round(sec / 60)} minutes`
  if (sec < 172800) return `${Math.round(sec / 3600)} hours`
  const days = Math.round(sec / 86400)
  return `${days} day${days === 1 ? '' : 's'}`
}

/** What one plotted point covers, in words. */
function bucketWords(name: string, sec: number): string {
  if (name) return name
  if (sec >= 2592000) return 'month'
  if (sec >= 604800) return 'week'
  if (sec >= 86400) return 'day'
  if (sec >= 3600) return `${Math.round(sec / 3600)}h`
  return ''
}
function UsageVpnBlock({
  vpn,
  nowSec,
  unit,
  series,
}: {
  vpn: ClientVpn
  nowSec: number
  unit: ByteUnit
  series: Series
}) {
  const navigate = useNavigate()

  /*
   * A dormant VPN is NOT polled (D-094).
   *
   * Sleeping exists to stop a VPN asking for attention, and two requests per
   * visit on a screen the customer opens to look at their WORKING VPNs is that
   * attention, spent on the server's bandwidth and the customer's data. So the
   * parent skips the fetch entirely rather than making it and ignoring it.
   *
   * The block is still rendered with a sentence and a way back, because a VPN
   * that silently disappeared from this screen would look deleted — and the one
   * promise dormancy makes is that nothing was.
   */
  const asleep = vpn.dormantAt !== null


  const daysLeft = vpn.expiresAt === null ? null : Math.ceil((vpn.expiresAt - nowSec) / 86400)

  return (
    <article className="usg" data-testid={`usage-vpn-${vpn.apiId}`}>
      <header className="usg__head">
        <div className="usg__id">
          <span className="usg__name">{vpnDisplayName(vpn)}</span>
          <VpnKindBadge kind={vpn.kind} />
        </div>
        <Button
          size="sm"
          variant="outline"
          leading={<ExternalLink size={13} />}
          onClick={() => navigate(`/vpn/${vpn.id}`)}
        >
          Open
        </Button>
      </header>

      {/* The panel identity sits beside the package name on every row (D-091.4). */}
      <PanelIdentityRow
        panelEmail={vpn.panelEmail}
        serverLabel={vpnServerLabel(vpn)}
        layout="inline"
        testId={`usage-panel-identity-${vpn.apiId}`}
      />

      {/*
        A non-ACTIVE state is explained, not just coloured. This is the same
        helper the VPN list uses, so the two screens cannot drift apart.
      */}
      {vpn.status !== 'ACTIVE' && <p className="usg__state">{statusNote(vpn.status)}</p>}

      {/* A dormant VPN explains itself instead of showing a skeleton for ever:
          nothing was requested, so nothing is loading. The sentence states the
          two facts that matter — it was not deleted, and waking brings the
          numbers straight back. */}
      {asleep && (
        <p className="usg__note usg__asleep" data-testid={`usage-dormant-${vpn.apiId}`}>
          <Moon size={14} aria-hidden="true" />
          <span>
            This VPN is asleep, so Veylix is not measuring it. Nothing was deleted and its
            history is intact — wake it on My VPNs and the numbers return.
          </span>
        </p>
      )}

      {!asleep && series.state === 'loading' && <Skeleton height={92} />}

      {!asleep && series.state === 'error' && (
        <p className="usg__note usg__note--warn">
          Usage for this VPN could not be read. This is a connection problem, not an empty VPN
          — open it to try again.
        </p>
      )}

      {!asleep && series.state === 'ready' && (
        <>
          <div className="usg__meter">
            <span className="usg__meter-num">
              {bytesOrDash(series.total, unit)}
              {/*
                A limit the server did not state is NOT "0 B". Before this the
                fallback was `?? 0`, so an unreadable allowance rendered
                "1.2 GB of 0 B" — a fact the server never said. `limit_bytes`
                is 0 only when the VPN is genuinely unlimited, and that case
                says "used" (D-096).
              */}
              {series.limit > 0 ? ` of ${formatBytesIn(series.limit, unit)}` : ' used'}
            </span>
            {series.limit > 0 && (
              <span className="usg__track">
                <span
                  className={`usg__fill${series.pct >= 100 ? ' is-full' : series.pct >= 80 ? ' is-high' : ''}`}
                  style={{ width: `${Math.min(100, Math.max(0, series.pct))}%` }}
                />
              </span>
            )}
            {daysLeft !== null && (
              <span className="usg__meta">
                {daysLeft < 0
                  ? `expired ${Math.abs(daysLeft)} day${Math.abs(daysLeft) === 1 ? '' : 's'} ago`
                  : daysLeft === 0
                    ? 'expires today'
                    : `${daysLeft} day${daysLeft === 1 ? '' : 's'} left`}
              </span>
            )}
          </div>

          <div className="usg__charts">
            <figure className="usg__chart">
              <figcaption className="usg__cap">
                Last 24 hours
                <span className="usg__cap-sub">
                  {series.hourly.length} hourly bucket{series.hourly.length === 1 ? '' : 's'}
                </span>
              </figcaption>
              {series.hourly.length > 1 ? (
                <LineChart
                  series={[{ label: 'Traffic', values: series.hourly, color: seriesColor(0) }]}
                  height={120}
                  style="area"
                  labels={['24h ago', 'now']}
                  yAxis
                  zeroBased
                  valueFormat={(n) => formatBytesIn(n, unit)}
                  ariaLabel={`Hourly traffic for ${vpnDisplayName(vpn)} over the last 24 hours`}
                  showLegend={false}
                />
              ) : (
                <p className="usg__empty">Not enough snapshots yet to draw this window.</p>
              )}
            </figure>

            <figure className="usg__chart">
              <figcaption className="usg__cap">
                {/* The heading names the window that was ASKED for; the sub-line
                    states what the data actually covers (D-096). Splitting them
                    this way is the difference between "this is the range you
                    chose" and "this is the range you got", and the second one
                    is the only one the chart may claim. */}
                Last 30 days
                <span className="usg__cap-sub" data-testid="usage-daily-span">
                  {series.daily.length === 0
                    ? 'no readings'
                    : `${series.daily.length} reading${series.daily.length === 1 ? '' : 's'} · ${spanWords(series.dailySpanSec)} of history`}
                  {series.bucketName || series.bucketSec > 0
                    ? ` · ${bucketWords(series.bucketName, series.bucketSec)} per point`
                    : ''}
                </span>
              </figcaption>
              {series.daily.length > 1 ? (
                <LineChart
                  series={[{ label: 'Traffic', values: series.daily, color: seriesColor(1) }]}
                  height={120}
                  style="area"
                  /*
                   * The axis is labelled from the REAL span, never from the range
                   * key. "30d ago" on a one-day chart was the visible symptom of
                   * the bug this fixes, so it is gone rather than kept for looks.
                   */
                  labels={[`${spanWords(series.dailySpanSec)} ago`, 'now']}
                  yAxis
                  zeroBased
                  valueFormat={(n) => formatBytesIn(n, unit)}
                  ariaLabel={`Traffic for ${vpnDisplayName(vpn)} over ${spanWords(series.dailySpanSec)} of recorded history`}
                  showLegend={false}
                />
              ) : series.daily.length === 1 ? (
                /* One bucket is a real reading, but a line needs two. Say that
                   instead of drawing a dot that reads as "no traffic". */
                <p className="usg__empty">
                  One reading so far — there is not enough history yet to draw a trend.
                </p>
              ) : (
                <p className="usg__empty">No usage history recorded yet for this VPN.</p>
              )}
            </figure>
          </div>
        </>
      )}
    </article>
  )
}


export function ClientUsagePage() {
  const navigate = useNavigate()
  const vpns = useVpnStore((s) => s.vpns)
  const loading = useVpnStore((s) => s.loading)
  const load = useVpnStore((s) => s.load)

  /*
   * One clock read per mount so "days left" cannot differ between blocks.
   *
   * A lazy `useState` initializer, not `useMemo`: the value must be read exactly
   * once and never recomputed, and `Date.now()` is impure, so calling it inside a
   * memo trips the React Compiler purity rule. This is the same pattern
   * `ClientDashboardPage` uses (`useNowSeconds`).
   */
  const [nowSec] = useState(() => Date.now() / 1000)

  /*
   * One unit for the whole page, so every block reads in the same scale. Per-block
   * pickers would let two VPN cards state the same traffic two ways, which is the
   * kind of drift this page exists to avoid.
   *
   * It is display state only: changing it re-renders, and issues no request.
   */
  const [unit, setUnit] = useState<ByteUnit>('auto')

  useEffect(() => {
    void load()
  }, [load])

  /*
   * ALL blocks are read together, then rendered together (D-096).
   *
   * The blocks used to fetch their own pair of endpoints, so every card arrived
   * on its own clock: the first VPN painted, then the second, then the third,
   * and the "N of M VPNs" header re-rendered underneath each one. With more VPNs
   * that is the stutter the owner described — records arriving one at a time.
   *
   * One pass over the list, one `setState`, one paint: either the whole page is
   * ready or none of it is. A block that fails still fails on its own, because
   * each entry carries its own state.
   */
  /*
   * Filters first, requests second — and in THAT order (D-097).
   *
   * These used to be the other way round: `awakeKey` was built from every owned
   * VPN, so narrowing the view to one VPN still issued two requests for all of
   * them. A customer with a hundred VPNs paid two hundred requests to look at
   * one chart, and the filter — the control whose entire purpose is to reduce
   * the screen — changed nothing about the cost.
   *
   * Now `shown` is computed from the rows already in the store, and only the
   * rows that will actually be RENDERED are fetched. Filtering is free (no
   * request) and narrowing is a real saving rather than a cosmetic one.
   */
  const query = useListQuery({
    defaults: { source: 'all', q: '' },
    debounced: ['q'],
  })
  const source = query.values.source
  const q = query.values.q

  /* One entry per server, so a customer with several servers can narrow to one.
     Built from the rows already in the store: no extra request. */
  const serverOptions = useMemo(() => {
    const seen = new Map<number, string>()
    for (const v of vpns) {
      const label = vpnServerLabel(v)
      if (!seen.has(v.serverId)) seen.set(v.serverId, label)
    }
    return [...seen.entries()].sort((a, b) => a[1].localeCompare(b[1]))
  }, [vpns])

  /*
   * The rows this screen will render: the source filter AND the search.
   *
   * The haystack is exactly the labels a customer can read on the card
   * (D-091.4) plus the panel email — because the panel email is the VPN's real
   * NAME, not an internal detail. `nimal@gmail.com_paid_2` and
   * `nimal@gmail.com_paid_7` are two different VPNs that share a package, so a
   * search that only looked at the package name could never tell them apart.
   */
  const shown = useMemo(() => {
    let rows = vpns
    if (source.startsWith('server:')) {
      const id = Number(source.slice('server:'.length))
      rows = rows.filter((v) => v.serverId === id)
    } else if (source.startsWith('vpn:')) {
      const id = Number(source.slice('vpn:'.length))
      rows = rows.filter((v) => v.apiId === id)
    }
    const needle = q.trim().toLowerCase()
    if (!needle) return rows
    return rows.filter((v) =>
      `${vpnDisplayName(v)} ${v.panelEmail} ${vpnServerLabel(v)} ${v.offerName ?? ''}`
        .toLowerCase()
        .includes(needle),
    )
  }, [vpns, source, q])

  /*
   * ONLY the rows being rendered are fetched. Dormant VPNs are excluded here
   * rather than fetched and skipped (D-094): a sleeping VPN asks for no
   * attention, and that includes no request.
   */
  const awakeKey = shown
    .filter((v) => v.dormantAt === null && v.apiId > 0)
    .map((v) => v.apiId)
    .join(',')

  const [seriesById, setSeriesById] = useState<Record<number, Series>>({})

  useEffect(() => {
    const ids = awakeKey ? awakeKey.split(',').map(Number) : []
    if (ids.length === 0) return
    let cancelled = false

    void (async () => {
      const entries = await Promise.all(
        ids.map(async (id) => {
          try {
            const [usage, hourly] = await Promise.all([
              api.vpnUsage(id, '30d'),
              api.vpnUsageHourly(id, '24h'),
            ])
            const buckets: ApiHourlyBucket[] = hourly.buckets ?? []
            const points: ApiUsagePoint[] = usage.points ?? []
            // The span the points REALLY cover, from the points themselves.
            // Never assumed from the range that was asked for (D-096).
            const firstAt = points.length > 0 ? points[0].taken_at : 0
            const lastAt = points.length > 0 ? points[points.length - 1].taken_at : 0
            const ready: Series = {
              state: 'ready',
              total: usage.lifetime_total_bytes ?? 0,
              limit: usage.limit_bytes ?? 0,
              pct: usage.usage_pct ?? 0,
              hourly: buckets.map((b) => b.total_bytes),
              daily: points.map((p) => p.total_bytes),
              dailySpanSec: lastAt > firstAt ? lastAt - firstAt : 0,
              bucketSec: usage.window?.bucket_sec ?? usage.bucket_sec ?? 0,
              bucketName: usage.window?.bucket ?? usage.bucket ?? '',
            }
            return [id, ready] as const
          } catch {
            /* A block that could not be read says so. It never falls back to zeros. */
            return [id, { state: 'error' } as Series] as const
          }
        }),
      )
      if (cancelled) return
      const next: Record<number, Series> = {}
      for (const [id, s] of entries) next[id] = s
      setSeriesById(next)
    })()

    return () => {
      cancelled = true
    }
  }, [awakeKey])


  /*
   * Readable label for whatever is filtered, so a pasted link explains itself.
   *
   * The source decides the noun and the search adds the reason, because an
   * empty screen has to say WHY it is empty: "no usage for every VPN" would be
   * a false statement when a search term is what removed the rows.
   */
  const sourceLabel =
    source === 'all'
      ? 'every VPN'
      : source.startsWith('server:')
        ? (serverOptions.find(([id]) => `server:${id}` === source)?.[1] ?? 'this server')
        : (vpns.find((v) => `vpn:${v.apiId}` === source)
            ? vpnDisplayName(vpns.find((v) => `vpn:${v.apiId}` === source)!)
            : 'this VPN')

  /*
   * The source picker offers only the VPNs the SEARCH left, so the two controls
   * cannot contradict each other: selecting a VPN that the current search term
   * excludes would produce an empty screen with no visible cause.
   *
   * When the user picks a single VPN from the list the search is cleared,
   * because "vpn:X plus q=nimal" is a conjunction they never asked for.
   */
  const searchable = useMemo(() => {
    const needle = q.trim().toLowerCase()
    if (!needle) return vpns
    return vpns.filter((v) =>
      `${vpnDisplayName(v)} ${v.panelEmail} ${vpnServerLabel(v)} ${v.offerName ?? ''}`
        .toLowerCase()
        .includes(needle),
    )
  }, [vpns, q])

  const pickSource = (value: string) => {
    // Choosing a specific VPN replaces the search rather than stacking on it.
    query.setNow(value.startsWith('vpn:') ? { source: value, q: '' } : { source: value })
  }

  return (
    <ClientPage
      testId="client-usage"
      title="Usage & History"
      description="What each VPN has used - measured per VPN by the panel, never summed into a figure the server did not state"
      actions={
        <Button size="sm" onClick={() => navigate('/client/vpns')}>
          My VPNs
        </Button>
      }
    >
      <ClientSection
        title="Per-VPN usage"
        actions={
          <div className="usg__controls">
            {/*
              The SEARCH (D-097). Before this the only way to reach one VPN was a
              select listing every VPN by package name — so a customer with thirty
              VPNs scrolled a thirty-row menu, and two VPNs off the same package
              ("Free 5GB Hysteria" ×3) were indistinguishable in it.

              It searches the panel email as well, because that is the VPN's real
              name (D-091.4): `nimal@gmail.com_paid_2` is what identifies it, and
              it is the string an operator will ask for on a support thread.
            */}
            {vpns.length > 1 && source === 'all' && (
              <div className="client-filters__search">
                <label className="ui-sr-only" htmlFor="usg-search">
                  Search your VPNs by name, panel email, server or package
                </label>
                <Search size={14} aria-hidden="true" className="client-filters__icon" />
                <input
                  id="usg-search"
                  className="client-filters__input"
                  type="search"
                  value={query.draft.q}
                  placeholder="Search VPNs…"
                  autoComplete="off"
                  data-testid="usg-search"
                  onChange={(e) => query.set({ q: e.target.value })}
                />
                {query.draft.q ? (
                  <button
                    type="button"
                    className="client-filters__clear"
                    aria-label="Clear search"
                    data-testid="usg-search-clear"
                    onClick={() => query.setNow({ q: '' })}
                  >
                    <X size={13} aria-hidden="true" />
                  </button>
                ) : null}
              </div>
            )}
            {/* The SOURCE picker (D-095). It lists only what the search left, so
                the two controls cannot contradict each other. */}
            <Select
              id="usg-source"
              value={source}
              aria-label="Show usage for which VPN"
              data-testid="usg-source"
              onChange={(e) => pickSource(e.target.value)}
            >
              <option value="all">All VPNs</option>
              {serverOptions.length > 1 &&
                serverOptions.map(([id, label]) => (
                  <option key={`s${id}`} value={`server:${id}`}>
                    Server: {label}
                  </option>
                ))}
              {searchable.map((v) => (
                <option key={`v${v.apiId}`} value={`vpn:${v.apiId}`}>
                  {vpnDisplayName(v)} · {v.panelEmail}
                </option>
              ))}
            </Select>
            <Select
              id="usg-unit"
              value={unit}
              aria-label="Display unit for usage figures"
              data-testid="usg-unit"
              onChange={(e) => setUnit(e.target.value as ByteUnit)}
            >
              {BYTE_UNITS.map((u) => (
                <option key={u} value={u}>
                  {u === 'auto' ? 'Auto units' : unitLabel(u)}
                </option>
              ))}
            </Select>
            {!loading && (
              <span className="ui-muted t-sm" data-testid="usg-count">
                {shown.length} of {vpns.length} VPN{vpns.length === 1 ? '' : 's'}
                {q.trim() ? ' · filtered' : ''}
              </span>
            )}
          </div>
        }
      >
        {vpns.length === 0 && !loading ? (
          <EmptyState
            icon={<Activity size={22} />}
            title="No VPNs to measure yet"
            description="Usage is recorded per VPN, so there is nothing to chart until you own one. Claim a free VPN from your dashboard or buy a premium package."
            action={
              <div className="usg__empty-actions">
                <Button size="sm" variant="primary" onClick={() => navigate('/client')}>
                  Claim a free VPN
                </Button>
                <Button size="sm" variant="outline" onClick={() => navigate('/buy')}>
                  Buy premium
                </Button>
              </div>
            }
          />
        ) : shown.length === 0 ? (
          /* An empty screen must name the CAUSE. "No usage for every VPN" would
             be false when a search term is what removed the rows, and false
             emptiness reads as "my VPN was deleted" (D-095, D-097). */
          <EmptyState
            icon={<Search size={22} />}
            title={
              q.trim()
                ? `No VPN matches “${q.trim()}”`
                : `No usage to show for ${sourceLabel}`
            }
            description={
              q.trim()
                ? 'Your VPNs are still there. Search matches the package name, the panel email, the server and the package it came from - clear the search or widen the filter to see them all.'
                : 'This filter matches no VPN in your account. Nothing was deleted - clear the filter to see every VPN again.'
            }
            action={
              <Button size="sm" variant="outline" onClick={() => query.reset()}>
                Show all VPNs
              </Button>
            }
          />
        ) : (
          <div className="usg__list">
            <p className="usg__intro">
              <TrendingUp size={14} aria-hidden="true" />
              Each block is measured from that VPN&rsquo;s own panel counters. There is no
              account-wide total because the API does not expose one - open a VPN for its full
              history and the window switch.
            </p>
            {shown.map((v) => (
              <UsageVpnBlock
                  key={v.id}
                  vpn={v}
                  nowSec={nowSec}
                  unit={unit}
                  series={v.dormantAt !== null ? { state: 'ready', total: 0, limit: 0, pct: 0, hourly: [], daily: [], dailySpanSec: 0, bucketSec: 0, bucketName: '' } : (seriesById[v.apiId] ?? { state: 'loading' })}
                />
            ))}
          </div>
        )}
      </ClientSection>
    </ClientPage>
  )
}

