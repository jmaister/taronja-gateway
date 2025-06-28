import { useAuth, getUserDisplayName } from '../contexts/AuthContext';
import { Link } from 'react-router-dom';

/**
 * Home page - A simple welcome page without detailed user information
 */
export const HomePage = () => {
    const { currentUser, isLoading, isAuthenticated } = useAuth();

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
                    <p className="text-gray-600">Please log in to access the application.</p>
                </div>
            </div>
        );
    }

    const displayName = getUserDisplayName(currentUser);

    return (
        <div className="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
            {/* Welcome Section */}
            <div className="bg-white overflow-hidden shadow rounded-lg mb-6">
                <div className="px-4 py-5 sm:p-6">
                    <div className="text-center">
                        <h1 className="text-3xl font-bold text-gray-900 mb-4">
                            Welcome to Taronja Gateway, {displayName}!
                        </h1>
                        <p className="text-lg text-gray-600 mb-8">
                            Your application and API gateway for managing microservices
                        </p>
                    </div>
                </div>
            </div>

            {/* Quick Navigation Cards */}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                {/* Users Management Card */}
                <div className="bg-white overflow-hidden shadow rounded-lg hover:shadow-lg transition-shadow">
                    <div className="p-6">
                        <div className="flex items-center">
                            <div className="flex-shrink-0">
                                <svg className="h-8 w-8 text-blue-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197m13.5-9a2.5 2.5 0 11-5 0 2.5 2.5 0 015 0z"></path>
                                </svg>
                            </div>
                            <div className="ml-4">
                                <h3 className="text-lg font-medium text-gray-900">User Management</h3>
                                <p className="text-sm text-gray-500">Manage users, roles, and permissions</p>
                            </div>
                        </div>
                        <div className="mt-4">
                            <Link
                                to="/users"
                                className="text-blue-600 hover:text-blue-500 text-sm font-medium"
                            >
                                View Users →
                            </Link>
                        </div>
                    </div>
                </div>

                {/* Profile Card */}
                <div className="bg-white overflow-hidden shadow rounded-lg hover:shadow-lg transition-shadow">
                    <div className="p-6">
                        <div className="flex items-center">
                            <div className="flex-shrink-0">
                                <svg className="h-8 w-8 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"></path>
                                </svg>
                            </div>
                            <div className="ml-4">
                                <h3 className="text-lg font-medium text-gray-900">My Profile</h3>
                                <p className="text-sm text-gray-500">View and manage your account</p>
                            </div>
                        </div>
                        <div className="mt-4">
                            <Link
                                to="/profile"
                                className="text-green-600 hover:text-green-500 text-sm font-medium"
                            >
                                View Profile →
                            </Link>
                        </div>
                    </div>
                </div>

                {/* Gateway Info Card */}
                <div className="bg-white overflow-hidden shadow rounded-lg hover:shadow-lg transition-shadow">
                    <div className="p-6">
                        <div className="flex items-center">
                            <div className="flex-shrink-0">
                                <svg className="h-8 w-8 text-purple-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"></path>
                                </svg>
                            </div>
                            <div className="ml-4">
                                <h3 className="text-lg font-medium text-gray-900">Gateway Status</h3>
                                <p className="text-sm text-gray-500">Monitor system health and performance</p>
                            </div>
                        </div>
                        <div className="mt-4">
                            <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                                Online
                            </span>
                        </div>
                    </div>
                </div>
            </div>

            {/* System Status */}
            <div className="mt-8 bg-white overflow-hidden shadow rounded-lg">
                <div className="px-4 py-5 sm:p-6">
                    <h3 className="text-lg leading-6 font-medium text-gray-900 mb-4">
                        System Overview
                    </h3>
                    <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                        <div className="text-center">
                            <div className="text-2xl font-bold text-blue-600">Active</div>
                            <div className="text-sm text-gray-500">Gateway Status</div>
                        </div>
                        <div className="text-center">
                            <div className="text-2xl font-bold text-green-600">
                                {currentUser.isAdmin ? 'Admin' : 'User'}
                            </div>
                            <div className="text-sm text-gray-500">Access Level</div>
                        </div>
                        <div className="text-center">
                            <div className="text-2xl font-bold text-purple-600">
                                {new Date().toLocaleDateString()}
                            </div>
                            <div className="text-sm text-gray-500">Today's Date</div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
};
