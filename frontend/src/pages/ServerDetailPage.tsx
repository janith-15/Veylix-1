import { useEffect, useMemo, useState } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import {
  AsyncState,
  Button,
  Card,
  EmptyState,
  Input,
  MetricCard,
  PageHeader,
  Skeleton,
  Table,
  formatBytes,
  type Column,
} from '../components/ui'
import { ArrowLeft, RefreshCw } from 'lucide-react'
import type { ApiPanelClient } from '../lib/api'
import { useServersStore } from '../stores/serversStore'
import { useServerDetailStore } from '../stores/serverDetailStore'
import './server-detail.css'

/**
 * L7 — one server's drill-down (Phase G, D-037).
 *
 * The owner's question when opening a server is: *"which VPNs are on here, how
 * many are free, who holds them, and how much of each has been used?"* Before
 * this screen that answer only existed on the panel itself.
 *
 * Everything here is read-only against the panel (D-024 Stage C) — it can look,
 * never change a remote client. That is deliberate: the only write path is the
 * audited provisioning/release flow.
 */

type TagFilter = '' | 'FREE' | 'PREMIUM' | 'untagged'
type AssignedFilter = '' | 'yes' | 'no'

export function ServerDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { servers, load: loadServers } = useServersStore()
  const { rows, loading, error, load, reset } = useServerDetailStore()

  const serverApiId = Number(id)
  const server = servers.find((s) => s.apiId === serverApiId) ?? null

  const [tag, setTag] = useState<TagFilter>('')
  const [assigned, setAssigned] = useState<AssignedFilter>('')
  const [q, setQ] = useState('')

  useEffect(() => {
    if (servers.length === 0) void loadServers()
  }, [servers.length, loadServers])

  useEffect(() => {
    if (!Number.isFinite(serverApiId) || serverApiId <= 0) return
    // Filters are applied server-side so a large panel is not shipped wholesale.
    void load(serverApiId, { tag: tag || undefined, assigned: assigned || undefined, q: q.trim() || undefined })
  }, [serverApiId, tag, assigned, q, load])

  // Leaving the page drops the rows, so returning to a different server never
  // paints the previous one's clients for a moment.
  useEffect(() => reset, [reset])

  /* Computed from what the server returned, never invented. */
  const summary = useMemo(() => {
    return {
      total: rows.length,
      free: rows.filter((r) => r.tag === 'FREE').length,
      premium: rows.filter((r) => r.tag === 'PREMIUM').length,
      unowned: rows.filter((r) => !r.assigned).length,
    }
  }, [rows])

  const refresh = () => {
    if (!Number.isFinite(serverApiId) || serverApiId <= 0) return
    void load(serverApiId, { tag: tag || undefined, assigned: assigned || undefined, q: q.trim() || undefined })
  }


  const COLUMNS: Column<ApiPanelClient>[] = [
    {
      key: 'email',
      header: 'Client',
      sortable: true,
      render: (r) => (
        /*
         * The client name was plain text, so this screen was a dead end: the one
         * place that lists a panel's clients offered no way to open one. The
         * console already routes `/vpns/:serverId/:panelEmail` for exactly this,
         * so the name links there. A client with no panel e-mail (sub-id only)
         * stays text rather than linking to a route that cannot resolve.
         */
        <span>
          {r.panel_email ? (
            <Link
              to={`/vpns/${serverApiId}/${encodeURIComponent(r.panel_email)}`}
              className="sd-email sd-email--link"
            >
              {r.panel_email}
            </Link>
          ) : (
            <code className="sd-email">{r.sub_id}</code>
          )}
          <span className="sd-sub">
            {r.protocol ? `${r.protocol}${r.transport ? ` · ${r.transport}` : ''}` : r.sub_id}
          </span>
        </span>
      ),
    },
    {
      key: 'owner',
      header: 'Held by',
      render: (r) =>
        r.assigned ? (
          <span>
            {r.owner_name || r.owner_email || `user #${r.owner_user_id ?? '?'}`}
            {r.seller_email && <span className="sd-sub">seller {r.seller_email}</span>}
          </span>
        ) : (
          <span className="ui-muted t-sm2">unassigned</span>
        ),
    },
    {
      key: 'tag',
      header: 'Kind',
      sortable: true,
      render: (r) =>
        r.tag ? (
          <span className={`sd-tag sd-tag--${r.tag.toLowerCase()}`}>{r.tag}</span>
        ) : (
          <span className="sd-tag sd-tag--untagged">untagged</span>
        ),
    },
    {
      key: 'usage',
      header: 'Used of capacity',
      sortable: true,
      render: (r) => {
        // `limit_bytes <= 0` means "no limit", which must never render as 0%.
        if (r.limit_bytes <= 0) {
          return <span className="ui-muted t-sm2">{formatBytes(r.used_bytes)} · no limit</span>
        }
        const pct = Math.max(0, Math.min(100, r.usage_pct))
        const fillClass = pct >= 90 ? 'sd-bar__fill--high' : pct >= 70 ? 'sd-bar__fill--mid' : ''
        return (
          <span className="sd-usage">
            <span className="sd-bar" aria-hidden>
              <span className={`sd-bar__fill ${fillClass}`} style={{ width: `${pct}%` }} />
            </span>
            <span className="sd-usage__text">
              {formatBytes(r.used_bytes)} / {formatBytes(r.limit_bytes)} · {pct}%
            </span>
          </span>
        )
      },
    },
    {
      key: 'days',
      header: 'Expires',
      align: 'right',
      sortable: true,
      render: (r) =>
        r.days_left > 0 ? (
          <span className={r.days_left <= 3 ? 'sd-expire sd-expire--soon' : 'sd-expire'}>
            {r.days_left} {r.days_left === 1 ? 'day' : 'days'}
          </span>
        ) : (
          <span className="ui-muted t-sm2">no expiry</span>
        ),
    },
    {
      key: 'state',
      header: 'State',
      align: 'center',
      render: (r) => (
        <span className={r.enable ? 'sd-state sd-state--on' : 'sd-state sd-state--off'}>
          {r.enable ? 'enabled' : 'disabled'}
        </span>
      ),
    },
  ]

  return (
    <div className="sd-page">
      <PageHeader
        title={server ? server.name : `Server #${serverApiId || '?'}`}
        subtitle={
          server
            ? `${server.panelUrl} · ${server.status}${server.version ? ` · panel ${server.version}` : ''}`
            : 'Every client this panel holds, with who it belongs to and how much of it is used.'
        }
        actions={
          <div className="sd-actions">
            <Button variant="ghost" leading={<ArrowLeft size={14} />} onClick={() => navigate('/servers')}>
              All servers
            </Button>
            <Button
              variant="ghost"
              leading={<RefreshCw size={14} />}
              onClick={refresh}
              disabled={loading}
            >
              Refresh
            </Button>
          </div>
        }
      />

      <div className="sd-metrics">
        <MetricCard label="VPNs on this panel" value={String(summary.total)} variant="card" />
        <MetricCard label="Free" value={String(summary.free)} variant="card" />
        <MetricCard label="Premium" value={String(summary.premium)} variant="card" />
        <MetricCard label="Not held by anyone" value={String(summary.unowned)} variant="card" />
      </div>

      <Card className="sd-filters">
        <div className="sd-filters__row">
          <label className="ui-field">
            <span className="ui-field__label">Kind</span>
            <select className="ui-input" value={tag} onChange={(e) => setTag(e.target.value as TagFilter)}>
              <option value="">All kinds</option>
              <option value="FREE">Free</option>
              <option value="PREMIUM">Premium</option>
              <option value="untagged">Untagged</option>
            </select>
          </label>
          <label className="ui-field">
            <span className="ui-field__label">Assigned</span>
            <select
              className="ui-input"
              value={assigned}
              onChange={(e) => setAssigned(e.target.value as AssignedFilter)}
            >
              <option value="">Any</option>
              <option value="yes">Held by a client</option>
              <option value="no">Not held</option>
            </select>
          </label>
          <Input
            label="Search"
            placeholder="e-mail or sub-id"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
      </Card>

      {/*
        The error was rendered ABOVE as a plain notice and `AsyncState` was never
        told about it, so a failed panel read produced a dead-end screen: a
        sentence, no "Try again", and an empty-state card that could be mistaken
        for "this server has no clients". D-060 requires every data view to offer
        a way back from failure, so the failure is fed INTO AsyncState and the
        retry re-runs the same query.
      */}
      <AsyncState
        status={error ? 'error' : loading ? 'loading' : rows.length === 0 ? 'empty' : 'ready'}
        error={error ?? undefined}
        onRetry={refresh}
        skeleton={<Skeleton height={220} />}
        empty={
          <EmptyState
            title="No clients match"
            description="Try clearing a filter. A client created directly in 3X-UI still appears here."
          />
        }
      >
        <Card>
          <Table
            columns={COLUMNS}
            data={rows}
            rowKey={(r) => r.panel_email || r.sub_id}
            initialSort={{ key: 'email', dir: 'asc' }}
          />
        </Card>
      </AsyncState>
    </div>
  )
}

