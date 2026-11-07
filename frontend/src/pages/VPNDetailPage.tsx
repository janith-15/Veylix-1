import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Crown, ExternalLink, HelpCircle, MessageSquare, TriangleAlert } from 'lucide-react'
import { Button, Card, PageHeader, QrCode, Skeleton, StatusBadge, Tabs, useToast } from '../components/ui'
import { Progress } from '../components/charts'
import { useVpnStore, vpnDisplayName, vpnServerLabel } from '../stores/vpnStore'
import { VpnKindBadge } from '../components/vpn/VpnKindBadge'
import { api, ApiError, type ApiVpnLinks, type ApiVpnPing, type ApiVpnProtocol, type ApiVpnUsage } from '../lib/api'
import { measureClientPing, type ClientPingResult } from '../lib/clientPing'
import { PremiumInsights } from '../components/vpn/PremiumInsights'
import { SetupGuideDialog } from '../components/vpn/SetupGuideDialog'
import { ProtocolDetail } from '../components/vpn/ProtocolDetail'
import { DeviceList } from '../components/vpn/DeviceList'
import { useVpnActions } from '../components/vpn/useVpnActions'
import { useAuthStore } from '../stores/authStore'
import './client.css'

/**
 * Deep links for a seller's contact channels (Bible §2.3).
 *
 * Stated once, here, because the normalisation matters: a `wa.me` link needs
 * digits only, and a `t.me` link needs the handle without its leading `@`. A
 * malformed link is worse than no link, so the caller renders a channel only
 * when the seller actually configured one — an empty value never becomes a
 * broken URL.
 */
function whatsappHref(raw: string): string {
  return `https://wa.me/${raw.replace(/\D/g, '')}`
}

function telegramHref(raw: string): string {
  return `https://t.me/${raw.replace(/^@+/, '').trim()}`
}

/**
 * Per-VPN detail — Premium: full page; Free: the same page with the panels the
 * backend can actually fill (D-026).
 *
 * Connection material is REAL now (Milestone 5-B step 11): `GET /api/vpns/{id}/links`
 * returns the panel's own URIs, passed through untouched. Three distinct states
 * are rendered because they mean different things and only one of them is the
 * user's problem:
 *   - `source: 'unavailable'` → the panel could not be read (NOT "no link");
 *   - `links: []`             → the panel answered and had no URI for this client;
 *   - `links: [...]`          → a real, connectable configuration.
 *
 * Usage comes from `GET /api/vpns/{id}/usage` (E5); the page repeats the
 * backend's `history` verdict instead of drawing a zero line that would imply
 * "no traffic".
 */

/**
 * Setup guides that actually tell a client what to install.
 *
 * The previous copy said "install the recommended client app" and named nothing —
 * a customer holding a working VPN could not connect, which turns a working
 * product into a support ticket. Each entry names a REAL client, links to its
 * official source, and gives steps that match that app's actual import flow.
 *
 * Only widely-used, open-source-or-official clients are listed: recommending an
 * app is a promise that it will not be abandoned or bundled with adware.
 */
interface Guide {
  /** Apps that accept a vless:// or vmess:// import URI. */
  apps: { name: string; url: string; note?: string }[]
  steps: string[]
}

const GUIDES: Record<string, Guide> = {
  Android: {
    apps: [
      {
        name: 'v2rayNG',
        url: 'https://github.com/2dust/v2rayNG/releases',
        note: 'open source, no ads',
      },
      {
        name: 'NekoBox',
        url: 'https://github.com/MatsuriDayo/NekoBoxForAndroid/releases',
        note: 'open source, supports more protocols',
      },
    ],
    steps: [
      'Install one of the apps above (allow “Install unknown apps” if your browser asks).',
      'Copy your connection link from the top of this page.',
      'Open the app → “+” (top right) → “Import config from clipboard”.',
      'Tap the new entry, then press the connect button. Allow the VPN permission prompt.',
      'Return here and press “Check reachability” to confirm the server answers.',
    ],
  },
  iOS: {
    apps: [
      {
        name: 'Streisand',
        url: 'https://apps.apple.com/app/streisand/id6450534064',
        note: 'free on the App Store',
      },
      {
        name: 'Shadowrocket',
        url: 'https://apps.apple.com/app/shadowrocket/id932747118',
        note: 'paid, one-time purchase',
      },
    ],
    steps: [
      'Install one of the apps above from the App Store.',
      'Copy your connection link from the top of this page.',
      'Open the app → “+” → “Add from clipboard” (or paste the link into a new configuration).',
      'Save, then toggle the VPN switch on and allow the configuration prompt.',
      'Return here and press “Check reachability” to confirm the server answers.',
    ],
  },
  Windows: {
    apps: [
      {
        name: 'v2rayN',
        url: 'https://github.com/2dust/v2rayN/releases',
        note: 'open source, needs .NET Desktop Runtime',
      },
      {
        name: 'NekoRay',
        url: 'https://github.com/MatsuriDayo/nekoray/releases',
        note: 'open source, portable',
      },
    ],
    steps: [
      'Download and unzip one of the apps above (no installer needed for the portable builds).',
      'Copy your connection link from the top of this page.',
      'Open the app → “Servers” → “Import from clipboard”.',
      'Select the imported server and press Ctrl+R (or the “Connect” button).',
      'Return here and press “Check reachability” to confirm the server answers.',
    ],
  },
}


