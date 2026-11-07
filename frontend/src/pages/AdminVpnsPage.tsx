import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  AsyncState,
  Button,
  Card,
  EmptyState,
  Input,
  MetricCard,
  PageHeader,
  Skeleton,
  StatusBadge,
  Table,
  type Column,
} from '../components/ui'
import { Gauge, Globe, TriangleAlert, Users } from 'lucide-react'
import { api, type ApiQuota } from '../lib/api'
import { useVpnStore, type ClientVpn } from '../stores/vpnStore'
import { VpnKindBadge } from '../components/vpn/VpnKindBadge'
import './clients.css'

/**
 * Admin (reseller) — the VPNs this reseller sold (Phase-4 UX, R2).
 *
 * The gap this closes: a reseller could see the *clients* it sold to, but never
 * the *VPNs* it sold them. Answering "is Nimal's VPN running out of data?" meant
 * opening each client in turn, or asking the Super Admin — and the support
 * question that arrives most often is exactly that one.
 *
 * Scope comes from the server, never from this screen: `/api/vpns` returns only
 * rows this reseller sold (or owns) and only rows the Super Admin has published
 * (an untagged panel client is invisible to a reseller, D-025.1). Nothing here
 * filters by role, because a client-side filter is not a permission.
 */
export function AdminVpnsPage() {
  const navigate = useNavigate()
  const rows = useVpnStore((s) => s.vpns)
  const loadVpns = useVpnStore((s) => s.load)
  const storeLoading = useVpnStore((s) => s.loading)
  const storeError = useVpnStore((s) => s.error)
  const [quota, setQuota] = useState<ApiQuota | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [q, setQ] = useState('')

  useEffect(() => {
    void loadVpns()
    // The quota is a separate read and its failure must not blank the list: the
    // reseller needs the VPNs more than they need the slot counter.
    api
      .myQuota()
      .then(setQuota)
      .catch(() => setQuota(null))
  }, [loadVpns])

  useEffect(() => {
    void (async () => {
      await Promise.resolve()
      setLoading(storeLoading)
      setError(storeError)
    })()
  }, [storeLoading, storeError])

  const filtered = useMemo(() => {
    const term = q.trim().toLowerCase()
    if (!term) return rows
    return rows.filter((v) =>
      `${v.panelEmail} ${v.offerName ?? ''}`.toLowerCase().includes(term),
    )
  }, [rows, q])

  /**
   * Days until expiry, computed against a clock read taken ONCE per load.
   *
   * `Date.now()` inside the render would make the component impure (and every
   * re-render would recompute a different number for the same row), so the
   * reference time is state that is set when the data arrives — which is also
   * exactly when the number becomes meaningful.
   *
   * `null` means "no expiry", which is a different statement from "0 days left"
   * and must never render as one.
   */
  const [nowSec] = useState(() => Date.now() / 1000)
  const daysLeft = (v: ClientVpn): number | null =>
    v.expiresAt === null ? null : Math.ceil((v.expiresAt - nowSec) / 86400)

  /* Counted from the rows the server already scoped, so the tiles can never
     describe VPNs this reseller is not allowed to see. */
  const premium = rows.filter((v) => v.kind === 'PREMIUM').length
  const free = rows.filter((v) => v.kind === 'FREE').length
  const expiring = rows.filter((v) => {
    const d = daysLeft(v)
    return d !== null && d >= 0 && d <= 7
  }).length
  const exhausted = rows.filter((v) => v.status === 'TRAFFIC_DEPLETED').length


  return (
    <div className="dash" data-testid="admin-vpns-page">
      <PageHeader
        title="My VPNs"
        subtitle="Every VPN you sold, with its usage and expiry — only panels assigned to you"
        actions={
          <Button size="sm" leading={<Globe size={14} />} onClick={() => navigate('/sale')}>
            Sell premium
          </Button>
        }
      />

      <div className="ui-grid">
        <MetricCard
          label="Premium sold"
          value={String(premium)}
          delta={`${free} free`}
          icon={<Globe size={18} />}
        />
        <MetricCard
          label="Quota"
          value={quota && quota.limit > 0 ? `${quota.used} / ${quota.limit}` : 'unlimited'}
          delta={quota && quota.limit > 0 ? `${quota.remaining} slot(s) left` : 'no ceiling'}
          deltaDirection={quota && quota.limit > 0 && quota.remaining <= 0 ? 'down' : 'up'}
          icon={<Gauge size={18} />}
        />
        <MetricCard
          label="Expiring ≤7d"
          value={String(expiring)}
          delta={expiring > 0 ? 'renewal conversation' : 'nothing urgent'}
          deltaDirection={expiring > 0 ? 'down' : 'up'}
          icon={<TriangleAlert size={18} />}
        />
        <MetricCard
          label="Out of data"
          value={String(exhausted)}
          delta={exhausted > 0 ? 'customers cannot connect' : 'all have data'}
          deltaDirection={exhausted > 0 ? 'down' : 'up'}
          icon={<Users size={18} />}
        />
      </div>

      {error && (
        <p className="cl-alert" role="alert">
          {error}
        </p>
      )}

      <Card title="Sold VPNs" actions={<span className="ui-muted t-sm">{rows.length} total</span>}>
        <div className="cl-filters__search" style={{ marginBottom: 10 }}>
          <Input
            label="Search"
            placeholder="panel name or package"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        <AsyncState
          status={loading ? 'loading' : filtered.length === 0 ? 'empty' : 'ready'}
          skeleton={<Skeleton height={160} />}
          empty={
            <EmptyState
              icon={<Globe size={22} />}
              title={rows.length === 0 ? 'You have not sold any VPNs yet' : 'Nothing matches'}
              description={
                rows.length === 0
                  ? 'A VPN appears here as soon as you assign one to a client from Sell premium.'
                  : 'Clear the search to see every VPN you sold.'
              }
            />
          }
        >
          <Table
            columns={COLUMNS(navigate, daysLeft)}
            data={filtered}
            rowKey={(v) => v.id}
          />
        </AsyncState>
      </Card>

      <p className="ui-muted cl-note">
        Panels you were not assigned are invisible here — that is a server-side rule, not a filter
        on this page. Ask the Super Admin for a panel if something is missing.
      </p>
    </div>
  )
}

