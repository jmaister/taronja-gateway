import { useState } from 'react';
import type { FormEvent } from 'react';
import type { Organization } from '@/apiclient';
import { QueryState } from '../components/QueryState';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { FormField } from '../components/ui/FormField';
import { PageHeader } from '../components/ui/PageHeader';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import {
    useOrganizations,
    useCreateOrganization,
    useDeleteOrganization,
    useOrganization,
    useSetOrganizationMember,
    useRemoveOrganizationMember,
} from '../services/organizations';

function CreateOrganizationForm() {
    const create = useCreateOrganization();
    const [name, setName] = useState('');
    const [ownerId, setOwnerId] = useState('');

    const onSubmit = (e: FormEvent) => {
        e.preventDefault();
        if (!name) return;
        create.mutate(
            { name, ownerUserId: ownerId || undefined },
            {
                onSuccess: () => {
                    setName('');
                    setOwnerId('');
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
                    <FormField label="Owner User ID (optional)" htmlFor="org-owner">
                        <Input
                            id="org-owner"
                            value={ownerId}
                            onChange={e => setOwnerId(e.target.value)}
                            placeholder="user-123"
                        />
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

interface DetailPanelProps {
    organization: Organization;
    onClose: () => void;
}

function OrganizationDetailPanel({ organization, onClose }: DetailPanelProps) {
    const details = useOrganization(organization.id);
    const addMember = useSetOrganizationMember();
    const removeMember = useRemoveOrganizationMember();
    const [userId, setUserId] = useState('');
    const [role, setRole] = useState<'owner' | 'member'>('member');
    const [memberToRemove, setMemberToRemove] = useState<string | null>(null);

    const onAddMember = (e: FormEvent) => {
        e.preventDefault();
        if (!userId) return;
        addMember.mutate(
            { orgId: organization.id, userId, body: { role } },
            {
                onSuccess: () => {
                    setUserId('');
                    setRole('member');
                },
            },
        );
    };

    return (
        <div className="fixed inset-0 z-40 bg-black/40 flex items-start justify-end pt-16">
            <div className="w-96 bg-bg border-l border-border shadow-lg rounded-l-lg overflow-y-auto max-h-[calc(100vh-64px)]">
                <div className="p-6 border-b border-border flex items-center justify-between">
                    <h3 className="text-lg font-semibold">{organization.name}</h3>
                    <Button variant="ghost" size="sm" onClick={onClose}>
                        ✕
                    </Button>
                </div>

                <div className="p-6 space-y-6">
                    <div>
                        <h4 className="font-semibold mb-4">Add Member</h4>
                        <form onSubmit={onAddMember} className="space-y-3">
                            <FormField label="User ID" htmlFor="member-user-id" required>
                                <Input
                                    id="member-user-id"
                                    value={userId}
                                    onChange={e => setUserId(e.target.value)}
                                    placeholder="user-123"
                                />
                            </FormField>
                            <FormField label="Role" htmlFor="member-role" required>
                                <select
                                    id="member-role"
                                    value={role}
                                    onChange={e => setRole(e.target.value as 'owner' | 'member')}
                                    className="w-full px-3 py-2 border border-border rounded-md bg-bg"
                                >
                                    <option value="member">Member</option>
                                    <option value="owner">Owner</option>
                                </select>
                            </FormField>
                            <Button type="submit" disabled={!userId || addMember.isPending} className="w-full">
                                {addMember.isPending ? 'Adding…' : 'Add Member'}
                            </Button>
                        </form>
                        {addMember.error && <p className="mt-2 text-sm text-danger">{addMember.error.message}</p>}
                    </div>

                    <div>
                        <h4 className="font-semibold mb-4">Members</h4>
                        <QueryState
                            isLoading={details.isLoading}
                            error={details.error}
                            onRetry={() => details.refetch()}
                        >
                            {details.data?.members && details.data.members.length > 0 ? (
                                <div className="space-y-2">
                                    {details.data.members.map(member => (
                                        <div
                                            key={member.userId}
                                            className="flex items-center justify-between p-3 bg-muted/30 rounded"
                                        >
                                            <div className="text-sm">
                                                <div className="font-medium">{member.userId}</div>
                                                <div className="text-xs text-muted-fg">{member.role}</div>
                                            </div>
                                            <Button
                                                variant="ghost"
                                                size="sm"
                                                onClick={() => setMemberToRemove(member.userId)}
                                                className="text-danger"
                                            >
                                                Remove
                                            </Button>
                                        </div>
                                    ))}
                                </div>
                            ) : (
                                <p className="text-sm text-muted-fg">No members</p>
                            )}
                        </QueryState>
                    </div>
                </div>
            </div>

            <ConfirmDialog
                open={memberToRemove !== null}
                title="Remove member"
                description={`Remove ${memberToRemove} from ${organization.name}?`}
                confirmLabel="Remove"
                variant="danger"
                isLoading={removeMember.isPending}
                onCancel={() => setMemberToRemove(null)}
                onConfirm={() => {
                    if (memberToRemove) {
                        removeMember.mutate(
                            { orgId: organization.id, userId: memberToRemove },
                            { onSettled: () => setMemberToRemove(null) },
                        );
                    }
                }}
            />
        </div>
    );
}

export function OrganizationsPage() {
    const organizations = useOrganizations();
    const deleteOrg = useDeleteOrganization();
    const [selectedOrg, setSelectedOrg] = useState<Organization | null>(null);
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
                                            <td className="px-4 py-3 font-medium">{org.name}</td>
                                            <td className="px-4 py-3 text-muted-fg">
                                                {org.memberCount}
                                            </td>
                                            <td className="px-4 py-3 text-muted-fg">
                                                {new Date(org.createdAt).toLocaleDateString()}
                                            </td>
                                            <td className="px-4 py-3 text-right space-x-2">
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    onClick={() => setSelectedOrg(org)}
                                                >
                                                    View
                                                </Button>
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

            {selectedOrg && <OrganizationDetailPanel organization={selectedOrg} onClose={() => setSelectedOrg(null)} />}

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
