/**
 * Typed HTTP client for the Veylix Go backend.
 *
 * Security notes (docs/API_CONTRACT.md §0):
 *  - the session lives in an HttpOnly cookie, so nothing is stored in JS;
 *  - every request sends `credentials: 'include'`;
 *  - errors are surfaced as `ApiError` with the server's safe code/message.
 *
 * The backend base URL is configurable so `vite dev` can proxy to :8080 and a
 * production build can be served same-origin by the Go binary.
 */

const BASE = import.meta.env.VITE_API_BASE ?? ''

/**
 * The Google entry point (D-039).
 *
 * A full page navigation, not fetch: the server runs the whole OAuth exchange and
 * answers with a redirect, so no token ever reaches JavaScript.
 */
export const GOOGLE_START_URL = '/api/auth/google/start'

/** The account's own profile (D-041) — never contains a password or a hash. */
export interface ApiProfile {
  id: number
  email: string
  email_verified: boolean
  username: string
  display_name: string
  role: ApiRole
  status: string
  avatar_url: string
  phone: string
  whatsapp: string
  telegram: string
  discord: string
  birthday: string
  /** Derived by the server from `birthday`; 0 when no date is set. */
  age: number
  bio: string
  timezone: string
  google_linked: boolean
  has_password: boolean
  free_vpn_limit: number
  created_at: number
  last_login_at: number
  last_login_ip: string
}

/** The fields a person may change themselves. `email` is deliberately absent. */
export interface ApiProfilePatch {
  username?: string
  display_name?: string
  avatar_url?: string
  phone?: string
  whatsapp?: string
  telegram?: string
  discord?: string
  /** ISO YYYY-MM-DD; an empty string clears it. The server derives `age`. */
  birthday?: string
  age?: number
  bio?: string
  timezone?: string
}

export interface ApiEmailChangeRequest {
  sent: boolean
  /** "smtp" when a relay delivered it; "outbox" when none is configured. */
  delivery: 'smtp' | 'outbox'
  to: string
  expires_minutes: number
  /** Present only for a loopback operator when no relay is configured. */
  code?: string
}

// --- VPN transfer (D-064) ---------------------------------------------------

/**
 * One side of a VPN transfer (D-064).
 *
 * `seller_id` is who vouches for that holder: a reseller who sold it, or the
 * operator who issued it directly. It is kept per side because a transfer can
 * credit a different seller for the new holder than the old one had.
 */
export interface ApiTransferParty {
  user_id: number
  email: string
  name: string
  seller_id: number
}

/** One entry of a VPN's transfer history, as `GET /api/vpns/{id}/transfers` returns it. */
export interface ApiVpnTransfer {
  id: number
  vpn_id: number
  server_id: number
  panel_email: string
  from_user_id: number
  from_email: string
  to_user_id: number
  to_email: string
  actor_id: number
  actor_email: string
  offer_id: number
  kind: string
  reason: string
  free_claim_moved: boolean
  created_at: number
}

// --- Super Admin: client management (D-042) --------------------------------

/**
 * One row of the client console. Mirrors `store.ClientAdminRow` — `free_used` is
 * the FREE claims inside the rolling window, `premium_count` the assignments held.
 */
export interface ApiAdminClientRow {
  id: number
  email: string
  email_verified: boolean
  username: string
  display_name: string
  status: string
  avatar_url: string
  phone: string
  whatsapp: string
  telegram: string
  discord: string
  age: number
  google_linked: boolean
  free_vpn_limit: number
  free_used: number
  premium_count: number
  vpn_count: number
  seller_ids: number[]
  created_at: number
  last_login_at: number
  last_login_ip: string
}

/** The full record the client detail page renders. */
export interface ApiAdminClientDetail {
  profile: ApiProfile
  free_used: number
  /** The EFFECTIVE ceiling: the per-client override, else the platform default. */
  free_limit: number
  /** The raw override (0 = platform default). */
  free_limit_override: number
  free_window_days: number
  monthly_claims: { label: string; count: number }[]
  premium_count: number
  vpn_count: number
  seller_ids: number[]
  recent_audit: {
    id: number
    actor_id: number | null
    action: string
    target_type: string
    target_id: string
    result: string
    context: string
    created_at: number
  }[]
}

export interface ApiAdminClientPage {
  clients: ApiAdminClientRow[]
  total: number
  page: number
  limit: number
}

/** The filters the client console sends. Empty/undefined means "no restriction". */
export interface ApiClientQuery {
  q?: string
  status?: string
  from?: string
  to?: string
  free_min?: number
  free_max?: number
  has_premium?: boolean
  has_free?: boolean
  google?: boolean
  sort?: string
  page?: number
  limit?: number
}

// --- Super Admin: VPN management console (D-043) ---------------------------

/**
 * One row of the VPN console. It is the merged catalogue row plus the panel's own
 * facts. `read_error` means the panel could not be read on this request, so the
 * usage/expiry numbers are ABSENT rather than zero.
 */
export interface ApiAdminVPNRow {
  id: number
  server_id: number
  panel_email: string
  status: string
  owner_user_id: number
  seller_user_id: number
  expires_at: number | null
  created_at: number
  tag: string | null
  source: 'panel' | 'local'
  owner_email?: string
  owner_name?: string
  assigned: boolean
  offer_id?: number
  offer_kind?: string
  offer_name?: string
  usage_pct: number
  used_bytes: number
  limit_bytes: number
  protocol?: string
  transport?: string
  enabled: boolean
  limit_ip: number
  limit_hwid: number
  days_left: number
  read_error: boolean
}

export interface ApiSyncRun {
  servers: {
    server_id: number
    clients: number
    snapshots: number
    resets: number
    depleted: number
    expired: number
    absent: number
    unverified: number
    mirrored: number
    /** Present only when THIS server failed while the others succeeded. */
    error?: string
  }[]
  duration_ms: number
}

export interface ApiAdminVPNPage {
  vpns: ApiAdminVPNRow[]
  total: number
  /** Rows whose panel could not be read, or that the panel no longer holds. */
  unread: number
  /**
   * The At-a-glance state of the WHOLE catalogue, counted before any filter was
   * applied — so a tile keeps its number when a different tile is opened.
   *
   * Optional because an older server omits it; a screen must then say nothing
   * rather than show six zeroes.
   */
  summary?: ApiAdminVPNSummary
  /** The window the server actually served, echoed back. */
  limit: number
  offset: number
  /** True when rows remain after this window. */
  has_more: boolean
}

/**
 * The catalogue in one object: what is free, what is premium, what is waiting for
 * a decision, and what will generate a support ticket this week.
 *
 * The buckets overlap on purpose — a VPN can be `premium` AND `expiring_soon`
 * because those answer different questions.
 */
export interface ApiAdminVPNSummary {
  total: number
  free: number
  premium: number
  /** Waiting for the Super Admin's FREE/PREMIUM decision. */
  untagged: number
  assigned: number
  unassigned: number
  exhausted: number
  expiring_soon: number
  expired: number
  disabled: number
  /** Panel could not be read, so usage/expiry are unknown — not zero. */
  unread: number
}

/**
 * The Super Admin overview (D-107).
 *
 * Three rules are encoded in these types, and each one came from a decision:
 *
 *  1. **Money is NET** — confirmed minus refunded. `refund_cents` is reported
 *     separately so the screen can say what came back out rather than hiding it.
 *  2. **Currencies are never summed.** When `mixed_currency` is true the server
 *     deliberately answers `0` for every summed amount. Rendering that `0` would be
 *     a lie, so the UI must branch on the flag and show counts + the currency list.
 *  3. **A missing section is not a zero.** `errors` names the sections that failed
 *     to load; a section that is absent must not render as "0".
 */
export interface ApiSuperOverview {
  money: ApiPaymentTotals
  resellers: ApiResellerHealth
  /** Section names that failed to resolve. Empty on a healthy read. */
  errors: string[]
  generated_at: number
}

/** Money summary. Every amount is integer **cents** of `currency`. */
export interface ApiPaymentTotals {
  /** Empty when there are no payments yet, or when `mixed_currency` is true. */
  currency: string
  currencies: string[]
  /** More than one currency exists, so the summed amounts are 0 on purpose. */
  mixed_currency: boolean
  /** NET revenue inside the calendar month. */
  month_cents: number
  month_sales: number
  avg_ticket_cents: number
  /** Whole percent vs the previous calendar month; 0 when there was no prior revenue. */
  month_delta_pct: number
  /** Last six calendar months, oldest first, zero-filled where nothing sold. */
  months: { month: string; cents: number }[]
  /** NET revenue all time. */
  confirmed_cents: number
  pending_cents: number
  pending_count: number
  refund_cents: number
}

/** The reseller book, reduced to the facts an owner acts on. */
export interface ApiResellerHealth {
  total: number
  /** Signed in within the last 30 days. */
  active: number
  /** At >= 90% of the ceiling but not yet full. */
  quota_near: number
  quota_full: number
  /** No ACTIVE assignment created this month. */
  no_sales: number
  never_logged_in: number
  /** The five resellers closest to their ceiling, fullest first. */
  top: {
    id: number
    display_name: string
    email: string
    quota_used: number
    quota_limit: number
  }[]
}

/** The console's filters. Empty/undefined means "no restriction". */
export interface ApiAdminVPNQuery {
  tag?: string
  assigned?: string
  server_id?: number
  owner_id?: number
  seller_id?: number
  q?: string
  usage?: string
  expiry?: string
  limit?: number
  offset?: number
}

// --- Super Admin: staff (admin) console (D-044) ----------------------------

export interface ApiStaffRow {
  id: number
  email: string
  email_verified: boolean
  username: string
  display_name: string
  role: ApiRole
  status: string
  avatar_url: string
  quota_limit: number
  /** VPN assignments this reseller sold — the number the ceiling measures. */
  quota_used: number
  server_ids: number[]
  created_at: number
  last_login_at: number
  last_login_ip: string
}

export interface ApiStaffDetail {
  profile: ApiStaffRow
  clients: number
  vpns_sold: number
  payments: number
  servers: ApiServer[]
  recent_audit: ApiAdminClientDetail['recent_audit']
}

export interface ApiStaffPage {
  staff: ApiStaffRow[]
  total: number
  page: number
  limit: number
}

export interface ApiStaffQuery {
  q?: string
  role?: string
  status?: string
  from?: string
  to?: string
  quota_min?: number
  quota_max?: number
  server_id?: number
  sort?: string
  page?: number
  limit?: number
}

/** Maps the backend's snake_case role to the frontend's hyphenated role. */
export type ApiRole = 'super_admin' | 'admin' | 'client'

/**
 * The two sign-in doors (D-041). `/login` uses `client`; `/staff/login` uses
 * `staff` and accepts admin + super admin only. The server decides, not the UI.
 */
export type ApiPortal = 'client' | 'staff'

