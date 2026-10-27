import { useState } from 'react';
import type { ChangeEvent, FormEvent } from 'react';
import { Link, useParams } from 'react-router-dom';
import { QueryState } from '../components/QueryState';
import { Card, CardContent } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { UserPicker } from '../components/UserPicker';
import { FormField } from '../components/ui/FormField';
import { PageHeader } from '../components/ui/PageHeader';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import {
    useOrganization,
    useSetOrganizationMember,
    useRemoveOrganizationMember,
    useSetOrganizationLogo,
    useDeleteOrganizationLogo,
} from '../services/organizations';

const MAX_LOGO_BYTES = 512 * 1024;

function OrganizationLogoEditor({ orgId, name, logoUrl }: { orgId: string; name: string; logoUrl?: string }) {
    const setLogo = useSetOrganizationLogo();
    const removeLogo = useDeleteOrganizationLogo();
    const [tooLarge, setTooLarge] = useState(false);

    const onFile = (e: ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        e.target.value = '';
        if (!file) return;
        setTooLarge(file.size > MAX_LOGO_BYTES);
        if (file.size > MAX_LOGO_BYTES) return;
        setLogo.mutate({ orgId, file });
    };

    return (
        <div>
            <h4 className="font-semibold mb-4">Logo</h4>
            <div className="flex items-center gap-4">
                {logoUrl ? (
                    <img src={logoUrl} alt={`${name} logo`} className="h-16 w-16 rounded border border-border object-contain" />
                ) : (
                    <div className="h-16 w-16 rounded border border-dashed border-border flex items-center justify-center text-xs text-muted-fg">
                        No logo
                    </div>
                )}
                <div className="space-y-2">
                    <label className="block text-sm">
                        <span className="sr-only">Upload logo</span>
                        <input
                            type="file"
                            accept="image/png,image/jpeg,image/webp"
                            aria-label="Upload logo"
                            onChange={onFile}
                            disabled={setLogo.isPending}
                            className="text-sm"
                        />
                    </label>
                    {logoUrl && (
                        <Button
                            variant="ghost"
                            size="sm"
                            className="text-danger"
                            disabled={removeLogo.isPending}
                            onClick={() => removeLogo.mutate(orgId)}
                        >
                            Remove logo
                        </Button>
                    )}
                </div>
            </div>
            <p className="mt-2 text-xs text-muted-fg">PNG, JPEG or WebP, up to 512 KiB.</p>
            {tooLarge && <p className="mt-2 text-sm text-danger">The image is larger than 512 KiB.</p>}
            {setLogo.error && <p className="mt-2 text-sm text-danger">{setLogo.error.message}</p>}
            {removeLogo.error && <p className="mt-2 text-sm text-danger">{removeLogo.error.message}</p>}
        </div>
    );
}

export function OrganizationDetailPage() {
    const { id: orgId = '' } = useParams();
    const details = useOrganization(orgId);
    const addMember = useSetOrganizationMember();
    const removeMember = useRemoveOrganizationMember();
    const [userId, setUserId] = useState('');
    const [pickerKey, setPickerKey] = useState(0);
    const [role, setRole] = useState<'owner' | 'member'>('member');
    const [memberToRemove, setMemberToRemove] = useState<string | null>(null);
    const name = details.data?.name ?? 'Organization';

    const onAddMember = (e: FormEvent) => {
        e.preventDefault();
        if (!userId) return;
        addMember.mutate(
            { orgId, userId, body: { role } },
            {
                onSuccess: () => {
                    setUserId('');
                    setPickerKey(k => k + 1);
                    setRole('member');
                },
            },
        );
    };

    return (
        <div className="space-y-6">
            <Link to="/organizations" className="text-sm text-muted-fg hover:underline">
                ← Organizations
            </Link>
            <PageHeader title={name} description="Logo and members" />

            <QueryState isLoading={details.isLoading} error={details.error} onRetry={() => details.refetch()}>
                <div className="grid gap-6 lg:grid-cols-2">
                    <Card>
                        <CardContent className="space-y-6">
                            <OrganizationLogoEditor orgId={orgId} name={name} logoUrl={details.data?.logoUrl || undefined} />
                            <div>
                                <h4 className="font-semibold mb-4">Add Member</h4>
                                <form onSubmit={onAddMember} className="space-y-3">
                                    <FormField label="User" htmlFor="member-user-id" required>
                                        <UserPicker key={pickerKey} id="member-user-id" value={userId} onChange={setUserId} />
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
                        </CardContent>
                    </Card>
                    <Card>
                        <CardContent>
                            <h4 className="font-semibold mb-4">Members</h4>
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
                        </CardContent>
                    </Card>
                </div>
            </QueryState>

            <ConfirmDialog
                open={memberToRemove !== null}
                title="Remove member"
                description={`Remove ${memberToRemove} from ${name}?`}
                confirmLabel="Remove"
                variant="danger"
                isLoading={removeMember.isPending}
                onCancel={() => setMemberToRemove(null)}
                onConfirm={() => {
                    if (memberToRemove) {
                        removeMember.mutate(
                            { orgId, userId: memberToRemove },
                            { onSettled: () => setMemberToRemove(null) },
                        );
                    }
                }}
            />
        </div>
    );
}
