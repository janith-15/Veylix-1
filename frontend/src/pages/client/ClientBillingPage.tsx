import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { CreditCard, ExternalLink, Receipt, ShieldCheck } from 'lucide-react'
import { Button, EmptyState, Skeleton } from '../../components/ui'
import { ClientPage, ClientSection } from '../../components/client/ClientPage'
import { usePaymentsStore } from '../../stores/paymentsStore'
import { useVpnStore } from '../../stores/vpnStore'
import '../../components/client/client.css'
import './billing.css'

/**
 * Payments & Billing — client (S7).
 *
 * WHY THIS SCREEN CHANGED
 *
 *   The client sidebar carried `Payments & Billing` and `Purchases` side by
 *   side. They read the SAME payment records: `/history` drew the full
 *   statement, and this page spent 117 lines telling the reader to go there.
 *   Two destinations for one fact, and the one the customer picked first was a
 *   dead end.
 *
 *   The screen now does the two things a billing screen is for, on one page:
 *
 *     1. WHAT IS WAITING — the payment records that still need a human
 *        decision, with amount, reference and age.
 *     2. WHAT I HAVE BOUGHT — a link to the full statement on `/history`,
 *        whose route stays so bookmarks and e-mail links keep working.
 *
 * WHAT THIS SCREEN DOES NOT DO
 *
 *   It does not confirm, reject, cancel or refund a payment. It does not upload
 *   proof-of-payment and it does not download a receipt: no endpoint exists for
 *   either yet (`lib/api.ts` has `listPayments` and nothing else here). A button
 *   that cannot complete is the false affordance this programme removes, so the
 *   absence is stated in words instead of drawn as a control.
 *
 *   `GET /api/payments` is scoped to the caller by the STORE
 *   (`payer_user_id = ?`), never by a query string, so this read cannot surface
 *   another payer's rows.
 */

/** The two statuses that still need a human decision. Everything else is final. */
const OPEN: ReadonlyArray<string> = ['PENDING', 'AWAITING_CONFIRMATION']

/** Integer cents → "LKR 1,500.00". Money is formatted only at render time. */
function formatMoney(cents: number, currency: string): string {
  return `${currency} ${(cents / 100).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}

/** "today" / "1 day ago" / "N days ago" - never a raw epoch. */
function sinceLabel(createdAt: number, nowSec: number): string {
  const days = Math.floor((nowSec - createdAt) / 86400)
  if (days <= 0) return 'today'
  return days === 1 ? '1 day ago' : `${days} days ago`
}

export function ClientBillingPage() {
  const navigate = useNavigate()
  const payments = usePaymentsStore((s) => s.payments)
  const loading = usePaymentsStore((s) => s.loading)
  const error = usePaymentsStore((s) => s.error)
  const load = usePaymentsStore((s) => s.load)

  /*
   * The VPN list is read too, so "no payments yet" can be told apart from
   * "an account with nothing on it". Without it the empty state would offer
   * "Buy premium" to a customer whose only blocker is a read failure.
   */
  const vpns = useVpnStore((s) => s.vpns)
  const vpnsLoading = useVpnStore((s) => s.loading)
  const loadVpns = useVpnStore((s) => s.load)

  /* One clock read per mount, so every "N days ago" agrees with every other. */
  const [nowSec] = useState(() => Date.now() / 1000)

  useEffect(() => {
    void load()
    void loadVpns()
  }, [load, loadVpns])

  const open = payments.filter((p) => OPEN.includes(p.status))
  const settledCount = payments.length - open.length

  return (
    <ClientPage
      testId="client-billing"
      title="Payments & Billing"
      description="What is still waiting, and where your full statement lives"
      actions={
        <Button size="sm" leading={<CreditCard size={14} />} onClick={() => navigate('/buy')}>
          Buy premium
        </Button>
      }
    >
      {/* ---------- 1 . WAITING FOR CONFIRMATION ------------------------ */}
      <ClientSection
        title="Waiting for confirmation"
        actions={
          !loading && !error ? (
            <span className="ui-muted t-sm">
              {open.length === 0 ? 'nothing waiting' : `${open.length} open`}
            </span>
          ) : undefined
        }
      >
        {loading && <Skeleton height={90} />}

        {error && (
          /* A failed read is never turned into "0 records": "none" and "unknown"
             are different facts (D-090). */
          <EmptyState
            icon={<Receipt size={22} />}
            title="Could not load your payments"
            description="This is a connection problem, not an empty account. Your payments are safe - try again, or open the full statement on Purchases."
            action={
              <div className="bil__actions">
                <Button size="sm" variant="primary" onClick={() => void load()}>
                  Try again
                </Button>
                <Button size="sm" variant="ghost" onClick={() => navigate('/history')}>
                  Open Purchases
                </Button>
              </div>
            }
          />
        )}

        {!loading && !error && open.length === 0 && (
          <div className="bil__calm" data-testid="billing-none-waiting">
            <span className="bil__calm-title">
              <ShieldCheck size={16} aria-hidden="true" />
              {payments.length === 0 ? 'No payments yet' : 'Nothing is waiting'}
            </span>
            <p className="bil__calm-note">
              {payments.length === 0
                ? 'A payment appears here the moment you buy a premium package. A free VPN never produces one.'
                : `All ${settledCount} of your payment record${settledCount === 1 ? ' is' : 's are'} settled - no action from you is needed.`}
            </p>
            {payments.length === 0 && !vpnsLoading && (
              <div className="bil__actions">
                <Button size="sm" variant="primary" onClick={() => navigate('/client')}>
                  {vpns.length === 0 ? 'Claim a free VPN' : 'Back to my VPNs'}
                </Button>
                <Button size="sm" variant="outline" onClick={() => navigate('/buy')}>
                  Buy premium
                </Button>
              </div>
            )}
          </div>
        )}

        {!loading && !error && open.length > 0 && (
          <ul className="bil__list">
            {open.map((p) => (
              <li className="bil__row" key={p.apiId} data-testid={`billing-open-${p.apiId}`}>
                <div className="bil__row-main">
                  <span className="bil__row-amount">
                    {formatMoney(p.amountCents, p.currency)}
                  </span>
                  <span className="bil__row-meta">
                    {p.reference ? `${p.reference} · ` : ''}
                    {sinceLabel(p.createdAt, nowSec)}
                  </span>
                </div>
                <span className="bil__row-status">{p.status.replace(/_/g, ' ').toLowerCase()}</span>
              </li>
            ))}
          </ul>
        )}
      </ClientSection>

      {/* ---------- 2 . THE FULL STATEMENT -------------------------------
        * The complete record table - every payment and every VPN, with its
        * status and creation date, plus the per-order timeline - already
        * exists on `/history`. It is LINKED, never copied: duplicating that
        * table here is exactly the defect this screen was rewritten to remove.
        * ------------------------------------------------------------------ */}
      <ClientSection title="Full statement">
        <div className="bil__statement" data-testid="billing-statement-link">
          <p className="bil__statement-note">
            Every payment and every VPN you have ever held, with each order&rsquo;s timeline, lives
            on Purchases. Nothing is deleted from it - not even when a server becomes unavailable.
          </p>
          <div className="bil__actions">
            <Button
              size="sm"
              variant="outline"
              leading={<ExternalLink size={13} />}
              onClick={() => navigate('/history')}
            >
              Open Purchases
            </Button>
            <span className="bil__statement-count">
              {payments.length} payment record{payments.length === 1 ? '' : 's'} ·{' '}
              {vpns.length} VPN{vpns.length === 1 ? '' : 's'}
            </span>
          </div>
        </div>
      </ClientSection>
    </ClientPage>
  )
}
