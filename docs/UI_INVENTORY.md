# VEYLIX VPN — UI INVENTORY

**Type:** Living document — update this file whenever UI ships, changes or is removed (Bible §0 process rule).
**Last synced:** 2026-10-23 · **Rule ref:** D-004 (scope control), D-008 (theme/UI 2.0), D-009 (in-house palette), D-010 (stage order)
**Style:** token-driven (`styles/tokens.css` is the single source); no hardcoded theme colors in components.
**Data:** **partially wired.** Auth (`authStore`) and Server integrations (`serversStore`) talk to the real Go backend through `lib/api.ts`; every other screen still reads **API-shaped mock stores** (`offersStore`, `adminsStore`, `paymentsStore`, `chatStore`, `notifStore`, `auditStore`, `peopleStore`, `vpnStore` — D-006). Pending wiring list in §10.

---

## 1. Screens & routes (`frontend/src/App.tsx` — NAV at 46-77, routes at 126-178)

| Route | Screen | Guard | Files |
|---|---|---|---|
| `/login` | Sign-in (split-screen: brand panel + form, Google pending) | public; signed-in → role home; **no accounts yet → `/setup`** | `pages/LoginPage.tsx`, `pages/auth.css` — **real API** |
| `/setup` | **First-run wizard: create the one and only Super Admin** (only while `bootstrap_required`) | public; already bootstrapped → `/login` | `pages/SetupPage.tsx` — **real API** |
| `/register` | Create account (name/email/password/confirm + policy hint) | public; signed-in → role home | `pages/RegisterPage.tsx` — **real API** |
| `/sample` | Theme/UI QA page (all primitives + sub-theme picker, full width) | public (QA tool) | `pages/SamplePage.tsx`, `pages/sample.css` |
| `/` | Role landing → redirects to that role's home | `RequireAuth` | `App.tsx` (`RoleLanding`, `roleHome`) |
| `/dashboard` | Super Admin overview (metrics, charts, live pulse) | `super-admin` | `pages/DashboardPage.tsx`, `pages/dashboard.css` |
| `/servers` | Server integrations (add/edit/test/disable/remove) | `super-admin` | `pages/ServersPage.tsx`, `pages/servers.css`, `stores/serversStore.ts` — **real API** |
| `/offers` | VPN offers (Free offer rules, claims, active toggle) | `super-admin` | `pages/OffersPage.tsx`, `pages/offers.css`, `stores/offersStore.ts` (mock) |
| `/admins` | Admin management (create/suspend/quota/servers) | `super-admin` | `pages/AdminsPage.tsx`, `pages/admins.css`, `stores/adminsStore.ts` (mock) |
| `/audit` | Audit log (filter chips, entries) | `super-admin` | `pages/AuditPage.tsx`, `stores/auditStore.ts` (mock; the real `GET /api/audit` exists) |
| `/admin` | Admin overview (quota, clients, sales) | `admin` | `pages/AdminOverviewPage.tsx` |
| `/admin-servers` | Assigned servers (role-filtered) | `admin` | `pages/AdminServersPage.tsx` |
| `/admin-clients` | Admin's clients + assign-Premium dialog (quota-gated) | `admin` | `pages/AdminClientsPage.tsx` |
| `/sale` | Premium sale flow (3-step, audited confirm) | `admin` | `pages/SaleFlowPage.tsx` |
| `/retail-price` | Retail price editor | `admin` | `pages/RetailPricePage.tsx` |
| `/client` | Client dashboard (Free offers + own VPN cards) | `client` | `pages/ClientDashboardPage.tsx`, `pages/client.css` |
| `/vpn/:id` | VPN detail (status/usage/limits/QR/link/guide/seller, unavailable state) | `client` | `pages/VPNDetailPage.tsx` |
| `/buy` | Buy Premium (3-step + pending order) | `client` | `pages/BuyPremiumPage.tsx` |
| `/history` | Purchase/assignment history + pending banner | `client` | `pages/HistoryPage.tsx` |
| `/payments` | Payments (confirm/audit trail), prices card | `super-admin`, `admin` | `pages/PaymentsPage.tsx`, `pages/payments.css` |
| `/chat`, `/chat/:threadId` | Internal chat (ChatPanel, role-filtered threads) | all roles | `pages/ChatPage.tsx`, `pages/chat.css`, `components/chat/*` |
| `/notifications` | Notifications inbox | all roles | `pages/NotificationsPage.tsx`, `stores/notifStore.ts` |
| `/profile` | Profile + password change dialog + theme prefs entry (role is server-owned, read-only) | all roles | `pages/ProfilePage.tsx` — **password: real API** |
| `*` | Fallback → role home | `RequireAuth` | `App.tsx` |

