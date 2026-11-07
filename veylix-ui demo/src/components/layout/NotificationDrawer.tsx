import React from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { X, Bell, ShieldCheck, CreditCard, KeyRound, AlertTriangle, Check } from 'lucide-react';

interface NotificationDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  onNavigateToOrders: () => void;
  onNavigateToInventory: () => void;
}

export const NotificationDrawer: React.FC<NotificationDrawerProps> = ({
  isOpen,
  onClose,
  onNavigateToOrders,
  onNavigateToInventory
}) => {
  const { auditLogs, orders } = useVeylixStore();

  if (!isOpen) return null;

  const pendingOrders = orders.filter(o => o.status === 'PENDING');
  const recentEvents = auditLogs.slice(0, 10);

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/60 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="w-full max-w-sm bg-neutral-900 border-l border-neutral-800 h-full flex flex-col justify-between shadow-2xl animate-in slide-in-from-right duration-200">
        {/* Header */}
        <div className="p-4 border-b border-neutral-800 bg-neutral-950/50 flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <Bell className="w-4 h-4 text-emerald-400" />
            <h3 className="font-bold text-white text-sm">Notifications & Alerts</h3>
          </div>
          <button 
            onClick={onClose}
            className="p-1 text-neutral-400 hover:text-white rounded-lg hover:bg-neutral-800 transition"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Notifications Body */}
        <div className="flex-1 p-4 overflow-y-auto space-y-4 text-xs">
          {/* Pending Action Items */}
          {pendingOrders.length > 0 && (
            <div className="p-3 bg-amber-500/10 border border-amber-500/30 rounded-xl space-y-1.5">
              <div className="flex items-center justify-between font-semibold text-amber-300">
                <span className="flex items-center space-x-1.5">
                  <CreditCard className="w-3.5 h-3.5" />
                  <span>Action Required</span>
                </span>
                <span className="text-[10px] bg-amber-500/20 px-1.5 py-0.2 rounded font-mono">
                  {pendingOrders.length} Pending
                </span>
              </div>
              <p className="text-neutral-300 text-[11px]">
                {pendingOrders.length} customer order(s) waiting for proof verification and auto-provisioning.
              </p>
              <button
                onClick={() => {
                  onClose();
                  onNavigateToOrders();
                }}
                className="text-[11px] font-semibold text-amber-400 hover:underline pt-1 block"
              >
                Review Pending Orders →
              </button>
            </div>
          )}

          {/* Activity Log Feed */}
          <div className="space-y-2">
            <span className="text-[10px] font-semibold uppercase tracking-wider text-neutral-500 font-mono">
              Real-Time Platform Events
            </span>

            <div className="space-y-2">
              {recentEvents.map(event => (
                <div key={event.id} className="p-2.5 bg-neutral-950/60 border border-neutral-800/80 rounded-xl space-y-1">
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-[10px] text-emerald-400 font-semibold">{event.action}</span>
                    <span className="text-[10px] text-neutral-500">
                      {new Date(event.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                    </span>
                  </div>
                  <p className="text-neutral-300 text-[11px]">{event.details}</p>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="p-4 border-t border-neutral-800 bg-neutral-950/60 flex items-center justify-between text-xs text-neutral-400">
          <span>Cluster status: Normal</span>
          <button
            onClick={onClose}
            className="px-3 py-1 bg-neutral-800 hover:bg-neutral-700 text-white rounded-lg text-xs"
          >
            Dismiss
          </button>
        </div>
      </div>
    </div>
  );
};
