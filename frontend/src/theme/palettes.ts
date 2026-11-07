/**
 * SUB-THEME PALETTES, ported from the UI design reference.
 *
 * **This file has no dependency on the design reference.** The hex values were
 * copied in by hand and are owned by this repo from here on: deleting the
 * reference folder cannot break a build, and nothing here imports from it.
 *
 * Each entry is a flat palette, not a component. `subthemes.ts` turns these
 * into full `SubTheme` objects by deriving the token set the UI reads
 * (`--bg-input`, `--bg-hover`, `--border`, status colours) from the six values
 * given. That keeps the ported data small and stops a palette from shipping a
 * half-finished token set.
 *
 * Naming: ids are kebab-case so they survive `[data-variant='…']` matching.
 */

/** The six values a palette actually specifies. */
export interface PaletteSpec {
  id: string
  name: string
  /** One-line character note, shown on the Theme Studio tile. */
  blurb: string
  /** Accent used for primary actions and active nav. */
  primary: string
  /** Secondary accent: gradients, charts, focus glow. */
  accent: string
  /** Page background. */
  bg: string
  /** Card background. */
  card: string
  /** Raised surface (inputs, table headers, hover fills). */
  surface: string
  /** Body text. */
  text: string
}

/**
 * Ten palettes per preset: the reference's own catalogue, plus neighbours in
 * the same colour family so each preset reaches ten without repeating itself.
 * A duplicate would render two identical tiles, which is worse than a gap.
 */
