import { Badge, type BadgeTone } from './Badge'
import { useThemeStore } from '../../theme/themeStore'
import { humanize } from '../../lib/format'

/**
 * Status badge coloured by the platform status model
 * (see docs/REQUIREMENTS.md §7). No invented states: unknown
 * input falls back to the default tone.
 */

const STATUS_TONE: Record<string, BadgeTone> = {
  ACTIVE: 'success',
  PENDING: 'info',
  EXPIRED: 'warning',
  TRAFFIC_DEPLETED: 'warning',
  SUSPENDED: 'error',
  SERVER_UNAVAILABLE: 'error',
  REAUTH_REQUIRED: 'warning',
  CONNECTION_UNAVAILABLE: 'error',
  REVOCATION_PENDING: 'warning',
  REVOKED: 'error',
  REMOVED_FROM_CATALOG: 'default',
  DISABLED: 'default',
  REMOVED: 'default',
  PENDING_VERIFICATION: 'info',
  AWAITING_CONFIRMATION: 'info',
  CONFIRMED: 'success',
  REJECTED: 'error',
  CANCELLED: 'default',
  REFUND_PENDING: 'warning',
  REFUNDED: 'info',
}

export function StatusBadge({ status, className }: { status: string; className?: string }) {
  const tone = STATUS_TONE[status] ?? 'default'
  const showDot = useThemeStore((s) => s.settings.components?.badgeDot ?? false)
  return <Badge tone={tone} className={className} dot={showDot}>{humanize(status)}</Badge>
}
