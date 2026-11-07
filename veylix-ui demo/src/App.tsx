import React, { useState, useEffect } from 'react';
import { useVeylixStore } from './store/useVeylixStore';
import { Navbar } from './components/layout/Navbar';
import { Sidebar } from './components/layout/Sidebar';
import { NotificationDrawer } from './components/layout/NotificationDrawer';
import { BackgroundAnimation } from './components/layout/BackgroundAnimation';

// Modals
import { QrCodeModal } from './components/modals/QrCodeModal';
import { AddServerModal } from './components/modals/AddServerModal';
import { ProvisionVpnModal } from './components/modals/ProvisionVpnModal';
import { PaymentProofModal } from './components/modals/PaymentProofModal';
import { ClientSetupGuideModal } from './components/modals/ClientSetupGuideModal';
import { OneCommandInstallerModal } from './components/modals/OneCommandInstallerModal';
import { BackupRestoreModal } from './components/modals/BackupRestoreModal';
import { OrderPackageModal } from './components/modals/OrderPackageModal';

// Views
import { SuperAdminDashboard } from './components/views/SuperAdminDashboard';
import { ServerManagementView } from './components/views/ServerManagementView';
import { VpnInventoryView } from './components/views/VpnInventoryView';
import { AdminManagementView } from './components/views/AdminManagementView';
import { ClientManagementView } from './components/views/ClientManagementView';
import { BusinessStoreView } from './components/views/BusinessStoreView';
import { OrdersPaymentsView } from './components/views/OrdersPaymentsView';
import { ThemeStudioView } from './components/views/ThemeStudioView';
import { SystemSettingsView } from './components/views/SystemSettingsView';
import { ClientPortalView } from './components/views/ClientPortalView';
import { LiveChatView } from './components/views/LiveChatView';
import { ProfileSettingsView } from './components/views/ProfileSettingsView';

import { VpnAccount, Order, VpnPackage } from './types';

