import { create } from 'zustand'
import {
  api,
  ApiError,
  type ApiVpn,
  type ApiVpnLinks,
  type ApiVpnPing,
  type ApiVpnRelease,
  type ApiVpnUsage,
} from '../lib/api'

/**
 * Client VPNs — backed by the real Go backend (`/api/vpns`, Stage E4).
 *
 * Phase 5: this store previously returned two seeded fake VPNs and fabricated
 * `vless://demo-…` links. Both are gone. Every field below now comes from the
 * server, and anything the backend cannot confirm is `null` rather than a
 * plausible-looking placeholder.
 *
 * What is deliberately NOT here:
 *  - `link` / QR: `GET /clients/links/{email}` is Stage E6. Until then the UI
 *    must say "not issued yet", never show a working-looking fake token.
 *  - `purchases`: there is no payment API yet (P6).
 */

export type VpnStatus =
  | 'PENDING'
  | 'ACTIVE'
  | 'TRAFFIC_DEPLETED'
  | 'EXPIRED'
  | 'SUSPENDED'
  | 'SERVER_UNAVAILABLE'
  | 'REVOCATION_PENDING'
  | 'REVOKED'
  | 'REMOVED_FROM_CATALOG'
  // D-027's 3-day undo window. Added with the backend status (migration 0018):
  // without it a released VPN renders as an unknown state, and the console could
  // not offer "Restore" on the one row where it is still possible.
  | 'RELEASED'

/** Free / Premium / untagged — the split the client screen must show (D-032). */
export type VpnKind = 'FREE' | 'PREMIUM' | 'UNTAGGED'

export interface ClientVpn {
  /** Local UI identity (string); the backend's numeric id lives in `apiId`. */
  id: string
  apiId: number
  /** Panel email — the only stable human label the backend exposes today. */
  name: string
  panelEmail: string
  serverId: number
  /** The operator's own label for the server ("indian-1"); empty if unlabelled. */
  serverName: string
  status: VpnStatus
  ownerId: number
  sellerId: number
  /**
   * Who sold it (Bible §2.3) — the seller's display name and configured
   * contacts. Empty strings mean "no seller was recorded", which the screen
   * states plainly rather than guessing at an identity.
   */
  sellerName: string
  sellerEmail: string
  sellerWhatsApp: string
  sellerTelegram: string
  expiresAt: number | null
  createdAt: number
  /** Connection link — always null until Stage E6 issues a real one. */
  link: null
  /**
   * The package kind this VPN came from (`offer_kind` on the wire).
   *
   * `UNTAGGED` is a real third state, not a missing value: an untagged panel
   * client is visible to the Super Admin alone, so a client never receives one.
   * Before this field existed the client screen had no way to separate Free from
   * Premium and drew every card identically.
   */
  kind: VpnKind
  /** The package's name, when the row names one ("Phase A Premium"). */
  offerName: string | null
  /** True once a customer holds it (D-037). */
  assigned: boolean
  /**
   * When the customer put this VPN to sleep, or `null` when it is awake (D-094).
   *
   * Dormant is a LOCAL preference, not a status: `status` keeps reporting what
   * the panel says. A dormant VPN is shown last, left out of "Needs attention"
   * and not polled — but it is never hidden and never deleted, because the
   * customer has to be able to find it again to wake it.
   */
  dormantAt: number | null
  /**
   * When this VPN entered its current status (D-098), or null if unknown.
   *
   * Shown beside a non-active status so "suspended" becomes "suspended 12 days
   * ago". A bare status word cannot tell a customer whether something happened
   * this morning or last quarter, and that gap is what a support call fills.
   */
  statusChangedAt: number | null
  /*
   * NO usage fields here, deliberately (D-088.1).
   *
   * `GET /api/vpns` - the endpoint that feeds this store for a client - does not
   * send `used_bytes`, `limit_bytes` or `usage_pct`. Those belong to
   * `ApiAdminVPNRow` (`/api/super/vpns`), a different endpoint for a different
   * role. An optional field that the wire never populates is worse than no field
   * at all: the next card that reads it renders `0` and claims a measurement
   * nobody took.
   *
   * Usage therefore stays where it already lives - the VPN detail screen, which
   * asks `GET /api/vpns/{id}/usage` for that one VPN.
   */
}

