import { Suspense, lazy, useEffect, useState } from 'react'
import { Routes, Route, Navigate, Outlet } from 'react-router-dom'
import { useAuthStore } from './stores/authStore'
import { totalUnread, useChatStore } from './stores/chatStore'
import { useServersStore } from './stores/serversStore'
import { useAdminsStore } from './stores/adminsStore'
import { useOffersStore } from './stores/offersStore'
import { useClientsStore } from './stores/clientsStore'
import { usePaymentsStore } from './stores/paymentsStore'
import { AppShell } from './components/layout'
import { SceneBackground } from './components/effects/SceneBackground'
import { ThemeStudioPanel } from './components/theme-studio/ThemeStudioPanel'
import { CommandPalette } from './components/palette/CommandPalette'
import { RequireAuth, roleHome } from './components/auth/RequireAuth'
/* ---------------------------------------------------------------------------
 * Route-level code splitting (Phase A / UI improvement programme).
 *
 * Before this, every one of the ~30 screens was a STATIC import, so a visitor
 * who only ever saw /login still downloaded the whole operator console — one
 * 754 kB main chunk, over Vite's 500 kB warning, which on a 1 vCPU box is a
 * first-paint cost paid by everybody for screens most accounts never open.
 *
 * The shell is NOT split: AppShell/Sidebar/Topbar/RequireAuth and the two
 * sign-in doors stay eager, because a Suspense fallback that replaced them
 * would unmount the sidebar and flicker the chrome on every navigation.
 *
 * Each page already imports its own stylesheet (`./login.css`, `./clients.css`
 * …), so splitting the route module splits that CSS into the same chunk — no
 * extra work, and a page that is never visited never ships its CSS.
 *
 * `lazy()` needs a module with a default export to be as terse as this; Vite
 * handles the named exports either way, so the mapping is explicit here rather
 * than relying on a barrel.
 * ------------------------------------------------------------------------- */
const SetupPage = lazy(() => import('./pages/SetupPage').then((m) => ({ default: m.SetupPage })))
const LoginPage = lazy(() => import('./pages/LoginPage').then((m) => ({ default: m.LoginPage })))
const StaffLoginPage = lazy(() => import('./pages/StaffLoginPage').then((m) => ({ default: m.StaffLoginPage })))
const RegisterPage = lazy(() => import('./pages/RegisterPage').then((m) => ({ default: m.RegisterPage })))
const ForgotPasswordPage = lazy(() =>
  import('./pages/ForgotPasswordPage').then((m) => ({ default: m.ForgotPasswordPage })),
)
const ResetPasswordPage = lazy(() =>
  import('./pages/ResetPasswordPage').then((m) => ({ default: m.ResetPasswordPage })),
)
const SamplePage = lazy(() => import('./pages/SamplePage').then((m) => ({ default: m.SamplePage })))

const DashboardPage = lazy(() => import('./pages/DashboardPage').then((m) => ({ default: m.DashboardPage })))
const ClientsPage = lazy(() => import('./pages/ClientsPage').then((m) => ({ default: m.ClientsPage })))
const ClientDetailPage = lazy(() => import('./pages/ClientDetailPage').then((m) => ({ default: m.ClientDetailPage })))
const VpnsPage = lazy(() => import('./pages/VpnsPage').then((m) => ({ default: m.VpnsPage })))
const StaffPage = lazy(() => import('./pages/StaffPage').then((m) => ({ default: m.StaffPage })))
const OffersPage = lazy(() => import('./pages/OffersPage').then((m) => ({ default: m.OffersPage })))
const ServersPage = lazy(() => import('./pages/ServersPage').then((m) => ({ default: m.ServersPage })))
const ServerDetailPage = lazy(() => import('./pages/ServerDetailPage').then((m) => ({ default: m.ServerDetailPage })))
const ClientDashboardPage = lazy(() =>
  import('./pages/ClientDashboardPage').then((m) => ({ default: m.ClientDashboardPage })),
)
/* S3: My VPNs is its own screen. The path is NESTED under /client on purpose —
 * `/vpns` belongs to the Super Admin console (`App.tsx`, D-043) and React Router
 * takes the first matching route, so declaring it twice would make one of the
 * two screens unreachable. That is the exact defect `App.tsx` documents for
 * `/vpn/:id`, and it is why the reseller's equivalent is `/admin-vpns`. */
