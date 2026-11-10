import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'

/**
 * List state that lives in the URL.
 *
 * Why the URL and not `useState`: a filtered list is a thing a person wants to
 * keep, share and step back through. With local state a refresh drops every
 * filter, a link to "blocked clients with a free claim" cannot be sent to a
 * colleague, and the browser Back button leaves the page instead of undoing the
 * last filter. Eight list screens had this problem identically.
 *
 * Design decisions:
 *   - **Only non-default values reach the URL**, so `/clients` stays clean instead
 *     of growing `?status=&q=&page=1&sort=`.
 *   - **Text is debounced** (default 300 ms). A search box that fires per keystroke
 *     is one SQL query per character, and the target host is one small vCPU.
 *   - **The URL is the source of truth**, so Back/Forward and a pasted link work.
 *   - **Types come from the defaults**, because `"false"` must not become truthy.
 */
export type QueryValue = string | number | boolean | null | undefined

export interface ListQueryOptions<T extends Record<string, QueryValue>> {
  defaults: T
  /** Fields to debounce in the UI (text inputs). */
  debounced?: (keyof T)[]
  debounceMs?: number
}

export interface ListQuery<T extends Record<string, QueryValue>> {
  /** The committed filters, as they should be sent to the server. */
  values: T
  /** Live values: text updates immediately, `values` lags by the debounce. */
  draft: T
  /** Update fields. Debounced ones wait for a pause; the rest apply at once. */
  set: (patch: Partial<T>) => void
  /** Update immediately, skipping the debounce (Enter, blur, a select). */
  setNow: (patch: Partial<T>) => void
  /** Reset everything to the defaults and clean the URL. */
  reset: () => void
  /** True when any value differs from its default. */
  isFiltered: boolean
}

/** Coerce a URL string back to the type the default implies. */
function parse(raw: string, fallback: QueryValue): QueryValue {
  if (typeof fallback === 'number') {
    const n = Number(raw)
    return Number.isFinite(n) ? n : fallback
  }
  if (typeof fallback === 'boolean') return raw === 'true'
  return raw
}

/** Serialise a value, dropping empties so they never reach the URL. */
function serialise(v: QueryValue): string | null {
  if (v === undefined || v === null || v === '') return null
  return String(v)
}

export function useListQuery<T extends Record<string, QueryValue>>({
  defaults,
  debounced = [],
  debounceMs = 300,
}: ListQueryOptions<T>): ListQuery<T> {
  const [params, setParams] = useSearchParams()

  // Read the URL; this is what Back/Forward and a shared link give us.
  //
  // The dependency is a STRING, not the objects: `useSearchParams` returns a new
  // object every render, and callers pass `defaults` as a literal, so keying on
  // either rebuilt `values` on every render. A caller's `useEffect([values])` then
  // fired forever — that produced 653 identical requests in a single paint.
  const paramsKey = params.toString()
  const defaultsKey = JSON.stringify(defaults)
  const values = useMemo(() => {
    const out = { ...defaults }
    const current = new URLSearchParams(paramsKey)
    for (const key of Object.keys(defaults) as (keyof T)[]) {
      const raw = current.get(String(key))
      if (raw !== null) out[key] = parse(raw, defaults[key]) as T[keyof T]
    }
    return out
    // eslint-disable-next-line react-hooks/exhaustive-deps -- keys above cover both
  }, [paramsKey, defaultsKey])

  const commit = useCallback(
    (patch: Partial<T>, { replace = true }: { replace?: boolean } = {}) => {
      const next = new URLSearchParams(params)
      for (const [k, v] of Object.entries(patch)) {
        const asValue = v as QueryValue
        const isDefault = String(asValue ?? '') === String(defaults[k as keyof T] ?? '')
        const encoded = serialise(asValue)
        if (encoded === null || isDefault) next.delete(k)
        else next.set(k, encoded)
      }
      setParams(next, { replace })
    },
    [params, setParams, defaults],
  )

  // Text fields show the typed value immediately; the URL (and so the request)
  // follows once the typing pauses.
  const [draft, setDraft] = useState<T>(values)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  // The params string the draft was last built from. When the URL changes for a
  // reason we did not initiate (Back/Forward, a shared link, reset), the draft is
  // rebuilt during render instead of in an effect — an effect would render once
  // with the stale draft, then again, which is the cascade the lint rule flags.
  const [syncedFrom, setSyncedFrom] = useState(params.toString())
  if (syncedFrom !== params.toString()) {
    setSyncedFrom(params.toString())
    setDraft(values)
  }

  const set = useCallback(
    (patch: Partial<T>) => {
      setDraft((d) => ({ ...d, ...patch }))
      const needsDebounce = Object.keys(patch).some((k) =>
        (debounced as string[]).includes(k),
      )
      if (!needsDebounce) {
        commit(patch)
        return
      }
      if (timer.current) clearTimeout(timer.current)
      timer.current = setTimeout(() => {
        // Reset to page 1 when a filter changes: staying on page 7 of the old
        // result set is how a freshly filtered list looks empty for no reason.
        const withPage = 'page' in defaults ? ({ ...patch, page: 1 } as Partial<T>) : patch
        commit(withPage)
      }, debounceMs)
    },
    [commit, debounced, debounceMs, defaults],
  )

  const setNow = useCallback(
    (patch: Partial<T>) => {
      if (timer.current) clearTimeout(timer.current)
      setDraft((d) => ({ ...d, ...patch }))
      commit(patch)
    },
    [commit],
  )

  const reset = useCallback(() => {
    if (timer.current) clearTimeout(timer.current)
    setDraft(defaults)
    setParams(new URLSearchParams(), { replace: true })
  }, [defaults, setParams])

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current)
    },
    [],
  )

  const isFiltered = useMemo(
    () =>
      (Object.keys(defaults) as (keyof T)[]).some(
        (k) => String(values[k] ?? '') !== String(defaults[k] ?? ''),
      ),
    [values, defaults],
  )

  return { values, draft, set, setNow, reset, isFiltered }
}
