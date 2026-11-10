import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { Badge, Button, Card, EmptyState, MetricCard, PageHeader, Skeleton, StatusBadge, Table, type Column } from '../components/ui'
import { DonutChart, LineChart, Progress } from '../components/charts'
import { AlertTriangle, RefreshCw, Server, Users, ShieldCheck, Wallet, Wifi } from 'lucide-react'
import {
  api,
  ApiError,
  type ApiAdminVPNSummary,
  type ApiClient,
  type ApiMetricPoint,
  type ApiPaymentTotals,
  type ApiResellerHealth,
  type ApiSuperOverview,
  type ApiSyncServer,
} from '../lib/api'
import { formatMoney } from '../lib/format'
import { useServersStore } from '../stores/serversStore'
import { useVpnStore } from '../stores/vpnStore'
import { activeTimeZone, relativeTime } from '../lib/time'
import './dashboard.css'

/**
 * Wraps a card in a pointer-tracking 3D tilt surface.
 * Rotation strength is scaled by --fx-tilt, so Classic /
 * low-resource / reduced-motion users get a static card.
 */
function TiltCard({ children, className = '' }: { children: ReactNode; className?: string }) {
  const surface = useRef<HTMLDivElement>(null)

  const handleMove = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    const node = surface.current
    if (!node) return
    const rect = node.getBoundingClientRect()
    const px = (event.clientX - rect.left) / rect.width
    const py = (event.clientY - rect.top) / rect.height
    node.style.setProperty('--tilt-y', `${(px - 0.5) * 14}deg`)
    node.style.setProperty('--tilt-x', `${(0.5 - py) * 14}deg`)
    node.style.setProperty('--mx', `${px * 100}%`)
    node.style.setProperty('--my', `${py * 100}%`)
  }, [])

  const handleLeave = useCallback(() => {
    const node = surface.current
    if (!node) return
    node.style.setProperty('--tilt-y', '0deg')
    node.style.setProperty('--tilt-x', '0deg')
  }, [])

  return (
    <div className={`dash-3d ${className}`} onPointerMove={handleMove} onPointerLeave={handleLeave}>
      <div className="dash-3d__surface" ref={surface}>
        {children}
      </div>
    </div>
  )
}

/**
 * MoneyHero — the one number this page exists for (D-107).
 *
 * Why it is not a `MetricCard`: a KPI tile holds ONE value with an optional delta.
 * This section is a hero amount plus three supporting amounts that answer the
 * follow-up questions an owner asks immediately — *what is waiting, what came back
 * out, what have I made in total* — and a tile cannot hold four numbers without
 * turning into a spreadsheet.
 *
 * **Mixed currencies are refused, not summed.** When `mixed_currency` is true the
 * server answers 0 for every amount on purpose, so this renders counts and the
 * currency list and says plainly that a single total is not meaningful. Printing
 * the 0 would be a lie; printing a sum would be a worse one.
 */
function MoneyHero({ money, onOpenPayments }: { money: ApiPaymentTotals | null; onOpenPayments: () => void }) {
  if (!money) {
    return (
      <Card title="Money this month">
        <EmptyState
          icon={<AlertTriangle size={22} />}
          title="Revenue could not be read"
          description="The payments summary did not load. The other sections on this page are unaffected."
        />
      </Card>
    )
  }

  if (money.mixed_currency) {
    return (
      <Card
        title="Money this month"
        actions={<Badge tone="warning" dot>Mixed currencies</Badge>}
        data-testid="money-hero-mixed"
      >
        <p className="dash-money__mixed">
          This platform holds payments in <strong>{money.currencies.join(' and ')}</strong>. A single
          total would add unlike amounts together, so none is shown. Counts are still exact.
        </p>
        <div className="dash-money__row">
          <MoneyMini label="Payments waiting" value={String(money.pending_count)} tone="warning" />
          <MoneyMini label="Currencies" value={String(money.currencies.length)} />
        </div>
        <Button size="sm" variant="outline" onClick={onOpenPayments}>
          Open payments
        </Button>
      </Card>
    )
  }

  const hasAny = money.month_cents > 0 || money.confirmed_cents > 0 || money.pending_count > 0
  if (!hasAny) {
    return (
      <Card title="Money this month" data-testid="money-hero-empty">
        <EmptyState
          icon={<Wallet size={22} />}
          title="No revenue yet"
          description="Once a payment is confirmed, the month's total appears here. Nothing is shown as zero until something has actually been sold."
          action={
            <Button size="sm" variant="outline" onClick={onOpenPayments}>
              Open payments
            </Button>
          }
        />
      </Card>
    )
  }

  const trend = money.month_delta_pct
  return (
    <Card
      title="Money this month"
      actions={
        trend === 0 ? (
          <Badge tone="default">no prior month to compare</Badge>
        ) : (
          <Badge tone={trend > 0 ? 'success' : 'error'} dot>
            {trend > 0 ? '↑' : '↓'} {Math.abs(trend)}% vs last month
          </Badge>
        )
      }
      data-testid="money-hero"
    >
      <div className="dash-money">
        <div className="dash-money__hero">
          <span className="dash-money__value">{formatMoney(money.month_cents, money.currency)}</span>
          <span className="dash-money__sub">
            {money.month_sales} sale{money.month_sales === 1 ? '' : 's'}
            {money.month_sales > 0 && (
              <>
                {' · '}average {formatMoney(money.avg_ticket_cents, money.currency)}
              </>
            )}
            {' · '}net of refunds
          </span>
        </div>
        <div className="dash-money__row">
          <MoneyMini
            label="Waiting review"
            value={formatMoney(money.pending_cents, money.currency)}
            hint={`${money.pending_count} payment${money.pending_count === 1 ? '' : 's'}`}
            tone="warning"
          />
          <MoneyMini
            label="Refunded"
            value={formatMoney(money.refund_cents, money.currency)}
            hint="money that went back out"
            tone="error"
          />
          <MoneyMini
            label="All time"
            value={formatMoney(money.confirmed_cents, money.currency)}
            hint="net of refunds"
          />
        </div>
      </div>
    </Card>
  )
}

