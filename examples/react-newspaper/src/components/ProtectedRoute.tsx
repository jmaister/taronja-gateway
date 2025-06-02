import React, { useEffect } from 'react';
import { useAuth } from '../contexts/AuthContext';

interface ProtectedRouteProps {
  children: React.ReactNode;
}

const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children }) => {
  const { isAuthenticated, isLoading } = useAuth();

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      // Construct the redirect URL. If the app is hosted at a subpath,
      // ensure `/_/login` is correctly relative to the domain root.
      // For this example, assuming direct domain root.
      const loginUrl = '/_/login';

      // Add current path as a 'return_to' query parameter
      // Ensure it's properly encoded.
      const returnToPath = window.location.pathname + window.location.search;
      const redirectUrl = `${loginUrl}?redirect=${encodeURIComponent(returnToPath)}`;

      window.location.href = redirectUrl;
    }
  }, [isLoading, isAuthenticated]);

  if (isLoading) {
    return (
      <div className="flex justify-center items-center min-h-screen">
        <p className="text-xl text-gray-700">Loading session...</p>
      </div>
    );
  }

  if (!isAuthenticated) {
    // While useEffect handles redirection, returning null prevents rendering children.
    // A loading or "Redirecting to login..." message could also be shown here.
    return (
        <div className="flex justify-center items-center min-h-screen">
            <p className="text-xl text-gray-700">Redirecting to login...</p>
        </div>
    );
  }

  return <>{children}</>;
};

export default ProtectedRoute;
