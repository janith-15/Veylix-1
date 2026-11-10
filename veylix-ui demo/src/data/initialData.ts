import { 
  User, 
  ThreeXUIServer, 
  VpnAccount, 
  VpnPackage, 
  Order, 
  AuditLog, 
  ChatMessage, 
  SystemSettings, 
  ThemeConfig 
} from '../types';

export interface SubthemeDefinition {
  id: string;
  name: string;
  category: 'modern' | 'dark' | 'light' | 'classic' | 'gaming';
  primary: string;
  accent: string;
  bgDark: string;
  cardBg: string;
  surfaceBg: string;
  textColor: string;
  tag: string;
}

export const SUBTHEMES_CATALOG: SubthemeDefinition[] = [
  // Modern Category (8)
  { id: 'emerald_vault', name: 'Emerald Vault', category: 'modern', primary: '#10b981', accent: '#059669', bgDark: '#091511', cardBg: '#0f241d', surfaceBg: '#133026', textColor: '#ecfdf5', tag: 'Cyber Finance' },
  { id: 'cyan_surge', name: 'Cyan Surge', category: 'modern', primary: '#06b6d4', accent: '#0891b2', bgDark: '#08171f', cardBg: '#0e2634', surfaceBg: '#143447', textColor: '#f0fdfa', tag: 'Cloud Stream' },
  { id: 'indigo_prime', name: 'Indigo Prime', category: 'modern', primary: '#6366f1', accent: '#4f46e5', bgDark: '#0e1124', cardBg: '#161a38', surfaceBg: '#1e234c', textColor: '#eef2ff', tag: 'Enterprise' },
  { id: 'violet_pulse', name: 'Violet Pulse', category: 'modern', primary: '#8b5cf6', accent: '#7c3aed', bgDark: '#120f26', cardBg: '#1c173c', surfaceBg: '#251f4f', textColor: '#f5f3ff', tag: 'Deep Tech' },
  { id: 'sky_velocity', name: 'Sky Velocity', category: 'modern', primary: '#38bdf8', accent: '#0284c7', bgDark: '#081421', cardBg: '#0e2034', surfaceBg: '#152d49', textColor: '#f0f9ff', tag: 'Ultra Fast' },
  { id: 'teal_matrix', name: 'Teal Matrix', category: 'modern', primary: '#14b8a6', accent: '#0d9488', bgDark: '#071616', cardBg: '#0e2524', surfaceBg: '#143433', textColor: '#f0fdfa', tag: 'Encrypted' },
  { id: 'blue_nordic', name: 'Blue Nordic', category: 'modern', primary: '#2563eb', accent: '#1d4ed8', bgDark: '#0a1224', cardBg: '#101d3a', surfaceBg: '#172952', textColor: '#eff6ff', tag: 'Global CDN' },
  { id: 'slate_titan', name: 'Slate Titan', category: 'modern', primary: '#64748b', accent: '#475569', bgDark: '#0c1017', cardBg: '#151b26', surfaceBg: '#1e2636', textColor: '#f8fafc', tag: 'Minimal Pro' },

  // Dark Category (8)
  { id: 'obsidian_stealth', name: 'Obsidian Stealth', category: 'dark', primary: '#38bdf8', accent: '#818cf8', bgDark: '#06080c', cardBg: '#0e121a', surfaceBg: '#141a24', textColor: '#f8fafc', tag: 'Zero Trace' },
  { id: 'midnight_void', name: 'Midnight Void', category: 'dark', primary: '#a855f7', accent: '#c084fc', bgDark: '#05040a', cardBg: '#0e0b1c', surfaceBg: '#17122e', textColor: '#faf5ff', tag: 'Dark Orbit' },
  { id: 'carbon_graphite', name: 'Carbon Graphite', category: 'dark', primary: '#f59e0b', accent: '#d97706', bgDark: '#0b0c0e', cardBg: '#15171c', surfaceBg: '#20232a', textColor: '#fffbeb', tag: 'Carbon Fiber' },
  { id: 'phantom_black', name: 'Phantom Black', category: 'dark', primary: '#f43f5e', accent: '#e11d48', bgDark: '#0a0507', cardBg: '#170c10', surfaceBg: '#26141c', textColor: '#fff1f2', tag: 'Ghost Node' },
  { id: 'abyssal_deep', name: 'Abyssal Deep', category: 'dark', primary: '#2dd4bf', accent: '#0f766e', bgDark: '#030d0d', cardBg: '#081a1b', surfaceBg: '#0e2729', textColor: '#f0fdf4', tag: 'Trench Link' },
  { id: 'onyx_gold', name: 'Onyx Gold', category: 'dark', primary: '#fbbf24', accent: '#f59e0b', bgDark: '#0a0905', cardBg: '#18150c', surfaceBg: '#242013', textColor: '#fefce8', tag: 'VIP Vault' },
  { id: 'eclipse_mono', name: 'Eclipse Mono', category: 'dark', primary: '#e2e8f0', accent: '#94a3b8', bgDark: '#080808', cardBg: '#141414', surfaceBg: '#212121', textColor: '#f8fafc', tag: 'Pure Monolith' },
  { id: 'shadow_infrared', name: 'Shadow Infrared', category: 'dark', primary: '#ef4444', accent: '#b91c1c', bgDark: '#0d0404', cardBg: '#1a0909', surfaceBg: '#2a0f0f', textColor: '#fef2f2', tag: 'Covert Ops' },

  // Light Category (8)
  { id: 'arctic_cloud', name: 'Arctic Cloud', category: 'light', primary: '#0284c7', accent: '#0369a1', bgDark: '#f8fafc', cardBg: '#ffffff', surfaceBg: '#f1f5f9', textColor: '#0f172a', tag: 'Daylight Pro' },
  { id: 'pure_paper', name: 'Pure Paper', category: 'light', primary: '#4f46e5', accent: '#4338ca', bgDark: '#fcfcfc', cardBg: '#ffffff', surfaceBg: '#f4f4f5', textColor: '#18181b', tag: 'Clean Doc' },
  { id: 'frost_mint', name: 'Frost Mint', category: 'light', primary: '#059669', accent: '#047857', bgDark: '#f0fdf4', cardBg: '#ffffff', surfaceBg: '#e2f7eb', textColor: '#064e3b', tag: 'Eco Tunnel' },
  { id: 'morning_sky', name: 'Morning Sky', category: 'light', primary: '#0284c7', accent: '#0ea5e9', bgDark: '#f0f9ff', cardBg: '#ffffff', surfaceBg: '#e0f2fe', textColor: '#0c4a6e', tag: 'Breeze' },
  { id: 'ivory_gold', name: 'Ivory Gold', category: 'light', primary: '#b45309', accent: '#92400e', bgDark: '#fffbeb', cardBg: '#ffffff', surfaceBg: '#fef3c7', textColor: '#78350f', tag: 'Warm Glow' },
  { id: 'lavender_mist', name: 'Lavender Mist', category: 'light', primary: '#7c3aed', accent: '#6d28d9', bgDark: '#faf5ff', cardBg: '#ffffff', surfaceBg: '#f3e8ff', textColor: '#581c87', tag: 'Soft Lilac' },
  { id: 'rose_quartz', name: 'Rose Quartz', category: 'light', primary: '#e11d48', accent: '#be123c', bgDark: '#fff1f2', cardBg: '#ffffff', surfaceBg: '#ffe4e6', textColor: '#881337', tag: 'Pastel Ruby' },
  { id: 'sandstone_modern', name: 'Sandstone Modern', category: 'light', primary: '#78716c', accent: '#57534e', bgDark: '#fafaf9', cardBg: '#ffffff', surfaceBg: '#f5f5f4', textColor: '#292524', tag: 'Neutral Light' },

  // Classic Category (8)
  { id: 'classic_blue', name: 'Classic Blue', category: 'classic', primary: '#2563eb', accent: '#1d4ed8', bgDark: '#0f172a', cardBg: '#1e293b', surfaceBg: '#334155', textColor: '#f8fafc', tag: 'Sysadmin Def' },
  { id: 'terminal_green', name: 'Terminal Green', category: 'classic', primary: '#22c55e', accent: '#16a34a', bgDark: '#051208', cardBg: '#09210f', surfaceBg: '#103319', textColor: '#dcfce7', tag: 'Unix Green' },
  { id: 'amber_crt', name: 'Amber CRT', category: 'classic', primary: '#f59e0b', accent: '#d97706', bgDark: '#120d03', cardBg: '#211707', surfaceBg: '#33230a', textColor: '#fef3c7', tag: 'Vintage VT100' },
  { id: 'cisco_navy', name: 'Cisco Navy', category: 'classic', primary: '#0284c7', accent: '#0369a1', bgDark: '#0b1626', cardBg: '#13233c', surfaceBg: '#1d3356', textColor: '#f0f9ff', tag: 'Router Spec' },
  { id: 'telecom_gray', name: 'Telecom Gray', category: 'classic', primary: '#94a3b8', accent: '#64748b', bgDark: '#111317', cardBg: '#1a1d24', surfaceBg: '#272c37', textColor: '#f8fafc', tag: 'Rack Mount' },
  { id: 'bsd_daemon', name: 'BSD Crimson', category: 'classic', primary: '#dc2626', accent: '#b91c1c', bgDark: '#160808', cardBg: '#281111', surfaceBg: '#3d1a1a', textColor: '#fee2e2', tag: 'FreeBSD Daemon' },
  { id: 'debian_red', name: 'Debian Ruby', category: 'classic', primary: '#e11d48', accent: '#be123c', bgDark: '#16080e', cardBg: '#27101b', surfaceBg: '#3c182a', textColor: '#ffe4e6', tag: 'Debian Spiral' },
  { id: 'arch_cyan', name: 'Arch Cyan', category: 'classic', primary: '#0ea5e9', accent: '#0284c7', bgDark: '#0a1622', cardBg: '#102235', surfaceBg: '#193450', textColor: '#f0f9ff', tag: 'Rolling Release' },

  // Gaming Category (8)
  { id: 'cyberpunk_neon', name: 'Cyberpunk Neon', category: 'gaming', primary: '#f43f5e', accent: '#06b6d4', bgDark: '#080512', cardBg: '#140c2b', surfaceBg: '#1f1342', textColor: '#fff1f2', tag: 'Night City' },
  { id: 'tokyo_drift', name: 'Tokyo Drift', category: 'gaming', primary: '#d946ef', accent: '#8b5cf6', bgDark: '#0d041a', cardBg: '#1b0933', surfaceBg: '#2b0f52', textColor: '#fae8ff', tag: 'Synthwave' },
  { id: 'razer_matrix', name: 'Razer Matrix', category: 'gaming', primary: '#10b981', accent: '#34d399', bgDark: '#040d07', cardBg: '#091c10', surfaceBg: '#0f2f1b', textColor: '#ecfdf5', tag: 'Zero Ping' },
  { id: 'crimson_rog', name: 'Crimson ROG', category: 'gaming', primary: '#ef4444', accent: '#f87171', bgDark: '#0d0404', cardBg: '#1c0909', surfaceBg: '#2e0f0f', textColor: '#fef2f2', tag: 'Overclocked' },
  { id: 'hyper_solar', name: 'Hyper Solar', category: 'gaming', primary: '#f97316', accent: '#fbbf24', bgDark: '#120702', cardBg: '#241006', surfaceBg: '#381a0b', textColor: '#fff7ed', tag: 'Plasma Core' },
  { id: 'quantum_glow', name: 'Quantum Glow', category: 'gaming', primary: '#38bdf8', accent: '#a855f7', bgDark: '#070a18', cardBg: '#101530', surfaceBg: '#182047', textColor: '#f0f9ff', tag: 'Sub-1ms Ping' },
  { id: 'toxic_hazard', name: 'Toxic Hazard', category: 'gaming', primary: '#84cc16', accent: '#65a30d', bgDark: '#070f03', cardBg: '#102107', surfaceBg: '#1a350c', textColor: '#f7fee7', tag: 'Bio Surge' },
  { id: 'valkyrie_gold', name: 'Valkyrie Gold', category: 'gaming', primary: '#fbbf24', accent: '#f59e0b', bgDark: '#0f0c05', cardBg: '#211a0c', surfaceBg: '#352a13', textColor: '#fefce8', tag: 'Apex Champion' }
];

