import { create } from 'zustand'
import { api, ApiError, type ApiOffer } from '../lib/api'

/**
 * Offer catalogue — backed by the real backend (`/api/offers`, P3).
 *
 * Phase A: this store used to hold three seeded, invented offers. Every row now
 * comes from the server, which also enforces the audience: a client is only
 * ever served active FREE offers, so the same store backs both the public
 * catalogue and the admin screen.
 *
 * What is deliberately NOT here:
 *  - `claims`: nothing counts redemptions yet. Showing "486 claims" would be an
 *    invented number; a real count needs a query over vpn_instances.
 *  - `inbound` label: the API returns `inbound_id` (a number). Resolving it to a
 *    protocol/remark needs the server's inbound cache, which the UI does not
 *    have on this screen yet.
 */

export interface VpnOffer {
  /** Local UI identity (string); the backend's numeric id lives in `apiId`. */
  id: string
  apiId: number
  kind: 'FREE' | 'PREMIUM'
  name: string
  serverId: number
  inboundId: number
  trafficLimitGB: number
  expiryDays: number
  deviceLimit: number
  /** Buyer-facing price in minor units (D-047). 0 means the seller set none. */
  priceCents: number
  currency: string
  active: boolean
  /**
   * Set once the offer was retired from the catalogue (P-4).
   *
   * Distinguished from `active:false`, which was the ONLY way to hide an offer
   * before: that said nothing about WHY, so a deliberately paused offer and a
   * retired one looked identical in every list.
   */
  archived: boolean
  createdAt: number
}

interface OffersState {
  offers: VpnOffer[]
  loading: boolean
  error: string | null
  /** Offer id whose update is in flight. */
  busy: number | null

  load: (activeOnly?: boolean) => Promise<void>
  /** Load the premium (resale) catalogue for the buyer (D-048). */
  loadPremium: () => Promise<void>
  addOffer: (input: {
    kind: 'FREE' | 'PREMIUM'
    name: string
    serverId: number
    inboundId?: number
    trafficLimitGB?: number
    expiryDays?: number
    deviceLimit?: number
    active?: boolean
  }) => Promise<VpnOffer>
  updateOffer: (
    apiId: number,
    patch: { name?: string; trafficLimitGB?: number; expiryDays?: number; deviceLimit?: number },
  ) => Promise<void>
  toggleActive: (apiId: number) => Promise<void>
  /** Retires an offer from the catalogue, keeping the row (P-4). */
  archiveOffer: (apiId: number) => Promise<void>
  /** Removes an offer nothing depends on (P-4). */
  deleteOffer: (apiId: number) => Promise<void>
}

function toRow(o: ApiOffer): VpnOffer {
  return {
    id: String(o.id),
    apiId: o.id,
    kind: o.kind,
    name: o.name,
    serverId: o.server_id,
    inboundId: o.inbound_id,
    // 0 keeps its documented meaning: unlimited / no expiry.
    trafficLimitGB: o.traffic_gb,
    expiryDays: o.duration_days,
    deviceLimit: o.device_limit,
    priceCents: o.price_cents ?? 0,
    currency: o.currency ?? '',
    active: o.active,
    archived: (o.archived_at ?? 0) > 0,
    createdAt: o.created_at,
  }
}

function message(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return 'Unexpected error'
}

export const useOffersStore = create<OffersState>()((set, get) => ({
  offers: [],
  loading: false,
  error: null,
  busy: null,

  load: async (activeOnly = false) => {
    set({ loading: true, error: null })
    try {
      const rows = await api.listOffers(activeOnly)
      set({ offers: rows.map(toRow), loading: false })
    } catch (err) {
      set({ loading: false, error: message(err) })
    }
  },

  /**
   * Load the resale catalogue for the Buy Premium screen (D-048).
   *
   * Uses `/api/offers/premium` rather than `load(true)`: a client's view of
   * `/api/offers` is FREE-only by D-024.3, so the buy page would otherwise always
   * be empty.
   */
  loadPremium: async () => {
    set({ loading: true, error: null })
    try {
      const rows = await api.listPremiumOffers()
      set({ offers: rows.map(toRow), loading: false })
    } catch (err) {
      set({ loading: false, error: message(err) })
    }
  },

  addOffer: async (input) => {
    const created = await api.createOffer({
      kind: input.kind,
      name: input.name,
      serverId: input.serverId,
      inboundId: input.inboundId,
      trafficGb: input.trafficLimitGB,
      durationDays: input.expiryDays,
      deviceLimit: input.deviceLimit,
      active: input.active,
    })
    const row = toRow(created)
    set((s) => ({ offers: [...s.offers, row] }))
    return row
  },

  updateOffer: async (apiId, patch) => {
    set({ busy: apiId })
    try {
      const updated = await api.updateOffer(apiId, {
        name: patch.name,
        trafficGb: patch.trafficLimitGB,
        durationDays: patch.expiryDays,
        deviceLimit: patch.deviceLimit,
      })
      const row = toRow(updated)
      set((s) => ({
        busy: null,
        offers: s.offers.map((o) => (o.apiId === apiId ? row : o)),
      }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  toggleActive: async (apiId) => {
    const current = get().offers.find((o) => o.apiId === apiId)
    if (!current) return
    set({ busy: apiId })
    try {
      const updated = await api.updateOffer(apiId, { active: !current.active })
      const row = toRow(updated)
      set((s) => ({
        busy: null,
        offers: s.offers.map((o) => (o.apiId === apiId ? row : o)),
      }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  /**
   * Retires an offer from the catalogue (P-4). The row stays, so anything that
   * references it keeps working — only its presence in the shop changes.
   */
  archiveOffer: async (apiId) => {
    set({ busy: apiId })
    try {
      const updated = await api.archiveOffer(apiId)
      const row = toRow(updated)
      set((s) => ({
        busy: null,
        offers: s.offers.map((o) => (o.apiId === apiId ? row : o)),
      }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  /**
   * Removes an offer nothing depends on (P-4).
   *
   * The row is dropped from the local list only AFTER the API confirms it; a 409
   * `offer_in_use` leaves the list untouched and rethrows, so the caller can tell
   * the operator to archive instead. Removing it optimistically would make the
   * screen disagree with the database.
   */
  deleteOffer: async (apiId) => {
    set({ busy: apiId })
    try {
      await api.deleteOffer(apiId)
      set((s) => ({ busy: null, offers: s.offers.filter((o) => o.apiId !== apiId) }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },
}))
