import { useMemo, useState, type ReactNode } from 'react'

export interface Column<T> {
  key: string
  header: ReactNode
  render?: (row: T) => ReactNode
  align?: 'left' | 'right' | 'center'
  width?: string
  /** Hidden below 900px so the important columns are not pushed off-screen. */
  hideOnMobile?: boolean
  /** Click-to-sort header: asc → desc → off. */
  sortable?: boolean
  /**
   * The value to sort by, when it is not `row[key]`.
   *
   * Without this, sorting read `row[key]` and a column whose key is a display
   * name (`placedBy`, `package`) sorted on `undefined` — every row compared
   * equal, so the header arrow moved and nothing else did. A column is allowed
   * to be NAMED differently from the field it renders; the table must be told
   * which field that is instead of guessing.
   */
  sortValue?: (row: T) => string | number | null | undefined
  /**
   * Everything a SEARCH should match for this row's column.
   *
   * Separate from `sortValue` on purpose: sorting wants one canonical field,
   * searching wants every field a human might type. Using the sort field for
   * both meant an order bought FOR somebody was unfindable by their address,
   * because the column sorted on the payer (D-116).
   */
  searchValue?: (row: T) => string
  /**
   * A filter control shown in the table toolbar for this column.
   *
   * `getValue` returns the string a filter matches against, so filtering never
   * depends on what the cell happens to RENDER (a badge, a formatted amount, a
   * two-line cell). A filter that matched rendered text would silently miss
   * rows as soon as someone restyled a cell.
   */
  filter?: {
    kind: 'text' | 'select'
    /** For `kind: 'select'` — the choices offered, in display order. */
    options?: { value: string; label: string }[]
    /** The value a filter matches. Defaults to `row[key]`. */
    getValue?: (row: T) => string
    /** Placeholder for the text input. */
    placeholder?: string
  }
  /**
   * Stays visible on the right edge while the table scrolls horizontally.
   * Use it for the actions column: a primary control that scrolls out of the
   * viewport is indistinguishable from a missing one.
   */
  sticky?: boolean
}

export interface SortState {
  key: string
  dir: 'asc' | 'desc'
}

/**
 * Row selection, opt-in.
 *
 * Without it a bulk operation is impossible: blocking or tagging fifty clients
 * meant fifty dialogs. Selection is deliberately **controlled** (`selected` +
 * `onSelectedChange`) rather than internal, because the bulk bar lives outside the
 * table and both must agree.
 */
export interface SelectionProps {
  /** Keys currently selected (the values produced by `rowKey`). */
  selected: Set<string>
  onSelectedChange: (next: Set<string>) => void
  /** Rows that cannot be selected (e.g. your own account). */
  isSelectable?: (row: unknown) => boolean
}

