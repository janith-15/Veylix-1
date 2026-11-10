/**
 * Lightweight 2D particle field (Canvas 2D).
 * Used for `effectLevel = 'subtle'` — no WebGL, no dependencies.
 * Each theme gets its own `mode` so the fallback also changes identity:
 *   orbit       — drifting dots + connecting lines (Modern)
 *   nebula      — soft blurred glow dots (Dark)
 *   aurora-wave — soft ribbons drifting sideways (Light)
 *   grid        — scrolling perspective grid (Gaming)
 * Pauses when the tab is hidden.
 */
export interface ParticleOptions {
  primary: string
  secondary: string
  count?: number
  speed?: number
  mode?: 'orbit' | 'nebula' | 'aurora-wave' | 'grid'
}

export function createParticleField(canvas: HTMLCanvasElement, opts: ParticleOptions) {
  const ctx = canvas.getContext('2d')
  if (!ctx) return () => {}

  const dpr = Math.min(window.devicePixelRatio || 1, 1.5)
  let width = 0
  let height = 0
  let raf = 0
  let running = true

  interface P {
    x: number
    y: number
    z: number
    r: number
    vx: number
    vy: number
    hue: 'a' | 'b'
  }

  const area = () => width * height
  const targetCount = () =>
    opts.count ?? Math.max(24, Math.min(90, Math.round(area() / 26000)))

  let particles: P[] = []

  const seed = () => {
    const n = targetCount()
    particles = Array.from({ length: n }, () => ({
      x: Math.random() * width,
      y: Math.random() * height,
      z: 0.3 + Math.random() * 0.7,
      r: 0.6 + Math.random() * 2.2,
      vx: (Math.random() - 0.5) * 0.14,
      vy: (Math.random() - 0.5) * 0.14,
      hue: Math.random() > 0.5 ? 'a' : 'b',
    }))
  }

  const resize = () => {
    // Measure the parent wrapper: the canvas itself is still at its 300x150
    // HTML default on the first frame, so its own rect would lock the backing
    // store to that size while CSS stretched the bitmap to the viewport.
    const host = canvas.parentElement
    const rect = host ? host.getBoundingClientRect() : canvas.getBoundingClientRect()
    width = Math.max(1, rect.width)
    height = Math.max(1, rect.height)
    canvas.width = Math.round(width * dpr)
    canvas.height = Math.round(height * dpr)
    canvas.style.width = `${width}px`
    canvas.style.height = `${height}px`
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    seed()
  }

  /** Gaming fallback: scrolling perspective floor grid. */
  const drawGrid = (speed: number) => {
    const horizon = height * 0.42
    const spacing = 46
    const offset = (performance.now() * 0.03 * speed) % spacing

    ctx.strokeStyle = opts.primary
    ctx.globalAlpha = 0.16
    ctx.lineWidth = 1
    for (let y = horizon; y < height + spacing; y += spacing / 2) {
      const depth = (y - horizon) / (height - horizon || 1)
      ctx.globalAlpha = 0.05 + depth * 0.22
      ctx.beginPath()
      ctx.moveTo(0, y + offset * depth)
      ctx.lineTo(width, y + offset * depth)
      ctx.stroke()
    }
    // Converging verticals
    ctx.globalAlpha = 0.12
    const cx = width / 2
    for (let i = -14; i <= 14; i++) {
      ctx.beginPath()
      ctx.moveTo(cx + i * 18, horizon)
      ctx.lineTo(cx + i * (width / 12), height)
      ctx.stroke()
    }
    ctx.globalAlpha = 1
  }

  /** Light fallback: soft aurora ribbons drifting sideways. */
  const drawAurora = (speed: number) => {
    const t = performance.now() * 0.00012 * speed
    for (let i = 0; i < 3; i++) {
      const y0 = height * (0.28 + i * 0.14)
      const grad = ctx.createLinearGradient(0, 0, width, 0)
      const c = i % 2 === 0 ? opts.primary : opts.secondary
      grad.addColorStop(0, 'transparent')
      grad.addColorStop(0.5, c)
      grad.addColorStop(1, 'transparent')
      ctx.globalAlpha = 0.10 - i * 0.02
      ctx.strokeStyle = grad
      ctx.lineWidth = 60 + i * 10
      ctx.beginPath()
      for (let x = 0; x <= width; x += Math.max(8, width / 90)) {
        const y =
          y0 +
          Math.sin(x * 0.004 + t * 1.4 + i * 1.7) * 22 +
          Math.sin(x * 0.0011 - t * 0.9 + i) * 40
        if (x === 0) ctx.moveTo(x, y)
        else ctx.lineTo(x, y)
      }
      ctx.stroke()
    }
    ctx.globalAlpha = 1
  }

  const draw = () => {
    ctx.clearRect(0, 0, width, height)
    const speed = opts.speed ?? 1
    const mode = opts.mode ?? 'orbit'

    if (mode === 'grid') {
      drawGrid(speed)
      return
    }

    if (mode === 'aurora-wave') {
      drawAurora(speed)
      return
    }

    // Soft connecting lines (only for larger screens to save CPU)
    if (mode === 'orbit' && width > 700) {
      for (let i = 0; i < particles.length; i++) {
        for (let j = i + 1; j < particles.length; j++) {
          const a = particles[i]
          const b = particles[j]
          const dx = a.x - b.x
          const dy = a.y - b.y
          const d2 = dx * dx + dy * dy
          if (d2 < 15000) {
            const alpha = (1 - d2 / 15000) * 0.09
            ctx.strokeStyle = `rgba(140, 150, 220, ${alpha})`
            ctx.lineWidth = 0.6
            ctx.beginPath()
            ctx.moveTo(a.x, a.y)
            ctx.lineTo(b.x, b.y)
            ctx.stroke()
          }
        }
      }
    }

    for (const p of particles) {
      p.x += p.vx * speed * p.z
      p.y += p.vy * speed * p.z
      if (p.x < -20) p.x = width + 20
      if (p.x > width + 20) p.x = -20
      if (p.y < -20) p.y = height + 20
      if (p.y > height + 20) p.y = -20

      const color = p.hue === 'a' ? opts.primary : opts.secondary
      ctx.globalAlpha = 0.16 + p.z * 0.3
      ctx.fillStyle = color
      ctx.beginPath()
      if (mode === 'nebula') {
        // Soft glow blobs instead of crisp dots
        const g = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, p.r * p.z * 9)
        g.addColorStop(0, color)
        g.addColorStop(1, 'transparent')
        ctx.globalAlpha = 0.1 + p.z * 0.12
        ctx.fillStyle = g
        ctx.arc(p.x, p.y, p.r * p.z * 9, 0, Math.PI * 2)
      } else {
        ctx.arc(p.x, p.y, p.r * p.z, 0, Math.PI * 2)
      }
      ctx.fill()
    }
    ctx.globalAlpha = 1
  }

  const loop = () => {
    if (!running) return
    draw()
    raf = requestAnimationFrame(loop)
  }

  const onVisibility = () => {
    const hidden = document.hidden
    if (hidden) {
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

  /*
   * A window `resize` event never fires for the case that actually broke this:
   * the effect mounts before the shell has laid out, so the first `resize()`
   * measured 0 (or the 300x150 default) and nothing ever re-measured. Watching
   * the wrapper catches both the initial layout AND later sidebar/density
   * changes, so the backing store always matches the pixels on screen.
   */
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
