/**
 * P-05 — a ping measured from the CUSTOMER'S OWN BROWSER (D-038).
 *
 * The owner was explicit: *"ping eka test karaddi e app eke ewele ping eka,
 * methanadi server eke ping eka denna epa"* — show the latency the app sees,
 * not a ping taken on the server. `POST /api/vpns/{id}/ping` measures the
 * server's view and is correctly labelled as such; this module measures the
 * other half.
 *
 * **What a browser can honestly measure.** JavaScript cannot send an ICMP echo
 * or a raw TCP packet. What it can do is open a WebSocket to the VPN endpoint
 * and time how long the handshake takes to resolve. For a WebSocket-capable
 * inbound (the common `ws`/`tls` transport, and Xray answers an upgrade on the
 * inbound's own port) that is a genuine round trip to the VPN server from this
 * device. For an inbound that speaks something else on that port, the handshake
 * fails — and a failed probe is reported as "could not be measured", never as a
 * latency, because a made-up number here would be indistinguishable from a real
 * one.
 *
 * The scope label travels with the result so no screen can present this as
 * anything more than it is.
 */

/** One measurement attempt. */
export interface ClientPingResult {
  /** True only when a handshake actually completed and was timed. */
  measured: boolean
  /** Round-trip milliseconds. `-1` when not measured — never 0, which reads as instant. */
  latencyMs: number
  /** `host:port` the probe went to, so the customer can see what was tested. */
  target: string
  /**
   * Constant label. Mirrors the server's own contract (`server_reachability`),
   * so the two numbers on the page are never confused for one another.
   */
  scope: 'client_to_server'
  /** Why it was not measured, when `measured` is false. */
  reason?: string
}

const DEFAULT_TIMEOUT_MS = 4000

/** Split `host:port` (also `[v6]:port`) into its parts. `null` = unusable. */
export function parseTarget(server: string): { host: string; port: number } | null {
  const text = server.trim()
  if (!text) return null
  // Bracketed IPv6: [2001:db8::1]:443
  const v6 = /^\[([^\]]+)\]:(\d+)$/.exec(text)
  if (v6) {
    const port = Number(v6[2])
    return Number.isFinite(port) && port > 0 ? { host: v6[1], port } : null
  }
  const idx = text.lastIndexOf(':')
  if (idx <= 0) return null
  const host = text.slice(0, idx)
  const port = Number(text.slice(idx + 1))
  if (!host || !Number.isFinite(port) || port <= 0 || port > 65535) return null
  return { host, port }
}

/**
 * Measures one handshake round trip to `server` (`host:port`).
 *
 * Resolves — it never rejects — so a caller can always render an outcome:
 * either a number with its scope, or an honest "could not be measured" with the
 * reason. `timeoutMs` bounds the whole attempt, so a black-holed port cannot
 * hang the page.
 */
export function measureClientPing(server: string, timeoutMs = DEFAULT_TIMEOUT_MS): Promise<ClientPingResult> {
  const target = server.trim()
  const notMeasured = (reason: string): ClientPingResult => ({
    measured: false,
    latencyMs: -1,
    target,
    scope: 'client_to_server',
    reason,
  })

  const parsed = parseTarget(target)
  if (!parsed) return Promise.resolve(notMeasured('The server address is not usable, so nothing could be tested.'))

  // A browser may only open a secure WebSocket to a secure page; `wss` also
  // matches most real inbounds, which sit behind TLS.
  const url = `wss://${parsed.host}:${parsed.port}`

  return new Promise<ClientPingResult>((resolve) => {
    let settled = false
    let socket: WebSocket | null = null
    const started = performance.now()

    const finish = (result: ClientPingResult) => {
      if (settled) return
      settled = true
      window.clearTimeout(timer)
      if (socket) {
        // Detach handlers before closing so a late event cannot re-enter.
        socket.onopen = null
        socket.onerror = null
        socket.onclose = null
        try {
          socket.close()
        } catch {
          /* already closing */
        }
      }
      resolve(result)
    }

    const timer = window.setTimeout(() => {
      finish(notMeasured('The server did not answer within the time allowed.'))
    }, timeoutMs)

    try {
      socket = new WebSocket(url)
    } catch {
      finish(notMeasured('This browser refused to open the connection to the server.'))
      return
    }

    // A completed handshake is the measurement: `open` fires once the server has
    // accepted the upgrade, so the elapsed time is a real round trip.
    socket.onopen = () => {
      const elapsed = Math.max(0, Math.round(performance.now() - started))
      finish({
        measured: true,
        latencyMs: elapsed,
        target,
        scope: 'client_to_server',
      })
    }

    // An error or immediate close means the port does not speak WebSocket. That
    // is not a latency, so it is reported as unmeasured rather than as a number.
    socket.onerror = () => {
      finish(
        notMeasured(
          'This server did not accept a WebSocket handshake, so a latency could not be measured from the browser.',
        ),
      )
    }

    socket.onclose = () => {
      finish(notMeasured('The connection closed before the handshake completed.'))
    }
  })
}
