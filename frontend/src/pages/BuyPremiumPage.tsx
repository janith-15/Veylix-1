import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { CheckCheck } from 'lucide-react'
import { Button, Card, PageHeader, Input, StepProgress, useToast } from '../components/ui'
import { usePaymentsStore } from '../stores/paymentsStore'
import { useOffersStore, type VpnOffer } from '../stores/offersStore'
import { formatMoney } from '../lib/format'

/**
 * Buy a premium VPN — manual payment flow (REQUIREMENTS §2.5).
 *
 * Phase 5: this page used to offer two invented plans ("Premium 50 GB — sold by
 * Nimal Silva") and three invented payment methods, then "confirm" the order
 * locally. Neither the plans nor the confirmation existed anywhere else.
 *
 * Both lists are now real:
 *  - plans come from the offer catalogue (`GET /api/offers`), so the buyer only
 *    ever sees an offer the backend actually published;
 *  - destinations come from `GET /api/payment-methods` (global + own).
 *
 * The amount is typed in whole currency units and converted to integer cents at
 * the boundary — the wire never carries a float.
 */
export function BuyPremiumPage() {
  const navigate = useNavigate()
  const toast = useToast()

  const offers = useOffersStore((s) => s.offers)
  const loadOffers = useOffersStore((s) => s.loadPremium)
  const createPayment = usePaymentsStore((s) => s.createPayment)
  const methods = usePaymentsStore((s) => s.methods)
  const loadMethods = usePaymentsStore((s) => s.loadMethods)

  const [step, setStep] = useState<1 | 2 | 3>(1)
  const [offerId, setOfferId] = useState<number | null>(null)
  const [methodId, setMethodId] = useState<number | null>(null)
  const [amount, setAmount] = useState('')
  const [note, setNote] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void loadOffers()
    void loadMethods()
  }, [loadOffers, loadMethods])

  const plan: VpnOffer | null = offers.find((o) => o.apiId === offerId) ?? null
  const method = methods.find((m) => m.id === methodId) ?? null

  const completePurchase = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    const units = Number.parseFloat(amount)
    if (!Number.isFinite(units) || units <= 0) {
      setError('Enter the amount you are paying, greater than zero.')
      return
    }
    /*
     * A plan chosen in step 1 MUST travel with the order. Without offer_id the
     * server records a deposit, confirmation releases no product, and the buyer
     * pays for a VPN that is never built (D-031.1). Refusing here is better than
     * sending a payment that cannot be fulfilled.
     */
    if (offerId === null) {
      setError('Choose a plan first, so the seller knows which product to deliver.')
      setStep(1)
      return
    }
    setSubmitting(true)
    try {
      await createPayment({
        // Integer cents at the boundary — never a float on the wire.
        amountCents: Math.round(units * 100),
        methodId: methodId ?? undefined,
        reference: note.trim(),
        // The product, not just the money: this is what the confirmation builds.
        offerId,
      })
      setStep(3)
      toast('success', 'Order recorded', 'The seller confirms once your payment arrives')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not record the order')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="dash" data-testid="buy-premium">
      <PageHeader
        title="Buy premium"
        subtitle="Pick a plan, pay the seller directly, and they confirm your order"
      />

      {/* The step rail only makes sense once there is something to choose.
          With no published plan the flow cannot start, so showing "Step 1 of 3"
          pointed at a step that had no controls (I-40). */}
      {offers.length > 0 && (
        <StepProgress
          steps={['Choose a plan', 'Payment', 'Submitted']}
          current={step}
        />
      )}

      {step === 1 && (
        <Card title="1 · Choose a plan">
          {offers.length === 0 ? (
            // The copy was honest but the page was a dead end: no plan, no
            // controls, nowhere to go. Offer the two things a customer can
            // actually do while they wait for one to be published (I-40).
            <div className="ui-col" style={{ gap: 10 }} data-testid="buy-empty">
              <span className="ui-strong">No premium plans are published yet</span>
              <span className="ui-muted t-sm2" style={{ margin: 0 }}>
                The seller has to create an offer before it can be bought. Your free VPNs still
                work — and you can ask the seller directly when Premium will be available.
              </span>
              <div className="ui-row" style={{ gap: 8 }}>
                <Button size="sm" onClick={() => navigate('/client')}>
                  Go to my VPNs
                </Button>
                <Button variant="outline" size="sm" onClick={() => navigate('/chat')}>
                  Ask the seller
                </Button>
              </div>
            </div>
          ) : (
            <div className="client-offers-grid">
              {offers.map((p) => (
                <Card key={p.id} className="client-vpn-card">
                  <div className="client-vpn-card__top">
                    <span className="ui-strong">{p.name}</span>
                    {/* The price is the seller's, never the buyer's: it comes from
                        the catalogue (D-047) and a plan without one says so
                        instead of inviting the buyer to invent a number. */}
                    <span className="client-price">
                      {p.priceCents > 0
                        ? formatMoney(p.priceCents, p.currency)
                        : 'Price on request'}
                    </span>
                  </div>
                  <span className="ui-muted t-sm2">
                    {p.kind === 'FREE' ? 'Free' : 'Premium'} ·{' '}
                    {p.trafficLimitGB > 0 ? `${p.trafficLimitGB} GB` : 'unlimited traffic'} ·{' '}
                    {p.expiryDays > 0 ? `${p.expiryDays} days` : 'no expiry'} ·{' '}
                    {p.deviceLimit > 0 ? `${p.deviceLimit} devices` : 'unlimited devices'}
                  </span>
                  <Button
                    size="sm"
                    variant={offerId === p.apiId ? 'primary' : 'outline'}
                    onClick={() => {
                      setOfferId(p.apiId)
                      setMethodId(null)
                      setAmount(
                        p.priceCents > 0 ? (p.priceCents / 100).toFixed(2) : '',
                      )
                      setStep(2)
                    }}
                  >
                    {offerId === p.apiId ? 'Selected' : 'Select plan'}
                  </Button>
                </Card>
              ))}
            </div>
          )}
        </Card>
      )}

      {step === 2 && (
        <Card
          title="2 · Payment instructions"
          actions={
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setStep(1)
                // Clear the chosen plan so step 1 is a neutral choice again, not a
                // preselected card the buyer has to notice and undo.
                setOfferId(null)
              }}
            >
              Back
            </Button>
          }
        >
          <form onSubmit={completePurchase} className="ui-col">
            {/*
              * U11 (UI/UX defect board). Step 2 used to stack three separate
              * warn blocks (method details, "no destinations", the how-it-works
              * note) plus two field hints, so the form read as a wall of
              * paragraphs and the actual instruction was lost in the middle.
              * There is now ONE instruction line at the top saying what to do,
              * and the blocks below it carry only the facts that block progress.
              */}
            <p className="ui-muted t-sm2" style={{ margin: 0 }}>
              Transfer the amount, then submit. The seller confirms once the money arrives —
              until then the order shows as <strong>pending</strong>.
            </p>

            <div className="vpn-chips" style={{ marginBottom: 6 }}>
              <span className="vpn-chip">{plan?.name ?? 'Offer'}</span>
              <span className="vpn-chip">
                {plan?.trafficLimitGB ? `${plan.trafficLimitGB} GB` : 'Unlimited traffic'}
              </span>
              {plan && plan.priceCents > 0 && (
                <span className="vpn-chip">{formatMoney(plan.priceCents, plan.currency)}</span>
              )}
            </div>

            <label className="ui-field">
              <span className="ui-field__label">Payment method</span>
              <select
                className="ui-input"
                value={methodId ?? ''}
                onChange={(e) => setMethodId(e.target.value ? Number(e.target.value) : null)}
              >
                <option value="">Select a destination…</option>
                {methods.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.label} ({m.kind}){m.global ? '' : ' — seller'}
                  </option>
                ))}
              </select>
            </label>

            {method && (method.details || method.instructions) && (
              <div className="vpn-warn t-sm2">
                {method.details && <strong>{method.details}</strong>}
                {method.details && method.instructions ? ' — ' : ''}
                {method.instructions}
              </div>
            )}
            {methods.length === 0 && (
              <div className="vpn-warn t-sm2">
                No payment destinations are published yet, so this order would have nowhere to
                go. Ask your seller to add one under Payments.
              </div>
            )}

            <Input
              label="Amount you are paying"
              type="number"
              placeholder="e.g. 1500"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              // A priced plan fixes the amount: letting the buyer retype it would
              // let them pay less than the catalogue says. Only an unpriced plan
              // ("Price on request") leaves the field open.
              readOnly={Boolean(plan && plan.priceCents > 0)}
              hint={
                plan && plan.priceCents > 0
                  ? `Fixed by the plan: ${formatMoney(plan.priceCents, plan.currency)}`
                  : 'Enter what you transferred — the seller checks it against the catalogue'
              }
            />
            <Input
              label="Payment reference / note (optional)"
              placeholder="e.g. slip #4471"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              hint="A bank reference makes confirmation faster"
            />

            {error && (
              <div className="vpn-warn t-sm2" role="alert">
                {error}
              </div>
            )}

            <Button type="submit" leading={<CheckCheck size={14} />} disabled={submitting || !plan}>
              {submitting ? 'Recording…' : 'Submit order'}
            </Button>
          </form>
        </Card>
      )}

      {step === 3 && (
        <Card title="3 · Order submitted">
          <div className="ui-col" style={{ gap: 10 }}>
            <span className="ui-row" style={{ gap: 8 }}>
              <CheckCheck size={18} aria-hidden="true" style={{ color: 'var(--success)' }} />
              <strong>
                Order pending — {plan?.name ?? 'Premium'} ({method?.label ?? 'no method'})
              </strong>
            </span>
            <p className="ui-muted t-base" style={{ margin: 0 }}>
              The seller confirms your payment. You'll see it under Purchases, and the VPN
              activates right after confirmation.
            </p>
            <div className="ui-row">
              <Button variant="outline" size="sm" onClick={() => navigate('/history')}>
                View purchases
              </Button>
              <Button variant="ghost" size="sm" onClick={() => navigate('/chat')}>
                Message seller
              </Button>
            </div>
            {/* Submitting twice bills twice; the only safe way back to the start
                of the flow is an explicit "buy another", never a Back arrow that
                would imply the order can be un-sent. */}
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setStep(1)
                setOfferId(null)
                setMethodId(null)
                setAmount('')
                setNote('')
              }}
            >
              Buy another plan
            </Button>
          </div>
        </Card>
      )}
    </div>
  )
}
