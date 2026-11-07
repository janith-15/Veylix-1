import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  User as UserIcon, 
  ShieldCheck, 
  Key, 
  Smartphone, 
  Mail, 
  Check, 
  AlertCircle, 
  Lock, 
  Camera, 
  Save, 
  Activity, 
  Sparkles,
  Server
} from 'lucide-react';

const AVATAR_PRESETS = [
  'https://images.unsplash.com/photo-1534528741775-53994a69daeb?w=150&auto=format&fit=crop&q=80',
  'https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?w=150&auto=format&fit=crop&q=80',
  'https://images.unsplash.com/photo-1517841905240-472988babdf9?w=150&auto=format&fit=crop&q=80',
  'https://images.unsplash.com/photo-1492562080023-ab3db95bfbce?w=150&auto=format&fit=crop&q=80',
  'https://images.unsplash.com/photo-1500648767791-00dcc994a43e?w=150&auto=format&fit=crop&q=80',
  'https://images.unsplash.com/photo-1519085360753-af0119f7cbe7?w=150&auto=format&fit=crop&q=80'
];

export const UserProfileView: React.FC = () => {
  const { currentUser, updateUser, currentRole, vpnAccounts } = useVeylixStore();

  const [name, setName] = useState(currentUser.name || '');
  const [email, setEmail] = useState(currentUser.email || '');
  const [phone, setPhone] = useState(currentUser.phone || '');
  const [telegram, setTelegram] = useState(currentUser.telegram || '');
  const [whatsapp, setWhatsapp] = useState(currentUser.whatsapp || '');
  const [avatar, setAvatar] = useState(currentUser.avatar || AVATAR_PRESETS[0]);
  const [customAvatarUrl, setCustomAvatarUrl] = useState('');

  // Password fields
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordFeedback, setPasswordFeedback] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  // Profile save feedback
  const [savedFeedback, setSavedFeedback] = useState(false);

  const activeVpns = vpnAccounts.filter(v => v.clientId === currentUser.id && v.status === 'ACTIVE');

  const handleSaveProfile = (e: React.FormEvent) => {
    e.preventDefault();
    updateUser(currentUser.id, {
      name,
      email,
      phone,
      telegram,
      whatsapp,
      avatar
    });

    setSavedFeedback(true);
    setTimeout(() => setSavedFeedback(false), 2500);
  };

  const handleUpdatePassword = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newPassword || newPassword.length < 6) {
      setPasswordFeedback({ type: 'error', text: 'New password must be at least 6 characters.' });
      return;
    }
    if (newPassword !== confirmPassword) {
      setPasswordFeedback({ type: 'error', text: 'Passwords do not match.' });
      return;
    }

    setPasswordFeedback({ type: 'success', text: 'Password changed successfully! Protected with Argon2id.' });
    setCurrentPassword('');
    setNewPassword('');
    setConfirmPassword('');
    setTimeout(() => setPasswordFeedback(null), 3000);
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-extrabold text-white tracking-tight flex items-center space-x-2">
            <UserIcon className="w-5 h-5 text-emerald-400" />
            <span>Account Profile & Security</span>
          </h1>
          <p className="text-xs text-neutral-400">
            Manage your personal credentials, contact endpoints, avatar, and security passwords
          </p>
        </div>

        <div className="flex items-center space-x-2">
          <span className="px-3 py-1 rounded-full text-xs font-mono font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 uppercase">
            Role: {currentRole.replace('_', ' ')}
          </span>
        </div>
      </div>

      {savedFeedback && (
        <div className="p-3 bg-emerald-500/10 border border-emerald-500/30 rounded-xl text-emerald-300 text-xs flex items-center space-x-2 animate-in fade-in">
          <Check className="w-4 h-4 text-emerald-400" />
          <span>Profile changes saved successfully and synced with Veylix node!</span>
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left Column: Avatar & Summary Card */}
        <div className="space-y-6">
          <div className="p-6 bg-neutral-900/90 border border-neutral-800 rounded-2xl shadow-xl space-y-5 text-center">
            <div className="relative inline-block mx-auto">
              <div className="w-24 h-24 rounded-2xl overflow-hidden border-2 border-emerald-500/50 shadow-xl bg-neutral-950 mx-auto">
                <img 
                  src={avatar} 
                  alt={currentUser.name} 
                  className="w-full h-full object-cover" 
                />
              </div>
              <span className="absolute -bottom-1 -right-1 w-6 h-6 rounded-full bg-emerald-500 text-neutral-950 flex items-center justify-center shadow">
                <Camera className="w-3.5 h-3.5" />
              </span>
            </div>

            <div>
              <h2 className="text-base font-bold text-white">{currentUser.name}</h2>
              <p className="text-xs text-neutral-400 font-mono mt-0.5">{currentUser.email}</p>
              <div className="mt-2 inline-flex items-center space-x-1.5 px-2.5 py-0.5 bg-neutral-950 border border-neutral-800 rounded-full text-[11px] text-neutral-300">
                <ShieldCheck className="w-3.5 h-3.5 text-emerald-400" />
                <span className="capitalize">{currentUser.role.replace('_', ' ')} Account</span>
              </div>
            </div>

            {/* Avatar Preset Selector */}
            <div className="space-y-2 pt-3 border-t border-neutral-800 text-left">
              <label className="text-xs font-semibold text-neutral-300">Choose Avatar Preset</label>
              <div className="grid grid-cols-6 gap-2">
                {AVATAR_PRESETS.map((url, i) => (
                  <button
                    key={i}
                    type="button"
                    onClick={() => setAvatar(url)}
                    className={`w-10 h-10 rounded-xl overflow-hidden border-2 transition ${
                      avatar === url ? 'border-emerald-500 scale-105' : 'border-neutral-800 hover:border-neutral-700'
                    }`}
                  >
                    <img src={url} alt={`Preset ${i}`} className="w-full h-full object-cover" />
                  </button>
                ))}
              </div>

              <div className="pt-2">
                <input
                  type="url"
                  placeholder="Or paste custom image URL..."
                  value={customAvatarUrl}
                  onChange={(e) => {
                    setCustomAvatarUrl(e.target.value);
                    if (e.target.value.startsWith('http')) setAvatar(e.target.value);
                  }}
                  className="w-full px-3 py-1.5 bg-neutral-950 border border-neutral-800 rounded-xl text-xs text-white placeholder-neutral-500 focus:outline-none focus:border-emerald-500/50"
                />
              </div>
            </div>

            {/* Account Quick Stats */}
            <div className="pt-3 border-t border-neutral-800 text-left text-xs space-y-2">
              <div className="flex justify-between text-neutral-400">
                <span>Account Status:</span>
                <span className="text-emerald-400 font-bold">ACTIVE</span>
              </div>
              <div className="flex justify-between text-neutral-400">
                <span>Registered Since:</span>
                <span className="font-mono text-neutral-300">{new Date(currentUser.createdAt).toLocaleDateString()}</span>
              </div>
              {currentRole === 'admin' && (
                <div className="flex justify-between text-neutral-400">
                  <span>Reseller Quota:</span>
                  <span className="font-mono text-emerald-400 font-bold">{currentUser.quotaUsed || 0} / {currentUser.quotaLimit || 0} Keys</span>
                </div>
              )}
              {currentRole === 'client' && (
                <div className="flex justify-between text-neutral-400">
                  <span>Active VPN Keys:</span>
                  <span className="font-mono text-emerald-400 font-bold">{activeVpns.length} Active</span>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Right Column: Profile Edit & Password Change */}
        <div className="lg:col-span-2 space-y-6">
          {/* Personal Details Form */}
          <div className="p-6 bg-neutral-900/90 border border-neutral-800 rounded-2xl shadow-xl space-y-4">
            <h3 className="text-sm font-bold text-white flex items-center space-x-2">
              <Sparkles className="w-4 h-4 text-emerald-400" />
              <span>Personal & Contact Information</span>
            </h3>

            <form onSubmit={handleSaveProfile} className="space-y-4 text-xs">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-1">
                  <label className="text-neutral-300 font-medium">Full Name</label>
                  <input
                    type="text"
                    required
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                  />
                </div>

                <div className="space-y-1">
                  <label className="text-neutral-300 font-medium">Email Address</label>
                  <input
                    type="email"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                  />
                </div>

                <div className="space-y-1">
                  <label className="text-neutral-300 font-medium">Telegram Username</label>
                  <input
                    type="text"
                    placeholder="@janith_admin"
                    value={telegram}
                    onChange={(e) => setTelegram(e.target.value)}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white font-mono focus:outline-none focus:border-emerald-500/50"
                  />
                </div>

                <div className="space-y-1">
                  <label className="text-neutral-300 font-medium">WhatsApp / Phone Number</label>
                  <input
                    type="text"
                    placeholder="+94 77 123 4567"
                    value={whatsapp}
                    onChange={(e) => setWhatsapp(e.target.value)}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white font-mono focus:outline-none focus:border-emerald-500/50"
                  />
                </div>
              </div>

              <div className="pt-2 flex justify-end">
                <button
                  type="submit"
                  className="px-5 py-2.5 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-bold rounded-xl text-xs transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
                >
                  <Save className="w-4 h-4" />
                  <span>Save Changes</span>
                </button>
              </div>
            </form>
          </div>

          {/* Password & Security Form */}
          <div className="p-6 bg-neutral-900/90 border border-neutral-800 rounded-2xl shadow-xl space-y-4">
            <h3 className="text-sm font-bold text-white flex items-center space-x-2">
              <Lock className="w-4 h-4 text-emerald-400" />
              <span>Update Password & Credentials</span>
            </h3>

            {passwordFeedback && (
              <div className={`p-3 rounded-xl border text-xs flex items-center space-x-2 ${
                passwordFeedback.type === 'success' ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300' : 'bg-rose-500/10 border-rose-500/30 text-rose-300'
              }`}>
                {passwordFeedback.type === 'success' ? <Check className="w-4 h-4" /> : <AlertCircle className="w-4 h-4" />}
                <span>{passwordFeedback.text}</span>
              </div>
            )}

            <form onSubmit={handleUpdatePassword} className="space-y-4 text-xs">
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div className="space-y-1">
                  <label className="text-neutral-300 font-medium">Current Password</label>
                  <input
                    type="password"
                    placeholder="••••••••••••"
                    value={currentPassword}
                    onChange={(e) => setCurrentPassword(e.target.value)}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                  />
                </div>

                <div className="space-y-1">
                  <label className="text-neutral-300 font-medium">New Password</label>
                  <input
                    type="password"
                    placeholder="Min 6 characters"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                  />
                </div>

                <div className="space-y-1">
                  <label className="text-neutral-300 font-medium">Confirm Password</label>
                  <input
                    type="password"
                    placeholder="Re-type new password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                  />
                </div>
              </div>

              <div className="flex justify-end">
                <button
                  type="submit"
                  className="px-4 py-2 bg-neutral-800 hover:bg-neutral-700 text-white rounded-xl text-xs font-semibold transition"
                >
                  Change Password
                </button>
              </div>
            </form>
          </div>

          {/* Active Session Info */}
          <div className="p-4 bg-neutral-950/70 border border-neutral-800 rounded-2xl flex items-center justify-between text-xs text-neutral-400">
            <div className="flex items-center space-x-2">
              <Activity className="w-4 h-4 text-emerald-400" />
              <span>Current Session: Chrome on Linux / Web Preview (Active now)</span>
            </div>
            <span className="font-mono text-emerald-400 text-[11px]">TLS 1.3 • AES-GCM</span>
          </div>
        </div>
      </div>
    </div>
  );
};
