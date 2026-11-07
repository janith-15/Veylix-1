import { useState } from 'react'
import { seriesColor } from './LineChart'

export interface DonutSegment {
  label: string
  value: number
  color?: string
}

/**
 * Donut / ring chart. Segments default to the theme chart palette.
 * Hover (segment or legend row) swaps the center label to that share and
 * slightly grows the hovered arc.
 */
export function DonutChart({
  segments,
  size = 168,
  thickness = 22,
  centerLabel,
  ariaLabel,
  legend = true,
}: {
  segments: DonutSegment[]
  size?: number
  thickness?: number
  centerLabel?: string
  ariaLabel?: string
  legend?: boolean
}) {
  const [hover, setHover] = useState<number | null>(null)
  const total = segments.reduce((a, s) => a + s.value, 0)
  const r = (size - thickness) / 2
  const mid = size / 2
  const circ = 2 * Math.PI * r

  // Pure derivation: arc lengths, then a reduce-based running offset.
  const lengths = segments.map((s) => (total > 0 ? s.value / total : 0) * circ)
  const arcs = lengths.map((len, i) => ({
    key: segments[i].label + i,
    len,
    dashoffset: -lengths.slice(0, i).reduce((a, b) => a + b, 0),
    color: segments[i].color ?? seriesColor(i),
  }))

  const share = (i: number) => Math.round((segments[i].value / (total || 1)) * 100) + '%'
  const center = hover !== null && segments[hover] ? share(hover) : centerLabel

  return (
    <div className="ui-chart-root" style={{ width: '100%', minWidth: 0 }}>
      {/* The ring owns its own fixed box; the legend flows underneath it at
          full container width instead of being trapped inside a 132px wrap. */}
      <div className="ui-chart-wrap" style={{ width: size + 'px', maxWidth: '100%', margin: '0 auto' }}>
        <svg
          className="ui-chart ui-chart--donut"
          role="img"
          aria-label={ariaLabel ?? 'Donut chart'}
          width={size}
          height={size}
          viewBox={'0 0 ' + size + ' ' + size}
          onMouseLeave={() => setHover(null)}
          /* Hover grows stroke-width by 3px — keep it inside the viewBox. */
          style={{ margin: '0 auto', display: 'block', touchAction: 'pan-y', overflow: 'visible', padding: 2 }}
        >
          <circle cx={mid} cy={mid} r={r} fill="none" stroke="var(--chart-grid, rgba(127,127,160,.2))" strokeWidth={thickness} />
          {arcs.map((a, i) => (
            <circle
              key={a.key}
              cx={mid}
              cy={mid}
              r={r}
              fill="none"
              stroke={a.color}
              strokeWidth={hover === i ? thickness + 3 : thickness}
              strokeDasharray={a.len + ' ' + (circ - a.len)}
              strokeDashoffset={a.dashoffset}
              transform={'rotate(-90 ' + mid + ' ' + mid + ')'}
              style={{ pointerEvents: 'stroke' }}
              onMouseEnter={() => setHover(i)}
              opacity={hover === null || hover === i ? 1 : 0.55}
            />
          ))}
          <text
            x={mid}
            y={mid}
            textAnchor="middle"
            dominantBaseline="central"
            fontSize="20"
            fontWeight="700"
            fill="var(--text-strong, currentColor)"
          >
            {center}
          </text>
        </svg>
      </div>

      {legend && segments.length > 0 && (
        <div className="ui-chart-legend flow-top" role="list" aria-label="Segments">
          {segments.map((s, i) => (
            <div
              key={s.label + i}
              role="listitem"
              className="ui-chart-legend__item"
              data-hover={hover === i}
              style={hover === i ? { background: 'var(--bg-hover)' } : undefined}
              onMouseEnter={() => setHover(i)}
              onMouseLeave={() => setHover(null)}
            >
              <span className="ui-chart-legend__dot" style={{ background: s.color ?? seriesColor(i) }} aria-hidden="true" />
              {s.label}
              <span className="ui-chart-legend__pct">{share(i)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
