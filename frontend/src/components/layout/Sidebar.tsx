import { useEffect, useMemo, useState, type ComponentType, type ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { ChevronDown, PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import { VeylixLogo } from '../brand'
import { useViewTransitionNavigate } from '../../hooks/useViewTransitionNavigate'
import type { UserRole } from '../../stores/authStore'

/** DOM-safe id fragment for a group label ("People & Access" → "people-access"). */
const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '')

export interface AppNavItem {
  to: string
  label: string
  icon: ComponentType<{ size?: number }>
  /** Optional counter chip (e.g. Servers 30). Values > 99 render as 99+. */
  count?: number
  /** Role filter — omit to show for every role. */
  roles?: UserRole[]
}
export interface AppNavGroup {
  label: string
  items: AppNavItem[]
}

export function Sidebar({
  groups,
  footer,
  open,
  rail = false,
  onToggleRail,
  onNavigate,
  brandLabel = 'VEYLIX',
}: {
  groups: AppNavGroup[]
  footer?: ReactNode
  open: boolean
  /** Collapsed icon-rail mode (desktop). */
  rail?: boolean
  onToggleRail?: () => void
  /** Called after a nav item is followed — the shell uses it to close the drawer. */
  onNavigate?: () => void
  brandLabel?: string
}) {
  const location = useLocation()
  const transitionNavigate = useViewTransitionNavigate()

  /* Collapsed groups, persisted so a user's folding survives a reload. Keyed by
     group label: the label is stable across role trees even though the items
     are not. */
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(() => {
    try {
      const raw = localStorage.getItem('veylix.nav.collapsed')
      return raw ? (JSON.parse(raw) as Record<string, boolean>) : {}
    } catch {
      return {}
    }
  })
  useEffect(() => {
    try {
      localStorage.setItem('veylix.nav.collapsed', JSON.stringify(collapsed))
    } catch {
      /* Storage disabled (private mode): folding still works for this session. */
    }
  }, [collapsed])

  /* A collapsed group must never hide the page you are currently on. If the
     active route lives inside a folded group, force it open — otherwise the
     sidebar would show no active item at all, which reads as a bug. */
  const activeGroupLabel = useMemo(() => {
    for (const g of groups) {
      if (g.items.some((i) => i.to === location.pathname)) return g.label
    }
    return null
  }, [groups, location.pathname])
  useEffect(() => {
    if (activeGroupLabel && collapsed[activeGroupLabel] === true) {
      setCollapsed((c) => ({ ...c, [activeGroupLabel]: false }))
    }
  }, [activeGroupLabel, collapsed])

  /* The drawer state is a MOBILE concept. On desktop the sidebar is always
     visible, so the closed class must not be applied at all (avoids relying on
     a cascade override). */
  const [isDesktop, setIsDesktop] = useState(
    () => typeof window !== 'undefined' && window.matchMedia('(min-width: 1024px)').matches,
  )
  useEffect(() => {
    const mq = window.matchMedia('(min-width: 1024px)')
    const onChange = (e: MediaQueryListEvent) => setIsDesktop(e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const drawerClosed = !isDesktop && !open

  return (
    <aside
      className={`app-sidebar glass ${rail ? 'app-sidebar--rail' : ''} ${drawerClosed ? 'app-sidebar--closed' : ''}`}
      aria-label="Primary navigation"
      data-drawer-closed={drawerClosed ? 'true' : 'false'}
      data-is-desktop={isDesktop ? 'true' : 'false'}
    >
      <div className="app-sidebar__brand">
        <span className="app-sidebar__brand-logo">
          <VeylixLogo size={30} />
        </span>
        <span className="app-sidebar__brand-label">{brandLabel}</span>
        {onToggleRail && (
          <button
            type="button"
            className="app-sidebar__toggle"
            aria-label={rail ? 'Expand sidebar' : 'Collapse sidebar'}
            title={rail ? 'Expand sidebar' : 'Collapse sidebar'}
            onClick={onToggleRail}
          >
            {rail ? <PanelLeftOpen size={15} /> : <PanelLeftClose size={15} />}
          </button>
        )}
      </div>
      <nav className="app-nav">
        {groups.map((g) => {
          /* D-084: a group only becomes collapsible when it is long enough to
             be worth folding. Every role tree has at least one group that
             qualifies; a client's short groups stay permanently open so the
             client portal never hides a destination behind a disclosure they
             did not ask for. */
          const collapsible = g.items.length > 4 && !rail
          const open = collapsed[g.label] !== true
          return (
            <div className="app-nav__group" key={g.label}>
              {collapsible ? (
                <button
                  type="button"
                  className="app-nav__group-toggle"
                  aria-expanded={open}
                  aria-controls={`nav-group-${slug(g.label)}`}
                  onClick={() => setCollapsed((c) => ({ ...c, [g.label]: open }))}
                >
                  <ChevronDown size={12} aria-hidden="true" />
                  {g.label}
                </button>
              ) : (
                <div className="app-nav__group-label">{g.label}</div>
              )}
              <div id={`nav-group-${slug(g.label)}`} hidden={!open}>
                {g.items.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    title={item.label}
                    onClick={(e) => {
                      /* Same-route clicks stay native; others get a view transition. */
                      if (location.pathname !== item.to) {
                        e.preventDefault()
                        transitionNavigate(item.to)
                      }
                      /* On mobile the drawer must close on navigation, or the
                         user is left on the page with the menu covering it. */
                      if (onNavigate) onNavigate()
                    }}
                    className={({ isActive }) =>
                      `app-nav__item${isActive ? ' app-nav__item--active' : ''}`
                    }
                  >
                    <item.icon size={18} />
                    <span className="app-nav__label">{item.label}</span>
                    {typeof item.count === 'number' && (
                      <span className="app-nav__count">{item.count > 99 ? '99+' : item.count}</span>
                    )}
                  </NavLink>
                ))}
              </div>
            </div>
          )
        })}
      </nav>
      {footer && <div className="app-sidebar__footer">{footer}</div>}
    </aside>
  )
}
