import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Receipt } from 'lucide-react'
import { Button, EmptyState } from '../../components/ui'
import { ClientPage, ClientSection } from '../../components/client/ClientPage'
import { usePaymentsStore } from '../../stores/paymentsStore'
import '../../components/client/client.css'

/**
 * Payments & Billing — client shell (S7 outer layer).
 *
 * SCOPE OF THIS FILE: the frame only. It names the screen and states, from a
 * real API read, whether the account has any payment records at all.
 *
 * `GET /api/payments` is scoped to the caller by the STORE (`payer_user_id = ?`),
 * never by a query string, so this read cannot surface another payer's rows.
 *
 * TWO HONEST LIMITATIONS, stated rather than hidden:
 *
 *   1. The list is capped at 200 rows server-side, so the count below is
 *      worded as a presence check ("you have N record(s)") rather than a total
 *      for the lifetime of the account.
 *   2. There is no receipt download and no proof-of-payment upload endpoint in
 *      the product yet. Rather than draw a "Download receipt" button that does
 *      nothing, this shell says so. A control that cannot complete is the
 *      false affordance the audit programme removes.
 *
 * The detailed statement — pending, confirmed, rejected, refunded, with each
 * order's timeline — already exists on `/history` and is linked, not duplicated.
 */
export function ClientBillingPage() {
  const navigate = useNavigate()
  const payments = usePaymentsStore((s) => s.payments)
  const loading = usePaymentsStore((s) => s.loading)
  const error = usePaymentsStore((s) => s.error)
  const load = usePaymentsStore((s) => s.load)

  useEffect(() => {
    void load()
  }, [load])

  const pending = payments.filter(
    (p) => p.status === 'PENDING' || p.status === 'AWAITING_CONFIRMATION',
  ).length

  return (
    <ClientPage
      testId="client-billing"
      title="Payments & Billing"
      description="Every payment you have made, and what is still waiting"
      actions={
        <Button size="sm" onClick={() => navigate('/buy')}>
          Buy premium
        </Button>
      }
    >
      <ClientSection
        title="Your payments"
        actions={
          !loading && !error ? (
            <span className="ui-muted t-sm">
              {payments.length} record{payments.length === 1 ? '' : 's'}
              {pending > 0 ? ` · ${pending} waiting` : ''}
            </span>
          ) : undefined
        }
      >
        {error ? (
          /* A failed read is never turned into "0 records": "none" and
             "unknown" are different facts (D-090). */
          <EmptyState
            icon={<Receipt size={22} />}
            title="Could not load your payments"
            description="This is a connection problem, not an empty account. Try again, or open Purchases which reads the same records."
            action={
              <div className="ui-row" style={{ gap: 8 }}>
                <Button size="sm" variant="primary" onClick={() => void load()}>
                  Try again
                </Button>
                <Button size="sm" variant="ghost" onClick={() => navigate('/history')}>
                  Open Purchases
                </Button>
              </div>
            }
          />
        ) : payments.length === 0 && !loading ? (
          <EmptyState
            icon={<Receipt size={22} />}
            title="No payments yet"
            description="Payments appear here once you buy a premium package. A free VPN never produces a payment."
            action={
              <div className="ui-row" style={{ gap: 8 }}>
                <Button size="sm" variant="primary" onClick={() => navigate('/buy')}>
                  Buy premium
                </Button>
                <Button size="sm" variant="ghost" onClick={() => navigate('/client')}>
                  Claim a free VPN
                </Button>
              </div>
            }
          />
        ) : (
          <EmptyState
            icon={<Receipt size={22} />}
            title="Your payment records are ready on Purchases"
            description="Full detail — each order's status, its reference and its timeline — lives on Purchases today. Receipts and proof-of-payment upload are not available in this build yet."
            action={
              <Button size="sm" variant="outline" onClick={() => navigate('/history')}>
                Open Purchases
              </Button>
            }
          />
        )}
      </ClientSection>
    </ClientPage>
  )
}
