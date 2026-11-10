import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { Order } from '../../types';
import { X, CheckCircle, XCircle, CreditCard, User, AlertCircle, FileText, ExternalLink } from 'lucide-react';

interface PaymentProofModalProps {
  order: Order | null;
  onClose: () => void;
  onSuccessOpenQr?: (vpnAccount: any) => void;
}

export const PaymentProofModal: React.FC<PaymentProofModalProps> = ({ 
  order, 
  onClose,
  onSuccessOpenQr 
}) => {
  const { verifyOrder, rejectOrder, currentUser, vpnAccounts } = useVeylixStore();
  const [rejectReason, setRejectReason] = useState('');
  const [isRejecting, setIsRejecting] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');

  if (!order) return null;

  const linkedVpn = order.vpnAccountId ? vpnAccounts.find(v => v.id === order.vpnAccountId) : null;

  const handleVerify = () => {
    setErrorMsg('');
    const res = verifyOrder(order.id, currentUser.name);
    if (!res.success) {
      setErrorMsg(res.error || 'Verification failed');
      return;
    }
    onClose();
    if (onSuccessOpenQr && linkedVpn) {
      onSuccessOpenQr(linkedVpn);
    }
  };

  const handleReject = () => {
    if (!rejectReason) {
      setErrorMsg('Please enter a rejection reason.');
      return;
    }
    rejectOrder(order.id, rejectReason);
    onClose();
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
              <h3 className="text-base font-semibold text-white">Order Review & Settlement</h3>
              <p className="text-xs text-neutral-400">Ref #{order.orderNumber} • {order.packageName}</p>
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
        <div className="p-6 overflow-y-auto space-y-4 text-xs">
          {errorMsg && (
            <div className="p-3 bg-rose-500/10 border border-rose-500/30 rounded-xl text-rose-300 flex items-center space-x-2">
              <AlertCircle className="w-4 h-4 shrink-0" />
              <span>{errorMsg}</span>
            </div>
          )}

          {/* Order Details summary */}
          <div className="grid grid-cols-2 gap-3 p-4 bg-neutral-950/70 border border-neutral-800 rounded-xl">
            <div>
              <span className="text-neutral-400">Customer:</span>
              <p className="font-semibold text-neutral-200 text-sm mt-0.5">{order.clientName}</p>
            </div>
            <div>
              <span className="text-neutral-400">Total Settlement:</span>
              <p className="font-bold text-emerald-400 text-base mt-0.5 font-mono">${order.amount.toFixed(2)} {order.currency}</p>
            </div>
            <div>
              <span className="text-neutral-400">Payment Gateway:</span>
              <p className="font-medium text-neutral-200 mt-0.5">{order.paymentMethod.replace('_', ' ')}</p>
            </div>
            <div>
              <span className="text-neutral-400">Status:</span>
              <div className="mt-0.5">
                <span className={`px-2 py-0.5 rounded-full text-[11px] font-semibold ${
                  order.status === 'VERIFIED' ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30' :
                  order.status === 'PENDING' ? 'bg-amber-500/10 text-amber-400 border border-amber-500/30' :
                  'bg-rose-500/10 text-rose-400 border border-rose-500/30'
                }`}>
                  {order.status}
                </span>
              </div>
            </div>
          </div>

          {/* Proof Info & Image */}
          <div className="space-y-2">
            <span className="font-medium text-neutral-300">Transaction Proof / Wire Reference:</span>
            {order.proofNote ? (
              <div className="p-3 bg-neutral-950 border border-neutral-800 rounded-xl text-neutral-200 font-mono text-xs break-all">
                {order.proofNote}
              </div>
            ) : (
              <p className="text-neutral-500 italic">No text transaction reference provided.</p>
            )}

            {order.proofImageUrl && (
              <div className="mt-2 space-y-1">
                <span className="text-neutral-400">Uploaded Bank Transfer Receipt:</span>
                <div className="relative group rounded-xl overflow-hidden border border-neutral-800 bg-neutral-950 max-h-48">
                  <img 
                    src={order.proofImageUrl} 
                    alt="Payment Slip" 
                    className="w-full object-cover object-center group-hover:scale-105 transition duration-300" 
                  />
                  <a 
                    href={order.proofImageUrl} 
                    target="_blank" 
                    rel="noreferrer"
                    className="absolute bottom-2 right-2 px-2.5 py-1 bg-black/80 backdrop-blur rounded-lg text-[11px] text-white flex items-center space-x-1"
                  >
                    <ExternalLink className="w-3 h-3" />
                    <span>View Full Slip</span>
                  </a>
                </div>
              </div>
            )}
          </div>

          {/* Rejection input box */}
          {isRejecting && (
            <div className="p-3 bg-neutral-950 border border-rose-500/30 rounded-xl space-y-2 animate-in fade-in">
              <label className="text-rose-300 font-medium">Rejection Reason</label>
              <textarea
                value={rejectReason}
                onChange={(e) => setRejectReason(e.target.value)}
                placeholder="e.g. Transaction ID could not be located in bank statement"
                className="w-full p-2.5 bg-neutral-900 border border-neutral-800 rounded-lg text-neutral-200 focus:outline-none focus:border-rose-500/50"
                rows={2}
              />
              <div className="flex justify-end space-x-2">
                <button
                  type="button"
                  onClick={() => setIsRejecting(false)}
                  className="px-3 py-1 bg-neutral-800 hover:bg-neutral-700 text-neutral-300 rounded-lg text-xs"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={handleReject}
                  className="px-3 py-1 bg-rose-500 hover:bg-rose-600 text-white font-medium rounded-lg text-xs"
                >
                  Confirm Rejection
                </button>
              </div>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="px-6 py-4 bg-neutral-950/60 border-t border-neutral-800 flex items-center justify-between">
          {order.status === 'PENDING' ? (
            <>
              {!isRejecting && (
                <button
                  type="button"
                  onClick={() => setIsRejecting(true)}
                  className="px-4 py-2 text-xs font-medium text-rose-400 hover:text-rose-300 bg-rose-500/10 hover:bg-rose-500/20 border border-rose-500/20 rounded-xl transition flex items-center space-x-1.5"
                >
                  <XCircle className="w-4 h-4" />
                  <span>Reject Order</span>
                </button>
              )}
              <button
                type="button"
                onClick={handleVerify}
                className="ml-auto px-5 py-2 text-xs font-semibold text-neutral-950 bg-emerald-400 hover:bg-emerald-300 rounded-xl transition flex items-center space-x-1.5 shadow-lg shadow-emerald-500/10"
              >
                <CheckCircle className="w-4 h-4" />
                <span>Verify & Auto-Provision VPN</span>
              </button>
            </>
          ) : (
            <div className="w-full flex items-center justify-between text-neutral-400">
              <span>Settlement finalized by {order.verifierName || 'Admin'}</span>
              <button
                onClick={onClose}
                className="px-4 py-2 bg-neutral-800 hover:bg-neutral-700 text-white rounded-xl text-xs"
              >
                Close
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
