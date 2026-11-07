import { useEffect, useState, type FormEvent } from 'react'
import {
  AsyncState,
  Button,
  Card,
  Dialog,
  EmptyState,
  Input,
  PageHeader,
  Skeleton,
  Switch,
  Table,
  useToast,
  type Column,
} from '../components/ui'
import { Package, Pencil, Plus } from 'lucide-react'
import { useOffersStore, type VpnOffer } from '../stores/offersStore'
import { useServersStore } from '../stores/serversStore'
import { api, type ApiInbound } from '../lib/api'
import './offers.css'

/**
 * Offer catalogue — real API (`/api/offers`, P3).
 *
 * Phase A changes, forced by what the backend can store:
 *  - `kind` (FREE / PREMIUM) is now a field: the backend requires it and it
 *    decides who may claim the offer;
 *  - the inbound is chosen from the panel (`GET /api/servers/{id}/inbounds`)
 *    instead of being free text, because the backend stores `inbound_id`;
 *  - the "Claims" column is gone — nothing counts redemptions yet.
 */

interface FormData {
  kind: 'FREE' | 'PREMIUM'
  name: string
  serverId: string
  inboundId: string
  traffic: string
  expiry: string
  devices: string
  active: boolean
}

const emptyForm = (serverId: string): FormData => ({
  kind: 'FREE',
  name: '',
  serverId,
  inboundId: '',
  traffic: '10',
  expiry: '30',
  devices: '2',
  active: true,
})

