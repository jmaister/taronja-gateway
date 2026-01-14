import { useUsers } from '@/services/services';
import { Link } from 'react-router-dom'; 
import { Card, CardContent, CardHeader } from '../components/ui/Card';

// Props for UsersListPage
interface UsersListPageProps {
  // Props are empty for now
}

export function UsersListPage({}: UsersListPageProps) {
  const { data: users, isLoading, isError } = useUsers();

  if (!users || isLoading) {
    return <p className="text-primary">Loading users...</p>;
  }

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">User Management</h1>
        <p className="mt-1 text-sm text-muted-fg">Manage users and their permissions</p>
      </div>

      <Card>
        <CardHeader>
          <h2 className="text-base font-semibold">User List</h2>
        </CardHeader>
        <CardContent>

        {isLoading && (
          <p className="text-primary">Loading users...</p>
        )}

        {isError && !isLoading && (
          <p className="mb-4 font-medium text-danger">Failed to load users.</p>
        )}

        {!isLoading && !isError && users.length > 0 && (
          <div className="overflow-x-auto">
            <table className="mt-5 w-full border-collapse text-sm">
              <thead>
                <tr>
                  <th className="bg-surface-2 p-3 text-left font-semibold text-muted-fg">ID</th>
                  <th className="bg-surface-2 p-3 text-left font-semibold text-muted-fg">Username</th>
                  <th className="bg-surface-2 p-3 text-left font-semibold text-muted-fg">Email</th>
                  <th className="bg-surface-2 p-3 text-left font-semibold text-muted-fg">Provider</th>
                  <th className="bg-surface-2 p-3 text-left font-semibold text-muted-fg">Actions</th>
                </tr>
              </thead>
              <tbody>
                {users.map((user) => (
                  <tr key={user.id} className="border-b border-border hover:bg-muted/40">
                    <td className="p-3">{user.id}</td>
                    <td className="p-3 font-medium">{user.username}</td>
                    <td className="p-3 text-muted-fg">{user.email}</td>
                    <td className="p-3 text-muted-fg">{user.provider}</td>
                    <td className="p-3">
                      <Link 
                        to={`/users/${user.id}`} // Use Link for SPA navigation
                        className="tg-link"
                      >
                        View
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {!isLoading && !isError && users.length === 0 && (
          <p className="mt-5 text-muted-fg">No users found.</p>
        )}

        <div className="mt-5 border-t border-border pt-3 text-sm">
          <p>
            <Link 
              to="/users/new" // Use Link for SPA navigation
              className="tg-link"
            >
              Create New User
            </Link>
            {' | '}
            <Link 
              to="/"
              className="tg-link"
            >
              Home
            </Link>
          </p>
        </div>
        </CardContent>
      </Card>
    </div>
  );
}

// Removed default export: export default UsersListPage;
