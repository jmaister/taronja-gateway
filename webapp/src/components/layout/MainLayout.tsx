import { ReactNode, useState, useEffect } from 'react';
import Sidebar from './Sidebar';
import Header from './Header';
import { useLocation } from 'react-router-dom';

interface MainLayoutProps {
  children: ReactNode;
}

// Helper to derive a title from the path
const getPageTitleFromPath = (path: string): string => {
  if (path.startsWith('/users/new')) return 'Create New User';
  if (path.startsWith('/users/')) return 'User Details'; // Could be more specific if an ID or name is available
  if (path.startsWith('/users')) return 'User Management';
  if (path.startsWith('/counters')) return 'Counters Management';
  if (path.startsWith('/statistics/request-summary')) return 'Request Summary';
  if (path.startsWith('/statistics/requests-details')) return 'Request Details';
  if (path.startsWith('/statistics/rate-limiter')) return 'Rate Limiter Stats';
  if (path.startsWith('/statistics')) return 'Statistics';
  if (path.startsWith('/profile')) return 'Profile Settings';
  if (path.startsWith('/home')) return 'Home';
  // Add more specific titles as needed
  // if (path.startsWith('/settings')) return 'Settings';
  return 'Home'; // Default title
};

const MainLayout = ({ children }: MainLayoutProps) => {
  const [isMobileSidebarOpen, setIsMobileSidebarOpen] = useState(false);
  const [isCollapsed, setIsCollapsed] = useState(false);
  const location = useLocation();
  const [pageTitle, setPageTitle] = useState(getPageTitleFromPath(location.pathname));

  useEffect(() => {
    setPageTitle(getPageTitleFromPath(location.pathname));
  }, [location.pathname]);

  const toggleMobileSidebar = () => {
    setIsMobileSidebarOpen(!isMobileSidebarOpen);
  };

  const toggleCollapse = () => {
    setIsCollapsed(!isCollapsed);
  };

  return (
    <div className="min-h-screen bg-bg text-fg">
      <div className="flex min-h-screen">
        {/* Sidebar */}
        <Sidebar
          isOpenOnMobile={isMobileSidebarOpen}
          toggleMobileSidebar={toggleMobileSidebar}
          isCollapsed={isCollapsed}
          toggleCollapse={toggleCollapse}
        />

        {/* Content */}
        <div className="flex min-w-0 flex-1 flex-col">
          <Header
            pageTitle={pageTitle}
            toggleMobileSidebar={toggleMobileSidebar}
            isCollapsed={isCollapsed}
            toggleCollapse={toggleCollapse}
          />
          <main className="flex-1 overflow-x-auto px-4 py-6 sm:px-6 lg:px-8">
            {children}
          </main>
        </div>
      </div>
    </div>
  );
};

export default MainLayout;
