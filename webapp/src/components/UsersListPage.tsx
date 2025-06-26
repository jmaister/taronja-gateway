import { useState, useEffect } from 'react'; // Removed React import
import { Link } from 'react-router-dom'; 
import { fetchUsers, User } from '../services/api'; 

// Props for UsersListPage
interface UsersListPageProps {
  // Props are empty for now
}

export function UsersListPage({}: UsersListPageProps) {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const loadUsers = async () => {
      try {
        setLoading(true);
        const fetchedUsers = await fetchUsers();
        setUsers(fetchedUsers);
        setError(null);
      } catch (err: any) {
        setError(err.message || 'Failed to fetch users');
        setUsers([]); // Clear users on error
      } finally {
        setLoading(false);
      }
    };
    loadUsers();
  }, []); // Empty dependency array to run only on mount

  return (
    <div className="w-full p-6">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">User Management</h1>
        <p className="text-gray-600 mt-2">Manage users and their permissions</p>
      </div>

      <div className="bg-white rounded-lg shadow-lg overflow-hidden">
        <div className="px-6 py-4 border-b border-gray-200">
          <h2 className="text-xl font-semibold text-gray-800">User List</h2>
        </div>
        
        <div className="p-6">

        {loading && (
          <p className="text-blue-500">Loading users...</p>
        )}

        {error && !loading && (
          <p className="text-red-500 font-bold mb-4">{error}</p>
        )}

        {!loading && !error && users.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse mt-5">
              <thead>
                <tr>
                  <th className="text-left p-2 border-b border-gray-300 bg-gray-100">ID</th>
                  <th className="text-left p-2 border-b border-gray-300 bg-gray-100">Username</th>
                  <th className="text-left p-2 border-b border-gray-300 bg-gray-100">Email</th>
                  <th className="text-left p-2 border-b border-gray-300 bg-gray-100">Provider</th>
                  <th className="text-left p-2 border-b border-gray-300 bg-gray-100">Actions</th>
                </tr>
              </thead>
              <tbody>
                {users.map((user) => (
                  <tr key={user.id} className="hover:bg-gray-50">
                    <td className="text-left p-2 border-b border-gray-300">{user.id}</td>
                    <td className="text-left p-2 border-b border-gray-300">{user.username}</td>
                    <td className="text-left p-2 border-b border-gray-300">{user.email}</td>
                    <td className="text-left p-2 border-b border-gray-300">{user.provider}</td>
                    <td className="text-left p-2 border-b border-gray-300">
                      <Link 
                        to={`/users/${user.id}`} // Use Link for SPA navigation
                        className="text-blue-600 hover:underline"
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

        {!loading && !error && users.length === 0 && (
          <p className="mt-5">No users found.</p>
        )}

        <div className="mt-5 pt-3 border-t border-gray-200">
          <p>
            <Link 
              to="/users/new" // Use Link for SPA navigation
              className="text-blue-600 hover:underline"
            >
              Create New User
            </Link>
            {' | '}
            <Link 
              to="/"
              className="text-blue-600 hover:underline"
            >
              Home
            </Link>
          </p>
        </div>
        </div>
      </div>
    </div>
  );
}

// Removed default export: export default UsersListPage;
