import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css'; // Global styles including Tailwind and daisyUI
import App from './App'; // .tsx extension is usually omitted in imports
import { AuthProvider } from './contexts/AuthContext';
import { QueryClientProvider } from '@tanstack/react-query';
import { ReactQueryDevtools } from '@tanstack/react-query-devtools';
import { queryClient } from './services/services';


const rootElement = document.getElementById('root');

if (rootElement) {
  createRoot(rootElement).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <App />
        </AuthProvider>
        <ReactQueryDevtools initialIsOpen={false} />
      </QueryClientProvider>
    </StrictMode>,
  );
} else {
  console.error("Failed to find the root element");
}
