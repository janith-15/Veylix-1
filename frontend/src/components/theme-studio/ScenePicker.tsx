import { useThemeStore } from '../../theme/themeStore'
import {
  SCENES_FOR_PRESET,
  SCENE_BLURBS,
  SCENE_LABELS,
  PRESET_LABELS,
  type SceneType,
  type ThemePreset,
} from '../../theme/themes'
import './scene-picker.css'

/**
 * Background-effect picker.
 *
 * This is the piece the studio was missing: the app shipped sixteen animated
 * scenes but the ONLY way to reach one was to change theme, so eleven of them
 * were unreachable. The rule the owner asked for is implemented literally here:
 *
 *  - the active theme's own scenes are listed FIRST, under its name, so "the
 *    effect belongs to the theme" stays true and the default is obvious;
 *  - every other theme's scenes follow, so a user CAN pick any of them
 *    ("kemathi widihata thoranna");
 *  - `flat` is offered separately as "None", because a customer who is sensitive
 *    to motion should not have to change theme to switch the background off.
 *
 * Picking writes `scene` through the same draft the rest of the studio uses, so
 * Cancel still discards it — the picker has no private state.
 */
export function ScenePicker({ preset }: { preset: ThemePreset }) {
  const { draft, settings: live, draftUpdate } = useThemeStore()
  const settings = draft ?? live
  const active = settings.scene

  const own = SCENES_FOR_PRESET[preset]

  // Every other scene, grouped by the theme that owns it, in preset order.
  const others = (Object.keys(PRESET_LABELS) as ThemePreset[])
    .filter((p) => p !== preset)
    .map((p) => ({
      preset: p,
      scenes: SCENES_FOR_PRESET[p].filter((s) => !own.includes(s)),
    }))
    .filter((g) => g.scenes.length > 0)

  const tile = (scene: SceneType, ownerPreset?: ThemePreset) => {
    const isActive = active === scene
    const label = SCENE_LABELS[scene]
    return (
      <button
        key={scene}
        type="button"
        className={'scene-tile' + (isActive ? ' scene-tile--active' : '')}
        onClick={() => draftUpdate({ scene })}
        aria-pressed={isActive}
        data-scene-id={scene}
        title={SCENE_BLURBS[scene]}
      >
        {/* A live miniature: the same gradient family the scene paints, so a
            tile is recognisable without running sixteen animations at once. */}
        <span className="scene-tile__preview" data-preview={scene} aria-hidden="true" />
        <span className="scene-tile__meta">
          <span className="scene-tile__name">{label}</span>
          <span className="scene-tile__blurb">{SCENE_BLURBS[scene]}</span>
          {ownerPreset && <em className="scene-tile__owner">{PRESET_LABELS[ownerPreset]}</em>}
        </span>
      </button>
    )
  }

  return (
    <div className="scene-picker">
      <div className="scene-picker__group">
        <div className="scene-picker__title">
          {PRESET_LABELS[preset]} backgrounds
          <em>default for this theme</em>
        </div>
        <div className="scene-grid">{own.map((s) => tile(s))}</div>
      </div>

      <div className="scene-picker__group">
        <div className="scene-picker__title">
          Motion
          <em>switch the background off entirely</em>
        </div>
        <div className="scene-grid">{tile('flat')}</div>
      </div>

      {others.map((g) => (
        <div className="scene-picker__group" key={g.preset}>
          <div className="scene-picker__title">{PRESET_LABELS[g.preset]} backgrounds</div>
          <div className="scene-grid">{g.scenes.map((s) => tile(s, g.preset))}</div>
        </div>
      ))}

      <p className="scene-picker__note">
        The scene is saved with the theme. Choosing a sub-theme recolours the
        background you picked here — it never swaps it.
      </p>
    </div>
  )
}
