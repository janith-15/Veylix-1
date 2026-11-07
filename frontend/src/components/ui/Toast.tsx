import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { CheckCircle2, Info, AlertTriangle, XCircle, X } from 'lucide-react'
import { useThemeStore } from '../../theme/themeStore'
import { DEFAULT_COMPONENTS } from '../../theme/themes'

type ToastKind = 'success' | 'info' | 'warning' | 'error'
interface ToastItem {
  id: number
  kind: ToastKind
  title: string
  message?: string
  /** Exit animation in progress — removed from the DOM right after. */
  leaving?: boolean
}

interface ToastContextValue {
  toast: (kind: ToastKind, title: string, message?: string) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

// oxlint-disable-next-line react/only-export-components -- hook co-located with its provider is the standard React context pattern; Fast Refresh trade-off accepted.
export function useToast() {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within ToastProvider')
  return ctx.toast
}

const ICONS: Record<ToastKind, ReactNode> = {
  success: <CheckCircle2 size={18} />,
  info: <Info size={18} />,
  warning: <AlertTriangle size={18} />,
  error: <XCircle size={18} />,
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])
  /*
   * Fall back to the shipped defaults rather than trusting the persisted blob.
   * `settings.components` comes from localStorage, and a stale or partially
   * migrated payload used to undefined-deref here and take the ENTIRE app down
   * to a blank screen — a toast timer is not worth losing the product over.
   */
  const stored = useThemeStore((s) => s.settings.components)
  const components = stored ?? DEFAULT_COMPONENTS
  const counter = useRef(0)
  const timers = useRef<Map<number, ReturnType<typeof setTimeout>>>(new Map())

  /* Play the exit animation first, then drop the node from the DOM. */
  const dismiss = useCallback((id: number) => {
    const timer = timers.current.get(id)
    if (timer) clearTimeout(timer)
    timers.current.delete(id)
    setItems((prev) => {
      const target = prev.find((t) => t.id === id)
      if (!target || target.leaving) return prev
      window.setTimeout(() => setItems((cur) => cur.filter((t) => t.id !== id)), 180)
      return prev.map((t) => (t.id === id ? { ...t, leaving: true } : t))
    })
  }, [])

  const toast = useCallback(
    (kind: ToastKind, title: string, message?: string) => {
      const id = ++counter.current
      setItems((prev) => [...prev, { id, kind, title, message }])
      const timer = setTimeout(() => dismiss(id), Math.max(1500, components.toastDuration * 1000))
      timers.current.set(id, timer)
    },
    [dismiss, components.toastDuration],
  )

  const value = useMemo(() => ({ toast }), [toast])

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div
        aria-live="polite"
        className="toast-viewport"
        role="status"
        data-testid="toast-viewport"
      >
        {items.map((t) => (
          <div
            key={t.id}
            className={`toast toast--${t.kind} ${t.leaving ? 'toast--leaving' : ''} glass`}
            data-kind={t.kind}
            data-toast-style={components.toastStyle}
          >
            <span className="toast-icon">{ICONS[t.kind]}</span>
            <div className="toast-body">
              <div className="toast-title">{t.title}</div>
              {t.message && <div className="toast-message">{t.message}</div>}
            </div>
            <button
              className="toast-close"
              aria-label="Dismiss notification"
              onClick={() => dismiss(t.id)}
            >
              <X size={15} />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}