export const INITIAL_SYSTEM_SETTINGS: SystemSettings = {
  installationId: 'vyx-node-8504-prod-a7',
  appName: 'Veylix VPN Platform',
  appVersion: 'v2.8.4-enterprise',
  domain: 'vpn.veylix.network',
  sslActive: true,
  uptimeSeconds: 1248500,
  currency: 'USD',
  currencySymbol: '$',
  timezone: 'UTC+05:30 (Asia/Colombo)',
  freeVpnEnabled: true,
  freeVpnTrafficLimitGb: 5,
  freeVpnDurationDays: 3,
  freeVpnMaxPerUser: 1,
  telegramSupportHandle: '@VeylixSupportBot',
  whatsappSupportNumber: '+94 77 123 4567',
  supportEmail: 'ops@veylix.network',
  maintenanceMode: false,
  oneCommandInstallerUrl: 'curl -sSL https://get.veylix.io/install.sh | sudo bash -s -- --domain vpn.veylix.network --port 443'
};

export const INITIAL_USERS: User[] = [
  {
    id: 'user_super_admin',
    name: 'Janith (Owner)',
    email: 'Janith14dawan@gmail.com',
    role: 'super_admin',
    avatar: 'https://images.unsplash.com/photo-1534528741775-53994a69daeb?w=150&auto=format&fit=crop&q=80',
    phone: '+94 77 987 6543',
    telegram: '@janith_admin',
    whatsapp: '+94 77 987 6543',
    status: 'ACTIVE',
    createdAt: '2026-01-10T08:00:00Z',
    lastLogin: '2026-11-05T19:30:00Z'
  },
  {
    id: 'user_admin_alpha',
    name: 'AlphaVPN Reseller',
    email: 'reseller@alphavpn.lk',
    role: 'admin',
    avatar: 'https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?w=150&auto=format&fit=crop&q=80',
    phone: '+94 71 234 5678',
    telegram: '@alpha_reseller',
    whatsapp: '+94 71 234 5678',
    assignedServerIds: ['srv_sg_01', 'srv_fra_01', 'srv_tyo_02'],
    quotaLimit: 60,
    quotaUsed: 38,
    trafficLimitGb: 3000,
    status: 'ACTIVE',
    createdAt: '2026-02-15T11:20:00Z',
    lastLogin: '2026-11-05T18:15:00Z'
  },
  {
    id: 'user_admin_speedy',
    name: 'SpeedyNet Reseller',
    email: 'sales@speedynet.io',
    role: 'admin',
    avatar: 'https://images.unsplash.com/photo-1500648767791-00dcc994a43e?w=150&auto=format&fit=crop&q=80',
    phone: '+94 76 555 4321',
    telegram: '@speedynet_vpn',
    whatsapp: '+94 76 555 4321',
    assignedServerIds: ['srv_sg_01', 'srv_us_03'],
    quotaLimit: 40,
    quotaUsed: 22,
    trafficLimitGb: 2000,
    status: 'ACTIVE',
    createdAt: '2026-03-01T14:40:00Z',
    lastLogin: '2026-11-05T17:00:00Z'
  },
  {
    id: 'user_client_alex',
    name: 'Alex Rivers (Client)',
    email: 'alex.rivers@gmail.com',
    role: 'client',
    avatar: 'https://images.unsplash.com/photo-1492562080023-ab3db95bfbce?w=150&auto=format&fit=crop&q=80',
    phone: '+94 70 888 9999',
    telegram: '@alexrivers',
    assignedAdminId: 'user_admin_alpha',
    status: 'ACTIVE',
    createdAt: '2026-04-12T09:15:00Z',
    lastLogin: '2026-11-05T19:10:00Z'
  },
  {
    id: 'user_client_sarah',
    name: 'Sarah Chen',
    email: 'sarah.chen@outlook.com',
    role: 'client',
    avatar: 'https://images.unsplash.com/photo-1517841905240-472988babdf9?w=150&auto=format&fit=crop&q=80',
    phone: '+65 9123 4567',
    telegram: '@sarahchen_sg',
    assignedAdminId: 'user_admin_alpha',
    status: 'ACTIVE',
    createdAt: '2026-05-18T16:20:00Z',
    lastLogin: '2026-11-05T16:00:00Z'
  },
  {
    id: 'user_client_david',
    name: 'David Kumara',
    email: 'david.k@hotmail.com',
    role: 'client',
    avatar: 'https://images.unsplash.com/photo-1519085360753-af0119f7cbe7?w=150&auto=format&fit=crop&q=80',
    phone: '+94 72 333 4444',
    assignedAdminId: 'user_admin_speedy',
    status: 'ACTIVE',
    createdAt: '2026-07-22T10:05:00Z',
    lastLogin: '2026-11-04T12:00:00Z'
  }
];

