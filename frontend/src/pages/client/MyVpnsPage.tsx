import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Crown,
  ExternalLink,
  Eye,
  Filter,
  LayoutGrid,
  List as ListIcon,
  MapPin,
  Moon,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  Sun,
  TriangleAlert,
  X,
} from 'lucide-react'
import {
  AsyncState,
  Button,
  Dialog,
  EmptyState,
  Pagination,
  Skeleton,
  StatusBadge,
} from '../../components/ui'
import { ClientPage, ClientSection } from '../../components/client/ClientPage'
import { api } from '../../lib/api'
import { useListQuery } from '../../hooks/useListQuery'
import { useVpnStore, vpnDisplayName, vpnServerLabel, statusNote, type ClientVpn } from '../../stores/vpnStore'
import { VpnKindBadge } from '../../components/vpn/VpnKindBadge'
import { PanelIdentityRow } from '../../components/vpn/PanelIdentityRow'
import { VpnQuickView } from '../../components/vpn/VpnQuickView'
import './vpns.css'
import { activeTimeZone, formatDateTime, zoneLabel } from '../../lib/time'

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

/**
 * How far down the list a status belongs, lower reading higher.
 *
 * This exists because the list is sorted by NAME or DATE, not by "can I use
 * this?". A client sorting by NEWEST therefore got a suspended or revoked VPN
 * sitting at the very top of their inventory — the loudest possible position for
 * the row they can do least with — purely because it was created recently.
 *
 * The order is not a preference, it is the answer to "what should I look at
 * first?":
 *
 *   0  ACTIVE              works now; this is the inventory
 *   1  needs attention     Veylix or the seller must act, the customer cannot
 *   2  recoverable         a problem the customer can resolve (top-up, re-buy)
 *   3  not in service      suspended / revoked / released — nothing to do
 *   4  dormant             already filed away by the customer (D-094)
 *
 * Anything unrecognised lands at 2: an unknown status is more likely to need a
 * look than to be finished with.
 */
const STATUS_RANK: Record<string, number> = {
  ACTIVE: 0,
  SERVER_UNAVAILABLE: 1,
  REVOCATION_PENDING: 1,
  PENDING: 1,
  TRAFFIC_DEPLETED: 2,
  EXPIRED: 2,
  SUSPENDED: 3,
  REVOKED: 3,
  RELEASED: 3,
  REMOVED_FROM_CATALOG: 3,
  DORMANT: 4,
}

/**
 * The statuses that belong in the "Not in service" section.
 *
 * A client cannot use any of these, so listing them beside working VPNs made the
 * working ones harder to find. They are NOT hidden: each still renders with its
 * sentence and its panel identity, one section further down, because a customer
 * who released or lost a VPN must still be able to see what happened to it
 * (D-091.5 / INV-1).
 */
const NOT_IN_SERVICE = new Set(['SUSPENDED', 'REVOKED', 'RELEASED', 'REMOVED_FROM_CATALOG'])

/** True when this VPN is a record rather than something the client can use. */
function isNotInService(v: ClientVpn): boolean {
  return NOT_IN_SERVICE.has(v.status)
}

/** The sort rank for a VPN: status first, then dormant last of all. */
function rankOf(v: ClientVpn): number {
  // Dormant always sorts LAST whatever its status (D-094) — the customer has
  // already said "not now", so re-floating it to the top would undo the switch.
  if (v.dormantAt !== null) return 4
  return STATUS_RANK[v.status] ?? 2
}

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

/**
 * "Since when" for a non-active VPN, or null when there is nothing honest to say.
 *
 * Two rules, both about not inventing a fact:
 *
 *   - ACTIVE needs no date. "Active since March" invites the reader to treat the
 *     date as meaningful, when it only records the last sync's decision.
 *   - A row with no clock (null, or a database that predates D-098's backfill)
 *     shows NOTHING. "1 Jan 1970" would be worse than silence, and a client
 *     must never be told a date the record does not hold.
 *
 * The wording is relative for anything inside a week because that is the range
 * where "3 days ago" is more useful than a date; beyond it, the date.
 */
function sinceLabel(v: ClientVpn, nowSec: number): string | null {
  if (v.status === 'ACTIVE') return null
  const at = v.statusChangedAt
  if (at === null || at <= 0) return null
  const days = Math.floor((nowSec - at) / 86400)
  if (days <= 0) return 'today'
  if (days === 1) return 'yesterday'
  if (days < 7) return `${days} days ago`
  return `${formatDateTime(at)} (${zoneLabel()})`
}

