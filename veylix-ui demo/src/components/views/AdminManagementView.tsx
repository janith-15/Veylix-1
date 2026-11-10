import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  Users, 
  Plus, 
  Shield, 
  Server, 
  KeyRound, 
  Edit3, 
  CheckCircle2, 
  AlertCircle,
  MessageSquare,
  TrendingUp,
  Percent
} from 'lucide-react';
import { User } from '../../types';

interface AdminManagementViewProps {
  onOpenChat: (userId: string) => void;
}

export const AdminManagementView: React.FC<AdminManagementViewProps> = ({ onOpenChat }) => {
  const { 
    users, 
    servers, 
    vpnAccounts, 
    adjustAdminQuota, 
    addUser, 
    updateUser 
  } = useVeylixStore();

  const resellers = users.filter(u => u.role === 'admin');

  const [showAddModal, setShowAddModal] = useState(false);
  const [editingAdmin, setEditingAdmin] = useState<User | null>(null);
  const [newQuotaValue, setNewQuotaValue] = useState<number>(50);
  const [statusMessage, setStatusMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  // Form states for creating reseller
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [telegram, setTelegram] = useState('');
  const [initialQuota, setInitialQuota] = useState(50);
  const [selectedServers, setSelectedServers] = useState<string[]>([servers[0]?.id || '']);

  const handleOpenQuotaModal = (admin: User) => {
    setEditingAdmin(admin);
    setNewQuotaValue(admin.quotaLimit || 50);
  };

  const handleSaveQuota = () => {
    if (!editingAdmin) return;
    const success = adjustAdminQuota(editingAdmin.id, newQuotaValue);
    if (!success) {
      setStatusMessage({ 
        type: 'error', 
        text: `Quota adjustment rejected: Cannot set quota lower than currently active accounts (${editingAdmin.quotaUsed || 0}).` 
      });
    } else {
      setStatusMessage({ 
        type: 'success', 
        text: `Successfully updated ${editingAdmin.name}'s quota to ${newQuotaValue} keys.` 
      });
      setEditingAdmin(null);
    }
  };

  const handleCreateReseller = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name || !email) return;

    addUser({
      name,
      email,
      role: 'admin',
      telegram,
      status: 'ACTIVE',
      assignedServerIds: selectedServers,
      quotaLimit: initialQuota,
      quotaUsed: 0,
      trafficLimitGb: initialQuota * 50
    });

    setShowAddModal(false);
    setName('');
    setEmail('');
    setTelegram('');
    setStatusMessage({ type: 'success', text: `Created reseller account for ${name} with ${initialQuota} key quota.` });
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-extrabold text-white tracking-tight flex items-center space-x-2">
            <Users className="w-5 h-5 text-emerald-400" />
            <span>Reseller & Admin Quota Management</span>
          </h1>
          <p className="text-xs text-neutral-400">
            Provision independent reseller tenants, assign server node access, and control atomic VPN key quotas
          </p>
        </div>

        <button
          onClick={() => setShowAddModal(true)}
          className="px-4 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl text-xs transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
        >
          <Plus className="w-4 h-4" />
          <span>Add New Reseller</span>
        </button>
      </div>

      {statusMessage && (
        <div className={`p-3 rounded-xl border flex items-center justify-between text-xs ${
          statusMessage.type === 'error' ? 'bg-rose-500/10 border-rose-500/30 text-rose-300' : 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300'
        }`}>
          <div className="flex items-center space-x-2">
            {statusMessage.type === 'error' ? <AlertCircle className="w-4 h-4" /> : <CheckCircle2 className="w-4 h-4" />}
            <span>{statusMessage.text}</span>
          </div>
          <button onClick={() => setStatusMessage(null)} className="text-neutral-400 hover:text-white">✕</button>
        </div>
      )}

      {/* Resellers Card Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {resellers.map(admin => {
          const used = admin.quotaUsed || 0;
          const limit = admin.quotaLimit || 0;
          const remaining = Math.max(0, limit - used);
          const percent = Math.min(100, Math.round((used / (limit || 1)) * 100));

          const adminVpns = vpnAccounts.filter(v => v.adminId === admin.id);
          const activeAdminClients = users.filter(u => u.assignedAdminId === admin.id);

          return (
            <div key={admin.id} className="p-5 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl space-y-4">
              <div className="flex items-start justify-between">
                <div className="flex items-center space-x-3">
                  <div className="w-10 h-10 rounded-xl bg-violet-500/20 text-violet-400 flex items-center justify-center font-bold text-sm">
                    {admin.name.charAt(0)}
                  </div>
                  <div>
                    <h3 className="font-bold text-white text-sm">{admin.name}</h3>
                    <p className="text-xs text-neutral-400 font-mono">{admin.email}</p>
                    {admin.telegram && (
                      <p className="text-[11px] text-emerald-400 mt-0.5">{admin.telegram}</p>
                    )}
                  </div>
                </div>

                <div className="flex items-center space-x-1.5">
                  <button
                    onClick={() => handleOpenQuotaModal(admin)}
                    className="p-1.5 bg-neutral-800 hover:bg-neutral-700 text-neutral-300 hover:text-white rounded-lg text-xs transition flex items-center space-x-1"
                    title="Adjust Quota"
                  >
                    <Edit3 className="w-3.5 h-3.5 text-emerald-400" />
                    <span>Adjust Quota</span>
                  </button>
                </div>
              </div>

              {/* Atomic Quota Tracker */}
              <div className="p-3 bg-neutral-950 border border-neutral-800/80 rounded-xl space-y-2">
                <div className="flex items-center justify-between text-xs font-medium">
                  <span className="text-neutral-300 flex items-center space-x-1">
                    <KeyRound className="w-3.5 h-3.5 text-emerald-400" />
                    <span>VPN Key Allocation</span>
                  </span>
                  <span className="font-mono text-emerald-400 font-bold">
                    {used} / {limit} Keys ({percent}%)
                  </span>
                </div>

                <div className="w-full bg-neutral-800 h-2 rounded-full overflow-hidden">
                  <div 
                    className={`h-full rounded-full transition-all ${
                      percent > 85 ? 'bg-amber-500' : 'bg-emerald-500'
                    }`}
                    style={{ width: `${percent}%` }}
                  ></div>
                </div>

                <div className="flex justify-between text-[11px] text-neutral-400 font-mono pt-1">
                  <span>Available Remaining: <strong className="text-white">{remaining}</strong> keys</span>
                  <span>Active Clients: <strong className="text-white">{activeAdminClients.length}</strong></span>
                </div>
              </div>

              {/* Assigned Servers Badges */}
              <div className="space-y-1.5">
                <span className="text-neutral-400 text-xs font-medium">Assigned Server Nodes:</span>
                <div className="flex flex-wrap gap-1.5">
                  {admin.assignedServerIds && admin.assignedServerIds.length > 0 ? (
                    admin.assignedServerIds.map(srvId => {
                      const srv = servers.find(s => s.id === srvId);
                      return (
                        <span key={srvId} className="px-2 py-0.5 bg-neutral-950 border border-neutral-800 rounded-lg text-[11px] text-neutral-300 flex items-center space-x-1">
                          <span>{srv?.flag || '🌐'}</span>
                          <span>{srv?.name || srvId}</span>
                        </span>
                      );
                    })
                  ) : (
                    <span className="text-neutral-500 text-[11px]">All cluster nodes accessible</span>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {/* Adjust Quota Modal */}
      {editingAdmin && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in">
          <div className="w-full max-w-md bg-neutral-900 border border-neutral-800 rounded-2xl p-6 shadow-2xl space-y-4">
            <h3 className="text-sm font-bold text-white">Adjust Quota for {editingAdmin.name}</h3>
            <p className="text-xs text-neutral-400">
              Current used: <span className="text-white font-mono">{editingAdmin.quotaUsed || 0}</span> accounts. New limit cannot be lower than currently active accounts.
            </p>

            <div className="space-y-1">
              <label className="text-xs text-neutral-300 font-medium">Max Active Keys Allocation</label>
              <input
                type="number"
                min={editingAdmin.quotaUsed || 0}
                max={500}
                value={newQuotaValue}
                onChange={(e) => setNewQuotaValue(Number(e.target.value))}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white font-mono text-xs focus:outline-none focus:border-emerald-500/50"
              />
            </div>

            <div className="flex justify-end space-x-2 pt-2">
              <button
                type="button"
                onClick={() => setEditingAdmin(null)}
                className="px-4 py-2 bg-neutral-800 hover:bg-neutral-700 text-white rounded-xl text-xs"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleSaveQuota}
                className="px-4 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl text-xs"
              >
                Save Quota
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Add New Reseller Modal */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in">
          <div className="w-full max-w-md bg-neutral-900 border border-neutral-800 rounded-2xl p-6 shadow-2xl space-y-4 text-xs">
            <h3 className="text-sm font-bold text-white">Register Reseller / Admin Account</h3>

            <form onSubmit={handleCreateReseller} className="space-y-3">
              <div className="space-y-1">
                <label className="text-neutral-300 font-medium">Reseller Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. TurboVPN Sri Lanka"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                />
              </div>

              <div className="space-y-1">
                <label className="text-neutral-300 font-medium">Email Address</label>
                <input
                  type="email"
                  required
                  placeholder="reseller@turbovpn.lk"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                />
              </div>

              <div className="space-y-1">
                <label className="text-neutral-300 font-medium">Telegram Handle</label>
                <input
                  type="text"
                  placeholder="@turbovpn_admin"
                  value={telegram}
                  onChange={(e) => setTelegram(e.target.value)}
                  className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                />
              </div>

              <div className="space-y-1">
                <label className="text-neutral-300 font-medium">Initial VPN Key Quota Limit</label>
                <input
                  type="number"
                  min={1}
                  max={500}
                  value={initialQuota}
                  onChange={(e) => setInitialQuota(Number(e.target.value))}
                  className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white font-mono focus:outline-none focus:border-emerald-500/50"
                />
              </div>

              <div className="flex justify-end space-x-2 pt-3">
                <button
                  type="button"
                  onClick={() => setShowAddModal(false)}
                  className="px-4 py-2 bg-neutral-800 hover:bg-neutral-700 text-white rounded-xl"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="px-4 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl"
                >
                  Create Reseller
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