/**
 * One device registered against a VPN's device cap (`GET
 * /api/panel-clients/{server}/{email}/hwids`, D8).
 *
 * `fingerprint` is the panel's short label, not a hardware hash: 3X-UI never
 * exposes the full identifier, and Veylix must not invent one it cannot verify.
 */
export interface ApiHwidDevice {
  id: number
  fingerprint: string
  userAgent: string
  deviceOs: string
  osVersion: string
  deviceModel: string
  /** Unix seconds. */
  firstSeen: number
  /** Unix seconds — the panel's own timestamp for the last connection. */
  lastSeen: number
}

export interface ApiUser {
  id: number
  email: string
  display_name: string
  role: ApiRole
  email_verified?: boolean
  /** Present on `GET/POST/PATCH /api/admins` (auth.UserRow), absent on /api/me. */
  status?: string
  quota_limit?: number
  /**
   * Unix seconds. Present on /api/me since D2 (it was absent before, which is
   * why the profile screen could not say when the account was created).
   */
  created_at?: number
  /**
   * Set by /api/me when the account row could not be read. The caller is still
   * signed in — this only means the display name and creation date are unknown.
   */
  profile_read_error?: boolean
  /**
   * The IANA zone every screen must render an absolute instant in (D-099), e.g.
   * `Asia/Colombo`. Absent on responses other than /api/me, and absent entirely
   * from a server that has not been configured with one.
   *
   * It is a zone NAME, not an offset: `+05:30` would keep showing the wrong hour
   * if the zone's rules ever change, and an offset is exactly what drifts.
   */
  display_timezone?: string
}

/**
 * The answer to `POST /api/auth/register` (D-076).
 *
 * Either a signed-in user (an auto-verified account, i.e. an operator-created one)
 * or a **pending** result: the account exists, no session was issued, and a
 * confirmation is outstanding. The pending shape has no `id` or `role`, which is
 * why it is a union rather than an optional field — a caller cannot accidentally
 * read `result.role` and route a person into the app.
 */
export type ApiRegisterResult =
  | (ApiUser & { pending_verification?: false })
  | {
      pending_verification: true
      email: string
      email_verified: false
      /** "smtp" when a relay delivered the link, "outbox" when none is configured. */
      delivery: '' | 'smtp' | 'outbox'
      note?: string
    }

export interface ApiServer {
  id: number
  name: string
  panel_url: string
  status: string
  panel_version: string
  xray_version: string
  last_health_check: number | null
  last_sync?: number | null
  allow_private_network?: boolean
}

export interface ApiInbound {
  id: number
  remark: string
  tag: string
  protocol: string
  port: number
  enable: boolean
  network?: string
  security?: string
}

export interface ApiAuditEntry {
  id: number
  actor_id: number | null
  /** The actor's display name or e-mail; absent for system entries. */
  actor_name?: string
  action: string
  target_type: string
  target_id: string
  result: string
  context: string
  created_at: number
}

/** The audit read's server-side filters (D-082). All optional. */
export interface ApiAuditQuery {
  kind?: string
  actor_id?: number
  /** Unix seconds — created_at >= from. */
  from?: number
  /** Unix seconds — created_at <= to. */
  to?: number
  q?: string
  limit?: number
  offset?: number
}

/** One window over the trail. `total` counts matches AFTER the filters but
 *  BEFORE the window, so a page can say "37 matches" honestly. */
export interface ApiAuditPage {
  entries: ApiAuditEntry[]
  total: number
  limit: number
  offset: number
  has_more: boolean
}

export interface ApiQuota {
  used: number
  limit: number
  remaining: number
}

/**
 * A VPN as the backend's `GET /api/vpns` returns it (httpapi.VPNRow).
 *
 * Deliberately mirrors the server projection exactly — there is no `plan`
 * field, because the backend does not store one. The offer decides what a VPN
 * is, so the UI must not invent a Free/Premium label the server cannot confirm.
 */
export interface ApiVpn {
  id: number
  server_id: number
  panel_email: string
  status: string
  owner_user_id: number
  seller_user_id: number
  expires_at: number | null
  created_at: number
  /*
   * The server has always sent these; this interface used to drop them, so the
   * client screen could not separate Free from Premium (the owner's requirement)
   * and every VPN card looked identical.
   */
  /** `FREE` / `PREMIUM`; null means untagged (Super Admin only). */
  tag?: string | null
  /** `panel` (read from 3X-UI) or `local` (Veylix only). */
  source?: string
  /** Who holds it (D-037) — absent on a row nobody was assigned. */
  owner_email?: string
  owner_name?: string
  assigned?: boolean
  /** The package this VPN came from, so Free and Premium are separable (D-032). */
  offer_id?: number
  offer_kind?: string
  offer_name?: string
  /**
   * The operator's label for the server this VPN lives on ("indian-1"). Present
   * so a card can say where a VPN is, instead of printing a bare `server_id`
   * number that means nothing to a customer.
   */
  server_name?: string
  /**
   * Unix seconds when the customer put this VPN to sleep (D-094); absent or
   * null means awake.
   *
   * It is optional because the row is also produced by endpoints that predate
   * dormancy — an absent field reads as "awake", which is the behaviour every
   * row had before this existed.
   */
  dormant_at?: number | null
  /**
   * Unix seconds this row entered its current `status` (D-098). Absent or null
   * when it has never moved.
   *
   * A suspended VPN that can only say "suspended" leaves the customer asking the
   * one question this answers — since when? — and that becomes a support call.
   */
  status_changed_at?: number | null
  /*
   * Who sold it (Bible §2.3) — a client must be able to see the responsible
   * seller and reach them. Before these fields the screen could do no better
   * than "Sold by user #3". Values are the seller's own configured contacts,
   * the same ones the payment destination already shows a payer (D-036.2).
   */
  seller_name?: string
  seller_email?: string
  seller_whatsapp?: string
  seller_telegram?: string
}

/** One time-stamped usage observation (httpapi.UsagePoint). */
export interface ApiUsagePoint {
  taken_at: number
  up_bytes: number
  down_bytes: number
  /** P-01: the server states the total; the chart never adds up/down itself. */
  total_bytes: number
  lifetime_up_bytes: number
  lifetime_down_bytes: number
  lifetime_total_bytes: number
}

/**
 * The ranges the owner named (D-038 P-06), extended by D-092 with `24h` and a
 * free-form `custom` window driven by `since`/`until`. The backend answers
 * **422** for anything else rather than silently showing a different window.
 */
export type ApiUsageRange = '24h' | '7d' | '30d' | '3m' | '1y' | 'all' | 'custom'

/**
 * Which MEASUREMENT SOURCE a history read covers (D-095).
 *
 * There is exactly one today: the server measuring whether the VPN's panel
 * client answers (`server_reachability`). `all` means "every source".
 *
 * This is a real distinction rather than a decorative filter: the product must
 * never present the server's probe as the customer's own device latency
 * (D-026.2), so when a client-side probe is added it gets its own value here
 * instead of being merged into the same line.
 */
export type ApiUsageSource = 'server_reachability' | 'all'

/**
 * One hour of traffic movement (`GET /api/vpns/{id}/usage/hourly`, D-092).
 *
 * A DELTA, not a running total: `total_bytes` is what moved during that hour.
 * `measurements` is how many sync snapshots landed in it — the tick is ~15
 * minutes, so a full hour holds four and a single one is a gap worth naming.
 */
export interface ApiHourlyBucket {
  hour_start: number
  /** 0–23 in UTC, matching the bucket the rows were summed into. */
  hour_of_day: number
  up_bytes: number
  down_bytes: number
  total_bytes: number
  measurements: number
}

export interface ApiHourlyUsage {
  vpn_id: number
  window?: { key: ApiUsageRange; since_sec: number; until_sec: number }
  source: string
  buckets: ApiHourlyBucket[]
}

/**
 * One measured rate between two consecutive sync snapshots
 * (`GET /api/vpns/{id}/speed-history`, D-092).
 *
 * `source: 'unavailable'` means the interval cannot be measured (zero elapsed,
 * or a counter reset). It is NOT an idle link — zero would be the idle answer.
 */
export interface ApiSpeedPoint {
  taken_at: number
  up_bps: number
  down_bps: number
  elapsed_sec: number
  source: 'measured' | 'unavailable'
}

export interface ApiSpeedHistory {
  vpn_id: number
  window?: { key: ApiUsageRange; since_sec: number; until_sec: number }
  /** Constant label: an average between sync readings, never a speed test. */
  scope: 'average_between_sync_readings'
  points: ApiSpeedPoint[]
}

/**
 * One stored reachability probe (`GET /api/vpns/{id}/ping/history`, D-092).
 *
 * `latency_ms` is **-1** when the probe failed, and `reachable: false` is a
 * MEASUREMENT — the probe ran and the server did not answer.
 */
export interface ApiPingPoint {
  taken_at: number
  reachable: boolean
  latency_ms: number
  scope: string
}

export interface ApiPingHistory {
  vpn_id: number
  window?: { key: ApiUsageRange; since_sec: number; until_sec: number }
  /** Constant label the UI must display: server reachability, not own latency. */
  scope: 'server_reachability'
  /**
   * The measurement source the filter ACTUALLY applied (D-095): the named scope,
   * or `all`. The server echoes it so a chart cannot claim a filtered read covers
   * every source.
   */
  source?: 'server_reachability' | 'all'
  points: ApiPingPoint[]
}

/**
 * `GET /api/vpns/{id}/usage` (httpapi.VPNUsage).
 *
 * `history` is the backend being honest about what it actually has
 * (REQUIREMENTS §2.7 "never invent history"): `NONE` means no snapshot exists
 * yet, not "zero traffic".
 *
 * P-01: upload, download and TOTAL are three separate numbers, computed by the
 * server — a chart must not add them itself and risk disagreeing.
 * P-02: `usage_pct` / `remaining_bytes` answer "how far along is it"; a limit of
 * 0 means unlimited and `remaining_bytes` is **-1**, never "0 left".
 */
export interface ApiVpnUsage {
  vpn_id: number
  status: string
  limit_bytes: number
  lifetime_up_bytes: number
  lifetime_down_bytes: number
  lifetime_total_bytes: number
  usage_pct: number
  remaining_bytes: number
  /** Epoch seconds — the window the owner described as creation day → end day. */
  created_at: number
  expires_at: number | null
  remote_reset_count: number
  /** `PARTIAL` = snapshots exist but the window starts later than creation. */
  history: 'AVAILABLE' | 'NONE' | 'PARTIAL'
  first_snapshot_at: number | null
  /**
   * Echoed by the server so a chart can label its range without guessing.
   *
   * `bucket_sec` / `bucket` say how wide one plotted point is (D-096). Before
   * they existed the UI labelled an 18-hour slice "Last 30 days", because the
   * server returned the newest 240 raw rows for every range. A chart must label
   * its axis from these, never from the range key.
   */
  window?: {
    key: ApiUsageRange
    since_sec: number
    until_sec: number
    bucket_sec?: number
    bucket?: string
  }
  /** The width/name the points were REALLY plotted at, after any span sizing. */
  bucket_sec?: number
  bucket?: string
  points: ApiUsagePoint[]
}

