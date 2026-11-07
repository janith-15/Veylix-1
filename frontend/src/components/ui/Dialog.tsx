import { useEffect, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'

interface DialogProps {
  open: boolean
  onClose: () => void
  title: string
  children: ReactNode
  footer?: ReactNode
  wide?: boolean
}

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])'

export function Dialog({ open, onClose, title, children, footer, wide = false }: DialogProps) {
  const panelRef = useRef<HTMLDivElement>(null)
  const lastFocused = useRef<HTMLElement | null>(null)

  /* Focus management per WAI-ARIA: move focus in, trap Tab, restore on close. */
  useEffect(() => {
    if (!open) return
    lastFocused.current = document.activeElement as HTMLElement | null
    const raf = requestAnimationFrame(() => {
      const first = panelRef.current?.querySelector<HTMLElement>('[data-autofocus]') ??
        panelRef.current?.querySelector<HTMLElement>(FOCUSABLE)
      first?.focus()
    })
    return () => {
      cancelAnimationFrame(raf)
      lastFocused.current?.focus?.()
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
      if (e.key !== 'Tab' || !panelRef.current) return
      const nodes = Array.from(panelRef.current.querySelectorAll<HTMLElement>(FOCUSABLE))
      if (nodes.length === 0) return
      const first = nodes[0]
      const last = nodes[nodes.length - 1]
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault()
        first.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  useEffect(() => {
    if (!open) return
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [open])

  if (!open) return null

  return createPortal(
    <div className="ui-dialog-overlay" onClick={onClose}>
      <div
        ref={panelRef}
        className="ui-dialog glass"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        style={wide ? { width: 'min(820px, 100%)' } : undefined}
        onClick={(e) => e.stopPropagation()}
      >
        <header className="ui-dialog__header">
          <h2 className="ui-dialog__title">{title}</h2>
          <button className="ui-dialog__close" aria-label="Close dialog" onClick={onClose}>
            <X size={18} />
          </button>
        </header>
        <div className="ui-col">{children}</div>
        {footer && <footer className="ui-dialog__footer">{footer}</footer>}
      </div>
    </div>,
    document.body,
  )
}
