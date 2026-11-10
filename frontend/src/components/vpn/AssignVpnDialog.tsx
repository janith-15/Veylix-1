import { useEffect, useMemo, useState } from 'react'
import { Dialog, Input } from '../ui'
import { UserPlus, Users } from 'lucide-react'
import { api, ApiError, type ApiAdminClientRow } from '../../lib/api'
import { useOffersStore } from '../../stores/offersStore'
import { formatMoney } from '../../lib/format'
import { VpnKindBadge } from './VpnKindBadge'

/**
 * Hands an UNASSIGNED panel client to a customer (D-037, D-034, D-063).
 *
 * `VpnsPage` used to render the word "assign" as a `<span>`, so a synced-but-
 * unowned panel client could only be handed over with `curl`. This dialog is the
 * missing caller for `POST /api/panel-clients/{server_id}/{email}/assign`.
 *
 * An assignment is ALWAYS premium: there is no free/paid choice, because a FREE
 * credential is claimed from an offer rather than assigned. Asking for the kind
 * was what let a paid VPN be stored with no package — and a VPN with no package
 * carried no kind, so the customer's own screen filed it under UNTAGGED.
 */
export function AssignVpnDialog({
  open,
  onClose,
  serverId,
  panelEmail,
  onAssigned,
}: {
  open: boolean
  onClose: () => void
  serverId: number
  panelEmail: string
  /**
   * Called after the server confirms; the parent reloads its list.
   *
   * `warning` is non-empty only when the operator asked for a rename that the
   * panel did not confirm — the assignment still succeeded and the VPN kept its
   * existing name, so the parent reports that rather than an error (D-063).
   */
  onAssigned: (result: { vpnId: number; panelEmail: string; renamed: boolean; warning: string }) => void
}) {
  const [clients, setClients] = useState<ApiAdminClientRow[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<number | null>(null)
  const [offerId, setOfferId] = useState<number | null>(null)
  const [submitting, setSubmitting] = useState(false)

  /*
   * The package is a refinement, not a requirement (D-063).
   *
   * It used to be required in practice: without one the row had `offer_id = NULL`
   * and therefore no kind at all, so the customer saw an UNTAGGED VPN in neither
   * list — which is exactly what "premium never shows up" looked like. The server
   * now attaches the platform's premium package when none is chosen, so the
   * assignment is premium whether or not this picker is used.
   */
  const offers = useOffersStore((s) => s.offers)
  const loadOffers = useOffersStore((s) => s.load)
  useEffect(() => {
    if (open) void loadOffers()
  }, [open, loadOffers])

  /* The candidate list is the server's own client console, so this can only ever
   * offer real accounts. Read when the dialog opens, never per keystroke. */
  useEffect(() => {
    if (!open) return
    let cancelled = false
    void (async () => {
      // Set state AFTER a tick, never synchronously inside the effect — the same
      // shape `VpnsPage.load` uses to avoid a cascading render (react(set-state-in-effect)).
      await Promise.resolve()
      if (cancelled) return
      setLoading(true)
      setError(null)
      setSelected(null)
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
    const q = query.trim().toLowerCase()
    if (!q) return clients
    return clients.filter(
      (c) =>
        c.email.toLowerCase().includes(q) ||
        (c.display_name ?? '').toLowerCase().includes(q) ||
        (c.username ?? '').toLowerCase().includes(q),
    )
  }, [clients, query])

  const packages = useMemo(
    // An assignment is always premium (D-063): FREE credentials are claimed from
    // an offer, never assigned, so only PREMIUM packages are offered here.
    () => offers.filter((o) => o.active && o.kind === 'PREMIUM'),
    [offers],
  )

  const submit = async () => {
    if (!selected) return
    setSubmitting(true)
    setError(null)
    try {
      const res = await api.assignPanelClient(serverId, panelEmail, {
        clientUserId: selected,
        offerId: offerId ?? 0,
      })
      onAssigned({ vpnId: res.vpn_id, panelEmail: res.panel_email, renamed: res.renamed, warning: res.warning })
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Could not assign this panel client'
      setError(msg)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="Assign this VPN to a customer"
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
            {submitting ? 'Assigning…' : 'Assign VPN'}
          </button>
        </div>
      }
    >
      <div className="ui-col" style={{ gap: 12 }}>
        <div className="vpn-chips">
          <span className="vpn-chip">server #{serverId}</span>
          <span className="vpn-chip">{panelEmail}</span>
        </div>

        {/* An assignment is always a premium issue (D-063), so there is nothing
            to choose: a FREE credential is claimed from an offer, not assigned.
            The old Free/Paid radio was the source of the whole mismatch. */}
        <div className="ui-row" style={{ gap: 8, alignItems: 'center' }}>
          <VpnKindBadge kind="PREMIUM" />
          <span className="ui-muted t-sm2">Assigning gives this customer a premium VPN.</span>
        </div>

        <label className="ui-field">
          <span className="ui-field__label">Package (optional)</span>
          <select
            className="ui-input"
            value={offerId ?? ''}
            onChange={(e) => setOfferId(e.target.value ? Number(e.target.value) : null)}
          >
            <option value="">Use the platform's premium package</option>
            {packages.map((o) => (
              <option key={o.id} value={o.apiId}>
                {o.name}
                {o.priceCents > 0 ? ` — ${formatMoney(o.priceCents)}` : ''}
              </option>
            ))}
          </select>
          <span className="ui-field__hint">
            {packages.length === 0
              ? 'No PREMIUM package exists yet, so the platform package is recorded. The VPN is premium either way.'
              : 'The package is what the customer sees on their own screen. Leaving it empty still records a premium VPN.'}
          </span>
        </label>

        <Input
          label="Find a customer"
          placeholder="name, username or email"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />

        {error && (
          <div className="vpn-warn t-sm2" role="alert">
            {error}
          </div>
        )}

        <div
          className="ui-col cl-pick-list"
          style={{ gap: 4, maxHeight: 280, overflowY: 'auto' }}
        >
          {loading && <p className="ui-muted t-sm2">Loading clients…</p>}
          {!loading && filtered.length === 0 && (
            <p className="ui-muted t-sm2">
              No active clients match. A customer must register before a VPN can be assigned to
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

        <p className="ui-muted t-sm2">
          <UserPlus size={12} aria-hidden="true" /> The panel client is renamed to the customer's
          address scheme on assign (D-034), so the credential is attributable in 3X-UI.
        </p>
      </div>
    </Dialog>
  )
}
