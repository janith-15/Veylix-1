import { useEffect, useState } from 'react'
import {
  AsyncState,
  Badge,
  Button,
  Card,
  Dialog,
  EmptyState,
  PageHeader,
  Pagination,
  Skeleton,
  Table,
  useToast,
  type Column,
} from '../components/ui'
import { Download, ScrollText } from 'lucide-react'
import { useAuditStore, AUDIT_RANGES, type AuditEntry, type AuditFilters } from '../stores/auditStore'
import { useListQuery } from '../hooks/useListQuery'
import { api, type ApiAuditEntry, type ApiAuditQuery } from '../lib/api'
import { formatDateTime } from '../lib/time'
import { csvCell as csvCellImpl } from '../lib/csv'

const PAGE_SIZE = 50

/** Chip list — the ids are exactly the backend's accepted `kind` tokens. */
const KINDS: { id: string; label: string }[] = [
  { id: '', label: 'All' },
  { id: 'auth', label: 'Sign-ins' },
  { id: 'vpn', label: 'VPNs' },
  { id: 'payment', label: 'Payments' },
  { id: 'server', label: 'Servers' },
  { id: 'admin', label: 'Admin' },
  { id: 'client', label: 'Clients' },
  { id: 'other', label: 'Other' },
]

const RANGES: { id: AuditFilters['range']; label: string }[] = [
  { id: '', label: 'All time' },
  { id: '24h', label: '24h' },
  { id: '7d', label: '7d' },
  { id: '30d', label: '30d' },
]

/**
 * The dotted action is for machines; the operator reads a sentence. The raw
 * string stays on the row anyway — when a label drifts from the backend's
 * constants, the raw value is what makes the discrepancy visible.
 */
const ACTION_LABELS: Record<string, string> = {
  'auth.bootstrap': 'Platform bootstrapped',
  'auth.login': 'Signed in',
  'auth.login_failed': 'Sign-in refused',
  'auth.logout': 'Signed out',
  'auth.password_change': 'Password changed',
  'auth.password_change_failed': 'Password change refused',
  'auth.register': 'Account registered',
  'admin.quota_adjust': 'Quota changed',
  'admin.servers_assign': 'Server assignments updated',
  'chat.lock': 'Conversation lock changed',
  'chat.pin': 'Pinned message changed',
  'data.reset': 'Database reset',
  'offer.create': 'Offer created',
  'offer.update': 'Offer updated',
  'panel_client.tag': 'Panel client classified',
  'panel_client.tag_denied': 'Classification refused',
  'payment.cancel': 'Payment cancelled',
  'payment.confirm': 'Payment confirmed',
  'payment.create': 'Payment opened',
  'payment.reference': 'Payment reference attached',
  'payment.reject': 'Payment rejected',
  'payment.refund': 'Payment refunded',
  'payment.update': 'Order updated',
  'payment_method.create': 'Payment method added',
  'payment_method.update': 'Payment method updated',
  'server.add': 'Panel added',
  'server.credential_change': 'Panel credentials changed',
  'server.disable': 'Panel disabled',
  'server.remove': 'Panel removed',
  'server.test': 'Panel health check',
  'user.role_change': 'Account role changed',
  'user.seed': 'Account seeded',
  'vpn.absent': 'VPN missing on the panel',
  'vpn.provision': 'VPN created',
  'vpn.release': 'VPN released - 3 days to undo',
  'vpn.remove_absent': 'VPN record removed',
  'vpn.restore': 'VPN restored',
  'vpn.resume': 'VPN resumed',
  'vpn.revoke': 'VPN revoked',
  'vpn.suspend': 'VPN suspended',
  'vpn.transfer': 'VPN moved to another customer',
  'vpn.purge': 'Released VPN record removed',
}

/** OK/DENIED/ERROR are outcomes, not account states — they get their own tones
 *  instead of borrowing the account StatusBadge vocabulary (a trail entry is
 *  not a server). */
const RESULT_TONE: Record<string, 'success' | 'error' | 'warning'> = {
  OK: 'success',
  DENIED: 'error',
  ERROR: 'error',
}

