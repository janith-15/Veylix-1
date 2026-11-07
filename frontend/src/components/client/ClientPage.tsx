import type { ReactNode } from 'react'
import { PageHeader } from '../ui'
import './client.css'

/**
 * The shared frame every Client Portal screen renders inside (S3).
 *
 * Why a frame at all: the client portal grew one page at a time, so each screen
 * invented its own header, its own summary strip and its own spacing. The result
 * was that "what page am I on" and "is this the same product" had to be answered
 * per screen. This component states the four things that make a client screen
 * consistent, once:
 *
 *   1. one `<h1>` and one description (`PageHeader`, which every role already
 *      uses — so the client portal cannot drift from the operator console);
 *   2. one optional summary strip directly under it, for the two or four numbers
 *      that answer "how am I doing" before the list;
 *   3. one optional toolbar for search/filter controls;
 *   4. one content region that owns its own loading/empty/error states.
 *
 * This is a LAYOUT component only. It holds no state, fetches nothing and
 * derives nothing — every number it renders comes from the caller, so a page
 * cannot accidentally claim a fact the backend did not send.
 *
 * It deliberately renders no card chrome of its own: the surfaces belong to
 * `Card`/`MetricCard`, which already consume the theme tokens. That keeps this
 * file working across every preset and sub-theme without knowing they exist.
 */
export function ClientPage({
  title,
  description,
  actions,
  summary,
  toolbar,
  children,
  testId,
}: {
  title: ReactNode
  description?: ReactNode
  /** Page-level actions (top right), e.g. "Buy premium". */
  actions?: ReactNode
  /**
   * The summary strip. Render `MetricCard`s (or nothing). Kept as a slot rather
   * than a prop shape so a page decides which numbers are honest to show — the
   * dashboard shows four, a list page may show two, and a page with no data
   * shows none instead of four zeroes.
   */
  summary?: ReactNode
  /** Search / filter / sort controls, above the list. */
  toolbar?: ReactNode
  children: ReactNode
  testId?: string
}) {
  return (
    <div className="dash client-page" data-testid={testId}>
      <PageHeader title={title} subtitle={description} actions={actions} />
      {summary && <div className="ui-grid client-page__summary">{summary}</div>}
      {toolbar && <div className="client-page__toolbar">{toolbar}</div>}
      <div className="client-page__body">{children}</div>
    </div>
  )
}

/**
 * A named section inside a client page (S3).
 *
 * The VPN detail screen already established this pattern with
 * `.vpn-section-title` — a real `<h2>`, so the sections are navigable by a
 * screen reader and not merely separated visually. This reuses that class
 * rather than defining a second heading style, which is what keeps a new page
 * looking like the rest of the portal without touching any stylesheet token.
 */
export function ClientSection({
  title,
  actions,
  children,
}: {
  title: string
  /** Counts or a link, shown on the same row as the title. */
  actions?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="client-section" aria-label={title}>
      <div className="client-section__head">
        <h2 className="vpn-section-title client-section__title">{title}</h2>
        {actions && <div className="client-section__actions">{actions}</div>}
      </div>
      {children}
    </section>
  )
}
