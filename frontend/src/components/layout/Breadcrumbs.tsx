import { Link, useLocation } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'
import { useAuthStore } from '../../stores/authStore'
import { navigationFor } from './navModel'

/**
 * Breadcrumbs (D-084).
 *
 * The audit found ZERO breadcrumb code anywhere in the frontend (a grep for
 * `breadcrumb` across every `.ts`/`.tsx` returned nothing). The consequence was
 * real: on `/vpns/:serverId/:panelEmail`, `/orders/:id`, `/servers/:id`,
 * `/clients/:id`, `/vpn/:id` and `/chat/:threadId` the only orientation was a
 * `PageHeader` string, and the route param itself was never shown. A user three
 * clicks deep had no way back to the list except the browser button.
 *
 * Design rules:
 *  - The trail is DERIVED from the route, never passed in by each page, so a
 *    page cannot forget to provide it and cannot disagree with the sidebar.
 *  - The first crumb is the role's own home, labelled from the SAME
 *    `navigationFor()` the sidebar uses. One source, no drift.
 *  - Detail routes show their param, so the crumb states which record you are
 *    on rather than just which screen.
 *  - Renders nothing at depth 1: a crumb trail that says only "Overview" is
 *    decoration, and a topbar is scarce space.
 */

interface TrailEntry {
  /** Label for this level. */
  label: string
  /** Destination. Omitted for the current (last) level. */
  to?: string
}

/**
 * One row per route that benefits from a trail.
 *
 * `:param` segments use the same names as the `Route` declarations in App.tsx.
 * A route absent from this table simply gets the role home + its nav label,
 * which is still correct — the table only adds depth where depth exists.
 *
 * Labels are deliberately STATIC rather than derived from the route param.
 * Showing the raw record id in a crumb ("/orders/9f3a…") is noise, and showing
 * a human name would require the trail to fetch, which would put a network
 * dependency inside the shell chrome. The page itself names the record.
 */
const TRAILS: { pattern: RegExp; trail: TrailEntry[] }[] = [
  {
    pattern: /^\/vpns\/[^/]+\/[^/]+\/?$/,
    trail: [{ label: 'All VPNs', to: '/vpns' }, { label: 'VPN' }],
  },
  {
    pattern: /^\/servers\/[^/]+\/?$/,
    trail: [{ label: 'Servers', to: '/servers' }, { label: 'Server' }],
  },
  {
    pattern: /^\/clients\/[^/]+\/?$/,
    trail: [{ label: 'Clients', to: '/clients' }, { label: 'Client' }],
  },
  {
    pattern: /^\/orders\/[^/]+\/?$/,
    trail: [{ label: 'Orders', to: '/orders' }, { label: 'Order' }],
  },
  {
    pattern: /^\/vpn\/[^/]+\/?$/,
    // S3: the client VPN detail's parent is now the My VPNs screen, which moved
    // to `/client/vpns` when the dashboard was split. `/client` is the
    // dashboard, so linking "My VPNs" to it would name a screen that is not
    // that screen — the crumb must point at the list it came from.
    trail: [{ label: 'My VPNs', to: '/client/vpns' }, { label: 'VPN' }],
  },
  {
    pattern: /^\/chat\/[^/]+\/?$/,
    trail: [{ label: 'Chat', to: '/chat' }, { label: 'Conversation' }],
  },
]

/** Static one-level screens: the crumb is the screen's own nav label. */
function crumbForPath(path: string, role: Parameters<typeof navigationFor>[0]): string | null {
  const groups = navigationFor(role)
  for (const g of groups) {
    for (const item of g.items) {
      if (item.to === path) return item.label
    }
  }
  return null
}

export function Breadcrumbs() {
  const { pathname } = useLocation()
  const role = useAuthStore((s) => s.user?.role ?? 'client')

  /* Role home: the first item of the role's first group. Derived, not hardcoded,
     so it can never point somewhere the sidebar does not offer. */
  const groups = navigationFor(role)
  const home = groups[0]?.items[0]
  const current = crumbForPath(pathname, role)

  const dynamic = TRAILS.find((t) => t.pattern.test(pathname))
  const atRoot = !!home && pathname === home.to

  /* Depth 1 → nothing. The topbar title already says where you are. */
  if (atRoot) return null
  if (!home) return null
  /* Unknown path with no trail and no nav label: stay silent rather than
     render a trail that mislabels the page. */
  if (!dynamic && !current) return null

  const items: { label: string; to?: string }[] = [{ label: home.label, to: home.to }]

  if (dynamic) {
    items.push(...dynamic.trail)
  } else if (current && current !== home.label) {
    items.push({ label: current })
  }

  return (
    <nav className="app-crumbs" aria-label="Breadcrumb">
      <ol className="app-crumbs__list">
        {items.map((item, i) => {
          const last = i === items.length - 1
          return (
            <li key={`${item.label}-${i}`} className="app-crumbs__item">
              {i > 0 && (
                <ChevronRight size={13} className="app-crumbs__sep" aria-hidden="true" />
              )}
              {last || !item.to ? (
                <span className="app-crumbs__current" aria-current="page">
                  {item.label}
                </span>
              ) : (
                <Link to={item.to} className="app-crumbs__link">
                  {item.label}
                </Link>
              )}
            </li>
          )
        })}
      </ol>
    </nav>
  )
}

/* `isDetailRoute` lived here as a second export, which trips the fast-refresh
 * rule (a module should export either components or helpers, not both). Nothing
 * consumed it yet, so it is dropped rather than relocated — the header comment
 * above is the contract, and a future caller can read `TRAILS` directly. */
