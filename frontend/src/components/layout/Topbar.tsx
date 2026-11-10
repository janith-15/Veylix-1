import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Bell, Menu, Moon, Palette, Search, Sun } from 'lucide-react'
import { Dropdown, DropdownItem, DropdownSeparator } from '../ui/Dropdown'
import { useAuthStore } from '../../stores/authStore'
import { useNotifStore } from '../../stores/notifStore'
import { relativeTime } from '../../lib/time'
import { useThemeStore } from '../../theme/themeStore'
import { useNavigate } from 'react-router-dom'
import { Breadcrumbs } from './Breadcrumbs'
import { ROLE_LABEL } from './navModel'

export function Topbar({
  onMenuClick,
  userName,
  onTheme,
  onOpenPalette,
}: {
  onMenuClick: () => void
  userName: string
  onTheme: () => void
  notifications?: number
  onOpenPalette?: () => void
}) {
  const navigate = useNavigate()
  const { user, signOut } = useAuthStore()
  const displayName = user?.name ?? userName
  /** D-084: the role badge. Three roles share one shell and, before this, nothing
   *  told you which scope you were in — in an app where the reseller's "My
   *  clients" and the owner's "Clients" are different tables. */
  const role = user?.role
  const [scrolled, setScrolled] = useState(false)
  const [notifOpen, setNotifOpen] = useState(false)
  const notifRef = useRef<HTMLDivElement>(null)
  const { items: notifItems, markAllRead, markRead, load: loadNotifs } = useNotifStore()
  const unread = notifItems.filter((n) => !n.read).length

  /*
   * Light / dark switch.
   *
   * The app already ships six presets, but the only way to reach them was the
   * Theme Studio — so "does this support light mode?" was answered by opening a
   * design tool. This is the one-click answer: it flips between the `light` and
   * `dark` presets through `setPreset`, which preserves the user's chosen scene,
   * sub-theme, density and motion rather than resetting them.
   *
   * `light` is treated as the light half; every other preset (classic, modern,
   * gaming, hacking) counts as the dark half, so the button always has an
   * unambiguous opposite and its aria-pressed state is always truthful.
   */
  const preset = useThemeStore((s) => s.settings.preset)
  const setPreset = useThemeStore((s) => s.setPreset)
  const isLight = preset === 'light'
  const toggleTheme = () => setPreset(isLight ? 'dark' : 'light')

  // The badge is real: it reflects the account's own inbox, fetched once the
  // topbar mounts (i.e. once the session is established).
  useEffect(() => {
    void loadNotifs()
  }, [loadNotifs])

  useEffect(() => {
    if (!notifOpen) return
    const onDown = (e: MouseEvent) => {
      if (notifRef.current && !notifRef.current.contains(e.target as Node)) setNotifOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [notifOpen])

  /* Premium detail: the topbar picks up a border + shadow once the page scrolls. */
  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 4)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  return (
    <header className={`app-topbar glass ${scrolled ? 'app-topbar--scrolled' : ''}`}>
      <button className="app-topbar__icon-btn app-mobile-toggle" aria-label="Open menu" onClick={onMenuClick}>
        <Menu size={20} />
      </button>
      {/* D-084: context block. Before this the topbar carried no page title and
          no trail, so a user three clicks into /orders/:id had no orientation
          and no way back except the browser button. The trail is derived from
          the route (see Breadcrumbs.tsx) and renders nothing at depth 1. */}
      <div className="app-topbar__context">
        <Breadcrumbs />
      </div>
      <div className="app-search">
        <button
          type="button"
          className="app-search__btn"
          onClick={onOpenPalette}
          aria-label="Search pages, themes and actions (Ctrl K)"
        >
          <Search size={16} />
          <span className="app-search__placeholder">Search pages, actions…</span>
          <kbd className="app-search__kbd" aria-hidden="true">Ctrl K</kbd>
        </button>
      </div>
      <div className="app-topbar__spacer" />
      {role && (
        <span className="app-topbar__role" title={`Signed in as ${ROLE_LABEL[role]}`}>
          {ROLE_LABEL[role]}
        </span>
      )}
      <button
        type="button"
        className="app-topbar__icon-btn"
        aria-label={isLight ? 'Switch to dark appearance' : 'Switch to light appearance'}
        aria-pressed={isLight}
        title={isLight ? 'Switch to dark appearance' : 'Switch to light appearance'}
        onClick={toggleTheme}
      >
        {isLight ? <Moon size={18} /> : <Sun size={18} />}
      </button>
      <button className="app-topbar__icon-btn" aria-label="Open design studio" onClick={onTheme}>
        <Palette size={18} />
      </button>
      <div className="app-topbar__notif" ref={notifRef}>
        <button
          className="app-topbar__icon-btn"
          aria-label={unread > 0 ? `Notifications (${unread} unread)` : 'Notifications'}
          aria-expanded={notifOpen}
          onClick={() => setNotifOpen((o) => !o)}
        >
          <Bell size={18} />
          {unread > 0 && <span className="app-topbar__badge">{unread > 9 ? '9+' : unread}</span>}
        </button>
        {notifOpen &&
          createPortal(
            <div
            className="notif-overlay"
            onMouseDown={(e) => {
              if (e.target === e.currentTarget) setNotifOpen(false)
            }}
          >
            <div className="notif-modal" role="dialog" aria-modal="true" aria-label="Notifications">
              <div className="notif-modal__head">
                <strong>Notifications</strong>
                <button type="button" className="notif-modal__mark" onClick={markAllRead}>
                  Mark all read
                </button>
              </div>
              <div className="notif-modal__list">
                {notifItems.length === 0 ? (
                  <div className="notif-modal__empty">You're all caught up 🎉</div>
                ) : (
                  notifItems.map((n) => (
                    <button
                      key={n.id}
                      type="button"
                      className={`notif-modal__item ${n.read ? '' : 'notif-modal__item--unread'}`}
                      onClick={() => markRead(n.id)}
                    >
                      <span className={`notif-modal__dot notif-modal__dot--${n.kind}`} aria-hidden="true" />
                      <span className="notif-modal__txt">
                        <strong>{n.title}</strong>
                        {n.body && <span>{n.body}</span>}
                        <time dateTime={new Date(n.at * 1000).toISOString()}>{relativeTime(n.at)}</time>
                      </span>
                      {!n.read && <span className="notif-modal__unread" aria-label="Unread" />}
                    </button>
                  ))
                )}
              </div>
              <button
                type="button"
                className="notif-modal__view-all"
                onClick={() => {
                  setNotifOpen(false)
                  navigate('/notifications')
                }}
              >
                View all notifications
              </button>
            </div>
          </div>,
            document.body,
          )}
      </div>
      <Dropdown
        ariaLabel="Account menu"
        trigger={
          <span className="app-topbar__user">
            <span className="app-avatar" aria-hidden="true">
              {displayName.slice(0, 1).toUpperCase()}
            </span>
            <span className="app-topbar__user-name">{displayName}</span>
          </span>
        }
      >
        <DropdownItem onClick={() => navigate('/profile')}>Profile</DropdownItem>
        <DropdownItem onClick={onTheme}>Design studio</DropdownItem>
        <DropdownSeparator />
        <DropdownItem danger onClick={() => { void signOut(); navigate('/login') }}>Sign out</DropdownItem>
      </Dropdown>
    </header>
  )
}
