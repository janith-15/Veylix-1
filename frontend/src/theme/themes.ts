/**
 * VEYLIx VPN — Theme types and presets.
 * The Five mandated presets: dark, light, classic (Normal/Classic),
 * gaming, modern (recommended default).
 */

export type ThemePreset = 'dark' | 'light' | 'classic' | 'gaming' | 'modern' | 'hacking'
export type EffectLevel = 'off' | 'subtle' | 'enhanced'
export type Density = 'comfortable' | 'compact'
export type FontPreset = 'default' | 'system' | 'tidy' | 'mono' | 'serif'

/** Per-preset UI language (drives [data-ui] in every stylesheet). */
export type UiVariant = 'soft' | 'glass' | 'editorial' | 'flat' | 'neon'

/**
 * Per-preset 3D background scene.
 *
 * `rings` was REMOVED (owner decision, 2026-11-06): the soft concentric rings
 * were the one scene nobody kept, and they collided with the three new
 * terminal/CRT scenes the Hacking preset introduced. Every sub-theme that
 * used to carry `rings` now carries `aurora-wave` — see `migrateScene()`.
 *
 * `flat` is not a 3D scene: it means "no decorative background at all", which
 * is what the Classic preset (a solid, professional admin panel) wants.
 */
export type SceneType =
  | 'orbit'
  | 'nebula'
  | 'grid'
  | 'flat'
  /* --- Hacking preset: terminal / CRT family --- */
  | 'scanline-crt'
  | 'classic-terminal'
  | 'classic-oscilloscope'
  /* --- Light family (replaces `rings`) --- */
  | 'aurora-wave'
  | 'light-caustics'
  | 'light-geometric'
  /* --- Dark family --- */
  | 'subtle-orbs'
  | 'dark-stealth-radar'
  | 'dark-starfall'
  /* --- Modern / Pro family --- */
  | 'particles'
  | 'pro-worldmap'
  | 'pro-fiber'
  /* --- Gaming family --- */
  | 'cyber-grid'
  | 'gaming-warp'
  | 'gaming-hex'

/** Scene ids that actually paint something. `flat` is deliberately excluded. */
export type AnimatedSceneType = Exclude<SceneType, 'flat'>

/** Layout shell shape: floating panel sidebar vs. edge-to-edge admin. */
export type ShellType = 'floating' | 'edge' | 'hud' | 'ruled'

/** Button / control shapes (Theme Studio -> Components). */
export type ButtonShape = 'soft' | 'square' | 'pill' | 'clipped'
/** Notification attachment style. */
export type ToastStyle = 'bar' | 'line' | 'badge'
/** Toast corner placement. */
export type ToastPosition = 'tr' | 'tl' | 'br' | 'bl'
/** Chart line / area rendering. */
export type ChartStyle = 'smooth' | 'sharp' | 'area'
/** Palette used by chart primitives. */
export type ChartPalette = 'theme' | 'primary' | 'secondary' | 'custom'

/** Component-level customisation (user editable). */
/** Card surface treatment. */
export type CardBg = 'none' | 'gradient' | 'noise' | 'grid' | 'scanlines'
/** Entrance animation for cards and panels. */
export type MountAnim = 'none' | 'fade' | 'rise' | 'slide'
/** Card hover behaviour. */
export type HoverAnim = 'none' | 'lift' | 'tilt' | 'glow'
/** Button press feedback. */
export type ButtonHover = 'none' | 'squash' | 'ripple' | 'glow'
/** Motion temperament. */
export type MotionMood = 'none' | 'calm' | 'snappy' | 'playful'

/**
 * Free-form visual controls (Theme Studio -> Style / Motion).
 * Everything here maps to a --style-* custom property, so a single
 * settings object re-skins surfaces, type and motion at once.
 */
export interface StyleSettings {
  /* Surfaces */
  cardBg: CardBg
  cardBgAngle: number
  cardBgIntensity: number
  textureOpacity: number

  /* Typography */
  headingTransform: 'none' | 'uppercase'
  headingFont: 'body' | 'mono' | 'serif'
  headingWeight: number
  headingTracking: number
  labelTransform: 'none' | 'uppercase'

