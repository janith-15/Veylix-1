/**
 * Lazy-loaded Three.js scene (only imported when effectLevel === 'enhanced').
 *
 * One background per theme — the scene is chosen by `scene` so switching
 * preset changes the 3D identity, not just the colour:
 *  - orbit : rotating wireframe icosahedron + halo   (Modern)
 *  - nebula: drifting volumetric blob + starfield    (Dark)
 *  - grid  : synthwave perspective floor grid        (Gaming)
 *  - flat  : no scene (caller should not import us)
 *
 * `rings` (Light) used to be here and was removed on 2026-11-06; the Light
 * family now lives in the Canvas 2D renderer (`scenes.ts`), and
 * `migrateScene()` rewrites any saved id to `aurora-wave`.
 *
 * Shared guardrails for every scene:
 *  - single WebGL context, DPR clamped to 1.5
 *  - paused when the tab is hidden
 *  - generic disposal (every geometry/material added via the scene is freed)
 */
import * as THREE from 'three'
import type { AnimatedSceneType, SceneType } from '../../theme/themes'

export interface ThreeSceneOptions {
  scene: SceneType
  primary: string
  secondary: string
  bg?: string
  /** Motion multiplier (per theme). */
  speed?: number
}

/** A scene builder returns its objects and a per-frame updater. */
type SceneBuild = (ctx: {
  scene: THREE.Scene
  group: THREE.Group
  primary: THREE.Color
  secondary: THREE.Color
}) => (t: number) => void

export function createThreeScene(host: HTMLElement, opts: ThreeSceneOptions): () => void {
  const width = () => host.clientWidth || window.innerWidth
  const height = () => host.clientHeight || window.innerHeight
  const speed = opts.speed ?? 1

  const scene = new THREE.Scene()
  const camera = new THREE.PerspectiveCamera(52, width() / height(), 0.1, 100)
  camera.position.z = 6.4

  const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true })
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5))
  renderer.setSize(width(), height())
  renderer.setClearColor(0x000000, 0)
  host.replaceChildren(renderer.domElement)

  const group = new THREE.Group()
  scene.add(group)

  const primary = new THREE.Color(opts.primary)
  const secondary = new THREE.Color(opts.secondary)
  const update = buildThreeScene(opts.scene, { scene, group, primary, secondary })

  let raf = 0
  let running = true
  const clock = new THREE.Clock()

  const animate = () => {
    if (!running) return
    const t = clock.getElapsedTime() * speed

    update(t)

    renderer.render(scene, camera)
    raf = requestAnimationFrame(animate)
  }

  const onVisibility = () => {
    if (document.hidden) {
      running = false
      cancelAnimationFrame(raf)
    } else if (!running) {
      running = true
      clock.getDelta()
      animate()
    }
  }

  let resizeTimer: ReturnType<typeof setTimeout>
  const onResize = () => {
    clearTimeout(resizeTimer)
    resizeTimer = setTimeout(() => {
      camera.aspect = width() / height()
      camera.updateProjectionMatrix()
      renderer.setSize(width(), height())
    }, 180)
  }

  animate()
  window.addEventListener('resize', onResize)
  document.addEventListener('visibilitychange', onVisibility)

  return () => {
    running = false
    cancelAnimationFrame(raf)
    clearTimeout(resizeTimer)
    window.removeEventListener('resize', onResize)
    document.removeEventListener('visibilitychange', onVisibility)

    // Generic disposal: free everything reachable from the scene graph.
    scene.traverse((obj) => {
      const mesh = obj as THREE.Mesh
      mesh.geometry?.dispose()
      const material = mesh.material as THREE.Material | THREE.Material[] | undefined
      if (Array.isArray(material)) material.forEach((m) => m.dispose())
      else material?.dispose()
    })
    scene.clear()

    renderer.dispose()
    renderer.domElement.remove()
  }
}

/* ------------------------------------------------------------------
   Scene: orbit — Modern. Wireframe icosahedron + vertex points + halo.
   ------------------------------------------------------------------ */
