import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Activity, TrendingUp } from 'lucide-react'
import { Button, EmptyState } from '../../components/ui'
import { ClientPage, ClientSection } from '../../components/client/ClientPage'
import { useVpnStore, vpnDisplayName } from '../../stores/vpnStore'
import '../../components/client/client.css'

/**
 * Usage & History — client shell (S8 outer layer).
 *
 * SCOPE OF THIS FILE: the frame only. It states where the client is and lists
 * one link per owned VPN, because per-VPN usage is the only usage the API
 * actually exposes to a client.
 *
 * WHAT THE API HONESTLY OFFERS TODAY
 *
 *   `GET /api/vpns/{id}/usage?range=7d|30d|3m|1y|all` returns one VPN's series
 *   plus its own `availability` flag (PARTIAL | AVAILABLE | NONE). It is
 *   per-VPN and accepts a range — that is exactly the data a chart needs.
 *
 *   There is NO account-wide usage endpoint, and NO cross-VPN activity/history
 *   feed. So this screen does not draw an aggregate chart: summing per-VPN
 *   series in the browser would produce a number the backend never stated, and
 *   a usage figure is precisely the kind of value the UI must not invent.
 *
 *   Consequently this shell lists the owned VPNs and sends the reader to the
 *   VPN's own detail screen, which already renders the real usage chart with
 *   the server's own window switch. That is the working path today; an
 *   aggregated view is the S8 stage's decision, not something to fake here.
 *
 * The list is NOT filtered locally: `GET /api/vpns` is already scoped to the
 * caller's own rows by the server.
 */
export function ClientUsagePage() {
  const navigate = useNavigate()
  const vpns = useVpnStore((s) => s.vpns)
  const loading = useVpnStore((s) => s.loading)
  const load = useVpnStore((s) => s.load)

  useEffect(() => {
    void load()
  }, [load])

  return (
    <ClientPage
      testId="client-usage"
      title="Usage & History"
      description="How much data each of your VPNs has used"
      actions={
        <Button size="sm" onClick={() => navigate('/client/vpns')}>
          My VPNs
        </Button>
      }
    >
      <ClientSection
        title="Per-VPN usage"
        actions={
          !loading ? <span className="ui-muted t-sm">{vpns.length} VPN{vpns.length === 1 ? '' : 's'}</span> : undefined
        }
      >
        {vpns.length === 0 && !loading ? (
          <EmptyState
            icon={<Activity size={22} />}
            title="No VPNs to measure yet"
            description="Usage is recorded per VPN, so there is nothing to chart until you own one. Claim a free VPN from your dashboard or buy a premium package."
            action={
              <div className="ui-row" style={{ gap: 8 }}>
                <Button size="sm" variant="primary" onClick={() => navigate('/client')}>
                  Claim a free VPN
                </Button>
                <Button size="sm" variant="outline" onClick={() => navigate('/buy')}>
                  Buy premium
                </Button>
              </div>
            }
          />
        ) : (
          <div className="ui-col" style={{ gap: 8 }}>
            <p className="ui-muted t-sm2" style={{ margin: 0 }}>
              Usage is measured per VPN, from the panel. Open a VPN to see its own chart and switch
              between 7 days, 30 days, 3 months and 1 year — the range the server records.
            </p>
            {vpns.map((v) => (
              <div
                key={v.id}
                className="ui-row"
                style={{ gap: 10, alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap' }}
              >
                <span className="ui-row" style={{ gap: 8, alignItems: 'center' }}>
                  <TrendingUp size={15} aria-hidden="true" style={{ color: 'var(--text-muted)' }} />
                  <span className="ui-strong">{vpnDisplayName(v)}</span>
                </span>
                <Button size="sm" variant="outline" onClick={() => navigate(`/vpn/${v.id}`)}>
                  View usage
                </Button>
              </div>
            ))}
          </div>
        )}
      </ClientSection>
    </ClientPage>
  )
}
