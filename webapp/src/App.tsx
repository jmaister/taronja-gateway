import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom';
import './App.css'; // Keep if it has essential base styles not covered by Tailwind preflight

// Layout and Page Components
import MainLayout from './components/layout/MainLayout';
import { UsersListPage } from './components/UsersListPage';
import { CreateUserPage } from './components/CreateUserPage';
import { UserInfoPage } from './components/UserInfoPage';
import { HomePage } from './components/HomePage';
import { ProfilePage } from './components/ProfilePage';
import { NotFoundPage } from './components/NotFoundPage';
import { StatisticsPage } from './components/StatisticsPage';
import RequestsDetailsPage from './pages/RequestsDetailsPage';

// Authentication components
import { useAuth } from './contexts/AuthContext';

// A component to group routes under MainLayout with admin protection
const AdminLayoutRoutes = () => {
    const { isAuthenticated, currentUser, isLoading } = useAuth();

    // Show loading state while checking authentication
    if (isLoading) {
        return (
            <div className="min-h-screen flex items-center justify-center bg-gray-100">
                <div className="text-center">
                    <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500 mx-auto mb-4"></div>
                    <p className="text-gray-600">Loading...</p>
                </div>
            </div>
        );
    }

    // Redirect to login if not authenticated
    if (!isAuthenticated) {
        window.location.href = '/_/login';
        return (
            <div className="min-h-screen flex items-center justify-center bg-gray-100">
                <div className="text-center">
                    <p className="text-gray-600 mb-4">Redirecting to login...</p>
                    <a href="/login" className="text-blue-500 hover:text-blue-700">
                        Click here if not redirected automatically
                    </a>
                </div>
            </div>
        );
    }

    // Check for admin privileges
    if (!currentUser?.isAdmin) {
        return (
            <div className="min-h-screen flex flex-col items-center justify-center bg-gray-100 text-center p-4">
                <h1 className="text-4xl font-bold text-red-600 mb-4">Access Denied</h1>
                <p className="text-lg text-gray-600 mb-8">
                    You need administrator privileges to access this admin panel.
                </p>
                <div className="space-y-4">
                    <p className="text-sm text-gray-500">
                        Current user: {currentUser?.username} ({currentUser?.email})
                    </p>
                    <a
                        href="/_/admin"
                        className="inline-block px-6 py-3 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 transition-colors"
                    >
                        Go to Main Site
                    </a>
                </div>
                <footer className="absolute bottom-4 text-center p-4 text-gray-500 text-xs">
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
                    <Route path="/statistics" element={<StatisticsPage />} />
                    <Route path="/users" element={<UsersListPage />} />
                    <Route path="/users/new" element={<CreateUserPage />} />
                    <Route path="/users/:userId" element={<UserInfoPage />} />
                    <Route path="/statistics/requests-details" element={<RequestsDetailsPage />} />
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
