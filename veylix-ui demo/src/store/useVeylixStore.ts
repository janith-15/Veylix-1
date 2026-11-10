import { create } from 'zustand';
import { 
  User, 
  UserRole, 
  ThreeXUIServer, 
  ThreeXUIInbound,
  VpnAccount, 
  VpnPackage, 
  Order, 
  AuditLog, 
  ChatMessage, 
  SystemSettings, 
  ThemeConfig,
  VpnProtocol,
  VpnStatus
} from '../types';
import { 
  INITIAL_USERS, 
  INITIAL_SERVERS, 
  INITIAL_VPN_ACCOUNTS, 
  INITIAL_PACKAGES, 
  INITIAL_ORDERS, 
  INITIAL_AUDIT_LOGS, 
  INITIAL_CHAT_MESSAGES, 
  INITIAL_SYSTEM_SETTINGS, 
  SUBTHEMES_CATALOG 
} from '../data/initialData';

interface VeylixStoreState {
  // Current user & authentication
  currentUser: User;
  users: User[];
  currentRole: UserRole;
  setCurrentUser: (user: User) => void;
  switchRole: (role: UserRole) => void;
  addUser: (user: Omit<User, 'id' | 'createdAt'>) => void;
  updateUser: (id: string, updates: Partial<User>) => void;
  deleteUser: (id: string) => void;

  // 3X-UI Servers
  servers: ThreeXUIServer[];
  addServer: (serverData: Omit<ThreeXUIServer, 'id' | 'lastSyncAt' | 'syncStatus'>) => void;
  updateServer: (id: string, updates: Partial<ThreeXUIServer>) => void;
  deleteServer: (id: string) => boolean;
  syncServer: (id: string) => Promise<void>;
  testServerConnection: (host: string, port: number) => Promise<{ success: boolean; latency: number; version: string }>;

  // VPN Accounts & Inventory
  vpnAccounts: VpnAccount[];
  provisionVpnAccount: (data: {
    name: string;
    serverId: string;
    inboundId: number;
    protocol: VpnProtocol;
    clientId: string;
    totalTrafficGb: number;
    durationDays: number;
    isFree?: boolean;
    adminId?: string;
  }) => { success: boolean; account?: VpnAccount; error?: string };
  updateVpnStatus: (id: string, status: VpnStatus) => void;
  extendVpnExpiry: (id: string, extraDays: number, extraTrafficGb?: number) => void;
  deleteVpnAccount: (id: string) => void;
  claimFreeVpn: (clientId: string) => { success: boolean; account?: VpnAccount; error?: string };

  // Resellers & Quotas
  adjustAdminQuota: (adminId: string, newQuotaLimit: number, newTrafficLimitGb?: number) => boolean;

  // Packages & Store
  packages: VpnPackage[];
  addPackage: (pkg: Omit<VpnPackage, 'id'>) => void;
  updatePackage: (id: string, updates: Partial<VpnPackage>) => void;
  deletePackage: (id: string) => void;

  // Orders & Payments
  orders: Order[];
  createOrder: (data: {
    packageId: string;
    clientId: string;
    paymentMethod: Order['paymentMethod'];
    proofNote?: string;
    proofImageUrl?: string;
    adminId?: string;
  }) => Order;
  verifyOrder: (orderId: string, verifierName: string) => { success: boolean; error?: string };
  rejectOrder: (orderId: string, reason: string) => void;

  // Live Chat
  chatMessages: ChatMessage[];
  sendMessage: (senderId: string, text: string, channelId?: string, isImportant?: boolean) => void;
  markMessagesRead: (channelId: string) => void;

  // Audit Logs
  auditLogs: AuditLog[];
  logAudit: (data: {
    action: string;
    category: AuditLog['category'];
    details: string;
    severity?: AuditLog['severity'];
  }) => void;

  // System Settings & Installer
  systemSettings: SystemSettings;
  updateSystemSettings: (updates: Partial<SystemSettings>) => void;
  exportBackup: () => string;
  importBackup: (jsonString: string) => { success: boolean; message: string };
  resetToDefaults: () => void;

