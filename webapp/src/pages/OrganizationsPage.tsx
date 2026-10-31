import { useState } from 'react';
import { Link } from 'react-router-dom';
import type { FormEvent } from 'react';
import { QueryState } from '../components/QueryState';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { UserPicker } from '../components/UserPicker';
import { FormField } from '../components/ui/FormField';
import { PageHeader } from '../components/ui/PageHeader';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import {
    useOrganizations,
    useCreateOrganization,
    useDeleteOrganization,
} from '../services/organizations';

function CreateOrganizationForm() {
    const create = useCreateOrganization();
    const [name, setName] = useState('');
    const [ownerId, setOwnerId] = useState('');
    const [pickerKey, setPickerKey] = useState(0);

    const onSubmit = (e: FormEvent) => {
        e.preventDefault();
        if (!name) return;
        create.mutate(
            { name, ownerUserId: ownerId || undefined },
            {
                onSuccess: () => {
                    setName('');
                    setOwnerId('');
                    setPickerKey(k => k + 1);
                },
            },
        );
    };

    return (
        <Card>
            <CardHeader>
                <h2 className="text-lg font-semibold">New organization</h2>
            </CardHeader>
            <CardContent>
                <form onSubmit={onSubmit} className="grid gap-4 md:grid-cols-[2fr_2fr_auto] md:items-end">
                    <FormField label="Name" htmlFor="org-name" required>
                        <Input
                            id="org-name"
                            value={name}
                            onChange={e => setName(e.target.value)}
                            placeholder="Acme Inc"
                        />
                    </FormField>
                    <FormField label="Owner (optional)" htmlFor="org-owner">
                        <UserPicker key={pickerKey} id="org-owner" value={ownerId} onChange={setOwnerId} />
                    </FormField>
                    <Button type="submit" disabled={!name || create.isPending}>
                        {create.isPending ? 'Creating…' : 'Create'}
                    </Button>
                </form>
                {create.error && <p className="mt-3 text-sm text-danger">{create.error.message}</p>}
            </CardContent>
        </Card>
    );
}


export function OrganizationsPage() {
    const organizations = useOrganizations();
    const deleteOrg = useDeleteOrganization();
    const [pendingDelete, setPendingDelete] = useState<string | null>(null);

    return (
        <div className="space-y-6">
            <PageHeader
                title="Organizations"
                description="Manage organizations and their members"
            />

            <CreateOrganizationForm />

            <QueryState isLoading={organizations.isLoading} error={organizations.error} onRetry={() => organizations.refetch()}>
                <Card>
                    {organizations.data && organizations.data.length === 0 ? (
                        <CardContent className="text-sm text-muted-fg">No organizations yet.</CardContent>
                    ) : (
                        <div className="overflow-x-auto">
                            <table className="w-full text-sm">
                                <thead>
                                    <tr className="border-b border-border text-left text-muted-fg">
                                        <th className="px-4 py-3 font-medium">Name</th>
                                        <th className="px-4 py-3 font-medium">Members</th>
                                        <th className="px-4 py-3 font-medium">Created</th>
                                        <th className="px-4 py-3" />
                                    </tr>
                                </thead>
                                <tbody>
                                    {organizations.data?.map(org => (
                                        <tr key={org.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                                            <td className="px-4 py-3 font-medium">
                                                <span className="flex items-center gap-3">
                                                    {org.logoUrl && (
                                                        <img src={org.logoUrl} alt="" className="h-6 w-6 rounded object-contain" />
                                                    )}
                                                    {org.name}
                                                </span>
                                            </td>
                                            <td className="px-4 py-3 text-muted-fg">
                                                {org.memberCount}
                                            </td>
                                            <td className="px-4 py-3 text-muted-fg">
                                                {new Date(org.createdAt).toLocaleDateString()}
                                            </td>
                                            <td className="px-4 py-3 text-right space-x-2">
                                                <Link
                                                    to={`/organizations/${org.id}`}
                                                    className="inline-flex items-center rounded-md px-3 py-1.5 text-sm hover:bg-muted"
                                                >
                                                    View
                                                </Link>
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    onClick={() => setPendingDelete(org.id)}
                                                    className="text-danger"
                                                >
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
                title="Delete organization"
                description={`Delete this organization? Its subscriptions will remain, but members will no longer be part of it.`}
                confirmLabel="Delete"
                variant="danger"
                isLoading={deleteOrg.isPending}
                onCancel={() => setPendingDelete(null)}
                onConfirm={() => {
                    if (pendingDelete) {
                        deleteOrg.mutate(pendingDelete, { onSettled: () => setPendingDelete(null) });
                    }
                }}
            />
        </div>
    );
}
