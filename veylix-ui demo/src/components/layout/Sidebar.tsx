import React from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
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
  ShieldCheck,
  ChevronRight,
  TrendingUp,
  Cpu,
  X,
  UserCog
} from 'lucide-react';

interface SidebarProps {
  activeView: string;
  onSelectView: (view: string) => void;
  isMobileOpen?: boolean;
  onCloseMobile?: () => void;
}

interface NavItem {
  id: string;
  label: string;
  icon: any;
  badge?: string;
  badgeColor?: string;
}

interface NavSection {
  title: string;
  items: NavItem[];
}

export const Sidebar: React.FC<SidebarProps> = ({ 
  activeView, 
  onSelectView, 
  isMobileOpen, 
  onCloseMobile 
}) => {
  const { currentRole, currentUser, orders, vpnAccounts } = useVeylixStore();

  const pendingOrdersCount = orders.filter(o => o.status === 'PENDING').length;
  const activeKeysCount = vpnAccounts.filter(v => v.status === 'ACTIVE').length;

  const handleItemClick = (viewId: string) => {
    onSelectView(viewId);
    if (onCloseMobile) {
      onCloseMobile();
    }
  };

  // Build menu items based on current active role
  const getNavSections = (): NavSection[] => {
    if (currentRole === 'super_admin') {
      return [
        {
          title: 'Infrastructure & Nodes',
          items: [
            { id: 'dashboard', label: 'Master Dashboard', icon: LayoutDashboard },
            { id: 'servers', label: '3X-UI Servers', icon: Server, badge: '4' },
            { id: 'inventory', label: 'VPN Inventory', icon: KeyRound, badge: `${activeKeysCount}` }
          ]
        },
        {
          title: 'Accounts & Quotas',
          items: [
            { id: 'resellers', label: 'Resellers / Admins', icon: Users },
            { id: 'clients', label: 'Client Directory', icon: UserCheck }
          ]
        },
        {
          title: 'Commercial & Store',
          items: [
            { id: 'packages', label: 'Packages & Pricing', icon: Package },
            { id: 'orders', label: 'Orders & Payments', icon: CreditCard, badge: pendingOrdersCount > 0 ? `${pendingOrdersCount}` : undefined, badgeColor: 'bg-amber-500/20 text-amber-400' }
          ]
        },
        {
          title: 'Operations & Platform',
          items: [
            { id: 'chat', label: 'Support & Chat', icon: MessageSquare },
            { id: 'theme', label: 'Theme Studio (40 Themes)', icon: Palette },
            { id: 'profile', label: 'Profile & Security', icon: UserCog },
            { id: 'system', label: 'System & Independent Node', icon: Settings },
            { id: 'audit', label: 'Audit Security Log', icon: History }
          ]
        }
      ];
    } else if (currentRole === 'admin') {
      const remainingQuota = (currentUser.quotaLimit || 0) - (currentUser.quotaUsed || 0);
      return [
        {
          title: 'Reseller Portal',
          items: [
            { id: 'dashboard', label: 'Reseller Dashboard', icon: LayoutDashboard },
            { id: 'inventory', label: 'My VPN Inventory', icon: KeyRound, badge: `${remainingQuota} Left` },
            { id: 'clients', label: 'My Clients', icon: UserCheck },
            { id: 'orders', label: 'Client Orders', icon: CreditCard, badge: pendingOrdersCount > 0 ? `${pendingOrdersCount}` : undefined },
            { id: 'chat', label: 'Client Support Chat', icon: MessageSquare },
            { id: 'profile', label: 'Reseller Profile & Security', icon: UserCog }
          ]
        }
      ];
    } else {
      // Client role
      return [
        {
          title: 'Client Portal',
          items: [
            { id: 'dashboard', label: 'My Active VPNs & QR', icon: KeyRound },
            { id: 'client_free', label: 'Free VPN Pass', icon: Gift, badge: 'FREE' },
            { id: 'packages', label: 'VPN Store & Packages', icon: Package },
            { id: 'orders', label: 'Billing & Invoices', icon: CreditCard },
            { id: 'guide', label: 'Device Setup Guides', icon: BookOpen },
            { id: 'chat', label: 'Live Support Chat', icon: HelpCircle },
            { id: 'profile', label: 'My Profile & Security', icon: UserCog }
          ]
        }
      ];
    }
  };

  const sections = getNavSections();

  const sidebarContent = (
    <div className="flex flex-col justify-between h-full p-4 space-y-6">
      <div className="space-y-6">
        {/* Mobile Header with close button */}
        <div className="flex items-center justify-between md:hidden pb-2 border-b border-neutral-800">
          <div className="flex items-center space-x-2">
            <span className="font-mono font-bold text-white text-sm">VEYLIX MENU</span>
          </div>
          <button
            onClick={onCloseMobile}
            className="p-1.5 text-neutral-400 hover:text-white rounded-lg hover:bg-neutral-800 transition"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Role Identity Pill */}
        <div className="p-3 bg-neutral-900/90 border border-neutral-800 rounded-xl space-y-1">
          <div className="flex items-center justify-between text-[11px] text-neutral-400">
            <span>Operating Role:</span>
            <span className="font-mono text-emerald-400 font-semibold uppercase text-[10px]">
              {currentRole.replace('_', ' ')}
            </span>
          </div>
          <p className="text-xs font-semibold text-white truncate">{currentUser.name}</p>
          {currentRole === 'admin' && (
            <div className="pt-1.5 text-[11px] text-neutral-400 flex items-center justify-between">
              <span>Quota Used:</span>
              <span className="text-emerald-400 font-mono font-medium">
                {currentUser.quotaUsed || 0} / {currentUser.quotaLimit || 0} keys
              </span>
            </div>
          )}
        </div>

        {/* Navigation Sections */}
        <nav className="space-y-5">
          {sections.map((section, idx) => (
            <div key={idx} className="space-y-1.5">
              <p className="px-2 text-[10px] font-semibold text-neutral-500 uppercase tracking-wider font-mono">
                {section.title}
              </p>
              <div className="space-y-1">
                {section.items.map(item => {
                  const Icon = item.icon;
                  const isActive = activeView === item.id;
                  return (
                    <button
                      key={item.id}
                      onClick={() => handleItemClick(item.id)}
                      className={`w-full flex items-center justify-between px-3 py-2 rounded-xl text-xs font-medium transition group ${
                        isActive
                          ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-semibold shadow-sm'
                          : 'text-neutral-400 hover:text-neutral-100 hover:bg-neutral-900/80'
                      }`}
                    >
                      <div className="flex items-center space-x-2.5">
                        <Icon className={`w-4 h-4 transition ${isActive ? 'text-emerald-400' : 'text-neutral-500 group-hover:text-neutral-300'}`} />
                        <span>{item.label}</span>
                      </div>
                      {item.badge && (
                        <span className={`px-1.5 py-0.5 text-[10px] rounded font-mono font-bold ${item.badgeColor || 'bg-emerald-500/20 text-emerald-400'}`}>
                          {item.badge}
                        </span>
                      )}
                    </button>
                  );
                })}
              </div>
            </div>
          ))}
        </nav>
      </div>

      {/* Bottom Footer Info */}
      <div className="pt-4 border-t border-neutral-900 space-y-2">
        <div className="p-2.5 bg-neutral-900/50 rounded-xl border border-neutral-800/60 flex items-center justify-between text-[11px] text-neutral-400">
          <div className="flex items-center space-x-2">
            <Cpu className="w-3.5 h-3.5 text-emerald-400" />
            <span>Core v1.8.28</span>
          </div>
          <span className="text-neutral-500 font-mono">SQLite WAL</span>
        </div>
      </div>
    </div>
  );

  return (
    <>
      {/* Desktop Persistent Sidebar */}
      <aside className="w-64 shrink-0 bg-neutral-950/70 border-r border-neutral-800/80 hidden md:flex flex-col justify-between min-h-[calc(100vh-4rem)]">
        {sidebarContent}
      </aside>

      {/* Mobile Slide-over Drawer with Backdrop */}
      {isMobileOpen && (
        <div className="fixed inset-0 z-50 md:hidden flex">
          <div 
            onClick={onCloseMobile}
            className="fixed inset-0 bg-black/75 backdrop-blur-sm transition-opacity"
          />
          <div className="relative w-72 max-w-[85vw] bg-neutral-950 border-r border-neutral-800 h-full z-10 overflow-y-auto animate-in slide-in-from-left duration-200">
            {sidebarContent}
          </div>
        </div>
      )}
    </>
  );
};
