import { useState } from 'react'
import { useThemeStore } from '../../theme/themeStore'
import { ALL_SUB_THEMES, SUB_THEMES, resolveVariant } from '../../theme/subthemes'
import { Input } from '../ui'
import type { ThemePreset } from '../../theme/themes'
import './subthemes-ui.css'

/**
 * Sub-theme picker.
 *
 * Renders the active theme variants as tiles with a live colour preview.
 * With a search term it widens to EVERY preset: each theme now ships ten
 * identities, so finding one specific look meant clicking through six themes
 * and reading sixty tiles. An empty query behaves exactly as before - the
 * current preset only - so nothing moves for someone happy browsing.
 */
export function SubThemePicker({ preset }: { preset: ThemePreset }) {
  const { draft, settings: live, draftSetVariant } = useThemeStore()
  const settings = draft ?? live
  const own = SUB_THEMES[preset]
  const activeVariant = resolveVariant(preset, settings.variant)
  const [query, setQuery] = useState('')
  const q = query.trim().toLowerCase()

  const variants = q
    ? ALL_SUB_THEMES.filter(
        (v) =>
          v.name.toLowerCase().includes(q) ||
          v.blurb.toLowerCase().includes(q) ||
          v.id.includes(q) ||
          v.preset.includes(q),
      )
    : own

  return (
    <div className="ui-col" style={{ gap: 'var(--space-2)' }}>
      <Input
        label="Find a sub-theme"
        placeholder="Name, mood or theme..."
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        hint={
          q
            ? variants.length + ' across all themes'
            : own.length + ' in ' + preset + ' - search all ' + ALL_SUB_THEMES.length
        }
      />
      {variants.length === 0 && (
        <p className="ui-muted t-sm2" role="status">
          No sub-theme matches that search.
        </p>
      )}
      <div className="sub-grid">
    <div className="sub-grid">
      {variants.map((v) => {
        const active = activeVariant?.id === v.id
        const primary = v.vars['--c-primary'] ?? 'var(--c-primary)'
        const secondary = v.vars['--c-secondary'] ?? 'var(--c-secondary)'
        const page = v.vars['--bg-page'] ?? 'var(--bg-page)'
        const card = v.vars['--bg-card'] ?? 'var(--bg-card)'
        const radius = v.vars['--radius'] ?? 'var(--radius)'

        return (
          <button
            key={v.id}
            type="button"
            className={'sub-tile' + (active ? ' sub-tile--active' : '')}
            onClick={() => draftSetVariant(v.id)}
            aria-pressed={active}
          >
            {/* Miniature of the real interface, painted with the variant palette */}
            <span className="sub-tile__preview" style={{ background: page }}>
              <span className="sub-tile__bar" style={{ background: card, borderRadius: radius }}>
                <span className="sub-tile__dot" style={{ background: primary }} />
                <span className="sub-tile__line" style={{ background: secondary }} />
              </span>
              <span className="sub-tile__card" style={{ background: card, borderRadius: radius, borderColor: primary }}>
                <span className="sub-tile__btn" style={{ background: primary, borderRadius: radius }} />
              </span>
            </span>

            <span className="sub-tile__meta">
              <span className="sub-tile__name">{v.name}</span>
              <span className="sub-tile__blurb">{v.blurb}</span>
              <span className="sub-tile__tags">
                <em>{v.uiVariant}</em>
                <em>{v.scene}</em>
              </span>
            </span>
          </button>
        )
      })}
    </div>
      </div>
    </div>
  )
}
