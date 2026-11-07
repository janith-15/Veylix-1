import { useEffect, useMemo, useState } from 'react'
import {
  AsyncState,
  Button,
  Card,
  Dialog,
  EmptyState,
  Input,
  PageHeader,
  Table,
  useToast,
  type Column,
} from '../components/ui'
import { Archive, Plus, Trash2 } from 'lucide-react'
import { ApiError, type ApiPlan, type ApiPlanKind } from '../lib/api'
import { usePackagesStore } from '../stores/packagesStore'
import { useServersStore } from '../stores/serversStore'
import { useAuthStore } from '../stores/authStore'
import './packages.css'

/**
 * L5 — the package catalogue (D-035).
 *
 * Three kinds, and the rules differ (D-035.2):
 *   FREE           — a public offer; a client claims it instantly.
 *   PREMIUM        — a fixed template owned by the Super Admin or a reseller.
 *   CUSTOM PREMIUM — "state your own spec". Super Admin only; its price is
 *                    computed per order from the rate card, never stored here.
 *
 * Deleting a package destroys the ability to explain past sales, so Archive is
 * the primary action; Delete is offered only to the Super Admin, explicitly.
 */

function formatMoney(cents: number, currency: string): string {
  return `${currency} ${(cents / 100).toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`
}

/** Major unit ("12.50") → integer cents. `null` = not a valid amount. */
function toCents(text: string): number | null {
  const trimmed = text.trim()
  if (trimmed === '') return null
  if (!/^\d+(\.\d{1,2})?$/.test(trimmed)) return null
  const [whole, frac = ''] = trimmed.split('.')
  return Number(whole) * 100 + Number((frac + '00').slice(0, 2))
}

interface FormState {
  kind: ApiPlanKind
  name: string
  serverId: string
  inboundId: string
  trafficGb: string
  durationDays: string
  deviceLimit: string
  price: string
  currency: string
  slotsTotal: string
  isCustom: boolean
}

const EMPTY_FORM: FormState = {
  kind: 'PREMIUM',
  name: '',
  serverId: '',
  inboundId: '',
  trafficGb: '',
  durationDays: '',
  deviceLimit: '',
  price: '',
  currency: 'LKR',
  slotsTotal: '',
  isCustom: false,
}

