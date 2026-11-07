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
      <div className="ui-progress__track" style={{ height }}>
        <div className="ui-progress__fill" style={{ width: pct + '%' }} />
      </div>
    </div>
  )
}
