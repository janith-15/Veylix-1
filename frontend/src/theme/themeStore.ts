import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import {
  type ThemePreset,
  type ThemeSettings,
  settingsForPreset,
  DEFAULT_SETTINGS,
  DEFAULT_VARIANT,
  componentsForPreset,
  migrateScene,
  PRESET_LABELS,
} from './themes'
import { findSubTheme, resolveVariant, type SubTheme } from './subthemes'

/** The two studio layouts. One studio, two views. */
export type StudioView = 'compact' | 'full'

export interface ThemeState {
  /** LIVE, applied theme. Changes only through saveTheme() / resetTheme(). */
  settings: ThemeSettings
  /** Editing buffer. `null` means "not editing" (studio closed, nothing pending). */
  draft: ThemeSettings | null
  /** True once a theme has been explicitly saved. */
  isLocked: boolean
  /**
   * Which studio layout is open: the 420px quick drawer, or the full-page
   * advanced view. Persisted so reopening the studio lands where the user left
   * it - someone who works in the full view should not be demoted to the drawer
   * on every session.
   */
  studioView: StudioView
  setStudioView: (v: StudioView) => void

  /* --- draft editing (Theme Studio) --- */
  beginDraft: () => void
  draftSetPreset: (preset: ThemePreset) => void
  draftSetVariant: (id: string) => void
  draftUpdate: (patch: Partial<ThemeSettings>) => void

  /* --- commit / revert --- */
  saveTheme: () => void
  discardDraft: () => void
  resetTheme: () => void
  /**
   * Swap the LIGHT/DARK preset on the LIVE theme, without opening the studio.
   *
   * The Theme Studio is the only other way to change `preset`, and it goes
   * through a draft that must then be saved - a two-step journey for the most
   * common preference there is. This commits immediately and applies the new
   * preset's base tokens while **preserving every other choice the user made**
   * (variant sub-theme, background scene, component shapes, motion, density).
   * `settingsForPreset()` is used only for fields the preset owns, and the
   * caller's explicit overrides are spread back on top, so toggling appearance
   * never silently resets a chosen scene or sub-theme.
   */
  setPreset: (preset: ThemePreset) => void

  hasUnsavedChanges: () => boolean
  /** Convenience: the settings to render while editing (draft ?? live). */
  activeSettings: () => ThemeSettings
}

/** Map a sub-theme CSS variable onto its ThemeSettings field. */
const VAR_TO_FIELD: Record<string, keyof ThemeSettings> = {
  '--c-primary': 'primary',
  '--c-primary-hover': 'primary',
  '--c-secondary': 'secondary',
  '--bg-page': 'bgPage',
  '--bg-sidebar': 'bgSidebar',
  '--bg-card': 'bgCard',
  '--border': 'border',
}

/** Convert the variant CSS variables into editable settings values. */
function varsToSettings(vars: Record<string, string>): Partial<ThemeSettings> {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(vars)) {
    if (key === '--radius') {
      const px = Number.parseFloat(value)
      if (Number.isFinite(px)) out.radius = px
      continue
    }
    if (key === '--border-weight') {
      const px = Number.parseFloat(value)
      if (Number.isFinite(px)) out.borderWidth = px
      continue
    }
    const field = VAR_TO_FIELD[key]
    if (field) out[field] = value
  }
  return out as Partial<ThemeSettings>
}

/**
 * Picking a sub-theme applies its structure (UI skin, shell, button shape) and
 * its palette — but **not** its scene.
 *
 * The scene belongs to the user now: the Theme Studio has a picker for it, and
 * choosing "Amber Console" should recolour the background the user already
 * chose, not silently swap it for a different animation. The preset still
 * supplies a default scene when the PRESET is picked (see `draftSetPreset`), so
 * "the effect is bound to the theme" and "the user may override it" are both
 * true at once, which was the whole requirement.
 */
