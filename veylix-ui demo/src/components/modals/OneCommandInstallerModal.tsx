import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { X, Terminal, Copy, Check, ShieldCheck, Server, Cpu, HardDrive } from 'lucide-react';

interface OneCommandInstallerModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const OneCommandInstallerModal: React.FC<OneCommandInstallerModalProps> = ({ isOpen, onClose }) => {
  const { systemSettings } = useVeylixStore();
  const [domain, setDomain] = useState(systemSettings.domain || 'vpn.mydomain.com');
  const [email, setEmail] = useState('admin@mydomain.com');
  const [copied, setCopied] = useState(false);

  if (!isOpen) return null;

  const generatedCommand = `curl -sSL https://get.veylix.io/install.sh | sudo bash -s -- --domain ${domain} --email ${email} --port 443 --arch auto`;

  const handleCopy = () => {
    navigator.clipboard.writeText(generatedCommand);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="relative w-full max-w-2xl bg-neutral-900 border border-neutral-800 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[92vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-800 bg-neutral-950/50">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
              <Terminal className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-semibold text-white">One-Command Self-Hosted Linux Installer</h3>
              <p className="text-xs text-neutral-400">Section 12 Master Specification • Debian, Ubuntu & Alpine</p>
            </div>
          </div>
          <button 
            onClick={onClose}
            className="p-1.5 text-neutral-400 hover:text-white rounded-lg hover:bg-neutral-800 transition"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6 overflow-y-auto space-y-5 text-xs text-neutral-300">
          {/* Target Architecture & Constraints */}
          <div className="grid grid-cols-3 gap-3">
            <div className="p-3 bg-neutral-950 border border-neutral-800 rounded-xl space-y-1">
              <div className="flex items-center space-x-1.5 text-neutral-400">
                <Cpu className="w-3.5 h-3.5 text-emerald-400" />
                <span>Target Specs</span>
              </div>
              <p className="font-semibold text-white">1 vCPU / 500 MB RAM</p>
              <p className="text-[11px] text-neutral-500">Ultra-lightweight Go + SQLite WAL</p>
            </div>

            <div className="p-3 bg-neutral-950 border border-neutral-800 rounded-xl space-y-1">
              <div className="flex items-center space-x-1.5 text-neutral-400">
                <Server className="w-3.5 h-3.5 text-cyan-400" />
                <span>Reverse Proxy</span>
              </div>
              <p className="font-semibold text-white">Caddy Auto HTTPS</p>
              <p className="text-[11px] text-neutral-500">Automatic Let's Encrypt TLS renewal</p>
            </div>

            <div className="p-3 bg-neutral-950 border border-neutral-800 rounded-xl space-y-1">
              <div className="flex items-center space-x-1.5 text-neutral-400">
                <HardDrive className="w-3.5 h-3.5 text-violet-400" />
                <span>Database</span>
              </div>
              <p className="font-semibold text-white">Independent SQLite</p>
              <p className="text-[11px] text-neutral-500">Zero shared cloud database locks</p>
            </div>
          </div>

          {/* Configurator */}
          <div className="p-4 bg-neutral-950/70 border border-neutral-800 rounded-xl space-y-3">
            <p className="font-semibold text-white text-xs">Configure Production Parameters</p>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1">
                <label className="text-neutral-400">Server Domain / FQDN</label>
                <input
                  type="text"
                  value={domain}
                  onChange={(e) => setDomain(e.target.value)}
                  placeholder="vpn.example.com"
                  className="w-full px-3 py-2 bg-neutral-900 border border-neutral-800 rounded-lg text-white font-mono text-xs focus:outline-none focus:border-emerald-500/50"
                />
              </div>
              <div className="space-y-1">
                <label className="text-neutral-400">Admin Email (for SSL renewal)</label>
                <input
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="admin@example.com"
                  className="w-full px-3 py-2 bg-neutral-900 border border-neutral-800 rounded-lg text-white font-mono text-xs focus:outline-none focus:border-emerald-500/50"
                />
              </div>
            </div>
          </div>

          {/* Command box */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="font-semibold text-neutral-200">Terminal Shell Command (Run as root):</span>
              <button
                onClick={handleCopy}
                className="flex items-center space-x-1 text-emerald-400 hover:text-emerald-300 font-medium"
              >
                {copied ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
                <span>{copied ? 'Copied Command!' : 'Copy to Clipboard'}</span>
              </button>
            </div>
            <div className="p-3.5 bg-neutral-950 border border-emerald-500/30 rounded-xl font-mono text-emerald-400 text-xs break-all select-all leading-relaxed shadow-inner">
              {generatedCommand}
            </div>
          </div>

          {/* Automated execution checklist */}
          <div className="space-y-2">
            <p className="text-neutral-400 font-medium">What the script automatically executes:</p>
            <ul className="space-y-1.5 text-[11px] text-neutral-400">
              <li className="flex items-center space-x-2">
                <ShieldCheck className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                <span>1. Validates Linux OS distro, creates isolated service user <code>veylix</code> and permissions.</span>
              </li>
              <li className="flex items-center space-x-2">
                <ShieldCheck className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                <span>2. Generates unique instance cryptographic seed, JWT secret, and database salt.</span>
              </li>
              <li className="flex items-center space-x-2">
                <ShieldCheck className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                <span>3. Compiles/downloads static Veylix single-binary with embedded React frontend.</span>
              </li>
              <li className="flex items-center space-x-2">
                <ShieldCheck className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                <span>4. Deploys Caddy reverse proxy with HTTP-01 Let's Encrypt SSL certificate automation.</span>
              </li>
              <li className="flex items-center space-x-2">
                <ShieldCheck className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                <span>5. Registers and enables <code>systemctl enable --now veylix.service</code>.</span>
              </li>
            </ul>
          </div>
        </div>

        {/* Footer */}
        <div className="px-6 py-4 bg-neutral-950/60 border-t border-neutral-800 flex items-center justify-between">
          <span className="text-neutral-500 text-[11px]">Installation ID: {systemSettings.installationId}</span>
          <button
            onClick={handleCopy}
            className="px-5 py-2 text-xs font-semibold text-neutral-950 bg-emerald-400 hover:bg-emerald-300 rounded-xl transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
          >
            {copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
            <span>{copied ? 'Command Copied' : 'Copy Installer Command'}</span>
          </button>
        </div>
      </div>
    </div>
  );
};
