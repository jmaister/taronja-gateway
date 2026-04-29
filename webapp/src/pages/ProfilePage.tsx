import { getUserAvatar, getUserDisplayName, getUserInitials, useTaronjaAuth } from 'taronja-gateway-react';
import { Button } from '../components/ui/Button';
import { Card, CardContent } from '../components/ui/Card';

/**
 * ProfilePage component - Full page view for user profile
 */
export const ProfilePage = () => {
    const { isAuthenticated, currentUser, isLoading, logout } = useTaronjaAuth();

    if (isLoading) {
        return (
            <div className="min-h-screen flex items-center justify-center">
                <div className="text-center">
                    <div className="mx-auto mb-4 h-12 w-12 animate-spin rounded-full border-b-2 border-primary"></div>
                    <p className="text-muted-fg">Loading profile...</p>
                </div>
            </div>
        );
    }

    if (!isAuthenticated || !currentUser) {
        return (
            <div className="min-h-screen flex items-center justify-center">
                <div className="text-center">
                    <h1 className="mb-4 text-2xl font-bold">Access Denied</h1>
                    <p className="mb-6 text-muted-fg">You must be logged in to view your profile.</p>
                    <a
                        href="/login"
                        className="inline-flex h-10 items-center justify-center rounded-lg bg-primary px-4 text-sm font-medium text-primary-fg hover:bg-primary/90"
                    >
                        Login
                    </a>
                </div>
            </div>
        );
    }

    const displayName = getUserDisplayName(currentUser);
    const avatarUrl = getUserAvatar(currentUser);
    const initials = getUserInitials(currentUser);

    return (
        <div className="mx-auto max-w-7xl space-y-6">
            {/* Page Header */}
            <div>
                <h1 className="text-2xl font-semibold tracking-tight">My Profile</h1>
                <p className="mt-1 text-sm text-muted-fg">Manage your account settings and preferences</p>
            </div>

            <Card className="overflow-hidden">
                <div className="bg-gradient-to-r from-primary to-primary/60 px-6 py-8">
                    <div className="flex items-center gap-6">
                        <div className="shrink-0">
                            {avatarUrl ? (
                                <img
                                    className="w-24 h-24 rounded-full object-cover border-4 border-white shadow-lg"
                                    src={avatarUrl}
                                    alt={`${displayName}'s avatar`}
                                    onError={(e) => {
                                        e.currentTarget.style.display = 'none';
                                        e.currentTarget.nextElementSibling?.classList.remove('hidden');
                                    }}
                                />
                            ) : null}
                            <div
                                className={`flex h-24 w-24 items-center justify-center rounded-full bg-white text-2xl font-bold text-primary shadow-lg ${avatarUrl ? 'hidden' : ''}`}
                            >
                                {initials}
                            </div>
                        </div>

                        {/* User Info */}
                        <div className="text-primary-fg">
                            <h2 className="text-2xl font-bold">{displayName}</h2>
                            <p className="text-primary-fg/80 text-lg">{currentUser.email}</p>
                            {currentUser.provider && (
                                <p className="mt-1 text-sm text-primary-fg/70">
                                    Authenticated via {currentUser.provider}
                                </p>
                            )}
                            {currentUser.isAdmin && (
                                <span className="mt-2 inline-flex items-center rounded-full bg-black/20 px-3 py-1 text-sm font-medium">
                                    Administrator
                                </span>
                            )}
                        </div>
                    </div>
                </div>

                {/* Content Section */}
                <CardContent>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                        {/* Account Information */}
                        <div className="space-y-4">
                            <h3 className="border-b border-border pb-2 text-base font-semibold">Account Information</h3>
                            
                            <div>
                                <label className="block text-sm font-medium text-muted-fg">Username</label>
                                <p className="mt-1 text-sm">{currentUser.username || 'Not set'}</p>
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-muted-fg">Email</label>
                                <p className="mt-1 text-sm">{currentUser.email}</p>
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-muted-fg">Given Name</label>
                                <p className="mt-1 text-sm">{currentUser.givenName || 'Not set'}</p>
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-muted-fg">Family Name</label>
                                <p className="mt-1 text-sm">{currentUser.familyName || 'Not set'}</p>
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-muted-fg">Authentication Provider</label>
                                <p className="mt-1 text-sm">{currentUser.provider || 'Local'}</p>
                            </div>
                        </div>

                        {/* Account Status */}
                        <div className="space-y-4">
                            <h3 className="border-b border-border pb-2 text-base font-semibold">Account Status</h3>
                            
                            <div>
                                <label className="block text-sm font-medium text-muted-fg">Account Type</label>
                                <p className="mt-1">
                                    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                                        currentUser.isAdmin 
                                            ? 'bg-danger/10 text-danger' 
                                            : 'bg-success/10 text-success'
                                    }`}>
                                        {currentUser.isAdmin ? 'Administrator' : 'Standard User'}
                                    </span>
                                </p>
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-muted-fg">Status</label>
                                <p className="mt-1">
                                    <span className="inline-flex items-center rounded-full bg-success/10 px-2.5 py-0.5 text-xs font-medium text-success">
                                        Active
                                    </span>
                                </p>
                            </div>

                            {currentUser.timestamp && (
                                <div>
                                    <label className="block text-sm font-medium text-muted-fg">Last Login</label>
                                    <p className="mt-1 text-sm">
                                        {new Date(currentUser.timestamp).toLocaleDateString()}
                                    </p>
                                </div>
                            )}
                        </div>
                    </div>
                </CardContent>

                {/* Actions Section */}
                <div className="bg-surface-2 px-6 py-4 border-t border-border">
                    <div className="flex justify-between items-center">
                        <div className="text-sm text-muted-fg">
                            Last updated: {new Date().toLocaleDateString()}
                        </div>
                        <Button variant="danger" onClick={() => void logout()}>
                            Logout
                        </Button>
                    </div>
                </div>
            </Card>
        </div>
    );
};

export default ProfilePage;
