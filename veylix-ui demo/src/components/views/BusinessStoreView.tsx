import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  Package, 
  Plus, 
  Gift, 
  Check, 
  Sparkles, 
  Zap, 
  ShieldCheck, 
  DollarSign, 
  Sliders, 
  Server,
  Trash2,
  Edit3
} from 'lucide-react';
import { VpnPackage, VpnProtocol } from '../../types';

interface BusinessStoreViewProps {
  onOrderPackage?: (pkg: VpnPackage) => void;
}

export const BusinessStoreView: React.FC<BusinessStoreViewProps> = ({ onOrderPackage }) => {
  const { 
    packages, 
    systemSettings, 
    updateSystemSettings, 
    currentRole, 
    addPackage, 
    deletePackage, 
    servers 
  } = useVeylixStore();

  const [showAddModal, setShowAddModal] = useState(false);
  const [name, setName] = useState('');
  const [tagline, setTagline] = useState('');
  const [protocol, setProtocol] = useState<VpnProtocol>('VLESS');
  const [trafficGb, setTrafficGb] = useState(100);
  const [durationDays, setDurationDays] = useState(30);
  const [superAdminPrice, setSuperAdminPrice] = useState(3.50);
  const [retailPrice, setRetailPrice] = useState(7.99);
  const [featuresList, setFeaturesList] = useState('High-Speed Singapore Node\nAnti-Censorship Reality\n5 Simultaneous Devices');

  const isSuperAdmin = currentRole === 'super_admin';

  const handleToggleFreePolicy = () => {
    updateSystemSettings({ freeVpnEnabled: !systemSettings.freeVpnEnabled });
  };

  const handleCreatePackage = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name) return;

    addPackage({
      name,
      tagline,
      protocol,
      trafficGb,
      durationDays,
      superAdminPrice,
      retailPrice,
      features: featuresList.split('\n').filter(f => f.trim().length > 0),
      recommendedServers: [servers[0]?.id || ''],
      status: 'ACTIVE'
    });

    setShowAddModal(false);
    setName('');
    setTagline('');
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-extrabold text-white tracking-tight flex items-center space-x-2">
            <Package className="w-5 h-5 text-emerald-400" />
            <span>Commercial Packages & Pricing</span>
          </h1>
          <p className="text-xs text-neutral-400">
            Configure wholesale allocation prices, retail customer rates, and automated free trial policies
          </p>
        </div>

        {isSuperAdmin && (
          <button
            onClick={() => setShowAddModal(true)}
            className="px-4 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl text-xs transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
          >
            <Plus className="w-4 h-4" />
            <span>Create New Package</span>
          </button>
        )}
      </div>

      {/* Free VPN Policy Controls (Super Admin only) */}
      {isSuperAdmin && (
        <div className="p-5 bg-gradient-to-r from-neutral-900 to-neutral-950 border border-neutral-800 rounded-2xl shadow-xl space-y-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-3">
              <div className="w-10 h-10 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 flex items-center justify-center">
                <Gift className="w-5 h-5" />
              </div>
              <div>
                <h3 className="font-bold text-white text-sm">Free VPN Distribution Policy</h3>
                <p className="text-xs text-neutral-400">Section 3.2 Master Specification • Automated trial onboarding</p>
              </div>
            </div>

            <button
              onClick={handleToggleFreePolicy}
              className={`px-4 py-1.5 rounded-xl text-xs font-semibold transition border ${
                systemSettings.freeVpnEnabled
                  ? 'bg-emerald-500/20 text-emerald-300 border-emerald-500/40 hover:bg-emerald-500/30'
                  : 'bg-neutral-800 text-neutral-400 border-neutral-700'
              }`}
            >
              {systemSettings.freeVpnEnabled ? 'Active (Clients Can Claim)' : 'Disabled'}
            </button>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-2 text-xs">
            <div className="p-3 bg-neutral-950 border border-neutral-800/80 rounded-xl">
              <span className="text-neutral-400">Traffic Limit Per Pass:</span>
              <p className="font-mono text-emerald-400 font-bold text-sm mt-0.5">
                {systemSettings.freeVpnTrafficLimitGb} GB High-Speed
              </p>
            </div>
            <div className="p-3 bg-neutral-950 border border-neutral-800/80 rounded-xl">
              <span className="text-neutral-400">Trial Validity Duration:</span>
              <p className="font-mono text-white font-bold text-sm mt-0.5">
                {systemSettings.freeVpnDurationDays} Days from Claim
              </p>
            </div>
            <div className="p-3 bg-neutral-950 border border-neutral-800/80 rounded-xl">
              <span className="text-neutral-400">Max Active Passes per Client:</span>
              <p className="font-mono text-cyan-400 font-bold text-sm mt-0.5">
                {systemSettings.freeVpnMaxPerUser} Pass (Prevents duplicate abuse)
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Packages Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {packages.map(pkg => {
          const isFree = pkg.isFree || pkg.retailPrice === 0;

          return (
            <div
              key={pkg.id}
              className={`p-5 rounded-2xl border flex flex-col justify-between transition relative ${
                pkg.isPopular 
                  ? 'bg-neutral-900 border-emerald-500/50 shadow-xl shadow-emerald-500/5' 
                  : 'bg-neutral-900/80 border-neutral-800 hover:border-neutral-700'
              }`}
            >
              {pkg.isPopular && (
                <div className="absolute -top-3 left-1/2 -translate-x-1/2 px-3 py-0.5 bg-emerald-500 text-neutral-950 text-[10px] font-extrabold uppercase rounded-full shadow font-mono">
                  POPULAR CHOICE
                </div>
              )}

              <div className="space-y-3">
                <div className="flex items-start justify-between">
                  <div>
                    <h3 className="font-bold text-white text-sm">{pkg.name}</h3>
                    <span className="px-2 py-0.5 rounded text-[10px] font-mono font-semibold bg-neutral-800 text-neutral-300 mt-1 inline-block">
                      {pkg.protocol}
                    </span>
                  </div>
                  {isSuperAdmin && !pkg.isFree && (
                    <button
                      onClick={() => deletePackage(pkg.id)}
                      className="text-neutral-500 hover:text-rose-400 p-1"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  )}
                </div>

                <p className="text-[11px] text-neutral-400 leading-relaxed min-h-[32px]">
                  {pkg.tagline}
                </p>

                {/* Pricing Block */}
                <div className="p-3 bg-neutral-950 border border-neutral-800/80 rounded-xl space-y-1">
                  <div className="flex items-baseline space-x-1">
                    <span className="text-2xl font-black text-white font-mono">
                      {isFree ? 'FREE' : `$${pkg.retailPrice.toFixed(2)}`}
                    </span>
                    {!isFree && <span className="text-xs text-neutral-400">/ {pkg.durationDays}d</span>}
                  </div>

                  {isSuperAdmin && !isFree && (
                    <div className="text-[10px] text-neutral-400 pt-1 border-t border-neutral-900 flex justify-between font-mono">
                      <span>Wholesale Cost:</span>
                      <span className="text-emerald-400">${pkg.superAdminPrice.toFixed(2)}</span>
                    </div>
                  )}
                </div>

                {/* Specs List */}
                <ul className="space-y-1.5 text-[11px] text-neutral-300 pt-1">
                  <li className="flex items-center space-x-2">
                    <Check className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                    <span><strong>{pkg.trafficGb} GB</strong> High-Speed Traffic</span>
                  </li>
                  <li className="flex items-center space-x-2">
                    <Check className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                    <span><strong>{pkg.durationDays} Days</strong> Service Validity</span>
                  </li>
                  {pkg.features.map((feat, i) => (
                    <li key={i} className="flex items-center space-x-2">
                      <Check className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                      <span>{feat}</span>
                    </li>
                  ))}
                </ul>
              </div>

              {/* Action Button */}
              <div className="pt-4 mt-4 border-t border-neutral-800/80">
                {currentRole === 'client' ? (
                  <button
                    onClick={() => onOrderPackage && onOrderPackage(pkg)}
                    className="w-full py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl text-xs transition text-center shadow-lg shadow-emerald-500/10"
                  >
                    {isFree ? 'Claim Free Pass' : 'Order Package'}
                  </button>
                ) : (
                  <div className="text-center text-[11px] text-neutral-400">
                    <span>Ready for automated issuance</span>
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {/* Add Package Modal */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in">
          <div className="w-full max-w-md bg-neutral-900 border border-neutral-800 rounded-2xl p-6 shadow-2xl space-y-4 text-xs">
            <h3 className="text-sm font-bold text-white">Create New Commercial VPN Package</h3>

            <form onSubmit={handleCreatePackage} className="space-y-3">
              <div className="space-y-1">
                <label className="text-neutral-300 font-medium">Package Title</label>
                <input
                  type="text"
                  required
                  placeholder="e.g. VIP Gaming Low-Ping 200GB"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                />
              </div>

              <div className="space-y-1">
                <label className="text-neutral-300 font-medium">Short Tagline</label>
                <input
                  type="text"
                  placeholder="Sub-25ms gaming with Shadowsocks 2022"
                  value={tagline}
                  onChange={(e) => setTagline(e.target.value)}
                  className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1">
                  <label className="text-neutral-300 font-medium">Protocol</label>
                  <select
                    value={protocol}
                    onChange={(e) => setProtocol(e.target.value as any)}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                  >
                    <option value="VLESS">VLESS (Reality)</option>
                    <option value="VMess">VMess (WS TLS)</option>
                    <option value="Trojan">Trojan (gRPC)</option>
                    <option value="Shadowsocks">Shadowsocks (2022)</option>
                  </select>
                </div>

                <div className="space-y-1">
                  <label className="text-neutral-300 font-medium">Traffic (GB)</label>
                  <input
                    type="number"
                    value={trafficGb}
                    onChange={(e) => setTrafficGb(Number(e.target.value))}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white font-mono focus:outline-none focus:border-emerald-500/50"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1">
                  <label className="text-neutral-300 font-medium">Wholesale Price ($)</label>
                  <input
                    type="number"
                    step="0.01"
                    value={superAdminPrice}
                    onChange={(e) => setSuperAdminPrice(Number(e.target.value))}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white font-mono focus:outline-none focus:border-emerald-500/50"
                  />
                </div>

                <div className="space-y-1">
                  <label className="text-neutral-300 font-medium">Retail Customer Price ($)</label>
                  <input
                    type="number"
                    step="0.01"
                    value={retailPrice}
                    onChange={(e) => setRetailPrice(Number(e.target.value))}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white font-mono focus:outline-none focus:border-emerald-500/50"
                  />
                </div>
              </div>

              <div className="space-y-1">
                <label className="text-neutral-300 font-medium">Features (one per line)</label>
                <textarea
                  rows={3}
                  value={featuresList}
                  onChange={(e) => setFeaturesList(e.target.value)}
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
                  Publish Package
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
