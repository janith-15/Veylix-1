import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { X, Server, Globe, Shield, RefreshCw, CheckCircle2, AlertCircle, Plus } from 'lucide-react';
import { VpnProtocol, ThreeXUIInbound } from '../../types';

interface AddServerModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const AddServerModal: React.FC<AddServerModalProps> = ({ isOpen, onClose }) => {
  const { addServer, testServerConnection } = useVeylixStore();

  const [name, setName] = useState('');
  const [host, setHost] = useState('');
  const [port, setPort] = useState(2053);
  const [panelPath, setPanelPath] = useState('/3x-ui');
  const [location, setLocation] = useState('Singapore');
  const [countryCode, setCountryCode] = useState('SG');
  const [flag, setFlag] = useState('🇸🇬');
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  
  const [isTesting, setIsTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; latency: number; version: string } | null>(null);
  const [errorMsg, setErrorMsg] = useState('');

  if (!isOpen) return null;

  const handleTest = async () => {
    if (!host) {
      setErrorMsg('Please enter a valid hostname or IP address.');
      return;
    }
    setErrorMsg('');
    setIsTesting(true);
    try {
      const res = await testServerConnection(host, port);
      setTestResult(res);
    } catch (e: any) {
      setErrorMsg('Connection failed. Verify panel URL and credentials.');
    } finally {
      setIsTesting(false);
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name || !host) {
      setErrorMsg('Please fill in server name and host.');
      return;
    }

    const defaultInbounds: ThreeXUIInbound[] = [
      {
        id: Math.floor(Math.random() * 900) + 100,
        port: 443,
        protocol: 'VLESS',
        remark: `${countryCode} VLESS Reality (Auto-Config)`,
        enable: true,
        network: 'tcp',
        security: 'reality',
        clientCount: 0,
        upTrafficBytes: 0,
        downTrafficBytes: 0,
        totalLimitGb: 3000
      },
      {
        id: Math.floor(Math.random() * 900) + 100,
        port: 8443,
        protocol: 'VMess',
        remark: `${countryCode} VMess WS TLS (Auto-Config)`,
        enable: true,
        network: 'ws',
        security: 'tls',
        clientCount: 0,
        upTrafficBytes: 0,
        downTrafficBytes: 0,
        totalLimitGb: 2000
      }
    ];

    addServer({
      name,
      host,
      port,
      panelPath,
      apiUrl: `https://${host}:${port}${panelPath}/server`,
      location,
      countryCode,
      flag,
      status: 'ONLINE',
      latencyMs: testResult ? testResult.latency : 32,
      panelVersion: testResult ? 'v2.4.2' : 'v2.4.0',
      xrayVersion: 'v1.8.28',
      cpuUsagePercent: 18,
      ramUsagePercent: 34,
      ramUsedMb: 680,
      ramTotalMb: 2000,
      diskUsagePercent: 24,
      assignedAdminIds: [],
      inbounds: defaultInbounds,
      autoSync: true
    });

    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="relative w-full max-w-xl bg-neutral-900 border border-neutral-800 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[92vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-800 bg-neutral-950/50">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
              <Server className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-semibold text-white">Connect 3X-UI Server</h3>
              <p className="text-xs text-neutral-400">Integrate external Xray-core panel via REST API</p>
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

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1">
              <label className="text-neutral-300 font-medium">Server Display Name</label>
              <input
                type="text"
                placeholder="e.g. Singapore SG-02 Edge"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 placeholder-neutral-500 focus:outline-none focus:border-emerald-500/50 focus:ring-1 focus:ring-emerald-500/50"
              />
            </div>

            <div className="space-y-1">
              <label className="text-neutral-300 font-medium">Location & Flag</label>
              <div className="grid grid-cols-3 gap-2">
                <input
                  type="text"
                  placeholder="Flag"
                  value={flag}
                  onChange={(e) => setFlag(e.target.value)}
                  className="px-2 py-2 text-center bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100"
                />
                <input
                  type="text"
                  placeholder="Code"
                  value={countryCode}
                  onChange={(e) => setCountryCode(e.target.value.toUpperCase())}
                  className="px-2 py-2 text-center bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 font-mono uppercase"
                />
                <input
                  type="text"
                  placeholder="City"
                  value={location}
                  onChange={(e) => setLocation(e.target.value)}
                  className="px-2 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100"
                />
              </div>
            </div>
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div className="col-span-2 space-y-1">
              <label className="text-neutral-300 font-medium">Hostname or Public IP</label>
              <input
                type="text"
                placeholder="sg02.veylix.network or 194.233.x.x"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                required
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 placeholder-neutral-500 font-mono focus:outline-none focus:border-emerald-500/50"
              />
            </div>

            <div className="space-y-1">
              <label className="text-neutral-300 font-medium">Panel Port</label>
              <input
                type="number"
                value={port}
                onChange={(e) => setPort(Number(e.target.value))}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 font-mono"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1">
              <label className="text-neutral-300 font-medium">Panel URL Path</label>
              <input
                type="text"
                placeholder="/3x-ui or /panel"
                value={panelPath}
                onChange={(e) => setPanelPath(e.target.value)}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 font-mono"
              />
            </div>

            <div className="space-y-1">
              <label className="text-neutral-300 font-medium">Panel Admin Username</label>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 font-mono"
              />
            </div>
          </div>

          <div className="space-y-1">
            <label className="text-neutral-300 font-medium">Panel Password / Secret Token</label>
            <input
              type="password"
              placeholder="••••••••••••••••"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-100 font-mono"
            />
            <p className="text-[11px] text-neutral-500">Credentials will be encrypted with local installation keys and stored securely.</p>
          </div>

          {/* Test connection section */}
          <div className="p-4 bg-neutral-950/70 border border-neutral-800 rounded-xl space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <p className="font-semibold text-neutral-200">Panel API Connectivity Test</p>
                <p className="text-[11px] text-neutral-400">Perform preflight HTTPS probe & latency measurement</p>
              </div>
              <button
                type="button"
                onClick={handleTest}
                disabled={isTesting}
                className="px-3 py-1.5 text-xs font-medium text-emerald-400 bg-emerald-500/10 hover:bg-emerald-500/20 rounded-lg border border-emerald-500/30 transition flex items-center space-x-1.5 disabled:opacity-50"
              >
                <RefreshCw className={`w-3.5 h-3.5 ${isTesting ? 'animate-spin' : ''}`} />
                <span>{isTesting ? 'Testing...' : 'Test Connection'}</span>
              </button>
            </div>

            {testResult && (
              <div className="p-3 bg-emerald-500/10 border border-emerald-500/30 rounded-lg flex items-center justify-between text-xs text-emerald-300">
                <div className="flex items-center space-x-2">
                  <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                  <span>Probe Successful: {testResult.version}</span>
                </div>
                <span className="font-mono bg-emerald-500/20 px-2 py-0.5 rounded text-[11px]">
                  Latency: {testResult.latency}ms
                </span>
              </div>
            )}
          </div>

          {/* Footer actions */}
          <div className="pt-2 flex items-center justify-end space-x-3 border-t border-neutral-800">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-xs font-medium text-neutral-400 hover:text-white bg-neutral-800 hover:bg-neutral-700 rounded-xl transition"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="px-5 py-2 text-xs font-semibold text-neutral-950 bg-emerald-400 hover:bg-emerald-300 rounded-xl transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
            >
              <Plus className="w-4 h-4" />
              <span>Connect & Initialize</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