const MyVpnsPage = lazy(() =>
  import('./pages/client/MyVpnsPage').then((m) => ({ default: m.MyVpnsPage })),
)
/* S6-S11 client shells. Each is a lazily-loaded route module like every other
 * page, so a client who never opens one never downloads it. */
const ClientOffersPage = lazy(() =>
  import('./pages/client/ClientOffersPage').then((m) => ({ default: m.ClientOffersPage })),
)
const ClientBillingPage = lazy(() =>
  import('./pages/client/ClientBillingPage').then((m) => ({ default: m.ClientBillingPage })),
)
const ClientUsagePage = lazy(() =>
  import('./pages/client/ClientUsagePage').then((m) => ({ default: m.ClientUsagePage })),
)
const ClientGuidesPage = lazy(() =>
  import('./pages/client/ClientGuidesPage').then((m) => ({ default: m.ClientGuidesPage })),
)
const ClientSupportPage = lazy(() =>
  import('./pages/client/ClientSupportPage').then((m) => ({ default: m.ClientSupportPage })),
)
const ClientNotificationsPage = lazy(() =>
  import('./pages/client/ClientNotificationsPage').then((m) => ({ default: m.ClientNotificationsPage })),
)
const VPNDetailPage = lazy(() => import('./pages/VPNDetailPage').then((m) => ({ default: m.VPNDetailPage })))
const BuyPremiumPage = lazy(() => import('./pages/BuyPremiumPage').then((m) => ({ default: m.BuyPremiumPage })))
const HistoryPage = lazy(() => import('./pages/HistoryPage').then((m) => ({ default: m.HistoryPage })))
const ChatPage = lazy(() => import('./pages/ChatPage').then((m) => ({ default: m.ChatPage })))
const PaymentsPage = lazy(() => import('./pages/PaymentsPage').then((m) => ({ default: m.PaymentsPage })))
const OrderDetailPage = lazy(() => import('./pages/OrderDetailPage').then((m) => ({ default: m.OrderDetailPage })))
const PackagesPage = lazy(() => import('./pages/PackagesPage').then((m) => ({ default: m.PackagesPage })))
const OrdersPage = lazy(() => import('./pages/OrdersPage').then((m) => ({ default: m.OrdersPage })))
const PricingPage = lazy(() => import('./pages/PricingPage').then((m) => ({ default: m.PricingPage })))
const AuditPage = lazy(() => import('./pages/AuditPage').then((m) => ({ default: m.AuditPage })))
const NotificationsPage = lazy(() => import('./pages/NotificationsPage').then((m) => ({ default: m.NotificationsPage })))
const ProfilePage = lazy(() => import('./pages/ProfilePage').then((m) => ({ default: m.ProfilePage })))
const AdminOverviewPage = lazy(() => import('./pages/AdminOverviewPage').then((m) => ({ default: m.AdminOverviewPage })))
const AdminServersPage = lazy(() => import('./pages/AdminServersPage').then((m) => ({ default: m.AdminServersPage })))
const AdminClientsPage = lazy(() => import('./pages/AdminClientsPage').then((m) => ({ default: m.AdminClientsPage })))
const AdminVpnsPage = lazy(() => import('./pages/AdminVpnsPage').then((m) => ({ default: m.AdminVpnsPage })))
const SaleFlowPage = lazy(() => import('./pages/SaleFlowPage').then((m) => ({ default: m.SaleFlowPage })))
const RetailPricePage = lazy(() => import('./pages/RetailPricePage').then((m) => ({ default: m.RetailPricePage })))
const SuperSettingsPage = lazy(() =>
  import('./pages/SuperSettingsPage').then((m) => ({ default: m.SuperSettingsPage })),
)

/* AdminsPage is no longer routed: `/admins` redirects to `/staff`, which now owns
 * the quota, the panel scope, the profile and the password (Phase-1 UX, P1.3).
 * The module is kept because `adminsStore` still backs the reseller list and a
 * future "reseller-only" view can reuse it without rediscovering the API. */
/* D-084: the role-scoped navigation model lives in `navModel.ts` so the Topbar
 * and Breadcrumbs read the same tree the sidebar renders — one source of truth,
 * and no import cycle back to this module. */
import { navigationFor } from './components/layout/navModel'


