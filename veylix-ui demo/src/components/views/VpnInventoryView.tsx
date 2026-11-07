import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  KeyRound, 
  Plus, 
  Search, 
  QrCode, 
  Copy, 
  Check, 
  Calendar, 
  PauseCircle, 
  PlayCircle, 
  Trash2, 
  RefreshCw,
  HardDrive,
  Filter
} from 'lucide-react';
import { VpnAccount, VpnProtocol, VpnStatus, ThreeXUIServer } from '../../types';

interface VpnInventoryViewProps {
  onOpenProvision: () => void;
  onSelectVpnForQr: (vpn: VpnAccount) => void;
}

export const VpnInventoryView: React.FC<VpnInventoryViewProps> = ({ 
  onOpenProvision, 
  onSelectVpnForQr 
}) => {
  const { 
    vpnAccounts, 
    servers, 
    updateVpnStatus, 
    deleteVpnAccount, 
    extendVpnExpiry, 
    currentRole,
    currentUser 
  } = useVeylixStore();

  const [search, setSearch] = useState('');
  const [selectedProtocol, setSelectedProtocol] = useState<string>('ALL');
  const [selectedServer, setSelectedServer] = useState<string>('ALL');
  const [selectedStatus, setSelectedStatus] = useState<string>('ALL');
  const [copiedId, setCopiedId] = useState<string>('');

  // If reseller, show their clients' VPNs only
  const visibleAccounts = currentRole === 'admin'
    ? vpnAccounts.filter(v => v.adminId === currentUser.id)
    : vpnAccounts;

  const filteredAccounts = visibleAccounts.filter(vpn => {
    const matchesSearch = vpn.name.toLowerCase().includes(search.toLowerCase()) ||
      (vpn.clientName && vpn.clientName.toLowerCase().includes(search.toLowerCase())) ||
      vpn.serverHost.toLowerCase().includes(search.toLowerCase()) ||
      vpn.uuid.toLowerCase().includes(search.toLowerCase());

    const matchesProtocol = selectedProtocol === 'ALL' || vpn.protocol === selectedProtocol;
    const matchesServer = selectedServer === 'ALL' || vpn.serverId === selectedServer;
    const matchesStatus = selectedStatus === 'ALL' || vpn.status === selectedStatus;

    return matchesSearch && matchesProtocol && matchesServer && matchesStatus;
  });

  const handleCopyUri = (vpn: VpnAccount) => {
    navigator.clipboard.writeText(vpn.connectionUri);
    setCopiedId(vpn.id);
    setTimeout(() => setCopiedId(''), 2000);
  };

  const handleExtend = (vpn: VpnAccount) => {
    extendVpnExpiry(vpn.id, 30, 50);
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-extrabold text-white tracking-tight flex items-center space-x-2">
            <KeyRound className="w-5 h-5 text-emerald-400" />
            <span>VPN Inventory & Inbound Keys</span>
          </h1>
          <p className="text-xs text-neutral-400">
            {currentRole === 'admin'
              ? `Manage issued keys for your reseller account (${visibleAccounts.length} Total)`
              : `Comprehensive multi-protocol inventory across all 3X-UI nodes (${vpnAccounts.length} Total)`}
          </p>
        </div>

        <button
          onClick={onOpenProvision}
          className="px-4 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl text-xs transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
        >
          <Plus className="w-4 h-4" />
          <span>Provision New Key</span>
        </button>
      </div>

      {/* Filter and Search Bar */}
      <div className="p-4 bg-neutral-900 border border-neutral-800 rounded-2xl space-y-3">
        <div className="grid grid-cols-1 sm:grid-cols-4 gap-3">
          {/* Search text */}
          <div className="relative sm:col-span-1">
            <Search className="w-4 h-4 text-neutral-400 absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search keys, clients, hosts..."
              className="w-full pl-9 pr-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-xs text-white placeholder-neutral-500 focus:outline-none focus:border-emerald-500/50"
            />
          </div>

          {/* Protocol Filter */}
          <div>
            <select
              value={selectedProtocol}
              onChange={(e) => setSelectedProtocol(e.target.value)}
              className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-xs text-neutral-200 focus:outline-none focus:border-emerald-500/50"
            >
              <option value="ALL">All Protocols (VLESS, VMess, Trojan, SS)</option>
              <option value="VLESS">VLESS (Reality)</option>
              <option value="VMess">VMess (WebSocket)</option>
              <option value="Trojan">Trojan (gRPC)</option>
              <option value="Shadowsocks">Shadowsocks (2022)</option>
            </select>
          </div>

          {/* Server Filter */}
          <div>
            <select
              value={selectedServer}
              onChange={(e) => setSelectedServer(e.target.value)}
              className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-xs text-neutral-200 focus:outline-none focus:border-emerald-500/50"
            >
              <option value="ALL">All Cluster Nodes</option>
              {servers.map((s: ThreeXUIServer) => (
                <option key={s.id} value={s.id}>
                  {s.flag} {s.name}
                </option>
              ))}
            </select>
          </div>

          {/* Status Filter */}
          <div>
            <select
              value={selectedStatus}
              onChange={(e) => setSelectedStatus(e.target.value)}
              className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-xs text-neutral-200 focus:outline-none focus:border-emerald-500/50"
            >
              <option value="ALL">All Statuses</option>
              <option value="ACTIVE">Active</option>
              <option value="EXPIRED">Expired</option>
              <option value="SUSPENDED">Suspended</option>
              <option value="RESERVED">Reserved</option>
            </select>
          </div>
        </div>
      </div>

      {/* Inventory Table */}
      <div className="bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-neutral-800 text-neutral-400 bg-neutral-950/70">
                <th className="py-3 px-4 font-medium">Config / Tag</th>
                <th className="py-3 px-4 font-medium">Protocol & Security</th>
                <th className="py-3 px-4 font-medium">Node Host & Port</th>
                <th className="py-3 px-4 font-medium">Assigned Client</th>
                <th className="py-3 px-4 font-medium">Traffic Consumption</th>
                <th className="py-3 px-4 font-medium">Expiry</th>
                <th className="py-3 px-4 font-medium">Status</th>
                <th className="py-3 px-4 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-800/60">
              {filteredAccounts.length === 0 ? (
                <tr>
                  <td colSpan={8} className="py-12 text-center text-neutral-500">
                    No VPN configurations match the current filter criteria.
                  </td>
                </tr>
              ) : (
                filteredAccounts.map((vpn: VpnAccount) => {
                  const isSuspended = vpn.status === 'SUSPENDED';
                  const percentUsed = Math.min(100, (vpn.usedTrafficGb / (vpn.totalTrafficGb || 1)) * 100);

                  return (
                    <tr key={vpn.id} className="hover:bg-neutral-800/30 transition">
                      {/* Name / Tag */}
                      <td className="py-3 px-4">
                        <div className="font-semibold text-white flex items-center space-x-1.5">
                          <span>{vpn.name}</span>
                          {vpn.isFree && (
                            <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-emerald-500/20 text-emerald-400">
                              FREE
                            </span>
                          )}
                        </div>
                        <p className="font-mono text-[11px] text-neutral-500 mt-0.5 truncate max-w-[170px]">
                          {vpn.uuid}
                        </p>
                      </td>

                      {/* Protocol */}
                      <td className="py-3 px-4">
                        <span className="px-2 py-0.5 rounded font-mono font-semibold bg-neutral-800 text-neutral-200">
                          {vpn.protocol}
                        </span>
                        <p className="text-[11px] text-neutral-400 capitalize mt-0.5">
                          {vpn.security !== 'none' ? vpn.security : 'Standard'}
                        </p>
                      </td>

                      {/* Server Host */}
                      <td className="py-3 px-4">
                        <p className="font-medium text-neutral-200">{vpn.serverHost}:{vpn.port}</p>
                        <p className="text-[11px] text-neutral-500">{vpn.serverName}</p>
                      </td>

                      {/* Client */}
                      <td className="py-3 px-4">
                        <p className="font-medium text-neutral-200">{vpn.clientName || 'Unassigned'}</p>
                        {vpn.adminName && (
                          <p className="text-[11px] text-neutral-500">Seller: {vpn.adminName}</p>
                        )}
                      </td>

                      {/* Traffic */}
                      <td className="py-3 px-4">
                        <div className="space-y-1 w-32">
                          <div className="flex justify-between text-[11px] font-mono text-neutral-300">
                            <span>{vpn.usedTrafficGb.toFixed(1)} GB</span>
                            <span className="text-neutral-500">{vpn.totalTrafficGb} GB</span>
                          </div>
                          <div className="w-full bg-neutral-800 h-1.5 rounded-full overflow-hidden">
                            <div 
                              className={`h-full rounded-full ${percentUsed > 85 ? 'bg-rose-500' : 'bg-emerald-500'}`}
                              style={{ width: `${percentUsed}%` }}
                            ></div>
                          </div>
                        </div>
                      </td>

                      {/* Expiry */}
                      <td className="py-3 px-4 font-mono text-[11px] text-neutral-300">
                        {new Date(vpn.expiryDate).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
                      </td>

                      {/* Status */}
                      <td className="py-3 px-4">
                        <span className={`px-2 py-0.5 rounded-full text-[10px] font-bold ${
                          vpn.status === 'ACTIVE' ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30' :
                          vpn.status === 'SUSPENDED' ? 'bg-amber-500/10 text-amber-400 border border-amber-500/30' :
                          'bg-neutral-800 text-neutral-400'
                        }`}>
                          {vpn.status}
                        </span>
                      </td>

                      {/* Action buttons */}
                      <td className="py-3 px-4 text-right">
                        <div className="flex items-center justify-end space-x-1.5">
                          {/* QR Code button */}
                          <button
                            onClick={() => onSelectVpnForQr(vpn)}
                            className="p-1.5 bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-400 rounded-lg border border-emerald-500/20 transition"
                            title="Show In-Browser QR Code"
                          >
                            <QrCode className="w-3.5 h-3.5" />
                          </button>

                          {/* Copy Link */}
                          <button
                            onClick={() => handleCopyUri(vpn)}
                            className="p-1.5 bg-neutral-800 hover:bg-neutral-700 text-neutral-300 hover:text-white rounded-lg transition"
                            title="Copy URI String"
                          >
                            {copiedId === vpn.id ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                          </button>

                          {/* Extend / Renew */}
                          <button
                            onClick={() => handleExtend(vpn)}
                            className="p-1.5 bg-neutral-800 hover:bg-neutral-700 text-neutral-300 hover:text-white rounded-lg transition"
                            title="Add +30 Days & +50GB"
                          >
                            <RefreshCw className="w-3.5 h-3.5 text-cyan-400" />
                          </button>

                          {/* Suspend / Resume */}
                          <button
                            onClick={() => updateVpnStatus(vpn.id, isSuspended ? 'ACTIVE' : 'SUSPENDED')}
                            className="p-1.5 bg-neutral-800 hover:bg-neutral-700 text-neutral-300 hover:text-white rounded-lg transition"
                            title={isSuspended ? 'Resume Key' : 'Suspend Key'}
                          >
                            {isSuspended ? (
                              <PlayCircle className="w-3.5 h-3.5 text-emerald-400" />
                            ) : (
                              <PauseCircle className="w-3.5 h-3.5 text-amber-400" />
                            )}
                          </button>

                          {/* Delete */}
                          <button
                            onClick={() => {
                              if (confirm(`Revoke and delete VPN key ${vpn.name}?`)) {
                                deleteVpnAccount(vpn.id);
                              }
                            }}
                            className="p-1.5 text-neutral-500 hover:text-rose-400 hover:bg-rose-500/10 rounded-lg transition"
                            title="Revoke / Delete"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
