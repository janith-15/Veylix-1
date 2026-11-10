import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { X, Database, Download, Upload, CheckCircle2, AlertCircle, RefreshCw } from 'lucide-react';

interface BackupRestoreModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const BackupRestoreModal: React.FC<BackupRestoreModalProps> = ({ isOpen, onClose }) => {
  const { exportBackup, importBackup, resetToDefaults, systemSettings } = useVeylixStore();
  const [importJson, setImportJson] = useState('');
  const [resultMsg, setResultMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  if (!isOpen) return null;

  const handleDownloadBackup = () => {
    const jsonStr = exportBackup();
    const blob = new Blob([jsonStr], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `veylix-backup-${systemSettings.installationId}-${new Date().toISOString().slice(0, 10)}.json`;
    a.click();
    URL.revokeObjectURL(url);
    setResultMsg({ type: 'success', text: 'Backup manifest generated and downloaded successfully!' });
  };

  const handleImportSubmit = () => {
    if (!importJson.trim()) {
      setResultMsg({ type: 'error', text: 'Please paste a valid JSON backup manifest.' });
      return;
    }

    const res = importBackup(importJson);
    if (res.success) {
      setResultMsg({ type: 'success', text: res.message });
      setImportJson('');
    } else {
      setResultMsg({ type: 'error', text: res.message });
    }
  };

  const handleFactoryReset = () => {
    if (confirm('Are you sure you want to reset Veylix to default seed data? All custom additions will be restored to default.')) {
      resetToDefaults();
      setResultMsg({ type: 'success', text: 'Installation restored to default state.' });
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="relative w-full max-w-xl bg-neutral-900 border border-neutral-800 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[92vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-800 bg-neutral-950/50">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
              <Database className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-semibold text-white">Database Backup & Provenance</h3>
              <p className="text-xs text-neutral-400">Installation ID: {systemSettings.installationId}</p>
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
          {resultMsg && (
            <div className={`p-3 rounded-xl border flex items-center space-x-2 ${
              resultMsg.type === 'success' 
                ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300' 
                : 'bg-rose-500/10 border-rose-500/30 text-rose-300'
            }`}>
              {resultMsg.type === 'success' ? <CheckCircle2 className="w-4 h-4 shrink-0" /> : <AlertCircle className="w-4 h-4 shrink-0" />}
              <span>{resultMsg.text}</span>
            </div>
          )}

          {/* Export Section */}
          <div className="p-4 bg-neutral-950/70 border border-neutral-800 rounded-xl space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <p className="font-semibold text-white">Export Full Database Manifest</p>
                <p className="text-[11px] text-neutral-400">Includes servers, inbounds, VPN accounts, users, packages, orders & audit stream</p>
              </div>
              <button
                type="button"
                onClick={handleDownloadBackup}
                className="px-4 py-2 bg-emerald-500/20 hover:bg-emerald-500/30 border border-emerald-500/30 text-emerald-300 font-semibold rounded-xl flex items-center space-x-1.5 transition"
              >
                <Download className="w-3.5 h-3.5" />
                <span>Export JSON</span>
              </button>
            </div>
          </div>

          {/* Restore Section */}
          <div className="p-4 bg-neutral-950/70 border border-neutral-800 rounded-xl space-y-3">
            <p className="font-semibold text-white">Restore from Backup Manifest</p>
            <p className="text-[11px] text-neutral-400">Paste backup manifest JSON to verify provenance and import tables</p>
            <textarea
              rows={4}
              value={importJson}
              onChange={(e) => setImportJson(e.target.value)}
              placeholder="Paste { &quot;veylixBackupVersion&quot;: &quot;2.8&quot;, ... } here"
              className="w-full p-2.5 bg-neutral-900 border border-neutral-800 rounded-lg text-neutral-200 font-mono text-[11px] focus:outline-none focus:border-emerald-500/50"
            />
            <div className="flex justify-end">
              <button
                type="button"
                onClick={handleImportSubmit}
                className="px-4 py-2 bg-neutral-800 hover:bg-neutral-700 text-white font-medium rounded-xl flex items-center space-x-1.5 transition"
              >
                <Upload className="w-3.5 h-3.5" />
                <span>Validate & Restore</span>
              </button>
            </div>
          </div>

          {/* Factory Reset */}
          <div className="pt-2 flex items-center justify-between text-neutral-400">
            <span>Need clean test baseline?</span>
            <button
              type="button"
              onClick={handleFactoryReset}
              className="text-neutral-400 hover:text-rose-400 transition flex items-center space-x-1 text-xs"
            >
              <RefreshCw className="w-3 h-3" />
              <span>Reset to Defaults</span>
            </button>
          </div>
        </div>

        {/* Footer */}
        <div className="px-6 py-3.5 bg-neutral-950/60 border-t border-neutral-800 flex justify-end">
          <button
            onClick={onClose}
            className="px-4 py-2 bg-neutral-800 hover:bg-neutral-700 text-white rounded-xl text-xs font-medium"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  );
};