/**
 * `GET /api/vpns/{id}/protocol` (P-09) — what the customer actually connects with.
 *
 * `source: 'unavailable'` means the panel could not be read; it must never be
 * rendered as "this VPN has no credential".
 */
export interface ApiVpnProtocol {
  vpn_id: number
  panel_email: string
  protocol: string
  transport: string
  security: string
  /** host:port */
  server: string
  uuid?: string
  password?: string
  flow?: string
  sub_id?: string
  remark?: string
  source: 'panel' | 'unavailable'
}

/**
 * `GET /api/vpns/{id}/traffic/live` (P-04) — a RATE, never a total.
 *
 * `source: 'unavailable'` means there are not yet two readings (or the counter
 * was reset). A screen must not draw 0 B/s as "idle" when it simply does not
 * know (D-026.3).
 */
export interface ApiVpnLiveTraffic {
  vpn_id: number
  up_bps: number
  down_bps: number
  measured_from: number
  measured_to: number
  connected_now: boolean | null
  source: 'sync' | 'unavailable'
}

/** `GET /api/sync/status` — per-server freshness. */
export interface ApiSyncServer {
  server_id: number
  name: string
  status: string
  last_sync: number | null
  stale: boolean
  age_seconds: number
}

export interface ApiClient {
  id: number
  email: string
  display_name: string
  status: string
  created_at: number
  vpn_count: number
}

/** `GET /api/servers/{id}/metrics` — server-wide CPU/RAM samples. */
export interface ApiMetricPoint {
  taken_at: number
  cpu_pct: number
  mem_used: number
  mem_total: number
}

/**
 * An offer (P3). `traffic_gb` / `duration_days` / `device_limit` of 0 mean
 * unlimited / no expiry / unlimited devices, per the schema.
 */
export interface ApiOffer {
  id: number
  kind: 'FREE' | 'PREMIUM'
  name: string
  server_id: number
  inbound_id: number
  traffic_gb: number
  duration_days: number
  device_limit: number
  /** What the buyer pays, in minor units (D-047). 0 means "not priced yet". */
  price_cents: number
  /** ISO code the price is quoted in, e.g. LKR. */
  currency: string
  active: boolean
  /**
   * Unix seconds when the offer was retired from the catalogue (P-4); `null` while
   * it is still listed. The backend reads this from `vpn_offers.archived_at`.
   */
  archived_at?: number | null
  created_at: number
  updated_at: number
}

/**
 * One entry of the CUSTOMER catalogue (`GET /api/packages`, D-035.2).
 *
 * WHY THIS TYPE EXISTS AND `ApiPlan` IS NOT USED HERE.
 *
 * `/api/packages` and `/api/plans` do not send the same shape, and the difference
 * is not cosmetic. Verified against the running server on 2026-11-08: the packages
 * payload carries exactly these eleven keys —
 *
 *   currency · device_limit · duration_days · id · kind · name · price_cents ·
 *   priced_per_order · purchasable · server_label · traffic_gb
 *
 * it does **not** send `active`, `archived` or `slots_remaining`, which `ApiPlan`
 * declares as required. Typing this endpoint as `ApiPlan` therefore handed callers
 * `undefined` for three fields while TypeScript reported success — a false type, not
 * a convenience. The rows here are already filtered to active packages by the
 * server, so `active` has no meaning on the wire and is not invented here.
 *
 * `kind` is widened to `string` on purpose: the handler rewrites a custom package to
 * `"CUSTOM"` before serialising (plan_routes.go), so a customer-facing screen can
 * receive a third value the `ApiPlanKind` union does not admit. The screen folds it
 * into Premium (S6 category rule) rather than trusting the discriminant.
 */
export interface ApiPackage {
  id: number
  /** `FREE`, `PREMIUM`, or `CUSTOM` for a package priced per order. */
  kind: string
  name: string
  traffic_gb: number
  duration_days: number
  device_limit: number
  price_cents: number
  /** Empty string when nobody priced the package — NOT the same as zero. */
  currency: string
  /** A coarse server identifier such as `server 11`; never a panel URL or token. */
  server_label: string
  /** Whether the caller may obtain it at all (active + has stock, server-side). */
  purchasable: boolean
  /** True for a package whose price is computed by `POST /api/orders/quote`. */
  priced_per_order: boolean
}

/** One release record (D-027). `panel_client_kept` is always true. */
export interface ApiVpnRelease {
  id: number
  vpn_id: number
  client_user_id: number
  reason: string
  released_at: number
  /** Epoch seconds — the undo button works until this passes. */
  restore_deadline: number
  restored_at: number | null
  removed_at: number | null
  panel_client_kept: boolean
}

/**
 * The result of replacing a VPN (D-112).
 *
 * Two panel clients exist afterwards: `new` is the freshly created one, and
 * `old_*` names the VPN that was released — not deleted, so its undo window is
 * still open. `claim_consumed` records what a FREE replace cost, because the
 * customer must be told before the action and this is the proof afterwards.
 */
export interface ApiVpnReplace {
  new: {
    vpn_id: number
    /** The panel identity of the NEW client — a different value from the old one. */
    panel_email: string
    status: string
    confirmed: boolean
    recorded: boolean
  }
  old_vpn_id: number
  old_panel_email: string
  release_id: number
  /** Epoch seconds — the old VPN can be restored until this passes. */
  restore_deadline: number
  claim_consumed: boolean
  claims_remaining: number
}

/** The connection material a client needs to actually connect (D-026). */
export interface ApiVpnLinks {
  vpn_id: number
  panel_email: string
  status: string
  /** The panel's own URIs, passed through untouched. Empty = the panel had none. */
  links: string[]
  /** `panel` when the panel answered, `unavailable` when it could not be read. */
  source: 'panel' | 'unavailable'
}

/** Server reachability, NOT the user's own latency (D-026). */
export interface ApiVpnPing {
  vpn_id: number
  server_id: number
  reachable: boolean
  /** -1 when unreachable — never 0, which would read as "instant". */
  latency_ms: number
  /** Constant label the UI must display. */
  scope: 'server_reachability'
}

/**
 * One inbox entry (`GET /api/notifications`).
 *
 * `read_at` is `null` until the account marks it read — there is no separate
 * boolean, because the timestamp is what the badge and the UI both derive from.
 */
export interface ApiNotification {
  id: number
  kind: 'info' | 'success' | 'warning' | 'error'
  title: string
  body: string
  read_at: number | null
  created_at: number
}

/** One account the caller may address (`GET /api/directory`). */
export interface ApiDirectoryEntry {
  id: number
  email: string
  display_name: string
  role: ApiRole
  status: string
}

/** A payment destination (`GET /api/payment-methods`). `owner_user_id: null` = global. */
export interface ApiPaymentMethod {
  id: number
  owner_user_id: number | null
  kind: 'BANK' | 'EZCASH' | 'MOBILE' | 'OTHER'
  label: string
  details: string
  instructions: string
  active: boolean
  created_at: number
}

/** Payment lifecycle, mirroring the SQL CHECK constraint exactly. */
export type ApiPaymentStatus =
  | 'PENDING'
  | 'AWAITING_CONFIRMATION'
  | 'CONFIRMED'
  | 'REJECTED'
  | 'CANCELLED'
  | 'REFUND_PENDING'
  | 'REFUNDED'

/** One payment record (`GET /api/payments`). Amounts are integer cents. */
export interface ApiPayment {
  id: number
  assignment_id: number | null
  payer_user_id: number
  method_id: number | null
  amount_cents: number
  currency: string
  reference: string
  status: ApiPaymentStatus
  confirmed_by: number | null
  confirmed_at: number | null
  created_at: number
  updated_at: number
  /**
   * WHO placed the order (D-113). The row used to carry only `payer_user_id`,
   * so an operator could not tell whose money it was without a second lookup.
   * Empty means the account no longer exists — the UI states that plainly.
   */
  payer_email?: string
  /** The account the order is FOR; differs when bought on someone's behalf. */
  target_email?: string
  /** The package being bought. Empty for a manual record — never a blank cell. */
  offer_name?: string
  offer_kind?: string
}

/** One chat message (`GET /api/chat/threads/{id}/messages`). */
export interface ApiChatMessage {
  id: number
  thread_id: number
  sender_id: number
  sender_name: string
  sender_email: string
  body: string
  created_at: number
  /** Set when the author edited it; `body` is the current text. */
  edited_at: number | null
  /** Per-viewer flags — these belong to the caller, not to the message. */
  important: boolean
  saved: boolean
}

/** One conversation (`GET /api/chat/threads`). */
export interface ApiChatThread {
  id: number
  peer_id: number
  peer_name: string
  peer_email: string
  peer_role: ApiRole
  locked: boolean
  locked_by_id: number | null
  pinned_message_id: number | null
  unread: number
  last_message: ApiChatMessage | null
  created_at: number
}

/**
 * The rate card row (`GET /api/pricing`, D-035.4).
 *
 * `server_id: null` is the platform default; a number is that server's
 * override. Every amount is integer **cents** — money never becomes a float.
 */
export interface ApiPricingRow {
  id: number
  server_id: number | null
  currency: string
  rate_per_gb_cents: number
  rate_per_day_cents: number
  rate_per_device_cents: number
  rate_per_ip_cents: number
  min_price_cents: number
  /** 0 = no ceiling. */
  max_price_cents: number
  /** 0 = no bound. */
  max_traffic_gb: number
  max_duration_days: number
  max_devices: number
  max_ips: number
  updated_by: number | null
  updated_at: number
}

/** What a CUSTOM PREMIUM buyer asks for. */
export interface ApiSpec {
  traffic_gb: number
  days: number
  devices: number
  ips: number
}

/**
 * The server-computed price for one spec (`POST /api/orders/quote`).
 *
 * The line items are included so the screen can explain the total instead of
 * asserting it. `minimum_applied` means the floor raised the price.
 */
export interface ApiQuote {
  spec: ApiSpec
  currency: string
  traffic_cents: number
  duration_cents: number
  devices_cents: number
  ips_cents: number
  subtotal_cents: number
  total_cents: number
  minimum_applied: boolean
}

/** The three package kinds (D-035). */
export type ApiPlanKind = 'FREE' | 'PREMIUM' | 'CUSTOM_PREMIUM'

/** One package in the catalogue (`GET /api/plans`, `GET /api/packages`). */
export interface ApiPlan {
  id: number
  kind: ApiPlanKind
  name: string
  traffic_gb: number
  duration_days: number
  device_limit: number
  price_cents: number
  currency: string
  server_label: string
  /** `null` = unlimited / not tracked. */
  slots_remaining: number | null
  purchasable: boolean
  /** Whether the plan is currently offered. An inactive plan is not a price
   * anybody can be charged, so a quoting screen must exclude it. */
  active: boolean
  /** An archived plan keeps its sales history but is out of the catalogue. */
  archived: boolean
}

