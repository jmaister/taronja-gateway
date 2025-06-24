/**
 * Sidebar Component - Uses DaisyUI 5 components
 * 
 * DaisyUI Components used:
 * - drawer: For sidebar layout
 * - menu: For navigation items
 * - collapse: For accordion functionality (Users submenu)
 * - btn: For buttons
 * - card: For user info section
 * 
 * Reference: https://daisyui.com/llms.txt
 * Follow DaisyUI best practices:
 * - Use semantic component classes
 * - Combine with Tailwind utilities
 * - Use daisyUI color names (primary, secondary, etc.)
 */
import { useAuth, getUserDisplayName } from '../../contexts/AuthContext';
import { Link } from 'react-router-dom';

interface SidebarProps {
  isOpenOnMobile: boolean; // Keep for consistency but not used internally
  toggleMobileSidebar: () => void;
}

const Sidebar = ({
  toggleMobileSidebar,
}: SidebarProps) => {
  const { currentUser, logout } = useAuth();

  const navItems = [
    { name: 'Dashboard', icon: '📊', path: '/dashboard' },
    { 
      name: 'Users', 
      icon: '👥', 
      submenu: [
        { name: 'List', path: '/users' },
        { name: 'Create New', path: '/users/new' }
      ]
    },
  ];

  return (
    <div className="drawer-side z-40">
      {/* Mobile overlay */}
      <label 
        htmlFor="sidebar-drawer" 
        className="drawer-overlay md:hidden"
        onClick={toggleMobileSidebar}
      ></label>
      
      {/* Sidebar */}
      <aside className="min-h-full bg-base-200 flex flex-col w-64 transition-all duration-300 ease-in-out">
        {/* Header */}
        <div className="navbar bg-base-300 min-h-16">
          <div className="navbar-start">
            <h1 className="text-sm font-bold">Taronja Gateway</h1>
          </div>
          <div className="navbar-end">
            {/* Mobile close button */}
            <button
              onClick={toggleMobileSidebar}
              className="btn btn-ghost btn-sm md:hidden"
              aria-label="Close sidebar"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        {/* Navigation Menu */}
        <div className="flex-1 p-4">
          <ul className="menu menu-vertical w-full">
            {navItems.map((item) => {
              // Handle items with submenus
              if (item.submenu) {
                return (
                  <li key={item.name}>
                    <details className="group">
                      <summary className="group-hover:bg-base-300">
                        <span className="text-lg">{item.icon}</span>
                        <span>{item.name}</span>
                      </summary>
                      <ul className="menu-compact">
                        {item.submenu.map((subItem) => (
                          <li key={subItem.name}>
                            <Link to={subItem.path} className="pl-8">
                              {subItem.name}
                            </Link>
                          </li>
                        ))}
                      </ul>
                    </details>
                  </li>
                );
              }
              
              // Handle regular menu items
              if (item.path) {
                return (
                  <li key={item.name}>
                    <Link to={item.path}>
                      <span className="text-lg">{item.icon}</span>
                      <span>{item.name}</span>
                    </Link>
                  </li>
                );
              }
              
              return null;
            })}
          </ul>
        </div>

        {/* User Info Footer */}
        {currentUser && (
          <div className="p-4 border-t border-base-300">
            <div className="card card-compact bg-base-100 shadow-sm">
              <div className="card-body">
                <div className="text-xs opacity-70">Logged in as:</div>
                <div className="font-medium text-sm mb-3">
                  {getUserDisplayName(currentUser)}
                </div>
                <button
                  onClick={logout}
                  className="btn btn-outline btn-sm w-full"
                >
                  Logout
                </button>
              </div>
            </div>
            <div className="text-center text-xs opacity-50 mt-3">
              © {new Date().getFullYear()} Admin Panel
            </div>
          </div>
        )}
      </aside>
    </div>
  );
};

export default Sidebar;
