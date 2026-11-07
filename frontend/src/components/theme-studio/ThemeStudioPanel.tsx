import { useCallback, useEffect, useState } from 'react'
import { X, RotateCcw, Check, Eye, Maximize2, Minimize2 } from 'lucide-react'
import { ThemePreview } from './ThemePreview'
import { Button, Switch, Badge, useToast } from '../ui'
import { LineChart } from '../charts'
import { VeylixLogo } from '../brand'
import { useThemeStore } from '../../theme/themeStore'
import {
  PRESET_COLORS,
  PRESET_LABELS,
  SCENE_LABELS,
  UI_VARIANT_LABELS,
  type ThemePreset,
  type EffectLevel,
  type FontPreset,
  type ThemeSettings,
  type ButtonShape,
  type ToastStyle,
  type ToastPosition,
  type ChartPalette,
  type ChartStyle,
  type ComponentSettings,
} from '../../theme/themes'
import { StyleTab } from './StyleTab'
import './studio.css'
import { MotionTab } from './MotionTab'
import { SubThemePicker } from './SubThemePicker'
import { ScenePicker } from './ScenePicker'

const PRESETS: ThemePreset[] = ['modern', 'dark', 'light', 'classic', 'gaming', 'hacking']
const EFFECTS: EffectLevel[] = ['off', 'subtle', 'enhanced']
const FONTS: FontPreset[] = ['default', 'system', 'tidy']

const BUTTON_SHAPES: ButtonShape[] = ['soft', 'square', 'pill', 'clipped']
const TOAST_STYLES: ToastStyle[] = ['bar', 'line', 'badge']
const TOAST_POSITIONS: ToastPosition[] = ['tr', 'tl', 'br', 'bl']
const BADGE_SHAPES: Array<'pill' | 'square' | 'tag'> = ['pill', 'square', 'tag']
const CHART_PALETTES: ChartPalette[] = ['theme', 'primary', 'secondary']
const CHART_STYLES: ChartStyle[] = ['area', 'smooth', 'sharp']

const COLOR_FIELDS: Array<{ key: keyof ThemeSettings; label: string }> = [
  { key: 'primary', label: 'Primary / accent' },
  { key: 'secondary', label: 'Secondary accent' },
  { key: 'bgPage', label: 'Page background' },
  { key: 'bgCard', label: 'Card / surface' },
  { key: 'border', label: 'Border' },
  { key: 'success', label: 'Success' },
  { key: 'warning', label: 'Warning' },
  { key: 'error', label: 'Error' },
]

const HEX_RE = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i

function ColorField({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (v: string) => void
}) {
  const safe = HEX_RE.test(value) ? value : '#000000'
  return (
    <div className="studio-color">
      <span className="studio-color__label">{label}</span>
      <div className="studio-color__row">
        <input
          type="color"
          className="studio-color__swatch"
          value={safe}
          aria-label={label}
          onChange={(e) => onChange(e.target.value)}
        />
        <input
          className="studio-color__hex"
          value={value}
          aria-label={`${label} hex value`}
          onChange={(e) => {
            const v = e.target.value
            // Only valid hex values are ever stored — no arbitrary CSS injection
            if (v === '' || HEX_RE.test(v)) onChange(v)
          }}
          maxLength={7}
        />
      </div>
    </div>
  )
}

// oxlint-disable-next-line react/only-export-components -- mini-layout helpers tightly coupled to the studio panel
export function StudioSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="studio__section">
      <div className="studio__section-title">{title}</div>
      {children}
    </div>
  )
}

// oxlint-disable-next-line react/only-export-components -- mini-layout helpers tightly coupled to the studio panel
export function StudioSwitch({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="studio-toggle">
      <div className="studio-toggle__text">{label}</div>
      <Switch checked={checked} onChange={onChange} label={label} />
    </div>
  )
}