function fmtBytes(n: number): string {
  if (n <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)))
  return `${(n / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

/**
 * What to tell a reader when a VPN is not connectable — one sentence per status,
 * because "SUSPENDED" and "TRAFFIC_DEPLETED" need different next steps and the
 * old copy lumped them all into "not connectable right now".
 *
 * Only statuses the backend actually emits appear here (`stores/vpnStore.ts`
 * `VpnStatus`). An unknown status falls through to the generic sentence rather
 * than being given an invented explanation.
 */
const NOT_CONNECTABLE: Partial<Record<string, string>> = {
  SUSPENDED: 'Paused by your seller — nothing is deleted and it can be resumed.',
  TRAFFIC_DEPLETED: 'Your data allowance is used up. Premium packages have their own allowance.',
  EXPIRED: 'This VPN has expired. A new package is needed to reconnect.',
  SERVER_UNAVAILABLE: 'The server this VPN lives on is not reachable right now.',
  REVOCATION_PENDING: 'A removal has been requested and is waiting for the server to confirm it.',
  REVOKED: 'This VPN has been revoked.',
  RELEASED: 'You released this VPN. It stays undoable for 3 days, then only this record is removed.',
  REMOVED_FROM_CATALOG: 'This package is no longer offered. The VPN itself still exists.',
  PENDING: 'Still being set up on the server — this page updates when it finishes.',
}

/**
 * "12 days left" — the answer a customer is actually asking for.
 *
 * A date requires the reader to know today and do arithmetic. One clock read per
 * mount keeps the render pure, and `null` (no expiry) is never rendered as "0
 * days", which is a different promise.
 */
function daysToExpiry(unix: number): string {
  const days = Math.ceil((unix - Date.now() / 1000) / 86400)
  if (days < 0) return `expired ${Math.abs(days)} day${Math.abs(days) === 1 ? '' : 's'} ago`
  if (days === 0) return 'expires today'
  return `${days} day${days === 1 ? '' : 's'} left`
}

export function VPNDetailPage() {
  const { id, serverId: serverIdParam, panelEmail } = useParams()
  const navigate = useNavigate()
  const vpns = useVpnStore((s) => s.vpns)
  const vpnsLoading = useVpnStore((s) => s.loading)
  const load = useVpnStore((s) => s.load)
  const [guide, setGuide] = useState('Android')
  const [usage, setUsage] = useState<ApiVpnUsage | null>(null)
  const [usageError, setUsageError] = useState<string | null>(null)
  /* Connection material (D-026). `null` = not loaded yet; an OUTAGE is a
     distinct `source: 'unavailable'`, never an empty list. */
  const [links, setLinks] = useState<ApiVpnLinks | null>(null)
  const [ping, setPing] = useState<ApiVpnPing | null>(null)
  const [pingBusy, setPingBusy] = useState(false)
  const toast = useToast()

  /*
   * Two ways in: `/vpn/:id` (the client's own screen) and
   * `/vpns/:serverId/:panelEmail` (the Super Admin's console, which has no local
   * id for a panel-only client).
   *
   * The panel email is matched INSIDE its server. The route has always carried
   * `:serverId` and this comment has always claimed the match was scoped by it,
   * but the code compared the address alone — so the same panel email on two
   * servers could open the wrong VPN, with the wrong link and the wrong QR.
   *
   * `serverIdParam` is optional so `/vpn/:id` keeps working; when it is present
   * it is authoritative.
   */
  const decodedEmail = panelEmail ? decodeURIComponent(panelEmail) : null
  const routeServerID = serverIdParam ? Number(serverIdParam) : null
  const vpn = (() => {
    if (decodedEmail) {
      const onServer = vpns.filter((v) => v.panelEmail === decodedEmail)
      // Scope by server when the route names one, and fall back to the unscoped
      // match only when it cannot disambiguate (an old link with no server).
      if (routeServerID && Number.isFinite(routeServerID)) {
        const exact = onServer.find((v) => v.serverId === routeServerID)
        if (exact) return exact
        return undefined
      }
      return onServer[0]
    }
    return vpns.find((v) => v.id === id || String(v.apiId) === id)
  })()

  /**
   * Staff (Super Admin / reseller) get the lifecycle controls; a client does
   * not. The check is a UI convenience only — `loadVPN` re-checks ownership and
   * role server-side on every action, so hiding the button is not the guard.
   */
  const role = useAuthStore((s) => s.user?.role ?? 'client')
  const isStaff = role === 'super-admin' || role === 'admin'
  const reloadVpns = useVpnStore((s) => s.load)
  // The hook must be called unconditionally (Rules of Hooks). `vpn` is only
  // read inside the handlers, and every handler is behind `isStaff && vpn`,
  // so a placeholder object is never dereferenced.
  const lifecycle = useVpnActions({
    vpn: vpn ?? ({} as never),
    onChanged: () => void reloadVpns(),
  })

  // A direct hit on /vpn/:id (or a reload) has an empty cache.
  useEffect(() => {
    if (!vpn && vpns.length === 0) void load()
  }, [vpn, vpns.length, load])

  /*
   * Links are fetched on every mount and NEVER cached: a rotated credential
   * would leave a link on screen that silently stopped working. Switching VPN
   * re-runs this effect, and the previous row's answer is replaced by the new
   * one (or by `unavailable` if the fetch fails).
   *
   * A failed fetch is NOT one situation. Before this, every error — a 404 for a
   * VPN this account cannot see, a 401 for an expired session, a 5xx from the
   * panel, a network drop — produced the same "the panel could not be read"
   * panel. That message was wrong for three of the four and left the customer
   * with nothing to act on. The status code now decides the sentence.
   */
  const [linksError, setLinksError] = useState<string | null>(null)
  /** Which connection link the "How to connect" dialog is explaining, or null. */
  const [guideOpen, setGuideOpen] = useState<number | null>(null)
  /**
   * Clear the previous VPN's error the moment the target changes.
   *
   * This is React's documented "adjust state when a prop changes" pattern: the
   * reset happens during render, guarded by a comparison, so it costs one extra
   * render instead of an effect cascade — and it cannot leave a stale
   * "not visible to your account" banner on the next VPN.
   */
  const [linksForVpn, setLinksForVpn] = useState<number | null>(null)
  if (vpn && linksForVpn !== vpn.apiId) {
    setLinksForVpn(vpn.apiId)
    setLinksError(null)
  }
  useEffect(() => {
    if (!vpn) return
    let cancelled = false
    // `linksError` is cleared where it can change: a new VPN clears it below,
    // and the fetch clears it on success. Resetting it here would be a
    // synchronous setState inside an effect — a cascading render for no reason.
    api
      .vpnLinks(vpn.apiId)
      .then((l) => {
        if (!cancelled) {
          setLinksError(null)
          setLinks(l)
        }
      })
      .catch((err: unknown) => {
        if (cancelled) return
        setLinks({
          vpn_id: vpn.apiId,
          panel_email: vpn.panelEmail,
          status: vpn.status,
          links: [],
          source: 'unavailable',
        })
        const status = err instanceof ApiError ? err.status : 0
        if (status === 404) {
          setLinksError(
            'This VPN is not visible to your account, so its link cannot be read. ' +
              'If you were given this link by a seller, sign in with the account that bought it.',
          )
        } else if (status === 401 || status === 403) {
          setLinksError('Your session has expired. Sign in again to load the connection link.')
        } else {
          setLinksError(null) // a real outage: the panel warn block below says it
        }
      })
    return () => {
      cancelled = true
    }
  }, [vpn])

  /** Server reachability — the label is in the response, so the UI cannot
   *  accidentally present it as the user's own latency. */
  const checkReachability = async () => {
    if (!vpn) return
    setPingBusy(true)
    try {
      setPing(await api.vpnPing(vpn.apiId))
    } catch {
      toast('error', 'Ping failed', 'The server did not answer')
    } finally {
      setPingBusy(false)
    }
  }

  /**
   * P-05 — the latency THIS device sees, measured by opening a WebSocket to the
   * VPN server and timing the handshake.
   *
   * The server's own `host:port` comes from `/protocol`, which is panel-sourced,
   * so the probe targets the real endpoint rather than anything synthesised. A
   * failed handshake is reported as "could not be measured" — never as a number.
   */
  const [clientPing, setClientPing] = useState<ClientPingResult | null>(null)
  const [clientPingBusy, setClientPingBusy] = useState(false)

  const measureFromHere = async () => {
    if (!vpn) return
    setClientPingBusy(true)
    setClientPing(null)
    try {
      const protocol: ApiVpnProtocol = await api.vpnProtocol(vpn.apiId)
      if (protocol.source === 'unavailable') {
        setClientPing({
          measured: false,
          latencyMs: -1,
          target: '',
          scope: 'client_to_server',
          reason: 'The panel could not be read, so this VPN’s server address is unknown right now.',
        })
        return
      }
      setClientPing(await measureClientPing(protocol.server))
    } catch {
      setClientPing({
        measured: false,
        latencyMs: -1,
        target: '',
        scope: 'client_to_server',
        reason: 'The server address could not be read.',
      })
    } finally {
      setClientPingBusy(false)
    }
  }

  /* ---- release lifecycle (D-027) ---- */
  const releases = useVpnStore((s) => s.releases)
  const loadReleases = useVpnStore((s) => s.loadReleases)
  const release = useVpnStore((s) => s.release)
  const restoreRelease = useVpnStore((s) => s.restoreRelease)
  const [releaseBusy, setReleaseBusy] = useState(false)
  /* One reading of the clock per render: the countdown must not shift between
     two reads inside the same paint. */
  const [nowMs] = useState(() => Date.now())

  useEffect(() => {
    void loadReleases()
  }, [loadReleases])

  /** The newest release of THIS vpn that is still undoable. */
  const pendingRelease = releases.find(
    (r) => r.vpn_id === vpn?.apiId && r.restored_at === null && r.removed_at === null,
  ) ?? null

  /** Whole days left in the window, rounded up so "1 day" never shows as 0.
   *  `now` is captured once per render so the value cannot change mid-paint. */
  const nowSec = Math.floor(nowMs / 1000)
  const daysLeft = pendingRelease
    ? Math.max(0, Math.ceil((pendingRelease.restore_deadline - nowSec) / 86_400))
    : 0

  /** A depleted VPN is the one the client releases; the backend is the judge. */
  const releasable = vpn ? (vpn.status === 'TRAFFIC_DEPLETED' || vpn.status === 'EXPIRED') : false

  const doRelease = async () => {
    if (!vpn) return
    setReleaseBusy(true)
    try {
      const row = await release(vpn.apiId, 'quota_exhausted')
      const left = Math.max(1, Math.ceil((row.restore_deadline - Date.now() / 1000) / 86_400))
      toast('success', 'VPN released', `You can undo this for ${left} more day(s)`)
    } catch (err) {
      toast('error', 'Could not release', err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setReleaseBusy(false)
    }
  }

  const undoRelease = async () => {
    if (!pendingRelease) return
    setReleaseBusy(true)
    try {
      await restoreRelease(pendingRelease.id)
      setPing(null)
      toast('success', 'Release undone', 'Your VPN stays as it was — nothing was deleted')
    } catch (err) {
      toast('error', 'Could not undo', err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setReleaseBusy(false)
    }
  }

  useEffect(() => {
    if (!vpn) return
    let cancelled = false
    api
      .vpnUsage(vpn.apiId)
      .then((u) => {
        if (!cancelled) {
          setUsage(u)
          setUsageError(null)
        }
      })
      .catch(() => {
        if (!cancelled) setUsageError('Usage history could not be read')
      })
    return () => {
      cancelled = true
    }
  }, [vpn])

  /*
   * A VPN can be missing from the cache for three DIFFERENT reasons, and the
   * screen used to show one string for all of them — "it may have been revoked,
   * or it belongs to another account" — which sent an operator looking for a
   * permissions problem when the real cause was that `GET /api/vpns` never
   * returns another account's rows, or that the list had not finished loading.
   * Naming the reason is what makes this page debuggable.
   */
  const missingReason = (() => {
    if (!decodedEmail && !id) return 'That address does not name a VPN.'
    if (vpnsLoading) return null
    if (decodedEmail && routeServerID && Number.isFinite(routeServerID)) {
      return (
        `No VPN named “${decodedEmail}” is on server ${routeServerID} in the list this ` +
        'account can read. A VPN owned by a customer is only listed for its owner and ' +
        'for staff — it may also have been revoked.'
      )
    }
    return 'This VPN is not in the list this account can read. It may have been revoked, or it may belong to another account.'
  })()

  if (!vpn) {
    // The way back must depend on WHO is looking: the old screen always sent
    // the operator to /client, a route a Super Admin cannot even reach.
    return (
      <div className="dash">
        <PageHeader
          title={vpnsLoading ? 'Loading this VPN…' : 'VPN not found'}
          subtitle={missingReason ?? undefined}
        />
        {!vpnsLoading && (
          <div className="ui-row" style={{ gap: 8 }}>
            <Button variant="outline" onClick={() => void load()}>
              Reload
            </Button>
            <Button variant="outline" onClick={() => navigate(isStaff ? '/vpns' : '/client')}>
              {isStaff ? 'Back to VPN management' : 'Back to my VPNs'}
            </Button>
          </div>
        )}
      </div>
    )
  }

  const active = vpn.status === 'ACTIVE'

  // Real numbers only, and the SERVER's numbers (D-038 P-01/P-02): the page
  // repeats `usage_pct` / `lifetime_total_bytes` instead of adding up+down and
  // rounding a percentage itself — two places doing that arithmetic is how a
  // screen ends up disagreeing with the API. `limit_bytes = 0` means unlimited.
  const limit = usage?.limit_bytes ?? 0
  const used = usage?.lifetime_total_bytes ?? 0
  const usagePct = usage?.usage_pct ?? 0
  const hasHistory = usage?.history === 'AVAILABLE' || usage?.history === 'PARTIAL'

  return (
    <div className="dash" data-testid="vpn-detail">
      <PageHeader
        title={vpnDisplayName(vpn)}
        subtitle={`${vpnServerLabel(vpn)} · created ${new Date(vpn.createdAt * 1000).toLocaleDateString()}`}
        actions={
          <span className="ui-row" style={{ gap: 8 }}>
            <VpnKindBadge kind={vpn.kind} />
            {isStaff && lifecycle.actions}
            <Button size="sm" variant="outline" onClick={() => navigate(-1)}>
              Back
            </Button>
          </span>
        }
      />

      {/*
        * U9 (UI/UX defect board) — the hero used to open with a sentence about
        * the background sync, which is an implementation detail in the customer's
        * first paragraph. The verdict band states what the reader actually came
        * for, from facts the server has already reported: status, how long is
        * left, and how much data is left. Nothing here is computed locally except
        * the days-left subtraction against one clock read per mount, which is the
        * same rule the client dashboard cards follow.
        */}
      <div className="vpn-detail-hero">
        <div className="ui-col" style={{ gap: 8, maxWidth: 520 }}>
          <div className="vpn-detail-hero__meta">
            <StatusBadge status={vpn.status} />
            <span className="vpn-chip">{vpnServerLabel(vpn)}</span>
            <span className="vpn-chip">
              {vpn.expiresAt
                ? `Expires ${new Date(vpn.expiresAt * 1000).toLocaleDateString()}`
                : 'No expiry'}
            </span>
          </div>
          <p className="vpn-verdict" data-testid="vpn-verdict">
            {active
              ? limit > 0
                ? `${fmtBytes(Math.max(0, limit - used))} of data left${
                    vpn.expiresAt ? ` · ${daysToExpiry(vpn.expiresAt)}` : ' · no expiry'
                  }`
                : `Unlimited data${vpn.expiresAt ? ` · ${daysToExpiry(vpn.expiresAt)}` : ' · no expiry'}`
              : NOT_CONNECTABLE[vpn.status] ?? 'This VPN is not connectable right now.'}
          </p>
        </div>
        <div className="ui-col" style={{ gap: 10, minWidth: 220 }}>
          {usageError ? (
            <span className="ui-muted t-sm2">{usageError}</span>
          ) : !usage ? (
            <Skeleton height={44} />
          ) : (
            <>
              <Progress
                value={usagePct}
                label={
                  limit > 0
                    ? `Usage — ${fmtBytes(used)} of ${fmtBytes(limit)}`
                    : `Usage — ${fmtBytes(used)} (unlimited)`
                }
              />
              <span className="ui-muted t-sm">
                {hasHistory
                  ? `${fmtBytes(usage.lifetime_down_bytes)} down · ${fmtBytes(usage.lifetime_up_bytes)} up · ${fmtBytes(used)} total`
                  : 'No usage samples yet — the first sync tick starts the history.'}
              </span>
            </>
          )}
        </div>
      </div>

      {!active && (
        <div className="vpn-warn" style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
          <TriangleAlert size={16} aria-hidden="true" />
          <div>
            <strong>This VPN is not connectable right now</strong>
            <p className="t-sm2" style={{ margin: '4px 0 0' }}>
              Status: {vpn.status}. Connection material is hidden until the server is verified
              again — your purchase history is safe. Contact your seller below.
            </p>
          </div>
        </div>
      )}

      {/* U10 (UI/UX defect board) — the cards used to stack in one column with no
          grouping, so a reader had to scroll a wall of panels to find the one they
          wanted. They are now three named sections in reading order: what it is
          doing (Usage), how to connect (Connection), and the account-level facts
          (Account). Each heading is an <h2> so the sections are navigable by
          screen reader, not just visually separated. */}
      <h2 className="vpn-section-title">Usage</h2>
      {/* The owner's Premium feature set (D-038): P-01 usage history with
          up/down/total, P-02 capacity, P-03 CPU/RAM, P-04 live rate,
          P-06 range switch, P-07 per-metric charts, P-08 chart type.
          Mounted for BOTH kinds: a Free VPN is a VPN too, and its charts use
          the same panel-sourced numbers — only the window differs (D-026). */}
      <PremiumInsights vpnId={vpn.apiId} active={active} />

      {isStaff && (
        <>
          <h2 className="vpn-section-title">Devices</h2>
          {/* D8: the machines holding this VPN's device cap. Mounted for staff
              only — a client sees their own devices through support, and the panel
              list is an operator tool. The panel is the only source, so this
              renders the panel's truth or says it could not be read. */}
          <DeviceList serverId={vpn.serverId} panelEmail={vpn.panelEmail} />
        </>
      )}

      <h2 className="vpn-section-title">Connection</h2>
      <div className="dash-split">
        <Card
          title="Connection"
          actions={
            active ? (
              <div className="ui-row" style={{ gap: 8 }}>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => void checkReachability()}
                  disabled={pingBusy}
                  title="Measures the server's own reachability — not your latency"
                >
                  {pingBusy ? 'Pinging…' : 'Ping server'}
                </Button>
                {/* P-05 (D-038): the customer's OWN latency. A separate button so
                    the two numbers can never be confused for one another. */}
                <Button
                  size="sm"
                  onClick={() => void measureFromHere()}
                  disabled={clientPingBusy}
                  title="Measures the round trip from this device to the VPN server"
                >
                  {clientPingBusy ? 'Measuring…' : 'Test my ping'}
                </Button>
              </div>
            ) : undefined
          }
        >
          {active ? (
            <div className="ui-col" style={{ gap: 10 }}>
              {links === null ? (
                <span className="ui-muted t-sm2">Loading connection details…</span>
              ) : linksError ? (
                /* A 404/401 is NOT an outage: naming it stops the customer
                   hunting for a panel problem that does not exist. */
                <div className="vpn-warn" style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
                  <TriangleAlert size={16} aria-hidden="true" />
                  <div>
                    <strong>The connection link could not be loaded</strong>
                    <p className="t-sm2" style={{ margin: '4px 0 0' }}>{linksError}</p>
                  </div>
                </div>
              ) : links.source === 'unavailable' ? (
                <div className="vpn-warn" style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
                  <TriangleAlert size={16} aria-hidden="true" />
                  <div>
                    <strong>The panel could not be read</strong>
                    <p className="t-sm2" style={{ margin: '4px 0 0' }}>
                      We cannot show a link while the panel is unreachable — that is not the
                      same as “no link exists”. Try again shortly.
                    </p>
                  </div>
                </div>
              ) : links.links.length === 0 ? (
                <div className="vpn-warn" style={{ display: 'flex', gap: 10, alignItems: 'flex-start' }}>
                  <TriangleAlert size={16} aria-hidden="true" />
                  <div>
                    <strong>No connection link was issued</strong>
                    <p className="t-sm2" style={{ margin: '4px 0 0' }}>
                      The panel reports this client but returned no URI for it. Contact your
                      seller — a placeholder link would look valid and fail to connect.
                    </p>
                  </div>
                </div>
              ) : (
                <div className="vpn-connect-grid">
                  {links.links.map((uri, i) => (
                    <div key={uri} className="ui-col" style={{ gap: 8 }}>
                      <span className="ui-field__label">Connection link{i > 0 ? ` ${i + 1}` : ''}</span>
                      {/* D-033: the QR is what a phone user actually needs, and it
                          encodes the panel's own URI — never a synthesised one. */}
                      <QrCode
                        value={uri}
                        label={`QR code for connection link ${i + 1}`}
                        size={180}
                      />
                      <code
                        style={{
                          fontSize: 11.5,
                          wordBreak: 'break-all',
                          background: 'var(--surface-2)',
                          padding: '8px 10px',
                          borderRadius: 'var(--radius-sm)',
                        }}
                      >
                        {uri}
                      </code>
                      <div className="ui-row" style={{ gap: 8 }}>
                        <Button
                          size="sm"
                          onClick={() => {
                            void navigator.clipboard.writeText(uri)
                            toast('success', 'Copied', 'Paste it into your VPN client')
                          }}
                        >
                          Copy link
                        </Button>
                        <a href={uri} className="no-underline">
                          <Button size="sm" variant="outline">Open in app</Button>
                        </a>
                        {/* The link alone does not tell a first-time customer
                            what to DO with it. This is the missing "next step". */}
                        <Button
                          size="sm"
                          variant="ghost"
                          leading={<HelpCircle size={14} />}
                          onClick={() => setGuideOpen(i)}
                        >
                          How to connect
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {ping && (
                <span className="ui-muted t-sm">
                  {ping.reachable
                    ? `Server reachable — ${ping.latency_ms} ms (server reachability, not your own latency)`
                    : 'Server did not answer — this measures the panel host, not your connection'}
                </span>
              )}

              {/* P-05 — the number the customer actually cares about, labelled
                  with what it measured and where it was measured from. A failed
                  probe says so; it never renders a latency it did not observe. */}
              {clientPing && (
                <span className="ui-muted t-sm">
                  {clientPing.measured ? (
                    <>
                      <strong>{clientPing.latencyMs} ms</strong> from this device to{' '}
                      <code>{clientPing.target}</code> — measured in your browser, so this is the
                      latency your connection sees (not the server&apos;s own).
                    </>
                  ) : (
                    <>Could not measure your latency. {clientPing.reason}</>
                  )}
                </span>
              )}

              {/* D-027: a depleted VPN can be RELEASED. The release is undoable
                  for 3 days, so the copy says so instead of threatening deletion. */}
              {releasable && (
                <div className="vpn-warn t-sm2" style={{ display: 'block' }}>
                  <strong>Out of traffic?</strong>
                  <p style={{ margin: '4px 0 8px' }}>
                    You can release this VPN. It keeps working on the server and you have
                    <strong> 3 days to undo</strong> — after that only the record here is
                    removed, not the panel client.
                  </p>
                  {pendingRelease ? (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={releaseBusy}
                      onClick={() => void undoRelease()}
                    >
                      Undo release ({daysLeft} day{daysLeft === 1 ? '' : 's'} left)
                    </Button>
                  ) : (
                    <Button size="sm" variant="outline" disabled={releaseBusy} onClick={() => void doRelease()}>
                      {releaseBusy ? 'Releasing…' : 'Release this VPN'}
                    </Button>
                  )}
                </div>
              )}

              {/*
                * U4 (UI/UX defect board): this line used to read
                * "Panel identity: … on server N", which sounds like something
                * the customer must act on. It is a support reference, so it is
                * labelled as one and de-emphasised — and the server is named
                * with the operator's own label, not a database id.
                */}
              <span className="ui-muted t-sm">
                Connection ID (technical — quote it to support):{' '}
                <code>{vpn.panelEmail}</code> on {vpnServerLabel(vpn)}
              </span>
            </div>
          ) : (
            <p className="ui-muted" style={{ margin: 0 }}>
              Connection details are hidden while this VPN is unavailable.
            </p>
          )}
        </Card>

        <Card title="Setup guide">
          <Tabs
            items={[
              { value: 'Android', label: 'Android' },
              { value: 'iOS', label: 'iOS' },
              { value: 'Windows', label: 'Windows' },
            ]}
            value={guide}
            onChange={setGuide}
          />
          {(() => {
            const g = GUIDES[guide]
            if (!g) return null
            return (
              <>
                <div className="guide-apps">
                  <span className="ui-muted t-sm2">
                    Install one of these
                  </span>
                  {g.apps.map((app) => (
                    <a
                      key={app.name}
                      className="guide-app"
                      href={app.url}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <span className="guide-app__name">{app.name}</span>
                      {app.note && <span className="guide-app__note">{app.note}</span>}
                      <ExternalLink size={13} aria-hidden="true" />
                      <span className="ui-sr-only"> (opens in a new tab)</span>
                    </a>
                  ))}
                </div>
                <ol className="guide-steps ui-mt-1">
                  {g.steps.map((step) => (
                    <li key={step}>{step}</li>
                  ))}
                </ol>
              </>
            )
          })()}
        </Card>
      </div>

      {/* P-09 (D-038): vless/vmess, TCP/WS and the credential, read from the
          panel's own client record. Full width because the two-column split
          above is sized for exactly two children. */}
      <ProtocolDetail vpnId={vpn.apiId} />

      {/* A Free VPN is a real VPN, so there is no nagging banner — just one
          honest statement of what Premium is and how to get it. A customer who
          never sees the paid option cannot choose it. */}
      {vpn.kind === 'FREE' && (
        <Card title="Keep this VPN going">
          <div
            className="ui-row"
            style={{ justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}
          >
            <span className="ui-muted t-sm2">
              This is a Free VPN. A Premium VPN comes from a package you choose, with
              its own data allowance, expiry and device limit.
            </span>
            <Button
              size="sm"
              variant="primary"
              leading={<Crown size={14} />}
              onClick={() => navigate('/buy')}
            >
              See Premium options
            </Button>
          </div>
        </Card>
      )}

      <Card title="Support">
        <div className="ui-col" style={{ gap: 10 }}>
          {vpn.sellerId > 0 ? (
            <>
              <span className="ui-muted t-sm2">
                <strong className="ui-strong">
                  {vpn.sellerName || `Seller #${vpn.sellerId}`}
                </strong>{' '}
                sold you this VPN. Message them on any channel below.
              </span>
              {/* The seller's own configured contacts — the same ones the payment
                  destination already shows a payer (D-036.2). A channel that was
                  never set is omitted, never rendered empty. */}
              <div className="ui-row" style={{ gap: 14, flexWrap: 'wrap' }}>
                {vpn.sellerEmail && (
                  <a className="ui-muted t-sm2" href={`mailto:${vpn.sellerEmail}`}>
                    {vpn.sellerEmail}
                  </a>
                )}
                {vpn.sellerWhatsApp && (
                  <a
                    className="ui-muted t-sm2"
                    href={whatsappHref(vpn.sellerWhatsApp)}
                    target="_blank"
                    rel="noreferrer"
                  >
                    WhatsApp {vpn.sellerWhatsApp}
                  </a>
                )}
                {vpn.sellerTelegram && (
                  <a
                    className="ui-muted t-sm2"
                    href={telegramHref(vpn.sellerTelegram)}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Telegram {vpn.sellerTelegram}
                  </a>
                )}
              </div>
            </>
          ) : (
            <span className="ui-muted t-sm2">
              This VPN was issued by the platform. Support is available through the
              internal chat.
            </span>
          )}
          <div className="ui-row">
            <Button
              size="sm"
              variant="outline"
              leading={<MessageSquare size={14} />}
              onClick={() => navigate('/chat')}
            >
              Open chat
            </Button>
          </div>
        </div>
      </Card>

      {/* Super Admin lifecycle confirms. Rendered for staff only; the handlers
          are unreachable otherwise. */}
      {isStaff && lifecycle.dialogs}

      <SetupGuideDialog
        open={guideOpen !== null}
        onClose={() => setGuideOpen(null)}
        link={guideOpen !== null ? links?.links[guideOpen] : undefined}
      />
    </div>
  )
}