  /* Motion */
  mood: MotionMood
  speed: number
  mount: MountAnim
  stagger: boolean
  hover: HoverAnim
  buttonHover: ButtonHover
  bgDrift: boolean
}
export interface ComponentSettings {
  buttonShape: ButtonShape
  buttonRadius: number
  buttonUppercase: boolean
  buttonGlow: number
  toastStyle: ToastStyle
  toastPosition: ToastPosition
  toastRadius: number
  toastDuration: number
  badgeShape: 'pill' | 'square' | 'tag'
  badgeDot: boolean
  chartPalette: ChartPalette
  chartStyle: ChartStyle
  chartGrid: boolean
  chartAnimate: boolean
}

/** A user-saved theme snapshot. */
export interface SavedPreset {
  id: string
  name: string
  createdAt: number
  settings: ThemeSettings
}

export interface ThemeSettings {
  preset: ThemePreset
  /** Sub-theme editable colors */
  primary: string
  secondary: string
  bgPage: string
  bgSidebar: string
  bgCard: string
  border: string
  success: string
  warning: string
  error: string
  info: string
  /** Effects */
  effectLevel: EffectLevel
  /** Per-theme UI language, 3D scene and shell shape (set by the preset) */
  /** Active sub-theme id (see theme/subthemes.ts) */
  variant: string
  uiVariant: UiVariant
  scene: SceneType
  shell: ShellType
  /** Scene / animation speed multiplier (per theme) */
  sceneSpeed: number
  /** Glassmorphism surfaces (default on; Classic disables it) */
  glassEffect: boolean
  radius: number
  borderWidth: number
  shadowLevel: number
  /** Typography */
  fontPreset: FontPreset
  fontSize: number
  lineHeight: number
  density: Density
  boldText: boolean
  /** Component-level customisation (Theme Studio -> Components) */
  components: ComponentSettings
  /** Free-form surfaces / type / motion (Theme Studio -> Style + Motion) */


  style: StyleSettings
  /** User-saved themes */
  customPresets: SavedPreset[]
  /** Accessibility / resource */
  reducedMotion: boolean
  lowResourceMode: boolean
}

export const FONT_STACKS: Record<FontPreset, string> = {
  default:
    "'Inter', 'Segoe UI', 'Noto Sans Sinhala', 'Iskoola Pota', 'Nirmala UI', system-ui, -apple-system, sans-serif",
  system:
    "system-ui, -apple-system, 'Segoe UI', 'Noto Sans Sinhala', 'Iskoola Pota', 'Nirmala UI', sans-serif",
  tidy:
    "'Inter', 'Aptos', 'Noto Sans Sinhala', 'Iskoola Pota', 'Nirmala UI', system-ui, sans-serif",
  mono:
    "'Cascadia Code', 'Consolas', 'Noto Sans Sinhala', ui-monospace, monospace",
  serif:
    "Georgia, 'Cambria', 'Noto Serif Sinhala', 'Iskoola Pota', 'Times New Roman', serif",

}
export const PRESET_LABELS: Record<ThemePreset, string> = {
  modern: 'Modern',
  dark: 'Dark',
  light: 'Light',
  classic: 'Normal / Classic',
  gaming: 'Gaming',
  hacking: 'Hacking',
}

/** Human labels for the per-preset UI languages (shown in Theme Studio). */
export const UI_VARIANT_LABELS: Record<UiVariant, string> = {
  soft: 'Soft layers',
  glass: 'Deep glass',
  editorial: 'Editorial',
  flat: 'Flat admin',
  neon: 'Neon HUD',
}

/**
 * Every background scene, grouped by the preset that owns it.
 *
 * The group is what the Theme Studio shows as a section heading, and it is the
 * end of the argument about "which theme does this effect belong to": a scene
 * has exactly one family, and a user may still pick any of them.
 */
