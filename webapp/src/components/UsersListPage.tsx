import { useState, useEffect } from 'react'; // Removed React import
import { Link } from 'react-router-dom'; 
import { fetchUsers, User } from '../services/api'; 

// Props for UsersListPage
interface UsersListPageProps {
  // Props are empty for now
}

export function UsersListPage({}: UsersListPageProps): JSX.Element {
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
    <div className="font-sans m-5 bg-gray-100 text-gray-800 min-h-screen flex flex-col items-center">
      <div className="bg-white p-5 rounded-lg shadow-md max-w-3xl w-full my-5">
        <h1 className="text-2xl font-semibold text-gray-800 border-b-2 border-gray-200 pb-2 mb-4">
          User List
        </h1>

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
  );
};

// Removed default export: export default UsersListPage;
