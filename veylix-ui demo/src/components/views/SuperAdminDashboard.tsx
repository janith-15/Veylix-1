import React from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  Server, 
  KeyRound, 
  Users, 
  CreditCard, 
  HardDrive, 
  Activity, 
  RefreshCw, 
  Plus, 
  ShieldCheck, 
  TrendingUp, 
  Globe, 
  Zap, 
  Terminal, 
  ArrowUpRight,
  Clock,
  AlertTriangle
} from 'lucide-react';
import { VpnAccount } from '../../types';

interface SuperAdminDashboardProps {
  onOpenAddServer: () => void;
  onOpenProvision: () => void;
  onOpenInstaller: () => void;
  onSelectVpnForQr: (vpn: VpnAccount) => void;
  onNavigate: (view: string) => void;
}

export const SuperAdminDashboard: React.FC<SuperAdminDashboardProps> = ({
  onOpenAddServer,
  onOpenProvision,
  onOpenInstaller,
  onSelectVpnForQr,
  onNavigate
}) => {
  const { 
    servers, 
    vpnAccounts, 
    users, 
    orders, 
    auditLogs, 
    syncServer, 
    systemSettings 
  } = useVeylixStore();

  const activeVpns = vpnAccounts.filter(v => v.status === 'ACTIVE');
  const freeVpns = vpnAccounts.filter(v => v.isFree);
  const resellers = users.filter(u => u.role === 'admin');
  const clients = users.filter(u => u.role === 'client');
  const pendingOrders = orders.filter(o => o.status === 'PENDING');

  const totalRevenue = orders
    .filter(o => o.status === 'VERIFIED')
    .reduce((acc, o) => acc + o.amount, 0);

  const totalUsedTraffic = vpnAccounts.reduce((acc, v) => acc + v.usedTrafficGb, 0);
  const totalAllocatedTraffic = vpnAccounts.reduce((acc, v) => acc + v.totalTrafficGb, 0);

  return (
    <div className="space-y-6">
      {/* Welcome & Fast Action Bar */}
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4 p-6 bg-gradient-to-r from-neutral-900 via-neutral-900/90 to-neutral-950 border border-neutral-800 rounded-2xl shadow-xl relative overflow-hidden">
        <div className="absolute top-0 right-0 w-80 h-80 bg-emerald-500/5 rounded-full blur-3xl pointer-events-none -mr-20 -mt-20"></div>
        <div className="space-y-1 relative z-10">
          <div className="flex items-center space-x-2">
            <h1 className="text-xl md:text-2xl font-extrabold text-white tracking-tight">
              Master Infrastructure & Business Control
            </h1>
            <span className="px-2 py-0.5 text-[11px] font-mono font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 rounded-full">
              LIVE NODE
            </span>
          </div>
          <p className="text-xs text-neutral-400">
            Installation ID: <span className="font-mono text-neutral-300">{systemSettings.installationId}</span> • 3X-UI Cluster Active
          </p>
        </div>

        {/* Action Triggers */}
        <div className="flex flex-wrap items-center gap-2.5 relative z-10">
          <button
            onClick={onOpenProvision}
            className="px-4 py-2 text-xs font-semibold text-neutral-950 bg-emerald-400 hover:bg-emerald-300 rounded-xl transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
          >
            <KeyRound className="w-4 h-4" />
            <span>Issue VPN Key</span>
          </button>

          <button
            onClick={onOpenAddServer}
            className="px-4 py-2 text-xs font-semibold text-white bg-neutral-800 hover:bg-neutral-700 border border-neutral-700/60 rounded-xl transition flex items-center space-x-1.5"
          >
            <Server className="w-4 h-4 text-cyan-400" />
            <span>Add 3X-UI Server</span>
          </button>

          <button
            onClick={onOpenInstaller}
            className="px-3.5 py-2 text-xs font-medium text-neutral-300 hover:text-white bg-neutral-900 hover:bg-neutral-800 border border-neutral-800 rounded-xl transition flex items-center space-x-1.5"
          >
            <Terminal className="w-3.5 h-3.5 text-emerald-400" />
            <span>Linux Installer</span>
          </button>
        </div>
      </div>

      {/* Primary KPI Grid */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Total Connected Servers */}
        <div className="p-4 bg-neutral-900/80 border border-neutral-800 rounded-2xl space-y-3 relative group hover:border-neutral-700 transition">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium text-neutral-400">3X-UI Panels</span>
            <div className="w-8 h-8 rounded-xl bg-cyan-500/10 text-cyan-400 flex items-center justify-center">
              <Server className="w-4 h-4" />
            </div>
          </div>
          <div>
            <div className="flex items-baseline space-x-2">
              <span className="text-2xl font-extrabold text-white font-mono">{servers.length}</span>
              <span className="text-xs text-emerald-400 font-medium">100% Online</span>
            </div>
            <p className="text-[11px] text-neutral-500 mt-0.5">Singapore, FRA, Tokyo, US</p>
          </div>
        </div>

        {/* Active VPN Accounts */}
        <div className="p-4 bg-neutral-900/80 border border-neutral-800 rounded-2xl space-y-3 relative group hover:border-neutral-700 transition">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium text-neutral-400">Active VPN Keys</span>
            <div className="w-8 h-8 rounded-xl bg-emerald-500/10 text-emerald-400 flex items-center justify-center">
              <KeyRound className="w-4 h-4" />
            </div>
          </div>
          <div>
            <div className="flex items-baseline space-x-2">
              <span className="text-2xl font-extrabold text-white font-mono">{activeVpns.length}</span>
              <span className="text-xs text-neutral-400 font-medium font-mono">({freeVpns.length} Free Tier)</span>
            </div>
            <p className="text-[11px] text-neutral-500 mt-0.5">VLESS Reality, VMess, Trojan</p>
          </div>
        </div>

        {/* Resellers & Clients */}
        <div className="p-4 bg-neutral-900/80 border border-neutral-800 rounded-2xl space-y-3 relative group hover:border-neutral-700 transition">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium text-neutral-400">Resellers & Clients</span>
            <div className="w-8 h-8 rounded-xl bg-violet-500/10 text-violet-400 flex items-center justify-center">
              <Users className="w-4 h-4" />
            </div>
          </div>
          <div>
            <div className="flex items-baseline space-x-2">
              <span className="text-2xl font-extrabold text-white font-mono">{clients.length}</span>
              <span className="text-xs text-violet-400 font-medium">({resellers.length} Resellers)</span>
            </div>
            <p className="text-[11px] text-neutral-500 mt-0.5">Multi-tenant quota enforcement</p>
          </div>
        </div>

        {/* Total Settled Revenue */}
        <div className="p-4 bg-neutral-900/80 border border-neutral-800 rounded-2xl space-y-3 relative group hover:border-neutral-700 transition">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium text-neutral-400">Settled Revenue</span>
            <div className="w-8 h-8 rounded-xl bg-amber-500/10 text-amber-400 flex items-center justify-center">
              <CreditCard className="w-4 h-4" />
            </div>
          </div>
          <div>
            <div className="flex items-baseline space-x-2">
              <span className="text-2xl font-extrabold text-white font-mono">${totalRevenue.toFixed(2)}</span>
              {pendingOrders.length > 0 && (
                <span className="text-xs text-amber-400 font-medium bg-amber-500/10 px-1.5 py-0.5 rounded">
                  {pendingOrders.length} Pending
                </span>
              )}
            </div>
            <p className="text-[11px] text-neutral-500 mt-0.5">Automated verified workflow</p>
          </div>
        </div>
      </div>

      {/* Cluster Nodes Live Status Cards */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <Server className="w-4 h-4 text-emerald-400" />
            <h2 className="text-sm font-bold text-white tracking-wide">3X-UI Cluster Nodes Status</h2>
          </div>
          <button
            onClick={() => servers.forEach(s => syncServer(s.id))}
            className="text-xs text-neutral-400 hover:text-emerald-400 flex items-center space-x-1 transition font-medium"
          >
            <RefreshCw className="w-3.5 h-3.5" />
            <span>Sync All Nodes</span>
          </button>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {servers.map(server => (
            <div
              key={server.id}
              className="p-4 bg-neutral-900/90 border border-neutral-800 rounded-2xl space-y-3 hover:border-neutral-700/80 transition"
            >
              <div className="flex items-start justify-between">
                <div>
                  <div className="flex items-center space-x-1.5">
                    <span className="text-lg">{server.flag}</span>
                    <h3 className="font-bold text-white text-xs truncate max-w-[130px]">{server.name}</h3>
                  </div>
                  <p className="text-[11px] font-mono text-neutral-400 mt-0.5">{server.host}:{server.port}</p>
                </div>
                <div className="flex flex-col items-end">
                  <span className="px-2 py-0.5 text-[10px] font-mono font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 rounded-full">
                    {server.latencyMs}ms
                  </span>
                  <span className="text-[10px] text-neutral-500 mt-1">{server.panelVersion}</span>
                </div>
              </div>

              {/* Resource Bars */}
              <div className="space-y-1.5 text-[11px]">
                <div className="flex justify-between text-neutral-400">
                  <span>CPU Usage:</span>
                  <span className="font-mono text-neutral-200">{server.cpuUsagePercent}%</span>
                </div>
                <div className="w-full bg-neutral-800 h-1.5 rounded-full overflow-hidden">
                  <div 
                    className={`h-full rounded-full transition-all ${
                      server.cpuUsagePercent > 75 ? 'bg-rose-500' : 'bg-emerald-500'
                    }`}
                    style={{ width: `${server.cpuUsagePercent}%` }}
                  ></div>
                </div>

                <div className="flex justify-between text-neutral-400 pt-1">
                  <span>RAM ({server.ramUsedMb}MB / {server.ramTotalMb}MB):</span>
                  <span className="font-mono text-neutral-200">{server.ramUsagePercent}%</span>
                </div>
                <div className="w-full bg-neutral-800 h-1.5 rounded-full overflow-hidden">
                  <div 
                    className="bg-cyan-500 h-full rounded-full transition-all"
                    style={{ width: `${server.ramUsagePercent}%` }}
                  ></div>
                </div>
              </div>

              {/* Inbounds & Action */}
              <div className="pt-2 border-t border-neutral-800/80 flex items-center justify-between">
                <span className="text-[11px] text-neutral-400">
                  {server.inbounds.length} Inbounds ({server.inbounds.map(i => i.protocol).join(', ')})
                </span>
                <button
                  onClick={() => syncServer(server.id)}
                  disabled={server.syncStatus === 'SYNCING'}
                  className="p-1 text-neutral-400 hover:text-emerald-400 transition"
                  title="Sync telemetry"
                >
                  <RefreshCw className={`w-3.5 h-3.5 ${server.syncStatus === 'SYNCING' ? 'animate-spin text-emerald-400' : ''}`} />
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Lower Section: Active Keys & Audit Feed */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Active VPN Accounts Snippet */}
        <div className="lg:col-span-2 p-5 bg-neutral-900/90 border border-neutral-800 rounded-2xl space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2">
              <KeyRound className="w-4 h-4 text-emerald-400" />
              <h3 className="text-sm font-bold text-white">Recent VPN Provisioning & Inbounds</h3>
            </div>
            <button
              onClick={() => onNavigate('inventory')}
              className="text-xs text-emerald-400 hover:underline flex items-center space-x-1"
            >
              <span>View All ({vpnAccounts.length})</span>
              <ArrowUpRight className="w-3.5 h-3.5" />
            </button>
          </div>

          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-neutral-800 text-neutral-400">
                  <th className="pb-2 font-medium">Config Name</th>
                  <th className="pb-2 font-medium">Protocol</th>
                  <th className="pb-2 font-medium">Node</th>
                  <th className="pb-2 font-medium">Traffic Used</th>
                  <th className="pb-2 font-medium">Status</th>
                  <th className="pb-2 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-800/60">
                {vpnAccounts.slice(0, 5).map(vpn => (
                  <tr key={vpn.id} className="hover:bg-neutral-800/30 transition">
                    <td className="py-2.5 font-medium text-neutral-200">
                      <div>
                        <span>{vpn.name}</span>
                        {vpn.isFree && (
                          <span className="ml-1.5 px-1.5 py-0.2 bg-emerald-500/20 text-emerald-400 text-[10px] rounded font-mono font-bold">
                            FREE
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="py-2.5">
                      <span className="px-2 py-0.5 rounded text-[11px] font-mono font-semibold bg-neutral-800 text-neutral-300">
                        {vpn.protocol}
                      </span>
                    </td>
                    <td className="py-2.5 text-neutral-300">{vpn.serverHost}:{vpn.port}</td>
                    <td className="py-2.5">
                      <div className="space-y-1">
                        <span className="text-[11px] font-mono text-neutral-300">
                          {vpn.usedTrafficGb.toFixed(1)} / {vpn.totalTrafficGb} GB
                        </span>
                        <div className="w-20 bg-neutral-800 h-1 rounded-full overflow-hidden">
                          <div 
                            className="bg-emerald-500 h-full rounded-full"
                            style={{ width: `${Math.min(100, (vpn.usedTrafficGb / vpn.totalTrafficGb) * 100)}%` }}
                          ></div>
                        </div>
                      </div>
                    </td>
                    <td className="py-2.5">
                      <span className={`px-2 py-0.5 rounded-full text-[10px] font-bold ${
                        vpn.status === 'ACTIVE' ? 'bg-emerald-500/10 text-emerald-400' : 'bg-amber-500/10 text-amber-400'
                      }`}>
                        {vpn.status}
                      </span>
                    </td>
                    <td className="py-2.5 text-right">
                      <button
                        onClick={() => onSelectVpnForQr(vpn)}
                        className="px-2.5 py-1 bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-400 rounded-lg text-xs font-semibold border border-emerald-500/20 transition"
                      >
                        Show QR
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        {/* Live Security Audit Log Stream */}
        <div className="p-5 bg-neutral-900/90 border border-neutral-800 rounded-2xl space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2">
              <ShieldCheck className="w-4 h-4 text-emerald-400" />
              <h3 className="text-sm font-bold text-white">Live Audit Security Stream</h3>
            </div>
            <button
              onClick={() => onNavigate('audit')}
              className="text-xs text-neutral-400 hover:text-white"
            >
              Full Log
            </button>
          </div>

          <div className="space-y-3">
            {auditLogs.slice(0, 5).map(log => (
              <div 
                key={log.id} 
                className="p-2.5 bg-neutral-950/70 border border-neutral-800/80 rounded-xl space-y-1 text-xs"
              >
                <div className="flex items-center justify-between">
                  <span className="font-mono text-[10px] text-emerald-400 font-semibold">{log.action}</span>
                  <span className="text-[10px] text-neutral-500">
                    {new Date(log.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                  </span>
                </div>
                <p className="text-neutral-300 text-[11px] leading-snug">{log.details}</p>
                <div className="flex items-center justify-between text-[10px] text-neutral-500 pt-0.5">
                  <span>Actor: {log.actorName}</span>
                  <span className="font-mono">{log.ipAddress}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};
