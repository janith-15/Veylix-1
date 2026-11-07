import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Activity, ArrowDown, ArrowUp, Cpu, MemoryStick, RefreshCw } from 'lucide-react'
import { Button, Card, MetricCard, Tabs, formatBytes } from '../ui'
import { useAuthStore } from '../../stores/authStore'
import { LineChart, BarChart, DonutChart, type Series } from '../charts'
import {
  api,
  type ApiMetricPoint,
  type ApiUsageRange,
  type ApiVpnLiveTraffic,
  type ApiVpnUsage,
} from '../../lib/api'
import './vpn-insights.css'

/**
 * The Premium VPN feature set the owner mandated (D-038) — P-01 … P-08.
 *
 * Every number here is the SERVER's. The page never derives up/down/total, a
 * percentage, or a rate by itself, because two places doing the same arithmetic
 * is how a chart ends up disagreeing with the API.
 *
 * Three rules from D-026 are enforced rather than decorated:
 *   - an unmeasured value is stated as unmeasured (`unavailable`), never 0;
 *   - every chart names its window and its source;
 *   - an empty window says "no data" instead of drawing a zero line.
 *
 * P-09 (protocol detail) lives in `ProtocolDetail.tsx` and P-10 (QR + link) in
 * `VPNDetailPage.tsx`, because they belong with the connection material.
 */

/** Bytes per second → a rate string like "2.1 kB/s". A rate, never a total (P-04). */
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

/** Day label for an epoch-second sample: 12 Oct. */
function dayLabel(sec: number): string {
  return new Date(sec * 1000).toLocaleDateString(undefined, { day: '2-digit', month: 'short' })
}

/** The four ranges the owner named, plus "all" from the creation day (P-06). */
const RANGES: { value: ApiUsageRange; label: string }[] = [
  { value: '7d', label: '7 days' },
  { value: '30d', label: '30 days' },
  { value: '3m', label: '3 months' },
  { value: '1y', label: '1 year' },
  { value: 'all', label: 'All' },
]

type ChartKind = 'line' | 'area' | 'bar' | 'pie'

const CHART_KINDS: { value: ChartKind; label: string }[] = [
  { value: 'line', label: 'Line' },
  { value: 'area', label: 'Area' },
  { value: 'bar', label: 'Bar' },
  { value: 'pie', label: 'Pie' },
]

/** How often the live rate is re-read. The sync tick is ~15 min, so polling
 *  faster would only re-read the same two snapshots. This is a cheap local
 *  read (D-026.1 forbids a background ping) and it stops on unmount. */
const LIVE_REFRESH_MS = 60_000