export const INITIAL_SERVERS: ThreeXUIServer[] = [
  {
    id: 'srv_sg_01',
    name: 'Singapore SG-01 (Speed/Gaming)',
    host: 'sg01.veylix.network',
    port: 2053,
    panelPath: '/3x-ui',
    apiUrl: 'https://sg01.veylix.network:2053/3x-ui/server',
    location: 'Singapore (Changi Datacenter)',
    countryCode: 'SG',
    flag: '🇸🇬',
    status: 'ONLINE',
    latencyMs: 24,
    panelVersion: 'v2.4.2',
    xrayVersion: 'v1.8.28',
    cpuUsagePercent: 22,
    ramUsagePercent: 41,
    ramUsedMb: 820,
    ramTotalMb: 2000,
    diskUsagePercent: 35,
    lastSyncAt: '2026-11-05T19:25:00Z',
    syncStatus: 'SUCCESS',
    assignedAdminIds: ['user_admin_alpha', 'user_admin_speedy'],
    autoSync: true,
    inbounds: [
      {
        id: 1,
        port: 443,
        protocol: 'VLESS',
        remark: 'SG VLESS Reality Vision (TCP)',
        enable: true,
        network: 'tcp',
        security: 'reality',
        clientCount: 42,
        upTrafficBytes: 154000000000,
        downTrafficBytes: 890000000000,
        totalLimitGb: 5000
      },
      {
        id: 2,
        port: 8443,
        protocol: 'VMess',
        remark: 'SG VMess WebSocket TLS',
        enable: true,
        network: 'ws',
        security: 'tls',
        clientCount: 28,
        upTrafficBytes: 65000000000,
        downTrafficBytes: 420000000000,
        totalLimitGb: 3000
      }
    ]
  },
  {
    id: 'srv_fra_01',
    name: 'Frankfurt FRA-01 (EU Gateway)',
    host: 'fra01.veylix.network',
    port: 2053,
    panelPath: '/panel',
    apiUrl: 'https://fra01.veylix.network:2053/panel/server',
    location: 'Frankfurt, Germany (Equinix)',
    countryCode: 'DE',
    flag: '🇩🇪',
    status: 'ONLINE',
    latencyMs: 135,
    panelVersion: 'v2.4.2',
    xrayVersion: 'v1.8.28',
    cpuUsagePercent: 18,
    ramUsagePercent: 38,
    ramUsedMb: 760,
    ramTotalMb: 2000,
    diskUsagePercent: 29,
    lastSyncAt: '2026-11-05T19:20:00Z',
    syncStatus: 'SUCCESS',
    assignedAdminIds: ['user_admin_alpha'],
    autoSync: true,
    inbounds: [
      {
        id: 11,
        port: 443,
        protocol: 'Trojan',
        remark: 'FRA Trojan gRPC (TLS)',
        enable: true,
        network: 'grpc',
        security: 'tls',
        clientCount: 31,
        upTrafficBytes: 92000000000,
        downTrafficBytes: 580000000000,
        totalLimitGb: 4000
      },
      {
        id: 12,
        port: 2083,
        protocol: 'VLESS',
        remark: 'FRA VLESS Reality (TCP)',
        enable: true,
        network: 'tcp',
        security: 'reality',
        clientCount: 19,
        upTrafficBytes: 44000000000,
        downTrafficBytes: 290000000000,
        totalLimitGb: 3000
      }
    ]
  },
  {
    id: 'srv_tyo_02',
    name: 'Tokyo TYO-02 (Low Jitter Gaming)',
    host: 'tyo02.veylix.network',
    port: 2053,
    panelPath: '/3x-ui',
    apiUrl: 'https://tyo02.veylix.network:2053/3x-ui/server',
    location: 'Tokyo, Japan (SoftBank Core)',
    countryCode: 'JP',
    flag: '🇯🇵',
    status: 'ONLINE',
    latencyMs: 78,
    panelVersion: 'v2.3.9',
    xrayVersion: 'v1.8.24',
    cpuUsagePercent: 29,
    ramUsagePercent: 49,
    ramUsedMb: 980,
    ramTotalMb: 2000,
    diskUsagePercent: 41,
    lastSyncAt: '2026-11-05T19:22:00Z',
    syncStatus: 'SUCCESS',
    assignedAdminIds: ['user_admin_alpha'],
    autoSync: true,
    inbounds: [
      {
        id: 21,
        port: 443,
        protocol: 'Shadowsocks',
        remark: 'TYO Shadowsocks 2022-blake3',
        enable: true,
        network: 'tcp',
        security: 'none',
        clientCount: 24,
        upTrafficBytes: 110000000000,
        downTrafficBytes: 670000000000,
        totalLimitGb: 3500
      }
    ]
  },
  {
    id: 'srv_us_03',
    name: 'New York US-03 (Streaming VIP)',
    host: 'us03.veylix.network',
    port: 2053,
    panelPath: '/3x-ui',
    apiUrl: 'https://us03.veylix.network:2053/3x-ui/server',
    location: 'New York, USA (DigitalOcean)',
    countryCode: 'US',
    flag: '🇺🇸',
    status: 'ONLINE',
    latencyMs: 192,
    panelVersion: 'v2.4.2',
    xrayVersion: 'v1.8.28',
    cpuUsagePercent: 15,
    ramUsagePercent: 32,
    ramUsedMb: 640,
    ramTotalMb: 2000,
    diskUsagePercent: 22,
    lastSyncAt: '2026-11-05T19:18:00Z',
    syncStatus: 'SUCCESS',
    assignedAdminIds: ['user_admin_speedy'],
    autoSync: true,
    inbounds: [
      {
        id: 31,
        port: 443,
        protocol: 'VLESS',
        remark: 'US VLESS Reality Netflix Unblock',
        enable: true,
        network: 'tcp',
        security: 'reality',
        clientCount: 35,
        upTrafficBytes: 88000000000,
        downTrafficBytes: 740000000000,
        totalLimitGb: 6000
      }
    ]
  }
];

