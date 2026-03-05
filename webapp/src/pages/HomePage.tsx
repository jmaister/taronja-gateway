import { useAuth, getUserDisplayName } from '../contexts/AuthContext';
import { Link } from 'react-router-dom';
import { Badge } from '../components/ui/Badge';
import { Card, CardContent, CardHeader } from '../components/ui/Card';
import { useRateLimiterConfig } from '../services/services';

/**
 * Home page - A simple welcome page without detailed user information
 */
export const HomePage = () => {
    const { currentUser, isLoading, isAuthenticated } = useAuth();
    const { data: rateLimiterConfig, isLoading: configLoading } = useRateLimiterConfig();

    if (isLoading) {
        return (
            <div className="mx-auto max-w-7xl py-6">
                <div className="animate-pulse">
                    <div className="mb-6 h-8 w-1/4 rounded bg-muted"></div>
                    <div className="space-y-4">
                        <div className="h-4 w-3/4 rounded bg-muted"></div>
                        <div className="h-4 w-1/2 rounded bg-muted"></div>
                    </div>
                </div>
            </div>
        );
    }

    if (!isAuthenticated || !currentUser) {
        return (
            <div className="mx-auto max-w-7xl py-6">
                <div className="text-center">
                    <h1 className="mb-4 text-2xl font-bold">Not Authenticated</h1>
                    <p className="text-muted-fg">Please log in to access the application.</p>
                </div>
            </div>
        );
    }

    const displayName = getUserDisplayName(currentUser);

    return (
        <div className="mx-auto max-w-7xl space-y-6">
            <Card>
                <CardContent className="py-10">
                    <div className="text-center">
                        <h1 className="mb-3 text-3xl font-semibold tracking-tight">
                            Welcome, {displayName}
                        </h1>
                        <p className="mx-auto max-w-2xl text-muted-fg">
                            Admin console for users, counters, sessions, and traffic analytics.
                        </p>
                    </div>
                </CardContent>
            </Card>

            {/* Quick Navigation Cards */}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                {/* Users Management Card */}
                <Card className="transition-shadow hover:shadow-soft">
                    <CardContent>
                        <div className="flex items-center">
                            <div className="shrink-0">
                                <svg className="h-8 w-8 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197m13.5-9a2.5 2.5 0 11-5 0 2.5 2.5 0 015 0z"></path>
                                </svg>
                            </div>
                            <div className="ml-4">
                                <h3 className="text-lg font-semibold">User Management</h3>
                                <p className="text-sm text-muted-fg">Users, roles, and access</p>
                            </div>
                        </div>
                        <div className="mt-4">
                            <Link
                                to="/users"
                                className="tg-link text-sm font-medium"
                            >
                                View Users →
                            </Link>
                        </div>
                    </CardContent>
                </Card>

                {/* Profile Card */}
                <Card className="transition-shadow hover:shadow-soft">
                    <CardContent>
                        <div className="flex items-center">
                            <div className="shrink-0">
                                <svg className="h-8 w-8 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"></path>
                                </svg>
                            </div>
                            <div className="ml-4">
                                <h3 className="text-lg font-semibold">My Profile</h3>
                                <p className="text-sm text-muted-fg">Account and preferences</p>
                            </div>
                        </div>
                        <div className="mt-4">
                            <Link
                                to="/profile"
                                className="tg-link text-sm font-medium"
                            >
                                View Profile →
                            </Link>
                        </div>
                    </CardContent>
                </Card>

                {/* Gateway Info Card */}
                <Card className="transition-shadow hover:shadow-soft">
                    <CardContent>
                        <div className="flex items-center">
                            <div className="shrink-0">
                                <svg className="h-8 w-8 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"></path>
                                </svg>
                            </div>
                            <div className="ml-4">
                                <h3 className="text-lg font-semibold">Gateway Status</h3>
                                <p className="text-sm text-muted-fg">Health and performance</p>
                            </div>
                        </div>
                        <div className="mt-4">
                            <Badge variant="success">Online</Badge>
                        </div>
                    </CardContent>
                </Card>
            </div>

            {/* Rate Limiter Config Card */}
            <Card>
                <CardHeader>
                    <div className="flex items-center justify-between">
                        <h3 className="text-base font-semibold">Rate Limiter Configuration</h3>
                        <Link to="/statistics/rate-limiter" className="tg-link text-sm font-medium">
                            View Live Stats →
                        </Link>
                    </div>
                </CardHeader>
                <CardContent>
                    {configLoading ? (
                        <div className="animate-pulse space-y-2">
                            <div className="h-4 w-1/3 rounded bg-muted"></div>
                            <div className="h-4 w-1/4 rounded bg-muted"></div>
                        </div>
                    ) : !rateLimiterConfig || (rateLimiterConfig.requestsPerMinute == null && rateLimiterConfig.maxErrors == null) ? (
                        <p className="text-sm text-muted-fg">Rate limiter is not configured.</p>
                    ) : (
                        <>
                            {/* three-section layout: 1) RPM 2) max errors and block minutes 3) vulnerability scan details */}
                            <div className="space-y-4">
                            {/* section 1: requests per minute */}
                            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                                <div>
                                    <div className="text-2xl font-semibold text-primary">
                                        {rateLimiterConfig.requestsPerMinute ?? '—'}
                                    </div>
                                    <div className="text-sm text-muted-fg">Requests / minute</div>
                                </div>
                            </div>

                            {/* section 2: errors and block duration */}
                            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                                <div>
                                    <div className="text-2xl font-semibold text-primary">
                                        {rateLimiterConfig.maxErrors ?? '—'}
                                    </div>
                                    <div className="text-sm text-muted-fg">Max errors before block</div>
                                </div>
                                <div>
                                    <div className="text-2xl font-semibold text-primary">
                                        {rateLimiterConfig.blockMinutes ?? '—'}
                                    </div>
                                    <div className="text-sm text-muted-fg">Block duration (min)</div>
                                </div>
                            </div>

                            {/* section 3: vulnerability scan details */}
                            {rateLimiterConfig.vulnerabilityScan && (
                                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                                    <div>
                                        <div className="text-2xl font-semibold text-warning">
                                            {rateLimiterConfig.vulnerabilityScan.max404}
                                        </div>
                                        <div className="text-sm text-muted-fg">Max scan 404s</div>
                                    </div>
                                    <div>
                                        <div className="text-2xl font-semibold text-warning">
                                            {rateLimiterConfig.vulnerabilityScan.blockMinutes}
                                        </div>
                                        <div className="text-sm text-muted-fg">Scan block (min)</div>
                                    </div>
                                    <div>
                                        <div className="text-sm font-medium text-fg">
                                            {rateLimiterConfig.vulnerabilityScan.urls.length} monitored path{rateLimiterConfig.vulnerabilityScan.urls.length !== 1 ? 's' : ''}
                                        </div>
                                        <div className="text-xs text-muted-fg truncate max-w-xs">
                                            {rateLimiterConfig.vulnerabilityScan.urls.slice(0, 3).join(', ')}
                                            {rateLimiterConfig.vulnerabilityScan.urls.length > 3 && '…'}
                                        </div>
                                    </div>
                                </div>
                            )}
                        </div>
                        </>
                    )}
                </CardContent>
            </Card>

            {/* System Status */}
            <Card>
                <CardHeader>
                    <h3 className="text-base font-semibold">System Overview</h3>                </CardHeader>
                <CardContent>
                    <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                        <div className="text-center">
                            <div className="text-2xl font-semibold text-primary">Active</div>
                            <div className="text-sm text-muted-fg">Gateway Status</div>
                        </div>
                        <div className="text-center">
                            <div className="text-2xl font-semibold text-primary">
                                {currentUser.isAdmin ? 'Admin' : 'User'}
                            </div>
                            <div className="text-sm text-muted-fg">Access Level</div>
                        </div>
                        <div className="text-center">
                            <div className="text-2xl font-semibold text-primary">
                                {new Date().toLocaleDateString()}
                            </div>
                            <div className="text-sm text-muted-fg">Today's Date</div>
                        </div>
                    </div>
                </CardContent>
            </Card>
        </div>
    );
};