/**
 * The columns.
 *
 * Ordered by what a support conversation needs: which VPN, is it working, how
 * much is left, and when does it end.
 */
function COLUMNS(navigate: (to: string) => void, daysLeft: (v: ClientVpn) => number | null): Column<ClientVpn>[] {
  return [
    {
      key: 'panelEmail',
      header: 'VPN',
      sortable: true,
      render: (v) => (
        <span className="cl-cell__text">
          <span className="ui-strong">{v.panelEmail}</span>
          {v.offerName && <span className="ui-muted cl-cell__sub">{v.offerName}</span>}
        </span>
      ),
    },
    {
      key: 'kind',
      header: 'Kind',
      // `kind` is already resolved by `toRow`, which is the one place the
      // `offer_kind || tag` fallback lives (D-063) — repeating it here would be a
      // second opinion that can drift.
      render: (v) => <VpnKindBadge kind={v.kind} />,
    },
    { key: 'status', header: 'Status', render: (v) => <StatusBadge status={v.status} /> },
    {
      key: 'expiresAt',
      header: 'Expiry',
      align: 'center',
      sortable: true,
      render: (v) => {
        const d = daysLeft(v)
        if (d === null) return <span className="ui-muted">never</span>
        if (d < 0) return <span className="cl-tag cl-tag--warn">expired</span>
        return <span className={d <= 7 ? 'cl-tag cl-tag--warn' : ''}>{d}d left</span>
      },
    },
    {
      key: 'open',
      header: '',
      align: 'right',
      render: (v) => (
        <button type="button" className="cl-open" onClick={() => navigate(`/vpn/${v.apiId}`)}>
          Detail →
        </button>
      ),
    },
  ]
}
