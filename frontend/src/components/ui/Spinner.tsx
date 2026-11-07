export function Spinner({ size = 24, label }: { size?: number; label?: string }) {
  return (
    <span className="ui-spinner" style={{ width: size, height: size }} role="status" aria-label={label ?? 'Loading'} aria-live="polite" />
  )
}
