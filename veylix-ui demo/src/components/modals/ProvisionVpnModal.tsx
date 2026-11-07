import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { X, ShieldPlus, Server, User, Gauge, Calendar, AlertCircle, CheckCircle2 } from 'lucide-react';
import { VpnProtocol } from '../../types';

interface ProvisionVpnModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccessOpenQr?: (vpnAccount: any) => void;
  preselectedClientId?: string;
}

export const ProvisionVpnModal: React.FC<ProvisionVpnModalProps> = ({ 
  isOpen, 
  onClose, 
  onSuccessOpenQr,
  preselectedClientId 
}) => {
  const { servers, users, currentUser, currentRole, provisionVpnAccount } = useVeylixStore();

  const clients = users.filter(u => u.role === 'client');
  const availableServers = currentRole === 'admin' && currentUser.assignedServerIds
    ? servers.filter(s => currentUser.assignedServerIds?.includes(s.id))
    : servers;

  const [clientId, setClientId] = useState(preselectedClientId || clients[0]?.id || '');
  const [serverId, setServerId] = useState(availableServers[0]?.id || '');
  const [protocol, setProtocol] = useState<VpnProtocol>('VLESS');
  const [totalTrafficGb, setTotalTrafficGb] = useState(100);
  const [durationDays, setDurationDays] = useState(30);
  const [vpnName, setVpnName] = useState('');
  const [errorMsg, setErrorMsg] = useState('');

  if (!isOpen) return null;

  const selectedServer = servers.find(s => s.id === serverId) || availableServers[0];
  const matchingInbound = selectedServer?.inbounds.find(i => i.protocol === protocol) || selectedServer?.inbounds[0];

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!clientId) {
      setErrorMsg('Please select a client account.');
      return;
    }
    if (!serverId || !matchingInbound) {
      setErrorMsg('Please select an active server and inbound.');
      return;
    }

    const result = provisionVpnAccount({
      name: vpnName || 'Auto-Provisioned-Key',
      serverId: selectedServer.id,
      inboundId: matchingInbound.id,
      protocol: protocol,
      clientId: clientId,
      totalTrafficGb: totalTrafficGb,
      durationDays: durationDays,
      adminId: currentRole === 'admin' ? currentUser.id : undefined
    });

    if (!result.success) {
      setErrorMsg(result.error || 'Failed to provision VPN account.');
      return;
    }

    onClose();
    if (onSuccessOpenQr && result.account) {
      onSuccessOpenQr(result.account);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="relative w-full max-w-lg bg-neutral-900 border border-neutral-800 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[92vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-800 bg-neutral-950/50">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
              <ShieldPlus className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-semibold text-white">Provision VPN Access</h3>
              <p className="text-xs text-neutral-400">
                {currentRole === 'admin' 
                  ? `Reseller Quota Available: ${(currentUser.quotaLimit || 0) - (currentUser.quotaUsed || 0)} accounts`
                  : 'Super Admin Direct Infrastructure Provisioning'}
              </p>
            </div>
          </div>
          <button 
            onClick={onClose}
            className="p-1.5 text-neutral-400 hover:text-white rounded-lg hover:bg-neutral-800 transition"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Form Body */}
        <form onSubmit={handleSubmit} className="p-6 overflow-y-auto space-y-4 text-xs">
          {errorMsg && (
            <div className="p-3 bg-rose-500/10 border border-rose-500/30 rounded-xl text-rose-300 flex items-center space-x-2">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{errorMsg}</span>
            </div>
          )}

          {/* Client Selection */}
          <div className="space-y-1">
            <label className="text-neutral-300 font-medium flex items-center space-x-1.5">
              <User className="w-3.5 h-3.5 text-neutral-400" />
              <span>Assign To Client</span>
            </label>
            <select
              value={clientId}
              onChange={(e) => setClientId(e.target.value)}
              required
              className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 focus:outline-none focus:border-emerald-500/50"
            >
              <option value="">-- Choose Client --</option>
              {clients.map(c => (
                <option key={c.id} value={c.id}>
                  {c.name} ({c.email})
                </option>
              ))}
            </select>
          </div>

          {/* Target Server Selection */}
          <div className="space-y-1">
            <label className="text-neutral-300 font-medium flex items-center space-x-1.5">
              <Server className="w-3.5 h-3.5 text-neutral-400" />
              <span>Target 3X-UI Server Node</span>
            </label>
            <select
              value={serverId}
              onChange={(e) => setServerId(e.target.value)}
              required
              className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 focus:outline-none focus:border-emerald-500/50"
            >
              {availableServers.map(s => (
                <option key={s.id} value={s.id}>
                  {s.flag} {s.name} ({s.latencyMs}ms ping • {s.countryCode})
                </option>
              ))}
            </select>
          </div>

          {/* Protocol Chooser */}
          <div className="space-y-1">
            <label className="text-neutral-300 font-medium">Protocol & Obfuscation Layer</label>
            <div className="grid grid-cols-4 gap-2">
              {(['VLESS', 'VMess', 'Trojan', 'Shadowsocks'] as VpnProtocol[]).map((p) => (
                <button
                  type="button"
                  key={p}
                  onClick={() => setProtocol(p)}
                  className={`py-2 px-1 text-center font-semibold rounded-xl border transition ${
                    protocol === p
                      ? 'bg-emerald-500/20 text-emerald-400 border-emerald-500/50'
                      : 'bg-neutral-950 border-neutral-800 text-neutral-400 hover:text-white'
                  }`}
                >
                  {p}
                </button>
              ))}
            </div>
            <p className="text-[11px] text-neutral-500">
              {protocol === 'VLESS' && 'XTLS Reality obfuscation with TLS Camouflage (Fastest for anti-censorship)'}
              {protocol === 'VMess' && 'WebSocket transport over TLS 443 with Cloudflare CDN compatibility'}
              {protocol === 'Trojan' && 'Standard HTTPS masquerade with gRPC high-throughput tunneling'}
              {protocol === 'Shadowsocks' && '2022-blake3 cipher with minimal CPU overhead for ultra-low ping gaming'}
            </p>
          </div>

          {/* Traffic and Duration */}
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1">
              <label className="text-neutral-300 font-medium flex items-center space-x-1.5">
                <Gauge className="w-3.5 h-3.5 text-neutral-400" />
                <span>Traffic Quota (GB)</span>
              </label>
              <input
                type="number"
                min={1}
                max={5000}
                value={totalTrafficGb}
                onChange={(e) => setTotalTrafficGb(Number(e.target.value))}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 font-mono"
              />
            </div>

            <div className="space-y-1">
              <label className="text-neutral-300 font-medium flex items-center space-x-1.5">
                <Calendar className="w-3.5 h-3.5 text-neutral-400" />
                <span>Duration (Days)</span>
              </label>
              <input
                type="number"
                min={1}
                max={365}
                value={durationDays}
                onChange={(e) => setDurationDays(Number(e.target.value))}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 font-mono"
              />
            </div>
          </div>

          {/* Custom Remark/Name */}
          <div className="space-y-1">
            <label className="text-neutral-300 font-medium">Custom Remark Tag (Optional)</label>
            <input
              type="text"
              placeholder="e.g. VIP-Work-SG"
              value={vpnName}
              onChange={(e) => setVpnName(e.target.value)}
              className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 placeholder-neutral-500 font-mono"
            />
          </div>

          {/* Footer */}
          <div className="pt-3 flex items-center justify-end space-x-3 border-t border-neutral-800">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-xs font-medium text-neutral-400 hover:text-white bg-neutral-800 rounded-xl transition"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="px-5 py-2 text-xs font-semibold text-neutral-950 bg-emerald-400 hover:bg-emerald-300 rounded-xl transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
            >
              <ShieldPlus className="w-4 h-4" />
              <span>Issue & Generate QR</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