**Guard:** `components/auth/RequireAuth.tsx` → unauthenticated `/login` (keeps destination); wrong role → own role home.
**Session:** `stores/authStore.ts` — **real backend session** via `lib/api.ts` (HttpOnly cookie; no credential in JS). Session restore waits for `hydrate()` before rendering, so a refresh does not flash the login page.
**Known gaps** (tracked in `GAP_ANALYSIS.md` §6): Super Admin *clients* screen, *incidents* screen, Admin *own contacts/payment details* screen, email-based password **reset** (change is implemented, reset is not).
### 1-A. Screens required by D-024 … D-028 (2026-10-24) — **specified, not built**

> None of these exist yet. They are listed so the UI work has one checklist, and so the screens in §1 are never mistaken for them.

| Screen | Role | Must show | Decision |
|---|---|---|---|
| **Panel clients** (`/panel-clients`) | super-admin | **Every** client on the panel — tagged `FREE`/`PREMIUM` or **untagged** — with the panel's own `totalGB`, `expiryTime`, `limitIp`, `limitHwid`, `enable`; filter by server / tag / untagged; tag or re-tag (refused once issued) | D-024.3, D-025.1/.2, D-028 |
| **Server panel drill-down** (`/servers/:id`) | super-admin | URL + API edit, live client list loaded automatically, filter, per-server ping, server CPU/RAM/network | D-024, owner Q16 |
| **Super Admin dashboard (extended)** | super-admin | Counts: admins, clients, servers, FREE (issued/unissued), PREMIUM; totals across all | owner Q16 |
| **Admin deal summary** | admin | Only their own deals: their clients, their sold VPNs, their quota | owner Q16 |
| **Client summary** | client | Their own VPNs, FREE claim allowance, expiry | owner Q16 |
| **VPN detail — PREMIUM** (rebuild `/vpn/:id`) | client | QR + copyable link, obtained→expiry, **"Test ping" button** with the measured result, usage charts with **7d/30d/All** + **line/bar/area/pie** toggle, separate **CPU / RAM / network** charts with range toggle, editable `uuid`/password | D-026, D-028.6 |
| **VPN detail — FREE** (same route, reduced) | client | QR + link, ping button, **7-day** usage chart only | D-026 |
| **FREE catalogue** (`/free`) | client | Public list of FREE-tagged, unassigned VPNs, with the **claim cap rule shown before claiming** ("3 at a time, 5 per 30 days; releasing does not refund") | D-025.3/.4/.5 |
| **Released VPNs / restore** | client | Pending removals with a **countdown and an Undo** | D-027.2 |
| **Chat storage meter** | client | Usage against the Super-Admin-set budget + "delete my messages to free space" | D-027.6 |
| **Client storage limits dialog** | super-admin | Per-client chat budget (bytes and/or messages) | D-027.6 |

**Status styling (all screens):** `ACTIVE` · `SUSPENDED` · `BLOCKED` · `EXPIRED` · `TRAFFIC_DEPLETED` · `SERVER_UNAVAILABLE` · `REVOKED` must each read as a distinct state, and every colour must come from **theme tokens** so the Theme Studio recolours them (D-026.4, owner Q17). A blocked/suspended VPN shows a **red note with the reason**, and the client may remove it (owner Q17).



## 2. Shell (`components/layout/`)

