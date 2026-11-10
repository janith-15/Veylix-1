/**
 * Canvas 2D scene renderers — the "gela pena" effects.
 *
 * Every scene is a pure function of time: `(canvas, opts) => cleanup`. They share
 * one contract so `SceneBackground` can swap them without knowing what they draw:
 *
 *  - paint into the given canvas, never the DOM;
 *  - respect `opts.primary` / `opts.secondary` (theme tokens, never literals);
 *  - multiply motion by `opts.speed` so "calm" and "snappy" themes feel different;
 *  - pause when the tab is hidden and stop completely on cleanup;
 *  - clamp DPR to 1.5 — a 3x phone would otherwise paint 9x the pixels.
 *
 * These are deliberately Canvas 2D, not Three.js: eleven of them are light
 * enough that a WebGL context would cost more than it returns, and the bundle
 * already ships `three` for the four scenes that need real geometry.
 */
export interface SceneOptions {
  primary: string
  secondary: string
  /** Motion multiplier; 0.4 (Classic) … 1.6 (Gaming). */
  speed?: number
}

/** What a renderer returns: call it to stop and release everything. */
export type SceneCleanup = () => void

/** The shape every scene module exports. */
export type SceneRenderer = (canvas: HTMLCanvasElement, opts: SceneOptions) => SceneCleanup

/** Shared frame loop + resize/visibility plumbing for every scene. */
function animate(
  canvas: HTMLCanvasElement,
  draw: (ctx: CanvasRenderingContext2D, w: number, h: number, t: number) => void,
): SceneCleanup {
  const ctx = canvas.getContext('2d')
  if (!ctx) return () => {}

  const dpr = Math.min(window.devicePixelRatio || 1, 1.5)
  let width = 1
  let height = 1
  let raf = 0
  let running = true
  const t0 = performance.now()

  const resize = () => {
    /*
     * Measure the PARENT, not the canvas.
     *
     * `canvas` carries `width:100%; height:100%` from effects.css, so its own
     * rect is only correct once the browser has laid it out. On the very first
     * frame the element is still 300x150 (the HTML default) *and* its rect
     * reflects that default, so measuring ourselves leaves the backing store at
     * 300x150 for ever while CSS stretches the bitmap — the effect "runs" and
     * paints, and the user sees nothing but a blur.
     *
     * The host wrapper is a plain positioned div that is already full-size when
     * the effect mounts, so it is the reliable measurement.
     */
    const host = canvas.parentElement
    const rect = host ? host.getBoundingClientRect() : canvas.getBoundingClientRect()
    width = Math.max(1, rect.width)
    height = Math.max(1, rect.height)
    canvas.width = Math.round(width * dpr)
    canvas.height = Math.round(height * dpr)
    canvas.style.width = `${width}px`
    canvas.style.height = `${height}px`
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  }

  const loop = () => {
    if (!running) return
    ctx.clearRect(0, 0, width, height)
    draw(ctx, width, height, (performance.now() - t0) / 1000)
    raf = requestAnimationFrame(loop)
  }

  const onVisibility = () => {
    if (document.hidden) {
      running = false
      cancelAnimationFrame(raf)
    } else if (!running) {
      running = true
      loop()
    }
  }

  let resizeTimer: ReturnType<typeof setTimeout>
  const onResize = () => {
    clearTimeout(resizeTimer)
    resizeTimer = setTimeout(resize, 180)
  }

  resize()
  loop()
  window.addEventListener('resize', onResize)
  document.addEventListener('visibilitychange', onVisibility)

  // See the note in `particles.ts`: a window resize event never fires for the
  // mount-before-layout case, so the wrapper is observed directly.
  const hostEl = canvas.parentElement
  const ro = hostEl && typeof ResizeObserver !== 'undefined' ? new ResizeObserver(onResize) : null
  if (ro && hostEl) ro.observe(hostEl)

  return () => {
    running = false
    cancelAnimationFrame(raf)
    ro?.disconnect()
    window.removeEventListener('resize', onResize)
    document.removeEventListener('visibilitychange', onVisibility)
    clearTimeout(resizeTimer)
  }
}