export function PackagesPage() {
  const { plans, packages, loading, saving, busy, error, load, create, archive, remove } = usePackagesStore()
  const {
    servers,
    load: loadServers,
    inbounds: inboundsByServer,
    inboundsLoading,
    loadInbounds,
  } = useServersStore()
  const role = useAuthStore((s) => s.user?.role)
  const isSuperAdmin = role === 'super-admin'
  const toast = useToast()

  const [formOpen, setFormOpen] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [formError, setFormError] = useState('')
  const [archiveTarget, setArchiveTarget] = useState<ApiPlan | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ApiPlan | null>(null)

  useEffect(() => {
    void load()
    void loadServers()
  }, [load, loadServers])

  /* Inbounds are per server; they are cached in the store, read once each. */
  useEffect(() => {
    const serverId = Number(form.serverId)
    if (!Number.isFinite(serverId) || serverId <= 0) return
    if (!inboundsByServer[serverId]) void loadInbounds(serverId)
  }, [form.serverId, inboundsByServer, loadInbounds])

  const inbounds = useMemo(() => {
    const serverId = Number(form.serverId)
    if (!Number.isFinite(serverId) || serverId <= 0) return []
    return inboundsByServer[serverId] ?? []
  }, [form.serverId, inboundsByServer])

  const inboundsBusy = inboundsLoading === Number(form.serverId)

  /** The buyer-facing list, for the "in catalogue" column. */
  const purchasableIds = useMemo(() => new Set(packages.map((p) => p.id)), [packages])

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }))
    setFormError('')
  }

  const openForm = () => {
    setForm({ ...EMPTY_FORM })
    setFormError('')
    setFormOpen(true)
  }

  const submit = async () => {
    if (!form.name.trim()) {
      setFormError('A package needs a name.')
      return
    }
    const serverId = Number(form.serverId)
    const inboundId = Number(form.inboundId)
    if (!Number.isFinite(serverId) || serverId <= 0) {
      setFormError('Choose the server this package sells from.')
      return
    }
    if (!Number.isFinite(inboundId) || inboundId <= 0) {
      setFormError('Choose the inbound on that server.')
      return
    }

    // A CUSTOM template is priced per order (`POST /api/orders/quote`), so a
    // stored price there would be a second, contradicting source of truth.
    const priceCents = form.isCustom ? 0 : toCents(form.price)
    if (!form.isCustom && priceCents === null) {
      setFormError('Enter a price with at most two decimals, or make it a CUSTOM package.')
      return
    }

    const intField = (text: string, label: string): number | null => {
      const n = Number(text.trim() === '' ? '0' : text)
      if (!Number.isInteger(n) || n < 0) {
        setFormError(`${label}: enter a whole number of 0 or more.`)
        return null
      }
      return n
    }

    const trafficGb = intField(form.trafficGb, 'Traffic')
    if (trafficGb === null) return
    const durationDays = intField(form.durationDays, 'Duration')
    if (durationDays === null) return
    const deviceLimit = intField(form.deviceLimit, 'Devices')
    if (deviceLimit === null) return
    const slotsTotal = intField(form.slotsTotal, 'Slots')
    if (slotsTotal === null) return

    try {
      await create({
        kind: form.kind,
        name: form.name.trim(),
        serverId,
        inboundId,
        trafficGb,
        durationDays,
        deviceLimit,
        priceCents: priceCents ?? 0,
        currency: form.currency.trim().toUpperCase() || 'LKR',
        slotsTotal,
        isCustom: form.isCustom,
      })
      setFormOpen(false)
      toast('success', 'Package created', `${form.name.trim()} is in the catalogue.`)
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : 'Could not create the package.')
    }
  }

  const COLUMNS: Column<ApiPlan>[] = [
    {
      key: 'name',
      header: 'Package',
      sortable: true,
      render: (p) => (
        <span>
          <strong>{p.name}</strong>
          <span className="pkg-sub">{p.server_label || 'no server'}</span>
        </span>
      ),
    },
    {
      key: 'kind',
      header: 'Kind',
      sortable: true,
      render: (p) => (
        <span className={`pkg-kind pkg-kind--${p.kind.toLowerCase().replace('_', '-')}`}>
          {p.kind.replace('_', ' ')}
        </span>
      ),
    },
    {
      key: 'traffic',
      header: 'Traffic',
      align: 'right',
      sortable: true,
      render: (p) => (p.traffic_gb > 0 ? `${p.traffic_gb} GB` : 'unlimited'),
    },
    {
      key: 'duration',
      header: 'Duration',
      align: 'right',
      hideOnMobile: true,
      render: (p) => (p.duration_days > 0 ? `${p.duration_days} d` : 'no expiry'),
    },
    {
      key: 'devices',
      header: 'Devices',
      align: 'right',
      hideOnMobile: true,
      render: (p) => (p.device_limit > 0 ? String(p.device_limit) : '—'),
    },
    {
      key: 'price',
      header: 'Price',
      align: 'right',
      sortable: true,
      render: (p) =>
        p.kind === 'FREE' ? (
          <span className="ui-muted t-sm2">free</span>
        ) : p.price_cents > 0 ? (
          <span className="pkg-price">{formatMoney(p.price_cents, p.currency)}</span>
        ) : (
          // A CUSTOM template has no stored price; saying so is the honest cell.
          <span className="ui-muted t-sm2">priced per order</span>
        ),
    },
    {
      key: 'slots',
      header: 'Slots left',
      align: 'right',
      hideOnMobile: true,
      render: (p) => (p.slots_remaining === null ? '∞' : String(p.slots_remaining)),
    },
    {
      key: 'purchasable',
      header: 'In catalogue',
      align: 'center',
      render: (p) =>
        purchasableIds.has(p.id) ? (
          <span className="pkg-on">yes</span>
        ) : (
          <span className="ui-muted t-sm2">no</span>
        ),
    },
    {
      key: 'actions',
      header: '',
      align: 'right',
      sticky: true,
      render: (p) => (
        <div className="pkg-actions">
          <Button
            size="sm"
            variant="ghost"
            leading={<Archive size={14} />}
            onClick={() => setArchiveTarget(p)}
            disabled={busy === p.id}
            title={`Archive ${p.name} — keeps its sales history`}
          >
            Archive
          </Button>
          {isSuperAdmin && (
            <Button
              size="sm"
              variant="ghost"
              className="pkg-danger"
              leading={<Trash2 size={14} />}
              onClick={() => setDeleteTarget(p)}
              disabled={busy === p.id}
              title={`Delete ${p.name}`}
            >
              Delete
            </Button>
          )}
        </div>
      ),
    },
  ]

  return (
    <div className="pkg-page">
      <PageHeader
        title="Packages"
        subtitle="What the platform sells. FREE is claimable, PREMIUM is a fixed template, and CUSTOM lets a customer state a spec priced from the rate card."
        actions={
          <Button leading={<Plus size={14} />} onClick={openForm}>
            New package
          </Button>
        }
      />

      {error && (
        <Card className="pkg-notice">
          <p className="t-sm2">{error}</p>
        </Card>
      )}

      <AsyncState status={loading ? 'loading' : plans.length === 0 ? 'empty' : 'ready'}>
        <Card>
          {plans.length === 0 ? (
            <EmptyState
              title="No packages yet"
              description="A client cannot buy or claim anything until at least one package exists. Create a FREE offer first — it is the one a client can claim instantly."
              action={
                <Button leading={<Plus size={14} />} onClick={openForm}>
                  New package
                </Button>
              }
            />
          ) : (
            <Table
              columns={COLUMNS}
              data={plans}
              rowKey={(p) => String(p.id)}
              initialSort={{ key: 'name', dir: 'asc' }}
            />
          )}
        </Card>
      </AsyncState>

      <Dialog
        open={formOpen}
        onClose={() => setFormOpen(false)}
        title="New package"
        wide
        footer={
          <>
            <Button variant="ghost" onClick={() => setFormOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => void submit()} disabled={saving}>
              {saving ? 'Creating…' : 'Create package'}
            </Button>
          </>
        }
      >
        <div className="pkg-fields">
          <label className="ui-field">
            <span className="ui-field__label">Kind</span>
            <select
              className="ui-input"
              value={form.kind}
              onChange={(e) => {
                const kind = e.target.value as ApiPlanKind
                // CUSTOM is a Super-Admin-only template whose price is computed
                // per order, so the two fields follow the kind automatically.
                setForm((f) => ({ ...f, kind, isCustom: kind === 'CUSTOM_PREMIUM' }))
                setFormError('')
              }}
            >
              <option value="FREE">FREE — claimable instantly</option>
              <option value="PREMIUM">PREMIUM — fixed template</option>
              {isSuperAdmin && (
                <option value="CUSTOM_PREMIUM">CUSTOM PREMIUM — customer states a spec</option>
              )}
            </select>
          </label>

          <Input
            label="Name"
            placeholder="e.g. 50 GB / 30 days"
            value={form.name}
            onChange={(e) => set('name', e.target.value)}
          />

          <label className="ui-field">
            <span className="ui-field__label">Server</span>
            <select
              className="ui-input"
              value={form.serverId}
              onChange={(e) => {
                setForm((f) => ({ ...f, serverId: e.target.value, inboundId: '' }))
                setFormError('')
              }}
            >
              <option value="">Choose a server…</option>
              {servers.map((s) => (
                <option key={s.id} value={s.apiId}>
                  {s.name}
                </option>
              ))}
            </select>
          </label>

          <label className="ui-field">
            <span className="ui-field__label">Inbound</span>
            <select
              className="ui-input"
              value={form.inboundId}
              onChange={(e) => set('inboundId', e.target.value)}
              disabled={!form.serverId || inboundsBusy}
            >
              <option value="">
                {!form.serverId
                  ? 'Pick a server first'
                  : inboundsBusy
                    ? 'Reading inbounds…'
                    : inbounds.length === 0
                      ? 'No inbounds on this server'
                      : 'Choose an inbound…'}
              </option>
              {inbounds.map((i) => (
                <option key={i.id} value={i.id}>
                  {i.remark || `inbound #${i.id}`} · {i.protocol}
                </option>
              ))}
            </select>
          </label>

          <Input
            label="Traffic (GB)"
            hint="0 means unlimited"
            inputMode="numeric"
            placeholder="0"
            value={form.trafficGb}
            onChange={(e) => set('trafficGb', e.target.value)}
          />
          <Input
            label="Duration (days)"
            hint="0 means no expiry"
            inputMode="numeric"
            placeholder="0"
            value={form.durationDays}
            onChange={(e) => set('durationDays', e.target.value)}
          />
          <Input
            label="Device limit"
            hint="0 means no limit"
            inputMode="numeric"
            placeholder="0"
            value={form.deviceLimit}
            onChange={(e) => set('deviceLimit', e.target.value)}
          />
          <Input
            label="Slots"
            hint="How many may be sold; 0 means no cap"
            inputMode="numeric"
            placeholder="0"
            value={form.slotsTotal}
            onChange={(e) => set('slotsTotal', e.target.value)}
          />

          {!form.isCustom && (
            <>
              <Input
                label="Price"
                hint="The seller's price, not the buyer's"
                inputMode="decimal"
                placeholder="0.00"
                value={form.price}
                onChange={(e) => set('price', e.target.value)}
              />
              <Input
                label="Currency"
                hint="Three-letter code"
                value={form.currency}
                onChange={(e) => set('currency', e.target.value)}
                maxLength={3}
              />
            </>
          )}
        </div>

        {form.isCustom && (
          <p className="ui-muted t-sm2">
            A CUSTOM package has no stored price. Every order is priced by the rate card
            (<code>/api/orders/quote</code>), so a customer is never quoted from a stale number.
          </p>
        )}

        {formError && (
          <p className="pkg-error" role="alert">
            {formError}
          </p>
        )}
      </Dialog>

      <Dialog
        open={archiveTarget !== null}
        onClose={() => setArchiveTarget(null)}
        title="Archive this package?"
        footer={
          <>
            <Button variant="ghost" onClick={() => setArchiveTarget(null)}>
              Cancel
            </Button>
            <Button
              onClick={async () => {
                if (!archiveTarget) return
                const name = archiveTarget.name
                try {
                  await archive(archiveTarget.id)
                  toast('success', 'Package archived', `${name} is out of the catalogue; its sales history is kept.`)
                } catch {
                  toast('error', 'Could not archive', 'The catalogue now shows the server record.')
                }
                setArchiveTarget(null)
              }}
            >
              Archive
            </Button>
          </>
        }
      >
        <p>
          <strong>{archiveTarget?.name}</strong> stops being offered. Clients who already hold a VPN from
          it keep it — archiving changes the catalogue, never a customer&apos;s access.
        </p>
      </Dialog>

      <Dialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        title="Delete this package permanently?"
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleteTarget(null)}>
              Cancel
            </Button>
            <Button
              variant="ghost"
              className="pkg-danger"
              onClick={async () => {
                if (!deleteTarget) return
                const name = deleteTarget.name
                try {
                  await remove(deleteTarget.id)
                  toast('success', 'Package deleted', name)
                } catch {
                  toast('error', 'Could not delete', 'The server refused — archive it instead.')
                }
                setDeleteTarget(null)
              }}
            >
              Delete permanently
            </Button>
          </>
        }
      >
        <p>
          <strong>{deleteTarget?.name}</strong> will be removed for good. Use <em>Archive</em> unless you
          are certain nothing was ever sold from it.
        </p>
      </Dialog>
    </div>
  )
}
