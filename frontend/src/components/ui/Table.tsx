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
}: {
  columns: Column<T>[]
  data: T[]
  rowKey: (row: T) => string
  empty?: ReactNode
  initialSort?: SortState | null
  selection?: SelectionProps
  /** Human name for a row, used in checkbox labels. Defaults to the key. */
  rowLabel?: (row: T) => string
}) {
  const [sort, setSort] = useState<SortState | null>(initialSort ?? null)

  const sorted = useMemo(() => {
    if (!sort) return data
    const col = columns.find((c) => c.key === sort.key)
    if (!col?.sortable) return data
    const factor = sort.dir === 'asc' ? 1 : -1
    const value = (row: T) => (row as Record<string, unknown>)[sort.key]
    return [...data].sort((a, b) => {
      const av = value(a)
      const bv = value(b)
      /* Numeric-aware: numeric strings ("10" vs "9") must not sort lexically. */
      const an = typeof av === 'number' ? av : Number.parseFloat(String(av))
      const bn = typeof bv === 'number' ? bv : Number.parseFloat(String(bv))
      if (Number.isFinite(an) && Number.isFinite(bn)) return (an - bn) * factor
      return String(av ?? '').localeCompare(String(bv ?? '')) * factor
    })
  }, [data, columns, sort])

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

  return (
    <div className="ui-table-wrap" data-testid="table">
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
                  style={{ textAlign: c.align, width: c.width }}
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
                    style={{ textAlign: c.align }}
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