/** Deterministic pseudo-random in [0,1) so a scene layout is stable per seed. */
function rng(seed: number) {
  let s = seed
  return () => {
    s = (s * 1664525 + 1013904223) % 4294967296
    return s / 4294967296
  }
}

/* ==================================================================
   LIGHT — aurora-wave lives in `particles.ts`; these two replace `rings`.
   ================================================================== */

/** Water caustics: overlapping interference rings drifting on a slow swell. */
export const lightCaustics: SceneRenderer = (canvas, opts) => {
  const rand = rng(4242)
  const orbs = Array.from({ length: 14 }, () => ({
    x: rand(),
    y: rand(),
    r: 90 + rand() * 170,
    dx: (rand() - 0.5) * 0.012,
    dy: (rand() - 0.5) * 0.012,
    ph: rand() * Math.PI * 2,
  }))

  return animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1
    for (let i = 0; i < orbs.length; i++) {
      const o = orbs[i]
      const x = (o.x + o.dx * t * speed + Math.sin(t * 0.2 + o.ph) * 0.02) * w
      const y = (o.y + o.dy * t * speed + Math.cos(t * 0.17 + o.ph) * 0.02) * h
      const pulse = 0.82 + Math.sin(t * 0.6 * speed + o.ph) * 0.18
      const g = ctx.createRadialGradient(x, y, 0, x, y, o.r * pulse)
      const c = i % 3 === 0 ? opts.secondary : opts.primary
      g.addColorStop(0, c)
      g.addColorStop(0.55, 'transparent')
      g.addColorStop(0.72, c)
      g.addColorStop(1, 'transparent')
      ctx.globalAlpha = 0.055
      ctx.fillStyle = g
      ctx.beginPath()
      ctx.arc(x, y, o.r * pulse, 0, Math.PI * 2)
      ctx.fill()
    }
    ctx.globalAlpha = 1
  })
}

/** Floating wireframe solids in soft light. */
export const lightGeometric: SceneRenderer = (canvas, opts) => {
  const rand = rng(909)
  const solids = Array.from({ length: 9 }, () => ({
    x: rand(),
    y: rand(),
    size: 26 + rand() * 46,
    sides: 3 + Math.floor(rand() * 3),
    spin: (rand() - 0.5) * 0.5,
    drift: (rand() - 0.5) * 0.006,
    ph: rand() * Math.PI * 2,
  }))

  return animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1
    for (const s of solids) {
      const x = s.x * w
      const y = (s.y + s.drift * t * speed) * h + Math.sin(t * 0.4 + s.ph) * 10
      const rot = t * s.spin * speed + s.ph
      ctx.globalAlpha = 0.11
      ctx.strokeStyle = s.sides % 2 === 0 ? opts.secondary : opts.primary
      ctx.lineWidth = 1.1
      ctx.beginPath()
      for (let i = 0; i <= s.sides; i++) {
        const a = rot + (i / s.sides) * Math.PI * 2
        const px = x + Math.cos(a) * s.size
        const py = y + Math.sin(a) * s.size
        if (i === 0) ctx.moveTo(px, py)
        else ctx.lineTo(px, py)
      }
      ctx.stroke()
    }
    ctx.globalAlpha = 1
  })
}

/* ==================================================================
   HACKING — terminal / CRT family
   ================================================================== */

