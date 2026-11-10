import { create } from 'zustand'
import { api, ApiError, type ApiPayment, type ApiPaymentMethod, type ApiPaymentStatus } from '../lib/api'

/**
 * Payments — backed by the real backend (`/api/payments`, P6).
 *
 * Phase 5: this store held two invented records and let any screen mark a
 * payment "confirmed" locally. The backend now owns the state machine
 * (PENDING → AWAITING_CONFIRMATION → CONFIRMED | REJECTED, plus CANCELLED and
 * REFUNDED), so the UI's job is to *offer the legal transition* and show the
 * server's answer — never to decide one.
 *
 * Statuses map 1:1 onto the wire values (no prettified local enum): a UI that
 * renames them drifts from the audit trail the moment somebody reads it.
 */

export type PaymentStatus = ApiPaymentStatus

export interface Payment {
  /** Local UI identity (string); the backend's numeric id lives in `apiId`. */
  id: string
  apiId: number
  payerUserId: number
  /** Integer cents — money is never a float, on either side of the wire. */
  amountCents: number
  currency: string
  reference: string
  status: PaymentStatus
  methodId: number | null
  assignmentId: number | null
  /** Unix seconds. Format with `relativeTime`/`toLocaleString` at render time. */
  createdAt: number
  updatedAt: number
  confirmedAt: number | null
  confirmedBy: number | null
  /**
   * Identities carried on the row itself (D-113).
   *
   * These exist because `payerUserId` alone could not answer "whose order is
   * this?" — the one question an operator opens the list to answer. `''` means
   * the value is genuinely absent (account gone, manual record, deleted plan),
   * and every renderer must SAY that rather than show a blank.
   */
  payerEmail: string
  targetEmail: string
  offerName: string
  offerKind: string
}

/** A destination the payer transfers money to. `global` = platform-wide. */
export interface PaymentMethod {
  id: number
  kind: ApiPaymentMethod['kind']
  label: string
  details: string
  instructions: string
  global: boolean
  active: boolean
}

interface PaymentsState {
  payments: Payment[]
  methods: PaymentMethod[]
  loading: boolean
  busy: number | null
  error: string | null
  /**
   * How many orders exist in total, beyond the capped page (D-116).
   *
   * `-1` means the count could not be taken — rendered as "the total could not
   * be read", never as zero. A screen that shows 200 of an unknown number is
   * honest; one that shows 200 of 0 is not.
   */
  totalOrders: number
  /** True when older orders exist beyond what `payments` holds. */
  truncated: boolean

  load: (pendingOnly?: boolean) => Promise<void>
  loadMethods: () => Promise<void>
  /** Publishes a destination: global for a super admin, own for an admin. */
  createMethod: (input: {
    kind: PaymentMethod['kind']
    label: string
    details?: string
    instructions?: string
  }) => Promise<PaymentMethod>
  createPayment: (input: {
    amountCents: number
    methodId?: number
    assignmentId?: number
    reference?: string
    /** The plan being bought. Required for a real order: without it the payment
     *  is a deposit and confirmation builds no VPN (D-031.1). */
    offerId?: number
    quantity?: number
  }) => Promise<Payment>
  attachReference: (apiId: number, reference: string) => Promise<void>
  /** Operator actions. The server refuses an illegal transition with 409. */
  confirm: (apiId: number) => Promise<void>
  reject: (apiId: number) => Promise<void>
  refund: (apiId: number) => Promise<void>
  cancel: (apiId: number) => Promise<void>
  /** Shared runner: optimistic lock, error mapping, reload on refusal. */
  runTransition: (apiId: number, fn: () => Promise<ApiPayment>) => Promise<void>
}