/** One entry of an order's immutable timeline (`GET /api/payments/{id}/events`). */
export interface ApiOrderEvent {
  id: number
  payment_id: number
  kind: string
  actor_id: number | null
  actor_email: string
  detail: string
  created_at: number
}

/**
 * One client as the PANEL holds it, joined with Veylix's tag and assignment
 * (`GET /api/servers/{id}/panel-clients`, D-024 Stage C).
 *
 * Read-only: this never writes to the panel, so a screen built on it can look
 * but cannot change anything remotely.
 */
export interface ApiPanelClient {
  panel_email: string
  sub_id: string
  inbound_ids: number[]
  total_gb: number
  expiry_time: number
  limit_ip: number
  limit_hwid: number
  enable: boolean
  up_bytes: number
  down_bytes: number
  /** `null` for an unclassified VPN — still a VPN, Super-Admin visible only. */
  tag: string | null
  assigned: boolean
  veylix_status?: string
  vpn_instance_id?: number
  owner_user_id?: number
  owner_email?: string
  owner_name?: string
  seller_user_id?: number
  seller_email?: string
  offer_id?: number
  offer_name?: string
  offer_kind?: string
  limit_bytes: number
  used_bytes: number
  usage_pct: number
  days_left: number
  created_at?: number
  protocol?: string
  transport?: string
}

/** A backend error carrying the HTTP status and the safe error code. */
export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }

  /** True when the user simply needs to sign in again. */
  get isAuthError(): boolean {
    return this.status === 401
  }
}

// --- system / version (D-081) ---------------------------------------------

/** What GET /api/system/version reports: the RUNNING build, not a file on disk. */
export interface ApiSystemVersion {
  version: string
  schema_version: number
  ui: string
  go_version: string
  platform: string
}

/** One published release row from GET /api/system/releases. */
export interface ApiRelease {
  tag: string
  published_at?: string
  is_current: boolean
  is_latest: boolean
  notes_url?: string
}

/**
 * GET /api/system/update-check. `error` is set INSTEAD of fabricating values when
 * the release feed cannot be read, so the UI must show "unknown", never "up to
 * date" in that case.
 */
export interface ApiUpdateCheck {
  current: string
  latest?: string
  update_available: boolean
  checked_at?: string
  release_url?: string
  error?: string
}

// --- Super Admin settings (D-080) -----------------------------------------

/** The masked Google OAuth settings. The secret is never included in the body. */
export interface ApiGoogleSettings {
  client_id: string
  has_secret: boolean
  redirect_url: string
  configured: boolean
  /** Where the values came from: an in-app edit or the environment. */
  source: 'database' | 'environment' | 'none' | string
  /**
   * The redirect URL derived from the origin this page was served on, i.e.
   * `<origin>/api/auth/google/callback`. Register THIS in Google Cloud Console —
   * it is built from where the server actually answers, so it cannot be typoed
   * into a `redirect_uri_mismatch`.
   */
  expected_redirect_url?: string
}

/** What the settings form submits. An empty client_secret keeps the stored one. */
export interface ApiGoogleSettingsInput {
  client_id: string
  client_secret: string
  redirect_url: string
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  let res: Response
  try {
    res = await fetch(`${BASE}${path}`, {
      ...init,
      headers,
      credentials: 'include',
    })
  } catch {
    throw new ApiError(0, 'network_error', 'Cannot reach the server. Is the backend running?')
  }

  if (res.status === 204) return undefined as T

  const text = await res.text()
  const body = text ? (JSON.parse(text) as unknown) : null

  if (!res.ok) {
    const err = (body as { error?: { code?: string; message?: string } } | null)?.error
    // A 401 is the normal "you are not signed in" answer. Saying what to do
    // turns a silent "authentication required" into an actionable message.
    const message =
      res.status === 401
        ? `${err?.message ?? 'Request failed'} — your session has ended. Sign in again at /login.`
        : (err?.message ?? 'Request failed')
    throw new ApiError(res.status, err?.code ?? 'unknown_error', message)
  }
  return body as T
}

