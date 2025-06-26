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
  if (path.startsWith('/profile')) return 'Profile Settings';
  if (path.startsWith('/home')) return 'Home';
  // Add more specific titles as needed
  // if (path.startsWith('/settings')) return 'Settings';
  return 'Home'; // Default title
};

const MainLayout = ({ children }: MainLayoutProps) => {
  const [isMobileSidebarOpen, setIsMobileSidebarOpen] = useState(false);
  const location = useLocation();
  const [pageTitle, setPageTitle] = useState(getPageTitleFromPath(location.pathname));

  useEffect(() => {
    setPageTitle(getPageTitleFromPath(location.pathname));
  }, [location.pathname]);

  const toggleMobileSidebar = () => {
    setIsMobileSidebarOpen(!isMobileSidebarOpen);
  };

  return (
    <div className="drawer lg:drawer-open">
      <input 
        id="sidebar-drawer" 
        type="checkbox" 
        className="drawer-toggle" 
        checked={isMobileSidebarOpen}
        onChange={toggleMobileSidebar}
      />
      
      {/* Page content */}
      <div className="drawer-content flex flex-col">
        <Header
          pageTitle={pageTitle}
          toggleMobileSidebar={toggleMobileSidebar}
        />
        <main className="flex-1 p-6 bg-base-100">
          {children}
        </main>
      </div>

      {/* Sidebar */}
      <Sidebar
        isOpenOnMobile={isMobileSidebarOpen}
        toggleMobileSidebar={toggleMobileSidebar}
      />
    </div>
  );
};

export default MainLayout;
