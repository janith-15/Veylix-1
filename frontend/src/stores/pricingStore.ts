import { create } from 'zustand'
import { api, ApiError, type ApiPricingRow, type ApiQuote, type ApiSpec } from '../lib/api'

/**
 * Pricing — the rate card (D-035.4), backed by `GET/PUT /api/pricing`.
 *
 * Before this store existed the platform had no published price: a CUSTOM
 * order's amount could only come from whatever the buyer typed (M7-15/M7-08).
 * The rate card is what makes "price eka calculate wela pennanna one" true —
 * the server owns the arithmetic, and `quote()` below is the only path an
 * amount takes to the screen.
 *
 * Money is integer cents everywhere. It becomes a string only at render time.
 */

/** One rate scope: `serverId: null` is the platform default. */
export interface PricingRates {
  id: number
  serverId: number | null
  currency: string
  perGbCents: number
  perDayCents: number
  perDeviceCents: number
  perIpCents: number
  minPriceCents: number
  maxPriceCents: number
  maxTrafficGb: number
  maxDurationDays: number
  maxDevices: number
  maxIps: number
  updatedAt: number
}

/** The server's answer for one spec, line items included so it can be explained. */
export interface Quote {
  spec: ApiSpec
  currency: string
  trafficCents: number
  durationCents: number
  devicesCents: number
  ipsCents: number
  subtotalCents: number
  totalCents: number
  minimumApplied: boolean
}

/** What `PUT /api/pricing` accepts. Omitted fields are left untouched. */
export interface PricingInput {
  serverId?: number | null
  currency?: string
  perGbCents?: number
  perDayCents?: number
  perDeviceCents?: number
  perIpCents?: number
  minPriceCents?: number
  maxPriceCents?: number
  maxTrafficGb?: number
  maxDurationDays?: number
  maxDevices?: number
  maxIps?: number
}

function toRow(r: ApiPricingRow): PricingRates {
  return {
    id: r.id,
    serverId: r.server_id,
    currency: r.currency,
    perGbCents: r.rate_per_gb_cents,
    perDayCents: r.rate_per_day_cents,
    perDeviceCents: r.rate_per_device_cents,
    perIpCents: r.rate_per_ip_cents,
    minPriceCents: r.min_price_cents,
    maxPriceCents: r.max_price_cents,
    maxTrafficGb: r.max_traffic_gb,
    maxDurationDays: r.max_duration_days,
    maxDevices: r.max_devices,
    maxIps: r.max_ips,
    updatedAt: r.updated_at,
  }
}

function toQuote(q: ApiQuote): Quote {
  return {
    spec: q.spec,
    currency: q.currency,
    trafficCents: q.traffic_cents,
    durationCents: q.duration_cents,
    devicesCents: q.devices_cents,
    ipsCents: q.ips_cents,
    subtotalCents: q.subtotal_cents,
    totalCents: q.total_cents,
    minimumApplied: q.minimum_applied,
  }
}

function message(err: unknown): string {
  if (err instanceof ApiError) {
    // Name the two refusals an operator actually meets, so the screen explains
    // them instead of showing "something went wrong".
    if (err.code === 'pricing_not_set') {
      return 'No rates are published yet, so nothing can be priced. Set the platform default first.'
    }
    if (err.code === 'invalid_spec') {
      return err.message
    }
    return err.message
  }
  return 'Unexpected error'
}

interface PricingState {
  rates: PricingRates[]
  loading: boolean
  saving: boolean
  error: string | null
  /** The last quote, or null before one is asked for. */
  quote: Quote | null
  quoting: boolean
  quoteError: string | null

  load: () => Promise<void>
  save: (input: PricingInput) => Promise<PricingRates>
  /** Prices a CUSTOM spec. The buyer never supplies an amount. */
  priceSpec: (spec: ApiSpec, serverId?: number) => Promise<Quote | null>
  clearQuote: () => void
  /** The rate that will actually apply to a server (override, else default). */
  effectiveFor: (serverId: number | null) => PricingRates | null
}

export const usePricingStore = create<PricingState>()((set, get) => ({
  rates: [],
  loading: false,
  saving: false,
  error: null,
  quote: null,
  quoting: false,
  quoteError: null,

  load: async () => {
    set({ loading: true, error: null })
    try {
      const rows = await api.listPricing()
      set({ rates: rows.map(toRow), loading: false })
    } catch (err) {
      set({ loading: false, error: message(err) })
    }
  },

  save: async (input) => {
    set({ saving: true, error: null })
    try {
      const saved = toRow(await api.savePricing(input))
      set((s) => {
        // Replace in place when the scope already exists, otherwise append:
        // a scope is identified by its server, not by its row id.
        const exists = s.rates.some((r) => r.serverId === saved.serverId)
        const rates = exists
          ? s.rates.map((r) => (r.serverId === saved.serverId ? saved : r))
          : [...s.rates, saved]
        return { saving: false, rates }
      })
      return saved
    } catch (err) {
      set({ saving: false, error: message(err) })
      throw err
    }
  },

  priceSpec: async (spec, serverId = 0) => {
    set({ quoting: true, quoteError: null })
    try {
      const q = toQuote(await api.quoteOrder(spec, serverId))
      set({ quoting: false, quote: q })
      return q
    } catch (err) {
      // A refused spec is normal (a bound was exceeded), so it clears the quote
      // rather than leaving a stale price on screen for a spec that is invalid.
      set({ quoting: false, quote: null, quoteError: message(err) })
      return null
    }
  },

  clearQuote: () => set({ quote: null, quoteError: null }),

  effectiveFor: (serverId) => {
    const { rates } = get()
    if (serverId !== null) {
      const override = rates.find((r) => r.serverId === serverId)
      if (override) return override
    }
    return rates.find((r) => r.serverId === null) ?? null
  },
}))
