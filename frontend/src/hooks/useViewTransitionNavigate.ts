import { flushSync } from 'react-dom'
import { useNavigate } from 'react-router-dom'

type NavOptions = { replace?: boolean }

/**
 * SPA navigation wrapped in the View Transitions API.
 * Falls back to instant navigation when the API is unavailable or the user
 * prefers reduced motion. React state updates are flushed synchronously so
 * the startViewTransition snapshot captures the *new* view.
 */
export function useViewTransitionNavigate() {
  const navigate = useNavigate()

  return (to: string, options?: NavOptions) => {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      navigate(to, options)
      return
    }
    const doc = document as Document & {
      startViewTransition?: (updateCallback: () => void) => unknown
    }
    if (typeof doc.startViewTransition === 'function') {
      doc.startViewTransition(() => {
        flushSync(() => navigate(to, options))
      })
      return
    }
    navigate(to, options)
  }
}