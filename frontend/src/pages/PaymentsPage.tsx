import { useEffect, useMemo, useState } from 'react'
import {
  AsyncState,
  Badge,
  Button,
  Card,
  Dialog,
  EmptyState,
  Input,
  PageHeader,
  Skeleton,
  StatusBadge,
  Table,
  useToast,
  type Column,
} from '../components/ui'
import { usePaymentsStore, allowedTransitions, type Payment } from '../stores/paymentsStore'
import { CreditCard, Plus, Wallet } from 'lucide-react'
import { activeTimeZone } from '../lib/time'

/**
 * Billing — the manual-payment confirmation queue (P6, REQUIREMENTS §2.5).
 *
 * Phase 5: this page read two invented payments and "confirmed" one locally,
 * which meant the screen could say a customer had paid when no record existed.
 * It now drives the real state machine and offers only the transitions the
 * server will accept — the server still has the final word (409 on a stale row).
 */
export function PaymentsPage() {
  const {
    payments,
    methods,
    loading,
    busy,
    error,
    load,
    loadMethods,
    confirm,
    reject,
    refund,
    cancel,
    createMethod,
  } = usePaymentsStore()
  const toast = useToast()

  const [target, setTarget] = useState<{ payment: Payment; action: 'confirm' | 'reject' | 'refund' } | null>(
    null,
  )
  const [methodOpen, setMethodOpen] = useState(false)
  const [methodLabel, setMethodLabel] = useState('')
  const [methodKind, setMethodKind] = useState<'BANK' | 'EZCASH' | 'MOBILE' | 'OTHER'>('BANK')
  const [methodDetails, setMethodDetails] = useState('')

  useEffect(() => {
    void load()
    void loadMethods()
  }, [load, loadMethods])

  // The backend already scopes the list by role; pending rows float to the top
  // because they are the only ones needing a decision.
  const visible = useMemo(() => {
    const rank = (s: Payment['status']) =>
      s === 'AWAITING_CONFIRMATION' ? 0 : s === 'PENDING' ? 1 : 2
    return [...payments].sort((a, b) => rank(a.status) - rank(b.status) || b.apiId - a.apiId)
  }, [payments])

  const pendingCount = payments.filter(
    (p) => p.status === 'PENDING' || p.status === 'AWAITING_CONFIRMATION',
  ).length

  const run = async (action: 'confirm' | 'reject' | 'refund' | 'cancel', id: number) => {
    const fn = action === 'confirm' ? confirm : action === 'reject' ? reject : action === 'refund' ? refund : cancel
    try {
      await fn(id)
      toast(
        'success',
        action === 'confirm' ? 'Payment confirmed' : action === 'reject' ? 'Payment rejected' : action === 'refund' ? 'Payment refunded' : 'Payment cancelled',
        `Payment #${id} is now recorded with your identity`,
      )
    } catch {
      toast('error', 'Not applied', 'The payment changed state — the list now shows the server record')
    }
    setTarget(null)
  }

  const addMethod = async () => {
    try {
      const created = await createMethod({
        kind: methodKind,
        label: methodLabel.trim(),
        details: methodDetails.trim(),
      })
      if (!created) return
      setMethodOpen(false)
      setMethodLabel('')
      setMethodDetails('')
      toast('success', 'Payment method added', created.label)
    } catch (err) {
      toast('error', 'Could not add the method', err instanceof Error ? err.message : 'Unknown error')
    }
  }

  const COLUMNS: Column<Payment>[] = [
    {
      key: 'id',
      header: 'Payment',
      sortable: true,
      sortValue: (p) => p.apiId,
      searchValue: (p) => `#${p.apiId} ${p.reference}`,
      filter: { kind: 'text', getValue: (p) => `${p.apiId} ${p.reference}`, placeholder: 'Payment # or ref…' },
      render: (p) => {
        const since = waitingSince(p)
        return (
          <span>
            <span className="ui-strong">#{p.apiId}</span>
            <span className="pay-client">
              {p.reference ? `ref ${p.reference}` : 'no reference yet'}
              {since && (
                <span
                  className={`pay-age${Date.now() / 1000 - since >= 86_400 ? ' pay-age--cold' : ''}`}
                  title="Time waiting in this state — a queue, not a history"
                >
                  waiting {queueAge(since)}
                </span>
              )}
            </span>
          </span>
        )
      },
    },
    {
      key: 'amountCents',
      header: 'Amount',
      sortable: true,
      align: 'right',
      render: (p) => <span className="pay-amount">{formatMoney(p.amountCents, p.currency)}</span>,
    },
    {
      key: 'status',
      header: 'Status',
      sortable: true,
      searchValue: (p) => String(p.status).replace(/_/g, ' '),
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
      render: (p) => (
        <span
          className={
            p.status === 'PENDING' || p.status === 'AWAITING_CONFIRMATION' ? 'pay-pending' : undefined
          }
        >
          <StatusBadge status={p.status} />
        </span>
      ),
    },
    {
      key: 'createdAt',
      header: 'Created',
      sortable: true,
      render: (p) => new Date(p.createdAt * 1000).toLocaleDateString(undefined, { timeZone: activeTimeZone() }),
    },
    {
      key: 'actions',
      header: '',
      render: (p) => {
        const allowed = allowedTransitions(p.status)
        return (
          <span className="ui-row pay-row-actions">
            {allowed.includes('confirm') && (
              <Button size="sm" disabled={busy === p.apiId} onClick={() => setTarget({ payment: p, action: 'confirm' })}>
                Confirm
              </Button>
            )}
            {allowed.includes('reject') && (
              <Button
                size="sm"
                variant="outline"
                disabled={busy === p.apiId}
                onClick={() => setTarget({ payment: p, action: 'reject' })}
              >
                Reject
              </Button>
            )}
            {allowed.includes('refund') && (
              <Button
                size="sm"
                variant="ghost"
                disabled={busy === p.apiId}
                onClick={() => setTarget({ payment: p, action: 'refund' })}
              >
                Refund
              </Button>
            )}
            {allowed.includes('cancel') && (
              <Button size="sm" variant="ghost" disabled={busy === p.apiId} onClick={() => void run('cancel', p.apiId)}>
                Cancel
              </Button>
            )}
          </span>
        )
      },
    },
  ]

  return (
    <div className="dash" data-testid="payments-page">
      <PageHeader
        title="Payments"
        subtitle="Every confirmation is made by an authorised seller and is recorded in the audit log"
        actions={
          <Button size="sm" variant="outline" leading={<Plus size={14} />} onClick={() => setMethodOpen(true)}>
            Add payment method
          </Button>
        }
      />

      {error && (
        <Card>
          <p className="ui-muted pay-flat" role="alert">
            {error}
          </p>
        </Card>
      )}

      <Card
        title="Payment destinations"
        actions={<span className="ui-muted t-sm">{methods.length} active</span>}
      >
        {methods.length === 0 ? (
          <p className="ui-muted pay-empty-note">
            No destination is published yet, so a client has nowhere to pay. Add a bank account or
            wallet to start accepting manual payments.
          </p>
        ) : (
          <div className="pay-prices">
            {methods.map((m) => (
              <div key={m.id} className="ui-col pay-method">
                <span className="ui-strong">
                  {m.label}{' '}
                  <span className="ui-muted">
                    ({m.kind}
                    {m.global ? ', platform' : ''})
                  </span>
                </span>
                <span className="ui-muted t-sm2">
                  {m.details || 'No details recorded'}
                </span>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card
        title="Payments"
        actions={
          <span className="ui-row pay-head-actions">
            {pendingCount > 0 ? (
              <Badge tone="warning">{pendingCount} awaiting review</Badge>
            ) : (
              <span className="ui-muted t-sm">nothing needs a decision</span>
            )}
            <span className="ui-muted t-sm">· {visible.length} records</span>
          </span>
        }
      >
        <AsyncState
          status={error ? 'error' : loading && visible.length === 0 ? 'loading' : visible.length === 0 ? 'empty' : 'ready'}
          error={error}
          onRetry={() => void load()}
          skeleton={<Skeleton height={180} />}
          empty={
            <EmptyState
              icon={<CreditCard size={22} />}
              title="No payments recorded yet"
              description="A payment appears here when a client submits one from “Buy premium”, or when a reseller sells a plan."
            />
          }
        >
          <Table
            columns={COLUMNS}
            data={visible}
            rowKey={(r) => r.id}
            initialSort={{ key: 'id', dir: 'desc' }}
            searchable
            searchPlaceholder="Search payment #, reference or status…"
            caption="payments"
          />
        </AsyncState>
      </Card>

      <Dialog
        open={!!target}
        onClose={() => setTarget(null)}
        title={
          target?.action === 'confirm'
            ? 'Confirm payment received'
            : target?.action === 'reject'
              ? 'Reject this payment'
              : 'Refund this payment'
        }
        footer={
          <>
            <Button variant="ghost" onClick={() => setTarget(null)}>
              Close
            </Button>
            <Button
              onClick={() => target && void run(target.action, target.payment.apiId)}
              disabled={!!target && busy === target.payment.apiId}
            >
              {target?.action === 'confirm' ? 'Confirm payment' : target?.action === 'reject' ? 'Reject' : 'Refund'}
            </Button>
          </>
        }
      >
        <p className="flow-top-0">
          Payment <strong>#{target?.payment.apiId}</strong> —{' '}
          {target ? formatMoney(target.payment.amountCents, target.payment.currency) : ''}
          {target?.payment.reference ? ` (ref ${target.payment.reference})` : ' (no reference)'}.
        </p>
        <p className="ui-muted t-sm2">
          {target?.action === 'confirm'
            ? 'Confirm only after the money is received. This records your identity and cannot be undone — the only exit afterwards is a refund.'
            : target?.action === 'reject'
              ? 'Rejecting tells the client the payment was not accepted. It cannot be re-opened; they must create a new payment.'
              : 'Refunding is only available for a confirmed payment. It also cannot be reversed.'}
        </p>
      </Dialog>

      <Dialog
        open={methodOpen}
        onClose={() => setMethodOpen(false)}
        title="Add a payment destination"
        footer={
          <>
            <Button variant="ghost" onClick={() => setMethodOpen(false)}>
              Cancel
            </Button>
            <Button leading={<Wallet size={14} />} onClick={() => void addMethod()} disabled={!methodLabel.trim()}>
              Add method
            </Button>
          </>
        }
      >
        <div className="ui-col">
          <label className="ui-field">
            <span className="ui-field__label">Kind</span>
            <select
              className="ui-input"
              value={methodKind}
              onChange={(e) => setMethodKind(e.target.value as typeof methodKind)}
            >
              <option value="BANK">Bank transfer</option>
              <option value="EZCASH">eZ Cash</option>
              <option value="MOBILE">Mobile money</option>
              <option value="OTHER">Other</option>
            </select>
          </label>
          <Input label="Label" placeholder="e.g. Veylix Bank — Commercial" value={methodLabel} onChange={(e) => setMethodLabel(e.target.value)} />
          <Input label="Account details" placeholder="e.g. AC 0012 3456 7890" value={methodDetails} onChange={(e) => setMethodDetails(e.target.value)} />
          <p className="ui-muted t-sm2 pay-flat">
            A super admin publishes a method for the whole platform; an admin publishes one that
            only their own clients can see.
          </p>
        </div>
      </Dialog>
    </div>
  )
}

/** Integer cents → "USD 15.00". Money is formatted only at render time. */
function formatMoney(cents: number, currency: string): string {
  return `${currency} ${(cents / 100).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}

/**
 * How long a payment has been waiting on a decision, measured from the moment
 * it entered its CURRENT state: PENDING waits from creation (the client has not
 * paid yet), AWAITING_CONFIRMATION waits from the moment proof arrived (the
 * review the operator owes). A settled payment has no queue age — the chip
 * disappears the moment a decision exists, which is the reward for clearing it.
 */
function waitingSince(p: Payment): number | null {
  if (p.status === 'PENDING') return p.createdAt || null
  if (p.status === 'AWAITING_CONFIRMATION') return p.updatedAt || p.createdAt || null
  return null
}

/** Seconds → "40s" / "12m" / "3h" / "2d" — a queue age, not a date. */
function queueAge(unixSeconds: number): string {
  const delta = Math.max(0, Math.floor(Date.now() / 1000 - unixSeconds))
  if (delta < 60) return `${delta}s`
  if (delta < 3600) return `${Math.floor(delta / 60)}m`
  if (delta < 86_400) return `${Math.floor(delta / 3600)}h`
  return `${Math.floor(delta / 86_400)}d`
}