export const SCENE_GROUP: Record<AnimatedSceneType, ThemePreset> = {
  'particles': 'modern',
  'pro-worldmap': 'modern',
  'pro-fiber': 'modern',
  'subtle-orbs': 'dark',
  'dark-stealth-radar': 'dark',
  'dark-starfall': 'dark',
  'aurora-wave': 'light',
  'light-caustics': 'light',
  'light-geometric': 'light',
  'cyber-grid': 'gaming',
  'gaming-warp': 'gaming',
  'gaming-hex': 'gaming',
  'scanline-crt': 'hacking',
  'classic-terminal': 'hacking',
  'classic-oscilloscope': 'hacking',
  'orbit': 'modern',
  'nebula': 'dark',
  'grid': 'gaming',
}

export const SCENE_LABELS: Record<SceneType, string> = {
  orbit: 'Orbit wireframe',
  nebula: 'Nebula drift',
  grid: 'Neon grid',
  flat: 'None (flat)',
  'scanline-crt': 'Scanline CRT',
  'classic-terminal': 'Terminal stream',
  'classic-oscilloscope': 'Oscilloscope',
  'aurora-wave': 'Aurora wave',
  'light-caustics': 'Light caustics',
  'light-geometric': 'Geometric shapes',
  'subtle-orbs': 'Soft orbs',
  'dark-stealth-radar': 'Stealth radar',
  'dark-starfall': 'Starfall',
  particles: 'Particle network',
  'pro-worldmap': 'World map',
  'pro-fiber': 'Fiber optics',
  'cyber-grid': 'Cyber grid',
  'gaming-warp': 'Hyperspace warp',
  'gaming-hex': 'Hex matrix',
}

/** A one-line description per scene — shown under the picker tile. */
export const SCENE_BLURBS: Record<SceneType, string> = {
  orbit: 'Rotating wireframe icosahedron with a soft halo',
  nebula: 'Slow drifting nebula with a starfield',
  grid: 'Synthwave perspective floor grid',
  flat: 'No decorative background — solid surfaces only',
  'scanline-crt': 'CRT scanlines and a slow rolling bar',
  'classic-terminal': 'Streaming terminal log lines',
  'classic-oscilloscope': 'A phosphor oscilloscope trace',
  'aurora-wave': 'Layered aurora ribbons drifting sideways',
  'light-caustics': 'Water caustics rippling across the page',
  'light-geometric': 'Floating wireframe solids in soft light',
  'subtle-orbs': 'Blurred bokeh orbs breathing in and out',
  'dark-stealth-radar': 'Radar sweep with fading contact blips',
  'dark-starfall': 'Stars falling through a dark field',
  particles: 'Connected node network with travelling packets',
  'pro-worldmap': 'Dotted world map with pulsing hubs',
  'pro-fiber': 'Light pulses racing along fiber buses',
  'cyber-grid': 'Neon grid floor with travelling packets',
  'gaming-warp': 'Stars streaking past at warp speed',
  'gaming-hex': 'Honeycomb of hex cells pulsing in waves',
}

/** The 3 scenes the Theme Studio offers for each preset, in display order. */
export const SCENES_FOR_PRESET: Record<ThemePreset, SceneType[]> = {
  modern: ['particles', 'pro-worldmap', 'pro-fiber'],
  dark: ['subtle-orbs', 'dark-stealth-radar', 'dark-starfall'],
  light: ['aurora-wave', 'light-caustics', 'light-geometric'],
  classic: ['flat'],
  gaming: ['cyber-grid', 'gaming-warp', 'gaming-hex'],
  hacking: ['scanline-crt', 'classic-terminal', 'classic-oscilloscope'],
}

/**
 * Rewrite a scene id that no longer exists.
 *
 * Called from two places and both matter:
 *  1. `themeStore` hydration — a saved theme may carry `rings`;
 *  2. `subthemes.ts` — 12 shipped sub-themes carried it too.
 *
 * Mapping `rings` → `aurora-wave` is not arbitrary: both are soft, low-contrast,
 * light-theme backgrounds, so a user who chose "Soft rings" gets the closest
 * thing that still exists rather than a jarring switch to a radar sweep.
 */
export function migrateScene(scene: string | undefined | null): SceneType {
  if (!scene) return 'particles'
  if (scene === 'rings') return 'aurora-wave'
  if (scene in SCENE_LABELS) return scene as SceneType
  // An id from a newer build than this one: fall back rather than render null.
  return 'particles'
}

