import React, { useState, useEffect } from 'react';
import { fetchUserTokens, createToken, revokeToken, TokenResponse, TokenCreateRequest, TokenCreateResponse } from '../services/api';

interface UserTokensSectionProps {
    userId: string;
}

export const UserTokensSection = ({ userId }: UserTokensSectionProps) => {
    const [tokens, setTokens] = useState<TokenResponse[]>([]);
    const [isLoadingTokens, setIsLoadingTokens] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [isCreating, setIsCreating] = useState(false);
    const [showCreateModal, setShowCreateModal] = useState(false);
    const [newToken, setNewToken] = useState<TokenCreateResponse | null>(null);

    // Form state for creating new token
    const [tokenName, setTokenName] = useState('');
    const [expiresAt, setExpiresAt] = useState('');
    const [neverExpires, setNeverExpires] = useState(false);

    // Load tokens on component mount
    useEffect(() => {
        loadTokens();
    }, [userId]);

    const loadTokens = async () => {
        try {
            setIsLoadingTokens(true);
            setError(null);
            const tokenList = await fetchUserTokens();
            setTokens(tokenList);
        } catch (err) {
            console.error('Failed to load tokens:', err);
            setError('Failed to load tokens. Please try again.');
        } finally {
            setIsLoadingTokens(false);
        }
    };

    const handleCreateToken = async (e: React.FormEvent) => {
        e.preventDefault();
        
        if (!tokenName.trim()) {
            setError('Token name is required');
            return;
        }

        try {
            setIsCreating(true);
            setError(null);

            const tokenData: TokenCreateRequest = {
                name: tokenName.trim(),
                expires_at: neverExpires ? null : (expiresAt || null),
                scopes: [] // Default to empty scopes for now
            };

            const result = await createToken(tokenData);
            setNewToken(result);
            
            // Refresh the tokens list
            await loadTokens();
            
            // Reset form
            setTokenName('');
            setExpiresAt('');
            setNeverExpires(false);
            setShowCreateModal(false);
        } catch (err) {
            console.error('Failed to create token:', err);
            setError('Failed to create token. Please try again.');
        } finally {
            setIsCreating(false);
        }
    };

    const handleRevokeToken = async (token: TokenResponse) => {
        if (!confirm(`Are you sure you want to revoke the token "${token.name}"? This action cannot be undone.`)) {
            return;
        }

        try {
            setError(null);
            await revokeToken(token.id);
            
            // Refresh the tokens list
            await loadTokens();
        } catch (err) {
            console.error('Failed to revoke token:', err);
            setError('Failed to revoke token. Please try again.');
        }
    };

    const formatDate = (dateString: string | null | undefined) => {
        if (!dateString) return 'Never';
        return new Date(dateString).toLocaleString();
    };

    const getStatusBadge = (token: TokenResponse) => {
        if (token.revoked_at) {
            return <span className="px-2 py-1 text-xs font-semibold bg-red-100 text-red-800 rounded-full">Revoked</span>;
        }
        if (token.expires_at && new Date(token.expires_at) < new Date()) {
            return <span className="px-2 py-1 text-xs font-semibold bg-yellow-100 text-yellow-800 rounded-full">Expired</span>;
        }
        if (token.is_active) {
            return <span className="px-2 py-1 text-xs font-semibold bg-green-100 text-green-800 rounded-full">Active</span>;
        }
        return <span className="px-2 py-1 text-xs font-semibold bg-gray-100 text-gray-800 rounded-full">Inactive</span>;
    };

    const closeNewTokenModal = () => {
        setNewToken(null);
    };

    return (
        <div className="mt-8">
            {/* Section Header */}
            <div className="mb-6 flex justify-between items-center border-b-2 border-gray-200 pb-2">
                <h2 className="text-xl font-semibold text-gray-800">API Tokens</h2>
                <button
                    onClick={() => setShowCreateModal(true)}
                    className="bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded text-sm"
                >
                    Create Token
                </button>
            </div>

            {/* Error Message */}
            {error && (
                <div className="mb-4 bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded">
                    {error}
                </div>
            )}

            {/* Tokens List */}
            {isLoadingTokens ? (
                <div className="text-center py-4">
                    <div className="inline-block animate-spin rounded-full h-6 w-6 border-b-2 border-blue-500"></div>
                    <p className="text-gray-600 mt-2">Loading tokens...</p>
                </div>
            ) : (tokens == null || tokens.length === 0) ? (
                <div className="text-center py-8">
                    <div className="text-gray-400 mb-4">
                        <svg className="w-12 h-12 mx-auto" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                        </svg>
                    </div>
                    <h3 className="text-lg font-medium text-gray-900 mb-2">No tokens yet</h3>
                    <p className="text-gray-600 mb-4">Create an API token to get started</p>
                    <button
                        onClick={() => setShowCreateModal(true)}
                        className="bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded"
                    >
                        Create Token
                    </button>
                </div>
            ) : (
                <div className="overflow-x-auto">
                    <table className="w-full border border-gray-200">
                        <thead className="bg-gray-50">
                            <tr>
                                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase border-b">Name</th>
                                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase border-b">Status</th>
                                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase border-b">Created</th>
                                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase border-b">Expires</th>
                                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase border-b">Usage</th>
                                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase border-b">Actions</th>
                            </tr>
                        </thead>
                        <tbody className="bg-white">
                            {tokens.map((token, index) => (
                                <tr key={token.id} className={index % 2 === 0 ? 'bg-white' : 'bg-gray-50'}>
                                    <td className="px-4 py-3 border-b">
                                        <div className="text-sm font-medium text-gray-900">{token.name}</div>
                                        <div className="text-xs text-gray-500">ID: {token.id}</div>
                                    </td>
                                    <td className="px-4 py-3 border-b">
                                        {getStatusBadge(token)}
                                    </td>
                                    <td className="px-4 py-3 border-b text-sm text-gray-500">
                                        {formatDate(token.created_at)}
                                    </td>
                                    <td className="px-4 py-3 border-b text-sm text-gray-500">
                                        {formatDate(token.expires_at)}
                                    </td>
                                    <td className="px-4 py-3 border-b text-sm text-gray-500">
                                        {token.usage_count} times
                                    </td>
                                    <td className="px-4 py-3 border-b text-sm">
                                        {token.is_active && !token.revoked_at && (
                                            <button 
                                                onClick={() => handleRevokeToken(token)}
                                                className="text-red-600 hover:text-red-900 text-sm"
                                            >
                                                Revoke
                                            </button>
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
                <div className="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50">
                    <div className="bg-white rounded-lg shadow-xl max-w-md w-full">
                        <div className="px-6 py-4 border-b border-gray-200">
                            <h3 className="text-lg font-semibold text-gray-900">Create New Token</h3>
                        </div>
                        <form onSubmit={handleCreateToken} className="p-6">
                            <div className="mb-4">
                                <label htmlFor="tokenName" className="block text-sm font-medium text-gray-700 mb-2">
                                    Token Name
                                </label>
                                <input
                                    type="text"
                                    id="tokenName"
                                    value={tokenName}
                                    onChange={(e) => setTokenName(e.target.value)}
                                    className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                                    placeholder="e.g., My API Token"
                                    required
                                />
                            </div>

                            <div className="mb-4">
                                <div className="flex items-center mb-2">
                                    <input
                                        type="checkbox"
                                        id="neverExpires"
                                        checked={neverExpires}
                                        onChange={(e) => setNeverExpires(e.target.checked)}
                                        className="mr-2"
                                    />
                                    <label htmlFor="neverExpires" className="text-sm font-medium text-gray-700">
                                        Never expires
                                    </label>
                                </div>
                                
                                {!neverExpires && (
                                    <div>
                                        <label htmlFor="expiresAt" className="block text-sm font-medium text-gray-700 mb-2">
                                            Expires At
                                        </label>
                                        <input
                                            type="datetime-local"
                                            id="expiresAt"
                                            value={expiresAt}
                                            onChange={(e) => setExpiresAt(e.target.value)}
                                            className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
                                            min={new Date().toISOString().slice(0, 16)}
                                        />
                                    </div>
                                )}
                            </div>

                            <div className="flex justify-end space-x-3">
                                <button
                                    type="button"
                                    onClick={() => setShowCreateModal(false)}
                                    className="px-4 py-2 text-sm font-medium text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-md"
                                >
                                    Cancel
                                </button>
                                <button
                                    type="submit"
                                    disabled={isCreating}
                                    className="px-4 py-2 text-sm font-medium text-white bg-blue-500 hover:bg-blue-700 rounded-md disabled:opacity-50"
                                >
                                    {isCreating ? 'Creating...' : 'Create Token'}
                                </button>
                            </div>
                        </form>
                    </div>
                </div>
            )}

            {/* New Token Display Modal */}
            {newToken && (
                <div className="fixed inset-0 bg-gray-600 bg-opacity-50 flex items-center justify-center p-4 z-50">
                    <div className="bg-white rounded-lg shadow-xl max-w-md w-full">
                        <div className="px-6 py-4 border-b border-gray-200">
                            <h3 className="text-lg font-semibold text-gray-900">Token Created Successfully</h3>
                        </div>
                        <div className="p-6">
                            <div className="mb-4 p-4 bg-yellow-50 border border-yellow-200 rounded-md">
                                <p className="text-sm text-yellow-800 font-medium mb-2">
                                    ⚠️ Important: Copy your token now
                                </p>
                                <p className="text-sm text-yellow-700">
                                    This is the only time you'll be able to see the full token. Make sure to copy it and store it securely.
                                </p>
                            </div>
                            
                            <div className="mb-4">
                                <label className="block text-sm font-medium text-gray-700 mb-2">
                                    Your new token:
                                </label>
                                <div className="relative">
                                    <input
                                        type="text"
                                        value={newToken.token}
                                        readOnly
                                        className="w-full px-3 py-2 border border-gray-300 rounded-md bg-gray-50 font-mono text-sm"
                                    />
                                    <button
                                        onClick={() => navigator.clipboard.writeText(newToken.token)}
                                        className="absolute right-2 top-2 text-blue-600 hover:text-blue-800"
                                        title="Copy to clipboard"
                                    >
                                        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                                        </svg>
                                    </button>
                                </div>
                            </div>

                            <div className="mb-4">
                                <p className="text-sm text-gray-600">
                                    <strong>Token Name:</strong> {newToken.token_info.name}
                                </p>
                                <p className="text-sm text-gray-600">
                                    <strong>Created:</strong> {formatDate(newToken.token_info.created_at)}
                                </p>
                                <p className="text-sm text-gray-600">
                                    <strong>Expires:</strong> {formatDate(newToken.token_info.expires_at)}
                                </p>
                            </div>

                            <div className="flex justify-end">
                                <button
                                    onClick={closeNewTokenModal}
                                    className="px-4 py-2 text-sm font-medium text-white bg-blue-500 hover:bg-blue-700 rounded-md"
                                >
                                    I've copied the token
                                </button>
                            </div>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
};
