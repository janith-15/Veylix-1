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