export function ThemeStudioPanel({ onClose }: { onClose: () => void }) {
  const {
    settings: liveSettings,
    draft,
    isLocked,
    beginDraft,
    draftSetPreset,
    draftUpdate,
    saveTheme,
    discardDraft,
    resetTheme,
    hasUnsavedChanges,
    studioView,
    setStudioView,
  } = useThemeStore()

  /* Every control edits the DRAFT — the live theme changes only on Save. */
  const settings = draft ?? liveSettings
  const [previewOpen, setPreviewOpen] = useState(false)
  const dirty = hasUnsavedChanges()
  const toast = useToast()

  useEffect(() => {
    beginDraft()
  }, [beginDraft])
  const components = settings.components
  const [tab, setTab] = useState<'appearance' | 'components' | 'style' | 'effects' | 'motion' | 'type'>('appearance')
  /**
   * Closing the panel with unsaved edits used to throw them away silently: the
   * X button called `onClose` and the draft vanished with no warning, so a user
   * who had just spent five minutes picking colours lost the lot. A dirty draft
   * now asks first. `confirm` is the browser dialog on purpose: this is a
   * destructive abandon, and it must be impossible to do by reflex.
   */
  const requestClose = useCallback(() => {
    if (!dirty) {
      onClose()
      return
    }
    const abandon = window.confirm(
      'You have unsaved theme changes. Close without saving?\\n\\nChoose Cancel to stay and press Save to keep them.',
    )
    if (abandon) {
      discardDraft()
      onClose()
    }
  }, [dirty, discardDraft, onClose])

  /* Lock the page behind the drawer so the panel is the only scroll context. */
  useEffect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') requestClose()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      document.body.style.overflow = prev
      window.removeEventListener('keydown', onKey)
    }
  }, [onClose, requestClose])

  const patch = (p: Partial<ThemeSettings>) => draftUpdate(p)
  const patchComponents = (p: Partial<ComponentSettings>) =>
    draftUpdate({ components: { ...settings.components, ...p } })

  return (
    <>
      {studioView === 'compact' && (<div className="studio-scrim" onClick={requestClose} aria-hidden="true" />)}
      <aside className={"studio studio--" + studioView} data-view={studioView} role="dialog" aria-modal="true" aria-label="Design Studio">
        <header className="studio__head">
          <span className="studio__title">Design Studio</span>
          {/* One studio, two views: the drawer for a quick change, the full
              page for advanced work. The choice is persisted so a user who
              works full-page is not dropped back into the drawer each time. */}
          <button
            type="button"
            className="studio__expand"
            onClick={() => setStudioView(studioView === 'compact' ? 'full' : 'compact')}
            aria-label={studioView === 'compact' ? 'Expand to full view' : 'Collapse to drawer'}
          >
            {studioView === 'compact' ? <Maximize2 size={15} /> : <Minimize2 size={15} />}
            <span>{studioView === 'compact' ? 'Full view' : 'Compact'}</span>
          </button>
          <button className="ui-dialog__close" aria-label="Close design studio" onClick={requestClose}>
            <X size={18} />
          </button>
        </header>

        <div className="studio__body">
          <div className="studio__section">
            <div className="studio-preview ui-row">
              <VeylixLogo size={34} />
              <div className="ui-col" style={{ gap: 4, flex: 1 }}>
                <span className="ui-strong t-base">Live preview</span>
                <div className="ui-row" style={{ gap: 6 }}>
                  <Badge tone="success">Active</Badge>
                  <Badge tone="warning">Pending</Badge>
                  <Badge tone="info">Info</Badge>
                </div>
              </div>
              <Button size="sm">Action</Button>
            </div>
          </div>

          <div className="studio-seg" role="tablist" style={{ marginBottom: 18 }}>
            {(['appearance', 'components', 'style', 'effects', 'motion', 'type'] as const).map((t) => (
              <button
                key={t}
                role="tab"
                aria-selected={tab === t}
                className={`studio-seg__btn ${tab === t ? 'studio-seg__btn--active' : ''}`}
                onClick={() => setTab(t)}
              >
{t === 'appearance' ? 'Colors' : t === 'components' ? 'Components' : t === 'style' ? 'Style' : t === 'effects' ? 'Effects' : t === 'motion' ? 'Motion' : 'Text'}
              </button>
            ))}
          </div>

          {tab === 'appearance' && (
            <>
              <div className="studio__section">
                <div className="studio__section-title">Theme presets</div>
                <div className="studio-presets">
                  {PRESETS.map((p) => {
                    const c = PRESET_COLORS[p]
                    return (
                      <button
                        key={p}
                        className={`studio-preset ${settings.preset === p ? 'studio-preset--active' : ''}`}
                        onClick={() => draftSetPreset(p)}
                      >
                        <div className="studio-preset__swatch" style={{ background: c.bgPage }}>
                          <span className="studio-preset__dot" style={{ background: c.primary }} />
                          <span className="studio-preset__dot" style={{ background: c.secondary }} />
                          <span
                            className="studio-preset__dot"
                            style={{ background: c.bgCard, border: '1px solid rgba(0,0,0,.2)' }}
                          />
                        </div>
                        <span className="studio-preset__name">{PRESET_LABELS[p]}</span>
                        <span className="studio-preset__meta">
                          {UI_VARIANT_LABELS[c.uiVariant ?? 'soft']} · {SCENE_LABELS[c.scene ?? 'orbit']}
                        </span>
                      </button>
                    )
                  })}
                </div>
              </div>

              <div className="studio__section">
                <div className="studio__section-title">Sub-themes</div>
                <SubThemePicker preset={settings.preset} />
              </div>

              <div className="studio__section">
                <div className="studio__section-title">Sub-theme colours</div>
                <div className="studio-colors">
                  {COLOR_FIELDS.map((f) => (
                    <ColorField
                      key={f.key}
                      label={f.label}
                      value={String(settings[f.key])}
                      onChange={(v) => patch({ [f.key]: v } as Partial<ThemeSettings>)}
                    />
                  ))}
                </div>
              </div>

              <div className="studio__section">
                <div className="studio__section-title">Glass effect</div>
                <div className="studio-toggle">
                  <div>
                    <div className="studio-toggle__text">Frosted glass surfaces</div>
                    <div className="studio-toggle__hint">
                      Blurs content behind cards, sidebar and dialogs
                    </div>
                  </div>
                  <Switch
                    checked={settings.glassEffect}
                    onChange={(v) => patch({ glassEffect: v })}
                    label="Glass effect"
                  />
                </div>
              </div>
            </>
          )}


ï»¿          {tab === 'components' && (
            <>
              <div className="studio__section">
                <div className="studio__section-title">Buttons</div>
                <div className="studio-seg">
                  {BUTTON_SHAPES.map((s) => (
                    <button
                      key={s}
                      className={'studio-seg__btn ' + (components.buttonShape === s ? 'studio-seg__btn--active' : '')}
                      onClick={() => patchComponents({ buttonShape: s })}
                    >
                      {s}
                    </button>
                  ))}
                </div>
                <div className="studio-slider">
                  <div className="studio-slider__head">
                    <span>Corner radius</span>
                    <span>{components.buttonRadius}px</span>
                  </div>
                  <input
                    type="range"
                    min={0}
                    max={20}
                    value={components.buttonRadius}
                    onChange={(e) => patchComponents({ buttonRadius: Number(e.target.value) })}
                  />
                </div>
                <div className="studio-slider">
                  <div className="studio-slider__head">
                    <span>Glow strength</span>
                    <span>{components.buttonGlow}%</span>
                  </div>
                  <input
                    type="range"
                    min={0}
                    max={100}
                    value={components.buttonGlow}
                    onChange={(e) => patchComponents({ buttonGlow: Number(e.target.value) })}
                  />
                </div>
                <div className="studio-toggle">
                  <div className="studio-toggle__text">Uppercase labels</div>
                  <Switch
                    checked={components.buttonUppercase}
                    onChange={(v) => patchComponents({ buttonUppercase: v })}
                    label="Uppercase button labels"
                  />
                </div>
                <div className="ui-row flow-top-3">
                  <Button size="sm">Primary</Button>
                  <Button size="sm" variant="secondary">Secondary</Button>
                  <Button size="sm" variant="danger">Danger</Button>
                </div>
              </div>

              <div className="studio__section">
                <div className="studio__section-title">Notifications</div>
                <div className="studio-seg">
                  {TOAST_STYLES.map((s) => (
                    <button
                      key={s}
                      className={'studio-seg__btn ' + (components.toastStyle === s ? 'studio-seg__btn--active' : '')}
                      onClick={() => patchComponents({ toastStyle: s })}
                    >
                      {s}
                    </button>
                  ))}
                </div>
                <div className="studio-seg" style={{ marginTop: 8 }}>
                  {TOAST_POSITIONS.map((p) => (
                    <button
                      key={p}
                      className={'studio-seg__btn ' + (components.toastPosition === p ? 'studio-seg__btn--active' : '')}
                      onClick={() => patchComponents({ toastPosition: p })}
                    >
                      {p.toUpperCase()}
                    </button>
                  ))}
                </div>
                <div className="studio-slider">
                  <div className="studio-slider__head">
                    <span>Auto-dismiss</span>
                    <span>{components.toastDuration}s</span>
                  </div>
                  <input
                    type="range"
                    min={2}
                    max={12}
                    value={components.toastDuration}
                    onChange={(e) => patchComponents({ toastDuration: Number(e.target.value) })}
                  />
                </div>
              </div>

ï»¿              <div className="studio__section">
                <div className="studio__section-title">Badges</div>
                <div className="studio-seg">
                  {BADGE_SHAPES.map((s) => (
                    <button
                      key={s}
                      className={'studio-seg__btn ' + (components.badgeShape === s ? 'studio-seg__btn--active' : '')}
                      onClick={() => patchComponents({ badgeShape: s })}
                    >
                      {s}
                    </button>
                  ))}
                </div>
                <div className="studio-toggle flow-top">
                  <div className="studio-toggle__text">Status dot</div>
                  <Switch
                    checked={components.badgeDot}
                    onChange={(v) => patchComponents({ badgeDot: v })}
                    label="Show status dot on badges"
                  />
                </div>
                <div className="ui-row flow-top-3">
                  <Badge tone="success" dot={components.badgeDot}>Active</Badge>
                  <Badge tone="warning" dot={components.badgeDot}>Pending</Badge>
                  <Badge tone="info" dot={components.badgeDot}>Info</Badge>
                </div>
              </div>

              <div className="studio__section">
                <div className="studio__section-title">Charts</div>
                <div className="studio-seg">
                  {CHART_PALETTES.map((p) => (
                    <button
                      key={p}
                      className={'studio-seg__btn ' + (components.chartPalette === p ? 'studio-seg__btn--active' : '')}
                      onClick={() => patchComponents({ chartPalette: p })}
                    >
                      {p}
                    </button>
                  ))}
                </div>
                <div className="studio-seg" style={{ marginTop: 8 }}>
                  {CHART_STYLES.map((s) => (
                    <button
                      key={s}
                      className={'studio-seg__btn ' + (components.chartStyle === s ? 'studio-seg__btn--active' : '')}
                      onClick={() => patchComponents({ chartStyle: s })}
                    >
                      {s}
                    </button>
                  ))}
                </div>
                <div className="studio-toggle flow-top">
                  <div className="studio-toggle__text">Grid lines</div>
                  <Switch checked={components.chartGrid} onChange={(v) => patchComponents({ chartGrid: v })} label="Show chart grid" />
                </div>
                <div className="studio-toggle">
                  <div className="studio-toggle__text">Animate charts</div>
                  <Switch checked={components.chartAnimate} onChange={(v) => patchComponents({ chartAnimate: v })} label="Animate charts" />
                </div>
                <div className="studio-chart-preview">
                  <LineChart
                    series={[{ label: 'Traffic', values: [12, 26, 18, 34, 28, 44, 38] }]}
                    height={110}
                    style={components.chartStyle}
                    showGrid={components.chartGrid}
                  />
                </div>
              </div>
            </>
          )}

          {tab === 'effects' && (
            <>
              <div className="studio__section">
                <div className="studio__section-title">Visual effect level</div>
                <div className="studio-seg">
                  {EFFECTS.map((e) => (
                    <button
                      key={e}
                      className={`studio-seg__btn ${settings.effectLevel === e ? 'studio-seg__btn--active' : ''}`}
                      onClick={() => patch({ effectLevel: e })}
                    >
                      {e}
                    </button>
                  ))}
                </div>
                <p className="ui-muted t-sm" style={{ marginTop: 8 }}>
                  {settings.effectLevel === 'off'
                    ? 'No background animation and no WebGL — lowest CPU/GPU use.'
                    : settings.effectLevel === 'subtle'
                      ? 'Lightweight canvas particles — recommended default.'
                      : 'Animated 3D scene (Three.js, lazy-loaded). Disable on slow devices.'}
                </p>
              </div>

              <div className="studio__section">
                <div className="studio__section-title">Background effect</div>
                <ScenePicker preset={settings.preset} />
              </div>

              <div className="studio__section">
                <div className="studio-slider">
                  <div className="studio-slider__head">
                    <span>Corner radius</span>
                    <span>{settings.radius}px</span>
                  </div>
                  <input
                    type="range"
                    min={0}
                    max={24}
                    value={settings.radius}
                    onChange={(e) => patch({ radius: Number(e.target.value) })}
                  />
                </div>
                <div className="studio-slider">
                  <div className="studio-slider__head">
                    <span>Border weight</span>
                    <span>{settings.borderWidth}px</span>
                  </div>
                  <input
                    type="range"
                    min={1}
                    max={3}
                    value={settings.borderWidth}
                    onChange={(e) => patch({ borderWidth: Number(e.target.value) })}
                  />
                </div>
                <div className="studio-slider">
                  <div className="studio-slider__head">
                    <span>Shadow depth</span>
                    <span>{settings.shadowLevel}</span>
                  </div>
                  <input
                    type="range"
                    min={0}
                    max={3}
                    value={settings.shadowLevel}
                    onChange={(e) => patch({ shadowLevel: Number(e.target.value) })}
                  />
                </div>
              </div>

              <div className="studio__section">
                <div className="studio-toggle">
                  <div>
                    <div className="studio-toggle__text">Reduce motion</div>
                    <div className="studio-toggle__hint">Disables all animation</div>
                  </div>
                  <Switch
                    checked={settings.reducedMotion}
                    onChange={(v) => patch({ reducedMotion: v })}
                    label="Reduce motion"
                  />
                </div>
                <div className="studio-toggle">
                  <div>
                    <div className="studio-toggle__text">Low-resource mode</div>
                    <div className="studio-toggle__hint">Turns off glass blur and effects</div>
                  </div>
                  <Switch
                    checked={settings.lowResourceMode}
                    onChange={(v) => patch({ lowResourceMode: v })}
                    label="Low-resource mode"
                  />
                </div>
              </div>
            </>
          )}


          {tab === 'style' && <StyleTab Section={StudioSection} />}

          {tab === 'motion' && <MotionTab Section={StudioSection} SwitchCtl={StudioSwitch} />}

          {tab === 'type' && (
            <>
              <div className="studio__section">
                <div className="studio__section-title">Font family</div>
                <div className="studio-seg">
                  {FONTS.map((f) => (
                    <button
                      key={f}
                      className={`studio-seg__btn ${settings.fontPreset === f ? 'studio-seg__btn--active' : ''}`}
                      onClick={() => patch({ fontPreset: f })}
                    >
                      {f}
                    </button>
                  ))}
                </div>
              </div>

              <div className="studio__section">
                <div className="studio-slider">
                  <div className="studio-slider__head">
                    <span>Text size</span>
                    <span>{Math.round(settings.fontSize * 100)}%</span>
                  </div>
                  <input
                    type="range"
                    min={0.9}
                    max={1.2}
                    step={0.05}
                    value={settings.fontSize}
                    onChange={(e) => patch({ fontSize: Number(e.target.value) })}
                  />
                </div>
                <div className="studio-slider">
                  <div className="studio-slider__head">
                    <span>Line height</span>
                    <span>{settings.lineHeight.toFixed(2)}</span>
                  </div>
                  <input
                    type="range"
                    min={1.3}
                    max={1.9}
                    step={0.05}
                    value={settings.lineHeight}
                    onChange={(e) => patch({ lineHeight: Number(e.target.value) })}
                  />
                </div>
                <div className="studio-toggle">
                  <div>
                    <div className="studio-toggle__text">Bolder text</div>
                    <div className="studio-toggle__hint">Increases interface text weight</div>
                  </div>
                  <Switch
                    checked={settings.boldText}
                    onChange={(v) => patch({ boldText: v })}
                    label="Bolder text"
                  />
                </div>
              </div>

              <div className="studio__section">
                <div className="studio__section-title">Sinhala / English sample</div>
                <div
                  className="studio-preview"
                  style={{
                    fontSize: `calc(14px * ${settings.fontSize})`,
                    lineHeight: settings.lineHeight,
                    fontWeight: settings.boldText ? 600 : 400,
                  }}
                >
                  à·€à·šà¶½à·’à¶šà·Šà·ƒà·Š VPN — à¶†à¶»à¶šà·Šà·‚à·’à¶­, à·€à·šà¶œà·€à¶­à·Š à·ƒà¶¸à·Šà¶¶à¶±à·Šà¶°à¶­à·à·€à¶º
                  <br />
                  VEYLIX VPN — Secure, fast connectivity
                </div>
              </div>
            </>
          )}
        </div>

        <footer className="studio__foot">
          <Button
            variant="ghost"
            leading={<RotateCcw size={15} />}
            onClick={resetTheme}
            title="Reset to the default Modern theme (unlocks)"
          >
            Reset
          </Button>
          <Button
            variant="outline"
            leading={<Eye size={15} />}
            onClick={() => setPreviewOpen(true)}
            title="Preview this theme on a full app mock"
          >
            Preview
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              discardDraft()
              toast('info', 'Changes discarded', 'Your saved theme is unchanged')
            }}
            disabled={!dirty}
            style={{ flex: 1 }}
          >
            Discard
          </Button>
          <Button
            onClick={() => {
              saveTheme()
              toast('success', 'Theme saved & locked', 'It stays until you save another theme')
            }}
            disabled={!dirty}
            leading={<Check size={15} />}
            style={{ flex: 1 }}
          >
            {isLocked ? 'Save & lock' : 'Save'}
          </Button>
        </footer>
      </aside>

      {previewOpen && settings && (
        <ThemePreview
          draft={settings}
          onSave={() => {
            saveTheme()
            setPreviewOpen(false)
            toast('success', 'Theme saved & locked', 'It stays until you save another theme')
          }}
          onDiscard={() => setPreviewOpen(false)}
        />
      )}
    </>
  )
}




