import type { ReactNode } from 'react'

export function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon?: ReactNode
  title: string
  description?: string
  action?: ReactNode
}) {
  return (
    <div className="ui-empty" data-testid="empty-state">
      {icon && <div className="ui-empty__icon">{icon}</div>}
      {/*
        A real heading, not a div: an empty or not-found screen is the WHOLE
        content of the page, and a screen reader that gets no heading has nothing
        to announce. `ui-empty__title` strips the browser's h2 defaults so the
        look is unchanged.
      */}
      <h2 className="ui-strong ui-empty__title">{title}</h2>
      {/* D-059: the measure lives in `.ui-empty__desc`, not a style prop. */}
      {description && <p className="ui-muted ui-empty__desc">{description}</p>}
      {action && <div className="ui-mt-1">{action}</div>}
    </div>
  )
}
