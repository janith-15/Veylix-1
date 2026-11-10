import { useEffect, useState } from 'react'

/**
 * Count-up animation for dashboard numbers (rAF).
 * The animation is driven from inside requestAnimationFrame — never with a
 * synchronous setState in the effect body — and fully respects
 * prefers-reduced-motion (jumps straight to the final value on the first
 * animation frame).
 */
export function useCountUp(target: number, duration = 800, enabled = true): number {
  const [value, setValue] = useState(0)
  /* Guard: NaN/Infinity targets would render "NaN" in the UI. */
  const safeTarget = Number.isFinite(target) ? target : 0

  useEffect(() => {
    if (!enabled) return
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    let raf = 0
    const t0 = performance.now()
    const tick = (t: number) => {
      if (reduce) {
        setValue(safeTarget)
        return
      }
      const p = Math.min(1, (t - t0) / duration)
      setValue(Math.round(safeTarget * (1 - Math.pow(1 - p, 3))))
      if (p < 1) raf = requestAnimationFrame(tick)
    }
    raf = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf)
  }, [safeTarget, duration, enabled])

  return enabled ? value : safeTarget
}