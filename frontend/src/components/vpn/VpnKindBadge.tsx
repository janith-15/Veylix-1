import { Badge } from '../ui'
import type { VpnKind } from '../../stores/vpnStore'

/**
 * Free / Premium / Untagged, stated the same way everywhere.
 *
 * The owner's requirement is that a client sees Free and Premium as two
 * separate things, never a mixed list where the difference has to be inferred.
 * One component owns that wording so a card, a table row and a detail header
 * cannot drift apart — and `UNTAGGED` is labelled rather than hidden, because an
 * untagged panel client is a real state (Super Admin only), not a missing value.
 */
export function VpnKindBadge({ kind }: { kind: VpnKind }) {
  if (kind === 'PREMIUM') return <Badge tone="primary">PREMIUM</Badge>
  if (kind === 'FREE') return <Badge tone="info">FREE</Badge>
  return <Badge tone="warning">UNTAGGED</Badge>
}
