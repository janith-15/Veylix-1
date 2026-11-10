export function Spinner({ size = 24, label }: { size?: number; label?: string }) {
  return (
    /*
     * D-059: a pixel size is genuinely dynamic, so it is passed as a CSS custom
     * property — the one inline form the rule allows — and the stylesheet reads
     * it. A fixed `width`/`height` style prop would have been an inline
     * declaration on every loading state in the app.
     */
    <span
      className="ui-spinner"
      style={{ ['--ui-spinner-size' as string]: `${size}px` }}
      role="status"
      aria-label={label ?? 'Loading'}
      aria-live="polite"
    />
  )
}
