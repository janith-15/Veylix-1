import { useRef, type KeyboardEvent as ReactKeyboardEvent, type ReactNode } from 'react'

export interface TabItem {
  value: string
  label: ReactNode
}

export function Tabs({
  items,
  value,
  onChange,
}: {
  items: TabItem[]
  value: string
  onChange: (value: string) => void
}) {
  const refs = useRef<(HTMLButtonElement | null)[]>([])

  /* APG tablist: roving tabindex + arrow-key navigation. */
  const onKeyDown = (e: ReactKeyboardEvent) => {
    const idx = items.findIndex((t) => t.value === value)
    let next = idx
    if (e.key === 'ArrowRight') next = (idx + 1) % items.length
    else if (e.key === 'ArrowLeft') next = (idx - 1 + items.length) % items.length
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = items.length - 1
    else return
    e.preventDefault()
    onChange(items[next].value)
    refs.current[next]?.focus()
  }

  return (
    <div className="ui-tabs" role="tablist" onKeyDown={onKeyDown}>
      {items.map((t, i) => (
        <button
          key={t.value}
          ref={(el) => {
            refs.current[i] = el
          }}
          role="tab"
          tabIndex={value === t.value ? 0 : -1}
          aria-selected={value === t.value}
          className={`ui-tabs__item ${value === t.value ? 'ui-tabs__item--active' : ''}`}
          onClick={() => onChange(t.value)}
        >
          {t.label}
        </button>
      ))}
    </div>
  )
}
