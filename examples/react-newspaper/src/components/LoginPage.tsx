import React, { useState } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';

const LoginPage: React.FC = () => {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const { login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

  const from = location.state?.from?.pathname || '/';

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(''); 
    try {
      await login(username, password); 
      navigate(from, { replace: true });
    } catch (err) {
      setError('Failed to login. Please try again.'); 
      console.error(err);
    }
  };

  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-slate-100 p-4">
      <div className="p-8 sm:p-10 max-w-md w-full bg-white shadow-2xl rounded-xl">
        <div className="text-center mb-8">
          <h1 className="text-4xl font-serif font-bold text-slate-800">The React Times</h1>
          <p className="text-slate-600 mt-2">Access your account</p>
        </div>
        {error && <p className="bg-red-100 text-red-700 p-3 rounded-md text-center mb-6">{error}</p>}
        <form onSubmit={handleSubmit} className="space-y-6">
          <div>
            <label className="block text-slate-700 text-sm font-semibold mb-2" htmlFor="username">
              Username
            </label>
            <input
              type="text"
              id="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="appearance-none border border-slate-300 rounded-md w-full py-3 px-4 text-slate-700 leading-tight focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent placeholder-slate-400"
              placeholder="e.g., user"
              required
            />
          </div>
          <div>
            <label className="block text-slate-700 text-sm font-semibold mb-2" htmlFor="password">
              Password
            </label>
            <input
              type="password"
              id="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="appearance-none border border-slate-300 rounded-md w-full py-3 px-4 text-slate-700 mb-1 leading-tight focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent placeholder-slate-400"
              placeholder="e.g., pass"
              required
            />
          </div>
          <div>
            <button
              type="submit"
              className="w-full bg-slate-700 hover:bg-slate-800 text-white font-bold py-3 px-4 rounded-md focus:outline-none focus:shadow-outline transition duration-150 ease-in-out"
            >
              Sign In
            </button>
          </div>
          <p className="text-center text-slate-500 text-xs mt-4">
            (For this demo, any username/password will work)
          </p>
        </form>
      </div>
    </div>
  );
};

export default LoginPage;
