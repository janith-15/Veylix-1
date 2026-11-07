import { useEffect, useMemo } from 'react'
import { Card, PageHeader, Table, type Column } from '../components/ui'
import { useAdminsStore } from '../stores/adminsStore'
import { useAuthStore } from '../stores/authStore'
import { useOffersStore } from '../stores/offersStore'
import { usePaymentsStore } from '../stores/paymentsStore'
import './admins.css'

/**
 * Retail price — real offers, real money collected (2026-10-24).
 *
 * This page previously listed three invented plans (600/900/1400 wholesale) that
 * the backend had never heard of, and its Save button showed a success toast
 * while persisting nothing. Both are gone:
 *
 *  - the plan rows are the active PREMIUM offers this reseller can sell
 *    (`GET /api/offers`), with the traffic/duration the panel will enforce;
 *  - the money card is measured from confirmed payments
 *    (`GET /api/payments`, already role-scoped) — the only revenue figure the
 *    server can actually prove.
 *
 * There is deliberately no editable "retail price" input: no endpoint stores
 * one, so an input here would be a number that looks saved and is not. A sale's
 * amount is recorded per assignment on the Sell and Payments screens instead.
 */
export function RetailPricePage() {
  const user = useAuthStore((s) => s.user)
  const admins = useAdminsStore((s) => s.admins)
  const loadAdmins = useAdminsStore((s) => s.load)
  const me = admins.find((a) => a.email === (user?.email ?? ''))

  const offers = useOffersStore((s) => s.offers)
  const offersLoading = useOffersStore((s) => s.loading)
  const loadOffers = useOffersStore((s) => s.load)

  const payments = usePaymentsStore((s) => s.payments)
  const loadPayments = usePaymentsStore((s) => s.load)

  useEffect(() => {
    void loadOffers()
    void loadPayments()
    void loadAdmins()
  }, [loadOffers, loadPayments, loadAdmins])

  // "Sellable" = what the reseller can actually put on the Sell screen.
  const premiumOffers = offers.filter((o) => o.kind === 'PREMIUM' && o.active)

  const confirmedRevenue = useMemo(
    () => payments.filter((p) => p.status === 'CONFIRMED'),
    [payments],
  )

  // Revenue per plan is not attributable: a payment points at an assignment,
  // not at an offer, and the API exposes neither join. So this page reports one
  // honest total instead of a per-row number it would have to guess.
  const totalCents = confirmedRevenue.reduce((sum, p) => sum + p.amountCents, 0)
  const currency = confirmedRevenue[0]?.currency ?? 'LKR'
  const quotaMax = me?.quotaMax ?? 0

  interface Row {
    id: string
    name: string
    traffic: string
    duration: string
    devices: string
    server: string
  }

  const rows: Row[] = premiumOffers.map((o) => ({
    id: String(o.apiId),
    name: o.name,
    traffic: o.trafficLimitGB > 0 ? `${o.trafficLimitGB} GB` : 'unlimited',
    duration: o.expiryDays > 0 ? `${o.expiryDays} days` : 'no expiry',
    devices: o.deviceLimit > 0 ? String(o.deviceLimit) : 'unlimited',
    server: `server ${o.serverId}`,
  }))

  const COLUMNS: Column<Row>[] = [
    { key: 'name', header: 'Premium plan', render: (r) => <span className="ui-strong">{r.name}</span> },
    { key: 'traffic', header: 'Traffic' },
    { key: 'duration', header: 'Duration', hideOnMobile: true },
    { key: 'devices', header: 'Devices', hideOnMobile: true },
    { key: 'server', header: 'Panel', hideOnMobile: true, render: (r) => <span className="ui-muted">{r.server}</span> },
  ]

  return (
    <div className="dash" data-testid="retail-price">
      <PageHeader
        title="Retail price"
        subtitle={
          quotaMax > 0
            ? `Your resale catalogue · quota ceiling ${quotaMax} assignments · the sale amount is recorded per sale on Payments`
            : 'Your resale catalogue · the sale amount is recorded per sale on Payments'
        }
      />

      <div className="ui-grid">
        <Card title="Sellable premium plans">
          <span className="ui-strong" style={{ fontSize: 22 }}>{premiumOffers.length}</span>
          <div className="ui-muted t-sm">
            active PREMIUM offers from the platform catalogue
          </div>
        </Card>
        <Card title="Confirmed revenue">
          <span className="ui-strong" style={{ fontSize: 22 }}>{formatMoney(totalCents, currency)}</span>
          <div className="ui-muted t-sm">
            {confirmedRevenue.length} confirmed payment(s) — server-confirmed only
          </div>
        </Card>
        <Card title="Assigned panels">
          <span className="ui-strong" style={{ fontSize: 22 }}>{new Set(premiumOffers.map((o) => o.serverId)).size}</span>
          <div className="ui-muted t-sm">distinct panels behind your sellable plans</div>
        </Card>
      </div>

      <Card
        title="Premium plans — what the panel will enforce"
        actions={<span className="ui-muted t-sm">{offersLoading ? 'loading…' : `${rows.length} plans`}</span>}
      >
        {rows.length === 0 ? (
          <p className="ui-muted rhythm">
            No active premium offers {offersLoading ? 'are loading' : 'exist yet'} — a Super Admin
            creates them on the Offers screen. Until then there is nothing to sell, and this page
            will not invent a plan.
          </p>
        ) : (
          <Table columns={COLUMNS} data={rows} rowKey={(r) => r.id} />
        )}
      </Card>
    </div>
  )
}

/** Integer cents → "LKR 1,500.00". Formatted only at render time. */
function formatMoney(cents: number, currency: string): string {
  return `${currency} ${(cents / 100).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}
