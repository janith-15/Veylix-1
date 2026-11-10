import { useState } from 'react'
import { Button, Dialog, Input } from '../ui'
import { Bookmark, BookmarkPlus, Trash2 } from 'lucide-react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useSavedViews } from '../../hooks/useSavedViews'

/**
 * Saved views for one list screen (Phase-6 UX, 4.4).
 *
 * The gap this closes: filters already live in the URL, so a link could be shared
 * — but there was nowhere to KEEP one. An operator who works the same queue every
 * morning (untagged, or expiring this week) re-built the filter from four
 * dropdowns each day.
 *
 * The control only appears when it can do something: saving needs a non-empty
 * filter, and a screen with no saved views shows a single "Save this view" button
 * rather than an empty menu.
 */
export function SavedViewsBar({ screen }: { screen: string }) {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const current = params.toString()
  const { views, save, remove } = useSavedViews(screen)
  const [naming, setNaming] = useState(false)
  const [draft, setDraft] = useState('')
  const [error, setError] = useState('')

  const commit = () => {
    const name = draft.trim()
    if (!name) {
      setError('Give the view a name.')
      return
    }
    if (!save(name, current)) {
      setError('Filter the list first — an unfiltered view is the default page.')
      return
    }
    setNaming(false)
    setDraft('')
    setError('')
  }

  return (
    <div className="saved-views" role="group" aria-label="Saved views">
      {views.length > 0 && (
        <span className="saved-views__list">
          {views.map((v) => (
            <span key={v.name} className="saved-views__chip">
              <button
                type="button"
                className="saved-views__open"
                title={v.query}
                onClick={() => navigate(`?${v.query}`)}
              >
                <Bookmark size={12} aria-hidden="true" /> {v.name}
              </button>
              <button
                type="button"
                className="saved-views__del"
                aria-label={`Delete the view ${v.name}`}
                title="Delete this view"
                onClick={() => remove(v.name)}
              >
                <Trash2 size={11} />
              </button>
            </span>
          ))}
        </span>
      )}

      <Button
        size="sm"
        variant="ghost"
        leading={<BookmarkPlus size={13} />}
        disabled={!current}
        title={
          current
            ? 'Keep these filters for next time'
            : 'Filter the list first — there is nothing to save yet'
        }
        onClick={() => {
          setNaming(true)
          setError('')
        }}
      >
        Save view
      </Button>

      <Dialog
        open={naming}
        onClose={() => setNaming(false)}
        title="Save this view"
        footer={
          <>
            <Button variant="ghost" onClick={() => setNaming(false)}>
              Cancel
            </Button>
            <Button onClick={commit}>Save</Button>
          </>
        }
      >
        <div className="ui-col ui-saved-views__form">
          <Input
            label="Name"
            placeholder="Untagged queue, Expiring this week…"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
          />
          <p className="ui-muted t-sm2 ui-saved-views__note">
            Saved to this browser, not your account. Filters: <code>{current}</code>
          </p>
          {error && <p className="ui-field__error" role="alert">{error}</p>}
        </div>
      </Dialog>
    </div>
  )
}