export const INITIAL_VPN_ACCOUNTS: VpnAccount[] = [
  {
    id: 'vpn_acc_01',
    uuid: '98a72b14-43ea-4c91-9128-4efc1341a980',
    name: 'Alex-SG-Reality-VIP',
    serverId: 'srv_sg_01',
    serverName: 'Singapore SG-01 (Speed/Gaming)',
    serverHost: 'sg01.veylix.network',
    inboundId: 1,
    protocol: 'VLESS',
    port: 443,
    flow: 'xtls-rprx-vision',
    security: 'reality',
    sni: 'www.microsoft.com',
    publicKey: '7K_d98W2H8vJ4N2a61XkL9rT3pQy8U0eWs',
    shortId: '6a8c4f',
    network: 'tcp',
    connectionUri: 'vless://98a72b14-43ea-4c91-9128-4efc1341a980@sg01.veylix.network:443?type=tcp&security=reality&pbk=7K_d98W2H8vJ4N2a61XkL9rT3pQy8U0eWs&fp=chrome&sni=www.microsoft.com&sid=6a8c4f&flow=xtls-rprx-vision#Alex-SG-Reality-VIP',
    isFree: false,
    status: 'ACTIVE',
    clientId: 'user_client_alex',
    clientName: 'Alex Rivers (Client)',
    adminId: 'user_admin_alpha',
    adminName: 'AlphaVPN Reseller',
    totalTrafficGb: 100,
    usedTrafficGb: 44.8,
    expiryDate: '2026-12-15T23:59:59Z',
    createdAt: '2026-10-15T09:00:00Z',
    provisionOutcome: 'CONFIRMED'
  },
  {
    id: 'vpn_acc_02',
    uuid: 'c5e62f01-7b93-4a12-88ef-31dfb85e0031',
    name: 'Alex-Tokyo-Gaming-SS',
    serverId: 'srv_tyo_02',
    serverName: 'Tokyo TYO-02 (Low Jitter Gaming)',
    serverHost: 'tyo02.veylix.network',
    inboundId: 21,
    protocol: 'Shadowsocks',
    port: 443,
    security: 'none',
    network: 'tcp',
    connectionUri: 'ss://MjAyMi1ibGFrZTMtYWVzLTI1Ni1nY206ZXhhbXBsZVBhc3N3b3JkMTIz@tyo02.veylix.network:443#Alex-Tokyo-Gaming-SS',
    isFree: false,
    status: 'ACTIVE',
    clientId: 'user_client_alex',
    clientName: 'Alex Rivers (Client)',
    adminId: 'user_admin_alpha',
    adminName: 'AlphaVPN Reseller',
    totalTrafficGb: 50,
    usedTrafficGb: 18.2,
    expiryDate: '2026-11-28T23:59:59Z',
    createdAt: '2026-10-28T14:30:00Z',
    provisionOutcome: 'CONFIRMED'
  },
  {
    id: 'vpn_acc_03',
    uuid: '11fe28ba-9943-4e20-9a3e-77cc65412899',
    name: 'Sarah-SG-VMess-TLS',
    serverId: 'srv_sg_01',
    serverName: 'Singapore SG-01 (Speed/Gaming)',
    serverHost: 'sg01.veylix.network',
    inboundId: 2,
    protocol: 'VMess',
    port: 8443,
    security: 'tls',
    path: '/veylix-ws',
    network: 'ws',
    connectionUri: 'vmess://eyJhZGQiOiJzZzAxLnZleWxpeC5uZXR3b3JrIiwiYWlkIjowLCJob3N0Ijoic2cwMS52ZXlsaXgubmV0d29yayIsImlkIjoiMTFmZTI4YmEtOTk0My00ZTIwLTlhM2UtNzdjYzY1NDEyODk5IiwibmV0Ijoid3MiLCJwYXRoIjoiL3ZleWxpeC13cyIsInBvcnQiOjg0NDMsInBzIjoiU2FyYWgtU0ctVk1lc3MtVExTIiwidGxzIjoidGxzIiwidHlwZSI6Im5vbmUifQ==',
    isFree: false,
    status: 'ACTIVE',
    clientId: 'user_client_sarah',
    clientName: 'Sarah Chen',
    adminId: 'user_admin_alpha',
    adminName: 'AlphaVPN Reseller',
    totalTrafficGb: 150,
    usedTrafficGb: 88.5,
    expiryDate: '2026-12-01T23:59:59Z',
    createdAt: '2026-09-01T11:00:00Z',
    provisionOutcome: 'CONFIRMED'
  },
  {
    id: 'vpn_acc_04',
    uuid: '88bc34dd-1212-4aa8-b190-234988771190',
    name: 'Starter-Free-Tier-David',
    serverId: 'srv_sg_01',
    serverName: 'Singapore SG-01 (Speed/Gaming)',
    serverHost: 'sg01.veylix.network',
    inboundId: 1,
    protocol: 'VLESS',
    port: 443,
    flow: 'xtls-rprx-vision',
    security: 'reality',
    sni: 'www.microsoft.com',
    publicKey: '7K_d98W2H8vJ4N2a61XkL9rT3pQy8U0eWs',
    shortId: '6a8c4f',
    network: 'tcp',
    connectionUri: 'vless://88bc34dd-1212-4aa8-b190-234988771190@sg01.veylix.network:443?type=tcp&security=reality&pbk=7K_d98W2H8vJ4N2a61XkL9rT3pQy8U0eWs&fp=chrome&sni=www.microsoft.com&sid=6a8c4f&flow=xtls-rprx-vision#Starter-Free-Tier-David',
    isFree: true,
    status: 'ACTIVE',
    clientId: 'user_client_david',
    clientName: 'David Kumara',
    adminId: 'user_admin_speedy',
    adminName: 'SpeedyNet Reseller',
    totalTrafficGb: 5,
    usedTrafficGb: 3.8,
    expiryDate: '2026-11-08T23:59:59Z',
    createdAt: '2026-11-04T12:00:00Z',
    provisionOutcome: 'CONFIRMED'
  },
  {
    id: 'vpn_acc_05',
    uuid: '33e14455-6677-4488-9900-aabbccddeeff',
    name: 'Unassigned-FRA-Trojan-Stock',
    serverId: 'srv_fra_01',
    serverName: 'Frankfurt FRA-01 (EU Gateway)',
    serverHost: 'fra01.veylix.network',
    inboundId: 11,
    protocol: 'Trojan',
    port: 443,
    security: 'tls',
    network: 'grpc',
    connectionUri: 'trojan://pass33e14455@fra01.veylix.network:443?security=tls&type=grpc&serviceName=veylix-grpc#Unassigned-FRA-Trojan-Stock',
    isFree: false,
    status: 'RESERVED',
    totalTrafficGb: 100,
    usedTrafficGb: 0,
    expiryDate: '2026-12-31T23:59:59Z',
    createdAt: '2026-11-01T08:00:00Z',
    provisionOutcome: 'CONFIRMED'
  }
];

