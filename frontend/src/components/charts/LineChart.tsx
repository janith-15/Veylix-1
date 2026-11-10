import { useId, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { plainValue } from '../../lib/format'

export interface Series {
  label: string
  values: number[]
  color?: string
}

const PALETTE = [
  'var(--chart-1, #5b5bd6)',
  'var(--chart-2, #06b6d4)',
  'var(--chart-3, #3b82f6)',
  'var(--chart-4, #10b981)',
]

// oxlint-disable-next-line react/only-export-components -- shared chart helper consumed by BarChart/DonutChart; co-located with the palette on purpose
export function seriesColor(index: number): string {
  return PALETTE[index % PALETTE.length]
}

function extent(series: Series[]): [number, number] {
  let lo = Infinity
  let hi = -Infinity
  for (const s of series) {
    for (const v of s.values) {
      if (v < lo) lo = v
      if (v > hi) hi = v
    }
  }
  if (!Number.isFinite(lo)) return [0, 1]
  if (lo === hi) return [lo - 1, hi + 1]
  return [lo, hi]
}
interface LineChartProps {
  series: Series[]
  height?: number
  style?: 'smooth' | 'sharp' | 'area'
  showGrid?: boolean
  labels?: string[]
  ariaLabel?: string
  /** Force legend chips on/off (defaults to on when 2+ series). */
  showLegend?: boolean
  /**
   * How a value is turned into text — the tooltip and the Y axis both use it.
   *
   * This is what lets a byte series read `33.3 GB` instead of `33,327,637,690`.
   * Defaulted to a plain grouped number so every existing caller is unchanged.
   */
  valueFormat?: (value: number) => string
  /**
   * Draw a labelled Y axis. Off by default: a 92 px sparkline on a dashboard has
   * no room for an axis, and turning it on everywhere would be a layout change
   * nobody asked for.
   */
  yAxis?: boolean
  /**
   * Anchor the value axis at zero.
   *
   * A line pinned to `lo` exaggerates small differences — for BYTES that is a
   * lie the reader will act on ("33 GB vs 34 GB" drawn as a cliff). A usage
   * series therefore asks for a zero baseline.
   */
  zeroBased?: boolean
}

/** The default value label: a grouped number, never a raw float. */


function linePath(values: number[], w: number, h: number, lo: number, hi: number, smooth: boolean): string {
  const span = hi - lo || 1
  const step = values.length > 1 ? w / (values.length - 1) : w
  const px = (i: number) => i * step
  const py = (v: number) => h - ((v - lo) / span) * h
  let d = ''
  values.forEach((v, i) => {
    if (i === 0) {
      d = 'M' + px(i).toFixed(2) + ' ' + py(v).toFixed(2)
      return
    }
    if (smooth) {
      const cx = (px(i - 1) + px(i)) / 2
      d += ' C' + cx.toFixed(2) + ' ' + py(values[i - 1]).toFixed(2) + ',' + cx.toFixed(2) + ' ' + py(v).toFixed(2) + ',' + px(i).toFixed(2) + ' ' + py(v).toFixed(2)
    } else {
      d += ' L' + px(i).toFixed(2) + ' ' + py(v).toFixed(2)
    }
  })
  return d
}

export function LineChart({ series, height = 180, style = 'area', showGrid = true, labels, ariaLabel, showLegend, valueFormat = plainValue, yAxis = false, zeroBased = false }: LineChartProps) {
  const uid = useId().replace(/:/g, '')
  const W = 600
  const H = height
  /* A labelled axis needs a gutter to sit in; without one the tick text is drawn
     over the plot and clipped at the SVG edge (the "axis labels are clipped"
     defect). 46 px holds the widest realistic byte label, e.g. "1023.9 GB". */
  const padX = yAxis ? 46 : 8
  const padY = 12
  const innerW = W - padX - (yAxis ? 8 : padX)
  const innerH = H - padY * 2 - (yAxis ? 12 : 0)
  const smooth = style !== 'sharp'

  /* Hover + series-toggle interaction (UI 2.1). */
  const [hoverIdx, setHoverIdx] = useState<number | null>(null)
  const [hidden, setHidden] = useState<number[]>([])

  const visible = series
    .map((s, i) => ({ s, i }))
    .filter(({ i }) => !hidden.includes(i))

  const count = visible[0]?.s.values.length ?? 0
  const [rawLo, rawHi] = extent(visible.map(({ s }) => s))
  /* A zero-based axis keeps the plot honest for quantities (bytes, counts). A
     min-max axis is kept for anything else, because a trend line that always
     starts at zero hides the variation it exists to show. */
  const lo = zeroBased ? 0 : rawLo
  const hi = zeroBased ? Math.max(rawHi, 1) : rawHi
  const safeIdx = hoverIdx === null ? null : Math.max(0, Math.min(count - 1, hoverIdx))
  const step = count > 1 ? innerW / (count - 1) : innerW
  const px = (i: number) => i * step
  const py = (v: number) => innerH - ((v - lo) / (hi - lo || 1)) * innerH
  const multi = showLegend ?? series.length > 1

  /* Ticks at 0%, 50% and 100% of the axis — the same three places the grid is
     drawn, so a tick always has a line to sit beside. */
  const ticks = [0, 0.5, 1].map((f) => ({ f, value: hi - (hi - lo) * f }))

  const onPointerMove = (e: ReactPointerEvent<SVGSVGElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    const xRatio = (e.clientX - rect.left) / rect.width
    const idx = Math.round(((xRatio * W - padX) / (step || 1)))
    setHoverIdx(Math.max(0, Math.min(count - 1, idx)))
  }

  const toggleSeries = (i: number) => {
    setHidden((h) =>
      h.includes(i) ? h.filter((x) => x !== i) : hidden.length === series.length - 1 ? h : [...h, i],
    )
  }

  const tipSeries = visible[0]?.s
  const tipX = ((safeIdx === null ? 0 : (padX + px(safeIdx)) / W) * 100)
  const tipYValue = safeIdx === null || !tipSeries ? 0 : Math.max(0, Math.min(innerH, py(tipSeries.values[safeIdx] ?? lo)))
  const tipY = ((padY + tipYValue) / H) * 100
  const flip = tipX > 64
  const showTip = safeIdx !== null && !!tipSeries && tipSeries.values[safeIdx] !== undefined

  return (
    <div className="ui-chart-wrap">
      {multi && (
        <div className="ui-chart-legend" role="list" aria-label="Chart series">
          {series.map((s, i) => {
            const off = hidden.includes(i)
            return (
              <button
                key={s.label + i}
                type="button"
                role="listitem"
                className="ui-chart-legend__item"
                data-off={off}
                aria-pressed={!off}
                onClick={() => toggleSeries(i)}
              >
                <span
                  className="ui-chart-legend__dot"
                  /* D-059: a series colour is runtime data, so it travels as a
                   * custom property and the stylesheet applies it. */
                  style={{ ['--legend-color' as string]: s.color ?? seriesColor(i) }}
                  aria-hidden="true"
                />
                {s.label}
              </button>
            )
          })}
        </div>
      )}
      <svg
        className="ui-chart"
        role="img"
        aria-label={ariaLabel ?? 'Line chart'}
        viewBox={'0 0 ' + W + ' ' + H}
        preserveAspectRatio="none"
        style={{
          /* D-059: the SVG's own height is per-call; width comes from CSS. */
          ['--ui-chart-h' as string]: `${height}px`,
        }}
        onPointerMove={onPointerMove}
        onPointerLeave={() => setHoverIdx(null)}
      >
        <defs>
          {visible.map(({ s, i }) => (
            <linearGradient key={i} id={uid + '-g' + i} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={s.color ?? seriesColor(i)} stopOpacity="0.35" />
              <stop offset="100%" stopColor={s.color ?? seriesColor(i)} stopOpacity="0" />
            </linearGradient>
          ))}
        </defs>
        {showGrid &&
          [0.25, 0.5, 0.75].map((f) => (
            <line
              key={f}
              x1={padX}
              x2={W - padX}
              y1={padY + innerH * f}
              y2={padY + innerH * f}
              stroke="var(--chart-grid, rgba(127,127,160,.2))"
              strokeWidth="1"
              strokeDasharray="3 5"
            />
          ))}

        {/* The value axis. A chart that draws a shape and never states a number
            leaves the reader unable to answer "how much?" — which is the whole
            question a usage chart exists to answer. */}
        {yAxis &&
          ticks.map(({ f, value }) => (
            <text
              key={`tick-${f}`}
              className="ui-chart-axis"
              x={padX - 6}
              y={padY + innerH * f + 4}
              textAnchor="end"
            >
              {valueFormat(value)}
            </text>
          ))}

        {safeIdx !== null && count > 0 && (
          <line
            x1={padX + px(safeIdx)}
            x2={padX + px(safeIdx)}
            y1={padY}
            y2={padY + innerH}
            stroke="var(--chart-axis, var(--text-muted))"
            strokeDasharray="4 4"
            opacity="0.55"
          />
        )}

        <g transform={'translate(' + padX + ',' + padY + ')'}>
          {visible.map(({ s, i }) => {
            const c = s.color ?? seriesColor(i)
            const d = linePath(s.values, innerW, innerH, lo, hi, smooth)
            const hv = safeIdx !== null ? s.values[safeIdx] : undefined
            return (
              <g key={i}>
                {style === 'area' && (
                  <path d={d + ' L' + innerW + ' ' + innerH + ' L0 ' + innerH + ' Z'} fill={'url(#' + uid + '-g' + i + ')'} />
                )}
                <path
                  d={d}
                  fill="none"
                  stroke={c}
                  strokeWidth="2.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  vectorEffect="non-scaling-stroke"
                />
                {hv !== undefined && safeIdx !== null && (
                  <circle cx={px(safeIdx)} cy={py(hv)} r="4.5" fill={c} stroke="var(--bg-card, #fff)" strokeWidth="2" />
                )}
              </g>
            )
          })}
        </g>

        {labels &&
          labels.map((l, i) => {
            const x = padX + (labels.length > 1 ? (innerW / (labels.length - 1)) * i : 0)
            return (
              <text key={i} x={x} y={H - 1} fontSize="11" fill="var(--chart-axis, var(--text-muted))" textAnchor="middle">
                {l}
              </text>
            )
          })}
      </svg>

      {showTip && safeIdx !== null && tipSeries && (
        <div
          className="chart-tip"
          data-flip={flip}
          role="presentation"
          style={{
            /* D-059: tooltip position is genuinely dynamic, so it travels as
             * custom properties and the stylesheet positions the box. */
            ['--chart-tip-x' as string]: `${tipX}%`,
            ['--chart-tip-y' as string]: `${tipY}%`,
          }}
        >
          <div className="chart-tip__title">{labels?.[safeIdx] ?? 'Point ' + (safeIdx + 1)}</div>
          {visible.map(({ s, i }) => (
            <div className="chart-tip__row" key={s.label + i}>
              <span
                className="chart-tip__dot"
                style={{ ['--legend-color' as string]: s.color ?? seriesColor(i) }}
                aria-hidden="true"
              />
              <span>{s.label}</span>
              <span className="chart-tip__val">{valueFormat(s.values[safeIdx] ?? 0)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
