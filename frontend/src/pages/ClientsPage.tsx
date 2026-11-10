import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  AsyncState,
  Button,
  Card,
  Dialog,
  EmptyState,
  Input,
  PageHeader,
  Pagination,
  Select,
  SelectionBar,
  Skeleton,
  StatusBadge,
  useToast,
  Table,
  type Column,
} from '../components/ui'
import { Check, UserX, Users } from 'lucide-react'
import { useClientAdminStore } from '../stores/clientAdminStore'
import type { ApiAdminClientRow, ApiClientQuery } from '../lib/api'
import { useListQuery } from '../hooks/useListQuery'
import { relativeTime } from '../lib/time'
import './clients.css'

/**
 * Super Admin — Client Management (D-042).
 *
 * Filters are SERVER-SIDE: each control calls `/api/super/clients` with a query,
 * so the page stays correct with thousands of customers and a filter cannot be
 * bypassed by editing the request. Every row links to the detail page, where the
 * block toggle and the FREE limit live.
 */
export function ClientsPage() {
  const rows = useClientAdminStore((s) => s.rows)
  const total = useClientAdminStore((s) => s.total)
  const loading = useClientAdminStore((s) => s.loading)
  const error = useClientAdminStore((s) => s.error)
  const load = useClientAdminStore((s) => s.load)
  const setQuery = useClientAdminStore((s) => s.setQuery)
  const setStatus = useClientAdminStore((s) => s.setStatus)

  /**
   * Bulk selection (D-054). Blocking fifty clients one dialog at a time is not a
   * feature an operator will use, so selection lives here and the action bar acts on
   * every selected id. The confirm step is mandatory: a bulk block ends every
   * session of every selected account, and that must never be one misclick away.
   */
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [bulkBusy, setBulkBusy] = useState(false)
  const [bulkConfirm, setBulkConfirm] = useState<'ACTIVE' | 'SUSPENDED' | null>(null)
  const [bulkResult, setBulkResult] = useState<string | null>(null)
  const toast = useToast()

  /**
   * Filters live in the URL (D-051): a filtered client list is a thing an operator
   * wants to refresh, bookmark and send to a colleague. Text is debounced so typing
   * does not fire one SQL query per keystroke; every other control applies at once.
   */
  const q = useListQuery({
    defaults: {
      q: '',
      status: '',
      has_free: '',
      has_premium: '',
      google: '',
      from: '',
      to: '',
      free_min: '',
      sort: '',
      page: 1,
    },
    debounced: ['q'],
  })

  // Push the committed filters into the store, which owns the request and cache.
  useEffect(() => {
    void setQuery({
      q: q.values.q || undefined,
      status: q.values.status || undefined,
      has_free: q.values.has_free === '' ? undefined : q.values.has_free === 'yes',
      has_premium: q.values.has_premium === '' ? undefined : q.values.has_premium === 'yes',
      google: q.values.google === '' ? undefined : q.values.google === 'yes',
      from: q.values.from || undefined,
      to: q.values.to || undefined,
      free_min: q.values.free_min ? Number(q.values.free_min) : undefined,
      sort: (q.values.sort || undefined) as ApiClientQuery['sort'],
      page: q.values.page,
    })
    // Only the committed values matter; the draft is UI-only.
  }, [q.values, setQuery])

  const columns: Column<ApiAdminClientRow>[] = [
    {
      key: 'display_name',
      header: 'Client',
      sortable: true,
      // Sorted by name, falling back to the email the card shows when there is
      // no display name.
      sortValue: (c) => c.display_name || c.email,
      searchValue: (c) => `${c.display_name ?? ''} ${c.email} ${c.username ?? ''}`,
      filter: { kind: 'text', getValue: (c) => `${c.display_name ?? ''} ${c.email} ${c.username ?? ''}`, placeholder: 'Name or email…' },
      render: (c) => (
        <Link to={`/clients/${c.id}`} className="cl-cell">
          <span className="app-avatar cl-avatar" aria-hidden="true">
            {c.avatar_url ? (
              <img src={c.avatar_url} alt="" className="cl-avatar__img" />
            ) : (
              (c.display_name || c.email).slice(0, 1).toUpperCase()
            )}
          </span>
          <span className="cl-cell__text">
            <span className="ui-strong">{c.display_name || c.email}</span>
            <span className="ui-muted cl-cell__sub">
              {c.username ? '@' + c.username : c.email}
              {c.google_linked && <span className="cl-tag">Google</span>}
              {!c.email_verified && <span className="cl-tag cl-tag--warn">unverified</span>}
            </span>
          </span>
        </Link>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      sortable: true,
      hideOnMobile: true,
      searchValue: (c) => String(c.status ?? ''),
      filter: {
        kind: 'select',
        options: [
          { value: 'ACTIVE', label: 'Active' },
          { value: 'SUSPENDED', label: 'Suspended' },
          { value: 'DISABLED', label: 'Disabled' },
        ],
      },
      render: (c) => <StatusBadge status={c.status} />,
    },
    {
      key: 'free_used',
      header: 'FREE',
      sortable: true,
      align: 'center',
      render: (c) => (
        <span className="cl-free" title={`${c.free_used} claimed in the last 30 days`}>
          {c.free_used}
          <span className="ui-muted"> / {c.free_vpn_limit > 0 ? c.free_vpn_limit : 'default'}</span>
        </span>
      ),
    },
    {
      key: 'premium_count',
      header: 'Premium',
      sortable: true,
      align: 'center',
      hideOnMobile: true,
      render: (c) => c.premium_count,
    },
    {
      key: 'vpn_count',
      header: 'VPNs',
      align: 'center',
      hideOnMobile: true,
      sortable: true,
      render: (c) => c.vpn_count,
    },
    {
      key: 'created_at',
      header: 'Joined',
      sortable: true,
      hideOnMobile: true,
      render: (c) => <span className="ui-muted">{relativeTime(c.created_at)}</span>,
    },
    {
      key: 'last_login_at',
      header: 'Last sign-in',
      hideOnMobile: true,
      sortable: true,
      // Never-signed-in sorts last: it is not "longest ago", it is "unknown".
      sortValue: (c) => c.last_login_at ?? 0,
      filter: {
        kind: 'select',
        options: [{ value: 'never', label: 'Never signed in' }],
        getValue: (c) => (c.last_login_at > 0 ? 'yes' : 'never'),
      },
      render: (c) =>
        c.last_login_at > 0 ? (
          <span className="ui-muted">{relativeTime(c.last_login_at)}</span>
        ) : (
          <span className="ui-muted">never</span>
        ),
    },
    {
      key: 'open',
      header: '',
      sticky: true,
      align: 'right',
      render: (c) => (
        <Link to={`/clients/${c.id}`} className="cl-open">Open →</Link>
      ),
    },
  ]

  /**
   * Rows per page. MUST match `useClientAdminStore`'s default `limit`, because the
   * server returns `total` (the whole filtered set) while only this many rows come
   * back per request.
   *
   * B-1: this used to read `Math.ceil(total / q.values.page)` — dividing by the
   * page NUMBER instead of the page SIZE. On an account with 6 clients that
   * rendered **6 pages** (verified live by `verify-clients.mjs`, which saw
   * pagination buttons `[1,2,3,4,5,6]`), and the error INVERTED as paging
   * advanced: page 50 of 1250 clients produced 25 entries for a 50-page result.
   */
  const PAGE_SIZE = 25
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))

  /**
   * Run the bulk status change one id at a time and count real outcomes.
   *
   * Sequential on purpose: the API takes one client per call, and firing fifty
   * parallel writes at a 1 vCPU host is how a bulk action becomes an outage. Partial
   * failure is reported honestly ("12 of 15") rather than as a success.
   */
  const runBulkStatus = async (status: 'ACTIVE' | 'SUSPENDED') => {
    const ids = [...selected]
    setBulkConfirm(null)
    setBulkBusy(true)
    let ok = 0
    let failed = 0
    for (const id of ids) {
      const err = await setStatus(Number(id), status)
      if (err) failed++
      else ok++
    }
    setBulkBusy(false)
    setSelected(new Set())
    await load()
    const verb = status === 'SUSPENDED' ? 'Blocked' : 'Unblocked'
    setBulkResult(
      failed === 0
        ? `${verb} ${ok} client${ok === 1 ? '' : 's'}.`
        : `${verb} ${ok} of ${ids.length}; ${failed} failed.`,
    )
    if (failed === 0) toast('success', verb, `${ok} client${ok === 1 ? '' : 's'} updated`)
    else toast('error', 'Some updates failed', `${failed} of ${ids.length} were not applied`)
  }

  return (
    <div className="dash" data-testid="clients-page">
      <PageHeader
        title="Client management"
        subtitle="Every customer, their limits, and what they own"
        actions={
          <div className="ui-row cl-head-actions">
            {/* Only offered when something is actually filtered, so the control
                never sits there doing nothing. */}
            {q.isFiltered && (
              <Button variant="ghost" size="sm" onClick={q.reset}>
                Clear filters
              </Button>
            )}
            <Button variant="outline" size="sm" leading={<Check size={14} />} onClick={() => void load()}>
              Refresh
            </Button>
          </div>
        }
      />

      {error && <p className="cl-alert" role="alert">{error}</p>}

      <Card>
        <form className="cl-filters" onSubmit={(e) => e.preventDefault()}>
          <div className="cl-filters__search">
            <Input
              label="Search"
              placeholder="name, e-mail, username or phone"
              value={q.draft.q}
              onChange={(e) => q.set({ q: e.target.value })}
              onBlur={() => q.setNow({ q: q.draft.q })}
            />
          </div>
          <Select label="Status" value={q.draft.status} onChange={(e) => q.set({ status: e.target.value })}>
            <option value="">Any status</option>
            <option value="ACTIVE">Active</option>
            <option value="SUSPENDED">Blocked</option>
            <option value="PENDING_VERIFICATION">Pending verification</option>
          </Select>
          <Select label="FREE VPNs" value={q.draft.has_free} onChange={(e) => q.set({ has_free: e.target.value })}>
            <option value="">Anyone</option>
            <option value="yes">Has claimed</option>
            <option value="no">Never claimed</option>
          </Select>
          <Select label="Premium" value={q.draft.has_premium} onChange={(e) => q.set({ has_premium: e.target.value })}>
            <option value="">Anyone</option>
            <option value="yes">Has premium</option>
            <option value="no">No premium</option>
          </Select>
          <Select label="Google" value={q.draft.google} onChange={(e) => q.set({ google: e.target.value })}>
            <option value="">Anyone</option>
            <option value="yes">Linked</option>
            <option value="no">Not linked</option>
          </Select>
          <Input label="Joined from" type="date" value={q.draft.from} onChange={(e) => q.set({ from: e.target.value })} />
          <Input label="Joined to" type="date" value={q.draft.to} onChange={(e) => q.set({ to: e.target.value })} />
          <Input
            label="FREE claims ≥"
            type="number"
            min={0}
            value={q.draft.free_min}
            onChange={(e) => q.set({ free_min: e.target.value })}
          />
          <Select label="Sort" value={q.draft.sort} onChange={(e) => q.set({ sort: e.target.value })}>
            <option value="">Newest first</option>
            <option value="oldest">Oldest first</option>
            <option value="name">Name</option>
            <option value="email">E-mail</option>
            <option value="free">Most FREE claims</option>
            <option value="premium">Most premium</option>
          </Select>
        </form>
      </Card>

      <Card title={total > 0 ? `Clients · ${total}` : 'Clients'}>
        <AsyncState
          status={
            // An error wins over the row count: showing "no clients match" when the
            // request failed would blame the operator's filters for a server fault.
            error ? 'error' : loading && rows.length === 0 ? 'loading' : rows.length === 0 ? 'empty' : 'ready'
          }
          error={error}
          onRetry={() => void load()}
          skeleton={<Skeleton height={220} />}
          empty={
            q.isFiltered ? (
              <EmptyState
                icon={<Users size={22} />}
                title="No clients match these filters"
                description="Widen the filters, or clear them to see every customer."
                action={
                  <Button variant="outline" size="sm" onClick={q.reset}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={<Users size={22} />}
                title="No customers yet"
                description="A client account appears here after the first sign-up or when you create one."
              />
            )
          }
        >
          <SelectionBar count={selected.size} onClear={() => setSelected(new Set())}>
            <Button
              size="sm"
              variant="outline"
              disabled={bulkBusy}
              onClick={() => setBulkConfirm('ACTIVE')}
            >
              Unblock
            </Button>
            <Button
              size="sm"
              variant="danger"
              disabled={bulkBusy}
              onClick={() => setBulkConfirm('SUSPENDED')}
            >
              Block
            </Button>
          </SelectionBar>
          <Table
            columns={columns}
            data={rows}
            rowKey={(r) => String(r.id)}
            rowLabel={(r) => r.display_name || r.email}
            searchable
            searchPlaceholder="Search name, email, username or status…"
            caption="clients"
            selection={{
              selected,
              onSelectedChange: setSelected,
            }}
          />
          <Pagination
            page={q.values.page}
            pageCount={pageCount}
            onChange={(p) => q.setNow({ page: p })}
          />
          {bulkResult && (
            <p className="ui-muted t-sm2 flow-top" role="status">
              {bulkResult}
            </p>
          )}
        </AsyncState>
      </Card>

      {/* Bulk actions always confirm. The dialog names the count and the effect,
          because "Block" on twelve accounts ends twelve people's sessions. */}
      <Dialog
        open={bulkConfirm !== null}
        onClose={() => setBulkConfirm(null)}
        title={bulkConfirm === 'SUSPENDED' ? 'Block these clients?' : 'Unblock these clients?'}
      >
        <p className="flow-top-0">
          {bulkConfirm === 'SUSPENDED' ? (
            <>
              <strong>{selected.size}</strong> client{selected.size === 1 ? '' : 's'} will be
              blocked. Every one of their sessions ends immediately and they cannot sign in
              until unblocked.
            </>
          ) : (
            <>
              <strong>{selected.size}</strong> client{selected.size === 1 ? '' : 's'} will be
              able to sign in again.
            </>
          )}
        </p>
        <div className="ui-row cl-dialog-actions">
          <Button variant="ghost" size="sm" onClick={() => setBulkConfirm(null)}>
            Cancel
          </Button>
          <Button
            variant={bulkConfirm === 'SUSPENDED' ? 'danger' : 'primary'}
            size="sm"
            onClick={() => bulkConfirm && void runBulkStatus(bulkConfirm)}
          >
            {bulkConfirm === 'SUSPENDED' ? 'Block them' : 'Unblock them'}
          </Button>
        </div>
      </Dialog>

      <p className="ui-muted cl-note">
        <UserX size={13} aria-hidden="true" /> Blocking a client ends every one of their
        sessions immediately. FREE limits are per client; 0 means the platform default.
      </p>
    </div>
  )
}