| Element | Features | File |
|---|---|---|
| Sidebar | fixed 250px · **rail collapse** (72px icon-only, desktop toggle) · active-item indicator bar · **count chips (99+ cap)** · group labels · brand + logo | `Sidebar.tsx`, `layout.css` |
| Sidebar footer | user mini-card → **account dropdown** (Profile/Settings/Sign out) | `AppShell.tsx` |
| Topbar | glass · **scroll shadow** · sticky · mobile menu toggle | `Topbar.tsx` |
| Topbar search | button → **⌘K palette** + `Ctrl K` kbd hint | `Topbar.tsx` |
| Topbar bell | **notifications popover** (unread dots, per-item mark read, mark-all-read, empty state, live badge, View-all → page) | `Topbar.tsx`, `stores/notifStore.ts` (D-006 mock) |
| Topbar avatar | accent-gradient initial + name → account dropdown | `Topbar.tsx` |
| Theme button | opens Theme Studio overlay | `AppShell.tsx` |
| Mobile | drawer + scrim < 1024px | `layout.css` |

## 3. Component library (`components/ui/`)

| Component | Key features |
|---|---|
| Button | 5 variants × 3 sizes, loading, leading icon, gradient primary (top-light), secondary hover token, focus outline |
| Card | radius/shadow tokens, hover personalities per UI language, optional title/actions |
| Badge / StatusBadge | 6 tones, dot option (never color-only — REQUIREMENTS §7), platform status model map |
| Input / Textarea / Select | label, hint, error (role=alert), error border, hover transition, search variant |
| Table | **sortable columns** (aria-sort, asc→desc→off), hover rows, empty state slot |
| Tabs | **arrow-key/Home/End** navigation, roving tabindex, per-language skins |
| Dialog | **focus trap + initial focus + restore (WAI-ARIA)**, Esc, scrim, enter animation, wide variant |
| Toast | 4 kinds × bar/line/badge attachment styles, exit animation, aria-live viewport |
| Dropdown / Tooltip / Switch / Checkbox / Pagination / Skeleton (**shimmer sweep**) / Spinner / **EmptyState** · **PageHeader** (title+subtitle+actions) | per-theme skins |

Charts live in §4; charts are also part of the library (`components/charts/`).

## 4. Charts (`components/charts/`, SVG + tokens, zero chart-lib deps)

| Chart | Interactivity |
|---|---|
| LineChart | **hover crosshair + nearest-index highlight dots + HTML tooltip** (animated series rows, edge-flip), **legend chips — click toggles series**, area/smooth/sharp styles, per-language weights |
| BarChart | pointer-slot hover → **dim others + outline + exact-value tooltip**, `showValues` option |
| DonutChart | hover segment/legend → **center % swap** + arc emphasis + dim others, **legend rows with live %** |
| Sparkline / Progress | inline trend + labeled progress (tones) |

Per-language chart personalities: glass = dim/thin + glow, neon = thick/glow/dashed, editorial = hairlines, flat = static (`charts.css [data-ui]`).

## 5. Theme system

**5 main presets** (`styles/tokens.css` + `theme/themes.ts` PRESET_COLORS — full token table in the report Appendix B):

| Preset | Identity | Primary / Secondary |
|---|---|---|
| Modern (default) | Indigo Nebula | `#5b5bd6` / `#06b6d4` |
| Dark | Midnight Aurora | `#7c3aed` / `#22d3ee` |
| Light | Cloud Indigo | `#4f46e5` / `#f59e0b` |
| Classic | Slate Pro | `#2f6bef` / `#5e7191` |
| Gaming | Neon Prime | `#00f0ff` / `#d6117d` |

**40 sub-themes** (8 per preset — `theme/subthemes.ts`; official community hexes: Nord, Dracula, Tokyo Night, Catppuccin Mocha/Latte, Solarized Light, One Light, Rosé Pine Dawn, Gruvbox). Every variant defines accents **+ contrast inks** (AA), surfaces, borders, text and status colors; contrast audited (2026-10-22).

Theme Studio (`components/theme-studio/`): preset picker, sub-theme tiles, color pickers (hex-validated) with contrast inks, typography, radius/border/shadow, effects **Off/Subtle/Enhanced**, glass toggle, low-resource mode, Style/Motion tabs, reset. Persist: localStorage (`veylix-theme` persist v4 migration).

