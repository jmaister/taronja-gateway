import { useState, useEffect } from 'react'; // Removed React import
import { useParams, Link } from 'react-router-dom'; 
import { fetchUser, User } from '../services/api'; 
import { UserTokensSection } from '../components/UserTokensSection'; 

interface UserInfoPageProps {
  // Props are empty for now
}

export function UserInfoPage({}: UserInfoPageProps) {
  const { userId } = useParams<{ userId: string }>(); 
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

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

  useEffect(() => {
    if (!userId) {
      setUser(null);
      setError("No user ID provided. Please select a user.");
      setLoading(false);
      return;
    }

    const loadUser = async () => {
      try {
        setLoading(true);
        setError(null); 
        const fetchedUser = await fetchUser(userId);
        setUser(fetchedUser);
      } catch (err: any) {
        setError(err.message || 'Failed to fetch user details');
        setUser(null);
      } finally {
        setLoading(false);
      }
    };

    loadUser();
  }, [userId]); 

  return (
    <div className="font-sans m-5 bg-gray-100 text-gray-800 min-h-screen flex flex-col items-center py-5">
      <div className="bg-white p-5 rounded-lg shadow-md max-w-3xl w-full my-5">
        <h1 className="text-2xl font-semibold text-gray-800 border-b-2 border-gray-200 pb-2 mb-4">
          User Information
        </h1>

        {loading && (
          <p className="text-blue-500">Loading user details...</p>
        )}

        {error && !loading && (
          <p className="text-red-500 font-bold mb-4">{error}</p>
        )}

        {!loading && !error && user && (
          <div className="mb-6">
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-gray-600">ID:</span> {user.id}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-gray-600">Username:</span> {user.username}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-gray-600">Email:</span> {user.email}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-gray-600">Name:</span> {user.name || 'N/A'}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-gray-600 align-top">Picture:</span>
              {user.picture ? (
                <img src={user.picture} alt="User Picture" className="max-w-[100px] max-h-[100px] align-middle rounded inline-block" />
              ) : (
                'N/A'
              )}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-gray-600">Provider:</span> {user.provider}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-gray-600">Created At:</span> {formatDate(user.createdAt)}
            </div>
            <div className="mb-2.5">
              <span className="inline-block min-w-[100px] font-semibold text-gray-600">Updated At:</span> {formatDate(user.updatedAt)}
            </div>

            {/* User Tokens Section */}
            <UserTokensSection userId={user.id} />
          </div>
        )}
        
        {!loading && !error && !user && (
          <p className="mt-5">User details are not available.</p>
        )}

        <div className="mt-6 pt-4 border-t border-gray-200">
          <p>
            <Link 
              to="/users" 
              className="text-blue-600 hover:underline mr-2"
            >
              Back to List
            </Link> | 
            <Link 
              to="/users/new" 
              className="text-blue-600 hover:underline mx-2"
            >
              Create Another User
            </Link> | 
            {/* The "Secret Page" link might be an external link or handled differently. */}
            {/* If it's external, `a` tag is fine. If SPA, it needs a route. */}
            <a 
              href="/secret/index.html" 
              className="text-blue-600 hover:underline mx-2"
            >
              Secret Page
            </a> | 
            <Link 
              to="/"
              className="text-blue-600 hover:underline ml-2"
            >
              Home
            </Link>
          </p>
        </div>
      </div>
    </div>
  );
};

// Removed default export: export default UserInfoPage;
