import { useEffect, type ComponentType, type ReactNode } from 'react';
import type { LoginActionOptions } from './auth';
import { useTaronjaAuth } from './auth';

export interface RequireAuthProps {
    children: ReactNode;
    fallback?: ReactNode;
    loadingFallback?: ReactNode;
    redirectToLogin?: boolean;
    loginOptions?: LoginActionOptions;
}

export interface RequireAdminProps extends RequireAuthProps {
    unauthorizedFallback?: ReactNode;
}

export function RequireAuth({
    children,
    fallback = null,
    loadingFallback = null,
    redirectToLogin = false,
    loginOptions,
}: RequireAuthProps) {
    const { isAuthenticated, isLoading, login } = useTaronjaAuth();

    useEffect(() => {
        if (!isLoading && !isAuthenticated && redirectToLogin) {
            login(loginOptions);
        }
    }, [isAuthenticated, isLoading, login, loginOptions, redirectToLogin]);

    if (isLoading) {
        return <>{loadingFallback}</>;
    }

    if (!isAuthenticated) {
        return <>{fallback}</>;
    }

    return <>{children}</>;
}

export function RequireAdmin({
    children,
    fallback = null,
    loadingFallback = null,
    unauthorizedFallback = null,
    redirectToLogin = false,
    loginOptions,
}: RequireAdminProps) {
    const { isAuthenticated, isAdmin, isLoading, login } = useTaronjaAuth();

    useEffect(() => {
        if (!isLoading && !isAuthenticated && redirectToLogin) {
            login(loginOptions);
        }
    }, [isAuthenticated, isLoading, login, loginOptions, redirectToLogin]);

    if (isLoading) {
        return <>{loadingFallback}</>;
    }

    if (!isAuthenticated) {
        return <>{fallback}</>;
    }

    if (!isAdmin) {
        return <>{unauthorizedFallback}</>;
    }

    return <>{children}</>;
}

export function withRequireAuth<P extends object>(
    Component: ComponentType<P>,
    options?: Omit<RequireAuthProps, 'children'>,
) {
    return function TaronjaRequireAuthComponent(props: P) {
        return (
            <RequireAuth {...options}>
                <Component {...props} />
            </RequireAuth>
        );
    };
}

export function withRequireAdmin<P extends object>(
    Component: ComponentType<P>,
    options?: Omit<RequireAdminProps, 'children'>,
) {
    return function TaronjaRequireAdminComponent(props: P) {
        return (
            <RequireAdmin {...options}>
                <Component {...props} />
            </RequireAdmin>
        );
    };
}
