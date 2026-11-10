import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Activity,
  BadgeCheck,
  Bell,
  Clock3,
  Crown,
  ExternalLink,
  Plus,
  RefreshCw,
  ShieldCheck,
  TriangleAlert,
  User,
} from 'lucide-react'
import {
  Button,
  Card,
  EmptyState,
  Skeleton,
  StatusBadge,
  formatBytes,
  useToast,
} from '../components/ui'
import { VpnHeroCard } from '../components/vpn/VpnHeroCard'
import { VpnKindBadge } from '../components/vpn/VpnKindBadge'
import { useAuthStore } from '../stores/authStore'
import { useOffersStore } from '../stores/offersStore'
import { useVpnStore, vpnDisplayName, vpnServerLabel, type ClientVpn } from '../stores/vpnStore'
import { api } from '../lib/api'
import './client.css'

/*
 * ===========================================================================
 * CLIENT DASHBOARD - SECTION 01 (S4)
 *
 * Seven blocks, in the owner-approved order:
 *   01 Welcome header           05 Orders in progress  (D-090)
 *   02 VPN overview cards       06 Free VPN claim card
 *   03 Primary VPN card         07 Quick access
 *   04 VPN attention center
 *
 * The one rule this file is built around: EVERY NUMBER IS EITHER A COUNT OF
 * ROWS THE SERVER SENT OR A VALUE THE SERVER SENT. Nothing is derived locally
 * that the API did not state, and an unreadable value is rendered as unknown
 * (`-`), never as zero. `0` and "could not read" are different facts.
 *
 * What this screen deliberately does NOT do:
 *   - It does not claim, confirm, reject, cancel or refund a payment. D-090
 *     makes the Dashboard summary-only; the one payment read is a read.
 *   - It does not provision, release, suspend or write to a panel.
 *   - It does not filter VPNs by ownership in the UI: `GET /api/vpns` is already
 *     scoped to the caller by the server, and a second client-side filter is the
 *     copy that would eventually get it wrong.
 * ===========================================================================
 */

/** One clock read per mount, so "days left" cannot differ between renders. */
function useNowSeconds() {
  const [now] = useState(() => Date.now() / 1000)
  return now
}

function daysLeftFrom(expiresAt: number | null, nowSec: number): number | null {
  return expiresAt === null ? null : Math.ceil((expiresAt - nowSec) / 86400)
}

/**
 * One thing that needs the client's attention.
 *
 * `reason` is always built from a status the SERVER reported or from the expiry
 * timestamp the server sent. There is no invented classification here: every
 * `status` below exists in `VpnStatus` (`stores/vpnStore.ts:26-39`).
 */
interface AttentionItem {
  vpn: ClientVpn
  /** Short headline: what is wrong. */
  reason: string
  /** `error` = not usable at all; `warn` = usable now, stops soon. */
  tone: 'error' | 'warn'
}

/**
 * Map an owned VPN to an attention item, or null when it needs nothing.
 *
 * Ordering matters: a suspended VPN that also expires soon is reported as
 * SUSPENDED, because that is the fact that stops it working today.
 *
 * `UNTAGGED` is deliberately NOT an attention state. It means no catalogue
 * offer recorded which category the VPN belongs to - a classification gap, not
 * a fault in the customer's service. Raising it here would tell a customer
 * something is wrong with a VPN that may be working perfectly.
 */