export const api = {
  // --- health -------------------------------------------------------------
  health: () => request<{ status: string; version: string }>('/api/health'),

  // --- system / version + releases (D-081) --------------------------------
  /** The RUNNING build: version, applied schema, and whether the UI is embedded. */
  systemVersion: () => request<ApiSystemVersion>('/api/system/version'),
  /** Every published release, newest first, with the running one marked. */
  systemReleases: () => request<{ releases: ApiRelease[] }>('/api/system/releases'),
  /** Is there something newer? An unreachable feed answers `error`, not "current". */
  updateCheck: () => request<ApiUpdateCheck>('/api/system/update-check'),

  // --- Super Admin settings (D-080) ---------------------------------------
  /** The masked Google OAuth client (never the secret). */
  googleSettings: () => request<ApiGoogleSettings>('/api/super/settings/google'),
  /** Save it; applied live in the running process. Empty secret keeps the old one. */
  saveGoogleSettings: (input: ApiGoogleSettingsInput) =>
    request<ApiGoogleSettings>('/api/super/settings/google', {
      method: 'PUT',
      body: JSON.stringify(input),
    }),

  // --- auth ---------------------------------------------------------------
  /** Whether the installation still needs its first super admin. */
  setupStatus: () => request<{ bootstrap_required: boolean }>('/api/setup/status'),

  /**
   * Asks for a NEW confirmation link (D-078).
   *
   * Public on purpose: an unverified account cannot sign in, so it cannot reach
   * the authenticated `/api/auth/verify/request`. The server answers the same way
   * whether or not the address is registered — see `note` — so the UI must not
   * claim "we sent it to your inbox" as a fact.
   */
  resendVerification: (identifier: string) =>
    request<{ sent: boolean; note: string }>('/api/auth/verify/resend', {
      method: 'POST',
      body: JSON.stringify({ identifier }),
    }),

  /**
   * Sign up (D-076).
   *
   * The answer is NOT always a user: a self-registered account is created
   * UNVERIFIED, so the server refuses to hand out a session it would refuse at the
   * login door. In that case it answers **202** with `pending_verification: true`
   * and no cookie, and the caller must send the person to their mailbox instead of
   * into the app.
   */
  register: (email: string, password: string, displayName: string) =>
    request<ApiRegisterResult>('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, password, display_name: displayName }),
    }),

  /**
   * Sign in through one of the two doors (D-041).
   *
   * `identifier` is an e-mail address OR a username. `portal` is `client` for
   * /login and `staff` for /staff/login — the server refuses an account at the
   * wrong door with `wrong_portal`, so the UI can say "use the staff login"
   * instead of "wrong password".
   */
  login: (identifier: string, password: string, portal: ApiPortal = 'client') =>
    request<ApiUser>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ identifier, password, portal }),
    }),

  logout: () => request<void>('/api/auth/logout', { method: 'POST' }),

  /**
   * Ends every session for this account EXCEPT the one making the call (D3).
   *
   * The current browser stays signed in — the point is to evict a device you no
   * longer hold. `ended` is how many sessions the server actually removed, so the
   * screen can say "2 other devices signed out" instead of guessing.
   */
  logoutOthers: () =>
    request<{ ended: number }>('/api/auth/logout-others', { method: 'POST' }),

  /**
   * Ask for a password-reset code (D-044).
   *
   * The server answers **202** whether or not the account exists, so the screen
   * must say "if that account exists, a code is on its way" and never imply that
   * the address was found. `delivery` reports how the code left the building:
   * `"smtp"` for a real relay, `"outbox"` when none is configured — the UI must
   * not claim an e-mail was sent in that case.
   */
  forgotPassword: (identifier: string) =>
    request<{ sent: boolean; message?: string; delivery?: 'smtp' | 'outbox' }>(
      '/api/auth/forgot',
      { method: 'POST', body: JSON.stringify({ identifier }) },
    ),

  /** Redeem a reset code. Every session for the account is signed out. */
  resetPassword: (identifier: string, code: string, newPassword: string) =>
    request<{ reset: boolean; note?: string }>('/api/auth/reset', {
      method: 'POST',
      body: JSON.stringify({ identifier, code, new_password: newPassword }),
    }),

  me: () => request<ApiUser>('/api/me'),

  /* --- devices (HWID) — D8 ------------------------------------------------ */

  /**
   * The hardware identifiers a VPN has registered against its device cap.
   *
   * The panel is the only source: Veylix never stores a device, so this call
   * fails honestly when the panel cannot be read rather than showing a cached
   * or empty list.
   */
  panelClientDevices: (serverId: number, panelEmail: string) =>
    request<{ server_id: number; panel_email: string; devices: ApiHwidDevice[] }>(
      `/api/panel-clients/${serverId}/${encodeURIComponent(panelEmail)}/hwids`,
    ),

  /** Frees one device slot. The backend confirms with a fresh panel read. */
  revokePanelClientDevice: (serverId: number, panelEmail: string, hwidId: number) =>
    request<void>(
      `/api/panel-clients/${serverId}/${encodeURIComponent(panelEmail)}/hwids/${hwidId}`,
      { method: 'DELETE' },
    ),

  // --- my profile (D-041) ---------------------------------------------------
  profile: () => request<ApiProfile>('/api/me/profile'),
  updateProfile: (patch: ApiProfilePatch) =>
    request<ApiProfile>('/api/me/profile', { method: 'PATCH', body: JSON.stringify(patch) }),
  usernameAvailable: (username: string) =>
    request<{ available: boolean; reason?: string }>(
      `/api/me/username-available?username=${encodeURIComponent(username)}`,
    ),
  requestEmailChange: (newEmail: string, currentPassword: string) =>
    request<ApiEmailChangeRequest>('/api/me/email-change/request', {
      method: 'POST',
      body: JSON.stringify({ new_email: newEmail, current_password: currentPassword }),
    }),
  confirmEmailChange: (code: string) =>
    request<{ changed: boolean; profile: ApiProfile }>('/api/me/email-change/confirm', {
      method: 'POST',
      body: JSON.stringify({ code }),
    }),

  // --- Super Admin: client console (D-042) ----------------------------------
  adminClients: (q: ApiClientQuery = {}) => {
    const params = new URLSearchParams()
    for (const [k, v] of Object.entries(q)) {
      // Only send what is set: an empty filter must not become "status=".
      if (v === undefined || v === null || v === '') continue
      params.set(k, String(v))
    }
    const qs = params.toString()
    return request<ApiAdminClientPage>(`/api/super/clients${qs ? '?' + qs : ''}`)
  },
  /**
   * Deletes a customer account (Super Admin only).
   *
   * Soft: the account is marked DELETED and can never sign in again, while its
   * VPNs, payments and audit rows are kept. Refused with 409 while the client
   * still owns VPNs, because those live on the panel.
   */
  deleteClient: (id: number) =>
    request<{ deleted: boolean; note?: string }>(`/api/super/clients/${id}`, {
      method: 'DELETE',
    }),

  adminClient: (id: number) => request<ApiAdminClientDetail>(`/api/super/clients/${id}`),

  /**
   * Issues a fresh password for a customer (Super Admin only).
   *
   * The password comes back exactly once and is never stored in clear text; every
   * session for that account ends. It is the only recovery path when the client's
   * mailbox is wrong or gone, because both reset and verification are e-mails.
   */
  resetClientPassword: (id: number) =>
    request<{ password: string; note?: string }>(
      `/api/super/clients/${id}/reset-password`,
      { method: 'POST' },
    ),
  /**
   * Starts an e-mail change for a customer (Super Admin only, D-077).
   *
   * The address does NOT move yet: a 6-digit code goes to the NEW mailbox and
   * must be confirmed. Without that step a typo could silently move an account's
   * sign-in identifier and recovery target.
   *
   * `code` is present only when no mail relay is configured — see `delivery`.
   */
  requestClientEmailChange: (id: number, newEmail: string) =>
    request<{
      sent: boolean
      to: string
      /** "smtp" when a relay delivered it; "outbox" when none is configured. */
      delivery: 'smtp' | 'outbox'
      expires_minutes: number
      note?: string
      /** Present only for `delivery: "outbox"`. */
      code?: string
    }>(`/api/super/clients/${id}/email-change`, {
      method: 'POST',
      body: JSON.stringify({ new_email: newEmail }),
    }),

  /** Finishes it with the code from the new mailbox. */
  confirmClientEmailChange: (id: number, code: string) =>
    request<{ email: string; note?: string }>(
      `/api/super/clients/${id}/email-change/confirm`,
      { method: 'POST', body: JSON.stringify({ code }) },
    ),

  /**
   * Block/unblock and/or set the FREE limit. One endpoint because both are the
   * same kind of operator decision and both are audited.
   */
  updateAdminClient: (id: number, patch: { status?: 'ACTIVE' | 'SUSPENDED'; free_vpn_limit?: number }) =>
    request<ApiAdminClientDetail>(`/api/super/clients/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),

  // --- Super Admin: staff console (D-044) -----------------------------------
  adminStaff: (q: ApiStaffQuery = {}) => {
    const params = new URLSearchParams()
    for (const [k, v] of Object.entries(q)) {
      if (v === undefined || v === null || v === '') continue
      params.set(k, String(v))
    }
    const qs = params.toString()
    return request<ApiStaffPage>(`/api/super/staff${qs ? '?' + qs : ''}`)
  },
  adminStaffOne: (id: number) => request<ApiStaffDetail>(`/api/super/staff/${id}`),
  /**
   * Creates an admin or super admin. The BACKEND generates the password and
   * returns it once — an operator-typed password would be weak or reused, and it
   * would end up in a shell history.
   */
  createAdminStaff: (input: { email: string; display_name: string; role: 'admin' | 'super_admin'; quota_limit?: number }) =>
    request<{ account: ApiUser; password: string; note: string }>('/api/super/staff', {
      method: 'POST',
      body: JSON.stringify(input),
    }),
  updateAdminStaff: (id: number, patch: { status?: 'ACTIVE' | 'SUSPENDED'; quota_limit?: number; role?: 'admin' | 'super_admin' }) =>
    request<{ id: number; ok: boolean }>(`/api/super/staff/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),
  resetAdminStaffPassword: (id: number) =>
    request<{ password: string; note: string }>(`/api/super/staff/${id}/reset-password`, { method: 'POST' }),

  /**
   * The Super Admin overview (D-107): money, the reseller book, and which sections
   * failed to load.
   *
   * ONE call, because the page renders once. `errors` names the sections that did
   * not resolve, so the screen shows the parts that worked and says which did not
   * — the alternative, a 500 for the whole page, throws away a money row that
   * arrived perfectly well.
   */
  superOverview: () => request<ApiSuperOverview>('/api/super/overview/summary'),
  adminVpns: (q: ApiAdminVPNQuery = {}) => {
    const params = new URLSearchParams()
    for (const [k, v] of Object.entries(q)) {
      if (v === undefined || v === null || v === '') continue
      params.set(k, String(v))
    }
    const qs = params.toString()
    return request<ApiAdminVPNPage>(`/api/super/vpns${qs ? '?' + qs : ''}`)
  },

  /**
   * Assigns an unowned panel client to a customer (D-037, D-034, D-063).
   *
   * This route existed and was tested on the server long before the UI could
   * call it: `VpnsPage` rendered the word "assign" as plain text, so an
   * unassigned panel client could only be handed over by hand against the API.
   *
   * An assignment is ALWAYS premium (D-063) — there is no `kind` to choose,
   * because a FREE VPN is claimed from an offer rather than assigned. `offerId`
   * is optional: the server attaches the platform's premium carrier when the
   * operator does not pick a package, so the VPN can never end up without a kind
   * (which is what made it display as UNTAGGED). `rename` is opt-in and cosmetic.
   */
  assignPanelClient: (
    serverId: number,
    panelEmail: string,
    input: { clientUserId: number; offerId?: number; sellerId?: number; rename?: boolean },
  ) =>
    request<{
      vpn_id: number
      panel_email: string
      tag: string | null
      offer_id: number
      renamed: boolean
      /** True whenever this call returns 201 — the assignment itself succeeded. */
      assigned: boolean
      /** Present only when a requested rename did not land; the VPN kept its name. */
      warning: string
    }>(`/api/panel-clients/${serverId}/${encodeURIComponent(panelEmail)}/assign`, {
      method: 'POST',
      body: JSON.stringify({
        client_user_id: input.clientUserId,
        kind: 'paid',
        offer_id: input.offerId ?? 0,
        seller_user_id: input.sellerId ?? 0,
        ...(input.rename === undefined ? {} : { rename: input.rename }),
      }),
    }),

  /**
   * Classifies a panel client FREE or PREMIUM (D-024.3).
   *
   * This is the act that PUBLISHES a VPN: an untagged panel client is visible to
   * the Super Admin alone, so without this call a client created directly in
   * 3X-UI could never enter the FREE catalogue at all — which is exactly what the
   * console was missing.
   *
   * It is a LOCAL write: it never talks to the panel, so a panel outage cannot
   * block classification (the panel owns the VPN, not the label on it).
   *
   * Refused with `409 tag_locked` once the VPN has been issued — flipping
   * FREE↔PREMIUM after a sale would retroactively change what the customer paid
   * for (D-025.2), so the lock is enforced in SQL, not by the UI.
   */
  setPanelClientTag: (serverId: number, panelEmail: string, tag: 'FREE' | 'PREMIUM') =>
    request<{
      tagged: boolean
      tag: string
    }>(`/api/panel-clients/${serverId}/${encodeURIComponent(panelEmail)}/tag`, {
      method: 'PUT',
      body: JSON.stringify({ tag }),
    }),

  /**
   * Moves an ALREADY-assigned VPN to another customer (D-064).
   *
   * Distinct from `assignPanelClient`, which claims a panel client nobody holds.
   * This one moves ownership of a VPN that already works, so the panel client is
   * NOT renamed and the customer's existing connection keeps functioning — the
   * response says so by returning the same `panel_email` that went in.
   *
   * The refusals are precise on purpose (`vpn_terminal`, `vpn_released`,
   * `vpn_not_on_panel`, `cannot_transfer_self`, `not_a_client`,
   * `concurrent_change`), because "cannot transfer" with no reason is what makes
   * an operator revoke a working VPN to get past it.
   */
  transferVPN: (
    vpnId: number,
    input: { toClientUserId: number; reason?: string; sellerId?: number },
  ) =>
    request<{
      vpn_id: number
      server_id: number
      panel_email: string
      offer_id: number
      kind: string
      from: ApiTransferParty
      to: ApiTransferParty
      /** True when a FREE quota slot followed the VPN to its new owner. */
      free_claim_moved: boolean
    }>(`/api/vpns/${vpnId}/transfer`, {
      method: 'POST',
      body: JSON.stringify({
        to_client_user_id: input.toClientUserId,
        seller_user_id: input.sellerId ?? 0,
        reason: input.reason ?? '',
      }),
    }),

  /** One VPN's transfer history, newest first (D-064). Super Admin only. */
  vpnTransfers: (vpnId: number) => request<ApiVpnTransfer[]>(`/api/vpns/${vpnId}/transfers`),

  /** Whether "Continue with Google" can work at all on this server (D-039). */
  googleStatus: () => request<{ configured: boolean }>('/api/auth/google/status'),

  /**
   * Asks the backend for an e-mail verification link (D-041).
   *
   * `token`/`link` are present **only** for a loopback caller: with no SMTP relay
   * the link would otherwise be unstuck, so an operator on the same machine gets
   * it back, while a deployment never leaks it to the browser.
   */
  requestEmailVerification: () =>
    request<{
      sent: boolean
      email: string
      delivery?: 'smtp' | 'outbox'
      token?: string
      link?: string
    }>('/api/auth/verify/request', { method: 'POST' }),

  bootstrap: (email: string, password: string, displayName: string) =>
    request<ApiUser>('/api/setup/bootstrap', {
      method: 'POST',
      body: JSON.stringify({ email, password, display_name: displayName }),
    }),

  /**
   * Rotates the credential. The backend revokes every other session, re-issues
   * this browser's cookie, and e-mails a warning to the account holder.
   *
   * `delivery` reports whether that warning went out (`smtp`) or could not
   * (`outbox`, no relay) — the UI must not claim a mail was sent when it was not.
   */
  changePassword: (currentPassword: string, newPassword: string) =>
    request<{ changed: boolean; delivery?: 'smtp' | 'outbox'; note?: string }>(
      '/api/auth/change-password',
      {
        method: 'POST',
        body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
      },
    ),

  // --- server integrations ------------------------------------------------
  listServers: () => request<ApiServer[]>('/api/servers'),

  addServer: (input: { name: string; panelUrl: string; token: string; allowPrivateNetwork?: boolean }) =>
    request<ApiServer>('/api/servers', {
      method: 'POST',
      body: JSON.stringify({
        name: input.name,
        panel_url: input.panelUrl,
        token: input.token,
        allow_private_network: input.allowPrivateNetwork ?? false,
      }),
    }),

  testServer: (id: number) => request<ApiServer>(`/api/servers/${id}/test`, { method: 'POST' }),

  /**
   * Edits a panel. Every field is optional — an omitted `token` keeps the
   * stored credential (it can never be read back), and changing `panelUrl`
   * requires a new token because the ciphertext is bound to the URL.
   */
  updateServer: (
    id: number,
    input: { name?: string; panelUrl?: string; token?: string; allowPrivateNetwork?: boolean },
  ) =>
    request<ApiServer>(`/api/servers/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({
        ...(input.name !== undefined ? { name: input.name } : {}),
        ...(input.panelUrl !== undefined ? { panel_url: input.panelUrl } : {}),
        ...(input.token ? { token: input.token } : {}),
        ...(input.allowPrivateNetwork !== undefined
          ? { allow_private_network: input.allowPrivateNetwork }
          : {}),
      }),
    }),

  serverInbounds: (id: number) => request<ApiInbound[]>(`/api/servers/${id}/inbounds`),

  disableServer: (id: number) => request<void>(`/api/servers/${id}/disable`, { method: 'POST' }),

  /** Permanently deletes the integration (and its cached inbounds). 409 while
   *  VPN instances still reference it. */
  removeServer: (id: number) => request<void>(`/api/servers/${id}`, { method: 'DELETE' }),

  // --- account ------------------------------------------------------------
  /**
   * The audit trail, filtered SERVER-SIDE (D-082). The envelope carries the
   * count AFTER the filters, so a page can say "37 matches" while showing one
   * window of them — a filter must never hide its own answer behind a fixed
   * limit, which is exactly what the old `?limit=`-only read allowed.
   */
  audit: (query: ApiAuditQuery = {}) => {
    const p = new URLSearchParams()
    if (query.kind) p.set('kind', query.kind)
    if (query.actor_id) p.set('actor_id', String(query.actor_id))
    if (query.from) p.set('from', String(query.from))
    if (query.to) p.set('to', String(query.to))
    if (query.q) p.set('q', query.q)
    p.set('limit', String(query.limit ?? 100))
    p.set('offset', String(query.offset ?? 0))
    return request<ApiAuditPage>(`/api/audit?${p.toString()}`)
  },

  myQuota: () => request<ApiQuota>('/api/quotas/me'),

  // --- VPNs (Stage E4 backend; wired here in Phase 5) ----------------------
  /**
   * Lists the VPNs this caller may see. The backend scopes the rows by role —
   * a client gets only their own, an admin only what they sold — so the caller
   * never passes an owner id.
   */
  listVpns: () => request<ApiVpn[]>('/api/vpns'),

  /**
   * What this client has used of its FREE allowance on one server (D-025.4/.5).
   *
   * Exists so the caps are SHOWN BEFORE a claim — REQUIREMENTS §2.0.1 requires it,
   * because releasing a VPN does not refund a monthly slot. The endpoint has
   * returned this since the claim caps shipped; the screen never asked, so a
   * client learned the limit by hitting it.
   *
   * `server_id` is required because both caps are per server.
   */
  freeClaimUsage: (serverId: number) =>
    request<{
      /** FREE VPNs held right now (the concurrent cap counts these). */
      concurrent: number
      /** Claims inside the rolling window, including released ones. */
      used_this_window: number
      monthly_limit: number
      concurrent_limit: number
    }>(`/api/vpns/claims/me?server_id=${serverId}`),

  /**
   * One VPN's usage, WITHOUT the history points (`?summary=1`, D-068).
   *
   * The full endpoint returns every snapshot — several kilobytes per VPN — which
   * is right for the detail chart and wrong for a card that needs two numbers.
   * `limit_bytes: 0` with `remaining_bytes: -1` means unlimited, not "0 % of 0".
   */
  vpnUsageSummary: (vpnId: number) =>
    request<{
      vpn_id: number
      status: string
      limit_bytes: number
      lifetime_total_bytes: number
      usage_pct: number
      remaining_bytes: number
      expires_at: number | null
      point_count: number
    }>(`/api/vpns/${vpnId}/usage?summary=1`),

  /**
   * Provisions a VPN from an offer.
   *
   * `clientUserId` is only meaningful for an admin/super admin assigning a
   * Premium VPN to someone else; a client omits it and the backend uses the
   * authenticated principal.
   */
  createVpn: (offerId: number, clientUserId?: number) =>
    request<{ vpn_id: number; panel_email: string; status: string; confirmed: boolean }>(
      '/api/vpns',
      {
        method: 'POST',
        body: JSON.stringify({
          offer_id: offerId,
          ...(clientUserId ? { client_user_id: clientUserId } : {}),
        }),
      },
    ),

  suspendVpn: (id: number) => request<void>(`/api/vpns/${id}/suspend`, { method: 'POST' }),

  resumeVpn: (id: number) => request<void>(`/api/vpns/${id}/resume`, { method: 'POST' }),

  /**
   * Puts a VPN to sleep for this customer (D-094). No panel client is touched
   * and nothing is deleted — the VPN just stops asking for attention.
   */
  dormantVpn: (id: number) => request<void>(`/api/vpns/${id}/dormant`, { method: 'POST' }),

  /** Wakes a dormant VPN. It does not repair it; it only makes it visible again. */
  wakeVpn: (id: number) => request<void>(`/api/vpns/${id}/wake`, { method: 'POST' }),

  /** Revocation is asynchronous: the backend returns the status it reached. */
  revokeVpn: (id: number) =>
    request<{ status: string; confirmed: boolean }>(`/api/vpns/${id}/revoke`, { method: 'POST' }),

  /**
   * Erases Veylix's record of a VPN the panel no longer holds (D-027.3).
   *
   * The route has existed and been tested on the server for a long time, but no
   * client ever called it: a VPN whose panel client had vanished kept a row in
   * every list forever, and the console offered no way to clear it. The panel is
   * NEVER touched — revoke is the operation that deletes a panel client, and this
   * one refuses with 409 `vpn_still_on_panel` while the client still exists.
   */
  deleteVpn: (id: number) => request<void>(`/api/vpns/${id}`, { method: 'DELETE' }),

  /**
   * Retires a VPN RECORD while deliberately KEEPING its panel client (P-3).
   *
   * `deleteVpn` correctly refuses (409) while the panel still holds the client,
   * which left an operator with no way to clear a record whose client must stay
   * (D-027.3 forbids Veylix from ever deleting a panel client). This is that path.
   *
   * Super Admin only. `confirmPanelEmail` must repeat the VPN's identity back —
   * the API answers 422 `confirm_panel_email_required` when it does not, naming the
   * value to type. The call never reaches the panel: `DeleteClient` is not invoked.
   */
  purgeVpnRecord: (id: number, confirmPanelEmail: string) =>
    request<void>(`/api/vpns/${id}/purge`, {
      method: 'POST',
      body: JSON.stringify({ confirm_panel_email: confirmPanelEmail }),
    }),

  /**
   * Runs a sync pass NOW, instead of waiting for the 5-minute scheduler.
   *
   * `GET /api/sync/status` could already tell an operator that the data was
   * stale, but nothing could freshen it: they could only wait, with no evidence
   * that waiting was working. The response is the pass's own result, so the
   * answer says what happened rather than "started".
   */
  syncRun: (serverIds?: number[]) =>
    request<ApiSyncRun>('/api/sync/run', {
      method: 'POST',
      body: JSON.stringify(serverIds && serverIds.length > 0 ? { server_ids: serverIds } : {}),
    }),

  /**
   * Usage history for one VPN.
   *
   * `range` is the owner's switch (P-06), extended by D-092 with `24h` and a
   * custom window. The backend refuses a typo with 422 instead of showing a
   * different window, so the value is passed through untouched.
   *
   * A custom window needs `since` (and usually `until`) as epoch seconds. Both
   * are appended only when supplied, because a bare `?since=` is a well-defined
   * request — a named range with its lower edge moved — and the backend accepts
   * it.
   */
  vpnUsage: (id: number, range: ApiUsageRange = '30d', limit = 240, bounds?: { since?: number; until?: number }) => {
    const q = new URLSearchParams({ range, limit: String(limit) })
    if (bounds?.since && bounds.since > 0) q.set('since', String(Math.floor(bounds.since)))
    if (bounds?.until && bounds.until > 0) q.set('until', String(Math.floor(bounds.until)))
    return request<ApiVpnUsage>(`/api/vpns/${id}/usage?${q.toString()}`)
  },

  /**
   * Per-hour traffic deltas for the "usage by time" chart (D-092).
   *
   * The same window as `vpnUsage`, so both charts always describe the same
   * period — a page whose two charts covered different ranges would be worse
   * than having one.
   */
  vpnUsageHourly: (id: number, range: ApiUsageRange = '30d', bounds?: { since?: number; until?: number }) => {
    const q = new URLSearchParams({ range })
    if (bounds?.since && bounds.since > 0) q.set('since', String(Math.floor(bounds.since)))
    if (bounds?.until && bounds.until > 0) q.set('until', String(Math.floor(bounds.until)))
    return request<ApiHourlyUsage>(`/api/vpns/${id}/usage/hourly?${q.toString()}`)
  },

  /**
   * Measured rates between consecutive sync snapshots (D-092).
   *
   * An interval the counters cannot describe comes back with
   * `source: 'unavailable'`; the caller must render a gap, never a zero.
   */
  vpnSpeedHistory: (id: number, range: ApiUsageRange = '30d', bounds?: { since?: number; until?: number }) => {
    const q = new URLSearchParams({ range })
    if (bounds?.since && bounds.since > 0) q.set('since', String(Math.floor(bounds.since)))
    if (bounds?.until && bounds.until > 0) q.set('until', String(Math.floor(bounds.until)))
    return request<ApiSpeedHistory>(`/api/vpns/${id}/speed-history?${q.toString()}`)
  },

  /**
   * Stored reachability probes for one VPN (D-092).
   *
   * Answers 404 when the deployment has no sample store, which is honest: there
   * is no latency history to read, and an empty 200 would look like a measured
   * silence.
   */
  /**
   * Stored latency probes (D-092) with an optional measurement-source filter
   * (D-095).
   *
   * `source` is forwarded verbatim so an unknown value is refused by the server
   * with `422` instead of being silently dropped here — a client-side default
   * would turn a typo into "everything matched".
   */
  vpnPingHistory: (id: number, range: ApiUsageRange = '24h', source?: ApiUsageSource) => {
    const q = new URLSearchParams({ range })
    if (source && source !== 'all') q.set('source', source)
    return request<ApiPingHistory>(`/api/vpns/${id}/ping/history?${q.toString()}`)
  },

  // --- release / restore (D-027) -------------------------------------------
  /**
   * Releases a depleted VPN and opens the **3-day undo window** (D-027.2).
   * Nothing is deleted: the panel client keeps working and the record survives
   * until the window closes.
   *
   * `reason` is optional for the OWNER (it defaults to "quota exhausted") and
   * **mandatory for a Super Admin** releasing somebody else's VPN (P-2) — the API
   * answers 422 `reason_required` without one, and audits the act as
   * `vpn.force_release`.
   */
  releaseVpn: (id: number, reason?: string) =>
    request<ApiVpnRelease>(`/api/vpns/${id}/release`, {
      method: 'POST',
      body: JSON.stringify(reason ? { reason } : {}),
    }),

  /** The undo button. Refused with 409 once the window has closed. */
  restoreRelease: (releaseId: number) =>
    request<ApiVpnRelease>(`/api/vpn-releases/${releaseId}/restore`, { method: 'POST' }),

  /**
   * Replace a VPN (D-112): build a NEW one, then release the current one.
   *
   * `confirm` is required by the route — the action has two side effects (a new
   * panel client and a released VPN), so it is never taken on a request that
   * could be a stray retry. `offer_id` is omitted to keep the same package.
   */
  replaceVpn: (id: number, body: { offerId?: number; reason?: string } = {}) =>
    request<ApiVpnReplace>(`/api/vpns/${id}/replace`, {
      method: 'POST',
      body: JSON.stringify({
        confirm: true,
        ...(body.offerId ? { offer_id: body.offerId } : {}),
        ...(body.reason ? { reason: body.reason } : {}),
      }),
    }),

  /** The caller's release timeline (a client sees only their own). */
  listReleases: () => request<ApiVpnRelease[]>('/api/vpn-releases'),

  // --- admins: server scope (P5) ------------------------------------------
  /** Which panels a reseller may operate on. */
  adminServers: (id: number) =>
    request<{ admin_id: number; server_ids: number[] }>(`/api/admins/${id}/servers`),

  /**
   * Replaces the whole assignment set. Replace (not merge) keeps the call
   * idempotent, so a checkbox UI can just send what is ticked.
   */
  setAdminServers: (id: number, serverIds: number[]) =>
    request<{ admin_id: number; server_ids: number[] }>(`/api/admins/${id}/servers`, {
      method: 'PUT',
      body: JSON.stringify({ server_ids: serverIds }),
    }),

  // --- clients (P5) -------------------------------------------------------
  /**
   * Clients the caller may act on. The backend scopes it: a super admin sees
   * every client, a reseller only those who own a VPN they sold.
   */
  listClients: () => request<ApiClient[]>('/api/clients'),

  // --- offers (P3) --------------------------------------------------------
  /**
   * The catalogue. The backend scopes it by role: a client receives only the
   * active FREE offers, so this single call serves every screen.
   */
  listOffers: (activeOnly = false) =>
    request<ApiOffer[]>(`/api/offers${activeOnly ? '?active=1' : ''}`),

  /**
   * The resale catalogue a client may BUY from (D-048).
   *
   * Separate from `listOffers`, whose client view is FREE-only by D-024.3 — that
   * rule is about which VPNs a client can SEE, not which plans they can buy. The
   * Buy Premium screen needs the latter, and without this endpoint it could never
   * show a plan at all.
   */
  listPremiumOffers: () => request<ApiOffer[]>('/api/offers/premium'),

  createOffer: (input: {
    kind: 'FREE' | 'PREMIUM'
    name: string
    serverId: number
    inboundId?: number
    trafficGb?: number
    durationDays?: number
    deviceLimit?: number
    active?: boolean
  }) =>
    request<ApiOffer>('/api/offers', {
      method: 'POST',
      body: JSON.stringify({
        kind: input.kind,
        name: input.name,
        server_id: input.serverId,
        inbound_id: input.inboundId ?? 0,
        traffic_gb: input.trafficGb ?? 0,
        duration_days: input.durationDays ?? 0,
        device_limit: input.deviceLimit ?? 0,
        ...(input.active !== undefined ? { active: input.active } : {}),
      }),
    }),

  updateOffer: (
    id: number,
    patch: {
      name?: string
      trafficGb?: number
      durationDays?: number
      deviceLimit?: number
      active?: boolean
    },
  ) =>
    request<ApiOffer>(`/api/offers/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({
        ...(patch.name !== undefined ? { name: patch.name } : {}),
        ...(patch.trafficGb !== undefined ? { traffic_gb: patch.trafficGb } : {}),
        ...(patch.durationDays !== undefined ? { duration_days: patch.durationDays } : {}),
        ...(patch.deviceLimit !== undefined ? { device_limit: patch.deviceLimit } : {}),
        ...(patch.active !== undefined ? { active: patch.active } : {}),
      }),
    }),

  /**
   * Retires an offer from the catalogue without destroying it (P-4).
   *
   * `updateOffer({active:false})` hid an offer but recorded no reason, and the row
   * stayed in every listing labelled "inactive" forever. Archiving records the
   * moment and takes it out of the shop, keeping the row for anything that
   * references it. Idempotent: the FIRST timestamp is kept.
   */
  archiveOffer: (id: number) =>
    request<ApiOffer>(`/api/offers/${id}/archive`, { method: 'POST' }),

  /**
   * Removes an offer NOTHING depends on (P-4).
   *
   * Refused with 409 `offer_in_use` when a VPN or a payment names it — both
   * foreign keys are `ON DELETE SET NULL`, so without that guard the delete would
   * silently erase which product a customer paid for. The UI offers archive in
   * that case instead of pretending the delete worked.
   */
  deleteOffer: (id: number) => request<void>(`/api/offers/${id}`, { method: 'DELETE' }),

  // --- observability (Stage E5) -------------------------------------------
  /** Server-wide CPU/RAM history. Labelled `server_wide` by the backend. */
  serverMetrics: (id: number, limit = 240) =>
    request<{ scope: string; points: ApiMetricPoint[] }>(
      `/api/servers/${id}/metrics?limit=${limit}`,
    ),

  syncStatus: () =>
    request<{ stale_after_seconds: number; servers: ApiSyncServer[] }>('/api/sync/status'),

  // --- accounts (super admin only) ----------------------------------------
  listAdmins: (role: ApiRole = 'admin') => request<ApiUser[]>(`/api/admins?role=${role}`),

  createAdmin: (input: {
    email: string
    password: string
    displayName: string
    role?: 'admin' | 'super_admin'
    quotaLimit?: number
  }) =>
    request<ApiUser>('/api/admins', {
      method: 'POST',
      body: JSON.stringify({
        email: input.email,
        password: input.password,
        display_name: input.displayName,
        role: input.role ?? 'admin',
        quota_limit: input.quotaLimit ?? 0,
      }),
    }),

  updateAdmin: (id: number, patch: { status?: 'ACTIVE' | 'SUSPENDED'; quotaLimit?: number }) =>
    request<ApiUser>(`/api/admins/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({
        ...(patch.status ? { status: patch.status } : {}),
        ...(patch.quotaLimit !== undefined ? { quota_limit: patch.quotaLimit } : {}),
      }),
    }),

  // --- notifications (P8) --------------------------------------------------
  /** The caller's own inbox, newest first. There is no "somebody else's". */
  listNotifications: (limit = 50) => request<ApiNotification[]>(`/api/notifications?limit=${limit}`),

  unreadNotificationCount: () => request<{ unread: number }>('/api/notifications/unread'),

  markNotificationRead: (id: number) =>
    request<{ id: number; read: boolean }>(`/api/notifications/${id}/read`, { method: 'POST' }),

  markAllNotificationsRead: () =>
    request<{ marked: number }>('/api/notifications/read-all', { method: 'POST' }),

  // --- directory -----------------------------------------------------------
  /**
   * Accounts the caller may address. The backend scopes it by role, so the chat
   * participant picker cannot enumerate the platform.
   */
  directory: () => request<ApiDirectoryEntry[]>('/api/directory'),

  // --- billing (P6) --------------------------------------------------------
  listPaymentMethods: () => request<ApiPaymentMethod[]>('/api/payment-methods'),

  createPaymentMethod: (input: {
    kind: ApiPaymentMethod['kind']
    label: string
    details?: string
    instructions?: string
  }) =>
    request<ApiPaymentMethod>('/api/payment-methods', {
      method: 'POST',
      body: JSON.stringify({
        kind: input.kind,
        label: input.label,
        details: input.details ?? '',
        instructions: input.instructions ?? '',
      }),
    }),

  setPaymentMethodActive: (id: number, active: boolean) =>
    request<ApiPaymentMethod>(`/api/payment-methods/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ active }),
    }),

  listPayments: (pendingOnly = false) =>
    request<ApiPayment[]>(`/api/payments${pendingOnly ? '?pending=1' : ''}`),

  /**
   * The order list together with the UNCAPPED total (D-116).
   *
   * The plain `listPayments` returns at most 200 rows and says nothing about
   * the rest, so a screen built on it cannot tell "there are 200 orders" from
   * "there are 200 shown of 2,400". This call carries the count, which is the
   * only honest basis for that sentence.
   *
   * `total` is **-1 when the count could not be taken** — never 0. A failed
   * count must not be rendered as "no orders exist".
   */
  listPaymentsPaged: (pendingOnly = false) =>
    request<{
      payments: ApiPayment[]
      shown: number
      total: number
      truncated: boolean
    }>(`/api/payments?envelope=1${pendingOnly ? '&pending=1' : ''}`),

  /**
   * Records an intent to pay. Amount is integer **cents**, never a float.
   *
   * `offerId` is what turns an intent into an ORDER: with it the server prices the
   * plan itself (the payer never sets the product's price) and confirmation builds
   * a VPN (D-031.1). Without it the payment is a deposit that releases no product,
   * so a buyer choosing a plan must always send it.
   */
  createPayment: (input: {
    amountCents: number
    methodId?: number
    assignmentId?: number
    reference?: string
    offerId?: number
    quantity?: number
  }) =>
    request<ApiPayment>('/api/payments', {
      method: 'POST',
      body: JSON.stringify({
        amount_cents: input.amountCents,
        method_id: input.methodId ?? 0,
        assignment_id: input.assignmentId ?? 0,
        reference: input.reference ?? '',
        offer_id: input.offerId ?? 0,
        quantity: input.quantity ?? 0,
      }),
    }),

  attachPaymentReference: (id: number, reference: string) =>
    request<ApiPayment>(`/api/payments/${id}/reference`, {
      method: 'POST',
      body: JSON.stringify({ reference }),
    }),

  confirmPayment: (id: number) => request<ApiPayment>(`/api/payments/${id}/confirm`, { method: 'POST' }),
  rejectPayment: (id: number) => request<ApiPayment>(`/api/payments/${id}/reject`, { method: 'POST' }),
  refundPayment: (id: number) => request<ApiPayment>(`/api/payments/${id}/refund`, { method: 'POST' }),
  cancelPayment: (id: number) => request<ApiPayment>(`/api/payments/${id}/cancel`, { method: 'POST' }),

  // --- order lifecycle (D-036, Phase I) ------------------------------------
  /**
   * The order's immutable timeline. Both sides read the same list: a customer
   * and the seller must never be able to disagree about what happened.
   */
  paymentEvents: (id: number) => request<ApiOrderEvent[]>(`/api/payments/${id}/events`),

  /** Payer re-routes the money to a different destination (D-036.4). */
  setPaymentMethod: (id: number, methodId: number) =>
    request<ApiPayment>(`/api/payments/${id}/method`, {
      method: 'POST',
      body: JSON.stringify({ method_id: methodId }),
    }),

  /**
   * D-036.5 — the owner blocks the VPN when money does not arrive.
   * `reason` is required by the server: a block with no reason is unauditable.
   */
  blockPayment: (id: number, reason: string) =>
    request<ApiPayment>(`/api/payments/${id}/block`, {
      method: 'POST',
      body: JSON.stringify({ reason }),
    }),

  unblockPayment: (id: number) =>
    request<ApiPayment>(`/api/payments/${id}/unblock`, { method: 'POST' }),

  /**
   * D-036.6 — a CUSTOM order is released onto a chosen server + inbound.
   * This is the Super Admin's only way to turn a custom spec into a real VPN.
   */
  setReleaseTarget: (id: number, serverId: number, inboundId: number) =>
    request<ApiPayment>(`/api/payments/${id}/release-target`, {
      method: 'POST',
      body: JSON.stringify({ server_id: serverId, inbound_id: inboundId }),
    }),

  // --- pricing & quote (D-035.4) -------------------------------------------
  /** Every rate row: the platform default plus per-server overrides. */
  listPricing: () => request<ApiPricingRow[]>('/api/pricing'),

  /**
   * States the rate card for one scope.
   *
   * Only the fields provided are written — the server starts from the current
   * row, so a partial update cannot silently zero the rest.
   */
  savePricing: (input: {
    serverId?: number | null
    currency?: string
    ratePerGbCents?: number
    ratePerDayCents?: number
    ratePerDeviceCents?: number
    ratePerIpCents?: number
    minPriceCents?: number
    maxPriceCents?: number
    maxTrafficGb?: number
    maxDurationDays?: number
    maxDevices?: number
    maxIps?: number
  }) =>
    request<ApiPricingRow>('/api/pricing', {
      method: 'PUT',
      body: JSON.stringify({
        server_id: input.serverId ?? null,
        currency: input.currency,
        rate_per_gb_cents: input.ratePerGbCents,
        rate_per_day_cents: input.ratePerDayCents,
        rate_per_device_cents: input.ratePerDeviceCents,
        rate_per_ip_cents: input.ratePerIpCents,
        min_price_cents: input.minPriceCents,
        max_price_cents: input.maxPriceCents,
        max_traffic_gb: input.maxTrafficGb,
        max_duration_days: input.maxDurationDays,
        max_devices: input.maxDevices,
        max_ips: input.maxIps,
      }),
    }),

  /**
   * Prices a CUSTOM spec on the server.
   *
   * The buyer never sends an amount — this call is the only source of one, so
   * there is no code path where a customer states their own price (D-035.4).
   */
  quoteOrder: (spec: ApiSpec, serverId = 0) =>
    request<ApiQuote>('/api/orders/quote', {
      method: 'POST',
      body: JSON.stringify({
        server_id: serverId,
        traffic_gb: spec.traffic_gb,
        days: spec.days,
        devices: spec.devices,
        ips: spec.ips,
      }),
    }),

  // --- packages (D-035) ----------------------------------------------------
  /** The full catalogue (operator view). */
  listPlans: () => request<ApiPlan[]>('/api/plans'),

  /** Only what the caller may actually buy or claim. */
  listPackages: () => request<ApiPlan[]>('/api/packages'),

  /**
   * The customer catalogue, correctly typed (`GET /api/packages`).
   *
   * `listPackages()` above is kept unchanged because `packagesStore` already
   * consumes it, and repointing one endpoint at a second TypeScript shape is how a
   * shared module breaks a screen it was not meant to touch. That older signature
   * is nonetheless wrong — it promises `active`, `archived` and `slots_remaining`,
   * none of which this endpoint sends — so a caller that wants to READ the real
   * payload (Section 03's catalogue) calls this instead. One endpoint, one request;
   * only the declared shape differs, and this one matches the wire.
   *
   * See the note on `ApiPackage` for the verified field list.
   */
  listPackagesTyped: () => request<ApiPackage[]>('/api/packages'),

  getPlan: (id: number) => request<ApiPlan>(`/api/plans/${id}`),

  createPlan: (input: {
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
  }) => {
    const isCustom = input.isCustom || input.kind === 'CUSTOM_PREMIUM'
    const kind = isCustom ? 'PREMIUM' : input.kind
    return request<ApiPlan>('/api/plans', {
      method: 'POST',
      body: JSON.stringify({
        kind,
        name: input.name,
        server_id: input.serverId,
        inbound_id: input.inboundId,
        traffic_gb: input.trafficGb,
        duration_days: input.durationDays,
        device_limit: input.deviceLimit,
        price_cents: input.priceCents,
        currency: input.currency,
        slots_total: input.slotsTotal,
        is_custom: isCustom,
      }),
    })
  },

  /** Only the fields provided change; the catalogue is never rewritten wholesale. */
  updatePlan: (
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
  ) =>
    request<ApiPlan>(`/api/plans/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({
        name: input.name,
        traffic_gb: input.trafficGb,
        duration_days: input.durationDays,
        device_limit: input.deviceLimit,
        active: input.active,
        price_cents: input.priceCents,
        currency: input.currency,
        slots_total: input.slotsTotal,
        owner_user_id: input.ownerUserId,
        unowned: input.unowned,
      }),
    }),

  /** Archiving hides a package without destroying the sales history (D-030.4). */
  archivePlan: (id: number) => request<ApiPlan>(`/api/plans/${id}/archive`, { method: 'POST' }),

  deletePlan: (id: number) => request<void>(`/api/plans/${id}`, { method: 'DELETE' }),

  // --- server drill-down (L7, D-037) ---------------------------------------
  /**
   * Every client the panel holds, joined with its tag, owner and usage.
   *
   * Read-only against the panel. The filters are applied server-side so a
   * server with hundreds of clients does not ship all of them to the browser.
   */
  panelClients: (
    serverId: number,
    query: { tag?: string; assigned?: 'yes' | 'no'; usage?: 'empty' | 'low' | 'high'; q?: string } = {},
  ) => {
    const params = new URLSearchParams()
    if (query.tag) params.set('tag', query.tag)
    if (query.assigned) params.set('assigned', query.assigned)
    if (query.usage) params.set('usage', query.usage)
    if (query.q) params.set('q', query.q)
    const qs = params.toString()
    return request<ApiPanelClient[]>(`/api/servers/${serverId}/panel-clients${qs ? `?${qs}` : ''}`)
  },

  // --- VPN connection (P10 / step 11) --------------------------------------
  /**
   * The panel's own connection URIs for a VPN.
   *
   * `links` is passed through untouched — rewriting a vless:// URI produces
   * something that looks right and cannot connect. `source: 'unavailable'`
   * means the panel could not be read; it does NOT mean the VPN has no links.
   */
  vpnLinks: (id: number) => request<ApiVpnLinks>(`/api/vpns/${id}/links`),

  /**
   * Measures the SERVER's reachability (D-026). The response carries
   * `scope: 'server_reachability'` so the UI can never pass this off as the
   * user's own latency.
   */
  vpnPing: (id: number) => request<ApiVpnPing>(`/api/vpns/${id}/ping`, { method: 'POST' }),

  /**
   * P-09 — the protocol detail the owner listed: vless/vmess, tcp/ws, and the
   * credential (uuid / username / password) for the protocol in use.
   *
   * Read-only and panel-sourced: `source: 'unavailable'` still reports the
   * locally known protocol, so an outage never reads as "no credential exists".
   */
  vpnProtocol: (id: number) => request<ApiVpnProtocol>(`/api/vpns/${id}/protocol`),

  /**
   * P-04 — live traffic as a RATE (bytes/sec), computed from the last two sync
   * readings. `source: 'unavailable'` means "not measured yet", which the UI
   * shows as such rather than as 0 B/s.
   */
  vpnLiveTraffic: (id: number) =>
    request<ApiVpnLiveTraffic>(`/api/vpns/${id}/traffic/live`),

  // --- chat (P7) -----------------------------------------------------------
  /** The caller's own conversations. There is no "somebody else's". */
  listChatThreads: () => request<ApiChatThread[]>('/api/chat/threads'),

  /** Opens (or reuses) the conversation with a peer. Idempotent per pair. */
  openChatThread: (peerUserId: number) =>
    request<ApiChatThread>('/api/chat/threads', {
      method: 'POST',
      body: JSON.stringify({ peer_user_id: peerUserId }),
    }),

  getChatThread: (threadId: number) => request<ApiChatThread>(`/api/chat/threads/${threadId}`),

  /** `afterId` is a cursor, so a page never skips or repeats a message. */
  listChatMessages: (threadId: number, afterId = 0, limit = 100) =>
    request<ApiChatMessage[]>(`/api/chat/threads/${threadId}/messages?after=${afterId}&limit=${limit}`),

  sendChatMessage: (threadId: number, body: string) =>
    request<ApiChatMessage>(`/api/chat/threads/${threadId}/messages`, {
      method: 'POST',
      body: JSON.stringify({ body }),
    }),

  /** The author edits within the server's window; the old text is preserved. */
  editChatMessage: (threadId: number, messageId: number, body: string) =>
    request<ApiChatMessage>(`/api/chat/messages/${messageId}`, {
      method: 'PATCH',
      body: JSON.stringify({ body, thread_id: threadId }),
    }),

  /** Hides the message for the caller only — the peer's copy is evidence. */
  hideChatMessage: (threadId: number, messageId: number) =>
    request<void>(`/api/chat/messages/${messageId}?thread_id=${threadId}`, { method: 'DELETE' }),

  /**
   * Undo a hide: puts the message back in the CALLER's view only.
   * Idempotent, so a retried undo is not an error.
   */
  unhideChatMessage: (threadId: number, messageId: number) =>
    request<ApiChatMessage>(`/api/chat/messages/${messageId}/unhide`, {
      method: 'POST',
      body: JSON.stringify({ thread_id: threadId }),
    }),

  /** What THIS viewer deleted for himself — the restore path's index. */
  listHiddenChatMessages: (threadId: number, limit = 100) =>
    request<ApiChatMessage[]>(`/api/chat/threads/${threadId}/hidden?limit=${limit}`),

  markChatRead: (threadId: number) =>
    request<{ thread_id: number; read: boolean }>(`/api/chat/threads/${threadId}/read`, { method: 'POST' }),

  pinChatMessage: (threadId: number, messageId: number) =>
    request<{ thread_id: number; pinned_message_id: number }>(`/api/chat/threads/${threadId}/pin`, {
      method: 'POST',
      body: JSON.stringify({ message_id: messageId }),
    }),

  lockChatThread: (threadId: number, locked: boolean) =>
    request<{ thread_id: number; locked: boolean }>(`/api/chat/threads/${threadId}/lock`, {
      method: 'POST',
      body: JSON.stringify({ locked }),
    }),

  saveChatMessage: (threadId: number, messageId: number, on: boolean) =>
    request<{ message_id: number; saved: boolean }>(`/api/chat/messages/${messageId}/save`, {
      method: 'POST',
      body: JSON.stringify({ thread_id: threadId, on }),
    }),

  markChatMessageImportant: (threadId: number, messageId: number, on: boolean) =>
    request<{ message_id: number; important: boolean }>(`/api/chat/messages/${messageId}/important`, {
      method: 'POST',
      body: JSON.stringify({ thread_id: threadId, on }),
    }),
}