/** Shipped component defaults (used when a preset does not override them). */
export const DEFAULT_COMPONENTS: ComponentSettings = {
  buttonShape: 'soft',
  buttonRadius: 8,
  buttonUppercase: false,
  buttonGlow: 40,
  toastStyle: 'bar',
  toastPosition: 'br',
  toastRadius: 8,
  toastDuration: 6,
  badgeShape: 'pill',
  badgeDot: false,
  chartPalette: 'theme',
  chartStyle: 'area',
  chartGrid: true,
  chartAnimate: true,
}

/** Per-preset component personalities - this is what makes each theme feel different. */
export const PRESET_COMPONENTS: Record<ThemePreset, Partial<ComponentSettings>> = {
  modern: { buttonShape: 'pill', badgeShape: 'pill', toastStyle: 'bar', chartStyle: 'area', buttonGlow: 55 },
  dark: { buttonShape: 'soft', badgeShape: 'pill', toastStyle: 'line', chartStyle: 'smooth', buttonGlow: 70 },
  light: { buttonShape: 'square', badgeShape: 'tag', toastStyle: 'bar', chartStyle: 'sharp', buttonGlow: 0, badgeDot: true },
  classic: { buttonShape: 'square', badgeShape: 'square', toastStyle: 'badge', chartStyle: 'sharp', buttonGlow: 0, chartGrid: true, chartAnimate: false },
  gaming: { buttonShape: 'clipped', badgeShape: 'tag', toastStyle: 'badge', chartStyle: 'smooth', buttonGlow: 100, badgeDot: true },
  /* Terminal: square, uppercase, glowing phosphor, no grid on the charts. */
  hacking: { buttonShape: 'square', badgeShape: 'square', toastStyle: 'line', chartStyle: 'sharp', buttonGlow: 85, buttonUppercase: true, badgeDot: true, chartGrid: false, chartAnimate: true },
}

/** Resolve the effective component settings for a preset. */
export function componentsForPreset(preset: ThemePreset): ComponentSettings {
  return { ...DEFAULT_COMPONENTS, ...PRESET_COMPONENTS[preset] }
}
/** Shipped style defaults. */
export const DEFAULT_STYLE: StyleSettings = {
  cardBg: 'none',
  cardBgAngle: 135,
  cardBgIntensity: 40,
  textureOpacity: 8,
  headingTransform: 'none',
  headingFont: 'body',
  headingWeight: 700,
  headingTracking: 0,
  labelTransform: 'none',
  mood: 'calm',
  speed: 1,
  mount: 'fade',
  stagger: false,
  hover: 'none',
  buttonHover: 'none',
  bgDrift: false,
}

/** Per-theme visual personality for surfaces, type and motion. */
export const PRESET_STYLE: Record<ThemePreset, Partial<StyleSettings>> = {
  modern: {
    cardBg: 'gradient', cardBgIntensity: 30, mount: 'rise', stagger: true,
    hover: 'lift', buttonHover: 'squash', mood: 'playful', speed: 1,
    headingWeight: 700, bgDrift: true,
  },
  dark: {
    cardBg: 'noise', textureOpacity: 10, mount: 'fade', hover: 'glow',
    buttonHover: 'glow', mood: 'calm', speed: 0.6, headingWeight: 600, bgDrift: true,
  },
  light: {
    cardBg: 'none', mount: 'fade', hover: 'none', mood: 'snappy', speed: 1.2,
    headingTransform: 'uppercase', headingWeight: 800, headingTracking: 0.08,
    labelTransform: 'uppercase',
  },
  classic: {
    cardBg: 'none', mount: 'none', hover: 'none', mood: 'none', speed: 0,
    headingWeight: 700, stagger: false,
  },
  gaming: {
    cardBg: 'scanlines', textureOpacity: 12, mount: 'slide', stagger: true,
    hover: 'glow', buttonHover: 'ripple', mood: 'playful', speed: 1.6,
    headingTransform: 'uppercase', headingFont: 'mono', headingWeight: 800,
    headingTracking: 0.04, labelTransform: 'uppercase', bgDrift: true,
  },
  /* Terminal: mono everywhere, no flourish, punctuation-speed motion. */
  hacking: {
    cardBg: 'scanlines', textureOpacity: 9, mount: 'none', hover: 'glow',
    buttonHover: 'glow', mood: 'snappy', speed: 1.1,
    headingTransform: 'uppercase', headingFont: 'mono', headingWeight: 700,
    headingTracking: 0.06, labelTransform: 'uppercase', stagger: false,
  },
}