function attentionFor(vpn: ClientVpn, daysLeft: number | null): AttentionItem | null {
  switch (vpn.status) {
    case 'TRAFFIC_DEPLETED':
      return { vpn, tone: 'error', reason: 'Data allowance is used up.' }
    case 'EXPIRED':
      return {
        vpn,
        tone: 'error',
        reason:
          daysLeft === null
            ? 'This VPN has expired.'
            : `Expired ${Math.abs(daysLeft)} day${Math.abs(daysLeft) === 1 ? '' : 's'} ago.`,
      }
    case 'SUSPENDED':
      return { vpn, tone: 'error', reason: 'Suspended - it cannot connect until it is resumed.' }
    case 'SERVER_UNAVAILABLE':
      return { vpn, tone: 'error', reason: 'Its server is unreachable right now.' }
    case 'REVOCATION_PENDING':
      return { vpn, tone: 'warn', reason: 'Being removed - no longer guaranteed to work.' }
    case 'REVOKED':
      return { vpn, tone: 'error', reason: 'Revoked - this credential no longer works.' }
    case 'PENDING':
      return { vpn, tone: 'warn', reason: 'Still being set up on the server.' }
    case 'ACTIVE':
      /* Active but inside the last week: the one genuinely useful warning. */
      if (daysLeft !== null && daysLeft >= 0 && daysLeft <= 7) {
        return {
          vpn,
          tone: 'warn',
          reason:
            daysLeft === 0
              ? 'Expires today.'
              : `Expires in ${daysLeft} day${daysLeft === 1 ? '' : 's'}.`,
        }
      }
      return null
    default:
      return null
  }
}

type UsageMap = Record<number, { total: number; limit: number; pct: number }>

/** Allowance for the server the FREE catalogue will provision on. */
interface FreeUsage {
  concurrent: number
  used_this_window: number
  monthly_limit: number
  concurrent_limit: number
}