export default function App() {
  const { 
    currentRole, 
    themeConfig, 
    activeView, 
    setActiveView,
    orders,
    claimFreeVpn,
    currentUser
  } = useVeylixStore();

  // Modal states
  const [selectedVpnForQr, setSelectedVpnForQr] = useState<VpnAccount | null>(null);
  const [isAddServerOpen, setIsAddServerOpen] = useState(false);
  const [isProvisionOpen, setIsProvisionOpen] = useState(false);
  const [preselectedClientId, setPreselectedClientId] = useState<string | undefined>(undefined);
  const [selectedOrderForReview, setSelectedOrderForReview] = useState<Order | null>(null);
  const [isInstallerOpen, setIsInstallerOpen] = useState(false);
  const [isBackupOpen, setIsBackupOpen] = useState(false);
  const [isSetupGuideOpen, setIsSetupGuideOpen] = useState(false);
  const [selectedPackageForOrder, setSelectedPackageForOrder] = useState<VpnPackage | null>(null);
  const [isNotificationsOpen, setIsNotificationsOpen] = useState(false);
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

  const pendingOrdersCount = orders.filter(o => o.status === 'PENDING').length;

  // Handle Free VPN claim quick action from nav
  useEffect(() => {
    if (activeView === 'client_free') {
      const res = claimFreeVpn(currentUser.id);
      if (res.account) {
        setSelectedVpnForQr(res.account);
      }
      setActiveView('dashboard');
    } else if (activeView === 'guide') {
      setIsSetupGuideOpen(true);
      setActiveView('dashboard');
    }
  }, [activeView]);

  // Set CSS custom variables & dark class for active subtheme
  useEffect(() => {
    document.documentElement.style.setProperty('--primary-color', themeConfig.primaryColor);
    document.documentElement.style.setProperty('--accent-color', themeConfig.accentColor);
    document.documentElement.style.setProperty('--bg-dark', themeConfig.bgDark);

    if (themeConfig.preset === 'light') {
      document.documentElement.classList.remove('dark');
    } else {
      document.documentElement.classList.add('dark');
    }
  }, [themeConfig]);

  // Open provision modal with preselected client
  const handleOpenProvisionForClient = (clientId: string) => {
    setPreselectedClientId(clientId);
    setIsProvisionOpen(true);
  };

  // Render main content area based on current active view and role
  const renderViewContent = () => {
    // If Client role is active and user is on dashboard, render the ClientPortalView
    if (currentRole === 'client' && activeView === 'dashboard') {
      return (
        <ClientPortalView
          onSelectVpnForQr={(vpn) => setSelectedVpnForQr(vpn)}
          onOpenSetupGuide={() => setIsSetupGuideOpen(true)}
          onNavigateToStore={() => setActiveView('packages')}
          onOpenChat={() => setActiveView('chat')}
        />
      );
    }

    switch (activeView) {
      case 'dashboard':
        return (
          <SuperAdminDashboard
            onOpenAddServer={() => setIsAddServerOpen(true)}
            onOpenProvision={() => {
              setPreselectedClientId(undefined);
              setIsProvisionOpen(true);
            }}
            onOpenInstaller={() => setIsInstallerOpen(true)}
            onSelectVpnForQr={(vpn) => setSelectedVpnForQr(vpn)}
            onNavigate={(view) => setActiveView(view)}
          />
        );

      case 'servers':
        return (
          <ServerManagementView
            onOpenAddServer={() => setIsAddServerOpen(true)}
            onOpenProvision={() => {
              setPreselectedClientId(undefined);
              setIsProvisionOpen(true);
            }}
          />
        );

      case 'inventory':
        return (
          <VpnInventoryView
            onOpenProvision={() => {
              setPreselectedClientId(undefined);
              setIsProvisionOpen(true);
            }}
            onSelectVpnForQr={(vpn) => setSelectedVpnForQr(vpn)}
          />
        );

      case 'resellers':
        return (
          <AdminManagementView
            onOpenChat={() => setActiveView('chat')}
          />
        );

      case 'clients':
        return (
          <ClientManagementView
            onOpenProvisionForClient={handleOpenProvisionForClient}
            onOpenChat={() => setActiveView('chat')}
            onSelectVpnForQr={(vpn) => setSelectedVpnForQr(vpn)}
          />
        );

      case 'packages':
        return (
          <BusinessStoreView
            onOrderPackage={(pkg) => setSelectedPackageForOrder(pkg)}
          />
        );

      case 'orders':
        return (
          <OrdersPaymentsView
            onSelectOrderForReview={(order) => setSelectedOrderForReview(order)}
          />
        );

      case 'theme':
        return <ThemeStudioView />;

      case 'profile':
        return <ProfileSettingsView />;

      case 'system':
      case 'audit':
        return (
          <SystemSettingsView
            onOpenInstaller={() => setIsInstallerOpen(true)}
            onOpenBackupModal={() => setIsBackupOpen(true)}
          />
        );

      case 'chat':
        return <LiveChatView />;

      default:
        return (
          <SuperAdminDashboard
            onOpenAddServer={() => setIsAddServerOpen(true)}
            onOpenProvision={() => {
              setPreselectedClientId(undefined);
              setIsProvisionOpen(true);
            }}
            onOpenInstaller={() => setIsInstallerOpen(true)}
            onSelectVpnForQr={(vpn) => setSelectedVpnForQr(vpn)}
            onNavigate={(view) => setActiveView(view)}
          />
        );
    }
  };

  return (
    <div 
      className="min-h-screen flex flex-col font-sans antialiased selection:bg-emerald-500 selection:text-black transition-colors duration-300 relative"
      style={{ 
        backgroundColor: themeConfig.bgDark || (themeConfig.preset === 'light' ? '#f8fafc' : '#090a0f'),
        color: themeConfig.preset === 'light' ? '#0f172a' : '#f8fafc' 
      }}
    >
      {/* Dynamic Ambient Animated Background */}
      <BackgroundAnimation />

      {/* Top Navbar */}
      <div className="relative z-20">
        <Navbar
          onOpenInstaller={() => setIsInstallerOpen(true)}
          onOpenThemeStudio={() => setActiveView('theme')}
          onOpenProfile={() => setActiveView('profile')}
          onToggleNotifications={() => setIsNotificationsOpen(true)}
          onToggleMobileMenu={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
          unreadCount={pendingOrdersCount}
        />
      </div>

      {/* Main Layout Area */}
      <div className="flex-1 flex overflow-hidden relative z-10">
        {/* Sidebar */}
        <Sidebar
          activeView={activeView}
          onSelectView={(v) => setActiveView(v)}
          isMobileOpen={isMobileMenuOpen}
          onCloseMobile={() => setIsMobileMenuOpen(false)}
        />

        {/* Content View Container */}
        <main className="flex-1 p-4 lg:p-8 overflow-y-auto max-h-[calc(100vh-4rem)]">
          <div className="max-w-7xl mx-auto">
            {renderViewContent()}
          </div>
        </main>
      </div>

      {/* Modals & Dialogs */}
      <QrCodeModal
        vpn={selectedVpnForQr}
        onClose={() => setSelectedVpnForQr(null)}
        onOpenGuide={() => setIsSetupGuideOpen(true)}
      />

      <AddServerModal
        isOpen={isAddServerOpen}
        onClose={() => setIsAddServerOpen(false)}
      />

      <ProvisionVpnModal
        isOpen={isProvisionOpen}
        onClose={() => setIsProvisionOpen(false)}
        preselectedClientId={preselectedClientId}
        onSuccessOpenQr={(vpn) => setSelectedVpnForQr(vpn)}
      />

      <PaymentProofModal
        order={selectedOrderForReview}
        onClose={() => setSelectedOrderForReview(null)}
        onSuccessOpenQr={(vpn) => setSelectedVpnForQr(vpn)}
      />

      <OneCommandInstallerModal
        isOpen={isInstallerOpen}
        onClose={() => setIsInstallerOpen(false)}
      />

      <BackupRestoreModal
        isOpen={isBackupOpen}
        onClose={() => setIsBackupOpen(false)}
      />

      <ClientSetupGuideModal
        isOpen={isSetupGuideOpen}
        onClose={() => setIsSetupGuideOpen(false)}
      />

      <OrderPackageModal
        pkg={selectedPackageForOrder}
        onClose={() => setSelectedPackageForOrder(null)}
        onSuccessOpenOrders={() => setActiveView('orders')}
      />

      <NotificationDrawer
        isOpen={isNotificationsOpen}
        onClose={() => setIsNotificationsOpen(false)}
        onNavigateToOrders={() => setActiveView('orders')}
        onNavigateToInventory={() => setActiveView('inventory')}
      />
    </div>
  );
}
