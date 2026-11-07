export type UserRole = 'super_admin' | 'admin' | 'client';

export type VpnProtocol = 'VLESS' | 'VMess' | 'Trojan' | 'Shadowsocks';

export type VpnStatus = 'ACTIVE' | 'EXPIRED' | 'SUSPENDED' | 'RESERVED' | 'FREE' | 'REVOKED';

export type OrderStatus = 'PENDING' | 'VERIFIED' | 'REJECTED' | 'REFUNDED';

export type PaymentMethod = 'CRYPTO_USDT' | 'BANK_TRANSFER' | 'CREDIT_CARD' | 'PROMPTPAY' | 'CASH';

export interface User {
  id: string;
  name: string;
  email: string;
  role: UserRole;
  avatar?: string;
  phone?: string;
  telegram?: string;
  whatsapp?: string;
  assignedAdminId?: string; // If client, who is their reseller
  assignedServerIds?: string[]; // If admin, which servers they can access
  quotaLimit?: number; // Max active VPNs
  quotaUsed?: number;
  trafficLimitGb?: number;
  status: 'ACTIVE' | 'SUSPENDED' | 'INACTIVE';
  createdAt: string;
  lastLogin?: string;
}

export interface ThreeXUIInbound {
  id: number;
  port: number;
  protocol: VpnProtocol;
  remark: string;
  enable: boolean;
  network: 'tcp' | 'ws' | 'grpc';
  security: 'reality' | 'tls' | 'none';
  clientCount: number;
  upTrafficBytes: number;
  downTrafficBytes: number;
  totalLimitGb?: number;
}

export interface ThreeXUIServer {
  id: string;
  name: string;
  host: string;
  port: number;
  panelPath: string; // e.g. "/3x-ui" or "/panel"
  apiUrl: string;
  location: string;
  countryCode: string;
  flag: string;
  status: 'ONLINE' | 'OFFLINE' | 'DEGRADED';
  latencyMs: number;
  panelVersion: string;
  xrayVersion: string;
  cpuUsagePercent: number;
  ramUsagePercent: number;
  ramUsedMb: number;
  ramTotalMb: number;
  diskUsagePercent: number;
  lastSyncAt: string;
  syncStatus: 'SUCCESS' | 'ERROR' | 'SYNCING';
  assignedAdminIds: string[];
  inbounds: ThreeXUIInbound[];
  autoSync: boolean;
}

export interface VpnAccount {
  id: string;
  uuid: string; // UUID or Password
  name: string;
  serverId: string;
  serverName: string;
  serverHost: string;
  inboundId: number;
  protocol: VpnProtocol;
  port: number;
  flow?: string; // e.g. "xtls-rprx-vision"
  security: 'reality' | 'tls' | 'none';
  sni?: string;
  publicKey?: string;
  shortId?: string;
  path?: string;
  network: 'tcp' | 'ws' | 'grpc';
  connectionUri: string;
  isFree: boolean;
  status: VpnStatus;
  clientId?: string;
  clientName?: string;
  adminId?: string; // Reseller owner
  adminName?: string;
  totalTrafficGb: number;
  usedTrafficGb: number;
  expiryDate: string;
  createdAt: string;
  provisionOutcome: 'CONFIRMED' | 'REJECTED' | 'UNKNOWN';
}

export interface VpnPackage {
  id: string;
  name: string;
  tagline: string;
  protocol: VpnProtocol;
  trafficGb: number;
  durationDays: number;
  superAdminPrice: number; // Wholesale allocation cost
  retailPrice: number; // Customer retail price
  features: string[];
  recommendedServers: string[];
  isPopular?: boolean;
  isFree?: boolean;
  status: 'ACTIVE' | 'ARCHIVED';
}

export interface Order {
  id: string;
  orderNumber: string;
  packageId: string;
  packageName: string;
  clientId: string;
  clientName: string;
  adminId?: string; // Selling reseller
  amount: number;
  currency: string;
  status: OrderStatus;
  paymentMethod: PaymentMethod;
  proofImageUrl?: string;
  proofNote?: string;
  rejectionReason?: string;
  vpnAccountId?: string;
  createdAt: string;
  verifiedAt?: string;
  verifierName?: string;
}

export interface AuditLog {
  id: string;
  timestamp: string;
  actorId: string;
  actorName: string;
  actorRole: UserRole;
  action: string;
  category: 'AUTH' | 'SERVER' | 'VPN' | 'QUOTA' | 'ORDER' | 'SYSTEM';
  details: string;
  ipAddress: string;
  severity: 'INFO' | 'WARNING' | 'CRITICAL';
}

export interface ChatMessage {
  id: string;
  senderId: string;
  senderName: string;
  senderRole: UserRole;
  receiverId?: string; // Empty = broadcast/support channel
  channelId: string;
  text: string;
  timestamp: string;
  isRead: boolean;
  isImportant?: boolean;
}

export interface SystemSettings {
  installationId: string;
  appName: string;
  appVersion: string;
  domain: string;
  sslActive: boolean;
  uptimeSeconds: number;
  currency: string;
  currencySymbol: string;
  timezone: string;
  freeVpnEnabled: boolean;
  freeVpnTrafficLimitGb: number;
  freeVpnDurationDays: number;
  freeVpnMaxPerUser: number;
  telegramSupportHandle: string;
  whatsappSupportNumber: string;
  supportEmail: string;
  maintenanceMode: boolean;
  oneCommandInstallerUrl: string;
}

export interface ThemeConfig {
  preset: 'modern' | 'dark' | 'light' | 'classic' | 'gaming';
  subtheme: string;
  primaryColor: string;
  accentColor: string;
  bgDark: string;
  cardBg: string;
  surfaceBg: string;
  borderRadius: 'sm' | 'md' | 'lg' | 'xl' | 'full';
  enableAnimations: boolean;
  enableGlassmorphism: boolean;
  density: 'compact' | 'comfortable' | 'spacious';
  bgAnimationType: 
    | 'cyber_grid' 
    | 'gaming_warp' 
    | 'gaming_hex' 
    | 'scanline_crt' 
    | 'classic_terminal' 
    | 'classic_oscilloscope' 
    | 'particles' 
    | 'pro_worldmap' 
    | 'pro_fiber' 
    | 'subtle_orbs' 
    | 'dark_stealth_radar' 
    | 'dark_starfall' 
    | 'aurora_wave' 
    | 'light_caustics' 
    | 'light_geometric' 
    | 'none';
}
