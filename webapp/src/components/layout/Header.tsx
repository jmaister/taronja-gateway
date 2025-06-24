import React from 'react';

interface HeaderProps {
  pageTitle?: string;
  toggleMobileSidebar?: () => void;
  isMobileView?: boolean;
}

const Header: React.FC<HeaderProps> = ({
  pageTitle = "User Management",
  toggleMobileSidebar,
  isMobileView,
}) => {
  return (
    <header className="bg-white shadow-md p-4 sticky top-0 z-20">
      <div className="container mx-auto flex items-center justify-between h-12">
        <div className="flex items-center">
          {isMobileView && (
            <button
              onClick={toggleMobileSidebar}
              className="mr-3 p-2 rounded-md text-gray-500 hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-500 md:hidden"
              aria-label="Open sidebar"
            >
              <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 6h16M4 12h16m-7 6h7"></path>
              </svg>
            </button>
          )}
          <h1 className="text-xl md:text-2xl font-semibold text-gray-800">{pageTitle}</h1>
        </div>

        <div className="flex items-center space-x-3 sm:space-x-5">
          {/* Search Bar Placeholder */}
          <div className="relative hidden md:block">
            <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
              <svg className="h-5 w-5 text-gray-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                <path fillRule="evenodd" d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z" clipRule="evenodd" />
              </svg>
            </div>
            <input
              type="text"
              placeholder="Search users, etc..." // Updated placeholder
              className="block w-full bg-gray-50 border border-gray-300 rounded-lg
                         py-2 pl-10 pr-3 text-sm placeholder-gray-500
                         focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 transition-colors"
            />
          </div>

          {/* Notification Icon Button Placeholder */}
          <button
            type="button"
            className="p-1.5 rounded-full text-gray-500 hover:text-gray-700 hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-blue-500"
            aria-label="View notifications"
          >
            <span className="sr-only">View notifications</span>
            <svg className="h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor" aria-hidden="true">
              <path strokeLinecap="round" strokeLinejoin="round" d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0" />
            </svg>
          </button>

          {/* User Profile Placeholder */}
          <div className="flex items-center space-x-2 cursor-pointer group">
            <img
              src="https://via.placeholder.com/32"
              alt="User Avatar"
              className="w-8 h-8 rounded-full ring-2 ring-offset-1 ring-transparent group-hover:ring-blue-500 transition-all duration-150"
            />
            <span className="hidden sm:inline text-sm font-medium text-gray-700 group-hover:text-blue-600">Admin User</span>
          </div>
        </div>
      </div>
    </header>
  );
};

export default Header;
