import { useEffect, useRef, useState } from 'react'
import { useThemeStore } from '../../theme/themeStore'
import type { SceneType } from '../../theme/themes'
import { createParticleField } from './particles'
import { CANVAS_SCENES } from './scenes'
import './effects.css'

/**
 * Scenes the shared particle field can draw (it owns one canvas and a handful
 * of modes). Everything else is either a dedicated renderer in `scenes.ts`, a
 * Three.js scene, or `flat`.
 *
 * `particles` is the Modern default and is drawn by the particle field's
 * `orbit` geometry — leaving it out of this list meant Modern's own default
 * scene resolved to NO renderer at all, so its canvas kept the 300x150 HTML
 * default and painted nothing.
 */
const PARTICLE_MODES = ['orbit', 'nebula', 'aurora-wave', 'grid'] as const
type ParticleMode = (typeof PARTICLE_MODES)[number]

/** Scene id → the particle-field mode that draws it. */
const SCENE_TO_MODE: Partial<Record<string, ParticleMode>> = {
  orbit: 'orbit',
  nebula: 'nebula',
  'aurora-wave': 'aurora-wave',
  grid: 'grid',
  particles: 'orbit',
}

/** Scenes Three.js has real geometry for — the richest renderer wins. */
const SCENES_3D = ['orbit', 'nebula', 'grid', 'cyber-grid', 'particles'] as const

function asParticleMode(scene: string): ParticleMode | null {
  return SCENE_TO_MODE[scene] ?? null
}

function has3D(scene: string): boolean {
  return (SCENES_3D as readonly string[]).includes(scene)
}

/** Everything the scene renderer needs. Deliberately primitives, not a store. */
export interface SceneLayerProps {
  scene: SceneType
  effectLevel: 'off' | 'subtle' | 'enhanced'
  primary: string
  secondary: string
  bgPage: string
  speed?: number
  lowResource?: boolean
  reducedMotion?: boolean
}

/**
 * The background renderer, as a PRESENTATIONAL component.
 *
 * It owns no state of its own beyond "is the 3D chunk ready", and it reads no
 * store: every value comes in as a prop. That is what lets the same renderer
 * paint the live app (via `SceneBackground`, which feeds it the committed
 * settings) and the Theme Studio preview (which feeds it the DRAFT), so the
 * preview can never disagree with what Save will actually produce.
 *
 *  - off      : nothing is mounted (true low-resource)
 *  - subtle   : Canvas 2D, dedicated renderer or the shared particle field
 *  - enhanced : Three.js when the scene has geometry, otherwise Canvas 2D
 *
 * Always pointer-events:none + aria-hidden so it can never block the UI.
 */
export function SceneLayer({
  scene,
  effectLevel,
  primary,
  secondary,
  bgPage,
  speed = 1,
  lowResource = false,
  reducedMotion = false,
}: SceneLayerProps) {
  const hostRef = useRef<HTMLDivElement>(null)
  const [threeReady, setThreeReady] = useState(false)

  const [mediaReduce, setMediaReduce] = useState(
    () => window.matchMedia('(prefers-reduced-motion: reduce)').matches,
  )

  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const onChange = (e: MediaQueryListEvent) => setMediaReduce(e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const motionOff = reducedMotion || mediaReduce
  const disabled = effectLevel === 'off' || lowResource || motionOff || scene === 'flat'

  /*
   * `enhanced` means "use the richest renderer available for THIS scene", not
   * "always use WebGL". Eleven scene ids have no Three.js geometry yet, so
   * asking for `enhanced` while on one of them used to mount a Three.js canvas
   * that drew nothing and leave the "Preparing 3D scene…" chip on screen for
   * ever — a loader that could never finish. Resolving the renderer here keeps
   * that honest: `true` only when there is something for WebGL to draw.
   */
  const wantThree = effectLevel === 'enhanced' && has3D(scene)
  const particleMode = asParticleMode(scene)


  // ---- render the active scene ------------------------------------------
  useEffect(() => {
    if (disabled) return
    const host = hostRef.current
    if (!host) return

    const canvas = document.createElement('canvas')
    host.replaceChildren(canvas)

    let cleanup: (() => void) | undefined
    let cancelled = false

    /**
     * One effect, three possible renderers, checked in richness order:
     *   1. Three.js — only for `enhanced` + a scene it has geometry for;
     *   2. the shared particle field — the four modes it owns;
     *   3. a dedicated Canvas 2D renderer from `scenes.ts`.
     *
     * Anything that reaches here has already been proven drawable, so the
     * "Preparing 3D scene…" chip can never be left waiting on a renderer that
     * does not exist.
     */
    const start = async () => {
      if (wantThree) {
        try {
          const mod = await import('./threeScene')
          if (cancelled || !hostRef.current) return
          host.replaceChildren()
          cleanup = mod.createThreeScene(hostRef.current, {
            scene,
            primary,
            secondary,
            bg: bgPage,
            speed,
          })
          setThreeReady(true)
          return
        } catch {
          // Chunk failed to load: fall through to Canvas 2D rather than blank.
          if (cancelled || !hostRef.current) return
          host.replaceChildren(canvas)
        }
      }

      if (particleMode) {
        cleanup = createParticleField(canvas, {
          primary,
          secondary,
          speed,
          mode: particleMode,
        })
        return
      }

      const renderer = CANVAS_SCENES[scene as keyof typeof CANVAS_SCENES]
      if (renderer) {
        cleanup = renderer(canvas, { primary, secondary, speed })
      }
    }

    setThreeReady(!wantThree)
    void start()

    return () => {
      cancelled = true
      cleanup?.()
      canvas.remove()
    }
  }, [disabled, wantThree, particleMode, scene, primary, secondary, bgPage, speed])

  if (disabled) return null

  const showLoader = wantThree && !threeReady

  return (
    <div className="scene-bg" aria-hidden="true" data-scene={effectLevel} data-scene-type={scene}>
      <div className="scene-bg__fallback" />
      <div ref={hostRef} style={{ position: 'absolute', inset: 0 }} />
      {showLoader && <div className="scene-bg__loader">Preparing 3D scene…</div>}
    </div>
  )
}

/**
 * App-wide background: reads the COMMITTED theme and feeds `SceneLayer`.
 *
 * The split exists so the Theme Studio preview can paint the DRAFT with the
 * exact same renderer. Keeping the store read here — and only here — means the
 * renderer itself stays pure and testable.
 */
export function SceneBackground() {
  const settings = useThemeStore((s) => s.settings)
  return (
    <SceneLayer
      scene={settings.scene}
      effectLevel={settings.effectLevel}
      primary={settings.primary}
      secondary={settings.secondary}
      bgPage={settings.bgPage}
      speed={settings.sceneSpeed}
      lowResource={settings.lowResourceMode}
    />
  )
}