/** CRT scanlines with a slow rolling refresh bar. */
export const scanlineCrt: SceneRenderer = (canvas, opts) =>
  animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1
    ctx.globalAlpha = 1

    // Horizontal scanlines
    ctx.fillStyle = opts.primary
    ctx.globalAlpha = 0.05
    for (let y = 0; y < h; y += 3) ctx.fillRect(0, y, w, 1)

    // Rolling refresh bar — the thing that makes a CRT feel alive
    const barY = ((t * 42 * speed) % (h + 160)) - 80
    const g = ctx.createLinearGradient(0, barY - 80, 0, barY + 80)
    g.addColorStop(0, 'transparent')
    g.addColorStop(0.5, opts.primary)
    g.addColorStop(1, 'transparent')
    ctx.globalAlpha = 0.07
    ctx.fillStyle = g
    ctx.fillRect(0, barY - 80, w, 160)

    // Faint vignette, phosphor bloom at the edges
    const v = ctx.createRadialGradient(w / 2, h / 2, Math.min(w, h) * 0.3, w / 2, h / 2, Math.max(w, h) * 0.75)
    v.addColorStop(0, 'transparent')
    v.addColorStop(1, opts.primary)
    ctx.globalAlpha = 0.045
    ctx.fillStyle = v
    ctx.fillRect(0, 0, w, h)
    ctx.globalAlpha = 1
  })

/** Streaming terminal log lines, typed on. */
export const classicTerminal: SceneRenderer = (canvas, opts) => {
  const rand = rng(1337)
  const cols = 6
  const rows = 18
  const lines: { col: number; row: number; width: number; at: number }[] = []
  for (let i = 0; i < cols * 3; i++) {
    lines.push({
      col: Math.floor(rand() * cols),
      row: Math.floor(rand() * rows),
      width: 18 + rand() * 72,
      at: rand() * 24,
    })
  }

  return animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1
    const cellW = w / cols
    const cellH = h / rows
    const now = t * speed

    for (const l of lines) {
      // Each line "types" itself in, holds, then fades and restarts.
      const cycle = 9
      const phase = ((now + l.at) % cycle) / cycle
      const typed = Math.min(1, phase / 0.45)
      const alpha = phase > 0.75 ? (1 - (phase - 0.75) / 0.25) * 0.22 : 0.22
      if (alpha <= 0) continue
      const x = l.col * cellW + 10
      const y = l.row * cellH + cellH / 2
      const len = (l.width / 100) * (cellW - 20) * typed

      ctx.globalAlpha = alpha
      ctx.fillStyle = l.row % 3 === 0 ? opts.secondary : opts.primary
      ctx.fillRect(x, y, len, 2)

      // Cursor block at the writing head
      if (typed < 1 && Math.floor(now * 3) % 2 === 0) {
        ctx.globalAlpha = 0.34
        ctx.fillRect(x + len + 3, y - 3, 5, 8)
      }
    }
    ctx.globalAlpha = 1
  })
}

/** Phosphor oscilloscope trace. */
export const classicOscilloscope: SceneRenderer = (canvas, opts) =>
  animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1
    const mid = h * 0.52
    const amp = h * 0.14

    // Graticule
    ctx.globalAlpha = 0.06
    ctx.strokeStyle = opts.primary
    ctx.lineWidth = 1
    for (let i = 1; i < 10; i++) {
      ctx.beginPath()
      ctx.moveTo(0, (h / 10) * i)
      ctx.lineTo(w, (h / 10) * i)
      ctx.stroke()
    }
    for (let i = 1; i < 16; i++) {
      ctx.beginPath()
      ctx.moveTo((w / 16) * i, 0)
      ctx.lineTo((w / 16) * i, h)
      ctx.stroke()
    }

    // Two traces, like a dual-channel scope
    const traces = [
      { freq: 2.1, phase: 0, alpha: 0.38, color: opts.primary, wob: 1.6 },
      { freq: 3.7, phase: 1.9, alpha: 0.2, color: opts.secondary, wob: 0.9 },
    ]
    for (const tr of traces) {
      ctx.globalAlpha = tr.alpha
      ctx.strokeStyle = tr.color
      ctx.lineWidth = 1.6
      ctx.beginPath()
      for (let x = 0; x <= w; x += 3) {
        const u = x / w
        const y =
          mid +
          Math.sin(u * Math.PI * 2 * tr.freq + t * tr.wob * speed + tr.phase) * amp +
          Math.sin(u * Math.PI * 2 * tr.freq * 2.3 + t * 0.7 * speed) * amp * 0.18
        if (x === 0) ctx.moveTo(x, y)
        else ctx.lineTo(x, y)
      }
      ctx.stroke()
    }
    ctx.globalAlpha = 1
  })

