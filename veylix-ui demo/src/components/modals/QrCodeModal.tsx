import React, { useEffect, useRef, useState } from 'react';
import QRCode from 'qrcode';
import { VpnAccount } from '../../types';
import { X, Copy, Check, Download, ShieldCheck, Zap, Server, Calendar, HardDrive } from 'lucide-react';

interface QrCodeModalProps {
  vpn: VpnAccount | null;
  onClose: () => void;
  onOpenGuide?: () => void;
}

export const QrCodeModal: React.FC<QrCodeModalProps> = ({ vpn, onClose, onOpenGuide }) => {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const [copied, setCopied] = useState(false);
  const [qrGenerated, setQrGenerated] = useState(false);

  useEffect(() => {
    if (!vpn || !canvasRef.current) return;

    QRCode.toCanvas(
      canvasRef.current,
      vpn.connectionUri,
      {
        width: 256,
        margin: 2,
        color: {
          dark: '#000000',
          light: '#ffffff'
        },
        errorCorrectionLevel: 'M'
      },
      (err) => {
        if (err) {
          console.error('QR Code render error:', err);
        } else {
          setQrGenerated(true);
        }
      }
    );
  }, [vpn]);

  if (!vpn) return null;

  const handleCopy = () => {
    navigator.clipboard.writeText(vpn.connectionUri);
    setCopied(true);
    setTimeout(() => setCopied(false), 2200);
  };

  const handleDownloadQr = () => {
    if (!canvasRef.current) return;
    const url = canvasRef.current.toDataURL('image/png');
    const a = document.createElement('a');
    a.href = url;
    a.download = `veylix-${vpn.name}-qr.png`;
    a.click();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="relative w-full max-w-lg bg-neutral-900 border border-neutral-800 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-800/80 bg-neutral-950/40">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
              <Zap className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-semibold text-white tracking-tight">{vpn.name}</h3>
              <p className="text-xs text-neutral-400 flex items-center space-x-1.5">
                <span className="inline-block w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
                <span>{vpn.protocol} {vpn.security !== 'none' ? `• ${vpn.security.toUpperCase()}` : ''}</span>
                <span>• {vpn.serverHost}:{vpn.port}</span>
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

        {/* Body */}
        <div className="p-6 overflow-y-auto space-y-6">
          {/* Local QR Container */}
          <div className="flex flex-col items-center">
            <div className="p-4 bg-white rounded-2xl shadow-lg border-4 border-emerald-500/20 relative group">
              <canvas ref={canvasRef} className="rounded-lg w-52 h-52"></canvas>
              <div className="absolute inset-0 bg-neutral-950/20 opacity-0 group-hover:opacity-100 transition flex items-center justify-center rounded-xl pointer-events-none">
                <span className="text-xs font-medium text-white bg-black/80 px-2.5 py-1 rounded-md backdrop-blur">
                  100% Client-Side Render
                </span>
              </div>
            </div>

            <p className="mt-3 text-xs text-neutral-400 text-center flex items-center space-x-1">
              <ShieldCheck className="w-3.5 h-3.5 text-emerald-400" />
              <span>Zero-leak guarantee: QR generated in-browser without 3rd-party APIs</span>
            </p>

            <div className="mt-3 flex items-center space-x-2">
              <button
                onClick={handleDownloadQr}
                className="inline-flex items-center space-x-1.5 px-3 py-1.5 text-xs font-medium text-neutral-300 bg-neutral-800 hover:bg-neutral-700 rounded-lg border border-neutral-700/60 transition"
              >
                <Download className="w-3.5 h-3.5 text-neutral-400" />
                <span>Save PNG</span>
              </button>
              {onOpenGuide && (
                <button
                  onClick={onOpenGuide}
                  className="inline-flex items-center space-x-1.5 px-3 py-1.5 text-xs font-medium text-emerald-400 bg-emerald-500/10 hover:bg-emerald-500/20 rounded-lg border border-emerald-500/30 transition"
                >
                  <span>App Setup Guide</span>
                </button>
              )}
            </div>
          </div>

          {/* Connection URI string box */}
          <div className="space-y-1.5">
            <div className="flex items-center justify-between text-xs text-neutral-400 font-medium">
              <span>Connection URI (Standard Format)</span>
              <button
                onClick={handleCopy}
                className="flex items-center space-x-1 text-emerald-400 hover:text-emerald-300 transition"
              >
                {copied ? (
                  <>
                    <Check className="w-3.5 h-3.5" />
                    <span>Copied!</span>
                  </>
                ) : (
                  <>
                    <Copy className="w-3.5 h-3.5" />
                    <span>Copy String</span>
                  </>
                )}
              </button>
            </div>
            <div className="p-3 bg-neutral-950 border border-neutral-800 rounded-xl font-mono text-xs text-neutral-300 break-all select-all max-h-24 overflow-y-auto leading-relaxed">
              {vpn.connectionUri}
            </div>
          </div>

          {/* Quick Specs Grid */}
          <div className="grid grid-cols-2 gap-3 text-xs">
            <div className="p-3 bg-neutral-950/60 border border-neutral-800/80 rounded-xl space-y-1">
              <div className="flex items-center space-x-1.5 text-neutral-400">
                <Server className="w-3.5 h-3.5" />
                <span>Node & Protocol</span>
              </div>
              <p className="font-semibold text-neutral-200">{vpn.serverName}</p>
              <p className="text-[11px] text-neutral-400">Port {vpn.port} • {vpn.network.toUpperCase()}</p>
            </div>

            <div className="p-3 bg-neutral-950/60 border border-neutral-800/80 rounded-xl space-y-1">
              <div className="flex items-center space-x-1.5 text-neutral-400">
                <HardDrive className="w-3.5 h-3.5" />
                <span>Traffic Allocation</span>
              </div>
              <p className="font-semibold text-neutral-200">
                {vpn.usedTrafficGb.toFixed(1)} / {vpn.totalTrafficGb} GB
              </p>
              <div className="w-full bg-neutral-800 h-1.5 rounded-full overflow-hidden mt-1">
                <div 
                  className="bg-emerald-500 h-full rounded-full transition-all"
                  style={{ width: `${Math.min(100, (vpn.usedTrafficGb / vpn.totalTrafficGb) * 100)}%` }}
                ></div>
              </div>
            </div>

            <div className="col-span-2 p-3 bg-neutral-950/60 border border-neutral-800/80 rounded-xl flex items-center justify-between">
              <div className="flex items-center space-x-2">
                <Calendar className="w-4 h-4 text-neutral-400" />
                <span className="text-neutral-400">Valid Until:</span>
                <span className="font-medium text-neutral-200">
                  {new Date(vpn.expiryDate).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
                </span>
              </div>
              <span className={`px-2 py-0.5 rounded-full text-[11px] font-medium ${
                vpn.status === 'ACTIVE' ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30' : 'bg-amber-500/10 text-amber-400'
              }`}>
                {vpn.status}
              </span>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="px-6 py-3.5 bg-neutral-950/60 border-t border-neutral-800 flex items-center justify-between">
          <span className="text-[11px] text-neutral-500 font-mono">ID: {vpn.uuid.substring(0, 18)}...</span>
          <button
            onClick={handleCopy}
            className="px-4 py-2 text-xs font-semibold text-neutral-950 bg-emerald-400 hover:bg-emerald-300 rounded-xl transition flex items-center space-x-1.5"
          >
            {copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
            <span>{copied ? 'Copied to Clipboard' : 'Copy Connection Link'}</span>
          </button>
        </div>
      </div>
    </div>
  );
};
