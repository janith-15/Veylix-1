import type { HTMLAttributes, ReactNode } from 'react'

interface CardProps {
  title?: ReactNode
  actions?: ReactNode
  children: ReactNode
  className?: string
  hover?: boolean
}

export function Card({
  title,
  actions,
  children,
  className = '',
  hover = false,
  ...rest
}: CardProps & Omit<HTMLAttributes<HTMLElement>, 'title' | 'children'>) {
  return (
    /**
     * Glass is part of the design identity — cards keep it. Scene bleed is
     * handled at the scene layer (low opacity + blur), not by flattening cards.
     * Users who want solid surfaces can toggle "Glass effect" in Theme Studio.
     *
     * `...rest` forwards `data-*` and ARIA attributes: a card that silently
     * dropped `data-testid` made a verification probe report "0 cards" about a
     * screen that rendered fine.
     */
    <section
      {...rest}
      className={`ui-card glass ${hover ? 'ui-card--hover' : ''} ${className}`}
    >
      {(title || actions) && (
        <header className="ui-card__header">
          {title && <h3 className="ui-card__title">{title}</h3>}
          {actions && <div className="ui-row">{actions}</div>}
        </header>
      )}
      {children}
    </section>
  )
}
