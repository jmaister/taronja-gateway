import { useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom';
// App-level styles live in index.css

// Layout and Page Components
import MainLayout from './components/layout/MainLayout';
import { UsersListPage } from './pages/UsersListPage';
import { CreateUserPage } from './pages/CreateUserPage';
import { UserInfoPage } from './pages/UserInfoPage';
import { HomePage } from './pages/HomePage';
import { ProfilePage } from './pages/ProfilePage';
import { NotFoundPage } from './pages/NotFoundPage';
import { RequestSummaryPage } from './pages/RequestSummaryPage';
import { RequestsDetailsPage } from './pages/RequestsDetailsPage';
import { CountersManagementPage } from './pages/CountersManagementPage';
import { RateLimiterStatsPage } from './pages/RateLimiterStatsPage';
import { MiddlewarePage } from './pages/MiddlewarePage';

// Authentication components
import { useTaronjaAuth } from 'taronja-gateway-react-sdk';

// Navigates to the login page and shows a fallback message/link while that
// navigation is in flight. The navigation itself runs in an effect, not
// directly in the render body of AdminLayoutRoutes below — a real
// DOM/browser mutation like window.location.href is a side effect, and
// running it during render (rather than after commit) is unsafe under
// React's rules (Strict Mode's double-invoked render in dev, or a
// concurrent render started and then discarded) even though a full-page
// navigation mostly hides the consequences in practice.
const RedirectToLogin = () => {
    useEffect(() => {
        window.location.href = '/_/login';
    }, []);

    return (
        <div className="min-h-screen flex items-center justify-center bg-bg">
            <div className="text-center">
                <p className="text-muted-fg mb-4">Redirecting to login...</p>
                <a href="/login" className="text-primary hover:text-primary/80">
                    Click here if not redirected automatically
                </a>
            </div>
        </div>
    );
};

// A component to group routes under MainLayout with admin protection
const AdminLayoutRoutes = () => {
    const { isAuthenticated, currentUser, isLoading } = useTaronjaAuth();

    // Show loading state while checking authentication
    if (isLoading) {
        return (
            <div className="min-h-screen flex items-center justify-center bg-bg">
                <div className="text-center">
                    <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary mx-auto mb-4"></div>
                    <p className="text-muted-fg">Loading...</p>
                </div>
            </div>
        );
    }

    // Redirect to login if not authenticated.
    if (!isAuthenticated) {
        return <RedirectToLogin />;
    }

    // Check for admin privileges
    if (!currentUser?.isAdmin) {
        return (
            <div className="min-h-screen flex flex-col items-center justify-center bg-bg text-center p-4">
                <h1 className="text-4xl font-bold text-danger mb-4">Access Denied</h1>
                <p className="text-lg text-muted-fg mb-8">
                    You need administrator privileges to access this admin panel.
                </p>
                <div className="space-y-4">
                    <p className="text-sm text-muted-fg">
                        Current user: {currentUser?.username} ({currentUser?.email})
                    </p>
                    <a
                        href="/_/admin"
                        className="inline-block px-6 py-3 text-sm font-medium text-primary-fg bg-primary rounded-lg hover:bg-primary/90 transition-colors"
                    >
                        Go to Main Site
                    </a>
                </div>
                <footer className="absolute bottom-4 text-center p-4 text-muted-fg text-xs">
                    <p>Taronja Gateway Admin</p>
                </footer>
            </div>
        );
    }

    // User is authenticated and has admin privileges
    return (
        <MainLayout>
            <Outlet /> {/* Child routes will render here through MainLayout's children prop */}
        </MainLayout>
    );
};


function App() {
    return (
        <BrowserRouter basename="/_/admin">
            <Routes>
                {/* Routes that use the MainLayout */}
                <Route element={<AdminLayoutRoutes />}>
                    <Route path="/home" element={<HomePage />} />
                    <Route path="/profile" element={<ProfilePage />} />
                    <Route path="/statistics/request-summary" element={<RequestSummaryPage />} />
                    <Route path="/users" element={<UsersListPage />} />
                    <Route path="/users/new" element={<CreateUserPage />} />
                    <Route path="/users/:userId" element={<UserInfoPage />} />
                    <Route path="/statistics/requests-details" element={<RequestsDetailsPage />} />
                    <Route path="/statistics/rate-limiter" element={<RateLimiterStatsPage />} />
                    <Route path="/middleware" element={<MiddlewarePage />} />
                    <Route path="/counters" element={<CountersManagementPage />} />
                    {/* Add other admin routes that should use MainLayout here */}
                </Route>

                {/* Root path redirect to /home */}
                <Route path="/" element={<Navigate replace to="/home" />} />

                {/* Catch-all for unmatched routes */}
                <Route path="*" element={<NotFoundPage />} />
            </Routes>
        </BrowserRouter>
    );
}

export default App;
