import { Link } from 'react-router-dom';
import { useTaronjaAuth } from 'taronja-gateway-react';
import { Button } from '../components/ui/Button';

// Simple 404 Page component for admin context
export const NotFoundPage = () => {
    const { isAuthenticated, currentUser } = useTaronjaAuth();

    return (
        <div className="flex min-h-[70vh] flex-col items-center justify-center text-center">
            <h1 className="mb-4 text-4xl font-semibold tracking-tight">404</h1>
            <p className="mb-8 max-w-xl text-muted-fg">
                Sorry, the admin page you are looking for does not exist or has been moved.
            </p>
            {isAuthenticated && currentUser?.isAdmin ? (
                <Link
                    to="/home" // Link back to the home page
                    className="inline-flex"
                >
                    <Button>Go to Home</Button>
                </Link>
            ) : (
                <a
                    href="/"
                    className="inline-flex"
                >
                    <Button>Go to Main Site</Button>
                </a>
            )}
            <footer className="mt-10 text-center text-xs text-muted-fg">
                <p>Taronja Gateway Admin</p>
            </footer>
        </div>
    );
};