function variantSurface(v: SubTheme): Partial<ThemeSettings> {
  return {
    preset: v.preset,
    uiVariant: v.uiVariant,
    shell: v.shell,
    components: { ...componentsForPreset(v.preset), buttonShape: v.buttonShape },
    ...varsToSettings(v.vars),
  }
}

export const useThemeStore = create<ThemeState>()(
  persist(
    (set, get) => ({

      /* Kept for completeness with ThemeState; the persisted value is replaced
         by `migrate()` on rehydrate and re-completed by `merge()` below. */
      settings: settingsForPreset('modern'),
      draft: null,
      isLocked: false,
      studioView: 'compact',

      setStudioView: (v) => set({ studioView: v }),

      /* ---------------- draft editing ---------------- */
      beginDraft: () =>
        set((state) => ({ draft: state.draft ?? { ...state.settings } })),

      draftSetPreset: (preset) =>
        set((state) => ({
          draft: { ...(state.draft ?? state.settings), ...settingsForPreset(preset), preset },
        })),

      draftSetVariant: (id) =>
        set((state) => {
          const v = findSubTheme(id)
          if (!v) return state
          return { draft: { ...(state.draft ?? state.settings), variant: id, ...variantSurface(v) } }
        }),

      draftUpdate: (patch) =>
        set((state) => ({ draft: { ...(state.draft ?? state.settings), ...patch } })),

      /* ---------------- commit / revert ---------------- */
      /** Commit the draft: lock it in as the live theme. */
      saveTheme: () =>
        set((state) => {
          const next = state.draft ?? state.settings
          return { settings: next, draft: null, isLocked: true }
        }),

      /** Throw away pending edits (live theme is untouched). */
      discardDraft: () => set({ draft: null }),

      /** Back to factory default, unlocked. */
      resetTheme: () =>
        set({ settings: settingsForPreset('modern'), draft: null, isLocked: false }),

      /**
       * Commit a preset swap on the LIVE theme (the topbar light/dark toggle).
       *
       * Order matters and is deliberate:
       *   1. the new preset's base tokens (`settingsForPreset`) - defines the
       *      palette, so a light preset cannot inherit a dark background;
       *   2. the user's current sub-theme, re-resolved FOR THE NEW PRESET. A
       *      variant id is preset-scoped, so the old one may not exist; when it
       *      does not, the new preset's default variant is used rather than a
       *      dangling id that `resolveVariant` would fail to style;
       *   3. the user's own non-colour choices (scene, density, motion, font,
       *      low-resource, reduced-motion) - an appearance toggle must not
       *      discard them.
       *
       * Colors are NOT carried over: they are what the toggle exists to change.
       * Any pending draft is dropped, because the live theme just moved and a
       * stale draft would silently revert the toggle on the next Save.
       */
      setPreset: (preset) =>
        set((state) => {
          const base = settingsForPreset(preset)
          const visual = base.variant ?? DEFAULT_VARIANT[preset]
          const variant = resolveVariant(preset, visual) ?? resolveVariant(preset, DEFAULT_VARIANT[preset])
          const prev = state.settings
          return {
            settings: {
              ...base,
              ...(variant ? varsToSettings(variant.vars) : {}),
              ...(variant
                ? {
                    uiVariant: variant.uiVariant,
                    shell: variant.shell,
                    components: { ...base.components, buttonShape: variant.buttonShape },
                  }
                : {}),
              preset,
              variant: variant?.id ?? visual,
              /* Keep the user's own choices. */
              scene: prev.scene,
              density: prev.density,
              fontPreset: prev.fontPreset,
              boldText: prev.boldText,
              reducedMotion: prev.reducedMotion,
              lowResourceMode: prev.lowResourceMode,
              glassEffect: prev.glassEffect,
              effectLevel: prev.effectLevel,
              style: prev.style,
            },
            draft: null,
            isLocked: true,
          }
        }),

      hasUnsavedChanges: () => {
        const { settings, draft } = get()
        return draft !== null && JSON.stringify(draft) !== JSON.stringify(settings)
      },

      activeSettings: () => get().draft ?? get().settings,
    }),
    {
      name: 'veylix-theme',
      version: 6,
      partialize: (state) => ({
        settings: state.settings,
        isLocked: state.isLocked,
        studioView: state.studioView,
      }),

      /**
       * Runs on EVERY rehydrate, after `migrate()`.
       *
       * This is the single place that guarantees `settings.components` and
       * `settings.style` are complete objects. Without it, any component that
       * reads `s.settings.components.x` throws `Cannot read properties of
       * undefined` on a partially-saved payload — and because those reads run
       * inside render/effects, the whole shell dies with no visual clue beyond a
       * blank screen. Completing the shape once, here, is cheaper and safer than
       * making every consumer defensive.
       */
      merge: (persisted, current) => {
        const p = persisted as
          | { settings?: Partial<ThemeSettings>; isLocked?: boolean; studioView?: StudioView }
          | undefined
        const saved = p?.settings ?? {}
        const preset = saved.preset ?? current.settings.preset
        const base = settingsForPreset(preset)
        return {
          ...current,
          settings: {
            ...base,
            ...saved,
            components: { ...base.components, ...(saved.components ?? {}) },
            style: { ...base.style, ...(saved.style ?? {}) },
          },
          isLocked: p?.isLocked ?? current.isLocked,
          studioView: p?.studioView ?? current.studioView,
          draft: null,
        }
      },

      /**
       * v5: introduced the draft/lock model. Anything saved before v5 is
       * treated as a committed (locked) theme; no draft is restored.
       *
       * v6: `rings` was removed and the `hacking` preset was added. A saved
       * theme may therefore carry a scene id that no longer exists, which would
       * leave the file with a background it cannot draw — a silent blank screen
       * rather than an error. `migrateScene()` rewrites `rings` to `aurora-wave`
       * and any unknown id to the modern default, so an old session always
       * resolves to a scene this build can actually paint.
       */
      migrate: (persisted) => {
        const state = persisted as
          | { settings?: Partial<ThemeSettings>; isLocked?: boolean; studioView?: StudioView }
          | undefined
        const saved = state?.settings ?? {}
        const rawPreset = saved.preset as string | undefined
        // A preset id from a newer build must not break `PRESET_COLORS[preset]`.
        const preset: ThemePreset =
          rawPreset && rawPreset in PRESET_LABELS
            ? (rawPreset as ThemePreset)
            : DEFAULT_SETTINGS.preset
        const base = settingsForPreset(preset)
        const v = resolveVariant(preset, saved.variant ?? DEFAULT_VARIANT[preset])
        const legacyColors: (keyof ThemeSettings)[] = [
          'primary', 'secondary', 'bgPage', 'bgSidebar', 'bgCard', 'border',
          'success', 'warning', 'error', 'info',
        ]
        const rest: Record<string, unknown> = { ...saved }
        for (const key of legacyColors) delete rest[key]

        /*
         * `components` and `style` must be COMPLETE objects, not whatever the
         * saved payload happened to contain. A partially-migrated blob (or a
         * missing key) used to reach `ToastProvider`, which reads
         * `components.toastDuration` and crashed the whole app to a blank
         * screen — every page, not just the theme studio. Merging onto the
         * preset defaults keeps a saved tweak while guaranteeing every field
         * the UI dereferences exists.
         */
        const savedComponents = (rest.components ?? {}) as Partial<ThemeSettings['components']>
        const savedStyle = (rest.style ?? {}) as Partial<ThemeSettings['style']>

        return {
          settings: {
            ...base,
            ...varsToSettings(v.vars),
            ...(rest as Partial<ThemeSettings>),
            preset,
            variant: v.id,
            scene: migrateScene((rest.scene as string | undefined) ?? base.scene),
            components: { ...base.components, ...savedComponents },
            style: { ...base.style, ...savedStyle },
          },
          isLocked: state?.isLocked ?? true,
          studioView: state?.studioView ?? 'compact',
          draft: null,
        }
      },
    },
  ),
)

