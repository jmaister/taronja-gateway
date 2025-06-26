import { useState, FormEvent } from 'react'; // Removed React import as it's not directly used after JSX transform
import { useNavigate, Link } from 'react-router-dom'; 
import { createUser, UserCreateRequest, User } from '../services/api'; 

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
    <div className="font-sans m-5 bg-gray-100 min-h-screen flex flex-col items-center py-10">
      <div className="bg-white p-8 rounded-lg shadow-md max-w-md w-full">
        <h1 className="text-gray-800 text-center text-2xl mb-6 font-semibold">
          Create New User
        </h1>

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

        <div className="text-center mt-8 pt-6 border-t border-gray-300">
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
  );
};

// Removed default export: export default CreateUserPage;