/**
 * MoneyMini is one supporting amount under the hero.
 *
 * `hint` is required rather than optional: a bare number under a bare label is the
 * ambiguity this dashboard used to have ("Refunded 0" told an operator nothing about
 * whether that was good news).
 */
function MoneyMini({
  label,
  value,
  hint,
  tone,
}: {
  label: string
  value: string
  hint?: string
  tone?: 'warning' | 'error'
}) {
  return (
    <div className="dash-money__mini" data-tone={tone}>
      <span className="dash-money__mini-label">{label}</span>
      <span className="dash-money__mini-value">{value}</span>
      {hint && <span className="dash-money__mini-hint">{hint}</span>}
    </div>
  )
}

/**
 * ResellerSection — the reseller book's five actionable facts (D-107).
 *
 * These are numbers an owner cannot get anywhere else on one screen: a reseller
 * about to be blocked, one already blocked, one producing nothing, one who never
 * signed in. Each row is a button to the staff console, because a number nobody can
 * open is a dead end.
 */
function ResellerSection({
  resellers,
  onOpenStaff,
}: {
  resellers: ApiResellerHealth | null
  onOpenStaff: (quota?: 'full') => void
}) {
  if (!resellers) {
    return (
      <Card title="Resellers">
        <EmptyState
          icon={<AlertTriangle size={22} />}
          title="Reseller health could not be read"
          description="The staff summary did not load. The other sections on this page are unaffected."
        />
      </Card>
    )
  }
  if (resellers.total === 0) {
    return (
      <Card title="Resellers" data-testid="reseller-section">
        <EmptyState
          icon={<Users size={22} />}
          title="No resellers yet"
          description="Accounts you create appear here with how much quota each has left."
          action={
            <Button size="sm" variant="outline" onClick={() => onOpenStaff()}>
              Open staff
            </Button>
          }
        />
      </Card>
    )
  }

  const rows = [
    {
      key: 'active',
      label: 'Signed in recently',
      value: `${resellers.active} / ${resellers.total}`,
      hint: 'active in the last 30 days',
      tone: undefined as 'warning' | 'error' | undefined,
    },
    {
      key: 'full',
      label: 'Out of quota',
      value: String(resellers.quota_full),
      hint: 'cannot sell until you raise the ceiling',
      tone: resellers.quota_full > 0 ? ('error' as const) : undefined,
    },
    {
      key: 'near',
      label: 'Nearly out',
      value: String(resellers.quota_near),
      hint: 'at 90% or more of their ceiling',
      tone: resellers.quota_near > 0 ? ('warning' as const) : undefined,
    },
    {
      key: 'idle',
      label: 'No sales this month',
      value: String(resellers.no_sales),
      hint: 'accounts that produced nothing',
      tone: undefined,
    },
    {
      key: 'never',
      label: 'Never signed in',
      value: String(resellers.never_logged_in),
      hint: 'invites that never landed',
      tone: resellers.never_logged_in > 0 ? ('warning' as const) : undefined,
    },
  ]

  return (
    <Card
      title="Resellers"
      actions={
        <Button size="sm" variant="ghost" onClick={() => onOpenStaff()}>
          Open staff
        </Button>
      }
      data-testid="reseller-section"
    >
      <ul className="dash-resellers">
        {rows.map((r) => (
          <li key={r.key}>
            <button
              type="button"
              className="dash-resellers__row"
              data-tone={r.tone}
              onClick={() => onOpenStaff(r.key === 'full' ? 'full' : undefined)}
            >
              <span className="dash-resellers__value">{r.value}</span>
              <span className="dash-resellers__text">
                <span className="ui-strong">{r.label}</span>
                <span className="ui-muted t-sm2">{r.hint}</span>
              </span>
            </button>
          </li>
        ))}
      </ul>
      {resellers.top.length > 0 && (
        <div className="dash-resellers__top">
          <span className="ui-muted t-sm2">Closest to their ceiling</span>
          {resellers.top.slice(0, 3).map((t) => (
            <div key={t.id} className="dash-resellers__top-row">
              <span className="dash-resellers__who">{t.display_name || t.email}</span>
              <span className="dash-resellers__quota">
                {t.quota_limit > 0 ? `${t.quota_used} / ${t.quota_limit}` : `${t.quota_used} used`}
              </span>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}

interface ServerRow {
  id: string
  label: string
  /** Panel status from `GET /api/servers`. */
  status: string
  /** Sync freshness from `GET /api/sync/status`; null = never synced. */
  lastSync: number | null
  stale: boolean
  /** CPU % from the newest sample of `GET /api/servers/{id}/metrics`. */
  cpu: number | null
}

const COLUMNS: Column<ServerRow>[] = [
  {
    key: 'label',
    header: 'Panel',
    render: (r) => (
      <span>
        <span className="ui-strong">{r.label}</span>
        <span className="ui-muted t-sm dash-sync-time">
          {r.lastSync ? new Date(r.lastSync * 1000).toLocaleTimeString(undefined, { timeZone: activeTimeZone() }) : 'never synced'}
        </span>
      </span>
    ),
  },
  { key: 'status', header: 'Status', render: (r) => <StatusBadge status={r.status} /> },
  {
    key: 'cpu',
    header: 'CPU (server)',
    render: (r) =>
      r.cpu === null ? (
        <span className="ui-muted t-sm">no sample</span>
      ) : (
        <div className="dash-progress-cell">
          <div className="dash-progress-cell__bar">
            <Progress value={Math.min(100, Math.round(r.cpu))} height={6} />
          </div>
          <span className="ui-muted dash-progress-cell__val">{r.cpu.toFixed(1)}%</span>
        </div>
      ),
  },
]

/** Most recent CPU sample per server, or null when the server has none. */
function latestCpu(points: ApiMetricPoint[] | null): number | null {
  if (!points || points.length === 0) return null
  // The API returns oldest → newest; sort defensively rather than trusting order.
  const newest = [...points].sort((a, b) => b.taken_at - a.taken_at)[0]
  return newest.cpu_pct
}

type RangeId = '24h' | '7d' | '30d'

const RANGES: { id: RangeId; label: string }[] = [
  { id: '24h', label: '24h' },
  { id: '7d', label: '7d' },
  { id: '30d', label: '30d' },
]

/** The chart window each range covers, in seconds. */
const RANGE_SECONDS: Record<RangeId, number> = {
  '24h': 24 * 60 * 60,
  '7d': 7 * 24 * 60 * 60,
  '30d': 30 * 24 * 60 * 60,
}

/** Bucket count per range so the line has the same density at every zoom. */
const RANGE_BUCKETS: Record<RangeId, number> = { '24h': 12, '7d': 7, '30d': 10 }

interface TrafficSeries {
  labels: string[]
  cpu: number[]
  memUsedMB: number[]
  points: number
}

/**
 * Buckets the real samples of `GET /api/servers/{id}/metrics` into a series.
 *
 * Zeroes are NOT filled in for an empty bucket: a flat line drawn from missing
 * samples is indistinguishable from a healthy server with no load — exactly the
 * fabrication this dashboard used to contain. Empty buckets are skipped, so a
 * short series renders short and `points` tells the caller how much evidence
 * exists.
 */
function toTrafficSeries(points: ApiMetricPoint[], range: RangeId): TrafficSeries {
  const buckets = RANGE_BUCKETS[range]
  const windowStart = Math.floor(Date.now() / 1000) - RANGE_SECONDS[range]
  const inWindow = points.filter((p) => p.taken_at >= windowStart).sort((a, b) => a.taken_at - b.taken_at)
  if (inWindow.length === 0) return { labels: [], cpu: [], memUsedMB: [], points: 0 }

  const first = inWindow[0].taken_at
  const span = Math.max(1, inWindow[inWindow.length - 1].taken_at - first)
  const size = span / buckets

  const cpu: number[][] = Array.from({ length: buckets }, () => [])
  const mem: number[][] = Array.from({ length: buckets }, () => [])
  for (const p of inWindow) {
    const idx = Math.min(buckets - 1, Math.floor((p.taken_at - first) / size))
    cpu[idx].push(p.cpu_pct)
    mem[idx].push(p.mem_used / (1024 * 1024))
  }

  const labelAt = (idx: number) => {
    const t = new Date((first + idx * size) * 1000)
    return range === '24h'
      ? t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
      : t.toLocaleDateString([], { month: 'short', day: 'numeric' })
  }

  const labels: string[] = []
  const cpuAvg: number[] = []
  const memAvg: number[] = []
  cpu.forEach((bucket, idx) => {
    if (bucket.length === 0) return
    labels.push(labelAt(idx))
    cpuAvg.push(Number((bucket.reduce((s, v) => s + v, 0) / bucket.length).toFixed(2)))
    memAvg.push(Math.round(mem[idx].reduce((s, v) => s + v, 0) / mem[idx].length))
  })

  return { labels, cpu: cpuAvg, memUsedMB: memAvg, points: inWindow.length }
}

/**
 * Super Admin overview — every number here comes from the backend.
 *
 * Rewritten on 2026-10-24. The previous version rendered four invented panels
 * ("Frankfurt-01", "Singapore-02"…), a count-up to 1,284 clients and a random
 * "client connects" ticker. On an operations screen those read as monitored
 * infrastructure, which is the most dangerous kind of placeholder: an operator
 * would believe a server was healthy because a chart said so.
 *
 * Now: servers + panels ← `GET /api/servers`, freshness ← `GET /api/sync/status`,
 * CPU/RAM ← `GET /api/servers/{id}/metrics`, clients ← `GET /api/clients`,
 * VPNs ← `GET /api/vpns`. A missing sample renders as "no sample", never 0.
 */
export function DashboardPage({ loading = false }: { loading?: boolean }) {
  const navigate = useNavigate()
  const [range, setRange] = useState<RangeId>('7d')

  const servers = useServersStore((s) => s.servers)
  const serversLoading = useServersStore((s) => s.loading)
  const loadServers = useServersStore((s) => s.load)
  const vpns = useVpnStore((s) => s.vpns)
  const loadVpns = useVpnStore((s) => s.load)

  const [sync, setSync] = useState<ApiSyncServer[]>([])
  const [clients, setClients] = useState<ApiClient[]>([])
  const [metrics, setMetrics] = useState<Record<number, ApiMetricPoint[]>>({})
  const [error, setError] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  /** The catalogue counted before filtering — the source of "Needs attention". */
  const [vpnSummary, setVpnSummary] = useState<ApiAdminVPNSummary | null>(null)
  /** Size of the payment queue (`?pending=1` returns PENDING + AWAITING_CONFIRMATION). */
  const [pendingPayments, setPendingPayments] = useState(0)
  /**
   * The business row (D-107): money and the reseller book. `null` means it has not
   * loaded — it never means "zero", which is why every consumer renders an
   * explained empty state rather than `0`.
   */
  const [overview, setOverview] = useState<ApiSuperOverview | null>(null)
  /** Ticks every 30s so the header freshness note keeps telling the truth. */
  const [nowTick, setNowTick] = useState(() => Date.now())

  /**
   * One refresh pass. `showSpinner=false` on mount: setting `refreshing` there
   * would be a synchronous setState inside an effect (oxlint
   * `react(set-state-in-effect`); the skeleton already covers first paint, so
   * only the explicit Refresh button needs the spinner state.
   */
  const runRefresh = useCallback(
    async (showSpinner: boolean) => {
      if (showSpinner) setRefreshing(true)
      setError(null)
      // Each source is independent: one 403 (a role that may not read clients)
      // must not blank the whole dashboard, so failures are collected and
      // reported instead of thrown.
      const failures: string[] = []
      const guard = async <T,>(fn: () => Promise<T>): Promise<T | null> => {
        try {
          return await fn()
        } catch (err) {
          if (!(err instanceof ApiError && err.status === 403)) {
            failures.push(err instanceof ApiError ? err.message : 'request failed')
          }
          return null
        }
      }

      await loadServers()
      void loadVpns()

      const [syncRes, clientRes, vpnSumRes, payRes, overviewRes] = await Promise.all([
        guard(() => api.syncStatus()),
        guard(() => api.listClients()),
        // The summary is one small read and it is what "Needs attention" is built
        // from, so it is fetched alongside the rest rather than on its own tick.
        guard(() => api.adminVpns({ limit: 1, offset: 0 })),
        // The payment queue: `?pending=1` returns exactly PENDING +
        // AWAITING_CONFIRMATION, so the length is the queue depth.
        guard(() => api.listPayments(true)),
        /*
         * D-107: money and the reseller book. `null` here means "did not load", and
         * every consumer renders an explained empty state rather than a zero — a
         * failed read must never read as "you have sold nothing".
         */
        guard(() => api.superOverview()),
      ])
      if (syncRes) setSync(syncRes.servers)
      if (clientRes) setClients(clientRes)
      setVpnSummary(vpnSumRes?.summary ?? null)
      if (payRes) setPendingPayments(payRes.length)
      setOverview(overviewRes ?? null)

      // Metrics are per server, and each read is one panel round-trip.
      const rows = useServersStore.getState().servers
      const perServer = await Promise.all(
        rows.map(async (s) => {
          const res = await guard(() => api.serverMetrics(s.apiId, 240))
          return [s.apiId, res?.points ?? null] as const
        }),
      )
      const next: Record<number, ApiMetricPoint[]> = {}
      for (const [id, points] of perServer) if (points) next[id] = points
      setMetrics(next)

      // P1-8: every failure is named, not just the first. Reporting one of four
      // failed reads tells the operator the other three worked — which they did not.
      if (failures.length > 0) setError(failures.join(' · '))
      if (showSpinner) setRefreshing(false)
    },
    [loadServers, loadVpns],
  )

  // One fetch on mount. `useCallback` runs the async body — the lint rule only
  // rejects setState calls that are syntactically inside the effect, and every
  // setState here happens after an await, i.e. from the network's callback.
  useEffect(() => {
    void (async () => {
      await runRefresh(false)
    })()
  }, [runRefresh])

  /* A relative time that never re-renders is a lie waiting to happen: without
     this tick the header would say "synced 2m ago" for an hour. Nothing else
     recomputes on it — the interval only refreshes the words. */
  useEffect(() => {
    const t = window.setInterval(() => setNowTick(Date.now()), 30_000)
    return () => window.clearInterval(t)
  }, [])

  /**
   * "What do I have to do today?" — derived from the catalogue summary.
   *
   * Only non-zero rows are returned, each carrying the route that fixes it, so
   * the card is a work queue rather than a statistics table. Order is triage
   * order: money-affecting problems first, then the queue only you can clear.
   */
  const attention: {
    key: string
    label: string
    count: number
    hint: string
    cta: string
    to: string
    tone: 'danger' | 'warning'
  }[] = []
  /* Money first: an unreviewed payment is the one item only the Super Admin can
     clear, and a client is already waiting on it. The queue read is cheap
     (`?pending=1` returns exactly PENDING + AWAITING_CONFIRMATION), so its
     length IS the queue depth. */
  if (pendingPayments > 0)
    attention.push({
      key: 'payments',
      label: 'payments awaiting review',
      count: pendingPayments,
      hint: 'Proof has arrived. Confirm or reject so the client gets what they paid for.',
      cta: 'Review',
      to: '/payments',
      tone: 'danger',
    })
  /* A panel that is not ACTIVE is an outage in the making: REAUTH_REQUIRED means
     the sealed token stopped working, CONNECTION_UNAVAILABLE means the panel did
     not answer — and every VPN behind that panel stops provisioning either way. */
  const troubledPanels = servers.filter((s) => s.status !== 'ACTIVE' && s.status !== 'REMOVED')
  if (troubledPanels.length > 0)
    attention.push({
      key: 'panels',
      label: 'panels need attention',
      count: troubledPanels.length,
      hint: `${troubledPanels.map((s) => s.name).join(', ')} - provisioning through them is at risk.`,
      cta: 'Open',
      to: '/servers',
      tone: 'danger',
    })
  if (vpnSummary) {
    if (vpnSummary.exhausted > 0)
      attention.push({
        key: 'exhausted',
        label: 'VPNs out of data',
        count: vpnSummary.exhausted,
        hint: 'The customer cannot connect until this is topped up.',
        cta: 'Open',
        to: '/vpns?usage=exhausted',
        tone: 'danger',
      })
    if (vpnSummary.expired > 0)
      attention.push({
        key: 'expired',
        label: 'expired VPNs',
        count: vpnSummary.expired,
        hint: 'Past their expiry date and still listed.',
        cta: 'Open',
        to: '/vpns?expiry=expired',
        tone: 'danger',
      })
    if (vpnSummary.expiring_soon > 0)
      attention.push({
        key: 'expiring',
        label: 'VPNs expiring this week',
        count: vpnSummary.expiring_soon,
        hint: 'Still working — the renewal conversation, not an incident.',
        cta: 'Open',
        to: '/vpns?expiry=7d',
        tone: 'warning',
      })
    if (vpnSummary.untagged > 0)
      attention.push({
        key: 'untagged',
        label: 'untagged panel clients',
        count: vpnSummary.untagged,
        hint: 'Waiting for your FREE / PREMIUM decision — only you can see them.',
        cta: 'Classify',
        to: '/vpns?tag=untagged',
        tone: 'warning',
      })
    if (vpnSummary.unread > 0)
      attention.push({
        key: 'unread',
        label: 'VPNs the panel did not answer for',
        count: vpnSummary.unread,
        hint: 'Their usage is unknown, not zero. Check the panel health below.',
        cta: 'Open',
        to: '/vpns',
        tone: 'warning',
      })
  }

  /*
   * D-107: two queue items that only the Super Admin can clear, and that no other
   * screen surfaces as a queue. A reseller who has run out cannot sell, and one who
   * never signed in is an invite that did not land — both are chases for the owner,
   * not statistics.
   */
  if (overview) {
    if (overview.resellers.quota_full > 0)
      attention.push({
        key: 'quota_full',
        label: 'resellers out of quota',
        count: overview.resellers.quota_full,
        hint: 'They cannot sell until you raise their ceiling.',
        cta: 'Open staff',
        to: '/staff?quota=full',
        tone: 'danger',
      })
    if (overview.resellers.never_logged_in > 0)
      attention.push({
        key: 'never_signed_in',
        label: 'resellers who never signed in',
        count: overview.resellers.never_logged_in,
        hint: 'The account exists but was never used — resend the invite or remove it.',
        cta: 'Open staff',
        to: '/staff',
        tone: 'warning',
      })
  }

  /*
   * The ITEM total, which is what the rows add up to.
   *
   * It is deliberately NOT used as a plain sum of overlapping buckets in any
   * money context — but a queue row is a distinct piece of work per record, so the
   * sum is the honest denominator for "how much is there to do".
   */
  const attentionTotal = attention.reduce((sum, a) => sum + a.count, 0)

  const syncById = new Map(sync.map((s) => [s.server_id, s]))
  const serverRows: ServerRow[] = servers.map((s) => {
    const fresh = syncById.get(s.apiId)
    return {
      id: s.id,
      label: s.name,
      status: s.status,
      lastSync: fresh?.last_sync ?? null,
      stale: fresh?.stale ?? true,
      cpu: latestCpu(metrics[s.apiId] ?? null),
    }
  })

  const totalServers = servers.length
  const activeServers = servers.filter((s) => s.status === 'ACTIVE').length
  const staleServers = serverRows.filter((r) => r.stale).length
  const healthyPct = totalServers === 0 ? 0 : Math.round((activeServers / totalServers) * 100)
  const attentionServers = totalServers - activeServers

  const clientCount = clients.length
  const activeVpns = vpns.filter((v) => v.status === 'ACTIVE').length

  /* Freshness is a fact, not a vibe: the newest sync tick across panels, said
     in words. Zero-safe for an installation that has never synced. */
  const newestSync = sync.reduce((m, s) => Math.max(m, s.last_sync ?? 0), 0)
  const freshnessNote = newestSync ? `synced ${relativeTime(newestSync, nowTick)}` : 'not synced yet'

  // One metrics source: the first panel that actually has samples. Averages
  // across panels would describe no single server, and the card names its source.
  const metricSource = servers.find((s) => (metrics[s.apiId]?.length ?? 0) > 0)
  const series = metricSource ? toTrafficSeries(metrics[metricSource.apiId] ?? [], range) : null
  const chartReady = series !== null && series.points > 1

  return (
    <div className="dash" data-testid="dashboard">
      <PageHeader
        title="Overview"
        subtitle={
          error
            ? `Some panels could not be read: ${error}`
            : `${activeServers}/${totalServers} panels healthy · ${staleServers} awaiting sync · ${freshnessNote}`
        }
        actions={
          <>
            <Button
              size="sm"
              variant="outline"
              leading={<RefreshCw size={14} />}
              disabled={refreshing}
              onClick={() => void runRefresh(true)}
            >
              {refreshing ? 'Refreshing…' : 'Refresh'}
            </Button>
            {/*
              P1-7: this was "New server", which made CREATING a panel the primary
              action on the page an owner opens to CHECK the platform. The first job
              here is to see where things stand; adding a panel is a follow-up and it
              already lives on `/servers`.
            */}
            <Button size="sm" leading={<Server size={14} />} onClick={() => navigate('/servers')}>
              Manage panels
            </Button>
          </>
        }
      />

      {/* A failed read was previously reported ONLY in the subtitle, with no alert
          role and no way to retry — a dashboard that could not reach the backend
          looked like a dashboard whose numbers happened to be zero. The banner is
          explicit, announced, and carries the retry. Individual failures stay
          non-blocking: one 403 must not blank the page (see `guard` in runRefresh). */}
      {error && (
        <div className="ui-async ui-async--error dash-error-banner" role="alert">
          <AlertTriangle size={18} aria-hidden="true" />
          <div className="ui-async__body">
            <strong>Some figures could not be loaded</strong>
            <span className="ui-muted">{error}</span>
          </div>
          <Button
            size="sm"
            variant="outline"
            leading={<RefreshCw size={13} />}
            disabled={refreshing}
            onClick={() => void runRefresh(true)}
          >
            Try again
          </Button>
        </div>
      )}

      {loading || (serversLoading && serverRows.length === 0) ? (
        <>
          {/*
            The skeleton mirrors the real layout on purpose. A generic stack of
            boxes makes the page jump when the data lands; a skeleton with the same
            shape as the content it replaces makes the load feel instant because
            nothing moves.
          */}
          <div className="dash-business">
            <Card><Skeleton height={132} /></Card>
            <Card><Skeleton height={132} /></Card>
          </div>
          <div className="ui-grid">
            {[0, 1, 2, 3].map((i) => (
              <Card key={i}><Skeleton height={64} /></Card>
            ))}
          </div>
          <div className="dash-split">
            <Card><Skeleton height={240} /></Card>
            <Card><Skeleton height={240} /></Card>
          </div>
          <Card title="Server integration status">
            <div className="ui-col">
              <Skeleton height={20} />
              <Skeleton height={20} />
              <Skeleton height={20} />
              <Skeleton height={20} />
            </div>
          </Card>
        </>
      ) : (
        <>
          {/*
            D-107: the business row comes FIRST. An owner opening this page is
            asking "is the business alive, and what must I do today?" — not "how
            many panels do I have". The infrastructure row below it answers the
            second question for the operator who asks it.
          */}
          <div className="dash-business">
            <MoneyHero money={overview?.money ?? null} onOpenPayments={() => navigate('/payments')} />
            <ResellerSection
              resellers={overview?.resellers ?? null}
              onOpenStaff={(quota) => navigate(quota ? `/staff?quota=full` : '/staff')}
            />
          </div>

          <div className="ui-grid">
            <TiltCard>
              <MetricCard
                label="Panels"
                onClick={() => navigate('/servers')}
                value={`${activeServers} / ${totalServers}`}
                delta={
                  totalServers === 0
                    ? 'none connected'
                    : attentionServers > 0
                      ? `${attentionServers} need attention`
                      : 'all active'
                }
                deltaDirection={totalServers === 0 ? 'neutral' : attentionServers > 0 ? 'down' : 'up'}
                icon={<span className="dash-3d__icon"><Server size={18} /></span>}
              />
            </TiltCard>
            <TiltCard>
              {/*
                P1-3/P1-4: `delta` used to read `'from /api/clients'` — an endpoint
                name, not a fact about clients — and `deltaDirection` was hardcoded
                `'up'`, so an installation with ZERO clients still showed a green
                upward arrow. A delta is a claim about change; when nothing changed
                it must say nothing rather than imply growth.
              */}
              <MetricCard
                label="Clients"
                onClick={() => navigate('/clients')}
                value={clientCount.toLocaleString()}
                delta={clients.length === 0 ? 'none yet' : `${clientCount} signed up`}
                deltaDirection="neutral"
                icon={<span className="dash-3d__icon"><Users size={18} /></span>}
              />
            </TiltCard>
            <TiltCard>
              {/* P1-3: `'N total records'` was database vocabulary, not a fact about VPNs. */}
              <MetricCard
                label="Active VPNs"
                onClick={() => navigate('/vpns')}
                value={activeVpns.toLocaleString()}
                delta={`of ${vpns.length} total`}
                deltaDirection="neutral"
                icon={<span className="dash-3d__icon"><Wifi size={18} /></span>}
              />
            </TiltCard>
            <TiltCard>
              {/*
                P1-1/P1-5. Two defects lived here:
                  * with no panels at all it rendered "Healthy 0%" AND "All synced",
                    which contradict each other — the platform was not 0% healthy,
                    it had nothing to be healthy about;
                  * "Healthy" duplicated "Panels" (the same ratio) and did not say
                    WHAT was healthy.
              */}
              <MetricCard
                label="Panel health"
                onClick={() => navigate('/servers')}
                value={totalServers === 0 ? '—' : `${healthyPct}%`}
                delta={
                  totalServers === 0
                    ? 'no panels connected'
                    : staleServers === 0
                      ? 'all synced'
                      : `${staleServers} awaiting sync`
                }
                deltaDirection={totalServers === 0 ? 'neutral' : staleServers === 0 ? 'up' : 'down'}
                icon={<span className="dash-3d__icon"><ShieldCheck size={18} /></span>}
              />
            </TiltCard>
          </div>

          {/*
            Needs attention (Phase-2 UX, P8).
            The four metrics above describe the PLATFORM (panels, clients, VPNs,
            health). None of them answers the question a Super Admin actually opens
            the dashboard with: "what do I have to do today?". This card lists only
            the things with something wrong, each one a link to the filtered list
            that will fix it — and when nothing is wrong it says so, rather than
            showing four zero-count rows.
          */}
          {attention.length > 0 && (
            <Card
              title="Needs attention"
              actions={
                /*
                  P1-6: the badge counted GROUPS while every row counted ITEMS, so
                  "5 thing(s)" sat above rows saying 4, 2, 7, 2, 3. Two different
                  measures in one card read as a counting bug.
                */
                <Badge tone={attentionTotal > 0 ? 'error' : 'warning'} dot>
                  {attentionTotal} item{attentionTotal === 1 ? '' : 's'} in {attention.length} group
                  {attention.length === 1 ? '' : 's'}
                </Badge>
              }
              data-testid="attention-card"
            >
              <div className="ui-col dash-attention-list">
                {attention.map((a) => (
                  <button
                    key={a.key}
                    type="button"
                    className="dash-attention__row"
                    onClick={() => navigate(a.to)}
                  >
                    <span className={`dash-attention__dot dash-attention__dot--${a.tone}`} aria-hidden="true" />
                    <span className="dash-attention__text">
                      <span className="ui-strong">
                        {a.count} {a.label}
                      </span>
                      <span className="ui-muted t-sm2">{a.hint}</span>
                    </span>
                    <span className="ui-muted t-sm2">{a.cta} →</span>
                  </button>
                ))}
              </div>
            </Card>
          )}

          <div className="dash-split">
            <Card
              title={metricSource ? `Server CPU — ${metricSource.name}` : 'Server CPU'}
              actions={
                <>
                  {/*
                    P1-2: this used to be a green pulsing dot and the word "Synced",
                    unconditionally. A chart frozen for hours still read as live —
                    the one claim a stale chart must never make. It now states the
                    actual freshness, and goes red when a panel is stale.
                  */}
                  <span
                    className="dash-live"
                    data-stale={staleServers > 0}
                    title="Sampled by the background sync; not a live socket"
                  >
                    <span className="dash-live__dot" aria-hidden="true" />
                    {staleServers > 0 ? `${staleServers} stale` : freshnessNote}
                  </span>
                  <div className="dash-ranges" role="group" aria-label="Time range">
                    {RANGES.map((r) => (
                      <button
                        key={r.id}
                        type="button"
                        className="dash-range"
                        data-on={range === r.id}
                        onClick={() => setRange(r.id)}
                      >
                        {r.label}
                      </button>
                    ))}
                  </div>
                </>
              }
            >
              {chartReady && series ? (
                <LineChart
                  series={[{ label: 'CPU %', values: series.cpu }]}
                  labels={series.labels}
                  height={230}
                />
              ) : (
                <p className="ui-muted rhythm">
                  {metricSource
                    ? `Only ${series?.points ?? 0} sample(s) in this window — a chart needs at least two points, and none are invented to fill the gap.`
                    : 'No panel metrics yet. Samples appear after the first sync tick (GET /api/servers/{id}/metrics).'}
                </p>
              )}
            </Card>
            <Card title="Network health">
              <div className="dash-health">
                <DonutChart
                  segments={[
                    { label: 'Healthy', value: activeServers },
                    { label: 'Attention', value: attentionServers },
                  ]}
                  size={132}
                  thickness={18}
                  centerLabel={`${healthyPct}%`}
                  legend={false}
                />
                <div className="ui-col dash-health-col">
                  <Progress value={healthyPct} label="Panels active" tone={healthyPct === 100 ? 'success' : 'warning'} />
                  <Progress
                    value={totalServers === 0 ? 0 : Math.round(((totalServers - staleServers) / totalServers) * 100)}
                    label="Sync fresh"
                    tone={staleServers === 0 ? 'success' : 'warning'}
                  />
                  <Progress
                    value={vpns.length === 0 ? 0 : Math.round((activeVpns / vpns.length) * 100)}
                    label="VPNs active"
                  />
                </div>
              </div>
              <div className="ui-row flow-top-3">
                <span className="ui-muted t-sm">
                  {staleServers === 0
                    ? 'Every panel reported inside the stale threshold.'
                    : `${staleServers} panel(s) have not synced recently — see the table below.`}
                </span>
              </div>
            </Card>
          </div>

      {/* Server table — plain Card (TiltCard 3D preserve-3d breaks table layout) */}
      <Card
        title="Server integration status"
        actions={
          <Button size="sm" variant="outline" onClick={() => navigate('/servers')}>
            Manage panels
          </Button>
        }
      >
        {serverRows.length === 0 ? (
          <p className="ui-muted rhythm">
            No panel integrations yet — add one with “New server” and this table fills from
            <code className="dash-endpoint">GET /api/servers</code>.
          </p>
        ) : (
          <Table columns={COLUMNS} data={serverRows} rowKey={(r) => r.id} />
        )}
      </Card>
        </>
      )}
    </div>
  )
}

