import React, { ReactNode, useState, useEffect } from 'react';
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
  // Add more specific titles as needed
  // if (path.startsWith('/settings')) return 'Settings';
  return 'Admin Dashboard'; // Default title
};

const MainLayout: React.FC<MainLayoutProps> = ({ children }) => {
  const [isMobileSidebarOpen, setIsMobileSidebarOpen] = useState(false);
  const [isDesktopSidebarCollapsed, setIsDesktopSidebarCollapsed] = useState(false);
  const [isMobileView, setIsMobileView] = useState(false);
  const location = useLocation();
  const [pageTitle, setPageTitle] = useState(getPageTitleFromPath(location.pathname));

  useEffect(() => {
    const checkMobileView = () => {
      const mobile = window.innerWidth < 768; // md breakpoint
      setIsMobileView(mobile);
      if (!mobile && isMobileSidebarOpen) {
        setIsMobileSidebarOpen(false);
      }
    };
    checkMobileView();
    window.addEventListener('resize', checkMobileView);
    return () => window.removeEventListener('resize', checkMobileView);
  }, [isMobileSidebarOpen]);

  useEffect(() => {
    setPageTitle(getPageTitleFromPath(location.pathname));
  }, [location.pathname]);

  const toggleMobileSidebar = () => {
    setIsMobileSidebarOpen(!isMobileSidebarOpen);
  };

  const toggleDesktopCollapse = () => {
    setIsDesktopSidebarCollapsed(!isDesktopSidebarCollapsed);
  };

  return (
    <div className="flex h-screen bg-gray-100">
      <Sidebar
        isOpenOnMobile={isMobileSidebarOpen}
        toggleMobileSidebar={toggleMobileSidebar}
        isDesktopCollapsed={isDesktopSidebarCollapsed}
        toggleDesktopCollapse={toggleDesktopCollapse}
      />
      <div className="flex-1 flex flex-col overflow-hidden">
        <Header
          pageTitle={pageTitle}
          toggleMobileSidebar={toggleMobileSidebar}
          isMobileView={isMobileView}
        />
        <main className="flex-1 overflow-x-hidden overflow-y-auto bg-gray-100 p-6">
          {children}
        </main>
      </div>
    </div>
  );
};

export default MainLayout;
