import { useEffect, useState } from 'react'
import { Badge, Button, Card, PageHeader, Table, type Column } from '../components/ui'
import { CheckCircle2, Circle, Clock, XCircle } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { useVpnStore, vpnDisplayName, vpnServerLabel } from '../stores/vpnStore'
import { usePaymentsStore } from '../stores/paymentsStore'
import { api, type ApiOrderEvent } from '../lib/api'
import './history.css'

/**
 * Purchase history.
 *
 * Phase 5: the "waiting for confirmation" block read invented payments
 * (`clientEmail === email`) out of a mock array. It now reads the account's own
 * payments from `GET /api/payments`, which the backend already scopes by role —
 * so there is no client-side ownership filter to get wrong.
 *
 * Phase 6 (C5): every pending order now shows its own STATUS TIMELINE. The page
 * used to print `· PENDING` and stop, so the one question a paying customer
 * actually asks — "did my money arrive, and what happens next?" — had no answer
 * here even though `GET /api/payments/{id}/events` records every step of it.
 */
export function HistoryPage() {
  const navigate = useNavigate()
  const vpns = useVpnStore((s) => s.vpns)
  const vpnsLoading = useVpnStore((s) => s.loading)
  const loadVpns = useVpnStore((s) => s.load)
  const payments = usePaymentsStore((s) => s.payments)
  const loadPayments = usePaymentsStore((s) => s.load)

  /**
   * The immutable timeline of each pending order, keyed by payment id.
   *
   * Only the orders that still need a human decision are fetched: a CONFIRMED
   * order's story is finished and its result is on the row already. Reading all
   * of them would be one request per order for information nobody is asking for.
   */
  const [events, setEvents] = useState<Record<number, ApiOrderEvent[]>>({})

  useEffect(() => {
    void loadPayments()
    // The VPN list is a SEPARATE store and was never loaded here (I-37). A
    // client who opened /history directly — a bookmark, a refresh, a link in an
    // e-mail — saw "Records 0 items" over an empty table even though they own
    // VPNs, because only /client had ever populated this store.
    void loadVpns()
  }, [loadPayments, loadVpns])

  // "Pending" is exactly the two states that still need a human decision.
  const pendingPayments = payments.filter(
    (p) => p.status === 'PENDING' || p.status === 'AWAITING_CONFIRMATION',
  )

  const pendingKey = pendingPayments.map((p) => p.apiId).join(',')

  useEffect(() => {
    if (!pendingKey) return
    let cancelled = false
    void (async () => {
      await Promise.resolve()
      if (cancelled) return
      const pairs = await Promise.all(
        pendingKey.split(',').map(async (raw) => {
          const id = Number(raw)
          try {
            return [id, await api.paymentEvents(id)] as const
          } catch {
            // A timeline that cannot be read must not hide the order itself.
            return [id, [] as ApiOrderEvent[]] as const
          }
        }),
      )
      if (cancelled) return
      const next: Record<number, ApiOrderEvent[]> = {}
      for (const [id, evs] of pairs) next[id] = evs
      setEvents(next)
    })()
    return () => {
      cancelled = true
    }
  }, [pendingKey])

  interface Row {
    id: string
    name: string
    /** The product the customer bought (U2) — never the panel mailbox. */
    title: string
    server: string
    created: string
    status: string
  }

  const fmt = (unix: number | null) => (unix ? new Date(unix * 1000).toLocaleDateString() : '—')

  const rows: Row[] = [
    ...pendingPayments.map((p) => ({
      id: `payment ${p.apiId}`,
      name: `${formatMoney(p.amountCents, p.currency)}${p.reference ? ` · ${p.reference}` : ''}`,
      title: 'Premium order',
      server: '—',
      created: fmt(p.createdAt),
      status: p.status,
    })),
    ...vpns.map((v) => ({
      id: v.id,
      name: v.panelEmail,
      /*
       * U2 + U3 (UI/UX defect board). This table used to be titled with
       * `v.panelEmail` over `server ${v.serverId}`: a customer read their own
       * purchase history as "vyx-23 · server 1". The product they bought is the
       * title now, and the panel email moves to a muted technical line that a
       * support agent can still read back. The server label comes from the same
       * helper every other screen uses, so an unlabelled server says so
       * honestly instead of exposing a database id.
       */
      title: vpnDisplayName(v),
      server: vpnServerLabel(v),
      created: fmt(v.createdAt),
      status: v.status,
    })),
  ]

  const COLUMNS: Column<Row>[] = [
    {
      key: 'id',
      header: 'Record',
      render: (r) => (
        <span>
          <span className="ui-strong">{r.title}</span>
          <span className="ui-muted t-sm" style={{ display: 'block' }}>
            {r.name}
          </span>
        </span>
      ),
    },
    { key: 'server', header: 'Server', hideOnMobile: true },
    { key: 'created', header: 'Created', sortable: true },
    {
      key: 'status',
      header: 'Status',
      render: (r) => (
        <Badge
          tone={
            r.status === 'ACTIVE' || r.status === 'CONFIRMED'
              ? 'success'
              : r.status === 'PENDING' || r.status === 'AWAITING_CONFIRMATION'
                ? 'warning'
                : 'info'
          }
        >
          {r.status}
        </Badge>
      ),
    },
  ]

  return (
    <div className="dash" data-testid="history-page">
      <PageHeader
        title="Purchase history"
        subtitle="Your records are never deleted — even when a server is unavailable"
      />
      {pendingPayments.length > 0 && (
        <Card
          title="Waiting for confirmation"
          actions={
            <span className="ui-row" style={{ gap: 10 }}>
              <span className="ui-muted t-sm">{pendingPayments.length} open</span>
              {/* U12 (UI/UX defect board): the pending block and the records
                  table below it read as two unrelated screens, so a customer
                  who has just paid had no cue that their history is further
                  down. This is the cue — a same-page jump, not a new route. */}
              <a className="ui-muted t-sm" href="#records">
                Jump to records ↓
              </a>
            </span>
          }
        >
          <p className="ui-muted t-sm2" style={{ marginTop: 0 }}>
            Your money is recorded. A seller confirms receipt next, and the VPN is built at that
            moment — nothing is created before the payment is confirmed.
          </p>
          <div className="ui-col" style={{ gap: 12 }}>
            {pendingPayments.map((p) => (
              <OrderTimeline
                key={p.apiId}
                orderId={p.apiId}
                status={p.status}
                amount={formatMoney(p.amountCents, p.currency)}
                reference={p.reference}
                events={events[p.apiId] ?? []}
                since={fmt(p.createdAt)}
              />
            ))}
          </div>
        </Card>
      )}
      <Card
        id="records"
        title="Records"
        actions={
          <span className="ui-muted t-sm">
            {rows.length} items
          </span>
        }
      >
        {rows.length === 0 ? (
          // A bare "0 items" over an empty table told a customer nothing (I-37).
          // An empty history is a normal state — a new account has not bought or
          // claimed yet — so it says what is true and offers the next step.
          <div className="ui-col" style={{ gap: 10, padding: '8px 2px' }} data-testid="history-empty">
            <span className="ui-strong">
              {vpnsLoading ? 'Loading your records…' : 'Nothing here yet'}
            </span>
            <span className="ui-muted t-sm2" style={{ margin: 0 }}>
              {vpnsLoading
                ? 'Reading your VPNs and purchases from the server.'
                : 'Your VPNs and Premium orders appear here once you have one. A free VPN can be claimed from your dashboard; Premium is bought from the seller.'}
            </span>
            {!vpnsLoading && (
              <div className="ui-row" style={{ gap: 8 }}>
                <Button size="sm" onClick={() => navigate('/client')}>
                  Go to my VPNs
                </Button>
                <Button variant="outline" size="sm" onClick={() => navigate('/buy')}>
                  Buy premium
                </Button>
              </div>
            )}
          </div>
        ) : (
          <Table columns={COLUMNS} data={rows} rowKey={(r) => `${r.id}-${r.created}`} />
        )}
      </Card>
    </div>
  )
}