/** Resolve the effective style for a preset. */
export function styleForPreset(preset: ThemePreset): StyleSettings {
  return { ...DEFAULT_STYLE, ...PRESET_STYLE[preset] }
}

export const DEFAULT_SETTINGS: ThemeSettings = {
  preset: 'modern',
  primary: '#5b5bd6',
  secondary: '#06b6d4',
  bgPage: '',
  bgSidebar: '',
  bgCard: '',
  border: '',
  success: '',
  warning: '',
  error: '',
  info: '',
  effectLevel: 'subtle',
  uiVariant: 'soft',
  variant: 'soft',
  scene: 'orbit',
  shell: 'floating',
  sceneSpeed: 1,
  components: DEFAULT_COMPONENTS,
  style: DEFAULT_STYLE,
  customPresets: [],
  glassEffect: true,
  radius: 10,
  borderWidth: 1,
  shadowLevel: 2,
  fontPreset: 'default',
  fontSize: 1,
  lineHeight: 1.5,
  density: 'comfortable',
  boldText: false,
  reducedMotion: false,
  lowResourceMode: false,
}

/** Per-preset base colors (only what differs from DEFAULT_SETTINGS). */
export const PRESET_COLORS: Record<ThemePreset, Partial<ThemeSettings>> = {
  modern: {
    primary: '#5b5bd6',
    secondary: '#06b6d4',
    bgPage: '#f5f5fa',
    bgSidebar: '#ffffff',
    bgCard: '#ffffff',
    border: '#e4e4f2',
    success: '#10b981',
    warning: '#f59e0b',
    error: '#e11d48',
    info: '#3b82f6',
    radius: 12,
    uiVariant: 'soft',
    scene: 'orbit',
    shell: 'floating',
    sceneSpeed: 1,
  },
  dark: {
    primary: '#7c3aed',
    secondary: '#22d3ee',
    bgPage: '#0a0a14',
    bgSidebar: '#0e0e1a',
    bgCard: '#121222',
    border: '#262550',
    success: '#34d399',
    warning: '#fbbf24',
    error: '#e11d48',
    info: '#60a5fa',
    radius: 12,
    uiVariant: 'glass',
    scene: 'nebula',
    shell: 'edge',
    sceneSpeed: 0.6,
  },
  light: {
    primary: '#4f46e5',
    secondary: '#f59e0b',
    bgPage: '#f7f8fd',
    bgSidebar: '#ffffff',
    bgCard: '#ffffff',
    border: '#dee3f1',
    success: '#059669',
    warning: '#d97706',
    error: '#dc2626',
    info: '#2563eb',
    radius: 8,
    uiVariant: 'editorial',
    scene: 'aurora-wave',
    shell: 'ruled',
    sceneSpeed: 0.8,
  },
  classic: {
    primary: '#2f6bef',
    secondary: '#5e7191',
    bgPage: '#f2f4f8',
    bgSidebar: '#ffffff',
    bgCard: '#ffffff',
    border: '#dde2ec',
    success: '#16a34a',
    warning: '#dc6803',
    error: '#d92d20',
    info: '#1570ef',
    radius: 6,
    glassEffect: false,
    uiVariant: 'flat',
    scene: 'flat',
    shell: 'edge',
    sceneSpeed: 0.4,
  },
  /**
   * Hacking — terminal / CRT. Dark by necessity (phosphor on black), mono type,
   * hairline borders and a scanline background. It is NOT a rename of Classic:
   * Classic is the light, solid admin panel above; this is the operator who
   * lives in a shell.
   */
  hacking: {
    primary: '#22d37a',
    secondary: '#0ea5a4',
    bgPage: '#050a08',
    bgSidebar: '#07110d',
    bgCard: '#0a1712',
    border: '#123326',
    success: '#22d37a',
    warning: '#e3b341',
    error: '#ff5c6c',
    info: '#4cc2ff',
    radius: 4,
    uiVariant: 'neon',
    scene: 'scanline-crt',
    shell: 'hud',
    sceneSpeed: 0.9,
  },
  gaming: {
    primary: '#00f0ff',
    secondary: '#d6117d',
    bgPage: '#04060d',
    bgSidebar: '#080d18',
    bgCard: '#0b1322',
    border: '#182a4a',
    success: '#00e58a',
    warning: '#ffc53d',
    error: '#ff4d6d',
    info: '#3d9bff',
    radius: 14,
    effectLevel: 'enhanced',
    uiVariant: 'neon',
    scene: 'grid',
    shell: 'hud',
    sceneSpeed: 1.6,
  },
}

