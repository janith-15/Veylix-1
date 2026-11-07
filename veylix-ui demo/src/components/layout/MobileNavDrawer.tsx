import React from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  X, 
  LayoutDashboard, 
  Server, 
  KeyRound, 
  Users, 
  UserCheck, 
  Package, 
  CreditCard, 
  MessageSquare, 
  Palette, 
  Settings, 
  History, 
  Gift, 
  BookOpen, 
  HelpCircle,
  Shield,
  User as UserIcon,
  Sparkles,
  Moon,
  Sun,
  Gamepad2,
  Terminal,
  Activity
} from 'lucide-react';
import { UserRole } from '../../types';

interface MobileNavDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  activeView: string;
  onSelectView: (view: string) => void;
  onOpenInstaller: () => void;
}

export const MobileNavDrawer: React.FC<MobileNavDrawerProps> = ({
  isOpen,
  onClose,
  activeView,
  onSelectView,
  onOpenInstaller
}) => {
  const { 
    currentRole, 
    currentUser, 
    switchRole, 
    themeConfig, 
    applyMasterPreset, 
    orders, 
    vpnAccounts 
  } = useVeylixStore();

  if (!isOpen) return null;

  const pendingOrdersCount = orders.filter(o => o.status === 'PENDING').length;
  const activeKeysCount = vpnAccounts.filter(v => v.status === 'ACTIVE').length;

  const getNavItems = () => {
    if (currentRole === 'super_admin') {
      return [
        { id: 'dashboard', label: 'Master Dashboard', icon: LayoutDashboard },
        { id: 'servers', label: '3X-UI Servers', icon: Server, badge: '4' },
        { id: 'inventory', label: 'VPN Inventory', icon: KeyRound, badge: `${activeKeysCount}` },
        { id: 'resellers', label: 'Resellers & Quotas', icon: Users },
        { id: 'clients', label: 'Client Directory', icon: UserCheck },
        { id: 'packages', label: 'Packages & Pricing', icon: Package },
        { id: 'orders', label: 'Orders & Payments', icon: CreditCard, badge: pendingOrdersCount > 0 ? `${pendingOrdersCount}` : undefined },
        { id: 'chat', label: 'Live Support Chat', icon: MessageSquare },
        { id: 'theme', label: 'Theme Studio (5 Themes + 40 Subthemes)', icon: Palette },
        { id: 'profile', label: 'My Account Profile', icon: UserIcon },
        { id: 'system', label: 'System & Independent Identity', icon: Settings },
        { id: 'audit', label: 'Security Audit Log', icon: History }
      ];
    } else if (currentRole === 'admin') {
      const remainingQuota = (currentUser.quotaLimit || 0) - (currentUser.quotaUsed || 0);
      return [
        { id: 'dashboard', label: 'Reseller Dashboard', icon: LayoutDashboard },
        { id: 'inventory', label: 'My VPN Inventory', icon: KeyRound, badge: `${remainingQuota} Left` },
        { id: 'clients', label: 'My Assigned Clients', icon: UserCheck },
        { id: 'orders', label: 'Client Orders Review', icon: CreditCard, badge: pendingOrdersCount > 0 ? `${pendingOrdersCount}` : undefined },
        { id: 'chat', label: 'Customer Support Chat', icon: MessageSquare },
        { id: 'profile', label: 'My Profile Settings', icon: UserIcon }
      ];
    } else {
      return [
        { id: 'dashboard', label: 'My Active VPNs & QR', icon: KeyRound },
        { id: 'client_free', label: 'Free VPN Pass', icon: Gift, badge: 'FREE' },
        { id: 'packages', label: 'VPN Packages Store', icon: Package },
        { id: 'orders', label: 'Billing & Invoices', icon: CreditCard },
        { id: 'guide', label: 'Device Connection Guides', icon: BookOpen },
        { id: 'chat', label: 'Support Live Chat', icon: HelpCircle },
        { id: 'profile', label: 'My Account Profile', icon: UserIcon }
      ];
    }
  };

  const navItems = getNavItems();

  return (
    <div className="fixed inset-0 z-50 flex bg-black/70 backdrop-blur-sm md:hidden animate-in fade-in duration-200">
      <div className="w-80 max-w-[85vw] bg-neutral-900 border-r border-neutral-800 h-full flex flex-col justify-between shadow-2xl animate-in slide-in-from-left duration-200">
        {/* Drawer Header */}
        <div className="p-4 border-b border-neutral-800 flex items-center justify-between bg-neutral-950/60">
          <div className="flex items-center space-x-2">
            <div className="w-8 h-8 rounded-xl bg-gradient-to-br from-emerald-400 to-teal-600 text-neutral-950 font-black flex items-center justify-center">
              <Shield className="w-4 h-4" />
            </div>
            <div>
              <p className="font-extrabold text-white text-sm tracking-wider font-mono">VEYLIX</p>
              <p className="text-[10px] text-neutral-400">Mobile Navigation</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 text-neutral-400 hover:text-white rounded-lg hover:bg-neutral-800 transition"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* 5 Themes Fast Switcher inside Drawer */}
        <div className="p-4 border-b border-neutral-800 space-y-2 bg-neutral-950/40">
          <span className="text-[10px] font-semibold text-neutral-400 uppercase tracking-wider font-mono">
            5 Core Themes:
          </span>
          <div className="grid grid-cols-5 gap-1.5">
            {[
              { id: 'modern' as const, label: 'Pro', icon: Sparkles },
              { id: 'dark' as const, label: 'Dark', icon: Moon },
              { id: 'light' as const, label: 'Light', icon: Sun },
              { id: 'gaming' as const, label: 'Game', icon: Gamepad2 },
              { id: 'classic' as const, label: 'Unix', icon: Terminal }
            ].map(t => {
              const Icon = t.icon;
              const isCur = themeConfig.preset === t.id;
              return (
                <button
                  key={t.id}
                  onClick={() => applyMasterPreset(t.id)}
                  className={`p-2 rounded-xl text-center flex flex-col items-center space-y-1 transition ${
                    isCur
                      ? 'bg-emerald-500/20 text-emerald-300 font-bold border border-emerald-500/40'
                      : 'bg-neutral-900 border border-neutral-800 text-neutral-400 hover:text-white'
                  }`}
                >
                  <Icon className="w-4 h-4" />
                  <span className="text-[10px]">{t.label}</span>
                </button>
              );
            })}
          </div>
        </div>

        {/* Navigation Items List */}
        <div className="flex-1 p-3 overflow-y-auto space-y-1">
          {navItems.map(item => {
            const Icon = item.icon;
            const isActive = activeView === item.id;
            return (
              <button
                key={item.id}
                onClick={() => {
                  onSelectView(item.id);
                  onClose();
                }}
                className={`w-full flex items-center justify-between p-3 rounded-xl text-xs font-medium transition ${
                  isActive
                    ? 'bg-emerald-500/15 text-emerald-300 border border-emerald-500/30 font-semibold'
                    : 'text-neutral-400 hover:text-white hover:bg-neutral-800/60'
                }`}
              >
                <div className="flex items-center space-x-3">
                  <Icon className={`w-4 h-4 ${isActive ? 'text-emerald-400' : 'text-neutral-500'}`} />
                  <span>{item.label}</span>
                </div>
                {item.badge && (
                  <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-emerald-500/20 text-emerald-400">
                    {item.badge}
                  </span>
                )}
              </button>
            );
          })}
        </div>

        {/* User Card & Role Switcher */}
        <div className="p-4 border-t border-neutral-800 bg-neutral-950/80 space-y-3">
          <div className="flex items-center space-x-2.5">
            <div className="w-8 h-8 rounded-xl overflow-hidden border border-emerald-500/30">
              <img src={currentUser.avatar} alt={currentUser.name} className="w-full h-full object-cover" />
            </div>
            <div className="truncate">
              <p className="text-xs font-bold text-white truncate">{currentUser.name}</p>
              <p className="text-[10px] text-emerald-400 font-mono uppercase">{currentRole.replace('_', ' ')}</p>
            </div>
          </div>

          <div className="grid grid-cols-3 gap-1 pt-1 text-[10px]">
            {(['super_admin', 'admin', 'client'] as UserRole[]).map(r => (
              <button
                key={r}
                onClick={() => switchRole(r)}
                className={`py-1 rounded font-mono uppercase text-center border transition ${
                  currentRole === r
                    ? 'bg-emerald-500/20 text-emerald-300 border-emerald-500/40 font-bold'
                    : 'bg-neutral-900 border-neutral-800 text-neutral-400'
                }`}
              >
                {r.split('_')[0]}
              </button>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};