/* ==================================================================
   DARK
   ================================================================== */

/** Blurred bokeh orbs breathing in and out. */
export const subtleOrbs: SceneRenderer = (canvas, opts) => {
  const rand = rng(77)
  const orbs = Array.from({ length: 12 }, () => ({
    x: rand(),
    y: rand(),
    r: 70 + rand() * 190,
    ph: rand() * Math.PI * 2,
    rate: 0.18 + rand() * 0.3,
    drift: (rand() - 0.5) * 0.004,
  }))

  return animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1
    for (let i = 0; i < orbs.length; i++) {
      const o = orbs[i]
      const breath = 0.6 + Math.sin(t * o.rate * speed + o.ph) * 0.4
      const x = (o.x + o.drift * t * speed) * w
      const y = o.y * h
      const rr = o.r * breath
      const g = ctx.createRadialGradient(x, y, 0, x, y, rr)
      g.addColorStop(0, i % 2 === 0 ? opts.primary : opts.secondary)
      g.addColorStop(1, 'transparent')
      ctx.globalAlpha = 0.09 * breath
      ctx.fillStyle = g
      ctx.beginPath()
      ctx.arc(x, y, rr, 0, Math.PI * 2)
      ctx.fill()
    }
    ctx.globalAlpha = 1
  })
}

/** Stars falling through a dark field. */
export const darkStarfall: SceneRenderer = (canvas, opts) => {
  const rand = rng(31337)
  const stars = Array.from({ length: 130 }, () => ({
    x: rand(),
    y: rand(),
    z: 0.3 + rand() * 0.7,
    len: 8 + rand() * 26,
    speed: 0.02 + rand() * 0.05,
  }))

  return animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1
    for (let i = 0; i < stars.length; i++) {
      const s = stars[i]
      const y = ((s.y + s.speed * t * speed) % 1.2) * h
      const x = s.x * w
      const len = s.len * s.z
      ctx.globalAlpha = 0.1 + s.z * 0.26
      const g = ctx.createLinearGradient(x, y - len, x, y)
      const c = i % 5 === 0 ? opts.secondary : opts.primary
      g.addColorStop(0, 'transparent')
      g.addColorStop(1, c)
      ctx.strokeStyle = g
      ctx.lineWidth = 0.9 * s.z
      ctx.beginPath()
      ctx.moveTo(x, y - len)
      ctx.lineTo(x, y)
      ctx.stroke()
    }
    ctx.globalAlpha = 1
  })
}




