import { useAuth, getUserDisplayName, getUserAvatar, getUserInitials } from '../contexts/AuthContext';

/**
 * Dashboard page that demonstrates the AuthContext usage
 */
export const DashboardPage = () => {
    const { currentUser, isLoading, checkUserSession, isAdmin, isAuthenticated } = useAuth();

    if (isLoading) {
        return (
            <div className="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
                <div className="animate-pulse">
                    <div className="h-8 bg-gray-300 rounded w-1/4 mb-6"></div>
                    <div className="space-y-4">
                        <div className="h-4 bg-gray-300 rounded w-3/4"></div>
                        <div className="h-4 bg-gray-300 rounded w-1/2"></div>
                    </div>
                </div>
            </div>
        );
    }

    if (!isAuthenticated || !currentUser) {
        return (
            <div className="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
                <div className="text-center">
                    <h1 className="text-2xl font-bold text-gray-900 mb-4">Not Authenticated</h1>
                    <p className="text-gray-600">Please log in to access the dashboard.</p>
                </div>
            </div>
        );
    }

    const displayName = getUserDisplayName(currentUser);
    const avatarUrl = getUserAvatar(currentUser);
    const initials = getUserInitials(currentUser);

    return (
        <div className="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
            {/* Welcome Section */}
            <div className="bg-white overflow-hidden shadow rounded-lg mb-6">
                <div className="px-4 py-5 sm:p-6">
                    <div className="flex items-center">
                        <div className="flex-shrink-0">
                            {avatarUrl ? (
                                <img
                                    className="h-16 w-16 rounded-full object-cover"
                                    src={avatarUrl}
                                    alt={`${displayName}'s avatar`}
                                    onError={(e) => {
                                        e.currentTarget.style.display = 'none';
                                        e.currentTarget.nextElementSibling?.classList.remove('hidden');
                                    }}
                                />
                            ) : null}
                            <div
                                className={`h-16 w-16 rounded-full bg-blue-500 text-white flex items-center justify-center text-xl font-medium ${avatarUrl ? 'hidden' : ''}`}
                            >
                                {initials}
                            </div>
                        </div>
                        <div className="ml-4">
                            <h1 className="text-2xl font-bold text-gray-900">
                                Welcome back, {displayName}!
                            </h1>
                            <p className="text-sm text-gray-500">
                                {currentUser.email}
                                {currentUser.provider && (
                                    <span className="ml-2 text-xs bg-gray-100 text-gray-800 px-2 py-1 rounded">
                                        via {currentUser.provider}
                                    </span>
                                )}
                            </p>
                            {isAdmin && (
                                <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-100 text-red-800 mt-1">
                                    Administrator
                                </span>
                            )}
                        </div>
                    </div>
                </div>
            </div>

            {/* User Information Card */}
            <div className="bg-white overflow-hidden shadow rounded-lg mb-6">
                <div className="px-4 py-5 sm:p-6">
                    <h3 className="text-lg leading-6 font-medium text-gray-900 mb-4">
                        User Information
                    </h3>
                    <dl className="grid grid-cols-1 gap-x-4 gap-y-6 sm:grid-cols-2">
                        <div>
                            <dt className="text-sm font-medium text-gray-500">Username</dt>
                            <dd className="mt-1 text-sm text-gray-900">{currentUser.username}</dd>
                        </div>
                        <div>
                            <dt className="text-sm font-medium text-gray-500">Email</dt>
                            <dd className="mt-1 text-sm text-gray-900">{currentUser.email || 'Not provided'}</dd>
                        </div>
                        <div>
                            <dt className="text-sm font-medium text-gray-500">Full Name</dt>
                            <dd className="mt-1 text-sm text-gray-900">
                                {currentUser.name || `${currentUser.givenName || ''} ${currentUser.familyName || ''}`.trim() || 'Not provided'}
                            </dd>
                        </div>
                        <div>
                            <dt className="text-sm font-medium text-gray-500">Authentication Provider</dt>
                            <dd className="mt-1 text-sm text-gray-900">{currentUser.provider || 'Not specified'}</dd>
                        </div>
                        <div>
                            <dt className="text-sm font-medium text-gray-500">Account Type</dt>
                            <dd className="mt-1 text-sm text-gray-900">
                                {isAdmin ? 'Administrator' : 'Regular User'}
                            </dd>
                        </div>
                        <div>
                            <dt className="text-sm font-medium text-gray-500">Last Updated</dt>
                            <dd className="mt-1 text-sm text-gray-900">
                                {new Date(currentUser.timestamp).toLocaleString()}
                            </dd>
                        </div>
                    </dl>
                </div>
            </div>

            {/* Actions Card */}
            <div className="bg-white overflow-hidden shadow rounded-lg">
                <div className="px-4 py-5 sm:p-6">
                    <h3 className="text-lg leading-6 font-medium text-gray-900 mb-4">
                        Quick Actions
                    </h3>
                    <div className="flex flex-wrap gap-4">
                        <button
                            onClick={checkUserSession}
                            className="inline-flex items-center px-4 py-2 border border-gray-300 shadow-sm text-sm font-medium rounded-md text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
                        >
                            <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path>
                            </svg>
                            Refresh Session
                        </button>

                        {isAdmin && (
                            <>
                                <button className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500">
                                    <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197m13.5-9a2.5 2.5 0 11-5 0 2.5 2.5 0 015 0z"></path>
                                    </svg>
                                    Manage Users
                                </button>

                                <button className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500">
                                    <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path>
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path>
                                    </svg>
                                    System Settings
                                </button>
                            </>
                        )}
                    </div>
                </div>
            </div>

            {/* Development Info (only show in development) */}
            {process.env.NODE_ENV === 'development' && (
                <div className="mt-6 bg-yellow-50 border border-yellow-200 rounded-lg p-4">
                    <h4 className="text-sm font-medium text-yellow-800 mb-2">
                        Development Information
                    </h4>
                    <pre className="text-xs text-yellow-700 overflow-auto">
                        {JSON.stringify(currentUser, null, 2)}
                    </pre>
                </div>
            )}
        </div>
    );
};
