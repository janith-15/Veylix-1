import { useEffect, useState } from 'react'
import { Outlet } from 'react-router-dom'
import { Sidebar, type AppNavGroup } from './Sidebar'
import { Topbar } from './Topbar'
import { useAuthStore } from '../../stores/authStore'
import './layout.css'

export function AppShell({
  groups,
  userName = 'Super Admin',
  onTheme,
  onOpenPalette,
}: {
  groups: AppNavGroup[]
  userName?: string
  onTheme?: () => void
  onOpenPalette?: () => void
}) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [rail, setRail] = useState(false)
  const user = useAuthStore((s) => s.user)

  /**
   * D-004 (approved): single shell + role-filtered navigation.
   * Server-side authorization remains the real guard (Phase 3+).
   */
  const role = user?.role ?? 'super-admin'
  const visibleGroups = groups
    .map((g) => ({ ...g, items: g.items.filter((i) => !i.roles || i.roles.includes(role)) }))
    .filter((g) => g.items.length > 0)

  /*
   * Debug helper. It only EXECUTES when a developer types
   * `window.__veylixLayout()` in DevTools — nothing runs on page load, so no
   * console output ships to a customer (U6, UI/UX defect board). Gate it on
   * `import.meta.env.DEV` so the function itself is not even installed in a
   * production bundle.
   */
  useEffect(() => {
    if (!import.meta.env.DEV) return
    type LayoutDebug = () => void
    const w = window as unknown as { __veylixLayout?: LayoutDebug }
    w.__veylixLayout = () => {
      const pick = (sel: string) => {
        const el = document.querySelector(sel) as HTMLElement | null
        if (!el) return { sel, missing: true }
        const r = el.getBoundingClientRect()
        const s = getComputedStyle(el)
        return {
          sel,
          x: Math.round(r.x),
          w: Math.round(r.width),
          cls: el.className,
          transform: s.transform,
          marginLeft: s.marginLeft,
          position: s.position,
        }
      }
      const root = getComputedStyle(document.documentElement)
      // eslint-disable-next-line no-console
      console.table([
        pick('.app-shell'),
        pick('.app-sidebar'),
        pick('.app-main'),
        pick('.app-content'),
      ])
      // eslint-disable-next-line no-console
      console.log('tokens', {
        sidebarWidth: root.getPropertyValue('--sidebar-width'),
        sidebarOffset: root.getPropertyValue('--sidebar-offset'),
        dataShell: document.documentElement.dataset.shell,
        viewport: window.innerWidth,
        dpr: window.devicePixelRatio,
      })
    }
    return () => {
      delete (window as unknown as { __veylixLayout?: LayoutDebug }).__veylixLayout
    }
  }, [])

  return (
    <div className={`app-shell ${rail ? 'app-shell--rail' : ''}`}>
      {menuOpen && (
        <div className="app-mobile-scrim app-mobile-scrim--visible" aria-hidden="true" onClick={() => setMenuOpen(false)} />
      )}
      {/* Account lives ONLY in the topbar — the sidebar has no user footer. */}
      <Sidebar
        groups={visibleGroups}
        open={menuOpen}
        rail={rail}
        onToggleRail={() => setRail((r) => !r)}
        /* On mobile the drawer is an overlay: following a link must dismiss it,
           or the destination renders underneath the menu. Desktop ignores this
           because the sidebar is not an overlay there. */
        onNavigate={() => setMenuOpen(false)}
      />
      <main className="app-main">
        <Topbar
          onMenuClick={() => setMenuOpen(true)}
          userName={user?.name ?? userName}
          onTheme={onTheme ?? (() => {})}
          onOpenPalette={onOpenPalette}
        />
        <div className="app-content">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
