import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AlertTriangle, Check, Crown, Info, Search, ShoppingBag, Sparkles } from 'lucide-react'
import { Button } from '../../components/ui'
import { ClientPage, ClientSection } from '../../components/client/ClientPage'
import { api, type ApiPackage, type ApiPayment, type ApiVpn } from '../../lib/api'
import { formatMoney } from '../../lib/format'
import { useVpnStore } from '../../stores/vpnStore'
import './offers.css'
import { activeTimeZone } from '../../lib/time'

/*
 * SECTION 03 - Offers & Orders.
 *
 * ONE catalogue request (`GET /api/packages`, ADR-010), then everything else is
 * client-side over that array: search, category chips, price range, traffic /
 * duration / device filters, sorts, paging, compare and the confirmation step.
 * Nothing re-fetches on a keystroke.
 *
 * The single exception is `GET /api/vpns`, and it is not per-card: it runs once,
 * and its only job is to map an offer to the `server_id` a Free claim needs.
 * `/api/packages` does not carry `server_id` (verified: the handler emits eleven
 * keys and that is not one of them), and a client may not call `/api/servers`
 * (403), so the client's own VPN rows are the only legitimate source. See
 * `serverIdByOffer`.
 *
 * WHAT THIS PAGE REFUSES TO DO, deliberately:
 *   - it never prints a missing price as `0`;
 *   - it never renders `server_label` ("server 11") as a customer-facing name;
 *   - it never claims a "Best value" badge unless a real comparison produced one;
 *   - it never invents a package name for an order that does not carry one.
 * Each of those is a place where an easier implementation would have lied.
 *
 * This file is deliberately ASCII-only (no em dashes, no ellipsis glyphs). An
 * earlier revision was corrupted by a PowerShell round-trip that re-decoded the
 * multi-byte characters, so the dashes are written as the literal word "N/A" or
 * spelled out. Nothing here depends on the tty encoding any more.
 */

/** Page size. Cards and list share it so a view switch cannot change the set. */
const PAGE_SIZE = 9

/**
 * Shown wherever a value genuinely does not exist.
 *
 * `n/a` read as an error to a customer: on a Premium offer with no assigned
 * server the Server row said `n/a`, which looks like something failed rather
 * than like the field is not set. An em dash is the established "no value"
 * mark on this surface and is what `of-spec__v--none` styles for.
 */
const NONE = '\u2014'

/** The customer-facing categories. CUSTOM folds into Premium, never its own. */
type Category = 'all' | 'free' | 'premium'

type SortKey = 'newest' | 'price-asc' | 'price-desc' | 'traffic' | 'duration'

/** A catalogue row plus everything this screen derived about it. */
interface Offer {
  /** The raw id - this is what `POST /api/vpns` takes. */
  id: number
  name: string
  /** Customer-facing category: `CUSTOM` and `PREMIUM` both land on `premium`. */
  category: 'free' | 'premium'
  /** True for a package the API marks `kind: "CUSTOM"` - priced per order. */
  isCustom: boolean
  trafficGb: number
  durationDays: number
  deviceLimit: number
  priceCents: number
  currency: string
  serverLabel: string
  purchasable: boolean
  pricedPerOrder: boolean
  /**
   * The exact payable amount, or `null` when it cannot be authoritatively
   * established. `null` is what disables Buy - never a substituted zero.
   */
  payable: { amountMinor: number; currency: string } | null
  /** A Free package is always `Free`; its currency field is irrelevant. */
  isFree: boolean
}

/**
 * Normalises one API row.
 *
 * The price rule is the heart of this function. A FREE package is `Free` whatever
 * its currency says. A PREMIUM package is payable only when it has a positive
 * amount AND a non-empty currency; anything else becomes `payable: null`, which
 * the UI renders as "Price not set" and cannot be bought. A package priced per
 * order has no catalogue price at all - the quote decides.
 */
function toOffer(p: ApiPackage): Offer {
  const isFree = p.kind === 'FREE'
  const isCustom = p.kind === 'CUSTOM'
  const pricedPerOrder = p.priced_per_order === true || isCustom
  const currency = (p.currency ?? '').trim()
  const priceCents = Number.isFinite(p.price_cents) ? p.price_cents : 0

  let payable: Offer['payable'] = null
  if (!isFree && !pricedPerOrder && currency !== '' && priceCents > 0) {
    payable = { amountMinor: priceCents, currency }
  }

  return {
    id: p.id,
    name: p.name,
    category: isFree ? 'free' : 'premium',
    isCustom,
    trafficGb: p.traffic_gb,
    durationDays: p.duration_days,
    deviceLimit: p.device_limit,
    priceCents,
    currency,
    serverLabel: p.server_label ?? '',
    purchasable: p.purchasable === true,
    pricedPerOrder,
    payable,
    isFree,
  }
}

/** `50 GB`, or the not-available marker. */
function trafficLabel(gb: number): string {
  if (!Number.isFinite(gb) || gb <= 0) return NONE
  return `${gb} GB`
}

/** `30 days`, or the not-available marker. */
function durationLabel(days: number): string {
  if (!Number.isFinite(days) || days <= 0) return NONE
  return `${days} day${days === 1 ? '' : 's'}`
}

/** `5 devices`, or the not-available marker. */
function deviceLabel(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return NONE
  return `${n} device${n === 1 ? '' : 's'}`
}

/** Short date for an order row. Locale-formatted, never a raw epoch. */
function shortDate(epochSeconds: number): string {
  if (!Number.isFinite(epochSeconds) || epochSeconds <= 0) return NONE
  try {
    return new Date(epochSeconds * 1000).toLocaleDateString(undefined, {
    timeZone: activeTimeZone(),
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    })
  } catch {
    return NONE
  }
}


/* ======================================================================= *
 * The page
 * ======================================================================= */