/**
 * One pending order, drawn as the three steps it actually has (C5).
 *
 * The three steps are fixed by the product, not inferred from the event log:
 * an order is placed, the payer reports the transfer, a seller confirms and the
 * VPN is built (D-031: confirming is what provisions). Deriving the step from the
 * status keeps the timeline honest even when the event log is unavailable, and the
 * recorded events are shown underneath as evidence rather than as the only source
 * of truth.
 */
function OrderTimeline({
  orderId,
  status,
  amount,
  reference,
  events,
  since,
}: {
  orderId: number
  status: string
  amount: string
  reference: string | null
  events: ApiOrderEvent[]
  since: string
}) {
  const awaiting = status === 'AWAITING_CONFIRMATION'
  const steps = [
    {
      key: 'placed',
      title: 'Order placed',
      done: true,
      detail: `${amount} · ${since}`,
    },
    {
      key: 'reported',
      title: reference ? 'Transfer reported' : 'Transfer not reported yet',
      done: Boolean(reference),
      detail: reference ? `reference ${reference}` : 'Open the order to submit your payment reference',
    },
    {
      key: 'confirm',
      title: awaiting ? 'Waiting for the seller to confirm' : 'Seller confirms receipt',
      done: false,
      detail: 'Confirming is what builds your VPN — nothing is created before it',
      waiting: awaiting,
    },
  ]

  return (
    <div className="ord-tl" data-testid={`order-timeline-${orderId}`}>
      <div className="ord-tl__head">
        <span className="ui-strong">Order #{orderId}</span>
        <Badge tone={awaiting ? 'warning' : 'info'}>{status}</Badge>
      </div>

      <ol className="ord-tl__steps">
        {steps.map((s) => (
          <li key={s.key} className={`ord-tl__step${s.done ? ' is-done' : ''}${s.waiting ? ' is-waiting' : ''}`}>
            <span className="ord-tl__icon" aria-hidden="true">
              {s.done ? (
                <CheckCircle2 size={16} />
              ) : s.waiting ? (
                <Clock size={16} />
              ) : (
                <Circle size={16} />
              )}
            </span>
            <span className="ord-tl__body">
              <span className="ui-strong">{s.title}</span>
              <span className="ui-muted t-sm2">{s.detail}</span>
            </span>
          </li>
        ))}
      </ol>

      {/* The log, when there is one: proof of each step with its own timestamp. */}
      {events.length > 0 && (
        <details className="ord-tl__log">
          <summary className="ui-muted t-sm2">
            {events.length} recorded event{events.length === 1 ? '' : 's'}
          </summary>
          <ul>
            {events.map((e) => (
              <li key={e.id}>
                {e.kind === 'REJECTED' || e.kind === 'CANCELLED' ? (
                  <XCircle size={12} aria-hidden="true" />
                ) : (
                  <CheckCircle2 size={12} aria-hidden="true" />
                )}{' '}
                <span className="ui-strong">{e.kind}</span>{' '}
                <span className="ui-muted">
                  {new Date(e.created_at * 1000).toLocaleString()}
                  {e.detail ? ` — ${e.detail}` : ''}
                </span>
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  )
}

/** Integer cents → "LKR 1,500.00". Money is formatted only at render time. */
function formatMoney(cents: number, currency: string): string {
  return `${currency} ${(cents / 100).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}