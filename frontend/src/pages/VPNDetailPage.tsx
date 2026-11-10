import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, RefreshCw } from 'lucide-react'
import { Button, Card, PageHeader, StatusBadge } from '../components/ui'
import { VpnKindBadge } from '../components/vpn/VpnKindBadge'
import { PanelIdentityRow } from '../components/vpn/PanelIdentityRow'
import { VpnUsageCards } from '../components/vpn/VpnUsageCards'
import { VpnUsageSection } from '../components/vpn/VpnUsageSection'
import { VpnConnectSection } from '../components/vpn/VpnConnectSection'
import { useVpnActions } from '../components/vpn/useVpnActions'
import { useAuthStore } from '../stores/authStore'
import { api, type ApiVpnUsage } from '../lib/api'
import { statusNote, useVpnStore, vpnDisplayName, vpnServerLabel, type ClientVpn } from '../stores/vpnStore'
import './client.css'
import './vpn-detail.css'
import { activeTimeZone } from '../lib/time'

/**
 * The VPN details screen (owner's brief §6).
 *
 * THREE SECTIONS, NOT EIGHT — and that is the whole point.
 *
 * The previous screen stacked every panel it could find: a hero band, an `h2`
 * "Usage" over a chart block, an `h2` "Connection" whose card was ALSO titled
 * "Connection", a protocol block with no heading at all, a "Keep this VPN going"
 * card, a "Support" card, and 23 `style={{ … }}` props. A customer had no
 * outline to follow and no idea what to read first.
 *
 * This screen answers questions in the order a customer asks them:
 *
 *   1. **Overview**           — is it working, and how much do I have left?
 *   2. **Usage**              — what has it been doing?          (D-092 charts)
 *   3. **Connection & Help**  — how do I connect, and what if it will not?
 *
 * ONE HIERARCHY, ONCE
 *
 * The section names are `<h2>`. Inside a section, `Card` titles describe their
 * CONTENT. No word appears at two levels — the duplicate "Connection" pair is
 * gone because the section heading says "Connection & Help" while the card holds
 * the connection itself.
 *
 * NOTHING IS GUESSED
 *
 * Account status is not device connectivity: `ACTIVE` means the panel reports
 * this client, and the page says so in those words rather than drawing a green
 * "Connected" light it cannot back up. A non-ACTIVE status shows the sentence
 * from `statusNote`, never a bare enum.
 */

type SectionKey = 'overview' | 'usage' | 'connect'

const SECTIONS: { key: SectionKey; label: string }[] = [
  { key: 'overview', label: 'Overview' },
  { key: 'usage', label: 'Usage' },
  { key: 'connect', label: 'Connection & Help' },
]

function daysLeft(expiresAt: number | null, nowSec: number): string | null {
  if (expiresAt === null) return null
  const days = Math.ceil((expiresAt - nowSec) / 86400)
  if (days < 0) return `expired ${Math.abs(days)} day${Math.abs(days) === 1 ? '' : 's'} ago`
  if (days === 0) return 'expires today'
  return `${days} day${days === 1 ? '' : 's'} left`
}

function gb(bytes: number): string {
  const v = bytes / 2 ** 30
  return `${v >= 10 ? v.toFixed(1) : v.toFixed(2)} GB`
}

/**
 * The staff-only lifecycle controls (suspend / resume / revoke / release), as a
 * child component purely so `useVpnActions` receives a non-null VPN — a hook
 * cannot be called conditionally, and the missing-VPN branch returns early.
 */
function VpnLifecycleActions({ vpn }: { vpn: ClientVpn }) {
  const lifecycle = useVpnActions({ vpn })
  return (
    <>
      {lifecycle.actions}
      {lifecycle.dialogs}
    </>
  )
}

