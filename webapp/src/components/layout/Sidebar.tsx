import { useTaronjaAuth } from 'taronja-gateway-react';
import { NavLink } from 'react-router-dom';
import { Button } from '../ui/Button';

interface SidebarProps {
  isOpenOnMobile: boolean; // Keep for consistency but not used internally
  toggleMobileSidebar: () => void;
  isCollapsed?: boolean;
  toggleCollapse?: () => void;
}

const Sidebar = ({
  isOpenOnMobile,
  toggleMobileSidebar,
  isCollapsed = false,
  toggleCollapse,
}: SidebarProps) => {
  const { currentUser, logout } = useTaronjaAuth();

  const navItems = [
    { name: 'Home', icon: '🏠', path: '/home' },
    { name: 'Profile', icon: '👤', path: '/profile' },
    { 
      name: 'Users', 
      icon: '👥', 
      submenu: [
        { name: 'List', path: '/users' },
        { name: 'Create New', path: '/users/new' }
      ]
    },
    { name: 'Counters', icon: '💰', path: '/counters' },
    { 
      name: 'Statistics', 
      icon: '📊', 
      submenu: [
        { name: 'Summary', path: '/statistics/request-summary' },
        { name: 'Details', path: '/statistics/requests-details' },
        { name: 'Rate Limiter', path: '/statistics/rate-limiter' }
      ]
    },
  ];

  if (isCollapsed) {
    return null;
  }

  const baseLink = 'flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors';
  const inactiveLink = 'text-muted-fg hover:bg-muted/70 hover:text-fg';
  const activeLink = 'bg-primary/10 text-fg ring-1 ring-primary/20';

  return (
    <>
      {/* Mobile overlay */}
      {isOpenOnMobile && (
        <div
          className="fixed inset-0 z-40 bg-black/40 lg:hidden"
          aria-hidden="true"
          onClick={toggleMobileSidebar}
        />
      )}

      <aside
        className={
          `fixed inset-y-0 left-0 z-50 w-72 border-r border-border bg-surface shadow-soft transition-transform lg:static lg:z-auto lg:translate-x-0 lg:shadow-none ` +
          (isOpenOnMobile ? 'translate-x-0' : '-translate-x-full lg:translate-x-0')
        }
      >
        <div className="flex h-16 items-center justify-between gap-2 px-4">
          <div className="min-w-0">
            <div className="text-sm font-semibold tracking-tight">Taronja Gateway</div>
            <div className="text-xs text-muted-fg">Admin console</div>
          </div>

          <div className="flex items-center gap-2">
            {toggleCollapse && (
              <Button
                variant="ghost"
                size="sm"
                className="hidden w-9 px-0 lg:inline-flex"
                aria-label="Hide sidebar"
                onClick={toggleCollapse}
              >
                <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
                </svg>
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              className="w-9 px-0 lg:hidden"
              aria-label="Close sidebar"
              onClick={toggleMobileSidebar}
            >
              <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </Button>
          </div>
        </div>

        <nav className="px-3 pb-4">
          <div className="px-2 pb-2 text-xs font-medium uppercase tracking-wide text-muted-fg">
            Navigation
          </div>

          <div className="space-y-1">
            {navItems.map((item) => {
              if (item.submenu) {
                return (
                  <details key={item.name} className="group">
                    <summary className={`${baseLink} ${inactiveLink} cursor-pointer list-none`}>
                      <span className="text-base" aria-hidden="true">{item.icon}</span>
                      <span className="flex-1">{item.name}</span>
                      <svg className="h-4 w-4 opacity-70 transition-transform group-open:rotate-180" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                      </svg>
                    </summary>
                    <div className="mt-1 space-y-1 pl-6">
                      {item.submenu.map((subItem) => (
                        <NavLink
                          key={subItem.name}
                          to={subItem.path}
                          className={({ isActive }) =>
                            `${baseLink} ${isActive ? activeLink : inactiveLink}`
                          }
                          onClick={() => {
                            if (window.matchMedia('(max-width: 1023px)').matches) {
                              toggleMobileSidebar();
                            }
                          }}
                        >
                          {subItem.name}
                        </NavLink>
                      ))}
                    </div>
                  </details>
                );
              }

              if (item.path) {
                return (
                  <NavLink
                    key={item.name}
                    to={item.path}
                    className={({ isActive }) => `${baseLink} ${isActive ? activeLink : inactiveLink}`}
                    onClick={() => {
                      if (window.matchMedia('(max-width: 1023px)').matches) {
                        toggleMobileSidebar();
                      }
                    }}
                  >
                    <span className="text-base" aria-hidden="true">{item.icon}</span>
                    <span>{item.name}</span>
                  </NavLink>
                );
              }

              return null;
            })}
          </div>
        </nav>

        {currentUser && (
          <div className="mt-auto border-t border-border p-4">
            <Button variant="outline" className="w-full" onClick={() => void logout()}>
              Logout
            </Button>
          </div>
        )}
      </aside>
    </>
  );
};

export default Sidebar;
