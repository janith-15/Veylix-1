import type { ReactNode } from 'react'
import { Search, X } from 'lucide-react'
import { Button } from '../ui'
import './client.css'

/**
 * The Client Portal's one search + filter bar (S3).
 *
 * Built ON TOP of the existing `hooks/useListQuery` rather than beside it. That
 * hook already solves the four problems this bar would otherwise re-invent:
 * state lives in the URL (so Back/Forward and a shared link work), only
 * non-default values reach the URL, text is debounced so a search box is not one
 * query per keystroke, and changing a filter resets the page number.
 *
 * What this component adds is only the markup: a labelled search field and a set
 * of single-select filters, all keyboard reachable and all announced. A caller
 * passes `value` and `onChange`, so the bar holds no state and cannot disagree
 * with the URL.
 *
 * Deliberately does NOT do the filtering. Every list in this portal is filtered
 * from data the server already returned, so an empty result can never be caused
 * by the UI deciding not to render a row the backend sent.
 */
export interface ClientFilterOption {
  /** The value written to the URL. */
  value: string
  /** What the person reads. */
  label: string
}

export interface ClientFilterGroup {
  /** URL key — must exist in the caller's `useListQuery` defaults. */
  key: string
  /** Accessible label for the group. */
  label: string
  options: ClientFilterOption[]
  /** Optional live count per option value, shown in the label. */
  counts?: Record<string, number>
}

export function ClientFilters({
  search,
  onSearch,
  searchPlaceholder = 'Search…',
  groups = [],
  values,
  onChange,
  onReset,
  isFiltered,
  children,
}: {
  /** The current (draft) search text. */
  search?: string
  onSearch?: (next: string) => void
  searchPlaceholder?: string
  groups?: ClientFilterGroup[]
  /** Current committed value per group key (a non-string is stringified). */
  values: Record<string, string | number | boolean>
  onChange: (key: string, value: string) => void
  onReset: () => void
  isFiltered: boolean
  /** Extra controls rendered at the end of the row (e.g. a sort select). */
  children?: ReactNode
}) {
  return (
    <div className="client-filters" role="group" aria-label="Search and filter">
      {onSearch && (
        <div className="client-filters__search">
          <label className="ui-sr-only" htmlFor="client-filter-search">
            {searchPlaceholder}
          </label>
          <Search size={15} aria-hidden="true" className="client-filters__icon" />
          <input
            id="client-filter-search"
            className="client-filters__input"
            type="search"
            value={search ?? ''}
            placeholder={searchPlaceholder}
            autoComplete="off"
            onChange={(e) => onSearch(e.target.value)}
          />
          {search ? (
            <button
              type="button"
              className="client-filters__clear"
              aria-label="Clear search"
              onClick={() => onSearch('')}
            >
              <X size={14} aria-hidden="true" />
            </button>
          ) : null}
        </div>
      )}

      {groups.map((g) => {
        const current = String(values[g.key] ?? '')
        return (
          <div className="client-filters__group" key={g.key}>
            <label className="ui-sr-only" htmlFor={`client-filter-${g.key}`}>
              {g.label}
            </label>
            <select
              id={`client-filter-${g.key}`}
              className="client-filters__select"
              value={current}
              onChange={(e) => onChange(g.key, e.target.value)}
            >
              {g.options.map((o) => {
                // A count is only rendered when the caller measured it, so an
                // option can never display an invented total.
                const n = g.counts?.[o.value]
                return (
                  <option key={o.value} value={o.value}>
                    {n === undefined ? o.label : `${o.label} (${n})`}
                  </option>
                )
              })}
            </select>
          </div>
        )
      })}

      {children}

      {/* Reset appears only when there is something to reset: a permanently
          visible "Clear" on an unfiltered list is a control that does nothing. */}
      {isFiltered && (
        <Button size="sm" variant="ghost" onClick={onReset} leading={<X size={13} />}>
          Clear filters
        </Button>
      )}
    </div>
  )
}