interface VpnState {
  vpns: ClientVpn[]
  loading: boolean
  error: string | null
  /** VPN id whose lifecycle action is in flight. */
  busy: number | null

  load: () => Promise<void>
  claimFree: (offerId: number) => Promise<number>
  assignPremium: (offerId: number, clientUserId: number) => Promise<number>
  suspend: (apiId: number) => Promise<void>
  resume: (apiId: number) => Promise<void>
  /** Puts a VPN to sleep for this customer only — never deletes or touches the panel (D-094). */
  dormant: (apiId: number) => Promise<void>
  /** Brings a dormant VPN back into view; it does not repair it (D-094). */
  wake: (apiId: number) => Promise<void>
  revoke: (apiId: number) => Promise<'REVOKED' | 'REVOCATION_PENDING'>
  loadUsage: (apiId: number) => Promise<ApiVpnUsage>
  /** The panel's connection URIs (step 11). Never cached: a stale credential
   *  would produce a link that silently stops working. */
  loadLinks: (apiId: number) => Promise<ApiVpnLinks>
  /** Server reachability (D-026) — labelled as such on the wire. */
  ping: (apiId: number) => Promise<ApiVpnPing>
  /**
   * Releases a depleted VPN (D-027). Opens a 3-day undo window; nothing is
   * deleted, so the VPN stays in the list marked as released.
   */
  release: (apiId: number, reason?: string) => Promise<ApiVpnRelease>
  /** Undo a release inside the window. */
  restoreRelease: (releaseId: number) => Promise<ApiVpnRelease>
  /**
   * Erase the local record of a VPN whose panel client is gone (D-027.3).
   *
   * Only valid for a VPN the panel no longer holds; the server refuses with
   * `vpn_still_on_panel` otherwise, and that refusal is surfaced rather than
   * swallowed — an operator who clicked "Remove record" on a VPN that still works
   * must be told why nothing happened.
   */
  removeAbsent: (apiId: number) => Promise<void>
  releases: ApiVpnRelease[]
  loadReleases: () => Promise<void>
}

function toRow(v: ApiVpn): ClientVpn {
  // `assigned` decides whether an unclassified row may read as UNTAGGED (D-063):
  // only a panel client nobody holds can. Derived once, so the kind and the
  // `assigned` flag cannot disagree.
  const assigned = v.assigned ?? v.owner_user_id > 0
  return {
    id: String(v.id),
    apiId: v.id,
    // The panel email is the only identifier the API returns; it is not a
    // marketing name, and pretending otherwise would be invented data.
    name: v.panel_email,
    panelEmail: v.panel_email,
    serverId: v.server_id,
    serverName: v.server_name ?? '',
    status: v.status as VpnStatus,
    ownerId: v.owner_user_id,
    sellerId: v.seller_user_id,
    sellerName: v.seller_name ?? '',
    sellerEmail: v.seller_email ?? '',
    sellerWhatsApp: v.seller_whatsapp ?? '',
    sellerTelegram: v.seller_telegram ?? '',
    expiresAt: v.expires_at,
    createdAt: v.created_at,
    link: null,
    /*
     * `offer_kind` decides Free vs Premium. `tag` is the Super Admin's separate
     * FREE/PREMIUM classification of the panel client; when a row carries neither
     * it is untagged, which is a fact about the row rather than a default.
     *
     * `||`, not `??` (D-063). The server sends `offer_kind: ""` — not null — for a
     * VPN with no package, and `"" ?? x` does NOT fall back because an empty
     * string is not nullish. That single character is why an assigned, locked
     * PREMIUM VPN was displayed as UNTAGGED and pushed into "Other VPNs" while
     * "My Premium VPNs" claimed the customer had none.
     */
    kind: toKind(v.offer_kind || v.tag, assigned),
    offerName: v.offer_name ?? null,
    assigned,
    dormantAt: v.dormant_at ?? null,
    statusChangedAt: v.status_changed_at ?? null,
  }
}