export const INITIAL_PACKAGES: VpnPackage[] = [
  {
    id: 'pkg_free_trial',
    name: 'Starter Free Pass',
    tagline: 'Experience high-speed VLESS Reality with zero credit card required.',
    protocol: 'VLESS',
    trafficGb: 5,
    durationDays: 3,
    superAdminPrice: 0,
    retailPrice: 0,
    features: [
      'Singapore SG-01 High-Speed node',
      'VLESS Reality TLS Obfuscation',
      '5 GB High-Speed Data',
      'Instant Local QR Code',
      'Community Telegram Support'
    ],
    recommendedServers: ['srv_sg_01'],
    isFree: true,
    status: 'ACTIVE'
  },
  {
    id: 'pkg_pro_stream',
    name: 'Pro Streamer & Privacy 100GB',
    tagline: 'Flawless 4K streaming, zero throttling, geo-unblocking for Netflix & YouTube.',
    protocol: 'VLESS',
    trafficGb: 100,
    durationDays: 30,
    superAdminPrice: 3.50,
    retailPrice: 7.99,
    features: [
      'Singapore + Frankfurt + US Nodes',
      'VLESS Reality & VMess WebSocket',
      '100 GB High-Speed Quota',
      'Up to 5 Simultaneous Devices',
      'Priority 24/7 Support Channel',
      'Instant Auto-Provisioning'
    ],
    recommendedServers: ['srv_sg_01', 'srv_us_03', 'srv_fra_01'],
    isPopular: true,
    status: 'ACTIVE'
  },
  {
    id: 'pkg_gaming_turbo',
    name: 'Ultra Low-Ping Gaming 150GB',
    tagline: 'Optimized routing for Tokyo & Singapore with sub-30ms jitter buffer.',
    protocol: 'Shadowsocks',
    trafficGb: 150,
    durationDays: 30,
    superAdminPrice: 4.80,
    retailPrice: 11.99,
    features: [
      'Tokyo & Singapore Direct Peering',
      'Shadowsocks 2022-Blake3 Protocol',
      '150 GB Gaming Traffic',
      'Anti-DDoS Protected Nodes',
      'Low Jitter & Packet Loss Guarantee',
      'VIP Telegram Direct Chat'
    ],
    recommendedServers: ['srv_tyo_02', 'srv_sg_01'],
    status: 'ACTIVE'
  },
  {
    id: 'pkg_enterprise_unlimited',
    name: 'Business Enterprise 500GB',
    tagline: 'Dedicated port with static IP reservation and multi-country failover.',
    protocol: 'Trojan',
    trafficGb: 500,
    durationDays: 60,
    superAdminPrice: 12.00,
    retailPrice: 28.50,
    features: [
      'All 4 Global Datacenters (SG, FRA, TYO, US)',
      'Trojan gRPC + VLESS Reality Failover',
      '500 GB High-Speed Enterprise Pool',
      'Unlimited Device Connections',
      'Dedicated IP Reservation Option',
      'Direct WhatsApp & Email Support'
    ],
    recommendedServers: ['srv_sg_01', 'srv_fra_01', 'srv_tyo_02', 'srv_us_03'],
    status: 'ACTIVE'
  }
];

