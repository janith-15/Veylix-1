import type { ThemePreset, UiVariant, SceneType, ShellType, ButtonShape } from './themes'
import { PALETTES, isDark, shift, type PaletteSpec } from './palettes'

/**
 * Sub-themes. Every main theme ships its own variants; each variant is a
 * coherent bundle of overrides applied on top of the preset defaults.
 * Keeping them declarative means Theme Studio can render them as tiles and
 * the CSS only has to read [data-variant].
 */
export type SubThemeId =
  | 'soft' | 'aurora' | 'minimal' | 'frosted' | 'mint' | 'sakura' | 'candy' | 'espresso'
  | 'deep-glass' | 'midnight' | 'void' | 'carbon'
  | 'nord' | 'dracula' | 'tokyo-night' | 'catppuccin'
  | 'editorial' | 'paper' | 'clean' | 'soft-grey'
  | 'latte' | 'solar-light' | 'one-light' | 'rose-dawn'
  | 'flat-admin' | 'compact' | 'legacy'
  | 'high-contrast' | 'warm-admin' | 'indigo-admin' | 'green-admin' | 'maroon-admin'
  | 'neon-hud' | 'cyberpunk' | 'terminal' | 'retrowave' | 'gruvbox' | 'crimson' | 'acid' | 'amber-crt'
  | 'terminal-green' | 'amber-console' | 'cyan-shell'

export interface SubTheme {
  id: SubThemeId
  name: string
  /** One-line description shown in Theme Studio. */
  blurb: string
  preset: ThemePreset
  /** CSS variables applied when this variant is active. */
  vars: Record<string, string>
  /** Attribute values driven by this variant. */
  uiVariant: UiVariant
  scene: SceneType
  shell: ShellType
  buttonShape: ButtonShape
}

