import {
    createContext,
    useContext,
    useEffect,
    useRef,
    useState,
    type ComponentType,
    type ReactNode,
} from 'react';
import { createTaronjaClient, isAuthenticatedUser, type LogoutOptions, type TaronjaClient, type TaronjaClientOptions } from './client';
import type { CurrentUser } from './types';

export const DEFAULT_SESSION_POLL_INTERVAL = 300000;

export interface LoginActionOptions {
    redirectTo?: string;
}

export interface LogoutActionOptions extends LogoutOptions {
    redirect?: boolean;
}

export interface TaronjaAuthContextValue {
    client: TaronjaClient;
    currentUser: CurrentUser | null;
    isAuthenticated: boolean;
    isLoading: boolean;
    isAdmin: boolean;
    isGuest: boolean;
    error: Error | null;
    refreshSession: () => Promise<CurrentUser | null>;
    login: (options?: LoginActionOptions) => void;
    logout: (options?: LogoutActionOptions) => Promise<void>;
}

export interface TaronjaAuthProviderProps {
    children: ReactNode;
    client?: TaronjaClient;
    clientOptions?: TaronjaClientOptions;
    initialUser?: CurrentUser | null;
    checkOnMount?: boolean;
    pollIntervalMs?: number;
    clearSessionOnError?: boolean;
    logoutRedirectTo?: string;
    onAuthChange?: (user: CurrentUser | null) => void;
    onError?: (error: unknown) => void;
}

const TaronjaAuthContext = createContext<TaronjaAuthContextValue | undefined>(undefined);

export function TaronjaAuthProvider({
    children,
    client,
    clientOptions,
    initialUser = null,
    checkOnMount = true,
    pollIntervalMs = DEFAULT_SESSION_POLL_INTERVAL,
    clearSessionOnError = true,
    logoutRedirectTo,
    onAuthChange,
    onError,
}: TaronjaAuthProviderProps) {
    const defaultClientRef = useRef<TaronjaClient | null>(null);
    if (!defaultClientRef.current) {
        defaultClientRef.current = createTaronjaClient(clientOptions);
    }

    const resolvedClient = client ?? defaultClientRef.current;
    const clientRef = useRef<TaronjaClient>(resolvedClient);
    const clearSessionOnErrorRef = useRef(clearSessionOnError);
    const onAuthChangeRef = useRef(onAuthChange);
    const onErrorRef = useRef(onError);
    const logoutRedirectToRef = useRef(logoutRedirectTo);

    const [currentUser, setCurrentUser] = useState<CurrentUser | null>(isAuthenticatedUser(initialUser) ? initialUser : null);
    const [isLoading, setIsLoading] = useState<boolean>(checkOnMount);
    const [error, setError] = useState<Error | null>(null);
    const refreshSessionRef = useRef<() => Promise<CurrentUser | null>>(async () => null);

    useEffect(() => {
        clientRef.current = resolvedClient;
    }, [resolvedClient]);

    useEffect(() => {
        clearSessionOnErrorRef.current = clearSessionOnError;
    }, [clearSessionOnError]);

    useEffect(() => {
        onAuthChangeRef.current = onAuthChange;
    }, [onAuthChange]);

    useEffect(() => {
        onErrorRef.current = onError;
    }, [onError]);

    useEffect(() => {
        logoutRedirectToRef.current = logoutRedirectTo;
    }, [logoutRedirectTo]);

    refreshSessionRef.current = async () => {
        setIsLoading(true);
        try {
            const user = await clientRef.current.getCurrentUser();
            const nextUser = isAuthenticatedUser(user) ? user : null;
            setCurrentUser(nextUser);
            setError(null);
            onAuthChangeRef.current?.(nextUser);
            return nextUser;
        } catch (cause) {
            const nextError = cause instanceof Error ? cause : new Error('Failed to refresh Taronja session.');
            setError(nextError);
            if (clearSessionOnErrorRef.current) {
                setCurrentUser(null);
                onAuthChangeRef.current?.(null);
            }
            onErrorRef.current?.(cause);
            return null;
        } finally {
            setIsLoading(false);
        }
    };

    useEffect(() => {
        if (checkOnMount) {
            void refreshSessionRef.current();
        } else {
            setIsLoading(false);
        }

        if (!pollIntervalMs || pollIntervalMs <= 0) {
            return undefined;
        }

        const intervalId = setInterval(() => {
            void refreshSessionRef.current();
        }, pollIntervalMs);

        return () => {
            clearInterval(intervalId);
        };
    }, [checkOnMount, pollIntervalMs]);

    const refreshSession = async () => {
        return refreshSessionRef.current();
    };

    const login = (options?: LoginActionOptions) => {
        if (typeof window === 'undefined') {
            return;
        }

        window.location.assign(clientRef.current.getLoginUrl(options));
    };

    const logout = async (options?: LogoutActionOptions) => {
        try {
            const result = await clientRef.current.logout({
                redirectTo: options?.redirectTo ?? logoutRedirectToRef.current,
                signal: options?.signal,
            });

            setCurrentUser(null);
            setError(null);
            onAuthChangeRef.current?.(null);

            if (options?.redirect !== false && typeof window !== 'undefined') {
                window.location.assign(result.url || options?.redirectTo || logoutRedirectToRef.current || '/');
            }
        } catch (cause) {
            const nextError = cause instanceof Error ? cause : new Error('Failed to logout from Taronja Gateway.');
            setCurrentUser(null);
            setError(nextError);
            onAuthChangeRef.current?.(null);
            onErrorRef.current?.(cause);

            if (options?.redirect !== false && typeof window !== 'undefined') {
                window.location.assign(options?.redirectTo || logoutRedirectToRef.current || '/');
            }
        }
    };

    const isAuthenticated = currentUser !== null;
    const isAdmin = Boolean(currentUser?.isAdmin);
    const isGuest = !isAuthenticated;

    return (
        <TaronjaAuthContext.Provider
            value={{
                client: resolvedClient,
                currentUser,
                isAuthenticated,
                isLoading,
                isAdmin,
                isGuest,
                error,
                refreshSession,
                login,
                logout,
            }}
        >
            {children}
        </TaronjaAuthContext.Provider>
    );
}

export function useTaronjaAuth(): TaronjaAuthContextValue {
    const context = useContext(TaronjaAuthContext);
    if (!context) {
        throw new Error('useTaronjaAuth must be used within a TaronjaAuthProvider.');
    }
    return context;
}

export function useTaronjaClient(): TaronjaClient {
    return useTaronjaAuth().client;
}

export function withTaronjaAuth<P extends object>(
    Component: ComponentType<P>,
    fallback?: ComponentType,
) {
    return function TaronjaAuthenticatedComponent(props: P) {
        const { isAuthenticated, isLoading } = useTaronjaAuth();

        if (isLoading) {
            return null;
        }

        if (!isAuthenticated) {
            if (fallback) {
                const Fallback = fallback;
                return <Fallback />;
            }

            return null;
        }

        return <Component {...props} />;
    };
}

export function withTaronjaAdmin<P extends object>(
    Component: ComponentType<P>,
    fallback?: ComponentType,
) {
    return function TaronjaAdminComponent(props: P) {
        const { isAuthenticated, isAdmin, isLoading } = useTaronjaAuth();

        if (isLoading) {
            return null;
        }

        if (!isAuthenticated || !isAdmin) {
            if (fallback) {
                const Fallback = fallback;
                return <Fallback />;
            }

            return null;
        }

        return <Component {...props} />;
    };
}
