export function Skeleton({
  width = '100%',
  height = 16,
  className = '',
  radius,
  style,
}: {
  width?: string | number
  height?: string | number
  className?: string
  radius?: number
  style?: React.CSSProperties
}) {
  return (
    <div
      className={`ui-skeleton ${className}`}
      aria-hidden="true"
      data-testid="skeleton"
      style={{ width, height, borderRadius: radius, ...style }}
    />
  )
}