/** Radar sweep with fading contact blips. */
export const darkStealthRadar: SceneRenderer = (canvas, opts) => {
  const rand = rng(5150)
  const blips = Array.from({ length: 9 }, () => ({
    a: rand() * Math.PI * 2,
    d: 0.15 + rand() * 0.8,
  }))

  return animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1
    const cx = w * 0.5
    const cy = h * 0.5
    const maxR = Math.min(w, h) * 0.42
    const sweep = (t * 0.55 * speed) % (Math.PI * 2)

    // Range rings
    ctx.strokeStyle = opts.primary
    ctx.lineWidth = 1
    for (let i = 1; i <= 4; i++) {
      ctx.globalAlpha = 0.05
      ctx.beginPath()
      ctx.arc(cx, cy, (maxR / 4) * i, 0, Math.PI * 2)
      ctx.stroke()
    }

    // Sweep wedge: 26 rays fading away from the leading edge
    for (let i = 0; i < 26; i++) {
      const a = sweep - i * 0.018
      ctx.globalAlpha = 0.16 * (1 - i / 26)
      ctx.strokeStyle = opts.primary
      ctx.lineWidth = 2
      ctx.beginPath()
      ctx.moveTo(cx, cy)
      ctx.lineTo(cx + Math.cos(a) * maxR, cy + Math.sin(a) * maxR)
      ctx.stroke()
    }

    // Contacts: brightest right after the sweep passes over them
    for (const b of blips) {
      let delta = sweep - b.a
      while (delta < 0) delta += Math.PI * 2
      const age = delta / (Math.PI * 2)
      if (age > 0.55) continue
      const alpha = (1 - age / 0.55) * 0.5
      const x = cx + Math.cos(b.a) * maxR * b.d
      const y = cy + Math.sin(b.a) * maxR * b.d
      ctx.globalAlpha = alpha
      ctx.fillStyle = opts.secondary
      ctx.beginPath()
      ctx.arc(x, y, 2.6, 0, Math.PI * 2)
      ctx.fill()

      ctx.globalAlpha = alpha * 0.35
      ctx.strokeStyle = opts.secondary
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.arc(x, y, 9 * (1 - age), 0, Math.PI * 2)
      ctx.stroke()
    }
    ctx.globalAlpha = 1
  })
}

/* ==================================================================
   MODERN / PRO — `particles` lives in `particles.ts` (the orbit field).
   ================================================================== */

/** Dotted world-map with pulsing hubs and traffic arcs. */
export const proWorldmap: SceneRenderer = (canvas, opts) => {
  const rand = rng(2010)
  // Rough continental clusters in normalised space, so it reads as a map
  // without shipping geographic data (a background is not an atlas).
  const hubs = [
    { x: 0.2, y: 0.34 }, { x: 0.27, y: 0.42 }, { x: 0.33, y: 0.3 },
    { x: 0.47, y: 0.32 }, { x: 0.53, y: 0.44 }, { x: 0.62, y: 0.36 },
    { x: 0.7, y: 0.5 }, { x: 0.78, y: 0.38 }, { x: 0.83, y: 0.55 },
    { x: 0.36, y: 0.62 }, { x: 0.5, y: 0.7 }, { x: 0.66, y: 0.68 },
  ]
  const dots = Array.from({ length: 220 }, () => ({
    x: rand(),
    y: 0.22 + rand() * 0.6,
    a: 0.05 + rand() * 0.1,
  }))
  const arcs = Array.from({ length: 7 }, () => ({
    from: Math.floor(rand() * hubs.length),
    to: Math.floor(rand() * hubs.length),
    ph: rand() * 6,
  }))

  return animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1

    // Landmass speckle
    ctx.fillStyle = opts.primary
    for (const d of dots) {
      ctx.globalAlpha = d.a
      ctx.fillRect(d.x * w, d.y * h, 1.4, 1.4)
    }

    // Traffic arcs between hubs
    for (const arc of arcs) {
      if (arc.from === arc.to) continue
      const a = hubs[arc.from]
      const b = hubs[arc.to]
      const x1 = a.x * w
      const y1 = a.y * h
      const x2 = b.x * w
      const y2 = b.y * h
      const mx = (x1 + x2) / 2
      const my = (y1 + y2) / 2 - Math.abs(x2 - x1) * 0.22
      const p = ((t * 0.22 * speed + arc.ph) % 1 + 1) % 1

      ctx.globalAlpha = 0.1
      ctx.strokeStyle = opts.secondary
      ctx.lineWidth = 0.8
      ctx.beginPath()
      ctx.moveTo(x1, y1)
      ctx.quadraticCurveTo(mx, my, x2, y2)
      ctx.stroke()

      // Travelling packet at parameter p along the same curve
      const qx = (1 - p) * (1 - p) * x1 + 2 * (1 - p) * p * mx + p * p * x2
      const qy = (1 - p) * (1 - p) * y1 + 2 * (1 - p) * p * my + p * p * y2
      ctx.globalAlpha = 0.55
      ctx.fillStyle = opts.secondary
      ctx.beginPath()
      ctx.arc(qx, qy, 2, 0, Math.PI * 2)
      ctx.fill()
    }

    // Hub pulses
    for (let i = 0; i < hubs.length; i++) {
      const hub = hubs[i]
      const x = hub.x * w
      const y = hub.y * h
      const pulse = (Math.sin(t * 1.4 * speed + i) + 1) / 2
      ctx.globalAlpha = 0.3 + pulse * 0.35
      ctx.fillStyle = opts.primary
      ctx.beginPath()
      ctx.arc(x, y, 2 + pulse * 1.6, 0, Math.PI * 2)
      ctx.fill()

      ctx.globalAlpha = 0.14 * (1 - pulse)
      ctx.strokeStyle = opts.primary
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.arc(x, y, 4 + pulse * 16, 0, Math.PI * 2)
      ctx.stroke()
    }
    ctx.globalAlpha = 1
  })
}

