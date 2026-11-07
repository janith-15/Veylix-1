import { seriesColor } from './LineChart'

/** Tiny inline trend line for tables and metric cards. */
export function Sparkline({
  values,
  width = '100%',
  height = 28,
  color,
  ariaLabel,
}: {
  values: number[]
  /** A number fixes the width; `'100%'` (default) fills the container. */
  width?: number | string
  height?: number
  color?: string
  ariaLabel?: string
}) {
  if (!values || values.length === 0) return null
  const vbW = 100
  const lo = Math.min(...values)
  const hi = Math.max(...values)
  const span = hi - lo || 1
  /* Single point: draw a flat mid-line instead of a zero-length path. */
  const step = values.length > 1 ? vbW / (values.length - 1) : vbW
  const pts = values.map((v, i) => [i * step, height - ((v - lo) / span) * height])
  const d = pts.map((p, i) => (i === 0 ? 'M' : 'L') + p[0].toFixed(2) + ' ' + p[1].toFixed(2)).join(' ')
  const c = color ?? seriesColor(0)
  const unit = typeof width === 'number'

  return (
    <svg
      className="ui-sparkline"
      role="img"
      aria-label={ariaLabel ?? 'Trend'}
      viewBox={'0 0 ' + vbW + ' ' + height}
      /* `none` stretches strokes when the container is wide — `meet` keeps
         line weight honest. */
      preserveAspectRatio="xMidYMid meet"
      style={{ width: unit ? width : '100%', height, display: 'block' }}
    >
      <path
        d={d}
        fill="none"
        stroke={c}
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  )
}
