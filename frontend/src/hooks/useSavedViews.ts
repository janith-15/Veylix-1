import { useCallback, useMemo, useState } from 'react'

/**
 * Named filter sets for one list screen, kept in `localStorage` (Phase-6 UX, 4.4).
 *
 * Why local and not a server table: a saved view is one person's shortcut to a
 * filtered list, it contains no shared truth, and it must survive a reload —
 * all three are `localStorage`. A server table would need a migration, an owner
 * column, a scope rule and a sync story to deliver the same thing.
 *
 * Why a hook and not a component: each list screen already owns its URL-backed
 * filters through `useListQuery`; this only records the string form of those
 * filters and replays them. It never interprets them, so a screen cannot save a
 * view that its own URL logic would not accept.
 *
 * Storage is namespaced per screen (`veylix.views.<screen>`) so two lists cannot
 * overwrite each other, and a corrupt entry is dropped rather than crashing the
 * page it belongs to.
 */
export interface SavedView {
  /** The name the operator typed. */
  name: string
  /** The URL params as they were when saved, e.g. `tag=untagged&usage=exhausted`. */
  query: string
  /** Epoch ms, so the newest names can be ordered predictably. */
  savedAt: number
}

const MAX_VIEWS = 8

function storageKey(screen: string): string {
  return `veylix.views.${screen}`
}

function readViews(screen: string): SavedView[] {
  try {
    const raw = window.localStorage.getItem(storageKey(screen))
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    // Validate rather than trust: localStorage is user-editable, and a malformed
    // entry must not blank a screen the operator is trying to use.
    return parsed
      .filter(
        (v): v is SavedView =>
          typeof v === 'object' &&
          v !== null &&
          typeof (v as SavedView).name === 'string' &&
          typeof (v as SavedView).query === 'string',
      )
      .map((v) => ({ name: v.name, query: v.query, savedAt: Number(v.savedAt) || 0 }))
  } catch {
    return []
  }
}

export interface SavedViews {
  views: SavedView[]
  /** Saves the CURRENT query string under a name. Returns false when it is refused. */
  save: (name: string, query: string) => boolean
  remove: (name: string) => void
}

/**
 * The query string is passed to `save` by the caller, not held here: the screen
 * already owns it through `useSearchParams`, and a second copy inside the hook
 * could disagree with the address bar during a debounce.
 */
export function useSavedViews(screen: string): SavedViews {
  /*
   * Initialised from storage DURING RENDER, not in an effect.
   *
   * `localStorage` is synchronously readable, so an effect would only add a second
   * render — and `react(set-state-in-effect)` is right to refuse it. Keying the
   * initialiser on `screen` would not re-read on a screen change, so the key is
   * applied by remounting the consumer (each list screen owns one instance).
   */
  const [views, setViews] = useState<SavedView[]>(() => readViews(screen))

  const persist = useCallback(
    (next: SavedView[]) => {
      setViews(next)
      try {
        window.localStorage.setItem(storageKey(screen), JSON.stringify(next))
      } catch {
        // Quota or a private window: the in-memory list still works for this
        // session, which is better than an error nobody can act on.
      }
    },
    [screen],
  )

  const save = useCallback(
    (name: string, current: string) => {
      const trimmed = name.trim()
      if (!trimmed) return false
      // An empty query is "all rows", which is the default view — saving it would
      // create a shortcut to the page the operator is already on.
      if (!current) return false
      const entry: SavedView = { name: trimmed, query: current, savedAt: Date.now() }
      // Same name replaces: a "view" is a slot, not an append-only log, and two
      // entries with one name would make the list answer differently on click.
      const without = readViews(screen).filter((v) => v.name !== trimmed)
      persist([entry, ...without].slice(0, MAX_VIEWS))
      return true
    },
    [screen, persist],
  )

  const remove = useCallback(
    (name: string) => {
      persist(readViews(screen).filter((v) => v.name !== name))
    },
    [screen, persist],
  )

  return useMemo(() => ({ views, save, remove }), [views, save, remove])
}