  // Theme Studio
  themeConfig: ThemeConfig;
  setThemeConfig: (updates: Partial<ThemeConfig>) => void;
  applySubtheme: (subthemeId: string) => void;
  applyMasterPreset: (preset: 'modern' | 'dark' | 'light' | 'classic' | 'gaming') => void;

  // Search & Navigation
  searchQuery: string;
  setSearchQuery: (q: string) => void;
  activeView: string;
  setActiveView: (view: string) => void;
}

const STORAGE_KEY = 'veylix_platform_state_v2';

const loadSavedState = () => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      return JSON.parse(raw);
    }
  } catch (err) {
    console.error('Failed to parse localStorage state:', err);
  }
  return null;
};

const defaultTheme: ThemeConfig = {
  preset: 'modern',
  subtheme: 'emerald_vault',
  primaryColor: '#10b981',
  accentColor: '#059669',
  bgDark: '#091511',
  cardBg: '#0f241d',
  surfaceBg: '#133026',
  borderRadius: 'lg',
  enableAnimations: true,
  enableGlassmorphism: true,
  density: 'comfortable',
  bgAnimationType: 'particles'
};

export const useVeylixStore = create<VeylixStoreState>((set: any, get: () => VeylixStoreState) => {
  const saved = loadSavedState();

  const initialState = {
    currentUser: saved?.currentUser || INITIAL_USERS[0],
    users: saved?.users || INITIAL_USERS,
    currentRole: (saved?.currentRole || 'super_admin') as UserRole,
    servers: saved?.servers || INITIAL_SERVERS,
    vpnAccounts: saved?.vpnAccounts || INITIAL_VPN_ACCOUNTS,
    packages: saved?.packages || INITIAL_PACKAGES,
    orders: saved?.orders || INITIAL_ORDERS,
    auditLogs: saved?.auditLogs || INITIAL_AUDIT_LOGS,
    chatMessages: saved?.chatMessages || INITIAL_CHAT_MESSAGES,
    systemSettings: saved?.systemSettings || INITIAL_SYSTEM_SETTINGS,
    themeConfig: saved?.themeConfig || defaultTheme,
    searchQuery: '',
    activeView: 'dashboard'
  };

  const persist = (partial: Partial<VeylixStoreState>) => {
    try {
      const current = get();
      const updated = { ...current, ...partial };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({
        currentUser: updated.currentUser,
        users: updated.users,
        currentRole: updated.currentRole,
        servers: updated.servers,
        vpnAccounts: updated.vpnAccounts,
        packages: updated.packages,
        orders: updated.orders,
        auditLogs: updated.auditLogs,
        chatMessages: updated.chatMessages,
        systemSettings: updated.systemSettings,
        themeConfig: updated.themeConfig
      }));
    } catch (err) {
      console.warn('Could not persist to localStorage:', err);
    }
  };

  return {
    ...initialState,

    setCurrentUser: (user) => {
      set({ currentUser: user, currentRole: user.role });
      persist({ currentUser: user, currentRole: user.role });
    },

    switchRole: (role) => {
      const matchedUser = get().users.find(u => u.role === role) || get().users[0];
      set({ currentRole: role, currentUser: matchedUser });
      persist({ currentRole: role, currentUser: matchedUser });
    },

    addUser: (userData) => {
      const newUser: User = {
        ...userData,
        id: `user_${Date.now()}_${Math.random().toString(36).substring(2, 6)}`,
        createdAt: new Date().toISOString()
      };
      const updatedUsers = [...get().users, newUser];
      set({ users: updatedUsers });
      persist({ users: updatedUsers });
      get().logAudit({
        action: 'USER_CREATED',
        category: 'AUTH',
        details: `Created user ${newUser.name} with role ${newUser.role}`,
        severity: 'INFO'
      });
    },

    updateUser: (id, updates) => {
      const updatedUsers = get().users.map(u => u.id === id ? { ...u, ...updates } : u);
      let currentUser = get().currentUser;
      if (currentUser.id === id) {
        currentUser = { ...currentUser, ...updates };
      }
      set({ users: updatedUsers, currentUser });
      persist({ users: updatedUsers, currentUser });
    },

    deleteUser: (id) => {
      const user = get().users.find(u => u.id === id);
      const updatedUsers = get().users.filter(u => u.id !== id);
      set({ users: updatedUsers });
      persist({ users: updatedUsers });
      get().logAudit({
        action: 'USER_DELETED',
        category: 'AUTH',
        details: `Deleted user ${user?.name || id}`,
        severity: 'WARNING'
      });
    },

    addServer: (serverData) => {
      const newServer: ThreeXUIServer = {
        ...serverData,
        id: `srv_${Date.now()}`,
        lastSyncAt: new Date().toISOString(),
        syncStatus: 'SUCCESS'
      };
      const updatedServers = [...get().servers, newServer];
      set({ servers: updatedServers });
      persist({ servers: updatedServers });
      get().logAudit({
        action: 'SERVER_CONNECTED',
        category: 'SERVER',
        details: `Added new 3X-UI server: ${newServer.name} (${newServer.host}:${newServer.port})`,
        severity: 'INFO'
      });
    },

    updateServer: (id, updates) => {
      const updatedServers = get().servers.map(s => s.id === id ? { ...s, ...updates } : s);
      set({ servers: updatedServers });
      persist({ servers: updatedServers });
    },

    deleteServer: (id) => {
      const hasActiveVpns = get().vpnAccounts.some(v => v.serverId === id && v.status === 'ACTIVE');
      if (hasActiveVpns) {
        return false;
      }
      const updatedServers = get().servers.filter(s => s.id !== id);
      set({ servers: updatedServers });
      persist({ servers: updatedServers });
      get().logAudit({
        action: 'SERVER_REMOVED',
        category: 'SERVER',
        details: `Removed server ${id}`,
        severity: 'WARNING'
      });
      return true;
    },

    syncServer: async (id) => {
      const server = get().servers.find(s => s.id === id);
      if (!server) return;

      // Mark syncing
      set({
        servers: get().servers.map(s => s.id === id ? { ...s, syncStatus: 'SYNCING' } : s)
      });

      // Simulate 3X-UI API roundtrip latency
      await new Promise(r => setTimeout(r, 600));

      const updatedServers = get().servers.map(s => {
        if (s.id !== id) return s;
        const newPing = Math.floor(Math.random() * 25) + 20;
        return {
          ...s,
          latencyMs: newPing,
          status: 'ONLINE' as const,
          syncStatus: 'SUCCESS' as const,
          lastSyncAt: new Date().toISOString(),
          cpuUsagePercent: Math.min(95, Math.max(10, s.cpuUsagePercent + (Math.floor(Math.random() * 9) - 4))),
          ramUsagePercent: Math.min(95, Math.max(20, s.ramUsagePercent + (Math.floor(Math.random() * 5) - 2)))
        };
      });

      set({ servers: updatedServers });
      persist({ servers: updatedServers });
      get().logAudit({
        action: 'SERVER_SYNC_SUCCESS',
        category: 'SERVER',
        details: `Synchronized panel telemetry for ${server.name} (Latency: ${server.latencyMs}ms)`,
        severity: 'INFO'
      });
    },

    testServerConnection: async (host, port) => {
      await new Promise(r => setTimeout(r, 500));
      return {
        success: true,
        latency: Math.floor(Math.random() * 30) + 25,
        version: '3X-UI v2.4.2 (Xray-core v1.8.28)'
      };
    },

    provisionVpnAccount: (data) => {
      const server = get().servers.find(s => s.id === data.serverId);
      if (!server) return { success: false, error: 'Target server not found' };

      const client = get().users.find(u => u.id === data.clientId);
      if (!client) return { success: false, error: 'Target client not found' };

      // Quota check if provisioned by admin/reseller
      if (data.adminId) {
        const admin = get().users.find(u => u.id === data.adminId);
        if (admin && admin.quotaLimit !== undefined) {
          const used = admin.quotaUsed || 0;
          if (used >= admin.quotaLimit) {
            return { 
              success: false, 
              error: `Quota exceeded: Reseller ${admin.name} has exhausted allocation (${used}/${admin.quotaLimit} accounts used).` 
            };
          }
        }
      }

      const generatedUuid = crypto.randomUUID ? crypto.randomUUID() : `uuid-${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
      const expiry = new Date();
      expiry.setDate(expiry.getDate() + data.durationDays);

      const inbound = server.inbounds.find(i => i.id === data.inboundId) || server.inbounds[0];
      const port = inbound?.port || 443;
      const cleanName = `${client.name.replace(/[^a-zA-Z0-9]/g, '')}-${server.countryCode}-${data.protocol}`;

      // Construct standard compliant V2Ray / Xray URIs
      let connectionUri = '';
      if (data.protocol === 'VLESS') {
        connectionUri = `vless://${generatedUuid}@${server.host}:${port}?type=tcp&security=reality&pbk=7K_d98W2H8vJ4N2a61XkL9rT3pQy8U0eWs&fp=chrome&sni=www.microsoft.com&sid=6a8c4f&flow=xtls-rprx-vision#${encodeURIComponent(cleanName)}`;
      } else if (data.protocol === 'VMess') {
        const vmessObj = {
          v: '2',
          ps: cleanName,
          add: server.host,
          port: port,
          id: generatedUuid,
          aid: '0',
          scy: 'auto',
          net: 'ws',
          type: 'none',
          host: server.host,
          path: '/veylix-ws',
          tls: 'tls'
        };
        connectionUri = `vmess://${btoa(JSON.stringify(vmessObj))}`;
      } else if (data.protocol === 'Trojan') {
        connectionUri = `trojan://${generatedUuid}@${server.host}:${port}?security=tls&type=grpc&serviceName=veylix-grpc#${encodeURIComponent(cleanName)}`;
      } else {
        const creds = btoa(`2022-blake3-aes-256-gcm:${generatedUuid}`);
        connectionUri = `ss://${creds}@${server.host}:${port}#${encodeURIComponent(cleanName)}`;
      }

      const newAccount: VpnAccount = {
        id: `vpn_${Date.now()}`,
        uuid: generatedUuid,
        name: cleanName,
        serverId: server.id,
        serverName: server.name,
        serverHost: server.host,
        inboundId: data.inboundId,
        protocol: data.protocol,
        port: port,
        security: inbound?.security || 'reality',
        network: inbound?.network || 'tcp',
        flow: data.protocol === 'VLESS' ? 'xtls-rprx-vision' : undefined,
        connectionUri,
        isFree: !!data.isFree,
        status: 'ACTIVE',
        clientId: client.id,
        clientName: client.name,
        adminId: data.adminId,
        adminName: data.adminId ? get().users.find(u => u.id === data.adminId)?.name : undefined,
        totalTrafficGb: data.totalTrafficGb,
        usedTrafficGb: 0,
        expiryDate: expiry.toISOString(),
        createdAt: new Date().toISOString(),
        provisionOutcome: 'CONFIRMED'
      };

      const updatedAccounts = [...get().vpnAccounts, newAccount];

      // Update admin quota usage if applicable
      let updatedUsers = get().users;
      if (data.adminId) {
        updatedUsers = updatedUsers.map(u => {
          if (u.id === data.adminId) {
            return { ...u, quotaUsed: (u.quotaUsed || 0) + 1 };
          }
          return u;
        });
      }

      set({ vpnAccounts: updatedAccounts, users: updatedUsers });
      persist({ vpnAccounts: updatedAccounts, users: updatedUsers });

      get().logAudit({
        action: 'VPN_PROVISIONED',
        category: 'VPN',
        details: `Provisioned ${data.protocol} key for client ${client.name} on server ${server.name}`,
        severity: 'INFO'
      });

      return { success: true, account: newAccount };
    },

    updateVpnStatus: (id, status) => {
      const updatedAccounts = get().vpnAccounts.map(v => v.id === id ? { ...v, status } : v);
      set({ vpnAccounts: updatedAccounts });
      persist({ vpnAccounts: updatedAccounts });
      get().logAudit({
        action: `VPN_${status}`,
        category: 'VPN',
        details: `Changed VPN account ${id} status to ${status}`,
        severity: 'INFO'
      });
    },

    extendVpnExpiry: (id, extraDays, extraTrafficGb = 0) => {
      const updatedAccounts = get().vpnAccounts.map(v => {
        if (v.id !== id) return v;
        const currentExp = new Date(v.expiryDate);
        currentExp.setDate(currentExp.getDate() + extraDays);
        return {
          ...v,
          expiryDate: currentExp.toISOString(),
          totalTrafficGb: v.totalTrafficGb + extraTrafficGb,
          status: 'ACTIVE' as const
        };
      });
      set({ vpnAccounts: updatedAccounts });
      persist({ vpnAccounts: updatedAccounts });
      get().logAudit({
        action: 'VPN_RENEWED',
        category: 'VPN',
        details: `Extended VPN account ${id} by ${extraDays} days and +${extraTrafficGb}GB`,
        severity: 'INFO'
      });
    },

    deleteVpnAccount: (id) => {
      const target = get().vpnAccounts.find(v => v.id === id);
      const updatedAccounts = get().vpnAccounts.filter(v => v.id !== id);

      // Decrement reseller quota if applicable
      let updatedUsers = get().users;
      if (target?.adminId) {
        updatedUsers = updatedUsers.map(u => {
          if (u.id === target.adminId) {
            return { ...u, quotaUsed: Math.max(0, (u.quotaUsed || 1) - 1) };
          }
          return u;
        });
      }

      set({ vpnAccounts: updatedAccounts, users: updatedUsers });
      persist({ vpnAccounts: updatedAccounts, users: updatedUsers });
      get().logAudit({
        action: 'VPN_DELETED',
        category: 'VPN',
        details: `Deleted VPN account ${target?.name || id}`,
        severity: 'WARNING'
      });
    },

    claimFreeVpn: (clientId) => {
      const settings = get().systemSettings;
      if (!settings.freeVpnEnabled) {
        return { success: false, error: 'Free VPN program is currently disabled by administrator' };
      }

      // Check if client already has an active free VPN
      const existingFree = get().vpnAccounts.filter(v => v.clientId === clientId && v.isFree && v.status === 'ACTIVE');
      if (existingFree.length >= settings.freeVpnMaxPerUser) {
        return { 
          success: false, 
          error: `Limit reached: You already have ${existingFree.length} active Free tier VPN pass(es).` 
        };
      }

      const defaultServer = get().servers[0];
      if (!defaultServer) {
        return { success: false, error: 'No available servers configured for free tier' };
      }

      return get().provisionVpnAccount({
        name: `FreePass-${Date.now()}`,
        serverId: defaultServer.id,
        inboundId: defaultServer.inbounds[0]?.id || 1,
        protocol: 'VLESS',
        clientId: clientId,
        totalTrafficGb: settings.freeVpnTrafficLimitGb,
        durationDays: settings.freeVpnDurationDays,
        isFree: true
      });
    },

    adjustAdminQuota: (adminId, newQuotaLimit, newTrafficLimitGb) => {
      const admin = get().users.find(u => u.id === adminId && u.role === 'admin');
      if (!admin) return false;

      const currentUsed = admin.quotaUsed || 0;
      if (newQuotaLimit < currentUsed) {
        // Enforce safety constraint: quota cannot be lower than active accounts
        return false;
      }

      const updatedUsers = get().users.map(u => {
        if (u.id === adminId) {
          return {
            ...u,
            quotaLimit: newQuotaLimit,
            trafficLimitGb: newTrafficLimitGb !== undefined ? newTrafficLimitGb : u.trafficLimitGb
          };
        }
        return u;
      });

      set({ users: updatedUsers });
      persist({ users: updatedUsers });
      get().logAudit({
        action: 'QUOTA_ALLOCATED',
        category: 'QUOTA',
        details: `Adjusted quota for ${admin.name} to ${newQuotaLimit} accounts (Used: ${currentUsed})`,
        severity: 'INFO'
      });
      return true;
    },

    addPackage: (pkg) => {
      const newPkg: VpnPackage = {
        ...pkg,
        id: `pkg_${Date.now()}`
      };
      const updatedPackages = [...get().packages, newPkg];
      set({ packages: updatedPackages });
      persist({ packages: updatedPackages });
    },

    updatePackage: (id, updates) => {
      const updatedPackages = get().packages.map(p => p.id === id ? { ...p, ...updates } : p);
      set({ packages: updatedPackages });
      persist({ packages: updatedPackages });
    },

    deletePackage: (id) => {
      const updatedPackages = get().packages.filter(p => p.id !== id);
      set({ packages: updatedPackages });
      persist({ packages: updatedPackages });
    },

    createOrder: (data) => {
      const pkg = get().packages.find(p => p.id === data.packageId);
      const client = get().users.find(u => u.id === data.clientId);

      const newOrder: Order = {
        id: `ord_${Date.now()}`,
        orderNumber: `VYX-${new Date().getFullYear()}-${Math.floor(1000 + Math.random() * 9000)}`,
        packageId: data.packageId,
        packageName: pkg?.name || 'VPN Package',
        clientId: data.clientId,
        clientName: client?.name || 'Customer',
        adminId: data.adminId || client?.assignedAdminId,
        amount: pkg?.retailPrice || 0,
        currency: get().systemSettings.currency,
        status: 'PENDING',
        paymentMethod: data.paymentMethod,
        proofNote: data.proofNote,
        proofImageUrl: data.proofImageUrl,
        createdAt: new Date().toISOString()
      };

      const updatedOrders = [newOrder, ...get().orders];
      set({ orders: updatedOrders });
      persist({ orders: updatedOrders });

      get().logAudit({
        action: 'ORDER_PLACED',
        category: 'ORDER',
        details: `Client ${newOrder.clientName} created order ${newOrder.orderNumber} for ${newOrder.packageName} ($${newOrder.amount})`,
        severity: 'INFO'
      });

      return newOrder;
    },

    verifyOrder: (orderId, verifierName) => {
      const order = get().orders.find(o => o.id === orderId);
      if (!order) return { success: false, error: 'Order not found' };
      if (order.status === 'VERIFIED') return { success: false, error: 'Order already verified' };

      const pkg = get().packages.find(p => p.id === order.packageId);
      if (!pkg) return { success: false, error: 'Linked package not found' };

      // Find best server for this package
      const candidateServerId = pkg.recommendedServers[0] || get().servers[0]?.id;
      const server = get().servers.find(s => s.id === candidateServerId) || get().servers[0];
      if (!server) return { success: false, error: 'No active server available for auto-provisioning' };

      // Provision VPN automatically
      const inbound = server.inbounds[0];
      const provisionResult = get().provisionVpnAccount({
        name: `${order.packageName.substring(0, 15)}-${order.orderNumber}`,
        serverId: server.id,
        inboundId: inbound?.id || 1,
        protocol: pkg.protocol,
        clientId: order.clientId,
        totalTrafficGb: pkg.trafficGb,
        durationDays: pkg.durationDays,
        adminId: order.adminId
      });

      if (!provisionResult.success) {
        return { success: false, error: `Provisioning failed: ${provisionResult.error}` };
      }

      // Mark order verified
      const updatedOrders = get().orders.map(o => {
        if (o.id === orderId) {
          return {
            ...o,
            status: 'VERIFIED' as const,
            verifiedAt: new Date().toISOString(),
            verifierName,
            vpnAccountId: provisionResult.account?.id
          };
        }
        return o;
      });

      set({ orders: updatedOrders });
      persist({ orders: updatedOrders });

      get().logAudit({
        action: 'ORDER_VERIFIED',
        category: 'ORDER',
        details: `Verified order ${order.orderNumber} and auto-provisioned VPN account ${provisionResult.account?.name}`,
        severity: 'INFO'
      });

      return { success: true };
    },

    rejectOrder: (orderId, reason) => {
      const updatedOrders = get().orders.map(o => {
        if (o.id === orderId) {
          return {
            ...o,
            status: 'REJECTED' as const,
            rejectionReason: reason
          };
        }
        return o;
      });
      set({ orders: updatedOrders });
      persist({ orders: updatedOrders });

      get().logAudit({
        action: 'ORDER_REJECTED',
        category: 'ORDER',
        details: `Rejected order ${orderId}. Reason: ${reason}`,
        severity: 'WARNING'
      });
    },

    sendMessage: (senderId, text, channelId = 'general', isImportant = false) => {
      const sender = get().users.find(u => u.id === senderId) || get().currentUser;
      const newMsg: ChatMessage = {
        id: `msg_${Date.now()}`,
        senderId: sender.id,
        senderName: sender.name,
        senderRole: sender.role,
        channelId,
        text,
        timestamp: new Date().toISOString(),
        isRead: false,
        isImportant
      };

      const updatedChat = [...get().chatMessages, newMsg];
      set({ chatMessages: updatedChat });
      persist({ chatMessages: updatedChat });
    },

    markMessagesRead: (channelId) => {
      const updatedChat = get().chatMessages.map(m => {
        if (m.channelId === channelId) {
          return { ...m, isRead: true };
        }
        return m;
      });
      set({ chatMessages: updatedChat });
      persist({ chatMessages: updatedChat });
    },

    logAudit: (data) => {
      const actor = get().currentUser;
      const newLog: AuditLog = {
        id: `audit_${Date.now()}_${Math.random().toString(36).substring(2, 5)}`,
        timestamp: new Date().toISOString(),
        actorId: actor?.id || 'system',
        actorName: actor?.name || 'System',
        actorRole: actor?.role || 'super_admin',
        action: data.action,
        category: data.category,
        details: data.details,
        ipAddress: '127.0.0.1',
        severity: data.severity || 'INFO'
      };

      const updatedLogs = [newLog, ...get().auditLogs.slice(0, 150)];
      set({ auditLogs: updatedLogs });
      persist({ auditLogs: updatedLogs });
    },

    updateSystemSettings: (updates) => {
      const updatedSettings = { ...get().systemSettings, ...updates };
      set({ systemSettings: updatedSettings });
      persist({ systemSettings: updatedSettings });
      get().logAudit({
        action: 'SYSTEM_SETTINGS_UPDATED',
        category: 'SYSTEM',
        details: 'Super Admin updated platform configuration',
        severity: 'INFO'
      });
    },

    exportBackup: () => {
      const state = get();
      const backupManifest = {
        veylixBackupVersion: '2.8',
        installationId: state.systemSettings.installationId,
        exportedAt: new Date().toISOString(),
        data: {
          users: state.users,
          servers: state.servers,
          vpnAccounts: state.vpnAccounts,
          packages: state.packages,
          orders: state.orders,
          systemSettings: state.systemSettings,
          auditLogs: state.auditLogs
        }
      };
      return JSON.stringify(backupManifest, null, 2);
    },

    importBackup: (jsonString) => {
      try {
        const parsed = JSON.parse(jsonString);
        if (!parsed.veylixBackupVersion || !parsed.data) {
          return { success: false, message: 'Invalid Veylix backup manifest format.' };
        }

        const data = parsed.data;
        const newUsers = data.users || get().users;
        const newServers = data.servers || get().servers;
        const newAccounts = data.vpnAccounts || get().vpnAccounts;
        const newPackages = data.packages || get().packages;
        const newOrders = data.orders || get().orders;
        const newSettings = data.systemSettings || get().systemSettings;

        set({
          users: newUsers,
          servers: newServers,
          vpnAccounts: newAccounts,
          packages: newPackages,
          orders: newOrders,
          systemSettings: newSettings
        });

        persist({
          users: newUsers,
          servers: newServers,
          vpnAccounts: newAccounts,
          packages: newPackages,
          orders: newOrders,
          systemSettings: newSettings
        });

        get().logAudit({
          action: 'BACKUP_RESTORED',
          category: 'SYSTEM',
          details: `Restored backup from manifest exported at ${parsed.exportedAt}`,
          severity: 'CRITICAL'
        });

        return { success: true, message: 'Backup restored successfully! All tables updated.' };
      } catch (err: any) {
        return { success: false, message: `Failed to restore: ${err?.message || 'Invalid JSON'}` };
      }
    },

    resetToDefaults: () => {
      localStorage.removeItem(STORAGE_KEY);
      set({
        currentUser: INITIAL_USERS[0],
        users: INITIAL_USERS,
        currentRole: 'super_admin',
        servers: INITIAL_SERVERS,
        vpnAccounts: INITIAL_VPN_ACCOUNTS,
        packages: INITIAL_PACKAGES,
        orders: INITIAL_ORDERS,
        auditLogs: INITIAL_AUDIT_LOGS,
        chatMessages: INITIAL_CHAT_MESSAGES,
        systemSettings: INITIAL_SYSTEM_SETTINGS,
        themeConfig: defaultTheme
      });
      get().logAudit({
        action: 'FACTORY_RESET',
        category: 'SYSTEM',
        details: 'Restored Veylix installation to default seed baseline',
        severity: 'CRITICAL'
      });
    },

    setThemeConfig: (updates) => {
      const updatedTheme = { ...get().themeConfig, ...updates };
      set({ themeConfig: updatedTheme });
      persist({ themeConfig: updatedTheme });
    },

    applySubtheme: (subthemeId) => {
      const found = SUBTHEMES_CATALOG.find(s => s.id === subthemeId);
      if (!found) return;

      const updatedTheme: ThemeConfig = {
        ...get().themeConfig,
        preset: found.category,
        subtheme: found.id,
        primaryColor: found.primary,
        accentColor: found.accent,
        bgDark: found.bgDark,
        cardBg: found.cardBg,
        surfaceBg: found.surfaceBg
      };

      set({ themeConfig: updatedTheme });
      persist({ themeConfig: updatedTheme });
    },

    applyMasterPreset: (preset: 'modern' | 'dark' | 'light' | 'classic' | 'gaming') => {
      let subthemeId = 'emerald_vault';
      let animType: ThemeConfig['bgAnimationType'] = 'particles';

      if (preset === 'dark') {
        subthemeId = 'obsidian_stealth';
        animType = 'subtle_orbs';
      } else if (preset === 'light') {
        subthemeId = 'arctic_cloud';
        animType = 'aurora_wave';
      } else if (preset === 'gaming') {
        subthemeId = 'cyberpunk_neon';
        animType = 'cyber_grid';
      } else if (preset === 'classic') {
        subthemeId = 'terminal_green';
        animType = 'scanline_crt';
      } else {
        subthemeId = 'emerald_vault';
        animType = 'particles';
      }

      const found = SUBTHEMES_CATALOG.find(s => s.id === subthemeId) || SUBTHEMES_CATALOG[0];
      const updatedTheme: ThemeConfig = {
        ...get().themeConfig,
        preset,
        subtheme: found.id,
        primaryColor: found.primary,
        accentColor: found.accent,
        bgDark: found.bgDark,
        cardBg: found.cardBg,
        surfaceBg: found.surfaceBg,
        bgAnimationType: animType
      };

      set({ themeConfig: updatedTheme });
      persist({ themeConfig: updatedTheme });
    },

    setSearchQuery: (q: string) => set({ searchQuery: q }),
    setActiveView: (view: string) => set({ activeView: view })
  };
});
