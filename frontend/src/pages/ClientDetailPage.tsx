import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  Badge,
  Button,
  Card,
  Dialog,
  EmptyState,
  Input,
  PageHeader,
  Skeleton,
  StatusBadge,
  Table,
  useToast,
  type Column,
} from '../components/ui'
import { ArrowLeft, Ban, CheckCircle2, Gauge, Globe, History, KeyRound, Mail, ShieldAlert, Trash2 } from 'lucide-react'
import { useClientAdminStore } from '../stores/clientAdminStore'
import { api, type ApiAdminVPNRow } from '../lib/api'
import { formatDateTime, relativeTime } from '../lib/time'
import { toKind } from '../stores/vpnStore'
import { VpnKindBadge } from '../components/vpn/VpnKindBadge'
import './clients.css'

/**
 * The columns of "Their VPNs".
 *
 * Declared at module scope because the set is fixed: a table that rebuilt its
 * columns each render would remount every row, and nothing here depends on the
 * profile being open.
 */
const VPN_COLUMNS: Column<ApiAdminVPNRow>[] = [
  {
    key: 'panel_email',
    header: 'VPN',
    render: (v) => (
      <span className="cl-cell__text">
        <span className="ui-strong">{v.panel_email}</span>
        {v.offer_name && <span className="ui-muted cl-cell__sub">{v.offer_name}</span>}
      </span>
    ),
  },
  {
    key: 'tag',
    header: 'Kind',
    // `||`, not `??` (D-063): the server sends "" for a VPN with no package, and
    // an empty string does not trigger a `??` fallback.
    render: (v) => <VpnKindBadge kind={toKind(v.offer_kind || v.tag, v.assigned)} />,
  },
  {
    key: 'status',
    header: 'Status',
    render: (v) => <StatusBadge status={v.status} />,
  },
  {
    key: 'usage_pct',
    header: 'Usage',
    align: 'center',
    render: (v) =>
      v.read_error ? (
        <span className="ui-muted" title="The panel could not be read">
          —
        </span>
      ) : (
        <span>{v.usage_pct}%</span>
      ),
  },
  {
    key: 'days_left',
    header: 'Expiry',
    render: (v) =>
      v.read_error ? (
        <span className="ui-muted">—</span>
      ) : v.days_left < 0 ? (
        <span className="cl-tag cl-tag--warn">expired</span>
      ) : v.days_left === 0 ? (
        <span className="ui-muted">no expiry</span>
      ) : (
        <span className={v.days_left <= 7 ? 'cl-tag cl-tag--warn' : ''}>{v.days_left}d left</span>
      ),
  },
  {
    key: 'open',
    header: '',
    align: 'right',
    render: (v) => (
      <Link
        to={`/vpns/${v.server_id}/${encodeURIComponent(v.panel_email)}`}
        className="cl-open"
      >
        Open →
      </Link>
    ),
  },
]

/**
 * Super Admin — one client's full record (D-042).
 *
 * Everything the owner asked to be visible lives here: who they are, how to reach
 * them, when they joined and last signed in, how much FREE they have used against
 * their limit, what premium they hold, and every operator action taken on them.
 *
 * The two writes differ in weight on purpose: a FREE limit is a number change,
 * while BLOCKING ends every session — so blocking asks first and says so.
 *
 * "Their VPNs" (Phase-2 UX, P4) lists the actual VPNs, not just how many. The page
 * used to report `Premium VPNs: 2` and `VPNs owned: 3` while showing none of them,
 * so answering "is this customer's VPN working?" meant leaving for `/vpns` and
 * searching by e-mail. The counts and the list now sit together.
 */