/** '{}' is how the backend says "nothing was recorded beyond the action". */
function prettyContext(raw: string): string {
  if (!raw || raw.trim() === '{}') return ''
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

/**
 * The CSV cell escaper lives in `lib/csv.ts`, so it is unit-tested
 * (`npm run audit:csv-selftest`) instead of only reviewed. B-3: the previous
 * inline version quoted delimiters but let `=`, `+`, `-` and `@` through, so an
 * audit export opened in Excel could execute a cell as a formula.
 */
function csvCell(value: string): string {
  return csvCellImpl(value)
}

/** Writes entries as CSV — the format an incident write-up actually gets pasted
 *  into. Timestamps go out as UTC ISO so the file stays unambiguous. */
function downloadAuditCsv(entries: ApiAuditEntry[]): void {
  const lines = ['id,time_utc,actor_id,actor,action,target_type,target_id,result,context']
  for (const e of entries) {
    lines.push(
      [
        String(e.id),
        new Date(e.created_at * 1000).toISOString(),
        e.actor_id === null ? '' : String(e.actor_id),
        e.actor_name ?? '',
        e.action,
        e.target_type,
        e.target_id,
        e.result,
        e.context ?? '',
      ]
        .map(csvCell)
        .join(','),
    )
  }
  const blob = new Blob([lines.join('\r\n')], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `veylix-audit-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

/**
 * Audit log — the REAL trail (`GET /api/audit`).
 *
 * Phase 5: this page used to render three seeded, invented entries and a "Kind"
 * badge derived from the current filter rather than from the row. Both are gone.
 *
 * D-082 made the filters SERVER-SIDE (kind, a time window and a substring
 * search, in SQL) and returned the match count beside the window. The old
 * client-side filter over a fixed 100-row read could say "Nothing recorded for
 * payments" while the matching entry sat one page deeper — a filter that can
 * hide its own answer is worse than no filter.
 */
export function AuditPage() {
  const toast = useToast()
  const items = useAuditStore((s) => s.items)
  const total = useAuditStore((s) => s.total)
  const loading = useAuditStore((s) => s.loading)
  const error = useAuditStore((s) => s.error)
  const load = useAuditStore((s) => s.load)

  /** Filters live in the URL (D-051); the text box is debounced by the hook. */
  const filters = useListQuery({
    defaults: { kind: '', q: '', range: '', page: 1 },
    debounced: ['q'],
  })
  const { kind, q, range } = filters.values
  const page = typeof filters.values.page === 'number' ? filters.values.page : 1
  const pageOffset = (page - 1) * PAGE_SIZE

  // Context viewer + export state (event-driven, never set inside an effect).
  const [contextOf, setContextOf] = useState<AuditEntry | null>(null)
  const [exporting, setExporting] = useState(false)

  useEffect(() => {
    void load({ kind, q, range: range as AuditFilters['range'] }, pageOffset)
  }, [load, kind, q, range, pageOffset])

  /**
   * Exports up to 200 rows matching the CURRENT filters — the report format an
   * incident write-up actually needs. The toast says how much was written
   * against how much matched, so a capped export is never mistaken for all of it.
   */
  const exportCsv = async () => {
    setExporting(true)
    try {
      const query: ApiAuditQuery = { kind: kind || undefined, q: q.trim() || undefined, limit: 200, offset: 0 }
      if (range) query.from = Math.floor(Date.now() / 1000) - AUDIT_RANGES[range as '24h' | '7d' | '30d']
      const exported = await api.audit(query)
      downloadAuditCsv(exported.entries)
      toast(
        'success',
        'Export downloaded',
        exported.entries.length < exported.total
          ? `${exported.entries.length} of ${exported.total} matching entries written (the read caps at 200) - narrow the filters for the rest.`
          : `${exported.total} matching entries written.`,
      )
    } catch (err) {
      toast('error', 'Could not export', err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setExporting(false)
    }
  }

  const COLUMNS: Column<AuditEntry>[] = [
    { key: 'at', header: 'Time', width: '175px', render: (r) => formatDateTime(r.at) },
    {
      key: 'actor',
      header: 'Actor',
      hideOnMobile: true,
      render: (r) =>
        r.actorName ? (
          <span className="ui-strong">{r.actorName}</span>
        ) : r.actorId ? (
          <span className="ui-strong">user #{r.actorId}</span>
        ) : (
          <span className="ui-muted">system</span>
        ),
    },
    {
      key: 'action',
      header: 'Action',
      render: (r) => (
        <span>
          <span className="ui-strong">{ACTION_LABELS[r.action] ?? r.action}</span>
          <span className="ui-muted t-sm2 audit-action-code">
            {r.action}
          </span>
        </span>
      ),
    },
    { key: 'target', header: 'Target', hideOnMobile: true, render: (r) => r.target || '—' },
    {
      key: 'result',
      header: 'Result',
      render: (r) => <Badge tone={RESULT_TONE[r.result] ?? 'warning'}>{r.result || 'UNKNOWN'}</Badge>,
    },
    {
      key: 'context',
      header: '',
      hideOnMobile: true,
      render: (r) =>
        prettyContext(r.context) ? (
          <Button size="sm" variant="ghost" onClick={() => setContextOf(r)}>
            Detail
          </Button>
        ) : (
          <span className="ui-muted t-sm2">no detail</span>
        ),
    },
  ]

  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="dash" data-testid="audit-page">
      <PageHeader
        title="Audit log"
        subtitle="Append-only — nothing here can be edited or removed, and filters run server-side"
        actions={
          <Button
            size="sm"
            variant="outline"
            leading={<Download size={14} />}
            disabled={exporting}
            onClick={() => void exportCsv()}
          >
            {exporting ? 'Exporting…' : 'Export CSV'}
          </Button>
        }
      />
      <Card
        title="Trail"
        actions={
          <span className="ui-muted t-sm">
            {items.length} of {total} entries
          </span>
        }
      >
        {/* The filter bar: one search box, the kind chips, the time window.
            Kind and range apply at once; the search debounces; `page` rides
            along so any filter change restarts at page 1. */}
        <div className="ui-row audit-filters-bar">
          <input
            className="ui-input audit-filters-bar__search"
            placeholder="Search action, actor, target…"
            aria-label="Search the audit trail"
            value={filters.draft.q}
            onChange={(e) => filters.set({ q: e.target.value })}
          />
          <div className="audit-filters" role="group" aria-label="Filter by kind">
            {KINDS.map((k) => (
              <button
                key={k.id}
                type="button"
                className="dash-range"
                data-on={kind === k.id}
                onClick={() => filters.set({ kind: k.id, page: 1 })}
              >
                {k.label}
              </button>
            ))}
          </div>
          <div className="audit-filters" role="group" aria-label="Time window">
            {RANGES.map((r) => (
              <button
                key={r.id}
                type="button"
                className="dash-range"
                data-on={range === r.id}
                onClick={() => filters.set({ range: r.id, page: 1 })}
              >
                {r.label}
              </button>
            ))}
          </div>
          {filters.isFiltered && (
            <Button size="sm" variant="ghost" onClick={() => filters.reset()}>
              Clear filters
            </Button>
          )}
        </div>
        <AsyncState
          status={error ? 'error' : loading && items.length === 0 ? 'loading' : items.length === 0 ? 'empty' : 'ready'}
          error={error}
          onRetry={() => void load({ kind, q, range: range as AuditFilters['range'] }, pageOffset)}
          skeleton={<Skeleton height={180} />}
          empty={
            filters.isFiltered ? (
              <EmptyState
                icon={<ScrollText size={22} />}
                title="Nothing matches these filters"
                description="The trail holds entries, but none within this kind, window and search combined — the count above is the server's own answer."
                action={
                  <Button variant="outline" size="sm" onClick={() => filters.reset()}>
                    Clear filters
                  </Button>
                }
              />
            ) : (
              <EmptyState
                icon={<ScrollText size={22} />}
                title="Nothing recorded yet"
                description="Every sign-in, VPN change and payment decision is written here as it happens."
              />
            )
          }
        >
          <Table columns={COLUMNS} data={items} rowKey={(r) => r.id} />
          {total > PAGE_SIZE && (
            <Pagination page={page} pageCount={pageCount} onChange={(p) => filters.setNow({ page: p })} />
          )}
        </AsyncState>
      </Card>

      <Dialog
        open={contextOf !== null}
        onClose={() => setContextOf(null)}
        title={`Entry #${contextOf?.id ?? ''}`}
        footer={
          <Button variant="ghost" onClick={() => setContextOf(null)}>
            Close
          </Button>
        }
      >
        {contextOf && (
          <div className="ui-col audit-context-wrap">
            <span className="ui-muted t-sm">
              {ACTION_LABELS[contextOf.action] ?? contextOf.action} ·{' '}
              {contextOf.actorName || (contextOf.actorId ? `user #${contextOf.actorId}` : 'system')} ·{' '}
              {formatDateTime(contextOf.at)}
            </span>
            <pre className="audit-context">
              {prettyContext(contextOf.context) || 'No detail was recorded for this entry.'}
            </pre>
          </div>
        )}
      </Dialog>
    </div>
  )
}