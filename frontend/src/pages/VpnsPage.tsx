import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  AsyncState,
  Badge,
  Button,
  Card,
  Dialog,
  Dropdown,
  DropdownItem,
  EmptyState,
  Input,
  PageHeader,
  Pagination,
  Select,
  SelectionBar,
  SavedViewsBar,
  Skeleton,
  StatusBadge,
  Table,
  useToast,
  type Column,
} from '../components/ui'
import { ArrowRightLeft, Check, Globe, Server, UserPlus } from 'lucide-react'
import { api, ApiError, type ApiAdminVPNRow, type ApiAdminVPNQuery } from '../lib/api'
import { useListQuery } from '../hooks/useListQuery'
import { useServersStore } from '../stores/serversStore'
import { relativeTime } from '../lib/time'
import { AssignVpnDialog } from '../components/vpn/AssignVpnDialog'
import { TransferVpnDialog } from '../components/vpn/TransferVpnDialog'
import { VPNCommandBar } from '../components/vpn/VPNCommandBar'
import type { ApiAdminVPNSummary } from '../lib/api'
import '../components/vpn/vpn-cmdbar.css'
import './clients.css'

/** Identifies a row by its panel identity, which exists for a VPN Veylix has no row for. */
function rowKey(v: ApiAdminVPNRow): string {
  return `${v.server_id}:${v.panel_email}`
}

/**
 * Super Admin — VPN Management (D-043).
 *
 * The owner's requirement, in one screen: "the system's every VPN — including the
 * ones nobody has been assigned — and who holds each one". The list is the panel's
 * clients MERGED with Veylix's rows (`source` says which), so an untagged or
 * unassigned panel client appears here even though no Veylix record exists for it.
 *
 * A row whose panel could not be read says so (`read_error`) instead of showing
 * zero usage: "we could not ask" and "nothing used" are different facts.
 */
