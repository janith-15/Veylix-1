import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  Shield, 
  Search, 
  Bell, 
  Terminal, 
  Palette, 
  ChevronDown, 
  User as UserIcon, 
  Activity,
  Layers,
  Sparkles,
  Moon,
  Sun,
  Gamepad2,
  Menu,
  KeyRound,
  Lock,
  Zap
} from 'lucide-react';
import { UserRole } from '../../types';

interface NavbarProps {
  onOpenInstaller: () => void;
  onOpenThemeStudio: () => void;
  onOpenProfile: () => void;
  onToggleNotifications: () => void;
  onToggleMobileMenu: () => void;
  unreadCount: number;
}

export const Navbar: React.FC<NavbarProps> = ({
  onOpenInstaller,
  onOpenThemeStudio,
  onOpenProfile,
  onToggleNotifications,
  onToggleMobileMenu,
  unreadCount
}) => {
  const { 
    currentUser, 
    currentRole, 
    switchRole, 
    searchQuery, 
    setSearchQuery, 
    themeConfig,
    setThemeConfig,
    applyMasterPreset,
    servers 
  } = useVeylixStore();

  const [roleDropdownOpen, setRoleDropdownOpen] = useState(false);
  const [vfxDropdownOpen, setVfxDropdownOpen] = useState(false);

  const onlineServers = servers.filter(s => s.status === 'ONLINE').length;
  const avgLatency = Math.round(servers.reduce((acc, s) => acc + s.latencyMs, 0) / (servers.length || 1));

  return (
    <header className="sticky top-0 z-40 w-full h-16 bg-neutral-950/85 backdrop-blur-md border-b border-neutral-800/80 px-3 lg:px-6 flex items-center justify-between">
      {/* Brand & Mobile Hamburger Toggle */}
      <div className="flex items-center space-x-3">
        {/* Mobile menu trigger */}
        <button
          onClick={onToggleMobileMenu}
          className="p-2 text-neutral-400 hover:text-white rounded-xl bg-neutral-900 border border-neutral-800 md:hidden transition"
          aria-label="Toggle Navigation Menu"
        >
          <Menu className="w-5 h-5" />
        </button>

        <div className="flex items-center space-x-2.5">
          <div className="relative flex items-center justify-center w-9 h-9 rounded-xl bg-gradient-to-br from-emerald-400 to-teal-600 text-neutral-950 font-black shadow-lg shadow-emerald-500/20">
            <Shield className="w-5 h-5 text-neutral-950" />
            <span className="absolute -top-0.5 -right-0.5 flex h-2.5 w-2.5">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-emerald-500"></span>
            </span>
          </div>
          <div>
            <div className="flex items-center space-x-1.5">
              <span className="text-base font-extrabold text-white tracking-wider font-mono">VEYLIX</span>
              <span className="px-1.5 py-0.5 text-[10px] font-mono font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 rounded">
                ENTERPRISE
              </span>
            </div>
            <p className="text-[10px] text-neutral-400 tracking-tight hidden sm:block">
              VPN Business & 3X-UI Cluster
            </p>
          </div>
        </div>

        {/* Global Node Health Badge */}
        <div className="hidden lg:flex items-center space-x-2 px-2.5 py-1 bg-neutral-900 border border-neutral-800 rounded-lg text-xs">
          <Activity className="w-3.5 h-3.5 text-emerald-400 animate-pulse" />
          <span className="text-neutral-300 font-medium">{onlineServers}/{servers.length} Nodes Online</span>
          <span className="text-neutral-500">•</span>
          <span className="text-emerald-400 font-mono text-[11px] font-semibold">{avgLatency}ms</span>
        </div>
      </div>

      {/* Center Search Input */}
      <div className="flex-1 max-w-xs xl:max-w-md mx-3 hidden md:block">
        <div className="relative">
          <Search className="w-4 h-4 text-neutral-400 absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search servers, clients, inbounds, UUIDs..."
            className="w-full pl-9 pr-4 py-1.5 bg-neutral-900/90 border border-neutral-800 rounded-xl text-xs text-neutral-200 placeholder-neutral-500 focus:outline-none focus:border-emerald-500/50 transition font-sans"
          />
        </div>
      </div>

      {/* Right Action Tools */}
      <div className="flex items-center space-x-2 sm:space-x-2.5">
        {/* 5 Main Master Themes Quick Switcher */}
        <div className="flex items-center p-1 bg-neutral-900 border border-neutral-800 rounded-xl space-x-1">
          {[
            { id: 'modern' as const, label: 'Pro', icon: Sparkles, tip: 'Professional Suite (Particle Network)' },
            { id: 'dark' as const, label: 'Dark', icon: Moon, tip: 'Dark Stealth (Deep Space Nebula)' },
            { id: 'light' as const, label: 'Light', icon: Sun, tip: 'Daylight Mode (Aurora Waves)' },
            { id: 'gaming' as const, label: 'Gaming', icon: Gamepad2, tip: 'Cyberpunk HUD (Tron Grid)' },
            { id: 'classic' as const, label: 'Classic', icon: Terminal, tip: 'Classical Unix (CRT Matrix Rain)' }
          ].map(themeItem => {
            const Icon = themeItem.icon;
            const isCurrent = themeConfig.preset === themeItem.id;
            return (
              <button
                key={themeItem.id}
                onClick={() => applyMasterPreset(themeItem.id)}
                className={`p-1.5 rounded-lg text-xs transition flex items-center space-x-1 ${
                  isCurrent
                    ? 'bg-emerald-500/20 text-emerald-300 font-semibold border border-emerald-500/40 shadow-sm'
                    : 'text-neutral-400 hover:text-white hover:bg-neutral-800'
                }`}
                title={themeItem.tip}
              >
                <Icon className="w-3.5 h-3.5" />
                <span className="hidden xl:inline text-[11px]">{themeItem.label}</span>
              </button>
            );
          })}
        </div>

        {/* Live Background VFX Animation Quick Switcher */}
        <div className="relative hidden lg:block">
          <button
            onClick={() => setVfxDropdownOpen(!vfxDropdownOpen)}
            className={`flex items-center space-x-1.5 px-2.5 py-1.5 border rounded-xl text-xs font-medium transition ${
              themeConfig.enableAnimations
                ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300 hover:bg-emerald-500/20'
                : 'bg-neutral-900 border-neutral-800 text-neutral-400 hover:text-white'
            }`}
            title="Interactive 60FPS Background VFX Animation"
          >
            <Zap className={`w-3.5 h-3.5 ${themeConfig.enableAnimations ? 'text-emerald-400 animate-pulse' : 'text-neutral-500'}`} />
            <span className="text-[11px] font-mono font-semibold">
              {themeConfig.enableAnimations 
                ? (themeConfig.bgAnimationType === 'cyber_grid' ? 'Cyber Grid' :
                   themeConfig.bgAnimationType === 'scanline_crt' ? 'Matrix Rain' :
                   themeConfig.bgAnimationType === 'subtle_orbs' ? 'Cosmic Space' :
                   themeConfig.bgAnimationType === 'aurora_wave' ? 'Aurora Wave' :
                   themeConfig.preset === 'gaming' ? 'Cyber Grid' :
                   themeConfig.preset === 'classic' ? 'Matrix Rain' :
                   themeConfig.preset === 'dark' ? 'Cosmic Space' :
                   themeConfig.preset === 'light' ? 'Aurora Wave' : 'Node Mesh')
                : 'VFX Disabled'}
            </span>
            <ChevronDown className="w-3 h-3 text-neutral-400" />
          </button>

          {vfxDropdownOpen && (
            <div className="absolute right-0 mt-2 w-72 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-2xl p-2 z-50 animate-in fade-in slide-in-from-top-2">
              <div className="px-3 py-1.5 border-b border-neutral-800 flex items-center justify-between">
                <div>
                  <span className="text-[10px] font-bold text-white uppercase font-mono">
                    {themeConfig.preset.toUpperCase()} VFX SUITE
                  </span>
                  <p className="text-[9px] text-neutral-400">Strictly matched to theme</p>
                </div>
                <button
                  onClick={() => setThemeConfig({ enableAnimations: !themeConfig.enableAnimations })}
                  className={`text-[10px] px-2 py-0.5 rounded font-bold ${
                    themeConfig.enableAnimations ? 'bg-emerald-500/20 text-emerald-400' : 'bg-neutral-800 text-neutral-400'
                  }`}
                >
                  {themeConfig.enableAnimations ? 'ON' : 'OFF'}
                </button>
              </div>

              <div className="p-1 space-y-1">
                {(
                  themeConfig.preset === 'gaming' ? [
                    { id: 'cyber_grid' as const, label: 'Cyberpunk Tron Grid', desc: 'Neon 3D horizon with shooting data packets' },
                    { id: 'gaming_warp' as const, label: 'Hyperspace Warp Speed', desc: 'Accelerating speed lines to cursor vanishing point' },
                    { id: 'gaming_hex' as const, label: 'Cyber Hex Shield Grid', desc: 'Hexagonal energy pulses & audio spectrum waves' }
                  ] :
                  themeConfig.preset === 'classic' ? [
                    { id: 'scanline_crt' as const, label: 'Matrix Digital Rain', desc: 'Phosphor green code rain with CRT scanlines' },
                    { id: 'classic_terminal' as const, label: 'Unix Boot Shell', desc: 'Live boot telemetry & memory hex addresses' },
                    { id: 'classic_oscilloscope' as const, label: 'Vector CRT Oscilloscope', desc: 'Harmonic green sine waves reacting to mouse' }
                  ] :
                  themeConfig.preset === 'dark' ? [
                    { id: 'subtle_orbs' as const, label: 'Deep Cosmic Nebula', desc: 'Breathing gas clouds, diamond stars & meteors' },
                    { id: 'dark_stealth_radar' as const, label: 'Covert 360° Radar Sweep', desc: 'Stealth scanner illuminating hidden VPN nodes' },
                    { id: 'dark_starfall' as const, label: 'Celestial Starfall Shower', desc: 'Parallax deep space star drift & meteors' }
                  ] :
                  themeConfig.preset === 'light' ? [
                    { id: 'aurora_wave' as const, label: 'Luminous Aurora Waves', desc: 'Smooth flowing daylight silk ribbons & bokeh' },
                    { id: 'light_caustics' as const, label: 'Prismatic Sunlight Caustics', desc: 'Sunlight refraction rays shimmering across view' },
                    { id: 'light_geometric' as const, label: 'Minimal Origami Geometry', desc: 'Floating isometric transparent wireframe polyhedra' }
                  ] : [
                    { id: 'particles' as const, label: 'VPN Node Constellation', desc: 'Global server nodes linked to cursor' },
                    { id: 'pro_worldmap' as const, label: 'World Route Map & Pings', desc: 'Live latency pins for Singapore, Frankfurt, Tokyo' },
                    { id: 'pro_fiber' as const, label: 'Fiber Optic Data Streams', desc: 'High-speed light buses & enterprise blueprint' }
                  ]
                ).map(item => {
                  const isCurrent = themeConfig.bgAnimationType === item.id;
                  return (
                    <button
                      key={item.id}
                      onClick={() => {
                        setThemeConfig({ bgAnimationType: item.id, enableAnimations: true });
                        setVfxDropdownOpen(false);
                      }}
                      className={`w-full text-left p-2 rounded-xl transition flex flex-col ${
                        isCurrent
                          ? 'bg-emerald-500/15 border border-emerald-500/30 text-emerald-300'
                          : 'hover:bg-neutral-800/70 text-neutral-300'
                      }`}
                    >
                      <div className="flex items-center justify-between">
                        <span className="font-semibold text-xs text-white">{item.label}</span>
                        {isCurrent && <span className="text-[10px] text-emerald-400 font-mono">ACTIVE</span>}
                      </div>
                      <span className="text-[10px] text-neutral-400">{item.desc}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          )}
        </div>

        {/* Theme Studio Trigger */}
        <button
          onClick={onOpenThemeStudio}
          className="hidden sm:flex items-center space-x-1.5 px-2.5 py-1.5 bg-neutral-900 hover:bg-neutral-800 border border-neutral-800 rounded-xl text-xs font-medium text-neutral-300 hover:text-white transition"
          title="Theme Studio: 5 Presets + 40 Subthemes"
        >
          <Palette className="w-3.5 h-3.5 text-cyan-400" />
          <span className="hidden md:inline capitalize">{themeConfig.subtheme.replace('_', ' ')}</span>
        </button>

        {/* Notifications Icon with Badge */}
        <button
          onClick={onToggleNotifications}
          className="relative p-2 bg-neutral-900 hover:bg-neutral-800 border border-neutral-800 rounded-xl text-neutral-300 hover:text-white transition"
          aria-label="Notifications"
        >
          <Bell className="w-4 h-4" />
          {unreadCount > 0 && (
            <span className="absolute -top-1 -right-1 w-4 h-4 bg-emerald-500 text-neutral-950 font-bold rounded-full text-[10px] flex items-center justify-center">
              {unreadCount}
            </span>
          )}
        </button>

        {/* User Profile & Role Switcher Menu */}
        <div className="relative">
          <button
            onClick={() => setRoleDropdownOpen(!roleDropdownOpen)}
            className="flex items-center space-x-2 pl-1.5 pr-2.5 py-1 bg-neutral-900 hover:bg-neutral-800/90 border border-neutral-800 rounded-xl transition"
          >
            <div className="w-7 h-7 rounded-lg bg-emerald-500/20 text-emerald-400 flex items-center justify-center font-bold text-xs uppercase overflow-hidden">
              {currentUser.avatar ? (
                <img src={currentUser.avatar} alt={currentUser.name} className="w-full h-full object-cover" />
              ) : (
                <UserIcon className="w-4 h-4" />
              )}
            </div>
            <div className="text-left hidden sm:block">
              <p className="text-xs font-semibold text-white leading-tight truncate max-w-[90px]">{currentUser.name}</p>
              <p className="text-[10px] text-emerald-400 uppercase font-mono font-medium">
                {currentRole.replace('_', ' ')}
              </p>
            </div>
            <ChevronDown className="w-3.5 h-3.5 text-neutral-400" />
          </button>

          {/* Switcher & Profile Dropdown */}
          {roleDropdownOpen && (
            <div className="absolute right-0 mt-2 w-72 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-2xl py-2 z-50 animate-in fade-in slide-in-from-top-2">
              <div className="px-4 py-2.5 border-b border-neutral-800 flex items-center justify-between">
                <div>
                  <p className="text-xs font-bold text-white">{currentUser.name}</p>
                  <p className="text-[11px] text-neutral-400 font-mono truncate max-w-[170px]">{currentUser.email}</p>
                </div>
                <button
                  onClick={() => {
                    onOpenProfile();
                    setRoleDropdownOpen(false);
                  }}
                  className="px-2.5 py-1 bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 rounded-lg text-xs font-semibold transition"
                >
                  Edit Profile
                </button>
              </div>

              <div className="px-4 py-2 border-b border-neutral-800">
                <p className="text-[10px] font-bold text-neutral-400 uppercase tracking-wider">Switch Simulated RBAC Role</p>
              </div>

              <div className="p-1 space-y-1">
                {[
                  { role: 'super_admin' as UserRole, title: 'Super Admin (Owner)', desc: 'Controls infrastructure, quotas, themes, billing' },
                  { role: 'admin' as UserRole, title: 'Reseller Admin (AlphaVPN)', desc: 'Manages own clients & quota allocation' },
                  { role: 'client' as UserRole, title: 'Client (Alex Rivers)', desc: 'Accesses active VPN keys & local QR codes' }
                ].map(item => (
                  <button
                    key={item.role}
                    onClick={() => {
                      switchRole(item.role);
                      setRoleDropdownOpen(false);
                    }}
                    className={`w-full text-left p-2.5 rounded-xl transition flex flex-col ${
                      currentRole === item.role 
                        ? 'bg-emerald-500/10 border border-emerald-500/30 text-emerald-300' 
                        : 'hover:bg-neutral-800/60 text-neutral-300'
                    }`}
                  >
                    <span className="font-semibold text-xs text-white">{item.title}</span>
                    <span className="text-[11px] text-neutral-400">{item.desc}</span>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </header>
  );
};