/** Light pulses racing along fibre buses. */
export const proFiber: SceneRenderer = (canvas, opts) => {
  const rand = rng(8800)
  const buses = Array.from({ length: 7 }, (_, i) => ({
    y: 0.14 + i * 0.12,
    tilt: (rand() - 0.5) * 0.16,
    pulses: Array.from({ length: 4 }, () => ({
      p: rand(),
      speed: 0.03 + rand() * 0.07,
      len: 30 + rand() * 90,
    })),
  }))

  return animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1
    for (let bi = 0; bi < buses.length; bi++) {
      const bus = buses[bi]
      const y0 = bus.y * h
      const y1 = y0 + bus.tilt * h

      // The fibre itself
      ctx.globalAlpha = 0.06
      ctx.strokeStyle = opts.primary
      ctx.lineWidth = 1
      ctx.beginPath()
      ctx.moveTo(0, y0)
      ctx.lineTo(w, y1)
      ctx.stroke()

      // Light pulses running along it
      for (const pulse of bus.pulses) {
        const p = ((pulse.p + pulse.speed * t * speed) % 1.4) - 0.2
        const headX = p * w
        const tailP = Math.max(0, p - pulse.len / w)
        const tailX = tailP * w
        if (headX < -10 || tailX > w + 10) continue

        const g = ctx.createLinearGradient(tailX, y0, headX, y1)
        g.addColorStop(0, 'transparent')
        g.addColorStop(1, bi % 2 === 0 ? opts.primary : opts.secondary)
        ctx.globalAlpha = 0.4
        ctx.strokeStyle = g
        ctx.lineWidth = 2
        ctx.beginPath()
        ctx.moveTo(tailX, y0 + (tailX / w) * (y1 - y0))
        ctx.lineTo(headX, y0 + (headX / w) * (y1 - y0))
        ctx.stroke()
      }
    }
    ctx.globalAlpha = 1
  })
}


/* ==================================================================
   GAMING — `grid` / `cyber-grid` live in `particles.ts` + `threeScene.ts`.
   ================================================================== */

