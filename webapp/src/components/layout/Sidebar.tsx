import { useState, useEffect } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useAuth, getUserDisplayName } from '../../contexts/AuthContext';

interface SubMenuItem {
  name: string;
  path: string;
}

interface NavItemConfig {
  name: string;
  icon: string;
  path?: string;
  submenu?: SubMenuItem[];
}

interface SidebarProps {
  isOpenOnMobile: boolean;
  toggleMobileSidebar: () => void;
  isDesktopCollapsed: boolean;
  toggleDesktopCollapse: () => void;
}

const Sidebar = ({
  isOpenOnMobile,
  toggleMobileSidebar,
  isDesktopCollapsed,
  toggleDesktopCollapse,
}: SidebarProps) => {
  const [isMobileView, setIsMobileView] = useState(false);
  const [expandedMenus, setExpandedMenus] = useState<string[]>([]);
  const location = useLocation();
  const { currentUser, logout } = useAuth();

  useEffect(() => {
    const checkMobileView = () => setIsMobileView(window.innerWidth < 768); // md breakpoint
    checkMobileView();
    window.addEventListener('resize', checkMobileView);
    return () => window.removeEventListener('resize', checkMobileView);
  }, []);

  // Auto-expand Users menu if we're on a users page
  useEffect(() => {
    if (location.pathname.startsWith('/users')) {
      setExpandedMenus(prev => prev.includes('Users') ? prev : [...prev, 'Users']);
    }
  }, [location.pathname]);

  const navItems: NavItemConfig[] = [
    { name: 'Dashboard', icon: '📊', path: '/dashboard' },
    { 
      name: 'Users', 
      icon: '👥', 
      submenu: [
        { name: 'List', path: '/users' },
        { name: 'Create New', path: '/users/new' }
      ]
    },
    // { name: 'Settings', icon: '⚙️', path: '/settings' }, // Example of another potential link
  ];

  const displayIconsOnly = !isMobileView && isDesktopCollapsed;

  const isActiveLink = (path: string) => {
    // For exact path matching
    if (location.pathname === path) return true;
    // For users paths, also match sub-paths like /users/123
    if (path === '/users' && location.pathname.startsWith('/users')) return true;
    return false;
  };

  const isMenuExpanded = (menuName: string) => {
    return expandedMenus.includes(menuName);
  };

  const toggleMenu = (menuName: string) => {
    setExpandedMenus(prev => 
      prev.includes(menuName) 
        ? prev.filter(name => name !== menuName)
        : [...prev, menuName]
    );
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
          bg-slate-900 text-white h-screen p-4 flex flex-col
          transition-all duration-300 ease-in-out shadow-xl border-r border-slate-700
          fixed md:relative z-40
          ${isMobileView ? (isOpenOnMobile ? 'translate-x-0 w-64' : '-translate-x-full w-64') : (displayIconsOnly ? 'w-20' : 'w-64')}
        `}
      >
        <div className="flex items-center justify-between mb-8 h-10">
          {!displayIconsOnly && <h1 className="text-2xl font-bold text-white">Admin</h1>}

          {!isMobileView && (
            <button
              onClick={toggleDesktopCollapse}
              className={`p-2 rounded-md text-slate-400 hover:bg-slate-700 hover:text-white focus:outline-none focus:ring-2 focus:ring-emerald-500 transition-all duration-200 ${displayIconsOnly ? 'mx-auto' : 'ml-auto'}`}
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
                className="p-2 rounded-md text-slate-400 hover:bg-slate-700 hover:text-white focus:outline-none focus:ring-2 focus:ring-emerald-500 transition-all duration-200 ml-auto md:hidden"
                aria-label="Close sidebar"
            >
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor" className="w-5 h-5"><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
          )}
        </div>

        <nav className="flex-grow">
          <ul className="space-y-1.5">
            {navItems.map((item) => {
              // Handle items with submenus
              if (item.submenu) {
                const isExpanded = isMenuExpanded(item.name);
                const hasActiveChild = item.submenu.some(subItem => isActiveLink(subItem.path));
                
                return (
                  <li key={item.name}>
                    <button
                      onClick={() => displayIconsOnly ? null : toggleMenu(item.name)}
                      title={displayIconsOnly ? item.name : undefined}
                      className={`
                        w-full flex items-center p-3 rounded-lg transition-all duration-200 group
                        ${displayIconsOnly ? 'justify-center' : 'justify-between'}
                        ${hasActiveChild
                          ? 'bg-blue-700 text-white shadow-lg border border-blue-600'
                          : 'bg-blue-500 hover:bg-slate-700 hover:text-white focus:bg-slate-700 focus:text-white focus:outline-none'
                        }
                      `}
                    >
                      <div className="flex items-center">
                        <span className={`text-lg ${hasActiveChild ? 'text-white' : 'text-slate-300 group-hover:text-white group-focus:text-white'}`}>
                          {item.icon}
                        </span>
                        {!displayIconsOnly && <span className="ml-3 font-medium">{item.name}</span>}
                      </div>
                      {!displayIconsOnly && (
                        <svg
                          className={`w-4 h-4 transition-transform duration-200 ${isExpanded ? 'rotate-90' : ''}`}
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                        </svg>
                      )}
                    </button>
                    
                    {/* Submenu items */}
                    {!displayIconsOnly && isExpanded && (
                      <ul className="mt-2 ml-4 space-y-1">
                        {item.submenu.map((subItem) => {
                          const isActive = isActiveLink(subItem.path);
                          return (
                            <li key={subItem.name}>
                              <Link
                                to={subItem.path}
                                className={`
                                  flex items-center p-2.5 pl-8 rounded-lg transition-all duration-200 text-sm font-medium
                                  ${isActive
                                    ? 'bg-blue-600 text-white shadow-md border border-blue-500'
                                    : 'text-slate-200 hover:bg-slate-600 hover:text-white focus:bg-slate-600 focus:text-white focus:outline-none'
                                  }
                                `}
                              >
                                {subItem.name}
                              </Link>
                            </li>
                          );
                        })}
                      </ul>
                    )}
                  </li>
                );
              }
              
              // Handle regular menu items (items with direct path)
              if (item.path) {
                const isActive = isActiveLink(item.path);
                return (
                  <li key={item.name}>
                    <Link
                      to={item.path}
                      title={displayIconsOnly ? item.name : undefined}
                      className={`
                        flex items-center p-3 rounded-lg transition-all duration-200 group
                        ${displayIconsOnly ? 'justify-center' : ''}
                        ${isActive
                          ? 'bg-blue-700 text-white shadow-lg border border-blue-600'
                          : 'text-slate-200 hover:bg-slate-600 hover:text-white focus:bg-slate-600 focus:text-white focus:outline-none'
                        }
                      `}
                    >
                      <span className={`text-lg ${isActive ? 'text-white' : 'text-slate-300 group-hover:text-white group-focus:text-white'}`}>
                        {item.icon}
                      </span>
                      {!displayIconsOnly && <span className="ml-3 font-medium">{item.name}</span>}
                    </Link>
                  </li>
                );
              }
              
              return null;
            })}
          </ul>
        </nav>

        {!displayIconsOnly && (
          <div className="mt-auto pt-6 border-t border-slate-700">
            {currentUser && (
              <div className="mb-4 px-3">
                <div className="text-xs text-slate-400 mb-2">Logged in as:</div>
                <div className="text-sm text-white font-medium mb-2">
                  {getUserDisplayName(currentUser)}
                </div>
                <button
                  onClick={logout}
                  className="w-full text-xs text-slate-300 hover:text-white hover:bg-slate-700 py-2 px-3 rounded border border-slate-600 hover:border-slate-500 transition-all duration-200"
                >
                  Logout
                </button>
              </div>
            )}
            <p className="text-xs text-slate-500 text-center">© {new Date().getFullYear()} Admin Panel</p>
          </div>
        )}
      </div>
    </>
  );
};

export default Sidebar;
