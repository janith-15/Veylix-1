import { useEffect, useMemo, useRef, useState, type ComponentType, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import {
  Bell, CreditCard, Globe, LayoutDashboard, Package, Palette, ScrollText, Search, Server, UserCog, Users,
} from 'lucide-react'
import { api } from '../../lib/api'
import { useThemeStore } from '../../theme/themeStore'
import { useViewTransitionNavigate } from '../../hooks/useViewTransitionNavigate'
import { PRESET_COLORS, PRESET_LABELS, type ThemePreset } from '../../theme/themes'
import './palette.css'

interface PaletteItem {
  id: string
  group: string
  label: string
  hint?: string
  keywords?: string
  icon?: ComponentType<{ size?: number }>
  swatch?: string
  run: () => void
}

const NAV_PAGES: { to: string; label: string; icon: ComponentType<{ size?: number }> }[] = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/servers', label: 'Servers', icon: Server },
  { to: '/offers', label: 'VPN Offers', icon: Package },
  { to: '/staff', label: 'Staff & Admins', icon: UserCog },
  { to: '/clients', label: 'Clients', icon: Users },
  { to: '/payments', label: 'Payments', icon: CreditCard },
  { to: '/audit', label: 'Audit log', icon: ScrollText },
  { to: '/notifications', label: 'Notifications', icon: Bell },
]

const PRESETS: ThemePreset[] = ['modern', 'dark', 'light', 'classic', 'gaming', 'hacking']

/**
 * ⌘K command palette — in-house, zero dependencies (cmdk-pattern):
 * filter-as-you-type, grouped results, full keyboard control.
 */
