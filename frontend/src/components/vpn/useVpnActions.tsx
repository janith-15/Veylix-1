import { useEffect, useState, type ReactNode } from 'react'
import { Ban, PlayCircle, RotateCcw, ShieldOff, Trash2, TriangleAlert, Undo2 } from 'lucide-react'
import { Button, Dialog, Input, StatusBadge, useToast } from '../ui'
import { useVpnStore, type ClientVpn } from '../../stores/vpnStore'
import { ApiError } from '../../lib/api'

/**
 * Super Admin lifecycle controls for one VPN (D-036.5, D-024.5).
 *
 * Every action here has had a working API since Phase 5 — `suspend`, `resume`,
 * `revoke` — and **no screen ever called any of them**: `vpnStore` exposed all
 * three and the entire UI never invoked one, so a Super Admin could watch a VPN
 * but could not stop it. This component is the missing entry point.
 *
 * Rules that are not negotiable:
 *  - Each action says what it does to the PANEL and to Veylix, because the two
 *    diverge (suspend flips the panel's enable flag; revoke deletes the client).
 *  - Destructive actions confirm first, and the confirm text names the customer.
 *  - The backend's refusal is shown verbatim (`ApiError.message`), never
 *    replaced with a generic "it failed": `vpn_still_on_panel`, `plan_exhausted`
 *    and `panel_error` mean completely different things to the operator.
 *  - A revoke the panel did not confirm reports REVOCATION_PENDING as pending;
 *    it must never look finished.
 */
