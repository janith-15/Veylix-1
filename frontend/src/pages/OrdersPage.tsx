import { useEffect, useMemo, useState } from 'react'
import {
  AsyncState,
  Button,
  Card,
  Dialog,
  EmptyState,
  Input,
  PageHeader,
  StatusBadge,
  Table,
  useToast,
  type Column,
} from '../components/ui'
import { useNavigate } from 'react-router-dom'
import { Ban, CheckCircle2, History, Settings2, ShieldCheck, XCircle } from 'lucide-react'
import { usePaymentsStore, allowedTransitions, type Payment } from '../stores/paymentsStore'
import { useOrdersStore } from '../stores/ordersStore'
import './orders.css'
import { activeTimeZone } from '../lib/time'

/**
 * L2 — the order queue (D-031, D-036).
 *
 * An order is a payment: confirming it is the step that *builds* the VPN, so
 * this screen is the only place that turns money into a working VPN. It exists
 * because the backend had no screen for it (M7-16): fifteen tested routes with
 * no caller meant the owner's confirm → VPN → block flow could only be performed
 * by hand.
 *
 * Rules the screen is careful about:
 *  - it offers only the transitions `allowedTransitions` permits, and the server
 *    still has the final word (409 on a stale row);
 *  - blocking requires a reason, because an unexplained block is unauditable;
 *  - a CUSTOM order is released onto a server + inbound chosen here, never
 *    guessed.
 */

