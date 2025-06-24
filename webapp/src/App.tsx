import React from 'react';
import { BrowserRouter, Routes, Route, Navigate, Outlet, Link } from 'react-router-dom';
import './App.css'; // Keep if it has essential base styles not covered by Tailwind preflight

// Layout and Page Components
import MainLayout from './components/layout/MainLayout';
import { UsersListPage } from './components/UsersListPage';
import { CreateUserPage } from './components/CreateUserPage';
import { UserInfoPage } from './components/UserInfoPage';

// A component to group routes under MainLayout
const AdminLayoutRoutes: React.FC = () => (
  <MainLayout>
    <Outlet /> {/* Child routes will render here through MainLayout's children prop */}
  </MainLayout>
);

// Simple 404 Page component
const NotFoundPage: React.FC = () => (
  <div className="min-h-screen flex flex-col items-center justify-center bg-gray-100 text-center p-4">
    <h1 className="text-4xl font-bold text-slate-700 mb-4">404 - Page Not Found</h1>
    <p className="text-lg text-slate-600 mb-8">
      Sorry, the page you are looking for does not exist or has been moved.
    </p>
    <Link
      to="/users" // Link back to the main admin page
      className="px-6 py-3 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 transition-colors"
    >
      Go to User Management
    </Link>
    <footer className="absolute bottom-4 text-center p-4 text-gray-500 text-xs">
        <p>Taronja Gateway Admin</p>
    </footer>
  </div>
);


function App() {
  return (
    <BrowserRouter basename="/_/admin">
      <Routes>
        {/* Routes that use the MainLayout */}
        <Route element={<AdminLayoutRoutes />}>
          <Route path="/users" element={<UsersListPage />} />
          <Route path="/users/new" element={<CreateUserPage />} />
          <Route path="/users/:userId" element={<UserInfoPage />} />
          {/* Add other admin routes that should use MainLayout here */}
        </Route>

        {/* Root path redirect to /users */}
        <Route path="/" element={<Navigate replace to="/users" />} />

        {/* Catch-all for unmatched routes */}
        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;
