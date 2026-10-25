import { useState } from 'react';
import { QueryState } from '../components/QueryState';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { FormField } from '../components/ui/FormField';
import { PageHeader } from '../components/ui/PageHeader';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import {
    useBillingSubscriptions,
    useActivateBillingSubscription,
    useCancelBillingSubscription,
} from '../services/billing';

export function SubscriptionsPage() {
    const [filters, setFilters] = useState({
        subject: '',
        status: '',
        plan: '',
    });

    const subscriptions = useBillingSubscriptions(
        Object.values(filters).some(v => v) ? { subject: filters.subject || undefined, status: filters.status || undefined, plan: filters.plan || undefined } : undefined,
    );

    const activate = useActivateBillingSubscription();
    const cancel = useCancelBillingSubscription();
    const [pendingAction, setPendingAction] = useState<{ type: 'activate' | 'cancel'; id: string } | null>(null);
    const [cancelAtPeriodEnd, setCancelAtPeriodEnd] = useState(true);
    const [showCancelOptions, setShowCancelOptions] = useState(false);

    const handleActivate = (id: string) => {
        activate.mutate(id);
        setPendingAction(null);
    };

    const handleCancel = (id: string) => {
        cancel.mutate({
            id,
            body: { atPeriodEnd: cancelAtPeriodEnd },
        });
        setPendingAction(null);
    };

    return (
        <div className="space-y-6">
            <PageHeader
                title="Subscriptions"
                description="Manage active subscriptions"
            />

            <Card>
                <CardHeader>
                    <h3 className="font-semibold">Filters</h3>
                </CardHeader>
                <CardContent>
                    <div className="grid gap-4 grid-cols-1 md:grid-cols-3">
                        <FormField label="Subject" htmlFor="filter-subject">
                            <Input
                                id="filter-subject"
                                value={filters.subject}
                                onChange={e => setFilters({ ...filters, subject: e.target.value })}
                                placeholder="user:123 or org:456"
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
                                <option value="awaiting_activation">Awaiting Activation</option>
                                <option value="trialing">Trialing</option>
                                <option value="active">Active</option>
                                <option value="past_due">Past Due</option>
                                <option value="canceled">Canceled</option>
                            </select>
                        </FormField>
                        <FormField label="Plan" htmlFor="filter-plan">
                            <Input
                                id="filter-plan"
                                value={filters.plan}
                                onChange={e => setFilters({ ...filters, plan: e.target.value })}
                                placeholder="Plan key"
                            />
                        </FormField>
                    </div>
                </CardContent>
            </Card>

            <QueryState isLoading={subscriptions.isLoading} error={subscriptions.error} onRetry={() => subscriptions.refetch()}>
                <Card>
                    {subscriptions.data && subscriptions.data.length === 0 ? (
                        <CardContent className="text-sm text-muted-fg">No subscriptions found.</CardContent>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                                <thead>
                                    <tr className="border-b border-border text-left text-muted-fg">
                                        <th className="px-4 py-3 font-medium">Subject</th>
                                        <th className="px-4 py-3 font-medium">Plan</th>
                                        <th className="px-4 py-3 font-medium">Status</th>
                                        <th className="px-4 py-3 font-medium">Current Period End</th>
                                        <th className="px-4 py-3 font-medium">Trial End</th>
                                        <th className="px-4 py-3 font-medium">Cancel At</th>
                                        <th className="px-4 py-3 font-medium">Grace Until</th>
                                        <th className="px-4 py-3" />
                                    </tr>
                                </thead>
                                <tbody>
                                    {subscriptions.data?.map(sub => (
                                        <tr key={sub.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                                            <td className="px-4 py-3 font-mono text-xs">{sub.subject}</td>
                                            <td className="px-4 py-3 font-medium">{sub.planKey}</td>
                                            <td className="px-4 py-3">
                                                <span className={`px-2 py-1 rounded text-xs font-medium ${
                                                    sub.status === 'active' ? 'bg-success/10 text-success' :
                                                    sub.status === 'trialing' ? 'bg-info/10 text-info' :
                                                    sub.status === 'past_due' ? 'bg-warning/10 text-warning' :
                                                    sub.status === 'canceled' ? 'bg-danger/10 text-danger' :
                                                    'bg-muted/10 text-muted-fg'
                                                }`}>
                                                    {sub.status}
                                                </span>
                                            </td>
                                            <td className="px-4 py-3 text-muted-fg text-xs">
                                                {sub.currentPeriodEnd ? new Date(sub.currentPeriodEnd).toLocaleDateString() : '—'}
                                            </td>
                                            <td className="px-4 py-3 text-muted-fg text-xs">
                                                {sub.trialEnd ? new Date(sub.trialEnd).toLocaleDateString() : '—'}
                                            </td>
                                            <td className="px-4 py-3 text-muted-fg text-xs">
                                                {sub.cancelAtPeriodEnd ? 'Yes' : '—'}
                                            </td>
                                            <td className="px-4 py-3 text-muted-fg text-xs">
                                                {sub.graceUntil ? new Date(sub.graceUntil).toLocaleDateString() : '—'}
                                            </td>
                                            <td className="px-4 py-3 text-right space-x-2">
                                                {sub.status === 'awaiting_activation' && (
                                                    <Button
                                                        variant="ghost"
                                                        size="sm"
                                                        onClick={() => setPendingAction({ type: 'activate', id: sub.id })}
                                                    >
                                                        Activate
                                                    </Button>
                                                )}
                                                {(sub.status === 'active' || sub.status === 'trialing' || sub.status === 'past_due') && (
                                                    <Button
                                                        variant="ghost"
                                                        size="sm"
                                                        onClick={() => setPendingAction({ type: 'cancel', id: sub.id })}
                                                        className="text-danger"
                                                    >
                                                        Cancel
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

            <ConfirmDialog
                open={pendingAction?.type === 'activate' || false}
                title="Activate subscription"
                description="Activate this subscription?"
                confirmLabel="Activate"
                isLoading={activate.isPending}
                onCancel={() => setPendingAction(null)}
                onConfirm={() => {
                    if (pendingAction?.type === 'activate') {
                        handleActivate(pendingAction.id);
                    }
                }}
            />

            {showCancelOptions && pendingAction?.type === 'cancel' && (
                <Card className="border-danger/20 bg-danger/5">
                    <CardHeader>
                        <h3 className="font-semibold">Cancel Subscription</h3>
                    </CardHeader>
                    <CardContent className="space-y-4">
                        <label className="flex items-center gap-2 text-sm">
                            <input
                                type="checkbox"
                                checked={cancelAtPeriodEnd}
                                onChange={e => setCancelAtPeriodEnd(e.target.checked)}
                            />
                            Cancel at end of period (uncheck for immediate cancel)
                        </label>
                        <div className="flex gap-2">
                            <Button
                                variant="danger"
                                onClick={() => {
                                    if (pendingAction?.type === 'cancel') {
                                        handleCancel(pendingAction.id);
                                    }
                                }}
                                disabled={cancel.isPending}
                            >
                                {cancel.isPending ? 'Canceling…' : 'Confirm Cancel'}
                            </Button>
                            <Button
                                variant="ghost"
                                onClick={() => {
                                    setShowCancelOptions(false);
                                    setPendingAction(null);
                                }}
                            >
                                Back
                            </Button>
                        </div>
                    </CardContent>
                </Card>
            )}

            <ConfirmDialog
                open={pendingAction?.type === 'cancel' && !showCancelOptions || false}
                title="Cancel subscription"
                description="Cancel this subscription?"
                confirmLabel="Next"
                variant="danger"
                isLoading={false}
                onCancel={() => setPendingAction(null)}
                onConfirm={() => {
                    setShowCancelOptions(true);
                }}
            />
        </div>
    );
}
