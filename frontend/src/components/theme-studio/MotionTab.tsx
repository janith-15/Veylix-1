import type { ReactNode } from 'react'
import { useThemeStore } from '../../theme/themeStore'
import type { StyleSettings, MotionMood, MountAnim, HoverAnim, ButtonHover } from '../../theme/themes'

const MOODS: MotionMood[] = ['none', 'calm', 'snappy', 'playful']
const MOUNTS: MountAnim[] = ['none', 'fade', 'rise', 'slide']
const HOVERS: HoverAnim[] = ['none', 'lift', 'tilt', 'glow']
const BTN_HOVERS: ButtonHover[] = ['none', 'squash', 'ripple', 'glow']

function Seg<T extends string>({ options, value, onPick }: { options: readonly T[]; value: T; onPick: (v: T) => void }) {
  return (
    <div className="studio-seg">
      {options.map((o) => (
        <button
          key={o}
          type="button"
          className={'studio-seg__btn ' + (value === o ? 'studio-seg__btn--active' : '')}
          onClick={() => onPick(o)}
        >
          {o}
        </button>
      ))}
    </div>
  )
}

export function MotionTab({ Section, SwitchCtl }: {
  Section: (p: { title: string; children: ReactNode }) => ReactNode
  SwitchCtl: (p: { label: string; checked: boolean; onChange: (v: boolean) => void }) => ReactNode
}) {
  const { draft, settings: live, draftUpdate } = useThemeStore()
  const settings = draft ?? live
  const updateSettings = draftUpdate
  const style = settings.style
  const patch = (p: Partial<StyleSettings>) => updateSettings({ style: { ...style, ...p } })

  return (
    <>
      <Section title="Motion style">
        <Seg options={MOODS} value={style.mood} onPick={(mood) => patch({ mood })} />
        <div className="studio-slider">
          <div className="studio-slider__head">
            <span>Speed</span>
            <span>{style.speed.toFixed(1)}x</span>
          </div>
          <input type="range" min={0.5} max={2} step={0.1} value={style.speed} onChange={(e) => patch({ speed: Number(e.target.value) })} />
        </div>
      </Section>

      <Section title="Entrances">
        <Seg options={MOUNTS} value={style.mount} onPick={(mount) => patch({ mount })} />
        <div className="flow-top">
          <SwitchCtl label="Stagger cards" checked={style.stagger} onChange={(stagger) => patch({ stagger })} />
        </div>
      </Section>

      <Section title="Hover">
        <div className="studio__section-title">Card</div>
        <Seg options={HOVERS} value={style.hover} onPick={(hover) => patch({ hover })} />
        <div className="studio__section-title flow-top-3">Button</div>
        <Seg options={BTN_HOVERS} value={style.buttonHover} onPick={(buttonHover) => patch({ buttonHover })} />
      </Section>

      <Section title="Background and accessibility">
        <SwitchCtl label="Background drift" checked={style.bgDrift} onChange={(bgDrift) => patch({ bgDrift })} />
        <SwitchCtl label="Reduce motion" checked={settings.reducedMotion} onChange={(reducedMotion) => updateSettings({ reducedMotion })} />
        <SwitchCtl label="Low-resource mode" checked={settings.lowResourceMode} onChange={(lowResourceMode) => updateSettings({ lowResourceMode })} />
      </Section>
    </>
  )
}

