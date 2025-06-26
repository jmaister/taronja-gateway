import { UserBadge } from '../UserProfile';

interface HeaderProps {
  pageTitle?: string;
  toggleMobileSidebar?: () => void;
  isCollapsed?: boolean;
  toggleCollapse?: () => void;
}

const Header = ({
  pageTitle = "User Management",
  toggleMobileSidebar,
  isCollapsed = false,
  toggleCollapse,
}: HeaderProps) => {
  return (
    <div className="navbar bg-base-100 shadow-lg">
      <div className="navbar-start">
        {/* Universal menu button - works for both mobile drawer and desktop collapse */}
        <button
          onClick={() => {
            if (isCollapsed && toggleCollapse) {
              // Desktop: Show collapsed sidebar
              toggleCollapse();
            } else if (toggleMobileSidebar) {
              // Mobile: Toggle drawer
              toggleMobileSidebar();
            }
          }}
          className="btn btn-square btn-ghost"
          aria-label={isCollapsed ? "Show sidebar" : "Toggle menu"}
        >
          <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 6h16M4 12h16M4 18h16"></path>
          </svg>
        </button>
        
        <h1 className="text-xl font-semibold ml-2">{pageTitle}</h1>
      </div>

      <div className="navbar-end">
        {/* Search Bar */}
        <div className="form-control hidden md:block mr-4">
          <input 
            type="text" 
            placeholder="Search users, etc..." 
            className="input input-bordered input-sm w-64" 
          />
        </div>

        {/* User Profile */}
        <UserBadge />
      </div>
    </div>
  );
};

export default Header;