export function PremiumInsights({ vpnId, active }: { vpnId: number; active: boolean }) {
  /* ---- P-01 / P-02 / P-06 : usage history, totals, capacity ---- */
  const [range, setRange] = useState<ApiUsageRange>('30d')
  const [usage, setUsage] = useState<ApiVpnUsage | null>(null)
  const [usageBusy, setUsageBusy] = useState(false)
  const [usageError, setUsageError] = useState<string | null>(null)

  /* ---- P-08 : the customer picks the chart type ---- */
  const [chartKind, setChartKind] = useState<ChartKind>('line')

  /* ---- P-03 / P-07 : server CPU and RAM, each switchable ---- */
  const [metricTab, setMetricTab] = useState<'cpu' | 'ram'>('cpu')
  const [metrics, setMetrics] = useState<ApiMetricPoint[] | null>(null)
  const [metricsError, setMetricsError] = useState<string | null>(null)
  const [serverId, setServerId] = useState<number | null>(null)

  /* ---- P-04 : live rate for THIS vpn ---- */
  const [live, setLive] = useState<ApiVpnLiveTraffic | null>(null)

  /* The range switch is a user action: a late answer for the PREVIOUS range
     must not overwrite the current one, so each request carries a token. */
  const usageReq = useRef(0)

  /** Fetches usage for `r` and applies it. Returns nothing: a stale answer for
   *  a previous range is discarded by the token check. */
  const loadUsage = useCallback(
    async (r: ApiUsageRange) => {
      const token = ++usageReq.current
      setUsageBusy(true)
      try {
        const u = await api.vpnUsage(vpnId, r)
        if (usageReq.current !== token) return
        setUsage(u)
        setUsageError(null)
      } catch (err) {
        if (usageReq.current !== token) return
        setUsageError(err instanceof Error ? err.message : 'Usage history could not be read')
      } finally {
        if (usageReq.current === token) setUsageBusy(false)
      }
    },
    [vpnId],
  )

  /* The first read and every range change. Deferred into a microtask so the
     effect does not set state synchronously during the commit phase
     (react/set-state-in-effect), which also keeps the busy flag kosher. */
  useEffect(() => {
    const timer = window.setTimeout(() => void loadUsage(range), 0)
    return () => window.clearTimeout(timer)
  }, [loadUsage, range])

  /* The VPN's server is needed for P-03/P-07. Read it from the caller's own VPN
     list — no extra endpoint, and the list is already role-scoped server-side. */
  useEffect(() => {
    let cancelled = false
    void api
      .listVpns()
      .then((rows) => {
        if (cancelled) return
        const row = rows.find((v) => v.id === vpnId)
        setServerId(row ? row.server_id : null)
      })
      .catch(() => {
        if (!cancelled) setServerId(null)
      })
    return () => {
      cancelled = true
    }
  }, [vpnId])

  /* Server-wide metrics are an OPERATOR read (`PermServerMetrics`: super admin and
   * admin, never a client). A client's own VPN page used to request them anyway,
   * which produced a guaranteed `403` on every visit — a console error that looked
   * like a broken page and told the customer nothing. Asking for a permission the
   * role cannot hold is a bug in the screen, not a refusal by the server. */
  const role = useAuthStore((s) => s.user?.role ?? 'client')
  const canReadServerMetrics = role === 'super-admin' || role === 'admin'

  useEffect(() => {
    if (serverId === null || !canReadServerMetrics) return
    let cancelled = false
    void api
      .serverMetrics(serverId, 240)
      .then((res) => {
        if (cancelled) return
        setMetrics(res.points)
        setMetricsError(null)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setMetrics(null)
        setMetricsError(err instanceof Error ? err.message : 'Server metrics unavailable')
      })
    return () => {
      cancelled = true
    }
  }, [serverId, canReadServerMetrics])

  /* P-04: a local read of the last two sync snapshots. Allowed to fail quietly —
     an unmeasured rate is shown as unmeasured, never as zero. */
  useEffect(() => {
    let cancelled = false
    const read = () => {
      void api
        .vpnLiveTraffic(vpnId)
        .then((l) => {
          if (!cancelled) setLive(l)
        })
        .catch(() => {
          if (!cancelled) setLive(null)
        })
    }
    read()
    const timer = window.setInterval(read, LIVE_REFRESH_MS)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [vpnId])

  /* Memoized so the series memos below hold a stable dependency — an inline
     `?? []` would hand them a fresh array on every render. */
  const points = useMemo(() => usage?.points ?? [], [usage])
  const hasPoints = points.length > 0
  const labels = useMemo(() => points.map((p) => dayLabel(p.taken_at)), [points])

  /* P-01: up, down and TOTAL as three series — the server's numbers, not ours. */
  const usageSeries = useMemo<Series[]>(
    () => [
      { label: 'Download', values: points.map((p) => p.lifetime_down_bytes) },
      { label: 'Upload', values: points.map((p) => p.lifetime_up_bytes) },
      { label: 'Total', values: points.map((p) => p.lifetime_total_bytes) },
    ],
    [points],
  )

  /* The split inside the selected window: a cumulative line and a
     window-total bar answer different questions, so both are offered (P-08). */
  const windowTotals = useMemo<{ label: string; value: number }[]>(() => {
    if (!hasPoints) return []
    const first = points[0]
    const last = points[points.length - 1]
    const up = Math.max(0, last.lifetime_up_bytes - first.lifetime_up_bytes)
    const down = Math.max(0, last.lifetime_down_bytes - first.lifetime_down_bytes)
    return [
      { label: 'Download', value: down },
      { label: 'Upload', value: up },
    ]
  }, [points, hasPoints])

  const rangeLabel = RANGES.find((r) => r.value === range)?.label ?? range
  const usedPct = usage?.usage_pct ?? 0
  const unlimited = usage ? usage.limit_bytes <= 0 : false
  const remaining = usage?.remaining_bytes ?? null

  /* P-03 / P-07: one selected metric at a time, each with its own series. */
  const metricSeries = useMemo<Series[]>(() => {
    const rows = metrics ?? []
    if (metricTab === 'ram') {
      return [{ label: 'RAM used', values: rows.map((m) => m.mem_used) }]
    }
    return [{ label: 'CPU %', values: rows.map((m) => m.cpu_pct) }]
  }, [metrics, metricTab])

  const metricLabels = useMemo(() => (metrics ?? []).map((m) => dayLabel(m.taken_at)), [metrics])

  const latest = metrics && metrics.length > 0 ? metrics[metrics.length - 1] : null

  return (
    <>
      {/* ---------- P-02 + P-04 : capacity and the live rate ---------- */}
      <div className="vpn-insights__top">
        <Card title="Usage against capacity">
          {usageError ? (
            <p className="ui-muted t-sm2" style={{ margin: 0 }}>{usageError}</p>
          ) : !usage ? (
            <p className="ui-muted t-sm2" style={{ margin: 0 }}>Loading usage…</p>
          ) : (
            <div className="ui-col" style={{ gap: 10 }}>
              <div className="vpn-cap" data-testid="vpn-capacity">
                <div className="vpn-cap__bar" aria-hidden="true">
                  <span
                    className="vpn-cap__fill"
                    data-over={usedPct >= 100}
                    style={{ width: `${Math.min(100, Math.max(0, usedPct))}%` }}
                  />
                </div>
                <div className="vpn-cap__meta">
                  <strong>{unlimited ? 'Unlimited' : `${usedPct}% used`}</strong>
                  <span className="ui-muted">
                    {formatBytes(usage.lifetime_total_bytes)}{' '}
                    {unlimited ? 'used' : `of ${formatBytes(usage.limit_bytes)}`}
                  </span>
                </div>
              </div>
              <div className="vpn-chips">
                {/* P-01: upload, download and total, separately. */}
                <span className="vpn-chip">↓ {formatBytes(usage.lifetime_down_bytes)} down</span>
                <span className="vpn-chip">↑ {formatBytes(usage.lifetime_up_bytes)} up</span>
                <span className="vpn-chip">Σ {formatBytes(usage.lifetime_total_bytes)} total</span>
                <span className="vpn-chip">
                  {unlimited
                    ? 'no limit'
                    : remaining !== null && remaining >= 0
                      ? `${formatBytes(remaining)} left`
                      : 'exhausted'}
                </span>
              </div>
              <span className="ui-muted t-xs">
                From {new Date(usage.created_at * 1000).toLocaleDateString()}
                {usage.expires_at
                  ? ` to ${new Date(usage.expires_at * 1000).toLocaleDateString()}`
                  : ' — no expiry'}
                {usage.remote_reset_count > 0 &&
                  ` · ${usage.remote_reset_count} panel counter reset(s) folded into the lifetime total`}
              </span>
            </div>
          )}
        </Card>

        <Card title="Live traffic">
          <div className="vpn-live" data-testid="vpn-live-traffic">
            <div className="vpn-live__row">
              <span className="vpn-live__icon" data-dir="up" aria-hidden="true">
                <ArrowUp size={14} />
              </span>
              <strong>{live?.source === 'sync' ? formatRate(live.up_bps) : '—'}</strong>
              <span className="ui-muted">upload</span>
            </div>
            <div className="vpn-live__row">
              <span className="vpn-live__icon" data-dir="down" aria-hidden="true">
                <ArrowDown size={14} />
              </span>
              <strong>{live?.source === 'sync' ? formatRate(live.down_bps) : '—'}</strong>
              <span className="ui-muted">download</span>
            </div>
            <span className="ui-muted t-xs">
              {live?.source === 'sync'
                ? `Rate between the last two sync readings (${new Date(
                    live.measured_from * 1000,
                  ).toLocaleTimeString()} → ${new Date(
                    live.measured_to * 1000,
                  ).toLocaleTimeString()}). A rate, not a total.`
                : 'Not measured yet — this needs two sync readings with traffic between them.'}
            </span>
          </div>
        </Card>
      </div>

      {/* ---------- P-01 + P-06 + P-08 : usage history ---------- */}
      <Card
        title="Usage history"
        actions={
          <div className="vpn-insights__actions">
            <Tabs
              items={RANGES.map((r) => ({ value: r.value, label: r.label }))}
              value={range}
              onChange={(v) => setRange(v as ApiUsageRange)}
            />
            <Button
              size="sm"
              variant="outline"
              onClick={() => void loadUsage(range)}
              disabled={usageBusy}
              leading={<RefreshCw size={13} />}
            >
              {usageBusy ? 'Loading…' : 'Refresh'}
            </Button>
          </div>
        }
      >
        <div className="ui-col" style={{ gap: 10 }}>
          <div className="vpn-insights__sub">
            <Tabs
              items={CHART_KINDS.map((c) => ({ value: c.value, label: c.label }))}
              value={chartKind}
              onChange={(v) => setChartKind(v as ChartKind)}
            />
            <span className="ui-muted t-xs">
              Last {rangeLabel} · source: panel sync snapshots
            </span>
          </div>

          {!hasPoints ? (
            <div className="vpn-empty" data-testid="vpn-usage-empty">
              <Activity size={16} aria-hidden="true" />
              <span>
                No data in this window
                {usage?.history === 'NONE'
                  ? ' — history starts from the first sync reading.'
                  : ' — nothing was recorded for the selected range.'}
              </span>
            </div>
          ) : chartKind === 'pie' ? (
            <DonutChart
              segments={windowTotals}
              centerLabel={formatBytes(windowTotals.reduce((a, b) => a + b.value, 0))}
              ariaLabel={`Traffic split over the last ${rangeLabel}`}
            />
          ) : chartKind === 'bar' ? (
            <BarChart
              data={windowTotals}
              showValues
              ariaLabel={`Traffic over the last ${rangeLabel}`}
            />
          ) : (
            <LineChart
              series={usageSeries}
              labels={labels}
              style={chartKind === 'area' ? 'area' : 'smooth'}
              ariaLabel={`Usage history over the last ${rangeLabel}: download, upload and total`}
            />
          )}
        </div>
      </Card>

      {/* ---------- P-03 + P-07 : server CPU and RAM ---------- */}
      <Card
        title="Server health"
        actions={
          <span className="ui-muted t-xs">
            Server-wide, not this VPN&apos;s traffic
          </span>
        }
      >
        {metricsError ? (
          <p className="ui-muted t-sm2" style={{ margin: 0 }}>{metricsError}</p>
        ) : !metrics ? (
          <p className="ui-muted t-sm2" style={{ margin: 0 }}>Loading server metrics…</p>
        ) : (
          <div className="ui-col" style={{ gap: 12 }}>
            <div className="vpn-insights__metrics">
              <MetricCard
                label="CPU"
                value={latest ? `${latest.cpu_pct.toFixed(0)}%` : '—'}
                icon={<Cpu size={14} />}
              />
              <MetricCard
                label="RAM"
                value={
                  latest
                    ? latest.mem_total > 0
                      ? `${formatBytes(latest.mem_used)} / ${formatBytes(latest.mem_total)}`
                      : formatBytes(latest.mem_used)
                    : '—'
                }
                icon={<MemoryStick size={14} />}
              />
              <MetricCard
                label="Samples"
                value={metrics.length}
                delta={latest ? `latest ${new Date(latest.taken_at * 1000).toLocaleTimeString()}` : undefined}
              />
            </div>

            <Tabs
              items={[
                { value: 'cpu', label: 'CPU' },
                { value: 'ram', label: 'RAM' },
              ]}
              value={metricTab}
              onChange={(v) => setMetricTab(v as 'cpu' | 'ram')}
            />

            {metrics.length === 0 ? (
              <div className="vpn-empty">
                <Activity size={16} aria-hidden="true" />
                <span>No server samples yet — they start with the first health check.</span>
              </div>
            ) : (
              <LineChart
                series={metricSeries}
                labels={metricLabels}
                style="area"
                ariaLabel={`Server-wide ${metricTab.toUpperCase()} over the recent samples`}
              />
            )}
          </div>
        )}
      </Card>

      {!active && (
        <p className="ui-muted t-xs" style={{ margin: 0 }}>
          A Free VPN shows the same usage charts; its history is bounded by the FREE offer rather
          than a purchase period (D-026).
        </p>
      )}
    </>
  )
}

