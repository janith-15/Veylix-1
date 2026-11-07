import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

interface DropdownProps {
  trigger: ReactNode
  children: ReactNode
  ariaLabel?: string
}

interface MenuPos {
  top: number
  right: number
  minWidth: number
}

/**
 * Account/action menu.
 *
 * The menu is PORTALED to <body> and positioned with `position: fixed`.
 * Why: the trigger lives inside `.app-topbar` which is a `.glass` element
 * (`isolation: isolate` → its own stacking context). An absolutely
 * positioned menu would be trapped inside that context and would render
 * *under* the page content. Portalling + fixed positioning escapes every
 * ancestor stacking context, so the menu is always on top.
 */
export function Dropdown({ trigger, children, ariaLabel }: DropdownProps) {
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<MenuPos>({ top: 0, right: 0, minWidth: 180 })
  const wrapRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  const place = useCallback(() => {
    const el = triggerRef.current
    if (!el) return
    const r = el.getBoundingClientRect()
    setPos({
      top: Math.round(r.bottom + 8),
      right: Math.round(window.innerWidth - r.right),
      minWidth: Math.max(180, Math.round(r.width)),
    })
  }, [])

  useLayoutEffect(() => {
    if (open) place()
  }, [open, place])

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      const target = e.target as Node
      if (wrapRef.current?.contains(target) || menuRef.current?.contains(target)) return
      setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    const onScroll = () => place()
    document.addEventListener('mousedown', onDoc)
    window.addEventListener('keydown', onKey)
    window.addEventListener('resize', onScroll)
    window.addEventListener('scroll', onScroll, true)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('resize', onScroll)
      window.removeEventListener('scroll', onScroll, true)
    }
  }, [open, place])

  return (
    <>
      <div className="ui-dropdown" ref={wrapRef}>
        <button
          ref={triggerRef}
          type="button"
          aria-haspopup="menu"
          aria-expanded={open}
          aria-label={ariaLabel}
          onClick={() => setOpen((o) => !o)}
        >
          {trigger}
        </button>
      </div>
      {open &&
        createPortal(
          <div
            ref={menuRef}
            className="ui-dropdown__menu"
            role="menu"
            style={{ top: pos.top, right: pos.right, minWidth: pos.minWidth }}
          >
            {children}
          </div>,
          document.body,
        )}
    </>
  )
}

export function DropdownItem({
  children,
  onClick,
  danger = false,
}: {
  children: ReactNode
  onClick?: () => void
  danger?: boolean
}) {
  return (
    <button
      type="button"
      className={`ui-dropdown__item ${danger ? 'ui-dropdown__item--danger' : ''}`}
      onClick={onClick}
    >
      {children}
    </button>
  )
}

export function DropdownSeparator() {
  return <div className="ui-dropdown__separator" />
}
