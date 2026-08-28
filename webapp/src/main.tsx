import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './index.css'; // Global styles including Tailwind and daisyUI
import App from './App'; // .tsx extension is usually omitted in imports
import { TaronjaAuthProvider } from 'taronja-gateway-react-sdk';
import { ThemeProvider } from './contexts/ThemeContext';
import { QueryClientProvider } from '@tanstack/react-query';
import { ReactQueryDevtools } from '@tanstack/react-query-devtools';
import { queryClient } from './services/services';
import { ErrorBoundary } from './components/ErrorBoundary';


const rootElement = document.getElementById('root');

if (rootElement) {
  createRoot(rootElement).render(
    <StrictMode>
      <ErrorBoundary>
        <QueryClientProvider client={queryClient}>
          <ThemeProvider>
            <TaronjaAuthProvider>
              <App />
            </TaronjaAuthProvider>
          </ThemeProvider>
          <ReactQueryDevtools initialIsOpen={false} />
        </QueryClientProvider>
      </ErrorBoundary>
    </StrictMode>,
  );
} else {
  console.error("Failed to find the root element");
}
