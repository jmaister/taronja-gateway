import { Component, ErrorInfo, ReactNode } from 'react';
import { Button } from './ui/Button';

interface Props {
    children: ReactNode;
}

interface State {
    error: Error | null;
}

// Catches uncaught render/lifecycle errors anywhere below it in the tree.
// Without this, an uncaught exception in any page/component unmounts the
// whole React tree and leaves the admin looking at a blank white screen
// with no way back except manually reloading the page.
export class ErrorBoundary extends Component<Props, State> {
    state: State = { error: null };

    static getDerivedStateFromError(error: Error): State {
        return { error };
    }

    componentDidCatch(error: Error, errorInfo: ErrorInfo) {
        console.error('Uncaught error in admin dashboard:', error, errorInfo);
    }

    render() {
        if (this.state.error) {
            return (
                <div className="flex min-h-screen flex-col items-center justify-center bg-bg p-4 text-center">
                    <div className="mb-4 text-5xl">⚠️</div>
                    <h1 className="mb-2 text-2xl font-semibold tracking-tight">Something went wrong</h1>
                    <p className="mb-6 max-w-md text-muted-fg" role="alert">
                        {this.state.error.message || 'An unexpected error occurred in the admin dashboard.'}
                    </p>
                    <div className="flex gap-3">
                        <Button onClick={() => this.setState({ error: null })} variant="outline">
                            Try again
                        </Button>
                        <Button onClick={() => window.location.reload()}>Reload page</Button>
                    </div>
                </div>
            );
        }

        return this.props.children;
    }
}
