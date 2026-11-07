import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { VpnPackage, PaymentMethod } from '../../types';
import { X, CreditCard, ShieldCheck, CheckCircle2, AlertCircle, Coins, Building, Smartphone } from 'lucide-react';

interface OrderPackageModalProps {
  pkg: VpnPackage | null;
  onClose: () => void;
  onSuccessOpenOrders?: () => void;
}

export const OrderPackageModal: React.FC<OrderPackageModalProps> = ({ 
  pkg, 
  onClose,
  onSuccessOpenOrders 
}) => {
  const { createOrder, currentUser, currentRole } = useVeylixStore();
  const [paymentMethod, setPaymentMethod] = useState<PaymentMethod>('CRYPTO_USDT');
  const [proofNote, setProofNote] = useState('');
  const [submitted, setSubmitted] = useState(false);

  if (!pkg) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    createOrder({
      packageId: pkg.id,
      clientId: currentUser.id,
      paymentMethod,
      proofNote: proofNote || `Payment submitted via ${paymentMethod} on ${new Date().toLocaleDateString()}`
    });

    setSubmitted(true);
    setTimeout(() => {
      onClose();
      if (onSuccessOpenOrders) {
        onSuccessOpenOrders();
      }
    }, 1500);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-in fade-in duration-200">
      <div className="relative w-full max-w-lg bg-neutral-900 border border-neutral-800 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[92vh]">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-800 bg-neutral-950/50">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
              <CreditCard className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-semibold text-white">Order {pkg.name}</h3>
              <p className="text-xs text-neutral-400">Total Settlement: ${pkg.retailPrice.toFixed(2)} USD • {pkg.durationDays} Days</p>
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
        {submitted ? (
          <div className="p-10 text-center space-y-3">
            <CheckCircle2 className="w-12 h-12 text-emerald-400 mx-auto animate-bounce" />
            <h3 className="text-base font-bold text-white">Order Submitted Successfully!</h3>
            <p className="text-xs text-neutral-400 max-w-sm mx-auto">
              Your payment reference has been recorded. Once verified by your seller, your VPN key will be auto-provisioned immediately.
            </p>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="p-6 overflow-y-auto space-y-4 text-xs">
            {/* Package Summary Box */}
            <div className="p-4 bg-neutral-950 border border-neutral-800 rounded-xl space-y-2">
              <div className="flex items-center justify-between">
                <span className="font-semibold text-white text-sm">{pkg.name}</span>
                <span className="font-mono text-emerald-400 font-bold text-base">${pkg.retailPrice.toFixed(2)}</span>
              </div>
              <p className="text-neutral-400 text-[11px]">{pkg.tagline}</p>
              <div className="flex items-center space-x-3 text-[11px] text-neutral-400 pt-1 font-mono">
                <span>Data: {pkg.trafficGb} GB</span>
                <span>•</span>
                <span>Validity: {pkg.durationDays} Days</span>
                <span>•</span>
                <span>Protocol: {pkg.protocol}</span>
              </div>
            </div>

            {/* Payment Method Selector */}
            <div className="space-y-1.5">
              <label className="text-neutral-300 font-medium">Select Payment Gateway</label>
              <div className="grid grid-cols-2 gap-2">
                {[
                  { id: 'CRYPTO_USDT' as PaymentMethod, label: 'USDT (TRC20 / Polygon)', icon: Coins },
                  { id: 'BANK_TRANSFER' as PaymentMethod, label: 'Direct Bank Wire', icon: Building },
                  { id: 'CREDIT_CARD' as PaymentMethod, label: 'Credit / Debit Card', icon: CreditCard },
                  { id: 'PROMPTPAY' as PaymentMethod, label: 'PromptPay / Local QR', icon: Smartphone }
                ].map(m => {
                  const Icon = m.icon;
                  const isSel = paymentMethod === m.id;
                  return (
                    <button
                      type="button"
                      key={m.id}
                      onClick={() => setPaymentMethod(m.id)}
                      className={`p-3 rounded-xl border text-left flex items-center space-x-2.5 transition ${
                        isSel
                          ? 'bg-emerald-500/10 border-emerald-500/40 text-emerald-300 font-semibold'
                          : 'bg-neutral-950 border-neutral-800 text-neutral-400 hover:text-white'
                      }`}
                    >
                      <Icon className="w-4 h-4 shrink-0" />
                      <span className="text-[11px]">{m.label}</span>
                    </button>
                  );
                })}
              </div>
            </div>

            {/* Settlement Instructions */}
            <div className="p-3 bg-neutral-950/70 border border-neutral-800 rounded-xl space-y-1 text-neutral-300 text-[11px]">
              {paymentMethod === 'CRYPTO_USDT' && (
                <>
                  <p className="font-semibold text-white">USDT TRC-20 Address:</p>
                  <p className="font-mono text-emerald-400 select-all break-all">TXYZ98a72b1443ea4c9191284efc1341a980</p>
                  <p className="text-neutral-500 text-[10px]">Send exact ${pkg.retailPrice.toFixed(2)} USDT.</p>
                </>
              )}
              {paymentMethod === 'BANK_TRANSFER' && (
                <>
                  <p className="font-semibold text-white">Bank Wire Settlement Details:</p>
                  <p className="text-neutral-300">Bank: Commercial Bank PLC • Acct: 8819002341 (Veylix Network)</p>
                </>
              )}
              {paymentMethod === 'CREDIT_CARD' && (
                <>
                  <p className="font-semibold text-white">Credit Card Processing:</p>
                  <p className="text-neutral-300">Instant gateway simulation (Stripe / 3D Secure)</p>
                </>
              )}
              {paymentMethod === 'PROMPTPAY' && (
                <>
                  <p className="font-semibold text-white">PromptPay ID:</p>
                  <p className="font-mono text-emerald-400">089-123-4567 (Veylix Services)</p>
                </>
              )}
            </div>

            {/* Transaction Ref Input */}
            <div className="space-y-1">
              <label className="text-neutral-300 font-medium">Transaction Reference / TX Hash / Slip Note</label>
              <input
                type="text"
                placeholder="e.g. 0x88c42... or Bank ref #192003"
                value={proofNote}
                onChange={(e) => setProofNote(e.target.value)}
                className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-white font-mono text-xs focus:outline-none focus:border-emerald-500/50"
              />
            </div>

            {/* Footer */}
            <div className="pt-3 flex items-center justify-end space-x-3 border-t border-neutral-800">
              <button
                type="button"
                onClick={onClose}
                className="px-4 py-2 bg-neutral-800 hover:bg-neutral-700 text-white rounded-xl text-xs"
              >
                Cancel
              </button>
              <button
                type="submit"
                className="px-5 py-2 bg-emerald-400 hover:bg-emerald-300 text-neutral-950 font-bold rounded-xl text-xs transition shadow-lg shadow-emerald-500/10"
              >
                Submit Order & Settlement
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
};
