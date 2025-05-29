import React from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';

interface NewspaperLayoutProps {
  children: React.ReactNode;
}

const NewspaperLayout: React.FC<NewspaperLayoutProps> = ({ children }) => {
  const { isAuthenticated, logout } = useAuth();
  const navigate = useNavigate();

  const handleLogout = () => {
    logout();
    navigate('/'); // Redirect to home page after logout
  };

  return (
    <div className="min-h-screen bg-slate-100 flex flex-col font-sans">
      <header className="bg-slate-800 text-white shadow-lg sticky top-0 z-50">
        <div className="container mx-auto px-4 py-3 flex flex-wrap justify-between items-center">
          <h1 className="text-4xl font-bold font-serif tracking-tight">
            <Link to="/" className="hover:text-slate-300 transition-colors">The React Times</Link>
          </h1>
          <nav className="space-x-1 sm:space-x-2 mt-3 sm:mt-0 flex items-center">
            <Link to="/" className="text-sm sm:text-base hover:text-slate-300 px-3 py-2 rounded-md hover:bg-slate-700 transition-colors">Home</Link>
            <Link to="/article/1" className="text-sm sm:text-base hover:text-slate-300 px-3 py-2 rounded-md hover:bg-slate-700 transition-colors">Article 1</Link>
            <Link to="/article/2" className="text-sm sm:text-base hover:text-slate-300 px-3 py-2 rounded-md hover:bg-slate-700 transition-colors">Article 2</Link>
            <Link to="/premium/article/3" className="text-sm sm:text-base hover:text-slate-300 px-3 py-2 rounded-md hover:bg-slate-700 transition-colors">Premium 3</Link>
            <Link to="/premium/article/4" className="text-sm sm:text-base hover:text-slate-300 px-3 py-2 rounded-md hover:bg-slate-700 transition-colors">Premium 4</Link>
            {isAuthenticated ? (
              <button
                onClick={handleLogout}
                className="text-sm sm:text-base bg-red-600 hover:bg-red-700 text-white font-semibold py-2 px-4 rounded-md transition-colors shadow"
              >
                Logout
              </button>
            ) : (
              <Link 
                to="/login" 
                className="text-sm sm:text-base bg-green-600 hover:bg-green-700 text-white font-semibold py-2 px-4 rounded-md transition-colors shadow"
              >
                Login
              </Link>
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
