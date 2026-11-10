import React, { useState } from 'react';
import { X, Smartphone, Monitor, Apple, Terminal, Check, Copy } from 'lucide-react';

interface ClientSetupGuideModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const ClientSetupGuideModal: React.FC<ClientSetupGuideModalProps> = ({ isOpen, onClose }) => {
  const [activeTab, setActiveTab] = useState<'android' | 'windows' | 'ios' | 'mac'>('android');
  const [copiedLink, setCopiedLink] = useState('');

  if (!isOpen) return null;

  const handleCopy = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedLink(id);
    setTimeout(() => setCopiedLink(''), 2000);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="relative w-full max-w-2xl bg-neutral-900 border border-neutral-800 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[92vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-800 bg-neutral-950/50">
          <div>
            <h3 className="text-base font-semibold text-white">VPN Client Setup & Connection Guides</h3>
            <p className="text-xs text-neutral-400">Step-by-step connection instructions for VLESS, VMess, Trojan, and Shadowsocks</p>
          </div>
          <button 
            onClick={onClose}
            className="p-1.5 text-neutral-400 hover:text-white rounded-lg hover:bg-neutral-800 transition"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Platform Tabs */}
        <div className="flex border-b border-neutral-800 bg-neutral-950/30 px-6 gap-2">
          {[
            { id: 'android', label: 'Android (v2rayNG)', icon: Smartphone },
            { id: 'windows', label: 'Windows (v2rayN)', icon: Monitor },
            { id: 'ios', label: 'iOS (Streisand / FoXray)', icon: Apple },
            { id: 'mac', label: 'macOS / Linux (Clash / Sing-box)', icon: Terminal }
          ].map(tab => {
            const Icon = tab.icon;
            const isSelected = activeTab === tab.id;
            return (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id as any)}
                className={`py-3 px-3 text-xs font-medium border-b-2 transition flex items-center space-x-2 ${
                  isSelected 
                    ? 'border-emerald-400 text-emerald-400 font-semibold' 
                    : 'border-transparent text-neutral-400 hover:text-neutral-200'
                }`}
              >
                <Icon className="w-3.5 h-3.5" />
                <span>{tab.label}</span>
              </button>
            );
          })}
        </div>

        {/* Tab Body */}
        <div className="p-6 overflow-y-auto space-y-4 text-xs text-neutral-300">
          {activeTab === 'android' && (
            <div className="space-y-4">
              <div className="p-3.5 bg-neutral-950 border border-neutral-800 rounded-xl flex items-center justify-between">
                <div>
                  <p className="font-semibold text-white">Recommended App: v2rayNG</p>
                  <p className="text-[11px] text-neutral-400">Free, open-source, full Xray-core with VLESS Reality support</p>
                </div>
                <a
                  href="https://play.google.com/store/apps/details?id=com.v2ray.ang"
                  target="_blank"
                  rel="noreferrer"
                  className="px-3 py-1.5 bg-emerald-500/20 text-emerald-400 rounded-lg text-xs font-semibold hover:bg-emerald-500/30 border border-emerald-500/30"
                >
                  Google Play
                </a>
              </div>

              <ol className="space-y-3 list-decimal list-inside text-neutral-300 leading-relaxed">
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 1:</strong> Install <strong>v2rayNG</strong> from Google Play Store or GitHub releases.
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 2:</strong> In your Veylix dashboard, click <strong>"Show QR Code"</strong> on your active VPN key.
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 3:</strong> Open v2rayNG, tap the <strong>"+"</strong> button at the top right, and choose <strong>"Scan QR Code"</strong> (or <em>"Import config from Clipboard"</em> if you copied the link).
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 4:</strong> Tap on the imported server, then tap the floating <strong>"V" (Connect)</strong> button at bottom right. Enjoy uncensored high-speed internet!
                </li>
              </ol>
            </div>
          )}

          {activeTab === 'windows' && (
            <div className="space-y-4">
              <div className="p-3.5 bg-neutral-950 border border-neutral-800 rounded-xl flex items-center justify-between">
                <div>
                  <p className="font-semibold text-white">Recommended Client: v2rayN / Nekoray</p>
                  <p className="text-[11px] text-neutral-400">System proxy & TUN mode for all Windows apps and PC gaming</p>
                </div>
                <a
                  href="https://github.com/2dust/v2rayN/releases"
                  target="_blank"
                  rel="noreferrer"
                  className="px-3 py-1.5 bg-emerald-500/20 text-emerald-400 rounded-lg text-xs font-semibold hover:bg-emerald-500/30 border border-emerald-500/30"
                >
                  Download v2rayN
                </a>
              </div>

              <ol className="space-y-3 list-decimal list-inside text-neutral-300 leading-relaxed">
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 1:</strong> Download and extract <strong>v2rayN-with-Core.zip</strong> from GitHub.
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 2:</strong> Click <strong>"Copy Connection Link"</strong> on your VPN in Veylix.
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 3:</strong> Open v2rayN window and press <kbd className="px-1.5 py-0.5 bg-neutral-800 rounded font-mono text-white">Ctrl + V</kbd>. The configuration will immediately import.
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 4:</strong> Select the server, press Enter, and set bottom bar to <strong>"Set System Proxy"</strong> (or enable <em>"Enable TUN Mode"</em> for all games).
                </li>
              </ol>
            </div>
          )}

          {activeTab === 'ios' && (
            <div className="space-y-4">
              <div className="p-3.5 bg-neutral-950 border border-neutral-800 rounded-xl flex items-center justify-between">
                <div>
                  <p className="font-semibold text-white">Recommended App: Streisand or FoXray</p>
                  <p className="text-[11px] text-neutral-400">Available on Apple App Store with native VLESS Reality support</p>
                </div>
                <a
                  href="https://apps.apple.com/app/streisand/id6450534064"
                  target="_blank"
                  rel="noreferrer"
                  className="px-3 py-1.5 bg-emerald-500/20 text-emerald-400 rounded-lg text-xs font-semibold hover:bg-emerald-500/30 border border-emerald-500/30"
                >
                  App Store
                </a>
              </div>

              <ol className="space-y-3 list-decimal list-inside text-neutral-300 leading-relaxed">
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 1:</strong> Install <strong>Streisand</strong> or <strong>FoXray</strong> from the iOS App Store.
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 2:</strong> In Streisand, tap <strong>"+"</strong> at the top right, then tap <strong>"Scan QR Code"</strong>.
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 3:</strong> Point your iPhone camera at the QR code displayed on your Veylix dashboard.
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 4:</strong> Tap <strong>"Connect"</strong> and allow iOS VPN profile prompt when prompted.
                </li>
              </ol>
            </div>
          )}

          {activeTab === 'mac' && (
            <div className="space-y-4">
              <div className="p-3.5 bg-neutral-950 border border-neutral-800 rounded-xl flex items-center justify-between">
                <div>
                  <p className="font-semibold text-white">Recommended: Clash Verge / Sing-box</p>
                  <p className="text-[11px] text-neutral-400">High performance Apple Silicon native core</p>
                </div>
                <a
                  href="https://github.com/clash-verge-rev/clash-verge-rev"
                  target="_blank"
                  rel="noreferrer"
                  className="px-3 py-1.5 bg-emerald-500/20 text-emerald-400 rounded-lg text-xs font-semibold hover:bg-emerald-500/30 border border-emerald-500/30"
                >
                  GitHub
                </a>
              </div>

              <ol className="space-y-3 list-decimal list-inside text-neutral-300 leading-relaxed">
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 1:</strong> Download <strong>Clash Verge Rev</strong> (.dmg for Apple Silicon or Intel).
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 2:</strong> In Veylix, click <strong>"Copy Connection Link"</strong>.
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 3:</strong> In Clash Verge, navigate to Profiles &gt; New &gt; Import from Clipboard.
                </li>
                <li className="p-2.5 bg-neutral-950/60 rounded-lg border border-neutral-800/80">
                  <strong className="text-white">Step 4:</strong> Toggle <strong>"System Proxy"</strong> to ON.
                </li>
              </ol>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="px-6 py-3.5 bg-neutral-950/60 border-t border-neutral-800 flex items-center justify-between">
          <span className="text-neutral-500 text-[11px]">Need assistance? Contact support directly in the Live Chat tab.</span>
          <button
            onClick={onClose}
            className="px-4 py-2 bg-neutral-800 hover:bg-neutral-700 text-white rounded-xl text-xs font-medium"
          >
            Got It
          </button>
        </div>
      </div>
    </div>
  );
};