function toRow(p: ApiPayment): Payment {
  return {
    id: String(p.id),
    apiId: p.id,
    payerUserId: p.payer_user_id,
    amountCents: p.amount_cents,
    currency: p.currency,
    reference: p.reference,
    status: p.status,
    methodId: p.method_id,
    assignmentId: p.assignment_id,
    createdAt: p.created_at,
    updatedAt: p.updated_at,
    confirmedAt: p.confirmed_at,
    confirmedBy: p.confirmed_by,
    // `?? ''` rather than omitting: `Payment` declares these non-optional so a
    // renderer must handle absence explicitly instead of silently printing
    // `undefined` into the page.
    payerEmail: p.payer_email ?? '',
    targetEmail: p.target_email ?? '',
    offerName: p.offer_name ?? '',
    offerKind: p.offer_kind ?? '',
  }
}

function toMethod(m: ApiPaymentMethod): PaymentMethod {
  return {
    id: m.id,
    kind: m.kind,
    label: m.label,
    details: m.details,
    instructions: m.instructions,
    global: m.owner_user_id === null,
    active: m.active,
  }
}

function message(err: unknown): string {
  if (err instanceof ApiError) {
    // Name the one refusal an operator will actually hit, so the UI can explain
    // it instead of showing "something went wrong".
    if (err.code === 'illegal_transition') {
      return 'That payment has already been reviewed and cannot change again.'
    }
    return err.message
  }
  return 'Unexpected error'
}


/** Which transitions this status still allows — mirrors the server's guards. */
export function allowedTransitions(status: PaymentStatus): Array<'confirm' | 'reject' | 'refund' | 'cancel'> {
  switch (status) {
    case 'PENDING':
      return ['confirm', 'reject', 'cancel']
    case 'AWAITING_CONFIRMATION':
      return ['confirm', 'reject']
    case 'CONFIRMED':
      return ['refund']
    default:
      return []
  }
}

export const usePaymentsStore = create<PaymentsState>()((set, get) => ({
  payments: [],
  methods: [],
  loading: false,
  busy: null,
  error: null,
  totalOrders: 0,
  truncated: false,

  load: async (pendingOnly = false) => {
    set({ loading: true, error: null })
    try {
      // The envelope carries the uncapped total, so the screen can say how many
      // orders exist beyond the newest page instead of implying there are only
      // the ones it can show (D-116).
      const page = await api.listPaymentsPaged(pendingOnly)
      set({
        payments: page.payments.map(toRow),
        totalOrders: page.total,
        truncated: page.truncated,
        loading: false,
      })
    } catch (err) {
      set({ loading: false, error: message(err) })
    }
  },

  loadMethods: async () => {
    try {
      const rows = await api.listPaymentMethods()
      set({ methods: rows.map(toMethod) })
    } catch (err) {
      set({ error: message(err) })
    }
  },

  createMethod: async (input) => {
    const created = toMethod(await api.createPaymentMethod(input))
    set((s) => ({ methods: [...s.methods, created] }))
    return created
  },

  createPayment: async (input) => {
    const created = await api.createPayment(input)
    const row = toRow(created)
    set((s) => ({ payments: [row, ...s.payments] }))
    return row
  },

  attachReference: async (apiId, reference) => {
    const updated = toRow(await api.attachPaymentReference(apiId, reference))
    set((s) => ({ payments: s.payments.map((p) => (p.apiId === apiId ? updated : p)) }))
  },

  confirm: (apiId) => get().runTransition(apiId, () => api.confirmPayment(apiId)),
  reject: (apiId) => get().runTransition(apiId, () => api.rejectPayment(apiId)),
  refund: (apiId) => get().runTransition(apiId, () => api.refundPayment(apiId)),
  cancel: (apiId) => get().runTransition(apiId, () => api.cancelPayment(apiId)),

  runTransition: async (apiId, fn) => {
    set({ busy: apiId, error: null })
    try {
      const updated = toRow(await fn())
      set((s) => ({
        busy: null,
        payments: s.payments.map((p) => (p.apiId === apiId ? updated : p)),
      }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      // Reload: the refusal may mean this tab held a stale status, so the
      // server's row is the thing to show.
      await get().load()
      throw err
    }
  },
}))