export const INITIAL_ORDERS: Order[] = [
  {
    id: 'ord_1001',
    orderNumber: 'VYX-2026-8911',
    packageId: 'pkg_pro_stream',
    packageName: 'Pro Streamer & Privacy 100GB',
    clientId: 'user_client_alex',
    clientName: 'Alex Rivers (Client)',
    adminId: 'user_admin_alpha',
    amount: 7.99,
    currency: 'USD',
    status: 'VERIFIED',
    paymentMethod: 'CRYPTO_USDT',
    proofNote: 'USDT TRC20 TX: 0x98f48b1239c88...192b',
    vpnAccountId: 'vpn_acc_01',
    createdAt: '2026-10-15T08:50:00Z',
    verifiedAt: '2026-10-15T09:00:00Z',
    verifierName: 'AlphaVPN Reseller'
  },
  {
    id: 'ord_1002',
    orderNumber: 'VYX-2026-8912',
    packageId: 'pkg_gaming_turbo',
    packageName: 'Ultra Low-Ping Gaming 150GB',
    clientId: 'user_client_alex',
    clientName: 'Alex Rivers (Client)',
    adminId: 'user_admin_alpha',
    amount: 11.99,
    currency: 'USD',
    status: 'VERIFIED',
    paymentMethod: 'PROMPTPAY',
    proofNote: 'Slip ref #982133 uploaded',
    vpnAccountId: 'vpn_acc_02',
    createdAt: '2026-10-28T14:20:00Z',
    verifiedAt: '2026-10-28T14:30:00Z',
    verifierName: 'AlphaVPN Reseller'
  },
  {
    id: 'ord_1003',
    orderNumber: 'VYX-2026-8913',
    packageId: 'pkg_enterprise_unlimited',
    packageName: 'Business Enterprise 500GB',
    clientId: 'user_client_sarah',
    clientName: 'Sarah Chen',
    adminId: 'user_admin_alpha',
    amount: 28.50,
    currency: 'USD',
    status: 'PENDING',
    paymentMethod: 'BANK_TRANSFER',
    proofImageUrl: 'https://images.unsplash.com/photo-1554224155-8d04cb21cd6c?w=400&auto=format&fit=crop&q=80',
    proofNote: 'Bank receipt wire from DBS Bank Singapore #DBS-881920',
    createdAt: '2026-11-05T17:45:00Z'
  }
];

