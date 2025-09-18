import { Link } from 'react-router-dom';
import { useState } from 'react';

import type { CounterAdjustmentRequest } from '@/apiclient/types.gen';
import {
  useAllUserCounters,
  useCounterHistory,
  useAdjustCounters,
} from '@/services/services';

export function CountersManagementPage() {
  const [counterId, setCounterId] = useState<string>('credits');
  const [selectedUser, setSelectedUser] = useState<string | null>(null);
  const [adjustmentForm, setAdjustmentForm] = useState<CounterAdjustmentRequest>({ amount: 0, description: '' });
  
  const {
    data: allUserCounters,
    isLoading: loadingUsers,
    error: errorUsers,
    refetch: refetchUsers,
  } = useAllUserCounters(counterId);
  
  const {
    data: counterHistory,
    error: errorHistory,
  } = useCounterHistory(counterId, selectedUser);
  
  const adjustCountersMutation = useAdjustCounters();
  const mutationLoading = adjustCountersMutation.status === 'pending';
  const error = errorUsers?.message || errorHistory?.message || (adjustCountersMutation.error instanceof Error ? adjustCountersMutation.error.message : null);

  return (
    <div className="w-full p-6">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">Counters Management</h1>
        <p className="text-gray-600 mt-2">Manage user counters and view transaction history</p>
      </div>

      {/* Counter Type Selection */}
        <div className="bg-white rounded-lg shadow-lg overflow-hidden mb-8">
          <div className="px-6 py-4 border-b border-gray-200">
            <h2 className="text-xl font-semibold text-gray-800">Counter ID</h2>
          </div>
          <div className="p-6">
            <input
              type="text"
              value={counterId}
              onChange={(e) => {
                setCounterId(e.target.value);
                setSelectedUser(null); // Reset selected user when changing counter type
              }}
              className="w-full p-3 border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
              placeholder="Enter counter ID (e.g. credits, coins, points, tokens)"
            />
          </div>
        </div>

      {error && (
        <div className="mb-4 p-4 bg-red-100 border border-red-400 text-red-700 rounded">
          {error}
        </div>
      )}

      {/* User Counters Overview */}
      <div className="bg-white rounded-lg shadow-lg overflow-hidden mb-8">
        <div className="px-6 py-4 border-b border-gray-200 flex justify-between items-center">
          <h2 className="text-xl font-semibold text-gray-800">
            User {counterId.charAt(0).toUpperCase() + counterId.slice(1)} Overview
          </h2>
          <button
            onClick={() => refetchUsers()}
            disabled={loadingUsers || !counterId}
            className="bg-blue-500 text-white px-4 py-2 rounded hover:bg-blue-600 disabled:opacity-50"
          >
            {loadingUsers ? 'Loading...' : 'Refresh'}
          </button>
        </div>
        <div className="p-6">
          {loadingUsers && !allUserCounters && (
            <p className="text-blue-500">Loading user counters...</p>
          )}

          {!counterId && (
            <p className="text-gray-500 text-center py-8">Please select a counter type</p>
          )}

          {counterId && allUserCounters && (
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead>
                  <tr>
                    <th className="text-left p-3 border-b border-gray-300 bg-gray-100">Username</th>
                    <th className="text-left p-3 border-b border-gray-300 bg-gray-100">Email</th>
                    <th className="text-left p-3 border-b border-gray-300 bg-gray-100">Balance</th>
                    <th className="text-left p-3 border-b border-gray-300 bg-gray-100">Last Updated</th>
                    <th className="text-left p-3 border-b border-gray-300 bg-gray-100">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {allUserCounters.users.map((user: any) => (
                    <tr key={user.user_id} className="hover:bg-gray-50">
                      <td className="p-3 border-b border-gray-200">{user.username}</td>
                      <td className="p-3 border-b border-gray-200">{user.email}</td>
                      <td className="p-3 border-b border-gray-200">
                        <span className={`font-semibold ${user.balance < 0 ? 'text-red-600' : 'text-green-600'}`}>
                          {user.balance}
                        </span>
                      </td>
                      <td className="p-3 border-b border-gray-200">
                        {new Date(user.last_updated).toLocaleDateString()}
                      </td>
                      <td className="p-3 border-b border-gray-200">
                        <button
                          onClick={() => setSelectedUser(user.user_id)}
                          className="bg-green-500 text-white px-3 py-1 rounded text-sm hover:bg-green-600 mr-2"
                        >
                          View History
                        </button>
                        <Link
                          to={`/users/${user.user_id}`}
                          className="bg-blue-500 text-white px-3 py-1 rounded text-sm hover:bg-blue-600"
                        >
                          View User
                        </Link>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {allUserCounters.users.length === 0 && (
                <p className="text-gray-500 text-center py-8">No users found</p>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Counter Adjustment Form */}
      {selectedUser && counterId && (
        <div className="bg-white rounded-lg shadow-lg overflow-hidden mb-8">
          <div className="px-6 py-4 border-b border-gray-200">
            <h2 className="text-xl font-semibold text-gray-800">
              Adjust {counterId.charAt(0).toUpperCase() + counterId.slice(1)}
            </h2>
          </div>
          <div className="p-6">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">Amount</label>
                <input
                  type="number"
                  value={adjustmentForm.amount}
                  onChange={(e) => setAdjustmentForm({ ...adjustmentForm, amount: parseInt(e.target.value) || 0 })}
                  className="w-full p-3 border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
                  placeholder="Enter amount (positive to add, negative to deduct)"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-2">Description</label>
                <input
                  type="text"
                  value={adjustmentForm.description}
                  onChange={(e) => setAdjustmentForm({ ...adjustmentForm, description: e.target.value })}
                  className="w-full p-3 border border-gray-300 rounded focus:ring-blue-500 focus:border-blue-500"
                  placeholder="Reason for adjustment"
                />
              </div>
              <div className="flex items-end">
                <button
                  onClick={() => adjustCountersMutation.mutate({ counterId, userId: selectedUser, adjustment: adjustmentForm })}
                  disabled={mutationLoading || !adjustmentForm.amount || !adjustmentForm.description.trim()}
                  className="w-full bg-orange-500 text-white px-4 py-3 rounded hover:bg-orange-600 disabled:opacity-50"
                >
                  {mutationLoading ? 'Processing...' : `Adjust ${counterId.charAt(0).toUpperCase() + counterId.slice(1)}`}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Counter History */}
      {selectedUser && counterId && counterHistory && (
        <div className="bg-white rounded-lg shadow-lg overflow-hidden">
          <div className="px-6 py-4 border-b border-gray-200">
            <h2 className="text-xl font-semibold text-gray-800">
              {counterId.charAt(0).toUpperCase() + counterId.slice(1)} History - Current Balance:
              <span className={`ml-2 ${counterHistory.current_balance < 0 ? 'text-red-600' : 'text-green-600'}`}>
                {counterHistory.current_balance}
              </span>
            </h2>
          </div>
          <div className="p-6">
            {counterHistory.transactions.length > 0 ? (
              <div className="overflow-x-auto">
                <table className="w-full border-collapse">
                  <thead>
                    <tr>
                      <th className="text-left p-3 border-b border-gray-300 bg-gray-100">Date</th>
                      <th className="text-left p-3 border-b border-gray-300 bg-gray-100">Amount</th>
                      <th className="text-left p-3 border-b border-gray-300 bg-gray-100">Balance After</th>
                      <th className="text-left p-3 border-b border-gray-300 bg-gray-100">Description</th>
                    </tr>
                  </thead>
                  <tbody>
                    {counterHistory.transactions.map((transaction: any) => (
                      <tr key={transaction.id} className="hover:bg-gray-50">
                        <td className="p-3 border-b border-gray-200">{new Date(transaction.created_at).toLocaleString()}</td>
                        <td className="p-3 border-b border-gray-200">
                          <span className={`font-semibold ${transaction.amount < 0 ? 'text-red-600' : 'text-green-600'}`}>
                            {transaction.amount > 0 ? '+' : ''}{transaction.amount}
                          </span>
                        </td>
                        <td className="p-3 border-b border-gray-200">{transaction.balance_after}</td>
                        <td className="p-3 border-b border-gray-200">{transaction.description}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p className="text-gray-500 text-center py-8">No counter transactions found</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
