import React, { useState } from 'react'; // Ensure React is in scope for JSX
import { BrowserRouter, Routes, Route, Link, Navigate } from 'react-router-dom';
import reactLogo from './assets/react.svg';
import viteLogo from '/vite.svg';
import './App.css';

// Import the components that will be used as pages
import { UsersListPage } from './components/UsersListPage'; // Updated import
import { CreateUserPage } from './components/CreateUserPage'; 
import { UserInfoPage } from './components/UserInfoPage'; 

// Sample data is no longer needed here for page props, but might be used for other examples if any.
// const sampleUser = { id: '1', username: 'john.doe', email: 'john.doe@example.com', provider: 'local', name: 'John Doe', picture: '', createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() };

function App() {
  const [count, setCount] = useState<number>(0);

  // managementPrefix is no longer needed for client-side routing.
  // API calls use absolute paths. Non-SPA links (if any) would need it,
  // but current components primarily use <Link> or navigate.
  const basename = "/_/admin"; // Set the basename based on management.prefix

  return (
    <BrowserRouter basename={basename}>
      <div className="min-h-screen bg-gray-100">
        <nav className="bg-blue-600 text-white p-4 shadow-md">
          <div className="container mx-auto flex justify-between items-center">
            <Link to="/users" className="text-xl font-bold hover:cursor-pointer">User Management App</Link>
            <div>
              <Link to="/users" className="px-3 py-2 rounded hover:bg-blue-700">Users List</Link>
              <Link to="/users/new" className="px-3 py-2 rounded hover:bg-blue-700">Create User</Link>
            </div>
          </div>
        </nav>

        <main className="container mx-auto p-4">
          <Routes>
            <Route path="/users" element={<UsersListPage />} />
            <Route path="/users/new" element={<CreateUserPage />} />
            <Route path="/users/:userId" element={<UserInfoPage />} />
            <Route path="/" element={<Navigate replace to="/users" />} />
            <Route path="*" element={
              <div className="text-center p-10">
                <h1 className="text-3xl font-bold mb-4">404 - Not Found</h1>
                <p className="mb-4">The page you are looking for does not exist.</p>
                <Link to="/users" className="text-blue-600 hover:underline">Go to Users List</Link>
              </div>
            } />
          </Routes>
        </main>

        <footer className="text-center p-4 text-gray-600 text-sm">
          <p>Vite + React + TypeScript + Tailwind CSS v4</p>
          <div className="card mt-4 p-4 bg-white shadow rounded max-w-sm mx-auto">
            <button onClick={() => setCount((prevCount) => prevCount + 1)} className="bg-blue-500 hover:bg-blue-600 text-white font-bold py-2 px-4 rounded">
              Vite Counter: {count}
            </button>
          </div>
          <p className="mt-2 text-xs">
            Edit <code>src/App.tsx</code> and save to test HMR.
          </p>
          <p className="read-the-docs mt-2 text-xs">
            Click on the Vite and React logos to learn more.
          </p>
          <div className="flex justify-center items-center space-x-4 mt-2">
            <a href="https://vite.dev" target="_blank" rel="noopener noreferrer">
              <img src={viteLogo} className="h-10 w-10" alt="Vite logo" />
            </a>
            <a href="https://react.dev" target="_blank" rel="noopener noreferrer">
              <img src={reactLogo} className="h-10 w-10" alt="React logo" />
            </a>
          </div>
        </footer>
      </div>
    </BrowserRouter>
  );
}

export default App;
