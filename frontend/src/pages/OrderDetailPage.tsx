import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Button, Card, PageHeader, Skeleton, useToast } from '../components/ui'
import { ArrowLeft, Check, Server as ServerIcon } from 'lucide-react'
import { ApiError } from '../lib/api'
import { useOrdersStore } from '../stores/ordersStore'
import { useServersStore } from '../stores/serversStore'
import './order-detail.css'

/**
 * L3 — releasing a CUSTOM order (D-036.6).
 *
 * A CUSTOM PREMIUM order names a spec, not a server, so somebody has to decide
 * *where* it is built. That is the Super Admin's job and it happens here:
 * pick a server, pick an inbound on it, then release.
 *
 * The release target is recorded on the order's timeline before the release,
 * so the decision is attributable even if the release later fails.
 *
 * The order and its timeline live in the store rather than in page state,
 * matching every other screen in the app.
 */
export function OrderDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const toast = useToast()
  const { servers, load: loadServers, inbounds: inboundsByServer, inboundsLoading, loadInbounds } =
    useServersStore()
  const {
    order,
    orderLoading,
    orderError,
    loadOrder,
    resetOrder,
    setReleaseTarget,
    busy,
    loadEvents,
    eventsFor,
  } = useOrdersStore()

  const orderId = Number(id)

  const [serverId, setServerId] = useState('')
  const [inboundId, setInboundId] = useState('')

  useEffect(() => {
    void loadServers()
  }, [loadServers])

  /* The order and its history, read once per id. */
  useEffect(() => {
    if (!Number.isFinite(orderId) || orderId <= 0) return
    void loadOrder(orderId)
    void loadEvents(orderId)
  }, [orderId, loadOrder, loadEvents])

  // Leaving the page drops the order, so a later visit never flashes the old one.
  useEffect(() => resetOrder, [resetOrder])

  /* Inbounds belong to a server, so they are re-read on every change. */
  useEffect(() => {
    const sid = Number(serverId)
    if (!Number.isFinite(sid) || sid <= 0) return
    // Cached in the store, so re-selecting the same server does not re-read.
    if (!inboundsByServer[sid]) void loadInbounds(sid)
  }, [serverId, inboundsByServer, loadInbounds])

  /** The inbounds of the server currently chosen, if they have been read. */
  const inboundsList = useMemo(() => {
    const sid = Number(serverId)
    if (!Number.isFinite(sid) || sid <= 0) return []
    return inboundsByServer[sid] ?? []
  }, [serverId, inboundsByServer])

  const inboundsBusy = inboundsLoading === Number(serverId)

  const selectedServer = useMemo(
    () => servers.find((s) => s.apiId === Number(serverId)) ?? null,
    [servers, serverId],
  )

  const canRelease = serverId !== '' && inboundId !== '' && !busy

  const release = async () => {
    if (!canRelease) return
    try {
      await setReleaseTarget(orderId, Number(serverId), Number(inboundId))
      await loadEvents(orderId)
      toast(
        'success',
        'Release target set',
        `Order #${orderId} will be built on ${selectedServer?.name ?? 'the chosen server'}.`,
      )
      setServerId('')
      setInboundId('')
    } catch (err) {
      toast(
        'error',
        'Could not set the release target',
        err instanceof ApiError ? err.message : 'The server refused the change.',
      )
    }
  }

  const events = eventsFor(orderId)
  const error =
    orderError ?? (!Number.isFinite(orderId) || orderId <= 0 ? 'That order id is not a number.' : null)

  return (
    <div className="od-page">
      <PageHeader
        title={order ? `Order #${order.id}` : `Order #${id}`}
        subtitle="A CUSTOM order names a spec, not a server. Choose where it is built, then release it."
        actions={
          <Button variant="ghost" leading={<ArrowLeft size={14} />} onClick={() => navigate('/orders')}>
            All orders
          </Button>
        }
      />

      {error && (
        <Card className="od-notice">
          <p className="t-sm2">{error}</p>
        </Card>
      )}

      {orderLoading ? (
        <Card>
          <Skeleton height={140} />
        </Card>
      ) : (
        order && (
          <>
            <Card className="od-summary">
              <div className="od-fact">
                <span className="od-fact__label">Amount</span>
                <span className="od-fact__value">
                  {order.currency} {(order.amount_cents / 100).toFixed(2)}
                </span>
              </div>
              <div className="od-fact">
                <span className="od-fact__label">Status</span>
                <span className="od-fact__value">{order.status}</span>
              </div>
              <div className="od-fact">
                <span className="od-fact__label">Reference</span>
                <span className="od-fact__value">{order.reference || 'none yet'}</span>
              </div>
              <div className="od-fact">
                <span className="od-fact__label">Created</span>
                <span className="od-fact__value">
                  {new Date(order.created_at * 1000).toLocaleString()}
                </span>
              </div>
            </Card>

            <Card className="od-release">
              <div className="od-release__head">
                <ServerIcon size={16} aria-hidden />
                <h2>Release this order</h2>
              </div>
              <p className="ui-muted t-sm2">
                Pick the server and the inbound the VPN will be created on. Both are recorded on this
                order&apos;s timeline before anything is built.
              </p>

              <div className="od-release__fields">
                <label className="ui-field">
                  <span className="ui-field__label">Server</span>
                  <select
                    className="ui-input"
                    value={serverId}
                    onChange={(e) => {
                      setServerId(e.target.value)
                      setInboundId('')
                    }}
                  >
                    <option value="">Choose a server…</option>
                    {servers.map((s) => (
                      <option key={s.id} value={s.apiId}>
                        {s.name} · {s.status}
                      </option>
                    ))}
                  </select>
                </label>

                <label className="ui-field">
                  <span className="ui-field__label">Inbound</span>
                  <select
                    className="ui-input"
                    value={inboundId}
                    onChange={(e) => setInboundId(e.target.value)}
                    disabled={!serverId || inboundsBusy}
                  >
                    <option value="">
                      {!serverId
                        ? 'Pick a server first'
                        : inboundsBusy
                          ? 'Reading inbounds…'
                          : inboundsList.length === 0
                            ? 'No inbounds on this server'
                            : 'Choose an inbound…'}
                    </option>
                    {inboundsList.map((i) => (
                      <option key={i.id} value={i.id}>
                        {i.remark || `inbound #${i.id}`} · {i.protocol}
                      </option>
                    ))}
                  </select>
                </label>
              </div>

              <div className="od-release__actions">
                <Button leading={<Check size={14} />} onClick={() => void release()} disabled={!canRelease}>
                  {busy ? 'Releasing…' : 'Set release target'}
                </Button>
                {servers.length === 0 && (
                  <span className="ui-muted t-sm2">
                    No server is configured yet — add one under Servers first.
                  </span>
                )}
              </div>
            </Card>

            <Card>
              <div className="od-release__head">
                <h2>Timeline</h2>
              </div>
              {events.length === 0 ? (
                <p className="ui-muted t-sm2">
                  No events recorded yet. Open the order from the queue to load its history.
                </p>
              ) : (
                <ol className="od-events">
                  {events.map((e) => (
                    <li key={e.id}>
                      <span className="od-events__kind">{e.kind}</span>
                      <span className="ui-muted t-sm2">
                        {new Date(e.createdAt * 1000).toLocaleString()}
                        {e.actorEmail ? ` · ${e.actorEmail}` : ''}
                      </span>
                      {e.detail && <span className="od-events__detail">{e.detail}</span>}
                    </li>
                  ))}
                </ol>
              )}
            </Card>
          </>
        )
      )}
    </div>
  )
}
