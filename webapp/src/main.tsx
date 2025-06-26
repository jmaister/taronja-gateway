import React from 'react'; // Ensure React is in scope for JSX
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css'; // Global styles including Tailwind and daisyUI
import App from './App'; // .tsx extension is usually omitted in imports
import { AuthProvider } from './contexts/AuthContext';

const rootElement = document.getElementById('root');

if (rootElement) {
  createRoot(rootElement).render(
    <StrictMode>
      <AuthProvider>
        <App />
      </AuthProvider>
    </StrictMode>,
  );
} else {
  console.error("Failed to find the root element");
}
