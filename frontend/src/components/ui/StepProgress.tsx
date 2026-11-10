import type { ReactNode } from 'react'

/**
 * Progress for a multi-step flow.
 *
 * The visual dots were previously `aria-hidden`, so a screen-reader user had no
 * way to know which step they were on — the flow appeared to be one long form with
 * no beginning or end. This renders the same dots AND an announced label
 * ("Step 2 of 3 — Payment"), so the state is available to everyone without
 * changing the look.
 *
 * `nav` is for a real `<nav>` landmark only when the flow has its own address;
 * a wizard inside one page uses a plain div with `aria-current="step"`.
 */
export function StepProgress({
  steps,
  current,
  label,
}: {
  /** Human names, in order. The index of `current` is 1-based against this list. */
  steps: string[]
  current: number
  /** Optional override for the announced text. */
  label?: ReactNode
}) {
  const total = steps.length
  const safe = Math.min(Math.max(current, 1), total)
  const name = steps[safe - 1] ?? ''

  return (
    <div className="ui-col ui-step-progress">
      <div className="sell-step-dots" aria-hidden="true">
        {steps.map((_, i) => (
          <span key={i} data-on={safe >= i + 1} />
        ))}
      </div>
      <span className="ui-muted t-sm2">
        {label ?? `Step ${safe} of ${total} — ${name}`}
      </span>
      {/* Announced politely: a step change is progress, not an interruption. */}
      <span className="ui-sr-only" role="status" aria-live="polite">
        {`Step ${safe} of ${total} — ${name}`}
      </span>
    </div>
  )
}
