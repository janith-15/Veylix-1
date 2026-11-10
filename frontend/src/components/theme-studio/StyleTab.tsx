import type { ReactNode } from 'react'
import { useThemeStore } from '../../theme/themeStore'
import { Button } from '../ui'
import type {
  CardBg,
  StyleSettings,
  FontPreset,
} from '../../theme/themes'

const CARD_BGS: CardBg[] = ['none', 'gradient', 'noise', 'grid', 'scanlines']
const FONTS: FontPreset[] = ['default', 'system', 'tidy', 'mono', 'serif']
const WEIGHTS = [400, 500, 600, 700, 800]

/** Small labelled slider used across the Style tab. */
function Slider({
  label,
  value,
  min,
  max,
  step = 1,
  suffix = '',
  onChange,
}: {
  label: string
  value: number
  min: number
  max: number
  step?: number
  suffix?: string
  onChange: (v: number) => void
}) {
  return (
    <div className="studio-slider">
      <div className="studio-slider__head">
        <span>{label}</span>
        <span>{value}{suffix}</span>
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
      />
    </div>
  )
}

/** Segmented control. */
function Seg<T extends string>({
  options,
  value,
  onPick,
}: {
  options: readonly T[]
  value: T
  onPick: (v: T) => void
}) {
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

export function StyleTab({ Section }: { Section: (p: { title: string; children: ReactNode }) => ReactNode }) {
  const { draft, settings: live, draftUpdate } = useThemeStore()
  const settings = draft ?? live
  const updateSettings = draftUpdate
  const style = settings.style
  const patch = (p: Partial<StyleSettings>) =>
    updateSettings({ style: { ...style, ...p } })

  return (
    <>
      <Section title="Card background">
        <Seg options={CARD_BGS} value={style.cardBg} onPick={(cardBg) => patch({ cardBg })} />
        {style.cardBg === 'gradient' && (
          <>
            <Slider label="Angle" value={style.cardBgAngle} min={0} max={360} suffix=" deg" onChange={(cardBgAngle) => patch({ cardBgAngle })} />
            <Slider label="Intensity" value={style.cardBgIntensity} min={0} max={100} suffix="%" onChange={(cardBgIntensity) => patch({ cardBgIntensity })} />
          </>
        )}
        {(style.cardBg === 'noise' || style.cardBg === 'grid' || style.cardBg === 'scanlines') && (
          <Slider label="Texture" value={Math.round(style.textureOpacity * 100)} min={0} max={30} suffix="%" onChange={(v) => patch({ textureOpacity: v / 100 })} />
        )}
      </Section>

      <Section title="Typography">
        <div className="studio__section-title">Font family</div>
        <Seg options={FONTS} value={settings.fontPreset} onPick={(fontPreset) => updateSettings({ fontPreset })} />

        <div className="studio__section-title flow-top-3">Headings</div>
        <Seg
          options={['none', 'uppercase'] as const}
          value={style.headingTransform}
          onPick={(headingTransform) => patch({ headingTransform })}
        />
        <div style={{ height: 8 }} />
        <Seg
          options={['body', 'mono', 'serif'] as const}
          value={style.headingFont}
          onPick={(headingFont) => patch({ headingFont })}
        />
        <div style={{ height: 8 }} />
        <Seg
          options={WEIGHTS.map(String) as unknown as readonly string[]}
          value={String(style.headingWeight)}
          onPick={(w) => patch({ headingWeight: Number(w) })}
        />

        <div className="studio__section-title flow-top-3">Labels</div>
        <Seg
          options={['none', 'uppercase'] as const}
          value={style.labelTransform}
          onPick={(labelTransform) => patch({ labelTransform })}
        />

        <Slider label="Font size" value={settings.fontSize} min={0.9} max={1.2} step={0.05} suffix="x" onChange={(fontSize) => updateSettings({ fontSize })} />
        <Slider label="Line height" value={settings.lineHeight} min={1.2} max={1.9} step={0.05} onChange={(lineHeight) => updateSettings({ lineHeight })} />
        <Slider label="Heading spacing" value={style.headingTracking} min={-0.05} max={0.2} step={0.01} suffix="em" onChange={(headingTracking) => patch({ headingTracking })} />

        <div className="style-preview">
          <div className="style-preview__h">Overview</div>
          <div className="style-preview__p">Aa සිංහල 0123 - The quick brown fox</div>
        </div>
      </Section>

      <Section title="Spacing and shape">
        <Slider label="Corner radius" value={settings.radius} min={0} max={24} suffix="px" onChange={(radius) => updateSettings({ radius })} />
        <Slider label="Border weight" value={settings.borderWidth} min={1} max={3} suffix="px" onChange={(borderWidth) => updateSettings({ borderWidth })} />
        <Slider label="Elevation" value={settings.shadowLevel} min={0} max={3} onChange={(shadowLevel) => updateSettings({ shadowLevel })} />

        <div className="ui-row flow-top">
          <Button size="sm" variant="outline" onClick={() => updateSettings({ style: { ...style } })}>Re-apply</Button>
        </div>
      </Section>
    </>
  )
}