export const INITIAL_AUDIT_LOGS: AuditLog[] = [
  {
    id: 'audit_01',
    timestamp: '2026-11-05T19:25:00Z',
    actorId: 'system',
    actorName: 'System Background Worker',
    actorRole: 'super_admin',
    action: 'SERVER_METRICS_SYNC',
    category: 'SERVER',
    details: 'Synced 4 3X-UI servers successfully. Ping range: 24ms - 192ms. All panels healthy.',
    ipAddress: '127.0.0.1',
    severity: 'INFO'
  },
  {
    id: 'audit_02',
    timestamp: '2026-11-05T18:15:20Z',
    actorId: 'user_admin_alpha',
    actorName: 'AlphaVPN Reseller',
    actorRole: 'admin',
    action: 'VPN_PROVISION_SUCCESS',
    category: 'VPN',
    details: 'Provisioned VLESS Reality config for Alex Rivers on srv_sg_01 (443). Deducted 1 from remaining quota.',
    ipAddress: '198.51.100.42',
    severity: 'INFO'
  },
  {
    id: 'audit_03',
    timestamp: '2026-11-05T17:50:10Z',
    actorId: 'user_client_sarah',
    actorName: 'Sarah Chen',
    actorRole: 'client',
    action: 'ORDER_SUBMITTED',
    category: 'ORDER',
    details: 'Submitted order VYX-2026-8913 for Business Enterprise 500GB ($28.50). Proof attached.',
    ipAddress: '203.0.113.15',
    severity: 'INFO'
  },
  {
    id: 'audit_04',
    timestamp: '2026-11-05T16:30:00Z',
    actorId: 'user_super_admin',
    actorName: 'Janith (Owner)',
    actorRole: 'super_admin',
    action: 'QUOTA_ADJUSTMENT',
    category: 'QUOTA',
    details: 'Adjusted quota for AlphaVPN Reseller from 50 to 60 accounts. Traffic pool updated to 3000 GB.',
    ipAddress: '192.168.1.1',
    severity: 'INFO'
  },
  {
    id: 'audit_05',
    timestamp: '2026-11-05T14:10:00Z',
    actorId: 'system',
    actorName: 'Security Monitor',
    actorRole: 'super_admin',
    action: 'SSRF_PROTECTION_PASSED',
    category: 'SYSTEM',
    details: 'Verified panel API endpoint https://sg01.veylix.network:2053. Internal loopback interception blocked.',
    ipAddress: '127.0.0.1',
    severity: 'INFO'
  }
];

