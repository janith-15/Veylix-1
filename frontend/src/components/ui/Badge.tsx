import type { ReactNode } from 'react'

export type BadgeTone = 'default' | 'primary' | 'success' | 'warning' | 'error' | 'info'

export function Badge({
  tone = 'default',
  children,
  className = '',
  dot = false,
}: {
  tone?: BadgeTone
  children: ReactNode
  className?: string
  /** Show a leading status dot (styled per theme). */
  dot?: boolean
}) {
  return (
    <span className={'ui-badge ui-badge--' + tone + (dot ? ' ui-badge--dot' : '') + ' ' + className}>
      {dot && <span className="ui-badge__dot" aria-hidden="true" />}
      {children}
    </span>
  )
}
