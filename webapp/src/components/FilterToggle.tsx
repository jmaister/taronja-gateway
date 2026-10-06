import type { ReactNode } from 'react';

// A value the user can click to filter by it; clicking it again while it is
// the active filter clears it.
export function FilterToggle({
    label,
    active,
    onToggle,
    className = '',
    children,
}: {
    label: string;
    active: boolean;
    onToggle: () => void;
    className?: string;
    children: ReactNode;
}) {
    return (
        <button
            type="button"
            onClick={onToggle}
            aria-pressed={active}
            title={active ? `Clear filter: ${label}` : `Filter by ${label}`}
            className={`cursor-pointer rounded text-left transition-colors hover:bg-muted/70 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary ${
                active ? 'bg-primary/10 font-semibold text-primary' : ''
            } ${className}`}
        >
            {children}
        </button>
    );
}
