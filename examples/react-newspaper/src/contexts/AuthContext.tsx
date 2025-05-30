import React, { createContext, useContext, useState, useEffect, type ReactNode } from 'react';

interface AuthContextType {
  isAuthenticated: boolean;
  login: (username?: string, password?: string) => Promise<void>; // Made params optional for mock
  logout: () => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(false);

  useEffect(() => {
    // Check localStorage for persisted login state
    const storedUser = localStorage.getItem('reactNewspaperUser');
    if (storedUser) {
      setIsAuthenticated(true);
    }
  }, []);

  const login = async (username?: string, password?: string): Promise<void> => {
    // Mock login: accepts any username/password or specific ones
    // For simplicity, we'll just simulate a successful login
    console.log('Attempting login with:', username, password); // For debugging
    return new Promise((resolve) => {
      setTimeout(() => {
        setIsAuthenticated(true);
        // Persist mock user data/token
        localStorage.setItem('reactNewspaperUser', JSON.stringify({ username: username || 'mockUser', token: 'mockToken123' }));
        resolve();
      }, 500); // Simulate network delay
    });
  };

  const logout = () => {
    setIsAuthenticated(false);
    localStorage.removeItem('reactNewspaperUser');
    // Optionally, redirect here or let the component handle it
  };

  return (
    <AuthContext.Provider value={{ isAuthenticated, login, logout }}>
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
