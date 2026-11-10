import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  Server, 
  Plus, 
  RefreshCw, 
  Trash2, 
  ShieldCheck, 
  Cpu, 
  HardDrive, 
  Globe, 
  ExternalLink, 
  AlertCircle,
  Activity,
  CheckCircle2,
  Users
} from 'lucide-react';
import { ThreeXUIServer } from '../../types';

interface ServerManagementViewProps {
  onOpenAddServer: () => void;
  onOpenProvision: () => void;
}

export const ServerManagementView: React.FC<ServerManagementViewProps> = ({ 
  onOpenAddServer,
  onOpenProvision 
}) => {
  const { servers, syncServer, deleteServer, users } = useVeylixStore();
  const [selectedServerId, setSelectedServerId] = useState<string>(servers[0]?.id || '');
  const [statusMsg, setStatusMsg] = useState<{ type: 'error' | 'success'; text: string } | null>(null);

  const selectedServer = servers.find(s => s.id === selectedServerId) || servers[0];

  const handleDelete = (id: string) => {
    if (confirm('Are you sure you want to delete this 3X-UI server connection?')) {
      const success = deleteServer(id);
      if (!success) {
        setStatusMsg({ type: 'error', text: 'Cannot delete server with active VPN accounts. Revoke or transfer accounts first.' });
      } else {
        setStatusMsg({ type: 'success', text: 'Server connection removed successfully.' });
      }
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-extrabold text-white tracking-tight flex items-center space-x-2">
            <Server className="w-5 h-5 text-emerald-400" />
            <span>3X-UI Cluster Server Management</span>
          </h1>
          <p className="text-xs text-neutral-400">
            Control external Xray-core panels, telemetry sync, inbounds, and reseller permissions
          </p>
        </div>
        <div className="flex items-center space-x-2.5">
          <button
            onClick={() => servers.forEach(s => syncServer(s.id))}
            className="px-3.5 py-2 bg-neutral-900 hover:bg-neutral-800 border border-neutral-800 rounded-xl text-xs font-medium text-neutral-300 hover:text-white transition flex items-center space-x-1.5"
          >
            <RefreshCw className="w-3.5 h-3.5 text-emerald-400" />
            <span>Sync All Panels</span>
          </button>
          <button
            onClick={onOpenAddServer}
            className="px-4 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl text-xs transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
          >
            <Plus className="w-4 h-4" />
            <span>Add 3X-UI Server</span>
          </button>
        </div>
      </div>

      {statusMsg && (
        <div className={`p-3 rounded-xl border flex items-center justify-between text-xs ${
          statusMsg.type === 'error' ? 'bg-rose-500/10 border-rose-500/30 text-rose-300' : 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300'
        }`}>
          <div className="flex items-center space-x-2">
            {statusMsg.type === 'error' ? <AlertCircle className="w-4 h-4" /> : <CheckCircle2 className="w-4 h-4" />}
            <span>{statusMsg.text}</span>
          </div>
          <button onClick={() => setStatusMsg(null)} className="text-neutral-400 hover:text-white">✕</button>
        </div>
      )}

      {/* Servers Horizontal Selector */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {servers.map(server => {
          const isSelected = selectedServer?.id === server.id;
          return (
            <div
              key={server.id}
              onClick={() => setSelectedServerId(server.id)}
              className={`p-4 rounded-2xl border cursor-pointer transition relative group ${
                isSelected 
                  ? 'bg-neutral-900 border-emerald-500/60 shadow-lg shadow-emerald-500/5' 
                  : 'bg-neutral-950/70 border-neutral-800 hover:border-neutral-700'
              }`}
            >
              <div className="flex items-start justify-between">
                <div className="flex items-center space-x-2">
                  <span className="text-2xl">{server.flag}</span>
                  <div>
                    <h3 className="text-xs font-bold text-white truncate max-w-[130px]">{server.name}</h3>
                    <p className="text-[11px] font-mono text-neutral-400">{server.countryCode} • {server.port}</p>
                  </div>
                </div>
                <span className="px-2 py-0.5 text-[10px] font-mono font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 rounded-full">
                  {server.latencyMs}ms
                </span>
              </div>

              <div className="mt-3 pt-3 border-t border-neutral-800/80 flex items-center justify-between text-[11px] text-neutral-400">
                <span>{server.inbounds.length} Inbounds</span>
                <span className="text-emerald-400 font-medium">{server.status}</span>
              </div>
            </div>
          );
        })}
      </div>

      {/* Selected Server Detailed Inspector */}
      {selectedServer && (
        <div className="p-6 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl space-y-6">
          {/* Header of selected server */}
          <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4 pb-6 border-b border-neutral-800">
            <div className="flex items-center space-x-3">
              <span className="text-3xl p-2 bg-neutral-950 rounded-2xl border border-neutral-800">{selectedServer.flag}</span>
              <div>
                <div className="flex items-center space-x-2">
                  <h2 className="text-base font-bold text-white">{selectedServer.name}</h2>
                  <span className="px-2 py-0.5 rounded-full text-[10px] font-mono font-bold bg-emerald-500/20 text-emerald-400 border border-emerald-500/40">
                    ONLINE
                  </span>
                </div>
                <p className="text-xs text-neutral-400 font-mono mt-0.5">
                  Endpoint: {selectedServer.apiUrl} • Location: {selectedServer.location}
                </p>
              </div>
            </div>

            <div className="flex items-center space-x-2.5">
              <button
                onClick={() => syncServer(selectedServer.id)}
                disabled={selectedServer.syncStatus === 'SYNCING'}
                className="px-3 py-1.5 bg-neutral-800 hover:bg-neutral-700 text-white rounded-xl text-xs font-medium flex items-center space-x-1.5 transition"
              >
                <RefreshCw className={`w-3.5 h-3.5 ${selectedServer.syncStatus === 'SYNCING' ? 'animate-spin text-emerald-400' : ''}`} />
                <span>Sync Node</span>
              </button>
              <button
                onClick={onOpenProvision}
                className="px-3.5 py-1.5 bg-emerald-500/20 hover:bg-emerald-500/30 border border-emerald-500/30 text-emerald-300 rounded-xl text-xs font-semibold transition"
              >
                Issue Key on this Node
              </button>
              <button
                onClick={() => handleDelete(selectedServer.id)}
                className="p-2 text-neutral-400 hover:text-rose-400 rounded-xl hover:bg-rose-500/10 transition"
                title="Remove connection"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          </div>

          {/* Node Hardware & Specs telemetry */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="p-3.5 bg-neutral-950 border border-neutral-800 rounded-xl space-y-1">
              <span className="text-xs text-neutral-400">Panel & Xray Core</span>
              <p className="font-semibold text-white text-xs">{selectedServer.panelVersion}</p>
              <p className="text-[11px] font-mono text-emerald-400">Core: {selectedServer.xrayVersion}</p>
            </div>

            <div className="space-y-1 p-3.5 bg-neutral-950 border border-neutral-800 rounded-xl">
              <div className="flex justify-between text-xs text-neutral-400">
                <span>CPU Load:</span>
                <span className="font-mono text-white">{selectedServer.cpuUsagePercent}%</span>
              </div>
              <div className="w-full bg-neutral-800 h-1.5 rounded-full overflow-hidden mt-2">
                <div 
                  className="bg-emerald-400 h-full rounded-full"
                  style={{ width: `${selectedServer.cpuUsagePercent}%` }}
                ></div>
              </div>
            </div>

            <div className="space-y-1 p-3.5 bg-neutral-950 border border-neutral-800 rounded-xl">
              <div className="flex justify-between text-xs text-neutral-400">
                <span>RAM Usage:</span>
                <span className="font-mono text-white">{selectedServer.ramUsedMb} / {selectedServer.ramTotalMb} MB</span>
              </div>
              <div className="w-full bg-neutral-800 h-1.5 rounded-full overflow-hidden mt-2">
                <div 
                  className="bg-cyan-400 h-full rounded-full"
                  style={{ width: `${selectedServer.ramUsagePercent}%` }}
                ></div>
              </div>
            </div>

            <div className="space-y-1 p-3.5 bg-neutral-950 border border-neutral-800 rounded-xl">
              <div className="flex justify-between text-xs text-neutral-400">
                <span>Disk Utilization:</span>
                <span className="font-mono text-white">{selectedServer.diskUsagePercent}%</span>
              </div>
              <div className="w-full bg-neutral-800 h-1.5 rounded-full overflow-hidden mt-2">
                <div 
                  className="bg-violet-400 h-full rounded-full"
                  style={{ width: `${selectedServer.diskUsagePercent}%` }}
                ></div>
              </div>
            </div>
          </div>

          {/* Inbound Protocols Inspector */}
          <div className="space-y-3">
            <h3 className="text-sm font-bold text-white flex items-center space-x-2">
              <ShieldCheck className="w-4 h-4 text-emerald-400" />
              <span>Configured 3X-UI Inbounds on this Node</span>
            </h3>

            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead>
                  <tr className="border-b border-neutral-800 text-neutral-400 bg-neutral-950/60">
                    <th className="py-2.5 px-3 font-medium">Inbound Port</th>
                    <th className="py-2.5 px-3 font-medium">Protocol</th>
                    <th className="py-2.5 px-3 font-medium">Remark</th>
                    <th className="py-2.5 px-3 font-medium">Network / Security</th>
                    <th className="py-2.5 px-3 font-medium">Active Clients</th>
                    <th className="py-2.5 px-3 font-medium">Traffic Counter</th>
                    <th className="py-2.5 px-3 font-medium text-right">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-neutral-800/60">
                  {selectedServer.inbounds.map(inbound => (
                    <tr key={inbound.id} className="hover:bg-neutral-800/30 transition">
                      <td className="py-3 px-3 font-mono font-bold text-emerald-400">:{inbound.port}</td>
                      <td className="py-3 px-3">
                        <span className="px-2 py-0.5 rounded font-mono font-semibold bg-neutral-800 text-neutral-200">
                          {inbound.protocol}
                        </span>
                      </td>
                      <td className="py-3 px-3 text-neutral-200 font-medium">{inbound.remark}</td>
                      <td className="py-3 px-3">
                        <span className="capitalize text-neutral-300 font-mono text-[11px]">
                          {inbound.network} • {inbound.security}
                        </span>
                      </td>
                      <td className="py-3 px-3 text-neutral-200 font-mono font-medium">
                        {inbound.clientCount} clients
                      </td>
                      <td className="py-3 px-3 font-mono text-neutral-400 text-[11px]">
                        ↑ {(inbound.upTrafficBytes / 1e9).toFixed(1)} GB / ↓ {(inbound.downTrafficBytes / 1e9).toFixed(1)} GB
                      </td>
                      <td className="py-3 px-3 text-right">
                        <span className="px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-500/10 text-emerald-400">
                          ACTIVE
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
