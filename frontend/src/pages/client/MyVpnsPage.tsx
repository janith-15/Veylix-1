import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Crown,
  ExternalLink,
  Filter,
  LayoutGrid,
  List as ListIcon,
  MapPin,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  TriangleAlert,
  X,
} from 'lucide-react'
import {
  AsyncState,
  Button,
  EmptyState,
  Pagination,
  Skeleton,
  StatusBadge,
} from '../../components/ui'
import { ClientPage, ClientSection } from '../../components/client/ClientPage'
import { useListQuery } from '../../hooks/useListQuery'
import { useVpnStore, vpnDisplayName, vpnServerLabel, type ClientVpn } from '../../stores/vpnStore'
import { VpnKindBadge } from '../../components/vpn/VpnKindBadge'
import './vpns.css'

/*
 * ===========================================================================
 * MY VPNS - SECTION 02 (D-088)
 *
 * One inventory of the VPNs this client owns, in three categories and two
 * display modes.
 *
 * D-088 governs this file:
 *   - Categories are All / Free / Premium. **Three, not four.** A Custom
 *     Premium VPN is a Premium VPN; `is_custom` is a pricing detail about the
 *     package, not a fourth kind of VPN, and the wire values stay
 *     FREE / PREMIUM / empty.
 *   - Overview counts cover the WHOLE owned inventory, never the visible page
 *     and never the filtered subset.
 *   - Page size stays nine.
 *   - Cards and Compact List render the SAME rows; switching views issues no
 *     request and duplicates no dataset.
 *
 * The one thing this page does NOT do that S3 did: it fires **no per-card usage
 * request**. `GET /api/vpns` already carries `used_bytes`, `limit_bytes` and
 * `usage_pct` on every row (`api.ts:213-215`), and `vpnStore` now maps them, so
 * the N+1 pattern is removed rather than optimised.
 *
 * SCOPE INVARIANTS
 *   - Lists ONLY owned VPNs. `GET /api/vpns` is scoped to the caller by the
 *     server; no second, weaker ownership filter is applied here, because that
 *     is the copy that would eventually be wrong.
 *   - Unclaimed Free offers are never mixed in - they are not in this response.
 *   - The only navigation is the existing `/vpn/:id` route. Nothing new is
 *     invented and no connection is described as working unless its status says
 *     so.
 * ===========================================================================
 */

/** Page size. D-088 keeps the S3 value of nine. */
const PAGE_SIZE = 9

type SortKey = 'newest' | 'name' | 'expiry'

const SORTS: { value: SortKey; label: string }[] = [
  { value: 'newest', label: 'Newest first' },
  { value: 'name', label: 'Name A-Z' },
  { value: 'expiry', label: 'Expiry date' },
]

/** Days until expiry, or null when the row has no expiry at all. */
function daysLeft(expiresAt: number | null, nowSec: number): number | null {
  return expiresAt === null ? null : Math.ceil((expiresAt - nowSec) / 86400)
}

/**
 * The expiry label a card shows.
 *
 * Three distinct facts, never collapsed: no expiry at all, expired N days ago,
 * and N days left. "No expiry" must not read as day zero.
 */
function expiryLabel(expiresAt: number | null, nowSec: number): string {
  const d = daysLeft(expiresAt, nowSec)
  if (d === null) return 'No expiry'
  if (d < 0) return `Expired ${Math.abs(d)}d ago`
  if (d === 0) return 'Expires today'
  return `${d}d left`
}

/** True when the VPN needs the client's attention (same rule as the dashboard). */
function needsAttention(v: ClientVpn, nowSec: number): boolean {
  if (v.status !== 'ACTIVE') return true
  const d = daysLeft(v.expiresAt, nowSec)
  return d !== null && d >= 0 && d <= 7
}

