import { Link } from 'react-router-dom'
import { StatusBadge, Button, formatBytes } from '../ui'
import { MapPin, ExternalLink, ShieldCheck, Crown } from 'lucide-react'
import { VpnKindBadge } from './VpnKindBadge'
import { PanelIdentityRow } from './PanelIdentityRow'
import type { ClientVpn } from '../../stores/vpnStore'
import { vpnDisplayName, vpnServerLabel } from '../../stores/vpnStore'
import { activeTimeZone } from '../../lib/time'

export interface VpnHeroCardProps {
  vpn: ClientVpn
  usage?: { total: number; limit: number; pct: number } | null
  daysLeft: number | null
}

/**
 * Client VPN hero card — Stage 4 of the frontend redesign.
 *
 * The client's primary connection, stated once and prominently at the top of
 * the dashboard. Everything here is already owned by the server (status,
 * expiry, usage); the card only presents it, and every number links down to
 * the full detail screen instead of pretending to be the detail screen.
 */
export function VpnHeroCard({ vpn, usage, daysLeft }: VpnHeroCardProps) {
  const u = usage ?? null
  return (
    <div className="vpn-detail-hero client-vpn-hero" data-testid="vpn-hero-card">
      <div className="chv__body">
        <span className="chv__row chv__row--head">
          <span className="chv__name">
            {vpnDisplayName(vpn)}
          </span>
          <VpnKindBadge kind={vpn.kind} />
          <StatusBadge status={vpn.status} />
        </span>
        <span className="ui-muted t-sm2 chv__row chv__row--tight">
          <MapPin size={13} aria-hidden="true" />
          {vpnServerLabel(vpn)}
        </span>
        {/*
          The panel identity, beside the package name (D-091.4, non-negotiable 7).
          Every client screen that lists a VPN shows this, so the customer can
          read and copy it without opening the detail page - which is the only
          way they can quote it to support or paste it into a client config.
        */}
        <PanelIdentityRow
          panelEmail={vpn.panelEmail}
          layout="inline"
          testId={`hero-panel-identity-${vpn.apiId}`}
        />
        <span className="chv__row chv__row--head">
          <span className="ui-muted t-sm2">
            expires {vpn.expiresAt ? new Date(vpn.expiresAt * 1000).toLocaleDateString(undefined, { timeZone: activeTimeZone() }) : 'never'}
          </span>
          {daysLeft !== null &&
            (daysLeft < 0 ? (
              <span className="cl-tag cl-tag--warn">expired {Math.abs(daysLeft)}d ago</span>
            ) : daysLeft <= 7 ? (
              <span className="cl-tag cl-tag--warn">{daysLeft}d left</span>
            ) : (
              <span className="cl-tag">{daysLeft}d left</span>
            ))}
        </span>
        {u && (
          <span className="chv__meter">
            <span className="cl-quotabar__num">
              {formatBytes(u.total)}
              {u.limit > 0 ? ` / ${formatBytes(u.limit)}` : ' used · unlimited'}
            </span>
            {u.limit > 0 && (
              <span className="cl-quotabar__track">
                <span
                  className={`cl-quotabar__fill${u.pct >= 100 ? ' is-full' : u.pct >= 80 ? ' is-high' : ''}`}
                  style={{ width: `${Math.min(100, u.pct)}%` }}
                />
              </span>
            )}
          </span>
        )}
        {vpn.status !== 'ACTIVE' && (
          <span className="vpn-warn chv__warn">
            <ShieldCheck size={15} aria-hidden="true" /> Not connectable right now — details inside.
          </span>
        )}
      </div>
      <div className="chv__side">
        <Link to={`/vpn/${vpn.id}`} className="no-underline">
          <Button size="sm" leading={<ExternalLink size={14} />}>
            Open
          </Button>
        </Link>
        {vpn.kind === 'PREMIUM' ? (
          <span className="ui-muted t-sm2 chv__kind">
            <Crown size={12} aria-hidden="true" /> Premium credential
          </span>
        ) : (
          <span className="ui-muted t-sm2">Free credential</span>
        )}
      </div>
    </div>
  )
}
