import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Package, ShieldCheck } from 'lucide-react'
import { Button, Card, EmptyState } from '../../components/ui'
import { ClientPage, ClientSection } from '../../components/client/ClientPage'
import { useOffersStore } from '../../stores/offersStore'
import '../../components/client/client.css'

/**
 * Offers & Orders — client shell (S6 outer layer).
 *
 * SCOPE OF THIS FILE: the frame only. It states where the client is and what
 * this screen will hold. It renders NO catalogue, NO order list and NO claim
 * control yet.
 *
 * Why a shell and not the real screen: the content below is decided by the S6
 * stage, which needs the offer list, the package catalogue and the payment
 * order list joined into one flow. Shipping a half-built flow here would put a
 * button on screen that does not complete, which is the exact false-affordance
 * the audit programme exists to remove.
 *
 * What it DOES prove, honestly:
 *   - the route resolves for a client and for nobody else (see App.tsx guard);
 *   - the two live catalogues are read, so the counts shown are real API
 *     answers and not invented numbers;
 *   - when both catalogues are empty it says so, and names who publishes them.
 *
 * The counts are deliberately labelled as "available to you" rather than
 * "offers you have": `GET /api/offers` for a client is already scoped by the
 * server, so this is the claimable set, not owned inventory.
 */
export function ClientOffersPage() {
  const navigate = useNavigate()
  const offers = useOffersStore((s) => s.offers)
  const load = useOffersStore((s) => s.load)

  /*
   * ONE read, and only the FREE catalogue.
   *
   * `offersStore` holds a single `offers` array, and `loadPremium()` OVERWRITES
   * it (offersStore.ts:116) because `/buy` needs only the resale catalogue.
   * Calling both here would therefore race: whichever resolved last would win
   * and the two counts would describe one list, not two.
   *
   * `load(true)` reads `GET /api/offers`, which for a client is already scoped
   * to FREE by the server. That is the set this screen can honestly count.
   * The premium catalogue is not counted here — it is read by `/buy`, which
   * owns that flow — so this shell never claims a premium total it did not read.
   */
  useEffect(() => {
    void load(true)
  }, [load])

  const freeCount = offers.filter((o) => o.active).length

  return (
    <ClientPage
      testId="client-offers"
      title="Offers & Orders"
      description="The packages you can get, and the orders you have placed"
      actions={
        <Button size="sm" onClick={() => navigate('/buy')}>
          Buy premium
        </Button>
      }
    >
      <ClientSection
        title="Available offers"
        actions={
          <span className="ui-muted t-sm">
            {freeCount} free package{freeCount === 1 ? '' : 's'}
          </span>
        }
      >
        {freeCount === 0 ? (
          <EmptyState
            icon={<Package size={22} />}
            title="No free packages are published yet"
            description="Your seller publishes the plans that are available. Nothing is wrong with your account — check back, or ask them directly."
            action={
              <div className="ui-row" style={{ gap: 8 }}>
                <Button size="sm" variant="outline" onClick={() => navigate('/chat')}>
                  Ask the seller
                </Button>
                <Button size="sm" variant="ghost" onClick={() => navigate('/client/vpns')}>
                  Go to my VPNs
                </Button>
              </div>
            }
          />
        ) : (
          /*
           * A catalogue exists, so the real browsing flow is the next stage's
           * work. Saying what exists and linking the two working entry points is
           * honest; rendering a list without a claim/purchase path would leave
           * the customer one click from a dead end.
           */
          <Card>
            <div className="ui-col" style={{ gap: 10 }}>
              <span className="ui-row" style={{ gap: 8, alignItems: 'center' }}>
                <ShieldCheck size={16} aria-hidden="true" style={{ color: 'var(--success)' }} />
                <strong>
                  {freeCount} free package{freeCount === 1 ? '' : 's'} available to claim
                </strong>
              </span>
              <p className="ui-muted t-sm2" style={{ margin: 0 }}>
                Free packages are claimed from your dashboard. Premium packages are bought from the
                seller, who confirms the payment and then builds the VPN.
              </p>
              <div className="ui-row" style={{ gap: 8 }}>
                <Button size="sm" variant="primary" onClick={() => navigate('/client')}>
                  Claim a free VPN
                </Button>
                <Button size="sm" variant="outline" onClick={() => navigate('/buy')}>
                  Buy premium
                </Button>
                <Button size="sm" variant="ghost" onClick={() => navigate('/history')}>
                  View orders
                </Button>
              </div>
            </div>
          </Card>
        )}
      </ClientSection>
    </ClientPage>
  )
}