export function VpnsPage() {
  const navigate = useNavigate()
  const toast = useToast()
  const [rows, setRows] = useState<ApiAdminVPNRow[]>([])
  const [total, setTotal] = useState(0)
  const [unread, setUnread] = useState(0)
  /**
   * The whole catalogue counted before filtering, so a tile keeps its number
   * while a different tile's list is open. `null` until the first response —
   * the strip then renders nothing rather than six zeroes.
   */
  const [summary, setSummary] = useState<ApiAdminVPNSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  /** Panel sync freshness (#9) and the manual trigger's own state (#8). */
  const [syncAge, setSyncAge] = useState<number | null>(null)
  const [syncStale, setSyncStale] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [syncNote, setSyncNote] = useState('')
  /** The panel client awaiting an owner, or null when the dialog is closed. */
  const [assignTarget, setAssignTarget] = useState<ApiAdminVPNRow | null>(null)
  /**
   * The ASSIGNED VPN awaiting a new owner (D-064). Separate from `assignTarget`
   * because the two dialogs do different jobs: one claims a client nobody holds,
   * the other moves a VPN that already works.
   */
  const [transferTarget, setTransferTarget] = useState<ApiAdminVPNRow | null>(null)
  /**
   * P-2: the row the operator is releasing on the owner's behalf.
   *
   * A separate dialog from transfer/assign because a release is destructive-ish —
   * it opens a 3-day undo window and takes the VPN away from a paying customer. It
   * REQUIRES a typed reason, which the API enforces (422 `reason_required`); the
   * dialog makes that requirement visible instead of surfacing it as an error.
   */
  const [releaseTarget, setReleaseTarget] = useState<ApiAdminVPNRow | null>(null)
  /**
   * P-3: the row whose local RECORD the operator is retiring while keeping the
   * panel client. Guarded by type-to-confirm on the panel email — the VPN's
   * identity (D-091.4) — so a mis-clicked row cannot destroy the wrong record.
   */
  const [purgeTarget, setPurgeTarget] = useState<ApiAdminVPNRow | null>(null)
  /** Typed reason for a forced release. Required by the API (P-2). */
  const [releaseReason, setReleaseReason] = useState('')
  const [busyRelease, setBusyRelease] = useState(false)
  /** Typed panel email for a purge confirmation (P-3). */
  const [purgeConfirm, setPurgeConfirm] = useState('')
  const [busyPurge, setBusyPurge] = useState(false)
  /**
   * The row currently being re-classified, keyed by `rowKey`. Present so the
   * clicked pair disables itself instead of accepting a second click while the
   * first is in flight — a double-tag would be two audit entries for one intent.
   */
  const [tagging, setTagging] = useState<string | null>(null)
  /**
   * Rows selected for a bulk tag (Phase-3 UX, 3.1).
   *
   * Keyed by `rowKey` rather than by VPN id, because a panel client Veylix has no
   * row for has NO id — and those are precisely the untagged clients a bulk tag
   * exists to classify.
   */
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [bulkBusy, setBulkBusy] = useState<string | null>(null)

  /* Server grouping: the owner asked to organise VPNs per server and see one
   * server's whole set by clicking it, so the list starts as servers, not rows. */
  const servers = useServersStore((s) => s.servers)
  const loadServers = useServersStore((s) => s.load)
  useEffect(() => {
    void loadServers()
  }, [loadServers])

  /**
   * Filters live in the URL (D-051) and text is debounced: a VPN list is something an
   * operator narrows, keeps and sends to a colleague, and typing must not fire one
   * SQL query per keystroke.
   */
  const q = useListQuery({
    defaults: {
      q: '',
      tag: '',
      assigned: '',
      usage: '',
      expiry: '',
      server_id: '',
      owner_id: '',
      seller_id: '',
      page: 1,
    },
    debounced: ['q'],
  })

  /** Rows per page. Matches the server default so the first load is one page. */
  const PAGE_SIZE = 50

  const load = async (next: ApiAdminVPNQuery) => {
    // `loading` is set at the START of a request, but only from an event handler or
    // after a tick — never synchronously inside the effect that triggers it, which
    // is the cascading render `react(set-state-in-effect)` warns about.
    await Promise.resolve()
    setLoading(true)
    setError(null)
    try {
      const res = await api.adminVpns(next)
      setRows(res.vpns ?? [])
      setTotal(res.total ?? 0)
      setUnread(res.unread ?? 0)
      setSummary(res.summary ?? null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not read the VPN list')
      setRows([])
    } finally {
      setLoading(false)
    }
  }

  const committed: ApiAdminVPNQuery = useMemo(
    () => ({
      q: q.values.q || undefined,
      tag: (q.values.tag || undefined) as ApiAdminVPNQuery['tag'],
      assigned: (q.values.assigned || undefined) as ApiAdminVPNQuery['assigned'],
      usage: (q.values.usage || undefined) as ApiAdminVPNQuery['usage'],
      expiry: (q.values.expiry || undefined) as ApiAdminVPNQuery['expiry'],
      server_id: q.values.server_id ? Number(q.values.server_id) : undefined,
      owner_id: q.values.owner_id ? Number(q.values.owner_id) : undefined,
      seller_id: q.values.seller_id ? Number(q.values.seller_id) : undefined,
      // The page travels with the filters, so a shared link reopens the same
      // page of the same list (and Back returns to it).
      limit: PAGE_SIZE,
      offset: (Math.max(1, q.values.page) - 1) * PAGE_SIZE,
    }),
    [q.values],
  )

  /**
   * Publishes a panel client as FREE or PREMIUM (D-024.3).
   *
   * The reload afterwards is not cosmetic: the summary tiles are counted from the
   * same response, so tagging the last untagged VPN must visibly empty the
   * "Untagged" tile. A screen that kept showing 4 after the fourth was classified
   * would send the operator looking for a fifth that does not exist.
   *
   * `409 tag_locked` is surfaced as a refusal, never swallowed: an operator who
   * clicked a control the server rejects must be told why nothing happened.
   */
  const setTag = async (v: ApiAdminVPNRow, tag: 'FREE' | 'PREMIUM') => {
    const key = rowKey(v)
    if (tagging) return
    if (v.tag === tag) {
      // Already classified this way — a re-write would only add an audit entry.
      return
    }
    setTagging(key)
    try {
      await api.setPanelClientTag(v.server_id, v.panel_email, tag)
      toast('success', `${v.panel_email} is ${tag}`, `Published — it is now ${tag} everywhere.`)
      await load(committed)
    } catch (err) {
      const msg =
        err instanceof ApiError ? err.message : 'The tag could not be saved'
      toast('error', 'Tag not changed', msg)
    } finally {
      setTagging(null)
    }
  }

  /**
   * Copies a panel client's name, which is how an operator finds that exact
   * client inside 3X-UI. Clipboard access can be refused (insecure context), so
   * the failure is reported rather than swallowed.
   */
  const copyPanelEmail = async (panelEmail: string) => {
    try {
      await navigator.clipboard.writeText(panelEmail)
      toast('success', 'Copied', panelEmail)
    } catch {
      toast('error', 'Could not copy', 'Your browser refused clipboard access')
    }
  }

  /**
   * P-2 — release a customer's VPN on their behalf.
   *
   * The reason is not optional and not invented here: it goes into the audit trail
   * as `vpn.force_release` and it is what the customer will read. The API refuses a
   * blank one with 422, so the dialog disables the button rather than letting an
   * operator discover the rule by error message.
   */
  const submitRelease = async () => {
    const target = releaseTarget
    if (!target || releaseReason.trim() === '') return
    setBusyRelease(true)
    try {
      await api.releaseVpn(target.id, releaseReason.trim())
      toast(
        'success',
        'Released',
        `${target.panel_email} is set aside. The owner has 3 days to undo it and the panel client is untouched.`,
      )
      setReleaseTarget(null)
      setReleaseReason('')
      void load(committed)
    } catch (err) {
      toast(
        'error',
        'Could not release the VPN',
        err instanceof Error ? err.message : 'Nothing was changed',
      )
    } finally {
      setBusyRelease(false)
    }
  }

  /**
   * P-3 — retire the local record while keeping the panel client.
   *
   * The confirmation is checked here as well as on the server: the button only
   * enables on an exact (case-insensitive) match, so a wrong value never becomes a
   * round-trip. The server repeats the check, because a client-side guard is not a
   * guard.
   */
  const submitPurge = async () => {
    const target = purgeTarget
    if (!target) return
    if (purgeConfirm.trim().toLowerCase() !== target.panel_email.toLowerCase()) return
    setBusyPurge(true)
    try {
      await api.purgeVpnRecord(target.id, purgeConfirm.trim())
      toast(
        'success',
        'Record purged',
        `${target.panel_email} is gone from Veylix. The 3X-UI client is still there and the customer is unaffected.`,
      )
      setPurgeTarget(null)
      setPurgeConfirm('')
      void load(committed)
    } catch (err) {
      toast(
        'error',
        'Could not purge the record',
        err instanceof Error ? err.message : 'Nothing was changed',
      )
    } finally {
      setBusyPurge(false)
    }
  }

  /**
   * Classifies every selected VPN, ONE AT A TIME.
   *
   * Sequential on purpose: the deployment target is 1 vCPU (MASTER_PRODUCT_BIBLE
   * §9), and fifty parallel writes against SQLite is how a bulk action becomes an
   * outage. Each row's real outcome is counted, so a partial success is reported
   * as a partial success rather than rounded to "done".
   *
   * Rows the list no longer holds (someone else filtered them away) are skipped
   * silently: they were never in this selection's scope.
   */
  const runBulkTag = async (tag: 'FREE' | 'PREMIUM') => {
    if (selected.size === 0 || bulkBusy) return
    let ok = 0
    let failed = 0
    const byKey = new Map(rows.map((r) => [rowKey(r), r]))
    for (const key of selected) {
      const v = byKey.get(key)
      if (!v) continue
      try {
        await api.setPanelClientTag(v.server_id, v.panel_email, tag)
        ok++
      } catch {
        failed++
      }
    }
    setBulkBusy(null)
    setSelected(new Set())
    await load(committed)
    if (failed === 0) {
      toast('success', `${tag} applied`, `${ok} VPN${ok === 1 ? '' : 's'} classified`)
    } else {
      toast(
        'error',
        'Some tags were refused',
        `${ok} applied, ${failed} refused — an issued VPN's tag is locked`,
      )
    }
  }

  /** Pages the server says this result set fills. */
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))

  /**
   * Sync freshness (#9). The console shows usage numbers read from the panel by a
   * 5-minute scheduler; without the age of that data an operator cannot tell
   * "62% used now" from "62% used an hour ago". The failure is swallowed
   * deliberately: an unreadable status must not blank the VPN list.
   */
  const loadSync = useCallback(async () => {
    try {
      const s = await api.syncStatus()
      const ages = (s.servers ?? []).map((x) => x.age_seconds ?? 0)
      const stale = (s.servers ?? []).some((x) => x.stale)
      setSyncAge(ages.length > 0 ? Math.min(...ages) : null)
      setSyncStale(stale)
    } catch {
      setSyncAge(null)
    }
  }, [])

  useEffect(() => {
    void loadSync()
  }, [loadSync])

  /**
   * "Sync now" (#8). Runs the same pass the scheduler runs and reports what it
   * actually did — the counts come from the pass itself, not from an assumption
   * that the click worked.
   */
  const runSync = async () => {
    setSyncing(true)
    setSyncNote('')
    try {
      const res = await api.syncRun()
      const n = res.servers?.length ?? 0
      const failed = (res.servers ?? []).filter((s) => s.error).length
      setSyncNote(
        failed > 0
          ? `Read ${n - failed} of ${n} panel(s) in ${res.duration_ms} ms — ${failed} did not answer.`
          : `Read ${n} panel(s) in ${res.duration_ms} ms.`,
      )
      await Promise.all([loadSync(), load(committed)])
    } catch (err) {
      setSyncNote(err instanceof ApiError ? err.message : 'The sync pass could not run.')
    } finally {
      setSyncing(false)
    }
  }

  /** "3 min ago" / "just now" — a raw second count is not readable. */
  const syncLabel =
    syncAge === null ? 'sync unknown' : syncAge < 60 ? 'synced just now' : `synced ${Math.round(syncAge / 60)} min ago`

  // One request per committed filter change, never on the draft. `committed` is
  // stable between URL changes (see useListQuery), so this fires once per change and
  // not once per render — the 653-request loop from D-052 cannot recur here.
  // oxlint-disable-next-line react-hooks/exhaustive-deps -- load sets state after an await, not during commit
  useEffect(() => {
    void load(committed)
  }, [committed])

  const columns: Column<ApiAdminVPNRow>[] = [
    {
      key: 'panel_email',
      header: 'Panel client',
      sortable: true,
      searchValue: (v) => v.panel_email,
      filter: { kind: 'text', getValue: (v) => v.panel_email, placeholder: 'Panel client…' },
      render: (v) => (
        <span className="cl-cell__text">
          <span className="ui-strong">{v.panel_email}</span>
          <span className="ui-muted cl-cell__sub">
            server #{v.server_id}
            {v.protocol && (
              <span className="cl-tag">
                {v.protocol}
                {v.transport ? '/' + v.transport : ''}
              </span>
            )}
            {v.source === 'local' && <span className="cl-tag cl-tag--warn">local only</span>}
          </span>
        </span>
      ),
    },
    {
      key: 'tag',
      header: 'Tag',
      sortable: true,
      sortValue: (v) => v.tag || '',
      searchValue: (v) => `${v.tag ?? ''} ${v.tag ? '' : 'untagged'}`,
      // The tag is a small closed set, so it is a select — an operator wants
      // "show me the untagged ones", not a substring match.
      filter: {
        kind: 'select',
        options: [
          { value: 'FREE', label: 'FREE' },
          { value: 'PREMIUM', label: 'PREMIUM' },
          // `''` cannot mean "untagged" — an empty select value is how the
          // toolbar says "no filter". A sentinel keeps the two distinguishable.
          { value: '(none)', label: 'Untagged' },
        ],
        getValue: (v) => (v.tag ? v.tag : '(none)'),
      },
      /*
       * FREE / PREMIUM is the Super Admin's publishing decision (D-024.3), and
       * until now it existed only as an API route — so an untagged panel client
       * created directly in 3X-UI could never enter the FREE catalogue at all.
       *
       * The control is a segmented pair rather than a dropdown because there are
       * exactly two states and both must be one click away: the decision is taken
       * while scanning the list, not after opening a menu.
       *
       * It is disabled once the VPN is ISSUED, mirroring the SQL lock (D-025.2):
       * re-pricing a sold VPN would retroactively change what the customer paid
       * for. The server refuses with `409 tag_locked`; disabling here just avoids
       * offering a control that cannot work.
       */
      render: (v) => (
        <span className="cl-tagpick" role="group" aria-label={`Tag for ${v.panel_email}`}>
          <button
            type="button"
            className={`cl-tagpick__btn cl-tagpick__btn--hover${v.tag === 'FREE' ? ' is-on' : ''}`}
            disabled={tagging === rowKey(v) || v.assigned}
            title={
              v.assigned
                ? 'Issued — the tag is locked (D-025.2)'
                : 'Publish this VPN in the FREE catalogue'
            }
            onClick={() => void setTag(v, 'FREE')}
          >
            FREE
          </button>
          <button
            type="button"
            className={`cl-tagpick__btn cl-tagpick__btn--sep cl-tagpick__btn--hover${v.tag === 'PREMIUM' ? ' is-on' : ''}`}
            disabled={tagging === rowKey(v) || v.assigned}
            title={
              v.assigned
                ? 'Issued — the tag is locked (D-025.2)'
                : 'Publish this VPN as PREMIUM (resellable)'
            }
            onClick={() => void setTag(v, 'PREMIUM')}
          >
            PREMIUM
          </button>
        </span>
      ),
    },
    {
      key: 'owner_name',
      header: 'Assigned to',
      sortable: true,
      // Sorted by account, then "unassigned" sorts ahead of everyone because an
      // empty string precedes any name — which is where the operator's eye
      // should go first.
      sortValue: (v) => (v.assigned ? v.owner_name || v.owner_email || '' : ''),
      searchValue: (v) =>
        v.assigned ? `${v.owner_name ?? ''} ${v.owner_email ?? ''} unassigned` : 'unassigned',
      filter: {
        kind: 'select',
        options: [
          { value: 'yes', label: 'Assigned' },
          { value: '(none)', label: 'Unassigned' },
        ],
        getValue: (v) => (v.assigned ? 'yes' : '(none)'),
      },
      render: (v) =>
        v.assigned ? (
          <span className="cl-cell__text">
            <span>{v.owner_name || v.owner_email || '#' + v.owner_user_id}</span>
            {v.seller_user_id > 0 && (
              <span className="ui-muted cl-cell__sub">sold by #{v.seller_user_id}</span>
            )}
          </span>
        ) : (
          <Badge tone="warning">unassigned</Badge>
        ),
    },
    {
      key: 'usage_pct',
      header: 'Usage',
      align: 'center',
      sortable: true,
      sortValue: (v) => (v.read_error ? -1 : v.usage_pct),
      render: (v) =>
        v.read_error ? (
          <span className="ui-muted" title="The panel could not be read for this server">—</span>
        ) : (
          <span className="cl-free" title={`${v.used_bytes} of ${v.limit_bytes} bytes`}>
            {v.usage_pct}%
          </span>
        ),
    },
    {
      key: 'days_left',
      header: 'Expiry',
      align: 'center',
      hideOnMobile: true,
      sortable: true,
      // A VPN with no expiry sorts last: "never expires" is not sooner than
      // everything else.
      sortValue: (v) => (v.read_error || !v.expires_at ? Number.MAX_SAFE_INTEGER : v.days_left),
      filter: {
        kind: 'select',
        options: [
          { value: 'expired', label: 'Expired' },
          { value: 'soon', label: 'Expiring within 7 days' },
          { value: 'never', label: 'Never expires' },
        ],
        getValue: (v) => {
          if (v.read_error) return ''
          if (!v.expires_at) return 'never'
          if (v.days_left < 0) return 'expired'
          return v.days_left <= 7 ? 'soon' : ''
        },
      },
      render: (v) => {
        if (v.read_error) return <span className="ui-muted">—</span>
        if (!v.expires_at) return <span className="ui-muted">never</span>
        return v.days_left < 0 ? (
          <Badge tone="error">expired</Badge>
        ) : (
          <span className={v.days_left <= 7 ? 'cl-tag cl-tag--warn' : 'ui-muted'}>{v.days_left}d</span>
        )
      },
    },
    {
      key: 'enabled',
      header: 'Panel',
      align: 'center',
      hideOnMobile: true,
      sortable: true,
      sortValue: (v) => (v.enabled ? 1 : 0),
      filter: {
        kind: 'select',
        options: [
          { value: 'on', label: 'Enabled' },
          { value: 'off', label: 'Disabled' },
        ],
        getValue: (v) => (v.enabled ? 'on' : 'off'),
      },
      render: (v) =>
        v.enabled ? <Badge tone="success">enabled</Badge> : <Badge tone="warning">disabled</Badge>,
    },
    {
      key: 'status',
      header: 'Veylix',
      hideOnMobile: true,
      sortable: true,
      sortValue: (v) => v.status || '',
      searchValue: (v) => (v.status || 'no record').replace(/_/g, ' '),
      filter: {
        kind: 'select',
        options: [
          { value: 'ACTIVE', label: 'Active' },
          { value: 'SUSPENDED', label: 'Suspended' },
          { value: 'SERVER_UNAVAILABLE', label: 'Server unavailable' },
          { value: 'REVOKED', label: 'Revoked' },
          { value: 'RELEASED', label: 'Released' },
          { value: '(none)', label: 'No record' },
        ],
        // A panel client with no Veylix row is a real and important state, so it
        // gets its own choice instead of hiding among the blanks.
        getValue: (v) => (v.status ? v.status : '(none)'),
      },
      render: (v) => {
        /*
         * Veylix's own lifecycle state and the panel's enable flag are two
         * independent facts, and they disagree in the one case that matters:
         * the panel has disabled a client while Veylix still calls the VPN
         * ACTIVE. Read on its own, "ACTIVE" told the operator a VPN worked when
         * it could not connect. The conflict is now stated on the row — never
         * inferred, never silently resolved, because only the operator knows
         * which side is right.
         */
        if (!v.status) return <span className="ui-muted">no record</span>
        const conflict =
          (v.status === 'ACTIVE' && !v.enabled) ||
          (v.status === 'SUSPENDED' && v.enabled)
        return (
          <span className="cl-cell__text">
            <StatusBadge status={v.status} />
            {conflict && (
              <span
                className="cl-tag cl-tag--warn"
                title={
                  v.status === 'ACTIVE'
                    ? 'Veylix says ACTIVE but the panel has this client disabled — it cannot connect.'
                    : 'Veylix says SUSPENDED but the panel has this client enabled — the block is not in force.'
                }
              >
                panel disagrees
              </span>
            )}
          </span>
        )
      },
    },
    {
      key: 'created_at',
      header: 'Since',
      sortable: true,
      sortValue: (v) => v.created_at ?? 0,
      hideOnMobile: true,
      render: (v) => <span className="ui-muted">{relativeTime(v.created_at)}</span>,
    },
    {
      key: 'assign',
      header: '',
      sticky: true,
      align: 'right',
      /*
       * One primary action plus one menu (Phase-3 UX, 3.3).
       *
       * The slot used to carry an "Owner →" link, a "Move" button, a "Detail →"
       * button AND the table's own row affordances — four targets inside a row
       * that is itself clickable on a phone. The primary intent stays a button;
       * everything else is behind "⋯", so a mis-tap can no longer mean "revoke".
       */
      render: (v) => (
        <span className="ui-row vp-row-actions">
          {v.assigned ? (
            <Button
              size="sm"
              variant="outline"
              leading={<ArrowRightLeft size={13} />}
              onClick={() => setTransferTarget(v)}
            >
              Move
            </Button>
          ) : (
            <Button
              size="sm"
              variant="primary"
              leading={<UserPlus size={13} />}
              onClick={() => setAssignTarget(v)}
            >
              Assign
            </Button>
          )}
          <Dropdown
            ariaLabel={`Actions for ${v.panel_email}`}
            trigger={
              <span className="cl-open" role="button" tabIndex={0} title="More actions">
                ⋯
              </span>
            }
          >
            <DropdownItem
              onClick={() => navigate(`/vpns/${v.server_id}/${encodeURIComponent(v.panel_email)}`)}
            >
              Open detail
            </DropdownItem>
            {v.assigned && v.owner_user_id > 0 && (
              <DropdownItem onClick={() => navigate(`/clients/${v.owner_user_id}`)}>
                Open the owner's profile
              </DropdownItem>
            )}
            {!v.assigned && (
              <DropdownItem>
                Move — assign it to somebody first
              </DropdownItem>
            )}
            {v.assigned && (
              <DropdownItem onClick={() => void copyPanelEmail(v.panel_email)}>
                Copy panel name
              </DropdownItem>
            )}
            {/*
              P-2 (D-106): the platform owner could not release a VPN at all — the
              route answered 403 to every non-client, which left the 3X-UI panel as
              the only way to act on an abused account, where nothing is audited.
              Offered only for a VPN that is actually assigned: releasing an
              unassigned one means nothing to a customer.
            */}
            {v.assigned && v.status !== 'RELEASED' && (
              <DropdownItem onClick={() => setReleaseTarget(v)}>
                Release on the owner's behalf…
              </DropdownItem>
            )}
            {/*
              P-3 (D-106): retire the RECORD while keeping the panel client. Offered
              separately from Delete because the two answer different questions, and
              "delete" would imply the panel client goes too — it never does
              (D-027.3).
            */}
            <DropdownItem onClick={() => setPurgeTarget(v)}>
              Purge the Veylix record (keep the panel client)…
            </DropdownItem>
          </Dropdown>
        </span>
      ),
    },
  ]

  return (
    <div className="dash" data-testid="vpns-page">
      <PageHeader
        title="VPN management"
        subtitle="Every VPN the system knows — panel clients included, assigned or not"
        actions={
          <div className="ui-row vp-toolbar">
            {/*
              Freshness #9: the usage numbers come from a scheduled panel read, so
              their age is part of the number. Without this, "62% used" looked the
              same whether it was measured a minute or an hour ago.
            */}
            <span
              className={syncStale ? 'cl-tag cl-tag--warn' : 'cl-tag'}
              title={
                syncStale
                  ? 'The panel read is older than the stale threshold — the usage shown may be out of date.'
                  : 'How old the panel read behind these numbers is.'
              }
            >
              {syncLabel}
            </span>
            {/*
              #8: the button existed nowhere. It runs the SAME pass the scheduler
              runs and reports the pass's own counts.
            */}
            <Button size="sm" variant="outline" loading={syncing} onClick={() => void runSync()}>
              Sync now
            </Button>
            {q.isFiltered && (
              <Button variant="ghost" size="sm" onClick={q.reset}>
                Clear filters
              </Button>
            )}
            <Button variant="outline" size="sm" leading={<Check size={14} />} onClick={() => void load(committed)}>
              Refresh
            </Button>
            {/* Saved views sit with the filters they save (Phase-6 UX, 4.4). */}
            <SavedViewsBar screen="vpns" />
          </div>
        }
      />

      {error && <p className="cl-alert" role="alert">{error}</p>}

      {/*
        The catalogue at a glance (Phase-1 UX, P2). Rendered above the server
        picker because "what needs me today" outranks "which server" — and every
        tile is a filter, so the number and the list it opens cannot disagree.
      */}
      <VPNCommandBar
        summary={summary}
        values={committed}
        onPick={(patch) => q.setNow(patch)}
        onShowAll={q.reset}
      />

      {/*
       * VPNs are grouped by SERVER first — the owner's requirement: click a server
       * and see every VPN on it. The chosen server lives in the URL
       * (`?server_id=`), so the filtered view is shareable and the back button
       * returns to the full set.
       */}
      <Card
        title="Servers"
        actions={
          q.values.server_id ? (
            <Button size="sm" variant="ghost" onClick={() => q.set({ server_id: '' })}>
              Show all servers
            </Button>
          ) : (
            <span className="ui-muted t-sm">{servers.length} integrated</span>
          )
        }
      >
        {servers.length === 0 ? (
          <p className="ui-muted rhythm">
            No 3X-UI server is integrated yet, so there is nothing to group by. Add one under
            Server integrations.
          </p>
        ) : (
          <div className="client-offers-grid">
            {servers.map((srv) => {
              const chosen = q.values.server_id === String(srv.apiId)
              return (
                <button
                  key={srv.id}
                  type="button"
                  className={`client-vpn-card cl-server-pick${chosen ? ' is-active' : ''}`}
                  onClick={() => q.setNow({ server_id: chosen ? '' : String(srv.apiId) })}
                  aria-pressed={chosen}
                >
                  <div className="client-vpn-card__top">
                    <span className="ui-strong">
                      <Server size={13} aria-hidden="true" /> {srv.name}
                    </span>
                    {chosen ? <Badge tone="primary">showing</Badge> : <Badge tone="default">#{srv.apiId}</Badge>}
                  </div>
                  <span className="ui-muted t-sm2">
                    {chosen ? 'Showing every VPN on this server' : 'Click to see every VPN on this server'}
                  </span>
                </button>
              )
            })}
          </div>
        )}
      </Card>

      {unread > 0 && (
        <p className="cl-alert cl-alert--soft" role="status">
          {unread} of {total} VPNs could not be read from their panel. Their usage and
          expiry are shown as “—” rather than zero.
        </p>
      )}

      {syncNote && (
        <p className="cl-alert cl-alert--soft" role="status">
          {syncNote}
        </p>
      )}

      <Card>
        <form className="cl-filters" onSubmit={(e) => e.preventDefault()}>
          <div className="cl-filters__search">
            <Input
              label="Search"
              placeholder="panel client, owner, offer"
              value={q.draft.q}
              onChange={(e) => q.set({ q: e.target.value })}
              onBlur={() => q.setNow({ q: q.draft.q })}
            />
          </div>
          <Select label="Tag" value={q.draft.tag} onChange={(e) => q.set({ tag: e.target.value })}>
            <option value="">Any tag</option>
            <option value="free">FREE</option>
            <option value="premium">PREMIUM</option>
            <option value="untagged">Untagged</option>
          </Select>
          <Select label="Assignment" value={q.draft.assigned} onChange={(e) => q.set({ assigned: e.target.value })}>
            <option value="">Anyone</option>
            <option value="1">Assigned</option>
            <option value="0">Unassigned</option>
          </Select>
          <Select label="Usage" value={q.draft.usage} onChange={(e) => q.set({ usage: e.target.value })}>
            <option value="">Any usage</option>
            <option value="healthy">Healthy</option>
            <option value="expiring">Expiring (≤7d)</option>
            <option value="expired">Expired</option>
            <option value="exhausted">Traffic exhausted</option>
            <option value="unused">Unused</option>
          </Select>
          <Select label="Expiry" value={q.draft.expiry} onChange={(e) => q.set({ expiry: e.target.value })}>
            <option value="">Any expiry</option>
            <option value="7d">Within 7 days</option>
            <option value="30d">Within 30 days</option>
          </Select>
          {/* #14: owner/seller exist on the wire and are used by ops ("show me
              everything this reseller sold"), but had no control. Kept as ids
              rather than a picker because the console does not fetch the staff
              list, and inventing a name→id map here would be a guess. */}
          <Input
            label="Owner user id"
            type="number"
            min={0}
            placeholder="any"
            value={q.draft.owner_id}
            onChange={(e) => q.set({ owner_id: e.target.value })}
          />
          <Input
            label="Seller user id"
            type="number"
            min={0}
            placeholder="any"
            value={q.draft.seller_id}
            onChange={(e) => q.set({ seller_id: e.target.value })}
          />
          {/* A server DROPDOWN, not a number box: the old control made the
              operator memorise a database id and type it correctly. */}
          <Select
            label="Server"
            value={q.draft.server_id}
            onChange={(e) => q.set({ server_id: e.target.value })}
          >
            <option value="">Any server</option>
            {servers.map((s) => (
              <option key={s.apiId} value={s.apiId}>
                {s.name}
                {s.status !== 'ACTIVE' ? ` (${s.status.toLowerCase().replaceAll('_', ' ')})` : ''}
              </option>
            ))}
          </Select>
        </form>
      </Card>

      {/*
        Bulk classification (Phase-3 UX, 3.1).
        The untagged queue is a *queue*: it arrives in batches of a dozen and the
        decision is the same for all of them. Classifying them one dialog at a time
        was the slowest routine job on this screen.
      */}
      <SelectionBar count={selected.size} onClear={() => setSelected(new Set())}>
        <Button
          size="sm"
          variant="outline"
          loading={bulkBusy === 'FREE'}
          disabled={bulkBusy !== null}
          onClick={() => void runBulkTag('FREE')}
        >
          Mark FREE
        </Button>
        <Button
          size="sm"
          variant="outline"
          loading={bulkBusy === 'PREMIUM'}
          disabled={bulkBusy !== null}
          onClick={() => void runBulkTag('PREMIUM')}
        >
          Mark PREMIUM
        </Button>
      </SelectionBar>

      <Card title={total > 0 ? `VPNs · ${total}` : 'VPNs'}>
        <AsyncState
          status={error ? 'error' : loading ? 'loading' : rows.length === 0 ? 'empty' : 'ready'}
          error={error}
          onRetry={() => void load(committed)}
          skeleton={<Skeleton height={220} />}
          empty={
            q.isFiltered ? (
              <EmptyState
                icon={<Globe size={22} />}
                title="No VPNs match these filters"
                description="Widen the filters, or clear them. Unassigned panel clients appear here too."
                action={
                  <Button variant="outline" size="sm" onClick={q.reset}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={<Globe size={22} />}
                title="No VPNs yet"
                description="A VPN appears here once its panel client is synced from 3X-UI. If you have not integrated a panel yet, that is the first step."
                /*
                 * On a fresh install this screen is the operator's first stop and
                 * used to be a dead end: it said a VPN would appear once synced,
                 * without saying from where. The action is the actual first step.
                 */
                action={
                  <Button size="sm" onClick={() => navigate('/servers')}>
                    {servers.length === 0 ? 'Add a 3X-UI server' : 'Open server integrations'}
                  </Button>
                }
              />
            )
          }
        >
          <Table
            columns={columns}
            data={rows}
            rowKey={(v) => `${v.server_id}:${v.panel_email}`}
            searchable
            searchPlaceholder="Search panel client, owner, status or tag…"
            caption="VPNs"
            selection={{
              selected,
              onSelectedChange: setSelected,
              /*
               * An ISSUED VPN cannot be re-classified (D-025.2), so it is not
               * selectable for a bulk tag. Offering the checkbox and then
               * refusing every row would teach the operator that the bulk action
               * is unreliable rather than that the tag is locked.
               */
              isSelectable: (row) => !(row as ApiAdminVPNRow).assigned,
            }}
            rowLabel={(v) => v.panel_email}
          />
        </AsyncState>
        {/* The list is windowed server-side. Without this control page 1 looked
            like the whole estate: `total` said 240 and 50 rows were shown. */}
        <Pagination
          page={Math.max(1, q.values.page)}
          pageCount={pageCount}
          onChange={(p) => q.setNow({ page: p })}
        />
        {total > 0 && (
          <p className="ui-muted t-sm2 vp-footnote">
            Showing {rows.length} of {total} VPN{total === 1 ? '' : 's'}
            {q.values.server_id ? ' on this server' : ''}. Unassigned panel clients sort first.
          </p>
        )}
      </Card>

      <AssignVpnDialog
        open={assignTarget !== null}
        onClose={() => setAssignTarget(null)}
        serverId={assignTarget?.server_id ?? 0}
        panelEmail={assignTarget?.panel_email ?? ''}
        onAssigned={(res) => {
          setAssignTarget(null)
          // The assignment succeeded in both branches; only the optional rename
          // differs, and a failed rename keeps the existing name (D-063) — so it
          // is a note, never an error.
          toast(
            'success',
            res.renamed ? 'VPN assigned and renamed' : 'VPN assigned',
            res.warning || `Now held by a customer as ${res.panelEmail} — premium.`,
          )
          void load(committed)
        }}
      />

      <TransferVpnDialog
        open={transferTarget !== null}
        onClose={() => setTransferTarget(null)}
        vpnId={transferTarget?.id ?? 0}
        panelEmail={transferTarget?.panel_email ?? ''}
        fromEmail={transferTarget?.owner_email ?? ''}
        onTransferred={(res) => {
          setTransferTarget(null)
          toast(
            'success',
            'VPN moved',
            `${res.panelEmail} now belongs to ${res.toEmail}. Its connection details did not change.`,
          )
          void load(committed)
        }}
      />

      <p className="ui-muted cl-note">
        <UserPlus size={13} aria-hidden="true" /> An unassigned panel client can be handed to a
        customer straight from this table. The VPN keeps the name the panel holds and becomes
        PREMIUM immediately (D-063); renaming it into the customer's scheme is a separate,
        optional step.
      </p>
      <p className="ui-muted cl-note">
        <ArrowRightLeft size={13} aria-hidden="true" /> <strong>Move</strong> transfers an
        already-assigned VPN to another customer. The 3X-UI client is never renamed, so the
        connection keeps working; the previous owner loses access immediately.
      </p>

      {/*
        P-2 — releasing on the owner's behalf.
        The reason field is required and the button stays disabled until it has
        something in it, so the 422 the API would return is never reached by a real
        operator. NN/g: be specific about what will happen (3 days, undoable, panel
        client untouched), not "are you sure?".
      */}
      <Dialog
        open={releaseTarget !== null}
        title={releaseTarget ? `Release ${releaseTarget.panel_email}?` : 'Release VPN'}
        onClose={() => {
          setReleaseTarget(null)
          setReleaseReason('')
        }}
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => {
                setReleaseTarget(null)
                setReleaseReason('')
              }}
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              disabled={releaseReason.trim() === '' || busyRelease}
              onClick={() => void submitRelease()}
            >
              Release this VPN
            </Button>
          </>
        }
      >
        <p className="ui-muted">
          This sets the VPN aside for its owner and opens a <strong>3-day undo window</strong>. The
          3X-UI client is <strong>not</strong> deleted and keeps working; the customer sees the
          reason you write here.
        </p>
        <Input
          label="Why are you releasing it? (required)"
          placeholder="e.g. abuse report received from the upstream provider"
          value={releaseReason}
          onChange={(e) => setReleaseReason(e.target.value)}
        />
      </Dialog>

      {/*
        P-3 — retire the record, keep the panel client.
        Type-to-confirm on the panel email: the VPN's identity (D-091.4). A dialog
        that only says "are you sure" would let a mis-clicked row destroy the wrong
        record.
      */}
      <Dialog
        open={purgeTarget !== null}
        title={purgeTarget ? `Purge the Veylix record for ${purgeTarget.panel_email}?` : 'Purge record'}
        onClose={() => {
          setPurgeTarget(null)
          setPurgeConfirm('')
        }}
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => {
                setPurgeTarget(null)
                setPurgeConfirm('')
              }}
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              disabled={purgeConfirm.trim().toLowerCase() !== (purgeTarget?.panel_email ?? '').toLowerCase() || busyPurge}
              onClick={() => void submitPurge()}
            >
              Purge the record
            </Button>
          </>
        }
      >
        <p className="ui-muted">
          This removes Veylix&rsquo;s <strong>record</strong> of the VPN. The 3X-UI client is{' '}
          <strong>never deleted</strong> — the customer&rsquo;s connection keeps working — but this
          console will no longer show the VPN, so it can no longer be suspended, moved or reported
          on. Prefer <strong>Release</strong> unless you are sure.
        </p>
        <Input
          label="Type the panel email to confirm"
          placeholder={purgeTarget?.panel_email ?? ''}
          value={purgeConfirm}
          onChange={(e) => setPurgeConfirm(e.target.value)}
        />
      </Dialog>
    </div>
  )
}


