import type { ReactNode } from 'react'
import { X } from 'lucide-react'
import { Button } from './Button'

/**
 * The action bar for a table selection.
 *
 * It appears only when something is selected, states the count in words, and can be
 * dismissed — so the operator always knows what a bulk action will touch and can
 * back out without hunting for an undo.
 */
export function SelectionBar({
  count,
  onClear,
  children,
}: {
  count: number
  onClear: () => void
  children: ReactNode
}) {
  if (count === 0) return null
  return (
    <div className="ui-selection" role="region" aria-label="Bulk actions">
      <span className="ui-selection__count">
        <strong>{count}</strong> selected
      </span>
      <div className="ui-selection__actions">{children}</div>
      <Button size="sm" variant="ghost" leading={<X size={13} />} onClick={onClear}>
        Clear
      </Button>
    </div>
  )
}
