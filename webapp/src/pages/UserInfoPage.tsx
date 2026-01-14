import { useParams, Link } from 'react-router-dom'; 
import { UserTokensSection } from '../components/UserTokensSection'; 
import { useUser } from '@/services/services';
import { Card, CardContent, CardHeader } from '../components/ui/Card';

interface UserInfoPageProps {
  // Props are empty for now
}

export function UserInfoPage({}: UserInfoPageProps) {
  const { userId } = useParams<{ userId: string }>(); 
  
  const {data:user, isLoading, isError, error} = useUser(userId || '');

  const formatDate = (dateString?: string | null): string => {
    if (!dateString) return 'N/A';
    try {
      return new Date(dateString).toLocaleDateString(undefined, { 
        year: 'numeric', month: 'long', day: 'numeric', 
        hour: '2-digit', minute: '2-digit' 
      });
    } catch (e) {
      console.error("Error formatting date:", e);
      return dateString; 
    }
  };

  return (
    <div className="mx-auto max-w-3xl">
      <Card>
        <CardHeader>
          <h1 className="text-base font-semibold">User Information</h1>
        </CardHeader>
        <CardContent>

        {isLoading && (
          <p className="text-primary">Loading user details...</p>
        )}

        {isError && !isLoading && (
          <p className="mb-4 font-medium text-danger">{error.message}</p>
        )}

        {!isLoading && !isError && user && (
          <div className="mb-6">
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-muted-fg">ID:</span> {user.id}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-muted-fg">Username:</span> {user.username}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-muted-fg">Email:</span> {user.email}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-muted-fg">Name:</span> {user.name || 'N/A'}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-muted-fg align-top">Picture:</span>
              {user.picture ? (
                <img src={user.picture} alt="User Picture" className="max-w-[100px] max-h-[100px] align-middle rounded inline-block" />
              ) : (
                'N/A'
              )}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-muted-fg">Provider:</span> {user.provider}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-muted-fg">Created At:</span> {formatDate(user.createdAt)}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-muted-fg">Updated At:</span> {formatDate(user.updatedAt)}
            </div>

            {/* User Tokens Section */}
            <UserTokensSection userId={user.id} />
          </div>
        )}

        {!isLoading && !isError && !user && (
          <p className="mt-5 text-muted-fg">User details are not available.</p>
        )}

        <div className="mt-6 border-t border-border pt-4 text-sm">
          <p>
            <Link 
              to="/users" 
              className="tg-link mr-2"
            >
              Back to List
            </Link> | 
            <Link 
              to="/users/new" 
              className="tg-link mx-2"
            >
              Create Another User
            </Link> | 
            {/* The "Secret Page" link might be an external link or handled differently. */}
            {/* If it's external, `a` tag is fine. If SPA, it needs a route. */}
            <a 
              href="/secret/index.html" 
              className="tg-link mx-2"
            >
              Secret Page
            </a> | 
            <Link 
              to="/"
              className="tg-link ml-2"
            >
              Home
            </Link>
          </p>
        </div>
        </CardContent>
      </Card>
    </div>
  );
};

// Removed default export: export default UserInfoPage;