/**
 * Normalises the server's `FREE` / `PREMIUM` / null into the UI's three states.
 *
 * `assigned` is passed in so an owned VPN can never fall back to UNTAGGED: a
 * panel client somebody holds is not "untagged", it is premium (an assignment is
 * always a premium issue — D-063). UNTAGGED therefore means exactly one thing: a
 * panel client no Veylix account holds yet, which only the Super Admin ever sees.
 */
export function toKind(raw: string | null | undefined, assigned = false): VpnKind {
  const k = (raw ?? '').toUpperCase().trim()
  if (k === 'FREE') return 'FREE'
  if (k === 'PREMIUM') return 'PREMIUM'
  // An owned VPN with no classification is premium, never untagged.
  if (assigned) return 'PREMIUM'
  return 'UNTAGGED'
}

/**
 * A VPN's human title.
 *
 * A customer cannot read `panelEmail` ("vyx-23") or a bare `server_id`, so the
 * title is the package they actually bought. The panel identity is not lost —
 * it stays as technical detail on the detail screen — it is simply not the
 * headline of a product card.
 *
 * U1 (UI/UX defect board): an assignment made without a chosen catalogue offer
 * gets the backend's internal carrier `"Platform assignment"`
 * (`store/assignment.go:332`). That string is an implementation name for "this
 * issue is premium", not a product, and it used to reach a customer's card
 * verbatim. It is now treated as NO name, so the honest kind label is shown.
 */
const INTERNAL_CARRIER_NAMES = new Set(['platform assignment'])

export function vpnDisplayName(v: { offerName?: string | null; kind: VpnKind }): string {
  const pkg = (v.offerName ?? '').trim()
  if (pkg !== '' && !INTERNAL_CARRIER_NAMES.has(pkg.toLowerCase())) return pkg
  if (v.kind === 'FREE') return 'Free VPN'
  if (v.kind === 'PREMIUM') return 'Premium VPN'
  return 'Panel client'
}

/**
 * Where a VPN lives: the operator's own server label, or an explicit fallback.
 *
 * The fallback still says "server N" rather than inventing a city name — an
 * unlabelled server is a fact, and a made-up location would be a lie a customer
 * would act on.
 */
export function vpnServerLabel(v: { serverName?: string; serverId: number }): string {
  const name = (v.serverName ?? '').trim()
  return name !== '' ? name : `server ${v.serverId}`
}

/**
 * The panel email split into what a human can read and what support needs.
 *
 * A panel client is named `<account email>_<kind>_<n>` (D-034.1), e.g.
 * `nimal@gmail.com_paid_2`. That single string is the VPN's NAME and its
 * identity on the server (D-091.4), but rendered raw it reads as a machine
 * value. Splitting it lets a screen show a readable label and still hand the
 * exact string to a copy button — which is the string an operator will search
 * for on the panel.
 *
 * `suffix` is `null` when the name does not follow the scheme (a client created
 * directly in 3X-UI before it was assigned). That is not an error: the row is
 * still a VPN, and the screen shows the whole name rather than inventing a
 * split point.
 */
export interface PanelIdentity {
  /** The exact panel email — what the copy button must copy verbatim. */
  email: string
  /** The account part, e.g. `nimal@gmail.com`. Empty when unparseable. */
  account: string
  /** `free` / `paid` when the suffix was recognised. */
  kindLabel: 'Free' | 'Premium' | null
  /** The 1-based slot, e.g. 2 for `_paid_2`. */
  slot: number | null
}

