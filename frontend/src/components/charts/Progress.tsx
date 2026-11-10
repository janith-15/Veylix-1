/** Horizontal progress meter; fill follows the theme accent. */
export function Progress({
  value,
  max = 100,
  label,
  tone = 'accent',
  height = 8,
}: {
  value: number
  max?: number
  label?: string
  tone?: 'accent' | 'success' | 'warning' | 'error'
  height?: number
}) {
  const pct = Math.max(0, Math.min(100, (value / (max || 1)) * 100))
  return (
    <div className={'ui-progress ui-progress--' + tone} role="progressbar" aria-valuenow={Math.round(pct)} aria-valuemin={0} aria-valuemax={100} aria-label={label ?? 'Progress'}>
      {label && (
        <div className="ui-progress__head">
          <span>{label}</span>
          <span>{Math.round(pct)}%</span>
        </div>
      )}
      <div
        className="ui-progress__track"
        /* D-059: `height` and the fill percentage are genuine per-call values,
         * so they travel as custom properties and the stylesheet consumes
         * them. A plain `style={{ height }}` put two inline declarations on
         * every meter in the application. */
        style={{ ['--ui-progress-h' as string]: `${height}px`, ['--ui-progress-pct' as string]: `${pct}%` }}
      >
        <div className="ui-progress__fill" />
      </div>
    </div>
  )
}
