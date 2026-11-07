import { create } from 'zustand'
import { api, ApiError, type ApiOrderEvent, type ApiPayment } from '../lib/api'

/**
 * Orders — the operator's queue (L2, D-031/D-036).
 *
 * An order *is* a payment row: confirming it is what builds the VPN, so the two
 * are not separate concepts and must never be presented as though they were.
 * This store adds the two things `/payments` cannot answer — the immutable
 * timeline and the block/unblock pair — because those belong to the order, not
 * to the money.
 *
 * Nothing here decides a state. Every action returns the server's row, and the
 * server refuses an illegal move with 409.
 */

/** One entry of the order's timeline, as both sides see it. */
export interface OrderEvent {
  id: number
  paymentId: number
  kind: string
  actorId: number | null
  actorEmail: string
  detail: string
  createdAt: number
}

function toEvent(e: ApiOrderEvent): OrderEvent {
  return {
    id: e.id,
    paymentId: e.payment_id,
    kind: e.kind,
    actorId: e.actor_id,
    actorEmail: e.actor_email,
    detail: e.detail,
    createdAt: e.created_at,
  }
}

function message(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'illegal_transition') {
      return 'That order has already moved on — the list now shows the server record.'
    }
    if (err.code === 'blocked') {
      return 'That VPN is already blocked.'
    }
    return err.message
  }
  return 'Unexpected error'
}

interface OrdersState {
  /** The timeline for one order, keyed by payment id. */
  events: Record<number, OrderEvent[]>
  /** Payment id currently loading its timeline. */
  loadingEvents: number | null
  /** Payment id currently mid-action, so its own buttons can disable. */
  busy: number | null
  error: string | null
  /** One order, for the L3 screen. */
  order: ApiPayment | null
  orderLoading: boolean
  orderError: string | null

  loadEvents: (paymentId: number) => Promise<void>
  eventsFor: (paymentId: number) => OrderEvent[]
  /** Reads a single order without a component holding the pending promise. */
  loadOrder: (paymentId: number) => Promise<void>
  resetOrder: () => void
  /** D-036.5 — block a VPN whose money has not arrived. */
  block: (paymentId: number, reason: string) => Promise<ApiPayment>
  unblock: (paymentId: number) => Promise<ApiPayment>
  /** The payer re-routes to another destination (D-036.4). */
  changeMethod: (paymentId: number, methodId: number) => Promise<ApiPayment>
  /** D-036.6 — pick the server + inbound a custom order is released onto. */
  setReleaseTarget: (paymentId: number, serverId: number, inboundId: number) => Promise<ApiPayment>
}

export const useOrdersStore = create<OrdersState>()((set, get) => ({
  events: {},
  loadingEvents: null,
  busy: null,
  error: null,
  order: null,
  orderLoading: false,
  orderError: null,

  loadOrder: async (paymentId) => {
    set({ orderLoading: true, orderError: null, order: null })
    try {
      const rows = await api.listPayments()
      const found = rows.find((p) => p.id === paymentId) ?? null
      set({
        orderLoading: false,
        order: found,
        orderError: found ? null : `Order #${paymentId} was not found.`,
      })
    } catch (err) {
      set({ orderLoading: false, order: null, orderError: message(err) })
    }
  },

  resetOrder: () => set({ order: null, orderLoading: false, orderError: null }),

  loadEvents: async (paymentId) => {
    set({ loadingEvents: paymentId, error: null })
    try {
      const rows = await api.paymentEvents(paymentId)
      set((s) => ({
        loadingEvents: null,
        events: { ...s.events, [paymentId]: rows.map(toEvent) },
      }))
    } catch (err) {
      set({ loadingEvents: null, error: message(err) })
    }
  },

  eventsFor: (paymentId) => get().events[paymentId] ?? [],

  block: async (paymentId, reason) => {
    set({ busy: paymentId, error: null })
    try {
      const updated = await api.blockPayment(paymentId, reason)
      set({ busy: null })
      // The block is a timeline entry, so make the operator see it.
      await get().loadEvents(paymentId)
      return updated
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  unblock: async (paymentId) => {
    set({ busy: paymentId, error: null })
    try {
      const updated = await api.unblockPayment(paymentId)
      set({ busy: null })
      await get().loadEvents(paymentId)
      return updated
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  changeMethod: async (paymentId, methodId) => {
    set({ busy: paymentId, error: null })
    try {
      const updated = await api.setPaymentMethod(paymentId, methodId)
      set({ busy: null })
      await get().loadEvents(paymentId)
      return updated
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  setReleaseTarget: async (paymentId, serverId, inboundId) => {
    set({ busy: paymentId, error: null })
    try {
      const updated = await api.setReleaseTarget(paymentId, serverId, inboundId)
      set({ busy: null })
      await get().loadEvents(paymentId)
      return updated
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },
}))
