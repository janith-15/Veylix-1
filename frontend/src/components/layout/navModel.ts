import type { UserRole } from '../../stores/authStore'
import {
  Activity,
  Bell,
  BookOpen,
  Boxes,
  ClipboardList,
  Coins,
  CreditCard,
  Globe,
  LayoutDashboard,
  MessageSquare,
  Package,
  ScrollText,
  Server,
  Settings2,
  ShieldCheck,
  ShoppingBag,
  User,
  UserCog,
  Users,
  Wallet,
} from 'lucide-react'
import type { AppNavGroup } from './Sidebar'

/* ---------------------------------------------------------------------------
 * ROLE-SCOPED NAVIGATION MODEL (D-084)
 *
 * The audit found the old model was ONE flat array of 22 entries in two generic
 * groups ("Main", "Operations"), each item filterable by `roles`. That produced
 * a Super Admin sidebar of 12 items where Settings and Rate card sat beside
 * daily Orders and Payments, and a CLIENT sidebar of 6 items of which none
 * named a server, a connection or usage — the three things a VPN customer
 * actually opens the app for. `/vpn/:id`, the only VPN-centric screen in the
 * product, had no nav entry at all.
 *
 * So the array is now built PER ROLE. Three roles are three different products;
 * they get three different structures instead of one structure fighting itself.
 *
 * Two invariants hold this together:
 *
 *   1. EVERY `to` below is a route actually declared in App.tsx. This is
 *      navigation, not a wish-list — inventing a destination here would create
 *      a dead link that the role guards turn into a silent redirect, which is
 *      exactly the "confusing navigation" the redesign exists to remove.
 *   2. No route PATH changes. 30+ committed `verify-*.mjs` scripts and the
 *      server-side role guards depend on these strings; only labels and
 *      grouping are redesigned.
 *
 * This lives in its own module (not App.tsx) so the Topbar and Breadcrumbs can
 * read the SAME tree the sidebar renders without importing the app root — one
 * source of truth, and no import cycle.
 * ------------------------------------------------------------------------- */

const SUPER_ADMIN_NAV: AppNavGroup[] = [
  {
    label: 'Platform',
    items: [{ to: '/dashboard', label: 'Overview', icon: LayoutDashboard }],
  },
  {
    label: 'VPN Management',
    items: [
      { to: '/vpns', label: 'All VPNs', icon: Globe },
      { to: '/offers', label: 'VPN Offers', icon: Package },
    ],
  },
  {
    label: 'Infrastructure',
    items: [{ to: '/servers', label: 'Servers', icon: Server }],
  },
  {
    label: 'People & Access',
    items: [
      { to: '/staff', label: 'Staff & Admins', icon: UserCog },
      { to: '/clients', label: 'Clients', icon: Users },
    ],
  },
  {
    label: 'Commerce',
    items: [
      { to: '/orders', label: 'Orders', icon: ClipboardList },
      { to: '/payments', label: 'Payments', icon: CreditCard },
      { to: '/packages', label: 'Packages', icon: Boxes },
      { to: '/pricing', label: 'Rate card', icon: Coins },
    ],
  },
  {
    label: 'System',
    items: [
      { to: '/audit', label: 'Audit log', icon: ScrollText },
      { to: '/settings', label: 'Settings', icon: Settings2 },
    ],
  },
  {
    label: 'Account',
    items: [
      { to: '/chat', label: 'Chat', icon: MessageSquare },
      { to: '/notifications', label: 'Notifications', icon: Bell },
      { to: '/profile', label: 'Profile', icon: User },
    ],
  },
]

const ADMIN_NAV: AppNavGroup[] = [
  {
    label: 'Business',
    items: [{ to: '/admin', label: 'Overview', icon: LayoutDashboard }],
  },
  {
    label: 'Sell',
    items: [
      { to: '/sale', label: 'Sell premium', icon: ShoppingBag },
      { to: '/admin-vpns', label: 'My VPNs', icon: Globe },
      // Labelled "My clients" on purpose: the super admin's `/clients` is a
      // different table. Two nav items both called "Clients" taught a reseller
      // to expect the wrong page.
      { to: '/admin-clients', label: 'My clients', icon: Users },
      { to: '/admin-servers', label: 'Assigned servers', icon: Server },
    ],
  },
  {
    label: 'Commerce',
    items: [
      { to: '/orders', label: 'Orders', icon: ClipboardList },
      { to: '/payments', label: 'Payments', icon: CreditCard },
      { to: '/packages', label: 'Packages', icon: Boxes },
      { to: '/retail-price', label: 'Retail price', icon: Wallet },
    ],
  },
  {
    label: 'Account',
    items: [
      { to: '/chat', label: 'Chat', icon: MessageSquare },
      { to: '/notifications', label: 'Notifications', icon: Bell },
      { to: '/profile', label: 'Profile', icon: User },
    ],
  },
]

