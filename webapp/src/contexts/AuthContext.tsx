import { createContext, useContext, useState, useEffect, useRef, type ReactNode } from 'react';

// User Interface based on the API response from /me endpoint
// TODO: Generate using openapi-generator-cli or similar tool
export interface User {
    authenticated: boolean;
    username: string;
    email?: string;
    name?: string;
    picture?: string;
    givenName?: string;
    familyName?: string;
    provider?: string;
    timestamp: string;
    isAdmin: boolean;
}

interface AuthContextType {
    isAuthenticated: boolean;
    currentUser: User | null;
    isLoading: boolean;
    checkUserSession: () => Promise<void>;
    logout: () => Promise<void>;
    // Permission utilities
    isAdmin: boolean;
    isGuest: boolean;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

// Check session every 5 minutes (300000 ms)
const SESSION_CHECK_INTERVAL = 300000;

export const AuthProvider = ({ children }: { children: ReactNode }) => {
    const [isAuthenticated, setIsAuthenticated] = useState<boolean>(false);
    const [currentUser, setCurrentUser] = useState<User | null>(null);
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const intervalRef = useRef<NodeJS.Timeout | null>(null);

    // Computed permission values
    const isAdmin = isAuthenticated && currentUser?.isAdmin === true;
    const isGuest = !isAuthenticated;

    const checkUserSession = async () => {
        setIsLoading(true);
        try {
            const userData = await fetchMe(); // Use apiService
            if (userData && userData.authenticated) {
                setIsAuthenticated(true);
                setCurrentUser(userData);
            } else {
                // fetchMe returned null or user not authenticated
                setIsAuthenticated(false);
                setCurrentUser(null);
            }
        } catch (error) {
            // Errors from fetchMe (network, unexpected server response)
            console.error('Error during session check:', error);
            setIsAuthenticated(false);
            setCurrentUser(null);
            // If error is an ApiError, you could potentially inspect error.status
            // For now, any error during fetchMe leads to logged-out state.
        } finally {
            setIsLoading(false);
        }
    };

    const logout = async () => {
        try {
            // Call the logout endpoint which will clear the session cookie
            const response = await fetch('/_/logout', {
                method: 'GET',
                credentials: 'same-origin'
            });

            // Update state regardless of response status
            setIsAuthenticated(false);
            setCurrentUser(null);

            // Check if there's a redirect location in the response
            if (response.redirected) {
                window.location.href = response.url;
            } else {
                // Fallback: redirect to home page
                window.location.href = '/';
            }
        } catch (error) {
            console.error('Error during logout:', error);
            // Still update state even if logout request failed
            setIsAuthenticated(false);
            setCurrentUser(null);
            // Redirect to home as fallback
            window.location.href = '/';
        }
    };

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
        <AuthContext.Provider value={{ 
            isAuthenticated, 
            currentUser, 
            isLoading, 
            checkUserSession, 
            logout,
            isAdmin,
            isGuest,
        }}>
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

/**
 * Fetches the current user's data from the backend.
 * @returns {Promise<User | null>} The user data if authenticated, otherwise null.
 * @throws {ApiError} If there's a network error or unexpected server response.
 */
export const fetchMe = async (): Promise<User | null> => {
    try {
        const response = await fetch('/_/me', {
            method: 'GET',
            credentials: 'same-origin', // Include cookies for session authentication
            headers: {
                'Accept': 'application/json',
            }
        });

        if (response.ok) { // Status 200-299
            const userData = await response.json() as User;
            // Validate that we have a proper user response
            if (userData && typeof userData.authenticated === 'boolean') {
                return userData;
            } else {
                throw new ApiError('Invalid user data format received from server.', response.status);
            }
        }

        if (response.status === 401) { // Not authenticated
            return null;
        }

        // Other non-ok responses
        throw new ApiError(`Failed to fetch user data. Status: ${response.status}`, response.status);

    } catch (error) {
        if (error instanceof ApiError) {
            throw error; // Re-throw ApiError
        }
        // Network errors or other unexpected errors during fetch/JSON parsing
        console.error('Network or parsing error in fetchMe:', error);
        throw new ApiError('Network error or failed to parse user data.', error instanceof Error ? undefined : undefined);
    }
};

export class ApiError extends Error {
    public status?: number;

    constructor(message: string, status?: number) {
        super(message);
        this.name = 'ApiError';
        this.status = status;
    }
}

/**
 * Utility functions for working with user data
 */
export const getUserDisplayName = (user: User | null): string => {
    if (!user) {
        return 'Guest';
    }            

    // Priority: name > givenName + familyName > username
    if (user.name) {
        return user.name;
    }
    if (user.givenName || user.familyName) {
        return [user.givenName, user.familyName].filter(Boolean).join(' ').trim();
    }
    return user.username;
};

export const getUserAvatar = (user: User | null): string | null => {
    return user?.picture || null;
};

export const getUserInitials = (user: User | null): string => {
    if (!user) {
        return 'G';
    }

    const displayName = getUserDisplayName(user);
    const words = displayName.split(' ').filter(Boolean);

    if (words.length >= 2) {
        return (words[0][0] + words[words.length - 1][0]).toUpperCase();
    } else if (words.length === 1) {
        return words[0].substring(0, 2).toUpperCase();
    }

    return user.username.substring(0, 2).toUpperCase();
};

/**
 * HOC for protecting components that require authentication
 */
export const withAuth = <P extends object>(
    Component: React.ComponentType<P>,
    fallback?: React.ComponentType
) => {
    return (props: P) => {
        const { isAuthenticated, isLoading } = useAuth();
        
        if (isLoading) {
            return (
                <div className="flex items-center justify-center p-8">
                    <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
                </div>
            );
        }
        
        if (!isAuthenticated) {
            if (fallback) {
                const Fallback = fallback;
                return <Fallback />;
            }
            
            return (
                <div className="flex flex-col items-center justify-center p-8 space-y-4">
                    <h2 className="text-xl font-semibold text-gray-700">Authentication Required</h2>
                    <p className="text-gray-500">Please log in to access this content.</p>
                    <a 
                        href="/_/login" 
                        className="bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded"
                    >
                        Login
                    </a>
                </div>
            );
        }
        
        return <Component {...props} />;
    };
};

/**
 * HOC for protecting components that require admin privileges
 */
export const withAdminAuth = <P extends object>(
    Component: React.ComponentType<P>,
    fallback?: React.ComponentType
) => {
    return (props: P) => {
        const { isAuthenticated, currentUser, isLoading } = useAuth();
        
        if (isLoading) {
            return (
                <div className="flex items-center justify-center p-8">
                    <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-red-500"></div>
                </div>
            );
        }
        
        if (!isAuthenticated) {
            if (fallback) {
                const Fallback = fallback;
                return <Fallback />;
            }
            
            return (
                <div className="flex flex-col items-center justify-center p-8 space-y-4">
                    <h2 className="text-xl font-semibold text-gray-700">Authentication Required</h2>
                    <p className="text-gray-500">Please log in to access this content.</p>
                    <a 
                        href="/_/login" 
                        className="bg-blue-500 hover:bg-blue-700 text-white font-bold py-2 px-4 rounded"
                    >
                        Login
                    </a>
                </div>
            );
        }
        
        if (!currentUser?.isAdmin) {
            if (fallback) {
                const Fallback = fallback;
                return <Fallback />;
            }
            
            return (
                <div className="flex flex-col items-center justify-center p-8 space-y-4">
                    <h2 className="text-xl font-semibold text-red-600">Access Denied</h2>
                    <p className="text-gray-500">You need administrator privileges to access this content.</p>
                </div>
            );
        }
        
        return <Component {...props} />;
    };
};