/**
/**
 * Usage on this screen: a LINK, not a number (D-088.1).
 *
 * `GET /api/vpns` - the list endpoint a client reads - carries NO usage fields.
 * `used_bytes` / `limit_bytes` / `usage_pct` belong to `ApiAdminVPNRow`
 * (`GET /api/super/vpns`), a different role endpoint. There is therefore no
 * figure here to render, and inventing one is the fabrication this programme
 * exists to remove.
 *
 * Two worse options were rejected:
 *   - firing `GET /api/vpns/{id}/usage` per card - the S3 N+1 pattern, which the
 *     brief forbids;
 *   - rendering "0 B" - a measurement nobody took.
 *
 * So the card states where the real number lives; the detail screen asks the
 * server for that one VPN.
 */
function UsageCell() {
  return (
    <span className="mv-card__row">
      <span>Usage</span>
      <span className="ui-muted">on the detail screen</span>
    </span>
  )
}


export function MyVpnsPage() {
  const navigate = useNavigate()
  const vpns = useVpnStore((s) => s.vpns)
  const loading = useVpnStore((s) => s.loading)
  const error = useVpnStore((s) => s.error)
  const load = useVpnStore((s) => s.load)

  useEffect(() => {
    void load()
  }, [load])

  /** One clock read per mount, so "days left" cannot differ between renders. */
  const [nowSec] = useState(() => Date.now() / 1000)

  /*
   * All list state lives in the URL (`useListQuery`), so a filtered view is
   * shareable, survives a refresh, and Back undoes the last filter instead of
   * leaving the screen. `q` is debounced because it is free text; every other
   * control commits immediately and resets to page 1.
   *
   * `view` is part of the URL too: a client who prefers the compact list keeps
   * it across a reload, and a shared link reproduces what the sender saw.
   */
  const query = useListQuery({
    defaults: {
      q: '',
      cat: 'all',
      status: 'all',
      server: 'all',
      expiry: 'all',
      sort: 'newest',
      page: 1,
      view: 'cards',
    },
    debounced: ['q'],
  })

  const { q, cat, status: statusFilter, server, expiry, sort, view } = query.values
  const [advOpen, setAdvOpen] = useState(false)

  /* ---- overview: the WHOLE inventory, never the visible page (D-088) ---- */
  const total = vpns.length
  const activeCount = useMemo(() => vpns.filter((v) => v.status === 'ACTIVE').length, [vpns])
  const attentionCount = useMemo(
    () => vpns.filter((v) => needsAttention(v, nowSec)).length,
    [vpns, nowSec],
  )

  /* Category counts for the three chips. Three, not four (D-088). */
  const catCounts = useMemo(
    () => ({
      all: vpns.length,
      FREE: vpns.filter((v) => v.kind === 'FREE').length,
      PREMIUM: vpns.filter((v) => v.kind === 'PREMIUM').length,
    }),
    [vpns],
  )

  /* ---- server options, derived only from rows the server sent ---------- */
  const serverOptions = useMemo(() => {
    const seen = new Map<number, string>()
    for (const v of vpns) if (!seen.has(v.serverId)) seen.set(v.serverId, vpnServerLabel(v))
    return [...seen.entries()]
      .map(([id, label]) => ({ value: String(id), label }))
      .sort((a, b) => a.label.localeCompare(b.label))
  }, [vpns])

  /*
   * Status options are the statuses ACTUALLY present in this client's own rows,
   * with the counts the server would return. Offering a status nobody has would
   * be a filter that can only produce an empty screen.
   */
  const statusOptions = useMemo(() => {
    const counts = new Map<string, number>()
    for (const v of vpns) counts.set(v.status, (counts.get(v.status) ?? 0) + 1)
    return [...counts.entries()]
      .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
      .map(([value, n]) => ({ value, label: `${value.replace(/_/g, ' ')} (${n})` }))
  }, [vpns])

  /* ---- filtering ------------------------------------------------------- */
  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase()
    return vpns.filter((v) => {
      /* Category: three values only (D-088). */
      if (cat !== 'all' && v.kind !== cat) return false
      if (statusFilter !== 'all' && v.status !== statusFilter) return false
      if (server !== 'all' && String(v.serverId) !== server) return false
      if (expiry !== 'all') {
        const d = daysLeft(v.expiresAt, nowSec)
        if (expiry === 'none' && d !== null) return false
        if (expiry === 'soon' && (d === null || d < 0 || d > 7)) return false
        if (expiry === 'expired' && (d === null || d >= 0)) return false
        if (expiry === 'ok' && d !== null && d <= 7) return false
      }
      if (needle) {
        /* Exactly the labels the card displays (D-088), so a person never has
           to guess which field a match came from. */
        const hay = `${vpnDisplayName(v)} ${vpnServerLabel(v)} ${v.offerName ?? ''}`.toLowerCase()
        if (!hay.includes(needle)) return false
      }
      return true
    })
    /* `daysLeft` is pure given `nowSec`, which is fixed for the mount. */
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [vpns, q, cat, statusFilter, server, expiry, nowSec])

  /* ---- sorting --------------------------------------------------------- */
  const sorted = useMemo(() => {
    const rows = [...filtered]
    switch (sort) {
      case 'name':
        return rows.sort((a, b) => vpnDisplayName(a).localeCompare(vpnDisplayName(b)))
      case 'expiry':
        /* "No expiry" sorts LAST, not first: a reader asking "what runs out
           next" does not want a never-expiring VPN at the top. */
        return rows.sort(
          (a, b) => (a.expiresAt ?? Number.MAX_SAFE_INTEGER) - (b.expiresAt ?? Number.MAX_SAFE_INTEGER),
        )
      default:
        return rows.sort((a, b) => b.apiId - a.apiId)
    }
  }, [filtered, sort])

  /* ---- paging ---------------------------------------------------------- */
  const pageCount = Math.max(1, Math.ceil(sorted.length / PAGE_SIZE))
  const page = Math.min(Math.max(1, Number(query.values.page) || 1), pageCount)
  const visible = sorted.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)

  const asyncStatus =
    error ? 'error' : loading && vpns.length === 0 ? 'loading' : vpns.length === 0 ? 'empty' : 'ready'

  const sectionTitle =
    cat === 'FREE' ? 'My Free VPNs' : cat === 'PREMIUM' ? 'My Premium VPNs' : 'My VPNs'

  /* Human labels for the active-filter tags. */
  const activeTags: { key: string; label: string }[] = []
  if (cat !== 'all') activeTags.push({ key: 'cat', label: cat === 'FREE' ? 'Free' : 'Premium' })
  if (statusFilter !== 'all')
    activeTags.push({ key: 'status', label: statusFilter.replace(/_/g, ' ') })
  if (server !== 'all')
    activeTags.push({
      key: 'server',
      label: serverOptions.find((o) => o.value === server)?.label ?? `server ${server}`,
    })
  if (expiry !== 'all') {
    const map: Record<string, string> = {
      soon: 'expiring within 7 days',
      expired: 'expired',
      ok: 'more than 7 days left',
      none: 'no expiry',
    }
    activeTags.push({ key: 'expiry', label: map[expiry] ?? expiry })
  }
  if (q.trim()) activeTags.push({ key: 'q', label: `"${q.trim()}"` })

  /* ---- one card -------------------------------------------------------- */
  const renderCard = (v: ClientVpn) => (
    <article
      key={v.id}
      className={`mv-card${v.kind === 'PREMIUM' ? ' mv-card--premium' : ''}`}
      data-testid={`vpn-card-${v.apiId}`}
    >
      <div className="mv-card__head">
        <div className="mv-card__title">
          <span className="mv-card__name" title={vpnDisplayName(v)}>{vpnDisplayName(v)}</span>
          <span className="mv-card__sub">
            <MapPin size={11} aria-hidden="true" style={{ marginRight: 3, verticalAlign: '-1px' }} />
            {vpnServerLabel(v)}
          </span>
        </div>
        <span className="mv-card__badges">
          <VpnKindBadge kind={v.kind} />
        </span>
      </div>

      <div className="mv-card__meta">
        <div className="mv-card__row">
          <span>Status</span>
          <span><StatusBadge status={v.status} /></span>
        </div>
        <UsageCell />
        <div className="mv-card__row">
          <span>Expiry</span>
          <span>{expiryLabel(v.expiresAt, nowSec)}</span>
        </div>
      </div>

      {v.status !== 'ACTIVE' && (
        <span className="vpn-warn t-sm2" style={{ display: 'flex', gap: 8, marginBottom: 0 }}>
          <TriangleAlert size={14} aria-hidden="true" />
          Not connectable right now - details inside.
        </span>
      )}

      <div className="mv-card__actions">
        <Button
          size="sm"
          variant="outline"
          leading={<ExternalLink size={13} />}
          onClick={() => navigate(`/vpn/${v.id}`)}
        >
          View Details
        </Button>
        {v.kind === 'FREE' && (
          <Button
            size="sm"
            variant="ghost"
            leading={<Crown size={13} />}
            onClick={() => navigate('/buy')}
          >
            Upgrade
          </Button>
        )}
      </div>
    </article>
  )

  /* ---- one list row ---------------------------------------------------- */
  /* ---- one list row ---------------------------------------------------- */
  const renderRow = (v: ClientVpn) => (
    <tr key={v.id} data-testid={`vpn-row-${v.apiId}`}>
      <td>
        <div className="mv-list__name">
          <strong>{vpnDisplayName(v)}</strong>
          <span>{v.offerName ?? vpnServerLabel(v)}</span>
        </div>
      </td>
      <td><VpnKindBadge kind={v.kind} /></td>
      <td><StatusBadge status={v.status} /></td>
      <td>{vpnServerLabel(v)}</td>
      <td className="mv-list__num">
        <span className="ui-muted">detail screen</span>
      </td>
      <td className="mv-list__num">{expiryLabel(v.expiresAt, nowSec)}</td>
      <td className="mv-list__act">
        <Button size="sm" variant="ghost" onClick={() => navigate(`/vpn/${v.id}`)}>
          View Details
        </Button>
      </td>
    </tr>
  )

  return (
    <ClientPage
      testId="client-vpns"
      title="My VPNs"
      description="The VPNs on your account, from the live panel"
      actions={
        <Button size="sm" onClick={() => navigate('/buy')}>
          Buy premium
        </Button>
      }
    >
      <div className="mv">
        {/* ---- overview: whole inventory, never the visible page ------- */}
        {total > 0 && (
          <section className="mv-overview" aria-label="VPN overview">
            <div className="mv-ov">
              <span className="mv-ov__label">Total VPNs</span>
              <span className="mv-ov__value">{total}</span>
              <p className="mv-ov__delta">
                {catCounts.PREMIUM} premium - {catCounts.FREE} free
              </p>
            </div>
            <div className="mv-ov mv-ov--ok">
              <span className="mv-ov__label">Active</span>
              <span className="mv-ov__value">{activeCount}</span>
              <p className="mv-ov__delta">connectable right now</p>
            </div>
            <div className={`mv-ov ${attentionCount > 0 ? 'mv-ov--warn' : 'mv-ov--ok'}`}>
              <span className="mv-ov__label">Needs attention</span>
              <span className="mv-ov__value">{attentionCount}</span>
              <p className="mv-ov__delta">
                {attentionCount > 0 ? 'expiring or not connectable' : 'all settled'}
              </p>
            </div>
          </section>
        )}

        {/* ---- toolbar ------------------------------------------------ */}
        {vpns.length > 0 && (
          <div className="mv-toolbar">
            <div className="mv-toolbar__row">
              {/* Search */}
              <div className="client-filters__search" style={{ flex: '1 1 240px', maxWidth: 420 }}>
                <label className="ui-sr-only" htmlFor="mv-search">
                  Search your VPNs
                </label>
                <Search size={15} aria-hidden="true" className="client-filters__icon" />
                <input
                  id="mv-search"
                  className="client-filters__input"
                  type="search"
                  value={query.draft.q}
                  placeholder="Search by name, server or package..."
                  autoComplete="off"
                  onChange={(e) => query.set({ q: e.target.value })}
                />
                {query.draft.q ? (
                  <button
                    type="button"
                    className="client-filters__clear"
                    aria-label="Clear search"
                    onClick={() => query.setNow({ q: '' })}
                  >
                    <X size={14} aria-hidden="true" />
                  </button>
                ) : null}
              </div>

              {/* View toggle: same rows, no request */}
              <div className="mv-views" role="group" aria-label="Display mode">
                <button
                  type="button"
                  className="mv-view"
                  aria-pressed={view !== 'list'}
                  onClick={() => query.setNow({ view: 'cards' })}
                >
                  <LayoutGrid size={13} aria-hidden="true" /> Cards
                </button>
                <button
                  type="button"
                  className="mv-view"
                  aria-pressed={view === 'list'}
                  onClick={() => query.setNow({ view: 'list' })}
                >
                  <ListIcon size={13} aria-hidden="true" /> List
                </button>
              </div>
            </div>

            <div className="mv-toolbar__row">
              {/* Category chips: three, never four (D-088) */}
              <div className="mv-chips" role="group" aria-label="Category">
                {([
                  { key: 'all', label: 'All VPNs', n: catCounts.all },
                  { key: 'FREE', label: 'Free', n: catCounts.FREE },
                  { key: 'PREMIUM', label: 'Premium', n: catCounts.PREMIUM },
                ] as const).map((c) => (
                  <button
                    key={c.key}
                    type="button"
                    className="mv-chip"
                    aria-pressed={cat === c.key}
                    onClick={() => query.setNow({ cat: c.key, page: 1 })}
                  >
                    {c.label}
                    <span className="mv-chip__count">{c.n}</span>
                  </button>
                ))}
              </div>

              <Button
                size="sm"
                variant="ghost"
                leading={<Filter size={13} />}
                onClick={() => setAdvOpen((o) => !o)}
                aria-expanded={advOpen}
              >
                {advOpen ? 'Hide filters' : 'Filters'}
              </Button>
            </div>

            {/* Active filters are always visible, collapsed or not. */}
            {activeTags.length > 0 && (
              <div className="mv-selected" aria-label="Active filters">
                {activeTags.map((t) => (
                  <span key={t.key} className="mv-selected__tag">
                    {t.label}
                    <button
                      type="button"
                      aria-label={`Remove filter ${t.label}`}
                      style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'inherit', padding: 0, display: 'inline-flex' }}
                      onClick={() => query.setNow({ [t.key]: t.key === 'q' ? '' : 'all', page: 1 })}
                    >
                      <X size={11} aria-hidden="true" />
                    </button>
                  </span>
                ))}
                <Button size="sm" variant="ghost" onClick={query.reset}>
                  Clear filters
                </Button>
              </div>
            )}

            {/* Advanced filters, collapsed by default */}
            {advOpen && (
              <div className="mv-adv">
                <label className="mv-adv__group">
                  <span className="mv-adv__label">Status</span>
                  <select
                    className="client-filters__select"
                    value={statusFilter}
                    onChange={(e) => query.setNow({ status: e.target.value, page: 1 })}
                  >
                    <option value="all">Any status ({vpns.length})</option>
                    {statusOptions.map((o) => (
                      <option key={o.value} value={o.value}>{o.label}</option>
                    ))}
                  </select>
                </label>

                <label className="mv-adv__group">
                  <span className="mv-adv__label">Server</span>
                  <select
                    className="client-filters__select"
                    value={server}
                    onChange={(e) => query.setNow({ server: e.target.value, page: 1 })}
                  >
                    <option value="all">Any server ({serverOptions.length})</option>
                    {serverOptions.map((o) => (
                      <option key={o.value} value={o.value}>{o.label}</option>
                    ))}
                  </select>
                </label>

                <label className="mv-adv__group">
                  <span className="mv-adv__label">Expiry</span>
                  <select
                    className="client-filters__select"
                    value={expiry}
                    onChange={(e) => query.setNow({ expiry: e.target.value, page: 1 })}
                  >
                    <option value="all">Any expiry</option>
                    <option value="soon">Expiring within 7 days</option>
                    <option value="expired">Expired</option>
                    <option value="ok">More than 7 days left</option>
                    <option value="none">No expiry</option>
                  </select>
                </label>

                <label className="mv-adv__group">
                  <span className="mv-adv__label">Sort</span>
                  <select
                    className="client-filters__select"
                    value={sort}
                    onChange={(e) => query.setNow({ sort: e.target.value as SortKey, page: 1 })}
                  >
                    {SORTS.map((s) => (
                      <option key={s.value} value={s.value}>{s.label}</option>
                    ))}
                  </select>
                </label>
              </div>
            )}
          </div>
        )}

        {/* ---- results ------------------------------------------------- */}
        <AsyncState
          status={asyncStatus}
          error={error}
          onRetry={() => void load()}
          skeleton={<Skeleton height={200} />}
          empty={
            <EmptyState
              icon={<ShieldCheck size={22} />}
              title="You have no VPN yet"
              description="Claim a free VPN from the dashboard, or buy a premium package - it is provisioned on the panel immediately."
              action={
                <div className="ui-row" style={{ gap: 8, flexWrap: 'wrap' }}>
                  <Button size="sm" variant="primary" leading={<Plus size={14} />} onClick={() => navigate('/client')}>
                    Claim a free VPN
                  </Button>
                  <Button size="sm" variant="outline" onClick={() => navigate('/buy')}>
                    Buy premium
                  </Button>
                </div>
              }
            />
          }
        >
          {sorted.length === 0 ? (
            <EmptyState
              icon={<Search size={22} />}
              title="No VPNs match these filters"
              description="Your VPNs are still there - widen the filters or clear the search to see them."
              action={
                <Button size="sm" variant="outline" leading={<RefreshCw size={13} />} onClick={query.reset}>
                  Clear filters
                </Button>
              }
            />
          ) : (
            <>
              <ClientSection
                title={sectionTitle}
                actions={
                  <span className="ui-muted t-sm">
                    {sorted.length} of {vpns.length}
                  </span>
                }
              >
                {view === 'list' ? (
                  <div className="mv-listwrap">
                    <table className="mv-list">
                      <thead>
                        <tr>
                          <th scope="col">VPN</th>
                          <th scope="col">Category</th>
                          <th scope="col">Status</th>
                          <th scope="col">Server</th>
                          <th scope="col">Usage</th>
                          <th scope="col">Expiry</th>
                          <th scope="col" style={{ textAlign: 'right' }}>Actions</th>
                        </tr>
                      </thead>
                      <tbody>{visible.map(renderRow)}</tbody>
                    </table>
                  </div>
                ) : (
                  <div className="mv-grid">{visible.map(renderCard)}</div>
                )}
              </ClientSection>

              {pageCount > 1 && (
                <div className="mv-pager">
                  <span className="mv-pager__count">
                    Showing {(page - 1) * PAGE_SIZE + 1}-{Math.min(page * PAGE_SIZE, sorted.length)} of{' '}
                    {sorted.length}
                  </span>
                  <Pagination
                    page={page}
                    pageCount={pageCount}
                    onChange={(p) => query.setNow({ page: p })}
                  />
                </div>
              )}
            </>
          )}
        </AsyncState>
      </div>
    </ClientPage>
  )
}