const buildOrbit: SceneBuild = ({ group, primary, secondary }) => {
  const geometry = new THREE.IcosahedronGeometry(1.85, 1)

  const wireframe = new THREE.LineSegments(
    new THREE.WireframeGeometry(geometry),
    new THREE.LineBasicMaterial({ color: primary, transparent: true, opacity: 0.55 }),
  )
  group.add(wireframe)

  const shell = new THREE.Mesh(
    new THREE.IcosahedronGeometry(1.72, 1),
    new THREE.MeshBasicMaterial({ color: primary, transparent: true, opacity: 0.05, depthWrite: false }),
  )
  group.add(shell)

  const pointGeometry = new THREE.BufferGeometry()
  pointGeometry.setAttribute(
    'position',
    new THREE.Float32BufferAttribute(Array.from(geometry.attributes.position.array as Float32Array), 3),
  )
  const points = new THREE.Points(
    pointGeometry,
    new THREE.PointsMaterial({
      color: secondary,
      size: 0.075,
      transparent: true,
      opacity: 0.85,
      sizeAttenuation: true,
    }),
  )
  group.add(points)

  // Particle halo
  const haloCount = 420
  const haloPositions = new Float32Array(haloCount * 3)
  for (let i = 0; i < haloCount; i++) {
    const r = 3.2 + Math.random() * 3.6
    const theta = Math.random() * Math.PI * 2
    const phi = Math.acos(2 * Math.random() - 1)
    haloPositions[i * 3] = r * Math.sin(phi) * Math.cos(theta)
    haloPositions[i * 3 + 1] = r * Math.sin(phi) * Math.sin(theta) * 0.6
    haloPositions[i * 3 + 2] = r * Math.cos(phi)
  }
  const haloGeometry = new THREE.BufferGeometry()
  haloGeometry.setAttribute('position', new THREE.BufferAttribute(haloPositions, 3))
  const halo = new THREE.Points(
    haloGeometry,
    new THREE.PointsMaterial({
      color: secondary,
      size: 0.06,
      transparent: true,
      opacity: 0.5,
      sizeAttenuation: true,
    }),
  )
  group.add(halo)

  return (t) => {
    group.rotation.y = t * 0.16
    group.rotation.x = Math.sin(t * 0.22) * 0.16
    group.position.y = Math.sin(t * 0.45) * 0.12
    halo.rotation.y = -t * 0.045
  }
}


/* ------------------------------------------------------------------
   Scene: nebula — Dark. Slow drifting blob + starfield, no geometry
   rotation so it reads as "deep and calm" rather than mechanical.
   ------------------------------------------------------------------ */
const buildNebula: SceneBuild = ({ scene, primary, secondary }) => {
  // Starfield
  const stars = 900
  const starPos = new Float32Array(stars * 3)
  for (let i = 0; i < stars; i++) {
    const r = 4 + Math.random() * 9
    const theta = Math.random() * Math.PI * 2
    const phi = Math.acos(2 * Math.random() - 1)
    starPos[i * 3] = r * Math.sin(phi) * Math.cos(theta)
    starPos[i * 3 + 1] = r * Math.sin(phi) * Math.sin(theta)
    starPos[i * 3 + 2] = r * Math.cos(phi)
  }
  const starGeometry = new THREE.BufferGeometry()
  starGeometry.setAttribute('position', new THREE.BufferAttribute(starPos, 3))
  const starField = new THREE.Points(
    starGeometry,
    new THREE.PointsMaterial({ color: 0xffffff, size: 0.035, transparent: true, opacity: 0.55 }),
  )
  scene.add(starField)

  // Layered translucent blobs (cheap fake volumetrics)
  const blobs: { mesh: THREE.Mesh; phase: number; baseY: number }[] = []
  const layers = [
    { r: 2.1, color: primary, opacity: 0.12, x: -0.9, y: 0.2, z: 0 },
    { r: 1.6, color: secondary, opacity: 0.1, x: 1.2, y: -0.4, z: -0.6 },
    { r: 2.6, color: primary, opacity: 0.05, x: 0.3, y: 0.5, z: -1.4 },
  ]
  layers.forEach((layer, i) => {
    const mesh = new THREE.Mesh(
      new THREE.IcosahedronGeometry(layer.r, 3),
      new THREE.MeshBasicMaterial({
        color: layer.color,
        transparent: true,
        opacity: layer.opacity,
        depthWrite: false,
        wireframe: i === 2,
      }),
    )
    mesh.position.set(layer.x, layer.y, layer.z)
    scene.add(mesh)
    blobs.push({ mesh, phase: i * 1.7, baseY: layer.y })
  })

  return (t) => {
    starField.rotation.y = t * 0.012
    for (const blob of blobs) {
      blob.mesh.position.y = blob.baseY + Math.sin(t * 0.24 + blob.phase) * 0.35
      blob.mesh.rotation.y = t * 0.05 + blob.phase
      blob.mesh.rotation.x = Math.sin(t * 0.14 + blob.phase) * 0.25
      const s = 1 + Math.sin(t * 0.3 + blob.phase) * 0.06
      blob.mesh.scale.setScalar(s)
    }
  }
}

