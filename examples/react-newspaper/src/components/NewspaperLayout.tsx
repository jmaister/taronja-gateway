import React from 'react';
import { Link } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext'; // Ensure currentUser is exported and imported if not already

interface NewspaperLayoutProps {
  children: React.ReactNode;
}

const NewspaperLayout: React.FC<NewspaperLayoutProps> = ({ children }) => {
  const { isAuthenticated, currentUser, isLoading } = useAuth(); // Removed logout and useNavigate since we're using direct URL redirect

  const handleLogout = () => {
    // Simply redirect to the logout URL - the gateway will handle the logout process
    window.location.href = '/_/logout';
  };

  // Construct the login URL with a return_to parameter
  const getLoginUrl = () => {
    const returnToPath = window.location.pathname + window.location.search;
    return `/_/login?redirect=${encodeURIComponent(returnToPath)}`;
  };

  return (
    <div className="min-h-screen bg-slate-100 flex flex-col font-sans">
      <header className="bg-slate-900 text-white shadow-xl sticky top-0 z-50 border-b border-slate-700">
        <div className="container mx-auto px-4 py-4 flex flex-wrap justify-between items-center">
          <h1 className="text-3xl sm:text-4xl lg:text-5xl font-black font-serif tracking-wide text-white drop-shadow-lg">
            <Link to="/" className="text-white hover:text-blue-200 visited:text-white active:text-white focus:text-white transition-all duration-300 hover:drop-shadow-xl border-b-2 border-transparent hover:border-blue-300 pb-1 no-underline">
              THE REACT TIMES
            </Link>
          </h1>
          <nav className="space-x-2 sm:space-x-3 mt-3 sm:mt-0 flex items-center flex-wrap">
            <Link to="/" className="text-sm sm:text-base text-white hover:text-blue-300 px-3 py-2 rounded-md hover:bg-slate-800 transition-all duration-200 font-medium">Home</Link>
            <Link to="/article/1" className="text-sm sm:text-base text-white hover:text-blue-300 px-3 py-2 rounded-md hover:bg-slate-800 transition-all duration-200 font-medium">Article 1</Link>
            <Link to="/article/2" className="text-sm sm:text-base text-white hover:text-blue-300 px-3 py-2 rounded-md hover:bg-slate-800 transition-all duration-200 font-medium">Article 2</Link>
            <Link to="/premium/article/3" className="text-sm sm:text-base text-amber-300 hover:text-amber-200 px-3 py-2 rounded-md hover:bg-slate-800 transition-all duration-200 font-medium border border-amber-400/30">Premium 3</Link>
            <Link to="/premium/article/4" className="text-sm sm:text-base text-amber-300 hover:text-amber-200 px-3 py-2 rounded-md hover:bg-slate-800 transition-all duration-200 font-medium border border-amber-400/30">Premium 4</Link>

            {isLoading ? (
              <span className="text-sm sm:text-base text-slate-300 px-3 py-2 font-medium">Loading...</span>
            ) : isAuthenticated && currentUser ? (
              <>
                <span className="text-sm sm:text-base text-slate-200 px-3 py-2 hidden sm:inline font-medium">
                  Welcome, {currentUser.name || currentUser.id}
                </span>
                <button
                  onClick={handleLogout}
                  className="text-sm sm:text-base bg-red-600 hover:bg-red-700 text-white font-semibold py-2 px-4 rounded-md transition-all duration-200 shadow-md hover:shadow-lg border border-red-500"
                >
                  Logout
                </button>
              </>
            ) : (
              <a
                href={getLoginUrl()}
                className="text-sm sm:text-base bg-green-600 hover:bg-green-700 text-white font-semibold py-2 px-4 rounded-md transition-all duration-200 shadow-md hover:shadow-lg border border-green-500"
              >
                Login
              </a>
            )}
          </nav>
        </div>
      </header>
      <main className="container mx-auto p-4 sm:p-6 md:p-8 flex-grow w-full">
        {children}
      </main>
      <footer className="bg-slate-900 text-slate-300 p-6 text-center mt-auto">
        <p>&copy; {new Date().getFullYear()} The React Times. All rights reserved. Crafted with React & Tailwind CSS.</p>
      </footer>
    </div>
  );
};

export default NewspaperLayout;
