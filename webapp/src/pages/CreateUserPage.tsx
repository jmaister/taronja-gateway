import { useState, FormEvent } from 'react'; // Removed React import as it's not directly used after JSX transform
import { useNavigate, Link } from 'react-router-dom'; 
import { createUser, UserCreateRequest } from '../services/api'; 

interface CreateUserPageProps {
  // Props are empty for now
}

type MessageType = 'success' | 'error' | '';

export function CreateUserPage({}: CreateUserPageProps) {
  const navigate = useNavigate(); 
  const [formData, setFormData] = useState<UserCreateRequest>({
    username: '',
    email: '',
    password: '',
  });
  const [message, setMessage] = useState<string>('');
  const [messageType, setMessageType] = useState<MessageType>('');
  const [loading, setLoading] = useState<boolean>(false);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value } = e.target;
    setFormData(prevFormData => ({
      ...prevFormData,
      [name]: value,
    }));
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage('');
    setMessageType('');
    setLoading(true);

    // UserData is now directly from formData state
    try {
      const createdUser = await createUser(formData);
      setMessage(`User created successfully! ID: ${createdUser.id}, Username: ${createdUser.username}. Redirecting...`);
      setMessageType('success');
      setFormData({ // Reset form
        username: '',
        email: '',
        password: '',
      });
      
      setTimeout(() => {
        navigate(`/users/${createdUser.id}`); 
      }, 1500);

    } catch (err: any) {
      console.error('Submission error:', err);
      setMessage(err.message || 'An unexpected error occurred. Please try again.');
      setMessageType('error');
    } finally {
      setLoading(false);
    }
  };

  let messageClasses = "p-3 mb-5 rounded-md text-sm";
  if (messageType === 'success') {
    messageClasses += " bg-green-100 text-green-700 border border-green-300";
  } else if (messageType === 'error') {
    messageClasses += " bg-red-100 text-red-700 border border-red-300";
  }

  return (
    <div className="w-full p-6">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-gray-900">User Management</h1>
        <p className="text-gray-600 mt-2">Create a new user account</p>
      </div>

      <div className="bg-white rounded-lg shadow-lg overflow-hidden max-w-2xl">
        <div className="px-6 py-4 border-b border-gray-200">
          <h2 className="text-xl font-semibold text-gray-800">Create New User</h2>
        </div>
        
        <div className="p-6">

        {message && messageType && (
          <div className={messageClasses} role="alert">
            {message}
          </div>
        )}

        <form onSubmit={handleSubmit}>
          <div>
            <label htmlFor="username" className="block mb-2 font-bold text-gray-700">
              Username:
            </label>
            <input
              type="text"
              id="username"
              name="username"
              value={formData.username}
              onChange={handleChange}
              required
              disabled={loading}
              className="w-full p-2 mb-5 border border-gray-300 rounded box-border focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:bg-gray-100"
            />
          </div>
          <div>
            <label htmlFor="email" className="block mb-2 font-bold text-gray-700">
              Email:
            </label>
            <input
              type="email"
              id="email"
              name="email"
              value={formData.email}
              onChange={handleChange}
              required
              disabled={loading}
              className="w-full p-2 mb-5 border border-gray-300 rounded box-border focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:bg-gray-100"
            />
          </div>
          <div>
            <label htmlFor="password" className="block mb-2 font-bold text-gray-700">
              Password:
            </label>
            <input
              type="password"
              id="password"
              name="password"
              value={formData.password}
              onChange={handleChange}
              required
              disabled={loading}
              className="w-full p-2 mb-5 border border-gray-300 rounded box-border focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:bg-gray-100"
            />
          </div>
          <button
            type="submit"
            disabled={loading}
            className="bg-blue-600 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded w-full text-base disabled:bg-blue-400 disabled:cursor-not-allowed"
          >
            {loading ? 'Creating...' : 'Create User'}
          </button>
        </form>

        <div className="mt-8 pt-6 border-t border-gray-200 text-center">
          <Link 
            to="/"
            className="text-blue-600 hover:underline mr-5"
          >
            &larr; Home
          </Link>
          <Link 
            to="/users"
            className="text-blue-600 hover:underline"
          >
            View All Users
          </Link>
        </div>
        </div>
      </div>
    </div>
  );
}

// Removed default export: export default CreateUserPage;