export const INITIAL_CHAT_MESSAGES: ChatMessage[] = [
  {
    id: 'msg_01',
    senderId: 'user_admin_alpha',
    senderName: 'AlphaVPN Reseller',
    senderRole: 'admin',
    channelId: 'channel_alex',
    text: 'Hello Alex! Welcome to Veylix. Your VLESS Reality config is active with 100GB quota.',
    timestamp: '2026-10-15T09:05:00Z',
    isRead: true
  },
  {
    id: 'msg_02',
    senderId: 'user_client_alex',
    senderName: 'Alex Rivers (Client)',
    senderRole: 'client',
    channelId: 'channel_alex',
    text: 'Thank you! The ping to Singapore server is amazing (24ms). Can I import the QR code on v2rayNG on Android?',
    timestamp: '2026-10-15T09:12:00Z',
    isRead: true
  },
  {
    id: 'msg_03',
    senderId: 'user_admin_alpha',
    senderName: 'AlphaVPN Reseller',
    senderRole: 'admin',
    channelId: 'channel_alex',
    text: 'Yes! Just click the "Show QR Code" button in your dashboard and scan directly in v2rayNG or Streisand.',
    timestamp: '2026-10-15T09:15:00Z',
    isRead: true
  },
  {
    id: 'msg_04',
    senderId: 'user_client_sarah',
    senderName: 'Sarah Chen',
    senderRole: 'client',
    channelId: 'channel_sarah',
    text: 'Hi team, I just uploaded the wire transfer receipt for my Enterprise 500GB package. Please verify when possible.',
    timestamp: '2026-11-05T17:46:00Z',
    isRead: false,
    isImportant: true
  }
];