export const SUB_THEMES: Record<ThemePreset, SubTheme[]> = {
  modern: [
    {
      id: 'soft',
      name: 'Soft layers',
      blurb: 'Rounded, floaty, gentle glow',
      preset: 'modern',
      uiVariant: 'soft', scene: 'orbit', shell: 'floating', buttonShape: 'pill',
      vars: { '--c-primary': '#5b5bd6', '--c-secondary': '#06b6d4', '--radius': '12px' },
    },
    {
      id: 'aurora',
      name: 'Aurora',
      blurb: 'Violet to teal gradient wash',
      preset: 'modern',
      uiVariant: 'soft', scene: 'nebula', shell: 'floating', buttonShape: 'pill',
      vars: {
        '--c-primary': '#7c3aed', '--c-primary-hover': '#6d28d9', '--c-secondary': '#22d3ee',
        '--radius': '16px', '--bg-page': '#f4f2fd',
      },
    },
    {
      id: 'minimal',
      name: 'Minimal',
      blurb: 'Flat surfaces, hairline borders',
      preset: 'modern',
      uiVariant: 'soft', scene: 'aurora-wave', shell: 'edge', buttonShape: 'soft',
      vars: {
        '--c-primary': '#4b5563', '--c-primary-hover': '#374151', '--c-secondary': '#9ca3af',
        '--radius': '6px', '--border': '#e5e7eb', '--shadow-md': 'none',
      },
    },
    {
      id: 'frosted',
      name: 'Frosted',
      blurb: 'Heavier blur, cool tint',
      preset: 'modern',
      uiVariant: 'glass', scene: 'nebula', shell: 'floating', buttonShape: 'pill',
      vars: {
        '--c-primary': '#2f6ce4', '--c-primary-hover': '#2456c4', '--c-secondary': '#7dd3fc',
        '--radius': '18px', '--glass-blur': '24px', '--bg-page': '#eef2fb',
      },
    },
    {
      id: 'mint',
      name: 'Fresh Mint',
      blurb: 'Spotify-green freshness, cool cyan',
      preset: 'modern',
      uiVariant: 'soft', scene: 'aurora-wave', shell: 'floating', buttonShape: 'pill',
      vars: {
        '--c-primary': '#15803d', '--c-primary-hover': '#166534', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#06b6d4', '--c-secondary-contrast': '#062f3d',
        '--bg-page': '#f1faf5', '--bg-surface': '#e7f5ee', '--bg-hover': '#dceee4',
        '--border': '#d5eae0',
        '--success': '#16a34a',
        '--radius': '12px',
      },
    },
    {
      id: 'sakura',
      name: 'Sakura',
      blurb: 'Soft cherry-blossom pastel',
      preset: 'modern',
      uiVariant: 'soft', scene: 'aurora-wave', shell: 'floating', buttonShape: 'pill',
      vars: {
        '--c-primary': '#db2777', '--c-primary-hover': '#be185d', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#9d174d', '--c-secondary-contrast': '#ffffff',
        '--bg-page': '#fdf2f8', '--bg-surface': '#fce7f3', '--bg-hover': '#f9d8e9',
        '--border': '#f9c9e0',
        '--radius': '12px',
      },
    },
    {
      id: 'candy',
      name: 'Candy Pop',
      blurb: 'Violet-to-pink, bold and playful',
      preset: 'modern',
      uiVariant: 'soft', scene: 'orbit', shell: 'floating', buttonShape: 'pill',
      vars: {
        '--c-primary': '#9333ea', '--c-primary-hover': '#7e22ce', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#db2777', '--c-secondary-contrast': '#ffffff',
        '--bg-page': '#faf5ff', '--bg-surface': '#f3e8ff', '--bg-hover': '#e9d5ff',
        '--border': '#e9d5ff',
        '--radius': '12px',
      },
    },
    {
      id: 'espresso',
      name: 'Espresso',
      blurb: 'Warm caf� browns, teal counterpoint',
      preset: 'modern',
      uiVariant: 'soft', scene: 'orbit', shell: 'floating', buttonShape: 'soft',
      vars: {
        '--c-primary': '#9a5b1b', '--c-primary-hover': '#7e4a14', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#0f766e', '--c-secondary-contrast': '#ffffff',
        '--bg-page': '#faf5ef', '--bg-card': '#fffdf8', '--bg-surface': '#f2eae0', '--bg-hover': '#eee2d2',
        '--border': '#e8dccb',
        '--radius': '10px',
      },
    },
  ],
  dark: [
    {
      id: 'deep-glass',
      name: 'Deep glass',
      blurb: 'Translucent panels, violet edge light',
      preset: 'dark',
      uiVariant: 'glass', scene: 'nebula', shell: 'edge', buttonShape: 'soft',
      vars: { '--c-primary': '#7c3aed', '--c-secondary': '#22d3ee', '--radius': '12px' },
    },
    {
      id: 'midnight',
      name: 'Midnight',
      blurb: 'True black, no blur',
      preset: 'dark',
      uiVariant: 'flat', scene: 'aurora-wave', shell: 'edge', buttonShape: 'square',
      vars: {
        '--c-primary': '#64748b', '--c-primary-hover': '#7f8fa3', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#5b6478', '--c-secondary-contrast': '#ffffff',
        '--bg-page': '#000000', '--bg-card': '#0a0a0c', '--bg-sidebar': '#050506',
        '--border': '#1c1c20', '--radius': '4px',
      },
    },
    {
      id: 'void',
      name: 'Void',
      blurb: 'Purple-black, saturated neon',
      preset: 'dark',
      uiVariant: 'neon', scene: 'grid', shell: 'hud', buttonShape: 'clipped',
      vars: {
        '--c-primary': '#a855f7', '--c-primary-contrast': '#170a26', '--c-secondary': '#22d3ee',
        '--bg-page': '#07030d', '--bg-card': '#100820', '--border': '#251440',
      },
    },
    {
      id: 'carbon',
      name: 'Carbon',
      blurb: 'Neutral greys, zero colour',
      preset: 'dark',
      uiVariant: 'glass', scene: 'orbit', shell: 'edge', buttonShape: 'soft',
      vars: {
        '--c-primary': '#a1a1aa', '--c-primary-contrast': '#0b0b0d', '--c-secondary': '#71717a',
        '--c-secondary-contrast': '#ffffff',
        '--bg-page': '#0b0b0d', '--bg-card': '#141417', '--border': '#26262b',
      },
    },
    {
      id: 'nord',
      name: 'Nord',
      blurb: 'Arctic frost � official Nord palette',
      preset: 'dark',
      uiVariant: 'glass', scene: 'orbit', shell: 'edge', buttonShape: 'soft',
      vars: {
        '--c-primary': '#88c0d0', '--c-primary-hover': '#8fbcbb', '--c-primary-contrast': '#2e3440',
        '--c-secondary': '#81a1c1', '--c-secondary-contrast': '#2e3440',
        '--bg-page': '#2e3440', '--bg-sidebar': '#2a3039', '--bg-card': '#3b4252',
        '--bg-surface': '#434c5e', '--bg-input': '#3b4252', '--bg-hover': '#434c5e',
        '--text': '#eceff4', '--text-muted': '#d8dee9',
        '--border': '#434c5e', '--border-strong': '#4c566a',
        '--success': '#a3be8c', '--warning': '#ebcb8b', '--error': '#bf616a', '--info': '#5e81ac',
        '--radius': '10px',
      },
    },
    {
      id: 'dracula',
      name: 'Dracula',
      blurb: 'The famous purple � official Dracula palette',
      preset: 'dark',
      uiVariant: 'glass', scene: 'nebula', shell: 'edge', buttonShape: 'soft',
      vars: {
        '--c-primary': '#bd93f9', '--c-primary-hover': '#d3bfff', '--c-primary-contrast': '#282a36',
        '--c-secondary': '#ff79c6', '--c-secondary-contrast': '#282a36',
        '--bg-page': '#282a36', '--bg-sidebar': '#22232e', '--bg-card': '#2f3142',
        '--bg-surface': '#44475a', '--bg-input': '#343746', '--bg-hover': '#44475a',
        '--text': '#f8f8f2', '--text-muted': '#8c93b8',
        '--border': '#44475a', '--border-strong': '#6272a4',
        '--success': '#50fa7b', '--warning': '#f1fa8c', '--error': '#ff5555', '--info': '#8be9fd',
        '--radius': '10px',
      },
    },
    {
      id: 'tokyo-night',
      name: 'Tokyo Night',
      blurb: 'Downtown Tokyo lights over deep indigo',
      preset: 'dark',
      uiVariant: 'glass', scene: 'aurora-wave', shell: 'edge', buttonShape: 'soft',
      vars: {
        '--c-primary': '#7aa2f7', '--c-primary-hover': '#8fb0ff', '--c-primary-contrast': '#1a1b26',
        '--c-secondary': '#bb9af7', '--c-secondary-contrast': '#1a1b26',
        '--bg-page': '#1a1b26', '--bg-sidebar': '#16161e', '--bg-card': '#1f2335',
        '--bg-surface': '#24283b', '--bg-input': '#24283b', '--bg-hover': '#2f3347',
        '--text': '#c0caf5', '--text-muted': '#a9b1d6',
        '--border': '#2f3347', '--border-strong': '#3b4261',
        '--success': '#9ece6a', '--warning': '#e0af68', '--error': '#f7768e', '--info': '#7dcfff',
        '--radius': '10px',
      },
    },
    {
      id: 'catppuccin',
      name: 'Catppuccin Mocha',
      blurb: 'Cozy pastels � official Mocha flavor',
      preset: 'dark',
      uiVariant: 'glass', scene: 'nebula', shell: 'edge', buttonShape: 'soft',
      vars: {
        '--c-primary': '#cba6f7', '--c-primary-hover': '#b4befe', '--c-primary-contrast': '#11111b',
        '--c-secondary': '#f5c2e7', '--c-secondary-contrast': '#11111b',
        '--bg-page': '#11111b', '--bg-sidebar': '#181825', '--bg-card': '#1e1e2e',
        '--bg-surface': '#313244', '--bg-input': '#181825', '--bg-hover': '#45475a',
        '--text': '#cdd6f4', '--text-muted': '#a6adc8',
        '--border': '#313244', '--border-strong': '#45475a',
        '--success': '#a6e3a1', '--warning': '#f9e2af', '--error': '#f38ba8', '--info': '#89b4fa',
        '--radius': '12px',
      },
    },
  ],
  light: [
    {
      id: 'editorial',
      name: 'Editorial',
      blurb: 'Accent rule, uppercase labels',
      preset: 'light',
      uiVariant: 'editorial', scene: 'aurora-wave', shell: 'ruled', buttonShape: 'square',
      vars: { '--c-primary': '#4f46e5', '--c-secondary': '#f59e0b', '--radius': '8px' },
    },
    {
      id: 'paper',
      name: 'Paper',
      blurb: 'Warm off-white, ink accents',
      preset: 'light',
      uiVariant: 'editorial', scene: 'aurora-wave', shell: 'ruled', buttonShape: 'square',
      vars: {
        '--c-primary': '#8b5e34', '--c-secondary': '#c2a37a',
        '--bg-page': '#faf7f2', '--bg-card': '#fffdf9', '--bg-surface': '#f5f0e8',
        '--border': '#e6ddcf', '--radius': '3px',
      },
    },
    {
      id: 'clean',
      name: 'Clean',
      blurb: 'Pure white, no borders',
      preset: 'light',
      uiVariant: 'soft', scene: 'flat', shell: 'edge', buttonShape: 'soft',
      vars: {
        '--c-primary': '#2563eb', '--c-secondary': '#60a5fa',
        '--bg-page': '#ffffff', '--bg-card': '#ffffff', '--border': '#f1f5f9',
        '--radius': '12px',
      },
    },
    {
      id: 'soft-grey',
      name: 'Soft grey',
      blurb: 'Low contrast, muted, calm',
      preset: 'light',
      uiVariant: 'soft', scene: 'aurora-wave', shell: 'floating', buttonShape: 'pill',
      vars: {
        '--c-primary': '#64748b', '--c-secondary': '#94a3b8',
        '--bg-page': '#f4f5f7', '--bg-card': '#fbfbfc', '--text': '#334155',
        '--radius': '14px',
      },
    },
    {
      id: 'latte',
      name: 'Catppuccin Latte',
      blurb: 'Pastel light � official Latte flavor',
      preset: 'light',
      uiVariant: 'soft', scene: 'flat', shell: 'edge', buttonShape: 'soft',
      vars: {
        '--c-primary': '#8839ef', '--c-primary-hover': '#7428d4', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#ea76cb', '--c-secondary-contrast': '#402238',
        '--bg-page': '#eff1f5', '--bg-surface': '#e6e9ef', '--bg-hover': '#dce0e8',
        '--border': '#ccd0da', '--border-strong': '#bcc0cc',
        '--success': '#40a02b', '--warning': '#df8e1d', '--error': '#d20f39', '--info': '#1e66f5',
        '--radius': '10px',
      },
    },
    {
      id: 'solar-light',
      name: 'Solarized Light',
      blurb: 'The classic warm paper palette',
      preset: 'light',
      uiVariant: 'editorial', scene: 'flat', shell: 'ruled', buttonShape: 'square',
      vars: {
        '--c-primary': '#1b74b8', '--c-primary-hover': '#16639d', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#2aa198', '--c-secondary-contrast': '#002b36',
        '--bg-page': '#fdf6e3', '--bg-sidebar': '#fdf6e3', '--bg-card': '#fdf6e3',
        '--bg-surface': '#eee8d5', '--bg-input': '#fffef2', '--bg-hover': '#eee8d5',
        '--text': '#073642', '--text-muted': '#657b83',
        '--border': '#eee0c2', '--border-strong': '#d3c8a8',
        '--success': '#859900', '--warning': '#cb4b16', '--error': '#dc322f', '--info': '#6c71c4',
        '--radius': '4px',
      },
    },
    {
      id: 'one-light',
      name: 'One Light',
      blurb: 'Atom One Light � the editor classic',
      preset: 'light',
      uiVariant: 'soft', scene: 'flat', shell: 'edge', buttonShape: 'soft',
      vars: {
        '--c-primary': '#4078f2', '--c-primary-hover': '#3a5fc8', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#c18401', '--c-secondary-contrast': '#2d2510',
        '--bg-page': '#fafafa', '--bg-surface': '#f0f0f0', '--bg-hover': '#e6e6e6',
        '--border': '#d9d9dc', '--border-strong': '#c0c0c5',
        '--success': '#50a14f', '--warning': '#c18401', '--error': '#e45649', '--info': '#0184bc',
        '--radius': '6px',
      },
    },
    {
      id: 'rose-dawn',
      name: 'Ros� Pine Dawn',
      blurb: 'Warm designer light, iris & gold',
      preset: 'light',
      uiVariant: 'editorial', scene: 'aurora-wave', shell: 'ruled', buttonShape: 'square',
      vars: {
        '--c-primary': '#7a6899', '--c-primary-hover': '#67577f', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#286983', '--c-secondary-contrast': '#ffffff',
        '--bg-page': '#faf4ed', '--bg-sidebar': '#fffaf3', '--bg-card': '#fffaf3',
        '--bg-surface': '#f2e9e1', '--bg-input': '#fffaf3', '--bg-hover': '#f2e9e1',
        '--text': '#575279', '--text-muted': '#797593',
        '--border': '#e5d9ce', '--border-strong': '#d4c4b5',
        '--success': '#286983', '--warning': '#ea9d34', '--error': '#b4637a', '--info': '#56949f',
        '--radius': '6px',
      },
    },
  ],
  classic: [
    {
      id: 'flat-admin',
      name: 'Flat admin',
      blurb: 'Dense, squared, zero effects',
      preset: 'classic',
      uiVariant: 'flat', scene: 'flat', shell: 'edge', buttonShape: 'square',
      vars: { '--c-primary': '#2f6bef', '--c-secondary': '#5e7191', '--radius': '6px' },
    },
    {
      id: 'compact',
      name: 'Compact',
      blurb: 'Tighter spacing for big tables',
      preset: 'classic',
      uiVariant: 'flat', scene: 'flat', shell: 'edge', buttonShape: 'square',
      vars: {
        '--c-primary': '#0f766e', '--c-secondary': '#5e7191',
        '--radius': '2px', '--space-4': '0.7rem', '--space-6': '1rem',
      },
    },
    {
      id: 'legacy',
      name: 'Legacy',
      blurb: 'Hard borders, bevel feel',
      preset: 'classic',
      uiVariant: 'flat', scene: 'flat', shell: 'edge', buttonShape: 'square',
      vars: {
        '--c-primary': '#1e40af', '--c-secondary': '#7c8794',
        '--c-secondary-contrast': '#20242a',
        '--bg-page': '#d9dde3', '--bg-card': '#f0f2f5', '--border': '#9aa3ad',
        '--border-weight': '2px', '--radius': '0px',
      },
    },
    {
      id: 'high-contrast',
      name: 'High Contrast',
      blurb: 'Accessibility first � maximum legibility',
      preset: 'classic',
      uiVariant: 'flat', scene: 'flat', shell: 'edge', buttonShape: 'square',
      vars: {
        '--c-primary': '#0b5ed7', '--c-primary-hover': '#094cb0', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#374151', '--c-secondary-contrast': '#ffffff',
        '--bg-page': '#ffffff', '--bg-sidebar': '#ffffff', '--bg-card': '#ffffff',
        '--bg-surface': '#f6f7f9', '--bg-hover': '#eef0f3',
        '--text': '#0a0a0a', '--text-muted': '#333a45',
        '--border': '#9aa4b2', '--border-strong': '#6b7480',
        '--border-weight': '2px',
        '--success': '#15803d', '--warning': '#a16207', '--error': '#b91c1c', '--info': '#1d4ed8',
        '--radius': '2px',
      },
    },
    {
      id: 'warm-admin',
      name: 'Terracotta',
      blurb: 'Warm clay tones for CRM-style ops',
      preset: 'classic',
      uiVariant: 'flat', scene: 'flat', shell: 'edge', buttonShape: 'square',
      vars: {
        '--c-primary': '#c2410c', '--c-primary-hover': '#9a3412', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#78716c', '--c-secondary-contrast': '#ffffff',
        '--bg-page': '#faf7f2', '--bg-sidebar': '#fffdf9', '--bg-card': '#fffdf9',
        '--bg-surface': '#f5efe6', '--bg-hover': '#efe7da',
        '--border': '#e9e1d5', '--border-strong': '#d6c9b6',
        '--success': '#4d7c0f', '--warning': '#b45309', '--error': '#b91c1c', '--info': '#0369a1',
        '--radius': '4px',
      },
    },
    {
      id: 'indigo-admin',
      name: 'Indigo Office',
      blurb: 'Enterprise indigo, crisp whites',
      preset: 'classic',
      uiVariant: 'flat', scene: 'flat', shell: 'edge', buttonShape: 'square',
      vars: {
        '--c-primary': '#4338ca', '--c-primary-hover': '#3730a3', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#5c6f8c', '--c-secondary-contrast': '#ffffff',
        '--bg-page': '#f4f4fc', '--bg-sidebar': '#ffffff', '--bg-card': '#ffffff',
        '--bg-surface': '#ececf8', '--bg-hover': '#e2e2f5',
        '--border': '#dedeef', '--border-strong': '#c2c2e0',
        '--success': '#15803d', '--warning': '#b45309', '--error': '#be123c', '--info': '#1d4ed8',
        '--radius': '6px',
      },
    },
    {
      id: 'green-admin',
      name: 'Forest Ops',
      blurb: 'Calm forest greens for daily admin',
      preset: 'classic',
      uiVariant: 'flat', scene: 'flat', shell: 'edge', buttonShape: 'square',
      vars: {
        '--c-primary': '#2f6b4f', '--c-primary-hover': '#285c43', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#5b6b62', '--c-secondary-contrast': '#ffffff',
        '--bg-page': '#f3f7f4', '--bg-sidebar': '#ffffff', '--bg-card': '#ffffff',
        '--bg-surface': '#e9f0eb', '--bg-hover': '#dfe9e2',
        '--border': '#dce5de', '--border-strong': '#c0cfc4',
        '--success': '#15803d', '--warning': '#b45309', '--error': '#b91c1c', '--info': '#0369a1',
        '--radius': '4px',
      },
    },
    {
      id: 'maroon-admin',
      name: 'Maroon Ops',
      blurb: 'Deep maroon, quiet authority',
      preset: 'classic',
      uiVariant: 'flat', scene: 'flat', shell: 'edge', buttonShape: 'square',
      vars: {
        '--c-primary': '#9f1239', '--c-primary-hover': '#881337', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#514049', '--c-secondary-contrast': '#ffffff',
        '--bg-page': '#fcf4f6', '--bg-sidebar': '#ffffff', '--bg-card': '#ffffff',
        '--bg-surface': '#f8e8ec', '--bg-hover': '#f3dde3',
        '--border': '#f0dce2', '--border-strong': '#dcb8c3',
        '--success': '#15803d', '--warning': '#b45309', '--error': '#b91c1c', '--info': '#1d4ed8',
        '--radius': '4px',
      },
    },
  ],
  gaming: [
    {
      id: 'neon-hud',
      name: 'Neon HUD',
      blurb: 'Clipped panels, cyan/magenta glow',
      preset: 'gaming',
      uiVariant: 'neon', scene: 'grid', shell: 'hud', buttonShape: 'clipped',
      vars: { '--c-primary': '#00f0ff', '--c-secondary': '#d6117d' },
    },
    {
      id: 'cyberpunk',
      name: 'Cyberpunk',
      blurb: 'Hot pink dominant, industrial',
      preset: 'gaming',
      uiVariant: 'neon', scene: 'grid', shell: 'hud', buttonShape: 'clipped',
      vars: {
        '--c-primary': '#ff2d95', '--c-primary-hover': '#ff5bad', '--c-secondary': '#ffe600',
        '--c-secondary-contrast': '#1a1600',
        '--bg-page': '#0a0410', '--bg-card': '#150618',
      },
    },
    {
      id: 'terminal',
      name: 'Terminal',
      blurb: 'Green phosphor, monospace',
      preset: 'gaming',
      uiVariant: 'neon', scene: 'aurora-wave', shell: 'edge', buttonShape: 'square',
      vars: {
        '--c-primary': '#22c55e', '--c-primary-hover': '#4ade80', '--c-secondary': '#86efac',
        '--c-secondary-contrast': '#06210f',
        '--bg-page': '#030805', '--bg-card': '#06110a', '--border': '#14361f',
        '--radius': '0px',
      },
    },
    {
      id: 'retrowave',
      name: 'Retrowave',
      blurb: 'Sunset orange over purple grid',
      preset: 'gaming',
      uiVariant: 'neon', scene: 'grid', shell: 'hud', buttonShape: 'clipped',
      vars: {
        '--c-primary': '#ff8a3d', '--c-primary-hover': '#ffa35e', '--c-secondary': '#a855f7',
        '--c-secondary-contrast': '#170a26',
        '--bg-page': '#0d0618', '--bg-card': '#1b0b2e', '--border': '#3b1d63',
      },
    },
    {
      id: 'gruvbox',
      name: 'Gruvbox',
      blurb: 'Warm retro groove, amber glow',
      preset: 'gaming',
      uiVariant: 'neon', scene: 'grid', shell: 'hud', buttonShape: 'clipped',
      vars: {
        '--c-primary': '#fe8019', '--c-primary-hover': '#ff9a3d', '--c-primary-contrast': '#1d2021',
        '--c-secondary': '#fabd2f', '--c-secondary-contrast': '#1d2021',
        '--bg-page': '#1d2021', '--bg-sidebar': '#16191a', '--bg-card': '#282828',
        '--bg-surface': '#3c3836', '--bg-input': '#282828', '--bg-hover': '#504945',
        '--text': '#ebdbb2', '--text-muted': '#bdae93',
        '--border': '#3c3836', '--border-strong': '#504945',
        '--success': '#b8bb26', '--warning': '#fabd2f', '--error': '#fb4934', '--info': '#8ec07c',
        '--radius': '10px',
      },
    },
    {
      id: 'crimson',
      name: 'Crimson',
      blurb: 'Deep crimson esports over black chrome',
      preset: 'gaming',
      uiVariant: 'neon', scene: 'aurora-wave', shell: 'hud', buttonShape: 'clipped',
      vars: {
        '--c-primary': '#d92646', '--c-primary-hover': '#f43f5e', '--c-primary-contrast': '#ffffff',
        '--c-secondary': '#ff8fa3', '--c-secondary-contrast': '#2a050d',
        '--bg-page': '#0a0508', '--bg-sidebar': '#0e070a', '--bg-card': '#140a0e',
        '--bg-surface': '#1d1015', '--bg-input': '#140a0e', '--bg-hover': '#2a141b',
        '--text': '#ffe9ec', '--text-muted': '#c99da8',
        '--border': '#3a1220', '--border-strong': '#571b31',
        '--success': '#22c55e', '--warning': '#f59e0b', '--error': '#fb7185', '--info': '#93c5fd',
        '--radius': '10px',
      },
    },
    {
      id: 'acid',
      name: 'Acid',
      blurb: 'Cyber-yellow over industrial black',
      preset: 'gaming',
      uiVariant: 'neon', scene: 'grid', shell: 'hud', buttonShape: 'clipped',
      vars: {
        '--c-primary': '#fcee0a', '--c-primary-hover': '#fff45c', '--c-primary-contrast': '#0b0b04',
        '--c-secondary': '#06b6d4', '--c-secondary-contrast': '#04252b',
        '--bg-page': '#0b0b04', '--bg-sidebar': '#0f0f06', '--bg-card': '#121208',
        '--bg-surface': '#1a1a0c', '--bg-input': '#121208', '--bg-hover': '#232312',
        '--text': '#eef7d9', '--text-muted': '#9aa877',
        '--border': '#2e2e14', '--border-strong': '#48482a',
        '--success': '#a3e635', '--warning': '#facc15', '--error': '#ff4d6d', '--info': '#22d3ee',
        '--radius': '8px',
      },
    },
    {
      id: 'amber-crt',
      name: 'Amber CRT',
      blurb: 'Phosphor amber glow, retro terminal',
      preset: 'gaming',
      uiVariant: 'neon', scene: 'aurora-wave', shell: 'edge', buttonShape: 'square',
      vars: {
        '--c-primary': '#ffb000', '--c-primary-hover': '#ffc433', '--c-primary-contrast': '#140d00',
        '--c-secondary': '#e8e4b8', '--c-secondary-contrast': '#14130a',
        '--bg-page': '#120d00', '--bg-sidebar': '#0d0a00', '--bg-card': '#1d1506',
        '--bg-surface': '#281e0a', '--bg-input': '#1d1506', '--bg-hover': '#33270e',
        '--text': '#ffe9b8', '--text-muted': '#c9a55e',
        '--border': '#3f2f0a', '--border-strong': '#5f4815',
        '--success': '#c8a13a', '--warning': '#ffb000', '--error': '#ff5c33', '--info': '#c9b06a',
        '--radius': '4px',
      },
    },
  ],
  /**
   * Hacking � terminal / CRT. Three phosphor personalities, all on the scanline
   * background family so the preset reads as one product.
   */
  hacking: [
    {
      id: 'terminal-green',
      name: 'Terminal Green',
      blurb: 'Phosphor green on black, the classic shell',
      preset: 'hacking',
      uiVariant: 'neon', scene: 'scanline-crt', shell: 'hud', buttonShape: 'square',
      vars: {
        '--c-primary': '#22d37a', '--c-primary-hover': '#3ee68f', '--c-primary-contrast': '#04140c',
        '--c-secondary': '#0ea5a4', '--c-secondary-contrast': '#001413',
        '--bg-page': '#050a08', '--bg-sidebar': '#07110d', '--bg-card': '#0a1712',
        '--bg-surface': '#0d1f17', '--bg-input': '#0a1712', '--bg-hover': '#12301f',
        '--text': '#d7ffe8', '--text-muted': '#6f9c85',
        '--border': '#123326', '--border-strong': '#1d533b',
        '--success': '#22d37a', '--warning': '#e3b341', '--error': '#ff5c6c', '--info': '#4cc2ff',
        '--radius': '4px',
      },
    },
    {
      id: 'amber-console',
      name: 'Amber Console',
      blurb: 'Warm amber phosphor, VT100 terminal',
      preset: 'hacking',
      uiVariant: 'neon', scene: 'classic-terminal', shell: 'hud', buttonShape: 'square',
      vars: {
        '--c-primary': '#ffb749', '--c-primary-hover': '#ffc96b', '--c-primary-contrast': '#140d00',
        '--c-secondary': '#d98b1c', '--c-secondary-contrast': '#140d00',
        '--bg-page': '#0a0703', '--bg-sidebar': '#0d0904', '--bg-card': '#150f06',
        '--bg-surface': '#1c1409', '--bg-input': '#150f06', '--bg-hover': '#2a1d0c',
        '--text': '#ffeecb', '--text-muted': '#a98a55',
        '--border': '#33260e', '--border-strong': '#4f3a16',
        '--success': '#c8a13a', '--warning': '#ffb749', '--error': '#ff6a3d', '--info': '#c9b06a',
        '--radius': '4px',
      },
    },
    {
      id: 'cyan-shell',
      name: 'Cyan Shell',
      blurb: 'Cold cyan phosphor, oscilloscope',
      preset: 'hacking',
      uiVariant: 'neon', scene: 'classic-oscilloscope', shell: 'hud', buttonShape: 'square',
      vars: {
        '--c-primary': '#3ddbff', '--c-primary-hover': '#6ce6ff', '--c-primary-contrast': '#001318',
        '--c-secondary': '#0a7f9c', '--c-secondary-contrast': '#001318',
        '--bg-page': '#03080c', '--bg-sidebar': '#050d13', '--bg-card': '#07141b',
        '--bg-surface': '#0a1c25', '--bg-input': '#07141b', '--bg-hover': '#0e2a36',
        '--text': '#d9f6ff', '--text-muted': '#6a93a3',
        '--border': '#0f2d3a', '--border-strong': '#17495c',
        '--success': '#22d37a', '--warning': '#e3b341', '--error': '#ff5c6c', '--info': '#3ddbff',
        '--radius': '4px',
      },
    },
  ],
}
export const ALL_SUB_THEMES: SubTheme[] = Object.values(SUB_THEMES).flat()

