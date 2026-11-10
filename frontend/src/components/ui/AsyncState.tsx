import type { ReactNode } from 'react'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { Button } from './Button'
import { Skeleton } from './Skeleton'

/**
 * The one way a screen reports "loading", "failed" and "nothing here".
 *
 * Every list screen hand-rolled these three states, so they drifted: some showed a
 * bare sentence while fetching (indistinguishable from "no data"), some printed an
 * error with **no way to try again** — 211 error surfaces across the app and only
 * three retries — and empty states were written 44 different ways.
 *
 * The rules this encodes:
 *   - **Loading looks like the thing being loaded** (a skeleton), not a spinner, and
 *     never like an empty result.
 *   - **A failure always offers a way forward.** If `onRetry` is given, there is a
 *     Try again button; without one the error is still visible, never swallowed.
 *   - **"Nothing here" is different from "failed"**, and can carry the next step.
 *
 * `status` is explicit rather than inferred from data length, because "an empty
 * array" means both "still loading" and "genuinely empty" — and those must never
 * look the same.
 */
export type AsyncStatus = 'loading' | 'error' | 'empty' | 'ready'

export interface AsyncStateProps {
  status: AsyncStatus
  /** Human message for `error`; also used as the fallback empty line. */
  error?: string | null
  onRetry?: () => void
  /** Shown while loading. Defaults to a skeleton block. */
  skeleton?: ReactNode
  /** Shown for `empty`; pass an `EmptyState` for an icon + action. */
  empty?: ReactNode
  children: ReactNode
}

export function AsyncState({
  status,
  error,
  onRetry,
  skeleton,
  empty,
  children,
}: AsyncStateProps) {
  if (status === 'loading') {
    return <>{skeleton ?? <Skeleton height={180} />}</>
  }

  if (status === 'error') {
    return (
      <div className="ui-async ui-async--error" role="alert">
        <AlertTriangle size={18} aria-hidden="true" />
        <div className="ui-async__body">
          <strong>That didn’t load</strong>
          <span className="ui-muted">{error || 'The server did not answer.'}</span>
        </div>
        {onRetry && (
          <Button size="sm" variant="outline" leading={<RefreshCw size={13} />} onClick={onRetry}>
            Try again
          </Button>
        )}
      </div>
    )
  }

  if (status === 'empty') {
    return <>{empty ?? <p className="ui-muted">Nothing here yet.</p>}</>
  }

  return <>{children}</>
}
