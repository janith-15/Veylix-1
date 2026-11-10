import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  Settings, 
  Terminal, 
  Database, 
  ShieldCheck, 
  History, 
  Copy, 
  Check, 
  Download, 
  Upload, 
  RefreshCw, 
  Server,
  Globe,
  Clock,
  Key
} from 'lucide-react';

interface SystemSettingsViewProps {
  onOpenInstaller: () => void;
  onOpenBackupModal: () => void;
}

export const SystemSettingsView: React.FC<SystemSettingsViewProps> = ({ 
  onOpenInstaller, 
  onOpenBackupModal 
}) => {
  const { 
    systemSettings, 
    updateSystemSettings, 
    auditLogs, 
    exportBackup 
  } = useVeylixStore();

  const [domain, setDomain] = useState(systemSettings.domain);
  const [appName, setAppName] = useState(systemSettings.appName);
  const [supportTelegram, setSupportTelegram] = useState(systemSettings.telegramSupportHandle);
  const [supportEmail, setSupportEmail] = useState(systemSettings.supportEmail);
  const [savedFeedback, setSavedFeedback] = useState(false);

  const [auditCategory, setAuditCategory] = useState<string>('ALL');

  const handleSaveSettings = (e: React.FormEvent) => {
    e.preventDefault();
    updateSystemSettings({
      domain,
      appName,
      telegramSupportHandle: supportTelegram,
      supportEmail
    });
    setSavedFeedback(true);
    setTimeout(() => setSavedFeedback(false), 2500);
  };

  const filteredLogs = auditCategory === 'ALL'
    ? auditLogs
    : auditLogs.filter(l => l.category === auditCategory);

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-extrabold text-white tracking-tight flex items-center space-x-2">
            <Settings className="w-5 h-5 text-emerald-400" />
            <span>System Node & Independent Identity</span>
          </h1>
          <p className="text-xs text-neutral-400">
            Section 11 & 12 Master Specification • Isolated database provenance & Linux server management
          </p>
        </div>

        <div className="flex items-center space-x-2.5">
          <button
            onClick={onOpenBackupModal}
            className="px-3.5 py-2 bg-neutral-900 hover:bg-neutral-800 border border-neutral-800 rounded-xl text-xs font-medium text-neutral-200 hover:text-white transition flex items-center space-x-1.5"
          >
            <Database className="w-3.5 h-3.5 text-emerald-400" />
            <span>Backup & Restore</span>
          </button>
          <button
            onClick={onOpenInstaller}
            className="px-4 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl text-xs transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
          >
            <Terminal className="w-4 h-4" />
            <span>One-Command Installer</span>
          </button>
        </div>
      </div>

      {/* Independent Installation Identity Card */}
      <div className="p-6 bg-gradient-to-r from-neutral-900 via-neutral-900 to-neutral-950 border border-neutral-800 rounded-2xl shadow-xl space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 pb-4 border-b border-neutral-800">
          <div>
            <span className="text-neutral-400 text-xs font-mono">INSTALLATION IDENTITY (SECTION 11)</span>
            <div className="flex items-center space-x-2 mt-0.5">
              <h2 className="text-lg font-mono font-bold text-emerald-400">{systemSettings.installationId}</h2>
              <span className="px-2 py-0.5 bg-emerald-500/10 text-emerald-400 text-[10px] rounded font-mono font-bold">
                ISOLATED NODE
              </span>
            </div>
          </div>
          <div className="text-xs text-neutral-400 space-y-0.5 font-mono">
            <p>Platform: Go 1.22 + SQLite WAL Mode</p>
            <p>Target: 1 vCPU / 500 MB RAM Optimized</p>
          </div>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-xs">
          <div className="p-3 bg-neutral-950 border border-neutral-800 rounded-xl">
            <span className="text-neutral-400">Database Engine:</span>
            <p className="font-semibold text-white mt-0.5">SQLite Pure-Go</p>
            <p className="text-[11px] text-neutral-500">WAL & Foreign Keys Enforced</p>
          </div>
          <div className="p-3 bg-neutral-950 border border-neutral-800 rounded-xl">
            <span className="text-neutral-400">Reverse Proxy:</span>
            <p className="font-semibold text-white mt-0.5">Caddy Auto-HTTPS</p>
            <p className="text-[11px] text-emerald-400">TLS Cert Active</p>
          </div>
          <div className="p-3 bg-neutral-950 border border-neutral-800 rounded-xl">
            <span className="text-neutral-400">Core Daemon:</span>
            <p className="font-semibold text-white mt-0.5">systemd Service</p>
            <p className="text-[11px] text-neutral-500">Auto-Restart on Crash</p>
          </div>
          <div className="p-3 bg-neutral-950 border border-neutral-800 rounded-xl">
            <span className="text-neutral-400">Pass Hash Algorithm:</span>
            <p className="font-semibold text-white mt-0.5">Argon2id</p>
            <p className="text-[11px] text-neutral-500">64MB / 2 Iterations</p>
          </div>
        </div>
      </div>

      {/* Platform Settings Form */}
      <div className="p-6 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl space-y-4">
        <h3 className="text-sm font-bold text-white">General Platform Settings</h3>

        <form onSubmit={handleSaveSettings} className="space-y-4 text-xs">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div className="space-y-1">
              <label className="text-neutral-300 font-medium">Application Name</label>
              <input
                type="text"
                value={appName}
                onChange={(e) => setAppName(e.target.value)}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
              />
            </div>

            <div className="space-y-1">
              <label className="text-neutral-300 font-medium">Public Domain / FQDN</label>
              <input
                type="text"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white font-mono focus:outline-none focus:border-emerald-500/50"
              />
            </div>

            <div className="space-y-1">
              <label className="text-neutral-300 font-medium">Support Telegram Handle</label>
              <input
                type="text"
                value={supportTelegram}
                onChange={(e) => setSupportTelegram(e.target.value)}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
              />
            </div>

            <div className="space-y-1">
              <label className="text-neutral-300 font-medium">Support Operations Email</label>
              <input
                type="email"
                value={supportEmail}
                onChange={(e) => setSupportEmail(e.target.value)}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
              />
            </div>
          </div>

          <div className="pt-2 flex items-center justify-end space-x-3">
            {savedFeedback && (
              <span className="text-xs text-emerald-400 flex items-center space-x-1">
                <Check className="w-3.5 h-3.5" />
                <span>Settings saved to SQLite!</span>
              </span>
            )}
            <button
              type="submit"
              className="px-5 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-semibold rounded-xl text-xs transition"
            >
              Save Configuration
            </button>
          </div>
        </form>
      </div>

      {/* Security Audit Log Stream Viewer */}
      <div className="p-6 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
          <div className="flex items-center space-x-2">
            <History className="w-4 h-4 text-emerald-400" />
            <h3 className="text-sm font-bold text-white">Cryptographic & Security Audit Log Stream</h3>
          </div>

          {/* Category Filter */}
          <div className="flex items-center space-x-2 text-xs">
            <span className="text-neutral-400">Category:</span>
            <select
              value={auditCategory}
              onChange={(e) => setAuditCategory(e.target.value)}
              className="px-2.5 py-1 bg-neutral-950 border border-neutral-800 rounded-lg text-neutral-200"
            >
              <option value="ALL">All Categories</option>
              <option value="AUTH">Authentication</option>
              <option value="SERVER">3X-UI Servers</option>
              <option value="VPN">VPN Provisioning</option>
              <option value="QUOTA">Quotas</option>
              <option value="ORDER">Orders & Payments</option>
              <option value="SYSTEM">System</option>
            </select>
          </div>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-neutral-800 text-neutral-400 bg-neutral-950/60">
                <th className="py-2.5 px-3 font-medium">Timestamp</th>
                <th className="py-2.5 px-3 font-medium">Action Event</th>
                <th className="py-2.5 px-3 font-medium">Actor</th>
                <th className="py-2.5 px-3 font-medium">Details & Operation Notes</th>
                <th className="py-2.5 px-3 font-medium">Source IP</th>
                <th className="py-2.5 px-3 font-medium text-right">Severity</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-800/60 font-mono text-[11px]">
              {filteredLogs.map(log => (
                <tr key={log.id} className="hover:bg-neutral-800/30 transition">
                  <td className="py-2.5 px-3 text-neutral-400">
                    {new Date(log.timestamp).toLocaleString()}
                  </td>
                  <td className="py-2.5 px-3 font-bold text-emerald-400">
                    {log.action}
                  </td>
                  <td className="py-2.5 px-3 text-neutral-200 font-sans">
                    {log.actorName} ({log.actorRole})
                  </td>
                  <td className="py-2.5 px-3 text-neutral-300 font-sans max-w-xs truncate">
                    {log.details}
                  </td>
                  <td className="py-2.5 px-3 text-neutral-500">
                    {log.ipAddress}
                  </td>
                  <td className="py-2.5 px-3 text-right">
                    <span className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                      log.severity === 'CRITICAL' ? 'bg-rose-500/20 text-rose-400' :
                      log.severity === 'WARNING' ? 'bg-amber-500/20 text-amber-400' :
                      'bg-neutral-800 text-neutral-400'
                    }`}>
                      {log.severity}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