/** Role-aware landing — sends each role to its own home screen. */
function RoleLanding() {
  const role = useAuthStore((s) => s.user?.role ?? 'super-admin')
  return <Navigate to={roleHome(role)} replace />
}

/**
 * The pending state for a lazily-loaded route.
 *
 * Deliberately a calm, skeleton-shaped block rather than a spinner: on a slow
 * link the chunk fetch is the visible wait, and a full-page spinner that
 * disappears into a real page is more jarring than a placeholder that already
 * has the page's shape. Announced once via role="status" so a screen reader
 * user is not left on a silent, empty page.
 */
function RouteFallback() {
  return (
    <div className="route-fallback" role="status" aria-live="polite">
      <span className="route-fallback__label">Loading…</span>
    </div>
  )
}

/**
 * In-shell pending state. Rendered as a Route element so it lands INSIDE
 * `.app-content` (AppShell's <Outlet/>), keeping the sidebar, topbar and their
 * scroll position mounted while the page chunk arrives.
 */
function ShellSuspense() {
  return (
    <Suspense fallback={<RouteFallback />}>
      <Outlet />
    </Suspense>
  )
}

export default function App() {
  const [studioOpen, setStudioOpen] = useState(false)
  const [paletteOpen, setPaletteOpen] = useState(false)

  /* Restore the session from the HttpOnly cookie exactly once on start. */
  const hydrate = useAuthStore((s) => s.hydrate)
  useEffect(() => {
    void hydrate()
  }, [hydrate])

  /* Live unread badge on the Chat nav item (same pattern as the bell). The
     server already scoped the list to this account, so the badge is simply the
     sum of the unread counts it reported. */
  const chatThreads = useChatStore((s) => s.threads)
  const loadChatThreads = useChatStore((s) => s.load)
  const chatUnread = totalUnread(chatThreads)
  const signedIn = useAuthStore((s) => !!s.user)
  /** The signed-in role — decides which lists the nav may fetch. */
  const navRole = useAuthStore((s) => s.user?.role ?? 'client')

  /* Nav counts are REAL, not literals (F-019). They come from the same lists the
     destination pages render, so a badge can never disagree with the page it
     points at — and a count only appears once the list has actually loaded,
     instead of a stale number left over from a mock. */
  const servers = useServersStore((s) => s.servers)
  const loadServers = useServersStore((s) => s.load)
  const admins = useAdminsStore((s) => s.admins)
  const loadAdmins = useAdminsStore((s) => s.load)
  const offers = useOffersStore((s) => s.offers)
  const loadOffers = useOffersStore((s) => s.load)
  const clients = useClientsStore((s) => s.clients)
  const loadClients = useClientsStore((s) => s.load)
  /* The payments badge is the operator's queue depth: money is the first thing
     a Super Admin clears, so it must surface before the page is even opened. */
  const payments = usePaymentsStore((s) => s.payments)
  const loadPayments = usePaymentsStore((s) => s.load)

  useEffect(() => {
    if (!signedIn) return
    void loadChatThreads()
    // Only fetch what this role's navigation can show: a client asking for the
    // reseller list would just earn a 403.
    if (navRole === 'super-admin') {
      void loadServers()
      void loadAdmins()
      void loadOffers()
      void loadPayments()
    } else if (navRole === 'admin') {
      void loadServers()
      void loadClients()
      void loadPayments()
    }
  }, [signedIn, navRole, loadChatThreads, loadServers, loadAdmins, loadOffers, loadClients, loadPayments])

  /* Payments: the badge counts the QUEUE (PENDING + AWAITING_CONFIRMATION) — the
     work needing a decision — never the size of the ledger. A badge must point
     at work, not at history. */
  const paymentQueue = payments.filter(
    (p) => p.status === 'PENDING' || p.status === 'AWAITING_CONFIRMATION',
  ).length

  const counts: Record<string, number> = {
    '/payments': paymentQueue,
    '/servers': servers.length,
    '/admin-servers': servers.length,
    '/offers': offers.length,
    '/admins': admins.length,
    '/admin-clients': clients.length,
    '/chat': chatUnread,
  }

  const nav = navigationFor(navRole).map((g) => ({
    ...g,
    items: g.items.map((i) => {
      const n = counts[i.to] ?? 0
      // `count: undefined` renders no chip at all, which is the honest answer for
      // an empty list (a "0" chip is noise on every fresh installation).
      return n > 0 ? { ...i, count: n } : { ...i, count: undefined }
    }),
  }))

  /* Global ⌘K / Ctrl+K */
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setPaletteOpen((v) => !v)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  /* Profile page → Theme Studio (decoupled via event) */
  useEffect(() => {
    const open = () => setStudioOpen(true)
    window.addEventListener('veylix:open-theme-studio', open)
    return () => window.removeEventListener('veylix:open-theme-studio', open)
  }, [])

  return (
    <>
      <SceneBackground />
      <Routes>
        {/* Public: auth + QA. Their own boundary keeps a cold chunk load from
            blanking the screen before the shell exists. */}
        <Route
          path="/setup"
          element={
            <Suspense fallback={<RouteFallback />}>
              <SetupPage />
            </Suspense>
          }
        />
        <Route
          path="/login"
          element={
            <Suspense fallback={<RouteFallback />}>
              <LoginPage />
            </Suspense>
          }
        />
        {/* D-041: staff (admin + super admin) sign in at their own door. The
            shortcuts keep old bookmarks working without duplicating the page. */}
        <Route
          path="/staff/login"
          element={
            <Suspense fallback={<RouteFallback />}>
              <StaffLoginPage />
            </Suspense>
          }
        />
        <Route path="/admin/login" element={<Navigate to="/staff/login?role=admin" replace />} />
        <Route path="/super/login" element={<Navigate to="/staff/login?role=super" replace />} />
        <Route
          path="/register"
          element={
            <Suspense fallback={<RouteFallback />}>
              <RegisterPage />
            </Suspense>
          }
        />
        {/* D-057: the component/theme showcase is a development tool, not a
            feature. It renders demo data and its own stylesheet, so shipping it
            publicly puts fake content in front of real customers. It exists in
            `npm run dev` (import.meta.env.DEV) and is compiled out of a
            production build. */}
        {import.meta.env.DEV && <Route path="/sample" element={<Suspense fallback={<RouteFallback />}><SamplePage /></Suspense>} />}
        {/* D-044: BOTH doors lead here — an admin who forgot a password must not be
            told to "use the staff login" when the only thing missing is a code. */}
        <Route
          path="/forgot"
          element={
            <Suspense fallback={<RouteFallback />}>
              <ForgotPasswordPage />
            </Suspense>
          }
        />
        <Route
          path="/reset"
          element={
            <Suspense fallback={<RouteFallback />}>
              <ResetPasswordPage />
            </Suspense>
          }
        />
        {/* Role-guarded shell. The session is a real backend HttpOnly cookie
            (Stages E2); roles come from the server, never from local state. */}
        <Route element={<RequireAuth />}>
        <Route
          element={
            <AppShell
              groups={nav}
              userName="Super Admin"
              onTheme={() => setStudioOpen(true)}
              onOpenPalette={() => setPaletteOpen(true)}
            />
          }
        >
          {/* ONE boundary for every in-shell route. It sits here — BELOW
              AppShell — on purpose: a fallback rendered above the shell would
              unmount the sidebar, topbar and nav state on every first visit to
              a lazily-loaded page, which reads as a full app reload. */}
          <Route element={<ShellSuspense />}>
          <Route index element={<RoleLanding />} />
          {/* Super Admin only */}
          <Route element={<RequireAuth roles={['super-admin']} />}>
            <Route path="/dashboard" element={<DashboardPage loading={false} />} />
            <Route path="/servers" element={<ServersPage />} />
            <Route path="/offers" element={<OffersPage />} />
            <Route path="/admins" element={<Navigate to="/staff" replace />} />
            <Route path="/audit" element={<AuditPage />} />
                <Route path="/settings" element={<SuperSettingsPage />} />
            {/* D-042: the client console — every customer, with block and limits. */}
            <Route path="/clients" element={<ClientsPage />} />
            <Route path="/clients/:id" element={<ClientDetailPage />} />
            {/* D-043: the VPN console — panel clients included, assigned or not. */}
            <Route path="/vpns" element={<VpnsPage />} />
            {/*
             * The Super Admin's per-VPN detail. Without this, "click a VPN to see
             * its full detail" had nowhere to go.
             */}
            <Route path="/vpns/:serverId/:panelEmail" element={<VPNDetailPage />} />
            {/*
             * `/vpn/:id` is deliberately NOT declared here.
             *
             * It used to appear BOTH in this super-admin block and in the client
             * block below. React Router takes the FIRST matching route, so a client
             * opening `/vpn/<id>` matched the super-admin guard, was refused by
             * `RequireAuth`, and landed back on `/client` — the VPN detail screen
             * was UNREACHABLE for the only role that owns it. The operator path is
             * `/vpns/:serverId/:panelEmail` above, so this block does not need it.
             * One path, one owner.
             */}
            {/* D-044: the staff console — admins, their quotas, their panels. */}
            <Route path="/staff" element={<StaffPage />} />
            {/* L4 (D-035.4): the rate card a CUSTOM order is priced from. */}
            <Route path="/pricing" element={<PricingPage />} />
            {/* L5 (D-035): the package catalogue — FREE / PREMIUM / CUSTOM. */}
            <Route path="/packages" element={<PackagesPage />} />
            {/* L3 (D-036.6): choose the server + inbound a CUSTOM order is built on. */}
            <Route path="/orders/:id" element={<OrderDetailPage />} />
            {/* L7 (Phase G / D-037): one server's VPNs, owners and usage. */}
            <Route path="/servers/:id" element={<ServerDetailPage />} />
          </Route>
          {/* Admin (reseller) only */}
          <Route element={<RequireAuth roles={['admin']} />}>
            <Route path="/admin" element={<AdminOverviewPage />} />
            <Route path="/admin-servers" element={<AdminServersPage />} />
            <Route path="/admin-clients" element={<AdminClientsPage />} />
            {/* Phase-4 UX (R2): the reseller's own VPNs. Without this an admin
                could see the clients it sold to but never the VPNs it sold them. */}
            <Route path="/admin-vpns" element={<AdminVpnsPage />} />
            <Route path="/sale" element={<SaleFlowPage />} />
            <Route path="/retail-price" element={<RetailPricePage />} />
          </Route>
          {/* Client only */}
          <Route element={<RequireAuth roles={['client']} />}>
            <Route path="/client" element={<ClientDashboardPage />} />
            {/* S3: owned VPN inventory, with search/filter/sort/paging. Nested
                under /client because `/vpns` is the Super Admin console — one
                path, one owner. */}
            <Route path="/client/vpns" element={<MyVpnsPage />} />
            {/* S6-S11 outer layer: the six remaining Client Portal sections.
                Each is a SHELL — the frame, the route and the honest empty
                state. Content lands stage by stage. They are nested under
                `/client/*` for the same one-path-one-owner reason as above. */}
            <Route path="/client/offers" element={<ClientOffersPage />} />
            <Route path="/client/billing" element={<ClientBillingPage />} />
            <Route path="/client/usage" element={<ClientUsagePage />} />
            <Route path="/client/guides" element={<ClientGuidesPage />} />
            <Route path="/client/support" element={<ClientSupportPage />} />
            <Route path="/client/notifications" element={<ClientNotificationsPage />} />
            <Route path="/vpn/:id" element={<VPNDetailPage />} />
            {/* The two S3-era destinations are kept as-is: 30+ committed
                verify scripts and the breadcrumb table depend on these exact
                strings, and they already work. They are reached from the
                client nav tree, not duplicated under /client. */}
            <Route path="/buy" element={<BuyPremiumPage />} />
            <Route path="/history" element={<HistoryPage />} />
          </Route>
          {/* Shared (super-admin + admin) */}
          <Route element={<RequireAuth roles={['super-admin', 'admin']} />}>
            <Route path="/payments" element={<PaymentsPage />} />
            {/* L2 (D-031/D-036): the order queue — confirm, reject, block, unblock. */}
            <Route path="/orders" element={<OrdersPage />} />
          </Route>
          {/* Shared: all roles */}
          <Route path="/notifications" element={<NotificationsPage />} />
          <Route path="/profile" element={<ProfilePage />} />
          <Route path="/chat" element={<ChatPage />} />
          <Route path="/chat/:threadId" element={<ChatPage />} />
          <Route path="*" element={<RoleLanding />} />
          </Route>
          </Route>
        </Route>
      </Routes>
      {studioOpen && <ThemeStudioPanel onClose={() => setStudioOpen(false)} />}
      <CommandPalette
        key={paletteOpen ? 'open' : 'closed'}
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        onOpenStudio={() => {
          setPaletteOpen(false)
          setStudioOpen(true)
        }}
      />
    </>
  )
}
