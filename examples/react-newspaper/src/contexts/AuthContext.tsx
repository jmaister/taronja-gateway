import React, { createContext, useContext, useState, useEffect, type ReactNode } from 'react';
import { fetchMe } from '../services/apiService'; // Only importing fetchMe since we removed logout functionality

// User Interface (can also be moved to a shared types file)
export interface User {
  id: string;
  name: string;
  email?: string;
}

interface AuthContextType {
  isAuthenticated: boolean;
  currentUser: User | null;
  isLoading: boolean;
  checkUserSession: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

const AUTH_STORAGE_KEY = 'reactNewspaperAuthStatus';

export const AuthProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(false);
  const [currentUser, setCurrentUser] = useState<User | null>(null);
  const [isLoading, setIsLoading] = useState<boolean>(true);

  const checkUserSession = async () => {
    setIsLoading(true);
    try {
      const userData = await fetchMe(); // Use apiService
      if (userData) {
        setIsAuthenticated(true);
        setCurrentUser(userData);
        localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify({ isAuthenticated: true, user: userData }));
      } else {
        // fetchMe returned null (e.g., 401)
        setIsAuthenticated(false);
        setCurrentUser(null);
        localStorage.removeItem(AUTH_STORAGE_KEY);
      }
    } catch (error) {
      // Errors from fetchMe (network, unexpected server response)
      console.error('Error during session check:', error);
      setIsAuthenticated(false);
      setCurrentUser(null);
      localStorage.removeItem(AUTH_STORAGE_KEY);
      // If error is an ApiError, you could potentially inspect error.status
      // For now, any error during fetchMe leads to logged-out state.
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    const storedAuthStatus = localStorage.getItem(AUTH_STORAGE_KEY);
    if (storedAuthStatus) {
      try {
        const { isAuthenticated: storedIsAuthenticated, user: storedUser } = JSON.parse(storedAuthStatus);
        if (storedIsAuthenticated && storedUser) {
          setIsAuthenticated(true);
          setCurrentUser(storedUser);
        }
      } catch (e) {
        console.error("Error parsing auth status from localStorage", e);
        localStorage.removeItem(AUTH_STORAGE_KEY);
      }
    }
    checkUserSession();
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
