import { useAuth, getUserDisplayName, getUserAvatar, getUserInitials } from '../contexts/AuthContext';
import { Link } from 'react-router-dom';

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
                    className="bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded"
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
        <div className="flex items-center space-x-3 p-4 bg-white rounded-lg shadow">
            {/* User Avatar */}
            <div className="flex-shrink-0">
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
                    className={`w-10 h-10 rounded-full bg-blue-500 text-white flex items-center justify-center text-sm font-medium ${avatarUrl ? 'hidden' : ''}`}
                >
                    {initials}
                </div>
            </div>

            {/* User Info */}
            <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-gray-900 truncate">
                    <Link to="/profile" className="hover:text-blue-600 hover:underline">
                        {displayName}
                    </Link>
                </p>
                <p className="text-sm text-gray-500 truncate">
                    {currentUser.email}
                </p>
                {currentUser.provider && (
                    <p className="text-xs text-gray-400">
                        via {currentUser.provider}
                    </p>
                )}
                {currentUser.isAdmin && (
                    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-red-100 text-red-800">
                        Admin
                    </span>
                )}
            </div>

            {/* Logout Button */}
            <div className="flex-shrink-0">
                <button
                    onClick={logout}
                    className="bg-gray-100 hover:bg-gray-200 text-gray-700 font-medium py-1 px-3 rounded text-sm"
                >
                    Logout
                </button>
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
                className={`w-8 h-8 rounded-full bg-blue-500 text-white flex items-center justify-center text-xs font-medium ${avatarUrl ? 'hidden' : ''}`}
            >
                {initials}
            </div>
            <span className="text-sm font-medium text-gray-700 hidden sm:block">
                {displayName}
            </span>
        </div>
    );
};