export function VPNDetailPage() {
  const { id, serverId: serverIdParam, panelEmail } = useParams()
  const navigate = useNavigate()
  const vpns = useVpnStore((s) => s.vpns)
  const vpnsLoading = useVpnStore((s) => s.loading)
  const load = useVpnStore((s) => s.load)
  const wake = useVpnStore((s) => s.wake)
  const role = useAuthStore((s) => s.user?.role ?? 'client')
  const isStaff = role === 'super-admin' || role === 'admin'

  const [section, setSection] = useState<SectionKey>(() => {
    /* A `?section=` deep link lands on that section. The Quick View's
       "Connect now" uses it, so the button takes the customer to the connection
       rather than to a page they must then navigate. An unknown value falls back
       to Overview rather than rendering nothing. */
    const want = new URLSearchParams(window.location.search).get('section')
    return want === 'connect' || want === 'usage' || want === 'overview' ? want : 'overview'
  })
  const [usage, setUsage] = useState<ApiVpnUsage | null>(null)
  const [usageError, setUsageError] = useState<string | null>(null)

  /* One clock read per mount (D-066): a re-render must not move "days left". */
  const [nowSec] = useState(() => Date.now() / 1000)

  useEffect(() => {
    void load()
  }, [load])

  /*
   * Two ways in: `/vpn/:id` (the client's own screen) and
   * `/vpns/:serverId/:panelEmail` (the Super Admin console, which has no local
   * id for a panel-only client). The panel email is matched INSIDE its server.
   */
  const decodedEmail = panelEmail ? decodeURIComponent(panelEmail) : null
  const routeServerID = serverIdParam ? Number(serverIdParam) : null

  const vpn =
    id !== undefined
      ? (vpns.find((v) => String(v.apiId) === id) ?? null)
      : decodedEmail && routeServerID !== null
        ? (vpns.find((v) => v.panelEmail === decodedEmail && v.serverId === routeServerID) ?? null)
        : null

  const vpnId = vpn?.apiId ?? null

  /* The Overview shows the two direction totals, which the `?summary=1` response
     does not include — so this asks for a bounded history and keeps the totals.
     `limit=2` keeps the payload tiny: the series themselves are loaded by
     `VpnUsageSection` only when the Usage tab is opened. */
  useEffect(() => {
    if (vpnId === null) return
    const ctrl = new AbortController()
    const timer = window.setTimeout(() => {
      void api
        .vpnUsage(vpnId, '30d', 2)
        .then((u) => {
          if (ctrl.signal.aborted) return
          setUsage(u)
          setUsageError(null)
        })
        .catch(() => {
          if (!ctrl.signal.aborted) setUsageError('Usage could not be read')
        })
    }, 0)
    return () => {
      ctrl.abort()
      window.clearTimeout(timer)
    }
  }, [vpnId])

  /*
   * `useVpnActions` needs a non-null VPN, and a hook cannot be called
   * conditionally — so the lifecycle controls live in a child that is only
   * mounted when the VPN resolved. That keeps the hook order stable for the
   * missing-VPN branch above, which returns early and must not skip a hook.
   */
  const lifecycleActions = vpn && isStaff ? <VpnLifecycleActions vpn={vpn} /> : null

  /*
   * Auto-wake (D-094).
   *
   * A dormant VPN is one the panel had lost. `SERVER_UNAVAILABLE → ACTIVE` is a
   * legal transition, so the panel can find the client again on its own — and
   * when it does, a VPN sitting at the bottom of the list under a "you put this
   * to sleep" sentence is actively misleading: it works, and the customer is not
   * being told.
   *
   * So the detail page — the one screen that just asked the server for a live
   * status — wakes it. Scoped to `id` routes only: the staff route
   * (`/vpns/:serverId/:email`) is a read-only console view and must not change a
   * customer's preference as a side effect of being opened.
   *
   * On failure it does nothing at all: a VPN that stays asleep is the safe
   * outcome, and the customer can always wake it by hand.
   */
  const dormantNow = vpn?.dormantAt ?? null
  const wokeRef = useRef(false)
  useEffect(() => {
    if (id === undefined || dormantNow === null) return
    if (vpn?.status !== 'ACTIVE' || wokeRef.current) return
    wokeRef.current = true
    void wake(vpn.apiId).catch(() => {
      /* best-effort; see above */
    })
  }, [id, dormantNow, vpn?.status, vpn?.apiId, wake])

  const missingReason = (() => {
    if (!decodedEmail && !id) return 'That address does not name a VPN.'
    if (vpnsLoading) return null
    if (decodedEmail && routeServerID !== null && Number.isFinite(routeServerID)) {
      return (
        `No VPN named “${decodedEmail}” is on server ${routeServerID} in the list this account ` +
        'can read. A customer-owned VPN is listed only for its owner and for staff — it may ' +
        'also have been revoked.'
      )
    }
    return 'This VPN is not in the list this account can read. It may have been revoked, or it may belong to another account.'
  })()

  if (!vpn) {
    return (
      <div className="dash">
        <PageHeader
          title={vpnsLoading ? 'Loading this VPN…' : 'VPN not found'}
          subtitle={missingReason ?? undefined}
        />
        {!vpnsLoading && (
          <div className="ui-row vd-actions">
            <Button variant="outline" onClick={() => void load()}>
              Reload
            </Button>
            <Button variant="outline" onClick={() => navigate(isStaff ? '/vpns' : '/client')}>
              {isStaff ? 'Back to VPN management' : 'Back to my VPNs'}
            </Button>
          </div>
        )}
      </div>
    )
  }

  const active = vpn.status === 'ACTIVE'
  const note = statusNote(vpn.status)
  const limit = usage?.limit_bytes ?? 0
  const left = daysLeft(vpn.expiresAt, nowSec)

  return (
    <div className="dash vd" data-testid="vpn-detail">
      <PageHeader
        title={vpnDisplayName(vpn)}
        /*
         * A panel client Veylix did not create has no local created_at, so the
         * field arrives as 0. `new Date(0)` is 1/1/1970 — a date nobody chose,
         * shown as fact. Absent knowledge is stated in words instead (D-059).
         */
        subtitle={
          vpn.createdAt > 0
            ? `${vpnServerLabel(vpn)} · added ${new Date(vpn.createdAt * 1000).toLocaleDateString(undefined, { timeZone: activeTimeZone() })}`
            : `${vpnServerLabel(vpn)} · created outside Veylix, so its start date is unknown`
        }
        actions={
          <div className="ui-row vd-actions">
            <VpnKindBadge kind={vpn.kind} />
            {lifecycleActions}
            <Button
              size="sm"
              variant="outline"
              leading={<ArrowLeft size={13} />}
              onClick={() => navigate(-1)}
            >
              Back
            </Button>
          </div>
        }
      />

      {/* The identity sits at the top of the page, readable and copyable, beside
          the package name in the header — never buried as "technical detail"
          (D-091.4). */}
      <PanelIdentityRow
        panelEmail={vpn.panelEmail}
        serverLabel={vpnServerLabel(vpn)}
        layout="block"
        testId="vd-panel-identity"
      />

      {/* One tab row per screen, so the three sections are the only hierarchy. */}
      <nav className="vd-tabs" aria-label="VPN details sections">
        {SECTIONS.map((s) => (
          <button
            key={s.key}
            type="button"
            className="vd-tabs__btn"
            aria-current={section === s.key ? 'page' : undefined}
            onClick={() => setSection(s.key)}
            data-testid={`vd-tab-${s.key}`}
          >
            {s.label}
          </button>
        ))}
      </nav>

      {section === 'overview' && (
        <section className="vd-section" aria-labelledby="vd-h-overview" data-testid="vd-overview">
          <h2 className="vd-h2" id="vd-h-overview">
            Overview
          </h2>

          <Card title="This VPN right now">
            <div className="vd-status">
              <StatusBadge status={vpn.status} />
              {left && <span className="vui-muted">{left}</span>}
              {/* Account status is NOT device connectivity, and this page does
                  not pretend otherwise: Veylix knows what the panel says about
                  the client, not what the customer's phone is doing. */}
              <span className="vui-muted">
                Device connection is not verified here — this shows what the server reports about
                your account.
              </span>
            </div>
            {note && (
              <div className="vd-note" role="status" data-testid="vd-status-note">
                {note}
              </div>
            )}
          </Card>

          <Card title="Data used">
            {usageError ? (
              <p className="vui-muted">{usageError}</p>
            ) : (
              <VpnUsageCards
                downBytes={usage?.lifetime_down_bytes ?? 0}
                upBytes={usage?.lifetime_up_bytes ?? 0}
                totalBytes={usage?.lifetime_total_bytes ?? 0}
                limitBytes={limit}
                loading={usage === null && usageError === null}
              />
            )}
            <p className="vui-muted vui-note">
              These are the server&apos;s own counters, not this page&apos;s arithmetic. Open
              Usage for the history behind them.
            </p>
          </Card>

          <Card title="At a glance">
            <dl className="vd-facts">
              <div>
                <dt>Expires</dt>
                <dd>
                  {vpn.expiresAt ? new Date(vpn.expiresAt * 1000).toLocaleDateString(undefined, { timeZone: activeTimeZone() }) : 'No expiry'}
                </dd>
              </div>
              <div>
                <dt>Data left</dt>
                <dd>
                  {limit > 0
                    ? `${gb(Math.max(0, usage?.remaining_bytes ?? 0))} of ${gb(limit)}`
                    : 'No data limit'}
                </dd>
              </div>
              <div>
                <dt>Server</dt>
                <dd>{vpnServerLabel(vpn)}</dd>
              </div>
              <div>
                <dt>Package</dt>
                <dd>{vpnDisplayName(vpn)}</dd>
              </div>
            </dl>
            <div className="ui-row vd-actions">
              <Button
                size="sm"
                variant="primary"
                onClick={() => setSection('connect')}
                data-testid="vd-go-connect"
              >
                Connect this VPN
              </Button>
              <Button
                size="sm"
                variant="outline"
                leading={<RefreshCw size={13} />}
                onClick={() => void load()}
              >
                Refresh status
              </Button>
            </div>
          </Card>
        </section>
      )}

      {section === 'usage' && (
        <section className="vd-section" aria-labelledby="vd-h-usage" data-testid="vd-usage">
          <h2 className="vd-h2" id="vd-h-usage">
            Usage
          </h2>
          <VpnUsageSection
            vpnId={vpn.apiId}
            kind={vpn.kind}
            active={active}
            status={vpn.status}
            statusChangedAt={vpn.statusChangedAt}
          />
        </section>
      )}

      {section === 'connect' && (
        <section className="vd-section" aria-labelledby="vd-h-connect" data-testid="vd-connect">
          <h2 className="vd-h2" id="vd-h-connect">
            Connection &amp; Help
          </h2>
          <VpnConnectSection
            vpnId={vpn.apiId}
            active={active}
            seller={{
              id: vpn.sellerId,
              name: vpn.sellerName,
              email: vpn.sellerEmail,
              whatsapp: vpn.sellerWhatsApp,
              telegram: vpn.sellerTelegram,
            }}
            onOpenChat={() => navigate('/chat')}
          />
        </section>
      )}

      
    </div>
  )
}


