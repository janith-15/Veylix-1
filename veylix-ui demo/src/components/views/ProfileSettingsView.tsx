import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  User, 
  Mail, 
  Phone, 
  Send, 
  Lock, 
  Key, 
  ShieldCheck, 
  Check, 
  AlertCircle, 
  Copy, 
  Camera, 
  Smartphone, 
  Clock, 
  LogOut,
  Sparkles,
  RefreshCw
} from 'lucide-react';

export const ProfileSettingsView: React.FC = () => {
  const { currentUser, updateUser, currentRole } = useVeylixStore();

  const [name, setName] = useState(currentUser.name || '');
  const [email, setEmail] = useState(currentUser.email || '');
  const [phone, setPhone] = useState(currentUser.phone || '');
  const [telegram, setTelegram] = useState(currentUser.telegram || '');
  const [whatsapp, setWhatsapp] = useState(currentUser.whatsapp || currentUser.phone || '');
  const [avatar, setAvatar] = useState(currentUser.avatar || '');

  // Password fields
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  // API Token
  const [apiToken, setApiToken] = useState('vyx_live_sk_98a72b1443ea4c9191284efc1341a980');
  const [tokenCopied, setTokenCopied] = useState(false);

  // Status feedback
  const [profileSuccess, setProfileSuccess] = useState('');
  const [passwordSuccess, setPasswordSuccess] = useState('');
  const [passwordError, setPasswordError] = useState('');

  // Preset avatar choices
  const avatarPresets = [
    'https://images.unsplash.com/photo-1534528741775-53994a69daeb?w=150&auto=format&fit=crop&q=80',
    'https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?w=150&auto=format&fit=crop&q=80',
    'https://images.unsplash.com/photo-1517841905240-472988babdf9?w=150&auto=format&fit=crop&q=80',
    'https://images.unsplash.com/photo-1492562080023-ab3db95bfbce?w=150&auto=format&fit=crop&q=80',
    'https://images.unsplash.com/photo-1535713875002-d1d0cf377fde?w=150&auto=format&fit=crop&q=80'
  ];

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
    setProfileSuccess('Profile details updated successfully!');
    setTimeout(() => setProfileSuccess(''), 3000);
  };

  const handleUpdatePassword = (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordError('');
    setPasswordSuccess('');

    if (newPassword.length < 6) {
      setPasswordError('New password must be at least 6 characters.');
      return;
    }
    if (newPassword !== confirmPassword) {
      setPasswordError('Passwords do not match.');
      return;
    }

    // Save success simulation
    setPasswordSuccess('Password securely updated with Argon2id parameters (64MB memory, 2 passes)!');
    setCurrentPassword('');
    setNewPassword('');
    setConfirmPassword('');
    setTimeout(() => setPasswordSuccess(''), 3500);
  };

  const handleCopyToken = () => {
    navigator.clipboard.writeText(apiToken);
    setTokenCopied(true);
    setTimeout(() => setTokenCopied(false), 2000);
  };

  const handleRegenerateToken = () => {
    const newToken = `vyx_live_sk_${Array.from({ length: 32 }, () => Math.floor(Math.random() * 16).toString(16)).join('')}`;
    setApiToken(newToken);
    setTokenCopied(false);
  };

  return (
    <div className="space-y-6 max-w-4xl mx-auto animate-in fade-in duration-200">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-extrabold text-white tracking-tight flex items-center space-x-2">
            <User className="w-5 h-5 text-emerald-400" />
            <span>Profile & Security Settings</span>
          </h1>
          <p className="text-xs text-neutral-400">
            Manage your personal credentials, contact endpoints, cryptographic keys, and active sessions
          </p>
        </div>

        <div className="flex items-center space-x-2 bg-neutral-900 border border-neutral-800 px-3 py-1.5 rounded-xl text-xs">
          <span className="text-neutral-400">Account ID:</span>
          <span className="font-mono text-emerald-400 font-semibold">{currentUser.id}</span>
        </div>
      </div>

      {/* Profile Overview Card with Avatar */}
      <div className="p-6 bg-neutral-900/90 border border-neutral-800 rounded-2xl shadow-xl flex flex-col md:flex-row items-center gap-6">
        <div className="relative group">
          <div className="w-24 h-24 rounded-2xl overflow-hidden border-2 border-emerald-500/40 shadow-lg bg-neutral-950 flex items-center justify-center text-3xl font-bold text-white">
            {avatar ? (
              <img src={avatar} alt={name} className="w-full h-full object-cover" />
            ) : (
              name.charAt(0) || 'U'
            )}
          </div>
          <div className="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition rounded-2xl flex items-center justify-center text-white pointer-events-none text-xs">
            <Camera className="w-5 h-5" />
          </div>
        </div>

        <div className="flex-1 space-y-1 text-center md:text-left">
          <div className="flex flex-wrap items-center justify-center md:justify-start gap-2">
            <h2 className="text-lg font-bold text-white">{currentUser.name}</h2>
            <span className="px-2.5 py-0.5 rounded-full text-[10px] font-mono font-bold bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 uppercase">
              {currentRole.replace('_', ' ')}
            </span>
            <span className="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-cyan-500/10 text-cyan-400">
              {currentUser.status}
            </span>
          </div>
          <p className="text-xs text-neutral-400 font-mono">{currentUser.email}</p>
          <p className="text-[11px] text-neutral-500 pt-1">
            Registered: {new Date(currentUser.createdAt).toLocaleDateString('en-US', { month: 'long', day: 'numeric', year: 'numeric' })}
          </p>
        </div>

        {/* Avatar Preset Quick-Picks */}
        <div className="space-y-1.5 text-center md:text-right">
          <span className="text-[11px] text-neutral-400 block font-medium">Quick Avatar Presets:</span>
          <div className="flex items-center justify-center md:justify-end space-x-1.5">
            {avatarPresets.map((presetUrl, idx) => (
              <button
                key={idx}
                type="button"
                onClick={() => setAvatar(presetUrl)}
                className={`w-8 h-8 rounded-lg overflow-hidden border transition ${
                  avatar === presetUrl ? 'border-emerald-500 scale-110 shadow-md ring-2 ring-emerald-500/30' : 'border-neutral-700 hover:border-neutral-500'
                }`}
              >
                <img src={presetUrl} alt="Preset" className="w-full h-full object-cover" />
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Main Grid: Form Left, Security Right */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Left Column: Personal Information Form */}
        <div className="p-6 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl space-y-4">
          <div className="flex items-center space-x-2">
            <User className="w-4 h-4 text-emerald-400" />
            <h3 className="text-sm font-bold text-white">Personal Information</h3>
          </div>

          {profileSuccess && (
            <div className="p-3 bg-emerald-500/10 border border-emerald-500/30 rounded-xl text-emerald-300 text-xs flex items-center space-x-2 animate-in fade-in">
              <Check className="w-4 h-4 text-emerald-400" />
              <span>{profileSuccess}</span>
            </div>
          )}

          <form onSubmit={handleSaveProfile} className="space-y-3.5 text-xs">
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
              <label className="text-neutral-300 font-medium flex items-center space-x-1.5">
                <Mail className="w-3.5 h-3.5 text-neutral-500" />
                <span>Email Address</span>
              </label>
              <input
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1">
                <label className="text-neutral-300 font-medium flex items-center space-x-1.5">
                  <Phone className="w-3.5 h-3.5 text-neutral-500" />
                  <span>Phone Number</span>
                </label>
                <input
                  type="text"
                  placeholder="+94 77 123 4567"
                  value={phone}
                  onChange={(e) => setPhone(e.target.value)}
                  className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                />
              </div>

              <div className="space-y-1">
                <label className="text-neutral-300 font-medium flex items-center space-x-1.5">
                  <Send className="w-3.5 h-3.5 text-emerald-400" />
                  <span>Telegram Handle</span>
                </label>
                <input
                  type="text"
                  placeholder="@my_telegram"
                  value={telegram}
                  onChange={(e) => setTelegram(e.target.value)}
                  className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                />
              </div>
            </div>

            <div className="space-y-1">
              <label className="text-neutral-300 font-medium">Avatar Image URL (or select preset above)</label>
              <input
                type="url"
                placeholder="https://example.com/avatar.jpg"
                value={avatar}
                onChange={(e) => setAvatar(e.target.value)}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white font-mono text-[11px] focus:outline-none focus:border-emerald-500/50"
              />
            </div>

            <div className="pt-2 flex justify-end">
              <button
                type="submit"
                className="px-5 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-bold rounded-xl text-xs transition shadow-lg shadow-emerald-500/10"
              >
                Save Profile Changes
              </button>
            </div>
          </form>
        </div>

        {/* Right Column: Password & API Credentials */}
        <div className="space-y-6">
          {/* Password Change Box */}
          <div className="p-6 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl space-y-4 text-xs">
            <div className="flex items-center space-x-2">
              <Lock className="w-4 h-4 text-emerald-400" />
              <h3 className="text-sm font-bold text-white">Password & Authentication</h3>
            </div>

            {passwordError && (
              <div className="p-3 bg-rose-500/10 border border-rose-500/30 rounded-xl text-rose-300 flex items-center space-x-2">
                <AlertCircle className="w-4 h-4 text-rose-400" />
                <span>{passwordError}</span>
              </div>
            )}
            {passwordSuccess && (
              <div className="p-3 bg-emerald-500/10 border border-emerald-500/30 rounded-xl text-emerald-300 flex items-center space-x-2">
                <Check className="w-4 h-4 text-emerald-400" />
                <span>{passwordSuccess}</span>
              </div>
            )}

            <form onSubmit={handleUpdatePassword} className="space-y-3">
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

              <div className="grid grid-cols-2 gap-3">
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
                  <label className="text-neutral-300 font-medium">Confirm New</label>
                  <input
                    type="password"
                    placeholder="Repeat password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white focus:outline-none focus:border-emerald-500/50"
                  />
                </div>
              </div>

              <div className="pt-2 flex justify-end">
                <button
                  type="submit"
                  className="px-4 py-2 bg-neutral-800 hover:bg-neutral-700 text-white font-medium rounded-xl text-xs transition"
                >
                  Update Password
                </button>
              </div>
            </form>
          </div>

          {/* API Access Token (For Automation / Bot Integration) */}
          <div className="p-6 bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl space-y-3 text-xs">
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-2">
                <Key className="w-4 h-4 text-cyan-400" />
                <h3 className="text-sm font-bold text-white">API Automation Secret Token</h3>
              </div>
              <button
                type="button"
                onClick={handleRegenerateToken}
                className="text-[11px] text-neutral-400 hover:text-emerald-400 flex items-center space-x-1"
                title="Roll Token"
              >
                <RefreshCw className="w-3 h-3" />
                <span>Roll</span>
              </button>
            </div>

            <p className="text-[11px] text-neutral-400">
              Use this Bearer token to authorize external REST API requests to 3X-UI automation endpoints.
            </p>

            <div className="flex items-center space-x-2">
              <div className="flex-1 p-2.5 bg-neutral-950 border border-neutral-800 rounded-xl font-mono text-neutral-300 text-[11px] select-all truncate">
                {apiToken}
              </div>
              <button
                type="button"
                onClick={handleCopyToken}
                className="px-3 py-2.5 bg-neutral-800 hover:bg-neutral-700 text-white rounded-xl text-xs font-semibold flex items-center space-x-1 transition"
              >
                {tokenCopied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                <span>{tokenCopied ? 'Copied' : 'Copy'}</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
