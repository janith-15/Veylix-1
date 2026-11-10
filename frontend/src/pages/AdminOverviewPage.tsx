import { useEffect, useState } from 'react'
import { Button, Card, PageHeader } from '../components/ui'
import { Globe, Server, Users, Wallet, Gauge } from 'lucide-react'
import { Link } from 'react-router-dom'
import { useServersStore } from '../stores/serversStore'
import { useClientsStore } from '../stores/clientsStore'
import { usePaymentsStore } from '../stores/paymentsStore'
import { api, type ApiQuota } from '../lib/api'
import { MetricCard } from '../components/ui'
import { Progress } from '../components/charts'

/**
 * Reseller overview.
 *
 * Phase 5: the invented metrics ("24 clients", "18,400 LKR", "2 servers") are
 * gone. Every figure now has an endpoint behind it:
 *  - assigned servers ← `GET /api/servers` (already role-scoped by the backend)
 *  - quota ← `GET /api/quotas/me` (D-021: a client gets {0,0,-1})
 *  - clients ← `GET /api/clients`
 *  - revenue ← confirmed rows from `GET /api/payments` (2026-10-24: this card
 *    used to read "no API yet (P6)" long after the payments API shipped)
 */
export function AdminOverviewPage() {
  const servers = useServersStore((s) => s.servers)
  const loadServers = useServersStore((s) => s.load)
  const clients = useClientsStore((s) => s.clients)
  const loadClients = useClientsStore((s) => s.load)
  const payments = usePaymentsStore((s) => s.payments)
  const loadPayments = usePaymentsStore((s) => s.load)
  const [quota, setQuota] = useState<ApiQuota | null>(null)

  useEffect(() => {
    void loadServers()
    void loadClients()
    void loadPayments()
    api
      .myQuota()
      .then(setQuota)
      .catch(() => setQuota(null))
  }, [loadServers, loadClients, loadPayments])

  const assignedServers = servers.length
  const quotaMax = quota?.limit ?? 0
  const quotaUsed = quota?.used ?? 0
  const quotaLabel = quotaMax > 0 ? `${quotaUsed} / ${quotaMax}` : 'unlimited'

  const clientCount = clients.length
  const confirmed = payments.filter((p) => p.status === 'CONFIRMED')
  const confirmedCount = confirmed.length
  const pendingPayments = payments.filter(
    (p) => p.status === 'PENDING' || p.status === 'AWAITING_CONFIRMATION',
  ).length
  // Summed from the rows the server returned; there is no aggregate endpoint,
  // and inventing one here would be the same mistake the mock store made.
  const confirmedCents = confirmed.reduce((sum, p) => sum + p.amountCents, 0)
  const revenueLabel =
    confirmedCount === 0
      ? '0.00'
      : `${(confirmedCents / 100).toLocaleString(undefined, {
          minimumFractionDigits: 2,
          maximumFractionDigits: 2,
        })} ${confirmed[0].currency}`

  return (
    <div className="dash" data-testid="admin-overview">
      <PageHeader
        title="Reseller overview"
        subtitle={`Assigned servers: ${assignedServers} · active-assignment quota ${quotaLabel}`}
      />

      <div className="ui-grid">
        <MetricCard label="Assigned servers" value={String(assignedServers)} icon={<Server size={18} />} />
        <MetricCard label="Quota used" value={quotaLabel} delta="premium assignments" icon={<Gauge size={18} />} />
        <MetricCard label="Clients" value={String(clientCount)} delta="from /api/clients" icon={<Users size={18} />} />
        <MetricCard
          label="Confirmed revenue"
          value={revenueLabel}
          delta={`${confirmedCount} confirmed payment(s)`}
          icon={<Wallet size={18} />}
        />
      </div>

      <Card title="Quota & attention">
        <div className="ui-col" style={{ gap: 12 }}>
          <div>
            <span className="ui-muted t-sm2" style={{ display: 'block', marginBottom: 4 }}>
              Quota used — {quotaLabel}
            </span>
            <Progress value={quotaMax > 0 ? Math.round((quotaUsed / quotaMax) * 100) : 0} />
          </div>
          {pendingPayments > 0 && (
            <p className="vpn-warn" style={{ marginBottom: 0 }}>
              {pendingPayments} payment(s) awaiting your review.
            </p>
          )}
        </div>
      </Card>

      <Card title="Quick actions">
        <div className="ui-row">
          <Link to="/admin-servers" className="no-underline">
            <Button size="sm" variant="outline" leading={<Server size={14} />}>Assigned servers</Button>
          </Link>
          <Link to="/admin-clients" className="no-underline">
            <Button size="sm" variant="outline" leading={<Users size={14} />}>Clients</Button>
          </Link>
          <Link to="/admin-vpns" className="no-underline">
            <Button size="sm" variant="outline" leading={<Globe size={14} />}>My VPNs</Button>
          </Link>
          <Link to="/sale" className="no-underline">
            <Button size="sm" leading={<Wallet size={14} />}>Sell premium VPN</Button>
          </Link>
        </div>
        <p className="ui-muted t-sm2" style={{ marginBottom: 0 }}>
          Quota rule: you can assign premium VPNs only while quotaUsed &lt; quotaMax — expired or
          revoked assignments free a slot.
        </p>
      </Card>
    </div>
  )
}