export function panelIdentity(panelEmail: string): PanelIdentity {
  const email = (panelEmail ?? '').trim()
  if (email === '') return { email: '', account: '', kindLabel: null, slot: null }

  // Greedy match on the account so an email containing underscores still splits
  // at the LAST two segments, which is where the scheme puts them.
  const m = /^(.*)_(free|paid)_(\d+)$/i.exec(email)
  if (!m) return { email, account: email, kindLabel: null, slot: null }

  return {
    email,
    account: m[1],
    kindLabel: m[2].toLowerCase() === 'free' ? 'Free' : 'Premium',
    slot: Number(m[3]),
  }
}

/**
 * What to tell a customer when their VPN is not `ACTIVE` (D-091.5).
 *
 * One sentence per status, because "SUSPENDED" and "TRAFFIC_DEPLETED" need
 * different next steps. This is the single source for those sentences: the list,
 * the quick view and the detail page all read from here, so the three screens
 * cannot drift into describing the same state three different ways.
 *
 * `ACTIVE` has no entry — a healthy VPN needs no explanation.
 *
 * An UNKNOWN status falls through to `GENERIC_STATUS_NOTE`. Inventing an
 * explanation for a state the backend never emitted would be worse than saying
 * we do not know it, because the customer would act on the invented one.
 */
export const NOT_CONNECTABLE: Partial<Record<VpnStatus, string>> = {
  SUSPENDED: 'Paused by your seller — nothing is deleted and it can be resumed.',
  TRAFFIC_DEPLETED: 'Your data allowance is used up. Premium packages have their own allowance.',
  EXPIRED: 'This VPN has expired. A new package is needed to reconnect.',
  SERVER_UNAVAILABLE:
    'Veylix cannot see this VPN on its server right now. Nothing has been deleted — it will show again once the server answers.',
  REVOCATION_PENDING: 'A removal has been requested and is waiting for the server to confirm it.',
  REVOKED: 'This VPN has been revoked.',
  RELEASED: 'You released this VPN. It stays undoable for 3 days, then only this record is removed.',
  REMOVED_FROM_CATALOG: 'This package is no longer offered. The VPN itself still exists.',
  PENDING: 'Still being set up on the server — this page updates when it finishes.',
}

/** The sentence for a status we do not recognise. Never an invented cause. */
export const GENERIC_STATUS_NOTE =
  'This VPN is not connectable right now. Open it for the current state, or contact your seller.'

/** The explanation for a non-active status; `null` for `ACTIVE`. */
export function statusNote(status: VpnStatus): string | null {
  if (status === 'ACTIVE') return null
  return NOT_CONNECTABLE[status] ?? GENERIC_STATUS_NOTE
}

function message(err: unknown): string {
  if (err instanceof ApiError) return err.message
  return 'Unexpected error'
}

