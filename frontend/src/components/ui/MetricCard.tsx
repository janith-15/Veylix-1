import type { ReactNode } from 'react'

interface MetricCardProps {
  label: string
  value: ReactNode
  delta?: string
  deltaDirection?: 'up' | 'down' | 'neutral'
  icon?: ReactNode
  /** Optional compact chart under the value (e.g. Sparkline). */
  chart?: ReactNode
  /**
   * `plain` (default) renders bare content — used inside 3D tiles that already
   * provide the surface. `card` wraps in a glass card for standalone use.
   */
  variant?: 'plain' | 'card'
  /**
   * When set, the whole tile becomes one button that opens the list behind the
   * number — a summary you cannot open is a dead end (a KPI card is the start
   * of a question, not the answer). Renders a real <button>, so keyboard
   * activation and the focus ring come for free; the resting look is unchanged
   * (`ui-metric--link` adds only the hover/focus affordance).
   */
  onClick?: () => void
  className?: string
}

export function MetricCard({
  label,
  value,
  delta,
  deltaDirection = 'neutral',
  icon,
  chart,
  variant = 'plain',
  onClick,
  className = '',
}: MetricCardProps) {
  const body = (
    <>
      <div className="ui-row--spread">
        <span className="ui-metric__label">{label}</span>
        {icon}
      </div>
      <div className="ui-metric__value">{value}</div>
      {chart && <div className="ui-metric__chart">{chart}</div>}
      {delta && (
        <span className={`ui-metric__delta ui-metric__delta--${deltaDirection}`}>
          {deltaDirection === 'up' ? '↑' : deltaDirection === 'down' ? '↓' : '•'} {delta}
        </span>
      )}
    </>
  )

  if (onClick) {
    return (
      <button type="button" className={`ui-metric ui-metric--link ${className}`} onClick={onClick}>
        {body}
      </button>
    )
  }
  if (variant === 'card') {
    return <section className={`ui-card glass ui-metric ${className}`}>{body}</section>
  }
  return <div className={`ui-metric ${className}`}>{body}</div>
}