/** Stars streaking past at warp speed. */
export const gamingWarp: SceneRenderer = (canvas, opts) => {
  const rand = rng(1984)
  const stars = Array.from({ length: 160 }, () => ({
    a: rand() * Math.PI * 2,
    d: rand() * 0.9 + 0.05,
    z: 0.2 + rand() * 0.8,
  }))

  return animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1
    const cx = w / 2
    const cy = h / 2
    const maxR = Math.hypot(w, h) * 0.55

    for (let i = 0; i < stars.length; i++) {
      const s = stars[i]
      // Distance grows then wraps, so stars rush outward from the centre
      const d = ((s.d + t * 0.09 * speed * s.z) % 1) + 0.02
      const r = d * maxR
      const prevD = Math.max(0.01, d - 0.045 * s.z * speed)
      const pr = prevD * maxR
      const x = cx + Math.cos(s.a) * r
      const y = cy + Math.sin(s.a) * r
      const px = cx + Math.cos(s.a) * pr
      const py = cy + Math.sin(s.a) * pr

      ctx.globalAlpha = 0.12 + d * 0.5
      ctx.strokeStyle = i % 7 === 0 ? opts.secondary : opts.primary
      ctx.lineWidth = 0.7 + d * 2
      ctx.beginPath()
      ctx.moveTo(px, py)
      ctx.lineTo(x, y)
      ctx.stroke()
    }

    // Core glow
    const g = ctx.createRadialGradient(cx, cy, 0, cx, cy, maxR * 0.3)
    g.addColorStop(0, opts.secondary)
    g.addColorStop(1, 'transparent')
    ctx.globalAlpha = 0.1
    ctx.fillStyle = g
    ctx.fillRect(0, 0, w, h)
    ctx.globalAlpha = 1
  })
}

/** Honeycomb of hex cells pulsing in waves. */
export const gamingHex: SceneRenderer = (canvas, opts) => {
  const size = 34

  return animate(canvas, (ctx, w, h, t) => {
    const speed = opts.speed ?? 1
    const hStep = size * 1.5
    const vStep = Math.sqrt(3) * size
    let row = 0

    for (let y = -vStep; y < h + vStep; y += vStep / 2) {
      const offset = row % 2 === 0 ? 0 : size * 1.5
      for (let x = -hStep; x < w + hStep; x += hStep * 2) {
        const cx = x + offset
        const key = Math.sin(cx * 0.004 + y * 0.006 + t * 0.9 * speed)
        const pulse = (key + 1) / 2
        ctx.globalAlpha = 0.03 + pulse * 0.09
        ctx.strokeStyle = pulse > 0.7 ? opts.secondary : opts.primary
        ctx.lineWidth = 1
        ctx.beginPath()
        for (let i = 0; i <= 6; i++) {
          const a = (Math.PI / 3) * i
          const px = cx + Math.cos(a) * size
          const py = y + Math.sin(a) * size
          if (i === 0) ctx.moveTo(px, py)
          else ctx.lineTo(px, py)
        }
        ctx.stroke()
      }
      row++
    }
    ctx.globalAlpha = 1
  })
}

/* ==================================================================
   REGISTRY — the single source of truth for "which id paints what".
   ================================================================== */

/**
 * Every animated scene this module can draw.
 *
 * `SceneBackground` consults this before mounting anything, so a scene id can
 * never silently resolve to "nothing": either it has a renderer here, or it is
 * drawn by Three.js in `threeScene.ts`, or the theme is `flat` on purpose.
 */
export const CANVAS_SCENES = {
  'scanline-crt': scanlineCrt,
  'classic-terminal': classicTerminal,
  'classic-oscilloscope': classicOscilloscope,
  'aurora-wave': null, // drawn by `particles.ts` (shares its particle field)
  'light-caustics': lightCaustics,
  'light-geometric': lightGeometric,
  'subtle-orbs': subtleOrbs,
  'dark-stealth-radar': darkStealthRadar,
  'dark-starfall': darkStarfall,
  particles: null, // `particles.ts` orbit field
  'pro-worldmap': proWorldmap,
  'pro-fiber': proFiber,
  'cyber-grid': null, // Three.js (GridHelper), with the 2D grid as fallback
  'gaming-warp': gamingWarp,
  'gaming-hex': gamingHex,
  orbit: null, // `particles.ts` orbit field
  nebula: null, // `particles.ts` nebula blobs
  grid: null, // `particles.ts` perspective grid
} as const satisfies Record<string, SceneRenderer | null>
