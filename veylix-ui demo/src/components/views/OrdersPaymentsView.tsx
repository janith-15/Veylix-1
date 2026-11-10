import React, { useState } from 'react';
import { useVeylixStore } from '../../store/useVeylixStore';
import { 
  CreditCard, 
  CheckCircle2, 
  Clock, 
  XCircle, 
  Eye, 
  FileText, 
  DollarSign, 
  Search, 
  Filter 
} from 'lucide-react';
import { Order, OrderStatus } from '../../types';

interface OrdersPaymentsViewProps {
  onSelectOrderForReview: (order: Order) => void;
}

export const OrdersPaymentsView: React.FC<OrdersPaymentsViewProps> = ({ onSelectOrderForReview }) => {
  const { orders, currentRole, currentUser } = useVeylixStore();
  const [filterStatus, setFilterStatus] = useState<string>('ALL');
  const [search, setSearch] = useState('');

  // If reseller, show their clients' orders
  const visibleOrders = currentRole === 'admin'
    ? orders.filter(o => o.adminId === currentUser.id)
    : currentRole === 'client'
    ? orders.filter(o => o.clientId === currentUser.id)
    : orders;

  const filteredOrders = visibleOrders.filter(order => {
    const matchesStatus = filterStatus === 'ALL' || order.status === filterStatus;
    const matchesSearch = order.orderNumber.toLowerCase().includes(search.toLowerCase()) ||
      order.clientName.toLowerCase().includes(search.toLowerCase()) ||
      order.packageName.toLowerCase().includes(search.toLowerCase());
    return matchesStatus && matchesSearch;
  });

  const totalSettled = visibleOrders
    .filter(o => o.status === 'VERIFIED')
    .reduce((sum, o) => sum + o.amount, 0);

  const pendingAmount = visibleOrders
    .filter(o => o.status === 'PENDING')
    .reduce((sum, o) => sum + o.amount, 0);

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-xl font-extrabold text-white tracking-tight flex items-center space-x-2">
            <CreditCard className="w-5 h-5 text-emerald-400" />
            <span>Orders & Payment Settlement</span>
          </h1>
          <p className="text-xs text-neutral-400">
            Section 3.6 Master Specification • Verification before activation workflow
          </p>
        </div>
      </div>

      {/* Revenue & Metrics Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="p-4 bg-neutral-900 border border-neutral-800 rounded-2xl space-y-1">
          <span className="text-xs text-neutral-400">Settled Revenue</span>
          <p className="text-2xl font-extrabold text-emerald-400 font-mono">${totalSettled.toFixed(2)} USD</p>
          <p className="text-[11px] text-neutral-500">Verified & provisioned accounts</p>
        </div>

        <div className="p-4 bg-neutral-900 border border-neutral-800 rounded-2xl space-y-1">
          <span className="text-xs text-neutral-400">Pending Review Settlement</span>
          <p className="text-2xl font-extrabold text-amber-400 font-mono">${pendingAmount.toFixed(2)} USD</p>
          <p className="text-[11px] text-neutral-500">
            {visibleOrders.filter(o => o.status === 'PENDING').length} order(s) awaiting verification
          </p>
        </div>

        <div className="p-4 bg-neutral-900 border border-neutral-800 rounded-2xl space-y-1">
          <span className="text-xs text-neutral-400">Total Transaction Count</span>
          <p className="text-2xl font-extrabold text-white font-mono">{visibleOrders.length}</p>
          <p className="text-[11px] text-neutral-500">Crypto USDT, Wire, Cards</p>
        </div>
      </div>

      {/* Filter and Search */}
      <div className="p-4 bg-neutral-900 border border-neutral-800 rounded-2xl flex flex-col sm:flex-row gap-3">
        <div className="relative flex-1">
          <Search className="w-4 h-4 text-neutral-400 absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search order ref, customer, package..."
            className="w-full pl-9 pr-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-xs text-white placeholder-neutral-500 focus:outline-none focus:border-emerald-500/50"
          />
        </div>

        <div className="w-full sm:w-48">
          <select
            value={filterStatus}
            onChange={(e) => setFilterStatus(e.target.value)}
            className="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-xl text-xs text-neutral-200 focus:outline-none focus:border-emerald-500/50"
          >
            <option value="ALL">All Order Statuses</option>
            <option value="PENDING">Pending Review</option>
            <option value="VERIFIED">Verified & Active</option>
            <option value="REJECTED">Rejected</option>
          </select>
        </div>
      </div>

      {/* Orders Table */}
      <div className="bg-neutral-900 border border-neutral-800 rounded-2xl shadow-xl overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead>
              <tr className="border-b border-neutral-800 text-neutral-400 bg-neutral-950/70">
                <th className="py-3 px-4 font-medium">Order Number</th>
                <th className="py-3 px-4 font-medium">Customer</th>
                <th className="py-3 px-4 font-medium">Package Item</th>
                <th className="py-3 px-4 font-medium">Method</th>
                <th className="py-3 px-4 font-medium">Amount</th>
                <th className="py-3 px-4 font-medium">Date</th>
                <th className="py-3 px-4 font-medium">Status</th>
                <th className="py-3 px-4 font-medium text-right">Review</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-800/60">
              {filteredOrders.length === 0 ? (
                <tr>
                  <td colSpan={8} className="py-10 text-center text-neutral-500">
                    No orders found.
                  </td>
                </tr>
              ) : (
                filteredOrders.map(order => (
                  <tr key={order.id} className="hover:bg-neutral-800/30 transition">
                    <td className="py-3 px-4 font-mono font-bold text-white">
                      {order.orderNumber}
                    </td>
                    <td className="py-3 px-4 text-neutral-200 font-medium">
                      {order.clientName}
                    </td>
                    <td className="py-3 px-4 text-neutral-300">
                      {order.packageName}
                    </td>
                    <td className="py-3 px-4 text-neutral-400 capitalize">
                      {order.paymentMethod.replace('_', ' ')}
                    </td>
                    <td className="py-3 px-4 font-mono font-bold text-emerald-400">
                      ${order.amount.toFixed(2)}
                    </td>
                    <td className="py-3 px-4 font-mono text-[11px] text-neutral-400">
                      {new Date(order.createdAt).toLocaleDateString('en-US', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
                    </td>
                    <td className="py-3 px-4">
                      <span className={`px-2.5 py-0.5 rounded-full text-[10px] font-bold ${
                        order.status === 'VERIFIED' ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30' :
                        order.status === 'PENDING' ? 'bg-amber-500/10 text-amber-400 border border-amber-500/30' :
                        'bg-rose-500/10 text-rose-400 border border-rose-500/30'
                      }`}>
                        {order.status}
                      </span>
                    </td>
                    <td className="py-3 px-4 text-right">
                      <button
                        onClick={() => onSelectOrderForReview(order)}
                        className={`px-3 py-1 rounded-lg text-xs font-semibold transition flex items-center space-x-1.5 ml-auto ${
                          order.status === 'PENDING'
                            ? 'bg-amber-500/20 hover:bg-amber-500/30 text-amber-300 border border-amber-500/30'
                            : 'bg-neutral-800 hover:bg-neutral-700 text-neutral-300'
                        }`}
                      >
                        <Eye className="w-3.5 h-3.5" />
                        <span>{order.status === 'PENDING' ? 'Verify Proof' : 'View Slip'}</span>
                      </button>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};
