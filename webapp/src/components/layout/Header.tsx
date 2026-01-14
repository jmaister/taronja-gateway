import { UserBadge } from '../UserProfile';
import { ThemeSwitcher } from '../theme/ThemeSwitcher';
import { Button } from '../ui/Button';
import { Input } from '../ui/Input';

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
    <header className="sticky top-0 z-30 border-b border-border bg-surface/80 backdrop-blur">
      <div className="flex h-16 items-center gap-3 px-4 sm:px-6 lg:px-8">
        {/* Menu button: mobile opens drawer; desktop collapses sidebar */}
        <Button
          variant="ghost"
          size="sm"
          aria-label={isCollapsed ? 'Show sidebar' : 'Toggle sidebar'}
          onClick={() => {
            const isDesktop = window.matchMedia('(min-width: 1024px)').matches;
            if (isDesktop) {
              if (toggleCollapse) {
                toggleCollapse();
              }
              return;
            }

            if (toggleMobileSidebar) {
              toggleMobileSidebar();
            }
          }}
          className="w-9 px-0"
        >
          <svg className="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 6h16M4 12h16M4 18h16" />
          </svg>
        </Button>

        <div className="min-w-0 flex-1">
          <h1 className="truncate text-base font-semibold tracking-tight">{pageTitle}</h1>
          <p className="hidden text-xs text-muted-fg sm:block">
            Taronja Gateway Admin
          </p>
        </div>

        {/* Search */}
        <div className="hidden w-[22rem] lg:block">
          <Input placeholder="Search…" aria-label="Search" />
        </div>

        <ThemeSwitcher />
        <UserBadge />
      </div>
    </header>
  );
};

export default Header;