export function Table<T>({
  columns,
  data,
  rowKey,
  empty,
  initialSort,
  selection,
  rowLabel,
  searchable,
  searchPlaceholder,
  caption,
}: {
  columns: Column<T>[]
  data: T[]
  rowKey: (row: T) => string
  empty?: ReactNode
  initialSort?: SortState | null
  selection?: SelectionProps
  /** Human name for a row, used in checkbox labels. Defaults to the key. */
  rowLabel?: (row: T) => string
  /**
   * Show a search box that matches ANY column's `searchValue`.
   *
   * The box exists so an operator can find one order among hundreds without
   * knowing which column it lives in — the common case is "I have an order
   * number / an email / a reference and I need that row".
   */
  searchable?: boolean
  searchPlaceholder?: string
  /** What the toolbar calls these rows, e.g. "orders". Used in the count line. */
  caption?: string
}) {
  const [sort, setSort] = useState<SortState | null>(initialSort ?? null)
  const [query, setQuery] = useState('')
  const [filters, setFilters] = useState<Record<string, string>>({})

  const filterable = useMemo(() => columns.filter((c) => c.filter), [columns])

  const searched = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return data
    /*
     * One box covers every column. Each column contributes its `searchValue`
     * when it has one, otherwise its sort/filter value — so an order is
     * findable by number, payer, recipient, package or reference without the
     * operator having to pick the right box first.
     */
    return data.filter((row) =>
      columns.some((c) => {
        const raw = c.searchValue
          ? c.searchValue(row)
          : c.filter?.getValue
            ? c.filter.getValue(row)
            : c.sortValue
              ? c.sortValue(row)
              : (row as Record<string, unknown>)[c.key]
        return String(raw ?? '').toLowerCase().includes(q)
      }),
    )
  }, [data, columns, query])

  const filtered = useMemo(() => {
    const active = Object.entries(filters).filter(([, v]) => v !== '')
    if (active.length === 0) return searched
    return searched.filter((row) =>
      active.every(([key, want]) => {
        const col = columns.find((c) => c.key === key)
        if (!col?.filter) return true
        const raw = col.filter.getValue
          ? col.filter.getValue(row)
          : String((row as Record<string, unknown>)[col.key] ?? '')
        // Selects match exactly: "PENDING" must not also match "PENDING_REVIEW".
        return col.filter.kind === 'select' ? raw === want : raw.toLowerCase().includes(want.toLowerCase())
      }),
    )
  }, [searched, filters, columns])

  const sorted = useMemo(() => {
    if (!sort) return filtered
    const col = columns.find((c) => c.key === sort.key)
    if (!col?.sortable) return filtered
    const factor = sort.dir === 'asc' ? 1 : -1
    const value = (row: T) => (col.sortValue ? col.sortValue(row) : (row as Record<string, unknown>)[sort.key])
    return [...filtered].sort((a, b) => {
      const av = value(a)
      const bv = value(b)
      /* Numeric-aware: numeric strings ("10" vs "9") must not sort lexically. */
      const an = typeof av === 'number' ? av : Number.parseFloat(String(av))
      const bn = typeof bv === 'number' ? bv : Number.parseFloat(String(bv))
      if (Number.isFinite(an) && Number.isFinite(bn)) return (an - bn) * factor
      return String(av ?? '').localeCompare(String(bv ?? '')) * factor
    })
  }, [filtered, columns, sort])

  const toggleSort = (col: Column<T>) => {
    if (!col.sortable) return
    setSort((s) =>
      s && s.key === col.key ? (s.dir === 'asc' ? { key: col.key, dir: 'desc' } : null) : { key: col.key, dir: 'asc' },
    )
  }

  // Select-all covers what is VISIBLE (the current page and filter), never the whole
  // database: a checkbox that silently means "everything, including what you cannot
  // see" is how a bulk action destroys data.
  const selectableRows = selection
    ? sorted.filter((r) => (selection.isSelectable ? selection.isSelectable(r) : true))
    : []
  const visibleKeys = selectableRows.map((r) => rowKey(r))
  const selectedVisible = visibleKeys.filter((k) => selection?.selected.has(k)).length
  const allVisible = visibleKeys.length > 0 && selectedVisible === visibleKeys.length
  const someVisible = selectedVisible > 0 && !allVisible

  const toggleAll = () => {
    if (!selection) return
    const next = new Set(selection.selected)
    if (allVisible) visibleKeys.forEach((k) => next.delete(k))
    else visibleKeys.forEach((k) => next.add(k))
    selection.onSelectedChange(next)
  }

  const toggleOne = (key: string) => {
    if (!selection) return
    const next = new Set(selection.selected)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    selection.onSelectedChange(next)
  }

  if (data.length === 0 && empty) return <div className="ui-card">{empty}</div>

  const activeFilters = Object.entries(filters).filter(([, v]) => v !== '')
  const total = data.length
  const shown = sorted.length
  const isNarrowed = shown !== total

  return (
    <div className="ui-table-wrap" data-testid="table">
      {(searchable || filterable.length > 0) && (
        <div className="ui-table__bar" data-testid="table-filters">
          {searchable && (
            <input
              type="search"
              className="ui-input ui-table__search"
              data-testid="table-search"
              placeholder={searchPlaceholder ?? 'Search…'}
              aria-label={searchPlaceholder ?? 'Search'}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          )}
          {filterable.map((c) => {
            const f = c.filter!
            const value = filters[c.key] ?? ''
            if (f.kind === 'select') {
              return (
                <select
                  key={c.key}
                  className="ui-input ui-table__filter"
                  data-testid={`table-filter-${c.key}`}
                  aria-label={`Filter by ${typeof c.header === 'string' ? c.header : c.key}`}
                  value={value}
                  onChange={(e) => setFilters({ ...filters, [c.key]: e.target.value })}
                >
                  <option value="">
                    {typeof c.header === 'string' ? `All ${c.header.toLowerCase()}` : 'All'}
                  </option>
                  {(f.options ?? []).map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              )
            }
            return (
              <input
                key={c.key}
                type="search"
                className="ui-input ui-table__filter"
                data-testid={`table-filter-${c.key}`}
                aria-label={`Filter by ${typeof c.header === 'string' ? c.header : c.key}`}
                placeholder={f.placeholder ?? `Filter ${typeof c.header === 'string' ? c.header.toLowerCase() : c.key}…`}
                value={value}
                onChange={(e) => setFilters({ ...filters, [c.key]: e.target.value })}
              />
            )
          })}
          {(activeFilters.length > 0 || query !== '') && (
            <button
              type="button"
              className="ui-table__clear"
              data-testid="table-clear-filters"
              onClick={() => {
                setFilters({})
                setQuery('')
              }}
            >
              Clear
            </button>
          )}
          {/*
           * The count is stated, not implied. A filtered table that silently
           * shows 3 of 240 rows reads as "there are 3 orders"; saying
           * "3 of 240" is the only honest form.
           */}
          <span className="ui-table__count t-sm2" data-testid="table-count">
            {isNarrowed
              ? `Showing ${shown} of ${total}${caption ? ` ${caption}` : ''}`
              : `${total}${caption ? ` ${caption}` : ''}`}
          </span>
        </div>
      )}
      <table className="ui-table">
        <thead>
          <tr>
            {selection && (
              <th className="ui-table__check" scope="col">
                <input
                  type="checkbox"
                  aria-label="Select every row on this page"
                  checked={allVisible}
                  ref={(el) => {
                    // `indeterminate` cannot be set from JSX; it is the honest third
                    // state when only some visible rows are selected.
                    if (el) el.indeterminate = someVisible
                  }}
                  onChange={toggleAll}
                />
              </th>
            )}
            {columns.map((c) => {
              const dir = sort?.key === c.key ? sort.dir : null
              return (
                <th
                  key={c.key}
                  aria-sort={dir === 'asc' ? 'ascending' : dir === 'desc' ? 'descending' : undefined}
                  // D-059: alignment and width are DATA, not inline styles, so the
                  // rule lives in the stylesheet keyed on the attribute. Using a
                  // style prop here put one inline declaration on every cell of
                  // every table in the app.
                  data-align={c.align}
                  data-width={c.width ? 'fixed' : undefined}
                  className={cellClass(c)}
                >
                  {c.sortable ? (
                    <button type="button" className="ui-table__sort" onClick={() => toggleSort(c)}>
                      {c.header}
                      <span className="ui-table__sort-arrow" data-dir={dir ?? undefined} aria-hidden="true">
                        ↕
                      </span>
                    </button>
                  ) : (
                    c.header
                  )}
                </th>
              )
            })}
          </tr>
        </thead>
        <tbody>
          {sorted.map((row) => {
            const key = rowKey(row)
            const canSelect = selection
              ? selection.isSelectable
                ? selection.isSelectable(row)
                : true
              : false
            const checked = selection?.selected.has(key) ?? false
            const name = rowLabel ? rowLabel(row) : key
            return (
              <tr key={key} data-selected={selection ? checked : undefined}>
                {selection && (
                  <td className="ui-table__check">
                    <input
                      type="checkbox"
                      aria-label={canSelect ? `Select ${name}` : `${name} cannot be selected`}
                      checked={checked}
                      disabled={!canSelect}
                      onChange={() => toggleOne(key)}
                    />
                  </td>
                )}
                {columns.map((c) => (
                  <td
                    key={c.key}
                    data-align={c.align}
                    className={cellClass(c)}
                    // On a phone the header row is hidden and each cell becomes a
                    // labelled row, so a value never appears without its column name.
                    data-label={typeof c.header === 'string' ? c.header : undefined}
                  >
                    {c.render ? c.render(row) : String((row as Record<string, unknown>)[c.key] ?? '')}
                  </td>
                ))}
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

/** Class names for a column: responsive hiding and the sticky actions edge. */
function cellClass<T>(c: Column<T>): string {
  const cls: string[] = []
  if (c.hideOnMobile) cls.push('ui-table__cell--hide-sm')
  if (c.sticky) cls.push('ui-table__cell--sticky')
  return cls.join(' ')
}
