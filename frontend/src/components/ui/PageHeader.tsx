import type { ReactNode } from 'react'

/**
 * Standard page header used by every page: title + subtitle on the
 * left, contextual actions on the right. Keeps page rhythm consistent
 * across the app (Refactoring UI: one clear hierarchy per page).
 */
export function PageHeader({
  title,
  subtitle,
  actions,
}: {
  title: ReactNode
  subtitle?: ReactNode
  actions?: ReactNode
}) {
  return (
    <header className="ui-page-head">
      <div className="ui-page-head__text">
        <h1 className="ui-page-head__title">{title}</h1>
        {subtitle && <p className="ui-page-head__subtitle">{subtitle}</p>}
      </div>
      {actions && <div className="ui-page-head__actions">{actions}</div>}
    </header>
  )
}
