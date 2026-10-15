import { useState } from 'react';
import type { FormEvent } from 'react';
import type { FeatureFlag } from '@/apiclient';
import { QueryState } from '../components/QueryState';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { FormField } from '../components/ui/FormField';
import { PageHeader } from '../components/ui/PageHeader';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import {
    useFeatureFlags,
    useCreateFeatureFlag,
    useUpdateFeatureFlag,
    useDeleteFeatureFlag,
} from '../services/featureFlags';

const keyPattern = /^[a-z0-9][a-z0-9._-]{0,99}$/;

function FlagSwitch({ flag }: { flag: FeatureFlag }) {
    const update = useUpdateFeatureFlag();
    return (
        <button
            type="button"
            role="switch"
            aria-checked={flag.enabled}
            aria-label={`${flag.enabled ? 'Disable' : 'Enable'} ${flag.key}`}
            disabled={update.isPending}
            onClick={() => update.mutate({ key: flag.key, body: { enabled: !flag.enabled } })}
            className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50 ${
                flag.enabled ? 'bg-success' : 'bg-muted'
            }`}
        >
            <span
                className={`inline-block h-5 w-5 rounded-full bg-white shadow transition-transform ${
                    flag.enabled ? 'translate-x-5' : 'translate-x-0.5'
                }`}
            />
        </button>
    );
}

function CreateFlagForm() {
    const create = useCreateFeatureFlag();
    const [key, setKey] = useState('');
    const [description, setDescription] = useState('');
    const [enabled, setEnabled] = useState(false);

    const keyError = key !== '' && !keyPattern.test(key)
        ? "Use lowercase letters, digits, '-', '_' or '.', starting with a letter or digit"
        : undefined;

    const onSubmit = (e: FormEvent) => {
        e.preventDefault();
        if (!key || keyError) return;
        create.mutate(
            { key, description, enabled, type: 'boolean' },
            {
                onSuccess: () => {
                    setKey('');
                    setDescription('');
                    setEnabled(false);
                },
            },
        );
    };

    return (
        <Card>
            <CardHeader>
                <h2 className="text-lg font-semibold">New flag</h2>
            </CardHeader>
            <CardContent>
                <form onSubmit={onSubmit} className="grid gap-4 md:grid-cols-[1fr_2fr_auto_auto] md:items-end">
                    <FormField label="Key" htmlFor="flag-key" required error={keyError}>
                        <Input id="flag-key" value={key} onChange={e => setKey(e.target.value)} placeholder="new-checkout" />
                    </FormField>
                    <FormField label="Description" htmlFor="flag-description">
                        <Input id="flag-description" value={description} onChange={e => setDescription(e.target.value)} placeholder="What does this flag control?" />
                    </FormField>
                    <label className="flex h-10 items-center gap-2 text-sm">
                        <input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} />
                        On
                    </label>
                    <Button type="submit" disabled={!key || !!keyError || create.isPending}>
                        {create.isPending ? 'Creating…' : 'Create'}
                    </Button>
                </form>
                {create.error && <p className="mt-3 text-sm text-danger">{create.error.message}</p>}
            </CardContent>
        </Card>
    );
}

export function FeatureFlagsPage() {
    const flags = useFeatureFlags();
    const remove = useDeleteFeatureFlag();
    const [pendingDelete, setPendingDelete] = useState<string | null>(null);

    return (
        <div className="space-y-6">
            <PageHeader
                title="Feature Flags"
                description="Turn features on and off without redeploying. Frontends read GET /_/flags; proxied backends receive the enabled keys in the X-Feature-Flags header."
            />

            <CreateFlagForm />

            <QueryState isLoading={flags.isLoading} error={flags.error} onRetry={() => flags.refetch()}>
                <Card>
                    {flags.data && flags.data.length === 0 ? (
                        <CardContent className="text-sm text-muted-fg">No feature flags yet.</CardContent>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                                <thead>
                                    <tr className="border-b border-border text-left text-muted-fg">
                                        <th className="px-4 py-3 font-medium">Key</th>
                                        <th className="px-4 py-3 font-medium">Description</th>
                                        <th className="px-4 py-3 font-medium">Updated</th>
                                        <th className="px-4 py-3 font-medium text-center">State</th>
                                        <th className="px-4 py-3" />
                                    </tr>
                                </thead>
                                <tbody>
                                    {flags.data?.map(flag => (
                                        <tr key={flag.key} className="border-b border-border last:border-0 hover:bg-muted/30">
                                            <td className="px-4 py-3 font-mono">{flag.key}</td>
                                            <td className="px-4 py-3 text-muted-fg">{flag.description || '—'}</td>
                                            <td className="px-4 py-3 text-muted-fg">{new Date(flag.updatedAt).toLocaleString()}</td>
                                            <td className="px-4 py-3">
                                                <div className="flex justify-center">
                                                    <FlagSwitch flag={flag} />
                                                </div>
                                            </td>
                                            <td className="px-4 py-3 text-right">
                                                <Button variant="ghost" size="sm" onClick={() => setPendingDelete(flag.key)}>
                                                    Delete
                                                </Button>
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
                open={pendingDelete !== null}
                title="Delete feature flag"
                description={`Delete "${pendingDelete}"? Code that still reads it will see it as missing.`}
                confirmLabel="Delete"
                variant="danger"
                isLoading={remove.isPending}
                onCancel={() => setPendingDelete(null)}
                onConfirm={() => {
                    if (pendingDelete) {
                        remove.mutate(pendingDelete, { onSettled: () => setPendingDelete(null) });
                    }
                }}
            />
        </div>
    );
}
