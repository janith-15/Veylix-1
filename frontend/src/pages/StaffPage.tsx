import { useEffect, useMemo, useState } from 'react'
import {
  AsyncState,
  Badge,
  Button,
  Card,
  Dialog,
  EmptyState,
  Input,
  PageHeader,
  Pagination,
  Select,
  Skeleton,
  StatusBadge,
  Table,
  useToast,
  type Column,
} from '../components/ui'
import { UserCog, UserPlus } from 'lucide-react'
import { api, type ApiStaffDetail, type ApiStaffQuery, type ApiStaffRow } from '../lib/api'
import { useListQuery } from '../hooks/useListQuery'
import { useServersStore } from '../stores/serversStore'
import { formatDateTime, relativeTime } from '../lib/time'
import './clients.css'
// D-106: the quota bar's classes live in admins.css. Without this import the bar
// rendered as unstyled text — the numbers were right and the meter was invisible,
// so a reseller at 19/20 slots looked identical to one at 1/20.
import './admins.css'

/**
 * Super Admin — Staff & Admin management (D-044).
 *
 * The third console: who else holds power, how much they may sell, which panels
 * they may touch, and the two things only a Super Admin can do — create an account
 * and hand back its password.
 *
 * This page absorbed `/admins` (Phase-1 UX, P1.3). The two screens carried the
 * SAME page title — "Admin management" — while one owned the profile and the
 * password and the other owned the quota and the panel scope, so the operator had
 * to guess which one held the control they needed, and the command palette only
 * knew about one of them. Everything now lives here: list → open an account →
 * profile, quota, panel scope, password, role, suspend.
 * `/admins` redirects here so old bookmarks keep working.
 *
 * A created password is shown ONCE in a dialog and never stored: it is the
 * operator's job to pass it on, and the person's job to change it at first
 * sign-in.
 */
