import { create } from 'zustand'
import { api, ApiError, type ApiPlan, type ApiPlanKind } from '../lib/api'

/**
 * Packages — the catalogue (L5, D-035).
 *
 * Three kinds, and the difference matters (D-035.2):
 *   FREE           — a public offer the Super Admin creates; a client claims it.
 *   PREMIUM        — a fixed template, owned by the Super Admin or a reseller.
 *   CUSTOM_PREMIUM — a template that says "state your own spec". Super Admin
 *                    only: a reseller never sells a customer-authored spec.
 *
 * The store keeps `listPlans` (what an operator manages) and `packages` (what
 * the caller may actually buy) separate, because conflating them is how a
 * reseller ends up seeing a package they cannot sell.
 */

export interface PackageInput {
  kind: ApiPlanKind
  name: string
  serverId: number
  inboundId: number
  trafficGb: number
  durationDays: number
  deviceLimit: number
  priceCents: number
  currency: string
  slotsTotal: number
  isCustom: boolean
}

function message(err: unknown): string {
  if (err instanceof ApiError) {
    // The two refusals an operator actually meets, named so the dialog can
    // explain them rather than showing a bare "request failed".
    if (err.code === 'forbidden') {
      return 'Only the Super Admin may create a CUSTOM PREMIUM package.'
    }
    if (err.code === 'uowned_plan' || err.code === 'plan_unowned') {
      return 'That package has no owner — assign it before it can be sold.'
    }
    return err.message
  }
  return 'Unexpected error'
}

interface PackagesState {
  /** Operator view: every package, including ones not purchasable. */
  plans: ApiPlan[]
  /** Buyer view: only what the caller may claim or buy. */
  packages: ApiPlan[]
  loading: boolean
  saving: boolean
  busy: number | null
  error: string | null

  load: () => Promise<void>
  create: (input: PackageInput) => Promise<ApiPlan>
  update: (
    id: number,
    input: {
      name?: string
      trafficGb?: number
      durationDays?: number
      deviceLimit?: number
      active?: boolean
      priceCents?: number
      currency?: string
      slotsTotal?: number
      ownerUserId?: number
      unowned?: boolean
    },
  ) => Promise<ApiPlan>
  /** Archive rather than delete: the sales history must survive (D-030.4). */
  archive: (id: number) => Promise<void>
  remove: (id: number) => Promise<void>
}

export const usePackagesStore = create<PackagesState>()((set, get) => ({
  plans: [],
  packages: [],
  loading: false,
  saving: false,
  busy: null,
  error: null,

  load: async () => {
    set({ loading: true, error: null })
    try {
      // The two lists come from different endpoints on purpose: one is the
      // operator's catalogue, the other is what this caller may buy.
      const [plans, packages] = await Promise.all([api.listPlans(), api.listPackages()])
      set({ plans, packages, loading: false })
    } catch (err) {
      set({ loading: false, error: message(err) })
    }
  },

  create: async (input) => {
    set({ saving: true, error: null })
    try {
      const created = await api.createPlan(input)
      set((s) => ({ saving: false, plans: [created, ...s.plans] }))
      await get().load()
      return created
    } catch (err) {
      set({ saving: false, error: message(err) })
      throw err
    }
  },

  update: async (id, input) => {
    set({ busy: id, error: null })
    try {
      const updated = await api.updatePlan(id, input)
      set((s) => ({
        busy: null,
        plans: s.plans.map((p) => (p.id === id ? updated : p)),
      }))
      await get().load()
      return updated
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  archive: async (id) => {
    set({ busy: id, error: null })
    try {
      await api.archivePlan(id)
      set({ busy: null })
      await get().load()
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  remove: async (id) => {
    set({ busy: id, error: null })
    try {
      await api.deletePlan(id)
      set((s) => ({ busy: null, plans: s.plans.filter((p) => p.id !== id) }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },
}))
