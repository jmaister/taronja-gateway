import React, { useState, useEffect } from 'react';
import { Link, useLocation } from 'react-router-dom';

interface NavItemConfig {
  name: string;
  icon: string;
  path: string;
}

interface SidebarProps {
  isOpenOnMobile: boolean;
  toggleMobileSidebar: () => void;
  isDesktopCollapsed: boolean;
  toggleDesktopCollapse: () => void;
}

const Sidebar: React.FC<SidebarProps> = ({
  isOpenOnMobile,
  toggleMobileSidebar,
  isDesktopCollapsed,
  toggleDesktopCollapse,
}) => {
  const [isMobileView, setIsMobileView] = useState(false);
  const location = useLocation();

  useEffect(() => {
    const checkMobileView = () => setIsMobileView(window.innerWidth < 768); // md breakpoint
    checkMobileView();
    window.addEventListener('resize', checkMobileView);
    return () => window.removeEventListener('resize', checkMobileView);
  }, []);

  const navItems: NavItemConfig[] = [
    { name: 'Users', icon: '👥', path: '/users' },
    // { name: 'Settings', icon: '⚙️', path: '/settings' }, // Example of another potential link
  ];

  const displayIconsOnly = !isMobileView && isDesktopCollapsed;

  const isActiveLink = (path: string) => {
    // Highlight "Users" if current path is /users or any sub-path like /users/new or /users/:userId
    if (path === "/users") {
      return location.pathname === path || location.pathname.startsWith(path + '/');
    }
    return location.pathname === path;
  };

  return (
    <>
      {isMobileView && isOpenOnMobile && (
        <div
          className="fixed inset-0 bg-black/50 z-30 md:hidden"
          onClick={toggleMobileSidebar}
        ></div>
      )}

      <div
        className={`
          bg-slate-800 text-slate-100 h-screen p-4 flex flex-col
          transition-all duration-300 ease-in-out shadow-xl
          fixed md:relative z-40
          ${isMobileView ? (isOpenOnMobile ? 'translate-x-0 w-64' : '-translate-x-full w-64') : (displayIconsOnly ? 'w-20' : 'w-64')}
        `}
      >
        <div className="flex items-center justify-between mb-8 h-10">
          {!displayIconsOnly && <h1 className="text-2xl font-bold text-white">Admin</h1>}

          {!isMobileView && (
            <button
              onClick={toggleDesktopCollapse}
              className={`p-2 rounded-md text-slate-300 hover:bg-slate-700 focus:outline-none focus:ring-2 focus:ring-slate-500 ${displayIconsOnly ? 'mx-auto' : 'ml-auto'}`}
              aria-label={isDesktopCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            >
              {displayIconsOnly ? (
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" className="w-5 h-5"><path strokeLinecap="round" strokeLinejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" /></svg>
              ) : (
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" className="w-5 h-5"><path strokeLinecap="round" strokeLinejoin="round" d="M15.75 19.5L8.25 12l7.5-7.5" /></svg>
              )}
            </button>
          )}

          {isMobileView && isOpenOnMobile && (
             <button
                onClick={toggleMobileSidebar}
                className="p-2 rounded-md text-slate-300 hover:bg-slate-700 focus:outline-none focus:ring-2 focus:ring-slate-500 ml-auto md:hidden"
                aria-label="Close sidebar"
            >
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" className="w-5 h-5"><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
          )}
        </div>

        <nav className="flex-grow">
          <ul className="space-y-1.5">
            {navItems.map((item) => {
              const isActive = isActiveLink(item.path);
              return (
                <li key={item.name}>
                  <Link
                    to={item.path}
                    title={displayIconsOnly ? item.name : undefined}
                    className={`
                      flex items-center p-2.5 rounded-lg transition-colors duration-150 group
                      ${displayIconsOnly ? 'justify-center' : ''}
                      ${isActive
                        ? 'bg-blue-600 text-white shadow-md'
                        : 'text-slate-300 hover:bg-slate-700 hover:text-white focus:bg-slate-700 focus:text-white focus:outline-none'
                      }
                    `}
                  >
                    <span className={`text-lg ${isActive ? 'text-white' : 'text-slate-400 group-hover:text-slate-200 group-focus:text-slate-200'}`}>{item.icon}</span>
                    {!displayIconsOnly && <span className="ml-3 font-medium">{item.name}</span>}
                  </Link>
                </li>
              );
            })}
          </ul>
        </nav>

        {!displayIconsOnly && (
          <div className="mt-auto pt-6 border-t border-slate-700">
            <p className="text-xs text-slate-400 text-center">© {new Date().getFullYear()} Admin Panel</p>
          </div>
        )}
      </div>
    </>
  );
};

export default Sidebar;