export const useVpnStore = create<VpnState>()((set) => ({
  vpns: [],
  loading: false,
  error: null,
  busy: null,

  load: async () => {
    set({ loading: true, error: null })
    try {
      const rows = await api.listVpns()
      set({ vpns: rows.map(toRow), loading: false })
    } catch (err) {
      set({ loading: false, error: message(err) })
    }
  },

  claimFree: async (offerId) => {
    // The backend provisions on the panel and confirms with a fresh read, so a
    // returned id means the client really exists.
    const res = await api.createVpn(offerId)
    const refreshed = await api.listVpns()
    set({ vpns: refreshed.map(toRow) })
    return res.vpn_id
  },

  assignPremium: async (offerId, clientUserId) => {
    const res = await api.createVpn(offerId, clientUserId)
    const refreshed = await api.listVpns()
    set({ vpns: refreshed.map(toRow) })
    return res.vpn_id
  },

  suspend: async (apiId) => {
    set({ busy: apiId })
    try {
      await api.suspendVpn(apiId)
      set((s) => ({
        busy: null,
        vpns: s.vpns.map((v) => (v.apiId === apiId ? { ...v, status: 'SUSPENDED' } : v)),
      }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  resume: async (apiId) => {
    set({ busy: apiId })
    try {
      await api.resumeVpn(apiId)
      set((s) => ({
        busy: null,
        vpns: s.vpns.map((v) => (v.apiId === apiId ? { ...v, status: 'ACTIVE' } : v)),
      }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  /**
   * Puts a VPN to sleep (D-094).
   *
   * The local flag is set optimistically from the SERVER's answer, not invented:
   * the call is awaited first, and the row is only changed when it succeeded. A
   * failure leaves the VPN awake and visible, which is the safe direction — the
   * customer can retry, and nothing is hidden on the strength of a failed
   * request.
   *
   * `status` is deliberately untouched. Dormancy says nothing about whether the
   * VPN works, and copying it into status would let a dormant VPN claim to be
   * healthy.
   */
  dormant: async (apiId) => {
    set({ busy: apiId })
    try {
      await api.dormantVpn(apiId)
      const now = Math.floor(Date.now() / 1000)
      set((s) => ({
        busy: null,
        vpns: s.vpns.map((v) => (v.apiId === apiId ? { ...v, dormantAt: now } : v)),
      }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  /**
   * Wakes a VPN (D-094). Like `dormant`, this changes only the flag: if the
   * panel still cannot see the client, the status stays SERVER_UNAVAILABLE and
   * the screen says so. Waking makes a VPN visible again, not working again.
   */
  wake: async (apiId) => {
    set({ busy: apiId })
    try {
      await api.wakeVpn(apiId)
      set((s) => ({
        busy: null,
        vpns: s.vpns.map((v) => (v.apiId === apiId ? { ...v, dormantAt: null } : v)),
      }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  revoke: async (apiId) => {
    set({ busy: apiId })
    try {
      // Revocation is two-phase: it only reports REVOKED once the panel
      // confirms the client is gone. Anything else stays REVOCATION_PENDING.
      const res = await api.revokeVpn(apiId)
      const next = res.confirmed ? 'REVOKED' : 'REVOCATION_PENDING'
      set((s) => ({
        busy: null,
        vpns: s.vpns.map((v) => (v.apiId === apiId ? { ...v, status: next } : v)),
      }))
      return next
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  loadUsage: (apiId) => api.vpnUsage(apiId),
  loadLinks: (apiId) => api.vpnLinks(apiId),
  ping: (apiId) => api.vpnPing(apiId),

  release: async (apiId, reason) => {
    const row = await api.releaseVpn(apiId, reason)
    set((s) => ({ releases: [row, ...s.releases] }))
    return row
  },

  restoreRelease: async (releaseId) => {
    const row = await api.restoreRelease(releaseId)
    set((s) => ({ releases: s.releases.map((r) => (r.id === releaseId ? row : r)) }))
    return row
  },

  removeAbsent: async (apiId) => {
    set({ busy: apiId })
    try {
      await api.deleteVpn(apiId)
      // The row is gone on the server; drop it locally instead of refetching the
      // whole list, so the operator sees the row disappear immediately.
      set((s) => ({ busy: null, vpns: s.vpns.filter((v) => v.apiId !== apiId) }))
    } catch (err) {
      set({ busy: null, error: message(err) })
      throw err
    }
  },

  releases: [],

  loadReleases: async () => {
    try {
      set({ releases: await api.listReleases() })
    } catch (err) {
      set({ error: err instanceof ApiError ? err.message : 'Could not load releases' })
    }
  },
}))

/** Reads a VPN from the cache by its routing id. */
export function selectVpn(id: string | undefined) {
  if (!id) return undefined
  return useVpnStore.getState().vpns.find((v) => v.id === id || String(v.apiId) === id)
}