/** Default sub-theme id per preset (kept here to avoid a circular import). */
export const DEFAULT_VARIANT: Record<ThemePreset, string> = {
  modern: 'soft',
  dark: 'deep-glass',
  light: 'editorial',
  classic: 'flat-admin',
  gaming: 'neon-hud',
  hacking: 'terminal-green',
}
/** Build a full settings object for a preset. */
export function settingsForPreset(preset: ThemePreset): ThemeSettings {
  return {
    ...DEFAULT_SETTINGS,
    ...PRESET_COLORS[preset],
    variant: DEFAULT_VARIANT[preset],
    components: componentsForPreset(preset),
    style: styleForPreset(preset),
    preset,
  }
}

/** Per-preset background gradient (single source of visual identity). */
export const PRESET_GRADIENTS: Record<ThemePreset, string> = {
  modern:
    'radial-gradient(1100px 560px at 12% -10%, color-mix(in srgb, var(--c-primary) 16%, transparent), transparent 60%), radial-gradient(900px 480px at 108% 14%, color-mix(in srgb, var(--c-secondary) 14%, transparent), transparent 60%)',
  dark:
    'radial-gradient(1200px 620px at 78% -14%, color-mix(in srgb, var(--c-primary) 30%, transparent), transparent 62%), radial-gradient(900px 520px at 4% 82%, color-mix(in srgb, var(--c-secondary) 16%, transparent), transparent 60%)',
  light:
    'radial-gradient(900px 420px at 50% -18%, color-mix(in srgb, var(--c-primary) 9%, transparent), transparent 65%)',
  classic: 'none',
  gaming:
    'radial-gradient(1200px 600px at 15% -10%, rgba(0,240,255,0.18), transparent 55%), radial-gradient(1000px 520px at 110% 20%, rgba(214,17,125,0.15), transparent 55%)',
  hacking:
    'radial-gradient(1100px 520px at 50% -20%, rgba(34,211,122,0.14), transparent 60%), radial-gradient(900px 480px at 90% 110%, rgba(14,165,164,0.10), transparent 60%)',
}

/** Per-preset tilt strength for the 3D cards (0 = no tilt). */
const PRESET_TILT: Record<ThemePreset, string> = {
  modern: '0.45',
  dark: '0.7',
  light: '0.25',
  classic: '0',
  gaming: '1',
  hacking: '0.15',
}

