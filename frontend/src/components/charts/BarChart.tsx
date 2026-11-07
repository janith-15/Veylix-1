import { useState, type PointerEvent as ReactPointerEvent } from 'react'
import { seriesColor } from './LineChart'

export interface BarDatum {
  label: string
  value: number
}

/** Vertical bar chart with hover tooltip, dim-others feedback and value labels. */
export function BarChart({
  data,
  height = 180,
  showGrid = true,
  showValues = false,
  ariaLabel,
}: {
  data: BarDatum[]
  height?: number
  showGrid?: boolean
  showValues?: boolean
  ariaLabel?: string
}) {
  const [hover, setHover] = useState<number | null>(null)

  const W = 600
  const H = height
  const padX = 8
  const padY = 12
  const innerW = W - padX * 2
  const innerH = H - padY * 2 - 16
  const max = Math.max(1, ...data.map((d) => d.value))
  const slot = innerW / Math.max(1, data.length)
  const barW = Math.min(46, slot * 0.58)

  const hovered = hover !== null ? data[hover] : null
  const tipX = hover === null ? 0 : ((padX + slot * hover + slot / 2) / W) * 100
  const tipY = hovered === null ? 0 : ((padY + innerH - (hovered.value / max) * innerH) / H) * 100
  const flip = tipX > 62

  return (
    <div className="ui-chart-wrap">
      <svg
        className="ui-chart"
        role="img"
        aria-label={ariaLabel ?? 'Bar chart'}
        viewBox={'0 0 ' + W + ' ' + H}
        preserveAspectRatio="none"
        style={{ width: '100%', height, display: 'block', touchAction: 'pan-y' }}
        onPointerLeave={() => setHover(null)}
        onPointerMove={(e: ReactPointerEvent<SVGSVGElement>) => {
          /* Pointer-driven slot detection keeps touch + mouse identical. */
          const rect = e.currentTarget.getBoundingClientRect()
          const xRatio = (e.clientX - rect.left) / rect.width
          const idx = Math.floor(((xRatio * W - padX) / (slot || 1)))
          setHover(idx >= 0 && idx < data.length ? idx : null)
        }}
      >
        {showGrid &&
          [0.25, 0.5, 0.75].map((f) => (
            <line
              key={f}
              x1={padX}
              x2={W - padX}
              y1={padY + innerH * f}
              y2={padY + innerH * f}
              stroke="var(--chart-grid, rgba(127,127,160,.2))"
              strokeDasharray="3 5"
            />
          ))}
        {data.map((d, i) => {
          const c = seriesColor(i)
          const h = (d.value / max) * innerH
          const x = padX + slot * i + (slot - barW) / 2
          const y = padY + innerH - h
          const dim = hover !== null && hover !== i
          return (
            <g key={d.label + i}>
              <rect
                x={x}
                y={y}
                width={barW}
                height={Math.max(2, h)}
                rx="var(--bar-radius, 4)"
                fill={c}
                opacity={hover === null ? 0.9 : dim ? 0.28 : 1}
                style={{ transition: 'opacity var(--dur-fast, 120ms) var(--ease, ease)' }}
              />
              {hover === i && (
                <rect
                  x={x - 2}
                  y={y - 2}
                  width={barW + 4}
                  height={Math.max(2, h) + 2}
                  rx="calc(var(--bar-radius, 4) + 2px)"
                  fill="none"
                  stroke={c}
                  strokeWidth="1.5"
                  pointerEvents="none"
                />
              )}
              {showValues && (
                <text
                  x={padX + slot * i + slot / 2}
                  y={y - 6}
                  fontSize="11"
                  fontWeight="600"
                  textAnchor="middle"
                  fill="var(--text-muted)"
                >
                  {d.value.toLocaleString()}
                </text>
              )}
              <text
                x={padX + slot * i + slot / 2}
                y={H - 3}
                fontSize="11"
                textAnchor="middle"
                fill="var(--chart-axis, var(--text-muted))"
              >
                {d.label}
              </text>
            </g>
          )
        })}
      </svg>

      {hovered && (
        <div
          className="chart-tip"
          data-flip={flip}
          role="presentation"
          style={{ left: tipX + '%', top: tipY + '%' }}
        >
          <div className="chart-tip__title">{hovered.label}</div>
          <div className="chart-tip__row">
            <span className="chart-tip__dot" style={{ background: seriesColor(hover ?? 0) }} aria-hidden="true" />
            <span>{hovered.label}</span>
            <span className="chart-tip__val">{hovered.value.toLocaleString()}</span>
          </div>
        </div>
      )}
    </div>
  )
}