export const PALETTES: Record<string, PaletteSpec[]> = {
  modern: [
    { id: 'emerald-vault', name: 'Emerald Vault', blurb: 'Cyber finance green', primary: '#10b981', accent: '#059669', bg: '#091511', card: '#0f241d', surface: '#133026', text: '#ecfdf5' },
    { id: 'cyan-surge', name: 'Cyan Surge', blurb: 'Cloud stream cyan', primary: '#06b6d4', accent: '#0891b2', bg: '#08171f', card: '#0e2634', surface: '#143447', text: '#f0fdfa' },
    { id: 'indigo-prime', name: 'Indigo Prime', blurb: 'Enterprise indigo', primary: '#6366f1', accent: '#4f46e5', bg: '#0e1124', card: '#161a38', surface: '#1e234c', text: '#eef2ff' },
    { id: 'violet-pulse', name: 'Violet Pulse', blurb: 'Deep tech violet', primary: '#8b5cf6', accent: '#7c3aed', bg: '#120f26', card: '#1c173c', surface: '#251f4f', text: '#f5f3ff' },
    { id: 'sky-velocity', name: 'Sky Velocity', blurb: 'Ultra fast sky', primary: '#38bdf8', accent: '#0284c7', bg: '#081421', card: '#0e2034', surface: '#152d49', text: '#f0f9ff' },
    { id: 'teal-matrix', name: 'Teal Matrix', blurb: 'Encrypted teal', primary: '#14b8a6', accent: '#0d9488', bg: '#071616', card: '#0e2524', surface: '#143433', text: '#f0fdfa' },
    { id: 'blue-nordic', name: 'Blue Nordic', blurb: 'Global CDN blue', primary: '#2563eb', accent: '#1d4ed8', bg: '#0a1224', card: '#101d3a', surface: '#172952', text: '#eff6ff' },
    { id: 'slate-titan', name: 'Slate Titan', blurb: 'Minimal pro slate', primary: '#64748b', accent: '#475569', bg: '#0c1017', card: '#151b26', surface: '#1e2636', text: '#f8fafc' },
    { id: 'amber-forge', name: 'Amber Forge', blurb: 'Warm industrial amber', primary: '#f59e0b', accent: '#b45309', bg: '#12100a', card: '#1d1810', surface: '#2a2216', text: '#fffbeb' },
    { id: 'rose-signal', name: 'Rose Signal', blurb: 'Alert rose', primary: '#f43f5e', accent: '#be123c', bg: '#140a0e', card: '#22121a', surface: '#301a24', text: '#fff1f2' },
  ],
  dark: [
    { id: 'obsidian-stealth', name: 'Obsidian Stealth', blurb: 'Zero trace', primary: '#38bdf8', accent: '#818cf8', bg: '#06080c', card: '#0e121a', surface: '#141a24', text: '#f8fafc' },
    { id: 'midnight-void', name: 'Midnight Void', blurb: 'Dark orbit', primary: '#a855f7', accent: '#c084fc', bg: '#05040a', card: '#0e0b1c', surface: '#17122e', text: '#faf5ff' },
    { id: 'carbon-graphite', name: 'Carbon Graphite', blurb: 'Carbon fibre', primary: '#f59e0b', accent: '#d97706', bg: '#0b0c0e', card: '#15171c', surface: '#20232a', text: '#fffbeb' },
    { id: 'phantom-black', name: 'Phantom Black', blurb: 'Ghost node', primary: '#f43f5e', accent: '#e11d48', bg: '#0a0507', card: '#170c10', surface: '#26141c', text: '#fff1f2' },
    { id: 'abyssal-deep', name: 'Abyssal Deep', blurb: 'Trench link', primary: '#2dd4bf', accent: '#0f766e', bg: '#030d0d', card: '#081a1b', surface: '#0e2729', text: '#f0fdf4' },
    { id: 'onyx-gold', name: 'Onyx Gold', blurb: 'VIP vault', primary: '#fbbf24', accent: '#f59e0b', bg: '#0a0905', card: '#18150c', surface: '#242013', text: '#fefce8' },
    { id: 'eclipse-mono', name: 'Eclipse Mono', blurb: 'Pure monolith', primary: '#e2e8f0', accent: '#94a3b8', bg: '#080808', card: '#141414', surface: '#212121', text: '#f8fafc' },
    { id: 'shadow-infrared', name: 'Shadow Infrared', blurb: 'Covert ops', primary: '#ef4444', accent: '#b91c1c', bg: '#0d0404', card: '#1a0909', surface: '#2a0f0f', text: '#fef2f2' },
    { id: 'deep-emerald', name: 'Deep Emerald', blurb: 'Submerged green', primary: '#10b981', accent: '#047857', bg: '#040c09', card: '#0a1712', surface: '#0f231a', text: '#ecfdf5' },
    { id: 'cobalt-night', name: 'Cobalt Night', blurb: 'Cold cobalt', primary: '#3b82f6', accent: '#1d4ed8', bg: '#050914', card: '#0b1526', surface: '#111e33', text: '#eff6ff' },
  ],
  light: [
    { id: 'arctic-cloud', name: 'Arctic Cloud', blurb: 'Daylight pro', primary: '#0284c7', accent: '#0369a1', bg: '#f8fafc', card: '#ffffff', surface: '#f1f5f9', text: '#0f172a' },
    { id: 'pure-paper', name: 'Pure Paper', blurb: 'Clean doc', primary: '#4f46e5', accent: '#4338ca', bg: '#fcfcfc', card: '#ffffff', surface: '#f4f4f5', text: '#18181b' },
    { id: 'frost-mint', name: 'Frost Mint', blurb: 'Cool mint', primary: '#059669', accent: '#047857', bg: '#f0fdf9', card: '#ffffff', surface: '#e6faf3', text: '#064e3b' },
    { id: 'lilac-mist', name: 'Lilac Mist', blurb: 'Soft lilac', primary: '#7c3aed', accent: '#6d28d9', bg: '#faf5ff', card: '#ffffff', surface: '#f3e8ff', text: '#581c87' },
    { id: 'rose-quartz', name: 'Rose Quartz', blurb: 'Pastel ruby', primary: '#e11d48', accent: '#be123c', bg: '#fff1f2', card: '#ffffff', surface: '#ffe4e6', text: '#881337' },
    { id: 'sandstone-modern', name: 'Sandstone Modern', blurb: 'Neutral light', primary: '#78716c', accent: '#57534e', bg: '#fafaf9', card: '#ffffff', surface: '#f5f5f4', text: '#292524' },
    { id: 'ocean-breeze', name: 'Ocean Breeze', blurb: 'Sea blue', primary: '#0ea5e9', accent: '#0284c7', bg: '#f0f9ff', card: '#ffffff', surface: '#e0f2fe', text: '#0c4a6e' },
    { id: 'apricot-day', name: 'Apricot Day', blurb: 'Warm apricot', primary: '#f97316', accent: '#ea580c', bg: '#fff7ed', card: '#ffffff', surface: '#ffedd5', text: '#7c2d12' },
    { id: 'graphite-paper', name: 'Graphite Paper', blurb: 'Ink on paper', primary: '#334155', accent: '#1e293b', bg: '#f1f5f9', card: '#ffffff', surface: '#e2e8f0', text: '#0f172a' },
    { id: 'spring-leaf', name: 'Spring Leaf', blurb: 'Fresh green', primary: '#65a30d', accent: '#4d7c0f', bg: '#f7fee7', card: '#ffffff', surface: '#ecfccb', text: '#365314' },
  ],
  classic: [
    { id: 'classic-blue', name: 'Classic Blue', blurb: 'Sysadmin default', primary: '#2563eb', accent: '#1d4ed8', bg: '#f2f4f8', card: '#ffffff', surface: '#e8edf5', text: '#0f172a' },
    { id: 'steel-navy', name: 'Steel Navy', blurb: 'Rack panel navy', primary: '#0284c7', accent: '#0369a1', bg: '#0b1626', card: '#13233c', surface: '#1d3356', text: '#f0f9ff' },
    { id: 'telecom-gray', name: 'Telecom Gray', blurb: 'Rack mount gray', primary: '#94a3b8', accent: '#64748b', bg: '#111317', card: '#1a1d24', surface: '#272c37', text: '#f8fafc' },
    { id: 'bsd-crimson', name: 'BSD Crimson', blurb: 'FreeBSD daemon', primary: '#dc2626', accent: '#b91c1c', bg: '#160808', card: '#281111', surface: '#3d1a1a', text: '#fee2e2' },
    { id: 'debian-ruby', name: 'Debian Ruby', blurb: 'Debian spiral', primary: '#e11d48', accent: '#be123c', bg: '#16080e', card: '#27101b', surface: '#3c182a', text: '#ffe4e6' },
    { id: 'arch-cyan', name: 'Arch Cyan', blurb: 'Rolling release', primary: '#0ea5e9', accent: '#0284c7', bg: '#0a1622', card: '#102235', surface: '#193450', text: '#f0f9ff' },
    { id: 'unix-forest', name: 'Unix Forest', blurb: 'Terminal forest', primary: '#16a34a', accent: '#15803d', bg: '#f0fdf4', card: '#ffffff', surface: '#dcfce7', text: '#14532d' },
    { id: 'mainframe-amber', name: 'Mainframe Amber', blurb: 'Big iron amber', primary: '#d97706', accent: '#b45309', bg: '#fef3c7', card: '#fffbeb', surface: '#fde68a', text: '#78350f' },
    { id: 'blueprint', name: 'Blueprint', blurb: 'Drafting blue', primary: '#1e40af', accent: '#1e3a8a', bg: '#eff6ff', card: '#ffffff', surface: '#dbeafe', text: '#172554' },
    { id: 'slate-admin', name: 'Slate Admin', blurb: 'Neutral admin', primary: '#475569', accent: '#334155', bg: '#f8fafc', card: '#ffffff', surface: '#f1f5f9', text: '#0f172a' },
  ],
  gaming: [
    { id: 'cyberpunk-neon', name: 'Cyberpunk Neon', blurb: 'Night city', primary: '#f43f5e', accent: '#06b6d4', bg: '#080512', card: '#140c2b', surface: '#1f1342', text: '#fff1f2' },
    { id: 'tokyo-drift', name: 'Tokyo Drift', blurb: 'Synthwave', primary: '#d946ef', accent: '#8b5cf6', bg: '#0d041a', card: '#1b0933', surface: '#2b0f52', text: '#fae8ff' },
    { id: 'razer-matrix', name: 'Razer Matrix', blurb: 'Zero ping', primary: '#10b981', accent: '#34d399', bg: '#040d07', card: '#091c10', surface: '#0f2f1b', text: '#ecfdf5' },
    { id: 'crimson-rog', name: 'Crimson ROG', blurb: 'Overclocked', primary: '#ef4444', accent: '#f87171', bg: '#0d0404', card: '#1c0909', surface: '#2e0f0f', text: '#fef2f2' },
    { id: 'hyper-solar', name: 'Hyper Solar', blurb: 'Plasma core', primary: '#f97316', accent: '#fbbf24', bg: '#120702', card: '#241006', surface: '#381a0b', text: '#fff7ed' },
    { id: 'quantum-glow', name: 'Quantum Glow', blurb: 'Sub-1ms ping', primary: '#38bdf8', accent: '#a855f7', bg: '#070a18', card: '#101530', surface: '#182047', text: '#f0f9ff' },
    { id: 'toxic-hazard', name: 'Toxic Hazard', blurb: 'Bio surge', primary: '#84cc16', accent: '#65a30d', bg: '#070f03', card: '#102107', surface: '#1a350c', text: '#f7fee7' },
    { id: 'valkyrie-gold', name: 'Valkyrie Gold', blurb: 'Apex champion', primary: '#fbbf24', accent: '#f59e0b', bg: '#0f0c05', card: '#211a0c', surface: '#352a13', text: '#fefce8' },
    { id: 'neon-ice', name: 'Neon Ice', blurb: 'Frozen arena', primary: '#67e8f9', accent: '#22d3ee', bg: '#04090f', card: '#0a1522', surface: '#102033', text: '#ecfeff' },
    { id: 'ultra-violate', name: 'Ultra Violate', blurb: 'Arcade violet', primary: '#c084fc', accent: '#9333ea', bg: '#0a0410', card: '#160a24', surface: '#211038', text: '#faf5ff' },
  ],
  hacking: [
    { id: 'terminal-green', name: 'Terminal Green', blurb: 'Phosphor green shell', primary: '#22c55e', accent: '#16a34a', bg: '#051208', card: '#09210f', surface: '#103319', text: '#dcfce7' },
    { id: 'amber-console', name: 'Amber Console', blurb: 'Vintage VT100', primary: '#f59e0b', accent: '#d97706', bg: '#120d03', card: '#211707', surface: '#33230a', text: '#fef3c7' },
    { id: 'cyan-shell', name: 'Cyan Shell', blurb: 'Cold oscilloscope', primary: '#3ddbff', accent: '#0a7f9c', bg: '#03080c', card: '#07141b', surface: '#0a1c25', text: '#d9f6ff' },
    { id: 'matrix-rain', name: 'Matrix Rain', blurb: 'Cascading green', primary: '#4ade80', accent: '#22c55e', bg: '#020a04', card: '#06140a', surface: '#0b1f11', text: '#dcfce7' },
    { id: 'red-alert', name: 'Red Alert', blurb: 'Intrusion red', primary: '#ff5c6c', accent: '#b91c1c', bg: '#0e0304', card: '#1a0709', surface: '#260b0e', text: '#ffe4e6' },
    { id: 'ice-blue', name: 'Ice Blue', blurb: 'Frosted terminal', primary: '#7dd3fc', accent: '#38bdf8', bg: '#040a12', card: '#0a1622', surface: '#0f2233', text: '#e0f2fe' },
    { id: 'purple-payload', name: 'Purple Payload', blurb: 'Exploit violet', primary: '#c084fc', accent: '#a855f7', bg: '#0a0414', card: '#150a22', surface: '#201030', text: '#f5f3ff' },
    { id: 'gold-root', name: 'Gold Root', blurb: 'Root access gold', primary: '#fbbf24', accent: '#d97706', bg: '#0c0902', card: '#171207', surface: '#241c0b', text: '#fefce8' },
    { id: 'mono-tty', name: 'Mono TTY', blurb: 'Bare monochrome', primary: '#e5e5e5', accent: '#a3a3a3', bg: '#0a0a0a', card: '#141414', surface: '#1f1f1f', text: '#fafafa' },
    { id: 'signal-teal', name: 'Signal Teal', blurb: 'Packet trace teal', primary: '#2dd4bf', accent: '#0d9488', bg: '#030f0e', card: '#071d1b', surface: '#0c2b28', text: '#f0fdfa' },
  ],
}

