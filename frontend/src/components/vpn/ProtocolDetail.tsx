import { useEffect, useState } from 'react'
import { Card, Spinner, Tooltip } from '../ui'
import { api, type ApiVpnProtocol } from '../../lib/api'

/**
 * P-09 — the protocol detail the owner listed: *"vless vmess, protocol eka
 * mokadda tcp ws, username password"*.
 *
 * Two rules make this honest:
 *   - the protocol/transport come from the **inbound**, the credential from the
 *     **panel's client record** — nothing is derived from a link;
 *   - `source: 'unavailable'` is rendered as "the panel could not be read while
 *     showing what is still known locally", never as "this VPN has no
 *     credential". An outage is not an absence (REQUIREMENTS §2.0 I-5).
 */
export function ProtocolDetail({ vpnId }: { vpnId: number }) {
  const [detail, setDetail] = useState<ApiVpnProtocol | null>(null)
  const [error, setError] = useState<string | null>(null)
  /* The credential is re-masked for a different VPN: the reveal state is keyed
     by id and derived at render, so switching VPNs cannot leave a previous
     client's secret on screen. */
  const [revealedFor, setRevealedFor] = useState<number | null>(null)
  const revealed = revealedFor === vpnId

  useEffect(() => {
    let cancelled = false
    void api
      .vpnProtocol(vpnId)
      .then((d) => {
        if (!cancelled) {
          setDetail(d)
          setError(null)
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setDetail(null)
          setError(err instanceof Error ? err.message : 'Protocol detail is unavailable')
        }
      })
    return () => {
      cancelled = true
    }
  }, [vpnId])

  if (error) {
    return (
      <Card title="Protocol">
        <p className="ui-muted t-sm2" style={{ margin: 0 }}>{error}</p>
      </Card>
    )
  }
  if (!detail) {
    return (
      <Card title="Protocol">
        <div className="ui-row" style={{ gap: 8 }}>
          <Spinner size={14} />
          <span className="ui-muted t-sm2">Reading protocol…</span>
        </div>
      </Card>
    )
  }

  /* A credential is sensitive: it stays masked until the customer asks for it,
     so a screenshot of the page does not leak a working config. */
  const secret = detail.uuid || detail.password || ''
  const secretLabel = detail.uuid ? 'UUID' : 'Password'
  const masked = secret ? `${secret.slice(0, 4)}${'•'.repeat(Math.min(20, Math.max(6, secret.length - 4)))}` : ''

  return (
    <Card
      title="Protocol"
      actions={
        <span className="ui-muted t-xs">
          {detail.source === 'panel' ? 'From the panel' : 'Panel unreadable — locally known fields only'}
        </span>
      }
    >
      <div className="vpn-proto" data-testid="vpn-protocol">
        <div className="vpn-proto__row">
          <span className="vpn-proto__k">Protocol</span>
          <span className="vpn-proto__v">
            <span className="vpn-chip">{detail.protocol || 'unknown'}</span>
          </span>
        </div>
        <div className="vpn-proto__row">
          <span className="vpn-proto__k">Transport</span>
          <span className="vpn-proto__v">
            <span className="vpn-chip">{(detail.transport || 'unknown').toUpperCase()}</span>
          </span>
        </div>
        <div className="vpn-proto__row">
          <span className="vpn-proto__k">Security</span>
          <span className="vpn-proto__v">
            <span className="vpn-chip">{detail.security || 'unknown'}</span>
            {detail.flow && <span className="vpn-chip">flow {detail.flow}</span>}
          </span>
        </div>
        <div className="vpn-proto__row">
          <span className="vpn-proto__k">Server</span>
          <span className="vpn-proto__v">
            <code>{detail.server || '—'}</code>
          </span>
        </div>
        {secret && (
          <div className="vpn-proto__row">
            <span className="vpn-proto__k">{secretLabel}</span>
            <span className="vpn-proto__v">
              <code className="vpn-proto__secret">{revealed ? secret : masked}</code>
              <Tooltip label={revealed ? 'Hide the credential' : 'Show the credential'}>
                <button
                  type="button"
                  className="vpn-proto__reveal"
                  onClick={() => setRevealedFor(revealed ? null : vpnId)}
                  aria-label={revealed ? 'Hide credential' : 'Show credential'}
                >
                  {revealed ? 'Hide' : 'Show'}
                </button>
              </Tooltip>
            </span>
          </div>
        )}
        {detail.sub_id && (
          <div className="vpn-proto__row">
            <span className="vpn-proto__k">Subscription</span>
            <span className="vpn-proto__v">
              <code>{detail.sub_id}</code>
            </span>
          </div>
        )}
        {detail.remark && (
          <div className="vpn-proto__row">
            <span className="vpn-proto__k">Inbound</span>
            <span className="vpn-proto__v">{detail.remark}</span>
          </div>
        )}
      </div>
      {detail.source === 'unavailable' && (
        <p className="ui-muted t-xs" style={{ margin: '10px 0 0' }}>
          The panel did not answer, so the credential and server fields may be incomplete. This is
          an outage, not an empty VPN — try again shortly.
        </p>
      )}
    </Card>
  )
}
