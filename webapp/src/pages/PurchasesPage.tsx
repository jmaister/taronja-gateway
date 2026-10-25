import { useState } from 'react';
import { QueryState } from '../components/QueryState';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { FormField } from '../components/ui/FormField';
import { PageHeader } from '../components/ui/PageHeader';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import {
    useBillingPurchases,
    useRefundBillingPurchase,
} from '../services/billing';

const formatter = new Intl.NumberFormat('es-ES', {
    style: 'currency',
    currency: 'EUR',
});

function formatPrice(cents: number) {
    return formatter.format(cents / 100);
}

function eurosTocents(euros: number | string) {
    const num = typeof euros === 'string' ? parseFloat(euros) : euros;
    return Math.round(num * 100);
}

export function PurchasesPage() {
    const [filters, setFilters] = useState({
        plan: '',
        status: '',
        since: '',
    });

    const purchases = useBillingPurchases(
        Object.values(filters).some(v => v) ? { plan: filters.plan || undefined, status: filters.status || undefined, since: filters.since || undefined } : undefined,
    );

    const refund = useRefundBillingPurchase();
    const [pendingRefund, setPendingRefund] = useState<{ id: string; amount: number; hasSubscription: boolean } | null>(null);
    const [refundAmount, setRefundAmount] = useState('');
    const [cancelSubscription, setCancelSubscription] = useState(false);
    const [showRefundForm, setShowRefundForm] = useState(false);

    const handleRefund = (id: string) => {
        const refundData = pendingRefund;
        if (!refundData) return;

        const body = {
            amountCents: refundAmount ? eurosTocents(refundAmount) : undefined,
            cancelSubscription,
        };

        refund.mutate({ id, body });
        setPendingRefund(null);
        setRefundAmount('');
        setCancelSubscription(false);
        setShowRefundForm(false);
    };

    return (
        <div className="space-y-6">
            <PageHeader
                title="Purchases"
                description="Manage one-time payments and subscription first payments"
            />

            <Card>
                <CardHeader>
                    <h3 className="font-semibold">Filters</h3>
                </CardHeader>
                <CardContent>
                    <div className="grid gap-4 grid-cols-1 md:grid-cols-3">
                        <FormField label="Plan" htmlFor="filter-plan">
                            <Input
                                id="filter-plan"
                                value={filters.plan}
                                onChange={e => setFilters({ ...filters, plan: e.target.value })}
                                placeholder="Plan key"
                            />
                        </FormField>
                        <FormField label="Status" htmlFor="filter-status">
                            <select
                                id="filter-status"
                                value={filters.status}
                                onChange={e => setFilters({ ...filters, status: e.target.value })}
                                className="w-full px-3 py-2 border border-border rounded-md bg-bg"
                            >
                                <option value="">All statuses</option>
                                <option value="pending">Pending</option>
                                <option value="paid">Paid</option>
                                <option value="refunded">Refunded</option>
                            </select>
                        </FormField>
                        <FormField label="Since" htmlFor="filter-since">
                            <Input
                                id="filter-since"
                                type="date"
                                value={filters.since}
                                onChange={e => setFilters({ ...filters, since: e.target.value })}
                            />
                        </FormField>
                    </div>
                </CardContent>
            </Card>

            <QueryState isLoading={purchases.isLoading} error={purchases.error} onRetry={() => purchases.refetch()}>
                <Card>
                    {purchases.data && purchases.data.length === 0 ? (
                        <CardContent className="text-sm text-muted-fg">No purchases found.</CardContent>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                                <thead>
                                    <tr className="border-b border-border text-left text-muted-fg">
                                        <th className="px-4 py-3 font-medium">Subject</th>
                                        <th className="px-4 py-3 font-medium">Plan</th>
                                        <th className="px-4 py-3 font-medium">Amount</th>
                                        <th className="px-4 py-3 font-medium">Refunded</th>
                                        <th className="px-4 py-3 font-medium">Status</th>
                                        <th className="px-4 py-3 font-medium">Paid At</th>
                                        <th className="px-4 py-3 font-medium">Shipping</th>
                                        <th className="px-4 py-3" />
                                    </tr>
                                </thead>
                                <tbody>
                                    {purchases.data?.map(purchase => (
                                        <tr key={purchase.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                                            <td className="px-4 py-3 font-mono text-xs">{purchase.subject}</td>
                                            <td className="px-4 py-3 font-medium">{purchase.planKey}</td>
                                            <td className="px-4 py-3 font-mono">{formatPrice(purchase.amountCents)}</td>
                                            <td className="px-4 py-3 font-mono text-xs">
                                                {purchase.refundedCents > 0 ? formatPrice(purchase.refundedCents) : '—'}
                                            </td>
                                            <td className="px-4 py-3">
                                                <span className={`px-2 py-1 rounded text-xs font-medium ${
                                                    purchase.status === 'paid' ? 'bg-success/10 text-success' :
                                                    purchase.status === 'refunded' ? 'bg-info/10 text-info' :
                                                    'bg-warning/10 text-warning'
                                                }`}>
                                                    {purchase.status}
                                                </span>
                                            </td>
                                            <td className="px-4 py-3 text-muted-fg text-xs">
                                                {purchase.paidAt ? new Date(purchase.paidAt).toLocaleDateString() : '—'}
                                            </td>
                                            <td className="px-4 py-3 text-muted-fg text-xs">
                                                {purchase.shipping ? (
                                                    <div>
                                                        {purchase.shipping.name}
                                                        <br />
                                                        {purchase.shipping.line1}
                                                        {purchase.shipping.line2 && (
                                                            <>
                                                                <br />
                                                                {purchase.shipping.line2}
                                                            </>
                                                        )}
                                                    </div>
                                                ) : '—'}
                                            </td>
                                            <td className="px-4 py-3 text-right">
                                                {purchase.status === 'paid' && (
                                                    <Button
                                                        variant="ghost"
                                                        size="sm"
                                                        onClick={() => setPendingRefund({
                                                            id: purchase.id,
                                                            amount: purchase.amountCents,
                                                            hasSubscription: !!purchase.subscriptionId,
                                                        })}
                                                    >
                                                        Refund
                                                    </Button>
                                                )}
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </Card>
            </QueryState>

            {showRefundForm && pendingRefund && (
                <Card className="border-danger/20 bg-danger/5">
                    <CardHeader>
                        <h3 className="font-semibold">Refund Details</h3>
                    </CardHeader>
                    <CardContent className="space-y-4">
                        <FormField label="Amount in € (empty = full refund)" htmlFor="refund-amount">
                            <Input
                                id="refund-amount"
                                type="number"
                                step="0.01"
                                value={refundAmount}
                                onChange={e => setRefundAmount(e.target.value)}
                                placeholder={formatPrice(pendingRefund.amount)}
                            />
                        </FormField>
                        {pendingRefund.hasSubscription && (
                            <label className="flex items-center gap-2 text-sm">
                                <input
                                    type="checkbox"
                                    checked={cancelSubscription}
                                    onChange={e => setCancelSubscription(e.target.checked)}
                                />
                                Also cancel the subscription
                            </label>
                        )}
                        <div className="flex gap-2">
                            <Button
                                variant="danger"
                                onClick={() => handleRefund(pendingRefund.id)}
                                disabled={refund.isPending}
                            >
                                {refund.isPending ? 'Refunding…' : 'Confirm Refund'}
                            </Button>
                            <Button
                                variant="ghost"
                                onClick={() => {
                                    setShowRefundForm(false);
                                    setPendingRefund(null);
                                    setRefundAmount('');
                                    setCancelSubscription(false);
                                }}
                            >
                                Cancel
                            </Button>
                        </div>
                    </CardContent>
                </Card>
            )}

            <ConfirmDialog
                open={pendingRefund !== null && !showRefundForm}
                title="Refund purchase"
                description={`Refund this purchase${pendingRefund?.hasSubscription ? ' (has associated subscription)' : ''}?`}
                confirmLabel="Continue"
                variant="danger"
                isLoading={false}
                onCancel={() => {
                    setPendingRefund(null);
                    setRefundAmount('');
                    setCancelSubscription(false);
                }}
                onConfirm={() => {
                    setShowRefundForm(true);
                }}
            />
        </div>
    );
}
