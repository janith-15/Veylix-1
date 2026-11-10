import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useNavigate } from 'react-router-dom'
import { Clock, ShieldCheck, TriangleAlert, X, Zap } from 'lucide-react'
import { Button, StatusBadge } from '../ui'
import { VpnUsageCards } from './VpnUsageCards'
import { PanelIdentityRow } from './PanelIdentityRow'
import { api, ApiError } from '../../lib/api'
import { statusNote, type ClientVpn, vpnDisplayName, vpnServerLabel } from '../../stores/vpnStore'
import './vpn-quick-view.css'

/**
 * The first thing a customer sees when they pick a VPN (details-page brief §4.2).
 *
 * WHY A SHEET AND NOT A PAGE
 *
 * The advanced screen exists and is reachable — this is deliberately not a
 * second copy of it. The owner's complaint was that the details page overwhelms
 * a non-technical customer, so the selection flow answers the four questions a
 * customer actually has before showing anything else:
 *
 *   1. Is it working?          (status + the sentence explaining it)
 *   2. Which VPN is this?      (package name AND panel identity, D-091.4)
 *   3. How much have I used?   (download / upload / total, D-092)
 *   4. How do I connect?       (one action; the QR lives on the full page)
 *
 * A sheet also keeps the My VPNs list mounted behind it, so opening and closing
 * costs no refetch — which matters on the 1 vCPU / 500 MB target.
 *
 * DATA DISCIPLINE
 *
 * Only the SUMMARY read is issued here (`?summary=1`): the allowance, the
 * remaining bytes and the expiry. No history, no charts, no ping, no protocol.
 * A customer who closes the sheet pays for one small request; the full page
 * loads its own heavier reads when they ask for it.
 */

/**
 * The shape `?summary=1` actually returns (httpapi `/usage` summary branch,
 * D-068). It is NOT `ApiVpnUsage`: there are no `points[]`, and — importantly —
 * no `lifetime_up_bytes` / `lifetime_down_bytes`. Typing it as `ApiVpnUsage`
 * would let this screen read fields the server never sent and render `undefined`
 * as if it were a measurement.
 */
interface ApiVpnUsageSummary {
  vpn_id: number
  status: string
  limit_bytes: number
  lifetime_total_bytes: number
  usage_pct: number
  remaining_bytes: number
  expires_at: number | null
  point_count: number
}

interface Props {
  vpn: ClientVpn | null
  onClose: () => void
}