export function CommandPalette({
  open,
  onClose,
  onOpenStudio,
}: {
  open: boolean
  onClose: () => void
  onOpenStudio: () => void
}) {
  const transitionNavigate = useViewTransitionNavigate()
  const { draftSetPreset } = useThemeStore()
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  const items = useMemo<PaletteItem[]>(() => {
    const navigateItem = (to: string, label: string, icon: ComponentType<{ size?: number }>): PaletteItem => ({
      id: `nav-${to}`,
      group: 'Navigation',
      label,
      keywords: 'go open page',
      icon,
      run: () => transitionNavigate(to),
    })
    return [
      ...NAV_PAGES.map((p) => navigateItem(p.to, p.label, p.icon)),
      {
        id: 'act-studio',
        group: 'Actions',
        label: 'Open Design Studio',
        hint: 'appearance',
        keywords: 'customize design colors',
        icon: Palette,
        run: onOpenStudio,
      },
      ...PRESETS.map<PaletteItem>((p) => ({
        id: `theme-${p}`,
        group: 'Switch theme',
        label: PRESET_LABELS[p],
        hint: 'preset',
        keywords: 'theme color mode',
        swatch: PRESET_COLORS[p].primary,
        run: () => draftSetPreset(p),
      })),
    ]
  }, [transitionNavigate, draftSetPreset, onOpenStudio])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return items
    return items.filter((i) => `${i.label} ${i.hint ?? ''} ${i.keywords ?? ''}`.toLowerCase().includes(q))
  }, [items, query])

  /*
   * Entity search (Phase-3 UX, 3.2).
   *
   * Navigation alone meant that "find janith's VPN" was: close the palette, open
   * /vpns, type the e-mail, wait. The palette now resolves a typed term against
   * the console's own endpoints and offers the actual rows.
   *
   * Debounced 200ms and gated on a 2-character minimum, because every keystroke
   * would otherwise be two panel-backed queries — on a 1 vCPU host that is the
   * difference between a search and a stall.
   */
  const [entities, setEntities] = useState<PaletteItem[]>([])
  const [searching, setSearching] = useState('')
  const term = query.trim()
  /* One character is not a query. Derived during render (never written from the
   * effect) so a cleared box needs no synchronous setState — the lint rule that
   * guards against cascading renders is right: this is a value, not a sync. */
  const searchingTerm = term.length >= 2

  useEffect(() => {
    if (!open || !searchingTerm) return
    let cancelled = false
    const timer = setTimeout(() => {
      setSearching(term)
      void (async () => {
        const found: PaletteItem[] = []
        // Both reads are independent and either may be refused for a role that
        // may not see the other, so each failure only removes its own results.
        const [vpnRes, clientRes] = await Promise.allSettled([
          api.adminVpns({ q: term, limit: 6, offset: 0 }),
          api.adminClients({ q: term, limit: 6 }),
        ])
        if (vpnRes.status === 'fulfilled') {
          for (const v of vpnRes.value.vpns ?? []) {
            found.push({
              id: `vpn-${v.server_id}-${v.panel_email}`,
              group: 'VPNs',
              label: v.panel_email,
              hint: [v.owner_email || 'unassigned', v.tag ?? 'untagged', `${v.usage_pct}%`]
                .filter(Boolean)
                .join(' · '),
              icon: Globe,
              run: () =>
                transitionNavigate(`/vpns/${v.server_id}/${encodeURIComponent(v.panel_email)}`),
            })
          }
        }
        if (clientRes.status === 'fulfilled') {
          for (const c of clientRes.value.clients ?? []) {
            found.push({
              id: `client-${c.id}`,
              group: 'Clients',
              label: c.display_name || c.username || c.email,
              hint: c.email,
              icon: Users,
              run: () => transitionNavigate(`/clients/${c.id}`),
            })
          }
        }
        if (!cancelled) {
          setEntities(found)
          setSearching('')
        }
      })()
    }, 200)
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [open, term, searchingTerm, transitionNavigate])

  /*
   * Entities lead the list: when somebody types "janith" they mean a person or a
   * VPN, not a page whose description happens to contain it.
   *
   * `entities` is cleared implicitly: results that no longer match the typed term
   * belong to a previous search, so they are filtered out at render rather than
   * blanked by a state write.
   */
  const relevantEntities = useMemo(
    () => (searchingTerm ? entities : []),
    [entities, searchingTerm],
  )
  const combined = useMemo(
    () => [...relevantEntities, ...filtered],
    [relevantEntities, filtered],
  )

  const safeActive = Math.min(active, Math.max(0, combined.length - 1))

  useEffect(() => {
    if (!open) return
    /* Query/selection reset happens via remount (see `key` in App.tsx),
       so this effect only handles focus + scroll lock — no setState here. */
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const raf = requestAnimationFrame(() => inputRef.current?.focus())
    return () => {
      cancelAnimationFrame(raf)
      document.body.style.overflow = prevOverflow
    }
  }, [open])

  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>('[data-active="true"]')?.scrollIntoView({ block: 'nearest' })
  }, [safeActive, query])

  const runIndex = (i: number) => {
    const item = combined[i]
    if (!item) return
    onClose()
    item.run()
  }

  const onKeyDown = (e: ReactKeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((a) => Math.min(combined.length - 1, a + 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((a) => Math.max(0, a - 1))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      runIndex(safeActive)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }

  if (!open) return null

  const rows: ReactNode[] = []
  let prevGroup = ''
  combined.forEach((item, i) => {
    if (item.group !== prevGroup) {
      prevGroup = item.group
      rows.push(
        <div className="palette__group" key={`group-${item.group}`}>
          {item.group}
        </div>,
      )
    }
    const Icon = item.icon
    rows.push(
      <button
        key={item.id}
        id={item.id}
        type="button"
        role="option"
        aria-selected={i === safeActive}
        data-active={i === safeActive}
        className="palette__item"
        onMouseEnter={() => setActive(i)}
        onClick={() => runIndex(i)}
      >
        {item.swatch ? (
          <span className="palette__dot" style={{ background: item.swatch }} aria-hidden="true" />
        ) : Icon ? (
          <span className="palette__icon" aria-hidden="true">
            <Icon size={15} />
          </span>
        ) : null}
        <span className="palette__label">{item.label}</span>
        {item.hint && <span className="palette__hint">{item.hint}</span>}
      </button>,
    )
  })

  return createPortal(
    <div
      className="palette-overlay"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
      onKeyDown={onKeyDown}
    >
      <div className="palette glass" role="dialog" aria-modal="true" aria-label="Command palette">
        <div className="palette__input-row">
          <Search size={16} aria-hidden="true" />
          <input
            ref={inputRef}
            className="palette__input"
            placeholder="Search pages, themes, actions…"
            value={query}
            role="combobox"
            aria-expanded="true"
            aria-controls="palette-list"
            aria-activedescendant={combined[safeActive]?.id}
            onChange={(e) => setQuery(e.target.value)}
          />
          <kbd className="palette__kbd" aria-hidden="true">esc</kbd>
        </div>
        <div className="palette__list" id="palette-list" role="listbox" ref={listRef}>
          {combined.length === 0 ? (
            <div className="palette__empty">
              {searching ? `Searching for “${query}”…` : `No matches for “${query}”`}
            </div>
          ) : (
            rows
          )}
        </div>
        <div className="palette__footer" aria-hidden="true">
          <span><kbd>↑</kbd><kbd>↓</kbd> navigate</span>
          <span><kbd>↵</kbd> select</span>
          <span><kbd>esc</kbd> close</span>
        </div>
      </div>
    </div>,
    document.body,
  )
}