export function StaffPage() {
  const toast = useToast()
  const [rows, setRows] = useState<ApiStaffRow[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  /** Filters live in the URL (D-051); the text box is debounced. */
  const q = useListQuery({
    defaults: {
      q: '',
      role: '',
      status: '',
      quota_min: '',
      server_id: '',
      sort: '',
      page: '1',
    },
    debounced: ['q'],
  })

  /**
   * Rows per page. MUST match `store.StaffDefaultLimit` on the backend: the server
   * returns `total` (the whole filtered set) while only this many rows come back,
   * so a page count divided by anything else is arithmetic about nothing (B-1).
   */
  const PAGE_SIZE = 50
  const pageNumber = Math.max(1, Number(q.values.page) || 1)
  /**
   * The page count is derived from the page SIZE, never from the page number.
   *
   * U-2: this console had no pager at all — the store clamped `limit` and the API
   * returned `total`, but the screen offered no way to reach page 2, so a reseller
   * list longer than one page was silently truncated. The arithmetic below is the
   * same expression ClientsPage uses, deliberately, so the two cannot drift.
   */
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))

  // Create dialog.
  const [createOpen, setCreateOpen] = useState(false)
  const [newEmail, setNewEmail] = useState('')
  const [newName, setNewName] = useState('')
  const [newRole, setNewRole] = useState<'admin' | 'super_admin'>('admin')
  const [newQuota, setNewQuota] = useState('25')
  const [busy, setBusy] = useState(false)

  // The one-time password, shown until dismissed.
  const [issued, setIssued] = useState<{ password: string; email: string } | null>(null)

  // Selected staff account.
  const [detail, setDetail] = useState<ApiStaffDetail | null>(null)
  const [quotaDraft, setQuotaDraft] = useState('')
  /**
   * The panels this account may operate, edited in place.
   *
   * Server scope used to live on a SEPARATE page (`/admins`) whose title was
   * identical to this one's, so "where do I assign a reseller's panels?" had two
   * plausible answers and neither was obvious. It belongs beside the quota it
   * constrains: quota says how many, scope says where.
   */
  const [scopeDraft, setScopeDraft] = useState<number[] | null>(null)
  const [savingScope, setSavingScope] = useState(false)
  const servers = useServersStore((s) => s.servers)
  const loadServers = useServersStore((s) => s.load)

  const load = async (next: ApiStaffQuery) => {
    await Promise.resolve()
    setLoading(true)
    setError(null)
    try {
      const res = await api.adminStaff(next)
      setRows(res.staff ?? [])
      setTotal(res.total ?? 0)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not read the staff list')
      setRows([])
    } finally {
      setLoading(false)
    }
  }

  /** The committed filters, memoised so the effect fires per change and not per render. */
  const committed: ApiStaffQuery = useMemo(
    () => ({
      q: q.values.q || undefined,
      role: (q.values.role || undefined) as ApiStaffQuery['role'],
      status: (q.values.status || undefined) as ApiStaffQuery['status'],
      quota_min: q.values.quota_min ? Number(q.values.quota_min) : undefined,
      server_id: q.values.server_id ? Number(q.values.server_id) : undefined,
      sort: (q.values.sort || undefined) as ApiStaffQuery['sort'],
      // U-2: the page travels with the filters, so paging survives a filter change
      // and a reload (D-051: state lives in the URL).
      page: pageNumber,
      limit: PAGE_SIZE,
    }),
    [q.values, pageNumber],
  )

  // One request per committed filter change (D-051), never on the draft.
  // oxlint-disable-next-line react-hooks/exhaustive-deps -- load sets state after an await, not during commit
  useEffect(() => {
    void load(committed)
  }, [committed])

  /* The panel list is needed by the scope editor in the detail card, so it is
   * loaded once on mount rather than each time an account is opened. */
  useEffect(() => {
    void loadServers()
  }, [loadServers])

  const openDetail = async (row: ApiStaffRow) => {
    try {
      const d = await api.adminStaffOne(row.id)
      setDetail(d)
      setQuotaDraft(String(d.profile.quota_limit))
      setScopeDraft(d.servers.map((s) => s.id))
    } catch {
      toast('error', 'Could not open that account', 'Try refreshing the list')
    }
  }

  const create = async () => {
    if (!newEmail.trim() || !newName.trim()) {
      toast('error', 'Missing fields', 'An e-mail and a name are required')
      return
    }
    setBusy(true)
    try {
      const email = newEmail.trim()
      const res = await api.createAdminStaff({
        email,
        display_name: newName.trim(),
        role: newRole,
        quota_limit: Number(newQuota) || 0,
      })
      setCreateOpen(false)
      setNewEmail('')
      setNewName('')
      setNewQuota('25')
      setIssued({ password: res.password, email })
      void load(committed)
    } catch (err) {
      toast('error', 'Could not create the account', err instanceof Error ? err.message : 'unknown error')
    } finally {
      setBusy(false)
    }
  }

  const resetPassword = async (row: ApiStaffRow) => {
    try {
      const res = await api.resetAdminStaffPassword(row.id)
      setIssued({ password: res.password, email: row.email })
      toast('success', 'Password reset', 'Every session for that account was ended')
    } catch (err) {
      toast('error', 'Could not reset', err instanceof Error ? err.message : 'unknown error')
    }
  }

  const setStatus = async (row: ApiStaffRow, status: 'ACTIVE' | 'SUSPENDED') => {
    try {
      await api.updateAdminStaff(row.id, { status })
      toast(
        'success',
        status === 'ACTIVE' ? 'Reactivated' : 'Suspended',
        status === 'ACTIVE' ? 'They can sign in again' : 'Every session for that account was ended',
      )
      void load(committed)
      if (detail?.profile.id === row.id) void openDetail(row)
    } catch (err) {
      toast('error', 'Could not change the status', err instanceof Error ? err.message : 'unknown error')
    }
  }

  const saveQuota = async (row: ApiStaffRow) => {
    const n = Number(quotaDraft)
    if (!Number.isFinite(n) || n < 0 || n > 10000) {
      toast('error', 'Invalid quota', 'Use a number between 0 and 10000')
      return
    }
    try {
      await api.updateAdminStaff(row.id, { quota_limit: n })
      toast('success', 'Quota saved', `${n} clients`)
      void load(committed)
      if (detail?.profile.id === row.id) void openDetail(row)
    } catch (err) {
      toast('error', 'Could not save the quota', err instanceof Error ? err.message : 'unknown error')
    }
  }

  /**
   * Replaces the panel scope of one account (D-021).
   *
   * PUT, not PATCH: the backend takes the WHOLE set, because a partial add/remove
   * would make "this reseller now sees exactly these panels" impossible to
   * express in one idempotent request — and a retry must not silently add a
   * second copy of the same panel.
   */
  const saveScope = async (row: ApiStaffRow) => {
    if (scopeDraft === null) return
    setSavingScope(true)
    try {
      await api.setAdminServers(row.id, scopeDraft)
      toast(
        'success',
        scopeDraft.length === 0 ? 'No panels assigned' : 'Panel scope saved',
        scopeDraft.length === 0
          ? 'This account can no longer sell on any server.'
          : `${scopeDraft.length} panel(s) — this account can only sell there.`,
      )
      void load(committed)
      if (detail?.profile.id === row.id) void openDetail(row)
    } catch (err) {
      toast('error', 'Could not save the scope', err instanceof Error ? err.message : 'unknown error')
    } finally {
      setSavingScope(false)
    }
  }


  const changeRole = async (row: ApiStaffRow, role: 'admin' | 'super_admin') => {
    try {
      await api.updateAdminStaff(row.id, { role })
      toast('success', 'Role changed', role === 'admin' ? 'Now an administrator' : 'Now a Super Admin')
      void load(committed)
      if (detail?.profile.id === row.id) void openDetail(row)
    } catch (err) {
      toast('error', 'Could not change the role', err instanceof Error ? err.message : 'unknown error')
    }
  }

  const columns: Column<ApiStaffRow>[] = [
    {
      key: 'display_name',
      header: 'Staff member',
      sortable: true,
      render: (s) => (
        <button type="button" className="cl-cell cl-cell--btn" onClick={() => void openDetail(s)}>
          <span className="app-avatar cl-avatar" aria-hidden="true">
            {s.avatar_url ? (
              <img src={s.avatar_url} alt="" className="cl-avatar__img" />
            ) : (
              (s.display_name || s.email).slice(0, 1).toUpperCase()
            )}
          </span>
          <span className="cl-cell__text">
            <span className="ui-strong">{s.display_name || s.email}</span>
            <span className="ui-muted cl-cell__sub">{s.username ? '@' + s.username : s.email}</span>
          </span>
        </button>
      ),
    },
    {
      key: 'role',
      header: 'Role',
      sortable: true,
      render: (s) =>
        s.role === 'super_admin' ? (
          <Badge tone="primary">Super Admin</Badge>
        ) : (
          <Badge tone="info">Admin</Badge>
        ),
    },
    { key: 'status', header: 'Status', sortable: true, render: (s) => <StatusBadge status={s.status} /> },
    {
      key: 'quota_limit',
      header: 'Quota',
      sortable: true,
      align: 'center',
      /*
       * Used / limit with a bar (Phase-2 UX, P5).
       *
       * The number alone was not enough: "28 / 30" and "2 / 30" render at the same
       * width, so a reseller about to run out looked like one with room to spare.
       * `/api/super/staff` has always returned `quota_used`; only the bar was
       * missing (it lived on the now-merged `/admins` page).
       */
      render: (s) =>
        s.role === 'super_admin' ? (
          <span className="ui-muted">∞</span>
        ) : s.quota_limit <= 0 ? (
          <span className="cl-free">unlimited</span>
        ) : (
          <span className="cl-quotabar" title={`${s.quota_used} of ${s.quota_limit} active assignments`}>
            <span className="cl-quotabar__num">
              {s.quota_used} / {s.quota_limit}
            </span>
            <span className="cl-quotabar__track">
              <span
                className={`cl-quotabar__fill${
                  s.quota_used >= s.quota_limit
                    ? ' is-full'
                    : s.quota_used / s.quota_limit >= 0.8
                      ? ' is-high'
                      : ''
                }`}
                style={{ ['--quota-pct' as string]: `${Math.min(100, Math.round((s.quota_used / s.quota_limit) * 100))}%` }}
              />
            </span>
          </span>
        ),
    },
    {
      key: 'server_ids',
      header: 'Servers',
      align: 'center',
      hideOnMobile: true,
      render: (s) =>
        s.server_ids.length > 0 ? (
          s.server_ids.map((id) => `#${id}`).join(', ')
        ) : (
          <span className="ui-muted">none</span>
        ),
    },
    {
      key: 'last_login_at',
      header: 'Last sign-in',
      hideOnMobile: true,
      render: (s) =>
        s.last_login_at > 0 ? (
          <span className="ui-muted">{relativeTime(s.last_login_at)}</span>
        ) : (
          <span className="ui-muted">never</span>
        ),
    },
    {
      key: 'actions',
      header: '',
      sticky: true,
      align: 'right',
      render: (s) => (
        <span className="cl-actions">
          <button type="button" className="cl-open" onClick={() => void openDetail(s)}>Open</button>
          <button
            type="button"
            className="cl-open"
            title="Generate a new password for this account"
            onClick={() => void resetPassword(s)}
          >
            reset
          </button>
        </span>
      ),
    },
  ]


  return (
    <div className="dash" data-testid="staff-page">
      <PageHeader
        title="Staff & Admins"
        subtitle="Who can sell, how much, and on which panels — profile, quota, panel scope and password in one place"
        actions={
          <Button size="sm" leading={<UserPlus size={14} />} onClick={() => setCreateOpen(true)}>
            New admin
          </Button>
        }
      />

      <Card>
        <form className="cl-filters" onSubmit={(e) => e.preventDefault()}>
          <div className="cl-filters__search">
            <Input
              label="Search"
              placeholder="name, e-mail or username"
              value={q.draft.q}
              onChange={(e) => q.set({ q: e.target.value })}
              onBlur={() => q.setNow({ q: q.draft.q })}
            />
          </div>
          <Select label="Role" value={q.draft.role} onChange={(e) => q.set({ role: e.target.value })}>
            <option value="">All staff</option>
            <option value="admin">Administrators</option>
            <option value="super_admin">Super Admins</option>
          </Select>
          <Select label="Status" value={q.draft.status} onChange={(e) => q.set({ status: e.target.value })}>
            <option value="">Any status</option>
            <option value="ACTIVE">Active</option>
            <option value="SUSPENDED">Suspended</option>
          </Select>
          <Input
            label="Quota ≥"
            type="number"
            min={0}
            value={q.draft.quota_min}
            onChange={(e) => q.set({ quota_min: e.target.value })}
          />
          <Input
            label="Server id"
            type="number"
            min={0}
            value={q.draft.server_id}
            onChange={(e) => q.set({ server_id: e.target.value })}
          />
          <Select label="Sort" value={q.draft.sort} onChange={(e) => q.set({ sort: e.target.value })}>
            <option value="">Newest first</option>
            <option value="oldest">Oldest first</option>
            <option value="name">Name</option>
            <option value="email">E-mail</option>
            <option value="quota">Largest quota</option>
          </Select>
        </form>
      </Card>

      <Card title={total > 0 ? `Staff · ${total}` : 'Staff'}>
        <AsyncState
          status={error ? 'error' : loading ? 'loading' : rows.length === 0 ? 'empty' : 'ready'}
          error={error}
          onRetry={() => void load(committed)}
          skeleton={<Skeleton height={200} />}
          empty={
            q.isFiltered ? (
              <EmptyState
                icon={<UserCog size={22} />}
                title="No staff match these filters"
                description="Widen the filters, or clear them."
                action={
                  <Button variant="outline" size="sm" onClick={q.reset}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={<UserCog size={22} />}
                title="No staff accounts yet"
                description="Create the first administrator with “New admin”."
              />
            )
          }
        >
          <Table columns={columns} data={rows} rowKey={(s) => String(s.id)} />
          {/* U-2: only rendered when there IS a second page — a pager for one page
              is clutter that teaches nothing (NN/g: do not show controls that cannot
              do anything). */}
          {pageCount > 1 && (
            <Pagination
              page={pageNumber}
              pageCount={pageCount}
              onChange={(next) => q.set({ page: String(next) })}
            />
          )}
        </AsyncState>
      </Card>

      {detail && (
        <Card
          title={`${detail.profile.display_name || detail.profile.email} — ${detail.profile.role === 'super_admin' ? 'Super Admin' : 'Administrator'}`}
        >
          <dl className="cl-facts">
            <div><dt>E-mail</dt><dd>{detail.profile.email}</dd></div>
            <div><dt>Username</dt><dd>{detail.profile.username || '—'}</dd></div>
            <div><dt>Status</dt><dd><StatusBadge status={detail.profile.status} /></dd></div>
            <div><dt>Joined</dt><dd>{formatDateTime(detail.profile.created_at)}</dd></div>
            <div>
              <dt>Last sign-in</dt>
              <dd>{detail.profile.last_login_at > 0 ? formatDateTime(detail.profile.last_login_at) : 'never'}</dd>
            </div>
            <div><dt>Clients sold to</dt><dd>{detail.clients}</dd></div>
            <div><dt>VPNs sold</dt><dd>{detail.vpns_sold}</dd></div>
            <div><dt>Quota used</dt><dd>{detail.profile.quota_used} / {detail.profile.quota_limit}</dd></div>
            <div>
              <dt>Panel scope</dt>
              <dd>{detail.servers.length > 0 ? detail.servers.map((s) => s.name).join(', ') : 'none assigned'}</dd>
            </div>
          </dl>

          <div className="cl-inline cl-inline--wrap">
            <Input
              label="Quota (clients)"
              type="number"
              min={0}
              value={quotaDraft}
              onChange={(e) => setQuotaDraft(e.target.value.replace(/[^0-9]/g, ''))}
            />
            <Button loading={busy} onClick={() => void saveQuota(detail.profile)}>Save quota</Button>
            {detail.profile.role === 'admin' && (
              <Button
                variant="outline"
                loading={savingScope}
                onClick={() => void saveScope(detail.profile)}
              >
                Save panel scope
              </Button>
            )}
            {detail.profile.status === 'ACTIVE' ? (
              <Button variant="danger" onClick={() => void setStatus(detail.profile, 'SUSPENDED')}>
                Suspend
              </Button>
            ) : (
              <Button onClick={() => void setStatus(detail.profile, 'ACTIVE')}>Reactivate</Button>
            )}
            {detail.profile.role === 'admin' ? (
              <Button variant="outline" onClick={() => void changeRole(detail.profile, 'super_admin')}>
                Promote to Super Admin
              </Button>
            ) : (
              <Button variant="outline" onClick={() => void changeRole(detail.profile, 'admin')}>
                Demote to Admin
              </Button>
            )}
            <Button variant="ghost" onClick={() => setDetail(null)}>Close</Button>
          </div>

          <p className="ui-muted prof-note">
            You cannot suspend, demote or reset your own account here — do that from
            Profile → Security, which asks for your current password.
          </p>
        </Card>
      )}

      {/*
        Panel scope editor (absorbed from /admins, P1.3).
        Shown as its own card because it is the one control on this page with a
        blast radius beyond the account: changing it changes which customers this
        reseller can reach at all, so it reads better as a decision than as a
        checkbox wedged between Quota and Suspend.
      */}
      {detail && detail.profile.role === 'admin' && scopeDraft && (
        <Card
          title="Panel scope"
          actions={
            <span className="ui-muted t-sm">
              {scopeDraft.length} of {servers.length} panel(s)
            </span>
          }
        >
          <p className="ui-muted t-sm2 st-dialog-note">
            This account can only sell on the panels selected here. A panel left unchecked is
            invisible to them — they cannot list it, sell on it, or see its clients.
          </p>
          {servers.length === 0 ? (
            <p className="ui-muted rhythm">
              No 3X-UI server is integrated yet, so there is nothing to scope.
            </p>
          ) : (
            <div className="ui-col st-scope-list">
              {servers.map((srv) => {
                const on = scopeDraft.includes(srv.apiId)
                return (
                  <label key={srv.id} className="ui-row st-scope-row">
                    <input
                      type="checkbox"
                      checked={on}
                      onChange={() =>
                        setScopeDraft(
                          on
                            ? scopeDraft.filter((id) => id !== srv.apiId)
                            : [...scopeDraft, srv.apiId],
                        )
                      }
                    />
                    <span>{srv.name}</span>
                    <StatusBadge status={srv.status} />
                  </label>
                )
              })}
            </div>
          )}
          {scopeDraft.length === 0 && (
            <p className="vpn-warn t-sm2 st-dialog-warn">
              With nothing selected this account cannot sell anything. That is a legal state — it
              is how you park a reseller without deleting their history.
            </p>
          )}
        </Card>
      )}

      <p className="ui-muted cl-note">
        A created password is shown once. Share it, and the person changes it at first
        sign-in.
      </p>


      <Dialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        title="New staff account"
        footer={
          <>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>Cancel</Button>
            <Button loading={busy} onClick={() => void create()}>Create account</Button>
          </>
        }
      >
        <div className="ui-col">
          <Input
            label="E-mail"
            type="email"
            placeholder="nimal@example.com"
            value={newEmail}
            onChange={(e) => setNewEmail(e.target.value)}
          />
          <Input
            label="Display name"
            placeholder="Nimal Silva"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
          />
          <Select
            label="Role"
            value={newRole}
            onChange={(e) => setNewRole(e.target.value === 'super_admin' ? 'super_admin' : 'admin')}
          >
            <option value="admin">Administrator (reseller)</option>
            <option value="super_admin">Super Admin</option>
          </Select>
          <Input
            label="Quota (clients they may sell to)"
            type="number"
            min={0}
            value={newQuota}
            hint="Administrators only; a Super Admin has no ceiling"
            onChange={(e) => setNewQuota(e.target.value.replace(/[^0-9]/g, ''))}
          />
          <p className="ui-muted prof-note">
            The password is generated by the server and shown once, so it is never
            something you had to invent or remember.
          </p>
        </div>
      </Dialog>

      <Dialog
        open={!!issued}
        onClose={() => setIssued(null)}
        title="Password for this account"
        footer={<Button onClick={() => setIssued(null)}>I have saved it</Button>}
      >
        {issued && (
          <div className="ui-col">
            <p className="mt-0">
              Hand this to <strong>{issued.email}</strong>. It is shown once and cannot be
              read again — use <em>reset</em> on the row if it is lost.
            </p>
            <p className="cl-once"><code>{issued.password}</code></p>
            <p className="ui-muted prof-note">
              They should change it at first sign-in: Profile → Security.
            </p>
          </div>
        )}
      </Dialog>
    </div>
  )
}