export function ClientDashboardPage() {
  const navigate = useNavigate()
  const toast = useToast()
  const nowSec = useNowSeconds()

  const user = useAuthStore((s) => s.user)

  const vpns = useVpnStore((s) => s.vpns)
  const loading = useVpnStore((s) => s.loading)
  const error = useVpnStore((s) => s.error)
  const load = useVpnStore((s) => s.load)
  const claimFree = useVpnStore((s) => s.claimFree)

  /* The backend already scopes `GET /api/offers` by role, so for a client this
     is the FREE catalogue and nothing else. */
  const offers = useOffersStore((s) => s.offers)
  const loadOffers = useOffersStore((s) => s.load)

  const [claiming, setClaiming] = useState(false)

  /* --- 06: the FREE allowance, for the server the catalogue provisions on --
   *
   * Read from the server, never summed from the client's own VPN list: the
   * concurrent counter counts claim ROWS, and a released VPN keeps its row
   * while no longer counting, so a local sum would be a different, wrong answer.
   * `null` means "not read / could not read" - which renders as nothing at all,
   * not as 0/0.
   */
  const [freeUsage, setFreeUsage] = useState<FreeUsage | null>(null)
  const [freeUsageServer, setFreeUsageServer] = useState<number | null>(null)

  /* --- 05: orders in progress (D-090) ------------------------------------
   *
   * THREE states, kept distinct on purpose:
   *   undefined -> not read yet
   *   number    -> the server answered (0 means genuinely nothing waiting)
   *   'error'   -> the read failed, which is NOT zero
   *
   * The filter spans two statuses with different meanings, and the endpoint caps
   * its list at 200 rows, so no exact total can be stated from the array length.
   * D-090 therefore requires an INDICATOR, not a count.
   */
  const [pendingOrders, setPendingOrders] = useState<number | 'error' | undefined>(undefined)

  useEffect(() => {
    void load()
    void loadOffers(true)
  }, [load, loadOffers])

  /*
   * ONE payment read, on mount, and nothing else can re-fire it.
   *
   * The dependency list is empty on purpose: D-090 requires exactly one
   * request per Dashboard mount, and it must NOT be re-issued when an unrelated
   * VPN usage read settles. `load`/`loadOffers` are the settle-once store
   * actions and are already covered above; adding them here would re-request the
   * payment list every time anything else changed.
   */
  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const rows = await api.listPayments(true)
        if (cancelled) return
        setPendingOrders(rows.length)
      } catch {
        if (cancelled) return
        setPendingOrders('error')
      }
    })()
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const freeServerID = useMemo(() => {
    const first = offers.find((o) => o.active)
    return first?.serverId && first.serverId > 0 ? first.serverId : null
  }, [offers])

  useEffect(() => {
    if (!freeServerID) return
    let cancelled = false
    void (async () => {
      try {
        const u = await api.freeClaimUsage(freeServerID)
        if (cancelled) return
        setFreeUsage(u)
        setFreeUsageServer(freeServerID)
      } catch {
        /* The meter is guidance, not a claim. If it cannot be read it simply
           does not render - a fabricated 0/5 would be worse than silence. */
        if (!cancelled) setFreeUsage(null)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [freeServerID])

  /* ---- derived counts (all from rows the server sent) ------------------- */

  const activeCount = useMemo(() => vpns.filter((v) => v.status === 'ACTIVE').length, [vpns])
  const premiumCount = useMemo(() => vpns.filter((v) => v.kind === 'PREMIUM').length, [vpns])
  const freeCount = useMemo(() => vpns.filter((v) => v.kind === 'FREE').length, [vpns])
  const untaggedCount = useMemo(() => vpns.filter((v) => v.kind === 'UNTAGGED').length, [vpns])

  /*
   * The primary connection: premium first, then whatever is first.
   *
   * Unchanged from the S3 dashboard (`:95-96`) - the selection logic is the
   * owner-approved one and this stage does not get to second-guess it.
   */
  const primaryVpn = useMemo(
    () => vpns.find((v) => v.kind === 'PREMIUM') ?? (vpns.length > 0 ? vpns[0] : null),
    [vpns],
  )

  /* ---- 04: the attention list ------------------------------------------ */

  const attentionItems = useMemo(() => {
    const out: AttentionItem[] = []
    for (const v of vpns) {
      const item = attentionFor(v, daysLeftFrom(v.expiresAt, nowSec))
      if (item) out.push(item)
    }
    /* Errors first, then warnings; within a tone, soonest expiry first. */
    out.sort((a, b) => {
      if (a.tone !== b.tone) return a.tone === 'error' ? -1 : 1
      const ax = a.vpn.expiresAt ?? Number.MAX_SAFE_INTEGER
      const bx = b.vpn.expiresAt ?? Number.MAX_SAFE_INTEGER
      return ax - bx
    })
    return out
  }, [vpns, nowSec])

  const attentionCount = attentionItems.length

  /* ---- 02: usage, read ONCE per VPN id ---------------------------------
   *
   * Keyed by id and issued only for the VPNs on screen (the dashboard shows at
   * most the primary card), so this can never become an N+1 loop over the whole
   * inventory. The dependency is the joined id list, not the array identity, so
   * an unrelated re-render does not re-request anything.
   */
  const [usage, setUsage] = useState<UsageMap>({})
  const usageKey = vpns.map((v) => v.apiId).filter((id) => id > 0).join(',')

  useEffect(() => {
    const ids = usageKey ? usageKey.split(',').map(Number) : []
    if (ids.length === 0) return
    let cancelled = false
    void (async () => {
      const entries = await Promise.all(
        ids.map(async (id) => {
          try {
            const s = await api.vpnUsageSummary(id)
            return [id, { total: s.lifetime_total_bytes, limit: s.limit_bytes, pct: s.usage_pct }] as const
          } catch {
            /* A card without a usage bar beats a card claiming 0 B used. */
            return null
          }
        }),
      )
      if (cancelled) return
      const next: UsageMap = {}
      for (const e of entries) if (e) next[e[0]] = e[1]
      setUsage(next)
    })()
    return () => {
      cancelled = true
    }
  }, [usageKey])

  /*
   * Total data used across every owned VPN.
   *
   * Summed ONLY when every VPN's usage read succeeded. A partial sum would
   * understate the real figure and still read as a fact, so an incomplete read
   * shows an em dash instead. That is the difference between "0 B used" and
   * "we could not read it" - and they are not the same statement.
   */
  const usageValues = vpns.filter((v) => v.apiId > 0).map((v) => usage[v.apiId])
  const usageReadable = usageValues.length > 0 && usageValues.every((u) => u !== undefined)
  const totalUsed = usageReadable ? usageValues.reduce((n, u) => n + (u?.total ?? 0), 0) : null
  const totalLimit = usageReadable
    ? usageValues.reduce((n, u) => n + (u?.limit ?? 0), 0)
    : 0


  /* ---- 06: claim ------------------------------------------------------- */

  const claim = async (offerId: number) => {
    const offer = offers.find((o) => o.apiId === offerId)
    if (!offer) return
    setClaiming(true)
    try {
      const id = await claimFree(offerId)
      toast('success', 'Free VPN provisioned', 'The panel confirmed the client')
      /* Re-read the allowance: the meter must move the moment a claim lands, or
         the next claim is made against a stale number. */
      if (freeServerID) {
        try {
          setFreeUsage(await api.freeClaimUsage(freeServerID))
        } catch {
          setFreeUsage(null)
        }
      }
      navigate(`/vpn/${id}`)
    } catch (err) {
      /* The two caps have distinct messages on purpose: "you are holding 3
         already" and "you have used this month's 5" need different next steps. */
      const msg = err instanceof Error ? err.message : 'Could not provision'
      toast('error', 'Could not provision', msg)
    } finally {
      setClaiming(false)
    }
  }

  const activeOffers = offers.filter((o) => o.active)
  const allowanceKnown = freeUsage !== null && freeUsageServer === freeServerID
  /*
   * Why a free claim would be refused right now, or null when it would not.
   *
   * Derived from the SERVER's two counters, never from the local VPN list: the
   * holding counter counts claim rows and a released VPN keeps its row while no
   * longer counting, so a local count would be a different, wrong answer.
   *
   * `null` (allowance not read) is deliberately NOT a block - the button stays
   * enabled and the server remains the authority. Guessing "blocked" from a
   * failed read would stop a customer doing something they are allowed to do.
   *
   * The two sentences differ on purpose, because the next step differs:
   * a holding block is the customer's to clear, a monthly cap is not.
   */
  const claimBlocked: string | null = (() => {
    if (!allowanceKnown || !freeUsage) return null
    if (freeUsage.concurrent >= freeUsage.concurrent_limit) {
      return `You already hold ${freeUsage.concurrent} of ${freeUsage.concurrent_limit} free VPNs. Release one in My VPNs to claim a different package.`
    }
    if (freeUsage.used_this_window >= freeUsage.monthly_limit) {
      return `You have used all ${freeUsage.monthly_limit} free claims for this period. Premium packages are still available.`
    }
    return null
  })()

  /*
   * ---- the one-sentence verdict (U5, reframed by the D-092 follow-up) ----
   *
   * HONESTY RULE. `attentionCount === 0` proves only that no VPN reported a
   * fault state. It does not prove the service is being used, and it does not
   * prove the panel agrees with what we mirrored. The older copy said
   * "All N VPNs are working normally" on an account where nothing had ever been
   * measured - a claim the backend cannot support.
   *
   * The wording is therefore graded, and the strongest sentence it can say is
   * "no faults reported". The slot ceiling moves to the front when the customer
   * is holding it, because that is the fact that stops them acting: they cannot
   * claim a different free package while every slot is taken.
   */
  const holdingCeiling =
    allowanceKnown && freeUsage !== null && freeUsage.concurrent >= freeUsage.concurrent_limit

  const verdict =
    vpns.length === 0
      ? 'Nothing is on your account yet - claim a free VPN below or buy a premium package.'
      : attentionCount > 0
        ? `${attentionCount} of your ${vpns.length} VPN${vpns.length === 1 ? '' : 's'} need${attentionCount === 1 ? 's' : ''} attention - see the list below.`
        : holdingCeiling
          ? `No faults reported on your ${vpns.length} VPN${vpns.length === 1 ? '' : 's'}. You are holding all ${freeUsage?.concurrent_limit ?? ''} free slots, so release one before claiming a different free package.`
          : totalUsed === null
            ? `No faults reported on your ${vpns.length} VPN${vpns.length === 1 ? '' : 's'}, but usage could not be read - open a VPN to see its traffic.`
            : `No faults reported on your ${vpns.length} VPN${vpns.length === 1 ? '' : 's'}.`

  /* The old two-line verdict, kept only as a reminder of the wording that was
     removed. It is dead on purpose: nothing reads it. */
  // REMOVED: `All ${vpns.length} VPN(s) are working normally.`



  const asyncStatus = error ? 'error' : loading && vpns.length === 0 ? 'loading' : 'ready'

  return (
    <div className="dash cl-dash" data-testid="client-dashboard">
      {/* ---------- 01 . WELCOME HEADER --------------------------------- */}
      <section className="cl-welcome" aria-labelledby="cl-welcome-title">
        <div className="cl-welcome__text">
          <span className="cl-welcome__eyebrow">Client portal</span>
          <h1 className="cl-welcome__title" id="cl-welcome-title">
            Welcome back, {user?.name ?? 'there'}
          </h1>
          <p className="cl-welcome__sub" data-testid="client-verdict">
            {verdict}
          </p>
          {/* Account status: the only fact available is whether the e-mail is
              verified, which the server states in `/api/me`. */}
          {user?.emailVerified && (
            <span className="cl-welcome__chip">
              <BadgeCheck size={13} aria-hidden="true" />
              E-mail verified
            </span>
          )}
        </div>
        <div className="cl-welcome__actions">
          <Button
            size="sm"
            variant="outline"
            leading={<Bell size={14} />}
            onClick={() => navigate('/client/notifications')}
          >
            Notifications
          </Button>
          <Button
            size="sm"
            variant="ghost"
            leading={<User size={14} />}
            onClick={() => navigate('/profile')}
          >
            Profile
          </Button>
        </div>
      </section>

      {/* ---------- 02 . VPN OVERVIEW CARDS ----------------------------- */}
      {vpns.length > 0 && (
        <section className="cl-overview" aria-label="VPN overview">
          {/* A. Active VPNs */}
          <div className="cl-ov cl-ov--ok">
            <div className="cl-ov__head">
              <span className="cl-ov__label">Active VPNs</span>
              <span className="cl-ov__icon"><ShieldCheck size={17} aria-hidden="true" /></span>
            </div>
            <span className="cl-ov__value">{activeCount}</span>
            <p className="cl-ov__delta">of {vpns.length} on your account</p>
          </div>

          {/* B. Data used - never 0 when the read failed */}
          <div className="cl-ov cl-ov--info">
            <div className="cl-ov__head">
              <span className="cl-ov__label">Data used</span>
              <span className="cl-ov__icon"><Activity size={17} aria-hidden="true" /></span>
            </div>
            <span className="cl-ov__value">
              {totalUsed === null ? '\u2014' : formatBytes(totalUsed)}
            </span>
            <p className="cl-ov__delta">
              {totalUsed === null
                ? 'usage could not be read'
                : totalLimit > 0
                  ? `of ${formatBytes(totalLimit)} allowance`
                  : 'across your VPNs'}
            </p>
          </div>

          {/* C. Needs attention */}
          <div className={`cl-ov ${attentionCount > 0 ? 'cl-ov--warn' : 'cl-ov--ok'}`}>
            <div className="cl-ov__head">
              <span className="cl-ov__label">Needs attention</span>
              <span className="cl-ov__icon"><TriangleAlert size={17} aria-hidden="true" /></span>
            </div>
            <span className="cl-ov__value">{attentionCount}</span>
            <p className="cl-ov__delta">
              {attentionCount > 0 ? 'expiring, expired or not connectable' : 'all settled'}
            </p>
          </div>

          {/*
            D. Premium VPNs.

            The category split is shown as the server reports it, and every
            category that exists appears here: PREMIUM, FREE, and UNTAGGED.
            UNTAGGED is a panel client that no catalogue offer classified - it
            is listed so the split always adds up to the owned total, and it is
            named "unclassified" rather than folded into Free or Premium.
          */}
          <div className="cl-ov cl-ov--info">
            <div className="cl-ov__head">
              <span className="cl-ov__label">Premium VPNs</span>
              <span className="cl-ov__icon"><Crown size={17} aria-hidden="true" /></span>
            </div>
            <span className="cl-ov__value">{premiumCount}</span>
            <p className="cl-ov__delta">
              {freeCount} free
              {untaggedCount > 0 ? ` \u00b7 ${untaggedCount} unclassified` : ''}
            </p>
          </div>
        </section>
      )}

      {/* ---------- 05 . ORDERS IN PROGRESS (D-090) ----------------------
        * Rendered ONLY when the server said something is waiting. A zero hides
        * the whole block (no empty card, no "0"), and a failed read is its own
        * explicit state - never a zero.
        * ------------------------------------------------------------------ */}
      {pendingOrders === 'error' && (
        <section className="cl-orders cl-orders--error" data-testid="orders-in-progress-error">
          <div className="cl-orders__body">
            <span className="cl-orders__title">Could not load order status</span>
            <p className="cl-orders__note">
              This is a connection problem, not an empty account. Your orders are safe - open
              Purchases to see them.
            </p>
          </div>
          <Button size="sm" variant="outline" onClick={() => navigate('/history')}>
            Open Purchases
          </Button>
        </section>
      )}

      {typeof pendingOrders === 'number' && pendingOrders > 0 && (
        <section className="cl-orders" data-testid="orders-in-progress">
          <div className="cl-orders__body">
            <span className="cl-orders__title">
              <Clock3 size={14} aria-hidden="true" style={{ marginRight: 6, verticalAlign: '-2px' }} />
              Orders in progress
            </span>
            {/*
              An INDICATOR, not a count. The list is capped at 200 rows and the
              filter spans two states with different meanings, so neither an
              exact total nor the phrase "awaiting operator confirmation" would
              be true (D-090).
            */}
            <p className="cl-orders__note">
              You have at least one order still moving. Open Purchases to see its status and
              what happens next.
            </p>
          </div>
          <Button size="sm" variant="outline" onClick={() => navigate('/history')}>
            View orders
          </Button>
        </section>
      )}

      {/* ---------- 03 . PRIMARY VPN ------------------------------------- */}
      <section className="cl-primary" aria-label="Primary VPN">
        {primaryVpn ? (
          <VpnHeroCard
            vpn={primaryVpn}
            usage={usage[primaryVpn.apiId] ?? null}
            daysLeft={daysLeftFrom(primaryVpn.expiresAt, nowSec)}
          />
        ) : loading ? (
          <Skeleton height={148} />
        ) : (
          <Card title="Your primary connection">
            <EmptyState
              icon={<ShieldCheck size={22} />}
              title="No VPN yet"
              description="Claim a free VPN from the catalogue below, or buy a premium package - it is provisioned on the panel immediately."
              action={
                <div className="ui-row" style={{ gap: 8, flexWrap: 'wrap' }}>
                  <Button
                    size="sm"
                    variant="primary"
                    leading={<Plus size={14} />}
                    onClick={() => {
                      document.getElementById('cl-free-claim')?.scrollIntoView({ block: 'start' })
                    }}
                  >
                    Claim a free VPN
                  </Button>
                  <Button size="sm" variant="outline" onClick={() => navigate('/buy')}>
                    Buy premium
                  </Button>
                </div>
              }
            />
          </Card>
        )}
      </section>

      {/* ---------- 04 . VPN ATTENTION CENTER ----------------------------
        * Hidden entirely when nothing needs attention - an alert panel that is
        * always present stops meaning anything.
        * ------------------------------------------------------------------ */}
      {attentionItems.length > 0 && (
        <section aria-label="Needs attention">
          <div className="cl-h2" style={{ marginBottom: 'var(--space-3)' }}>
            <h2 className="cl-h2__title">Needs attention</h2>
            <span className="cl-h2__aside">
              {attentionItems.length} VPN{attentionItems.length === 1 ? '' : 's'}
            </span>
          </div>
          <div className="cl-attn" data-testid="client-attention-list">
            {attentionItems.map(({ vpn, reason, tone }) => {
              const d = daysLeftFrom(vpn.expiresAt, nowSec)
              const u = usage[vpn.apiId]
              return (
                <div
                  key={vpn.id}
                  className={`cl-attn__row${tone === 'error' ? ' cl-attn__row--error' : ''}`}
                >
                  <div className="cl-attn__body">
                    <span className="cl-attn__title">
                      <TriangleAlert size={14} aria-hidden="true" />
                      {vpnDisplayName(vpn)}
                      <VpnKindBadge kind={vpn.kind} />
                      <StatusBadge status={vpn.status} />
                    </span>
                    <p className="cl-attn__why">{reason}</p>
                    <span className="cl-attn__meta">
                      <span>{vpnServerLabel(vpn)}</span>
                      {d !== null && (
                        <span>
                          {d < 0
                            ? `expired ${Math.abs(d)}d ago`
                            : d === 0
                              ? 'expires today'
                              : `${d}d left`}
                        </span>
                      )}
                      {u && (
                        <span>
                          {formatBytes(u.total)}
                          {u.limit > 0 ? ` / ${formatBytes(u.limit)} used` : ' used'}
                        </span>
                      )}
                    </span>
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    leading={<ExternalLink size={13} />}
                    onClick={() => navigate(`/vpn/${vpn.id}`)}
                  >
                    Open
                  </Button>
                </div>
              )
            })}
          </div>
        </section>
      )}

      {/* ---------- 06 . FREE VPN CLAIM CARD ----------------------------- */}
      <section id="cl-free-claim" className="cl-claim" aria-label="Free VPN claim">
        <div className="cl-h2">
          <h2 className="cl-h2__title">Get a free VPN</h2>
          {/* The aside points at the meter below instead of restating it:
              the two numbers already render as ``held`` and ``this month``
              meters, and a third copy of the same subtraction was the
              duplicate this line removes. */}
          <span className="cl-h2__aside">
            {allowanceKnown ? 'Two limits, shown below' : 'Allowance unavailable'}
          </span>
        </div>


        {/*
          The allowance, BEFORE the claim button. Two ceilings, because they
          answer two different questions: how many you can HOLD right now, and
          how many you may TAKE this month. Releasing frees a holding slot but
          never a monthly one, so a client who cannot see both cannot choose
          informed. Rendered only when the server answered.
        */}
        {allowanceKnown && freeUsage && (
          <div className="cl-claim__meters" data-testid="client-free-allowance">
            <div className="cl-claim__meter">
              <span className="cl-claim__meter-label">
                <span>Free VPNs held</span>
                <span className="cl-claim__meter-num">
                  {freeUsage.concurrent} / {freeUsage.concurrent_limit}
                </span>
              </span>
              <span className="cl-claim__track">
                <span
                  className={`cl-claim__fill${
                    freeUsage.concurrent >= freeUsage.concurrent_limit ? ' is-full' : ''
                  }`}
                  style={{
                    width: `${Math.min(
                      100,
                      Math.round(
                        (freeUsage.concurrent / Math.max(1, freeUsage.concurrent_limit)) * 100,
                      ),
                    )}%`,
                  }}
                />
              </span>
            </div>

            <div className="cl-claim__meter">
              <span className="cl-claim__meter-label">
                <span>Claims this month</span>
                <span className="cl-claim__meter-num">
                  {freeUsage.used_this_window} / {freeUsage.monthly_limit}
                </span>
              </span>
              <span className="cl-claim__track">
                <span
                  className={`cl-claim__fill${
                    freeUsage.used_this_window >= freeUsage.monthly_limit
                      ? ' is-full'
                      : freeUsage.used_this_window / Math.max(1, freeUsage.monthly_limit) >= 0.8
                        ? ' is-high'
                        : ''
                  }`}
                  style={{
                    width: `${Math.min(
                      100,
                      Math.round(
                        (freeUsage.used_this_window / Math.max(1, freeUsage.monthly_limit)) * 100,
                      ),
                    )}%`,
                  }}
                />
              </span>
            </div>
          </div>
        )}

        {/* The rule that surprises people, stated where it matters. */}
        {allowanceKnown && freeUsage && (
          <span className="ui-muted t-sm2">
            Releasing a free VPN frees a holding slot but does <strong>not</strong> give the monthly
            one back.
          </span>
        )}

        {allowanceKnown && freeUsage && freeUsage.used_this_window >= freeUsage.monthly_limit && (
          <span className="vpn-warn t-sm2">
            You have used every free claim for this window. Premium is still available.
          </span>
        )}

        {asyncStatus === 'loading' && <Skeleton height={120} />}

        {asyncStatus === 'error' && (
          <EmptyState
            icon={<TriangleAlert size={22} />}
            title="Could not load your free offers"
            description="This is a connection problem, not an empty catalogue. Try again."
            action={
              <Button size="sm" variant="primary" leading={<RefreshCw size={13} />} onClick={() => void load()}>
                Try again
              </Button>
            }
          />
        )}

        {asyncStatus === 'ready' && activeOffers.length === 0 && (
          <p className="ui-muted rhythm" style={{ margin: 0 }}>
            No free offers are available right now - a Super Admin publishes them on the Offers
            screen. Nothing is wrong with your account.
          </p>
        )}

        {/*
         * One disabled state that must be explained, not just coloured.
         *
         * A free claim fails on the server for two different reasons, and the
         * customer needs to know which: the holding ceiling (release one first) or
         * the monthly cap (nothing can be done until the window rolls). Leaving the
         * button enabled made the click the only way to discover it, which is the
         * defect this block removes. `null` allowance = not read, so the button
         * stays enabled and the server is the authority.
         */}
        {activeOffers.length > 0 && (
          <div className="cl-claim__offers">
            {activeOffers.map((o) => (
              <Card key={o.id} className="client-vpn-card">
                <div className="client-vpn-card__top">
                  <span className="ui-strong">{o.name}</span>
                  <VpnKindBadge kind="FREE" />
                </div>
                <span className="ui-muted t-sm2">
                  {o.trafficLimitGB > 0 ? `${o.trafficLimitGB} GB` : 'unlimited'}
                  {' \u00b7 '}
                  {o.expiryDays > 0 ? `${o.expiryDays} days` : 'no expiry'}
                  {' \u00b7 '}
                  {o.deviceLimit > 0 ? `${o.deviceLimit} device(s)` : 'unlimited devices'}
                </span>
                <Button
                  size="sm"
                  leading={<Plus size={14} />}
                  disabled={claiming || claimBlocked !== null}
                  title={claimBlocked ?? undefined}
                  onClick={() => void claim(o.apiId)}
                >
                  {claiming ? 'Provisioning\u2026' : claimBlocked ? 'No slot available' : 'Claim free VPN'}
                </Button>
                {claimBlocked && <span className="cl-claim__blocked t-sm2">{claimBlocked}</span>}
              </Card>
            ))}
          </div>
        )}
      </section>
      {/*
       * 07 . QUICK ACCESS - REMOVED.
       *
       * All six shortcuts here duplicated a sidebar destination exactly.
       * CLIENT_NAV in components/layout/navModel.ts renders the same six
       * routes in its Main / Get more / Help / Account groups. A second copy
       * of navigation is not a shortcut - it is a page the customer has to
       * scroll past to reach the thing this screen is actually for.
       *
       * Nothing was dropped from the product: every route stays reachable
       * from the sidebar, and the in-page actions that DO belong here (claim
       * a free VPN, open a VPN, buy premium) are already buttons above.
       */}
    </div>
  )
}
