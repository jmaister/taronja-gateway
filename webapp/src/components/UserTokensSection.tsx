import React, { useState } from 'react';
import { useUserTokens, useCreateToken, useRevokeToken } from '../services/services';
import { TokenCreateRequest, TokenCreateResponse, TokenResponse } from '@/apiclient';
import { Button } from './ui/Button';
import { Card, CardContent, CardHeader } from './ui/Card';
import { FormField } from './ui/FormField';
import { Input } from './ui/Input';
import { StatusPill } from './ui/StatusPill';

interface UserTokensSectionProps {
    userId: string;
}

export const UserTokensSection = ({ userId }: UserTokensSectionProps) => {
    const [showCreateModal, setShowCreateModal] = useState(false);
    const [newToken, setNewToken] = useState<TokenCreateResponse | null>(null);
    const [copied, setCopied] = useState(false);

    // Form state for creating new token
    const [tokenName, setTokenName] = useState('');
    const [expiresAt, setExpiresAt] = useState('');
    const [neverExpires, setNeverExpires] = useState(false);

    // TanStack Query hooks
    const { data: tokens, isLoading: isLoadingTokens, error } = useUserTokens(userId);
    const createTokenMutation = useCreateToken();
    const revokeTokenMutation = useRevokeToken();

    const handleCreateToken = async (e: React.FormEvent) => {
        e.preventDefault();
        
        if (!tokenName.trim()) {
            return;
        }

        const tokenData: TokenCreateRequest = {
            name: tokenName.trim(),
            expires_at: neverExpires ? null : (expiresAt || null),
            scopes: [] // Default to empty scopes for now
        };

        try {
            const result = await createTokenMutation.mutateAsync({ userId, tokenData });
            setNewToken(result);
            
            // Reset form
            setTokenName('');
            setExpiresAt('');
            setNeverExpires(false);
            setShowCreateModal(false);
        } catch (err) {
            console.error('Failed to create token:', err);
        }
    };

    const handleRevokeToken = async (tokenId: string, tokenName: string) => {
        if (!confirm(`Are you sure you want to revoke the token "${tokenName}"? This action cannot be undone.`)) {
            return;
        }

        try {
            await revokeTokenMutation.mutateAsync(tokenId);
        } catch (err) {
            console.error('Failed to revoke token:', err);
        }
    };

    const formatDate = (dateString: string | null | undefined) => {
        if (!dateString) return 'Never';
        return new Date(dateString).toLocaleString();
    };

    const getStatusBadge = (token: TokenResponse) => {
        if (token.revoked_at) {
            return <StatusPill variant="danger">Revoked</StatusPill>;
        }
        if (token.expires_at && new Date(token.expires_at) < new Date()) {
            return <StatusPill variant="warning">Expired</StatusPill>;
        }
        if (token.is_active) {
            return <StatusPill variant="success">Active</StatusPill>;
        }
        return <StatusPill>Inactive</StatusPill>;
    };

    const closeNewTokenModal = () => {
        setNewToken(null);
    };

    // Show error message from mutations or queries
    const errorMessage = error || createTokenMutation.error || revokeTokenMutation.error;

    return (
        <div className="mt-8">
            {/* Section Header */}
            <div className="mb-6 flex flex-wrap items-center justify-between gap-4 border-b border-border pb-3">
                <div>
                    <h2 className="text-xl font-semibold">API Tokens</h2>
                    <p className="mt-1 text-sm text-muted-fg">Create and manage API tokens for this user.</p>
                </div>
                <Button size="sm" onClick={() => setShowCreateModal(true)}>
                    Create Token
                </Button>
            </div>

            {/* Error Message */}
            {errorMessage && (
                <div className="mb-4 rounded-lg border border-danger/30 bg-danger/5 px-4 py-3 text-danger">
                    {String(errorMessage)}
                </div>
            )}

            {/* Tokens List */}
            {isLoadingTokens ? (
                <div className="py-4 text-center">
                    <div className="inline-block h-6 w-6 animate-spin rounded-full border-b-2 border-primary"></div>
                    <p className="mt-2 text-muted-fg">Loading tokens...</p>
                </div>
            ) : tokens == null || tokens.length === 0 ? (
                <div className="py-8 text-center">
                    <div className="mb-4 text-muted-fg">
                        <svg className="mx-auto h-12 w-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                strokeWidth={1}
                                d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"
                            />
                        </svg>
                    </div>
                    <h3 className="mb-2 text-lg font-medium">No tokens yet</h3>
                    <p className="mb-4 text-muted-fg">Create an API token to get started.</p>
                    <Button onClick={() => setShowCreateModal(true)}>Create Token</Button>
                </div>
            ) : (
                <div className="overflow-x-auto rounded-lg border border-border bg-surface">
                    <table className="w-full border-collapse text-sm">
                        <thead className="bg-surface-2">
                            <tr>
                                <th className="border-b border-border px-4 py-3 text-left text-xs font-semibold uppercase text-muted-fg">
                                    Name
                                </th>
                                <th className="border-b border-border px-4 py-3 text-left text-xs font-semibold uppercase text-muted-fg">
                                    Status
                                </th>
                                <th className="border-b border-border px-4 py-3 text-left text-xs font-semibold uppercase text-muted-fg">
                                    Created
                                </th>
                                <th className="border-b border-border px-4 py-3 text-left text-xs font-semibold uppercase text-muted-fg">
                                    Expires
                                </th>
                                <th className="border-b border-border px-4 py-3 text-left text-xs font-semibold uppercase text-muted-fg">
                                    Usage
                                </th>
                                <th className="border-b border-border px-4 py-3 text-left text-xs font-semibold uppercase text-muted-fg">
                                    Actions
                                </th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-border">
                            {tokens.map((token) => (
                                <tr key={token.id} className="hover:bg-muted/40">
                                    <td className="px-4 py-3">
                                        <div className="text-sm font-medium">{token.name}</div>
                                        <div className="text-xs text-muted-fg">ID: {token.id}</div>
                                    </td>
                                    <td className="px-4 py-3">{getStatusBadge(token)}</td>
                                    <td className="px-4 py-3 text-sm text-muted-fg">{formatDate(token.created_at)}</td>
                                    <td className="px-4 py-3 text-sm text-muted-fg">{formatDate(token.expires_at)}</td>
                                    <td className="px-4 py-3 text-sm text-muted-fg">{token.usage_count} times</td>
                                    <td className="px-4 py-3 text-sm">
                                        {token.is_active && !token.revoked_at && (
                                            <Button
                                                variant="danger"
                                                size="sm"
                                                onClick={() => handleRevokeToken(token.id, token.name)}
                                                disabled={revokeTokenMutation.isPending}
                                            >
                                                {revokeTokenMutation.isPending ? 'Revoking...' : 'Revoke'}
                                            </Button>
                                        )}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}

            {/* Create Token Modal */}
            {showCreateModal && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
                    <Card className="w-full max-w-md">
                        <CardHeader>
                            <h3 className="text-lg font-semibold">Create New Token</h3>
                        </CardHeader>
                        <CardContent>
                            <form onSubmit={handleCreateToken} className="space-y-4">
                                <FormField label="Token Name" htmlFor="tokenName" required>
                                    <Input
                                        type="text"
                                        id="tokenName"
                                        value={tokenName}
                                        onChange={(e) => setTokenName(e.target.value)}
                                        placeholder="e.g., My API Token"
                                        required
                                    />
                                </FormField>

                                <div className="space-y-2">
                                    <label htmlFor="neverExpires" className="flex items-center gap-2 text-sm font-medium text-muted-fg">
                                        <input
                                            type="checkbox"
                                            id="neverExpires"
                                            checked={neverExpires}
                                            onChange={(e) => setNeverExpires(e.target.checked)}
                                            className="h-4 w-4 rounded border-border"
                                        />
                                        Never expires
                                    </label>

                                    {!neverExpires && (
                                        <FormField label="Expires At" htmlFor="expiresAt">
                                            <Input
                                                type="datetime-local"
                                                id="expiresAt"
                                                value={expiresAt}
                                                onChange={(e) => setExpiresAt(e.target.value)}
                                                min={new Date().toISOString().slice(0, 16)}
                                            />
                                        </FormField>
                                    )}
                                </div>

                                <div className="flex justify-end gap-3">
                                    <Button type="button" variant="secondary" onClick={() => setShowCreateModal(false)}>
                                        Cancel
                                    </Button>
                                    <Button type="submit" disabled={createTokenMutation.isPending}>
                                        {createTokenMutation.isPending ? 'Creating...' : 'Create Token'}
                                    </Button>
                                </div>
                            </form>
                        </CardContent>
                    </Card>
                </div>
            )}

            {/* New Token Display Modal */}
            {newToken && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
                    <Card className="w-full max-w-md">
                        <CardHeader>
                            <h3 className="text-lg font-semibold">Token Created Successfully</h3>
                        </CardHeader>
                        <CardContent>
                            <div className="mb-4 rounded-md border border-warning/30 bg-warning/5 p-4">
                                <p className="mb-2 text-sm font-medium text-warning">
                                    ⚠️ Important: Copy your token now
                                </p>
                                <p className="text-sm text-warning/80">
                                    This is the only time you'll be able to see the full token. Make sure to copy it and store it securely.
                                </p>
                            </div>

                            <div className="mb-4 space-y-2">
                                <label className="block text-sm font-medium text-muted-fg">Your new token:</label>
                                <Input
                                    type="text"
                                    value={newToken.token}
                                    readOnly
                                    className={`font-mono ${copied ? 'border-success bg-success/10' : ''}`}
                                />
                                <Button
                                    variant="secondary"
                                    size="sm"
                                    onClick={async () => {
                                        await navigator.clipboard.writeText(newToken.token);
                                        setCopied(true);
                                        setTimeout(() => setCopied(false), 800);
                                    }}
                                    title="Copy to clipboard"
                                >
                                    Copy token
                                </Button>
                            </div>

                            <div className="mb-4 space-y-1 text-sm text-muted-fg">
                                <p>
                                    <strong className="text-fg">Token Name:</strong> {newToken.token_info.name}
                                </p>
                                <p>
                                    <strong className="text-fg">Created:</strong> {formatDate(newToken.token_info.created_at)}
                                </p>
                                <p>
                                    <strong className="text-fg">Expires:</strong> {formatDate(newToken.token_info.expires_at)}
                                </p>
                            </div>

                            <div className="flex justify-end">
                                <Button onClick={closeNewTokenModal}>I've copied the token</Button>
                            </div>
                        </CardContent>
                    </Card>
                </div>
            )}
        </div>
    );
};
