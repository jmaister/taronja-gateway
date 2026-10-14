import React, { createContext, useContext, useState, useEffect, useRef, type ReactNode } from 'react';
import { fetchMe } from '../services/apiService'; // Only importing fetchMe since we removed logout functionality

// User Interface (can also be moved to a shared types file)
export interface User {
  username: string;
  email?: string;
}

interface AuthContextType {
  isAuthenticated: boolean;
  currentUser: User | null;
  isLoading: boolean;
  checkUserSession: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

// Check session every 5 minutes (300000 ms)
const SESSION_CHECK_INTERVAL = 300000;

export const AuthProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(false);
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);
  const intervalRef = useRef<number | null>(null);

  // isLoading starts true and only covers the first check: background checks
  // must not swap protected pages for the loading screen.
  // State is only set in the promise callbacks, after fetchMe resolves.
  const checkUserSession = () =>
    fetchMe()
      .then((userData) => {
        // fetchMe returns null on 401
        setIsAuthenticated(userData !== null);
        setCurrentUser(userData);
      })
      .catch((error) => {
        // Errors from fetchMe (network, unexpected server response)
        console.error('Error during session check:', error);
        setIsAuthenticated(false);
        setCurrentUser(null);
      })
      .finally(() => setIsLoading(false));

  useEffect(() => {
    // Initial session check
    checkUserSession();

    // Set up periodic session checks
    intervalRef.current = setInterval(() => {
      checkUserSession();
    }, SESSION_CHECK_INTERVAL);

    // Cleanup interval on unmount
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, []);

  return (
    <AuthContext.Provider value={{ isAuthenticated, currentUser, isLoading, checkUserSession }}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = (): AuthContextType => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};
