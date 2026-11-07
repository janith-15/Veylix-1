import type { ReactNode } from 'react'

export function Tooltip({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <span className="ui-tooltip" tabIndex={0}>
      {children}
      <span className="ui-tooltip__tip" role="tooltip">
        {label}
      </span>
    </span>
  )
}
