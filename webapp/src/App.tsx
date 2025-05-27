import React, { useState } from 'react'; // Ensure React is in scope for JSX
import { BrowserRouter, Routes, Route, Link, Navigate } from 'react-router-dom';
import './App.css';

// Import the components that will be used as pages
import { UsersListPage } from './components/UsersListPage'; // Updated import
import { CreateUserPage } from './components/CreateUserPage'; 
import { UserInfoPage } from './components/UserInfoPage'; 

// Sample data is no longer needed here for page props, but might be used for other examples if any.
// const sampleUser = { id: '1', username: 'john.doe', email: 'john.doe@example.com', provider: 'local', name: 'John Doe', picture: '', createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() };

function App() {
  // managementPrefix is no longer needed for client-side routing.
  // API calls use absolute paths. Non-SPA links (if any) would need it,
  // but current components primarily use <Link> or navigate.

  return (
    <BrowserRouter>
      <div className="min-h-screen bg-gray-100">
        <nav className="bg-blue-300 text-white p-4 shadow-md">
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
                <h1 className="text-3xl font-bold mb-4">Welcome to
                    <br />
                    <span className="text-5xl font-bold bg-gradient-to-r from-orange-400 to-orange-600 bg-clip-text text-transparent">
                        Taronja Gateway
                    </span>
                    <br />
                    dashboard
                    </h1>
              </div>
            } />
          </Routes>
        </main>

        <footer className="text-center p-4 text-gray-600 text-sm">
          <p>Vite + React + TypeScript + Tailwind CSS v4</p>
        </footer>
      </div>
    </BrowserRouter>
  );
}

export default App;