export function VpnQuickView({ vpn, onClose }: Props) {
  const navigate = useNavigate()
  const [usage, setUsage] = useState<ApiVpnUsageSummary | null>(null)
  const [usageError, setUsageError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const closeRef = useRef<HTMLButtonElement | null>(null)

  const vpnId = vpn?.apiId ?? null

  /* Load the summary whenever a different VPN is opened. Scoped by an
     AbortController so closing the sheet cannot leave a response that lands
     afterwards and overwrites the next VPN's numbers.
  
     The reset is deferred into a timeout so the effect does not set state
     synchronously during commit (react/set-state-in-effect), which is the same
     pattern `VpnUsageSection` uses for its first read. */
  useEffect(() => {
    if (vpnId === null) return
    const ctrl = new AbortController()
    const timer = window.setTimeout(() => {
      setUsage(null)
      setUsageError(null)
      setBusy(true)
      void api
        .vpnUsageSummary(vpnId)
        .then((u) => {
          if (ctrl.signal.aborted) return
          setUsage(u as unknown as ApiVpnUsageSummary)
        })
        .catch((err: unknown) => {
          if (ctrl.signal.aborted) return
          setUsageError(err instanceof ApiError ? err.message : 'Usage could not be read')
        })
        .finally(() => {
          if (!ctrl.signal.aborted) setBusy(false)
        })
    }, 0)
    return () => {
      ctrl.abort()
      window.clearTimeout(timer)
    }
  }, [vpnId])

  /* Escape closes, and focus starts on the close button so a keyboard user is
     never stranded behind the sheet. */
  useEffect(() => {
    if (!vpn) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    const t = window.setTimeout(() => closeRef.current?.focus(), 0)
    return () => {
      document.removeEventListener('keydown', onKey)
      window.clearTimeout(t)
    }
  }, [vpn, onClose])

  if (!vpn) return null

  const note = statusNote(vpn.status)
  const title = vpnDisplayName(vpn)
  const days = vpn.expiresAt === null ? null : Math.ceil((vpn.expiresAt - Date.now() / 1000) / 86400)


  return createPortal(
    <div className="qv-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div
        className="qv"
        role="dialog"
        aria-modal="true"
        aria-labelledby="qv-title"
        data-testid="vpn-quick-view"
      >
        <header className="qv__head">
          <div className="qv__headings">
            <h2 className="qv__title" id="qv-title">
              {title}
            </h2>
            {/* Identity and package name together — never one without the
                other (D-091.4). */}
            <PanelIdentityRow
              panelEmail={vpn.panelEmail}
              serverLabel={vpnServerLabel(vpn)}
              testId="qv-panel-identity"
            />
          </div>
          <button
            type="button"
            className="qv__close"
            onClick={onClose}
            ref={closeRef}
            aria-label="Close VPN summary"
          >
            <X size={16} aria-hidden="true" />
          </button>
        </header>

        <div className="qv__status">
          <StatusBadge status={vpn.status} />
          {vpn.kind !== 'UNTAGGED' && (
            <span className="qv__kind">
              <ShieldCheck size={13} aria-hidden="true" />
              {vpn.kind === 'FREE' ? 'Free VPN' : 'Premium VPN'}
            </span>
          )}
        </div>

        {/* The sentence, not just the colour (D-091.5). */}
        {note && (
          <div className="qv__warn" role="status" data-testid="qv-status-note">
            <TriangleAlert size={15} aria-hidden="true" />
            <span>{note}</span>
          </div>
        )}

        {usageError ? (
          <div className="qv__warn" role="alert">
            <TriangleAlert size={15} aria-hidden="true" />
            <span>{usageError}. This is a connection problem, not an empty account.</span>
          </div>
        ) : (
          /* The summary read answers only the TOTAL. The two directions are
             NOT in this response, so they are not drawn: showing `0 B
             received` would be a fabricated measurement, which is exactly what
             the whole ruleset forbids. The full page has the split. */
          <VpnUsageCards
            showDirections={false}
            downBytes={0}
            upBytes={0}
            totalBytes={usage?.lifetime_total_bytes ?? 0}
            limitBytes={usage?.limit_bytes ?? 0}
            loading={busy || !usage}
          />
        )}

        <div className="qv__facts">
          {days !== null && (
            <span className="qv__fact">
              <Clock size={13} aria-hidden="true" />
              {days <= 0 ? 'Expired' : `${days} day${days === 1 ? '' : 's'} left`}
            </span>
          )}
          {usage && usage.limit_bytes <= 0 && <span className="qv__fact">No data limit</span>}
        </div>

        <footer className="qv__foot">
          {/* Connect is the primary action, not "go read another page".
              The sheet exists because a customer opens it to GET ONLINE; the
              full details are the second question, for when something is wrong. */}
          <Button
            variant="primary"
            onClick={() => {
              onClose()
              navigate(`/vpn/${vpn.apiId}?section=connect`)
            }}
            leading={<Zap size={14} />}
            data-testid="qv-connect"
          >
            Connect now
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              onClose()
              navigate(`/vpn/${vpn.apiId}`)
            }}
            data-testid="qv-open-full"
          >
            Full details
          </Button>
          <span className="qv__hint">
            The connection QR and link are on the VPN page — they stay hidden there until you ask
            for them.
          </span>
        </footer>
      </div>
    </div>,
    document.body,
  )
}
