import { useState } from 'react';
import { Link } from 'react-router-dom';

import type { CounterAdjustmentRequest, UserCountersResponse } from '@/apiclient/types.gen';
import { useAdjustCounters, useAllUserCounters, useAvailableCounters, useCounterHistory } from '@/services/services';
import { Button } from '../components/ui/Button';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { FormField } from '../components/ui/FormField';
import { Input } from '../components/ui/Input';
import { PageHeader } from '../components/ui/PageHeader';
import { StatusPill } from '../components/ui/StatusPill';

export function CountersManagementPage() {
    const [counterId, setCounterId] = useState<string>('credits');
    const [selectedUser, setSelectedUser] = useState<string | null>(null);
    const [adjustmentForm, setAdjustmentForm] = useState<CounterAdjustmentRequest>({ amount: 0, description: '' });

    const {
        data: availableCounters,
        isLoading: loadingAvailableCounters,
        error: errorAvailableCounters,
        refetch: refetchAvailableCounters,
    } = useAvailableCounters();

    const {
        data: allUserCounters,
        isLoading: loadingUsers,
        error: errorUsers,
        refetch: refetchUsers,
    } = useAllUserCounters(counterId);

    const { data: counterHistory, error: errorHistory } = useCounterHistory(counterId, selectedUser);

    const adjustCountersMutation = useAdjustCounters();
    const mutationLoading = adjustCountersMutation.status === 'pending';

    const pageError = errorAvailableCounters?.message || errorUsers?.message || errorHistory?.message || null;

    const counterLabel = counterId ? counterId.charAt(0).toUpperCase() + counterId.slice(1) : 'Counter';

    return (
        <div className="mx-auto w-full max-w-7xl space-y-6">
            <PageHeader
                title="Counters"
                description="Manage user counters and review transaction history."
            />

            <Card>
                <CardHeader>
                    <div className="flex items-center justify-between gap-4">
                        <div>
                            <h2 className="text-lg font-semibold">Available Counters</h2>
                            <p className="mt-1 text-sm text-muted-fg">Quick-select an existing counter type.</p>
                        </div>
                        <Button
                            variant="outline"
                            size="sm"
                            onClick={() => refetchAvailableCounters()}
                            disabled={loadingAvailableCounters}
                        >
                            {loadingAvailableCounters ? 'Loading…' : 'Refresh'}
                        </Button>
                    </div>
                </CardHeader>
                <CardContent>
                    {errorAvailableCounters && (
                        <div className="rounded-lg border border-danger/30 bg-danger/5 p-4 text-danger">
                            <div className="font-semibold">Error loading available counters</div>
                            <div className="mt-1 text-sm text-danger/80">{errorAvailableCounters.message}</div>
                            <div className="mt-2 text-xs text-danger/70">
                                If you’re not an admin, you may not have permission to access this feature.
                            </div>
                            <div className="mt-3">
                                <Button variant="danger" size="sm" onClick={() => refetchAvailableCounters()}>
                                    Retry
                                </Button>
                            </div>
                        </div>
                    )}

                    {availableCounters && availableCounters.counters.length > 0 && (
                        <div className="flex flex-wrap gap-2">
                            {availableCounters.counters.map((counter) => (
                                <Button
                                    key={counter}
                                    type="button"
                                    size="sm"
                                    variant={counterId === counter ? 'primary' : 'secondary'}
                                    onClick={() => {
                                        setCounterId(counter);
                                        setSelectedUser(null);
                                    }}
                                >
                                    {counter}
                                </Button>
                            ))}
                        </div>
                    )}

                    {availableCounters && availableCounters.counters.length === 0 && (
                        <div className="rounded-lg border border-warning/30 bg-warning/5 p-4 text-warning">
                            <div className="font-semibold">No counter types available</div>
                            <div className="mt-1 text-sm text-warning/80">
                                No counters have been created yet. You can still manually enter a counter ID below.
                            </div>
                        </div>
                    )}
                </CardContent>
            </Card>

            <Card>
                <CardHeader>
                    <h2 className="text-lg font-semibold">Counter ID</h2>
                </CardHeader>
                <CardContent>
                    <FormField label="Counter ID" required>
                        <Input
                            type="text"
                            value={counterId}
                            onChange={(e) => {
                                setCounterId(e.target.value);
                                setSelectedUser(null);
                            }}
                            placeholder="Enter counter ID (e.g. credits, coins, points, tokens)"
                        />
                    </FormField>
                </CardContent>
            </Card>

            {pageError && (
                <div className="rounded-lg border border-danger/30 bg-danger/5 p-4 text-danger">
                    {pageError}
                </div>
            )}

            <Card>
                <CardHeader>
                    <div className="flex items-center justify-between gap-4">
                        <div>
                            <h2 className="text-lg font-semibold">User {counterLabel} Overview</h2>
                            <p className="mt-1 text-sm text-muted-fg">
                                Select a user to view history and apply adjustments.
                            </p>
                        </div>
                        <Button
                            onClick={() => refetchUsers()}
                            disabled={loadingUsers || !counterId.trim()}
                            variant="outline"
                            size="sm"
                        >
                            {loadingUsers ? 'Loading…' : 'Refresh'}
                        </Button>
                    </div>
                </CardHeader>
                <CardContent>
                    {loadingUsers && !allUserCounters && <p className="text-muted-fg">Loading user counters…</p>}

                    {!counterId.trim() && (
                        <p className="py-8 text-center text-muted-fg">Please enter a counter ID.</p>
                    )}

                    {counterId.trim() && allUserCounters && (
                        <div className="overflow-x-auto">
                            <table className="w-full border-collapse text-sm">
                                <thead>
                                    <tr>
                                        <th className="whitespace-nowrap border-b border-border bg-surface-2 px-3 py-3 text-left text-xs font-semibold uppercase tracking-wide text-muted-fg">
                                            Username
                                        </th>
                                        <th className="whitespace-nowrap border-b border-border bg-surface-2 px-3 py-3 text-left text-xs font-semibold uppercase tracking-wide text-muted-fg">
                                            Email
                                        </th>
                                        <th className="whitespace-nowrap border-b border-border bg-surface-2 px-3 py-3 text-left text-xs font-semibold uppercase tracking-wide text-muted-fg">
                                            Balance
                                        </th>
                                        <th className="whitespace-nowrap border-b border-border bg-surface-2 px-3 py-3 text-left text-xs font-semibold uppercase tracking-wide text-muted-fg">
                                            Has History
                                        </th>
                                        <th className="whitespace-nowrap border-b border-border bg-surface-2 px-3 py-3 text-left text-xs font-semibold uppercase tracking-wide text-muted-fg">
                                            Last Updated
                                        </th>
                                        <th className="whitespace-nowrap border-b border-border bg-surface-2 px-3 py-3 text-left text-xs font-semibold uppercase tracking-wide text-muted-fg">
                                            Actions
                                        </th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {allUserCounters.users.map((user: UserCountersResponse) => (
                                        <tr key={user.user_id} className="hover:bg-muted/40">
                                            <td className="border-b border-border px-3 py-3">{user.username}</td>
                                            <td className="border-b border-border px-3 py-3">{user.email}</td>
                                            <td className="border-b border-border px-3 py-3">
                                                <span
                                                    className={
                                                        user.balance < 0
                                                            ? 'font-semibold text-danger'
                                                            : 'font-semibold text-success'
                                                    }
                                                >
                                                    {user.balance}
                                                </span>
                                            </td>
                                            <td className="border-b border-border px-3 py-3">
                                                {user.has_history ? (
                                                    <StatusPill variant="success">✓ Yes</StatusPill>
                                                ) : (
                                                    <StatusPill>○ No</StatusPill>
                                                )}
                                            </td>
                                            <td className="border-b border-border px-3 py-3">
                                                {new Date(user.last_updated).toLocaleDateString()}
                                            </td>
                                            <td className="border-b border-border px-3 py-3">
                                                <div className="flex flex-wrap gap-2">
                                                    <Button
                                                        onClick={() => setSelectedUser(user.user_id)}
                                                        variant="secondary"
                                                        size="sm"
                                                    >
                                                        View History
                                                    </Button>
                                                    <Link
                                                        to={`/users/${user.user_id}`}
                                                        className="inline-flex h-9 items-center justify-center rounded-lg border border-border bg-transparent px-3 text-sm font-medium text-fg transition-colors hover:bg-muted/60"
                                                    >
                                                        View User
                                                    </Link>
                                                </div>
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>

                            {allUserCounters.users.length === 0 && (
                                <p className="py-8 text-center text-muted-fg">No users found.</p>
                            )}
                        </div>
                    )}
                </CardContent>
            </Card>

            {selectedUser && counterId.trim() && (
                <Card>
                    <CardHeader>
                        <div className="flex flex-wrap items-center justify-between gap-3">
                            <h2 className="text-lg font-semibold">Adjust {counterLabel}</h2>
                            <p className="text-sm text-muted-fg">
                                User: <span className="font-mono text-fg">{selectedUser}</span>
                            </p>
                        </div>
                    </CardHeader>
                    <CardContent>
                        <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                            <FormField label="Amount" required>
                                <Input
                                    type="number"
                                    value={adjustmentForm.amount}
                                    onChange={(e) =>
                                        setAdjustmentForm({
                                            ...adjustmentForm,
                                            amount: parseInt(e.target.value, 10) || 0,
                                        })
                                    }
                                    placeholder="Positive to add, negative to deduct"
                                />
                            </FormField>
                            <FormField label="Description" required>
                                <Input
                                    type="text"
                                    value={adjustmentForm.description}
                                    onChange={(e) =>
                                        setAdjustmentForm({
                                            ...adjustmentForm,
                                            description: e.target.value,
                                        })
                                    }
                                    placeholder="Reason for adjustment"
                                />
                            </FormField>
                            <div className="flex items-end">
                                <Button
                                    className="w-full"
                                    onClick={() =>
                                        adjustCountersMutation.mutate({
                                            counterId,
                                            userId: selectedUser,
                                            adjustment: adjustmentForm,
                                        })
                                    }
                                    disabled={
                                        mutationLoading ||
                                        adjustmentForm.amount === 0 ||
                                        !adjustmentForm.description.trim()
                                    }
                                >
                                    {mutationLoading ? 'Processing…' : `Adjust ${counterLabel}`}
                                </Button>
                            </div>
                        </div>

                        {adjustCountersMutation.error && (
                            <div className="mt-6 rounded-lg border border-danger/30 bg-danger/5 p-4 text-danger">
                                <strong>Adjustment Error:</strong>{' '}
                                {adjustCountersMutation.error instanceof Error
                                    ? adjustCountersMutation.error.message
                                    : String(adjustCountersMutation.error)}
                            </div>
                        )}
                    </CardContent>
                </Card>
            )}

            {selectedUser && counterId.trim() && counterHistory && (
                <Card>
                    <CardHeader>
                        <div className="flex flex-wrap items-center justify-between gap-3">
                            <h2 className="text-lg font-semibold">{counterLabel} History</h2>
                            <div className="text-sm text-muted-fg">
                                Current Balance:{' '}
                                <span
                                    className={
                                        counterHistory.current_balance < 0
                                            ? 'ml-2 font-semibold text-danger'
                                            : 'ml-2 font-semibold text-success'
                                    }
                                >
                                    {counterHistory.current_balance}
                                </span>
                            </div>
                        </div>
                    </CardHeader>
                    <CardContent>
                        {counterHistory.transactions.length > 0 ? (
                            <div className="overflow-x-auto">
                                <table className="w-full border-collapse text-sm">
                                    <thead>
                                        <tr>
                                            <th className="whitespace-nowrap border-b border-border bg-surface-2 px-3 py-3 text-left text-xs font-semibold uppercase tracking-wide text-muted-fg">
                                                Date
                                            </th>
                                            <th className="whitespace-nowrap border-b border-border bg-surface-2 px-3 py-3 text-left text-xs font-semibold uppercase tracking-wide text-muted-fg">
                                                Amount
                                            </th>
                                            <th className="whitespace-nowrap border-b border-border bg-surface-2 px-3 py-3 text-left text-xs font-semibold uppercase tracking-wide text-muted-fg">
                                                Balance After
                                            </th>
                                            <th className="whitespace-nowrap border-b border-border bg-surface-2 px-3 py-3 text-left text-xs font-semibold uppercase tracking-wide text-muted-fg">
                                                Description
                                            </th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {counterHistory.transactions.map((transaction: any) => (
                                            <tr key={transaction.id} className="hover:bg-muted/40">
                                                <td className="border-b border-border px-3 py-3">
                                                    {new Date(transaction.created_at).toLocaleString()}
                                                </td>
                                                <td className="border-b border-border px-3 py-3">
                                                    <span
                                                        className={
                                                            transaction.amount < 0
                                                                ? 'font-semibold text-danger'
                                                                : 'font-semibold text-success'
                                                        }
                                                    >
                                                        {transaction.amount > 0 ? '+' : ''}
                                                        {transaction.amount}
                                                    </span>
                                                </td>
                                                <td className="border-b border-border px-3 py-3">{transaction.balance_after}</td>
                                                <td className="border-b border-border px-3 py-3">{transaction.description}</td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>
                        ) : (
                            <p className="py-8 text-center text-muted-fg">No counter transactions found.</p>
                        )}
                    </CardContent>
                </Card>
            )}
        </div>
    );
}
