import { Link } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';

// Simple 404 Page component for admin context
export const NotFoundPage = () => {
    const { isAuthenticated, currentUser } = useAuth();

    return (
        <div className="min-h-screen flex flex-col items-center justify-center bg-gray-100 text-center p-4">
            <h1 className="text-4xl font-bold text-slate-700 mb-4">404 - Page Not Found</h1>
            <p className="text-lg text-slate-600 mb-8">
                Sorry, the admin page you are looking for does not exist or has been moved.
            </p>
            {isAuthenticated && currentUser?.isAdmin ? (
                <Link
                    to="/dashboard" // Link back to the main admin page
                    className="px-6 py-3 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 transition-colors"
                >
                    Go to Dashboard
                </Link>
            ) : (
                <a
                    href="/"
                    className="px-6 py-3 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 transition-colors"
                >
                    Go to Main Site
                </a>
            )}
            <footer className="absolute bottom-4 text-center p-4 text-gray-500 text-xs">
                <p>Taronja Gateway Admin</p>
            </footer>
        </div>
    );
};