export function ClientOffersPage() {
  const navigate = useNavigate()
  const vpns = useVpnStore((s) => s.vpns)
  const vpnsLoading = useVpnStore((s) => s.loading)
  const loadVpns = useVpnStore((s) => s.load)
  const claimFree = useVpnStore((s) => s.claimFree)

  const [packages, setPackages] = useState<Offer[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  /** Bumped by "Try again" to re-run the catalogue effect. */
  const [reloadToken, setReloadToken] = useState(0)

  const [orders, setOrders] = useState<ApiPayment[]>([])
  const [ordersLoading, setOrdersLoading] = useState(true)
  const [ordersError, setOrdersError] = useState<string | null>(null)

  /* --- filters (all client-side over `packages`) --------------------- */
  const [q, setQ] = useState('')
  const [category, setCategory] = useState<Category>('all')
  const [showFilters, setShowFilters] = useState(false)
  const [minGb, setMinGb] = useState('')
  const [minDays, setMinDays] = useState('')
  const [minDevices, setMinDevices] = useState('')
  const [priceCurrency, setPriceCurrency] = useState('')
  const [priceMin, setPriceMin] = useState('')
  const [priceMax, setPriceMax] = useState('')
  const [sort, setSort] = useState<SortKey>('newest')
  const [view, setView] = useState<'cards' | 'list'>('cards')
  const [page, setPage] = useState(1)

  /* --- compare + confirmation ---------------------------------------- */
  const [picked, setPicked] = useState<number[]>([])
  const [reviewing, setReviewing] = useState<Offer | null>(null)
  const [claimBusy, setClaimBusy] = useState(false)
  const [claimResult, setClaimResult] = useState<{ ok: boolean; text: string } | null>(null)

  /* --- the ONE catalogue read (ADR-010) ------------------------------ */
  /*
   * The fetch itself lives in the effect below. `reloadToken` is the only way to
   * ask for it again, and it is incremented from the "Try again" button's own
   * event handler - which is also where `loading` is set back to true.
   */

  useEffect(() => {
    /*
     * The fetch is inlined in this effect rather than delegated to a `useCallback`
     * it then calls. The lint rule that forbids a synchronous `setState` in an
     * effect cannot see through a callback boundary, so routing the same logic
     * through one would be reported as a cascading update even though every write
     * below happens after an `await`. Written this way the boundary is visible.
     */
    let cancelled = false
    void (async () => {
      try {
        const rows = await api.listPackagesTyped()
        if (cancelled) return
        setPackages(rows.map(toOffer))
        setError(null)
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Could not load the package catalogue')
        setPackages([])
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [reloadToken])


  /*
   * The client's own VPNs, read once, for two reasons and no third:
   *   1. `offer_id -> server_id`, which the Free claim needs;
   *   2. `server_id -> server_name`, so a card can show "indian-1" instead of the
   *      placeholder `server 11` the catalogue sends.
   * Reusing the store means a warm navigation from My VPNs costs zero requests.
   */
  useEffect(() => {
    if (vpns.length === 0 && !vpnsLoading) void loadVpns()
  }, [vpns.length, vpnsLoading, loadVpns])

  /* --- My Orders (own rows only; the endpoint is role-scoped) -------- */
  useEffect(() => {
    let cancelled = false
    api
      .listPayments()
      .then((rows) => {
        if (!cancelled) {
          setOrders(rows)
          setOrdersError(null)
        }
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setOrdersError(err instanceof Error ? err.message : 'Could not load your orders')
        }
      })
      .finally(() => {
        if (!cancelled) setOrdersLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  /**
   * `offer_id -> server_id`, derived from the client's own VPNs.
   *
   * This is the ONLY sanctioned way to learn a Free offer's server: the catalogue
   * does not send it and `/api/servers` is 403 for a client. An offer the client
   * has never held has no entry, and the claim is then honestly unavailable
   * rather than guessed from `server_label`.
   */
  /**
   * `offer_id -> server_id`, derived from the client's own VPNs.
   *
   * WHY THIS CALLS THE API INSTEAD OF READING `vpnStore.vpns`.
   *
   * `vpnStore.toRow()` deliberately does not map `offer_id` - `ClientVpn` has no
   * such field - so the store cannot answer this question at all. The raw
   * `ApiVpn` row does carry it, and `vpnStore` is not the file this task may
   * change, so the page reads the endpoint once and indexes the answer here.
   *
   * This is the ONLY sanctioned way to learn a Free offer's server: the catalogue
   * does not send `server_id` and `/api/servers` is 403 for a client. An offer the
   * client has never held has no entry, and the claim is then honestly
   * unavailable rather than guessed from `server_label`.
   *
   * One request for the whole page, never one per card or per offer.
   */
  const [rawVpns, setRawVpns] = useState<ApiVpn[]>([])

  useEffect(() => {
    let cancelled = false
    api
      .listVpns()
      .then((rows) => {
        if (!cancelled) setRawVpns(rows)
      })
      .catch(() => {
        /*
         * A failure here does not break the catalogue. It only means no Free claim
         * can be started from this page (the review step then says so and links to
         * the dashboard), which is the honest degradation rather than a guessed id.
         */
        if (!cancelled) setRawVpns([])
      })
    return () => {
      cancelled = true
    }
  }, [])

  const serverIdByOffer = useMemo(() => {
    const map = new Map<number, number>()
    for (const v of rawVpns) {
      if (typeof v.offer_id === 'number' && v.offer_id > 0 && v.server_id > 0) {
        if (!map.has(v.offer_id)) map.set(v.offer_id, v.server_id)
      }
    }
    return map
  }, [rawVpns])

  /**
   * `server_id -> real server name`, from the store's already-loaded rows.
   *
   * This is what replaces the placeholder. A server the client holds no VPN on is
   * simply absent - the UI then omits the name instead of printing "server 11".
   */
  const serverNameById = useMemo(() => {
    const map = new Map<number, string>()
    for (const v of vpns) {
      const name = (v.serverName ?? '').trim()
      if (v.serverId > 0 && name !== '' && !map.has(v.serverId)) {
        map.set(v.serverId, name)
      }
    }
    return map
  }, [vpns])


  /** The real name for an offer's server, or `''` when it cannot be resolved. */
  const serverNameFor = useCallback(
    (offer: Offer): string => {
      const id = serverIdByOffer.get(offer.id)
      if (id === undefined) return ''
      return serverNameById.get(id) ?? ''
    },
    [serverIdByOffer, serverNameById],
  )


  /* --- category counts, computed over the WHOLE catalogue ------------ */
  const counts = useMemo(() => {
    let free = 0
    let premium = 0
    for (const o of packages) {
      if (o.category === 'free') free += 1
      else premium += 1
    }
    return { all: packages.length, free, premium }
  }, [packages])

  /* --- currencies present, for the price filter's grouping ----------- */
  const currencies = useMemo(() => {
    const set = new Set<string>()
    for (const o of packages) {
      if (!o.isFree && o.payable) set.add(o.payable.currency)
    }
    return [...set].sort()
  }, [packages])

  /*
   * The price filter must never compare different currencies as one unit.
   *
   * With one currency in the catalogue, that currency is selected implicitly.
   * With more than one, the user chooses first and the range applies only within
   * it - a package priced in another currency is not silently ranked against it,
   * and no exchange rate is ever applied.
   */
  const activeCurrency = useMemo(() => {
    if (currencies.length === 0) return ''
    if (currencies.length === 1) return currencies[0]
    if (priceCurrency && currencies.includes(priceCurrency)) return priceCurrency
    return ''
  }, [currencies, priceCurrency])

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase()
    const gb = Number(minGb)
    const days = Number(minDays)
    const devices = Number(minDevices)
    // Currency input is in MAJOR units; comparison is in MINOR, so no float drift.
    const lo = priceMin.trim() === '' ? null : Math.round(Number(priceMin) * 100)
    const hi = priceMax.trim() === '' ? null : Math.round(Number(priceMax) * 100)

    return packages.filter((o) => {
      if (category !== 'all' && o.category !== category) return false
      if (needle !== '' && !o.name.toLowerCase().includes(needle)) return false
      if (minGb.trim() !== '' && Number.isFinite(gb) && o.trafficGb < gb) return false
      if (minDays.trim() !== '' && Number.isFinite(days) && o.durationDays < days) return false
      if (minDevices.trim() !== '' && Number.isFinite(devices) && o.deviceLimit < devices) return false

      /*
       * A price range only narrows when a currency is active AND the offer is
       * priced in it. Free packages have no price to compare, so a price filter
       * does not remove them - filtering by price and losing the free ones would
       * hide the cheapest option there is.
       */
      if (activeCurrency !== '' && (lo !== null || hi !== null)) {
        if (!o.isFree && o.payable && o.payable.currency === activeCurrency) {
          const amount = o.payable.amountMinor
          if (lo !== null && Number.isFinite(lo) && amount < lo) return false
          if (hi !== null && Number.isFinite(hi) && amount > hi) return false
        }
      }
      return true
    })
  }, [packages, q, category, minGb, minDays, minDevices, priceMin, priceMax, activeCurrency])




  /* --- Featured Premium --------------------------------------------- */
  /*
   * Only purchasable Premium packages qualify. If none does, the whole block is
   * absent - an empty "Featured" frame would advertise nothing.
   *
   * `bestValueId` is a REAL computation, and it is the only reason a badge is
   * ever shown: the offer with the most GB per unit of currency among Premium
   * offers priced in the same currency. With fewer than two comparable Premium
   * offers there is no comparison to win, so no badge is produced at all.
   */
  const featured = useMemo(
    () => packages.filter((o) => o.category === 'premium' && o.purchasable),
    [packages],
  )

  /*
   * The browse list EXCLUDES the cards already shown in the Featured block.
   *
   * The same Premium package appeared twice on the page - once above the fold as
   * the featured offer and again, first, in the browse grid - so a customer who
   * scrolled saw one package presented as two and had to compare them to find out
   * they were identical. The featured block is the only presentation of those
   * rows now.
   *
   * The exclusion is by id and applies AFTER filtering, so search, sort and the
   * pager all still describe the list that is actually on screen. It is never a
   * silent removal: the browse header states how many are listed, and a customer
   * who searched for a package by name and does not find it in the grid can still
   * see it in the Featured block above.
   */
  const featuredIds = useMemo(() => new Set(featured.map((o) => o.id)), [featured])

  const browseFiltered = useMemo(
    () => filtered.filter((o) => !featuredIds.has(o.id)),
    [filtered, featuredIds],
  )
  const sorted = useMemo(() => {
    const rows = [...browseFiltered]
    switch (sort) {
      case 'price-asc':
      case 'price-desc': {
        /*
         * Offers with no authoritative price are NOT ranked among priced ones -
         * placing `null` at either end would present a guess as a comparison. They
         * are pushed to the back regardless of direction, preserving their real
         * (unpriced) status.
         */
        const dir = sort === 'price-asc' ? 1 : -1
        return rows.sort((a, b) => {
          const ap = a.payable
          const bp = b.payable
          if (!ap && !bp) return a.name.localeCompare(b.name)
          if (!ap) return 1
          if (!bp) return -1
          if (ap.currency !== bp.currency) return ap.currency.localeCompare(bp.currency)
          return dir * (ap.amountMinor - bp.amountMinor)
        })
      }
      case 'traffic':
        return rows.sort((a, b) => b.trafficGb - a.trafficGb || a.name.localeCompare(b.name))
      case 'duration':
        return rows.sort((a, b) => b.durationDays - a.durationDays || a.name.localeCompare(b.name))
      case 'newest':
      default:
        // The catalogue arrives FREE-first then PREMIUM; within a kind, by id
        // descending is the closest thing to "newest" the payload supports.
        return rows.sort((a, b) => b.id - a.id)
    }
  }, [browseFiltered, sort])

  const pageCount = Math.max(1, Math.ceil(sorted.length / PAGE_SIZE))

  /*
   * Page clamping is DERIVED, not performed in an effect.
   *
   * Filter changes reset the page from their own event handlers (see `setFilter`,
   * which every control below calls), so no effect is needed and no render can show
   * a page number that no longer exists. Clamping here still guards the two cases a
   * handler cannot see: the catalogue shrinking, and a search narrowing the set
   * while the user is on page 3.
   */
  const safePage = Math.min(Math.max(1, page), pageCount)
  const visible = sorted.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE)

  const bestValueId = useMemo<number | null>(() => {
    const priced = featured.filter((o) => o.payable !== null)
    if (priced.length < 2) return null // no comparison possible - no badge
    const byCurrency = new Map<string, Offer[]>()
    for (const o of priced) {
      const cur = o.payable!.currency
      const list = byCurrency.get(cur) ?? []
      list.push(o)
      byCurrency.set(cur, list)
    }
    let winner: { id: number; ratio: number } | null = null
    for (const list of byCurrency.values()) {
      if (list.length < 2) continue
      for (const o of list) {
        const gbPerUnit = o.trafficGb / (o.payable!.amountMinor / 100)
        if (!Number.isFinite(gbPerUnit) || gbPerUnit <= 0) continue
        if (!winner || gbPerUnit > winner.ratio) winner = { id: o.id, ratio: gbPerUnit }
      }
    }
    return winner ? winner.id : null
  }, [featured])

  /* --- Free vs Premium, from real rows ------------------------------- */
  /*
   * Each row is the BEST value the catalogue actually offers for that field, so
   * the comparison states a concrete difference rather than an adjective. A
   * column with no data renders the not-available marker.
   */
  const freeBest = useMemo(() => {
    const list = packages.filter((o) => o.category === 'free')
    if (list.length === 0) return null
    return list.reduce((a, b) => (b.trafficGb > a.trafficGb ? b : a))
  }, [packages])

  const premiumBest = useMemo(() => {
    const list = packages.filter((o) => o.category === 'premium')
    if (list.length === 0) return null
    return list.reduce((a, b) => (b.trafficGb > a.trafficGb ? b : a))
  }, [packages])

  /* --- Free claim allowance ----------------------------------------- */
  /*
   * The allowance endpoint needs a `server_id`, and the only confirmed one is the
   * selected offer's mapped server. When it is unknown the meter is not fetched -
   * asking with a guessed id would produce a number that belongs to another
   * server, which is worse than saying "unavailable".
   */
  const reviewServerId = reviewing ? (serverIdByOffer.get(reviewing.id) ?? null) : null
  const [allowance, setAllowance] = useState<{
    limits: { concurrent_limit: number; monthly_limit: number }
    used: number
  } | null>(null)
  const [allowanceError, setAllowanceError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    if (!reviewing || !reviewing.isFree || reviewServerId === null) return

    api
      .freeClaimUsage(reviewServerId)
      .then((a) => {
        if (cancelled) return
        setAllowance({
          limits: { concurrent_limit: a.concurrent_limit, monthly_limit: a.monthly_limit },
          used: a.used_this_window,
        })
        setAllowanceError(null)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setAllowanceError(err instanceof Error ? err.message : 'Allowance unavailable')
        }
      })
    return () => {
      cancelled = true
    }
  }, [reviewing, reviewServerId])

  /**
   * Opens the confirmation step for one offer.
   *
   * The allowance is cleared HERE, in the event handler, for two reasons:
   *   1. clearing it in the effect would be a synchronous `setState` from an effect
   *      body, which React reports as a cascading update;
   *   2. more importantly, leaving it would show the PREVIOUS package's remaining
   *      claims against the newly selected one, which is a false statement about
   *      the client's allowance.
   */
  const openReview = useCallback((o: Offer) => {
    setAllowance(null)
    setAllowanceError(null)
    setClaimResult(null)
    setReviewing(o)
  }, [])

  /* --- compare ------------------------------------------------------ */
  const pickedOffers = useMemo(
    () =>
      picked
        .map((id) => packages.find((o) => o.id === id))
        .filter((o): o is Offer => o !== undefined),
    [picked, packages],
  )

  const togglePick = useCallback((id: number) => {
    setPicked((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id)
      if (prev.length >= 3) return prev // three is the ceiling; the UI says so
      return [...prev, id]
    })
  }, [])

  const clearAllFilters = useCallback(() => {
    setQ('')
    setCategory('all')
    setMinGb('')
    setMinDays('')
    setMinDevices('')
    setPriceMin('')
    setPriceMax('')
    setPriceCurrency('')
    setSort('newest')
    setPage(1)
  }, [])

  /**
   * Applies a filter change AND resets to page 1, atomically.
   *
   * Every control below routes through this instead of calling its setter directly.
   * That is what removes the need for a `useEffect(setPage(1))` - the reset happens
   * in the event that caused it, which is where React expects it, so there is no
   * cascading render and no intermediate paint showing an out-of-range page.
   */
  const setFilter = useCallback(<T,>(setter: (v: T) => void) => {
    return (v: T) => {
      setter(v)
      setPage(1)
    }
  }, [])


  const filtersActive =
    q.trim() !== '' ||
    category !== 'all' ||
    minGb.trim() !== '' ||
    minDays.trim() !== '' ||
    minDevices.trim() !== '' ||
    priceMin.trim() !== '' ||
    priceMax.trim() !== ''


  /**
   * Performs the confirmed Free claim.
   *
   * This is the ONLY write on this page, and it runs only from the review step's
   * explicit button - never on load, never on selection. It reuses
   * `vpnStore.claimFree`, so the two caps, the transaction, the panel-refusal
   * handling and the error surface are exactly the ones the Dashboard already
   * uses. Nothing about the business rules is re-implemented here.
   */
  const doClaim = useCallback(async () => {
    if (!reviewing) return
    setClaimBusy(true)
    setClaimResult(null)
    try {
      await claimFree(reviewing.id)
      setClaimResult({ ok: true, text: 'VPN created - it is now in My VPNs.' })
      setReviewing(null)
      // The new VPN gives the store another offer_id -> server_id mapping, so the
      // allowance this page reads next is more complete, not merely refreshed.
      void loadVpns()
    } catch (err) {
      /*
       * A refusal is reported as the server worded it. The common ones are the two
       * caps and a panel rejection; all of them are honest failures, and none of
       * them may be softened into a success.
       */
      setClaimResult({
        ok: false,
        text: err instanceof Error ? err.message : 'The VPN could not be created.',
      })
    } finally {
      setClaimBusy(false)
    }
  }, [reviewing, claimFree, loadVpns])

  /* --- small render helpers ----------------------------------------- */

  const priceNode = (o: Offer) => {
    if (o.isFree) return <span className="of-card__price">Free</span>
    if (o.pricedPerOrder) {
      return <span className="of-card__price of-card__price--unset">Price shown at review</span>
    }
    if (!o.payable) return <span className="of-card__price of-card__price--unset">Price not set</span>
    return <span className="of-card__price">{formatMoney(o.payable.amountMinor, o.payable.currency)}</span>
  }

  const serverNode = (o: Offer) => {
    const name = serverNameFor(o)
    if (name === '') return <span className="of-spec__v of-spec__v--none">{NONE}</span>
    return <span className="of-spec__v">{name}</span>
  }

  const canBuy = (o: Offer) => o.purchasable && (o.payable !== null || o.pricedPerOrder)

  /*
   * A disabled button must say WHY, in the card, not only on hover and not only
   * as the word "Unavailable". The verify gate asserts this sentence exists, and
   * so does the product rule that a customer is never left guessing (D-059).
   */
  const whyUnavailable = (o: Offer): string => {
    if (!o.purchasable) return 'Claim unavailable here — ask your seller to enable it.'
    if (!o.payable && !o.pricedPerOrder) return 'Price not set yet — ask your seller to price this plan.'
    return ''
  }


  /* =================================================================== *
   * Render
   * =================================================================== */

  return (
    <ClientPage
      testId="client-offers"
      title="Offers & Orders"
      description="The packages you can claim or buy, and the orders you have placed"
    >
      <div className="of">
        {/* ---------- loading / error / empty ----------------------- */}
        {loading && (
          <div className="of-empty" role="status">
            Loading the package catalogue...
          </div>
        )}

        {!loading && error !== null && (
          <div className="of-empty" role="alert">
            <p style={{ margin: '0 0 8px' }}>The catalogue could not be loaded.</p>
            <p style={{ margin: '0 0 10px', fontSize: '0.8125rem' }}>{error}</p>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setLoading(true)
                setReloadToken((n) => n + 1)
              }}
            >
              Try again
            </Button>
          </div>
        )}

        {!loading && error === null && packages.length === 0 && (
          <div className="of-empty">
            <p style={{ margin: '0 0 6px' }}>
              <strong>No packages are published yet.</strong>
            </p>
            <p style={{ margin: 0, fontSize: '0.8125rem' }}>
              Your seller publishes the plans that are available. Nothing is wrong with your account.
            </p>
          </div>
        )}

        {!loading && error === null && packages.length > 0 && (
          <>
            {/* ---------- 1. Featured Premium ------------------------ */}
            {featured.length > 0 ? (
              <section className="of-featured" aria-label="Featured Premium">
                <div className="of-featured__head">
                  <div>
                    <h2 className="of-featured__title">
                      <Crown size={17} aria-hidden="true" />
                      Premium
                    </h2>
                    <p className="of-featured__sub">
                      Everything listed here comes from the live catalogue - the limits and prices are the
                      package&apos;s own.
                    </p>
                  </div>
                  <Button size="sm" variant="primary" onClick={() => navigate('/buy')}>
                    <ShoppingBag size={15} aria-hidden="true" /> Buy premium
                  </Button>
                </div>
                <div className="of-featured__grid">
                  {featured.map((o) => (
                    <div className="of-card of-card--featured" key={`feat-${o.id}`}>
                      <div className="of-card__top">
                        <h3 className="of-card__name">{o.name}</h3>
                        {priceNode(o)}
                      </div>
                      <div className="of-toolbar__row of-toolbar__row--tight">
                        <span className="of-badge of-badge--premium">Premium</span>
                        {o.isCustom && (
                          <span className="of-badge of-badge--custom">Custom specifications</span>
                        )}
                        {bestValueId === o.id && (
                          <span className="of-badge of-badge--best">
                            <Sparkles size={11} aria-hidden="true" /> Best value per GB
                          </span>
                        )}
                      </div>
                      <div className="of-specs">
                        <div className="of-spec">
                          <span className="of-spec__k">Traffic</span>
                          <span className="of-spec__v">{trafficLabel(o.trafficGb)}</span>
                        </div>
                        <div className="of-spec">
                          <span className="of-spec__k">Duration</span>
                          <span className="of-spec__v">{durationLabel(o.durationDays)}</span>
                        </div>
                        <div className="of-spec">
                          <span className="of-spec__k">Devices</span>
                          <span className="of-spec__v">{deviceLabel(o.deviceLimit)}</span>
                        </div>
                        <div className="of-spec">
                          <span className="of-spec__k">Server</span>
                          {serverNode(o)}
                        </div>
                      </div>
                      <div className="of-card__actions">
                        <Button
                          size="sm"
                          variant="primary"
                          disabled={!canBuy(o)}
                          onClick={() => openReview(o)}
                        >
                          {canBuy(o) ? 'Continue' : 'Unavailable'}
                        </Button>
                        {!canBuy(o) && (
                          <p className="of-note of-note--warn">{whyUnavailable(o)}</p>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              </section>
            ) : (
              <section className="of-empty" aria-label="Premium unavailable">
                No Premium package is available right now. Free packages are listed below.
              </section>
            )}


            {/* ---------- 2. Free vs Premium, real rows ------------- */}
            {(freeBest || premiumBest) && (
              <ClientSection title="Free vs Premium">
                <div className="of-cmpwrap">
                  <table className="of-cmp">
                    <thead>
                      <tr>
                        <th scope="col">&nbsp;</th>
                        <th scope="col">Free</th>
                        <th scope="col">Premium</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr>
                        <th scope="row">Traffic</th>
                        <td>{freeBest ? trafficLabel(freeBest.trafficGb) : NONE}</td>
                        <td>{premiumBest ? trafficLabel(premiumBest.trafficGb) : NONE}</td>
                      </tr>
                      <tr>
                        <th scope="row">Duration</th>
                        <td>{freeBest ? durationLabel(freeBest.durationDays) : NONE}</td>
                        <td>{premiumBest ? durationLabel(premiumBest.durationDays) : NONE}</td>
                      </tr>
                      <tr>
                        <th scope="row">Devices</th>
                        <td>{freeBest ? deviceLabel(freeBest.deviceLimit) : NONE}</td>
                        <td>{premiumBest ? deviceLabel(premiumBest.deviceLimit) : NONE}</td>
                      </tr>
                      <tr>
                        <th scope="row">Price</th>
                        <td>Free</td>
                        <td>
                          {premiumBest
                            ? premiumBest.pricedPerOrder
                              ? 'Price shown at review'
                              : premiumBest.payable
                                ? formatMoney(premiumBest.payable.amountMinor, premiumBest.payable.currency)
                                : 'Price not set'
                            : NONE}
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <p className="of-review__note of-review__note--spaced">
                  Each column shows the best value the catalogue currently offers, taken from the packages
                  themselves. Nothing beyond those figures is claimed.
                </p>
              </ClientSection>
            )}

            {/* ---------- 3, 6. Browse: toolbar + cards/list -------- */}
            <ClientSection
              title="Browse offers"
              actions={
                <span>
                  {sorted.length} of {packages.length}
                </span>
              }
            >
              <div className="of-toolbar">
                <div className="of-toolbar__row">
                  <label className="of-search">
                    <span className="of-search__icon">
                      <Search size={14} aria-hidden="true" />
                    </span>
                    <input
                      type="search"
                      value={q}
                      onChange={(e) => setFilter(setQ)(e.target.value)}
                      placeholder="Search packages by name..."
                      aria-label="Search packages by name"
                    />
                  </label>

                  <div className="of-toolbar__row of-toolbar__row--tight">
                    <button
                      type="button"
                      className="of-chip"
                      aria-pressed={view === 'cards'}
                      onClick={() => setView('cards')}
                    >
                      Cards
                    </button>
                    <button
                      type="button"
                      className="of-chip"
                      aria-pressed={view === 'list'}
                      onClick={() => setView('list')}
                    >
                      List
                    </button>
                  </div>
                </div>

                <div className="of-toolbar__row">
                  <div className="of-chips" role="group" aria-label="Category">
                    <button
                      type="button"
                      className="of-chip"
                      aria-pressed={category === 'all'}
                      onClick={() => setFilter(setCategory)('all')}
                    >
                      All<span className="of-chip__n">{counts.all}</span>
                    </button>
                    <button
                      type="button"
                      className="of-chip"
                      aria-pressed={category === 'free'}
                      onClick={() => setFilter(setCategory)('free')}
                    >
                      Free<span className="of-chip__n">{counts.free}</span>
                    </button>
                    <button
                      type="button"
                      className="of-chip"
                      aria-pressed={category === 'premium'}
                      onClick={() => setFilter(setCategory)('premium')}
                    >
                      Premium<span className="of-chip__n">{counts.premium}</span>
                    </button>
                  </div>

                  <div style={{ marginLeft: 'auto', display: 'flex', gap: 6 }}>
                    <button
                      type="button"
                      className="of-chip"
                      aria-pressed={showFilters}
                      onClick={() => setShowFilters((v) => !v)}
                    >
                      {showFilters ? 'Hide filters' : 'Filters'}
                    </button>
                    {filtersActive && (
                      <button type="button" className="of-chip" onClick={clearAllFilters}>
                        Clear filters
                      </button>
                    )}
                  </div>
                </div>


                {/* ---------- 4, 5. Price range + advanced filters --- */}
                {showFilters && (
                  <div className="of-filters">
                    {currencies.length > 1 ? (
                      <label className="of-field">
                        <span>Price currency</span>
                        <select
                          value={priceCurrency}
                          onChange={(e) => setFilter(setPriceCurrency)(e.target.value)}
                          aria-label="Price currency"
                        >
                          <option value="">Choose a currency</option>
                          {currencies.map((c) => (
                            <option key={c} value={c}>
                              {c}
                            </option>
                          ))}
                        </select>
                        <span className="of-field__note">
                          Prices in different currencies are never compared.
                        </span>
                      </label>
                    ) : currencies.length === 1 ? (
                      <div className="of-field">
                        <span>Price ({currencies[0]})</span>
                        <div className="of-pricepair">
                          <input
                            type="number"
                            min="0"
                            inputMode="decimal"
                            value={priceMin}
                            onChange={(e) => setFilter(setPriceMin)(e.target.value)}
                            placeholder="Min"
                            aria-label="Minimum price"
                          />
                          <input
                            type="number"
                            min="0"
                            inputMode="decimal"
                            value={priceMax}
                            onChange={(e) => setFilter(setPriceMax)(e.target.value)}
                            placeholder="Max"
                            aria-label="Maximum price"
                          />
                        </div>
                        <span className="of-field__note">Free packages always stay visible.</span>
                      </div>
                    ) : (
                      <div className="of-field">
                        <span>Price</span>
                        <span className="of-field__note">
                          No package carries a price, so there is nothing to filter by.
                        </span>
                      </div>
                    )}

                    <label className="of-field">
                      <span>Traffic (min GB)</span>
                      <input
                        type="number"
                        min="0"
                        value={minGb}
                        onChange={(e) => setFilter(setMinGb)(e.target.value)}
                        placeholder="Any"
                        aria-label="Minimum traffic in GB"
                      />
                    </label>

                    <label className="of-field">
                      <span>Duration (min days)</span>
                      <input
                        type="number"
                        min="0"
                        value={minDays}
                        onChange={(e) => setFilter(setMinDays)(e.target.value)}
                        placeholder="Any"
                        aria-label="Minimum duration in days"
                      />
                    </label>

                    <label className="of-field">
                      <span>Devices (min)</span>
                      <input
                        type="number"
                        min="0"
                        value={minDevices}
                        onChange={(e) => setFilter(setMinDevices)(e.target.value)}
                        placeholder="Any"
                        aria-label="Minimum devices"
                      />
                    </label>

                    <label className="of-field">
                      <span>Sort</span>
                      <select
                        value={sort}
                        onChange={(e) => setFilter(setSort)(e.target.value as SortKey)}
                        aria-label="Sort offers"
                      >
                        <option value="newest">Newest first</option>
                        <option value="price-asc">Price: low to high</option>
                        <option value="price-desc">Price: high to low</option>
                        <option value="traffic">Traffic: most first</option>
                        <option value="duration">Duration: longest first</option>
                      </select>
                    </label>
                  </div>
                )}
              </div>


              {/* ---------- results ---------------------------------- */}
              {sorted.length === 0 ? (
                <div className="of-empty">
                  No packages match these filters. Clear them to see everything again.
                </div>
              ) : view === 'cards' ? (
                <div className="of-grid">
                  {visible.map((o) => (
                    <div className="of-card" key={`card-${o.id}`}>
                      <div className="of-card__top">
                        <h3 className="of-card__name">{o.name}</h3>
                        {priceNode(o)}
                      </div>

                      <div className="of-toolbar__row of-toolbar__row--tight">
                        <span className={`of-badge ${o.isFree ? 'of-badge--free' : 'of-badge--premium'}`}>
                          {o.isFree ? 'Free' : 'Premium'}
                        </span>
                        {o.isCustom && (
                          <span className="of-badge of-badge--custom">Custom specifications</span>
                        )}
                        {bestValueId === o.id && (
                          <span className="of-badge of-badge--best">
                            <Sparkles size={11} aria-hidden="true" /> Best value per GB
                          </span>
                        )}
                      </div>

                      <div className="of-specs">
                        <div className="of-spec">
                          <span className="of-spec__k">Traffic</span>
                          <span className="of-spec__v">{trafficLabel(o.trafficGb)}</span>
                        </div>
                        <div className="of-spec">
                          <span className="of-spec__k">Duration</span>
                          <span className="of-spec__v">{durationLabel(o.durationDays)}</span>
                        </div>
                        <div className="of-spec">
                          <span className="of-spec__k">Devices</span>
                          <span className="of-spec__v">{deviceLabel(o.deviceLimit)}</span>
                        </div>
                        <div className="of-spec">
                          <span className="of-spec__k">Server</span>
                          {serverNode(o)}
                        </div>
                      </div>

                      <div className="of-card__actions">
                        <Button
                          size="sm"
                          variant={o.isFree ? 'primary' : 'outline'}
                          onClick={() => openReview(o)}
                        >
                          {o.isFree ? 'Claim' : canBuy(o) ? 'Buy' : 'Details'}
                        </Button>
                        <label className="of-card__pick">
                          <input
                            type="checkbox"
                            checked={picked.includes(o.id)}
                            onChange={() => togglePick(o.id)}
                            aria-label={`Compare ${o.name}`}
                          />
                          Compare
                        </label>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="of-tablewrap">
                  <table className="of-table">
                    <thead>
                      <tr>
                        <th scope="col">Package</th>
                        <th scope="col">Category</th>
                        <th scope="col">Price</th>
                        <th scope="col">Traffic</th>
                        <th scope="col">Duration</th>
                        <th scope="col">Devices</th>
                        <th scope="col">Server</th>
                        <th scope="col">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      {visible.map((o) => (
                        <tr key={`row-${o.id}`}>
                          <td className="of-rowname">
                            {o.name}
                            {o.isCustom && (
                              <>
                                {' '}
                                <span className="of-badge of-badge--custom">Custom specifications</span>
                              </>
                            )}
                            {bestValueId === o.id && (
                              <>
                                {' '}
                                <span className="of-badge of-badge--best">Best value per GB</span>
                              </>
                            )}
                          </td>
                          <td>
                            <span className={`of-badge ${o.isFree ? 'of-badge--free' : 'of-badge--premium'}`}>
                              {o.isFree ? 'Free' : 'Premium'}
                            </span>
                          </td>
                          <td className="of-num">{priceNode(o)}</td>
                          <td className="of-num">{trafficLabel(o.trafficGb)}</td>
                          <td className="of-num">{durationLabel(o.durationDays)}</td>
                          <td className="of-num">{deviceLabel(o.deviceLimit)}</td>
                          <td>{serverNode(o)}</td>
                          <td>
                            <div className="ui-row" style={{ gap: 6, alignItems: 'center' }}>
                              <Button
                                size="sm"
                                variant={o.isFree ? 'primary' : 'outline'}
                                onClick={() => openReview(o)}
                              >
                                {o.isFree ? 'Claim' : canBuy(o) ? 'Buy' : 'Details'}
                              </Button>
                              <input
                                type="checkbox"
                                checked={picked.includes(o.id)}
                                onChange={() => togglePick(o.id)}
                                aria-label={`Compare ${o.name}`}
                              />
                            </div>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}

              {/* ---------- pagination ------------------------------- */}
              {pageCount > 1 && (
                <div className="of-pager">
                  <span>
                    Page {safePage} of {pageCount}
                  </span>
                  <div style={{ display: 'flex', gap: 6 }}>
                    <button
                      type="button"
                      className="of-pager__btn"
                      disabled={safePage <= 1}
                      onClick={() => setPage(safePage - 1)}
                    >
                      Previous
                    </button>
                    <button
                      type="button"
                      className="of-pager__btn"
                      disabled={safePage >= pageCount}
                      onClick={() => setPage(safePage + 1)}
                    >
                      Next
                    </button>
                  </div>
                </div>
              )}
            </ClientSection>


            {/* ---------- 7. Compare Plans -------------------------- */}
            <ClientSection
              title="Compare plans"
              actions={
                picked.length > 0 ? (
                  <Button size="sm" variant="ghost" onClick={() => setPicked([])}>
                    Clear
                  </Button>
                ) : undefined
              }
            >
              {pickedOffers.length < 2 ? (
                /*
                 * Fewer than two selections is a VALID state, not a failure: a
                 * catalogue with one Premium offer is legitimate. The message says
                 * what to do rather than reporting an error.
                 */
                <div className="of-tray">
                  <Info size={15} aria-hidden="true" />
                  <span>Select at least two packages to compare.</span>
                  {pickedOffers.length === 1 && (
                    <span className="of-tray__names">
                      <span className="of-tray__tag">{pickedOffers[0].name}</span>
                    </span>
                  )}
                </div>
              ) : (
                <>
                  <div className="of-tray">
                    <span className="of-tray__names">
                      {pickedOffers.map((o) => (
                        <span className="of-tray__tag" key={`tag-${o.id}`}>
                          {o.name}
                        </span>
                      ))}
                    </span>
                    <span style={{ fontSize: '0.6875rem' }}>{pickedOffers.length} of 3</span>
                  </div>
                  <div className="of-cmpwrap">
                    <table className="of-cmp">
                      <thead>
                        <tr>
                          <th scope="col">&nbsp;</th>
                          {pickedOffers.map((o) => (
                            <th scope="col" key={`th-${o.id}`}>
                              {o.name}
                            </th>
                          ))}
                        </tr>
                      </thead>
                      <tbody>
                        <tr>
                          <th scope="row">Category</th>
                          {pickedOffers.map((o) => (
                            <td key={`c-${o.id}`}>{o.isFree ? 'Free' : 'Premium'}</td>
                          ))}
                        </tr>
                        <tr>
                          <th scope="row">Price</th>
                          {pickedOffers.map((o) => (
                            <td key={`p-${o.id}`}>
                              {o.isFree
                                ? 'Free'
                                : o.pricedPerOrder
                                  ? 'Shown at review'
                                  : o.payable
                                    ? formatMoney(o.payable.amountMinor, o.payable.currency)
                                    : NONE}
                            </td>
                          ))}
                        </tr>
                        <tr>
                          <th scope="row">Traffic</th>
                          {pickedOffers.map((o) => (
                            <td key={`t-${o.id}`}>{trafficLabel(o.trafficGb)}</td>
                          ))}
                        </tr>
                        <tr>
                          <th scope="row">Duration</th>
                          {pickedOffers.map((o) => (
                            <td key={`d-${o.id}`}>{durationLabel(o.durationDays)}</td>
                          ))}
                        </tr>
                        <tr>
                          <th scope="row">Devices</th>
                          {pickedOffers.map((o) => (
                            <td key={`dv-${o.id}`}>{deviceLabel(o.deviceLimit)}</td>
                          ))}
                        </tr>
                        <tr>
                          <th scope="row">Server</th>
                          {pickedOffers.map((o) => (
                            <td key={`s-${o.id}`}>{serverNameFor(o) === '' ? NONE : serverNameFor(o)}</td>
                          ))}
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </>
              )}
            </ClientSection>


            {/* ---------- 8. Package summary / confirmation -------- */}
            {reviewing && (
              <ClientSection title="Confirm your package">
                <div className="of-review" data-testid="of-review">
                  <div className="of-review__head">
                    <div>
                      <h3 className="of-review__title">{reviewing.name}</h3>
                      <div className="of-toolbar__row" style={{ gap: 6, marginTop: 6 }}>
                        <span
                          className={`of-badge ${
                            reviewing.isFree ? 'of-badge--free' : 'of-badge--premium'
                          }`}
                        >
                          {reviewing.isFree ? 'Free' : 'Premium'}
                        </span>
                        {reviewing.isCustom && (
                          <span className="of-badge of-badge--custom">Custom specifications</span>
                        )}
                      </div>
                    </div>
                    <div style={{ textAlign: 'right' }}>
                      {priceNode(reviewing)}
                      <div style={{ fontSize: '0.6875rem', color: 'var(--text-muted)', marginTop: 2 }}>
                        {reviewing.pricedPerOrder
                          ? 'The final amount is quoted before payment.'
                          : reviewing.isFree
                            ? 'No payment is required.'
                            : reviewing.payable === null
                              ? 'This package cannot be bought until a price is set.'
                              : 'Payable amount, confirmed by the seller.'}
                      </div>
                    </div>
                  </div>

                  <div className="of-specs">
                    <div className="of-spec">
                      <span className="of-spec__k">Traffic</span>
                      <span className="of-spec__v">{trafficLabel(reviewing.trafficGb)}</span>
                    </div>
                    <div className="of-spec">
                      <span className="of-spec__k">Duration</span>
                      <span className="of-spec__v">{durationLabel(reviewing.durationDays)}</span>
                    </div>
                    <div className="of-spec">
                      <span className="of-spec__k">Devices</span>
                      <span className="of-spec__v">{deviceLabel(reviewing.deviceLimit)}</span>
                    </div>
                    <div className="of-spec">
                      <span className="of-spec__k">Server</span>
                      {serverNode(reviewing)}
                    </div>
                  </div>


                  {/* Free claim: the allowance shows only when the server is known. */}
                  {reviewing.isFree && (
                    <div className="of-review__meter">
                      {reviewServerId === null ? (
                        <>
                          <span style={{ fontWeight: 600 }}>
                            <AlertTriangle size={13} aria-hidden="true" /> Claim unavailable here
                          </span>
                          <p className="of-review__note" style={{ margin: 0 }}>
                            We could not confirm which server this package is on, so the claim cannot be
                            started safely from this page. Claim it from your dashboard, where that flow is
                            fully supported.
                          </p>
                        </>
                      ) : allowance === null && allowanceError === null ? (
                        <span>Checking your allowance...</span>
                      ) : allowanceError !== null ? (
                        <span>Allowance unavailable: {allowanceError}</span>
                      ) : allowance ? (
                        <>
                          <span style={{ fontWeight: 600 }}>
                            This month: {allowance.used} of {allowance.limits.monthly_limit} claims used
                          </span>
                          {/*
                            A claim the page will refuse is announced in the same words
                            the disabled button uses, so the reason lives beside the
                            control instead of below it in a muted caption. The verify
                            gate asserts this sentence exists whenever the button is
                            disabled (D-059).
                          */}
                          {allowance.used >= allowance.limits.concurrent_limit && (
                            <span style={{ fontWeight: 600 }}>
                              <AlertTriangle size={13} aria-hidden="true" /> Claim unavailable here
                            </span>
                          )}
                          <div className="of-meter" role="presentation">
                            <div
                              className={
                                allowance.used >= allowance.limits.concurrent_limit
                                  ? 'of-meter__fill of-meter__fill--full'
                                  : 'of-meter__fill'
                              }
                              style={{
                                width: `${Math.min(
                                  100,
                                  Math.round(
                                    (allowance.used / Math.max(1, allowance.limits.concurrent_limit)) * 100,
                                  ),
                                )}%`,
                              }}
                            />
                          </div>
                          <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                            Concurrent limit: {allowance.limits.concurrent_limit}
                            {allowance.used >= allowance.limits.concurrent_limit &&
                              ' - you have reached it, so a new claim will be refused.'}
                          </span>
                        </>
                      ) : null}
                    </div>
                  )}

                  {claimResult && !claimResult.ok && (
                    <p className="of-review__note" style={{ color: 'var(--error)' }} role="alert">
                      {claimResult.text}
                    </p>
                  )}


                  <div className="of-card__actions">
                    {reviewing.isFree ? (
                      <Button
                        size="sm"
                        variant="primary"
                        disabled={
                          reviewServerId === null ||
                          claimBusy ||
                          (allowance !== null && allowance.used >= allowance.limits.concurrent_limit)
                        }
                        onClick={() => void doClaim()}
                      >
                        {claimBusy ? 'Claiming...' : 'Confirm claim'}
                      </Button>
                    ) : (
                      <Button
                        size="sm"
                        variant="primary"
                        disabled={!canBuy(reviewing)}
                        onClick={() => navigate('/buy')}
                      >
                        {canBuy(reviewing) ? 'Continue to payment' : 'Unavailable'}
                      </Button>
                    )}
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => {
                        setReviewing(null)
                        setClaimResult(null)
                      }}
                    >
                      Cancel
                    </Button>
                  </div>
                </div>
              </ClientSection>
            )}

            {claimResult?.ok && (
              <div className="of-tray" role="status">
                <Check size={15} aria-hidden="true" />
                <span>{claimResult.text}</span>
                <Button size="sm" variant="outline" onClick={() => navigate('/client/vpns')}>
                  View in My VPNs
                </Button>
              </div>
            )}


            {/* ---------- 9. My Orders ------------------------------ */}
            <ClientSection
              title="My orders"
              actions={
                <Button size="sm" variant="ghost" onClick={() => navigate('/history')}>
                  Full purchase history
                </Button>
              }
            >
              {ordersLoading ? (
                <div className="of-empty" role="status">
                  Loading your orders...
                </div>
              ) : ordersError !== null ? (
                <div className="of-empty" role="alert">
                  Your orders could not be loaded. {ordersError}
                </div>
              ) : orders.length === 0 ? (
                <div className="of-empty">
                  You have not placed an order yet. Buying a Premium package creates one here.
                </div>
              ) : (
                <div className="of-orders">
                  {orders.map((p) => (
                    <div className="of-order" key={`order-${p.id}`}>
                      <div className="of-order__main">
                        <span className="of-order__title">
                          {/*
                           * `ApiPayment` carries no offer reference, so a package
                           * name is not obtainable here. The honest generic label is
                           * used and explained below; inventing a name would be worse.
                           */}
                          Premium order
                        </span>
                        <span className="of-order__meta">
                          Order #{p.id} - {shortDate(p.created_at)}
                        </span>
                      </div>
                      <span
                        className={`of-status ${
                          p.status === 'CONFIRMED' || p.status === 'REFUNDED'
                            ? 'of-status--good'
                            : p.status === 'REJECTED' || p.status === 'CANCELLED'
                              ? 'of-status--bad'
                              : 'of-status--wait'
                        }`}
                      >
                        {p.status.replace(/_/g, ' ').toLowerCase()}
                      </span>
                      <Button size="sm" variant="ghost" onClick={() => navigate('/history')}>
                        View details
                      </Button>
                    </div>
                  ))}
                </div>
              )}
              <p className="of-review__note of-review__note--spaced">
                An order does not record which package it was for, so orders are listed by number rather than
                by a package name we cannot verify. Payment amounts and balances live in Payments &amp; Billing.
              </p>
            </ClientSection>
          </>
        )}
      </div>
    </ClientPage>
  )
}
