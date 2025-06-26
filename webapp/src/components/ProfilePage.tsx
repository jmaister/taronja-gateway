import { useAuth, getUserDisplayName, getUserAvatar, getUserInitials } from '../contexts/AuthContext';

/**
 * ProfilePage component - Full page view for user profile
 */
export const ProfilePage = () => {
    const { isAuthenticated, currentUser, isLoading, logout } = useAuth();

    if (isLoading) {
        return (
            <div className="min-h-screen flex items-center justify-center">
                <div className="text-center">
                    <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500 mx-auto mb-4"></div>
                    <p className="text-gray-600">Loading profile...</p>
                </div>
            </div>
        );
    }

    if (!isAuthenticated || !currentUser) {
        return (
            <div className="min-h-screen flex items-center justify-center">
                <div className="text-center">
                    <h1 className="text-2xl font-bold text-gray-900 mb-4">Access Denied</h1>
                    <p className="text-gray-600 mb-6">You must be logged in to view your profile.</p>
                    <a
                        href="/login"
                        className="bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded"
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
        <div className="w-full p-6">
            {/* Page Header */}
            <div className="mb-8">
                <h1 className="text-3xl font-bold text-gray-900">My Profile</h1>
                <p className="text-gray-600 mt-2">Manage your account settings and preferences</p>
            </div>

            {/* Profile Card */}
            <div className="bg-white rounded-lg shadow-lg overflow-hidden">
                {/* Header Section */}
                <div className="bg-gradient-to-r from-blue-500 to-purple-600 px-6 py-8">
                    <div className="flex items-center space-x-6">
                        {/* Avatar */}
                        <div className="flex-shrink-0">
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
                                className={`w-24 h-24 rounded-full bg-white text-blue-500 flex items-center justify-center text-2xl font-bold border-4 border-white shadow-lg ${avatarUrl ? 'hidden' : ''}`}
                            >
                                {initials}
                            </div>
                        </div>

                        {/* User Info */}
                        <div className="text-white">
                            <h2 className="text-2xl font-bold">{displayName}</h2>
                            <p className="text-blue-100 text-lg">{currentUser.email}</p>
                            {currentUser.provider && (
                                <p className="text-blue-200 text-sm mt-1">
                                    Authenticated via {currentUser.provider}
                                </p>
                            )}
                            {currentUser.isAdmin && (
                                <span className="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium bg-red-500 text-white mt-2">
                                    Administrator
                                </span>
                            )}
                        </div>
                    </div>
                </div>

                {/* Content Section */}
                <div className="p-6">
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                        {/* Account Information */}
                        <div className="space-y-4">
                            <h3 className="text-lg font-semibold text-gray-900 border-b pb-2">Account Information</h3>
                            
                            <div>
                                <label className="block text-sm font-medium text-gray-700">Username</label>
                                <p className="mt-1 text-sm text-gray-900">{currentUser.username || 'Not set'}</p>
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-gray-700">Email</label>
                                <p className="mt-1 text-sm text-gray-900">{currentUser.email}</p>
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-gray-700">Given Name</label>
                                <p className="mt-1 text-sm text-gray-900">{currentUser.givenName || 'Not set'}</p>
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-gray-700">Family Name</label>
                                <p className="mt-1 text-sm text-gray-900">{currentUser.familyName || 'Not set'}</p>
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-gray-700">Authentication Provider</label>
                                <p className="mt-1 text-sm text-gray-900">{currentUser.provider || 'Local'}</p>
                            </div>
                        </div>

                        {/* Account Status */}
                        <div className="space-y-4">
                            <h3 className="text-lg font-semibold text-gray-900 border-b pb-2">Account Status</h3>
                            
                            <div>
                                <label className="block text-sm font-medium text-gray-700">Account Type</label>
                                <p className="mt-1">
                                    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
                                        currentUser.isAdmin 
                                            ? 'bg-red-100 text-red-800' 
                                            : 'bg-green-100 text-green-800'
                                    }`}>
                                        {currentUser.isAdmin ? 'Administrator' : 'Standard User'}
                                    </span>
                                </p>
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-gray-700">Status</label>
                                <p className="mt-1">
                                    <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                                        Active
                                    </span>
                                </p>
                            </div>

                            {currentUser.timestamp && (
                                <div>
                                    <label className="block text-sm font-medium text-gray-700">Last Login</label>
                                    <p className="mt-1 text-sm text-gray-900">
                                        {new Date(currentUser.timestamp).toLocaleDateString()}
                                    </p>
                                </div>
                            )}
                        </div>
                    </div>
                </div>

                {/* Actions Section */}
                <div className="bg-gray-50 px-6 py-4 border-t">
                    <div className="flex justify-between items-center">
                        <div className="text-sm text-gray-600">
                            Last updated: {new Date().toLocaleDateString()}
                        </div>
                        <button
                            onClick={logout}
                            className="bg-red-500 hover:bg-red-600 text-white font-medium py-2 px-4 rounded transition-colors"
                        >
                            Logout
                        </button>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default ProfilePage;