export function findSubTheme(id: string): SubTheme | undefined {
  return ALL_SUB_THEMES.find((s) => s.id === id)
}

/**
 * Turn a six-value palette into a complete `SubTheme`.
 *
 * The UI reads a dozen CSS custom properties per variant, but a palette only
 * specifies six. Deriving the rest here — instead of writing them out 60 times —
 * means a ported palette can never ship a half-finished token set, and the
 * light/dark direction comes from the background's own luminance rather than a
 * hand-written flag that could disagree with the colours.
 */
function fromPalette(p: PaletteSpec, preset: ThemePreset, scene: SceneType): SubTheme {
  const dark = isDark(p.bg)
  // Lifts go away from the background: lighter on dark themes, darker on light.
  const lift = dark ? 0.06 : -0.05
  const liftStrong = dark ? 0.12 : -0.1

  return {
    id: p.id as SubThemeId,
    name: p.name,
    blurb: p.blurb,
    preset,
    uiVariant:
      preset === 'classic'
        ? 'flat'
        : preset === 'gaming' || preset === 'hacking'
          ? 'neon'
          : dark
            ? 'glass'
            : 'editorial',
    scene,
    shell:
      preset === 'classic' || preset === 'dark'
        ? 'edge'
        : preset === 'gaming' || preset === 'hacking'
          ? 'hud'
          : 'floating',
    buttonShape:
      preset === 'classic' || preset === 'hacking'
        ? 'square'
        : preset === 'gaming'
          ? 'clipped'
          : 'pill',
    vars: {
      '--c-primary': p.primary,
      '--c-primary-hover': shift(p.primary, dark ? 0.18 : -0.12),
      '--c-primary-contrast': isDark(p.primary) ? '#ffffff' : '#08110c',
      '--c-secondary': p.accent,
      '--c-secondary-contrast': isDark(p.accent) ? '#ffffff' : '#08110c',
      '--bg-page': p.bg,
      '--bg-sidebar': p.card,
      '--bg-card': p.card,
      '--bg-surface': p.surface,
      '--bg-input': p.card,
      '--bg-hover': shift(p.surface, lift),
      '--text': p.text,
      '--text-muted': shift(p.text, dark ? -0.34 : 0.34),
      '--border': shift(p.surface, liftStrong),
      '--border-strong': shift(p.surface, liftStrong * 2),
      '--success': '#22c55e',
      '--warning': '#f59e0b',
      '--error': '#ef4444',
      '--info': '#3b82f6',
      '--radius':
        preset === 'gaming'
          ? '14px'
          : preset === 'hacking'
            ? '4px'
            : preset === 'classic'
              ? '6px'
              : '12px',
    },
  }
}

