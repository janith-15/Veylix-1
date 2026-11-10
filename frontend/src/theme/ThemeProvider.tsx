import { useEffect, useRef, type ReactNode } from 'react'
import { useThemeStore } from './themeStore'
import { FONT_STACKS, themeToCssVars, DEFAULT_COMPONENTS, DEFAULT_STYLE } from './themes'
import { resolveVariant } from './subthemes'

/**
 * Applies the active theme to <html>:
 *  data-theme / data-variant / data-ui / data-shell / data-scene /
 *  data-glass / data-effect / data-style-* / data-btn-shape ...
 */
export function ThemeBootstrap({ children }: { children: ReactNode }) {
  const settings = useThemeStore((s) => s.settings)
  const firstRun = useRef(true)

  useEffect(() => {
    const root = document.documentElement
    const variant = resolveVariant(settings.preset, settings.variant)
    const variantVars = variant?.vars ?? {}

    /*
     * `settings` is rehydrated from localStorage, so any field can be missing on
     * a stale payload. Deriving these two sub-objects once, each merged onto the
     * shipped defaults, keeps this effect total: dereferencing
     * `settings.components.buttonShape` directly threw `Cannot read properties
     * of undefined` inside the theme effect on a partial save — after paint, so
     * the app looked mounted while the shell was actually broken.
     */
    const comp = { ...DEFAULT_COMPONENTS, ...(settings.components ?? {}) }
    const sty = { ...DEFAULT_STYLE, ...(settings.style ?? {}) }

    root.setAttribute('data-theme', settings.preset)
    root.setAttribute('data-variant', variant?.id ?? settings.variant)
    root.setAttribute('data-ui', settings.uiVariant)
    root.setAttribute('data-shell', settings.shell)
    root.setAttribute('data-scene', settings.scene)
    root.setAttribute('data-glass', settings.glassEffect ? 'on' : 'off')
    root.setAttribute('data-effect', settings.effectLevel)
    root.setAttribute('data-btn-shape', comp.buttonShape)
    root.setAttribute('data-chart-style', comp.chartStyle)
    root.setAttribute('data-chart-animate', comp.chartAnimate ? 'true' : 'false')
    root.setAttribute('data-badge-shape', comp.badgeShape)
    root.setAttribute('data-style-card-bg', sty.cardBg)
    root.setAttribute('data-style-mount', sty.mount)
    root.setAttribute('data-style-stagger', String(sty.stagger))
    root.setAttribute('data-style-hover', sty.hover)
    root.setAttribute('data-style-btn-hover', sty.buttonHover)
    root.setAttribute('data-style-mood', sty.mood)

    for (const [key, value] of Object.entries(themeToCssVars(settings))) {
      root.style.setProperty(key, value)
    }
    root.style.setProperty('--font-family', FONT_STACKS[settings.fontPreset])
    root.style.setProperty('--font-weight-body', settings.boldText ? '500' : '400')

    // Sub-theme overrides win: applied after the base tokens.
    for (const [key, value] of Object.entries(variantVars)) {
      root.style.setProperty(key, value)
    }

    root.classList.toggle('density-compact', settings.density === 'compact')
    root.classList.toggle('low-resource', settings.lowResourceMode)

    const mediaReduce = window.matchMedia('(prefers-reduced-motion: reduce)')
    const reduce = settings.reducedMotion || mediaReduce.matches
    root.classList.toggle('reduce-motion', reduce)

    if (firstRun.current) {
      firstRun.current = false
      return
    }
    if (reduce) return

    root.classList.add('theme-switching')
    const veilTimer = setTimeout(() => root.classList.remove('theme-switching'), 400)
    return () => {
      clearTimeout(veilTimer)
      root.classList.remove('theme-switching')
    }
  }, [settings])

  return <>{children}</>
}