/* ------------------------------------------------------------------
   Scene: rings — REMOVED.
   The soft concentric rings were dropped on 2026-11-06 (owner decision)
   and replaced by the `light-*` and `aurora-wave` scenes, which live in
   the Canvas 2D renderer (`scenes.ts`). `migrateScene()` in
   `theme/themes.ts` rewrites any saved `rings` id to `aurora-wave`.
   ------------------------------------------------------------------ */

/* ------------------------------------------------------------------
   Scene: grid — Gaming. Synthwave perspective floor + speed streaks.
   ------------------------------------------------------------------ */
const buildGrid: SceneBuild = ({ scene, group, primary, secondary }) => {
  const divisions = 44
  const grid = new THREE.GridHelper(60, divisions, primary, primary)
  grid.position.set(0, -2.2, -6)
  const gridMaterial = grid.material as THREE.Material
  gridMaterial.transparent = true
  gridMaterial.opacity = 0.35
  scene.add(grid)

  // Second, dimmer grid for depth
  const grid2 = new THREE.GridHelper(60, divisions / 2, secondary, secondary)
  grid2.position.set(0, -2.2, -6)
  const grid2Material = grid2.material as THREE.Material
  grid2Material.transparent = true
  grid2Material.opacity = 0.14
  scene.add(grid2)

  // Floating wireframe core
  const core = new THREE.LineSegments(
    new THREE.WireframeGeometry(new THREE.OctahedronGeometry(1.5, 0)),
    new THREE.LineBasicMaterial({ color: primary, transparent: true, opacity: 0.75 }),
  )
  core.position.y = 0.8
  group.add(core)

  // Speed streaks flying towards the camera
  const streakCount = 90
  const positions = new Float32Array(streakCount * 3)
  const speeds = new Float32Array(streakCount)
  for (let i = 0; i < streakCount; i++) {
    positions[i * 3] = (Math.random() - 0.5) * 18
    positions[i * 3 + 1] = (Math.random() - 0.5) * 9
    positions[i * 3 + 2] = -20 + Math.random() * 22
    speeds[i] = 4 + Math.random() * 10
  }
  const streakGeometry = new THREE.BufferGeometry()
  streakGeometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))
  const streaks = new THREE.Points(
    streakGeometry,
    new THREE.PointsMaterial({ color: secondary, size: 0.07, transparent: true, opacity: 0.85 }),
  )
  scene.add(streaks)

  return (t) => {
    // Scroll the floor towards the viewer (one cell = 60 / 44 units)
    const cell = 60 / divisions
    grid.position.z = -6 + (t * 6) % cell
    grid2.position.z = grid.position.z

    core.rotation.y = t * 0.6
    core.rotation.x = t * 0.25
    core.position.y = 0.8 + Math.sin(t * 1.1) * 0.18

    const attr = streakGeometry.getAttribute('position') as THREE.BufferAttribute
    for (let i = 0; i < streakCount; i++) {
      let z = attr.getZ(i) + speeds[i] * 0.06
      if (z > 2) z = -20
      attr.setZ(i, z)
    }
    attr.needsUpdate = true
  }
}

/** Dispatch table (declared after the builders so the references resolve). */
const BUILDERS: Partial<Record<AnimatedSceneType, SceneBuild>> = {
  orbit: buildOrbit,
  nebula: buildNebula,
  grid: buildGrid,
  'cyber-grid': buildGrid,
}

/**
 * Build the scene matching the theme.
 *
 * Three.js only paints the four scenes it actually has geometry for. Every
 * other scene id returns a no-op updater: the caller (`SceneBackground`) mounts
 * the Canvas 2D renderer for those instead, so a missing entry here is a
 * deliberate hand-off, never a blank screen.
 */
export function buildThreeScene(
  scene: SceneType,
  ctx: Parameters<SceneBuild>[0],
): (t: number) => void {
  if (scene === 'flat') return () => {}
  const builder = BUILDERS[scene as AnimatedSceneType]
  return builder ? builder(ctx) : () => {}
}