/**
 * The ported palettes, expanded into variants.
 *
 * Appended AFTER the hand-written sub-themes so those keep their position and
 * ids (a saved theme pointing at `soft` or `deep-glass` still resolves). This is
 * additive: it can only grow the list, never reorder it.
 */
const PORTED: Record<ThemePreset, SubTheme[]> = {
  modern: PALETTES.modern.map((p) => fromPalette(p, 'modern', 'particles')),
  dark: PALETTES.dark.map((p) => fromPalette(p, 'dark', 'subtle-orbs')),
  light: PALETTES.light.map((p) => fromPalette(p, 'light', 'aurora-wave')),
  classic: PALETTES.classic.map((p) => fromPalette(p, 'classic', 'flat')),
  gaming: PALETTES.gaming.map((p) => fromPalette(p, 'gaming', 'cyber-grid')),
  hacking: PALETTES.hacking.map((p) => fromPalette(p, 'hacking', 'scanline-crt')),
}

/**
 * Merge hand-written and ported variants, dropping a ported twin.
 *
 * A palette whose id already exists (or whose name matches) is skipped rather
 * than duplicated: one tile per identity, and the hand-written one wins because
 * it carries the tuned extras.
 */
function mergeVariants(hand: SubTheme[], ported: SubTheme[]): SubTheme[] {
  const seenId = new Set(hand.map((s) => s.id))
  const seenName = new Set(hand.map((s) => s.name.toLowerCase()))
  const out = [...hand]
  for (const p of ported) {
    if (seenId.has(p.id) || seenName.has(p.name.toLowerCase())) continue
    seenId.add(p.id)
    seenName.add(p.name.toLowerCase())
    out.push(p)
  }
  return out
}

/**
 * Grow every preset to ten identities.
 *
 * Nothing is invented to reach the number: if a preset cannot make ten the list
 * ships whatever it honestly has, because a padded duplicate tile is worse than
 * a short grid.
 */
const TARGET_PER_PRESET = 10
for (const preset of Object.keys(SUB_THEMES) as ThemePreset[]) {
  SUB_THEMES[preset] = mergeVariants(SUB_THEMES[preset], PORTED[preset]).slice(
    0,
    TARGET_PER_PRESET,
  )
}

/** Default variant id for a preset (first entry). */
export function defaultVariantId(preset: ThemePreset): SubThemeId {
  return SUB_THEMES[preset][0].id
}

/**
 * Resolve the variant that should actually be shown for a preset.
 * A stored id from another preset (or a stale/unknown id) falls back to
 * that preset default, otherwise no tile is active and no skin applies.
 */
export function resolveVariant(preset: ThemePreset, id?: string): SubTheme {
  const list = SUB_THEMES[preset]
  return list.find((v) => v.id === id) ?? list[0]
}
