import { ChevronLeft, ChevronRight } from 'lucide-react'

function buildPages(current: number, total: number): (number | '…')[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)
  const pages: (number | '…')[] = [1]
  if (current > 3) pages.push('…')
  for (let p = Math.max(2, current - 1); p <= Math.min(total - 1, current + 1); p++) pages.push(p)
  if (current < total - 2) pages.push('…')
  pages.push(total)
  return pages
}

export function Pagination({
  page,
  pageCount,
  onChange,
}: {
  page: number
  pageCount: number
  onChange: (page: number) => void
}) {
  if (pageCount <= 1) return null
  const pages = buildPages(page, pageCount)
  return (
    <nav className="ui-pagination" aria-label="Pagination">
      <button className="ui-pagination__btn" aria-label="Previous page" disabled={page <= 1} onClick={() => onChange(page - 1)}>
        <ChevronLeft size={16} />
      </button>
      {pages.map((p, i) =>
        p === '…' ? (
          <span key={`dots-${i}`} className="ui-muted ui-pagination__dots">
            …
          </span>
        ) : (
          <button
            key={p}
            className={`ui-pagination__btn ${p === page ? 'ui-pagination__btn--active' : ''}`}
            aria-current={p === page ? 'page' : undefined}
            onClick={() => onChange(p)}
          >
            {p}
          </button>
        ),
      )}
      <button className="ui-pagination__btn" aria-label="Next page" disabled={page >= pageCount} onClick={() => onChange(page + 1)}>
        <ChevronRight size={16} />
      </button>
    </nav>
  )
}
