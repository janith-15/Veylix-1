import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  UserCheck, 
  Plus, 
  Search, 
  KeyRound, 
  MessageSquare, 
  Calendar, 
  ShieldCheck, 
  UserPlus, 
  ExternalLink 
} from 'lucide-react';
import { User, VpnAccount } from '../../types';

interface ClientManagementViewProps {
  onOpenProvisionForClient: (clientId: string) => void;
  onOpenChat: (userId: string) => void;
  onSelectVpnForQr: (vpn: VpnAccount) => void;
}

export const ClientManagementView: React.FC<ClientManagementViewProps> = ({
  onOpenProvisionForClient,
  onOpenChat,
  onSelectVpnForQr
}) => {
  const { users, vpnAccounts, addUser, currentRole, currentUser } = useVeylixStore();
  const [search, setSearch] = useState('');
  const [showAddModal, setShowAddModal] = useState(false);

  // Form states for creating new client
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [telegram, setTelegram] = useState('');
  const [phone, setPhone] = useState('');

  // Filter clients
  const allClients = users.filter(u => u.role === 'client');
  const visibleClients = currentRole === 'admin'
    ? allClients.filter(c => c.assignedAdminId === currentUser.id)
    : allClients;

  const filteredClients = visibleClients.filter(c => 
    c.name.toLowerCase().includes(search.toLowerCase()) ||
    c.email.toLowerCase().includes(search.toLowerCase()) ||
    (c.telegram && c.telegram.toLowerCase().includes(search.toLowerCase()))
  );

  const handleCreateClient = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name || !email) return;

    addUser({
      name,
      email,
      role: 'client',
      telegram,
      phone,
      status: 'ACTIVE',
      assignedAdminId: currentRole === 'admin' ? currentUser.id : undefined
    });

    setShowAddModal(false);
    setName('');
    setEmail('');
    setTelegram('');
    setPhone('');
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-extrabold text-white tracking-tight flex items-center space-x-2">
            <UserCheck className="w-5 h-5 text-emerald-400" />
            <span>Client Accounts Directory</span>
          </h1>
          <p className="text-xs text-neutral-400">
            {currentRole === 'admin'
              ? `Your assigned client accounts (${visibleClients.length} Total)`
              : `Total registered clients across all business channels (${allClients.length} Total)`}
          </p>
        </div>

        <button
          onClick={() => setShowAddModal(true)}
          className="px-4 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl text-xs transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
        >
          <UserPlus className="w-4 h-4" />
          <span>Register New Client</span>
        </button>
      </div>

      {/* Search Input */}
      <div className="p-4 bg-neutral-900 border border-neutral-800 rounded-2xl">
        <div className="relative max-w-md">
          <Search className="w-4 h-4 text-neutral-400 absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search client by name, email, telegram..."
            className="w-full pl-9 pr-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-xs text-white placeholder-neutral-500 focus:outline-none focus:border-emerald-500/50"
          />
        </div>
      </div>

      {/* Client Cards Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {filteredClients.map(client => {
          const clientVpns = vpnAccounts.filter(v => v.clientId === client.id && v.status === 'ACTIVE');
          const seller = users.find(u => u.id === client.assignedAdminId);

          return (
            <div 
              key={client.id}
              className="p-5 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl space-y-4 hover:border-neutral-700 transition"
            >
              <div className="flex items-start justify-between">
                <div className="flex items-center space-x-3">
                  <div className="w-10 h-10 rounded-xl bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 flex items-center justify-center font-bold text-sm">
                    {client.name.charAt(0)}
                  </div>
                  <div>
                    <h3 className="font-bold text-white text-xs">{client.name}</h3>
                    <p className="text-[11px] text-neutral-400 font-mono">{client.email}</p>
                    {client.telegram && (
                      <p className="text-[11px] text-emerald-400 mt-0.5">{client.telegram}</p>
                    )}
                  </div>
                </div>

                <span className="px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-500/10 text-emerald-400">
                  {client.status}
                </span>
              </div>

              {/* Active Keys Section */}
              <div className="p-3 bg-neutral-950 border border-neutral-800/80 rounded-xl space-y-2 text-xs">
                <div className="flex items-center justify-between text-neutral-300">
                  <span className="font-medium flex items-center space-x-1">
                    <KeyRound className="w-3.5 h-3.5 text-emerald-400" />
                    <span>Active VPN Keys:</span>
                  </span>
                  <span className="font-mono text-emerald-400 font-bold">{clientVpns.length}</span>
                </div>

                {clientVpns.length > 0 ? (
                  <div className="space-y-1.5 pt-1 border-t border-neutral-900">
                    {clientVpns.slice(0, 2).map(vpn => (
                      <div key={vpn.id} className="flex items-center justify-between text-[11px]">
                        <span className="text-neutral-300 truncate max-w-[140px]">{vpn.name}</span>
                        <button
                          onClick={() => onSelectVpnForQr(vpn)}
                          className="px-2 py-0.5 bg-neutral-800 hover:bg-neutral-700 text-emerald-400 rounded text-[10px] font-semibold"
                        >
                          Show QR
                        </button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-[11px] text-neutral-500 italic">No active VPN keys currently assigned.</p>
                )}
              </div>

              {/* Seller attribution and actions */}
              <div className="pt-2 border-t border-neutral-800/80 flex items-center justify-between text-xs">
                <span className="text-[11px] text-neutral-500">
                  {seller ? `Seller: ${seller.name}` : 'Direct Super Admin Client'}
                </span>

                <div className="flex items-center space-x-1.5">
                  <button
                    onClick={() => onOpenChat(client.id)}
                    className="p-1.5 bg-neutral-800 hover:bg-neutral-700 text-neutral-300 hover:text-white rounded-lg transition"
                    title="Open Live Chat"
                  >
                    <MessageSquare className="w-3.5 h-3.5" />
                  </button>
                  <button
                    onClick={() => onOpenProvisionForClient(client.id)}
                    className="px-2.5 py-1.5 bg-emerald-500/20 hover:bg-emerald-500/30 text-emerald-300 font-semibold rounded-lg text-xs transition"
                  >
                    + Issue Key
                  </button>
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {/* Add Client Modal */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in">
          <div className="w-full max-w-md bg-neutral-900 border border-neutral-800 rounded-2xl p-6 shadow-2xl space-y-4 text-xs">
            <h3 className="text-sm font-bold text-white">Create New Client Account</h3>

            <form onSubmit={handleCreateClient} className="space-y-3">
              <div className="space-y-1">
                <label className="text-neutral-300 font-medium">Customer Full Name</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. Kasun Perera"
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
                  placeholder="kasun@gmail.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                />
              </div>

              <div className="space-y-1">
                <label className="text-neutral-300 font-medium">Telegram Handle (Optional)</label>
                <input
                  type="text"
                  placeholder="@kasun_vpn"
                  value={telegram}
                  onChange={(e) => setTelegram(e.target.value)}
                  className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                />
              </div>

              <div className="space-y-1">
                <label className="text-neutral-300 font-medium">Phone / WhatsApp</label>
                <input
                  type="text"
                  placeholder="+94 77 123 4567"
                  value={phone}
                  onChange={(e) => setPhone(e.target.value)}
                  className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
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
                  Create Account
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
