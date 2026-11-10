import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  KeyRound, 
  QrCode, 
  Copy, 
  Check, 
  Gift, 
  Calendar, 
  Server, 
  MessageSquare, 
  BookOpen, 
  Zap, 
  ShieldCheck, 
  ExternalLink,
  AlertCircle,
  HelpCircle,
  ArrowRight
} from 'lucide-react';
import { VpnAccount } from '../../types';

interface ClientPortalViewProps {
  onSelectVpnForQr: (vpn: VpnAccount) => void;
  onOpenSetupGuide: () => void;
  onNavigateToStore: () => void;
  onOpenChat: (userId: string) => void;
}

export const ClientPortalView: React.FC<ClientPortalViewProps> = ({
  onSelectVpnForQr,
  onOpenSetupGuide,
  onNavigateToStore,
  onOpenChat
}) => {
  const { 
    currentUser, 
    vpnAccounts, 
    claimFreeVpn, 
    systemSettings, 
    users 
  } = useVeylixStore();

  const [copiedId, setCopiedId] = useState<string>('');
  const [claimStatus, setClaimStatus] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  const myVpns = vpnAccounts.filter(v => v.clientId === currentUser.id);
  const seller = currentUser.assignedAdminId 
    ? users.find(u => u.id === currentUser.assignedAdminId) 
    : users.find(u => u.role === 'super_admin');

  const handleCopyUri = (vpn: VpnAccount) => {
    navigator.clipboard.writeText(vpn.connectionUri);
    setCopiedId(vpn.id);
    setTimeout(() => setCopiedId(''), 2000);
  };

  const handleClaimFree = () => {
    const res = claimFreeVpn(currentUser.id);
    if (res.success && res.account) {
      setClaimStatus({ type: 'success', text: `Success! Free ${systemSettings.freeVpnTrafficLimitGb}GB pass generated. Click Show QR to connect!` });
      onSelectVpnForQr(res.account);
    } else {
      setClaimStatus({ type: 'error', text: res.error || 'Failed to claim free VPN.' });
    }
  };

  return (
    <div className="space-y-6">
      {/* Welcome Banner */}
      <div className="p-6 bg-gradient-to-r from-neutral-900 via-neutral-900 to-neutral-950 border border-neutral-800 rounded-2xl shadow-xl flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div className="space-y-1">
          <h1 className="text-xl font-extrabold text-white tracking-tight">
            Welcome, {currentUser.name}
          </h1>
          <p className="text-xs text-neutral-400">
            High-speed encrypted tunnels via VLESS Reality, VMess, and Shadowsocks
          </p>
        </div>

        <div className="flex items-center space-x-2.5">
          <button
            onClick={onOpenSetupGuide}
            className="px-3.5 py-2 bg-neutral-800 hover:bg-neutral-700 text-white rounded-xl text-xs font-medium flex items-center space-x-1.5 transition"
          >
            <BookOpen className="w-3.5 h-3.5 text-cyan-400" />
            <span>Connection Guides</span>
          </button>
          <button
            onClick={onNavigateToStore}
            className="px-4 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl text-xs transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
          >
            <span>Upgrade / Add Traffic</span>
            <ArrowRight className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Free VPN Claim Banner if enabled */}
      {systemSettings.freeVpnEnabled && (
        <div className="p-5 bg-gradient-to-r from-emerald-950/40 via-neutral-900 to-neutral-950 border border-emerald-500/30 rounded-2xl shadow-xl flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div className="flex items-center space-x-3.5">
            <div className="w-11 h-11 rounded-2xl bg-emerald-500/20 text-emerald-400 flex items-center justify-center shrink-0">
              <Gift className="w-6 h-6" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <h3 className="font-bold text-white text-sm">Complimentary Free Starter Pass</h3>
                <span className="px-2 py-0.5 bg-emerald-500/20 text-emerald-400 text-[10px] rounded font-mono font-bold">
                  ACTIVE PROMO
                </span>
              </div>
              <p className="text-xs text-neutral-400 mt-0.5">
                Claim {systemSettings.freeVpnTrafficLimitGb}GB high-speed VLESS Reality access valid for {systemSettings.freeVpnDurationDays} days. Zero payment needed.
              </p>
            </div>
          </div>

          <button
            onClick={handleClaimFree}
            className="px-5 py-2.5 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-bold rounded-xl text-xs transition shrink-0 shadow-lg shadow-emerald-500/10"
          >
            Claim Free Pass Now
          </button>
        </div>
      )}

      {claimStatus && (
        <div className={`p-3 rounded-xl border flex items-center justify-between text-xs ${
          claimStatus.type === 'error' ? 'bg-rose-500/10 border-rose-500/30 text-rose-300' : 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300'
        }`}>
          <span>{claimStatus.text}</span>
          <button onClick={() => setClaimStatus(null)} className="text-neutral-400 hover:text-white">✕</button>
        </div>
      )}

      {/* Active VPN Keys Grid */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-bold text-white flex items-center space-x-2">
            <KeyRound className="w-4 h-4 text-emerald-400" />
            <span>My Active VPN Configurations ({myVpns.length})</span>
          </h2>
          <span className="text-xs text-neutral-500">Scan QR directly on Android / iOS / PC</span>
        </div>

        {myVpns.length === 0 ? (
          <div className="p-10 bg-neutral-900 border border-neutral-800 rounded-2xl text-center space-y-3">
            <KeyRound className="w-8 h-8 text-neutral-600 mx-auto" />
            <h3 className="text-sm font-bold text-white">No Active VPN Configurations Found</h3>
            <p className="text-xs text-neutral-400 max-w-sm mx-auto">
              Claim the free starter pass above or order a premium package to get instant high-speed connection links.
            </p>
            <button
              onClick={handleClaimFree}
              className="px-4 py-2 bg-emerald-400 text-neutral-950 font-semibold rounded-xl text-xs"
            >
              Claim Free 5GB Key
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {myVpns.map(vpn => {
              const percentUsed = Math.min(100, Math.round((vpn.usedTrafficGb / (vpn.totalTrafficGb || 1)) * 100));
              const remainingGb = Math.max(0, vpn.totalTrafficGb - vpn.usedTrafficGb);
              const daysLeft = Math.max(0, Math.ceil((new Date(vpn.expiryDate).getTime() - Date.now()) / (1000 * 60 * 60 * 24)));

              return (
                <div
                  key={vpn.id}
                  className="p-5 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl space-y-4 hover:border-neutral-700 transition"
                >
                  <div className="flex items-start justify-between">
                    <div>
                      <div className="flex items-center space-x-2">
                        <h3 className="font-bold text-white text-sm">{vpn.name}</h3>
                        {vpn.isFree && (
                          <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-emerald-500/20 text-emerald-400">
                            FREE PASS
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-neutral-400 flex items-center space-x-1.5 mt-0.5">
                        <Server className="w-3.5 h-3.5 text-neutral-500" />
                        <span>{vpn.serverName} ({vpn.protocol})</span>
                      </p>
                    </div>

                    <span className="px-2.5 py-0.5 rounded-full text-[10px] font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/30">
                      ACTIVE
                    </span>
                  </div>

                  {/* Traffic & Expiry Progress Card */}
                  <div className="p-3.5 bg-neutral-950 border border-neutral-800/80 rounded-xl space-y-2 text-xs">
                    <div className="flex items-center justify-between">
                      <span className="text-neutral-400">Bandwidth Consumption:</span>
                      <span className="font-mono text-emerald-400 font-bold">
                        {vpn.usedTrafficGb.toFixed(1)} / {vpn.totalTrafficGb} GB
                      </span>
                    </div>

                    <div className="w-full bg-neutral-800 h-2 rounded-full overflow-hidden">
                      <div 
                        className={`h-full rounded-full transition-all ${
                          percentUsed > 85 ? 'bg-amber-500' : 'bg-emerald-500'
                        }`}
                        style={{ width: `${percentUsed}%` }}
                      ></div>
                    </div>

                    <div className="flex items-center justify-between text-[11px] font-mono text-neutral-400 pt-1">
                      <span>Remaining: <strong className="text-white">{remainingGb.toFixed(1)} GB</strong></span>
                      <span className="flex items-center space-x-1">
                        <Calendar className="w-3 h-3" />
                        <span>Expires in <strong>{daysLeft} days</strong></span>
                      </span>
                    </div>
                  </div>

                  {/* Action Buttons */}
                  <div className="pt-2 flex items-center space-x-2.5">
                    <button
                      onClick={() => onSelectVpnForQr(vpn)}
                      className="flex-1 py-2 bg-emerald-500/15 hover:bg-emerald-500/25 border border-emerald-500/30 text-emerald-300 font-semibold rounded-xl text-xs flex items-center justify-center space-x-1.5 transition"
                    >
                      <QrCode className="w-3.5 h-3.5" />
                      <span>Show QR Code</span>
                    </button>

                    <button
                      onClick={() => handleCopyUri(vpn)}
                      className="px-3 py-2 bg-neutral-800 hover:bg-neutral-700 text-neutral-300 hover:text-white rounded-xl text-xs font-medium flex items-center space-x-1.5 transition"
                    >
                      {copiedId === vpn.id ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                      <span>{copiedId === vpn.id ? 'Copied' : 'Copy Link'}</span>
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Seller & Support Contact Card */}
      {seller && (
        <div className="p-5 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div className="space-y-1">
            <span className="text-[11px] font-medium text-neutral-400">YOUR AUTHORIZED RESELLER / SUPPORT</span>
            <h3 className="font-bold text-white text-sm">{seller.name}</h3>
            <p className="text-xs text-neutral-400 font-mono">
              Email: {seller.email} {seller.telegram ? `• Telegram: ${seller.telegram}` : ''}
            </p>
          </div>

          <button
            onClick={() => onOpenChat(seller.id)}
            className="px-4 py-2 bg-neutral-800 hover:bg-neutral-700 text-white rounded-xl text-xs font-semibold flex items-center space-x-1.5 transition"
          >
            <MessageSquare className="w-3.5 h-3.5 text-emerald-400" />
            <span>Open Direct Support Chat</span>
          </button>
        </div>
      )}
    </div>
  );
};
