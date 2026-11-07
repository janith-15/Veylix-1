import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { SUBTHEMES_CATALOG } from '../../data/initialData';
import { 
  Palette, 
  Sparkles, 
  Check, 
  Sliders, 
  Sun, 
  Moon, 
  Gamepad2, 
  Terminal, 
  Layers, 
  Eye,
  RefreshCw,
  Activity,
  Zap,
  Shield,
  Monitor
} from 'lucide-react';
import { ThemeConfig } from '../../types';

export const ThemeStudioView: React.FC = () => {
  const { themeConfig, setThemeConfig, applySubtheme, applyMasterPreset } = useVeylixStore();
  const [activeCategory, setActiveCategory] = useState<'all' | 'modern' | 'dark' | 'light' | 'classic' | 'gaming'>('all');

  const filteredSubthemes = activeCategory === 'all'
    ? SUBTHEMES_CATALOG
    : SUBTHEMES_CATALOG.filter(s => s.category === activeCategory);

  const masterPresets = [
    {
      id: 'modern' as const,
      name: 'Professional',
      kicker: 'Fintech & Enterprise Cloud',
      desc: 'Balanced slate surfaces, precision emerald accents, crisp typography, and particle grid motion.',
      icon: Sparkles,
      primary: '#10b981',
      accent: '#059669',
      bg: '#091511',
      animType: 'particles' as const,
      badge: 'ENTERPRISE RECOMMENDED'
    },
    {
      id: 'dark' as const,
      name: 'Dark Stealth',
      kicker: 'Deep Obsidian & Cosmic Space',
      desc: 'Deep black background with slow, ethereal floating nebula orbs and cyber cyan luminescence.',
      icon: Moon,
      primary: '#38bdf8',
      accent: '#818cf8',
      bg: '#06080c',
      animType: 'subtle_orbs' as const,
      badge: 'HIGH CONTRAST'
    },
    {
      id: 'light' as const,
      name: 'Clean Light',
      kicker: 'Pristine Daylight & Soft Glass',
      desc: 'Airy daylight palette with soft pastel aurora waves, crisp card surfaces, and dark text.',
      icon: Sun,
      primary: '#0284c7',
      accent: '#0369a1',
      bg: '#f8fafc',
      animType: 'aurora_wave' as const,
      badge: 'DAYLIGHT MODE'
    },
    {
      id: 'gaming' as const,
      name: 'Gaming HUD',
      kicker: 'Cyberpunk Neon & Sub-1ms Zero Ping',
      desc: 'High-octane synthwave aesthetics with animated cyber grid lines, laser spotlights, and neon glow.',
      icon: Gamepad2,
      primary: '#f43f5e',
      accent: '#06b6d4',
      bg: '#080512',
      animType: 'cyber_grid' as const,
      badge: 'ULTRA LOW PING'
    },
    {
      id: 'classic' as const,
      name: 'Classical',
      kicker: 'Vintage Unix CRT & VT100 Terminal',
      desc: 'Retro hacker workstation feel with phosphor green glow, CRT horizontal scanlines, and monospace telemetry.',
      icon: Terminal,
      primary: '#22c55e',
      accent: '#16a34a',
      bg: '#051208',
      animType: 'scanline_crt' as const,
      badge: 'SYSADMIN CLASSIC'
    }
  ];

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-extrabold text-white tracking-tight flex items-center space-x-2">
            <Palette className="w-5 h-5 text-emerald-400" />
            <span>Theme Studio & Visual Experience Engine</span>
          </h1>
          <p className="text-xs text-neutral-400">
            5 Master System Presets, Dynamic Background Animations, and 40 Curated Subtheme Palettes
          </p>
        </div>

        <div className="flex items-center space-x-2 bg-neutral-900 border border-neutral-800 p-1.5 rounded-xl text-xs">
          <span className="text-neutral-400 px-2">Active Preset:</span>
          <span className="font-bold text-emerald-400 capitalize bg-neutral-950 px-2.5 py-1 rounded-lg border border-neutral-800">
            {themeConfig.preset} ({themeConfig.subtheme.replace('_', ' ')})
          </span>
        </div>
      </div>

      {/* SECTION 1: 5 MASTER THEMES SHOWCASE */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-sm font-bold text-white flex items-center space-x-2">
              <Zap className="w-4 h-4 text-emerald-400" />
              <span>5 Core Master Themes</span>
            </h2>
            <p className="text-xs text-neutral-400">Instant full-system visual transformation with matching animated backdrops</p>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-5 gap-4">
          {masterPresets.map(preset => {
            const Icon = preset.icon;
            const isSelected = themeConfig.preset === preset.id;

            return (
              <div
                key={preset.id}
                onClick={() => applyMasterPreset(preset.id)}
                className={`p-5 rounded-2xl border cursor-pointer transition relative flex flex-col justify-between group overflow-hidden ${
                  isSelected
                    ? 'bg-neutral-900 border-emerald-500 shadow-2xl shadow-emerald-500/10 ring-1 ring-emerald-500'
                    : 'bg-neutral-950/80 border-neutral-800 hover:border-neutral-700 hover:bg-neutral-900/60'
                }`}
              >
                {/* Visual accent glow */}
                <div 
                  className="absolute -top-12 -right-12 w-28 h-28 rounded-full blur-2xl opacity-20 pointer-events-none transition group-hover:opacity-40"
                  style={{ backgroundColor: preset.primary }}
                />

                <div className="space-y-3 relative z-10">
                  <div className="flex items-center justify-between">
                    <div 
                      className="w-10 h-10 rounded-xl flex items-center justify-center font-bold shadow-inner"
                      style={{ 
                        backgroundColor: `${preset.primary}25`,
                        color: preset.primary,
                        border: `1px solid ${preset.primary}40`
                      }}
                    >
                      <Icon className="w-5 h-5" />
                    </div>
                    {isSelected && (
                      <span className="w-5 h-5 rounded-full bg-emerald-500 text-neutral-950 flex items-center justify-center">
                        <Check className="w-3.5 h-3.5 stroke-[3]" />
                      </span>
                    )}
                  </div>

                  <div>
                    <h3 className="font-extrabold text-white text-sm">{preset.name}</h3>
                    <p className="text-[11px] text-emerald-400 font-medium mt-0.5">{preset.kicker}</p>
                    <p className="text-[11px] text-neutral-400 mt-1 leading-relaxed">
                      {preset.desc}
                    </p>
                  </div>

                  {/* Swatch preview bar */}
                  <div className="flex items-center space-x-1.5 p-1.5 bg-neutral-900/90 rounded-lg border border-neutral-800/80">
                    <span 
                      className="w-4 h-4 rounded-full border border-neutral-700/50 shrink-0" 
                      style={{ backgroundColor: preset.primary }} 
                      title="Primary Color"
                    />
                    <span 
                      className="w-4 h-4 rounded-full border border-neutral-700/50 shrink-0" 
                      style={{ backgroundColor: preset.accent }} 
                      title="Accent Color"
                    />
                    <span 
                      className="w-4 h-4 rounded-full border border-neutral-700/50 shrink-0" 
                      style={{ backgroundColor: preset.bg }} 
                      title="Background"
                    />
                    <span className="text-[10px] text-neutral-400 font-mono ml-auto capitalize">
                      {preset.animType.replace('_', ' ')}
                    </span>
                  </div>
                </div>

                <div className="mt-4 pt-3 border-t border-neutral-800/80 flex items-center justify-between text-[11px]">
                  <span className="text-neutral-500 text-[10px] uppercase font-mono">{preset.badge}</span>
                  <span className={`font-semibold ${isSelected ? 'text-emerald-400' : 'text-neutral-400 group-hover:text-white'}`}>
                    {isSelected ? 'Active Theme' : 'Apply →'}
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* SECTION 2: BACKGROUND ANIMATION CONTROLS */}
      <div className="p-6 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
          <div className="flex items-center space-x-2">
            <Activity className="w-4 h-4 text-emerald-400" />
            <h2 className="text-sm font-bold text-white">Ambient Background Animation Studio</h2>
          </div>
          <div className="flex items-center space-x-2">
            <span className="text-xs text-neutral-400">Master Animation Engine:</span>
            <button
              onClick={() => setThemeConfig({ enableAnimations: !themeConfig.enableAnimations })}
              className={`px-3 py-1 rounded-lg text-xs font-semibold border transition ${
                themeConfig.enableAnimations
                  ? 'bg-emerald-500/20 text-emerald-300 border-emerald-500/40'
                  : 'bg-neutral-800 text-neutral-400 border-neutral-700'
              }`}
            >
              {themeConfig.enableAnimations ? 'Enabled' : 'Disabled'}
            </button>
          </div>
        </div>

        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 text-xs">
          <p className="text-neutral-400">
            Animations are strictly matched to the active theme palette (<span className="text-emerald-400 font-bold uppercase">{themeConfig.preset}</span>) to guarantee visual harmony.
          </p>
          <span className="text-[11px] font-mono text-neutral-400">
            60 FPS GPU Compositor Layer
          </span>
        </div>

        {/* Theme-Matched Animation Suite */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 text-xs">
          {(
            themeConfig.preset === 'gaming' ? [
              { id: 'cyber_grid' as const, label: 'Cyberpunk Tron Grid', category: 'Gaming', desc: 'Neon 3D horizon with shooting data packets and wireframe sun' },
              { id: 'gaming_warp' as const, label: 'Hyperspace Warp Speed', category: 'Gaming', desc: 'High-velocity lightspeed star tunnel focused on your cursor' },
              { id: 'gaming_hex' as const, label: 'Cyber Hex Shield Grid', category: 'Gaming', desc: 'Pulsing hexagonal energy matrix with audio spectrum wave' }
            ] :
            themeConfig.preset === 'classic' ? [
              { id: 'scanline_crt' as const, label: 'Matrix Digital Rain', category: 'Classical', desc: 'Phosphor green cascading code streams with CRT scanline' },
              { id: 'classic_terminal' as const, label: 'Unix Boot Shell', category: 'Classical', desc: 'Live boot telemetry, memory hex dump & blinking prompt' },
              { id: 'classic_oscilloscope' as const, label: 'Vector CRT Oscilloscope', category: 'Classical', desc: 'Harmonic sine wave vectors reacting to mouse cursor' }
            ] :
            themeConfig.preset === 'dark' ? [
              { id: 'subtle_orbs' as const, label: 'Deep Cosmic Nebula', category: 'Dark Stealth', desc: 'Breathing multi-layer gas clouds, diamond stars & meteors' },
              { id: 'dark_stealth_radar' as const, label: 'Covert 360° Radar Sweep', category: 'Dark Stealth', desc: '360° rotating radar scanner illuminating hidden VPN nodes' },
              { id: 'dark_starfall' as const, label: 'Celestial Starfall Shower', category: 'Dark Stealth', desc: 'Downward parallax star drift with glowing meteors' }
            ] :
            themeConfig.preset === 'light' ? [
              { id: 'aurora_wave' as const, label: 'Luminous Aurora Waves', category: 'Clean Light', desc: 'Gentle flowing daylight silk ribbons & soft floating bokeh' },
              { id: 'light_caustics' as const, label: 'Prismatic Sunlight Caustics', category: 'Clean Light', desc: 'Prismatic light refraction beams shimmering across view' },
              { id: 'light_geometric' as const, label: 'Minimal Origami Geometry', category: 'Clean Light', desc: 'Floating isometric wireframe polyhedra reacting to motion' }
            ] : [
              { id: 'particles' as const, label: 'VPN Node Constellation', category: 'Professional', desc: 'Encrypted tunnel links connecting global VPN cluster nodes' },
              { id: 'pro_worldmap' as const, label: 'World Route Map & Pings', category: 'Professional', desc: 'Global server pins (Singapore, Tokyo, Frankfurt) with pings' },
              { id: 'pro_fiber' as const, label: 'Fiber Optic Data Streams', category: 'Professional', desc: 'High-speed light buses & enterprise blueprint architecture' }
            ]
          ).map(anim => {
            const isCurrent = themeConfig.bgAnimationType === anim.id;
            return (
              <button
                key={anim.id}
                onClick={() => setThemeConfig({ bgAnimationType: anim.id, enableAnimations: true })}
                className={`p-4 rounded-xl border text-left transition flex flex-col justify-between ${
                  isCurrent
                    ? 'bg-emerald-500/15 border-emerald-500/50 text-white shadow-lg ring-1 ring-emerald-500/20'
                    : 'bg-neutral-950 border-neutral-800 text-neutral-400 hover:text-white hover:bg-neutral-900'
                }`}
              >
                <div>
                  <div className="flex items-center justify-between">
                    <p className="font-bold text-xs text-white">{anim.label}</p>
                    <span className="text-[10px] font-mono text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded border border-emerald-500/20">
                      {anim.category}
                    </span>
                  </div>
                  <p className="text-[11px] text-neutral-400 mt-1">{anim.desc}</p>
                </div>
                <div className="mt-3 pt-2 border-t border-neutral-800/60 flex items-center justify-between">
                  <span className="text-[10px] text-neutral-500">Theme Harmonized</span>
                  <span className={`text-[10px] font-mono font-bold ${isCurrent ? 'text-emerald-400' : 'text-neutral-500'}`}>
                    {isCurrent ? '● ACTIVE' : 'SELECT →'}
                  </span>
                </div>
              </button>
            );
          })}
        </div>
      </div>

      {/* SECTION 3: 40 SUBTHEMES EXPLORER */}
      <div className="space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
          <div>
            <h2 className="text-sm font-bold text-white flex items-center space-x-2">
              <Layers className="w-4 h-4 text-emerald-400" />
              <span>Full 40 Subthemes Catalog</span>
            </h2>
            <p className="text-xs text-neutral-400">Filter subthemes by aesthetic category or apply specialized custom palettes</p>
          </div>

          {/* Category Filter Tabs */}
          <div className="flex flex-wrap gap-1.5">
            {[
              { id: 'all', label: 'All 40' },
              { id: 'modern', label: 'Modern / Pro' },
              { id: 'dark', label: 'Dark' },
              { id: 'light', label: 'Light' },
              { id: 'classic', label: 'Classical' },
              { id: 'gaming', label: 'Gaming' }
            ].map(cat => {
              const isSelected = activeCategory === cat.id;
              return (
                <button
                  key={cat.id}
                  onClick={() => setActiveCategory(cat.id as any)}
                  className={`px-3 py-1 rounded-lg text-xs font-medium transition ${
                    isSelected
                      ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/40'
                      : 'bg-neutral-900 text-neutral-400 hover:text-white border border-neutral-800'
                  }`}
                >
                  {cat.label}
                </button>
              );
            })}
          </div>
        </div>

        {/* Subthemes Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3.5">
          {filteredSubthemes.map(subtheme => {
            const isCurrent = themeConfig.subtheme === subtheme.id;

            return (
              <div
                key={subtheme.id}
                onClick={() => applySubtheme(subtheme.id)}
                className={`p-4 rounded-2xl border cursor-pointer transition relative group flex flex-col justify-between ${
                  isCurrent 
                    ? 'bg-neutral-900 border-emerald-500 shadow-xl shadow-emerald-500/10 ring-1 ring-emerald-500' 
                    : 'bg-neutral-950/70 border-neutral-800 hover:border-neutral-700 hover:bg-neutral-900/60'
                }`}
              >
                <div>
                  <div className="flex items-center justify-between">
                    <h3 className="font-bold text-white text-xs">{subtheme.name}</h3>
                    <span className="text-[10px] font-mono uppercase px-1.5 py-0.5 rounded bg-neutral-900 text-neutral-400 border border-neutral-800">
                      {subtheme.tag}
                    </span>
                  </div>

                  {/* Swatch Previews */}
                  <div className="mt-2.5 grid grid-cols-4 gap-1.5 p-2 rounded-xl bg-neutral-900/90 border border-neutral-800">
                    <div 
                      className="h-6 rounded-md shadow-inner flex items-center justify-center text-[9px] font-bold text-black"
                      style={{ backgroundColor: subtheme.primary }}
                      title="Primary"
                    >
                      PRI
                    </div>
                    <div 
                      className="h-6 rounded-md shadow-inner flex items-center justify-center text-[9px] font-bold text-white"
                      style={{ backgroundColor: subtheme.accent }}
                      title="Accent"
                    >
                      ACC
                    </div>
                    <div 
                      className="h-6 rounded-md shadow-inner border border-neutral-700/50"
                      style={{ backgroundColor: subtheme.surfaceBg }}
                      title="Surface"
                    />
                    <div 
                      className="h-6 rounded-md shadow-inner border border-neutral-700/50"
                      style={{ backgroundColor: subtheme.bgDark }}
                      title="Background"
                    />
                  </div>
                </div>

                <div className="mt-3 pt-2.5 border-t border-neutral-800/80 flex items-center justify-between text-[11px]">
                  <span className="text-neutral-500 capitalize">{subtheme.category}</span>
                  {isCurrent ? (
                    <span className="flex items-center space-x-1 text-emerald-400 font-bold">
                      <Check className="w-3.5 h-3.5" />
                      <span>Active</span>
                    </span>
                  ) : (
                    <span className="text-neutral-500 group-hover:text-emerald-400 transition">Apply →</span>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* SECTION 4: ADVANCED UI PARAMETERS */}
      <div className="p-6 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl space-y-4">
        <div className="flex items-center space-x-2">
          <Sliders className="w-4 h-4 text-emerald-400" />
          <h2 className="text-sm font-bold text-white">Layout Geometry & Surface Styling</h2>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 text-xs">
          {/* Glassmorphism toggle */}
          <div className="p-4 bg-neutral-950 border border-neutral-800 rounded-xl flex items-center justify-between">
            <div>
              <p className="font-semibold text-white">Glassmorphism Translucency</p>
              <p className="text-[11px] text-neutral-400">Frosted blur for cards & headers</p>
            </div>
            <button
              onClick={() => setThemeConfig({ enableGlassmorphism: !themeConfig.enableGlassmorphism })}
              className={`w-11 h-6 rounded-full transition relative ${
                themeConfig.enableGlassmorphism ? 'bg-emerald-500' : 'bg-neutral-800'
              }`}
            >
              <span className={`absolute top-1 left-1 w-4 h-4 rounded-full bg-white transition-transform ${
                themeConfig.enableGlassmorphism ? 'translate-x-5' : 'translate-x-0'
              }`} />
            </button>
          </div>

          {/* Density toggle */}
          <div className="p-4 bg-neutral-950 border border-neutral-800 rounded-xl space-y-2">
            <span className="font-semibold text-white">Information Density</span>
            <div className="grid grid-cols-3 gap-1.5">
              {(['compact', 'comfortable', 'spacious'] as const).map(d => (
                <button
                  key={d}
                  onClick={() => setThemeConfig({ density: d })}
                  className={`py-1 text-center font-semibold rounded-lg border capitalize transition ${
                    themeConfig.density === d
                      ? 'bg-emerald-500/20 text-emerald-400 border-emerald-500/50'
                      : 'bg-neutral-900 border-neutral-800 text-neutral-400'
                  }`}
                >
                  {d}
                </button>
              ))}
            </div>
          </div>

          {/* Border Radius */}
          <div className="p-4 bg-neutral-950 border border-neutral-800 rounded-xl space-y-2">
            <span className="font-semibold text-white">Corner Geometry</span>
            <div className="grid grid-cols-4 gap-1.5">
              {(['sm', 'md', 'lg', 'xl'] as const).map(rad => (
                <button
                  key={rad}
                  onClick={() => setThemeConfig({ borderRadius: rad })}
                  className={`py-1 text-center font-mono font-semibold rounded-lg border transition ${
                    themeConfig.borderRadius === rad
                      ? 'bg-emerald-500/20 text-emerald-400 border-emerald-500/50'
                      : 'bg-neutral-900 border-neutral-800 text-neutral-400'
                  }`}
                >
                  {rad.toUpperCase()}
                </button>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