export function useVpnActions({ vpn, onChanged }: { vpn: ClientVpn; onChanged?: () => void }) {
  const suspend = useVpnStore((s) => s.suspend)
  const resume = useVpnStore((s) => s.resume)
  const revoke = useVpnStore((s) => s.revoke)
  const release = useVpnStore((s) => s.release)
  const removeAbsent = useVpnStore((s) => s.removeAbsent)
  const releases = useVpnStore((s) => s.releases)
  const restoreRelease = useVpnStore((s) => s.restoreRelease)
  const loadReleases = useVpnStore((s) => s.loadReleases)
  const toast = useToast()
  const [busy, setBusy] = useState<'suspend' | 'resume' | 'revoke' | 'remove' | 'release' | 'restore' | null>(null)
  const [confirm, setConfirm] = useState<'suspend' | 'revoke' | 'remove' | 'release' | null>(null)
  const [error, setError] = useState('')
  /**
   * Typed confirmation for "Remove record".
   *
   * A single click previously erased the row for good. I know exactly how that
   * ends, because a script of mine did it twice to a real database. The operator
   * now has to type the panel e-mail, the same way client deletion already works
   * (`ClientDetailPage`) — the cost of a mistake is a lost record, so the cost of
   * confirming must be more than a click.
   */
  const [removeConfirm, setRemoveConfirm] = useState('')
  /*
   * `vpn` is a placeholder while the page is still loading its store (the hook
   * must be called unconditionally, so `useVpnActions` receives `{}` until the
   * row resolves). Dereferencing `vpn.panelEmail` here crashed the whole page
   * with "Cannot read properties of undefined (reading 'toLowerCase')" — the
   * guard must be on the value, not on the dialog being open.
   */
  const removeConfirmed =
    !!vpn.panelEmail && removeConfirm.trim().toLowerCase() === vpn.panelEmail.toLowerCase()

  /** One shared runner so every action reports failure the same way. */
  const run = async (
    kind: 'suspend' | 'resume' | 'revoke' | 'remove' | 'release' | 'restore',
    fn: () => Promise<unknown>,
    okTitle: string,
    okBody: string,
  ) => {
    setBusy(kind)
    setError('')
    try {
      await fn()
      toast('success', okTitle, okBody)
      onChanged?.()
    } catch (err) {
      // The server's own message is the only one that names the real cause.
      const msg = err instanceof ApiError ? err.message : 'Unexpected error'
      setError(msg)
      toast('error', 'The panel refused the change', msg)
    } finally {
      setBusy(null)
      setConfirm(null)
      // A used confirmation is spent: the next removal asks for the e-mail again.
      setRemoveConfirm('')
    }
  }

  const suspended = vpn.status === 'SUSPENDED'
  const revoked = vpn.status === 'REVOKED' || vpn.status === 'REVOCATION_PENDING'
  /**
   * "Remove record" is only meaningful for a VPN the panel no longer holds.
   * Offering it on a working VPN would be a trap: the server refuses with
   * `vpn_still_on_panel`, but the button would still have invited the click.
   */
  const canRemoveRecord = vpn.status === 'SERVER_UNAVAILABLE'

  /**
   * D-027: a released VPN sits in a 3-day undo window. The row's own status is
   * now RELEASED (#2), and the release record carries the deadline. The record is
   * looked up from the store's list so the button can say "Restore" and show how
   * long is left, instead of offering "Release" on something already released.
   */
  const isReleased = vpn.status === 'RELEASED'
  const pendingRelease = releases.find(
    (r) => r.vpn_id === vpn.apiId && !r.restored_at && !r.removed_at,
  )

  useEffect(() => {
    if (!vpn.apiId) return
    void loadReleases()
  }, [vpn.apiId, loadReleases])

  /**
   * "Now" is captured once per mount rather than read during render: `Date.now()`
   * in a render body is impure, which React's compiler rules (and this repo's
   * oxlint config) reject — the value could change between two renders of the
   * same state. The countdown only needs day-level accuracy, so one read is
   * enough.
   */
  const [nowMs] = useState(() => Date.now())

  /** Whole days until the undo window closes; null when there is none. */
  const daysLeftToRestore = (() => {
    if (!pendingRelease?.restore_deadline) return null
    const secs = pendingRelease.restore_deadline - Math.floor(nowMs / 1000)
    return Math.max(0, Math.ceil(secs / 86400))
  })()

  const actions: ReactNode = (
    <div className="vpn-actions" role="group" aria-label="VPN lifecycle">
      {suspended ? (
        <Button
          size="sm"
          variant="outline"
          leading={<PlayCircle size={14} />}
          loading={busy === 'resume'}
          onClick={() =>
            void run('resume', () => resume(vpn.apiId), 'VPN resumed', `${vpn.panelEmail} can connect again`)
          }
        >
          Resume
        </Button>
      ) : (
        <Button
          size="sm"
          variant="outline"
          leading={<Ban size={14} />}
          disabled={revoked}
          onClick={() => setConfirm('suspend')}
        >
          Suspend
        </Button>
      )}
      <Button
        size="sm"
        variant="danger"
        leading={<ShieldOff size={14} />}
        disabled={revoked}
        onClick={() => setConfirm('revoke')}
      >
        Revoke
      </Button>
      {canRemoveRecord && (
        <Button
          size="sm"
          variant="ghost"
          leading={<Trash2 size={14} />}
          onClick={() => setConfirm('remove')}
        >
          Remove record
        </Button>
      )}
      {/*
        D-027 for operators. Releasing was reachable from the client's own screen
        only, so a Super Admin had no way to set a VPN aside — and no way to undo
        it when a customer asked within the 3-day window.
      */}
      {isReleased && pendingRelease ? (
        <Button
          size="sm"
          variant="outline"
          leading={<Undo2 size={14} />}
          loading={busy === 'restore'}
          onClick={() =>
            void run(
              'restore',
              () => restoreRelease(pendingRelease.id),
              'Release undone',
              'The VPN is back to ACTIVE and can connect again.',
            )
          }
        >
          Restore{daysLeftToRestore !== null ? ` (${daysLeftToRestore}d left)` : ''}
        </Button>
      ) : (
        <Button
          size="sm"
          variant="ghost"
          leading={<RotateCcw size={14} />}
          disabled={revoked}
          onClick={() => setConfirm('release')}
        >
          Release
        </Button>
      )}
    </div>
  )

  const dialogs: ReactNode = (
    <>
      <Dialog
        open={confirm === 'suspend'}
        onClose={() => setConfirm(null)}
        title="Suspend this VPN?"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirm(null)}>Cancel</Button>
            <Button
              loading={busy === 'suspend'}
              onClick={() =>
                void run(
                  'suspend',
                  () => suspend(vpn.apiId),
                  'VPN suspended',
                  'The panel client is disabled. Nothing is deleted — Resume undoes this.',
                )
              }
            >
              Suspend
            </Button>
          </>
        }
      >
        <p className="flow-top-0">
          <strong>{vpn.panelEmail}</strong> stops connecting immediately. The panel client is
          kept, so this is reversible.
        </p>
        {error && <p className="ui-field__error" role="alert">{error}</p>}
      </Dialog>

      <Dialog
        open={confirm === 'revoke'}
        onClose={() => setConfirm(null)}
        title="Revoke this VPN?"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirm(null)}>Cancel</Button>
            <Button
              variant="danger"
              loading={busy === 'revoke'}
              onClick={() =>
                void run(
                  'revoke',
                  () => revoke(vpn.apiId),
                  'Revoke requested',
                  'Veylix reports REVOKED only once the panel confirms the client is gone.',
                )
              }
            >
              Revoke permanently
            </Button>
          </>
        }
      >
        <p className="flow-top-0">
          <strong>{vpn.panelEmail}</strong> is deleted from the panel. The credential stops
          working for good and cannot be restored — a new VPN has to be issued.
        </p>
        {vpn.status === 'REVOCATION_PENDING' && (
          <p className="vpn-warn t-sm2">
            <TriangleAlert size={14} aria-hidden="true" /> A revoke is already pending for this
            VPN. Repeating it is safe; the reconciler finishes it.
          </p>
        )}
        {error && <p className="ui-field__error" role="alert">{error}</p>}
      </Dialog>

      <Dialog
        open={confirm === 'remove'}
        onClose={() => {
          setConfirm(null)
          // Clearing the typed confirmation matters: reopening the dialog must
          // never find the destructive button already enabled.
          setRemoveConfirm('')
        }}
        title="Remove this VPN record?"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirm(null)}>Cancel</Button>
            <Button
              variant="danger"
              loading={busy === 'remove'}
              disabled={!removeConfirmed}
              onClick={() =>
                void run(
                  'remove',
                  () => removeAbsent(vpn.apiId),
                  'Record removed',
                  'The VPN is gone from Veylix. The panel was not touched.',
                )
              }
            >
              Remove record
            </Button>
          </>
        }
      >
        <p className="flow-top-0">
          <strong>{vpn.panelEmail}</strong> is no longer held by its panel, so this record
          describes a VPN nobody can connect to. Removing it clears the row from every list.
        </p>
        <p className="vpn-warn t-sm2">
          <TriangleAlert size={14} aria-hidden="true" /> The 3X-UI client is{' '}
          <strong>not</strong> deleted (D-027.3) — the panel is where an operator reuses or
          clears it. If the client still exists on the panel this is refused, and you should
          revoke instead.
        </p>
        {/* Typing the panel e-mail is the guard: this is the one action in the
            product that destroys a row with no undo. */}
        <Input
          label={`Type ${vpn.panelEmail} to confirm`}
          value={removeConfirm}
          onChange={(e) => setRemoveConfirm(e.target.value)}
          autoComplete="off"
          error={removeConfirm && !removeConfirmed ? 'The panel e-mail does not match.' : undefined}
        />
        {error && <p className="ui-field__error" role="alert">{error}</p>}
      </Dialog>

      <Dialog
        open={confirm === 'release'}
        onClose={() => setConfirm(null)}
        title="Release this VPN?"
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirm(null)}>Cancel</Button>
            <Button
              loading={busy === 'release'}
              onClick={() =>
                void run(
                  'release',
                  () => release(vpn.apiId, 'admin_release'),
                  'VPN released',
                  'A 3-day undo window is open: Restore puts it back. The panel client is kept.',
                )
              }
            >
              Release
            </Button>
          </>
        }
      >
        <p className="flow-top-0">
          <strong>{vpn.panelEmail}</strong> is set aside and marked RELEASED. A
          <strong> 3-day window</strong> opens during which Restore undoes this.
        </p>
        <p className="vpn-warn t-sm2">
          <TriangleAlert size={14} aria-hidden="true" /> The 3X-UI client itself is{' '}
          <strong>not deleted</strong> (D-027.3). When the window closes the record is
          removed from Veylix; the panel client stays, for you to reuse or clear.
        </p>
        {error && <p className="ui-field__error" role="alert">{error}</p>}
      </Dialog>
    </>
  )

  return {
    actions,
    dialogs,
    busy,
    error,
    /** The state chip, so a screen never has to guess which badge to draw. */
    statusChip: <StatusBadge status={vpn.status} />,
  }
}
