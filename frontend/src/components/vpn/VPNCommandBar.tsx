import type { ApiAdminVPNSummary } from '../../lib/api'

/**
 * The page's URL-backed filter values, in the STRING form `useListQuery` uses.
 *
 * Declared here rather than importing the hook's generic so the strip stays a
 * dumb presentational component: it reports what was clicked and lets the page
 * decide what that means for its URL.
 */
export interface VPNCommandValues {
  tag?: string | null
  assigned?: string | null
  usage?: string | null
  expiry?: string | null
}

/** The filter patch a tile applies. Strings and `page`, matching `useListQuery`. */
export interface VPNCommandPatch {
  tag?: string
  assigned?: string
  usage?: string
  expiry?: string
  page?: number
}

/**
 * The catalogue at a glance, as one row of one-click filters (P2).
 *
 * Why this exists: the backend has always known how many VPNs are untagged,
 * expiring or exhausted, but the console made the operator *find out* by opening
 * a filter dropdown, reading a row count, and remembering the number while
 * choosing the next dropdown. The most routine question a Super Admin asks —
 * "what needs me today?" — cost four interactions and a memory.
 *
 * Two rules shape it:
 *   1. every tile is a FILTER, not decoration: clicking it applies exactly the
 *      filter whose count it shows, so the number and the list can never
 *      disagree;
 *   2. a tile with a zero count is not clickable, because a control that
 *      provably returns nothing is a dead end dressed as an action.
 */
type Tile = {
  /** Stable key for tests and for the active-state comparison. */
  key: string
  label: string
  count: number
  /** The query this tile applies. `null` means "never clickable". */
  to: VPNCommandPatch | null
  /** Draws attention when it is the thing that needs doing. */
  tone?: 'attention' | 'danger'
  /** Tooltip: why this number matters. */
  hint: string
}

/**
 * Builds the tiles from the server's summary, in the order an operator triages.
 *
 * Module-private on purpose: exporting a plain function beside a component breaks
 * React Fast Refresh for this file (oxlint `only-export-components`), and the
 * tiles are derived state, not an API this module owes anyone.
 */
function summariseTiles(s: ApiAdminVPNSummary): Tile[] {
  return [
    {
      key: 'untagged',
      label: 'Untagged',
      count: s.untagged,
      to: { tag: 'untagged' },
      tone: 'attention',
      hint: 'Panel clients waiting for a FREE or PREMIUM decision. Only you can see them.',
    },
    {
      key: 'expiring',
      label: 'Expiring ≤7d',
      count: s.expiring_soon,
      to: { expiry: '7d' },
      tone: 'attention',
      hint: 'Still working, but running out this week — the renewal conversation.',
    },
    {
      key: 'exhausted',
      label: 'Out of data',
      count: s.exhausted,
      to: { usage: 'exhausted' },
      tone: 'danger',
      hint: 'Traffic limit reached. The customer cannot connect until you act.',
    },
    {
      key: 'expired',
      label: 'Expired',
      count: s.expired,
      to: { expiry: 'expired' },
      tone: 'danger',
      hint: 'Past their expiry date.',
    },
    {
      key: 'unassigned',
      label: 'Unassigned',
      count: s.unassigned,
      to: { assigned: '0' },
      hint: 'Nobody holds these yet — stock on the shelf.',
    },
    {
      key: 'unread',
      label: 'Unreadable',
      count: s.unread,
      to: null,
      tone: 'danger',
      hint: 'The panel did not answer for these, so their usage is unknown rather than zero.',
    },
  ]
}

/** Which tile is currently applied, so the strip can show where you are. */
function activeKey(values: VPNCommandValues): string | null {
  if (values.tag === 'untagged') return 'untagged'
  if (values.expiry === '7d') return 'expiring'
  if (values.expiry === 'expired') return 'expired'
  if (values.usage === 'exhausted') return 'exhausted'
  if (values.assigned === '0') return 'unassigned'
  return null
}

export function VPNCommandBar({
  summary,
  values,
  onPick,
  onShowAll,
}: {
  summary: ApiAdminVPNSummary | null
  values: VPNCommandValues
  /** Applies a tile's filter. Always resets the page: page 3 of the old filter may not exist. */
  onPick: (patch: VPNCommandPatch) => void
  onShowAll: () => void
}) {
  /*
   * Nothing is rendered without a summary. A strip of six zeroes would assert
   * "your platform is empty", which is a different claim from "this server did
   * not send counts" — and the second one is the truth when `summary` is absent.
   */
  if (!summary) return null

  const active = activeKey(values)
  const tiles = summariseTiles(summary)

  return (
    <div className="vpn-cmdbar" role="group" aria-label="VPN catalogue at a glance">
      <button
        type="button"
        className={`vpn-cmdbar__cell vpn-cmdbar__cell--total${active === null ? ' is-active' : ''}`}
        onClick={onShowAll}
        aria-pressed={active === null}
        title="Every VPN the system knows about"
      >
        <span className="vpn-cmdbar__count">{summary.total}</span>
        <span className="vpn-cmdbar__label">All VPNs</span>
      </button>

      {tiles.map((t) => {
        const clickable = t.count > 0 && t.to !== null
        const isActive = active === t.key
        return (
          <button
            key={t.key}
            type="button"
            className={[
              'vpn-cmdbar__cell',
              t.tone ? `vpn-cmdbar__cell--${t.tone}` : '',
              isActive ? 'is-active' : '',
              !clickable ? 'is-empty' : '',
            ]
              .filter(Boolean)
              .join(' ')}
            /* A zero tile is inert: it would filter to an empty list. */
            disabled={!clickable}
            onClick={() => {
              if (!t.to) return
              onPick({ ...t.to, page: 1 })
            }}
            aria-pressed={isActive}
            title={t.hint}
          >
            <span className="vpn-cmdbar__count">{t.count}</span>
            <span className="vpn-cmdbar__label">{t.label}</span>
          </button>
        )
      })}

      {/* The split is a headline in its own right: "12 premium / 2 free". */}
      <span className="vpn-cmdbar__split" aria-label="Free and premium split">
        <strong>{summary.premium}</strong> premium · <strong>{summary.free}</strong> free
        {summary.disabled > 0 && (
          <>
            {' · '}
            <span className="vpn-cmdbar__warn">{summary.disabled} switched off on the panel</span>
          </>
        )}
      </span>
    </div>
  )
}