/**
 * Darken or lighten a hex colour by `amount` (-1 … 1).
 *
 * Palette data is deliberately six values, so the remaining tokens have to be
 * derived rather than guessed per entry. Doing it arithmetically keeps every
 * palette consistent: `--bg-hover` is always "the surface, nudged away from the
 * background", never an arbitrary seventh hex somebody forgot to pick.
 */
export function shift(hex: string, amount: number): string {
  const h = hex.replace('#', '')
  if (h.length !== 6) return hex
  const num = Number.parseInt(h, 16)
  const r = (num >> 16) & 255
  const g = (num >> 8) & 255
  const b = num & 255
  const move = (c: number) =>
    Math.max(0, Math.min(255, Math.round(amount >= 0 ? c + (255 - c) * amount : c * (1 + amount))))
  return `#${((move(r) << 16) | (move(g) << 8) | move(b)).toString(16).padStart(6, '0')}`
}

/** `true` when the background is dark, so text/lifts go the right way. */
export function isDark(hex: string): boolean {
  const h = hex.replace('#', '')
  const num = Number.parseInt(h, 16)
  const r = (num >> 16) & 255
  const g = (num >> 8) & 255
  const b = num & 255
  // Rec. 601 luma — good enough to pick a direction, and cheap.
  return (0.299 * r + 0.587 * g + 0.114 * b) / 255 < 0.45
}