/** Convert settings to a record of CSS custom properties. */
export function themeToCssVars(s: ThemeSettings): Record<string, string> {
  /*
   * Rehydrated settings can be partial (an older saved payload, or a migration
   * that predates a field), so both sub-objects are merged onto the shipped
   * defaults before anything is read. Without this a stale `components` object
   * threw `Cannot read properties of undefined` inside the theme effect — after
   * paint, which leaves a mounted-looking but dead shell.
   */
  const components: ComponentSettings = { ...DEFAULT_COMPONENTS, ...(s.components ?? {}) }
  const style: StyleSettings = { ...DEFAULT_STYLE, ...(s.style ?? {}) }

  const depth =
    s.shadowLevel <= 0
      ? 'none'
      : s.shadowLevel === 1
        ? '0 1px 2px rgba(0,0,0,0.08)'
        : s.shadowLevel === 2
          ? '0 6px 18px rgba(0,0,0,0.14)'
          : '0 12px 40px rgba(0,0,0,0.22)'

  const gfx = s.effectLevel
  const gradient = gfx === 'off' ? 'none' : PRESET_GRADIENTS[s.preset]

  return {
    '--c-primary': s.primary,
    '--c-primary-hover': s.primary,
    '--c-secondary': s.secondary,
    '--bg-page': s.bgPage,
    '--bg-sidebar': s.bgSidebar,
    '--bg-card': s.bgCard,
    '--border': s.border,
    '--success': s.success,
    '--warning': s.warning,
    '--error': s.error,
    '--info': s.info,
    '--radius': `${Math.max(0, Math.min(24, s.radius))}px`,
    '--border-weight': `${Math.max(1, Math.min(3, s.borderWidth))}px`,
    '--shadow-md': depth,
    '--font-size-scale': String(Math.max(0.9, Math.min(1.2, s.fontSize))),
    '--line-height': String(s.lineHeight),
    '--fx-gradient': gradient,
    '--fx-glow': gfx === 'enhanced' ? '0 0 26px color-mix(in srgb, var(--c-primary) 40%, transparent)' : 'none',
    '--fx-tilt': gfx === 'off' ? '0' : PRESET_TILT[s.preset],
    '--fx-animate-bg': gfx === 'off' ? '0' : gfx === 'enhanced' ? '2' : '1',

    /* Brand mark follows the user's custom accent colours */
    '--logo-a': s.primary,
    '--logo-a-2': s.primary,
    '--logo-b': s.secondary,
    '--logo-b-2': s.secondary,
    '--logo-lock': s.bgPage,
    '--logo-shield-fill': `color-mix(in srgb, ${s.primary} 10%, transparent)`,

    /* ---- Component tokens (Layer 3 consistency engine) ---- */
    '--radius-card': s.radius + 6 + 'px',
    '--radius-card-lg': s.radius + 10 + 'px',
    '--cmp-radius-button': components.buttonShape === 'pill' ? '999px' : components.buttonShape === 'square' ? '0px' : components.buttonRadius + 'px',
    '--cmp-radius-badge': components.badgeShape === 'pill' ? '999px' : components.badgeShape === 'square' ? '0px' : '3px',
    '--cmp-radius-toast': components.toastRadius + 'px',
    '--cmp-btn-uppercase': components.buttonUppercase ? 'uppercase' : 'none',
    '--cmp-glow-alpha': String(components.buttonGlow / 100),
    '--cmp-accent-grad': 'linear-gradient(135deg, ' + s.primary + ', ' + s.secondary + ')',
    '--chart-1': components.chartPalette === 'secondary' ? s.secondary : s.primary,
    '--chart-2': components.chartPalette === 'primary' ? 'color-mix(in srgb, ' + s.primary + ' 55%, ' + s.secondary + ')' : s.secondary,
    '--chart-3': s.info,
    '--chart-4': s.success,
    '--chart-grid': components.chartGrid ? 'color-mix(in srgb, var(--text-muted) 22%, transparent)' : 'transparent',
    '--chart-axis': components.chartGrid ? 'color-mix(in srgb, var(--text-muted) 45%, transparent)' : 'transparent',
    /* ---- Style tokens: surfaces / type / motion (Theme Studio) ---- */
    '--style-card-bg': style.cardBg,
    '--style-card-angle': style.cardBgAngle + 'deg',
    '--style-card-intensity': String(style.cardBgIntensity / 100),
    '--style-texture': String(style.textureOpacity / 100),
    '--style-heading-transform': style.headingTransform,
    '--style-heading-font':
      style.headingFont === 'mono' ? 'var(--font-mono)' :
      style.headingFont === 'serif' ? "Georgia, 'Times New Roman', serif" :
      'var(--font-family)',
    '--style-heading-weight': String(style.headingWeight),
    '--style-heading-tracking': style.headingTracking + 'em',
    '--style-label-transform': style.labelTransform,
    '--style-speed': String(style.speed || 1),
    '--style-mount': style.mount,
    '--style-hover': style.hover,
    '--style-btn-hover': style.buttonHover,
    '--style-mood': style.mood,

    /* Glass: user toggle wins; Classic preset keeps solid surfaces */
    '--glass-enabled': s.glassEffect ? '1' : '0',
  }
}