function formatMoney(cents: number, currency: string): string {
  return `${currency} ${(cents / 100).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}

/** Seconds → a short local timestamp; the wire never carries a formatted date. */
function when(seconds: number): string {
  return new Date(seconds * 1000).toLocaleString(undefined, { timeZone: activeTimeZone() })
}

/** Human label for a timeline `kind`, without inventing one the server cannot send. */
const EVENT_LABEL: Record<string, string> = {
  created: 'Order created',
  reference: 'Reference added',
  method_changed: 'Payment destination changed',
  confirmed: 'Payment confirmed — VPN built',
  rejected: 'Payment rejected',
  refunded: 'Payment refunded',
  cancelled: 'Payment cancelled',
  blocked: 'VPN blocked',
  unblocked: 'VPN unblocked',
  release_target: 'Release target chosen',
}

type Action = 'confirm' | 'reject' | 'refund' | 'cancel' | 'block' | 'unblock'

interface Target {
  payment: Payment
  action: Action
}

export function OrdersPage() {
  const { payments, loading, busy, error, load, confirm, reject, refund, cancel, totalOrders, truncated } =
    usePaymentsStore()

  const { loadEvents, eventsFor, block, unblock } = useOrdersStore()
  const toast = useToast()
  const navigate = useNavigate()

  const [target, setTarget] = useState<Target | null>(null)
  const [blockReason, setBlockReason] = useState('')
  const [timelineFor, setTimelineFor] = useState<Payment | null>(null)

  useEffect(() => {
    void load()
  }, [load])

  /** Pending decisions float to the top: they are the only rows needing action. */
  const visible = useMemo(() => {
    const rank = (s: Payment['status']) =>
      s === 'AWAITING_CONFIRMATION' ? 0 : s === 'PENDING' ? 1 : s === 'CONFIRMED' ? 2 : 3
    return [...payments].sort((a, b) => rank(a.status) - rank(b.status) || b.apiId - a.apiId)
  }, [payments])

  const pending = payments.filter((p) => p.status === 'PENDING' || p.status === 'AWAITING_CONFIRMATION').length
  const confirmed = payments.filter((p) => p.status === 'CONFIRMED').length

  const openTimeline = (p: Payment) => {
    setTimelineFor(p)
    void loadEvents(p.apiId)
  }

  const run = async (payment: Payment, action: Action) => {
    try {
      if (action === 'confirm') {
        await confirm(payment.apiId)
        toast('success', 'Payment confirmed', `Order #${payment.apiId} is confirmed — the VPN is built and its links are ready.`)
      } else if (action === 'reject') {
        await reject(payment.apiId)
        toast('success', 'Order rejected', `Order #${payment.apiId} was rejected and cannot be re-opened.`)
      } else if (action === 'refund') {
        await refund(payment.apiId)
        toast('success', 'Order refunded', `Order #${payment.apiId} is recorded as refunded.`)
      } else if (action === 'cancel') {
        await cancel(payment.apiId)
        toast('success', 'Order cancelled', `Order #${payment.apiId} was cancelled.`)
      } else if (action === 'block') {
        await block(payment.apiId, blockReason.trim())
        toast('success', 'VPN blocked', `The VPN for order #${payment.apiId} is blocked until the money arrives.`)
      } else {
        await unblock(payment.apiId)
        toast('success', 'VPN unblocked', `The VPN for order #${payment.apiId} is restored.`)
      }
    } catch {
      toast('error', 'Not applied', 'The order changed state — the list now shows the server record.')
      await load()
    }
    setTarget(null)
    setBlockReason('')
  }

  const COLUMNS: Column<Payment>[] = [
    {
      key: 'id',
      header: 'Order',
      sortable: true,
      // The cell renders "#12" plus a human date, so the raw id is not what
      // sorts — but it IS what an operator means by "newest first".
      sortValue: (p) => p.apiId,
      filter: {
        kind: 'text',
        getValue: (p) => String(p.apiId),
        placeholder: 'Order #…',
      },
      render: (p) => (
        <span>
          <strong>#{p.apiId}</strong>
          <span className="ord-when">{when(p.createdAt)}</span>
        </span>
      ),
    },
    {
      key: 'placedBy',
      header: 'Placed by',
      // D-113: the operator's first question is whose money this is. It sits
      // right after the order number so a row reads left to right: this order,
      // placed by whom, for what. An order bought FOR someone else shows both,
      // because naming only the payer would mislead about who receives the VPN.
      sortable: true,
      // Sorting by the PAYER, then the recipient, so an order bought for
      // somebody else files under the account that paid. Without `sortValue`
      // this column sorted on `row.placedBy`, which does not exist, so the
      // arrow moved and the order never changed.
      sortValue: (p) => p.payerEmail || p.targetEmail || '',
      // Search matches BOTH identities, so an order bought for somebody is
      // findable by their address as well as the payer's.
      searchValue: (p) => `${p.payerEmail} ${p.targetEmail}`,
      filter: {
        kind: 'text',
        // Matches either identity: an operator searching "for tenant" or the
        // reseller's address both find the order.
        getValue: (p) => `${p.payerEmail} ${p.targetEmail}`,
        placeholder: 'Email…',
      },
      render: (p) => (
        <span className="ord-who">
          {p.payerEmail ? (
            <span className="ord-who__name">{p.payerEmail}</span>
          ) : (
            <span className="ui-muted t-sm2">account no longer exists</span>
          )}
          {p.targetEmail && p.targetEmail !== p.payerEmail && (
            <span className="ord-who__for t-sm2">for {p.targetEmail}</span>
          )}
        </span>
      ),
    },
    {
      key: 'package',
      header: 'Package',
      hideOnMobile: true,
      sortable: true,
      // Sorted by the plan NAME; an order with no plan sorts first, which puts
      // the manual records together instead of scattering them.
      sortValue: (p) => p.offerName || '',
      searchValue: (p) => `${p.offerName} ${p.offerKind}`,
      filter: { kind: 'text', getValue: (p) => p.offerName, placeholder: 'Package…' },
      // A manual record or a deleted plan has no name. Say so — an empty cell
      // reads as "we lost it", which is a different and worse claim.
      render: (p) =>
        p.offerName ? (
          <span className="ord-package">
            <span>{p.offerName}</span>
            {p.offerKind && <span className="ord-package__kind t-sm2">{p.offerKind}</span>}
          </span>
        ) : (
          <span className="ui-muted t-sm2">no package (manual record)</span>
        ),
    },
    {
      key: 'amount',
      header: 'Amount',
      align: 'right',
      sortable: true,
      // Money is integer cents on the wire and a formatted string on screen.
      // Sorting the string would order "LKR 900" before "LKR 1,200".
      sortValue: (p) => p.amountCents,
      searchValue: (p) => formatMoney(p.amountCents, p.currency),
      render: (p) => <span className="ord-amount">{formatMoney(p.amountCents, p.currency)}</span>,
    },
    {
      key: 'status',
      header: 'Status',
      sortable: true,
      // Typing "confirmed" into the search box should find confirmed orders.
      searchValue: (p) => p.status.replace(/_/g, ' '),
      filter: {
        kind: 'select',
        options: [
          { value: 'PENDING', label: 'Pending' },
          { value: 'AWAITING_CONFIRMATION', label: 'Awaiting confirmation' },
          { value: 'CONFIRMED', label: 'Confirmed' },
          { value: 'REJECTED', label: 'Rejected' },
          { value: 'CANCELLED', label: 'Cancelled' },
          { value: 'REFUND_PENDING', label: 'Refund pending' },
          { value: 'REFUNDED', label: 'Refunded' },
        ],
      },
      render: (p) => <StatusBadge status={p.status} />,
    },
    {
      key: 'reference',
      header: 'Reference',
      hideOnMobile: true,
      sortable: true,
      sortValue: (p) => p.reference || '',
      filter: { kind: 'text', getValue: (p) => p.reference, placeholder: 'Reference…' },
      render: (p) =>
        p.reference ? (
          <code className="ord-ref">{p.reference}</code>
        ) : (
          <span className="ui-muted t-sm2">none yet</span>
        ),
    },
    {
      key: 'timeline',
      header: 'History',
      hideOnMobile: true,
      render: (p) => (
        <div className="ord-actions">
          <Button
            variant="ghost"
            leading={<History size={14} />}
            onClick={() => openTimeline(p)}
            title={`Order #${p.apiId} timeline`}
          >
            Timeline
          </Button>
          <Button
            variant="ghost"
            leading={<Settings2 size={14} />}
            onClick={() => navigate(`/orders/${p.apiId}`)}
            title={`Release order #${p.apiId} onto a server`}
          >
            Release
          </Button>
        </div>
      ),
    },
    {
      key: 'actions',
      header: '',
      align: 'right',
      sticky: true,
      render: (p) => {
        const allowed = allowedTransitions(p.status)
        const canConfirm = allowed.includes('confirm')
        const canReject = allowed.includes('reject')
        const canRefund = allowed.includes('refund')
        const canCancel = allowed.includes('cancel')
        const isConfirmed = p.status === 'CONFIRMED'
        return (
          <div className="ord-actions">
            {canConfirm && (
              <Button
                leading={<CheckCircle2 size={14} />}
                onClick={() => setTarget({ payment: p, action: 'confirm' })}
                disabled={busy === p.apiId}
              >
                Confirm
              </Button>
            )}
            {canReject && (
              <Button
                variant="ghost"
                leading={<XCircle size={14} />}
                onClick={() => setTarget({ payment: p, action: 'reject' })}
                disabled={busy === p.apiId}
              >
                Reject
              </Button>
            )}
            {isConfirmed && (
              <>
                <Button
                  variant="ghost"
                  leading={<Ban size={14} />}
                  onClick={() => setTarget({ payment: p, action: 'block' })}
                  disabled={busy === p.apiId}
                >
                  Block VPN
                </Button>
                <Button
                  variant="ghost"
                  leading={<ShieldCheck size={14} />}
                  onClick={() => setTarget({ payment: p, action: 'unblock' })}
                  disabled={busy === p.apiId}
                >
                  Unblock
                </Button>
              </>
            )}
            {canRefund && (
              <Button
                variant="ghost"
                onClick={() => setTarget({ payment: p, action: 'refund' })}
                disabled={busy === p.apiId}
              >
                Refund
              </Button>
            )}
            {canCancel && (
              <Button
                variant="ghost"
                onClick={() => setTarget({ payment: p, action: 'cancel' })}
                disabled={busy === p.apiId}
              >
                Cancel
              </Button>
            )}
          </div>
        )
      },
    },
  ]

  const title = target
    ? target.action === 'confirm'
      ? 'Confirm payment received'
      : target.action === 'reject'
        ? 'Reject this order'
        : target.action === 'refund'
          ? 'Refund this order'
          : target.action === 'cancel'
            ? 'Cancel this order'
            : target.action === 'block'
              ? 'Block this VPN'
              : 'Unblock this VPN'
    : ''

  return (
    <div className="orders-page">
      <PageHeader
        title="Orders"
        subtitle="Every order is a payment. Confirming one builds the customer’s VPN — blocking one switches it off when the money does not arrive."
        actions={
          <div className="ord-summary">
            <span className="ord-pill">
              <strong>{pending}</strong> awaiting decision
            </span>
            <span className="ord-pill">
              <strong>{confirmed}</strong> confirmed
            </span>
          </div>
        }
      />

      {error && (
        <Card className="ord-notice">
          <p className="t-sm2">{error}</p>
        </Card>
      )}

      <AsyncState status={loading ? 'loading' : visible.length === 0 ? 'empty' : 'ready'}>
        <Card>
          {visible.length === 0 ? (
            <EmptyState
              title="No orders yet"
              description="An order appears here the moment a customer or reseller creates one. Confirming it builds the VPN."
              /*
               * There is no button that CREATES an order on this screen — orders
               * arrive from customers — so the action points at the one thing the
               * operator can usefully do with an empty queue: make sure the
               * catalogue customers order from is not empty too.
               */
              action={
                <Button size="sm" variant="outline" onClick={() => navigate('/packages')}>
                  Check the package catalogue
                </Button>
              }
            />
          ) : (
            <>
              {/*
               * D-116: the list is capped at the newest 200. Silence here was the
               * defect — the page looked like the complete set. The sentence says
               * which orders are missing and that the total is unknown when the
               * count could not be read, rather than showing a reassuring zero.
               */}
              {truncated && (
                <p className="ord-truncated" data-testid="orders-truncated">
                  {totalOrders < 0
                    ? `Showing the newest ${payments.length} orders. The total could not be read, so there may be older orders not listed here.`
                    : `Showing the newest ${payments.length} of ${totalOrders.toLocaleString()} orders. Older orders are not listed here.`}
                </p>
              )}
              <Table
                columns={COLUMNS}
                data={visible}
                rowKey={(p) => p.id}
                initialSort={{ key: 'id', dir: 'desc' }}
                searchable
                searchPlaceholder="Search order #, email, package or reference…"
                caption="orders"
              />
            </>
          )}
        </Card>
      </AsyncState>

      <Dialog
        open={target !== null}
        onClose={() => {
          setTarget(null)
          setBlockReason('')
        }}
        title={title}
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => {
                setTarget(null)
                setBlockReason('')
              }}
            >
              Close
            </Button>
            <Button
              onClick={() => target && void run(target.payment, target.action)}
              disabled={
                !target ||
                busy === target.payment.apiId ||
                (target.action === 'block' && blockReason.trim().length < 3)
              }
            >
              {target?.action === 'confirm'
                ? 'Confirm and build the VPN'
                : target?.action === 'reject'
                  ? 'Reject'
                  : target?.action === 'refund'
                    ? 'Refund'
                    : target?.action === 'cancel'
                      ? 'Cancel order'
                      : target?.action === 'block'
                        ? 'Block VPN'
                        : 'Unblock VPN'}
            </Button>
          </>
        }
      >
        <p>
          Order <strong>#{target?.payment.apiId}</strong> —{' '}
          {target ? formatMoney(target.payment.amountCents, target.payment.currency) : ''}
          {target?.payment.reference ? ` (ref ${target.payment.reference})` : ' (no reference)'}.
        </p>

        {target?.action === 'block' ? (
          <Input
            label="Why is this VPN blocked?"
            hint="Required — an unexplained block cannot be audited. The reason is stored on the order’s timeline."
            value={blockReason}
            onChange={(e) => setBlockReason(e.target.value)}
            placeholder="e.g. payment not received after 7 days"
            data-autofocus
          />
        ) : (
          <p className="ui-muted t-sm2">
            {target?.action === 'confirm'
              ? 'Confirm only after the money is received. This builds the VPN on the panel and cannot be undone — the only exit afterwards is a refund.'
              : target?.action === 'reject'
                ? 'Rejecting tells the customer the payment was not accepted. It cannot be re-opened; they must create a new order.'
                : target?.action === 'refund'
                  ? 'Refunding is only available for a confirmed payment. It cannot be reversed.'
                  : target?.action === 'cancel'
                    ? 'Cancelling withdraws the order before anyone confirmed it.'
                    : 'Unblocking restores the VPN. The block and this unblock both stay on the timeline.'}
          </p>
        )}
      </Dialog>

      <Dialog
        open={timelineFor !== null}
        onClose={() => setTimelineFor(null)}
        title={`Order #${timelineFor?.apiId ?? ''} — timeline`}
        wide
        footer={
          <Button variant="ghost" onClick={() => setTimelineFor(null)}>
            Close
          </Button>
        }
      >
        {timelineFor && eventsFor(timelineFor.apiId).length === 0 ? (
          <p className="ui-muted t-sm2">No events recorded for this order yet.</p>
        ) : (
          <ol className="ord-timeline">
            {timelineFor &&
              eventsFor(timelineFor.apiId).map((e) => (
                <li key={e.id} className="ord-timeline__item">
                  <span className="ord-timeline__dot" aria-hidden />
                  <div>
                    <p className="ord-timeline__kind">{EVENT_LABEL[e.kind] ?? e.kind}</p>
                    <p className="ui-muted t-sm2">
                      {when(e.createdAt)}
                      {e.actorEmail ? ` · ${e.actorEmail}` : ''}
                    </p>
                    {e.detail && <p className="ord-timeline__detail">{e.detail}</p>}
                  </div>
                </li>
              ))}
          </ol>
        )}
      </Dialog>
    </div>
  )
}
