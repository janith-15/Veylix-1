import { useEffect, useState, type FormEvent } from 'react'
import {
  AsyncState,
  Button,
  Card,
  Dialog,
  EmptyState,
  PageHeader,
  Skeleton,
  StatusBadge,
  Table,
  useToast,
  type Column,
} from '../components/ui'
import { TriangleAlert, Users, Wifi } from 'lucide-react'
import { useAdminsStore } from '../stores/adminsStore'
import { useAuthStore } from '../stores/authStore'
import { useClientsStore, type ClientAccount } from '../stores/clientsStore'
import { useOffersStore } from '../stores/offersStore'
import { useVpnStore } from '../stores/vpnStore'
import { api } from '../lib/api'
import './admins.css'

/**
 * A reseller's clients — real data (`GET /api/clients`, P5).
 *
 * Phase A follow-up: the demo client list and the hard-coded plan table are
 * gone. The list now contains only clients who own a VPN this reseller sold, and
 * the offer picker lists the real PREMIUM offers instead of invented prices.
 */
export function AdminClientsPage() {
  const user = useAuthStore((s) => s.user)
  const admins = useAdminsStore((s) => s.admins)
  const clients = useClientsStore((s) => s.clients)
  const clientsLoading = useClientsStore((s) => s.loading)
  const clientsError = useClientsStore((s) => s.error)
  const loadClients = useClientsStore((s) => s.load)
  const offers = useOffersStore((s) => s.offers)
  const loadOffers = useOffersStore((s) => s.load)
  const assignPremium = useVpnStore((s) => s.assignPremium)
  const toast = useToast()

  const me = admins.find((a) => a.email === (user?.email ?? ''))
  /*
   * Quota (Phase-4 UX, R1).
   *
   * This page used to print a comment instead of a number — "per-admin usage is
   * not exposed yet, so 'left' is unknown" — while `/api/quotas/me` had been
   * returning `{used, limit, remaining}` all along. A reseller selling the last
   * slot had no way to see that it was the last slot.
   */
  const quotaMax = me?.quotaMax ?? 0
  const [quota, setQuota] = useState<{ used: number; limit: number; remaining: number } | null>(null)
  useEffect(() => {
    api
      .myQuota()
      .then(setQuota)
      .catch(() => setQuota(null))
  }, [])

  const premiumOffers = offers.filter((o) => o.kind === 'PREMIUM' && o.active)

  useEffect(() => {
    void loadClients()
    void loadOffers()
  }, [loadClients, loadOffers])

  const [assignFor, setAssignFor] = useState<ClientAccount | null>(null)
  const [offerId, setOfferId] = useState('')
  const [assigning, setAssigning] = useState(false)
  const [error, setError] = useState('')

  const openAssign = (client: ClientAccount) => {
    setAssignFor(client)
    setOfferId(premiumOffers[0] ? String(premiumOffers[0].apiId) : '')
    setError('')
  }

  const submitAssign = async (e: FormEvent) => {
    e.preventDefault()
    if (!assignFor) return
    if (!offerId) {
      setError('Pick a premium offer first — an assignment needs one.')
      return
    }
    setAssigning(true)
    setError('')
    try {
      await assignPremium(Number(offerId), assignFor.apiId)
      toast('success', 'Premium assigned', `${assignFor.name} — the panel confirmed the client`)
      setAssignFor(null)
      void loadClients()
    } catch {
      setError('The panel did not confirm the assignment, so nothing was created.')
    } finally {
      setAssigning(false)
    }
  }

  const COLUMNS: Column<ClientAccount>[] = [
    {
      key: 'name',
      header: 'Client',
      sortable: true,
      render: (c) => (
        <span>
          <span className="ui-strong">{c.name}</span>
          <span className="adm-email">{c.email}</span>
        </span>
      ),
    },
    { key: 'vpnCount', header: 'VPNs', align: 'right', sortable: true },
    {
      key: 'status',
      header: 'Status',
      render: (c) => <StatusBadge status={c.status === 'ACTIVE' ? 'ACTIVE' : c.status} />,
    },
    {
      key: 'actions',
      header: '',
      render: (c) => (
        <Button size="sm" variant="outline" leading={<Wifi size={14} />} onClick={() => openAssign(c)}>
          Assign premium
        </Button>
      ),
    },
  ]

  return (
    <div className="dash" data-testid="admin-clients">
      <PageHeader
        title="Clients"
        subtitle={
          quota && quota.limit > 0
            ? `Only clients you sold to are listed — quota ${quota.used} / ${quota.limit} used, ${quota.remaining} slot(s) left`
            : quotaMax > 0
              ? `Only clients you sold to are listed — quota ceiling ${quotaMax} assignments`
              : 'Only clients you sold to are listed — quota ceiling unlimited'
        }
      />

      {/* The meter, not just the ceiling: at 80%+ a reseller is about to be
          refused, and that has to be visible before they click Sell. */}
      {quota && quota.limit > 0 && (
        <div className="cl-quotabar" style={{ marginBottom: 12, minWidth: 220 }}>
          <span className="cl-quotabar__num">
            Quota {quota.used} / {quota.limit} — {quota.remaining} left
          </span>
          <span className="cl-quotabar__track">
            <span
              className={`cl-quotabar__fill${
                quota.used >= quota.limit
                  ? ' is-full'
                  : quota.used / quota.limit >= 0.8
                    ? ' is-high'
                    : ''
              }`}
              style={{ width: `${Math.min(100, Math.round((quota.used / quota.limit) * 100))}%` }}
            />
          </span>
        </div>
      )}

      {premiumOffers.length === 0 && (
        <div className="vpn-warn" style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
          <TriangleAlert size={16} aria-hidden="true" />
          <span>
            No active PREMIUM offers exist, so nothing can be assigned yet. A Super
            Admin creates offers under Offers.
          </span>
        </div>
      )}

      <Card title="Clients" actions={<span className="ui-muted t-sm">{clients.length} clients</span>}>
        {/*
         * Phase D: these three states were bare sentences. "Loading..." was
         * indistinguishable from "you have no clients", and the error line had
         * no way to try again — a reseller staring at a failed fetch had to
         * reload the whole page. AsyncState is the app's single pattern for
         * loading / failed / empty, and it carries the retry.
         */}
        <AsyncState
          status={clientsLoading ? 'loading' : clientsError ? 'error' : clients.length === 0 ? 'empty' : 'ready'}
          error={clientsError}
          onRetry={() => void loadClients()}
          skeleton={<Skeleton height={160} />}
          empty={
            <EmptyState
              icon={<Users size={22} />}
              title="No clients yet"
              description="A client appears here once you sell them a Premium VPN."
            />
          }
        >
          <Table columns={COLUMNS} data={clients} rowKey={(r) => r.id} />
        </AsyncState>
      </Card>

      <Dialog
        open={!!assignFor}
        onClose={() => setAssignFor(null)}
        title={`Assign premium — ${assignFor?.name ?? ''}`}
        footer={
          <>
            <Button variant="ghost" onClick={() => setAssignFor(null)}>Cancel</Button>
            <Button type="submit" form="assign-form" disabled={assigning || premiumOffers.length === 0}>
              {assigning ? 'Provisioning…' : 'Assign & use a slot'}
            </Button>
          </>
        }
      >
        <form id="assign-form" onSubmit={submitAssign} className="ui-col" noValidate>
          <label className="ui-field">
            <span className="ui-field__label">Premium offer</span>
            <select
              className="ui-input"
              value={offerId}
              onChange={(e) => setOfferId(e.target.value)}
            >
              {premiumOffers.length === 0 ? (
                <option value="">No active premium offers</option>
              ) : (
                premiumOffers.map((o) => (
                  <option key={o.apiId} value={o.apiId}>
                    {o.name} — {o.trafficLimitGB > 0 ? `${o.trafficLimitGB} GB` : 'unlimited'}
                    {o.expiryDays > 0 ? ` · ${o.expiryDays}d` : ''}
                  </option>
                ))
              )}
            </select>
          </label>
          <p className="ui-muted t-sm2" style={{ margin: 0 }}>
            The backend provisions on the panel and confirms with a fresh read. If
            the panel cannot be reached the VPN is not created and nothing is billed.
          </p>
          {error && (
            <p className="ui-field__error" role="alert">
              {error}
            </p>
          )}
        </form>
      </Dialog>
    </div>
  )
}