export function ClientDetailPage() {
  const { id } = useParams<{ id: string }>()
  const clientID = Number(id)
  const navigate = useNavigate()
  const toast = useToast()

  const detail = useClientAdminStore((s) => s.detail)
  const loading = useClientAdminStore((s) => s.detailLoading)
  const saving = useClientAdminStore((s) => s.saving)
  const error = useClientAdminStore((s) => s.detailError)
  const loadDetail = useClientAdminStore((s) => s.loadDetail)
  const setStatus = useClientAdminStore((s) => s.setStatus)
  const setFreeLimit = useClientAdminStore((s) => s.setFreeLimit)
  const resetPassword = useClientAdminStore((s) => s.resetPassword)
  const deleteClient = useClientAdminStore((s) => s.deleteClient)

  const [blockOpen, setBlockOpen] = useState(false)
  // The generated password is held only long enough to show it once.
  const [resetOpen, setResetOpen] = useState(false)
  const [issuedPassword, setIssuedPassword] = useState<string | null>(null)
  // Deleting is irreversible, so it asks the operator to type the address back.
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteConfirm, setDeleteConfirm] = useState('')
  /*
   * E-mail change (D-077): declared HERE, with the other state, because the
   * component returns early for a bad id / a loading detail / a missing record.
   * A hook after those returns is a Rules-of-Hooks violation — it is simply not
   * called on some renders, and `oxlint` refuses it.
   */
  const requestEmailChange = useClientAdminStore((s) => s.requestEmailChange)
  const confirmEmailChange = useClientAdminStore((s) => s.confirmEmailChange)
  const [emailOpen, setEmailOpen] = useState(false)
  const [newEmail, setNewEmail] = useState('')
  const [emailIssued, setEmailIssued] = useState<{ to: string; delivery: 'smtp' | 'outbox'; code?: string } | null>(null)
  const [emailCode, setEmailCode] = useState('')

  /**
   * This customer's VPNs, read once the profile arrives.
   *
   * `adminVpns({ q: email })` is the console's own server-side search, so the
   * list is the same rows `/vpns` would show — no second, richer source that
   * could disagree with it.
   */
  const [clientVpns, setClientVpns] = useState<ApiAdminVPNRow[]>([])
  const [vpnsLoading, setVpnsLoading] = useState(false)
  const email = detail?.profile.email ?? ''

  useEffect(() => {
    if (Number.isFinite(clientID) && clientID > 0) void loadDetail(clientID)
  }, [clientID, loadDetail])

  /* The VPN list needs the address, which only exists once the profile has
   * loaded — so it is a second effect keyed on it rather than a parallel fetch
   * that would have to guess the query. */
  useEffect(() => {
    if (!email) return
    let cancelled = false
    void (async () => {
      await Promise.resolve()
      if (cancelled) return
      setVpnsLoading(true)
      try {
        const res = await api.adminVpns({ q: email, limit: 100, offset: 0 })
        if (!cancelled) setClientVpns(res.vpns ?? [])
      } catch {
        // The VPN list is context, not the page: a failure must not blank the
        // profile the operator actually came here to read.
        if (!cancelled) setClientVpns([])
      } finally {
        if (!cancelled) setVpnsLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [email])

  if (!Number.isFinite(clientID) || clientID <= 0) {
    return (
      <div className="dash">
        <EmptyState title="Client not found" description="That link does not point at a client." />
      </div>
    )
  }

  if (loading && !detail) {
    return (
      <div className="dash">
        <Skeleton height={260} />
      </div>
    )
  }

  if (!detail) {
    return (
      <div className="dash">
        <EmptyState title="Client not found" description={error ?? 'This client no longer exists.'} />
      </div>
    )
  }

  const p = detail.profile
  const blocked = p.status !== 'ACTIVE'
  const initials = (p.display_name || p.email).slice(0, 1).toUpperCase()
  const doBlock = async () => {
    const reason = await setStatus(p.id, 'SUSPENDED')
    setBlockOpen(false)
    if (reason) return
    toast('success', 'Client blocked', 'Every session for this account was ended')
  }

  const doUnblock = async () => {
    const reason = await setStatus(p.id, 'ACTIVE')
    if (reason) return
    toast('success', 'Client unblocked', 'They can sign in again')
  }

  const saveLimit = async (n: number): Promise<string | null> => {
    if (!Number.isFinite(n) || n < 0 || n > 100) {
      return 'Use a number between 0 (default) and 100'
    }
    return setFreeLimit(p.id, n)
  }

  const doResetPassword = async () => {
    const { error: reason, password } = await resetPassword(p.id)
    if (reason) return
    setResetOpen(false)
    setIssuedPassword(password ?? null)
    toast('success', 'Password reset', 'Every session for this account was ended')
  }

  /**
   * E-mail change (D-077).
   *
   * Two steps on purpose: requesting mails a code to the NEW address, and the
   * address only moves when that code comes back. An operator correcting a typo
   * therefore cannot silently hand an account's sign-in identifier (and its
   * recovery target) to a mailbox nobody controls.
   *
   * `issued` holds the code when no relay is configured — the same honesty the
   * password reset uses: without it, "outbox" would be a dead end for exactly the
   * customer this feature exists to rescue.
   */
  const doRequestEmailChange = async () => {
    const res = await requestEmailChange(p.id, newEmail.trim())
    if ('error' in res && res.error) {
      toast('error', 'Could not start the change', res.error)
      return
    }
    // Narrowed: the success shape carries the delivery channel.
    const ok = res as { delivery: 'smtp' | 'outbox'; to: string; code?: string }
    setEmailIssued({ to: ok.to, delivery: ok.delivery, code: ok.code })
    setEmailCode('')
    if (ok.delivery === 'smtp') {
      toast('success', 'Code sent', `A confirmation code was e-mailed to ${ok.to}`)
    }
  }

  const doConfirmEmailChange = async () => {
    const reason = await confirmEmailChange(p.id, emailCode.trim())
    if (reason) {
      toast('error', 'Could not confirm', reason)
      return
    }
    setEmailOpen(false)
    setEmailIssued(null)
    setNewEmail('')
    toast('success', 'E-mail changed', 'The new address is their sign-in identifier now')
  }

  const doDelete = async () => {
    const reason = await deleteClient(p.id)
    if (reason) return
    setDeleteOpen(false)
    toast('success', 'Account deleted', 'They can no longer sign in; their history is kept')
    navigate('/clients')
  }

  // Deleting is terminal, so the button stays disabled until the address is typed
  // exactly. A single mis-click must not be able to remove a customer.
  const deleteConfirmed = deleteConfirm.trim().toLowerCase() === p.email.toLowerCase()

  const auditColumns: Column<(typeof detail.recent_audit)[number]>[] = [
    { key: 'created_at', header: 'When', render: (a) => <span className="ui-muted">{relativeTime(a.created_at)}</span> },
    { key: 'action', header: 'Action', render: (a) => <code className="cl-code">{a.action}</code> },
    {
      key: 'result',
      header: 'Result',
      render: (a) => <Badge tone={a.result === 'OK' ? 'success' : 'warning'}>{a.result || 'OK'}</Badge>,
    },
    { key: 'actor_id', header: 'By', hideOnMobile: true, render: (a) => (a.actor_id ? `#${a.actor_id}` : 'system') },
  ]

  const maxClaims = Math.max(...detail.monthly_claims.map((x) => x.count), 1)

  return (
    <div className="dash" data-testid="client-detail-page">
      <PageHeader
        title={p.display_name || p.email}
        subtitle={p.username ? '@' + p.username : p.email}
        actions={
          <>
            <Button variant="ghost" size="sm" leading={<ArrowLeft size={14} />} onClick={() => navigate('/clients')}>
              All clients
            </Button>
            {/* The customer's only way back in when their mailbox is wrong or gone:
                password reset and verification are both e-mails. */}
            <Button size="sm" variant="outline" leading={<KeyRound size={14} />}
              onClick={() => setResetOpen(true)}>
              Reset password
            </Button>
            {!blocked && (
              <Button size="sm" variant="ghost" leading={<Trash2 size={14} />}
                onClick={() => { setDeleteConfirm(''); setDeleteOpen(true) }}>
                Delete
              </Button>
            )}
            {blocked ? (
              <Button size="sm" leading={<CheckCircle2 size={14} />} loading={saving} onClick={() => void doUnblock()}>
                Unblock
              </Button>
            ) : (
              <Button size="sm" variant="danger" leading={<Ban size={14} />} onClick={() => setBlockOpen(true)}>
                Block client
              </Button>
            )}
          </>
        }
      />

      {error && <p className="cl-alert" role="alert">{error}</p>}

      <Card title="Identity">
        <div className="cl-identity">
          <span className="app-avatar cl-avatar cl-avatar--lg" aria-hidden="true">
            {p.avatar_url ? <img src={p.avatar_url} alt="" className="cl-avatar__img" /> : initials}
          </span>
          <dl className="cl-facts">
            <div><dt>Name</dt><dd>{p.display_name || '—'}</dd></div>
            <div><dt>Username</dt><dd>{p.username || '—'}</dd></div>
            <div>
              <dt>E-mail</dt>
              <dd>
                {p.email}{' '}
                {p.email_verified ? <Badge tone="success">verified</Badge> : <Badge tone="warning">unverified</Badge>}
                {p.google_linked && <Badge tone="info">Google</Badge>}
              </dd>
            </div>
            <div><dt>Status</dt><dd><StatusBadge status={p.status} /></dd></div>
            <div><dt>Age</dt><dd>{p.age > 0 ? p.age : '—'}</dd></div>
            <div><dt>Joined</dt><dd>{formatDateTime(p.created_at)}</dd></div>
            <div>
              <dt>Last sign-in</dt>
              <dd>
                {p.last_login_at > 0
                  ? `${formatDateTime(p.last_login_at)} · ${p.last_login_ip || 'unknown IP'}`
                  : 'never'}
              </dd>
            </div>
            <div><dt>Role</dt><dd><Badge tone="primary">{p.role}</Badge></dd></div>
          </dl>
        </div>
      </Card>

      <div className="cl-grid">
        <Card title="Contact">
          <dl className="cl-facts cl-facts--single">
            <div><dt>Phone</dt><dd>{p.phone || '—'}</dd></div>
            <div><dt>WhatsApp</dt><dd>{p.whatsapp || '—'}</dd></div>
            <div><dt>Telegram</dt><dd>{p.telegram || '—'}</dd></div>
            <div><dt>Discord</dt><dd>{p.discord || '—'}</dd></div>
          </dl>
        </Card>

        <Card title="Usage">
          <dl className="cl-facts cl-facts--single">
            <div>
              <dt>FREE claims</dt>
              <dd>
                <strong>{detail.free_used}</strong> / {detail.free_limit}
                {detail.free_limit_override > 0 && <span className="ui-muted"> (own limit)</span>}
                <span className="ui-muted"> in {detail.free_window_days} days</span>
              </dd>
            </div>
            <div><dt>Premium VPNs</dt><dd>{detail.premium_count}</dd></div>
            <div><dt>VPNs owned</dt><dd>{detail.vpn_count}</dd></div>
            <div>
              <dt>Sold by</dt>
              <dd>{detail.seller_ids.length > 0 ? detail.seller_ids.map((s) => `#${s}`).join(', ') : 'direct (platform)'}</dd>
            </div>
          </dl>
        </Card>
      </div>

      {/*
        Their VPNs (Phase-2 UX, P4). The Usage card reports how MANY; this lists
        WHICH — because "this customer's VPN is not working" is the most common
        support question and it cannot be answered from a count.
      */}
      <Card
        title="Their VPNs"
        actions={<span className="ui-muted t-sm">{clientVpns.length} listed</span>}
      >
        {vpnsLoading ? (
          <Skeleton height={80} />
        ) : clientVpns.length === 0 ? (
          <EmptyState
            icon={<Globe size={22} />}
            title="This customer holds no VPNs"
            description="Assign one from All VPNs, or let them claim a free offer themselves."
          />
        ) : (
          <Table
            columns={VPN_COLUMNS}
            data={clientVpns}
            rowKey={(v) => `${v.server_id}:${v.panel_email}`}
          />
        )}
      </Card>

      <Card title="FREE VPN limit">
        <p className="ui-muted mt-0 mb-3 text-[12.5px]">
          A per-client override of the platform cap. <strong>0</strong> means "use the
          platform default". This is the number the client is measured against when they
          claim a free VPN.
        </p>
        <FreeLimitEditor
          key={p.id + ':' + detail.free_limit_override}
          value={detail.free_limit_override}
          saving={saving}
          onSave={saveLimit}
        />
      </Card>

      <Card title="Claim history">
        {detail.monthly_claims.every((m) => m.count === 0) ? (
          <EmptyState
            icon={<History size={20} />}
            title="No FREE claims yet"
            description="Nothing claimed in the last 6 months."
          />
        ) : (
          <div className="cl-bars" role="img" aria-label="FREE claims per month">
            {detail.monthly_claims.map((m) => (
              <div key={m.label} className="cl-bar">
                <span className="cl-bar__value">{m.count}</span>
                <span className="cl-bar__fill" style={{ height: `${(m.count / maxClaims) * 100}%` }} />
                <span className="cl-bar__label">{m.label.slice(5)}</span>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card title="Operator activity">
        {detail.recent_audit.length === 0 ? (
          <EmptyState
            icon={<ShieldAlert size={20} />}
            title="No actions recorded"
            description="Nothing has been done to this account yet."
          />
        ) : (
          <Table columns={auditColumns} data={detail.recent_audit} rowKey={(a) => String(a.id)} />
        )}
      </Card>

      <p className="ui-muted cl-note">
        The customer can change their own address from Profile → Security (a code goes to
        the new address). If their mailbox is wrong or dead, <strong>change it here</strong> —
        the code is sent to the new address and nothing moves until it comes back.{' '}
        <Button size="sm" variant="outline" leading={<Mail size={14} />} onClick={() => {
          setEmailIssued(null)
          setNewEmail('')
          setEmailOpen(true)
        }}>
          Change e-mail
        </Button>{' '}
        <Link to="/clients">Back to all clients</Link>
      </p>

      <Dialog
        open={blockOpen}
        onClose={() => setBlockOpen(false)}
        title="Block this client?"
        footer={
          <>
            <Button variant="outline" onClick={() => setBlockOpen(false)}>Cancel</Button>
            <Button variant="danger" loading={saving} onClick={() => void doBlock()}>
              Block client
            </Button>
          </>
        }
      >
        <p className="mt-0">
          <strong>{p.display_name || p.email}</strong> will be signed out of every device
          immediately and cannot sign in again until you unblock them. Their VPNs stay on
          the panel; this only stops their access to Veylix.
        </p>
      </Dialog>

      <Dialog
        open={resetOpen}
        onClose={() => setResetOpen(false)}
        title="Reset this client's password?"
        footer={
          <>
            <Button variant="outline" onClick={() => setResetOpen(false)}>Cancel</Button>
            <Button loading={saving} onClick={() => void doResetPassword()}>
              Reset password
            </Button>
          </>
        }
      >
        <p className="mt-0">
          A new password will be generated for <strong>{p.display_name || p.email}</strong> and
          shown to you <strong>once</strong>. Every session they have will be ended, so any
          device they are signed in on will need the new password.
        </p>
        <p className="ui-muted t-sm2">
          Use this when their inbox is wrong or unreachable — the normal recovery e-mails
          cannot reach them.
        </p>
      </Dialog>

      <Dialog
        open={issuedPassword !== null}
        onClose={() => setIssuedPassword(null)}
        title="New password"
        footer={
          <Button onClick={() => setIssuedPassword(null)}>Done</Button>
        }
      >
        <p className="mt-0">
          Give this to <strong>{p.display_name || p.email}</strong> now. It is
          <strong> not stored anywhere</strong> and cannot be shown again — reset again if
          it is lost.
        </p>
        <code
          className="cl-issued-password"
          style={{
            display: 'block', padding: '12px 14px', borderRadius: 8,
            fontSize: 16, letterSpacing: '0.5px', wordBreak: 'break-all',
            background: 'var(--bg-surface)', border: '1px solid var(--border-strong)',
          }}
        >
          {issuedPassword}
        </code>
        <p className="ui-muted t-sm2">
          Ask them to change it from <code>/profile</code> once they are back in.
        </p>
      </Dialog>

      <Dialog
        open={emailOpen}
        onClose={() => { setEmailOpen(false); setEmailIssued(null) }}
        title={emailIssued ? 'Enter the confirmation code' : 'Change this client’s e-mail'}
        footer={
          <>
            <Button variant="outline" onClick={() => { setEmailOpen(false); setEmailIssued(null) }}>
              Cancel
            </Button>
            {emailIssued ? (
              <Button loading={saving} disabled={emailCode.trim().length !== 6}
                onClick={() => void doConfirmEmailChange()}>
                Confirm change
              </Button>
            ) : (
              <Button loading={saving} disabled={!newEmail.trim()}
                onClick={() => void doRequestEmailChange()}>
                Send code
              </Button>
            )}
          </>
        }
      >
        {!emailIssued ? (
          <>
            <p className="mt-0">
              A 6-digit code is sent to the <strong>new</strong> address. The account keeps{' '}
              <strong>{p.email}</strong> as its sign-in identifier until that code is entered,
              so a mistyped address cannot move the account.
            </p>
            <Input
              label="New e-mail address"
              type="email"
              autoComplete="off"
              placeholder="corrected@example.com"
              value={newEmail}
              onChange={(e) => setNewEmail(e.target.value)}
            />
          </>
        ) : (
          <>
            <p className="mt-0">
              The code was sent to <strong>{emailIssued.to}</strong>. Ask them to read it out,
              or open that mailbox now.
            </p>
            {emailIssued.code && (
              <p className="ui-muted t-sm2">
                No mail relay is configured on this server, so the code is shown here
                instead:{' '}
                <code style={{ fontSize: 15, letterSpacing: '2px' }}>{emailIssued.code}</code>
              </p>
            )}
            <Input
              label="6-digit code"
              inputMode="numeric"
              autoComplete="one-time-code"
              placeholder="••••••"
              value={emailCode}
              onChange={(e) => setEmailCode(e.target.value.replace(/[^0-9]/g, '').slice(0, 6))}
            />
          </>
        )}
      </Dialog>

      <Dialog
        open={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        title="Delete this client account?"
        footer={
          <>
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>Cancel</Button>
            <Button variant="danger" disabled={!deleteConfirmed} loading={saving}
              onClick={() => void doDelete()}>
              Delete account
            </Button>
          </>
        }
      >
        <p className="mt-0">
          <strong>{p.display_name || p.email}</strong> will be locked out for good. This cannot
          be undone from the console — DELETED is final.
        </p>
        <p className="ui-muted t-sm2">
          Their VPNs, payments and audit history are kept, so billing and the panel stay
          consistent. If they still own VPNs the delete is refused — revoke or reassign those
          first.
        </p>
        <Input
          label={`Type ${p.email} to confirm`}
          value={deleteConfirm}
          onChange={(e) => setDeleteConfirm(e.target.value)}
          autoComplete="off"
          error={deleteConfirm && !deleteConfirmed ? 'The address does not match.' : undefined}
        />
      </Dialog>
    </div>
  )

/**
 * The FREE-limit editor.
 *
 * Its draft lives in `useState`, initialised from the prop, and the parent gives it
 * a `key` built from the saved value — so a save remounts it with the new number
 * and typing is never overwritten by a background refresh. (An effect that copied
 * the prop into state would fight the person using the field.)
 */
function FreeLimitEditor({
  value,
  saving,
  onSave,
}: {
  value: number
  saving: boolean
  onSave: (n: number) => Promise<string | null>
}) {
  const toast = useToast()
  const [draft, setDraft] = useState(() => String(value))

  const submit = async () => {
    const n = Number(draft)
    const err = await onSave(n)
    if (err) {
      toast('error', 'Could not save', err)
      return
    }
    toast('success', 'FREE limit saved', n === 0 ? 'Using the platform default' : `${n} per 30 days`)
  }

  return (
    <div className="cl-inline">
      <Input
        label="FREE VPNs per 30 days"
        type="number"
        min={0}
        max={100}
        value={draft}
        hint="0 = platform default"
        onChange={(e) => setDraft(e.target.value.replace(/[^0-9]/g, ''))}
      />
      <Button loading={saving} leading={<Gauge size={14} />} onClick={() => void submit()}>
        Save limit
      </Button>
    </div>
  )
}

}
