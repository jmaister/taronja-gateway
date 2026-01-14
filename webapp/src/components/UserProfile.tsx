import { useAuth, getUserDisplayName, getUserAvatar, getUserInitials } from '../contexts/AuthContext';
import { Link } from 'react-router-dom';
import { Button } from './ui/Button';

/**
 * UserProfile component that demonstrates how to use the enhanced AuthContext
 */
export const UserProfile = () => {
    const { isAuthenticated, currentUser, isLoading, logout } = useAuth();

    if (isLoading) {
        return (
            <div className="flex items-center space-x-2 p-4">
                <div className="w-8 h-8 bg-gray-300 rounded-full animate-pulse"></div>
                <div className="h-4 bg-gray-300 rounded w-20 animate-pulse"></div>
            </div>
        );
    }

    if (!isAuthenticated || !currentUser) {
        return (
            <div className="p-4">
                <a
                    href="/login"
                    className="inline-flex h-10 items-center justify-center rounded-lg bg-primary px-4 text-sm font-medium text-primary-fg hover:bg-primary/90"
                >
                    Login
                </a>
            </div>
        );
    }

    const displayName = getUserDisplayName(currentUser);
    const avatarUrl = getUserAvatar(currentUser);
    const initials = getUserInitials(currentUser);

    return (
        <div className="flex items-center space-x-3 rounded-xl border border-border bg-surface p-4 shadow-soft">
            {/* User Avatar */}
            <div className="shrink-0">
                {avatarUrl ? (
                    <img
                        className="w-10 h-10 rounded-full object-cover"
                        src={avatarUrl}
                        alt={`${displayName}'s avatar`}
                        onError={(e) => {
                            // Fallback to initials if image fails to load
                            e.currentTarget.style.display = 'none';
                            e.currentTarget.nextElementSibling?.classList.remove('hidden');
                        }}
                    />
                ) : null}
                <div
                    className={`flex h-10 w-10 items-center justify-center rounded-full bg-primary text-sm font-medium text-primary-fg ${avatarUrl ? 'hidden' : ''}`}
                >
                    {initials}
                </div>
            </div>

            {/* User Info */}
            <div className="flex-1 min-w-0">
                <p className="truncate text-sm font-medium">
                    <Link to="/profile" className="tg-link">
                        {displayName}
                    </Link>
                </p>
                <p className="truncate text-sm text-muted-fg">
                    {currentUser.email}
                </p>
                {currentUser.provider && (
                    <p className="text-xs text-muted-fg">
                        via {currentUser.provider}
                    </p>
                )}
                {currentUser.isAdmin && (
                    <span className="mt-1 inline-flex items-center rounded-full bg-danger/10 px-2 py-0.5 text-xs font-medium text-danger">
                        Admin
                    </span>
                )}
            </div>

            {/* Logout Button */}
            <div className="shrink-0">
                <Button variant="secondary" size="sm" onClick={logout}>
                    Logout
                </Button>
            </div>
        </div>
    );
};

/**
 * Simple user badge component for navbar/header use
 */
export const UserBadge = () => {
    const { isAuthenticated, currentUser, isLoading } = useAuth();

    if (isLoading) {
        return <div className="w-8 h-8 bg-gray-300 rounded-full animate-pulse"></div>;
    }

    if (!isAuthenticated || !currentUser) {
        return null;
    }

    const displayName = getUserDisplayName(currentUser);
    const avatarUrl = getUserAvatar(currentUser);
    const initials = getUserInitials(currentUser);

    return (
        <div className="flex items-center space-x-2">
            {avatarUrl ? (
                <img
                    className="w-8 h-8 rounded-full object-cover"
                    src={avatarUrl}
                    alt={`${displayName}'s avatar`}
                    onError={(e) => {
                        e.currentTarget.style.display = 'none';
                        e.currentTarget.nextElementSibling?.classList.remove('hidden');
                    }}
                />
            ) : null}
            <div
                className={`flex h-8 w-8 items-center justify-center rounded-full bg-primary text-xs font-medium text-primary-fg ${avatarUrl ? 'hidden' : ''}`}
            >
                {initials}
            </div>
            <span className="hidden text-sm font-medium text-muted-fg sm:block">
                {displayName}
            </span>
        </div>
    );
};
