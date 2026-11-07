import { Link } from 'react-router-dom'
import { StatusBadge, Button, formatBytes } from '../ui'
import { MapPin, ExternalLink, ShieldCheck, Crown } from 'lucide-react'
import { VpnKindBadge } from './VpnKindBadge'
import type { ClientVpn } from '../../stores/vpnStore'
import { vpnDisplayName, vpnServerLabel } from '../../stores/vpnStore'

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
      <div className="ui-col" style={{ gap: 8, flex: 1, minWidth: 0 }}>
        <span className="ui-row" style={{ gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          <span className="ui-strong" style={{ fontSize: 18 }}>
            {vpnDisplayName(vpn)}
          </span>
          <VpnKindBadge kind={vpn.kind} />
          <StatusBadge status={vpn.status} />
        </span>
        <span className="ui-muted t-sm2 ui-row" style={{ gap: 6, alignItems: 'center' }}>
          <MapPin size={13} aria-hidden="true" />
          {vpnServerLabel(vpn)}
        </span>
        <span className="ui-row" style={{ gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
          <span className="ui-muted t-sm2">
            expires {vpn.expiresAt ? new Date(vpn.expiresAt * 1000).toLocaleDateString() : 'never'}
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
          <span className="cl-quotabar" style={{ minWidth: 200, maxWidth: 360 }}>
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
          <span className="vpn-warn" style={{ display: 'flex', gap: 8 }}>
            <ShieldCheck size={15} aria-hidden="true" /> Not connectable right now — details inside.
          </span>
        )}
      </div>
      <div className="ui-col" style={{ gap: 8, alignItems: 'flex-end' }}>
        <Link to={`/vpn/${vpn.id}`} className="no-underline">
          <Button size="sm" leading={<ExternalLink size={14} />}>
            Open
          </Button>
        </Link>
        {vpn.kind === 'PREMIUM' ? (
          <span className="ui-muted t-sm2" style={{ display: 'inline-flex', gap: 4, alignItems: 'center' }}>
            <Crown size={12} aria-hidden="true" /> Premium credential
          </span>
        ) : (
          <span className="ui-muted t-sm2">Free credential</span>
        )}
      </div>
    </div>
  )
}