export function OffersPage() {
  const offers = useOffersStore((s) => s.offers)
  const loading = useOffersStore((s) => s.loading)
  const error = useOffersStore((s) => s.error)
  const busy = useOffersStore((s) => s.busy)
  const load = useOffersStore((s) => s.load)
  const addOffer = useOffersStore((s) => s.addOffer)
  const updateOffer = useOffersStore((s) => s.updateOffer)
  const toggleActive = useOffersStore((s) => s.toggleActive)

  const servers = useServersStore((s) => s.servers)
  const loadServers = useServersStore((s) => s.load)

  const toast = useToast()

  const [formOpen, setFormOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<VpnOffer | null>(null)
  const [form, setForm] = useState<FormData>(emptyForm(''))
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [inbounds, setInbounds] = useState<ApiInbound[]>([])

  useEffect(() => {
    void load()
    void loadServers()
  }, [load, loadServers])

  const serverName = (id: number) => servers.find((s) => s.apiId === id)?.name ?? `server ${id}`

  const openCreate = () => {
    const first = servers[0]?.apiId
    setEditTarget(null)
    setForm(emptyForm(first ? String(first) : ''))
    setFormError('')
    setFormOpen(true)
  }

  const openEdit = (o: VpnOffer) => {
    setEditTarget(o)
    setForm({
      kind: o.kind,
      name: o.name,
      serverId: String(o.serverId),
      inboundId: o.inboundId ? String(o.inboundId) : '',
      traffic: String(o.trafficLimitGB),
      expiry: String(o.expiryDays),
      devices: String(o.deviceLimit),
      active: o.active,
    })
    setFormError('')
    setFormOpen(true)
  }

  // Load the selected server's inbounds so the operator picks a real one.
  // Only the async path calls setState; the "nothing to load" case is derived
  // during render below, which avoids a synchronous setState inside an effect.
  const inboundServerId = formOpen ? form.serverId : ''

  useEffect(() => {
    if (!inboundServerId) return
    let cancelled = false
    api
      .serverInbounds(Number(inboundServerId))
      .then((rows) => {
        if (!cancelled) setInbounds(rows)
      })
      .catch(() => {
        if (!cancelled) setInbounds([])
      })
    return () => {
      cancelled = true
    }
  }, [inboundServerId])

  // A stale list from a previously selected server must not be offered.
  const visibleInbounds = inboundServerId ? inbounds : []

  const saveForm = async (e: FormEvent) => {
    e.preventDefault()
    setFormError('')
    if (!form.name.trim()) {
      setFormError('Offer name is required.')
      return
    }
    if (!form.serverId) {
      setFormError('Pick a server — an offer without one cannot be provisioned.')
      return
    }
    setSaving(true)
    try {
      if (editTarget) {
        await updateOffer(editTarget.apiId, {
          name: form.name.trim(),
          trafficLimitGB: Number.parseInt(form.traffic, 10) || 0,
          expiryDays: Number.parseInt(form.expiry, 10) || 0,
          deviceLimit: Number.parseInt(form.devices, 10) || 0,
        })
        // `active` is a separate call so a failed limit edit cannot flip the switch.
        if (editTarget.active !== form.active) await toggleActive(editTarget.apiId)
        toast('success', 'Offer updated', form.name.trim())
      } else {
        await addOffer({
          kind: form.kind,
          name: form.name.trim(),
          serverId: Number(form.serverId),
          inboundId: form.inboundId ? Number(form.inboundId) : undefined,
          trafficLimitGB: Number.parseInt(form.traffic, 10) || 0,
          expiryDays: Number.parseInt(form.expiry, 10) || 0,
          deviceLimit: Number.parseInt(form.devices, 10) || 0,
          active: form.active,
        })
        toast('success', 'Offer created', `${form.name.trim()} — claimable while active`)
      }
      setFormOpen(false)
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Could not save the offer')
    } finally {
      setSaving(false)
    }
  }

  const COLUMNS: Column<VpnOffer>[] = [
    {
      key: 'name',
      header: 'Offer',
      sortable: true,
      render: (o) => (
        <span>
          <span className="ui-strong">{o.name}</span>
          <span className="of-inbound">{o.kind}</span>
        </span>
      ),
    },
    { key: 'serverId', header: 'Server', sortable: true, render: (o) => serverName(o.serverId) },
    {
      key: 'trafficLimitGB',
      header: 'Traffic',
      sortable: true,
      align: 'right',
      render: (o) => (o.trafficLimitGB > 0 ? `${o.trafficLimitGB} GB` : 'unlimited'),
    },
    {
      key: 'expiryDays',
      header: 'Expiry',
      sortable: true,
      align: 'right',
      render: (o) => (o.expiryDays > 0 ? `${o.expiryDays}d` : 'never'),
    },
    {
      key: 'deviceLimit',
      header: 'Devices',
      align: 'right',
      render: (o) => (o.deviceLimit > 0 ? String(o.deviceLimit) : '∞'),
    },
    {
      key: 'active',
      header: 'Active',
      render: (o) => (
        <Switch
          checked={o.active}
          onChange={() => {
            void toggleActive(o.apiId).catch(() =>
              toast('error', 'Could not change the offer', 'Nothing was saved'),
            )
          }}
          label=""
        />
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (o) => (
        <Button
          size="sm"
          variant="ghost"
          disabled={busy === o.apiId}
          aria-label={`Edit ${o.name}`}
          onClick={() => openEdit(o)}
        >
          <Pencil size={14} />
        </Button>
      ),
    },
  ]

  return (
    <div className="dash" data-testid="offers-page">
      <PageHeader
        title="VPN offers"
        subtitle="Each claim creates its own private credential"
        actions={
          <Button size="sm" leading={<Plus size={14} />} onClick={openCreate}>
            New offer
          </Button>
        }
      />

      <Card
        title="Offers"
        actions={<span className="ui-muted t-sm">{offers.filter((o) => o.active).length} active</span>}
      >
        <AsyncState
          status={error ? 'error' : loading ? 'loading' : offers.length === 0 ? 'empty' : 'ready'}
          error={error}
          onRetry={() => void load()}
          skeleton={<Skeleton height={160} />}
          empty={
            <EmptyState
              icon={<Package size={22} />}
              title="No plans in the catalogue yet"
              description="A VPN cannot be provisioned without an offer. Create the first plan to start selling."
              action={
                <Button size="sm" variant="primary" onClick={() => setFormOpen(true)}>
                  New offer
                </Button>
              }
            />
          }
        >
          <Table columns={COLUMNS} data={offers} rowKey={(r) => r.id} initialSort={{ key: 'name', dir: 'asc' }} />
        </AsyncState>
      </Card>

      <Dialog
        open={formOpen}
        onClose={() => setFormOpen(false)}
        title={editTarget ? 'Edit offer' : 'New offer'}
        footer={
          <>
            <Button variant="ghost" onClick={() => setFormOpen(false)}>Cancel</Button>
            <Button type="submit" form="of-form" disabled={saving}>
              {saving ? 'Saving…' : editTarget ? 'Save changes' : 'Create offer'}
            </Button>
          </>
        }
      >
        <form id="of-form" onSubmit={saveForm} className="ui-col" noValidate>
          <label className="ui-field">
            <span className="ui-field__label">Kind</span>
            <select
              className="ui-input"
              value={form.kind}
              disabled={!!editTarget}
              onChange={(e) => setForm((f) => ({ ...f, kind: e.target.value as 'FREE' | 'PREMIUM' }))}
            >
              <option value="FREE">FREE — public catalogue</option>
              <option value="PREMIUM">PREMIUM — sold by a reseller</option>
            </select>
          </label>
          <Input
            label="Offer name"
            placeholder="Starter — Frankfurt"
            value={form.name}
            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          />
          <label className="ui-field">
            <span className="ui-field__label">Server</span>
            <select
              className="ui-input"
              value={form.serverId}
              disabled={!!editTarget}
              onChange={(e) => setForm((f) => ({ ...f, serverId: e.target.value, inboundId: '' }))}
            >
              <option value="">Select a server…</option>
              {servers.map((s) => (
                <option key={s.apiId} value={s.apiId}>
                  {s.name}
                  {s.status !== 'ACTIVE' ? ` (${s.status.toLowerCase().replaceAll('_', ' ')})` : ''}
                </option>
              ))}
            </select>
          </label>
          <label className="ui-field">
            <span className="ui-field__label">Inbound</span>
            <select
              className="ui-input"
              value={form.inboundId}
              disabled={!!editTarget || visibleInbounds.length === 0}
              onChange={(e) => setForm((f) => ({ ...f, inboundId: e.target.value }))}
            >
              <option value="">
                {visibleInbounds.length === 0 ? 'No inbounds cached for this server' : 'Let the panel decide…'}
              </option>
              {visibleInbounds.map((ib) => (
                <option key={ib.id} value={ib.id}>
                  {ib.remark || ib.tag || `#${ib.id}`} — {ib.protocol}:{ib.port}
                </option>
              ))}
            </select>
          </label>
          <Input
            label="Traffic limit (GB, 0 = unlimited)"
            type="number"
            min={0}
            value={form.traffic}
            onChange={(e) => setForm((f) => ({ ...f, traffic: e.target.value }))}
          />
          <Input
            label="Expiry (days, 0 = never)"
            type="number"
            min={0}
            value={form.expiry}
            onChange={(e) => setForm((f) => ({ ...f, expiry: e.target.value }))}
          />
          <Input
            label="Device limit (0 = unlimited)"
            type="number"
            min={0}
            value={form.devices}
            onChange={(e) => setForm((f) => ({ ...f, devices: e.target.value }))}
          />
          <Switch
            checked={form.active}
            onChange={(v) => setForm((f) => ({ ...f, active: v }))}
            label="Active (visible to clients)"
          />
          {formError && (
            <p className="ui-field__error" role="alert">
              {formError}
            </p>
          )}
        </form>
      </Dialog>
    </div>
  )
}