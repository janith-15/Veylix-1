import { useEffect, useMemo, useState } from 'react'
import { Dialog, Input } from '../ui'
import { ArrowRightLeft, Users } from 'lucide-react'
import { api, ApiError, type ApiAdminClientRow } from '../../lib/api'
import { VpnKindBadge } from './VpnKindBadge'

/**
 * Moves an ALREADY-assigned VPN from one customer to another (D-064).
 *
 * Why this is not `AssignVpnDialog` with an extra prop: the two dialogs answer
 * opposite questions, and merging them produced the confusion this feature
 * exists to end. `AssignVpnDialog` claims a panel client **nobody** holds and
 * therefore chooses a package; this one moves a VPN that **somebody already
 * holds**, so its package, expiry, limits and tag are already decided and must
 * not be offered for change — a transfer is not a new sale.
 *
 * Two facts the operator must be told, because both are surprising and both are
 * load-bearing:
 *   1. the customer's existing connection keeps working — the panel client is
 *      never renamed, so every link and QR already handed out still resolves;
 *   2. the previous owner loses access immediately, with nothing to undo.
 */
export function TransferVpnDialog({
  open,
  onClose,
  vpnId,
  panelEmail,
  fromEmail,
  onTransferred,
}: {
  open: boolean
  onClose: () => void
  vpnId: number
  /** The panel client being moved; shown so the operator confirms the right VPN. */
  panelEmail: string
  /** The current holder, shown read-only. */
  fromEmail: string
  /** Called after the server confirms; the parent reloads its list. */
  onTransferred: (result: { vpnId: number; toEmail: string; panelEmail: string }) => void
}) {
  const [clients, setClients] = useState<ApiAdminClientRow[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<number | null>(null)
  const [reason, setReason] = useState('')
  const [submitting, setSubmitting] = useState(false)

  /* The candidate list is the server's own client console, so this can only ever
   * offer real accounts. Read when the dialog opens, never per keystroke — the
   * same shape `AssignVpnDialog` uses to avoid a cascading render. */
  useEffect(() => {
    if (!open) return
    let cancelled = false
    void (async () => {
      await Promise.resolve()
      if (cancelled) return
      setLoading(true)
      setError(null)
      setSelected(null)
      setReason('')
      setQuery('')
      try {
        const page = await api.adminClients({ status: 'ACTIVE' })
        if (!cancelled) setClients(page.clients ?? [])
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : 'Could not load clients')
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [open])

  const filtered = useMemo(() => {
    // The current holder is not a destination: moving a VPN to the account that
    // already has it is refused by the server (`cannot_transfer_self`), so
    // offering it would only invite a click that cannot work.
    const candidates = clients.filter((c) => c.email !== fromEmail)
    const q = query.trim().toLowerCase()
    if (!q) return candidates
    return candidates.filter(
      (c) =>
        c.email.toLowerCase().includes(q) ||
        (c.display_name ?? '').toLowerCase().includes(q) ||
        (c.username ?? '').toLowerCase().includes(q),
    )
  }, [clients, query, fromEmail])

  const target = filtered.find((c) => c.id === selected) ?? null

  const submit = async () => {
    if (!selected) return
    setSubmitting(true)
    setError(null)
    try {
      const res = await api.transferVPN(vpnId, {
        toClientUserId: selected,
        reason: reason.trim(),
      })
      onTransferred({ vpnId: res.vpn_id, toEmail: res.to.email, panelEmail: res.panel_email })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not move this VPN')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Move this VPN to another customer"
      footer={
        <div className="ui-row" style={{ gap: 8, justifyContent: 'flex-end' }}>
          <button type="button" className="ui-btn ui-btn--ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            type="button"
            className="ui-btn ui-btn--primary"
            disabled={!selected || submitting}
            onClick={() => void submit()}
          >
            {submitting ? 'Moving…' : 'Move VPN'}
          </button>
        </div>
      }
    >
      <div className="ui-col" style={{ gap: 12 }}>
        <div className="vpn-chips">
          <span className="vpn-chip">{panelEmail}</span>
          <span className="vpn-chip">
            from <strong>{fromEmail}</strong>
          </span>
        </div>

        {/* Stated plainly, because a successful transfer looks like nothing
            happened on the panel: the client is not renamed and its links do
            not change, which is exactly what makes the VPN keep working. */}
        <div className="ui-col" style={{ gap: 6 }}>
          <div className="ui-row" style={{ gap: 8, alignItems: 'center' }}>
            <VpnKindBadge kind="PREMIUM" />
            <span className="ui-muted t-sm2">
              The VPN keeps its package, expiry and connection details.
            </span>
          </div>
          <p className="ui-muted t-sm2">
            <ArrowRightLeft size={12} aria-hidden="true" /> The new owner sees it immediately and{' '}
            {fromEmail} loses access. The 3X-UI client is not renamed, so any link or QR already
            handed out keeps working.
          </p>
        </div>

        <Input
          label="Find the customer who receives it"
          placeholder="name, username or email"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />

        {error && (
          <div className="vpn-warn t-sm2" role="alert">
            {error}
          </div>
        )}

        <div className="ui-col cl-pick-list" style={{ gap: 4, maxHeight: 260, overflowY: 'auto' }}>
          {loading && <p className="ui-muted t-sm2">Loading clients…</p>}
          {!loading && filtered.length === 0 && (
            <p className="ui-muted t-sm2">
              No other active client matches. A customer must register before a VPN can be moved to
              them.
            </p>
          )}
          {filtered.map((c) => {
            const name = c.display_name || c.username || c.email
            const active = selected === c.id
            return (
              <button
                key={c.id}
                type="button"
                className={`cl-pick${active ? ' is-active' : ''}`}
                onClick={() => setSelected(c.id)}
                aria-pressed={active}
              >
                <span className="cl-pick__icon" aria-hidden="true">
                  <Users size={14} />
                </span>
                <span className="cl-cell__text">
                  <span className="ui-strong">{name}</span>
                  <span className="ui-muted cl-cell__sub">{c.email}</span>
                </span>
                <span className="ui-muted t-sm2">
                  {c.premium_count} premium · {c.free_used} free used
                </span>
              </button>
            )
          })}
        </div>

        <Input
          label="Reason (optional)"
          placeholder="why this VPN is moving — kept in the history"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />

        {target && (
          <p className="ui-muted t-sm2">
            <ArrowRightLeft size={12} aria-hidden="true" /> {panelEmail} will belong to{' '}
            <strong>{target.display_name || target.username || target.email}</strong> (
            {target.email}).
          </p>
        )}
      </div>
    </Dialog>
  )
}
