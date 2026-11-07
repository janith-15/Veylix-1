import { useCallback, useEffect, useState } from 'react'
import { Button, useToast } from '../../components/ui'
import { api, type ApiHwidDevice } from '../../lib/api'
import { Smartphone, Trash2, RefreshCw } from 'lucide-react'
import './device-list.css'

/**
 * DeviceList — the hardware identifiers a VPN has registered against its device
 * cap (D8).
 *
 * Three rules this component follows, because a device list is something an
 * operator acts on:
 *
 *  1. **The panel is the only source.** Nothing is cached and nothing is
 *     invented; a panel that cannot be read says so, it does not show "no
 *     devices", because those two states mean opposite things to a customer
 *     who cannot connect.
 *  2. **Removal is confirmed, not optimistic.** The row disappears only after the
 *     DELETE succeeds — the backend has already re-read the panel to confirm it.
 *  3. **No device shape is assumed.** 3X-UI returns a short fingerprint and an
 *     optional model/OS string; an empty field is shown as an em dash rather than
 *     an empty cell that looks like a rendering bug.
 */
export function DeviceList({ serverId, panelEmail }: { serverId: number; panelEmail: string }) {
  const toast = useToast()
  const [devices, setDevices] = useState<ApiHwidDevice[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busyId, setBusyId] = useState<number | null>(null)

  const load = useCallback(async () => {
    setError(null)
    try {
      const res = await api.panelClientDevices(serverId, panelEmail)
      setDevices(res.devices ?? [])
    } catch (err) {
      // "Could not read" is not "no devices": keep the list null so the screen
      // renders the failure state, never an empty table.
      setDevices(null)
      setError(err instanceof Error ? err.message : 'The device list could not be read.')
    } finally {
      setLoading(false)
    }
  }, [serverId, panelEmail])

  // Deferred by a tick, the same way PremiumInsights starts its first read:
  // `loading` is already true, so the first paint is honest and the effect
  // performs no synchronous setState.
  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0)
    return () => window.clearTimeout(timer)
  }, [load])

  const refresh = () => {
    setLoading(true)
    void load()
  }

  const revoke = async (device: ApiHwidDevice) => {
    setBusyId(device.id)
    try {
      await api.revokePanelClientDevice(serverId, panelEmail, device.id)
      // The server confirmed the removal before answering, so dropping the row
      // now states a verified fact.
      setDevices((cur) => (cur ? cur.filter((d) => d.id !== device.id) : cur))
      toast('success', 'Device signed out', `${device.fingerprint || 'The device'} no longer holds a slot.`)
    } catch (err) {
      toast('error', 'The device was not signed out',
        err instanceof Error ? err.message : 'The panel did not confirm the removal.')
    } finally {
      setBusyId(null)
    }
  }

  return (
    <section className="dl" aria-labelledby="dl-title">
      <div className="dl-head">
        <h2 id="dl-title" className="dl-title">
          <Smartphone size={15} aria-hidden="true" /> Devices
        </h2>
        <Button variant="ghost" leading={<RefreshCw size={13} />} onClick={refresh}
          loading={loading} disabled={loading}>
          Refresh
        </Button>
      </div>

      {error && (
        <p className="dl-error" role="status">
          The device list could not be read: {error}
        </p>
      )}

      {!error && loading && devices === null && (
        <p className="ui-muted dl-note">Reading the device list from the panel…</p>
      )}

      {!error && devices !== null && devices.length === 0 && (
        <p className="ui-muted dl-note">
          No device has registered against this VPN yet. A device appears here the
          first time it connects.
        </p>
      )}

      {!error && devices !== null && devices.length > 0 && (
        <div className="dl-rows">
          {devices.map((d) => (
            <div className="dl-row" key={d.id}>
              <div className="dl-what">
                <code className="dl-fp">{d.fingerprint || `device ${d.id}`}</code>
                <span className="dl-meta">
                  {[d.deviceModel, d.deviceOs, d.osVersion].filter(Boolean).join(' · ') || '—'}
                </span>
                <span className="dl-when">
                  last seen {d.lastSeen ? new Date(d.lastSeen * 1000).toLocaleString() : '—'}
                </span>
              </div>
              <Button variant="ghost" leading={<Trash2 size={13} />}
                loading={busyId === d.id}
                disabled={busyId !== null}
                onClick={() => void revoke(d)}
                aria-label={`Sign out device ${d.fingerprint || d.id}`}>
                Sign out
              </Button>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
