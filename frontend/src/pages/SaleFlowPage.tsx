import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, PageHeader, StepProgress, useToast } from '../components/ui'
import { useAdminsStore } from '../stores/adminsStore'
import { useAuthStore } from '../stores/authStore'
import { useClientsStore } from '../stores/clientsStore'
import { useOffersStore } from '../stores/offersStore'
import { useVpnStore } from '../stores/vpnStore'
import './admins.css'

const METHODS = ['Bank transfer', 'eZ Cash', 'Mobile money']

/**
 * Sell a premium VPN — real clients and real offers (P5).
 *
 * Phase A follow-up: the demo client list and the invented price table are gone.
 * The client picker is `GET /api/clients` (scoped to this reseller) and the plan
 * picker is the active PREMIUM catalogue, so the only thing this flow needs is
 * an offer — exactly what `POST /api/vpns` requires.
 */
export function SaleFlowPage() {
  const user = useAuthStore((s) => s.user)
  const admins = useAdminsStore((s) => s.admins)
  const clients = useClientsStore((s) => s.clients)
  const loadClients = useClientsStore((s) => s.load)
  const offers = useOffersStore((s) => s.offers)
  const loadOffers = useOffersStore((s) => s.load)
  const assignPremium = useVpnStore((s) => s.assignPremium)
  const toast = useToast()
  const navigate = useNavigate()

  const me = admins.find((a) => a.email === (user?.email ?? ''))
  // The quota ceiling is real (from `GET /api/admins`); usage is not exposed
  // per-admin yet, so the remaining count cannot be computed honestly.
  const quotaMax = me?.quotaMax ?? 0

  const premiumOffers = offers.filter((o) => o.kind === 'PREMIUM' && o.active)

  useEffect(() => {
    void loadClients()
    void loadOffers()
  }, [loadClients, loadOffers])

  const [step, setStep] = useState<1 | 2 | 3>(1)
  const [clientId, setClientId] = useState('')
  const [offerId, setOfferId] = useState('')
  const [method, setMethod] = useState(METHODS[0])
  const [selling, setSelling] = useState(false)

  // Keep the pickers pointed at a real row as the lists load.
  const effectiveClientId = clientId || (clients[0] ? String(clients[0].apiId) : '')
  const effectiveOfferId = offerId || (premiumOffers[0] ? String(premiumOffers[0].apiId) : '')
  const client = clients.find((c) => String(c.apiId) === effectiveClientId)
  const offer = premiumOffers.find((o) => String(o.apiId) === effectiveOfferId)

  const finish = async (e: FormEvent) => {
    e.preventDefault()
    if (!user || !me) return
    if (!client || !effectiveOfferId) {
      toast('error', 'Nothing to sell yet', 'A client and an active premium offer are both required')
      return
    }
    setSelling(true)
    try {
      await assignPremium(Number(effectiveOfferId), client.apiId)
      toast('success', 'Premium assigned', `${offer?.name ?? 'Offer'} → ${client.name}`)
      setStep(3)
    } catch {
      toast(
        'error',
        'The panel did not confirm the assignment',
        'Nothing was created and no quota slot was consumed',
      )
    } finally {
      setSelling(false)
    }
  }

  return (
    <div className="dash" data-testid="sale-flow">
      <PageHeader
        title="Sell premium VPN"
        subtitle={
          quotaMax > 0
            ? `Quota ceiling ${quotaMax} assignments · a slot is consumed only after the panel confirms the client`
            : 'Quota ceiling unlimited · a slot is consumed only after the panel confirms the client'
        }
      />

      <StepProgress steps={['Client', 'Plan', 'Done']} current={step} />

      {step === 1 && (
        <Card title="1 · Client">
          {clients.length === 0 ? (
            <p className="ui-muted" style={{ margin: 0 }}>
              You have no clients yet. A client appears here once they own a VPN you
              sold — assign one from the Clients page first.
            </p>
          ) : (
            <div className="ui-col" style={{ gap: 8 }}>
              {clients.map((c) => (
                <button
                  key={c.apiId}
                  type="button"
                  className="ui-chart-legend__item"
                  data-off={effectiveClientId !== String(c.apiId)}
                  onClick={() => setClientId(String(c.apiId))}
                  style={{ justifyContent: 'flex-start' }}
                >
                  <strong>{c.name}</strong>
                  <span className="ui-muted t-xs">
                    {c.email} · {c.vpnCount} VPN(s)
                  </span>
                </button>
              ))}
            </div>
          )}
          <Button
            className="flow-top-3"
            disabled={clients.length === 0}
            onClick={() => setStep(2)}
          >
            Continue
          </Button>
        </Card>
      )}

      {step === 2 && (
        <Card
          title="2 · Offer"
          actions={<Button size="sm" variant="ghost" onClick={() => setStep(1)}>Back</Button>}
        >
          <form onSubmit={finish} className="ui-col">
            <label className="ui-field">
              <span className="ui-field__label">Premium offer</span>
              <select
                className="ui-input"
                value={effectiveOfferId}
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
            <label className="ui-field">
              <span className="ui-field__label">Payment method</span>
              <select className="ui-input" value={method} onChange={(e) => setMethod(e.target.value)}>
                {METHODS.map((m) => (
                  <option key={m}>{m}</option>
                ))}
              </select>
            </label>
            <div className="vpn-warn t-sm2">
              Payment collection is recorded by the seller in the Payments screen. This flow
              assigns the VPN and reports what the panel confirmed — it does not invent a price
              the platform cannot verify.
            </div>
            <Button type="submit" disabled={selling || !client || !effectiveOfferId}>
              {selling ? 'Provisioning…' : 'Assign VPN'}
            </Button>
          </form>
        </Card>
      )}

      {step === 3 && (
        <Card title="3 · Done">
          <div className="ui-col" style={{ gap: 10 }}>
            <strong>Premium assigned</strong>
            <span className="ui-muted t-base">
              {client?.name ?? 'The client'} now owns {offer?.name ?? 'the offer'}. Usage
              appears once the background sync reads the panel; the connection link is
              available on the VPN detail screen, fetched live from the panel.
            </span>
            <div className="ui-row">
              <Button size="sm" variant="outline" onClick={() => navigate('/admin-clients')}>Back to clients</Button>
              <Button size="sm" variant="ghost" onClick={() => setStep(1)}>Sell another</Button>
            </div>
          </div>
        </Card>
      )}
    </div>
  )
}

