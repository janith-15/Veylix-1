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
    /*
     * D-059: a skeleton's width/height are genuinely per-call values, so they are
     * passed as CSS custom properties (the one allowed inline form) and the
     * stylesheet reads them. `style` remains available for a caller's own custom
     * properties; it is no longer used to carry plain layout values.
     */
    <div
      className={`ui-skeleton ${className}`}
      aria-hidden="true"
      data-testid="skeleton"
      style={{
        ['--ui-skeleton-w' as string]: typeof width === 'number' ? `${width}px` : width,
        ['--ui-skeleton-h' as string]: typeof height === 'number' ? `${height}px` : height,
        ...(radius !== undefined ? { ['--ui-skeleton-r' as string]: `${radius}px` } : {}),
        ...style,
      }}
    />
  )
}
