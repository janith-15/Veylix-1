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
  return new Date(seconds * 1000).toLocaleString()
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
  const { payments, loading, busy, error, load, confirm, reject, refund, cancel } = usePaymentsStore()
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
      render: (p) => (
        <span>
          <strong>#{p.apiId}</strong>
          <span className="ord-when">{when(p.createdAt)}</span>
        </span>
      ),
    },
    {
      key: 'amount',
      header: 'Amount',
      align: 'right',
      sortable: true,
      render: (p) => <span className="ord-amount">{formatMoney(p.amountCents, p.currency)}</span>,
    },
    {
      key: 'status',
      header: 'Status',
      sortable: true,
      render: (p) => <StatusBadge status={p.status} />,
    },
    {
      key: 'reference',
      header: 'Reference',
      hideOnMobile: true,
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
            <Table
              columns={COLUMNS}
              data={visible}
              rowKey={(p) => p.id}
              initialSort={{ key: 'id', dir: 'desc' }}
            />
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