Per-preset UI languages `[data-ui]`: soft / glass / editorial / flat / neon (`styles/ui-themes.css`).

## 6. Interactions & motion (`styles/base.css`, `components/ui/ui.css`, `hooks/useViewTransitionNavigate.ts`)

- **⌘K command palette** — in-house combobox: Navigation/Themes/Actions groups, filter-as-you-type, ↑↓/Enter/Esc, glass + elevation-3 (`components/palette/`)
- **View Transitions API** route changes (sidebar + palette) — fade/rise, feature-detected fallback, reduced-motion skip
- Gradient primary buttons (top-light), secondary hover tokens, ghost-danger on glass skin
- Elevation ladder `--elev-1/2/3` · motion tokens `--ease cubic-bezier(.2,.8,.2,1)`, `--dur-fast/dur/dur-slow`
- Skeleton shimmer sweep · toast exit animation · card mount/stagger/hover (lift/glow/tilt) per Style tab · pointer-tilt 3D tiles (dashboard)
- **Alive dashboard**: range chips (24h/7d/30d dataset swap), count-up metrics (`useCountUp`), sparklines, live pulse ticker

## 7. States

Loading: skeleton shimmer (full-page rows on dashboard) · Empty: EmptyState (its title renders as an `<h2>`, so an empty or not-found screen still has a heading — I-19) · Error: inline field errors + toasts · Notifications: unread tint + badge counts · Charts: hover/hidden-series/dim states.

**Data honesty note (2026-10-23):** all of the above states are driven by mock stores. States that depend on a real backend — stale/unknown server status, partial-failure banners, `REVOCATION_PENDING`, "history unavailable" — are **designed but not backed by data** yet (`GAP_ANALYSIS.md` §3, §7).


## 8. Accessibility
- WAI-ARIA dialog: focus trap, initial focus, focus restore, aria-modal
- APG tablist: roving tabindex + arrow keys; tabs/sort/palette announce state (`aria-selected`, `aria-sort`, `aria-expanded`, `aria-activedescendant`)
- Status colors never color-only (dot + text; REQUIREMENTS §7)
- `prefers-reduced-motion` respected globally + per-component; low-resource mode kills 3D/glass/animations

## 9. Metrics (2026-10-23 build)

- `tsc -b && vite build` ✓ · oxlint **0 errors / 0 warnings** (94 files, 116 rules)
- Bundle: main JS 477.04 kB (141.18 gz) · CSS 136.23 kB (23.01 gz) · `threeScene` 537.96 kB (134.20 gz, lazy chunk)
- Source size: 61 TS/TSX + 18 CSS files · dependency tree: react, react-dom, react-router-dom, zustand, lucide-react, three (lazy)
- Automated tests: **none** (no runner) — `IMPROVEMENT_LEDGER.md` F-005

## 10. Pending wiring (D-006 exit list)

Each mock store keeps its public selector surface when it is re-pointed at the real API, so pages do not need a rewrite (`new_update.md` §3 "smallest safe improvement").

| Mock store | Real source (planned) | Notes |
|---|---|---|
| `authStore` | `POST /api/auth/login`, `/api/auth/google`, `GET /api/me` | Stage E2 |
| `serversStore` | `GET/POST/PATCH /api/servers`, `POST /api/servers/:id/test` | health = `GET /panel/api/server/status` (F-002) |
| `offersStore` | `/api/offers` + `/api/inbounds` (picker from `/inbounds/options`) | Stage E3/E4 |
| `adminsStore` | `/api/admins`, `/api/admins/:id/quota` | Stage E2/E4 |
| `vpnStore` | `/api/vpns` (backed by `vpn_instances` + panel reads) | Stage E5 |
| `paymentsStore` | `/api/payments` (+ audited confirm) | Stage E4 |
| `chatStore` | `/api/chat/*` with server-side participant checks | Stage E2 |
| `notifStore` | `/api/notifications` | Stage E5 |
| `auditStore` | `/api/audit` (read-only) | Stage E2 |
| `peopleStore` | `/api/clients` (role-scoped) | Stage E2 |

**Sync rule:** every UI change ships with this file updated in the same commit (Bible §0).