/**
 * True when the VPN needs the client's attention (same rule as the dashboard).
 *
 * A dormant VPN never does (D-094). That is the entire point of the switch: the
 * customer has already seen the problem and said "not now", so re-counting it
 * every visit would make the switch do nothing.
 */
function needsAttention(v: ClientVpn, nowSec: number): boolean {
  if (v.dormantAt !== null) return false
  if (v.status !== 'ACTIVE') return true
  const d = daysLeft(v.expiresAt, nowSec)
  return d !== null && d >= 0 && d <= 7
}

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


/**
 * "3 Nov" — the day a VPN was put to sleep (D-094).
 *
 * Deliberately short and locale-formatted: the exact clock time of a "not now"
 * decision is noise, but the date is what makes the sentence believable (it
 * proves the customer did this, and when). A missing stamp returns an empty
 * string rather than "Invalid Date" — a dormant row always has one, and if that
 * ever changed the card should read as a plain sentence, not as a crash.
 */
function asleepSinceLabel(dormantAt: number | null): string {
  if (dormantAt === null) return ''
  return new Date(dormantAt * 1000).toLocaleDateString(undefined, {
    timeZone: activeTimeZone(),
    day: 'numeric',
    month: 'short',
  })
}

export function MyVpnsPage() {
  const navigate = useNavigate()
  const vpns = useVpnStore((s) => s.vpns)
  const loading = useVpnStore((s) => s.loading)
  const error = useVpnStore((s) => s.error)
  const load = useVpnStore((s) => s.load)
  const busy = useVpnStore((s) => s.busy)
  const dormant = useVpnStore((s) => s.dormant)
  const wake = useVpnStore((s) => s.wake)

  /**
   * Puts a VPN to sleep or wakes it (D-094).
   *
   * One handler for both directions because the two differ only in which call
   * runs; keeping them separate invited the two paths to drift apart on the
   * error handling, which is the part that must behave identically.
   *
   * A failure is swallowed here on purpose: the store already recorded the
   * message and `error` is rendered by `<AsyncState>`. Re-throwing would produce
   * an unhandled rejection in the console for a condition the customer can
   * already read on screen.
   */
  const toggleSleep = async (v: ClientVpn) => {
    try {
      if (v.dormantAt !== null) await wake(v.apiId)
      else await dormant(v.apiId)
    } catch {
      /* reported by the store; see above */
    }
  }

  /*
   * Replace (D-112).
   *
   * A replace builds a NEW VPN and then releases the current one, so the
   * customer ends up with a genuinely different panel client rather than the
   * same one renamed. It has two side effects, so it is never a single click:
   * the confirmation names both halves and, for a FREE VPN, the monthly claim it
   * costs (Bible §17.18 — releasing does not refund a claim).
   */
  const [replaceTarget, setReplaceTarget] = useState<ClientVpn | null>(null)
  const [replacing, setReplacing] = useState(false)
  const [replaceError, setReplaceError] = useState('')
  const [replaceDone, setReplaceDone] = useState<{
    newEmail: string
    oldEmail: string
    oldVpnId: number
    claimConsumed: boolean
    claimsRemaining: number
  } | null>(null)

  const doReplace = async () => {
    if (!replaceTarget) return
    setReplacing(true)
    setReplaceError('')
    try {
      const res = await api.replaceVpn(replaceTarget.apiId)
      // Say exactly what happened, with both panel identities: the customer has
      // to be able to tell support which client they now hold and which one is
      // in its undo window.
      setReplaceDone({
        newEmail: res.new.panel_email,
        oldEmail: res.old_panel_email,
        oldVpnId: res.old_vpn_id,
        claimConsumed: res.claim_consumed,
        claimsRemaining: res.claims_remaining,
      })
      setReplaceTarget(null)
      await load()
    } catch (err) {
      // The route answers with a sentence the customer can act on — a spent
      // allowance names the window, a refused create says nothing changed.
      setReplaceError(
        err instanceof Error && err.message
          ? err.message
          : 'Could not replace this VPN. Nothing was changed.',
      )
    } finally {
      setReplacing(false)
    }
  }

  useEffect(() => {
    void load()
  }, [load])

  /** One clock read per mount, so "days left" cannot differ between renders. */
  const [nowSec] = useState(() => Date.now() / 1000)

  /*
   * The VPN whose quick view is open, or null.
   *
   * Holding the row itself (not an id) means the sheet can render its identity
   * and status immediately, with no second list lookup and no spinner on the
   * frame the customer cares about. Only the usage summary is fetched.
   */
  const [quick, setQuick] = useState<ClientVpn | null>(null)

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
  /*
   * Dormant VPNs always sort LAST, whatever the chosen key (D-094).
   *
   * This is applied as an OUTER comparison rather than inside each `case`, so
   * "sort by name" cannot float a sleeping VPN back to the top — the one thing
   * the customer asked for is that it stops being first. Within each group the
   * chosen sort still applies in full, so the order the customer picked is never
   * silently replaced.
   */
  const sorted = useMemo(() => {
    const rows = [...filtered]
    switch (sort) {
      case 'name':
        rows.sort((a, b) => vpnDisplayName(a).localeCompare(vpnDisplayName(b)))
        break
      case 'expiry':
        /* "No expiry" sorts LAST, not first: a reader asking "what runs out
           next" does not want a never-expiring VPN at the top. */
        rows.sort(
          (a, b) => (a.expiresAt ?? Number.MAX_SAFE_INTEGER) - (b.expiresAt ?? Number.MAX_SAFE_INTEGER),
        )
        break
      default:
        rows.sort((a, b) => b.apiId - a.apiId)
    }
    /* STATUS beats the chosen key (D-100).
     *
     * Before this, the only outer comparison was dormancy — so a SUSPENDED or
     * REVOKED VPN floated to the TOP under "Newest first" purely because its row
     * was created recently. The customer's most prominent slot was given to the
     * one VPN they could do nothing with.
     *
     * `rankOf` puts it below everything usable, and dormant keeps its place at
     * the very bottom. The chosen sort still applies IN FULL inside each rank,
     * so the order the customer picked is never silently replaced — only
     * grouped. Array.prototype.sort is stable, so equal ranks keep that order. */
    return rows.sort((a, b) => rankOf(a) - rankOf(b))
  }, [filtered, sort])

  /* ---- the two sections (D-100) ---------------------------------------- *
   *
   * `sorted` is split into what the client can USE and what is a RECORD of
   * something finished. They are not two datasets: one filter pass, one sort
   * pass, and the same rows — just presented in the order a person reads them.
   *
   * Nothing is dropped. Every matching row still renders, with its status
   * sentence and its panel identity; the second group simply stops competing
   * with the working inventory for the top of the page.
   *
   * The Overview counts below deliberately still count ALL owned VPNs (D-088),
   * not `usable`: "Total VPNs" is a property of the account, not of one
   * section, and a count that silently excluded suspended rows would be a
   * smaller number than the truth.
   */
  const usable = useMemo(() => sorted.filter((v) => !isNotInService(v)), [sorted])
  const resting = useMemo(() => sorted.filter(isNotInService), [sorted])

  /* ---- paging ---------------------------------------------------------- */
  const pageCount = Math.max(1, Math.ceil(usable.length / PAGE_SIZE))
  const page = Math.min(Math.max(1, Number(query.values.page) || 1), pageCount)
  const visible = usable.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)

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
      className={`mv-card${v.kind === 'PREMIUM' ? ' mv-card--premium' : ''}${
        v.dormantAt !== null ? ' mv-card--dormant' : ''
      }`}
      data-testid={`vpn-card-${v.apiId}`}
      data-dormant={v.dormantAt !== null ? 'true' : 'false'}
    >
      <div className="mv-card__head">
        <div className="mv-card__title">
          <span className="mv-card__name" title={vpnDisplayName(v)}>{vpnDisplayName(v)}</span>
          {/* D-091.4: the panel identity sits beside the package name on the
              very screen a customer opens most — never one without the other,
              and copyable because that is the string support will ask for. */}
          <PanelIdentityRow
            panelEmail={v.panelEmail}
            layout="inline"
            testId={`vpn-identity-${v.apiId}`}
          />

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
        {/* D-100: WHEN, beside WHAT. "Suspended" alone cannot tell a customer
            whether it happened this morning or last quarter, and that gap is
            exactly what a support call fills. Rendered only when the server
            actually sent a clock — a guessed date would be worse than none. */}
        {sinceLabel(v, nowSec) !== null && (
          <div className="mv-card__row" data-testid={`vpn-since-${v.apiId}`}>
            <span>Since</span>
            <span>{sinceLabel(v, nowSec)}</span>
          </div>
        )}
        <UsageCell />
        <div className="mv-card__row">
          <span>Expiry</span>
          <span>{expiryLabel(v.expiresAt, nowSec)}</span>
        </div>
      </div>

      {/* D-091.5: the row explains a non-active state instead of only colouring
          it. The sentence comes from `statusNote`, the same function the quick
          view and the detail page use, so the three cannot drift apart. */}
      {statusNote(v.status) !== null && (
        <span className="vpn-warn t-sm2 mv-card__note" data-testid={`vpn-note-${v.apiId}`}>
          <TriangleAlert size={14} aria-hidden="true" />
          {statusNote(v.status)}
        </span>
      )}

      {/* D-094: a dormant VPN says what was done and what it means, in the
          customer's own terms. It is deliberately NOT phrased as an error —
          nothing is broken and nothing was lost; the customer chose this, and
          the sentence has to make that obvious or they will think they deleted
          something. The real status above stays visible, so sleeping never
          disguises a dead VPN as a healthy one. */}
      {v.dormantAt !== null && (
        <span className="mv-card__asleep t-sm2" data-testid={`vpn-dormant-note-${v.apiId}`}>
          <Moon size={14} aria-hidden="true" />
          <span>
            You put this VPN to sleep on {asleepSinceLabel(v.dormantAt)}. It is safe — nothing was
            deleted and it still works exactly as before. You can bring it back any time.
          </span>
        </span>
      )}

      <div className="mv-card__actions">
        <Button
          size="sm"
          variant="outline"
          leading={<Eye size={13} />}
          onClick={() => setQuick(v)}
          data-testid={`vpn-quick-${v.apiId}`}
        >
          Quick view
        </Button>

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
        {/* The "not now" switch. It sits last because it is the least common
            action, and its label states the effect rather than the mechanism:
            "Sleep" / "Wake", never "dormant" or "deactivate". */}
        <Button
          size="sm"
          variant="ghost"
          leading={v.dormantAt !== null ? <Sun size={13} /> : <Moon size={13} />}
          onClick={() => void toggleSleep(v)}
          disabled={busy === v.apiId}
          data-testid={`vpn-sleep-${v.apiId}`}
        >
          {v.dormantAt !== null ? 'Wake' : 'Sleep'}
        </Button>
        {/*
          Replace (D-112). Offered only for a VPN that is actually in service:
          replacing a released or revoked one would be a second VPN on top of a
          dead one, and the customer wants `restore` in that case. The label
          states the outcome ("Replace"), never the mechanism.
        */}
        {!NOT_IN_SERVICE.has(v.status) && (
          <Button
            size="sm"
            variant="ghost"
            leading={<RefreshCw size={13} />}
            onClick={() => {
              setReplaceError('')
              setReplaceDone(null)
              setReplaceTarget(v)
            }}
            disabled={busy === v.apiId}
            data-testid={`vpn-replace-${v.apiId}`}
          >
            Replace
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
          <PanelIdentityRow
            panelEmail={v.panelEmail}
            layout="inline"
            testId={`vpn-row-identity-${v.apiId}`}
          />
          <span className="ui-muted t-xs2">{vpnServerLabel(v)}</span>
        </div>
      </td>
      <td><VpnKindBadge kind={v.kind} /></td>
      <td>
        <div className="mv-list__status">
          <StatusBadge status={v.status} />
          {statusNote(v.status) !== null && (
            <span className="t-xs2 ui-muted">{statusNote(v.status)}</span>
          )}
          {/* D-100: WHEN, inside the same cell as WHAT — a whole extra column
              for a date most rows do not have would be a column of dashes. */}
          {sinceLabel(v, nowSec) !== null && (
            <span className="t-xs2 ui-muted" data-testid={`vpn-row-since-${v.apiId}`}>
              since {sinceLabel(v, nowSec)}
            </span>
          )}
        </div>
      </td>
      <td>{vpnServerLabel(v)}</td>
      <td className="mv-list__num">
        <span className="ui-muted">detail screen</span>
      </td>
      <td className="mv-list__num">{expiryLabel(v.expiresAt, nowSec)}</td>
      <td className="mv-list__act">
        <Button size="sm" variant="ghost" onClick={() => setQuick(v)}>
          Quick view
        </Button>
        <Button size="sm" variant="ghost" onClick={() => navigate(`/vpn/${v.id}`)}>
          Full details
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
                    {usable.length} of {vpns.length}
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
                          <th scope="col" className="mv-list__actions-head">Actions</th>
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

              {/* ---- Not in service (D-100) ---------------------------------
               *
               * A suspended or revoked VPN is a RECORD, not inventory: the
               * client cannot connect through it and cannot resume it without
               * the seller. Sitting it beside working VPNs meant the working
               * ones were harder to find, and it is what pushed it to the top
               * under "Newest first".
               *
               * It is shown, not hidden (D-091.5 / INV-1). Each row keeps its
               * sentence and its panel identity, because "where did my VPN go?"
               * is answered by seeing it here — never by a list that quietly
               * stopped listing it.
               */}
              {resting.length > 0 && (
                <ClientSection
                  title="Not in service"
                  actions={
                    <span className="ui-muted t-sm">
                      {resting.length} {resting.length === 1 ? 'VPN' : 'VPNs'} kept for your records
                    </span>
                  }
                >
                  <p className="mv-resting__note">
                    These are still yours and still on file. They cannot be used right now, so
                    they are kept here instead of among your working VPNs.
                  </p>
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
                            <th scope="col" className="mv-list__actions-head">Actions</th>
                          </tr>
                        </thead>
                        <tbody>{resting.map(renderRow)}</tbody>
                      </table>
                    </div>
                  ) : (
                    <div className="mv-grid">{resting.map(renderCard)}</div>
                  )}
                </ClientSection>
              )}
            </>
          )}
        </AsyncState>
      </div>
      <VpnQuickView vpn={quick} onClose={() => setQuick(null)} />

      {/* Replace confirmation (D-112). Names both halves — a new VPN, and the
          current one released with a 3-day undo — because the action has two
          side effects and a customer must not discover the second one after. */}
      <Dialog
        open={replaceTarget !== null}
        title={replaceTarget ? `Replace ${vpnDisplayName(replaceTarget)}?` : 'Replace this VPN?'}
        onClose={() => {
          if (!replacing) setReplaceTarget(null)
        }}
        footer={
          <>
            <Button variant="ghost" onClick={() => setReplaceTarget(null)} disabled={replacing}>
              Cancel
            </Button>
            <Button
              onClick={() => void doReplace()}
              disabled={replacing}
              data-testid="vpn-replace-confirm"
            >
              {replacing ? 'Replacing…' : 'Replace this VPN'}
            </Button>
          </>
        }
      >
        {replaceTarget && (
          <div className="ui-col" data-testid="vpn-replace-dialog">
            <p className="t-sm">
              This creates a <strong>new</strong> VPN with fresh traffic and expiry, and confirms it
              on the server <strong>before</strong> the current one is released — so you are never
              left without a working VPN. You will have <strong>3 days</strong> to undo the release.
            </p>
            <p className="t-sm2">
              Current VPN: <code data-testid="vpn-replace-old">{replaceTarget.panelEmail}</code>
              <br />
              <span className="t-xs">
                A replacement always gets a new panel identity — it is a different VPN on the
                server, not the same one renamed.
              </span>
            </p>
            {replaceTarget.kind === 'FREE' && (
              <p className="t-sm2 of-note of-note--warn" data-testid="vpn-replace-claim-note">
                This uses <strong>one FREE claim</strong> from your 30-day allowance on this
                server. Releasing the old VPN does not refund it.
              </p>
            )}
            {replaceError && (
              <p className="ui-field__error" role="alert" data-testid="vpn-replace-error">
                {replaceError}
              </p>
            )}
          </div>
        )}
      </Dialog>

      {/* The outcome, stated with both panel identities so a support thread can
          begin without guessing which client the customer now holds. */}
      <Dialog
        open={replaceDone !== null}
        title="VPN replaced"
        onClose={() => setReplaceDone(null)}
        footer={
          <Button onClick={() => setReplaceDone(null)} data-testid="vpn-replace-done-close">
            Done
          </Button>
        }
      >
        {replaceDone && (
          <div className="ui-col" data-testid="vpn-replace-done">
            <p className="t-sm">
              Your new VPN is <code data-testid="vpn-replace-new">{replaceDone.newEmail}</code>. It
              is ready to use now.
            </p>
            <p className="t-sm2">
              The previous VPN <code data-testid="vpn-replace-released">{replaceDone.oldEmail}</code>{' '}
              was released, not deleted. You can restore it for 3 days from its entry in your list.
            </p>
            {replaceDone.claimConsumed && (
              <p className="t-sm2">
                This used one FREE claim. {replaceDone.claimsRemaining} left in the last 30 days on
                this server.
              </p>
            )}
          </div>
        )}
      </Dialog>
    </ClientPage>
  )
}