/* Client nav leads with the VPN, not with the shop. The old order put
 * "My VPNs" and "Buy premium" side by side at the top, which framed the product
 * as a store rather than a connection.
 *
 * S3 — THIS IS A PROGRESSIVE TREE, NOT THE FINAL EIGHT SECTIONS.
 *
 * The Client Portal will end up with eight sections: Dashboard, My VPNs, VPN
 * Offers & Orders, Payments & Billing, Usage & History, VPN Guides, Support &
 * Chat, My Profile. Only the two that exist and are verified are here.
 *
 * A destination is added to this array in the SAME commit that lands the route
 * and its gate — never before. The reason is invariant 1 above: an entry whose
 * route does not exist becomes a dead link that the role guard turns into a
 * silent redirect, which is the "confusing navigation" this redesign exists to
 * remove. So the remaining six arrive as: S6 `/client/offers`, S7 `/client/
 * billing`, S8 `/client/usage`, S9 `/client/guides` — each when its own screen
 * and browser gate are green.
 *
 * Paths here are therefore:
 *   /client               Dashboard      (existing route)
 *   /client/vpns          My VPNs        (S3; NESTED because `/vpns` is the
 *                                         Super Admin console)
 *   /client/offers        Offers & Orders  (S6 shell)
 *   /buy                  Buy premium    (existing; the purchase flow)
 *   /client/billing       Payments & Billing (S7 shell)
 *   /history              Purchases      (existing; the detailed statement)
 *   /client/usage         Usage & History (S8 shell)
 *   /client/guides        VPN Guides     (S9 shell)
 *   /client/support       Support & Chat (S10 shell)
 *   /chat                 Chat           (existing; the working inbox)
 *   /client/notifications Notifications  (S11 shell)
 *   /profile              My profile     (existing)
 *
 * S6-S11 SHELLS ARE MARKED, NOT HIDDEN.
 *
 * The eight sections the Client Portal ends up with are now all reachable, but
 * six of them are OUTER LAYERS: the frame, the route and an honest empty state.
 * They are in the nav because a section the customer was promised and cannot
 * find is worse than one that says what it currently holds — and every shell
 * states plainly what it does and does not do yet. Adding a destination to this
 * tree is still only done in the same commit as its route and its guard, which
 * is why all six arrived together with their `App.tsx` entries.
 *
 * `/notifications` remains reachable from the topbar bell for every role; it is
 * listed here as well because the client brief names it as a section of its own.
 */
const CLIENT_NAV: AppNavGroup[] = [
  {
    label: 'My VPN',
    items: [
      { to: '/client', label: 'Dashboard', icon: LayoutDashboard },
      { to: '/client/vpns', label: 'My VPNs', icon: ShieldCheck },
      { to: '/client/usage', label: 'Usage & History', icon: Activity },
    ],
  },
  {
    label: 'Get more',
    items: [
      { to: '/client/offers', label: 'Offers & Orders', icon: Package },
      { to: '/buy', label: 'Buy premium', icon: ShoppingBag },
      /*
       * ONE billing destination, not two.
       *
       * `Payments & Billing` and `Purchases` sat side by side and read the same
       * payment records: `/history` drew the full statement, and `/client/billing`
       * spent its whole length telling the reader to go there. A customer had to
       * guess which of the two was the real one, and the one they picked first
       * was a dead end.
       *
       * `/client/billing` is now the single entry point: it shows what is still
       * waiting and links to the full statement. The `/history` ROUTE is kept -
       * bookmarks, e-mail links and the Dashboard's "Open Purchases" button all
       * still resolve - it is simply no longer a second navigation item.
       */
      { to: '/client/billing', label: 'Payments & Billing', icon: CreditCard },
    ],
  },
  {
    label: 'Help',
    items: [
      { to: '/client/guides', label: 'VPN Guides', icon: BookOpen },
      { to: '/client/support', label: 'Support & Chat', icon: MessageSquare },
    ],
  },
  {
    label: 'Account',
    items: [
      { to: '/client/notifications', label: 'Notifications', icon: Bell },
      { to: '/profile', label: 'Profile & Account', icon: User },
    ],
  },
]

/** The navigation for a role. Never merged — a role sees exactly one tree. */
export function navigationFor(role: UserRole): AppNavGroup[] {
  if (role === 'admin') return ADMIN_NAV
  if (role === 'client') return CLIENT_NAV
  return SUPER_ADMIN_NAV
}

/** Human label for the role scope, shown beside the avatar in the topbar. */
export const ROLE_LABEL: Record<UserRole, string> = {
  'super-admin': 'Super Admin',
  admin: 'Reseller',
  client: 'Client',
}
