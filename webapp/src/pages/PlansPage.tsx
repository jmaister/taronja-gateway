import { useState } from 'react';
import type { FormEvent } from 'react';
import type { BillingPlan, BillingPlanInput } from '@/apiclient';
import { QueryState } from '../components/QueryState';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { FormField } from '../components/ui/FormField';
import { PageHeader } from '../components/ui/PageHeader';
import {
    useBillingPlans,
    useCreateBillingPlan,
    useUpdateBillingPlan,
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

function CreatePlanForm() {
    const create = useCreateBillingPlan();
    const [formData, setFormData] = useState({
        key: '',
        name: '',
        description: '',
        kind: 'subscription' as 'subscription' | 'one_time',
        interval: 'month' as 'month' | 'year',
        amountEuros: '',
        features: '',
        subject: 'either' as 'user' | 'org' | 'either',
        trialDays: 0,
        graceDays: 7,
        setupFeeEuros: '',
        setupFeeName: '',
        startsOn: 'on_payment' as 'on_payment' | 'on_activation',
        accessDays: 0,
        collectShipping: false,
        group: '',
    });

    const onSubmit = (e: FormEvent) => {
        e.preventDefault();
        if (!formData.key || !formData.name || !formData.amountEuros) return;

        const planInput: BillingPlanInput = {
            key: formData.key,
            name: formData.name,
            description: formData.description,
            kind: formData.kind,
            interval: formData.kind === 'subscription' ? formData.interval : undefined,
            amountCents: eurosTocents(formData.amountEuros),
            features: formData.features.split(',').map(f => f.trim()).filter(Boolean),
            subject: formData.subject,
            trialDays: formData.trialDays,
            graceDays: formData.graceDays,
            setupFeeCents: formData.setupFeeEuros ? eurosTocents(formData.setupFeeEuros) : undefined,
            setupFeeName: formData.setupFeeName,
            starts: formData.startsOn,
            accessDays: formData.kind === 'one_time' ? formData.accessDays : undefined,
            collectShipping: formData.collectShipping,
            group: formData.group,
        };

        create.mutate(planInput, {
            onSuccess: () => {
                setFormData({
                    key: '',
                    name: '',
                    description: '',
                    kind: 'subscription',
                    interval: 'month',
                    amountEuros: '',
                    features: '',
                    subject: 'either',
                    trialDays: 0,
                    graceDays: 7,
                    setupFeeEuros: '',
                    setupFeeName: '',
                    startsOn: 'on_payment',
                    accessDays: 0,
                    collectShipping: false,
                    group: '',
                });
            },
        });
    };

    return (
        <Card>
            <CardHeader>
                <h2 className="text-lg font-semibold">New plan</h2>
            </CardHeader>
            <CardContent>
                <form onSubmit={onSubmit} className="grid gap-4 grid-cols-1 md:grid-cols-2">
                    <FormField label="Key" htmlFor="plan-key" required>
                        <Input
                            id="plan-key"
                            value={formData.key}
                            onChange={e => setFormData({ ...formData, key: e.target.value })}
                            placeholder="academia-monthly"
                        />
                    </FormField>
                    <FormField label="Name" htmlFor="plan-name" required>
                        <Input
                            id="plan-name"
                            value={formData.name}
                            onChange={e => setFormData({ ...formData, name: e.target.value })}
                            placeholder="Academia Monthly"
                        />
                    </FormField>
                    <FormField label="Description" htmlFor="plan-desc">
                        <Input
                            id="plan-desc"
                            value={formData.description}
                            onChange={e => setFormData({ ...formData, description: e.target.value })}
                            placeholder="Description"
                        />
                    </FormField>
                    <FormField label="Kind" htmlFor="plan-kind" required>
                        <select
                            id="plan-kind"
                            value={formData.kind}
                            onChange={e => setFormData({ ...formData, kind: e.target.value as any })}
                            className="w-full px-3 py-2 border border-border rounded-md bg-bg"
                        >
                            <option value="subscription">Subscription</option>
                            <option value="one_time">One-time</option>
                        </select>
                    </FormField>

                    {formData.kind === 'subscription' && (
                        <FormField label="Interval" htmlFor="plan-interval" required>
                            <select
                                id="plan-interval"
                                value={formData.interval}
                                onChange={e => setFormData({ ...formData, interval: e.target.value as any })}
                                className="w-full px-3 py-2 border border-border rounded-md bg-bg"
                            >
                                <option value="month">Month</option>
                                <option value="year">Year</option>
                            </select>
                        </FormField>
                    )}

                    <FormField label="Amount (€)" htmlFor="plan-amount" required>
                        <Input
                            id="plan-amount"
                            type="number"
                            step="0.01"
                            value={formData.amountEuros}
                            onChange={e => setFormData({ ...formData, amountEuros: e.target.value })}
                            placeholder="9.99"
                        />
                    </FormField>

                    <FormField label="Subject" htmlFor="plan-subject" required>
                        <select
                            id="plan-subject"
                            value={formData.subject}
                            onChange={e => setFormData({ ...formData, subject: e.target.value as any })}
                            className="w-full px-3 py-2 border border-border rounded-md bg-bg"
                        >
                            <option value="user">User</option>
                            <option value="org">Organization</option>
                            <option value="either">Either</option>
                        </select>
                    </FormField>

                    <FormField label="Features (comma-separated)" htmlFor="plan-features">
                        <Input
                            id="plan-features"
                            value={formData.features}
                            onChange={e => setFormData({ ...formData, features: e.target.value })}
                            placeholder="feature1, feature2"
                        />
                    </FormField>

                    <FormField label="Trial Days" htmlFor="plan-trial">
                        <Input
                            id="plan-trial"
                            type="number"
                            value={formData.trialDays}
                            onChange={e => setFormData({ ...formData, trialDays: parseInt(e.target.value) || 0 })}
                        />
                    </FormField>

                    <FormField label="Grace Days" htmlFor="plan-grace">
                        <Input
                            id="plan-grace"
                            type="number"
                            value={formData.graceDays}
                            onChange={e => setFormData({ ...formData, graceDays: parseInt(e.target.value) || 0 })}
                        />
                    </FormField>

                    <FormField label="Setup Fee (€)" htmlFor="plan-setup-fee">
                        <Input
                            id="plan-setup-fee"
                            type="number"
                            step="0.01"
                            value={formData.setupFeeEuros}
                            onChange={e => setFormData({ ...formData, setupFeeEuros: e.target.value })}
                        />
                    </FormField>

                    <FormField label="Setup Fee Name" htmlFor="plan-setup-name">
                        <Input
                            id="plan-setup-name"
                            value={formData.setupFeeName}
                            onChange={e => setFormData({ ...formData, setupFeeName: e.target.value })}
                        />
                    </FormField>

                    {formData.kind === 'subscription' && (
                        <FormField label="Starts" htmlFor="plan-starts" required>
                            <select
                                id="plan-starts"
                                value={formData.startsOn}
                                onChange={e => setFormData({ ...formData, startsOn: e.target.value as any })}
                                className="w-full px-3 py-2 border border-border rounded-md bg-bg"
                            >
                                <option value="on_payment">On Payment</option>
                                <option value="on_activation">On Activation</option>
                            </select>
                        </FormField>
                    )}

                    {formData.kind === 'one_time' && (
                        <FormField label="Access Days" htmlFor="plan-access-days">
                            <Input
                                id="plan-access-days"
                                type="number"
                                value={formData.accessDays}
                                onChange={e => setFormData({ ...formData, accessDays: parseInt(e.target.value) || 0 })}
                            />
                        </FormField>
                    )}

                    <FormField label="Group" htmlFor="plan-group">
                        <Input
                            id="plan-group"
                            value={formData.group}
                            onChange={e => setFormData({ ...formData, group: e.target.value })}
                            placeholder="group-name"
                        />
                    </FormField>

                    <label className="flex items-center gap-2 h-10">
                        <input
                            type="checkbox"
                            checked={formData.collectShipping}
                            onChange={e => setFormData({ ...formData, collectShipping: e.target.checked })}
                        />
                        <span className="text-sm">Collect Shipping</span>
                    </label>

                    <div className="md:col-span-2">
                        <Button type="submit" disabled={!formData.key || !formData.name || !formData.amountEuros || create.isPending}>
                            {create.isPending ? 'Creating…' : 'Create'}
                        </Button>
                    </div>
                </form>
                {create.error && <p className="mt-3 text-sm text-danger">{create.error.message}</p>}
            </CardContent>
        </Card>
    );
}

export function PlansPage() {
    const [showArchived, setShowArchived] = useState(false);
    const plans = useBillingPlans({ archived: showArchived });
    const update = useUpdateBillingPlan();

    const handleCopyPayLink = (key: string) => {
        const payLink = `${window.location.origin}/_/pay/${key}`;
        navigator.clipboard.writeText(payLink);
    };

    const handleArchive = (plan: BillingPlan) => {
        update.mutate({
            key: plan.key,
            body: { active: false },
        });
    };

    const handleUnarchive = (plan: BillingPlan) => {
        update.mutate({
            key: plan.key,
            body: { active: true },
        });
    };

    return (
        <div className="space-y-6">
            <PageHeader
                title="Billing Plans"
                description="Manage subscription and one-time payment plans"
            />

            <CreatePlanForm />

            <div className="flex items-center gap-2">
                <label className="flex items-center gap-2">
                    <input
                        type="checkbox"
                        checked={showArchived}
                        onChange={e => setShowArchived(e.target.checked)}
                    />
                    <span className="text-sm">Show Archived</span>
                </label>
            </div>

            <QueryState isLoading={plans.isLoading} error={plans.error} onRetry={() => plans.refetch()}>
                <Card>
                    {plans.data && plans.data.length === 0 ? (
                        <CardContent className="text-sm text-muted-fg">
                            {showArchived ? 'No archived plans.' : 'No active plans.'}
                        </CardContent>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                                <thead>
                                    <tr className="border-b border-border text-left text-muted-fg">
                                        <th className="px-4 py-3 font-medium">Key</th>
                                        <th className="px-4 py-3 font-medium">Name</th>
                                        <th className="px-4 py-3 font-medium">Kind</th>
                                        <th className="px-4 py-3 font-medium">Interval</th>
                                        <th className="px-4 py-3 font-medium">Price</th>
                                        <th className="px-4 py-3 font-medium">Features</th>
                                        <th className="px-4 py-3" />
                                    </tr>
                                </thead>
                                <tbody>
                                    {plans.data?.map(plan => (
                                        <tr key={plan.key} className="border-b border-border last:border-0 hover:bg-muted/30">
                                            <td className="px-4 py-3 font-mono">{plan.key}</td>
                                            <td className="px-4 py-3 font-medium">{plan.name}</td>
                                            <td className="px-4 py-3 text-muted-fg">{plan.kind}</td>
                                            <td className="px-4 py-3 text-muted-fg">
                                                {plan.interval ? plan.interval : '—'}
                                            </td>
                                            <td className="px-4 py-3 font-mono">{formatPrice(plan.amountCents)}</td>
                                            <td className="px-4 py-3 text-muted-fg text-xs">
                                                {plan.features.join(', ') || '—'}
                                            </td>
                                            <td className="px-4 py-3 text-right space-x-2">
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    onClick={() => handleCopyPayLink(plan.key)}
                                                    title="Copy pay link"
                                                >
                                                    📋
                                                </Button>
                                                {plan.active ? (
                                                    <Button
                                                        variant="ghost"
                                                        size="sm"
                                                        onClick={() => handleArchive(plan)}
                                                        className="text-warning"
                                                        disabled={update.isPending}
                                                    >
                                                        Archive
                                                    </Button>
                                                ) : (
                                                    <Button
                                                        variant="ghost"
                                                        size="sm"
                                                        onClick={() => handleUnarchive(plan)}
                                                        className="text-success"
                                                        disabled={update.isPending}
                                                    >
                                                        Unarchive
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

            {update.error && <Card className="bg-danger/10 border-danger/20"><CardContent className="text-sm text-danger">{update.error.message}</CardContent></Card>}
        </div>
    );
}
