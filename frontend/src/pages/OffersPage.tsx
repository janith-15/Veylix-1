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
import { Archive, Package, Pencil, Plus, Trash2 } from 'lucide-react'
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
  const archiveOfferAction = useOffersStore((s) => s.archiveOffer)
  const deleteOfferAction = useOffersStore((s) => s.deleteOffer)

  const servers = useServersStore((s) => s.servers)
  const loadServers = useServersStore((s) => s.load)

  const toast = useToast()

  const [formOpen, setFormOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<VpnOffer | null>(null)
  const [form, setForm] = useState<FormData>(emptyForm(''))
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [inbounds, setInbounds] = useState<ApiInbound[]>([])
  /** Offer awaiting delete confirmation. Type-to-confirm is overkill here (an
   * offer is not a customer's property), but a one-click destroy is not acceptable
   * either: the dialog names the offer and what will happen to it. */
  const [deleteTarget, setDeleteTarget] = useState<VpnOffer | null>(null)

  /**
   * Archive an offer (P-4). Says what actually happened, including the one case
   * that matters: the row is KEPT, so anything that already references it still
   * works.
   */
  const archiveOffer = async (o: VpnOffer) => {
    try {
      await archiveOfferAction(o.apiId)
      toast('success', 'Offer archived', `${o.name} is out of the catalogue. Nothing was deleted.`)
    } catch {
      toast('error', 'Could not archive the offer', 'Nothing was changed')
    }
  }

  /**
   * Delete an offer nothing depends on (P-4).
   *
   * A 409 `offer_in_use` is the interesting outcome, not an error to hide: the
   * offer has been sold or used by a VPN, so it must stay. The message names the
   * remedy (archive it) instead of leaving the operator with a red toast.
   */
  const deleteOffer = async (o: VpnOffer) => {
    try {
      await deleteOfferAction(o.apiId)
      toast('success', 'Offer deleted', `${o.name} was removed.`)
    } catch (err) {
      const inUse = err instanceof Error && /in use|cannot be deleted/i.test(err.message)
      if (inUse) {
        toast('warning', 'This offer has been used', `Archive ${o.name} instead — a sold offer must keep its record.`)
      } else {
        toast('error', 'Could not delete the offer', 'Nothing was changed')
      }
    } finally {
      setDeleteTarget(null)
    }
  }

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

  /*
   * Load the selected server's inbounds so the operator picks a real one.
   *
   * The failure reason is kept, not swallowed: "the panel is not connected",
   * "the panel needs re-authenticating" and "this panel genuinely has no inbound"
   * look identical in an empty dropdown, and the operator cannot fix a problem
   * nobody named (D-075). A swallowed error here is what made offer creation look
   * broken with no explanation.
   *
   * Only the async callbacks set state. "Still loading" is DERIVED during render
   * from the server whose answer has arrived, which is why no setState runs
   * synchronously in this effect.
   */
  const [inboundsFor, setInboundsFor] = useState<string>('')
  const [inboundError, setInboundError] = useState<string | null>(null)
  const inboundServerId = formOpen ? form.serverId : ''
  const inboundLoading = Boolean(inboundServerId) && inboundsFor !== inboundServerId

  useEffect(() => {
    if (!inboundServerId) return
    let cancelled = false
    api
      .serverInbounds(Number(inboundServerId))
      .then((rows) => {
        if (cancelled) return
        setInbounds(rows)
        setInboundError(null)
        setInboundsFor(inboundServerId)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setInbounds([])
        setInboundError(
          err instanceof Error && err.message ? err.message : 'the panel could not be read',
        )
        setInboundsFor(inboundServerId)
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
    // The inbound is what the panel client is created inside, so an offer
    // without one can never build a VPN. The backend already refuses it with a
    // 422 `offer_incomplete`; refusing here too means the operator is told at
    // the field, in the product's own words, instead of reading a backend error
    // about `inbound_id` after the fact (D-075, D-059).
    if (!editTarget && !(Number(form.inboundId) > 0)) {
      setFormError('Choose the inbound this offer creates its clients in.')
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
        <span className="ui-row of-row-actions">
          <Button
            size="sm"
            variant="ghost"
            disabled={busy === o.apiId}
            aria-label={`Edit ${o.name}`}
            onClick={() => openEdit(o)}
          >
            <Pencil size={14} />
          </Button>
          {/*
            P-4: retiring an offer used to mean flipping the Active switch, which
            hid it but recorded no reason and left it in this list forever labelled
            "inactive". Archive records when it left the catalogue.
          */}
          <Button
            size="sm"
            variant="ghost"
            disabled={busy === o.apiId || o.archived}
            aria-label={`Archive ${o.name}`}
            title="Take this offer out of the catalogue. The row is kept."
            onClick={() => void archiveOffer(o)}
          >
            <Archive size={14} />
          </Button>
          {/*
            Delete is offered only as a real option: the API refuses it (409
            offer_in_use) for anything a VPN or a payment names, and the toast says
            so with the remedy. A hidden button would leave the operator guessing.
          */}
          <Button
            size="sm"
            variant="ghost"
            disabled={busy === o.apiId}
            aria-label={`Delete ${o.name}`}
            title="Delete an offer nothing has used yet"
            onClick={() => setDeleteTarget(o)}
          >
            <Trash2 size={14} />
          </Button>
        </span>
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
              {/*
                There is deliberately no "let the panel decide" option. Veylix
                cannot choose an inbound: the outbound client is created *in* a
                named inbound, so an offer with none can never build a VPN and
                the backend refuses it (D-075, 422 offer_incomplete). Offering
                that label promised a choice the system does not implement — the
                operator picked the default, and the only feedback was a backend
                error naming a field they never saw. Now the empty value is a
                prompt to choose, and `saveForm` refuses locally with the same
                sentence the rule carries.
              */}
              <option value="">
                {inboundLoading
                  ? 'Reading the panel…'
                  : visibleInbounds.length === 0
                    ? 'No inbound available — see the note below'
                    : 'Choose an inbound…'}
              </option>
              {visibleInbounds.map((ib) => (
                <option key={ib.id} value={ib.id}>
                  {ib.remark || ib.tag || `#${ib.id}`} — {ib.protocol}:{ib.port}
                </option>
              ))}
            </select>
          </label>
          {/*
            An empty inbound list is the single most common reason an offer cannot
            be saved, and the form used to say nothing about it. Each state gets a
            sentence that names the cause and the next step (D-059: explained, not
            just coloured).
          */}
          {inboundServerId && !inboundLoading && inboundError && (
            <p className="of-note of-note--warn" role="alert">
              This server's panel could not be read: {inboundError}. Reconnect it
              under Servers, then reopen this form.
            </p>
          )}
          {inboundServerId &&
            !inboundLoading &&
            !inboundError &&
            visibleInbounds.length === 0 && (
              <p className="of-note of-note--warn" role="alert">
                This panel has no inbound yet, and Veylix never creates one (D-024). Open
                3X-UI, add an inbound on this server, sync it, then reopen this form — an
                offer without an inbound cannot build a VPN and will be refused.
              </p>
            )}
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

      {/*
        Delete confirmation (P-4). Names the offer and states the consequence in
        the customer's terms — "permanently", and why it might be refused — rather
        than "Are you sure?". NN/g: a confirmation must be specific about what will
        happen, and must not be shown for everything.
      */}
      <Dialog
        open={deleteTarget !== null}
        title={deleteTarget ? `Delete “${deleteTarget.name}”?` : 'Delete offer'}
        onClose={() => setDeleteTarget(null)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setDeleteTarget(null)}>
              Cancel
            </Button>
            <Button
              variant="danger"
              disabled={busy !== null}
              onClick={() => deleteTarget && void deleteOffer(deleteTarget)}
            >
              Delete this offer
            </Button>
          </>
        }
      >
        <p className="ui-muted">
          This permanently removes the offer. It will be <strong>refused</strong> if a VPN or a
          payment already names it — an offer that has been sold must keep its record, so use{' '}
          <strong>Archive</strong> for those: archiving takes it out of the catalogue and keeps the
          row.
        </p>
      </Dialog>
    </div>
  )
}