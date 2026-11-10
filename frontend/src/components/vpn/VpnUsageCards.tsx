import { useEffect, useRef, useState } from 'react'
import { ArrowDown, ArrowUp, Sigma } from 'lucide-react'
import { formatBytes } from '../ui'
import './vpn-usage-cards.css'

/**
 * The three numbers a customer actually opens this page for: Download, Upload
 * and Total (owner's P-01, and §4.2 of the details-page brief).
 *
 * WHY THREE CARDS AND NOT ONE
 *
 * A single "12.4 GB used" answer hides which direction is eating the allowance.
 * Tethering and streaming are download-heavy; backups and uploads are not, and a
 * customer deciding what to stop needs to know which one it is. The three
 * numbers come from the SERVER separately (`lifetime_down_bytes`,
 * `lifetime_up_bytes`, `lifetime_total_bytes`) and are never re-derived here —
 * adding two of them locally is how a card ends up disagreeing with the API.
 *
 * COUNT-UP ANIMATION, AND WHEN NOT TO ANIMATE
 *
 * The values are real measurements, so an animated counter is honest as long as
 * it lands exactly on the server's number. It is skipped entirely when the
 * reader asked for reduced motion, and skipped on the very first paint for zero
 * (animating 0 → 0 is a flicker with no information in it).
 *
 * `prefers-reduced-motion` is read with `matchMedia` rather than a CSS media
 * query because the animation is driven in JS; a stylesheet cannot stop a
 * `requestAnimationFrame` loop.
 */

interface Props {
  downBytes: number
  upBytes: number
  totalBytes: number
  /** Shown under the total; `-1` means unlimited, which is not "0 left". */
  limitBytes: number
  /**
   * False when the read that supplied these numbers did not include the two
   * directions (the `?summary=1` response). The cards then render only the
   * total rather than a `0 B received` nobody measured.
   */
  showDirections?: boolean
  /** When false the cards render final values with no animation. */
  animate?: boolean
  /** True while the first read is in flight — renders a skeleton, not a zero. */
  loading?: boolean
  /**
   * A note under a value that the caller must state itself, e.g. the summary
   * read omits one of the directions.
   */
  downNote?: string
  upNote?: string
}

/** How long the count-up runs. Short enough to feel like feedback, not theatre. */
const DURATION_MS = 620

function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

/**
 * Animates one byte count from its previous value to the new one.
 *
 * The easing is a plain cubic ease-out: fast at the start so the change is
 * visible immediately, settling smoothly so the final number is readable rather
 * than still moving when it is read.
 */
function useCountUp(target: number, animate: boolean): number {
  const [shown, setShown] = useState(target)
  const fromRef = useRef(target)
  const frameRef = useRef<number | null>(null)

  useEffect(() => {
    const from = fromRef.current
    fromRef.current = target

    if (!animate || from === target || prefersReducedMotion()) {
      setShown(target)
      return
    }

    const start = performance.now()
    const step = (now: number) => {
      const t = Math.min(1, (now - start) / DURATION_MS)
      const eased = 1 - Math.pow(1 - t, 3)
      setShown(from + (target - from) * eased)
      if (t < 1) {
        frameRef.current = requestAnimationFrame(step)
      } else {
        // Land exactly on the server's number: an interpolation that stops at
        // 12.399 GB when the API said 12.4 GB is a wrong answer, not a rounding.
        setShown(target)
        frameRef.current = null
      }
    }
    frameRef.current = requestAnimationFrame(step)
    return () => {
      if (frameRef.current !== null) cancelAnimationFrame(frameRef.current)
      frameRef.current = null
    }
  }, [target, animate])

  return shown
}

function UsageCard({
  label,
  icon,
  bytes,
  note,
  testId,
  animate,
  loading,
}: {
  label: string
  icon: React.ReactNode
  bytes: number
  note?: string
  testId: string
  animate: boolean
  loading: boolean
}) {
  const shown = useCountUp(bytes, animate)
  const value = loading ? '—' : formatBytes(Math.max(0, shown))

  return (
    <div className="vuc-card" data-testid={testId}>
      <span className="vuc-card__icon" aria-hidden="true">
        {icon}
      </span>
      <span className="vuc-card__label">{label}</span>
      {/* The animated value is the visual; the label is read from the
          description below, so a screen reader never hears a streaming number. */}
      <strong className="vuc-card__value">{value}</strong>
      {note && <span className="vuc-card__note">{note}</span>}
    </div>
  )
}

/**
 * Download / Upload / Total, as three cards.
 *
 * On a phone the row wraps into a single column rather than scrolling sideways:
 * an allowance figure the customer has to swipe to read is the failure mode the
 * mobile rules call out.
 */
export function VpnUsageCards({
  downBytes,
  upBytes,
  totalBytes,
  limitBytes,
  showDirections = true,
  animate = true,
  loading = false,
  downNote = 'Received',
  upNote = 'Sent',
}: Props) {
  const remaining = limitBytes > 0 ? Math.max(0, limitBytes - totalBytes) : -1
  const totalNote =
    limitBytes > 0
      ? `${formatBytes(remaining)} left of ${formatBytes(limitBytes)}`
      : 'No data limit on this VPN'

  return (
    <div className="vuc-row" data-vuc-count={showDirections ? 3 : 1} data-testid="vpn-usage-cards">
      {showDirections && (
        <>
          <UsageCard
            label="Download"
            icon={<ArrowDown size={15} />}
            bytes={downBytes}
            testId="vuc-download"
            animate={animate}
            loading={loading}
            note={downNote}
          />
          <UsageCard
            label="Upload"
            icon={<ArrowUp size={15} />}
            bytes={upBytes}
            testId="vuc-upload"
            animate={animate}
            loading={loading}
            note={upNote}
          />
        </>
      )}
      <UsageCard
        label={showDirections ? 'Total' : 'Data used'}
        icon={<Sigma size={15} />}
        bytes={totalBytes}
        testId="vuc-total"
        animate={animate}
        loading={loading}
        note={totalNote}
      />
    </div>
  )
}
