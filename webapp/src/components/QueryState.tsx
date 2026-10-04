import type { ReactNode } from 'react';
import { Button } from './ui/Button';
import { Card, CardContent } from './ui/Card';

interface QueryStateProps {
    isLoading: boolean;
    error: unknown;
    onRetry?: () => void;
    errorTitle?: string;
    loadingText?: string;
    children: ReactNode;
}

export function QueryState({
    isLoading,
    error,
    onRetry,
    errorTitle = 'Error loading data',
    loadingText = 'Loading…',
    children,
}: QueryStateProps) {
    if (error) {
        return (
            <Card className="border-danger/30 bg-danger/5" role="alert">
                <CardContent className="py-4">
                    <div className="flex items-center">
                        <div>
                            <h3 className="font-medium text-danger">{errorTitle}</h3>
                            <p className="mt-1 text-sm text-danger/80">
                                {error instanceof Error ? error.message : 'Unknown error'}
                            </p>
                        </div>
                        {onRetry && (
                            <div className="ml-auto">
                                <Button variant="danger" size="sm" onClick={onRetry}>
                                    Retry
                                </Button>
                            </div>
                        )}
                    </div>
                </CardContent>
            </Card>
        );
    }
    if (isLoading) {
        return <div className="py-12 text-center text-sm text-muted-fg">{loadingText}</div>;
    }
    return <>{children}</>;
}

export function SkeletonRows({ rows = 4 }: { rows?: number }) {
    return (
        <div className="p-6 space-y-3">
            {[...Array(rows)].map((_, i) => (
                <div key={i} className="animate-pulse h-8 rounded bg-muted"></div>
            ))}
        </div>
    );
}

export function InlineQueryState({
    isLoading,
    error,
    errorMessage,
    children,
}: {
    isLoading: boolean;
    error: unknown;
    errorMessage: string;
    children: ReactNode;
}) {
    if (isLoading) return <SkeletonRows />;
    if (error) {
        return (
            <div className="p-6 text-sm text-danger" role="alert">
                {errorMessage}
            </div>
        );
    }
    return <>{children}</>;
}
